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
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// afterTaxEquity returns a copy of equity adjusted for cumulative realized
// capital-gains taxes within the inclusive [start, end] window. The
// returned slice is parallel to times.
//
// FIFO lot matching replays the full transaction log so that buys before
// start still build up cost basis for sells inside the window. Realized
// gains from in-window sells are taxed at rates.STCG (held <= 365 days)
// or rates.LTCG (held > 365 days) and accumulated. The cumulative tax at
// or before each timestamp is subtracted from the matching equity point.
//
// Buys, sells outside the window, and dividends contribute zero tax: this
// mirrors TaxDrag, which explicitly excludes dividend taxation. A zero
// start or end disables the bound on that side.
func afterTaxEquity(equity []float64, times []time.Time, txns []Transaction, start, end time.Time, rates taxRates) []float64 {
	if len(equity) == 0 || len(times) != len(equity) {
		return nil
	}

	type lot struct {
		date  time.Time
		qty   float64
		price float64
	}

	type taxEvent struct {
		date   time.Time
		amount float64
	}

	inRange := func(date time.Time) bool {
		if !start.IsZero() && date.Before(start) {
			return false
		}

		if !end.IsZero() && date.After(end) {
			return false
		}

		return true
	}

	lots := make(map[string][]lot)

	var events []taxEvent

	for _, txn := range txns {
		key := txn.Asset.CompositeFigi

		switch txn.Type {
		case asset.BuyTransaction:
			lots[key] = append(lots[key], lot{
				date:  txn.Date,
				qty:   txn.Qty,
				price: txn.Price,
			})
		case asset.SellTransaction:
			remaining := txn.Qty
			lotList := lots[key]
			attribute := inRange(txn.Date)

			var (
				ltcg float64
				stcg float64
			)

			lotIdx := 0
			for lotIdx < len(lotList) && remaining > 0 {
				matched := lotList[lotIdx].qty
				if matched > remaining {
					matched = remaining
				}

				if attribute {
					gain := (txn.Price - lotList[lotIdx].price) * matched

					holdingDays := txn.Date.Sub(lotList[lotIdx].date).Hours() / 24
					if holdingDays > 365 {
						ltcg += gain
					} else {
						stcg += gain
					}
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

			if !attribute {
				continue
			}

			tax := 0.0
			if stcg > 0 {
				tax += rates.STCG * stcg
			}

			if ltcg > 0 {
				tax += rates.LTCG * ltcg
			}

			if tax > 0 {
				events = append(events, taxEvent{date: txn.Date, amount: tax})
			}
		}
	}

	after := make([]float64, len(equity))
	cumTax := 0.0
	eventIdx := 0

	for ii, ts := range times {
		for eventIdx < len(events) && !events[eventIdx].date.After(ts) {
			cumTax += events[eventIdx].amount
			eventIdx++
		}

		after[ii] = equity[ii] - cumTax
	}

	return after
}
