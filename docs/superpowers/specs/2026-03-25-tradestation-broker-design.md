# TradeStation Broker Design Spec

## Summary

Implement the `broker.Broker` interface for TradeStation using their v3 REST API. The broker supports live equity trading through TradeStation accounts with OAuth 2.0 authentication (Auth0-based), HTTP chunked streaming for fill delivery, and native OCO/bracket group support via `GroupSubmitter`.

## Architecture

The broker lives in `broker/tradestation/` and has three layers:

1. **Auth layer** -- a `tokenManager` struct handling OAuth 2.0 authorization code flow via Auth0 (`signin.tradestation.com`). Background goroutine refreshes the access token every 15 minutes (token expires in 20 minutes).
2. **Client layer** -- a `apiClient` struct wrapping `go-resty/resty/v2` that exposes typed methods for each TradeStation endpoint (place order, cancel, replace, account balance, positions, order list, quote).
3. **Broker layer** -- the `TradeStationBroker` struct implementing `broker.Broker`, translating between pvbt types and TradeStation API types, and managing the HTTP chunked stream for fill delivery.

`GroupSubmitter` **is** implemented. TradeStation supports OCO (`"OCO"`) and bracket (`"BRK"`) group orders natively via `POST /v3/orderexecution/ordergroups`.

## Design Decisions

1. **OAuth 2.0 only.** TradeStation uses Auth0-based OAuth 2.0 authorization code flow. No alternative auth model exists.

2. **HTTP chunked streaming for fills.** TradeStation provides `GET /v3/brokerage/stream/accounts/{accountIds}/orders` which delivers newline-delimited JSON over a long-lived HTTP connection. This is preferred over polling. On reconnect, a single poll of the orders endpoint catches any missed fills.

3. **`GroupSubmitter` implemented.** TradeStation natively supports OCO and bracket (BRK) order groups. The broker implements `GroupSubmitter` to submit these atomically.

4. **`Transactions()` returns empty with a warning.** TradeStation v3 API has no dedicated endpoint for dividends, splits, or fees. The method returns an empty slice and logs a warning at info level.

5. **Sandbox via URL swap.** TradeStation provides a full simulation environment at `sim-api.tradestation.com` with identical API surface. The `WithSandbox()` option switches the base URL.

6. **Dollar-amount orders.** When `Qty == 0` and `Amount > 0`, the broker fetches a quote and computes share quantity as `floor(Amount / price)`. This matches the pattern used by Schwab and other brokers.

7. **Order IDs stripped of dashes.** TradeStation requires order IDs without dashes for modify/cancel operations. The broker strips dashes before calling these endpoints.

## Authentication

OAuth 2.0 Authorization Code flow via Auth0.

- Authorization endpoint: `https://signin.tradestation.com/authorize`
- Token endpoint: `https://signin.tradestation.com/oauth/token`
- Required scopes: `openid profile offline_access MarketData ReadAccount Trade`
- Access token expires in 20 minutes
- Refresh token is non-expiring (when `offline_access` scope is granted)
- Background goroutine refreshes access token every 15 minutes

Environment variables:

- `TRADESTATION_CLIENT_ID` -- OAuth app key (required)
- `TRADESTATION_CLIENT_SECRET` -- OAuth app secret (required)
- `TRADESTATION_ACCOUNT_ID` -- Account ID (optional; uses first account if unset)
- `TRADESTATION_CALLBACK_URL` -- OAuth redirect URI (default `https://127.0.0.1:5174`)
- `TRADESTATION_TOKEN_FILE` -- Token persistence path (default `~/.config/pvbt/tradestation-tokens.json`)

Token persistence: same JSON format as other brokers (`accessToken`, `refreshToken`, `accessExpiresAt`). No `refreshExpiresAt` needed since the refresh token does not expire.

## Order Translation

### Order Types

| pvbt | TradeStation |
|------|-------------|
| `Market` | `Market` |
| `Limit` | `Limit` |
| `Stop` | `StopMarket` |
| `StopLimit` | `StopLimit` |

### Time-in-Force

| pvbt | TradeStation Duration |
|------|----------------------|
| `Day` | `DAY` |
| `GTC` | `GTC` |
| `GTD` | `GTD` (with expiration date from `Order.GTDDate`) |
| `IOC` | `IOC` |
| `FOK` | `FOK` |
| `OnOpen` | `OPG` |
| `OnClose` | `CLO` |

All pvbt TIF values have a direct TradeStation mapping.

### Trade Action Mapping

| pvbt Side | TradeStation TradeAction |
|-----------|------------------------|
| `Buy` | `BUY` |
| `Sell` | `SELL` |

Short selling (`SELLSHORT`, `BUYTOCOVER`) is not exposed in the pvbt `Side` enum and is not mapped.

### Order Request Body

```json
{
  "AccountID": "...",
  "Symbol": "AAPL",
  "Quantity": "100",
  "OrderType": "Limit",
  "TradeAction": "BUY",
  "TimeInForce": {"Duration": "DAY"},
  "LimitPrice": "150.00",
  "Route": "Intelligent"
}
```

Route is always `"Intelligent"` (TradeStation's smart order routing).

## Fill Delivery

The broker opens an HTTP chunked stream to `GET /v3/brokerage/stream/accounts/{accountId}/orders` during `Connect()`. A background goroutine reads newline-delimited JSON objects from the stream and translates fill events into `broker.Fill` structs.

### Stream Parsing

JSON objects may span multiple HTTP chunks (proxies can re-chunk). The goroutine uses a streaming JSON decoder (`json.Decoder`) on the response body to handle this correctly.

### Event Handling

The stream delivers order status updates. The broker filters for orders with status `Filled` or `PartiallyFilled` and extracts fill information (price, quantity, timestamp) from the `Legs[].Fills[]` array.

### Signals

| Signal | Action |
|--------|--------|
| `EndSnapshot` | Initial snapshot complete; switch to incremental mode |
| `GoAway` | Server requests stream restart; reconnect immediately |
| Error JSON | Log error, close connection, trigger reconnect |

### Deduplication

Fills are tracked by order ID + cumulative filled quantity. The broker compares cumulative quantity against what has been delivered. If unchanged, the fill is suppressed. If increased, a `broker.Fill` is sent for the delta.

### Reconnection

If the stream disconnects or receives a `GoAway`, the goroutine reconnects with exponential backoff (1s, 2s, 4s, capped at 30s). On reconnect, it polls `GET /v3/brokerage/accounts/{accountId}/orders` once to catch any missed fills.

### Shutdown

`Close()` cancels the stream context, waits for the goroutine to exit, and closes the fills channel.

## Order Groups

The broker implements `broker.GroupSubmitter` via `POST /v3/orderexecution/ordergroups`.

### OCO Groups

```json
{"Type": "OCO", "Orders": [{...}, {...}]}
```

When one order fills, TradeStation automatically cancels the other.

### Bracket Groups

```json
{"Type": "BRK", "Orders": [{entry}, {stop-loss}, {take-profit}]}
```

The entry order is placed first. When it fills, TradeStation activates the child stop-loss and take-profit legs as an OCO pair.

The broker maps `GroupRole` values: `RoleEntry` becomes the first order, `RoleStopLoss` and `RoleTakeProfit` become child orders.

## Transactions

`Transactions()` returns an empty slice and logs a warning at info level: "TradeStation v3 API does not provide a transaction history endpoint; dividends, splits, and fees will not be synced."

## Error Handling

- HTTP errors wrapped via `broker.NewHTTPError`
- Rate limit responses (429) handled by resty retry with backoff
- Auth failures (401) trigger a token refresh and retry once; if refresh fails, return `broker.ErrNotAuthenticated`
- Broker-specific errors: `ErrTokenExpired`, `ErrAuthorizationRequired`, `ErrStreamDisconnected`

## Constructor Options

| Option | Description |
|--------|-------------|
| `WithSandbox()` | Use simulation environment (`sim-api.tradestation.com`) |
| `WithTokenFile(path)` | Override token persistence location |
| `WithCallbackURL(url)` | Override OAuth callback URL |
| `WithAccountID(id)` | Override account selection (alternative to env var) |

## API Endpoints & Rate Limits

| Endpoint | Rate Limit |
|----------|-----------|
| Accounts | 250 / 5min |
| Balances | 250 / 5min |
| Positions | 250 / 5min |
| Orders | 250 / 5min |
| Quote Snapshot | 30 / 1min |
| Order Stream | 40 concurrent |

Base URLs:

| Environment | URL |
|-------------|-----|
| Production | `https://api.tradestation.com/v3` |
| Simulation | `https://sim-api.tradestation.com/v3` |
| Auth | `https://signin.tradestation.com` |

## File Structure

| File | Responsibility |
|------|---------------|
| `broker/tradestation/doc.go` | Package documentation |
| `broker/tradestation/errors.go` | TradeStation-specific errors |
| `broker/tradestation/auth.go` | OAuth 2.0 token manager, browser auth flow, background refresh |
| `broker/tradestation/client.go` | HTTP client wrapping resty, typed endpoint methods |
| `broker/tradestation/types.go` | JSON request/response structs, translation to `broker.*` types |
| `broker/tradestation/broker.go` | `TradeStationBroker` struct implementing `broker.Broker` and `broker.GroupSubmitter` |
| `broker/tradestation/streamer.go` | HTTP chunked stream goroutine, reconnection, deduplication |
| `broker/tradestation/exports_test.go` | Test-only exports |
| `broker/tradestation/tradestation_suite_test.go` | Ginkgo test suite wiring |
| `broker/tradestation/auth_test.go` | Auth flow and token refresh tests |
| `broker/tradestation/client_test.go` | HTTP client endpoint tests |
| `broker/tradestation/types_test.go` | Type translation tests |
| `broker/tradestation/broker_test.go` | Broker integration tests |
| `broker/tradestation/streamer_test.go` | Stream parsing, reconnection, deduplication tests |

## Tech Stack

- Go
- `go-resty/resty/v2` (HTTP client with retry)
- `encoding/json` (streaming JSON decoder for HTTP chunked stream)
- Ginkgo/Gomega (tests)
- `net/http/httptest` (mock HTTP servers)
