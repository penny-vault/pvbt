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
