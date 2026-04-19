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

// Transaction is a single entry in the portfolio's transaction log. Every
// event that changes the portfolio's cash balance or holdings produces a
// transaction: trades, dividends, fees, deposits, and withdrawals.
type Transaction struct {
	// ID uniquely identifies this transaction for deduplication. Set when
	// the transaction originates from a broker sync. Empty for transactions
	// created directly by the engine (buys, sells from strategy execution).
	ID string

	// Date is when the transaction occurred.
	Date time.Time

	// Asset is the asset involved. Nil for cash-only events like
	// deposits, withdrawals, and account-level fees.
	Asset asset.Asset

	// Type identifies the kind of event.
	Type asset.TransactionType

	// Qty is the number of shares or units involved. Zero for cash-only
	// events like fees, deposits, and withdrawals.
	Qty float64

	// Price is the per-share price at which a trade was executed. Zero
	// for non-trade events.
	Price float64

	// Amount is the total cash impact of the transaction. Positive for
	// cash inflows (sells, dividends, deposits), negative for cash
	// outflows (buys, fees, withdrawals).
	Amount float64

	// Qualified indicates whether a dividend meets the IRS holding
	// period requirement for preferential tax rates. Only meaningful
	// for DividendTransaction. Set automatically by Record() based
	// on whether the position was held for more than 60 days before
	// the dividend date.
	Qualified bool

	// Justification is an optional explanation of why this trade was made.
	// Set automatically from the Allocation's Justification field during
	// RebalanceTo, or from the WithJustification OrderModifier during Order.
	Justification string

	// LotSelection overrides the account-level default lot selection method
	// for this specific sell transaction. Zero (LotFIFO) means use the
	// account default. Set automatically from broker.Order.LotSelection
	// when a fill is recorded via submitAndRecord.
	LotSelection LotSelection

	// BatchID is the portfolio batch that produced this transaction.
	// Zero means the transaction was recorded outside any batch
	// (deposits, withdrawals, stock splits, manual Record calls).
	// Batch IDs start at 1 and increment monotonically within an Account.
	BatchID int
}
