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
	"context"
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

// Ensure optimizerReport satisfies the report.Report interface.
var _ report.Report = (*optimizerReport)(nil)

// comboResult holds per-split scores for a single parameter combination.
type comboResult struct {
	comboID      string
	preset       string
	params       map[string]string
	oosScores    []float64 // one per split
	isScores     []float64 // one per split
	equityTimes  []time.Time
	equityValues []float64
}

// analyzeResults is the shared implementation used by Optimizer.Analyze.
func analyzeResults(
	splits []study.Split,
	objective portfolio.Rankable,
	topN int,
	results []study.RunResult,
) (report.Report, error) {
	combos := groupByCombination(splits, objective, results)
	rankCombos(combos, objective)

	// Discard equity data for combos outside the top N to bound memory.
	for idx := topN; idx < len(combos); idx++ {
		combos[idx].equityTimes = nil
		combos[idx].equityValues = nil
	}

	rpt := &optimizerReport{
		ObjectiveName: objective.Name(),
		Rankings:      computeRankings(combos),
		Overfitting:   computeOverfitting(combos),
		EquityCurves:  computeEquityCurves(combos, topN),
		Warning:       degenerateRankingWarning(combos, objective),
	}

	if rpt.Warning != "" {
		log.Warn().
			Str("objective", objective.Name()).
			Int("combos", len(combos)).
			Str("warning", rpt.Warning).
			Msg("optimizer ranking is not meaningful")
	}

	if len(combos) > 0 {
		rpt.BestDetail = computeBestComboDetail(combos[0], splits)
	}

	return rpt, nil
}

// degenerateRankingEpsilon is the absolute tolerance for treating two OOS mean
// scores as equal when checking for an uninformative ranking.
const degenerateRankingEpsilon = 1e-12

// degenerateRankingWarning returns a non-empty warning when the OOS mean
// scores across combos are uninformative (all NaN, or all equal within
// degenerateRankingEpsilon). In that case the apparent ranking reflects
// insertion order rather than the objective metric and is not meaningful.
// Returns an empty string when there are fewer than two combos to compare.
func degenerateRankingWarning(combos []*comboResult, objective portfolio.Rankable) string {
	if len(combos) < 2 {
		return ""
	}

	var (
		minScore   = math.Inf(1)
		maxScore   = math.Inf(-1)
		validCount int
	)

	for _, cr := range combos {
		score := meanIgnoringNaN(cr.oosScores)
		if math.IsNaN(score) {
			continue
		}

		validCount++

		if score < minScore {
			minScore = score
		}

		if score > maxScore {
			maxScore = score
		}
	}

	if validCount == 0 {
		return fmt.Sprintf(
			"every combination produced an undefined %s score; "+
				"the ranking reflects insertion order, not the objective metric. "+
				"Check that the strategy actually trades over the test windows and "+
				"that any required inputs (e.g. risk-free rate) are configured.",
			objective.Name(),
		)
	}

	if maxScore-minScore <= degenerateRankingEpsilon {
		return fmt.Sprintf(
			"every combination produced the same %s score (%.6g); "+
				"the ranking reflects insertion order, not the objective metric. "+
				"This typically means the parameter sweep does not change strategy "+
				"behavior, or the strategy produces identical equity curves across "+
				"all sampled values.",
			objective.Name(), minScore,
		)
	}

	return ""
}

// groupByCombination groups RunResults by _combination_id metadata and
// computes IS/OOS scores for each combo+split pair.
func groupByCombination(
	splits []study.Split,
	objective portfolio.PerformanceMetric,
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

		// Extract equity curve for this split's test window.
		perfData := rr.Portfolio.PerfDataView(context.Background())
		if perfData != nil {
			eqWindow := perfData.Between(sp.Test.Start, sp.Test.End)
			if eqWindow != nil {
				portfolioAsset := asset.Asset{
					CompositeFigi: "_PORTFOLIO_",
					Ticker:        "_PORTFOLIO_",
				}
				eqCol := eqWindow.Column(portfolioAsset, data.PortfolioEquity)
				eqTimes := eqWindow.Times()
				cr.equityTimes = append(cr.equityTimes, eqTimes...)
				cr.equityValues = append(cr.equityValues, eqCol...)
			}
		}
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
func rankCombos(combos []*comboResult, objective portfolio.Rankable) {
	ascending := !objective.HigherIsBetter()

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

// computeRankings builds the rankings slice: rank, params, mean OOS, mean IS, OOS stddev.
func computeRankings(combos []*comboResult) []rankingRow {
	rows := make([]rankingRow, len(combos))

	for idx, cr := range combos {
		rows[idx] = rankingRow{
			Rank:       idx + 1,
			Parameters: paramsLabel(cr),
			MeanOOS:    meanIgnoringNaN(cr.oosScores),
			MeanIS:     meanIgnoringNaN(cr.isScores),
			OOSStdDev:  stddevIgnoringNaN(cr.oosScores),
		}
	}

	return rows
}

// computeBestComboDetail builds a per-fold detail for the top-ranked combination.
func computeBestComboDetail(best *comboResult, splits []study.Split) *bestComboDetail {
	folds := make([]foldDetail, len(splits))

	for idx, sp := range splits {
		isScore := math.NaN()
		oosScore := math.NaN()

		if idx < len(best.isScores) {
			isScore = best.isScores[idx]
		}

		if idx < len(best.oosScores) {
			oosScore = best.oosScores[idx]
		}

		folds[idx] = foldDetail{
			FoldName: sp.Name,
			ISScore:  isScore,
			OOSScore: oosScore,
		}
	}

	return &bestComboDetail{
		Parameters: paramsLabel(best),
		Folds:      folds,
	}
}

// computeOverfitting builds an overfitting diagnostic slice comparing
// mean IS and mean OOS scores for every combination.
func computeOverfitting(combos []*comboResult) []overfittingRow {
	rows := make([]overfittingRow, len(combos))

	for idx, cr := range combos {
		meanOOS := meanIgnoringNaN(cr.oosScores)
		meanIS := meanIgnoringNaN(cr.isScores)

		var degradation float64
		if !math.IsNaN(meanIS) && !math.IsNaN(meanOOS) && meanIS != 0 {
			degradation = (meanIS - meanOOS) / math.Abs(meanIS)
		} else {
			degradation = math.NaN()
		}

		rows[idx] = overfittingRow{
			Parameters:  paramsLabel(cr),
			MeanIS:      meanIS,
			MeanOOS:     meanOOS,
			Degradation: degradation,
		}
	}

	return rows
}

// computeEquityCurves builds equity curve series for the top N
// combinations from their stored equity data.
func computeEquityCurves(combos []*comboResult, topN int) []equityCurveSeries {
	limit := min(topN, len(combos))

	curves := make([]equityCurveSeries, limit)

	for idx := range limit {
		curves[idx] = equityCurveSeries{
			Name:   paramsLabel(combos[idx]),
			Times:  combos[idx].equityTimes,
			Values: combos[idx].equityValues,
		}
	}

	return curves
}
