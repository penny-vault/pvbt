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
	"errors"
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

// partialRemainder tracks an order that was partially filled and is
// waiting to be retried on the next bar.
type partialRemainder struct {
	order broker.Order
	bars  int // how many bars this remainder has been pending
}

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
	maxLeverage       float64
	borrowRate        float64
	lastPrices        map[asset.Asset]float64
	fillPipeline      *broker.Pipeline
	partialRemainders map[string]partialRemainder
}

// NewSimulatedBroker creates a SimulatedBroker with no price provider set.
func NewSimulatedBroker() *SimulatedBroker {
	return &SimulatedBroker{
		fills:             make(chan broker.Fill, fillChannelSize),
		pending:           make(map[string]broker.Order),
		groups:            make(map[string][]string),
		lastPrices:        make(map[asset.Asset]float64),
		fillPipeline:      broker.NewPipeline(broker.FillAtClose(), nil),
		partialRemainders: make(map[string]partialRemainder),
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

// SetBorrowRate sets the annualized borrow fee rate for short positions.
func (b *SimulatedBroker) SetBorrowRate(rate float64) {
	b.borrowRate = rate
}

// SetMaxLeverage sets the gross-leverage cap (LMV+SMV)/Equity used to
// reject orders that would push the account above the cap. Values <= 0
// disable the broker-level check; the account-level default still
// applies via portfolio.Account.MaxLeverage.
func (b *SimulatedBroker) SetMaxLeverage(ratio float64) {
	b.maxLeverage = ratio
}

// SetFillPipeline replaces the default close-price fill pipeline.
func (b *SimulatedBroker) SetFillPipeline(pp *broker.Pipeline) {
	b.fillPipeline = pp
}

// SetDataFetcher propagates a DataFetcher to the fill pipeline's models.
func (b *SimulatedBroker) SetDataFetcher(df broker.DataFetcher) {
	b.fillPipeline.SetDataFetcher(df)
}

func (b *SimulatedBroker) Connect(_ context.Context) error { return nil }
func (b *SimulatedBroker) Close() error                    { return nil }

// Fills returns the receive-only channel on which fill reports are delivered.
func (b *SimulatedBroker) Fills() <-chan broker.Fill {
	return b.fills
}

// deliverFill places a fill on the fills channel without ever blocking.
// In simulation, fills are produced (Submit/EvaluatePending) and consumed
// (Account drain) on the same goroutine, so a batch with more orders than
// the channel's capacity would deadlock on a blocking send. When the
// channel is full it is replaced with one twice the size, preserving fill
// order; consumers re-fetch the channel via Fills() on every drain.
func (b *SimulatedBroker) deliverFill(fill broker.Fill) {
	select {
	case b.fills <- fill:
		return
	default:
	}

	grown := make(chan broker.Fill, 2*cap(b.fills)+1)

	for {
		select {
		case queued := <-b.fills:
			grown <- queued
		default:
			grown <- fill

			b.fills = grown

			return
		}
	}
}

// failOrder resolves an order as failed: it emits a fill carrying the reason
// (Qty 0, no price) on the fills channel so the account records the failure
// and the originating strategy can react to it, instead of the order silently
// vanishing or the simulation aborting.
func (b *SimulatedBroker) failOrder(order broker.Order, reason error) {
	b.deliverFill(broker.Fill{
		OrderID:  order.ID,
		FilledAt: b.date,
		Err:      reason,
	})
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
		// No price to fill against (e.g. an asset whose intraday data
		// coverage starts after this date) is a recoverable, per-order
		// failure: resolve the order as failed rather than aborting the
		// whole simulation. Any other fetch error is genuine and fatal.
		if errors.Is(err, ErrNoMinuteBar) {
			zerolog.Ctx(ctx).Warn().
				Err(err).
				Str("asset", order.Asset.Ticker).
				Msg("order failed: no price available to fill")
			b.failOrder(order, fmt.Errorf("no price available to fill %s: %w", order.Asset.Ticker, err))

			return nil
		}

		return fmt.Errorf("simulated broker: fetching price for %s: %w", order.Asset.Ticker, err)
	}

	// Phase 1: Base model determines the price.
	baseResult, baseErr := b.fillPipeline.FillBase(ctx, order, df)
	if baseErr != nil {
		zerolog.Ctx(ctx).Warn().
			Err(baseErr).
			Str("asset", order.Asset.Ticker).
			Msg("order failed: fill model error")
		b.failOrder(order, fmt.Errorf("fill model error for %s: %w", order.Asset.Ticker, baseErr))

		return nil
	}

	// Convert dollar-amount orders between base and adjusters.
	qty := baseResult.Quantity
	if qty == 0 && order.Amount > 0 {
		qty = math.Floor(order.Amount / baseResult.Price)
	}

	if qty == 0 {
		b.failOrder(order, fmt.Errorf("order amount for %s is too small to fill a whole share", order.Asset.Ticker))

		return nil
	}

	baseResult.Quantity = qty

	// Phase 2: Run adjusters on the result with computed quantity.
	result, adjErr := b.fillPipeline.Adjust(ctx, order, df, baseResult)
	if adjErr != nil {
		zerolog.Ctx(ctx).Warn().
			Err(adjErr).
			Str("asset", order.Asset.Ticker).
			Msg("order failed: fill adjuster error")
		b.failOrder(order, fmt.Errorf("fill adjuster error for %s: %w", order.Asset.Ticker, adjErr))

		return nil
	}

	// Margin checks: initial margin on short-opening sells, and the
	// gross-leverage cap on any order that increases gross notional.
	if b.portfolio != nil {
		currentPos := b.portfolio.Position(order.Asset)

		signedDelta := result.Quantity
		if order.Side == broker.Sell {
			signedDelta = -signedDelta
		}

		postQty := currentPos + signedDelta

		preLong := math.Max(currentPos, 0)
		preShort := math.Max(-currentPos, 0)
		postLong := math.Max(postQty, 0)
		postShort := math.Max(-postQty, 0)

		newShortValue := b.portfolio.ShortMarketValue() + (postShort-preShort)*result.Price
		equity := b.portfolio.Equity()

		if order.Side == broker.Sell && postQty < 0 && newShortValue > 0 {
			initialRate := b.initialMarginRate
			if initialRate == 0 {
				initialRate = 0.50
			}

			if equity/newShortValue < initialRate {
				zerolog.Ctx(ctx).Warn().
					Str("asset", order.Asset.Ticker).
					Float64("equity", equity).
					Float64("new_short_value", newShortValue).
					Msg("order rejected: insufficient margin")
				b.failOrder(order, fmt.Errorf("order rejected: insufficient margin for %s", order.Asset.Ticker))

				return nil
			}
		}

		if b.maxLeverage > 0 {
			preLongValue := b.portfolio.LongMarketValue()
			preGross := preLongValue + b.portfolio.ShortMarketValue()
			newLongValue := preLongValue + (postLong-preLong)*result.Price
			postGross := newLongValue + newShortValue

			// Only reject orders that increase gross notional; allow
			// closing trades through even if the account is already
			// above the cap (the margin-call path handles that).
			if postGross > preGross {
				breach := equity <= 0
				if !breach {
					breach = postGross/equity > b.maxLeverage
				}

				if breach {
					zerolog.Ctx(ctx).Warn().
						Str("asset", order.Asset.Ticker).
						Float64("equity", equity).
						Float64("post_gross", postGross).
						Float64("max_leverage", b.maxLeverage).
						Msg("order rejected: gross leverage cap")
					b.failOrder(order, fmt.Errorf("order rejected: gross leverage cap for %s", order.Asset.Ticker))

					return nil
				}
			}
		}
	}

	b.deliverFill(broker.Fill{
		OrderID:  order.ID,
		Price:    result.Price,
		Qty:      result.Quantity,
		FilledAt: b.date,
	})

	// Handle partial fills: queue remainder for next bar.
	if result.Partial {
		remainderQty := qty - result.Quantity
		if remainderQty > 0 {
			remainderOrder := order
			remainderOrder.Qty = remainderQty
			remainderOrder.Amount = 0

			b.partialRemainders[order.ID] = partialRemainder{
				order: remainderOrder,
				bars:  1,
			}
		}
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
	if b.prices == nil {
		return
	}

	if len(b.pending) == 0 {
		// No bracket orders, but still process partial remainders.
		b.evaluatePartialRemainders()

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
			b.deliverFill(broker.Fill{
				OrderID:  stopOrder.ID,
				Price:    stopFillPrice,
				Qty:      stopOrder.Qty,
				FilledAt: b.date,
			})

			delete(b.pending, stopOrder.ID)

			if hasTP {
				delete(b.pending, tpOrder.ID)
			}

			delete(b.groups, groupID)

		case tpTriggered:
			b.deliverFill(broker.Fill{
				OrderID:  tpOrder.ID,
				Price:    tpFillPrice,
				Qty:      tpOrder.Qty,
				FilledAt: b.date,
			})

			delete(b.pending, tpOrder.ID)

			if hasStop {
				delete(b.pending, stopOrder.ID)
			}

			delete(b.groups, groupID)
		}
	}

	// Process partial fill remainders from prior bars.
	b.evaluatePartialRemainders()
}

// evaluatePartialRemainders retries partial fill remainders from prior bars.
// After two bars without a full fill, the remainder is cancelled.
func (b *SimulatedBroker) evaluatePartialRemainders() {
	if b.prices == nil || len(b.partialRemainders) == 0 {
		return
	}

	ctx := context.Background()

	// Prune expired remainders and collect unique assets.
	assetSet := make(map[string]asset.Asset)

	for orderID, pr := range b.partialRemainders {
		if pr.bars >= 2 {
			delete(b.partialRemainders, orderID)
			continue
		}

		assetSet[pr.order.Asset.CompositeFigi] = pr.order.Asset
	}

	if len(b.partialRemainders) == 0 {
		return
	}

	// Batch-fetch prices for all remaining assets.
	assets := make([]asset.Asset, 0, len(assetSet))
	for _, held := range assetSet {
		assets = append(assets, held)
	}

	df, err := b.prices.Prices(ctx, assets...)
	if err != nil {
		return
	}

	// Evaluate each remainder against the batched DataFrame.
	for orderID, pr := range b.partialRemainders {
		result, fillErr := b.fillPipeline.Fill(ctx, pr.order, df)
		if fillErr != nil {
			continue
		}

		if result.Quantity > 0 {
			b.deliverFill(broker.Fill{
				OrderID:  orderID,
				Price:    result.Price,
				Qty:      result.Quantity,
				FilledAt: b.date,
			})
		}

		if !result.Partial {
			delete(b.partialRemainders, orderID)
		} else {
			remainderQty := pr.order.Qty - result.Quantity
			if remainderQty > 0 {
				updatedOrder := pr.order
				updatedOrder.Qty = remainderQty

				b.partialRemainders[orderID] = partialRemainder{
					order: updatedOrder,
					bars:  pr.bars + 1,
				}
			} else {
				delete(b.partialRemainders, orderID)
			}
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

func (b *SimulatedBroker) Transactions(ctx context.Context, _ time.Time) ([]broker.Transaction, error) {
	if b.prices == nil || b.portfolio == nil {
		return nil, nil
	}

	holdings := b.portfolio.Holdings()

	heldAssets := make([]asset.Asset, 0, len(holdings))
	for ast := range holdings {
		heldAssets = append(heldAssets, ast)
	}

	if len(heldAssets) == 0 {
		return nil, nil
	}

	df, err := b.prices.Prices(ctx, heldAssets...)
	if err != nil {
		return nil, fmt.Errorf("simulated broker: fetching housekeeping prices: %w", err)
	}

	var txns []broker.Transaction

	for _, ast := range heldAssets {
		qty := b.portfolio.Position(ast)
		if qty == 0 {
			continue
		}

		closePrice := df.ValueAt(ast, data.MetricClose, b.date)
		if math.IsNaN(closePrice) || closePrice == 0 {
			if lastPrice, ok := b.lastPrices[ast]; ok && lastPrice > 0 {
				amount := lastPrice * math.Abs(qty)
				if qty < 0 {
					amount = -amount
				}

				txns = append(txns, broker.Transaction{
					ID:            fmt.Sprintf("sim-delist-%s-%s", ast.CompositeFigi, b.date.Format("2006-01-02")),
					Date:          b.date,
					Asset:         ast,
					Type:          asset.SellTransaction,
					Qty:           qty,
					Price:         lastPrice,
					Amount:        amount,
					Justification: fmt.Sprintf("delisted: %s liquidated at last known price $%.2f", ast.Ticker, lastPrice),
				})

				delete(b.lastPrices, ast)
			}

			continue
		}

		b.lastPrices[ast] = closePrice

		splitFactor := df.ValueAt(ast, data.SplitFactor, b.date)

		hasSplit := !math.IsNaN(splitFactor) && splitFactor != 1.0
		if hasSplit {
			txns = append(txns, broker.Transaction{
				ID:    fmt.Sprintf("sim-split-%s-%s", ast.CompositeFigi, b.date.Format("2006-01-02")),
				Date:  b.date,
				Asset: ast,
				Type:  asset.SplitTransaction,
				Price: splitFactor,
			})
		}

		divPerShare := df.ValueAt(ast, data.Dividend, b.date)
		if !math.IsNaN(divPerShare) && divPerShare > 0 {
			divQty := qty
			if hasSplit {
				divQty = qty * splitFactor
			}

			amount := divPerShare * divQty

			justification := ""
			if qty < 0 {
				justification = fmt.Sprintf("short dividend obligation: %s ex-date %s",
					ast.Ticker, b.date.Format("2006-01-02"))
			}

			txns = append(txns, broker.Transaction{
				ID:            fmt.Sprintf("sim-div-%s-%s", ast.CompositeFigi, b.date.Format("2006-01-02")),
				Date:          b.date,
				Asset:         ast,
				Type:          asset.DividendTransaction,
				Qty:           divQty,
				Price:         divPerShare,
				Amount:        amount,
				Justification: justification,
			})
		}

		if qty < 0 && b.borrowRate > 0 {
			if !math.IsNaN(closePrice) && closePrice != 0 {
				dailyFee := math.Abs(qty) * closePrice * (b.borrowRate / 252.0)
				txns = append(txns, broker.Transaction{
					ID:            fmt.Sprintf("sim-fee-%s-%s", ast.CompositeFigi, b.date.Format("2006-01-02")),
					Date:          b.date,
					Asset:         ast,
					Type:          asset.FeeTransaction,
					Amount:        -dailyFee,
					Justification: fmt.Sprintf("borrow fee: %s %.2f%% annualized", ast.Ticker, b.borrowRate*100),
				})
			}
		}
	}

	return txns, nil
}
