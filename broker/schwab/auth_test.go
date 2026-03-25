package schwab

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("tokenManager", func() {
	Describe("Token file I/O", func() {
		It("saves and loads tokens from a file", func() {
			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			store := &tokenStore{
				AccessToken:      "access-abc",
				RefreshToken:     "refresh-xyz",
				AccessExpiresAt:  time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC),
				RefreshExpiresAt: time.Date(2026, 3, 29, 16, 0, 0, 0, time.UTC),
			}

			saveErr := saveTokens(tokenPath, store)
			Expect(saveErr).ToNot(HaveOccurred())

			loaded, loadErr := loadTokens(tokenPath)
			Expect(loadErr).ToNot(HaveOccurred())
			Expect(loaded.AccessToken).To(Equal("access-abc"))
			Expect(loaded.RefreshToken).To(Equal("refresh-xyz"))
			Expect(loaded.AccessExpiresAt).To(BeTemporally("~", store.AccessExpiresAt, time.Second))
			Expect(loaded.RefreshExpiresAt).To(BeTemporally("~", store.RefreshExpiresAt, time.Second))
		})

		It("returns an error when the file does not exist", func() {
			_, loadErr := loadTokens("/nonexistent/path/tokens.json")
			Expect(loadErr).To(HaveOccurred())
		})

		It("creates parent directories when saving", func() {
			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "sub", "dir", "tokens.json")

			store := &tokenStore{AccessToken: "test"}
			saveErr := saveTokens(tokenPath, store)
			Expect(saveErr).ToNot(HaveOccurred())

			_, statErr := os.Stat(tokenPath)
			Expect(statErr).ToNot(HaveOccurred())
		})
	})

	Describe("Token expiry detection", func() {
		It("detects an expired access token", func() {
			manager := &tokenManager{
				tokens: &tokenStore{
					AccessExpiresAt: time.Now().Add(-1 * time.Minute),
				},
			}

			Expect(manager.accessTokenExpired()).To(BeTrue())
		})

		It("detects a valid access token", func() {
			manager := &tokenManager{
				tokens: &tokenStore{
					AccessExpiresAt: time.Now().Add(10 * time.Minute),
				},
			}

			Expect(manager.accessTokenExpired()).To(BeFalse())
		})

		It("detects an expired refresh token", func() {
			manager := &tokenManager{
				tokens: &tokenStore{
					RefreshExpiresAt: time.Now().Add(-1 * time.Hour),
				},
			}

			Expect(manager.refreshTokenExpired()).To(BeTrue())
		})

		It("detects a valid refresh token", func() {
			manager := &tokenManager{
				tokens: &tokenStore{
					RefreshExpiresAt: time.Now().Add(24 * time.Hour),
				},
			}

			Expect(manager.refreshTokenExpired()).To(BeFalse())
		})
	})

	Describe("refreshAccessToken", func() {
		It("exchanges a refresh token for a new access token", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal("POST"))
				Expect(req.URL.Path).To(Equal("/v1/oauth/token"))

				parseErr := req.ParseForm()
				Expect(parseErr).ToNot(HaveOccurred())
				Expect(req.Form.Get("grant_type")).To(Equal("refresh_token"))
				Expect(req.Form.Get("refresh_token")).To(Equal("old-refresh"))

				username, password, hasAuth := req.BasicAuth()
				Expect(hasAuth).To(BeTrue())
				Expect(username).To(Equal("test-client-id"))
				Expect(password).To(Equal("test-client-secret"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(schwabTokenResponse{
					AccessToken:  "new-access",
					RefreshToken: "new-refresh",
					ExpiresIn:    1800,
					TokenType:    "Bearer",
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			manager := &tokenManager{
				clientID:     "test-client-id",
				clientSecret: "test-client-secret",
				tokenFile:    tokenPath,
				authBaseURL:  server.URL,
				tokens: &tokenStore{
					RefreshToken:     "old-refresh",
					RefreshExpiresAt: time.Now().Add(24 * time.Hour),
				},
			}

			refreshErr := manager.refreshAccessToken()
			Expect(refreshErr).ToNot(HaveOccurred())
			Expect(manager.tokens.AccessToken).To(Equal("new-access"))
			Expect(manager.tokens.RefreshToken).To(Equal("new-refresh"))
			Expect(manager.tokens.AccessExpiresAt).To(BeTemporally(">", time.Now()))
		})

		It("returns an error on HTTP failure", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.WriteHeader(http.StatusUnauthorized)
				writer.Write([]byte(`{"error":"invalid_grant"}`))
			}))
			DeferCleanup(server.Close)

			manager := &tokenManager{
				clientID:     "test-client-id",
				clientSecret: "test-client-secret",
				tokenFile:    "/tmp/unused.json",
				authBaseURL:  server.URL,
				tokens: &tokenStore{
					RefreshToken:     "bad-refresh",
					RefreshExpiresAt: time.Now().Add(24 * time.Hour),
				},
			}

			refreshErr := manager.refreshAccessToken()
			Expect(refreshErr).To(HaveOccurred())
		})
	})

	Describe("exchangeAuthCode", func() {
		It("exchanges an authorization code for tokens", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				parseErr := req.ParseForm()
				Expect(parseErr).ToNot(HaveOccurred())
				Expect(req.Form.Get("grant_type")).To(Equal("authorization_code"))
				Expect(req.Form.Get("code")).To(Equal("auth-code-123"))
				Expect(req.Form.Get("redirect_uri")).To(Equal("https://127.0.0.1:5174"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(schwabTokenResponse{
					AccessToken:  "fresh-access",
					RefreshToken: "fresh-refresh",
					ExpiresIn:    1800,
					TokenType:    "Bearer",
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			manager := &tokenManager{
				clientID:     "test-client-id",
				clientSecret: "test-client-secret",
				callbackURL:  "https://127.0.0.1:5174",
				tokenFile:    tokenPath,
				authBaseURL:  server.URL,
				tokens:       &tokenStore{},
			}

			exchangeErr := manager.exchangeAuthCode("auth-code-123")
			Expect(exchangeErr).ToNot(HaveOccurred())
			Expect(manager.tokens.AccessToken).To(Equal("fresh-access"))
			Expect(manager.tokens.RefreshToken).To(Equal("fresh-refresh"))
		})
	})

	Describe("ensureValidToken", func() {
		It("does nothing when the access token is still valid", func() {
			manager := &tokenManager{
				tokens: &tokenStore{
					AccessToken:      "valid-token",
					AccessExpiresAt:  time.Now().Add(10 * time.Minute),
					RefreshExpiresAt: time.Now().Add(24 * time.Hour),
				},
			}

			ensureErr := manager.ensureValidToken()
			Expect(ensureErr).ToNot(HaveOccurred())
			Expect(manager.tokens.AccessToken).To(Equal("valid-token"))
		})

		It("refreshes the access token when expired but refresh token is valid", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(schwabTokenResponse{
					AccessToken:  "refreshed-access",
					RefreshToken: "still-valid-refresh",
					ExpiresIn:    1800,
					TokenType:    "Bearer",
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			manager := &tokenManager{
				clientID:     "cid",
				clientSecret: "csecret",
				tokenFile:    tokenPath,
				authBaseURL:  server.URL,
				tokens: &tokenStore{
					AccessToken:      "expired-token",
					RefreshToken:     "valid-refresh",
					AccessExpiresAt:  time.Now().Add(-5 * time.Minute),
					RefreshExpiresAt: time.Now().Add(24 * time.Hour),
				},
			}

			ensureErr := manager.ensureValidToken()
			Expect(ensureErr).ToNot(HaveOccurred())
			Expect(manager.tokens.AccessToken).To(Equal("refreshed-access"))
		})

		It("returns ErrTokenExpired when both tokens are expired", func() {
			manager := &tokenManager{
				tokens: &tokenStore{
					AccessExpiresAt:  time.Now().Add(-5 * time.Minute),
					RefreshExpiresAt: time.Now().Add(-1 * time.Hour),
				},
			}

			ensureErr := manager.ensureValidToken()
			Expect(ensureErr).To(MatchError(ErrTokenExpired))
		})
	})

	Describe("startAuthFlow", func() {
		It("starts HTTPS server and captures callback code", func() {
			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(schwabTokenResponse{
					AccessToken:  "auth-flow-access",
					RefreshToken: "auth-flow-refresh",
					ExpiresIn:    1800,
					TokenType:    "Bearer",
				})
			}))
			DeferCleanup(tokenServer.Close)

			// Create the TLS listener up front so it is bound and accepting
			// at the kernel level before startAuthFlowServer is called.
			// This eliminates the race between server startup and the test
			// client's first connection attempt that caused CI flakes.
			tlsCert, certErr := generateSelfSignedCert()
			Expect(certErr).ToNot(HaveOccurred())

			tlsListener, listenErr := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
				Certificates: []tls.Certificate{tlsCert},
			})
			Expect(listenErr).ToNot(HaveOccurred())
			// No DeferCleanup needed: http.Server.Serve closes the listener on return.

			callbackAddr := tlsListener.Addr().String()

			manager := &tokenManager{
				clientID:     "test-id",
				clientSecret: "test-secret",
				callbackURL:  fmt.Sprintf("https://%s", callbackAddr),
				tokenFile:    tokenPath,
				authBaseURL:  tokenServer.URL,
				tokens:       &tokenStore{},
				testListener: tlsListener,
			}

			authDone := make(chan error, 1)

			go func() {
				_, startErr := manager.startAuthFlowServer()
				authDone <- startErr
			}()

			httpClient := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			}
			callbackReqURL := fmt.Sprintf("https://%s?code=test-auth-code%%40extra", callbackAddr)

			Eventually(func() error {
				resp, getErr := httpClient.Get(callbackReqURL)
				if getErr != nil {
					return getErr
				}
				resp.Body.Close()

				return nil
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

			Eventually(authDone, 5*time.Second).Should(Receive(Not(HaveOccurred())))
			Expect(manager.tokens.AccessToken).To(Equal("auth-flow-access"))
		})
	})
})
