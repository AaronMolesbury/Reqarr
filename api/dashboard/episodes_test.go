package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"reqarr/config"
	"reqarr/model"
	"reqarr/upstream"
)

func TestEpisodeQueueMatchingIsConservative(t *testing.T) {
	season := 2
	otherSeason := 1
	for _, tc := range []struct {
		name  string
		queue []upstream.QueueRecord
		want  string
	}{
		{"exact download", []upstream.QueueRecord{{SeriesID: 7, EpisodeID: 42, Status: "downloading"}}, model.StatusDownloading},
		{"exact completed download", []upstream.QueueRecord{{SeriesID: 7, EpisodeID: 42, Status: "completed"}}, model.StatusAwaitingImport},
		{"different episode", []upstream.QueueRecord{{SeriesID: 7, EpisodeID: 43, SeasonNumber: &season, Status: "downloading"}}, ""},
		{"different series", []upstream.QueueRecord{{SeriesID: 8, EpisodeID: 42, Status: "downloading"}}, ""},
		{"unknown episode in same season", []upstream.QueueRecord{{SeriesID: 7, SeasonNumber: &season, Status: "downloading"}}, model.StatusUnknown},
		{"unknown episode in different season", []upstream.QueueRecord{{SeriesID: 7, SeasonNumber: &otherSeason, Status: "downloading"}}, ""},
		{"unknown season and episode", []upstream.QueueRecord{{SeriesID: 7, Status: "downloading"}}, model.StatusUnknown},
		{"unknown state", []upstream.QueueRecord{{SeriesID: 7, EpisodeID: 42, Status: "unrecognised"}}, model.StatusUnknown},
		{"missing queue identity", []upstream.QueueRecord{{Status: "downloading"}}, model.StatusUnknown},
		{"failed is not active", []upstream.QueueRecord{{SeriesID: 7, EpisodeID: 42, Status: "failed"}}, model.StatusNeedsAttention},
		{"active beats old failure", []upstream.QueueRecord{{SeriesID: 7, EpisodeID: 42, Status: "failed"}, {SeriesID: 7, EpisodeID: 42, Status: "downloading"}}, model.StatusDownloading},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := episodeQueueState(tc.queue, 7, 42, 2); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestEpisodeCollectionDeduplicatesSeriesAndRejectsFailedChecks(t *testing.T) {
	for _, mode := range []string{"success", "episode failure", "queue failure", "capped queue"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Api-Key") != "test" {
					t.Error("missing API key")
				}
				switch r.URL.Path {
				case "/api/v1/request":
					fmt.Fprint(w, `{"results":[{"id":1,"type":"tv","status":2,"seasons":[{"seasonNumber":1}],"media":{"tmdbId":10,"tvdbId":20,"status":3}},{"id":2,"type":"tv","status":2,"seasons":[{"seasonNumber":2}],"media":{"tmdbId":10,"tvdbId":20,"status":3}}]}`)
				case "/api/v1/tv/10":
					fmt.Fprint(w, `{"name":"A show"}`)
				case "/api/v3/series":
					fmt.Fprint(w, `[{"id":7,"tvdbId":20,"seasons":[{"seasonNumber":1,"statistics":{"episodeFileCount":1,"totalEpisodeCount":10}},{"seasonNumber":2,"statistics":{"episodeFileCount":1,"totalEpisodeCount":10}}]}]`)
				case "/api/v3/queue":
					if mode == "queue failure" {
						w.WriteHeader(503)
						return
					}
					if mode == "capped queue" {
						json.NewEncoder(w).Encode(map[string]any{"totalRecords": 500, "records": make([]upstream.QueueRecord, 500)})
						return
					}
					fmt.Fprint(w, `{"records":[]}`)
				case "/api/v3/history":
					fmt.Fprint(w, `{"records":[]}`)
				case "/api/v3/episode":
					calls++
					if r.URL.Query().Get("seriesId") != "7" {
						t.Error("wrong series correlation")
					}
					if mode == "episode failure" {
						w.WriteHeader(503)
						return
					}
					fmt.Fprint(w, `[{"id":41,"seriesId":7,"seasonNumber":1,"episodeNumber":2,"hasFile":false,"airDateUtc":"2026-01-01T00:00:00Z"},{"id":42,"seriesId":7,"seasonNumber":2,"episodeNumber":3,"hasFile":false,"airDateUtc":"2026-01-01T00:00:00Z"},{"id":43,"seriesId":8,"seasonNumber":1,"episodeNumber":3,"hasFile":false}]`)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer fixture.Close()
			service := config.Service{URL: fixture.URL, Key: "test"}
			snapshot, err := NewCollector(fixture.Client(), service, config.Service{}, service).Collect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if mode == "success" {
				if calls != 1 {
					t.Fatalf("shared series fetched %d times", calls)
				}
				for i, item := range snapshot.Requests {
					if item.EpisodeCheck == nil || len(item.EpisodeCheck.Episodes) != 1 || item.EpisodeCheck.Episodes[0].SeasonNumber != i+1 {
						t.Fatalf("wrong episode scope: %+v", item.EpisodeCheck)
					}
				}
			} else {
				for _, item := range snapshot.Requests {
					if item.EpisodeCheck != nil {
						t.Fatal("failed or capped data allowed a missing inference")
					}
				}
				if mode == "episode failure" && calls != 1 {
					t.Fatal("failed series lookup was retried within the same poll")
				}
				if mode != "episode failure" && calls != 0 {
					t.Fatal("episode lookups should be skipped without a complete queue")
				}
			}
		})
	}
}
