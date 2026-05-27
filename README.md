# gopro-yank

Bulk-download your entire GoPro Plus cloud library — fast, resumable, and
designed for the moment before you cancel your subscription.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Python: 3.10+](https://img.shields.io/badge/python-3.10%2B-blue)](https://www.python.org/)

## Why

GoPro's web UI caps downloads at **25 files per batch**. Their `/zip/source`
API endpoint streams a fresh zip on the fly — no HTTP Range support — so a
single connection drop in a multi-gigabyte download wastes the whole thing.

`gopro-yank` works around both:

- **One file per request**: requests a single-ID zip per media item, so a
  network blip only loses that file (not 25 GB of buffer)
- **Adaptive concurrency**: starts conservative, ramps up after sustained
  success, shrinks on failure. Auto-tunes to whatever GoPro's per-account
  throttle actually is.
- **Resumable**: per-file markers in `~/.local/share/gopro-yank/state/`.
  Ctrl-C anytime, re-run, picks up where it left off.
- **Year/month organized**: files land in `out/YYYY/MM/` based on each item's
  `created_at` (so 5 years of footage doesn't pile into one folder).
- **Async + HTTP/2**: built on `httpx` + `asyncio`, with a Rich-powered TUI
  showing live throughput, ETA, and per-file progress bars.

In a real run against a 348-item / 510 GB library, it sustained ~100 MB/s with
no failures except 3 `MultiClipEdit` items (edit timelines, not media).

## Install

```bash
pipx install gopro-yank
```

Or from source:

```bash
git clone https://github.com/azohra/gopro-yank
cd gopro-yank
pip install .
```

Requires Python 3.10+.

## Getting your credentials

`gopro-yank` reads two cookies from gopro.com:

1. Log into <https://gopro.com/media-library/> in Chrome, Firefox, or Safari.
2. Open DevTools (`Cmd+Opt+I` on Mac, `Ctrl+Shift+I` elsewhere).
3. **Application** tab → **Storage** → **Cookies** → `https://gopro.com`.
4. Copy the **Value** column for these two cookies:
   - `gp_access_token` — long JWT, starts with `eyJhbGc...`
   - `gp_user_id` — UUID like `00000000-0000-0000-0000-000000000000`

Create `~/.config/gopro-yank/.env`:

```
AUTH_TOKEN=eyJhbGc...whole-thing
USER_ID=your-user-id-uuid
```

Restrict permissions: `chmod 600 ~/.config/gopro-yank/.env`.

Cookies eventually expire (GoPro doesn't publish how long). When you see
`HTTP 401`, repeat the steps above and re-run — completed files are skipped.

## Usage

### Pull the whole library

```bash
gopro-yank pull --out ~/GoPro
```

Defaults: `--initial 4 --ceiling 16 --floor 1 --grow-after 8`. The adaptive
limiter starts at 4 concurrent downloads, increments after 8 successes in a
row, and shrinks on every failure.

For an aggressive run with a fast pipe and a forgiving GoPro:

```bash
gopro-yank pull --out ~/GoPro --initial 8 --ceiling 24 --grow-after 4
```

For a careful run on a flaky connection:

```bash
gopro-yank pull --out ~/GoPro --initial 2 --ceiling 6
```

### Other commands

```bash
gopro-yank list                       # show every item + done/pending state
gopro-yank list --json                # JSON one-per-line
gopro-yank status --out ~/GoPro       # summarize markers vs disk vs library
gopro-yank verify --out ~/GoPro       # check every marker matches a real file
gopro-yank manifest --out-file m.json # JSON snapshot of library + markers
gopro-yank skip <media_id> ...        # mark items as deliberately skipped
```

## How the adaptive limiter works

```text
                              ┌────────────────────────┐
                              │  AdaptiveLimiter       │
   acquire ─────────────►     │  inflight: 5/8 target  │
                              │                        │
                              │  on success:           │
   release(success=True) ───► │    streak += 1         │
                              │    if streak >= 8:     │
                              │      target += 1       │  ← ramp up
                              │                        │
                              │  on failure:           │
   release(success=False) ──► │    target -= 1         │  ← back off
                              │    streak = 0          │
                              └────────────────────────┘
                                    floor ≤ target ≤ ceiling
```

The header panel in the TUI shows live target, inflight, and success streak —
so you can watch ramp-up happen in real time. Tune `--ceiling` to your pipe.

## State and storage

```
~/.config/gopro-yank/.env                 # your cookies (gitignored)
~/.local/share/gopro-yank/state/          # per-file markers
  └── <media_id>.json                     # one per completed item
```

If you want a portable setup, pass `--state-dir ./state --env-file ./.env`.

## Things this does NOT do

- **Doesn't bypass GoPro's terms of service.** This downloads files you've
  already uploaded to *your* account. Use it on your own library only.
- **Doesn't cancel your subscription.** Do that yourself once you've verified
  the backup, at <https://gopro.com/en/us/account/subscription>.
- **Doesn't preserve `MultiClipEdit` items.** Those are edit timelines, not
  media. Re-export them as videos in the GoPro Quik app first if you want them.
- **Doesn't refresh tokens.** When cookies expire, you'll see `HTTP 401`;
  re-extract the cookies and re-run. The script resumes cleanly.

## Acknowledgements

Inspired by [itsankoff/gopro-plus](https://github.com/itsankoff/gopro-plus),
which proved the `/media/x/zip/source` endpoint works for bulk download. This
project re-implements the approach with per-file requests, async + HTTP/2,
adaptive concurrency, and a focus on robustness for very large libraries.

## License

MIT. See [LICENSE](LICENSE).
