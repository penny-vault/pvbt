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

	"github.com/penny-vault/pvbt/data"
)

// UpdateExcursions reads the daily High and Low prices from the DataFrame
// and updates the running extremes for each open position. If High or Low
// is NaN for an asset, that asset is skipped for that day.
func (acct *Account) UpdateExcursions(df *data.DataFrame) {
	for held := range acct.excursions {
		highPrice := df.Value(held, data.MetricHigh)
		lowPrice := df.Value(held, data.MetricLow)

		record := acct.excursions[held]

		if !math.IsNaN(highPrice) && highPrice > record.HighPrice {
			record.HighPrice = highPrice
		}

		if !math.IsNaN(lowPrice) && lowPrice < record.LowPrice {
			record.LowPrice = lowPrice
		}

		acct.excursions[held] = record
	}
}

// ExcursionRecord tracks the running price extremes for an open position.
// EntryPrice is the fill price of the first buy that opened the position.
// HighPrice tracks the running maximum of daily High prices observed while
// the position is open. LowPrice tracks the running minimum of daily Low
// prices. These are used to compute MFE and MAE when the position closes.
type ExcursionRecord struct {
	EntryPrice float64
	HighPrice  float64
	LowPrice   float64
}
