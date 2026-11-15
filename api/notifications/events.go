package notifications

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"reqarr/model"
	"reqarr/storage"
)

// observation and outbox updates commit together: a failed delivery can be
// retried without creating another event on the next successful poll.
func Observe(ctx context.Context, db *sql.DB, snapshot model.Snapshot, fetchErr error, now time.Time) error {
	tx, err := storage.BeginState(ctx, db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	get := func(key string) (string, bool, error) { return tx.Get(ctx, key) }
	cutoff, initialized, err := get("baseline")
	if err != nil {
		return err
	}
	if !initialized && fetchErr != nil {
		return nil // An unsuccessful request list must never establish a baseline.
	}
	if !initialized {
		cutoff = now.UTC().Format(time.RFC3339Nano)
	}
	state := &observationState{ctx: ctx, tx: tx, initialized: initialized}
	change := state.change
	integrations := incidentScope{Key: "integrations"}
	queueBaselined := make(map[string]bool)
	for _, source := range []string{"radarr", "sonarr"} {
		_, queueBaselined[source], err = get("queue-baseline/" + source)
		if err != nil {
			return err
		}
	}
	allHealthy := true
	for _, source := range []string{"seerr", "radarr", "sonarr"} {
		status := snapshot.Integrations[source]
		if status != model.IntegrationSuccess && status != model.IntegrationNotConfigured {
			allHealthy = false
		}
	}
	if allHealthy {
		if err := tx.Set(ctx, "attention/integrations", "clear"); err != nil {
			return err
		}
	}
	for _, source := range []string{"seerr", "radarr", "sonarr"} {
		status := snapshot.Integrations[source]
		if status != model.IntegrationSuccess && status != model.IntegrationPartial && status != model.IntegrationError {
			continue
		}
		title := "Reqarr: integration needs attention"
		value := "unhealthy"
		if status == model.IntegrationSuccess {
			value = model.IntegrationSuccess
			title = ""
		}
		if err := change(integrations, "integration/"+source, value, title, status != model.IntegrationSuccess); err != nil {
			return err
		}
	}
	if fetchErr == nil {
		problems := make(map[string]bool)
		for _, item := range snapshot.Requests {
			if currentProblem(item) {
				problems[attentionScope(item)] = true
			}
		}
		for _, item := range snapshot.Requests {
			scope := incidentScope{Key: attentionScope(item), CanRecover: !problems[attentionScope(item)]}
			prefix := fmt.Sprintf("request/%d/", item.ID)
			_, known, err := get(prefix + "seen")
			if err != nil {
				return err
			}
			isNew := !known && model.AfterRequest(item.CreatedAt, cutoff)
			previousMedia, mediaKnown, err := get(prefix + "media")
			if err != nil {
				return err
			}
			if err := change(scope, prefix+"seen", "1", newRequestTitle, isNew); err != nil {
				return err
			}
			// Existing requests remain silent initially; subsequent observed changes
			// are eligible. Newly discovered old requests are also baselined.
			notifyFirst := isNew
			mediaTitle := ""
			if item.AvailabilitySource == "" || item.AvailabilitySource == "seerr" {
				mediaTitle = mediaEventTitle(item.MediaStatus)
			}
			for _, field := range []struct {
				key, value, title string
			}{
				{"approval", strconv.Itoa(item.RequestStatus), requestEventTitle(item.RequestStatus)},
				{"media", strconv.Itoa(item.MediaStatus), mediaTitle},
			} {
				if err := change(scope, prefix+field.key, field.value, field.title, notifyFirst); err != nil {
					return err
				}
			}
			if !item.AvailabilityStale {
				completion := "not_available"
				if item.EffectiveStatus() == model.StatusAvailable {
					completion = "available"
				}
				_, completionKnown, err := get(prefix + "completion")
				if err != nil {
					return err
				}
				// Reuse earlier completion evidence without replaying old fulfilled requests.
				if !completionKnown && known && mediaKnown {
					baseline := "not_available"
					if previousMedia == "5" {
						baseline = "available"
					}
					if err := tx.Set(ctx, prefix+"completion", baseline); err != nil {
						return err
					}
				}
				title := ""
				if completion == "available" {
					title = completedTitle
				}
				if err := change(scope, prefix+"completion", completion, title, isNew); err != nil {
					return err
				}
			}
			source := "radarr"
			if item.Type == "tv" {
				source = "sonarr"
			}
			// Missing/failed enrichment must not reset an incident or manufacture
			// a recovery. Only positively reported stages update the saved state.
			if item.Type == "tv" && !item.EpisodeMissingStale {
				if item.EpisodeMissingResolved {
					if err := change(scope, prefix+"episode-attention", "clear", "", false); err != nil {
						return err
					}
				} else if len(item.MissingEpisodes) > 0 {
					if err := change(scope, prefix+"episode-attention", "active", "", false); err != nil {
						return err
					}
				}
			}
			if !item.AvailabilityStale && !item.EpisodeMissingStale && snapshot.Integrations[source] == model.IntegrationSuccess && item.StatusOverride != "" {
				stage := item.StatusOverride
				title := ""
				if stage == model.StatusNeedsAttention {
					title = attentionTitle
					if item.StatusReason == model.MissingAttentionReason {
						stage = "Needs attention/missing"
					}
					if item.StatusReason == model.EpisodeMissingAttentionReason {
						stage = "Needs attention/episodes"
					}
					for _, event := range item.Activity {
						if event.QueueID > 0 && event.QueueState == model.StatusNeedsAttention {
							title = "" // Queue evidence identifies the same incident below.
						}
					}
				}
				if err := change(scope, prefix+"stage", stage, title, isNew); err != nil {
					return err
				}
			} else if snapshot.Integrations[source] == model.IntegrationSuccess {
				if _, exists, err := get(prefix + "stage"); err != nil {
					return err
				} else if !exists {
					if err := change(scope, prefix+"stage", "", "", false); err != nil {
						return err
					}
				}
			}
			for _, event := range item.Activity {
				if event.QueueID > 0 && event.QueueState != "" && snapshot.Integrations[event.Source] == model.IntegrationSuccess {
					title := ""
					mediaID := item.TMDBID
					if event.QueueState == model.StatusNeedsAttention {
						title = attentionTitle
					}
					if event.Source == "sonarr" {
						mediaID = item.TVDBID
					}
					key := fmt.Sprintf("queue/%s/%d/%d", event.Source, mediaID, event.QueueID)
					if err := change(scope, key, event.QueueState, title, queueBaselined[event.Source] || isNew); err != nil {
						return err
					}
				}
				if event.Date == "" || !model.AfterRequest(event.Date, cutoff) || snapshot.Integrations[event.Source] != model.IntegrationSuccess || (event.Type != "Download failed" && event.Type != "File deleted") {
					continue // Never replay history that already existed at baseline.
				}
				// History event identity is shared across requests for the same media.
				mediaID := item.TMDBID
				if event.Source == "sonarr" {
					mediaID = item.TVDBID
				}
				key := fmt.Sprintf("history/%s/%d/%s/%s", event.Source, mediaID, event.Type, event.Date)
				if err := change(scope, key, "1", attentionTitle, true); err != nil {
					return err
				}
			}
		}
	}
	for _, source := range []string{"radarr", "sonarr"} {
		if fetchErr == nil && snapshot.Integrations[source] == model.IntegrationSuccess {
			if err := change(integrations, "queue-baseline/"+source, "1", "", false); err != nil {
				return err
			}
		}
	}
	if !initialized {
		if err := change(integrations, "baseline", cutoff, "", false); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if !initialized {
		log.Printf("Notification baseline recorded: requests=%d; existing requests remain silent", len(snapshot.Requests))
	} else if state.queued > 0 {
		log.Printf("Notifications queued: count=%d", state.queued)
	}
	return nil
}

func requestEventTitle(status int) string {
	return map[int]string{3: attentionTitle, 4: attentionTitle}[status]
}

func mediaEventTitle(status int) string {
	return map[int]string{6: attentionTitle, 7: attentionTitle}[status]
}
