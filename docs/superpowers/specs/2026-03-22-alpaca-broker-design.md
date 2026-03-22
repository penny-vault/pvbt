# Alpaca Broker Design Spec

## Summary

Implement the `broker.Broker` and `broker.GroupSubmitter` interfaces for Alpaca, enabling live and paper trading through Alpaca's v2 Trading API. As a prerequisite, promote shared error sentinels from the tastytrade package into the broker package so all broker implementations use a common error vocabulary.

## Motivation

Issue #14. Alpaca is commission-free and widely used in the algo trading community. Adding Alpaca as a second live broker validates the broker interface design and gives users a choice of brokerage.

## Package Structure

New package `broker/alpaca/` mirroring the tastytrade layout:

| File | Purpose |
|------|---------|
| `broker.go` | `AlpacaBroker` struct implementing `broker.Broker` + `broker.GroupSubmitter` |
| `client.go` | REST API client wrapping Alpaca's v2 endpoints |
| `types.go` | Request/response structs, mapping functions between Alpaca and broker types |
| `streamer.go` | WebSocket fill streamer subscribing to `trade_updates` |
| `errors.go` | Alpaca-specific error helpers (uses shared broker errors) |
| `doc.go` | Package documentation |

## Shared Broker Errors

Before building the Alpaca broker, move sentinel errors and error utilities from `broker/tastytrade/errors.go` into `broker/errors.go`. Both broker implementations then use the shared set.

### Shared sentinels in `broker/errors.go`

| Error | Condition |
|-------|-----------|
| `ErrMissingCredentials` | Required credentials not configured |
| `ErrNotAuthenticated` | Operation attempted without a valid session |
| `ErrAccountNotFound` | No account found during authentication |
| `ErrAccountNotActive` | Account exists but is not in a tradeable state |
| `ErrStreamDisconnected` | WebSocket fill stream exhausted reconnect attempts |
| `ErrEmptyOrderGroup` | `SubmitGroup` called with zero orders |
| `ErrNoEntryOrder` | Bracket group missing an entry-role order |
| `ErrMultipleEntryOrders` | Bracket group has more than one entry-role order |
| `ErrOrderRejected` | Broker rejected the order |

### Shared utilities in `broker/errors.go`

- `HTTPError` struct with `StatusCode` and `Message` fields.
- `NewHTTPError(statusCode int, message string) *HTTPError` constructor.
- `IsTransient(err error) bool` returning true for 5xx `HTTPError`, 429 `HTTPError` (rate limiting), `net.OpError`, `net.DNSError`, and `url.Error` wrapping a transient cause.

The tastytrade package deletes its duplicate sentinels and imports from `broker` instead.

## Authentication

Environment variables:

| Variable | Description |
|----------|-------------|
| `ALPACA_API_KEY` | Alpaca API key ID |
| `ALPACA_API_SECRET` | Alpaca API secret key |

Passed as HTTP headers `APCA-API-KEY-ID` and `APCA-API-SECRET-KEY` on every REST request. The WebSocket auth message sends these as `key` and `secret` fields. No session tokens or refresh logic needed -- Alpaca's auth is stateless.

## Constructor and Options

```go
import "github.com/penny-vault/pvbt/broker/alpaca"

// Live trading, whole shares
alpacaBroker := alpaca.New()

// Paper trading
alpacaBroker := alpaca.New(alpaca.WithPaper())

// Fractional shares (dollar-amount orders use Alpaca's notional field)
alpacaBroker := alpaca.New(alpaca.WithFractionalShares())

// Combined
alpacaBroker := alpaca.New(alpaca.WithPaper(), alpaca.WithFractionalShares())
```

### Base URLs

| Environment | REST | WebSocket |
|-------------|------|-----------|
| Live | `https://api.alpaca.markets` | `wss://api.alpaca.markets/stream` |
| Paper | `https://paper-api.alpaca.markets` | `wss://paper-api.alpaca.markets/stream` |

## AlpacaBroker Struct

```go
type AlpacaBroker struct {
    client          *apiClient
    streamer        *fillStreamer
    fills           chan broker.Fill
    paper           bool
    fractional      bool
    submittedOrders map[string]broker.Order // tracks orders for Replace
    mu              sync.Mutex
}
```

The `fills` channel is buffered at 1024 (matching tastytrade). The `Fills()` method returns it as `<-chan broker.Fill`.

## Broker Interface Methods

### Connect

1. Read `ALPACA_API_KEY` and `ALPACA_API_SECRET` from environment. Return `broker.ErrMissingCredentials` if either is empty.
2. Call `GET /v2/account` to validate credentials and check account status. Return `broker.ErrAccountNotActive` if the account status is not `ACTIVE`.
3. Start the WebSocket fill streamer (see Streamer section).

### Fills

Returns `fills` as a receive-only `<-chan broker.Fill`. Identical to the tastytrade pattern.

### Close

Shut down the WebSocket connection, wait for the background goroutine, close the fills channel.

### Submit

Map `broker.Order` to Alpaca's `POST /v2/orders` request body:

- `side`: `broker.Buy` -> `"buy"`, `broker.Sell` -> `"sell"`
- `type`: `broker.Market` -> `"market"`, `broker.Limit` -> `"limit"`, `broker.Stop` -> `"stop"`, `broker.StopLimit` -> `"stop_limit"`
- `time_in_force`: `broker.Day` -> `"day"`, `broker.GTC` -> `"gtc"`, `broker.GTD` -> `"gtd"`, `broker.IOC` -> `"ioc"`, `broker.FOK` -> `"fok"`, `broker.OnOpen` -> `"opg"`, `broker.OnClose` -> `"cls"`
- `limit_price`: set when order type is Limit or StopLimit
- `stop_price`: set when order type is Stop or StopLimit
- `expire_time`: set to `GTDDate` (ISO 8601) when time-in-force is GTD
- `client_order_id`: set to a generated UUID for idempotent submission (prevents duplicates on retry)

The `LotSelection` field on `broker.Order` is ignored -- Alpaca does not support tax lot selection on the order API.

Extended hours trading (`extended_hours` field) is out of scope for the initial implementation.

**Dollar-amount orders** (Qty == 0, Amount > 0):
- If fractional shares enabled: send `notional` field with the dollar amount. Alpaca handles the share calculation.
- If fractional shares disabled: fetch current price via `GET /v2/stocks/{symbol}/quotes/latest`, compute `math.Floor(Amount / price)`, send `qty`. Skip if computed qty is 0.

Store the submitted order in the mutex-protected `submittedOrders` map keyed by the Alpaca-returned order ID, for use by Replace.

### Cancel

`DELETE /v2/orders/{id}`. Remove the order from the internal tracking map.

### Replace (smart cancel-replace)

All tracking map access is mutex-protected.

1. Look up the original order from `submittedOrders`.
2. Compare non-mutable fields (asset, side, order type) between original and replacement.
3. If only mutable fields changed (`qty`, `limit_price`, `stop_price`, `time_in_force`): send `PATCH /v2/orders/{id}` with the changed fields. Update the tracking map.
4. If any non-mutable field changed: cancel the original via `DELETE /v2/orders/{id}`. If the cancel returns 422 (order already filled/cancelled), return the error without submitting a replacement. Otherwise submit the replacement as a new order via `POST /v2/orders`. Update the tracking map with the new order ID.

### Orders

`GET /v2/orders?status=open`. Map each Alpaca order to `broker.Order`. Alpaca does not paginate this endpoint when the default limit (50) is sufficient; if more orders are expected, pass `limit=500`. Pagination beyond 500 is not currently needed.

Status mapping:

| Alpaca status | broker.OrderStatus |
|---------------|-------------------|
| `new`, `accepted`, `pending_new` | `broker.OrderSubmitted` |
| `partially_filled` | `broker.OrderPartiallyFilled` |
| `filled` | `broker.OrderFilled` |
| `canceled`, `expired`, `rejected`, `suspended` | `broker.OrderCancelled` |

### Positions

`GET /v2/positions`. Map each position:

| Alpaca field | broker.Position field |
|-------------|----------------------|
| `symbol` | `Asset.Ticker` |
| `qty` | `Qty` |
| `avg_entry_price` | `AvgOpenPrice` |
| `current_price` | `MarkPrice` |
| `unrealized_intraday_pl` | `RealizedDayPL` (note: Alpaca exposes unrealized, not realized, intraday P&L -- this is the closest available field) |

### Balance

`GET /v2/account`. Map account fields:

| Alpaca field | broker.Balance field |
|-------------|---------------------|
| `cash` | `CashBalance` |
| `equity` | `NetLiquidatingValue` |
| `buying_power` | `EquityBuyingPower` |
| `maintenance_margin` | `MaintenanceReq` |

## GroupSubmitter

### Bracket (broker.GroupBracket)

Find the entry-role order from the group. Submit it as a single `POST /v2/orders` with:
- `order_class: "bracket"`
- `take_profit: {"limit_price": "<price>"}` from the take-profit-role order
- `stop_loss: {"stop_price": "<price>"}` from the stop-loss-role order

Alpaca manages the contingent legs atomically.

Return `broker.ErrNoEntryOrder` if no entry order, `broker.ErrMultipleEntryOrders` if more than one.

### OCO (broker.GroupOCO)

Submit with `order_class: "oco"`. The first order in the slice is the primary order; the second is the dependent leg. Both orders must share the same asset and side. Alpaca cancels the remaining leg when one fills.

## WebSocket Fill Streamer

### Connection flow

1. Dial `wss://api.alpaca.markets/stream` (or `wss://paper-api.alpaca.markets/stream`).
2. Send `{"action": "auth", "key": "<api_key>", "secret": "<api_secret>"}`.
3. Read the next message with a 10-second deadline. Expect `{"stream": "authorization", "data": {"status": "authorized"}}`. If unauthorized or timeout, return error.
4. Send `{"action": "listen", "data": {"streams": ["trade_updates"]}}`.
5. Read the next message with a 10-second deadline. Expect `{"stream": "listening", "data": {"streams": ["trade_updates"]}}`. Log and continue if the format differs.
6. Send WebSocket-level ping frames every 30 seconds to detect stale connections.

### Processing trade updates

On `fill` or `partial_fill` events:
- Extract `execution_id` (for deduplication), `price`, `qty`, `timestamp` from the `data` field.
- Extract the order ID from the nested `order` object.
- Deduplicate by `execution_id` using a `map[string]time.Time` (same pattern as tastytrade's `seenFills`).
- Deliver as `broker.Fill` on the fills channel.

### Reconnection

On WebSocket read error:
- Close the old connection.
- Reconnect with exponential backoff (1s, 2s, 4s), max 3 attempts.
- On successful reconnect, poll `GET /v2/orders?status=closed` for any fills missed during the outage.
- Deduplicate against `seenFills`.
- Return `broker.ErrStreamDisconnected` if all attempts fail.

### Pruning

Prune `seenFills` entries older than 24 hours, at most once per calendar day (same as tastytrade).

## REST Client

Use `go-resty/resty/v2` with:
- 3 retries, 1s base wait, 4s max wait.
- Retry on transient errors (5xx, network errors) via `broker.IsTransient`.
- API key headers set once on the client.
- No session token management.

### Endpoints used

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `GET` | `/v2/account` | Validate credentials, get balance |
| `POST` | `/v2/orders` | Submit order (simple, bracket, OCO) |
| `GET` | `/v2/orders` | List orders |
| `DELETE` | `/v2/orders/{id}` | Cancel order |
| `PATCH` | `/v2/orders/{id}` | Replace order (mutable fields) |
| `GET` | `/v2/positions` | List positions |
| `GET` | `/v2/stocks/{symbol}/quotes/latest` | Get quote for dollar-amount orders (non-fractional mode) |

## Testing

- Unit tests with HTTP response mocks for every client method.
- Unit tests for all type mapping functions (broker <-> Alpaca).
- WebSocket tests using a local `httptest` server for the streamer, covering auth, fill delivery, deduplication, and reconnection.
- Integration test (build-tagged `integration`) hitting Alpaca's paper trading environment.
- Verify the tastytrade refactor by running existing tastytrade tests after moving errors to the shared package.

## Usage

```go
import "github.com/penny-vault/pvbt/broker/alpaca"

alpacaBroker := alpaca.New(alpaca.WithPaper())

eng := engine.New(&MyStrategy{},
    engine.WithBroker(alpacaBroker),
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
```

Environment:
```
export ALPACA_API_KEY=your_key
export ALPACA_API_SECRET=your_secret
```
