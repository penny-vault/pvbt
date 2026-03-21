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
	"context"
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// OrderGroupSpec describes an order group before submission.
type OrderGroupSpec struct {
	GroupID    string
	Type       broker.GroupType
	EntryIndex int // index into batch.Orders; -1 for standalone OCO
	StopLoss   ExitTarget
	TakeProfit ExitTarget
}

// Batch accumulates orders and annotations produced during a single engine
// frame. Rather than executing trades immediately, a Batch buffers them so
// that middleware layers can inspect or modify the full set before any order
// reaches the broker. Account.ExecuteBatch drains the batch through the
// middleware chain and submits the final orders.
type Batch struct {
	// Timestamp is the frame's trading date.
	Timestamp time.Time

	// Orders holds orders accumulated by calls to Order and RebalanceTo.
	Orders []broker.Order

	// Annotations holds key-value metadata accumulated by calls to Annotate.
	Annotations map[string]string

	// SkipMiddleware bypasses the middleware chain when true. Used for
	// margin-call response batches where risk limits must not block
	// emergency position adjustments.
	SkipMiddleware bool

	// portfolio is the read-only portfolio snapshot used for price and
	// position queries. It is unexported to prevent strategy code from
	// modifying it through the Batch.
	portfolio Portfolio

	// groups holds order group specifications accumulated during Order calls.
	groups []OrderGroupSpec
}

// NewBatch creates an empty Batch for the given timestamp and portfolio snapshot.
func NewBatch(timestamp time.Time, port Portfolio) *Batch {
	return &Batch{
		Timestamp:   timestamp,
		Orders:      nil,
		Annotations: make(map[string]string),
		portfolio:   port,
	}
}

// Portfolio returns the read-only portfolio reference associated with this batch.
func (b *Batch) Portfolio() Portfolio {
	return b.portfolio
}

// Groups returns the order group specifications accumulated during Order calls.
func (b *Batch) Groups() []OrderGroupSpec {
	return b.groups
}

// Annotate stores a key-value pair in the batch's annotation map.
// Calling Annotate again with the same key overwrites the previous value.
func (b *Batch) Annotate(key, value string) {
	b.Annotations[key] = value
}

// Order accumulates a broker.Order in the batch without executing it.
// The side, asset, and quantity are recorded along with any modifiers
// (Limit, Stop, time-in-force, WithJustification, etc.).
func (b *Batch) Order(_ context.Context, ast asset.Asset, side Side, qty float64, mods ...OrderModifier) error {
	order := broker.Order{
		Asset:       ast,
		Qty:         qty,
		OrderType:   broker.Market,
		TimeInForce: broker.Day,
	}

	switch side {
	case Buy:
		order.Side = broker.Buy
	case Sell:
		order.Side = broker.Sell
	}

	var hasLimit, hasStop bool

	var bracket *bracketModifier

	var oco *ocoModifier

	for _, mod := range mods {
		switch modifier := mod.(type) {
		case limitModifier:
			order.LimitPrice = modifier.price
			hasLimit = true
		case stopModifier:
			order.StopPrice = modifier.price
			hasStop = true
		case dayOrderModifier:
			order.TimeInForce = broker.Day
		case goodTilCancelModifier:
			order.TimeInForce = broker.GTC
		case fillOrKillModifier:
			order.TimeInForce = broker.FOK
		case immediateOrCancelModifier:
			order.TimeInForce = broker.IOC
		case onTheOpenModifier:
			order.TimeInForce = broker.OnOpen
		case onTheCloseModifier:
			order.TimeInForce = broker.OnClose
		case goodTilDateModifier:
			order.TimeInForce = broker.GTD
			order.GTDDate = modifier.date
		case justificationModifier:
			order.Justification = modifier.reason
		case lotSelectionModifier:
			order.LotSelection = int(modifier.method)
		case bracketModifier:
			// handled in post-expansion below
			bracket = &modifier
		case ocoModifier:
			// handled in post-expansion below
			oco = &modifier
		}
	}

	if hasLimit && hasStop {
		order.OrderType = broker.StopLimit
	} else if hasLimit {
		order.OrderType = broker.Limit
	} else if hasStop {
		order.OrderType = broker.Stop
	}

	groupIndex := len(b.groups)
	ts := b.Timestamp.UnixNano()

	if oco != nil {
		// Discard the original order. Create two broker.Order entries from the OCO legs.
		groupID := fmt.Sprintf("oco-%d-%d", ts, groupIndex)

		legAOrder := broker.Order{
			Asset:       ast,
			Side:        order.Side,
			Qty:         qty,
			TimeInForce: order.TimeInForce,
			GTDDate:     order.GTDDate,
			GroupID:     groupID,
			GroupRole:   broker.RoleStopLoss,
			OrderType:   oco.legA.OrderType,
		}
		if oco.legA.OrderType == broker.Stop {
			legAOrder.StopPrice = oco.legA.Price
		} else {
			legAOrder.LimitPrice = oco.legA.Price
		}

		legBOrder := broker.Order{
			Asset:       ast,
			Side:        order.Side,
			Qty:         qty,
			TimeInForce: order.TimeInForce,
			GTDDate:     order.GTDDate,
			GroupID:     groupID,
			GroupRole:   broker.RoleTakeProfit,
			OrderType:   oco.legB.OrderType,
		}
		if oco.legB.OrderType == broker.Stop {
			legBOrder.StopPrice = oco.legB.Price
		} else {
			legBOrder.LimitPrice = oco.legB.Price
		}

		b.Orders = append(b.Orders, legAOrder, legBOrder)
		b.groups = append(b.groups, OrderGroupSpec{
			GroupID:    groupID,
			Type:       broker.GroupOCO,
			EntryIndex: -1,
		})

		return nil
	}

	if bracket != nil {
		// Tag the entry order with GroupID and GroupRole=RoleEntry.
		groupID := fmt.Sprintf("bracket-%d-%d", ts, groupIndex)
		entryIndex := len(b.Orders)

		order.GroupID = groupID
		order.GroupRole = broker.RoleEntry

		b.Orders = append(b.Orders, order)
		b.groups = append(b.groups, OrderGroupSpec{
			GroupID:    groupID,
			Type:       broker.GroupBracket,
			EntryIndex: entryIndex,
			StopLoss:   bracket.stopLoss,
			TakeProfit: bracket.takeProfit,
		})

		return nil
	}

	b.Orders = append(b.Orders, order)

	return nil
}

// RebalanceTo computes the orders needed to move from the current portfolio
// state to the target allocations and appends them to batch.Orders. It
// mirrors Account.RebalanceTo but uses projectedPositionValue / ProjectedValue
// so that earlier orders in the batch are taken into account when computing
// buy/sell amounts.
func (b *Batch) RebalanceTo(_ context.Context, allocs ...Allocation) error {
	for _, alloc := range allocs {
		// Filter out $CASH entries -- cash is the implicit remainder.
		filtered := make(map[asset.Asset]float64, len(alloc.Members))
		for memberAsset, weight := range alloc.Members {
			if memberAsset.Ticker != "$CASH" {
				filtered[memberAsset] = weight
			}
		}

		alloc = Allocation{
			Date:          alloc.Date,
			Members:       filtered,
			Justification: alloc.Justification,
		}

		totalValue := b.ProjectedValue()

		type pendingOrder struct {
			asset  asset.Asset
			side   Side
			qty    float64 // share count for full liquidations
			amount float64 // dollar amount for partial adjustments
		}

		var sells []pendingOrder

		// Sell all holdings not in the target allocation.
		b.portfolio.Holdings(func(ast asset.Asset, qty float64) {
			if _, ok := alloc.Members[ast]; !ok && qty > 0 {
				sells = append(sells, pendingOrder{asset: ast, side: Sell, qty: qty})
			}
		})

		// Sell overweight positions.
		for ast, weight := range alloc.Members {
			targetDollars := weight * totalValue
			currentDollars := b.projectedPositionValue(ast)
			diff := targetDollars - currentDollars

			if diff < 0 {
				sells = append(sells, pendingOrder{asset: ast, side: Sell, amount: -diff})
			}
		}

		for _, sell := range sells {
			order := broker.Order{
				Asset:         sell.asset,
				Side:          broker.Sell,
				Qty:           sell.qty,
				Amount:        sell.amount,
				OrderType:     broker.Market,
				TimeInForce:   broker.Day,
				Justification: alloc.Justification,
			}
			b.Orders = append(b.Orders, order)
		}

		// Recompute value after projected sells to use actual available cash.
		postSellValue := b.ProjectedValue()

		var buys []pendingOrder

		for ast, weight := range alloc.Members {
			targetDollars := weight * postSellValue
			currentDollars := b.projectedPositionValue(ast)
			diff := targetDollars - currentDollars

			if diff > 0 {
				buys = append(buys, pendingOrder{asset: ast, side: Buy, amount: diff})
			}
		}

		for _, buy := range buys {
			order := broker.Order{
				Asset:         buy.asset,
				Side:          broker.Buy,
				Amount:        buy.amount,
				OrderType:     broker.Market,
				TimeInForce:   broker.Day,
				Justification: alloc.Justification,
			}
			b.Orders = append(b.Orders, order)
		}
	}

	return nil
}

// ProjectedHoldings returns what holdings would be if all batch orders
// executed at last known prices. Dollar-amount orders are converted to share
// quantities using math.Floor(amount / price). Assets with unknown prices
// (priceOf returns 0) that only appear in buy orders are not added to the
// projected holdings.
//
// When active substitutions exist (the portfolio implements TaxAware),
// order assets that are substitutes for a logical original are recorded
// under the original asset name, consistent with the logical view
// returned by Holdings().
func (b *Batch) ProjectedHoldings() map[asset.Asset]float64 {
	holdings := make(map[asset.Asset]float64)

	// Seed with current holdings (already mapped to logical by Account.Holdings).
	b.portfolio.Holdings(func(ast asset.Asset, qty float64) {
		holdings[ast] = qty
	})

	// Obtain active substitutions if the portfolio supports them.
	var subs map[asset.Asset]Substitution

	if taxAware, ok := b.portfolio.(TaxAware); ok {
		subs = taxAware.ActiveSubstitutions()
	}

	for _, order := range b.Orders {
		price := b.priceOf(order.Asset)

		var qty float64

		if order.Qty != 0 {
			qty = order.Qty
		} else if price > 0 && order.Amount > 0 {
			qty = math.Floor(order.Amount / price)
		}

		// Map the order asset to its logical original if a substitution is active.
		orderAsset := order.Asset
		if len(subs) > 0 {
			orderAsset = mapToLogical(order.Asset, subs, b.Timestamp)
		}

		switch order.Side {
		case broker.Buy:
			holdings[orderAsset] += qty
		case broker.Sell:
			holdings[orderAsset] -= qty
		}

		if holdings[orderAsset] == 0 {
			delete(holdings, orderAsset)
		}
	}

	return holdings
}

// ProjectedValue returns the total portfolio value after all batch orders
// execute at last known prices.
func (b *Batch) ProjectedValue() float64 {
	total := b.projectedCash()

	for ast, qty := range b.ProjectedHoldings() {
		price := b.priceOf(ast)
		if price > 0 {
			total += qty * price
		}
	}

	return total
}

// ProjectedWeights returns position weights after projected execution.
// Each weight is the position's dollar value divided by total projected value.
// Returns an empty map if projected value is zero.
func (b *Batch) ProjectedWeights() map[asset.Asset]float64 {
	total := b.ProjectedValue()
	if total == 0 {
		return make(map[asset.Asset]float64)
	}

	holdings := b.ProjectedHoldings()
	weights := make(map[asset.Asset]float64, len(holdings))

	for ast, qty := range holdings {
		price := b.priceOf(ast)
		if price > 0 {
			weights[ast] = (qty * price) / total
		}
	}

	return weights
}

// projectedPositionValue returns the dollar value of an asset's position
// in the projected state (after applying all batch orders so far).
func (b *Batch) projectedPositionValue(ast asset.Asset) float64 {
	holdings := b.ProjectedHoldings()
	qty := holdings[ast]
	price := b.priceOf(ast)

	return qty * price
}

// projectedCash returns the cash balance after all batch orders execute.
// Sell orders (by qty or dollar amount) add cash; buy orders subtract cash.
func (b *Batch) projectedCash() float64 {
	cash := b.portfolio.Cash()

	for _, order := range b.Orders {
		price := b.priceOf(order.Asset)

		var dollarAmount float64

		if order.Amount > 0 {
			dollarAmount = order.Amount
		} else if order.Qty != 0 && price > 0 {
			dollarAmount = order.Qty * price
		}

		switch order.Side {
		case broker.Buy:
			cash -= dollarAmount
		case broker.Sell:
			cash += dollarAmount
		}
	}

	return cash
}

// priceOf derives the per-share price for an asset. For held assets the price
// is computed from PositionValue / Position. For assets not currently held, it
// falls back to the portfolio's most-recent price DataFrame.
func (b *Batch) priceOf(ast asset.Asset) float64 {
	qty := b.portfolio.Position(ast)
	if qty != 0 {
		return b.portfolio.PositionValue(ast) / qty
	}

	// Fall back to price data for assets not yet held.
	prices := b.portfolio.Prices()
	if prices == nil {
		return 0
	}

	v := prices.Value(ast, data.MetricClose)
	if math.IsNaN(v) {
		return 0
	}

	return v
}
