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

type downsideDeviation struct{}

func (downsideDeviation) Name() string { return "DownsideDeviation" }

func (downsideDeviation) Compute(a *Account, window *Period) float64 {
	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(eq)
	rf := returns(windowSlice(a.RiskFreePrices(), a.EquityTimes(), window))
	er := excessReturns(r, rf)

	// Filter to only negative excess returns.
	var neg []float64
	for _, v := range er {
		if v < 0 {
			neg = append(neg, v)
		}
	}

	if len(neg) == 0 {
		return 0
	}

	af := annualizationFactor(a.EquityTimes())
	return stddev(neg) * math.Sqrt(af)
}

func (downsideDeviation) ComputeSeries(a *Account, window *Period) []float64 {
	return nil
}

// DownsideDeviation measures the volatility of returns below the
// risk-free rate.
var DownsideDeviation PerformanceMetric = downsideDeviation{}
