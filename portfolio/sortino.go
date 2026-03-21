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

	"github.com/penny-vault/pvbt/data"
	"gonum.org/v1/gonum/stat"
)

type sortino struct{}

func (sortino) Name() string { return "Sortino" }

func (sortino) Description() string {
	return "Like Sharpe but penalizes only downside volatility. Uses downside deviation instead of standard deviation, making it more appropriate for strategies with asymmetric return distributions. Higher is better."
}

func (sortino) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	pd := stats.PerfDataView(ctx)
	if pd == nil {
		return 0, nil
	}

	rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
	if len(rfCol) == 0 || rfCol[0] == 0 {
		return 0, ErrNoRiskFreeRate
	}

	df := stats.ExcessReturns(ctx, window)
	if df == nil {
		return 0, nil
	}

	erCol := removeNaN(df.Column(portfolioAsset, data.PortfolioEquity))

	// Downside deviation: sqrt(mean(min(r_i, 0)^2)) using all N observations.
	count := len(erCol)
	if count == 0 {
		return 0, nil
	}

	sumSq := 0.0

	for _, v := range erCol {
		if v < 0 {
			sumSq += v * v
		}
	}

	downsideDev := math.Sqrt(sumSq / float64(count))
	if downsideDev == 0 {
		return 0, nil
	}

	af := annualizationFactor(df.Times())

	return stat.Mean(erCol, nil) / downsideDev * math.Sqrt(af), nil
}

func (sortino) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// Sortino is the Sortino ratio: like Sharpe but uses downside deviation
// instead of total standard deviation, penalizing only negative volatility.
func (sortino) BenchmarkTargetable() {}

var Sortino PerformanceMetric = sortino{}
