# Webull Broker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `broker.Broker` interface for Webull using their official OpenAPI platform, enabling live equity trading through Webull accounts.

**Architecture:** Webull uses two auth models: Direct API (HMAC-SHA1 per-request signing) and Connect API (OAuth 2.0 authorization code flow). A `Signer` interface abstracts both behind a common surface. The broker uses gRPC server-streaming for real-time fill delivery, with polling fallback on reconnect. No `GroupSubmitter` -- the account layer handles group cancellation. The package follows the same structure as existing brokers (Alpaca, Tradier, Schwab): separate files for auth, HTTP client, types, streaming, and the broker facade.

**Tech Stack:** Go, `go-resty/resty/v2` (HTTP client), `crypto/hmac` + `crypto/sha1` (HMAC-SHA1 signing), `google.golang.org/grpc` (gRPC streaming for trade events), Ginkgo/Gomega (tests), `net/http/httptest` (mock HTTP servers)

**Spec:** `docs/superpowers/specs/2026-03-25-webull-broker-design.md`

---

## Design Decisions

1. **Both auth models supported.** Direct API (HMAC-SHA1 signing) is the default for personal use. OAuth 2.0 is available for third-party integrations. Selection is automatic based on environment variables. The auth layer is abstracted behind a `Signer` interface so the client and broker code are auth-agnostic.

2. **gRPC streaming for fills.** Webull provides gRPC server-streaming at `events-api.webull.com` for order status changes. On reconnect, a single poll of the orders endpoint catches missed fills. Deduplication tracks cumulative filled quantity per order ID.

3. **No `GroupSubmitter`.** Combo order documentation is insufficient to implement reliably.

4. **`Transactions()` returns empty with an info-level log warning.** Webull has no transaction history endpoint.

5. **Replace is strict.** Webull only allows changing qty and price. Differences in side, TIF, or order type return an error.

6. **Dollar-amount orders require market type and `WithFractionalShares()`.** Non-market dollar-amount orders return an error.

## File Structure

| File | Responsibility |
|------|---------------|
| `broker/webull/doc.go` | Package documentation |
| `broker/webull/errors.go` | Webull-specific error sentinel and re-exported common broker errors |
| `broker/webull/auth.go` | `Signer` interface, `hmacSigner` (Direct API), `oauthSigner` (OAuth 2.0), `tokenManager` |
| `broker/webull/client.go` | HTTP client wrapping resty; typed methods for each endpoint |
| `broker/webull/types.go` | JSON request/response structs; bidirectional mapping to `broker.*` types |
| `broker/webull/broker.go` | `WebullBroker` struct implementing `broker.Broker`; constructor and options |
| `broker/webull/streamer.go` | gRPC event stream goroutine; reconnection; deduplication; poll fallback |
| `broker/webull/exports_test.go` | Test-only exports for internal types and functions |
| `broker/webull/webull_suite_test.go` | Ginkgo test suite wiring |
| `broker/webull/auth_test.go` | HMAC signing tests; OAuth token refresh tests |
| `broker/webull/client_test.go` | HTTP client endpoint tests using httptest |
| `broker/webull/types_test.go` | Type mapping roundtrip tests |
| `broker/webull/broker_test.go` | Broker-level integration tests |
| `broker/webull/streamer_test.go` | gRPC stream, reconnection, deduplication tests |

---

## Task 1: Package Scaffolding

**Files:**
- Create: `broker/webull/doc.go`
- Create: `broker/webull/errors.go`
- Create: `broker/webull/webull_suite_test.go`
- Create: `broker/webull/exports_test.go`

- [ ] **Step 1: Create `broker/webull/doc.go`**

```go
// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package webull implements broker.Broker for the Webull brokerage using the
// official Webull OpenAPI platform.
//
// # Authentication
//
// The broker supports two authentication models selected automatically by
// environment variables:
//
// Direct API (HMAC-SHA1): set WEBULL_APP_KEY and WEBULL_APP_SECRET. Every
// request is signed with HMAC-SHA1. No token refresh or browser auth needed.
//
// Connect API (OAuth 2.0): set WEBULL_CLIENT_ID and WEBULL_CLIENT_SECRET.
// Uses authorization code flow with browser-based consent. Access tokens are
// refreshed automatically. Optionally set WEBULL_CALLBACK_URL (default
// https://127.0.0.1:5174) and WEBULL_TOKEN_FILE (default
// ~/.pvbt/webull_token.json).
//
// If both are set, Direct API takes priority. Set WEBULL_ACCOUNT_ID to select
// an account; otherwise the first account is used.
//
// # UAT Environment
//
// Use WithUAT() to target Webull's UAT/test environment:
//
//	webullBroker := webull.New(webull.WithUAT())
//
// # Fractional Shares
//
// Use WithFractionalShares() to enable dollar-amount orders:
//
//	webullBroker := webull.New(webull.WithFractionalShares())
//
// # Fill Delivery
//
// Fills are delivered via a gRPC server-streaming connection to Webull's trade
// events endpoint. On disconnect, the broker reconnects with exponential
// backoff and polls for any fills missed during the outage.
package webull
```

- [ ] **Step 2: Create `broker/webull/errors.go`**

```go
package webull

import (
	"errors"

	"github.com/penny-vault/pvbt/broker"
)

var (
	// ErrUnsupportedTimeInForce is returned when the order uses a TIF that
	// Webull does not support (anything other than Day or GTC).
	ErrUnsupportedTimeInForce = errors.New("webull: unsupported time-in-force; only Day and GTC are supported")

	// ErrFractionalNotMarket is returned when a dollar-amount order uses a
	// non-market order type. Webull only supports fractional shares on market orders.
	ErrFractionalNotMarket = errors.New("webull: dollar-amount orders require market order type")

	// ErrFractionalNotEnabled is returned when a dollar-amount order is submitted
	// but WithFractionalShares() was not set.
	ErrFractionalNotEnabled = errors.New("webull: dollar-amount orders require WithFractionalShares option")

	// ErrReplaceFieldNotAllowed is returned when a replace order attempts to
	// change a field that Webull does not allow (side, TIF, order type).
	ErrReplaceFieldNotAllowed = errors.New("webull: replace may only change qty and price; side, time-in-force, and order type must match the original")
)

// Re-export common broker errors for use in tests.
var (
	ErrMissingCredentials = broker.ErrMissingCredentials
	ErrNotAuthenticated   = broker.ErrNotAuthenticated
	ErrAccountNotFound    = broker.ErrAccountNotFound
	ErrStreamDisconnected = broker.ErrStreamDisconnected
	ErrRateLimited        = broker.ErrRateLimited
	ErrOrderRejected      = broker.ErrOrderRejected
)
```

- [ ] **Step 3: Create `broker/webull/webull_suite_test.go`**

```go
package webull_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWebull(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webull Suite")
}
```

- [ ] **Step 4: Create `broker/webull/exports_test.go`** (initial skeleton -- expanded in later tasks)

```go
package webull

import (
	"github.com/penny-vault/pvbt/broker"
)

// HTTPError is a type alias for broker.HTTPError.
type HTTPError = broker.HTTPError
```

- [ ] **Step 5: Verify the package compiles**

Run: `go build ./broker/webull/...`
Expected: Success (no errors).

- [ ] **Step 6: Run the test suite to verify wiring**

Run: `ginkgo run ./broker/webull/...`
Expected: "Webull Suite" runs with 0 specs, 0 failures.

- [ ] **Step 7: Commit**

```bash
git add broker/webull/
git commit -m "feat(webull): add package scaffolding with doc, errors, and test suite (#18)"
```

---

## Task 2: HMAC-SHA1 Signing (Direct API Auth)

**Files:**
- Create: `broker/webull/auth.go`
- Create: `broker/webull/auth_test.go`
- Modify: `broker/webull/exports_test.go`

- [ ] **Step 1: Add test exports to `exports_test.go`**

Append to `broker/webull/exports_test.go`:

```go
package webull

import (
	"net/http"

	"github.com/penny-vault/pvbt/broker"
)

// HTTPError is a type alias for broker.HTTPError.
type HTTPError = broker.HTTPError

// SignerForTestType is an exported alias for the signer interface.
type SignerForTestType = signer

// NewHMACSigner creates an hmacSigner for testing.
func NewHMACSigner(appKey, appSecret string) signer {
	return &hmacSigner{appKey: appKey, appSecret: appSecret}
}

// ExtractSignatureHeaders reads the HMAC headers from an http.Request for test
// assertions.
func ExtractSignatureHeaders(req *http.Request) (appKey, timestamp, signature, algorithm, version, nonce string) {
	appKey = req.Header.Get("x-app-key")
	timestamp = req.Header.Get("x-timestamp")
	signature = req.Header.Get("x-signature")
	algorithm = req.Header.Get("x-signature-algorithm")
	version = req.Header.Get("x-signature-version")
	nonce = req.Header.Get("x-signature-nonce")
	return
}

// DetectAuthModeExport exposes detectAuthMode for testing.
func DetectAuthModeExport() (AuthModeExport, error) {
	mode, err := detectAuthMode()
	return AuthModeExport(mode), err
}

// AuthModeExport is an exported alias for authMode.
type AuthModeExport = authMode

// Auth mode constants for tests.
var (
	AuthModeDirect = authModeDirect
	AuthModeOAuth  = authModeOAuth
)
```

- [ ] **Step 2: Write failing tests in `broker/webull/auth_test.go`**

```go
package webull_test

import (
	"net/http"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/broker/webull"
)

var _ = Describe("Auth", func() {
	Describe("detectAuthMode", func() {
		AfterEach(func() {
			os.Unsetenv("WEBULL_APP_KEY")
			os.Unsetenv("WEBULL_APP_SECRET")
			os.Unsetenv("WEBULL_CLIENT_ID")
			os.Unsetenv("WEBULL_CLIENT_SECRET")
		})

		It("returns direct mode when WEBULL_APP_KEY is set", func() {
			os.Setenv("WEBULL_APP_KEY", "test-key")
			os.Setenv("WEBULL_APP_SECRET", "test-secret")
			mode, err := webull.DetectAuthModeExport()
			Expect(err).ToNot(HaveOccurred())
			Expect(mode).To(Equal(webull.AuthModeDirect))
		})

		It("returns oauth mode when WEBULL_CLIENT_ID is set", func() {
			os.Setenv("WEBULL_CLIENT_ID", "client-id")
			os.Setenv("WEBULL_CLIENT_SECRET", "client-secret")
			mode, err := webull.DetectAuthModeExport()
			Expect(err).ToNot(HaveOccurred())
			Expect(mode).To(Equal(webull.AuthModeOAuth))
		})

		It("prefers direct mode when both are set", func() {
			os.Setenv("WEBULL_APP_KEY", "test-key")
			os.Setenv("WEBULL_APP_SECRET", "test-secret")
			os.Setenv("WEBULL_CLIENT_ID", "client-id")
			os.Setenv("WEBULL_CLIENT_SECRET", "client-secret")
			mode, err := webull.DetectAuthModeExport()
			Expect(err).ToNot(HaveOccurred())
			Expect(mode).To(Equal(webull.AuthModeDirect))
		})

		It("returns error when neither is set", func() {
			_, err := webull.DetectAuthModeExport()
			Expect(err).To(MatchError(webull.ErrMissingCredentials))
		})
	})

	Describe("hmacSigner", func() {
		It("sets all required HMAC headers on a request", func() {
			sign := webull.NewHMACSigner("my-app-key", "my-secret")
			req, _ := http.NewRequest(http.MethodGet, "https://api.webull.com/v1/test", nil)
			err := sign.Sign(req)
			Expect(err).ToNot(HaveOccurred())

			appKey, timestamp, signature, algorithm, version, nonce := webull.ExtractSignatureHeaders(req)
			Expect(appKey).To(Equal("my-app-key"))
			Expect(timestamp).ToNot(BeEmpty())
			Expect(signature).ToNot(BeEmpty())
			Expect(algorithm).To(Equal("HmacSHA1"))
			Expect(version).To(Equal("1.0"))
			Expect(nonce).ToNot(BeEmpty())
		})

		It("produces different nonces on consecutive calls", func() {
			sign := webull.NewHMACSigner("key", "secret")
			req1, _ := http.NewRequest(http.MethodGet, "https://api.webull.com/v1/test", nil)
			req2, _ := http.NewRequest(http.MethodGet, "https://api.webull.com/v1/test", nil)
			Expect(sign.Sign(req1)).To(Succeed())
			Expect(sign.Sign(req2)).To(Succeed())

			_, _, _, _, _, nonce1 := webull.ExtractSignatureHeaders(req1)
			_, _, _, _, _, nonce2 := webull.ExtractSignatureHeaders(req2)
			Expect(nonce1).ToNot(Equal(nonce2))
		})

		It("computes a valid HMAC-SHA1 signature", func() {
			sign := webull.NewHMACSigner("key", "secret")
			req, _ := http.NewRequest(http.MethodGet, "https://api.webull.com/v1/test", nil)
			Expect(sign.Sign(req)).To(Succeed())

			_, _, signature, _, _, _ := webull.ExtractSignatureHeaders(req)
			// HMAC-SHA1 base64 is always 28 characters
			Expect(len(signature)).To(Equal(28))
		})
	})
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `ginkgo run ./broker/webull/...`
Expected: FAIL -- `detectAuthMode` and `hmacSigner` not defined.

- [ ] **Step 4: Implement `broker/webull/auth.go`**

```go
package webull

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/broker"
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `ginkgo run ./broker/webull/...`
Expected: All auth tests PASS.

- [ ] **Step 6: Run lint**

Run: `golangci-lint run ./broker/webull/...`
Expected: No lint errors.

- [ ] **Step 7: Commit**

```bash
git add broker/webull/auth.go broker/webull/auth_test.go broker/webull/exports_test.go
git commit -m "feat(webull): implement HMAC-SHA1 signing and auth mode detection (#18)"
```

---

## Task 3: OAuth 2.0 Authentication (Connect API)

**Files:**
- Modify: `broker/webull/auth.go`
- Modify: `broker/webull/auth_test.go`
- Modify: `broker/webull/exports_test.go`

This task adds the `oauthSigner` and `tokenManager` to `auth.go`, following the Tradier/Schwab pattern: browser-based authorization code flow, self-signed TLS cert for callback, background token refresh, and file-based persistence.

- [ ] **Step 1: Add OAuth test exports to `exports_test.go`**

Append to `broker/webull/exports_test.go`:

```go
// NewOAuthSignerForTest creates an oauthSigner with a pre-set access token.
func NewOAuthSignerForTest(accessToken string) signer {
	return &oauthSigner{
		tokenMgr: &tokenManager{
			tokens: &tokenStore{
				AccessToken: accessToken,
			},
		},
	}
}

// NewTokenManagerForTest creates a tokenManager pointing at a custom auth URL.
func NewTokenManagerForTest(clientID, clientSecret, callbackURL, tokenFile, authBaseURL string) *TokenManagerExport {
	return newTokenManager(authModeOAuth, clientID, clientSecret, callbackURL, tokenFile, authBaseURL)
}

// TokenManagerExport is an exported alias for tokenManager.
type TokenManagerExport = tokenManager

// LoadTokensExport exposes loadTokens for testing.
func (tm *TokenManagerExport) LoadTokensExport() error {
	return tm.loadTokens()
}

// SaveTokensExport exposes saveTokens for testing.
func (tm *TokenManagerExport) SaveTokensExport() error {
	return tm.saveTokens()
}

// AccessTokenExport returns the current access token for assertions.
func (tm *TokenManagerExport) AccessTokenExport() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.tokens == nil {
		return ""
	}
	return tm.tokens.AccessToken
}
```

- [ ] **Step 2: Write failing OAuth tests in `auth_test.go`**

Append to `broker/webull/auth_test.go`:

```go
	Describe("oauthSigner", func() {
		It("sets the Authorization Bearer header", func() {
			sign := webull.NewOAuthSignerForTest("test-access-token")
			req, _ := http.NewRequest(http.MethodGet, "https://api.webull.com/v1/test", nil)
			Expect(sign.Sign(req)).To(Succeed())
			Expect(req.Header.Get("Authorization")).To(Equal("Bearer test-access-token"))
		})
	})

	Describe("tokenManager", func() {
		var tokenFile string

		BeforeEach(func() {
			tmpFile, err := os.CreateTemp("", "webull-token-*.json")
			Expect(err).ToNot(HaveOccurred())
			tokenFile = tmpFile.Name()
			tmpFile.Close()
		})

		AfterEach(func() {
			os.Remove(tokenFile)
		})

		It("saves and loads tokens from disk", func() {
			tm := webull.NewTokenManagerForTest("cid", "csecret", "https://127.0.0.1:5174", tokenFile, "https://example.com")
			// Set a token manually via the export, then save.
			tm.SetTokensForTest("access-123", "refresh-456")
			Expect(tm.SaveTokensExport()).To(Succeed())

			// Create a new manager pointing at the same file and load.
			tm2 := webull.NewTokenManagerForTest("cid", "csecret", "https://127.0.0.1:5174", tokenFile, "https://example.com")
			Expect(tm2.LoadTokensExport()).To(Succeed())
			Expect(tm2.AccessTokenExport()).To(Equal("access-123"))
		})
	})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `ginkgo run ./broker/webull/...`
Expected: FAIL -- `oauthSigner`, `tokenManager`, `SetTokensForTest` not defined.

- [ ] **Step 4: Implement OAuth types and oauthSigner in `auth.go`**

Append to `broker/webull/auth.go`:

```go
const (
	defaultCallbackURL   = "https://127.0.0.1:5174"
	defaultTokenFile     = "~/.pvbt/webull_token.json"
	accessTokenBuffer    = 5 * time.Minute
	refreshCheckInterval = 25 * time.Minute
)

// tokenStore holds the persisted OAuth tokens.
type tokenStore struct {
	AccessToken     string    `json:"access_token"`
	RefreshToken    string    `json:"refresh_token"`
	AccessExpiresAt time.Time `json:"access_expires_at"`
}

// tokenManager handles OAuth 2.0 token lifecycle: initial auth, refresh,
// and persistence.
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

// oauthSigner implements signer using OAuth 2.0 Bearer tokens.
type oauthSigner struct {
	tokenMgr *tokenManager
}

// Sign sets the Authorization Bearer header from the current access token.
func (os *oauthSigner) Sign(req *http.Request) error {
	os.tokenMgr.mu.Lock()
	token := os.tokenMgr.tokens.AccessToken
	os.tokenMgr.mu.Unlock()

	if token == "" {
		return broker.ErrNotAuthenticated
	}

	req.Header.Set("Authorization", "Bearer "+token)

	return nil
}

// newTokenManager constructs a tokenManager with defaults applied.
func newTokenManager(mode authMode, clientID, clientSecret, callbackURL, tokenFile, authBaseURL string) *tokenManager {
	if callbackURL == "" {
		callbackURL = defaultCallbackURL
	}

	if tokenFile == "" {
		tokenFile = expandHome(defaultTokenFile)
	}

	return &tokenManager{
		mode:         mode,
		clientID:     clientID,
		clientSecret: clientSecret,
		callbackURL:  callbackURL,
		tokenFile:    tokenFile,
		authBaseURL:  authBaseURL,
		stopRefresh:  make(chan struct{}),
	}
}

// loadTokens reads the token file from disk.
func (tm *tokenManager) loadTokens() error {
	data, err := os.ReadFile(tm.tokenFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("webull: read token file: %w", err)
	}

	var store tokenStore
	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("webull: parse token file: %w", err)
	}

	tm.mu.Lock()
	tm.tokens = &store
	tm.mu.Unlock()

	return nil
}

// saveTokens writes the current tokens to disk.
func (tm *tokenManager) saveTokens() error {
	tm.mu.Lock()
	store := tm.tokens
	tm.mu.Unlock()

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("webull: marshal tokens: %w", err)
	}

	dir := filepath.Dir(tm.tokenFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("webull: create token directory: %w", err)
	}

	if err := os.WriteFile(tm.tokenFile, data, 0o600); err != nil {
		return fmt.Errorf("webull: write token file: %w", err)
	}

	return nil
}

// startRefreshLoop launches a background goroutine that refreshes the access
// token before it expires.
func (tm *tokenManager) startRefreshLoop() {
	tm.refreshWg.Add(1)

	go func() {
		defer tm.refreshWg.Done()

		ticker := time.NewTicker(refreshCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-tm.stopRefresh:
				return
			case <-ticker.C:
				tm.mu.Lock()
				needsRefresh := tm.tokens != nil &&
					time.Until(tm.tokens.AccessExpiresAt) < accessTokenBuffer
				tm.mu.Unlock()

				if needsRefresh {
					if err := tm.refreshAccessToken(); err != nil {
						log.Error().Err(err).Msg("webull: failed to refresh access token")
					}
				}
			}
		}
	}()
}

// stopRefreshLoop signals the background refresh goroutine to exit and waits.
func (tm *tokenManager) stopRefreshLoop() {
	tm.stopOnce.Do(func() {
		close(tm.stopRefresh)
	})

	tm.refreshWg.Wait()
}

// refreshAccessToken exchanges the refresh token for new access + refresh tokens.
func (tm *tokenManager) refreshAccessToken() error {
	tm.mu.Lock()
	refreshToken := tm.tokens.RefreshToken
	tm.mu.Unlock()

	client := resty.New().SetBaseURL(tm.authBaseURL)

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}

	resp, err := client.R().
		SetFormData(map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": refreshToken,
			"client_id":     tm.clientID,
			"client_secret": tm.clientSecret,
		}).
		SetResult(&result).
		Post("/oauth-openapi/token")
	if err != nil {
		return fmt.Errorf("webull: refresh token request: %w", err)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	tm.mu.Lock()
	tm.tokens = &tokenStore{
		AccessToken:     result.AccessToken,
		RefreshToken:    result.RefreshToken,
		AccessExpiresAt: time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}
	tm.mu.Unlock()

	if err := tm.saveTokens(); err != nil {
		return err
	}

	if tm.onRefresh != nil {
		tm.onRefresh(result.AccessToken)
	}

	return nil
}

// authorize performs the initial OAuth 2.0 authorization code flow:
// 1. Opens the user's browser to Webull's authorization URL
// 2. Starts a local HTTPS server to receive the callback
// 3. Exchanges the authorization code for access + refresh tokens
func (tm *tokenManager) authorize(ctx context.Context) error {
	callbackURL, parseErr := url.Parse(tm.callbackURL)
	if parseErr != nil {
		return fmt.Errorf("webull: parse callback URL: %w", parseErr)
	}

	// Generate self-signed TLS cert for the callback server.
	tlsCert, certErr := generateSelfSignedCert()
	if certErr != nil {
		return fmt.Errorf("webull: generate callback cert: %w", certErr)
	}

	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		code := req.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("webull: callback missing authorization code")
			return
		}

		fmt.Fprintf(rw, "Authorization successful. You may close this window.")
		codeChan <- code
	})

	server := &http.Server{
		Addr:    callbackURL.Host,
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			MinVersion:   tls.VersionTLS12,
		},
	}

	go func() {
		if listenErr := server.ListenAndServeTLS("", ""); listenErr != http.ErrServerClosed {
			errChan <- fmt.Errorf("webull: callback server: %w", listenErr)
		}
	}()

	defer server.Shutdown(ctx)

	// Build the authorization URL and instruct the user to open it.
	authURL := fmt.Sprintf("%s/oauth-openapi/authorize?client_id=%s&redirect_uri=%s&response_type=code",
		tm.authBaseURL,
		url.QueryEscape(tm.clientID),
		url.QueryEscape(tm.callbackURL),
	)

	log.Info().Str("url", authURL).Msg("webull: open this URL in your browser to authorize")

	// Wait for the authorization code.
	var code string

	select {
	case code = <-codeChan:
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}

	// Exchange the authorization code for tokens.
	return tm.exchangeCode(code)
}

// exchangeCode exchanges an authorization code for access + refresh tokens.
func (tm *tokenManager) exchangeCode(code string) error {
	client := resty.New().SetBaseURL(tm.authBaseURL)

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}

	resp, err := client.R().
		SetFormData(map[string]string{
			"grant_type":    "authorization_code",
			"code":          code,
			"client_id":     tm.clientID,
			"client_secret": tm.clientSecret,
			"redirect_uri":  tm.callbackURL,
		}).
		SetResult(&result).
		Post("/oauth-openapi/token")
	if err != nil {
		return fmt.Errorf("webull: exchange code: %w", err)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	tm.mu.Lock()
	tm.tokens = &tokenStore{
		AccessToken:     result.AccessToken,
		RefreshToken:    result.RefreshToken,
		AccessExpiresAt: time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}
	tm.mu.Unlock()

	return tm.saveTokens()
}

// generateSelfSignedCert creates a self-signed TLS certificate for the
// local callback server.
func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate key: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"pvbt"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if len(path) < 2 || path[:2] != "~/" {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	return filepath.Join(home, path[2:])
}
```

Add these imports to the top of `auth.go` (replace existing import block):

```go
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
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)
```

Add `SetTokensForTest` to `exports_test.go`:

```go
// SetTokensForTest sets tokens directly for testing.
func (tm *TokenManagerExport) SetTokensForTest(access, refresh string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tokens = &tokenStore{
		AccessToken:     access,
		RefreshToken:    refresh,
		AccessExpiresAt: time.Now().Add(30 * time.Minute),
	}
}
```

Also add `"time"` to the imports in `exports_test.go`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `ginkgo run ./broker/webull/...`
Expected: All auth tests PASS.

- [ ] **Step 6: Run lint**

Run: `golangci-lint run ./broker/webull/...`
Expected: No lint errors.

- [ ] **Step 7: Commit**

```bash
git add broker/webull/auth.go broker/webull/auth_test.go broker/webull/exports_test.go
git commit -m "feat(webull): implement OAuth 2.0 auth with token manager (#18)"
```

---

## Task 4: Types and Mapping Functions

**Files:**
- Create: `broker/webull/types.go`
- Create: `broker/webull/types_test.go`
- Modify: `broker/webull/exports_test.go`

- [ ] **Step 1: Add type exports to `exports_test.go`**

Append to `broker/webull/exports_test.go`:

```go
// OrderRequestExport is an exported alias for orderRequest.
type OrderRequestExport = orderRequest

// OrderResponseExport is an exported alias for orderResponse.
type OrderResponseExport = orderResponse

// ReplaceRequestExport is an exported alias for replaceRequest.
type ReplaceRequestExport = replaceRequest

// AccountResponseExport is an exported alias for accountResponse.
type AccountResponseExport = accountResponse

// AccountListResponseExport is an exported alias for accountListResponse.
type AccountListResponseExport = accountListResponse

// PositionResponseExport is an exported alias for positionResponse.
type PositionResponseExport = positionResponse

// ToWebullOrder exposes toWebullOrder for testing.
func ToWebullOrder(order broker.Order, fractional bool) orderRequest {
	return toWebullOrder(order, fractional)
}

// ToBrokerOrder exposes toBrokerOrder for testing.
func ToBrokerOrder(resp orderResponse) broker.Order {
	return toBrokerOrder(resp)
}

// ToBrokerPosition exposes toBrokerPosition for testing.
func ToBrokerPosition(resp positionResponse) broker.Position {
	return toBrokerPosition(resp)
}

// ToBrokerBalance exposes toBrokerBalance for testing.
func ToBrokerBalance(resp accountResponse) broker.Balance {
	return toBrokerBalance(resp)
}
```

- [ ] **Step 2: Write failing tests in `broker/webull/types_test.go`**

```go
package webull_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/webull"
)

var _ = Describe("Types", func() {
	Describe("toWebullOrder", func() {
		It("maps a market buy order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			req := webull.ToWebullOrder(order, false)
			Expect(req.Symbol).To(Equal("AAPL"))
			Expect(req.Side).To(Equal("BUY"))
			Expect(req.OrderType).To(Equal("MARKET"))
			Expect(req.TimeInForce).To(Equal("DAY"))
			Expect(req.Qty).To(Equal("10"))
		})

		It("maps a limit sell order with GTC", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Sell,
				Qty:         5,
				OrderType:   broker.Limit,
				LimitPrice:  350.50,
				TimeInForce: broker.GTC,
			}

			req := webull.ToWebullOrder(order, false)
			Expect(req.Side).To(Equal("SELL"))
			Expect(req.OrderType).To(Equal("LIMIT"))
			Expect(req.TimeInForce).To(Equal("GTC"))
			Expect(req.LimitPrice).To(Equal("350.5"))
		})

		It("maps a stop-limit order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "GOOG"},
				Side:        broker.Sell,
				Qty:         20,
				OrderType:   broker.StopLimit,
				LimitPrice:  140,
				StopPrice:   138,
				TimeInForce: broker.Day,
			}

			req := webull.ToWebullOrder(order, false)
			Expect(req.OrderType).To(Equal("STOP_LOSS_LIMIT"))
			Expect(req.LimitPrice).To(Equal("140"))
			Expect(req.StopPrice).To(Equal("138"))
		})

		It("uses notional for fractional dollar-amount orders", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Amount:      500,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			req := webull.ToWebullOrder(order, true)
			Expect(req.Qty).To(BeEmpty())
			Expect(req.Notional).To(Equal("500"))
		})

		It("uses qty when fractional is false even with Amount set", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				Amount:      500,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			req := webull.ToWebullOrder(order, false)
			Expect(req.Qty).To(Equal("10"))
			Expect(req.Notional).To(BeEmpty())
		})
	})

	Describe("toBrokerOrder", func() {
		It("maps a Webull order response to broker.Order", func() {
			resp := webull.OrderResponseExport{
				ID:         "order-123",
				Symbol:     "AAPL",
				Side:       "BUY",
				Status:     "FILLED",
				OrderType:  "LIMIT",
				Qty:        "10",
				FilledQty:  "10",
				LimitPrice: "150.25",
			}

			order := webull.ToBrokerOrder(resp)
			Expect(order.ID).To(Equal("order-123"))
			Expect(order.Asset.Ticker).To(Equal("AAPL"))
			Expect(order.Side).To(Equal(broker.Buy))
			Expect(order.Status).To(Equal(broker.OrderFilled))
			Expect(order.OrderType).To(Equal(broker.Limit))
			Expect(order.Qty).To(Equal(10.0))
			Expect(order.LimitPrice).To(Equal(150.25))
		})
	})

	Describe("toBrokerPosition", func() {
		It("maps a Webull position response to broker.Position", func() {
			resp := webull.PositionResponseExport{
				Symbol:       "MSFT",
				Qty:          "25",
				AvgCost:      "320.50",
				MarketValue:  "8500",
				UnrealizedPL: "487.50",
			}

			pos := webull.ToBrokerPosition(resp)
			Expect(pos.Asset.Ticker).To(Equal("MSFT"))
			Expect(pos.Qty).To(Equal(25.0))
			Expect(pos.AvgOpenPrice).To(Equal(320.50))
		})
	})

	Describe("toBrokerBalance", func() {
		It("maps a Webull account response to broker.Balance", func() {
			resp := webull.AccountResponseExport{
				NetLiquidation: "100000.50",
				CashBalance:    "25000.00",
				BuyingPower:    "50000.00",
				MaintenanceReq: "15000.00",
			}

			bal := webull.ToBrokerBalance(resp)
			Expect(bal.NetLiquidatingValue).To(Equal(100000.50))
			Expect(bal.CashBalance).To(Equal(25000.00))
			Expect(bal.EquityBuyingPower).To(Equal(50000.00))
			Expect(bal.MaintenanceReq).To(Equal(15000.00))
		})
	})
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `ginkgo run ./broker/webull/...`
Expected: FAIL -- types not defined.

- [ ] **Step 4: Implement `broker/webull/types.go`**

```go
package webull

import (
	"strconv"

	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// --- Request types ---

type orderRequest struct {
	Symbol      string `json:"instrument_id,omitempty"`
	Side        string `json:"side"`
	OrderType   string `json:"order_type"`
	TimeInForce string `json:"time_in_force"`
	Qty         string `json:"qty,omitempty"`
	Notional    string `json:"notional,omitempty"`
	LimitPrice  string `json:"limit_price,omitempty"`
	StopPrice   string `json:"stop_price,omitempty"`
	ClientID    string `json:"client_order_id"`
}

type replaceRequest struct {
	Qty        string `json:"qty,omitempty"`
	LimitPrice string `json:"limit_price,omitempty"`
	StopPrice  string `json:"stop_price,omitempty"`
}

// --- Response types ---

type orderResponse struct {
	ID         string `json:"order_id"`
	Symbol     string `json:"symbol"`
	Side       string `json:"side"`
	Status     string `json:"order_status"`
	OrderType  string `json:"order_type"`
	Qty        string `json:"qty"`
	FilledQty  string `json:"filled_qty"`
	FilledPrice string `json:"filled_avg_price"`
	LimitPrice string `json:"limit_price"`
	StopPrice  string `json:"stop_price"`
}

type positionResponse struct {
	Symbol       string `json:"symbol"`
	Qty          string `json:"qty"`
	AvgCost      string `json:"avg_cost"`
	MarketValue  string `json:"market_value"`
	UnrealizedPL string `json:"unrealized_pl"`
}

type accountResponse struct {
	AccountID      string `json:"account_id"`
	NetLiquidation string `json:"net_liquidation"`
	CashBalance    string `json:"cash_balance"`
	BuyingPower    string `json:"buying_power"`
	MaintenanceReq string `json:"maintenance_req"`
}

type accountListResponse struct {
	Accounts []accountEntry `json:"accounts"`
}

type accountEntry struct {
	AccountID string `json:"account_id"`
	Status    string `json:"status"`
}

// --- Helper functions ---

func parseFloat(value string) float64 {
	result, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}

	return result
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// --- Outbound mapping: broker.* -> Webull ---

func toWebullOrder(order broker.Order, fractional bool) orderRequest {
	req := orderRequest{
		Symbol:      order.Asset.Ticker,
		Side:        mapSide(order.Side),
		OrderType:   mapOrderType(order.OrderType),
		TimeInForce: mapTimeInForce(order.TimeInForce),
		ClientID:    uuid.New().String(),
	}

	if order.OrderType == broker.Limit || order.OrderType == broker.StopLimit {
		req.LimitPrice = formatFloat(order.LimitPrice)
	}

	if order.OrderType == broker.Stop || order.OrderType == broker.StopLimit {
		req.StopPrice = formatFloat(order.StopPrice)
	}

	if fractional && order.Qty == 0 && order.Amount > 0 {
		req.Notional = formatFloat(order.Amount)
	} else {
		req.Qty = formatFloat(order.Qty)
	}

	return req
}

// --- Inbound mapping: Webull -> broker.* ---

func toBrokerOrder(resp orderResponse) broker.Order {
	return broker.Order{
		ID:         resp.ID,
		Asset:      asset.Asset{Ticker: resp.Symbol},
		Side:       mapWebullSide(resp.Side),
		Status:     mapWebullStatus(resp.Status),
		Qty:        parseFloat(resp.Qty),
		OrderType:  mapWebullOrderType(resp.OrderType),
		LimitPrice: parseFloat(resp.LimitPrice),
		StopPrice:  parseFloat(resp.StopPrice),
	}
}

func toBrokerPosition(resp positionResponse) broker.Position {
	return broker.Position{
		Asset:        asset.Asset{Ticker: resp.Symbol},
		Qty:          parseFloat(resp.Qty),
		AvgOpenPrice: parseFloat(resp.AvgCost),
		MarkPrice:    parseFloat(resp.MarketValue),
	}
}

func toBrokerBalance(resp accountResponse) broker.Balance {
	return broker.Balance{
		NetLiquidatingValue: parseFloat(resp.NetLiquidation),
		CashBalance:         parseFloat(resp.CashBalance),
		EquityBuyingPower:   parseFloat(resp.BuyingPower),
		MaintenanceReq:      parseFloat(resp.MaintenanceReq),
	}
}

// --- Outbound enum mappers ---

func mapSide(side broker.Side) string {
	switch side {
	case broker.Buy:
		return "BUY"
	case broker.Sell:
		return "SELL"
	default:
		return "BUY"
	}
}

func mapOrderType(orderType broker.OrderType) string {
	switch orderType {
	case broker.Market:
		return "MARKET"
	case broker.Limit:
		return "LIMIT"
	case broker.Stop:
		return "STOP_LOSS"
	case broker.StopLimit:
		return "STOP_LOSS_LIMIT"
	default:
		return "MARKET"
	}
}

func mapTimeInForce(tif broker.TimeInForce) string {
	switch tif {
	case broker.Day:
		return "DAY"
	case broker.GTC:
		return "GTC"
	default:
		return "DAY"
	}
}

// --- Inbound enum mappers ---

func mapWebullSide(side string) broker.Side {
	switch side {
	case "BUY":
		return broker.Buy
	case "SELL":
		return broker.Sell
	default:
		return broker.Buy
	}
}

func mapWebullOrderType(orderType string) broker.OrderType {
	switch orderType {
	case "MARKET":
		return broker.Market
	case "LIMIT":
		return broker.Limit
	case "STOP_LOSS":
		return broker.Stop
	case "STOP_LOSS_LIMIT":
		return broker.StopLimit
	default:
		return broker.Market
	}
}

func mapWebullStatus(status string) broker.OrderStatus {
	switch status {
	case "PENDING", "NEW":
		return broker.OrderSubmitted
	case "PARTIALLY_FILLED":
		return broker.OrderPartiallyFilled
	case "FILLED":
		return broker.OrderFilled
	case "CANCELLED", "EXPIRED", "REJECTED":
		return broker.OrderCancelled
	default:
		return broker.OrderOpen
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `ginkgo run ./broker/webull/...`
Expected: All type tests PASS.

- [ ] **Step 6: Run lint**

Run: `golangci-lint run ./broker/webull/...`
Expected: No lint errors.

- [ ] **Step 7: Commit**

```bash
git add broker/webull/types.go broker/webull/types_test.go broker/webull/exports_test.go
git commit -m "feat(webull): add request/response types and mapping functions (#18)"
```

---

## Task 5: HTTP Client

**Files:**
- Create: `broker/webull/client.go`
- Create: `broker/webull/client_test.go`
- Modify: `broker/webull/exports_test.go`

- [ ] **Step 1: Add client exports to `exports_test.go`**

Append to `broker/webull/exports_test.go`:

```go
// APIClientForTestType is an exported alias for apiClient.
type APIClientForTestType = apiClient

// NewAPIClientForTest creates an apiClient pointing at a custom base URL
// with an hmacSigner for testing.
func NewAPIClientForTest(baseURL, appKey, appSecret string) *apiClient {
	sign := &hmacSigner{appKey: appKey, appSecret: appSecret}
	return newAPIClient(baseURL, sign)
}

// GetAccount exposes getAccount for testing.
func (client *apiClient) GetAccount(ctx context.Context) (accountResponse, error) {
	return client.getAccount(ctx)
}

// GetAccounts exposes getAccounts for testing.
func (client *apiClient) GetAccounts(ctx context.Context) ([]accountEntry, error) {
	return client.getAccounts(ctx)
}

// SubmitOrder exposes submitOrder for testing.
func (client *apiClient) SubmitOrder(ctx context.Context, accountID string, order orderRequest) (string, error) {
	return client.submitOrder(ctx, accountID, order)
}

// CancelOrder exposes cancelOrder for testing.
func (client *apiClient) CancelOrder(ctx context.Context, accountID, orderID string) error {
	return client.cancelOrder(ctx, accountID, orderID)
}

// ReplaceOrder exposes replaceOrder for testing.
func (client *apiClient) ReplaceOrder(ctx context.Context, accountID, orderID string, req replaceRequest) error {
	return client.replaceOrder(ctx, accountID, orderID, req)
}

// GetOrders exposes getOrders for testing.
func (client *apiClient) GetOrders(ctx context.Context, accountID string) ([]orderResponse, error) {
	return client.getOrders(ctx, accountID)
}

// GetPositions exposes getPositions for testing.
func (client *apiClient) GetPositions(ctx context.Context, accountID string) ([]positionResponse, error) {
	return client.getPositions(ctx, accountID)
}

// GetBalance exposes getBalance for testing.
func (client *apiClient) GetBalance(ctx context.Context, accountID string) (accountResponse, error) {
	return client.getBalance(ctx, accountID)
}
```

Also add `"context"` to imports in `exports_test.go`.

- [ ] **Step 2: Write failing tests in `broker/webull/client_test.go`**

```go
package webull_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/broker/webull"
)

var _ = Describe("Client", func() {
	var (
		server *httptest.Server
		client *webull.APIClientForTestType
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Describe("getAccounts", func() {
		It("returns the account list", func() {
			server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/api/trade/account/list"))
				json.NewEncoder(rw).Encode(map[string]interface{}{
					"accounts": []map[string]string{
						{"account_id": "acct-1", "status": "ACTIVE"},
						{"account_id": "acct-2", "status": "ACTIVE"},
					},
				})
			}))
			client = webull.NewAPIClientForTest(server.URL, "key", "secret")

			accounts, err := client.GetAccounts(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(accounts).To(HaveLen(2))
			Expect(accounts[0].AccountID).To(Equal("acct-1"))
		})
	})

	Describe("submitOrder", func() {
		It("sends the order and returns the order ID", func() {
			server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal(http.MethodPost))
				Expect(req.URL.Path).To(Equal("/api/trade/order/place"))
				json.NewEncoder(rw).Encode(map[string]string{
					"order_id": "ord-123",
				})
			}))
			client = webull.NewAPIClientForTest(server.URL, "key", "secret")

			orderID, err := client.SubmitOrder(ctx, "acct-1", webull.OrderRequestExport{
				Symbol:    "AAPL",
				Side:      "BUY",
				OrderType: "MARKET",
				Qty:       "10",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(orderID).To(Equal("ord-123"))
		})

		It("returns an HTTPError on non-2xx response", func() {
			server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(http.StatusBadRequest)
				rw.Write([]byte("invalid order"))
			}))
			client = webull.NewAPIClientForTest(server.URL, "key", "secret")

			_, err := client.SubmitOrder(ctx, "acct-1", webull.OrderRequestExport{})
			Expect(err).To(HaveOccurred())

			var httpErr *webull.HTTPError
			Expect(err).To(BeAssignableToTypeOf(httpErr))
		})
	})

	Describe("cancelOrder", func() {
		It("cancels the order", func() {
			server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal(http.MethodPost))
				Expect(req.URL.Path).To(Equal("/api/trade/order/cancel"))
				rw.WriteHeader(http.StatusOK)
			}))
			client = webull.NewAPIClientForTest(server.URL, "key", "secret")

			err := client.CancelOrder(ctx, "acct-1", "ord-123")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("replaceOrder", func() {
		It("replaces the order", func() {
			server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal(http.MethodPost))
				Expect(req.URL.Path).To(Equal("/api/trade/order/replace"))
				rw.WriteHeader(http.StatusOK)
			}))
			client = webull.NewAPIClientForTest(server.URL, "key", "secret")

			err := client.ReplaceOrder(ctx, "acct-1", "ord-123", webull.ReplaceRequestExport{
				Qty: "20",
			})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("getOrders", func() {
		It("returns the order list", func() {
			server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/api/trade/order/list"))
				json.NewEncoder(rw).Encode(map[string]interface{}{
					"orders": []map[string]string{
						{"order_id": "ord-1", "symbol": "AAPL", "side": "BUY", "order_status": "FILLED"},
					},
				})
			}))
			client = webull.NewAPIClientForTest(server.URL, "key", "secret")

			orders, err := client.GetOrders(ctx, "acct-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].ID).To(Equal("ord-1"))
		})
	})

	Describe("getPositions", func() {
		It("returns positions", func() {
			server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/api/trade/account/positions"))
				json.NewEncoder(rw).Encode(map[string]interface{}{
					"positions": []map[string]string{
						{"symbol": "AAPL", "qty": "10", "avg_cost": "150"},
					},
				})
			}))
			client = webull.NewAPIClientForTest(server.URL, "key", "secret")

			positions, err := client.GetPositions(ctx, "acct-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Symbol).To(Equal("AAPL"))
		})
	})

	Describe("getBalance", func() {
		It("returns the account balance", func() {
			server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/api/trade/account/detail"))
				json.NewEncoder(rw).Encode(map[string]string{
					"account_id":      "acct-1",
					"net_liquidation": "100000.50",
					"cash_balance":    "25000",
					"buying_power":    "50000",
					"maintenance_req": "15000",
				})
			}))
			client = webull.NewAPIClientForTest(server.URL, "key", "secret")

			balance, err := client.GetBalance(ctx, "acct-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(balance.NetLiquidation).To(Equal("100000.50"))
		})
	})
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `ginkgo run ./broker/webull/...`
Expected: FAIL -- `apiClient` and methods not defined.

- [ ] **Step 4: Implement `broker/webull/client.go`**

```go
package webull

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
)

type apiClient struct {
	resty  *resty.Client
	signer signer
}

// newAPIClient creates an apiClient with retry and the given signer.
func newAPIClient(baseURL string, sign signer) *apiClient {
	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(4 * time.Second).
		SetHeader("Content-Type", "application/json")

	httpClient.AddRetryCondition(func(resp *resty.Response, err error) bool {
		if err != nil {
			return broker.IsRetryableError(err)
		}

		return resp.StatusCode() == 429 || resp.StatusCode() >= 500
	})

	// Use OnBeforeRequest to sign every request.
	httpClient.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		rawReq := req.RawRequest
		if rawReq == nil {
			// Build a temporary request for signing.
			tmpReq, buildErr := http.NewRequestWithContext(
				req.Context(),
				req.Method,
				baseURL+req.URL,
				nil,
			)
			if buildErr != nil {
				return fmt.Errorf("webull: build request for signing: %w", buildErr)
			}

			if signErr := sign.Sign(tmpReq); signErr != nil {
				return signErr
			}

			for key, values := range tmpReq.Header {
				for _, val := range values {
					req.SetHeader(key, val)
				}
			}

			return nil
		}

		return sign.Sign(rawReq)
	})

	return &apiClient{
		resty:  httpClient,
		signer: sign,
	}
}

// getAccounts retrieves the list of accounts.
func (client *apiClient) getAccounts(ctx context.Context) ([]accountEntry, error) {
	var result accountListResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/api/trade/account/list")
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Accounts, nil
}

// getAccount retrieves account details (alias for getBalance for compatibility).
func (client *apiClient) getAccount(ctx context.Context) (accountResponse, error) {
	return client.getBalance(ctx, "")
}

// submitOrder sends an order and returns the order ID.
func (client *apiClient) submitOrder(ctx context.Context, accountID string, order orderRequest) (string, error) {
	type submitResponse struct {
		OrderID string `json:"order_id"`
	}

	var result submitResponse

	body := struct {
		AccountID string `json:"account_id"`
		orderRequest
	}{
		AccountID:    accountID,
		orderRequest: order,
	}

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(body).
		SetResult(&result).
		Post("/api/trade/order/place")
	if err != nil {
		return "", fmt.Errorf("submit order: %w", err)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.OrderID, nil
}

// cancelOrder cancels an existing order.
func (client *apiClient) cancelOrder(ctx context.Context, accountID, orderID string) error {
	body := struct {
		AccountID string `json:"account_id"`
		OrderID   string `json:"order_id"`
	}{
		AccountID: accountID,
		OrderID:   orderID,
	}

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(body).
		Post("/api/trade/order/cancel")
	if err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// replaceOrder modifies an existing order's qty and/or price.
func (client *apiClient) replaceOrder(ctx context.Context, accountID, orderID string, replacement replaceRequest) error {
	body := struct {
		AccountID string `json:"account_id"`
		OrderID   string `json:"order_id"`
		replaceRequest
	}{
		AccountID:      accountID,
		OrderID:        orderID,
		replaceRequest: replacement,
	}

	resp, err := client.resty.R().
		SetContext(ctx).
		SetBody(body).
		Post("/api/trade/order/replace")
	if err != nil {
		return fmt.Errorf("replace order: %w", err)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// getOrders retrieves all orders for the account.
func (client *apiClient) getOrders(ctx context.Context, accountID string) ([]orderResponse, error) {
	type ordersResponse struct {
		Orders []orderResponse `json:"orders"`
	}

	var result ordersResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetQueryParam("account_id", accountID).
		SetResult(&result).
		Get("/api/trade/order/list")
	if err != nil {
		return nil, fmt.Errorf("get orders: %w", err)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Orders, nil
}

// getPositions retrieves all positions for the account.
func (client *apiClient) getPositions(ctx context.Context, accountID string) ([]positionResponse, error) {
	type positionsResponse struct {
		Positions []positionResponse `json:"positions"`
	}

	var result positionsResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetQueryParam("account_id", accountID).
		SetResult(&result).
		Get("/api/trade/account/positions")
	if err != nil {
		return nil, fmt.Errorf("get positions: %w", err)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Positions, nil
}

// getBalance retrieves the account balance.
func (client *apiClient) getBalance(ctx context.Context, accountID string) (accountResponse, error) {
	var result accountResponse

	resp, err := client.resty.R().
		SetContext(ctx).
		SetQueryParam("account_id", accountID).
		SetResult(&result).
		Get("/api/trade/account/detail")
	if err != nil {
		return accountResponse{}, fmt.Errorf("get balance: %w", err)
	}

	if resp.IsError() {
		return accountResponse{}, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `ginkgo run ./broker/webull/...`
Expected: All client tests PASS.

- [ ] **Step 6: Run lint**

Run: `golangci-lint run ./broker/webull/...`
Expected: No lint errors.

- [ ] **Step 7: Commit**

```bash
git add broker/webull/client.go broker/webull/client_test.go broker/webull/exports_test.go
git commit -m "feat(webull): implement HTTP client with signed requests (#18)"
```

---

## Task 6: Broker Facade

**Files:**
- Create: `broker/webull/broker.go`
- Create: `broker/webull/broker_test.go`
- Modify: `broker/webull/exports_test.go`

- [ ] **Step 1: Add broker exports to `exports_test.go`**

Append to `broker/webull/exports_test.go`:

```go
// SetClientForTest replaces the broker's internal client with one
// pointing at the given URL.
func SetClientForTest(wb *WebullBroker, baseURL, appKey, appSecret string) {
	sign := &hmacSigner{appKey: appKey, appSecret: appSecret}
	wb.client = newAPIClient(baseURL, sign)
	wb.accountID = "test-account"
}
```

- [ ] **Step 2: Write failing tests in `broker/webull/broker_test.go`**

```go
package webull_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/webull"
)

var _ = Describe("Broker", func() {
	Describe("Connect", func() {
		AfterEach(func() {
			os.Unsetenv("WEBULL_APP_KEY")
			os.Unsetenv("WEBULL_APP_SECRET")
			os.Unsetenv("WEBULL_ACCOUNT_ID")
		})

		It("returns ErrMissingCredentials when no env vars are set", func() {
			wb := webull.New()
			err := wb.Connect(context.Background())
			Expect(err).To(MatchError(webull.ErrMissingCredentials))
		})
	})

	Describe("Submit", func() {
		It("rejects unsupported time-in-force", func() {
			wb := webull.New()
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				json.NewEncoder(rw).Encode(map[string]string{"order_id": "ord-1"})
			}))
			defer server.Close()
			webull.SetClientForTest(wb, server.URL, "key", "secret")

			err := wb.Submit(context.Background(), broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.IOC, // Not supported by Webull
			})
			Expect(err).To(MatchError(webull.ErrUnsupportedTimeInForce))
		})

		It("rejects dollar-amount orders without WithFractionalShares", func() {
			wb := webull.New()
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				json.NewEncoder(rw).Encode(map[string]string{"order_id": "ord-1"})
			}))
			defer server.Close()
			webull.SetClientForTest(wb, server.URL, "key", "secret")

			err := wb.Submit(context.Background(), broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Amount:      500,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})
			Expect(err).To(MatchError(webull.ErrFractionalNotEnabled))
		})

		It("rejects non-market dollar-amount orders", func() {
			wb := webull.New(webull.WithFractionalShares())
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				json.NewEncoder(rw).Encode(map[string]string{"order_id": "ord-1"})
			}))
			defer server.Close()
			webull.SetClientForTest(wb, server.URL, "key", "secret")

			err := wb.Submit(context.Background(), broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Amount:      500,
				OrderType:   broker.Limit,
				LimitPrice:  150,
				TimeInForce: broker.Day,
			})
			Expect(err).To(MatchError(webull.ErrFractionalNotMarket))
		})

		It("submits a valid order and returns no error", func() {
			wb := webull.New()
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				json.NewEncoder(rw).Encode(map[string]string{"order_id": "ord-1"})
			}))
			defer server.Close()
			webull.SetClientForTest(wb, server.URL, "key", "secret")

			err := wb.Submit(context.Background(), broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Replace", func() {
		It("rejects replace when side differs", func() {
			wb := webull.New()
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				json.NewEncoder(rw).Encode(map[string]string{"order_id": "ord-1"})
			}))
			defer server.Close()
			webull.SetClientForTest(wb, server.URL, "key", "secret")

			// Submit the original order first.
			Expect(wb.Submit(context.Background(), broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})).To(Succeed())

			err := wb.Replace(context.Background(), "ord-1", broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Sell, // Changed side
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})
			Expect(err).To(MatchError(webull.ErrReplaceFieldNotAllowed))
		})
	})

	Describe("Orders", func() {
		It("returns broker orders", func() {
			wb := webull.New()
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				json.NewEncoder(rw).Encode(map[string]interface{}{
					"orders": []map[string]string{
						{"order_id": "ord-1", "symbol": "AAPL", "side": "BUY", "order_status": "FILLED", "order_type": "MARKET", "qty": "10"},
					},
				})
			}))
			defer server.Close()
			webull.SetClientForTest(wb, server.URL, "key", "secret")

			orders, err := wb.Orders(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].ID).To(Equal("ord-1"))
		})
	})

	Describe("Positions", func() {
		It("returns broker positions", func() {
			wb := webull.New()
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				json.NewEncoder(rw).Encode(map[string]interface{}{
					"positions": []map[string]string{
						{"symbol": "AAPL", "qty": "10", "avg_cost": "150"},
					},
				})
			}))
			defer server.Close()
			webull.SetClientForTest(wb, server.URL, "key", "secret")

			positions, err := wb.Positions(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Asset.Ticker).To(Equal("AAPL"))
		})
	})

	Describe("Balance", func() {
		It("returns the account balance", func() {
			wb := webull.New()
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				json.NewEncoder(rw).Encode(map[string]string{
					"net_liquidation": "100000",
					"cash_balance":    "25000",
					"buying_power":    "50000",
					"maintenance_req": "15000",
				})
			}))
			defer server.Close()
			webull.SetClientForTest(wb, server.URL, "key", "secret")

			balance, err := wb.Balance(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(Equal(25000.0))
		})
	})

	Describe("Transactions", func() {
		It("returns an empty slice", func() {
			wb := webull.New()
			txns, err := wb.Transactions(context.Background(), time.Time{})
			Expect(err).ToNot(HaveOccurred())
			Expect(txns).To(BeEmpty())
		})
	})
})
```

Add `"time"` to the test imports.

- [ ] **Step 3: Run tests to verify they fail**

Run: `ginkgo run ./broker/webull/...`
Expected: FAIL -- `WebullBroker` not defined.

- [ ] **Step 4: Implement `broker/webull/broker.go`**

```go
package webull

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	productionBaseURL = "https://api.webull.com"
	uatBaseURL        = "https://us-openapi-alb.uat.webullbroker.com"
	productionGRPC    = "events-api.webull.com:443"
	uatGRPC           = "events-api.uat.webullbroker.com:443"
	productionAuthURL = "https://us-oauth-open-api.webull.com"
	uatAuthURL        = "https://us-oauth-open-api.uat.webullbroker.com"
	fillChannelSize   = 1024
)

// WebullBroker implements broker.Broker for the Webull brokerage.
type WebullBroker struct {
	client          *apiClient
	fills           chan broker.Fill
	accountID       string
	fractional      bool
	uat             bool
	tokenFile       string
	callbackURL     string
	submittedOrders map[string]broker.Order
	mu              sync.Mutex
}

// Option configures a WebullBroker.
type Option func(*WebullBroker)

// WithUAT configures the broker to use Webull's UAT/test environment.
func WithUAT() Option {
	return func(wb *WebullBroker) {
		wb.uat = true
	}
}

// WithFractionalShares enables dollar-amount orders.
func WithFractionalShares() Option {
	return func(wb *WebullBroker) {
		wb.fractional = true
	}
}

// WithTokenFile overrides the OAuth token persistence location.
func WithTokenFile(path string) Option {
	return func(wb *WebullBroker) {
		wb.tokenFile = path
	}
}

// WithCallbackURL overrides the OAuth callback URL.
func WithCallbackURL(callbackURL string) Option {
	return func(wb *WebullBroker) {
		wb.callbackURL = callbackURL
	}
}

// WithAccountID overrides the account selection.
func WithAccountID(accountID string) Option {
	return func(wb *WebullBroker) {
		wb.accountID = accountID
	}
}

// New creates a new WebullBroker with the given options.
func New(opts ...Option) *WebullBroker {
	wb := &WebullBroker{
		fills:           make(chan broker.Fill, fillChannelSize),
		submittedOrders: make(map[string]broker.Order),
	}

	for _, opt := range opts {
		opt(wb)
	}

	return wb
}

// Connect establishes a session with Webull by detecting the auth mode,
// creating the appropriate signer, and selecting an account.
func (wb *WebullBroker) Connect(ctx context.Context) error {
	mode, err := detectAuthMode()
	if err != nil {
		return err
	}

	baseURL := productionBaseURL
	authBaseURL := productionAuthURL

	if wb.uat {
		baseURL = uatBaseURL
		authBaseURL = uatAuthURL
	}

	var sign signer

	switch mode {
	case authModeDirect:
		sign = &hmacSigner{
			appKey:    getEnv("WEBULL_APP_KEY"),
			appSecret: getEnv("WEBULL_APP_SECRET"),
		}
	case authModeOAuth:
		tm := newTokenManager(
			mode,
			getEnv("WEBULL_CLIENT_ID"),
			getEnv("WEBULL_CLIENT_SECRET"),
			wb.callbackURL,
			wb.tokenFile,
			authBaseURL,
		)

		if loadErr := tm.loadTokens(); loadErr != nil {
			return fmt.Errorf("webull: load tokens: %w", loadErr)
		}

		// If no valid tokens exist, run the authorization flow.
		if tm.tokens == nil || tm.tokens.AccessToken == "" {
			if authErr := tm.authorize(ctx); authErr != nil {
				return fmt.Errorf("webull: authorize: %w", authErr)
			}
		}

		tm.startRefreshLoop()

		sign = &oauthSigner{tokenMgr: tm}
	}

	wb.client = newAPIClient(baseURL, sign)

	// Select account.
	if wb.accountID == "" {
		envAccountID := getEnv("WEBULL_ACCOUNT_ID")
		if envAccountID != "" {
			wb.accountID = envAccountID
		} else {
			accounts, accountErr := wb.client.getAccounts(ctx)
			if accountErr != nil {
				return fmt.Errorf("webull: get accounts: %w", accountErr)
			}

			if len(accounts) == 0 {
				return broker.ErrAccountNotFound
			}

			wb.accountID = accounts[0].AccountID
			log.Info().Str("account_id", wb.accountID).Msg("webull: using first account")
		}
	}

	return nil
}

// Close tears down the broker session and releases resources.
func (wb *WebullBroker) Close() error {
	close(wb.fills)

	return nil
}

// Fills returns a receive-only channel on which fill reports are delivered.
func (wb *WebullBroker) Fills() <-chan broker.Fill {
	return wb.fills
}

// Submit sends an order to Webull.
func (wb *WebullBroker) Submit(ctx context.Context, order broker.Order) error {
	if err := wb.validateOrder(order); err != nil {
		return err
	}

	webullOrder := toWebullOrder(order, wb.fractional)

	orderID, err := wb.client.submitOrder(ctx, wb.accountID, webullOrder)
	if err != nil {
		return fmt.Errorf("webull: submit order: %w", err)
	}

	wb.mu.Lock()
	wb.submittedOrders[orderID] = order
	wb.mu.Unlock()

	return nil
}

// Cancel requests cancellation of an open order by ID.
func (wb *WebullBroker) Cancel(ctx context.Context, orderID string) error {
	if err := wb.client.cancelOrder(ctx, wb.accountID, orderID); err != nil {
		return fmt.Errorf("webull: cancel order: %w", err)
	}

	wb.mu.Lock()
	delete(wb.submittedOrders, orderID)
	wb.mu.Unlock()

	return nil
}

// Replace cancels an existing order and submits a replacement. Webull only
// allows changing qty and price; differences in side, TIF, or order type
// return an error.
func (wb *WebullBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	wb.mu.Lock()
	original, exists := wb.submittedOrders[orderID]
	wb.mu.Unlock()

	if exists {
		if original.Side != order.Side ||
			original.TimeInForce != order.TimeInForce ||
			original.OrderType != order.OrderType {
			return ErrReplaceFieldNotAllowed
		}
	}

	replacement := replaceRequest{}

	if !exists || original.Qty != order.Qty {
		replacement.Qty = formatFloat(order.Qty)
	}

	if !exists || original.LimitPrice != order.LimitPrice {
		replacement.LimitPrice = formatFloat(order.LimitPrice)
	}

	if !exists || original.StopPrice != order.StopPrice {
		replacement.StopPrice = formatFloat(order.StopPrice)
	}

	if err := wb.client.replaceOrder(ctx, wb.accountID, orderID, replacement); err != nil {
		return fmt.Errorf("webull: replace order: %w", err)
	}

	wb.mu.Lock()
	wb.submittedOrders[orderID] = order
	wb.mu.Unlock()

	return nil
}

// Orders returns all orders for the current trading day.
func (wb *WebullBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	responses, err := wb.client.getOrders(ctx, wb.accountID)
	if err != nil {
		return nil, fmt.Errorf("webull: get orders: %w", err)
	}

	orders := make([]broker.Order, len(responses))
	for idx, resp := range responses {
		orders[idx] = toBrokerOrder(resp)
	}

	return orders, nil
}

// Positions returns all current positions in the account.
func (wb *WebullBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	responses, err := wb.client.getPositions(ctx, wb.accountID)
	if err != nil {
		return nil, fmt.Errorf("webull: get positions: %w", err)
	}

	positions := make([]broker.Position, len(responses))
	for idx, resp := range responses {
		positions[idx] = toBrokerPosition(resp)
	}

	return positions, nil
}

// Balance returns the current account balance.
func (wb *WebullBroker) Balance(ctx context.Context) (broker.Balance, error) {
	resp, err := wb.client.getBalance(ctx, wb.accountID)
	if err != nil {
		return broker.Balance{}, fmt.Errorf("webull: get balance: %w", err)
	}

	return toBrokerBalance(resp), nil
}

// Transactions returns an empty slice because Webull OpenAPI does not provide
// a transaction history endpoint for dividends, splits, or fees.
func (wb *WebullBroker) Transactions(_ context.Context, _ time.Time) ([]broker.Transaction, error) {
	log.Info().Msg("webull: Webull OpenAPI does not provide a transaction history endpoint; dividends, splits, and fees will not be synced")

	return nil, nil
}

// validateOrder checks that the order uses supported TIF, and that
// dollar-amount orders meet the fractional requirements.
func (wb *WebullBroker) validateOrder(order broker.Order) error {
	if order.TimeInForce != broker.Day && order.TimeInForce != broker.GTC {
		return ErrUnsupportedTimeInForce
	}

	if order.Qty == 0 && order.Amount > 0 {
		if !wb.fractional {
			return ErrFractionalNotEnabled
		}

		if order.OrderType != broker.Market {
			return ErrFractionalNotMarket
		}
	}

	return nil
}

// getEnv reads an environment variable, returning empty string if unset.
func getEnv(key string) string {
	return os.Getenv(key)
}
```

Add `"os"` to the imports.

- [ ] **Step 5: Run tests to verify they pass**

Run: `ginkgo run ./broker/webull/...`
Expected: All broker tests PASS.

- [ ] **Step 6: Run lint**

Run: `golangci-lint run ./broker/webull/...`
Expected: No lint errors.

- [ ] **Step 7: Commit**

```bash
git add broker/webull/broker.go broker/webull/broker_test.go broker/webull/exports_test.go
git commit -m "feat(webull): implement broker facade with order validation (#18)"
```

---

## Task 7: gRPC Fill Streamer

**Files:**
- Create: `broker/webull/streamer.go`
- Create: `broker/webull/streamer_test.go`
- Modify: `broker/webull/exports_test.go`
- Modify: `broker/webull/broker.go` (wire streamer into Connect/Close)

This task implements fill delivery via gRPC server-streaming, with exponential backoff reconnection and poll-based deduplication on reconnect.

- [ ] **Step 1: Add streamer exports to `exports_test.go`**

Append to `broker/webull/exports_test.go`:

```go
// FillStreamerForTestType is an exported alias for fillStreamer.
type FillStreamerForTestType = fillStreamer

// NewFillStreamerForTest creates a fillStreamer for testing with an injected
// poll function instead of a real gRPC connection.
func NewFillStreamerForTest(fills chan broker.Fill, pollFn func(ctx context.Context) ([]orderResponse, error)) *fillStreamer {
	return &fillStreamer{
		fills:       fills,
		done:        make(chan struct{}),
		cumulFilled: make(map[string]float64),
		pollOrders:  pollFn,
	}
}

// HandleTradeEvent exposes handleTradeEvent for testing.
func (fs *fillStreamer) HandleTradeEvent(orderID, status string, filledQty, filledPrice float64) {
	fs.handleTradeEvent(orderID, status, filledQty, filledPrice)
}

// PollMissedFills exposes pollMissedFills for testing.
func (fs *fillStreamer) PollMissedFills(ctx context.Context) {
	fs.pollMissedFills(ctx)
}

// CumulFilledForTest returns the cumulative filled qty for an order ID.
func (fs *fillStreamer) CumulFilledForTest(orderID string) float64 {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.cumulFilled[orderID]
}
```

- [ ] **Step 2: Write failing tests in `broker/webull/streamer_test.go`**

```go
package webull_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/webull"
)

var _ = Describe("Streamer", func() {
	var fills chan broker.Fill

	BeforeEach(func() {
		fills = make(chan broker.Fill, 16)
	})

	Describe("handleTradeEvent", func() {
		It("sends a fill for a FILLED event", func() {
			fs := webull.NewFillStreamerForTest(fills, nil)
			fs.HandleTradeEvent("ord-1", "FILLED", 5, 150.25)

			var fill broker.Fill
			Eventually(fills).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("ord-1"))
			Expect(fill.Qty).To(Equal(5.0))
			Expect(fill.Price).To(Equal(150.25))
		})

		It("sends a fill for a FINAL_FILLED event", func() {
			fs := webull.NewFillStreamerForTest(fills, nil)
			fs.HandleTradeEvent("ord-1", "FINAL_FILLED", 10, 151.00)

			var fill broker.Fill
			Eventually(fills).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("ord-1"))
			Expect(fill.Qty).To(Equal(10.0))
		})

		It("does not send a fill for non-fill events", func() {
			fs := webull.NewFillStreamerForTest(fills, nil)
			fs.HandleTradeEvent("ord-1", "CANCEL_SUCCESS", 0, 0)

			Consistently(fills, 100*time.Millisecond).ShouldNot(Receive())
		})

		It("deduplicates fills with same cumulative qty", func() {
			fs := webull.NewFillStreamerForTest(fills, nil)
			fs.HandleTradeEvent("ord-1", "FILLED", 5, 150)
			fs.HandleTradeEvent("ord-1", "FILLED", 5, 150) // duplicate

			var fill broker.Fill
			Eventually(fills).Should(Receive(&fill))
			Consistently(fills, 100*time.Millisecond).ShouldNot(Receive())
		})

		It("sends delta fill when cumulative qty increases", func() {
			fs := webull.NewFillStreamerForTest(fills, nil)
			fs.HandleTradeEvent("ord-1", "FILLED", 5, 150)
			fs.HandleTradeEvent("ord-1", "FINAL_FILLED", 10, 151)

			var fill1, fill2 broker.Fill
			Eventually(fills).Should(Receive(&fill1))
			Eventually(fills).Should(Receive(&fill2))
			Expect(fill1.Qty).To(Equal(5.0))
			Expect(fill2.Qty).To(Equal(5.0)) // delta: 10 - 5
		})
	})

	Describe("pollMissedFills", func() {
		It("sends fills from polled orders not yet seen", func() {
			pollFn := func(ctx context.Context) ([]webull.OrderResponseExport, error) {
				return []webull.OrderResponseExport{
					{ID: "ord-1", Symbol: "AAPL", Side: "BUY", Status: "FILLED", Qty: "10", FilledQty: "10", FilledPrice: "150"},
				}, nil
			}
			fs := webull.NewFillStreamerForTest(fills, pollFn)
			fs.PollMissedFills(context.Background())

			var fill broker.Fill
			Eventually(fills).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("ord-1"))
			Expect(fill.Qty).To(Equal(10.0))
		})

		It("does not duplicate fills already delivered via stream", func() {
			pollFn := func(ctx context.Context) ([]webull.OrderResponseExport, error) {
				return []webull.OrderResponseExport{
					{ID: "ord-1", Symbol: "AAPL", Side: "BUY", Status: "FILLED", Qty: "10", FilledQty: "10", FilledPrice: "150"},
				}, nil
			}
			fs := webull.NewFillStreamerForTest(fills, pollFn)

			// Deliver via stream first.
			fs.HandleTradeEvent("ord-1", "FINAL_FILLED", 10, 150)
			var fill broker.Fill
			Eventually(fills).Should(Receive(&fill))

			// Poll should not produce another fill.
			fs.PollMissedFills(context.Background())
			Consistently(fills, 100*time.Millisecond).ShouldNot(Receive())
		})
	})
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `ginkgo run ./broker/webull/...`
Expected: FAIL -- `fillStreamer` not defined.

- [ ] **Step 4: Implement `broker/webull/streamer.go`**

```go
package webull

import (
	"context"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	maxReconnectBackoff = 30 * time.Second
	initialBackoff      = 1 * time.Second
)

// fillStreamer delivers fill reports from gRPC trade events. On reconnect,
// it polls orders to catch any missed fills.
type fillStreamer struct {
	fills       chan broker.Fill
	done        chan struct{}
	wg          sync.WaitGroup
	mu          sync.Mutex
	cumulFilled map[string]float64 // order ID -> cumulative filled qty

	// pollOrders is a function that retrieves current orders. In production,
	// this calls client.getOrders; in tests, it can be injected.
	pollOrders func(ctx context.Context) ([]orderResponse, error)

	// gRPC connection fields (set during connect).
	grpcTarget string
	sign       signer
	accountID  string
}

// handleTradeEvent processes a trade event from the gRPC stream. It sends a
// broker.Fill on the fills channel when the cumulative filled quantity increases.
func (fs *fillStreamer) handleTradeEvent(orderID, status string, filledQty, filledPrice float64) {
	if status != "FILLED" && status != "FINAL_FILLED" {
		log.Info().Str("order_id", orderID).Str("status", status).Msg("webull: trade event")
		return
	}

	fs.mu.Lock()
	prev := fs.cumulFilled[orderID]

	if filledQty <= prev {
		fs.mu.Unlock()
		return
	}

	delta := filledQty - prev
	fs.cumulFilled[orderID] = filledQty
	fs.mu.Unlock()

	fill := broker.Fill{
		OrderID:  orderID,
		Price:    filledPrice,
		Qty:      delta,
		FilledAt: time.Now(),
	}

	select {
	case fs.fills <- fill:
	case <-fs.done:
	}
}

// pollMissedFills queries orders and sends any fills not yet delivered.
func (fs *fillStreamer) pollMissedFills(ctx context.Context) {
	if fs.pollOrders == nil {
		return
	}

	orders, err := fs.pollOrders(ctx)
	if err != nil {
		log.Error().Err(err).Msg("webull: poll missed fills failed")
		return
	}

	for _, order := range orders {
		if order.Status != "FILLED" && order.Status != "PARTIALLY_FILLED" {
			continue
		}

		filledQty := parseFloat(order.FilledQty)
		filledPrice := parseFloat(order.FilledPrice)

		fs.handleTradeEvent(order.ID, "FILLED", filledQty, filledPrice)
	}
}

// close signals the background goroutine to exit and waits for it.
func (fs *fillStreamer) close() error {
	close(fs.done)
	fs.wg.Wait()

	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `ginkgo run ./broker/webull/...`
Expected: All streamer tests PASS.

- [ ] **Step 6: Wire streamer into broker.go**

Update `Connect` in `broker.go` to initialize the streamer after the client is set up. Update `Close` to stop the streamer before closing the fills channel.

In the `Connect` method, after the account selection block, add:

```go
	// Initialize the fill streamer.
	wb.streamer = &fillStreamer{
		fills:       wb.fills,
		done:        make(chan struct{}),
		cumulFilled: make(map[string]float64),
		pollOrders: func(ctx context.Context) ([]orderResponse, error) {
			return wb.client.getOrders(ctx, wb.accountID)
		},
		grpcTarget: grpcTarget,
		sign:       sign,
		accountID:  wb.accountID,
	}
```

Add `grpcTarget` variable alongside `baseURL` and `authBaseURL`:

```go
	grpcTarget := productionGRPC
	if wb.uat {
		grpcTarget = uatGRPC
	}
```

Add `streamer *fillStreamer` field to `WebullBroker` struct.

Update `Close`:

```go
func (wb *WebullBroker) Close() error {
	if wb.streamer != nil {
		if err := wb.streamer.close(); err != nil {
			return err
		}
	}

	close(wb.fills)

	return nil
}
```

- [ ] **Step 7: Run all tests**

Run: `ginkgo run ./broker/webull/...`
Expected: All tests PASS.

- [ ] **Step 8: Run lint**

Run: `golangci-lint run ./broker/webull/...`
Expected: No lint errors.

- [ ] **Step 9: Commit**

```bash
git add broker/webull/streamer.go broker/webull/streamer_test.go broker/webull/broker.go broker/webull/exports_test.go
git commit -m "feat(webull): implement gRPC fill streamer with deduplication (#18)"
```

---

## Task 8: Integration Wiring and Final Verification

**Files:**
- Modify: `broker/webull/broker.go` (ensure `go get` dependencies are resolved)
- Run: full test suite, lint, build

- [ ] **Step 1: Add module dependencies**

Run: `go get github.com/go-resty/resty/v2 github.com/google/uuid google.golang.org/grpc`

- [ ] **Step 2: Run go mod tidy**

Run: `go mod tidy`
Expected: Success.

- [ ] **Step 3: Build the full project**

Run: `make build`
Expected: Success.

- [ ] **Step 4: Run the Webull test suite**

Run: `ginkgo run -race ./broker/webull/...`
Expected: All tests PASS with race detector.

- [ ] **Step 5: Run full project test suite**

Run: `make test`
Expected: All tests PASS (existing tests not broken).

- [ ] **Step 6: Run full lint**

Run: `make lint`
Expected: No lint errors.

- [ ] **Step 7: Commit any dependency changes**

```bash
git add go.mod go.sum
git commit -m "chore: add Webull broker dependencies (#18)"
```
