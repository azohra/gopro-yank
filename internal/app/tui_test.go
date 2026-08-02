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
