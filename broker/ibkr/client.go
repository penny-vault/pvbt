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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
	"golang.org/x/time/rate"
)

const (
	clientTimeout    = 15 * time.Second
	retryCount       = 3
	retryWaitTime    = 1 * time.Second
	retryMaxWaitTime = 4 * time.Second
	rateLimit        = 9 // requests per second
)

// apiClient handles HTTP communication with the IB Web API.
type apiClient struct {
	resty   *resty.Client
	limiter *rate.Limiter
	auth    Authenticator

	// pending holds the last order reply awaiting confirmation.
	pending *ibOrderReply
	// secdef caches the most recently resolved contract definition.
	secdef *ibSecdefResult
	// lastTrade holds the most recent trade entry received from the streamer.
	lastTrade *ibTradeEntry
}

func newAPIClient(baseURL string, auth Authenticator) *apiClient {
	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetTimeout(clientTimeout).
		SetRetryCount(retryCount).
		SetRetryWaitTime(retryWaitTime).
		SetRetryMaxWaitTime(retryMaxWaitTime).
		SetHeader("Content-Type", "application/json")

	httpClient.AddRetryCondition(func(resp *resty.Response, retryErr error) bool {
		if retryErr != nil {
			return broker.IsRetryableError(retryErr)
		}

		return resp.StatusCode() >= 500 || resp.StatusCode() == http.StatusTooManyRequests
	})

	httpClient.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		if auth != nil {
			return auth.Decorate(req.RawRequest)
		}

		return nil
	})

	return &apiClient{
		resty:   httpClient,
		limiter: rate.NewLimiter(rate.Limit(rateLimit), 1),
		auth:    auth,
	}
}

// checkResponse inspects the HTTP response and returns an appropriate error
// for non-2xx status codes. A 429 wraps broker.ErrRateLimited so callers can
// distinguish throttling from other failures.
func checkResponse(resp *resty.Response) error {
	if resp.IsSuccess() {
		return nil
	}

	if resp.StatusCode() == http.StatusTooManyRequests {
		return fmt.Errorf("ibkr: HTTP 429: %w", broker.ErrRateLimited)
	}

	return broker.NewHTTPError(resp.StatusCode(), resp.String())
}

// decodeBody unmarshals a JSON response body into the given target.
func decodeBody(resp *resty.Response, target any) error {
	if unmarshalErr := json.Unmarshal(resp.Body(), target); unmarshalErr != nil {
		return fmt.Errorf("decode response: %w", unmarshalErr)
	}

	return nil
}

// resolveAccount returns the first account ID associated with the session.
func (ac *apiClient) resolveAccount(ctx context.Context) (string, error) {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return "", fmt.Errorf("resolve account rate limit: %w", waitErr)
	}

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		Get("/iserver/accounts")
	if reqErr != nil {
		return "", fmt.Errorf("resolve account: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return "", fmt.Errorf("resolve account: %w", respErr)
	}

	var result struct {
		Accounts []string `json:"accounts"`
	}

	if decodeErr := decodeBody(resp, &result); decodeErr != nil {
		return "", fmt.Errorf("resolve account: %w", decodeErr)
	}

	if len(result.Accounts) == 0 {
		return "", fmt.Errorf("resolve account: %w", broker.ErrAccountNotFound)
	}

	return result.Accounts[0], nil
}

// submitOrder posts an array of orders and returns the reply objects.
func (ac *apiClient) submitOrder(ctx context.Context, accountID string, orders []ibOrderRequest) ([]ibOrderReply, error) {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return nil, fmt.Errorf("submit order rate limit: %w", waitErr)
	}

	endpoint := fmt.Sprintf("/iserver/account/%s/orders", accountID)

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		SetBody(orders).
		Post(endpoint)
	if reqErr != nil {
		return nil, fmt.Errorf("submit order: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return nil, fmt.Errorf("submit order: %w", respErr)
	}

	var replies []ibOrderReply
	if decodeErr := decodeBody(resp, &replies); decodeErr != nil {
		return nil, fmt.Errorf("submit order: %w", decodeErr)
	}

	return replies, nil
}

// cancelOrder sends a DELETE to cancel the specified order.
func (ac *apiClient) cancelOrder(ctx context.Context, accountID string, orderID string) error {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return fmt.Errorf("cancel order rate limit: %w", waitErr)
	}

	endpoint := fmt.Sprintf("/iserver/account/%s/order/%s", accountID, orderID)

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		Delete(endpoint)
	if reqErr != nil {
		return fmt.Errorf("cancel order: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return fmt.Errorf("cancel order: %w", respErr)
	}

	return nil
}

// replaceOrder modifies an existing order in place.
func (ac *apiClient) replaceOrder(ctx context.Context, accountID string, orderID string, order ibOrderRequest) ([]ibOrderReply, error) {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return nil, fmt.Errorf("replace order rate limit: %w", waitErr)
	}

	endpoint := fmt.Sprintf("/iserver/account/%s/order/%s", accountID, orderID)

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		SetBody(order).
		Put(endpoint)
	if reqErr != nil {
		return nil, fmt.Errorf("replace order: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return nil, fmt.Errorf("replace order: %w", respErr)
	}

	var replies []ibOrderReply
	if decodeErr := decodeBody(resp, &replies); decodeErr != nil {
		return nil, fmt.Errorf("replace order: %w", decodeErr)
	}

	return replies, nil
}

// getOrders fetches all live orders for the session.
func (ac *apiClient) getOrders(ctx context.Context) ([]ibOrderResponse, error) {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return nil, fmt.Errorf("get orders rate limit: %w", waitErr)
	}

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		Get("/iserver/account/orders")
	if reqErr != nil {
		return nil, fmt.Errorf("get orders: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return nil, fmt.Errorf("get orders: %w", respErr)
	}

	var result struct {
		Orders []ibOrderResponse `json:"orders"`
	}

	if decodeErr := decodeBody(resp, &result); decodeErr != nil {
		return nil, fmt.Errorf("get orders: %w", decodeErr)
	}

	return result.Orders, nil
}

// getPositions fetches all positions for the given account.
func (ac *apiClient) getPositions(ctx context.Context, accountID string) ([]ibPositionEntry, error) {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return nil, fmt.Errorf("get positions rate limit: %w", waitErr)
	}

	endpoint := fmt.Sprintf("/portfolio/%s/positions/0", accountID)

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		Get(endpoint)
	if reqErr != nil {
		return nil, fmt.Errorf("get positions: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return nil, fmt.Errorf("get positions: %w", respErr)
	}

	var positions []ibPositionEntry
	if decodeErr := decodeBody(resp, &positions); decodeErr != nil {
		return nil, fmt.Errorf("get positions: %w", decodeErr)
	}

	return positions, nil
}

// getBalance fetches the account summary for the given account.
func (ac *apiClient) getBalance(ctx context.Context, accountID string) (ibAccountSummary, error) {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return ibAccountSummary{}, fmt.Errorf("get balance rate limit: %w", waitErr)
	}

	endpoint := fmt.Sprintf("/portfolio/%s/summary", accountID)

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		Get(endpoint)
	if reqErr != nil {
		return ibAccountSummary{}, fmt.Errorf("get balance: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return ibAccountSummary{}, fmt.Errorf("get balance: %w", respErr)
	}

	var summary ibAccountSummary
	if decodeErr := decodeBody(resp, &summary); decodeErr != nil {
		return ibAccountSummary{}, fmt.Errorf("get balance: %w", decodeErr)
	}

	return summary, nil
}

// searchSecdef resolves a ticker symbol to contract definitions.
func (ac *apiClient) searchSecdef(ctx context.Context, symbol string) ([]ibSecdefResult, error) {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return nil, fmt.Errorf("search secdef rate limit: %w", waitErr)
	}

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		SetBody(map[string]string{"symbol": symbol}).
		Post("/iserver/secdef/search")
	if reqErr != nil {
		return nil, fmt.Errorf("search secdef: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return nil, fmt.Errorf("search secdef: %w", respErr)
	}

	var results []ibSecdefResult
	if decodeErr := decodeBody(resp, &results); decodeErr != nil {
		return nil, fmt.Errorf("search secdef: %w", decodeErr)
	}

	return results, nil
}

// getSnapshot fetches the last price for a contract. IB returns field "31"
// (last price) as a string, so we parse it to float64.
func (ac *apiClient) getSnapshot(ctx context.Context, conid int64) (float64, error) {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return 0, fmt.Errorf("get snapshot rate limit: %w", waitErr)
	}

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"conids": strconv.FormatInt(conid, 10),
			"fields": "31",
		}).
		Get("/iserver/marketdata/snapshot")
	if reqErr != nil {
		return 0, fmt.Errorf("get snapshot: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return 0, fmt.Errorf("get snapshot: %w", respErr)
	}

	var snapshots []map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(resp.Body(), &snapshots); unmarshalErr != nil {
		return 0, fmt.Errorf("get snapshot parse: %w", unmarshalErr)
	}

	if len(snapshots) == 0 {
		return 0, fmt.Errorf("get snapshot: no data for conid %d", conid)
	}

	rawPrice, exists := snapshots[0]["31"]
	if !exists {
		return 0, fmt.Errorf("get snapshot: field 31 not present for conid %d", conid)
	}

	// IB returns the value as a JSON string (e.g. "155.25").
	var priceStr string
	if unmarshalErr := json.Unmarshal(rawPrice, &priceStr); unmarshalErr != nil {
		return 0, fmt.Errorf("get snapshot parse price: %w", unmarshalErr)
	}

	price, parseErr := strconv.ParseFloat(priceStr, 64)
	if parseErr != nil {
		return 0, fmt.Errorf("get snapshot parse price %q: %w", priceStr, parseErr)
	}

	return price, nil
}

// confirmReply confirms a message reply from the order-submission flow.
func (ac *apiClient) confirmReply(ctx context.Context, replyID string, confirmed bool) ([]ibOrderReply, error) {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return nil, fmt.Errorf("confirm reply rate limit: %w", waitErr)
	}

	endpoint := fmt.Sprintf("/iserver/reply/%s", replyID)

	body := map[string]bool{"confirmed": confirmed}

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		SetBody(body).
		Post(endpoint)
	if reqErr != nil {
		return nil, fmt.Errorf("confirm reply: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return nil, fmt.Errorf("confirm reply: %w", respErr)
	}

	var replies []ibOrderReply
	if decodeErr := decodeBody(resp, &replies); decodeErr != nil {
		return nil, fmt.Errorf("confirm reply: %w", decodeErr)
	}

	return replies, nil
}

// getTrades fetches recent trades (executions) for the session.
func (ac *apiClient) getTrades(ctx context.Context) ([]ibTradeEntry, error) {
	if waitErr := ac.limiter.Wait(ctx); waitErr != nil {
		return nil, fmt.Errorf("get trades rate limit: %w", waitErr)
	}

	resp, reqErr := ac.resty.R().
		SetContext(ctx).
		Get("/iserver/account/trades")
	if reqErr != nil {
		return nil, fmt.Errorf("get trades: %w", reqErr)
	}

	if respErr := checkResponse(resp); respErr != nil {
		return nil, fmt.Errorf("get trades: %w", respErr)
	}

	var trades []ibTradeEntry
	if decodeErr := decodeBody(resp, &trades); decodeErr != nil {
		return nil, fmt.Errorf("get trades: %w", decodeErr)
	}

	return trades, nil
}
