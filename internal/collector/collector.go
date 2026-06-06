// Package collector orchestrates the inventory pipeline: it drives the
// fcclient to fetch all resources, builds cross-reference maps, and
// flattens the data into per-sheet rows using the fieldmap package.
//
// Cancellation is achieved through the context passed to CollectAll. A
// SIGINT/SIGTERM cancels that context; the collector checks ctx.Err()
// between every resource fetch (mirrors the Python `_check_cancelled`).
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kimzhong/fc-inventory/internal/config"
	"github.com/kimzhong/fc-inventory/internal/fcclient"
)

// Sheets is the per-sheet data the collector hands to the Excel builder.
// Keys are sheet names; values are lists of row maps.
type Sheets map[string][]map[string]any

// ProgressFunc receives progress updates (percent, step). A nil func is
// a no-op (matches the original Python logger-only behaviour when no
// caller subscribes).
type ProgressFunc func(percent int, step string)

// Collector drives a single run. It is not safe for concurrent use.
type Collector struct {
	client *fcclient.Client
	cfg    *config.Config
	log    *slog.Logger
	progress ProgressFunc

	// Raw data fetched from FC.
	sites        []fcclient.Site
	clusters     []fcclient.Cluster
	hosts        []fcclient.Host
	hostDetails  map[string]json.RawMessage
	vms          []fcclient.VM
	vmDetails    map[string]json.RawMessage
	vmNics       map[string][]fcclient.VMNic
	vmDisks      map[string][]fcclient.VMDisk
	datastores   []fcclient.Datastore
	dvswitches   []fcclient.DVSwitch
	portgroups   []fcclient.PortGroup

	// Lookup maps (URN -> name).
	hostMap      map[string]string
	clusterMap   map[string]string
	datastoreMap map[string]string
}

// New constructs a Collector wired to the given client and config.
func New(client *fcclient.Client, cfg *config.Config, progress ProgressFunc) *Collector {
	if progress == nil {
		progress = func(int, string) {}
	}
	return &Collector{
		client:      client,
		cfg:         cfg,
		log:         slog.With("component", "collector"),
		progress:    progress,
		hostDetails: map[string]json.RawMessage{},
		vmDetails:   map[string]json.RawMessage{},
		vmNics:      map[string][]fcclient.VMNic{},
		vmDisks:     map[string][]fcclient.VMDisk{},
		hostMap:     map[string]string{},
		clusterMap:  map[string]string{},
		datastoreMap: map[string]string{},
	}
}

// CollectAll runs the full pipeline. Returns the per-sheet data, or an
// error if login, fetch, or context cancellation fails.
//
// Mirrors the structure of InventoryCollector.collect_all in
// collector.py:288-450 with the cancellation flag replaced by ctx.Err().
func (c *Collector) CollectAll(ctx context.Context) (Sheets, error) {
	// 1. Login
	c.step(5, "Logging in to FusionCompute...")
	if err := c.client.Login(ctx); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	// Best-effort logout in every error path; success path logs out at the end.
	committed := false
	defer func() {
		if !committed {
			_ = c.client.Logout(context.Background())
		}
	}()

	// 2. Sites
	c.step(10, "Fetching sites...")
	sites, err := c.client.GetSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("get sites: %w", err)
	}
	c.sites = sites
	if len(sites) > 0 {
		c.logSample("SITE", sites[0])
	}

	for _, site := range sites {
		siteURI := site.URI
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// 3. Clusters
		c.step(15, "Fetching clusters...")
		clusters, err := c.client.GetClusters(ctx, siteURI)
		if err != nil {
			c.log.Warn("get clusters failed", "site", siteURI, "err", err)
		} else {
			c.clusters = append(c.clusters, clusters...)
			if len(clusters) > 0 {
				c.logSample("CLUSTER", clusters[0])
			}
		}

		// 4. Hosts
		c.step(20, "Fetching hosts...")
		hosts, err := c.client.GetHosts(ctx, siteURI)
		if err != nil {
			c.log.Warn("get hosts failed", "site", siteURI, "err", err)
		} else {
			c.hosts = append(c.hosts, hosts...)
			if len(hosts) > 0 {
				c.logSample("HOST list", hosts[0])
			}
		}

		// 4b. Host details
		for i, host := range c.hosts {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			pct := 20 + percent(i+1, len(c.hosts), 10)
			c.step(pct, fmt.Sprintf("Fetching host detail: %s...", host.Name))
			raw, err := c.client.GetHostDetail(ctx, host.URI)
			if err != nil {
				c.log.Warn("host detail failed", "host", host.Name, "err", err)
				continue
			}
			c.hostDetails[host.Urn] = raw
			if i == 0 {
				c.logSampleRaw("HOST detail", raw)
			}
		}

		// 5. Datastores
		c.step(35, "Fetching datastores...")
		datastores, err := c.client.GetDatastores(ctx, siteURI)
		if err != nil {
			c.log.Warn("get datastores failed", "site", siteURI, "err", err)
		} else {
			c.datastores = append(c.datastores, datastores...)
			if len(datastores) > 0 {
				c.logSample("DATASTORE", datastores[0])
			}
		}

		// 6. Networks
		c.step(40, "Fetching networks...")
		dvswitches, err := c.client.GetDVSwitches(ctx, siteURI)
		if err != nil {
			c.log.Warn("get dvswitches failed", "site", siteURI, "err", err)
		} else {
			c.dvswitches = append(c.dvswitches, dvswitches...)
			if len(dvswitches) > 0 {
				c.logSample("DVSWITCH", dvswitches[0])
			}
		}

		pgLogged := false
		for _, dvs := range dvswitches {
			pgs, err := c.client.GetPortgroups(ctx, dvs.URI)
			if err != nil {
				c.log.Warn("get portgroups failed", "dvs", dvs.Name, "err", err)
				continue
			}
			for i := range pgs {
				pgs[i].DvswitchName = dvs.Name
			}
			c.portgroups = append(c.portgroups, pgs...)
			if len(pgs) > 0 && !pgLogged {
				c.logSample("PORTGROUP", pgs[0])
				pgLogged = true
			}
		}
		// Site-level fallback (mirrors collector.py:367-377).
		if len(c.portgroups) == 0 {
			sitePgs, err := c.client.GetSitePortgroups(ctx, siteURI)
			if err != nil {
				c.log.Warn("site portgroup fallback failed", "err", err)
			} else {
				for i := range sitePgs {
					if sitePgs[i].DvswitchName == "" {
						sitePgs[i].DvswitchName = ""
					}
				}
				c.portgroups = append(c.portgroups, sitePgs...)
				if len(sitePgs) > 0 && !pgLogged {
					c.logSample("PORTGROUP (site-level)", sitePgs[0])
				}
			}
		}

		// 7. VMs
		c.step(45, "Fetching VM list...")
		vms, err := c.client.GetVMs(ctx, siteURI)
		if err != nil {
			c.log.Warn("get vms failed", "site", siteURI, "err", err)
		} else {
			c.vms = append(c.vms, vms...)
			if len(vms) > 0 {
				c.logSample("VM list", vms[0])
			}
		}
	}

	// 8. VM details (after all VMs from all sites are gathered).
	totalVMs := len(c.vms)
	for i, vm := range c.vms {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pct := 50 + percent(i+1, totalVMs, 40)
		c.step(pct, fmt.Sprintf("Fetching VM detail (%d/%d): %s...", i+1, totalVMs, vm.Name))
		detail, err := c.client.GetVMDetail(ctx, vm.URI)
		if err != nil {
			c.log.Warn("vm detail failed", "vm", vm.Name, "err", err)
			continue
		}
		c.vmDetails[vm.Urn] = detail

		// Decode the detail for NIC/disk extraction.
		var detailMap map[string]any
		if jerr := json.Unmarshal(detail, &detailMap); jerr != nil {
			c.log.Warn("vm detail decode failed", "vm", vm.Name, "err", jerr)
			continue
		}
		nics, err := c.client.ExtractVMNics(ctx, vm.URI, detailMap)
		if err != nil {
			c.log.Warn("vm nics failed", "vm", vm.Name, "err", err)
		}
		c.vmNics[vm.Urn] = nics
		disks, err := c.client.ExtractVMDisks(ctx, vm.URI, detailMap)
		if err != nil {
			c.log.Warn("vm disks failed", "vm", vm.Name, "err", err)
		}
		c.vmDisks[vm.Urn] = disks
		if i == 0 {
			c.log.Info("first VM detail extracted", "nics", len(nics), "disks", len(disks))
		}
	}

	// 9. Build lookup maps and the sheet data.
	c.step(92, "Processing collected data...")
	c.buildLookupMaps()
	sheets := c.buildAllSheets()

	// 10. Logout.
	c.step(98, "Logging out...")
	if err := c.client.Logout(ctx); err != nil {
		c.log.Warn("logout failed", "err", err)
	}
	committed = true

	c.step(100, "Collection complete!")
	return sheets, nil
}

// step reports progress to the progress callback.
func (c *Collector) step(percent int, msg string) {
	c.progress(percent, msg)
}

// percent computes pct = base + (i/total)*span, rounded.
func percent(i, total, span int) int {
	if total <= 0 {
		return span
	}
	return (i * span) / total
}

// buildLookupMaps mirrors _build_lookup_maps.
func (c *Collector) buildLookupMaps() {
	for _, h := range c.hosts {
		c.hostMap[h.Urn] = h.Name
	}
	for _, cl := range c.clusters {
		c.clusterMap[cl.Urn] = cl.Name
	}
	for _, d := range c.datastores {
		c.datastoreMap[d.Urn] = d.Name
	}
}

// logSample and logSampleRaw dump a small overview of a sample object to
// the log. Mirrors the collector._log_sample helper used for debugging
// field-mapping mismatches.
func (c *Collector) logSample(label string, obj any) {
	c.log.Info("sample", "label", label, "keys", typeOrKeys(obj))
}

func (c *Collector) logSampleRaw(label string, raw json.RawMessage) {
	var any_ any
	if err := json.Unmarshal(raw, &any_); err == nil {
		c.logSample(label, any_)
	} else {
		c.log.Info("sample (raw bytes)", "label", label, "len", len(raw))
	}
}
