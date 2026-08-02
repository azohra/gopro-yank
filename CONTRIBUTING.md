# Contributing

Use the Go version declared in [`go.mod`](go.mod). The program does not require
Python, Node.js, or CGO.

```sh
make check
go run ./cmd/gopro-yank --demo
```

The demo is offline. Tests use fixtures and local test servers; never use a real
GoPro account.

## Code map

| Area | Location |
|---|---|
| Entry point and version | [`cmd/gopro-yank/`](cmd/gopro-yank/) |
| Shared workflows | [`operations.go`](internal/app/operations.go) |
| TUI and supported CLI | [`tui.go`](internal/app/tui.go), [`tui_view.go`](internal/app/tui_view.go), [`cli.go`](internal/app/cli.go) |
| Historical CLI aliases | [`cli_compat.go`](internal/app/cli_compat.go) |
| Archive, transfer, checks, and deletion | [`archive.go`](internal/app/archive.go), [`transfer.go`](internal/app/transfer.go), [`delete.go`](internal/app/delete.go) |
| Offline report | [`report.go`](internal/app/report.go) |
| Static website | [`site/`](site/) |
| Release packaging | [`build-release.sh`](scripts/build-release.sh) |

Keep behavior in shared workflows so the TUI and CLI agree. Compatibility
commands support old scripts; they are not a second product surface.

## Safety contracts

- Library inspection does not write or download.
- Downloads require confirmation; cloud media is never deleted.
- Local deletion preserves unrelated files and the selected archive folder.
- Credentials remain local and absent from logs and archive output.
- Completion covers downloadable originals; unavailable media remains visible.

Use `t.TempDir()` for filesystem tests and `httptest.Server` for network tests.
Test failure paths when a partial operation could affect user data.

## Validation

```sh
make fmt
make check
make build
make snapshot
make release VERSION=1.2.3
```

`make check` is required. Run `make snapshot` when packaging, dependencies,
platforms, or source-archive contents change. It builds release artifacts
without publishing them. Review website changes locally at desktop and phone
widths.

Keep the README consumer-focused and update `docs/brand.md` only for shared
voice or visual rules. CI and publishing behavior belong in
[`.github/workflows/`](.github/workflows/), which is the source of truth.
