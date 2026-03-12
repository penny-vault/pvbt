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

type treynor struct{}

func (treynor) Name() string { return "Treynor" }

func (treynor) Description() string {
	return "Excess return per unit of systematic risk (beta). Similar to Sharpe but uses beta instead of standard deviation. Appropriate for well-diversified portfolios where unsystematic risk has been eliminated."
}

func (treynor) Compute(a *Account, window *Period) (float64, error) {
	if len(a.RiskFreePrices()) == 0 {
		return 0, ErrNoRiskFreeRate
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	rf := windowSlice(a.RiskFreePrices(), a.EquityTimes(), window)

	if len(eq) < 2 || len(rf) < 2 {
		return 0, nil
	}

	portfolioReturn := (eq[len(eq)-1] / eq[0]) - 1
	riskFreeReturn := (rf[len(rf)-1] / rf[0]) - 1

	b, err := Beta.Compute(a, window)
	if err != nil {
		return 0, err
	}
	if b == 0 {
		return 0, nil
	}

	return (portfolioReturn - riskFreeReturn) / b, nil
}

func (treynor) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// Treynor is the Treynor ratio: excess return per unit of systematic
// risk (beta).
var Treynor PerformanceMetric = treynor{}
