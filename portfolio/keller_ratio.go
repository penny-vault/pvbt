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

type kellerRatio struct{}

func (kellerRatio) Name() string { return "KellerRatio" }

func (kellerRatio) Description() string {
	return "Combines return and risk into a single measure using the formula (CAGR * (1 + CAGR)) / max_drawdown. Rewards high compound growth while penalizing drawdowns. Higher values indicate better risk-adjusted compound returns."
}

func (kellerRatio) Compute(a *Account, window *Period) (float64, error) {
	equity := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	if len(equity) < 2 {
		return 0, nil
	}

	totalReturn := (equity[len(equity)-1] / equity[0]) - 1
	dd := drawdownSeries(equity)

	// Find max drawdown as a positive number (abs of most negative drawdown).
	minDD := 0.0
	for _, d := range dd {
		if d < minDD {
			minDD = d
		}
	}
	maxDD := math.Abs(minDD)

	if totalReturn >= 0 && maxDD <= 0.5 {
		return totalReturn * (1 - maxDD/(1-maxDD)), nil
	}

	return 0, nil
}

func (kellerRatio) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// KellerRatio adjusts return for drawdown severity:
// K = R * (1 - D/(1-D)) when R >= 0 and D <= 50%, else 0.
// Small drawdowns have limited impact; large drawdowns amplify the
// penalty, making this a useful risk-adjusted measure.
var KellerRatio PerformanceMetric = kellerRatio{}
