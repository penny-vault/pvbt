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

package broker

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// Broker is the interface between the portfolio and a brokerage. When a
// portfolio has an associated broker, order execution, position queries,
// and balance lookups are delegated to the broker. When no broker is
// attached, the portfolio uses simulated execution for backtesting.
type Broker interface {
	// Connect establishes a session with the brokerage, performing
	// authentication and any setup required before trading. Credentials
	// and token refresh are implementation details of each broker.
	Connect(ctx context.Context) error

	// Close tears down the broker session and releases resources.
	Close() error

	// Submit sends an order to the brokerage. Fills are delivered
	// asynchronously through the channel returned by Fills.
	Submit(ctx context.Context, order Order) error

	// Fills returns a receive-only channel on which fill reports are
	// delivered after each Submit call.
	Fills() <-chan Fill

	// Cancel requests cancellation of an open order by ID.
	Cancel(ctx context.Context, orderID string) error

	// Replace cancels an existing order and submits a replacement atomically
	// (cancel-replace).
	Replace(ctx context.Context, orderID string, order Order) error

	// Orders returns all orders for the current trading day.
	Orders(ctx context.Context) ([]Order, error)

	// Positions returns all current positions in the account.
	Positions(ctx context.Context) ([]Position, error)

	// Balance returns the current account balance.
	Balance(ctx context.Context) (Balance, error)
}

// Side indicates a buy or sell direction at the broker level.
type Side int

const (
	Buy Side = iota
	Sell
)

// OrderStatus tracks the lifecycle state of an order.
type OrderStatus int

const (
	OrderOpen OrderStatus = iota
	OrderSubmitted
	OrderFilled
	OrderPartiallyFilled
	OrderCancelled
)

// Order represents an order submitted to a broker.
// When Qty is 0 and Amount > 0, the broker treats it as a dollar-amount
// order and computes the share quantity from the current market price.
type Order struct {
	ID            string
	Asset         asset.Asset
	Side          Side
	Status        OrderStatus
	Qty           float64
	Amount        float64
	OrderType     OrderType
	TimeInForce   TimeInForce
	LimitPrice    float64
	StopPrice     float64
	GTDDate       time.Time
	Justification string
}

// OrderType identifies the price behavior of an order.
type OrderType int

const (
	Market OrderType = iota
	Limit
	Stop
	StopLimit
)

// TimeInForce controls how long an order remains active.
type TimeInForce int

const (
	Day TimeInForce = iota
	GTC
	GTD
	IOC
	FOK
	OnOpen
	OnClose
)

// Fill represents the execution result of an order.
type Fill struct {
	OrderID  string
	Price    float64
	Qty      float64
	FilledAt time.Time
}

// Position represents a holding in the account.
type Position struct {
	Asset         asset.Asset
	Qty           float64
	AvgOpenPrice  float64
	MarkPrice     float64
	RealizedDayPL float64
}

// Balance represents the account's financial state.
type Balance struct {
	CashBalance         float64
	NetLiquidatingValue float64
	EquityBuyingPower   float64
	MaintenanceReq      float64
}

// PriceProvider supplies current market prices. The engine implements
// this interface; the simulated broker uses it to determine fill prices
// and convert dollar-amount orders to share quantities.
type PriceProvider interface {
	Prices(ctx context.Context, assets ...asset.Asset) (*data.DataFrame, error)
}
