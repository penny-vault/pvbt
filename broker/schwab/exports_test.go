package schwab

import (
	"context"

	"github.com/penny-vault/pvbt/broker"
)

// Type aliases for test access.
type SchwabOrderRequest = schwabOrderRequest
type SchwabOrderLegEntry = schwabOrderLegEntry
type SchwabInstrument = schwabInstrument
type SchwabOrderResponse = schwabOrderResponse
type SchwabAccountResponse = schwabAccountResponse
type SchwabUserPreference = schwabUserPreference
type SchwabStreamerInfo = schwabStreamerInfo
type HTTPError = broker.HTTPError

// NewAPIClientForTest creates an apiClient for testing.
func NewAPIClientForTest(baseURL, token string) *apiClient {
	return newAPIClient(baseURL, token)
}

// SetAccountHash sets the account hash for testing.
func (client *apiClient) SetAccountHash(hash string) {
	client.accountHash = hash
}

func (client *apiClient) ResolveAccount(ctx context.Context, desiredAccount string) (string, error) {
	return client.resolveAccount(ctx, desiredAccount)
}

func (client *apiClient) SubmitOrder(ctx context.Context, order schwabOrderRequest) (string, error) {
	return client.submitOrder(ctx, order)
}

func (client *apiClient) CancelOrder(ctx context.Context, orderID string) error {
	return client.cancelOrder(ctx, orderID)
}

func (client *apiClient) ReplaceOrder(ctx context.Context, orderID string, order schwabOrderRequest) (string, error) {
	return client.replaceOrder(ctx, orderID, order)
}

func (client *apiClient) GetOrders(ctx context.Context) ([]schwabOrderResponse, error) {
	return client.getOrders(ctx)
}

func (client *apiClient) GetPositions(ctx context.Context) ([]schwabPositionEntry, error) {
	return client.getPositions(ctx)
}

func (client *apiClient) GetBalance(ctx context.Context) (schwabAccountResponse, error) {
	return client.getBalance(ctx)
}

func (client *apiClient) GetQuote(ctx context.Context, symbol string) (float64, error) {
	return client.getQuote(ctx, symbol)
}

func (client *apiClient) GetUserPreference(ctx context.Context) (schwabUserPreference, error) {
	return client.getUserPreference(ctx)
}

// --- Streamer test exports ---

// ActivityStreamerForTestType is an exported alias.
type ActivityStreamerForTestType = activityStreamer

// NewActivityStreamerForTest creates an activityStreamer for testing.
func NewActivityStreamerForTest(client *apiClient, fills chan broker.Fill, wsURL string, info schwabStreamerInfo, accountHash string, accessToken string) *activityStreamer {
	return newActivityStreamer(client, fills, wsURL, info, accountHash, func() string { return accessToken })
}

// ConnectStreamer exposes connect for testing.
func (streamer *activityStreamer) ConnectStreamer(ctx context.Context) error {
	return streamer.connect(ctx)
}

// CloseStreamer exposes close for testing.
func (streamer *activityStreamer) CloseStreamer() error {
	return streamer.close()
}

// --- Broker test exports ---

// SetClientForTest sets the broker's internal client for testing.
func SetClientForTest(schwabBroker *SchwabBroker, client *apiClient) {
	schwabBroker.client = client
}

// SetAccountHashForTest sets the broker's account hash for testing.
func SetAccountHashForTest(schwabBroker *SchwabBroker, hash string) {
	schwabBroker.accountHash = hash
	if schwabBroker.client != nil {
		schwabBroker.client.accountHash = hash
	}
}
