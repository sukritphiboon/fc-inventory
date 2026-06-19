"""Tests for the GitHub-release lookup wrapper in version_utils.

The HTTP call is faked via monkeypatching ``version_utils.requests.get``.
"""

import requests
import pytest

import version_utils
from version_utils import get_latest_release


class FakeResponse:
    def __init__(self, json_data=None, status_code=200):
        self._json = json_data or {}
        self.status_code = status_code

    def json(self):
        return self._json

    def raise_for_status(self):
        if self.status_code >= 400:
            raise requests.exceptions.HTTPError(f"HTTP {self.status_code}")


def test_get_latest_release_parses_tag_and_url(monkeypatch):
    def fake_get(url, **kwargs):
        assert "releases/latest" in url
        return FakeResponse({"tag_name": "v2.3.0", "html_url": "https://x/rel"})

    monkeypatch.setattr(version_utils.requests, "get", fake_get)
    result = get_latest_release("owner/repo")
    assert result == {"tag": "v2.3.0", "url": "https://x/rel"}


def test_get_latest_release_missing_keys_default_to_empty(monkeypatch):
    monkeypatch.setattr(version_utils.requests, "get",
                        lambda url, **kw: FakeResponse({}))
    result = get_latest_release("owner/repo")
    assert result == {"tag": "", "url": ""}


def test_get_latest_release_raises_on_http_error(monkeypatch):
    monkeypatch.setattr(version_utils.requests, "get",
                        lambda url, **kw: FakeResponse(status_code=404))
    with pytest.raises(requests.exceptions.HTTPError):
        get_latest_release("owner/repo")


def test_get_latest_release_propagates_network_error(monkeypatch):
    def boom(url, **kwargs):
        raise requests.exceptions.ConnectionError("offline")

    monkeypatch.setattr(version_utils.requests, "get", boom)
    with pytest.raises(requests.exceptions.ConnectionError):
        get_latest_release("owner/repo")
