package tradestation

import (
	"context"
	"fmt"
	"github.com/bytedance/sonic"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
)

type apiClient struct {
	resty     *resty.Client
	accountID string
}

func newAPIClient(baseURL, token string) *apiClient {
	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetRetryCount(3).
		SetRetryWaitTime(1*time.Second).
		SetRetryMaxWaitTime(4*time.Second).
		SetAuthToken(token).
		SetHeader("Content-Type", "application/json")

	httpClient.AddRetryCondition(func(resp *resty.Response, retryErr error) bool {
		if retryErr != nil {
			return broker.IsRetryableError(retryErr)
		}

		return resp.StatusCode() >= 500 || resp.StatusCode() == 429
	})

	return &apiClient{
		resty: httpClient,
	}
}

func (client *apiClient) setToken(token string) {
	client.resty.SetAuthToken(token)
}

func (client *apiClient) resolveAccount(ctx context.Context, desiredAccountID string) (string, error) {
	var accounts []tsAccountEntry

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&accounts).
		Get("/v3/brokerage/accounts")
	if reqErr != nil {
		return "", fmt.Errorf("resolve account: %w", reqErr)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	if len(accounts) == 0 {
		return "", ErrAccountNotFound
	}

	if desiredAccountID == "" {
		return accounts[0].AccountID, nil
	}

	for _, account := range accounts {
		if account.AccountID == desiredAccountID {
			return account.AccountID, nil
		}
	}

	return "", ErrAccountNotFound
}

func (client *apiClient) submitOrder(ctx context.Context, order tsOrderRequest) (string, error) {
	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		Post("/v3/orderexecution/orders")
	if reqErr != nil {
		return "", fmt.Errorf("submit order: %w", reqErr)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return extractOrderID(resp.Body()), nil
}

func (client *apiClient) cancelOrder(ctx context.Context, orderID string) error {
	endpoint := fmt.Sprintf("/v3/orderexecution/orders/%s", orderID)

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		Delete(endpoint)
	if reqErr != nil {
		return fmt.Errorf("cancel order: %w", reqErr)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

func (client *apiClient) replaceOrder(ctx context.Context, orderID string, order tsOrderRequest) error {
	endpoint := fmt.Sprintf("/v3/orderexecution/orders/%s", orderID)

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		Put(endpoint)
	if reqErr != nil {
		return fmt.Errorf("replace order: %w", reqErr)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

func (client *apiClient) getOrders(ctx context.Context) ([]tsOrderResponse, error) {
	endpoint := fmt.Sprintf("/v3/brokerage/accounts/%s/orders", client.accountID)

	var result struct {
		Orders []tsOrderResponse `json:"Orders"`
	}

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get(endpoint)
	if reqErr != nil {
		return nil, fmt.Errorf("get orders: %w", reqErr)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Orders, nil
}

func (client *apiClient) getPositions(ctx context.Context) ([]tsPositionEntry, error) {
	endpoint := fmt.Sprintf("/v3/brokerage/accounts/%s/positions", client.accountID)

	var positions []tsPositionEntry

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&positions).
		Get(endpoint)
	if reqErr != nil {
		return nil, fmt.Errorf("get positions: %w", reqErr)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return positions, nil
}

func (client *apiClient) getBalance(ctx context.Context) (tsBalanceResponse, error) {
	endpoint := fmt.Sprintf("/v3/brokerage/accounts/%s/balances", client.accountID)

	var balances []tsBalanceResponse

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&balances).
		Get(endpoint)
	if reqErr != nil {
		return tsBalanceResponse{}, fmt.Errorf("get balance: %w", reqErr)
	}

	if resp.IsError() {
		return tsBalanceResponse{}, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	if len(balances) == 0 {
		return tsBalanceResponse{}, fmt.Errorf("get balance: no balance data returned")
	}

	return balances[0], nil
}

func (client *apiClient) getQuote(ctx context.Context, symbol string) (float64, error) {
	endpoint := fmt.Sprintf("/v3/marketdata/quotes/%s", symbol)

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		Get(endpoint)
	if reqErr != nil {
		return 0, fmt.Errorf("get quote: %w", reqErr)
	}

	if resp.IsError() {
		return 0, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var quoteResp tsQuoteResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &quoteResp); unmarshalErr != nil {
		return 0, fmt.Errorf("parse quote: %w", unmarshalErr)
	}

	if len(quoteResp.Quotes) == 0 {
		return 0, fmt.Errorf("get quote: no data for symbol %s", symbol)
	}

	return quoteResp.Quotes[0].Last, nil
}

func (client *apiClient) submitGroupOrder(ctx context.Context, group tsGroupOrderRequest) error {
	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetBody(group).
		Post("/v3/orderexecution/ordergroups")
	if reqErr != nil {
		return fmt.Errorf("submit group order: %w", reqErr)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// extractOrderID parses the first OrderID from a TradeStation order response body.
func extractOrderID(body []byte) string {
	var result struct {
		Orders []struct {
			OrderID string `json:"OrderID"`
		} `json:"Orders"`
	}

	if unmarshalErr := sonic.Unmarshal(body, &result); unmarshalErr != nil || len(result.Orders) == 0 {
		return ""
	}

	return result.Orders[0].OrderID
}
