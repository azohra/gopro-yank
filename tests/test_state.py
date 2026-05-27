"""Tests for the marker store."""

import json
from pathlib import Path

from gopro_yank.state import Marker, MarkerStore


def test_marker_roundtrip(tmp_path: Path) -> None:
    s = MarkerStore(tmp_path / "state")
    m = Marker(
        media_id="abc123",
        status="ok",
        filename="GX010001.MP4",
        file_size=12345,
        created_at="2024-05-10T04:08:26Z",
        saved=["2024/05/GX010001.MP4"],
    )
    assert not s.has("abc123")
    s.write(m)
    assert s.has("abc123")
    data = s.read("abc123")
    assert data == {
        "media_id": "abc123",
        "status": "ok",
        "filename": "GX010001.MP4",
        "file_size": 12345,
        "created_at": "2024-05-10T04:08:26Z",
        "saved": ["2024/05/GX010001.MP4"],
    }


def test_all_ids(tmp_path: Path) -> None:
    s = MarkerStore(tmp_path / "state")
    for mid in ("a", "b", "c"):
        s.write(Marker(media_id=mid, status="ok"))
    assert s.all_ids() == {"a", "b", "c"}


def test_skipped_marker_serializes_reason(tmp_path: Path) -> None:
    s = MarkerStore(tmp_path / "state")
    s.write(Marker(media_id="skip-me", status="skipped", reason="MultiClipEdit"))
    raw = json.loads((s.dir / "skip-me.json").read_text())
    assert raw["status"] == "skipped"
    assert raw["reason"] == "MultiClipEdit"
