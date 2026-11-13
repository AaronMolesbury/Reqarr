package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reqarr/model"
)

func TestRequestsForwardAndKeepSeerrJSON(t *testing.T) {
	seerr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/request" || r.URL.Query().Get("take") != "7" || r.URL.Query().Get("filter") != "all" {
			t.Errorf("unexpected Seerr URL: %s", r.URL.String())
		}
		if r.Header.Get("X-Api-Key") != "secret" {
			t.Errorf("missing API key")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pageInfo":{"pages":1},"results":[{"id":4,"media":{"status":5}}]}`))
	}))
	defer seerr.Close()

	proxy := httptest.NewServer(newHandler(seerr.Client(), seerr.URL, "secret"))
	defer proxy.Close()

	resp, err := proxy.Client().Get(proxy.URL + "/api/requests?take=7&filter=all&ignored=yes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var data struct {
		PageInfo struct {
			Pages int `json:"pages"`
		} `json:"pageInfo"`
		Results []struct {
			ID int `json:"id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data.PageInfo.Pages != 1 || len(data.Results) != 1 || data.Results[0].ID != 4 {
		t.Fatalf("unexpected result: %+v", data)
	}
}

func TestUpstreamFailure(t *testing.T) {
	seerr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer seerr.Close()
	proxy := httptest.NewServer(newHandler(seerr.Client(), seerr.URL, "secret"))
	defer proxy.Close()
	resp, err := proxy.Client().Get(proxy.URL + "/api/requests")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestDashboardPaginatesAndResolvesTitle(t *testing.T) {
	calls := 0
	seerr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret" {
			t.Error("missing API key")
		}
		switch r.URL.Path {
		case "/api/v1/request":
			calls++
			if r.URL.Query().Get("skip") == "0" {
				w.Write([]byte(`{"pageInfo":{"results":2},"results":[{"id":26,"type":"movie","status":2,"media":{"tmdbId":123,"status":5,"downloadStatus":[]},"requestedBy":{"username":"tester"}}]}`))
			} else {
				w.Write([]byte(`{"pageInfo":{"results":2},"results":[]}`))
			}
		case "/api/v1/movie/123":
			w.Write([]byte(`{"title":"Verified title"}`))
		default:
			t.Errorf("unexpected URL: %s", r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer seerr.Close()
	proxy := httptest.NewServer(newHandler(seerr.Client(), seerr.URL, "secret"))
	defer proxy.Close()
	resp, err := proxy.Client().Get(proxy.URL + "/api/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var result struct {
		Requests []model.Request `json:"requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Requests) != 1 || result.Requests[0].Title != "Verified title" {
		t.Fatalf("wrong dashboard: %+v", result)
	}
	if calls != 1 {
		t.Fatalf("expected one request page call, got %d", calls)
	}
}

func TestDashboardRadarrAvailability(t *testing.T) {
	seerr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/request":
			_, _ = w.Write([]byte(`{"pageInfo":{"results":3},"results":[{"id":1,"type":"movie","media":{"tmdbId":1765072,"status":3}},{"id":2,"type":"movie","media":{"tmdbId":5,"status":5}},{"id":3,"type":"tv","media":{"tmdbId":99,"status":3}}]}`))
		case "/api/v1/movie/1765072", "/api/v1/movie/5", "/api/v1/tv/99":
			_, _ = w.Write([]byte(`{"title":"Movie","name":"Series"}`))
		default:
			t.Errorf("unexpected Seerr path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer seerr.Close()
	radarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "radarr-secret" {
			t.Errorf("unexpected Radarr request: %v", r)
		}
		if r.URL.Path != "/api/v3/movie" {
			_, _ = w.Write([]byte(`{"records":[],"totalRecords":0}`))
			return
		}
		_, _ = w.Write([]byte(`[{"tmdbId":1765072,"id":7,"status":"inCinemas","isAvailable":false,"hasFile":false},{"tmdbId":5,"id":8,"status":"released","isAvailable":true,"hasFile":true}]`))
	}))
	defer radarr.Close()

	server := httptest.NewServer(newHandler(seerr.Client(), seerr.URL, "secret", radarrConfig{url: radarr.URL, key: "radarr-secret"}))
	defer server.Close()
	resp, err := server.Client().Get(server.URL + "/api/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("HTTP %d", resp.StatusCode)
	}
	var got struct {
		Requests []model.Request `json:"requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Requests) != 3 {
		t.Fatalf("got %d requests", len(got.Requests))
	}
	if got.Requests[0].StatusOverride != "Awaiting release" || got.Requests[0].RadarrStatus != "inCinemas" {
		t.Fatalf("expected awaiting release: %+v", got.Requests[0])
	}
	if got.Requests[1].StatusOverride != "Available" {
		t.Fatalf("available movie overridden: %+v", got.Requests[1])
	}
	if got.Requests[2].StatusOverride != "" {
		t.Fatalf("TV overridden: %+v", got.Requests[2])
	}
}

func TestDashboardRadarrFailureLeavesAvailabilityUnconfirmed(t *testing.T) {
	seerr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/request" {
			_, _ = w.Write([]byte(`{"pageInfo":{"results":1},"results":[{"id":1,"type":"movie","media":{"tmdbId":15,"status":3}}]}`))
		} else {
			_, _ = w.Write([]byte(`{"title":"Test"}`))
		}
	}))
	defer seerr.Close()
	radarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer radarr.Close()
	server := httptest.NewServer(newHandler(seerr.Client(), seerr.URL, "secret", radarrConfig{url: radarr.URL, key: "key"}))
	defer server.Close()
	resp, err := server.Client().Get(server.URL + "/api/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("HTTP %d", resp.StatusCode)
	}
	var got struct {
		Requests []model.Request `json:"requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Requests) != 1 || got.Requests[0].MediaStatus != 3 || got.Requests[0].StatusOverride != "Unknown" || !got.Requests[0].AvailabilityStale {
		t.Fatalf("unexpected fallback %+v", got.Requests)
	}
}

func TestDashboardSonarrMatchingRequiresRequestedSeasonEvidence(t *testing.T) {
	seerr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/request":
			_, _ = w.Write([]byte(`{"pageInfo":{"results":3},"results":[{"id":1,"type":"tv","media":{"tmdbId":100,"status":3}},{"id":2,"type":"tv","media":{"tmdbId":200,"tvdbId":202,"status":3}},{"id":3,"type":"tv","media":{"tmdbId":300,"status":5}}]}`))
		case "/api/v1/tv/100":
			_, _ = w.Write([]byte(`{"name":"Upcoming show","externalIds":{"tvdbId":101}}`))
		case "/api/v1/tv/200":
			_, _ = w.Write([]byte(`{"name":"Continuing show"}`))
		case "/api/v1/tv/300":
			_, _ = w.Write([]byte(`{"name":"Available show","mediaInfo":{"tvdbId":303}}`))
		default:
			t.Errorf("unexpected Seerr path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer seerr.Close()

	sonarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "sonarr-secret" {
			t.Errorf("unexpected Sonarr request %s", r.URL.Path)
		}
		if r.URL.Path != "/api/v3/series" {
			_, _ = w.Write([]byte(`{"records":[],"totalRecords":0}`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":11,"tvdbId":101,"status":"upcoming","statistics":{"episodeFileCount":0}},{"id":12,"tvdbId":202,"status":"continuing","statistics":{"episodeFileCount":0}},{"id":13,"tvdbId":303,"status":"upcoming","statistics":{"episodeFileCount":0}}]`))
	}))
	defer sonarr.Close()

	server := httptest.NewServer(newHandler(seerr.Client(), seerr.URL, "key", sonarrConfig{url: sonarr.URL, key: "sonarr-secret"}))
	defer server.Close()
	resp, err := server.Client().Get(server.URL + "/api/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var data struct {
		Requests []model.Request `json:"requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Requests) != 3 {
		t.Fatalf("requests: %+v", data.Requests)
	}
	if data.Requests[0].TVDBID != 101 || data.Requests[0].SonarrStatus != "upcoming" || data.Requests[0].StatusOverride != "Unknown" || !data.Requests[0].AvailabilityStale {
		t.Errorf("upcoming series %+v", data.Requests[0])
	}
	if data.Requests[1].TVDBID != 202 || data.Requests[1].StatusOverride != "Unknown" || !data.Requests[1].AvailabilityStale {
		t.Errorf("continuing series %+v", data.Requests[1])
	}
	if data.Requests[2].TVDBID != 303 || data.Requests[2].StatusOverride != "Unknown" || !data.Requests[2].AvailabilityStale {
		t.Errorf("available series %+v", data.Requests[2])
	}
}

func TestDashboardSonarrFailureLeavesAvailabilityUnconfirmed(t *testing.T) {
	seerr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/request" {
			_, _ = w.Write([]byte(`{"pageInfo":{"results":1},"results":[{"id":7,"type":"tv","media":{"tmdbId":10,"tvdbId":20,"status":3}}]}`))
		} else {
			_, _ = w.Write([]byte(`{"name":"Test"}`))
		}
	}))
	defer seerr.Close()
	sonarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer sonarr.Close()
	server := httptest.NewServer(newHandler(seerr.Client(), seerr.URL, "key", sonarrConfig{url: sonarr.URL, key: "key"}))
	defer server.Close()
	resp, err := server.Client().Get(server.URL + "/api/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var data struct {
		Requests []model.Request `json:"requests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || len(data.Requests) != 1 || data.Requests[0].StatusOverride != "Unknown" || !data.Requests[0].AvailabilityStale {
		t.Fatalf("fallback %+v; status %d", data, resp.StatusCode)
	}
}
