package broker

import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

var (
	ErrMissingCredentials  = errors.New("broker: missing credentials")
	ErrNotAuthenticated    = errors.New("broker: not authenticated")
	ErrAccountNotFound     = errors.New("broker: account not found")
	ErrAccountNotActive    = errors.New("broker: account not active")
	ErrStreamDisconnected  = errors.New("broker: stream disconnected")
	ErrEmptyOrderGroup     = errors.New("broker: empty order group")
	ErrNoEntryOrder        = errors.New("broker: no entry order in group")
	ErrMultipleEntryOrders = errors.New("broker: multiple entry orders in group")
	ErrOrderRejected       = errors.New("broker: order rejected")
)

// HTTPError represents an HTTP response with a non-2xx status code.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (httpErr *HTTPError) Error() string {
	return fmt.Sprintf("broker: HTTP %d: %s", httpErr.StatusCode, httpErr.Message)
}

// NewHTTPError creates an HTTPError with the given status code and message.
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{StatusCode: statusCode, Message: message}
}

// IsTransient returns true if the error is a transient failure that should
// be retried (network errors, HTTP 429, HTTP 5xx). Returns false for permanent
// failures (HTTP 4xx, order rejections, auth errors).
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 429 || httpErr.StatusCode >= 500
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return IsTransient(urlErr.Err)
	}

	return false
}
