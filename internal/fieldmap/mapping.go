// Package fieldmap implements the "hybrid field mapping" used by the
// collector to flatten loosely-typed FusionCompute JSON responses into
// fixed Excel columns. Each column maps to a list of candidate JSON paths
// (in priority order); the first path that resolves to a non-empty value
// wins. This is a 1:1 port of the OrderedDict tables in
// collector.py:76-184 and the helper functions in collector.py:21-62.
package fieldmap

// FieldMap is a column-name -> candidate-paths mapping. The order of
// paths encodes priority: the first non-empty wins.
//
// We intentionally use map[string][]string instead of an ordered structure
// because the order of *keys* in the output is decided by the Excel
// builder (union-of-keys, dedup, preserve first-seen order). The order of
// *paths* within a single key is what matters for value resolution.
type FieldMap = map[string][]string

// VMFields -> vInfo sheet.
var VMFields = FieldMap{
	"VM Name":          {"name"},
	"Guest OS":         {"osOptions.osType", "vmConfig.osOptions.osType"},
	"CPUs":             {"vmConfig.cpu.quantity", "cpu.quantity"},
	"Cores Per Socket": {"vmConfig.cpu.coresPerSocket", "cpu.coresPerSocket"},
	"Memory (MB)":      {"vmConfig.memory.quantityMB", "memory.quantityMB"},
	"VM Tools":         {"toolsVersion", "pvDriverStatus", "toolInstallStatus", "vmToolsVersion"},
	"UUID":             {"uuid"},
	"Description":      {"description"},
	"Create Date":      {"createTime"},
	"Host URN":         {"locationUrn", "hostUrn", "location"},
	"Cluster URN":      {"clusterUrn"},
	"URN":              {"urn"},
}

// CPUFields -> vCPU sheet.
var CPUFields = FieldMap{
	"VM Name":             {"name"},
	"Total CPUs":          {"vmConfig.cpu.quantity"},
	"Cores Per Socket":    {"vmConfig.cpu.coresPerSocket"},
	"CPU Reservation MHz": {"vmConfig.cpu.reservation"},
	"CPU Limit MHz":       {"vmConfig.cpu.limit"},
	"CPU Weight":          {"vmConfig.cpu.weight"},
	"CPU Hot Plug":        {"vmConfig.cpu.cpuHotPlug"},
	"CPU Bind Type":       {"vmConfig.cpu.cpuBindType"},
	"CPU Policy":          {"vmConfig.cpu.cpuPolicy"},
}

// MemoryFields -> vMemory sheet.
var MemoryFields = FieldMap{
	"VM Name":          {"name"},
	"Memory (MB)":      {"vmConfig.memory.quantityMB"},
	"Reservation (MB)": {"vmConfig.memory.reservation"},
	"Limit (MB)":       {"vmConfig.memory.limit"},
	"Weight":           {"vmConfig.memory.weight"},
	"Memory Hot Plug":  {"vmConfig.memory.memHotPlug"},
	"Huge Page":        {"vmConfig.memory.hugePage"},
}

// DiskFields -> vDisk sheet. The first column "Disk Name" is filled by
// BuildDiskRows below because the source field varies (volumeUuid, urn,
// name) per FC version; this map is the column skeleton.
var DiskFields = FieldMap{
	"Disk Name":      {"volumeUuid", "volumeUrn", "name"},
	"Capacity (GB)":  {"quantityGB"},
	"Bus Type":       {"pciType", "busType"},
	"Thin Provision": {"isThin", "thinFlag"},
	"Sequence":       {"sequenceNum"},
	"Datastore URN":  {"datastoreUrn"},
	"Storage Type":   {"storageType"},
	"Independent":    {"indepDisk"},
	"Persistent":     {"persistentDisk"},
	"Volume URN":     {"volumeUrn"},
}

// NICFields -> vNetwork sheet.
var NICFields = FieldMap{
	"NIC Name":          {"name"},
	"MAC Address":       {"mac"},
	"IP Address":        {"ip"},
	"IP List":           {"ipList"},
	"IPv6":              {"ipv6s"},
	"Port Group":        {"portGroupName"},
	"Port Group URN":    {"portGroupUrn"},
	"Port Group Type":   {"portGroupType"},
	"VLAN Range":        {"portGroupVlanRange"},
	"Sequence":          {"sequenceNum"},
	"VirtIO":            {"virtIo"},
	"NIC Type":          {"nicType", "virtualNicType"},
	"Connect at PowerOn": {"connectAtPowerOn"},
	"URN":               {"urn"},
}

// HostFields -> vHost sheet.
var HostFields = FieldMap{
	"Host Name":         {"name"},
	"IP Address":        {"ip"},
	"Status":            {"status"},
	"CPU Model":         {"cpuModel", "cpuType"},
	"CPU Cores":         {"cpuQuantity", "cpuCores"},
	"CPU MHz":           {"cpuMHz", "cpuFrequency"},
	"Memory Total (MB)": {"memoryQuantityMB", "memoryCapacity", "memResource.totalSizeMB"},
	"Memory Used (MB)":  {"memoryUsedMB", "memResource.usedSizeMB"},
	"Running VMs":       {"runningVmCount"},
	"Cluster URN":       {"clusterUrn"},
	"BMC IP":            {"bmcIp"},
	"Maintenance":       {"isMaintaining"},
	"Hypervisor":        {"hypervisor"},
	"URN":               {"urn"},
}

// ClusterFields -> vCluster sheet.
var ClusterFields = FieldMap{
	"Cluster Name":       {"name"},
	"Description":        {"description"},
	"Tag":                {"tag"},
	"HA Enabled":         {"isEnableHa", "isHA"},
	"DRS Enabled":        {"isEnableDrs", "isDRS"},
	"Mem Overcommit":     {"isMemOvercommit"},
	"Auto Adjust NUMA":   {"isAutoAdjustNuma"},
	"Resource Strategy":  {"resStrategy"},
	"DRS Level":          {"drsSetting.drsLevel"},
	"CPU Reservation":    {"haResSetting.cpuReservation"},
	"Memory Reservation": {"haResSetting.memoryReservation"},
	"URN":                {"urn"},
}

// DatastoreFields -> vDatastore sheet.
var DatastoreFields = FieldMap{
	"Datastore Name": {"name"},
	"Storage Type":   {"storageType"},
	"Capacity (GB)":  {"capacityGB"},
	"Free (GB)":      {"freeSpaceGB", "freeSpace", "freeCapacityGB", "freeSizeGB"},
	"Status":         {"status"},
	"Thin Support":   {"thinProvisionSupport"},
	"Description":    {"description"},
	"URN":            {"urn"},
}
