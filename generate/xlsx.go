package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// sheetMeta maps a tabular ArtifactType to its workbook sheet name.
var sheetMeta = []struct {
	artifact  ArtifactType
	sheetName string
}{
	{SOA, "SOA"},
	{RiskCSV, "Risk Register"},
	{EvidenceRegistry, "Evidence Registry"},
	{DependencyMap, "Dependency Map"},
	{CollectiveRisk, "Collective Risk"},
}

// AssembleWorkbook builds a styled multi-sheet Excel workbook from the tabular
// results already produced by the other generators. It is called by RunPipeline
// when the Artifacts list includes XLSX.
//
// Workbook structure:
//   - Cover sheet: product name, framework, generation date.
//   - One sheet per tabular artifact with data in result.Data.
//
// Styling mirrors psc-ms xlsx.py:
//   - Header row: #404040 fill, white bold text, center-aligned, frozen below row 1.
//   - Alternating rows: #F2F2F2 fill on even data rows.
//   - PENDING rows: #FCEAEA fill when any cell starts with "PENDING" or "NO LINKS".
//   - Columns auto-sized from content, capped at 80 characters wide.
func AssembleWorkbook(results []ArtifactResult, ps *ProgramState, cfg PipelineConfig) ArtifactResult {
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck

	// Build cover sheet on the default "Sheet1".
	if err := buildCoverSheet(f, ps, cfg); err != nil {
		return ArtifactResult{Artifact: XLSX, Error: fmt.Errorf("cover sheet: %w", err)}
	}
	// Rename Sheet1 to "Cover" after populating it (Sheet1 exists by default).
	if err := f.SetSheetName("Sheet1", "Cover"); err != nil {
		return ArtifactResult{Artifact: XLSX, Error: fmt.Errorf("renaming cover sheet: %w", err)}
	}

	totalRows := 0
	// Build a sheet for each tabular artifact that has data.
	for _, meta := range sheetMeta {
		var data [][]string
		for _, r := range results {
			if r.Artifact == meta.artifact && len(r.Data) > 0 {
				data = r.Data
				break
			}
		}
		if len(data) == 0 {
			continue
		}
		if _, err := f.NewSheet(meta.sheetName); err != nil {
			return ArtifactResult{Artifact: XLSX, Error: fmt.Errorf("creating sheet %q: %w", meta.sheetName, err)}
		}
		n, err := writeDataSheet(f, meta.sheetName, data)
		if err != nil {
			return ArtifactResult{Artifact: XLSX, Error: fmt.Errorf("writing sheet %q: %w", meta.sheetName, err)}
		}
		totalRows += n
	}

	path := filepath.Join(cfg.OutputDir, "compliance.xlsx")
	if cfg.DryRun {
		return ArtifactResult{Artifact: XLSX, OutputPath: path, Rows: totalRows, DryRun: true}
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return ArtifactResult{Artifact: XLSX, Error: fmt.Errorf("creating output dir: %w", err)}
	}
	if err := f.SaveAs(path); err != nil {
		return ArtifactResult{Artifact: XLSX, Error: fmt.Errorf("saving workbook: %w", err)}
	}
	return ArtifactResult{Artifact: XLSX, OutputPath: path, Rows: totalRows}
}

func buildCoverSheet(f *excelize.File, ps *ProgramState, cfg PipelineConfig) error {
	sheet := "Sheet1"

	titleStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
	})
	if err != nil {
		return err
	}
	labelStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		return err
	}

	program := ps.Program
	if program == "" {
		program = cfg.Program
	}
	framework := ps.Framework
	if framework == "" {
		framework = cfg.Framework
	}
	product := ps.ProductName

	f.SetCellValue(sheet, "A1", "Compliance Artifact Package") //nolint:errcheck
	f.SetCellStyle(sheet, "A1", "A1", titleStyle)              //nolint:errcheck

	rows := [][]string{
		{"Program", program},
		{"Framework", framework},
		{"Product", product},
		{"Generated", time.Now().Format("2006-01-02")},
	}
	for i, row := range rows {
		rowNum := i + 3
		cellA := fmt.Sprintf("A%d", rowNum)
		cellB := fmt.Sprintf("B%d", rowNum)
		f.SetCellValue(sheet, cellA, row[0]) //nolint:errcheck
		f.SetCellValue(sheet, cellB, row[1]) //nolint:errcheck
		f.SetCellStyle(sheet, cellA, cellA, labelStyle) //nolint:errcheck
	}
	f.SetColWidth(sheet, "A", "A", 16) //nolint:errcheck
	f.SetColWidth(sheet, "B", "B", 40) //nolint:errcheck
	return nil
}

func writeDataSheet(f *excelize.File, sheet string, data [][]string) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"404040"}},
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
			WrapText:   true,
		},
	})
	if err != nil {
		return 0, err
	}
	altStyle, err := f.NewStyle(&excelize.Style{
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"F2F2F2"}},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "top", WrapText: true},
	})
	if err != nil {
		return 0, err
	}
	defaultStyle, err := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "top", WrapText: true},
	})
	if err != nil {
		return 0, err
	}
	pendingStyle, err := f.NewStyle(&excelize.Style{
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FCEAEA"}},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "top", WrapText: true},
	})
	if err != nil {
		return 0, err
	}

	// Track max content width per column for auto-sizing.
	colWidths := make([]int, len(data[0]))

	for rowIdx, row := range data {
		xlRow := rowIdx + 1
		for colIdx, cell := range row {
			colName, _ := excelize.ColumnNumberToName(colIdx + 1)
			cellRef := fmt.Sprintf("%s%d", colName, xlRow)
			f.SetCellValue(sheet, cellRef, cell) //nolint:errcheck
			if l := len(cell); l > colWidths[colIdx] {
				colWidths[colIdx] = l
			}
		}

		colEnd, _ := excelize.ColumnNumberToName(len(row))
		colStart, _ := excelize.ColumnNumberToName(1)
		startRef := fmt.Sprintf("%s%d", colStart, xlRow)
		endRef := fmt.Sprintf("%s%d", colEnd, xlRow)

		if rowIdx == 0 {
			f.SetCellStyle(sheet, startRef, endRef, headerStyle) //nolint:errcheck
		} else {
			style := defaultStyle
			if isPendingRow(row) {
				style = pendingStyle
			} else if rowIdx%2 == 0 {
				style = altStyle
			}
			f.SetCellStyle(sheet, startRef, endRef, style) //nolint:errcheck
		}
	}

	// Freeze header row.
	f.SetPanes(sheet, &excelize.Panes{ //nolint:errcheck
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
		Selection:   []excelize.Selection{{SQRef: "A2", ActiveCell: "A2", Pane: "bottomLeft"}},
	})

	// Auto-size columns, capped at 80.
	const maxWidth = 80.0
	for i, w := range colWidths {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		width := float64(w) + 2 // padding
		if width > maxWidth {
			width = maxWidth
		}
		if width < 8 {
			width = 8
		}
		f.SetColWidth(sheet, colName, colName, width) //nolint:errcheck
	}

	return len(data) - 1, nil
}

// isPendingRow returns true when any cell in the row starts with "PENDING" or "NO LINKS".
func isPendingRow(row []string) bool {
	for _, cell := range row {
		up := strings.ToUpper(strings.TrimSpace(cell))
		if strings.HasPrefix(up, "PENDING") || strings.HasPrefix(up, "NO LINKS") {
			return true
		}
	}
	return false
}
