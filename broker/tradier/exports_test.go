package tradier

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/penny-vault/pvbt/broker"
)

// Type aliases for test access.
type TradierOrderResponse = tradierOrderResponse
type TradierPositionResponse = tradierPositionResponse
type TradierBalanceResponse = tradierBalanceResponse
type TradierMarginBalance = tradierMarginBalance
type TradierCashBalance = tradierCashBalance

// UnmarshalFlexible exposes unmarshalFlexible for testing.
func UnmarshalFlexible[T any](raw json.RawMessage) ([]T, error) {
	return unmarshalFlexible[T](raw)
}

// ToTradierOrderParams exposes toTradierOrderParams for testing.
func ToTradierOrderParams(order broker.Order) (url.Values, error) {
	return toTradierOrderParams(order)
}

// ToBrokerOrder exposes toBrokerOrder for testing.
func ToBrokerOrder(resp tradierOrderResponse) broker.Order {
	return toBrokerOrder(resp)
}

// ToBrokerPosition exposes toBrokerPosition for testing.
func ToBrokerPosition(resp tradierPositionResponse) broker.Position {
	return toBrokerPosition(resp)
}

// ToBrokerBalance exposes toBrokerBalance for testing.
func ToBrokerBalance(resp tradierBalanceResponse) broker.Balance {
	return toBrokerBalance(resp)
}

// MapTradierSide exposes mapTradierSide for testing.
func MapTradierSide(side string) broker.Side {
	return mapTradierSide(side)
}

// --- Client test exports ---

// APIClientForTest is a type alias giving tests access to apiClient.
type APIClientForTest = apiClient

// NewAPIClientForTest constructs an apiClient pointing at the given test server.
func NewAPIClientForTest(baseURL, token, accountID string) *apiClient {
	return newAPIClient(baseURL, token, accountID)
}

// SubmitOrder exposes submitOrder for testing.
func (client *apiClient) SubmitOrder(ctx context.Context, params url.Values) (string, error) {
	return client.submitOrder(ctx, params)
}

// CancelOrder exposes cancelOrder for testing.
func (client *apiClient) CancelOrder(ctx context.Context, orderID string) error {
	return client.cancelOrder(ctx, orderID)
}

// ModifyOrder exposes modifyOrder for testing.
func (client *apiClient) ModifyOrder(ctx context.Context, orderID string, params url.Values) error {
	return client.modifyOrder(ctx, orderID, params)
}

// GetOrders exposes getOrders for testing.
func (client *apiClient) GetOrders(ctx context.Context) ([]tradierOrderResponse, error) {
	return client.getOrders(ctx)
}

// GetPositions exposes getPositions for testing.
func (client *apiClient) GetPositions(ctx context.Context) ([]tradierPositionResponse, error) {
	return client.getPositions(ctx)
}

// GetBalance exposes getBalance for testing.
func (client *apiClient) GetBalance(ctx context.Context) (tradierBalanceResponse, error) {
	return client.getBalance(ctx)
}

// GetQuote exposes getQuote for testing.
func (client *apiClient) GetQuote(ctx context.Context, symbol string) (float64, error) {
	return client.getQuote(ctx, symbol)
}

// CreateStreamSession exposes createStreamSession for testing.
func (client *apiClient) CreateStreamSession(ctx context.Context) (string, error) {
	return client.createStreamSession(ctx)
}
