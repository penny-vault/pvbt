// Package ibkr implements broker.Broker for the Interactive Brokers brokerage.
//
// This broker integrates with IB's REST Web API for order management and
// WebSocket streaming for real-time fill delivery. Two authentication backends
// are supported: OAuth (for users with registered consumer keys) and the Client
// Portal Gateway (for everyone else).
//
// # Authentication
//
// Use WithOAuth for OAuth authentication:
//
//   - IBKR_CONSUMER_KEY: OAuth consumer key from IB Self Service Portal
//   - IBKR_SIGNING_KEY_FILE: Path to RSA signing key (PEM, PKCS#8)
//   - IBKR_ACCESS_TOKEN: Pre-existing access token (optional)
//   - IBKR_ACCESS_TOKEN_SECRET: Pre-existing access token secret (optional)
//
// Use WithGateway for Client Portal Gateway authentication:
//
//   - IBKR_GATEWAY_URL: Gateway URL (default: https://localhost:5000)
//
// # Fill Delivery
//
// Fills are delivered via a WebSocket connection. On disconnect, the broker
// reconnects with exponential backoff and polls for missed fills. Duplicate
// fills are suppressed automatically.
//
// # Order Types
//
// Market, Limit, Stop, and StopLimit orders are supported. GTD and FOK
// time-in-force values are not supported and return an error. Dollar-amount
// orders (Qty=0, Amount>0) are converted to share quantities by fetching a
// real-time quote.
//
// # Order Groups
//
// IBBroker implements broker.GroupSubmitter for native bracket and OCA order
// support.
//
// # Usage
//
//	import "github.com/penny-vault/pvbt/broker/ibkr"
//
//	ib := ibkr.New(ibkr.WithGateway("localhost:5000"))
//	eng := engine.New(&MyStrategy{},
//	    engine.WithBroker(ib),
//	)
package ibkr
