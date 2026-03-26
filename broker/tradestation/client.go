package tradestation

import "context"

// apiClient handles HTTP communication with the TradeStation REST API.
type apiClient struct {
	baseURL   string
	accountID string
}

func newAPIClient(baseURL, accountID string) *apiClient {
	return &apiClient{
		baseURL:   baseURL,
		accountID: accountID,
	}
}

func (cl *apiClient) getAccounts(_ context.Context) ([]tsAccountEntry, error) {
	return nil, nil
}

func (cl *apiClient) getOrders(_ context.Context) ([]tsOrderResponse, error) {
	return nil, nil
}

func (cl *apiClient) getPositions(_ context.Context) ([]tsPositionEntry, error) {
	return nil, nil
}

func (cl *apiClient) getBalance(_ context.Context) (tsBalanceResponse, error) {
	return tsBalanceResponse{}, nil
}

func (cl *apiClient) getQuote(_ context.Context, symbol string) (tsQuote, error) {
	_ = symbol

	return tsQuote{}, nil
}

func (cl *apiClient) getQuotes(_ context.Context, symbols []string) (tsQuoteResponse, error) {
	_ = symbols

	return tsQuoteResponse{}, nil
}
