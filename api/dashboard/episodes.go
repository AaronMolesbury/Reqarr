package dashboard

import (
	"context"
	"log"
	"slices"

	"reqarr/model"
	"reqarr/upstream"
)

// Episode lists are fetched serially once per relevant series in a shared poll.
func (c *Collector) collectEpisodeChecks(ctx context.Context, items []model.Request, seriesIDs map[int]int, queue []upstream.QueueRecord) bool {
	type result struct {
		episodes []upstream.Episode
		err      error
	}
	bySeries := make(map[int]result)
	ok := true
	for i := range items {
		item := &items[i]
		seriesID := seriesIDs[i]
		if item.Type != "tv" || seriesID <= 0 || len(item.RequestedSeasons) == 0 || item.EffectiveStatus() == model.StatusAvailable || (item.RequestStatus != 2 && item.RequestStatus != 5) {
			continue
		}
		fetched, found := bySeries[seriesID]
		if !found {
			fetched.episodes, fetched.err = upstream.FetchSonarrEpisodes(ctx, c.Client, c.Sonarr, seriesID)
			bySeries[seriesID] = fetched
			if fetched.err != nil {
				log.Print("Sonarr episode lookup failed; missing-episode checks remain unconfirmed")
			}
		}
		if fetched.err != nil {
			ok = false
			continue
		}
		check := &model.EpisodeCheck{SeriesID: seriesID}
		for _, episode := range fetched.episodes {
			if episode.ID <= 0 || episode.SeriesID != seriesID || episode.SeasonNumber == nil || episode.EpisodeNumber == nil || *episode.EpisodeNumber <= 0 || !slices.Contains(item.RequestedSeasons, *episode.SeasonNumber) {
				continue
			}
			check.Episodes = append(check.Episodes, model.EpisodeEvidence{
				ID: episode.ID, SeasonNumber: *episode.SeasonNumber, EpisodeNumber: *episode.EpisodeNumber,
				HasFile: episode.HasFile, AirDateUTC: episode.AirDateUTC, QueueState: episodeQueueState(queue, seriesID, episode.ID, *episode.SeasonNumber),
			})
		}
		item.EpisodeCheck = check
	}
	return ok
}

func episodeQueueState(queue []upstream.QueueRecord, seriesID, episodeID, seasonNumber int) string {
	result := ""
	for _, record := range queue {
		if record.SeriesID == 0 {
			if record.EpisodeID == 0 || record.EpisodeID == episodeID {
				result = model.StatusUnknown
			}
			continue
		}
		if record.SeriesID != seriesID {
			continue
		}
		if record.EpisodeID > 0 {
			if record.EpisodeID != episodeID {
				continue
			}
		} else {
			if record.SeasonNumber == nil || *record.SeasonNumber == seasonNumber {
				result = model.StatusUnknown
			}
			continue
		}
		state := queueState(record)
		switch state {
		case model.StatusDownloading, model.StatusAwaitingImport, model.StatusQueued:
			return state
		case "":
			result = model.StatusUnknown // Uncorrelated or unrecognised activity blocks a missing inference.
		case model.StatusNeedsAttention:
			if result == "" {
				result = state
			}
		}
	}
	return result
}
