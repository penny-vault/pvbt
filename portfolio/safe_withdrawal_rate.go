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

type safeWithdrawalRate struct{}

func (safeWithdrawalRate) Name() string { return "SafeWithdrawalRate" }

func (safeWithdrawalRate) Description() string {
	return "The maximum percentage of the initial portfolio that can be withdrawn annually (inflation-adjusted) without depleting the portfolio over the actual return path."
}

func (safeWithdrawalRate) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}

	windowed := pd.Window(window)
	equity := windowed.Column(portfolioAsset, data.PortfolioEquity)
	times := windowed.Times()

	// Need at least 1 year of data for a meaningful withdrawal rate.
	if len(equity) < 2 || len(times) < 2 {
		return 0, nil
	}

	calendarDays := times[len(times)-1].Sub(times[0]).Hours() / 24
	if calendarDays < 365 {
		return 0, nil
	}

	// Survival criterion: balance never reaches zero (checked in-loop).
	criterion := func(startBalance, endBalance, inflationFactor float64) bool {
		return true
	}

	// Linear scan from 0.1% to 20%.
	bestRate := 0.0

	for rateBps := 1; rateBps <= 200; rateBps++ {
		rate := float64(rateBps) / 1000.0
		if withdrawalSustainable(equity, times, rate, false, criterion) {
			bestRate = rate
		} else {
			break
		}
	}

	return bestRate, nil
}

func (safeWithdrawalRate) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	return nil, nil
}

// SafeWithdrawalRate is the maximum constant annual withdrawal rate
// (as a percentage of initial balance) where the portfolio balance
// never reaches zero over the actual backtest period.
var SafeWithdrawalRate PerformanceMetric = safeWithdrawalRate{}
