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

// HurstDFASignal is the metric name for Hurst exponent (DFA) output.
const HurstDFASignal data.Metric = "HurstDFA"

// HurstDFA computes the Hurst exponent via Detrended Fluctuation Analysis for
// each asset in the universe. Interpretation is the same as [HurstRS]: H < 0.5
// indicates mean reversion, H = 0.5 indicates a random walk, and H > 0.5
// indicates trending behavior. DFA is more robust to short-term correlations
// than R/S analysis. Returns a single-row DataFrame with [HurstDFASignal].
func HurstDFA(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("HurstDFA: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("HurstDFA: need at least 2 data points, got %d", df.Len()))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	hCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		col := df.Column(aa, data.MetricClose)

		hh, hErr := hurstDFA(col)
		if hErr != nil {
			return data.WithErr(fmt.Errorf("HurstDFA [%s]: %w", aa.Ticker, hErr))
		}

		hCols[ii] = []float64{hh}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{HurstDFASignal}, df.Frequency(), hCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("HurstDFA: %w", err))
	}

	return result
}

// hurstDFA computes the Hurst exponent from a price series using DFA.
func hurstDFA(prices []float64) (float64, error) {
	nn := len(prices)
	if nn < 2 {
		return 0, fmt.Errorf("need at least 2 prices, got %d", nn)
	}

	// Compute returns.
	returns := make([]float64, nn-1)
	for ii := 1; ii < nn; ii++ {
		if prices[ii-1] == 0 {
			return 0, fmt.Errorf("zero price at index %d", ii-1)
		}

		returns[ii-1] = (prices[ii] - prices[ii-1]) / prices[ii-1]
	}

	// Build cumulative deviation profile (integrate the mean-subtracted returns).
	retMean := 0.0
	for _, rr := range returns {
		retMean += rr
	}

	retMean /= float64(len(returns))

	profile := make([]float64, len(returns))
	profile[0] = returns[0] - retMean

	for ii := 1; ii < len(returns); ii++ {
		profile[ii] = profile[ii-1] + (returns[ii] - retMean)
	}

	// Generate window sizes: powers of 2 from 4 up to len(profile)/2.
	var sizes []int

	for ss := 4; ss <= len(profile)/2; ss *= 2 {
		sizes = append(sizes, ss)
	}

	if len(sizes) < 2 {
		return 0, fmt.Errorf("insufficient data for DFA: need at least 20 data points")
	}

	logN := make([]float64, len(sizes))
	logF := make([]float64, len(sizes))

	for si, ss := range sizes {
		numSegments := len(profile) / ss
		fluctSum := 0.0

		for seg := range numSegments {
			start := seg * ss
			segment := profile[start : start+ss]

			// Fit linear trend to the segment via least-squares.
			xx := make([]float64, ss)
			for jj := range ss {
				xx[jj] = float64(jj)
			}

			slope, intercept, regErr := linRegress(xx, segment)
			if regErr != nil {
				return 0, fmt.Errorf("DFA trend fit: %w", regErr)
			}

			// Compute RMS of detrended values.
			rmsSum := 0.0

			for jj := range ss {
				detrended := segment[jj] - (intercept + slope*float64(jj))
				rmsSum += detrended * detrended
			}

			fluctSum += math.Sqrt(rmsSum / float64(ss))
		}

		logN[si] = math.Log(float64(ss))
		logF[si] = math.Log(fluctSum / float64(numSegments))
	}

	slope, _, regErr := linRegress(logN, logF)
	if regErr != nil {
		return 0, fmt.Errorf("DFA regression: %w", regErr)
	}

	return slope, nil
}
