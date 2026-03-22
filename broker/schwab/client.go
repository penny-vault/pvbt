package schwab

import "net/http"

const schwabBaseURL = "https://api.schwabapi.com/trader/v1"

// apiClient handles authenticated HTTP requests to the Schwab Trader API.
// Full implementation is added in a later task.
type apiClient struct {
	baseURL    string
	httpClient *http.Client
}

// getAccountNumbers retrieves the list of linked accounts and their hash
// values used in subsequent API calls. Called during Connect (implemented
// later).
func (ac *apiClient) getAccountNumbers() ([]schwabAccountNumberEntry, error) {
	return nil, nil
}

// getAccount retrieves account details including balances and positions.
// Called by GetBalance and GetPositions (implemented later).
func (ac *apiClient) getAccount(accountHash string) (schwabAccountResponse, error) {
	_ = accountHash

	return schwabAccountResponse{}, nil
}

// getQuote retrieves the latest quote for a symbol.
// Called during dollar-amount order conversion (implemented later).
func (ac *apiClient) getQuote(symbol string) (schwabQuoteResponse, error) {
	_ = symbol

	return schwabQuoteResponse{Quote: schwabQuote{LastPrice: 0}}, nil
}

// getUserPreferences retrieves the user preferences including streamer info.
// Called during streamer setup (implemented later).
func (ac *apiClient) getUserPreferences() (schwabUserPreference, error) {
	return schwabUserPreference{}, nil
}
