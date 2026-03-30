package etrade

import (
	"context"
	"fmt"
	"github.com/bytedance/sonic"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

// apiClient is a resty-based HTTP client for the E*TRADE API. It signs every
// request with OAuth 1.0a credentials via an OnBeforeRequest middleware.
type apiClient struct {
	resty        *resty.Client
	creds        *oauthCredentials
	accountIDKey string
}

// newAPIClient constructs an apiClient targeting baseURL with OAuth signing.
func newAPIClient(baseURL string, creds *oauthCredentials, accountIDKey string) *apiClient {
	cl := &apiClient{
		creds:        creds,
		accountIDKey: accountIDKey,
	}

	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(4 * time.Second)

	httpClient.AddRetryCondition(func(resp *resty.Response, retryErr error) bool {
		if retryErr != nil {
			return broker.IsRetryableError(retryErr)
		}

		return resp.StatusCode() >= 500 || resp.StatusCode() == 429
	})

	httpClient.OnBeforeRequest(func(rc *resty.Client, req *resty.Request) error {
		rawURL := rc.HostURL + req.URL

		authHdr := buildAuthHeader(
			req.Method, rawURL,
			cl.creds.ConsumerKey, cl.creds.ConsumerSecret,
			cl.creds.AccessToken, cl.creds.AccessSecret,
			generateNonce(), generateTimestamp(),
			nil,
		)

		req.SetHeader("Authorization", authHdr)

		return nil
	})

	cl.resty = httpClient

	return cl
}

// setCreds updates the stored credentials pointer used for OAuth signing.
func (cl *apiClient) setCreds(creds *oauthCredentials) {
	cl.creds = creds
}

// getBalance fetches the account balance from E*TRADE.
func (cl *apiClient) getBalance(ctx context.Context) (etradeBalanceResponse, error) {
	path := fmt.Sprintf("/v1/accounts/%s/balance.json", cl.accountIDKey)

	resp, reqErr := cl.resty.R().
		SetContext(ctx).
		SetQueryParam("instType", "BROKERAGE").
		SetQueryParam("realTimeNAV", "true").
		Get(path)
	if reqErr != nil {
		return etradeBalanceResponse{}, fmt.Errorf("etrade: get balance: %w", reqErr)
	}

	if resp.IsError() {
		log.Error().Int("status", resp.StatusCode()).Str("body", resp.String()).Msg("etrade: get balance: non-2xx response")

		return etradeBalanceResponse{}, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var result etradeBalanceResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &result); unmarshalErr != nil {
		return etradeBalanceResponse{}, fmt.Errorf("etrade: get balance: decode response: %w", unmarshalErr)
	}

	return result, nil
}

// getPositions fetches all positions in the account from E*TRADE.
func (cl *apiClient) getPositions(ctx context.Context) ([]etradePosition, error) {
	path := fmt.Sprintf("/v1/accounts/%s/portfolio.json", cl.accountIDKey)

	resp, reqErr := cl.resty.R().
		SetContext(ctx).
		Get(path)
	if reqErr != nil {
		return nil, fmt.Errorf("etrade: get positions: %w", reqErr)
	}

	if resp.IsError() {
		log.Error().Int("status", resp.StatusCode()).Str("body", resp.String()).Msg("etrade: get positions: non-2xx response")

		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var result etradePortfolioResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &result); unmarshalErr != nil {
		return nil, fmt.Errorf("etrade: get positions: decode response: %w", unmarshalErr)
	}

	positions := make([]etradePosition, 0)
	for _, portfolio := range result.PortfolioResponse.AccountPortfolio {
		positions = append(positions, portfolio.Position...)
	}

	return positions, nil
}

// getOrders fetches all orders for the current trading day from E*TRADE.
func (cl *apiClient) getOrders(ctx context.Context) ([]etradeOrderDetail, error) {
	path := fmt.Sprintf("/v1/accounts/%s/orders.json", cl.accountIDKey)

	resp, reqErr := cl.resty.R().
		SetContext(ctx).
		Get(path)
	if reqErr != nil {
		return nil, fmt.Errorf("etrade: get orders: %w", reqErr)
	}

	if resp.IsError() {
		log.Error().Int("status", resp.StatusCode()).Str("body", resp.String()).Msg("etrade: get orders: non-2xx response")

		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var result etradeOrdersResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &result); unmarshalErr != nil {
		return nil, fmt.Errorf("etrade: get orders: decode response: %w", unmarshalErr)
	}

	orders := result.OrdersResponse.Order
	if orders == nil {
		return []etradeOrderDetail{}, nil
	}

	return orders, nil
}

// getQuote returns the last trade price for the given symbol from E*TRADE.
func (cl *apiClient) getQuote(ctx context.Context, symbol string) (float64, error) {
	path := fmt.Sprintf("/v1/market/quote/%s.json", symbol)

	resp, reqErr := cl.resty.R().
		SetContext(ctx).
		Get(path)
	if reqErr != nil {
		return 0, fmt.Errorf("etrade: get quote %s: %w", symbol, reqErr)
	}

	if resp.IsError() {
		log.Error().Int("status", resp.StatusCode()).Str("symbol", symbol).Str("body", resp.String()).Msg("etrade: get quote: non-2xx response")

		return 0, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var result etradeQuoteResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &result); unmarshalErr != nil {
		return 0, fmt.Errorf("etrade: get quote %s: decode response: %w", symbol, unmarshalErr)
	}

	return quoteFromResponse(result), nil
}

// getTransactions returns account transactions since the given time.
func (cl *apiClient) getTransactions(ctx context.Context, since time.Time) ([]etradeTransaction, error) {
	path := fmt.Sprintf("/v1/accounts/%s/transactions.json", cl.accountIDKey)

	resp, reqErr := cl.resty.R().
		SetContext(ctx).
		SetQueryParam("startDate", formatDate(since)).
		Get(path)
	if reqErr != nil {
		return nil, fmt.Errorf("etrade: get transactions: %w", reqErr)
	}

	if resp.IsError() {
		log.Error().Int("status", resp.StatusCode()).Str("body", resp.String()).Msg("etrade: get transactions: non-2xx response")

		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var result etradeTransactionsResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &result); unmarshalErr != nil {
		return nil, fmt.Errorf("etrade: get transactions: decode response: %w", unmarshalErr)
	}

	txns := result.TransactionListResponse.Transaction
	if txns == nil {
		return []etradeTransaction{}, nil
	}

	return txns, nil
}

// previewOrder sends a preview order request and returns the preview ID.
func (cl *apiClient) previewOrder(ctx context.Context, req etradePreviewRequest) (int64, error) {
	path := fmt.Sprintf("/v1/accounts/%s/orders/preview.json", cl.accountIDKey)

	body := map[string]any{
		"PreviewOrderRequest": req,
	}

	resp, reqErr := cl.resty.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(path)
	if reqErr != nil {
		return 0, fmt.Errorf("etrade: preview order: %w", reqErr)
	}

	if resp.IsError() {
		log.Error().Int("status", resp.StatusCode()).Str("body", resp.String()).Msg("etrade: preview order: non-2xx response")

		return 0, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var result etradePreviewResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &result); unmarshalErr != nil {
		return 0, fmt.Errorf("etrade: preview order: decode response: %w", unmarshalErr)
	}

	previewID := previewIDFromResponse(result)

	return previewID, nil
}

// placeOrder places a previously previewed order and returns the order ID.
func (cl *apiClient) placeOrder(ctx context.Context, req etradePreviewRequest, previewID int64) (int64, error) {
	path := fmt.Sprintf("/v1/accounts/%s/orders/place.json", cl.accountIDKey)

	body := map[string]any{
		"PlaceOrderRequest": map[string]any{
			"orderType":     req.OrderType,
			"clientOrderId": req.ClientOrderID,
			"Order":         req.Order,
			"PreviewIds": []map[string]any{
				{"previewId": previewID},
			},
		},
	}

	resp, reqErr := cl.resty.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(path)
	if reqErr != nil {
		return 0, fmt.Errorf("etrade: place order: %w", reqErr)
	}

	if resp.IsError() {
		log.Error().Int("status", resp.StatusCode()).Str("body", resp.String()).Msg("etrade: place order: non-2xx response")

		return 0, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var result etradePlaceResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &result); unmarshalErr != nil {
		return 0, fmt.Errorf("etrade: place order: decode response: %w", unmarshalErr)
	}

	return orderIDFromPlaceResponse(result), nil
}

// cancelOrder requests cancellation of the order identified by orderID.
func (cl *apiClient) cancelOrder(ctx context.Context, orderID string) error {
	path := fmt.Sprintf("/v1/accounts/%s/orders/cancel.json", cl.accountIDKey)

	numericID, parseErr := strconv.ParseInt(orderID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("etrade: cancel order: parse order ID %q: %w", orderID, parseErr)
	}

	body := map[string]any{
		"CancelOrderRequest": map[string]any{
			"orderId": numericID,
		},
	}

	resp, reqErr := cl.resty.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Put(path)
	if reqErr != nil {
		return fmt.Errorf("etrade: cancel order: %w", reqErr)
	}

	if resp.IsError() {
		log.Error().Int("status", resp.StatusCode()).Str("body", resp.String()).Msg("etrade: cancel order: non-2xx response")

		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// previewModifyOrder sends a preview request for modifying an existing order.
func (cl *apiClient) previewModifyOrder(ctx context.Context, orderID string, req etradePreviewRequest) (int64, error) {
	path := fmt.Sprintf("/v1/accounts/%s/orders/%s/change/preview.json", cl.accountIDKey, orderID)

	body := map[string]any{
		"PreviewOrderRequest": req,
	}

	resp, reqErr := cl.resty.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Put(path)
	if reqErr != nil {
		return 0, fmt.Errorf("etrade: preview modify order %s: %w", orderID, reqErr)
	}

	if resp.IsError() {
		log.Error().Int("status", resp.StatusCode()).Str("orderId", orderID).Str("body", resp.String()).Msg("etrade: preview modify order: non-2xx response")

		return 0, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var result etradePreviewResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &result); unmarshalErr != nil {
		return 0, fmt.Errorf("etrade: preview modify order %s: decode response: %w", orderID, unmarshalErr)
	}

	return previewIDFromResponse(result), nil
}

// placeModifyOrder places a previously previewed order modification.
func (cl *apiClient) placeModifyOrder(ctx context.Context, orderID string, req etradePreviewRequest, previewID int64) (int64, error) {
	path := fmt.Sprintf("/v1/accounts/%s/orders/%s/change/place.json", cl.accountIDKey, orderID)

	body := map[string]any{
		"PlaceOrderRequest": map[string]any{
			"orderType":     req.OrderType,
			"clientOrderId": req.ClientOrderID,
			"Order":         req.Order,
			"PreviewIds": []map[string]any{
				{"previewId": previewID},
			},
		},
	}

	resp, reqErr := cl.resty.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Put(path)
	if reqErr != nil {
		return 0, fmt.Errorf("etrade: place modify order %s: %w", orderID, reqErr)
	}

	if resp.IsError() {
		log.Error().Int("status", resp.StatusCode()).Str("orderId", orderID).Str("body", resp.String()).Msg("etrade: place modify order: non-2xx response")

		return 0, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var result etradePlaceResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &result); unmarshalErr != nil {
		return 0, fmt.Errorf("etrade: place modify order %s: decode response: %w", orderID, unmarshalErr)
	}

	return orderIDFromPlaceResponse(result), nil
}
