package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestStaticSiteIsSelfContained(t *testing.T) {
	site := filepath.Join("..", "..", "site")
	payload, err := os.ReadFile(filepath.Join(site, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(payload)
	for _, expected := range []string{
		"Bring your GoPro library home.",
		"Nothing downloads.",
		"never deletes cloud media",
		"brew install --cask azohra/tools/gopro-yank",
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("site is missing %q", expected)
		}
	}
	localReference := regexp.MustCompile(`(?:href|src)="(/[^"#?]*)(?:[?#][^"]*)?"`)
	for _, match := range localReference.FindAllStringSubmatch(html, -1) {
		if match[1] == "/" {
			continue
		}
		if _, err := os.Stat(filepath.Join(site, filepath.FromSlash(strings.TrimPrefix(match[1], "/")))); err != nil {
			t.Errorf("missing local site asset %s: %v", match[1], err)
		}
	}
	headers, err := os.ReadFile(filepath.Join(site, "_headers"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(headers), "Content-Security-Policy:") || !strings.Contains(string(headers), "frame-ancestors 'none'") {
		t.Fatal("site security headers are incomplete")
	}
}
