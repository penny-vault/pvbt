package etrade_test

import (
	"context"
	"errors"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/etrade"
)

// Compile-time interface check: EtradeBroker must implement broker.Broker.
var _ broker.Broker = (*etrade.EtradeBroker)(nil)

var _ = Describe("EtradeBroker", func() {
	Describe("New", func() {
		It("creates a broker with default settings", func() {
			eb := etrade.New()
			Expect(eb).NotTo(BeNil())
			Expect(eb.Fills()).NotTo(BeNil())
			Expect(etrade.IsSandbox(eb)).To(BeFalse())
		})

		It("applies WithSandbox option", func() {
			eb := etrade.New(etrade.WithSandbox())
			Expect(etrade.IsSandbox(eb)).To(BeTrue())
		})

		It("applies WithTokenFile option without panicking", func() {
			eb := etrade.New(etrade.WithTokenFile("/tmp/test-tokens.json"))
			Expect(eb).NotTo(BeNil())
		})

		It("applies WithCallbackURL option without panicking", func() {
			eb := etrade.New(etrade.WithCallbackURL("https://example.com/callback"))
			Expect(eb).NotTo(BeNil())
		})

		It("returns a fills channel", func() {
			eb := etrade.New()
			Expect(eb.Fills()).NotTo(BeNil())
		})
	})

	Describe("Connect", func() {
		It("returns ErrMissingCredentials when ETRADE_CONSUMER_KEY is not set", func() {
			GinkgoT().Setenv("ETRADE_CONSUMER_KEY", "")
			GinkgoT().Setenv("ETRADE_CONSUMER_SECRET", "secret")
			GinkgoT().Setenv("ETRADE_ACCOUNT_ID_KEY", "acct123")

			eb := etrade.New()
			err := eb.Connect(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, broker.ErrMissingCredentials)).To(BeTrue())
		})

		It("returns ErrMissingCredentials when ETRADE_CONSUMER_SECRET is not set", func() {
			GinkgoT().Setenv("ETRADE_CONSUMER_KEY", "key")
			GinkgoT().Setenv("ETRADE_CONSUMER_SECRET", "")
			GinkgoT().Setenv("ETRADE_ACCOUNT_ID_KEY", "acct123")

			eb := etrade.New()
			err := eb.Connect(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, broker.ErrMissingCredentials)).To(BeTrue())
		})

		It("returns ErrMissingCredentials when ETRADE_ACCOUNT_ID_KEY is not set", func() {
			GinkgoT().Setenv("ETRADE_CONSUMER_KEY", "key")
			GinkgoT().Setenv("ETRADE_CONSUMER_SECRET", "secret")
			GinkgoT().Setenv("ETRADE_ACCOUNT_ID_KEY", "")

			eb := etrade.New()
			err := eb.Connect(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, broker.ErrMissingCredentials)).To(BeTrue())
		})
	})

	Describe("Close", func() {
		It("does not panic when auth and poller are nil", func() {
			eb := etrade.New()
			Expect(func() { _ = eb.Close() }).NotTo(Panic())
		})
	})

	Describe("Submit", func() {
		var (
			mux    *http.ServeMux
			server *httptest.Server
			eb     *etrade.EtradeBroker
		)

		BeforeEach(func() {
			mux = http.NewServeMux()
			server = httptest.NewServer(mux)
			creds := testCreds()
			cl := etrade.NewAPIClientForTest(server.URL, creds, testAccountIDKey)
			eb = etrade.New()
			etrade.SetClientForTest(eb, cl)
		})

		AfterEach(func() {
			server.Close()
		})

		It("calls preview then place for a market order", func() {
			previewCalled := false
			placeCalled := false

			// Positions endpoint: empty portfolio so detectAction returns BUY.
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/portfolio.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"PortfolioResponse": map[string]any{
						"AccountPortfolio": []any{},
					},
				})
			})

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/preview.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodPost))
				previewCalled = true
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"PreviewOrderResponse": map[string]any{
						"PreviewIds": []map[string]any{
							{"previewId": int64(100)},
						},
					},
				})
			})

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/place.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodPost))
				placeCalled = true
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"PlaceOrderResponse": map[string]any{
						"orderId": int64(200),
					},
				})
			})

			order := broker.Order{
				Asset:     asset.Asset{Ticker: "AAPL"},
				Side:      broker.Buy,
				Qty:       10,
				OrderType: broker.Market,
			}

			err := eb.Submit(context.Background(), order)
			Expect(err).NotTo(HaveOccurred())
			Expect(previewCalled).To(BeTrue())
			Expect(placeCalled).To(BeTrue())
		})

		It("fetches a quote and computes qty for a dollar-amount order", func() {
			quoteCalled := false

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/portfolio.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"PortfolioResponse": map[string]any{
						"AccountPortfolio": []any{},
					},
				})
			})

			mux.HandleFunc("/v1/market/quote/MSFT.json", func(ww http.ResponseWriter, _ *http.Request) {
				quoteCalled = true
				ww.Header().Set("Content-Type", "application/json")
				// $200 price -> $1000 / $200 = 5 shares
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"QuoteResponse": map[string]any{
						"QuoteData": []map[string]any{
							{"All": map[string]any{"lastTrade": 200.0}},
						},
					},
				})
			})

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/preview.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"PreviewOrderResponse": map[string]any{
						"PreviewIds": []map[string]any{
							{"previewId": int64(101)},
						},
					},
				})
			})

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/place.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"PlaceOrderResponse": map[string]any{
						"orderId": int64(201),
					},
				})
			})

			order := broker.Order{
				Asset:     asset.Asset{Ticker: "MSFT"},
				Side:      broker.Buy,
				Qty:       0,
				Amount:    1000.0,
				OrderType: broker.Market,
			}

			err := eb.Submit(context.Background(), order)
			Expect(err).NotTo(HaveOccurred())
			Expect(quoteCalled).To(BeTrue())
		})

		It("returns an error when a dollar-amount order results in zero shares", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/portfolio.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"PortfolioResponse": map[string]any{
						"AccountPortfolio": []any{},
					},
				})
			})

			mux.HandleFunc("/v1/market/quote/GOOG.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				// $5000 price -> $10 / $5000 = 0 shares
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"QuoteResponse": map[string]any{
						"QuoteData": []map[string]any{
							{"All": map[string]any{"lastTrade": 5000.0}},
						},
					},
				})
			})

			order := broker.Order{
				Asset:     asset.Asset{Ticker: "GOOG"},
				Side:      broker.Buy,
				Qty:       0,
				Amount:    10.0,
				OrderType: broker.Market,
			}

			err := eb.Submit(context.Background(), order)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("zero shares"))
		})
	})

	Describe("Cancel", func() {
		var (
			mux    *http.ServeMux
			server *httptest.Server
			eb     *etrade.EtradeBroker
		)

		BeforeEach(func() {
			mux = http.NewServeMux()
			server = httptest.NewServer(mux)
			creds := testCreds()
			cl := etrade.NewAPIClientForTest(server.URL, creds, testAccountIDKey)
			eb = etrade.New()
			etrade.SetClientForTest(eb, cl)
		})

		AfterEach(func() {
			server.Close()
		})

		It("calls cancelOrder on the client", func() {
			cancelCalled := false

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/cancel.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodPut))
				var body map[string]any
				Expect(sonic.ConfigDefault.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
				cancelReq, ok := body["CancelOrderRequest"].(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(cancelReq["orderId"]).To(BeNumerically("==", 42))
				cancelCalled = true
				ww.WriteHeader(http.StatusOK)
			})

			err := eb.Cancel(context.Background(), "42")
			Expect(err).NotTo(HaveOccurred())
			Expect(cancelCalled).To(BeTrue())
		})
	})

	Describe("Replace", func() {
		var (
			mux    *http.ServeMux
			server *httptest.Server
			eb     *etrade.EtradeBroker
		)

		BeforeEach(func() {
			mux = http.NewServeMux()
			server = httptest.NewServer(mux)
			creds := testCreds()
			cl := etrade.NewAPIClientForTest(server.URL, creds, testAccountIDKey)
			eb = etrade.New()
			etrade.SetClientForTest(eb, cl)
		})

		AfterEach(func() {
			server.Close()
		})

		It("calls preview-modify then place-modify", func() {
			previewCalled := false
			placeCalled := false

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/55/change/preview.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodPut))
				previewCalled = true
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"PreviewOrderResponse": map[string]any{
						"PreviewIds": []map[string]any{
							{"previewId": int64(102)},
						},
					},
				})
			})

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders/55/change/place.json", func(ww http.ResponseWriter, rr *http.Request) {
				Expect(rr.Method).To(Equal(http.MethodPut))
				placeCalled = true
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"PlaceOrderResponse": map[string]any{
						"orderId": int64(202),
					},
				})
			})

			order := broker.Order{
				Asset:      asset.Asset{Ticker: "AAPL"},
				Side:       broker.Buy,
				Qty:        15,
				OrderType:  broker.Limit,
				LimitPrice: 148.0,
			}

			err := eb.Replace(context.Background(), "55", order)
			Expect(err).NotTo(HaveOccurred())
			Expect(previewCalled).To(BeTrue())
			Expect(placeCalled).To(BeTrue())
		})
	})

	Describe("Orders", func() {
		var (
			mux    *http.ServeMux
			server *httptest.Server
			eb     *etrade.EtradeBroker
		)

		BeforeEach(func() {
			mux = http.NewServeMux()
			server = httptest.NewServer(mux)
			creds := testCreds()
			cl := etrade.NewAPIClientForTest(server.URL, creds, testAccountIDKey)
			eb = etrade.New()
			etrade.SetClientForTest(eb, cl)
		})

		AfterEach(func() {
			server.Close()
		})

		It("returns translated broker orders", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/orders.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"OrdersResponse": map[string]any{
						"Order": []map[string]any{
							{
								"orderId":     int64(10),
								"orderType":   "EQ",
								"orderStatus": "OPEN",
								"OrderDetail": []map[string]any{
									{
										"priceType": "MARKET",
										"orderTerm": "GOOD_FOR_DAY",
										"Instrument": []map[string]any{
											{
												"Product":         map[string]any{"symbol": "TSLA", "securityType": "EQ"},
												"orderAction":     "BUY",
												"orderedQuantity": 3.0,
											},
										},
									},
								},
							},
						},
					},
				})
			})

			orders, err := eb.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].Asset.Ticker).To(Equal("TSLA"))
			Expect(orders[0].Status).To(Equal(broker.OrderOpen))
		})
	})

	Describe("Positions", func() {
		var (
			mux    *http.ServeMux
			server *httptest.Server
			eb     *etrade.EtradeBroker
		)

		BeforeEach(func() {
			mux = http.NewServeMux()
			server = httptest.NewServer(mux)
			creds := testCreds()
			cl := etrade.NewAPIClientForTest(server.URL, creds, testAccountIDKey)
			eb = etrade.New()
			etrade.SetClientForTest(eb, cl)
		})

		AfterEach(func() {
			server.Close()
		})

		It("returns translated broker positions with mark price from quote", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/portfolio.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"PortfolioResponse": map[string]any{
						"AccountPortfolio": []map[string]any{
							{
								"Position": []map[string]any{
									{
										"quantity":     5.0,
										"positionType": "LONG",
										"costPerShare": 100.0,
										"marketValue":  510.0,
										"Product":      map[string]any{"symbol": "NVDA", "securityType": "EQ"},
									},
								},
							},
						},
					},
				})
			})

			mux.HandleFunc("/v1/market/quote/NVDA.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"QuoteResponse": map[string]any{
						"QuoteData": []map[string]any{
							{"All": map[string]any{"lastTrade": 105.0}},
						},
					},
				})
			})

			positions, err := eb.Positions(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Asset.Ticker).To(Equal("NVDA"))
			Expect(positions[0].Qty).To(Equal(5.0))
			Expect(positions[0].MarkPrice).To(Equal(105.0))
		})
	})

	Describe("Balance", func() {
		var (
			mux    *http.ServeMux
			server *httptest.Server
			eb     *etrade.EtradeBroker
		)

		BeforeEach(func() {
			mux = http.NewServeMux()
			server = httptest.NewServer(mux)
			creds := testCreds()
			cl := etrade.NewAPIClientForTest(server.URL, creds, testAccountIDKey)
			eb = etrade.New()
			etrade.SetClientForTest(eb, cl)
		})

		AfterEach(func() {
			server.Close()
		})

		It("returns translated broker balance", func() {
			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/balance.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"BalanceResponse": map[string]any{
						"accountType": "MARGIN",
						"Computed": map[string]any{
							"cashAvailableForInvestment": 30000.0,
							"cashBuyingPower":            30000.0,
							"marginBuyingPower":          60000.0,
							"reqMaintenanceValue":        5000.0,
							"RealTimeValues": map[string]any{
								"totalAccountValue": 120000.0,
							},
						},
					},
				})
			})

			bal, err := eb.Balance(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(bal.CashBalance).To(Equal(30000.0))
			Expect(bal.NetLiquidatingValue).To(Equal(120000.0))
			Expect(bal.EquityBuyingPower).To(Equal(60000.0))
			Expect(bal.MaintenanceReq).To(Equal(5000.0))
		})
	})

	Describe("Transactions", func() {
		var (
			mux    *http.ServeMux
			server *httptest.Server
			eb     *etrade.EtradeBroker
		)

		BeforeEach(func() {
			mux = http.NewServeMux()
			server = httptest.NewServer(mux)
			creds := testCreds()
			cl := etrade.NewAPIClientForTest(server.URL, creds, testAccountIDKey)
			eb = etrade.New()
			etrade.SetClientForTest(eb, cl)
		})

		AfterEach(func() {
			server.Close()
		})

		It("returns translated broker transactions", func() {
			since := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)

			mux.HandleFunc("/v1/accounts/"+testAccountIDKey+"/transactions.json", func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				_ = sonic.ConfigDefault.NewEncoder(ww).Encode(map[string]any{
					"TransactionListResponse": map[string]any{
						"Transaction": []map[string]any{
							{
								"transactionId":   int64(5000),
								"transactionDate": "03022024",
								"amount":          -800.0,
								"description":     "BOUGHT 4 AMZN",
								"Brokerage": map[string]any{
									"Product":  map[string]any{"symbol": "AMZN"},
									"quantity": 4.0,
									"price":    200.0,
									"fee":      0.0,
								},
							},
						},
					},
				})
			})

			txns, err := eb.Transactions(context.Background(), since)
			Expect(err).NotTo(HaveOccurred())
			Expect(txns).To(HaveLen(1))
			Expect(txns[0].Asset.Ticker).To(Equal("AMZN"))
			Expect(txns[0].Qty).To(Equal(4.0))
		})
	})

	Describe("detectAction", func() {
		It("returns BUY when buying with no existing position", func() {
			positions := []etrade.EtradePosition{}
			action := etrade.DetectAction(broker.Buy, "AAPL", positions)
			Expect(action).To(Equal("BUY"))
		})

		It("returns BUY_TO_COVER when buying and a short position exists", func() {
			pos := etrade.EtradePosition{Quantity: -5}
			pos.Product.Symbol = "AAPL"
			action := etrade.DetectAction(broker.Buy, "AAPL", []etrade.EtradePosition{pos})
			Expect(action).To(Equal("BUY_TO_COVER"))
		})

		It("returns SELL when selling with an existing long position", func() {
			pos := etrade.EtradePosition{Quantity: 10}
			pos.Product.Symbol = "AAPL"
			action := etrade.DetectAction(broker.Sell, "AAPL", []etrade.EtradePosition{pos})
			Expect(action).To(Equal("SELL"))
		})

		It("returns SELL_SHORT when selling with no existing position", func() {
			positions := []etrade.EtradePosition{}
			action := etrade.DetectAction(broker.Sell, "AAPL", positions)
			Expect(action).To(Equal("SELL_SHORT"))
		})

		It("returns SELL_SHORT when selling and position quantity is zero", func() {
			pos := etrade.EtradePosition{Quantity: 0}
			pos.Product.Symbol = "AAPL"
			action := etrade.DetectAction(broker.Sell, "AAPL", []etrade.EtradePosition{pos})
			Expect(action).To(Equal("SELL_SHORT"))
		})
	})
})
