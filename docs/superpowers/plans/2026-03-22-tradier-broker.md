# Tradier Broker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `broker.Broker` and `broker.GroupSubmitter` for the Tradier brokerage API.

**Architecture:** New `broker/tradier/` sub-package following the same structure as the Schwab, Alpaca, and Tastytrade brokers. REST client via `go-resty/resty/v2`, WebSocket account streamer for fill delivery (with polling fallback in sandbox), dual-mode auth (API token or OAuth 2.0).

**Tech Stack:** Go, go-resty/resty/v2, gorilla/websocket, zerolog, Ginkgo/Gomega

**Spec:** `docs/superpowers/specs/2026-03-22-tradier-broker-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `broker/tradier/doc.go` | Package documentation |
| `broker/tradier/errors.go` | Re-export broker sentinel errors |
| `broker/tradier/types.go` | Tradier API request/response types, JSON collection quirk helper, translation functions (to/from broker types) |
| `broker/tradier/client.go` | REST API client: order CRUD, positions, balances, quotes, streaming session |
| `broker/tradier/auth.go` | Dual-mode token management: static API token or OAuth 2.0 with browser flow and refresh |
| `broker/tradier/streamer.go` | WebSocket account event streamer + sandbox polling fallback |
| `broker/tradier/broker.go` | `TradierBroker` struct, `New()`, `Option` pattern, all `broker.Broker` and `broker.GroupSubmitter` methods |
| `broker/tradier/tradier_suite_test.go` | Ginkgo test suite wiring |
| `broker/tradier/exports_test.go` | Test-only exports for injecting clients and account IDs |
| `broker/tradier/types_test.go` | Tests for type translation and JSON collection quirk |
| `broker/tradier/client_test.go` | Tests for REST client methods |
| `broker/tradier/auth_test.go` | Tests for dual auth mode detection, token persistence |
| `broker/tradier/broker_test.go` | Tests for broker interface methods and group orders |
| `broker/tradier/streamer_test.go` | Tests for WebSocket streamer and sandbox polling |

---

### Task 1: Package scaffolding (doc.go, errors.go, suite test)

**Files:**
- Create: `broker/tradier/doc.go`
- Create: `broker/tradier/errors.go`
- Create: `broker/tradier/tradier_suite_test.go`

- [ ] **Step 1: Create the package directory**

Run: `mkdir -p broker/tradier`

- [ ] **Step 2: Write `doc.go`**

```go
// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package tradier implements broker.Broker for the Tradier brokerage.
//
// Tradier offers a clean REST API popular with algo traders. This broker
// supports equities only. Strategies that work with the SimulatedBroker
// require no changes to run live -- swap the broker via
// engine.WithBroker(tradier.New()).
//
// # Authentication
//
// The broker supports two authentication modes:
//
//   - API access token: Set TRADIER_ACCESS_TOKEN. Individual tokens never
//     expire. This is the simplest mode for personal use.
//   - OAuth 2.0: Set TRADIER_CLIENT_ID and TRADIER_CLIENT_SECRET. On first
//     run, Connect() prints an authorization URL. Access tokens expire in
//     24 hours; refresh tokens (partner accounts only) are used automatically.
//
// Both modes require TRADIER_ACCOUNT_ID.
//
// # Sandbox
//
// Use WithSandbox() to target the Tradier sandbox environment:
//
//	broker := tradier.New(tradier.WithSandbox())
//
// The sandbox uses paper money. Note: account streaming is not available
// in sandbox mode; the broker falls back to polling for fills.
//
// # Fill Delivery
//
// Fills are delivered via a WebSocket connection to Tradier's account
// events streamer. On disconnect, the broker reconnects with exponential
// backoff and polls for any fills missed during the outage. Duplicate
// fills are suppressed automatically.
//
// # Order Types
//
// Market, Limit, Stop, and StopLimit are supported. Duration supports
// Day and GTC. Dollar-amount orders (Qty=0, Amount>0) are converted to
// share quantities by fetching a real-time quote.
//
// # Order Groups
//
// TradierBroker implements broker.GroupSubmitter for native OCO and
// bracket (OTOCO) order support.
package tradier
```

- [ ] **Step 3: Write `errors.go`**

```go
package tradier

import "github.com/penny-vault/pvbt/broker"

var (
	ErrMissingCredentials  = broker.ErrMissingCredentials
	ErrNotAuthenticated    = broker.ErrNotAuthenticated
	ErrAccountNotFound     = broker.ErrAccountNotFound
	ErrOrderRejected       = broker.ErrOrderRejected
	ErrStreamDisconnected  = broker.ErrStreamDisconnected
	ErrEmptyOrderGroup     = broker.ErrEmptyOrderGroup
	ErrNoEntryOrder        = broker.ErrNoEntryOrder
	ErrMultipleEntryOrders = broker.ErrMultipleEntryOrders
)

// HTTPError is a type alias for broker.HTTPError.
type HTTPError = broker.HTTPError

// NewHTTPError creates an HTTPError with the given status code and message.
var NewHTTPError = broker.NewHTTPError

// IsTransient returns true if the error is a transient failure that should be retried.
var IsTransient = broker.IsTransient
```

- [ ] **Step 4: Write `tradier_suite_test.go`**

```go
package tradier_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTradier(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tradier Suite")
}
```

- [ ] **Step 5: Verify it compiles**

Run: `go build ./broker/tradier/`
Expected: Success (no output)

- [ ] **Step 6: Commit**

```bash
git add broker/tradier/
git commit -m "feat(tradier): scaffold package with doc, errors, and test suite"
```

---

### Task 2: Types and JSON collection helper

**Files:**
- Create: `broker/tradier/types.go`
- Create: `broker/tradier/types_test.go`

- [ ] **Step 1: Write the failing tests for the JSON collection quirk helper and type translation functions**

In `types_test.go`, write Ginkgo tests covering:
- `unmarshalFlexible[T]()`: single object `{"position": {...}}` -> `[]T` with 1 element
- `unmarshalFlexible[T]()`: array `{"position": [{...}, {...}]}` -> `[]T` with 2 elements
- `unmarshalFlexible[T]()`: null/missing -> empty `[]T`
- `toTradierOrderParams()`: Market/Buy/Day -> correct form params
- `toTradierOrderParams()`: Limit with LimitPrice -> includes `price` param
- `toTradierOrderParams()`: Stop with StopPrice -> includes `stop` param
- `toTradierOrderParams()`: StopLimit -> includes both `price` and `stop`
- `toTradierOrderParams()`: unsupported TIF (IOC, FOK, GTD, OnOpen, OnClose) -> error
- `toBrokerOrder()`: maps all Tradier statuses correctly (pending->Submitted, open->Open, filled->Filled, etc.)
- `toBrokerPosition()`: maps fields including negative qty for short; `MarkPrice` set to 0 (populated later by `Positions()`)
- `toBrokerPosition()`: `AvgOpenPrice` computed as `CostBasis / abs(Quantity)`
- `toBrokerBalance()`: maps margin account subsection
- `toBrokerBalance()`: maps cash account subsection
- `mapTradierSide()`: buy->Buy, sell->Sell, sell_short->Sell, buy_to_cover->Buy

- [ ] **Step 2: Run tests to verify they fail**

Run: `ginkgo run -race ./broker/tradier/`
Expected: FAIL -- functions not defined

- [ ] **Step 3: Write `types.go`**

Define all Tradier API types and translation functions:

**Tradier API types:**
- `tradierOrderResponse` -- fields: `ID` (int64), `Type`, `Symbol`, `Side`, `Quantity` (float64), `Status`, `Duration`, `AvgFillPrice`, `ExecQuantity`, `LastFillPrice`, `LastFillQuantity`, `RemainingQuantity`, `CreateDate`, `TransactionDate`, `Class`, `Price`, `Stop`, `Tag`
- `tradierOrderWrapper` -- `Order json.RawMessage` for the collection quirk
- `tradierOrdersWrapper` -- `Orders struct { Order json.RawMessage }` for the collection quirk
- `tradierPositionResponse` -- fields: `ID` (int64), `Symbol`, `Quantity` (float64), `CostBasis` (float64), `DateAcquired`
- `tradierPositionsWrapper` -- `Positions struct { Position json.RawMessage }` for the collection quirk
- `tradierBalanceResponse` -- fields: `AccountNumber`, `AccountType`, `TotalEquity`, `TotalCash`, `MarketValue`, `Margin` (sub-struct with `StockBuyingPower`, `CurrentRequirement`), `Cash` (sub-struct with `CashAvailable`, `UnsettledFunds`)
- `tradierBalancesWrapper` -- `Balances tradierBalanceResponse`
- `tradierQuoteResponse` -- `Quotes struct { Quote struct { Last float64 } }`
- `tradierOrderSubmitResponse` -- `Order struct { ID int64; Status string }`
- `tradierSessionResponse` -- `Stream struct { SessionID string; URL string }`
- `tradierAccountEvent` -- fields: `ID` (int64), `Event`, `Status`, `Type`, `Price`, `StopPrice`, `AvgFillPrice`, `ExecutedQuantity`, `LastFillQuantity`, `LastFillPrice`, `RemainingQuantity`, `TransactionDate`, `CreateDate`, `Account`

**Generic collection helper:**
```go
func unmarshalFlexible[T any](raw json.RawMessage) ([]T, error)
```
Checks if `raw` starts with `[` (array) or `{` (single object). Returns `[]T` in both cases. Returns empty slice for null/empty.

**Translation functions:**
- `toTradierOrderParams(order broker.Order) (url.Values, error)` -- maps broker.Order to form params; returns error for unsupported TIF
- `toBrokerOrder(resp tradierOrderResponse) broker.Order` -- maps response to broker.Order
- `toBrokerPosition(resp tradierPositionResponse) broker.Position` -- maps position; sets `MarkPrice` to 0 and `RealizedDayPL` to 0 (neither is available from Tradier's positions endpoint; `MarkPrice` is populated by the caller in `Positions()` via a supplementary quote fetch)
- `toBrokerBalance(resp tradierBalanceResponse) broker.Balance` -- reads margin or cash subsection
- Mapping helpers: `mapOrderType`, `mapSide`, `mapTimeInForce`, `mapTradierStatus`, `mapTradierSide`, `mapTradierOrderType`

- [ ] **Step 4: Run tests to verify they pass**

Run: `ginkgo run -race ./broker/tradier/`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./broker/tradier/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add broker/tradier/
git commit -m "feat(tradier): add API types, JSON collection helper, and translation functions"
```

---

### Task 3: REST API client

**Files:**
- Create: `broker/tradier/client.go`
- Create: `broker/tradier/client_test.go`
- Create: `broker/tradier/exports_test.go`

- [ ] **Step 1: Write `exports_test.go` with test helpers**

Export internal types and constructors needed by tests:
- `NewAPIClientForTest(baseURL, token, accountID string) *apiClient`
- `SetClientForTest(tradierBroker *TradierBroker, client *apiClient)`
- `SetAccountIDForTest(tradierBroker *TradierBroker, accountID string)`
- Exported method wrappers for each client method (SubmitOrder, CancelOrder, ModifyOrder, GetOrders, GetPositions, GetBalance, GetQuote, CreateStreamSession)
- Type aliases for test access to internal types

- [ ] **Step 2: Write failing tests for client methods**

In `client_test.go`, write Ginkgo tests using `httptest.NewServer()` mocks:
- `submitOrder`: POST to correct path, form-encoded body, returns order ID
- `submitOrder`: HTTP error returns wrapped error
- `cancelOrder`: DELETE to correct path
- `modifyOrder`: PUT with form params for price/stop/type/duration
- `getOrders`: GET returns parsed orders (test both single and array response)
- `getPositions`: GET returns parsed positions (test both single and array)
- `getBalance`: GET returns parsed balance (test margin account)
- `getBalance`: GET returns parsed balance (test cash account)
- `getQuote`: GET returns last price
- `createStreamSession`: POST returns session ID

Use the `authenticatedClient` helper pattern from Schwab tests: create mock server, register routes on mux, construct client via `NewAPIClientForTest`.

- [ ] **Step 3: Run tests to verify they fail**

Run: `ginkgo run -race ./broker/tradier/`
Expected: FAIL

- [ ] **Step 4: Write `client.go`**

```go
type apiClient struct {
    resty     *resty.Client
    accountID string
}
```

`newAPIClient(baseURL, token, accountID string) *apiClient`:
- `resty.New()` with `SetBaseURL`, `SetRetryCount(3)`, `SetRetryWaitTime(1s)`, `SetRetryMaxWaitTime(4s)`, `SetAuthToken(token)`, `SetHeader("Accept", "application/json")`
- Add retry condition using `broker.IsTransient`

Methods (all take `ctx context.Context`). Base URL includes `/v1`, so paths omit the `/v1` prefix:
- `submitOrder(ctx, params url.Values) (int64, error)` -- POST `/accounts/{accountID}/orders`, form body, parse `tradierOrderSubmitResponse`
- `cancelOrder(ctx, orderID string) error` -- DELETE `/accounts/{accountID}/orders/{orderID}`
- `modifyOrder(ctx, orderID string, params url.Values) error` -- PUT `/accounts/{accountID}/orders/{orderID}`, form body
- `getOrders(ctx) ([]tradierOrderResponse, error)` -- GET `/accounts/{accountID}/orders`, use `unmarshalFlexible` on the orders collection
- `getPositions(ctx) ([]tradierPositionResponse, error)` -- GET `/accounts/{accountID}/positions`, use `unmarshalFlexible`
- `getBalance(ctx) (tradierBalanceResponse, error)` -- GET `/accounts/{accountID}/balances`
- `getQuote(ctx, symbol string) (float64, error)` -- GET `/markets/quotes?symbols={symbol}`, return `last` price
- `createStreamSession(ctx) (string, error)` -- POST `/accounts/events/session`, return session ID
- `setToken(token string)` -- update the resty auth token (called on OAuth refresh)

Helper: `checkResponse(resp *resty.Response) error` -- check HTTP status, return `broker.NewHTTPError` for non-2xx.

- [ ] **Step 5: Run tests to verify they pass**

Run: `ginkgo run -race ./broker/tradier/`
Expected: PASS

- [ ] **Step 6: Run linter**

Run: `golangci-lint run ./broker/tradier/`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
git add broker/tradier/
git commit -m "feat(tradier): add REST API client with order, position, balance, and quote endpoints"
```

---

### Task 4: Authentication (dual-mode token manager)

**Files:**
- Create: `broker/tradier/auth.go`
- Create: `broker/tradier/auth_test.go`

- [ ] **Step 1: Write failing tests for auth**

In `auth_test.go`, write Ginkgo tests:
- `detectAuthMode`: `TRADIER_ACCESS_TOKEN` set -> returns static token mode
- `detectAuthMode`: `TRADIER_CLIENT_ID` + `TRADIER_CLIENT_SECRET` set -> returns OAuth mode
- `detectAuthMode`: neither set -> returns `broker.ErrMissingCredentials`
- `detectAuthMode`: `TRADIER_ACCESS_TOKEN` takes priority over OAuth env vars
- Token file persistence: `saveTokens` writes JSON, `loadTokens` reads it back
- Token expiry check: expired token -> `ensureValidToken` returns error
- Token expiry check: valid token -> `ensureValidToken` returns nil
- OAuth token exchange: mock server, POST with Basic auth, returns token
- OAuth token refresh: mock server, POST with refresh_token grant, returns new token

Use `GinkgoT().TempDir()` for token file tests. Use `t.Setenv()` for environment variable tests.

- [ ] **Step 2: Run tests to verify they fail**

Run: `ginkgo run -race ./broker/tradier/`
Expected: FAIL

- [ ] **Step 3: Write `auth.go`**

Constants:
```go
const (
    defaultCallbackURL   = "https://127.0.0.1:5174"
    defaultTokenFile     = "~/.config/pvbt/tradier-tokens.json"
    productionAuthURL    = "https://api.tradier.com"
    accessTokenBuffer    = 5 * time.Minute
    refreshCheckInterval = 30 * time.Minute
)
```

Types:
```go
type authMode int
const (
    authModeStatic authMode = iota
    authModeOAuth
)

type tokenStore struct {
    AccessToken     string    `json:"access_token"`
    RefreshToken    string    `json:"refresh_token"`
    AccessExpiresAt time.Time `json:"access_expires_at"`
}

type tokenManager struct {
    mode           authMode
    staticToken    string
    clientID       string
    clientSecret   string
    callbackURL    string
    tokenFile      string
    authBaseURL    string
    tokens         *tokenStore
    onRefresh      func(token string)
    mu             sync.Mutex
    stopRefresh    chan struct{}
    refreshWg      sync.WaitGroup
    listenerAddrCh chan string
}
```

Functions:
- `detectAuthMode() (authMode, error)` -- check env vars, return mode or error
- `newTokenManager(mode authMode, ...) *tokenManager` -- construct with defaults
- `expandHome(path string) string` -- expand `~` to home dir
- `loadTokens(path string) (*tokenStore, error)` -- read JSON file
- `saveTokens(path string, tokens *tokenStore) error` -- write JSON with 0600 perms, create parent dirs
- `(tm *tokenManager) accessToken() string` -- return current token (static or from store)
- `(tm *tokenManager) ensureValidToken() error` -- check expiry, refresh if possible
- `(tm *tokenManager) exchangeAuthCode(ctx context.Context, code string) error` -- POST to `/v1/oauth/accesstoken`
- `(tm *tokenManager) refreshAccessToken() error` -- POST to `/v1/oauth/refreshtoken`
- `(tm *tokenManager) startAuthFlow() error` -- browser-based OAuth flow (same pattern as `broker/schwab/auth.go`)
- `(tm *tokenManager) startBackgroundRefresh()` -- goroutine that refreshes before expiry
- `(tm *tokenManager) stopBackgroundRefresh()` -- signal stop, wait for goroutine

- [ ] **Step 4: Run tests to verify they pass**

Run: `ginkgo run -race ./broker/tradier/`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./broker/tradier/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add broker/tradier/
git commit -m "feat(tradier): add dual-mode authentication with static token and OAuth 2.0"
```

---

### Task 5: WebSocket streamer and sandbox polling fallback

**Files:**
- Create: `broker/tradier/streamer.go`
- Create: `broker/tradier/streamer_test.go`

- [ ] **Step 1: Write failing tests for the streamer**

In `streamer_test.go`, write Ginkgo tests:
- WebSocket connection: connects to URL, sends subscription JSON with session ID
- Fill detection: receives order event with `status=filled` -> delivers `broker.Fill` on channel
- Fill detection: receives order event with `status=partially_filled` -> delivers fill
- Fill detection: receives order event with `status=open` -> no fill delivered
- Deduplication: same fill event received twice -> only one fill delivered
- Close: closes WebSocket connection, stops read loop
- Sandbox polling: calls `getOrders`, detects new fills by comparing against previous state, delivers fills

Use `httptest.NewServer` with `websocket.Upgrader` for WebSocket tests. For sandbox polling, use a mock `apiClient` with `httptest`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `ginkgo run -race ./broker/tradier/`
Expected: FAIL

- [ ] **Step 3: Write `streamer.go`**

Constants:
```go
const (
    maxReconnectAttempts = 3
    pruneThreshold       = 24 * time.Hour
    sandboxPollInterval  = 2 * time.Second
    productionWSURL      = "wss://ws.tradier.com/v1/accounts/events"
    sandboxWSURL         = "wss://sandbox-ws.tradier.com/v1/accounts/events"
)
```

Types:
```go
type accountStreamer struct {
    client       *apiClient
    fills        chan broker.Fill
    wsURL        string
    wsConn       *websocket.Conn
    sessionID    string
    seenFills    map[string]time.Time
    mu           sync.Mutex
    done         chan struct{}
    wg           sync.WaitGroup
    ctx          context.Context
    lastPruneDay time.Time
    sandbox      bool
    lastOrders   map[int64]string // orderID -> last known status, for sandbox polling
}

type wsSubscription struct {
    Events          []string `json:"events"`
    SessionID       string   `json:"sessionid"`
    ExcludeAccounts []string `json:"excludeAccounts"`
}
```

Functions:
- `newAccountStreamer(client *apiClient, fills chan broker.Fill, wsURL string, sessionID string, sandbox bool) *accountStreamer`
- `(streamer *accountStreamer) connect(ctx context.Context) error` -- dial WebSocket, send subscription, start read loop goroutine
- `(streamer *accountStreamer) close() error` -- signal done, close WebSocket, wait for goroutine
- `(streamer *accountStreamer) readLoop()` -- read messages, parse order events, detect fills, deduplicate, deliver to channel; on error attempt reconnect
- `(streamer *accountStreamer) reconnect() error` -- create new session, reconnect with backoff (1s, 2s, 4s), poll for missed fills on success
- `(streamer *accountStreamer) pollForMissedFills()` -- call `client.getOrders`, check for filled orders not in `seenFills`
- `(streamer *accountStreamer) processEvent(event tradierAccountEvent)` -- check status, build fill key, deduplicate, send to channel
- `(streamer *accountStreamer) pruneSeen()` -- remove entries older than 24h, once per day
- `(streamer *accountStreamer) startPolling(ctx context.Context)` -- sandbox fallback: poll getOrders every 2s, compare against `lastOrders` map, deliver new fills

- [ ] **Step 4: Run tests to verify they pass**

Run: `ginkgo run -race ./broker/tradier/`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./broker/tradier/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add broker/tradier/
git commit -m "feat(tradier): add WebSocket account streamer with sandbox polling fallback"
```

---

### Task 6: Broker struct, constructor, and interface methods

**Files:**
- Create: `broker/tradier/broker.go`
- Create: `broker/tradier/broker_test.go`

- [ ] **Step 1: Write failing tests for broker methods**

In `broker_test.go`, write Ginkgo tests:

Compile-time checks:
```go
var _ broker.Broker = (*tradier.TradierBroker)(nil)
var _ broker.GroupSubmitter = (*tradier.TradierBroker)(nil)
```

Use the `authenticatedBroker` helper pattern (mock server + test exports):

- `Connect`: missing both `TRADIER_ACCESS_TOKEN` and `TRADIER_CLIENT_ID` -> returns `ErrMissingCredentials`
- `Connect`: missing `TRADIER_ACCOUNT_ID` -> returns `ErrMissingCredentials`
- `Close`: stops streamer, closes fills channel without error
- `Submit`: market buy -> POST with correct form params, returns nil
- `Submit`: dollar-amount order -> fetches quote, calculates qty, submits
- `Submit`: dollar-amount order where qty rounds to 0 -> returns error
- `Submit`: unsupported TIF -> returns error
- `Submit`: short sell (no long position) -> uses `sell_short` side
- `Cancel`: calls DELETE on correct path
- `Replace`: modifies price/type -> PUT with form params
- `Replace`: modifies quantity -> cancels original + submits new order
- `Orders`: returns mapped orders from GET response
- `Positions`: returns mapped positions with `MarkPrice` populated via supplementary quote fetch (mock both `/accounts/{id}/positions` and `/markets/quotes`)
- `Balance`: returns mapped balance (margin account)
- `Balance`: returns mapped balance (cash account)
- `SubmitGroup` with `GroupOCO`: POST with `class=oco` and indexed params
- `SubmitGroup` with `GroupBracket`: POST with `class=otoco` and indexed params
- `SubmitGroup` with empty orders -> returns `broker.ErrEmptyOrderGroup`
- `SubmitGroup` bracket with no entry order -> returns `broker.ErrNoEntryOrder`
- `SubmitGroup` bracket with multiple entry orders -> returns `broker.ErrMultipleEntryOrders`

- [ ] **Step 2: Run tests to verify they fail**

Run: `ginkgo run -race ./broker/tradier/`
Expected: FAIL

- [ ] **Step 3: Write `broker.go`**

Constants:
```go
const (
    fillChannelSize    = 1024
    productionBaseURL  = "https://api.tradier.com/v1"
    sandboxBaseURL     = "https://sandbox.tradier.com/v1"
)
```

Struct and options:
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

type Option func(*TradierBroker)

func WithSandbox() Option { ... }
func WithTokenFile(path string) Option { ... }
func WithCallbackURL(url string) Option { ... }

func New(opts ...Option) *TradierBroker { ... }
```

Interface methods:
- `Fills() <-chan broker.Fill` -- return fills channel
- `Connect(ctx) error`:
  1. Call `detectAuthMode()`, construct `tokenManager`
  2. Read `TRADIER_ACCOUNT_ID` (or return `ErrMissingCredentials`)
  3. For static mode: create client with token directly
  4. For OAuth: load tokens, ensure valid, start auth flow if needed
  5. If not sandbox: create streaming session via `client.createStreamSession()` and start WebSocket streamer
  6. If sandbox: start polling fallback (skip createStreamSession -- not available in sandbox)
  7. Start background refresh if OAuth with refresh token
- `Close() error` -- stop auth refresh, close streamer, close fills channel
- `Submit(ctx, order) error`:
  1. Lock mutex
  2. Dollar-amount conversion if needed (getQuote + math.Floor)
  3. Check positions for short sell detection
  4. Build form params via `toTradierOrderParams`
  5. Call `client.submitOrder`
  6. Verify order wasn't rejected by querying order status
- `Cancel(ctx, orderID) error` -- delegate to `client.cancelOrder`
- `Replace(ctx, orderID, order) error`:
  1. If quantity changed: cancel + submit new order
  2. Otherwise: build form params for type/price/stop/duration, call `client.modifyOrder`
- `Orders(ctx) ([]broker.Order, error)` -- get orders, map each via `toBrokerOrder`
- `Positions(ctx) ([]broker.Position, error)` -- get positions, map via `toBrokerPosition`, then fetch quotes for all position symbols via `client.getQuote` and populate `MarkPrice`
- `Balance(ctx) (broker.Balance, error)` -- get balance, map via `toBrokerBalance`
- `SubmitGroup(ctx, orders, groupType) error`:
  1. Check empty orders -> `ErrEmptyOrderGroup`
  2. Lock mutex
  3. Switch on groupType: `GroupOCO` -> `submitOCO`, `GroupBracket` -> `submitBracket`
- `submitOCO(ctx, orders) error` -- build `class=oco` form params with indexed legs, submit
- `submitBracket(ctx, orders) error`:
  1. Find entry order (RoleEntry) -- error if none or multiple
  2. Find stop-loss (RoleStopLoss) and take-profit (RoleTakeProfit)
  3. Build `class=otoco` form params: entry=leg[0], TP=leg[1], SL=leg[2]
  4. Submit

- [ ] **Step 4: Run tests to verify they pass**

Run: `ginkgo run -race ./broker/tradier/`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `ginkgo run -race ./...`
Expected: All tests PASS

- [ ] **Step 6: Run linter**

Run: `golangci-lint run ./broker/tradier/`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
git add broker/tradier/
git commit -m "feat(tradier): implement Broker and GroupSubmitter interfaces"
```

---

### Task 7: Final integration check and changelog

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Run full test suite**

Run: `ginkgo run -race ./...`
Expected: All tests PASS

- [ ] **Step 2: Run full linter**

Run: `golangci-lint run ./...`
Expected: No errors

- [ ] **Step 3: Update changelog**

Add under `[Unreleased]` -> `Added`:
- `Users can now trade through Tradier with support for market, limit, stop, and stop-limit orders, OCO and bracket groups, and real-time fill streaming`

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add Tradier broker to changelog"
```
