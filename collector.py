"""
Inventory Collector
Orchestrates data collection from FusionCompute and builds flat data for Excel sheets.
"""

import logging
from fc_client import FCClient

logger = logging.getLogger(__name__)


def _extract_id(urn):
    """Extract the ID portion from a FusionCompute URN string."""
    if urn:
        return urn.split(":")[-1]
    return ""


class InventoryCollector:
    """Collects inventory data from FusionCompute and flattens it for Excel export."""

    def __init__(self, host, username, password, port=7443):
        self.client = FCClient(host, username, password, port=port)
        self.cancelled = False
        self.progress = {
            "status": "idle",
            "current_step": "",
            "percent": 0,
            "error": None,
        }

        # Raw data from API
        self.sites = []
        self.clusters = []
        self.hosts = []
        self.host_details = {}
        self.vms = []
        self.vm_details = {}
        self.vm_nics = {}
        self.vm_disks = {}
        self.datastores = []
        self.dvswitches = []
        self.portgroups = []

        # Lookup maps (URN -> name)
        self.host_map = {}
        self.cluster_map = {}
        self.datastore_map = {}

    def cancel(self):
        """Request cancellation of the running collection."""
        self.cancelled = True

    def _check_cancelled(self):
        """Raise if cancellation was requested."""
        if self.cancelled:
            raise InterruptedError("Collection cancelled by user.")

    def _update_progress(self, percent, step):
        self._check_cancelled()
        self.progress["percent"] = percent
        self.progress["current_step"] = step
        logger.info(f"[{percent}%] {step}")

    def collect_all(self):
        """Run the full collection pipeline. Returns dict of sheet data."""
        self.progress["status"] = "running"

        try:
            # Step 1: Login
            self._update_progress(5, "Logging in to FusionCompute...")
            self.client.login()

            # Step 2: Get sites
            self._update_progress(10, "Fetching sites...")
            self.sites = self.client.get_sites()

            for site in self.sites:
                site_uri = site.get("uri", "")

                # Step 3: Clusters
                self._update_progress(15, "Fetching clusters...")
                self.clusters.extend(self.client.get_clusters(site_uri))

                # Step 4: Hosts
                self._update_progress(20, "Fetching hosts...")
                hosts = self.client.get_hosts(site_uri)
                self.hosts.extend(hosts)

                # Fetch host details
                for i, host in enumerate(hosts):
                    host_uri = host.get("uri", "")
                    host_name = host.get("name", host_uri)
                    pct = 20 + int((i / max(len(hosts), 1)) * 10)
                    self._update_progress(pct, f"Fetching host detail: {host_name}...")
                    try:
                        detail = self.client.get_host_detail(host_uri)
                        self.host_details[host.get("urn")] = detail
                    except Exception as e:
                        logger.warning(f"Failed to get host detail {host_name}: {e}")

                # Step 5: Datastores
                self._update_progress(35, "Fetching datastores...")
                self.datastores.extend(self.client.get_datastores(site_uri))

                # Step 6: Networks
                self._update_progress(40, "Fetching networks...")
                dvswitches = self.client.get_dvswitches(site_uri)
                self.dvswitches.extend(dvswitches)

                for dvs in dvswitches:
                    dvs_uri = dvs.get("uri", "")
                    dvs_name = dvs.get("name", dvs_uri)
                    try:
                        pgs = self.client.get_portgroups(dvs_uri)
                        for pg in pgs:
                            pg["_dvswitch_name"] = dvs_name
                        self.portgroups.extend(pgs)
                    except Exception as e:
                        logger.warning(f"Failed to get portgroups for DVS {dvs_name}: {e}")

                # Step 7: VMs
                self._update_progress(45, "Fetching VM list...")
                vms = self.client.get_vms(site_uri)
                self.vms.extend(vms)

                # Step 8: VM details, NICs, disks (the slowest part)
                total_vms = len(vms)
                for i, vm in enumerate(vms):
                    vm_uri = vm.get("uri", "")
                    vm_name = vm.get("name", vm_uri)
                    vm_urn = vm.get("urn", "")
                    pct = 50 + int((i / max(total_vms, 1)) * 40)
                    self._update_progress(pct, f"Fetching VM detail ({i+1}/{total_vms}): {vm_name}...")

                    try:
                        detail = self.client.get_vm_detail(vm_uri)
                        self.vm_details[vm_urn] = detail

                        # Log first VM's full detail for debugging field names
                        if i == 0:
                            import json
                            logger.info(f"Sample VM detail keys: {list(detail.keys())}")
                            vm_config = detail.get("vmConfig", {})
                            if vm_config:
                                logger.info(f"vmConfig keys: {list(vm_config.keys())}")

                        # Extract NICs from VM detail (API /nics endpoint returns 404)
                        vm_config = detail.get("vmConfig", detail)
                        nics = vm_config.get("nics", detail.get("nics", []))
                        if nics:
                            self.vm_nics[vm_urn] = nics
                        else:
                            # Try separate API as fallback
                            try:
                                self.vm_nics[vm_urn] = self.client.get_vm_nics(vm_uri)
                            except Exception:
                                self.vm_nics[vm_urn] = []

                        # Extract Disks from VM detail
                        disks = vm_config.get("disks", vm_config.get("volumes", detail.get("disks", detail.get("volumes", []))))
                        if disks:
                            self.vm_disks[vm_urn] = disks
                        else:
                            # Try separate API as fallback
                            try:
                                self.vm_disks[vm_urn] = self.client.get_vm_disks(vm_uri)
                            except Exception:
                                self.vm_disks[vm_urn] = []

                    except Exception as e:
                        logger.warning(f"Failed to get VM detail {vm_name}: {e}")

            # Step 9: Build lookup maps and flatten data
            self._update_progress(92, "Processing collected data...")
            self._build_lookup_maps()
            result = self._build_all_sheets()

            # Step 10: Logout
            self._update_progress(98, "Logging out...")
            self.client.logout()

            self.progress["percent"] = 100
            self.progress["current_step"] = "Collection complete!"
            self.progress["status"] = "done"

            return result

        except Exception as e:
            logger.exception("Collection failed")
            self.progress["status"] = "error"
            self.progress["error"] = str(e)
            self.progress["current_step"] = "Error occurred"
            try:
                self.client.logout()
            except Exception:
                pass
            raise

    def _build_lookup_maps(self):
        """Build URN -> name lookup maps for cross-referencing."""
        for h in self.hosts:
            self.host_map[h.get("urn", "")] = h.get("name", "")
        for c in self.clusters:
            self.cluster_map[c.get("urn", "")] = c.get("name", "")
        for d in self.datastores:
            self.datastore_map[d.get("urn", "")] = d.get("name", "")

    def _build_all_sheets(self):
        """Build all Excel sheet data from collected raw data."""
        vinfo = self._build_vinfo()
        return {
            "vSummary": self._build_vsummary(vinfo),
            "vInfo": vinfo,
            "vCPU": self._build_vcpu(),
            "vMemory": self._build_vmemory(),
            "vDisk": self._build_vdisk(),
            "vNetwork": self._build_vnetwork(),
            "vHost": self._build_vhost(),
            "vCluster": self._build_vcluster(),
            "vDatastore": self._build_vdatastore(),
            "vSwitch": self._build_vswitch(),
        }

    # ── Sheet builders ───────────────────────────────────

    def _build_vsummary(self, vinfo):
        """Build summary sheet with power on/off counts per cluster."""
        from collections import Counter
        power_count = Counter()
        cluster_power = {}

        for vm in vinfo:
            power = vm.get("Power State", "")
            cluster = vm.get("Cluster", "N/A") or "N/A"
            power_count[power] += 1

            if cluster not in cluster_power:
                cluster_power[cluster] = {"ON": 0, "OFF": 0, "Other": 0}
            if power == "ON":
                cluster_power[cluster]["ON"] += 1
            elif power == "OFF":
                cluster_power[cluster]["OFF"] += 1
            else:
                cluster_power[cluster]["Other"] += 1

        rows = [
            {"Item": "Total VMs", "Count": len(vinfo)},
            {"Item": "Power ON", "Count": power_count.get("ON", 0)},
            {"Item": "Power OFF", "Count": power_count.get("OFF", 0)},
            {"Item": "Total Hosts", "Count": len(self.hosts)},
            {"Item": "Total Clusters", "Count": len(self.clusters)},
            {"Item": "Total Datastores", "Count": len(self.datastores)},
            {"Item": "", "Count": ""},
            {"Item": "=== Power State by Cluster ===", "Count": ""},
        ]
        for cluster, counts in sorted(cluster_power.items()):
            rows.append({
                "Item": f"  {cluster}",
                "Count": f"ON: {counts['ON']}  OFF: {counts['OFF']}",
            })

        return rows

    def _get_vm_config(self, vm_urn):
        """Get vmConfig from detail, or empty dict."""
        detail = self.vm_details.get(vm_urn, {})
        return detail.get("vmConfig", detail)

    def _build_vinfo(self):
        rows = []
        for vm in self.vms:
            urn = vm.get("urn", "")
            config = self._get_vm_config(urn)
            cpu = config.get("cpu", {})
            memory = config.get("memory", {})
            disks = self.vm_disks.get(urn, [])
            nics = self.vm_nics.get(urn, [])

            total_disk_gb = sum(d.get("quantityGB", 0) for d in disks)
            ip_list = ", ".join(
                n.get("ip", "") for n in nics if n.get("ip")
            )

            status = vm.get("status", "")
            power = "ON" if status in ("running", "started") else "OFF" if status in ("stopped", "shutOff") else status

            rows.append({
                "VM Name": vm.get("name", ""),
                "Power State": power,
                "Status": status,
                "Guest OS": vm.get("osOptions", {}).get("osType", config.get("osOptions", {}).get("osType", "")),
                "CPUs": cpu.get("quantity", ""),
                "Cores Per Socket": cpu.get("coresPerSocket", ""),
                "Memory (MB)": memory.get("quantityMB", ""),
                "Total Disk (GB)": total_disk_gb,
                "NICs": len(nics),
                "IP Addresses": ip_list,
                "Host": self.host_map.get(vm.get("locationUrn", vm.get("hostUrn", "")), ""),
                "Cluster": self.cluster_map.get(vm.get("clusterUrn", ""), ""),
                "VM Tools": vm.get("vmToolsVersion", config.get("vmToolsVersion", "")),
                "Description": vm.get("description", ""),
                "Create Date": vm.get("createTime", ""),
                "URN": urn,
            })
        return rows

    def _vm_power(self, vm):
        status = vm.get("status", "")
        return "ON" if status in ("running", "started") else "OFF" if status in ("stopped", "shutOff") else status

    def _build_vcpu(self):
        rows = []
        for vm in self.vms:
            urn = vm.get("urn", "")
            config = self._get_vm_config(urn)
            cpu = config.get("cpu", {})
            rows.append({
                "VM Name": vm.get("name", ""),
                "Power": self._vm_power(vm),
                "Total CPUs": cpu.get("quantity", ""),
                "Cores Per Socket": cpu.get("coresPerSocket", ""),
                "Sockets": cpu.get("quantity", 0) // max(cpu.get("coresPerSocket", 1), 1) if cpu.get("quantity") else "",
                "CPU Reservation (MHz)": cpu.get("reservation", ""),
                "CPU Limit (MHz)": cpu.get("limit", ""),
                "CPU Weight": cpu.get("weight", ""),
                "Host": self.host_map.get(vm.get("locationUrn", vm.get("hostUrn", "")), ""),
                "Cluster": self.cluster_map.get(vm.get("clusterUrn", ""), ""),
            })
        return rows

    def _build_vmemory(self):
        rows = []
        for vm in self.vms:
            urn = vm.get("urn", "")
            config = self._get_vm_config(urn)
            mem = config.get("memory", {})
            rows.append({
                "VM Name": vm.get("name", ""),
                "Power": self._vm_power(vm),
                "Memory (MB)": mem.get("quantityMB", ""),
                "Reservation (MB)": mem.get("reservation", ""),
                "Limit (MB)": mem.get("limit", ""),
                "Weight": mem.get("weight", ""),
                "Host": self.host_map.get(vm.get("locationUrn", vm.get("hostUrn", "")), ""),
                "Cluster": self.cluster_map.get(vm.get("clusterUrn", ""), ""),
            })
        return rows

    def _build_vdisk(self):
        rows = []
        for vm in self.vms:
            urn = vm.get("urn", "")
            disks = self.vm_disks.get(urn, [])
            for disk in disks:
                ds_urns = disk.get("datastoreUrn", "")
                rows.append({
                    "VM Name": vm.get("name", ""),
                    "Power": self._vm_power(vm),
                    "Disk Name": disk.get("volumeName", disk.get("name", "")),
                    "Capacity (GB)": disk.get("quantityGB", ""),
                    "Provisioning Type": disk.get("policyId", disk.get("thinFlag", "")),
                    "Datastore": self.datastore_map.get(ds_urns, ds_urns),
                    "Bus Type": disk.get("busType", ""),
                    "Sequence Num": disk.get("sequenceNum", ""),
                    "URN": disk.get("urn", ""),
                })
        return rows

    def _build_vnetwork(self):
        rows = []
        for vm in self.vms:
            urn = vm.get("urn", "")
            nics = self.vm_nics.get(urn, [])
            for nic in nics:
                rows.append({
                    "VM Name": vm.get("name", ""),
                    "Power": self._vm_power(vm),
                    "NIC Name": nic.get("name", ""),
                    "MAC Address": nic.get("mac", ""),
                    "Port Group": nic.get("portGroupName", nic.get("portGroupUrn", "")),
                    "IP Address": nic.get("ip", ""),
                    "NIC Type": nic.get("nicType", nic.get("virtualNicType", "")),
                    "Connected": nic.get("connected", ""),
                    "URN": nic.get("urn", ""),
                })
        return rows

    def _build_vhost(self):
        rows = []
        for host in self.hosts:
            detail = self.host_details.get(host.get("urn", ""), {})
            rows.append({
                "Host Name": host.get("name", ""),
                "IP Address": host.get("ip", ""),
                "Status": host.get("status", ""),
                "Cluster": self.cluster_map.get(host.get("clusterUrn", ""), ""),
                "CPU Model": detail.get("cpuModel", host.get("cpuModel", "")),
                "CPU Cores": detail.get("cpuQuantity", host.get("cpuQuantity", "")),
                "CPU MHz": detail.get("cpuMHz", host.get("cpuMHz", "")),
                "Memory Total (MB)": detail.get("memoryQuantityMB", host.get("memoryQuantityMB", "")),
                "Memory Used (MB)": detail.get("memoryUsedMB", ""),
                "Running VMs": detail.get("runningVmCount", ""),
                "Maintenance Mode": host.get("isMaintaining", ""),
                "Hypervisor": host.get("hypervisor", ""),
                "BMC IP": detail.get("bmcIp", host.get("bmcIp", "")),
                "URN": host.get("urn", ""),
            })
        return rows

    def _build_vcluster(self):
        rows = []
        for cluster in self.clusters:
            rows.append({
                "Cluster Name": cluster.get("name", ""),
                "Description": cluster.get("description", ""),
                "Tag": cluster.get("tag", ""),
                "HA Enabled": cluster.get("isHA", ""),
                "DRS Enabled": cluster.get("isDRS", ""),
                "Total Hosts": cluster.get("hostNum", ""),
                "URN": cluster.get("urn", ""),
            })
        return rows

    def _build_vdatastore(self):
        rows = []
        for ds in self.datastores:
            capacity_gb = ds.get("capacityGB", ds.get("totalSizeMB", 0) / 1024 if ds.get("totalSizeMB") else "")
            free_gb = ds.get("freeGB", ds.get("freeSizeMB", 0) / 1024 if ds.get("freeSizeMB") else "")
            used_pct = ""
            if isinstance(capacity_gb, (int, float)) and capacity_gb > 0 and isinstance(free_gb, (int, float)):
                used_pct = round((1 - free_gb / capacity_gb) * 100, 1)

            rows.append({
                "Datastore Name": ds.get("name", ""),
                "Storage Type": ds.get("storageType", ""),
                "Capacity (GB)": capacity_gb,
                "Free (GB)": free_gb,
                "Used %": used_pct,
                "Status": ds.get("status", ""),
                "Thin Provisioning": ds.get("thinProvisionSupport", ""),
                "URN": ds.get("urn", ""),
            })
        return rows

    def _build_vswitch(self):
        rows = []
        for dvs in self.dvswitches:
            rows.append({
                "Name": dvs.get("name", ""),
                "Type": "DVSwitch",
                "VLAN ID": "",
                "MTU": dvs.get("mtu", ""),
                "Description": dvs.get("description", ""),
                "URN": dvs.get("urn", ""),
            })
        for pg in self.portgroups:
            rows.append({
                "Name": pg.get("name", ""),
                "Type": "Port Group",
                "VLAN ID": pg.get("vlanId", ""),
                "MTU": pg.get("mtu", ""),
                "Description": pg.get("description", pg.get("_dvswitch_name", "")),
                "URN": pg.get("urn", ""),
            })
        return rows
