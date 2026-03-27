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
	"strings"

	"github.com/penny-vault/pvbt/study/report"
)

// renderRisk writes the risk metrics table with Strategy and Benchmark columns.
func renderRisk(builder *strings.Builder, risk report.Risk, hasBenchmark bool) {
	builder.WriteString(sectionTitleStyle.Render("Risk Metrics"))
	builder.WriteString("\n")

	// Column widths.
	const (
		nameCol = 22
		valCol  = 14
	)

	// Header.
	header := padRight(labelStyle.Render(""), nameCol) +
		padLeft(tableHeaderStyle.Render("Strategy"), valCol)
	if hasBenchmark {
		header += padLeft(tableHeaderStyle.Render("Benchmark"), valCol)
	}

	builder.WriteString("  " + header + "\n")

	type riskRow struct {
		label  string
		values [2]float64
		format func(float64) string
	}

	rows := []riskRow{
		{"Max Drawdown", risk.MaxDrawdown, fmtPct},
		{"Volatility", risk.Volatility, fmtPct},
		{"Downside Deviation", risk.DownsideDeviation, fmtPct},
		{"Sharpe", risk.Sharpe, fmtRatio},
		{"Sortino", risk.Sortino, fmtRatio},
		{"Calmar", risk.Calmar, fmtRatio},
		{"Ulcer Index", risk.UlcerIndex, fmtRatio},
		{"Value at Risk (95%)", risk.ValueAtRisk, fmtPct},
		{"Skewness", risk.Skewness, fmtRatio},
		{"Excess Kurtosis", risk.ExcessKurtosis, fmtRatio},
	}

	for _, row := range rows {
		line := padRight(labelStyle.Render(row.label), nameCol) +
			padLeft(row.format(row.values[0]), valCol)
		if hasBenchmark {
			line += padLeft(row.format(row.values[1]), valCol)
		}

		builder.WriteString("  " + line + "\n")
	}
}

// renderRiskVsBenchmark writes the relative-risk 2x4 grid.
func renderRiskVsBenchmark(builder *strings.Builder, rvb report.RiskVsBenchmark) {
	builder.WriteString(sectionTitleStyle.Render("Risk vs Benchmark"))
	builder.WriteString("\n")

	const (
		labelCol = 20
		valCol   = 14
	)

	type gridEntry struct {
		label string
		value string
	}

	// 4 rows, 2 pairs each.
	grid := [][2]gridEntry{
		{
			{label: "Beta", value: fmtRatio(rvb.Beta)},
			{label: "Alpha", value: fmtPct(rvb.Alpha)},
		},
		{
			{label: "R-Squared", value: fmtRatio(rvb.RSquared)},
			{label: "Tracking Error", value: fmtPct(rvb.TrackingError)},
		},
		{
			{label: "Info Ratio", value: fmtRatio(rvb.InformationRatio)},
			{label: "Treynor", value: fmtRatio(rvb.Treynor)},
		},
		{
			{label: "Upside Capture", value: fmtPct(rvb.UpsideCapture)},
			{label: "Downside Capture", value: fmtPct(rvb.DownsideCapture)},
		},
	}

	for _, row := range grid {
		left := padRight(labelStyle.Render(row[0].label), labelCol) + padLeft(row[0].value, valCol)
		right := padRight(labelStyle.Render(row[1].label), labelCol) + padLeft(row[1].value, valCol)
		builder.WriteString("  " + left + "  " + right + "\n")
	}
}
