# tastytrade Broker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `Broker` interface for tastytrade as the first live broker integration (equities only).

**Architecture:** Single package `broker/tastytrade/` with five files split by concern: broker orchestration, REST client (Resty), WebSocket fill streamer, API types/translation, and error handling. The engine gains `Connect`/`Close` lifecycle calls so live brokers initialize and tear down properly.

**Tech Stack:** Go, go-resty/resty/v2, gorilla/websocket, Ginkgo/Gomega

**Spec:** `docs/superpowers/specs/2026-03-20-tastytrade-broker-design.md`

---

## File Map

| File | Responsibility |
|------|---------------|
| **Create:** `broker/tastytrade/errors.go` | Sentinel errors, `isTransient()` classifier |
| **Create:** `broker/tastytrade/errors_test.go` | Tests for error classification |
| **Create:** `broker/tastytrade/types.go` | API request/response structs, translation functions |
| **Create:** `broker/tastytrade/types_test.go` | Tests for type translation |
| **Create:** `broker/tastytrade/client.go` | REST API client (auth, orders, positions, balance) |
| **Create:** `broker/tastytrade/client_test.go` | Tests with httptest.Server |
| **Create:** `broker/tastytrade/streamer.go` | WebSocket fill streaming + polling fallback |
| **Create:** `broker/tastytrade/streamer_test.go` | Tests with local WebSocket server |
| **Create:** `broker/tastytrade/broker.go` | `Broker` interface impl, `New()`, options |
| **Create:** `broker/tastytrade/broker_test.go` | Broker lifecycle, channel wiring, option tests |
| **Create:** `broker/tastytrade/doc.go` | Package documentation |
| **Create:** `broker/tastytrade/tastytrade_suite_test.go` | Ginkgo suite bootstrap |
| **Create:** `broker/tastytrade/integration_test.go` | Integration tests with `Label("integration")` |
| **Modify:** `engine/engine.go` | Add `broker.Close()` to existing `Close()` method |
| **Modify:** `engine/backtest.go` | Call `broker.Connect()` during init |
| **Modify:** `engine/live.go` | Call `broker.Connect()` during init |
| **Modify:** `engine/simulated_broker_test.go` | Add engine lifecycle tests |
| **Modify:** `CHANGELOG.md` | Add entry for tastytrade broker |

---

### Task 1: Add go-resty and gorilla/websocket dependencies

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add dependencies**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go get github.com/go-resty/resty/v2 github.com/gorilla/websocket`

- [ ] **Step 2: Tidy modules**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go mod tidy`

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "Add go-resty and gorilla/websocket dependencies"
```

---

### Task 2: Ginkgo suite bootstrap and sentinel errors

**Files:**
- Create: `broker/tastytrade/tastytrade_suite_test.go`
- Create: `broker/tastytrade/errors.go`
- Create: `broker/tastytrade/errors_test.go`

- [ ] **Step 1: Create the Ginkgo suite bootstrap**

Create `broker/tastytrade/tastytrade_suite_test.go`:

```go
package tastytrade_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTastytrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tastytrade Suite")
}
```

- [ ] **Step 2: Write the failing tests for error classification**

Create `broker/tastytrade/errors_test.go`:

```go
package tastytrade_test

import (
	"errors"
	"fmt"
	"net"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/tastytrade"
)

var _ = Describe("Errors", func() {
	Describe("Sentinel errors", func() {
		It("defines all expected sentinel errors", func() {
			Expect(tastytrade.ErrNotAuthenticated).To(MatchError("tastytrade: not authenticated"))
			Expect(tastytrade.ErrMissingCredentials).To(MatchError("tastytrade: TASTYTRADE_USERNAME and TASTYTRADE_PASSWORD must be set"))
			Expect(tastytrade.ErrAccountNotFound).To(MatchError("tastytrade: no accounts found"))
			Expect(tastytrade.ErrOrderRejected).To(MatchError("tastytrade: order rejected"))
			Expect(tastytrade.ErrStreamDisconnected).To(MatchError("tastytrade: WebSocket disconnected"))
		})
	})

	Describe("IsTransient", Label("translation"), func() {
		It("returns true for network timeout errors", func() {
			netErr := &net.OpError{Op: "read", Err: &net.DNSError{IsTimeout: true}}
			Expect(tastytrade.IsTransient(netErr)).To(BeTrue())
		})

		It("returns true for DNS errors", func() {
			dnsErr := &net.DNSError{Name: "api.tastyworks.com"}
			Expect(tastytrade.IsTransient(dnsErr)).To(BeTrue())
		})

		It("returns true for connection refused errors", func() {
			connErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
			Expect(tastytrade.IsTransient(connErr)).To(BeTrue())
		})

		It("returns true for URL errors wrapping net errors", func() {
			urlErr := &url.Error{Op: "Get", URL: "https://api.tastyworks.com", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}
			Expect(tastytrade.IsTransient(urlErr)).To(BeTrue())
		})

		It("returns true for HTTP 500 errors", func() {
			httpErr := tastytrade.NewHTTPError(500, "internal server error")
			Expect(tastytrade.IsTransient(httpErr)).To(BeTrue())
		})

		It("returns true for HTTP 502 errors", func() {
			httpErr := tastytrade.NewHTTPError(502, "bad gateway")
			Expect(tastytrade.IsTransient(httpErr)).To(BeTrue())
		})

		It("returns true for HTTP 503 errors", func() {
			httpErr := tastytrade.NewHTTPError(503, "service unavailable")
			Expect(tastytrade.IsTransient(httpErr)).To(BeTrue())
		})

		It("returns false for HTTP 400 errors", func() {
			httpErr := tastytrade.NewHTTPError(400, "bad request")
			Expect(tastytrade.IsTransient(httpErr)).To(BeFalse())
		})

		It("returns false for HTTP 401 errors", func() {
			httpErr := tastytrade.NewHTTPError(401, "unauthorized")
			Expect(tastytrade.IsTransient(httpErr)).To(BeFalse())
		})

		It("returns false for HTTP 422 errors", func() {
			httpErr := tastytrade.NewHTTPError(422, "unprocessable entity")
			Expect(tastytrade.IsTransient(httpErr)).To(BeFalse())
		})

		It("returns false for order rejected errors", func() {
			Expect(tastytrade.IsTransient(tastytrade.ErrOrderRejected)).To(BeFalse())
		})

		It("returns false for auth errors", func() {
			Expect(tastytrade.IsTransient(tastytrade.ErrNotAuthenticated)).To(BeFalse())
		})

		It("returns false for generic errors", func() {
			Expect(tastytrade.IsTransient(errors.New("something went wrong"))).To(BeFalse())
		})

		It("returns true for wrapped transient errors", func() {
			netErr := &net.OpError{Op: "read", Err: &net.DNSError{IsTimeout: true}}
			wrapped := fmt.Errorf("request failed: %w", netErr)
			Expect(tastytrade.IsTransient(wrapped)).To(BeTrue())
		})
	})
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v`
Expected: compilation error (package does not exist)

- [ ] **Step 4: Implement errors.go**

Create `broker/tastytrade/errors.go`:

```go
package tastytrade

import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

var (
	ErrNotAuthenticated   = errors.New("tastytrade: not authenticated")
	ErrMissingCredentials = errors.New("tastytrade: TASTYTRADE_USERNAME and TASTYTRADE_PASSWORD must be set")
	ErrAccountNotFound    = errors.New("tastytrade: no accounts found")
	ErrOrderRejected      = errors.New("tastytrade: order rejected")
	ErrStreamDisconnected = errors.New("tastytrade: WebSocket disconnected")
)

// HTTPError represents an HTTP response with a non-2xx status code.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("tastytrade: HTTP %d: %s", e.StatusCode, e.Message)
}

// NewHTTPError creates an HTTPError with the given status code and message.
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{StatusCode: statusCode, Message: message}
}

// IsTransient returns true if the error is a transient failure that should
// be retried (network errors, HTTP 5xx). Returns false for permanent
// failures (HTTP 4xx, order rejections, auth errors).
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	// Check for HTTPError.
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500
	}

	// Check for net.OpError (connection refused, timeouts).
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	// Check for DNS errors.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Check for url.Error wrapping a net error.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return IsTransient(urlErr.Err)
	}

	return false
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add broker/tastytrade/tastytrade_suite_test.go broker/tastytrade/errors.go broker/tastytrade/errors_test.go
git commit -m "Add tastytrade error types and transient classification"
```

---

### Task 3: API types and translation functions

**Files:**
- Create: `broker/tastytrade/types.go`
- Create: `broker/tastytrade/types_test.go`

- [ ] **Step 1: Write failing tests for translation functions**

Create `broker/tastytrade/types_test.go` with tests covering:

- `toTastytradeOrder` translates a Market/Buy order correctly (Side, OrderType, TimeInForce, Legs)
- `toTastytradeOrder` translates a Limit/Sell order with LimitPrice
- `toTastytradeOrder` translates a Stop order with StopPrice
- `toTastytradeOrder` translates a StopLimit order with both prices
- `toTastytradeOrder` maps GTC, GTD, IOC, FOK time-in-force values
- `toTastytradeOrder` maps OnOpen and OnClose to Day
- `toBrokerOrder` translates an orderResponse back to broker.Order
- `toBrokerOrder` maps each tastytrade status (Received, Routed, In Flight, Live, Filled, Cancelled, Expired, Rejected)
- `toBrokerPosition` translates a positionResponse
- `toBrokerBalance` translates a balanceResponse
- `toBrokerFill` translates a fillEvent

All tests use `Label("translation")`.

The test file uses `package tastytrade` (not `tastytrade_test`) since the translation functions are unexported. This is intentional and supported by Ginkgo -- internal test files (`package tastytrade`) and external test files (`package tastytrade_test`) can coexist in the same directory. The suite bootstrap in `tastytrade_suite_test.go` uses `package tastytrade_test`, and Ginkgo's test runner discovers and runs specs from both packages.

```go
package tastytrade

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("Types", Label("translation"), func() {
	Describe("toTastytradeOrder", func() {
		It("translates a market buy order", func() {
			order := broker.Order{
				ID:          "order-1",
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result := toTastytradeOrder(order)

			Expect(result.OrderType).To(Equal("Market"))
			Expect(result.TimeInForce).To(Equal("Day"))
			Expect(result.Price).To(BeZero())
			Expect(result.StopTrigger).To(BeZero())
			Expect(result.Legs).To(HaveLen(1))
			Expect(result.Legs[0].Symbol).To(Equal("AAPL"))
			Expect(result.Legs[0].Action).To(Equal("Buy to Open"))
			Expect(result.Legs[0].Quantity).To(Equal(100.0))
			Expect(result.Legs[0].InstrumentType).To(Equal("Equity"))
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

			result := toTastytradeOrder(order)

			Expect(result.OrderType).To(Equal("Limit"))
			Expect(result.Price).To(Equal(350.0))
			Expect(result.TimeInForce).To(Equal("GTC"))
			Expect(result.Legs[0].Action).To(Equal("Sell to Close"))
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

			result := toTastytradeOrder(order)

			Expect(result.OrderType).To(Equal("Stop"))
			Expect(result.StopTrigger).To(Equal(200.0))
			Expect(result.Price).To(BeZero())
		})

		It("translates a stop limit order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "GOOG"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.StopLimit,
				StopPrice:   150.0,
				LimitPrice:  155.0,
				TimeInForce: broker.Day,
			}

			result := toTastytradeOrder(order)

			Expect(result.OrderType).To(Equal("Stop Limit"))
			Expect(result.StopTrigger).To(Equal(150.0))
			Expect(result.Price).To(Equal(155.0))
		})

		It("maps all time-in-force values", func() {
			for _, tc := range []struct {
				tif    broker.TimeInForce
				expect string
			}{
				{broker.Day, "Day"},
				{broker.GTC, "GTC"},
				{broker.GTD, "GTD"},
				{broker.IOC, "IOC"},
				{broker.FOK, "FOK"},
				{broker.OnOpen, "Day"},
				{broker.OnClose, "Day"},
			} {
				order := broker.Order{
					Asset:       asset.Asset{Ticker: "X"},
					Side:        broker.Buy,
					Qty:         1,
					OrderType:   broker.Market,
					TimeInForce: tc.tif,
				}
				result := toTastytradeOrder(order)
				Expect(result.TimeInForce).To(Equal(tc.expect), "for TIF %d", tc.tif)
			}
		})
	})

	Describe("toBrokerOrder", func() {
		It("translates an order response", func() {
			resp := orderResponse{
				ID:          "tt-order-1",
				Status:      "Live",
				OrderType:   "Limit",
				TimeInForce: "GTC",
				Price:       150.0,
				Legs: []orderLegResponse{
					{
						Symbol:         "AAPL",
						InstrumentType: "Equity",
						Action:         "Buy to Open",
						Quantity:       100,
					},
				},
			}

			result := toBrokerOrder(resp)

			Expect(result.ID).To(Equal("tt-order-1"))
			Expect(result.Status).To(Equal(broker.OrderOpen))
			Expect(result.OrderType).To(Equal(broker.Limit))
			Expect(result.LimitPrice).To(Equal(150.0))
			Expect(result.Side).To(Equal(broker.Buy))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
		})

		It("maps all tastytrade statuses", func() {
			for _, tc := range []struct {
				ttStatus string
				expected broker.OrderStatus
			}{
				{"Received", broker.OrderSubmitted},
				{"Routed", broker.OrderSubmitted},
				{"In Flight", broker.OrderSubmitted},
				{"Live", broker.OrderOpen},
				{"Filled", broker.OrderFilled},
				{"Cancelled", broker.OrderCancelled},
				{"Expired", broker.OrderCancelled},
				{"Rejected", broker.OrderCancelled},
			} {
				resp := orderResponse{Status: tc.ttStatus}
				result := toBrokerOrder(resp)
				Expect(result.Status).To(Equal(tc.expected), "for status %q", tc.ttStatus)
			}
		})
	})

	Describe("toBrokerPosition", func() {
		It("translates a position response", func() {
			resp := positionResponse{
				Symbol:        "AAPL",
				Quantity:      100,
				AveragePrice:  150.0,
				MarkPrice:     155.0,
				RealizedDayPL: 200.0,
			}

			result := toBrokerPosition(resp)

			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.AvgOpenPrice).To(Equal(150.0))
			Expect(result.MarkPrice).To(Equal(155.0))
			Expect(result.RealizedDayPL).To(Equal(200.0))
		})
	})

	Describe("toBrokerBalance", func() {
		It("translates a balance response", func() {
			resp := balanceResponse{
				CashBalance:         10000.0,
				NetLiquidatingValue: 25000.0,
				EquityBuyingPower:   50000.0,
				MaintenanceReq:      5000.0,
			}

			result := toBrokerBalance(resp)

			Expect(result.CashBalance).To(Equal(10000.0))
			Expect(result.NetLiquidatingValue).To(Equal(25000.0))
			Expect(result.EquityBuyingPower).To(Equal(50000.0))
			Expect(result.MaintenanceReq).To(Equal(5000.0))
		})
	})

	Describe("toBrokerFill", func() {
		It("translates a fill event", func() {
			fillTime := time.Date(2026, 3, 20, 14, 30, 0, 0, time.UTC)
			event := fillEvent{
				OrderID:  "tt-order-1",
				FillID:   "fill-1",
				Price:    152.50,
				Quantity: 50,
				FilledAt: fillTime,
			}

			result := toBrokerFill(event)

			Expect(result.OrderID).To(Equal("tt-order-1"))
			Expect(result.Price).To(Equal(152.50))
			Expect(result.Qty).To(Equal(50.0))
			Expect(result.FilledAt).To(Equal(fillTime))
		})
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v`
Expected: compilation error (types not defined)

- [ ] **Step 3: Implement types.go**

Create `broker/tastytrade/types.go` with:

- Request structs: `sessionRequest`, `sessionResponse`, `userResponse`, `orderRequest`, `orderLeg`, `accountsResponse`
- Response structs: `orderResponse`, `orderLegResponse`, `positionResponse`, `balanceResponse`, `fillEvent`
- Envelope wrapper: `dataEnvelope[T any]` with `Data struct { Items []T }`
- Translation functions: `toTastytradeOrder`, `toBrokerOrder`, `toBrokerPosition`, `toBrokerBalance`, `toBrokerFill`
- Mapping helpers: `mapSide`, `mapOrderType`, `mapTimeInForce`, `mapTTStatus`, `mapTTOrderType`, `mapTTSide`

Key mappings per spec:
- Buy -> "Buy to Open", Sell -> "Sell to Close"
- Market/Limit/Stop/StopLimit -> "Market"/"Limit"/"Stop"/"Stop Limit"
- Day/GTC/GTD/IOC/FOK -> "Day"/"GTC"/"GTD"/"IOC"/"FOK", OnOpen/OnClose -> "Day"
- Received/Routed/In Flight -> OrderSubmitted, Live -> OrderOpen, Filled -> OrderFilled, Cancelled/Expired/Rejected -> OrderCancelled

```go
package tastytrade

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// --- Request types ---

type sessionRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Data struct {
		SessionToken string       `json:"session-token"`
		User         userResponse `json:"user"`
	} `json:"data"`
}

type userResponse struct {
	ExternalID string `json:"external-id"`
}

type accountsResponse struct {
	Data struct {
		Items []accountItem `json:"items"`
	} `json:"data"`
}

type accountItem struct {
	Account struct {
		AccountNumber string `json:"account-number"`
	} `json:"account"`
}

type orderRequest struct {
	TimeInForce string     `json:"time-in-force"`
	OrderType   string     `json:"order-type"`
	Price       float64    `json:"price,omitempty"`
	StopTrigger float64    `json:"stop-trigger,omitempty"`
	Legs        []orderLeg `json:"legs"`
}

type orderLeg struct {
	InstrumentType string  `json:"instrument-type"`
	Symbol         string  `json:"symbol"`
	Action         string  `json:"action"`
	Quantity       float64 `json:"quantity"`
}

// --- Response types ---

type orderSubmitResponse struct {
	Data struct {
		Order orderResponse `json:"order"`
	} `json:"data"`
}

type ordersListResponse struct {
	Data struct {
		Items []orderResponse `json:"items"`
	} `json:"data"`
}

type orderResponse struct {
	ID          string             `json:"id"`
	Status      string             `json:"status"`
	OrderType   string             `json:"order-type"`
	TimeInForce string             `json:"time-in-force"`
	Price       float64            `json:"price"`
	StopTrigger float64            `json:"stop-trigger"`
	Legs        []orderLegResponse `json:"legs"`
}

type orderLegResponse struct {
	Symbol         string  `json:"symbol"`
	InstrumentType string  `json:"instrument-type"`
	Action         string  `json:"action"`
	Quantity       float64 `json:"quantity"`
}

type positionsListResponse struct {
	Data struct {
		Items []positionResponse `json:"items"`
	} `json:"data"`
}

type positionResponse struct {
	Symbol        string  `json:"symbol"`
	Quantity      float64 `json:"quantity"`
	AveragePrice  float64 `json:"average-open-price"`
	MarkPrice     float64 `json:"mark-price"`
	RealizedDayPL float64 `json:"realized-day-gain-effect"`
}

// balanceResponse contains the inner balance fields only.
// The client method unwraps the tastytrade JSON envelope before populating this.
type balanceResponse struct {
	CashBalance         float64 `json:"cash-balance"`
	NetLiquidatingValue float64 `json:"net-liquidating-value"`
	EquityBuyingPower   float64 `json:"equity-buying-power"`
	MaintenanceReq      float64 `json:"maintenance-requirement"`
}

type quoteResponse struct {
	Data struct {
		Items []quoteItem `json:"items"`
	} `json:"data"`
}

type quoteItem struct {
	Symbol   string  `json:"symbol"`
	LastPrice float64 `json:"last"`
}

type fillEvent struct {
	OrderID  string    `json:"order-id"`
	FillID   string    `json:"fill-id"`
	Price    float64   `json:"price"`
	Quantity float64   `json:"quantity"`
	FilledAt time.Time `json:"filled-at"`
}

// --- Translation functions ---

func toTastytradeOrder(order broker.Order) orderRequest {
	return orderRequest{
		TimeInForce: mapTimeInForce(order.TimeInForce),
		OrderType:   mapOrderType(order.OrderType),
		Price:       order.LimitPrice,
		StopTrigger: order.StopPrice,
		Legs: []orderLeg{
			{
				InstrumentType: "Equity",
				Symbol:         order.Asset.Ticker,
				Action:         mapSide(order.Side),
				Quantity:       order.Qty,
			},
		},
	}
}

func toBrokerOrder(resp orderResponse) broker.Order {
	order := broker.Order{
		ID:          resp.ID,
		Status:      mapTTStatus(resp.Status),
		OrderType:   mapTTOrderType(resp.OrderType),
		LimitPrice:  resp.Price,
		StopPrice:   resp.StopTrigger,
	}

	if len(resp.Legs) > 0 {
		leg := resp.Legs[0]
		order.Asset = asset.Asset{Ticker: leg.Symbol}
		order.Qty = leg.Quantity
		order.Side = mapTTSide(leg.Action)
	}

	return order
}

func toBrokerPosition(resp positionResponse) broker.Position {
	return broker.Position{
		Asset:         asset.Asset{Ticker: resp.Symbol},
		Qty:           resp.Quantity,
		AvgOpenPrice:  resp.AveragePrice,
		MarkPrice:     resp.MarkPrice,
		RealizedDayPL: resp.RealizedDayPL,
	}
}

func toBrokerBalance(resp balanceResponse) broker.Balance {
	return broker.Balance{
		CashBalance:         resp.CashBalance,
		NetLiquidatingValue: resp.NetLiquidatingValue,
		EquityBuyingPower:   resp.EquityBuyingPower,
		MaintenanceReq:      resp.MaintenanceReq,
	}
}

func toBrokerFill(event fillEvent) broker.Fill {
	return broker.Fill{
		OrderID:  event.OrderID,
		Price:    event.Price,
		Qty:      event.Quantity,
		FilledAt: event.FilledAt,
	}
}

// --- Mapping helpers ---

func mapSide(side broker.Side) string {
	switch side {
	case broker.Buy:
		return "Buy to Open"
	case broker.Sell:
		return "Sell to Close"
	default:
		return "Buy to Open"
	}
}

func mapOrderType(orderType broker.OrderType) string {
	switch orderType {
	case broker.Market:
		return "Market"
	case broker.Limit:
		return "Limit"
	case broker.Stop:
		return "Stop"
	case broker.StopLimit:
		return "Stop Limit"
	default:
		return "Market"
	}
}

func mapTimeInForce(tif broker.TimeInForce) string {
	switch tif {
	case broker.Day, broker.OnOpen, broker.OnClose:
		return "Day"
	case broker.GTC:
		return "GTC"
	case broker.GTD:
		return "GTD"
	case broker.IOC:
		return "IOC"
	case broker.FOK:
		return "FOK"
	default:
		return "Day"
	}
}

func mapTTStatus(status string) broker.OrderStatus {
	switch status {
	case "Received", "Routed", "In Flight":
		return broker.OrderSubmitted
	case "Live":
		return broker.OrderOpen
	case "Filled":
		return broker.OrderFilled
	case "Cancelled", "Expired", "Rejected":
		return broker.OrderCancelled
	default:
		return broker.OrderOpen
	}
}

func mapTTOrderType(orderType string) broker.OrderType {
	switch orderType {
	case "Market":
		return broker.Market
	case "Limit":
		return broker.Limit
	case "Stop":
		return broker.Stop
	case "Stop Limit":
		return broker.StopLimit
	default:
		return broker.Market
	}
}

func mapTTSide(action string) broker.Side {
	switch action {
	case "Buy to Open", "Buy to Close":
		return broker.Buy
	case "Sell to Open", "Sell to Close":
		return broker.Sell
	default:
		return broker.Buy
	}
}
```

**Design note on JSON envelopes:** The `balanceResponse` struct is defined with flat fields (no `Data` wrapper) so that translation functions stay simple. The client method `getBalance()` is responsible for unwrapping the tastytrade JSON envelope (`{"data": {...}}`) before populating the flat struct. This is consistent: translation functions work with clean domain structs, and client methods handle API wire format.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add broker/tastytrade/types.go broker/tastytrade/types_test.go
git commit -m "Add tastytrade API types and translation functions"
```

---

### Task 4: REST API client

**Files:**
- Create: `broker/tastytrade/client.go`
- Create: `broker/tastytrade/client_test.go`

- [ ] **Step 1: Write failing tests for the API client**

Create `broker/tastytrade/client_test.go` with tests using `httptest.Server` to mock the tastytrade API. Tests should use `package tastytrade_test` and cover:

**Authentication (`Label("auth")`)**:
- Successful login: POST `/sessions` returns session token and user; `authenticate` stores token and retrieves account ID from `/customers/me/accounts`.
- Missing credentials: returns `ErrMissingCredentials`.
- Invalid credentials: returns wrapped error from API.
- Re-authentication on 401: first request returns 401, client re-auths, retries, succeeds.

**Orders (`Label("orders")`)**:
- `submitOrder`: POST to `/accounts/{id}/orders`, returns order ID from response.
- `cancelOrder`: DELETE to `/accounts/{id}/orders/{orderId}`, returns nil on success.
- `replaceOrder`: PUT to `/accounts/{id}/orders/{orderId}`, returns nil on success.
- `getOrders`: GET from `/accounts/{id}/orders`, returns translated `[]orderResponse`.

**Positions/Balance**:
- `getPositions`: GET from `/accounts/{id}/positions`, returns translated list.
- `getBalance`: GET from `/accounts/{id}/balances`, returns translated balance.

**Quotes**:
- `getQuote`: GET from `/market-data/{symbol}/quotes`, returns last price.

**Retry (`Label("auth")`)**:
- Server returns 500 twice then 200; client retries and succeeds.
- Server returns 400; client does not retry.

Each test starts an `httptest.Server`, creates an `apiClient` pointing at it, and validates the request/response cycle.

Since `apiClient` and its methods are unexported, tests need to be in `package tastytrade` (white-box) or expose via `exports_test.go`. Use `exports_test.go` approach.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v -count=1`
Expected: compilation error

- [ ] **Step 3: Create exports_test.go for white-box testing**

Create `broker/tastytrade/exports_test.go`:

```go
package tastytrade

import "context"

// NewAPIClientForTest creates an apiClient pointing at a custom base URL.
func NewAPIClientForTest(baseURL string) *apiClient {
	return newAPIClient(baseURL)
}

// Authenticate exposes authenticate for testing.
func (c *apiClient) Authenticate(ctx context.Context, username, password string) error {
	return c.authenticate(ctx, username, password)
}

// SubmitOrder exposes submitOrder for testing.
func (c *apiClient) SubmitOrder(ctx context.Context, order orderRequest) (string, error) {
	return c.submitOrder(ctx, order)
}

// CancelOrder exposes cancelOrder for testing.
func (c *apiClient) CancelOrder(ctx context.Context, orderID string) error {
	return c.cancelOrder(ctx, orderID)
}

// ReplaceOrder exposes replaceOrder for testing.
func (c *apiClient) ReplaceOrder(ctx context.Context, orderID string, order orderRequest) error {
	return c.replaceOrder(ctx, orderID, order)
}

// GetOrders exposes getOrders for testing.
func (c *apiClient) GetOrders(ctx context.Context) ([]orderResponse, error) {
	return c.getOrders(ctx)
}

// GetPositions exposes getPositions for testing.
func (c *apiClient) GetPositions(ctx context.Context) ([]positionResponse, error) {
	return c.getPositions(ctx)
}

// GetBalance exposes getBalance for testing.
func (c *apiClient) GetBalance(ctx context.Context) (balanceResponse, error) {
	return c.getBalance(ctx)
}

// GetQuote exposes getQuote for testing.
func (c *apiClient) GetQuote(ctx context.Context, symbol string) (float64, error) {
	return c.getQuote(ctx, symbol)
}

// AccountID returns the client's account ID for test assertions.
func (c *apiClient) AccountID() string {
	return c.accountID
}

// NewFillStreamerForTest creates a fillStreamer for testing.
func NewFillStreamerForTest(client *apiClient, fills chan broker.Fill, wsURL string) *fillStreamer {
	return newFillStreamer(client, fills, wsURL)
}

// ConnectStreamer exposes connect for testing.
func (s *fillStreamer) ConnectStreamer(ctx context.Context) error {
	return s.connect(ctx)
}

// CloseStreamer exposes close for testing.
func (s *fillStreamer) CloseStreamer() error {
	return s.close()
}
```

**Design note on `accountID`:** The spec defines method signatures like `submitOrder(ctx, accountID, order)` but the plan stores `accountID` on the `apiClient` struct during authentication and omits it from method signatures. This is a deliberate simplification -- the account ID is set once and does not change, so threading it through every call adds noise without value.

- [ ] **Step 4: Implement client.go**

Create `broker/tastytrade/client.go` with:

- `newAPIClient(baseURL string) *apiClient` -- creates Resty client with retry config
- Resty setup: `SetRetryCount(3)`, `SetRetryWaitTime(1*time.Second)`, `SetRetryMaxWaitTime(4*time.Second)`, `AddRetryCondition` using `IsTransient`, `OnAfterResponse` for 401 re-auth
- `authenticate(ctx context.Context, username, password string) error` -- POST `/sessions`, store token, GET `/customers/me/accounts` for account ID
- `submitOrder(ctx context.Context, order orderRequest) (string, error)` -- POST `/accounts/{id}/orders`
- `cancelOrder(ctx context.Context, orderID string) error` -- DELETE `/accounts/{id}/orders/{orderID}`
- `replaceOrder(ctx context.Context, orderID string, order orderRequest) error` -- PUT `/accounts/{id}/orders/{orderID}`
- `getOrders(ctx context.Context) ([]orderResponse, error)` -- GET `/accounts/{id}/orders`
- `getPositions(ctx context.Context) ([]positionResponse, error)` -- GET `/accounts/{id}/positions`
- `getBalance(ctx context.Context) (balanceResponse, error)` -- GET `/accounts/{id}/balances`
- `getQuote(ctx context.Context, symbol string) (float64, error)` -- GET `/market-data/{symbol}/quotes`

All methods accept `context.Context` as the first parameter and pass it to Resty via `R().SetContext(ctx)`, enabling context cancellation for in-flight HTTP requests. The Resty client handles auth token, retry, and JSON automatically.

**Design note on `accountID`:** The spec defines `accountID` as a parameter on most methods. The plan stores it on the `apiClient` struct during authentication and uses it internally, avoiding the need to thread it through every call.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v -count=1`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add broker/tastytrade/client.go broker/tastytrade/client_test.go broker/tastytrade/exports_test.go
git commit -m "Add tastytrade REST API client with Resty"
```

---

### Task 5: WebSocket fill streamer

**Files:**
- Create: `broker/tastytrade/streamer.go`
- Create: `broker/tastytrade/streamer_test.go`

- [ ] **Step 1: Write failing tests for the fill streamer**

Create `broker/tastytrade/streamer_test.go` with tests using a local `httptest.Server` upgraded to WebSocket via `gorilla/websocket`. Tests should use `Label("streaming")` and cover:

- **Fill delivery**: server sends a fill event JSON, streamer emits `broker.Fill` on the channel
- **Deduplication**: server sends the same fill ID twice, only one `broker.Fill` appears on channel
- **Partial fill**: server sends two fills for the same order with different quantities, both appear
- **Shutdown**: calling `close()` causes the background goroutine to exit and the fills channel to close
- **Reconnection**: server closes WebSocket; streamer reconnects to a new server; verify fills resume

Add the `fillStreamer` constructor and methods to `exports_test.go` for testing.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v -count=1 --label-filter=streaming`
Expected: compilation error

- [ ] **Step 3: Implement streamer.go**

Create `broker/tastytrade/streamer.go` with:

- `newFillStreamer(client *apiClient, fills chan broker.Fill, wsURL string) *fillStreamer`
- `connect(ctx context.Context) error` -- dial WebSocket, start background goroutine
- `close() error` -- close `done` channel, wait on `wg`, close fills channel
- Background goroutine (`run`): select on WebSocket read, `done` channel, and context cancellation. On message, parse as `fillEvent`, deduplicate via `seenFills`, convert via `toBrokerFill`, send on fills channel.
- `reconnect(ctx context.Context)` -- exponential backoff reconnect (1s, 2s, 4s), on success call `pollMissedFills`
- `pollMissedFills()` -- call `client.getOrders()`, filter filled, deduplicate, send new fills
- `pruneSeenFills()` -- remove entries where fill time is older than 24 hours (called at day boundaries)

The `seenFills` map should be typed as `map[string]time.Time` (fill ID -> fill time), not `map[string]bool` as shown in the spec. The timestamp is needed for time-based pruning (removing entries older than 24 hours). Update the spec's `fillStreamer` struct accordingly.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v -count=1 --label-filter=streaming`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add broker/tastytrade/streamer.go broker/tastytrade/streamer_test.go broker/tastytrade/exports_test.go
git commit -m "Add WebSocket fill streamer with reconnection and deduplication"
```

---

### Task 6: Broker implementation

**Files:**
- Create: `broker/tastytrade/broker.go`
- Create: `broker/tastytrade/broker_test.go`

- [ ] **Step 1: Write failing tests for the broker**

Create `broker/tastytrade/broker_test.go` with tests covering:

**Constructor and options**:
- `New()` creates broker with production base URL by default
- `New(WithSandbox())` creates broker with sandbox base URL
- `Fills()` returns a non-nil channel
- Compile-time interface check: `var _ broker.Broker = (*tastytrade.TastytradeBroker)(nil)`

**Lifecycle (`Label("auth")`)**:
- `Connect` with valid credentials: succeeds, broker is usable
- `Connect` with missing env vars: returns `ErrMissingCredentials`
- `Close` after `Connect`: succeeds, fills channel closes

**Submit (`Label("orders")`)**:
- `Submit` with qty-based order: sends correct request to httptest server
- `Submit` with dollar-amount order (Qty=0, Amount=5000): fetches quote, computes shares, submits
- `Submit` with dollar-amount where price results in 0 shares: returns nil (no order submitted)

**Cancel/Replace (`Label("orders")`)**:
- `Cancel`: delegates to client
- `Replace`: delegates to client

**Queries**:
- `Orders`: returns translated orders from httptest server
- `Positions`: returns translated positions
- `Balance`: returns translated balance

Each test uses an `httptest.Server` that serves the required endpoints.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v -count=1`
Expected: compilation error

- [ ] **Step 3: Implement broker.go**

Create `broker/tastytrade/broker.go` with:

```go
package tastytrade

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/penny-vault/pvbt/broker"
)

const (
	productionBaseURL = "https://api.tastyworks.com"
	sandboxBaseURL    = "https://api.cert.tastyworks.com"
	productionWSURL   = "wss://streamer.tastyworks.com"
	sandboxWSURL      = "wss://streamer.cert.tastyworks.com"
	fillChannelSize   = 1024
)

// TastytradeBroker implements broker.Broker for the tastytrade brokerage.
type TastytradeBroker struct {
	client   *apiClient
	streamer *fillStreamer
	fills    chan broker.Fill
	sandbox  bool
	mu       sync.Mutex
}

// Option configures a TastytradeBroker.
type Option func(*TastytradeBroker)

// WithSandbox configures the broker to use the tastytrade sandbox environment.
func WithSandbox() Option {
	return func(b *TastytradeBroker) {
		b.sandbox = true
	}
}

// New creates a new TastytradeBroker with the given options.
func New(opts ...Option) *TastytradeBroker {
	b := &TastytradeBroker{
		fills: make(chan broker.Fill, fillChannelSize),
	}

	for _, opt := range opts {
		opt(b)
	}

	baseURL := productionBaseURL
	if b.sandbox {
		baseURL = sandboxBaseURL
	}

	b.client = newAPIClient(baseURL)

	return b
}

func (b *TastytradeBroker) Connect(ctx context.Context) error {
	username := os.Getenv("TASTYTRADE_USERNAME")
	password := os.Getenv("TASTYTRADE_PASSWORD")
	if username == "" || password == "" {
		return ErrMissingCredentials
	}

	if err := b.client.authenticate(ctx, username, password); err != nil {
		return fmt.Errorf("tastytrade: connect: %w", err)
	}

	wsURL := productionWSURL
	if b.sandbox {
		wsURL = sandboxWSURL
	}

	b.streamer = newFillStreamer(b.client, b.fills, wsURL)
	if err := b.streamer.connect(ctx); err != nil {
		return fmt.Errorf("tastytrade: connect streamer: %w", err)
	}

	return nil
}

func (b *TastytradeBroker) Close() error {
	if b.streamer != nil {
		return b.streamer.close()
	}
	close(b.fills)
	return nil
}

func (b *TastytradeBroker) Fills() <-chan broker.Fill {
	return b.fills
}

func (b *TastytradeBroker) Submit(ctx context.Context, order broker.Order) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	qty := order.Qty
	if qty == 0 && order.Amount > 0 {
		price, err := b.client.getQuote(ctx, order.Asset.Ticker)
		if err != nil {
			return fmt.Errorf("tastytrade: fetching quote for %s: %w", order.Asset.Ticker, err)
		}

		qty = math.Floor(order.Amount / price)
		if qty == 0 {
			return nil
		}
	}

	order.Qty = qty
	ttOrder := toTastytradeOrder(order)

	orderID, err := b.client.submitOrder(ctx, ttOrder)
	if err != nil {
		return fmt.Errorf("tastytrade: submit order: %w", err)
	}

	order.ID = orderID

	return nil
}

func (b *TastytradeBroker) Cancel(ctx context.Context, orderID string) error {
	return b.client.cancelOrder(ctx, orderID)
}

func (b *TastytradeBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	ttOrder := toTastytradeOrder(order)
	return b.client.replaceOrder(ctx, orderID, ttOrder)
}

func (b *TastytradeBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	responses, err := b.client.getOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("tastytrade: get orders: %w", err)
	}

	orders := make([]broker.Order, len(responses))
	for idx, resp := range responses {
		orders[idx] = toBrokerOrder(resp)
	}

	return orders, nil
}

func (b *TastytradeBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	responses, err := b.client.getPositions(ctx)
	if err != nil {
		return nil, fmt.Errorf("tastytrade: get positions: %w", err)
	}

	positions := make([]broker.Position, len(responses))
	for idx, resp := range responses {
		positions[idx] = toBrokerPosition(resp)
	}

	return positions, nil
}

func (b *TastytradeBroker) Balance(ctx context.Context) (broker.Balance, error) {
	resp, err := b.client.getBalance(ctx)
	if err != nil {
		return broker.Balance{}, fmt.Errorf("tastytrade: get balance: %w", err)
	}

	return toBrokerBalance(resp), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add broker/tastytrade/broker.go broker/tastytrade/broker_test.go
git commit -m "Add TastytradeBroker implementing broker.Broker interface"
```

---

### Task 7: Engine lifecycle integration

**Files:**
- Modify: `engine/engine.go`
- Modify: `engine/backtest.go`
- Modify: `engine/live.go`
- Modify: `engine/engine_suite_test.go` (if needed)

- [ ] **Step 1: Write failing tests for broker lifecycle in the engine**

Add tests to `engine/simulated_broker_test.go` or a new file that verify:

- `Backtest` calls `broker.Connect()` at the start and `broker.Close()` on return
- `RunLive` calls `broker.Connect()` at the start
- If `broker.Connect()` fails, `Backtest` returns the error

Use a mock broker that records whether `Connect`/`Close` were called.

```go
// In engine/simulated_broker_test.go or a new file

type lifecycleBroker struct {
	SimulatedBroker // embed for defaults -- or use a standalone mock
	connected bool
	closed    bool
	connectErr error
}

func (b *lifecycleBroker) Connect(ctx context.Context) error {
	if b.connectErr != nil {
		return b.connectErr
	}
	b.connected = true
	return nil
}

func (b *lifecycleBroker) Close() error {
	b.closed = true
	return nil
}
```

Since the engine tests require significant setup (providers, assets, schedule), this test may be simpler as a unit test that directly calls `createAccount` + verifies the broker's `Connect` is called in `Backtest`. Alternatively, test at the integration level if feasible. Adapt to fit the existing test patterns.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -run Lifecycle`
Expected: FAIL (Connect/Close not called)

- [ ] **Step 3: Add broker.Connect() to Backtest and RunLive**

In `engine/backtest.go`, after `createAccount` returns (after account setup is complete, before schedule enumeration), add:

```go
// Connect the broker (no-op for SimulatedBroker, authenticates for live brokers).
if err := e.broker.Connect(ctx); err != nil {
    return nil, fmt.Errorf("engine: broker connect: %w", err)
}
```

Do NOT add `defer e.broker.Close()` here. The broker is closed by `engine.Close()` (see next step).

In `engine/live.go`, after account setup and before the main loop goroutine starts, add the same `broker.Connect()` call.

- [ ] **Step 4: Add broker.Close() to engine.Close()**

In `engine/engine.go`, the existing `Close()` method (line 601) closes data providers. Add `e.broker.Close()` to this method:

```go
func (e *Engine) Close() error {
    var firstErr error

    // Close the broker.
    if e.broker != nil {
        if err := e.broker.Close(); err != nil && firstErr == nil {
            firstErr = err
        }
    }

    for _, p := range e.providers {
        if err := p.Close(); err != nil && firstErr == nil {
            firstErr = err
        }
    }

    return firstErr
}
```

This is the single place where broker teardown happens, preventing double-close issues.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -count=1`
Expected: all PASS (both new lifecycle tests and existing tests)

- [ ] **Step 6: Run full test suite to check for regressions**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -count=1`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add engine/engine.go engine/backtest.go engine/live.go engine/simulated_broker_test.go
git commit -m "Add broker Connect/Close lifecycle calls to engine"
```

---

### Task 8: Package documentation and changelog

**Files:**
- Create: `broker/tastytrade/doc.go`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Create doc.go**

Create `broker/tastytrade/doc.go`:

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

// Package tastytrade implements broker.Broker for the tastytrade brokerage.
//
// This is the first live broker integration for pvbt. It supports equities
// only. Strategies that work with the SimulatedBroker require no changes
// to run live -- swap the broker via engine.WithBroker(tastytrade.New()).
//
// # Authentication
//
// The broker reads credentials from environment variables:
//
//   - TASTYTRADE_USERNAME: tastytrade account username
//   - TASTYTRADE_PASSWORD: tastytrade account password
//
// Authentication happens during Connect(). The session token is managed
// internally and refreshed automatically on 401 responses.
//
// # Sandbox
//
// Use WithSandbox() to target the tastytrade sandbox environment for
// testing without risking real money:
//
//	broker := tastytrade.New(tastytrade.WithSandbox())
//
// The sandbox API mirrors production but uses paper money.
//
// # Fill Delivery
//
// Fills are delivered via a WebSocket connection to tastytrade's account
// streamer. On disconnect, the broker reconnects with exponential backoff
// and polls for any fills missed during the outage. Duplicate fills are
// suppressed automatically.
//
// # Order Types
//
// All four broker.OrderType values are supported: Market, Limit, Stop,
// and StopLimit. Dollar-amount orders (Qty=0, Amount>0) are converted
// to share quantities by fetching a real-time quote.
package tastytrade
```

- [ ] **Step 2: Update CHANGELOG.md**

Add to the `[Unreleased]` / `### Added` section in `CHANGELOG.md`:

```
- The tastytrade broker integration implements the Broker interface for live equity trading, with WebSocket fill streaming, automatic session management, and a sandbox mode for paper trading
```

- [ ] **Step 3: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./broker/tastytrade/...`
Expected: no errors

- [ ] **Step 4: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add broker/tastytrade/doc.go CHANGELOG.md
git commit -m "Add tastytrade package documentation and changelog entry"
```

---

### Task 9: Integration tests (sandbox)

**Files:**
- Create: `broker/tastytrade/integration_test.go`

- [ ] **Step 1: Write integration tests**

Create `broker/tastytrade/integration_test.go` with `Label("integration")` on every test. These tests require `TASTYTRADE_USERNAME` and `TASTYTRADE_PASSWORD` to be set.

```go
package tastytrade_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/tastytrade"
)

var _ = Describe("Integration", Label("integration"), func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		tb     *tastytrade.TastytradeBroker
	)

	BeforeEach(func() {
		if os.Getenv("TASTYTRADE_USERNAME") == "" {
			Skip("TASTYTRADE_USERNAME not set")
		}

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		tb = tastytrade.New(tastytrade.WithSandbox())
		Expect(tb.Connect(ctx)).To(Succeed())
	})

	AfterEach(func() {
		if tb != nil {
			tb.Close()
		}
		cancel()
	})

	It("connects and retrieves balance", func() {
		balance, err := tb.Balance(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(balance.NetLiquidatingValue).To(BeNumerically(">", 0))
	})

	It("retrieves positions", func() {
		positions, err := tb.Positions(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(positions).NotTo(BeNil())
	})

	It("retrieves orders", func() {
		orders, err := tb.Orders(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(orders).NotTo(BeNil())
	})

	It("submits and cancels a limit order", Label("orders"), func() {
		// Submit a limit buy far below market to avoid fill
		err := tb.Submit(ctx, broker.Order{
			Asset:       asset.Asset{Ticker: "AAPL"},
			Side:        broker.Buy,
			Qty:         1,
			OrderType:   broker.Limit,
			LimitPrice:  1.00, // far below market
			TimeInForce: broker.Day,
		})
		Expect(err).NotTo(HaveOccurred())

		// Retrieve orders and find ours
		orders, err := tb.Orders(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(orders).NotTo(BeEmpty())

		// Cancel it
		err = tb.Cancel(ctx, orders[0].ID)
		Expect(err).NotTo(HaveOccurred())
	})
})
```

Adjust the test based on sandbox capabilities and available symbols.

- [ ] **Step 2: Verify integration tests are excluded from normal runs**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/ -v -count=1`
Expected: integration tests are skipped (no credentials set in CI)

- [ ] **Step 3: Commit**

```bash
git add broker/tastytrade/integration_test.go
git commit -m "Add tastytrade integration tests for sandbox environment"
```

---

## Task Dependency Summary

```
Task 1 (deps)
  └─> Task 2 (errors)
       └─> Task 3 (types)
            └─> Task 4 (client)
                 ├─> Task 5 (streamer)
                 │    └─> Task 6 (broker)
                 │         ├─> Task 7 (engine lifecycle)
                 │         ├─> Task 8 (docs/changelog)
                 │         └─> Task 9 (integration tests)
                 └─────────────┘
```

Tasks 7, 8, and 9 can be done in parallel after Task 6.
