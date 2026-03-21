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

type profitFactor struct{}

func (profitFactor) Name() string { return "ProfitFactor" }

func (profitFactor) Description() string {
	return "Gross profit divided by gross loss from FIFO-matched round-trip trades. A value " +
		"above 1.0 means the strategy is profitable overall; below 1.0 means it is losing " +
		"money. A value of 2.0 means the strategy made twice as much on winners as it lost " +
		"on losers."
}

func (profitFactor) Compute(a *Account, _ *Period) (float64, error) {
	trips, _ := roundTrips(a.TradeDetails(), a.Transactions())

	var sumWin, sumLoss float64

	for _, rt := range trips {
		if rt.pnl > 0 {
			sumWin += rt.pnl
		} else {
			sumLoss += rt.pnl
		}
	}

	if sumLoss == 0 {
		return math.NaN(), nil
	}

	return sumWin / math.Abs(sumLoss), nil
}

func (profitFactor) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// ProfitFactor is the ratio of gross profit to gross loss from round-trip trades.
var ProfitFactor PerformanceMetric = profitFactor{}
