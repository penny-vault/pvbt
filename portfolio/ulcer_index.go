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

package portfolio

import (
	"context"
	"math"
	"sort"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"gonum.org/v1/gonum/stat"
)

const ulcerLookback = 14

type ulcerIndex struct{}

func (ulcerIndex) Name() string { return "UlcerIndex" }

func (ulcerIndex) Description() string {
	return "Measures downside risk using both the depth and duration of drawdowns over a 14-period lookback window. Computed as the square root of the mean of squared percentage drawdowns. Values are on a 0-100 percentage scale. Higher values indicate more painful drawdown experiences. Returns 0 when fewer than 14 data points are available."
}

func (ulcerIndex) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, nil
	}

	eqCol := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(eqCol) < ulcerLookback {
		return 0, nil
	}

	series := rollingUlcerSeries(eqCol)

	return series[len(series)-1], nil
}

// rollingUlcerSeries implements the two-pass Ulcer Index formula matching
// TradingView's definition:
//
//  1. DD[i] = (price[i] - max(price[i-L+1:i+1])) / max × 100
//     Each bar's drawdown is measured from its own L-day rolling high.
//
//  2. UI[i] = sqrt(mean(DD[i-L+1:i+1]²))
//     The UI at each bar is the RMS of the last L per-bar drawdowns.
//
// Using a single running-peak per outer window (the old approach) causes bars
// near the start of each window to miss peaks that occurred in the L-1 days
// before the window, systematically underestimating the index.
func rollingUlcerSeries(eqCol []float64) []float64 {
	nn := len(eqCol)
	result := make([]float64, nn)

	// Pass 1: rolling-max drawdown at each bar.
	dd := make([]float64, nn)

	for ii := range nn {
		start := max(0, ii-ulcerLookback+1)

		peak := eqCol[start]

		for _, val := range eqCol[start : ii+1] {
			if val > peak {
				peak = val
			}
		}

		dd[ii] = (eqCol[ii] - peak) / peak * 100
	}

	// Pass 2: RMS of the last L per-bar drawdowns.
	for ii := ulcerLookback - 1; ii < nn; ii++ {
		sumSq := 0.0

		for _, dd := range dd[ii-ulcerLookback+1 : ii+1] {
			sumSq += dd * dd
		}

		result[ii] = math.Sqrt(sumSq / float64(ulcerLookback))
	}

	return result
}

func (ulcerIndex) ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period) (*data.DataFrame, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return nil, nil
	}

	eqCol := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(eqCol) < ulcerLookback {
		return nil, nil
	}

	uiSeries := rollingUlcerSeries(eqCol)

	return data.NewDataFrame(
		df.Times(),
		[]asset.Asset{portfolioAsset},
		[]data.Metric{data.PortfolioEquity},
		df.Frequency(),
		[][]float64{uiSeries},
	)
}

// UlcerIndex measures downside risk using both the depth and duration
// of drawdowns. Computed as the square root of the mean of squared
// percentage drawdowns over a 14-period lookback window. Values are
// on a 0-100 percentage scale. Higher values indicate more painful
// drawdown experiences. Returns 0 when fewer than 14 data points
// are available.
func (ulcerIndex) BenchmarkTargetable() {}

var UlcerIndex PerformanceMetric = ulcerIndex{}

// --- AvgUlcerIndex ---

type avgUlcerIndex struct{}

func (avgUlcerIndex) Name() string { return "AvgUlcerIndex" }

func (avgUlcerIndex) Description() string {
	return "Mean of the 14-period rolling Ulcer Index over the full window. Measures typical drawdown pain across the entire holding period rather than only the most recent 14 days. Returns 0 when fewer than 14 data points are available."
}

func (avgUlcerIndex) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, nil
	}

	eqCol := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(eqCol) < ulcerLookback {
		return 0, nil
	}

	series := rollingUlcerSeries(eqCol)
	active := series[ulcerLookback-1:]

	sum := 0.0
	for _, val := range active {
		sum += val
	}

	return sum / float64(len(active)), nil
}

func (avgUlcerIndex) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

func (avgUlcerIndex) BenchmarkTargetable() {}

var AvgUlcerIndex PerformanceMetric = avgUlcerIndex{}

// --- P90UlcerIndex ---

type p90UlcerIndex struct{}

func (p90UlcerIndex) Name() string { return "P90UlcerIndex" }

func (p90UlcerIndex) Description() string {
	return "90th percentile of the 14-period rolling Ulcer Index over the full window. Represents worst-typical drawdown pain: the level exceeded only 10% of the time. Returns 0 when fewer than 14 data points are available."
}

func (p90UlcerIndex) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, nil
	}

	eqCol := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(eqCol) < ulcerLookback {
		return 0, nil
	}

	series := rollingUlcerSeries(eqCol)
	active := make([]float64, len(series)-ulcerLookback+1)
	copy(active, series[ulcerLookback-1:])
	sort.Float64s(active)

	return stat.Quantile(0.9, stat.Empirical, active, nil), nil
}

func (p90UlcerIndex) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

func (p90UlcerIndex) BenchmarkTargetable() {}

var P90UlcerIndex PerformanceMetric = p90UlcerIndex{}

// --- MedianUlcerIndex ---

type medianUlcerIndex struct{}

func (medianUlcerIndex) Name() string { return "MedianUlcerIndex" }

func (medianUlcerIndex) Description() string {
	return "Median of the 14-period rolling Ulcer Index over the full window. More robust to extreme drawdown episodes than the mean. Returns 0 when fewer than 14 data points are available."
}

func (medianUlcerIndex) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, nil
	}

	eqCol := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(eqCol) < ulcerLookback {
		return 0, nil
	}

	series := rollingUlcerSeries(eqCol)
	active := make([]float64, len(series)-ulcerLookback+1)
	copy(active, series[ulcerLookback-1:])
	sort.Float64s(active)

	return stat.Quantile(0.5, stat.Empirical, active, nil), nil
}

func (medianUlcerIndex) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

func (medianUlcerIndex) BenchmarkTargetable() {}

var MedianUlcerIndex PerformanceMetric = medianUlcerIndex{}
