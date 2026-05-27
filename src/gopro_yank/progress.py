"""Rich-based progress display.

Implements the ProgressSink Protocol from download.py. Renders a header panel
with running stats (concurrency, throughput, errors) above a Rich Progress
showing one bar per in-flight file plus an overall bar.
"""

from __future__ import annotations

import time
from dataclasses import dataclass, field

from rich.console import Console, Group
from rich.live import Live
from rich.panel import Panel
from rich.progress import (
    BarColumn,
    DownloadColumn,
    Progress,
    SpinnerColumn,
    TaskID,
    TaskProgressColumn,
    TextColumn,
    TimeRemainingColumn,
    TransferSpeedColumn,
)
from rich.table import Table


@dataclass(slots=True)
class _RunStats:
    ok: int = 0
    fail: int = 0
    skip: int = 0
    auth: int = 0
    bytes_done: int = 0
    started: float = field(default_factory=time.monotonic)
    failed_items: list[tuple[str, str]] = field(default_factory=list)

    def elapsed(self) -> float:
        return max(1e-6, time.monotonic() - self.started)

    def mbps(self) -> float:
        return self.bytes_done / 1024 / 1024 / self.elapsed()


def _fmt_bytes(n: int | None) -> str:
    if n is None:
        return "?"
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if n < 1024:
            return f"{n:.1f} {unit}"
        n /= 1024
    return f"{n:.1f} PB"


def _fmt_eta(seconds: float) -> str:
    if seconds <= 0 or seconds != seconds:  # NaN
        return "—"
    h, rem = divmod(int(seconds), 3600)
    m, s = divmod(rem, 60)
    if h:
        return f"{h}h{m:02d}m"
    if m:
        return f"{m}m{s:02d}s"
    return f"{s}s"


class RichProgress:
    """ProgressSink that drives a Live Rich UI."""

    def __init__(
        self,
        console: Console | None = None,
        *,
        parallel: int = 0,
        total_items: int = 0,
        total_bytes: int = 0,
        already_done: int = 0,
    ) -> None:
        self.console = console or Console()
        self.parallel = parallel
        self.total_items = total_items
        self.total_bytes = total_bytes
        self.already_done = already_done
        self.stats = _RunStats()
        self._file_tasks: dict[str, TaskID] = {}
        self._file_names: dict[str, str] = {}
        self._file_bytes: dict[str, int] = {}
        self._inflight = 0

        self.progress = Progress(
            SpinnerColumn(style="cyan"),
            TextColumn("[progress.description]{task.description}"),
            BarColumn(bar_width=None),
            TaskProgressColumn(),
            DownloadColumn(),
            TransferSpeedColumn(),
            TimeRemainingColumn(),
            console=self.console,
            transient=False,
            expand=True,
        )
        self._overall_id = self.progress.add_task(
            "[bold]overall[/]",
            total=total_bytes if total_bytes > 0 else None,
        )

        self._live: Live | None = None

    # --- header rendering --------------------------------------------------

    def _header(self) -> Panel:
        t = Table.grid(padding=(0, 2))
        t.add_column(style="bold cyan", no_wrap=True)
        t.add_column()
        done = self.already_done + self.stats.ok + self.stats.skip
        eta_secs = (
            (self.total_bytes - self.stats.bytes_done) / max(1, self.stats.bytes_done) * self.stats.elapsed()
            if self.stats.bytes_done > 0
            else 0
        )
        rows = [
            ("library", f"{self.total_items} items   {_fmt_bytes(self.total_bytes)}"),
            (
                "progress",
                f"[green]ok {self.stats.ok}[/]  "
                f"[yellow]skip {self.stats.skip}[/]  "
                f"[red]fail {self.stats.fail}[/]  "
                f"[dim]done {done}/{self.total_items}[/]",
            ),
            (
                "throughput",
                f"{self.stats.mbps():.1f} MB/s   "
                f"transferred {_fmt_bytes(self.stats.bytes_done)}   "
                f"eta {_fmt_eta(eta_secs)}",
            ),
        ]
        if self.parallel:
            rows.append(
                ("workers", f"{self._inflight}/{self.parallel} active")
            )
        for k, v in rows:
            t.add_row(k, v)
        return Panel(t, title="[bold]gopro-yank[/]", border_style="cyan")

    def _renderable(self) -> Group:
        return Group(self._header(), self.progress)

    # --- live lifecycle ----------------------------------------------------

    def __enter__(self) -> RichProgress:
        self._live = Live(
            self._renderable(),
            console=self.console,
            refresh_per_second=8,
            transient=False,
        )
        self._live.__enter__()
        return self

    def __exit__(self, *args: object) -> None:
        if self._live:
            # final refresh so the last numbers stick
            self._live.update(self._renderable(), refresh=True)
            self._live.__exit__(*args)

    def _refresh(self) -> None:
        if self._live:
            self._live.update(self._renderable())

    # --- ProgressSink ------------------------------------------------------

    def file_start(self, media_id: str, filename: str, expected_size: int | None) -> None:
        tid = self.progress.add_task(
            f"  [dim]{filename[:40]}[/]",
            total=expected_size,
        )
        self._file_tasks[media_id] = tid
        self._file_names[media_id] = filename
        self._file_bytes[media_id] = 0
        self._inflight += 1
        self._refresh()

    def file_chunk(self, media_id: str, chunk_size: int) -> None:
        tid = self._file_tasks.get(media_id)
        if tid is not None:
            self.progress.update(tid, advance=chunk_size)
        self._file_bytes[media_id] = self._file_bytes.get(media_id, 0) + chunk_size
        self.stats.bytes_done += chunk_size
        self.progress.update(self._overall_id, advance=chunk_size)

    def file_done(self, media_id: str, status: str, info: str = "") -> None:
        tid = self._file_tasks.pop(media_id, None)
        self._file_bytes.pop(media_id, None)
        name = self._file_names.pop(media_id, media_id)
        if tid is not None:
            self.progress.remove_task(tid)
        self._inflight = max(0, self._inflight - 1)
        if status == "ok":
            self.stats.ok += 1
        elif status == "skip":
            self.stats.skip += 1
        elif status == "auth":
            self.stats.auth += 1
            self.stats.failed_items.append((media_id, f"auth: {info}"))
        else:
            self.stats.fail += 1
            self.stats.failed_items.append((media_id, info))
            self.console.log(f"[red]✗[/] {name} ({media_id}): {info}")
        self._refresh()
