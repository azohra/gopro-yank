# Contributing

GoPro Yank is one Go program with a terminal interface, a scriptable CLI, and a
small static website. Go 1.25 or newer is the only development requirement.
Python, Node.js, and CGO are not part of the build.

## Start here

```sh
go mod download
make check
make build
./gopro-yank --demo
```

The demo uses sample data and does not need a GoPro account. Tests use local
fixtures and test servers; never point them at a live GoPro account.

## Where changes belong

| Area | Location |
|---|---|
| Program entry and version | `cmd/gopro-yank/` |
| Shared application workflows | `internal/app/operations.go` |
| Interactive interface | `internal/app/tui.go`, `tui_view.go` |
| Supported CLI | `internal/app/cli.go` |
| Historical CLI compatibility | `internal/app/cli_compat.go` |
| Archive, transfer, checks, and deletion | `internal/app/archive.go`, `transfer.go`, `delete.go` |
| Offline report | `internal/app/report.go` |
| Public website | `site/` |
| Release packages and Homebrew cask | `scripts/build-release.sh` |

Keep behavior in the shared workflows so the TUI and CLI agree. Keep the main
interface small; compatibility commands exist for old scripts, not as a second
product surface.

## Safety contracts

Changes must preserve these guarantees:

- Viewing a library does not write to the archive or start downloads.
- Downloads require an explicit archive confirmation.
- GoPro cloud media is never deleted.
- Local deletion removes only tracked archive files and GoPro Yank records. It
  preserves unrelated files and the selected archive folder.
- Credentials remain local and are never printed or added to archive output.
- Completion means every downloadable original in the latest snapshot is
  present and checked; unavailable media remains visible as manual work.

Tests for filesystem behavior should use `t.TempDir()`. Network tests should
use `httptest.Server`. Cover failure paths before success paths when a partial
operation could affect user data.

## Checks

```sh
make fmt       # format Go files
make check     # formatting, vet, and race-enabled tests
make build     # build the current platform executable
make snapshot  # build all release packages with version "dev"
make release VERSION=1.3.1  # build versioned packages without publishing
```

Run `make snapshot` when changing dependencies, release packaging, supported
platforms, or the source archive. Website changes should also be served from
`site/` and checked at desktop and phone widths.

## Documentation and releases

The README is for people deciding whether and how to use GoPro Yank. Keep it
short, concrete, and free of internal implementation detail. Update
`docs/brand.md` only for shared visual or voice rules.

Every branch and pull request runs `make check` and builds all six release
packages. A `vX.Y.Z` tag publishes the GitHub release. The generated
`gopro-yank.rb` is then used to update `azohra/homebrew-tools`. Cloudflare Pages
deploys `site/` from `main`.
