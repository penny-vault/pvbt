# Interactive Brokers Broker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `broker.Broker` and `broker.GroupSubmitter` interfaces for Interactive Brokers, with dual auth backends (OAuth and Client Portal Gateway) and a cross-broker refactor introducing `broker.ErrRateLimited` and renaming `broker.IsTransient` to `broker.IsRetryableError`.

**Architecture:** The work splits into two phases. Phase 1 is a cross-broker refactor adding `broker.ErrRateLimited` and renaming `broker.IsTransient` to `broker.IsRetryableError` across all existing brokers. Phase 2 is a new `broker/ibkr/` package with eight source files: `broker.go` (IBBroker struct + Broker/GroupSubmitter methods), `auth.go` (Authenticator interface + OAuth/Gateway implementations), `client.go` (rate-limited REST client via go-resty/v2), `streamer.go` (WebSocket order/fill streamer), `types.go` (IB request/response structs + mapping functions), `errors.go` (ErrConidNotFound), `doc.go`, and `exports_test.go`. Each auth backend decorates HTTP requests differently but exposes the same `Authenticator` interface, so all endpoint logic is shared.

**Tech Stack:** Go, go-resty/resty/v2 (HTTP), gorilla/websocket (WebSocket), golang.org/x/time/rate (rate limiting), Ginkgo v2 / Gomega (tests), zerolog (logging)

**Spec:** `docs/superpowers/specs/2026-03-22-ibkr-broker-design.md`

---

### Task 1: Cross-broker refactor -- ErrRateLimited and IsRetryableError rename

**Files:**
- Modify: `broker/errors.go`
- Modify: `broker/errors_test.go`
- Modify: `broker/schwab/client.go`
- Modify: `broker/alpaca/client.go`
- Modify: `broker/tastytrade/client.go`
- Modify: `broker/tastytrade/errors.go`
- Modify: `broker/tastytrade/errors_test.go`

**Steps:**

- [ ] **Step 1: Write failing test for ErrRateLimited**

In `broker/errors_test.go`, add a test inside the existing `Describe("IsTransient"` block (which will be renamed in the next step):

```go
It("returns true for ErrRateLimited", func() {
    Expect(broker.IsTransient(broker.ErrRateLimited)).To(BeTrue())
})

It("returns true for wrapped ErrRateLimited", func() {
    wrapped := fmt.Errorf("ibkr: %w", broker.ErrRateLimited)
    Expect(broker.IsTransient(wrapped)).To(BeTrue())
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `ginkgo run -race ./broker/`
Expected: FAIL -- `broker.ErrRateLimited` undefined

- [ ] **Step 3: Add ErrRateLimited sentinel and update IsTransient**

In `broker/errors.go`, add `ErrRateLimited` to the var block:

```go
ErrRateLimited = errors.New("broker: rate limited")
```

In `IsTransient`, add an `errors.Is` check before the `HTTPError` check:

```go
if errors.Is(err, ErrRateLimited) {
    return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `ginkgo run -race ./broker/`
Expected: PASS

- [ ] **Step 5: Rename IsTransient to IsRetryableError in broker/errors.go**

Rename the function `IsTransient` to `IsRetryableError`. Update the recursive call inside the `url.Error` branch from `IsTransient(urlErr.Err)` to `IsRetryableError(urlErr.Err)`. Update the doc comment.

- [ ] **Step 6: Rename all IsTransient references in broker/errors_test.go**

Replace all `broker.IsTransient(` with `broker.IsRetryableError(` and rename the `Describe` block from `"IsTransient"` to `"IsRetryableError"`.

- [ ] **Step 7: Rename IsTransient in broker/schwab/client.go**

Replace `broker.IsTransient(retryErr)` with `broker.IsRetryableError(retryErr)`.

- [ ] **Step 8: Rename IsTransient in broker/alpaca/client.go**

Replace `broker.IsTransient(err)` with `broker.IsRetryableError(err)`.

- [ ] **Step 9: Rename IsTransient in broker/tastytrade/client.go and errors.go**

In `broker/tastytrade/client.go`: replace `IsTransient(err)` with `IsRetryableError(err)`.
In `broker/tastytrade/errors.go`: rename `var IsTransient = broker.IsTransient` to `var IsRetryableError = broker.IsRetryableError`.

- [ ] **Step 10: Rename IsTransient in broker/tastytrade/errors_test.go**

Replace all `tastytrade.IsTransient(` with `tastytrade.IsRetryableError(` and rename the `Describe` block.

- [ ] **Step 11: Run full test suite**

Run: `ginkgo run -race ./broker/...`
Expected: PASS -- all broker packages compile and tests pass with the rename

- [ ] **Step 12: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 13: Update Schwab design spec**

In `docs/superpowers/specs/2026-03-22-schwab-broker-design.md`, replace any reference to `broker.IsTransient` with `broker.IsRetryableError`.

- [ ] **Step 14: Commit**

```bash
git add broker/errors.go broker/errors_test.go broker/schwab/client.go broker/alpaca/client.go broker/tastytrade/client.go broker/tastytrade/errors.go broker/tastytrade/errors_test.go docs/superpowers/specs/2026-03-22-schwab-broker-design.md
git commit -m "refactor: add broker.ErrRateLimited and rename IsTransient to IsRetryableError"
```

---

### Task 2: IB package scaffold -- errors, doc, suite, exports

**Files:**
- Create: `broker/ibkr/errors.go`
- Create: `broker/ibkr/errors_test.go`
- Create: `broker/ibkr/doc.go`
- Create: `broker/ibkr/ibkr_suite_test.go`
- Create: `broker/ibkr/exports_test.go` (initial, grows with later tasks)

**Steps:**

- [ ] **Step 1: Create Ginkgo test suite**

Create `broker/ibkr/ibkr_suite_test.go`:

```go
package ibkr_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIBKR(tt *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(tt, "IBKR Suite")
}
```

- [ ] **Step 2: Write test for ErrConidNotFound**

Create `broker/ibkr/errors_test.go`:

```go
package ibkr_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/ibkr"
)

var _ = Describe("Errors", func() {
	It("defines ErrConidNotFound with a descriptive message", func() {
		Expect(ibkr.ErrConidNotFound).To(MatchError("ibkr: contract ID not found for symbol"))
	})
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: FAIL -- `ibkr.ErrConidNotFound` undefined

- [ ] **Step 4: Create errors.go**

Create `broker/ibkr/errors.go`:

```go
package ibkr

import "errors"

var ErrConidNotFound = errors.New("ibkr: contract ID not found for symbol")
```

- [ ] **Step 5: Create doc.go**

Create `broker/ibkr/doc.go`:

```go
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
```

- [ ] **Step 6: Create initial exports_test.go**

Create `broker/ibkr/exports_test.go` with a placeholder (expanded in later tasks):

```go
package ibkr

import "github.com/penny-vault/pvbt/broker"

// Type aliases for test access.
type HTTPError = broker.HTTPError
```

- [ ] **Step 7: Run tests**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: PASS

- [ ] **Step 8: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add broker/ibkr/
git commit -m "feat(ibkr): add package scaffold with errors, doc, and test suite"
```

---

### Task 3: Type definitions and mapping functions

**Files:**
- Create: `broker/ibkr/types.go`
- Create: `broker/ibkr/types_test.go`
- Modify: `broker/ibkr/exports_test.go` (add type aliases)

**Steps:**

- [ ] **Step 1: Write tests for broker-to-IB order mapping**

Create `broker/ibkr/types_test.go` with tests for `toIBOrder`:

```go
package ibkr_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/ibkr"
)

var _ = Describe("Types", func() {
	Describe("toIBOrder", func() {
		It("translates a market buy order", func() {
			order := broker.Order{
				ID:          "test-1",
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			result, translateErr := ibkr.ToIBOrder(order, 265598)
			Expect(translateErr).ToNot(HaveOccurred())
			Expect(result.OrderType).To(Equal("MKT"))
			Expect(result.Side).To(Equal("BUY"))
			Expect(result.Tif).To(Equal("DAY"))
			Expect(result.Quantity).To(BeNumerically("==", 100))
			Expect(result.Conid).To(Equal(int64(265598)))
		})

		It("translates a limit sell order", func() {
			order := broker.Order{
				ID:          "test-2",
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Sell,
				Qty:         50,
				OrderType:   broker.Limit,
				LimitPrice:  150.00,
				TimeInForce: broker.GTC,
			}
			result, translateErr := ibkr.ToIBOrder(order, 272093)
			Expect(translateErr).ToNot(HaveOccurred())
			Expect(result.OrderType).To(Equal("LMT"))
			Expect(result.Side).To(Equal("SELL"))
			Expect(result.Price).To(BeNumerically("==", 150.00))
			Expect(result.Tif).To(Equal("GTC"))
		})

		It("translates a stop-limit order with both prices", func() {
			order := broker.Order{
				ID:          "test-3",
				Side:        broker.Buy,
				Qty:         25,
				OrderType:   broker.StopLimit,
				LimitPrice:  155.00,
				StopPrice:   150.00,
				TimeInForce: broker.Day,
			}
			result, translateErr := ibkr.ToIBOrder(order, 265598)
			Expect(translateErr).ToNot(HaveOccurred())
			Expect(result.OrderType).To(Equal("STP_LIMIT"))
			Expect(result.Price).To(BeNumerically("==", 155.00))
			Expect(result.AuxPrice).To(BeNumerically("==", 150.00))
		})

		It("maps all supported time-in-force values", func() {
			for _, tc := range []struct {
				input    broker.TimeInForce
				expected string
			}{
				{broker.Day, "DAY"},
				{broker.GTC, "GTC"},
				{broker.IOC, "IOC"},
				{broker.OnOpen, "OPG"},
				{broker.OnClose, "MOC"},
			} {
				order := broker.Order{
					Side: broker.Buy, Qty: 1, OrderType: broker.Market,
					TimeInForce: tc.input,
				}
				result, translateErr := ibkr.ToIBOrder(order, 1)
				Expect(translateErr).ToNot(HaveOccurred(), "for TIF %d", tc.input)
				Expect(result.Tif).To(Equal(tc.expected), "for TIF %d", tc.input)
			}
		})

		It("returns error for unsupported GTD time-in-force", func() {
			order := broker.Order{Side: broker.Buy, Qty: 1, OrderType: broker.Market, TimeInForce: broker.GTD}
			_, translateErr := ibkr.ToIBOrder(order, 1)
			Expect(translateErr).To(MatchError(ContainSubstring("unsupported")))
		})

		It("returns error for unsupported FOK time-in-force", func() {
			order := broker.Order{Side: broker.Buy, Qty: 1, OrderType: broker.Market, TimeInForce: broker.FOK}
			_, translateErr := ibkr.ToIBOrder(order, 1)
			Expect(translateErr).To(MatchError(ContainSubstring("unsupported")))
		})
	})

	Describe("toBrokerOrder", func() {
		It("maps Submitted status to OrderOpen", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "123", Status: "Submitted", Side: "BUY",
				OrderType: "LMT", Ticker: "AAPL", Conid: 265598,
				FilledQuantity: 0, RemainingQuantity: 100, TotalQuantity: 100,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderOpen))
		})

		It("maps PreSubmitted status to OrderSubmitted", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "124", Status: "PreSubmitted", Side: "SELL",
				OrderType: "MKT", Ticker: "MSFT", Conid: 272093,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderSubmitted))
		})

		It("maps Filled status to OrderFilled", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "125", Status: "Filled", Side: "BUY",
				OrderType: "LMT", Ticker: "AAPL", Conid: 265598,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderFilled))
		})

		It("maps Cancelled status to OrderCancelled", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "126", Status: "Cancelled", Side: "BUY",
				OrderType: "MKT", Ticker: "GOOG", Conid: 208813720,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderCancelled))
		})

		It("maps PartiallyFilled status to OrderPartiallyFilled", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "126b", Status: "PartiallyFilled", Side: "BUY",
				OrderType: "LMT", Ticker: "AAPL", Conid: 265598,
				FilledQuantity: 50, RemainingQuantity: 50, TotalQuantity: 100,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderPartiallyFilled))
		})

		It("maps Inactive status to OrderCancelled", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "127", Status: "Inactive", Side: "BUY",
				OrderType: "MKT", Ticker: "GOOG", Conid: 208813720,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderCancelled))
		})
	})

	Describe("toBrokerPosition", func() {
		It("maps IB position to broker position", func() {
			ibPos := ibkr.IBPositionEntry{
				ContractId: 265598, Position: 100, AvgCost: 150.50,
				MktPrice: 155.25, Ticker: "AAPL", Currency: "USD",
			}
			result := ibkr.ToBrokerPosition(ibPos)
			Expect(result.Qty).To(BeNumerically("==", 100))
			Expect(result.AvgOpenPrice).To(BeNumerically("==", 150.50))
			Expect(result.MarkPrice).To(BeNumerically("==", 155.25))
		})
	})

	Describe("toBrokerBalance", func() {
		It("maps IB summary to broker balance", func() {
			summary := ibkr.IBAccountSummary{
				CashBalance:      ibkr.SummaryValue{Amount: 50000.00},
				NetLiquidation:   ibkr.SummaryValue{Amount: 150000.00},
				BuyingPower:      ibkr.SummaryValue{Amount: 200000.00},
				MaintMarginReq:   ibkr.SummaryValue{Amount: 75000.00},
			}
			result := ibkr.ToBrokerBalance(summary)
			Expect(result.CashBalance).To(BeNumerically("==", 50000.00))
			Expect(result.NetLiquidatingValue).To(BeNumerically("==", 150000.00))
			Expect(result.EquityBuyingPower).To(BeNumerically("==", 200000.00))
			Expect(result.MaintenanceReq).To(BeNumerically("==", 75000.00))
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: FAIL -- types not defined

- [ ] **Step 3: Create types.go with structs and mapping functions**

Create `broker/ibkr/types.go` with:

- Request structs: `ibOrderRequest` (conid, orderType, side, tif, quantity, price, auxPrice, cOID, parentId, ocaGroup, ocaType, outsideRTH)
- Response structs: `ibOrderResponse` (orderID, status, side, orderType, ticker, conid, filledQuantity, remainingQuantity, totalQuantity), `ibPositionEntry` (contractId, position, avgCost, mktPrice, ticker, currency), `ibAccountSummary` (cashBalance, netLiquidation, buyingPower, maintMarginReq as `summaryValue` structs with Amount field), `ibSecdefResult` (conid, companyName, ticker), `ibMarketSnapshot` (conid, field31 for last price)
- Mapping functions: `toIBOrder(order broker.Order, conid int64) (ibOrderRequest, error)`, `toBrokerOrder(ibOrderResponse) broker.Order`, `toBrokerPosition(ibPositionEntry) broker.Position`, `toBrokerBalance(ibAccountSummary) broker.Balance`
- String-to-enum maps for order type, side, time-in-force, status

The `toIBOrder` function returns `fmt.Errorf("unsupported time-in-force %d: %w", tif, broker.ErrOrderRejected)` for GTD and FOK.

- [ ] **Step 4: Add type aliases to exports_test.go**

Update `broker/ibkr/exports_test.go`:

```go
package ibkr

import "github.com/penny-vault/pvbt/broker"

type HTTPError = broker.HTTPError
type IBOrderRequest = ibOrderRequest
type IBOrderResponse = ibOrderResponse
type IBPositionEntry = ibPositionEntry
type IBAccountSummary = ibAccountSummary
type SummaryValue = summaryValue
type IBSecdefResult = ibSecdefResult

func ToIBOrder(order broker.Order, conid int64) (ibOrderRequest, error) {
	return toIBOrder(order, conid)
}

func ToBrokerOrder(resp ibOrderResponse) broker.Order {
	return toBrokerOrder(resp)
}

func ToBrokerPosition(pos ibPositionEntry) broker.Position {
	return toBrokerPosition(pos)
}

func ToBrokerBalance(summary ibAccountSummary) broker.Balance {
	return toBrokerBalance(summary)
}
```

- [ ] **Step 5: Run tests**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: PASS

- [ ] **Step 6: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add broker/ibkr/types.go broker/ibkr/types_test.go broker/ibkr/exports_test.go
git commit -m "feat(ibkr): add type definitions and mapping functions"
```

---

### Task 4: REST client with rate limiting

**Files:**
- Create: `broker/ibkr/client.go`
- Create: `broker/ibkr/client_test.go`
- Modify: `broker/ibkr/exports_test.go` (add client test helpers)

**Steps:**

- [ ] **Step 1: Write test for account resolution**

Create `broker/ibkr/client_test.go`. Use the same httptest mock server pattern as Schwab. Test that `resolveAccount` calls `GET /iserver/accounts` and returns the first account ID:

```go
package ibkr_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/ibkr"
)

var _ = Describe("Client", func() {
	var (
		ctx       context.Context
		cancelCtx context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancelCtx = context.WithTimeout(context.Background(), 10*time.Second)
	})

	AfterEach(func() {
		cancelCtx()
	})

	Describe("resolveAccount", func() {
		It("returns the first account ID", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /iserver/accounts", func(writer http.ResponseWriter, req *http.Request) {
				json.NewEncoder(writer).Encode(map[string]any{
					"accounts": []string{"U1234567"},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			accountID, resolveErr := client.ResolveAccount(ctx)
			Expect(resolveErr).ToNot(HaveOccurred())
			Expect(accountID).To(Equal("U1234567"))
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: FAIL -- `ibkr.NewAPIClientForTest` undefined

- [ ] **Step 3: Add golang.org/x/time dependency**

Run: `go get golang.org/x/time@latest`

- [ ] **Step 4: Implement apiClient with rate limiter**

Create `broker/ibkr/client.go` with:

- `apiClient` struct: resty client, rate limiter (`*rate.Limiter`), base URL
- `newAPIClient(baseURL string, auth Authenticator) *apiClient`: creates resty client with 15s timeout, 3 retries, `broker.IsRetryableError` retry condition, request middleware calling `auth.Decorate`, rate limiter at 9 req/s burst 1
- `resolveAccount(ctx context.Context) (string, error)`: `GET /iserver/accounts`, returns first account from `accounts` array
- `submitOrder(ctx context.Context, accountID string, orders []ibOrderRequest) ([]ibOrderReply, error)`: `POST /iserver/account/{id}/orders`
- `cancelOrder(ctx context.Context, accountID string, orderID string) error`: `DELETE /iserver/account/{id}/order/{orderID}`
- `replaceOrder(ctx context.Context, accountID string, orderID string, order ibOrderRequest) error`: `POST /iserver/account/{id}/order/{orderID}`
- `getOrders(ctx context.Context) ([]ibOrderResponse, error)`: `GET /iserver/account/orders`
- `getPositions(ctx context.Context, accountID string) ([]ibPositionEntry, error)`: `GET /portfolio/{id}/positions/0`
- `getBalance(ctx context.Context, accountID string) (ibAccountSummary, error)`: `GET /portfolio/{id}/summary`
- `searchSecdef(ctx context.Context, symbol string) ([]ibSecdefResult, error)`: `POST /iserver/secdef/search`
- `getSnapshot(ctx context.Context, conid int64) (float64, error)`: `GET /iserver/marketdata/snapshot?conids={conid}&fields=31`, returns last price
- `confirmReply(ctx context.Context, replyID string) error`: `POST /iserver/reply/{replyId}` with `{confirmed: true}`
- `getTrades(ctx context.Context) ([]ibTradeEntry, error)`: `GET /iserver/account/trades`

Each method acquires a rate limiter token via `client.limiter.Wait(ctx)` before making the HTTP call. Each method wraps errors with context and returns `broker.NewHTTPError` for non-2xx responses. On 429, return `fmt.Errorf("...: %w", broker.ErrRateLimited)`.

- [ ] **Step 5: Add test exports for client**

Add to `broker/ibkr/exports_test.go`:

```go
// noopAuthenticator satisfies Authenticator for testing.
type noopAuthenticator struct{}

func (na *noopAuthenticator) Init(ctx context.Context) error          { return nil }
func (na *noopAuthenticator) Decorate(req *http.Request) error        { return nil }
func (na *noopAuthenticator) Keepalive(ctx context.Context)           {}
func (na *noopAuthenticator) Close() error                            { return nil }

type IBOrderReply = ibOrderReply
type IBTradeEntry = ibTradeEntry

func NewAPIClientForTest(baseURL string) *apiClient {
	return newAPIClient(baseURL, &noopAuthenticator{})
}

func (client *apiClient) ResolveAccount(ctx context.Context) (string, error) {
	return client.resolveAccount(ctx)
}

func (client *apiClient) SubmitOrder(ctx context.Context, accountID string, orders []ibOrderRequest) ([]ibOrderReply, error) {
	return client.submitOrder(ctx, accountID, orders)
}

func (client *apiClient) CancelOrder(ctx context.Context, accountID string, orderID string) error {
	return client.cancelOrder(ctx, accountID, orderID)
}

func (client *apiClient) ReplaceOrder(ctx context.Context, accountID string, orderID string, order ibOrderRequest) error {
	return client.replaceOrder(ctx, accountID, orderID, order)
}

func (client *apiClient) GetOrders(ctx context.Context) ([]ibOrderResponse, error) {
	return client.getOrders(ctx)
}

func (client *apiClient) GetPositions(ctx context.Context, accountID string) ([]ibPositionEntry, error) {
	return client.getPositions(ctx, accountID)
}

func (client *apiClient) GetBalance(ctx context.Context, accountID string) (ibAccountSummary, error) {
	return client.getBalance(ctx, accountID)
}

func (client *apiClient) SearchSecdef(ctx context.Context, symbol string) ([]ibSecdefResult, error) {
	return client.searchSecdef(ctx, symbol)
}

func (client *apiClient) GetSnapshot(ctx context.Context, conid int64) (float64, error) {
	return client.getSnapshot(ctx, conid)
}

func (client *apiClient) ConfirmReply(ctx context.Context, replyID string) error {
	return client.confirmReply(ctx, replyID)
}

func (client *apiClient) GetTrades(ctx context.Context) ([]ibTradeEntry, error) {
	return client.getTrades(ctx)
}
```

- [ ] **Step 6: Write additional client tests**

Add to `broker/ibkr/client_test.go`:

```go
	Describe("submitOrder", func() {
		It("posts order array and parses reply", func() {
			var capturedBody []map[string]any
			mux := http.NewServeMux()
			mux.HandleFunc("POST /iserver/account/U123/orders", func(writer http.ResponseWriter, req *http.Request) {
				json.NewDecoder(req.Body).Decode(&capturedBody)
				json.NewEncoder(writer).Encode([]map[string]any{
					{"order_id": "resp-1", "order_status": "PreSubmitted"},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			orders := []ibkr.IBOrderRequest{{Conid: 265598, OrderType: "MKT", Side: "BUY", Quantity: 100, Tif: "DAY"}}
			replies, submitErr := client.SubmitOrder(ctx, "U123", orders)
			Expect(submitErr).ToNot(HaveOccurred())
			Expect(replies).To(HaveLen(1))
			Expect(capturedBody).To(HaveLen(1))
			Expect(capturedBody[0]["side"]).To(Equal("BUY"))
		})
	})

	Describe("cancelOrder", func() {
		It("sends DELETE for the order", func() {
			var deletedPath string
			mux := http.NewServeMux()
			mux.HandleFunc("DELETE /iserver/account/U123/order/", func(writer http.ResponseWriter, req *http.Request) {
				deletedPath = req.URL.Path
				json.NewEncoder(writer).Encode(map[string]any{"msg": "cancelled"})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			Expect(client.CancelOrder(ctx, "U123", "order-42")).To(Succeed())
			Expect(deletedPath).To(ContainSubstring("order-42"))
		})
	})

	Describe("getPositions", func() {
		It("fetches positions from portfolio endpoint", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /portfolio/U123/positions/0", func(writer http.ResponseWriter, req *http.Request) {
				json.NewEncoder(writer).Encode([]map[string]any{
					{"contractId": 265598, "position": 100.0, "avgCost": 150.50, "mktPrice": 155.0, "ticker": "AAPL", "currency": "USD"},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			positions, posErr := client.GetPositions(ctx, "U123")
			Expect(posErr).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Position).To(BeNumerically("==", 100))
		})
	})

	Describe("getBalance", func() {
		It("fetches summary from portfolio endpoint", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /portfolio/U123/summary", func(writer http.ResponseWriter, req *http.Request) {
				json.NewEncoder(writer).Encode(map[string]any{
					"cashbalance":    map[string]any{"amount": 50000.0},
					"netliquidation": map[string]any{"amount": 150000.0},
					"buyingpower":    map[string]any{"amount": 200000.0},
					"maintmarginreq": map[string]any{"amount": 75000.0},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			summary, balErr := client.GetBalance(ctx, "U123")
			Expect(balErr).ToNot(HaveOccurred())
			Expect(summary.CashBalance.Amount).To(BeNumerically("==", 50000))
		})
	})

	Describe("searchSecdef", func() {
		It("resolves ticker to conid", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
				var body map[string]string
				json.NewDecoder(req.Body).Decode(&body)
				Expect(body["symbol"]).To(Equal("AAPL"))
				json.NewEncoder(writer).Encode([]map[string]any{
					{"conid": 265598, "companyName": "APPLE INC", "ticker": "AAPL"},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			results, searchErr := client.SearchSecdef(ctx, "AAPL")
			Expect(searchErr).ToNot(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results[0].Conid).To(Equal(int64(265598)))
		})
	})

	Describe("getSnapshot", func() {
		It("fetches last price for conid", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /iserver/marketdata/snapshot", func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Query().Get("conids")).To(Equal("265598"))
				Expect(req.URL.Query().Get("fields")).To(Equal("31"))
				json.NewEncoder(writer).Encode([]map[string]any{
					{"conid": 265598, "31": "155.25"},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			lastPrice, snapErr := client.GetSnapshot(ctx, 265598)
			Expect(snapErr).ToNot(HaveOccurred())
			Expect(lastPrice).To(BeNumerically("==", 155.25))
		})
	})

	Describe("error handling", func() {
		It("returns ErrRateLimited on HTTP 429", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /iserver/accounts", func(writer http.ResponseWriter, req *http.Request) {
				writer.WriteHeader(http.StatusTooManyRequests)
				writer.Write([]byte(`{"error": "rate limited"}`))
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			_, resolveErr := client.ResolveAccount(ctx)
			Expect(resolveErr).To(MatchError(broker.ErrRateLimited))
		})

		It("returns HTTPError on HTTP 500", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /iserver/accounts", func(writer http.ResponseWriter, req *http.Request) {
				writer.WriteHeader(http.StatusInternalServerError)
				writer.Write([]byte(`{"error": "internal"}`))
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			_, resolveErr := client.ResolveAccount(ctx)
			var httpErr *ibkr.HTTPError
			Expect(errors.As(resolveErr, &httpErr)).To(BeTrue())
			Expect(httpErr.StatusCode).To(Equal(500))
		})
	})
```

- [ ] **Step 7: Run tests**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: PASS

- [ ] **Step 8: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum broker/ibkr/client.go broker/ibkr/client_test.go broker/ibkr/exports_test.go
git commit -m "feat(ibkr): add REST client with rate limiting"
```

---

### Task 5: Gateway authenticator

**Files:**
- Create: `broker/ibkr/auth.go`
- Create: `broker/ibkr/auth_test.go`
- Modify: `broker/ibkr/exports_test.go` (add auth test helpers)

**Steps:**

- [ ] **Step 1: Write tests for GatewayAuthenticator**

Create `broker/ibkr/auth_test.go`:

```go
package ibkr_test

var _ = Describe("Auth", func() {
	Describe("GatewayAuthenticator", func() {
		It("verifies an active session on Init", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /iserver/auth/status", func(writer http.ResponseWriter, req *http.Request) {
				json.NewEncoder(writer).Encode(map[string]any{
					"authenticated": true,
					"connected":     true,
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			auth := ibkr.NewGatewayAuthenticatorForTest(server.URL)
			Expect(auth.InitAuth(ctx)).To(Succeed())
		})

		It("calls reauthenticate when session is not active", func() {
			var reauthCalled atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("POST /iserver/auth/status", func(writer http.ResponseWriter, req *http.Request) {
				json.NewEncoder(writer).Encode(map[string]any{
					"authenticated": false,
					"connected":     false,
				})
			})
			mux.HandleFunc("POST /iserver/reauthenticate", func(writer http.ResponseWriter, req *http.Request) {
				reauthCalled.Add(1)
				json.NewEncoder(writer).Encode(map[string]any{"message": "triggered"})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			auth := ibkr.NewGatewayAuthenticatorForTest(server.URL)
			initErr := auth.InitAuth(ctx)
			Expect(initErr).To(MatchError(broker.ErrNotAuthenticated))
			Expect(reauthCalled.Load()).To(Equal(int32(1)))
		})

		It("Decorate is a no-op", func() {
			auth := ibkr.NewGatewayAuthenticatorForTest("http://localhost:5000")
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			Expect(auth.DecorateRequest(req)).To(Succeed())
			// No Authorization header added
			Expect(req.Header.Get("Authorization")).To(BeEmpty())
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: FAIL

- [ ] **Step 3: Implement Authenticator interface and GatewayAuthenticator**

Create `broker/ibkr/auth.go` with:

- `Authenticator` interface: `Init(ctx) error`, `Decorate(req *http.Request) error`, `Keepalive(ctx)`, `Close() error`
- `gatewayAuthenticator` struct: baseURL, resty client (for auth-specific calls), cancel func for keepalive goroutine
- `Init`: calls `POST /iserver/auth/status`, checks `authenticated` field. If false, calls `POST /iserver/reauthenticate` and returns `broker.ErrNotAuthenticated` (user must log in via gateway web UI)
- `Decorate`: no-op (returns nil)
- `Keepalive`: starts goroutine that calls `POST /tickle` every 55 seconds via ticker, stops on context cancellation
- `Close`: cancels keepalive context

- [ ] **Step 4: Add test exports for auth**

Add to `broker/ibkr/exports_test.go`:

```go
func NewGatewayAuthenticatorForTest(baseURL string) *gatewayAuthenticator {
	return newGatewayAuthenticator(baseURL)
}

func (auth *gatewayAuthenticator) InitAuth(ctx context.Context) error {
	return auth.Init(ctx)
}

func (auth *gatewayAuthenticator) DecorateRequest(req *http.Request) error {
	return auth.Decorate(req)
}
```

- [ ] **Step 5: Write keepalive test**

Add to `broker/ibkr/auth_test.go`:

```go
		It("sends POST /tickle periodically during Keepalive", func() {
			var tickleCalls atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("POST /iserver/auth/status", func(writer http.ResponseWriter, req *http.Request) {
				json.NewEncoder(writer).Encode(map[string]any{"authenticated": true, "connected": true})
			})
			mux.HandleFunc("POST /tickle", func(writer http.ResponseWriter, req *http.Request) {
				tickleCalls.Add(1)
				writer.WriteHeader(http.StatusOK)
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			auth := ibkr.NewGatewayAuthenticatorForTest(server.URL)
			ibkr.SetGatewayTickleIntervalForTest(auth, 100*time.Millisecond)
			Expect(auth.InitAuth(ctx)).To(Succeed())

			keepaliveCtx, keepaliveCancel := context.WithCancel(ctx)
			go auth.RunKeepalive(keepaliveCtx)
			DeferCleanup(keepaliveCancel)

			Eventually(func() int32 { return tickleCalls.Load() }, 2*time.Second).Should(BeNumerically(">=", 2))
		})
```

Add to `broker/ibkr/exports_test.go`:

```go
func SetGatewayTickleIntervalForTest(auth *gatewayAuthenticator, interval time.Duration) {
	auth.tickleInterval = interval
}

func (auth *gatewayAuthenticator) RunKeepalive(ctx context.Context) {
	auth.Keepalive(ctx)
}
```

- [ ] **Step 6: Run tests**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: PASS

- [ ] **Step 7: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add broker/ibkr/auth.go broker/ibkr/auth_test.go broker/ibkr/exports_test.go
git commit -m "feat(ibkr): add Authenticator interface and GatewayAuthenticator"
```

---

### Task 6: OAuth authenticator

**Files:**
- Modify: `broker/ibkr/auth.go`
- Modify: `broker/ibkr/auth_test.go`
- Modify: `broker/ibkr/exports_test.go`

**Steps:**

- [ ] **Step 1: Write tests for OAuthAuthenticator**

Add a test RSA key fixture at the top of `broker/ibkr/auth_test.go` (inside the file, before the Describe blocks). Generate it once per suite:

```go
var testRSAKeyPEM []byte

var _ = BeforeSuite(func() {
	// Generate a 2048-bit RSA key for testing
	key, genErr := rsa.GenerateKey(rand.Reader, 2048)
	Expect(genErr).ToNot(HaveOccurred())
	pkcs8Bytes, marshalErr := x509.MarshalPKCS8PrivateKey(key)
	Expect(marshalErr).ToNot(HaveOccurred())
	testRSAKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})
})
```

Add OAuth tests:

```go
Describe("OAuthAuthenticator", func() {
	It("loads signing key from file", func() {
		keyFile := filepath.Join(GinkgoT().TempDir(), "test-key.pem")
		Expect(os.WriteFile(keyFile, testRSAKeyPEM, 0600)).To(Succeed())

		auth := ibkr.NewOAuthAuthenticatorForTest("test-consumer", keyFile)
		Expect(auth).ToNot(BeNil())
	})

	It("performs request token exchange on Init", func() {
		keyFile := filepath.Join(GinkgoT().TempDir(), "test-key.pem")
		Expect(os.WriteFile(keyFile, testRSAKeyPEM, 0600)).To(Succeed())

		mux := http.NewServeMux()
		mux.HandleFunc("POST /oauth/request_token", func(writer http.ResponseWriter, req *http.Request) {
			writer.Write([]byte("oauth_token=req-token-123"))
		})
		mux.HandleFunc("POST /oauth/access_token", func(writer http.ResponseWriter, req *http.Request) {
			writer.Write([]byte("oauth_token=access-token-456&oauth_token_secret=secret-789"))
		})
		mux.HandleFunc("POST /oauth/live_session_token", func(writer http.ResponseWriter, req *http.Request) {
			json.NewEncoder(writer).Encode(map[string]string{
				"diffie_hellman_response":      "dh-response",
				"live_session_token_signature": "lst-sig",
			})
		})
		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		auth := ibkr.NewOAuthAuthenticatorForTest("test-consumer", keyFile)
		ibkr.SetOAuthBaseURLForTest(auth, server.URL)
		Expect(auth.InitAuth(ctx)).To(Succeed())
	})

	It("decorates requests with OAuth signature", func() {
		auth := ibkr.NewOAuthAuthenticatorForTestWithToken("test-consumer", "access-token", "session-token")
		req, _ := http.NewRequest("GET", "https://api.ibkr.com/v1/api/iserver/accounts", nil)
		Expect(auth.DecorateRequest(req)).To(Succeed())
		Expect(req.Header.Get("Authorization")).To(HavePrefix("OAuth"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: FAIL

- [ ] **Step 3: Implement OAuthAuthenticator**

Add to `broker/ibkr/auth.go`:

- `oauthAuthenticator` struct: consumerKey, signingKey (*rsa.PrivateKey), accessToken, accessTokenSecret, liveSessionToken, baseURL, cancel func
- `Init`: load PEM key, call request_token, call access_token, call live_session_token with Diffie-Hellman challenge/response, compute live session token from DH response
- `Decorate`: build OAuth Authorization header with HMAC-SHA256 signature using live session token
- `Keepalive`: refresh live session token before expiry (background goroutine)
- `Close`: cancel keepalive

If `IBKR_ACCESS_TOKEN` and `IBKR_ACCESS_TOKEN_SECRET` env vars are set, skip request_token and access_token steps (use pre-existing tokens).

- [ ] **Step 4: Add test exports**

Add to `broker/ibkr/exports_test.go`:

```go
func NewOAuthAuthenticatorForTest(consumerKey, keyFile string) *oauthAuthenticator {
	return newOAuthAuthenticator(consumerKey, keyFile)
}

func NewOAuthAuthenticatorForTestWithToken(consumerKey, accessToken, sessionToken string) *oauthAuthenticator {
	return &oauthAuthenticator{
		consumerKey:      consumerKey,
		accessToken:      accessToken,
		accessTokenSecret: "test-secret",
		liveSessionToken: []byte(sessionToken),
	}
}

func SetOAuthBaseURLForTest(auth *oauthAuthenticator, baseURL string) {
	auth.baseURL = baseURL
}

func (auth *oauthAuthenticator) InitAuth(ctx context.Context) error {
	return auth.Init(ctx)
}

func (auth *oauthAuthenticator) DecorateRequest(req *http.Request) error {
	return auth.Decorate(req)
}
```

- [ ] **Step 5: Write signature verification test**

Add a test that verifies the OAuth signature is computed correctly for a known request. Use a fixed nonce and timestamp so the signature is deterministic and can be compared to a known-good value.

- [ ] **Step 6: Run tests**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: PASS

- [ ] **Step 7: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add broker/ibkr/auth.go broker/ibkr/auth_test.go broker/ibkr/exports_test.go
git commit -m "feat(ibkr): add OAuthAuthenticator with RSA signing and DH key exchange"
```

---

### Task 7: WebSocket streamer

**Files:**
- Create: `broker/ibkr/streamer.go`
- Create: `broker/ibkr/streamer_test.go`
- Modify: `broker/ibkr/exports_test.go`

**Steps:**

- [ ] **Step 1: Write test for streamer connection and fill delivery**

Create `broker/ibkr/streamer_test.go` following the Schwab WebSocket test pattern:

```go
package ibkr_test

var _ = Describe("Streamer", func() {
	var wsUpgrader = websocket.Upgrader{
		CheckOrigin: func(req *http.Request) bool { return true },
	}

	wsServerURL := func(server *httptest.Server) string {
		return "ws" + strings.TrimPrefix(server.URL, "http")
	}

	Describe("connect and subscribe", func() {
		It("sends sor+{} subscription after connecting", func() {
			subscribeReceived := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				_, msgData, readErr := conn.ReadMessage()
				Expect(readErr).ToNot(HaveOccurred())
				subscribeReceived <- string(msgData)
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 1024)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(streamer.CloseStreamer)

			Eventually(subscribeReceived, 5*time.Second).Should(Receive(Equal("sor+{}")))
		})
	})

	Describe("fill delivery", func() {
		It("delivers fills from order update messages", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				// Read subscription
				conn.ReadMessage()

				// Send a fill message
				conn.WriteJSON(map[string]any{
					"topic": "sor",
					"args": map[string]any{
						"orderId":        "order-1",
						"status":         "Filled",
						"filledQuantity": 100.0,
						"avgPrice":       150.25,
					},
				})
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 1024)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(streamer.CloseStreamer)

			var fill broker.Fill
			Eventually(fills, 5*time.Second).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("order-1"))
			Expect(fill.Qty).To(BeNumerically("==", 100))
			Expect(fill.Price).To(BeNumerically("==", 150.25))
		})
	})

	Describe("deduplication", func() {
		It("suppresses duplicate fills", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				conn.ReadMessage()

				fillMsg := map[string]any{
					"topic": "sor",
					"args": map[string]any{
						"orderId": "order-1", "status": "Filled",
						"filledQuantity": 100.0, "avgPrice": 150.25,
					},
				}
				conn.WriteJSON(fillMsg)
				conn.WriteJSON(fillMsg) // duplicate
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 1024)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(streamer.CloseStreamer)

			var fill broker.Fill
			Eventually(fills, 5*time.Second).Should(Receive(&fill))
			Consistently(fills, 500*time.Millisecond).ShouldNot(Receive())
		})
	})

	Describe("heartbeat", func() {
		It("sends tic messages periodically", func() {
			ticCount := make(chan struct{}, 10)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				for {
					_, msgData, readErr := conn.ReadMessage()
					if readErr != nil {
						return
					}
					if string(msgData) == "tic" {
						ticCount <- struct{}{}
					}
				}
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 1024)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			ibkr.SetStreamerHeartbeatForTest(streamer, 100*time.Millisecond)
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(streamer.CloseStreamer)

			Eventually(ticCount, 2*time.Second).Should(Receive())
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: FAIL

- [ ] **Step 3: Implement orderStreamer**

Create `broker/ibkr/streamer.go` with:

- `orderStreamer` struct: wsURL, conn (*websocket.Conn), fills chan, seenFills map, heartbeatInterval, cancel func, mu sync.Mutex
- `newOrderStreamer(fills chan broker.Fill, wsURL string, tradesFn func(ctx context.Context) ([]ibTradeEntry, error)) *orderStreamer`

The `tradesFn` callback is provided by the broker (wrapping `client.getTrades`) so the streamer can poll for missed fills on reconnect without importing the client directly.
- `connect(ctx context.Context) error`: dial WebSocket, send `sor+{}`, start readLoop and heartbeatLoop goroutines
- `readLoop()`: reads JSON messages, checks topic for `"sor"`, extracts fill data, deduplicates via seenFills map, sends `broker.Fill` on channel
- `heartbeatLoop()`: sends `"tic"` every 10 seconds (configurable for testing)
- `close() error`: cancel context, close WebSocket connection
- Reconnection logic: on read error, attempt reconnect with exponential backoff (1s, 2s, 4s), max 3 attempts. On reconnect, re-subscribe with `sor+{}`
- Deduplication: key is `"{orderID}-{filledAt.Unix()}"`, prune entries older than 24 hours on each fill processing

- [ ] **Step 4: Add test exports**

Add to `broker/ibkr/exports_test.go`:

```go
func NewOrderStreamerForTest(fills chan broker.Fill, wsURL string) *orderStreamer {
	// nil tradesFn for unit tests that don't test reconnection fill recovery
	return newOrderStreamer(fills, wsURL, nil)
}

func SetStreamerHeartbeatForTest(streamer *orderStreamer, interval time.Duration) {
	streamer.heartbeatInterval = interval
}

func (streamer *orderStreamer) ConnectStreamer(ctx context.Context) error {
	return streamer.connect(ctx)
}

func (streamer *orderStreamer) CloseStreamer() error {
	return streamer.close()
}
```

- [ ] **Step 5: Write reconnection test**

Add to `broker/ibkr/streamer_test.go`:

```go
	Describe("reconnection", func() {
		It("reconnects and re-subscribes after disconnect", func() {
			connectCount := make(chan struct{}, 10)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				connectCount <- struct{}{}

				// Read subscription message
				conn.ReadMessage()

				// First connection: close immediately to trigger reconnect
				conn.Close()
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 1024)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(streamer.CloseStreamer)

			// Should see at least 2 connections (initial + reconnect)
			Eventually(func() int { return len(connectCount) }, 5*time.Second).Should(BeNumerically(">=", 2))
		})

		It("polls trades on reconnect for missed fills", func() {
			var tradesCalled atomic.Int32
			mockTradesFn := func(ctx context.Context) ([]ibkr.IBTradeEntry, error) {
				tradesCalled.Add(1)
				return []ibkr.IBTradeEntry{
					{OrderID: "missed-1", Price: 150.0, Quantity: 50, ExecutionTime: "20260322-10:30:00"},
				}, nil
			}

			connectCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				connectCount++

				conn.ReadMessage() // subscription

				if connectCount == 1 {
					// First connection: close to trigger reconnect
					conn.Close()
					return
				}
				// Second connection: keep alive
				select {}
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 1024)
			streamer := ibkr.NewOrderStreamerForTestWithTrades(fills, wsServerURL(server), mockTradesFn)
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(streamer.CloseStreamer)

			// Should have polled trades on reconnect
			Eventually(func() int32 { return tradesCalled.Load() }, 5*time.Second).Should(BeNumerically(">=", 1))

			// Missed fill should appear on fills channel
			var fill broker.Fill
			Eventually(fills, 5*time.Second).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("missed-1"))
		})
	})
```

Add to `broker/ibkr/exports_test.go`:

```go
func NewOrderStreamerForTestWithTrades(fills chan broker.Fill, wsURL string, tradesFn func(ctx context.Context) ([]ibTradeEntry, error)) *orderStreamer {
	return newOrderStreamer(fills, wsURL, tradesFn)
}
```

- [ ] **Step 6: Run tests**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: PASS

- [ ] **Step 7: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add broker/ibkr/streamer.go broker/ibkr/streamer_test.go broker/ibkr/exports_test.go
git commit -m "feat(ibkr): add WebSocket streamer for order/fill updates"
```

---

### Task 8: IBBroker struct and Broker interface methods

**Files:**
- Create: `broker/ibkr/broker.go`
- Create: `broker/ibkr/broker_test.go`
- Modify: `broker/ibkr/exports_test.go`

**Steps:**

- [ ] **Step 1: Write test for Submit with conid resolution**

Create `broker/ibkr/broker_test.go`:

```go
package ibkr_test

var _ = Describe("IBBroker", func() {
	authenticatedBroker := func(extraRoutes func(mux *http.ServeMux)) *ibkr.IBBroker {
		mux := http.NewServeMux()

		// extraRoutes registered first so they can override defaults
		if extraRoutes != nil {
			extraRoutes(mux)
		}

		// Default secdef handler -- only registered if extraRoutes didn't already register one.
		// We use a wrapper that checks if the pattern already exists by attempting to register;
		// instead, always register defaults first and let extraRoutes override via a different approach.
		// Actually, Go 1.22+ ServeMux panics on duplicate patterns, so extraRoutes must provide
		// ALL handlers it needs. Do NOT register defaults here -- each test provides its own handlers.
		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		ib := ibkr.New(ibkr.WithGateway(server.URL))
		client := ibkr.NewAPIClientForTest(server.URL)
		ibkr.SetClientForTest(ib, client)
		ibkr.SetAccountIDForTest(ib, "U1234567")

		return ib
	}

	Describe("Submit", func() {
		It("resolves conid and submits a market order", func() {
			var capturedBody []map[string]any
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
					json.NewEncoder(writer).Encode([]map[string]any{
						{"conid": 265598, "companyName": "APPLE INC", "ticker": "AAPL"},
					})
				})
				mux.HandleFunc("POST /iserver/account/U1234567/orders", func(writer http.ResponseWriter, req *http.Request) {
					json.NewDecoder(req.Body).Decode(&capturedBody)
					json.NewEncoder(writer).Encode([]map[string]any{
						{"order_id": "resp-1", "order_status": "PreSubmitted"},
					})
				})
			})

			order := broker.Order{
				ID: "test-1", Asset: asset.Asset{Ticker: "AAPL"},
				Side: broker.Buy, Qty: 100, OrderType: broker.Market, TimeInForce: broker.Day,
			}
			Expect(ib.Submit(ctx, order)).To(Succeed())
			Expect(capturedBody).To(HaveLen(1))
			Expect(capturedBody[0]["orderType"]).To(Equal("MKT"))
			Expect(capturedBody[0]["side"]).To(Equal("BUY"))
			Expect(capturedBody[0]["conid"]).To(BeNumerically("==", 265598))
		})

		It("caches conid across calls", func() {
			var secdefCalls atomic.Int32
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
					secdefCalls.Add(1)
					json.NewEncoder(writer).Encode([]map[string]any{
						{"conid": 265598, "ticker": "AAPL"},
					})
				})
				mux.HandleFunc("POST /iserver/account/U1234567/orders", func(writer http.ResponseWriter, req *http.Request) {
					json.NewEncoder(writer).Encode([]map[string]any{
						{"order_id": "resp-1", "order_status": "PreSubmitted"},
					})
				})
			})

			order := broker.Order{
				Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy,
				Qty: 100, OrderType: broker.Market, TimeInForce: broker.Day,
			}
			Expect(ib.Submit(ctx, order)).To(Succeed())
			Expect(ib.Submit(ctx, order)).To(Succeed())
			Expect(secdefCalls.Load()).To(Equal(int32(1)))
		})

		It("returns ErrConidNotFound for unknown symbol", func() {
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
					json.NewEncoder(writer).Encode([]map[string]any{})
				})
			})

			order := broker.Order{
				Asset: asset.Asset{Ticker: "ZZZZ"}, Side: broker.Buy,
				Qty: 1, OrderType: broker.Market, TimeInForce: broker.Day,
			}
			Expect(ib.Submit(ctx, order)).To(MatchError(ibkr.ErrConidNotFound))
		})

		It("returns error for unsupported GTD time-in-force", func() {
			ib := authenticatedBroker(nil)
			order := broker.Order{
				Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy,
				Qty: 1, OrderType: broker.Market, TimeInForce: broker.GTD,
			}
			Expect(ib.Submit(ctx, order)).To(MatchError(ContainSubstring("unsupported")))
		})
	})

	Describe("Cancel", func() {
		It("cancels an order by ID", func() {
			var deletedOrderID string
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("DELETE /iserver/account/U1234567/order/", func(writer http.ResponseWriter, req *http.Request) {
					deletedOrderID = req.URL.Path[len("/iserver/account/U1234567/order/"):]
					json.NewEncoder(writer).Encode(map[string]any{"order_id": deletedOrderID, "msg": "cancelled"})
				})
			})
			Expect(ib.Cancel(ctx, "order-42")).To(Succeed())
			Expect(deletedOrderID).To(Equal("order-42"))
		})
	})

	Describe("Orders", func() {
		It("returns mapped orders", func() {
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /iserver/account/orders", func(writer http.ResponseWriter, req *http.Request) {
					json.NewEncoder(writer).Encode(map[string]any{
						"orders": []map[string]any{
							{"orderId": "111", "status": "Submitted", "side": "BUY", "orderType": "LMT", "ticker": "AAPL", "conid": 265598},
						},
					})
				})
			})

			orders, ordersErr := ib.Orders(ctx)
			Expect(ordersErr).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].Status).To(Equal(broker.OrderOpen))
		})
	})

	Describe("Positions", func() {
		It("returns mapped positions", func() {
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /portfolio/U1234567/positions/0", func(writer http.ResponseWriter, req *http.Request) {
					json.NewEncoder(writer).Encode([]map[string]any{
						{"contractId": 265598, "position": 100.0, "avgCost": 150.50, "mktPrice": 155.25, "ticker": "AAPL", "currency": "USD"},
					})
				})
			})

			positions, posErr := ib.Positions(ctx)
			Expect(posErr).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Qty).To(BeNumerically("==", 100))
		})
	})

	Describe("Balance", func() {
		It("returns mapped balance", func() {
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /portfolio/U1234567/summary", func(writer http.ResponseWriter, req *http.Request) {
					json.NewEncoder(writer).Encode(map[string]any{
						"cashbalance":    map[string]any{"amount": 50000.0},
						"netliquidation": map[string]any{"amount": 150000.0},
						"buyingpower":    map[string]any{"amount": 200000.0},
						"maintmarginreq": map[string]any{"amount": 75000.0},
					})
				})
			})

			balance, balErr := ib.Balance(ctx)
			Expect(balErr).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(BeNumerically("==", 50000.0))
			Expect(balance.NetLiquidatingValue).To(BeNumerically("==", 150000.0))
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: FAIL

- [ ] **Step 3: Implement IBBroker**

Create `broker/ibkr/broker.go` with:

- `IBBroker` struct per spec (client, auth, streamer, fills, accountID, conidCache, seenFills, mu)
- `Option` type and options: `WithOAuth(OAuthConfig)`, `WithGateway(url string)`
- `OAuthConfig` struct: ConsumerKey, KeyFile
- `New(opts ...Option) *IBBroker`: creates fills channel (1024), initializes conidCache and seenFills maps, applies options. Must have exactly one auth option.
- `Fills() <-chan broker.Fill`
- `Connect(ctx)`: init auth, start keepalive, resolve account via client, start streamer
- `Submit(ctx, order)`: resolve conid if needed (with cache), map to IB order, submit via client, handle confirmation reply if needed
- `Cancel(ctx, orderID)`: delegate to client
- `Replace(ctx, orderID, order)`: resolve conid, map to IB order, delegate to client
- `Orders(ctx)`: get orders from client, map each to broker.Order
- `Positions(ctx)`: get positions from client, map each to broker.Position
- `Balance(ctx)`: get summary from client, map to broker.Balance
- `Close()`: stop streamer, close auth, close fills channel

Dollar-amount order handling in Submit: if `order.Qty == 0 && order.Amount > 0`, fetch quote via `client.getSnapshot`, compute qty = `math.Floor(order.Amount / lastPrice)`.

- [ ] **Step 4: Add broker test exports**

Add to `broker/ibkr/exports_test.go`:

```go
func SetClientForTest(ib *IBBroker, client *apiClient) {
	ib.client = client
}

func SetAccountIDForTest(ib *IBBroker, accountID string) {
	ib.accountID = accountID
}
```

- [ ] **Step 5: Write dollar-amount order test**

Add a test that verifies Submit fetches a snapshot quote and converts amount to quantity.

- [ ] **Step 6: Write confirmation reply test (gateway path)**

Add a test that verifies when the orders endpoint returns a reply ID, the broker auto-confirms via `POST /iserver/reply/{replyId}`.

- [ ] **Step 7: Run tests**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: PASS

- [ ] **Step 8: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add broker/ibkr/broker.go broker/ibkr/broker_test.go broker/ibkr/exports_test.go
git commit -m "feat(ibkr): add IBBroker implementing broker.Broker interface"
```

---

### Task 9: GroupSubmitter -- bracket and OCA orders

**Files:**
- Modify: `broker/ibkr/broker.go`
- Modify: `broker/ibkr/broker_test.go`

**Steps:**

- [ ] **Step 1: Write tests for SubmitGroup bracket orders**

Add to `broker/ibkr/broker_test.go`:

```go
Describe("SubmitGroup", func() {
	Describe("bracket orders", func() {
		It("submits entry with parentId on children", func() {
			var capturedBody []map[string]any
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/account/U1234567/orders", func(writer http.ResponseWriter, req *http.Request) {
					json.NewDecoder(req.Body).Decode(&capturedBody)
					json.NewEncoder(writer).Encode([]map[string]any{
						{"order_id": "resp-1", "order_status": "PreSubmitted"},
					})
				})
			})

			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 100, OrderType: broker.Limit, LimitPrice: 150, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 100, OrderType: broker.Stop, StopPrice: 145, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 100, OrderType: broker.Limit, LimitPrice: 160, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
			}
			Expect(ib.SubmitGroup(ctx, orders, broker.GroupBracket)).To(Succeed())
			Expect(capturedBody).To(HaveLen(3))

			// Entry has cOID, children have parentId
			entryCOID := capturedBody[0]["cOID"]
			Expect(entryCOID).ToNot(BeEmpty())
			Expect(capturedBody[1]["parentId"]).To(Equal(entryCOID))
			Expect(capturedBody[2]["parentId"]).To(Equal(entryCOID))
		})

		It("returns ErrNoEntryOrder when no entry role", func() {
			ib := authenticatedBroker(nil)
			orders := []broker.Order{
				{Side: broker.Sell, Qty: 100, OrderType: broker.Stop, StopPrice: 145, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}
			Expect(ib.SubmitGroup(ctx, orders, broker.GroupBracket)).To(MatchError(broker.ErrNoEntryOrder))
		})

		It("returns ErrMultipleEntryOrders when multiple entries", func() {
			ib := authenticatedBroker(nil)
			orders := []broker.Order{
				{Side: broker.Buy, Qty: 100, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Side: broker.Buy, Qty: 50, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
			}
			Expect(ib.SubmitGroup(ctx, orders, broker.GroupBracket)).To(MatchError(broker.ErrMultipleEntryOrders))
		})
	})

	Describe("OCA orders", func() {
		It("submits all orders with matching ocaGroup and ocaType", func() {
			var capturedBody []map[string]any
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/account/U1234567/orders", func(writer http.ResponseWriter, req *http.Request) {
					json.NewDecoder(req.Body).Decode(&capturedBody)
					json.NewEncoder(writer).Encode([]map[string]any{
						{"order_id": "resp-1", "order_status": "PreSubmitted"},
					})
				})
			})

			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 100, OrderType: broker.Limit, LimitPrice: 150, TimeInForce: broker.Day},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 100, OrderType: broker.Limit, LimitPrice: 148, TimeInForce: broker.Day},
			}
			Expect(ib.SubmitGroup(ctx, orders, broker.GroupOCO)).To(Succeed())
			Expect(capturedBody).To(HaveLen(2))
			Expect(capturedBody[0]["ocaGroup"]).To(Equal(capturedBody[1]["ocaGroup"]))
			Expect(capturedBody[0]["ocaType"]).To(BeNumerically("==", 1))
		})
	})

	It("returns ErrEmptyOrderGroup for empty slice", func() {
		ib := authenticatedBroker(nil)
		Expect(ib.SubmitGroup(ctx, nil, broker.GroupBracket)).To(MatchError(broker.ErrEmptyOrderGroup))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: FAIL -- `SubmitGroup` not defined

- [ ] **Step 3: Implement SubmitGroup**

Add `SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error` to `IBBroker` in `broker/ibkr/broker.go`:

- Validate: empty orders -> `broker.ErrEmptyOrderGroup`
- For `GroupBracket`: find entry (RoleEntry), validate exactly one, assign UUID as `cOID`, set `parentId` on children, resolve conids, map all to IB orders, submit as array
- For `GroupOCO`: generate shared `ocaGroup` name (UUID), set `ocaType: 1` on each order, resolve conids, map all, submit as array

- [ ] **Step 4: Run tests**

Run: `ginkgo run -race ./broker/ibkr/`
Expected: PASS

- [ ] **Step 5: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add broker/ibkr/broker.go broker/ibkr/broker_test.go
git commit -m "feat(ibkr): add GroupSubmitter for bracket and OCA orders"
```

---

### Task 10: Wire IBBroker into engine and add changelog

**Files:**
- Modify: `engine/option.go` (or wherever broker registration happens -- verify existing pattern)
- Modify: `CHANGELOG.md`

**Steps:**

- [ ] **Step 1: Verify engine integration**

Check that `engine.WithBroker(broker)` already accepts any `broker.Broker` implementation. If so, no engine changes are needed -- `ibkr.New()` already satisfies the interface. If the engine has a broker registry or factory, add the IB broker.

- [ ] **Step 2: Run the full test suite**

Run: `ginkgo run -race ./...`
Expected: PASS -- all packages compile and tests pass including the new ibkr package

- [ ] **Step 3: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 4: Add changelog entry**

Add to the `[Unreleased]` section of `CHANGELOG.md` under `### Added`:

```markdown
- Users can now trade through Interactive Brokers using either OAuth or the Client Portal Gateway for authentication
```

- [ ] **Step 5: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add changelog entry for Interactive Brokers broker"
```
