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
)

// afterTaxEquity returns a copy of equity adjusted for cumulative realized
// capital-gains taxes within the inclusive [start, end] window. The
// returned slice is parallel to times.
//
// FIFO lot matching (replayGainEvents) replays the full transaction log so
// that opens before start still build up cost basis for closes inside the
// window. Realized gains from in-window closes are taxed at rates.STCG
// (held <= 365 days, and all short-sale gains) or rates.LTCG (held > 365
// days) and accumulated. The cumulative tax at or before each timestamp is
// subtracted from the matching equity point.
//
// Opens, closes outside the window, and dividends contribute zero tax:
// this mirrors TaxDrag, which explicitly excludes dividend taxation. A
// zero start or end disables the bound on that side.
func afterTaxEquity(equity []float64, times []time.Time, txns []Transaction, start, end time.Time, rates taxRates) []float64 {
	if len(equity) == 0 || len(times) != len(equity) {
		return nil
	}

	type taxEvent struct {
		date   time.Time
		amount float64
	}

	gains, _, _ := replayGainEvents(txns, start, end)

	events := make([]taxEvent, 0, len(gains))

	for _, gain := range gains {
		tax := 0.0
		if gain.stcg > 0 {
			tax += rates.STCG * gain.stcg
		}

		if gain.ltcg > 0 {
			tax += rates.LTCG * gain.ltcg
		}

		if tax > 0 {
			events = append(events, taxEvent{date: gain.date, amount: tax})
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
