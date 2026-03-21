package tastytrade

import "context"

// APIClientForTestType is an exported alias so the _test package can name the type.
type APIClientForTestType = apiClient

// OrderRequest is an exported alias for orderRequest, used in tests.
type OrderRequest = orderRequest

// OrderLeg is an exported alias for orderLeg, used in tests.
type OrderLeg = orderLeg

// BalanceResponse is an exported alias for balanceResponse, used in tests.
type BalanceResponse = balanceResponse

// NewAPIClientForTest creates an apiClient pointing at a custom base URL.
func NewAPIClientForTest(baseURL string) *apiClient {
	return newAPIClient(baseURL)
}

// Authenticate exposes authenticate for testing.
func (c *apiClient) Authenticate(ctx context.Context, username, password string) error {
	return c.authenticate(ctx, username, password)
}

// SubmitOrder exposes submitOrder for testing.
func (c *apiClient) SubmitOrder(ctx context.Context, order orderRequest) (string, error) {
	return c.submitOrder(ctx, order)
}

// CancelOrder exposes cancelOrder for testing.
func (c *apiClient) CancelOrder(ctx context.Context, orderID string) error {
	return c.cancelOrder(ctx, orderID)
}

// ReplaceOrder exposes replaceOrder for testing.
func (c *apiClient) ReplaceOrder(ctx context.Context, orderID string, order orderRequest) error {
	return c.replaceOrder(ctx, orderID, order)
}

// GetOrders exposes getOrders for testing.
func (c *apiClient) GetOrders(ctx context.Context) ([]orderResponse, error) {
	return c.getOrders(ctx)
}

// GetPositions exposes getPositions for testing.
func (c *apiClient) GetPositions(ctx context.Context) ([]positionResponse, error) {
	return c.getPositions(ctx)
}

// GetBalance exposes getBalance for testing.
func (c *apiClient) GetBalance(ctx context.Context) (balanceResponse, error) {
	return c.getBalance(ctx)
}

// GetQuote exposes getQuote for testing.
func (c *apiClient) GetQuote(ctx context.Context, symbol string) (float64, error) {
	return c.getQuote(ctx, symbol)
}

// AccountID returns the client's account ID for test assertions.
func (c *apiClient) AccountID() string {
	return c.accountID
}
