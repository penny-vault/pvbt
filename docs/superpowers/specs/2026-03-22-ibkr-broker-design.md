# Interactive Brokers Broker Design Spec

## Summary

Implement the `broker.Broker` and `broker.GroupSubmitter` interfaces for Interactive Brokers, enabling live trading through IB's REST Web API. The broker supports two authentication backends -- OAuth (for users with registered consumer keys) and the Client Portal Gateway (for everyone else) -- behind a shared `Authenticator` interface. A small cross-broker refactor introduces `broker.ErrRateLimited` and renames `broker.IsTransient` to `broker.IsRetryableError`.

## Motivation

Issue #15. Interactive Brokers is the institutional standard with the broadest market access. Each broker integration makes pvbt useful to a different group of traders.

## Cross-Broker Refactors

These changes affect the shared `broker/` package and all existing broker implementations (Schwab, Alpaca, tastytrade):

1. **New sentinel `broker.ErrRateLimited`** in `broker/errors.go`. Returned when the broker API responds with HTTP 429.
2. **Rename `broker.IsTransient` to `broker.IsRetryableError`** across the codebase. Update all call sites in `broker/schwab/client.go`, `broker/alpaca/client.go`, `broker/tastytrade/client.go`, and all corresponding tests.

`broker.IsRetryableError` returns true for `broker.ErrRateLimited`, 5xx `HTTPError` values, and network errors (same logic as today, plus the new sentinel).

## Package Structure

New package `broker/ibkr/` with the following layout:

| File | Purpose |
|------|---------|
| `broker.go` | `IBBroker` struct implementing `broker.Broker` + `broker.GroupSubmitter` |
| `auth.go` | `Authenticator` interface, `OAuthAuthenticator`, `GatewayAuthenticator` |
| `client.go` | REST API client wrapping IB's Web API endpoints (go-resty/v2) |
| `streamer.go` | WebSocket streamer for order/fill updates |
| `types.go` | Request/response structs, mapping functions between IB and broker types |
| `errors.go` | IB-specific error (`ErrConidNotFound`), uses shared broker errors |
| `doc.go` | Package documentation |

## Authentication

An `Authenticator` interface abstracts the two auth backends:

```go
type Authenticator interface {
    Init(ctx context.Context) error
    Decorate(req *http.Request) error
    Keepalive(ctx context.Context)
    Close() error
}
```

### Constructor Options

```go
ibkr.New(ibkr.WithOAuth(ibkr.OAuthConfig{KeyFile: "...", ConsumerKey: "..."}))
ibkr.New(ibkr.WithGateway("localhost:5000"))
```

Exactly one auth option must be provided; `New` returns an error otherwise.

### OAuthAuthenticator

Implements IB's OAuth flow using `private_key_jwt` (RFC 7521/7523):

1. Load signing key from file
2. Request token via `POST /oauth/request_token` (RSA-SHA256 signed)
3. Exchange for access token via `POST /oauth/access_token`
4. Establish live session token via `POST /oauth/live_session_token` (Diffie-Hellman challenge/response)
5. Sign each outbound request with HMAC-SHA256 using the live session token
6. Background goroutine refreshes the live session token before expiry

Environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `IBKR_CONSUMER_KEY` | yes | OAuth consumer key from IB Self Service Portal |
| `IBKR_SIGNING_KEY_FILE` | yes | Path to RSA signing key (PEM, PKCS#8) |
| `IBKR_ACCESS_TOKEN` | no | Pre-existing access token (skip request token flow) |
| `IBKR_ACCESS_TOKEN_SECRET` | no | Pre-existing access token secret |

### GatewayAuthenticator

Connects to a running IB Client Portal Gateway process:

1. Call `POST /iserver/auth/status` to verify active session
2. If not authenticated, call `POST /iserver/reauthenticate` (user must have logged in via the gateway's web UI)
3. Background `/tickle` every 55 seconds to keep the session alive
4. `Decorate` is a no-op (gateway uses session cookies managed by the HTTP client's cookie jar)

Environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `IBKR_GATEWAY_URL` | no | Gateway URL (default: `https://localhost:5000`) |

## REST Client

Uses go-resty/v2 with:
- Authenticator's `Decorate` called on each request via a request middleware
- `rate.Limiter` from `golang.org/x/time/rate` set to 10 requests/second (IB's global limit)
- Retry on `broker.IsRetryableError` with 3 attempts and exponential backoff
- 15-second request timeout

Base URL:
- OAuth: `https://api.ibkr.com/v1/api`
- Gateway: `https://localhost:5000/v1/portal`

## Broker Methods

### Connect

1. Initialize authenticator (`Init`)
2. Start authenticator keepalive goroutine
3. Resolve account ID via `GET /accounts`
4. Start WebSocket streamer
5. Start rate limiter

### Submit

1. If order has ticker but no conid, resolve via `GET /secdef?type=STK&symbol={ticker}&currency=USD`. Cache resolved conids in-memory for the session.
2. Map `broker.Order` to IB order request (see field mapping below)
3. `POST /accounts/{id}/orders` with single-element array
4. If gateway path returns a confirmation prompt, auto-confirm via `POST /iserver/reply/{replyId}` with `{confirmed: true}`
5. Return `broker.ErrOrderRejected` if the order is rejected after confirmation

### Cancel

`DELETE /accounts/{id}/orders/{cOID}`

### Replace

`PUT /accounts/{id}/orders/{cOID}` with full replacement order body. Maps `OrigCustomerOrderId` to the existing order's `cOID`.

### Orders

`GET /accounts/{id}/orders` -- maps IB order status enum to `broker.OrderStatus`.

### Positions

`GET /accounts/{id}/positions` -- maps to `broker.Position` (Qty, AvgOpenPrice from AverageCost, ContractId).

### Balance

`GET /accounts/{id}/summary` -- maps Ledger/Summary fields to `broker.Balance` (CashBalance, NetLiquidatingValue, EquityBuyingPower from BuyingPower, MaintenanceReq from MaintMarginReq).

### Close

1. Stop WebSocket streamer
2. Stop authenticator keepalive
3. Close authenticator
4. Close fills channel

## Order Field Mapping

| broker type | IB API field | Values |
|---|---|---|
| `OrderType` Market | `OrderType` | 1 |
| `OrderType` Limit | `OrderType` | 2 |
| `OrderType` Stop | `OrderType` | 3 |
| `OrderType` StopLimit | `OrderType` | 4 |
| `Side` Buy | `Side` | 1 |
| `Side` Sell | `Side` | 2 |
| `TimeInForce` Day | `TimeInForce` | 0 |
| `TimeInForce` GTC | `TimeInForce` | 1 |
| `TimeInForce` IOC | `TimeInForce` | 3 |
| `TimeInForce` OnOpen | `TimeInForce` | 2 |
| `TimeInForce` OnClose | `TimeInForce` | 7 |
| `Qty` | `Quantity` | float64 |
| `LimitPrice` | `Price` | float64 |
| `StopPrice` | `AuxPrice` | float64 |

## Order Status Mapping

| IB Status | broker.OrderStatus |
|---|---|
| New (0), PendingNew (A) | OrderSubmitted |
| PartiallyFilled (1) | OrderPartiallyFilled |
| Filled (2) | OrderFilled |
| Canceled (4), Expired (C) | OrderCancelled |
| Rejected (8) | OrderCancelled |
| Replaced (5), PendingCancelReplace (6), PendingReplace (E) | OrderSubmitted |

## GroupSubmitter

### Bracket Orders (GroupBracket)

Submit parent + children as an array to `POST /accounts/{id}/orders`:

1. Find the entry-role order; return `broker.ErrNoEntryOrder` or `broker.ErrMultipleEntryOrders` on validation failure
2. Assign a `cOID` to the entry order
3. Set `parentId` on stop-loss and take-profit children referencing the entry's `cOID`
4. Submit the full array

### OCA Orders (GroupOCO)

Submit all orders as an array with `isSingleGroup: true` on each order. IB cancels remaining orders when one fills.

## WebSocket Streamer

**Endpoint:**
- OAuth: `wss://api.ibkr.com/v1/api/ws`
- Gateway: `wss://localhost:5000/v1/api/ws`

**Protocol:**
- Send heartbeat (`tic`) every 10 seconds
- Subscribe to order updates: `sor+{}`
- Subscribe to trade updates: `str+{}`
- Messages arrive as JSON with topic prefixes

**Fill delivery:** When the streamer receives a fill or partial-fill event, it maps to a `broker.Fill` (OrderID, Price, Qty, FilledAt) and sends on the buffered (1024) fills channel.

**Reconnection:** Exponential backoff (1s, 2s, 4s), max 3 attempts. On successful reconnect:
1. Re-subscribe to order and trade topics
2. Poll `GET /accounts/{id}/trades` for fills missed during disconnect
3. Deduplicate by order ID + fill timestamp before sending to fills channel

## Contract ID Resolution

IB uses `conid` (contract ID) rather than ticker symbols for all order operations. The broker resolves tickers to conids on demand:

1. On `Submit`, if the order specifies a ticker but no conid, call `GET /secdef?type=STK&symbol={ticker}&currency=USD`
2. Cache the result in an in-memory map (`map[string]int64`, keyed by `"{symbol}-{currency}"`)
3. Cache lives for the session duration (cleared on `Close`)
4. Return `ErrConidNotFound` if the symbol cannot be resolved

## Dollar-Amount Orders

IB does not support notional orders. When a `broker.Order` specifies an amount instead of quantity:

1. Fetch a quote via `GET /marketdata/snapshot` for the order's conid
2. Compute quantity: `math.Floor(amount / lastPrice)`
3. Submit with the computed quantity

## Rate Limiting

IB enforces a global limit of 10 requests per second. Exceeding this results in HTTP 429 and a 15-minute IP penalty box.

The client uses `golang.org/x/time/rate` with a limit of 9 req/s (leaving headroom) and burst of 1. All API calls acquire a token before sending. On 429, the client returns `broker.ErrRateLimited` (which `broker.IsRetryableError` considers retryable).

## Errors

| Condition | Error |
|---|---|
| Ticker cannot resolve to conid | `ibkr.ErrConidNotFound` |
| Session/token expired | `broker.ErrNotAuthenticated` |
| 429 rate limited | `broker.ErrRateLimited` |
| Order rejected | `broker.ErrOrderRejected` |
| No credentials provided | `broker.ErrMissingCredentials` |
| No auth option specified | error from `New` constructor |
| WebSocket disconnected | `broker.ErrStreamDisconnected` |
| Bracket missing entry | `broker.ErrNoEntryOrder` |
| Bracket multiple entries | `broker.ErrMultipleEntryOrders` |

All errors are wrapped with context via `fmt.Errorf("context: %w", err)`.

## Testing

- **Unit tests:** httptest server mocking REST endpoints (order CRUD, positions, balance, secdef resolution, dollar-amount quote fetch). WebSocket tests with httptest server for streamer (fill delivery, reconnection, heartbeat, deduplication).
- **Authenticator tests:** OAuth signature computation, token refresh lifecycle. Gateway session status check, tickle keepalive timing.
- **Integration tests:** Gated behind `IBKR_INTEGRATION_TEST=1` with real credentials. Exercise full Connect -> Submit -> Fills -> Cancel -> Close lifecycle.
