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
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/broker"
)

const (
	fillChannelSize  = 1024
	ocaTypeCancelAll = 1
)

// IBBroker implements broker.Broker for the Interactive Brokers brokerage.
type IBBroker struct {
	client     *apiClient
	fills      chan broker.Fill
	streamer   *orderStreamer
	accountID  string
	conidCache map[string]int64
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

// WithOAuth configures the broker to authenticate via IB's OAuth flow
// using the given consumer key and RSA private key file.
func WithOAuth(baseURL, consumerKey, keyFile string) Option {
	return func(ib *IBBroker) {
		auth := newOAuthAuthenticator(consumerKey, keyFile)
		auth.baseURL = baseURL
		ib.client = newAPIClient(baseURL, auth)
	}
}

// New creates a new IBBroker with the given options.
func New(opts ...Option) *IBBroker {
	ib := &IBBroker{
		client:     newAPIClient("", nil),
		fills:      make(chan broker.Fill, fillChannelSize),
		conidCache: make(map[string]int64),
	}

	for _, opt := range opts {
		opt(ib)
	}

	return ib
}

// Connect establishes a session with IB and starts the order streamer.
func (ib *IBBroker) Connect(ctx context.Context) error {
	ib.client.pending = nil
	ib.client.secdef = nil
	ib.client.lastTrade = nil

	accountID, resolveErr := ib.client.resolveAccount(ctx)
	if resolveErr != nil {
		return resolveErr
	}

	ib.accountID = accountID

	wsURL := toWebSocketURL(ib.client.resty.BaseURL)
	ib.streamer = newOrderStreamer(ib.fills, wsURL, ib.PollTrades)

	if connectErr := ib.streamer.connect(ctx); connectErr != nil {
		return connectErr
	}

	return nil
}

// Close tears down the broker session and stops the order streamer.
func (ib *IBBroker) Close() error {
	if ib.streamer != nil {
		return ib.streamer.close()
	}

	return nil
}

// resolveConid looks up the contract ID for a ticker, returning a cached
// value when available. If the symbol is unknown, ErrConidNotFound is returned.
func (ib *IBBroker) resolveConid(ctx context.Context, ticker string) (int64, error) {
	if conid, cached := ib.conidCache[ticker]; cached {
		return conid, nil
	}

	results, searchErr := ib.client.searchSecdef(ctx, ticker)
	if searchErr != nil {
		return 0, searchErr
	}

	if len(results) == 0 {
		return 0, ErrConidNotFound
	}

	conid := results[0].Conid
	ib.conidCache[ticker] = conid

	return conid, nil
}

// Submit sends an order to IB. It resolves the ticker to a contract ID,
// fetches a real-time snapshot for dollar-amount orders, submits the order,
// and auto-confirms any warning replies from the gateway.
func (ib *IBBroker) Submit(ctx context.Context, order broker.Order) error {
	conid, resolveErr := ib.resolveConid(ctx, order.Asset.Ticker)
	if resolveErr != nil {
		return resolveErr
	}

	if order.Qty == 0 && order.Amount > 0 {
		price, snapErr := ib.client.getSnapshot(ctx, conid)
		if snapErr != nil {
			return snapErr
		}

		order.Qty = math.Floor(order.Amount / price)
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

// SubmitGroup submits a group of orders as a native bracket or OCA order.
func (ib *IBBroker) SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error {
	if len(orders) == 0 {
		return broker.ErrEmptyOrderGroup
	}

	switch groupType {
	case broker.GroupBracket:
		return ib.submitBracket(ctx, orders)
	case broker.GroupOCO:
		return ib.submitOCA(ctx, orders)
	default:
		return fmt.Errorf("ibkr: unsupported group type %d", groupType)
	}
}

// submitBracket submits a bracket order group. The entry order receives a
// client order ID (cOID), and each child (stop-loss / take-profit) references
// the entry via parentId.
func (ib *IBBroker) submitBracket(ctx context.Context, orders []broker.Order) error {
	// Validate: exactly one entry order.
	entryIdx := -1

	for idx, order := range orders {
		if order.GroupRole == broker.RoleEntry {
			if entryIdx >= 0 {
				return broker.ErrMultipleEntryOrders
			}

			entryIdx = idx
		}
	}

	if entryIdx < 0 {
		return broker.ErrNoEntryOrder
	}

	entryCOID := uuid.New().String()

	ibOrders := make([]ibOrderRequest, 0, len(orders))

	for idx, order := range orders {
		conid, resolveErr := ib.resolveConid(ctx, order.Asset.Ticker)
		if resolveErr != nil {
			return resolveErr
		}

		ibReq, mapErr := toIBOrder(order, conid)
		if mapErr != nil {
			return mapErr
		}

		if idx == entryIdx {
			ibReq.COID = entryCOID
		} else {
			ibReq.ParentId = entryCOID
		}

		ibOrders = append(ibOrders, ibReq)
	}

	replies, submitErr := ib.client.submitOrder(ctx, ib.accountID, ibOrders)
	if submitErr != nil {
		return submitErr
	}

	for _, reply := range replies {
		if reply.ReplyID != "" {
			if _, confirmErr := ib.client.confirmReply(ctx, reply.ReplyID, true); confirmErr != nil {
				return confirmErr
			}
		}
	}

	return nil
}

// submitOCA submits a One-Cancels-All order group. All orders share the same
// ocaGroup identifier and ocaType=1 (cancel remaining).
func (ib *IBBroker) submitOCA(ctx context.Context, orders []broker.Order) error {
	ocaGroup := uuid.New().String()

	ibOrders := make([]ibOrderRequest, 0, len(orders))

	for _, order := range orders {
		conid, resolveErr := ib.resolveConid(ctx, order.Asset.Ticker)
		if resolveErr != nil {
			return resolveErr
		}

		ibReq, mapErr := toIBOrder(order, conid)
		if mapErr != nil {
			return mapErr
		}

		ibReq.OcaGroup = ocaGroup
		ibReq.OcaType = ocaTypeCancelAll

		ibOrders = append(ibOrders, ibReq)
	}

	replies, submitErr := ib.client.submitOrder(ctx, ib.accountID, ibOrders)
	if submitErr != nil {
		return submitErr
	}

	for _, reply := range replies {
		if reply.ReplyID != "" {
			if _, confirmErr := ib.client.confirmReply(ctx, reply.ReplyID, true); confirmErr != nil {
				return confirmErr
			}
		}
	}

	return nil
}

// PollTrades fetches recent executions from the IB API. This is used
// to recover missed fills after a WebSocket reconnect.
func (ib *IBBroker) PollTrades(ctx context.Context) ([]ibTradeEntry, error) {
	return ib.client.getTrades(ctx)
}

// toWebSocketURL converts an HTTP base URL to its WebSocket equivalent,
// appending the /v1/api/ws path used by the IB Client Portal Gateway.
func toWebSocketURL(baseURL string) string {
	wsURL := strings.Replace(baseURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)

	return wsURL + "/v1/api/ws"
}
