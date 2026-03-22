package alpaca_test

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
	"github.com/penny-vault/pvbt/broker/alpaca"
)

// Compile-time interface checks.
var _ broker.Broker = (*alpaca.AlpacaBroker)(nil)
var _ broker.GroupSubmitter = (*alpaca.AlpacaBroker)(nil)

var _ = Describe("AlpacaBroker", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// authenticatedBroker builds an AlpacaBroker backed by an httptest.Server.
	// The server has a GET /v2/account handler pre-registered, plus any extra
	// routes the caller supplies. The broker's internal client is pointed at the server.
	authenticatedBroker := func(extraRoutes func(mux *http.ServeMux)) *alpaca.AlpacaBroker {
		mux := http.NewServeMux()

		mux.HandleFunc("GET /v2/account", func(writer http.ResponseWriter, req *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			json.NewEncoder(writer).Encode(map[string]any{
				"id":                 "acct-001",
				"status":            "ACTIVE",
				"cash":              "25000",
				"equity":            "50000",
				"buying_power":      "45000",
				"maintenance_margin": "5000",
			})
		})

		if extraRoutes != nil {
			extraRoutes(mux)
		}

		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		alpacaBroker := alpaca.New()
		alpaca.SetClientForTest(alpacaBroker, server.URL, "test-key", "test-secret")

		return alpacaBroker
	}

	Describe("Constructor and options", func() {
		It("creates a broker with a non-nil fills channel", func() {
			alpacaBroker := alpaca.New()
			Expect(alpacaBroker.Fills()).ToNot(BeNil())
		})

		It("creates a broker with paper mode", func() {
			alpacaBroker := alpaca.New(alpaca.WithPaper())
			Expect(alpacaBroker).ToNot(BeNil())
			Expect(alpacaBroker.Fills()).ToNot(BeNil())
		})

		It("creates a broker with fractional shares", func() {
			alpacaBroker := alpaca.New(alpaca.WithFractionalShares())
			Expect(alpacaBroker).ToNot(BeNil())
			Expect(alpacaBroker.Fills()).ToNot(BeNil())
		})
	})

	Describe("Connect", Label("auth"), func() {
		It("returns ErrMissingCredentials when env vars are not set", func() {
			originalKey := os.Getenv("ALPACA_API_KEY")
			originalSecret := os.Getenv("ALPACA_API_SECRET")
			os.Unsetenv("ALPACA_API_KEY")
			os.Unsetenv("ALPACA_API_SECRET")
			DeferCleanup(func() {
				if originalKey != "" {
					os.Setenv("ALPACA_API_KEY", originalKey)
				}
				if originalSecret != "" {
					os.Setenv("ALPACA_API_SECRET", originalSecret)
				}
			})

			alpacaBroker := alpaca.New()
			err := alpacaBroker.Connect(ctx)
			Expect(err).To(MatchError(alpaca.ErrMissingCredentials))
		})

		It("returns ErrAccountNotActive when account status is not ACTIVE", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /v2/account", func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"id":     "acct-001",
					"status": "SUSPENDED",
					"cash":   "0",
					"equity": "0",
				})
			})

			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			originalKey := os.Getenv("ALPACA_API_KEY")
			originalSecret := os.Getenv("ALPACA_API_SECRET")
			os.Setenv("ALPACA_API_KEY", "test-key")
			os.Setenv("ALPACA_API_SECRET", "test-secret")
			DeferCleanup(func() {
				if originalKey != "" {
					os.Setenv("ALPACA_API_KEY", originalKey)
				} else {
					os.Unsetenv("ALPACA_API_KEY")
				}
				if originalSecret != "" {
					os.Setenv("ALPACA_API_SECRET", originalSecret)
				} else {
					os.Unsetenv("ALPACA_API_SECRET")
				}
			})

			alpacaBroker := alpaca.New()
			alpaca.SetClientForTest(alpacaBroker, server.URL, "test-key", "test-secret")

			err := alpacaBroker.Connect(ctx)
			Expect(err).To(MatchError(alpaca.ErrAccountNotActive))
		})
	})

	Describe("Submit", Label("orders"), func() {
		It("submits a qty-based order", func() {
			var submitCalled atomic.Int32
			var receivedBody map[string]any

			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)
					json.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"id":     "ORD-QTY-1",
						"status": "new",
					})
				})
			})

			err := alpacaBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         25,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(submitCalled.Load()).To(Equal(int32(1)))
			Expect(receivedBody["symbol"]).To(Equal("AAPL"))
			Expect(receivedBody["side"]).To(Equal("buy"))
			Expect(receivedBody["type"]).To(Equal("market"))
			Expect(receivedBody["qty"]).To(Equal("25"))
		})

		It("converts dollar-amount orders to share quantity when not fractional", func() {
			var submittedQty string

			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/stocks/TSLA/trades/latest", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"trade": map[string]any{
							"p": "100",
						},
					})
				})

				mux.HandleFunc("POST /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					var body map[string]any
					json.NewDecoder(req.Body).Decode(&body)
					submittedQty = body["qty"].(string)
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"id":     "ORD-AMT-1",
						"status": "new",
					})
				})
			})

			err := alpacaBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "TSLA"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      5000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(submittedQty).To(Equal("50")) // floor(5000 / 100) = 50
		})

		It("sends notional when fractional shares are enabled", func() {
			var receivedBody map[string]any

			mux := http.NewServeMux()
			mux.HandleFunc("GET /v2/account", func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"id": "acct-001", "status": "ACTIVE",
					"cash": "25000", "equity": "50000",
				})
			})
			mux.HandleFunc("POST /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
				json.NewDecoder(req.Body).Decode(&receivedBody)
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"id":     "ORD-FRAC-1",
					"status": "new",
				})
			})

			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			alpacaBroker := alpaca.New(alpaca.WithFractionalShares())
			alpaca.SetClientForTest(alpacaBroker, server.URL, "test-key", "test-secret")

			err := alpacaBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      1500.50,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(receivedBody["notional"]).To(Equal("1500.5"))
			Expect(receivedBody["qty"]).To(BeNil())
		})

		It("returns nil without submitting when dollar amount yields zero shares", func() {
			var submitCalled atomic.Int32

			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/stocks/BRK.A/trades/latest", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"trade": map[string]any{
							"p": "100",
						},
					})
				})

				mux.HandleFunc("POST /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)
					writer.WriteHeader(http.StatusOK)
				})
			})

			err := alpacaBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "BRK.A"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      50, // floor(50 / 100) = 0
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(submitCalled.Load()).To(Equal(int32(0)))
		})
	})

	Describe("Cancel", Label("orders"), func() {
		It("sends DELETE to the correct path", func() {
			var cancelCalled atomic.Int32
			var deletedPath string

			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("DELETE /v2/orders/ORD-CANCEL-1", func(writer http.ResponseWriter, req *http.Request) {
					cancelCalled.Add(1)
					deletedPath = req.URL.Path
					writer.WriteHeader(http.StatusNoContent)
				})
			})

			err := alpacaBroker.Cancel(ctx, "ORD-CANCEL-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(cancelCalled.Load()).To(Equal(int32(1)))
			Expect(deletedPath).To(Equal("/v2/orders/ORD-CANCEL-1"))
		})
	})

	Describe("Replace", Label("orders"), func() {
		It("uses PATCH when only mutable fields changed", func() {
			var patchCalled atomic.Int32
			var patchBody map[string]any

			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"id":     "ORD-ORIG-1",
						"status": "new",
					})
				})
				mux.HandleFunc("PATCH /v2/orders/ORD-ORIG-1", func(writer http.ResponseWriter, req *http.Request) {
					patchCalled.Add(1)
					json.NewDecoder(req.Body).Decode(&patchBody)
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"id":     "ORD-REPLACED-1",
						"status": "new",
					})
				})
			})

			// First, submit the original order so it's tracked.
			originalOrder := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Limit,
				LimitPrice:  150.0,
				TimeInForce: broker.Day,
			}
			err := alpacaBroker.Submit(ctx, originalOrder)
			Expect(err).ToNot(HaveOccurred())

			// Replace with changed qty and limit price.
			replacementOrder := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         20,
				OrderType:   broker.Limit,
				LimitPrice:  155.0,
				TimeInForce: broker.Day,
			}
			err = alpacaBroker.Replace(ctx, "ORD-ORIG-1", replacementOrder)
			Expect(err).ToNot(HaveOccurred())
			Expect(patchCalled.Load()).To(Equal(int32(1)))
			Expect(patchBody["qty"]).To(Equal("20"))
			Expect(patchBody["limit_price"]).To(Equal("155"))
		})

		It("cancels and resubmits when non-mutable fields changed", func() {
			var deleteCalled atomic.Int32
			var postCalled atomic.Int32

			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					postCalled.Add(1)
					writer.Header().Set("Content-Type", "application/json")
					orderID := "ORD-ORIG-2"
					if postCalled.Load() > 1 {
						orderID = "ORD-NEW-2"
					}
					json.NewEncoder(writer).Encode(map[string]any{
						"id":     orderID,
						"status": "new",
					})
				})
				mux.HandleFunc("DELETE /v2/orders/ORD-ORIG-2", func(writer http.ResponseWriter, req *http.Request) {
					deleteCalled.Add(1)
					writer.WriteHeader(http.StatusNoContent)
				})
			})

			// Submit original order.
			originalOrder := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			err := alpacaBroker.Submit(ctx, originalOrder)
			Expect(err).ToNot(HaveOccurred())

			// Replace with a different ticker (non-mutable).
			replacementOrder := broker.Order{
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			err = alpacaBroker.Replace(ctx, "ORD-ORIG-2", replacementOrder)
			Expect(err).ToNot(HaveOccurred())
			Expect(deleteCalled.Load()).To(Equal(int32(1)))
			Expect(postCalled.Load()).To(Equal(int32(2))) // original submit + resubmit
		})

		It("returns error when cancel returns HTTP 422", func() {
			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"id":     "ORD-422-1",
						"status": "new",
					})
				})
				mux.HandleFunc("DELETE /v2/orders/ORD-422-1", func(writer http.ResponseWriter, req *http.Request) {
					writer.WriteHeader(http.StatusUnprocessableEntity)
					writer.Write([]byte(`{"message":"order not cancelable"}`))
				})
			})

			// Submit original.
			err := alpacaBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})
			Expect(err).ToNot(HaveOccurred())

			// Replace with non-mutable change triggers cancel path.
			err = alpacaBroker.Replace(ctx, "ORD-422-1", broker.Order{
				Asset:       asset.Asset{Ticker: "GOOG"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("422"))
		})
	})

	Describe("Orders", func() {
		It("retrieves and maps orders", func() {
			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode([]map[string]any{
						{
							"id":           "ORD-LIST-1",
							"status":       "new",
							"type":         "limit",
							"side":         "buy",
							"symbol":       "GOOG",
							"qty":          "15",
							"limit_price":  "140.50",
							"time_in_force": "day",
						},
					})
				})
			})

			orders, err := alpacaBroker.Orders(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].ID).To(Equal("ORD-LIST-1"))
			Expect(orders[0].Asset.Ticker).To(Equal("GOOG"))
			Expect(orders[0].Qty).To(Equal(15.0))
			Expect(orders[0].Status).To(Equal(broker.OrderSubmitted))
			Expect(orders[0].OrderType).To(Equal(broker.Limit))
			Expect(orders[0].LimitPrice).To(Equal(140.5))
		})
	})

	Describe("Positions", func() {
		It("retrieves and maps positions", func() {
			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /v2/positions", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode([]map[string]any{
						{
							"symbol":                "NVDA",
							"qty":                   "200",
							"avg_entry_price":       "450",
							"current_price":         "475",
							"unrealized_intraday_pl": "1250",
						},
					})
				})
			})

			positions, err := alpacaBroker.Positions(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Asset.Ticker).To(Equal("NVDA"))
			Expect(positions[0].Qty).To(Equal(200.0))
			Expect(positions[0].AvgOpenPrice).To(Equal(450.0))
			Expect(positions[0].MarkPrice).To(Equal(475.0))
		})
	})

	Describe("Balance", func() {
		It("retrieves account and maps to balance", func() {
			alpacaBroker := authenticatedBroker(nil)

			balance, err := alpacaBroker.Balance(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(Equal(25000.0))
			Expect(balance.NetLiquidatingValue).To(Equal(50000.0))
			Expect(balance.EquityBuyingPower).To(Equal(45000.0))
			Expect(balance.MaintenanceReq).To(Equal(5000.0))
		})
	})

	Describe("SubmitGroup", Label("orders"), func() {
		It("submits a bracket order with correct order_class and legs", func() {
			var receivedBody map[string]any

			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					json.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"id":     "ORD-BRACKET-1",
						"status": "new",
					})
				})
			})

			err := alpacaBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Stop, StopPrice: 430.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}, broker.GroupBracket)

			Expect(err).ToNot(HaveOccurred())
			Expect(receivedBody["order_class"]).To(Equal("bracket"))
			Expect(receivedBody["symbol"]).To(Equal("SPY"))
			Expect(receivedBody["side"]).To(Equal("buy"))

			takeProfit, hasTakeProfit := receivedBody["take_profit"].(map[string]any)
			Expect(hasTakeProfit).To(BeTrue())
			Expect(takeProfit["limit_price"]).To(Equal("460"))

			stopLoss, hasStopLoss := receivedBody["stop_loss"].(map[string]any)
			Expect(hasStopLoss).To(BeTrue())
			Expect(stopLoss["stop_price"]).To(Equal("430"))
		})

		It("submits an OCO order with correct order_class", func() {
			var receivedBody map[string]any

			alpacaBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /v2/orders", func(writer http.ResponseWriter, req *http.Request) {
					json.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"id":     "ORD-OCO-1",
						"status": "new",
					})
				})
			})

			err := alpacaBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140.0, TimeInForce: broker.GTC},
			}, broker.GroupOCO)

			Expect(err).ToNot(HaveOccurred())
			Expect(receivedBody["order_class"]).To(Equal("oco"))

			stopLoss, hasStopLoss := receivedBody["stop_loss"].(map[string]any)
			Expect(hasStopLoss).To(BeTrue())
			Expect(stopLoss["stop_price"]).To(Equal("140"))
		})

		It("returns ErrEmptyOrderGroup for empty slice", func() {
			alpacaBroker := authenticatedBroker(nil)
			err := alpacaBroker.SubmitGroup(ctx, []broker.Order{}, broker.GroupOCO)
			Expect(err).To(MatchError(alpaca.ErrEmptyOrderGroup))
		})

		It("returns ErrNoEntryOrder when bracket has no entry", func() {
			alpacaBroker := authenticatedBroker(nil)
			err := alpacaBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}, broker.GroupBracket)
			Expect(err).To(MatchError(alpaca.ErrNoEntryOrder))
		})

		It("returns ErrMultipleEntryOrders when bracket has two entries", func() {
			alpacaBroker := authenticatedBroker(nil)
			err := alpacaBroker.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
			}, broker.GroupBracket)
			Expect(err).To(MatchError(alpaca.ErrMultipleEntryOrders))
		})
	})

	Describe("Close", func() {
		It("closes without error when no streamer is connected", func() {
			alpacaBroker := alpaca.New()
			err := alpacaBroker.Close()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
