package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"reqarr/config"
)

func RequestsHandler(client *http.Client, seerr config.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upstream, err := url.Parse(seerr.URL + "/api/v1/request")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid Seerr configuration"})
			return
		}

		// Only pass query parameters supported by the Seerr request-list endpoint.
		query := url.Values{}
		for _, key := range []string{"take", "skip", "filter", "sort", "sortDirection", "requestedBy", "mediaType"} {
			if values, ok := r.URL.Query()[key]; ok {
				for _, value := range values {
					query.Add(key, value)
				}
			}
		}
		upstream.RawQuery = query.Encode()

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream.String(), nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not prepare Seerr request"})
			return
		}
		req.Header.Set("X-Api-Key", seerr.Key)
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Seerr request failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Seerr unavailable"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("Seerr responded with status %d", resp.StatusCode)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("Seerr returned HTTP %d", resp.StatusCode)})
			return
		}

		// Preserve Seerr's raw JSON for the private debugging endpoint.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		if _, err := io.Copy(w, io.LimitReader(resp.Body, 10<<20)); err != nil {
			log.Printf("Seerr response copy: %v", err)
		}
	}
}
