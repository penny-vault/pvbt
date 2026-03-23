package ibkr

import (
	"context"
	"net/http"

	"github.com/penny-vault/pvbt/broker"
)

// Type aliases for test access.
type HTTPError = broker.HTTPError
type IBOrderRequest = ibOrderRequest
type IBOrderResponse = ibOrderResponse
type IBPositionEntry = ibPositionEntry
type IBAccountSummary = ibAccountSummary
type SummaryValue = summaryValue
type IBSecdefResult = ibSecdefResult
type IBOrderReply = ibOrderReply
type IBTradeEntry = ibTradeEntry

func ToIBOrder(order broker.Order, conid int64) (ibOrderRequest, error) {
	return toIBOrder(order, conid)
}

func ToBrokerOrder(resp ibOrderResponse) broker.Order {
	return toBrokerOrder(resp)
}

func ToBrokerPosition(pos ibPositionEntry) broker.Position {
	return toBrokerPosition(pos)
}

func ToBrokerBalance(summary ibAccountSummary) broker.Balance {
	return toBrokerBalance(summary)
}

// noopAuthenticator satisfies Authenticator for tests that do not need auth.
type noopAuthenticator struct{}

func (na *noopAuthenticator) Init(_ context.Context) error    { return nil }
func (na *noopAuthenticator) Decorate(_ *http.Request) error  { return nil }
func (na *noopAuthenticator) Keepalive(_ context.Context)     {}
func (na *noopAuthenticator) Close() error                    { return nil }

// NewAPIClientForTest creates an apiClient pointed at the given base URL
// with a no-op authenticator and retries disabled (so error tests are fast).
func NewAPIClientForTest(baseURL string) *apiClient {
	ac := newAPIClient(baseURL, &noopAuthenticator{})
	ac.resty.SetRetryCount(0)

	return ac
}

// Method wrappers to expose unexported client methods to tests.

func (ac *apiClient) ResolveAccount(ctx context.Context) (string, error) {
	return ac.resolveAccount(ctx)
}

func (ac *apiClient) SubmitOrder(ctx context.Context, accountID string, orders []ibOrderRequest) ([]ibOrderReply, error) {
	return ac.submitOrder(ctx, accountID, orders)
}

func (ac *apiClient) CancelOrder(ctx context.Context, accountID string, orderID string) error {
	return ac.cancelOrder(ctx, accountID, orderID)
}

func (ac *apiClient) ReplaceOrder(ctx context.Context, accountID string, orderID string, order ibOrderRequest) ([]ibOrderReply, error) {
	return ac.replaceOrder(ctx, accountID, orderID, order)
}

func (ac *apiClient) GetOrders(ctx context.Context) ([]ibOrderResponse, error) {
	return ac.getOrders(ctx)
}

func (ac *apiClient) GetPositions(ctx context.Context, accountID string) ([]ibPositionEntry, error) {
	return ac.getPositions(ctx, accountID)
}

func (ac *apiClient) GetBalance(ctx context.Context, accountID string) (ibAccountSummary, error) {
	return ac.getBalance(ctx, accountID)
}

func (ac *apiClient) SearchSecdef(ctx context.Context, symbol string) ([]ibSecdefResult, error) {
	return ac.searchSecdef(ctx, symbol)
}

func (ac *apiClient) GetSnapshot(ctx context.Context, conid int64) (float64, error) {
	return ac.getSnapshot(ctx, conid)
}

func (ac *apiClient) ConfirmReply(ctx context.Context, replyID string, confirmed bool) ([]ibOrderReply, error) {
	return ac.confirmReply(ctx, replyID, confirmed)
}

func (ac *apiClient) GetTrades(ctx context.Context) ([]ibTradeEntry, error) {
	return ac.getTrades(ctx)
}
