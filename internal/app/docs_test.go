package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var guidanceFiles = []string{
	"README.md",
	"CONTRIBUTING.md",
	filepath.Join("docs", "brand.md"),
	filepath.Join("docs", "hero.svg"),
	filepath.Join("site", "index.html"),
}

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func readGuidance(t *testing.T, root, name string) string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func TestGuidanceLocalLinksExist(t *testing.T) {
	root := projectRoot(t)
	markdownLink := regexp.MustCompile(`\]\(([^)]+)\)`)
	htmlLink := regexp.MustCompile(`(?:href|src)="([^"]+)"`)

	for _, name := range guidanceFiles {
		body := readGuidance(t, root, name)
		links := markdownLink.FindAllStringSubmatch(body, -1)
		links = append(links, htmlLink.FindAllStringSubmatch(body, -1)...)
		for _, match := range links {
			target := strings.SplitN(strings.SplitN(match[1], "#", 2)[0], "?", 2)[0]
			if target == "" || strings.HasPrefix(target, "/") || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.Dir(name), filepath.FromSlash(target))); err != nil {
				t.Errorf("%s links to missing %s", name, match[1])
			}
		}
	}
}

func TestDocumentedMakeTargetsExist(t *testing.T) {
	root := projectRoot(t)
	makefile := readGuidance(t, root, "Makefile")
	targetPattern := regexp.MustCompile(`(?m)^([a-z][a-z-]*):`)
	targets := map[string]bool{}
	for _, match := range targetPattern.FindAllStringSubmatch(makefile, -1) {
		targets[match[1]] = true
	}

	commandPattern := regexp.MustCompile(`\bmake ([a-z][a-z-]*)`)
	for _, name := range guidanceFiles {
		for _, match := range commandPattern.FindAllStringSubmatch(readGuidance(t, root, name), -1) {
			if !targets[match[1]] {
				t.Errorf("%s documents missing make target %q", name, match[1])
			}
		}
	}
}

func TestGuidanceAvoidsAbsoluteArchiveClaims(t *testing.T) {
	root := projectRoot(t)
	retired := regexp.MustCompile(`(?i)\b(every original|all originals|nothing is missing)\b`)
	for _, name := range guidanceFiles {
		if claim := retired.FindString(readGuidance(t, root, name)); claim != "" {
			t.Errorf("%s contains retired claim %q", name, claim)
		}
	}
}
