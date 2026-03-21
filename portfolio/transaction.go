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
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// String returns a human-readable name for the transaction type.
func (t TransactionType) String() string {
	switch t {
	case BuyTransaction:
		return "Buy"
	case SellTransaction:
		return "Sell"
	case DividendTransaction:
		return "Dividend"
	case FeeTransaction:
		return "Fee"
	case DepositTransaction:
		return "Deposit"
	case WithdrawalTransaction:
		return "Withdrawal"
	default:
		return fmt.Sprintf("TransactionType(%d)", int(t))
	}
}

// TransactionType identifies the kind of portfolio event recorded in the
// transaction log.
type TransactionType int

const (
	// BuyTransaction records a purchase of an asset.
	BuyTransaction TransactionType = iota

	// SellTransaction records a sale of an asset.
	SellTransaction

	// DividendTransaction records a dividend payment received.
	DividendTransaction

	// FeeTransaction records a fee or commission charged.
	FeeTransaction

	// DepositTransaction records cash added to the portfolio.
	DepositTransaction

	// WithdrawalTransaction records cash removed from the portfolio.
	WithdrawalTransaction
)

// Transaction is a single entry in the portfolio's transaction log. Every
// event that changes the portfolio's cash balance or holdings produces a
// transaction: trades, dividends, fees, deposits, and withdrawals.
type Transaction struct {
	// Date is when the transaction occurred.
	Date time.Time

	// Asset is the asset involved. Nil for cash-only events like
	// deposits, withdrawals, and account-level fees.
	Asset asset.Asset

	// Type identifies the kind of event.
	Type TransactionType

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
	LotSelection int
}
