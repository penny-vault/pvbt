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

// TradeDetail represents a completed round-trip trade with per-trade
// excursion data. MFE and MAE are expressed as percentages of the entry
// price: MFE >= 0 (best favorable move) and MAE <= 0 (worst adverse move).
type TradeDetail struct {
	Asset      asset.Asset
	EntryDate  time.Time
	ExitDate   time.Time
	EntryPrice float64
	ExitPrice  float64
	Qty        float64
	PnL        float64
	HoldDays   float64
	MFE        float64
	MAE        float64
}
