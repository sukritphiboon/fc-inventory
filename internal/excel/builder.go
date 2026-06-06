// Package excel writes the per-sheet data to a multi-sheet .xlsx file.
// This is a 1:1 port of excel_builder.py:11-97:
//   - SHEET_ORDER (RVTools convention)
//   - header style: bold white on #2C3E50, center-aligned, size 11
//   - autofilter on the data range
//   - freeze row 1
//   - column autosize: sample first 100 rows, cap at 50, floor 10
package excel

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/xuri/excelize/v2"

	"github.com/sukritphiboon/fc-inventory/internal/collector"
)

// SheetOrder mirrors SHEET_ORDER in excel_builder.py:17-20.
var SheetOrder = []string{
	"vSummary", "vInfo", "vCPU", "vMemory", "vDisk", "vNetwork",
	"vHost", "vCluster", "vDatastore", "vSwitch",
}

const (
	headerFontColor = "#FFFFFF"
	headerFillColor = "#2C3E50"
	headerFontSize  = 11
	colSampleRows   = 100
	colMaxWidth     = 50
	colMinWidth     = 10
	colPadding      = 3
)

// Build writes data to the .xlsx at outputPath. Returns an error if
// the file cannot be created or any sheet fails to write.
func Build(data collector.Sheets, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	f := excelize.NewFile()
	defer f.Close()

	styleID, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:   true,
			Color:  headerFontColor,
			Size:   float64(headerFontSize),
			Family: "Calibri",
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Pattern: 1,
			Color:   []string{headerFillColor},
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})
	if err != nil {
		return fmt.Errorf("create header style: %w", err)
	}

	// excelize creates a default "Sheet1"; remove it so SHEET_ORDER
	// controls the workbook structure end-to-end.
	if err := f.DeleteSheet(f.GetSheetName(0)); err != nil {
		slog.Warn("could not remove default sheet", "err", err)
	}

	for _, name := range SheetOrder {
		rows := data[name]
		sheetName := name
		if _, err := f.NewSheet(sheetName); err != nil {
			return fmt.Errorf("create sheet %s: %w", sheetName, err)
		}

		if len(rows) == 0 {
			if err := f.SetCellValue(sheetName, "A1", "No data collected"); err != nil {
				return err
			}
			continue
		}

		// Build the header list: union of all keys, preserve first-seen order.
		headers := unionHeaders(rows)

		// Write header row.
		for col, h := range headers {
			cellRef, _ := excelize.CoordinatesToCellName(col+1, 1)
			if err := f.SetCellValue(sheetName, cellRef, h); err != nil {
				return err
			}
		}

		// Write data rows.
		for rowIdx, row := range rows {
			for col, h := range headers {
				cellRef, _ := excelize.CoordinatesToCellName(col+1, rowIdx+2)
				v, ok := row[h]
				if !ok || v == nil {
					v = ""
				}
				if err := f.SetCellValue(sheetName, cellRef, v); err != nil {
					return err
				}
			}
		}

		// Apply header style.
		lastCol, _ := excelize.ColumnNumberToName(len(headers))
		if err := f.SetCellStyle(sheetName, "A1", lastCol+"1", styleID); err != nil {
			return err
		}

		// Autofilter on the data range.
		dataRange := fmt.Sprintf("A1:%s%d", lastCol, len(rows)+1)
		if err := f.AutoFilter(sheetName, dataRange, []excelize.AutoFilterOptions{}); err != nil {
			slog.Warn("autofilter failed", "sheet", sheetName, "err", err)
		}

		// Freeze top row.
		if err := f.SetPanes(sheetName, &excelize.Panes{
			Freeze:      true,
			YSplit:      1,
			TopLeftCell: "A2",
			ActivePane:  "bottomLeft",
		}); err != nil {
			slog.Warn("freeze pane failed", "sheet", sheetName, "err", err)
		}

		// Column autosize (sample first 100 rows, cap 50, floor 10).
		autoSizeColumns(f, sheetName, headers, rows)
	}

	// Save.
	if err := f.SaveAs(outputPath); err != nil {
		return fmt.Errorf("save xlsx: %w", err)
	}
	return nil
}

// unionHeaders returns the sorted list of column names. Sorting (rather
// than first-seen order) gives deterministic output across runs because
// Go's map iteration is randomised. The Python original used first-seen
// order from the dict literal; the result is a superset of the
// same columns regardless of the ordering.
func unionHeaders(rows []map[string]any) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		for k := range row {
			seen[k] = true
		}
	}
	headers := make([]string, 0, len(seen))
	for k := range seen {
		headers = append(headers, k)
	}
	sort.Strings(headers)
	return headers
}

// autoSizeColumns computes a width per column by sampling up to
// colSampleRows data rows, then clamps to [colMinWidth, colMaxWidth]
// with colPadding characters of slack. Mirrors _auto_size_columns in
// excel_builder.py:82-97.
func autoSizeColumns(f *excelize.File, sheet string, headers []string, rows []map[string]any) {
	sampleCount := len(rows)
	if sampleCount > colSampleRows {
		sampleCount = colSampleRows
	}

	for colIdx, h := range headers {
		maxLen := len(h)
		for i := 0; i < sampleCount; i++ {
			v, ok := rows[i][h]
			if !ok || v == nil {
				continue
			}
			l := len(fmt.Sprint(v))
			if l > maxLen {
				maxLen = l
			}
		}
		width := maxLen + colPadding
		if width > colMaxWidth {
			width = colMaxWidth
		}
		if width < colMinWidth {
			width = colMinWidth
		}
		colName, _ := excelize.ColumnNumberToName(colIdx + 1)
		if err := f.SetColWidth(sheet, colName, colName, float64(width)); err != nil {
			slog.Warn("set column width failed", "sheet", sheet, "col", colName, "err", err)
		}
	}
}
