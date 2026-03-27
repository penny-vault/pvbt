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

// PairsRatio computes the z-score of the price ratio between each primary asset
// and each reference asset. For each (primary, reference) pair, it computes
// ratio = primary_price / reference_price at each bar, then returns the z-score
// of the ratio series. Positive values mean the ratio is above its rolling
// mean (primary is rich); negative means below (primary is cheap).
//
// The reference universe may contain multiple assets. Each reference asset
// produces its own output metric named PairsRatio_{Ticker}. For example,
// with references SPY and EFA, the output has metrics PairsRatio_SPY and
// PairsRatio_EFA.
//
// Returns a single-row DataFrame with one metric per reference asset.
func PairsRatio(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, referenceUniverse universe.Universe) *data.DataFrame {
	primaryDF, err := assetUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsRatio: primary fetch: %w", err))
	}

	refDF, err := referenceUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsRatio: reference fetch: %w", err))
	}

	numRows := primaryDF.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("PairsRatio: need at least 2 data points, got %d", numRows))
	}

	primaryAssets := primaryDF.AssetList()
	refAssets := refDF.AssetList()
	times := primaryDF.Times()
	lastTime := []time.Time{times[len(times)-1]}

	// Cache reference price columns.
	refPrices := make(map[string][]float64, len(refAssets))

	for _, ra := range refAssets {
		refPrices[ra.Ticker] = refDF.Column(ra, data.MetricClose)
	}

	// Build metric names.
	metricNames := make([]data.Metric, len(refAssets))
	for ri, ra := range refAssets {
		metricNames[ri] = data.Metric("PairsRatio_" + ra.Ticker)
	}

	allCols := make([][]float64, len(primaryAssets)*len(refAssets))

	for pi, pa := range primaryAssets {
		paPrices := primaryDF.Column(pa, data.MetricClose)

		for ri, ra := range refAssets {
			raPrices := refPrices[ra.Ticker]

			// Compute price ratios.
			ratios := make([]float64, numRows)
			for ii := range numRows {
				if raPrices[ii] == 0 {
					return data.WithErr(fmt.Errorf("PairsRatio: zero reference price for %s at index %d", ra.Ticker, ii))
				}

				ratios[ii] = paPrices[ii] / raPrices[ii]
			}

			// Z-score of ratios.
			zz, zErr := zScore(ratios)
			if zErr != nil {
				return data.WithErr(fmt.Errorf("PairsRatio [%s vs %s]: %w", pa.Ticker, ra.Ticker, zErr))
			}

			allCols[pi*len(refAssets)+ri] = []float64{zz}
		}
	}

	result, err := data.NewDataFrame(lastTime, primaryAssets, metricNames, primaryDF.Frequency(), allCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsRatio: %w", err))
	}

	return result
}
