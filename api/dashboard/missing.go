package dashboard

import (
	"context"
	"database/sql"
	"time"

	"reqarr/model"
	"reqarr/storage"
)

const missingGracePeriod = 10 * time.Minute
const missingObservationGap = 3 * time.Minute

// Used only by the shared, serialized collector. When notifications are enabled,
// observations use the same SQLite database so restarts don't reset incidents.
type MissingTracker struct {
	DB     *sql.DB
	loaded bool
	items  map[int]storage.MissingObservation
}

func (t *MissingTracker) Apply(ctx context.Context, snapshot model.Snapshot, fetchErr error, now time.Time) (model.Snapshot, error) {
	if !t.loaded {
		t.items = make(map[int]storage.MissingObservation)
		if t.DB != nil {
			items, err := storage.LoadMissing(ctx, t.DB)
			if err != nil {
				return snapshot, err
			}
			t.items = items
		}
		t.loaded = true
	}
	next := make(map[int]storage.MissingObservation)
	// Unknown data breaks an unconfirmed streak, but doesn't resolve an already
	// escalated incident. Only explicit progress clears that incident.
	for id, observation := range t.items {
		if observation.Escalated {
			next[id] = observation
		}
	}
	requests := append([]model.Request(nil), snapshot.Requests...)
	if fetchErr == nil {
		for i := range requests {
			item := &requests[i]
			if item.Type != "movie" {
				continue
			}
			if item.AvailabilityStale {
				continue
			}
			if item.EffectiveStatus() == model.StatusAvailable || item.EffectiveStatus() == model.StatusPartiallyAvailable || (item.RadarrHasFile != nil && *item.RadarrHasFile) || (item.StatusOverride != "" && item.StatusOverride != model.StatusMissing) {
				delete(next, item.ID)
				continue
			}
			if snapshot.Integrations["radarr"] != model.IntegrationSuccess {
				continue
			}
			if item.StatusOverride != model.StatusMissing {
				continue
			}
			observation, exists := t.items[item.ID]
			if !exists || (!observation.Escalated && (now.Before(observation.LastSeen) || now.Sub(observation.LastSeen) > missingObservationGap)) {
				observation = storage.MissingObservation{Since: now}
			}
			observation.LastSeen = now
			if now.Sub(observation.Since) >= missingGracePeriod {
				observation.Escalated = true
			}
			if observation.Escalated {
				item.StatusOverride = model.StatusNeedsAttention
				item.StatusReason = model.MissingAttentionReason
			}
			next[item.ID] = observation
		}
	}
	if t.DB != nil {
		if err := storage.SaveMissing(ctx, t.DB, next); err != nil {
			return snapshot, err
		}
	}
	t.items = next
	snapshot.Requests = requests
	return snapshot, nil
}
