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

package ibkr

import (
	"context"

	"github.com/penny-vault/pvbt/broker"
)

const (
	fillChannelSize = 1024
)

// IBBroker implements broker.Broker for the Interactive Brokers brokerage.
type IBBroker struct {
	client    *apiClient
	fills     chan broker.Fill
	accountID string
}

// Option configures an IBBroker.
type Option func(*IBBroker)

// WithGateway configures the broker to authenticate via the IB Client
// Portal Gateway at the given base URL.
func WithGateway(baseURL string) Option {
	return func(ib *IBBroker) {
		auth := newGatewayAuthenticator(baseURL)
		ib.client = newAPIClient(baseURL, auth)
	}
}

// New creates a new IBBroker with the given options.
func New(opts ...Option) *IBBroker {
	ib := &IBBroker{
		client: newAPIClient("", nil),
		fills:  make(chan broker.Fill, fillChannelSize),
	}

	for _, opt := range opts {
		opt(ib)
	}

	return ib
}

// Connect establishes a session with IB.
func (ib *IBBroker) Connect(ctx context.Context) error {
	ib.client.pending = nil
	ib.client.secdef = nil
	ib.client.lastTrade = nil

	accountID, resolveErr := ib.client.resolveAccount(ctx)
	if resolveErr != nil {
		return resolveErr
	}

	ib.accountID = accountID

	return nil
}

// Close tears down the broker session.
func (ib *IBBroker) Close() error {
	return nil
}

// Submit sends an order to IB. It resolves the ticker to a contract ID,
// fetches a real-time snapshot for dollar-amount orders, submits the order,
// and auto-confirms any warning replies from the gateway.
func (ib *IBBroker) Submit(ctx context.Context, order broker.Order) error {
	results, searchErr := ib.client.searchSecdef(ctx, order.Asset.Ticker)
	if searchErr != nil {
		return searchErr
	}

	if len(results) == 0 {
		return ErrConidNotFound
	}

	conid := results[0].Conid

	if order.Qty == 0 && order.Amount > 0 {
		price, snapErr := ib.client.getSnapshot(ctx, conid)
		if snapErr != nil {
			return snapErr
		}

		order.Qty = order.Amount / price
	}

	ibOrder, mapErr := toIBOrder(order, conid)
	if mapErr != nil {
		return mapErr
	}

	replies, submitErr := ib.client.submitOrder(ctx, ib.accountID, []ibOrderRequest{ibOrder})
	if submitErr != nil {
		return submitErr
	}

	// Auto-confirm warning messages from the gateway.
	for _, reply := range replies {
		if reply.ReplyID != "" {
			if _, confirmErr := ib.client.confirmReply(ctx, reply.ReplyID, true); confirmErr != nil {
				return confirmErr
			}
		}
	}

	return nil
}

// Fills returns a channel on which fill reports are delivered.
func (ib *IBBroker) Fills() <-chan broker.Fill {
	return ib.fills
}

// Cancel requests cancellation of an open order.
func (ib *IBBroker) Cancel(ctx context.Context, orderID string) error {
	return ib.client.cancelOrder(ctx, ib.accountID, orderID)
}

// Replace cancels an existing order and submits a replacement.
func (ib *IBBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	ibOrder, mapErr := toIBOrder(order, 0)
	if mapErr != nil {
		return mapErr
	}

	_, replaceErr := ib.client.replaceOrder(ctx, ib.accountID, orderID, ibOrder)

	return replaceErr
}

// Orders returns all orders for the current trading day.
func (ib *IBBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	ibOrders, getErr := ib.client.getOrders(ctx)
	if getErr != nil {
		return nil, getErr
	}

	orders := make([]broker.Order, 0, len(ibOrders))
	for _, ibOrder := range ibOrders {
		orders = append(orders, toBrokerOrder(ibOrder))
	}

	return orders, nil
}

// Positions returns all current positions in the account.
func (ib *IBBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	ibPositions, getErr := ib.client.getPositions(ctx, ib.accountID)
	if getErr != nil {
		return nil, getErr
	}

	positions := make([]broker.Position, 0, len(ibPositions))
	for _, pos := range ibPositions {
		positions = append(positions, toBrokerPosition(pos))
	}

	return positions, nil
}

// Balance returns the current account balance.
func (ib *IBBroker) Balance(ctx context.Context) (broker.Balance, error) {
	summary, getErr := ib.client.getBalance(ctx, ib.accountID)
	if getErr != nil {
		return broker.Balance{}, getErr
	}

	return toBrokerBalance(summary), nil
}

// PollTrades fetches recent executions from the IB API. This is used
// to recover missed fills after a WebSocket reconnect.
func (ib *IBBroker) PollTrades(ctx context.Context) ([]ibTradeEntry, error) {
	return ib.client.getTrades(ctx)
}
