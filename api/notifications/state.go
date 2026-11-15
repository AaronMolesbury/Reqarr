package notifications

import (
	"context"

	"reqarr/storage"
)

type incidentScope struct {
	Key        string
	CanRecover bool
}
type observationState struct {
	ctx         context.Context
	tx          *storage.StateTx
	initialized bool
	queued      int
}

func (s *observationState) get(key string) (string, bool, error) { return s.tx.Get(s.ctx, key) }
func (s *observationState) change(incident incidentScope, key, value, title string, notifyFirst bool) error {
	title, message, priority := pushContent(title)
	old, exists, err := s.get(key)
	if err != nil {
		return err
	}
	attentionKey := "attention/" + incident.Key
	active, attentionKnown, err := s.get(attentionKey)
	if err != nil {
		return err
	}
	if exists && old == value {
		// Seed unchanged legacy incidents once. Old history must not undo recovery.
		if title == attentionTitle && !attentionKnown && !incident.CanRecover {
			return s.tx.Set(s.ctx, attentionKey, "active")
		}
		return nil
	}
	if title == attentionTitle {
		if err := s.tx.Set(s.ctx, attentionKey, "active"); err != nil {
			return err
		}
	}
	if err = s.tx.Set(s.ctx, key, value); err != nil {
		return err
	}
	if incident.CanRecover && exists && positiveProgress(key, value) {
		if err := s.tx.Set(s.ctx, attentionKey, "clear"); err != nil {
			return err
		}
	}
	if title == attentionTitle && active == "active" {
		return nil
	}
	if s.initialized && title != "" && (exists || notifyFirst) {
		err = s.tx.Queue(s.ctx, title, message, priority)
		if err == nil {
			s.queued++
		}
	}
	return err
}
