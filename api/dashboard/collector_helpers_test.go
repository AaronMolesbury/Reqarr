package dashboard

import (
	"context"
	"net/http"

	"reqarr/config"
	"reqarr/model"
	"reqarr/upstream"
)

func collectDashboard(ctx context.Context, client *http.Client, baseURL, key string, radarr, sonarr config.Service, caches ...*upstream.MetadataCache) (model.Snapshot, error) {
	c := NewCollector(client, config.Service{URL: baseURL, Key: key}, radarr, sonarr)
	if len(caches) > 0 {
		c.Metadata = caches[0]
	}
	return c.Collect(ctx)
}
