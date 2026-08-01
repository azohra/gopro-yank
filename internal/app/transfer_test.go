package app

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func zipBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func zipClient(payload []byte) (*GoProClient, func()) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
	client := NewGoProClient("token", "user")
	client.BaseURL = server.URL
	return client, server.Close
}

func TestDownloadHandlesChaptersAndFilenameCollisions(t *testing.T) {
	archive, _ := NewArchive(t.TempDir())
	item := testMedia("a/b")
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	payload := zipBytes(t, map[string][]byte{"first/GX010001--part-2.MP4": []byte("one"), "second/GX010001.MP4": []byte("two"), "third/gx010001.mp4": []byte("three")})
	client, closeServer := zipClient(payload)
	defer closeServer()
	result := downloadOne(context.Background(), client, item, archive, 1)
	if result.Status != "ok" {
		t.Fatalf("download failed: %+v", result)
	}
	record := archive.Item(item.ID)
	if len(record.Files) != 3 {
		t.Fatalf("lost ZIP members: %+v", record.Files)
	}
	paths := map[string]bool{}
	for _, file := range record.Files {
		if paths[collisionKey(file.Path)] {
			t.Fatalf("filename collision: %s", file.Path)
		}
		paths[collisionKey(file.Path)] = true
	}
	if !archive.Verify("").OK() {
		t.Fatal("download did not verify")
	}
}

func TestCorruptionIsRetainedAndRepaired(t *testing.T) {
	archive, _ := NewArchive(t.TempDir())
	item := testMedia("repair")
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	payload := zipBytes(t, map[string][]byte{"GX010001.MP4": []byte("source")})
	client, closeServer := zipClient(payload)
	defer closeServer()
	if result := downloadOne(context.Background(), client, item, archive, 1); result.Status != "ok" {
		t.Fatal(result.Info)
	}
	record := archive.Item(item.ID)
	path, _ := secureJoin(archive.Root, record.Files[0].Path)
	if err := os.WriteFile(path, []byte("broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	verification := archive.Verify("")
	if err := archive.RecordVerification(verification); err != nil {
		t.Fatal(err)
	}
	if result := downloadOne(context.Background(), client, item, archive, 1); result.Status != "ok" {
		t.Fatal(result.Info)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "source" {
		t.Fatal("good copy was not installed")
	}
	record = archive.Item(item.ID)
	if len(record.ReplacedArtifacts) != 1 {
		t.Fatal("damaged copy was not retained")
	}
	recovery, _ := secureJoin(archive.Root, record.ReplacedArtifacts[0].Path)
	entries, err := os.ReadDir(recovery)
	if err != nil || len(entries) != 1 {
		t.Fatalf("bad recovery: %v", err)
	}
	old, _ := os.ReadFile(filepath.Join(recovery, entries[0].Name()))
	if string(old) != "broken" {
		t.Fatal("recovery did not retain corruption")
	}
}

func TestBadZIPNeverCommits(t *testing.T) {
	archive, _ := NewArchive(t.TempDir())
	item := testMedia("bad")
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	client, closeServer := zipClient([]byte("not zip"))
	defer closeServer()
	result := downloadOne(context.Background(), client, item, archive, 1)
	if result.Status != "fail" || archive.Item(item.ID).Status != "failed" {
		t.Fatalf("bad ZIP committed: %+v", result)
	}
}
