"""Simulated download flow for `gopro-yank demo`.

Mirrors the real `pull` pipeline (adaptive limiter + Rich progress UI) but
uses fake media items and asyncio.sleep() to simulate network I/O. Useful
for:
  - first-time users wanting to see what the tool does
  - taking screenshots / asciinema recordings for the README
  - manually testing the UI changes without burning credentials
"""

from __future__ import annotations

import asyncio
import random
from collections.abc import Iterable
from dataclasses import dataclass

from rich.console import Console

from gopro_yank.adaptive import AdaptiveLimiter
from gopro_yank.progress import RichProgress


@dataclass(slots=True)
class FakeItem:
    id: str
    filename: str
    file_size: int
    created_at: str
    fail_probability: float


def _generate_items(count: int, *, seed: int = 7) -> list[FakeItem]:
    rng = random.Random(seed)
    extensions = ["MP4", "MP4", "MP4", "MP4", "360", "JPG"]
    cameras = ["GX01", "GX02", "GH01", "GS01"]
    years = [2022, 2023, 2024, 2025]
    months = list(range(1, 13))

    items: list[FakeItem] = []
    for i in range(count):
        ext = rng.choice(extensions)
        cam = rng.choice(cameras)
        # Real GoPro 4K MP4s tend to be 50 MB – 4 GB. Mix small + large.
        if ext == "JPG":
            size = rng.randint(2 * 1024**2, 12 * 1024**2)
        elif ext == "360":
            size = rng.randint(800 * 1024**2, 4 * 1024**3)
        else:
            size = rng.randint(80 * 1024**2, 2 * 1024**3)
        year = rng.choice(years)
        month = rng.choice(months)
        items.append(
            FakeItem(
                id=f"FAKE{i:04d}{rng.randrange(10**8):08x}",
                filename=f"{cam}{i:04d}.{ext}",
                file_size=size,
                created_at=f"{year}-{month:02d}-{rng.randint(1, 28):02d}T12:00:00Z",
                # 5% chance to fail once, exercise the limiter shrink path
                fail_probability=0.05 if i > 4 else 0.0,
            )
        )
    return items


async def _fake_download(
    item: FakeItem,
    *,
    sink,
    target_mbps: float,
    rng: random.Random,
) -> str:
    """Simulate downloading one file. Returns status: 'ok' or 'fail'."""
    sink.file_start(item.id, item.filename, item.file_size)

    # Simulate transient failure: spend a few chunks, then "drop"
    will_fail = rng.random() < item.fail_probability
    fail_after = rng.randint(2, 6) if will_fail else None

    chunk_size = 4 * 1024 * 1024  # 4 MB chunks like the real downloader
    bytes_per_second = target_mbps * 1024 * 1024
    sleep_per_chunk = chunk_size / max(bytes_per_second, 1)

    sent = 0
    chunks = 0
    while sent < item.file_size:
        chunk = min(chunk_size, item.file_size - sent)
        await asyncio.sleep(sleep_per_chunk * (0.7 + rng.random() * 0.6))  # jitter
        sink.file_chunk(item.id, chunk)
        sent += chunk
        chunks += 1
        if fail_after is not None and chunks >= fail_after:
            sink.file_done(item.id, "fail", "simulated network drop")
            return "fail"

    sink.file_done(item.id, "ok")
    return "ok"


async def run_demo(
    *,
    count: int,
    initial: int,
    ceiling: int,
    floor: int,
    grow_after: int,
    target_mbps: float,
    console: Console,
) -> None:
    items = _generate_items(count)
    total_bytes = sum(it.file_size for it in items)

    console.print(
        f"[bold]demo:[/] simulating {count} fake items "
        f"(~{total_bytes / 1024**3:.1f} GB) "
        f"at ~{target_mbps:.0f} MB/s per worker, "
        f"5% failure rate after the first 5 items."
    )
    console.print(
        "[dim]watch the [bold]concurrency[/dim] row in the header — "
        "it should ramp up after success streaks and drop on a failure.[/]"
    )

    limiter = AdaptiveLimiter(
        initial=initial, floor=floor, ceiling=ceiling, grow_after=grow_after
    )
    rng = random.Random(42)

    # We can't import FakeItem-vs-MediaItem cleanly because RichProgress only
    # touches expected_size in file_start, so duck-typing is fine.
    with RichProgress(
        console=console,
        limiter=limiter,
        total_items=len(items),
        total_bytes=total_bytes,
        already_done=0,
    ) as ui:

        async def worker(item: FakeItem) -> str:
            async with limiter.slot() as slot:
                status = await _fake_download(
                    item, sink=ui, target_mbps=target_mbps, rng=rng
                )
                slot.success = status == "ok"
                return status

        await asyncio.gather(*[worker(it) for it in items])

    console.print()
    stats = limiter.stats()
    console.print(
        f"[green]✓ demo complete.[/] "
        f"final concurrency: [bold]{stats.target}[/] "
        f"(grew from {initial} → {stats.target} on {stats.successes} successes, "
        f"shrank on {stats.failures} simulated failures)."
    )
