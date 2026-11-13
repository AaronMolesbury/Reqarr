package storage

import (
	"context"
	"database/sql"
	"errors"
)

type Message struct {
	ID       int64
	Title    string
	Message  string
	Priority int
}

func (t *StateTx) Queue(ctx context.Context, title, message string, priority int) error {
	_, err := t.tx.ExecContext(ctx, "INSERT INTO notification_outbox(title,message,priority) VALUES(?,?,?)", title, message, priority)
	return err
}

func NextPending(ctx context.Context, db *sql.DB) (Message, bool, error) {
	var m Message
	err := db.QueryRowContext(ctx, "SELECT id,title,message,priority FROM notification_outbox WHERE sent=0 ORDER BY id LIMIT 1").Scan(&m.ID, &m.Title, &m.Message, &m.Priority)
	if errors.Is(err, sql.ErrNoRows) {
		return m, false, nil
	}
	return m, err == nil, err
}

func MarkSent(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "UPDATE notification_outbox SET sent=1 WHERE id=?", id)
	return err
}
