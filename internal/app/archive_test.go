package app

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testMedia(id string) MediaItem {
	raw := map[string]any{"id": id, "filename": "GX010001.MP4", "file_size": int64(6), "created_at": "2026-07-12T12:00:00Z", "captured_at": "2026-07-12T11:59:00Z", "type": "Video", "camera_model": "HERO13 Black"}
	return MediaItem{ID: id, Filename: "GX010001.MP4", FileSize: 6, CreatedAt: "2026-07-12T12:00:00Z", CapturedAt: "2026-07-12T11:59:00Z", MediaType: "Video", Raw: raw}
}

func addArchived(t *testing.T, archive *Archive, item MediaItem, relative string, content []byte) {
	t.Helper()
	path := filepath.Join(archive.Root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	digest, size, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.RecordDownload(item, []FileRecord{{Path: relative, Size: size, SHA256: digest}}, ""); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotSanitizesAndReconciles(t *testing.T) {
	archive, err := NewArchive(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	video := testMedia("video")
	edit := testMedia("edit")
	edit.MediaType = "MultiClipEdit"
	edit.Raw["type"] = "MultiClipEdit"
	first, err := archive.RecordSnapshot([]MediaItem{video, edit}, "user", map[string]map[string]any{"video": {"id": "video", "access_token": "secret", "nested": map[string]any{"session_token": "also-secret"}, "url": "https://example.test/source?signature=secret&quality=source"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.RecordSnapshot([]MediaItem{video}, "user", nil); err != nil {
		t.Fatal(err)
	}
	if archive.Data.Items["edit"].Status != "manual" || archive.Data.Items["edit"].SourcePresent == nil || *archive.Data.Items["edit"].SourcePresent {
		t.Fatal("manual item was not retained as source-removed")
	}
	file, err := os.Open(filepath.Join(archive.SnapshotsDir, first+".json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	zipper, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.NewDecoder(zipper).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(snapshot)
	if strings.Contains(string(body), "secret") {
		t.Fatalf("snapshot leaked a secret: %s", body)
	}
	if !strings.Contains(string(body), "quality=source") {
		t.Fatal("non-secret URL metadata was lost")
	}
}

func TestVerifyPersistsCorruptionState(t *testing.T) {
	archive, _ := NewArchive(t.TempDir())
	item := testMedia("changed")
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	addArchived(t, archive, item, "originals/changed.MP4", []byte("source"))
	path := filepath.Join(archive.Root, "originals", "changed.MP4")
	if err := os.WriteFile(path, []byte("broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := archive.Verify("")
	if result.OK() || result.Issues[0].Kind != "checksum" {
		t.Fatalf("expected checksum issue: %+v", result)
	}
	if err := archive.RecordVerification(result); err != nil {
		t.Fatal(err)
	}
	if !archive.NeedsDownload(item) || archive.Summary().Blockers != 1 {
		t.Fatal("corruption was not made repairable")
	}
}

func TestVerifyCanBeStopped(t *testing.T) {
	archive, _ := NewArchive(t.TempDir())
	item := testMedia("cancel")
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	addArchived(t, archive, item, "originals/cancel.MP4", []byte("source"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := archive.VerifyContext(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestVerifyRejectsEscapingPathsAndSymlinks(t *testing.T) {
	root := t.TempDir()
	archive, _ := NewArchive(root)
	item := testMedia("unsafe")
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	if err := archive.RecordDownload(item, []FileRecord{{Path: "../outside.mp4", Size: 1, SHA256: strings.Repeat("0", 64)}}, ""); err != nil {
		t.Fatal(err)
	}
	if result := archive.Verify(""); result.OK() || result.Issues[0].Kind != "path" {
		t.Fatalf("unsafe path accepted: %+v", result)
	}
	if runtime.GOOS != "windows" {
		outside := t.TempDir()
		if err := os.RemoveAll(filepath.Join(root, "originals")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "originals")); err != nil {
			t.Fatal(err)
		}
		if _, err := secureJoin(root, "originals/file.mp4"); err == nil {
			t.Fatal("escaping symlink accepted")
		}
	}
}

func TestLegacyAdoptionHashesWithoutMoving(t *testing.T) {
	root := filepath.Join(t.TempDir(), "archive")
	state := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(root, "2024", "05", "GX010001.MP4")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := map[string]any{"media_id": "legacy", "status": "ok", "filename": "GX010001.MP4", "saved": []string{"2024/05/GX010001.MP4"}}
	payload, _ := json.Marshal(marker)
	if err := os.WriteFile(filepath.Join(state, "legacy.json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	archive, _ := NewArchive(root)
	result, err := archive.AdoptLegacy(state)
	if err != nil {
		t.Fatal(err)
	}
	if result.AdoptedItems != 1 || !archive.Verify("").OK() {
		t.Fatalf("legacy adoption failed: %+v", result)
	}
	body, _ := os.ReadFile(mediaPath)
	if string(body) != "legacy" {
		t.Fatal("legacy media was moved or changed")
	}
}

func TestLoadsPortableManifestProducedByAnotherImplementation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "originals", "portable.mp4")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, _, _ := sha256File(path)
	manifest := map[string]any{
		"schema_version": 1,
		"created_at":     "2026-01-01T00:00:00Z",
		"updated_at":     "2026-01-01T00:00:00Z",
		"source":         map[string]any{"provider": "gopro-cloud"},
		"snapshots":      []any{},
		"items": map[string]any{"portable": map[string]any{
			"media_id": "portable", "status": "archived", "source_present": true,
			"files": []map[string]any{{"path": "originals/portable.mp4", "size": 4, "sha256": digest, "zip_crc32": nil}},
		}},
	}
	payload, _ := json.Marshal(manifest)
	if err := os.MkdirAll(filepath.Join(root, controlName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, controlName, "manifest.json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	archive, err := NewArchive(root)
	if err != nil {
		t.Fatal(err)
	}
	if !archive.Verify("").OK() {
		t.Fatal("portable schema did not survive implementation change")
	}
}
