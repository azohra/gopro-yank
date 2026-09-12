package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteLocalArchivePreservesUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	archive, err := NewArchive(root)
	if err != nil {
		t.Fatal(err)
	}
	item := testMedia("delete-me")
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	addArchived(t, archive, item, "originals/2026/07/delete-me/GX010001.MP4", []byte("source"))
	unrelated := filepath.Join(root, "keep-me.txt")
	if err := os.WriteFile(unrelated, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanLocalArchive(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Originals != 1 || plan.Files != 1 || plan.Bytes != 6 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	result, err := DeleteLocalArchive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedFiles != 1 || result.RemovedBytes != 6 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if body, err := os.ReadFile(unrelated); err != nil || string(body) != "mine" {
		t.Fatalf("unrelated file changed: %q, %v", body, err)
	}
	if _, err := os.Stat(archive.ControlDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive records remain: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("archive root was deleted: %v", err)
	}
}

func TestDeleteLocalArchivePreflightsEveryPath(t *testing.T) {
	root := t.TempDir()
	archive, _ := NewArchive(root)
	item := testMedia("safe")
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	addArchived(t, archive, item, "originals/safe.MP4", []byte("source"))
	archive.Data.Items["unsafe"] = &ItemRecord{
		MediaID: "unsafe",
		Status:  "archived",
		Files:   []FileRecord{{Path: "../outside.MP4", Size: 7, SHA256: strings.Repeat("0", 64)}},
	}
	if err := archive.Save(); err != nil {
		t.Fatal(err)
	}

	if _, err := DeleteLocalArchive(context.Background(), root); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("unsafe archive was accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "originals", "safe.MP4")); err != nil {
		t.Fatalf("preflight deleted a safe file before rejecting the archive: %v", err)
	}
	if _, err := os.Stat(archive.ManifestPath); err != nil {
		t.Fatalf("preflight deleted archive records: %v", err)
	}
}

func TestDeleteLocalArchiveHonorsCancellationBeforeDeleting(t *testing.T) {
	root := t.TempDir()
	archive, _ := NewArchive(root)
	item := testMedia("cancel-delete")
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	addArchived(t, archive, item, "originals/cancel.MP4", []byte("source"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := DeleteLocalArchive(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "originals", "cancel.MP4")); err != nil {
		t.Fatalf("canceled deletion removed media: %v", err)
	}
}
