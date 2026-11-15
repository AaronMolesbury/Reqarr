package notifications

import (
	"path/filepath"
	"testing"
	"time"

	"reqarr/model"
	"reqarr/storage"
)

func TestIncidentRecoveryIsScopedToItsOwnMedia(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	snapshot := notificationFixture(now)
	second := snapshot.Requests[0]
	second.ID, second.TMDBID = 2, 20
	snapshot.Requests = append(snapshot.Requests, second)
	observeTest(t, db, snapshot, nil, now)
	for i := range snapshot.Requests {
		snapshot.Requests[i].StatusOverride = model.StatusNeedsAttention
	}
	observeTest(t, db, snapshot, nil, now.Add(time.Minute))
	if got := pendingNotifications(t, db); len(got) != 2 {
		t.Fatalf("independent problems must each notify: %v", got)
	}
	snapshot.Requests[1].StatusOverride = model.StatusDownloading
	observeTest(t, db, snapshot, nil, now.Add(2*time.Minute))
	snapshot.Requests[0].Activity = []model.Activity{{Source: "radarr", Type: "Download failed", Date: now.Add(3 * time.Minute).Format(time.RFC3339Nano)}}
	snapshot.Requests[1].StatusOverride = model.StatusNeedsAttention
	observeTest(t, db, snapshot, nil, now.Add(4*time.Minute))
	if got := pendingNotifications(t, db); len(got) != 3 {
		t.Fatalf("recovery must rearm only the second problem, not repeat the first: %v", got)
	}
}
