#!/usr/bin/env python3
"""Extract the Script-item JavaScript and resolved parameters from the template
YAML into a single JSON file the Node harness can load.

This avoids hand-copying the embedded JS: the harness always runs exactly what
ships in the template.
"""
import json
import os
import sys
import yaml

HERE = os.path.dirname(os.path.abspath(__file__))
TEMPLATE = os.path.join(
    HERE, "..", "Virtualization", "Huawei_FusionCompute", "7.0",
    "template_huawei_fusioncompute_http.yaml",
)


def macro_map(auth_mode):
    return {
        "{$FC.HOST}": "127.0.0.1",
        "{$FC.PORT}": "7443",
        "{$FC.USER}": "monitor",
        "{$FC.PASSWORD}": "Secret@123",
        "{$FC.API.VERSION}": "v8.0",
        "{$FC.AUTH.MODE}": auth_mode,
        "{$FC.DATA.TIMEOUT}": "30s",
    }


def resolve(val, macros):
    return macros.get(val, val)


def main():
    auth_mode = sys.argv[1] if len(sys.argv) > 1 else "plain"
    macros = macro_map(auth_mode)
    with open(TEMPLATE) as f:
        doc = yaml.safe_load(f)
    tpl = doc["zabbix_export"]["templates"][0]
    out = {}
    for item in tpl["items"]:
        if item.get("type") != "SCRIPT":
            continue
        params = {p["name"]: resolve(p["value"], macros)
                  for p in item.get("parameters", [])}
        out[item["key"]] = {"script": item["params"], "params": params}
    gen_dir = os.path.join(HERE, "_generated")
    os.makedirs(gen_dir, exist_ok=True)
    dest = os.path.join(gen_dir, "items.json")
    with open(dest, "w") as f:
        json.dump(out, f, indent=2)
    print(f"Wrote {len(out)} script items to {dest} (authMode={auth_mode})")


if __name__ == "__main__":
    main()
