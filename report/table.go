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
		for colIdx := range tbl.Columns {
			cell := ""
			if colIdx < len(row) {
				cell = fmt.Sprintf("%v", row[colIdx])
			}

			formatted[rowIdx][colIdx] = cell
			if len(cell) > widths[colIdx] {
				widths[colIdx] = len(cell)
			}
		}
	}

	// Build format strings per column.
	fmts := make([]string, len(tbl.Columns))
	for colIdx, col := range tbl.Columns {
		switch col.Align {
		case "right":
			fmts[colIdx] = fmt.Sprintf("%%%ds", widths[colIdx])
		case "center":
			fmts[colIdx] = fmt.Sprintf("%%%ds", widths[colIdx]) // center approximated as right
		default: // left
			fmts[colIdx] = fmt.Sprintf("%%-%ds", widths[colIdx])
		}
	}

	// Header line.
	headers := make([]string, len(tbl.Columns))
	for colIdx, col := range tbl.Columns {
		headers[colIdx] = fmt.Sprintf(fmts[colIdx], col.Header)
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
		for colIdx := range tbl.Columns {
			cells[colIdx] = fmt.Sprintf(fmts[colIdx], row[colIdx])
		}

		if _, err := fmt.Fprintf(writer, "%s\n", strings.Join(cells, "  ")); err != nil {
			return err
		}
	}

	return nil
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
