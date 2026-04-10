"""
FC Inventory Tool - Flask Application
Web-based FusionCompute inventory collector with Excel export.
"""

import os
import sys
import socket
import logging
import logging.handlers
import threading
import webbrowser
from datetime import datetime

from flask import Flask, render_template, request, jsonify, send_file

from collector import InventoryCollector
from excel_builder import build_excel

__version__ = "1.0.0"

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
    port = int(body.get("port", 7443))
    username = body.get("username", "").strip()
    password = body.get("password", "")

    if not host or not username or not password:
        return jsonify({"error": "Host, username, and password are required."}), 400

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


def main():
    # Bind to localhost by default for security.
    # Set FC_INVENTORY_BIND=0.0.0.0 to expose on the LAN (use with caution).
    host = os.environ.get("FC_INVENTORY_BIND", "127.0.0.1")
    port = int(os.environ.get("FC_INVENTORY_PORT", "5000"))

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


if __name__ == "__main__":
    main()
