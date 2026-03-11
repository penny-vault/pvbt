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

type perpetualWithdrawalRate struct{}

func (perpetualWithdrawalRate) Name() string { return "PerpetualWithdrawalRate" }

func (perpetualWithdrawalRate) Description() string {
	return "The withdrawal rate that preserves the real (inflation-adjusted) value of the portfolio indefinitely. More conservative than SafeWithdrawalRate as it aims to maintain purchasing power."
}

func (perpetualWithdrawalRate) Compute(a *Account, window *Period) float64 {
	equity := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	if len(equity) < 12 {
		return 0
	}

	monthly := monthlyReturnsFromEquity(equity)
	if len(monthly) == 0 {
		return 0
	}

	// Perpetual withdrawal: ending balance must be >= starting balance.
	criterion := func(rate, startBalance, endBalance float64) bool {
		return endBalance >= startBalance
	}

	return withdrawalSimulation(monthly, 30, 500, 0.95, criterion, false)
}

func (perpetualWithdrawalRate) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// PerpetualWithdrawalRate is the maximum constant annual withdrawal
// rate where the ending balance equals or exceeds the inflation-
// adjusted starting balance. This ensures the portfolio maintains
// its real purchasing power indefinitely.
var PerpetualWithdrawalRate PerformanceMetric = perpetualWithdrawalRate{}
