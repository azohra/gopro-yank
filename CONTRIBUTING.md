# Contributing

The repository contains a Go application in `app/` and a Vite website in
`site/`. Each component owns its tools and tasks through mise. The application
does not require Node.js or CGO; the website does not require Go.

```sh
mise run check
mise run //app:dev
mise run //site:dev
```

The demo is offline. Tests use fixtures and local test servers; never use a real
GoPro account.

## Code map

| Area | Location |
|---|---|
| Entry point and version | [`cmd/gopro-yank/`](app/cmd/gopro-yank/) |
| Shared workflows | [`operations.go`](app/internal/app/operations.go) |
| TUI and supported CLI | [`tui.go`](app/internal/app/tui.go), [`tui_view.go`](app/internal/app/tui_view.go), [`cli.go`](app/internal/app/cli.go) |
| Historical CLI aliases | [`cli_compat.go`](app/internal/app/cli_compat.go) |
| Archive, transfer, checks, and deletion | [`archive.go`](app/internal/app/archive.go), [`transfer.go`](app/internal/app/transfer.go), [`delete.go`](app/internal/app/delete.go) |
| Offline report | [`report.go`](app/internal/app/report.go) |
| Static website | [`site/`](site/) |
| Release packaging | [`build-dist.sh`](app/scripts/build-dist.sh) |

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
mise run //app:format
mise run check
mise run //app:build:dist
```

To check only affected components, run
`mise run --affected --affected-base origin/main '//...:check'`. Add
`--affected-explain --dry-run` to inspect the selection.

`mise run //app:build:dist` builds development archives in `app/release/`
without publishing them. Run it when packaging, dependencies or platforms change.

`mise run //site:dev` starts Vite with live reload. `mise run //site:build`
builds `site/dist/`, including generated CSS and JavaScript filenames. Files in
`site/public/` are copied unchanged. The website check runs browser tests against
the built site served by local Wrangler, plus installation tests using local
release fixtures. It installs the Chromium browser used by those tests.

Keep the README consumer-focused and update `docs/brand.md` only for shared
voice or visual rules. `mise.toml` owns executable CI and publishing behavior.
Workflows own triggers, permissions, credentials, and runners.

## Publishing a release

See [Conventional PR](https://github.com/azohra/conventional-pr) for the change-record
format and shared presentation. `mise run //app:changelog -- --json` exports structured
history; `app/mise.toml` follows the shared preset on main.

Run `mise run //app:changelog` to see application changes. Git-cliff scopes
history to `app/` from the task’s working directory. This view starts with the
directory move; earlier records remain in Git history and published GitHub releases.
Conventional squash commits touching the application determine its next version;
website-only commits do not. A mixed commit contributes its complete change
record to the application release. Before v1.0.0,
breaking changes increment the minor version; from v1.0.0 onward, they increment
the major. Features increment the minor, and other Conventional changes
increment the patch. Non-Conventional commits are excluded.
Notes link to the originating PR, falling back to the commit when no PR exists.
Set `GITHUB_TOKEN` for authenticated GitHub access when rendering notes; the shared
preset is fetched for every invocation, including version calculation.

From a clean checkout of current main, run `mise run //app:release`, or dispatch the
Release workflow on main. The command calculates the version once, builds the five
platform archives, Homebrew cask and checksums, then creates the
tag and GitHub Release with those assets and release notes. Source versions are
not edited. Merging a PR does not publish a release.

Main requires passing PR checks against the current base before merging. The
Check workflow runs on pull requests or manual dispatch, without repeating after
merge.

PR checks use mise affected selection for project checks and application
packaging. Website-only changes do not build application archives. Packaging uses `dev`
unless `RELEASE_VERSION` is supplied by the release task. It does not calculate
versions from branch commits. GitHub provides source downloads for each tag.
The archive names and layout remain the installer and Homebrew contract.

Publication does not overwrite an existing release. If an upload is interrupted,
inspect the draft and use GitHub CLI to upload missing assets and publish it.
Do not move a published tag. Homebrew's scheduled updater proposes the cask change
after publication. Website deployment remains independent of releases; its
installers download the latest published assets.

Website deployment uses mise affected selection for each push to main. Manual
dispatch on main deploys current main regardless of which files changed. If a
deployment fails or is superseded, dispatch it again on main.

For a local deployment, run `mise run //site:deploy` with Cloudflare credentials
in the environment. The task builds the checked-out website and passes any
arguments to Wrangler; `mise run //site:deploy -- --dry-run` validates without
publishing. Automatic deployments use the workflow’s checked-out revision.
