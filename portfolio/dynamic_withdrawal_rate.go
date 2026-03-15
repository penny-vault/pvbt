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

import "github.com/penny-vault/pvbt/data"

type dynamicWithdrawalRate struct{}

func (dynamicWithdrawalRate) Name() string { return "DynamicWithdrawalRate" }

func (dynamicWithdrawalRate) Description() string {
	return "A withdrawal rate that adjusts based on current portfolio value rather than initial value. Increases withdrawals when the portfolio grows and decreases them during drawdowns, providing a balance between income and capital preservation."
}

func (dynamicWithdrawalRate) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}

	equity := pd.Window(window).Column(portfolioAsset, data.PortfolioEquity)
	if len(equity) < 12 {
		return 0, nil
	}

	monthly := monthlyReturnsFromEquity(equity)
	if len(monthly) == 0 {
		return 0, nil
	}

	// Dynamic withdrawal: balance must never reach zero, but withdrawal
	// adapts downward when portfolio drops.
	criterion := func(rate, startBalance, endBalance float64) bool {
		return true // survival is checked in-loop
	}

	return withdrawalSimulation(monthly, 30, 500, 0.95, criterion, true), nil
}

func (dynamicWithdrawalRate) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	return nil, nil
}

// DynamicWithdrawalRate is the maximum annual withdrawal rate using
// dynamic adjustments: each year's withdrawal is the lesser of the
// inflation-adjusted initial withdrawal and the current balance
// times the rate. This adapts spending to portfolio performance.
var DynamicWithdrawalRate PerformanceMetric = dynamicWithdrawalRate{}
