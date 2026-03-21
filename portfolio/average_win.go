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

type averageWin struct{}

func (averageWin) Name() string { return "AverageWin" }

func (averageWin) Description() string {
	return "Average profit in currency units on winning round-trip trades, computed from " +
		"FIFO-matched buy/sell pairs. Higher is better. Because the value depends on position " +
		"sizing, compare within the same portfolio rather than across different ones."
}

func (averageWin) Compute(a *Account, _ *Period) (float64, error) {
	trips, _ := roundTrips(a.TradeDetails(), a.Transactions())

	var (
		wins   int
		sumWin float64
	)

	for _, rt := range trips {
		if rt.pnl > 0 {
			wins++
			sumWin += rt.pnl
		}
	}

	if wins == 0 {
		return 0, nil
	}

	return sumWin / float64(wins), nil
}

func (averageWin) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// AverageWin is the average profit on winning round-trip trades.
var AverageWin PerformanceMetric = averageWin{}
