package collector

import (
	"fmt"

	"github.com/kimzhong/fc-inventory/internal/fieldmap"
)

// buildVSummary produces the vSummary sheet: counts + per-cluster power
// state breakdown. Mirrors _build_vsummary (collector.py:480-516).
func (c *Collector) buildVSummary(vinfo []map[string]any) []map[string]any {
	powerCount := map[string]int{}
	clusterPower := map[string]map[string]int{}

	for _, vm := range vinfo {
		power, _ := vm["Power State"].(string)
		cluster, _ := vm["Cluster"].(string)
		if cluster == "" {
			cluster = "N/A"
		}
		powerCount[power]++
		cp, ok := clusterPower[cluster]
		if !ok {
			cp = map[string]int{"ON": 0, "OFF": 0, "Other": 0}
			clusterPower[cluster] = cp
		}
		switch power {
		case "ON":
			cp["ON"]++
		case "OFF":
			cp["OFF"]++
		default:
			cp["Other"]++
		}
	}

	rows := []map[string]any{
		{"Item": "Total VMs", "Count": len(vinfo)},
		{"Item": "Power ON", "Count": powerCount["ON"]},
		{"Item": "Power OFF", "Count": powerCount["OFF"]},
		{"Item": "Total Hosts", "Count": len(c.hosts)},
		{"Item": "Total Clusters", "Count": len(c.clusters)},
		{"Item": "Total Datastores", "Count": len(c.datastores)},
		{"Item": "Total DVSwitches", "Count": len(c.dvswitches)},
		{"Item": "Total Port Groups", "Count": len(c.portgroups)},
		{"Item": "", "Count": ""},
		{"Item": "=== Power State by Cluster ===", "Count": ""},
	}
	for cluster, counts := range clusterPower {
		rows = append(rows, map[string]any{
			"Item":  "  " + cluster,
			"Count": fmt.Sprintf("ON: %d  OFF: %d", counts["ON"], counts["OFF"]),
		})
	}
	return rows
}

// buildVInfo: the vInfo sheet (one row per VM, summary columns).
func (c *Collector) buildVInfo() []map[string]any {
	rows := []map[string]any{}
	for _, vm := range c.vms {
		merged := c.mergedVM(vm.Urn)
		urn := vm.Urn
		disks := c.disksAsMaps(urn)
		nics := c.nicsAsMaps(urn)

		totalDisk := sumDiskGB(disks)
		var ipList string
		for i, n := range nics {
			ip, _ := n["ip"].(string)
			if ip == "" {
				continue
			}
			if i == 0 {
				ipList = ip
			} else {
				ipList += ", " + ip
			}
		}

		hostUrn := fieldmap.TryString(merged, []string{"locationUrn", "hostUrn"})
		hostName := c.hostMap[hostUrn]
		if hostName == "" {
			hostName = fieldmap.TryString(merged, []string{"hostName", "locationName"})
		}
		clusterUrn := fieldmap.TryString(merged, []string{"clusterUrn"})
		clusterName := c.clusterMap[clusterUrn]
		if clusterName == "" {
			clusterName = fieldmap.TryString(merged, []string{"clusterName"})
		}

		rows = append(rows, map[string]any{
			"VM Name":         fieldmap.TryString(merged, []string{"name"}),
			"UUID":            fieldmap.TryString(merged, []string{"uuid"}),
			"Power State":     fieldmap.PowerState(fieldmap.TryPaths(merged, []string{"status"})),
			"Status":          fieldmap.TryString(merged, []string{"status"}),
			"Guest OS":        fieldmap.TryString(merged, []string{"osOptions.osType", "vmConfig.osOptions.osType"}),
			"CPUs":            fieldmap.TryString(merged, []string{"vmConfig.cpu.quantity", "cpu.quantity"}),
			"Cores Per Socket": fieldmap.TryString(merged, []string{"vmConfig.cpu.coresPerSocket"}),
			"Memory (MB)":     fieldmap.TryString(merged, []string{"vmConfig.memory.quantityMB", "memory.quantityMB"}),
			"Total Disk (GB)": totalDisk,
			"NICs":            len(nics),
			"IP Addresses":    ipList,
			"Host":            hostName,
			"Cluster":         clusterName,
			"VM Tools":        fieldmap.TryString(merged, []string{"toolsVersion", "pvDriverStatus", "toolInstallStatus"}),
			"Description":     fieldmap.TryString(merged, []string{"description"}),
			"Create Date":     fieldmap.TryString(merged, []string{"createTime"}),
			"URN":             urn,
		})
	}
	return rows
}

// buildVCPU: vCPU sheet, one row per VM. Adds a "Sockets" column derived
// from CPUs / CoresPerSocket.
func (c *Collector) buildVCPU() []map[string]any {
	rows := []map[string]any{}
	for _, vm := range c.vms {
		merged := c.mergedVM(vm.Urn)
		qty := fieldmap.TryPaths(merged, []string{"vmConfig.cpu.quantity"})
		cps := fieldmap.TryPaths(merged, []string{"vmConfig.cpu.coresPerSocket"})

		var sockets any = ""
		if qty != nil && cps != nil {
			q, cq := toInt(qty), toInt(cps)
			if q > 0 && cq > 0 {
				sockets = q / cq
				if q%cq != 0 {
					sockets = fmt.Sprintf("%.2f", float64(q)/float64(cq))
				}
			}
		}

		hostUrn := fieldmap.TryString(merged, []string{"locationUrn", "hostUrn"})
		clusterUrn := fieldmap.TryString(merged, []string{"clusterUrn"})

		rows = append(rows, map[string]any{
			"VM Name":              fieldmap.TryString(merged, []string{"name"}),
			"UUID":                 fieldmap.TryString(merged, []string{"uuid"}),
			"Power":                fieldmap.PowerState(fieldmap.TryPaths(merged, []string{"status"})),
			"Total CPUs":           qty,
			"Cores Per Socket":     cps,
			"Sockets":              sockets,
			"CPU Reservation (MHz)": fieldmap.TryString(merged, []string{"vmConfig.cpu.reservation"}),
			"CPU Limit (MHz)":      fieldmap.TryString(merged, []string{"vmConfig.cpu.limit"}),
			"CPU Weight":           fieldmap.TryString(merged, []string{"vmConfig.cpu.weight"}),
			"CPU Hot Plug":         fieldmap.TryString(merged, []string{"vmConfig.cpu.cpuHotPlug"}),
			"CPU Bind Type":        fieldmap.TryString(merged, []string{"vmConfig.cpu.cpuBindType"}),
			"CPU Policy":           fieldmap.TryString(merged, []string{"vmConfig.cpu.cpuPolicy"}),
			"Host":                 c.hostMap[hostUrn],
			"Cluster":              c.clusterMap[clusterUrn],
		})
	}
	return rows
}

// buildVMemory: vMemory sheet.
func (c *Collector) buildVMemory() []map[string]any {
	rows := []map[string]any{}
	for _, vm := range c.vms {
		merged := c.mergedVM(vm.Urn)
		hostUrn := fieldmap.TryString(merged, []string{"locationUrn", "hostUrn"})
		clusterUrn := fieldmap.TryString(merged, []string{"clusterUrn"})

		rows = append(rows, map[string]any{
			"VM Name":         fieldmap.TryString(merged, []string{"name"}),
			"UUID":            fieldmap.TryString(merged, []string{"uuid"}),
			"Power":           fieldmap.PowerState(fieldmap.TryPaths(merged, []string{"status"})),
			"Memory (MB)":     fieldmap.TryString(merged, []string{"vmConfig.memory.quantityMB"}),
			"Reservation (MB)": fieldmap.TryString(merged, []string{"vmConfig.memory.reservation"}),
			"Limit (MB)":      fieldmap.TryString(merged, []string{"vmConfig.memory.limit"}),
			"Weight":          fieldmap.TryString(merged, []string{"vmConfig.memory.weight"}),
			"Memory Hot Plug": fieldmap.TryString(merged, []string{"vmConfig.memory.memHotPlug"}),
			"Huge Page":       fieldmap.TryString(merged, []string{"vmConfig.memory.hugePage"}),
			"Host":            c.hostMap[hostUrn],
			"Cluster":         c.clusterMap[clusterUrn],
		})
	}
	return rows
}

// buildVDisk: vDisk sheet, one row per VM disk.
func (c *Collector) buildVDisk() []map[string]any {
	rows := []map[string]any{}
	for _, vm := range c.vms {
		disks := c.disksAsMaps(vm.Urn)
		for _, disk := range disks {
			dsUrn := fieldmap.TryString(disk, []string{"datastoreUrn"})
			rows = append(rows, map[string]any{
				"VM Name":       vm.Name,
				"Power":         fieldmap.PowerState(vm.Status),
				"Disk Name":     fieldmap.TryString(disk, []string{"name", "volumeName"}),
				"Disk UUID":     fieldmap.TryString(disk, []string{"volumeUuid"}),
				"Capacity (GB)": fieldmap.TryString(disk, []string{"quantityGB"}),
				"Bus Type":      fieldmap.TryString(disk, []string{"pciType", "busType"}),
				"Thin Provision": fieldmap.TryString(disk, []string{"isThin", "thinFlag"}),
				"Sequence":      fieldmap.TryString(disk, []string{"sequenceNum"}),
				"Datastore":     c.datastoreMap[dsUrn],
				"Datastore URN": dsUrn,
				"Storage Type":  fieldmap.TryString(disk, []string{"storageType"}),
				"Independent":   fieldmap.TryString(disk, []string{"indepDisk"}),
				"Persistent":    fieldmap.TryString(disk, []string{"persistentDisk"}),
				"Volume URN":    fieldmap.TryString(disk, []string{"volumeUrn"}),
			})
		}
	}
	return rows
}

// buildVNetwork: vNetwork sheet, one row per VM NIC.
func (c *Collector) buildVNetwork() []map[string]any {
	rows := []map[string]any{}
	for _, vm := range c.vms {
		merged := c.mergedVM(vm.Urn)
		vmUUID := fieldmap.TryString(merged, []string{"uuid"})
		nics := c.nicsAsMaps(vm.Urn)
		for _, nic := range nics {
			rows = append(rows, map[string]any{
				"VM Name":           vm.Name,
				"VM UUID":           vmUUID,
				"Power":             fieldmap.PowerState(vm.Status),
				"NIC Name":          fieldmap.TryString(nic, []string{"name"}),
				"MAC Address":       fieldmap.TryString(nic, []string{"mac"}),
				"IP Address":        fieldmap.TryString(nic, []string{"ip"}),
				"IP List":           fieldmap.TryString(nic, []string{"ipList"}),
				"IPv6":              fieldmap.TryString(nic, []string{"ipv6s"}),
				"Port Group":        fieldmap.TryString(nic, []string{"portGroupName"}),
				"Port Group URN":    fieldmap.TryString(nic, []string{"portGroupUrn"}),
				"Port Group Type":   fieldmap.TryString(nic, []string{"portGroupType"}),
				"VLAN Range":        fieldmap.TryString(nic, []string{"portGroupVlanRange"}),
				"Sequence":          fieldmap.TryString(nic, []string{"sequenceNum"}),
				"VirtIO":            fieldmap.TryString(nic, []string{"virtIo"}),
				"NIC Type":          fieldmap.TryString(nic, []string{"nicType", "virtualNicType"}),
				"Connect at Power-On": fieldmap.TryString(nic, []string{"connectAtPowerOn"}),
				"URN":               fieldmap.TryString(nic, []string{"urn"}),
			})
		}
	}
	return rows
}

// buildVHost: vHost sheet. Also computes Running VMs locally when the
// FC payload doesn't include runningVmCount.
func (c *Collector) buildVHost() []map[string]any {
	hostVMCount := map[string]int{}
	for _, vm := range c.vms {
		h := vm.LocationUrn
		if h == "" {
			h = vm.HostUrn
		}
		if h != "" {
			hostVMCount[h]++
		}
	}

	rows := []map[string]any{}
	for _, host := range c.hosts {
		urn := host.Urn
		merged := c.mergedHost(urn)
		clusterUrn := fieldmap.TryString(merged, []string{"clusterUrn"})
		running := fieldmap.TryPaths(merged, []string{"runningVmCount"})
		if running == nil || running == "" {
			running = hostVMCount[urn]
		}
		rows = append(rows, map[string]any{
			"Host Name":         fieldmap.TryString(merged, []string{"name"}),
			"IP Address":        fieldmap.TryString(merged, []string{"ip"}),
			"Status":            fieldmap.TryString(merged, []string{"status"}),
			"Cluster":           c.clusterMap[clusterUrn],
			"CPU Model":         fieldmap.TryString(merged, []string{"cpuModel", "cpuType"}),
			"CPU Cores":         fieldmap.TryString(merged, []string{"cpuQuantity", "cpuCores"}),
			"CPU MHz":           fieldmap.TryString(merged, []string{"cpuMHz", "cpuFrequency"}),
			"Memory Total (MB)": fieldmap.TryString(merged, []string{"memoryQuantityMB", "memoryCapacity"}),
			"Memory Used (MB)":  fieldmap.TryString(merged, []string{"memoryUsedMB"}),
			"Running VMs":       running,
			"BMC IP":            fieldmap.TryString(merged, []string{"bmcIp"}),
			"Maintenance":       fieldmap.TryString(merged, []string{"isMaintaining"}),
			"Hypervisor":        fieldmap.TryString(merged, []string{"hypervisor"}),
			"URN":               urn,
		})
	}
	return rows
}

// buildVCluster: vCluster sheet. Includes Total Hosts derived from
// self.hosts when hostNum is missing.
func (c *Collector) buildVCluster() []map[string]any {
	clusterHostCount := map[string]int{}
	for _, h := range c.hosts {
		if h.ClusterUrn != "" {
			clusterHostCount[h.ClusterUrn]++
		}
	}

	rows := []map[string]any{}
	for _, cl := range c.clusters {
		urn := cl.Urn
		hostNum := fieldmap.TryPaths(cl, []string{"hostNum"})
		if hostNum == nil || hostNum == "" {
			hostNum = clusterHostCount[urn]
		}
		rows = append(rows, map[string]any{
			"Cluster Name":       fieldmap.TryString(cl, []string{"name"}),
			"Description":        fieldmap.TryString(cl, []string{"description"}),
			"Tag":                fieldmap.TryString(cl, []string{"tag"}),
			"HA Enabled":         fieldmap.TryString(cl, []string{"isEnableHa", "isHA"}),
			"DRS Enabled":        fieldmap.TryString(cl, []string{"isEnableDrs", "isDRS"}),
			"Mem Overcommit":     fieldmap.TryString(cl, []string{"isMemOvercommit"}),
			"Auto Adjust NUMA":   fieldmap.TryString(cl, []string{"isAutoAdjustNuma"}),
			"DRS Level":          fieldmap.TryString(cl, []string{"drsSetting.drsLevel"}),
			"CPU Reservation":    fieldmap.TryString(cl, []string{"haResSetting.cpuReservation"}),
			"Memory Reservation": fieldmap.TryString(cl, []string{"haResSetting.memoryReservation"}),
			"Total Hosts":        hostNum,
			"URN":                urn,
		})
	}
	return rows
}

// buildVDatastore: vDatastore sheet. Performs the GB-preferred / MB-fallback
// conversion and adds a Used % column (mirrors collector.py:723-742).
func (c *Collector) buildVDatastore() []map[string]any {
	rows := []map[string]any{}
	for _, ds := range c.datastores {
		cap := fieldmap.TryPaths(ds, []string{"capacityGB"})
		if cap == nil || cap == "" {
			capMB := fieldmap.TryPaths(ds, []string{"capacityMB", "totalSizeMB"})
			if capMB != nil && capMB != "" {
				if f, ok := toFloatOk(capMB); ok {
					cap = round2(f / 1024)
				}
			}
		}

		free := fieldmap.TryPaths(ds, []string{"freeSpaceGB", "freeSpace", "freeCapacityGB", "freeSizeGB"})
		if free == nil || free == "" {
			freeMB := fieldmap.TryPaths(ds, []string{"freeSpaceMB", "freeSizeMB"})
			if freeMB != nil && freeMB != "" {
				if f, ok := toFloatOk(freeMB); ok {
					free = round2(f / 1024)
				}
			}
		}

		var usedPct any = ""
		if cf, cok := toFloatOk(cap); cok && cf > 0 {
			if ff, fok := toFloatOk(free); fok {
				pct := (1 - ff/cf) * 100
				usedPct = round1(pct)
			}
		}

		rows = append(rows, map[string]any{
			"Datastore Name": fieldmap.TryString(ds, []string{"name"}),
			"Storage Type":   fieldmap.TryString(ds, []string{"storageType"}),
			"Capacity (GB)":  cap,
			"Free (GB)":      free,
			"Used %":         usedPct,
			"Status":         fieldmap.TryString(ds, []string{"status"}),
			"Thin Support":   fieldmap.TryString(ds, []string{"thinProvisionSupport"}),
			"Description":    fieldmap.TryString(ds, []string{"description"}),
			"URN":            fieldmap.TryString(ds, []string{"urn"}),
		})
	}
	return rows
}

// buildVSwitch: vSwitch sheet, one row per DVSwitch and one per PortGroup.
func (c *Collector) buildVSwitch() []map[string]any {
	rows := []map[string]any{}
	for _, dvs := range c.dvswitches {
		rows = append(rows, map[string]any{
			"Name":        dvs.Name,
			"Type":        "DVSwitch",
			"VLAN ID":     "",
			"MTU":         dvs.MTU,
			"Description": fieldmap.TryString(dvs, []string{"description"}),
			"Parent":      "",
			"URN":         dvs.Urn,
		})
	}
	for _, pg := range c.portgroups {
		rows = append(rows, map[string]any{
			"Name":        pg.Name,
			"Type":        "Port Group",
			"VLAN ID":     pg.VlanID,
			"MTU":         pg.MTU,
			"Description": fieldmap.TryString(pg, []string{"description"}),
			"Parent":      pg.DvswitchName,
			"URN":         pg.Urn,
		})
	}
	return rows
}

// nicsAsMaps converts the typed VMNic slice for a VM into a slice of
// generic maps so the fieldmap helpers can be applied uniformly.
func (c *Collector) nicsAsMaps(urn string) []map[string]any {
	nics := c.vmNics[urn]
	out := make([]map[string]any, 0, len(nics))
	for _, n := range nics {
		out = append(out, map[string]any{
			"name":              n.Name,
			"mac":               n.MAC,
			"ip":                n.IP,
			"ipList":            n.IPList,
			"ipv6s":             n.IPv6s,
			"portGroupName":     n.PortGroupName,
			"portGroupUrn":      n.PortGroupUrn,
			"portGroupType":     n.PortGroupType,
			"portGroupVlanRange": n.PortGroupVlanRange,
			"sequenceNum":       n.SequenceNum,
			"virtIo":            n.VirtIO,
			"nicType":           n.NICType,
			"virtualNicType":    n.VirtualNICType,
			"connectAtPowerOn":  n.ConnectAtPowerOn,
			"urn":               n.Urn,
		})
	}
	return out
}

func (c *Collector) disksAsMaps(urn string) []map[string]any {
	disks := c.vmDisks[urn]
	out := make([]map[string]any, 0, len(disks))
	for _, d := range disks {
		out = append(out, map[string]any{
			"volumeUrn":      d.VolumeUrn,
			"volumeUuid":     d.VolumeUuid,
			"name":           d.Name,
			"quantityGB":     d.QuantityGB,
			"pciType":        d.PCIType,
			"busType":        d.BusType,
			"isThin":         d.IsThin,
			"thinFlag":       d.ThinFlag,
			"sequenceNum":    d.SequenceNum,
			"datastoreUrn":   d.DatastoreUrn,
			"storageType":    d.StorageType,
			"indepDisk":      d.IndepDisk,
			"persistentDisk": d.PersistentDisk,
		})
	}
	return out
}

func toFloatOk(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		var f float64
		if _, err := fmt.Sscan(x, &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

func round1(f float64) float64 {
	return float64(int64(f*10+0.5)) / 10
}
