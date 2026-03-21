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

package engine

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/rs/zerolog"
)

const fillChannelSize = 1024

// SimulatedBroker fills all orders at the close price for backtesting.
// The engine sets a PriceProvider and date before each Compute step.
type SimulatedBroker struct {
	prices            broker.PriceProvider
	date              time.Time
	fills             chan broker.Fill
	pending           map[string]broker.Order
	groups            map[string][]string // groupID -> orderIDs
	portfolio         portfolio.Portfolio
	initialMarginRate float64
}

// NewSimulatedBroker creates a SimulatedBroker with no price provider set.
func NewSimulatedBroker() *SimulatedBroker {
	return &SimulatedBroker{
		fills:   make(chan broker.Fill, fillChannelSize),
		pending: make(map[string]broker.Order),
		groups:  make(map[string][]string),
	}
}

// SetPriceProvider updates the price provider and simulation date.
func (b *SimulatedBroker) SetPriceProvider(p broker.PriceProvider, date time.Time) {
	b.prices = p
	b.date = date
}

// SetPortfolio sets the portfolio reference used for margin checks.
func (b *SimulatedBroker) SetPortfolio(p portfolio.Portfolio) {
	b.portfolio = p
}

// SetInitialMarginRate sets the initial margin rate for short sells.
func (b *SimulatedBroker) SetInitialMarginRate(rate float64) {
	b.initialMarginRate = rate
}

func (b *SimulatedBroker) Connect(_ context.Context) error { return nil }
func (b *SimulatedBroker) Close() error                    { return nil }

// Fills returns the receive-only channel on which fill reports are delivered.
func (b *SimulatedBroker) Fills() <-chan broker.Fill {
	return b.fills
}

func (b *SimulatedBroker) Submit(ctx context.Context, order broker.Order) error {
	if order.GroupRole == broker.RoleStopLoss || order.GroupRole == broker.RoleTakeProfit {
		b.pending[order.ID] = order

		if order.GroupID != "" {
			b.groups[order.GroupID] = append(b.groups[order.GroupID], order.ID)
		}

		return nil
	}

	if b.prices == nil {
		return fmt.Errorf("simulated broker: no price provider set")
	}

	df, err := b.prices.Prices(ctx, order.Asset)
	if err != nil {
		return fmt.Errorf("simulated broker: fetching price for %s: %w", order.Asset.Ticker, err)
	}

	price := df.Value(order.Asset, data.MetricClose)
	if math.IsNaN(price) || price == 0 {
		return fmt.Errorf("simulated broker: no price for %s (%s)",
			order.Asset.Ticker, order.Asset.CompositeFigi)
	}

	qty := order.Qty
	if qty == 0 && order.Amount > 0 {
		qty = math.Floor(order.Amount / price)
	}

	if qty == 0 {
		return nil
	}

	// Check initial margin for short-opening sells.
	if order.Side == broker.Sell && b.portfolio != nil {
		currentPos := b.portfolio.Position(order.Asset)
		if currentPos-qty < 0 {
			shortIncrease := qty
			if currentPos > 0 {
				shortIncrease = qty - currentPos
			}

			newShortValue := b.portfolio.ShortMarketValue() + shortIncrease*price
			equity := b.portfolio.Equity()

			initialRate := b.initialMarginRate
			if initialRate == 0 {
				initialRate = 0.50
			}

			if newShortValue > 0 && equity/newShortValue < initialRate {
				zerolog.Ctx(ctx).Warn().
					Str("asset", order.Asset.Ticker).
					Float64("equity", equity).
					Float64("new_short_value", newShortValue).
					Msg("order rejected: insufficient margin")

				return nil
			}
		}
	}

	b.fills <- broker.Fill{
		OrderID:  order.ID,
		Price:    price,
		Qty:      qty,
		FilledAt: b.date,
	}

	return nil
}

func (b *SimulatedBroker) Cancel(_ context.Context, orderID string) error {
	order, ok := b.pending[orderID]
	if !ok {
		return fmt.Errorf("simulated broker: order %s not found", orderID)
	}

	delete(b.pending, orderID)

	if order.GroupID != "" {
		ids := b.groups[order.GroupID]
		updated := ids[:0]

		for _, id := range ids {
			if id != orderID {
				updated = append(updated, id)
			}
		}

		if len(updated) == 0 {
			delete(b.groups, order.GroupID)
		} else {
			b.groups[order.GroupID] = updated
		}
	}

	return nil
}

// SubmitGroup submits a contingent order group by calling Submit for each order.
func (b *SimulatedBroker) SubmitGroup(ctx context.Context, orders []broker.Order, _ broker.GroupType) error {
	for _, order := range orders {
		if err := b.Submit(ctx, order); err != nil {
			return err
		}
	}

	return nil
}

// EvaluatePending checks all pending stop-loss and take-profit orders against
// the current bar's high/low prices, filling or cancelling as needed.
// For each group with both a stop-loss and take-profit: if stop-loss triggers
// (pessimistic outcome wins on same-bar conflict), fill at stop price and cancel
// take-profit. If only take-profit triggers, fill at limit price and cancel
// stop-loss. Falls back to close price when high/low are unavailable.
func (b *SimulatedBroker) EvaluatePending() {
	if b.prices == nil || len(b.pending) == 0 {
		return
	}

	ctx := context.Background()

	// Collect unique assets from pending orders.
	assetSet := make(map[string]asset.Asset)
	for _, order := range b.pending {
		assetSet[order.Asset.CompositeFigi] = order.Asset
	}

	assets := make([]asset.Asset, 0, len(assetSet))
	for _, a := range assetSet {
		assets = append(assets, a)
	}

	df, err := b.prices.Prices(ctx, assets...)
	if err != nil {
		return
	}

	// Process each group.
	for groupID, orderIDs := range b.groups {
		var stopOrder, tpOrder broker.Order

		var hasStop, hasTP bool

		for _, oid := range orderIDs {
			order, ok := b.pending[oid]
			if !ok {
				continue
			}

			switch order.GroupRole {
			case broker.RoleStopLoss:
				stopOrder = order
				hasStop = true
			case broker.RoleTakeProfit:
				tpOrder = order
				hasTP = true
			}
		}

		if !hasStop && !hasTP {
			continue
		}

		// Determine which asset to use (stop and tp should share the same asset).
		var targetAsset asset.Asset
		if hasStop {
			targetAsset = stopOrder.Asset
		} else {
			targetAsset = tpOrder.Asset
		}

		high := df.Value(targetAsset, data.MetricHigh)
		low := df.Value(targetAsset, data.MetricLow)
		closePrice := df.Value(targetAsset, data.MetricClose)

		// Fall back to close if high/low unavailable.
		highUnavailable := math.IsNaN(high) || high == 0
		lowUnavailable := math.IsNaN(low) || low == 0

		// Evaluate stop-loss trigger.
		stopTriggered := false
		stopFillPrice := 0.0

		if hasStop {
			if stopOrder.Side == broker.Sell {
				// Long position stop-loss: triggers when low <= StopPrice.
				triggerPrice := low
				if lowUnavailable {
					triggerPrice = closePrice
				}

				if (!math.IsNaN(triggerPrice) && triggerPrice != 0) && triggerPrice <= stopOrder.StopPrice {
					stopTriggered = true
					stopFillPrice = stopOrder.StopPrice
				}
			} else {
				// Short position stop-loss (Buy side): triggers when high >= StopPrice.
				triggerPrice := high
				if highUnavailable {
					triggerPrice = closePrice
				}

				if (!math.IsNaN(triggerPrice) && triggerPrice != 0) && triggerPrice >= stopOrder.StopPrice {
					stopTriggered = true
					stopFillPrice = stopOrder.StopPrice
				}
			}
		}

		// Evaluate take-profit trigger.
		tpTriggered := false
		tpFillPrice := 0.0

		if hasTP {
			if tpOrder.Side == broker.Sell {
				// Long position take-profit: triggers when high >= LimitPrice.
				triggerPrice := high
				if highUnavailable {
					triggerPrice = closePrice
				}

				if (!math.IsNaN(triggerPrice) && triggerPrice != 0) && triggerPrice >= tpOrder.LimitPrice {
					tpTriggered = true
					tpFillPrice = tpOrder.LimitPrice
				}
			} else {
				// Short position take-profit (Buy side): triggers when low <= LimitPrice.
				triggerPrice := low
				if lowUnavailable {
					triggerPrice = closePrice
				}

				if (!math.IsNaN(triggerPrice) && triggerPrice != 0) && triggerPrice <= tpOrder.LimitPrice {
					tpTriggered = true
					tpFillPrice = tpOrder.LimitPrice
				}
			}
		}

		switch {
		case stopTriggered:
			// Stop loss wins (pessimistic) — even if TP also triggered.
			b.fills <- broker.Fill{
				OrderID:  stopOrder.ID,
				Price:    stopFillPrice,
				Qty:      stopOrder.Qty,
				FilledAt: b.date,
			}

			delete(b.pending, stopOrder.ID)

			if hasTP {
				delete(b.pending, tpOrder.ID)
			}

			delete(b.groups, groupID)

		case tpTriggered:
			b.fills <- broker.Fill{
				OrderID:  tpOrder.ID,
				Price:    tpFillPrice,
				Qty:      tpOrder.Qty,
				FilledAt: b.date,
			}

			delete(b.pending, tpOrder.ID)

			if hasStop {
				delete(b.pending, stopOrder.ID)
			}

			delete(b.groups, groupID)
		}
	}
}

func (b *SimulatedBroker) Replace(_ context.Context, _ string, _ broker.Order) error {
	return fmt.Errorf("simulated broker: replace not supported")
}

func (b *SimulatedBroker) Orders(_ context.Context) ([]broker.Order, error) {
	orders := make([]broker.Order, 0, len(b.pending))
	for _, order := range b.pending {
		orders = append(orders, order)
	}

	return orders, nil
}

func (b *SimulatedBroker) Positions(_ context.Context) ([]broker.Position, error) {
	return nil, nil
}

func (b *SimulatedBroker) Balance(_ context.Context) (broker.Balance, error) {
	return broker.Balance{}, nil
}
