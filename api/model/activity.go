package model

import (
	"time"
)

// Activity is a recent event reported by Radarr/Sonarr, not a permanent
// event log. History may be truncated by the upstream application.
type Activity struct {
	Source     string `json:"source"`
	Type       string `json:"type"`
	Date       string `json:"date,omitempty"`
	Message    string `json:"message,omitempty"`
	QueueID    int    `json:"-"`
	QueueState string `json:"-"`
}

func AfterRequest(eventTime, requestTime string) bool {
	event, e1 := time.Parse(time.RFC3339Nano, eventTime)
	request, e2 := time.Parse(time.RFC3339Nano, requestTime)
	// If timestamps can't be compared, don't risk attributing older activity.
	return e1 == nil && e2 == nil && !event.Before(request)
}
