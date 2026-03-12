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

type smartSharpe struct{}

func (smartSharpe) Name() string { return "SmartSharpe" }

func (smartSharpe) Description() string {
	return "Sharpe ratio penalized for autocorrelation in returns. When returns are serially correlated, the standard Sharpe overstates risk-adjusted performance. The penalty factor is 1 + 2*sum(autocorrelations), following Lo (2002). Lower than Sharpe when returns exhibit positive autocorrelation."
}

func (smartSharpe) Compute(a *Account, window *Period) (float64, error) {
	if len(a.RiskFreePrices()) == 0 {
		return 0, ErrNoRiskFreeRate
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(eq)
	rf := returns(windowSlice(a.RiskFreePrices(), a.EquityTimes(), window))
	er := excessReturns(r, rf)

	sd := stddev(er)
	if sd == 0 {
		return 0, nil
	}

	af := annualizationFactor(a.EquityTimes())
	rawSharpe := mean(er) / sd * math.Sqrt(af)

	penalty := autocorrelationPenalty(er)
	if penalty == 0 {
		return 0, nil
	}

	return rawSharpe / penalty, nil
}

func (smartSharpe) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// autocorrelationPenalty computes the Lo (2002) correction factor:
// sqrt(1 + 2*sum(rho_k)) where rho_k is the autocorrelation at lag k.
// Uses lags 1 through min(n/4, 6) to avoid noise from high-lag estimates.
func autocorrelationPenalty(r []float64) float64 {
	n := len(r)
	if n < 4 {
		return 1
	}

	m := mean(r)
	maxLag := min(n/4, 6)

	// Compute variance (denominator for autocorrelation).
	var varSum float64
	for _, v := range r {
		d := v - m
		varSum += d * d
	}

	if varSum == 0 {
		return 1
	}

	// Sum autocorrelations at lags 1..maxLag.
	var acSum float64
	for lag := 1; lag <= maxLag; lag++ {
		var covSum float64
		for i := lag; i < n; i++ {
			covSum += (r[i] - m) * (r[i-lag] - m)
		}
		acSum += covSum / varSum
	}

	// Penalty = sqrt(1 + 2*sum(rho_k)).
	inner := 1 + 2*acSum
	if inner <= 0 {
		return 1
	}

	return math.Sqrt(inner)
}

// SmartSharpe is the Sharpe ratio corrected for autocorrelation in
// returns using the Lo (2002) method. It divides the standard Sharpe
// by a penalty factor derived from return serial correlation.
var SmartSharpe PerformanceMetric = smartSharpe{}
