<div align="center">
  <img src="docs/hero.svg" width="100%" alt="GoPro Yank — every original, downloaded and verified" />
  <p>
    <a href="https://github.com/azohra/gopro-yank/releases"><img alt="Release" src="https://img.shields.io/github/v/release/azohra/gopro-yank?color=58E0B4"></a>
    <a href="https://github.com/azohra/gopro-yank/blob/main/LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-F4F0E8"></a>
    <img alt="Pure Go" src="https://img.shields.io/badge/runtime-none-FF5C35">
  </p>
</div>

# GoPro Yank

**Bring your GoPro library home—and know nothing is missing.**

GoPro Yank downloads every original in your GoPro cloud library into one
portable archive, then verifies every file. It is a single native app: no
Python, no virtual environment, no account data inside the archive.

It does not stop at “download finished.” GoPro Yank checks that every file
arrived intact and records the result, so the archive can be checked again
later—even without a GoPro account or internet connection.

![GoPro Yank terminal demo](docs/demo.gif)

## Get GoPro Yank

Download the asset for your computer from
[Releases](https://github.com/azohra/gopro-yank/releases):

| Computer | Asset |
|---|---|
| Apple Silicon Mac | `gopro-yank_darwin_arm64.tar.gz` |
| Intel Mac | `gopro-yank_darwin_amd64.tar.gz` |
| Windows x64 / ARM64 | `gopro-yank_windows_amd64.zip` / `gopro-yank_windows_arm64.zip` |
| Linux x64 / ARM64 | `gopro-yank_linux_amd64.tar.gz` / `gopro-yank_linux_arm64.tar.gz` |

Check the download against `checksums.txt`, put the executable on `PATH`, and
run `gopro-yank demo`.

Or let Homebrew download, build, and install it:

```sh
brew install azohra/tools/gopro-yank
```

## Download your library

```sh
gopro-yank login
gopro-yank pull --out ~/Pictures/GoPro
gopro-yank verify --out ~/Pictures/GoPro
```

`pull` is the whole job: list your GoPro media, check that there is enough disk
space, download several originals at once, confirm each one arrived intact,
and write a report.

Interrupt it whenever you need to. The next run fetches only missing, failed,
or damaged items.

## What the archive contains

```text
GoPro/
├── originals/2026/07/15/id-<encoded-media-id>/...
└── .gopro-yank/
    ├── manifest.json        complete archive index
    ├── checksums.sha256     fingerprints used to check files
    ├── report.html          readable verification report
    ├── snapshots/           saved GoPro library lists
    ├── recovery/            prior files kept during repair
    └── staging/             unfinished downloads
```

File names are made safe for macOS, Windows, and Linux. ZIP contents are blocked
from writing outside the archive. Login cookies and other secret values are
removed from saved GoPro records.

## Verify, repair, or copy

Verification works without contacting GoPro and reads every archived file:

```sh
gopro-yank verify --out ~/Pictures/GoPro
```

Compare against your current GoPro library first, or check that a copied
archive matches the original:

```sh
gopro-yank verify --source --out ~/Pictures/GoPro
gopro-yank verify --out ~/Pictures/GoPro --replica /Volumes/Archive-2/GoPro
```

Cloud deletions never delete local media. If a file is damaged, the next
`pull` replaces it only after the new copy verifies and retains the prior
directory under `recovery/`.

`DOWNLOADABLE MEDIA EXPORT COMPLETE` means every original GoPro Yank found in
the latest library list was downloaded and passed its file checks. GoPro does
not provide the original camera checksums, so the report can verify the files
from the moment they enter the archive—not their earlier camera history.
`MultiClipEdit` timelines require manual export.

## Commands

| Command | Purpose |
|---|---|
| `login` | Save and check your GoPro login cookies |
| `pull` | Download and check every original |
| `verify` | Check the archive, your GoPro library, or a copied archive |
| `list` | List archived items |
| `status` | Show completion and any problems |
| `manifest` | Print or copy the complete archive index |
| `report` | Regenerate or open the offline report |
| `skip` | Classify an item for manual handling |
| `demo` | Exercise the CLI without credentials |

Run `gopro-yank <command> --help` for options.

## From v0 to v1

The first `pull` can read download records left by Python v0 in
`~/.local/share/gopro-yank/state/`. It checks existing files where they are and
leaves the old records untouched. Missing or ambiguous files are clearly
reported for attention.

## Build it

```sh
make fmt
make check
make build
make release VERSION=1.0.0
```

The pure-Go release build cross-compiles macOS, Windows, and Linux for ARM64
and AMD64. GitHub checks branches and pull requests; version tags use the same
release script.

Use GoPro Yank only with your own account. It relies on undocumented GoPro
cloud endpoints that may change. GoPro Yank is an independent open-source
project and is not affiliated with GoPro, Inc. No command deletes cloud or
archived media.

[Brand system](docs/brand.md) ·
[MIT license](https://github.com/azohra/gopro-yank/blob/main/LICENSE) ·
Inspired by [itsankoff/gopro-plus](https://github.com/itsankoff/gopro-plus)
