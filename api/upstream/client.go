package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"reqarr/config"
)

func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func NewRequest(ctx context.Context, service config.Service, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, service.URL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", service.Key)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func FetchJSON(ctx context.Context, client *http.Client, service config.Service, path, name string, limit int64, target any) error {
	req, err := NewRequest(ctx, service, path)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s HTTP %d", name, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, limit)).Decode(target)
}
