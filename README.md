<div align="center">
  <img src="docs/logo.svg" width="160" alt="gopro-yank" />

  <h1>gopro-yank</h1>

  <p><strong>Bulk-download your GoPro Plus cloud library. Fast, resumable, three commands.</strong></p>

  <p>
    <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-yellow.svg"></a>
    <a href="https://www.python.org/downloads/"><img alt="Python 3.10+" src="https://img.shields.io/badge/python-3.10%2B-blue"></a>
    <a href="https://github.com/azohra/gopro-yank/releases"><img alt="Release" src="https://img.shields.io/github/v/release/azohra/gopro-yank?color=brightgreen"></a>
  </p>

  <img src="docs/demo-r2.gif" alt="gopro-yank demo" width="780" />
</div>

---

GoPro's media library caps you at **25 files per batch**, and the underlying
`/zip/source` endpoint has no HTTP Range support — one dropped connection
wastes the whole multi-gigabyte transfer. `gopro-yank` requests one file
per zip, runs them in parallel, and resumes cleanly on any failure.

Validated against a real **348-item / 510 GB** library — 100% download
success at ~100 MB/s sustained.

## Install

```bash
pipx install git+https://github.com/azohra/gopro-yank
# or
uv tool install git+https://github.com/azohra/gopro-yank
```

After install, run `gopro-yank demo` to see the TUI against simulated data.

## Back up your library

```bash
gopro-yank login    # paste cookies from your browser (uses clipboard)
gopro-yank pull     # download everything (prompts for destination)
gopro-yank verify   # confirm files match
```

`pull` is resumable — Ctrl-C anytime and re-run picks up where it left off.

## Commands

| | |
|---|---|
| `gopro-yank` | Status: creds configured? how many items done? what's next? |
| `gopro-yank login` | Capture `gp_access_token` + `gp_user_id` via clipboard; validate; save with mode 600 |
| `gopro-yank demo` | Preview the TUI against fake data — no credentials |
| `gopro-yank list` | Library summary + sample. `--all`, `--pending`, `--done`, `--json` |
| `gopro-yank pull` | Download everything. `--parallel N` tunes concurrency (default 8) |
| `gopro-yank status` | What's done vs pending; what's on disk |
| `gopro-yank verify` | Confirm on-disk files match the library; handles multi-chapter clips |
| `gopro-yank manifest` | JSON snapshot of every library item + marker state |
| `gopro-yank skip <id>` | Mark items as deliberately skipped (e.g. `MultiClipEdit` timelines) |

Every command has `-h/--help`.

## How it stays out of your way

- Files land in `out/YYYY/MM/` based on each item's `created_at` — auto-filed, no manual sorting
- One JSON marker per completed file in `~/.local/share/gopro-yank/state/` drives the resume
- Multi-chapter videos (long recordings split into `GX010xxx.MP4`, `GX020xxx.MP4`…) are handled correctly by `verify`, which sums chapter sizes against the API total
- On HTTP 401 the CLI tells you exactly what to run (`gopro-yank login`); the download picks up from where it stopped

## State and configuration

```
~/.config/gopro-yank/.env                # cookies (mode 600, gitignored)
~/.local/share/gopro-yank/state/         # per-file markers
```

Pass `--state-dir` and `--env-file` for a portable setup.

## What this does NOT do

- **Bypass GoPro's TOS.** Use on your own library, on your own account.
- **Cancel your subscription.** That's a click at <https://gopro.com/en/us/account/subscription>.
- **Preserve `MultiClipEdit` items.** Those are edit timelines, not media. Re-export them in GoPro Quik first if you want to keep them.

## Upgrading

`pipx install --force` is unreliable when pipx uses uv as its backend — it
refuses to clobber the existing venv and silently keeps the old version.

```bash
pipx reinstall gopro-yank
# or for uv:
uv tool install --reinstall git+https://github.com/azohra/gopro-yank
```

Verify with `gopro-yank --version`.

## Acknowledgements

Inspired by [itsankoff/gopro-plus](https://github.com/itsankoff/gopro-plus),
which proved the `/media/x/zip/source` endpoint works for bulk download.
This is a re-implementation focused on per-file requests, parallel downloads,
a clipboard-based login flow, and robustness for very large libraries.

## License

[MIT](LICENSE).
