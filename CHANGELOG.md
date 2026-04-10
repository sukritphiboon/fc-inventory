# Changelog

All notable changes to FC Inventory Tool will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-04-10

First production-ready release.

### Added
- Web-based UI to collect FusionCompute inventory and export to multi-sheet Excel
- 10 RVTools-style sheets: vSummary, vInfo, vCPU, vMemory, vDisk, vNetwork, vHost, vCluster, vDatastore, vSwitch
- Power state (ON/OFF) per VM on every relevant sheet
- VM UUID + Disk UUID columns for asset tracking
- Hybrid field mapping: each Excel column tries multiple candidate JSON paths to support different FusionCompute versions
- Auto-detect login: tries multiple API versions (v8.0, v6.5, v6.3, v6.1, v1.0, v9.0) and auth methods (header+plain, header+SHA256, JSON body)
- Auto-detect base URL: scans `/service/session` on ports 7443 and 8443
- Live progress bar with cancel button
- Browser auto-opens when running .exe
- Remembers last credentials (host/port/username) in localStorage; password is never saved
- Enter key to submit
- Rotating debug log file (`fc_inventory.log`, 5 MB max, 3 backups)
- Production WSGI server (waitress) instead of Flask dev server
- Localhost binding by default for security; LAN exposure via `FC_INVENTORY_BIND` env var
- Custom port via `FC_INVENTORY_PORT` env var
- Standalone .exe build via PyInstaller (`build.bat`)
- Site-level port group fallback when DVSwitch enumeration is empty
- Excel saved next to the executable (not in temp folder)
- Comprehensive README.md with security guidance and troubleshooting
- `/api/version` endpoint
- `/api/changelog` endpoint to view release notes from the UI
- Footer link to "What's new" in the web UI

### Security
- Passwords are never logged
- Passwords are never stored in browser localStorage
- TLS verification is disabled when calling FusionCompute API (documented — required for self-signed certs)
- Web UI binds to `127.0.0.1` only by default
- No telemetry, no auto-update, no background persistence

### Known Limitations
- The .exe is not code-signed; Windows SmartScreen may show a warning on first run
- Single-user, single-job at a time
- Tested against FusionCompute 8.9.0 — other versions may show extra/missing fields (the hybrid mapping handles this gracefully)
