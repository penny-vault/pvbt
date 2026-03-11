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

type gainLossRatio struct{}

func (gainLossRatio) Name() string { return "GainLossRatio" }

func (gainLossRatio) Description() string {
	return "Average gain on winning periods divided by average loss on losing periods, computed from equity curve period returns. A ratio above 1.0 means gains are larger than losses on average. Reflects overall portfolio behavior including cash drag, dividends, and unrealized P&L. Compare with TradeGainLossRatio which uses round-trip trade PnL instead."
}

func (gainLossRatio) Compute(a *Account, window *Period) float64 {
	prices := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(prices)

	var positive, negative []float64
	for _, v := range r {
		if v > 0 {
			positive = append(positive, v)
		} else if v < 0 {
			negative = append(negative, v)
		}
	}

	if len(positive) == 0 || len(negative) == 0 {
		return 0
	}

	return mean(positive) / math.Abs(mean(negative))
}

func (gainLossRatio) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// GainLossRatio is the average gain on winning periods divided by the
// average loss on losing periods. A ratio above 1.0 means wins are
// larger than losses on average. Combined with NPositivePeriods, this
// gives a complete picture of the win/loss profile.
var GainLossRatio PerformanceMetric = gainLossRatio{}
