package webull

import (
	"context"
	"net/http"
	"time"

	"github.com/penny-vault/pvbt/broker"
)

// HTTPError is a type alias for broker.HTTPError.
type HTTPError = broker.HTTPError

// SignerForTestType is an exported alias for the signer interface.
type SignerForTestType = signer

// NewHMACSigner creates an hmacSigner for testing.
func NewHMACSigner(appKey, appSecret string) signer {
	return &hmacSigner{appKey: appKey, appSecret: appSecret}
}

// ExtractSignatureHeaders reads the HMAC headers from an http.Request for test assertions.
func ExtractSignatureHeaders(req *http.Request) (appKey, timestamp, signature, algorithm, version, nonce string) {
	appKey = req.Header.Get("x-app-key")
	timestamp = req.Header.Get("x-timestamp")
	signature = req.Header.Get("x-signature")
	algorithm = req.Header.Get("x-signature-algorithm")
	version = req.Header.Get("x-signature-version")
	nonce = req.Header.Get("x-signature-nonce")
	return
}

// DetectAuthModeExport exposes detectAuthMode for testing.
func DetectAuthModeExport() (AuthModeExport, error) {
	mode, err := detectAuthMode()
	return AuthModeExport(mode), err
}

// AuthModeExport is an exported alias for authMode.
type AuthModeExport = authMode

// Auth mode constants for tests.
var (
	AuthModeDirect = authModeDirect
	AuthModeOAuth  = authModeOAuth
)

// NewOAuthSignerForTest creates an oauthSigner with a pre-set access token.
func NewOAuthSignerForTest(accessToken string) signer {
	return &oauthSigner{
		tokenMgr: &tokenManager{
			tokens: &tokenStore{
				AccessToken:     accessToken,
				AccessExpiresAt: time.Now().Add(30 * time.Minute),
			},
		},
	}
}

// NewTokenManagerForTest creates a tokenManager pointing at a custom auth URL.
func NewTokenManagerForTest(clientID, clientSecret, callbackURL, tokenFile, authBaseURL string) *TokenManagerExport {
	return newTokenManager(authModeOAuth, clientID, clientSecret, callbackURL, tokenFile, authBaseURL)
}

// TokenManagerExport is an exported alias for tokenManager.
type TokenManagerExport = tokenManager

// LoadTokensExport exposes loadTokens for testing.
func (tm *TokenManagerExport) LoadTokensExport() error {
	return tm.loadTokens()
}

// SaveTokensExport exposes saveTokens for testing.
func (tm *TokenManagerExport) SaveTokensExport() error {
	return tm.saveTokens()
}

// OrderRequestExport is an exported alias for orderRequest.
type OrderRequestExport = orderRequest

// OrderResponseExport is an exported alias for orderResponse.
type OrderResponseExport = orderResponse

// ReplaceRequestExport is an exported alias for replaceRequest.
type ReplaceRequestExport = replaceRequest

// AccountResponseExport is an exported alias for accountResponse.
type AccountResponseExport = accountResponse

// AccountListResponseExport is an exported alias for accountListResponse.
type AccountListResponseExport = accountListResponse

// PositionResponseExport is an exported alias for positionResponse.
type PositionResponseExport = positionResponse

// ToWebullOrder exposes toWebullOrder for testing.
func ToWebullOrder(order broker.Order, fractional bool) orderRequest {
	return toWebullOrder(order, fractional)
}

// ToBrokerOrder exposes toBrokerOrder for testing.
func ToBrokerOrder(resp orderResponse) broker.Order {
	return toBrokerOrder(resp)
}

// ToBrokerPosition exposes toBrokerPosition for testing.
func ToBrokerPosition(resp positionResponse) broker.Position {
	return toBrokerPosition(resp)
}

// ToBrokerBalance exposes toBrokerBalance for testing.
func ToBrokerBalance(resp accountResponse) broker.Balance {
	return toBrokerBalance(resp)
}

// AccessTokenExport returns the current access token for assertions.
func (tm *TokenManagerExport) AccessTokenExport() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.tokens == nil {
		return ""
	}
	return tm.tokens.AccessToken
}

// SetTokensForTest sets tokens directly for testing.
func (tm *TokenManagerExport) SetTokensForTest(access, refresh string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tokens = &tokenStore{
		AccessToken:     access,
		RefreshToken:    refresh,
		AccessExpiresAt: time.Now().Add(30 * time.Minute),
	}
}

// --- API client test exports ---

// APIClientForTestType is an exported alias for apiClient.
type APIClientForTestType = apiClient

// NewAPIClientForTest creates an apiClient pointing at a custom base URL
// with an hmacSigner for testing.
func NewAPIClientForTest(baseURL, appKey, appSecret string) *apiClient {
	sign := &hmacSigner{appKey: appKey, appSecret: appSecret}
	return newAPIClient(baseURL, sign)
}

// GetAccounts exposes getAccounts for testing.
func (client *apiClient) GetAccounts(ctx context.Context) ([]accountEntry, error) {
	return client.getAccounts(ctx)
}

// SubmitOrder exposes submitOrder for testing.
func (client *apiClient) SubmitOrder(ctx context.Context, accountID string, order orderRequest) (string, error) {
	return client.submitOrder(ctx, accountID, order)
}

// CancelOrder exposes cancelOrder for testing.
func (client *apiClient) CancelOrder(ctx context.Context, accountID string, orderID string) error {
	return client.cancelOrder(ctx, accountID, orderID)
}

// ReplaceOrder exposes replaceOrder for testing.
func (client *apiClient) ReplaceOrder(ctx context.Context, accountID string, orderID string, replacement replaceRequest) error {
	return client.replaceOrder(ctx, accountID, orderID, replacement)
}

// GetOrders exposes getOrders for testing.
func (client *apiClient) GetOrders(ctx context.Context, accountID string) ([]orderResponse, error) {
	return client.getOrders(ctx, accountID)
}

// GetPositions exposes getPositions for testing.
func (client *apiClient) GetPositions(ctx context.Context, accountID string) ([]positionResponse, error) {
	return client.getPositions(ctx, accountID)
}

// GetBalance exposes getBalance for testing.
func (client *apiClient) GetBalance(ctx context.Context, accountID string) (accountResponse, error) {
	return client.getBalance(ctx, accountID)
}

// SetClientForTest replaces the broker's internal client with one
// pointing at the given URL.
func SetClientForTest(wb *WebullBroker, baseURL, appKey, appSecret string) {
	sign := &hmacSigner{appKey: appKey, appSecret: appSecret}
	wb.client = newAPIClient(baseURL, sign)
	wb.accountID = "test-account"
}

// --- Fill streamer test exports ---

// FillStreamerForTestType is an exported alias for fillStreamer.
type FillStreamerForTestType = fillStreamer

// NewFillStreamerForTest creates a fillStreamer for testing with an injected
// poll function instead of a real gRPC connection.
func NewFillStreamerForTest(fills chan broker.Fill, pollFn func(ctx context.Context) ([]orderResponse, error)) *fillStreamer {
	return &fillStreamer{
		fills:       fills,
		done:        make(chan struct{}),
		cumulFilled: make(map[string]float64),
		pollOrders:  pollFn,
	}
}

// HandleTradeEvent exposes handleTradeEvent for testing.
func (fs *fillStreamer) HandleTradeEvent(orderID, status string, filledQty, filledPrice float64) {
	fs.handleTradeEvent(orderID, status, filledQty, filledPrice)
}

// PollMissedFills exposes pollMissedFills for testing.
func (fs *fillStreamer) PollMissedFills(ctx context.Context) {
	fs.pollMissedFills(ctx)
}

// CumulFilledForTest returns the cumulative filled qty for an order ID.
func (fs *fillStreamer) CumulFilledForTest(orderID string) float64 {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.cumulFilled[orderID]
}
