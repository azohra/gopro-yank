"""Loading auth credentials from .env files or environment variables."""

from __future__ import annotations

import os
from pathlib import Path


def load_env_file(path: Path) -> None:
    """Best-effort .env loader. Sets vars in os.environ only if not already set."""
    if not path.exists():
        return
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        os.environ.setdefault(k.strip(), v.strip().strip('"').strip("'"))


def get_credentials(env_file: Path | None = None) -> tuple[str, str]:
    """Return (auth_token, user_id) from env. Raises RuntimeError if missing."""
    if env_file:
        load_env_file(env_file)
    token = os.environ.get("AUTH_TOKEN")
    user_id = os.environ.get("USER_ID")
    if not token or not user_id:
        raise RuntimeError(
            "missing AUTH_TOKEN and/or USER_ID. set env vars or use --env-file."
        )
    return token, user_id
