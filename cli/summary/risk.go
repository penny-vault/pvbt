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

package summary

import (
	"math"
	"strings"

	"github.com/penny-vault/pvbt/portfolio"
)

func buildRisk(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) risk {
	rsk := risk{}

	type riskField struct {
		target *[2]float64
		metric portfolio.PerformanceMetric
	}

	fields := []riskField{
		{&rsk.maxDrawdown, portfolio.MaxDrawdown},
		{&rsk.volatility, portfolio.StdDev},
		{&rsk.downsideDeviation, portfolio.DownsideDeviation},
		{&rsk.sharpe, portfolio.Sharpe},
		{&rsk.sortino, portfolio.Sortino},
		{&rsk.calmar, portfolio.Calmar},
		{&rsk.avgUlcerIndex, portfolio.AvgUlcerIndex},
		{&rsk.p90UlcerIndex, portfolio.P90UlcerIndex},
		{&rsk.medianUlcerIndex, portfolio.MedianUlcerIndex},
		{&rsk.valueAtRisk, portfolio.ValueAtRisk},
		{&rsk.skewness, portfolio.Skewness},
		{&rsk.excessKurtosis, portfolio.ExcessKurtosis},
	}

	for _, field := range fields {
		field.target[0] = metricVal(acct, field.metric, warnings)
		if hasBenchmark {
			field.target[1] = metricValBenchmark(acct, field.metric, warnings)
		} else {
			field.target[1] = math.NaN()
		}
	}

	return rsk
}

func buildRiskVsBenchmark(acct portfolio.Portfolio, warnings *[]string) riskVsBenchmark {
	return riskVsBenchmark{
		beta:             metricVal(acct, portfolio.Beta, warnings),
		alpha:            metricVal(acct, portfolio.Alpha, warnings),
		rSquared:         metricVal(acct, portfolio.RSquared, warnings),
		trackingError:    metricVal(acct, portfolio.TrackingError, warnings),
		informationRatio: metricVal(acct, portfolio.InformationRatio, warnings),
		treynor:          metricVal(acct, portfolio.Treynor, warnings),
		upsideCapture:    metricVal(acct, portfolio.UpsideCaptureRatio, warnings),
		downsideCapture:  metricVal(acct, portfolio.DownsideCaptureRatio, warnings),
	}
}

// renderRisk writes the risk metrics table with Strategy and Benchmark columns.
func renderRisk(builder *strings.Builder, rsk risk, hasBenchmark bool) {
	builder.WriteString(sectionTitleStyle.Render("Risk Metrics"))
	builder.WriteString("\n")

	// Column widths.
	const (
		nameCol = 22
		valCol  = 14
	)

	// Header.
	hdr := padRight(labelStyle.Render(""), nameCol) +
		padLeft(tableHeaderStyle.Render("Strategy"), valCol)
	if hasBenchmark {
		hdr += padLeft(tableHeaderStyle.Render("Benchmark"), valCol)
	}

	builder.WriteString("  " + hdr + "\n")

	type riskRow struct {
		label  string
		values [2]float64
		format func(float64) string
	}

	rows := []riskRow{
		{"Max Drawdown", rsk.maxDrawdown, fmtPct},
		{"Volatility", rsk.volatility, fmtPct},
		{"Downside Deviation", rsk.downsideDeviation, fmtPct},
		{"Sharpe", rsk.sharpe, fmtRatio},
		{"Sortino", rsk.sortino, fmtRatio},
		{"Calmar", rsk.calmar, fmtRatio},
		{"Avg Ulcer Index", rsk.avgUlcerIndex, fmtRatio},
		{"P90 Ulcer Index", rsk.p90UlcerIndex, fmtRatio},
		{"Median Ulcer Index", rsk.medianUlcerIndex, fmtRatio},
		{"Value at Risk (95%)", rsk.valueAtRisk, fmtPct},
		{"Skewness", rsk.skewness, fmtRatio},
		{"Excess Kurtosis", rsk.excessKurtosis, fmtRatio},
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
func renderRiskVsBenchmark(builder *strings.Builder, rvb riskVsBenchmark) {
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
			{label: "Beta", value: fmtRatio(rvb.beta)},
			{label: "Alpha", value: fmtPct(rvb.alpha)},
		},
		{
			{label: "R-Squared", value: fmtRatio(rvb.rSquared)},
			{label: "Tracking Error", value: fmtPct(rvb.trackingError)},
		},
		{
			{label: "Info Ratio", value: fmtRatio(rvb.informationRatio)},
			{label: "Treynor", value: fmtRatio(rvb.treynor)},
		},
		{
			{label: "Upside Capture", value: fmtPct(rvb.upsideCapture)},
			{label: "Downside Capture", value: fmtPct(rvb.downsideCapture)},
		},
	}

	for _, row := range grid {
		left := padRight(labelStyle.Render(row[0].label), labelCol) + padLeft(row[0].value, valCol)
		right := padRight(labelStyle.Render(row[1].label), labelCol) + padLeft(row[1].value, valCol)
		builder.WriteString("  " + left + "  " + right + "\n")
	}
}
