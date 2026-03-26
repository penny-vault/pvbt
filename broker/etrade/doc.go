// Package etrade implements broker.Broker for E*TRADE (Morgan Stanley).
//
// E*TRADE provides a REST API for equities trading. This broker supports
// equities only. Strategies that work with the SimulatedBroker require no
// changes to run live -- swap the broker via engine.WithBroker(etrade.New()).
//
// # Authentication
//
// The broker uses OAuth 1.0a. Set ETRADE_CONSUMER_KEY and
// ETRADE_CONSUMER_SECRET. On first run, Connect() prints an authorization
// URL. Access tokens expire at midnight US Eastern time every day; the
// broker renews the token every 90 minutes to prevent the 2-hour
// inactivity timeout.
//
// ETRADE_ACCOUNT_ID_KEY is required and must be the accountIdKey (not
// the display account number) from the List Accounts response.
//
// # Sandbox
//
// Use WithSandbox() to target the E*TRADE sandbox environment:
//
//	broker := etrade.New(etrade.WithSandbox())
//
// # Fill Delivery
//
// Fills are detected by polling the orders endpoint every 2 seconds.
// Duplicate fills are suppressed automatically.
//
// # Order Types
//
// Market, Limit, Stop, and StopLimit are supported. Duration supports
// Day, GTC, GTD, IOC, and FOK. Dollar-amount orders (Qty=0, Amount>0)
// are converted to share quantities by fetching a real-time quote.
//
// # Order Groups
//
// E*TRADE's API does not support contingent orders (OCO, bracket).
// The account layer manages group cancellation for this broker.
package etrade
