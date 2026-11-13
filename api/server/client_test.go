package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestClientAndAPIRouting(t *testing.T) {
	files := fstest.MapFS{
		"index.html":        {Data: []byte("<html>Reqarr</html>")},
		"assets/app-123.js": {Data: []byte("console.log('reqarr')")},
	}
	api := http.NewServeMux()
	api.HandleFunc("GET /api/health", HealthHandler)
	handler := WithClient(api, files)
	for _, tc := range []struct {
		path  string
		code  int
		body  string
		cache string
	}{
		{"/", 200, "<html>Reqarr</html>", "no-cache"},
		{"/assets/app-123.js", 200, "console.log", "public, max-age=31536000, immutable"},
		{"/api/health", 200, `"status":"ok"`, "no-store"},
		{"/api/unknown", 404, "404", ""},
		{"/assets/", 404, "404", ""},
		{"/assets/missing.js", 404, "404", ""},
		{"/missing", 404, "404", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != tc.code || !strings.Contains(response.Body.String(), tc.body) || response.Header().Get("Cache-Control") != tc.cache {
				t.Fatalf("unexpected response: code=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
			}
		})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatal("static files must only accept GET and HEAD")
	}
}
