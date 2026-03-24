package tradier

import (
	"errors"

	"github.com/penny-vault/pvbt/broker"
)

var (
	ErrMissingCredentials  = broker.ErrMissingCredentials
	ErrNotAuthenticated    = broker.ErrNotAuthenticated
	ErrAccountNotFound     = broker.ErrAccountNotFound
	ErrOrderRejected       = broker.ErrOrderRejected
	ErrStreamDisconnected  = broker.ErrStreamDisconnected
	ErrEmptyOrderGroup     = broker.ErrEmptyOrderGroup
	ErrNoEntryOrder        = broker.ErrNoEntryOrder
	ErrMultipleEntryOrders = broker.ErrMultipleEntryOrders
	ErrTokenExpired        = errors.New("tradier: access token expired, re-authorization required")
)

// HTTPError is a type alias for broker.HTTPError.
type HTTPError = broker.HTTPError

// NewHTTPError creates an HTTPError with the given status code and message.
var NewHTTPError = broker.NewHTTPError

// IsRetryableError returns true if the error is a transient failure that should be retried.
var IsRetryableError = broker.IsRetryableError
