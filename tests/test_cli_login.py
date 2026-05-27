"""Tests for the `login` command. Heavy mocking — we can't hit the live API.

We exercise:
  - refusing to overwrite an existing env file without --force or confirmation
  - writing the env file with mode 600 after a successful validation
  - exiting nonzero on AuthError from validation
"""

from __future__ import annotations

from pathlib import Path
from unittest.mock import patch

from click.testing import CliRunner

from gopro_yank.api import AuthError
from gopro_yank.cli import main


def test_login_writes_env_on_success(tmp_path: Path) -> None:
    env_file = tmp_path / ".env"
    fake_user = {"email": "j@example.com", "id": "u-1"}

    async def fake_validate(_token: str, _user_id: str) -> dict:
        return fake_user

    runner = CliRunner()
    with patch("gopro_yank.cli._validate_creds", side_effect=fake_validate):
        result = runner.invoke(
            main,
            ["login", "--env-file", str(env_file), "--no-browser"],
            input="JWT_VALUE\nUSER_VALUE\n",
        )
    assert result.exit_code == 0, result.output
    body = env_file.read_text()
    assert "AUTH_TOKEN=JWT_VALUE" in body
    assert "USER_ID=USER_VALUE" in body
    # Mode 600
    assert env_file.stat().st_mode & 0o777 == 0o600


def test_login_refuses_overwrite_without_confirm(tmp_path: Path) -> None:
    env_file = tmp_path / ".env"
    env_file.write_text("AUTH_TOKEN=existing\nUSER_ID=existing\n")
    runner = CliRunner()
    # Answer 'n' to the overwrite confirmation
    result = runner.invoke(
        main,
        ["login", "--env-file", str(env_file), "--no-browser"],
        input="n\n",
    )
    assert result.exit_code != 0
    # Untouched
    assert "existing" in env_file.read_text()


def test_login_force_overwrites(tmp_path: Path) -> None:
    env_file = tmp_path / ".env"
    env_file.write_text("AUTH_TOKEN=old\nUSER_ID=old\n")

    async def fake_validate(_token: str, _user_id: str) -> dict:
        return {"email": "j@example.com"}

    runner = CliRunner()
    with patch("gopro_yank.cli._validate_creds", side_effect=fake_validate):
        result = runner.invoke(
            main,
            ["login", "--env-file", str(env_file), "--no-browser", "--force"],
            input="NEW_JWT\nNEW_USER\n",
        )
    assert result.exit_code == 0, result.output
    assert "NEW_JWT" in env_file.read_text()
    assert "old" not in env_file.read_text()


def test_login_exits_on_auth_error(tmp_path: Path) -> None:
    env_file = tmp_path / ".env"

    async def fake_validate(_token: str, _user_id: str) -> dict:
        raise AuthError("HTTP 401")

    runner = CliRunner()
    with patch("gopro_yank.cli._validate_creds", side_effect=fake_validate):
        result = runner.invoke(
            main,
            ["login", "--env-file", str(env_file), "--no-browser"],
            input="bad\nbad\n",
        )
    assert result.exit_code != 0
    assert "rejected by GoPro" in result.output
    # No file should have been created
    assert not env_file.exists()
