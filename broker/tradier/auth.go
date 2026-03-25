package tradier

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
)

const (
	defaultCallbackURL   = "https://127.0.0.1:5174"
	defaultTokenFile     = "~/.config/pvbt/tradier-tokens.json"
	productionAuthURL    = "https://api.tradier.com"
	accessTokenBuffer    = 5 * time.Minute
	refreshCheckInterval = 30 * time.Minute
)

type authMode int

const (
	authModeStatic authMode = iota
	authModeOAuth
)

type tokenStore struct {
	AccessToken     string    `json:"access_token"`
	RefreshToken    string    `json:"refresh_token"`
	AccessExpiresAt time.Time `json:"access_expires_at"`
}

type tokenManager struct {
	mode         authMode
	staticToken  string
	clientID     string
	clientSecret string
	callbackURL  string
	tokenFile    string
	authBaseURL  string
	tokens       *tokenStore
	onRefresh    func(token string)
	mu           sync.Mutex
	stopRefresh  chan struct{}
	refreshWg    sync.WaitGroup
	stopOnce     sync.Once
}

// detectAuthMode inspects environment variables to determine how the broker
// should authenticate. TRADIER_ACCESS_TOKEN takes priority over OAuth env vars.
func detectAuthMode() (authMode, error) {
	if os.Getenv("TRADIER_ACCESS_TOKEN") != "" {
		return authModeStatic, nil
	}

	if os.Getenv("TRADIER_CLIENT_ID") != "" && os.Getenv("TRADIER_CLIENT_SECRET") != "" {
		return authModeOAuth, nil
	}

	return 0, ErrMissingCredentials
}

// newTokenManager constructs a tokenManager with sensible defaults applied.
func newTokenManager(mode authMode, clientID, clientSecret, callbackURL, tokenFile string) *tokenManager {
	if callbackURL == "" {
		callbackURL = defaultCallbackURL
	}

	if tokenFile == "" {
		tokenFile = expandHome(defaultTokenFile)
	}

	staticToken := ""
	if mode == authModeStatic {
		staticToken = os.Getenv("TRADIER_ACCESS_TOKEN")
	}

	return &tokenManager{
		mode:         mode,
		staticToken:  staticToken,
		clientID:     clientID,
		clientSecret: clientSecret,
		callbackURL:  callbackURL,
		tokenFile:    tokenFile,
		authBaseURL:  productionAuthURL,
		tokens:       &tokenStore{},
		stopRefresh:  make(chan struct{}),
	}
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr == nil {
			return filepath.Join(homeDir, path[2:])
		}
	}

	return path
}

// loadTokens reads a JSON token file and deserializes it into a tokenStore.
func loadTokens(path string) (*tokenStore, error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, fmt.Errorf("tradier: load tokens: %w", readErr)
	}

	var store tokenStore
	if unmarshalErr := json.Unmarshal(data, &store); unmarshalErr != nil {
		return nil, fmt.Errorf("tradier: parse tokens: %w", unmarshalErr)
	}

	return &store, nil
}

// saveTokens serializes a tokenStore to a JSON file with 0600 permissions,
// creating parent directories as needed.
func saveTokens(path string, store *tokenStore) error {
	parentDir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(parentDir, 0700); mkdirErr != nil {
		return fmt.Errorf("tradier: create token directory: %w", mkdirErr)
	}

	data, marshalErr := json.MarshalIndent(store, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("tradier: marshal tokens: %w", marshalErr)
	}

	if writeErr := os.WriteFile(path, data, 0600); writeErr != nil {
		return fmt.Errorf("tradier: write tokens: %w", writeErr)
	}

	return nil
}

// accessToken returns the current bearer token. For static mode it returns
// the static token; for OAuth mode it returns the stored access token.
func (tm *tokenManager) accessToken() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.mode == authModeStatic {
		return tm.staticToken
	}

	return tm.tokens.AccessToken
}

// accessTokenExpired reports whether the stored OAuth access token has expired
// (accounting for the buffer window).
func (tm *tokenManager) accessTokenExpired() bool {
	return time.Now().After(tm.tokens.AccessExpiresAt.Add(-accessTokenBuffer))
}

// ensureValidToken checks whether the current token is still valid. For static
// mode it always returns nil. For OAuth mode it attempts a refresh if the token
// is expired; returns ErrTokenExpired if refresh is not possible.
func (tm *tokenManager) ensureValidToken() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.mode == authModeStatic {
		return nil
	}

	if !tm.accessTokenExpired() {
		return nil
	}

	if tm.tokens.RefreshToken == "" {
		return ErrTokenExpired
	}

	return tm.refreshAccessToken()
}

// tradierTokenResponse is the JSON shape returned by Tradier token endpoints.
type tradierTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// exchangeAuthCode exchanges an OAuth authorization code for an access token.
// It POSTs to /v1/oauth/accesstoken with HTTP Basic auth.
func (tm *tokenManager) exchangeAuthCode(ctx context.Context, code string) error {
	client := resty.New()

	resp, reqErr := client.R().
		SetContext(ctx).
		SetBasicAuth(tm.clientID, tm.clientSecret).
		SetFormData(map[string]string{
			"grant_type": "authorization_code",
			"code":       code,
		}).
		Post(tm.authBaseURL + "/v1/oauth/accesstoken")
	if reqErr != nil {
		return fmt.Errorf("tradier: exchange auth code request: %w", reqErr)
	}

	if resp.IsError() {
		return fmt.Errorf("tradier: exchange auth code: HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	var tokenResp tradierTokenResponse
	if unmarshalErr := json.Unmarshal(resp.Body(), &tokenResp); unmarshalErr != nil {
		return fmt.Errorf("tradier: parse token response: %w", unmarshalErr)
	}

	tm.tokens.AccessToken = tokenResp.AccessToken
	tm.tokens.RefreshToken = tokenResp.RefreshToken
	tm.tokens.AccessExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return saveTokens(tm.tokenFile, tm.tokens)
}

// refreshAccessToken uses the stored refresh token to obtain a new access token.
// It POSTs to /v1/oauth/refreshtoken with HTTP Basic auth.
func (tm *tokenManager) refreshAccessToken() error {
	client := resty.New()

	resp, reqErr := client.R().
		SetBasicAuth(tm.clientID, tm.clientSecret).
		SetFormData(map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": tm.tokens.RefreshToken,
		}).
		Post(tm.authBaseURL + "/v1/oauth/refreshtoken")
	if reqErr != nil {
		return fmt.Errorf("tradier: refresh token request: %w", reqErr)
	}

	if resp.IsError() {
		return fmt.Errorf("tradier: refresh token: HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	var tokenResp tradierTokenResponse
	if unmarshalErr := json.Unmarshal(resp.Body(), &tokenResp); unmarshalErr != nil {
		return fmt.Errorf("tradier: parse token response: %w", unmarshalErr)
	}

	tm.tokens.AccessToken = tokenResp.AccessToken
	tm.tokens.RefreshToken = tokenResp.RefreshToken
	tm.tokens.AccessExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return saveTokens(tm.tokenFile, tm.tokens)
}

// authorizationURL builds the Tradier OAuth authorization URL.
func (tm *tokenManager) authorizationURL() string {
	return fmt.Sprintf(
		"%s/v1/oauth/authorize?client_id=%s&scope=read,write,trade,stream&state=pvbt",
		tm.authBaseURL,
		url.QueryEscape(tm.clientID),
	)
}

// startAuthFlow performs the browser-based OAuth flow: starts a local HTTPS
// callback server, prints the authorization URL, waits for the redirect, and
// exchanges the code for tokens.
func (tm *tokenManager) startAuthFlow() error {
	parsedURL, parseErr := url.Parse(tm.callbackURL)
	if parseErr != nil {
		fallback, fallbackErr := url.Parse(defaultCallbackURL)
		if fallbackErr != nil {
			return fmt.Errorf("tradier: parse fallback callback URL: %w", fallbackErr)
		}

		parsedURL = fallback
	}

	listenAddr := parsedURL.Host

	tlsCert, certErr := generateSelfSignedCert()
	if certErr != nil {
		return fmt.Errorf("tradier: generate TLS cert: %w", certErr)
	}

	codeChan := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, req *http.Request) {
		code := req.URL.Query().Get("code")

		decodedCode, decodeErr := url.QueryUnescape(code)
		if decodeErr != nil {
			decodedCode = code
		}

		writer.Header().Set("Content-Type", "text/html")
		fmt.Fprint(writer, "<html><body><h1>Authorization complete. You may close this window.</h1></body></html>")

		codeChan <- decodedCode
	})

	listener, listenErr := tls.Listen("tcp", listenAddr, &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
	if listenErr != nil {
		return fmt.Errorf("tradier: listen on %s: %w", listenAddr, listenErr)
	}

	actualAddr := listener.Addr().String()
	tm.callbackURL = fmt.Sprintf("https://%s", actualAddr)

	server := &http.Server{Handler: mux}

	go func() {
		serveErr := server.Serve(listener)
		if serveErr != nil && serveErr != http.ErrServerClosed {
			log.Error().Err(serveErr).Msg("tradier: auth callback server error")
		}
	}()

	log.Info().Str("url", tm.authorizationURL()).Msg("tradier: open the following URL in your browser to authorize")

	code := <-codeChan

	server.Close()

	return tm.exchangeAuthCode(context.Background(), code)
}

// startBackgroundRefresh launches a goroutine that proactively refreshes the
// OAuth access token before it expires.
func (tm *tokenManager) startBackgroundRefresh() {
	tm.refreshWg.Go(func() {
		ticker := time.NewTicker(refreshCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-tm.stopRefresh:
				return
			case <-ticker.C:
				tm.mu.Lock()
				if tm.accessTokenExpired() && tm.tokens.RefreshToken != "" {
					refreshErr := tm.refreshAccessToken()
					if refreshErr == nil && tm.onRefresh != nil {
						tm.onRefresh(tm.tokens.AccessToken)
					}
				}
				tm.mu.Unlock()
			}
		}
	})
}

// stopBackgroundRefresh signals the background refresh goroutine to stop and
// waits for it to exit.
func (tm *tokenManager) stopBackgroundRefresh() {
	tm.stopOnce.Do(func() {
		close(tm.stopRefresh)
	})
	tm.refreshWg.Wait()
}

// generateSelfSignedCert creates a self-signed ECDSA TLS certificate for the
// local OAuth callback server.
func generateSelfSignedCert() (tls.Certificate, error) {
	privateKey, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if keyErr != nil {
		return tls.Certificate{}, keyErr
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"pvbt"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, certErr := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if certErr != nil {
		return tls.Certificate{}, certErr
	}

	keyDER, marshalErr := x509.MarshalECPrivateKey(privateKey)
	if marshalErr != nil {
		return tls.Certificate{}, marshalErr
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}
