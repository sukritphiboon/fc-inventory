"""Tests for the remaining collector helpers, sheet builders and orchestration.

These complement test_collector.py and require no network access.
"""

import pytest

from collector import (
    InventoryCollector,
    _flatten_dict,
    _build_row,
    _prettify_key,
)


def _make_collector():
    return InventoryCollector("host", "user", "pass", port=7443)


# ── _flatten_dict ───────────────────────────────────────────


def test_flatten_dict_nested_and_scalar_lists():
    d = {
        "a": 1,
        "b": {"c": 2, "d": {"e": 3}},
        "tags": ["x", "y"],
        "empty": [],
    }
    flat = _flatten_dict(d)
    assert flat["a"] == 1
    assert flat["b.c"] == 2
    assert flat["b.d.e"] == 3
    assert flat["tags"] == "x, y"
    assert flat["empty"] == ""


def test_flatten_dict_skips_list_of_dicts():
    d = {"nics": [{"ip": "1.1.1.1"}], "name": "vm"}
    flat = _flatten_dict(d)
    assert "nics" not in flat
    assert flat["name"] == "vm"


def test_flatten_dict_non_dict_returns_empty():
    assert _flatten_dict(None) == {}
    assert _flatten_dict([1, 2]) == {}


# ── _build_row / extras capture ─────────────────────────────


def test_build_row_maps_columns_and_captures_extras():
    from collections import OrderedDict
    field_map = OrderedDict([("Name", ["name"]), ("CPU", ["cpu.quantity"])])
    data = {"name": "vm1", "cpu": {"quantity": 4}, "extraField": "kept"}

    row = _build_row(data, field_map, extras_allowed=True)
    assert row["Name"] == "vm1"
    assert row["CPU"] == 4
    # Unmapped field is captured under a prettified key.
    assert any("kept" == v for v in row.values())


def test_build_row_without_extras():
    from collections import OrderedDict
    field_map = OrderedDict([("Name", ["name"])])
    data = {"name": "vm1", "ignored": "x"}
    row = _build_row(data, field_map, extras_allowed=False)
    assert list(row.keys()) == ["Name"]


def test_prettify_key_camel_and_dotted():
    assert _prettify_key("vmConfig.cpu.quantity") == "Cpu - Quantity"
    assert _prettify_key("bmcIp") == "Bmc Ip"


# ── _build_vsummary ─────────────────────────────────────────


def test_build_vsummary_counts_power_and_clusters():
    c = _make_collector()
    c.hosts = [{"urn": "h1"}]
    c.clusters = [{"urn": "c1"}]
    c.datastores = []
    c.dvswitches = []
    c.portgroups = []
    vinfo = [
        {"Power State": "ON", "Cluster": "A"},
        {"Power State": "OFF", "Cluster": "A"},
        {"Power State": "ON", "Cluster": "B"},
    ]
    rows = c._build_vsummary(vinfo)
    items = {r["Item"]: r["Count"] for r in rows}
    assert items["Total VMs"] == 3
    assert items["Power ON"] == 2
    assert items["Power OFF"] == 1
    assert items["Total Hosts"] == 1
    # Per-cluster breakdown line for cluster A.
    a_line = next(r for r in rows if r["Item"].strip() == "A")
    assert "ON: 1" in a_line["Count"] and "OFF: 1" in a_line["Count"]


# ── _build_vcpu (socket math) ───────────────────────────────


def test_build_vcpu_socket_calculation():
    c = _make_collector()
    c.vms = [{"name": "vm1", "urn": "u1", "status": "running"}]
    c.vm_details = {
        "u1": {"vmConfig": {"cpu": {"quantity": 8, "coresPerSocket": 2}}}
    }
    rows = c._build_vcpu()
    assert rows[0]["Total CPUs"] == 8
    assert rows[0]["Sockets"] == 4


def test_build_vcpu_socket_handles_missing_values():
    c = _make_collector()
    c.vms = [{"name": "vm1", "urn": "u1", "status": "running"}]
    c.vm_details = {"u1": {"vmConfig": {"cpu": {}}}}
    rows = c._build_vcpu()
    assert rows[0]["Sockets"] == ""


# ── _build_vmemory ──────────────────────────────────────────


def test_build_vmemory_basic():
    c = _make_collector()
    c.vms = [{"name": "vm1", "urn": "u1", "status": "stopped"}]
    c.vm_details = {"u1": {"vmConfig": {"memory": {"quantityMB": 4096, "reservation": 1024}}}}
    rows = c._build_vmemory()
    assert rows[0]["Memory (MB)"] == 4096
    assert rows[0]["Reservation (MB)"] == 1024
    assert rows[0]["Power"] == "OFF"


# ── _build_vnetwork ─────────────────────────────────────────


def test_build_vnetwork_one_row_per_nic():
    c = _make_collector()
    c.vms = [{"name": "vm1", "urn": "u1", "status": "running", "uuid": "uu"}]
    c.vm_nics = {"u1": [
        {"name": "nic0", "mac": "aa", "ip": "10.0.0.1"},
        {"name": "nic1", "mac": "bb", "ip": "10.0.0.2"},
    ]}
    rows = c._build_vnetwork()
    assert len(rows) == 2
    assert rows[0]["MAC Address"] == "aa"
    assert rows[1]["IP Address"] == "10.0.0.2"
    assert rows[0]["VM UUID"] == "uu"


# ── _build_vhost ────────────────────────────────────────────


def test_build_vhost_counts_running_vms_fallback():
    c = _make_collector()
    c.hosts = [{"name": "esx1", "urn": "h1", "clusterUrn": "cl1"}]
    c.host_details = {}
    c.cluster_map = {"cl1": "Cluster-A"}
    # Two VMs located on h1; host has no runningVmCount so fallback count is used.
    c.vms = [{"locationUrn": "h1"}, {"locationUrn": "h1"}]
    rows = c._build_vhost()
    assert rows[0]["Host Name"] == "esx1"
    assert rows[0]["Cluster"] == "Cluster-A"
    assert rows[0]["Running VMs"] == 2


# ── _build_vcluster ─────────────────────────────────────────


def test_build_vcluster_counts_hosts():
    c = _make_collector()
    c.clusters = [{"name": "Cluster-A", "urn": "cl1", "isEnableHa": True}]
    c.hosts = [{"clusterUrn": "cl1"}, {"clusterUrn": "cl1"}, {"clusterUrn": "cl2"}]
    rows = c._build_vcluster()
    assert rows[0]["Cluster Name"] == "Cluster-A"
    assert rows[0]["HA Enabled"] is True
    assert rows[0]["Total Hosts"] == 2


# ── _build_vdatastore (unit conversion + used %) ────────────


def test_build_vdatastore_gb_direct_and_used_pct():
    c = _make_collector()
    c.datastores = [{
        "name": "DS1", "storageType": "SAN",
        "capacityGB": 100, "freeSpaceGB": 25, "status": "normal",
    }]
    rows = c._build_vdatastore()
    assert rows[0]["Capacity (GB)"] == 100
    assert rows[0]["Free (GB)"] == 25
    assert rows[0]["Used %"] == 75.0


def test_build_vdatastore_converts_mb_to_gb():
    c = _make_collector()
    c.datastores = [{"name": "DS2", "capacityMB": 2048, "freeSpaceMB": 1024}]
    rows = c._build_vdatastore()
    assert rows[0]["Capacity (GB)"] == 2.0
    assert rows[0]["Free (GB)"] == 1.0
    assert rows[0]["Used %"] == 50.0


def test_build_vdatastore_handles_zero_capacity():
    c = _make_collector()
    c.datastores = [{"name": "DS3", "capacityGB": 0, "freeSpaceGB": 0}]
    rows = c._build_vdatastore()
    # No divide-by-zero; used % left blank.
    assert rows[0]["Used %"] == ""


# ── _build_vswitch ──────────────────────────────────────────


def test_build_vswitch_combines_dvswitches_and_portgroups():
    c = _make_collector()
    c.dvswitches = [{"name": "dvs1", "mtu": 1500, "urn": "d1"}]
    c.portgroups = [{"name": "pg1", "vlanId": 10, "_dvswitch_name": "dvs1"}]
    rows = c._build_vswitch()
    assert rows[0]["Type"] == "DVSwitch"
    assert rows[1]["Type"] == "Port Group"
    assert rows[1]["VLAN ID"] == 10
    assert rows[1]["Parent"] == "dvs1"


# ── _build_lookup_maps ──────────────────────────────────────


def test_build_lookup_maps():
    c = _make_collector()
    c.hosts = [{"urn": "h1", "name": "esx1"}]
    c.clusters = [{"urn": "c1", "name": "Cluster-A"}]
    c.datastores = [{"urn": "d1", "name": "DS1"}]
    c._build_lookup_maps()
    assert c.host_map["h1"] == "esx1"
    assert c.cluster_map["c1"] == "Cluster-A"
    assert c.datastore_map["d1"] == "DS1"


# ── cancellation ────────────────────────────────────────────


def test_cancel_raises_in_update_progress():
    c = _make_collector()
    c.cancel()
    assert c.cancelled is True
    with pytest.raises(InterruptedError):
        c._update_progress(10, "step")


# ── collect_all orchestration (fully mocked client) ─────────


class FakeClient:
    """Minimal stand-in for FCClient covering the collect_all pipeline."""

    def __init__(self):
        self.logged_in = False
        self.logged_out = False

    def login(self):
        self.logged_in = True

    def logout(self):
        self.logged_out = True

    def get_sites(self):
        return [{"uri": "/site1"}]

    def get_clusters(self, site_uri):
        return [{"urn": "cl1", "name": "Cluster-A"}]

    def get_hosts(self, site_uri):
        return [{"uri": "/h1", "urn": "h1", "name": "esx1", "clusterUrn": "cl1"}]

    def get_host_detail(self, host_uri):
        return {"ip": "10.0.0.10"}

    def get_datastores(self, site_uri):
        return [{"urn": "d1", "name": "DS1", "capacityGB": 100, "freeSpaceGB": 50}]

    def get_dvswitches(self, site_uri):
        return [{"uri": "/dvs1", "urn": "dv1", "name": "dvs1"}]

    def get_portgroups(self, dvs_uri):
        return [{"name": "pg1", "vlanId": 10}]

    def get_site_portgroups(self, site_uri):
        return []

    def get_vms(self, site_uri):
        return [{"uri": "/vm1", "urn": "vm1", "name": "vm-a",
                 "status": "running", "locationUrn": "h1", "clusterUrn": "cl1"}]

    def get_vm_detail(self, vm_uri):
        return {
            "uuid": "uuid-a",
            "vmConfig": {
                "cpu": {"quantity": 4, "coresPerSocket": 2},
                "memory": {"quantityMB": 8192},
                "nics": [{"name": "nic0", "ip": "10.0.0.50"}],
                "disks": [{"name": "disk0", "quantityGB": 40}],
            },
        }


def test_collect_all_happy_path():
    c = _make_collector()
    c.client = FakeClient()

    result = c.collect_all()

    assert c.progress["status"] == "done"
    assert c.progress["percent"] == 100
    assert c.client.logged_in and c.client.logged_out

    # All expected sheets present and populated.
    assert set(result.keys()) == {
        "vSummary", "vInfo", "vCPU", "vMemory", "vDisk",
        "vNetwork", "vHost", "vCluster", "vDatastore", "vSwitch",
    }
    assert result["vInfo"][0]["VM Name"] == "vm-a"
    assert result["vInfo"][0]["CPUs"] == 4
    assert result["vInfo"][0]["Total Disk (GB)"] == 40
    assert result["vDisk"][0]["Disk Name"] == "disk0"
    assert result["vNetwork"][0]["IP Address"] == "10.0.0.50"
    assert result["vHost"][0]["Host Name"] == "esx1"


def test_collect_all_propagates_and_marks_error():
    c = _make_collector()

    class BoomClient(FakeClient):
        def get_sites(self):
            raise RuntimeError("api down")

    c.client = BoomClient()
    with pytest.raises(RuntimeError):
        c.collect_all()
    assert c.progress["status"] == "error"
    assert "api down" in c.progress["error"]
    assert c.client.logged_out  # cleanup logout still attempted


def test_collect_all_cancelled_midway():
    c = _make_collector()

    class CancelClient(FakeClient):
        def __init__(self, collector):
            super().__init__()
            self._collector = collector

        def get_sites(self):
            # Cancel after login so the next _update_progress raises.
            self._collector.cancel()
            return super().get_sites()

    c.client = CancelClient(c)
    with pytest.raises(InterruptedError):
        c.collect_all()
    assert c.progress["status"] == "cancelled"
