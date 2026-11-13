package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"reqarr/model"
)

func TestDashboardIntegrationResults(t *testing.T) {
	tests := []struct {
		name       string
		configured bool
		failPath   string
		empty      bool
		wantCode   int
		want       map[string]string
	}{
		{"unconfigured", false, "", false, 200, map[string]string{"seerr": "success", "radarr": "not_configured", "sonarr": "not_configured"}},
		{"success", true, "", false, 200, map[string]string{"seerr": "success", "radarr": "success", "sonarr": "success"}},
		{"empty library", true, "", true, 200, map[string]string{"seerr": "success", "radarr": "success", "sonarr": "success"}},
		{"seerr failure", true, "/api/v1/request", false, 502, map[string]string{"seerr": "error", "radarr": "unknown", "sonarr": "unknown"}},
		{"metadata failure", true, "/api/v1/movie/10", false, 200, map[string]string{"seerr": "partial", "radarr": "success", "sonarr": "success"}},
		{"radarr failure", true, "/radarr/api/v3/movie", false, 200, map[string]string{"seerr": "success", "radarr": "error", "sonarr": "success"}},
		{"sonarr failure", true, "/sonarr/api/v3/series", false, 200, map[string]string{"seerr": "success", "radarr": "success", "sonarr": "error"}},
		{"radarr queue failure", true, "/radarr/api/v3/queue", false, 200, map[string]string{"seerr": "success", "radarr": "partial", "sonarr": "success"}},
		{"radarr history failure", true, "/radarr/api/v3/history", false, 200, map[string]string{"seerr": "success", "radarr": "partial", "sonarr": "success"}},
		{"sonarr queue failure", true, "/sonarr/api/v3/queue", false, 200, map[string]string{"seerr": "success", "radarr": "success", "sonarr": "partial"}},
		{"sonarr history failure", true, "/sonarr/api/v3/history", false, 200, map[string]string{"seerr": "success", "radarr": "success", "sonarr": "partial"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := make(map[string]int)
			var callsMu sync.Mutex
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callsMu.Lock()
				calls[r.URL.Path]++
				callsMu.Unlock()
				if r.URL.Path == tc.failPath {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				switch r.URL.Path {
				case "/api/v1/request":
					if tc.empty {
						w.Write([]byte(`{"results":[]}`))
					} else {
						w.Write([]byte(`{"results":[{"id":1,"type":"movie","media":{"tmdbId":10,"status":3}},{"id":2,"type":"tv","media":{"tmdbId":20,"tvdbId":30,"status":3}}]}`))
					}
				case "/api/v1/movie/10", "/api/v1/tv/20":
					w.Write([]byte(`{"title":"Movie","name":"Series"}`))
				case "/radarr/api/v3/movie":
					w.Write([]byte(`[{"id":100,"tmdbId":10,"isAvailable":true}]`))
				case "/sonarr/api/v3/series":
					w.Write([]byte(`[{"id":200,"tvdbId":30}]`))
				case "/radarr/api/v3/queue", "/radarr/api/v3/history", "/sonarr/api/v3/queue", "/sonarr/api/v3/history":
					w.Write([]byte(`{"records":[],"totalRecords":0}`))
				default:
					t.Errorf("unexpected call: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			var radarr radarrConfig
			var sonarr sonarrConfig
			if tc.configured {
				radarr = radarrConfig{url: upstream.URL + "/radarr", key: "test"}
				sonarr = sonarrConfig{url: upstream.URL + "/sonarr", key: "test"}
			}
			response := httptest.NewRecorder()
			dashboardHandler(upstream.Client(), upstream.URL, "test", radarr, sonarr)(response, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
			if response.Code != tc.wantCode {
				t.Fatalf("HTTP %d, want %d", response.Code, tc.wantCode)
			}
			var result struct {
				Integrations map[string]string `json:"integrations"`
				Requests     []model.Request   `json:"requests"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			for service, want := range tc.want {
				if got := result.Integrations[service]; got != want {
					t.Errorf("%s = %q, want %q", service, got, want)
				}
			}
			if response.Code == 200 && !tc.empty && len(result.Requests) != 2 {
				t.Fatal("integration results must not discard request data")
			}
			if tc.empty && (calls["/radarr/api/v3/queue"] != 0 || calls["/sonarr/api/v3/queue"] != 0) {
				t.Fatal("must not add activity calls without matching requests")
			}
		})
	}
}
