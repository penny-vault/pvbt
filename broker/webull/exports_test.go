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

// ExtractSignatureHeaders reads the HMAC headers from an http.Request for test assertions.
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
