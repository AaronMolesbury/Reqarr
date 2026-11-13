package dashboard

import (
	"fmt"
	"strings"

	"reqarr/model"
	"reqarr/upstream"
)

// Current state comes from the queue. A historical grab alone is not proof
// that a download is still active, and completed != imported.
func queueState(q upstream.QueueRecord) string {
	status := strings.ToLower(q.Status)
	tracked := strings.ToLower(q.TrackedState)
	switch {
	case status == "failed" || tracked == "importfailed" || tracked == "importblocked":
		return model.StatusNeedsAttention
	case status == "downloading":
		return model.StatusDownloading
	case status == "completed" || tracked == "importpending" || tracked == "importing":
		return model.StatusAwaitingImport
	case status == "queued" || status == "paused" || status == "delay":
		return model.StatusQueued
	default:
		return ""
	}
}
func queueMessage(q upstream.QueueRecord) string {
	// Keep dashboard text generic; details and paths may be sensitive.
	if q.ErrorMessage != "" || len(q.StatusMessages) > 0 {
		return "See Radarr/Sonarr for queue details"
	}
	if q.Size > 0 && q.SizeLeft >= 0 && q.SizeLeft <= q.Size && strings.EqualFold(q.Status, "downloading") {
		return fmt.Sprintf("%.0f%% downloaded", 100*(q.Size-q.SizeLeft)/q.Size)
	}
	return ""
}
func historyType(raw string) string {
	switch strings.ToLower(raw) {
	case "grabbed":
		return "Grabbed"
	case "downloadfolderimported":
		return "Imported"
	case "downloadfailed":
		return "Download failed"
	case "moviefiledeleted", "episodefiledeleted":
		return "File deleted"
	default:
		return ""
	}
}
func applyActivity(items []model.Request, mediaIDs map[int]int, queue []upstream.QueueRecord, history []upstream.HistoryRecord, source string) {
	byID := make(map[int][]upstream.QueueRecord)
	for _, q := range queue {
		id := q.MovieID
		if source == "sonarr" {
			id = q.SeriesID
		}
		if id > 0 {
			byID[id] = append(byID[id], q)
		}
	}
	byHistory := make(map[int][]upstream.HistoryRecord)
	for _, h := range history {
		id := h.MovieID
		if source == "sonarr" {
			id = h.SeriesID
		}
		if id > 0 {
			byHistory[id] = append(byHistory[id], h)
		}
	}
	for i := range items {
		if (source == "radarr" && items[i].Type != "movie") || (source == "sonarr" && items[i].Type != "tv") {
			continue
		}
		id := mediaIDs[i]
		if id == 0 {
			continue
		}
		// Imported content stays available; the queue may represent an
		// optional quality upgrade or another TV episode.
		if items[i].EffectiveStatus() != model.StatusAvailable && !items[i].AvailabilityStale {
			for _, q := range byID[id] {
				state := queueState(q)
				if source == "radarr" && state != "" && items[i].StatusOverride != model.StatusNeedsAttention {
					// A warning/failure is explicit, but may still be unrelated to the
					// requested TV season. Do not override for vague warning states.
					if state == model.StatusNeedsAttention && source == "sonarr" {
						continue
					}
					items[i].StatusOverride = state
				}
				items[i].Activity = append(items[i].Activity, model.Activity{Source: source, Type: "Queue: " + q.Status, Message: queueMessage(q), QueueID: q.ID, QueueState: state})
			}
		}
		for _, h := range byHistory[id] {
			kind := historyType(h.EventType)
			if kind == "" || !model.AfterRequest(h.Date, items[i].CreatedAt) {
				continue
			}
			if len(items[i].Activity) >= 12 {
				break
			}
			items[i].Activity = append(items[i].Activity, model.Activity{Source: source, Type: kind, Date: h.Date})
		}
	}
}
