package dashboard

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"reqarr/model"
	"reqarr/storage"
)

func episodeMissingFixture(now time.Time) model.Snapshot {
	hasFile := false
	return model.Snapshot{Integrations: map[string]string{"seerr": "success", "sonarr": "success", "radarr": "not_configured"}, Requests: []model.Request{{
		ID: 1, Type: "tv", TMDBID: 10, TVDBID: 20, RequestStatus: 2, MediaStatus: 3, CreatedAt: now.Add(-time.Hour).Format(time.RFC3339Nano), AvailabilitySource: "sonarr", StatusOverride: model.StatusPartiallyAvailable, RequestedSeasons: []int{2},
		EpisodeCheck: &model.EpisodeCheck{SeriesID: 7, Episodes: []model.EpisodeEvidence{{ID: 42, SeasonNumber: 2, EpisodeNumber: 3, HasFile: &hasFile, AirDateUTC: now.Add(-48 * time.Hour).Format(time.RFC3339Nano)}}},
	}}}
}

func applyEpisodeTest(t *testing.T, tracker *EpisodeMissingTracker, snapshot model.Snapshot, now time.Time) model.Snapshot {
	t.Helper()
	result, err := tracker.Apply(context.Background(), snapshot, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestEpisodeMissingWaitsForReleaseAndContinuousGrace(t *testing.T) {
	now := time.Now().UTC()
	tracker := &EpisodeMissingTracker{}
	snapshot := episodeMissingFixture(now)
	snapshot.Requests[0].EpisodeCheck.Episodes[0].AirDateUTC = now.Add(-24 * time.Hour).Add(time.Minute).Format(time.RFC3339Nano)
	result := applyEpisodeTest(t, tracker, snapshot, now)
	if len(tracker.items) != 0 || result.Requests[0].EffectiveStatus() != model.StatusPartiallyAvailable {
		t.Fatal("release delay must remain silent")
	}
	for minute := 1; minute <= 11; minute++ {
		result = applyEpisodeTest(t, tracker, snapshot, now.Add(time.Duration(minute)*time.Minute))
		if minute < 11 && result.Requests[0].EffectiveStatus() != model.StatusPartiallyAvailable {
			t.Fatalf("premature attention at minute %d", minute)
		}
	}
	if result.Requests[0].EffectiveStatus() != model.StatusNeedsAttention || len(result.Requests[0].MissingEpisodes) != 1 || result.Requests[0].MissingEpisodes[0].EpisodeNumber != 3 {
		t.Fatalf("missing requested episode not escalated: %+v", result.Requests[0])
	}
}

func TestEpisodeMissingRejectsUncertainOrInProgressEvidence(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name   string
		modify func(*model.Request)
	}{
		{"future", func(r *model.Request) {
			r.EpisodeCheck.Episodes[0].AirDateUTC = now.Add(time.Hour).Format(time.RFC3339Nano)
		}},
		{"unknown air date", func(r *model.Request) { r.EpisodeCheck.Episodes[0].AirDateUTC = "" }},
		{"unknown file flag", func(r *model.Request) { r.EpisodeCheck.Episodes[0].HasFile = nil }},
		{"another requested season", func(r *model.Request) { r.RequestedSeasons = []int{1} }},
		{"pending approval", func(r *model.Request) { r.RequestStatus = 1 }},
		{"downloading", func(r *model.Request) { r.EpisodeCheck.Episodes[0].QueueState = model.StatusDownloading }},
		{"import pending", func(r *model.Request) { r.EpisodeCheck.Episodes[0].QueueState = model.StatusAwaitingImport }},
		{"queued", func(r *model.Request) { r.EpisodeCheck.Episodes[0].QueueState = model.StatusQueued }},
		{"ambiguous queue", func(r *model.Request) { r.EpisodeCheck.Episodes[0].QueueState = model.StatusUnknown }},
		{"episode lookup unavailable", func(r *model.Request) { r.EpisodeCheck = nil }},
		{"availability stale", func(r *model.Request) { r.AvailabilityStale = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracker := &EpisodeMissingTracker{}
			snapshot := episodeMissingFixture(now)
			tc.modify(&snapshot.Requests[0])
			for minute := 0; minute <= 12; minute++ {
				result := applyEpisodeTest(t, tracker, snapshot, now.Add(time.Duration(minute)*time.Minute))
				if result.Requests[0].EffectiveStatus() == model.StatusNeedsAttention || len(tracker.items) != 0 {
					t.Fatal("unconfirmed absence started a missing incident")
				}
			}
		})
	}
}

func TestEpisodeMissingTimersArePerEpisodeAndBreakOnFailure(t *testing.T) {
	now := time.Now().UTC()
	tracker := &EpisodeMissingTracker{}
	for minute := 0; minute <= 10; minute++ {
		snapshot := episodeMissingFixture(now)
		if minute >= 9 {
			snapshot.Requests[0].EpisodeCheck.Episodes[0].ID = 43
			snapshot.Requests[0].EpisodeCheck.Episodes[0].EpisodeNumber = 4
		}
		result := applyEpisodeTest(t, tracker, snapshot, now.Add(time.Duration(minute)*time.Minute))
		if result.Requests[0].EffectiveStatus() == model.StatusNeedsAttention {
			t.Fatal("new episode inherited another episode's timer")
		}
	}
	for _, mode := range []string{"partial", "missing episode", "poll failure", "gap"} {
		t.Run(mode, func(t *testing.T) {
			tracker := &EpisodeMissingTracker{}
			for minute := 0; minute <= 8; minute++ {
				applyEpisodeTest(t, tracker, episodeMissingFixture(now), now.Add(time.Duration(minute)*time.Minute))
			}
			snapshot := episodeMissingFixture(now)
			if mode == "gap" {
				applyEpisodeTest(t, tracker, snapshot, now.Add(12*time.Minute))
			} else {
				var err error
				if mode == "partial" {
					snapshot.Integrations["sonarr"] = "partial"
				}
				if mode == "missing episode" {
					snapshot.Requests[0].EpisodeCheck.Episodes = nil
				}
				if mode == "poll failure" {
					err = errors.New("offline")
				}
				if _, e := tracker.Apply(context.Background(), snapshot, err, now.Add(9*time.Minute)); e != nil {
					t.Fatal(e)
				}
			}
			result := applyEpisodeTest(t, tracker, episodeMissingFixture(now), now.Add(13*time.Minute))
			if result.Requests[0].EffectiveStatus() == model.StatusNeedsAttention {
				t.Fatal("uncertain time counted toward grace")
			}
		})
	}
}

func TestEpisodeMissingPersistsAndNotifiesOncePerRequest(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "notifications.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	tracker := &EpisodeMissingTracker{DB: db}
	for minute := 0; minute <= 12; minute++ {
		result := applyEpisodeTest(t, tracker, episodeMissingFixture(now), now.Add(time.Duration(minute)*time.Minute))
		observeTest(t, db, result, nil, now.Add(time.Duration(minute)*time.Minute))
		if minute == 5 {
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = storage.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			tracker = &EpisodeMissingTracker{DB: db}
		}
	}
	defer db.Close()
	if got := pendingNotifications(t, db); len(got) != 1 || got[0] != "Reqarr: Needs attention" {
		t.Fatalf("missing episodes should share one push: %v", got)
	}
	uncertain := episodeMissingFixture(now)
	uncertain.Requests[0].EpisodeCheck = nil
	result := applyEpisodeTest(t, tracker, uncertain, now.Add(20*time.Minute))
	observeTest(t, db, result, nil, now.Add(20*time.Minute))
	if !result.Requests[0].EpisodeMissingStale || result.Requests[0].EffectiveStatus() != model.StatusNeedsAttention || len(pendingNotifications(t, db)) != 1 {
		t.Fatal("unknown data cleared or repeated an incident")
	}
	progress := episodeMissingFixture(now)
	hasFile := true
	progress.Requests[0].EpisodeCheck.Episodes[0].HasFile = &hasFile
	result = applyEpisodeTest(t, tracker, progress, now.Add(21*time.Minute))
	observeTest(t, db, result, nil, now.Add(21*time.Minute))
	if !result.Requests[0].EpisodeMissingResolved || len(result.Requests[0].MissingEpisodes) != 0 || result.Requests[0].EffectiveStatus() != model.StatusPartiallyAvailable {
		t.Fatal("import must clear the specific missing episode even while series remains partial")
	}
	for minute := 22; minute <= 32; minute++ {
		result = applyEpisodeTest(t, tracker, episodeMissingFixture(now), now.Add(time.Duration(minute)*time.Minute))
		observeTest(t, db, result, nil, now.Add(time.Duration(minute)*time.Minute))
	}
	if got := pendingNotifications(t, db); len(got) != 2 {
		t.Fatalf("a new confirmed missing incident should notify after recovery: %v", got)
	}
}

func TestAdditionalMissingEpisodesDoNotRepeatAnActiveRequestPush(t *testing.T) {
	now := time.Now().UTC()
	db, err := storage.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tracker := &EpisodeMissingTracker{DB: db}
	var result model.Snapshot
	for minute := 0; minute <= 22; minute++ {
		snapshot := episodeMissingFixture(now)
		if minute >= 11 {
			second := snapshot.Requests[0].EpisodeCheck.Episodes[0]
			second.ID, second.EpisodeNumber = 43, 4
			snapshot.Requests[0].EpisodeCheck.Episodes = append(snapshot.Requests[0].EpisodeCheck.Episodes, second)
		}
		at := now.Add(time.Duration(minute) * time.Minute)
		result = applyEpisodeTest(t, tracker, snapshot, at)
		observeTest(t, db, result, nil, at)
	}
	if len(result.Requests[0].MissingEpisodes) != 2 {
		t.Fatal("dashboard should describe both missing episodes")
	}
	if got := pendingNotifications(t, db); len(got) != 1 {
		t.Fatalf("additional missing evidence repeated an ongoing incident: %v", got)
	}
}
