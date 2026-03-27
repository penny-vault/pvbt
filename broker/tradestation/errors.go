package tradestation

import "errors"

var (
	ErrTokenExpired          = errors.New("tradestation: refresh token expired, re-authorization required")
	ErrAuthorizationRequired = errors.New("tradestation: user must authorize via browser")
	ErrAccountNotFound       = errors.New("tradestation: no accounts found")
	ErrStreamDisconnected    = errors.New("tradestation: order stream disconnected")
)
