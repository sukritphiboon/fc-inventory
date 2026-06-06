# fc-inventory

> A one-shot CLI service for collecting inventory from Huawei FusionCompute VRM and exporting a RVTools-style multi-sheet Excel workbook.

[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

`fc-inventory` connects to a Huawei FusionCompute VRM REST API, fetches sites / clusters / hosts / VMs / datastores / networks, and writes a 10-sheet `.xlsx` workbook modelled on RVTools for VMware. The v2.0.0 release is a clean Go rewrite: a single statically-linked binary with no Python runtime, no web server, and no browser UI. All parameters come from a YAML config file; the service runs once and exits.

## Features

- **One-shot CLI service** — `fc-inventory collect` reads the YAML config, performs a single inventory pull, writes the workbook, and exits. Schedule it from cron / Task Scheduler / Kubernetes CronJob.
- **YAML config with `${ENV}` interpolation** — keep secrets out of the config file. `${FC_PASSWORD}` is resolved from the environment at startup; a missing variable fails fast.
- **Auto-detect login** — the same 6-API-version × 3-auth-method × 3-port matrix that the original v1.0.0 Python tool used.
- **Hybrid field mapping** — each Excel column maps to a list of candidate JSON paths; the first non-empty value wins, so the output stays stable across FusionCompute versions. Extra raw keys are also captured for forward compatibility.
- **Version-tolerant** — TLS verify is disabled by default because FC ships with self-signed certs (configurable per-host).
- **Cross-platform** — single static binary; releases for `windows/amd64`, `linux/amd64`, `darwin/amd64`.

## Output sheets (RVTools convention)

`vSummary`, `vInfo`, `vCPU`, `vMemory`, `vDisk`, `vNetwork`, `vHost`, `vCluster`, `vDatastore`, `vSwitch`. Each sheet has bold white-on-`#2C3E50` headers, an autofilter, and a frozen top row.

## Quick start

### 1. Download a release

Grab the binary for your platform from the [Releases](https://github.com/sukritphiboon/fc-inventory/releases) page. Linux/macOS example:

```bash
curl -LO https://github.com/sukritphiboon/fc-inventory/releases/latest/download/fc-inventory-linux-amd64.tar.gz
tar -xzf fc-inventory-linux-amd64.tar.gz
chmod +x fc-inventory
```

### 2. Create a config file

```bash
curl -LO https://raw.githubusercontent.com/sukritphiboon/fc-inventory/main/examples/fc-inventory.yaml
```

Edit `fc-inventory.yaml` to point at your VRM, then export the password:

```bash
export FC_PASSWORD='YourFusionComputePassword'
```

### 3. Run

```bash
./fc-inventory --config ./fc-inventory.yaml
```

You will see progress lines on stderr:

```
[  5%] Logging in to FusionCompute...
[ 10%] Fetching sites...
[ 15%] Fetching clusters...
...
[100%] Collection complete!
Excel saved: ./out/FC_Inventory_20260606_120000.xlsx
```

The workbook is saved next to the binary in `./out/`. Open it in Excel or LibreOffice.

## Command-line reference

```
fc-inventory collect
  --config       string   default: ./fc-inventory.yaml
  --log-level    string   debug|info|warn|error  (overrides config)
  --dry-run      bool     parse config + show plan, no network
  --output       string   override output.directory
  --filename     string   override output.filename_prefix

fc-inventory version [--short]
fc-inventory mock-server [--port 17443]   # local mock FC API for testing
```

`fc-inventory` with no subcommand defaults to `collect`.

### Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | collection / API / network error |
| 2 | config / validation error (e.g. missing `${ENV}`) |
| 130 | cancelled (Ctrl+C) |

## Configuration reference

See [`examples/fc-inventory.yaml`](examples/fc-inventory.yaml) for the full annotated schema. Key fields:

| Field | Default | Description |
| --- | --- | --- |
| `fc.host` | — (required) | VRM host or IP; `https://` prefix auto-stripped |
| `fc.port` | 7443 | primary port; auto-detect also tries 7443 and 8443 |
| `fc.username` | — (required) | FusionCompute username |
| `fc.password` | — (required) | password; supports `${ENV}` interpolation |
| `fc.insecure_tls` | `true` | disable TLS verify (FC uses self-signed certs) |
| `collection.page_size` | 100 | offset/limit page size for paginated GETs |
| `collection.request_timeout_seconds` | 60 | per-request HTTP timeout |
| `output.directory` | `.` | output directory (created if missing) |
| `output.filename_prefix` | `FC_Inventory` | prefix of the produced `.xlsx` |
| `output.overwrite` | `true` | remove the previous run's `.xlsx` before writing |
| `logging.level` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `logging.file` | `fc_inventory.log` | rotating log file path |

## Scheduling

The service is a one-shot CLI; run it on a schedule.

### Linux / macOS cron

```cron
# Every 6 hours, write to a date-stamped directory
0 */6 * * * cd /opt/fc-inventory && FC_PASSWORD="$(cat .fc_password)" ./fc-inventory --config fc-inventory.yaml
```

### Windows Task Scheduler

Create a Basic Task → Trigger: Daily, Recur every 1 day, Repeat task every 6 hours. Action: `fc-inventory.exe --config "C:\fc-inventory\fc-inventory.yaml"`. Set the password in a `.env` script that's sourced before the binary runs, or use a system environment variable.

## Security model

- **TLS verify disabled by default** because FusionCompute ships with self-signed certificates. If your deployment uses a CA-signed cert, set `fc.insecure_tls: false`.
- **Password is never logged.** The YAML loader expands `${ENV}` into the in-memory config struct; a custom redacting `LogValuer` strips it before any debug log line. Use `${FC_PASSWORD}` and keep the secret out of the YAML file.
- **No telemetry.** The only outbound network call is to the FusionCompute VRM you configure. The binary makes zero other connections.
- **No web server.** The v1.0.0 Flask layer is gone. There is no HTTP listener inside the service binary. You can point it at a remote VRM over the public internet; nobody else can connect to the collector.

## Building from source

```bash
git clone https://github.com/sukritphiboon/fc-inventory.git
cd fc-inventory
go build -o fc-inventory ./cmd/fc-inventory
```

Cross-compile:

```bash
GOOS=windows GOARCH=amd64 go build -o fc-inventory.exe ./cmd/fc-inventory
GOOS=linux   GOARCH=amd64 go build -o fc-inventory     ./cmd/fc-inventory
GOOS=darwin  GOARCH=amd64 go build -o fc-inventory-darwin ./cmd/fc-inventory
```

Run the unit tests:

```bash
go test ./...
```

## Project layout

```
fc-inventory/
├── cmd/fc-inventory/main.go        # Cobra CLI; entry point
├── internal/
│   ├── config/        # YAML loader, env-var interpolation
│   ├── logging/       # slog + lumberjack rotating file
│   ├── fieldmap/      # hybrid field-mapping tables + path helpers
│   ├── fcclient/      # FC REST adapter (auto-detect login, pagination, resources)
│   ├── collector/     # orchestrator: login -> fetch -> flatten -> sheet data
│   └── excel/         # openpyxl-equivalent .xlsx writer (excelize)
├── testdata/mock_fc/               # canned JSON for offline E2E
├── examples/fc-inventory.yaml      # annotated sample config
└── .github/workflows/build-release.yml  # cross-compile release pipeline
```

## Comparison with v1.0.0 (Python + Flask)

| Concern | v1.0.0 (Python) | v2.0.0 (Go) |
| --- | --- | --- |
| Runtime | Python 3.9+, Flask, waitress, requests, openpyxl | None — static Go binary |
| User surface | Web UI on `127.0.0.1:5000` | CLI (YAML config) |
| Distribution | PyInstaller `.exe` (one-dir, ~80 MB) | single ~10 MB binary per OS |
| Excel writer | `openpyxl` | `excelize` |
| TLS verify | disabled (self-signed certs) | disabled by default, opt-in to enable |
| Config | per-request JSON body + `localStorage` | YAML file with `${ENV}` interpolation |
| Cancellation | `cancelled` flag + `threading.Event` | `context.Context` + `signal.NotifyContext` |
| Auto-detect login | 6 versions × 3 auths × 3 ports | identical matrix |
| Hybrid field map | 8 `OrderedDict`s in `collector.py` | 8 `map[string][]string` in `fieldmap/mapping.go` |
| Sheet count | 10 (RVTools convention) | 10 (RVTools convention) |

## License

MIT — see [LICENSE](LICENSE).

## Authors

See [AUTHORS.md](AUTHORS.md). v2.0.0 is a Go port of the v1.0.0 Python tool by Sukrit Phiboon, with AI pair-programming assistance from Claude.
