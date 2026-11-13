package upstream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type SeerrDetails struct {
	Title       string `json:"title"`
	Name        string `json:"name"`
	ExternalIDs struct {
		TVDBID int `json:"tvdbId"`
	} `json:"externalIds"`
	MediaInfo struct {
		TVDBID int `json:"tvdbId"`
	} `json:"mediaInfo"`
}

type metadataEntry struct {
	details SeerrDetails
	expires time.Time
}

// Only title/identifier metadata is cached here, never availability or progress.
// Cache access serializes title lookups and deduplicates repeated media IDs.
type MetadataCache struct {
	mu          sync.Mutex
	entries     map[string]metadataEntry
	failedUntil time.Time
}

func (c *MetadataCache) Lookup(ctx context.Context, client *http.Client, baseURL, key, kind string, id int) (SeerrDetails, error) {
	path := fmt.Sprintf("/api/v1/%s/%d", kind, id)
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[path]; ok && time.Now().Before(entry.expires) {
		return entry.details, nil
	}
	var details SeerrDetails
	if time.Now().Before(c.failedUntil) {
		return details, errors.New("Seerr metadata lookup cooling down after a failed request")
	}
	if err := FetchSeerrJSON(ctx, client, baseURL, key, path, &details); err != nil {
		c.failedUntil = time.Now().Add(90 * time.Second)
		return details, err
	}
	if c.entries == nil {
		c.entries = make(map[string]metadataEntry)
	}
	// A bounded cache prevents accumulation when old requests leave the list.
	if len(c.entries) >= 1000 {
		for cachedPath, entry := range c.entries {
			if !time.Now().Before(entry.expires) {
				delete(c.entries, cachedPath)
			}
		}
		if len(c.entries) >= 1000 {
			for cachedPath := range c.entries {
				delete(c.entries, cachedPath)
				break
			}
		}
	}
	c.entries[path] = metadataEntry{details: details, expires: time.Now().Add(time.Hour)}
	return details, nil
}
