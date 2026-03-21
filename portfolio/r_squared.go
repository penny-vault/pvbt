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

type rSquared struct{}

func (rSquared) Name() string { return "RSquared" }

func (rSquared) Description() string {
	return "Proportion of portfolio return variance explained by benchmark returns. Ranges from 0 to 1. Values near 1.0 mean the portfolio closely tracks the benchmark. Low R-squared makes beta and alpha less meaningful."
}

func (rSquared) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
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

	pCol := removeNaN(rdf.Column(portfolioAsset, data.PortfolioEquity))
	bCol := removeNaN(bdf.Column(portfolioAsset, data.PortfolioBenchmark))

	if len(pCol) < 2 || len(bCol) < 2 {
		return 0, nil
	}

	portfolioStdDev := stat.StdDev(pCol, nil)
	benchStdDev := stat.StdDev(bCol, nil)

	if portfolioStdDev == 0 || benchStdDev == 0 || math.IsNaN(portfolioStdDev) || math.IsNaN(benchStdDev) {
		return 0, nil
	}

	corr := stat.Covariance(pCol, bCol, nil) / (portfolioStdDev * benchStdDev)

	return corr * corr, nil
}

func (rSquared) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// RSquared measures how well portfolio returns are explained by
// benchmark returns (coefficient of determination). Ranges from 0
// to 1. A value near 1 means the portfolio closely tracks the
// benchmark; a low value means returns are driven by other factors.
var RSquared PerformanceMetric = rSquared{}
