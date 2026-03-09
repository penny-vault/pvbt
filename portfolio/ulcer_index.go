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

func (ulcerIndex) Compute(a *Account, window *Period) float64 {
	equity := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	if len(equity) < 2 {
		return 0
	}

	dd := drawdownSeries(equity)
	sumSq := 0.0
	for _, d := range dd {
		sumSq += d * d
	}

	return math.Sqrt(sumSq / float64(len(dd)))
}

func (ulcerIndex) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// UlcerIndex measures downside risk using both the depth and duration
// of drawdowns. Computed as the square root of the mean of squared
// percentage drawdowns over a 14-day lookback. Higher values indicate
// more painful drawdown experiences.
var UlcerIndex = ulcerIndex{}
