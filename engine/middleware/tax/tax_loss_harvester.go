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

package tax

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// TaxLossHarvester is a middleware that scans portfolio positions for
// unrealized losses exceeding a configurable threshold and injects
// sell orders to realize those losses for tax purposes. When a
// substitute asset is configured, the harvester also injects a buy
// order for the substitute at the same dollar value.
type TaxLossHarvester struct {
	config HarvesterConfig
}

// NewTaxLossHarvester returns a middleware that harvests tax losses
// according to the given configuration.
func NewTaxLossHarvester(config HarvesterConfig) portfolio.Middleware {
	return &TaxLossHarvester{config: config}
}

// washSaleWindowDays is the IRS wash sale look-back period.
const washSaleWindowDays = 30

// Process implements portfolio.Middleware. It scans for harvestable
// losses and injects sell (and optionally substitute buy) orders.
func (h *TaxLossHarvester) Process(ctx context.Context, batch *portfolio.Batch) error {
	taxAware, ok := batch.Portfolio().(portfolio.TaxAware)
	if !ok {
		return nil
	}

	// In gain-offset-only mode, skip harvesting when no realized gains exist.
	if h.config.GainOffsetOnly {
		ltcg, stcg := taxAware.RealizedGainsYTD()
		if ltcg+stcg <= 0 {
			return nil
		}
	}

	// Check for expired substitutions and inject swap-back orders.
	if err := h.swapBackExpiredSubstitutions(ctx, batch, taxAware); err != nil {
		return err
	}

	// Collect held assets.
	var heldAssets []asset.Asset

	for ast, qty := range batch.Portfolio().Holdings() {
		if qty > 0 {
			heldAssets = append(heldAssets, ast)
		}
	}

	for _, held := range heldAssets {
		lots := taxAware.UnrealizedLots(held)
		if len(lots) == 0 {
			continue
		}

		// Compute current price from portfolio position data.
		currentPrice := currentPrice(batch, held)
		if currentPrice <= 0 {
			continue
		}

		// Compute aggregate unrealized loss across all lots.
		var totalCostBasis, totalQty float64

		for _, lot := range lots {
			totalCostBasis += lot.Price * lot.Qty
			totalQty += lot.Qty
		}

		currentValue := totalQty * currentPrice
		unrealizedPnL := currentValue - totalCostBasis

		// Only harvest losses (negative P&L).
		if unrealizedPnL >= 0 {
			continue
		}

		lossPct := -unrealizedPnL / totalCostBasis
		if lossPct < h.config.LossThreshold {
			continue
		}

		// Check wash sale window.
		washRecords := taxAware.WashSaleWindow(held)
		substitute, hasSubstitute := h.config.Substitutes[held]

		if len(washRecords) > 0 && !hasSubstitute {
			// Wash sale risk with no substitute -- skip this position.
			continue
		}

		// Inject the sell order with highest-cost lot selection.
		justification := fmt.Sprintf(
			"tax-loss harvest: %s down %.1f%% (threshold %.1f%%)",
			held.Ticker, lossPct*100, h.config.LossThreshold*100,
		)

		if err := batch.Order(ctx, held, portfolio.Sell, totalQty,
			portfolio.WithLotSelection(portfolio.LotHighestCost),
			portfolio.WithJustification(justification),
		); err != nil {
			return fmt.Errorf("tax loss harvester: sell %s: %w", held.Ticker, err)
		}

		// If a substitute is configured, buy it at the same dollar value
		// and register the substitution.
		if hasSubstitute {
			sellDollars := totalQty * currentPrice

			if err := batch.Order(ctx, substitute, portfolio.Buy, 0,
				portfolio.WithJustification(
					fmt.Sprintf("tax-loss harvest substitute: buy %s to replace %s",
						substitute.Ticker, held.Ticker),
				),
			); err != nil {
				return fmt.Errorf("tax loss harvester: buy substitute %s: %w", substitute.Ticker, err)
			}

			// Set the substitute buy amount via direct order modification
			// since batch.Order with qty=0 needs a dollar amount.
			lastIdx := len(batch.Orders) - 1
			batch.Orders[lastIdx].Amount = sellDollars
			batch.Orders[lastIdx].Qty = 0

			// Register the substitution for wash sale tracking.
			expiryDate := batch.Timestamp.AddDate(0, 0, washSaleWindowDays)
			taxAware.RegisterSubstitution(held, substitute, expiryDate)
		}
	}

	return nil
}

// swapBackExpiredSubstitutions checks for substitutions that have passed
// their wash sale window and injects orders to sell the substitute and
// buy back the original.
func (h *TaxLossHarvester) swapBackExpiredSubstitutions(ctx context.Context, batch *portfolio.Batch, taxAware portfolio.TaxAware) error {
	subs := taxAware.ActiveSubstitutions()
	for _, sub := range subs {
		if !sub.Until.Before(batch.Timestamp) {
			continue
		}

		// Substitution has expired -- swap back.
		subQty := batch.Portfolio().Position(sub.Substitute)
		if subQty <= 0 {
			continue
		}

		subPrice := currentPrice(batch, sub.Substitute)
		if subPrice <= 0 {
			continue
		}

		sellDollars := subQty * subPrice

		justification := fmt.Sprintf(
			"tax-loss harvest swap-back: sell %s, buy %s (wash sale window expired)",
			sub.Substitute.Ticker, sub.Original.Ticker,
		)

		if err := batch.Order(ctx, sub.Substitute, portfolio.Sell, subQty,
			portfolio.WithLotSelection(portfolio.LotHighestCost),
			portfolio.WithJustification(justification),
		); err != nil {
			return fmt.Errorf("tax loss harvester swap-back: sell %s: %w", sub.Substitute.Ticker, err)
		}

		if err := batch.Order(ctx, sub.Original, portfolio.Buy, 0,
			portfolio.WithJustification(justification),
		); err != nil {
			return fmt.Errorf("tax loss harvester swap-back: buy %s: %w", sub.Original.Ticker, err)
		}

		// Set buy amount to match sold value.
		lastIdx := len(batch.Orders) - 1
		batch.Orders[lastIdx].Amount = sellDollars
		batch.Orders[lastIdx].Qty = 0
	}

	return nil
}

// currentPrice derives the per-share price for an asset from the
// portfolio's position data or price DataFrame.
func currentPrice(batch *portfolio.Batch, ast asset.Asset) float64 {
	port := batch.Portfolio()
	qty := port.Position(ast)

	if qty > 0 {
		return port.PositionValue(ast) / qty
	}

	// Fall back to price data for assets not currently held (e.g., substitutes).
	prices := port.Prices()
	if prices == nil {
		return 0
	}

	val := prices.Value(ast, data.MetricClose)
	if math.IsNaN(val) {
		return 0
	}

	return val
}
