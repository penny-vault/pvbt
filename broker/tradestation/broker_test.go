package tradestation_test

import (
	"context"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"time"

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
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
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
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"Quotes": []map[string]any{
							{"Symbol": "TSLA", "Last": 100.0},
						},
					})
				})

				mux.HandleFunc("POST /v3/orderexecution/orders", func(writer http.ResponseWriter, req *http.Request) {
					var body map[string]any
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&body)
					submittedQty = body["Quantity"].(string)

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
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
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
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
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
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
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
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
					sonic.ConfigDefault.NewEncoder(writer).Encode([]map[string]any{
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
					sonic.ConfigDefault.NewEncoder(writer).Encode([]map[string]any{
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
			transactions, getErr := tsBroker.Transactions(ctx, time.Time{})
			Expect(getErr).ToNot(HaveOccurred())
			Expect(transactions).To(BeEmpty())
		})
	})

	Describe("SubmitGroup", Label("orders"), func() {
		It("submits an OCO group order", func() {
			var receivedBody map[string]any

			tsBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v3/orderexecution/ordergroups", func(writer http.ResponseWriter, req *http.Request) {
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
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
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
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
