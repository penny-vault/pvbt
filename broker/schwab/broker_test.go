package schwab_test

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
	"github.com/penny-vault/pvbt/broker/schwab"
)

// Compile-time interface checks.
var _ broker.Broker = (*schwab.SchwabBroker)(nil)
var _ broker.GroupSubmitter = (*schwab.SchwabBroker)(nil)

var _ = Describe("SchwabBroker", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	authenticatedBroker := func(extraRoutes func(mux *http.ServeMux)) *schwab.SchwabBroker {
		mux := http.NewServeMux()

		if extraRoutes != nil {
			extraRoutes(mux)
		}

		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		schwabBroker := schwab.New()
		client := schwab.NewAPIClientForTest(server.URL, "test-token")
		client.SetAccountHash("HASH-TEST")
		schwab.SetClientForTest(schwabBroker, client)
		schwab.SetAccountHashForTest(schwabBroker, "HASH-TEST")

		return schwabBroker
	}

	Describe("Constructor and options", func() {
		It("creates a broker with a non-nil fills channel", func() {
			schwabBroker := schwab.New()
			Expect(schwabBroker.Fills()).ToNot(BeNil())
		})

		It("applies WithTokenFile option", func() {
			schwabBroker := schwab.New(schwab.WithTokenFile("/custom/path/tokens.json"))
			Expect(schwabBroker).ToNot(BeNil())
		})

		It("applies WithCallbackURL option", func() {
			schwabBroker := schwab.New(schwab.WithCallbackURL("https://127.0.0.1:9999"))
			Expect(schwabBroker).ToNot(BeNil())
		})
	})

	Describe("Connect", Label("auth"), func() {
		It("returns ErrMissingCredentials when SCHWAB_CLIENT_ID is not set", func() {
			originalID := os.Getenv("SCHWAB_CLIENT_ID")
			originalSecret := os.Getenv("SCHWAB_CLIENT_SECRET")
			os.Unsetenv("SCHWAB_CLIENT_ID")
			os.Unsetenv("SCHWAB_CLIENT_SECRET")
			DeferCleanup(func() {
				if originalID != "" {
					os.Setenv("SCHWAB_CLIENT_ID", originalID)
				}
				if originalSecret != "" {
					os.Setenv("SCHWAB_CLIENT_SECRET", originalSecret)
				}
			})

			schwabBroker := schwab.New()
			connectErr := schwabBroker.Connect(ctx)
			Expect(connectErr).To(MatchError(broker.ErrMissingCredentials))
		})
	})

	Describe("Submit", Label("orders"), func() {
		It("submits a qty-based order", func() {
			var submitCalled atomic.Int32
			var receivedBody map[string]any

			schwabBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /trader/v1/accounts/HASH-TEST/orders", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)
					json.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Location", "/v1/accounts/HASH-TEST/orders/ORD-QTY-1")
					writer.WriteHeader(http.StatusCreated)
				})
			})

			submitErr := schwabBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         25,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(submitCalled.Load()).To(Equal(int32(1)))
			Expect(receivedBody["orderType"]).To(Equal("MARKET"))

			legs, ok := receivedBody["orderLegCollection"].([]any)
			Expect(ok).To(BeTrue())
			Expect(legs).To(HaveLen(1))

			firstLeg := legs[0].(map[string]any)
			inst := firstLeg["instrument"].(map[string]any)
			Expect(inst["symbol"]).To(Equal("AAPL"))
			Expect(firstLeg["quantity"]).To(BeNumerically("==", 25))
		})

		It("converts dollar-amount orders to share quantity", func() {
			var submittedQty float64

			schwabBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /marketdata/v1/TSLA/quotes", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"TSLA": map[string]any{
							"quote": map[string]any{
								"lastPrice": 100.0,
							},
						},
					})
				})

				mux.HandleFunc("POST /trader/v1/accounts/HASH-TEST/orders", func(writer http.ResponseWriter, req *http.Request) {
					var body map[string]any
					json.NewDecoder(req.Body).Decode(&body)

					legs := body["orderLegCollection"].([]any)
					firstLeg := legs[0].(map[string]any)
					submittedQty = firstLeg["quantity"].(float64)

					writer.Header().Set("Location", "/v1/accounts/HASH-TEST/orders/ORD-AMT-1")
					writer.WriteHeader(http.StatusCreated)
				})
			})

			submitErr := schwabBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "TSLA"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      5000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(submittedQty).To(Equal(50.0)) // floor(5000 / 100) = 50
		})

		It("returns nil without submitting when dollar amount yields zero shares", func() {
			var submitCalled atomic.Int32

			schwabBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /marketdata/v1/BRK.A/quotes", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"BRK.A": map[string]any{
							"quote": map[string]any{
								"lastPrice": 100.0,
							},
						},
					})
				})

				mux.HandleFunc("POST /trader/v1/accounts/HASH-TEST/orders", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)
					writer.WriteHeader(http.StatusCreated)
				})
			})

			submitErr := schwabBroker.Submit(ctx, broker.Order{
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
		It("delegates cancellation to the client", func() {
			var cancelCalled atomic.Int32

			schwabBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("DELETE /trader/v1/accounts/HASH-TEST/orders/ORD-CANCEL-1", func(writer http.ResponseWriter, req *http.Request) {
					cancelCalled.Add(1)
					writer.WriteHeader(http.StatusOK)
				})
			})

			cancelErr := schwabBroker.Cancel(ctx, "ORD-CANCEL-1")
			Expect(cancelErr).ToNot(HaveOccurred())
			Expect(cancelCalled.Load()).To(Equal(int32(1)))
		})
	})

	Describe("Replace", Label("orders"), func() {
		It("delegates replacement to the client", func() {
			var replaceCalled atomic.Int32

			schwabBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("PUT /trader/v1/accounts/HASH-TEST/orders/ORD-REPLACE-1", func(writer http.ResponseWriter, req *http.Request) {
					replaceCalled.Add(1)
					writer.Header().Set("Location", "/v1/accounts/HASH-TEST/orders/ORD-REPLACE-NEW")
					writer.WriteHeader(http.StatusCreated)
				})
			})

			replaceErr := schwabBroker.Replace(ctx, "ORD-REPLACE-1", broker.Order{
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Limit,
				LimitPrice:  400.0,
				TimeInForce: broker.Day,
			})

			Expect(replaceErr).ToNot(HaveOccurred())
			Expect(replaceCalled.Load()).To(Equal(int32(1)))
		})
	})

	Describe("Orders", func() {
		It("retrieves orders through the broker", func() {
			schwabBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /trader/v1/accounts/HASH-TEST/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode([]map[string]any{
						{
							"orderId":           123,
							"status":            "WORKING",
							"orderType":         "MARKET",
							"duration":          "DAY",
							"orderStrategyType": "SINGLE",
							"orderLegCollection": []map[string]any{
								{
									"instruction": "BUY",
									"quantity":    15,
									"instrument":  map[string]any{"symbol": "GOOG", "assetType": "EQUITY"},
								},
							},
						},
					})
				})
			})

			orders, getErr := schwabBroker.Orders(ctx)
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
			schwabBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /trader/v1/accounts/HASH-TEST", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"securitiesAccount": map[string]any{
							"positions": []map[string]any{
								{
									"instrument":           map[string]any{"symbol": "NVDA", "assetType": "EQUITY"},
									"longQuantity":         200.0,
									"shortQuantity":        0.0,
									"averagePrice":         450.0,
									"marketValue":          95000.0,
									"currentDayProfitLoss": 1250.0,
								},
							},
						},
					})
				})
			})

			positions, getErr := schwabBroker.Positions(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Asset.Ticker).To(Equal("NVDA"))
			Expect(positions[0].Qty).To(Equal(200.0))
			Expect(positions[0].AvgOpenPrice).To(Equal(450.0))
		})
	})

	Describe("Balance", func() {
		It("retrieves balance through the broker", func() {
			schwabBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /trader/v1/accounts/HASH-TEST", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"securitiesAccount": map[string]any{
							"accountNumber": "12345678",
							"type":          "MARGIN",
							"currentBalances": map[string]any{
								"cashBalance":            30000.0,
								"equity":                 75000.0,
								"buyingPower":            60000.0,
								"maintenanceRequirement": 10000.0,
							},
						},
					})
				})
			})

			balance, getErr := schwabBroker.Balance(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(Equal(30000.0))
			Expect(balance.NetLiquidatingValue).To(Equal(75000.0))
			Expect(balance.EquityBuyingPower).To(Equal(60000.0))
			Expect(balance.MaintenanceReq).To(Equal(10000.0))
		})
	})

	Describe("SubmitGroup", Label("orders"), func() {
		It("submits a bracket order with TRIGGER + nested OCO", func() {
			var receivedBody map[string]any

			schwabBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /trader/v1/accounts/HASH-TEST/orders", func(writer http.ResponseWriter, req *http.Request) {
					json.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Location", "/v1/accounts/HASH-TEST/orders/BRACKET-1")
					writer.WriteHeader(http.StatusCreated)
				})
			})

			submitErr := schwabBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Stop, StopPrice: 430.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}, broker.GroupBracket)

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(receivedBody["orderStrategyType"]).To(Equal("TRIGGER"))
			Expect(receivedBody["childOrderStrategies"]).ToNot(BeNil())
		})

		It("submits an OCO order", func() {
			var receivedBody map[string]any

			schwabBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /trader/v1/accounts/HASH-TEST/orders", func(writer http.ResponseWriter, req *http.Request) {
					json.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Location", "/v1/accounts/HASH-TEST/orders/OCO-1")
					writer.WriteHeader(http.StatusCreated)
				})
			})

			submitErr := schwabBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Limit, LimitPrice: 150.0, TimeInForce: broker.Day},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC},
			}, broker.GroupOCO)

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(receivedBody["orderStrategyType"]).To(Equal("OCO"))
		})

		It("returns ErrEmptyOrderGroup for empty slice", func() {
			schwabBroker := authenticatedBroker(nil)
			submitErr := schwabBroker.SubmitGroup(ctx, []broker.Order{}, broker.GroupOCO)
			Expect(submitErr).To(MatchError(broker.ErrEmptyOrderGroup))
		})

		It("returns ErrNoEntryOrder when bracket has no entry", func() {
			schwabBroker := authenticatedBroker(nil)
			submitErr := schwabBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}, broker.GroupBracket)
			Expect(submitErr).To(MatchError(broker.ErrNoEntryOrder))
		})

		It("returns ErrMultipleEntryOrders when bracket has two entries", func() {
			schwabBroker := authenticatedBroker(nil)
			submitErr := schwabBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
			}, broker.GroupBracket)
			Expect(submitErr).To(MatchError(broker.ErrMultipleEntryOrders))
		})
	})

	Describe("Close", func() {
		It("closes without error when no streamer is connected", func() {
			schwabBroker := schwab.New()
			closeErr := schwabBroker.Close()
			Expect(closeErr).ToNot(HaveOccurred())
		})
	})
})
