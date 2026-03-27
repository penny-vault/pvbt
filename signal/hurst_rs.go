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

// HurstRSSignal is the metric name for Hurst exponent (R/S) output.
const HurstRSSignal data.Metric = "HurstRS"

// HurstRS computes the Hurst exponent via Rescaled Range (R/S) analysis for
// each asset in the universe. H < 0.5 indicates mean reversion, H = 0.5
// indicates a random walk, and H > 0.5 indicates trending behavior. Returns a
// single-row DataFrame with [HurstRSSignal].
func HurstRS(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("HurstRS: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("HurstRS: need at least 2 data points, got %d", df.Len()))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	hCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		col := df.Column(aa, data.MetricClose)

		hh, hErr := hurstRS(col)
		if hErr != nil {
			return data.WithErr(fmt.Errorf("HurstRS [%s]: %w", aa.Ticker, hErr))
		}

		hCols[ii] = []float64{hh}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{HurstRSSignal}, df.Frequency(), hCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("HurstRS: %w", err))
	}

	return result
}

// hurstRS computes the Hurst exponent from a price series using R/S analysis.
func hurstRS(prices []float64) (float64, error) {
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

	// Generate sub-period sizes: powers of 2 from 4 up to len(returns)/2.
	var sizes []int

	for ss := 4; ss <= len(returns)/2; ss *= 2 {
		sizes = append(sizes, ss)
	}

	if len(sizes) < 2 {
		return 0, fmt.Errorf("insufficient data for R/S analysis: need at least 20 data points")
	}

	logN := make([]float64, len(sizes))
	logRS := make([]float64, len(sizes))

	for si, ss := range sizes {
		numSegments := len(returns) / ss
		rsSum := 0.0

		for seg := range numSegments {
			start := seg * ss
			segment := returns[start : start+ss]

			// Mean of segment.
			segMean := 0.0
			for _, rr := range segment {
				segMean += rr
			}

			segMean /= float64(ss)

			// Cumulative deviations from mean.
			cumDev := make([]float64, ss)
			cumDev[0] = segment[0] - segMean

			for jj := 1; jj < ss; jj++ {
				cumDev[jj] = cumDev[jj-1] + (segment[jj] - segMean)
			}

			// Range of cumulative deviations.
			maxDev := cumDev[0]
			minDev := cumDev[0]

			for _, dd := range cumDev[1:] {
				if dd > maxDev {
					maxDev = dd
				}

				if dd < minDev {
					minDev = dd
				}
			}

			rangeVal := maxDev - minDev

			// Standard deviation of segment.
			segVarSum := 0.0

			for _, rr := range segment {
				diff := rr - segMean
				segVarSum += diff * diff
			}

			stddev := math.Sqrt(segVarSum / float64(ss))

			if stddev > 0 {
				rsSum += rangeVal / stddev
			}
		}

		logN[si] = math.Log(float64(ss))
		logRS[si] = math.Log(rsSum / float64(numSegments))
	}

	slope, _, regErr := linRegress(logN, logRS)
	if regErr != nil {
		return 0, fmt.Errorf("R/S regression: %w", regErr)
	}

	return slope, nil
}
