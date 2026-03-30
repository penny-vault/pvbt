package etrade

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
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"github.com/bytedance/sonic"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	defaultEtradeTokenFile = "~/.config/pvbt/etrade-tokens.json"
	defaultEtradeAuthBase  = "https://api.etrade.com/oauth"
	defaultEtradeAuthorize = "https://us.etrade.com/e/t/etws/authorize"
	defaultCallbackAddr    = "127.0.0.1:5174"
	renewalInterval        = 90 * time.Minute
)

// oauthCredentials holds the four OAuth 1.0a credential values for E*TRADE.
// Fields are exported so the test package can access them via the OAuthCredentials
// type alias defined in exports_test.go.
type oauthCredentials struct {
	ConsumerKey    string `json:"consumer_key"`
	ConsumerSecret string `json:"consumer_secret"`
	AccessToken    string `json:"access_token"`
	AccessSecret   string `json:"access_secret"`
}

// tokenManager manages E*TRADE OAuth 1.0a tokens, including the initial auth
// flow, token renewal, and persistence to disk.
type tokenManager struct {
	creds         oauthCredentials
	callbackURL   string
	tokenFile     string
	authBaseURL   string
	authorizeURL  string
	mu            sync.Mutex
	cancelRefresh context.CancelFunc
	onRefresh     func(creds oauthCredentials)
}

// newTokenManager creates a tokenManager with sensible defaults.
func newTokenManager(consumerKey, consumerSecret, callbackURL, tokenFile string) *tokenManager {
	if tokenFile == "" {
		tokenFile = defaultEtradeTokenFile
	}

	return &tokenManager{
		creds: oauthCredentials{
			ConsumerKey:    consumerKey,
			ConsumerSecret: consumerSecret,
		},
		callbackURL:  callbackURL,
		tokenFile:    tokenFile,
		authBaseURL:  defaultEtradeAuthBase,
		authorizeURL: defaultEtradeAuthorize,
	}
}

// percentEncode applies RFC 5849 percent-encoding. Spaces become %20, * becomes
// %2A, and ~ is NOT encoded.
func percentEncode(ss string) string {
	encoded := url.QueryEscape(ss)
	// url.QueryEscape uses + for spaces; RFC 5849 requires %20.
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	// url.QueryEscape encodes ~ as %7E; RFC 5849 treats ~ as unreserved.
	encoded = strings.ReplaceAll(encoded, "%7E", "~")

	return encoded
}

// generateNonce returns a random 32-character lowercase hex string.
func generateNonce() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// Fall back to timestamp-based value if crypto/rand fails.
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}

	return hex.EncodeToString(buf)
}

// generateTimestamp returns the current Unix time as a decimal string.
func generateTimestamp() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}

// buildAuthHeader constructs an OAuth 1.0a Authorization header per RFC 5849.
func buildAuthHeader(
	method, rawURL, consumerKey, consumerSecret, token, tokenSecret, nonce, timestamp string,
	extraParams url.Values,
) string {
	// Parse the URL to extract base URL and any existing query parameters.
	parsed, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		log.Error().Err(parseErr).Str("url", rawURL).Msg("etrade: buildAuthHeader: failed to parse URL")
		return ""
	}

	// Collect all parameters: OAuth params + URL query params + extraParams.
	params := make([]string, 0, 16)

	addParam := func(key, val string) {
		params = append(params, percentEncode(key)+"="+percentEncode(val))
	}

	// Core OAuth params.
	addParam("oauth_consumer_key", consumerKey)
	addParam("oauth_nonce", nonce)
	addParam("oauth_signature_method", "HMAC-SHA1")
	addParam("oauth_timestamp", timestamp)

	if token != "" {
		addParam("oauth_token", token)
	}

	addParam("oauth_version", "1.0")

	// URL query parameters.
	for key, vals := range parsed.Query() {
		for _, val := range vals {
			addParam(key, val)
		}
	}

	// Extra parameters (e.g. oauth_verifier, oauth_callback).
	for key, vals := range extraParams {
		for _, val := range vals {
			addParam(key, val)
		}
	}

	// Sort lexicographically (key=value strings, already percent-encoded).
	sort.Strings(params)

	paramString := strings.Join(params, "&")

	// Build base URL: scheme + host + path, no query string.
	baseURL := fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, parsed.Path)

	// Signature base string per RFC 5849 §3.4.1.
	sigBase := strings.ToUpper(method) + "&" +
		percentEncode(baseURL) + "&" +
		percentEncode(paramString)

	// Signing key: percentEncode(consumerSecret)&percentEncode(tokenSecret).
	signingKey := percentEncode(consumerSecret) + "&" + percentEncode(tokenSecret)

	// Compute HMAC-SHA1 and base64-encode.
	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(sigBase))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// Build Authorization header. Include oauth_token only when non-empty.
	var hdr strings.Builder
	hdr.WriteString(`OAuth oauth_consumer_key="`)
	hdr.WriteString(percentEncode(consumerKey))
	hdr.WriteString(`", oauth_nonce="`)
	hdr.WriteString(percentEncode(nonce))
	hdr.WriteString(`", oauth_signature="`)
	hdr.WriteString(percentEncode(sig))
	hdr.WriteString(`", oauth_signature_method="HMAC-SHA1", oauth_timestamp="`)
	hdr.WriteString(timestamp)

	if token != "" {
		hdr.WriteString(`", oauth_token="`)
		hdr.WriteString(percentEncode(token))
	}

	hdr.WriteString(`", oauth_version="1.0"`)

	return hdr.String()
}

// requestToken obtains a request token from E*TRADE.
func (tm *tokenManager) requestToken() (string, string, error) {
	nonce := generateNonce()
	timestamp := generateTimestamp()

	extra := url.Values{"oauth_callback": []string{tm.callbackURL}}
	endpoint := tm.authBaseURL + "/request_token"

	authHdr := buildAuthHeader(
		"GET", endpoint,
		tm.creds.ConsumerKey, tm.creds.ConsumerSecret,
		"", "",
		nonce, timestamp,
		extra,
	)

	req, reqErr := http.NewRequest(http.MethodGet, endpoint, nil)
	if reqErr != nil {
		return "", "", fmt.Errorf("etrade: request token: build request: %w", reqErr)
	}

	req.Header.Set("Authorization", authHdr)

	resp, doErr := http.DefaultClient.Do(req)
	if doErr != nil {
		return "", "", fmt.Errorf("etrade: request token: %w", doErr)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", "", fmt.Errorf("etrade: request token: read response: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("etrade: request token: HTTP %d: %s", resp.StatusCode, string(body))
	}

	vals, parseErr := url.ParseQuery(string(body))
	if parseErr != nil {
		return "", "", fmt.Errorf("etrade: request token: parse response: %w", parseErr)
	}

	oauthToken := vals.Get("oauth_token")
	oauthSecret := vals.Get("oauth_token_secret")

	if oauthToken == "" {
		return "", "", fmt.Errorf("etrade: request token: missing oauth_token in response")
	}

	return oauthToken, oauthSecret, nil
}

// exchangeAccessToken exchanges a request token and verifier for an access token.
func (tm *tokenManager) exchangeAccessToken(requestToken, requestSecret, verifier string) error {
	nonce := generateNonce()
	timestamp := generateTimestamp()

	extra := url.Values{"oauth_verifier": []string{verifier}}
	endpoint := tm.authBaseURL + "/access_token"

	authHdr := buildAuthHeader(
		"GET", endpoint,
		tm.creds.ConsumerKey, tm.creds.ConsumerSecret,
		requestToken, requestSecret,
		nonce, timestamp,
		extra,
	)

	req, reqErr := http.NewRequest(http.MethodGet, endpoint, nil)
	if reqErr != nil {
		return fmt.Errorf("etrade: exchange access token: build request: %w", reqErr)
	}

	req.Header.Set("Authorization", authHdr)

	resp, doErr := http.DefaultClient.Do(req)
	if doErr != nil {
		return fmt.Errorf("etrade: exchange access token: %w", doErr)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("etrade: exchange access token: read response: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("etrade: exchange access token: HTTP %d: %s", resp.StatusCode, string(body))
	}

	vals, parseErr := url.ParseQuery(string(body))
	if parseErr != nil {
		return fmt.Errorf("etrade: exchange access token: parse response: %w", parseErr)
	}

	accessToken := vals.Get("oauth_token")
	accessSecret := vals.Get("oauth_token_secret")

	if accessToken == "" {
		return fmt.Errorf("etrade: exchange access token: missing oauth_token in response")
	}

	tm.mu.Lock()
	tm.creds.AccessToken = accessToken
	tm.creds.AccessSecret = accessSecret
	tm.mu.Unlock()

	return nil
}

// buildAuthorizeURL returns the URL the user must visit to authorize the app.
func (tm *tokenManager) buildAuthorizeURL(requestToken string) string {
	return fmt.Sprintf("%s?key=%s&token=%s",
		tm.authorizeURL,
		url.QueryEscape(tm.creds.ConsumerKey),
		url.QueryEscape(requestToken),
	)
}

// renewAccessToken calls the E*TRADE renew endpoint to keep the token active.
// E*TRADE tokens go inactive after 2 hours without activity.
func (tm *tokenManager) renewAccessToken() error {
	tm.mu.Lock()
	creds := tm.creds
	tm.mu.Unlock()

	nonce := generateNonce()
	timestamp := generateTimestamp()
	endpoint := tm.authBaseURL + "/renew_access_token"

	authHdr := buildAuthHeader(
		"GET", endpoint,
		creds.ConsumerKey, creds.ConsumerSecret,
		creds.AccessToken, creds.AccessSecret,
		nonce, timestamp,
		nil,
	)

	req, reqErr := http.NewRequest(http.MethodGet, endpoint, nil)
	if reqErr != nil {
		return fmt.Errorf("etrade: renew access token: build request: %w", reqErr)
	}

	req.Header.Set("Authorization", authHdr)

	resp, doErr := http.DefaultClient.Do(req)
	if doErr != nil {
		return fmt.Errorf("etrade: renew access token: %w", doErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readBodyErr := io.ReadAll(resp.Body)
		if readBodyErr != nil {
			return fmt.Errorf("etrade: renew access token: HTTP %d (read body: %w)", resp.StatusCode, readBodyErr)
		}

		return fmt.Errorf("etrade: renew access token: HTTP %d: %s", resp.StatusCode, string(body))
	}

	log.Info().Msg("etrade: access token renewed")

	return nil
}

// startBackgroundRenewal launches a goroutine that calls renewAccessToken every
// 90 minutes to prevent the token from going inactive.
func (tm *tokenManager) startBackgroundRenewal() {
	ctx, cancel := context.WithCancel(context.Background())

	tm.mu.Lock()
	tm.cancelRefresh = cancel
	tm.mu.Unlock()

	go func() {
		ticker := time.NewTicker(renewalInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if renewErr := tm.renewAccessToken(); renewErr != nil {
					log.Error().Err(renewErr).Msg("etrade: background token renewal failed")
					continue
				}

				tm.mu.Lock()
				if tm.onRefresh != nil {
					tm.onRefresh(tm.creds)
				}
				tm.mu.Unlock()
			}
		}
	}()
}

// stopBackgroundRenewal cancels the background renewal goroutine.
func (tm *tokenManager) stopBackgroundRenewal() {
	tm.mu.Lock()
	cancel := tm.cancelRefresh
	tm.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// startAuthFlow performs the interactive OAuth 1.0a flow:
//  1. Obtain a request token
//  2. Print the authorization URL to stdout
//  3. Start a local HTTPS callback server on 127.0.0.1:5174
//  4. Wait for the oauth_verifier from the callback
//  5. Exchange for an access token
//  6. Save tokens to disk
func (tm *tokenManager) startAuthFlow() error {
	requestToken, requestSecret, reqErr := tm.requestToken()
	if reqErr != nil {
		return fmt.Errorf("etrade: auth flow: %w", reqErr)
	}

	fmt.Printf("Open the following URL in your browser to authorize E*TRADE access:\n%s\n",
		tm.buildAuthorizeURL(requestToken))

	tlsCert, certErr := generateEtradeSelfSignedCert()
	if certErr != nil {
		return fmt.Errorf("etrade: auth flow: generate TLS cert: %w", certErr)
	}

	verifierChan := make(chan string, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(ww http.ResponseWriter, rr *http.Request) {
		verifier := rr.URL.Query().Get("oauth_verifier")

		ww.Header().Set("Content-Type", "text/html")
		fmt.Fprint(ww, "<html><body><h1>Authorization complete. You may close this window.</h1></body></html>")

		verifierChan <- verifier
	})

	listener, listenErr := tls.Listen("tcp", defaultCallbackAddr, &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
	if listenErr != nil {
		return fmt.Errorf("etrade: auth flow: listen on %s: %w", defaultCallbackAddr, listenErr)
	}

	server := &http.Server{Handler: mux}

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Error().Err(serveErr).Msg("etrade: auth callback server error")
		}
	}()

	verifier := <-verifierChan

	server.Close()

	if exchErr := tm.exchangeAccessToken(requestToken, requestSecret, verifier); exchErr != nil {
		return fmt.Errorf("etrade: auth flow: %w", exchErr)
	}

	tm.mu.Lock()
	creds := tm.creds
	tm.mu.Unlock()

	tokenPath := expandHome(tm.tokenFile)

	return saveTokens(tokenPath, &creds)
}

// saveTokens writes credentials to a JSON file at the given path with 0600
// permissions. Parent directories are created with 0700 permissions.
func saveTokens(path string, creds *oauthCredentials) error {
	expanded := expandHome(path)
	parentDir := filepath.Dir(expanded)

	if mkdirErr := os.MkdirAll(parentDir, 0700); mkdirErr != nil {
		return fmt.Errorf("etrade: save tokens: create directory: %w", mkdirErr)
	}

	data, marshalErr := sonic.MarshalIndent(creds, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("etrade: save tokens: marshal: %w", marshalErr)
	}

	if writeErr := os.WriteFile(expanded, data, 0600); writeErr != nil {
		return fmt.Errorf("etrade: save tokens: write: %w", writeErr)
	}

	return nil
}

// loadTokens reads and unmarshals credentials from a JSON file.
func loadTokens(path string) (*oauthCredentials, error) {
	expanded := expandHome(path)

	data, readErr := os.ReadFile(expanded)
	if readErr != nil {
		return nil, fmt.Errorf("etrade: load tokens: %w", readErr)
	}

	var creds oauthCredentials
	if unmarshalErr := sonic.Unmarshal(data, &creds); unmarshalErr != nil {
		return nil, fmt.Errorf("etrade: load tokens: parse: %w", unmarshalErr)
	}

	return &creds, nil
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}

	homeDir, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return path
	}

	if path == "~" {
		return homeDir
	}

	return filepath.Join(homeDir, path[1:])
}

// generateEtradeSelfSignedCert creates a self-signed ECDSA certificate for the
// local OAuth callback server.
func generateEtradeSelfSignedCert() (tls.Certificate, error) {
	privateKey, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if keyErr != nil {
		return tls.Certificate{}, fmt.Errorf("etrade: generate cert: key: %w", keyErr)
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
		return tls.Certificate{}, fmt.Errorf("etrade: generate cert: create: %w", certErr)
	}

	keyDER, marshalErr := x509.MarshalECPrivateKey(privateKey)
	if marshalErr != nil {
		return tls.Certificate{}, fmt.Errorf("etrade: generate cert: marshal key: %w", marshalErr)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}
