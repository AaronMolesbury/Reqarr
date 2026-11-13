package notifications

import (
	"fmt"
	"strings"

	"reqarr/model"
)

const (
	newRequestTitle         = "Reqarr: New request"
	completedTitle          = "Reqarr: Completed"
	attentionTitle          = "Reqarr: Needs attention"
	serviceAttentionMessage = "A service needs attention; see dashboard for details."
)

// Only these outcomes are pushed; detailed progress remains on the dashboard.
// Legacy titles are accepted so queued deliveries also follow the current policy.
func pushContent(title string) (string, string, int) {
	switch strings.ToLower(title) {
	case "reqarr: new request":
		return newRequestTitle, "A new request was made.", 3
	case "reqarr: available", "reqarr: completed":
		return completedTitle, "Request fulfilled.", 3
	case "reqarr: needs attention", "reqarr: series needs attention", "reqarr: missing for 10 minutes",
		"reqarr: request failed", "reqarr: request declined", "reqarr: download failed",
		"reqarr: file deleted", "reqarr: media deleted", "reqarr: media blocklisted":
		return attentionTitle, "Request had a problem; see dashboard for details.", 4
	case "reqarr: integration needs attention":
		return attentionTitle, serviceAttentionMessage, 4
	default:
		return "", "", 0
	}
}

func attentionScope(item model.Request) string {
	if item.Type == "tv" && item.TVDBID > 0 {
		return fmt.Sprintf("sonarr/%d", item.TVDBID)
	}
	if item.Type == "movie" && item.TMDBID > 0 {
		return fmt.Sprintf("radarr/%d", item.TMDBID)
	}
	return fmt.Sprintf("request/%d", item.ID)
}

func currentProblem(item model.Request) bool {
	seerrAvailability := item.AvailabilitySource == "" || item.AvailabilitySource == "seerr"
	if item.RequestStatus == 3 || item.RequestStatus == 4 || (seerrAvailability && (item.MediaStatus == 6 || item.MediaStatus == 7)) || item.StatusOverride == model.StatusNeedsAttention {
		return true
	}
	for _, event := range item.Activity {
		if event.QueueState == model.StatusNeedsAttention {
			return true
		}
	}
	return false
}

func positiveProgress(key, value string) bool {
	if strings.HasSuffix(key, "/completion") {
		return value == "available"
	}
	if strings.HasSuffix(key, "/stage") || strings.HasPrefix(key, "queue/") {
		switch value {
		case model.StatusDownloading, model.StatusAwaitingImport, model.StatusQueued, model.StatusAwaitingRelease:
			return true
		}
	}
	return false
}
