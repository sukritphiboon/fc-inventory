package fieldmap

import (
	"fmt"
	"strconv"
	"strings"
)

// GetPath walks a dot-separated path in a nested map[string]any and
// returns the value at the leaf (which may be a primitive or another
// map/slice). Returns nil if any segment is missing.
//
// Mirrors collector._get_path (collector.py:21-34).
func GetPath(d any, path string) any {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	cur := d
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		v, ok := m[p]
		if !ok {
			return nil
		}
		if v == nil {
			return nil
		}
		cur = v
	}
	return cur
}

// TryPaths returns the first non-nil, non-empty value found by walking
// the given paths in order. Empty string is treated as "missing" so
// fields like "description": "" don't get rendered as the literal value.
//
// Mirrors collector._try_paths (collector.py:37-43).
func TryPaths(d any, paths []string) any {
	for _, p := range paths {
		v := GetPath(d, p)
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		return v
	}
	return ""
}

// TryString is a convenience wrapper that stringifies the first match.
// Numbers and booleans are rendered with their natural Go formatting
// (matching the Python `str(v)` behaviour).
func TryString(d any, paths []string) string {
	v := TryPaths(d, paths)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// FlattenDict flattens a nested dict to a single-level map with dot
// keys. Lists-of-dicts are skipped (handled as sub-tables); primitive
// lists are joined with ", " (mirrors _flatten_dict, collector.py:46-62).
func FlattenDict(d any) map[string]any {
	out := map[string]any{}
	flattenInto(out, "", d)
	return out
}

func flattenInto(out map[string]any, prefix string, v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenInto(out, key, val)
		}
	case []any:
		if len(x) > 0 {
			if _, isDict := x[0].(map[string]any); isDict {
				return // skip sub-tables
			}
		}
		if prefix == "" {
			return
		}
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, fmt.Sprint(e))
		}
		out[prefix] = strings.Join(parts, ", ")
	default:
		if prefix != "" {
			out[prefix] = v
		}
	}
}

// BuildRow produces a map from FieldMap columns to the values resolved
// from data. If includeExtras is true, additional raw keys (not already
// consumed by the field map) are appended with prettified names so the
// .xlsx output remains a superset of the v1.0.0 Python output.
//
// Mirrors collector._build_row (collector.py:187-217).
func BuildRow(data any, fm FieldMap, includeExtras bool) map[string]any {
	row := make(map[string]any, len(fm)+8)
	consumed := map[string]struct{}{}

	for col, paths := range fm {
		// Track which dotted paths we already looked at so we can skip
		// the same key when the extras pass runs.
		for _, p := range paths {
			if v := GetPath(data, p); v != nil {
				if s, ok := v.(string); ok && s == "" {
					continue
				}
				row[col] = v
				consumed[p] = struct{}{}
				break
			}
		}
		if _, ok := row[col]; !ok {
			row[col] = ""
		}
	}

	if includeExtras {
		flat := FlattenDict(data)
		seenPretty := map[string]struct{}{}
		for k, v := range flat {
			if _, used := consumed[k]; used {
				continue
			}
			pretty := PrettifyKey(k)
			if _, exists := row[pretty]; exists {
				continue
			}
			if _, exists := seenPretty[pretty]; exists {
				continue
			}
			seenPretty[pretty] = struct{}{}
			row[pretty] = v
		}
	}
	return row
}

// PrettifyKey turns "vmConfig.cpu.quantity" into "VmConfig Cpu Quantity"
// for display in the spreadsheet's extra-columns section. Mirrors the
// collector.py `prettify_key` helper (collector.py:223-228).
func PrettifyKey(dotted string) string {
	parts := strings.Split(dotted, ".")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		// Title-case first letter; leave the rest as-is.
		out = append(out, strings.ToUpper(p[:1])+p[1:])
	}
	return strings.Join(out, " ")
}

// FormatNumber returns a stringified number with no trailing zeros, or
// "" if the input is empty/nil. Used by the vDatastore sheet builder
// for capacity/free columns.
func FormatNumber(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(v)
	}
}

// PowerState converts an FC status string to ON/OFF. Mirrors the
// collector._power_state helper (collector.py:65-71).
func PowerState(status any) string {
	s, _ := status.(string)
	switch s {
	case "running", "started", "Running", "RUNNING":
		return "ON"
	case "stopped", "shutOff", "Stopped", "STOPPED":
		return "OFF"
	}
	return s
}
