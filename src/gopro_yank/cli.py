"""Click CLI: pull, list, status, verify, manifest."""

from __future__ import annotations

import asyncio
import json
import sys
from collections.abc import Iterable
from pathlib import Path

import click
from rich.console import Console
from rich.table import Table

from gopro_yank import __version__
from gopro_yank.adaptive import AdaptiveLimiter
from gopro_yank.api import AuthError, GoProClient, MediaItem
from gopro_yank.download import DownloadResult, download_one, yyyy_mm
from gopro_yank.env import get_credentials
from gopro_yank.progress import RichProgress
from gopro_yank.state import Marker, MarkerStore

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
        console.print(
            f"\nGet your cookies from gopro.com, then create [cyan]{env_file}[/]:\n"
            "  AUTH_TOKEN=eyJhbGc...\n"
            "  USER_ID=00000000-...\n"
            "\nSee `gopro-yank --help` and the README for the cookie-extraction walkthrough."
        )
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
      1. Create ~/.config/gopro-yank/.env with AUTH_TOKEN + USER_ID
         (extract from gopro.com cookies — see README)
      2. gopro-yank pull --out ~/GoPro

    If no subcommand is given, runs `pull` with defaults.
    """
    if ctx.invoked_subcommand is None:
        ctx.invoke(pull)


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
            console.print(f"[red]✗ auth failed:[/] {e}")
            sys.exit(2)

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
            console.print("[red]✗ auth token expired mid-run.[/] refresh cookies and re-run.")
            sys.exit(2)
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
            console.print(f"[red]✗ {e}[/]")
            sys.exit(2)

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
            console.print(f"[red]✗ {e}[/]")
            sys.exit(2)
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
            console.print(f"[red]✗ {e}[/]")
            sys.exit(2)
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
