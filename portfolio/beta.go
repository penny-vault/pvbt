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

type beta struct{}

func (beta) Name() string { return "Beta" }

func (beta) Description() string {
	return "Sensitivity of portfolio returns to benchmark returns. A beta of 1.0 moves in lockstep with the benchmark. Above 1.0 amplifies benchmark moves, below 1.0 dampens them. Negative beta moves opposite to the benchmark."
}

func (beta) Compute(a *Account, window *Period) (float64, error) {
	if len(a.BenchmarkPrices()) == 0 {
		return 0, ErrNoBenchmark
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	bm := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)

	pReturns := returns(eq)
	bReturns := returns(bm)

	if len(pReturns) == 0 || len(bReturns) == 0 {
		return 0, nil
	}

	v := variance(bReturns)
	if v == 0 {
		return 0, nil
	}

	return covariance(pReturns, bReturns) / v, nil
}

func (beta) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// Beta measures the portfolio's sensitivity to benchmark movements.
var Beta PerformanceMetric = beta{}
