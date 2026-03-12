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

type trackingError struct{}

func (trackingError) Name() string { return "TrackingError" }

func (trackingError) Description() string {
	return "Annualized standard deviation of the difference between portfolio and benchmark returns. Measures how closely the portfolio follows its benchmark. Lower values indicate tighter tracking."
}

func (trackingError) Compute(a *Account, window *Period) (float64, error) {
	if len(a.BenchmarkPrices()) == 0 {
		return 0, ErrNoBenchmark
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	bm := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)

	pReturns := returns(eq)
	bReturns := returns(bm)

	activeReturns := excessReturns(pReturns, bReturns)
	if len(activeReturns) == 0 {
		return 0, nil
	}

	times := a.EquityTimes()
	af := annualizationFactor(times)

	return stddev(activeReturns) * math.Sqrt(af), nil
}

func (trackingError) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// TrackingError is the standard deviation of the difference between
// portfolio returns and benchmark returns.
var TrackingError PerformanceMetric = trackingError{}
