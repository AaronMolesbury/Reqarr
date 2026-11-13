package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"reqarr/config"
	"reqarr/model"
	"reqarr/upstream"
)

func TestSonarrAvailabilityUsesOnlyRequestedSeasons(t *testing.T) {
	for _, tc := range []struct {
		name      string
		seasons   string
		requested []int
		want      string
		known     bool
	}{
		{"requested complete", `[{"seasonNumber":1,"statistics":{"episodeFileCount":10,"totalEpisodeCount":10}},{"seasonNumber":2,"statistics":{"episodeFileCount":0,"totalEpisodeCount":12}}]`, []int{1}, "Available", true},
		{"different season complete", `[{"seasonNumber":1,"statistics":{"episodeFileCount":10,"totalEpisodeCount":10}},{"seasonNumber":2,"statistics":{"episodeFileCount":0,"totalEpisodeCount":12}}]`, []int{2}, "Processing", true},
		{"only part of requested content", `[{"seasonNumber":1,"statistics":{"episodeFileCount":10,"totalEpisodeCount":10}},{"seasonNumber":2,"statistics":{"episodeFileCount":0,"totalEpisodeCount":12}}]`, []int{1, 2}, "Partially available", true},
		{"future episodes still count", `[{"seasonNumber":1,"statistics":{"episodeFileCount":8,"episodeCount":8,"totalEpisodeCount":10}}]`, []int{1}, "Partially available", true},
		{"missing requested season", `[{"seasonNumber":1,"statistics":{"episodeFileCount":10,"totalEpisodeCount":10}}]`, []int{2}, "Unknown", false},
		{"no requested seasons", `[{"seasonNumber":1,"statistics":{"episodeFileCount":10,"totalEpisodeCount":10}}]`, nil, "Unknown", false},
		{"omitted total", `[{"seasonNumber":1,"statistics":{"episodeFileCount":10}}]`, []int{1}, "Unknown", false},
		{"omitted files", `[{"seasonNumber":1,"statistics":{"totalEpisodeCount":10}}]`, []int{1}, "Unknown", false},
		{"zero total", `[{"seasonNumber":1,"statistics":{"episodeFileCount":0,"totalEpisodeCount":0}}]`, []int{1}, "Unknown", false},
		{"null statistics", `[{"seasonNumber":1,"statistics":null}]`, []int{1}, "Unknown", false},
		{"invalid counts", `[{"seasonNumber":1,"statistics":{"episodeFileCount":12,"totalEpisodeCount":10}}]`, []int{1}, "Unknown", false},
		{"omitted season number is not specials", `[{"statistics":{"episodeFileCount":10,"totalEpisodeCount":10}}]`, []int{0}, "Unknown", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var show upstream.Series
			if err := json.Unmarshal([]byte(`{"statistics":{"episodeFileCount":100},"seasons":`+tc.seasons+`}`), &show); err != nil {
				t.Fatal(err)
			}
			status, known := sonarrStatusOverride(show, tc.requested)
			if status != tc.want || known != tc.known {
				t.Fatalf("got %q known=%t", status, known)
			}
		})
	}
}

func TestCollectorCorrelatesSeerrRequestedSeasons(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/request":
			fmt.Fprint(w, `{"results":[{"id":1,"type":"tv","seasons":[{"seasonNumber":2},{"seasonNumber":2}],"media":{"tmdbId":10,"tvdbId":20,"status":3}},{"id":2,"type":"tv","seasons":[{"seasonNumber":1}],"media":{"tmdbId":10,"tvdbId":20,"status":5}}]}`)
		case "/api/v1/tv/10":
			fmt.Fprint(w, `{"name":"A show"}`)
		case "/api/v3/series":
			fmt.Fprint(w, `[{"id":7,"tvdbId":20,"seasons":[{"seasonNumber":1,"statistics":{"episodeFileCount":0,"totalEpisodeCount":10}},{"seasonNumber":2,"statistics":{"episodeFileCount":8,"totalEpisodeCount":8}}]}]`)
		case "/api/v3/queue", "/api/v3/history":
			fmt.Fprint(w, `{"records":[]}`)
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
	if len(snapshot.Requests) != 2 {
		t.Fatal("missing requests")
	}
	first, second := snapshot.Requests[0], snapshot.Requests[1]
	if first.EffectiveStatus() != model.StatusAvailable || first.AvailabilitySource != "sonarr" || first.AvailabilityStale || len(first.RequestedSeasons) != 1 {
		t.Fatalf("requested season not correlated: %+v", first)
	}
	if second.EffectiveStatus() != model.StatusProcessing {
		t.Fatalf("Seerr overrode missing requested season files: %+v", second)
	}
}

func TestSourcePreservesConfirmedAvailabilityDuringUncertainty(t *testing.T) {
	for _, kind := range []string{"movie", "tv"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			source := &Source{Ctx: context.Background(), Collect: func(context.Context) (model.Snapshot, error) {
				calls++
				item := model.Request{ID: 1, Type: kind, TMDBID: 10, MediaStatus: 3, AvailabilitySource: "radarr", StatusOverride: model.StatusAvailable}
				if kind == "tv" {
					item.AvailabilitySource = "sonarr"
					item.TVDBID = 20
					item.RequestedSeasons = []int{2}
				}
				if calls == 2 || calls == 3 {
					item.StatusOverride = model.StatusUnknown
					item.AvailabilityStale = true
				}
				if calls == 4 {
					item.StatusOverride = model.StatusProcessing
				}
				return model.Snapshot{Requests: []model.Request{item}}, nil
			}}
			for call := 1; call <= 4; call++ {
				snapshot, err := source.Get(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				item := snapshot.Requests[0]
				want := model.StatusAvailable
				if call == 4 {
					want = model.StatusProcessing
				}
				if item.EffectiveStatus() != want || item.AvailabilityStale != (call == 2 || call == 3) {
					t.Fatalf("call %d: %+v", call, item)
				}
				source.mu.Lock()
				source.expires = time.Time{}
				source.mu.Unlock()
			}
		})
	}
}
