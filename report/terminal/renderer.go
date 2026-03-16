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

	"github.com/penny-vault/pvbt/report"
)

// Render writes a lipgloss-styled backtest report to the given writer.
func Render(rpt report.Report, writer io.Writer) error {
	var builder strings.Builder

	renderHeader(&builder, rpt.Header)

	// Show warnings if any.
	if len(rpt.Warnings) > 0 {
		renderWarnings(&builder, rpt.Warnings)
	}

	// If there is no equity curve data, show header + warnings only.
	if len(rpt.EquityCurve.StrategyValues) == 0 {
		_, err := io.WriteString(writer, builder.String())
		return err
	}

	// Full report sections.
	renderEquityCurve(&builder, rpt.EquityCurve, rpt.HasBenchmark)
	renderTrailingReturns(&builder, rpt.TrailingReturns, rpt.HasBenchmark)
	renderAnnualReturns(&builder, rpt.AnnualReturns, rpt.HasBenchmark)
	renderRisk(&builder, rpt.Risk, rpt.HasBenchmark)

	if rpt.HasBenchmark {
		renderRiskVsBenchmark(&builder, rpt.RiskVsBenchmark)
	}

	renderDrawdowns(&builder, rpt.Drawdowns)
	renderMonthlyReturns(&builder, rpt.MonthlyReturns)
	renderTrades(&builder, rpt.Trades)

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
