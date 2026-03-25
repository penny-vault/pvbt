package tradier_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/broker/tradier"
)

var _ = Describe("tokenManager", func() {
	Describe("detectAuthMode", func() {
		It("returns authModeStatic when TRADIER_ACCESS_TOKEN is set", func() {
			os.Setenv("TRADIER_ACCESS_TOKEN", "my-static-token")
			DeferCleanup(os.Unsetenv, "TRADIER_ACCESS_TOKEN")

			mode, err := tradier.DetectAuthMode()
			Expect(err).ToNot(HaveOccurred())
			Expect(mode).To(Equal(tradier.AuthModeStatic))
		})

		It("returns authModeOAuth when TRADIER_CLIENT_ID and TRADIER_CLIENT_SECRET are set", func() {
			os.Setenv("TRADIER_CLIENT_ID", "client-id")
			os.Setenv("TRADIER_CLIENT_SECRET", "client-secret")
			DeferCleanup(os.Unsetenv, "TRADIER_CLIENT_ID")
			DeferCleanup(os.Unsetenv, "TRADIER_CLIENT_SECRET")

			mode, err := tradier.DetectAuthMode()
			Expect(err).ToNot(HaveOccurred())
			Expect(mode).To(Equal(tradier.AuthModeOAuth))
		})

		It("returns ErrMissingCredentials when neither env var is set", func() {
			os.Unsetenv("TRADIER_ACCESS_TOKEN")
			os.Unsetenv("TRADIER_CLIENT_ID")
			os.Unsetenv("TRADIER_CLIENT_SECRET")

			_, err := tradier.DetectAuthMode()
			Expect(err).To(MatchError(tradier.ErrMissingCredentials))
		})

		It("gives TRADIER_ACCESS_TOKEN priority over OAuth env vars", func() {
			os.Setenv("TRADIER_ACCESS_TOKEN", "static-wins")
			os.Setenv("TRADIER_CLIENT_ID", "client-id")
			os.Setenv("TRADIER_CLIENT_SECRET", "client-secret")
			DeferCleanup(os.Unsetenv, "TRADIER_ACCESS_TOKEN")
			DeferCleanup(os.Unsetenv, "TRADIER_CLIENT_ID")
			DeferCleanup(os.Unsetenv, "TRADIER_CLIENT_SECRET")

			mode, err := tradier.DetectAuthMode()
			Expect(err).ToNot(HaveOccurred())
			Expect(mode).To(Equal(tradier.AuthModeStatic))
		})
	})

	Describe("Token file persistence", func() {
		It("saves and loads tokens from a file", func() {
			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			store := &tradier.TokenStore{
				AccessToken:     "access-abc",
				RefreshToken:    "refresh-xyz",
				AccessExpiresAt: time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC),
			}

			saveErr := tradier.SaveTokens(tokenPath, store)
			Expect(saveErr).ToNot(HaveOccurred())

			loaded, loadErr := tradier.LoadTokens(tokenPath)
			Expect(loadErr).ToNot(HaveOccurred())
			Expect(loaded.AccessToken).To(Equal("access-abc"))
			Expect(loaded.RefreshToken).To(Equal("refresh-xyz"))
			Expect(loaded.AccessExpiresAt).To(BeTemporally("~", store.AccessExpiresAt, time.Second))
		})

		It("returns an error when the file does not exist", func() {
			_, loadErr := tradier.LoadTokens("/nonexistent/path/tokens.json")
			Expect(loadErr).To(HaveOccurred())
		})

		It("creates parent directories when saving", func() {
			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "sub", "dir", "tokens.json")

			store := &tradier.TokenStore{AccessToken: "test"}
			saveErr := tradier.SaveTokens(tokenPath, store)
			Expect(saveErr).ToNot(HaveOccurred())

			_, statErr := os.Stat(tokenPath)
			Expect(statErr).ToNot(HaveOccurred())
		})
	})

	Describe("Token expiry check", func() {
		It("returns an error when the token is expired and no refresh token is available", func() {
			mgr := tradier.NewTokenManagerForTest(
				tradier.AuthModeOAuth, "cid", "csecret", "", "",
			)
			mgr.SetTokensForTest(&tradier.TokenStore{
				AccessToken:     "expired",
				AccessExpiresAt: time.Now().Add(-10 * time.Minute),
			})

			err := mgr.EnsureValidToken()
			Expect(err).To(MatchError(tradier.ErrTokenExpired))
		})

		It("returns nil when the token is still valid", func() {
			mgr := tradier.NewTokenManagerForTest(
				tradier.AuthModeOAuth, "cid", "csecret", "", "",
			)
			mgr.SetTokensForTest(&tradier.TokenStore{
				AccessToken:     "valid",
				AccessExpiresAt: time.Now().Add(10 * time.Minute),
			})

			err := mgr.EnsureValidToken()
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns nil for static mode regardless of token expiry", func() {
			os.Setenv("TRADIER_ACCESS_TOKEN", "static-token")
			DeferCleanup(os.Unsetenv, "TRADIER_ACCESS_TOKEN")

			mgr := tradier.NewTokenManagerForTest(
				tradier.AuthModeStatic, "", "", "", "",
			)
			mgr.SetTokensForTest(&tradier.TokenStore{
				AccessExpiresAt: time.Now().Add(-1 * time.Hour),
			})

			err := mgr.EnsureValidToken()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("OAuth token exchange", func() {
		It("exchanges an authorization code for tokens via POST with Basic auth", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal("POST"))
				Expect(req.URL.Path).To(Equal("/v1/oauth/accesstoken"))

				parseErr := req.ParseForm()
				Expect(parseErr).ToNot(HaveOccurred())
				Expect(req.Form.Get("grant_type")).To(Equal("authorization_code"))
				Expect(req.Form.Get("code")).To(Equal("auth-code-123"))

				username, password, hasAuth := req.BasicAuth()
				Expect(hasAuth).To(BeTrue())
				Expect(username).To(Equal("test-client-id"))
				Expect(password).To(Equal("test-client-secret"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]interface{}{
					"access_token":  "new-access",
					"refresh_token": "new-refresh",
					"expires_in":    86400,
					"token_type":    "Bearer",
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			mgr := tradier.NewTokenManagerForTest(
				tradier.AuthModeOAuth, "test-client-id", "test-client-secret", "", tokenPath,
			)
			tradier.SetAuthBaseURL(mgr, server.URL)

			exchangeErr := mgr.ExchangeAuthCode(server.URL, "auth-code-123")
			Expect(exchangeErr).ToNot(HaveOccurred())
			Expect(mgr.AccessToken()).To(Equal("new-access"))
		})
	})

	Describe("OAuth token refresh", func() {
		It("refreshes the access token using refresh_token grant with Basic auth", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal("POST"))
				Expect(req.URL.Path).To(Equal("/v1/oauth/refreshtoken"))

				parseErr := req.ParseForm()
				Expect(parseErr).ToNot(HaveOccurred())
				Expect(req.Form.Get("grant_type")).To(Equal("refresh_token"))
				Expect(req.Form.Get("refresh_token")).To(Equal("old-refresh"))

				username, password, hasAuth := req.BasicAuth()
				Expect(hasAuth).To(BeTrue())
				Expect(username).To(Equal("test-client-id"))
				Expect(password).To(Equal("test-client-secret"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]interface{}{
					"access_token":  "refreshed-access",
					"refresh_token": "refreshed-refresh",
					"expires_in":    86400,
					"token_type":    "Bearer",
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			mgr := tradier.NewTokenManagerForTest(
				tradier.AuthModeOAuth, "test-client-id", "test-client-secret", "", tokenPath,
			)
			tradier.SetAuthBaseURL(mgr, server.URL)
			mgr.SetTokensForTest(&tradier.TokenStore{
				RefreshToken:    "old-refresh",
				AccessExpiresAt: time.Now().Add(-10 * time.Minute),
			})

			refreshErr := mgr.RefreshAccessToken()
			Expect(refreshErr).ToNot(HaveOccurred())
			Expect(mgr.AccessToken()).To(Equal("refreshed-access"))
		})

		It("returns an error on HTTP failure", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.WriteHeader(http.StatusUnauthorized)
				writer.Write([]byte(`{"error":"invalid_grant"}`))
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			mgr := tradier.NewTokenManagerForTest(
				tradier.AuthModeOAuth, "test-client-id", "test-client-secret", "", tokenPath,
			)
			tradier.SetAuthBaseURL(mgr, server.URL)
			mgr.SetTokensForTest(&tradier.TokenStore{
				RefreshToken: "bad-refresh",
			})

			refreshErr := mgr.RefreshAccessToken()
			Expect(refreshErr).To(HaveOccurred())
		})
	})

})
