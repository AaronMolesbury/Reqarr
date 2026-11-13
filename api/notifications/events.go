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
	queued := 0
	scope := "integrations"
	canRecover := false
	change := func(key, value, title, message string, priority int, notifyFirst bool) error {
		title, message, priority = pushContent(title)
		old, exists, err := get(key)
		if err != nil {
			return err
		}
		attentionKey := "attention/" + scope
		active, attentionKnown, err := get(attentionKey)
		if err != nil {
			return err
		}
		if exists && old == value {
			// Seed unchanged legacy incidents once. Old history must not undo recovery.
			if title == attentionTitle && !attentionKnown && !canRecover {
				return tx.Set(ctx, attentionKey, "active")
			}
			return nil
		}
		if title == attentionTitle {
			if err := tx.Set(ctx, attentionKey, "active"); err != nil {
				return err
			}
		}
		if err = tx.Set(ctx, key, value); err != nil {
			return err
		}
		if canRecover && exists && positiveProgress(key, value) {
			if err := tx.Set(ctx, attentionKey, "clear"); err != nil {
				return err
			}
		}
		if title == attentionTitle && active == "active" {
			return nil
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
		title, message, priority := "Reqarr: integration needs attention", serviceAttentionMessage, 4
		value := "unhealthy"
		if status == model.IntegrationSuccess {
			value = model.IntegrationSuccess
			title, message, priority = "", "", 0
		}
		if err := change("integration/"+source, value, title, message, priority, status != model.IntegrationSuccess); err != nil {
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
			scope = attentionScope(item)
			canRecover = !problems[scope]
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
			if err := change(prefix+"seen", "1", "Reqarr: new request", "A new request was made.", 3, isNew); err != nil {
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
				key, value, title, message string
				priority                   int
			}{
				{"approval", strconv.Itoa(item.RequestStatus), requestEventTitle(item.RequestStatus), "", requestEventPriority(item.RequestStatus)},
				{"media", strconv.Itoa(item.MediaStatus), mediaTitle, "", 3},
			} {
				if err := change(prefix+field.key, field.value, field.title, field.message, field.priority, notifyFirst); err != nil {
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
				if err := change(prefix+"completion", completion, title, "", 3, isNew); err != nil {
					return err
				}
			}
			source := "radarr"
			if item.Type == "tv" {
				source = "sonarr"
			}
			// Missing/failed enrichment must not reset an incident or manufacture
			// a recovery. Only positively reported stages update the saved state.
			if !item.AvailabilityStale && snapshot.Integrations[source] == model.IntegrationSuccess && item.StatusOverride != "" {
				stage := item.StatusOverride
				title := ""
				if stage == model.StatusNeedsAttention {
					title = attentionTitle
					if item.StatusReason == model.MissingAttentionReason {
						stage = "Needs attention/missing"
					}
					for _, event := range item.Activity {
						if event.QueueID > 0 && event.QueueState == model.StatusNeedsAttention {
							title = "" // Queue evidence identifies the same incident below.
						}
					}
				}
				message, priority := "", 4
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
					message := ""
					if event.QueueState == model.StatusNeedsAttention {
						priority = 4
						title = attentionTitle
					}
					if event.Source == "sonarr" {
						mediaID = item.TVDBID
					}
					key := fmt.Sprintf("queue/%s/%d/%d", event.Source, mediaID, event.QueueID)
					if err := change(key, event.QueueState, title, message, priority, queueBaselined[event.Source] || isNew); err != nil {
						return err
					}
				}
				if event.Date == "" || !model.AfterRequest(event.Date, cutoff) || snapshot.Integrations[event.Source] != model.IntegrationSuccess || (event.Type != "Download failed" && event.Type != "File deleted") {
					continue // Never replay history that already existed at baseline.
				}
				priority, message := 4, ""
				// History event identity is shared across requests for the same media.
				mediaID := item.TMDBID
				if event.Source == "sonarr" {
					mediaID = item.TVDBID
				}
				key := fmt.Sprintf("history/%s/%d/%s/%s", event.Source, mediaID, event.Type, event.Date)
				if err := change(key, "1", attentionTitle, message, priority, true); err != nil {
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
	return map[int]string{3: attentionTitle, 4: attentionTitle}[status]
}

func requestEventPriority(status int) int {
	if status == 4 {
		return 4
	}
	return 3
}

func mediaEventTitle(status int) string {
	return map[int]string{6: attentionTitle, 7: attentionTitle}[status]
}
