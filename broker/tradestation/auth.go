package tradestation

import "time"

// tokenManager handles OAuth2 token lifecycle for the TradeStation API.
// It persists tokens to disk and refreshes them transparently.
type tokenManager struct {
	clientID     string
	clientSecret string
	redirectURI  string
	tokenFile    string
	tokens       *tokenStore
}

// tokenStore holds the persisted OAuth2 tokens.
type tokenStore struct {
	AccessToken     string    `json:"access_token"`
	RefreshToken    string    `json:"refresh_token"`
	AccessExpiresAt time.Time `json:"access_expires_at"`
}

func newTokenManager(clientID, clientSecret, redirectURI, tokenFile string) *tokenManager {
	return &tokenManager{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		tokenFile:    tokenFile,
	}
}

func (tm *tokenManager) applyTokenResponse(resp tsTokenResponse) {
	tm.tokens = &tokenStore{
		AccessToken:     resp.AccessToken,
		RefreshToken:    resp.RefreshToken,
		AccessExpiresAt: time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
	}
}

func (tm *tokenManager) accessTokenExpired() bool {
	if tm.tokens == nil {
		return true
	}

	return time.Now().After(tm.tokens.AccessExpiresAt.Add(-5 * time.Minute))
}
