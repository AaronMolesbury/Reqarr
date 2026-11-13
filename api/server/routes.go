package server

import (
	"net/http"

	"reqarr/config"
	"reqarr/model"
)

func NewHandler(client *http.Client, seerr config.Service, collect model.CollectFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", HealthHandler)
	mux.HandleFunc("GET /api/dashboard", DashboardHandler(collect))
	mux.HandleFunc("GET /api/requests", RequestsHandler(client, seerr))
	return mux
}
