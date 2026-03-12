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

type ulcerIndex struct{}

func (ulcerIndex) Name() string { return "UlcerIndex" }

func (ulcerIndex) Description() string {
	return "Measures downside risk using both the depth and duration of drawdowns over a 14-period lookback window. Computed as the square root of the mean of squared percentage drawdowns. Values are on a 0-100 percentage scale. Higher values indicate more painful drawdown experiences. Returns 0 when fewer than 14 data points are available."
}

func (ulcerIndex) Compute(a *Account, window *Period) (float64, error) {
	equity := windowSlice(a.EquityCurve(), a.EquityTimes(), window)

	const lookback = 14

	if len(equity) < lookback {
		return 0, nil
	}

	// Use the last 14 periods. Within that window, track the rolling
	// peak and compute percentage drawdowns -- matching the standard
	// Ulcer Index definition.
	tail := equity[len(equity)-lookback:]
	peak := tail[0]
	sumSq := 0.0

	for _, v := range tail {
		if v > peak {
			peak = v
		}
		dd := (v - peak) / peak * 100 // percentage scale
		sumSq += dd * dd
	}

	return math.Sqrt(sumSq / float64(lookback)), nil
}

func (ulcerIndex) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// UlcerIndex measures downside risk using both the depth and duration
// of drawdowns. Computed as the square root of the mean of squared
// percentage drawdowns over a 14-period lookback window. Values are
// on a 0-100 percentage scale. Higher values indicate more painful
// drawdown experiences. Returns 0 when fewer than 14 data points
// are available.
var UlcerIndex PerformanceMetric = ulcerIndex{}
