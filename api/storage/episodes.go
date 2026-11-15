package storage

import (
	"context"
	"database/sql"
	"time"
)

type EpisodeKey struct{ RequestID, SeriesID, EpisodeID int }
type EpisodeObservation struct {
	MissingObservation
	SeasonNumber, EpisodeNumber int
}

func LoadEpisodeMissing(ctx context.Context, db *sql.DB) (map[EpisodeKey]EpisodeObservation, error) {
	items := make(map[EpisodeKey]EpisodeObservation)
	rows, err := db.QueryContext(ctx, "SELECT request_id,series_id,episode_id,season_number,episode_number,since,last_seen,escalated FROM episode_missing_observations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key EpisodeKey
		var item EpisodeObservation
		var since, last int64
		if err := rows.Scan(&key.RequestID, &key.SeriesID, &key.EpisodeID, &item.SeasonNumber, &item.EpisodeNumber, &since, &last, &item.Escalated); err != nil {
			return nil, err
		}
		item.Since, item.LastSeen = time.Unix(0, since), time.Unix(0, last)
		items[key] = item
	}
	return items, rows.Err()
}

func SaveEpisodeMissing(ctx context.Context, db *sql.DB, items map[EpisodeKey]EpisodeObservation) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM episode_missing_observations"); err != nil {
		return err
	}
	for key, item := range items {
		if _, err := tx.ExecContext(ctx, "INSERT INTO episode_missing_observations(request_id,series_id,episode_id,season_number,episode_number,since,last_seen,escalated) VALUES(?,?,?,?,?,?,?,?)", key.RequestID, key.SeriesID, key.EpisodeID, item.SeasonNumber, item.EpisodeNumber, item.Since.UnixNano(), item.LastSeen.UnixNano(), item.Escalated); err != nil {
			return err
		}
	}
	return tx.Commit()
}
