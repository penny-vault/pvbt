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
)

type benchmarkAfterTaxTWRR struct{}

func (benchmarkAfterTaxTWRR) Name() string { return "BenchmarkAfterTaxTWRR" }

func (benchmarkAfterTaxTWRR) Description() string {
	return "Cumulative time-weighted return on the benchmark, after a buy-and-hold tax model. The benchmark is assumed to be bought at the window's start price and held to the window's end. If the cumulative gain over the window is positive, it is taxed at 15% (long-term capital gains); a loss is not taxed. The after-tax terminal value is then converted to a return relative to the start price."
}

// Compute returns the after-tax cumulative return of the benchmark over
// the window. Benchmarks are assumed to be buy-and-hold within the
// window, so only one tax event applies: LTCG at window end if the
// position is in the green.
func (benchmarkAfterTaxTWRR) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	pd := stats.PerfDataView(ctx)
	if pd == nil {
		return 0, ErrNoBenchmark
	}

	bmRaw := pd.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmRaw) == 0 || bmRaw[0] == 0 {
		return 0, ErrNoBenchmark
	}

	bench := pd.Window(window).Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bench) < 2 {
		return 0, ErrInsufficientData
	}

	startVal := bench[0]
	endVal := bench[len(bench)-1]

	if startVal <= 0 {
		return 0, ErrInsufficientData
	}

	gain := endVal - startVal
	if gain > 0 {
		endVal -= defaultTaxRates().LTCG * gain
	}

	return endVal/startVal - 1, nil
}

func (benchmarkAfterTaxTWRR) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// BenchmarkAfterTaxTWRR is the cumulative after-tax buy-and-hold return
// of the benchmark over the window. Pair with AfterTaxTWRR for an
// apples-to-apples portfolio-vs-benchmark comparison.
var BenchmarkAfterTaxTWRR PerformanceMetric = benchmarkAfterTaxTWRR{}
