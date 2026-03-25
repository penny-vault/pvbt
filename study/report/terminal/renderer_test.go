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

package terminal_test

import (
	"bytes"
	"testing"

	"github.com/penny-vault/pvbt/study/report"
	"github.com/penny-vault/pvbt/study/report/terminal"
)

func TestRenderDoesNotPanic(t *testing.T) {
	rpt := report.Report{
		Title: "TestStrategy",
		Sections: []report.Section{
			&report.Text{SectionName: "Warnings", Body: "WARNING: insufficient data for full report\n"},
		},
	}

	var buf bytes.Buffer

	err := terminal.Render(rpt, &buf)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("Render produced empty output")
	}
}

func TestRenderFullReport(t *testing.T) {
	rpt := report.Report{
		Title: "MyStrat",
		Sections: []report.Section{
			&report.MetricPairs{
				SectionName: "Header",
				Metrics: []report.MetricPair{
					{Label: "Strategy", Value: 0, Format: "label:MyStrat"},
					{Label: "Initial Cash", Value: 100000, Format: "currency"},
					{Label: "Final Value", Value: 120000, Format: "currency"},
				},
			},
			&report.TimeSeries{
				SectionName: "Equity Curve",
				Series: []report.NamedSeries{
					{Name: "Strategy", Values: []float64{100000, 120000}},
				},
			},
			&report.Table{
				SectionName: "Recent Returns",
				Columns: []report.Column{
					{Header: "Period", Format: "string", Align: "left"},
					{Header: "Strategy", Format: "percent", Align: "right"},
				},
				Rows: [][]any{{"1D", 0.001}},
			},
			&report.MetricPairs{
				SectionName: "Risk Metrics",
				Metrics: []report.MetricPair{
					{Label: "Sharpe", Value: 1.2, Format: "ratio"},
				},
			},
			&report.Table{
				SectionName: "Top Drawdowns",
				Columns: []report.Column{
					{Header: "#", Format: "string", Align: "left"},
					{Header: "Depth", Format: "percent", Align: "right"},
				},
				Rows: [][]any{{"1", -0.08}},
			},
			&report.Table{
				SectionName: "Monthly Returns",
				Columns: []report.Column{
					{Header: "Year", Format: "string", Align: "left"},
					{Header: "Jan", Format: "percent", Align: "right"},
				},
				Rows: [][]any{{"2024", 0.02}},
			},
			&report.MetricPairs{
				SectionName: "Trade Summary",
				Metrics: []report.MetricPair{
					{Label: "Win Rate", Value: 0.80, Format: "percent"},
				},
			},
		},
	}

	var buf bytes.Buffer

	err := terminal.Render(rpt, &buf)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Fatal("Render produced empty output for full report")
	}

	// Verify key sections appear in the output.
	for _, section := range []string{
		"MyStrat",
		"Recent Returns",
		"Risk Metrics",
		"Top Drawdowns",
		"Monthly Returns",
		"Trade Summary",
	} {
		if !bytes.Contains([]byte(output), []byte(section)) {
			t.Errorf("output missing section: %s", section)
		}
	}
}
