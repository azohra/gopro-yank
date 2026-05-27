"""Click CLI: pull, list, status, verify, manifest, login, skip."""

from __future__ import annotations

import asyncio
import json
import shutil
import subprocess
import sys
import webbrowser
from collections.abc import Iterable
from pathlib import Path

import click
from rich.console import Console
from rich.panel import Panel
from rich.prompt import Confirm, Prompt
from rich.table import Table

from gopro_yank import __version__
from gopro_yank.api import AuthError, GoProClient, MediaItem
from gopro_yank.demo import run_demo
from gopro_yank.download import DownloadResult, download_one, yyyy_mm
from gopro_yank.env import get_credentials
from gopro_yank.progress import RichProgress
from gopro_yank.state import Marker, MarkerStore


def _default_out_dir() -> Path:
    """Pick a sensible default destination for downloaded files.

    - macOS: ~/Pictures/GoPro
    - else: ./GoPro-Backup in the cwd
    """
    if sys.platform == "darwin":
        return Path.home() / "Pictures" / "GoPro"
    return Path.cwd() / "GoPro-Backup"


def _read_clipboard() -> str | None:
    """Best-effort cross-platform clipboard read. Returns None if unavailable.

    Used during `login` to capture cookies without having the user paste them
    into a terminal prompt — which is unreliable for long strings because
    POSIX canonical-mode tty input caps lines at MAX_CANON (~1024 bytes) and
    GoPro JWTs are routinely 1500+ chars.
    """
    cmds: list[list[str]] = []
    if sys.platform == "darwin":
        cmds = [["pbpaste"]]
    elif sys.platform.startswith("linux"):
        cmds = [
            ["wl-paste", "--no-newline"],
            ["xclip", "-selection", "clipboard", "-o"],
            ["xsel", "--clipboard", "--output"],
        ]
    elif sys.platform == "win32":
        cmds = [["powershell", "-NoProfile", "-Command", "Get-Clipboard"]]
    for cmd in cmds:
        if not shutil.which(cmd[0]):
            continue
        try:
            out = subprocess.check_output(cmd, text=True, stderr=subprocess.DEVNULL)
            return out.rstrip("\r\n")
        except (subprocess.CalledProcessError, FileNotFoundError):
            continue
    return None


def _preview(s: str, head: int = 24) -> str:
    if len(s) <= head:
        return s
    return f"{s[:head]}…"

MEDIA_LIBRARY_URL = "https://gopro.com/media-library/"

LOGIN_HINT = (
    "[yellow]→ run [bold cyan]gopro-yank login[/] to set up "
    "or refresh your cookies.[/]"
)

DEFAULT_ENV = Path.home() / ".config" / "gopro-yank" / ".env"
DEFAULT_STATE = Path.home() / ".local" / "share" / "gopro-yank" / "state"


def _common(fn):
    """Decorator applying shared options."""
    fn = click.option(
        "--env-file",
        "env_file",
        default=str(DEFAULT_ENV),
        type=click.Path(path_type=Path),
        show_default=True,
        help="Path to a .env file containing AUTH_TOKEN and USER_ID.",
    )(fn)
    fn = click.option(
        "--state-dir",
        "state_dir",
        default=str(DEFAULT_STATE),
        type=click.Path(path_type=Path),
        show_default=True,
        help="Where per-file markers are kept (drives resume behavior).",
    )(fn)
    return fn


def _credentials_or_die(env_file: Path, console: Console) -> tuple[str, str]:
    try:
        return get_credentials(env_file)
    except RuntimeError as e:
        console.print(f"[red]✗[/] {e}")
        console.print(LOGIN_HINT)
        sys.exit(2)


def _die_auth(console: Console, message: str) -> None:
    """Print a clear auth-failure message and exit. Used when the API rejects
    us mid-run with a 401 — usually expired cookies."""
    console.print(f"[red]✗ auth failed:[/] {message}")
    console.print(LOGIN_HINT)
    sys.exit(2)


@click.group(
    context_settings={"help_option_names": ["-h", "--help"]},
    invoke_without_command=True,
)
@click.version_option(__version__, prog_name="gopro-yank")
@click.pass_context
def main(ctx: click.Context) -> None:
    """Bulk-download your GoPro Plus cloud library.

    \b
    Quickstart:
      1. gopro-yank login              # interactive cookie setup
      2. gopro-yank pull --out ~/GoPro # download everything

    Run with no subcommand to see your current state and the next
    suggested command.
    """
    if ctx.invoked_subcommand is None:
        _show_status_banner()


def _show_status_banner() -> None:
    """Bare `gopro-yank` invocation: a friendly status overview that points
    the user at the right next command for where they are in the flow."""
    from rich.console import Group

    console = Console()
    has_env = DEFAULT_ENV.exists()
    state = MarkerStore(DEFAULT_STATE)
    marker_count = len(state.all_ids()) if state.dir.exists() else 0

    facts = Table.grid(padding=(0, 2))
    facts.add_column(style="bold cyan", no_wrap=True)
    facts.add_column()
    facts.add_row(
        "credentials",
        f"[green]✓[/] {DEFAULT_ENV}" if has_env else "[red]✗[/] not configured",
    )
    facts.add_row("state markers", f"{marker_count} item(s) recorded as done")

    if not has_env:
        next_cmd = (
            "[bold cyan]gopro-yank login[/]  — paste cookies, validate, save\n"
            "[dim]or[/] [cyan]gopro-yank demo[/]    — see the UI with simulated data (no creds)"
        )
    elif marker_count == 0:
        next_cmd = "[bold cyan]gopro-yank pull[/]  — first backup run (asks where to save)"
    else:
        next_cmd = (
            "[bold cyan]gopro-yank pull[/]    — resume / catch up\n"
            "[dim]or[/] [cyan]gopro-yank status[/]  — see what's pending"
        )

    console.print(
        Panel(
            Group(facts, "", f"[bold]next:[/]\n{next_cmd}"),
            title="[bold]gopro-yank[/]",
            border_style="cyan",
        )
    )


# ============================================================================
# pull
# ============================================================================


@main.command()
@_common
@click.option(
    "--out",
    "out",
    type=click.Path(path_type=Path),
    default=None,
    help="Where extracted files land (organized into YYYY/MM/ subfolders). Prompted if omitted.",
)
@click.option(
    "--parallel",
    default=8,
    show_default=True,
    type=int,
    help="How many files to download at once. Bump if your pipe is faster than GoPro's throttle.",
)
@click.option(
    "--per-page",
    default=30,
    show_default=True,
    type=int,
    help="Listing page size (does not affect downloads).",
)
def pull(
    env_file: Path,
    state_dir: Path,
    out: Path | None,
    parallel: int,
    per_page: int,
) -> None:
    """Download everything in the library (resumable)."""
    console = Console()
    token, user_id = _credentials_or_die(env_file, console)
    state = MarkerStore(state_dir)
    if out is None:
        default = _default_out_dir()
        out_str = click.prompt(
            "Where should files be saved?",
            default=str(default),
            type=str,
        )
        out = Path(out_str).expanduser()
    out.mkdir(parents=True, exist_ok=True)
    asyncio.run(
        _run_pull(
            token=token,
            user_id=user_id,
            state=state,
            out=out,
            parallel=parallel,
            per_page=per_page,
            console=console,
        )
    )


async def _run_pull(
    *,
    token: str,
    user_id: str,
    state: MarkerStore,
    out: Path,
    parallel: int,
    per_page: int,
    console: Console,
) -> None:
    async with GoProClient(
        token, user_id, max_connections=parallel + 4
    ) as client:
        try:
            await client.validate()
        except AuthError as e:
            _die_auth(console, str(e))

        with console.status("[cyan]listing media library...[/]"):
            items = await client.list_all(per_page=per_page)

        todo = [it for it in items if not state.has(it.id)]
        already_done = len(items) - len(todo)
        total_bytes = sum(int(it.file_size or 0) for it in todo)
        console.print(
            f"library: [bold]{len(items)}[/] items   "
            f"[green]{already_done} already done[/]   "
            f"[yellow]{len(todo)} to do (~{total_bytes / 1024**3:.1f} GB)[/]"
        )
        if not todo:
            console.print("[green]nothing to do.[/]")
            return

        sem = asyncio.Semaphore(parallel)
        auth_event = asyncio.Event()
        results: list[DownloadResult] = []

        with RichProgress(
            console=console,
            parallel=parallel,
            total_items=len(items),
            total_bytes=total_bytes,
            already_done=already_done,
        ) as ui:

            async def worker(item: MediaItem) -> None:
                if auth_event.is_set():
                    return
                async with sem:
                    res = await download_one(client, item, out, state, sink=ui)
                    if res.status == "auth":
                        auth_event.set()
                    results.append(res)

            tasks = [asyncio.create_task(worker(it)) for it in todo]
            try:
                await asyncio.gather(*tasks)
            except asyncio.CancelledError:
                for t in tasks:
                    t.cancel()
                raise

        # final summary
        ok = sum(1 for r in results if r.status == "ok")
        fail = [r for r in results if r.status == "fail"]
        auth_failed = any(r.status == "auth" for r in results)
        console.print()
        if auth_failed:
            _die_auth(console, "token expired mid-run (some files completed).")
        if fail:
            console.print(f"[red]{len(fail)} file(s) failed:[/]")
            for r in fail[:20]:
                console.print(f"  [dim]{r.media_id}[/]: {r.info}")
            console.print("re-run the same command to retry just the failures.")
            sys.exit(1)
        console.print(f"[green]✓ downloaded {ok} new file(s).[/]")
        console.print(
            f"\n[dim]tip:[/] [cyan]gopro-yank verify --out {out}[/]  "
            "[dim]— confirm every file landed on disk before you cancel.[/]"
        )


# ============================================================================
# list
# ============================================================================


@main.command("list")
@_common
@click.option("--per-page", default=30, show_default=True, type=int)
@click.option(
    "--all",
    "show_all",
    is_flag=True,
    help="Print every item. Without this flag, you get a summary + sample.",
)
@click.option(
    "--pending",
    is_flag=True,
    help="Only show items not yet downloaded.",
)
@click.option(
    "--done",
    is_flag=True,
    help="Only show items already downloaded.",
)
@click.option(
    "--limit",
    default=10,
    show_default=True,
    type=int,
    help="Number of items to show at head and tail of the sample (ignored with --all).",
)
@click.option("--json", "as_json", is_flag=True, help="Emit raw JSON, one item per line.")
def list_cmd(
    env_file: Path,
    state_dir: Path,
    per_page: int,
    show_all: bool,
    pending: bool,
    done: bool,
    limit: int,
    as_json: bool,
) -> None:
    """Summarize your library: counts, sizes, date range, with a sample.

    \b
    By default, prints:
      • a stats panel (counts by status, total size, date range, year breakdown)
      • the first + last N items as a sample

    Use --all to print every item, --pending/--done to filter, --json to stream
    raw API records (one item per line) for piping into jq.
    """
    if pending and done:
        click.echo("--pending and --done are mutually exclusive", err=True)
        sys.exit(2)
    console = Console()
    token, user_id = _credentials_or_die(env_file, console)
    state = MarkerStore(state_dir)
    asyncio.run(
        _run_list(
            token, user_id, state, per_page, show_all, pending, done, limit, as_json, console
        )
    )


def _human_bytes(n: int | None) -> str:
    if n is None:
        return "—"
    if n == 0:
        return "0 B"
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if n < 1024:
            return f"{n:.1f} {unit}"
        n /= 1024
    return f"{n:.1f} PB"


def _render_item_row(it: MediaItem, state: MarkerStore) -> tuple[str, str, str, str, str]:
    is_done = state.has(it.id)
    status = "[green]done[/]" if is_done else "[yellow]todo[/]"
    size = _human_bytes(it.file_size)
    created = (it.created_at or "—")[:10]  # YYYY-MM-DD
    return status, it.filename or "—", size, created, it.id


def _list_table(rows: Iterable[tuple]) -> Table:
    t = Table(show_header=True, header_style="bold", show_lines=False, expand=True)
    t.add_column("status", no_wrap=True)
    t.add_column("filename")
    t.add_column("size", justify="right", no_wrap=True)
    t.add_column("created", no_wrap=True)
    t.add_column("id", style="dim", no_wrap=True)
    for row in rows:
        t.add_row(*row)
    return t


async def _run_list(
    token: str,
    user_id: str,
    state: MarkerStore,
    per_page: int,
    show_all: bool,
    pending: bool,
    done: bool,
    limit: int,
    as_json: bool,
    console: Console,
) -> None:
    from rich.console import Group

    async with GoProClient(token, user_id) as client:
        try:
            await client.validate()
        except AuthError as e:
            _die_auth(console, str(e))

        # JSON path streams without buffering — efficient for huge libraries.
        if as_json:
            async for it in client.iter_media(per_page=per_page):
                marker = state.read(it.id)
                payload = {**it.raw, "_done": state.has(it.id)}
                if marker and marker.get("status") == "skipped":
                    payload["_skipped"] = True
                click.echo(json.dumps(payload))
            return

        with console.status("[cyan]fetching library...[/]"):
            items = await client.list_all(per_page=per_page)

    # Filter
    def included(it: MediaItem) -> bool:
        is_done = state.has(it.id)
        if pending and is_done:
            return False
        if done and not is_done:
            return False
        return True

    filtered = [it for it in items if included(it)]

    # Stats over the *original* library (not the filter) so summary numbers
    # match what `status` would report.
    n_total = len(items)
    n_done = sum(1 for it in items if state.has(it.id))
    n_skipped = sum(
        1
        for it in items
        if (m := state.read(it.id)) and m.get("status") == "skipped"
    )
    n_pending = n_total - n_done
    total_bytes = sum(int(it.file_size or 0) for it in items)
    dates = sorted(it.created_at[:10] for it in items if it.created_at)
    date_range = f"{dates[0]} → {dates[-1]}" if dates else "—"

    # Year breakdown
    years: dict[str, int] = {}
    for it in items:
        if it.created_at and len(it.created_at) >= 4:
            y = it.created_at[:4]
            years[y] = years.get(y, 0) + 1

    facts = Table.grid(padding=(0, 2))
    facts.add_column(style="bold cyan", no_wrap=True)
    facts.add_column()
    facts.add_row("library", f"{n_total} items   {_human_bytes(total_bytes)}")
    facts.add_row(
        "status",
        f"[green]{n_done} done[/]   "
        + (f"[dim]{n_skipped} skipped[/]   " if n_skipped else "")
        + f"[yellow]{n_pending - n_skipped} pending[/]"
        if n_skipped
        else f"[green]{n_done} done[/]   [yellow]{n_pending} pending[/]",
    )
    facts.add_row("date range", date_range)
    if years:
        year_str = "   ".join(
            f"[bold]{y}[/]: {c}" for y, c in sorted(years.items())
        )
        facts.add_row("by year", year_str)

    label = "library"
    if pending:
        label = f"pending ({len(filtered)} item{'s' if len(filtered) != 1 else ''})"
    elif done:
        label = f"done ({len(filtered)} item{'s' if len(filtered) != 1 else ''})"

    console.print(Panel(facts, title=f"[bold]{label}[/]", border_style="cyan"))

    if not filtered:
        console.print("[dim]nothing to list under that filter.[/]")
        return

    rows = [_render_item_row(it, state) for it in filtered]

    if show_all or len(filtered) <= 2 * limit + 1:
        console.print(_list_table(rows))
    else:
        head = rows[:limit]
        tail = rows[-limit:]
        ellipsis = (
            "[dim italic]…[/]",
            f"[dim italic]({len(filtered) - 2 * limit} more — pass --all to see them)[/]",
            "",
            "",
            "",
        )
        console.print(_list_table([*head, ellipsis, *tail]))

    if not show_all and len(filtered) > 2 * limit + 1:
        console.print(
            "[dim]tip:[/] [cyan]gopro-yank list --all[/]  "
            "[dim]·[/]  [cyan]--pending[/]  "
            "[dim]·[/]  [cyan]--json | jq[/]"
        )


# ============================================================================
# status
# ============================================================================


@main.command()
@_common
@click.option(
    "--out",
    "out",
    type=click.Path(path_type=Path),
    default=None,
    help="Where files were saved by pull. Prompted if omitted.",
)
def status(env_file: Path, state_dir: Path, out: Path | None) -> None:
    """Summarize what's done, what's pending, and what's on disk."""
    console = Console()
    token, user_id = _credentials_or_die(env_file, console)
    state = MarkerStore(state_dir)
    if out is None:
        out = Path(
            click.prompt("Where are files saved?", default=str(_default_out_dir()))
        ).expanduser()
    asyncio.run(_run_status(token, user_id, state, out, console))


async def _run_status(
    token: str, user_id: str, state: MarkerStore, out: Path, console: Console
) -> None:
    async with GoProClient(token, user_id) as client:
        try:
            await client.validate()
        except AuthError as e:
            _die_auth(console, str(e))
        items = await client.list_all()
    library_ids = {it.id for it in items}
    marker_ids = state.all_ids()
    pending = [it for it in items if it.id not in marker_ids]
    stranded = marker_ids - library_ids  # markers whose source item is gone

    disk_files = 0
    disk_bytes = 0
    if out.exists():
        for p in out.rglob("*"):
            if p.is_file() and not p.name.startswith("."):
                disk_files += 1
                try:
                    disk_bytes += p.stat().st_size
                except OSError:
                    pass

    t = Table.grid(padding=(0, 2))
    t.add_column(style="bold cyan")
    t.add_column()
    t.add_row("library items", str(len(items)))
    t.add_row("markers", str(len(marker_ids)))
    t.add_row("pending", f"{len(pending)} ({sum((it.file_size or 0) for it in pending) / 1024**3:.1f} GB)")
    t.add_row("stranded markers", str(len(stranded)))
    t.add_row("files on disk", f"{disk_files} ({disk_bytes / 1024**3:.1f} GB) at {out}")
    console.print(t)
    if pending:
        console.print(f"\nrun [cyan]gopro-yank pull --out {out}[/] to fetch the {len(pending)} pending items.")


# ============================================================================
# verify
# ============================================================================


@main.command()
@_common
@click.option(
    "--out",
    "out",
    type=click.Path(path_type=Path),
    default=None,
    help="Where files were saved by pull. Prompted if omitted.",
)
@click.option(
    "--size-tolerance",
    default=0.01,
    show_default=True,
    type=float,
    help="Acceptable fractional difference between summed chapter sizes and API total.",
)
def verify(
    env_file: Path, state_dir: Path, out: Path | None, size_tolerance: float
) -> None:
    """Check each marker's files exist and (for multi-chapter clips) sum to the
    media item's reported file_size within --size-tolerance.

    GoPro splits long recordings into chapters (GX010..., GX020..., ...). The
    API's file_size is the total for the media item; on disk you'll see one
    file per chapter. We sum the chapter sizes and compare to the total.
    """
    console = Console()
    state = MarkerStore(state_dir)
    if out is None:
        out = Path(
            click.prompt("Where are files saved?", default=str(_default_out_dir()))
        ).expanduser()
    bad: list[tuple[str, str]] = []
    n = 0
    for marker_path in state.dir.glob("*.json"):
        data = json.loads(marker_path.read_text())
        if data.get("status") == "skipped":
            continue
        n += 1
        saved = data.get("saved", [])
        total_on_disk = 0
        any_missing = False
        for rel in saved:
            p = out / rel
            if not p.exists():
                bad.append((data["media_id"], f"missing on disk: {rel}"))
                any_missing = True
                continue
            total_on_disk += p.stat().st_size
        if any_missing:
            continue
        expected = data.get("file_size")
        if expected and saved:
            diff = abs(total_on_disk - expected) / max(expected, 1)
            if diff > size_tolerance:
                bad.append(
                    (
                        data["media_id"],
                        f"summed size {total_on_disk} differs from API total {expected} "
                        f"by {diff * 100:.2f}% ({len(saved)} chapter(s))",
                    )
                )
    console.print(f"checked {n} markers")
    if bad:
        console.print(f"[red]{len(bad)} issue(s):[/]")
        for mid, msg in bad[:50]:
            console.print(f"  [dim]{mid}[/]: {msg}")
        sys.exit(1)
    console.print("[green]✓ everything matches[/]")
    console.print(
        "\n[dim]tip:[/] your backup is complete and verified. "
        "safe to cancel at\n"
        "  [cyan]https://gopro.com/en/us/account/subscription[/]"
    )


# ============================================================================
# manifest
# ============================================================================


@main.command()
@_common
@click.option(
    "--out-file",
    "out_file",
    type=click.Path(path_type=Path),
    default=None,
    help="Where to write the manifest JSON (default: stdout).",
)
def manifest(env_file: Path, state_dir: Path, out_file: Path | None) -> None:
    """Export a JSON manifest of every library item + marker state."""
    console = Console()
    token, user_id = _credentials_or_die(env_file, console)
    state = MarkerStore(state_dir)
    asyncio.run(_run_manifest(token, user_id, state, out_file, console))


async def _run_manifest(
    token: str, user_id: str, state: MarkerStore, out_file: Path | None, console: Console
) -> None:
    async with GoProClient(token, user_id) as client:
        try:
            await client.validate()
        except AuthError as e:
            _die_auth(console, str(e))
        items = await client.list_all()
    out_list: list[dict] = []
    for it in items:
        marker = state.read(it.id)
        out_list.append(
            {
                "id": it.id,
                "filename": it.filename,
                "file_size": it.file_size,
                "created_at": it.created_at,
                "yyyy_mm": yyyy_mm(it.created_at),
                "marker": marker,
            }
        )
    payload = json.dumps({"items": out_list, "count": len(out_list)}, indent=2)
    if out_file:
        out_file.write_text(payload)
        console.print(f"wrote manifest: {out_file}")
    else:
        click.echo(payload)


# ============================================================================
# demo — simulated downloads, no credentials, full TUI
# ============================================================================


@main.command()
@click.option("--count", default=24, show_default=True, type=int, help="Number of fake items.")
@click.option("--parallel", default=4, show_default=True, type=int)
@click.option(
    "--target-mbps",
    default=80.0,
    show_default=True,
    type=float,
    help="Simulated per-worker throughput. Lower = slower demo.",
)
def demo(count: int, parallel: int, target_mbps: float) -> None:
    """Preview the UI against simulated data — no credentials required.

    \b
    Great for:
      • seeing what gopro-yank looks like before installing creds
      • recording screenshots / asciinema demos
      • verifying a fresh install end-to-end
    """
    console = Console()
    try:
        asyncio.run(
            run_demo(
                count=count,
                parallel=parallel,
                target_mbps=target_mbps,
                console=console,
            )
        )
    except KeyboardInterrupt:
        console.print("\n[yellow]demo interrupted.[/]")
        sys.exit(130)


# ============================================================================
# login — interactive cookie capture + validation
# ============================================================================


_LOGIN_INTRO = """\
[bold]gopro-yank login[/] — capture your two API cookies from gopro.com.

In the browser:
  1. Make sure you're [bold]signed in[/] at [cyan]gopro.com[/]
  2. Open DevTools — [bold]Cmd+Opt+I[/] (Mac) or [bold]Ctrl+Shift+I[/] (other)
  3. Go to [bold]Application[/] tab → [bold]Storage[/] → [bold]Cookies[/] → [cyan]https://gopro.com[/]
  4. Find these two rows; you'll copy each [bold]Value[/] in turn:
       • [cyan]gp_access_token[/]  (long JWT, starts with [dim]eyJhbGc...[/])
       • [cyan]gp_user_id[/]       (UUID like [dim]00000000-0000-0000-0000-000000000000[/])

I'll read each value from your clipboard so long tokens don't get mangled by
your terminal's line-length limit.
"""


def _capture_cookie(
    console: Console,
    label: str,
    expected_prefix: str | None,
    *,
    use_paste: bool,
    clipboard_ok: bool,
) -> str:
    """Capture one cookie value.

    Clipboard flow (default on macOS/Linux/Win): tell the user what to copy,
    then wait for Enter, *then* read the clipboard. This avoids reading stale
    contents (the bug if we read at prompt-time before they've had a chance
    to copy) and gives them a confirmable preview.

    Direct-paste flow (`--paste` or no clipboard tool): falls back to Rich
    Prompt, which renders markup and accepts up to the terminal's MAX_CANON
    limit (~1024 chars on macOS — fine for the UUID but borderline for JWTs).
    """
    console.print()
    console.rule(f"[bold cyan]{label}[/]")
    if expected_prefix == "eyJ":
        console.print(
            "  [dim]expected: a long JWT, hundreds–thousands of chars, "
            f"starting with [bold]{expected_prefix}[/dim][/]"
        )
    elif expected_prefix:
        console.print(f"  [dim]expected format: {expected_prefix}…[/]")

    while True:
        if use_paste or not clipboard_ok:
            value = Prompt.ask(f"  paste [cyan]{label}[/]", console=console).strip()
        else:
            console.print(
                f"  In DevTools, copy the [bold cyan]{label}[/] [bold]Value[/], then come back here."
            )
            console.input(f"  [dim]press Enter when it's on your clipboard ›[/dim] ")
            value = (_read_clipboard() or "").strip()
            if not value:
                console.print("  [yellow]clipboard was empty — try again.[/]")
                continue
            console.print(
                f"  [green]✓[/] got [bold]{len(value)}[/] chars "
                f"[dim](preview: {_preview(value)})[/]"
            )

        if expected_prefix and not value.startswith(expected_prefix):
            console.print(
                f"  [yellow]⚠[/]  doesn't start with [dim]{expected_prefix}[/] — "
                "did you copy the right cookie?"
            )
            if not Confirm.ask("  use it anyway?", default=False, console=console):
                continue

        if Confirm.ask("  looks right?", default=True, console=console):
            return value


@main.command()
@click.option(
    "--env-file",
    "env_file",
    default=str(DEFAULT_ENV),
    type=click.Path(path_type=Path),
    show_default=True,
    help="Where to save AUTH_TOKEN and USER_ID.",
)
@click.option(
    "--no-browser",
    is_flag=True,
    help="Don't try to open gopro.com in your browser.",
)
@click.option(
    "--force",
    is_flag=True,
    help="Overwrite an existing env file without confirming.",
)
@click.option(
    "--paste",
    "use_paste",
    is_flag=True,
    help="Skip clipboard auto-read; prompt for direct input instead.",
)
def login(env_file: Path, no_browser: bool, force: bool, use_paste: bool) -> None:
    """Interactive cookie capture: clipboard-first, validate, save.

    On macOS, Linux (with xclip/xsel/wl-paste), or Windows, reads each cookie
    from your clipboard so long JWTs don't get truncated by the terminal's
    line-length limit. Falls back to direct prompt if the clipboard isn't
    available.
    """
    console = Console()

    if env_file.exists() and not force:
        if not click.confirm(
            f"{env_file} already exists. Overwrite?",
            default=False,
        ):
            console.print("[yellow]aborted — existing file kept.[/]")
            sys.exit(1)

    console.print(Panel(_LOGIN_INTRO, border_style="cyan", title="how this works"))

    clipboard_ok = _read_clipboard() is not None
    if not use_paste and not clipboard_ok:
        console.print(
            "[yellow]no clipboard tool available — falling back to direct prompts. "
            "if your token is very long, run with [bold]--paste[/] and use a graphical "
            "editor as a workaround, or install xclip/wl-clipboard on Linux.[/]"
        )

    if not no_browser:
        if click.confirm(
            f"Open {MEDIA_LIBRARY_URL} in your browser now?",
            default=True,
        ):
            try:
                webbrowser.open(MEDIA_LIBRARY_URL)
            except Exception:  # noqa: BLE001
                console.print(f"[yellow]couldn't open browser; visit {MEDIA_LIBRARY_URL} manually.[/]")

    token = _capture_cookie(
        console,
        "gp_access_token",
        expected_prefix="eyJ",
        use_paste=use_paste,
        clipboard_ok=clipboard_ok,
    )
    user_id = _capture_cookie(
        console,
        "gp_user_id",
        expected_prefix=None,
        use_paste=use_paste,
        clipboard_ok=clipboard_ok,
    )

    console.print()
    me: dict | None = None
    with console.status("[cyan]validating with the GoPro API...[/]"):
        try:
            me = asyncio.run(_validate_creds(token, user_id))
        except AuthError as e:
            console.print(f"[red]✗ rejected by GoPro:[/] {e}")
            console.print("[dim]Double-check you copied the entire Value column — JWTs are long.[/]")
            sys.exit(1)
        except Exception as e:  # noqa: BLE001
            console.print(f"[red]✗ network error during validation:[/] {e!r}")
            sys.exit(1)

    env_file.parent.mkdir(parents=True, exist_ok=True)
    env_file.write_text(f"AUTH_TOKEN={token}\nUSER_ID={user_id}\n")
    try:
        env_file.chmod(0o600)
    except OSError:
        pass

    who = me.get("email") or me.get("id") or user_id if me else user_id
    console.print(f"[green]✓ logged in[/] as [bold]{who}[/]")
    console.print(f"  saved to [dim]{env_file}[/]")
    console.print()
    console.print("[bold]Next:[/]")
    console.print("  [cyan]gopro-yank list[/]   show your library")
    console.print("  [cyan]gopro-yank pull[/]   download everything (prompts for destination)")


async def _validate_creds(token: str, user_id: str) -> dict:
    async with GoProClient(token, user_id) as client:
        return await client.validate()


# ============================================================================
# skip — mark items as deliberately skipped (e.g. MultiClipEdit)
# ============================================================================


@main.command("skip")
@_common
@click.argument("media_ids", nargs=-1, required=True)
@click.option("--reason", default="manually skipped", show_default=True)
def skip_cmd(env_file: Path, state_dir: Path, media_ids: Iterable[str], reason: str) -> None:
    """Write a 'skipped' marker for one or more media IDs so they don't retry."""
    console = Console()
    state = MarkerStore(state_dir)
    for mid in media_ids:
        state.write(Marker(media_id=mid, status="skipped", reason=reason))
        console.print(f"[yellow]skip[/] {mid}")


if __name__ == "__main__":  # pragma: no cover
    main()
