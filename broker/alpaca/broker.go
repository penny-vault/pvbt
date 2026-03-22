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

package alpaca

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
)

const (
	productionBaseURL = "https://api.alpaca.markets"
	paperBaseURL      = "https://paper-api.alpaca.markets"
	productionWSURL   = "wss://api.alpaca.markets/stream"
	paperWSURL        = "wss://paper-api.alpaca.markets/stream"
	fillChannelSize   = 1024
)

// AlpacaBroker implements broker.Broker and broker.GroupSubmitter for the
// Alpaca brokerage.
type AlpacaBroker struct {
	client          *apiClient
	streamer        *fillStreamer
	fills           chan broker.Fill
	paper           bool
	fractional      bool
	submittedOrders map[string]broker.Order
	mu              sync.Mutex
}

// Option configures an AlpacaBroker.
type Option func(*AlpacaBroker)

// WithPaper configures the broker to use Alpaca's paper trading environment.
func WithPaper() Option {
	return func(alpacaBroker *AlpacaBroker) {
		alpacaBroker.paper = true
	}
}

// WithFractionalShares enables dollar-amount orders using Alpaca's notional
// field instead of computing whole-share quantities.
func WithFractionalShares() Option {
	return func(alpacaBroker *AlpacaBroker) {
		alpacaBroker.fractional = true
	}
}

// New creates a new AlpacaBroker with the given options.
func New(opts ...Option) *AlpacaBroker {
	alpacaBroker := &AlpacaBroker{
		fills:           make(chan broker.Fill, fillChannelSize),
		submittedOrders: make(map[string]broker.Order),
	}

	for _, opt := range opts {
		opt(alpacaBroker)
	}

	return alpacaBroker
}

// Connect establishes a session with Alpaca by reading credentials from
// environment variables and validating the account.
func (alpacaBroker *AlpacaBroker) Connect(ctx context.Context) error {
	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")

	if apiKey == "" || apiSecret == "" {
		return broker.ErrMissingCredentials
	}

	baseURL := productionBaseURL
	if alpacaBroker.paper {
		baseURL = paperBaseURL
	}

	alpacaBroker.client = newAPIClient(baseURL, apiKey, apiSecret)

	account, err := alpacaBroker.client.getAccount(ctx)
	if err != nil {
		return fmt.Errorf("alpaca: connect: %w", err)
	}

	if account.Status != "ACTIVE" {
		return broker.ErrAccountNotActive
	}

	wsURL := productionWSURL
	if alpacaBroker.paper {
		wsURL = paperWSURL
	}

	alpacaBroker.streamer = &fillStreamer{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		fills:     alpacaBroker.fills,
		wsURL:     wsURL,
		seenFills: make(map[string]time.Time),
		done:      make(chan struct{}),
		client:    alpacaBroker.client,
	}

	if err := alpacaBroker.streamer.connect(ctx); err != nil {
		return fmt.Errorf("alpaca: connect streamer: %w", err)
	}

	return nil
}

// Close tears down the broker session and releases resources.
func (alpacaBroker *AlpacaBroker) Close() error {
	if alpacaBroker.streamer != nil {
		if err := alpacaBroker.streamer.close(); err != nil {
			return err
		}
	}

	close(alpacaBroker.fills)

	return nil
}

// Fills returns a receive-only channel on which fill reports are delivered.
func (alpacaBroker *AlpacaBroker) Fills() <-chan broker.Fill {
	return alpacaBroker.fills
}

// Submit sends an order to Alpaca. Dollar-amount orders (Qty==0, Amount>0)
// are converted to share quantities or sent as notional when fractional
// shares are enabled.
func (alpacaBroker *AlpacaBroker) Submit(ctx context.Context, order broker.Order) error {
	if order.Qty == 0 && order.Amount > 0 {
		if alpacaBroker.fractional {
			alpacaOrder := toAlpacaOrder(order, true)

			orderID, err := alpacaBroker.client.submitOrder(ctx, alpacaOrder)
			if err != nil {
				return fmt.Errorf("alpaca: submit order: %w", err)
			}

			alpacaBroker.mu.Lock()
			alpacaBroker.submittedOrders[orderID] = order
			alpacaBroker.mu.Unlock()

			return nil
		}

		price, err := alpacaBroker.client.getLatestTrade(ctx, order.Asset.Ticker)
		if err != nil {
			return fmt.Errorf("alpaca: fetching quote for %s: %w", order.Asset.Ticker, err)
		}

		computedQty := math.Floor(order.Amount / price)
		if computedQty == 0 {
			return nil
		}

		order.Qty = computedQty
	}

	alpacaOrder := toAlpacaOrder(order, false)

	orderID, err := alpacaBroker.client.submitOrder(ctx, alpacaOrder)
	if err != nil {
		return fmt.Errorf("alpaca: submit order: %w", err)
	}

	alpacaBroker.mu.Lock()
	alpacaBroker.submittedOrders[orderID] = order
	alpacaBroker.mu.Unlock()

	return nil
}

// Cancel requests cancellation of an open order by ID.
func (alpacaBroker *AlpacaBroker) Cancel(ctx context.Context, orderID string) error {
	if err := alpacaBroker.client.cancelOrder(ctx, orderID); err != nil {
		return fmt.Errorf("alpaca: cancel order: %w", err)
	}

	alpacaBroker.mu.Lock()
	delete(alpacaBroker.submittedOrders, orderID)
	alpacaBroker.mu.Unlock()

	return nil
}

// Replace cancels an existing order and submits a replacement. If only
// mutable fields changed (Qty, LimitPrice, StopPrice, TimeInForce), it
// uses Alpaca's PATCH endpoint. Otherwise it cancels and resubmits.
func (alpacaBroker *AlpacaBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	alpacaBroker.mu.Lock()
	original, exists := alpacaBroker.submittedOrders[orderID]
	alpacaBroker.mu.Unlock()

	// If we have the original and only mutable fields changed, use PATCH.
	if exists && original.Asset.Ticker == order.Asset.Ticker &&
		original.Side == order.Side &&
		original.OrderType == order.OrderType {
		replacement := buildReplaceRequest(original, order)

		newID, err := alpacaBroker.client.replaceOrder(ctx, orderID, replacement)
		if err != nil {
			return fmt.Errorf("alpaca: replace order: %w", err)
		}

		alpacaBroker.mu.Lock()
		delete(alpacaBroker.submittedOrders, orderID)
		alpacaBroker.submittedOrders[newID] = order
		alpacaBroker.mu.Unlock()

		return nil
	}

	// Non-mutable field changed: cancel and resubmit.
	cancelErr := alpacaBroker.client.cancelOrder(ctx, orderID)
	if cancelErr != nil {
		var httpErr *broker.HTTPError
		if errors.As(cancelErr, &httpErr) && httpErr.StatusCode == 422 {
			return fmt.Errorf("alpaca: cancel for replace: %w", cancelErr)
		}

		return fmt.Errorf("alpaca: cancel for replace: %w", cancelErr)
	}

	alpacaBroker.mu.Lock()
	delete(alpacaBroker.submittedOrders, orderID)
	alpacaBroker.mu.Unlock()

	return alpacaBroker.Submit(ctx, order)
}

// buildReplaceRequest creates a replaceRequest containing only the fields
// that differ between the original and replacement orders.
func buildReplaceRequest(original, replacement broker.Order) replaceRequest {
	request := replaceRequest{}

	if original.Qty != replacement.Qty {
		request.Qty = formatFloat(replacement.Qty)
	}

	if original.LimitPrice != replacement.LimitPrice {
		request.LimitPrice = formatFloat(replacement.LimitPrice)
	}

	if original.StopPrice != replacement.StopPrice {
		request.StopPrice = formatFloat(replacement.StopPrice)
	}

	if original.TimeInForce != replacement.TimeInForce {
		request.TimeInForce = mapTimeInForce(replacement.TimeInForce)
	}

	return request
}

// Orders returns all open orders for the current trading day.
func (alpacaBroker *AlpacaBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	responses, err := alpacaBroker.client.getOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("alpaca: get orders: %w", err)
	}

	orders := make([]broker.Order, len(responses))
	for idx, resp := range responses {
		orders[idx] = toBrokerOrder(resp)
	}

	return orders, nil
}

// Positions returns all current positions in the account.
func (alpacaBroker *AlpacaBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	responses, err := alpacaBroker.client.getPositions(ctx)
	if err != nil {
		return nil, fmt.Errorf("alpaca: get positions: %w", err)
	}

	positions := make([]broker.Position, len(responses))
	for idx, resp := range responses {
		positions[idx] = toBrokerPosition(resp)
	}

	return positions, nil
}

// Balance returns the current account balance.
func (alpacaBroker *AlpacaBroker) Balance(ctx context.Context) (broker.Balance, error) {
	resp, err := alpacaBroker.client.getAccount(ctx)
	if err != nil {
		return broker.Balance{}, fmt.Errorf("alpaca: get balance: %w", err)
	}

	return toBrokerBalance(resp), nil
}

// SubmitGroup submits a group of orders as a native Alpaca bracket or OCO order.
func (alpacaBroker *AlpacaBroker) SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error {
	if len(orders) == 0 {
		return broker.ErrEmptyOrderGroup
	}

	switch groupType {
	case broker.GroupBracket:
		return alpacaBroker.submitBracket(ctx, orders)
	case broker.GroupOCO:
		return alpacaBroker.submitOCO(ctx, orders)
	default:
		return fmt.Errorf("alpaca: unsupported group type %d", groupType)
	}
}

func (alpacaBroker *AlpacaBroker) submitBracket(ctx context.Context, orders []broker.Order) error {
	var (
		entryOrder      *broker.Order
		takeProfitOrder *broker.Order
		stopLossOrder   *broker.Order
	)

	for idx := range orders {
		switch orders[idx].GroupRole {
		case broker.RoleEntry:
			if entryOrder != nil {
				return broker.ErrMultipleEntryOrders
			}

			entryOrder = &orders[idx]
		case broker.RoleTakeProfit:
			takeProfitOrder = &orders[idx]
		case broker.RoleStopLoss:
			stopLossOrder = &orders[idx]
		}
	}

	if entryOrder == nil {
		return broker.ErrNoEntryOrder
	}

	request := toAlpacaOrder(*entryOrder, alpacaBroker.fractional)
	request.OrderClass = "bracket"

	if takeProfitOrder != nil {
		request.TakeProfit = &takeProfitRequest{
			LimitPrice: formatFloat(takeProfitOrder.LimitPrice),
		}
	}

	if stopLossOrder != nil {
		request.StopLoss = &stopLossRequest{
			StopPrice: formatFloat(stopLossOrder.StopPrice),
		}
	}

	orderID, err := alpacaBroker.client.submitOrder(ctx, request)
	if err != nil {
		return fmt.Errorf("alpaca: submit bracket order: %w", err)
	}

	alpacaBroker.mu.Lock()
	alpacaBroker.submittedOrders[orderID] = *entryOrder
	alpacaBroker.mu.Unlock()

	return nil
}

func (alpacaBroker *AlpacaBroker) submitOCO(ctx context.Context, orders []broker.Order) error {
	if len(orders) < 2 {
		return fmt.Errorf("alpaca: OCO requires at least 2 orders")
	}

	primaryOrder := orders[0]
	dependentOrder := orders[1]

	request := toAlpacaOrder(primaryOrder, alpacaBroker.fractional)
	request.OrderClass = "oco"

	// Attach the dependent leg based on its order type.
	if dependentOrder.OrderType == broker.Limit {
		request.TakeProfit = &takeProfitRequest{
			LimitPrice: formatFloat(dependentOrder.LimitPrice),
		}
	} else {
		request.StopLoss = &stopLossRequest{
			StopPrice: formatFloat(dependentOrder.StopPrice),
		}
	}

	orderID, err := alpacaBroker.client.submitOrder(ctx, request)
	if err != nil {
		return fmt.Errorf("alpaca: submit OCO order: %w", err)
	}

	alpacaBroker.mu.Lock()
	alpacaBroker.submittedOrders[orderID] = primaryOrder
	alpacaBroker.mu.Unlock()

	return nil
}
