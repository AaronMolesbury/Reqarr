package dashboard

import (
	"testing"

	"reqarr/model"
	"reqarr/upstream"
)

func TestQueueStateAndHistory(t *testing.T) {
	cases := []struct {
		q    upstream.QueueRecord
		want string
	}{
		{upstream.QueueRecord{Status: "downloading"}, "Downloading"},
		{upstream.QueueRecord{Status: "completed"}, "Awaiting import"},
		{upstream.QueueRecord{Status: "failed"}, "Needs attention"},
		{upstream.QueueRecord{Status: "queued"}, "Queued"},
		{upstream.QueueRecord{Status: "unknown"}, ""},
	}
	for _, tc := range cases {
		if got := queueState(tc.q); got != tc.want {
			t.Errorf("%q => %q, want %q", tc.q.Status, got, tc.want)
		}
	}
	if historyType("grabbed") != "Grabbed" || historyType("downloadFolderImported") != "Imported" {
		t.Fatal("unexpected history mapping")
	}
}

func TestApplyActivityDoesNotMarkCompletedAsAvailable(t *testing.T) {
	items := []model.Request{{ID: 1, Type: "movie", MediaStatus: 3, CreatedAt: "2026-11-11T10:00:00Z"}, {ID: 2, Type: "tv", MediaStatus: 3, CreatedAt: "2026-11-11T10:00:00Z"}, {ID: 3, Type: "movie", MediaStatus: 5, CreatedAt: "2026-11-11T10:00:00Z"}}
	queue := []upstream.QueueRecord{{MovieID: 42, SeriesID: 12, Status: "completed"}}
	history := []upstream.HistoryRecord{{MovieID: 42, EventType: "grabbed", Date: "2026-11-11T11:00:00Z"}, {MovieID: 42, EventType: "downloadFolderImported", Date: "2026-11-10T11:00:00Z"}}
	applyActivity(items, map[int]int{0: 42, 2: 42}, queue, history, "radarr")
	if items[0].StatusOverride != "Awaiting import" {
		t.Fatalf("unexpected status: %+v", items[0])
	}
	if len(items[0].Activity) != 2 {
		t.Fatalf("expected queue and post-request grab only: %+v", items[0].Activity)
	}
	if items[2].StatusOverride != "" {
		t.Fatalf("available movie overridden: %+v", items[2])
	}
	applyActivity(items, map[int]int{1: 12}, queue, nil, "sonarr")
	if items[1].StatusOverride != "" {
		t.Fatalf("sonarr series queue should not override request: %+v", items[1])
	}
}
