package tradier_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/tradier"
)

// Compile-time interface checks.
var _ broker.Broker = (*tradier.TradierBroker)(nil)
var _ broker.GroupSubmitter = (*tradier.TradierBroker)(nil)

var _ = Describe("TradierBroker", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// authenticatedBroker builds a TradierBroker backed by an httptest.Server.
	// Extra routes can be registered by the caller.
	authenticatedBroker := func(extraRoutes func(mux *http.ServeMux)) *tradier.TradierBroker {
		mux := http.NewServeMux()
		if extraRoutes != nil {
			extraRoutes(mux)
		}

		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		tb := tradier.New()
		client := tradier.NewAPIClientForTest(server.URL, "test-token", "TEST-ACCT")
		tradier.SetClientForTest(tb, client)

		return tb
	}

	// noPositionsHandler returns a handler that responds with an empty positions list.
	noPositionsHandler := func(writer http.ResponseWriter, req *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"positions":{"position":null}}`)
	}

	Describe("Connect", Label("auth"), func() {
		It("returns ErrMissingCredentials when both auth env vars are missing", func() {
			originalToken := os.Getenv("TRADIER_ACCESS_TOKEN")
			originalClientID := os.Getenv("TRADIER_CLIENT_ID")
			originalClientSecret := os.Getenv("TRADIER_CLIENT_SECRET")
			os.Unsetenv("TRADIER_ACCESS_TOKEN")
			os.Unsetenv("TRADIER_CLIENT_ID")
			os.Unsetenv("TRADIER_CLIENT_SECRET")
			DeferCleanup(func() {
				if originalToken != "" {
					os.Setenv("TRADIER_ACCESS_TOKEN", originalToken)
				}
				if originalClientID != "" {
					os.Setenv("TRADIER_CLIENT_ID", originalClientID)
				}
				if originalClientSecret != "" {
					os.Setenv("TRADIER_CLIENT_SECRET", originalClientSecret)
				}
			})

			tb := tradier.New()
			err := tb.Connect(ctx)
			Expect(err).To(MatchError(tradier.ErrMissingCredentials))
		})

		It("returns ErrMissingCredentials when TRADIER_ACCOUNT_ID is missing", func() {
			originalToken := os.Getenv("TRADIER_ACCESS_TOKEN")
			originalAccountID := os.Getenv("TRADIER_ACCOUNT_ID")
			os.Setenv("TRADIER_ACCESS_TOKEN", "test-token")
			os.Unsetenv("TRADIER_ACCOUNT_ID")
			DeferCleanup(func() {
				if originalToken != "" {
					os.Setenv("TRADIER_ACCESS_TOKEN", originalToken)
				} else {
					os.Unsetenv("TRADIER_ACCESS_TOKEN")
				}
				if originalAccountID != "" {
					os.Setenv("TRADIER_ACCOUNT_ID", originalAccountID)
				}
			})

			tb := tradier.New()
			err := tb.Connect(ctx)
			Expect(err).To(MatchError(tradier.ErrMissingCredentials))
		})
	})

	Describe("Close", func() {
		It("closes without error and closes the fills channel", func() {
			tb := authenticatedBroker(nil)

			err := tb.Close()
			Expect(err).ToNot(HaveOccurred())

			// Fills channel should be closed.
			fills := tb.Fills()
			_, open := <-fills
			Expect(open).To(BeFalse())
		})
	})

	Describe("Submit", Label("orders"), func() {
		It("submits a market buy with correct form params", func() {
			var submitCalled atomic.Int32
			var receivedForm url.Values

			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/TEST-ACCT/positions", noPositionsHandler)

				mux.HandleFunc("POST /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)
					if parseErr := req.ParseForm(); parseErr == nil {
						receivedForm = req.PostForm
					}

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{
							"id":     12345,
							"status": "ok",
						},
					})
				})
			})

			err := tb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(submitCalled.Load()).To(Equal(int32(1)))
			Expect(receivedForm.Get("symbol")).To(Equal("AAPL"))
			Expect(receivedForm.Get("side")).To(Equal("buy"))
			Expect(receivedForm.Get("type")).To(Equal("market"))
			Expect(receivedForm.Get("quantity")).To(Equal("10"))
			Expect(receivedForm.Get("class")).To(Equal("equity"))
		})

		It("fetches a quote and computes quantity for dollar-amount orders", func() {
			var quoteCalled atomic.Int32
			var submittedQty string

			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /markets/quotes", func(writer http.ResponseWriter, req *http.Request) {
					quoteCalled.Add(1)
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"quotes": map[string]any{
							"quote": map[string]any{
								"last": 200.0,
							},
						},
					})
				})

				mux.HandleFunc("GET /accounts/TEST-ACCT/positions", noPositionsHandler)

				mux.HandleFunc("POST /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					if parseErr := req.ParseForm(); parseErr == nil {
						submittedQty = req.PostForm.Get("quantity")
					}

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{"id": 1, "status": "ok"},
					})
				})
			})

			err := tb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "TSLA"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      1000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(quoteCalled.Load()).To(Equal(int32(1)))
			Expect(submittedQty).To(Equal("5")) // floor(1000/200) = 5
		})

		It("returns an error when dollar-amount order quantity rounds to zero", func() {
			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /markets/quotes", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"quotes": map[string]any{
							"quote": map[string]any{
								"last": 50000.0,
							},
						},
					})
				})

				mux.HandleFunc("GET /accounts/TEST-ACCT/positions", noPositionsHandler)
			})

			err := tb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "BRK.A"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("zero"))
		})

		It("returns an error for unsupported TIF", func() {
			tb := authenticatedBroker(nil)

			err := tb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.IOC,
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("IOC"))
		})

		It("uses sell_short side when no long position exists", func() {
			var receivedSide string

			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/TEST-ACCT/positions", noPositionsHandler)

				mux.HandleFunc("POST /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					if parseErr := req.ParseForm(); parseErr == nil {
						receivedSide = req.PostForm.Get("side")
					}

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{"id": 1, "status": "ok"},
					})
				})
			})

			err := tb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Sell,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(receivedSide).To(Equal("sell_short"))
		})

		It("uses buy_to_cover side when a short position exists", func() {
			var receivedSide string

			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/TEST-ACCT/positions", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"positions": map[string]any{
							"position": map[string]any{
								"id":            1,
								"symbol":        "AAPL",
								"quantity":      -10.0,
								"cost_basis":    -1500.0,
								"date_acquired": "2026-01-01T00:00:00.000Z",
							},
						},
					})
				})

				mux.HandleFunc("POST /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					if parseErr := req.ParseForm(); parseErr == nil {
						receivedSide = req.PostForm.Get("side")
					}

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{"id": 1, "status": "ok"},
					})
				})
			})

			err := tb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(receivedSide).To(Equal("buy_to_cover"))
		})
	})

	Describe("Cancel", Label("orders"), func() {
		It("sends DELETE to the correct path", func() {
			var cancelCalled atomic.Int32
			var deletedPath string

			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("DELETE /accounts/TEST-ACCT/orders/ORD-99", func(writer http.ResponseWriter, req *http.Request) {
					cancelCalled.Add(1)
					deletedPath = req.URL.Path

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{"id": 99, "status": "ok"},
					})
				})
			})

			err := tb.Cancel(ctx, "ORD-99")
			Expect(err).ToNot(HaveOccurred())
			Expect(cancelCalled.Load()).To(Equal(int32(1)))
			Expect(deletedPath).To(Equal("/accounts/TEST-ACCT/orders/ORD-99"))
		})
	})

	Describe("Replace", Label("orders"), func() {
		It("modifies price and type via PUT when quantity is unchanged", func() {
			var putCalled atomic.Int32
			var putForm url.Values

			// Seed the existing order so Replace can look it up.
			existingOrder := map[string]any{
				"id":       int64(100),
				"type":     "limit",
				"symbol":   "AAPL",
				"side":     "buy",
				"quantity": 10.0,
				"status":   "open",
				"duration": "day",
				"price":    150.0,
			}

			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"orders": map[string]any{
							"order": existingOrder,
						},
					})
				})

				mux.HandleFunc("PUT /accounts/TEST-ACCT/orders/100", func(writer http.ResponseWriter, req *http.Request) {
					putCalled.Add(1)
					if parseErr := req.ParseForm(); parseErr == nil {
						putForm = req.PostForm
					}

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{"id": 100, "status": "ok"},
					})
				})
			})

			err := tb.Replace(ctx, "100", broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Limit,
				LimitPrice:  160.0,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(putCalled.Load()).To(Equal(int32(1)))
			Expect(putForm.Get("price")).To(Equal("160"))
		})

		It("cancels and resubmits when quantity changes", func() {
			var deleteCalled atomic.Int32
			var postCalled atomic.Int32

			existingOrder := map[string]any{
				"id":       int64(200),
				"type":     "limit",
				"symbol":   "AAPL",
				"side":     "buy",
				"quantity": 10.0,
				"status":   "open",
				"duration": "day",
				"price":    150.0,
			}

			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"orders": map[string]any{
							"order": existingOrder,
						},
					})
				})

				mux.HandleFunc("DELETE /accounts/TEST-ACCT/orders/200", func(writer http.ResponseWriter, req *http.Request) {
					deleteCalled.Add(1)

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{"id": 200, "status": "ok"},
					})
				})

				mux.HandleFunc("GET /accounts/TEST-ACCT/positions", noPositionsHandler)

				mux.HandleFunc("POST /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					postCalled.Add(1)

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{"id": 201, "status": "ok"},
					})
				})
			})

			err := tb.Replace(ctx, "200", broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         20,
				OrderType:   broker.Limit,
				LimitPrice:  150.0,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(deleteCalled.Load()).To(Equal(int32(1)))
			Expect(postCalled.Load()).To(Equal(int32(1)))
		})
	})

	Describe("Orders", func() {
		It("retrieves and maps orders", func() {
			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"orders": map[string]any{
							"order": map[string]any{
								"id":       int64(300),
								"type":     "limit",
								"symbol":   "GOOG",
								"side":     "buy",
								"quantity": 5.0,
								"status":   "open",
								"duration": "day",
								"price":    140.0,
							},
						},
					})
				})
			})

			orders, err := tb.Orders(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].ID).To(Equal("300"))
			Expect(orders[0].Asset.Ticker).To(Equal("GOOG"))
			Expect(orders[0].Qty).To(Equal(5.0))
			Expect(orders[0].OrderType).To(Equal(broker.Limit))
			Expect(orders[0].LimitPrice).To(Equal(140.0))
		})
	})

	Describe("Positions", func() {
		It("retrieves positions and fetches MarkPrice via quote", func() {
			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/TEST-ACCT/positions", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"positions": map[string]any{
							"position": map[string]any{
								"id":            1,
								"symbol":        "NVDA",
								"quantity":      100.0,
								"cost_basis":    40000.0,
								"date_acquired": "2026-01-01T00:00:00.000Z",
							},
						},
					})
				})

				mux.HandleFunc("GET /markets/quotes", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"quotes": map[string]any{
							"quote": map[string]any{
								"last": 500.0,
							},
						},
					})
				})
			})

			positions, err := tb.Positions(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Asset.Ticker).To(Equal("NVDA"))
			Expect(positions[0].Qty).To(Equal(100.0))
			Expect(positions[0].AvgOpenPrice).To(Equal(400.0))
			Expect(positions[0].MarkPrice).To(Equal(500.0))
		})
	})

	Describe("Balance", func() {
		It("retrieves a margin account balance", func() {
			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/TEST-ACCT/balances", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"balances": map[string]any{
							"account_number": "TEST-ACCT",
							"account_type":   "margin",
							"total_equity":   100000.0,
							"total_cash":     25000.0,
							"market_value":   75000.0,
							"margin": map[string]any{
								"stock_buying_power":  50000.0,
								"current_requirement": 10000.0,
							},
						},
					})
				})
			})

			balance, err := tb.Balance(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(Equal(25000.0))
			Expect(balance.NetLiquidatingValue).To(Equal(100000.0))
			Expect(balance.EquityBuyingPower).To(Equal(50000.0))
			Expect(balance.MaintenanceReq).To(Equal(10000.0))
		})

		It("retrieves a cash account balance", func() {
			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/TEST-ACCT/balances", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"balances": map[string]any{
							"account_number": "TEST-ACCT",
							"account_type":   "cash",
							"total_equity":   30000.0,
							"total_cash":     30000.0,
							"market_value":   0.0,
							"cash": map[string]any{
								"cash_available":  28000.0,
								"unsettled_funds": 2000.0,
							},
						},
					})
				})
			})

			balance, err := tb.Balance(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(Equal(28000.0))
			Expect(balance.EquityBuyingPower).To(Equal(28000.0))
			Expect(balance.MaintenanceReq).To(Equal(0.0))
		})
	})

	Describe("SubmitGroup", Label("orders"), func() {
		It("submits OCO with class=oco and indexed params", func() {
			var receivedForm url.Values

			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					if parseErr := req.ParseForm(); parseErr == nil {
						receivedForm = req.PostForm
					}

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{"id": 1, "status": "ok"},
					})
				})
			})

			err := tb.SubmitGroup(ctx, []broker.Order{
				{
					Asset:       asset.Asset{Ticker: "AAPL"},
					Side:        broker.Sell,
					Qty:         100,
					OrderType:   broker.Limit,
					LimitPrice:  155.0,
					TimeInForce: broker.Day,
				},
				{
					Asset:       asset.Asset{Ticker: "AAPL"},
					Side:        broker.Sell,
					Qty:         100,
					OrderType:   broker.Stop,
					StopPrice:   145.0,
					TimeInForce: broker.Day,
				},
			}, broker.GroupOCO)

			Expect(err).ToNot(HaveOccurred())
			Expect(receivedForm.Get("class")).To(Equal("oco"))
			Expect(receivedForm.Get("symbol[0]")).To(Equal("AAPL"))
			Expect(receivedForm.Get("side[0]")).To(Equal("sell"))
			Expect(receivedForm.Get("quantity[0]")).To(Equal("100"))
			Expect(receivedForm.Get("type[0]")).To(Equal("limit"))
			Expect(receivedForm.Get("price[0]")).To(Equal("155"))
			Expect(receivedForm.Get("symbol[1]")).To(Equal("AAPL"))
			Expect(receivedForm.Get("side[1]")).To(Equal("sell"))
			Expect(receivedForm.Get("type[1]")).To(Equal("stop"))
			Expect(receivedForm.Get("stop[1]")).To(Equal("145"))
		})

		It("submits bracket with class=otoco and indexed params", func() {
			var receivedForm url.Values

			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					if parseErr := req.ParseForm(); parseErr == nil {
						receivedForm = req.PostForm
					}

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{"id": 1, "status": "ok"},
					})
				})
			})

			err := tb.SubmitGroup(ctx, []broker.Order{
				{
					Asset:       asset.Asset{Ticker: "AAPL"},
					Side:        broker.Buy,
					Qty:         100,
					OrderType:   broker.Limit,
					LimitPrice:  150.0,
					TimeInForce: broker.Day,
					GroupRole:   broker.RoleEntry,
				},
				{
					Asset:       asset.Asset{Ticker: "AAPL"},
					Side:        broker.Sell,
					Qty:         100,
					OrderType:   broker.Limit,
					LimitPrice:  155.0,
					TimeInForce: broker.Day,
					GroupRole:   broker.RoleTakeProfit,
				},
				{
					Asset:       asset.Asset{Ticker: "AAPL"},
					Side:        broker.Sell,
					Qty:         100,
					OrderType:   broker.Stop,
					StopPrice:   145.0,
					TimeInForce: broker.Day,
					GroupRole:   broker.RoleStopLoss,
				},
			}, broker.GroupBracket)

			Expect(err).ToNot(HaveOccurred())
			Expect(receivedForm.Get("class")).To(Equal("otoco"))
			// leg 0 = entry
			Expect(receivedForm.Get("symbol[0]")).To(Equal("AAPL"))
			Expect(receivedForm.Get("side[0]")).To(Equal("buy"))
			Expect(receivedForm.Get("type[0]")).To(Equal("limit"))
			Expect(receivedForm.Get("price[0]")).To(Equal("150"))
			// leg 1 = take-profit
			Expect(receivedForm.Get("side[1]")).To(Equal("sell"))
			Expect(receivedForm.Get("type[1]")).To(Equal("limit"))
			Expect(receivedForm.Get("price[1]")).To(Equal("155"))
			// leg 2 = stop-loss
			Expect(receivedForm.Get("side[2]")).To(Equal("sell"))
			Expect(receivedForm.Get("type[2]")).To(Equal("stop"))
			Expect(receivedForm.Get("stop[2]")).To(Equal("145"))
		})

		It("returns ErrEmptyOrderGroup for an empty slice", func() {
			tb := authenticatedBroker(nil)
			err := tb.SubmitGroup(ctx, []broker.Order{}, broker.GroupOCO)
			Expect(err).To(MatchError(tradier.ErrEmptyOrderGroup))
		})

		It("returns ErrNoEntryOrder when bracket has no entry", func() {
			tb := authenticatedBroker(nil)
			err := tb.SubmitGroup(ctx, []broker.Order{
				{
					Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10,
					OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC,
					GroupRole: broker.RoleTakeProfit,
				},
				{
					Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10,
					OrderType: broker.Stop, StopPrice: 140.0, TimeInForce: broker.GTC,
					GroupRole: broker.RoleStopLoss,
				},
			}, broker.GroupBracket)
			Expect(err).To(MatchError(tradier.ErrNoEntryOrder))
		})

		It("returns ErrMultipleEntryOrders when bracket has two entry legs", func() {
			tb := authenticatedBroker(nil)
			err := tb.SubmitGroup(ctx, []broker.Order{
				{
					Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10,
					OrderType: broker.Market, TimeInForce: broker.Day,
					GroupRole: broker.RoleEntry,
				},
				{
					Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10,
					OrderType: broker.Market, TimeInForce: broker.Day,
					GroupRole: broker.RoleEntry,
				},
			}, broker.GroupBracket)
			Expect(err).To(MatchError(tradier.ErrMultipleEntryOrders))
		})
	})

	Describe("Fills", func() {
		It("returns a non-nil channel", func() {
			tb := tradier.New()
			Expect(tb.Fills()).ToNot(BeNil())
		})
	})

	// Verify that limit and stop prices are formatted without trailing zeros.
	Describe("OCO price formatting", func() {
		It("formats limit and stop prices without trailing zeros", func() {
			var receivedBody string

			tb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					bodyBytes := make([]byte, 4096)
					nn, _ := req.Body.Read(bodyBytes)
					receivedBody = string(bodyBytes[:nn])

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"order": map[string]any{"id": 1, "status": "ok"},
					})
				})
			})

			_ = tb.SubmitGroup(ctx, []broker.Order{
				{
					Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10,
					OrderType: broker.Limit, LimitPrice: 155.0, TimeInForce: broker.Day,
				},
				{
					Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10,
					OrderType: broker.Stop, StopPrice: 145.0, TimeInForce: broker.Day,
				},
			}, broker.GroupOCO)

			// URL-encoded bracket notation: price%5B0%5D=155 (price[0]=155)
			Expect(strings.Contains(receivedBody, "price%5B0%5D=155")).To(BeTrue())
		})
	})
})
