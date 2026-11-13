package server

import (
	"net/http"

	"reqarr/config"
	"reqarr/dashboard"
)

type radarrConfig struct{ url, key string }
type sonarrConfig struct{ url, key string }

func dashboardHandler(client *http.Client, baseURL, key string, radarr radarrConfig, sonarr sonarrConfig) http.HandlerFunc {
	c := dashboard.NewCollector(client, config.Service{URL: baseURL, Key: key}, config.Service{URL: radarr.url, Key: radarr.key}, config.Service{URL: sonarr.url, Key: sonarr.key})
	return DashboardHandler(c.Collect)
}
func newHandler(client *http.Client, baseURL, key string, integrations ...any) http.Handler {
	var radarr config.Service
	var sonarr config.Service
	for _, integration := range integrations {
		switch v := integration.(type) {
		case radarrConfig:
			radarr = config.Service{URL: v.url, Key: v.key}
		case sonarrConfig:
			sonarr = config.Service{URL: v.url, Key: v.key}
		}
	}
	c := dashboard.NewCollector(client, config.Service{URL: baseURL, Key: key}, radarr, sonarr)
	return NewHandler(client, config.Service{URL: baseURL, Key: key}, c.Collect)
}
