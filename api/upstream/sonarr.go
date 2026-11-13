package upstream

import (
	"context"
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
