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
	"time"

	"github.com/penny-vault/pvbt/broker"
)

// APIClientForTestType is an exported alias so the _test package can name the type.
type APIClientForTestType = apiClient

// OrderRequest is an exported alias for orderRequest, used in tests.
type OrderRequestExport = orderRequest

// OrderResponse is an exported alias for orderResponse, used in tests.
type OrderResponseExport = orderResponse

// ReplaceRequest is an exported alias for replaceRequest, used in tests.
type ReplaceRequestExport = replaceRequest

// AccountResponse is an exported alias for accountResponse, used in tests.
type AccountResponseExport = accountResponse

// PositionResponse is an exported alias for positionResponse, used in tests.
type PositionResponseExport = positionResponse

// LatestTradeResponse is an exported alias for latestTradeResponse, used in tests.
type LatestTradeResponseExport = latestTradeResponse

// HTTPError is a type alias for broker.HTTPError.
type HTTPError = broker.HTTPError

// NewAPIClientForTest creates an apiClient pointing at a custom base URL.
func NewAPIClientForTest(baseURL, apiKey, apiSecret string) *apiClient {
	return newAPIClient(baseURL, apiKey, apiSecret)
}

// GetAccount exposes getAccount for testing.
func (client *apiClient) GetAccount(ctx context.Context) (accountResponse, error) {
	return client.getAccount(ctx)
}

// SubmitOrder exposes submitOrder for testing.
func (client *apiClient) SubmitOrder(ctx context.Context, order orderRequest) (string, error) {
	return client.submitOrder(ctx, order)
}

// CancelOrder exposes cancelOrder for testing.
func (client *apiClient) CancelOrder(ctx context.Context, orderID string) error {
	return client.cancelOrder(ctx, orderID)
}

// ReplaceOrder exposes replaceOrder for testing.
func (client *apiClient) ReplaceOrder(ctx context.Context, orderID string, req replaceRequest) (string, error) {
	return client.replaceOrder(ctx, orderID, req)
}

// GetOrders exposes getOrders for testing.
func (client *apiClient) GetOrders(ctx context.Context) ([]orderResponse, error) {
	return client.getOrders(ctx)
}

// GetPositions exposes getPositions for testing.
func (client *apiClient) GetPositions(ctx context.Context) ([]positionResponse, error) {
	return client.getPositions(ctx)
}

// GetLatestTrade exposes getLatestTrade for testing.
func (client *apiClient) GetLatestTrade(ctx context.Context, symbol string) (float64, error) {
	return client.getLatestTrade(ctx, symbol)
}

// --- Fill streamer test exports ---

// FillStreamerForTestType is an exported alias so the _test package can name the type.
type FillStreamerForTestType = fillStreamer

// NewFillStreamerForTest creates a fillStreamer for testing.
func NewFillStreamerForTest(client *apiClient, fills chan broker.Fill, wsURL, apiKey, apiSecret string) *fillStreamer {
	return &fillStreamer{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		fills:     fills,
		wsURL:     wsURL,
		seenFills: make(map[string]time.Time),
		done:      make(chan struct{}),
		client:    client,
	}
}

// ConnectStreamer exposes connect for testing.
func (streamer *fillStreamer) ConnectStreamer(ctx context.Context) error {
	return streamer.connect(ctx)
}

// CloseStreamer exposes close for testing.
func (streamer *fillStreamer) CloseStreamer() error {
	return streamer.close()
}
