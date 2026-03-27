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
	"fmt"
	"math"
)

// ---------------------------------------------------------------------------
// Conversion functions -- used by Section.Render implementations in
// section_impls.go to delegate text/JSON rendering to the generic
// section types (MetricPairs, Table, TimeSeries).
// ---------------------------------------------------------------------------

func headerToMetricPairs(header Header) *MetricPairs {
	metrics := []MetricPair{
		{Label: "Strategy", Value: 0, Format: "label:" + header.StrategyName},
	}

	if header.StrategyVersion != "" {
		metrics = append(metrics, MetricPair{
			Label: "Version", Value: 0, Format: "label:" + header.StrategyVersion,
		})
	}

	if header.Benchmark != "" {
		metrics = append(metrics, MetricPair{
			Label: "Benchmark", Value: 0, Format: "label:" + header.Benchmark,
		})
	}

	if !header.StartDate.IsZero() && !header.EndDate.IsZero() {
		dateRange := fmt.Sprintf("%s to %s",
			header.StartDate.Format("2006-01-02"),
			header.EndDate.Format("2006-01-02"))
		metrics = append(metrics, MetricPair{
			Label: "Period", Value: 0, Format: "label:" + dateRange,
		})
	}

	metrics = append(metrics, MetricPair{
		Label: "Initial Cash", Value: header.InitialCash, Format: "currency",
	})

	metrics = append(metrics, MetricPair{
		Label: "Final Value", Value: header.FinalValue, Format: "currency",
	})

	if header.Elapsed > 0 {
		metrics = append(metrics, MetricPair{
			Label: "Elapsed", Value: 0,
			Format: "label:" + fmt.Sprintf("%s (%d steps)", header.Elapsed, header.Steps),
		})
	}

	return &MetricPairs{
		SectionName: "Header",
		Metrics:     metrics,
	}
}

func equityCurveToTimeSeries(curve EquityCurve) *TimeSeries {
	series := []NamedSeries{
		{Name: "Strategy", Times: curve.Times, Values: curve.StrategyValues},
	}

	if len(curve.BenchmarkValues) > 0 {
		series = append(series, NamedSeries{Name: "Benchmark", Times: curve.Times, Values: curve.BenchmarkValues})
	}

	return &TimeSeries{SectionName: "Equity Curve", Series: series}
}

func returnTableToSection(name string, table ReturnTable, hasBenchmark bool) *Table {
	sectionName := name
	if !table.AsOf.IsZero() {
		sectionName = fmt.Sprintf("%s (as of %s)", name, table.AsOf.Format("2006-01-02"))
	}

	columns := []Column{
		{Header: "Period", Format: "string", Align: "left"},
		{Header: "Strategy", Format: "percent", Align: "right"},
	}

	if hasBenchmark {
		columns = append(columns,
			Column{Header: "Benchmark", Format: "percent", Align: "right"},
			Column{Header: "+/-", Format: "percent", Align: "right"},
		)
	}

	rows := make([][]any, len(table.Periods))

	for idx := range table.Periods {
		row := []any{table.Periods[idx], table.Strategy[idx]}

		if hasBenchmark {
			benchVal := table.Benchmark[idx]
			diff := table.Strategy[idx] - benchVal

			if math.IsNaN(table.Strategy[idx]) || math.IsNaN(benchVal) {
				diff = math.NaN()
			}

			row = append(row, benchVal, diff)
		}

		rows[idx] = row
	}

	return &Table{SectionName: sectionName, Columns: columns, Rows: rows}
}

func annualReturnsToSection(annual AnnualReturns, hasBenchmark bool) *Table {
	showBenchmark := hasBenchmark && len(annual.Benchmark) > 0

	columns := []Column{
		{Header: "Year", Format: "string", Align: "left"},
		{Header: "Strategy", Format: "percent", Align: "right"},
	}

	if showBenchmark {
		columns = append(columns,
			Column{Header: "Benchmark", Format: "percent", Align: "right"},
			Column{Header: "+/-", Format: "percent", Align: "right"},
		)
	}

	rows := make([][]any, len(annual.Years))

	for idx, year := range annual.Years {
		stratVal := math.NaN()
		if idx < len(annual.Strategy) {
			stratVal = annual.Strategy[idx]
		}

		row := []any{fmt.Sprintf("%d", year), stratVal}

		if showBenchmark {
			benchVal := math.NaN()
			if idx < len(annual.Benchmark) {
				benchVal = annual.Benchmark[idx]
			}

			diff := math.NaN()
			if !math.IsNaN(stratVal) && !math.IsNaN(benchVal) {
				diff = stratVal - benchVal
			}

			row = append(row, benchVal, diff)
		}

		rows[idx] = row
	}

	return &Table{SectionName: "Annual Returns", Columns: columns, Rows: rows}
}

func riskToMetricPairs(risk Risk, hasBenchmark bool) *MetricPairs {
	type riskDef struct {
		label     string
		values    [2]float64
		metricFmt string
	}

	defs := []riskDef{
		{"Max Drawdown", risk.MaxDrawdown, "percent"},
		{"Volatility", risk.Volatility, "percent"},
		{"Downside Deviation", risk.DownsideDeviation, "percent"},
		{"Sharpe", risk.Sharpe, "ratio"},
		{"Sortino", risk.Sortino, "ratio"},
		{"Calmar", risk.Calmar, "ratio"},
		{"Ulcer Index", risk.UlcerIndex, "ratio"},
		{"Value at Risk (95%)", risk.ValueAtRisk, "percent"},
		{"Skewness", risk.Skewness, "ratio"},
		{"Excess Kurtosis", risk.ExcessKurtosis, "ratio"},
	}

	metrics := make([]MetricPair, len(defs))
	for idx, def := range defs {
		pair := MetricPair{Label: def.label, Value: def.values[0], Format: def.metricFmt}

		if hasBenchmark && !math.IsNaN(def.values[1]) {
			benchVal := def.values[1]
			pair.Comparison = &benchVal
		}

		metrics[idx] = pair
	}

	return &MetricPairs{SectionName: "Risk Metrics", Metrics: metrics}
}

func riskVsBenchmarkToMetricPairs(rvb RiskVsBenchmark) *MetricPairs {
	return &MetricPairs{
		SectionName: "Risk vs Benchmark",
		Metrics: []MetricPair{
			{Label: "Beta", Value: rvb.Beta, Format: "ratio"},
			{Label: "Alpha", Value: rvb.Alpha, Format: "percent"},
			{Label: "R-Squared", Value: rvb.RSquared, Format: "ratio"},
			{Label: "Tracking Error", Value: rvb.TrackingError, Format: "percent"},
			{Label: "Info Ratio", Value: rvb.InformationRatio, Format: "ratio"},
			{Label: "Treynor", Value: rvb.Treynor, Format: "ratio"},
			{Label: "Upside Capture", Value: rvb.UpsideCapture, Format: "percent"},
			{Label: "Downside Capture", Value: rvb.DownsideCapture, Format: "percent"},
		},
	}
}

func drawdownsToSection(drawdowns Drawdowns) *Table {
	columns := []Column{
		{Header: "#", Format: "string", Align: "left"},
		{Header: "Start", Format: "date", Align: "left"},
		{Header: "End", Format: "string", Align: "left"},
		{Header: "Recovery", Format: "string", Align: "left"},
		{Header: "Depth", Format: "percent", Align: "right"},
		{Header: "Duration", Format: "string", Align: "right"},
	}

	rows := make([][]any, len(drawdowns.Entries))
	for idx, entry := range drawdowns.Entries {
		endStr := entry.End.Format("2006-01-02")
		if entry.End.IsZero() {
			endStr = "ongoing"
		}

		recoveryStr := "ongoing"
		if !entry.Recovery.IsZero() {
			recoveryStr = entry.Recovery.Format("2006-01-02")
		}

		rows[idx] = []any{
			fmt.Sprintf("%d", idx+1),
			entry.Start, endStr, recoveryStr,
			entry.Depth, fmt.Sprintf("%d days", entry.Days),
		}
	}

	return &Table{SectionName: "Top Drawdowns", Columns: columns, Rows: rows}
}

func monthlyReturnsToSection(monthly MonthlyReturns) *Table {
	monthHeaders := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

	columns := []Column{{Header: "Year", Format: "string", Align: "left"}}
	for _, month := range monthHeaders {
		columns = append(columns, Column{Header: month, Format: "percent", Align: "right"})
	}

	columns = append(columns, Column{Header: "Total", Format: "percent", Align: "right"})

	rows := make([][]any, len(monthly.Years))

	for yearIdx, year := range monthly.Years {
		row := make([]any, 0, 14)
		row = append(row, fmt.Sprintf("%d", year))

		yearCompound := 1.0

		for monthIdx := 0; monthIdx < 12; monthIdx++ {
			val := math.NaN()
			if yearIdx < len(monthly.Values) && monthIdx < len(monthly.Values[yearIdx]) {
				val = monthly.Values[yearIdx][monthIdx]
			}

			if !math.IsNaN(val) {
				yearCompound *= (1 + val)
			}

			row = append(row, val)
		}

		row = append(row, yearCompound-1)
		rows[yearIdx] = row
	}

	return &Table{SectionName: "Monthly Returns", Columns: columns, Rows: rows}
}

func tradesToSections(trades Trades) []Section {
	summaryMetrics := []MetricPair{
		{Label: "Total Transactions", Value: float64(trades.TotalTransactions), Format: "number"},
		{Label: "Round Trips", Value: float64(trades.RoundTrips), Format: "number"},
		{Label: "Win Rate", Value: trades.WinRate, Format: "percent"},
		{Label: "Avg Holding", Value: trades.AvgHolding, Format: "days"},
		{Label: "Avg Win", Value: trades.AvgWin, Format: "currency"},
		{Label: "Avg Loss", Value: trades.AvgLoss, Format: "currency"},
		{Label: "Profit Factor", Value: trades.ProfitFactor, Format: "ratio"},
		{Label: "Gain/Loss Ratio", Value: trades.GainLossRatio, Format: "ratio"},
		{Label: "Turnover", Value: trades.Turnover, Format: "percent"},
		{Label: "Positive Periods", Value: trades.PositivePeriods, Format: "percent"},
	}

	sections := []Section{&MetricPairs{SectionName: "Trade Summary", Metrics: summaryMetrics}}

	if len(trades.Trades) > 0 {
		maxTradesShown := 10

		columns := []Column{
			{Header: "Date", Format: "date", Align: "left"},
			{Header: "Action", Format: "string", Align: "left"},
			{Header: "Ticker", Format: "string", Align: "left"},
			{Header: "Shares", Format: "number", Align: "right"},
			{Header: "Price", Format: "currency", Align: "right"},
			{Header: "Amount", Format: "currency", Align: "right"},
		}

		startIdx := 0
		if len(trades.Trades) > maxTradesShown {
			startIdx = len(trades.Trades) - maxTradesShown
		}

		rows := make([][]any, 0, len(trades.Trades)-startIdx)
		for idx := startIdx; idx < len(trades.Trades); idx++ {
			trade := trades.Trades[idx]
			rows = append(rows, []any{trade.Date, trade.Action, trade.Ticker, trade.Shares, trade.Price, trade.Amount})
		}

		sections = append(sections, &Table{SectionName: "Recent Trades", Columns: columns, Rows: rows})
	}

	return sections
}
