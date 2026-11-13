package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// WithClient serves the production client alongside the API on the same origin.
func WithClient(api http.Handler, files fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	static := http.FileServer(http.FS(files))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		info, err := fs.Stat(files, name)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasPrefix(name, "assets/") {
			// Vite includes content hashes in generated asset names.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		static.ServeHTTP(w, r)
	})
	return mux
}
