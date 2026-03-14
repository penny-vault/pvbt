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

import "time"

// annualizationFactor estimates the number of periods per year from timestamps.
// If the average gap between timestamps exceeds 20 calendar days, it assumes
// monthly data (factor 12); otherwise it assumes daily (factor 252).
func annualizationFactor(times []time.Time) float64 {
	if len(times) < 2 {
		return 252 // default daily
	}
	avgDays := times[len(times)-1].Sub(times[0]).Hours() / 24 / float64(len(times)-1)
	if avgDays > 20 {
		return 12 // monthly
	}
	return 252 // daily
}

// roundTrip represents a completed buy-sell pair matched via FIFO.
type roundTrip struct {
	pnl      float64
	holdDays float64
}

// roundTrips builds round-trip trades from transactions using FIFO matching.
// It also returns the total sell value for turnover calculation.
func roundTrips(txns []Transaction) ([]roundTrip, float64) {
	type openLot struct {
		date  time.Time
		qty   float64
		price float64
	}

	openLots := make(map[string][]openLot) // keyed by CompositeFigi
	var trips []roundTrip
	var totalSellValue float64

	for _, tx := range txns {
		key := tx.Asset.CompositeFigi
		switch tx.Type {
		case BuyTransaction:
			openLots[key] = append(openLots[key], openLot{
				date:  tx.Date,
				qty:   tx.Qty,
				price: tx.Price,
			})
		case SellTransaction:
			totalSellValue += tx.Price * tx.Qty
			remaining := tx.Qty
			lots := openLots[key]
			for len(lots) > 0 && remaining > 0 {
				matched := lots[0].qty
				if matched > remaining {
					matched = remaining
				}
				pnl := (tx.Price - lots[0].price) * matched
				days := tx.Date.Sub(lots[0].date).Hours() / 24.0
				trips = append(trips, roundTrip{pnl: pnl, holdDays: days})

				lots[0].qty -= matched
				remaining -= matched
				if lots[0].qty == 0 {
					lots = lots[1:]
				}
			}
			openLots[key] = lots
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

	for _, tx := range txns {
		key := tx.Asset.CompositeFigi
		switch tx.Type {
		case BuyTransaction:
			lots[key] = append(lots[key], lot{
				date:  tx.Date,
				qty:   tx.Qty,
				price: tx.Price,
			})
		case SellTransaction:
			remaining := tx.Qty
			ll := lots[key]
			i := 0
			for i < len(ll) && remaining > 0 {
				matched := ll[i].qty
				if matched > remaining {
					matched = remaining
				}
				gain := (tx.Price - ll[i].price) * matched
				holdingDays := tx.Date.Sub(ll[i].date).Hours() / 24
				if holdingDays > 365 {
					ltcg += gain
				} else {
					stcg += gain
				}
				if ll[i].qty <= remaining {
					remaining -= ll[i].qty
					i++
				} else {
					ll[i].qty -= remaining
					remaining = 0
				}
			}
			lots[key] = ll[i:]
		case DividendTransaction:
			if tx.Qualified {
				qualDiv += tx.Amount
			} else {
				nonQualDiv += tx.Amount
			}
		}
	}

	return
}
