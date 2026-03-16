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

type sharpe struct{}

func (sharpe) Name() string { return "Sharpe" }

func (sharpe) Description() string {
	return "Risk-adjusted return relative to a risk-free rate. Computed as the mean excess return divided by its standard deviation, annualized. Higher values indicate better risk-adjusted performance. A Sharpe above 1.0 is generally considered good, above 2.0 is very good."
}

func (sharpe) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}

	rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
	if len(rfCol) == 0 || rfCol[0] == 0 {
		return 0, ErrNoRiskFreeRate
	}

	perfDF := pd.Window(window)
	returns := perfDF.Pct().Drop(math.NaN())

	er := returns.Metrics(data.PortfolioEquity).Sub(returns, data.PortfolioRiskFree)
	if er.Len() == 0 {
		return 0, nil
	}

	erCol := er.Column(portfolioAsset, data.PortfolioEquity)
	if len(erCol) < 2 {
		return 0, nil
	}

	stdDev := stat.StdDev(erCol, nil)
	if stdDev == 0 || math.IsNaN(stdDev) {
		return 0, nil
	}

	af := annualizationFactor(perfDF.Times())

	return stat.Mean(erCol, nil) / stdDev * math.Sqrt(af), nil
}

func (sharpe) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// Sharpe is the Sharpe ratio: risk-adjusted return relative to a
// risk-free rate, using standard deviation of returns as the risk measure.
func (sharpe) BenchmarkTargetable() {}

var Sharpe PerformanceMetric = sharpe{}
