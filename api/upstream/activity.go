package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type QueueRecord struct {
	ID             int     `json:"id"`
	MovieID        int     `json:"movieId"`
	SeriesID       int     `json:"seriesId"`
	Status         string  `json:"status"`
	TrackedStatus  string  `json:"trackedDownloadStatus"`
	TrackedState   string  `json:"trackedDownloadState"`
	Size           float64 `json:"size"`
	SizeLeft       float64 `json:"sizeleft"`
	StatusMessages []struct {
		Messages []string `json:"messages"`
	} `json:"statusMessages"`
	ErrorMessage string `json:"errorMessage"`
}
type HistoryRecord struct {
	MovieID   int               `json:"movieId"`
	SeriesID  int               `json:"seriesId"`
	EventType string            `json:"eventType"`
	Date      string            `json:"date"`
	Data      map[string]string `json:"data"`
}

type arrPage[T any] struct {
	TotalRecords int `json:"totalRecords"`
	Records      []T `json:"records"`
}

// Read a capped set of recent queue or history records, without storing API
// keys or exposing download paths, client names, or release names to the UI.
func FetchArrPages[T any](ctx context.Context, client *http.Client, baseURL, key, endpoint string, maxRecords int) ([]T, error) {
	const pageSize = 100
	records := make([]T, 0)
	for page := 1; len(records) < maxRecords; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		if endpoint == "queue" {
			params.Set("includeUnknownMovieItems", "false")
			params.Set("includeUnknownSeriesItems", "false")
		} else {
			params.Set("sortKey", "date")
			params.Set("sortDirection", "descending")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v3/"+endpoint+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Api-Key", key)
		req.Header.Set("Accept", "application/json")
		response, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, fmt.Errorf("%s HTTP %d", endpoint, response.StatusCode)
		}
		var result arrPage[T]
		err = json.NewDecoder(io.LimitReader(response.Body, 12<<20)).Decode(&result)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		records = append(records, result.Records...)
		if len(result.Records) == 0 || len(result.Records) < pageSize || (result.TotalRecords > 0 && len(records) >= result.TotalRecords) {
			break
		}
	}
	if len(records) > maxRecords {
		records = records[:maxRecords]
	}
	return records, nil
}
