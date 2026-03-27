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
							"OrderID":    "123",
							"Status":     "OPN",
							"OrderType":  "Limit",
							"LimitPrice": "150.00",
							"Duration":   "DAY",
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
