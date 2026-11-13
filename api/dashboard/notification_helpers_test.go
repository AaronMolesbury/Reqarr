package dashboard

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"reqarr/model"
	"reqarr/notifications"
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
	if err := notifications.Observe(context.Background(), db, snapshot, err, now); err != nil {
		t.Fatal(err)
	}
}
