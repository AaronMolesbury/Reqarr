package server

import (
	"net/http"

	"reqarr/model"
)

func NewHandler(collect model.CollectFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", HealthHandler)
	mux.HandleFunc("GET /api/dashboard", DashboardHandler(collect))
	return mux
}
