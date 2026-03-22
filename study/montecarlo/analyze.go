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

package montecarlo

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
)

// portfolioAsset is the sentinel asset used to look up equity columns in the
// performance DataFrame. It mirrors the unexported constant in portfolio/account.go.
var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}

// analyzeResults computes the Monte Carlo report from the collected simulation
// results. It filters failed runs, extracts equity curves and summary metrics,
// then builds a report with fan chart, terminal wealth, confidence intervals,
// probability of ruin, optional historical rank, and a summary narrative.
func analyzeResults(results []study.RunResult, historicalResult report.ReportablePortfolio, ruinThreshold float64) (report.Report, error) {
	// Step 1: Filter successful results.
	var successful []study.RunResult

	for _, result := range results {
		if result.Err == nil && result.Portfolio != nil {
			successful = append(successful, result)
		}
	}

	if len(successful) == 0 {
		return report.Report{
			Title: "Monte Carlo Simulation",
			Sections: []report.Section{
				&report.Text{
					SectionName: "Summary",
					Body:        "No successful simulation paths to analyze.",
				},
			},
		}, nil
	}

	// Step 2: Extract equity curves and summaries.
	equityCurves := make([][]float64, len(successful))
	summaries := make([]portfolio.Summary, len(successful))

	for idx, result := range successful {
		perfData := result.Portfolio.PerfData()
		equityCurves[idx] = perfData.Column(portfolioAsset, data.PortfolioEquity)

		summary, err := result.Portfolio.Summary()
		if err != nil {
			return report.Report{}, fmt.Errorf("computing summary for path %d: %w", idx, err)
		}

		summaries[idx] = summary
	}

	// Use the first successful result's perf data for the time axis.
	times := successful[0].Portfolio.PerfData().Times()

	// Step 3: Build report sections.
	sections := make([]report.Section, 0, 6)

	// Section 1: Fan Chart.
	fanChart := buildFanChart(times, equityCurves, historicalResult)
	sections = append(sections, fanChart)

	// Section 2: Terminal Wealth Distribution.
	terminalWealth := buildTerminalWealthTable(equityCurves, historicalResult)
	sections = append(sections, terminalWealth)

	// Section 3: Confidence Intervals on Key Metrics.
	confidenceIntervals := buildConfidenceTable(summaries)
	sections = append(sections, confidenceIntervals)

	// Section 4: Probability of Ruin.
	ruinSection := buildRuinSection(summaries, ruinThreshold)
	sections = append(sections, ruinSection)

	// Section 5: Historical Rank (only if historical result is present).
	if historicalResult != nil {
		historicalRank, err := buildHistoricalRank(equityCurves, summaries, historicalResult)
		if err != nil {
			return report.Report{}, fmt.Errorf("computing historical rank: %w", err)
		}

		sections = append(sections, historicalRank)
	}

	// Section 6: Summary Narrative.
	summaryText := buildMCSummaryText(len(results), len(successful), summaries, ruinThreshold)
	sections = append(sections, summaryText)

	return report.Report{
		Title:    "Monte Carlo Simulation",
		Sections: sections,
	}, nil
}

// buildFanChart constructs a TimeSeries section with P5/P25/P50/P75/P95
// percentile bands across all simulation paths' equity curves.
func buildFanChart(times []time.Time, equityCurves [][]float64, historicalResult report.ReportablePortfolio) *report.TimeSeries {
	numSteps := len(times)
	p5Values := make([]float64, numSteps)
	p25Values := make([]float64, numSteps)
	p50Values := make([]float64, numSteps)
	p75Values := make([]float64, numSteps)
	p95Values := make([]float64, numSteps)

	for step := range numSteps {
		valuesAtStep := make([]float64, 0, len(equityCurves))

		for _, curve := range equityCurves {
			if step < len(curve) {
				valuesAtStep = append(valuesAtStep, curve[step])
			}
		}

		sort.Float64s(valuesAtStep)

		p5Values[step] = percentile(valuesAtStep, 0.05)
		p25Values[step] = percentile(valuesAtStep, 0.25)
		p50Values[step] = percentile(valuesAtStep, 0.50)
		p75Values[step] = percentile(valuesAtStep, 0.75)
		p95Values[step] = percentile(valuesAtStep, 0.95)
	}

	series := []report.NamedSeries{
		{Name: "P5", Times: times, Values: p5Values},
		{Name: "P25", Times: times, Values: p25Values},
		{Name: "P50", Times: times, Values: p50Values},
		{Name: "P75", Times: times, Values: p75Values},
		{Name: "P95", Times: times, Values: p95Values},
	}

	if historicalResult != nil {
		histPerfData := historicalResult.PerfData()
		if histPerfData != nil {
			histTimes := histPerfData.Times()
			histValues := histPerfData.Column(portfolioAsset, data.PortfolioEquity)

			if len(histValues) > 0 {
				series = append(series, report.NamedSeries{
					Name:   "Historical",
					Times:  histTimes,
					Values: histValues,
				})
			}
		}
	}

	return &report.TimeSeries{
		SectionName: "Equity Curve Distribution",
		Series:      series,
	}
}

// buildTerminalWealthTable collects the final equity value from each path and
// presents percentiles plus mean and standard deviation.
func buildTerminalWealthTable(equityCurves [][]float64, historicalResult report.ReportablePortfolio) *report.Table {
	terminalValues := make([]float64, 0, len(equityCurves))

	for _, curve := range equityCurves {
		if len(curve) > 0 {
			terminalValues = append(terminalValues, curve[len(curve)-1])
		}
	}

	sorted := make([]float64, len(terminalValues))
	copy(sorted, terminalValues)
	sort.Float64s(sorted)

	mean := computeMean(terminalValues)
	stddev := computeStdDev(terminalValues, mean)

	type statRow struct {
		label string
		value float64
	}

	stats := []statRow{
		{"P1", percentile(sorted, 0.01)},
		{"P5", percentile(sorted, 0.05)},
		{"P10", percentile(sorted, 0.10)},
		{"P25", percentile(sorted, 0.25)},
		{"P50", percentile(sorted, 0.50)},
		{"P75", percentile(sorted, 0.75)},
		{"P90", percentile(sorted, 0.90)},
		{"P95", percentile(sorted, 0.95)},
		{"P99", percentile(sorted, 0.99)},
		{"Mean", mean},
		{"Std Dev", stddev},
	}

	if historicalResult != nil {
		histValue := historicalResult.Value()
		rank := percentileRank(sorted, histValue)

		stats = append(stats, statRow{
			label: fmt.Sprintf("Historical (P%.0f)", rank*100),
			value: histValue,
		})
	}

	rows := make([][]any, len(stats))
	for idx, stat := range stats {
		rows[idx] = []any{stat.label, stat.value}
	}

	return &report.Table{
		SectionName: "Terminal Wealth Distribution",
		Columns: []report.Column{
			{Header: "Statistic", Format: "string", Align: "left"},
			{Header: "Value", Format: "currency", Align: "right"},
		},
		Rows: rows,
	}
}

// buildConfidenceTable shows P5/P25/P50/P75/P95 for TWRR, MaxDrawdown, and Sharpe.
func buildConfidenceTable(summaries []portfolio.Summary) *report.Table {
	twrrValues := make([]float64, len(summaries))
	ddValues := make([]float64, len(summaries))
	sharpeValues := make([]float64, len(summaries))

	for idx, summary := range summaries {
		twrrValues[idx] = summary.TWRR
		ddValues[idx] = summary.MaxDrawdown
		sharpeValues[idx] = summary.Sharpe
	}

	sort.Float64s(twrrValues)
	sort.Float64s(ddValues)
	sort.Float64s(sharpeValues)

	type metricRow struct {
		name   string
		sorted []float64
	}

	metrics := []metricRow{
		{"TWRR", twrrValues},
		{"Max Drawdown", ddValues},
		{"Sharpe", sharpeValues},
	}

	rows := make([][]any, len(metrics))
	for idx, metric := range metrics {
		rows[idx] = []any{
			metric.name,
			percentile(metric.sorted, 0.05),
			percentile(metric.sorted, 0.25),
			percentile(metric.sorted, 0.50),
			percentile(metric.sorted, 0.75),
			percentile(metric.sorted, 0.95),
		}
	}

	return &report.Table{
		SectionName: "Confidence Intervals",
		Columns: []report.Column{
			{Header: "Metric", Format: "string", Align: "left"},
			{Header: "P5", Format: "number", Align: "right"},
			{Header: "P25", Format: "number", Align: "right"},
			{Header: "P50", Format: "number", Align: "right"},
			{Header: "P75", Format: "number", Align: "right"},
			{Header: "P95", Format: "number", Align: "right"},
		},
		Rows: rows,
	}
}

// buildRuinSection computes the probability of ruin -- the fraction of paths
// whose max drawdown exceeded the ruin threshold.
func buildRuinSection(summaries []portfolio.Summary, ruinThreshold float64) *report.MetricPairs {
	ruinCount := 0
	ddValues := make([]float64, len(summaries))

	for idx, summary := range summaries {
		ddValues[idx] = summary.MaxDrawdown

		if summary.MaxDrawdown < ruinThreshold {
			ruinCount++
		}
	}

	ruinPct := float64(ruinCount) / float64(len(summaries))

	sort.Float64s(ddValues)
	medianDD := percentile(ddValues, 0.50)

	return &report.MetricPairs{
		SectionName: "Probability of Ruin",
		Metrics: []report.MetricPair{
			{Label: "Probability of Ruin", Value: ruinPct, Format: "percent"},
			{Label: "Ruin Threshold", Value: ruinThreshold, Format: "percent"},
			{Label: "Median Max Drawdown", Value: medianDD, Format: "percent"},
		},
	}
}

// buildHistoricalRank computes where the historical result's metrics rank
// among the simulated paths.
func buildHistoricalRank(equityCurves [][]float64, summaries []portfolio.Summary, historicalResult report.ReportablePortfolio) (*report.MetricPairs, error) {
	histSummary, err := historicalResult.Summary()
	if err != nil {
		return nil, fmt.Errorf("computing historical summary: %w", err)
	}

	// Terminal value rank.
	terminalValues := make([]float64, 0, len(equityCurves))
	for _, curve := range equityCurves {
		if len(curve) > 0 {
			terminalValues = append(terminalValues, curve[len(curve)-1])
		}
	}

	sort.Float64s(terminalValues)
	tvRank := percentileRank(terminalValues, historicalResult.Value())

	// TWRR rank.
	twrrValues := make([]float64, len(summaries))
	for idx, summary := range summaries {
		twrrValues[idx] = summary.TWRR
	}

	sort.Float64s(twrrValues)
	twrrRank := percentileRank(twrrValues, histSummary.TWRR)

	// Max drawdown rank.
	ddValues := make([]float64, len(summaries))
	for idx, summary := range summaries {
		ddValues[idx] = summary.MaxDrawdown
	}

	sort.Float64s(ddValues)
	ddRank := percentileRank(ddValues, histSummary.MaxDrawdown)

	// Sharpe rank.
	sharpeValues := make([]float64, len(summaries))
	for idx, summary := range summaries {
		sharpeValues[idx] = summary.Sharpe
	}

	sort.Float64s(sharpeValues)
	sharpeRank := percentileRank(sharpeValues, histSummary.Sharpe)

	return &report.MetricPairs{
		SectionName: "Historical Rank",
		Metrics: []report.MetricPair{
			{Label: "Terminal Value Percentile", Value: tvRank, Format: "percent"},
			{Label: "TWRR Percentile", Value: twrrRank, Format: "percent"},
			{Label: "Max Drawdown Percentile", Value: ddRank, Format: "percent"},
			{Label: "Sharpe Percentile", Value: sharpeRank, Format: "percent"},
		},
	}, nil
}

// buildMCSummaryText produces a brief narrative summarizing the simulation.
func buildMCSummaryText(totalRuns int, successfulRuns int, summaries []portfolio.Summary, ruinThreshold float64) *report.Text {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Monte Carlo simulation completed: %d of %d paths succeeded.\n", successfulRuns, totalRuns)

	failedRuns := totalRuns - successfulRuns
	if failedRuns > 0 {
		fmt.Fprintf(&sb, "%d path(s) failed and are excluded from the analysis.\n", failedRuns)
	}

	// Compute median TWRR for the narrative.
	twrrValues := make([]float64, len(summaries))
	for idx, summary := range summaries {
		twrrValues[idx] = summary.TWRR
	}

	sort.Float64s(twrrValues)
	medianTWRR := percentile(twrrValues, 0.50)

	fmt.Fprintf(&sb, "Median TWRR across paths: %.2f%%.\n", medianTWRR*100)

	// Ruin probability.
	ruinCount := 0

	for _, summary := range summaries {
		if summary.MaxDrawdown < ruinThreshold {
			ruinCount++
		}
	}

	ruinPct := float64(ruinCount) / float64(len(summaries)) * 100
	fmt.Fprintf(&sb, "Probability of ruin (drawdown beyond %.0f%%): %.1f%%.\n", ruinThreshold*100, ruinPct)

	return &report.Text{
		SectionName: "Summary",
		Body:        sb.String(),
	}
}

// percentile returns the value at the given percentile (0-1) from a sorted slice.
// It uses linear interpolation between adjacent ranks.
func percentile(sorted []float64, pct float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}

	if len(sorted) == 1 {
		return sorted[0]
	}

	// Clamp percentile to [0, 1].
	if pct <= 0 {
		return sorted[0]
	}

	if pct >= 1 {
		return sorted[len(sorted)-1]
	}

	// Use the "exclusive" percentile method (similar to Excel PERCENTILE.EXC).
	rank := pct * float64(len(sorted)+1)
	lowerIdx := int(math.Floor(rank)) - 1
	fraction := rank - math.Floor(rank)

	if lowerIdx < 0 {
		return sorted[0]
	}

	if lowerIdx >= len(sorted)-1 {
		return sorted[len(sorted)-1]
	}

	return sorted[lowerIdx] + fraction*(sorted[lowerIdx+1]-sorted[lowerIdx])
}

// percentileRank returns what percentile (0-1) a value falls at in a sorted slice.
func percentileRank(sorted []float64, value float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}

	below := 0

	for _, val := range sorted {
		if val < value {
			below++
		}
	}

	return float64(below) / float64(len(sorted))
}

// computeMean returns the arithmetic mean of a slice of float64 values.
func computeMean(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}

	total := 0.0
	for _, val := range values {
		total += val
	}

	return total / float64(len(values))
}

// computeStdDev returns the population standard deviation given a precomputed mean.
func computeStdDev(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}

	sumSquares := 0.0

	for _, val := range values {
		diff := val - mean
		sumSquares += diff * diff
	}

	return math.Sqrt(sumSquares / float64(len(values)))
}
