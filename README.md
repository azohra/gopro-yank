<div align="center">
  <img src="https://raw.githubusercontent.com/azohra/gopro-yank/main/docs/logo.svg" width="160" alt="gopro-yank" />
  <h1>gopro-yank</h1>
  <p><strong>Export every GoPro original. Preserve its record. Prove what made it home.</strong></p>
  <p>
    <a href="https://github.com/azohra/gopro-yank/blob/main/LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-yellow.svg"></a>
    <a href="https://github.com/azohra/gopro-yank/releases"><img alt="Release" src="https://img.shields.io/github/v/release/azohra/gopro-yank?color=brightgreen"></a>
    <img alt="Pure Go" src="https://img.shields.io/badge/pure_Go-no_runtime-00ADD8">
  </p>
</div>

GoPro Yank turns a GoPro cloud library into a portable, self-verifying archive.
It downloads one source ZIP per media item, commits only validated originals,
and records a SHA-256 for every file. Runs are parallel, interruptible, and
resumable.

The archive carries its own source snapshots, canonical manifest, checksum
file, and offline report. Credentials never enter it.

## Install

Download the archive for your computer from
[Releases](https://github.com/azohra/gopro-yank/releases). Each contains one
native executable and the license; no Python or installer is required.

| System | Asset |
|---|---|
| macOS Apple Silicon | `gopro-yank_darwin_arm64.tar.gz` |
| macOS Intel | `gopro-yank_darwin_amd64.tar.gz` |
| Windows x64 / ARM64 | `gopro-yank_windows_amd64.zip` / `gopro-yank_windows_arm64.zip` |
| Linux x64 / ARM64 | `gopro-yank_linux_amd64.tar.gz` / `gopro-yank_linux_arm64.tar.gz` |

Verify the asset against `checksums.txt`, put the executable on `PATH`, then
run `gopro-yank demo`.

Homebrew builds from the same release source:

```sh
brew tap azohra/gopro-yank https://github.com/azohra/gopro-yank
brew install gopro-yank
```

Use `brew install --HEAD gopro-yank` for the current `main` branch.

## Export

```sh
gopro-yank login
gopro-yank pull --out ~/Pictures/GoPro
gopro-yank verify --out ~/Pictures/GoPro
```

`pull` captures the source inventory and full non-secret metadata, checks disk
space, downloads and extracts atomically, hashes every original, verifies the
result, and writes the report. Rerunning fetches only missing, failed, or
damaged items.

```text
GoPro/
├── originals/2026/07/15/id-<encoded-media-id>/...
└── .gopro-yank/
    ├── manifest.json
    ├── checksums.sha256
    ├── report.html
    ├── recovery/
    ├── snapshots/
    └── staging/
```

Paths are collision-safe across common filesystems, and ZIP members cannot
escape the archive root. Source secrets and secret-bearing URL parameters are
redacted before persistence.

## Verify and repair

The default verification is fully offline and reads every recorded byte:

```sh
gopro-yank verify --out ~/Pictures/GoPro
```

Refresh the cloud inventory first, or verify a transported copy against the
primary manifest:

```sh
gopro-yank verify --source --out ~/Pictures/GoPro
gopro-yank verify --out ~/Pictures/GoPro --replica /Volumes/Archive-2/GoPro
```

Cloud deletions never delete local media. If an item is damaged, the next
`pull` fetches it again and retains the prior directory under `recovery/`.

`DOWNLOADABLE MEDIA EXPORT COMPLETE` means every automatically downloadable
item in the latest snapshot has a local file record and no archive blocker. It
does not claim equivalence to an unavailable camera-side checksum or assess
other subscription benefits. `MultiClipEdit` timelines require manual export.

## Commands

| Command | Purpose |
|---|---|
| `login` | Capture, validate, and protect GoPro cookies |
| `pull` | Snapshot, plan, ingest, verify, and report |
| `verify` | Audit locally, reconcile the source, or check a replica |
| `list` | Inspect the manifest offline |
| `status` | Show the archive verdict |
| `manifest` | Print or copy the canonical manifest |
| `report` | Regenerate or open the offline report |
| `skip` | Classify an item for manual handling |
| `demo` | Exercise the CLI without credentials |

Run `gopro-yank <command> --help` for options.

## Existing archives

The first `pull` can adopt Python v0 markers from
`~/.local/share/gopro-yank/state/`. It hashes unambiguous existing files in
place and leaves the markers untouched. Missing, unsafe, or multiply claimed
paths are reported for attention.

## Development and releases

```sh
make fmt
make check
make build
make release VERSION=1.0.0
```

The pure-Go release build cross-compiles macOS, Windows, and Linux for ARM64
and AMD64. GitHub checks every branch and pull request, and tagged releases use
the same local script. The Go v1 port should complete a live-library validation
before its first release is published.

Use GoPro Yank only with your own account. It relies on undocumented GoPro
cloud endpoints that may change. No command deletes cloud or archived media.
macOS notarization and Windows Authenticode signing remain release-identity
work.

Inspired by [itsankoff/gopro-plus](https://github.com/itsankoff/gopro-plus).
Licensed under [MIT](https://github.com/azohra/gopro-yank/blob/main/LICENSE).
