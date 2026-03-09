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

type rSquared struct{}

func (rSquared) Name() string { return "RSquared" }

func (rSquared) Compute(a *Account, window *Period) float64 {
	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	bm := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)

	pReturns := returns(eq)
	bReturns := returns(bm)

	if len(pReturns) == 0 || len(bReturns) == 0 {
		return 0
	}

	sp := stddev(pReturns)
	sb := stddev(bReturns)

	if sp == 0 || sb == 0 {
		return 0
	}

	corr := covariance(pReturns, bReturns) / (sp * sb)

	return corr * corr
}

func (rSquared) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// RSquared measures how well portfolio returns are explained by
// benchmark returns (coefficient of determination). Ranges from 0
// to 1. A value near 1 means the portfolio closely tracks the
// benchmark; a low value means returns are driven by other factors.
var RSquared = rSquared{}
