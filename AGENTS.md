# Working on GoPro Yank

These instructions apply to the entire repository.

GoPro Yank is a consumer-facing Go application for bringing downloadable GoPro
cloud originals into a portable, verified local archive. Read `README.md` for
the user promise and `CONTRIBUTING.md` for the development workflow before
changing behavior.

## Product boundaries

- Opening the app and viewing the library are read-only. Downloads begin only
  after the user chooses Archive and confirms.
- GoPro Yank never deletes GoPro cloud media.
- Local deletion is manifest-scoped. Preflight every recorded path before the
  first removal; never remove the archive root or unrelated files.
- Credentials stay local, use mode `0600`, and must never appear in logs,
  errors, fixtures, reports, or commits.
- A completion verdict covers downloadable originals in the latest library
  snapshot. MultiClipEdit timelines and other unavailable media must remain
  clearly identified as manual exports.
- Tests must not call live GoPro services or depend on a real account.

## Shape of the code

- `cmd/gopro-yank/` is the thin executable and version entry point.
- `internal/app/operations.go` contains workflows shared by the TUI and CLI.
- `internal/app/tui*.go` owns the interactive experience; keep business rules
  in shared operations rather than view code.
- `internal/app/cli.go` contains the supported command line. Historical aliases
  stay isolated in `cli_compat.go` and should not return to primary guidance.
- `archive.go`, `transfer.go`, `report.go`, and `delete.go` own archive safety,
  transfer, reporting, and local deletion.
- `site/` is the framework-free Cloudflare Pages site. It deploys from `main`
  with `site` as the output directory and no build command.
- `scripts/build-release.sh` is the source of truth for release packages and
  the generated Homebrew cask.

## Change rules

- Prefer the smallest coherent change. Avoid speculative layers, duplicate
  helpers, hidden modes, and new dependencies without a concrete need.
- Keep output calm, plain, and specific: what happened, what remains, and what
  the user can do next. Do not overstate archive completeness.
- Preserve cross-platform behavior. Platform-specific file operations belong
  in the existing `_unix.go` and `_windows.go` files.
- Use `secureJoin` and atomic writes for archive paths and records. Extend the
  safety tests whenever path handling or deletion changes.
- Keep `README.md` consumer-focused. Put contributor process in
  `CONTRIBUTING.md`, visual language in `docs/brand.md`, and agent instructions
  here. Do not repeat the same explanation across all four.
- When `site/styles.css` changes, update its cache key in `site/index.html`.
- When adding source files that belong in release source packages, update the
  explicit `git archive` allowlist in `scripts/build-release.sh`.
- Do not edit or commit generated `gopro-yank`, `release/`, `dist/`, virtual
  environments, credentials, or downloaded media.

## Validate and deliver

Run `make fmt` after Go edits and `make check` before every commit. Run
`make snapshot` when release packaging, supported platforms, dependencies, or
the source-package allowlist changes. Exercise `go run ./cmd/gopro-yank --demo`
for TUI changes. Serve `site/` locally and check desktop and narrow layouts for
site changes.

Work on a branch and use a pull request. Do not tag a release, update the
Homebrew tap, or change Cloudflare configuration unless the user explicitly
asks. Leave the worktree clean and report exactly what was validated.
