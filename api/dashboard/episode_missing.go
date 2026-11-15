package dashboard

import (
	"context"
	"database/sql"
	"slices"
	"sort"
	"time"

	"reqarr/model"
	"reqarr/storage"
)

const episodeReleaseDelay = 24 * time.Hour

// Timers are per episode so a different missing episode cannot inherit elapsed time.
type EpisodeMissingTracker struct {
	DB     *sql.DB
	loaded bool
	items  map[storage.EpisodeKey]storage.EpisodeObservation
}

func (t *EpisodeMissingTracker) Apply(ctx context.Context, snapshot model.Snapshot, fetchErr error, now time.Time) (model.Snapshot, error) {
	if !t.loaded {
		t.items = make(map[storage.EpisodeKey]storage.EpisodeObservation)
		if t.DB != nil {
			items, err := storage.LoadEpisodeMissing(ctx, t.DB)
			if err != nil {
				return snapshot, err
			}
			t.items = items
		}
		t.loaded = true
	}
	next := make(map[storage.EpisodeKey]storage.EpisodeObservation)
	for key, item := range t.items {
		if item.Escalated {
			next[key] = item
		}
	}
	requests := append([]model.Request(nil), snapshot.Requests...)
	if fetchErr == nil {
		present := make(map[int]bool)
		for i := range requests {
			item := &requests[i]
			if item.Type != "tv" {
				continue
			}
			item.MissingEpisodes = nil
			item.EpisodeMissingStale = false
			item.EpisodeMissingResolved = false
			present[item.ID] = true
			hadIncident := false
			for key, observation := range t.items {
				if key.RequestID == item.ID && observation.Escalated {
					hadIncident = true
				}
			}
			for key, observation := range next {
				if key.RequestID != item.ID {
					continue
				}
				if (len(item.RequestedSeasons) > 0 && !slices.Contains(item.RequestedSeasons, observation.SeasonNumber)) || (item.EpisodeCheck != nil && key.SeriesID != item.EpisodeCheck.SeriesID) {
					delete(next, key)
				}
			}
			confirmed := make(map[storage.EpisodeKey]bool)
			complete := item.AvailabilitySource == "sonarr" && !item.AvailabilityStale && item.EffectiveStatus() == model.StatusAvailable
			cancelled := item.RequestStatus == 1 || item.RequestStatus == 3 || item.RequestStatus == 4
			if complete || cancelled {
				for key := range next {
					if key.RequestID == item.ID {
						delete(next, key)
					}
				}
			} else if item.EpisodeCheck != nil && !item.AvailabilityStale && snapshot.Integrations["sonarr"] == model.IntegrationSuccess && (item.RequestStatus == 2 || item.RequestStatus == 5) {
				for _, episode := range item.EpisodeCheck.Episodes {
					key := storage.EpisodeKey{RequestID: item.ID, SeriesID: item.EpisodeCheck.SeriesID, EpisodeID: episode.ID}
					if !slices.Contains(item.RequestedSeasons, episode.SeasonNumber) || episode.HasFile == nil {
						continue
					}
					if *episode.HasFile || episode.QueueState == model.StatusDownloading || episode.QueueState == model.StatusAwaitingImport || episode.QueueState == model.StatusQueued {
						delete(next, key)
						continue
					}
					airDate, err := time.Parse(time.RFC3339Nano, episode.AirDateUTC)
					if err != nil {
						continue
					}
					if now.Sub(airDate) < episodeReleaseDelay {
						delete(next, key)
						continue
					}
					if episode.QueueState == model.StatusUnknown {
						continue
					}
					observation, exists := t.items[key]
					if !exists || observation.SeasonNumber != episode.SeasonNumber || observation.EpisodeNumber != episode.EpisodeNumber || (!observation.Escalated && (now.Before(observation.LastSeen) || now.Sub(observation.LastSeen) > missingObservationGap)) {
						observation = storage.EpisodeObservation{MissingObservation: storage.MissingObservation{Since: now}, SeasonNumber: episode.SeasonNumber, EpisodeNumber: episode.EpisodeNumber}
					}
					observation.LastSeen = now
					if now.Sub(observation.Since) >= missingGracePeriod {
						observation.Escalated = true
					}
					next[key] = observation
					confirmed[key] = true
				}
			}
			for key, observation := range next {
				if key.RequestID != item.ID || !observation.Escalated {
					continue
				}
				item.MissingEpisodes = append(item.MissingEpisodes, model.MissingEpisode{SeasonNumber: observation.SeasonNumber, EpisodeNumber: observation.EpisodeNumber})
				if !confirmed[key] {
					item.EpisodeMissingStale = true
				}
			}
			if len(item.MissingEpisodes) > 0 {
				sort.Slice(item.MissingEpisodes, func(i, j int) bool {
					a, b := item.MissingEpisodes[i], item.MissingEpisodes[j]
					return a.SeasonNumber < b.SeasonNumber || (a.SeasonNumber == b.SeasonNumber && a.EpisodeNumber < b.EpisodeNumber)
				})
				item.StatusOverride = model.StatusNeedsAttention
				item.StatusReason = model.EpisodeMissingAttentionReason
				if item.EpisodeMissingStale {
					item.StatusReason += " Current episode or queue data is unconfirmed; the incident remains unresolved."
				}
			} else if hadIncident {
				item.EpisodeMissingResolved = true
			}
		}
		for key := range next {
			if !present[key.RequestID] {
				delete(next, key)
			}
		}
	}
	if t.DB != nil {
		if err := storage.SaveEpisodeMissing(ctx, t.DB, next); err != nil {
			return snapshot, err
		}
	}
	t.items = next
	snapshot.Requests = requests
	return snapshot, nil
}
