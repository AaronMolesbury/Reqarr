package dashboard

import (
	"reqarr/model"
	"reqarr/upstream"
)

func radarrStatusOverride(movie upstream.Movie, mediaStatus int) string {
	// Only an imported file confirms availability; release eligibility does not.
	if movie.HasFile == nil {
		return model.StatusUnknown
	}
	if *movie.HasFile {
		return model.StatusAvailable
	}
	if mediaStatus == 2 {
		return model.StatusPending
	}
	if movie.IsAvailable == nil {
		return model.StatusProcessing
	}
	if *movie.IsAvailable {
		return model.StatusMissing
	}
	return model.StatusAwaitingRelease
}

func sonarrStatusOverride(show upstream.Series, requested []int) (string, bool) {
	if len(requested) == 0 {
		return model.StatusUnknown, false
	}
	seasons := make(map[int]*upstream.SeasonStatistics)
	for _, season := range show.Seasons {
		if season.SeasonNumber != nil {
			seasons[*season.SeasonNumber] = season.Statistics
		}
	}
	complete, hasFiles, known := true, false, true
	for _, number := range requested {
		stats := seasons[number]
		if stats == nil || stats.EpisodeFileCount == nil || stats.TotalEpisodeCount == nil || *stats.EpisodeFileCount < 0 || *stats.TotalEpisodeCount <= 0 || *stats.EpisodeFileCount > *stats.TotalEpisodeCount {
			known, complete = false, false
			continue
		}
		hasFiles = hasFiles || *stats.EpisodeFileCount > 0
		if *stats.EpisodeFileCount < *stats.TotalEpisodeCount {
			complete = false
		}
	}
	if complete {
		return model.StatusAvailable, true
	}
	if hasFiles {
		return model.StatusPartiallyAvailable, true
	}
	if !known {
		return model.StatusUnknown, false
	}
	if show.Status == "upcoming" {
		return model.StatusAwaitingRelease, true
	}
	return model.StatusProcessing, true
}
