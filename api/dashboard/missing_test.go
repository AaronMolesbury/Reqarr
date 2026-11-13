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

func missingFixture(now time.Time) model.Snapshot {
	snapshot := notificationFixture(now)
	snapshot.Requests[0].StatusOverride = "Missing"
	return snapshot
}

func applyMissingTest(t *testing.T, tracker *MissingTracker, snapshot model.Snapshot, fetchErr error, now time.Time) model.Snapshot {
	t.Helper()
	result, err := tracker.Apply(context.Background(), snapshot, fetchErr, now)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMissingEscalatesAfterTenObservedMinutes(t *testing.T) {
	now := time.Now().UTC()
	tracker := &MissingTracker{}
	snapshot := missingFixture(now)
	for minute := 0; minute < 10; minute++ {
		result := applyMissingTest(t, tracker, snapshot, nil, now.Add(time.Duration(minute)*time.Minute))
		if result.Requests[0].StatusOverride != "Missing" || result.Requests[0].StatusReason != "" {
			t.Fatal("missing escalated before the grace period")
		}
	}
	result := applyMissingTest(t, tracker, snapshot, nil, now.Add(10*time.Minute))
	if result.Requests[0].StatusOverride != "Needs attention" || result.Requests[0].StatusReason != model.MissingAttentionReason {
		t.Fatalf("missing did not escalate with its factual reason: %+v", result.Requests[0])
	}
	if snapshot.Requests[0].StatusOverride != "Missing" {
		t.Fatal("tracking mutated an existing upstream snapshot")
	}
}

func TestMissingUncertaintyDoesNotCountTowardGracePeriod(t *testing.T) {
	for _, uncertainty := range []string{"fetch failure", "partial radarr", "unknown fields", "observation gap"} {
		t.Run(uncertainty, func(t *testing.T) {
			now := time.Now().UTC()
			tracker := &MissingTracker{}
			for minute := 0; minute <= 4; minute++ {
				applyMissingTest(t, tracker, missingFixture(now), nil, now.Add(time.Duration(minute)*time.Minute))
			}
			snapshot := missingFixture(now)
			var err error
			switch uncertainty {
			case "fetch failure":
				err = errors.New("request list unavailable")
			case "partial radarr":
				snapshot.Integrations["radarr"] = "partial"
			case "unknown fields":
				snapshot.Requests[0].StatusOverride = ""
			case "observation gap":
				result := applyMissingTest(t, tracker, snapshot, nil, now.Add(9*time.Minute))
				if result.Requests[0].StatusOverride != "Missing" || !tracker.items[1].Since.Equal(now.Add(9*time.Minute)) {
					t.Fatal("unobserved time counted as confirmed missing")
				}
				return
			}
			applyMissingTest(t, tracker, snapshot, err, now.Add(5*time.Minute))
			for minute := 6; minute <= 15; minute++ {
				result := applyMissingTest(t, tracker, missingFixture(now), nil, now.Add(time.Duration(minute)*time.Minute))
				if result.Requests[0].StatusOverride != "Missing" {
					t.Fatal("an upstream outage/unknown state contributed missing time")
				}
			}
			result := applyMissingTest(t, tracker, missingFixture(now), nil, now.Add(16*time.Minute))
			if result.Requests[0].StatusOverride != "Needs attention" {
				t.Fatal("a new confirmed missing streak should escalate")
			}
		})
	}
}

func TestMissingProgressAndTVDoNotEscalate(t *testing.T) {
	for _, stage := range []string{"Downloading", "Queued", "Awaiting import", "Awaiting release", "Needs attention", "Available", "Has file"} {
		t.Run(stage, func(t *testing.T) {
			now := time.Now().UTC()
			tracker := &MissingTracker{}
			applyMissingTest(t, tracker, missingFixture(now), nil, now)
			snapshot := missingFixture(now)
			snapshot.Requests[0].StatusOverride = stage
			if stage == "Available" {
				snapshot.Requests[0].StatusOverride = ""
				snapshot.Requests[0].MediaStatus = 5
				snapshot.Integrations["radarr"] = "error"
			}
			if stage == "Has file" {
				snapshot.Requests[0].StatusOverride = ""
				hasFile := true
				snapshot.Requests[0].RadarrHasFile = &hasFile
			}
			result := applyMissingTest(t, tracker, snapshot, nil, now.Add(time.Minute))
			if result.Requests[0].StatusOverride != snapshot.Requests[0].StatusOverride || len(tracker.items) != 0 {
				t.Fatal("positive progress must clear the missing streak and preserve the upstream status")
			}
			result = applyMissingTest(t, tracker, missingFixture(now), nil, now.Add(2*time.Minute))
			if result.Requests[0].StatusOverride != "Missing" {
				t.Fatal("a later missing incident must get its own grace period")
			}
		})
	}
	tracker := &MissingTracker{}
	snapshot := missingFixture(time.Now())
	snapshot.Requests[0].Type = "tv"
	applyMissingTest(t, tracker, snapshot, nil, time.Now())
	if len(tracker.items) != 0 {
		t.Fatal("movie missing policy must not classify requested TV seasons")
	}
}

func TestMissingStatePersistsAndAttentionNotifiesOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	tracker := &MissingTracker{DB: db}
	for minute := 0; minute <= 10; minute++ {
		at := now.Add(time.Duration(minute) * time.Minute)
		result := applyMissingTest(t, tracker, missingFixture(now), nil, at)
		observeTest(t, db, result, nil, at)
		if minute == 5 {
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = storage.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			tracker = &MissingTracker{DB: db}
		}
	}
	defer db.Close()
	if got := pendingNotifications(t, db); len(got) != 1 || got[0] != "Reqarr: missing for 10 minutes" {
		t.Fatalf("expected one attention notification after the silent baseline: %v", got)
	}
	tracker = &MissingTracker{DB: db}
	// Unknown time cannot clear an already raised incident or resend it.
	applyMissingTest(t, tracker, model.Snapshot{}, errors.New("offline"), now.Add(11*time.Minute))
	result := applyMissingTest(t, tracker, missingFixture(now), nil, now.Add(time.Hour))
	observeTest(t, db, result, nil, now.Add(time.Hour))
	if result.Requests[0].StatusOverride != "Needs attention" || len(pendingNotifications(t, db)) != 1 {
		t.Fatal("outages/restarts must not resolve or re-notify the same missing incident")
	}
}
