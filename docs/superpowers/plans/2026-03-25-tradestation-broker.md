# TradeStation Broker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `broker.Broker` and `broker.GroupSubmitter` for TradeStation using their v3 REST API with OAuth 2.0 auth, HTTP chunked streaming for fills, and native OCO/bracket group support.

**Architecture:** Three-layer design (auth, client, broker) in `broker/tradestation/`. OAuth 2.0 via Auth0 for authentication with background token refresh. HTTP chunked streaming (`json.Decoder`) for real-time fill delivery. Follows the same patterns established by the Schwab broker implementation.

**Tech Stack:** Go, go-resty/resty/v2, encoding/json, Ginkgo/Gomega, net/http/httptest

---

### Task 1: Package scaffolding and test suite

**Files:**
- Create: `broker/tradestation/doc.go`
- Create: `broker/tradestation/tradestation_suite_test.go`

- [ ] **Step 1: Create doc.go**

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

// Package tradestation implements broker.Broker for the TradeStation brokerage.
//
// This broker integrates with TradeStation's v3 REST API for order management
// and HTTP chunked streaming for real-time fill delivery via order status
// updates.
//
// # Authentication
//
// TradeStation uses OAuth 2.0 with the authorization_code grant type via Auth0.
// The broker reads credentials from environment variables:
//
//   - TRADESTATION_CLIENT_ID: OAuth app key
//   - TRADESTATION_CLIENT_SECRET: OAuth app secret
//   - TRADESTATION_CALLBACK_URL: Registered callback URL (default: https://127.0.0.1:5174)
//   - TRADESTATION_TOKEN_FILE: Path to persist tokens (default: ~/.config/pvbt/tradestation-tokens.json)
//   - TRADESTATION_ACCOUNT_ID: Account ID to trade (optional; uses first account if unset)
//
// On first run, Connect() prints an authorization URL. Open it in a browser,
// log in to TradeStation, and authorize the app. The callback server captures
// the tokens automatically. Subsequent runs reuse the stored refresh token.
//
// # Fill Delivery
//
// Fills are delivered via an HTTP chunked stream to the order status endpoint.
// On disconnect, the broker reconnects with exponential backoff and polls for
// any fills missed during the outage. Duplicate fills are suppressed
// automatically.
//
// # Order Types
//
// All four broker.OrderType values are supported: Market, Limit, Stop, and
// StopLimit. All seven broker.TimeInForce values are supported. Dollar-amount
// orders (Qty=0, Amount>0) are converted to share quantities by fetching a
// real-time quote.
//
// # Order Groups
//
// TradeStationBroker implements broker.GroupSubmitter for native bracket (BRK)
// and OCO order group support using TradeStation's order groups endpoint.
//
// # Usage
//
//	import "github.com/penny-vault/pvbt/broker/tradestation"
//
//	tsBroker := tradestation.New()
//	eng := engine.New(&MyStrategy{},
//	    engine.WithBroker(tsBroker),
//	)
package tradestation
```

- [ ] **Step 2: Create test suite file**

```go
package tradestation_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTradeStation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TradeStation Suite")
}
```

- [ ] **Step 3: Verify test suite runs**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: 0 specs passed, 0 specs failed (empty suite)

- [ ] **Step 4: Commit**

```bash
git add broker/tradestation/doc.go broker/tradestation/tradestation_suite_test.go
git commit -m "feat(tradestation): scaffold package with doc and test suite (#19)"
```

---

### Task 2: Errors

**Files:**
- Create: `broker/tradestation/errors.go`

- [ ] **Step 1: Create errors.go**

```go
package tradestation

import "errors"

var (
	ErrTokenExpired          = errors.New("tradestation: refresh token expired, re-authorization required")
	ErrAuthorizationRequired = errors.New("tradestation: user must authorize via browser")
	ErrAccountNotFound       = errors.New("tradestation: no accounts found")
	ErrStreamDisconnected    = errors.New("tradestation: order stream disconnected")
)
```

- [ ] **Step 2: Verify the package compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./broker/tradestation/...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add broker/tradestation/errors.go
git commit -m "feat(tradestation): add broker-specific error types (#19)"
```

---

### Task 3: Types and translation functions

**Files:**
- Create: `broker/tradestation/types.go`
- Create: `broker/tradestation/types_test.go`

- [ ] **Step 1: Write failing tests for type translation**

Create `broker/tradestation/types_test.go`:

```go
package tradestation

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("Types", Label("translation"), func() {
	Describe("toTSOrder", func() {
		It("translates a market buy order", func() {
			order := broker.Order{
				ID:          "order-1",
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result, translateErr := toTSOrder(order, "ACCT-123")
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.AccountID).To(Equal("ACCT-123"))
			Expect(result.Symbol).To(Equal("AAPL"))
			Expect(result.Quantity).To(Equal("100"))
			Expect(result.OrderType).To(Equal("Market"))
			Expect(result.TradeAction).To(Equal("BUY"))
			Expect(result.TimeInForce.Duration).To(Equal("DAY"))
			Expect(result.Route).To(Equal("Intelligent"))
			Expect(result.LimitPrice).To(Equal(""))
			Expect(result.StopPrice).To(Equal(""))
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

			result, translateErr := toTSOrder(order, "ACCT-123")
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.OrderType).To(Equal("Limit"))
			Expect(result.LimitPrice).To(Equal("350.00"))
			Expect(result.TimeInForce.Duration).To(Equal("GTC"))
			Expect(result.TradeAction).To(Equal("SELL"))
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

			result, translateErr := toTSOrder(order, "ACCT-123")
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.OrderType).To(Equal("StopMarket"))
			Expect(result.StopPrice).To(Equal("200.00"))
			Expect(result.LimitPrice).To(Equal(""))
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

			result, translateErr := toTSOrder(order, "ACCT-123")
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.OrderType).To(Equal("StopLimit"))
			Expect(result.StopPrice).To(Equal("150.00"))
			Expect(result.LimitPrice).To(Equal("155.00"))
		})

		It("maps all supported time-in-force values", func() {
			for _, testCase := range []struct {
				tif    broker.TimeInForce
				expect string
			}{
				{broker.Day, "DAY"},
				{broker.GTC, "GTC"},
				{broker.GTD, "GTD"},
				{broker.IOC, "IOC"},
				{broker.FOK, "FOK"},
				{broker.OnOpen, "OPG"},
				{broker.OnClose, "CLO"},
			} {
				order := broker.Order{
					Asset:       asset.Asset{Ticker: "X"},
					Side:        broker.Buy,
					Qty:         1,
					OrderType:   broker.Market,
					TimeInForce: testCase.tif,
				}
				result, translateErr := toTSOrder(order, "ACCT")
				Expect(translateErr).ToNot(HaveOccurred())
				Expect(result.TimeInForce.Duration).To(Equal(testCase.expect), "for TIF %d", testCase.tif)
			}
		})

		It("includes expiration date for GTD orders", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Limit,
				LimitPrice:  450.0,
				TimeInForce: broker.GTD,
				GTDDate:     time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
			}

			result, translateErr := toTSOrder(order, "ACCT")
			Expect(translateErr).ToNot(HaveOccurred())
			Expect(result.TimeInForce.Duration).To(Equal("GTD"))
			Expect(result.TimeInForce.Expiration).To(Equal("2026-04-15"))
		})
	})

	Describe("toBrokerOrder", func() {
		It("translates a TradeStation order response to broker.Order", func() {
			resp := tsOrderResponse{
				OrderID:     "123456",
				Status:      "OPN",
				OrderType:   "Limit",
				LimitPrice:  "150.00",
				StopPrice:   "",
				Duration:    "GTC",
				Legs: []tsOrderLeg{
					{
						BuySellSideCode: "1",
						Symbol:          "AAPL",
						QuantityOrdered: "100",
					},
				},
			}

			result := toBrokerOrder(resp)

			Expect(result.ID).To(Equal("123456"))
			Expect(result.Status).To(Equal(broker.OrderOpen))
			Expect(result.OrderType).To(Equal(broker.Limit))
			Expect(result.LimitPrice).To(Equal(150.0))
			Expect(result.Side).To(Equal(broker.Buy))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
		})

		It("maps all TradeStation statuses correctly", func() {
			for _, testCase := range []struct {
				tsStatus string
				expected broker.OrderStatus
			}{
				{"ACK", broker.OrderSubmitted},
				{"DON", broker.OrderSubmitted},
				{"OPN", broker.OrderOpen},
				{"FLL", broker.OrderFilled},
				{"FLP", broker.OrderPartiallyFilled},
				{"OUT", broker.OrderCancelled},
				{"CAN", broker.OrderCancelled},
				{"EXP", broker.OrderCancelled},
				{"REJ", broker.OrderCancelled},
				{"UCN", broker.OrderCancelled},
				{"BRO", broker.OrderCancelled},
			} {
				resp := tsOrderResponse{Status: testCase.tsStatus}
				result := toBrokerOrder(resp)
				Expect(result.Status).To(Equal(testCase.expected), "for status %q", testCase.tsStatus)
			}
		})
	})

	Describe("toBrokerPosition", func() {
		It("translates a long position", func() {
			resp := tsPositionEntry{
				Symbol:             "AAPL",
				Quantity:           "100",
				AveragePrice:       "150.00",
				MarketValue:        "15500.00",
				TodaysProfitLoss:   "200.00",
				Last:               "155.00",
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
		It("translates a TradeStation balance response", func() {
			resp := tsBalanceResponse{
				CashBalance: "30000.00",
				Equity:      "75000.00",
				BuyingPower: "60000.00",
				MarketValue: "45000.00",
			}

			result := toBrokerBalance(resp)

			Expect(result.CashBalance).To(Equal(30000.0))
			Expect(result.NetLiquidatingValue).To(Equal(75000.0))
			Expect(result.EquityBuyingPower).To(Equal(60000.0))
		})
	})

	Describe("mapOrderType", func() {
		It("maps all broker order types to TradeStation strings", func() {
			Expect(mapOrderType(broker.Market)).To(Equal("Market"))
			Expect(mapOrderType(broker.Limit)).To(Equal("Limit"))
			Expect(mapOrderType(broker.Stop)).To(Equal("StopMarket"))
			Expect(mapOrderType(broker.StopLimit)).To(Equal("StopLimit"))
		})
	})

	Describe("mapTSOrderType", func() {
		It("maps all TradeStation order type strings to broker types", func() {
			Expect(mapTSOrderType("Market")).To(Equal(broker.Market))
			Expect(mapTSOrderType("Limit")).To(Equal(broker.Limit))
			Expect(mapTSOrderType("StopMarket")).To(Equal(broker.Stop))
			Expect(mapTSOrderType("StopLimit")).To(Equal(broker.StopLimit))
			Expect(mapTSOrderType("Unknown")).To(Equal(broker.Market))
		})
	})

	Describe("mapSide", func() {
		It("maps broker sides to TradeStation trade actions", func() {
			Expect(mapSide(broker.Buy)).To(Equal("BUY"))
			Expect(mapSide(broker.Sell)).To(Equal("SELL"))
		})
	})

	Describe("mapTSSide", func() {
		It("maps TradeStation BuySellSideCode to broker sides", func() {
			Expect(mapTSSide("1")).To(Equal(broker.Buy))
			Expect(mapTSSide("2")).To(Equal(broker.Sell))
			Expect(mapTSSide("3")).To(Equal(broker.Sell))
			Expect(mapTSSide("4")).To(Equal(broker.Buy))
		})
	})

	Describe("mapTSStatus", func() {
		It("returns OrderOpen for OPN", func() {
			Expect(mapTSStatus("OPN")).To(Equal(broker.OrderOpen))
		})

		It("returns OrderFilled for FLL", func() {
			Expect(mapTSStatus("FLL")).To(Equal(broker.OrderFilled))
		})

		It("returns OrderSubmitted for ACK", func() {
			Expect(mapTSStatus("ACK")).To(Equal(broker.OrderSubmitted))
		})

		It("returns OrderCancelled for CAN", func() {
			Expect(mapTSStatus("CAN")).To(Equal(broker.OrderCancelled))
		})

		It("defaults to OrderOpen for unknown statuses", func() {
			Expect(mapTSStatus("SOMETHING_NEW")).To(Equal(broker.OrderOpen))
		})
	})

	Describe("mapTimeInForce", func() {
		It("maps all broker TIF values", func() {
			for _, testCase := range []struct {
				tif    broker.TimeInForce
				expect string
			}{
				{broker.Day, "DAY"},
				{broker.GTC, "GTC"},
				{broker.GTD, "GTD"},
				{broker.IOC, "IOC"},
				{broker.FOK, "FOK"},
				{broker.OnOpen, "OPG"},
				{broker.OnClose, "CLO"},
			} {
				result := mapTimeInForce(testCase.tif)
				Expect(result).To(Equal(testCase.expect), "for TIF %d", testCase.tif)
			}
		})
	})

	Describe("stripDashes", func() {
		It("removes dashes from order IDs", func() {
			Expect(stripDashes("1-2-3-456")).To(Equal("123456"))
			Expect(stripDashes("NODASHES")).To(Equal("NODASHES"))
			Expect(stripDashes("")).To(Equal(""))
		})
	})

	Describe("buildGroupOrder", func() {
		It("builds an OCO group order", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Limit, LimitPrice: 150.0, TimeInForce: broker.Day},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC},
			}

			result, buildErr := buildGroupOrder(orders, broker.GroupOCO, "ACCT-123")
			Expect(buildErr).ToNot(HaveOccurred())
			Expect(result.Type).To(Equal("OCO"))
			Expect(result.Orders).To(HaveLen(2))
		})

		It("builds a bracket group order with entry first", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Stop, StopPrice: 430.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}

			result, buildErr := buildGroupOrder(orders, broker.GroupBracket, "ACCT-123")
			Expect(buildErr).ToNot(HaveOccurred())
			Expect(result.Type).To(Equal("BRK"))
			Expect(result.Orders).To(HaveLen(3))
			// Entry must be first regardless of input order
			Expect(result.Orders[0].TradeAction).To(Equal("BUY"))
		})

		It("returns ErrNoEntryOrder when bracket has no entry", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Stop, StopPrice: 430.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}

			_, buildErr := buildGroupOrder(orders, broker.GroupBracket, "ACCT-123")
			Expect(buildErr).To(MatchError(broker.ErrNoEntryOrder))
		})

		It("returns ErrMultipleEntryOrders when bracket has two entries", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
			}

			_, buildErr := buildGroupOrder(orders, broker.GroupBracket, "ACCT-123")
			Expect(buildErr).To(MatchError(broker.ErrMultipleEntryOrders))
		})

		It("returns ErrEmptyOrderGroup for empty orders", func() {
			_, buildErr := buildGroupOrder([]broker.Order{}, broker.GroupOCO, "ACCT-123")
			Expect(buildErr).To(MatchError(broker.ErrEmptyOrderGroup))
		})
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: FAIL -- functions not defined

- [ ] **Step 3: Write types.go implementation**

Create `broker/tradestation/types.go`:

```go
package tradestation

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// --- TradeStation API request/response types ---

type tsOrderRequest struct {
	AccountID   string          `json:"AccountID"`
	Symbol      string          `json:"Symbol"`
	Quantity    string          `json:"Quantity"`
	OrderType   string          `json:"OrderType"`
	TradeAction string          `json:"TradeAction"`
	TimeInForce tsTimeInForce   `json:"TimeInForce"`
	Route       string          `json:"Route"`
	LimitPrice  string          `json:"LimitPrice,omitempty"`
	StopPrice   string          `json:"StopPrice,omitempty"`
}

type tsTimeInForce struct {
	Duration   string `json:"Duration"`
	Expiration string `json:"Expiration,omitempty"`
}

type tsGroupOrderRequest struct {
	Type   string           `json:"Type"`
	Orders []tsOrderRequest `json:"Orders"`
}

type tsOrderResponse struct {
	OrderID    string       `json:"OrderID"`
	Status     string       `json:"Status"`
	StatusDesc string       `json:"StatusDescription"`
	OrderType  string       `json:"OrderType"`
	LimitPrice string       `json:"LimitPrice"`
	StopPrice  string       `json:"StopPrice"`
	Duration   string       `json:"Duration"`
	Legs       []tsOrderLeg `json:"Legs"`
	FilledQty  string       `json:"FilledQuantity"`
	FilledPrice string      `json:"FilledPrice"`
}

type tsOrderLeg struct {
	BuySellSideCode string      `json:"BuyOrSell"`
	Symbol          string      `json:"Symbol"`
	QuantityOrdered string      `json:"QuantityOrdered"`
	ExecQuantity    string      `json:"ExecQuantity"`
	ExecPrice       string      `json:"ExecPrice"`
	Fills           []tsLegFill `json:"Fills"`
}

type tsLegFill struct {
	ExecID    string `json:"ExecId"`
	Quantity  string `json:"Quantity"`
	Price     string `json:"Price"`
	Timestamp string `json:"Timestamp"`
}

type tsAccountEntry struct {
	AccountID   string `json:"AccountID"`
	AccountType string `json:"AccountType"`
	Status      string `json:"Status"`
}

type tsPositionEntry struct {
	Symbol           string `json:"Symbol"`
	Quantity         string `json:"Quantity"`
	AveragePrice     string `json:"AveragePrice"`
	MarketValue      string `json:"MarketValue"`
	TodaysProfitLoss string `json:"TodaysProfitLoss"`
	Last             string `json:"Last"`
}

type tsBalanceResponse struct {
	CashBalance string `json:"CashBalance"`
	Equity      string `json:"Equity"`
	BuyingPower string `json:"BuyingPower"`
	MarketValue string `json:"MarketValue"`
}

type tsQuoteResponse struct {
	Quotes []tsQuote `json:"Quotes"`
}

type tsQuote struct {
	Symbol string  `json:"Symbol"`
	Last   float64 `json:"Last"`
}

type tsTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// tsStreamOrderEvent represents an order update from the HTTP streaming endpoint.
type tsStreamOrderEvent struct {
	OrderID     string       `json:"OrderID"`
	Status      string       `json:"Status"`
	OrderType   string       `json:"OrderType"`
	FilledQty   string       `json:"FilledQuantity"`
	FilledPrice string       `json:"FilledPrice"`
	Legs        []tsOrderLeg `json:"Legs"`
	// Heartbeat and snapshot signals
	Heartbeat   int    `json:"Heartbeat,omitempty"`
	EndSnapshot bool   `json:"EndSnapshot,omitempty"`
	GoAway      bool   `json:"GoAway,omitempty"`
	Error       string `json:"Error,omitempty"`
}

// --- Translation functions ---

func toTSOrder(order broker.Order, accountID string) (tsOrderRequest, error) {
	tif := mapTimeInForce(order.TimeInForce)

	tsOrder := tsOrderRequest{
		AccountID:   accountID,
		Symbol:      order.Asset.Ticker,
		Quantity:    strconv.FormatFloat(order.Qty, 'f', -1, 64),
		OrderType:   mapOrderType(order.OrderType),
		TradeAction: mapSide(order.Side),
		TimeInForce: tsTimeInForce{Duration: tif},
		Route:       "Intelligent",
	}

	if order.LimitPrice != 0 {
		tsOrder.LimitPrice = fmt.Sprintf("%.2f", order.LimitPrice)
	}

	if order.StopPrice != 0 {
		tsOrder.StopPrice = fmt.Sprintf("%.2f", order.StopPrice)
	}

	if order.TimeInForce == broker.GTD && !order.GTDDate.IsZero() {
		tsOrder.TimeInForce.Expiration = order.GTDDate.Format("2006-01-02")
	}

	return tsOrder, nil
}

func toBrokerOrder(resp tsOrderResponse) broker.Order {
	order := broker.Order{
		ID:        resp.OrderID,
		Status:    mapTSStatus(resp.Status),
		OrderType: mapTSOrderType(resp.OrderType),
	}

	order.LimitPrice = parseFloat(resp.LimitPrice)
	order.StopPrice = parseFloat(resp.StopPrice)

	if len(resp.Legs) > 0 {
		leg := resp.Legs[0]
		order.Asset = asset.Asset{Ticker: leg.Symbol}
		order.Qty = parseFloat(leg.QuantityOrdered)
		order.Side = mapTSSide(leg.BuySellSideCode)
	}

	return order
}

func toBrokerPosition(resp tsPositionEntry) broker.Position {
	return broker.Position{
		Asset:         asset.Asset{Ticker: resp.Symbol},
		Qty:           parseFloat(resp.Quantity),
		AvgOpenPrice:  parseFloat(resp.AveragePrice),
		MarkPrice:     parseFloat(resp.Last),
		RealizedDayPL: parseFloat(resp.TodaysProfitLoss),
	}
}

func toBrokerBalance(resp tsBalanceResponse) broker.Balance {
	return broker.Balance{
		CashBalance:         parseFloat(resp.CashBalance),
		NetLiquidatingValue: parseFloat(resp.Equity),
		EquityBuyingPower:   parseFloat(resp.BuyingPower),
	}
}

// --- Mapping helpers ---

func mapOrderType(orderType broker.OrderType) string {
	switch orderType {
	case broker.Market:
		return "Market"
	case broker.Limit:
		return "Limit"
	case broker.Stop:
		return "StopMarket"
	case broker.StopLimit:
		return "StopLimit"
	default:
		return "Market"
	}
}

func mapTSOrderType(orderType string) broker.OrderType {
	switch orderType {
	case "Market":
		return broker.Market
	case "Limit":
		return broker.Limit
	case "StopMarket":
		return broker.Stop
	case "StopLimit":
		return broker.StopLimit
	default:
		return broker.Market
	}
}

func mapSide(side broker.Side) string {
	switch side {
	case broker.Buy:
		return "BUY"
	case broker.Sell:
		return "SELL"
	default:
		return "BUY"
	}
}

// mapTSSide maps TradeStation BuySellSideCode to broker.Side.
// 1=Buy, 2=Sell, 3=SellShort, 4=BuyToCover.
func mapTSSide(code string) broker.Side {
	switch code {
	case "1", "4":
		return broker.Buy
	case "2", "3":
		return broker.Sell
	default:
		return broker.Buy
	}
}

func mapTimeInForce(tif broker.TimeInForce) string {
	switch tif {
	case broker.Day:
		return "DAY"
	case broker.GTC:
		return "GTC"
	case broker.GTD:
		return "GTD"
	case broker.IOC:
		return "IOC"
	case broker.FOK:
		return "FOK"
	case broker.OnOpen:
		return "OPG"
	case broker.OnClose:
		return "CLO"
	default:
		return "DAY"
	}
}

// mapTSStatus maps TradeStation order status codes to broker.OrderStatus.
// Reference: ACK=acknowledged, DON=condition pending, OPN=open, FLL=filled,
// FLP=partially filled, OUT=cancelled/completed, CAN=cancelled, EXP=expired,
// REJ=rejected, UCN=unable to cancel, BRO=broken.
func mapTSStatus(status string) broker.OrderStatus {
	switch status {
	case "ACK", "DON", "CND", "QUE", "REC":
		return broker.OrderSubmitted
	case "OPN":
		return broker.OrderOpen
	case "FLL":
		return broker.OrderFilled
	case "FLP":
		return broker.OrderPartiallyFilled
	case "OUT", "CAN", "EXP", "REJ", "UCN", "BRO", "TSC":
		return broker.OrderCancelled
	default:
		return broker.OrderOpen
	}
}

func stripDashes(orderID string) string {
	return strings.ReplaceAll(orderID, "-", "")
}

func parseFloat(str string) float64 {
	val, _ := strconv.ParseFloat(str, 64)
	return val
}

// --- Group order builders ---

func buildGroupOrder(orders []broker.Order, groupType broker.GroupType, accountID string) (tsGroupOrderRequest, error) {
	if len(orders) == 0 {
		return tsGroupOrderRequest{}, broker.ErrEmptyOrderGroup
	}

	var groupTypeStr string

	switch groupType {
	case broker.GroupOCO:
		groupTypeStr = "OCO"
	case broker.GroupBracket:
		groupTypeStr = "BRK"
	default:
		return tsGroupOrderRequest{}, fmt.Errorf("tradestation: unsupported group type %d", groupType)
	}

	tsOrders := make([]tsOrderRequest, 0, len(orders))

	if groupType == broker.GroupBracket {
		// Entry order must be first in the array.
		var entryOrder *tsOrderRequest
		var contingentOrders []tsOrderRequest

		for _, order := range orders {
			tsOrd, err := toTSOrder(order, accountID)
			if err != nil {
				return tsGroupOrderRequest{}, err
			}

			if order.GroupRole == broker.RoleEntry {
				if entryOrder != nil {
					return tsGroupOrderRequest{}, broker.ErrMultipleEntryOrders
				}

				entryOrder = &tsOrd
			} else {
				contingentOrders = append(contingentOrders, tsOrd)
			}
		}

		if entryOrder == nil {
			return tsGroupOrderRequest{}, broker.ErrNoEntryOrder
		}

		tsOrders = append(tsOrders, *entryOrder)
		tsOrders = append(tsOrders, contingentOrders...)
	} else {
		for _, order := range orders {
			tsOrd, err := toTSOrder(order, accountID)
			if err != nil {
				return tsGroupOrderRequest{}, err
			}

			tsOrders = append(tsOrders, tsOrd)
		}
	}

	return tsGroupOrderRequest{
		Type:   groupTypeStr,
		Orders: tsOrders,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add broker/tradestation/types.go broker/tradestation/types_test.go
git commit -m "feat(tradestation): add API types and translation functions (#19)"
```

---

### Task 4: Auth layer

**Files:**
- Create: `broker/tradestation/auth.go`
- Create: `broker/tradestation/auth_test.go`

- [ ] **Step 1: Write failing tests for auth**

Create `broker/tradestation/auth_test.go`:

```go
package tradestation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("tokenManager", func() {
	Describe("Token file I/O", func() {
		It("saves and loads tokens from a file", func() {
			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			store := &tokenStore{
				AccessToken:     "access-abc",
				RefreshToken:    "refresh-xyz",
				AccessExpiresAt: time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC),
			}

			saveErr := saveTokens(tokenPath, store)
			Expect(saveErr).ToNot(HaveOccurred())

			loaded, loadErr := loadTokens(tokenPath)
			Expect(loadErr).ToNot(HaveOccurred())
			Expect(loaded.AccessToken).To(Equal("access-abc"))
			Expect(loaded.RefreshToken).To(Equal("refresh-xyz"))
			Expect(loaded.AccessExpiresAt).To(BeTemporally("~", store.AccessExpiresAt, time.Second))
		})

		It("returns an error when the file does not exist", func() {
			_, loadErr := loadTokens("/nonexistent/path/tokens.json")
			Expect(loadErr).To(HaveOccurred())
		})

		It("creates parent directories when saving", func() {
			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "sub", "dir", "tokens.json")

			store := &tokenStore{AccessToken: "test"}
			saveErr := saveTokens(tokenPath, store)
			Expect(saveErr).ToNot(HaveOccurred())

			_, statErr := os.Stat(tokenPath)
			Expect(statErr).ToNot(HaveOccurred())
		})
	})

	Describe("Token expiration", func() {
		It("reports an expired access token", func() {
			manager := newTokenManager("id", "secret", "", "")
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(-10 * time.Minute),
			}

			Expect(manager.accessTokenExpired()).To(BeTrue())
		})

		It("reports a valid access token", func() {
			manager := newTokenManager("id", "secret", "", "")
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(10 * time.Minute),
			}

			Expect(manager.accessTokenExpired()).To(BeFalse())
		})

		It("accounts for the 5-minute buffer", func() {
			manager := newTokenManager("id", "secret", "", "")
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(3 * time.Minute),
			}

			Expect(manager.accessTokenExpired()).To(BeTrue())
		})
	})

	Describe("Token refresh", func() {
		It("exchanges a refresh token for a new access token", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/oauth/token"))
				Expect(req.FormValue("grant_type")).To(Equal("refresh_token"))
				Expect(req.FormValue("refresh_token")).To(Equal("refresh-old"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"access_token":  "access-new",
					"refresh_token": "refresh-new",
					"expires_in":    1200,
					"token_type":    "Bearer",
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			manager := newTokenManager("client-id", "client-secret", "", tokenPath)
			manager.authBaseURL = server.URL
			manager.tokens = &tokenStore{
				RefreshToken: "refresh-old",
			}

			refreshErr := manager.refreshAccessToken()
			Expect(refreshErr).ToNot(HaveOccurred())
			Expect(manager.tokens.AccessToken).To(Equal("access-new"))
			Expect(manager.tokens.RefreshToken).To(Equal("refresh-new"))
			Expect(manager.tokens.AccessExpiresAt).To(BeTemporally(">", time.Now()))
		})
	})

	Describe("Auth code exchange", func() {
		It("exchanges an authorization code for tokens", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/oauth/token"))
				Expect(req.FormValue("grant_type")).To(Equal("authorization_code"))
				Expect(req.FormValue("code")).To(Equal("auth-code-123"))
				Expect(req.FormValue("redirect_uri")).ToNot(BeEmpty())

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"access_token":  "access-from-code",
					"refresh_token": "refresh-from-code",
					"expires_in":    1200,
					"token_type":    "Bearer",
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			manager := newTokenManager("client-id", "client-secret", "https://127.0.0.1:5174", tokenPath)
			manager.authBaseURL = server.URL

			exchangeErr := manager.exchangeAuthCode("auth-code-123")
			Expect(exchangeErr).ToNot(HaveOccurred())
			Expect(manager.tokens.AccessToken).To(Equal("access-from-code"))
			Expect(manager.tokens.RefreshToken).To(Equal("refresh-from-code"))
		})
	})

	Describe("ensureValidToken", func() {
		It("returns nil when access token is still valid", func() {
			manager := newTokenManager("id", "secret", "", "")
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(10 * time.Minute),
			}

			Expect(manager.ensureValidToken()).To(Succeed())
		})

		It("refreshes when access token is expired", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"access_token":  "refreshed",
					"refresh_token": "refresh-new",
					"expires_in":    1200,
				})
			}))
			DeferCleanup(server.Close)

			tempDir := GinkgoT().TempDir()
			tokenPath := filepath.Join(tempDir, "tokens.json")

			manager := newTokenManager("id", "secret", "", tokenPath)
			manager.authBaseURL = server.URL
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(-1 * time.Minute),
				RefreshToken:    "valid-refresh",
			}

			Expect(manager.ensureValidToken()).To(Succeed())
			Expect(manager.tokens.AccessToken).To(Equal("refreshed"))
		})

		It("returns ErrTokenExpired when refresh token is empty", func() {
			manager := newTokenManager("id", "secret", "", "")
			manager.tokens = &tokenStore{
				AccessExpiresAt: time.Now().Add(-1 * time.Minute),
				RefreshToken:    "",
			}

			ensureErr := manager.ensureValidToken()
			Expect(ensureErr).To(MatchError(ErrTokenExpired))
		})
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: FAIL -- functions not defined

- [ ] **Step 3: Write auth.go implementation**

Create `broker/tradestation/auth.go`:

```go
package tradestation

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	defaultCallbackURL  = "https://127.0.0.1:5174"
	defaultTokenFile    = "~/.config/pvbt/tradestation-tokens.json"
	defaultAuthBaseURL  = "https://signin.tradestation.com"
	accessTokenBuffer   = 5 * time.Minute
	refreshInterval     = 15 * time.Minute
	oauthScopes         = "openid profile offline_access MarketData ReadAccount Trade"
)

type tokenStore struct {
	AccessToken     string    `json:"access_token"`
	RefreshToken    string    `json:"refresh_token"`
	AccessExpiresAt time.Time `json:"access_expires_at"`
}

type tokenManager struct {
	clientID     string
	clientSecret string
	callbackURL  string
	tokenFile    string
	authBaseURL  string
	tokens       *tokenStore
	onRefresh    func(token string)
	mu           sync.Mutex
	stopRefresh  chan struct{}
	refreshWg    sync.WaitGroup
}

func newTokenManager(clientID, clientSecret, callbackURL, tokenFile string) *tokenManager {
	if callbackURL == "" {
		callbackURL = defaultCallbackURL
	}

	if tokenFile == "" {
		tokenFile = expandHome(defaultTokenFile)
	}

	return &tokenManager{
		clientID:     clientID,
		clientSecret: clientSecret,
		callbackURL:  callbackURL,
		tokenFile:    tokenFile,
		authBaseURL:  defaultAuthBaseURL,
		tokens:       &tokenStore{},
		stopRefresh:  make(chan struct{}),
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr == nil {
			return filepath.Join(homeDir, path[2:])
		}
	}

	return path
}

func loadTokens(path string) (*tokenStore, error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, fmt.Errorf("load tokens: %w", readErr)
	}

	var store tokenStore
	if unmarshalErr := json.Unmarshal(data, &store); unmarshalErr != nil {
		return nil, fmt.Errorf("parse tokens: %w", unmarshalErr)
	}

	return &store, nil
}

func saveTokens(path string, store *tokenStore) error {
	parentDir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(parentDir, 0700); mkdirErr != nil {
		return fmt.Errorf("create token directory: %w", mkdirErr)
	}

	data, marshalErr := json.MarshalIndent(store, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshal tokens: %w", marshalErr)
	}

	if writeErr := os.WriteFile(path, data, 0600); writeErr != nil {
		return fmt.Errorf("write tokens: %w", writeErr)
	}

	return nil
}

func (manager *tokenManager) accessTokenExpired() bool {
	return time.Now().After(manager.tokens.AccessExpiresAt.Add(-accessTokenBuffer))
}

func (manager *tokenManager) refreshAccessToken() error {
	client := resty.New()

	resp, reqErr := client.R().
		SetFormData(map[string]string{
			"grant_type":    "refresh_token",
			"client_id":     manager.clientID,
			"client_secret": manager.clientSecret,
			"refresh_token": manager.tokens.RefreshToken,
		}).
		Post(manager.authBaseURL + "/oauth/token")
	if reqErr != nil {
		return fmt.Errorf("refresh token request: %w", reqErr)
	}

	if resp.IsError() {
		return fmt.Errorf("refresh token: HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	var tokenResp tsTokenResponse
	if unmarshalErr := json.Unmarshal(resp.Body(), &tokenResp); unmarshalErr != nil {
		return fmt.Errorf("parse token response: %w", unmarshalErr)
	}

	manager.tokens.AccessToken = tokenResp.AccessToken
	manager.tokens.RefreshToken = tokenResp.RefreshToken
	manager.tokens.AccessExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return saveTokens(manager.tokenFile, manager.tokens)
}

func (manager *tokenManager) exchangeAuthCode(code string) error {
	client := resty.New()

	resp, reqErr := client.R().
		SetFormData(map[string]string{
			"grant_type":    "authorization_code",
			"client_id":     manager.clientID,
			"client_secret": manager.clientSecret,
			"code":          code,
			"redirect_uri":  manager.callbackURL,
		}).
		Post(manager.authBaseURL + "/oauth/token")
	if reqErr != nil {
		return fmt.Errorf("exchange auth code request: %w", reqErr)
	}

	if resp.IsError() {
		return fmt.Errorf("exchange auth code: HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	var tokenResp tsTokenResponse
	if unmarshalErr := json.Unmarshal(resp.Body(), &tokenResp); unmarshalErr != nil {
		return fmt.Errorf("parse token response: %w", unmarshalErr)
	}

	manager.tokens.AccessToken = tokenResp.AccessToken
	manager.tokens.RefreshToken = tokenResp.RefreshToken
	manager.tokens.AccessExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return saveTokens(manager.tokenFile, manager.tokens)
}

func (manager *tokenManager) ensureValidToken() error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if !manager.accessTokenExpired() {
		return nil
	}

	if manager.tokens.RefreshToken == "" {
		return ErrTokenExpired
	}

	return manager.refreshAccessToken()
}

func (manager *tokenManager) authorizationURL() string {
	return fmt.Sprintf(
		"%s/authorize?response_type=code&client_id=%s&audience=%s&redirect_uri=%s&scope=%s",
		manager.authBaseURL,
		url.QueryEscape(manager.clientID),
		url.QueryEscape("https://api.tradestation.com"),
		url.QueryEscape(manager.callbackURL),
		url.QueryEscape(oauthScopes),
	)
}

func (manager *tokenManager) startAuthFlow() error {
	parsedURL, parseErr := url.Parse(manager.callbackURL)
	if parseErr != nil {
		fallback, fallbackErr := url.Parse(defaultCallbackURL)
		if fallbackErr != nil {
			return fmt.Errorf("parse fallback callback URL: %w", fallbackErr)
		}

		parsedURL = fallback
	}

	listenAddr := parsedURL.Host

	tlsCert, certErr := generateSelfSignedCert()
	if certErr != nil {
		return fmt.Errorf("generate TLS cert: %w", certErr)
	}

	codeChan := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, req *http.Request) {
		code := req.URL.Query().Get("code")

		decodedCode, decodeErr := url.QueryUnescape(code)
		if decodeErr != nil {
			decodedCode = code
		}

		writer.Header().Set("Content-Type", "text/html")
		fmt.Fprint(writer, "<html><body><h1>Authorization complete. You may close this window.</h1></body></html>")

		codeChan <- decodedCode
	})

	listener, listenErr := tls.Listen("tcp", listenAddr, &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
	if listenErr != nil {
		return fmt.Errorf("listen on %s: %w", listenAddr, listenErr)
	}

	actualAddr := listener.Addr().String()
	manager.callbackURL = fmt.Sprintf("https://%s", actualAddr)

	server := &http.Server{Handler: mux}

	go func() {
		serveErr := server.Serve(listener)
		if serveErr != nil && serveErr != http.ErrServerClosed {
			fmt.Printf("auth callback server error: %v\n", serveErr)
		}
	}()

	fmt.Printf("\nOpen the following URL in your browser to authorize:\n\n  %s\n\nWaiting for callback...\n", manager.authorizationURL())

	code := <-codeChan

	server.Close()

	return manager.exchangeAuthCode(code)
}

func (manager *tokenManager) startBackgroundRefresh() {
	manager.refreshWg.Add(1)

	go func() {
		defer manager.refreshWg.Done()

		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-manager.stopRefresh:
				return
			case <-ticker.C:
				manager.mu.Lock()
				if manager.accessTokenExpired() && manager.tokens.RefreshToken != "" {
					refreshErr := manager.refreshAccessToken()
					if refreshErr == nil && manager.onRefresh != nil {
						manager.onRefresh(manager.accessToken())
					}
				}
				manager.mu.Unlock()
			}
		}
	}()
}

func (manager *tokenManager) stopBackgroundRefresh() {
	close(manager.stopRefresh)
	manager.refreshWg.Wait()
}

func (manager *tokenManager) accessToken() string {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	return manager.tokens.AccessToken
}

func generateSelfSignedCert() (tls.Certificate, error) {
	privateKey, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if keyErr != nil {
		return tls.Certificate{}, keyErr
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"pvbt"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, certErr := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if certErr != nil {
		return tls.Certificate{}, certErr
	}

	keyDER, marshalErr := x509.MarshalECPrivateKey(privateKey)
	if marshalErr != nil {
		return tls.Certificate{}, marshalErr
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add broker/tradestation/auth.go broker/tradestation/auth_test.go
git commit -m "feat(tradestation): add OAuth 2.0 token manager (#19)"
```

---

### Task 5: API client

**Files:**
- Create: `broker/tradestation/client.go`
- Create: `broker/tradestation/client_test.go`
- Create: `broker/tradestation/exports_test.go`

- [ ] **Step 1: Write failing tests for client**

Create `broker/tradestation/exports_test.go`:

```go
package tradestation

import (
	"context"

	"github.com/penny-vault/pvbt/broker"
)

// Type aliases for test access.
type TSOrderRequest = tsOrderRequest
type TSTimeInForce = tsTimeInForce
type TSOrderResponse = tsOrderResponse
type TSOrderLeg = tsOrderLeg
type TSAccountEntry = tsAccountEntry
type TSPositionEntry = tsPositionEntry
type TSBalanceResponse = tsBalanceResponse
type TSGroupOrderRequest = tsGroupOrderRequest
type HTTPError = broker.HTTPError

// NewAPIClientForTest creates an apiClient for testing.
func NewAPIClientForTest(baseURL, token string) *apiClient {
	return newAPIClient(baseURL, token)
}

// SetAccountID sets the account ID for testing.
func (client *apiClient) SetAccountID(accountID string) {
	client.accountID = accountID
}

func (client *apiClient) ResolveAccount(ctx context.Context, desiredAccount string) (string, error) {
	return client.resolveAccount(ctx, desiredAccount)
}

func (client *apiClient) SubmitOrder(ctx context.Context, order tsOrderRequest) (string, error) {
	return client.submitOrder(ctx, order)
}

func (client *apiClient) CancelOrder(ctx context.Context, orderID string) error {
	return client.cancelOrder(ctx, orderID)
}

func (client *apiClient) ReplaceOrder(ctx context.Context, orderID string, order tsOrderRequest) error {
	return client.replaceOrder(ctx, orderID, order)
}

func (client *apiClient) GetOrders(ctx context.Context) ([]tsOrderResponse, error) {
	return client.getOrders(ctx)
}

func (client *apiClient) GetPositions(ctx context.Context) ([]tsPositionEntry, error) {
	return client.getPositions(ctx)
}

func (client *apiClient) GetBalance(ctx context.Context) (tsBalanceResponse, error) {
	return client.getBalance(ctx)
}

func (client *apiClient) GetQuote(ctx context.Context, symbol string) (float64, error) {
	return client.getQuote(ctx, symbol)
}

func (client *apiClient) SubmitGroupOrder(ctx context.Context, group tsGroupOrderRequest) error {
	return client.submitGroupOrder(ctx, group)
}

// --- Streamer test exports ---

type OrderStreamerForTestType = orderStreamer

func NewOrderStreamerForTest(client *apiClient, fills chan broker.Fill, baseURL string, accountID string, accessToken string) *orderStreamer {
	return newOrderStreamer(client, fills, baseURL, accountID, func() string { return accessToken })
}

func (streamer *orderStreamer) ConnectStreamer(ctx context.Context) error {
	return streamer.connect(ctx)
}

func (streamer *orderStreamer) CloseStreamer() error {
	return streamer.close()
}

// --- Broker test exports ---

func SetClientForTest(tsBroker *TradeStationBroker, client *apiClient) {
	tsBroker.client = client
}

func SetAccountIDForTest(tsBroker *TradeStationBroker, accountID string) {
	tsBroker.accountID = accountID
	if tsBroker.client != nil {
		tsBroker.client.accountID = accountID
	}
}
```

Create `broker/tradestation/client_test.go`:

```go
package tradestation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/tradestation"
)

var _ = Describe("apiClient", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("resolveAccount", func() {
		It("resolves the first account when no account ID is specified", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/v3/brokerage/accounts"))
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]string{
					{"AccountID": "11111111", "AccountType": "Margin", "Status": "Active"},
					{"AccountID": "22222222", "AccountType": "Cash", "Status": "Active"},
				})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			accountID, resolveErr := client.ResolveAccount(ctx, "")
			Expect(resolveErr).ToNot(HaveOccurred())
			Expect(accountID).To(Equal("11111111"))
		})

		It("matches the specified account ID", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]string{
					{"AccountID": "11111111", "AccountType": "Margin", "Status": "Active"},
					{"AccountID": "22222222", "AccountType": "Cash", "Status": "Active"},
				})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			accountID, resolveErr := client.ResolveAccount(ctx, "22222222")
			Expect(resolveErr).ToNot(HaveOccurred())
			Expect(accountID).To(Equal("22222222"))
		})

		It("returns ErrAccountNotFound when no accounts exist", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]string{})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			_, resolveErr := client.ResolveAccount(ctx, "")
			Expect(resolveErr).To(MatchError(tradestation.ErrAccountNotFound))
		})

		It("returns ErrAccountNotFound when specified account is not found", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]string{
					{"AccountID": "11111111", "AccountType": "Margin", "Status": "Active"},
				})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			_, resolveErr := client.ResolveAccount(ctx, "99999999")
			Expect(resolveErr).To(MatchError(tradestation.ErrAccountNotFound))
		})
	})

	Describe("submitOrder", func() {
		It("submits an order and extracts the order ID from the response", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal("POST"))
				Expect(req.URL.Path).To(Equal("/v3/orderexecution/orders"))
				Expect(req.Header.Get("Authorization")).To(Equal("Bearer test-token"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"Orders": []map[string]any{
						{"OrderID": "987654321"},
					},
				})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountID("ACCT-TEST")

			orderID, submitErr := client.SubmitOrder(ctx, tradestation.TSOrderRequest{
				AccountID:   "ACCT-TEST",
				Symbol:      "AAPL",
				Quantity:    "10",
				OrderType:   "Market",
				TradeAction: "BUY",
				TimeInForce: tradestation.TSTimeInForce{Duration: "DAY"},
				Route:       "Intelligent",
			})
			Expect(submitErr).ToNot(HaveOccurred())
			Expect(orderID).To(Equal("987654321"))
		})
	})

	Describe("cancelOrder", func() {
		It("sends a DELETE to the correct URL", func() {
			var deletePath string

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				deletePath = req.URL.Path
				Expect(req.Method).To(Equal("DELETE"))
				writer.WriteHeader(http.StatusOK)
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			cancelErr := client.CancelOrder(ctx, "ORD456")
			Expect(cancelErr).ToNot(HaveOccurred())
			Expect(deletePath).To(Equal("/v3/orderexecution/orders/ORD456"))
		})
	})

	Describe("replaceOrder", func() {
		It("sends a PUT to the correct URL", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal("PUT"))
				Expect(req.URL.Path).To(Equal("/v3/orderexecution/orders/ORDOLD"))
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"Orders": []map[string]any{
						{"OrderID": "ORDNEW"},
					},
				})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			replaceErr := client.ReplaceOrder(ctx, "ORDOLD", tradestation.TSOrderRequest{
				AccountID:   "ACCT-TEST",
				Symbol:      "AAPL",
				Quantity:    "5",
				OrderType:   "Limit",
				TradeAction: "BUY",
				TimeInForce: tradestation.TSTimeInForce{Duration: "DAY"},
				LimitPrice:  "155.00",
				Route:       "Intelligent",
			})
			Expect(replaceErr).ToNot(HaveOccurred())
		})
	})

	Describe("getOrders", func() {
		It("retrieves and parses orders", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/v3/brokerage/accounts/ACCT-TEST/orders"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"Orders": []map[string]any{
						{
							"OrderID":   "123",
							"Status":    "OPN",
							"OrderType": "Limit",
							"LimitPrice": "150.00",
							"Duration":  "DAY",
							"Legs": []map[string]any{
								{
									"BuyOrSell":       "1",
									"Symbol":          "AAPL",
									"QuantityOrdered": "10",
								},
							},
						},
					},
				})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountID("ACCT-TEST")

			orders, getErr := client.GetOrders(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].OrderID).To(Equal("123"))
			Expect(orders[0].Status).To(Equal("OPN"))
		})
	})

	Describe("getPositions", func() {
		It("retrieves and parses positions", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/v3/brokerage/accounts/ACCT-TEST/positions"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]any{
					{
						"Symbol":           "AAPL",
						"Quantity":         "100",
						"AveragePrice":     "150.00",
						"MarketValue":      "15500.00",
						"TodaysProfitLoss": "200.00",
						"Last":             "155.00",
					},
				})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountID("ACCT-TEST")

			positions, getErr := client.GetPositions(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Symbol).To(Equal("AAPL"))
			Expect(positions[0].Quantity).To(Equal("100"))
		})
	})

	Describe("getBalance", func() {
		It("retrieves and parses account balance", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/v3/brokerage/accounts/ACCT-TEST/balances"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]any{
					{
						"CashBalance": "25000.00",
						"Equity":      "50000.00",
						"BuyingPower": "45000.00",
						"MarketValue": "25000.00",
					},
				})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountID("ACCT-TEST")

			balance, getErr := client.GetBalance(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(Equal("25000.00"))
			Expect(balance.Equity).To(Equal("50000.00"))
		})
	})

	Describe("getQuote", func() {
		It("retrieves the last price for a symbol", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/v3/marketdata/quotes/TSLA"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"Quotes": []map[string]any{
						{"Symbol": "TSLA", "Last": 245.50},
					},
				})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			price, getErr := client.GetQuote(ctx, "TSLA")
			Expect(getErr).ToNot(HaveOccurred())
			Expect(price).To(Equal(245.50))
		})
	})

	Describe("submitGroupOrder", func() {
		It("submits a group order", func() {
			var receivedBody map[string]any

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal("POST"))
				Expect(req.URL.Path).To(Equal("/v3/orderexecution/ordergroups"))
				json.NewDecoder(req.Body).Decode(&receivedBody)
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"Orders": []map[string]any{
						{"OrderID": "GRP-1"},
						{"OrderID": "GRP-2"},
					},
				})
			}))
			DeferCleanup(server.Close)

			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			submitErr := client.SubmitGroupOrder(ctx, tradestation.TSGroupOrderRequest{
				Type: "OCO",
				Orders: []tradestation.TSOrderRequest{
					{AccountID: "ACCT", Symbol: "AAPL", Quantity: "10", OrderType: "Limit", TradeAction: "BUY", TimeInForce: tradestation.TSTimeInForce{Duration: "DAY"}, LimitPrice: "150.00", Route: "Intelligent"},
					{AccountID: "ACCT", Symbol: "AAPL", Quantity: "10", OrderType: "Limit", TradeAction: "SELL", TimeInForce: tradestation.TSTimeInForce{Duration: "GTC"}, LimitPrice: "160.00", Route: "Intelligent"},
				},
			})
			Expect(submitErr).ToNot(HaveOccurred())
			Expect(receivedBody["Type"]).To(Equal("OCO"))
		})
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: FAIL -- apiClient not defined

- [ ] **Step 3: Write client.go implementation**

Create `broker/tradestation/client.go`:

```go
package tradestation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
)

type apiClient struct {
	resty     *resty.Client
	accountID string
}

func newAPIClient(baseURL, token string) *apiClient {
	httpClient := resty.New().
		SetBaseURL(baseURL).
		SetRetryCount(3).
		SetRetryWaitTime(1*time.Second).
		SetRetryMaxWaitTime(4*time.Second).
		SetAuthToken(token).
		SetHeader("Content-Type", "application/json")

	httpClient.AddRetryCondition(func(resp *resty.Response, retryErr error) bool {
		if retryErr != nil {
			return broker.IsRetryableError(retryErr)
		}

		return resp.StatusCode() >= 500 || resp.StatusCode() == 429
	})

	return &apiClient{
		resty: httpClient,
	}
}

func (client *apiClient) setToken(token string) {
	client.resty.SetAuthToken(token)
}

func (client *apiClient) resolveAccount(ctx context.Context, desiredAccountID string) (string, error) {
	var accounts []tsAccountEntry

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&accounts).
		Get("/v3/brokerage/accounts")
	if reqErr != nil {
		return "", fmt.Errorf("resolve account: %w", reqErr)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	if len(accounts) == 0 {
		return "", ErrAccountNotFound
	}

	if desiredAccountID == "" {
		return accounts[0].AccountID, nil
	}

	for _, account := range accounts {
		if account.AccountID == desiredAccountID {
			return account.AccountID, nil
		}
	}

	return "", ErrAccountNotFound
}

func (client *apiClient) submitOrder(ctx context.Context, order tsOrderRequest) (string, error) {
	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		Post("/v3/orderexecution/orders")
	if reqErr != nil {
		return "", fmt.Errorf("submit order: %w", reqErr)
	}

	if resp.IsError() {
		return "", broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return extractOrderID(resp.Body()), nil
}

func (client *apiClient) cancelOrder(ctx context.Context, orderID string) error {
	endpoint := fmt.Sprintf("/v3/orderexecution/orders/%s", orderID)

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		Delete(endpoint)
	if reqErr != nil {
		return fmt.Errorf("cancel order: %w", reqErr)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

func (client *apiClient) replaceOrder(ctx context.Context, orderID string, order tsOrderRequest) error {
	endpoint := fmt.Sprintf("/v3/orderexecution/orders/%s", orderID)

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetBody(order).
		Put(endpoint)
	if reqErr != nil {
		return fmt.Errorf("replace order: %w", reqErr)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

func (client *apiClient) getOrders(ctx context.Context) ([]tsOrderResponse, error) {
	endpoint := fmt.Sprintf("/v3/brokerage/accounts/%s/orders", client.accountID)

	var result struct {
		Orders []tsOrderResponse `json:"Orders"`
	}

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&result).
		Get(endpoint)
	if reqErr != nil {
		return nil, fmt.Errorf("get orders: %w", reqErr)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return result.Orders, nil
}

func (client *apiClient) getPositions(ctx context.Context) ([]tsPositionEntry, error) {
	endpoint := fmt.Sprintf("/v3/brokerage/accounts/%s/positions", client.accountID)

	var positions []tsPositionEntry

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&positions).
		Get(endpoint)
	if reqErr != nil {
		return nil, fmt.Errorf("get positions: %w", reqErr)
	}

	if resp.IsError() {
		return nil, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return positions, nil
}

func (client *apiClient) getBalance(ctx context.Context) (tsBalanceResponse, error) {
	endpoint := fmt.Sprintf("/v3/brokerage/accounts/%s/balances", client.accountID)

	var balances []tsBalanceResponse

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetResult(&balances).
		Get(endpoint)
	if reqErr != nil {
		return tsBalanceResponse{}, fmt.Errorf("get balance: %w", reqErr)
	}

	if resp.IsError() {
		return tsBalanceResponse{}, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	if len(balances) == 0 {
		return tsBalanceResponse{}, fmt.Errorf("get balance: no balance data returned")
	}

	return balances[0], nil
}

func (client *apiClient) getQuote(ctx context.Context, symbol string) (float64, error) {
	endpoint := fmt.Sprintf("/v3/marketdata/quotes/%s", symbol)

	resp, reqErr := client.resty.R().
		SetContext(ctx).
		Get(endpoint)
	if reqErr != nil {
		return 0, fmt.Errorf("get quote: %w", reqErr)
	}

	if resp.IsError() {
		return 0, broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	var quoteResp tsQuoteResponse
	if unmarshalErr := json.Unmarshal(resp.Body(), &quoteResp); unmarshalErr != nil {
		return 0, fmt.Errorf("parse quote: %w", unmarshalErr)
	}

	if len(quoteResp.Quotes) == 0 {
		return 0, fmt.Errorf("get quote: no data for symbol %s", symbol)
	}

	return quoteResp.Quotes[0].Last, nil
}

func (client *apiClient) submitGroupOrder(ctx context.Context, group tsGroupOrderRequest) error {
	resp, reqErr := client.resty.R().
		SetContext(ctx).
		SetBody(group).
		Post("/v3/orderexecution/ordergroups")
	if reqErr != nil {
		return fmt.Errorf("submit group order: %w", reqErr)
	}

	if resp.IsError() {
		return broker.NewHTTPError(resp.StatusCode(), resp.String())
	}

	return nil
}

// extractOrderID parses the first OrderID from a TradeStation order response body.
func extractOrderID(body []byte) string {
	var result struct {
		Orders []struct {
			OrderID string `json:"OrderID"`
		} `json:"Orders"`
	}

	if unmarshalErr := json.Unmarshal(body, &result); unmarshalErr != nil || len(result.Orders) == 0 {
		return ""
	}

	return result.Orders[0].OrderID
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add broker/tradestation/client.go broker/tradestation/client_test.go broker/tradestation/exports_test.go
git commit -m "feat(tradestation): add API client with typed endpoint methods (#19)"
```

---

### Task 6: HTTP chunked streamer

**Files:**
- Create: `broker/tradestation/streamer.go`
- Create: `broker/tradestation/streamer_test.go`

- [ ] **Step 1: Write failing tests for streamer**

Create `broker/tradestation/streamer_test.go`:

```go
package tradestation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/tradestation"
)

var _ = Describe("orderStreamer", Label("streaming"), func() {
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

	Describe("Fill delivery", func() {
		It("emits a broker.Fill when a filled order event arrives on the stream", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/v3/brokerage/stream/accounts/ACCT-TEST/orders"))
				Expect(req.Header.Get("Authorization")).To(Equal("Bearer access-token"))

				writer.Header().Set("Content-Type", "application/vnd.tradestation.streams.v3+json")
				flusher := writer.(http.Flusher)

				event := map[string]any{
					"OrderID":        "987654",
					"Status":         "FLL",
					"OrderType":      "Market",
					"FilledQuantity": "50",
					"FilledPrice":    "150.25",
					"Legs": []map[string]any{
						{
							"Symbol":    "AAPL",
							"BuyOrSell": "1",
							"Fills": []map[string]any{
								{
									"ExecId":    "EXEC-1",
									"Quantity":  "50",
									"Price":     "150.25",
									"Timestamp": "2026-03-20T14:30:00Z",
								},
							},
						},
					},
				}

				data, _ := json.Marshal(event)
				fmt.Fprintf(writer, "%s\n", data)
				flusher.Flush()

				// Keep connection open until context cancelled.
				<-req.Context().Done()
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			streamer := tradestation.NewOrderStreamerForTest(client, fills, server.URL, "ACCT-TEST", "access-token")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var received broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&received))
			Expect(received.OrderID).To(Equal("987654"))
			Expect(received.Price).To(Equal(150.25))
			Expect(received.Qty).To(Equal(50.0))
		})
	})

	Describe("Deduplication", func() {
		It("delivers only one fill when the same fill is sent twice", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/vnd.tradestation.streams.v3+json")
				flusher := writer.(http.Flusher)

				event := map[string]any{
					"OrderID":        "111",
					"Status":         "FLL",
					"FilledQuantity": "10",
					"FilledPrice":    "99.50",
					"Legs": []map[string]any{
						{
							"Symbol":    "SPY",
							"BuyOrSell": "1",
							"Fills": []map[string]any{
								{
									"ExecId":    "EXEC-DUP",
									"Quantity":  "10",
									"Price":     "99.50",
									"Timestamp": "2026-03-20T14:30:00Z",
								},
							},
						},
					},
				}

				data, _ := json.Marshal(event)
				// Send same event twice.
				fmt.Fprintf(writer, "%s\n", data)
				flusher.Flush()
				time.Sleep(50 * time.Millisecond)
				fmt.Fprintf(writer, "%s\n", data)
				flusher.Flush()

				<-req.Context().Done()
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			streamer := tradestation.NewOrderStreamerForTest(client, fills, server.URL, "ACCT-TEST", "access-token")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var firstFill broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&firstFill))
			Expect(firstFill.OrderID).To(Equal("111"))

			Consistently(fills, 1*time.Second).ShouldNot(Receive())
		})
	})

	Describe("GoAway signal", func() {
		It("reconnects when a GoAway signal is received", func() {
			connectCount := 0

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				connectCount++
				writer.Header().Set("Content-Type", "application/vnd.tradestation.streams.v3+json")
				flusher := writer.(http.Flusher)

				if connectCount == 1 {
					// First connection: send GoAway.
					goAway := map[string]any{"GoAway": true}
					data, _ := json.Marshal(goAway)
					fmt.Fprintf(writer, "%s\n", data)
					flusher.Flush()
					return
				}

				// Second connection: send a fill then stay open.
				event := map[string]any{
					"OrderID":        "222",
					"Status":         "FLL",
					"FilledQuantity": "5",
					"FilledPrice":    "100.00",
					"Legs": []map[string]any{
						{
							"Symbol":    "QQQ",
							"BuyOrSell": "1",
							"Fills": []map[string]any{
								{
									"ExecId":    "EXEC-GA",
									"Quantity":  "5",
									"Price":     "100.00",
									"Timestamp": "2026-03-20T15:00:00Z",
								},
							},
						},
					},
				}

				data, _ := json.Marshal(event)
				fmt.Fprintf(writer, "%s\n", data)
				flusher.Flush()

				<-req.Context().Done()
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			streamer := tradestation.NewOrderStreamerForTest(client, fills, server.URL, "ACCT-TEST", "access-token")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var received broker.Fill
			Eventually(fills, 5*time.Second).Should(Receive(&received))
			Expect(received.OrderID).To(Equal("222"))
		})
	})

	Describe("Shutdown", func() {
		It("stops the goroutine when CloseStreamer is called", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/vnd.tradestation.streams.v3+json")
				<-req.Context().Done()
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			streamer := tradestation.NewOrderStreamerForTest(client, fills, server.URL, "ACCT-TEST", "access-token")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())

			closeDone := make(chan error, 1)
			go func() {
				closeDone <- streamer.CloseStreamer()
			}()

			Eventually(closeDone, 3*time.Second).Should(Receive(Not(HaveOccurred())))
		})
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: FAIL -- orderStreamer not defined

- [ ] **Step 3: Write streamer.go implementation**

Create `broker/tradestation/streamer.go`:

```go
package tradestation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	maxReconnectAttempts = 3
	maxBackoff           = 30 * time.Second
	pruneThreshold       = 24 * time.Hour
)

type orderStreamer struct {
	client       *apiClient
	fills        chan broker.Fill
	baseURL      string
	accountID    string
	tokenFunc    func() string
	seenFills    map[string]time.Time
	mu           sync.Mutex
	done         chan struct{}
	wg           sync.WaitGroup
	cancelStream context.CancelFunc
	lastPruneDay time.Time
}

func newOrderStreamer(client *apiClient, fills chan broker.Fill, baseURL string, accountID string, tokenFunc func() string) *orderStreamer {
	return &orderStreamer{
		client:    client,
		fills:     fills,
		baseURL:   baseURL,
		accountID: accountID,
		tokenFunc: tokenFunc,
		seenFills: make(map[string]time.Time),
		done:      make(chan struct{}),
	}
}

func (streamer *orderStreamer) connect(ctx context.Context) error {
	streamCtx, cancel := context.WithCancel(ctx)
	streamer.cancelStream = cancel

	resp, connectErr := streamer.openStream(streamCtx)
	if connectErr != nil {
		cancel()
		return fmt.Errorf("order streamer connect: %w", connectErr)
	}

	streamer.wg.Add(1)

	go streamer.run(streamCtx, resp)

	return nil
}

func (streamer *orderStreamer) openStream(ctx context.Context) (*http.Response, error) {
	streamURL := fmt.Sprintf("%s/v3/brokerage/stream/accounts/%s/orders", streamer.baseURL, streamer.accountID)

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if reqErr != nil {
		return nil, fmt.Errorf("create stream request: %w", reqErr)
	}

	req.Header.Set("Authorization", "Bearer "+streamer.tokenFunc())
	req.Header.Set("Accept", "application/vnd.tradestation.streams.v3+json")

	resp, doErr := http.DefaultClient.Do(req)
	if doErr != nil {
		return nil, fmt.Errorf("open stream: %w", doErr)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, broker.NewHTTPError(resp.StatusCode, "stream connection failed")
	}

	return resp, nil
}

func (streamer *orderStreamer) close() error {
	close(streamer.done)

	if streamer.cancelStream != nil {
		streamer.cancelStream()
	}

	streamer.wg.Wait()

	return nil
}

func (streamer *orderStreamer) run(ctx context.Context, resp *http.Response) {
	defer streamer.wg.Done()
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)

	for {
		select {
		case <-streamer.done:
			return
		case <-ctx.Done():
			return
		default:
		}

		var event tsStreamOrderEvent
		if decodeErr := decoder.Decode(&event); decodeErr != nil {
			select {
			case <-streamer.done:
				return
			case <-ctx.Done():
				return
			default:
			}

			if reconnectErr := streamer.reconnect(ctx); reconnectErr != nil {
				return
			}

			// Re-open the stream and restart the decoder.
			newResp, openErr := streamer.openStream(ctx)
			if openErr != nil {
				return
			}

			resp.Body.Close()
			resp = newResp
			decoder = json.NewDecoder(resp.Body)

			continue
		}

		if event.GoAway {
			resp.Body.Close()

			newResp, openErr := streamer.reconnectStream(ctx)
			if openErr != nil {
				return
			}

			resp = newResp
			decoder = json.NewDecoder(resp.Body)

			continue
		}

		if event.Error != "" {
			log.Warn().Str("error", event.Error).Msg("tradestation: stream error")
			continue
		}

		if event.EndSnapshot || event.Heartbeat != 0 {
			continue
		}

		streamer.pruneSeenFills()
		streamer.handleEvent(event)
	}
}

func (streamer *orderStreamer) handleEvent(event tsStreamOrderEvent) {
	if event.Status != "FLL" && event.Status != "FLP" {
		return
	}

	for _, leg := range event.Legs {
		for _, fill := range leg.Fills {
			fillKey := fmt.Sprintf("%s-%s", event.OrderID, fill.ExecID)

			streamer.mu.Lock()

			_, alreadySeen := streamer.seenFills[fillKey]
			if !alreadySeen {
				streamer.seenFills[fillKey] = time.Now()
			}

			streamer.mu.Unlock()

			if alreadySeen {
				continue
			}

			filledAt, parseErr := time.Parse(time.RFC3339, fill.Timestamp)
			if parseErr != nil {
				log.Warn().Err(parseErr).Str("timestamp", fill.Timestamp).Msg("tradestation: could not parse fill timestamp, using current time")
				filledAt = time.Now()
			}

			brokerFill := broker.Fill{
				OrderID:  event.OrderID,
				Price:    parseFloat(fill.Price),
				Qty:      parseFloat(fill.Quantity),
				FilledAt: filledAt,
			}

			select {
			case streamer.fills <- brokerFill:
			case <-streamer.done:
				return
			}
		}
	}
}

func (streamer *orderStreamer) reconnect(ctx context.Context) error {
	backoff := 1 * time.Second

	for attempt := range maxReconnectAttempts {
		select {
		case <-streamer.done:
			return ErrStreamDisconnected
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if attempt > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-streamer.done:
				timer.Stop()
				return ErrStreamDisconnected
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	streamer.pollMissedFills(ctx)

	return nil
}

func (streamer *orderStreamer) reconnectStream(ctx context.Context) (*http.Response, error) {
	backoff := 1 * time.Second

	for attempt := range maxReconnectAttempts {
		select {
		case <-streamer.done:
			return nil, ErrStreamDisconnected
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if attempt > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-streamer.done:
				timer.Stop()
				return nil, ErrStreamDisconnected
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		resp, openErr := streamer.openStream(ctx)
		if openErr != nil {
			continue
		}

		streamer.pollMissedFills(ctx)

		return resp, nil
	}

	return nil, ErrStreamDisconnected
}

func (streamer *orderStreamer) pollMissedFills(ctx context.Context) {
	orders, fetchErr := streamer.client.getOrders(ctx)
	if fetchErr != nil {
		return
	}

	for _, order := range orders {
		if order.Status != "FLL" && order.Status != "FLP" {
			continue
		}

		for _, leg := range order.Legs {
			for _, fill := range leg.Fills {
				fillKey := fmt.Sprintf("%s-%s", order.OrderID, fill.ExecID)

				streamer.mu.Lock()

				_, alreadySeen := streamer.seenFills[fillKey]
				if !alreadySeen {
					streamer.seenFills[fillKey] = time.Now()
				}

				streamer.mu.Unlock()

				if alreadySeen {
					continue
				}

				filledAt, parseErr := time.Parse(time.RFC3339, fill.Timestamp)
				if parseErr != nil {
					filledAt = time.Now()
				}

				brokerFill := broker.Fill{
					OrderID:  order.OrderID,
					Price:    parseFloat(fill.Price),
					Qty:      parseFloat(fill.Quantity),
					FilledAt: filledAt,
				}

				select {
				case streamer.fills <- brokerFill:
				case <-streamer.done:
					return
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (streamer *orderStreamer) pruneSeenFills() {
	today := time.Now().Truncate(pruneThreshold)

	streamer.mu.Lock()
	defer streamer.mu.Unlock()

	if today.Equal(streamer.lastPruneDay) {
		return
	}

	streamer.lastPruneDay = today
	cutoff := time.Now().Add(-pruneThreshold)

	for fillKey, seenAt := range streamer.seenFills {
		if seenAt.Before(cutoff) {
			delete(streamer.seenFills, fillKey)
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add broker/tradestation/streamer.go broker/tradestation/streamer_test.go
git commit -m "feat(tradestation): add HTTP chunked order streamer for fill delivery (#19)"
```

---

### Task 7: Broker implementation

**Files:**
- Create: `broker/tradestation/broker.go`
- Create: `broker/tradestation/broker_test.go`

- [ ] **Step 1: Write failing tests for broker**

Create `broker/tradestation/broker_test.go`:

```go
package tradestation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/tradestation"
)

// Compile-time interface checks.
var _ broker.Broker = (*tradestation.TradeStationBroker)(nil)
var _ broker.GroupSubmitter = (*tradestation.TradeStationBroker)(nil)

var _ = Describe("TradeStationBroker", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	authenticatedBroker := func(extraRoutes func(mux *http.ServeMux)) *tradestation.TradeStationBroker {
		mux := http.NewServeMux()

		if extraRoutes != nil {
			extraRoutes(mux)
		}

		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		tsBroker := tradestation.New()
		client := tradestation.NewAPIClientForTest(server.URL, "test-token")
		client.SetAccountID("ACCT-TEST")
		tradestation.SetClientForTest(tsBroker, client)
		tradestation.SetAccountIDForTest(tsBroker, "ACCT-TEST")

		return tsBroker
	}

	Describe("Constructor and options", func() {
		It("creates a broker with a non-nil fills channel", func() {
			tsBroker := tradestation.New()
			Expect(tsBroker.Fills()).ToNot(BeNil())
		})

		It("applies WithTokenFile option", func() {
			tsBroker := tradestation.New(tradestation.WithTokenFile("/custom/path/tokens.json"))
			Expect(tsBroker).ToNot(BeNil())
		})

		It("applies WithCallbackURL option", func() {
			tsBroker := tradestation.New(tradestation.WithCallbackURL("https://127.0.0.1:9999"))
			Expect(tsBroker).ToNot(BeNil())
		})

		It("applies WithSandbox option", func() {
			tsBroker := tradestation.New(tradestation.WithSandbox())
			Expect(tsBroker).ToNot(BeNil())
		})

		It("applies WithAccountID option", func() {
			tsBroker := tradestation.New(tradestation.WithAccountID("12345"))
			Expect(tsBroker).ToNot(BeNil())
		})
	})

	Describe("Connect", Label("auth"), func() {
		It("returns ErrMissingCredentials when TRADESTATION_CLIENT_ID is not set", func() {
			originalID := os.Getenv("TRADESTATION_CLIENT_ID")
			originalSecret := os.Getenv("TRADESTATION_CLIENT_SECRET")
			os.Unsetenv("TRADESTATION_CLIENT_ID")
			os.Unsetenv("TRADESTATION_CLIENT_SECRET")
			DeferCleanup(func() {
				if originalID != "" {
					os.Setenv("TRADESTATION_CLIENT_ID", originalID)
				}
				if originalSecret != "" {
					os.Setenv("TRADESTATION_CLIENT_SECRET", originalSecret)
				}
			})

			tsBroker := tradestation.New()
			connectErr := tsBroker.Connect(ctx)
			Expect(connectErr).To(MatchError(broker.ErrMissingCredentials))
		})
	})

	Describe("Submit", Label("orders"), func() {
		It("submits a qty-based order", func() {
			var submitCalled atomic.Int32
			var receivedBody map[string]any

			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v3/orderexecution/orders", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)
					json.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"Orders": []map[string]any{{"OrderID": "ORD-QTY-1"}},
					})
				})
			})

			submitErr := tsBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         25,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(submitCalled.Load()).To(Equal(int32(1)))
			Expect(receivedBody["OrderType"]).To(Equal("Market"))
			Expect(receivedBody["Symbol"]).To(Equal("AAPL"))
			Expect(receivedBody["Quantity"]).To(Equal("25"))
		})

		It("converts dollar-amount orders to share quantity", func() {
			var submittedQty string

			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v3/marketdata/quotes/TSLA", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"Quotes": []map[string]any{
							{"Symbol": "TSLA", "Last": 100.0},
						},
					})
				})

				mux.HandleFunc("POST /v3/orderexecution/orders", func(writer http.ResponseWriter, req *http.Request) {
					var body map[string]any
					json.NewDecoder(req.Body).Decode(&body)
					submittedQty = body["Quantity"].(string)

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"Orders": []map[string]any{{"OrderID": "ORD-AMT-1"}},
					})
				})
			})

			submitErr := tsBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "TSLA"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      5000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(submittedQty).To(Equal("50")) // floor(5000 / 100) = 50
		})

		It("returns nil without submitting when dollar amount yields zero shares", func() {
			var submitCalled atomic.Int32

			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v3/marketdata/quotes/BRK.A", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"Quotes": []map[string]any{
							{"Symbol": "BRK.A", "Last": 100.0},
						},
					})
				})

				mux.HandleFunc("POST /v3/orderexecution/orders", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)
					writer.WriteHeader(http.StatusOK)
				})
			})

			submitErr := tsBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "BRK.A"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      50, // floor(50 / 100) = 0
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(submitCalled.Load()).To(Equal(int32(0)))
		})
	})

	Describe("Cancel", Label("orders"), func() {
		It("delegates cancellation to the client with dashes stripped", func() {
			var cancelPath string

			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("DELETE /v3/orderexecution/orders/", func(writer http.ResponseWriter, req *http.Request) {
					cancelPath = req.URL.Path
					writer.WriteHeader(http.StatusOK)
				})
			})

			cancelErr := tsBroker.Cancel(ctx, "ORD-CANCEL-1")
			Expect(cancelErr).ToNot(HaveOccurred())
			Expect(cancelPath).To(Equal("/v3/orderexecution/orders/ORDCANCEL1"))
		})
	})

	Describe("Replace", Label("orders"), func() {
		It("delegates replacement to the client with dashes stripped", func() {
			var replacePath string

			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("PUT /v3/orderexecution/orders/", func(writer http.ResponseWriter, req *http.Request) {
					replacePath = req.URL.Path
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"Orders": []map[string]any{{"OrderID": "ORDREPLACENEW"}},
					})
				})
			})

			replaceErr := tsBroker.Replace(ctx, "ORD-REPLACE-1", broker.Order{
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Limit,
				LimitPrice:  400.0,
				TimeInForce: broker.Day,
			})

			Expect(replaceErr).ToNot(HaveOccurred())
			Expect(replacePath).To(Equal("/v3/orderexecution/orders/ORDREPLACE1"))
		})
	})

	Describe("Orders", func() {
		It("retrieves orders through the broker", func() {
			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v3/brokerage/accounts/ACCT-TEST/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"Orders": []map[string]any{
							{
								"OrderID":   "123",
								"Status":    "OPN",
								"OrderType": "Market",
								"Duration":  "DAY",
								"Legs": []map[string]any{
									{
										"BuyOrSell":       "1",
										"Symbol":          "GOOG",
										"QuantityOrdered": "15",
									},
								},
							},
						},
					})
				})
			})

			orders, getErr := tsBroker.Orders(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].ID).To(Equal("123"))
			Expect(orders[0].Asset.Ticker).To(Equal("GOOG"))
			Expect(orders[0].Qty).To(Equal(15.0))
			Expect(orders[0].Status).To(Equal(broker.OrderOpen))
		})
	})

	Describe("Positions", func() {
		It("retrieves positions through the broker", func() {
			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v3/brokerage/accounts/ACCT-TEST/positions", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode([]map[string]any{
						{
							"Symbol":           "NVDA",
							"Quantity":         "200",
							"AveragePrice":     "450.00",
							"MarketValue":      "95000.00",
							"TodaysProfitLoss": "1250.00",
							"Last":             "475.00",
						},
					})
				})
			})

			positions, getErr := tsBroker.Positions(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Asset.Ticker).To(Equal("NVDA"))
			Expect(positions[0].Qty).To(Equal(200.0))
			Expect(positions[0].AvgOpenPrice).To(Equal(450.0))
		})
	})

	Describe("Balance", func() {
		It("retrieves balance through the broker", func() {
			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v3/brokerage/accounts/ACCT-TEST/balances", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode([]map[string]any{
						{
							"CashBalance": "30000.00",
							"Equity":      "75000.00",
							"BuyingPower": "60000.00",
							"MarketValue": "45000.00",
						},
					})
				})
			})

			balance, getErr := tsBroker.Balance(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(Equal(30000.0))
			Expect(balance.NetLiquidatingValue).To(Equal(75000.0))
			Expect(balance.EquityBuyingPower).To(Equal(60000.0))
		})
	})

	Describe("Transactions", func() {
		It("returns empty slice", func() {
			tsBroker := authenticatedBroker(nil)
			transactions, getErr := tsBroker.Transactions(ctx, context.Background().Done)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(transactions).To(BeEmpty())
		})
	})

	Describe("SubmitGroup", Label("orders"), func() {
		It("submits an OCO group order", func() {
			var receivedBody map[string]any

			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v3/orderexecution/ordergroups", func(writer http.ResponseWriter, req *http.Request) {
					json.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"Orders": []map[string]any{{"OrderID": "OCO-1"}, {"OrderID": "OCO-2"}},
					})
				})
			})

			submitErr := tsBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Limit, LimitPrice: 150.0, TimeInForce: broker.Day},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC},
			}, broker.GroupOCO)

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(receivedBody["Type"]).To(Equal("OCO"))
		})

		It("submits a bracket group order", func() {
			var receivedBody map[string]any

			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v3/orderexecution/ordergroups", func(writer http.ResponseWriter, req *http.Request) {
					json.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"Orders": []map[string]any{{"OrderID": "BRK-1"}},
					})
				})
			})

			submitErr := tsBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Stop, StopPrice: 430.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}, broker.GroupBracket)

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(receivedBody["Type"]).To(Equal("BRK"))
		})

		It("returns ErrEmptyOrderGroup for empty slice", func() {
			tsBroker := authenticatedBroker(nil)
			submitErr := tsBroker.SubmitGroup(ctx, []broker.Order{}, broker.GroupOCO)
			Expect(submitErr).To(MatchError(broker.ErrEmptyOrderGroup))
		})

		It("returns ErrNoEntryOrder when bracket has no entry", func() {
			tsBroker := authenticatedBroker(nil)
			submitErr := tsBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}, broker.GroupBracket)
			Expect(submitErr).To(MatchError(broker.ErrNoEntryOrder))
		})

		It("returns ErrMultipleEntryOrders when bracket has two entries", func() {
			tsBroker := authenticatedBroker(nil)
			submitErr := tsBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
			}, broker.GroupBracket)
			Expect(submitErr).To(MatchError(broker.ErrMultipleEntryOrders))
		})
	})

	Describe("Close", func() {
		It("closes without error when no streamer is connected", func() {
			tsBroker := tradestation.New()
			closeErr := tsBroker.Close()
			Expect(closeErr).ToNot(HaveOccurred())
		})
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: FAIL -- TradeStationBroker not defined

- [ ] **Step 3: Write broker.go implementation**

Create `broker/tradestation/broker.go`:

```go
package tradestation

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	fillChannelSize    = 1024
	apiBaseURLDefault  = "https://api.tradestation.com/v3"
	apiBaseURLSandbox  = "https://sim-api.tradestation.com/v3"
)

// TradeStationBroker implements broker.Broker and broker.GroupSubmitter for the
// TradeStation brokerage.
type TradeStationBroker struct {
	client    *apiClient
	auth      *tokenManager
	streamer  *orderStreamer
	fills     chan broker.Fill
	accountID string
	tokenFile   string
	callbackURL string
	sandbox     bool
	desiredAccountID string
	mu        sync.Mutex
}

// Option configures a TradeStationBroker.
type Option func(*TradeStationBroker)

// WithTokenFile configures the path to the token persistence file.
func WithTokenFile(path string) Option {
	return func(tsBroker *TradeStationBroker) {
		tsBroker.tokenFile = path
	}
}

// WithCallbackURL configures the OAuth callback URL.
func WithCallbackURL(callbackURL string) Option {
	return func(tsBroker *TradeStationBroker) {
		tsBroker.callbackURL = callbackURL
	}
}

// WithSandbox configures the broker to use the simulation environment.
func WithSandbox() Option {
	return func(tsBroker *TradeStationBroker) {
		tsBroker.sandbox = true
	}
}

// WithAccountID configures the account ID to use for trading.
func WithAccountID(accountID string) Option {
	return func(tsBroker *TradeStationBroker) {
		tsBroker.desiredAccountID = accountID
	}
}

// New creates a new TradeStationBroker with the given options.
func New(opts ...Option) *TradeStationBroker {
	tsBroker := &TradeStationBroker{
		fills: make(chan broker.Fill, fillChannelSize),
	}

	for _, opt := range opts {
		opt(tsBroker)
	}

	return tsBroker
}

// Fills returns the channel on which fill notifications are delivered.
func (tsBroker *TradeStationBroker) Fills() <-chan broker.Fill {
	return tsBroker.fills
}

// Connect authenticates with TradeStation and starts the order streamer.
func (tsBroker *TradeStationBroker) Connect(ctx context.Context) error {
	clientID := os.Getenv("TRADESTATION_CLIENT_ID")
	clientSecret := os.Getenv("TRADESTATION_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		return broker.ErrMissingCredentials
	}

	callbackURL := tsBroker.callbackURL
	if callbackURL == "" {
		callbackURL = os.Getenv("TRADESTATION_CALLBACK_URL")
	}

	tokenFile := tsBroker.tokenFile
	if tokenFile == "" {
		tokenFile = os.Getenv("TRADESTATION_TOKEN_FILE")
	}

	tsBroker.auth = newTokenManager(clientID, clientSecret, callbackURL, tokenFile)

	// Attempt to load existing tokens.
	tokens, loadErr := loadTokens(tsBroker.auth.tokenFile)
	if loadErr == nil {
		tsBroker.auth.tokens = tokens
	}

	// Ensure we have a valid access token.
	if ensureErr := tsBroker.auth.ensureValidToken(); ensureErr != nil {
		if authErr := tsBroker.auth.startAuthFlow(); authErr != nil {
			return fmt.Errorf("tradestation: connect: %w", authErr)
		}
	}

	// Create the API client.
	baseURL := apiBaseURLDefault
	if tsBroker.sandbox {
		baseURL = apiBaseURLSandbox
	}

	tsBroker.client = newAPIClient(baseURL, tsBroker.auth.accessToken())

	tsBroker.auth.onRefresh = func(token string) {
		tsBroker.client.setToken(token)
	}

	// Resolve the account ID.
	desiredAccount := tsBroker.desiredAccountID
	if desiredAccount == "" {
		desiredAccount = os.Getenv("TRADESTATION_ACCOUNT_ID")
	}

	accountID, resolveErr := tsBroker.client.resolveAccount(ctx, desiredAccount)
	if resolveErr != nil {
		return fmt.Errorf("tradestation: connect: %w", resolveErr)
	}

	tsBroker.accountID = accountID
	tsBroker.client.accountID = accountID

	// Start the HTTP chunked order streamer.
	tsBroker.streamer = newOrderStreamer(
		tsBroker.client,
		tsBroker.fills,
		baseURL,
		accountID,
		tsBroker.auth.accessToken,
	)

	if connectErr := tsBroker.streamer.connect(ctx); connectErr != nil {
		return fmt.Errorf("tradestation: connect streamer: %w", connectErr)
	}

	// Start background token refresh.
	tsBroker.auth.startBackgroundRefresh()

	return nil
}

// Close stops the background refresh, closes the streamer, and closes the fills channel.
func (tsBroker *TradeStationBroker) Close() error {
	if tsBroker.auth != nil {
		tsBroker.auth.stopBackgroundRefresh()
	}

	if tsBroker.streamer != nil {
		if closeErr := tsBroker.streamer.close(); closeErr != nil {
			return closeErr
		}
	}

	close(tsBroker.fills)

	return nil
}

// Submit places a single order. If Qty is zero and Amount is set, the share
// quantity is derived from the current quote price using math.Floor.
func (tsBroker *TradeStationBroker) Submit(ctx context.Context, order broker.Order) error {
	tsBroker.mu.Lock()
	defer tsBroker.mu.Unlock()

	qty := order.Qty
	if qty == 0 && order.Amount > 0 {
		price, quoteErr := tsBroker.client.getQuote(ctx, order.Asset.Ticker)
		if quoteErr != nil {
			return fmt.Errorf("tradestation: fetching quote for %s: %w", order.Asset.Ticker, quoteErr)
		}

		qty = math.Floor(order.Amount / price)
		if qty == 0 {
			return nil
		}
	}

	order.Qty = qty

	tsOrder, translateErr := toTSOrder(order, tsBroker.accountID)
	if translateErr != nil {
		return translateErr
	}

	_, submitErr := tsBroker.client.submitOrder(ctx, tsOrder)
	if submitErr != nil {
		return fmt.Errorf("tradestation: submit order: %w", submitErr)
	}

	return nil
}

// Cancel cancels an open order by ID. Dashes are stripped per TradeStation requirements.
func (tsBroker *TradeStationBroker) Cancel(ctx context.Context, orderID string) error {
	return tsBroker.client.cancelOrder(ctx, stripDashes(orderID))
}

// Replace cancels an existing order and submits a replacement.
func (tsBroker *TradeStationBroker) Replace(ctx context.Context, orderID string, order broker.Order) error {
	tsOrder, translateErr := toTSOrder(order, tsBroker.accountID)
	if translateErr != nil {
		return translateErr
	}

	return tsBroker.client.replaceOrder(ctx, stripDashes(orderID), tsOrder)
}

// Orders returns all open and recently-completed orders for the account.
func (tsBroker *TradeStationBroker) Orders(ctx context.Context) ([]broker.Order, error) {
	responses, getErr := tsBroker.client.getOrders(ctx)
	if getErr != nil {
		return nil, fmt.Errorf("tradestation: get orders: %w", getErr)
	}

	orders := make([]broker.Order, len(responses))
	for idx, resp := range responses {
		orders[idx] = toBrokerOrder(resp)
	}

	return orders, nil
}

// Positions returns all current positions held in the account.
func (tsBroker *TradeStationBroker) Positions(ctx context.Context) ([]broker.Position, error) {
	responses, getErr := tsBroker.client.getPositions(ctx)
	if getErr != nil {
		return nil, fmt.Errorf("tradestation: get positions: %w", getErr)
	}

	positions := make([]broker.Position, len(responses))
	for idx, resp := range responses {
		positions[idx] = toBrokerPosition(resp)
	}

	return positions, nil
}

// Balance returns the current account balances.
func (tsBroker *TradeStationBroker) Balance(ctx context.Context) (broker.Balance, error) {
	resp, getErr := tsBroker.client.getBalance(ctx)
	if getErr != nil {
		return broker.Balance{}, fmt.Errorf("tradestation: get balance: %w", getErr)
	}

	return toBrokerBalance(resp), nil
}

// Transactions returns transactions since the given time. TradeStation v3 API
// does not provide a transaction history endpoint.
func (tsBroker *TradeStationBroker) Transactions(_ context.Context, _ time.Time) ([]broker.Transaction, error) {
	log.Info().Msg("tradestation: TradeStation v3 API does not provide a transaction history endpoint; dividends, splits, and fees will not be synced")
	return nil, nil
}

// SubmitGroup submits a group of orders as a native OCO or bracket order.
func (tsBroker *TradeStationBroker) SubmitGroup(ctx context.Context, orders []broker.Order, groupType broker.GroupType) error {
	tsBroker.mu.Lock()
	defer tsBroker.mu.Unlock()

	groupOrder, buildErr := buildGroupOrder(orders, groupType, tsBroker.accountID)
	if buildErr != nil {
		return buildErr
	}

	return tsBroker.client.submitGroupOrder(ctx, groupOrder)
}

// Ensure Qty field is serialized as an integer string when it's a whole number.
func formatQty(qty float64) string {
	if qty == math.Floor(qty) {
		return strconv.FormatInt(int64(qty), 10)
	}
	return strconv.FormatFloat(qty, 'f', -1, 64)
}
```

Note: In the `toTSOrder` function in `types.go`, the `Quantity` field should use `formatQty` instead of `strconv.FormatFloat`. Update the line in `types.go`:

Replace `Quantity: strconv.FormatFloat(order.Qty, 'f', -1, 64)` with `Quantity: formatQty(order.Qty)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: all tests pass

- [ ] **Step 5: Fix the Transactions test**

The `Transactions` test in broker_test.go has a bug -- it should use `time.Time{}` instead of `context.Background().Done`. Fix the test:

```go
// Replace:
transactions, getErr := tsBroker.Transactions(ctx, context.Background().Done)
// With:
transactions, getErr := tsBroker.Transactions(ctx, time.Time{})
```

- [ ] **Step 6: Run all tests to verify everything passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./broker/tradestation/...`
Expected: all tests pass

- [ ] **Step 7: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./broker/tradestation/...`
Expected: no lint errors. Fix any that appear.

- [ ] **Step 8: Commit**

```bash
git add broker/tradestation/broker.go broker/tradestation/broker_test.go
git commit -m "feat(tradestation): implement Broker and GroupSubmitter interfaces (#19)"
```

---

### Task 8: Final integration validation

**Files:**
- Modify: `broker/tradestation/types.go` (if `formatQty` move needed)

- [ ] **Step 1: Run the full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./broker/tradestation/...`
Expected: all tests pass with race detector enabled

- [ ] **Step 2: Run the project-wide linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && make lint`
Expected: no lint errors

- [ ] **Step 3: Run the project-wide test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && make test`
Expected: all tests pass (no regressions in other packages)

- [ ] **Step 4: Verify the package builds**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./broker/tradestation/...`
Expected: no errors

- [ ] **Step 5: Final commit if any fixes were needed**

Only if changes were required during integration validation.
