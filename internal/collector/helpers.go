package collector

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// typeOrKeys returns a short description of v: a type name, or for a map
// the list of top-level keys. Used by logSample.
func typeOrKeys(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		return keys
	default:
		return fmt.Sprintf("%T", v)
	}
}

// mergedVM produces a map[string]any with the VM list payload as the
// base and the VM detail overlaid on top. Mirrors `{**vm, **detail}`
// in collector.py:540.
func (c *Collector) mergedVM(urn string) map[string]any {
	merged := map[string]any{}
	for _, vm := range c.vms {
		if vm.Urn == urn {
			b, err := json.Marshal(vm)
			if err == nil {
				var m map[string]any
				if json.Unmarshal(b, &m) == nil {
					for k, v := range m {
						merged[k] = v
					}
				}
			}
			break
		}
	}
	if raw, ok := c.vmDetails[urn]; ok {
		var detail map[string]any
		if err := json.Unmarshal(raw, &detail); err == nil {
			for k, v := range detail {
				merged[k] = v
			}
		}
	}
	return merged
}

// mergedHost mirrors the host builder's {**host, **detail} merge.
func (c *Collector) mergedHost(urn string) map[string]any {
	merged := map[string]any{}
	for _, h := range c.hosts {
		if h.Urn == urn {
			b, err := json.Marshal(h)
			if err == nil {
				var m map[string]any
				if json.Unmarshal(b, &m) == nil {
					for k, v := range m {
						merged[k] = v
					}
				}
			}
			break
		}
	}
	if raw, ok := c.hostDetails[urn]; ok {
		var detail map[string]any
		if err := json.Unmarshal(raw, &detail); err == nil {
			for k, v := range detail {
				merged[k] = v
			}
		}
	}
	return merged
}

// sumDiskGB returns the total provisioned capacity across a disk slice.
// Mirrors the sum() at collector.py:549.
func sumDiskGB(disks []map[string]any) float64 {
	var total float64
	for _, d := range disks {
		v, ok := d["quantityGB"]
		if !ok || v == nil {
			continue
		}
		total += toFloat(v)
	}
	return total
}

// toFloat coerces a JSON number-or-string to float64. Mirrors the
// try/except float(...) pattern used in collector.py:567-571.
func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		var f float64
		_, _ = fmt.Sscan(x, &f)
		return f
	default:
		// reflect-based fallback for arbitrary numeric kinds.
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return float64(rv.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return float64(rv.Uint())
		case reflect.Float32, reflect.Float64:
			return rv.Float()
		}
		return 0
	}
}

// toInt coerces a JSON number-or-string to int. Used for socket counting
// (collector.py:580-583).
func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		var i int
		_, _ = fmt.Sscan(x, &i)
		return i
	default:
		return 0
	}
}
