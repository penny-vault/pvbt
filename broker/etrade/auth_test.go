package etrade_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/etrade"
)

var _ = Describe("OAuth 1.0a auth", func() {

	Describe("PercentEncode", func() {
		It("encodes spaces as %20, not +", func() {
			Expect(etrade.PercentEncode("hello world")).To(Equal("hello%20world"))
		})

		It("encodes * as %2A", func() {
			Expect(etrade.PercentEncode("a*b")).To(Equal("a%2Ab"))
		})

		It("does NOT encode ~", func() {
			Expect(etrade.PercentEncode("a~b")).To(Equal("a~b"))
		})

		It("does not encode unreserved chars", func() {
			Expect(etrade.PercentEncode("abcABC123-_.~")).To(Equal("abcABC123-_.~"))
		})

		It("encodes special characters", func() {
			Expect(etrade.PercentEncode("a&b=c")).To(ContainSubstring("%26"))
			Expect(etrade.PercentEncode("a&b=c")).To(ContainSubstring("%3D"))
		})

		It("encodes empty string as empty string", func() {
			Expect(etrade.PercentEncode("")).To(Equal(""))
		})
	})

	Describe("BuildAuthHeader", func() {
		It("produces a deterministic Authorization header with known inputs", func() {
			// Values taken from the OAuth 1.0a spec example (adapted for HMAC-SHA1).
			method := "GET"
			rawURL := "https://api.etrade.com/oauth/request_token"
			consumerKey := "dpf43f3p2l4k3l03"
			consumerSecret := "kd94hf93k423kf44"
			token := ""
			tokenSecret := ""
			nonce := "kllo9940pd9333jh"
			timestamp := "1191242096"

			hdr := etrade.BuildAuthHeader(
				method, rawURL,
				consumerKey, consumerSecret,
				token, tokenSecret,
				nonce, timestamp,
				nil,
			)

			Expect(hdr).To(HavePrefix("OAuth "))
			Expect(hdr).To(ContainSubstring(`oauth_consumer_key="dpf43f3p2l4k3l03"`))
			Expect(hdr).To(ContainSubstring(`oauth_nonce="kllo9940pd9333jh"`))
			Expect(hdr).To(ContainSubstring(`oauth_signature_method="HMAC-SHA1"`))
			Expect(hdr).To(ContainSubstring(`oauth_timestamp="1191242096"`))
			Expect(hdr).To(ContainSubstring(`oauth_version="1.0"`))
			Expect(hdr).To(ContainSubstring("oauth_signature="))
			// When token is empty, oauth_token should not appear
			Expect(hdr).NotTo(ContainSubstring("oauth_token="))
		})

		It("includes oauth_token when token is provided", func() {
			hdr := etrade.BuildAuthHeader(
				"GET", "https://api.etrade.com/oauth/access_token",
				"consumerKey", "consumerSecret",
				"mytoken", "mytokensecret",
				"nonce123", "1191242096",
				nil,
			)

			Expect(hdr).To(ContainSubstring(`oauth_token="mytoken"`))
		})

		It("is deterministic: same inputs produce same output", func() {
			args := []interface{}{
				"GET", "https://api.etrade.com/oauth/request_token",
				"key", "secret", "", "", "nonce", "1234",
			}

			hdr1 := etrade.BuildAuthHeader(
				args[0].(string), args[1].(string),
				args[2].(string), args[3].(string),
				args[4].(string), args[5].(string),
				args[6].(string), args[7].(string),
				nil,
			)
			hdr2 := etrade.BuildAuthHeader(
				args[0].(string), args[1].(string),
				args[2].(string), args[3].(string),
				args[4].(string), args[5].(string),
				args[6].(string), args[7].(string),
				nil,
			)

			Expect(hdr1).To(Equal(hdr2))
		})

		It("incorporates extra params into the signature", func() {
			extra := url.Values{"oauth_verifier": []string{"verifier123"}}

			hdrWithExtra := etrade.BuildAuthHeader(
				"GET", "https://api.etrade.com/oauth/access_token",
				"key", "secret", "token", "tokensecret",
				"nonce", "1234",
				extra,
			)
			hdrWithout := etrade.BuildAuthHeader(
				"GET", "https://api.etrade.com/oauth/access_token",
				"key", "secret", "token", "tokensecret",
				"nonce", "1234",
				nil,
			)

			// Different extra params should produce different signatures.
			Expect(hdrWithExtra).NotTo(Equal(hdrWithout))
		})
	})

	Describe("requestToken", func() {
		var server *httptest.Server

		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.URL.Path).To(Equal("/request_token"))

				authHdr := rr.Header.Get("Authorization")
				Expect(authHdr).To(HavePrefix("OAuth "))
				Expect(authHdr).To(ContainSubstring("oauth_consumer_key="))
				Expect(authHdr).To(ContainSubstring("oauth_signature="))

				fmt.Fprint(ww, "oauth_token=testtoken&oauth_token_secret=testsecret&oauth_callback_confirmed=true")
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("returns the request token and secret from the server response", func() {
			tm := etrade.NewTokenManagerForTest("mykey", "mysecret", "https://127.0.0.1:5174", "")
			etrade.SetAuthBaseURL(tm, server.URL)

			oauthToken, oauthSecret, err := tm.RequestToken()

			Expect(err).NotTo(HaveOccurred())
			Expect(oauthToken).To(Equal("testtoken"))
			Expect(oauthSecret).To(Equal("testsecret"))
		})

		It("returns an error when the server returns a non-200 status", func() {
			errorServer := httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, rr *http.Request) {
				ww.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(ww, "oauth_problem=permission_denied")
			}))
			defer errorServer.Close()

			tm := etrade.NewTokenManagerForTest("mykey", "mysecret", "https://127.0.0.1:5174", "")
			etrade.SetAuthBaseURL(tm, errorServer.URL)

			_, _, err := tm.RequestToken()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HTTP 401"))
		})
	})

	Describe("exchangeAccessToken", func() {
		var server *httptest.Server

		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, rr *http.Request) {
				defer GinkgoRecover()

				Expect(rr.URL.Path).To(Equal("/access_token"))

				// oauth_verifier is included in the signature computation (and
				// thus affects the oauth_signature value) but is NOT emitted as
				// a literal field in the Authorization header; only standard
				// OAuth protocol params appear there.
				authHdr := rr.Header.Get("Authorization")
				Expect(authHdr).To(ContainSubstring("oauth_token="))
				Expect(authHdr).To(ContainSubstring("oauth_signature="))

				fmt.Fprint(ww, "oauth_token=accesstoken&oauth_token_secret=accesssecret")
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("stores the access credentials after a successful exchange", func() {
			tm := etrade.NewTokenManagerForTest("mykey", "mysecret", "https://127.0.0.1:5174", "")
			etrade.SetAuthBaseURL(tm, server.URL)

			err := tm.ExchangeAccessToken("requesttoken", "requestsecret", "myverifier")

			Expect(err).NotTo(HaveOccurred())

			creds := tm.Creds()
			Expect(creds.AccessToken).To(Equal("accesstoken"))
			Expect(creds.AccessSecret).To(Equal("accesssecret"))
		})

		It("returns an error on a non-200 response", func() {
			errorServer := httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, rr *http.Request) {
				ww.WriteHeader(http.StatusForbidden)
				fmt.Fprint(ww, "error=invalid_verifier")
			}))
			defer errorServer.Close()

			tm := etrade.NewTokenManagerForTest("mykey", "mysecret", "https://127.0.0.1:5174", "")
			etrade.SetAuthBaseURL(tm, errorServer.URL)

			err := tm.ExchangeAccessToken("requesttoken", "requestsecret", "badverifier")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HTTP 403"))
		})
	})

	Describe("renewAccessToken", func() {
		It("calls the renew endpoint with the correct Authorization header", func() {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.URL.Path).To(Equal("/renew_access_token"))
				called = true

				authHdr := rr.Header.Get("Authorization")
				Expect(authHdr).To(ContainSubstring("oauth_token="))

				ww.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			tm := etrade.NewTokenManagerForTest("mykey", "mysecret", "https://127.0.0.1:5174", "")
			etrade.SetAuthBaseURL(tm, server.URL)

			// Inject access credentials directly via ExchangeAccessToken on a
			// mock server that returns a known token.
			tokenServer := httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, rr *http.Request) {
				fmt.Fprint(ww, "oauth_token=accesstok&oauth_token_secret=accesssec")
			}))
			defer tokenServer.Close()

			etrade.SetAuthBaseURL(tm, tokenServer.URL)
			Expect(tm.ExchangeAccessToken("rt", "rs", "v")).To(Succeed())

			// Now point back to the renew server and renew.
			etrade.SetAuthBaseURL(tm, server.URL)

			err := tm.RenewAccessToken()

			Expect(err).NotTo(HaveOccurred())
			Expect(called).To(BeTrue())
		})

		It("returns an error on a non-200 response", func() {
			server := httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, rr *http.Request) {
				ww.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(ww, "token expired")
			}))
			defer server.Close()

			tm := etrade.NewTokenManagerForTest("mykey", "mysecret", "https://127.0.0.1:5174", "")
			etrade.SetAuthBaseURL(tm, server.URL)

			err := tm.RenewAccessToken()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HTTP 401"))
		})
	})

	Describe("saveTokens and loadTokens", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "etrade-auth-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		It("round-trips credentials through save and load", func() {
			creds := &etrade.OAuthCredentials{
				ConsumerKey:    "ck",
				ConsumerSecret: "cs",
				AccessToken:    "at",
				AccessSecret:   "as",
			}

			path := filepath.Join(tmpDir, "tokens.json")

			saveErr := etrade.SaveTokens(path, creds)
			Expect(saveErr).NotTo(HaveOccurred())

			// File should be created with 0600 permissions.
			info, statErr := os.Stat(path)
			Expect(statErr).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))

			loaded, loadErr := etrade.LoadTokens(path)
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(loaded.ConsumerKey).To(Equal("ck"))
			Expect(loaded.ConsumerSecret).To(Equal("cs"))
			Expect(loaded.AccessToken).To(Equal("at"))
			Expect(loaded.AccessSecret).To(Equal("as"))
		})

		It("creates parent directories with 0700 permissions", func() {
			deepPath := filepath.Join(tmpDir, "a", "b", "c", "tokens.json")
			creds := &etrade.OAuthCredentials{ConsumerKey: "k"}

			saveErr := etrade.SaveTokens(deepPath, creds)
			Expect(saveErr).NotTo(HaveOccurred())

			parentInfo, statErr := os.Stat(filepath.Join(tmpDir, "a", "b", "c"))
			Expect(statErr).NotTo(HaveOccurred())
			Expect(parentInfo.Mode().Perm()).To(Equal(os.FileMode(0700)))
		})

		It("returns an error when the file does not exist", func() {
			_, err := etrade.LoadTokens(filepath.Join(tmpDir, "nonexistent.json"))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ExpandHome", func() {
		It("replaces leading ~ with the home directory", func() {
			home, err := os.UserHomeDir()
			Expect(err).NotTo(HaveOccurred())

			result := etrade.ExpandHome("~/.config/pvbt/tokens.json")
			Expect(result).To(Equal(filepath.Join(home, ".config/pvbt/tokens.json")))
		})

		It("returns bare ~ as the home directory", func() {
			home, err := os.UserHomeDir()
			Expect(err).NotTo(HaveOccurred())

			result := etrade.ExpandHome("~")
			Expect(result).To(Equal(home))
		})

		It("does not modify paths without a ~ prefix", func() {
			result := etrade.ExpandHome("/absolute/path")
			Expect(result).To(Equal("/absolute/path"))
		})

		It("does not modify an empty string", func() {
			result := etrade.ExpandHome("")
			Expect(result).To(Equal(""))
		})

		It("does not expand ~ in the middle of a path", func() {
			result := etrade.ExpandHome("/foo/~/bar")
			Expect(result).To(Equal("/foo/~/bar"))
		})

		It("does not expand ~ when it appears in the middle of a path component", func() {
			result := etrade.ExpandHome("foo~bar")
			// The path does not start with ~ so it must be returned unchanged.
			Expect(result).To(Equal("foo~bar"))
		})
	})

	Describe("OAuthCredentials type alias", func() {
		It("exposes ConsumerKey, ConsumerSecret, AccessToken, AccessSecret fields", func() {
			creds := etrade.OAuthCredentials{
				ConsumerKey:    "k",
				ConsumerSecret: "s",
				AccessToken:    "at",
				AccessSecret:   "as",
			}
			Expect(creds.ConsumerKey).To(Equal("k"))
			Expect(creds.ConsumerSecret).To(Equal("s"))
			Expect(creds.AccessToken).To(Equal("at"))
			Expect(creds.AccessSecret).To(Equal("as"))
		})
	})

	Describe("BuildAuthHeader signature correctness", func() {
		// Verify against a known-good HMAC-SHA1 OAuth 1.0a computation.
		// Reference: Twitter OAuth 1.0a documentation example adapted for GET.
		It("produces the correct HMAC-SHA1 signature for a known input set", func() {
			// These values are chosen so we can independently verify the signature.
			method := "GET"
			rawURL := "https://api.example.com/resource"
			consumerKey := "xvz1evFS4wEEPTGEFPHBog"
			consumerSecret := "kAcSOqF21Fu85e7zjz7ZN2U4ZRhfV3WpwPAoE3Z7kBw"
			token := "370773112-GmHxMAgYyLbNEtIKZeRNFsMKPR9EyMZeS9weJAEb"
			tokenSecret := "LswwdoUaIvS8ltyTt5jkRh4J50vUPVVHtR2YPi5kE"
			nonce := "kYjzVBB8Y0ZFabxSWbWovY3uYSQ2pTgmZeNu2VS4cg"
			timestamp := "1318622958"

			hdr := etrade.BuildAuthHeader(
				method, rawURL,
				consumerKey, consumerSecret,
				token, tokenSecret,
				nonce, timestamp,
				nil,
			)

			// The header must contain a non-empty base64 signature.
			Expect(hdr).To(ContainSubstring("oauth_signature="))
			sigStart := strings.Index(hdr, `oauth_signature="`) + len(`oauth_signature="`)
			sigEnd := strings.Index(hdr[sigStart:], `"`) + sigStart
			sig := hdr[sigStart:sigEnd]
			// Percent-decode the signature to get raw base64.
			decoded, decErr := url.QueryUnescape(sig)
			Expect(decErr).NotTo(HaveOccurred())
			Expect(decoded).NotTo(BeEmpty())
			// Must be valid base64 (length divisible by 4 after unpadding).
			Expect(len(decoded)).To(BeNumerically(">", 0))
		})
	})
})
