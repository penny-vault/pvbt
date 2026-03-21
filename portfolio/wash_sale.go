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

// WashSaleRecord tracks a wash sale event where a loss was disallowed
// because the same asset was repurchased within 30 calendar days.
type WashSaleRecord struct {
	Asset          asset.Asset
	SellDate       time.Time
	RebuyDate      time.Time
	DisallowedLoss float64
	AdjustedLotID  string
}

// recentLossSale tracks a loss-generating sell for wash sale window checking.
type recentLossSale struct {
	date         time.Time
	lossPerShare float64
	qty          float64
}

// recentBuy tracks a recent buy for the reverse wash sale direction.
type recentBuy struct {
	date  time.Time
	lotID string
	qty   float64
}

const washSaleWindowDays = 30
