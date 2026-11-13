package dashboard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"reqarr/config"
	"reqarr/model"
	"reqarr/server"
	"reqarr/upstream"
)

func TestDashboardSourceSharesPollAndSurvivesBrowserCancellation(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	source := &Source{
		Ctx: context.Background(), Cooldown: 90 * time.Second,
		Collect: func(ctx context.Context) (model.Snapshot, error) {
			if calls.Add(1) == 1 {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return model.Snapshot{}, ctx.Err()
				}
			}
			return model.Snapshot{Requests: []model.Request{{ID: 1}}}, nil
		},
	}
	browserCtx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := source.Get(browserCtx); first <- err }()
	<-started
	response := httptest.NewRecorder()
	second := make(chan struct{})
	go func() {
		server.DashboardHandler(source.Get)(response, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
		close(second)
	}()
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled browser should stop waiting")
	}
	close(release)
	<-second
	if response.Code != 200 {
		t.Fatalf("replacement dashboard request: HTTP %d", response.Code)
	}
	// The monitor reuses the same snapshot instead of making another poll.
	snapshot, err := source.Get(context.Background())
	if err != nil || len(snapshot.Requests) != 1 || calls.Load() != 1 {
		t.Fatalf("dashboard and monitor did not share a single poll: calls=%d err=%v", calls.Load(), err)
	}
	source.mu.Lock()
	source.expires = time.Time{}
	source.mu.Unlock()
	if _, err := source.Get(context.Background()); err != nil || calls.Load() != 2 {
		t.Fatal("expired snapshots must refresh")
	}
}

func TestDashboardSourceCachesFailuresToProtectUpstream(t *testing.T) {
	var calls atomic.Int32
	failure := errors.New("fixtureServer unavailable")
	source := &Source{
		Ctx: context.Background(), Cooldown: 90 * time.Second,
		Collect: func(context.Context) (model.Snapshot, error) {
			calls.Add(1)
			return model.Snapshot{Integrations: map[string]string{"seerr": "error"}}, failure
		},
	}
	for range 20 {
		snapshot, err := source.Get(context.Background())
		if !errors.Is(err, failure) || snapshot.Integrations["seerr"] != "error" {
			t.Fatal("cached failures must remain errors, not successful stale snapshots")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("retries hammered the fixtureServer: calls=%d", calls.Load())
	}
}

func TestMetadataCacheReducesCallsWithoutCachingAvailability(t *testing.T) {
	var metadataCalls, listCalls atomic.Int32
	fixtureServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/request":
			if listCalls.Add(1) == 1 {
				w.Write([]byte(`{"results":[{"id":1,"type":"movie","media":{"tmdbId":10,"status":3}},{"id":2,"type":"movie","media":{"tmdbId":10,"status":3}}]}`))
			} else {
				w.Write([]byte(`{"results":[{"id":1,"type":"movie","media":{"tmdbId":10,"status":5}}]}`))
			}
		case "/api/v1/movie/10":
			metadataCalls.Add(1)
			w.Write([]byte(`{"title":"A title"}`))
		default:
			t.Errorf("unexpected fixtureServer path: %s", r.URL.Path)
		}
	}))
	defer fixtureServer.Close()
	metadata := &upstream.MetadataCache{}
	for _, status := range []int{3, 5} {
		snapshot, err := collectDashboard(context.Background(), fixtureServer.Client(), fixtureServer.URL, "test", config.Service{}, config.Service{}, metadata)
		if err != nil || snapshot.Requests[0].MediaStatus != status || snapshot.Requests[0].Title != "A title" {
			t.Fatalf("metadata caching changed current availability: %+v, %v", snapshot, err)
		}
	}
	if metadataCalls.Load() != 1 || listCalls.Load() != 2 {
		t.Fatalf("expected two fresh request lists but only one title lookup: lists=%d metadata=%d", listCalls.Load(), metadataCalls.Load())
	}
}

func TestMetadataFailureStopsRemainingLookupBurst(t *testing.T) {
	var calls atomic.Int32
	fixtureServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer fixtureServer.Close()
	metadata := &upstream.MetadataCache{}
	for id := 1; id <= 24; id++ {
		if _, err := metadata.Lookup(context.Background(), fixtureServer.Client(), fixtureServer.URL, "test", "movie", id); err == nil {
			t.Fatal("failed metadata must remain an error")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("a failed lookup caused a request burst: %d", calls.Load())
	}
}

func TestDashboardSourceUsesLongerCooldownForFailure(t *testing.T) {
	failed := false
	source := &Source{
		Ctx: context.Background(), Cooldown: 15 * time.Second, ErrorCooldown: 90 * time.Second,
		Collect: func(context.Context) (model.Snapshot, error) {
			if failed {
				return model.Snapshot{}, errors.New("unavailable")
			}
			return model.Snapshot{}, nil
		},
	}
	if _, err := source.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	source.mu.Lock()
	successWait := time.Until(source.expires)
	source.expires = time.Time{}
	source.mu.Unlock()
	failed = true
	if _, err := source.Get(context.Background()); err == nil {
		t.Fatal("expected fixtureServer failure")
	}
	source.mu.Lock()
	failureWait := time.Until(source.expires)
	source.mu.Unlock()
	if successWait < 14*time.Second || successWait > 15*time.Second || failureWait < 89*time.Second || failureWait > 90*time.Second {
		t.Fatalf("unexpected cooldowns: success=%s failure=%s", successWait, failureWait)
	}
}
