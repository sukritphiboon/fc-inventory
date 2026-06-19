"""Tests for the FusionCompute REST client.

All network I/O is faked: each test installs a lightweight stand-in for the
``requests.Session`` the client builds in ``__init__``. No live VRM is needed.
"""

import requests
import pytest

from fc_client import FCClient


# ── Fakes ────────────────────────────────────────────────────


class FakeResponse:
    def __init__(self, status_code=200, headers=None, json_data=None, text=""):
        self.status_code = status_code
        self.headers = headers or {}
        self._json = json_data
        self.text = text
        self.content = text.encode("utf-8")

    def json(self):
        if self._json is None:
            raise ValueError("no json")
        return self._json

    def raise_for_status(self):
        if self.status_code >= 400:
            raise requests.exceptions.HTTPError(f"HTTP {self.status_code}")


class FakeSession:
    """Records calls and returns queued/scripted responses."""

    def __init__(self):
        self.headers = {}
        self.verify = True
        self.calls = []
        # Optional scripted handlers keyed by HTTP method (callables).
        self.post_handler = None
        self.put_handler = None
        self.get_handler = None
        self.delete_handler = None

    def _dispatch(self, handler, method, url, **kwargs):
        self.calls.append((method, url, kwargs))
        if handler is None:
            return FakeResponse(200, json_data={})
        result = handler(url, **kwargs)
        if isinstance(result, Exception):
            raise result
        return result

    def post(self, url, **kwargs):
        return self._dispatch(self.post_handler, "POST", url, **kwargs)

    def put(self, url, **kwargs):
        return self._dispatch(self.put_handler, "PUT", url, **kwargs)

    def get(self, url, **kwargs):
        return self._dispatch(self.get_handler, "GET", url, **kwargs)

    def delete(self, url, **kwargs):
        return self._dispatch(self.delete_handler, "DELETE", url, **kwargs)


def _make_client(**kwargs):
    client = FCClient("10.0.0.1", "admin", "secret", **kwargs)
    client.session = FakeSession()
    return client


# ── __init__ / host normalization ───────────────────────────


def test_init_strips_scheme_and_trailing_slash():
    c = FCClient("https://10.0.0.1/", "u", "p")
    assert c.host == "10.0.0.1"
    assert c.base_url == "https://10.0.0.1:7443"

    c2 = FCClient("http://vrm.example/", "u", "p", port=8443)
    assert c2.host == "vrm.example"
    assert c2.base_url == "https://vrm.example:8443"


def test_sha256_is_hex_digest():
    c = _make_client()
    digest = c._sha256("secret")
    assert len(digest) == 64
    assert all(ch in "0123456789abcdef" for ch in digest)


# ── login() ─────────────────────────────────────────────────


def test_login_success_token_in_header():
    c = _make_client()

    def handler(url, **kwargs):
        return FakeResponse(200, headers={"X-Auth-Token": "tok-123"},
                            json_data={"hello": "world"})

    c.session.post_handler = handler

    result = c.login()
    assert c.token == "tok-123"
    assert c.session.headers["X-Auth-Token"] == "tok-123"
    assert c.base_url.endswith("/service")
    assert result == {"hello": "world"}


def test_login_success_token_in_body():
    c = _make_client()

    def handler(url, **kwargs):
        # No token header -> must fall back to body.
        return FakeResponse(200, headers={}, json_data={"accessSession": "body-tok"})

    c.session.post_handler = handler

    c.login()
    assert c.token == "body-tok"


def test_login_skips_to_next_version_on_version_error():
    c = _make_client()
    seen_versions = []

    def handler(url, **kwargs):
        ver = c.session.headers["Accept"]
        seen_versions.append(ver)
        # First version rejected with the version-error code, then succeed.
        if "v8.0" in ver:
            return FakeResponse(403, text='{"errorCode":"10000022"}')
        return FakeResponse(200, headers={"X-Auth-Token": "ok"})

    c.session.post_handler = handler

    c.login()
    assert c.token == "ok"
    # The v8.0 attempt should not have been retried with other auth methods:
    # exactly one v8.0 call before moving on.
    v8_calls = [v for v in seen_versions if "v8.0" in v]
    assert len(v8_calls) == 1


def test_login_all_methods_fail_raises_connection_error():
    c = _make_client()

    def handler(url, **kwargs):
        return FakeResponse(401, text="unauthorized")

    c.session.post_handler = handler
    c.session.put_handler = handler

    with pytest.raises(ConnectionError):
        c.login()


def test_login_connection_refused_moves_to_next_port():
    c = _make_client(port=9999)
    attempted_ports = []

    def handler(url, **kwargs):
        # Record the port from the URL and always refuse.
        attempted_ports.append(url.split(":")[2].split("/")[0])
        raise requests.exceptions.ConnectionError("refused")

    c.session.post_handler = handler
    c.session.put_handler = handler

    with pytest.raises(ConnectionError):
        c.login()

    # Should have tried the requested port plus the documented fallbacks.
    assert "9999" in attempted_ports
    assert "7443" in attempted_ports
    assert "8443" in attempted_ports


# ── _extract_token ──────────────────────────────────────────


def test_extract_token_prefers_header():
    c = _make_client()
    resp = FakeResponse(200, headers={"X-Auth-Token": "h"}, json_data={"token": "b"})
    c._extract_token(resp, "label")
    assert c.token == "h"


def test_extract_token_body_fallback_keys():
    c = _make_client()
    resp = FakeResponse(200, headers={}, json_data={"token": "from-body"})
    c._extract_token(resp, "label")
    assert c.token == "from-body"


def test_extract_token_missing_raises():
    c = _make_client()
    resp = FakeResponse(200, headers={}, json_data={"nope": 1})
    with pytest.raises(ConnectionError):
        c._extract_token(resp, "label")


# ── logout ──────────────────────────────────────────────────


def test_logout_clears_token_and_swallows_errors():
    c = _make_client()
    c.token = "tok"

    def handler(url, **kwargs):
        raise requests.exceptions.ConnectionError("down")

    c.session.delete_handler = handler
    c.logout()  # must not raise
    assert c.token is None


# ── _get / URL construction ─────────────────────────────────


def test_get_uses_base_url_for_relative_path():
    c = _make_client()
    c.base_url = "https://10.0.0.1:7443/service"
    captured = {}

    def handler(url, **kwargs):
        captured["url"] = url
        return FakeResponse(200, json_data={"ok": True})

    c.session.get_handler = handler
    assert c._get("/sites") == {"ok": True}
    assert captured["url"] == "https://10.0.0.1:7443/service/sites"


def test_get_uses_host_port_for_service_prefixed_path():
    c = _make_client()
    captured = {}

    def handler(url, **kwargs):
        captured["url"] = url
        return FakeResponse(200, json_data={})

    c.session.get_handler = handler
    c._get("/service/vms/123")
    assert captured["url"] == "https://10.0.0.1:7443/service/vms/123"


def test_get_raises_on_http_error():
    c = _make_client()
    c.session.get_handler = lambda url, **kw: FakeResponse(500, text="boom")
    with pytest.raises(requests.exceptions.HTTPError):
        c._get("/sites")


# ── _get_all pagination ─────────────────────────────────────


def test_get_all_accumulates_pages(monkeypatch):
    c = _make_client()
    pages = [
        {"clusters": [{"id": 1}, {"id": 2}], "total": 3},
        {"clusters": [{"id": 3}], "total": 3},
    ]
    seen_offsets = []

    def fake_get(path, params=None):
        seen_offsets.append(params["offset"])
        return pages[len(seen_offsets) - 1]

    monkeypatch.setattr(c, "_get", fake_get)
    items = c._get_all("/site/clusters", "clusters")
    assert [i["id"] for i in items] == [1, 2, 3]
    assert seen_offsets == [0, 100]


def test_get_all_falls_back_to_items_key(monkeypatch):
    c = _make_client()
    monkeypatch.setattr(c, "_get",
                        lambda path, params=None: {"items": [{"id": 1}], "total": 1})
    assert c._get_all("/p", "clusters") == [{"id": 1}]


def test_get_all_falls_back_to_result_key(monkeypatch):
    c = _make_client()
    monkeypatch.setattr(c, "_get",
                        lambda path, params=None: {"result": [{"id": 9}], "total": 1})
    assert c._get_all("/p", "clusters") == [{"id": 9}]


def test_get_all_stops_on_empty_batch(monkeypatch):
    c = _make_client()
    monkeypatch.setattr(c, "_get", lambda path, params=None: {"clusters": []})
    assert c._get_all("/p", "clusters") == []


# ── Getters with fallback keys ──────────────────────────────


def test_get_sites(monkeypatch):
    c = _make_client()
    monkeypatch.setattr(c, "_get", lambda path, params=None: {"sites": [{"uri": "/s1"}]})
    assert c.get_sites() == [{"uri": "/s1"}]


def test_get_vm_disks_volumes_first(monkeypatch):
    c = _make_client()
    monkeypatch.setattr(c, "_get", lambda path: {"volumes": [{"id": "v1"}]})
    assert c.get_vm_disks("/vm/1") == [{"id": "v1"}]


def test_get_vm_disks_falls_back_to_disks(monkeypatch):
    c = _make_client()
    calls = {"n": 0}

    def fake_get(path):
        calls["n"] += 1
        if path.endswith("/volumes"):
            raise requests.exceptions.HTTPError("404")
        return {"disks": [{"id": "d1"}]}

    monkeypatch.setattr(c, "_get", fake_get)
    assert c.get_vm_disks("/vm/1") == [{"id": "d1"}]


def test_get_vm_disks_returns_empty_when_both_fail(monkeypatch):
    c = _make_client()

    def fake_get(path):
        raise requests.exceptions.HTTPError("404")

    monkeypatch.setattr(c, "_get", fake_get)
    assert c.get_vm_disks("/vm/1") == []


def test_get_dvswitches_key_fallbacks(monkeypatch):
    c = _make_client()
    monkeypatch.setattr(c, "_get", lambda path: {"dvSwitchs": [{"name": "dvs1"}]})
    assert c.get_dvswitches("/site") == [{"name": "dvs1"}]


def test_get_portgroups_key_fallback(monkeypatch):
    c = _make_client()
    monkeypatch.setattr(c, "_get", lambda path: {"portGroups": [{"name": "pg1"}]})
    assert c.get_portgroups("/dvs/1") == [{"name": "pg1"}]


def test_get_site_portgroups_swallows_errors(monkeypatch):
    c = _make_client()

    def fake_get(path):
        raise requests.exceptions.HTTPError("500")

    monkeypatch.setattr(c, "_get", fake_get)
    assert c.get_site_portgroups("/site") == []
