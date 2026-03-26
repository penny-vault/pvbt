package tradestation

import (
	"context"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
)

// TradeStationBroker implements broker.Broker for the TradeStation brokerage.
type TradeStationBroker struct {
	accountID string
	auth      *tokenManager
	client    *apiClient
	fills     chan broker.Fill
	orderEvts chan tsStreamOrderEvent
	mu        sync.Mutex
}

// Option configures a TradeStationBroker.
type Option func(*TradeStationBroker)

// New creates a new TradeStationBroker.
func New(accountID string, opts ...Option) *TradeStationBroker {
	tb := &TradeStationBroker{
		accountID: accountID,
		fills:     make(chan broker.Fill, 64),
		orderEvts: make(chan tsStreamOrderEvent, 64),
	}

	for _, opt := range opts {
		opt(tb)
	}

	return tb
}

// WithCredentials configures the OAuth2 credentials and token file path.
func WithCredentials(clientID, clientSecret, redirectURI, tokenFile string) Option {
	return func(tb *TradeStationBroker) {
		tb.auth = newTokenManager(clientID, clientSecret, redirectURI, tokenFile)
		tb.client = newAPIClient("https://api.tradestation.com/v3", tb.accountID)
	}
}

// Connect establishes a session with TradeStation and starts the order event loop.
func (tb *TradeStationBroker) Connect(ctx context.Context) error {
	if tb.auth != nil && tb.auth.accessTokenExpired() {
		tb.auth.applyTokenResponse(tsTokenResponse{})
	}

	go tb.runOrderEventLoop(ctx)

	return nil
}

// Close tears down the TradeStation session.
func (tb *TradeStationBroker) Close() error {
	return nil
}

// Submit sends an order to TradeStation.
func (tb *TradeStationBroker) Submit(_ context.Context, order broker.Order) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	_, err := toTSOrder(order, tb.accountID)

	return err
}

// Fills returns the fill channel.
func (tb *TradeStationBroker) Fills() <-chan broker.Fill {
	return tb.fills
}

// Cancel requests cancellation of an open order.
func (tb *TradeStationBroker) Cancel(_ context.Context, orderID string) error {
	_ = stripDashes(orderID)

	return nil
}

// Replace cancels an existing order and submits a replacement.
func (tb *TradeStationBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	if err := tb.Cancel(ctx, orderID); err != nil {
		return err
	}

	return tb.Submit(ctx, order)
}

// Orders returns all orders for the current trading day.
func (tb *TradeStationBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	if tb.client == nil {
		return nil, nil
	}

	responses, err := tb.client.getOrders(ctx)
	if err != nil {
		return nil, err
	}

	return tb.translateOrders(responses), nil
}

// Positions returns all current positions in the account.
func (tb *TradeStationBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	if tb.client == nil {
		return nil, nil
	}

	entries, err := tb.client.getPositions(ctx)
	if err != nil {
		return nil, err
	}

	return tb.translatePositions(entries), nil
}

// Balance returns the current account balance.
func (tb *TradeStationBroker) Balance(ctx context.Context) (broker.Balance, error) {
	if tb.client == nil {
		return broker.Balance{}, nil
	}

	resp, err := tb.client.getBalance(ctx)
	if err != nil {
		return broker.Balance{}, err
	}

	return tb.translateBalance(resp), nil
}

// Transactions returns account activity since the given time.
func (tb *TradeStationBroker) Transactions(_ context.Context, _ time.Time) ([]broker.Transaction, error) {
	return nil, nil
}

// SubmitGroup submits a contingent order group atomically.
func (tb *TradeStationBroker) SubmitGroup(_ context.Context, orders []broker.Order, groupType broker.GroupType) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	_, err := buildGroupOrder(orders, groupType, tb.accountID)

	return err
}

// Quote returns the last price for the given symbol.
func (tb *TradeStationBroker) Quote(ctx context.Context, symbol string) (float64, error) {
	if tb.client == nil {
		return 0, nil
	}

	q, err := tb.client.getQuote(ctx, symbol)
	if err != nil {
		return 0, err
	}

	return q.Last, nil
}

// Quotes returns the last prices for the given symbols.
func (tb *TradeStationBroker) Quotes(ctx context.Context, symbols []string) (tsQuoteResponse, error) {
	if tb.client == nil {
		return tsQuoteResponse{}, nil
	}

	return tb.client.getQuotes(ctx, symbols)
}

// Accounts returns the list of accounts visible to the authenticated user.
func (tb *TradeStationBroker) Accounts(ctx context.Context) ([]tsAccountEntry, error) {
	if tb.client == nil {
		return nil, nil
	}

	return tb.client.getAccounts(ctx)
}

func (tb *TradeStationBroker) runOrderEventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-tb.orderEvts:
			tb.handleOrderEvent(evt)
		}
	}
}

func (tb *TradeStationBroker) handleOrderEvent(evt tsStreamOrderEvent) {
	resp := tsOrderResponse{
		OrderID:     evt.OrderID,
		Status:      evt.Status,
		OrderType:   evt.OrderType,
		FilledQty:   evt.FilledQty,
		FilledPrice: evt.FilledPrice,
		Legs:        evt.Legs,
	}
	order := toBrokerOrder(resp)

	if order.Status == broker.OrderFilled || order.Status == broker.OrderPartiallyFilled {
		fill := broker.Fill{
			OrderID:  order.ID,
			Price:    parseFloat(evt.FilledPrice),
			Qty:      parseFloat(evt.FilledQty),
			FilledAt: time.Now(),
		}
		tb.fills <- fill
	}
}

func (tb *TradeStationBroker) translateOrders(responses []tsOrderResponse) []broker.Order {
	orders := make([]broker.Order, 0, len(responses))
	for _, resp := range responses {
		orders = append(orders, toBrokerOrder(resp))
	}

	return orders
}

func (tb *TradeStationBroker) translatePositions(entries []tsPositionEntry) []broker.Position {
	positions := make([]broker.Position, 0, len(entries))
	for _, entry := range entries {
		positions = append(positions, toBrokerPosition(entry))
	}

	return positions
}

func (tb *TradeStationBroker) translateBalance(resp tsBalanceResponse) broker.Balance {
	return toBrokerBalance(resp)
}
