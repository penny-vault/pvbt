package etrade

import (
	"errors"

	"github.com/penny-vault/pvbt/broker"
)

// Re-export common broker errors so callers can use etrade.ErrX.
var (
	ErrMissingCredentials  = broker.ErrMissingCredentials
	ErrNotAuthenticated    = broker.ErrNotAuthenticated
	ErrAccountNotFound     = broker.ErrAccountNotFound
	ErrAccountNotActive    = broker.ErrAccountNotActive
	ErrStreamDisconnected  = broker.ErrStreamDisconnected
	ErrEmptyOrderGroup     = broker.ErrEmptyOrderGroup
	ErrNoEntryOrder        = broker.ErrNoEntryOrder
	ErrMultipleEntryOrders = broker.ErrMultipleEntryOrders
	ErrOrderRejected       = broker.ErrOrderRejected
	ErrRateLimited         = broker.ErrRateLimited
)

// Re-export common broker HTTP error utilities.
type HTTPError = broker.HTTPError

var NewHTTPError = broker.NewHTTPError
var IsRetryableError = broker.IsRetryableError

// E*TRADE-specific errors.
var (
	// ErrTokenExpired is returned when the access token has expired
	// (midnight ET daily expiry or 2-hour inactivity timeout).
	ErrTokenExpired = errors.New("etrade: access token expired")

	// ErrPreviewFailed is returned when the order preview step fails.
	ErrPreviewFailed = errors.New("etrade: order preview failed")

	// ErrPreviewExpired is returned when the preview ID has expired
	// (3-minute validity window).
	ErrPreviewExpired = errors.New("etrade: preview expired")
)
