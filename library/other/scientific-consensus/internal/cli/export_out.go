// internal/cli/export_out.go
package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

	"github.com/xuri/excelize/v2"
)

// WriteJSON writes data as JSON to a file.
func WriteJSON(filename string, data any) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// WriteCSV writes data as CSV with UTF-8 BOM.
func WriteCSV(filename string, headers []string, rows [][]string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer file.Close()

	// UTF-8 BOM for Excel compatibility
	if _, err := file.WriteString("\xEF\xBB\xBF"); err != nil {
		return err
	}

	writer := csv.NewWriter(file)
	writer.Comma = ';'

	if err := writer.Write(headers); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

// WriteXLSX writes data to a formatted Excel file.
func WriteXLSX(filename string, sheetName string, headers []string, rows [][]string, colWidths map[int]float64) error {
	f := excelize.NewFile()
	defer f.Close()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		sheet = "Sheet1"
	}

	// Write headers
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Write data rows
	for rowIdx, row := range rows {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			f.SetCellValue(sheet, cell, val)
		}
	}

	// Set column widths
	for colIdx, width := range colWidths {
		colName, _ := excelize.ColumnNumberToName(colIdx + 1)
		f.SetColWidth(sheet, colName, colName, width)
	}

	// Header style
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#D9E1F2"}, Pattern: 1},
	})
	for i := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	// Freeze header row
	f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	// AutoFilter
	if len(rows) > 0 {
		endCol, _ := excelize.CoordinatesToCellName(len(headers), len(rows)+1)
		f.AutoFilter(sheet, "A1:"+endCol, []excelize.AutoFilterOptions{})
	}

	return f.SaveAs(filename)
}