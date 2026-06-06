# Changelog

All notable changes to `fc-inventory` are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the
project adheres to [Semantic Versioning](https://semver.org/).

## [2.0.1] - 2026-06-06

### Fixed

- **Cross-compile matrix was ignoring `GOOS`/`GOARCH`.** The
  `.github/workflows/build-release.yml` build step did not export the
  matrix values to `go build`, so all three jobs (windows/amd64,
  linux/amd64, darwin/amd64) compiled for the runner's native OS
  (Linux). The Windows zip that landed in the v2.0.0 release
  therefore contained a Linux ELF renamed to `fc-inventory.exe.bin`
  and could not be run on Windows. The v2.0.0 release has been
  removed; v2.0.1 publishes the correct cross-compiled binaries.
- **`SHA256SUMS.txt` was per-platform and got clobbered.** Each matrix
  job wrote its own `SHA256SUMS.txt` with the same filename, so the
  release step uploaded a file containing only the last job's checksums
  (windows in v2.0.0). v2.0.1 generates a single multi-platform
  `SHA256SUMS.txt` in the release job, listing all three archives with
  their release-page filenames so users can `sha256sum -c
  SHA256SUMS.txt` directly.

## [2.0.0] - 2026-06-06

### Changed — breaking

- **Complete rewrite in Go.** The Python 3 + Flask implementation is
  gone. The new `fc-inventory` is a single statically-linked Go binary
  (`cmd/fc-inventory/main.go`) with no Python runtime, no web server,
  and no browser UI.
- **Web UI removed.** The v1.0.0 Flask app (`app.py`, `templates/`,
  `static/`) and the `waitress` WSGI server are deleted. There is no
  HTTP listener in the new binary. All user interaction is via CLI
  flags and the YAML config file.
- **Configuration is now a YAML file.** The per-request JSON body that
  the old `/api/collect` endpoint accepted is gone. All connection
  parameters (host, port, username, password, TLS, timeouts) come from
  `fc-inventory.yaml`. The schema is documented in
  [`examples/fc-inventory.yaml`](examples/fc-inventory.yaml).
- **Password handling uses `${ENV}` interpolation.** `fc.password:
  "${FC_PASSWORD}"` is resolved from the environment at startup. A
  missing env var fails fast with a clear error.
- **Output filename is `FC_Inventory_{YYYYMMDD_HHMMSS}.xlsx`** in the
  configured `output.directory`. The previous run's file matching the
  prefix is removed when `output.overwrite: true` (the default).
- **Cancellation is now `SIGINT`/`SIGTERM`** (Ctrl+C). The Go binary
  uses `signal.NotifyContext`; the old Python `cancelled` flag and
  `threading.Thread` is gone.
- **Progress reporting is stderr text lines** (`[NN%] step`) instead of
  an HTTP-polled JSON progress bar. There is no longer any progress
  endpoint.
- **Release pipeline is now a Go cross-compile matrix** in
  `.github/workflows/build-release.yml`: `windows/amd64`,
  `linux/amd64`, `darwin/amd64`. The Windows-only PyInstaller pipeline
  is deleted.
- **Package layout is the standard Go `cmd/` + `internal/` split** with
  packages `config`, `logging`, `fieldmap`, `fcclient`, `collector`,
  `excel`.

### Preserved

- **Auto-detect login matrix** is identical: 6 API versions
  (`v8.0, v6.5, v6.3, v6.1, v1.0, v9.0`) × 3 auth methods (`POST +
  X-Auth-*` plain, `POST + X-Auth-Key` SHA-256, `PUT` JSON body) × 3
  ports (`[cfg.port, 7443, 8443]`). Rejection body code `10000022` is
  still the "try next version" signal.
- **TLS verify is disabled by default** because FC ships with
  self-signed certs. Set `fc.insecure_tls: false` to opt out.
- **Hybrid field mapping** (8 tables: `VMFields`, `CPUFields`,
  `MemoryFields`, `DiskFields`, `NICFields`, `HostFields`,
  `ClusterFields`, `DatastoreFields`) is a 1:1 port of
  `collector.py:76-184` so the `.xlsx` output is byte-equivalent
  where field names and order match.
- **Sheet order and styling** (10 RVTools-style sheets, bold white
  headers on `#2C3E50`, autofilter, frozen row 1, autosize from
  first 100 rows capped at 50) is preserved.
- **Rotating log file** (5 MB × 3 backups, 30 days, gzip-compressed)
  is preserved.
- **Pagination** (`offset`/`limit` loop with `total` and
  `len(batch) < limit` termination) is preserved.
- **Per-VM/disk fallback chain** (`vmConfig.disks` → `vmConfig.volumes`
  → `/volumes` → `/disks`) is preserved in `fcclient.ExtractVMDisks`.
- **Site-level portgroup fallback** (when per-DVSwitch enumeration
  returns empty) is preserved.

### Added

- `--dry-run` flag: parse the config and print a plan, no network
  calls.
- `--log-level` flag: override the `logging.level` from the YAML
  without editing the file.
- `--output` and `--filename` flags: override `output.directory` and
  `output.filename_prefix` per run.
- `mock-server` subcommand: starts a local mock FC API on
  `127.0.0.1:17443` for offline E2E testing. Serves canned JSON from
  `testdata/mock_fc/`.
- `version` subcommand: prints the binary version (set via
  `-ldflags "-X main.version=..."` in CI).
- Field-mapping table comments documenting the original Python
  source lines for traceability.
- Unit tests for `fieldmap` (golden path resolution), `config` (env
  interpolation + validation), and `excel` (round-trip).
- `go vet` and `go test` runs on every CI build.

### Removed

- `app.py`, `fc_client.py`, `collector.py`, `excel_builder.py`
  (Python sources).
- `requirements.txt`, `build.bat` (Python build artefacts).
- `templates/`, `static/` (Flask web UI).
- `docs/take_screenshots.py`, `docs/images/*.png` (screenshot helper
  for the web UI).
- `.github/workflows/build-release.yml` (Windows-only PyInstaller
  pipeline).
- `FC_INVENTORY_BIND` and `FC_INVENTORY_PORT` env vars (no longer
  relevant without a web server).

### Security

- Password never reaches the log sink. `config.LogValue` returns a
  redacted view (`Password: "***"`) for any debug print.
- No open ports. The binary makes outbound connections only to the
  configured FC host.

## [1.0.0] - 2026-04-10

Initial public release. Python 3 + Flask + waitress + openpyxl + PyInstaller.
See git history prior to the v2.0.0 commit for the v1.0.0 source.
