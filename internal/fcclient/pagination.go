package fcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// FetchAll walks an offset/limit-paginated endpoint, returning all
// accumulated JSON objects. Each object is returned as a raw json.RawMessage
// so callers can decode into the appropriate typed struct.
//
// Mirrors fc_client._get_all (fc_client.py:209-230).
func (c *Client) FetchAll(ctx context.Context, path, resultKey string) ([]json.RawMessage, error) {
	pageSize := c.cfg.Collection.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	var all []json.RawMessage
	offset := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, total, err := c.fetchPage(ctx, path, resultKey, offset, pageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		// Termination conditions:
		//   1. The page came back smaller than the requested limit
		//      (server ran out of rows).
		//   2. We have already reached the reported total.
		//   3. The page was empty (defensive: avoid infinite loop on
		//      empty sites).
		if len(page) < pageSize || (total > 0 && len(all) >= total) {
			return all, nil
		}
		if len(page) == 0 {
			return all, nil
		}
		offset += pageSize
	}
}

// fetchPage issues a single GET with the given offset/limit and unpacks
// the response. The response envelope is allowed to be {key|items|result}
// or a top-level JSON array (matches fc_client._get_all's fallback path).
func (c *Client) fetchPage(ctx context.Context, path, resultKey string, offset, limit int) ([]json.RawMessage, int, error) {
	// path is a suffix like "/sites" — baseURL ends in "/service".
	endpoint := c.baseURL + path
	q := endpoint + "?offset=" + strconv.Itoa(offset) + "&limit=" + strconv.Itoa(limit)

	req, err := httpNewGETWithAuth(ctx, q, c.token, c.version)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
		return nil, 0, fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, truncForLog(string(body)))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	batch, total := extractBatch(raw, resultKey)
	return batch, total, nil
}

// extractBatch inspects a JSON body and returns the array of items plus
// the server-reported total (if any). Tries the configured resultKey
// first, then falls back to common alternatives, then to a top-level array.
func extractBatch(body []byte, resultKey string) ([]json.RawMessage, int) {
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(body, &wrapper); err == nil {
		keys := []string{resultKey}
		// Append common fallbacks only if not already present.
		for _, k := range []string{"items", "result"} {
			dup := false
			for _, x := range keys {
				if x == k {
					dup = true
					break
				}
			}
			if !dup {
				keys = append(keys, k)
			}
		}
		for _, k := range keys {
			raw, ok := wrapper[k]
			if !ok {
				continue
			}
			var arr []json.RawMessage
			if err := json.Unmarshal(raw, &arr); err != nil {
				continue
			}
			total := len(arr)
			if tr, ok := wrapper["total"]; ok {
				var t int
				if err := json.Unmarshal(tr, &t); err == nil {
					total = t
				}
			}
			return arr, total
		}
		// Wrapper exists but no recognised key: return the whole body
		// as a single batch (caller can still decode it).
		return []json.RawMessage{body}, 1
	}
	// Top-level array.
	var arr []json.RawMessage
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr, len(arr)
	}
	return nil, 0
}
