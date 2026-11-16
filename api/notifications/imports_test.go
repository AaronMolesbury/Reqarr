package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"reqarr/config"
	"reqarr/model"
	"reqarr/storage"
)

func TestImportsCoverManualAndSeerrContentAndSilenceUpgrades(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-time.Hour).Format(time.RFC3339Nano)
	fresh := now.Add(time.Minute).Format(time.RFC3339Nano)
	movies := `[{"id":1,"tmdbId":101,"hasFile":true},{"id":2,"tmdbId":102,"hasFile":false}]`
	episodes := `[{"id":10,"seriesId":7,"seasonNumber":1,"episodeNumber":1,"hasFile":true},{"id":11,"seriesId":7,"seasonNumber":1,"episodeNumber":2,"hasFile":false}]`
	movieHistory := fmt.Sprintf(`[{"id":100,"movieId":1,"eventType":"downloadFolderImported","date":%q}]`, old)
	episodeHistory := fmt.Sprintf(`[{"id":200,"seriesId":7,"episodeId":10,"eventType":"downloadFolderImported","date":%q}]`, old)
	episodeCalls := 0
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test" {
			t.Error("missing API key")
		}
		switch r.URL.Path {
		case "/radarr/api/v3/movie":
			fmt.Fprint(w, movies)
		case "/radarr/api/v3/history":
			fmt.Fprintf(w, `{"records":%s}`, movieHistory)
		case "/sonarr/api/v3/series":
			fmt.Fprint(w, `[{"id":7,"tvdbId":70}]`)
		case "/sonarr/api/v3/episode":
			episodeCalls++
			fmt.Fprint(w, episodes)
		case "/sonarr/api/v3/history":
			fmt.Fprintf(w, `{"records":%s}`, episodeHistory)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer fixture.Close()
	path := filepath.Join(t.TempDir(), "notifications.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	watcher := &ImportWatcher{Client: fixture.Client(), Radarr: config.Service{URL: fixture.URL + "/radarr", Key: "test"}, Sonarr: config.Service{URL: fixture.URL + "/sonarr", Key: "test"}}
	poll := func(at time.Time) {
		t.Helper()
		if err := watcher.Poll(context.Background(), db, model.Snapshot{}, at); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := notificationFixture(now)
	snapshot.ImportSources = watcher.Sources()
	snapshot.Requests[0].TMDBID = 102
	snapshot.Requests = append(snapshot.Requests, model.Request{ID: 2, Type: "tv", TVDBID: 70, RequestStatus: 2, MediaStatus: 3, CreatedAt: old})
	observeTest(t, db, snapshot, nil, now)
	poll(now)
	if len(pendingNotifications(t, db)) != 0 {
		t.Fatal("existing files/history were not silently baselined")
	}
	movies = `[{"id":1,"tmdbId":101,"hasFile":true},{"id":2,"tmdbId":102,"hasFile":true},{"id":3,"tmdbId":103,"hasFile":true}]`
	episodes = `[{"id":10,"seriesId":7,"seasonNumber":1,"episodeNumber":1,"hasFile":true},{"id":11,"seriesId":7,"seasonNumber":1,"episodeNumber":2,"hasFile":true},{"id":12,"seriesId":7,"seasonNumber":2,"episodeNumber":1,"hasFile":true}]`
	movieHistory = fmt.Sprintf(`[{"id":101,"movieId":1,"eventType":"downloadFolderImported","date":%q},{"id":102,"movieId":2,"eventType":"downloadFolderImported","date":%q},{"id":103,"movieId":3,"eventType":"downloadFolderImported","date":%q}]`, fresh, fresh, fresh)
	episodeHistory = fmt.Sprintf(`[{"id":201,"seriesId":7,"episodeId":10,"eventType":"downloadFolderImported","date":%q},{"id":202,"seriesId":7,"episodeId":11,"eventType":"downloadFolderImported","date":%q},{"id":203,"seriesId":7,"episodeId":12,"eventType":"downloadFolderImported","date":%q}]`, fresh, fresh, fresh)
	poll(now.Add(2 * time.Minute))
	for i := range snapshot.Requests {
		snapshot.Requests[i].MediaStatus = 5
		snapshot.Requests[i].StatusOverride = model.StatusAvailable
	}
	observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 4 {
		t.Fatalf("expected two films and two episodes, with no upgrade or request duplicates: %v", got)
	}
	if episodeCalls != 2 {
		t.Fatalf("episode reference cache was not reused: calls=%d", episodeCalls)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	watcher = &ImportWatcher{Client: fixture.Client(), Radarr: watcher.Radarr, Sonarr: watcher.Sonarr}
	movieHistory = fmt.Sprintf(`[{"id":104,"movieId":2,"eventType":"downloadFolderImported","date":%q}]`, fresh)
	episodeHistory = fmt.Sprintf(`[{"id":204,"seriesId":7,"episodeId":11,"eventType":"downloadFolderImported","date":%q}]`, fresh)
	poll(now.Add(3 * time.Minute))
	poll(now.Add(4 * time.Minute))
	if len(pendingNotifications(t, db)) != 4 || episodeCalls != 2 {
		t.Fatal("restart or upgrades repeated notifications/lookups")
	}
	filmCount, episodeCount := 0, 0
	publisher := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch string(body) {
		case filmImportedMessage:
			filmCount++
		case episodeImportedMessage:
			episodeCount++
		default:
			t.Errorf("unexpected import message %q", string(body))
		}
		if r.Header.Get("Title") != completedTitle {
			t.Error("unexpected title")
		}
	}))
	defer publisher.Close()
	if err := Deliver(context.Background(), db, publisher.Client(), config.Notifications{URL: publisher.URL, Topic: "test", Token: "test"}); err != nil {
		t.Fatal(err)
	}
	if filmCount != 2 || episodeCount != 2 {
		t.Fatal("import messages were not preserved during delivery")
	}
}

func TestImportWatcherReusesCatalogsAndRetriesUnresolvedEvents(t *testing.T) {
	now := time.Now().UTC()
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("shared catalog caused another network call: %s", r.URL.Path)
		w.WriteHeader(500)
	}))
	defer fixture.Close()
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	watcher := &ImportWatcher{Client: fixture.Client(), Radarr: config.Service{URL: fixture.URL, Key: "test"}}
	snapshot := model.Snapshot{ImportCatalogs: map[string]model.ImportCatalog{"radarr": {Known: true, HistoryKnown: true}}}
	if err := watcher.Poll(context.Background(), db, snapshot, now); err != nil {
		t.Fatal(err)
	}
	catalog := snapshot.ImportCatalogs["radarr"]
	catalog.History = []model.ImportEvent{{ID: 1, MovieID: 5, Type: "downloadFolderImported", Date: now.Add(time.Second).Format(time.RFC3339Nano)}}
	snapshot.ImportCatalogs["radarr"] = catalog
	if err := watcher.Poll(context.Background(), db, snapshot, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(pendingNotifications(t, db)) != 0 {
		t.Fatal("unresolved identity invented a completion")
	}
	hasFile := true
	catalog.Movies = []model.ImportMovie{{ID: 5, TMDBID: 50, HasFile: &hasFile}}
	snapshot.ImportCatalogs["radarr"] = catalog
	if err := watcher.Poll(context.Background(), db, snapshot, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(pendingNotifications(t, db)) != 1 {
		t.Fatal("catalog/history race permanently lost the import")
	}
	// Old history arriving late must seed the identity silently, not notify on a later upgrade.
	catalog.Movies = append(catalog.Movies, model.ImportMovie{ID: 6, TMDBID: 60, HasFile: &hasFile})
	catalog.History = []model.ImportEvent{{ID: 2, MovieID: 6, Type: "downloadFolderImported", Date: now.Add(-time.Hour).Format(time.RFC3339Nano)}}
	snapshot.ImportCatalogs["radarr"] = catalog
	if err := watcher.Poll(context.Background(), db, snapshot, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	catalog.History = []model.ImportEvent{{ID: 3, MovieID: 6, Type: "downloadFolderImported", Date: now.Add(time.Minute).Format(time.RFC3339Nano)}}
	snapshot.ImportCatalogs["radarr"] = catalog
	if err := watcher.Poll(context.Background(), db, snapshot, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(pendingNotifications(t, db)) != 1 {
		t.Fatal("late old history or upgrade generated a backlog push")
	}
}

func TestImportObservationAndOutboxCommitTogether(t *testing.T) {
	now := time.Now().UTC()
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	watcher := &ImportWatcher{Radarr: config.Service{URL: "https://radarr.example.com", Key: "test"}}
	snapshot := model.Snapshot{ImportCatalogs: map[string]model.ImportCatalog{"radarr": {Known: true, HistoryKnown: true}}}
	if err := watcher.Poll(context.Background(), db, snapshot, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TRIGGER fail_import BEFORE INSERT ON notification_outbox BEGIN SELECT RAISE(ABORT,'test'); END"); err != nil {
		t.Fatal(err)
	}
	catalog := snapshot.ImportCatalogs["radarr"]
	catalog.Movies = []model.ImportMovie{{ID: 1, TMDBID: 10}}
	catalog.History = []model.ImportEvent{{ID: 1, MovieID: 1, Type: "downloadFolderImported", Date: now.Add(time.Second).Format(time.RFC3339Nano)}}
	snapshot.ImportCatalogs["radarr"] = catalog
	if err := watcher.Poll(context.Background(), db, snapshot, now.Add(time.Minute)); err == nil {
		t.Fatal("expected transaction failure")
	}
	if _, err := db.Exec("DROP TRIGGER fail_import"); err != nil {
		t.Fatal(err)
	}
	watcher = &ImportWatcher{Radarr: watcher.Radarr}
	if err := watcher.Poll(context.Background(), db, snapshot, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(pendingNotifications(t, db)) != 1 {
		t.Fatal("failed transaction advanced imported identity or cursor")
	}
	raw, _, err := storage.GetState(context.Background(), db, "import-baseline/radarr")
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint importCheckpoint
	if err := json.Unmarshal([]byte(raw), &checkpoint); err != nil || checkpoint.Cursor != 1 {
		t.Fatal("checkpoint was not committed")
	}
}

func TestImportMonitorContinuesWhenSeerrIsUnavailable(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var polls atomic.Int32
	delivered := make(chan struct{}, 1)
	date := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/movie":
			imported := polls.Add(1) > 1
			fmt.Fprintf(w, `[{"id":1,"tmdbId":10,"hasFile":%t}]`, imported)
		case "/api/v3/history":
			if polls.Load() > 1 {
				fmt.Fprintf(w, `{"records":[{"id":1,"movieId":1,"eventType":"downloadFolderImported","date":%q}]}`, date)
			} else {
				fmt.Fprint(w, `{"records":[]}`)
			}
		case "/notify":
			body, _ := io.ReadAll(r.Body)
			if string(body) != filmImportedMessage {
				t.Errorf("unexpected payload %q", body)
			}
			select {
			case delivered <- struct{}{}:
			default:
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer fixture.Close()
	watcher := &ImportWatcher{Client: fixture.Client(), Radarr: config.Service{URL: fixture.URL, Key: "test"}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(ctx, db, fixture.Client(), config.Notifications{URL: fixture.URL, Topic: "notify", Token: "test", Interval: time.Millisecond}, func(context.Context) (model.Snapshot, error) {
			return model.Snapshot{Integrations: map[string]string{"seerr": "error"}}, errors.New("Seerr unavailable")
		}, watcher)
	}()
	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		cancel()
		<-done
		t.Fatal("Seerr failure blocked a manual import notification")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("monitor did not stop")
	}
}

func TestImportMonitoringRejectsProgressAndFailedBaselines(t *testing.T) {
	now := time.Now().UTC()
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	watcher := &ImportWatcher{Radarr: config.Service{URL: "https://radarr.example.com", Key: "test"}}
	snapshot := model.Snapshot{ImportCatalogs: map[string]model.ImportCatalog{"radarr": {Known: false}}}
	if err := watcher.Poll(context.Background(), db, snapshot, now); err == nil {
		t.Fatal("expected unavailable catalog")
	}
	if _, exists, _ := storage.GetState(context.Background(), db, "import-baseline/radarr"); exists {
		t.Fatal("failed poll created a baseline")
	}
	hasFile := true
	catalog := model.ImportCatalog{Known: true, HistoryKnown: true, Movies: []model.ImportMovie{{ID: 1, TMDBID: 10, HasFile: &hasFile}}}
	snapshot.ImportCatalogs["radarr"] = catalog
	if err := watcher.Poll(context.Background(), db, snapshot, now.Add(91*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(pendingNotifications(t, db)) != 0 {
		t.Fatal("first good snapshot announced existing content")
	}
	catalog.Movies = append(catalog.Movies, model.ImportMovie{ID: 2, TMDBID: 20, HasFile: &hasFile})
	catalog.History = []model.ImportEvent{{ID: 1, MovieID: 2, Type: "grabbed", Date: now.Add(2 * time.Minute).Format(time.RFC3339Nano)}, {ID: 2, MovieID: 2, Type: "downloadFailed", Date: now.Add(2 * time.Minute).Format(time.RFC3339Nano)}}
	snapshot.ImportCatalogs["radarr"] = catalog
	if err := watcher.Poll(context.Background(), db, snapshot, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(pendingNotifications(t, db)) != 0 {
		t.Fatal("grab, failure or catalog presence was mistaken for an import event")
	}
}
