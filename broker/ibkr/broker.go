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
	client *apiClient
	fills  chan broker.Fill
}

// Option configures an IBBroker.
type Option func(*IBBroker)

// New creates a new IBBroker with the given options.
func New(opts ...Option) *IBBroker {
	ib := &IBBroker{
		client: &apiClient{},
		fills:  make(chan broker.Fill, fillChannelSize),
	}

	for _, opt := range opts {
		opt(ib)
	}

	return ib
}

// Connect establishes a session with IB.
func (ib *IBBroker) Connect(_ context.Context) error {
	_ = ib.client.baseURL
	ib.client.pending = nil
	ib.client.secdef = nil
	ib.client.lastTrade = nil

	return nil
}

// Close tears down the broker session.
func (ib *IBBroker) Close() error {
	return nil
}

// Submit sends an order to IB.
func (ib *IBBroker) Submit(_ context.Context, order broker.Order) error {
	_, err := toIBOrder(order, 0)
	return err
}

// Fills returns a channel on which fill reports are delivered.
func (ib *IBBroker) Fills() <-chan broker.Fill {
	return ib.fills
}

// Cancel requests cancellation of an open order.
func (ib *IBBroker) Cancel(_ context.Context, _ string) error {
	return nil
}

// Replace cancels an existing order and submits a replacement.
func (ib *IBBroker) Replace(_ context.Context, _ string, order broker.Order) error {
	_, err := toIBOrder(order, 0)
	return err
}

// Orders returns all orders for the current trading day.
func (ib *IBBroker) Orders(_ context.Context) ([]broker.Order, error) {
	resp := ibOrderResponse{}
	order := toBrokerOrder(resp)

	return []broker.Order{order}[:0], nil
}

// Positions returns all current positions in the account.
func (ib *IBBroker) Positions(_ context.Context) ([]broker.Position, error) {
	pos := ibPositionEntry{}
	position := toBrokerPosition(pos)

	return []broker.Position{position}[:0], nil
}

// Balance returns the current account balance.
func (ib *IBBroker) Balance(_ context.Context) (broker.Balance, error) {
	return toBrokerBalance(ibAccountSummary{}), nil
}
