package collector

// buildAllSheets assembles the per-sheet data dict.
// Mirrors _build_all_sheets in collector.py:466-478.
func (c *Collector) buildAllSheets() Sheets {
	vinfo := c.buildVInfo()
	return Sheets{
		"vSummary":  c.buildVSummary(vinfo),
		"vInfo":     vinfo,
		"vCPU":      c.buildVCPU(),
		"vMemory":   c.buildVMemory(),
		"vDisk":     c.buildVDisk(),
		"vNetwork":  c.buildVNetwork(),
		"vHost":     c.buildVHost(),
		"vCluster":  c.buildVCluster(),
		"vDatastore": c.buildVDatastore(),
		"vSwitch":   c.buildVSwitch(),
	}
}
