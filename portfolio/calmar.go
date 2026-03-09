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

type calmar struct{}

func (calmar) Name() string { return "Calmar" }

func (calmar) Compute(a *Account, window *Period) float64 {
	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	eqTimes := windowSliceTimes(a.EquityTimes(), window)

	if len(eq) < 2 || len(eqTimes) < 2 {
		return 0
	}

	years := eqTimes[len(eqTimes)-1].Sub(eqTimes[0]).Hours() / 24 / 365.25
	if years <= 0 {
		return 0
	}

	annualizedReturn := cagr(eq[0], eq[len(eq)-1], years)

	dd := drawdownSeries(eq)
	minDD := 0.0
	for _, v := range dd {
		if v < minDD {
			minDD = v
		}
	}

	if minDD == 0 {
		return 0
	}

	return annualizedReturn / math.Abs(minDD)
}

func (calmar) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// Calmar is the Calmar ratio: annualized return divided by maximum drawdown.
var Calmar PerformanceMetric = calmar{}
