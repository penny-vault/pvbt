package etrade

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/penny-vault/pvbt/broker"
)

const (
	defaultAPIBaseURL = "https://api.etrade.com"
	sandboxAPIBaseURL = "https://apisb.etrade.com"
)

// EtradeBroker implements broker.Broker for the E*TRADE brokerage.
type EtradeBroker struct {
	auth       *tokenManager
	client     *apiClient
	accountID  string
	apiBaseURL string
	fills      chan broker.Fill
}

// Option configures an EtradeBroker.
type Option func(*EtradeBroker)

// New creates a new EtradeBroker with the given options.
// It reads ETRADE_CONSUMER_KEY and ETRADE_CONSUMER_SECRET from the environment.
func New(opts ...Option) *EtradeBroker {
	consumerKey := os.Getenv("ETRADE_CONSUMER_KEY")
	consumerSecret := os.Getenv("ETRADE_CONSUMER_SECRET")

	eb := &EtradeBroker{
		auth:       newTokenManager(consumerKey, consumerSecret, "", ""),
		apiBaseURL: defaultAPIBaseURL,
		fills:      make(chan broker.Fill, 64),
	}

	for _, opt := range opts {
		opt(eb)
	}

	return eb
}

// WithSandbox configures the broker to target the E*TRADE sandbox environment.
func WithSandbox() Option {
	return func(eb *EtradeBroker) {
		eb.apiBaseURL = sandboxAPIBaseURL
	}
}

// WithAccountID sets the E*TRADE account ID to use.
func WithAccountID(id string) Option {
	return func(eb *EtradeBroker) {
		eb.accountID = id
	}
}

// Connect establishes a session with E*TRADE. If no access token is stored on
// disk, it starts the interactive OAuth 1.0a authorization flow.
func (eb *EtradeBroker) Connect(ctx context.Context) error {
	tokenPath := expandHome(eb.auth.tokenFile)

	existing, loadErr := loadTokens(tokenPath)
	if loadErr == nil {
		eb.auth.creds.AccessToken = existing.AccessToken
		eb.auth.creds.AccessSecret = existing.AccessSecret
	} else {
		if authErr := eb.auth.startAuthFlow(); authErr != nil {
			return fmt.Errorf("etrade: connect: auth flow: %w", authErr)
		}
	}

	eb.auth.startBackgroundRenewal()

	// Initialize the API client with the current credentials.
	eb.client = newAPIClient(eb.apiBaseURL, &eb.auth.creds, "")

	// Keep client credentials in sync when the token manager refreshes.
	eb.auth.onRefresh = func(creds oauthCredentials) {
		eb.client.setCreds(&creds)
	}

	// Resolve the accountIdKey by listing accounts.
	accountIDKey, resolveErr := eb.resolveAccountIDKey(ctx)
	if resolveErr != nil {
		return fmt.Errorf("etrade: connect: resolve account: %w", resolveErr)
	}

	eb.client.accountIDKey = accountIDKey

	return nil
}

// resolveAccountIDKey queries the account list and returns the accountIdKey for
// the configured accountID.
func (eb *EtradeBroker) resolveAccountIDKey(ctx context.Context) (string, error) {
	resp, reqErr := eb.client.resty.R().
		SetContext(ctx).
		Get("/v1/accounts/list.json")
	if reqErr != nil {
		return "", fmt.Errorf("list accounts: %w", reqErr)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var result etradeAccountListResponse
	if unmarshalErr := json.Unmarshal(resp.Body(), &result); unmarshalErr != nil {
		return "", fmt.Errorf("list accounts: decode: %w", unmarshalErr)
	}

	key := accountListToIDKey(result, eb.accountID)
	if key == "" {
		return "", fmt.Errorf("account %q not found", eb.accountID)
	}

	return key, nil
}

// Close tears down the broker session.
func (eb *EtradeBroker) Close() error {
	eb.auth.stopBackgroundRenewal()

	return nil
}

// Submit sends an order to E*TRADE.
func (eb *EtradeBroker) Submit(ctx context.Context, order broker.Order) error {
	// When a dollar amount is provided instead of a share quantity, look up the
	// current price to compute the quantity.
	qty := order.Qty
	if qty == 0 && order.Amount > 0 {
		price, quoteErr := eb.client.getQuote(ctx, order.Asset.Ticker)
		if quoteErr != nil {
			return fmt.Errorf("etrade: submit order: get quote for %s: %w", order.Asset.Ticker, quoteErr)
		}

		qty = math.Floor(order.Amount / price)
		if qty == 0 {
			return nil
		}
	}

	order.Qty = qty

	req, buildErr := toEtradeOrderRequest(order)
	if buildErr != nil {
		return buildErr
	}

	previewID, previewErr := eb.client.previewOrder(ctx, req)
	if previewErr != nil {
		return fmt.Errorf("etrade: submit order: preview: %w", previewErr)
	}

	_, placeErr := eb.client.placeOrder(ctx, req, previewID)
	if placeErr != nil {
		return fmt.Errorf("etrade: submit order: place: %w", placeErr)
	}

	return nil
}

// Fills returns the fills channel.
func (eb *EtradeBroker) Fills() <-chan broker.Fill {
	return eb.fills
}

// Cancel requests cancellation of an open order.
func (eb *EtradeBroker) Cancel(ctx context.Context, orderID string) error {
	if cancelErr := eb.client.cancelOrder(ctx, orderID); cancelErr != nil {
		return fmt.Errorf("etrade: cancel order: %w", cancelErr)
	}

	return nil
}

// Replace cancels an existing order and submits a replacement.
func (eb *EtradeBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	req, buildErr := toEtradeOrderRequest(order)
	if buildErr != nil {
		return buildErr
	}

	previewID, previewErr := eb.client.previewModifyOrder(ctx, orderID, req)
	if previewErr != nil {
		return fmt.Errorf("etrade: replace order: preview: %w", previewErr)
	}

	_, placeErr := eb.client.placeModifyOrder(ctx, orderID, req, previewID)
	if placeErr != nil {
		return fmt.Errorf("etrade: replace order: place: %w", placeErr)
	}

	return nil
}

// Orders returns all orders for the current trading day.
func (eb *EtradeBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	details, fetchErr := eb.client.getOrders(ctx)
	if fetchErr != nil {
		return nil, fmt.Errorf("etrade: orders: %w", fetchErr)
	}

	orders := make([]broker.Order, 0, len(details))
	for _, detail := range details {
		orders = append(orders, toBrokerOrder(detail))
	}

	return orders, nil
}

// Positions returns all current positions in the account.
func (eb *EtradeBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	rawPositions, fetchErr := eb.client.getPositions(ctx)
	if fetchErr != nil {
		return nil, fmt.Errorf("etrade: positions: %w", fetchErr)
	}

	positions := make([]broker.Position, 0, len(rawPositions))
	for _, pos := range rawPositions {
		positions = append(positions, toBrokerPosition(pos))
	}

	return positions, nil
}

// Balance returns the current account balance.
func (eb *EtradeBroker) Balance(ctx context.Context) (broker.Balance, error) {
	resp, fetchErr := eb.client.getBalance(ctx)
	if fetchErr != nil {
		return broker.Balance{}, fmt.Errorf("etrade: balance: %w", fetchErr)
	}

	return toBrokerBalance(resp), nil
}

// Transactions returns account activity since the given time.
func (eb *EtradeBroker) Transactions(ctx context.Context, since time.Time) ([]broker.Transaction, error) {
	rawTxns, fetchErr := eb.client.getTransactions(ctx, since)
	if fetchErr != nil {
		return nil, fmt.Errorf("etrade: transactions: %w", fetchErr)
	}

	txns := make([]broker.Transaction, 0, len(rawTxns))
	for _, txn := range rawTxns {
		txns = append(txns, toBrokerTransaction(txn))
	}

	return txns, nil
}

// accountListToIDKey returns the accountIdKey from an account list response.
// Used by Connect to resolve the account ID key.
func accountListToIDKey(resp etradeAccountListResponse, accountID string) string {
	for _, acct := range resp.AccountListResponse.Accounts.Account {
		if acct.AccountID == accountID {
			return acct.AccountIDKey
		}
	}

	return ""
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
