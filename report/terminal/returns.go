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

package terminal

import (
	"fmt"
	"math"
	"strings"

	"github.com/penny-vault/pvbt/report"
)

const colWidth = 12

// renderTrailingReturns writes the trailing returns table.
func renderTrailingReturns(builder *strings.Builder, trailing report.TrailingReturns, hasBenchmark bool) {
	if len(trailing.Periods) == 0 {
		return
	}

	builder.WriteString(sectionTitleStyle.Render("Trailing Returns"))
	builder.WriteString("\n")

	// Header row.
	header := padRight(labelStyle.Render(""), colWidth)
	for _, period := range trailing.Periods {
		header += padLeft(tableHeaderStyle.Render(period), colWidth)
	}

	builder.WriteString("  " + header + "\n")

	// Strategy row.
	stratRow := padRight(labelStyle.Render("Strategy"), colWidth)
	for _, val := range trailing.Strategy {
		stratRow += padLeft(fmtPct(val), colWidth)
	}

	builder.WriteString("  " + stratRow + "\n")

	// Benchmark row (if present).
	if hasBenchmark {
		benchRow := padRight(labelStyle.Render("Benchmark"), colWidth)
		for _, val := range trailing.Benchmark {
			benchRow += padLeft(fmtPct(val), colWidth)
		}

		builder.WriteString("  " + benchRow + "\n")

		// Diff row.
		diffRow := padRight(labelStyle.Render("+/-"), colWidth)

		for idx := range trailing.Strategy {
			diff := trailing.Strategy[idx] - trailing.Benchmark[idx]
			if math.IsNaN(trailing.Strategy[idx]) || math.IsNaN(trailing.Benchmark[idx]) {
				diff = math.NaN()
			}

			diffRow += padLeft(fmtPctDiff(diff), colWidth)
		}

		builder.WriteString("  " + diffRow + "\n")
	}
}

// renderAnnualReturns writes the annual returns table with years as rows
// so that it remains readable regardless of how many years are present.
func renderAnnualReturns(builder *strings.Builder, annual report.AnnualReturns, hasBenchmark bool) {
	if len(annual.Years) == 0 {
		return
	}

	builder.WriteString(sectionTitleStyle.Render("Annual Returns"))
	builder.WriteString("\n")

	// Column header row.
	header := padRight(labelStyle.Render(""), colWidth)
	header += padLeft(tableHeaderStyle.Render("Strategy"), colWidth)

	if hasBenchmark && len(annual.Benchmark) > 0 {
		header += padLeft(tableHeaderStyle.Render("Benchmark"), colWidth)
		header += padLeft(tableHeaderStyle.Render("+/-"), colWidth)
	}

	builder.WriteString("  " + header + "\n")

	// One row per year.
	for idx, year := range annual.Years {
		row := padRight(tableHeaderStyle.Render(fmt.Sprintf("%d", year)), colWidth)

		stratVal := math.NaN()
		if idx < len(annual.Strategy) {
			stratVal = annual.Strategy[idx]
		}

		row += padLeft(fmtPct(stratVal), colWidth)

		if hasBenchmark && len(annual.Benchmark) > 0 {
			benchVal := math.NaN()
			if idx < len(annual.Benchmark) {
				benchVal = annual.Benchmark[idx]
			}

			row += padLeft(fmtPct(benchVal), colWidth)

			diff := math.NaN()
			if !math.IsNaN(stratVal) && !math.IsNaN(benchVal) {
				diff = stratVal - benchVal
			}

			row += padLeft(fmtPctDiff(diff), colWidth)
		}

		builder.WriteString("  " + row + "\n")
	}
}
