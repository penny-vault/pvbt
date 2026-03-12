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

type averageHoldingPeriod struct{}

func (averageHoldingPeriod) Name() string { return "AverageHoldingPeriod" }

func (averageHoldingPeriod) Description() string {
	return "Average number of days positions are held, computed from FIFO-matched round-trip " +
		"trades. Characterizes whether the strategy is short-term (days) or long-term " +
		"(months/years). Useful for setting expectations about rebalance frequency and " +
		"transaction costs."
}

func (averageHoldingPeriod) Compute(a *Account, _ *Period) (float64, error) {
	trips, _ := roundTrips(a.Transactions())
	if len(trips) == 0 {
		return 0, nil
	}

	var sumHold float64
	for _, rt := range trips {
		sumHold += rt.holdDays
	}

	return sumHold / float64(len(trips)), nil
}

func (averageHoldingPeriod) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// AverageHoldingPeriod is the average number of days positions are held,
// computed from FIFO-matched round-trip trades.
var AverageHoldingPeriod PerformanceMetric = averageHoldingPeriod{}
