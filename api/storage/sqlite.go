package storage

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		PRAGMA busy_timeout = 5000;
		CREATE TABLE IF NOT EXISTS notification_state (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS missing_observations (
			request_id INTEGER PRIMARY KEY, since INTEGER NOT NULL,
			last_seen INTEGER NOT NULL, escalated INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS notification_outbox (
			id INTEGER PRIMARY KEY, title TEXT NOT NULL, message TEXT NOT NULL,
			priority INTEGER NOT NULL, sent INTEGER NOT NULL DEFAULT 0
		);
	`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
