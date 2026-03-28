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
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"io"
)

//go:embed base.html
var baseTemplate string

//go:embed bundle.js
var bundle string

// Report is the interface for Vue-based HTML reports. Each report
// provides a component name and its data as JSON.
type Report interface {
	// Name returns the Vue component name to mount (e.g. "MonteCarlo").
	Name() string

	// Data writes the report's JSON data to writer.
	Data(writer io.Writer) error
}

// templateData holds the values injected into the base HTML template.
type templateData struct {
	Title     string
	Data      template.JS
	Bundle    template.JS
	Component string
}

// Render writes a self-contained HTML file for the given report to w.
func Render(rpt Report, writer io.Writer) error {
	var dataBuf bytes.Buffer
	if err := rpt.Data(&dataBuf); err != nil {
		return fmt.Errorf("generating report data: %w", err)
	}

	tmpl, err := template.New("report").Parse(baseTemplate)
	if err != nil {
		return fmt.Errorf("parsing base template: %w", err)
	}

	td := templateData{
		Title:     rpt.Name(),
		Data:      template.JS(dataBuf.String()),
		Bundle:    template.JS(bundle),
		Component: rpt.Name(),
	}

	if err := tmpl.Execute(writer, td); err != nil {
		return fmt.Errorf("executing report template: %w", err)
	}

	return nil
}
