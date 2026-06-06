package fcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// GetVMDetail fetches the full VM detail payload. Mirrors
// fc_client.get_vm_detail.
func (c *Client) GetVMDetail(ctx context.Context, vmURI string) (json.RawMessage, error) {
	return c.getRaw(ctx, vmURI)
}

// getRaw issues an authenticated GET and returns the raw response body.
// It is used by GetVMDetail and GetHostDetail, both of which need the
// untyped JSON so the collector can apply the hybrid field map.
//
// path is the resource URI as it appears in the upstream response. The
// FC API uses full absolute paths like "/service/vms/1" — same shape
// as the baseURL. To avoid "/service/service/vms/1", we replace the
// baseURL's "/service" suffix with the incoming path's leading
// "/service" rather than concatenating the two strings.
func (c *Client) getRaw(ctx context.Context, path string) (json.RawMessage, error) {
	if path == "" {
		return nil, fmt.Errorf("empty resource URI")
	}
	endpoint := path
	if !startsWithScheme(path) {
		// Both c.baseURL and path share a "/service" prefix segment
		// when the path is the canonical FC URI. Stitch them together
		// by replacing the baseURL's /service with the path's /service.
		// For URNs or other opaque identifiers, fall back to
		// baseURL + path.
		if strings.HasPrefix(path, "/service/") && strings.HasSuffix(c.baseURL, "/service") {
			endpoint = strings.TrimSuffix(c.baseURL, "/service") + path
		} else {
			endpoint = c.baseURL + path
		}
	}
	req, err := httpNewGETWithAuth(ctx, endpoint, c.token, c.version)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, truncForLog(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func startsWithScheme(s string) bool {
	return len(s) >= 7 && (s[:7] == "http://" || (len(s) >= 8 && s[:8] == "https://"))
}

// ExtractVMNics returns the NIC list from a VM detail payload, falling
// back from vmConfig.nics to the per-VM /nics endpoint if absent.
// Mirrors fc_client.get_vm_nics (fc_client.py:265-269).
func (c *Client) ExtractVMNics(ctx context.Context, vmURI string, detail map[string]any) ([]VMNic, error) {
	if cfg, ok := detail["vmConfig"].(map[string]any); ok {
		if arr, ok := cfg["nics"].([]any); ok {
			nics, err := decodeAnySlice[VMNic](arr)
			if err != nil {
				return nil, err
			}
			if len(nics) > 0 {
				return nics, nil
			}
		}
	}
	raw, err := c.getRaw(ctx, vmURI+"/nics")
	if err != nil {
		return nil, err
	}
	var nics []VMNic
	if err := json.Unmarshal(raw, &nics); err != nil {
		return nil, err
	}
	return nics, nil
}

// ExtractVMDisks returns the disk list from a VM detail payload, falling
// back through vmConfig.disks -> vmConfig.volumes -> /volumes -> /disks
// until a non-empty list is found. Mirrors fc_client.get_vm_disks
// (fc_client.py:270-281) and the inline read at collector.py:403-407.
func (c *Client) ExtractVMDisks(ctx context.Context, vmURI string, detail map[string]any) ([]VMDisk, error) {
	if cfg, ok := detail["vmConfig"].(map[string]any); ok {
		for _, key := range []string{"disks", "volumes"} {
			if arr, ok := cfg[key].([]any); ok {
				disks, err := decodeAnySlice[VMDisk](arr)
				if err != nil {
					return nil, err
				}
				if len(disks) > 0 {
					return disks, nil
				}
			}
		}
	}
	for _, sub := range []string{"/volumes", "/disks"} {
		raw, err := c.getRaw(ctx, vmURI+sub)
		if err != nil {
			return nil, err
		}
		var disks []VMDisk
		if err := json.Unmarshal(raw, &disks); err != nil {
			return nil, err
		}
		if len(disks) > 0 {
			return disks, nil
		}
	}
	return nil, nil
}

func decodeAnySlice[T any](arr []any) ([]T, error) {
	out := make([]T, 0, len(arr))
	for _, e := range arr {
		b, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
