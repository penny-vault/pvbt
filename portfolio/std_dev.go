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
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"gonum.org/v1/gonum/stat"
)

type stdDev struct{}

func (stdDev) Name() string { return "StdDev" }

func (stdDev) Description() string {
	return "Annualized standard deviation of portfolio returns. Measures total volatility of the portfolio. Higher values mean more volatile returns. Used as the risk measure in the Sharpe ratio."
}

func (stdDev) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return 0, nil
	}

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioEquity))
	if len(col) < 2 {
		return 0, nil
	}

	stdDev := stat.StdDev(col, nil)
	if math.IsNaN(stdDev) {
		return 0, nil
	}

	af := annualizationFactor(df.Times())

	return stdDev * math.Sqrt(af), nil
}

// ComputeSeries returns an expanding-window annualized standard deviation:
// each point is the sample standard deviation of all returns observed up to
// and including that timestamp, annualized with the observation frequency.
// The final point matches the scalar Compute value. Timestamps stay aligned
// with the return each point incorporates, skipping NaN returns.
func (stdDev) ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period) (*data.DataFrame, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return nil, nil
	}

	col := df.Column(portfolioAsset, data.PortfolioEquity)
	times := df.Times()

	if len(col) == 0 || len(times) != len(col) {
		return nil, nil
	}

	seriesTimes := make([]time.Time, 0, len(col))
	values := make([]float64, 0, len(col))

	// Welford's online algorithm for a numerically stable running variance.
	count := 0
	mean := 0.0
	m2 := 0.0

	for idx, ret := range col {
		if math.IsNaN(ret) {
			continue
		}

		count++
		delta := ret - mean
		mean += delta / float64(count)
		m2 += delta * (ret - mean)

		if count < 2 {
			continue
		}

		af := annualizationFactor(times[:idx+1])
		values = append(values, math.Sqrt(m2/float64(count-1))*math.Sqrt(af))
		seriesTimes = append(seriesTimes, times[idx])
	}

	if len(values) == 0 {
		return nil, nil
	}

	return data.NewDataFrame(
		seriesTimes,
		[]asset.Asset{portfolioAsset},
		[]data.Metric{data.PortfolioEquity},
		df.Frequency(),
		[][]float64{values},
	)
}

// StdDev is the annualized standard deviation of portfolio returns.
func (stdDev) BenchmarkTargetable() {}

var StdDev PerformanceMetric = stdDev{}
