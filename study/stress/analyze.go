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

// analyzeResults builds a report.Report from the results slice. It is the
// shared implementation used by StressTest.Analyze.
func analyzeResults(scenarios []study.Scenario, results []study.RunResult) (report.Report, error) {
	analyses := make([]runAnalysis, len(results))

	for idx, result := range results {
		analyses[idx] = analyzeRun(scenarios, result)
	}

	sections := make([]report.Section, 0, 2+len(scenarios)+1)

	// Ranking table: one row per (run, scenario) sorted by max drawdown severity.
	rankingTable := buildRankingTable(analyses)
	sections = append(sections, rankingTable)

	// Per-scenario MetricPairs sections.
	for scenarioIdx, scenario := range scenarios {
		metricsSection := buildScenarioMetricPairs(scenario, scenarioIdx, analyses)
		sections = append(sections, metricsSection)
	}

	// Summary text.
	summaryText := buildSummaryText(scenarios, analyses)
	sections = append(sections, summaryText)

	return report.Report{
		Title:    "Stress Test Analysis",
		Sections: sections,
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

// buildRankingTable constructs a Table section ranking all (run, scenario)
// pairs by max drawdown severity (largest drawdown first).
func buildRankingTable(analyses []runAnalysis) *report.Table {
	type rankRow struct {
		runName      string
		scenarioName string
		maxDrawdown  float64
		totalReturn  float64
		worstDay     float64
		errorMsg     string
	}

	var rows []rankRow

	for _, analysis := range analyses {
		for _, scenarioResult := range analysis.scenarios {
			rows = append(rows, rankRow{
				runName:      analysis.runName,
				scenarioName: scenarioResult.scenarioName,
				maxDrawdown:  scenarioResult.maxDrawdown,
				totalReturn:  scenarioResult.totalReturn,
				worstDay:     scenarioResult.worstDayReturn,
				errorMsg:     analysis.errMsg,
			})
		}
	}

	// Sort by max drawdown ascending (most negative first = worst).
	sort.Slice(rows, func(left, right int) bool {
		leftDD := rows[left].maxDrawdown
		rightDD := rows[right].maxDrawdown

		if math.IsNaN(leftDD) {
			return false
		}

		if math.IsNaN(rightDD) {
			return true
		}

		return leftDD < rightDD
	})

	tableRows := make([][]any, len(rows))

	for rowIdx, row := range rows {
		if row.errorMsg != "" {
			tableRows[rowIdx] = []any{
				row.runName,
				row.scenarioName,
				row.errorMsg,
				"",
				"",
				"",
			}

			continue
		}

		tableRows[rowIdx] = []any{
			row.runName,
			row.scenarioName,
			"",
			row.maxDrawdown,
			row.totalReturn,
			row.worstDay,
		}
	}

	return &report.Table{
		SectionName: "Scenario Rankings by Max Drawdown",
		Columns: []report.Column{
			{Header: "Run", Format: "string", Align: "left"},
			{Header: "Scenario", Format: "string", Align: "left"},
			{Header: "Error", Format: "string", Align: "left"},
			{Header: "Max Drawdown", Format: "percent", Align: "right"},
			{Header: "Total Return", Format: "percent", Align: "right"},
			{Header: "Worst Day", Format: "percent", Align: "right"},
		},
		Rows: tableRows,
	}
}

// buildScenarioMetricPairs constructs a MetricPairs section for a single
// scenario, aggregating metrics across all runs.
func buildScenarioMetricPairs(scenario study.Scenario, scenarioIdx int, analyses []runAnalysis) *report.MetricPairs {
	pairs := make([]report.MetricPair, 0, len(analyses)*3)

	for _, analysis := range analyses {
		if scenarioIdx >= len(analysis.scenarios) {
			continue
		}

		scenarioResult := analysis.scenarios[scenarioIdx]
		prefix := analysis.runName

		if analysis.errMsg != "" {
			pairs = append(pairs, report.MetricPair{
				Label:  fmt.Sprintf("%s: Error", prefix),
				Value:  0,
				Format: fmt.Sprintf("label:%s", analysis.errMsg),
			})

			continue
		}

		if !scenarioResult.hasData {
			pairs = append(pairs, report.MetricPair{
				Label:  fmt.Sprintf("%s: Max Drawdown", prefix),
				Value:  0,
				Format: "label:N/A",
			})

			continue
		}

		pairs = append(pairs,
			report.MetricPair{
				Label:  fmt.Sprintf("%s: Max Drawdown", prefix),
				Value:  scenarioResult.maxDrawdown,
				Format: "percent",
			},
			report.MetricPair{
				Label:  fmt.Sprintf("%s: Total Return", prefix),
				Value:  scenarioResult.totalReturn,
				Format: "percent",
			},
			report.MetricPair{
				Label:  fmt.Sprintf("%s: Worst Day", prefix),
				Value:  scenarioResult.worstDayReturn,
				Format: "percent",
			},
		)
	}

	sectionName := fmt.Sprintf("%s (%s to %s)",
		scenario.Name,
		scenario.Start.Format("2006-01-02"),
		scenario.End.Format("2006-01-02"),
	)

	return &report.MetricPairs{
		SectionName: sectionName,
		Metrics:     pairs,
	}
}

// buildSummaryText produces a brief narrative text section summarizing the
// stress test results.
func buildSummaryText(scenarios []study.Scenario, analyses []runAnalysis) *report.Text {
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

	return &report.Text{
		SectionName: "Summary",
		Body:        sb.String(),
	}
}
