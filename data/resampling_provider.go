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

package data

import (
	"context"
	"fmt"
	"math/rand/v2"
)

// Compile-time interface check.
var _ BatchProvider = (*ResamplingProvider)(nil)

// ResamplingProvider implements BatchProvider by wrapping pre-fetched historical
// data and producing synthetic price series via resampling. It uses a Resampler
// to draw from historical daily returns and reconstructs price paths that share
// the statistical properties of the historical data.
type ResamplingProvider struct {
	historicalData *DataFrame
	resampler      Resampler
	seed           uint64
	metrics        []Metric
}

// NewResamplingProvider returns a ResamplingProvider backed by the given
// historical DataFrame. The resampler controls how synthetic returns are
// drawn; seed initialises the PRNG for reproducibility.
func NewResamplingProvider(historicalData *DataFrame, resampler Resampler, seed uint64, metrics []Metric) *ResamplingProvider {
	return &ResamplingProvider{
		historicalData: historicalData,
		resampler:      resampler,
		seed:           seed,
		metrics:        metrics,
	}
}

// Provides returns the set of metrics this provider can supply.
func (rp *ResamplingProvider) Provides() []Metric {
	return rp.metrics
}

// Close is a no-op for ResamplingProvider.
func (rp *ResamplingProvider) Close() error {
	return nil
}

// Fetch produces a synthetic price DataFrame for the requested assets, metrics,
// and time range. It:
//  1. Narrows historical data to the requested assets and MetricClose only.
//  2. Extracts daily returns from close prices for each asset.
//  3. Passes the returns through the configured Resampler.
//  4. Reconstructs synthetic prices from resampled returns, starting at each
//     asset's first historical close price.
//  5. Maps each requested metric to the synthetic series: Close/AdjClose/Open
//     receive the synthetic price; High gets price*1.005; Low gets price*0.995;
//     Dividend gets 0.0; SplitFactor gets 1.0.
//  6. Uses the historical time axis (narrowed to the requested range) as the
//     output time axis.
func (rp *ResamplingProvider) Fetch(ctx context.Context, req DataRequest) (*DataFrame, error) {
	// Narrow historical data to the requested assets using only MetricClose.
	narrow := rp.historicalData.Assets(req.Assets...).Metrics(MetricClose).Between(req.Start, req.End)
	if err := narrow.Err(); err != nil {
		return nil, fmt.Errorf("ResamplingProvider: narrowing historical data: %w", err)
	}

	times := narrow.Times()
	numTimes := len(times)
	assets := narrow.AssetList()
	numAssets := len(assets)

	if numTimes == 0 || numAssets == 0 {
		// Return an empty DataFrame matching the requested metrics.
		cols := make([][]float64, numAssets*len(req.Metrics))

		df, err := NewDataFrame(times, assets, req.Metrics, req.Frequency, cols)
		if err != nil {
			return nil, fmt.Errorf("ResamplingProvider: building empty result: %w", err)
		}

		return df, nil
	}

	// Extract daily returns for each asset from close prices.
	// Returns series has length numTimes-1 (need at least 2 prices).
	returnLen := numTimes - 1
	historicalReturns := make([][]float64, numAssets)
	firstPrices := make([]float64, numAssets)

	for assetIdx, ast := range assets {
		closePrices := narrow.Column(ast, MetricClose)
		firstPrices[assetIdx] = closePrices[0]

		rets := make([]float64, returnLen)
		for timeIdx := range returnLen {
			prev := closePrices[timeIdx]
			curr := closePrices[timeIdx+1]

			if prev == 0 {
				rets[timeIdx] = 0
			} else {
				rets[timeIdx] = (curr - prev) / prev
			}
		}

		historicalReturns[assetIdx] = rets
	}

	// Resample returns using the configured strategy.
	rng := rand.New(rand.NewPCG(rp.seed, rp.seed^0xdeadbeef))
	resampledReturns := rp.resampler.Resample(historicalReturns, returnLen, rng)

	// Reconstruct synthetic prices from resampled returns.
	// syntheticPrices[assetIdx] has length numTimes.
	syntheticPrices := make([][]float64, numAssets)
	for assetIdx := range numAssets {
		prices := make([]float64, numTimes)
		prices[0] = firstPrices[assetIdx]

		for timeIdx := range returnLen {
			prices[timeIdx+1] = prices[timeIdx] * (1 + resampledReturns[assetIdx][timeIdx])
		}

		syntheticPrices[assetIdx] = prices
	}

	// Build output columns for each requested metric.
	numMetrics := len(req.Metrics)
	cols := make([][]float64, numAssets*numMetrics)

	for assetIdx := range numAssets {
		prices := syntheticPrices[assetIdx]

		for mIdx, metric := range req.Metrics {
			col := make([]float64, numTimes)

			switch metric {
			case MetricClose, AdjClose, MetricOpen:
				copy(col, prices)
			case MetricHigh:
				for timeIdx, price := range prices {
					col[timeIdx] = price * 1.005
				}
			case MetricLow:
				for timeIdx, price := range prices {
					col[timeIdx] = price * 0.995
				}
			case Dividend:
				// All zeros (default value for new slice).
			case SplitFactor:
				for timeIdx := range numTimes {
					col[timeIdx] = 1.0
				}
			default:
				// Unknown metrics default to the synthetic price.
				copy(col, prices)
			}

			cols[assetIdx*numMetrics+mIdx] = col
		}
	}

	df, err := NewDataFrame(times, assets, req.Metrics, req.Frequency, cols)
	if err != nil {
		return nil, fmt.Errorf("ResamplingProvider: building result DataFrame: %w", err)
	}

	return df, nil
}
