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

import "math"

type sortino struct{}

func (sortino) Name() string { return "Sortino" }

func (sortino) Description() string {
	return "Like Sharpe but penalizes only downside volatility. Uses downside deviation instead of standard deviation, making it more appropriate for strategies with asymmetric return distributions. Higher is better."
}

func (sortino) Compute(a *Account, window *Period) (float64, error) {
	if len(a.RiskFreePrices()) == 0 {
		return 0, ErrNoRiskFreeRate
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(eq)
	rf := returns(windowSlice(a.RiskFreePrices(), a.EquityTimes(), window))
	er := excessReturns(r, rf)

	// Downside deviation: sqrt(mean(min(r_i, 0)^2)) using all N observations.
	// This differs from stddev of only negative returns -- it includes zeros
	// for positive returns in the denominator, matching the standard definition.
	n := len(er)
	if n == 0 {
		return 0, nil
	}

	sumSq := 0.0
	for _, v := range er {
		if v < 0 {
			sumSq += v * v
		}
	}

	dd := math.Sqrt(sumSq / float64(n))
	if dd == 0 {
		return 0, nil
	}

	af := annualizationFactor(a.EquityTimes())
	return mean(er) / dd * math.Sqrt(af), nil
}

func (sortino) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// Sortino is the Sortino ratio: like Sharpe but uses downside deviation
// instead of total standard deviation, penalizing only negative volatility.
var Sortino PerformanceMetric = sortino{}
