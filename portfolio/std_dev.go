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
	"github.com/penny-vault/pvbt/asset"
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

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioReturns))
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

func (stdDev) ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period) (*data.DataFrame, error) {
	df := stats.Returns(ctx, window)
	if df == nil {
		return nil, nil
	}

	col := removeNaN(df.Column(portfolioAsset, data.PortfolioReturns))
	if len(col) == 0 {
		return nil, nil
	}

	times := df.Times()
	seriesTimes := times[1:]

	return data.NewDataFrame(
		seriesTimes,
		[]asset.Asset{portfolioAsset},
		[]data.Metric{data.PortfolioReturns},
		df.Frequency(),
		[][]float64{col},
	)
}

// StdDev is the annualized standard deviation of portfolio returns.
func (stdDev) BenchmarkTargetable() {}

var StdDev PerformanceMetric = stdDev{}
