# Zabbix template for Huawei FusionCompute

This folder contains a Zabbix monitoring template for Huawei FusionCompute,
derived from the FusionCompute REST API knowledge in this project's
[`fc_client.py`](../fc_client.py).

The directory layout intentionally mirrors the
[`zabbix/community-templates`](https://github.com/zabbix/community-templates)
repository so it can be contributed upstream with a straight copy:

```
Virtualization/
└── Huawei_FusionCompute/
    └── 7.0/
        ├── template_huawei_fusioncompute_http.yaml
        └── README.md
```

- **`Virtualization/Huawei_FusionCompute/7.0/template_huawei_fusioncompute_http.yaml`**
  — the Zabbix 7.0 template export (Script items + LLD + triggers).
- **`Virtualization/Huawei_FusionCompute/7.0/README.md`** — user-facing setup,
  macros, metrics and triggers (required by upstream).
- **[`CONTRIBUTING_TO_ZABBIX.md`](CONTRIBUTING_TO_ZABBIX.md)** — how to validate
  the template and submit it to the official Zabbix community-templates repo.

Quick local sanity check (structure only):

```bash
python3 -c "import yaml; yaml.safe_load(open('zabbix/Virtualization/Huawei_FusionCompute/7.0/template_huawei_fusioncompute_http.yaml'))"
```

A real import/run test against a Zabbix 7.0 server is described in the template
README and the contributing guide.
