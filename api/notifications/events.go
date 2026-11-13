package notifications

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
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
	queued := 0
	change := func(key, value, title, message string, priority int, notifyFirst bool) error {
		old, exists, err := get(key)
		if err != nil || (exists && old == value) {
			return err
		}
		if err = tx.Set(ctx, key, value); err != nil {
			return err
		}
		if initialized && title != "" && (exists || notifyFirst) {
			err = tx.Queue(ctx, title, message, priority)
			if err == nil {
				queued++
			}
		}
		return err
	}
	queueBaselined := make(map[string]bool)
	for _, source := range []string{"radarr", "sonarr"} {
		_, queueBaselined[source], err = get("queue-baseline/" + source)
		if err != nil {
			return err
		}
	}
	for _, source := range []string{"seerr", "radarr", "sonarr"} {
		status := snapshot.Integrations[source]
		if status != model.IntegrationSuccess && status != model.IntegrationPartial && status != model.IntegrationError {
			continue
		}
		title, message, priority := "Reqarr: integration needs attention", "Some upstream data could not be retrieved. Check Reqarr.", 4
		value := "unhealthy"
		if status == model.IntegrationSuccess {
			value = model.IntegrationSuccess
			title, message, priority = "Reqarr: integration recovered", "Upstream checks are succeeding again.", 3
		}
		if err := change("integration/"+source, value, title, message, priority, status != model.IntegrationSuccess); err != nil {
			return err
		}
	}
	if fetchErr == nil {
		for _, item := range snapshot.Requests {
			prefix := fmt.Sprintf("request/%d/", item.ID)
			_, known, err := get(prefix + "seen")
			if err != nil {
				return err
			}
			isNew := !known && model.AfterRequest(item.CreatedAt, cutoff)
			if err := change(prefix+"seen", "1", "Reqarr: new request", "A new media request was received. Open Reqarr for details.", 3, isNew); err != nil {
				return err
			}
			// Existing requests remain silent initially; subsequent observed changes
			// are eligible. Newly discovered old requests are also baselined.
			notifyFirst := isNew
			for _, field := range []struct {
				key, value, title, message string
				priority                   int
			}{
				{"approval", strconv.Itoa(item.RequestStatus), requestEventTitle(item.RequestStatus), "A request's approval status changed. Open Reqarr for details.", requestEventPriority(item.RequestStatus)},
				{"media", strconv.Itoa(item.MediaStatus), mediaEventTitle(item.MediaStatus), "Seerr reports a media status update. Open Reqarr for details.", 3},
			} {
				if field.title == "" {
					continue // Unknown upstream values aren't notification events.
				}
				if err := change(prefix+field.key, field.value, field.title, field.message, field.priority, notifyFirst); err != nil {
					return err
				}
			}
			source := "radarr"
			if item.Type == "tv" {
				source = "sonarr"
			}
			// Missing/failed enrichment must not reset an incident or manufacture
			// a recovery. Only positively reported stages update the saved state.
			if snapshot.Integrations[source] == model.IntegrationSuccess && item.StatusOverride != "" {
				stage := item.StatusOverride
				title := "Reqarr: " + strings.ToLower(stage)
				message := "A request has a progress update. Check Reqarr for details."
				if stage == model.StatusMissing {
					message = "Radarr reports no imported file for a requested movie eligible for download. This does not mean a search or download failed."
					previousStage, _, err := get(prefix + "stage")
					if err != nil {
						return err
					}
					// Introducing a more precise label must not announce every
					// existing generic processing request as a new incident.
					if !isNew && previousStage == "" {
						title = ""
					}
				}
				priority := 3
				if item.StatusOverride == model.StatusNeedsAttention {
					priority = 4
					if item.StatusReason == model.MissingAttentionReason {
						stage = "Needs attention/missing"
						title = "Reqarr: missing for 10 minutes"
						message = model.MissingAttentionReason
					}
					for _, event := range item.Activity {
						if event.QueueID > 0 && event.QueueState == model.StatusNeedsAttention {
							// Stable queue IDs are tracked below. Changing membership
							// of a group of incidents must not re-alert old incidents.
							title = ""
						}
					}
				}
				if err := change(prefix+"stage", stage, title, message, priority, isNew); err != nil {
					return err
				}
			} else if snapshot.Integrations[source] == model.IntegrationSuccess {
				if _, exists, err := get(prefix + "stage"); err != nil {
					return err
				} else if !exists {
					if err := change(prefix+"stage", "", "", "", 0, false); err != nil {
						return err
					}
				}
			}
			for _, event := range item.Activity {
				if event.QueueID > 0 && event.QueueState != "" && snapshot.Integrations[event.Source] == model.IntegrationSuccess {
					priority := 3
					title := ""
					mediaID := item.TMDBID
					message := "Radarr reports a queue problem for a requested movie. Check Reqarr for details."
					if event.QueueState == model.StatusNeedsAttention {
						priority = 4
						title = "Reqarr: needs attention"
					}
					if event.Source == "sonarr" {
						mediaID = item.TVDBID
						title = "Reqarr: series " + strings.ToLower(event.QueueState)
						message = "Sonarr reports queue activity for a requested series. It may concern episodes outside the requested season. Check Reqarr."
					}
					key := fmt.Sprintf("queue/%s/%d/%d", event.Source, mediaID, event.QueueID)
					if err := change(key, event.QueueState, title, message, priority, queueBaselined[event.Source] || isNew); err != nil {
						return err
					}
				}
				if event.Date == "" || !model.AfterRequest(event.Date, cutoff) {
					continue // Never replay history that already existed at baseline.
				}
				priority := 3
				if event.Type == "Download failed" || event.Type == "File deleted" {
					priority = 4
				}
				message := "Radarr reports a new activity event. Open Reqarr for details. An import event alone does not confirm current availability."
				if event.Source == "sonarr" {
					message = "Sonarr reports new activity for a requested series. It may concern episodes outside the requested season. Open Reqarr for details."
				}
				// History event identity is shared across requests for the same media.
				mediaID := item.TMDBID
				if event.Source == "sonarr" {
					mediaID = item.TVDBID
				}
				key := fmt.Sprintf("history/%s/%d/%s/%s", event.Source, mediaID, event.Type, event.Date)
				if err := change(key, "1", "Reqarr: "+strings.ToLower(event.Type), message, priority, true); err != nil {
					return err
				}
			}
		}
	}
	for _, source := range []string{"radarr", "sonarr"} {
		if fetchErr == nil && snapshot.Integrations[source] == model.IntegrationSuccess {
			if err := change("queue-baseline/"+source, "1", "", "", 0, false); err != nil {
				return err
			}
		}
	}
	if !initialized {
		if err := change("baseline", cutoff, "", "", 0, false); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if !initialized {
		log.Printf("Notification baseline recorded: requests=%d; existing requests remain silent", len(snapshot.Requests))
	} else if queued > 0 {
		log.Printf("Notifications queued: count=%d", queued)
	}
	return nil
}

func requestEventTitle(status int) string {
	return map[int]string{1: "Reqarr: request pending", 2: "Reqarr: request approved", 3: "Reqarr: request declined", 4: "Reqarr: request failed", 5: "Reqarr: request marked completed"}[status]
}

func requestEventPriority(status int) int {
	if status == 4 {
		return 4
	}
	return 3
}

func mediaEventTitle(status int) string {
	return map[int]string{2: "Reqarr: media pending", 3: "Reqarr: processing", 4: "Reqarr: partially available", 5: "Reqarr: available", 6: "Reqarr: media blocklisted", 7: "Reqarr: media deleted"}[status]
}
