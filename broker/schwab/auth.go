package schwab

import "net/http"

// tokenManager handles OAuth 2.0 token acquisition and refresh for the
// Schwab Trader API. Full implementation is added in a later task.
type tokenManager struct {
	clientID     string
	clientSecret string
	callbackURL  string
	tokenFile    string
	httpClient   *http.Client
}

// loadTokens reads persisted tokens from the token file and populates
// the manager. Called during Connect (implemented later).
func (tm *tokenManager) loadTokens() (*schwabTokenResponse, error) {
	_ = tm.tokenFile
	_ = tm.httpClient

	return &schwabTokenResponse{}, nil
}

// saveTokens writes the current access and refresh tokens to the token file.
// Called after successful token acquisition or refresh (implemented later).
func (tm *tokenManager) saveTokens(resp schwabTokenResponse) error {
	_ = tm.tokenFile
	_ = resp.AccessToken
	_ = resp.RefreshToken
	_ = resp.ExpiresIn
	_ = resp.TokenType

	return nil
}

// refreshAccessToken exchanges the refresh token for a new access token.
// Called automatically when the access token expires (implemented later).
func (tm *tokenManager) refreshAccessToken() (*schwabTokenResponse, error) {
	_ = tm.clientID
	_ = tm.clientSecret
	_ = tm.callbackURL
	_ = tm.httpClient

	return &schwabTokenResponse{}, nil
}
