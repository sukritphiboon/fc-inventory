// Package fcclient is the REST adapter for Huawei FusionCompute VRM.
// It mirrors the fc_client.py module: an auto-detect login matrix,
// paginated GET helpers, and per-resource fetchers used by the collector.
package fcclient

// Site is the top-level VRM site.
type Site struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
	ID   string `json:"id"`
}

// Cluster represents a FusionCompute cluster.
type Cluster struct {
	Urn              string `json:"urn"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Tag              string `json:"tag"`
	IsEnableHA       any    `json:"isEnableHa"`
	IsEnableDRS      any    `json:"isEnableDrs"`
	IsMemOvercommit  any    `json:"isMemOvercommit"`
	IsAutoAdjustNuma any    `json:"isAutoAdjustNuma"`
	ResStrategy      string `json:"resStrategy"`
	DRSLevel         any    `json:"drsSetting.drsLevel,omitempty"`
	CPUReservation   any    `json:"haResSetting.cpuReservation,omitempty"`
	MemoryReservation any   `json:"haResSetting.memoryReservation,omitempty"`
}

// Host represents a single physical host.
type Host struct {
	Urn              string `json:"urn"`
	URI              string `json:"uri"`
	Name             string `json:"name"`
	IP               string `json:"ip"`
	Status           string `json:"status"`
	CpuModel         string `json:"cpuModel"`
	CpuType          string `json:"cpuType"`
	CpuQuantity      any    `json:"cpuQuantity"`
	CpuCores         any    `json:"cpuCores"`
	CpuMHz           any    `json:"cpuMHz"`
	CpuFrequency     any    `json:"cpuFrequency"`
	MemoryQuantityMB any    `json:"memoryQuantityMB"`
	MemoryCapacity   any    `json:"memoryCapacity"`
	MemoryUsedMB     any    `json:"memoryUsedMB"`
	RunningVMCount   any    `json:"runningVmCount"`
	ClusterUrn       string `json:"clusterUrn"`
	BMCIp            string `json:"bmcIp"`
	IsMaintaining    any    `json:"isMaintaining"`
	Hypervisor       string `json:"hypervisor"`
}

// VM is the primary aggregate. VmConfig is populated from
// GET {vm_uri} (vm_detail) and is the only source of CPU/memory/NIC/disk
// detail used by the collector.
type VM struct {
	Urn         string    `json:"urn"`
	URI         string    `json:"uri"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Uuid        string    `json:"uuid"`
	LocationUrn string    `json:"locationUrn"`
	HostUrn     string    `json:"hostUrn"`
	ClusterUrn  string    `json:"clusterUrn"`
	Description string    `json:"description"`
	CreateTime  string    `json:"createTime"`
	ToolsStatus string    `json:"toolsVersion"`
	VmConfig    *VMConfig `json:"vmConfig"`
}

// VMConfig is the inner block of the VM detail payload.
type VMConfig struct {
	Cpu     *VMCpu    `json:"cpu"`
	Memory  *VMMemory `json:"memory"`
	Os      *VMOs     `json:"osOptions"`
	Nics    []VMNic   `json:"nics"`
	Disks   []VMDisk  `json:"disks"`
	Volumes []VMDisk  `json:"volumes"`
}

// VMCpu holds the CPU allocation block.
type VMCpu struct {
	Quantity       any `json:"quantity"`
	CoresPerSocket any `json:"coresPerSocket"`
	Reservation    any `json:"reservation"`
	Limit          any `json:"limit"`
	Weight         any `json:"weight"`
	CPUHotPlug     any `json:"cpuHotPlug"`
	CPUBindType    any `json:"cpuBindType"`
	CPUPolicy      any `json:"cpuPolicy"`
}

// VMMemory holds the memory allocation block.
type VMMemory struct {
	QuantityMB  any `json:"quantityMB"`
	Reservation any `json:"reservation"`
	Limit       any `json:"limit"`
	Weight      any `json:"weight"`
	MemHotPlug  any `json:"memHotPlug"`
	HugePage    any `json:"hugePage"`
}

// VMOs holds OS metadata.
type VMOs struct {
	OsType string `json:"osType"`
}

// VMNic is a single virtual NIC. The collector reads Nics from
// VMConfig.Nics, falling back to the field map's NICFields if absent.
type VMNic struct {
	Name            string `json:"name"`
	MAC             string `json:"mac"`
	IP              string `json:"ip"`
	IPList          any    `json:"ipList"`
	IPv6s           any    `json:"ipv6s"`
	PortGroupName   string `json:"portGroupName"`
	PortGroupUrn    string `json:"portGroupUrn"`
	PortGroupType   string `json:"portGroupType"`
	PortGroupVlanRange any `json:"portGroupVlanRange"`
	SequenceNum     any    `json:"sequenceNum"`
	VirtIO          any    `json:"virtIo"`
	NICType         string `json:"nicType"`
	VirtualNICType  string `json:"virtualNicType"`
	ConnectAtPowerOn any   `json:"connectAtPowerOn"`
	Urn             string `json:"urn"`
}

// VMDisk represents one virtual disk / volume. May be sourced from
// VMConfig.Disks, VMConfig.Volumes, or a separate volumes endpoint
// depending on FC version.
type VMDisk struct {
	VolumeUrn      string `json:"volumeUrn"`
	VolumeUuid     string `json:"volumeUuid"`
	Name           string `json:"name"`
	QuantityGB     any    `json:"quantityGB"`
	PCIType        string `json:"pciType"`
	BusType        string `json:"busType"`
	IsThin         any    `json:"isThin"`
	ThinFlag       any    `json:"thinFlag"`
	SequenceNum    any    `json:"sequenceNum"`
	DatastoreUrn   string `json:"datastoreUrn"`
	StorageType    string `json:"storageType"`
	IndepDisk      any    `json:"indepDisk"`
	PersistentDisk any    `json:"persistentDisk"`
}

// Datastore represents a storage datastore.
type Datastore struct {
	Urn                    string `json:"urn"`
	Name                   string `json:"name"`
	StorageType            string `json:"storageType"`
	CapacityGB             any    `json:"capacityGB"`
	FreeSpaceGB            any    `json:"freeSpaceGB"`
	FreeSpace              any    `json:"freeSpace"`
	FreeCapacityGB         any    `json:"freeCapacityGB"`
	FreeSizeGB             any    `json:"freeSizeGB"`
	Status                 string `json:"status"`
	ThinProvisionSupport   any    `json:"thinProvisionSupport"`
	Description            string `json:"description"`
}

// DVSwitch represents a distributed virtual switch.
type DVSwitch struct {
	Urn  string `json:"urn"`
	URI  string `json:"uri"`
	Name string `json:"name"`
	MTU  any    `json:"mtu"`
}

// PortGroup represents a port group on a DVSwitch. DvswitchName is
// populated by the collector from the cross-ref lookup; it is not in
// the upstream payload.
type PortGroup struct {
	Urn          string `json:"urn"`
	URI          string `json:"uri"`
	Name         string `json:"name"`
	VlanID       any    `json:"vlanId"`
	MTU          any    `json:"mtu"`
	DvswitchName string `json:"-"`
}
