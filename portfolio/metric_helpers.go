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
	"math"
	"time"
)

// defaultInflationRate is the assumed annual inflation rate for withdrawal
// metric calculations.
const defaultInflationRate = 0.03

// withdrawalSustainable tests whether a given annual withdrawal rate is
// sustainable over the actual return path represented by the equity curve.
//
// Parameters:
//   - equity: daily equity curve (absolute values)
//   - times: corresponding timestamps
//   - rate: annual withdrawal rate as a fraction (e.g. 0.04 for 4%)
//   - dynamic: if true, each year's withdrawal is min(inflated initial, balance*rate)
//   - criterion: checks final balance for success (called only if portfolio survived)
func withdrawalSustainable(
	equity []float64,
	times []time.Time,
	rate float64,
	dynamic bool,
	criterion func(startBalance, endBalance, inflationFactor float64) bool,
) bool {
	if len(equity) < 2 || len(times) < 2 {
		return false
	}

	startBalance := equity[0]
	balance := startBalance
	startDate := times[0]
	yearBoundary := startDate.AddDate(1, 0, 0)
	yearsElapsed := 0

	for dayIdx := 1; dayIdx < len(equity); dayIdx++ {
		// Apply daily return.
		if equity[dayIdx-1] > 0 {
			dailyReturn := (equity[dayIdx] - equity[dayIdx-1]) / equity[dayIdx-1]
			balance *= (1 + dailyReturn)
		}

		// Check for year boundary.
		if !times[dayIdx].Before(yearBoundary) {
			yearsElapsed++
			inflationFactor := math.Pow(1+defaultInflationRate, float64(yearsElapsed))
			withdrawal := rate * startBalance * inflationFactor

			if dynamic {
				currentRateWithdrawal := balance * rate
				if currentRateWithdrawal < withdrawal {
					withdrawal = currentRateWithdrawal
				}
			}

			balance -= withdrawal
			if balance <= 0 {
				return false
			}

			yearBoundary = startDate.AddDate(yearsElapsed+1, 0, 0)
		}
	}

	// Check final criterion.
	totalYears := times[len(times)-1].Sub(times[0]).Hours() / 24 / 365.25
	inflationFactor := math.Pow(1+defaultInflationRate, totalYears)

	return criterion(startBalance, balance, inflationFactor)
}

// annualizationFactor computes the number of observation periods per year
// from the actual timestamps. This avoids hardcoding 252 or 12 and correctly
// handles any schedule frequency, market closures, and holidays.
func annualizationFactor(times []time.Time) float64 {
	if len(times) < 2 {
		return 1
	}

	calendarDays := times[len(times)-1].Sub(times[0]).Hours() / 24
	if calendarDays <= 0 {
		return 1
	}

	years := calendarDays / 365.25

	return float64(len(times)-1) / years
}

// YieldToCumulative converts an annualized yield percentage to the next
// value in a cumulative price-equivalent series. For example, a yield of
// 5.25 (meaning 5.25% annual) produces a daily return of
// (1 + 0.0525)^(1/252) - 1, and the cumulative series grows by that factor.
//
// Pass prevCumulative=0 on the first call; it returns 100.0 as the
// starting value. On subsequent calls, it returns
// prevCumulative * (1 + dailyReturn).
func YieldToCumulative(annualYieldPct, prevCumulative float64) float64 {
	if prevCumulative == 0 {
		return 100.0
	}

	if annualYieldPct <= 0 {
		return prevCumulative
	}

	dailyReturn := math.Pow(1+annualYieldPct/100, 1.0/252) - 1

	return prevCumulative * (1 + dailyReturn)
}

// roundTrip represents a completed buy-sell pair.
type roundTrip struct {
	pnl      float64
	holdDays float64
	mfe      float64
	mae      float64
}

// roundTrips builds round-trip data from TradeDetails.
// It also returns the total sell value for turnover calculation.
func roundTrips(details []TradeDetail, txns []Transaction) ([]roundTrip, float64) {
	trips := make([]roundTrip, len(details))
	for idx, td := range details {
		trips[idx] = roundTrip{
			pnl:      td.PnL,
			holdDays: td.HoldDays,
			mfe:      td.MFE,
			mae:      td.MAE,
		}
	}

	var totalSellValue float64
	for _, txn := range txns {
		if txn.Type == SellTransaction {
			totalSellValue += txn.Price * txn.Qty
		}
	}

	return trips, totalSellValue
}

// realizedGains replays the transaction log with FIFO lot matching to
// compute realized long-term capital gains, short-term capital gains,
// qualified dividend income, and non-qualified dividend income.
func realizedGains(txns []Transaction) (ltcg, stcg, qualDiv, nonQualDiv float64) {
	type lot struct {
		date  time.Time
		qty   float64
		price float64
	}

	lots := make(map[string][]lot) // keyed by CompositeFigi

	for _, txn := range txns {
		key := txn.Asset.CompositeFigi
		switch txn.Type {
		case BuyTransaction:
			lots[key] = append(lots[key], lot{
				date:  txn.Date,
				qty:   txn.Qty,
				price: txn.Price,
			})
		case SellTransaction:
			remaining := txn.Qty
			lotList := lots[key]

			lotIdx := 0
			for lotIdx < len(lotList) && remaining > 0 {
				matched := lotList[lotIdx].qty
				if matched > remaining {
					matched = remaining
				}

				gain := (txn.Price - lotList[lotIdx].price) * matched

				holdingDays := txn.Date.Sub(lotList[lotIdx].date).Hours() / 24
				if holdingDays > 365 {
					ltcg += gain
				} else {
					stcg += gain
				}

				if lotList[lotIdx].qty <= remaining {
					remaining -= lotList[lotIdx].qty
					lotIdx++
				} else {
					lotList[lotIdx].qty -= remaining
					remaining = 0
				}
			}

			lots[key] = lotList[lotIdx:]
		case DividendTransaction:
			if txn.Qualified {
				qualDiv += txn.Amount
			} else {
				nonQualDiv += txn.Amount
			}
		}
	}

	return
}
