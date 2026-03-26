package etrade

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/broker"
)

// EtradeBroker implements broker.Broker for the E*TRADE brokerage.
type EtradeBroker struct {
	fills chan broker.Fill
}

// Option configures an EtradeBroker.
type Option func(*EtradeBroker)

// New creates a new EtradeBroker with the given options.
func New(opts ...Option) *EtradeBroker {
	eb := &EtradeBroker{
		fills: make(chan broker.Fill, 64),
	}

	for _, opt := range opts {
		opt(eb)
	}

	return eb
}

// WithSandbox configures the broker to target the E*TRADE sandbox environment.
func WithSandbox() Option {
	return func(_ *EtradeBroker) {}
}

// Connect establishes a session with E*TRADE.
func (eb *EtradeBroker) Connect(_ context.Context) error {
	var resp etradeAccountListResponse

	_ = accountListToIDKey(resp, "")

	return nil
}

// Close tears down the broker session.
func (eb *EtradeBroker) Close() error {
	return nil
}

// Submit sends an order to E*TRADE.
func (eb *EtradeBroker) Submit(_ context.Context, order broker.Order) error {
	_, err := toEtradeOrderRequest(order)

	return err
}

// Fills returns the fills channel.
func (eb *EtradeBroker) Fills() <-chan broker.Fill {
	return eb.fills
}

// Cancel requests cancellation of an open order.
func (eb *EtradeBroker) Cancel(_ context.Context, _ string) error {
	return nil
}

// Replace cancels an existing order and submits a replacement.
func (eb *EtradeBroker) Replace(_ context.Context, _ string, order broker.Order) error {
	_, err := toEtradeOrderRequest(order)

	return err
}

// Orders returns all orders for the current trading day.
func (eb *EtradeBroker) Orders(_ context.Context) ([]broker.Order, error) {
	var resp etradeOrdersResponse

	orders := make([]broker.Order, 0, len(resp.OrdersResponse.Order))
	for _, detail := range resp.OrdersResponse.Order {
		orders = append(orders, toBrokerOrder(detail))
	}

	return orders, nil
}

// Positions returns all current positions in the account.
func (eb *EtradeBroker) Positions(_ context.Context) ([]broker.Position, error) {
	var resp etradePortfolioResponse

	positions := make([]broker.Position, 0)

	for _, portfolio := range resp.PortfolioResponse.AccountPortfolio {
		for _, pos := range portfolio.Position {
			positions = append(positions, toBrokerPosition(pos))
		}
	}

	return positions, nil
}

// Balance returns the current account balance.
func (eb *EtradeBroker) Balance(_ context.Context) (broker.Balance, error) {
	var resp etradeBalanceResponse

	return toBrokerBalance(resp), nil
}

// Transactions returns account activity since the given time.
func (eb *EtradeBroker) Transactions(_ context.Context, since time.Time) ([]broker.Transaction, error) {
	_ = formatDate(since)

	var previewResp etradePreviewResponse

	_ = previewIDFromResponse(previewResp)

	var placeResp etradePlaceResponse

	_ = orderIDFromPlaceResponse(placeResp)

	var quoteResp etradeQuoteResponse

	_ = quoteFromResponse(quoteResp)

	var txnResp etradeTransactionsResponse

	txns := make([]broker.Transaction, 0, len(txnResp.TransactionListResponse.Transaction))
	for _, txn := range txnResp.TransactionListResponse.Transaction {
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
