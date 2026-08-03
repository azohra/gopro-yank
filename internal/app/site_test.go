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
		"Bring your <em>GoPro library</em> home.",
		"Nothing was downloaded.",
		"Your password never enters GoPro Yank.",
		"It never changes or deletes cloud media.",
		"MultiClipEdit timelines remain listed for manual export",
		"data-primary-download",
		"releases/latest/download/gopro-yank_darwin_arm64.tar.gz",
		"releases/latest/download/gopro-yank_windows_amd64.zip",
		"releases/latest/download/gopro-yank_linux_amd64.tar.gz",
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("site is missing %q", expected)
		}
	}
	localReference := regexp.MustCompile(`(?:href|src)="(/[^"#?]*)(?:[?#][^"]*)?"`)
	for _, page := range []string{"index.html", "404.html"} {
		pagePayload, err := os.ReadFile(filepath.Join(site, page))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(pagePayload), "styles-v17.css") {
			t.Errorf("%s does not use the current stylesheet cache key", page)
		}
		for _, match := range localReference.FindAllStringSubmatch(string(pagePayload), -1) {
			if match[1] == "/" {
				continue
			}
			if _, err := os.Stat(filepath.Join(site, filepath.FromSlash(strings.TrimPrefix(match[1], "/")))); err != nil {
				t.Errorf("%s references missing local site asset %s: %v", page, match[1], err)
			}
		}
	}
	for _, expected := range []string{
		`styles-v17.css`,
		`og:image:width" content="1200"`,
		`og:image:height" content="630"`,
		`og:image:alt`,
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("site metadata is missing %q", expected)
		}
	}
	if strings.Contains(html, "raw.githubusercontent.com") {
		t.Fatal("site should not depend on a GitHub-hosted presentation asset")
	}
	preview, err := os.Stat(filepath.Join(site, "og.png"))
	if err != nil || preview.Size() == 0 {
		t.Fatalf("site is missing its evergreen social preview: %v", err)
	}
	for _, anchor := range regexp.MustCompile(`href="#([^"]+)"`).FindAllStringSubmatch(html, -1) {
		if !strings.Contains(html, `id="`+anchor[1]+`"`) {
			t.Errorf("site links to missing anchor #%s", anchor[1])
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
