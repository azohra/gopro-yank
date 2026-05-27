<div align="center">
  <img src="docs/logo.svg" width="160" alt="gopro-yank logo" />

  <h1>gopro-yank</h1>

  <p><strong>Yank your entire GoPro Plus cloud library before your subscription dies.</strong></p>

  <p>
    <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-yellow.svg"></a>
    <a href="https://www.python.org/downloads/"><img alt="Python 3.10+" src="https://img.shields.io/badge/python-3.10%2B-blue"></a>
    <a href="https://github.com/azohra/gopro-yank/releases"><img alt="Release" src="https://img.shields.io/github/v/release/azohra/gopro-yank?color=brightgreen"></a>
    <a href="https://github.com/azohra/gopro-yank/actions"><img alt="Tests" src="https://img.shields.io/badge/tests-18%20passing-success"></a>
  </p>

  <p>
    Async &middot; resumable &middot; adaptive concurrency &middot; one command to install &middot; one command to see it work
  </p>

  <img src="docs/demo.gif?v=0.2.1" alt="gopro-yank demo" width="780" />
</div>

---

## Why this exists

GoPro's media library web UI caps downloads at **25 files per batch**. The
underlying `/zip/source` API streams a fresh zip on every request — no HTTP
Range support — so a single dropped connection in a multi-gigabyte download
wastes the whole transfer.

`gopro-yank` works around both:

| | |
|---|---|
| **One file per request** | Drops only cost that file, not 25 GB of buffered zip |
| **Parallel by default** | 8 concurrent downloads out of the box; tune with `--parallel N` |
| **Resumable** | Per-file markers; Ctrl-C anytime and re-run picks up where it left off |
| **Year/month sorted** | Files land in `out/YYYY/MM/` based on each item's `created_at` |
| **HTTP/2 + async** | Built on `httpx` and `asyncio`; ~100 MB/s sustained on real runs |
| **Friendly TUI** | Rich-powered live stats, per-file progress, throughput, ETA |

Validated against a real **348-item / 510 GB** library — 100% download success, all markers verify clean.

## Install

```bash
# pipx (recommended)
pipx install git+https://github.com/azohra/gopro-yank

# or uv
uv tool install git+https://github.com/azohra/gopro-yank

# or from source
git clone https://github.com/azohra/gopro-yank && cd gopro-yank && pip install .
```

Requires Python 3.10+.

### Upgrading

`pipx install --force` doesn't work reliably when pipx uses uv as its
backend — it refuses to clobber the existing venv and silently keeps
the old version. Use one of:

```bash
pipx reinstall gopro-yank
# or
pipx uninstall gopro-yank && pipx install git+https://github.com/azohra/gopro-yank

# uv:
uv tool install --reinstall git+https://github.com/azohra/gopro-yank
```

Verify with `gopro-yank --version`.

## Try it in 10 seconds

```bash
gopro-yank demo
```

Runs the full TUI against simulated data — no credentials needed. Watch the
**concurrency** row in the header ramp up as fake downloads succeed.

## Real backup, three commands

```bash
gopro-yank login    # interactive cookie capture (uses your clipboard)
gopro-yank list     # summary of your library + sample rows
gopro-yank pull     # download everything (prompts for destination)
```

That's it. `pull` is resumable, idempotent, and writes directly into a
`YYYY/MM/` tree under whatever directory you choose.

## Commands

| Command | What it does |
|---|---|
| `gopro-yank` | Status banner: are credentials configured? how many items done? what's next? |
| `gopro-yank login` | Capture `gp_access_token` + `gp_user_id` cookies via clipboard; validate against the API; save with mode 600 |
| `gopro-yank demo` | Run the TUI against fake data — no credentials required |
| `gopro-yank list` | Library summary panel + head/tail sample. `--all`, `--pending`, `--done`, `--json` |
| `gopro-yank pull` | Download everything. Resumable. `--parallel N` tunes concurrency (default 8) |
| `gopro-yank status` | What's done vs pending; what's on disk |
| `gopro-yank verify` | For each marker, confirm the on-disk files exist and (for multi-chapter clips) sum to the API's reported size |
| `gopro-yank manifest` | JSON snapshot: every library item + marker state |
| `gopro-yank skip` | Write a "skipped" marker for one or more media IDs (e.g. `MultiClipEdit` items that aren't real media) |

Every command takes `-h/--help` for full options.

## State and storage

```
~/.config/gopro-yank/.env                 # your cookies (mode 600, gitignored)
~/.local/share/gopro-yank/state/          # per-file markers (drive resume)
  └── <media_id>.json                     # one per completed item
```

For a portable setup, pass `--state-dir ./state --env-file ./.env`.

## What this does *not* do

- **Bypass GoPro's TOS.** This downloads files you've uploaded to *your* own
  account. Use it on your own library.
- **Cancel your subscription.** Do that yourself once the backup is verified
  at <https://gopro.com/en/us/account/subscription>.
- **Preserve `MultiClipEdit` items.** Those are edit timelines, not media.
  Re-export them as videos in GoPro Quik first if you want to keep them.
- **Refresh tokens automatically.** When cookies expire, the CLI tells you
  exactly what to run (`gopro-yank login`). The download resumes from where
  it stopped.

## Acknowledgements

Inspired by [itsankoff/gopro-plus](https://github.com/itsankoff/gopro-plus),
which proved the `/media/x/zip/source` endpoint works for bulk download. This
project re-implements the approach with per-file requests, async + HTTP/2,
adaptive concurrency, a clipboard-first login flow, and a focus on robustness
for very large libraries.

## License

[MIT](LICENSE).
