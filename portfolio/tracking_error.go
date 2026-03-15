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

type trackingError struct{}

func (trackingError) Name() string { return "TrackingError" }

func (trackingError) Description() string {
	return "Annualized standard deviation of the difference between portfolio and benchmark returns. Measures how closely the portfolio follows its benchmark. Lower values indicate tighter tracking."
}

func (trackingError) Compute(a *Account, window *Period) (float64, error) {
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
	// Compute active returns = portfolio returns - benchmark returns
	activeReturns := returns.Metrics(data.PortfolioEquity).Sub(returns, data.PortfolioBenchmark)
	if activeReturns.Len() == 0 {
		return 0, nil
	}

	arCol := activeReturns.Column(portfolioAsset, data.PortfolioEquity)
	if len(arCol) < 2 {
		return 0, nil
	}

	stdDev := stat.StdDev(arCol, nil)
	if math.IsNaN(stdDev) {
		return 0, nil
	}

	af := annualizationFactor(perfDF.Times())

	return stdDev * math.Sqrt(af), nil
}

func (trackingError) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// TrackingError is the standard deviation of the difference between
// portfolio returns and benchmark returns.
var TrackingError PerformanceMetric = trackingError{}
