package tradier

import (
	"context"
	"fmt"
	"sync"

	"github.com/penny-vault/pvbt/broker"
)

// TradierBroker implements broker.Broker for the Tradier brokerage.
// Full implementation is in progress; this stub anchors the package types.
type TradierBroker struct {
	client   *apiClient
	streamer *activityStreamer
	messages chan []byte
	fills    chan broker.Fill
	mu       sync.Mutex
	sandbox  bool
}

// Option configures a TradierBroker.
type Option func(*TradierBroker)

// WithSandbox configures the broker to use the Tradier sandbox environment.
func WithSandbox() Option {
	return func(tb *TradierBroker) {
		tb.sandbox = true
	}
}

// New creates a new TradierBroker.
func New(opts ...Option) *TradierBroker {
	tb := &TradierBroker{
		fills:    make(chan broker.Fill, 1024),
		messages: make(chan []byte, 256),
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

// Connect establishes a session with Tradier.
func (tb *TradierBroker) Connect(ctx context.Context) error {
	baseURL := productionBaseURL
	if tb.sandbox {
		baseURL = sandboxBaseURL
	}

	tb.client = newAPIClient(baseURL, "")
	tb.streamer = newActivityStreamer(tb.client, tb.fills)

	if connectErr := tb.streamer.connect(ctx); connectErr != nil {
		return connectErr
	}

	go tb.streamer.listen(ctx, tb.messages)

	return nil
}

// Close tears down the broker session.
func (tb *TradierBroker) Close() error {
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

	_, replaceErr := tb.client.replaceOrder(ctx, orderID, params)

	return replaceErr
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
