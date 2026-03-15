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
	"math/rand"

	"github.com/penny-vault/pvbt/data"
)

// withdrawalCriterion returns true if a simulation path "succeeded" given the
// withdrawal rate, the starting balance, and the ending balance.
type withdrawalCriterion func(rate, startBalance, endBalance float64) bool

// withdrawalSimulation runs Monte Carlo withdrawal simulations using circular
// block bootstrap of historical returns. It returns the highest annual
// withdrawal rate (as a fraction) that meets the success criterion in at least
// successThreshold fraction of simulations.
//
// Parameters:
//   - monthlyReturns: historical monthly return series to bootstrap from
//   - simYears: number of years to simulate
//   - nSims: number of simulations
//   - successThreshold: fraction of simulations that must succeed
//   - criterion: function that returns true if a simulation path "succeeded"
//   - dynamic: if true, each year's withdrawal is min(inflationAdjustedInitial, balance*rate)
func withdrawalSimulation(
	monthlyReturns []float64,
	simYears int,
	nSims int,
	successThreshold float64,
	criterion withdrawalCriterion,
	dynamic bool,
) float64 {
	if len(monthlyReturns) == 0 {
		return 0
	}

	rng := rand.New(rand.NewSource(42))
	numMonths := len(monthlyReturns)
	blockSize := 12 // one year block for preserving autocorrelation

	// Binary search over withdrawal rates from 0% to 20% in 0.1% increments.
	bestRate := 0.0

	for rateBps := 1; rateBps <= 200; rateBps++ {
		rate := float64(rateBps) / 1000.0 // 0.001 to 0.200

		successes := 0

		for sim := 0; sim < nSims; sim++ {
			balance := 1_000_000.0
			startBalance := balance
			annualWithdrawal := rate * startBalance
			failed := false

			for year := 0; year < simYears; year++ {
				// Withdraw at the start of the year.
				withdrawal := annualWithdrawal

				if dynamic {
					currentRateWithdrawal := balance * rate
					if currentRateWithdrawal < withdrawal {
						withdrawal = currentRateWithdrawal
					}
				}

				balance -= withdrawal
				if balance <= 0 {
					failed = true
					break
				}

				// Apply 12 months of bootstrapped returns.
				startIdx := rng.Intn(numMonths)
				for m := 0; m < blockSize; m++ {
					idx := (startIdx + m) % numMonths

					balance *= (1 + monthlyReturns[idx])
					if balance <= 0 {
						failed = true
						break
					}
				}

				if failed {
					break
				}
			}

			if !failed && criterion(rate, startBalance, balance) {
				successes++
			}
		}

		successRate := float64(successes) / float64(nSims)
		if successRate >= successThreshold {
			bestRate = rate
		} else {
			// Once we fail at this rate, higher rates will also fail.
			break
		}
	}

	return bestRate
}

// monthlyReturnsFromEquity converts a daily equity curve into approximate
// monthly returns by sampling every ~21 trading days.
func monthlyReturnsFromEquity(equity []float64) []float64 {
	const tradingDaysPerMonth = 21

	if len(equity) < tradingDaysPerMonth+1 {
		return nil
	}

	var monthly []float64

	for monthIdx := tradingDaysPerMonth; monthIdx < len(equity); monthIdx += tradingDaysPerMonth {
		prev := equity[monthIdx-tradingDaysPerMonth]
		if prev <= 0 {
			continue
		}

		r := (equity[monthIdx] - prev) / prev
		monthly = append(monthly, r)
	}

	return monthly
}

type safeWithdrawalRate struct{}

func (safeWithdrawalRate) Name() string { return "SafeWithdrawalRate" }

func (safeWithdrawalRate) Description() string {
	return "The maximum percentage of the initial portfolio that can be withdrawn annually without depleting the portfolio over the observed period. Based on the classic \"4% rule\" research methodology."
}

func (safeWithdrawalRate) Compute(a *Account, window *Period) (float64, error) {
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

	// Safe withdrawal: balance must never reach zero.
	// The criterion only needs to confirm non-failure (handled by the
	// simulation loop itself); if we reach here the path survived.
	criterion := func(rate, startBalance, endBalance float64) bool {
		return true // survival is checked in-loop
	}

	return withdrawalSimulation(monthly, 30, 500, 0.95, criterion, false), nil
}

func (safeWithdrawalRate) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	return nil, nil
}

// SafeWithdrawalRate is the maximum constant annual withdrawal rate
// (as a percentage of initial balance) where the portfolio balance
// never reaches zero over the simulation period. Uses circular
// bootstrap Monte Carlo simulation of historical returns.
var SafeWithdrawalRate PerformanceMetric = safeWithdrawalRate{}
