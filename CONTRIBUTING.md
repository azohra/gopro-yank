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
- Local deletion validates every manifest path before removing anything. It
  preserves unrelated files and the selected archive folder.
- Credentials remain local. Credential files use mode `0600`, and secret values
  stay out of logs, fixtures, archive output, and commits.
- Completion covers downloadable originals; unavailable media remains visible.

Route archive-relative paths through `secureJoin`, and use `atomicWrite` for
archive records and credentials. Extend the safety tests whenever filesystem
behavior changes. Use `t.TempDir()` for filesystem tests and `httptest.Server`
for network tests. Test failure paths when a partial operation could affect user
data.

## Validation

```sh
make fmt
make check
make build
make snapshot
make release VERSION=1.2.3
```

`make check` is required. `make snapshot` builds release artifacts without
publishing them; run it when packaging, dependencies, or platforms change. If
the source package's top-level contents change, update the `git archive`
allowlist in `scripts/build-release.sh`. Review website changes locally at
desktop and phone widths.

Keep the README consumer-focused and update `docs/brand.md` only for shared
voice or visual rules. `mise.toml` owns executable CI and publishing behavior.
Workflows own triggers, permissions, credentials, and runners.
