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
	"strconv"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

// Summary constructs a Report from the given portfolio. It reuses
// the existing build* helpers to compute data and maps their results into
// Section primitives (MetricPairs, Table, TimeSeries, Text).
func Summary(acct ReportablePortfolio) (Report, error) {
	var warnings []string

	perfData := acct.PerfData()

	// Build the header from portfolio metadata.
	header := buildHeaderData(acct)

	// Determine whether a benchmark is configured.
	hasBenchmark := acct.Benchmark() != (asset.Asset{})

	if perfData != nil && perfData.Len() > 0 {
		header.StartDate = perfData.Start()
		header.EndDate = perfData.End()
		header.Steps = perfData.Len()
	}

	// Convert header to a MetricPairs section.
	headerSection := headerToMetricPairs(header)

	// Early exit when there is insufficient data for a full report.
	if perfData == nil || perfData.Len() < 2 {
		warnings = append(warnings, "insufficient data for full report")

		sections := []Section{headerSection}
		sections = append(sections, warningsToSection(warnings))

		return Report{
			Title:    header.StrategyName,
			Sections: sections,
		}, nil
	}

	var sections []Section

	sections = append(sections, headerSection)

	// Equity curve.
	equityCurve := buildEquityCurve(perfData, header.InitialCash)
	sections = append(sections, equityCurveToTimeSeries(equityCurve))

	// Recent returns.
	recentReturns := buildRecentReturns(acct, hasBenchmark, &warnings)
	sections = append(sections, returnTableToSection("Recent Returns", recentReturns, hasBenchmark))

	// Returns.
	returns := buildReturns(acct, hasBenchmark, &warnings)
	sections = append(sections, returnTableToSection("Returns", returns, hasBenchmark))

	// Annual returns.
	annualReturns := buildAnnualReturns(acct, hasBenchmark, &warnings)
	sections = append(sections, annualReturnsToSection(annualReturns, hasBenchmark))

	// Risk metrics.
	risk := buildRisk(acct, hasBenchmark, &warnings)
	sections = append(sections, riskToMetricPairs(risk, hasBenchmark))

	// Risk vs benchmark.
	if hasBenchmark {
		riskVsBenchmark := buildRiskVsBenchmark(acct, &warnings)
		sections = append(sections, riskVsBenchmarkToMetricPairs(riskVsBenchmark))
	}

	// Drawdowns.
	drawdowns := buildDrawdowns(acct, &warnings)
	sections = append(sections, drawdownsToSection(drawdowns))

	// Monthly returns.
	monthlyReturns := buildMonthlyReturns(acct, &warnings)
	sections = append(sections, monthlyReturnsToSection(monthlyReturns))

	// Trades.
	trades := buildTrades(acct, &warnings)
	sections = append(sections, tradesToSections(trades)...)

	// Warnings.
	if len(warnings) > 0 {
		sections = append(sections, warningsToSection(warnings))
	}

	return Report{
		Title:    header.StrategyName,
		Sections: sections,
	}, nil
}

// ---------------------------------------------------------------------------
// Header helpers
// ---------------------------------------------------------------------------

// headerData is an internal struct to hold parsed header fields before
// conversion to a MetricPairs section.
type headerData struct {
	StrategyName    string
	StrategyVersion string
	Benchmark       string
	StartDate       time.Time
	EndDate         time.Time
	InitialCash     float64
	FinalValue      float64
	Elapsed         time.Duration
	Steps           int
}

func buildHeaderData(acct ReportablePortfolio) headerData {
	header := headerData{
		StrategyName:    acct.GetMetadata(portfolio.MetaStrategyName),
		StrategyVersion: acct.GetMetadata(portfolio.MetaStrategyVersion),
		Benchmark:       acct.GetMetadata(portfolio.MetaStrategyBenchmark),
		FinalValue:      acct.Value(),
	}

	if cashStr := acct.GetMetadata(portfolio.MetaRunInitialCash); cashStr != "" {
		if parsed, parseErr := strconv.ParseFloat(cashStr, 64); parseErr == nil {
			header.InitialCash = parsed
		}
	}

	if elapsedStr := acct.GetMetadata(portfolio.MetaRunElapsed); elapsedStr != "" {
		if parsed, parseErr := time.ParseDuration(elapsedStr); parseErr == nil {
			header.Elapsed = parsed
		}
	}

	return header
}

func headerToMetricPairs(header headerData) *MetricPairs {
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

// ---------------------------------------------------------------------------
// Equity curve conversion
// ---------------------------------------------------------------------------

func equityCurveToTimeSeries(curve EquityCurve) *TimeSeries {
	series := []NamedSeries{
		{
			Name:   "Strategy",
			Times:  curve.Times,
			Values: curve.StrategyValues,
		},
	}

	if len(curve.BenchmarkValues) > 0 {
		series = append(series, NamedSeries{
			Name:   "Benchmark",
			Times:  curve.Times,
			Values: curve.BenchmarkValues,
		})
	}

	return &TimeSeries{
		SectionName: "Equity Curve",
		Series:      series,
	}
}

// ---------------------------------------------------------------------------
// Return table conversion
// ---------------------------------------------------------------------------

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

	return &Table{
		SectionName: sectionName,
		Columns:     columns,
		Rows:        rows,
	}
}

// ---------------------------------------------------------------------------
// Annual returns conversion
// ---------------------------------------------------------------------------

func annualReturnsToSection(annual AnnualReturns, hasBenchmark bool) *Table {
	columns := []Column{
		{Header: "Year", Format: "string", Align: "left"},
		{Header: "Strategy", Format: "percent", Align: "right"},
	}

	if hasBenchmark && len(annual.Benchmark) > 0 {
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

		if hasBenchmark && len(annual.Benchmark) > 0 {
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

	return &Table{
		SectionName: "Annual Returns",
		Columns:     columns,
		Rows:        rows,
	}
}

// ---------------------------------------------------------------------------
// Risk metrics conversion
// ---------------------------------------------------------------------------

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
		pair := MetricPair{
			Label:  def.label,
			Value:  def.values[0],
			Format: def.metricFmt,
		}

		if hasBenchmark && !math.IsNaN(def.values[1]) {
			benchVal := def.values[1]
			pair.Comparison = &benchVal
		}

		metrics[idx] = pair
	}

	return &MetricPairs{
		SectionName: "Risk Metrics",
		Metrics:     metrics,
	}
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

// ---------------------------------------------------------------------------
// Drawdowns conversion
// ---------------------------------------------------------------------------

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
		if entry.End.Equal(time.Time{}) {
			endStr = "ongoing"
		}

		recoveryStr := "ongoing"
		if !entry.Recovery.IsZero() {
			recoveryStr = entry.Recovery.Format("2006-01-02")
		}

		rows[idx] = []any{
			fmt.Sprintf("%d", idx+1),
			entry.Start,
			endStr,
			recoveryStr,
			entry.Depth,
			fmt.Sprintf("%d days", entry.Days),
		}
	}

	return &Table{
		SectionName: "Top Drawdowns",
		Columns:     columns,
		Rows:        rows,
	}
}

// ---------------------------------------------------------------------------
// Monthly returns conversion
// ---------------------------------------------------------------------------

func monthlyReturnsToSection(monthly MonthlyReturns) *Table {
	monthHeaders := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

	columns := []Column{
		{Header: "Year", Format: "string", Align: "left"},
	}

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

		yearTotal := yearCompound - 1
		row = append(row, yearTotal)

		rows[yearIdx] = row
	}

	return &Table{
		SectionName: "Monthly Returns",
		Columns:     columns,
		Rows:        rows,
	}
}

// ---------------------------------------------------------------------------
// Trades conversion
// ---------------------------------------------------------------------------

func tradesToSections(trades Trades) []Section {
	// Trade summary as MetricPairs.
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

	sections := []Section{
		&MetricPairs{
			SectionName: "Trade Summary",
			Metrics:     summaryMetrics,
		},
	}

	// Recent trades as a Table.
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
			rows = append(rows, []any{
				trade.Date,
				trade.Action,
				trade.Ticker,
				trade.Shares,
				trade.Price,
				trade.Amount,
			})
		}

		sections = append(sections, &Table{
			SectionName: "Recent Trades",
			Columns:     columns,
			Rows:        rows,
		})
	}

	return sections
}

// ---------------------------------------------------------------------------
// Warnings conversion
// ---------------------------------------------------------------------------

func warningsToSection(warnings []string) *Text {
	var builder strings.Builder

	for _, warning := range warnings {
		builder.WriteString("WARNING: ")
		builder.WriteString(warning)
		builder.WriteString("\n")
	}

	return &Text{
		SectionName: "Warnings",
		Body:        builder.String(),
	}
}
