package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDemoStartsWithReadOnlyLibrary(t *testing.T) {
	model := newTUIModel(context.Background(), "test", true)
	view := model.View().Content
	for _, expected := range []string{"GOPRO LIBRARY", "847", "Nothing was downloaded", "archive this library"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("demo view does not contain %q:\n%s", expected, view)
		}
	}
}

func TestArchiveNeedsConfirmation(t *testing.T) {
	model := newTUIModel(context.Background(), "test", true)
	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(tuiModel)
	if model.screen != screenConfirm {
		t.Fatalf("enter started an archive without confirmation: %v", model.screen)
	}
	view := model.View().Content
	for _, expected := range []string{"START ARCHIVING?", "never deletes cloud or archived media", "start archiving"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("confirmation does not contain %q:\n%s", expected, view)
		}
	}
}

func TestNormalizeArchivePathExpandsHome(t *testing.T) {
	root, err := normalizeArchivePath("~/GoPro Test")
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if root != filepath.Join(home, "GoPro Test") {
		t.Fatalf("unexpected path: %s", root)
	}
}

func TestStoppedArchiveReturnsHomeWithoutClaimingCompletion(t *testing.T) {
	model := newTUIModel(context.Background(), "test", true)
	model.screen = screenProgress
	model.busy = true
	updated, _ := model.Update(archiveMessage{err: context.Canceled})
	model = updated.(tuiModel)
	if model.screen != screenHome || model.busy || !strings.Contains(model.status, "Stopped safely") {
		t.Fatalf("unexpected stopped state: screen=%v busy=%v status=%q", model.screen, model.busy, model.status)
	}
	if strings.Contains(model.View().Content, "EXPORT COMPLETE") {
		t.Fatal("stopped archive claimed completion")
	}
}

func TestDeleteLocalArchiveRequiresExactConfirmation(t *testing.T) {
	root := t.TempDir()
	archive, _ := NewArchive(root)
	item := testMedia("delete-confirm")
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	addArchived(t, archive, item, "originals/delete-confirm.MP4", []byte("source"))

	model := newTUIModel(context.Background(), "test", false)
	model.archiveRoot = root
	model.archive = archive
	actions := model.homeActions()
	for index, action := range actions {
		if action.id == "delete" {
			model.cursor = index
			break
		}
	}
	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(tuiModel)
	if model.screen != screenDeleteConfirm {
		t.Fatalf("delete action did not open confirmation: %v", model.screen)
	}
	view := model.View().Content
	for _, expected := range []string{"DELETE LOCAL ARCHIVE?", "Recorded files", "Nothing will be deleted from GoPro", "folder itself will stay"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("delete confirmation does not contain %q:\n%s", expected, view)
		}
	}

	model.deleteInput.SetValue("delete")
	updated, command := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(tuiModel)
	if command != nil || model.busy || model.deleteInput.Err == nil {
		t.Fatal("lowercase confirmation started deletion")
	}
	if _, err := os.Stat(filepath.Join(root, "originals", "delete-confirm.MP4")); err != nil {
		t.Fatalf("failed confirmation deleted media: %v", err)
	}

	model.deleteInput.SetValue("DELETE")
	updated, command = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(tuiModel)
	if command == nil || !model.busy {
		t.Fatal("exact confirmation did not start deletion")
	}
}

func TestDemoNeverLoadsARealArchive(t *testing.T) {
	model := newTUIModel(context.Background(), "test", true)
	model.archiveRoot = t.TempDir()
	archive, _ := NewArchive(model.archiveRoot)
	if err := archive.Save(); err != nil {
		t.Fatal(err)
	}
	model.reloadArchive()
	if model.archive.Exists {
		t.Fatal("demo loaded a real archive")
	}
}
