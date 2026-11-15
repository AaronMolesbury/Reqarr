package upstream

import (
	"context"
	"fmt"
	"net/http"

	"reqarr/config"
)

type Series struct {
	ID         int    `json:"id"`
	TVDBID     int    `json:"tvdbId"`
	Status     string `json:"status"`
	Statistics struct {
		EpisodeFileCount int `json:"episodeFileCount"`
	} `json:"statistics"`
	Seasons []Season `json:"seasons"`
}

type Episode struct {
	ID            int    `json:"id"`
	SeriesID      int    `json:"seriesId"`
	SeasonNumber  *int   `json:"seasonNumber"`
	EpisodeNumber *int   `json:"episodeNumber"`
	HasFile       *bool  `json:"hasFile"`
	AirDateUTC    string `json:"airDateUtc"`
}

func FetchSonarrEpisodes(ctx context.Context, client *http.Client, service config.Service, seriesID int) ([]Episode, error) {
	var episodes []Episode
	err := FetchJSON(ctx, client, service, fmt.Sprintf("/api/v3/episode?seriesId=%d", seriesID), "Sonarr episodes", 20<<20, &episodes)
	return episodes, err
}

type Season struct {
	SeasonNumber *int              `json:"seasonNumber"`
	Statistics   *SeasonStatistics `json:"statistics"`
}

type SeasonStatistics struct {
	EpisodeFileCount  *int `json:"episodeFileCount"`
	TotalEpisodeCount *int `json:"totalEpisodeCount"`
}

func FetchSonarrSeries(ctx context.Context, client *http.Client, service config.Service) (map[int]Series, error) {
	var series []Series
	if err := FetchJSON(ctx, client, service, "/api/v3/series", "Sonarr", 20<<20, &series); err != nil {
		return nil, err
	}
	byID := make(map[int]Series, len(series))
	for _, show := range series {
		if show.TVDBID > 0 {
			byID[show.TVDBID] = show
		}
	}
	return byID, nil
}
