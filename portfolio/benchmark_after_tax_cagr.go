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
)

type benchmarkAfterTaxCAGR struct{}

func (benchmarkAfterTaxCAGR) Name() string { return "BenchmarkAfterTaxCAGR" }

func (benchmarkAfterTaxCAGR) Description() string {
	return "Annualized after-tax buy-and-hold growth rate of the benchmark. The benchmark is assumed to be bought at the window's start price and held to the window's end. If the cumulative gain over the window is positive, it is taxed at 15% (long-term capital gains); a loss is not taxed. The after-tax terminal value is then annualized over the window length."
}

func (benchmarkAfterTaxCAGR) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	pd := stats.PerfDataView(ctx)
	if pd == nil {
		return 0, ErrNoBenchmark
	}

	bmRaw := pd.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmRaw) == 0 || bmRaw[0] == 0 {
		return 0, ErrNoBenchmark
	}

	bdf := pd.Window(window)
	bench := bdf.Column(portfolioAsset, data.PortfolioBenchmark)
	times := bdf.Times()

	if len(bench) < 2 || len(times) < 2 {
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

	if endVal <= 0 {
		return 0, ErrInsufficientData
	}

	years := times[len(times)-1].Sub(times[0]).Hours() / 24 / 365.25
	if years <= 0 {
		return 0, ErrInsufficientData
	}

	return math.Pow(endVal/startVal, 1.0/years) - 1, nil
}

func (benchmarkAfterTaxCAGR) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

func (benchmarkAfterTaxCAGR) HigherIsBetter() bool { return true }

// BenchmarkAfterTaxCAGR is the annualized after-tax buy-and-hold growth
// rate of the benchmark over the window. Pair with AfterTaxCAGR for an
// apples-to-apples portfolio-vs-benchmark comparison.
var BenchmarkAfterTaxCAGR PerformanceMetric = benchmarkAfterTaxCAGR{}
