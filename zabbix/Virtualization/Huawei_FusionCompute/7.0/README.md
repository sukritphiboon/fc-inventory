# Huawei FusionCompute by HTTP

## Overview

This template monitors Huawei FusionCompute (the VRM REST API) using Zabbix
`Script` items. Each Script item authenticates to the VRM, fetches one resource
list, and returns JSON that is parsed locally by dependent items and low-level
discovery (LLD) rules. No agent or external script is required — the checks run
on the Zabbix server (or proxy).

The authentication and pagination logic mirrors the proven
[`fc-inventory`](https://github.com/sukritphiboon/fc-inventory) client
(`POST /service/session` → `X-Auth-Token` → paginated `GET`s with
`offset`/`limit`).

## Requirements

- Zabbix **6.0 or newer** (uses the `Script` item type). This folder targets
  Zabbix **7.0 LTS**.
- Huawei FusionCompute **6.1 – 9.0** (the API version is auto-negotiated:
  `v8.0`, `v6.5`, `v6.3`, `v6.1`, `v1.0`, `v9.0`).
- Network access from the Zabbix server/proxy to the VRM on TCP `7443`.
- A read-only FusionCompute user.

## Setup

1. **Create a monitoring user** in FusionCompute with read-only privileges.
2. In Zabbix, create a host (or reuse one) representing the VRM and set its
   interface/`{HOST.CONN}` to the VRM address, or override `{$FC.HOST}`.
3. Link the template **Huawei FusionCompute by HTTP** to the host.
4. Set the macros below — at minimum `{$FC.USER}` and `{$FC.PASSWORD}`.

### TLS / self-signed certificates

FusionCompute VRM typically presents a self-signed certificate. The Zabbix
`Script` `HttpRequest` engine verifies peers using the Zabbix server's system CA
store. If login fails with a TLS error, add the VRM certificate (or its issuing
CA) to the trust store of the host running the Zabbix server/proxy
(e.g. `/etc/pki/ca-trust` + `update-ca-trust`, or the distro equivalent).

### Authentication mode

`{$FC.AUTH.MODE}` selects how the password is sent (matches the two methods the
`fc-inventory` client tries):

- `plain` (default) — `X-Auth-Key: <password>`, `X-ENCRYPT-ALGORITHM: 1`.
- `sha256` — `X-Auth-Key: sha256(password)`, `X-ENCRYPT-ALGORITHM: 0`.

If login returns HTTP 200 but no token, or 401, try the other mode.

## Macros

| Macro | Default | Description |
|-------|---------|-------------|
| `{$FC.HOST}` | `{HOST.CONN}` | VRM address. |
| `{$FC.PORT}` | `7443` | VRM API port. |
| `{$FC.USER}` | `monitor` | Read-only monitoring user. |
| `{$FC.PASSWORD}` | *(secret)* | Password for `{$FC.USER}`. |
| `{$FC.API.VERSION}` | `v8.0` | Preferred API version tried first. |
| `{$FC.AUTH.MODE}` | `plain` | `plain` or `sha256`. |
| `{$FC.DATA.TIMEOUT}` | `30s` | Script HTTP timeout. |
| `{$FC.LLD.FILTER.HOST.MATCHES}` | `.*` | Discover hosts matching this regex. |
| `{$FC.LLD.FILTER.HOST.NOT_MATCHES}` | `CHANGE_IF_NEEDED` | Ignore hosts matching this regex. |
| `{$FC.LLD.FILTER.VM.MATCHES}` | `.*` | Discover VMs matching this regex. |
| `{$FC.LLD.FILTER.VM.NOT_MATCHES}` | `CHANGE_IF_NEEDED` | Ignore VMs matching this regex. |
| `{$FC.LLD.FILTER.DATASTORE.MATCHES}` | `.*` | Discover datastores matching this regex. |
| `{$FC.LLD.FILTER.DATASTORE.NOT_MATCHES}` | `CHANGE_IF_NEEDED` | Ignore datastores matching this regex. |
| `{$FC.DATASTORE.PUSED.MAX.WARN}` | `80` | Datastore used-space warning threshold, %. |
| `{$FC.DATASTORE.PUSED.MAX.CRIT}` | `90` | Datastore used-space critical threshold, %. |

`{$FC.DATASTORE.PUSED.MAX.WARN}` / `.CRIT` support context overrides per
datastore name, e.g. `{$FC.DATASTORE.PUSED.MAX.WARN:"IPSAN"}`.

## Collected metrics

### Master (Script) items — raw JSON, not displayed directly

| Key | Resource |
|-----|----------|
| `fusioncompute.hosts` | Hosts list (with per-host detail). |
| `fusioncompute.vms` | VMs list. |
| `fusioncompute.datastores` | Datastores list. |
| `fusioncompute.clusters` | Clusters list. |
| `fusioncompute.summary` | Cluster-wide counts + availability flag. |

### Top-level items

| Key | Description |
|-----|-------------|
| `fusioncompute.available` | VRM API reachable + authenticated (1/0). |
| `fusioncompute.vms.total` / `.running` / `.stopped` | VM counts. |
| `fusioncompute.hosts.total` | Host count. |

### Discovery

| LLD rule | Item prototypes |
|----------|-----------------|
| Host discovery | status, CPU cores, CPU frequency, memory total/used, running VMs |
| VM discovery | power state, vCPUs, memory |
| Datastore discovery | capacity, free space, used %, status |
| Cluster discovery | HA enabled, DRS enabled |

> Note: per-VM vCPU/memory are populated only when the VM list includes
> `vmConfig` (varies by FusionCompute version); otherwise they report `0`.

## Triggers

| Trigger | Severity |
|---------|----------|
| VRM API is unavailable | High |
| Host: Status is not normal | Warning |
| Datastore: Space usage is high (> WARN%) | Warning |
| Datastore: Space usage is critical (> CRIT%) | High |
| VM: Powered off | Info (disabled by default) |
| Cluster: HA is disabled | Info (disabled by default) |

## License

Distributed under the MIT license, consistent with the Zabbix
community-templates repository.
