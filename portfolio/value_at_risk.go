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

import (
	"math"
	"sort"
)

type valueAtRisk struct{}

func (valueAtRisk) Name() string { return "ValueAtRisk" }

func (valueAtRisk) Compute(a *Account, window *Period) float64 {
	equity := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(equity)
	if len(r) == 0 {
		return 0
	}

	sorted := make([]float64, len(r))
	copy(sorted, r)
	sort.Float64s(sorted)

	idx := int(math.Floor(0.05 * float64(len(sorted))))
	return sorted[idx]
}

func (valueAtRisk) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// ValueAtRisk estimates the maximum expected loss over a given time
// horizon at a specified confidence level (e.g., 95%). A VaR of 5%
// at 95% confidence means there is a 5% chance the portfolio loses
// more than 5% in the period.
var ValueAtRisk PerformanceMetric = valueAtRisk{}
