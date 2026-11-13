package upstream

import (
	"context"
	"encoding/json"
	"net/http"

	"reqarr/config"
)

type SeerrRequest struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Status      int    `json:"status"`
	UpdatedAt   string `json:"updatedAt"`
	CreatedAt   string `json:"createdAt"`
	RequestedBy struct {
		DisplayName string `json:"displayName"`
		Username    string `json:"username"`
	} `json:"requestedBy"`
	Media struct {
		TMDBID         int               `json:"tmdbId"`
		TVDBID         int               `json:"tvdbId"`
		Status         int               `json:"status"`
		DownloadStatus []json.RawMessage `json:"downloadStatus"`
	} `json:"media"`
}

type SeerrPage struct {
	PageInfo struct {
		Results int `json:"results"`
	} `json:"pageInfo"`
	Results []SeerrRequest `json:"results"`
}

func FetchSeerrJSON(ctx context.Context, client *http.Client, baseURL, key, path string, target any) error {
	return FetchJSON(ctx, client, config.Service{URL: baseURL, Key: key}, path, "Seerr", 5<<20, target)
}
