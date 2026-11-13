package model

import (
	"context"
)

type Request struct {
	ID              int        `json:"id"`
	Title           string     `json:"title"`
	Type            string     `json:"type"`
	Requester       string     `json:"requester"`
	RequestStatus   int        `json:"requestStatus"`
	MediaStatus     int        `json:"mediaStatus"`
	UpdatedAt       string     `json:"updatedAt"`
	CreatedAt       string     `json:"createdAt"`
	TMDBID          int        `json:"tmdbId"`
	TVDBID          int        `json:"tvdbId,omitempty"`
	DownloadEntries int        `json:"downloadEntries"`
	StatusOverride  string     `json:"statusOverride,omitempty"`
	StatusReason    string     `json:"statusReason,omitempty"`
	RadarrHasFile   *bool      `json:"-"`
	RadarrStatus    string     `json:"radarrStatus,omitempty"`
	SonarrStatus    string     `json:"sonarrStatus,omitempty"`
	Activity        []Activity `json:"activity,omitempty"`
}

type Snapshot struct {
	CheckedAt    string            `json:"checkedAt,omitempty"`
	Requests     []Request         `json:"requests"`
	Total        int               `json:"total"`
	Integrations map[string]string `json:"integrations"`
}

type CollectFunc func(context.Context) (Snapshot, error)
