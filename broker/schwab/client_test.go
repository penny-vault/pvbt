package schwab_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/schwab"
)

var _ = Describe("apiClient", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("resolveAccount", func() {
		It("resolves the first account when no SCHWAB_ACCOUNT_NUMBER is set", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/trader/v1/accounts/accountNumbers"))
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]string{
					{"accountNumber": "11111111", "hashValue": "HASH-AAA"},
					{"accountNumber": "22222222", "hashValue": "HASH-BBB"},
				})
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")

			accountHash, resolveErr := client.ResolveAccount(ctx, "")
			Expect(resolveErr).ToNot(HaveOccurred())
			Expect(accountHash).To(Equal("HASH-AAA"))
		})

		It("matches the specified account number", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]string{
					{"accountNumber": "11111111", "hashValue": "HASH-AAA"},
					{"accountNumber": "22222222", "hashValue": "HASH-BBB"},
				})
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")

			accountHash, resolveErr := client.ResolveAccount(ctx, "22222222")
			Expect(resolveErr).ToNot(HaveOccurred())
			Expect(accountHash).To(Equal("HASH-BBB"))
		})

		It("returns ErrAccountNotFound when no accounts exist", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]string{})
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")

			_, resolveErr := client.ResolveAccount(ctx, "")
			Expect(resolveErr).To(MatchError(schwab.ErrAccountNotFound))
		})

		It("returns ErrAccountNotFound when specified account is not found", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]string{
					{"accountNumber": "11111111", "hashValue": "HASH-AAA"},
				})
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")

			_, resolveErr := client.ResolveAccount(ctx, "99999999")
			Expect(resolveErr).To(MatchError(schwab.ErrAccountNotFound))
		})
	})

	Describe("submitOrder", func() {
		It("submits an order and extracts the order ID from the Location header", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal("POST"))
				Expect(req.URL.Path).To(Equal("/trader/v1/accounts/HASH-TEST/orders"))
				Expect(req.Header.Get("Authorization")).To(Equal("Bearer test-token"))

				writer.Header().Set("Location", "/v1/accounts/HASH-TEST/orders/987654321")
				writer.WriteHeader(http.StatusCreated)
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountHash("HASH-TEST")

			orderID, submitErr := client.SubmitOrder(ctx, schwab.SchwabOrderRequest{
				OrderType:         "MARKET",
				Session:           "NORMAL",
				Duration:          "DAY",
				OrderStrategyType: "SINGLE",
				OrderLegCollection: []schwab.SchwabOrderLegEntry{
					{Instruction: "BUY", Quantity: 10, Instrument: schwab.SchwabInstrument{Symbol: "AAPL", AssetType: "EQUITY"}},
				},
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

			client := schwab.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountHash("HASH-TEST")

			cancelErr := client.CancelOrder(ctx, "ORD-456")
			Expect(cancelErr).ToNot(HaveOccurred())
			Expect(deletePath).To(Equal("/trader/v1/accounts/HASH-TEST/orders/ORD-456"))
		})
	})

	Describe("replaceOrder", func() {
		It("sends a PUT and extracts the new order ID from Location header", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.Method).To(Equal("PUT"))
				Expect(req.URL.Path).To(Equal("/trader/v1/accounts/HASH-TEST/orders/ORD-OLD"))
				writer.Header().Set("Location", "/v1/accounts/HASH-TEST/orders/ORD-NEW")
				writer.WriteHeader(http.StatusCreated)
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountHash("HASH-TEST")

			newOrderID, replaceErr := client.ReplaceOrder(ctx, "ORD-OLD", schwab.SchwabOrderRequest{
				OrderType:         "LIMIT",
				Session:           "NORMAL",
				Duration:          "DAY",
				OrderStrategyType: "SINGLE",
				Price:             155.0,
				OrderLegCollection: []schwab.SchwabOrderLegEntry{
					{Instruction: "BUY", Quantity: 5, Instrument: schwab.SchwabInstrument{Symbol: "AAPL", AssetType: "EQUITY"}},
				},
			})
			Expect(replaceErr).ToNot(HaveOccurred())
			Expect(newOrderID).To(Equal("ORD-NEW"))
		})
	})

	Describe("getOrders", func() {
		It("retrieves and parses orders", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/trader/v1/accounts/HASH-TEST/orders"))
				Expect(req.URL.Query().Get("fromEnteredTime")).ToNot(BeEmpty())
				Expect(req.URL.Query().Get("toEnteredTime")).ToNot(BeEmpty())

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode([]map[string]any{
					{
						"orderId":           123,
						"status":            "WORKING",
						"orderType":         "LIMIT",
						"price":             150.0,
						"duration":          "DAY",
						"orderStrategyType": "SINGLE",
						"orderLegCollection": []map[string]any{
							{
								"instruction": "BUY",
								"quantity":    10,
								"instrument": map[string]any{
									"symbol":    "AAPL",
									"assetType": "EQUITY",
								},
							},
						},
					},
				})
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountHash("HASH-TEST")

			orders, getErr := client.GetOrders(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].OrderID).To(Equal(int64(123)))
			Expect(orders[0].Status).To(Equal("WORKING"))
		})
	})

	Describe("getPositions", func() {
		It("retrieves and parses positions", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/trader/v1/accounts/HASH-TEST"))
				Expect(req.URL.Query().Get("fields")).To(Equal("positions"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"securitiesAccount": map[string]any{
						"positions": []map[string]any{
							{
								"instrument":           map[string]any{"symbol": "AAPL", "assetType": "EQUITY"},
								"longQuantity":         100.0,
								"shortQuantity":        0.0,
								"averagePrice":         150.0,
								"marketValue":          15500.0,
								"currentDayProfitLoss": 200.0,
							},
						},
					},
				})
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountHash("HASH-TEST")

			positions, getErr := client.GetPositions(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Instrument.Symbol).To(Equal("AAPL"))
			Expect(positions[0].LongQuantity).To(Equal(100.0))
		})
	})

	Describe("getBalance", func() {
		It("retrieves and parses account balance", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/trader/v1/accounts/HASH-TEST"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"securitiesAccount": map[string]any{
						"accountNumber": "12345678",
						"type":          "MARGIN",
						"currentBalances": map[string]any{
							"cashBalance":            25000.0,
							"equity":                 50000.0,
							"buyingPower":            45000.0,
							"maintenanceRequirement": 5000.0,
						},
					},
				})
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountHash("HASH-TEST")

			balance, getErr := client.GetBalance(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(balance.SecuritiesAccount.CurrentBalances.CashBalance).To(Equal(25000.0))
			Expect(balance.SecuritiesAccount.CurrentBalances.Equity).To(Equal(50000.0))
		})
	})

	Describe("getQuote", func() {
		It("retrieves the last price for a symbol", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/marketdata/v1/TSLA/quotes"))
				Expect(req.URL.Query().Get("fields")).To(Equal("quote"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"TSLA": map[string]any{
						"quote": map[string]any{
							"lastPrice": 245.50,
						},
					},
				})
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")

			price, getErr := client.GetQuote(ctx, "TSLA")
			Expect(getErr).ToNot(HaveOccurred())
			Expect(price).To(Equal(245.50))
		})
	})

	Describe("getUserPreference", func() {
		It("retrieves streamer connection info", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/trader/v1/userPreference"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"streamerInfo": []map[string]any{
						{
							"streamerSocketUrl":      "wss://streamer.schwab.com",
							"schwabClientCustomerId": "cust-123",
							"schwabClientCorrelId":   "correl-456",
							"schwabClientChannel":    "channel-A",
							"schwabClientFunctionId": "func-B",
						},
					},
				})
			}))
			DeferCleanup(server.Close)

			client := schwab.NewAPIClientForTest(server.URL, "test-token")

			pref, getErr := client.GetUserPreference(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(pref.StreamerInfo).To(HaveLen(1))
			Expect(pref.StreamerInfo[0].StreamerSocketURL).To(Equal("wss://streamer.schwab.com"))
			Expect(pref.StreamerInfo[0].SchwabClientCustomerID).To(Equal("cust-123"))
		})
	})
})
