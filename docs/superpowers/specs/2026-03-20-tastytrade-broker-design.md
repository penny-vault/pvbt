# tastytrade Broker Integration Design

## Overview

Implement the `Broker` interface for tastytrade as the first live broker integration. Equities only. The integration lives in a single package `broker/tastytrade/` and uses tastytrade's REST API for order management and WebSocket streaming API for real-time fill notifications.

The design preserves the "same code for backtest and live" principle -- strategies interact through the Portfolio/Batch layer and never touch the broker directly. Swapping `SimulatedBroker` for `TastytradeBroker` via `engine.WithBroker()` is the primary change needed to go live.

### Engine Integration Notes

The engine currently does not call `broker.Connect()` or `broker.Close()`. The implementation must add these lifecycle calls:

- `Connect(ctx)` called during engine initialization (in `createAccount` or at the start of `Backtest`/`RunLive`), after the broker is assigned.
- `Close()` called during engine teardown (in the engine's `Close()` method), alongside data provider cleanup.

The engine calls `SetPriceProvider()` on the broker in several places, but uses a type assertion `if sb, ok := e.broker.(*SimulatedBroker); ok` for the parent broker. This means a non-SimulatedBroker safely skips that code. Child strategies always use `*SimulatedBroker` (the `childEntry.broker` field is typed as `*SimulatedBroker`), so child broker calls are unaffected.

## Package Structure

```
broker/tastytrade/
  broker.go      -- Broker interface implementation, lifecycle, orchestration
  client.go      -- REST API client (auth, orders, positions, balance)
  streamer.go    -- WebSocket fill streaming + polling fallback
  types.go       -- tastytrade API request/response structs, translation functions
  errors.go      -- error classification (transient vs permanent), sentinel errors
```

## Broker Type

```go
type TastytradeBroker struct {
    client    *apiClient
    streamer  *fillStreamer
    fills     chan broker.Fill  // buffered (1024), returned by Fills()
    sandbox   bool
    mu        sync.Mutex       // protects order state
}
```

### Constructor

```go
func New(opts ...Option) *TastytradeBroker
```

Options:

- `WithSandbox()` -- use sandbox API endpoint instead of production

The broker is wired into the engine with `engine.WithBroker(tastytrade.New())`.

## REST Client and Authentication

### apiClient

```go
type apiClient struct {
    resty     *resty.Client
    accountID string
    username  string
    password  string
    mu        sync.Mutex  // protects re-authentication
}
```

Uses [go-resty](https://github.com/go-resty/resty) for HTTP communication. Resty provides built-in retry with exponential backoff and automatic JSON serialization.

### Base URLs

- Production: `https://api.tastyworks.com`
- Sandbox: `https://api.cert.tastyworks.com`

### Authentication Flow (called during Connect)

1. Read `TASTYTRADE_USERNAME` and `TASTYTRADE_PASSWORD` from environment.
2. POST to `/sessions` with credentials.
3. Store the returned session token via `resty.SetAuthToken()`.
4. Retrieve account ID from the session response.
5. Return `ErrMissingCredentials` if env vars are absent, or wrap the API error if auth fails.

### Session Management

- Session token is set on the Resty client and included automatically on all requests.
- Resty `OnAfterResponse` middleware intercepts 401 responses, re-authenticates once, and retries the original request.
- If re-auth fails, the error is returned immediately.

### Retry Behavior

Configured on the Resty client:

- `SetRetryCount(3)` with exponential backoff: 1s, 2s, 4s.
- `AddRetryCondition()` using `isTransient()` to determine retryable errors.
- Transient: network errors (timeouts, connection refused, DNS), HTTP 5xx.
- Permanent (no retry): HTTP 4xx (except 401 which triggers re-auth), order rejections, auth failures.

### Internal REST Methods

- `authenticate(ctx, username, password) error`
- `submitOrder(ctx, accountID, order) (orderID string, err error)`
- `cancelOrder(ctx, accountID, orderID) error`
- `replaceOrder(ctx, accountID, orderID, order) error`
- `getOrders(ctx, accountID) ([]orderResponse, error)`
- `getPositions(ctx, accountID) ([]positionResponse, error)`
- `getBalance(ctx, accountID) (balanceResponse, error)`
- `getQuote(ctx, symbol) (quote, error)` -- used for dollar-amount-to-shares conversion

## WebSocket Fill Streaming

### fillStreamer

```go
type fillStreamer struct {
    client    *apiClient       // also provides accountID
    fills     chan broker.Fill  // shared channel with broker
    wsConn    *websocket.Conn
    seenFills map[string]bool  // deduplication by fill ID
    mu        sync.Mutex
    done      chan struct{}
    wg        sync.WaitGroup   // tracks background goroutine for clean shutdown
}
```

The streamer accesses the account ID through the `client` rather than storing its own copy.

### Streaming Behavior

- Connects to tastytrade's account streamer WebSocket endpoint during `Connect()`.
- A background goroutine listens for order fill events, converts them to `broker.Fill`, and sends on the fills channel.
- Fill channel is buffered at 1024, consistent with `SimulatedBroker`.

### Partial Fills

Each partial fill produces a separate `broker.Fill` on the channel with the partial quantity and price. The order status transitions to `OrderPartiallyFilled` until the final fill arrives, at which point it becomes `OrderFilled`. This matches tastytrade's behavior of emitting individual execution events.

### Reconnection and Polling Fallback

- On WebSocket disconnect, reconnects with exponential backoff (1s, 2s, 4s, up to 3 attempts).
- After reconnecting, polls `getOrders()` (filtered to filled status) to catch fills missed during the disconnect window.
- Uses `seenFills` map for deduplication -- fills already received via WebSocket are not re-sent on the channel.
- Polling is only triggered on reconnection, not on a regular interval.

### seenFills Cleanup

The `seenFills` map is pruned at the start of each trading day. Entries older than 24 hours are evicted. This bounds memory growth for long-running sessions while retaining enough history to deduplicate across reconnections within a trading day.

### Shutdown

- `Close()` closes the `done` channel, signaling the background goroutine to close the WebSocket connection and exit.
- The goroutine stops sending on the fills channel before exiting. `Close()` waits for the goroutine to fully exit (via `sync.WaitGroup`) before closing the fills channel. This prevents send-on-closed-channel panics.
- Consumers of the `Fills()` channel should detect closure (receive of zero value with `ok == false`) as a signal that the broker has shut down.

### Context Cancellation

The background goroutine's select loop includes a case for the context passed to `Connect()`. When the context is cancelled (e.g., during `RunLive` shutdown), the goroutine exits cleanly via the same shutdown path as `Close()`. All REST calls pass through context-aware Resty methods, so in-flight HTTP requests are also cancelled.

## Broker Interface Method Mapping

### Lifecycle

| Method | Implementation |
|--------|---------------|
| `Connect(ctx)` | Authenticate via REST, retrieve account ID, open WebSocket streamer, start background fill goroutine |
| `Close()` | Signal shutdown, close WebSocket, wait for goroutine exit, close fills channel |

### Trading

| Method | Implementation |
|--------|---------------|
| `Submit(ctx, order)` | Translate `broker.Order` to tastytrade order request, call `submitOrder()`. Dollar-amount orders (Qty=0, Amount>0) fetch a quote and floor to whole shares. |
| `Fills()` | Return the shared buffered channel |
| `Cancel(ctx, orderID)` | Call `cancelOrder()` |
| `Replace(ctx, orderID, order)` | Call `replaceOrder()` |

### Queries

| Method | Implementation |
|--------|---------------|
| `Orders(ctx)` | Call `getOrders()`, translate responses to `[]broker.Order` |
| `Positions(ctx)` | Call `getPositions()`, translate to `[]broker.Position` |
| `Balance(ctx)` | Call `getBalance()`, translate to `broker.Balance` |

### Special Cases

**Dollar-amount orders:** When `Order.Qty == 0` and `Order.Amount > 0`, the broker fetches a quote via `getQuote()`, computes `math.Floor(amount / price)` for the share count, and submits with that quantity. This matches `SimulatedBroker` behavior.

**Order types:** All four order types are supported in this phase:

| broker.OrderType | tastytrade Mapping |
|------------------|--------------------|
| Market | `"Market"` |
| Limit | `"Limit"` -- uses `Order.LimitPrice` |
| Stop | `"Stop"` -- uses `Order.StopPrice` |
| StopLimit | `"Stop Limit"` -- uses both `Order.StopPrice` and `Order.LimitPrice` |

**Time-in-force mapping:**

| broker.TimeInForce | tastytrade Mapping |
|--------------------|--------------------|
| Day | `"Day"` |
| GTC | `"GTC"` |
| GTD | `"GTD"` -- uses `Order.GTDDate` |
| IOC | `"IOC"` |
| FOK | `"FOK"` |
| OnOpen | `"Day"` (see note below) |
| OnClose | `"Day"` (see note below) |

**OnOpen/OnClose:** tastytrade does not natively support these as order attributes. They are mapped to Market/Day orders. The engine's tradecron scheduling already ensures `Compute()` fires at the correct time, so the broker relies on the engine's timing rather than encoding this in the order.

## Type Mappings

### API Request/Response Types

```go
// Authentication
type sessionRequest struct {
    Login    string `json:"login"`
    Password string `json:"password"`
}

type sessionResponse struct {
    SessionToken string       `json:"session-token"`
    User         userResponse `json:"user"`
}

// Orders
type orderRequest struct {
    TimeInForce  string     `json:"time-in-force"`
    OrderType    string     `json:"order-type"`
    Price        float64    `json:"price,omitempty"`         // limit price
    StopTrigger  float64    `json:"stop-trigger,omitempty"`  // stop trigger price
    Legs         []orderLeg `json:"legs"`
}

type orderLeg struct {
    InstrumentType string  `json:"instrument-type"`
    Symbol         string  `json:"symbol"`
    Action         string  `json:"action"`
    Quantity       float64 `json:"quantity"`
}
```

tastytrade uses a JSON envelope pattern: `{ "data": { "items": [...] } }`. Response types reflect this structure.

### Translation Functions

Unexported functions in `types.go`:

- `toBrokerOrder(orderResponse) broker.Order`
- `toBrokerPosition(positionResponse) broker.Position`
- `toBrokerBalance(balanceResponse) broker.Balance`
- `toBrokerFill(fillEvent) broker.Fill`
- `toTastytradeOrder(broker.Order) orderRequest`

### Equity Action Mapping

| broker.Side | tastytrade Action |
|-------------|-------------------|
| Buy | `"Buy to Open"` |
| Sell | `"Sell to Close"` |

### Order Status Mapping

| tastytrade Status | broker.OrderStatus |
|-------------------|--------------------|
| Received | `OrderSubmitted` |
| Routed | `OrderSubmitted` |
| In Flight | `OrderSubmitted` |
| Live | `OrderOpen` |
| Filled | `OrderFilled` |
| Cancelled | `OrderCancelled` |
| Expired | `OrderCancelled` |
| Rejected | `OrderCancelled` |

`Expired` and `Rejected` map to `OrderCancelled` since they are terminal states where the order did not fill. The rejection reason is preserved in the error returned by `Submit()` when applicable.

## Error Handling

### Sentinel Errors

```go
var (
    ErrNotAuthenticated   = errors.New("tastytrade: not authenticated")
    ErrMissingCredentials = errors.New("tastytrade: TASTYTRADE_USERNAME and TASTYTRADE_PASSWORD must be set")
    ErrAccountNotFound    = errors.New("tastytrade: no accounts found")
    ErrOrderRejected      = errors.New("tastytrade: order rejected")
    ErrStreamDisconnected = errors.New("tastytrade: WebSocket disconnected")
)
```

### Error Classification

```go
func isTransient(err error) bool
```

- Returns true for: network errors (timeouts, connection refused, DNS), HTTP 5xx, WebSocket unexpected close.
- Returns false for: HTTP 4xx (except 401), order rejections, authentication failures.

Retry logic is handled entirely by Resty's built-in retry mechanism using `isTransient` as the retry condition. No separate `withRetry` helper is needed.

## Testing Strategy

All tests use Ginkgo and Gomega.

### Ginkgo Labels

Tests are categorized with Ginkgo labels for selective execution:

- `Label("integration")` -- tests that require sandbox credentials and network access. Excluded from default `go test` runs; run with `ginkgo --label-filter=integration`.
- `Label("auth")` -- tests covering authentication flow, session management, and re-auth.
- `Label("orders")` -- tests covering order submission, cancellation, replacement, and status mapping.
- `Label("streaming")` -- tests covering WebSocket fill streaming, reconnection, and deduplication.
- `Label("translation")` -- tests covering type translation between broker and tastytrade representations.

Labels can be combined. For example, an integration test that exercises order submission would carry both `Label("integration")` and `Label("orders")`.

### Unit Tests (no network required)

- `broker_test.go` -- broker lifecycle, channel wiring, option application
- `client_test.go` -- REST client using `httptest.Server` to mock tastytrade API responses. Covers authentication flow, 401 re-auth, retry on 5xx, order/position/balance translation, dollar-amount-to-shares conversion.
- `streamer_test.go` -- WebSocket handling using a local test WebSocket server. Covers fill parsing, deduplication via `seenFills`, reconnection triggering polling fallback.
- `types_test.go` -- type translation functions (both directions) with representative tastytrade JSON payloads.
- `errors_test.go` -- `isTransient()` classification for various error types.

### Integration Tests (sandbox)

- Labeled with `Label("integration")` for selective execution via `ginkgo --label-filter=integration`.
- Run against tastytrade sandbox API (`api.cert.tastyworks.com`).
- Require `TASTYTRADE_USERNAME` and `TASTYTRADE_PASSWORD` set in the environment.
- Cover the full round trip: connect, submit order, receive fill, query positions/balance, cancel order, close.
- Use Ginkgo and Gomega, same as unit tests.

## Documentation and Changelog

- Update the project changelog with a new entry describing the tastytrade broker integration.
- Add documentation for the `broker/tastytrade` package in `broker/tastytrade/doc.go` covering usage, configuration, and the sandbox option.

## Configuration Summary

| Setting | Source | Default |
|---------|--------|---------|
| Username | `TASTYTRADE_USERNAME` env var | (required) |
| Password | `TASTYTRADE_PASSWORD` env var | (required) |
| Sandbox mode | `WithSandbox()` option | false (production) |
| Retry count | Resty config | 3 |
| Retry backoff | Resty config | 1s, 2s, 4s exponential |
| Fill channel buffer | Hardcoded | 1024 |
