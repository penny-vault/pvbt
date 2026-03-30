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

package webull_test

import (
	"context"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/webull"
)

var _ = Describe("apiClient", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	newClient := func(extraRoutes func(mux *http.ServeMux)) (*webull.APIClientForTestType, *httptest.Server) {
		mux := http.NewServeMux()
		if extraRoutes != nil {
			extraRoutes(mux)
		}

		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		client := webull.NewAPIClientForTest(server.URL, "test-key", "test-secret")

		return client, server
	}

	Describe("getAccounts", func() {
		It("returns account list", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /api/trade/account/list", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"accounts": []map[string]string{
							{"account_id": "acct-1", "status": "ACTIVE"},
							{"account_id": "acct-2", "status": "INACTIVE"},
						},
					})
				})
			})

			accounts, err := client.GetAccounts(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(accounts).To(HaveLen(2))
			Expect(accounts[0].AccountID).To(Equal("acct-1"))
			Expect(accounts[0].Status).To(Equal("ACTIVE"))
			Expect(accounts[1].AccountID).To(Equal("acct-2"))
		})
	})

	Describe("submitOrder", func() {
		It("sends order and returns order ID", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /api/trade/order/place", func(writer http.ResponseWriter, req *http.Request) {
					var body webull.OrderRequestExport
					Expect(sonic.ConfigDefault.NewDecoder(req.Body).Decode(&body)).To(Succeed())
					Expect(body.Side).To(Equal("BUY"))
					Expect(body.OrderType).To(Equal("MARKET"))

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]string{
						"order_id": "ord-abc-123",
					})
				})
			})

			orderID, err := client.SubmitOrder(ctx, "acct-1", webull.OrderRequestExport{
				Symbol:      "AAPL",
				Side:        "BUY",
				OrderType:   "MARKET",
				TimeInForce: "DAY",
				Qty:         "10",
				ClientID:    "test-client-id",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(orderID).To(Equal("ord-abc-123"))
		})

		It("returns HTTPError on non-2xx", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /api/trade/order/place", func(writer http.ResponseWriter, req *http.Request) {
					writer.WriteHeader(http.StatusBadRequest)
					writer.Write([]byte(`{"error":"invalid order"}`))
				})
			})

			orderID, err := client.SubmitOrder(ctx, "acct-1", webull.OrderRequestExport{
				Symbol:    "AAPL",
				Side:      "BUY",
				OrderType: "MARKET",
				Qty:       "10",
				ClientID:  "test-client-id",
			})
			Expect(err).To(HaveOccurred())
			Expect(orderID).To(BeEmpty())

			var httpErr *webull.HTTPError
			Expect(err).To(BeAssignableToTypeOf(httpErr))
		})
	})

	Describe("cancelOrder", func() {
		It("cancels an order", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /api/trade/order/cancel", func(writer http.ResponseWriter, req *http.Request) {
					var body map[string]string
					Expect(sonic.ConfigDefault.NewDecoder(req.Body).Decode(&body)).To(Succeed())
					Expect(body["account_id"]).To(Equal("acct-1"))
					Expect(body["order_id"]).To(Equal("ord-123"))

					writer.WriteHeader(http.StatusOK)
				})
			})

			err := client.CancelOrder(ctx, "acct-1", "ord-123")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("replaceOrder", func() {
		It("replaces an order", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /api/trade/order/replace", func(writer http.ResponseWriter, req *http.Request) {
					Expect(req.URL.Query().Get("account_id")).To(Equal("acct-1"))
					Expect(req.URL.Query().Get("order_id")).To(Equal("ord-123"))

					var body webull.ReplaceRequestExport
					Expect(sonic.ConfigDefault.NewDecoder(req.Body).Decode(&body)).To(Succeed())
					Expect(body.Qty).To(Equal("20"))
					Expect(body.LimitPrice).To(Equal("150.50"))

					writer.WriteHeader(http.StatusOK)
				})
			})

			err := client.ReplaceOrder(ctx, "acct-1", "ord-123", webull.ReplaceRequestExport{
				Qty:        "20",
				LimitPrice: "150.50",
			})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("getOrders", func() {
		It("returns order list", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /api/trade/order/list", func(writer http.ResponseWriter, req *http.Request) {
					Expect(req.URL.Query().Get("account_id")).To(Equal("acct-1"))

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"orders": []map[string]string{
							{
								"order_id":         "ord-1",
								"symbol":           "AAPL",
								"side":             "BUY",
								"order_status":     "FILLED",
								"order_type":       "MARKET",
								"qty":              "10",
								"filled_qty":       "10",
								"filled_avg_price": "150.25",
							},
							{
								"order_id":     "ord-2",
								"symbol":       "MSFT",
								"side":         "SELL",
								"order_status": "PENDING",
								"order_type":   "LIMIT",
								"qty":          "5",
								"limit_price":  "300.00",
							},
						},
					})
				})
			})

			orders, err := client.GetOrders(ctx, "acct-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(2))
			Expect(orders[0].ID).To(Equal("ord-1"))
			Expect(orders[0].Symbol).To(Equal("AAPL"))
			Expect(orders[0].FilledPrice).To(Equal("150.25"))
			Expect(orders[1].ID).To(Equal("ord-2"))
			Expect(orders[1].Side).To(Equal("SELL"))
		})
	})

	Describe("getPositions", func() {
		It("returns positions", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /api/trade/account/positions", func(writer http.ResponseWriter, req *http.Request) {
					Expect(req.URL.Query().Get("account_id")).To(Equal("acct-1"))

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"positions": []map[string]string{
							{
								"symbol":        "AAPL",
								"qty":           "100",
								"avg_cost":      "145.00",
								"market_value":  "15000.00",
								"unrealized_pl": "500.00",
							},
							{
								"symbol":        "MSFT",
								"qty":           "50",
								"avg_cost":      "280.00",
								"market_value":  "15000.00",
								"unrealized_pl": "1000.00",
							},
						},
					})
				})
			})

			positions, err := client.GetPositions(ctx, "acct-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(2))
			Expect(positions[0].Symbol).To(Equal("AAPL"))
			Expect(positions[0].Qty).To(Equal("100"))
			Expect(positions[0].AvgCost).To(Equal("145.00"))
			Expect(positions[1].Symbol).To(Equal("MSFT"))
		})
	})

	Describe("getBalance", func() {
		It("returns account balance", func() {
			client, _ := newClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /api/trade/account/detail", func(writer http.ResponseWriter, req *http.Request) {
					Expect(req.URL.Query().Get("account_id")).To(Equal("acct-1"))

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]string{
						"account_id":      "acct-1",
						"net_liquidation": "50000.00",
						"cash_balance":    "25000.00",
						"buying_power":    "45000.00",
						"maintenance_req": "5000.00",
					})
				})
			})

			balance, err := client.GetBalance(ctx, "acct-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(balance.AccountID).To(Equal("acct-1"))
			Expect(balance.NetLiquidation).To(Equal("50000.00"))
			Expect(balance.CashBalance).To(Equal("25000.00"))
			Expect(balance.BuyingPower).To(Equal("45000.00"))
			Expect(balance.MaintenanceReq).To(Equal("5000.00"))
		})
	})
})
