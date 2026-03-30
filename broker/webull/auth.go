package webull

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/bytedance/sonic"
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
	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

type authMode int

const (
	authModeDirect authMode = iota
	authModeOAuth
)

// signer attaches authentication credentials to an outbound HTTP request.
type signer interface {
	Sign(req *http.Request) error
}

// hmacSigner implements signer using HMAC-SHA1 per-request signing (Direct API).
type hmacSigner struct {
	appKey    string
	appSecret string
}

// Sign adds the required HMAC-SHA1 headers to the request.
func (hs *hmacSigner) Sign(req *http.Request) error {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	nonce := uuid.New().String()

	// The signature payload is: appKey + timestamp + nonce
	payload := hs.appKey + timestamp + nonce
	mac := hmac.New(sha1.New, []byte(hs.appSecret))

	if _, err := mac.Write([]byte(payload)); err != nil {
		return fmt.Errorf("webull: hmac write: %w", err)
	}

	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req.Header.Set("x-app-key", hs.appKey)
	req.Header.Set("x-timestamp", timestamp)
	req.Header.Set("x-signature", signature)
	req.Header.Set("x-signature-algorithm", "HmacSHA1")
	req.Header.Set("x-signature-version", "1.0")
	req.Header.Set("x-signature-nonce", nonce)

	return nil
}

// detectAuthMode inspects environment variables to determine how the broker
// should authenticate. WEBULL_APP_KEY takes priority over OAuth env vars.
func detectAuthMode() (authMode, error) {
	if os.Getenv("WEBULL_APP_KEY") != "" && os.Getenv("WEBULL_APP_SECRET") != "" {
		return authModeDirect, nil
	}

	if os.Getenv("WEBULL_CLIENT_ID") != "" && os.Getenv("WEBULL_CLIENT_SECRET") != "" {
		return authModeOAuth, nil
	}

	return 0, broker.ErrMissingCredentials
}

// ---------------------------------------------------------------------------
// OAuth 2.0 constants
// ---------------------------------------------------------------------------

const (
	defaultCallbackURL   = "https://127.0.0.1:5174"
	defaultTokenFile     = "~/.pvbt/webull_token.json"
	accessTokenBuffer    = 5 * time.Minute
	refreshCheckInterval = 25 * time.Minute
)

// ---------------------------------------------------------------------------
// OAuth 2.0 types
// ---------------------------------------------------------------------------

type tokenStore struct {
	AccessToken     string    `json:"access_token"`
	RefreshToken    string    `json:"refresh_token"`
	AccessExpiresAt time.Time `json:"access_expires_at"`
}

type tokenManager struct {
	mode         authMode
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

// oauthSigner implements signer using OAuth 2.0 bearer tokens.
type oauthSigner struct {
	tokenMgr *tokenManager
}

// Refresh attempts to obtain a new access token using the stored refresh token.
// This implements the refresher interface used by apiClient for 401/403 retry.
func (oa *oauthSigner) Refresh() error {
	return oa.tokenMgr.ensureValidToken()
}

// Close stops the background token refresh loop.
func (oa *oauthSigner) Close() {
	oa.tokenMgr.stopRefreshLoop()
}

// Sign adds an Authorization Bearer header to the request.
func (oa *oauthSigner) Sign(req *http.Request) error {
	if validErr := oa.tokenMgr.ensureValidToken(); validErr != nil {
		return validErr
	}

	oa.tokenMgr.mu.Lock()
	token := oa.tokenMgr.tokens.AccessToken
	oa.tokenMgr.mu.Unlock()

	if token == "" {
		return broker.ErrNotAuthenticated
	}

	req.Header.Set("Authorization", "Bearer "+token)

	return nil
}

// ensureValidToken checks whether the current token is still valid. If it is
// expired, it attempts a refresh using the stored refresh token.
func (tm *tokenManager) ensureValidToken() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !tm.accessTokenExpired() {
		return nil
	}

	if tm.tokens.RefreshToken == "" {
		return ErrTokenExpired
	}

	return tm.refreshAccessToken()
}

// ---------------------------------------------------------------------------
// tokenManager construction and helpers
// ---------------------------------------------------------------------------

// newTokenManager constructs a tokenManager with sensible defaults applied.
func newTokenManager(md authMode, clientID, clientSecret, callbackURL, tokenFile, authBaseURL string) *tokenManager {
	if callbackURL == "" {
		callbackURL = defaultCallbackURL
	}

	if tokenFile == "" {
		tokenFile = expandHome(defaultTokenFile)
	}

	if authBaseURL == "" {
		authBaseURL = productionAuthURL
	}

	return &tokenManager{
		mode:         md,
		clientID:     clientID,
		clientSecret: clientSecret,
		callbackURL:  callbackURL,
		tokenFile:    tokenFile,
		authBaseURL:  authBaseURL,
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

// loadTokens reads a JSON token file and deserializes it into the manager's
// token store. If the file does not exist, no error is returned.
func (tm *tokenManager) loadTokens() error {
	data, readErr := os.ReadFile(tm.tokenFile)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil
		}

		return fmt.Errorf("webull: load tokens: %w", readErr)
	}

	var store tokenStore
	if unmarshalErr := sonic.Unmarshal(data, &store); unmarshalErr != nil {
		return fmt.Errorf("webull: parse tokens: %w", unmarshalErr)
	}

	tm.tokens = &store

	return nil
}

// saveTokens serializes the token store to a JSON file with 0600 permissions,
// creating parent directories as needed.
func (tm *tokenManager) saveTokens() error {
	parentDir := filepath.Dir(tm.tokenFile)
	if mkdirErr := os.MkdirAll(parentDir, 0o700); mkdirErr != nil {
		return fmt.Errorf("webull: create token directory: %w", mkdirErr)
	}

	data, marshalErr := sonic.MarshalIndent(tm.tokens, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("webull: marshal tokens: %w", marshalErr)
	}

	if writeErr := os.WriteFile(tm.tokenFile, data, 0o600); writeErr != nil {
		return fmt.Errorf("webull: write tokens: %w", writeErr)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Token refresh
// ---------------------------------------------------------------------------

// webullTokenResponse is the JSON shape returned by Webull token endpoints.
type webullTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// refreshAccessToken uses the stored refresh token to obtain a new access token.
// It POSTs to /oauth-openapi/token with grant_type=refresh_token.
func (tm *tokenManager) refreshAccessToken() error {
	client := resty.New()

	resp, reqErr := client.R().
		SetFormData(map[string]string{
			"grant_type":    "refresh_token",
			"client_id":     tm.clientID,
			"client_secret": tm.clientSecret,
			"refresh_token": tm.tokens.RefreshToken,
		}).
		Post(tm.authBaseURL + "/oauth-openapi/token")
	if reqErr != nil {
		return fmt.Errorf("webull: refresh token request: %w", reqErr)
	}

	if resp.IsError() {
		return fmt.Errorf("webull: refresh token: HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	var tokenResp webullTokenResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &tokenResp); unmarshalErr != nil {
		return fmt.Errorf("webull: parse token response: %w", unmarshalErr)
	}

	tm.tokens.AccessToken = tokenResp.AccessToken
	tm.tokens.RefreshToken = tokenResp.RefreshToken
	tm.tokens.AccessExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return tm.saveTokens()
}

// startRefreshLoop launches a goroutine that proactively refreshes the OAuth
// access token before it expires.
func (tm *tokenManager) startRefreshLoop() {
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

					if refreshErr != nil {
						log.Error().Err(refreshErr).Msg("webull: background token refresh failed")
					}
				}
				tm.mu.Unlock()
			}
		}
	})
}

// stopRefreshLoop signals the background refresh goroutine to stop and waits
// for it to exit.
func (tm *tokenManager) stopRefreshLoop() {
	tm.stopOnce.Do(func() {
		close(tm.stopRefresh)
	})
	tm.refreshWg.Wait()
}

// accessTokenExpired reports whether the stored OAuth access token has expired
// (accounting for the buffer window).
func (tm *tokenManager) accessTokenExpired() bool {
	return time.Now().After(tm.tokens.AccessExpiresAt.Add(-accessTokenBuffer))
}

// ---------------------------------------------------------------------------
// Authorization code flow
// ---------------------------------------------------------------------------

// exchangeCode exchanges an OAuth authorization code for an access token.
// It POSTs to /oauth-openapi/token with grant_type=authorization_code.
func (tm *tokenManager) exchangeCode(ctx context.Context, code string) error {
	client := resty.New()

	resp, reqErr := client.R().
		SetContext(ctx).
		SetFormData(map[string]string{
			"grant_type":    "authorization_code",
			"client_id":     tm.clientID,
			"client_secret": tm.clientSecret,
			"code":          code,
			"redirect_uri":  tm.callbackURL,
		}).
		Post(tm.authBaseURL + "/oauth-openapi/token")
	if reqErr != nil {
		return fmt.Errorf("webull: exchange auth code request: %w", reqErr)
	}

	if resp.IsError() {
		return fmt.Errorf("webull: exchange auth code: HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	var tokenResp webullTokenResponse
	if unmarshalErr := sonic.Unmarshal(resp.Body(), &tokenResp); unmarshalErr != nil {
		return fmt.Errorf("webull: parse token response: %w", unmarshalErr)
	}

	tm.tokens.AccessToken = tokenResp.AccessToken
	tm.tokens.RefreshToken = tokenResp.RefreshToken
	tm.tokens.AccessExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return tm.saveTokens()
}

// authorizationURL builds the Webull OAuth authorization URL.
func (tm *tokenManager) authorizationURL() string {
	return fmt.Sprintf(
		"%s/oauth-openapi/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=trade",
		tm.authBaseURL,
		url.QueryEscape(tm.clientID),
		url.QueryEscape(tm.callbackURL),
	)
}

// authorize starts a local HTTPS server with a self-signed cert, prints the
// authorization URL, waits for the callback code, and exchanges it for tokens.
func (tm *tokenManager) authorize() error {
	parsedURL, parseErr := url.Parse(tm.callbackURL)
	if parseErr != nil {
		fallback, fallbackErr := url.Parse(defaultCallbackURL)
		if fallbackErr != nil {
			return fmt.Errorf("webull: parse fallback callback URL: %w", fallbackErr)
		}

		parsedURL = fallback
	}

	listenAddr := parsedURL.Host

	tlsCert, certErr := generateSelfSignedCert()
	if certErr != nil {
		return fmt.Errorf("webull: generate TLS cert: %w", certErr)
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
		return fmt.Errorf("webull: listen on %s: %w", listenAddr, listenErr)
	}

	actualAddr := listener.Addr().String()
	tm.callbackURL = fmt.Sprintf("https://%s", actualAddr)

	server := &http.Server{Handler: mux}

	go func() {
		serveErr := server.Serve(listener)
		if serveErr != nil && serveErr != http.ErrServerClosed {
			log.Error().Err(serveErr).Msg("webull: auth callback server error")
		}
	}()

	log.Info().Str("url", tm.authorizationURL()).Msg("webull: open the following URL in your browser to authorize")

	code := <-codeChan

	server.Close()

	return tm.exchangeCode(context.Background(), code)
}

// ---------------------------------------------------------------------------
// Self-signed TLS certificate
// ---------------------------------------------------------------------------

// generateSelfSignedCert creates a self-signed ECDSA TLS certificate for the
// local OAuth callback server. The cert is valid for 24 hours with a
// 127.0.0.1 SAN.
func generateSelfSignedCert() (tls.Certificate, error) {
	privateKey, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if keyErr != nil {
		return tls.Certificate{}, fmt.Errorf("webull: generate key: %w", keyErr)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"pvbt"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, certErr := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if certErr != nil {
		return tls.Certificate{}, fmt.Errorf("webull: create certificate: %w", certErr)
	}

	keyDER, marshalErr := x509.MarshalECPrivateKey(privateKey)
	if marshalErr != nil {
		return tls.Certificate{}, fmt.Errorf("webull: marshal key: %w", marshalErr)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}
