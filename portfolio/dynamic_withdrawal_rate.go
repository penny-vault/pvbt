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
	"context"

	"github.com/penny-vault/pvbt/data"
)

type dynamicWithdrawalRate struct{}

func (dynamicWithdrawalRate) Name() string { return "DynamicWithdrawalRate" }

func (dynamicWithdrawalRate) Description() string {
	return "A withdrawal rate that adjusts based on current portfolio value over the actual return path. Increases withdrawals when the portfolio grows and decreases them during drawdowns, providing a balance between income and capital preservation."
}

func (dynamicWithdrawalRate) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, nil
	}

	equity := df.Column(portfolioAsset, data.PortfolioEquity)
	times := df.Times()

	if len(equity) < 2 || len(times) < 2 {
		return 0, nil
	}

	calendarDays := times[len(times)-1].Sub(times[0]).Hours() / 24
	if calendarDays < 365 {
		return 0, nil
	}

	// Dynamic: survival checked in-loop; withdrawal adapts downward.
	criterion := func(startBalance, endBalance, inflationFactor float64) bool {
		return true
	}

	bestRate := 0.0

	for rateBps := 1; rateBps <= 200; rateBps++ {
		rate := float64(rateBps) / 1000.0
		if withdrawalSustainable(equity, times, rate, true, criterion) {
			bestRate = rate
		} else {
			break
		}
	}

	return bestRate, nil
}

func (dynamicWithdrawalRate) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// DynamicWithdrawalRate is the maximum annual withdrawal rate using
// dynamic adjustments: each year's withdrawal is the lesser of the
// inflation-adjusted initial withdrawal and the current balance
// times the rate. This adapts spending to portfolio performance.
var DynamicWithdrawalRate PerformanceMetric = dynamicWithdrawalRate{}
