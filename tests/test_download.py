"""Tests for the helpers in download.py."""

from gopro_yank.download import yyyy_mm


def test_yyyy_mm_iso() -> None:
    assert yyyy_mm("2024-05-10T04:08:26Z") == "2024/05"


def test_yyyy_mm_missing() -> None:
    assert yyyy_mm(None) == "_unsorted"
    assert yyyy_mm("") == "_unsorted"


def test_yyyy_mm_short_string() -> None:
    assert yyyy_mm("2024") == "_unsorted"


def test_yyyy_mm_january() -> None:
    assert yyyy_mm("2022-01-15T12:00:00Z") == "2022/01"
