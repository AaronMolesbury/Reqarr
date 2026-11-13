package dashboard

import (
	"context"
	"log"
	"slices"
	"sync"
	"time"

	"reqarr/model"
)

// One source is shared by every dashboard client and the notification worker.
// Successful and failed results both have a cooldown, so retries and multiple
// tabs cannot multiply upstream polling. Caller cancellation only stops waiting.
type Source struct {
	Ctx           context.Context
	Collect       func(context.Context) (model.Snapshot, error)
	Cooldown      time.Duration
	ErrorCooldown time.Duration
	mu            sync.Mutex
	running       chan struct{}
	expires       time.Time
	snapshot      model.Snapshot
	err           error
	Missing       *MissingTracker
	confirmed     map[int]model.Request
}

func (s *Source) Get(ctx context.Context) (model.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return model.Snapshot{}, err
	}
	s.mu.Lock()
	if time.Now().Before(s.expires) {
		snapshot, err := s.snapshot, s.err
		s.mu.Unlock()
		return snapshot, err
	}
	if s.running == nil {
		s.running = make(chan struct{})
		go s.poll(s.running)
	}
	done := s.running
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return model.Snapshot{}, ctx.Err()
	case <-done:
		s.mu.Lock()
		snapshot, err := s.snapshot, s.err
		s.mu.Unlock()
		return snapshot, err
	}
}

func (s *Source) poll(done chan struct{}) {
	ctx, cancel := context.WithTimeout(s.Ctx, 2*time.Minute)
	defer cancel()
	snapshot, err := s.Collect(ctx)
	if err == nil {
		snapshot.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil {
		for i := range snapshot.Requests {
			item := &snapshot.Requests[i]
			previous, found := s.confirmed[item.ID]
			if item.AvailabilityStale && found && previous.Type == item.Type && previous.TMDBID == item.TMDBID && previous.TVDBID == item.TVDBID && previous.AvailabilitySource == item.AvailabilitySource && slices.Equal(previous.RequestedSeasons, item.RequestedSeasons) {
				item.StatusOverride = previous.EffectiveStatus()
				item.StatusReason = "Showing the last confirmed status; current imported availability could not be checked."
			}
		}
	}
	if s.Missing != nil && ctx.Err() == nil {
		tracked, trackingErr := s.Missing.Apply(ctx, snapshot, err, time.Now())
		if trackingErr != nil {
			log.Print("Missing duration tracking failed; requests retain their upstream classifications")
		} else {
			snapshot = tracked
		}
	}
	if err == nil {
		next := make(map[int]model.Request)
		for _, item := range snapshot.Requests {
			if !item.AvailabilityStale {
				next[item.ID] = item
			} else if previous, found := s.confirmed[item.ID]; found {
				next[item.ID] = previous
			}
		}
		s.confirmed = next
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot, s.err = snapshot, err
	cooldown := s.Cooldown
	if err != nil && s.ErrorCooldown > cooldown {
		cooldown = s.ErrorCooldown
	}
	s.expires = time.Now().Add(cooldown)
	s.running = nil
	close(done)
}
