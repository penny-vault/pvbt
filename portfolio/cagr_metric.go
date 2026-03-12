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

type cagrMetric struct{}

func (cagrMetric) Name() string { return "CAGR" }

func (cagrMetric) Description() string {
	return "Compound Annual Growth Rate. The annualized rate of return that would produce the same total return if compounded each year. The standard metric for comparing returns across strategies with different time horizons."
}

func (cagrMetric) Compute(a *Account, window *Period) (float64, error) {
	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	eqTimes := windowSliceTimes(a.EquityTimes(), window)

	if len(eq) < 2 || len(eqTimes) < 2 {
		return 0, nil
	}

	years := eqTimes[len(eqTimes)-1].Sub(eqTimes[0]).Hours() / 24 / 365.25
	return cagr(eq[0], eq[len(eq)-1], years), nil
}

func (cagrMetric) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// CAGR is the Compound Annual Growth Rate -- the annualized return
// that accounts for compounding. It is the standard way to compare
// returns across different time horizons.
var CAGR PerformanceMetric = cagrMetric{}
