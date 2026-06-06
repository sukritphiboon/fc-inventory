package fieldmap

import (
	"reflect"
	"sort"
	"testing"
)

func TestGetPath(t *testing.T) {
	data := map[string]any{
		"vmConfig": map[string]any{
			"cpu": map[string]any{
				"quantity": 4,
			},
		},
		"name": "vm-01",
	}
	cases := []struct {
		path string
		want any
	}{
		{"name", "vm-01"},
		{"vmConfig.cpu.quantity", 4},
		{"vmConfig.cpu.missing", nil},
		{"missing.thing", nil},
		{"", nil},
	}
	for _, c := range cases {
		got := GetPath(data, c.path)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("GetPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestTryPaths(t *testing.T) {
	data := map[string]any{
		"a": "",
		"b": nil,
		"vmConfig": map[string]any{
			"osOptions": map[string]any{
				"osType": "CentOS",
			},
		},
	}
	cases := []struct {
		name  string
		paths []string
		want  any
	}{
		{"first empty, second hit", []string{"a", "b", "vmConfig.osOptions.osType"}, "CentOS"},
		{"all empty", []string{"a", "b"}, ""},
		{"nested hit", []string{"vmConfig.osOptions.osType"}, "CentOS"},
		{"missing", []string{"x.y.z"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := TryPaths(data, c.paths)
			if got != c.want {
				t.Errorf("TryPaths(%v) = %v, want %v", c.paths, got, c.want)
			}
		})
	}
}

func TestTryString(t *testing.T) {
	data := map[string]any{
		"vmConfig": map[string]any{
			"cpu": map[string]any{
				"quantity": 4,
			},
		},
		"name": "vm-01",
	}
	if got := TryString(data, []string{"vmConfig.cpu.quantity"}); got != "4" {
		t.Errorf("TryString numeric = %q, want \"4\"", got)
	}
	if got := TryString(data, []string{"name"}); got != "vm-01" {
		t.Errorf("TryString string = %q, want \"vm-01\"", got)
	}
	if got := TryString(data, []string{"missing"}); got != "" {
		t.Errorf("TryString missing = %q, want \"\"", got)
	}
}

func TestFlattenDict(t *testing.T) {
	data := map[string]any{
		"a": "x",
		"b": map[string]any{
			"c": "y",
			"d": map[string]any{
				"e": 5,
			},
		},
		"f": []any{1, 2, 3},
		"g": []any{map[string]any{"skip": true}},
	}
	got := FlattenDict(data)
	want := map[string]any{
		"a":       "x",
		"b.c":     "y",
		"b.d.e":   5,
		"f":       "1, 2, 3",
		// g is list-of-dicts and should be skipped
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FlattenDict = %v, want %v", got, want)
	}
}

func TestPrettifyKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"name", "Name"},
		{"vmConfig.cpu.quantity", "VmConfig Cpu Quantity"},
		{"", ""},
	}
	for _, c := range cases {
		if got := PrettifyKey(c.in); got != c.want {
			t.Errorf("PrettifyKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildRow_ExactMatch(t *testing.T) {
	data := map[string]any{
		"name": "vm-01",
		"vmConfig": map[string]any{
			"cpu":    map[string]any{"quantity": 4, "coresPerSocket": 2},
			"memory": map[string]any{"quantityMB": 8192},
		},
		"uuid": "abc",
	}
	fm := FieldMap{
		"VM Name":          {"name"},
		"CPUs":             {"vmConfig.cpu.quantity"},
		"Cores Per Socket": {"vmConfig.cpu.coresPerSocket"},
		"Memory (MB)":      {"vmConfig.memory.quantityMB"},
		"UUID":             {"uuid"},
	}
	row := BuildRow(data, fm, false)
	if row["VM Name"] != "vm-01" {
		t.Errorf("VM Name = %v", row["VM Name"])
	}
	if row["CPUs"] != 4 {
		t.Errorf("CPUs = %v", row["CPUs"])
	}
	if row["UUID"] != "abc" {
		t.Errorf("UUID = %v", row["UUID"])
	}
	// All mapped keys present (including empty).
	for k := range fm {
		if _, ok := row[k]; !ok {
			t.Errorf("row missing key %q", k)
		}
	}
}

func TestBuildRow_FallbackPath(t *testing.T) {
	// First path missing, second path hits.
	data := map[string]any{
		"vmConfig": map[string]any{
			"osOptions": map[string]any{"osType": "CentOS"},
		},
	}
	fm := FieldMap{
		"Guest OS": {"osOptions.osType", "vmConfig.osOptions.osType"},
	}
	row := BuildRow(data, fm, false)
	if row["Guest OS"] != "CentOS" {
		t.Errorf("Guest OS fallback = %v, want CentOS", row["Guest OS"])
	}
}

func TestBuildRow_Extras(t *testing.T) {
	data := map[string]any{
		"name": "vm-01",
		"urn":  "urn:vm:1",
		"vmConfig": map[string]any{
			"cpu": map[string]any{
				"quantity": 4,
				"newField": "future-version-tag",
			},
		},
	}
	fm := FieldMap{
		"VM Name": {"name"},
		"CPUs":    {"vmConfig.cpu.quantity"},
	}
	row := BuildRow(data, fm, true)
	if row["VM Name"] != "vm-01" {
		t.Errorf("VM Name = %v", row["VM Name"])
	}
	if row["CPUs"] != 4 {
		t.Errorf("CPUs = %v", row["CPUs"])
	}
	// "urn" prettifies to "Urn" (first letter upper-cased).
	if row["Urn"] != "urn:vm:1" {
		t.Errorf("Urn extra = %v, want urn:vm:1", row["Urn"])
	}
	// "newField" should appear under its prettified name.
	if row["VmConfig Cpu NewField"] != "future-version-tag" {
		t.Errorf("extra prettified key missing: row = %v", keysSorted(row))
	}
}

func TestPowerState(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"running", "running", "ON"},
		{"Running", "Running", "ON"},
		{"started", "started", "ON"},
		{"stopped", "stopped", "OFF"},
		{"shutOff", "shutOff", "OFF"},
		{"Stopped", "Stopped", "OFF"},
		{"unknown", "unknown", "unknown"},
		{"empty", "", ""},
		{"nil", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PowerState(c.in); got != c.want {
				t.Errorf("PowerState(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestFormatNumber(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"", ""},
		{"42", "42"},
		{42, "42"},
		{int64(42), "42"},
		{3.14, "3.14"},
		{true, "true"},
	}
	for _, c := range cases {
		if got := FormatNumber(c.in); got != c.want {
			t.Errorf("FormatNumber(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func keysSorted(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
