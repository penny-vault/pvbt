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
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// PairsResidual computes the z-score of OLS regression residuals between each
// primary asset and each reference asset. For each (primary, reference) pair,
// it regresses primary returns on reference returns, computes the residual
// series, and returns the z-score of the last residual. Positive values mean
// the primary asset is rich relative to the reference; negative means cheap.
//
// The reference universe may contain multiple assets. Each reference asset
// produces its own output metric named PairsResidual_{Ticker}. For example,
// with references SPY and EFA, the output has metrics PairsResidual_SPY and
// PairsResidual_EFA.
//
// Returns a single-row DataFrame with one metric per reference asset.
func PairsResidual(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, referenceUniverse universe.Universe) *data.DataFrame {
	primaryDF, err := assetUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsResidual: primary fetch: %w", err))
	}

	refDF, err := referenceUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsResidual: reference fetch: %w", err))
	}

	numRows := primaryDF.Len()
	if numRows < 3 {
		return data.WithErr(fmt.Errorf("PairsResidual: need at least 3 data points, got %d", numRows))
	}

	primaryAssets := primaryDF.AssetList()
	refAssets := refDF.AssetList()
	times := primaryDF.Times()
	lastTime := []time.Time{times[len(times)-1]}

	// Build return series for reference assets.
	refReturns := make(map[string][]float64, len(refAssets))

	for _, ra := range refAssets {
		prices := refDF.Column(ra, data.MetricClose)
		rets := make([]float64, numRows-1)

		for ii := 1; ii < numRows; ii++ {
			if prices[ii-1] == 0 {
				return data.WithErr(fmt.Errorf("PairsResidual: zero price for reference %s at index %d", ra.Ticker, ii-1))
			}

			rets[ii-1] = (prices[ii] - prices[ii-1]) / prices[ii-1]
		}

		refReturns[ra.Ticker] = rets
	}

	// Build metric names and output columns.
	metricNames := make([]data.Metric, len(refAssets))
	for ri, ra := range refAssets {
		metricNames[ri] = data.Metric("PairsResidual_" + ra.Ticker)
	}

	// Columns are ordered: asset0_ref0, asset0_ref1, ..., asset1_ref0, ...
	allCols := make([][]float64, len(primaryAssets)*len(refAssets))

	for pi, pa := range primaryAssets {
		prices := primaryDF.Column(pa, data.MetricClose)
		primaryRets := make([]float64, numRows-1)

		for ii := 1; ii < numRows; ii++ {
			if prices[ii-1] == 0 {
				return data.WithErr(fmt.Errorf("PairsResidual: zero price for %s at index %d", pa.Ticker, ii-1))
			}

			primaryRets[ii-1] = (prices[ii] - prices[ii-1]) / prices[ii-1]
		}

		for ri, ra := range refAssets {
			rr := refReturns[ra.Ticker]

			// OLS: regress primaryRets on rr.
			slope, intercept, regErr := linRegress(rr, primaryRets)
			if regErr != nil {
				return data.WithErr(fmt.Errorf("PairsResidual [%s vs %s]: %w", pa.Ticker, ra.Ticker, regErr))
			}

			// Compute residuals.
			residuals := make([]float64, len(primaryRets))
			for ii := range primaryRets {
				residuals[ii] = primaryRets[ii] - (intercept + slope*rr[ii])
			}

			// Z-score of residuals.
			zz, zErr := zScore(residuals)
			if zErr != nil {
				return data.WithErr(fmt.Errorf("PairsResidual [%s vs %s]: %w", pa.Ticker, ra.Ticker, zErr))
			}

			allCols[pi*len(refAssets)+ri] = []float64{zz}
		}
	}

	result, err := data.NewDataFrame(lastTime, primaryAssets, metricNames, primaryDF.Frequency(), allCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsResidual: %w", err))
	}

	return result
}
