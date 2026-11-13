package server

import (
	"context"
	"net/http"

	"reqarr/model"
)

func DashboardHandler(collect func(context.Context) (model.Snapshot, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := collect(r.Context())
		if r.Context().Err() != nil {
			return
		}
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "integrations": snapshot.Integrations})
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	}
}
