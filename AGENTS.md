# Working on GoPro Yank

These instructions apply to the whole repository. Read [README.md](README.md)
for the product contract and [CONTRIBUTING.md](CONTRIBUTING.md) for the code map
and development workflow.

## Guardrails

Treat the [safety contracts](CONTRIBUTING.md#safety-contracts) as product
requirements. In addition:

- Local deletion is manifest-scoped. Validate every recorded path before the
  first removal; preserve the archive root and unrelated files.
- Credentials stay local with mode `0600` and never appear in output, fixtures,
  reports, or commits.
- Tests never call live GoPro services or require a real account.

## Make changes coherently

- Put shared behavior in
  [`internal/app/operations.go`](internal/app/operations.go); the TUI and
  supported CLI should call the same workflows.
- Keep historical commands isolated in
  [`internal/app/cli_compat.go`](internal/app/cli_compat.go) and out of primary
  guidance.
- Use `secureJoin` and atomic writes for archive paths and records. Extend safety
  tests whenever filesystem behavior changes.
- Keep OS-specific behavior in platform files.
- Prefer small, complete changes over new layers, modes, helpers, or dependencies.
- Keep output plain and specific: what happened, what remains, and what to do next.
- Keep user guidance in the README, development guidance in CONTRIBUTING, and
  visual language in [`docs/brand.md`](docs/brand.md). Avoid repeating the same
  explanation.
- When [`site/styles.css`](site/styles.css) changes, update its cache key in
  [`site/index.html`](site/index.html).
- When release source contents change, update the `git archive` allowlist in
  [`scripts/build-release.sh`](scripts/build-release.sh).
- Do not edit or commit generated binaries, `release/`, `dist/`, credentials,
  virtual environments, or downloaded media.

## Validate and deliver

Run `make fmt` after Go edits and `make check` before committing. Also run:

- `go run ./cmd/gopro-yank --demo` for TUI changes.
- `make snapshot` for packaging, dependency, platform, or source-archive changes.
- A local desktop and narrow-width review for `site/` changes.

Use a branch and pull request. Do not tag a release or change Homebrew or
Cloudflare state unless explicitly asked. Leave a clean worktree and report the
checks you actually ran.
