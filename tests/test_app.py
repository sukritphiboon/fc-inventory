"""Tests for the Flask routes and the headless CLI entrypoint.

The collector is always mocked so no FusionCompute connection is attempted.
"""

import argparse

import pytest

import app as app_module
from app import app as flask_app


@pytest.fixture
def client():
    flask_app.config["TESTING"] = True
    # Reset global job state between tests.
    app_module.current_job = {"collector": None, "thread": None, "output_file": None}
    with flask_app.test_client() as c:
        yield c


class FakeThread:
    def __init__(self, alive=True):
        self._alive = alive

    def is_alive(self):
        return self._alive


# ── / and /api/version ──────────────────────────────────────


def test_version_endpoint(client):
    resp = client.get("/api/version")
    assert resp.status_code == 200
    assert resp.get_json()["version"] == app_module.__version__


# ── cross-origin / CSRF protection ──────────────────────────


def test_cross_origin_post_is_rejected(client):
    resp = client.post(
        "/api/collect",
        json={"host": "h", "username": "u", "password": "p"},
        headers={"Origin": "http://evil.example"},
    )
    assert resp.status_code == 403


def test_same_origin_post_is_allowed(client):
    # Matching Origin host passes the guard (then hits normal validation).
    resp = client.post(
        "/api/collect",
        json={"host": "", "username": "", "password": ""},
        headers={"Origin": "http://localhost"},
        base_url="http://localhost",
    )
    # Not a 403 from the origin guard; fails later on missing fields instead.
    assert resp.status_code == 400


def test_get_requests_skip_origin_check(client):
    resp = client.get("/api/progress", headers={"Origin": "http://evil.example"})
    assert resp.status_code == 200


def test_index_renders(client):
    resp = client.get("/")
    assert resp.status_code == 200


def test_changelog_page_renders(client):
    resp = client.get("/changelog")
    assert resp.status_code == 200


# ── helpers ─────────────────────────────────────────────────


def test_find_resource_existing_and_missing(monkeypatch, tmp_path):
    f = tmp_path / "thing.txt"
    f.write_text("hi")
    monkeypatch.setattr(app_module, "_base_dir", str(tmp_path))
    assert app_module._find_resource("thing.txt") == str(f)
    assert app_module._find_resource("nope.txt") is None


def test_get_lan_ip_returns_string():
    ip = app_module._get_lan_ip()
    assert isinstance(ip, str)
    assert ip


# ── /api/collect ────────────────────────────────────────────


def test_collect_requires_fields(client):
    resp = client.post("/api/collect", json={"host": "", "username": "", "password": ""})
    assert resp.status_code == 400
    assert "required" in resp.get_json()["error"].lower()


def test_collect_conflict_when_already_running(client):
    app_module.current_job["thread"] = FakeThread(alive=True)
    resp = client.post("/api/collect",
                       json={"host": "h", "username": "u", "password": "p"})
    assert resp.status_code == 409


def test_collect_rejects_invalid_port(client):
    resp = client.post("/api/collect",
                       json={"host": "h", "username": "u", "password": "p",
                             "port": "notaport"})
    assert resp.status_code == 400


def test_collect_rejects_out_of_range_port(client):
    resp = client.post("/api/collect",
                       json={"host": "h", "username": "u", "password": "p",
                             "port": 70000})
    assert resp.status_code == 400


def test_collect_starts_job(client, monkeypatch):
    started = {}

    class FakeThreadObj:
        def __init__(self, target, args, daemon):
            started["target"] = target
            started["args"] = args
            self.daemon = daemon

        def start(self):
            started["started"] = True

        def is_alive(self):
            return True

    monkeypatch.setattr(app_module.threading, "Thread", FakeThreadObj)
    monkeypatch.setattr(app_module, "InventoryCollector",
                        lambda *a, **kw: object())

    resp = client.post("/api/collect",
                       json={"host": "h", "username": "u", "password": "p", "port": 7443})
    assert resp.status_code == 202
    assert resp.get_json()["status"] == "started"
    assert started["started"] is True


# ── /api/progress ───────────────────────────────────────────


def test_progress_idle_when_no_collector(client):
    resp = client.get("/api/progress")
    assert resp.status_code == 200
    assert resp.get_json()["status"] == "idle"


def test_progress_returns_collector_state(client):
    class FakeCollector:
        progress = {"status": "running", "percent": 42, "current_step": "x", "error": None}

    app_module.current_job["collector"] = FakeCollector()
    resp = client.get("/api/progress")
    body = resp.get_json()
    assert body["status"] == "running"
    assert body["percent"] == 42


# ── /api/cancel ─────────────────────────────────────────────


def test_cancel_no_job(client):
    resp = client.post("/api/cancel")
    assert resp.status_code == 404


def test_cancel_running_job(client):
    class FakeCollector:
        def __init__(self):
            self.cancelled = False

        def cancel(self):
            self.cancelled = True

    collector = FakeCollector()
    app_module.current_job["collector"] = collector
    app_module.current_job["thread"] = FakeThread(alive=True)

    resp = client.post("/api/cancel")
    assert resp.status_code == 200
    assert resp.get_json()["status"] == "cancelling"
    assert collector.cancelled is True


# ── /api/download ───────────────────────────────────────────


def test_download_404_when_no_file(client):
    resp = client.get("/api/download")
    assert resp.status_code == 404


def test_download_sends_file(client, tmp_path):
    f = tmp_path / "FC_Inventory_x.xlsx"
    f.write_bytes(b"PK\x03\x04fakexlsx")
    app_module.current_job["output_file"] = str(f)

    resp = client.get("/api/download")
    assert resp.status_code == 200
    assert b"fakexlsx" in resp.data


# ── /api/update-check ───────────────────────────────────────


def test_update_check_disabled_by_env(client, monkeypatch):
    monkeypatch.setenv("FC_INVENTORY_DISABLE_UPDATE_CHECK", "1")
    resp = client.get("/api/update-check")
    body = resp.get_json()
    assert body["enabled"] is False


def test_update_check_reports_available(client, monkeypatch):
    monkeypatch.delenv("FC_INVENTORY_DISABLE_UPDATE_CHECK", raising=False)
    monkeypatch.setattr(app_module, "get_latest_release",
                        lambda repo: {"tag": "v99.0.0", "url": "https://x"})
    resp = client.get("/api/update-check")
    body = resp.get_json()
    assert body["update_available"] is True
    assert body["latest"] == "99.0.0"


def test_update_check_degrades_on_error(client, monkeypatch):
    monkeypatch.delenv("FC_INVENTORY_DISABLE_UPDATE_CHECK", raising=False)

    def boom(repo):
        raise RuntimeError("offline")

    monkeypatch.setattr(app_module, "get_latest_release", boom)
    resp = client.get("/api/update-check")
    assert resp.status_code == 200
    body = resp.get_json()
    assert body["enabled"] is True
    assert "error" in body


# ── /api/changelog ──────────────────────────────────────────


def test_changelog_returns_content(client):
    resp = client.get("/api/changelog")
    assert resp.status_code == 200
    assert "Content-Type" in resp.headers
    # Repo ships a CHANGELOG.md, so we expect real content.
    assert len(resp.get_data(as_text=True)) > 0


def test_changelog_404_when_missing(client, monkeypatch):
    monkeypatch.setattr(app_module, "_find_resource", lambda name: None)
    resp = client.get("/api/changelog")
    assert resp.status_code == 404


# ── _run_collection (background thread body) ────────────────


def test_run_collection_success_builds_excel(monkeypatch, tmp_path):
    monkeypatch.setattr(app_module, "_base_dir", str(tmp_path))
    built = {}

    class FakeCollector:
        progress = {"status": "running"}

        def collect_all(self):
            return {"vInfo": []}

    monkeypatch.setattr(app_module, "build_excel",
                        lambda data, path: built.update({"path": path, "data": data}))

    app_module._run_collection(FakeCollector())
    assert built["path"].endswith(".xlsx")
    assert app_module.current_job["output_file"] == built["path"]


def test_run_collection_handles_interrupt(monkeypatch):
    class FakeCollector:
        progress = {"status": "running"}

        def collect_all(self):
            raise InterruptedError()

    c = FakeCollector()
    app_module._run_collection(c)
    assert c.progress["status"] == "cancelled"


def test_run_collection_handles_error(monkeypatch):
    class FakeCollector:
        progress = {"status": "running"}

        def collect_all(self):
            raise RuntimeError("kaboom")

    c = FakeCollector()
    app_module._run_collection(c)
    assert c.progress["status"] == "error"
    assert "kaboom" in c.progress["error"]


# ── run_headless ────────────────────────────────────────────


def _headless_args(**overrides):
    defaults = dict(host="h", username="u", password="p", port=7443, out=None)
    defaults.update(overrides)
    return argparse.Namespace(**defaults)


def test_run_headless_missing_password_returns_2(monkeypatch):
    monkeypatch.delenv("FC_INVENTORY_PASSWORD", raising=False)
    monkeypatch.setattr(app_module.getpass, "getpass", lambda *a, **k: "")
    code = app_module.run_headless(_headless_args(password=None))
    assert code == 2


def test_run_headless_success(monkeypatch, tmp_path, capsys):
    out = tmp_path / "result.xlsx"

    class FakeCollector:
        def __init__(self, *a, **kw):
            pass

        def collect_all(self):
            return {"vInfo": []}

    monkeypatch.setattr(app_module, "InventoryCollector", FakeCollector)
    monkeypatch.setattr(app_module, "build_excel", lambda data, path: None)

    code = app_module.run_headless(_headless_args(out=str(out)))
    assert code == 0
    # Path is printed on its own line for scripts to capture.
    assert str(out) in capsys.readouterr().out


def test_run_headless_password_from_env(monkeypatch, tmp_path):
    monkeypatch.setenv("FC_INVENTORY_PASSWORD", "envpass")
    seen = {}

    class FakeCollector:
        def __init__(self, host, username, password, port=7443):
            seen["password"] = password

        def collect_all(self):
            return {}

    monkeypatch.setattr(app_module, "InventoryCollector", FakeCollector)
    monkeypatch.setattr(app_module, "build_excel", lambda data, path: None)

    code = app_module.run_headless(_headless_args(password=None, out=str(tmp_path / "o.xlsx")))
    assert code == 0
    assert seen["password"] == "envpass"


def test_run_headless_collection_failure_returns_1(monkeypatch):
    class FakeCollector:
        def __init__(self, *a, **kw):
            pass

        def collect_all(self):
            raise RuntimeError("boom")

    monkeypatch.setattr(app_module, "InventoryCollector", FakeCollector)
    code = app_module.run_headless(_headless_args())
    assert code == 1


def test_run_headless_keyboard_interrupt_returns_130(monkeypatch):
    class FakeCollector:
        def __init__(self, *a, **kw):
            pass

        def collect_all(self):
            raise KeyboardInterrupt()

    monkeypatch.setattr(app_module, "InventoryCollector", FakeCollector)
    code = app_module.run_headless(_headless_args())
    assert code == 130


# ── arg parser ──────────────────────────────────────────────


def test_arg_parser_collect_subcommand():
    parser = app_module._build_arg_parser()
    args = parser.parse_args(["collect", "--host", "h", "--username", "u"])
    assert args.command == "collect"
    assert args.host == "h"
    assert args.port == 7443


def test_arg_parser_web_default():
    parser = app_module._build_arg_parser()
    args = parser.parse_args([])
    assert args.command is None
