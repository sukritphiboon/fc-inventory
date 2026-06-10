# Offline test harness

Validates the FusionCompute Zabbix template **without** a live VRM or a Zabbix
server. It mocks Zabbix's `HttpRequest`, serves synthetic FusionCompute API
responses, runs the *actual* JavaScript embedded in each Script item (extracted
straight from the template YAML), and simulates the dependent-item / LLD
preprocessing.

## Run

```bash
cd zabbix/test
python3 extract_scripts.py plain    # or: sha256
node harness.js
```

`extract_scripts.py` reads the template YAML and writes
`_generated/items.json` (the script bodies + macro-resolved parameters) that
`harness.js` loads — so the harness always runs exactly what ships.

## What it covers

- Login incl. **API version fallback** (mock accepts only `v6.5`) and both
  `{$FC.AUTH.MODE}` values (`plain` and `sha256`, the latter verified against
  Node's `crypto`).
- **Pagination** (`offset`/`limit`) — the VM fixture spans two pages.
- Per-resource parsing for hosts (with per-host detail fetch), VMs, datastores
  (incl. `capacityMB` / `freeSizeGB` field fallbacks), clusters and summary.
- Dependent-item logic: status→1/0 mapping, MB/GB→bytes multipliers, datastore
  used-% calculation, and warn/crit threshold crossings.
- Failure path: bad credentials raise an error (item → unsupported → trigger).

## What it does NOT cover

This is a logic test only. It cannot validate Zabbix-side concerns:

- That the YAML imports cleanly into a real Zabbix 7.0 (schema correctness).
- Real TLS / self-signed-cert handling by the Zabbix `HttpRequest` engine.
- Real FusionCompute field names/shapes across VRM versions.

Do the live import/run test (see `../CONTRIBUTING_TO_ZABBIX.md`) before opening
the upstream PR.
