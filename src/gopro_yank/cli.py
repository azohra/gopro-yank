"""Click CLI: pull, list, status, verify, manifest, login, skip."""

from __future__ import annotations

import asyncio
import json
import sys
import webbrowser
from collections.abc import Iterable
from pathlib import Path

import click
from rich.console import Console
from rich.panel import Panel
from rich.table import Table

from gopro_yank import __version__
from gopro_yank.adaptive import AdaptiveLimiter
from gopro_yank.api import AuthError, GoProClient, MediaItem
from gopro_yank.download import DownloadResult, download_one, yyyy_mm
from gopro_yank.env import get_credentials
from gopro_yank.progress import RichProgress
from gopro_yank.state import Marker, MarkerStore

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
        next_cmd = "[bold cyan]gopro-yank login[/]  — paste cookies, validate, save"
    elif marker_count == 0:
        next_cmd = "[bold cyan]gopro-yank pull --out <directory>[/]  — first backup run"
    else:
        next_cmd = (
            "[bold cyan]gopro-yank pull --out <directory>[/]  — resume / catch up\n"
            "[dim]or[/] [cyan]gopro-yank status --out <directory>[/]  — see what's pending"
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
    required=True,
    type=click.Path(path_type=Path),
    help="Directory where extracted files land. Organized into YYYY/MM/ subfolders.",
)
@click.option(
    "--initial",
    default=4,
    show_default=True,
    type=int,
    help="Starting concurrency.",
)
@click.option(
    "--ceiling",
    default=16,
    show_default=True,
    type=int,
    help="Max concurrent downloads (adaptive ramp-up cap).",
)
@click.option(
    "--floor",
    default=1,
    show_default=True,
    type=int,
    help="Min concurrent downloads (adaptive shrink floor).",
)
@click.option(
    "--grow-after",
    default=8,
    show_default=True,
    type=int,
    help="Successful downloads in a row before bumping concurrency by 1.",
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
    out: Path,
    initial: int,
    ceiling: int,
    floor: int,
    grow_after: int,
    per_page: int,
) -> None:
    """Download everything in the library (resumable). Default when no subcommand."""
    console = Console()
    token, user_id = _credentials_or_die(env_file, console)
    state = MarkerStore(state_dir)
    out.mkdir(parents=True, exist_ok=True)
    asyncio.run(
        _run_pull(
            token=token,
            user_id=user_id,
            state=state,
            out=out,
            initial=initial,
            ceiling=ceiling,
            floor=floor,
            grow_after=grow_after,
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
    initial: int,
    ceiling: int,
    floor: int,
    grow_after: int,
    per_page: int,
    console: Console,
) -> None:
    async with GoProClient(
        token, user_id, max_connections=ceiling + 4
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

        limiter = AdaptiveLimiter(
            initial=initial,
            floor=floor,
            ceiling=ceiling,
            grow_after=grow_after,
        )
        auth_event = asyncio.Event()
        results: list[DownloadResult] = []

        with RichProgress(
            console=console,
            limiter=limiter,
            total_items=len(items),
            total_bytes=total_bytes,
            already_done=already_done,
        ) as ui:

            async def worker(item: MediaItem) -> None:
                if auth_event.is_set():
                    return
                async with limiter.slot() as slot:
                    res = await download_one(client, item, out, state, sink=ui)
                    # ok + skip both count as healthy (skip is "already done").
                    # fail/auth count against the limiter so it can shrink.
                    slot.success = res.status in ("ok", "skip")
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


# ============================================================================
# list
# ============================================================================


@main.command("list")
@_common
@click.option(
    "--per-page", default=30, show_default=True, type=int,
)
@click.option("--json", "as_json", is_flag=True, help="Emit raw JSON, one item per line.")
def list_cmd(env_file: Path, state_dir: Path, per_page: int, as_json: bool) -> None:
    """List every item in the library and its done/pending state."""
    console = Console()
    token, user_id = _credentials_or_die(env_file, console)
    state = MarkerStore(state_dir)
    asyncio.run(_run_list(token, user_id, state, per_page, as_json, console))


async def _run_list(
    token: str, user_id: str, state: MarkerStore, per_page: int, as_json: bool, console: Console
) -> None:
    async with GoProClient(token, user_id) as client:
        try:
            await client.validate()
        except AuthError as e:
            _die_auth(console, str(e))

        if as_json:
            async for it in client.iter_media(per_page=per_page):
                done = state.has(it.id)
                click.echo(json.dumps({**it.raw, "_done": done}))
            return

        t = Table(title="GoPro library", show_lines=False, expand=True)
        t.add_column("status", style="bold")
        t.add_column("filename")
        t.add_column("size", justify="right")
        t.add_column("created")
        t.add_column("id", style="dim")
        n = 0
        async for it in client.iter_media(per_page=per_page):
            status = "[green]done[/]" if state.has(it.id) else "[yellow]todo[/]"
            size = f"{(it.file_size or 0) / 1024**2:.1f} MB" if it.file_size else "—"
            t.add_row(status, it.filename or "—", size, it.created_at or "—", it.id)
            n += 1
        console.print(t)
        console.print(f"[dim]{n} items[/]")


# ============================================================================
# status
# ============================================================================


@main.command()
@_common
@click.option("--out", "out", required=True, type=click.Path(path_type=Path))
def status(env_file: Path, state_dir: Path, out: Path) -> None:
    """Summarize what's done, what's pending, and what's on disk."""
    console = Console()
    token, user_id = _credentials_or_die(env_file, console)
    state = MarkerStore(state_dir)
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
@click.option("--out", "out", required=True, type=click.Path(path_type=Path))
@click.option(
    "--size-tolerance",
    default=0.01,
    show_default=True,
    type=float,
    help="Acceptable fractional difference between summed chapter sizes and API total.",
)
def verify(env_file: Path, state_dir: Path, out: Path, size_tolerance: float) -> None:
    """Check each marker's files exist and (for multi-chapter clips) sum to the
    media item's reported file_size within --size-tolerance.

    GoPro splits long recordings into chapters (GX010..., GX020..., ...). The
    API's file_size is the total for the media item; on disk you'll see one
    file per chapter. We sum the chapter sizes and compare to the total.
    """
    console = Console()
    state = MarkerStore(state_dir)
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
# login — interactive cookie capture + validation
# ============================================================================


_LOGIN_INTRO = """\
[bold]gopro-yank login[/] — walk through getting your two API cookies.

In the browser:
  1. Make sure you're [bold]signed in[/] at [cyan]gopro.com[/]
  2. Open DevTools — [bold]Cmd+Opt+I[/] (Mac) or [bold]Ctrl+Shift+I[/] (other)
  3. Go to [bold]Application[/] tab → [bold]Storage[/] → [bold]Cookies[/] → [cyan]https://gopro.com[/]
  4. Find these two rows and copy the [bold]Value[/] column:
       • [cyan]gp_access_token[/]  (long JWT, starts with [dim]eyJhbGc...[/])
       • [cyan]gp_user_id[/]       (UUID like [dim]00000000-0000-0000-0000-000000000000[/])

I'll prompt for each value below. Paste, press Enter.
"""


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
def login(env_file: Path, no_browser: bool, force: bool) -> None:
    """Interactive cookie capture: paste, validate, save.

    Opens gopro.com in your browser, walks you through DevTools, prompts for
    the two cookie values, validates them against the GoPro API, and saves to
    a .env file with mode 600.
    """
    console = Console()

    if env_file.exists() and not force:
        if not click.confirm(
            f"{env_file} already exists. Overwrite?",
            default=False,
        ):
            console.print("[yellow]aborted — existing file kept.[/]")
            sys.exit(1)

    console.print(Panel(_LOGIN_INTRO, border_style="cyan", title="step 1 — get the cookies"))

    if not no_browser:
        if click.confirm(
            f"Open {MEDIA_LIBRARY_URL} in your browser now?",
            default=True,
        ):
            try:
                webbrowser.open(MEDIA_LIBRARY_URL)
            except Exception:  # noqa: BLE001
                console.print(f"[yellow]couldn't open browser; visit {MEDIA_LIBRARY_URL} manually.[/]")

    console.print()
    token = click.prompt("[bold cyan]gp_access_token[/]", prompt_suffix=" › ").strip()
    user_id = click.prompt("[bold cyan]gp_user_id[/]", prompt_suffix=" › ").strip()

    if not token.startswith("eyJ"):
        console.print(
            "[yellow]⚠[/]  that doesn't look like a JWT (expected to start with [dim]eyJ[/]). "
            "Continuing anyway — we'll find out for sure when we validate."
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
        pass  # Windows or unusual filesystems — don't crash, but warn

    who = me.get("email") or me.get("id") or user_id if me else user_id
    console.print(f"[green]✓ logged in[/] as [bold]{who}[/]")
    console.print(f"  saved to [dim]{env_file}[/]")
    console.print()
    console.print("[bold]Next:[/]")
    console.print("  [cyan]gopro-yank list[/]                   show your library")
    console.print("  [cyan]gopro-yank pull --out <directory>[/] download everything")


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
