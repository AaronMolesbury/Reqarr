package notifications

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reqarr/config"
	"reqarr/model"
	"reqarr/storage"
)

func notificationFixture(now time.Time) model.Snapshot {
	return model.Snapshot{
		Integrations: map[string]string{"seerr": "success", "radarr": "success", "sonarr": "success"},
		Requests:     []model.Request{{ID: 1, Type: "movie", TMDBID: 10, RequestStatus: 2, MediaStatus: 3, CreatedAt: now.Add(-time.Hour).Format(time.RFC3339Nano)}},
	}
}

func pendingNotifications(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query("SELECT title FROM notification_outbox WHERE sent=0 ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var titles []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			t.Fatal(err)
		}
		titles = append(titles, title)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return titles
}

func observeTest(t *testing.T, db *sql.DB, snapshot model.Snapshot, err error, now time.Time) {
	t.Helper()
	if err := Observe(context.Background(), db, snapshot, err, now); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationsBaselineAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	snapshot.Requests[0].StatusOverride = "Needs attention"
	snapshot.Requests[0].Activity = []model.Activity{{Source: "radarr", Type: "Download failed", Date: now.Add(-time.Minute).Format(time.RFC3339Nano)}}
	observeTest(t, db, snapshot, nil, now)
	if got := pendingNotifications(t, db); len(got) != 0 {
		t.Fatalf("startup must be silent: %v", got)
	}
	db.Close()
	db, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	if got := pendingNotifications(t, db); len(got) != 0 {
		t.Fatalf("restart replayed existing incidents: %v", got)
	}
	snapshot.Requests[0].MediaStatus = 5
	snapshot.Requests[0].StatusOverride = ""
	observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	observeTest(t, db, snapshot, nil, now.Add(3*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 1 || got[0] != "Reqarr: available" {
		t.Fatalf("expected one availability event: %v", got)
	}
}

func TestNotificationsNewRequestsAndKnownStatusChanges(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	observeTest(t, db, snapshot, nil, now)
	snapshot.Requests = append(snapshot.Requests,
		model.Request{ID: 2, Type: "movie", CreatedAt: now.Add(time.Second).Format(time.RFC3339Nano), RequestStatus: 1, MediaStatus: 2},
		model.Request{ID: 3, Type: "tv", CreatedAt: now.Add(-time.Hour).Format(time.RFC3339Nano), RequestStatus: 2, MediaStatus: 5})
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	got := pendingNotifications(t, db)
	if len(got) != 3 || got[0] != "Reqarr: new request" || got[1] != "Reqarr: request pending" || got[2] != "Reqarr: media pending" {
		t.Fatalf("new requests eligible; newly discovered old requests silent: %v", got)
	}
	snapshot.Requests[1].RequestStatus = 3
	observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	got = pendingNotifications(t, db)
	if got[len(got)-1] != "Reqarr: request declined" {
		t.Fatalf("missing decline: %v", got)
	}
}

func TestNotificationIncidentsAndUpstreamFailures(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	observeTest(t, db, snapshot, nil, now)
	snapshot.Requests[0].StatusOverride = "Needs attention"
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 1 {
		t.Fatalf("incident repeated: %v", got)
	}
	snapshot.Integrations["radarr"] = "error"
	snapshot.Requests[0].StatusOverride = ""
	observeTest(t, db, snapshot, nil, now.Add(3*time.Minute))
	observeTest(t, db, snapshot, nil, now.Add(4*time.Minute))
	snapshot.Integrations["radarr"] = "success"
	snapshot.Requests[0].StatusOverride = "Needs attention"
	observeTest(t, db, snapshot, nil, now.Add(5*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 3 {
		t.Fatalf("failure/recovery must not reset a request incident: %v", got)
	}
	snapshot.Requests[0].StatusOverride = "Downloading"
	observeTest(t, db, snapshot, nil, now.Add(6*time.Minute))
	snapshot.Requests[0].StatusOverride = "Needs attention"
	observeTest(t, db, snapshot, nil, now.Add(7*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 5 {
		t.Fatalf("confirmed progress allows a later incident: %v", got)
	}
}

func TestFailedInitialPollAndOldHistory(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	observeTest(t, db, model.Snapshot{Integrations: map[string]string{"seerr": "error"}}, errors.New("upstream failed"), now)
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	if got := pendingNotifications(t, db); len(got) != 0 {
		t.Fatalf("failed poll established a baseline: %v", got)
	}
	snapshot.Requests[0].Activity = []model.Activity{
		{Source: "radarr", Type: "Imported", Date: now.Format(time.RFC3339Nano)},
		{Source: "radarr", Type: "Grabbed", Date: now.Add(2 * time.Minute).Format(time.RFC3339Nano)},
		{Source: "radarr", Type: "Imported", Date: now.Add(3 * time.Minute).Format(time.RFC3339Nano)},
	}
	observeTest(t, db, snapshot, nil, now.Add(4*time.Minute))
	observeTest(t, db, snapshot, nil, now.Add(5*time.Minute))
	got := pendingNotifications(t, db)
	if len(got) != 2 || got[0] != "Reqarr: grabbed" || got[1] != "Reqarr: imported" {
		t.Fatalf("history replay or incorrect import availability: %v", got)
	}
}

func TestNotificationDeliveryRetryPrivacyAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	observeTest(t, db, snapshot, nil, now)
	snapshot.Requests[0].MediaStatus = 5
	snapshot.Requests[0].Title = "private-title"
	snapshot.Requests[0].Requester = "private-person"
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	var failed atomic.Bool
	failed.Store(true)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "private") || r.Header.Get("Authorization") != "Bearer test-token" || r.URL.Path != "/test-topic" || r.Method != http.MethodPost {
			t.Error("invalid or private notification payload")
		}
		if failed.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	c := config.Notifications{URL: server.URL, Topic: "test-topic", Token: "test-token"}
	if err := Deliver(context.Background(), db, server.Client(), c); err == nil {
		t.Fatal("expected delivery error")
	}
	if len(pendingNotifications(t, db)) != 1 {
		t.Fatal("failed delivery lost notification")
	}
	db.Close()
	db, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	failed.Store(false)
	for range 2 {
		if err := Deliver(context.Background(), db, server.Client(), c); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 || len(pendingNotifications(t, db)) != 0 {
		t.Fatal("successful delivery repeated or not marked sent")
	}
}

func TestNotificationStateAndOutboxAreAtomic(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	observeTest(t, db, snapshot, nil, now)
	if _, err := db.Exec("CREATE TRIGGER fail_outbox BEFORE INSERT ON notification_outbox BEGIN SELECT RAISE(ABORT,'test'); END"); err != nil {
		t.Fatal(err)
	}
	snapshot.Requests[0].MediaStatus = 5
	if err := Observe(context.Background(), db, snapshot, nil, now.Add(time.Minute)); err == nil {
		t.Fatal("expected transaction failure")
	}
	if _, err := db.Exec("DROP TRIGGER fail_outbox"); err != nil {
		t.Fatal(err)
	}
	observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 1 || got[0] != "Reqarr: available" {
		t.Fatalf("state advanced without durable notification: %v", got)
	}
}

func TestNotificationMonitorRunsWithoutBrowserRequests(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	delivered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "tester" || password != "test-password" {
			t.Error("missing ntfy basic authentication")
		}
		select {
		case delivered <- struct{}{}:
		default:
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	c := config.Notifications{URL: server.URL, Topic: "reqarr", Username: "tester", Password: "test-password", Interval: time.Millisecond}
	go func() {
		defer close(done)
		polls := 0
		Run(ctx, db, server.Client(), c, func(context.Context) (model.Snapshot, error) {
			polls++
			snapshot := notificationFixture(time.Now())
			if polls > 1 {
				snapshot.Requests[0].MediaStatus = 5
			}
			return snapshot, nil
		})
	}()
	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		cancel()
		<-done
		t.Fatal("background worker did not deliver notification")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("monitor did not stop on cancellation")
	}
}

func TestSonarrNotificationsAreSeriesActivityAndDeduplicateSharedRequests(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	series := model.Request{ID: 2, Type: "tv", TVDBID: 123, RequestStatus: 2, MediaStatus: 3, CreatedAt: now.Add(-time.Hour).Format(time.RFC3339Nano)}
	series.Activity = []model.Activity{{Source: "sonarr", QueueID: 42, QueueState: "Downloading"}}
	snapshot.Requests = []model.Request{series}
	observeTest(t, db, snapshot, nil, now)
	series.Activity[0].QueueState = "Needs attention"
	series.Activity = append(series.Activity, model.Activity{Source: "sonarr", Type: "Download failed", Date: now.Add(time.Minute).Format(time.RFC3339Nano)})
	snapshot.Requests = []model.Request{series, series}
	snapshot.Requests[1].ID = 3
	observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	observeTest(t, db, snapshot, nil, now.Add(3*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 2 || got[0] != "Reqarr: series needs attention" || got[1] != "Reqarr: download failed" {
		t.Fatalf("series activity must be qualified and shared events deduplicated: %v", got)
	}
	var message string
	if err := db.QueryRow("SELECT message FROM notification_outbox ORDER BY id LIMIT 1").Scan(&message); err != nil || !strings.Contains(message, "outside the requested season") {
		t.Fatal("series activity was presented as requested-season evidence")
	}
}

func TestNotificationPublisherRejectsRedirects(t *testing.T) {
	var forwarded atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			forwarded.Store(true)
			return
		}
		http.Redirect(w, r, "/redirected", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	err := Publish(context.Background(), server.Client(), config.Notifications{URL: server.URL, Topic: "reqarr", Token: "secret"}, "Reqarr", "Generic message", 3)
	if err == nil || forwarded.Load() {
		t.Fatal("publisher followed a redirect")
	}
}

func TestMovieQueueIncidentsDoNotRepeatWhenOtherItemsDisappear(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	snapshot.Requests[0].StatusOverride = "Needs attention"
	snapshot.Requests[0].Activity = []model.Activity{{Source: "radarr", QueueID: 42, QueueState: "Needs attention"}}
	observeTest(t, db, snapshot, nil, now)
	snapshot.Requests[0].Activity = append(snapshot.Requests[0].Activity, model.Activity{Source: "radarr", QueueID: 43, QueueState: "Needs attention"})
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	snapshot.Requests[0].Activity = snapshot.Requests[0].Activity[1:]
	observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 1 || got[0] != "Reqarr: needs attention" {
		t.Fatalf("only the new queue incident should alert: %v", got)
	}
	// Missing queue entries are not proof of recovery; their return is silent.
	snapshot.Requests[0].StatusOverride = ""
	snapshot.Requests[0].Activity = nil
	observeTest(t, db, snapshot, nil, now.Add(3*time.Minute))
	snapshot.Requests[0].StatusOverride = "Needs attention"
	snapshot.Requests[0].Activity = []model.Activity{{Source: "radarr", QueueID: 43, QueueState: "Needs attention"}}
	observeTest(t, db, snapshot, nil, now.Add(4*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 1 {
		t.Fatalf("a missing queue entry reset the incident: %v", got)
	}
	// A positive state change allows a later recurrence for the same queue ID.
	snapshot.Requests[0].StatusOverride = "Downloading"
	snapshot.Requests[0].Activity[0].QueueState = "Downloading"
	observeTest(t, db, snapshot, nil, now.Add(5*time.Minute))
	snapshot.Requests[0].StatusOverride = "Needs attention"
	snapshot.Requests[0].Activity[0].QueueState = "Needs attention"
	observeTest(t, db, snapshot, nil, now.Add(6*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 3 || got[2] != "Reqarr: needs attention" {
		t.Fatalf("confirmed progress should allow a later recurrence: %v", got)
	}
}

func TestNotificationDiagnosticLogsExcludePrivateData(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	snapshot.Requests[0].Title = "private-title"
	snapshot.Requests[0].Requester = "private-requester"
	observeTest(t, db, snapshot, nil, now)
	snapshot.Requests[0].MediaStatus = 5
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	var reject atomic.Bool
	reject.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reject.Load() {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer server.Close()
	c := config.Notifications{URL: server.URL, Topic: "private-topic", Token: "private-token"}
	if err := Deliver(context.Background(), db, server.Client(), c); err == nil {
		t.Fatal("expected rejected delivery")
	}
	reject.Store(false)
	if err := Deliver(context.Background(), db, server.Client(), c); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Notification baseline recorded", "Notifications queued: count=1", "ntfy returned HTTP 401", "Notification delivered"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("missing diagnostic: %s", want)
		}
	}
	for _, secret := range []string{"private-title", "private-requester", "private-topic", "private-token", server.URL} {
		if strings.Contains(logs.String(), secret) {
			t.Error("private data appeared in diagnostic logs")
		}
	}
}

func TestMissingClassificationDoesNotReplayExistingRequestsOrInventFailure(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	observeTest(t, db, snapshot, nil, now)
	snapshot.Requests[0].StatusOverride = "Missing"
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	if got := pendingNotifications(t, db); len(got) != 0 {
		t.Fatalf("newly precise label replayed an existing processing request: %v", got)
	}
	item := snapshot.Requests[0]
	item.ID = 2
	item.CreatedAt = now.Add(2 * time.Minute).Format(time.RFC3339Nano)
	snapshot.Requests = append(snapshot.Requests, item)
	observeTest(t, db, snapshot, nil, now.Add(3*time.Minute))
	observeTest(t, db, snapshot, nil, now.Add(4*time.Minute))
	var count, priority int
	var message string
	if err := db.QueryRow("SELECT count(*),min(priority),min(message) FROM notification_outbox WHERE title='Reqarr: missing'").Scan(&count, &priority, &message); err != nil {
		t.Fatal(err)
	}
	if count != 1 || priority != 3 || !strings.Contains(message, "does not mean a search or download failed") {
		t.Fatalf("missing must be deduplicated, informational and explicit about its limits: count=%d priority=%d message=%q", count, priority, message)
	}
}

func TestUnchangedNotificationPollsAreQuiet(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	observeTest(t, db, snapshot, nil, now)
	logs.Reset()
	for poll := 1; poll <= 4; poll++ {
		observeTest(t, db, snapshot, nil, now.Add(time.Duration(poll)*15*time.Second))
	}
	if logs.Len() != 0 {
		t.Fatalf("unchanged successful polls should be quiet: %s", logs.String())
	}
}
