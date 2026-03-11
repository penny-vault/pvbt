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

type exposure struct{}

func (exposure) Name() string { return "Exposure" }

func (exposure) Description() string {
	return "Fraction of periods where the portfolio had non-zero returns, indicating time invested in the market. A value of 1.0 means always invested; 0.5 means invested half the time. Essential for strategies that hold cash or go flat between signals."
}

func (exposure) Compute(a *Account, window *Period) float64 {
	equity := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(equity)
	if len(r) == 0 {
		return 0
	}

	active := 0
	for _, v := range r {
		if v != 0 {
			active++
		}
	}

	return float64(active) / float64(len(r))
}

func (exposure) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// Exposure measures the fraction of periods where the portfolio had
// non-zero returns. For strategies that go to cash between signals,
// this indicates what percentage of time was spent invested.
var Exposure PerformanceMetric = exposure{}
