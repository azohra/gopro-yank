package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectLibraryIsReadOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/media/user":
			_ = json.NewEncoder(writer).Encode(map[string]any{"id": "user"})
		case "/media/search":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"_embedded": map[string]any{"media": []map[string]any{
					{"id": "video", "filename": "GX010001.MP4", "file_size": 7, "captured_at": "2025-01-02T12:00:00Z", "type": "Video"},
					{"id": "edit", "filename": "My edit", "file_size": 3, "captured_at": "2026-02-03T12:00:00Z", "type": "MultiClipEdit"},
				}},
				"_pages": map[string]any{"total_pages": 1},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	previousBase := apiBaseURL
	apiBaseURL = server.URL
	defer func() { apiBaseURL = previousBase }()
	t.Setenv("AUTH_TOKEN", "")
	t.Setenv("USER_ID", "")

	root := filepath.Join(t.TempDir(), "new-archive")
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("AUTH_TOKEN=token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inspection, err := InspectLibrary(context.Background(), root, envPath, 100)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Total != 2 || inspection.Remaining != 1 || inspection.Manual != 1 || inspection.Types["Video"] != 1 {
		t.Fatalf("unexpected inspection: %+v", inspection)
	}
	if inspection.Earliest != "2025-01-02" || inspection.Latest != "2026-02-03" {
		t.Fatalf("unexpected date range: %s to %s", inspection.Earliest, inspection.Latest)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("inspection changed the archive folder: %v", err)
	}
}
