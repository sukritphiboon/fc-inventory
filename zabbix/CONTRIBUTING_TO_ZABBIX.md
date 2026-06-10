# Publishing the template to zabbix/community-templates

A step-by-step guide for getting this template into the official
[`zabbix/community-templates`](https://github.com/zabbix/community-templates)
repository. It follows the upstream
[community template guidelines](https://www.zabbix.com/documentation/guidelines/en/thosts/community_templates).

## 1. Read the upstream rules

- Guidelines: <https://www.zabbix.com/documentation/guidelines/en/thosts/community_templates>
- Repository: <https://github.com/zabbix/community-templates>

Key requirements the template already satisfies:

- One folder per template under a category, with a vendor subfolder
  (`[a-zA-Z0-9_-]`) and a per-version subfolder formatted `X.X`
  → `Virtualization/Huawei_FusionCompute/7.0/`.
- **YAML** export format (current Zabbix default) — no need for XML.
- A **non-empty `README.md`** with setup, macros, items and triggers.
- **MIT** license only (no GPL-licensed content, no links to private or
  file-sharing resources; all assets live in the repo).

## 2. Validate locally (structure)

```bash
python3 -c "import yaml; yaml.safe_load(open('Virtualization/Huawei_FusionCompute/7.0/template_huawei_fusioncompute_http.yaml'))"
```

## 3. Import and test on a real Zabbix 7.0 server

This is the most important step — the YAML must import cleanly and run against a
live (or lab) FusionCompute VRM.

1. In Zabbix: **Data collection → Templates → Import**, select the YAML.
2. Create a host, link **Huawei FusionCompute by HTTP**, set `{$FC.HOST}`,
   `{$FC.USER}`, `{$FC.PASSWORD}` (and `{$FC.AUTH.MODE}` if needed).
3. Open each master Script item and click **Test → Get value** to confirm it
   returns JSON (this surfaces login/TLS/version problems immediately).
4. Confirm the four LLD rules discover hosts/VMs/datastores/clusters and that
   the dependent items populate and triggers evaluate.
5. **Re-export** the template from Zabbix (**Templates → select → Export**) and
   commit that server-produced YAML. Upstream expects the canonical export form
   (normalized ordering/IDs) rather than a hand-written file.

## 4. Fork, branch, copy, PR

```bash
# after forking zabbix/community-templates on GitHub
git clone git@github.com:<you>/community-templates.git
cd community-templates
git checkout -b huawei-fusioncompute

mkdir -p Virtualization/Huawei_FusionCompute/7.0
cp <fc-inventory>/zabbix/Virtualization/Huawei_FusionCompute/7.0/* \
   Virtualization/Huawei_FusionCompute/7.0/

git add Virtualization/Huawei_FusionCompute
git commit -m "Add Huawei FusionCompute by HTTP template (7.0)"
git push -u origin huawei-fusioncompute
```

Then open a pull request against `zabbix/community-templates:main`. In the PR
description summarize what is monitored, the Zabbix and FusionCompute versions
supported, and how it authenticates. Be ready to iterate on maintainer feedback.

## 5. Checklist before opening the PR

- [ ] Imports without errors on a clean Zabbix 7.0.
- [ ] Master Script items return JSON against a real VRM.
- [ ] LLD discovers hosts, VMs, datastores, clusters.
- [ ] Triggers fire/resolve as expected.
- [ ] YAML is the re-exported (server canonical) version.
- [ ] `README.md` lists macros, items, triggers; no private links.
- [ ] MIT license, no GPL content.
