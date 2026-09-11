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
| Release packaging | [`build-dist.sh`](scripts/build-dist.sh) |

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
mise run build:dist
```

`make check` is required. `mise run build:dist` builds development archives without
publishing them; run it when packaging, dependencies, or platforms change. If
the source package's top-level contents change, update the `git archive`
allowlist in `scripts/build-dist.sh`. Review website changes locally at
desktop and phone widths.

Keep the README consumer-focused and update `docs/brand.md` only for shared
voice or visual rules. `mise.toml` owns executable CI and publishing behavior.
Workflows own triggers, permissions, credentials, and runners.

## Publishing a release

See [Conventional PR](https://github.com/azohra/conventional-pr) for the change-record
format and shared presentation. `mise run changelog -- --json` exports structured
history; `mise.toml` follows the shared preset on main.

Run `mise run changelog` to see released and unreleased changes. Conventional
squash commits determine the next version using git-cliff. Before v1.0.0,
breaking changes increment the minor version; from v1.0.0 onward, they increment
the major. Features increment the minor, and other Conventional changes
increment the patch. Non-Conventional commits are excluded.
Notes link to the originating PR, falling back to the commit when no PR exists.
Set `GITHUB_TOKEN` for authenticated GitHub access when rendering notes; the shared
preset is fetched for every invocation, including version calculation.

From a clean checkout of current main, run `mise run release`, or dispatch the
Release workflow on main. The command calculates the version once, builds the five
platform archives, Homebrew cask and checksums, then creates the
tag and GitHub Release with those assets and release notes. Source versions are
not edited. Merging a PR does not publish a release.

Main requires passing PR checks against the current base before merging. The
Check workflow runs on pull requests or manual dispatch, without repeating after
merge.

PR checks run `mise run check` and `mise run build:dist`. Packaging uses `dev`
unless `RELEASE_VERSION` is supplied by the release task. It does not calculate
versions from branch commits. GitHub provides source downloads for each tag.
The archive names and layout remain the installer and Homebrew contract.

Publication does not overwrite an existing release. If an upload is interrupted,
inspect the draft and use GitHub CLI to upload missing assets and publish it.
Do not move a published tag. Homebrew's scheduled updater proposes the cask change
after publication. Website deployment remains independent of releases; its
installers download the latest published assets.
