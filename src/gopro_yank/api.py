"""Async client for the (undocumented) GoPro Plus API.

Three endpoints are all we need:
  GET /media/user                   — auth check
  GET /media/search                 — paginated listing
  GET /media/x/zip/source?ids=...   — zip stream of one or more media items

The /zip/source endpoint generates the zip on the fly, doesn't support HTTP
Range, and so doesn't survive a mid-stream connection drop. We mitigate by
requesting one ID per zip — a drop only costs that file.
"""

from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Any

import httpx

API_BASE = "https://api.gopro.com"
DEFAULT_HEADERS = {
    "Accept": "application/vnd.gopro.jk.media+json; version=2.0.0",
    "User-Agent": (
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/131.0.0.0 Safari/537.36"
    ),
}


@dataclass(slots=True, frozen=True)
class MediaItem:
    """Minimal fields we care about from /media/search."""

    id: str
    filename: str | None
    file_size: int | None
    created_at: str | None
    file_extension: str | None
    raw: dict[str, Any]

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> MediaItem:
        return cls(
            id=d["id"],
            filename=d.get("filename"),
            file_size=d.get("file_size"),
            created_at=d.get("created_at"),
            file_extension=d.get("file_extension"),
            raw=d,
        )


class AuthError(Exception):
    """Auth token is missing, expired, or invalid (HTTP 401)."""


class GoProClient:
    """Thin async client around the GoPro Plus API."""

    def __init__(
        self,
        auth_token: str,
        user_id: str,
        *,
        max_connections: int = 32,
        timeout: float = 600.0,
    ) -> None:
        self.auth_token = auth_token
        self.user_id = user_id
        limits = httpx.Limits(
            max_connections=max_connections,
            max_keepalive_connections=max_connections,
        )
        self._client = httpx.AsyncClient(
            http2=True,
            headers=DEFAULT_HEADERS,
            cookies={"gp_access_token": auth_token, "gp_user_id": user_id},
            timeout=httpx.Timeout(connect=30.0, read=timeout, write=30.0, pool=10.0),
            limits=limits,
            follow_redirects=True,
        )

    async def __aenter__(self) -> GoProClient:
        return self

    async def __aexit__(self, *_: object) -> None:
        await self.aclose()

    async def aclose(self) -> None:
        await self._client.aclose()

    async def validate(self) -> dict[str, Any]:
        r = await self._client.get(f"{API_BASE}/media/user")
        if r.status_code == 401:
            raise AuthError("HTTP 401 — refresh gp_access_token and gp_user_id cookies")
        r.raise_for_status()
        return r.json()

    async def iter_media(self, per_page: int = 30) -> AsyncIterator[MediaItem]:
        """Yield every MediaItem in the user's library, paginated."""
        page = 1
        total_pages = None
        while True:
            r = await self._client.get(
                f"{API_BASE}/media/search",
                params={
                    "per_page": per_page,
                    "page": page,
                    "fields": "id,created_at,filename,file_extension,file_size,type",
                },
            )
            if r.status_code == 401:
                raise AuthError("HTTP 401 during /media/search")
            r.raise_for_status()
            body = r.json()
            for raw in body["_embedded"]["media"]:
                yield MediaItem.from_dict(raw)
            if total_pages is None:
                total_pages = body.get("_pages", {}).get("total_pages", page)
            if page >= total_pages:
                return
            page += 1

    async def list_all(self, per_page: int = 30) -> list[MediaItem]:
        return [item async for item in self.iter_media(per_page=per_page)]

    async def get_media(self, media_id: str) -> dict[str, Any]:
        """Full record for a single media item (used to identify skip-worthy types)."""
        r = await self._client.get(f"{API_BASE}/media/{media_id}")
        if r.status_code == 401:
            raise AuthError(f"HTTP 401 on /media/{media_id}")
        r.raise_for_status()
        return r.json()

    async def stream_source_zip(
        self,
        media_id: str,
        on_chunk,
        *,
        chunk_size: int = 4 * 1024 * 1024,
    ) -> int:
        """Stream the source-quality zip for one media id; call on_chunk(bytes).

        Returns total bytes downloaded. Raises AuthError on 401, httpx errors otherwise.
        """
        url = f"{API_BASE}/media/x/zip/source"
        params = {"ids": media_id, "access_token": self.auth_token}
        total = 0
        async with self._client.stream("GET", url, params=params) as r:
            if r.status_code == 401:
                raise AuthError(f"HTTP 401 on /zip/source for {media_id}")
            r.raise_for_status()
            async for chunk in r.aiter_bytes(chunk_size=chunk_size):
                if not chunk:
                    continue
                on_chunk(chunk)
                total += len(chunk)
        return total


async def _smoke() -> None:  # pragma: no cover
    import os

    async with GoProClient(os.environ["AUTH_TOKEN"], os.environ["USER_ID"]) as c:
        print(await c.validate())


if __name__ == "__main__":  # pragma: no cover
    asyncio.run(_smoke())
