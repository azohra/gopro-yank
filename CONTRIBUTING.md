# Contributing

The repository contains a Go application in `app/` and a Vite website in
`site/`. Each component owns its tools and tasks through mise. The Go executable does not require Node.js or CGO at runtime. Release
coordination uses Node.js, which also supplies the website toolchain.

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

Application and website versions are independent. The Release workflow updates
one `release/next` PR after changes merge to main. Git-cliff calculates each
component's next version from its Conventional squash commits and renders the
shared [Conventional PR](https://github.com/azohra/conventional-pr) format.
The PR contains `VERSION` and `CHANGELOG.md` changes for components with new work.
Merging ordinary PRs does not publish or deploy those changes.

Git-cliff scopes history to the component directory. Website-only commits do not
bump the application; mixed commits contribute their complete record to both.
Application history in this layout starts with the directory move; earlier
release records remain on GitHub. Before v1, breaking changes increment the
minor version; from v1 onward, they increment the major. Features increment the
minor, and the shared preset uses patch bumps for other Conventional changes.
Non-Conventional commits are excluded. Notes link to the originating PR where
GitHub supplies that association.

`mise run //app:changelog` and `mise run //site:changelog` preview history; add
`-- --context` for JSON. The shared git-cliff preset follows main. From a clean,
current main checkout, `mise run release:prepare` generates the same version and
changelog changes as CI. It does not create tags or publish anything.

The release PR's Check run validates each releasing component and builds its
final distributable. Go archives embed the proposed version, and the website
bundle is built once. The resulting assets are retained for 90 days under the
Git source tree's identity. Main requires the PR to be current and checks to pass.
Refresh a stale release PR by dispatching Release on main; preparation recreates
it from current main. Do not add source changes to the release PR.

Merging the release PR runs `mise run release:publish`. It requires a retained
artifact from a successful Check run for that PR's head, with the same source
tree as the merged commit. It publishes those exact assets without rebuilding
or recalculating versions. Application tags use `v…` and own GitHub's Latest
release; website tags use `site/v…` and do not take over Latest. Both tags identify
the merged release commit. GitHub also provides source downloads.

Application assets retain the five supported platform archives, Homebrew cask
and checksums. Homebrew's scheduled updater proposes the cask change after
publication. Website publication deploys the released `site.tar.gz` using the
Wrangler configuration at its tag. A website-only release does not build or
publish application binaries.

Publication uploads to a draft before making the release visible. Rerun a failed
Release run to retry the same commit and version; existing published versions
are not overwritten, and tags targeting another commit are refused. Missing or
expired check artifacts stop publication. Refresh checks before merging an old
release PR. A deployment failure leaves the published website available for retry.

The workflow uses GitHub's built-in token. Repository settings must allow Actions
to create pull requests. It explicitly dispatches Check and Conventional PR for
the generated branch because token-created PRs do not trigger those workflows.

## Website deployment

`mise run //site:deploy -- site/v0.1.0` downloads and deploys a published website
release without building from the checkout. Add `--dry-run` to validate the
release without deploying it. Omit the tag to select the latest website tag
reachable from the checkout. GitHub and Cloudflare credentials come from the
environment.

The Deploy website workflow accepts a published tag for retry or rollback.
Draft releases cannot be deployed. Website source changes can accumulate on
main until the release PR is merged.
