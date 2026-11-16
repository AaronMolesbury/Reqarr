package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"reqarr/config"
	"reqarr/model"
	"reqarr/storage"
	"reqarr/upstream"
)

const filmImportedMessage = "A film was imported."
const episodeImportedMessage = "A TV episode was imported."

// ImportWatcher covers the configured libraries independently of Seerr requests.
// Content identities persist permanently so upgrades/reimports remain silent.
type ImportWatcher struct {
	Client         *http.Client
	Radarr, Sonarr config.Service
	failedUntil    map[string]time.Time
}

type importCheckpoint struct {
	Cutoff time.Time
	Cursor int64
}

func (w *ImportWatcher) Sources() map[string]bool {
	return map[string]bool{"radarr": w.Radarr.URL != "", "sonarr": w.Sonarr.URL != ""}
}

func (w *ImportWatcher) Poll(ctx context.Context, db *sql.DB, snapshot model.Snapshot, now time.Time) error {
	if w.failedUntil == nil {
		w.failedUntil = make(map[string]time.Time)
	}
	var failures []error
	for _, entry := range []struct {
		name    string
		service config.Service
	}{{"radarr", w.Radarr}, {"sonarr", w.Sonarr}} {
		if entry.service.URL == "" || now.Before(w.failedUntil[entry.name]) {
			continue
		}
		if err := w.pollSource(ctx, db, entry.name, entry.service, snapshot.ImportCatalogs, now); err != nil {
			w.failedUntil[entry.name] = now.Add(90 * time.Second)
			// Upstream errors can contain URLs; expose only the service category.
			failures = append(failures, fmt.Errorf("%s import monitoring failed; retrying after cooldown", entry.name))
		}
	}
	return errors.Join(failures...)
}

func (w *ImportWatcher) pollSource(ctx context.Context, db *sql.DB, source string, service config.Service, catalogs map[string]model.ImportCatalog, now time.Time) error {
	checkpointKey := "import-baseline/" + source
	raw, initialized, err := storage.GetState(ctx, db, checkpointKey)
	if err != nil {
		return err
	}
	checkpoint := importCheckpoint{Cutoff: now}
	if initialized {
		if err := json.Unmarshal([]byte(raw), &checkpoint); err != nil {
			return err
		}
		if checkpoint.Cutoff.IsZero() {
			return errors.New("invalid import baseline")
		}
	}
	catalog, found := catalogs[source]
	if found && !catalog.Known {
		return errors.New("library catalog unavailable")
	}
	if !found {
		if source == "radarr" {
			movies, err := upstream.FetchRadarrMovies(ctx, w.Client, service)
			if err != nil {
				return err
			}
			catalog = upstream.MovieImportCatalog(movies)
		} else {
			series, err := upstream.FetchSonarrSeries(ctx, w.Client, service)
			if err != nil {
				return err
			}
			catalog = upstream.SeriesImportCatalog(series)
		}
	}
	if catalog.HistoryAttempted && !catalog.HistoryKnown {
		return errors.New("import history unavailable")
	}
	if !catalog.HistoryKnown {
		history, err := upstream.FetchArrPages[upstream.HistoryRecord](ctx, w.Client, service.URL, service.Key, "history", 500)
		if err != nil {
			return err
		}
		catalog.History = upstream.ImportHistory(history)
	}
	movies := make(map[int]model.ImportMovie)
	series := make(map[int]model.ImportSeries)
	refs := make(map[string]string)
	imported := make(map[string]bool)
	episodesBySeries := make(map[int][]upstream.Episode)
	loadEpisodes := func(show model.ImportSeries) ([]upstream.Episode, error) {
		if episodes, ok := episodesBySeries[show.ID]; ok {
			return episodes, nil
		}
		episodes, err := upstream.FetchSonarrEpisodes(ctx, w.Client, service, show.ID)
		if err != nil {
			return nil, err
		}
		episodesBySeries[show.ID] = episodes
		for _, episode := range episodes {
			if episode.ID <= 0 || episode.SeriesID != show.ID || episode.SeasonNumber == nil || *episode.SeasonNumber < 0 || episode.EpisodeNumber == nil || *episode.EpisodeNumber <= 0 {
				continue
			}
			identity := fmt.Sprintf("tvdb/%d/season/%d/episode/%d", show.TVDBID, *episode.SeasonNumber, *episode.EpisodeNumber)
			refs[episodeRefKey(show, episode.ID)] = identity
		}
		return episodes, nil
	}
	for _, movie := range catalog.Movies {
		if movie.ID <= 0 || movie.TMDBID <= 0 {
			continue
		}
		movies[movie.ID] = movie
		if !initialized {
			if movie.HasFile == nil {
				return errors.New("movie file evidence unavailable for baseline")
			}
			if *movie.HasFile {
				imported[fmt.Sprintf("tmdb/%d", movie.TMDBID)] = false
			}
		}
	}
	for _, show := range catalog.Series {
		if show.ID <= 0 || show.TVDBID <= 0 {
			return errors.New("series identity unavailable")
		}
		series[show.ID] = show
		if !initialized {
			episodes, err := loadEpisodes(show)
			if err != nil {
				return err
			}
			for _, episode := range episodes {
				if episode.HasFile == nil {
					return errors.New("episode file evidence unavailable for baseline")
				}
				if *episode.HasFile {
					identity := refs[episodeRefKey(show, episode.ID)]
					if identity == "" {
						return errors.New("episode identity unavailable for baseline")
					}
					imported[identity] = false
				}
			}
		}
	}
	catalog.History = append([]model.ImportEvent(nil), catalog.History...)
	sort.Slice(catalog.History, func(i, j int) bool { return catalog.History[i].ID < catalog.History[j].ID })
	cursor := checkpoint.Cursor
	unresolved := false
	for _, event := range catalog.History {
		if event.ID > cursor {
			cursor = event.ID
		}
		if event.ID <= 0 || (initialized && event.ID <= checkpoint.Cursor) || !strings.EqualFold(event.Type, "downloadFolderImported") {
			continue
		}
		date, err := time.Parse(time.RFC3339Nano, event.Date)
		if err != nil {
			continue
		}
		identity := ""
		if source == "radarr" {
			if movie, ok := movies[event.MovieID]; ok {
				identity = fmt.Sprintf("tmdb/%d", movie.TMDBID)
			}
		} else if show, ok := series[event.SeriesID]; ok && event.EpisodeID > 0 {
			refKey := episodeRefKey(show, event.EpisodeID)
			identity = refs[refKey]
			if identity == "" {
				value, exists, err := storage.GetState(ctx, db, refKey)
				if err != nil {
					return err
				}
				if exists {
					identity = value
				} else {
					if _, err := loadEpisodes(show); err != nil {
						return err
					}
					identity = refs[refKey]
				}
			}
		}
		if identity != "" {
			notify := initialized && !date.Before(checkpoint.Cutoff)
			if _, exists := imported[identity]; !exists || !notify {
				imported[identity] = notify
			}
		} else if (source == "radarr" && event.MovieID > 0) || (source == "sonarr" && event.SeriesID > 0 && event.EpisodeID > 0) {
			// Catalogs can precede a just-added item's import in the same poll.
			unresolved = true
		}
	}
	tx, err := storage.BeginState(ctx, db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queued := 0
	for identity, notify := range imported {
		key := "imported/" + source + "/" + identity
		_, exists, err := tx.Get(ctx, key)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := tx.Set(ctx, key, "1"); err != nil {
			return err
		}
		if initialized && notify {
			message := filmImportedMessage
			if source == "sonarr" {
				message = episodeImportedMessage
			}
			if err := tx.Queue(ctx, completedTitle, message, 3); err != nil {
				return err
			}
			queued++
		}
	}
	for key, identity := range refs {
		if err := tx.Set(ctx, key, identity); err != nil {
			return err
		}
	}
	if !unresolved {
		checkpoint.Cursor = cursor
	}
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	if err := tx.Set(ctx, checkpointKey, string(encoded)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if !initialized {
		log.Printf("Import notification baseline recorded: source=%s; existing imports remain silent", source)
	} else if queued > 0 {
		log.Printf("Import notifications queued: source=%s count=%d", source, queued)
	}
	return nil
}

func episodeRefKey(show model.ImportSeries, episodeID int) string {
	return fmt.Sprintf("import-episode-ref/sonarr/%d/%d/%d", show.TVDBID, show.ID, episodeID)
}
