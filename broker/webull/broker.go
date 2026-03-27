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

package webull

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	productionBaseURL = "https://api.webull.com"
	uatBaseURL        = "https://us-openapi-alb.uat.webullbroker.com"
	productionGRPC    = "events-api.webull.com:443"
	uatGRPC           = "events-api.uat.webullbroker.com:443"
	productionAuthURL = "https://us-oauth-open-api.webull.com"
	uatAuthURL        = "https://us-oauth-open-api.uat.webullbroker.com"
	fillChannelSize   = 1024
)

// WebullBroker implements broker.Broker for the Webull brokerage.
type WebullBroker struct {
	client          *apiClient
	fills           chan broker.Fill
	accountID       string
	fractional      bool
	uat             bool
	tokenFile       string
	callbackURL     string
	streamer        *fillStreamer
	submittedOrders map[string]broker.Order
	mu              sync.Mutex
}

// Option configures a WebullBroker.
type Option func(*WebullBroker)

// WithUAT configures the broker to use the Webull UAT (sandbox) environment.
func WithUAT() Option { return func(wb *WebullBroker) { wb.uat = true } }

// WithFractionalShares enables dollar-amount (notional) orders.
func WithFractionalShares() Option { return func(wb *WebullBroker) { wb.fractional = true } }

// WithTokenFile configures the path to the token persistence file.
func WithTokenFile(path string) Option { return func(wb *WebullBroker) { wb.tokenFile = path } }

// WithCallbackURL configures the OAuth callback URL.
func WithCallbackURL(callbackURL string) Option {
	return func(wb *WebullBroker) { wb.callbackURL = callbackURL }
}

// WithAccountID configures the account ID to use for trading.
func WithAccountID(id string) Option { return func(wb *WebullBroker) { wb.accountID = id } }

// New creates a new WebullBroker.
func New(opts ...Option) *WebullBroker {
	wb := &WebullBroker{
		fills:           make(chan broker.Fill, fillChannelSize),
		submittedOrders: make(map[string]broker.Order),
	}

	for _, opt := range opts {
		opt(wb)
	}

	return wb
}

// Fills returns the channel on which fill reports are delivered.
func (wb *WebullBroker) Fills() <-chan broker.Fill {
	return wb.fills
}

// Connect establishes an authenticated session with Webull.
func (wb *WebullBroker) Connect(ctx context.Context) error {
	mode, modeErr := detectAuthMode()
	if modeErr != nil {
		return fmt.Errorf("webull: connect: %w", modeErr)
	}

	var sign signer

	baseURL := productionBaseURL
	if wb.uat {
		baseURL = uatBaseURL
	}

	switch mode {
	case authModeDirect:
		sign = &hmacSigner{
			appKey:    os.Getenv("WEBULL_APP_KEY"),
			appSecret: os.Getenv("WEBULL_APP_SECRET"),
		}
	case authModeOAuth:
		authURL := productionAuthURL
		if wb.uat {
			authURL = uatAuthURL
		}

		callbackURL := wb.callbackURL
		if callbackURL == "" {
			callbackURL = os.Getenv("WEBULL_CALLBACK_URL")
		}

		tokenFile := wb.tokenFile
		if tokenFile == "" {
			tokenFile = os.Getenv("WEBULL_TOKEN_FILE")
		}

		mgr := newTokenManager(
			authModeOAuth,
			os.Getenv("WEBULL_CLIENT_ID"),
			os.Getenv("WEBULL_CLIENT_SECRET"),
			callbackURL,
			tokenFile,
			authURL,
		)

		if loadErr := mgr.loadTokens(); loadErr != nil {
			return fmt.Errorf("webull: connect: load tokens: %w", loadErr)
		}

		if mgr.tokens.AccessToken == "" {
			if authErr := mgr.authorize(); authErr != nil {
				return fmt.Errorf("webull: connect: %w", authErr)
			}
		}

		mgr.startRefreshLoop()

		sign = &oauthSigner{tokenMgr: mgr}
	default:
		return fmt.Errorf("webull: connect: unknown auth mode %d", mode)
	}

	wb.client = newAPIClient(baseURL, sign)

	// Select account if not explicitly provided.
	if wb.accountID == "" {
		accounts, acctErr := wb.client.getAccounts(ctx)
		if acctErr != nil {
			return fmt.Errorf("webull: connect: list accounts: %w", acctErr)
		}

		if len(accounts) == 0 {
			return fmt.Errorf("webull: connect: %w", broker.ErrAccountNotFound)
		}

		wb.accountID = accounts[0].AccountID
	}

	grpcTarget := productionGRPC
	if wb.uat {
		grpcTarget = uatGRPC
	}

	wb.streamer = &fillStreamer{
		fills:       wb.fills,
		done:        make(chan struct{}),
		cumulFilled: make(map[string]float64),
		pollOrders: func(pollCtx context.Context) ([]orderResponse, error) {
			return wb.client.getOrders(pollCtx, wb.accountID)
		},
		grpcTarget: grpcTarget,
		sign:       sign,
		accountID:  wb.accountID,
	}

	wb.streamer.wg.Add(1)

	go wb.streamer.run(ctx)

	log.Info().Str("account_id", wb.accountID).Msg("webull: connected")

	return nil
}

// Close tears down the broker session and releases resources.
func (wb *WebullBroker) Close() error {
	if wb.streamer != nil {
		if closeErr := wb.streamer.close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("webull: streamer close failed")
		}
	}

	if wb.client != nil {
		if oaSigner, ok := wb.client.signer.(*oauthSigner); ok {
			oaSigner.Close()
		}
	}

	close(wb.fills)

	return nil
}

// Submit sends an order to Webull.
func (wb *WebullBroker) Submit(ctx context.Context, order broker.Order) error {
	if validErr := wb.validateOrder(order); validErr != nil {
		return validErr
	}

	wb.mu.Lock()
	defer wb.mu.Unlock()

	req := toWebullOrder(order, wb.fractional)

	orderID, submitErr := wb.client.submitOrder(ctx, wb.accountID, req)
	if submitErr != nil {
		return fmt.Errorf("webull: submit: %w", submitErr)
	}

	order.ID = orderID
	wb.submittedOrders[orderID] = order

	return nil
}

// Cancel requests cancellation of an open order by ID.
func (wb *WebullBroker) Cancel(ctx context.Context, orderID string) error {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	if cancelErr := wb.client.cancelOrder(ctx, wb.accountID, orderID); cancelErr != nil {
		return fmt.Errorf("webull: cancel: %w", cancelErr)
	}

	delete(wb.submittedOrders, orderID)

	return nil
}

// Replace cancels an existing order and submits a replacement atomically.
// Only qty and price may change; side, time-in-force, and order type must
// match the original.
func (wb *WebullBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	if validErr := wb.validateOrder(order); validErr != nil {
		return validErr
	}

	wb.mu.Lock()
	defer wb.mu.Unlock()

	original, exists := wb.submittedOrders[orderID]
	if exists {
		if order.Side != original.Side {
			return ErrReplaceFieldNotAllowed
		}

		if order.TimeInForce != original.TimeInForce {
			return ErrReplaceFieldNotAllowed
		}

		if order.OrderType != original.OrderType {
			return ErrReplaceFieldNotAllowed
		}
	}

	replacement := replaceRequest{
		Qty: formatFloat(order.Qty),
	}

	if order.OrderType == broker.Limit || order.OrderType == broker.StopLimit {
		replacement.LimitPrice = formatFloat(order.LimitPrice)
	}

	if order.OrderType == broker.Stop || order.OrderType == broker.StopLimit {
		replacement.StopPrice = formatFloat(order.StopPrice)
	}

	if replaceErr := wb.client.replaceOrder(ctx, wb.accountID, orderID, replacement); replaceErr != nil {
		return fmt.Errorf("webull: replace: %w", replaceErr)
	}

	order.ID = orderID
	wb.submittedOrders[orderID] = order

	return nil
}

// Orders returns all orders for the current trading day.
func (wb *WebullBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	rawOrders, getErr := wb.client.getOrders(ctx, wb.accountID)
	if getErr != nil {
		return nil, fmt.Errorf("webull: get orders: %w", getErr)
	}

	orders := make([]broker.Order, len(rawOrders))
	for ii, raw := range rawOrders {
		orders[ii] = toBrokerOrder(raw)
	}

	return orders, nil
}

// Positions returns all current positions in the account.
func (wb *WebullBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	rawPositions, getErr := wb.client.getPositions(ctx, wb.accountID)
	if getErr != nil {
		return nil, fmt.Errorf("webull: get positions: %w", getErr)
	}

	positions := make([]broker.Position, len(rawPositions))
	for ii, raw := range rawPositions {
		positions[ii] = toBrokerPosition(raw)
	}

	return positions, nil
}

// Balance returns the current account balance.
func (wb *WebullBroker) Balance(ctx context.Context) (broker.Balance, error) {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	rawBalance, getErr := wb.client.getBalance(ctx, wb.accountID)
	if getErr != nil {
		return broker.Balance{}, fmt.Errorf("webull: get balance: %w", getErr)
	}

	return toBrokerBalance(rawBalance), nil
}

// Transactions returns account activity since the given time. Webull's Open
// API does not currently expose a transactions endpoint, so this returns an
// empty slice.
func (wb *WebullBroker) Transactions(_ context.Context, _ time.Time) ([]broker.Transaction, error) {
	log.Info().Msg("webull: transactions endpoint not available in Webull Open API")
	return nil, nil
}

// validateOrder checks that the order uses supported parameters.
func (wb *WebullBroker) validateOrder(order broker.Order) error {
	// Webull only supports Day and GTC time-in-force.
	if order.TimeInForce != broker.Day && order.TimeInForce != broker.GTC {
		return ErrUnsupportedTimeInForce
	}

	// Dollar-amount orders require fractional shares to be enabled.
	if order.Qty == 0 && order.Amount > 0 {
		if !wb.fractional {
			return ErrFractionalNotEnabled
		}

		if order.OrderType != broker.Market {
			return ErrFractionalNotMarket
		}
	}

	return nil
}
