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

import "sort"

type cvarMetric struct{}

func (cvarMetric) Name() string { return "CVaR" }

func (cvarMetric) Description() string {
	return "Conditional Value at Risk (Expected Shortfall) at 95% confidence. The average loss in the worst 5% of periods. More negative values indicate higher tail risk. Superior to VaR because it captures the magnitude of extreme losses, not just their threshold."
}

func (cvarMetric) Compute(a *Account, window *Period) (float64, error) {
	equity := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(equity)
	if len(r) == 0 {
		return 0, nil
	}

	sorted := make([]float64, len(r))
	copy(sorted, r)
	sort.Float64s(sorted)

	// Take the worst 5% of returns and average them.
	cutoff := int(0.05 * float64(len(sorted)))
	if cutoff == 0 {
		cutoff = 1
	}

	sum := 0.0
	for i := 0; i < cutoff; i++ {
		sum += sorted[i]
	}

	return sum / float64(cutoff), nil
}

func (cvarMetric) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// CVaR is Conditional Value at Risk (Expected Shortfall) at 95%
// confidence. It measures the average loss in the worst 5% of periods,
// providing a more complete picture of tail risk than VaR alone.
var CVaR PerformanceMetric = cvarMetric{}
