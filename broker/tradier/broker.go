package tradier

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"sync"

	"github.com/penny-vault/pvbt/broker"
)

// TradierBroker implements broker.Broker for the Tradier brokerage.
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

	accountID := os.Getenv("TRADIER_ACCOUNT_ID")
	if accountID == "" {
		return fmt.Errorf("tradier: connect: %w", ErrMissingCredentials)
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
		if closeErr := tb.streamer.close(); closeErr != nil {
			return closeErr
		}
	}

	close(tb.fills)

	return nil
}

// Submit sends an order to Tradier.
// If the order has Qty==0 and Amount>0, a quote is fetched first and the
// quantity is derived via floor(Amount / price). An error is returned if the
// resulting quantity is zero.
// Short-sell and buy-to-cover detection is performed by checking existing
// positions.
func (tb *TradierBroker) Submit(ctx context.Context, order broker.Order) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	return tb.submitLocked(ctx, order)
}

// submitLocked is the internal implementation of Submit. It must be called
// with tb.mu already held.
func (tb *TradierBroker) submitLocked(ctx context.Context, order broker.Order) error {
	// Handle dollar-amount orders.
	if order.Qty == 0 && order.Amount > 0 {
		price, quoteErr := tb.client.getQuote(ctx, order.Asset.Ticker)
		if quoteErr != nil {
			return fmt.Errorf("tradier: submit: get quote for dollar-amount order: %w", quoteErr)
		}

		qty := math.Floor(order.Amount / price)
		if qty == 0 {
			return fmt.Errorf("tradier: submit: dollar-amount order for %s results in zero shares at price %.4f", order.Asset.Ticker, price)
		}

		order.Qty = qty
	}

	params, paramsErr := toTradierOrderParams(order)
	if paramsErr != nil {
		return paramsErr
	}

	// Determine whether this is a short-sell or buy-to-cover.
	positions, posErr := tb.client.getPositions(ctx)
	if posErr != nil {
		return fmt.Errorf("tradier: submit: get positions for side detection: %w", posErr)
	}

	adjustedSide := detectSide(order.Side, order.Asset.Ticker, positions)
	params.Set("side", adjustedSide)

	orderID, submitErr := tb.client.submitOrder(ctx, params)
	if submitErr != nil {
		return fmt.Errorf("tradier: submit order: %w", submitErr)
	}

	// Tradier returns HTTP 200 even for downstream rejections. Verify the order
	// was not immediately rejected by querying its status.
	orders, ordersErr := tb.client.getOrders(ctx)
	if ordersErr != nil {
		return nil // order was placed; we just cannot verify status
	}

	for _, ord := range orders {
		if fmt.Sprintf("%d", ord.ID) == orderID && ord.Status == "rejected" {
			return fmt.Errorf("tradier: order %s rejected: %w", orderID, broker.ErrOrderRejected)
		}
	}

	return nil
}

// detectSide returns the Tradier-specific side string, accounting for short
// selling and buy-to-cover. It checks existing positions for the given ticker
// to decide whether a sell is a short sale and whether a buy is covering.
func detectSide(side broker.Side, ticker string, positions []tradierPositionResponse) string {
	var currentQty float64

	for _, pos := range positions {
		if pos.Symbol == ticker {
			currentQty = pos.Quantity
			break
		}
	}

	switch side {
	case broker.Sell:
		if currentQty <= 0 {
			// No long position (or already short): this is a short sale.
			return "sell_short"
		}

		return "sell"
	case broker.Buy:
		if currentQty < 0 {
			// Existing short position: this is a buy-to-cover.
			return "buy_to_cover"
		}

		return "buy"
	default:
		return "buy"
	}
}

// Cancel requests cancellation of an open order.
func (tb *TradierBroker) Cancel(ctx context.Context, orderID string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	return tb.client.cancelOrder(ctx, orderID)
}

// Replace modifies an existing order. If the new order changes the quantity,
// Tradier's modify endpoint cannot handle it, so the original is cancelled and
// a fresh order is submitted. For price/type/duration-only changes, the modify
// endpoint is used directly.
func (tb *TradierBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Fetch the current order to check whether the quantity is changing.
	existing, fetchErr := tb.findOrder(ctx, orderID)
	if fetchErr != nil {
		return fmt.Errorf("tradier: replace: look up order %s: %w", orderID, fetchErr)
	}

	if order.Qty != existing.Quantity {
		// Tradier cannot change quantity via modify; cancel and resubmit.
		if cancelErr := tb.client.cancelOrder(ctx, orderID); cancelErr != nil {
			return fmt.Errorf("tradier: replace: cancel original order: %w", cancelErr)
		}

		return tb.submitLocked(ctx, order)
	}

	params, paramsErr := toTradierOrderParams(order)
	if paramsErr != nil {
		return paramsErr
	}

	return tb.client.modifyOrder(ctx, orderID, params)
}

// findOrder fetches the order list and returns the order with the given ID.
// Must be called with tb.mu held.
func (tb *TradierBroker) findOrder(ctx context.Context, orderID string) (tradierOrderResponse, error) {
	orders, getErr := tb.client.getOrders(ctx)
	if getErr != nil {
		return tradierOrderResponse{}, getErr
	}

	for _, ord := range orders {
		if fmt.Sprintf("%d", ord.ID) == orderID {
			return ord, nil
		}
	}

	return tradierOrderResponse{}, fmt.Errorf("tradier: order %s not found", orderID)
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

// SubmitGroup submits a contingent order group to Tradier.
// GroupOCO maps to class=oco; GroupBracket maps to class=otoco.
func (tb *TradierBroker) SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error {
	if len(orders) == 0 {
		return ErrEmptyOrderGroup
	}

	switch groupType {
	case broker.GroupOCO:
		return tb.submitOCO(ctx, orders)
	case broker.GroupBracket:
		return tb.submitBracket(ctx, orders)
	default:
		return fmt.Errorf("tradier: submit group: unsupported group type %d", groupType)
	}
}

// submitOCO submits a one-cancels-other order group using Tradier's class=oco
// multi-leg form encoding.
func (tb *TradierBroker) submitOCO(ctx context.Context, orders []broker.Order) error {
	params := url.Values{}
	params.Set("class", "oco")

	for ii, order := range orders {
		duration, tifErr := mapTimeInForce(order.TimeInForce)
		if tifErr != nil {
			return tifErr
		}

		idx := strconv.Itoa(ii)
		params.Set("symbol["+idx+"]", order.Asset.Ticker)
		params.Set("side["+idx+"]", mapSide(order.Side))
		params.Set("quantity["+idx+"]", strconv.FormatFloat(order.Qty, 'f', -1, 64))
		params.Set("type["+idx+"]", mapOrderType(order.OrderType))
		params.Set("duration["+idx+"]", duration)

		if order.OrderType == broker.Limit || order.OrderType == broker.StopLimit {
			params.Set("price["+idx+"]", strconv.FormatFloat(order.LimitPrice, 'f', -1, 64))
		}

		if order.OrderType == broker.Stop || order.OrderType == broker.StopLimit {
			params.Set("stop["+idx+"]", strconv.FormatFloat(order.StopPrice, 'f', -1, 64))
		}
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	_, submitErr := tb.client.submitOrder(ctx, params)

	return submitErr
}

// submitBracket submits a bracket order group using Tradier's class=otoco
// multi-leg form encoding. The entry leg must be at index 0, take-profit at
// index 1, and stop-loss at index 2.
func (tb *TradierBroker) submitBracket(ctx context.Context, orders []broker.Order) error {
	// Validate: exactly one entry leg required.
	entryIdx := -1

	for ii, order := range orders {
		if order.GroupRole == broker.RoleEntry {
			if entryIdx != -1 {
				return ErrMultipleEntryOrders
			}

			entryIdx = ii
		}
	}

	if entryIdx == -1 {
		return ErrNoEntryOrder
	}

	// Build the legs in OTOCO order: entry, take-profit, stop-loss.
	legs := make([]broker.Order, 0, len(orders))
	legs = append(legs, orders[entryIdx])

	for _, order := range orders {
		if order.GroupRole == broker.RoleTakeProfit {
			legs = append(legs, order)
		}
	}

	for _, order := range orders {
		if order.GroupRole == broker.RoleStopLoss {
			legs = append(legs, order)
		}
	}

	// Append any remaining legs that have no assigned role (after the entry).
	for ii, order := range orders {
		if ii == entryIdx {
			continue
		}

		if order.GroupRole != broker.RoleTakeProfit && order.GroupRole != broker.RoleStopLoss {
			legs = append(legs, order)
		}
	}

	params := url.Values{}
	params.Set("class", "otoco")

	for ii, order := range legs {
		duration, tifErr := mapTimeInForce(order.TimeInForce)
		if tifErr != nil {
			return tifErr
		}

		idx := strconv.Itoa(ii)
		params.Set("symbol["+idx+"]", order.Asset.Ticker)
		params.Set("side["+idx+"]", mapSide(order.Side))
		params.Set("quantity["+idx+"]", strconv.FormatFloat(order.Qty, 'f', -1, 64))
		params.Set("type["+idx+"]", mapOrderType(order.OrderType))
		params.Set("duration["+idx+"]", duration)

		if order.OrderType == broker.Limit || order.OrderType == broker.StopLimit {
			params.Set("price["+idx+"]", strconv.FormatFloat(order.LimitPrice, 'f', -1, 64))
		}

		if order.OrderType == broker.Stop || order.OrderType == broker.StopLimit {
			params.Set("stop["+idx+"]", strconv.FormatFloat(order.StopPrice, 'f', -1, 64))
		}
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	_, submitErr := tb.client.submitOrder(ctx, params)

	return submitErr
}
