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
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
)

// refresher is implemented by signers that can transparently refresh expired
// credentials (e.g. oauthSigner). hmacSigner does not implement this.
type refresher interface {
	Refresh() error
}

type apiClient struct {
	resty     *resty.Client
	signer    signer
	refresher refresher // non-nil when signer supports token refresh
}

// newAPIClient creates a new apiClient configured with retry, request signing, and base URL.
func newAPIClient(baseURL string, sign signer) *apiClient {
	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetRetryCount(3).
		SetRetryWaitTime(1*time.Second).
		SetRetryMaxWaitTime(30*time.Second).
		SetHeader("Content-Type", "application/json")

	httpClient.AddRetryCondition(func(resp *resty.Response, err error) bool {
		if err != nil {
			return broker.IsRetryableError(err)
		}

		return resp.StatusCode() == http.StatusTooManyRequests || resp.StatusCode() >= 500
	})

	// Respect Retry-After header on 429 responses.
	httpClient.SetRetryAfter(func(_ *resty.Client, resp *resty.Response) (time.Duration, error) {
		if resp == nil || resp.StatusCode() != http.StatusTooManyRequests {
			return 0, nil
		}

		retryAfter := resp.Header().Get("Retry-After")
		if retryAfter == "" {
			return 0, nil
		}

		// Try parsing as seconds first.
		if seconds, parseErr := strconv.Atoi(retryAfter); parseErr == nil {
			return time.Duration(seconds) * time.Second, nil
		}

		// Try parsing as HTTP-date.
		if retryTime, parseErr := http.ParseTime(retryAfter); parseErr == nil {
			delay := time.Until(retryTime)
			if delay > 0 {
				return delay, nil
			}
		}

		return 0, nil
	})

	httpClient.SetPreRequestHook(func(_ *resty.Client, rawReq *http.Request) error {
		return sign.Sign(rawReq)
	})

	ac := &apiClient{
		resty:  httpClient,
		signer: sign,
	}

	// If the signer supports token refresh, store it for auth failure retry.
	if rf, ok := sign.(refresher); ok {
		ac.refresher = rf
	}

	return ac
}

// isAuthFailure returns true if the HTTP status code indicates an authentication
// or authorization failure that may be recoverable via token refresh.
func isAuthFailure(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

// refreshAndRetry attempts to refresh credentials and returns true if the
// caller should retry the request. Returns false if no refresher is configured
// or the refresh itself fails.
func (client *apiClient) refreshAndRetry(statusCode int) bool {
	if client.refresher == nil {
		return false
	}

	if !isAuthFailure(statusCode) {
		return false
	}

	return client.refresher.Refresh() == nil
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
		if client.refreshAndRetry(resp.StatusCode()) {
			result = accountListResponse{}

			resp, err = client.resty.R().
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
		if client.refreshAndRetry(resp.StatusCode()) {
			result = submitOrderResponse{}

			resp, err = client.resty.R().
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

		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.OrderID, nil
}

// cancelOrder cancels an existing order.
func (client *apiClient) cancelOrder(ctx context.Context, accountID string, orderID string) error {
	body := map[string]string{
		"account_id": accountID,
		"order_id":   orderID,
	}

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(body).
		Post("/api/trade/order/cancel")
	if err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}

	if resp.IsError() {
		if client.refreshAndRetry(resp.StatusCode()) {
			resp, err = client.resty.R().
				SetContext(ctx).
				SetBody(body).
				Post("/api/trade/order/cancel")
			if err != nil {
				return fmt.Errorf("cancel order: %w", err)
			}

			if resp.IsError() {
				return broker.NewHTTPError(resp.StatusCode(), resp.String())
			}

			return nil
		}

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
		if client.refreshAndRetry(resp.StatusCode()) {
			resp, err = client.resty.R().
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
		if client.refreshAndRetry(resp.StatusCode()) {
			result = orderListResponse{}

			resp, err = client.resty.R().
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
		if client.refreshAndRetry(resp.StatusCode()) {
			result = positionListResponse{}

			resp, err = client.resty.R().
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
		if client.refreshAndRetry(resp.StatusCode()) {
			result = accountResponse{}

			resp, err = client.resty.R().
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

		return accountResponse{}, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result, nil
}
