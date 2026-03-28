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

package stress

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/report"
)

// Ensure stressReport satisfies the report.Report interface.
var _ report.Report = (*stressReport)(nil)

// portfolioAsset is the sentinel asset used to look up equity columns in the
// performance DataFrame. It mirrors the unexported constant in portfolio/account.go.
var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}

// scenarioMetrics holds per-scenario computed values for a single run.
type scenarioMetrics struct {
	scenarioName   string
	maxDrawdown    float64
	totalReturn    float64
	worstDayReturn float64
	hasData        bool
}

// runAnalysis holds the result of analyzing one RunResult across all scenarios.
type runAnalysis struct {
	runName   string
	errMsg    string
	scenarios []scenarioMetrics
}

// analyzeResults builds a stressReport from the results slice. It is the
// shared implementation used by StressTest.Analyze.
func analyzeResults(scenarios []study.Scenario, results []study.RunResult) (report.Report, error) {
	analyses := make([]runAnalysis, len(results))

	for idx, result := range results {
		analyses[idx] = analyzeRun(scenarios, result)
	}

	// Build rankings: one entry per (run, scenario) sorted by max drawdown severity.
	rankings := buildRankings(analyses)

	// Build per-scenario detail sections.
	scenarioDetails := buildScenarioDetails(scenarios, analyses)

	// Build summary text.
	summaryText := buildSummary(scenarios, analyses)

	return &stressReport{
		Rankings:  rankings,
		Scenarios: scenarioDetails,
		Summary:   summaryText,
	}, nil
}

// analyzeRun computes metrics for each scenario window for a single run result.
func analyzeRun(scenarios []study.Scenario, result study.RunResult) runAnalysis {
	analysis := runAnalysis{runName: result.Config.Name}

	if result.Err != nil {
		analysis.errMsg = result.Err.Error()
		analysis.scenarios = make([]scenarioMetrics, len(scenarios))

		for idx, scenario := range scenarios {
			analysis.scenarios[idx] = scenarioMetrics{scenarioName: scenario.Name}
		}

		return analysis
	}

	perfData := result.Portfolio.PerfData()
	analysis.scenarios = make([]scenarioMetrics, len(scenarios))

	for idx, scenario := range scenarios {
		analysis.scenarios[idx] = computeScenarioMetrics(scenario, perfData)
	}

	return analysis
}

// computeScenarioMetrics slices the equity curve to the scenario window and
// derives max drawdown, total return, and worst single-day return.
func computeScenarioMetrics(scenario study.Scenario, perfData *data.DataFrame) scenarioMetrics {
	metrics := scenarioMetrics{scenarioName: scenario.Name}

	if perfData == nil {
		return metrics
	}

	sliced := perfData.Between(scenario.Start, scenario.End)
	if sliced == nil || sliced.Len() == 0 {
		return metrics
	}

	equityValues := sliced.Column(portfolioAsset, data.PortfolioEquity)
	if len(equityValues) == 0 {
		return metrics
	}

	metrics.hasData = true
	metrics.maxDrawdown = computeMaxDrawdown(equityValues)
	metrics.totalReturn = computeTotalReturn(equityValues)
	metrics.worstDayReturn = computeWorstDayReturn(equityValues)

	return metrics
}

// computeMaxDrawdown walks the equity series tracking the running peak and
// returns the maximum percentage decline from peak. The result is negative
// (or zero for a series with no decline).
func computeMaxDrawdown(equityValues []float64) float64 {
	if len(equityValues) == 0 {
		return math.NaN()
	}

	peak := equityValues[0]
	maxDD := 0.0

	for _, value := range equityValues {
		if value > peak {
			peak = value
		}

		if peak > 0 {
			drawdown := (value - peak) / peak
			if drawdown < maxDD {
				maxDD = drawdown
			}
		}
	}

	return maxDD
}

// computeTotalReturn returns (last / first) - 1, or NaN for a single-element
// series or a zero first value.
func computeTotalReturn(equityValues []float64) float64 {
	if len(equityValues) < 2 {
		return math.NaN()
	}

	first := equityValues[0]
	if first == 0 {
		return math.NaN()
	}

	return (equityValues[len(equityValues)-1] / first) - 1
}

// computeWorstDayReturn returns the minimum period-over-period return across
// the equity series. Returns NaN for series with fewer than two values.
func computeWorstDayReturn(equityValues []float64) float64 {
	if len(equityValues) < 2 {
		return math.NaN()
	}

	worst := math.MaxFloat64

	for idx := 1; idx < len(equityValues); idx++ {
		prev := equityValues[idx-1]

		if prev == 0 {
			continue
		}

		dailyReturn := (equityValues[idx] - prev) / prev
		if dailyReturn < worst {
			worst = dailyReturn
		}
	}

	if worst == math.MaxFloat64 {
		return math.NaN()
	}

	return worst
}

// buildRankings constructs a sorted slice of scenarioRanking entries, one per
// (run, scenario) pair, sorted by max drawdown severity (largest drawdown first).
func buildRankings(analyses []runAnalysis) []scenarioRanking {
	var rankings []scenarioRanking

	for _, analysis := range analyses {
		for _, scenarioResult := range analysis.scenarios {
			rankings = append(rankings, scenarioRanking{
				RunName:      analysis.runName,
				ScenarioName: scenarioResult.scenarioName,
				ErrorMsg:     analysis.errMsg,
				MaxDrawdown:  scenarioResult.maxDrawdown,
				TotalReturn:  scenarioResult.totalReturn,
				WorstDay:     scenarioResult.worstDayReturn,
			})
		}
	}

	// Sort by max drawdown ascending (most negative first = worst).
	sort.Slice(rankings, func(left, right int) bool {
		leftDD := rankings[left].MaxDrawdown
		rightDD := rankings[right].MaxDrawdown

		if math.IsNaN(leftDD) {
			return false
		}

		if math.IsNaN(rightDD) {
			return true
		}

		return leftDD < rightDD
	})

	return rankings
}

// buildScenarioDetails constructs a scenarioDetail for each scenario,
// aggregating metrics across all runs.
func buildScenarioDetails(scenarios []study.Scenario, analyses []runAnalysis) []scenarioDetail {
	details := make([]scenarioDetail, len(scenarios))

	for scenarioIdx, scenario := range scenarios {
		dateRange := fmt.Sprintf("%s to %s",
			scenario.Start.Format("2006-01-02"),
			scenario.End.Format("2006-01-02"),
		)

		runMetrics := make([]runMetricSet, 0, len(analyses))

		for _, analysis := range analyses {
			if scenarioIdx >= len(analysis.scenarios) {
				continue
			}

			scenarioResult := analysis.scenarios[scenarioIdx]

			runMetrics = append(runMetrics, runMetricSet{
				RunName:     analysis.runName,
				ErrorMsg:    analysis.errMsg,
				HasData:     scenarioResult.hasData,
				MaxDrawdown: scenarioResult.maxDrawdown,
				TotalReturn: scenarioResult.totalReturn,
				WorstDay:    scenarioResult.worstDayReturn,
			})
		}

		details[scenarioIdx] = scenarioDetail{
			Name:       scenario.Name,
			DateRange:  dateRange,
			RunMetrics: runMetrics,
		}
	}

	return details
}

// buildSummary produces a brief narrative string summarizing the stress test results.
func buildSummary(scenarios []study.Scenario, analyses []runAnalysis) string {
	failedRuns := 0

	for _, analysis := range analyses {
		if analysis.errMsg != "" {
			failedRuns++
		}
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "Stress test completed: %d scenario(s) evaluated", len(scenarios))

	if len(analyses) > 0 {
		fmt.Fprintf(&sb, " across %d run(s)", len(analyses))
	}

	fmt.Fprintf(&sb, ".\n")

	if failedRuns > 0 {
		fmt.Fprintf(&sb, "%d run(s) failed and are excluded from metric calculations.\n", failedRuns)
	}

	scenarioNames := make([]string, len(scenarios))
	for idx, scenario := range scenarios {
		scenarioNames[idx] = scenario.Name
	}

	fmt.Fprintf(&sb, "Scenarios covered: %s.\n", strings.Join(scenarioNames, ", "))

	return sb.String()
}
