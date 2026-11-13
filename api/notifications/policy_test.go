package notifications

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"reqarr/config"
	"reqarr/model"
	"reqarr/storage"
)

func TestPushLifecycleKeepsProgressOnDashboard(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	observeTest(t, db, snapshot, nil, now)
	item := model.Request{ID: 2, Type: "movie", TMDBID: 20, RequestStatus: 1, MediaStatus: 2, CreatedAt: now.Add(time.Second).Format(time.RFC3339Nano)}
	snapshot.Requests = append(snapshot.Requests, item)
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	for _, stage := range []string{model.StatusProcessing, model.StatusAwaitingRelease, model.StatusMissing, model.StatusQueued, model.StatusDownloading, model.StatusAwaitingImport} {
		snapshot.Requests[1].RequestStatus = 2
		snapshot.Requests[1].MediaStatus = 3
		snapshot.Requests[1].StatusOverride = stage
		observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	}
	// Seerr request completion alone and import history do not prove availability.
	snapshot.Requests[1].RequestStatus = 5
	snapshot.Requests[1].Activity = []model.Activity{{Source: "radarr", Type: "Imported", Date: now.Add(time.Minute).Format(time.RFC3339Nano)}}
	observeTest(t, db, snapshot, nil, now.Add(3*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 1 || got[0] != newRequestTitle {
		t.Fatalf("progress generated pushes: %v", got)
	}
	snapshot.Requests[1].MediaStatus = 5
	snapshot.Requests[1].StatusOverride = ""
	observeTest(t, db, snapshot, nil, now.Add(4*time.Minute))
	observeTest(t, db, snapshot, nil, now.Add(5*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 2 || got[1] != completedTitle {
		t.Fatalf("expected one completion: %v", got)
	}
}

func TestAttentionCoalescesHistoryAndSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	observeTest(t, db, snapshot, nil, now)
	snapshot.Requests[0].StatusOverride = model.StatusNeedsAttention
	snapshot.Requests[0].Activity = []model.Activity{{Source: "radarr", QueueID: 42, QueueState: model.StatusNeedsAttention}, {Source: "radarr", Type: "Download failed", Date: now.Add(time.Minute).Format(time.RFC3339Nano)}}
	observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 1 {
		t.Fatalf("duplicate incident evidence: %v", got)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	snapshot.Requests[0].Activity = append(snapshot.Requests[0].Activity, model.Activity{Source: "radarr", Type: "Download failed", Date: now.Add(3 * time.Minute).Format(time.RFC3339Nano)})
	observeTest(t, db, snapshot, nil, now.Add(4*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 1 {
		t.Fatalf("restart or delayed history repeated incident: %v", got)
	}
	snapshot.Requests[0].StatusOverride = model.StatusDownloading
	snapshot.Requests[0].Activity[0].QueueState = model.StatusDownloading
	observeTest(t, db, snapshot, nil, now.Add(5*time.Minute))
	snapshot.Requests[0].StatusOverride = model.StatusNeedsAttention
	snapshot.Requests[0].Activity[0].QueueState = model.StatusNeedsAttention
	observeTest(t, db, snapshot, nil, now.Add(6*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 2 {
		t.Fatalf("old history prevented a new incident after recovery: %v", got)
	}
}

func TestLegacyOutboxUsesReducedPushPolicy(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	tx, err := storage.BeginState(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Reqarr: request approved", "Reqarr: processing", "Reqarr: imported", "Reqarr: integration recovered", "Reqarr: available"} {
		if err := tx.Queue(ctx, title, "Old verbose message", 3); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Title") != completedTitle || string(body) != "Request fulfilled." {
			t.Error("legacy payload was not normalized")
		}
	}))
	defer server.Close()
	if err := Deliver(ctx, db, server.Client(), config.Notifications{URL: server.URL, Topic: "test", Token: "test"}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(pendingNotifications(t, db)) != 0 {
		t.Fatal("obsolete queued pushes were delivered or retained")
	}
}

func TestCompletionUsesImportedAvailabilityAndIgnoresSeerrCatchup(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	snapshot.Requests[0].AvailabilitySource = "radarr"
	snapshot.Requests[0].StatusOverride = model.StatusProcessing
	observeTest(t, db, snapshot, nil, now)
	snapshot.Requests[0].StatusOverride = model.StatusAvailable
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	snapshot.Requests[0].MediaStatus = 5
	observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	snapshot.Requests[0].AvailabilityStale = true
	observeTest(t, db, snapshot, nil, now.Add(3*time.Minute))
	snapshot.Requests[0].AvailabilityStale = false
	observeTest(t, db, snapshot, nil, now.Add(4*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 1 || got[0] != completedTitle {
		t.Fatalf("completion duplicated or missed: %v", got)
	}
	// A stale Seerr Available must not fulfil a different request without a file.
	item := snapshot.Requests[0]
	item.ID = 2
	item.TMDBID = 20
	item.CreatedAt = now.Add(5 * time.Minute).Format(time.RFC3339Nano)
	item.StatusOverride = model.StatusMissing
	snapshot.Requests = append(snapshot.Requests, item)
	observeTest(t, db, snapshot, nil, now.Add(6*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 2 || got[1] != newRequestTitle {
		t.Fatalf("Seerr overrode missing file evidence: %v", got)
	}
}
