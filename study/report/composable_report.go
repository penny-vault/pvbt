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

// ComposableReport is a titled collection of renderable sections.
type ComposableReport struct {
	Title        string
	Sections     []Section
	HasBenchmark bool
	Warnings     []string
}

// Render writes the report in the requested format to writer.
func (rpt ComposableReport) Render(format Format, writer io.Writer) error {
	switch format {
	case FormatJSON:
		return rpt.renderJSON(writer)
	case FormatText:
		return rpt.renderText(writer)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func (rpt ComposableReport) renderJSON(writer io.Writer) error {
	_, err := fmt.Fprintf(writer, `{"title":%s,"sections":[`, jsonString(rpt.Title))
	if err != nil {
		return err
	}

	for idx, section := range rpt.Sections {
		if idx > 0 {
			if _, err := writer.Write([]byte(",")); err != nil {
				return err
			}
		}

		if err := section.Render(FormatJSON, writer); err != nil {
			return fmt.Errorf("rendering section %q as JSON: %w", section.Name(), err)
		}
	}

	_, err = writer.Write([]byte("]}"))

	return err
}

func jsonString(str string) string {
	encoded, err := json.Marshal(str)
	if err != nil {
		// json.Marshal on a string only fails for invalid UTF-8, which Go strings
		// can technically contain but almost never do in practice. Fall back to
		// a quoted literal that is always valid JSON.
		return `""`
	}

	return string(encoded)
}

func (rpt ComposableReport) renderText(writer io.Writer) error {
	if _, err := fmt.Fprintf(writer, "\n  %s\n", rpt.Title); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(writer, strings.Repeat("─", len(rpt.Title)+4)); err != nil {
		return err
	}

	for _, section := range rpt.Sections {
		if _, err := fmt.Fprintln(writer); err != nil {
			return err
		}

		if _, err := fmt.Fprintf(writer, "  %s\n", section.Name()); err != nil {
			return err
		}

		if err := section.Render(FormatText, writer); err != nil {
			return fmt.Errorf("rendering section %q: %w", section.Name(), err)
		}
	}

	// Append warnings.
	for _, warning := range rpt.Warnings {
		if _, err := fmt.Fprintf(writer, "WARNING: %s\n", warning); err != nil {
			return err
		}
	}

	return nil
}
