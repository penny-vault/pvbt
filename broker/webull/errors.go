package webull

import (
	"errors"

	"github.com/penny-vault/pvbt/broker"
)

var (
	// ErrUnsupportedTimeInForce is returned when the order uses a TIF that
	// Webull does not support (anything other than Day or GTC).
	ErrUnsupportedTimeInForce = errors.New("webull: unsupported time-in-force; only Day and GTC are supported")

	// ErrFractionalNotMarket is returned when a dollar-amount order uses a
	// non-market order type. Webull only supports fractional shares on market orders.
	ErrFractionalNotMarket = errors.New("webull: dollar-amount orders require market order type")

	// ErrFractionalNotEnabled is returned when a dollar-amount order is submitted
	// but WithFractionalShares() was not set.
	ErrFractionalNotEnabled = errors.New("webull: dollar-amount orders require WithFractionalShares option")

	// ErrReplaceFieldNotAllowed is returned when a replace order attempts to
	// change a field that Webull does not allow (side, TIF, order type).
	ErrReplaceFieldNotAllowed = errors.New("webull: replace may only change qty and price; side, time-in-force, and order type must match the original")
)

// Re-export common broker errors for use in tests.
var (
	ErrMissingCredentials = broker.ErrMissingCredentials
	ErrNotAuthenticated   = broker.ErrNotAuthenticated
	ErrAccountNotFound    = broker.ErrAccountNotFound
	ErrStreamDisconnected = broker.ErrStreamDisconnected
	ErrRateLimited        = broker.ErrRateLimited
	ErrOrderRejected      = broker.ErrOrderRejected

	// ErrTokenExpired is returned when the access token has expired and no
	// refresh token is available.
	ErrTokenExpired = errors.New("webull: access token expired, re-authorization required")
)
