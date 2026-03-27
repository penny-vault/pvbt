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
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
)

type apiClient struct {
	resty  *resty.Client
	signer signer
}

// newAPIClient creates a new apiClient configured with retry, request signing, and base URL.
func newAPIClient(baseURL string, sign signer) *apiClient {
	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetRetryCount(3).
		SetRetryWaitTime(1*time.Second).
		SetRetryMaxWaitTime(4*time.Second).
		SetHeader("Content-Type", "application/json")

	httpClient.AddRetryCondition(func(resp *resty.Response, err error) bool {
		if err != nil {
			return broker.IsRetryableError(err)
		}

		return resp.StatusCode() == 429 || resp.StatusCode() >= 500
	})

	httpClient.SetPreRequestHook(func(_ *resty.Client, rawReq *http.Request) error {
		return sign.Sign(rawReq)
	})

	return &apiClient{
		resty:  httpClient,
		signer: sign,
	}
}

// submitOrderResponse wraps the Webull order placement response.
type submitOrderResponse struct {
	OrderID string `json:"order_id"`
}

// orderListResponse wraps the Webull order list response.
type orderListResponse struct {
	Orders []orderResponse `json:"orders"`
}

// positionListResponse wraps the Webull positions list response.
type positionListResponse struct {
	Positions []positionResponse `json:"positions"`
}

// getAccounts retrieves the list of accounts.
func (client *apiClient) getAccounts(ctx context.Context) ([]accountEntry, error) {
	var result accountListResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/api/trade/account/list")
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Accounts, nil
}

// submitOrder sends an order for a given account and returns the order ID.
func (client *apiClient) submitOrder(ctx context.Context, accountID string, order orderRequest) (string, error) {
	var result submitOrderResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		SetResult(&result).
		SetPathParam("account_id", accountID).
		Post("/api/trade/order/place")
	if err != nil {
		return "", fmt.Errorf("submit order: %w", err)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.OrderID, nil
}

// cancelOrder cancels an existing order.
func (client *apiClient) cancelOrder(ctx context.Context, accountID string, orderID string) error {
	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(map[string]string{
			"account_id": accountID,
			"order_id":   orderID,
		}).
		Post("/api/trade/order/cancel")
	if err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// replaceOrder modifies an existing order with new parameters.
func (client *apiClient) replaceOrder(ctx context.Context, accountID string, orderID string, replacement replaceRequest) error {
	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(replacement).
		SetQueryParam("account_id", accountID).
		SetQueryParam("order_id", orderID).
		Post("/api/trade/order/replace")
	if err != nil {
		return fmt.Errorf("replace order: %w", err)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// getOrders retrieves all orders for a given account.
func (client *apiClient) getOrders(ctx context.Context, accountID string) ([]orderResponse, error) {
	var result orderListResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		SetQueryParam("account_id", accountID).
		Get("/api/trade/order/list")
	if err != nil {
		return nil, fmt.Errorf("get orders: %w", err)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Orders, nil
}

// getPositions retrieves all positions for a given account.
func (client *apiClient) getPositions(ctx context.Context, accountID string) ([]positionResponse, error) {
	var result positionListResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		SetQueryParam("account_id", accountID).
		Get("/api/trade/account/positions")
	if err != nil {
		return nil, fmt.Errorf("get positions: %w", err)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Positions, nil
}

// getBalance retrieves the account balance details.
func (client *apiClient) getBalance(ctx context.Context, accountID string) (accountResponse, error) {
	var result accountResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		SetQueryParam("account_id", accountID).
		Get("/api/trade/account/detail")
	if err != nil {
		return accountResponse{}, fmt.Errorf("get balance: %w", err)
	}

	if resp.IsError() {
		return accountResponse{}, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result, nil
}

// Compile-time references so the unused linter does not flag the client
// infrastructure that is only consumed from the test-export bridge today
// but will be wired into the broker facade in a later task.
var (
	_ = newAPIClient
	_ = (*apiClient).getAccounts
	_ = (*apiClient).submitOrder
	_ = (*apiClient).cancelOrder
	_ = (*apiClient).replaceOrder
	_ = (*apiClient).getOrders
	_ = (*apiClient).getPositions
	_ = (*apiClient).getBalance
)
