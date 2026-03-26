# Webull Broker Design Spec

## Summary

Implement the `broker.Broker` interface for Webull using their official OpenAPI platform. The broker supports live equity trading through Webull accounts with two authentication models (Direct API and OAuth 2.0), gRPC streaming for fill delivery, and fractional share support.

## Architecture

The broker lives in `broker/webull/` and has three layers:

1. **Auth layer** -- a `Signer` interface with two implementations: `hmacSigner` (Direct API, HMAC-SHA1 per-request signing) and `oauthSigner` (Connect API, OAuth 2.0 authorization code flow). The signer attaches credentials to outbound HTTP requests.
2. **Client layer** -- a `Client` struct wrapping `go-resty/resty/v2` that uses the signer for auth and exposes typed methods for each Webull endpoint (place order, cancel, replace, account balance, positions, order history).
3. **Broker layer** -- the `WebullBroker` struct implementing `broker.Broker`, translating between pvbt types and Webull API types, and managing the gRPC event stream for fill delivery.

`GroupSubmitter` is **not** implemented. Webull lists combo orders (OCO/OTO/OTOCO) in their overview but the endpoint-level documentation is insufficient. The account layer handles group cancellation for brokers without `GroupSubmitter`.

## Design Decisions

1. **Both auth models supported.** Direct API (HMAC-SHA1 signing) is the default for personal use. OAuth 2.0 is available for third-party integrations. Selection is automatic based on which environment variables are set. This matches how IBKR supports both gateway auth and OAuth 1.0a.

2. **gRPC streaming for fills, not polling.** Webull provides a gRPC server-streaming endpoint at `events-api.webull.com` that pushes order status changes including fill notifications. This is preferred over polling because it delivers fills with lower latency and fewer API calls. On reconnect, a single poll of the orders endpoint catches any missed fills.

3. **No `GroupSubmitter`.** Combo order documentation is too sparse to implement reliably. The account layer already handles group cancellation for brokers without this interface.

4. **`Transactions()` returns empty with a warning.** Webull OpenAPI has no endpoint for dividends, splits, fees, or cash transfers. The method returns an empty slice and logs a warning at info level rather than silently returning nothing.

5. **Replace is strict.** Webull only allows modifying qty and price on a replace. If the caller's replacement order differs in side, TIF, or order type, the broker returns an error rather than silently ignoring the difference.

6. **Dollar-amount orders require market type.** Webull supports fractional/notional orders only for market orders. A non-market order with `Qty == 0` and `Amount > 0` returns an error.

## Authentication

### Direct API (HMAC-SHA1)

Every request is signed with `app_secret` using HMAC-SHA1. Required headers:

- `x-app-key` -- application key
- `x-timestamp` -- ISO8601 UTC timestamp
- `x-signature` -- HMAC-SHA1 signature
- `x-signature-algorithm` -- `HmacSHA1`
- `x-signature-version` -- `1.0`
- `x-signature-nonce` -- unique per request

Environment variables: `WEBULL_APP_KEY`, `WEBULL_APP_SECRET`.

No token refresh, browser auth, or persistence needed.

### Connect API (OAuth 2.0)

Standard authorization code flow with browser-based user consent.

- Authorization code expires in 60 seconds, single-use
- Access token expires in 30 minutes
- Refresh token expires in 15 days
- Background goroutine refreshes access token every 25 minutes
- Refresh token renewal requires user re-authentication after 15 days

Environment variables: `WEBULL_CLIENT_ID`, `WEBULL_CLIENT_SECRET`, `WEBULL_CALLBACK_URL` (default `https://127.0.0.1:5174`).

Token persistence: `WEBULL_TOKEN_FILE` (default `~/.pvbt/webull_token.json`).

### Selection Logic

If `WEBULL_APP_KEY` is set, use Direct API. If `WEBULL_CLIENT_ID` is set, use OAuth 2.0. If both are set, Direct API wins. If neither, return `broker.ErrMissingCredentials`.

### Account Selection

`WEBULL_ACCOUNT_ID` environment variable. If not set, `Connect()` fetches the account list and uses the first one, logging which account was selected.

## Order Translation

### Order Types

| pvbt | Webull |
|------|--------|
| `Market` | `MARKET` |
| `Limit` | `LIMIT` |
| `Stop` | `STOP_LOSS` |
| `StopLimit` | `STOP_LOSS_LIMIT` |

Trailing stop is not exposed through the pvbt `OrderType` enum and is not mapped.

### Time-in-Force

| pvbt | Webull |
|------|--------|
| `Day` | `DAY` |
| `GTC` | `GTC` |
| Others | Return error |

Webull only supports DAY and GTC. Submitting an order with any other TIF returns an error.

### Dollar-Amount Orders

When `Order.Qty == 0` and `Order.Amount > 0`, the broker submits a notional/fractional order. Only valid for market orders; non-market orders with dollar amounts return an error. Requires `WithFractionalShares()` option.

### Replace Restrictions

Webull only allows changing quantity and price on a replace operation. If the replacement order differs in side, TIF, or order type from the original, the broker returns an error.

## Fill Delivery

The broker connects a gRPC server-streaming call to `events-api.webull.com` during `Connect()`. A background goroutine reads from the stream and translates events into `broker.Fill` structs sent on the fills channel.

### Event Handling

| Event | Action |
|-------|--------|
| `FILLED` (partial) | Send `broker.Fill` with partial quantity and fill price |
| `FINAL_FILLED` | Send `broker.Fill` with remaining quantity |
| `PLACE_FAILED` | Log error |
| `CANCEL_SUCCESS` | Log info |
| `CANCEL_FAILED` | Log error |
| `MODIFY_SUCCESS` | Log info |
| `MODIFY_FAILED` | Log error |

### Reconnection

If the gRPC stream disconnects, the goroutine reconnects with exponential backoff (1s, 2s, 4s, capped at 30s). On reconnect, it polls the orders endpoint once to catch any fills missed during the gap.

### Deduplication

Fills are tracked by order ID + fill quantity. If a reconnect poll returns a fill already delivered via the stream, it is suppressed.

### Shutdown

`Close()` cancels the stream context, waits for the goroutine to exit, then closes the fills channel.

## Transactions

`Transactions()` returns an empty slice and logs a warning at info level: "Webull OpenAPI does not provide a transaction history endpoint; dividends, splits, and fees will not be synced."

## Error Handling

- HTTP errors wrapped via `broker.NewHTTPError`
- Re-exported common errors: `ErrMissingCredentials`, `ErrOrderNotFound`, `ErrInsufficientFunds`
- Rate limit responses (429) trigger a retry with backoff, respecting the `Retry-After` header
- Auth failures (401/403) on Direct API are terminal errors; on OAuth, trigger a token refresh and retry once

## Constructor Options

| Option | Description |
|--------|-------------|
| `WithUAT()` | Use UAT/test environment URLs |
| `WithTokenFile(path)` | Override OAuth token persistence location |
| `WithCallbackURL(url)` | Override OAuth callback URL |
| `WithFractionalShares()` | Enable dollar-amount orders |
| `WithAccountID(id)` | Override account selection (alternative to env var) |

## API Endpoints & Rate Limits

| Endpoint | Rate Limit |
|----------|-----------|
| Place Order | 600 / 60s |
| Cancel Order | 600 / 60s |
| Replace Order | 1 / 1s per App ID |
| Order History | 2 / 2s |
| Account Balance | 2 / 2s |
| Account Positions | 2 / 2s |

Base URLs:

| Environment | URL |
|-------------|-----|
| Production HTTP | `api.webull.com` |
| Production gRPC | `events-api.webull.com` |
| UAT HTTP | `us-openapi-alb.uat.webullbroker.com` |
| UAT OAuth | `us-oauth-open-api.uat.webullbroker.com` |
| Production OAuth | `us-oauth-open-api.webull.com` |

## File Structure

| File | Responsibility |
|------|---------------|
| `broker/webull/doc.go` | Package documentation |
| `broker/webull/errors.go` | Webull-specific and re-exported common broker errors |
| `broker/webull/auth.go` | Signer interface, HMAC and OAuth implementations |
| `broker/webull/client.go` | HTTP client wrapping resty, typed endpoint methods |
| `broker/webull/types.go` | JSON request/response structs, translation to `broker.*` types |
| `broker/webull/broker.go` | `WebullBroker` struct implementing `broker.Broker` |
| `broker/webull/streamer.go` | gRPC event stream goroutine, reconnection, deduplication |
| `broker/webull/exports_test.go` | Test-only exports for internal functions and types |
| `broker/webull/webull_suite_test.go` | Ginkgo test suite wiring |
| `broker/webull/auth_test.go` | Auth signing and token refresh tests |
| `broker/webull/client_test.go` | HTTP client endpoint tests |
| `broker/webull/types_test.go` | Type translation tests |
| `broker/webull/broker_test.go` | Broker integration tests |
| `broker/webull/streamer_test.go` | gRPC stream, reconnection, deduplication tests |

## Tech Stack

- Go
- `go-resty/resty/v2` (HTTP client)
- `crypto/hmac` + `crypto/sha1` (HMAC-SHA1 signing for Direct API)
- `google.golang.org/grpc` (gRPC streaming for trade events)
- Ginkgo/Gomega (tests)
- `net/http/httptest` (mock HTTP servers)
