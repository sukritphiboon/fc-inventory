# FC Inventory Tool

A web-based inventory collector for Huawei FusionCompute, similar to RVTools for VMware. Connects to the FusionCompute VRM REST API, gathers infrastructure data (VMs, hosts, clusters, datastores, networks), and exports to a multi-sheet Excel workbook.

**Version:** 1.0.0
**License:** Internal use
**Tested with:** FusionCompute 8.9.0

---

## Features

- **One-click web UI** — open in browser, enter credentials, click Collect
- **10 Excel sheets** matching RVTools conventions:
  `vSummary`, `vInfo`, `vCPU`, `vMemory`, `vDisk`, `vNetwork`, `vHost`, `vCluster`, `vDatastore`, `vSwitch`
- **Power state per VM** (ON/OFF) on every relevant sheet
- **VM UUID** included for asset tracking
- **Hybrid field mapping** — works across multiple FusionCompute versions by trying multiple field name candidates
- **Auto-detect login** — tries multiple API versions and auth methods
- **Live progress bar** with cancel support
- **Remembers last credentials** (host/port/username) in browser localStorage; password is never saved
- **Standalone .exe** — no Python install required on target machines
- **Rotating log file** for troubleshooting
- **Production WSGI server** (waitress) — not the Flask dev server

---

## Quick Start (End Users)

### Option A: Standalone .exe (recommended)

1. Download/copy the `FCInventoryTool` folder to your machine
2. Double-click `FCInventoryTool.exe`
3. Browser opens automatically at `http://localhost:5000`
4. Enter your FusionCompute VRM details:
   - **Host / IP**: e.g., `192.168.1.100` (no `https://` needed)
   - **Port**: `7443` (default for VRM REST API)
   - **Username** / **Password**
5. Click **Collect Inventory** and wait for the progress bar
6. Click **Download Excel** when complete

The Excel file is also saved next to `FCInventoryTool.exe` for safekeeping.

### Option B: Run from source (developers)

```bash
git clone https://github.com/sukritphiboon/fc-inventory.git
cd fc-inventory
pip install -r requirements.txt
python app.py
```

Then open <http://localhost:5000>

---

## Building the .exe

Requires Python 3.9+ and PyInstaller.

```bash
build.bat
```

Output goes to `dist\FCInventoryTool\`. Distribute the entire folder (~80 MB).

---

## Configuration

Environment variables (optional):

| Variable | Default | Description |
|---|---|---|
| `FC_INVENTORY_BIND` | `127.0.0.1` | Bind address. Set to `0.0.0.0` to expose on LAN (see Security below) |
| `FC_INVENTORY_PORT` | `5000` | Local web UI port |

Example:
```cmd
set FC_INVENTORY_BIND=0.0.0.0
set FC_INVENTORY_PORT=8080
FCInventoryTool.exe
```

---

## Excel Sheet Reference

| Sheet | Description |
|---|---|
| **vSummary** | Total counts (VMs, hosts, clusters) and Power ON/OFF breakdown per cluster |
| **vInfo** | VM overview: name, UUID, power state, OS, CPU, memory, disk, IPs, host, cluster |
| **vCPU** | VM CPU details: cores, sockets, reservation, limit, weight, hot-plug |
| **vMemory** | VM memory details: size, reservation, limit, hot-plug, hugepages |
| **vDisk** | One row per disk: capacity, bus type, thin provision, datastore |
| **vNetwork** | One row per NIC: MAC, IP, port group, VLAN, NIC type |
| **vHost** | Physical host info: CPU model/cores/MHz, memory, BMC IP, status |
| **vCluster** | Cluster config: HA, DRS, host count |
| **vDatastore** | Datastore capacity, free space, used %, type |
| **vSwitch** | Distributed virtual switches and port groups |

---

## Network Requirements

- The machine running FC Inventory Tool must reach **TCP 7443** on the FusionCompute VRM
- The browser only needs to reach the local machine (default `127.0.0.1:5000`)
- Self-signed TLS certificates on the VRM are accepted (verification disabled — see Security below)

---

## Security Considerations

This tool is intended for **internal/trusted environments only**. Please review the following before deploying:

### Built-in protections
- **Local-only by default** — the web UI binds to `127.0.0.1`. Other machines on your network cannot reach it unless you explicitly set `FC_INVENTORY_BIND=0.0.0.0`
- **Password is never persisted** — only host/username/port are remembered (browser localStorage)
- **Password is never logged** — credentials are never written to `fc_inventory.log`
- **No authentication on the web UI** — because access is limited to localhost
- **Production WSGI** — uses `waitress`, not Flask's debug server
- **Token-based session** to FusionCompute — token is held in memory only, logout on completion

### Known limitations / things to know
- **TLS certificate verification is disabled** when calling the FusionCompute API. Most FC deployments use self-signed certificates, so verification would fail for legitimate hosts. This is the same behavior as `curl -k` or RVTools' SSL setting. Acceptable for trusted internal networks.
- **Single-user, single-job** at a time — there is no multi-user session management
- **No CSRF tokens** on the API endpoints — relies on local-only binding
- **No rate limiting** — not needed because access is single-user, local-only
- **The .exe is unsigned** — Windows SmartScreen may show a warning the first time you run it. Click "More info" → "Run anyway". To avoid this in production, the binary should be code-signed with an organization certificate.

### Recommended deployment hardening
If you need to expose this on a network:
1. **Do NOT** set `FC_INVENTORY_BIND=0.0.0.0` without putting it behind a reverse proxy (nginx, Caddy) that adds:
   - HTTPS with a real certificate
   - HTTP basic auth or SSO
   - IP allowlist
2. Run as a non-admin Windows user
3. Place the executable in a write-protected directory (Excel output and log file need a writable location — pass a custom path via env var if needed)
4. Use a dedicated read-only FusionCompute account for inventory collection (only needs view/query permissions, not write)
5. Code-sign the executable with your organization's certificate to remove SmartScreen warnings
6. Run an antivirus scan on the .exe before distributing internally

### Why this tool is not malicious
- **Source code is fully open** in this repository — every line is auditable
- **No outbound network calls** except to the FusionCompute VRM you specify
- **No telemetry, analytics, or auto-update** — runs entirely offline (apart from FC API calls)
- **No persistence** — does not install services, scheduled tasks, registry entries, or background processes
- **Build is reproducible** — run `build.bat` against the same git commit to verify the binary

---

## Troubleshooting

### Login fails with HTTP 401
- Verify the username/password works on the FusionCompute web console
- Check the log file `fc_inventory.log` — it tries multiple login methods and logs each attempt
- Confirm port 7443 is reachable: `Test-NetConnection <vrm_ip> -Port 7443`

### Excel has missing columns
- Check `fc_inventory.log` for `=== SAMPLE ... keys:` lines — these show the actual API field names returned by your FusionCompute version
- File a bug report with those log lines so the field mapping can be extended

### "No space left on device"
- Output is saved next to the .exe. Make sure there's at least 50 MB free in that location.

### Port 5000 already in use
```cmd
set FC_INVENTORY_PORT=5050
FCInventoryTool.exe
```

### Browser doesn't open automatically
Open `http://localhost:5000` (or your custom port) manually.

---

## Logs

- **Location**: `fc_inventory.log` next to the .exe (or working directory if running from source)
- **Rotation**: 5 MB max per file, 3 backups kept
- **Level**: INFO (DEBUG details available by editing `app.py`)
- **What is logged**: API request paths, HTTP status codes, response sizes, sample field keys, errors. **Passwords are never logged.**

---

## Repository

<https://github.com/sukritphiboon/fc-inventory>

## Files

```
fc-inventory/
├── app.py              # Flask app + production main entry point
├── fc_client.py        # FusionCompute REST API client
├── collector.py        # Orchestrates collection + field mapping
├── excel_builder.py    # Multi-sheet Excel generator
├── requirements.txt    # Python dependencies
├── build.bat           # Build standalone .exe
├── templates/
│   └── index.html      # Single-page web UI
├── static/
│   ├── style.css       # UI styling
│   └── app.js          # UI logic, progress polling
└── README.md           # This file
```

---

## Credits

- Built with Flask, openpyxl, requests, waitress
- API field references from Huawei FusionCompute 8.9.0 VRM API documentation
