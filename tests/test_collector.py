"""Tests for collector helpers and sheet builders (no network required)."""

from collector import (
    InventoryCollector,
    _power_state,
    _try_paths,
    _get_path,
    _prettify_key,
)


def _make_collector():
    # __init__ creates an FCClient but performs no network calls until login().
    return InventoryCollector("host", "user", "pass", port=7443)


def test_power_state_mapping():
    assert _power_state("running") == "ON"
    assert _power_state("stopped") == "OFF"
    assert _power_state("shutOff") == "OFF"
    assert _power_state("migrating") == "migrating"
    assert _power_state("") == ""


def test_get_path_nested():
    d = {"vmConfig": {"cpu": {"quantity": 4}}}
    assert _get_path(d, "vmConfig.cpu.quantity") == 4
    assert _get_path(d, "vmConfig.cpu.missing") is None
    assert _get_path(d, "nope.here") is None


def test_try_paths_returns_first_non_empty():
    d = {"a": "", "b": "value"}
    assert _try_paths(d, ["a", "b"]) == "value"
    assert _try_paths(d, ["missing"]) == ""


def test_prettify_key():
    assert _prettify_key("vmConfig.cpu.quantity") == "Cpu - Quantity"


def test_build_vinfo_merges_detail_disks_and_nics():
    c = _make_collector()
    vm = {
        "name": "vm1",
        "urn": "urn:vm:1",
        "status": "running",
        "uuid": "uuid-1",
        "clusterUrn": "urn:cl:1",
        "locationUrn": "urn:host:1",
    }
    c.vms = [vm]
    c.vm_details = {
        "urn:vm:1": {
            "vmConfig": {
                "cpu": {"quantity": 4, "coresPerSocket": 2},
                "memory": {"quantityMB": 8192},
            },
            "osOptions": {"osType": "Linux"},
        }
    }
    c.vm_disks = {"urn:vm:1": [{"quantityGB": 40}, {"quantityGB": 60}]}
    c.vm_nics = {"urn:vm:1": [{"ip": "10.0.0.1"}, {"ip": ""}]}
    c.host_map = {"urn:host:1": "esx-01"}
    c.cluster_map = {"urn:cl:1": "Cluster-A"}

    rows = c._build_vinfo()
    assert len(rows) == 1
    row = rows[0]
    assert row["VM Name"] == "vm1"
    assert row["Power State"] == "ON"
    assert row["CPUs"] == 4
    assert row["Memory (MB)"] == 8192
    assert row["Total Disk (GB)"] == 100
    assert row["NICs"] == 2
    assert row["IP Addresses"] == "10.0.0.1"
    assert row["Host"] == "esx-01"
    assert row["Cluster"] == "Cluster-A"
    assert row["Guest OS"] == "Linux"


def test_build_vdisk_one_row_per_disk():
    c = _make_collector()
    c.vms = [{"name": "vm1", "urn": "urn:vm:1", "status": "stopped"}]
    c.vm_disks = {
        "urn:vm:1": [
            {"name": "disk0", "quantityGB": 40, "datastoreUrn": "urn:ds:1"},
            {"name": "disk1", "quantityGB": 60, "datastoreUrn": "urn:ds:1"},
        ]
    }
    c.datastore_map = {"urn:ds:1": "DS-A"}

    rows = c._build_vdisk()
    assert len(rows) == 2
    assert rows[0]["Power"] == "OFF"
    assert rows[0]["Datastore"] == "DS-A"
    assert rows[1]["Capacity (GB)"] == 60


def test_build_all_sheets_keys():
    c = _make_collector()
    sheets = c._build_all_sheets()
    expected = {
        "vSummary", "vInfo", "vCPU", "vMemory", "vDisk",
        "vNetwork", "vHost", "vCluster", "vDatastore", "vSwitch",
    }
    assert set(sheets.keys()) == expected
