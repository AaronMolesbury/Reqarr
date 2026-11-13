package storage

import (
	"context"
	"database/sql"
	"errors"
)

type StateTx struct{ tx *sql.Tx }

func BeginState(ctx context.Context, db *sql.DB) (*StateTx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &StateTx{tx: tx}, nil
}

func (t *StateTx) Get(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := t.tx.QueryRowContext(ctx, "SELECT value FROM notification_state WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return value, err == nil, err
}

func (t *StateTx) Set(ctx context.Context, key, value string) error {
	_, err := t.tx.ExecContext(ctx, "INSERT INTO notification_state(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return err
}

func (t *StateTx) Commit() error   { return t.tx.Commit() }
func (t *StateTx) Rollback() error { return t.tx.Rollback() }
