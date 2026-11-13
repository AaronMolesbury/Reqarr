package storage

import (
	"context"
	"database/sql"
	"time"
)

type MissingObservation struct {
	Since     time.Time
	LastSeen  time.Time
	Escalated bool
}

func LoadMissing(ctx context.Context, db *sql.DB) (map[int]MissingObservation, error) {
	items := make(map[int]MissingObservation)
	rows, err := db.QueryContext(ctx, "SELECT request_id,since,last_seen,escalated FROM missing_observations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var since, last int64
		var escalated bool
		if err := rows.Scan(&id, &since, &last, &escalated); err != nil {
			return nil, err
		}
		items[id] = MissingObservation{Since: time.Unix(0, since), LastSeen: time.Unix(0, last), Escalated: escalated}
	}
	return items, rows.Err()
}

func SaveMissing(ctx context.Context, db *sql.DB, items map[int]MissingObservation) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM missing_observations"); err != nil {
		return err
	}
	for id, item := range items {
		if _, err := tx.ExecContext(ctx, "INSERT INTO missing_observations(request_id,since,last_seen,escalated) VALUES(?,?,?,?)", id, item.Since.UnixNano(), item.LastSeen.UnixNano(), item.Escalated); err != nil {
			return err
		}
	}
	return tx.Commit()
}
