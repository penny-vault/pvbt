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
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/report"
	"github.com/rs/zerolog/log"
)

// Ensure monteCarloReport satisfies the report.Report interface.
var _ report.Report = (*monteCarloReport)(nil)

// portfolioAsset is the sentinel asset used to look up equity columns in the
// performance DataFrame. It mirrors the unexported constant in portfolio/account.go.
var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}

// analyzeResults computes the Monte Carlo report from the collected simulation
// results. It filters failed runs, extracts equity curves and summary metrics,
// then builds a monteCarloReport with fan chart, terminal wealth, confidence
// intervals, probability of ruin, optional historical rank, and a summary narrative.
func analyzeResults(results []study.RunResult, historicalResult report.ReportablePortfolio, ruinThreshold float64) (report.Report, error) {
	// Step 1: Filter successful results.
	var successful []study.RunResult

	for _, result := range results {
		if result.Err == nil && result.Portfolio != nil {
			successful = append(successful, result)
		}
	}

	if len(successful) == 0 {
		return &monteCarloReport{
			Summary: "No successful simulation paths to analyze.",
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
			// ErrNoRiskFreeRate and ErrNoBenchmark are configuration gaps, not data
			// errors. Summary() still populates the non-Sharpe fields, so we accept
			// the partial result and log a warning rather than aborting the analysis.
			if errors.Is(err, portfolio.ErrNoRiskFreeRate) || errors.Is(err, portfolio.ErrNoBenchmark) {
				log.Warn().Err(err).Int("path", idx).Msg("monte carlo: partial summary (risk-free rate or benchmark not configured)")
			} else {
				return nil, fmt.Errorf("computing summary for path %d: %w", idx, err)
			}
		}

		summaries[idx] = summary
	}

	// Use the first successful result's perf data for the time axis.
	times := successful[0].Portfolio.PerfData().Times()

	// Step 3: Build report.
	rpt := &monteCarloReport{}

	// Fan chart data.
	rpt.FanChart = computeFanChart(times, equityCurves, historicalResult)

	// Terminal wealth distribution.
	rpt.TerminalWealth = computeTerminalWealth(equityCurves, historicalResult)

	// Confidence intervals on key metrics.
	rpt.ConfidenceIntervals = computeConfidenceIntervals(summaries)

	// Probability of ruin.
	rpt.Ruin = computeRuin(summaries, ruinThreshold)

	// Historical rank (only if historical result is present).
	if historicalResult != nil {
		histRank, err := computeHistoricalRank(equityCurves, summaries, historicalResult)
		if err != nil {
			return nil, fmt.Errorf("computing historical rank: %w", err)
		}

		rpt.HistoricalRank = histRank
	}

	// Summary narrative.
	rpt.Summary = computeMCSummary(len(results), len(successful), summaries, ruinThreshold)

	return rpt, nil
}

// computeFanChart builds a fanChartData with P5/P25/P50/P75/P95 percentile
// bands across all simulation paths' equity curves.
func computeFanChart(times []time.Time, equityCurves [][]float64, historicalResult report.ReportablePortfolio) fanChartData {
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

	fc := fanChartData{
		Times: times,
		P5:    p5Values,
		P25:   p25Values,
		P50:   p50Values,
		P75:   p75Values,
		P95:   p95Values,
	}

	if historicalResult != nil {
		histPerfData := historicalResult.PerfData()
		if histPerfData != nil {
			histTimes := histPerfData.Times()
			histValues := histPerfData.Column(portfolioAsset, data.PortfolioEquity)

			if len(histValues) > 0 {
				fc.Actual = &actualSeries{
					Times:  histTimes,
					Values: histValues,
				}
			}
		}
	}

	return fc
}

// computeTerminalWealth collects the final equity value from each path and
// produces percentiles plus mean and standard deviation as terminalStat entries.
func computeTerminalWealth(equityCurves [][]float64, historicalResult report.ReportablePortfolio) []terminalStat {
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

	stats := []terminalStat{
		{Label: "P1", Value: percentile(sorted, 0.01)},
		{Label: "P5", Value: percentile(sorted, 0.05)},
		{Label: "P10", Value: percentile(sorted, 0.10)},
		{Label: "P25", Value: percentile(sorted, 0.25)},
		{Label: "P50", Value: percentile(sorted, 0.50)},
		{Label: "P75", Value: percentile(sorted, 0.75)},
		{Label: "P90", Value: percentile(sorted, 0.90)},
		{Label: "P95", Value: percentile(sorted, 0.95)},
		{Label: "P99", Value: percentile(sorted, 0.99)},
		{Label: "Mean", Value: mean},
		{Label: "Std Dev", Value: stddev},
	}

	if historicalResult != nil {
		histValue := historicalResult.Value()
		rank := percentileRank(sorted, histValue)

		stats = append(stats, terminalStat{
			Label: fmt.Sprintf("Historical (P%.0f)", rank*100),
			Value: histValue,
		})
	}

	return stats
}

// computeConfidenceIntervals computes P5/P25/P50/P75/P95 for TWRR, MaxDrawdown, and Sharpe.
func computeConfidenceIntervals(summaries []portfolio.Summary) []confidenceRow {
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

	type metricInput struct {
		name   string
		sorted []float64
	}

	metrics := []metricInput{
		{"TWRR", twrrValues},
		{"Max Drawdown", ddValues},
		{"Sharpe", sharpeValues},
	}

	rows := make([]confidenceRow, len(metrics))
	for idx, metric := range metrics {
		rows[idx] = confidenceRow{
			Metric: metric.name,
			P5:     percentile(metric.sorted, 0.05),
			P25:    percentile(metric.sorted, 0.25),
			P50:    percentile(metric.sorted, 0.50),
			P75:    percentile(metric.sorted, 0.75),
			P95:    percentile(metric.sorted, 0.95),
		}
	}

	return rows
}

// computeRuin computes the probability of ruin -- the fraction of paths
// whose max drawdown exceeded the ruin threshold.
func computeRuin(summaries []portfolio.Summary, ruinThreshold float64) ruinData {
	if len(summaries) == 0 {
		return ruinData{Threshold: ruinThreshold}
	}

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

	return ruinData{
		Probability:    ruinPct,
		Threshold:      ruinThreshold,
		MedianDrawdown: medianDD,
	}
}

// computeHistoricalRank computes where the historical result's metrics rank
// among the simulated paths.
func computeHistoricalRank(equityCurves [][]float64, summaries []portfolio.Summary, historicalResult report.ReportablePortfolio) (*historicalRankData, error) {
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

	return &historicalRankData{
		TerminalValuePercentile: tvRank,
		TWRRPercentile:          twrrRank,
		MaxDrawdownPercentile:   ddRank,
		SharpePercentile:        sharpeRank,
	}, nil
}

// computeMCSummary produces a brief narrative string summarizing the simulation.
func computeMCSummary(totalRuns int, successfulRuns int, summaries []portfolio.Summary, ruinThreshold float64) string {
	if len(summaries) == 0 {
		return "No successful simulation paths to analyze."
	}

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

	return sb.String()
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
