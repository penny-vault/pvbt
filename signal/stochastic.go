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

package signal

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// Output metrics for [StochasticFast].
const (
	// StochasticKSignal is the raw %K line (0-100).
	StochasticKSignal data.Metric = "StochasticK"

	// StochasticDSignal is the 3-period SMA of %K (0-100).
	StochasticDSignal data.Metric = "StochasticD"
)

// Output metrics for [StochasticSlow].
const (
	// StochasticSlowKSignal is the smoothed %K line (0-100).
	StochasticSlowKSignal data.Metric = "StochasticSlowK"

	// StochasticSlowDSignal is the 3-period SMA of Slow %K (0-100).
	StochasticSlowDSignal data.Metric = "StochasticSlowD"
)

// stochasticDPeriod is the universal convention for the %D smoothing period.
const stochasticDPeriod = 3

// StochasticFast computes the Fast Stochastic Oscillator for each asset in
// the universe. %K measures where the close sits relative to the high-low
// range over the period; %D is a 3-period SMA of %K. The function fetches
// period.N + 2 total trading bars (over-fetching calendar days to cover
// weekends and holidays) to compute the %D smoothing. Returns a single-row
// DataFrame with [StochasticKSignal] and [StochasticDSignal].
func StochasticFast(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	// Need period.N + 2 total bars: period.N bars per %K window, sliding
	// 3 times to get 3 %K values for the %D SMA.
	df, kWindow, err := extendedWindow(ctx, assetUniverse, period, stochasticDPeriod-1, data.MetricHigh, data.MetricLow, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticFast: %w", err))
	}

	numRows := df.Len()

	minRows := stochasticDPeriod + 1
	if numRows < minRows {
		return data.WithErr(fmt.Errorf("StochasticFast: need at least %d data points, got %d", minRows, numRows))
	}

	// Clamp the %K window so 3 %K values remain for the %D SMA when the
	// available history is shorter than requested.
	if maxK := numRows - stochasticDPeriod + 1; kWindow > maxK {
		kWindow = maxK
	}

	if kWindow < 1 {
		return data.WithErr(fmt.Errorf("StochasticFast: period must cover at least 1 bar, got %d", kWindow))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	kCols := make([][]float64, len(assets))
	dCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		kValues := stochasticKSeries(highs, lows, closes, kWindow)

		lastK := kValues[len(kValues)-1]

		// %D = SMA of last 3 %K values.
		dSum := 0.0
		allNaN := true

		for jj := len(kValues) - stochasticDPeriod; jj < len(kValues); jj++ {
			if math.IsNaN(kValues[jj]) {
				dSum = math.NaN()
				break
			}

			dSum += kValues[jj]
			allNaN = false
		}

		var lastD float64
		if allNaN || math.IsNaN(dSum) {
			lastD = math.NaN()
		} else {
			lastD = dSum / float64(stochasticDPeriod)
		}

		kCols[ii] = []float64{lastK}
		dCols[ii] = []float64{lastD}
	}

	kDF, err := data.NewDataFrame(lastTime, assets, []data.Metric{StochasticKSignal}, df.Frequency(), kCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticFast: %w", err))
	}

	dDF, err := data.NewDataFrame(lastTime, assets, []data.Metric{StochasticDSignal}, df.Frequency(), dCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticFast: %w", err))
	}

	result, err := data.MergeColumns(kDF, dDF)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticFast: %w", err))
	}

	return result
}

// StochasticSlow computes the Slow Stochastic Oscillator for each asset in
// the universe. Slow %K is an SMA of the raw %K over the smoothing period;
// Slow %D is a 3-period SMA of Slow %K. The function fetches
// period.N + smoothing.N + 1 total trading bars (over-fetching calendar days
// to cover weekends and holidays) to compute the required values. Returns a
// single-row DataFrame with [StochasticSlowKSignal] and [StochasticSlowDSignal].
func StochasticSlow(ctx context.Context, assetUniverse universe.Universe, period, smoothing portfolio.Period) *data.DataFrame {
	// Need enough bars for: period.N per raw %K window, smoothing.N raw %K
	// values for each Slow %K, and 3 Slow %K values for %D.
	warmupBars := smoothing.N + stochasticDPeriod - 2

	df, kWindow, err := extendedWindow(ctx, assetUniverse, period, warmupBars, data.MetricHigh, data.MetricLow, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticSlow: %w", err))
	}

	numRows := df.Len()

	minRows := warmupBars + 2
	if numRows < minRows {
		return data.WithErr(fmt.Errorf("StochasticSlow: need at least %d data points, got %d", minRows, numRows))
	}

	// Clamp the %K window so enough raw %K values remain for the Slow %K
	// smoothing and the %D SMA when history is shorter than requested.
	if maxK := numRows - warmupBars; kWindow > maxK {
		kWindow = maxK
	}

	if kWindow < 1 {
		return data.WithErr(fmt.Errorf("StochasticSlow: period must cover at least 1 bar, got %d", kWindow))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	slowKCols := make([][]float64, len(assets))
	slowDCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		rawK := stochasticKSeries(highs, lows, closes, kWindow)

		// Slow %K = SMA of raw %K over smoothing window.
		numSlowK := len(rawK) - smoothing.N + 1
		slowKValues := make([]float64, numSlowK)

		for jj := range numSlowK {
			sum := 0.0
			hasNaN := false

			for kk := jj; kk < jj+smoothing.N; kk++ {
				if math.IsNaN(rawK[kk]) {
					hasNaN = true
					break
				}

				sum += rawK[kk]
			}

			if hasNaN {
				slowKValues[jj] = math.NaN()
			} else {
				slowKValues[jj] = sum / float64(smoothing.N)
			}
		}

		lastSlowK := slowKValues[len(slowKValues)-1]

		// Slow %D = SMA of last 3 Slow %K values.
		dSum := 0.0
		hasNaN := false

		for jj := len(slowKValues) - stochasticDPeriod; jj < len(slowKValues); jj++ {
			if math.IsNaN(slowKValues[jj]) {
				hasNaN = true
				break
			}

			dSum += slowKValues[jj]
		}

		var lastSlowD float64
		if hasNaN {
			lastSlowD = math.NaN()
		} else {
			lastSlowD = dSum / float64(stochasticDPeriod)
		}

		slowKCols[ii] = []float64{lastSlowK}
		slowDCols[ii] = []float64{lastSlowD}
	}

	kDF, err := data.NewDataFrame(lastTime, assets, []data.Metric{StochasticSlowKSignal}, df.Frequency(), slowKCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticSlow: %w", err))
	}

	dDF, err := data.NewDataFrame(lastTime, assets, []data.Metric{StochasticSlowDSignal}, df.Frequency(), slowDCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticSlow: %w", err))
	}

	result, err := data.MergeColumns(kDF, dDF)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticSlow: %w", err))
	}

	return result
}

// stochasticKSeries computes the rolling %K values for the given window size.
// Returns one %K per rolling window position.
func stochasticKSeries(highs, lows, closes []float64, windowSize int) []float64 {
	numRows := len(highs)
	numK := numRows - windowSize + 1
	kValues := make([]float64, numK)

	for ii := range numK {
		start := ii
		end := ii + windowSize

		highestHigh := math.Inf(-1)
		lowestLow := math.Inf(1)

		for jj := start; jj < end; jj++ {
			if highs[jj] > highestHigh {
				highestHigh = highs[jj]
			}

			if lows[jj] < lowestLow {
				lowestLow = lows[jj]
			}
		}

		rangeHL := highestHigh - lowestLow
		if rangeHL == 0 {
			kValues[ii] = math.NaN()
		} else {
			kValues[ii] = (closes[end-1] - lowestLow) / rangeHL * 100
		}
	}

	return kValues
}
