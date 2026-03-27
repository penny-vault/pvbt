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
	"github.com/penny-vault/pvbt/data"
)

// TaxLot tracks the purchase date, quantity, and price of a position for
// tax gain/loss calculations.
type TaxLot struct {
	ID    string
	Date  time.Time
	Qty   float64
	Price float64
}

// PortfolioSnapshot exposes the state needed to reconstruct a portfolio
// from a previous run. Account implements this interface. The engine
// uses it with WithPortfolioSnapshot to resume from saved state.
type PortfolioSnapshot interface {
	Cash() float64
	Holdings() map[asset.Asset]float64
	Transactions() []Transaction
	PerfData() *data.DataFrame
	TaxLots() map[asset.Asset][]TaxLot
	Metrics() []MetricRow
	AllMetadata() map[string]string
	TradeDetails() []TradeDetail
	Excursions() map[asset.Asset]ExcursionRecord
	// ShortLots iterates over all open short tax lots grouped by asset.
	ShortLots(fn func(asset.Asset, []TaxLot))
}

// WithPortfolioSnapshot returns an Option that restores an Account from
// a previous snapshot. This is used by the engine to resume from saved state.
func WithPortfolioSnapshot(snap PortfolioSnapshot) Option {
	return func(acct *Account) {
		acct.cash = snap.Cash()
		for ast, qty := range snap.Holdings() {
			acct.holdings[ast] = qty
		}

		acct.transactions = append(acct.transactions, snap.Transactions()...)
		if snap.PerfData() != nil {
			acct.perfData = snap.PerfData().Copy()
		}

		for ast, lots := range snap.TaxLots() {
			acct.taxLots[ast] = append(acct.taxLots[ast], lots...)
		}

		acct.metrics = append(acct.metrics, snap.Metrics()...)
		for k, v := range snap.AllMetadata() {
			acct.metadata[k] = v
		}

		acct.tradeDetails = append(acct.tradeDetails, snap.TradeDetails()...)
		for ast, rec := range snap.Excursions() {
			acct.excursions[ast] = rec
		}

		snap.ShortLots(func(ast asset.Asset, lots []TaxLot) {
			acct.shortLots[ast] = append(acct.shortLots[ast], lots...)
		})
	}
}
