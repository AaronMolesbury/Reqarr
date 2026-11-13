package upstream

import (
	"context"
	"net/http"

	"reqarr/config"
)

type Movie struct {
	ID          int    `json:"id"`
	TMDBID      int    `json:"tmdbId"`
	IsAvailable *bool  `json:"isAvailable"`
	HasFile     *bool  `json:"hasFile"`
	Status      string `json:"status"`
}

func FetchRadarrMovies(ctx context.Context, client *http.Client, service config.Service) (map[int]Movie, error) {
	var movies []Movie
	if err := FetchJSON(ctx, client, service, "/api/v3/movie", "Radarr", 20<<20, &movies); err != nil {
		return nil, err
	}
	byID := make(map[int]Movie, len(movies))
	for _, movie := range movies {
		if movie.TMDBID > 0 {
			byID[movie.TMDBID] = movie
		}
	}
	return byID, nil
}
