package app

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
		"Download your <em>GoPro Cloud library.</em>",
		"Nothing was downloaded.",
		"curl -fsSL https://gopro-yank.azohra.com/install.sh | sh",
		"brew install --cask azohra/tools/gopro-yank",
		"Read the install script",
		"Manual downloads",
		"gopro-yank --demo",
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
		if !strings.Contains(string(pagePayload), "styles-v19.css") {
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
		`styles-v19.css`,
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

func TestUnixInstallerVerifiesAndInstallsRelease(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the shell installer supports macOS and Linux")
	}

	arch := map[string]string{"arm64": "arm64", "amd64": "amd64"}[runtime.GOARCH]
	if arch == "" {
		t.Skipf("no release fixture for %s", runtime.GOARCH)
	}
	osName := map[string]string{"darwin": "darwin", "linux": "linux"}[runtime.GOOS]
	asset := fmt.Sprintf("gopro-yank_%s_%s.tar.gz", osName, arch)
	release := t.TempDir()
	archivePath := filepath.Join(release, asset)

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	binary := []byte("#!/bin/sh\nprintf 'installed\\n'\n")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "gopro-yank", Mode: 0o755, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256(payload)
	if err := os.WriteFile(filepath.Join(release, "checksums.txt"), []byte(fmt.Sprintf("%x  %s\n", checksum, asset)), 0o644); err != nil {
		t.Fatal(err)
	}

	installDir := filepath.Join(t.TempDir(), "bin")
	installer := filepath.Join("..", "..", "site", "install.sh")
	command := exec.Command("sh", installer)
	command.Env = append(os.Environ(),
		"GOPRO_YANK_RELEASE_URL=file://"+release,
		"GOPRO_YANK_INSTALL_DIR="+installDir,
		"GOPRO_YANK_NO_PATH_UPDATE=1",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("installer failed: %v\n%s", err, output)
	}

	installed := filepath.Join(installDir, "gopro-yank")
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("installer did not create the executable: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("installed file is not executable: %v", info.Mode())
	}
	output, err := exec.Command(installed).CombinedOutput()
	if err != nil || string(output) != "installed\n" {
		t.Fatalf("installed executable did not run: %v\n%s", err, output)
	}
}
