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

import "sort"

// LotSelection determines which tax lots are consumed when selling a position.
type LotSelection int

const (
	// LotFIFO sells the earliest-acquired lots first (default).
	LotFIFO LotSelection = iota
	// LotLIFO sells the most-recently-acquired lots first.
	LotLIFO
	// LotHighestCost sells the lot with the highest cost basis first,
	// producing the largest realized loss when the position is underwater.
	LotHighestCost
	// LotSpecificID sells a specific lot identified by ID.
	LotSpecificID
)

// sortLotsByDateAsc sorts lots in ascending date order (earliest first).
func sortLotsByDateAsc(lots []TaxLot) {
	sort.SliceStable(lots, func(i, j int) bool {
		return lots[i].Date.Before(lots[j].Date)
	})
}

// sortLotsByPriceDesc sorts lots in descending price order (highest cost first).
func sortLotsByPriceDesc(lots []TaxLot) {
	sort.SliceStable(lots, func(i, j int) bool {
		return lots[i].Price > lots[j].Price
	})
}

// lotsInConsumptionOrder returns a copy of lots ordered the way the given
// lot-selection method consumes them: FIFO front-first, LIFO back-first,
// HighestCost by descending price. Callers use it to attribute per-lot
// details (entry price/date, PnL, holding period) to the same lots that
// consumeLots/consumeShortLots actually remove.
func lotsInConsumptionOrder(lots []TaxLot, method LotSelection) []TaxLot {
	ordered := make([]TaxLot, len(lots))
	copy(ordered, lots)

	switch method {
	case LotLIFO:
		for ii, jj := 0, len(ordered)-1; ii < jj; ii, jj = ii+1, jj-1 {
			ordered[ii], ordered[jj] = ordered[jj], ordered[ii]
		}
	case LotHighestCost:
		sortLotsByPriceDesc(ordered)
	default: // LotFIFO: already in date-ascending order.
	}

	return ordered
}
