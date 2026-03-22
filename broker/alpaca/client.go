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
	"fmt"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
)

type apiClient struct {
	resty     *resty.Client
	apiKey    string
	apiSecret string
}

// newAPIClient creates a new apiClient configured with retry, auth headers, and base URL.
func newAPIClient(baseURL, apiKey, apiSecret string) *apiClient {
	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetRetryCount(3).
		SetRetryWaitTime(1*time.Second).
		SetRetryMaxWaitTime(4*time.Second).
		SetHeader("APCA-API-KEY-ID", apiKey).
		SetHeader("APCA-API-SECRET-KEY", apiSecret).
		SetHeader("Content-Type", "application/json")

	httpClient.AddRetryCondition(func(resp *resty.Response, err error) bool {
		if err != nil {
			return broker.IsTransient(err)
		}

		return resp.StatusCode() == 429 || resp.StatusCode() >= 500
	})

	return &apiClient{
		resty:     httpClient,
		apiKey:    apiKey,
		apiSecret: apiSecret,
	}
}

// getAccount retrieves the account details.
func (client *apiClient) getAccount(ctx context.Context) (accountResponse, error) {
	var result accountResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/v2/account")
	if err != nil {
		return accountResponse{}, fmt.Errorf("get account: %w", err)
	}

	if resp.IsError() {
		return accountResponse{}, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result, nil
}

// submitOrder sends an order and returns the order ID.
func (client *apiClient) submitOrder(ctx context.Context, order orderRequest) (string, error) {
	var result orderResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		SetResult(&result).
		Post("/v2/orders")
	if err != nil {
		return "", fmt.Errorf("submit order: %w", err)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.ID, nil
}

// cancelOrder deletes an existing order.
func (client *apiClient) cancelOrder(ctx context.Context, orderID string) error {
	resp, err := client.resty.R().
		SetContext(ctx).
		Delete(fmt.Sprintf("/v2/orders/%s", orderID))
	if err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// replaceOrder updates an existing order with new parameters and returns the new order ID.
func (client *apiClient) replaceOrder(ctx context.Context, orderID string, replacement replaceRequest) (string, error) {
	var result orderResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(replacement).
		SetResult(&result).
		Patch(fmt.Sprintf("/v2/orders/%s", orderID))
	if err != nil {
		return "", fmt.Errorf("replace order: %w", err)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.ID, nil
}

// getOrders retrieves all open orders.
func (client *apiClient) getOrders(ctx context.Context) ([]orderResponse, error) {
	var result []orderResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/v2/orders?status=open&limit=500")
	if err != nil {
		return nil, fmt.Errorf("get orders: %w", err)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result, nil
}

// getPositions retrieves all positions.
func (client *apiClient) getPositions(ctx context.Context) ([]positionResponse, error) {
	var result []positionResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/v2/positions")
	if err != nil {
		return nil, fmt.Errorf("get positions: %w", err)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result, nil
}

// getLatestTrade retrieves the last trade price for a symbol.
func (client *apiClient) getLatestTrade(ctx context.Context, symbol string) (float64, error) {
	var result latestTradeResponse

	endpoint := fmt.Sprintf("/v2/stocks/%s/trades/latest", url.PathEscape(symbol))

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get(endpoint)
	if err != nil {
		return 0, fmt.Errorf("get latest trade: %w", err)
	}

	if resp.IsError() {
		return 0, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return parseFloat(result.Trade.Price), nil
}
