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

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

const fillChannelSize = 1024

// SimulatedBroker fills all orders at the close price for backtesting.
// The engine sets a PriceProvider and date before each Compute step.
type SimulatedBroker struct {
	prices  broker.PriceProvider
	date    time.Time
	fills   chan broker.Fill
	pending map[string]broker.Order
}

// NewSimulatedBroker creates a SimulatedBroker with no price provider set.
func NewSimulatedBroker() *SimulatedBroker {
	return &SimulatedBroker{
		fills:   make(chan broker.Fill, fillChannelSize),
		pending: make(map[string]broker.Order),
	}
}

// SetPriceProvider updates the price provider and simulation date.
func (b *SimulatedBroker) SetPriceProvider(p broker.PriceProvider, date time.Time) {
	b.prices = p
	b.date = date
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

	b.fills <- broker.Fill{
		OrderID:  order.ID,
		Price:    price,
		Qty:      qty,
		FilledAt: b.date,
	}

	return nil
}

func (b *SimulatedBroker) Cancel(_ context.Context, orderID string) error {
	if _, ok := b.pending[orderID]; !ok {
		return fmt.Errorf("simulated broker: order %s not found", orderID)
	}
	delete(b.pending, orderID)
	return nil
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
