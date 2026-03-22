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

	"github.com/penny-vault/pvbt/data"
	"gonum.org/v1/gonum/stat"
)

type alpha struct{}

func (alpha) Name() string { return "Alpha" }

func (alpha) Description() string {
	return "Annualized excess return above what the CAPM predicts given the portfolio's beta. Positive alpha indicates the portfolio outperformed its risk-adjusted expectation. The \"skill\" component of returns."
}

func (alpha) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	rdf := stats.Returns(ctx, window)
	bdf := stats.BenchmarkReturns(ctx, window)

	if rdf == nil || bdf == nil {
		return 0, nil
	}

	pd := stats.PerfDataView(ctx)
	if pd == nil {
		return 0, nil
	}

	rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
	if len(rfCol) == 0 || rfCol[0] == 0 {
		return 0, ErrNoRiskFreeRate
	}

	pCol, bCol := alignedRemoveNaN(
		rdf.Column(portfolioAsset, data.PortfolioEquity),
		bdf.Column(portfolioAsset, data.PortfolioBenchmark),
	)

	// Get risk-free returns for the window.
	rfPctDF := pd.Window(window).Metrics(data.PortfolioRiskFree).Pct()
	rCol := removeNaN(rfPctDF.Column(portfolioAsset, data.PortfolioRiskFree))

	// Truncate to shortest common length across all three series.
	minLen := min(len(pCol), len(rCol))

	if minLen < 2 {
		return 0, nil
	}

	pCol = pCol[:minLen]
	bCol = bCol[:minLen]
	rCol = rCol[:minLen]

	// Compute mean excess returns.
	excessPortfolio := make([]float64, minLen)
	excessBenchmark := make([]float64, minLen)

	for idx := range pCol {
		excessPortfolio[idx] = pCol[idx] - rCol[idx]
		excessBenchmark[idx] = bCol[idx] - rCol[idx]
	}

	meanExcessPortfolio := stat.Mean(excessPortfolio, nil)
	meanExcessBenchmark := stat.Mean(excessBenchmark, nil)

	betaVal, err := Beta.Compute(ctx, stats, window)
	if err != nil {
		return 0, err
	}

	af := annualizationFactor(rdf.Times())

	return (meanExcessPortfolio - betaVal*meanExcessBenchmark) * af, nil
}

func (alpha) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// Alpha is Jensen's alpha: the portfolio's excess return over what CAPM
// would predict given its beta.
var Alpha PerformanceMetric = alpha{}
