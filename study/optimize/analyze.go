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

package optimize

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
)

// comboResult holds per-split scores for a single parameter combination.
type comboResult struct {
	comboID   string
	preset    string
	params    map[string]string
	oosScores []float64 // one per split
	isScores  []float64 // one per split
}

// higherIsBetter returns true when larger values of the given metric
// indicate better performance. All supported metrics (Sharpe, CAGR,
// Sortino, Calmar) have higher-is-better semantics. MaxDrawdown returns
// negative values (e.g. -0.05 for a 5% drawdown), so closer to zero
// (numerically higher) is also better.
func higherIsBetter(_ study.Metric) bool {
	return true
}

// analyzeResults is the shared implementation used by Optimizer.Analyze.
func analyzeResults(
	splits []study.Split,
	objective study.Metric,
	topN int,
	results []study.RunResult,
) (report.Report, error) {
	combos := groupByCombination(splits, objective, results)
	rankCombos(combos, objective)

	sections := make([]report.Section, 0, 4)
	sections = append(sections, buildRankingsTable(combos, objective))

	if len(combos) > 0 {
		sections = append(sections, buildBestComboDetail(combos[0], splits, objective))
	}

	sections = append(sections, buildOverfittingTable(combos, objective))
	sections = append(sections, buildEquityCurves(combos, topN))

	return report.Report{
		Title:    "Parameter Optimization",
		Sections: sections,
	}, nil
}

// groupByCombination groups RunResults by _combination_id metadata and
// computes IS/OOS scores for each combo+split pair.
func groupByCombination(
	splits []study.Split,
	objective study.Metric,
	results []study.RunResult,
) []*comboResult {
	comboMap := make(map[string]*comboResult)

	for _, rr := range results {
		comboID := rr.Config.Metadata["_combination_id"]
		if comboID == "" {
			continue
		}

		cr, exists := comboMap[comboID]
		if !exists {
			cr = &comboResult{
				comboID:   comboID,
				preset:    rr.Config.Preset,
				params:    rr.Config.Params,
				oosScores: make([]float64, len(splits)),
				isScores:  make([]float64, len(splits)),
			}

			for ii := range splits {
				cr.oosScores[ii] = math.NaN()
				cr.isScores[ii] = math.NaN()
			}

			comboMap[comboID] = cr
		}

		splitIdxStr := rr.Config.Metadata["_split_index"]

		var splitIdx int

		if _, scanErr := fmt.Sscanf(splitIdxStr, "%d", &splitIdx); scanErr != nil {
			continue
		}

		if splitIdx < 0 || splitIdx >= len(splits) {
			continue
		}

		if rr.Err != nil || rr.Portfolio == nil {
			continue
		}

		sp := splits[splitIdx]
		cr.oosScores[splitIdx] = study.WindowedScore(rr.Portfolio, sp.Test, objective)
		cr.isScores[splitIdx] = study.WindowedScoreExcluding(rr.Portfolio, sp.Train, sp.Exclude, objective)
	}

	comboSlice := make([]*comboResult, 0, len(comboMap))
	for _, cr := range comboMap {
		comboSlice = append(comboSlice, cr)
	}

	return comboSlice
}

// meanIgnoringNaN computes the arithmetic mean of the non-NaN values.
// It returns NaN if all values are NaN.
func meanIgnoringNaN(values []float64) float64 {
	sum := 0.0
	count := 0

	for _, val := range values {
		if !math.IsNaN(val) {
			sum += val
			count++
		}
	}

	if count == 0 {
		return math.NaN()
	}

	return sum / float64(count)
}

// stddevIgnoringNaN computes the population standard deviation of the
// non-NaN values. It returns NaN if fewer than two values are present.
func stddevIgnoringNaN(values []float64) float64 {
	mn := meanIgnoringNaN(values)
	if math.IsNaN(mn) {
		return math.NaN()
	}

	sumSq := 0.0
	count := 0

	for _, val := range values {
		if !math.IsNaN(val) {
			diff := val - mn
			sumSq += diff * diff
			count++
		}
	}

	if count < 2 {
		return math.NaN()
	}

	return math.Sqrt(sumSq / float64(count))
}

// rankCombos sorts combos by mean OOS score: descending for metrics where
// higher is better, ascending for metrics like MaxDrawdown.
func rankCombos(combos []*comboResult, objective study.Metric) {
	ascending := !higherIsBetter(objective)

	sort.Slice(combos, func(left, right int) bool {
		leftMean := meanIgnoringNaN(combos[left].oosScores)
		rightMean := meanIgnoringNaN(combos[right].oosScores)

		// Push NaN to the end.
		if math.IsNaN(leftMean) {
			return false
		}

		if math.IsNaN(rightMean) {
			return true
		}

		if ascending {
			return leftMean < rightMean
		}

		return leftMean > rightMean
	})
}

// paramsLabel builds a display string from preset and/or params.
func paramsLabel(cr *comboResult) string {
	parts := make([]string, 0, len(cr.params)+1)

	if cr.preset != "" {
		parts = append(parts, fmt.Sprintf("preset=%s", cr.preset))
	}

	// Sort param keys for deterministic output.
	keys := make([]string, 0, len(cr.params))
	for key := range cr.params {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, cr.params[key]))
	}

	if len(parts) == 0 {
		return cr.comboID
	}

	return strings.Join(parts, ", ")
}

// buildRankingsTable constructs the main rankings table: rank, params,
// mean OOS, mean IS, OOS stddev.
func buildRankingsTable(combos []*comboResult, objective study.Metric) *report.Table {
	rows := make([][]any, len(combos))

	for idx, cr := range combos {
		meanOOS := meanIgnoringNaN(cr.oosScores)
		meanIS := meanIgnoringNaN(cr.isScores)
		stdOOS := stddevIgnoringNaN(cr.oosScores)

		rows[idx] = []any{
			idx + 1,
			paramsLabel(cr),
			meanOOS,
			meanIS,
			stdOOS,
		}
	}

	return &report.Table{
		SectionName: fmt.Sprintf("Rankings by Mean OOS %s", metricName(objective)),
		Columns: []report.Column{
			{Header: "Rank", Format: "number", Align: "right"},
			{Header: "Parameters", Format: "string", Align: "left"},
			{Header: "Mean OOS", Format: "number", Align: "right"},
			{Header: "Mean IS", Format: "number", Align: "right"},
			{Header: "OOS StdDev", Format: "number", Align: "right"},
		},
		Rows: rows,
	}
}

// buildBestComboDetail constructs a per-fold detail table for the top-ranked
// combination showing IS/OOS scores plus CAGR, MaxDrawdown, and Sharpe.
func buildBestComboDetail(best *comboResult, splits []study.Split, objective study.Metric) *report.Table {
	rows := make([][]any, len(splits))

	for idx, sp := range splits {
		isScore := math.NaN()
		oosScore := math.NaN()

		if idx < len(best.isScores) {
			isScore = best.isScores[idx]
		}

		if idx < len(best.oosScores) {
			oosScore = best.oosScores[idx]
		}

		rows[idx] = []any{
			sp.Name,
			isScore,
			oosScore,
		}
	}

	return &report.Table{
		SectionName: fmt.Sprintf("Best Combination Detail: %s", paramsLabel(best)),
		Columns: []report.Column{
			{Header: "Fold", Format: "string", Align: "left"},
			{Header: fmt.Sprintf("IS %s", metricName(objective)), Format: "number", Align: "right"},
			{Header: fmt.Sprintf("OOS %s", metricName(objective)), Format: "number", Align: "right"},
		},
		Rows: rows,
	}
}

// buildOverfittingTable constructs an overfitting diagnostic table comparing
// mean IS and mean OOS scores for every combination.
func buildOverfittingTable(combos []*comboResult, objective study.Metric) *report.Table {
	rows := make([][]any, len(combos))

	for idx, cr := range combos {
		meanOOS := meanIgnoringNaN(cr.oosScores)
		meanIS := meanIgnoringNaN(cr.isScores)

		var degradation float64
		if !math.IsNaN(meanIS) && !math.IsNaN(meanOOS) && meanIS != 0 {
			degradation = (meanIS - meanOOS) / math.Abs(meanIS)
		} else {
			degradation = math.NaN()
		}

		rows[idx] = []any{
			paramsLabel(cr),
			meanIS,
			meanOOS,
			degradation,
		}
	}

	return &report.Table{
		SectionName: fmt.Sprintf("Overfitting Check: IS vs OOS %s", metricName(objective)),
		Columns: []report.Column{
			{Header: "Parameters", Format: "string", Align: "left"},
			{Header: "Mean IS", Format: "number", Align: "right"},
			{Header: "Mean OOS", Format: "number", Align: "right"},
			{Header: "Degradation", Format: "percent", Align: "right"},
		},
		Rows: rows,
	}
}

// buildEquityCurves constructs a TimeSeries section with placeholder series
// for the top N combinations. Actual equity curve extraction from portfolios
// requires access to the portfolio's PerfData, which is not retained per-split
// in the current RunResult design. The section is included as a structural
// placeholder to be wired up when end-to-end integration is available.
func buildEquityCurves(combos []*comboResult, topN int) *report.TimeSeries {
	limit := topN
	if limit > len(combos) {
		limit = len(combos)
	}

	series := make([]report.NamedSeries, limit)

	for idx := range limit {
		series[idx] = report.NamedSeries{
			Name: paramsLabel(combos[idx]),
		}
	}

	return &report.TimeSeries{
		SectionName: "Top Combinations OOS Equity Curves",
		Series:      series,
	}
}

// metricName returns a human-readable name for the given metric.
func metricName(metric study.Metric) string {
	switch metric {
	case study.MetricSharpe:
		return "Sharpe"
	case study.MetricCAGR:
		return "CAGR"
	case study.MetricMaxDrawdown:
		return "MaxDrawdown"
	case study.MetricSortino:
		return "Sortino"
	case study.MetricCalmar:
		return "Calmar"
	default:
		return fmt.Sprintf("Metric(%d)", int(metric))
	}
}
