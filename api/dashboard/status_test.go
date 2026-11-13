package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"reqarr/config"
)

func TestDashboardRadarrMissingAndQueuePrecedence(t *testing.T) {
	tests := []struct {
		name        string
		fields      string
		mediaStatus int
		queue       string
		want        string
	}{
		{"eligible without file", `"isAvailable":true,"hasFile":false`, 3, "", "Missing"},
		{"not eligible without file", `"isAvailable":false,"hasFile":false`, 3, "", "Awaiting release"},
		{"imported file", `"isAvailable":true,"hasFile":true`, 3, "", ""},
		{"eligibility omitted", `"hasFile":false`, 3, "", ""},
		{"eligibility null", `"isAvailable":null,"hasFile":false`, 3, "", ""},
		{"file field omitted", `"isAvailable":true`, 3, "", ""},
		{"file field null", `"isAvailable":true,"hasFile":null`, 3, "", ""},
		{"all evidence absent", `"status":"released"`, 3, "", ""},
		{"available stays available", `"isAvailable":true,"hasFile":false`, 5, "", ""},
		{"partial stays partial", `"isAvailable":true,"hasFile":false`, 4, "", ""},
		{"pending stays pending", `"isAvailable":true,"hasFile":false`, 2, "", ""},
		{"download overrides missing", `"isAvailable":true,"hasFile":false`, 3, `{"movieId":7,"status":"downloading"}`, "Downloading"},
		{"completed is awaiting import", `"isAvailable":true,"hasFile":false`, 3, `{"movieId":7,"status":"completed"}`, "Awaiting import"},
		{"queued overrides missing", `"isAvailable":true,"hasFile":false`, 3, `{"movieId":7,"status":"queued"}`, "Queued"},
		{"explicit failure overrides missing", `"isAvailable":true,"hasFile":false`, 3, `{"movieId":7,"status":"failed"}`, "Needs attention"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixtureServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/request":
					fmt.Fprintf(w, `{"results":[{"id":1,"type":"movie","media":{"tmdbId":10,"status":%d}}]}`, tc.mediaStatus)
				case "/api/v1/movie/10":
					fmt.Fprint(w, `{"title":"A movie"}`)
				case "/api/v3/movie":
					fmt.Fprintf(w, `[{"id":7,"tmdbId":10,%s}]`, tc.fields)
				case "/api/v3/queue":
					fmt.Fprintf(w, `{"records":[%s]}`, tc.queue)
				case "/api/v3/history":
					fmt.Fprint(w, `{"records":[]}`)
				default:
					t.Errorf("unexpected fixtureServer path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer fixtureServer.Close()
			snapshot, err := collectDashboard(context.Background(), fixtureServer.Client(), fixtureServer.URL, "test", config.Service{URL: fixtureServer.URL, Key: "test"}, config.Service{})
			if err != nil || len(snapshot.Requests) != 1 {
				t.Fatalf("could not collect dashboard: %v", err)
			}
			if got := snapshot.Requests[0].StatusOverride; got != tc.want {
				t.Errorf("status override %q, want %q", got, tc.want)
			}
			if snapshot.Requests[0].MediaStatus != tc.mediaStatus {
				t.Fatal("Radarr enrichment modified Seerr's availability evidence")
			}
		})
	}
}
