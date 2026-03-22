package schwab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
)

type apiClient struct {
	resty       *resty.Client
	accountHash string
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
			return broker.IsTransient(retryErr)
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

func (client *apiClient) resolveAccount(ctx context.Context, desiredAccountNumber string) (string, error) {
	var accounts []schwabAccountNumberEntry

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&accounts).
		Get("/trader/v1/accounts/accountNumbers")
	if reqErr != nil {
		return "", fmt.Errorf("resolve account: %w", reqErr)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	if len(accounts) == 0 {
		return "", ErrAccountNotFound
	}

	if desiredAccountNumber == "" {
		return accounts[0].HashValue, nil
	}

	for _, account := range accounts {
		if account.AccountNumber == desiredAccountNumber {
			return account.HashValue, nil
		}
	}

	return "", ErrAccountNotFound
}

func (client *apiClient) getUserPreference(ctx context.Context) (schwabUserPreference, error) {
	var pref schwabUserPreference

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&pref).
		Get("/trader/v1/userPreference")
	if reqErr != nil {
		return schwabUserPreference{}, fmt.Errorf("get user preference: %w", reqErr)
	}

	if resp.IsError() {
		return schwabUserPreference{}, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return pref, nil
}

func (client *apiClient) submitOrder(ctx context.Context, order schwabOrderRequest) (string, error) {
	endpoint := fmt.Sprintf("/trader/v1/accounts/%s/orders", client.accountHash)

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		Post(endpoint)
	if reqErr != nil {
		return "", fmt.Errorf("submit order: %w", reqErr)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return extractOrderIDFromLocation(resp.Header().Get("Location")), nil
}

func (client *apiClient) cancelOrder(ctx context.Context, orderID string) error {
	endpoint := fmt.Sprintf("/trader/v1/accounts/%s/orders/%s", client.accountHash, orderID)

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

func (client *apiClient) replaceOrder(ctx context.Context, orderID string, order schwabOrderRequest) (string, error) {
	endpoint := fmt.Sprintf("/trader/v1/accounts/%s/orders/%s", client.accountHash, orderID)

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		Put(endpoint)
	if reqErr != nil {
		return "", fmt.Errorf("replace order: %w", reqErr)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return extractOrderIDFromLocation(resp.Header().Get("Location")), nil
}

func (client *apiClient) getOrders(ctx context.Context) ([]schwabOrderResponse, error) {
	endpoint := fmt.Sprintf("/trader/v1/accounts/%s/orders", client.accountHash)

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var orders []schwabOrderResponse

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"fromEnteredTime": startOfDay.Format(time.RFC3339),
			"toEnteredTime":   now.Format(time.RFC3339),
		}).
		SetResult(&orders).
		Get(endpoint)
	if reqErr != nil {
		return nil, fmt.Errorf("get orders: %w", reqErr)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return orders, nil
}

func (client *apiClient) getPositions(ctx context.Context) ([]schwabPositionEntry, error) {
	endpoint := fmt.Sprintf("/trader/v1/accounts/%s", client.accountHash)

	var account schwabAccountResponse

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetQueryParam("fields", "positions").
		SetResult(&account).
		Get(endpoint)
	if reqErr != nil {
		return nil, fmt.Errorf("get positions: %w", reqErr)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return account.SecuritiesAccount.Positions, nil
}

func (client *apiClient) getBalance(ctx context.Context) (schwabAccountResponse, error) {
	endpoint := fmt.Sprintf("/trader/v1/accounts/%s", client.accountHash)

	var account schwabAccountResponse

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&account).
		Get(endpoint)
	if reqErr != nil {
		return schwabAccountResponse{}, fmt.Errorf("get balance: %w", reqErr)
	}

	if resp.IsError() {
		return schwabAccountResponse{}, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return account, nil
}

func (client *apiClient) getQuote(ctx context.Context, symbol string) (float64, error) {
	endpoint := fmt.Sprintf("/marketdata/v1/%s/quotes?fields=quote", url.PathEscape(symbol))

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		Get(endpoint)
	if reqErr != nil {
		return 0, fmt.Errorf("get quote: %w", reqErr)
	}

	if resp.IsError() {
		return 0, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	// Schwab returns quotes keyed by symbol: {"AAPL": {"quote": {"lastPrice": 150.0}}}
	var quoteMap map[string]schwabQuoteResponse
	if unmarshalErr := json.Unmarshal(resp.Body(), &quoteMap); unmarshalErr != nil {
		return 0, fmt.Errorf("parse quote: %w", unmarshalErr)
	}

	quoteData, exists := quoteMap[symbol]
	if !exists {
		return 0, fmt.Errorf("get quote: no data for symbol %s", symbol)
	}

	return quoteData.Quote.LastPrice, nil
}

// extractOrderIDFromLocation parses an order ID from a Schwab Location header.
// Example: "/v1/accounts/HASH/orders/12345" -> "12345"
func extractOrderIDFromLocation(location string) string {
	if location == "" {
		return ""
	}

	parts := strings.Split(location, "/")
	if len(parts) == 0 {
		return ""
	}

	return parts[len(parts)-1]
}
