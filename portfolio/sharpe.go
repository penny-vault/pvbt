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

type sharpe struct{}

func (sharpe) Name() string { return "Sharpe" }

func (sharpe) Compute(a *Account, window *Period) float64 {
	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(eq)
	rf := returns(windowSlice(a.RiskFreePrices(), a.EquityTimes(), window))
	er := excessReturns(r, rf)

	sd := stddev(er)
	if sd == 0 {
		return 0
	}

	af := annualizationFactor(a.EquityTimes())
	return mean(er) / sd * math.Sqrt(af)
}

func (sharpe) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// Sharpe is the Sharpe ratio: risk-adjusted return relative to a
// risk-free rate, using standard deviation of returns as the risk measure.
var Sharpe = sharpe{}
