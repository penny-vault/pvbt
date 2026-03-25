package schwab

import (
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
)

const (
	defaultCallbackURL = "https://127.0.0.1:5174"
	defaultTokenFile   = "~/.config/pvbt/schwab-tokens.json"
	authBaseURLDefault = "https://api.schwabapi.com"
	accessTokenBuffer  = 5 * time.Minute
	refreshInterval    = 25 * time.Minute
	refreshTokenTTL    = 7 * 24 * time.Hour
)

type tokenStore struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type tokenManager struct {
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
	testListener net.Listener // if non-nil, startAuthFlowServer uses this instead of creating one
}

func newTokenManager(clientID, clientSecret, callbackURL, tokenFile string) *tokenManager {
	if callbackURL == "" {
		callbackURL = defaultCallbackURL
	}

	if tokenFile == "" {
		tokenFile = expandHome(defaultTokenFile)
	}

	return &tokenManager{
		clientID:     clientID,
		clientSecret: clientSecret,
		callbackURL:  callbackURL,
		tokenFile:    tokenFile,
		authBaseURL:  authBaseURLDefault,
		tokens:       &tokenStore{},
		stopRefresh:  make(chan struct{}),
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr == nil {
			return filepath.Join(homeDir, path[2:])
		}
	}

	return path
}

func loadTokens(path string) (*tokenStore, error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, fmt.Errorf("load tokens: %w", readErr)
	}

	var store tokenStore
	if unmarshalErr := json.Unmarshal(data, &store); unmarshalErr != nil {
		return nil, fmt.Errorf("parse tokens: %w", unmarshalErr)
	}

	return &store, nil
}

func saveTokens(path string, store *tokenStore) error {
	parentDir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(parentDir, 0700); mkdirErr != nil {
		return fmt.Errorf("create token directory: %w", mkdirErr)
	}

	data, marshalErr := json.MarshalIndent(store, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshal tokens: %w", marshalErr)
	}

	if writeErr := os.WriteFile(path, data, 0600); writeErr != nil {
		return fmt.Errorf("write tokens: %w", writeErr)
	}

	return nil
}

func (manager *tokenManager) accessTokenExpired() bool {
	return time.Now().After(manager.tokens.AccessExpiresAt.Add(-accessTokenBuffer))
}

func (manager *tokenManager) refreshTokenExpired() bool {
	return time.Now().After(manager.tokens.RefreshExpiresAt)
}

func (manager *tokenManager) refreshAccessToken() error {
	client := resty.New()

	resp, reqErr := client.R().
		SetBasicAuth(manager.clientID, manager.clientSecret).
		SetFormData(map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": manager.tokens.RefreshToken,
		}).
		Post(manager.authBaseURL + "/v1/oauth/token")
	if reqErr != nil {
		return fmt.Errorf("refresh token request: %w", reqErr)
	}

	if resp.IsError() {
		return fmt.Errorf("refresh token: HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	var tokenResp schwabTokenResponse
	if unmarshalErr := json.Unmarshal(resp.Body(), &tokenResp); unmarshalErr != nil {
		return fmt.Errorf("parse token response: %w", unmarshalErr)
	}

	manager.tokens.AccessToken = tokenResp.AccessToken
	manager.tokens.RefreshToken = tokenResp.RefreshToken
	manager.tokens.AccessExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	manager.tokens.RefreshExpiresAt = time.Now().Add(refreshTokenTTL)

	return saveTokens(manager.tokenFile, manager.tokens)
}

func (manager *tokenManager) exchangeAuthCode(code string) error {
	client := resty.New()

	resp, reqErr := client.R().
		SetBasicAuth(manager.clientID, manager.clientSecret).
		SetFormData(map[string]string{
			"grant_type":   "authorization_code",
			"code":         code,
			"redirect_uri": manager.callbackURL,
		}).
		Post(manager.authBaseURL + "/v1/oauth/token")
	if reqErr != nil {
		return fmt.Errorf("exchange auth code request: %w", reqErr)
	}

	if resp.IsError() {
		return fmt.Errorf("exchange auth code: HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	var tokenResp schwabTokenResponse
	if unmarshalErr := json.Unmarshal(resp.Body(), &tokenResp); unmarshalErr != nil {
		return fmt.Errorf("parse token response: %w", unmarshalErr)
	}

	manager.tokens.AccessToken = tokenResp.AccessToken
	manager.tokens.RefreshToken = tokenResp.RefreshToken
	manager.tokens.AccessExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	manager.tokens.RefreshExpiresAt = time.Now().Add(refreshTokenTTL)

	return saveTokens(manager.tokenFile, manager.tokens)
}

func (manager *tokenManager) ensureValidToken() error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if !manager.accessTokenExpired() {
		return nil
	}

	if manager.refreshTokenExpired() {
		return ErrTokenExpired
	}

	return manager.refreshAccessToken()
}

func (manager *tokenManager) authorizationURL() string {
	return fmt.Sprintf(
		"%s/v1/oauth/authorize?client_id=%s&redirect_uri=%s",
		manager.authBaseURL,
		url.QueryEscape(manager.clientID),
		url.QueryEscape(manager.callbackURL),
	)
}

func (manager *tokenManager) startAuthFlowServer() (string, error) {
	parsedURL, parseErr := url.Parse(manager.callbackURL)
	if parseErr != nil {
		fallback, fallbackErr := url.Parse(defaultCallbackURL)
		if fallbackErr != nil {
			return "", fmt.Errorf("parse fallback callback URL: %w", fallbackErr)
		}

		parsedURL = fallback
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

	listener := manager.testListener

	if listener == nil {
		listenAddr := parsedURL.Host

		tlsCert, certErr := generateSelfSignedCert()
		if certErr != nil {
			return "", fmt.Errorf("generate TLS cert: %w", certErr)
		}

		created, listenErr := tls.Listen("tcp", listenAddr, &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
		})
		if listenErr != nil {
			return "", fmt.Errorf("listen on %s: %w", listenAddr, listenErr)
		}

		listener = created
	}

	actualAddr := listener.Addr().String()

	manager.callbackURL = fmt.Sprintf("https://%s", actualAddr)

	server := &http.Server{Handler: mux}

	go func() {
		serveErr := server.Serve(listener)
		if serveErr != nil && serveErr != http.ErrServerClosed {
			fmt.Printf("auth callback server error: %v\n", serveErr)
		}
	}()

	fmt.Printf("\nOpen the following URL in your browser to authorize:\n\n  %s\n\nWaiting for callback...\n", manager.authorizationURL())

	code := <-codeChan

	server.Close()

	if exchangeErr := manager.exchangeAuthCode(code); exchangeErr != nil {
		return actualAddr, exchangeErr
	}

	return actualAddr, nil
}

func (manager *tokenManager) startAuthFlow() error {
	_, flowErr := manager.startAuthFlowServer()
	return flowErr
}

func (manager *tokenManager) startBackgroundRefresh() {
	manager.refreshWg.Go(func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-manager.stopRefresh:
				return
			case <-ticker.C:
				manager.mu.Lock()
				if manager.accessTokenExpired() && !manager.refreshTokenExpired() {
					refreshErr := manager.refreshAccessToken()
					if refreshErr == nil && manager.onRefresh != nil {
						manager.onRefresh(manager.accessToken())
					}
				}
				manager.mu.Unlock()
			}
		}
	})
}

func (manager *tokenManager) stopBackgroundRefresh() {
	close(manager.stopRefresh)
	manager.refreshWg.Wait()
}

func (manager *tokenManager) accessToken() string {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	return manager.tokens.AccessToken
}

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
