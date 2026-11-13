package dashboard

import (
	"reqarr/model"
	"reqarr/upstream"
)

func radarrStatusOverride(movie upstream.Movie, mediaStatus int) string {
	// Only refine processing requests, and require explicit file/release data.
	// Radarr's release eligibility is not library availability.
	if mediaStatus != 3 || movie.HasFile == nil || *movie.HasFile || movie.IsAvailable == nil {
		return ""
	}
	if *movie.IsAvailable {
		return model.StatusMissing
	}
	return model.StatusAwaitingRelease
}
