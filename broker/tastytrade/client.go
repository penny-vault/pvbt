package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

type apiClient struct {
	resty     *resty.Client
	accountID string
	username  string
	password  string
	mu        sync.Mutex // protects re-authentication
}

// newAPIClient creates a new apiClient configured with retry and default headers.
func newAPIClient(baseURL string) *apiClient {
	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetRetryCount(3).
		SetRetryWaitTime(1*time.Second).
		SetRetryMaxWaitTime(4*time.Second).
		SetHeader("Content-Type", "application/json")

	httpClient.AddRetryCondition(func(resp *resty.Response, err error) bool {
		if err != nil {
			return IsTransient(err)
		}

		return resp.StatusCode() >= 500
	})

	return &apiClient{
		resty: httpClient,
	}
}

// authenticate logs in and stores the session token and account ID.
func (client *apiClient) authenticate(ctx context.Context, username, password string) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	client.username = username
	client.password = password

	// POST /sessions to get a session token.
	var session sessionResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(sessionRequest{Login: username, Password: password}).
		SetResult(&session).
		Post("/sessions")
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	if resp.IsError() {
		return NewHTTPError(resp.StatusCode(), resp.String())
	}

	token := session.Data.SessionToken
	client.resty.SetAuthToken(token)

	// GET /customers/me/accounts to retrieve the account ID.
	var accounts accountsResponse

	resp, err = client.resty.R().
		SetContext(ctx).
		SetResult(&accounts).
		Get("/customers/me/accounts")
	if err != nil {
		return fmt.Errorf("get accounts: %w", err)
	}

	if resp.IsError() {
		return NewHTTPError(resp.StatusCode(), resp.String())
	}

	if len(accounts.Data.Items) == 0 {
		return ErrAccountNotFound
	}

	client.accountID = accounts.Data.Items[0].Account.AccountNumber

	return nil
}

// submitOrder sends an order and returns the order ID.
func (client *apiClient) submitOrder(ctx context.Context, order orderRequest) (string, error) {
	endpoint := fmt.Sprintf("/accounts/%s/orders", client.accountID)

	var result orderSubmitResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		SetResult(&result).
		Post(endpoint)
	if err != nil {
		return "", fmt.Errorf("submit order: %w", err)
	}

	if resp.IsError() {
		return "", NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Data.Order.ID, nil
}

// cancelOrder deletes an existing order.
func (client *apiClient) cancelOrder(ctx context.Context, orderID string) error {
	endpoint := fmt.Sprintf("/accounts/%s/orders/%s", client.accountID, orderID)

	resp, err := client.resty.R().
		SetContext(ctx).
		Delete(endpoint)
	if err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}

	if resp.IsError() {
		return NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// replaceOrder updates an existing order with new parameters.
func (client *apiClient) replaceOrder(ctx context.Context, orderID string, order orderRequest) error {
	endpoint := fmt.Sprintf("/accounts/%s/orders/%s", client.accountID, orderID)

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		Put(endpoint)
	if err != nil {
		return fmt.Errorf("replace order: %w", err)
	}

	if resp.IsError() {
		return NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// getOrders retrieves all orders for the account. The account-level
// orders endpoint does not support offset pagination; it returns all
// matching results in a single response.
func (client *apiClient) getOrders(ctx context.Context) ([]orderResponse, error) {
	endpoint := fmt.Sprintf("/accounts/%s/orders", client.accountID)

	var result ordersListResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("get orders: %w", err)
	}

	if resp.IsError() {
		return nil, NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Data.Items, nil
}

// submitComplexOrder sends an OCO or OTOCO complex order.
func (client *apiClient) submitComplexOrder(ctx context.Context, order complexOrderRequest) (complexOrderSubmitResponse, error) {
	endpoint := fmt.Sprintf("/accounts/%s/complex-orders", client.accountID)

	var result complexOrderSubmitResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		SetResult(&result).
		Post(endpoint)
	if err != nil {
		return complexOrderSubmitResponse{}, fmt.Errorf("submit complex order: %w", err)
	}

	if resp.IsError() {
		return complexOrderSubmitResponse{}, NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result, nil
}

// cancelComplexOrder deletes an existing complex order by its complex-order ID.
func (client *apiClient) cancelComplexOrder(ctx context.Context, complexOrderID string) error {
	endpoint := fmt.Sprintf("/accounts/%s/complex-orders/%s", client.accountID, complexOrderID)

	resp, err := client.resty.R().
		SetContext(ctx).
		Delete(endpoint)
	if err != nil {
		return fmt.Errorf("cancel complex order: %w", err)
	}

	if resp.IsError() {
		return NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// getPositions retrieves all positions for the account.
func (client *apiClient) getPositions(ctx context.Context) ([]positionResponse, error) {
	endpoint := fmt.Sprintf("/accounts/%s/positions", client.accountID)

	var result positionsListResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("get positions: %w", err)
	}

	if resp.IsError() {
		return nil, NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Data.Items, nil
}

// getBalance retrieves account balances, unwrapping the API envelope.
func (client *apiClient) getBalance(ctx context.Context) (balanceResponse, error) {
	endpoint := fmt.Sprintf("/accounts/%s/balances", client.accountID)

	resp, err := client.resty.R().
		SetContext(ctx).
		Get(endpoint)
	if err != nil {
		return balanceResponse{}, fmt.Errorf("get balance: %w", err)
	}

	if resp.IsError() {
		return balanceResponse{}, NewHTTPError(resp.StatusCode(), resp.String())
	}

	// The API wraps balance in {"data": {...}}. Unwrap it.
	var envelope struct {
		Data balanceResponse `json:"data"`
	}

	if unmarshalErr := json.Unmarshal(resp.Body(), &envelope); unmarshalErr != nil {
		return balanceResponse{}, fmt.Errorf("parse balance: %w", unmarshalErr)
	}

	return envelope.Data, nil
}

// sessionToken returns the current session token.
func (client *apiClient) sessionToken() string {
	return client.resty.Token
}

// account returns the account ID.
func (client *apiClient) account() string {
	return client.accountID
}

// getQuote retrieves the last price for a symbol.
func (client *apiClient) getQuote(ctx context.Context, symbol string) (float64, error) {
	endpoint := "/market-data/by-type?equity=" + url.QueryEscape(symbol)

	var result quoteResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get(endpoint)
	if err != nil {
		return 0, fmt.Errorf("get quote: %w", err)
	}

	if resp.IsError() {
		return 0, NewHTTPError(resp.StatusCode(), resp.String())
	}

	if len(result.Data.Items) == 0 {
		return 0, fmt.Errorf("get quote: no data for symbol %s", symbol)
	}

	return result.Data.Items[0].LastPrice, nil
}
