package etrade_test

import (
	"context"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/etrade"
)

// testCreds returns a set of fake OAuth credentials for tests.
func testCreds() *etrade.OAuthCredentials {
	return &etrade.OAuthCredentials{
		ConsumerKey:    "test-consumer-key",
		ConsumerSecret: "test-consumer-secret",
		AccessToken:    "test-access-token",
		AccessSecret:   "test-access-secret",
	}
}

const testAccountIDKey = "ABCD1234"

var _ = Describe("apiClient", func() {
	var (
		mux    *http.ServeMux
		server *httptest.Server
		cl     *etrade.APIClientForTest
	)

	BeforeEach(func() {
		mux = http.NewServeMux()
		server = httptest.NewServer(mux)
		cl = etrade.NewAPIClientForTest(server.URL, testCreds(), testAccountIDKey)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("GetBalance", func() {
		It("returns the balance response on success", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/balance.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodGet))
				Expect(rr.Header.Get("Authorization")).To(HavePrefix("OAuth "))
				Expect(rr.URL.Query().Get("instType")).To(Equal("BROKERAGE"))
				Expect(rr.URL.Query().Get("realTimeNAV")).To(Equal("true"))

				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"BalanceResponse": map[string]any{
						"totalAccountValue": 100000.0,
						"accountType":       "MARGIN",
						"Computed": map[string]any{
							"cashAvailableForInvestment": 25000.0,
							"cashBuyingPower":            25000.0,
							"marginBuyingPower":          50000.0,
							"reqMaintenanceValue":        10000.0,
							"RealTimeValues": map[string]any{
								"totalAccountValue": 100000.0,
							},
						},
					},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			result, err := cl.GetBalance(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(result.BalanceResponse.Computed.CashAvailableForInvestment).To(Equal(25000.0))
			Expect(result.BalanceResponse.Computed.MarginBuyingPower).To(Equal(50000.0))
		})

		It("returns an HTTPError on non-2xx response", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/balance.json", func(ww http.ResponseWriter, _ *http.Request) {
				http.Error(ww, "Unauthorized", http.StatusUnauthorized)
			})

			_, err := cl.GetBalance(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("401"))
		})
	})

	Describe("GetPositions", func() {
		It("returns positions on success", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/portfolio.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodGet))
				Expect(rr.Header.Get("Authorization")).To(HavePrefix("OAuth "))

				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"PortfolioResponse": map[string]any{
						"AccountPortfolio": []map[string]any{
							{
								"Position": []map[string]any{
									{
										"positionId":   int64(111),
										"quantity":     10.0,
										"positionType": "LONG",
										"costPerShare": 150.0,
										"marketValue":  1600.0,
										"totalGain":    100.0,
										"Product": map[string]any{
											"symbol":       "AAPL",
											"securityType": "EQ",
										},
									},
								},
							},
						},
					},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			positions, err := cl.GetPositions(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Product.Symbol).To(Equal("AAPL"))
			Expect(positions[0].Quantity).To(Equal(10.0))
		})

		It("returns empty slice for empty portfolio", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/portfolio.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"PortfolioResponse": map[string]any{
						"AccountPortfolio": []any{},
					},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			positions, err := cl.GetPositions(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(positions).To(BeEmpty())
		})

		It("returns an HTTPError on non-2xx response", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/portfolio.json", func(ww http.ResponseWriter, _ *http.Request) {
				http.Error(ww, "Internal Server Error", http.StatusInternalServerError)
			})

			_, err := cl.GetPositions(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))
		})
	})

	Describe("GetOrders", func() {
		It("returns orders on success", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodGet))
				Expect(rr.Header.Get("Authorization")).To(HavePrefix("OAuth "))

				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"OrdersResponse": map[string]any{
						"Order": []map[string]any{
							{
								"orderId":     int64(42),
								"orderType":   "EQ",
								"orderStatus": "OPEN",
								"OrderDetail": []map[string]any{
									{
										"priceType":  "LIMIT",
										"orderTerm":  "GOOD_FOR_DAY",
										"limitPrice": 155.0,
										"stopPrice":  0.0,
										"Instrument": []map[string]any{
											{
												"Product": map[string]any{
													"symbol":       "AAPL",
													"securityType": "EQ",
												},
												"orderAction":           "BUY",
												"orderedQuantity":       5.0,
												"filledQuantity":        0.0,
												"averageExecutionPrice": 0.0,
											},
										},
									},
								},
							},
						},
					},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			orders, err := cl.GetOrders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].OrderID).To(Equal(int64(42)))
			Expect(orders[0].Status).To(Equal("OPEN"))
		})

		It("returns empty slice when no orders exist", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"OrdersResponse": map[string]any{},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			orders, err := cl.GetOrders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(BeEmpty())
		})

		It("returns an HTTPError on non-2xx response", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders.json", func(ww http.ResponseWriter, _ *http.Request) {
				http.Error(ww, "Forbidden", http.StatusForbidden)
			})

			_, err := cl.GetOrders(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("403"))
		})
	})

	Describe("GetQuote", func() {
		It("returns the last trade price on success", func() {
			mux.HandleFunc("/v1/market/quote/MSFT.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodGet))
				Expect(rr.Header.Get("Authorization")).To(HavePrefix("OAuth "))

				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"QuoteResponse": map[string]any{
						"QuoteData": []map[string]any{
							{
								"All": map[string]any{
									"lastTrade": 320.50,
								},
							},
						},
					},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			price, err := cl.GetQuote(context.Background(), "MSFT")
			Expect(err).NotTo(HaveOccurred())
			Expect(price).To(Equal(320.50))
		})

		It("returns an HTTPError on non-2xx response", func() {
			mux.HandleFunc("/v1/market/quote/INVALID.json", func(ww http.ResponseWriter, _ *http.Request) {
				http.Error(ww, "Not Found", http.StatusNotFound)
			})

			_, err := cl.GetQuote(context.Background(), "INVALID")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("404"))
		})
	})

	Describe("GetTransactions", func() {
		It("returns transactions on success", func() {
			since := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/transactions.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodGet))
				Expect(rr.Header.Get("Authorization")).To(HavePrefix("OAuth "))
				Expect(rr.URL.Query().Get("startDate")).To(Equal("01152024"))

				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"TransactionListResponse": map[string]any{
						"Transaction": []map[string]any{
							{
								"transactionId":   int64(9001),
								"transactionDate": "01162024",
								"amount":          -1500.0,
								"description":     "BOUGHT 10 AAPL",
								"Brokerage": map[string]any{
									"Product": map[string]any{
										"symbol": "AAPL",
									},
									"quantity": 10.0,
									"price":    150.0,
									"fee":      0.0,
								},
							},
						},
					},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			txns, err := cl.GetTransactions(context.Background(), since)
			Expect(err).NotTo(HaveOccurred())
			Expect(txns).To(HaveLen(1))
			Expect(txns[0].TransactionID).To(Equal(int64(9001)))
			Expect(txns[0].Brokerage.Product.Symbol).To(Equal("AAPL"))
		})

		It("returns empty slice when no transactions exist", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/transactions.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"TransactionListResponse": map[string]any{},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			txns, err := cl.GetTransactions(context.Background(), time.Now())
			Expect(err).NotTo(HaveOccurred())
			Expect(txns).To(BeEmpty())
		})
	})

	Describe("PreviewOrder", func() {
		It("returns a preview ID on success", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/preview.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodPost))
				Expect(rr.Header.Get("Authorization")).To(HavePrefix("OAuth "))
				Expect(rr.Header.Get("Content-Type")).To(ContainSubstring("application/json"))

				var body map[string]any
				Expect(sonic.ConfigDefault.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
				Expect(body).To(HaveKey("PreviewOrderRequest"))

				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"PreviewOrderResponse": map[string]any{
						"PreviewIds": []map[string]any{
							{"previewId": int64(777)},
						},
					},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			req := etrade.EtradePreviewRequest{
				OrderType:     "EQ",
				ClientOrderID: "abc123",
			}
			previewID, err := cl.PreviewOrder(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(previewID).To(Equal(int64(777)))
		})

		It("returns an HTTPError on non-2xx response", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/preview.json", func(ww http.ResponseWriter, _ *http.Request) {
				http.Error(ww, "Bad Request", http.StatusBadRequest)
			})

			req := etrade.EtradePreviewRequest{OrderType: "EQ"}
			_, err := cl.PreviewOrder(context.Background(), req)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("400"))
		})
	})

	Describe("PlaceOrder", func() {
		It("returns an order ID on success", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/place.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodPost))
				Expect(rr.Header.Get("Authorization")).To(HavePrefix("OAuth "))

				var body map[string]any
				Expect(sonic.ConfigDefault.NewDecoder(rr.Body).Decode(&body)).To(Succeed())

				placeReq, ok := body["PlaceOrderRequest"].(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(placeReq).To(HaveKey("PreviewIds"))

				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"PlaceOrderResponse": map[string]any{
						"orderId": int64(555),
					},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			req := etrade.EtradePreviewRequest{
				OrderType:     "EQ",
				ClientOrderID: "xyz789",
			}
			orderID, err := cl.PlaceOrder(context.Background(), req, 777)
			Expect(err).NotTo(HaveOccurred())
			Expect(orderID).To(Equal(int64(555)))
		})
	})

	Describe("CancelOrder", func() {
		It("succeeds on a valid order ID", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/cancel.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodPut))
				Expect(rr.Header.Get("Authorization")).To(HavePrefix("OAuth "))

				var body map[string]any
				Expect(sonic.ConfigDefault.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
				cancelReq, ok := body["CancelOrderRequest"].(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(cancelReq["orderId"]).To(BeNumerically("==", 42))

				ww.WriteHeader(http.StatusOK)
			})

			err := cl.CancelOrder(context.Background(), "42")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error for a non-numeric order ID", func() {
			err := cl.CancelOrder(context.Background(), "not-a-number")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse order ID"))
		})

		It("returns an HTTPError on non-2xx response", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/cancel.json", func(ww http.ResponseWriter, _ *http.Request) {
				http.Error(ww, "Not Found", http.StatusNotFound)
			})

			err := cl.CancelOrder(context.Background(), "99")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("404"))
		})
	})

	Describe("OAuth signing", func() {
		It("sets the Authorization header with OAuth prefix on every request", func() {
			var capturedAuthHeader string

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/balance.json", func(ww http.ResponseWriter, rr *http.Request) {
				capturedAuthHeader = rr.Header.Get("Authorization")

				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"BalanceResponse": map[string]any{
						"Computed": map[string]any{
							"RealTimeValues": map[string]any{},
						},
					},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			_, err := cl.GetBalance(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedAuthHeader).To(HavePrefix("OAuth "))
			Expect(capturedAuthHeader).To(ContainSubstring("oauth_consumer_key"))
			Expect(capturedAuthHeader).To(ContainSubstring("oauth_signature"))
		})

		It("uses updated credentials after SetCreds is called", func() {
			newCreds := &etrade.OAuthCredentials{
				ConsumerKey:    "new-key",
				ConsumerSecret: "new-secret",
				AccessToken:    "new-token",
				AccessSecret:   "new-secret-token",
			}

			var capturedAuthHeader string
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/balance.json", func(ww http.ResponseWriter, rr *http.Request) {
				capturedAuthHeader = rr.Header.Get("Authorization")

				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"BalanceResponse": map[string]any{
						"Computed": map[string]any{
							"RealTimeValues": map[string]any{},
						},
					},
				}
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(resp)
			})

			cl.SetCreds(newCreds)

			_, err := cl.GetBalance(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedAuthHeader).To(ContainSubstring("new-key"))
		})
	})
})
