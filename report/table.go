// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Column describes a single column in a Table section.
type Column struct {
	Header string `json:"header"`
	Format string `json:"format"` // "percent", "currency", "number", "string", "date"
	Align  string `json:"align"`  // "left", "right", "center"
}

// Table is a Section that renders tabular data with typed, aligned columns.
type Table struct {
	SectionName string   `json:"name"`
	Columns     []Column `json:"columns"`
	Rows        [][]any  `json:"rows"`
}

// Type returns the discriminator "table".
func (tbl *Table) Type() string { return "table" }

// Name returns the human-readable section heading.
func (tbl *Table) Name() string { return tbl.SectionName }

// Render writes the table in the requested format.
func (tbl *Table) Render(format Format, writer io.Writer) error {
	switch format {
	case FormatText:
		return tbl.renderText(writer)
	case FormatJSON:
		return tbl.renderJSON(writer)
	default:
		return fmt.Errorf("unsupported format %q for table section", format)
	}
}

func (tbl *Table) renderText(writer io.Writer) error {
	if len(tbl.Columns) == 0 {
		return nil
	}

	// Compute column widths from headers and cell values.
	widths := make([]int, len(tbl.Columns))
	for colIdx, col := range tbl.Columns {
		widths[colIdx] = len(col.Header)
	}

	formatted := make([][]string, len(tbl.Rows))
	for rowIdx, row := range tbl.Rows {
		formatted[rowIdx] = make([]string, len(tbl.Columns))
		for colIdx, col := range tbl.Columns {
			cell := ""
			if colIdx < len(row) {
				cell = formatTableCell(row[colIdx], col.Format)
			}

			formatted[rowIdx][colIdx] = cell
			if len(cell) > widths[colIdx] {
				widths[colIdx] = len(cell)
			}
		}
	}

	// Build aligned cells using the computed column widths.
	alignCell := func(text string, width int, align string) string {
		switch align {
		case "right":
			return fmt.Sprintf("%*s", width, text)
		case "center":
			pad := width - len(text)
			leftPad := pad / 2
			rightPad := pad - leftPad

			return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
		default: // left
			return fmt.Sprintf("%-*s", width, text)
		}
	}

	// Header line.
	headers := make([]string, len(tbl.Columns))
	for colIdx, col := range tbl.Columns {
		headers[colIdx] = alignCell(col.Header, widths[colIdx], col.Align)
	}

	if _, err := fmt.Fprintf(writer, "%s\n", strings.Join(headers, "  ")); err != nil {
		return err
	}

	// Separator line.
	separators := make([]string, len(tbl.Columns))
	for colIdx := range tbl.Columns {
		separators[colIdx] = strings.Repeat("-", widths[colIdx])
	}

	if _, err := fmt.Fprintf(writer, "%s\n", strings.Join(separators, "  ")); err != nil {
		return err
	}

	// Data rows.
	for _, row := range formatted {
		cells := make([]string, len(tbl.Columns))
		for colIdx, col := range tbl.Columns {
			cells[colIdx] = alignCell(row[colIdx], widths[colIdx], col.Align)
		}

		if _, err := fmt.Fprintf(writer, "%s\n", strings.Join(cells, "  ")); err != nil {
			return err
		}
	}

	return nil
}

// formatTableCell converts a raw cell value to its display string using the column format.
func formatTableCell(value any, colFormat string) string {
	switch colFormat {
	case "percent":
		switch typed := value.(type) {
		case float64:
			return fmt.Sprintf("%.2f%%", typed*100)
		case float32:
			return fmt.Sprintf("%.2f%%", float64(typed)*100)
		}
	case "currency":
		var amount float64

		switch typed := value.(type) {
		case float64:
			amount = typed
		case float32:
			amount = float64(typed)
		case int:
			amount = float64(typed)
		case int64:
			amount = float64(typed)
		default:
			return fmt.Sprintf("%v", value)
		}

		// Format with comma-separated thousands and two decimal places.
		formatted := fmt.Sprintf("%.2f", amount)

		// Insert commas into the integer part.
		dotIdx := strings.Index(formatted, ".")
		intPart := formatted[:dotIdx]
		fracPart := formatted[dotIdx:]

		negative := strings.HasPrefix(intPart, "-")
		if negative {
			intPart = intPart[1:]
		}

		var sb strings.Builder

		for idx, digit := range intPart {
			if idx > 0 && (len(intPart)-idx)%3 == 0 {
				sb.WriteByte(',')
			}

			sb.WriteRune(digit)
		}

		prefix := "$"
		if negative {
			prefix = "-$"
		}

		return prefix + sb.String() + fracPart
	case "number":
		switch typed := value.(type) {
		case float64:
			return fmt.Sprintf("%g", typed)
		case float32:
			return fmt.Sprintf("%g", typed)
		}
	case "date":
		if typed, ok := value.(time.Time); ok {
			return typed.Format("2006-01-02")
		}
	}
	// "string" format and any unrecognised format fall through to %v.
	return fmt.Sprintf("%v", value)
}

func (tbl *Table) renderJSON(writer io.Writer) error {
	envelope := struct {
		Type    string   `json:"type"`
		Name    string   `json:"name"`
		Columns []Column `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}{
		Type:    tbl.Type(),
		Name:    tbl.SectionName,
		Columns: tbl.Columns,
		Rows:    tbl.Rows,
	}

	encoder := json.NewEncoder(writer)

	return encoder.Encode(envelope)
}
