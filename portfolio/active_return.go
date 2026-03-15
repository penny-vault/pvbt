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
	"math"

	"github.com/penny-vault/pvbt/data"
)

type activeReturn struct{}

func (activeReturn) Name() string { return "ActiveReturn" }

func (activeReturn) Description() string {
	return "The difference between portfolio return and benchmark return over the full period. Positive values indicate the portfolio outperformed the benchmark. Unlike Alpha, this is a raw return difference without adjusting for risk."
}

// Compute returns the portfolio total return minus the benchmark total
// return. Total return is (end/start) - 1.
func (activeReturn) Compute(a *Account, window *Period) (float64, error) {
	perfData := a.PerfData()
	if perfData == nil {
		return 0, nil
	}

	bmCol := perfData.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmCol) == 0 || bmCol[0] == 0 {
		return 0, ErrNoBenchmark
	}

	perfDF := perfData.Window(window)
	eqCol := perfDF.Column(portfolioAsset, data.PortfolioEquity)
	benchCol := perfDF.Column(portfolioAsset, data.PortfolioBenchmark)

	if len(eqCol) < 2 || len(benchCol) < 2 {
		return 0, nil
	}

	portReturn := (eqCol[len(eqCol)-1] / eqCol[0]) - 1
	benchReturn := (benchCol[len(benchCol)-1] / benchCol[0]) - 1

	return portReturn - benchReturn, nil
}

// ComputeSeries returns the element-wise difference between the
// portfolio cumulative return series and the benchmark cumulative
// return series.
func (activeReturn) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	perfData := a.PerfData()
	if perfData == nil {
		return nil, nil
	}

	bmCol := perfData.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmCol) == 0 || bmCol[0] == 0 {
		return nil, ErrNoBenchmark
	}

	perfDF := perfData.Window(window).Metrics(data.PortfolioEquity, data.PortfolioBenchmark)

	returns := perfDF.Pct().Drop(math.NaN())
	if returns.Len() == 0 {
		return nil, nil
	}

	portR := returns.Column(portfolioAsset, data.PortfolioEquity)
	benchR := returns.Column(portfolioAsset, data.PortfolioBenchmark)

	count := len(portR)
	if len(benchR) < count {
		count = len(benchR)
	}

	if count == 0 {
		return nil, nil
	}

	series := make([]float64, count)
	cumPort := 1.0
	cumBench := 1.0

	for i := 0; i < count; i++ {
		cumPort *= (1 + portR[i])
		cumBench *= (1 + benchR[i])
		series[i] = (cumPort - 1) - (cumBench - 1)
	}

	return series, nil
}

// ActiveReturn is the difference between portfolio return and benchmark
// return (strategy - benchmark). Measures the value added by active
// management. Highly dependent on appropriate benchmark selection.
var ActiveReturn PerformanceMetric = activeReturn{}
