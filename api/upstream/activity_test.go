package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchQueueAuthAndPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret" || r.URL.Query().Get("page") != "1" {
			t.Errorf("bad request %s", r.URL)
		}
		w.Write([]byte(`{"totalRecords":1,"records":[{"movieId":42,"status":"downloading"}]}`))
	}))
	defer server.Close()
	records, err := FetchArrPages[QueueRecord](context.Background(), server.Client(), server.URL, "secret", "queue", 500)
	if err != nil || len(records) != 1 || records[0].MovieID != 42 {
		t.Fatalf("%v %+v", err, records)
	}
}
