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

package asset

import "fmt"

// TransactionType identifies the kind of portfolio or broker event.
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

	// SplitTransaction records a stock split adjustment.
	SplitTransaction

	// InterestTransaction records interest earned or charged.
	InterestTransaction

	// JournalTransaction records an internal transfer between accounts.
	JournalTransaction
)

// String returns a human-readable name for the transaction type.
func (tt TransactionType) String() string {
	switch tt {
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
	case SplitTransaction:
		return "Split"
	case InterestTransaction:
		return "Interest"
	case JournalTransaction:
		return "Journal"
	default:
		return fmt.Sprintf("TransactionType(%d)", int(tt))
	}
}
