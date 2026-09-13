# Contributing

The repository is a Go module at the root and a Vite website in `www/`. The Go
executable does not require Node.js or CGO at runtime.

```sh
mise run check
mise run dev
mise run www:dev
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
| Website | [`www/`](www/) |
| Release packaging | [`.goreleaser.yaml`](.goreleaser.yaml) |

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
mise run format
mise run check
mise run dist
mise run changelog
```

`mise run check` formats, vets, builds and race-tests the application, then
builds the website and tests the installer. `mise run dist` runs goreleaser as
a snapshot: every release archive, the checksums and the Homebrew cask land in
`dist/` without publishing. Run it when packaging, dependencies or platforms
change; CI runs both on every pull request.

Tasks declare their inputs, and a task whose inputs match a previous successful
run replays that result instead of running: the Go files are one partition,
`www/` the other, and `mise.toml` invalidates both. Pull requests restore the
cache that main last wrote, so a website change does not rerun the Go tests
and a Go change does not rebuild the site. Only pushes to main write it.

`mise run www:dev` starts Vite with live reload. `mise run www:build` builds
`www/dist/`, including generated CSS and JavaScript filenames. Files in
`www/public/` are copied unchanged. Review page changes in a browser at desktop
and phone widths, including the installation instructions and copy buttons.

Keep the README consumer-focused and update `docs/brand.md` only for shared
voice or visual rules. `mise.toml` owns executable CI and publishing behavior.
Workflows own triggers, permissions, credentials, and runners.

## Releases

release-drafter keeps one draft release on GitHub. Every merge to main adds the
pull request's title under Added or Fixed, from labels the Conventional title
sets on its own, and resolves the next version: a breaking title is a major,
`feat` a minor, `fix` a patch. `build`, `chore`, `ci`, `docs`, `style` and
`test` titles stay out of the draft. The draft is the answer to "what is
unreleased", and editing it is where release notes get written.

Publishing the draft creates the tag. That runs the Release workflow, which is
`mise run release`: goreleaser builds the archives for macOS, Linux and Windows
with the version from the tag, writes `checksums.txt`, attaches them to the
release, and opens a pull request in homebrew-tools with the generated cask,
using a token minted from the Bosun app. Assets appear a minute or two after
publishing.

The version lives only in the tag. Nothing in the tree changes for a release.

## Website

Every merge to main that touches `www/` deploys it: the Deploy website workflow
runs `mise run www:check` and then `mise run www:deploy`, which deploys
`www/dist` to Cloudflare tagged with the commit. The same two commands work on
a laptop. Cloudflare keeps every deployed version; to roll back, use
`wrangler rollback` or check out the earlier commit and deploy again.
