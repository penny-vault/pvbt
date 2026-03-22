package schwab

import "errors"

var (
	ErrTokenExpired          = errors.New("schwab: refresh token expired, re-authorization required")
	ErrAuthorizationRequired = errors.New("schwab: user must authorize via browser")
	ErrAccountNotFound       = errors.New("schwab: no accounts found")
	ErrLoginDenied           = errors.New("schwab: streamer LOGIN denied")
	ErrStreamDisconnected    = errors.New("schwab: WebSocket disconnected")
)
