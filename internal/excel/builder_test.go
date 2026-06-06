package excel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/kimzhong/fc-inventory/internal/collector"
)

func TestBuild_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.xlsx")

	data := collector.Sheets{
		"vSummary": {
			{"Item": "Total VMs", "Count": 3},
			{"Item": "Power ON", "Count": 2},
		},
		"vInfo": {
			{"VM Name": "vm-1", "Power State": "ON", "CPUs": 4, "Memory (MB)": 8192},
			{"VM Name": "vm-2", "Power State": "OFF", "CPUs": 2, "Memory (MB)": 4096},
		},
		"vCPU": {{"VM Name": "vm-1", "Total CPUs": 4, "Sockets": 2}},
		// vMemory / vDisk / vNetwork / vHost / vCluster / vDatastore / vSwitch
		// intentionally omitted; the builder must still create them
		// (as empty "No data collected" sheets).
	}

	if err := Build(data, out); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Re-open with excelize and check basic structure.
	f, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer f.Close()

	for _, name := range SheetOrder {
		idx, err := f.GetSheetIndex(name)
		if err != nil || idx < 0 {
			t.Errorf("sheet %q missing (idx=%d, err=%v)", name, idx, err)
		}
	}

	// Spot-check vInfo contents.
	rows, err := f.GetRows("vInfo")
	if err != nil {
		t.Fatalf("GetRows vInfo: %v", err)
	}
	if len(rows) < 3 {
		t.Errorf("vInfo rows = %d, want >= 3", len(rows))
	}
	// Find VM Name column dynamically (headers are sorted).
	headerRow := rows[0]
	vmNameCol := -1
	for i, h := range headerRow {
		if h == "VM Name" {
			vmNameCol = i
			break
		}
	}
	if vmNameCol < 0 {
		t.Fatalf("vInfo header missing VM Name: %v", headerRow)
	}
	if rows[1][vmNameCol] != "vm-1" {
		t.Errorf("vInfo row[1][VM Name] = %q, want vm-1", rows[1][vmNameCol])
	}

	// Spot-check vSummary.
	srows, err := f.GetRows("vSummary")
	if err != nil {
		t.Fatalf("GetRows vSummary: %v", err)
	}
	if len(srows) < 2 {
		t.Fatalf("vSummary too few rows: %v", srows)
	}
	// Find Item and Count columns (sorted: Count, Item).
	itemCol, countCol := -1, -1
	for i, h := range srows[0] {
		if h == "Item" {
			itemCol = i
		}
		if h == "Count" {
			countCol = i
		}
	}
	if itemCol < 0 || countCol < 0 {
		t.Fatalf("vSummary missing columns: %v", srows[0])
	}
	// Row 1 is "Total VMs" / "3".
	if srows[1][itemCol] != "Total VMs" || srows[1][countCol] != "3" {
		t.Errorf("vSummary row 1 = %v, want [Total VMs, 3]", srows[1])
	}

	// vDatastore should exist with a single "No data collected" row.
	drows, err := f.GetRows("vDatastore")
	if err != nil {
		t.Fatalf("GetRows vDatastore: %v", err)
	}
	if len(drows) != 1 || drows[0][0] != "No data collected" {
		t.Errorf("vDatastore = %v, want [No data collected]", drows)
	}
}

func TestBuild_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.xlsx")
	// Pre-create the file to ensure Build handles "must be writable" correctly.
	if err := os.WriteFile(out, []byte("placeholder"), 0o600); err != nil {
		t.Fatalf("precreate: %v", err)
	}

	data := collector.Sheets{"vSummary": {{"Item": "X", "Count": "1"}}}
	if err := Build(data, out); err != nil {
		t.Fatalf("Build: %v", err)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Size() < 1000 {
		t.Errorf("xlsx too small: %d bytes", st.Size())
	}
}

func TestUnionHeaders_Sorted(t *testing.T) {
	rows := []map[string]any{
		{"B": 1, "A": 2},
		{"C": 3, "A": 4},
		{"A": 5},
	}
	got := unionHeaders(rows)
	want := []string{"A", "B", "C"}
	if len(got) != len(want) {
		t.Fatalf("unionHeaders = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unionHeaders[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
