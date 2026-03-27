package tradestation

import (
	"context"

	"github.com/penny-vault/pvbt/broker"
)

// Type aliases for test access.
type TSOrderRequest = tsOrderRequest
type TSTimeInForce = tsTimeInForce
type TSOrderResponse = tsOrderResponse
type TSOrderLeg = tsOrderLeg
type TSAccountEntry = tsAccountEntry
type TSPositionEntry = tsPositionEntry
type TSBalanceResponse = tsBalanceResponse
type TSGroupOrderRequest = tsGroupOrderRequest
type HTTPError = broker.HTTPError

// NewAPIClientForTest creates an apiClient for testing.
func NewAPIClientForTest(baseURL, token string) *apiClient {
	return newAPIClient(baseURL, token)
}

// SetAccountID sets the account ID for testing.
func (client *apiClient) SetAccountID(accountID string) {
	client.accountID = accountID
}

func (client *apiClient) ResolveAccount(ctx context.Context, desiredAccount string) (string, error) {
	return client.resolveAccount(ctx, desiredAccount)
}

func (client *apiClient) SubmitOrder(ctx context.Context, order tsOrderRequest) (string, error) {
	return client.submitOrder(ctx, order)
}

func (client *apiClient) CancelOrder(ctx context.Context, orderID string) error {
	return client.cancelOrder(ctx, orderID)
}

func (client *apiClient) ReplaceOrder(ctx context.Context, orderID string, order tsOrderRequest) error {
	return client.replaceOrder(ctx, orderID, order)
}

func (client *apiClient) GetOrders(ctx context.Context) ([]tsOrderResponse, error) {
	return client.getOrders(ctx)
}

func (client *apiClient) GetPositions(ctx context.Context) ([]tsPositionEntry, error) {
	return client.getPositions(ctx)
}

func (client *apiClient) GetBalance(ctx context.Context) (tsBalanceResponse, error) {
	return client.getBalance(ctx)
}

func (client *apiClient) GetQuote(ctx context.Context, symbol string) (float64, error) {
	return client.getQuote(ctx, symbol)
}

func (client *apiClient) SubmitGroupOrder(ctx context.Context, group tsGroupOrderRequest) error {
	return client.submitGroupOrder(ctx, group)
}

// --- Streamer test exports ---

type OrderStreamerForTestType = orderStreamer

func NewOrderStreamerForTest(client *apiClient, fills chan broker.Fill, baseURL string, accountID string, accessToken string) *orderStreamer {
	return newOrderStreamer(client, fills, baseURL, accountID, func() string { return accessToken })
}

func (streamer *orderStreamer) ConnectStreamer(ctx context.Context) error {
	return streamer.connect(ctx)
}

func (streamer *orderStreamer) CloseStreamer() error {
	return streamer.close()
}

// --- Broker test exports ---

func SetClientForTest(tsBroker *TradeStationBroker, client *apiClient) {
	tsBroker.client = client
}

func SetAccountIDForTest(tsBroker *TradeStationBroker, accountID string) {
	tsBroker.accountID = accountID
	if tsBroker.client != nil {
		tsBroker.client.accountID = accountID
	}
}
