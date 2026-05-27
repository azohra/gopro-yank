"""Tests for the `login` command.

We force `--paste` mode so the tests don't depend on whatever's in the dev's
real clipboard. _validate_creds is mocked since we can't reach the real API.
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
            ["login", "--env-file", str(env_file), "--no-browser", "--paste"],
            # paste value, confirm "looks right?", paste user_id, confirm
            input="eyJ_TEST_JWT\ny\nUSER_VALUE\ny\n",
        )
    assert result.exit_code == 0, result.output
    body = env_file.read_text()
    assert "AUTH_TOKEN=eyJ_TEST_JWT" in body
    assert "USER_ID=USER_VALUE" in body
    assert env_file.stat().st_mode & 0o777 == 0o600


def test_login_refuses_overwrite_without_confirm(tmp_path: Path) -> None:
    env_file = tmp_path / ".env"
    env_file.write_text("AUTH_TOKEN=existing\nUSER_ID=existing\n")
    runner = CliRunner()
    result = runner.invoke(
        main,
        ["login", "--env-file", str(env_file), "--no-browser", "--paste"],
        input="n\n",
    )
    assert result.exit_code != 0
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
            ["login", "--env-file", str(env_file), "--no-browser", "--paste", "--force"],
            input="eyJ_NEW_JWT\ny\nNEW_USER\ny\n",
        )
    assert result.exit_code == 0, result.output
    assert "eyJ_NEW_JWT" in env_file.read_text()
    assert "old" not in env_file.read_text()


def test_login_exits_on_auth_error(tmp_path: Path) -> None:
    env_file = tmp_path / ".env"

    async def fake_validate(_token: str, _user_id: str) -> dict:
        raise AuthError("HTTP 401")

    runner = CliRunner()
    with patch("gopro_yank.cli._validate_creds", side_effect=fake_validate):
        result = runner.invoke(
            main,
            ["login", "--env-file", str(env_file), "--no-browser", "--paste"],
            input="eyJ_BAD\ny\nBAD\ny\n",
        )
    assert result.exit_code != 0
    assert "rejected by GoPro" in result.output
    assert not env_file.exists()
