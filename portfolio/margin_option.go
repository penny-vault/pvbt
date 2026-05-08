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

// WithInitialMargin returns an Option that sets the initial margin rate
// required when opening a short position. The default is 0.50 (50%).
func WithInitialMargin(rate float64) Option {
	return func(a *Account) {
		a.initialMargin = rate
	}
}

// WithMaintenanceMargin returns an Option that sets the maintenance
// margin rate. If equity falls below this fraction of short market
// value, a margin call is triggered. The default is 0.30 (30%).
func WithMaintenanceMargin(rate float64) Option {
	return func(a *Account) {
		a.maintenanceMargin = rate
	}
}

// WithBorrowRate returns an Option that sets the annualized borrow rate
// charged on short positions.
func WithBorrowRate(rate float64) Option {
	return func(a *Account) {
		a.borrowRate = rate
	}
}

// WithMaxLeverage returns an Option that caps gross leverage,
// (LongMarketValue + ShortMarketValue) / Equity, at order-submission
// time. Orders that would push the account above this ratio are
// rejected; closing trades are always allowed through. The default is
// 1.0, which models a cash account; set to 2.0 for Reg T-style 2x
// initial leverage. Values <= 0 are clamped to the default.
//
// MaxLeverage is an entry-time gate only. Adverse price moves that
// drift gross leverage above the cap do not force liquidation;
// liquidation is driven by short-side maintenance margin (see
// WithMaintenanceMargin) and, when configured, gross maintenance
// leverage (see WithGrossMaintenanceLeverage or WithMarginModel).
func WithMaxLeverage(ratio float64) Option {
	return func(a *Account) {
		a.maxLeverage = ratio
	}
}

// WithGrossMaintenanceLeverage returns an Option that triggers a
// margin call when gross leverage, (LongMarketValue + ShortMarketValue)
// / Equity, exceeds the given ratio. Unlike WithMaxLeverage, this knob
// drives liquidation: when the threshold is breached the engine either
// invokes the strategy's MarginCallHandler or trims gross notional
// proportionally. The default is 0 (disabled), so only short-side
// maintenance margin can force liquidation. Set to 4.0 for Reg
// T-style 25% maintenance.
func WithGrossMaintenanceLeverage(ratio float64) Option {
	return func(a *Account) {
		a.grossMaintenanceLeverage = ratio
	}
}

// RegT describes a Reg T-style margin model: a single initial-margin
// rate that gates new orders and a single maintenance rate that
// triggers liquidation. Both rates are expressed as
// equity / position-value fractions, so the canonical Reg T values
// are Initial: 0.50 and Maintenance: 0.25. The corresponding leverage
// caps are 1/Initial (entry) and 1/Maintenance (liquidation). Zero
// values leave the corresponding setting at its existing default.
type RegT struct {
	Initial     float64
	Maintenance float64
}

// WithMarginModel applies a packaged margin model to the account.
// For RegT, this sets MaxLeverage = 1/Initial and gross maintenance
// leverage = 1/Maintenance. Short-side maintenance margin
// (WithMaintenanceMargin) is left untouched and continues to apply
// independently.
func WithMarginModel(model RegT) Option {
	return func(acct *Account) {
		if model.Initial > 0 {
			acct.maxLeverage = 1.0 / model.Initial
		}

		if model.Maintenance > 0 {
			acct.grossMaintenanceLeverage = 1.0 / model.Maintenance
		}
	}
}
