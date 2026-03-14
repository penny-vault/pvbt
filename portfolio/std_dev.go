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

type stdDev struct{}

func (stdDev) Name() string { return "StdDev" }

func (stdDev) Description() string {
	return "Annualized standard deviation of portfolio returns. Measures total volatility of the portfolio. Higher values mean more volatile returns. Used as the risk measure in the Sharpe ratio."
}

func (stdDev) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}
	perfDF := pd.Window(window)
	eq := perfDF.Metrics(data.PortfolioEquity)
	r := eq.Pct().Drop(math.NaN())
	if r.Len() == 0 {
		return 0, nil
	}
	col := r.Column(portfolioAsset, data.PortfolioEquity)
	if len(col) < 2 {
		return 0, nil
	}
	sd := stat.StdDev(col, nil)
	if math.IsNaN(sd) {
		return 0, nil
	}
	af := annualizationFactor(perfDF.Times())
	return sd * math.Sqrt(af), nil
}

func (stdDev) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return nil, nil
	}
	eq := pd.Window(window).Metrics(data.PortfolioEquity)
	r := eq.Pct().Drop(math.NaN())
	if r.Len() == 0 {
		return nil, nil
	}
	return r.Column(portfolioAsset, data.PortfolioEquity), nil
}

// StdDev is the annualized standard deviation of portfolio returns.
var StdDev PerformanceMetric = stdDev{}
