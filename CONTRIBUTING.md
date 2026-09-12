# Contributing

The repository contains a Go application in `app/` and a Vite website in
`site/`. Each component owns its tools and tasks through mise. The Go executable
does not require Node.js or CGO at runtime.

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
`site/public/` are copied unchanged. The website check builds the site and tests installation using local release
fixtures. Review page changes in a browser at desktop and phone widths, including
the installation instructions and copy buttons.

Keep the README consumer-focused and update `docs/brand.md` only for shared
voice or visual rules. `mise.toml` owns executable CI and publishing behavior.
Workflows own triggers, permissions, credentials, and runners.

## Releases

Application and website versions are independent.
[Release Please](https://github.com/googleapis/release-please) collects
Conventional commits that touch each component and maintains one release PR on
main. It changes the relevant `VERSION` and `CHANGELOG.md` files, so the next
version and its generated notes are visible and reviewable before a release.
Release Please's default Conventional Commit rules determine release intent:
features, fixes and breaking changes contribute to the next version. Other
changes can accumulate without opening a release. Website-only changes do not
release the application; releasable changes to both components release both.
Before v1, a breaking component change advances the minor version.

Let the release PR accumulate work on main. Once that collection is stable, add
any useful reader context inside the component release notes while keeping their
version markers and structure intact. Recheck this text after the PR updates;
automation can replace edits. The component notes in the merged PR become the
GitHub release notes. Source PR descriptions remain the detailed change record
linked from those notes. They do not require special section headings. Do not
add source changes to the release PR.

Its Check run calls `mise run release:check`. Mise validates and packages only
the components whose release files changed. Application archives embed the
proposed version; website releases contain the Vite bundle. The artifacts are
retained for 90 days. The release PR must be up to date with main and have
passing required checks before merging. Packaging and checks share mise's task
graph, so the website build runs once.

Merging the release PR makes draft GitHub Releases. `mise run release -- TAG`
verifies that the tag and checked candidate have the same source tree, uploads
the candidate's retained artifacts and publishes the release. The workflow then
deploys a published website release from its tag.
Application releases use `v…` tags and own GitHub's Latest release. Website
releases use `site/v…` tags and do not. The source tag identifies the released
main commit, and GitHub provides source downloads.

If publication fails after the release PR merges, rerun the failed publish job.
To resume separately, run the Release workflow with the existing tag, or run
`mise run release -- TAG` locally. Both use the same retained artifacts;
an already published release is left intact. Missing, expired or failed candidate
artifacts stop publication. A retry does not calculate another version or rebuild.
The manual Deploy website workflow remains available to deploy a published
website tag for recovery or rollback.

## Website deployment

`mise run //site:deploy -- site/v0.1.0` downloads and deploys a published website
release without building from the checkout. Add `--dry-run` to validate the
release without deploying it. Omit the tag to select the latest website tag
reachable from the checkout. GitHub and Cloudflare credentials come from the
environment.

The Deploy website workflow accepts a published tag for retry or rollback.
Draft releases cannot be deployed. Website source changes can accumulate on
main until the release PR is merged.
