package tradier

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
)

const (
	productionBaseURL = "https://api.tradier.com/v1"
	sandboxBaseURL    = "https://sandbox.tradier.com/v1"
)

type apiClient struct {
	resty       *resty.Client
	accountID   string
	accessToken string
}

func newAPIClient(baseURL, accessToken, accountID string) *apiClient {
	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetRetryCount(3).
		SetRetryWaitTime(1*time.Second).
		SetRetryMaxWaitTime(4*time.Second).
		SetAuthToken(accessToken).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/x-www-form-urlencoded")

	httpClient.AddRetryCondition(func(resp *resty.Response, retryErr error) bool {
		if retryErr != nil {
			return broker.IsTransient(retryErr)
		}

		return resp.StatusCode() >= 500 || resp.StatusCode() == 429
	})

	return &apiClient{
		resty:       httpClient,
		accessToken: accessToken,
		accountID:   accountID,
	}
}

// setToken updates the authentication token used by the client.
func (client *apiClient) setToken(token string) {
	client.accessToken = token
	client.resty.SetAuthToken(token)
}

func (client *apiClient) submitOrder(ctx context.Context, params url.Values) (string, error) {
	var wrapper tradierOrderSubmitResponse

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetFormDataFromValues(params).
		SetResult(&wrapper).
		Post(fmt.Sprintf("/accounts/%s/orders", client.accountID))
	if reqErr != nil {
		return "", fmt.Errorf("tradier: submit order: %w", reqErr)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return fmt.Sprintf("%d", wrapper.Order.ID), nil
}

func (client *apiClient) cancelOrder(ctx context.Context, orderID string) error {
	resp, reqErr := client.resty.R().
		SetContext(ctx).
		Delete(fmt.Sprintf("/accounts/%s/orders/%s", client.accountID, orderID))
	if reqErr != nil {
		return fmt.Errorf("tradier: cancel order: %w", reqErr)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

func (client *apiClient) modifyOrder(ctx context.Context, orderID string, params url.Values) error {
	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetFormDataFromValues(params).
		Put(fmt.Sprintf("/accounts/%s/orders/%s", client.accountID, orderID))
	if reqErr != nil {
		return fmt.Errorf("tradier: modify order: %w", reqErr)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

func (client *apiClient) getOrders(ctx context.Context) ([]tradierOrderResponse, error) {
	resp, reqErr := client.resty.R().
		SetContext(ctx).
		Get(fmt.Sprintf("/accounts/%s/orders", client.accountID))
	if reqErr != nil {
		return nil, fmt.Errorf("tradier: get orders: %w", reqErr)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var wrapper tradierOrdersWrapper
	if unmarshalErr := json.Unmarshal(resp.Body(), &wrapper); unmarshalErr != nil {
		return nil, fmt.Errorf("tradier: decode orders response: %w", unmarshalErr)
	}

	return unmarshalFlexible[tradierOrderResponse](wrapper.Orders.Order)
}

func (client *apiClient) getPositions(ctx context.Context) ([]tradierPositionResponse, error) {
	resp, reqErr := client.resty.R().
		SetContext(ctx).
		Get(fmt.Sprintf("/accounts/%s/positions", client.accountID))
	if reqErr != nil {
		return nil, fmt.Errorf("tradier: get positions: %w", reqErr)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var wrapper tradierPositionsWrapper
	if unmarshalErr := json.Unmarshal(resp.Body(), &wrapper); unmarshalErr != nil {
		return nil, fmt.Errorf("tradier: decode positions response: %w", unmarshalErr)
	}

	return unmarshalFlexible[tradierPositionResponse](wrapper.Positions.Position)
}

func (client *apiClient) getBalance(ctx context.Context) (tradierBalanceResponse, error) {
	var wrapper tradierBalancesWrapper

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&wrapper).
		Get(fmt.Sprintf("/accounts/%s/balances", client.accountID))
	if reqErr != nil {
		return tradierBalanceResponse{}, fmt.Errorf("tradier: get balance: %w", reqErr)
	}

	if resp.IsError() {
		return tradierBalanceResponse{}, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return wrapper.Balances, nil
}

func (client *apiClient) getQuote(ctx context.Context, symbol string) (float64, error) {
	var wrapper tradierQuoteResponse

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetQueryParam("symbols", symbol).
		SetResult(&wrapper).
		Get("/markets/quotes")
	if reqErr != nil {
		return 0, fmt.Errorf("tradier: get quote: %w", reqErr)
	}

	if resp.IsError() {
		return 0, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return wrapper.Quotes.Quote.Last, nil
}

func (client *apiClient) createStreamSession(ctx context.Context) (string, error) {
	var result tradierSessionResponse

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Post("/accounts/events/session")
	if reqErr != nil {
		return "", fmt.Errorf("tradier: create stream session: %w", reqErr)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Stream.SessionID, nil
}
