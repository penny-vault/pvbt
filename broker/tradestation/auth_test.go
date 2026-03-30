package tradestation

import (
	"github.com/bytedance/sonic"
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
				AccessToken:     "access-abc",
				RefreshToken:    "refresh-xyz",
				AccessExpiresAt: time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC),
			}

			saveErr := saveTokens(tokenPath, store)
			Expect(saveErr).ToNot(HaveOccurred())

			loaded, loadErr := loadTokens(tokenPath)
			Expect(loadErr).ToNot(HaveOccurred())
			Expect(loaded.AccessToken).To(Equal("access-abc"))
			Expect(loaded.RefreshToken).To(Equal("refresh-xyz"))
			Expect(loaded.AccessExpiresAt).To(BeTemporally("~", store.AccessExpiresAt, time.Second))
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

	Describe("Token expiration", func() {
		It("reports an expired access token", func() {
			manager := newTokenManager("id", "secret", "", "")
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(-10 * time.Minute),
			}

			Expect(manager.accessTokenExpired()).To(BeTrue())
		})

		It("reports a valid access token", func() {
			manager := newTokenManager("id", "secret", "", "")
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(10 * time.Minute),
			}

			Expect(manager.accessTokenExpired()).To(BeFalse())
		})

		It("accounts for the 5-minute buffer", func() {
			manager := newTokenManager("id", "secret", "", "")
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(3 * time.Minute),
			}

			Expect(manager.accessTokenExpired()).To(BeTrue())
		})
	})

	Describe("Token refresh", func() {
		It("exchanges a refresh token for a new access token", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/oauth/token"))
				Expect(req.FormValue("grant_type")).To(Equal("refresh_token"))
				Expect(req.FormValue("refresh_token")).To(Equal("refresh-old"))

				writer.Header().Set("Content-Type", "application/json")
				sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
					"access_token":  "access-new",
					"refresh_token": "refresh-new",
					"expires_in":    1200,
					"token_type":    "Bearer",
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			manager := newTokenManager("client-id", "client-secret", "", tokenPath)
			manager.authBaseURL = server.URL
			manager.tokens = &tokenStore{
				RefreshToken: "refresh-old",
			}

			refreshErr := manager.refreshAccessToken()
			Expect(refreshErr).ToNot(HaveOccurred())
			Expect(manager.tokens.AccessToken).To(Equal("access-new"))
			Expect(manager.tokens.RefreshToken).To(Equal("refresh-new"))
			Expect(manager.tokens.AccessExpiresAt).To(BeTemporally(">", time.Now()))
		})
	})

	Describe("Auth code exchange", func() {
		It("exchanges an authorization code for tokens", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/oauth/token"))
				Expect(req.FormValue("grant_type")).To(Equal("authorization_code"))
				Expect(req.FormValue("code")).To(Equal("auth-code-123"))
				Expect(req.FormValue("redirect_uri")).ToNot(BeEmpty())

				writer.Header().Set("Content-Type", "application/json")
				sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
					"access_token":  "access-from-code",
					"refresh_token": "refresh-from-code",
					"expires_in":    1200,
					"token_type":    "Bearer",
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			manager := newTokenManager("client-id", "client-secret", "https://127.0.0.1:5174", tokenPath)
			manager.authBaseURL = server.URL

			exchangeErr := manager.exchangeAuthCode("auth-code-123")
			Expect(exchangeErr).ToNot(HaveOccurred())
			Expect(manager.tokens.AccessToken).To(Equal("access-from-code"))
			Expect(manager.tokens.RefreshToken).To(Equal("refresh-from-code"))
		})
	})

	Describe("ensureValidToken", func() {
		It("returns nil when access token is still valid", func() {
			manager := newTokenManager("id", "secret", "", "")
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(10 * time.Minute),
			}

			Expect(manager.ensureValidToken()).To(Succeed())
		})

		It("refreshes when access token is expired", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
					"access_token":  "refreshed",
					"refresh_token": "refresh-new",
					"expires_in":    1200,
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			manager := newTokenManager("id", "secret", "", tokenPath)
			manager.authBaseURL = server.URL
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(-1 * time.Minute),
				RefreshToken:    "valid-refresh",
			}

			Expect(manager.ensureValidToken()).To(Succeed())
			Expect(manager.tokens.AccessToken).To(Equal("refreshed"))
		})

		It("returns ErrTokenExpired when refresh token is empty", func() {
			manager := newTokenManager("id", "secret", "", "")
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(-1 * time.Minute),
				RefreshToken:    "",
			}

			ensureErr := manager.ensureValidToken()
			Expect(ensureErr).To(MatchError(ErrTokenExpired))
		})
	})
})
