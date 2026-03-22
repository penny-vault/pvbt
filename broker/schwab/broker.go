package schwab

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/penny-vault/pvbt/broker"
)

const (
	fillChannelSize = 1024
)

// SchwabBroker implements broker.Broker and broker.GroupSubmitter for the
// Charles Schwab brokerage.
type SchwabBroker struct {
	client      *apiClient
	auth        *tokenManager
	streamer    *activityStreamer
	fills       chan broker.Fill
	accountHash string
	tokenFile   string
	callbackURL string
	mu          sync.Mutex
}

// Option configures a SchwabBroker.
type Option func(*SchwabBroker)

// WithTokenFile configures the path to the token persistence file.
func WithTokenFile(path string) Option {
	return func(schwabBroker *SchwabBroker) {
		schwabBroker.tokenFile = path
	}
}

// WithCallbackURL configures the OAuth callback URL.
func WithCallbackURL(callbackURL string) Option {
	return func(schwabBroker *SchwabBroker) {
		schwabBroker.callbackURL = callbackURL
	}
}

// New creates a new SchwabBroker with the given options.
func New(opts ...Option) *SchwabBroker {
	schwabBroker := &SchwabBroker{
		fills: make(chan broker.Fill, fillChannelSize),
	}

	for _, opt := range opts {
		opt(schwabBroker)
	}

	return schwabBroker
}

// Fills returns the channel on which fill notifications are delivered.
func (schwabBroker *SchwabBroker) Fills() <-chan broker.Fill {
	return schwabBroker.fills
}

// Connect authenticates with Schwab and starts the activity streamer.
func (schwabBroker *SchwabBroker) Connect(ctx context.Context) error {
	clientID := os.Getenv("SCHWAB_CLIENT_ID")
	clientSecret := os.Getenv("SCHWAB_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		return broker.ErrMissingCredentials
	}

	callbackURL := schwabBroker.callbackURL
	if callbackURL == "" {
		callbackURL = os.Getenv("SCHWAB_CALLBACK_URL")
	}

	tokenFile := schwabBroker.tokenFile
	if tokenFile == "" {
		tokenFile = os.Getenv("SCHWAB_TOKEN_FILE")
	}

	schwabBroker.auth = newTokenManager(clientID, clientSecret, callbackURL, tokenFile)

	// Attempt to load existing tokens.
	tokens, loadErr := loadTokens(schwabBroker.auth.tokenFile)
	if loadErr == nil {
		schwabBroker.auth.tokens = tokens
	}

	// Ensure we have a valid access token.
	if ensureErr := schwabBroker.auth.ensureValidToken(); ensureErr != nil {
		// Need to run the browser authorization flow.
		if authErr := schwabBroker.auth.startAuthFlow(); authErr != nil {
			return fmt.Errorf("schwab: connect: %w", authErr)
		}
	}

	// Create the API client with the access token.
	schwabBroker.client = newAPIClient(authBaseURLDefault, schwabBroker.auth.accessToken())

	schwabBroker.auth.onRefresh = func(token string) {
		schwabBroker.client.setToken(token)
	}

	// Resolve the account hash.
	desiredAccount := os.Getenv("SCHWAB_ACCOUNT_NUMBER")

	accountHash, resolveErr := schwabBroker.client.resolveAccount(ctx, desiredAccount)
	if resolveErr != nil {
		return fmt.Errorf("schwab: connect: %w", resolveErr)
	}

	schwabBroker.accountHash = accountHash
	schwabBroker.client.accountHash = accountHash

	// Fetch streamer connection info.
	pref, prefErr := schwabBroker.client.getUserPreference(ctx)
	if prefErr != nil {
		return fmt.Errorf("schwab: connect: %w", prefErr)
	}

	if len(pref.StreamerInfo) == 0 {
		return fmt.Errorf("schwab: connect: no streamer info available")
	}

	streamerInfo := pref.StreamerInfo[0]

	// Start the WebSocket streamer.
	schwabBroker.streamer = newActivityStreamer(
		schwabBroker.client,
		schwabBroker.fills,
		streamerInfo.StreamerSocketURL,
		streamerInfo,
		accountHash,
		schwabBroker.auth.accessToken,
	)

	if connectErr := schwabBroker.streamer.connect(ctx); connectErr != nil {
		return fmt.Errorf("schwab: connect streamer: %w", connectErr)
	}

	// Start background token refresh.
	schwabBroker.auth.startBackgroundRefresh()

	return nil
}

// Close stops the background refresh, closes the streamer, and closes the fills channel.
func (schwabBroker *SchwabBroker) Close() error {
	if schwabBroker.auth != nil {
		schwabBroker.auth.stopBackgroundRefresh()
	}

	if schwabBroker.streamer != nil {
		if closeErr := schwabBroker.streamer.close(); closeErr != nil {
			return closeErr
		}
	}

	close(schwabBroker.fills)

	return nil
}

// Submit places a single order. If Qty is zero and Amount is set, the share
// quantity is derived from the current quote price using math.Floor.
func (schwabBroker *SchwabBroker) Submit(ctx context.Context, order broker.Order) error {
	schwabBroker.mu.Lock()
	defer schwabBroker.mu.Unlock()

	qty := order.Qty
	if qty == 0 && order.Amount > 0 {
		price, quoteErr := schwabBroker.client.getQuote(ctx, order.Asset.Ticker)
		if quoteErr != nil {
			return fmt.Errorf("schwab: fetching quote for %s: %w", order.Asset.Ticker, quoteErr)
		}

		qty = math.Floor(order.Amount / price)
		if qty == 0 {
			return nil
		}
	}

	order.Qty = qty

	schwabOrder, tifErr := toSchwabOrder(order)
	if tifErr != nil {
		return tifErr
	}

	_, submitErr := schwabBroker.client.submitOrder(ctx, schwabOrder)
	if submitErr != nil {
		return fmt.Errorf("schwab: submit order: %w", submitErr)
	}

	return nil
}

// Cancel cancels an open order by ID.
func (schwabBroker *SchwabBroker) Cancel(ctx context.Context, orderID string) error {
	return schwabBroker.client.cancelOrder(ctx, orderID)
}

// Replace cancels an existing order and submits a replacement.
func (schwabBroker *SchwabBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	schwabOrder, tifErr := toSchwabOrder(order)
	if tifErr != nil {
		return tifErr
	}

	_, replaceErr := schwabBroker.client.replaceOrder(ctx, orderID, schwabOrder)

	return replaceErr
}

// Orders returns all open and recently-completed orders for the account.
func (schwabBroker *SchwabBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	responses, getErr := schwabBroker.client.getOrders(ctx)
	if getErr != nil {
		return nil, fmt.Errorf("schwab: get orders: %w", getErr)
	}

	orders := make([]broker.Order, len(responses))
	for idx, resp := range responses {
		orders[idx] = toBrokerOrder(resp)
	}

	return orders, nil
}

// Positions returns all current positions held in the account.
func (schwabBroker *SchwabBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	responses, getErr := schwabBroker.client.getPositions(ctx)
	if getErr != nil {
		return nil, fmt.Errorf("schwab: get positions: %w", getErr)
	}

	positions := make([]broker.Position, len(responses))
	for idx, resp := range responses {
		positions[idx] = toBrokerPosition(resp)
	}

	return positions, nil
}

// Balance returns the current account balances.
func (schwabBroker *SchwabBroker) Balance(ctx context.Context) (broker.Balance, error) {
	resp, getErr := schwabBroker.client.getBalance(ctx)
	if getErr != nil {
		return broker.Balance{}, fmt.Errorf("schwab: get balance: %w", getErr)
	}

	return toBrokerBalance(resp), nil
}

// SubmitGroup submits a group of orders as a native bracket or OCO order.
func (schwabBroker *SchwabBroker) SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error {
	if len(orders) == 0 {
		return broker.ErrEmptyOrderGroup
	}

	schwabBroker.mu.Lock()
	defer schwabBroker.mu.Unlock()

	switch groupType {
	case broker.GroupOCO:
		return schwabBroker.submitOCO(ctx, orders)
	case broker.GroupBracket:
		return schwabBroker.submitBracket(ctx, orders)
	default:
		return fmt.Errorf("schwab: unsupported group type %d", groupType)
	}
}

func (schwabBroker *SchwabBroker) submitOCO(ctx context.Context, orders []broker.Order) error {
	ocoOrder, buildErr := buildOCOOrder(orders)
	if buildErr != nil {
		return buildErr
	}

	_, submitErr := schwabBroker.client.submitOrder(ctx, ocoOrder)
	if submitErr != nil {
		return fmt.Errorf("schwab: submit OCO: %w", submitErr)
	}

	return nil
}

func (schwabBroker *SchwabBroker) submitBracket(ctx context.Context, orders []broker.Order) error {
	bracketOrder, buildErr := buildBracketOrder(orders)
	if buildErr != nil {
		return buildErr
	}

	_, submitErr := schwabBroker.client.submitOrder(ctx, bracketOrder)
	if submitErr != nil {
		return fmt.Errorf("schwab: submit bracket: %w", submitErr)
	}

	return nil
}
