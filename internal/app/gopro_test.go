package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAPIListingRetriesAndFullRecords(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/media/user":
			json.NewEncoder(w).Encode(map[string]any{"id": "user"})
		case r.URL.Path == "/media/search":
			if calls.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(429)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"_embedded": map[string]any{"media": []map[string]any{{"id": "one", "filename": "GX.MP4", "file_size": 4, "created_at": "2026-01-01T00:00:00Z", "type": "Video"}}}, "_pages": map[string]any{"total_pages": 1}})
		case r.URL.Path == "/media/one":
			json.NewEncoder(w).Encode(map[string]any{"id": "one", "extra": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewGoProClient("token", "user")
	client.BaseURL = server.URL
	items, err := client.ListAll(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || calls.Load() != 2 {
		t.Fatalf("unexpected listing: %d items, %d calls", len(items), calls.Load())
	}
	full, err := client.FullRecords(context.Background(), items, 2)
	if err != nil {
		t.Fatal(err)
	}
	if full["one"]["extra"] != true {
		t.Fatal("full record not retained")
	}
}

func TestSourceZIPErrorDoesNotExposeToken(t *testing.T) {
	secret := "secret token=="
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	client := NewGoProClient(secret, "user")
	client.BaseURL = server.URL
	_, err := client.StreamSourceZIP(context.Background(), "item", &strings.Builder{})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(safeError(err, secret), secret) || strings.Contains(safeError(err, secret), "secret+token") {
		t.Fatalf("token leaked: %v", err)
	}
}
