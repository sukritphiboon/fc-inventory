"""Tests for version parsing/comparison helpers."""

from version_utils import parse_version, is_newer


def test_parse_version_basic():
    assert parse_version("1.2.3") == (1, 2, 3)
    assert parse_version("v1.2.3") == (1, 2, 3)


def test_parse_version_pads_missing_parts():
    assert parse_version("1.1") == (1, 1, 0)
    assert parse_version("2") == (2, 0, 0)


def test_parse_version_ignores_prerelease_suffix():
    assert parse_version("2.0.0-rc.1") == (2, 0, 0)
    assert parse_version("1.4.0+build7") == (1, 4, 0)


def test_parse_version_handles_garbage():
    assert parse_version("") == (0, 0, 0)
    assert parse_version(None) == (0, 0, 0)


def test_is_newer():
    assert is_newer("1.1.0", "1.0.0") is True
    assert is_newer("v1.0.1", "1.0.0") is True
    assert is_newer("2.0.0", "1.9.9") is True


def test_is_not_newer_for_same_or_older():
    assert is_newer("1.0.0", "1.0.0") is False
    assert is_newer("1.0.0", "1.1.0") is False
    assert is_newer("1.0.0", "v1.0.0") is False
