package tastytrade

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/penny-vault/pvbt/broker"
)

const (
	productionBaseURL = "https://api.tastyworks.com"
	sandboxBaseURL    = "https://api.cert.tastyworks.com"
	productionWSURL   = "wss://streamer.tastyworks.com"
	sandboxWSURL      = "wss://streamer.cert.tastyworks.com"
	fillChannelSize   = 1024
)

// TastytradeBroker implements broker.Broker for the tastytrade brokerage.
type TastytradeBroker struct {
	client   *apiClient
	streamer *fillStreamer
	fills    chan broker.Fill
	sandbox  bool
	mu       sync.Mutex
}

// Option configures a TastytradeBroker.
type Option func(*TastytradeBroker)

// WithSandbox configures the broker to use the tastytrade sandbox environment.
func WithSandbox() Option {
	return func(ttBroker *TastytradeBroker) {
		ttBroker.sandbox = true
	}
}

// New creates a new TastytradeBroker with the given options.
func New(opts ...Option) *TastytradeBroker {
	ttBroker := &TastytradeBroker{
		fills: make(chan broker.Fill, fillChannelSize),
	}

	for _, opt := range opts {
		opt(ttBroker)
	}

	baseURL := productionBaseURL
	if ttBroker.sandbox {
		baseURL = sandboxBaseURL
	}

	ttBroker.client = newAPIClient(baseURL)

	return ttBroker
}

func (ttBroker *TastytradeBroker) Connect(ctx context.Context) error {
	username := os.Getenv("TASTYTRADE_USERNAME")

	password := os.Getenv("TASTYTRADE_PASSWORD")
	if username == "" || password == "" {
		return ErrMissingCredentials
	}

	if err := ttBroker.client.authenticate(ctx, username, password); err != nil {
		return fmt.Errorf("tastytrade: connect: %w", err)
	}

	wsURL := productionWSURL
	if ttBroker.sandbox {
		wsURL = sandboxWSURL
	}

	ttBroker.streamer = newFillStreamer(ttBroker.client, ttBroker.fills, wsURL)
	if err := ttBroker.streamer.connect(ctx); err != nil {
		return fmt.Errorf("tastytrade: connect streamer: %w", err)
	}

	return nil
}

func (ttBroker *TastytradeBroker) Close() error {
	if ttBroker.streamer != nil {
		return ttBroker.streamer.close()
	}

	close(ttBroker.fills)

	return nil
}

func (ttBroker *TastytradeBroker) Fills() <-chan broker.Fill {
	return ttBroker.fills
}

func (ttBroker *TastytradeBroker) Submit(ctx context.Context, order broker.Order) error {
	ttBroker.mu.Lock()
	defer ttBroker.mu.Unlock()

	qty := order.Qty
	if qty == 0 && order.Amount > 0 {
		price, err := ttBroker.client.getQuote(ctx, order.Asset.Ticker)
		if err != nil {
			return fmt.Errorf("tastytrade: fetching quote for %s: %w", order.Asset.Ticker, err)
		}

		qty = math.Floor(order.Amount / price)
		if qty == 0 {
			return nil
		}
	}

	order.Qty = qty
	ttOrder := toTastytradeOrder(order)

	_, err := ttBroker.client.submitOrder(ctx, ttOrder)
	if err != nil {
		return fmt.Errorf("tastytrade: submit order: %w", err)
	}

	return nil
}

func (ttBroker *TastytradeBroker) Cancel(ctx context.Context, orderID string) error {
	return ttBroker.client.cancelOrder(ctx, orderID)
}

func (ttBroker *TastytradeBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	ttOrder := toTastytradeOrder(order)
	return ttBroker.client.replaceOrder(ctx, orderID, ttOrder)
}

func (ttBroker *TastytradeBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	responses, err := ttBroker.client.getOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("tastytrade: get orders: %w", err)
	}

	orders := make([]broker.Order, len(responses))
	for idx, resp := range responses {
		orders[idx] = toBrokerOrder(resp)
	}

	return orders, nil
}

func (ttBroker *TastytradeBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	responses, err := ttBroker.client.getPositions(ctx)
	if err != nil {
		return nil, fmt.Errorf("tastytrade: get positions: %w", err)
	}

	positions := make([]broker.Position, len(responses))
	for idx, resp := range responses {
		positions[idx] = toBrokerPosition(resp)
	}

	return positions, nil
}

func (ttBroker *TastytradeBroker) Balance(ctx context.Context) (broker.Balance, error) {
	resp, err := ttBroker.client.getBalance(ctx)
	if err != nil {
		return broker.Balance{}, fmt.Errorf("tastytrade: get balance: %w", err)
	}

	return toBrokerBalance(resp), nil
}
