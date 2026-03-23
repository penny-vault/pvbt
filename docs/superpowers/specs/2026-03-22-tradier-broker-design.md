# Tradier Broker Design Spec

## Summary

Implement the `broker.Broker` and `broker.GroupSubmitter` interfaces for the Tradier API, enabling live trading through Tradier's REST and WebSocket APIs. This is the fourth live broker implementation after Alpaca, Tastytrade, and Schwab.

## Motivation

Issue #13. Tradier has a clean REST API and is popular with algo traders. Each broker integration makes pvbt useful to a different group of traders.

## Package Structure

New package `broker/tradier/` with the following layout:

| File | Purpose |
|------|---------|
| `broker.go` | `TradierBroker` struct implementing `broker.Broker` + `broker.GroupSubmitter` |
| `auth.go` | Dual-mode authentication: API access token or OAuth 2.0 authorization code flow |
| `client.go` | REST API client wrapping Tradier API v1 endpoints |
| `streamer.go` | WebSocket account event streamer for fill delivery |
| `types.go` | Request/response structs, mapping functions between Tradier and broker types |
| `errors.go` | Re-exports broker package sentinel errors as package-level variables |
| `doc.go` | Package documentation |

## Authentication

Tradier supports two authentication modes. The broker detects which to use based on environment variables.

### Mode 1: API Access Token (individual developers)

If `TRADIER_ACCESS_TOKEN` is set, use it directly as a Bearer token. Individual access tokens never expire.

### Mode 2: OAuth 2.0 (multi-user apps / partners)

If `TRADIER_CLIENT_ID` and `TRADIER_CLIENT_SECRET` are set (and `TRADIER_ACCESS_TOKEN` is not), run the OAuth 2.0 authorization code flow:

1. Start a local HTTPS server on the callback URL (self-signed TLS cert, same pattern as Schwab).
2. Print the authorization URL: `https://api.tradier.com/v1/oauth/authorize?client_id={id}&scope=read,write,trade,stream&state={state}`
3. Wait for the redirect containing `?code={auth_code}&state={state}`.
4. Exchange the code via `POST https://api.tradier.com/v1/oauth/accesstoken` with HTTP Basic auth (`base64(client_id:client_secret)`) and `grant_type=authorization_code&code={code}`.
5. Persist the access token and refresh token (if present) to the token file.
6. Access tokens expire in 24 hours. If a refresh token is available, start a background goroutine to refresh before expiry via `POST https://api.tradier.com/v1/oauth/refreshtoken` with `grant_type=refresh_token&refresh_token={token}`.

Note: Refresh tokens are only available to approved Tradier Partners. Without a refresh token, the user must re-authorize after 24 hours.

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `TRADIER_ACCESS_TOKEN` | no* | Long-lived API access token (individual developers) |
| `TRADIER_CLIENT_ID` | no* | OAuth app client ID |
| `TRADIER_CLIENT_SECRET` | no* | OAuth app client secret |
| `TRADIER_ACCOUNT_ID` | yes | Account number to trade |

*One of `TRADIER_ACCESS_TOKEN` or (`TRADIER_CLIENT_ID` + `TRADIER_CLIENT_SECRET`) is required. Return `broker.ErrMissingCredentials` if neither auth path has sufficient env vars.

### Token File Format

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "access_expires_at": "2026-03-23T15:30:00Z"
}
```

## Constructor and Options

```go
import "github.com/penny-vault/pvbt/broker/tradier"

// Default (production, API access token)
tradierBroker := tradier.New()

// Sandbox mode
tradierBroker := tradier.New(tradier.WithSandbox())

// OAuth with custom token file
tradierBroker := tradier.New(tradier.WithTokenFile("/path/to/tokens.json"))

// Custom callback URL for OAuth
tradierBroker := tradier.New(tradier.WithCallbackURL("https://127.0.0.1:9999"))
```

## TradierBroker Struct

```go
type TradierBroker struct {
    client      *apiClient
    auth        *tokenManager
    streamer    *accountStreamer
    fills       chan broker.Fill
    accountID   string
    sandbox     bool
    tokenFile   string
    callbackURL string
    mu          sync.Mutex
}
```

The `fills` channel is buffered at 1024 (matching other brokers). The `Fills()` method returns it as `<-chan broker.Fill`.

## REST Client

Use `go-resty/resty/v2` with:
- 3 retries, 1s base wait, 4s max wait
- Retry on 5xx, 429, and network errors
- Bearer token set via `SetAuthToken`
- `Accept: application/json` header on all requests (Tradier defaults to XML)
- `Content-Type: application/x-www-form-urlencoded` for POST/PUT (Tradier uses form-encoded request bodies, not JSON)

### Base URLs

| Environment | URL |
|-------------|-----|
| Production | `https://api.tradier.com/v1/` |
| Sandbox | `https://sandbox.tradier.com/v1/` |

### Rate Limits

Per-token, per-minute windows:

| Category | Production | Sandbox |
|----------|-----------|---------|
| Standard (accounts, orders reading) | 120/min | 60/min |
| Market Data | 120/min | 60/min |
| Trading (order placement/modification) | 60/min | 60/min |

Rate limit headers (`X-Ratelimit-Available`, `X-Ratelimit-Expiry`) are available on every response. The retry logic handles 429 responses automatically.

### Endpoints Used

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `POST` | `/v1/oauth/accesstoken` | Exchange auth code for tokens |
| `POST` | `/v1/oauth/refreshtoken` | Refresh access token |
| `GET` | `/v1/accounts/{id}/balances` | Get account balance |
| `GET` | `/v1/accounts/{id}/positions` | Get positions |
| `GET` | `/v1/accounts/{id}/orders` | List orders |
| `POST` | `/v1/accounts/{id}/orders` | Place order (simple or complex) |
| `PUT` | `/v1/accounts/{id}/orders/{id}` | Modify order |
| `DELETE` | `/v1/accounts/{id}/orders/{id}` | Cancel order |
| `GET` | `/v1/markets/quotes` | Get quote for dollar-amount conversion |
| `POST` | `/v1/accounts/events/session` | Create account streaming session |

### XML-to-JSON Quirk

Tradier's API performs XML-to-JSON conversion. Single-item collections serialize as objects while multi-item collections serialize as arrays. For example, `"position"` may be `{...}` (one position) or `[{...}, {...}]` (multiple). Use `json.RawMessage` for collection fields, then check whether the raw bytes start with `[` or `{` before unmarshalling into the target type.

## Broker Interface Methods

### Connect

1. Detect auth mode and authenticate (see Authentication section).
2. Validate `TRADIER_ACCOUNT_ID` is set.
3. Create the streaming session via `POST /v1/accounts/events/session`.
4. Start the WebSocket account streamer (production) or polling fallback (sandbox).
5. If using OAuth with a refresh token, start background refresh goroutine.

### Submit

Map `broker.Order` to `POST /v1/accounts/{id}/orders` with form parameters:

| broker field | Tradier parameter |
|-------------|-------------------|
| `class` | `equity` |
| `Asset.Ticker` | `symbol` |
| `Side == Buy` | `side=buy` |
| `Side == Sell` | `side=sell` |
| `Qty` | `quantity` |
| `OrderType == Market` | `type=market` |
| `OrderType == Limit` | `type=limit`, `price={LimitPrice}` |
| `OrderType == Stop` | `type=stop`, `stop={StopPrice}` |
| `OrderType == StopLimit` | `type=stop_limit`, `price={LimitPrice}`, `stop={StopPrice}` |
| `TimeInForce == Day` | `duration=day` |
| `TimeInForce == GTC` | `duration=gtc` |
| `TimeInForce == IOC` | return error (unsupported by Tradier) |
| `TimeInForce == FOK` | return error (unsupported by Tradier) |
| `TimeInForce == GTD` | return error (unsupported by Tradier) |
| `TimeInForce == OnOpen` | return error (unsupported by Tradier) |
| `TimeInForce == OnClose` | return error (unsupported by Tradier) |
| `Justification` | `tag` (truncated to Tradier's limit if needed) |

**Dollar-amount orders** (Qty == 0, Amount > 0): Fetch quote via `GET /v1/markets/quotes?symbols={sym}`, extract `last` price, compute `math.Floor(Amount / last)`. Return error if resulting qty is 0.

**Short selling**: When `Side == Sell` and no long position exists, use `side=sell_short`. When `Side == Buy` and a short position exists, use `side=buy_to_cover`. This requires checking current positions before submission. The mutex must be held across the positions check and order submission to make this atomic.

Response: `{"order": {"id": 123456, "status": "ok"}}`. Extract and set the order ID.

**Important**: Tradier returns HTTP 200 even for downstream order failures. After submission, the order status must be verified by querying the order. If the order has been rejected, return `broker.ErrOrderRejected` wrapped with the rejection reason from the order's `errors` field.

### Cancel

`DELETE /v1/accounts/{id}/orders/{order_id}`.

### Replace

`PUT /v1/accounts/{id}/orders/{order_id}` with form parameters for `type`, `duration`, `price`, `stop`.

Tradier's modify endpoint cannot change symbol, side, or quantity. If the replacement order changes quantity, cancel the original order and submit a new one.

### Orders

`GET /v1/accounts/{id}/orders`. Map each order to `broker.Order`.

Status mapping:

| Tradier status | broker.OrderStatus |
|----------------|-------------------|
| `pending` | `broker.OrderSubmitted` |
| `open` | `broker.OrderOpen` |
| `partially_filled` | `broker.OrderPartiallyFilled` |
| `filled` | `broker.OrderFilled` |
| `expired`, `canceled`, `rejected`, `pending_cancel` | `broker.OrderCancelled` |

Handle the single-item/array quirk when parsing the orders collection.

### Positions

`GET /v1/accounts/{id}/positions`. Map each position:

| Tradier field | broker.Position field |
|---------------|----------------------|
| `symbol` | `Asset.Ticker` |
| `quantity` (negative for short) | `Qty` |
| `cost_basis / quantity` | `AvgOpenPrice` |
| (from quote) | `MarkPrice` |
| (not directly available) | `RealizedDayPL` |

`MarkPrice` requires a supplementary quote fetch. `RealizedDayPL` can be derived from the gain/loss endpoint or set to 0 if not available from positions alone.

### Balance

`GET /v1/accounts/{id}/balances`. The response contains different subsections depending on account type (margin, cash, PDT).

| Source | broker.Balance field |
|--------|---------------------|
| `total_equity` | `NetLiquidatingValue` |
| `total_cash` (or margin: `stock_buying_power`) | `EquityBuyingPower` |
| margin: `current_requirement` | `MaintenanceReq` |
| cash: `cash_available` or margin: `total_cash` | `CashBalance` |

Read whichever subsection is present (margin, cash, or pdt) and map into `broker.Balance`.

## GroupSubmitter

### OCO (broker.GroupOCO)

`POST /v1/accounts/{id}/orders` with `class=oco`:

```
class=oco
duration=day
symbol[0]=AAPL&side[0]=sell&quantity[0]=100&type[0]=limit&price[0]=155.00
symbol[1]=AAPL&side[1]=sell&quantity[1]=100&type[1]=stop&stop[1]=145.00
```

Both legs must have the same symbol and different order types (e.g., one limit, one stop).

### Bracket (broker.GroupBracket)

`POST /v1/accounts/{id}/orders` with `class=otoco`:

```
class=otoco
duration=day
symbol[0]=AAPL&side[0]=buy&quantity[0]=100&type[0]=limit&price[0]=150.00
symbol[1]=AAPL&side[1]=sell&quantity[1]=100&type[1]=limit&price[1]=155.00
symbol[2]=AAPL&side[2]=sell&quantity[2]=100&type[2]=stop&stop[2]=145.00
```

Leg 0 is the entry order (triggers), legs 1-2 are the OCO pair (take-profit and stop-loss). Find orders by `GroupRole`: `RoleEntry` -> leg 0, `RoleTakeProfit` -> leg 1, `RoleStopLoss` -> leg 2.

## WebSocket Streamer

### Connection Flow (Production)

1. Create session via `POST /v1/accounts/events/session` to get `sessionid` (valid 5 minutes).
2. Connect to `wss://ws.tradier.com/v1/accounts/events`.
3. Send subscription: `{"events": ["order"], "sessionid": "{sessionid}", "excludeAccounts": []}`.
4. Process incoming order events in a background goroutine.

### Fill Detection

Order events include `status`, `avg_fill_price`, `executed_quantity`, `last_fill_quantity`, `last_fill_price`, and `transaction_date`. When `status` is `filled` or `partially_filled`, construct a `broker.Fill`:

```go
broker.Fill{
    OrderID:  fmt.Sprintf("%d", event.ID),
    Price:    event.LastFillPrice,
    Qty:      event.LastFillQuantity,
    FilledAt: event.TransactionDate,
}
```

### Deduplication

Track seen fills in `map[string]time.Time` keyed by `"{orderID}-{fillQty}-{fillTime}"`. Prune entries older than 24 hours once per day.

### Reconnection

On WebSocket read error:
- Close the old connection.
- Create a new session (old sessionid has likely expired).
- Reconnect with exponential backoff (1s, 2s, 4s), max 3 attempts.
- On successful reconnect, poll `GET /v1/accounts/{id}/orders` for fills missed during outage.
- Return `broker.ErrStreamDisconnected` if all attempts fail.

### Sandbox Fallback

Tradier sandbox does not support account streaming. In sandbox mode, poll `GET /v1/accounts/{id}/orders` every 2 seconds, comparing against previously known fill states to detect new fills. This polling loop runs in a background goroutine and delivers fills on the same channel.

## Close

1. Close the WebSocket connection (or stop the polling goroutine in sandbox mode).
2. Close the fills channel.
3. Stop the token refresh goroutine if running.

## Testing

- Compile-time interface assertions: `var _ broker.Broker = (*tradier.TradierBroker)(nil)` and `var _ broker.GroupSubmitter = (*tradier.TradierBroker)(nil)`
- Unit tests with `httptest.NewServer()` mock for every client method
- Unit tests for all type mapping functions (broker <-> Tradier), including the single-item/array JSON quirk
- Unit tests for dual auth mode detection (access token vs OAuth)
- Unit tests for Replace with quantity change (cancel + resubmit path)
- WebSocket tests using a local `httptest` server for the streamer
- Test exports in `exports_test.go` for injecting test clients and account IDs

## Usage

```go
import "github.com/penny-vault/pvbt/broker/tradier"

tradierBroker := tradier.New()

eng := engine.New(&MyStrategy{},
    engine.WithBroker(tradierBroker),
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
```

Environment (API access token mode):
```bash
export TRADIER_ACCESS_TOKEN=your_token
export TRADIER_ACCOUNT_ID=your_account_id
```

Environment (OAuth mode):
```bash
export TRADIER_CLIENT_ID=your_client_id
export TRADIER_CLIENT_SECRET=your_client_secret
export TRADIER_ACCOUNT_ID=your_account_id
# Optional:
export TRADIER_TOKEN_FILE=~/.config/pvbt/tradier-tokens.json
export TRADIER_CALLBACK_URL=https://127.0.0.1:5174
```
