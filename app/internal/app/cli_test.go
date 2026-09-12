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

func TestPullBuildsPortableVerifiedArchive(t *testing.T) {
	payload := zipBytes(t, map[string][]byte{"GX010001.MP4": []byte("payload")})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/media/user":
			_ = json.NewEncoder(writer).Encode(map[string]any{"id": "user"})
		case "/media/search":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"_embedded": map[string]any{"media": []map[string]any{{"id": "cloud-item", "filename": "GX010001.MP4", "file_size": 7, "created_at": "2026-07-12T12:00:00Z", "captured_at": "2026-07-12T11:59:00Z", "type": "Video"}}},
				"_pages":    map[string]any{"total_pages": 1},
			})
		case "/media/cloud-item":
			_ = json.NewEncoder(writer).Encode(map[string]any{"id": "cloud-item", "filename": "GX010001.MP4", "file_size": 7, "created_at": "2026-07-12T12:00:00Z", "type": "Video", "full": true})
		case "/media/x/zip/source":
			_, _ = writer.Write(payload)
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
	root := filepath.Join(t.TempDir(), "GoPro")
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("AUTH_TOKEN=token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pullCommand(context.Background(), []string{"-out", root, "-env-file", envPath, "-state-dir", filepath.Join(t.TempDir(), "none"), "-parallel", "1"}); err != nil {
		t.Fatal(err)
	}
	archive, err := NewArchive(root)
	if err != nil {
		t.Fatal(err)
	}
	if !archive.Verify("").OK() || archive.Data.Items["cloud-item"].Source["full"] != true {
		t.Fatal("pull did not preserve and verify the complete item")
	}
	if _, err := os.Stat(archive.ReportPath); err != nil {
		t.Fatal("pull did not create the offline report")
	}
}
