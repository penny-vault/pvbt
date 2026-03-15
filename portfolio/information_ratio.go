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
	"gonum.org/v1/gonum/stat"
)

type informationRatio struct{}

func (informationRatio) Name() string { return "InformationRatio" }

func (informationRatio) Description() string {
	return "Active return divided by tracking error. Measures how consistently the portfolio outperforms its benchmark per unit of active risk taken. Higher values indicate more consistent outperformance."
}

func (informationRatio) Compute(a *Account, window *Period) (float64, error) {
	perfData := a.PerfData()
	if perfData == nil {
		return 0, nil
	}

	bmCol := perfData.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmCol) == 0 || bmCol[0] == 0 {
		return 0, ErrNoBenchmark
	}

	perfDF := perfData.Window(window)

	returns := perfDF.Metrics(data.PortfolioEquity, data.PortfolioBenchmark).Pct().Drop(math.NaN())
	if returns.Len() == 0 {
		return 0, nil
	}
	// Active returns = portfolio returns - benchmark returns
	activeReturns := returns.Metrics(data.PortfolioEquity).Sub(returns, data.PortfolioBenchmark)
	if activeReturns.Len() == 0 {
		return 0, nil
	}

	arCol := activeReturns.Column(portfolioAsset, data.PortfolioEquity)
	if len(arCol) < 2 {
		return 0, nil
	}

	trackingErr := stat.StdDev(arCol, nil)
	if trackingErr == 0 || math.IsNaN(trackingErr) {
		return 0, nil
	}

	af := annualizationFactor(perfDF.Times())

	return stat.Mean(arCol, nil) / trackingErr * math.Sqrt(af), nil
}

func (informationRatio) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	return nil, nil
}

// InformationRatio measures the portfolio's active return per unit of
// tracking error relative to the benchmark.
var InformationRatio PerformanceMetric = informationRatio{}
