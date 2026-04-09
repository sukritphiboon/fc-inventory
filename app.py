"""
FC Inventory Tool - Flask Application
Web-based FusionCompute inventory collector with Excel export.
"""

import os
import sys
import logging
import logging.handlers
import tempfile
import threading
from datetime import datetime

from flask import Flask, render_template, request, jsonify, send_file

from collector import InventoryCollector
from excel_builder import build_excel
from mock_data import generate_mock_data

# Configure logging - console + file
LOG_FORMAT = "%(asctime)s [%(levelname)s] %(name)s - %(message)s"

# Determine log file path (next to exe or script)
if getattr(sys, "frozen", False):
    _base_dir = os.path.dirname(sys.executable)
else:
    _base_dir = os.path.dirname(os.path.abspath(__file__))

LOG_FILE = os.path.join(_base_dir, "fc_inventory.log")

logging.basicConfig(
    level=logging.DEBUG,
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
logging.getLogger("werkzeug").setLevel(logging.INFO)

app = Flask(__name__)

# Global state for single-user, single-job operation
current_job = {
    "collector": None,
    "thread": None,
    "output_file": None,
}


def _run_collection(collector, mock=False):
    """Background thread: collect data and build Excel file."""
    try:
        if mock:
            # Simulate collection with mock data
            import time
            steps = [
                (5, "Logging in to FusionCompute (mock)..."),
                (10, "Fetching sites..."),
                (15, "Fetching clusters..."),
                (25, "Fetching hosts..."),
                (40, "Fetching datastores..."),
                (50, "Fetching networks..."),
                (55, "Fetching VM list..."),
                (75, "Fetching VM details (15/30)..."),
                (90, "Fetching VM details (30/30)..."),
                (95, "Processing collected data..."),
            ]
            for pct, step in steps:
                if collector.cancelled:
                    raise InterruptedError("Collection cancelled by user.")
                collector.progress["percent"] = pct
                collector.progress["current_step"] = step
                time.sleep(0.5)

            data = generate_mock_data()
            collector.progress["percent"] = 100
            collector.progress["current_step"] = "Collection complete!"
            collector.progress["status"] = "done"
        else:
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
    mock = body.get("mock", False)

    if not mock and (not host or not username or not password):
        return jsonify({"error": "Host, username, and password are required."}), 400

    # Clean up old output file
    if current_job["output_file"] and os.path.exists(current_job["output_file"]):
        try:
            os.remove(current_job["output_file"])
        except OSError:
            pass

    # Create collector and start background thread
    if mock:
        collector = InventoryCollector("mock-host", "mock-user", "mock-pass")
        collector.progress["status"] = "running"
    else:
        collector = InventoryCollector(host, username, password, port=port)

    current_job["collector"] = collector
    current_job["output_file"] = None

    thread = threading.Thread(target=_run_collection, args=(collector, mock), daemon=True)
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


@app.route("/api/debug", methods=["POST"])
def debug_api():
    """Debug: login and fetch raw API response for a given path."""
    body = request.get_json(silent=True) or {}
    host = body.get("host", "").strip()
    port = int(body.get("port", 7443))
    username = body.get("username", "").strip()
    password = body.get("password", "")
    path = body.get("path", "/sites")

    from fc_client import FCClient
    client = FCClient(host, username, password, port=port)
    try:
        client.login()
        data = client._get(path)
        client.logout()
        return jsonify(data)
    except Exception as e:
        return jsonify({"error": str(e)}), 500


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5000, debug=True)
