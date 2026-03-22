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

type beta struct{}

func (beta) Name() string { return "Beta" }

func (beta) Description() string {
	return "Sensitivity of portfolio returns to benchmark returns. A beta of 1.0 moves in lockstep with the benchmark. Above 1.0 amplifies benchmark moves, below 1.0 dampens them. Negative beta moves opposite to the benchmark."
}

func (beta) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	// Check if benchmark is configured by examining the raw equity-level
	// benchmark column. A zero first value means no benchmark was set.
	pd := stats.PerfDataView(ctx)
	if pd == nil {
		return 0, nil
	}

	bmRaw := pd.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmRaw) == 0 || bmRaw[0] == 0 {
		return 0, ErrNoBenchmark
	}

	rdf := stats.Returns(ctx, window)
	bdf := stats.BenchmarkReturns(ctx, window)

	if rdf == nil || bdf == nil {
		return 0, nil
	}

	pCol, bCol := alignedRemoveNaN(
		rdf.Column(portfolioAsset, data.PortfolioEquity),
		bdf.Column(portfolioAsset, data.PortfolioBenchmark),
	)

	if len(pCol) < 2 {
		return 0, nil
	}

	benchVariance := stat.Variance(bCol, nil)
	if benchVariance == 0 || math.IsNaN(benchVariance) {
		return 0, nil
	}

	cov := stat.Covariance(pCol, bCol, nil)
	if math.IsNaN(cov) {
		return 0, nil
	}

	return cov / benchVariance, nil
}

func (beta) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// Beta measures the portfolio's sensitivity to benchmark movements.
var Beta PerformanceMetric = beta{}
