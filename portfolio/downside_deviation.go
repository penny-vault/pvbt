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

type downsideDeviation struct{}

func (downsideDeviation) Name() string { return "DownsideDeviation" }

func (downsideDeviation) Description() string {
	return "Standard deviation of negative returns only. Unlike standard deviation which treats upside and downside volatility equally, this focuses solely on harmful volatility. Used in the Sortino ratio denominator."
}

func (downsideDeviation) Compute(a *Account, window *Period) (float64, error) {
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

	// Filter to only negative excess returns.
	var neg []float64

	for _, v := range erCol {
		if v < 0 {
			neg = append(neg, v)
		}
	}

	if len(neg) == 0 {
		return 0, nil
	}

	af := annualizationFactor(perfDF.Times())

	return stat.StdDev(neg, nil) * math.Sqrt(af), nil
}

func (downsideDeviation) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	return nil, nil
}

// DownsideDeviation measures the volatility of returns below the
// risk-free rate.
var DownsideDeviation PerformanceMetric = downsideDeviation{}
