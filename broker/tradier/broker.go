package tradier

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/penny-vault/pvbt/broker"
)

// TradierBroker implements broker.Broker for the Tradier brokerage.
// Full implementation is in progress; this stub anchors the package types.
type TradierBroker struct {
	client      *apiClient
	auth        *tokenManager
	streamer    *accountStreamer
	fills       chan broker.Fill
	mu          sync.Mutex
	sandbox     bool
	tokenFile   string
	callbackURL string
}

// Option configures a TradierBroker.
type Option func(*TradierBroker)

// WithSandbox configures the broker to use the Tradier sandbox environment.
func WithSandbox() Option {
	return func(tb *TradierBroker) {
		tb.sandbox = true
	}
}

// WithTokenFile configures the path to the token persistence file.
func WithTokenFile(path string) Option {
	return func(tb *TradierBroker) {
		tb.tokenFile = path
	}
}

// WithCallbackURL configures the OAuth callback URL.
func WithCallbackURL(callbackURL string) Option {
	return func(tb *TradierBroker) {
		tb.callbackURL = callbackURL
	}
}

// New creates a new TradierBroker.
func New(opts ...Option) *TradierBroker {
	tb := &TradierBroker{
		fills: make(chan broker.Fill, 1024),
	}
	for _, opt := range opts {
		opt(tb)
	}

	return tb
}

// Fills returns the channel on which fill reports are delivered.
func (tb *TradierBroker) Fills() <-chan broker.Fill {
	return tb.fills
}

// Connect establishes an authenticated session with Tradier.
func (tb *TradierBroker) Connect(ctx context.Context) error {
	mode, modeErr := detectAuthMode()
	if modeErr != nil {
		return fmt.Errorf("tradier: connect: %w", modeErr)
	}

	callbackURL := tb.callbackURL
	if callbackURL == "" {
		callbackURL = os.Getenv("TRADIER_CALLBACK_URL")
	}

	tokenFile := tb.tokenFile
	if tokenFile == "" {
		tokenFile = os.Getenv("TRADIER_TOKEN_FILE")
	}

	clientID := os.Getenv("TRADIER_CLIENT_ID")
	clientSecret := os.Getenv("TRADIER_CLIENT_SECRET")

	tb.auth = newTokenManager(mode, clientID, clientSecret, callbackURL, tokenFile)

	// Attempt to load existing OAuth tokens (ignored in static mode).
	if mode == authModeOAuth {
		tokens, loadErr := loadTokens(tb.auth.tokenFile)
		if loadErr == nil {
			tb.auth.tokens = tokens
		}
	}

	// Ensure we have a valid access token.
	if ensureErr := tb.auth.ensureValidToken(); ensureErr != nil {
		if authErr := tb.auth.startAuthFlow(); authErr != nil {
			return fmt.Errorf("tradier: connect: %w", authErr)
		}
	}

	baseURL := productionBaseURL
	if tb.sandbox {
		baseURL = sandboxBaseURL
	}

	accountID := os.Getenv("TRADIER_ACCOUNT_ID")
	tb.client = newAPIClient(baseURL, tb.auth.accessToken(), accountID)

	tb.auth.onRefresh = func(token string) {
		tb.mu.Lock()
		defer tb.mu.Unlock()

		tb.client.setToken(token)
	}

	if mode == authModeOAuth {
		tb.auth.startBackgroundRefresh()
	}

	wsEndpoint := productionWSURL
	if tb.sandbox {
		wsEndpoint = sandboxWSURL
	}

	if tb.sandbox {
		// Sandbox does not support the WebSocket events API; use polling.
		tb.streamer = newAccountStreamer(tb.client, tb.fills, wsEndpoint, "", true)
		tb.streamer.startPolling(ctx)
	} else {
		sessionID, sessionErr := tb.client.createStreamSession(ctx)
		if sessionErr != nil {
			return fmt.Errorf("tradier: connect: create stream session: %w", sessionErr)
		}

		tb.streamer = newAccountStreamer(tb.client, tb.fills, wsEndpoint, sessionID, false)

		if connectErr := tb.streamer.connect(ctx); connectErr != nil {
			return connectErr
		}
	}

	return nil
}

// SetToken updates the authentication token used for all API requests.
// It is called by the authentication layer after a token is obtained or refreshed.
func (tb *TradierBroker) SetToken(token string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.client.setToken(token)
}

// Close tears down the broker session.
func (tb *TradierBroker) Close() error {
	if tb.auth != nil {
		tb.auth.stopBackgroundRefresh()
	}

	if tb.streamer != nil {
		return tb.streamer.close()
	}

	return nil
}

// Submit sends an order to Tradier.
func (tb *TradierBroker) Submit(ctx context.Context, order broker.Order) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	params, paramsErr := toTradierOrderParams(order)
	if paramsErr != nil {
		return paramsErr
	}

	_, submitErr := tb.client.submitOrder(ctx, params)

	return submitErr
}

// Cancel requests cancellation of an open order.
func (tb *TradierBroker) Cancel(ctx context.Context, orderID string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	return tb.client.cancelOrder(ctx, orderID)
}

// Replace cancels and resubmits an order.
func (tb *TradierBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	params, paramsErr := toTradierOrderParams(order)
	if paramsErr != nil {
		return paramsErr
	}

	return tb.client.modifyOrder(ctx, orderID, params)
}

// Orders returns all orders for the current trading day.
func (tb *TradierBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	rawOrders, getErr := tb.client.getOrders(ctx)
	if getErr != nil {
		return nil, getErr
	}

	orders := make([]broker.Order, len(rawOrders))
	for ii, rawOrder := range rawOrders {
		orders[ii] = toBrokerOrder(rawOrder)
	}

	return orders, nil
}

// Positions returns all current positions in the account.
func (tb *TradierBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	rawPositions, getErr := tb.client.getPositions(ctx)
	if getErr != nil {
		return nil, getErr
	}

	positions := make([]broker.Position, len(rawPositions))
	for ii, rawPos := range rawPositions {
		pos := toBrokerPosition(rawPos)

		quote, quoteErr := tb.client.getQuote(ctx, rawPos.Symbol)
		if quoteErr != nil {
			return nil, fmt.Errorf("tradier: get quote for %s: %w", rawPos.Symbol, quoteErr)
		}

		pos.MarkPrice = quote
		positions[ii] = pos
	}

	return positions, nil
}

// Balance returns the current account balance.
func (tb *TradierBroker) Balance(ctx context.Context) (broker.Balance, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	rawBalance, getErr := tb.client.getBalance(ctx)
	if getErr != nil {
		return broker.Balance{}, getErr
	}

	return toBrokerBalance(rawBalance), nil
}
