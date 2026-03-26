package etrade

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
)

const (
	defaultAPIBaseURL = "https://api.etrade.com"
	sandboxAPIBaseURL = "https://apisb.etrade.com"
)

// EtradeBroker implements broker.Broker for the E*TRADE brokerage.
type EtradeBroker struct {
	client      *apiClient
	auth        *tokenManager
	poller      *orderPoller
	fills       chan broker.Fill
	mu          sync.Mutex
	sandbox     bool
	tokenFile   string
	callbackURL string
}

// Option configures an EtradeBroker.
type Option func(*EtradeBroker)

// WithSandbox configures the broker to target the E*TRADE sandbox environment.
func WithSandbox() Option {
	return func(eb *EtradeBroker) {
		eb.sandbox = true
	}
}

// WithTokenFile configures the path to the token persistence file.
func WithTokenFile(path string) Option {
	return func(eb *EtradeBroker) {
		eb.tokenFile = path
	}
}

// WithCallbackURL configures the OAuth callback URL.
func WithCallbackURL(callbackURL string) Option {
	return func(eb *EtradeBroker) {
		eb.callbackURL = callbackURL
	}
}

// New creates a new EtradeBroker with the given options.
func New(opts ...Option) *EtradeBroker {
	eb := &EtradeBroker{fills: make(chan broker.Fill, 1024)}
	for _, opt := range opts {
		opt(eb)
	}

	return eb
}

// Fills returns the channel on which fill reports are delivered.
func (eb *EtradeBroker) Fills() <-chan broker.Fill {
	return eb.fills
}

// Connect establishes an authenticated session with E*TRADE. It reads
// credentials from environment variables, loads any existing tokens, attempts
// renewal, and falls back to the interactive auth flow when necessary.
func (eb *EtradeBroker) Connect(ctx context.Context) error {
	consumerKey := os.Getenv("ETRADE_CONSUMER_KEY")
	consumerSecret := os.Getenv("ETRADE_CONSUMER_SECRET")
	accountIDKey := os.Getenv("ETRADE_ACCOUNT_ID_KEY")

	if consumerKey == "" || consumerSecret == "" || accountIDKey == "" {
		return fmt.Errorf("etrade: connect: %w", broker.ErrMissingCredentials)
	}

	callbackURL := eb.callbackURL
	if callbackURL == "" {
		callbackURL = os.Getenv("ETRADE_CALLBACK_URL")
	}

	tokenFile := eb.tokenFile
	if tokenFile == "" {
		tokenFile = os.Getenv("ETRADE_TOKEN_FILE")
	}

	eb.auth = newTokenManager(consumerKey, consumerSecret, callbackURL, tokenFile)

	// Attempt to load existing tokens.
	existing, loadErr := loadTokens(expandHome(eb.auth.tokenFile))
	if loadErr == nil {
		eb.auth.creds.AccessToken = existing.AccessToken
		eb.auth.creds.AccessSecret = existing.AccessSecret
	}

	// Try to renew; if that fails, run the interactive auth flow.
	if renewErr := eb.auth.renewAccessToken(); renewErr != nil {
		if authErr := eb.auth.startAuthFlow(); authErr != nil {
			return fmt.Errorf("etrade: connect: auth flow: %w", authErr)
		}
	}

	baseURL := defaultAPIBaseURL
	if eb.sandbox {
		baseURL = sandboxAPIBaseURL
	}

	eb.client = newAPIClient(baseURL, &eb.auth.creds, accountIDKey)

	// Keep client credentials in sync when the token manager refreshes.
	eb.auth.onRefresh = func(creds oauthCredentials) {
		eb.client.setCreds(&creds)
	}

	eb.auth.startBackgroundRenewal()

	eb.poller = newOrderPoller(eb.client, eb.fills)
	eb.poller.start(ctx)

	return nil
}

// Close tears down the broker session and releases resources.
func (eb *EtradeBroker) Close() error {
	if eb.auth != nil {
		eb.auth.stopBackgroundRenewal()
	}

	if eb.poller != nil {
		eb.poller.stop()
	}

	return nil
}

// Submit sends an order to E*TRADE using a preview-then-place flow.
// If Qty is 0 and Amount > 0, a quote is fetched and qty = floor(Amount/price).
// An error is returned if the resulting quantity is zero.
// The order action (BUY/SELL/SELL_SHORT/BUY_TO_COVER) is determined by
// comparing the requested side against existing positions.
func (eb *EtradeBroker) Submit(ctx context.Context, order broker.Order) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Dollar-amount conversion.
	if order.Qty == 0 && order.Amount > 0 {
		price, quoteErr := eb.client.getQuote(ctx, order.Asset.Ticker)
		if quoteErr != nil {
			return fmt.Errorf("etrade: submit: get quote for %s: %w", order.Asset.Ticker, quoteErr)
		}

		qty := math.Floor(order.Amount / price)
		if qty == 0 {
			return fmt.Errorf("etrade: submit: dollar-amount order for %s results in zero shares at price %.4f", order.Asset.Ticker, price)
		}

		order.Qty = qty
	}

	// Detect the correct order action from current positions.
	positions, posErr := eb.client.getPositions(ctx)
	if posErr != nil {
		return fmt.Errorf("etrade: submit: get positions for side detection: %w", posErr)
	}

	action := detectAction(order.Side, order.Asset.Ticker, positions)

	req, buildErr := toEtradeOrderRequest(order)
	if buildErr != nil {
		return buildErr
	}

	// Override the orderAction in the first instrument leg.
	if len(req.Order) > 0 && len(req.Order[0].Instrument) > 0 {
		req.Order[0].Instrument[0].OrderAction = action
	}

	previewID, previewErr := eb.client.previewOrder(ctx, req)
	if previewErr != nil {
		return fmt.Errorf("etrade: submit: preview: %w", previewErr)
	}

	_, placeErr := eb.client.placeOrder(ctx, req, previewID)
	if placeErr != nil {
		return fmt.Errorf("etrade: submit: place: %w", placeErr)
	}

	return nil
}

// detectAction returns the E*TRADE order action string based on the requested
// side and any existing position for the ticker.
func detectAction(side broker.Side, ticker string, positions []etradePosition) string {
	var currentQty float64

	for _, pos := range positions {
		if pos.Product.Symbol == ticker {
			currentQty = pos.Quantity
			break
		}
	}

	switch side {
	case broker.Sell:
		if currentQty <= 0 {
			return "SELL_SHORT"
		}

		return "SELL"
	case broker.Buy:
		if currentQty < 0 {
			return "BUY_TO_COVER"
		}

		return "BUY"
	default:
		return "BUY"
	}
}

// Cancel requests cancellation of an open order.
func (eb *EtradeBroker) Cancel(ctx context.Context, orderID string) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	return eb.client.cancelOrder(ctx, orderID)
}

// Replace modifies an existing order via the E*TRADE change/preview + change/place flow.
func (eb *EtradeBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	req, buildErr := toEtradeOrderRequest(order)
	if buildErr != nil {
		return buildErr
	}

	previewID, previewErr := eb.client.previewModifyOrder(ctx, orderID, req)
	if previewErr != nil {
		return fmt.Errorf("etrade: replace: preview: %w", previewErr)
	}

	_, placeErr := eb.client.placeModifyOrder(ctx, orderID, req, previewID)
	if placeErr != nil {
		return fmt.Errorf("etrade: replace: place: %w", placeErr)
	}

	return nil
}

// Orders returns all orders for the current trading day.
func (eb *EtradeBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	rawOrders, err := eb.client.getOrders(ctx)
	if err != nil {
		return nil, err
	}

	orders := make([]broker.Order, len(rawOrders))
	for ii, raw := range rawOrders {
		orders[ii] = toBrokerOrder(raw)
	}

	return orders, nil
}

// Positions returns all current positions in the account, fetching a mark
// price quote for each position.
func (eb *EtradeBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	rawPositions, err := eb.client.getPositions(ctx)
	if err != nil {
		return nil, err
	}

	positions := make([]broker.Position, len(rawPositions))
	for ii, raw := range rawPositions {
		positions[ii] = toBrokerPosition(raw)

		quote, quoteErr := eb.client.getQuote(ctx, raw.Product.Symbol)
		if quoteErr != nil {
			return nil, fmt.Errorf("etrade: get quote for %s: %w", raw.Product.Symbol, quoteErr)
		}

		positions[ii].MarkPrice = quote
	}

	return positions, nil
}

// Balance returns the current account balance.
func (eb *EtradeBroker) Balance(ctx context.Context) (broker.Balance, error) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	rawBalance, err := eb.client.getBalance(ctx)
	if err != nil {
		return broker.Balance{}, err
	}

	return toBrokerBalance(rawBalance), nil
}

// Transactions returns account activity since the given time.
func (eb *EtradeBroker) Transactions(ctx context.Context, since time.Time) ([]broker.Transaction, error) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	rawTxns, err := eb.client.getTransactions(ctx, since)
	if err != nil {
		return nil, err
	}

	txns := make([]broker.Transaction, len(rawTxns))
	for ii, raw := range rawTxns {
		txns[ii] = toBrokerTransaction(raw)
	}

	return txns, nil
}

// quoteFromResponse extracts the last trade price from a quote response.
func quoteFromResponse(resp etradeQuoteResponse) float64 {
	if len(resp.QuoteResponse.QuoteData) == 0 {
		return 0
	}

	return resp.QuoteResponse.QuoteData[0].All.LastTrade
}

// previewIDFromResponse extracts the preview ID from a preview response.
func previewIDFromResponse(resp etradePreviewResponse) int64 {
	if len(resp.PreviewOrderResponse.PreviewIDs) == 0 {
		return 0
	}

	return resp.PreviewOrderResponse.PreviewIDs[0].PreviewID
}

// orderIDFromPlaceResponse extracts the order ID from a place response.
func orderIDFromPlaceResponse(resp etradePlaceResponse) int64 {
	return resp.PlaceOrderResponse.OrderID
}
