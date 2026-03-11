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

type alpha struct{}

func (alpha) Name() string { return "Alpha" }

func (alpha) Description() string {
	return "Annualized excess return above what the CAPM predicts given the portfolio's beta. Positive alpha indicates the portfolio outperformed its risk-adjusted expectation. The \"skill\" component of returns."
}

func (alpha) Compute(a *Account, window *Period) float64 {
	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	bm := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)
	rf := windowSlice(a.RiskFreePrices(), a.EquityTimes(), window)

	if len(eq) < 2 || len(bm) < 2 || len(rf) < 2 {
		return 0
	}

	portfolioReturn := (eq[len(eq)-1] / eq[0]) - 1
	benchmarkReturn := (bm[len(bm)-1] / bm[0]) - 1
	riskFreeReturn := (rf[len(rf)-1] / rf[0]) - 1

	b := Beta.Compute(a, window)

	return portfolioReturn - (riskFreeReturn + b*(benchmarkReturn-riskFreeReturn))
}

func (alpha) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// Alpha is Jensen's alpha: the portfolio's excess return over what CAPM
// would predict given its beta.
var Alpha PerformanceMetric = alpha{}
