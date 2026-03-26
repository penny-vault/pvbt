package tradestation

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	fillChannelSize   = 1024
	apiBaseURLDefault = "https://api.tradestation.com/v3"
	apiBaseURLSandbox = "https://sim-api.tradestation.com/v3"
)

// TradeStationBroker implements broker.Broker and broker.GroupSubmitter for the
// TradeStation brokerage.
type TradeStationBroker struct {
	client           *apiClient
	auth             *tokenManager
	streamer         *orderStreamer
	fills            chan broker.Fill
	accountID        string
	tokenFile        string
	callbackURL      string
	sandbox          bool
	desiredAccountID string
	mu               sync.Mutex
}

// Option configures a TradeStationBroker.
type Option func(*TradeStationBroker)

// WithTokenFile configures the path to the token persistence file.
func WithTokenFile(path string) Option {
	return func(tsBroker *TradeStationBroker) {
		tsBroker.tokenFile = path
	}
}

// WithCallbackURL configures the OAuth callback URL.
func WithCallbackURL(callbackURL string) Option {
	return func(tsBroker *TradeStationBroker) {
		tsBroker.callbackURL = callbackURL
	}
}

// WithSandbox configures the broker to use the simulation environment.
func WithSandbox() Option {
	return func(tsBroker *TradeStationBroker) {
		tsBroker.sandbox = true
	}
}

// WithAccountID configures the account ID to use for trading.
func WithAccountID(accountID string) Option {
	return func(tsBroker *TradeStationBroker) {
		tsBroker.desiredAccountID = accountID
	}
}

// New creates a new TradeStationBroker with the given options.
func New(opts ...Option) *TradeStationBroker {
	tsBroker := &TradeStationBroker{
		fills: make(chan broker.Fill, fillChannelSize),
	}

	for _, opt := range opts {
		opt(tsBroker)
	}

	return tsBroker
}

// Fills returns the channel on which fill notifications are delivered.
func (tsBroker *TradeStationBroker) Fills() <-chan broker.Fill {
	return tsBroker.fills
}

// Connect authenticates with TradeStation and starts the order streamer.
func (tsBroker *TradeStationBroker) Connect(ctx context.Context) error {
	clientID := os.Getenv("TRADESTATION_CLIENT_ID")
	clientSecret := os.Getenv("TRADESTATION_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		return broker.ErrMissingCredentials
	}

	callbackURL := tsBroker.callbackURL
	if callbackURL == "" {
		callbackURL = os.Getenv("TRADESTATION_CALLBACK_URL")
	}

	tokenFile := tsBroker.tokenFile
	if tokenFile == "" {
		tokenFile = os.Getenv("TRADESTATION_TOKEN_FILE")
	}

	tsBroker.auth = newTokenManager(clientID, clientSecret, callbackURL, tokenFile)

	// Attempt to load existing tokens.
	tokens, loadErr := loadTokens(tsBroker.auth.tokenFile)
	if loadErr == nil {
		tsBroker.auth.tokens = tokens
	}

	// Ensure we have a valid access token.
	if ensureErr := tsBroker.auth.ensureValidToken(); ensureErr != nil {
		if authErr := tsBroker.auth.startAuthFlow(); authErr != nil {
			return fmt.Errorf("tradestation: connect: %w", authErr)
		}
	}

	// Create the API client.
	baseURL := apiBaseURLDefault
	if tsBroker.sandbox {
		baseURL = apiBaseURLSandbox
	}

	tsBroker.client = newAPIClient(baseURL, tsBroker.auth.accessToken())

	tsBroker.auth.onRefresh = func(token string) {
		tsBroker.client.setToken(token)
	}

	// Resolve the account ID.
	desiredAccount := tsBroker.desiredAccountID
	if desiredAccount == "" {
		desiredAccount = os.Getenv("TRADESTATION_ACCOUNT_ID")
	}

	accountID, resolveErr := tsBroker.client.resolveAccount(ctx, desiredAccount)
	if resolveErr != nil {
		return fmt.Errorf("tradestation: connect: %w", resolveErr)
	}

	tsBroker.accountID = accountID
	tsBroker.client.accountID = accountID

	// Start the HTTP chunked order streamer.
	tsBroker.streamer = newOrderStreamer(
		tsBroker.client,
		tsBroker.fills,
		baseURL,
		accountID,
		tsBroker.auth.accessToken,
	)

	if connectErr := tsBroker.streamer.connect(ctx); connectErr != nil {
		return fmt.Errorf("tradestation: connect streamer: %w", connectErr)
	}

	// Start background token refresh.
	tsBroker.auth.startBackgroundRefresh()

	return nil
}

// Close stops the background refresh, closes the streamer, and closes the fills channel.
func (tsBroker *TradeStationBroker) Close() error {
	if tsBroker.auth != nil {
		tsBroker.auth.stopBackgroundRefresh()
	}

	if tsBroker.streamer != nil {
		if closeErr := tsBroker.streamer.close(); closeErr != nil {
			return closeErr
		}
	}

	close(tsBroker.fills)

	return nil
}

// Submit places a single order. If Qty is zero and Amount is set, the share
// quantity is derived from the current quote price using math.Floor.
func (tsBroker *TradeStationBroker) Submit(ctx context.Context, order broker.Order) error {
	tsBroker.mu.Lock()
	defer tsBroker.mu.Unlock()

	qty := order.Qty
	if qty == 0 && order.Amount > 0 {
		price, quoteErr := tsBroker.client.getQuote(ctx, order.Asset.Ticker)
		if quoteErr != nil {
			return fmt.Errorf("tradestation: fetching quote for %s: %w", order.Asset.Ticker, quoteErr)
		}

		qty = math.Floor(order.Amount / price)
		if qty == 0 {
			return nil
		}
	}

	order.Qty = qty

	tsOrder, translateErr := toTSOrder(order, tsBroker.accountID)
	if translateErr != nil {
		return translateErr
	}

	_, submitErr := tsBroker.client.submitOrder(ctx, tsOrder)
	if submitErr != nil {
		return fmt.Errorf("tradestation: submit order: %w", submitErr)
	}

	return nil
}

// Cancel cancels an open order by ID. Dashes are stripped per TradeStation requirements.
func (tsBroker *TradeStationBroker) Cancel(ctx context.Context, orderID string) error {
	return tsBroker.client.cancelOrder(ctx, stripDashes(orderID))
}

// Replace cancels an existing order and submits a replacement.
func (tsBroker *TradeStationBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	tsOrder, translateErr := toTSOrder(order, tsBroker.accountID)
	if translateErr != nil {
		return translateErr
	}

	return tsBroker.client.replaceOrder(ctx, stripDashes(orderID), tsOrder)
}

// Orders returns all open and recently-completed orders for the account.
func (tsBroker *TradeStationBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	responses, getErr := tsBroker.client.getOrders(ctx)
	if getErr != nil {
		return nil, fmt.Errorf("tradestation: get orders: %w", getErr)
	}

	orders := make([]broker.Order, len(responses))
	for idx, resp := range responses {
		orders[idx] = toBrokerOrder(resp)
	}

	return orders, nil
}

// Positions returns all current positions held in the account.
func (tsBroker *TradeStationBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	responses, getErr := tsBroker.client.getPositions(ctx)
	if getErr != nil {
		return nil, fmt.Errorf("tradestation: get positions: %w", getErr)
	}

	positions := make([]broker.Position, len(responses))
	for idx, resp := range responses {
		positions[idx] = toBrokerPosition(resp)
	}

	return positions, nil
}

// Balance returns the current account balances.
func (tsBroker *TradeStationBroker) Balance(ctx context.Context) (broker.Balance, error) {
	resp, getErr := tsBroker.client.getBalance(ctx)
	if getErr != nil {
		return broker.Balance{}, fmt.Errorf("tradestation: get balance: %w", getErr)
	}

	return toBrokerBalance(resp), nil
}

// Transactions returns transactions since the given time. TradeStation v3 API
// does not provide a transaction history endpoint.
func (tsBroker *TradeStationBroker) Transactions(_ context.Context, _ time.Time) ([]broker.Transaction, error) {
	log.Info().Msg("tradestation: TradeStation v3 API does not provide a transaction history endpoint; dividends, splits, and fees will not be synced")
	return nil, nil
}

// SubmitGroup submits a group of orders as a native OCO or bracket order.
func (tsBroker *TradeStationBroker) SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error {
	tsBroker.mu.Lock()
	defer tsBroker.mu.Unlock()

	groupOrder, buildErr := buildGroupOrder(orders, groupType, tsBroker.accountID)
	if buildErr != nil {
		return buildErr
	}

	return tsBroker.client.submitGroupOrder(ctx, groupOrder)
}

// formatQty formats a quantity as an integer string when it is a whole number,
// or as a decimal string otherwise.
func formatQty(qty float64) string {
	if qty == math.Floor(qty) {
		return strconv.FormatInt(int64(qty), 10)
	}

	return strconv.FormatFloat(qty, 'f', -1, 64)
}
