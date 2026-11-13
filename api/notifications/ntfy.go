package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"reqarr/config"
	"reqarr/storage"
)

func Publish(ctx context.Context, client *http.Client, c config.Notifications, title, message string, priority int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/"+c.Topic, strings.NewReader(message))
	if err != nil {
		return errors.New("could not prepare ntfy message")
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", title)
	req.Header.Set("Priority", strconv.Itoa(priority))
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else {
		req.SetBasicAuth(c.Username, c.Password)
	}
	// Publishing must never forward credentials or message text to redirects.
	publisher := *client
	publisher.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := publisher.Do(req)
	if err != nil {
		return errors.New("ntfy delivery failed") // Don't log URLs, credentials or topic names.
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func Deliver(ctx context.Context, db *sql.DB, client *http.Client, c config.Notifications) error {
	// One worker drains the durable queue in order, retrying next poll on failure.
	for count := 0; count < 50; count++ {
		pending, exists, err := storage.NextPending(ctx, db)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		id, title, message, priority := pending.ID, pending.Title, pending.Message, pending.Priority
		title, message, priority = pushContent(title)
		if pending.Title == attentionTitle && pending.Message == serviceAttentionMessage {
			message = serviceAttentionMessage
		}
		if title == "" {
			// Discard obsolete progress alerts queued by earlier versions.
			if err := storage.MarkSent(ctx, db, id); err != nil {
				return err
			}
			continue
		}
		if err := Publish(ctx, client, c, title, message, priority); err != nil {
			// Publisher errors are deliberately sanitized; never log its config.
			log.Printf("Notification delivery failed: event=%d; %v; message remains queued", id, err)
			return err
		}
		if err := storage.MarkSent(ctx, db, id); err != nil {
			return err
		}
		log.Printf("Notification delivered: event=%d; accepted by ntfy", id)
	}
	return nil
}
