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
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// SimulatedBroker fills all orders at the close price for backtesting.
// The engine updates the price function and date before each Compute step.
type SimulatedBroker struct {
	priceFn func(asset.Asset) (float64, bool)
	date    time.Time
}

// NewSimulatedBroker creates a SimulatedBroker with no prices set.
func NewSimulatedBroker() *SimulatedBroker {
	return &SimulatedBroker{
		priceFn: func(_ asset.Asset) (float64, bool) { return 0, false },
	}
}

// SetPrices updates the price lookup function and simulation date.
func (b *SimulatedBroker) SetPrices(fn func(asset.Asset) (float64, bool), date time.Time) {
	b.priceFn = fn
	b.date = date
}

func (b *SimulatedBroker) Connect(_ context.Context) error { return nil }
func (b *SimulatedBroker) Close() error                    { return nil }

func (b *SimulatedBroker) Submit(order broker.Order) ([]broker.Fill, error) {
	price, ok := b.priceFn(order.Asset)
	if !ok {
		return nil, fmt.Errorf("simulated broker: no price for %s (%s)",
			order.Asset.Ticker, order.Asset.CompositeFigi)
	}

	return []broker.Fill{{
		OrderID:  order.ID,
		Price:    price,
		Qty:      order.Qty,
		FilledAt: b.date,
	}}, nil
}

func (b *SimulatedBroker) Cancel(_ string) error {
	return fmt.Errorf("simulated broker: cancel not supported")
}

func (b *SimulatedBroker) Replace(_ string, _ broker.Order) ([]broker.Fill, error) {
	return nil, fmt.Errorf("simulated broker: replace not supported")
}

func (b *SimulatedBroker) Orders() ([]broker.Order, error)       { return nil, nil }
func (b *SimulatedBroker) Positions() ([]broker.Position, error) { return nil, nil }
func (b *SimulatedBroker) Balance() (broker.Balance, error)      { return broker.Balance{}, nil }
