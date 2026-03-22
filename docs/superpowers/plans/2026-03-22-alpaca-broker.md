# Alpaca Broker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `broker.Broker` and `broker.GroupSubmitter` for Alpaca's v2 Trading API, with a prerequisite refactor to promote shared broker errors.

**Architecture:** New `broker/alpaca/` package mirroring the tastytrade layout (broker.go, client.go, types.go, streamer.go, errors.go, doc.go). Shared error sentinels and utilities move from `broker/tastytrade/errors.go` to `broker/errors.go` first, then both implementations reference them. REST via go-resty, WebSocket fills via gorilla/websocket.

**Tech Stack:** Go, go-resty/resty/v2, gorilla/websocket, Ginkgo/Gomega for tests

**Spec:** `docs/superpowers/specs/2026-03-22-alpaca-broker-design.md`

---

### Task 1: Promote shared errors from tastytrade to broker package

**Files:**
- Create: `broker/errors.go`
- Create: `broker/errors_test.go`
- Modify: `broker/tastytrade/errors.go`
- Modify: `broker/tastytrade/errors_test.go`
- Modify: `broker/tastytrade/exports_test.go`
- Unchanged (verify compiles): `broker/tastytrade/broker.go`, `broker/tastytrade/client.go`, `broker/tastytrade/streamer.go` -- these reference `ErrMissingCredentials`, `IsTransient`, etc. which become re-exports; no source changes needed.

**Behavioral change note:** The shared `IsTransient` adds 429 (rate limiting) as a transient error. Both tastytrade and Alpaca will now retry on 429. The `HTTPError` error prefix changes from `"tastytrade: HTTP ..."` to `"broker: HTTP ..."` in formatted messages.

- [ ] **Step 1: Write tests for shared broker errors**

Create `broker/errors_test.go` in the `broker_test` package (the suite file already exists at `broker/broker_suite_test.go`). Tests verify all sentinel errors exist and `IsTransient` handles 5xx, 429, net errors, DNS errors, url.Error wrapping, and rejects 4xx/generic errors.

```go
package broker_test

import (
	"errors"
	"fmt"
	"net"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("Errors", func() {
	Describe("Sentinel errors", func() {
		It("defines all expected sentinel errors", func() {
			Expect(broker.ErrMissingCredentials).To(MatchError("broker: missing credentials"))
			Expect(broker.ErrNotAuthenticated).To(MatchError("broker: not authenticated"))
			Expect(broker.ErrAccountNotFound).To(MatchError("broker: account not found"))
			Expect(broker.ErrAccountNotActive).To(MatchError("broker: account not active"))
			Expect(broker.ErrStreamDisconnected).To(MatchError("broker: stream disconnected"))
			Expect(broker.ErrEmptyOrderGroup).To(MatchError("broker: empty order group"))
			Expect(broker.ErrNoEntryOrder).To(MatchError("broker: no entry order in group"))
			Expect(broker.ErrMultipleEntryOrders).To(MatchError("broker: multiple entry orders in group"))
			Expect(broker.ErrOrderRejected).To(MatchError("broker: order rejected"))
		})
	})

	Describe("HTTPError", func() {
		It("formats the error message with status code", func() {
			httpErr := broker.NewHTTPError(500, "internal server error")
			Expect(httpErr.Error()).To(Equal("broker: HTTP 500: internal server error"))
		})
	})

	Describe("IsTransient", func() {
		It("returns true for HTTP 500 errors", func() {
			Expect(broker.IsTransient(broker.NewHTTPError(500, "error"))).To(BeTrue())
		})

		It("returns true for HTTP 502 errors", func() {
			Expect(broker.IsTransient(broker.NewHTTPError(502, "bad gateway"))).To(BeTrue())
		})

		It("returns true for HTTP 503 errors", func() {
			Expect(broker.IsTransient(broker.NewHTTPError(503, "unavailable"))).To(BeTrue())
		})

		It("returns true for HTTP 429 rate limit errors", func() {
			Expect(broker.IsTransient(broker.NewHTTPError(429, "rate limited"))).To(BeTrue())
		})

		It("returns false for HTTP 400 errors", func() {
			Expect(broker.IsTransient(broker.NewHTTPError(400, "bad request"))).To(BeFalse())
		})

		It("returns false for HTTP 401 errors", func() {
			Expect(broker.IsTransient(broker.NewHTTPError(401, "unauthorized"))).To(BeFalse())
		})

		It("returns false for HTTP 422 errors", func() {
			Expect(broker.IsTransient(broker.NewHTTPError(422, "unprocessable"))).To(BeFalse())
		})

		It("returns true for net.OpError", func() {
			netErr := &net.OpError{Op: "read", Err: &net.DNSError{IsTimeout: true}}
			Expect(broker.IsTransient(netErr)).To(BeTrue())
		})

		It("returns true for net.DNSError", func() {
			Expect(broker.IsTransient(&net.DNSError{Name: "api.example.com"})).To(BeTrue())
		})

		It("returns true for url.Error wrapping net error", func() {
			urlErr := &url.Error{Op: "Get", URL: "https://example.com", Err: &net.OpError{Op: "dial", Err: errors.New("refused")}}
			Expect(broker.IsTransient(urlErr)).To(BeTrue())
		})

		It("returns false for generic errors", func() {
			Expect(broker.IsTransient(errors.New("something"))).To(BeFalse())
		})

		It("returns false for nil", func() {
			Expect(broker.IsTransient(nil)).To(BeFalse())
		})

		It("returns true for wrapped transient errors", func() {
			netErr := &net.OpError{Op: "read", Err: &net.DNSError{IsTimeout: true}}
			wrapped := fmt.Errorf("request failed: %w", netErr)
			Expect(broker.IsTransient(wrapped)).To(BeTrue())
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./broker/ -v -run "Errors" -count=1`
Expected: Compilation failure -- `broker.ErrMissingCredentials` not defined.

- [ ] **Step 3: Create `broker/errors.go` with shared errors**

```go
package broker

import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

var (
	ErrMissingCredentials  = errors.New("broker: missing credentials")
	ErrNotAuthenticated    = errors.New("broker: not authenticated")
	ErrAccountNotFound     = errors.New("broker: account not found")
	ErrAccountNotActive    = errors.New("broker: account not active")
	ErrStreamDisconnected  = errors.New("broker: stream disconnected")
	ErrEmptyOrderGroup     = errors.New("broker: empty order group")
	ErrNoEntryOrder        = errors.New("broker: no entry order in group")
	ErrMultipleEntryOrders = errors.New("broker: multiple entry orders in group")
	ErrOrderRejected       = errors.New("broker: order rejected")
)

// HTTPError represents an HTTP response with a non-2xx status code.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (httpErr *HTTPError) Error() string {
	return fmt.Sprintf("broker: HTTP %d: %s", httpErr.StatusCode, httpErr.Message)
}

// NewHTTPError creates an HTTPError with the given status code and message.
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{StatusCode: statusCode, Message: message}
}

// IsTransient returns true if the error is a transient failure that should
// be retried (network errors, HTTP 5xx, HTTP 429 rate limiting).
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 429 || httpErr.StatusCode >= 500
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return IsTransient(urlErr.Err)
	}

	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./broker/ -v -count=1`
Expected: All PASS including new error tests and existing broker_test.go tests.

- [ ] **Step 5: Refactor tastytrade to use shared errors**

Update `broker/tastytrade/errors.go` to remove duplicated sentinels and re-export from `broker` package. Keep only `ErrNotAuthenticated` as tastytrade-specific if its message differs, but alias the shared ones. Replace the tastytrade-prefixed error messages with broker-level ones. Delete `HTTPError`, `NewHTTPError`, and `IsTransient` from tastytrade since they now live in `broker`.

The tastytrade `errors.go` should become:

```go
package tastytrade

import "github.com/penny-vault/pvbt/broker"

// Re-export shared broker errors for backward compatibility with tests
// that reference tastytrade.ErrMissingCredentials, etc.
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

// HTTPError, NewHTTPError, and IsTransient are now in the broker package.
// Re-export for backward compatibility.
type HTTPError = broker.HTTPError

var NewHTTPError = broker.NewHTTPError
var IsTransient = broker.IsTransient
```

Update `broker/tastytrade/exports_test.go`: remove the `HTTPError` type alias export since it's now re-exported from the main tastytrade package.

- [ ] **Step 6: Update tastytrade tests for new error messages**

The shared errors use `broker:` prefix instead of `tastytrade:`. Update `broker/tastytrade/errors_test.go` sentinel error assertions to match the new messages (e.g., `"broker: missing credentials"` instead of `"tastytrade: TASTYTRADE_USERNAME and TASTYTRADE_PASSWORD must be set"`).

- [ ] **Step 7: Run all tastytrade tests**

Run: `go test ./broker/tastytrade/ -v -count=1`
Expected: All PASS. The re-exported aliases ensure existing test references work.

- [ ] **Step 8: Run full broker test suite**

Run: `go test ./broker/... -v -count=1`
Expected: All PASS across both `broker` and `broker/tastytrade`.

- [ ] **Step 9: Commit**

```bash
git add broker/errors.go broker/errors_test.go broker/tastytrade/errors.go broker/tastytrade/errors_test.go broker/tastytrade/exports_test.go
git commit -m "refactor: promote shared errors from tastytrade to broker package"
```

---

### Task 2: Alpaca types and mapping functions

**Files:**
- Create: `broker/alpaca/types.go`
- Create: `broker/alpaca/types_test.go`
- Create: `broker/alpaca/alpaca_suite_test.go`

- [ ] **Step 1: Create the Ginkgo test suite bootstrap**

```go
// broker/alpaca/alpaca_suite_test.go
package alpaca_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAlpaca(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alpaca Suite")
}
```

- [ ] **Step 2: Write tests for mapping functions**

Create `broker/alpaca/types_test.go` in the internal `alpaca` package (not `alpaca_test`) so it can access unexported mapping functions directly, matching the tastytrade pattern at `broker/tastytrade/types_test.go`. Both `package alpaca` and `package alpaca_test` test files coexist in the same directory -- this is standard Go practice. The suite file uses `package alpaca_test` (external), while types_test.go uses `package alpaca` (internal/white-box).

Tests should cover:
- `toAlpacaOrder`: market buy, limit sell, stop, stop-limit, all TIF values, GTD with expire_time, dollar-amount with notional (fractional mode), dollar-amount with qty (non-fractional)
- `toBrokerOrder`: all Alpaca status strings mapped correctly
- `toBrokerPosition`: field mapping
- `toBrokerBalance`: field mapping from account response
- `mapSide`, `mapOrderType`, `mapTimeInForce`, `mapAlpacaStatus`

```go
// broker/alpaca/types_test.go
package alpaca

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("Types", Label("translation"), func() {
	Describe("toAlpacaOrder", func() {
		It("translates a market buy order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result := toAlpacaOrder(order, false)

			Expect(result.Symbol).To(Equal("AAPL"))
			Expect(result.Side).To(Equal("buy"))
			Expect(result.Type).To(Equal("market"))
			Expect(result.TimeInForce).To(Equal("day"))
			Expect(result.Qty).To(Equal("100"))
			Expect(result.Notional).To(BeEmpty())
			Expect(result.LimitPrice).To(BeEmpty())
			Expect(result.StopPrice).To(BeEmpty())
		})

		It("translates a limit sell order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Sell,
				Qty:         50,
				OrderType:   broker.Limit,
				LimitPrice:  350.0,
				TimeInForce: broker.GTC,
			}

			result := toAlpacaOrder(order, false)

			Expect(result.Side).To(Equal("sell"))
			Expect(result.Type).To(Equal("limit"))
			Expect(result.TimeInForce).To(Equal("gtc"))
			Expect(result.LimitPrice).To(Equal("350"))
		})

		It("translates a stop order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "TSLA"},
				Side:        broker.Sell,
				Qty:         25,
				OrderType:   broker.Stop,
				StopPrice:   200.0,
				TimeInForce: broker.Day,
			}

			result := toAlpacaOrder(order, false)

			Expect(result.Type).To(Equal("stop"))
			Expect(result.StopPrice).To(Equal("200"))
			Expect(result.LimitPrice).To(BeEmpty())
		})

		It("translates a stop-limit order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "GOOG"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.StopLimit,
				StopPrice:   150.0,
				LimitPrice:  155.0,
				TimeInForce: broker.Day,
			}

			result := toAlpacaOrder(order, false)

			Expect(result.Type).To(Equal("stop_limit"))
			Expect(result.StopPrice).To(Equal("150"))
			Expect(result.LimitPrice).To(Equal("155"))
		})

		It("sets expire_time for GTD orders", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.GTD,
				GTDDate:     time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
			}

			result := toAlpacaOrder(order, false)

			Expect(result.TimeInForce).To(Equal("gtd"))
			Expect(result.ExpireTime).To(Equal("2026-04-15T00:00:00Z"))
		})

		It("maps OnOpen to opg", func() {
			order := broker.Order{
				Asset: asset.Asset{Ticker: "X"}, Side: broker.Buy, Qty: 1,
				OrderType: broker.Market, TimeInForce: broker.OnOpen,
			}
			result := toAlpacaOrder(order, false)
			Expect(result.TimeInForce).To(Equal("opg"))
		})

		It("maps OnClose to cls", func() {
			order := broker.Order{
				Asset: asset.Asset{Ticker: "X"}, Side: broker.Buy, Qty: 1,
				OrderType: broker.Market, TimeInForce: broker.OnClose,
			}
			result := toAlpacaOrder(order, false)
			Expect(result.TimeInForce).To(Equal("cls"))
		})

		It("uses notional for dollar-amount orders when fractional is enabled", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      5000.0,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result := toAlpacaOrder(order, true)

			Expect(result.Notional).To(Equal("5000"))
			Expect(result.Qty).To(BeEmpty())
		})

		It("sets client_order_id", func() {
			order := broker.Order{
				Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 1,
				OrderType: broker.Market, TimeInForce: broker.Day,
			}
			result := toAlpacaOrder(order, false)
			Expect(result.ClientOrderID).NotTo(BeEmpty())
		})
	})

	Describe("toBrokerOrder", func() {
		It("translates an order response", func() {
			resp := orderResponse{
				ID:            "alp-order-1",
				Status:        "new",
				Type:          "limit",
				Side:          "buy",
				Symbol:        "AAPL",
				Qty:           "100",
				LimitPrice:    "150.50",
				FilledQty:     "0",
				FilledAvgPrice: "0",
			}

			result := toBrokerOrder(resp)

			Expect(result.ID).To(Equal("alp-order-1"))
			Expect(result.Status).To(Equal(broker.OrderSubmitted))
			Expect(result.OrderType).To(Equal(broker.Limit))
			Expect(result.Side).To(Equal(broker.Buy))
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.LimitPrice).To(Equal(150.50))
		})

		It("maps all Alpaca status values", func() {
			for _, tc := range []struct {
				alpacaStatus string
				expected     broker.OrderStatus
			}{
				{"new", broker.OrderSubmitted},
				{"accepted", broker.OrderSubmitted},
				{"pending_new", broker.OrderSubmitted},
				{"partially_filled", broker.OrderPartiallyFilled},
				{"filled", broker.OrderFilled},
				{"canceled", broker.OrderCancelled},
				{"expired", broker.OrderCancelled},
				{"rejected", broker.OrderCancelled},
				{"suspended", broker.OrderCancelled},
			} {
				resp := orderResponse{Status: tc.alpacaStatus}
				result := toBrokerOrder(resp)
				Expect(result.Status).To(Equal(tc.expected), "for status %q", tc.alpacaStatus)
			}
		})
	})

	Describe("toBrokerPosition", func() {
		It("translates a position response", func() {
			resp := positionResponse{
				Symbol:               "AAPL",
				Qty:                  "100",
				AvgEntryPrice:        "150.25",
				CurrentPrice:         "155.00",
				UnrealizedIntradayPL: "475.00",
			}

			result := toBrokerPosition(resp)

			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.AvgOpenPrice).To(Equal(150.25))
			Expect(result.MarkPrice).To(Equal(155.0))
			Expect(result.RealizedDayPL).To(Equal(475.0))
		})
	})

	Describe("toBrokerBalance", func() {
		It("translates an account response", func() {
			resp := accountResponse{
				Cash:              "25000.50",
				Equity:            "50000.00",
				BuyingPower:       "45000.00",
				MaintenanceMargin: "5000.00",
			}

			result := toBrokerBalance(resp)

			Expect(result.CashBalance).To(Equal(25000.50))
			Expect(result.NetLiquidatingValue).To(Equal(50000.0))
			Expect(result.EquityBuyingPower).To(Equal(45000.0))
			Expect(result.MaintenanceReq).To(Equal(5000.0))
		})
	})
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./broker/alpaca/ -v -count=1`
Expected: Compilation failure -- types and functions not defined.

- [ ] **Step 4: Implement types.go**

Create `broker/alpaca/types.go` with request/response structs and all mapping functions. Alpaca uses string representations for numeric fields (prices, quantities). The mapping functions parse strings to float64.

Key structs:
- `orderRequest` -- fields: Symbol, Qty, Notional, Side, Type, TimeInForce, LimitPrice, StopPrice, ExpireTime, ClientOrderID, OrderClass, TakeProfit, StopLoss
- `orderResponse` -- fields from Alpaca's order object
- `positionResponse` -- fields from Alpaca's position object
- `accountResponse` -- fields from Alpaca's account object
- `quoteResponse` -- for latest quote endpoint
- `replaceRequest` -- mutable fields only for PATCH

Key functions:
- `toAlpacaOrder(order broker.Order, fractional bool) orderRequest`
- `toBrokerOrder(resp orderResponse) broker.Order`
- `toBrokerPosition(resp positionResponse) broker.Position`
- `toBrokerBalance(resp accountResponse) broker.Balance`
- `mapSide`, `mapOrderType`, `mapTimeInForce`, `mapAlpacaStatus`, `mapAlpacaSide`, `mapAlpacaOrderType`

Note: Alpaca returns all numeric values as strings. Use `strconv.ParseFloat` for parsing; return 0 on parse failure. For outbound prices/quantities, use `strconv.FormatFloat(value, 'f', -1, 64)` which produces `"350"` for 350.0 and `"150.5"` for 150.5 (no trailing zeros). Alpaca accepts both formats.

Note: The spec lists `broker/alpaca/errors.go` in the package structure. Since all errors are shared from `broker/errors.go` and there are no Alpaca-specific sentinel errors, this file is omitted. If Alpaca-specific errors are needed later, create it then.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./broker/alpaca/ -v -count=1`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add broker/alpaca/alpaca_suite_test.go broker/alpaca/types.go broker/alpaca/types_test.go
git commit -m "feat(alpaca): add request/response types and mapping functions"
```

---

### Task 3: Alpaca REST client

**Files:**
- Create: `broker/alpaca/client.go`
- Create: `broker/alpaca/client_test.go`
- Create: `broker/alpaca/exports_test.go`

- [ ] **Step 1: Write tests for the API client**

Create `broker/alpaca/client_test.go` in the `alpaca_test` package. Use `httptest.NewServer` to mock Alpaca's endpoints. Tests cover:
- `getAccount`: returns account data, handles errors
- `submitOrder`: sends correct JSON body, returns order ID
- `cancelOrder`: sends DELETE to correct path
- `replaceOrder`: sends PATCH with mutable fields
- `getOrders`: retrieves and parses order list
- `getPositions`: retrieves and parses positions
- `getQuote`: retrieves latest quote price
- Retry on 500: retries and eventually succeeds

Follow the tastytrade client_test.go pattern: create a helper `newClient` function that sets up the server and returns a configured client.

- [ ] **Step 2: Create `broker/alpaca/exports_test.go`**

Export unexported types and methods for the `_test` package, following the pattern in `broker/tastytrade/exports_test.go`. Export:
- `APIClientForTestType = apiClient`
- `NewAPIClientForTest(baseURL, apiKey, apiSecret string) *apiClient`
- Exported method wrappers: `GetAccount`, `SubmitOrder`, `CancelOrder`, `ReplaceOrder`, `GetOrders`, `GetPositions`, `GetQuote`
- Type aliases: `OrderRequest = orderRequest`, `OrderResponse = orderResponse`, `ReplaceRequest = replaceRequest`, `AccountResponse = accountResponse`, `PositionResponse = positionResponse`

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./broker/alpaca/ -v -count=1`
Expected: Compilation failure -- `apiClient` not defined.

- [ ] **Step 4: Implement client.go**

Create `broker/alpaca/client.go` with the `apiClient` struct and methods:

```go
type apiClient struct {
	resty     *resty.Client
	apiKey    string
	apiSecret string
}
```

Constructor `newAPIClient(baseURL, apiKey, apiSecret string)` configures resty with:
- Base URL
- 3 retries, 1s wait, 4s max
- Retry condition using `broker.IsTransient`
- Headers: `APCA-API-KEY-ID` and `APCA-API-SECRET-KEY` set once
- Content-Type: application/json

Methods:
- `getAccount(ctx) (accountResponse, error)` -- GET /v2/account
- `submitOrder(ctx, orderRequest) (string, error)` -- POST /v2/orders, returns order ID
- `cancelOrder(ctx, orderID string) error` -- DELETE /v2/orders/{id}
- `replaceOrder(ctx, orderID string, replaceRequest) (string, error)` -- PATCH /v2/orders/{id}
- `getOrders(ctx) ([]orderResponse, error)` -- GET /v2/orders?status=open&limit=500
- `getPositions(ctx) ([]positionResponse, error)` -- GET /v2/positions
- `getQuote(ctx, symbol string) (float64, error)` -- GET /v2/stocks/{symbol}/quotes/latest

Note: Alpaca returns order responses directly (no `data` envelope like tastytrade), so response types map directly to the JSON body.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./broker/alpaca/ -v -count=1`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add broker/alpaca/client.go broker/alpaca/client_test.go broker/alpaca/exports_test.go
git commit -m "feat(alpaca): add REST API client"
```

---

### Task 4: Alpaca WebSocket fill streamer

**Files:**
- Create: `broker/alpaca/streamer.go`
- Create: `broker/alpaca/streamer_test.go`
- Modify: `broker/alpaca/exports_test.go`

- [ ] **Step 1: Write tests for the fill streamer**

Create `broker/alpaca/streamer_test.go` using a local WebSocket test server (same pattern as `broker/tastytrade/streamer_test.go`). Tests cover:

1. **Auth flow**: streamer sends auth message with key/secret, receives authorized response, sends listen message
2. **Fill delivery**: server sends a `fill` trade update, streamer delivers a `broker.Fill` with correct fields
3. **Partial fill**: server sends `partial_fill`, streamer delivers fill
4. **Deduplication**: same `execution_id` sent twice, only one fill delivered
5. **Shutdown**: `CloseStreamer()` returns promptly

The test WebSocket server should:
- Accept upgrade
- Read auth message, verify key/secret, respond with authorized
- Read listen message, respond with acknowledgement
- Send trade update messages as needed per test

- [ ] **Step 2: Add streamer exports to exports_test.go**

Add to `broker/alpaca/exports_test.go`:
- `FillStreamerForTestType = fillStreamer`
- `NewFillStreamerForTest(apiKey, apiSecret string, fills chan broker.Fill, wsURL string) *fillStreamer`
- `ConnectStreamer(ctx) error` and `CloseStreamer() error` method exports

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./broker/alpaca/ -v -run "fillStreamer" -count=1`
Expected: Compilation failure -- `fillStreamer` not defined.

- [ ] **Step 4: Implement streamer.go**

Create `broker/alpaca/streamer.go` with:

```go
type fillStreamer struct {
	apiKey    string
	apiSecret string
	fills     chan broker.Fill
	wsURL     string
	wsConn    *websocket.Conn
	seenFills map[string]time.Time
	mu        sync.Mutex
	done      chan struct{}
	wg        sync.WaitGroup
	ctx       context.Context
	client    *apiClient      // for polling missed fills on reconnect
	lastPruneDay time.Time
}
```

Connection flow:
1. Dial WebSocket
2. Send `{"action": "auth", "key": "...", "secret": "..."}`
3. Read with 10s deadline, verify `{"stream": "authorization", "data": {"status": "authorized"}}`. If status is not "authorized", return `broker.ErrNotAuthenticated`.
4. Send `{"action": "listen", "data": {"streams": ["trade_updates"]}}`
5. Read with 10s deadline (listen ack -- log and continue if format differs)
6. Start background `run()` goroutine

The `run()` loop:
- Uses `readPump` goroutine pattern (same as tastytrade)
- Sends WebSocket ping frames every 30 seconds via a ticker
- On message: parse trade update, check event type (`fill` or `partial_fill`), extract `execution_id`/`price`/`qty`/`timestamp` and order ID, deduplicate, deliver fill
- On read error: reconnect with exponential backoff (1s, 2s, 4s, max 3 attempts), poll missed fills via `client.getOrders` on reconnect

Trade update message format:
```json
{"stream": "trade_updates", "data": {"event": "fill", "execution_id": "...", "price": "...", "qty": "...", "timestamp": "...", "order": {"id": "..."}}}
```

Pruning: same `pruneSeenFills` logic as tastytrade (24h threshold, once per day).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./broker/alpaca/ -v -run "fillStreamer" -count=1`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add broker/alpaca/streamer.go broker/alpaca/streamer_test.go broker/alpaca/exports_test.go
git commit -m "feat(alpaca): add WebSocket fill streamer"
```

---

### Task 5: AlpacaBroker struct (broker.go)

**Files:**
- Create: `broker/alpaca/broker.go`
- Create: `broker/alpaca/broker_test.go`
- Create: `broker/alpaca/doc.go`

- [ ] **Step 1: Write tests for AlpacaBroker**

Create `broker/alpaca/broker_test.go` in the `alpaca_test` package. Follow the tastytrade `broker_test.go` pattern with an `authenticatedBroker` helper that sets up an httptest server with a `/v2/account` handler and returns a configured broker.

Tests cover:
1. **Compile-time interface checks**: `var _ broker.Broker = (*alpaca.AlpacaBroker)(nil)` and `var _ broker.GroupSubmitter = (*alpaca.AlpacaBroker)(nil)`
2. **Constructor**: `New()` returns broker with non-nil fills channel; `WithPaper()` and `WithFractionalShares()` options work
3. **Connect**: returns `broker.ErrMissingCredentials` when env vars missing; returns `broker.ErrAccountNotActive` when account is not ACTIVE
4. **Submit qty-based order**: sends correct JSON, returns no error
5. **Submit dollar-amount order (non-fractional)**: fetches quote, computes qty, sends order
6. **Submit dollar-amount order (fractional)**: sends notional field
7. **Submit zero-qty dollar-amount**: returns nil without calling submit endpoint
8. **Cancel**: calls DELETE on correct path
9. **Replace mutable-only**: calls PATCH
10. **Replace non-mutable**: calls DELETE then POST
11. **Replace when cancel returns 422**: returns error without submitting replacement
12. **Orders**: retrieves and maps orders
13. **Positions**: retrieves and maps positions
14. **Balance**: retrieves account and maps to Balance
15. **SubmitGroup OCO**: sends correct order_class
16. **SubmitGroup Bracket**: sends bracket with take_profit and stop_loss
17. **SubmitGroup empty**: returns `broker.ErrEmptyOrderGroup`
18. **SubmitGroup no entry**: returns `broker.ErrNoEntryOrder`
19. **SubmitGroup multiple entries**: returns `broker.ErrMultipleEntryOrders`
20. **Close without streamer**: returns nil

- [ ] **Step 2: Add broker exports to exports_test.go**

Add to `broker/alpaca/exports_test.go`:
- `SetClientBaseURLForTest(alpacaBroker *AlpacaBroker, baseURL string)`
- `SetClientCredsForTest(alpacaBroker *AlpacaBroker, apiKey, apiSecret string)`

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./broker/alpaca/ -v -run "AlpacaBroker" -count=1`
Expected: Compilation failure -- `AlpacaBroker` not defined.

- [ ] **Step 4: Implement broker.go**

Create `broker/alpaca/broker.go` with:

```go
package alpaca

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/penny-vault/pvbt/broker"
)

const (
	productionBaseURL = "https://api.alpaca.markets"
	paperBaseURL      = "https://paper-api.alpaca.markets"
	productionWSURL   = "wss://api.alpaca.markets/stream"
	paperWSURL        = "wss://paper-api.alpaca.markets/stream"
	fillChannelSize   = 1024
)

type AlpacaBroker struct {
	client          *apiClient
	streamer        *fillStreamer
	fills           chan broker.Fill
	paper           bool
	fractional      bool
	submittedOrders map[string]broker.Order
	mu              sync.Mutex
}

type Option func(*AlpacaBroker)

func WithPaper() Option {
	return func(alpacaBroker *AlpacaBroker) {
		alpacaBroker.paper = true
	}
}

func WithFractionalShares() Option {
	return func(alpacaBroker *AlpacaBroker) {
		alpacaBroker.fractional = true
	}
}

func New(opts ...Option) *AlpacaBroker { ... }
func (alpacaBroker *AlpacaBroker) Connect(ctx context.Context) error { ... }
func (alpacaBroker *AlpacaBroker) Close() error { ... }
func (alpacaBroker *AlpacaBroker) Fills() <-chan broker.Fill { ... }
func (alpacaBroker *AlpacaBroker) Submit(ctx context.Context, order broker.Order) error { ... }
func (alpacaBroker *AlpacaBroker) Cancel(ctx context.Context, orderID string) error { ... }
func (alpacaBroker *AlpacaBroker) Replace(ctx context.Context, orderID string, order broker.Order) error { ... }
func (alpacaBroker *AlpacaBroker) Orders(ctx context.Context) ([]broker.Order, error) { ... }
func (alpacaBroker *AlpacaBroker) Positions(ctx context.Context) ([]broker.Position, error) { ... }
func (alpacaBroker *AlpacaBroker) Balance(ctx context.Context) (broker.Balance, error) { ... }
func (alpacaBroker *AlpacaBroker) SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error { ... }
```

Key implementation details:
- `Connect`: reads `ALPACA_API_KEY`/`ALPACA_API_SECRET` from env, calls `getAccount` to validate, starts streamer
- `Submit`: converts order via `toAlpacaOrder`, handles dollar-amount (fractional vs whole-share), stores in `submittedOrders`
- `Replace`: lock mutex to read/copy original from `submittedOrders`, unlock, then compare non-mutable fields (Asset.Ticker, Side, OrderType). Make HTTP calls (PATCH or DELETE+POST) without holding the mutex. Re-lock to update the map after the call completes. On 422 from cancel, return error without submitting replacement.
- `SubmitGroup` for Bracket: finds entry order, builds bracket request with `take_profit`/`stop_loss` sub-objects
- `SubmitGroup` for OCO: builds OCO request with two legs

- [ ] **Step 5: Create doc.go**

```go
// Package alpaca implements broker.Broker for the Alpaca brokerage.
//
// # Authentication
//
// The broker reads credentials from environment variables:
//
//   - ALPACA_API_KEY: Alpaca API key ID
//   - ALPACA_API_SECRET: Alpaca API secret key
//
// # Paper Trading
//
// Use WithPaper() to target Alpaca's paper trading environment:
//
//	alpacaBroker := alpaca.New(alpaca.WithPaper())
//
// # Fractional Shares
//
// Use WithFractionalShares() to enable dollar-amount orders using
// Alpaca's notional field instead of computing whole-share quantities:
//
//	alpacaBroker := alpaca.New(alpaca.WithFractionalShares())
//
// # Fill Delivery
//
// Fills are delivered via a WebSocket connection to Alpaca's trade
// updates stream. On disconnect, the broker reconnects with exponential
// backoff and polls for any fills missed during the outage.
//
// # Order Groups
//
// AlpacaBroker implements broker.GroupSubmitter for native bracket and
// OCO order support via Alpaca's order_class parameter.
package alpaca
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./broker/alpaca/ -v -count=1`
Expected: All PASS.

- [ ] **Step 7: Run full broker test suite**

Run: `go test ./broker/... -v -count=1`
Expected: All PASS across `broker`, `broker/tastytrade`, and `broker/alpaca`.

- [ ] **Step 8: Commit**

```bash
git add broker/alpaca/broker.go broker/alpaca/broker_test.go broker/alpaca/doc.go broker/alpaca/exports_test.go
git commit -m "feat(alpaca): implement AlpacaBroker with Broker and GroupSubmitter interfaces"
```

---

### Task 6: Update broker documentation

**Files:**
- Modify: `docs/broker.md`

- [ ] **Step 1: Add Alpaca section to broker.md**

Add an "### Alpaca" section under "## Implementations" (after the tastytrade section), documenting:
- Package import path
- Constructor with options (`WithPaper()`, `WithFractionalShares()`)
- Environment variables table
- Usage example with engine
- Note about fill delivery via WebSocket
- Note about OCO/bracket support

Follow the same structure as the existing tastytrade section.

- [ ] **Step 2: Update "Other brokers" section**

The "Other brokers" section at the end of broker.md says "Additional brokers can be added by implementing the Broker interface." Keep this section but it's now less prominent since there are two concrete implementations.

- [ ] **Step 3: Commit**

```bash
git add docs/broker.md
git commit -m "docs: add Alpaca broker to broker documentation"
```

---

### Task 7: Integration test

**Files:**
- Create: `broker/alpaca/integration_test.go`

- [ ] **Step 1: Write integration test**

Create `broker/alpaca/integration_test.go` using the Ginkgo `Label("integration")` pattern from tastytrade's integration test. Tests skip when `ALPACA_API_KEY` is not set.

Tests cover:
- Connects to paper trading and retrieves balance (net liquidating value > 0)
- Retrieves positions (not nil)
- Retrieves orders (not nil)
- Submits and cancels a limit order (buy 1 share AAPL at $1.00 limit, then cancel)

```go
package alpaca_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/alpaca"
)

var _ = Describe("Integration", Label("integration"), func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		alpacaBroker  *alpaca.AlpacaBroker
	)

	BeforeEach(func() {
		if os.Getenv("ALPACA_API_KEY") == "" {
			Skip("ALPACA_API_KEY not set")
		}

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		alpacaBroker = alpaca.New(alpaca.WithPaper())
		Expect(alpacaBroker.Connect(ctx)).To(Succeed())
	})

	AfterEach(func() {
		if alpacaBroker != nil {
			alpacaBroker.Close()
		}
		cancel()
	})

	It("connects and retrieves balance", func() {
		balance, err := alpacaBroker.Balance(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(balance.NetLiquidatingValue).To(BeNumerically(">", 0))
	})

	It("retrieves positions", func() {
		positions, err := alpacaBroker.Positions(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(positions).NotTo(BeNil())
	})

	It("retrieves orders", func() {
		orders, err := alpacaBroker.Orders(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(orders).NotTo(BeNil())
	})

	It("submits and cancels a limit order", Label("orders"), func() {
		err := alpacaBroker.Submit(ctx, broker.Order{
			Asset:       asset.Asset{Ticker: "AAPL"},
			Side:        broker.Buy,
			Qty:         1,
			OrderType:   broker.Limit,
			LimitPrice:  1.00,
			TimeInForce: broker.Day,
		})
		Expect(err).NotTo(HaveOccurred())

		orders, err := alpacaBroker.Orders(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(orders).NotTo(BeEmpty())

		err = alpacaBroker.Cancel(ctx, orders[0].ID)
		Expect(err).NotTo(HaveOccurred())
	})
})
```

- [ ] **Step 2: Run integration test (if credentials available)**

Run: `go test ./broker/alpaca/ -v -count=1 --ginkgo.label-filter=integration`
Expected: PASS (or skip if ALPACA_API_KEY not set).

- [ ] **Step 3: Commit**

```bash
git add broker/alpaca/integration_test.go
git commit -m "test(alpaca): add integration tests for paper trading"
```

---

### Task 8: Final lint and verification

- [ ] **Step 1: Run linter**

Run: `golangci-lint run ./broker/...`
Expected: No issues. Fix any that appear.

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -count=1`
Expected: All PASS across entire project.

- [ ] **Step 3: Commit any lint fixes**

```bash
git add -A
git commit -m "fix: resolve lint issues in broker packages"
```

(Skip if no lint fixes were needed.)

---

### Task 9: Changelog entry

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Add an entry under the "Unreleased" section describing the new Alpaca broker. Follow the project's changelog style: complete sentences, active voice, user-facing language, related items combined.

Example entry:
```
- Added Alpaca as a live broker implementation, supporting commission-free trading with market, limit, stop, and stop-limit orders, OCO and bracket order groups, and optional fractional share support via the `WithFractionalShares()` option. Promoted shared broker error sentinels so all broker implementations use a consistent error vocabulary.
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add changelog entry for Alpaca broker"
```
