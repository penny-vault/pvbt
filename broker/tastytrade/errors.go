package tastytrade

import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

var (
	ErrNotAuthenticated    = errors.New("tastytrade: not authenticated")
	ErrMissingCredentials  = errors.New("tastytrade: TASTYTRADE_USERNAME and TASTYTRADE_PASSWORD must be set")
	ErrAccountNotFound     = errors.New("tastytrade: no accounts found")
	ErrOrderRejected       = errors.New("tastytrade: order rejected")
	ErrStreamDisconnected  = errors.New("tastytrade: WebSocket disconnected")
	ErrEmptyOrderGroup     = errors.New("tastytrade: SubmitGroup called with no orders")
	ErrNoEntryOrder        = errors.New("tastytrade: OTOCO group has no entry order")
	ErrMultipleEntryOrders = errors.New("tastytrade: OTOCO group has multiple entry orders")
)

// HTTPError represents an HTTP response with a non-2xx status code.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("tastytrade: HTTP %d: %s", e.StatusCode, e.Message)
}

// NewHTTPError creates an HTTPError with the given status code and message.
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{StatusCode: statusCode, Message: message}
}

// IsTransient returns true if the error is a transient failure that should
// be retried (network errors, HTTP 5xx). Returns false for permanent
// failures (HTTP 4xx, order rejections, auth errors).
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	// Check for HTTPError.
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500
	}

	// Check for net.OpError (connection refused, timeouts).
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	// Check for DNS errors.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Check for url.Error wrapping a net error.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return IsTransient(urlErr.Err)
	}

	return false
}
