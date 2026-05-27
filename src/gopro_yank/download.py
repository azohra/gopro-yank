"""Per-file download + extract pipeline."""

from __future__ import annotations

import asyncio
import zipfile
from dataclasses import dataclass
from pathlib import Path
from typing import Protocol

from gopro_yank.api import AuthError, GoProClient, MediaItem
from gopro_yank.state import Marker, MarkerStore


class ProgressSink(Protocol):
    """Callback interface for download progress updates."""

    def file_start(self, media_id: str, filename: str, expected_size: int | None) -> None: ...
    def file_chunk(self, media_id: str, chunk_size: int) -> None: ...
    def file_done(self, media_id: str, status: str, info: str = "") -> None: ...


class _NullSink:
    def file_start(self, *_: object) -> None:  # noqa: D401
        pass

    def file_chunk(self, *_: object) -> None:
        pass

    def file_done(self, *_: object) -> None:
        pass


@dataclass(slots=True, frozen=True)
class DownloadResult:
    media_id: str
    status: str  # "ok" | "skip" | "fail" | "auth"
    info: str
    bytes_done: int = 0


def yyyy_mm(created_at: str | None) -> str:
    if not created_at or len(created_at) < 7:
        return "_unsorted"
    return f"{created_at[:4]}/{created_at[5:7]}"


async def _extract_zip_async(zip_path: Path, out_dir: Path) -> list[str]:
    """Extract sync inside a thread; preserves zip's internal layout."""

    def _do() -> list[str]:
        saved: list[str] = []
        with zipfile.ZipFile(zip_path) as zf:
            for info in zf.infolist():
                if info.is_dir():
                    continue
                # Flatten just the directory; keep filename as-is.
                base = Path(info.filename).name
                target = out_dir / base
                tmp = target.with_suffix(target.suffix + ".part")
                if target.exists() and target.stat().st_size == info.file_size:
                    saved.append(base)
                    continue
                with zf.open(info) as src, open(tmp, "wb") as dst:
                    while True:
                        c = src.read(4 * 1024 * 1024)
                        if not c:
                            break
                        dst.write(c)
                tmp.rename(target)
                saved.append(base)
        return saved

    return await asyncio.to_thread(_do)


async def download_one(
    client: GoProClient,
    item: MediaItem,
    out_root: Path,
    state: MarkerStore,
    *,
    sink: ProgressSink | None = None,
    max_attempts: int = 5,
    chunk_size: int = 4 * 1024 * 1024,
) -> DownloadResult:
    """Download one media item and extract it. Idempotent — re-running returns 'skip'."""
    sink = sink or _NullSink()
    if state.has(item.id):
        return DownloadResult(item.id, "skip", "marker exists")

    out_sub = out_root / yyyy_mm(item.created_at)
    out_sub.mkdir(parents=True, exist_ok=True)
    tmp_zip = state.dir / f"{item.id}.zip.part"

    sink.file_start(item.id, item.filename or item.id, item.file_size)

    last_err: str | None = None
    for attempt in range(max_attempts):
        try:
            written = 0

            def on_chunk(b: bytes) -> None:
                nonlocal written
                # We write to disk via an outer file handle; bound here.
                fh.write(b)
                written += len(b)
                sink.file_chunk(item.id, len(b))

            with open(tmp_zip, "wb") as fh:
                await client.stream_source_zip(item.id, on_chunk, chunk_size=chunk_size)

            saved = await _extract_zip_async(tmp_zip, out_sub)
            tmp_zip.unlink(missing_ok=True)

            state.write(
                Marker(
                    media_id=item.id,
                    status="ok",
                    filename=item.filename,
                    file_size=item.file_size,
                    created_at=item.created_at,
                    saved=[f"{yyyy_mm(item.created_at)}/{n}" for n in saved],
                )
            )
            sink.file_done(item.id, "ok")
            return DownloadResult(item.id, "ok", "", bytes_done=written)

        except AuthError as e:
            tmp_zip.unlink(missing_ok=True)
            sink.file_done(item.id, "auth", str(e))
            return DownloadResult(item.id, "auth", str(e))

        except zipfile.BadZipFile as e:
            tmp_zip.unlink(missing_ok=True)
            last_err = f"bad zip: {e}"
            await asyncio.sleep(min(30, 2 ** attempt))
            continue

        except Exception as e:  # noqa: BLE001 — network / IO errors
            tmp_zip.unlink(missing_ok=True)
            last_err = repr(e)
            await asyncio.sleep(min(30, 2 ** attempt))
            continue

    sink.file_done(item.id, "fail", last_err or "unknown")
    return DownloadResult(item.id, "fail", last_err or "unknown")
