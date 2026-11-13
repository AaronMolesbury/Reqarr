package dashboard

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"reqarr/config"
	"reqarr/model"
	"reqarr/upstream"
)

type Collector struct {
	Client   *http.Client
	Seerr    config.Service
	Radarr   config.Service
	Sonarr   config.Service
	Metadata *upstream.MetadataCache
}

func NewCollector(client *http.Client, seerr, radarr, sonarr config.Service) *Collector {
	return &Collector{Client: client, Seerr: seerr, Radarr: radarr, Sonarr: sonarr, Metadata: &upstream.MetadataCache{}}
}

func (c *Collector) Collect(ctx context.Context) (model.Snapshot, error) {
	client, baseURL, apiKey := c.Client, c.Seerr.URL, c.Seerr.Key
	radarr, sonarr := c.Radarr, c.Sonarr
	metadata := c.Metadata
	if metadata == nil {
		metadata = &upstream.MetadataCache{}
	}
	// Results of this refresh's calls, not continuous service health.
	integrations := map[string]string{"seerr": model.IntegrationError, "radarr": model.IntegrationNotConfigured, "sonarr": model.IntegrationNotConfigured}
	if radarr.URL != "" {
		integrations["radarr"] = model.IntegrationUnknown
	}
	if sonarr.URL != "" {
		integrations["sonarr"] = model.IntegrationUnknown
	}
	const pageSize = 50
	const maxRequests = 1000
	all := make([]upstream.SeerrRequest, 0)
	for skip := 0; ; skip += pageSize {
		var page upstream.SeerrPage
		path := fmt.Sprintf("/api/v1/request?take=%d&skip=%d&filter=all&sort=added", pageSize, skip)
		if err := upstream.FetchSeerrJSON(ctx, client, baseURL, apiKey, path, &page); err != nil {
			log.Printf("Seerr dashboard request: %v", err)
			return model.Snapshot{Integrations: integrations}, fmt.Errorf("could not retrieve Seerr requests")
		}
		all = append(all, page.Results...)
		if len(all) > maxRequests {
			integrations["seerr"] = model.IntegrationPartial
			return model.Snapshot{Integrations: integrations}, fmt.Errorf("request limit exceeded (1000)")
		}
		if len(page.Results) == 0 || (page.PageInfo.Results > 0 && len(all) >= page.PageInfo.Results) || len(page.Results) < pageSize {
			break
		}
	}
	items := make([]model.Request, len(all))
	integrations["seerr"] = model.IntegrationSuccess
	var metadataFailed atomic.Bool
	// Metadata lookups are bounded to avoid flooding Seerr.
	sem := make(chan struct{}, 1)
	var wg sync.WaitGroup
	for i, item := range all {
		requester := strings.TrimSpace(item.RequestedBy.DisplayName)
		if requester == "" {
			requester = strings.TrimSpace(item.RequestedBy.Username)
		}
		if requester == "" {
			requester = "Unknown"
		}
		items[i] = model.Request{ID: item.ID, Title: fmt.Sprintf("TMDB #%d", item.Media.TMDBID), Type: item.Type,
			Requester: requester, RequestStatus: item.Status, MediaStatus: item.Media.Status,
			UpdatedAt: item.UpdatedAt, CreatedAt: item.CreatedAt, TMDBID: item.Media.TMDBID, TVDBID: item.Media.TVDBID,
			DownloadEntries: len(item.Media.DownloadStatus)}
		items[i].AvailabilitySource = "seerr"
		for _, season := range item.Seasons {
			if season.SeasonNumber == nil || *season.SeasonNumber < 0 {
				items[i].RequestedSeasons = nil
				break
			}
			items[i].RequestedSeasons = append(items[i].RequestedSeasons, *season.SeasonNumber)
		}
		slices.Sort(items[i].RequestedSeasons)
		items[i].RequestedSeasons = slices.Compact(items[i].RequestedSeasons)
		service := ""
		if item.Type == "movie" && radarr.URL != "" {
			service = "radarr"
		}
		if item.Type == "tv" && sonarr.URL != "" {
			service = "sonarr"
		}
		if service != "" {
			items[i].AvailabilitySource = service
			items[i].AvailabilityStale = true
			items[i].StatusOverride = model.StatusUnknown
			items[i].StatusReason = "Imported availability could not be confirmed from " + service + "."
		}
		if item.Media.TMDBID <= 0 || (item.Type != "movie" && item.Type != "tv") {
			items[i].Title = "Unknown title"
			continue
		}
		wg.Add(1)
		go func(index, tmdbID int, kind string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			details, err := metadata.Lookup(ctx, client, baseURL, apiKey, kind, tmdbID)
			if err != nil {
				metadataFailed.Store(true)
				return
			}
			title := strings.TrimSpace(details.Title)
			if kind == "tv" {
				title = strings.TrimSpace(details.Name)
			}
			if title != "" {
				items[index].Title = title
			}
			if kind == "tv" && items[index].TVDBID == 0 {
				if details.MediaInfo.TVDBID > 0 {
					items[index].TVDBID = details.MediaInfo.TVDBID
				} else {
					items[index].TVDBID = details.ExternalIDs.TVDBID
				}
			}
		}(i, item.Media.TMDBID, item.Type)
	}
	wg.Wait()
	if metadataFailed.Load() {
		integrations["seerr"] = model.IntegrationPartial
	}
	radarrIDs := make(map[int]int)
	sonarrIDs := make(map[int]int)
	// Explicit imported-file evidence takes precedence over Seerr's media status.
	if radarr.URL != "" {
		movies, err := upstream.FetchRadarrMovies(ctx, client, radarr)
		integrations["radarr"] = model.IntegrationSuccess
		if err != nil {
			integrations["radarr"] = model.IntegrationError
			log.Printf("Radarr lookup failed (availability unconfirmed): %v", err)
		} else {
			for i := range items {
				if items[i].Type != "movie" || items[i].TMDBID <= 0 {
					continue
				}
				if movie, found := movies[items[i].TMDBID]; found {
					items[i].RadarrStatus = movie.Status
					items[i].RadarrHasFile = movie.HasFile
					radarrIDs[i] = movie.ID
					items[i].StatusOverride = radarrStatusOverride(movie, items[i].MediaStatus)
					if movie.HasFile != nil {
						items[i].AvailabilityStale = false
						items[i].StatusReason = ""
						if *movie.HasFile {
							items[i].StatusReason = "Radarr confirms an imported movie file. Media-server indexing is separate."
						}
					}
				} else {
					items[i].StatusOverride = model.StatusProcessing
					if items[i].MediaStatus == 2 {
						items[i].StatusOverride = model.StatusPending
					}
					items[i].AvailabilityStale = false
					items[i].StatusReason = "No matching movie was found in Radarr; this does not prove a search or download failed."
				}
			}
		}
	}
	// Optional Sonarr enrichment. Do not infer download/import state for a whole
	// series from aggregate counts: requested seasons may differ.
	if sonarr.URL != "" {
		series, err := upstream.FetchSonarrSeries(ctx, client, sonarr)
		integrations["sonarr"] = model.IntegrationSuccess
		if err != nil {
			integrations["sonarr"] = model.IntegrationError
			log.Printf("Sonarr lookup failed (availability unconfirmed): %v", err)
		} else {
			for i := range items {
				if items[i].Type != "tv" || items[i].TVDBID <= 0 {
					continue
				}
				if show, found := series[items[i].TVDBID]; found {
					items[i].SonarrStatus = show.Status
					sonarrIDs[i] = show.ID
					status, known := sonarrStatusOverride(show, items[i].RequestedSeasons)
					items[i].StatusOverride = status
					items[i].AvailabilityStale = !known
					if known {
						items[i].StatusReason = ""
						if status == model.StatusAvailable {
							items[i].StatusReason = "Sonarr confirms files for all known episodes in the requested seasons. Media-server indexing is separate."
						}
					} else {
						items[i].StatusReason = "Requested-season availability could not be confirmed from Sonarr."
					}
				} else {
					items[i].StatusOverride = model.StatusProcessing
					if items[i].MediaStatus == 2 {
						items[i].StatusOverride = model.StatusPending
					}
					items[i].AvailabilityStale = false
					items[i].StatusReason = "No matching series was found in Sonarr; this does not prove a search or download failed."
				}
			}
		}
	}
	// Queue/history failures do not invalidate a successful imported-file lookup.
	if radarr.URL != "" && len(radarrIDs) > 0 {
		queue, qe := upstream.FetchArrPages[upstream.QueueRecord](ctx, client, radarr.URL, radarr.Key, "queue", 500)
		history, he := upstream.FetchArrPages[upstream.HistoryRecord](ctx, client, radarr.URL, radarr.Key, "history", 500)
		if qe != nil || he != nil {
			integrations["radarr"] = model.IntegrationPartial
		}
		if qe != nil {
			log.Printf("Radarr queue: %v", qe)
		}
		if he != nil {
			log.Printf("Radarr history: %v", he)
		}
		if qe == nil || he == nil {
			applyActivity(items, radarrIDs, queue, history, "radarr")
		}
	}
	if sonarr.URL != "" && len(sonarrIDs) > 0 {
		queue, qe := upstream.FetchArrPages[upstream.QueueRecord](ctx, client, sonarr.URL, sonarr.Key, "queue", 500)
		history, he := upstream.FetchArrPages[upstream.HistoryRecord](ctx, client, sonarr.URL, sonarr.Key, "history", 500)
		if qe != nil || he != nil {
			integrations["sonarr"] = model.IntegrationPartial
		}
		if qe != nil {
			log.Printf("Sonarr queue: %v", qe)
		}
		if he != nil {
			log.Printf("Sonarr history: %v", he)
		}
		if qe == nil || he == nil {
			applyActivity(items, sonarrIDs, queue, history, "sonarr")
		}
	}
	return model.Snapshot{Requests: items, Total: len(items), Integrations: integrations}, nil
}
