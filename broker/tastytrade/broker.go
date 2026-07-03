package tastytrade

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
	productionBaseURL = "https://api.tastyworks.com"
	sandboxBaseURL    = "https://api.cert.tastyworks.com"
	productionWSURL   = "wss://streamer.tastyworks.com"
	sandboxWSURL      = "wss://streamer.cert.tastyworks.com"
	fillChannelSize   = 1024
)

// TastytradeBroker implements broker.Broker for the tastytrade brokerage.
type TastytradeBroker struct {
	client          *apiClient
	streamer        *fillStreamer
	fills           chan broker.Fill
	sandbox         bool
	complexOrderIDs map[string]string // maps child order ID -> complex order ID
	mu              sync.Mutex
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
		fills:           make(chan broker.Fill, fillChannelSize),
		complexOrderIDs: make(map[string]string),
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
	var err error
	if ttBroker.streamer != nil {
		err = ttBroker.streamer.close()
	}

	// Always close the fills channel so consumers ranging over Fills()
	// terminate, matching the other broker implementations.
	close(ttBroker.fills)

	return err
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
			return fmt.Errorf("tastytrade: order for %s: amount %.2f is less than the share price %.2f",
				order.Asset.Ticker, order.Amount, price)
		}
	}

	positions, posErr := ttBroker.client.getPositions(ctx)
	if posErr != nil {
		return fmt.Errorf("tastytrade: get positions for order action: %w", posErr)
	}

	order.Qty = qty
	ttOrder := toTastytradeOrder(order, detectAction(order.Side, order.Asset.Ticker, positions))

	_, err := ttBroker.client.submitOrder(ctx, ttOrder)
	if err != nil {
		return fmt.Errorf("tastytrade: submit order: %w", err)
	}

	return nil
}

func (ttBroker *TastytradeBroker) Cancel(ctx context.Context, orderID string) error {
	ttBroker.mu.Lock()
	complexID, isComplex := ttBroker.complexOrderIDs[orderID]
	ttBroker.mu.Unlock()

	if isComplex {
		return ttBroker.client.cancelComplexOrder(ctx, complexID)
	}

	return ttBroker.client.cancelOrder(ctx, orderID)
}

func (ttBroker *TastytradeBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	positions, posErr := ttBroker.client.getPositions(ctx)
	if posErr != nil {
		return fmt.Errorf("tastytrade: get positions for order action: %w", posErr)
	}

	ttOrder := toTastytradeOrder(order, detectAction(order.Side, order.Asset.Ticker, positions))

	return ttBroker.client.replaceOrder(ctx, orderID, ttOrder)
}

func (ttBroker *TastytradeBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	responses, err := ttBroker.client.getOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("tastytrade: get orders: %w", err)
	}

	ttBroker.mu.Lock()
	for _, resp := range responses {
		if resp.ComplexOrderID != "" {
			ttBroker.complexOrderIDs[resp.ID] = resp.ComplexOrderID
		}
	}
	ttBroker.mu.Unlock()

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

// Transactions returns transactions since the given time.
func (ttBroker *TastytradeBroker) Transactions(_ context.Context, _ time.Time) ([]broker.Transaction, error) {
	return nil, nil
}

// SubmitGroup submits a group of orders as a native complex order (OCO or OTOCO).
func (ttBroker *TastytradeBroker) SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error {
	if len(orders) == 0 {
		return ErrEmptyOrderGroup
	}

	ttBroker.mu.Lock()
	defer ttBroker.mu.Unlock()

	switch groupType {
	case broker.GroupOCO:
		return ttBroker.submitOCO(ctx, orders)
	case broker.GroupBracket:
		return ttBroker.submitOTOCO(ctx, orders)
	default:
		return fmt.Errorf("tastytrade: unsupported group type %d", groupType)
	}
}

func (ttBroker *TastytradeBroker) submitOCO(ctx context.Context, orders []broker.Order) error {
	positions, posErr := ttBroker.client.getPositions(ctx)
	if posErr != nil {
		return fmt.Errorf("tastytrade: get positions for order action: %w", posErr)
	}

	ttOrders := make([]orderRequest, len(orders))
	for idx, order := range orders {
		ttOrders[idx] = toTastytradeOrder(order, detectAction(order.Side, order.Asset.Ticker, positions))
	}

	req := complexOrderRequest{
		Type:   "OCO",
		Orders: ttOrders,
	}

	result, err := ttBroker.client.submitComplexOrder(ctx, req)
	if err != nil {
		return err
	}

	ttBroker.mapComplexOrderIDs(result)

	return nil
}

func (ttBroker *TastytradeBroker) submitOTOCO(ctx context.Context, orders []broker.Order) error {
	var (
		entrySide  broker.Side
		entryFound bool
	)

	for _, order := range orders {
		if order.GroupRole == broker.RoleEntry {
			if entryFound {
				return ErrMultipleEntryOrders
			}

			entrySide = order.Side
			entryFound = true
		}
	}

	if !entryFound {
		return ErrNoEntryOrder
	}

	positions, posErr := ttBroker.client.getPositions(ctx)
	if posErr != nil {
		return fmt.Errorf("tastytrade: get positions for order action: %w", posErr)
	}

	var (
		triggerOrder     *orderRequest
		contingentOrders []orderRequest
	)

	for _, order := range orders {
		var action string

		switch {
		case order.GroupRole == broker.RoleEntry:
			action = detectAction(order.Side, order.Asset.Ticker, positions)
		case entrySide == broker.Buy:
			// Contingent exits close the position the entry opens. That
			// position does not exist at submit time, so the action is the
			// closing complement of the entry rather than position-detected.
			action = "Sell to Close"
		default:
			action = "Buy to Close"
		}

		ttOrder := toTastytradeOrder(order, action)

		if order.GroupRole == broker.RoleEntry {
			triggerOrder = &ttOrder
		} else {
			contingentOrders = append(contingentOrders, ttOrder)
		}
	}

	if triggerOrder == nil {
		return ErrNoEntryOrder
	}

	req := complexOrderRequest{
		Type:         "OTOCO",
		TriggerOrder: triggerOrder,
		Orders:       contingentOrders,
	}

	result, err := ttBroker.client.submitComplexOrder(ctx, req)
	if err != nil {
		return err
	}

	ttBroker.mapComplexOrderIDs(result)

	return nil
}

func (ttBroker *TastytradeBroker) mapComplexOrderIDs(result complexOrderSubmitResponse) {
	complexID := result.Data.ComplexOrder.ID
	for _, childOrder := range result.Data.ComplexOrder.Orders {
		if childOrder.ID != "" {
			ttBroker.complexOrderIDs[childOrder.ID] = complexID
		}
	}
}
