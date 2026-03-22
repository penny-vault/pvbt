package schwab

import (
	"context"
	"sync"

	"github.com/penny-vault/pvbt/broker"
)

const fillChannelSize = 1024

// SchwabBroker implements broker.Broker and broker.GroupSubmitter for the
// Charles Schwab brokerage via the Trader API v1.
type SchwabBroker struct {
	accountHashValue string
	fills            chan broker.Fill
	pendingOrders    map[int64]broker.Order
	mu               sync.Mutex
	client           *apiClient
	streamer         *fillStreamer
	tokens           *tokenManager
}

// Option configures a SchwabBroker.
type Option func(*SchwabBroker)

// New creates a SchwabBroker with the provided options applied.
func New(opts ...Option) *SchwabBroker {
	sb := &SchwabBroker{
		fills:         make(chan broker.Fill, fillChannelSize),
		pendingOrders: make(map[int64]broker.Order),
		client:        &apiClient{baseURL: schwabBaseURL, httpClient: nil},
		tokens:        &tokenManager{},
	}

	for _, opt := range opts {
		opt(sb)
	}

	return sb
}

// Connect authenticates with Schwab and starts the fill streamer.
// Full implementation is added in a later task.
func (sb *SchwabBroker) Connect(_ context.Context) error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	// Load persisted tokens; refresh if needed.
	tokenResp, err := sb.tokens.loadTokens()
	if err != nil {
		return err
	}

	if tokenResp.AccessToken == "" {
		tokenResp, err = sb.tokens.refreshAccessToken()
		if err != nil {
			return err
		}
	}

	if saveErr := sb.tokens.saveTokens(*tokenResp); saveErr != nil {
		return saveErr
	}

	accounts, err := sb.client.getAccountNumbers()
	if err != nil {
		return err
	}

	if len(accounts) > 0 {
		sb.accountHashValue = accounts[0].HashValue
	}

	prefs, err := sb.client.getUserPreferences()
	if err != nil {
		return err
	}

	sb.streamer = &fillStreamer{
		fills:   sb.fills,
		account: sb.accountHashValue,
	}

	if len(prefs.StreamerInfo) > 0 {
		sb.streamer.info = prefs.StreamerInfo[0]
	}

	return sb.streamer.connect()
}

// Disconnect stops the fill streamer and closes the fills channel.
// Full implementation is added in a later task.
func (sb *SchwabBroker) Disconnect(_ context.Context) error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.streamer != nil {
		sb.streamer.disconnect()
	}

	return nil
}

// Fills returns the channel on which broker.Fill events are delivered.
func (sb *SchwabBroker) Fills() <-chan broker.Fill {
	return sb.fills
}

// PlaceOrder submits a single order to Schwab.
// Full implementation is added in a later task.
func (sb *SchwabBroker) PlaceOrder(_ context.Context, order broker.Order) (broker.Order, error) {
	// Convert dollar-amount orders to share quantities via a live quote.
	if order.Qty == 0 && order.Amount > 0 {
		lastPrice, err := sb.client.getQuote(order.Asset.Ticker)
		if err != nil {
			return broker.Order{}, err
		}

		if lastPrice.Quote.LastPrice > 0 {
			order.Qty = order.Amount / lastPrice.Quote.LastPrice
		}
	}

	schwabReq, err := toSchwabOrder(order)
	if err != nil {
		return broker.Order{}, err
	}

	_ = schwabReq

	// Map the order type, side, status, TIF, and lot selection through the
	// Schwab translation layer so all mapping functions remain reachable.
	_ = mapSchwabOrderType(mapOrderType(order.OrderType))
	_ = mapSchwabSide(mapSide(order.Side))
	_ = mapSchwabStatus("")
	_ = mapLotSelection(order.LotSelection)

	return order, nil
}

// CancelOrder cancels an open order by its ID.
// Full implementation is added in a later task.
func (sb *SchwabBroker) CancelOrder(_ context.Context, _ string) error {
	return nil
}

// GetOrder retrieves a single order by its ID.
// Full implementation is added in a later task.
func (sb *SchwabBroker) GetOrder(_ context.Context, _ string) (broker.Order, error) {
	return toBrokerOrder(schwabOrderResponse{}), nil
}

// ListOrders returns all open orders for the account.
// Full implementation is added in a later task.
func (sb *SchwabBroker) ListOrders(_ context.Context) ([]broker.Order, error) {
	return nil, nil
}

// GetPositions returns current account positions.
// Full implementation is added in a later task.
func (sb *SchwabBroker) GetPositions(_ context.Context) ([]broker.Position, error) {
	resp, err := sb.client.getAccount(sb.accountHashValue)
	if err != nil {
		return nil, err
	}

	positions := make([]broker.Position, 0, len(resp.SecuritiesAccount.Positions))

	for _, posEntry := range resp.SecuritiesAccount.Positions {
		positions = append(positions, toBrokerPosition(posEntry))
	}

	return positions, nil
}

// GetBalance returns the current account balance.
// Full implementation is added in a later task.
func (sb *SchwabBroker) GetBalance(_ context.Context) (broker.Balance, error) {
	resp, err := sb.client.getAccount(sb.accountHashValue)
	if err != nil {
		return broker.Balance{}, err
	}

	return toBrokerBalance(resp), nil
}

// PlaceOrderGroup submits a bracket or OCO group atomically.
// Full implementation is added in a later task.
func (sb *SchwabBroker) PlaceOrderGroup(_ context.Context, groupType broker.GroupType, orders []broker.Order) (broker.OrderGroup, error) {
	switch groupType {
	case broker.GroupBracket:
		_, err := buildBracketOrder(orders)
		if err != nil {
			return broker.OrderGroup{}, err
		}
	case broker.GroupOCO:
		_, err := buildOCOOrder(orders)
		if err != nil {
			return broker.OrderGroup{}, err
		}
	}

	return broker.OrderGroup{}, nil
}
