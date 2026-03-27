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
	"io"
	"strings"

	"github.com/penny-vault/pvbt/study/report"
)

// Render writes a lipgloss-styled backtest report to the given writer.
// It type-asserts each section to the domain type for styled rendering.
func Render(rpt report.Report, writer io.Writer) error {
	var builder strings.Builder

	hasBenchmark := rpt.HasBenchmark

	for _, section := range rpt.Sections {
		switch sec := section.(type) {
		case *report.Header:
			renderHeader(&builder, *sec)
		case *report.EquityCurve:
			renderEquityCurve(&builder, *sec, hasBenchmark)
		case *report.ReturnTable:
			switch sec.SectionName {
			case "Recent Returns":
				renderRecentReturns(&builder, *sec, hasBenchmark)
			default:
				renderReturns(&builder, *sec, hasBenchmark)
			}
		case *report.AnnualReturns:
			renderAnnualReturns(&builder, *sec, hasBenchmark)
		case *report.Risk:
			renderRisk(&builder, *sec, hasBenchmark)
		case *report.RiskVsBenchmark:
			renderRiskVsBenchmark(&builder, *sec)
		case *report.Drawdowns:
			renderDrawdowns(&builder, *sec)
		case *report.MonthlyReturns:
			renderMonthlyReturns(&builder, *sec)
		case *report.Trades:
			renderTrades(&builder, *sec)
		default:
			// Fallback for unknown section types.
			if err := section.Render(report.FormatText, &builder); err != nil {
				return err
			}
		}
	}

	// Show warnings if any.
	if len(rpt.Warnings) > 0 {
		renderWarnings(&builder, rpt.Warnings)
	}

	builder.WriteString("\n")

	_, err := io.WriteString(writer, builder.String())

	return err
}

// renderWarnings writes warning messages to the builder.
func renderWarnings(builder *strings.Builder, warnings []string) {
	builder.WriteString("\n")

	for _, warning := range warnings {
		builder.WriteString(negativeStyle.Render("  WARNING: " + warning))
		builder.WriteString("\n")
	}
}
