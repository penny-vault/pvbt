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
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/report"
)

const reportWidth = 80

// renderHeader writes the three-line header block to the builder.
func renderHeader(builder *strings.Builder, header report.Header) {
	// Line 1: Strategy Name (left) + date range (right)
	dateRange := fmt.Sprintf("%s to %s",
		header.StartDate.Format("2006-01-02"),
		header.EndDate.Format("2006-01-02"))

	nameText := headerStyle.Render(header.StrategyName)
	line1 := nameText + padLeft(subHeaderStyle.Render(dateRange), reportWidth-lipgloss.Width(nameText))
	builder.WriteString(line1)
	builder.WriteString("\n")

	// Line 2: Strategy: name vX.X.X (left) + Benchmark: NAME (right)
	stratLabel := fmt.Sprintf("Strategy: %s v%s", header.StrategyName, header.StrategyVersion)
	stratText := subHeaderStyle.Render("  " + stratLabel)

	benchText := ""

	if header.Benchmark != "" {
		benchLabel := fmt.Sprintf("Benchmark: %s", header.Benchmark)
		benchText = subHeaderStyle.Render(benchLabel)
	}

	line2 := stratText + padLeft(benchText, reportWidth-lipgloss.Width(stratText))
	builder.WriteString(line2)
	builder.WriteString("\n")

	// Line 3: Initial + Final + Elapsed
	initialText := labelStyle.Render("  Initial: ") + fmtCurrencyLarge(header.InitialCash)
	finalText := labelStyle.Render("Final: ") + fmtCurrencyLarge(header.FinalValue)

	elapsedStr := header.Elapsed.Round(100 * 1e6).String() // e.g. "1.2s"
	elapsedText := labelStyle.Render("Elapsed: ") +
		valueStyle.Render(fmt.Sprintf("%s (%d steps)", elapsedStr, header.Steps))

	// Space the three parts across the line.
	gap1 := reportWidth/3 - lipgloss.Width(initialText)
	if gap1 < 2 {
		gap1 = 2
	}

	gap2 := reportWidth - lipgloss.Width(initialText) - gap1 - lipgloss.Width(finalText) - lipgloss.Width(elapsedText)
	if gap2 < 2 {
		gap2 = 2
	}

	line3 := initialText + strings.Repeat(" ", gap1) + finalText + strings.Repeat(" ", gap2) + elapsedText
	builder.WriteString(line3)
	builder.WriteString("\n")
}
