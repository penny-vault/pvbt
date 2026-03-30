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

package alpaca_test

import (
	"context"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/alpaca"
)

var _ = Describe("apiClient", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	newClient := func(extraRoutes func(mux *http.ServeMux)) (*alpaca.APIClientForTestType, *httptest.Server) {
		mux := http.NewServeMux()
		if extraRoutes != nil {
			extraRoutes(mux)
		}

		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		client := alpaca.NewAPIClientForTest(server.URL, "test-key", "test-secret")

		return client, server
	}

	Describe("Account", func() {
		It("returns account data with correct fields", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/account", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"id":                 "abc-123",
						"status":             "ACTIVE",
						"cash":               "25000",
						"equity":             "50000",
						"buying_power":       "45000",
						"maintenance_margin": "5000",
					})
				})
			})

			account, err := client.GetAccount(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(account.ID).To(Equal("abc-123"))
			Expect(account.Status).To(Equal("ACTIVE"))
			Expect(account.Cash).To(Equal("25000"))
			Expect(account.Equity).To(Equal("50000"))
			Expect(account.BuyingPower).To(Equal("45000"))
			Expect(account.MaintenanceMargin).To(Equal("5000"))
		})

		It("sends API key headers", func() {
			var capturedKeyHeader string
			var capturedSecretHeader string

			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/account", func(writer http.ResponseWriter, req *http.Request) {
					capturedKeyHeader = req.Header.Get("APCA-API-KEY-ID")
					capturedSecretHeader = req.Header.Get("APCA-API-SECRET-KEY")

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"id":     "abc-123",
						"status": "ACTIVE",
					})
				})
			})

			_, err := client.GetAccount(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedKeyHeader).To(Equal("test-key"))
			Expect(capturedSecretHeader).To(Equal("test-secret"))
		})

		It("returns HTTPError on auth failure", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/account", func(writer http.ResponseWriter, req *http.Request) {
					writer.WriteHeader(http.StatusUnauthorized)
					writer.Write([]byte(`{"message":"forbidden"}`))
				})
			})

			_, err := client.GetAccount(ctx)
			Expect(err).To(HaveOccurred())

			var httpErr *alpaca.HTTPError
			Expect(err).To(BeAssignableToTypeOf(httpErr))
		})
	})

	Describe("Orders", func() {
		It("submits an order with correct JSON body and returns order ID", func() {
			var capturedBody map[string]any

			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&capturedBody)

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"id":     "order-123",
						"status": "new",
						"symbol": "AAPL",
					})
				})
			})

			order := alpaca.OrderRequestExport{
				Symbol:      "AAPL",
				Qty:         "10",
				Side:        "buy",
				Type:        "limit",
				TimeInForce: "day",
				LimitPrice:  "150.50",
			}

			orderID, err := client.SubmitOrder(ctx, order)
			Expect(err).ToNot(HaveOccurred())
			Expect(orderID).To(Equal("order-123"))
			Expect(capturedBody["symbol"]).To(Equal("AAPL"))
			Expect(capturedBody["type"]).To(Equal("limit"))
			Expect(capturedBody["limit_price"]).To(Equal("150.50"))
		})

		It("cancels an order at the correct path", func() {
			var cancelPath string

			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("DELETE /v2/orders/ORD-456", func(writer http.ResponseWriter, req *http.Request) {
					cancelPath = req.URL.Path
					writer.WriteHeader(http.StatusOK)
				})
			})

			err := client.CancelOrder(ctx, "ORD-456")
			Expect(err).ToNot(HaveOccurred())
			Expect(cancelPath).To(Equal("/v2/orders/ORD-456"))
		})

		It("replaces an order with PATCH and returns new order ID", func() {
			var replacePath string
			var replaceBody map[string]any

			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("PATCH /v2/orders/ORD-789", func(writer http.ResponseWriter, req *http.Request) {
					replacePath = req.URL.Path
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&replaceBody)

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"id":     "ORD-NEW",
						"status": "accepted",
					})
				})
			})

			replacement := alpaca.ReplaceRequestExport{
				Qty:        "20",
				LimitPrice: "155.00",
			}

			newOrderID, err := client.ReplaceOrder(ctx, "ORD-789", replacement)
			Expect(err).ToNot(HaveOccurred())
			Expect(replacePath).To(Equal("/v2/orders/ORD-789"))
			Expect(newOrderID).To(Equal("ORD-NEW"))
			Expect(replaceBody["qty"]).To(Equal("20"))
			Expect(replaceBody["limit_price"]).To(Equal("155.00"))
		})

		It("retrieves and parses an order list", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					Expect(req.URL.Query().Get("status")).To(Equal("open"))
					Expect(req.URL.Query().Get("limit")).To(Equal("500"))

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]map[string]any{
						{
							"id":            "ORD-A",
							"status":        "new",
							"type":          "market",
							"side":          "buy",
							"symbol":        "AAPL",
							"qty":           "10",
							"time_in_force": "day",
						},
						{
							"id":            "ORD-B",
							"status":        "partially_filled",
							"type":          "limit",
							"side":          "sell",
							"symbol":        "GOOG",
							"qty":           "5",
							"time_in_force": "gtc",
							"limit_price":   "100.00",
						},
					})
				})
			})

			orders, err := client.GetOrders(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(2))
			Expect(orders[0].ID).To(Equal("ORD-A"))
			Expect(orders[0].Status).To(Equal("new"))
			Expect(orders[1].ID).To(Equal("ORD-B"))
			Expect(orders[1].LimitPrice).To(Equal("100.00"))
		})
	})

	Describe("Positions", func() {
		It("retrieves and parses positions", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/positions", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]map[string]any{
						{
							"symbol":                 "AAPL",
							"qty":                    "100",
							"avg_entry_price":        "150.25",
							"current_price":          "155.00",
							"unrealized_intraday_pl": "475.00",
						},
					})
				})
			})

			positions, err := client.GetPositions(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Symbol).To(Equal("AAPL"))
			Expect(positions[0].Qty).To(Equal("100"))
			Expect(positions[0].AvgEntryPrice).To(Equal("150.25"))
			Expect(positions[0].CurrentPrice).To(Equal("155.00"))
			Expect(positions[0].UnrealizedIntradayPL).To(Equal("475.00"))
		})
	})

	Describe("Quotes", func() {
		It("retrieves the last trade price", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/stocks/TSLA/trades/latest", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"trade": map[string]any{
							"p": "245.50",
						},
					})
				})
			})

			price, err := client.GetLatestTrade(ctx, "TSLA")
			Expect(err).ToNot(HaveOccurred())
			Expect(price).To(Equal(245.50))
		})
	})

	Describe("Retry", func() {
		It("retries on 500 and eventually succeeds", func() {
			var requestCount int64

			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/positions", func(writer http.ResponseWriter, req *http.Request) {
					count := atomic.AddInt64(&requestCount, 1)
					if count <= 2 {
						writer.WriteHeader(http.StatusInternalServerError)
						writer.Write([]byte(`{"error":"server error"}`))

						return
					}

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]map[string]any{
						{
							"symbol": "GOOG",
							"qty":    "50",
						},
					})
				})
			})

			positions, err := client.GetPositions(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Symbol).To(Equal("GOOG"))
			Expect(atomic.LoadInt64(&requestCount)).To(BeNumerically(">=", 3))
		})
	})
})
