"""Per-file marker store. Each completed (or deliberately skipped) media item
gets a JSON marker keyed by its media_id. Re-running skips items with markers.
"""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any


@dataclass(slots=True)
class Marker:
    media_id: str
    status: str  # "ok" | "skipped"
    filename: str | None = None
    file_size: int | None = None
    created_at: str | None = None
    saved: list[str] = field(default_factory=list)
    reason: str | None = None  # populated for "skipped"

    def to_json(self) -> str:
        d = {k: v for k, v in asdict(self).items() if v is not None or k == "saved"}
        return json.dumps(d, indent=2)


class MarkerStore:
    def __init__(self, dir_: Path) -> None:
        self.dir = dir_
        self.dir.mkdir(parents=True, exist_ok=True)

    def path(self, media_id: str) -> Path:
        return self.dir / f"{media_id}.json"

    def has(self, media_id: str) -> bool:
        return self.path(media_id).exists()

    def write(self, marker: Marker) -> None:
        self.path(marker.media_id).write_text(marker.to_json())

    def read(self, media_id: str) -> dict[str, Any] | None:
        p = self.path(media_id)
        if not p.exists():
            return None
        return json.loads(p.read_text())

    def all_ids(self) -> set[str]:
        return {p.stem for p in self.dir.glob("*.json")}
