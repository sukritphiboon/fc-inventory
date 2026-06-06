package fcclient

import (
	"context"
	"encoding/json"
)

// GetSites lists all VRM sites. Mirrors fc_client.get_sites.
func (c *Client) GetSites(ctx context.Context) ([]Site, error) {
	raw, err := c.FetchAll(ctx, "/sites", "sites")
	if err != nil {
		return nil, err
	}
	return decodeAll[Site](raw)
}

// GetClusters lists all clusters. Mirrors fc_client.get_clusters.
func (c *Client) GetClusters(ctx context.Context, siteURI string) ([]Cluster, error) {
	raw, err := c.FetchAll(ctx, siteURI+"/clusters", "clusters")
	if err != nil {
		return nil, err
	}
	return decodeAll[Cluster](raw)
}

// GetHosts lists all hosts. Mirrors fc_client.get_hosts.
func (c *Client) GetHosts(ctx context.Context, siteURI string) ([]Host, error) {
	raw, err := c.FetchAll(ctx, siteURI+"/hosts", "hosts")
	if err != nil {
		return nil, err
	}
	return decodeAll[Host](raw)
}

// GetHostDetail fetches one host's full detail. Mirrors fc_client.get_host_detail.
func (c *Client) GetHostDetail(ctx context.Context, hostURI string) (json.RawMessage, error) {
	return c.getRaw(ctx, hostURI)
}

// GetVMs lists all VMs. Mirrors fc_client.get_vms.
func (c *Client) GetVMs(ctx context.Context, siteURI string) ([]VM, error) {
	raw, err := c.FetchAll(ctx, siteURI+"/vms", "vms")
	if err != nil {
		return nil, err
	}
	return decodeAll[VM](raw)
}

// GetDatastores lists all datastores. Mirrors fc_client.get_datastores.
func (c *Client) GetDatastores(ctx context.Context, siteURI string) ([]Datastore, error) {
	raw, err := c.FetchAll(ctx, siteURI+"/datastores", "datastores")
	if err != nil {
		return nil, err
	}
	return decodeAll[Datastore](raw)
}

// GetDVSwitches lists all distributed vSwitches. Mirrors
// fc_client.get_dvswitches.
func (c *Client) GetDVSwitches(ctx context.Context, siteURI string) ([]DVSwitch, error) {
	raw, err := c.FetchAll(ctx, siteURI+"/dvswitchs", "dvswitchs")
	if err != nil {
		return nil, err
	}
	return decodeAll[DVSwitch](raw)
}

// GetPortgroups lists all port groups for a DVSwitch. Mirrors
// fc_client.get_portgroups.
func (c *Client) GetPortgroups(ctx context.Context, dvswitchURI string) ([]PortGroup, error) {
	raw, err := c.FetchAll(ctx, dvswitchURI+"/portgroups", "portgroups")
	if err != nil {
		return nil, err
	}
	return decodeAll[PortGroup](raw)
}

// GetSitePortgroups lists port groups at the site level (the fallback
// path used by collector.py:367-377 when per-DVSwitch enumeration comes
// back empty).
func (c *Client) GetSitePortgroups(ctx context.Context, siteURI string) ([]PortGroup, error) {
	raw, err := c.FetchAll(ctx, siteURI+"/portgroups", "portgroups")
	if err != nil {
		return nil, err
	}
	return decodeAll[PortGroup](raw)
}

// decodeAll unmarshals a slice of json.RawMessage into a slice of T.
func decodeAll[T any](raw []json.RawMessage) ([]T, error) {
	out := make([]T, 0, len(raw))
	for _, r := range raw {
		var v T
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
