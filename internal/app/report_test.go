package app

import (
	"os"
	"strings"
	"testing"
)

func TestReportIsSelfContained(t *testing.T) {
	archive, _ := NewArchive(t.TempDir())
	item := testMedia("edit")
	item.MediaType = "MultiClipEdit"
	if _, err := archive.RecordSnapshot([]MediaItem{item}, "user", nil); err != nil {
		t.Fatal(err)
	}
	path, err := renderReport(archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	if !strings.Contains(body, "DOWNLOADABLE MEDIA EXPORT COMPLETE") || !strings.Contains(body, "MultiClipEdit") {
		t.Fatal("report omitted archive state")
	}
	if strings.Contains(body, "http://") || strings.Contains(body, "https://") {
		t.Fatal("report has remote assets")
	}
}
