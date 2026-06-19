"""
FC Inventory Tool - Flask Application
Web-based FusionCompute inventory collector with Excel export.
"""

import os
import sys
import socket
import argparse
import getpass
import logging
import logging.handlers
import threading
import webbrowser
from datetime import datetime
from urllib.parse import urlparse

from flask import Flask, render_template, request, jsonify, send_file

from collector import InventoryCollector
from excel_builder import build_excel
from version_utils import GITHUB_REPO, get_latest_release, is_newer

__version__ = "1.1.0"

# Configure logging - console + file
LOG_FORMAT = "%(asctime)s [%(levelname)s] %(name)s - %(message)s"

# Determine log file path (next to exe or script)
if getattr(sys, "frozen", False):
    _base_dir = os.path.dirname(sys.executable)
else:
    _base_dir = os.path.dirname(os.path.abspath(__file__))

LOG_FILE = os.path.join(_base_dir, "fc_inventory.log")

logging.basicConfig(
    level=logging.INFO,
    format=LOG_FORMAT,
    handlers=[
        logging.StreamHandler(),
        logging.handlers.RotatingFileHandler(
            LOG_FILE, maxBytes=5 * 1024 * 1024, backupCount=3, encoding="utf-8",
        ),
    ],
)

# Quiet down noisy libraries
logging.getLogger("urllib3").setLevel(logging.WARNING)
logging.getLogger("werkzeug").setLevel(logging.WARNING)

app = Flask(__name__)

# Global state for single-user, single-job operation
current_job = {
    "collector": None,
    "thread": None,
    "output_file": None,
}


@app.before_request
def _reject_cross_origin():
    """Lightweight CSRF / drive-by protection for state-changing requests.

    The tool has no login, so a malicious web page could otherwise POST to the
    local server (CSRF). Browsers always attach an ``Origin`` header on
    cross-origin requests; reject those whose origin host does not match the
    request host. Non-browser clients (curl, the headless CLI) send no Origin
    and are unaffected.
    """
    if request.method in ("GET", "HEAD", "OPTIONS"):
        return None
    origin = request.headers.get("Origin")
    if not origin:
        return None
    if urlparse(origin).netloc != request.host:
        return jsonify({"error": "Cross-origin request rejected."}), 403
    return None


def _run_collection(collector):
    """Background thread: collect data and build Excel file."""
    try:
        data = collector.collect_all()

        # Generate output file next to exe/script (not in temp)
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        output_path = os.path.join(
            _base_dir,
            f"FC_Inventory_{timestamp}.xlsx",
        )
        build_excel(data, output_path)
        current_job["output_file"] = output_path
        logging.info(f"Excel saved to: {output_path}")

    except InterruptedError:
        logging.info("Collection cancelled by user")
        collector.progress["status"] = "cancelled"
        collector.progress["current_step"] = "Cancelled"

    except Exception as e:
        logging.exception("Collection failed in background thread")
        collector.progress["status"] = "error"
        collector.progress["error"] = str(e)


# ── Routes ───────────────────────────────────────────────


@app.route("/")
def index():
    """Serve the main web UI."""
    return render_template("index.html")


@app.route("/api/collect", methods=["POST"])
def start_collection():
    """Start inventory collection in a background thread."""
    # Check if already running
    if (current_job["thread"] is not None
            and current_job["thread"].is_alive()):
        return jsonify({"error": "A collection is already in progress."}), 409

    # Parse request
    body = request.get_json(silent=True) or {}
    host = body.get("host", "").strip()
    username = body.get("username", "").strip()
    password = body.get("password", "")

    if not host or not username or not password:
        return jsonify({"error": "Host, username, and password are required."}), 400

    # Validate the port: reject non-numeric or out-of-range values up front
    # instead of failing deep inside the request handler.
    try:
        port = int(body.get("port", 7443))
    except (TypeError, ValueError):
        return jsonify({"error": "Port must be a number."}), 400
    if not 1 <= port <= 65535:
        return jsonify({"error": "Port must be between 1 and 65535."}), 400

    # Clean up old output file
    if current_job["output_file"] and os.path.exists(current_job["output_file"]):
        try:
            os.remove(current_job["output_file"])
        except OSError:
            pass

    # Create collector and start background thread
    collector = InventoryCollector(host, username, password, port=port)
    current_job["collector"] = collector
    current_job["output_file"] = None

    thread = threading.Thread(target=_run_collection, args=(collector,), daemon=True)
    current_job["thread"] = thread
    thread.start()

    return jsonify({"status": "started"}), 202


@app.route("/api/progress")
def get_progress():
    """Return current collection progress."""
    if current_job["collector"] is None:
        return jsonify({"status": "idle", "percent": 0, "current_step": "", "error": None})

    return jsonify(current_job["collector"].progress)


@app.route("/api/cancel", methods=["POST"])
def cancel_collection():
    """Cancel the running collection."""
    collector = current_job.get("collector")
    if collector and current_job["thread"] and current_job["thread"].is_alive():
        collector.cancel()
        return jsonify({"status": "cancelling"})
    return jsonify({"status": "no_job_running"}), 404


@app.route("/api/download")
def download_file():
    """Download the generated Excel file."""
    output_file = current_job.get("output_file")
    if not output_file or not os.path.exists(output_file):
        return jsonify({"error": "No file available for download."}), 404

    filename = os.path.basename(output_file)
    return send_file(
        output_file,
        as_attachment=True,
        download_name=filename,
        mimetype="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    )


@app.route("/api/version")
def version():
    return jsonify({"version": __version__})


@app.route("/api/update-check")
def update_check():
    """
    Check GitHub for a newer release. Returns whether an update is available
    so the web UI can show a non-intrusive notice. Never auto-updates, never
    sends any data about the user. Disable with FC_INVENTORY_DISABLE_UPDATE_CHECK.
    """
    if os.environ.get("FC_INVENTORY_DISABLE_UPDATE_CHECK"):
        return jsonify({"enabled": False, "current": __version__})

    try:
        latest = get_latest_release(GITHUB_REPO)
        tag = latest["tag"]
        available = bool(tag) and is_newer(tag, __version__)
        return jsonify({
            "enabled": True,
            "current": __version__,
            "latest": tag.lstrip("vV"),
            "update_available": available,
            "url": latest["url"],
        })
    except Exception as e:
        # Offline / rate-limited / no releases yet: degrade silently.
        logging.info(f"Update check skipped: {e}")
        return jsonify({"enabled": True, "current": __version__, "error": str(e)})


@app.route("/changelog")
def changelog_page():
    return render_template("changelog.html", version=__version__)


def _find_resource(filename):
    """Locate a bundled resource file (works in dev and PyInstaller frozen)."""
    if getattr(sys, "frozen", False):
        candidates = [
            os.path.join(sys._MEIPASS, filename),
            os.path.join(_base_dir, filename),
        ]
    else:
        candidates = [os.path.join(_base_dir, filename)]
    for p in candidates:
        if os.path.exists(p):
            return p
    return None


@app.route("/api/changelog")
def changelog():
    """Return the CHANGELOG.md content as plain text."""
    path = _find_resource("CHANGELOG.md")
    if not path:
        return "CHANGELOG.md not found.", 404, {"Content-Type": "text/plain; charset=utf-8"}
    try:
        with open(path, "r", encoding="utf-8") as f:
            content = f.read()
        return content, 200, {"Content-Type": "text/plain; charset=utf-8"}
    except Exception as e:
        return f"Error reading CHANGELOG: {e}", 500, {"Content-Type": "text/plain; charset=utf-8"}


def _get_lan_ip():
    """Best-effort LAN IP detection for the startup banner."""
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        return ip
    except Exception:
        return "127.0.0.1"


def _print_banner(host, port):
    lan_ip = _get_lan_ip()
    banner = f"""
========================================================
  FC Inventory Tool v{__version__}
  FusionCompute Inventory Collector
========================================================

  Open in your browser:
    -> http://localhost:{port}
    -> http://{lan_ip}:{port}

  Log file:    {LOG_FILE}
  Output dir:  {_base_dir}

  Press CTRL+C to stop the server.
========================================================
"""
    print(banner, flush=True)


def run_web(bind=None, port=None):
    """Launch the local web UI (default mode)."""
    # Bind to localhost by default for security.
    # Set FC_INVENTORY_BIND=0.0.0.0 to expose on the LAN (use with caution).
    host = bind or os.environ.get("FC_INVENTORY_BIND", "127.0.0.1")
    port = port or int(os.environ.get("FC_INVENTORY_PORT", "5000"))

    _print_banner(host, port)

    # Auto-open browser on startup (only when frozen exe to avoid dev annoyance)
    if getattr(sys, "frozen", False):
        try:
            threading.Timer(1.0, lambda: webbrowser.open(f"http://localhost:{port}")).start()
        except Exception:
            pass

    # Try waitress (production WSGI), fall back to Flask dev server
    try:
        from waitress import serve
        logging.info(f"Starting waitress on {host}:{port}")
        serve(app, host=host, port=port, threads=8, _quiet=True)
    except ImportError:
        logging.warning("waitress not installed, using Flask dev server")
        app.run(host=host, port=port, debug=False, use_reloader=False)


def run_headless(args):
    """
    Run a single collection without the web UI and write an Excel file, then
    exit. Intended for automation (e.g. Windows Task Scheduler). Returns a
    process exit code.
    """
    password = args.password or os.environ.get("FC_INVENTORY_PASSWORD")
    if not password:
        try:
            password = getpass.getpass("FusionCompute password: ")
        except Exception:
            password = ""
    if not password:
        print(
            "ERROR: password required. Pass --password, set the "
            "FC_INVENTORY_PASSWORD environment variable, or run interactively.",
            file=sys.stderr,
        )
        return 2

    collector = InventoryCollector(args.host, args.username, password, port=args.port)
    logging.info(
        f"Headless collection from {args.host}:{args.port} as {args.username}"
    )

    try:
        data = collector.collect_all()
    except KeyboardInterrupt:
        logging.warning("Interrupted by user")
        return 130
    except Exception as e:
        logging.error(f"Collection failed: {e}")
        return 1

    out = args.out
    if not out:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        out = os.path.join(os.getcwd(), f"FC_Inventory_{timestamp}.xlsx")
    out = os.path.abspath(out)

    out_dir = os.path.dirname(out)
    if out_dir and not os.path.isdir(out_dir):
        os.makedirs(out_dir, exist_ok=True)

    build_excel(data, out)
    logging.info(f"Excel saved to: {out}")
    # Print the path on its own line so scripts can capture it.
    print(out)
    return 0


def _build_arg_parser():
    parser = argparse.ArgumentParser(
        prog="FCInventoryTool",
        description="FusionCompute inventory collector (RVTools-style).",
    )
    parser.add_argument(
        "--version", action="version",
        version=f"FC Inventory Tool {__version__}",
    )
    sub = parser.add_subparsers(dest="command")

    web_p = sub.add_parser("web", help="Launch the web UI (this is the default).")
    web_p.add_argument(
        "--host", default=None,
        help="Bind address for the web UI (overrides FC_INVENTORY_BIND).",
    )
    web_p.add_argument(
        "--port", type=int, default=None,
        help="Web UI port (overrides FC_INVENTORY_PORT, default 5000).",
    )

    col_p = sub.add_parser(
        "collect", help="Run one headless collection to an .xlsx file and exit.",
    )
    col_p.add_argument("--host", required=True, help="FusionCompute VRM host or IP.")
    col_p.add_argument("--username", required=True, help="Login username.")
    col_p.add_argument(
        "--password", default=None,
        help="Login password. Omit to use FC_INVENTORY_PASSWORD or be prompted.",
    )
    col_p.add_argument(
        "--port", type=int, default=7443,
        help="FusionCompute API port (default 7443).",
    )
    col_p.add_argument(
        "--out", default=None,
        help="Output .xlsx path (default: ./FC_Inventory_<timestamp>.xlsx).",
    )
    return parser


def main():
    parser = _build_arg_parser()
    args = parser.parse_args()

    if args.command == "collect":
        sys.exit(run_headless(args))

    # Default (no subcommand) or explicit "web": launch the UI.
    run_web(getattr(args, "host", None), getattr(args, "port", None))


if __name__ == "__main__":
    main()
