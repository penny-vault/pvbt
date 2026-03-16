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

var monthHeaders = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun",
	"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// renderMonthlyReturns writes the month x year returns grid.
func renderMonthlyReturns(builder *strings.Builder, monthly report.MonthlyReturns) {
	if len(monthly.Years) == 0 {
		return
	}

	builder.WriteString(sectionTitleStyle.Render("Monthly Returns"))
	builder.WriteString("\n")

	const (
		monthCol     = 7
		yearLabelCol = 6
	)

	// Header row.
	header := padRight("", yearLabelCol)
	for _, month := range monthHeaders {
		header += padLeft(tableHeaderStyle.Render(month), monthCol)
	}

	header += padLeft(tableHeaderStyle.Render("Year"), monthCol)
	builder.WriteString("  " + header + "\n")

	// Data rows.
	for yearIdx, year := range monthly.Years {
		line := padRight(labelStyle.Render(fmt.Sprintf("%d", year)), yearLabelCol)

		// Compute year total as compound of monthly returns.
		yearCompound := 1.0

		for monthIdx := 0; monthIdx < 12; monthIdx++ {
			val := math.NaN()
			if yearIdx < len(monthly.Values) && monthIdx < len(monthly.Values[yearIdx]) {
				val = monthly.Values[yearIdx][monthIdx]
			}

			if math.IsNaN(val) {
				line += padLeft("", monthCol)
			} else {
				yearCompound *= (1 + val)
				line += padLeft(fmtMonthlyPct(val), monthCol)
			}
		}

		// Year total column.
		yearTotal := yearCompound - 1
		line += padLeft(fmtMonthlyPct(yearTotal), monthCol)

		builder.WriteString("  " + line + "\n")
	}
}

// fmtMonthlyPct formats a monthly return as a compact percentage (e.g. "1.2%").
func fmtMonthlyPct(val float64) string {
	if math.IsNaN(val) {
		return ""
	}

	text := fmt.Sprintf("%.1f%%", val*100)

	if val >= 0 {
		return positiveStyle.Render(text)
	}

	return negativeStyle.Render(text)
}
