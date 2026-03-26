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

// NewSigner creates the appropriate signer based on the detected auth mode.
// It is called by the broker facade during Connect.
func NewSigner() (signer, error) {
	mode, modeErr := detectAuthMode()
	if modeErr != nil {
		return nil, modeErr
	}

	switch mode {
	case authModeDirect:
		return &hmacSigner{
			appKey:    os.Getenv("WEBULL_APP_KEY"),
			appSecret: os.Getenv("WEBULL_APP_SECRET"),
		}, nil
	case authModeOAuth:
		// OAuth signer is implemented in a later task.
		return nil, fmt.Errorf("webull: oauth signer not yet implemented")
	default:
		return nil, fmt.Errorf("webull: unknown auth mode %d", mode)
	}
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
