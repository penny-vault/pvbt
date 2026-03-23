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

// --- Broker test exports ---

// SetClientForTest replaces the broker's internal client with one provided by the test.
func SetClientForTest(tb *TradierBroker, client *apiClient) {
	tb.client = client
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

// --- Streamer test exports ---

// NewAccountStreamerForTest creates an accountStreamer for testing.
func NewAccountStreamerForTest(client *apiClient, fills chan broker.Fill, wsEndpoint string, sessionID string, sandbox bool) *accountStreamer {
	return newAccountStreamer(client, fills, wsEndpoint, sessionID, sandbox)
}

// ConnectStreamer exposes connect for testing.
func (streamer *accountStreamer) ConnectStreamer(ctx context.Context) error {
	return streamer.connect(ctx)
}

// CloseStreamer exposes close for testing.
func (streamer *accountStreamer) CloseStreamer() error {
	return streamer.close()
}

// StartPollingForTest exposes startPolling for testing.
func (streamer *accountStreamer) StartPollingForTest(ctx context.Context) {
	streamer.startPolling(ctx)
}

// --- Auth test exports ---

// AuthModeStatic and AuthModeOAuth expose the authMode constants for testing.
const (
	AuthModeStatic = authModeStatic
	AuthModeOAuth  = authModeOAuth
)

// TokenStore is an exported alias for tokenStore, used in auth tests.
type TokenStore = tokenStore

// DetectAuthMode exposes detectAuthMode for testing.
func DetectAuthMode() (authMode, error) {
	return detectAuthMode()
}

// SaveTokens exposes saveTokens for testing.
func SaveTokens(path string, store *tokenStore) error {
	return saveTokens(path, store)
}

// LoadTokens exposes loadTokens for testing.
func LoadTokens(path string) (*tokenStore, error) {
	return loadTokens(path)
}

// NewTokenManagerForTest constructs a tokenManager for testing.
func NewTokenManagerForTest(mode authMode, clientID, clientSecret, callbackURL, tokenFile string) *tokenManager {
	return newTokenManager(mode, clientID, clientSecret, callbackURL, tokenFile)
}

// SetAuthBaseURL sets the authBaseURL on a tokenManager for testing.
func SetAuthBaseURL(tm *tokenManager, baseURL string) {
	tm.authBaseURL = baseURL
}

// SetListenerAddrCh sets the listenerAddrCh on a tokenManager for testing.
func SetListenerAddrCh(tm *tokenManager, ch chan string) {
	tm.listenerAddrCh = ch
}

// SetTokensForTest replaces the tokenStore on a tokenManager for testing.
func (tm *tokenManager) SetTokensForTest(store *tokenStore) {
	tm.tokens = store
}

// AccessToken exposes the accessToken method for testing.
func (tm *tokenManager) AccessToken() string {
	return tm.accessToken()
}

// EnsureValidToken exposes ensureValidToken for testing.
func (tm *tokenManager) EnsureValidToken() error {
	return tm.ensureValidToken()
}

// ExchangeAuthCode exposes exchangeAuthCode for testing.
func (tm *tokenManager) ExchangeAuthCode(_ string, code string) error {
	return tm.exchangeAuthCode(context.Background(), code)
}

// RefreshAccessToken exposes refreshAccessToken for testing.
func (tm *tokenManager) RefreshAccessToken() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	return tm.refreshAccessToken()
}

// StartAuthFlow exposes startAuthFlow for testing.
func (tm *tokenManager) StartAuthFlow() error {
	return tm.startAuthFlow()
}
