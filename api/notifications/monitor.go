package notifications

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"reqarr/config"
	"reqarr/model"
)

func Run(ctx context.Context, db *sql.DB, client *http.Client, c config.Notifications, collect func(context.Context) (model.Snapshot, error)) {
	for {
		snapshot, fetchErr := collect(ctx)
		if ctx.Err() != nil {
			return
		}
		if err := Observe(ctx, db, snapshot, fetchErr, time.Now()); err != nil {
			log.Print("Notification state update failed; retrying next poll")
		} else if err := Deliver(ctx, db, client, c); err != nil {
			log.Print("Notification delivery failed; queued messages will be retried")
		}
		timer := time.NewTimer(c.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
