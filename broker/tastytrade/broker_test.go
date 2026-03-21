package tastytrade_test

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
	"github.com/penny-vault/pvbt/broker/tastytrade"
)

// Compile-time interface check.
var _ broker.Broker = (*tastytrade.TastytradeBroker)(nil)

var _ = Describe("TastytradeBroker", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// authenticatedBroker builds a TastytradeBroker backed by an httptest.Server.
	// The server has /sessions and /customers/me/accounts handlers pre-registered,
	// plus any extra routes the caller supplies. The broker's internal client is
	// pointed at the server and authenticated.
	authenticatedBroker := func(extraRoutes func(mux *http.ServeMux)) *tastytrade.TastytradeBroker {
		mux := http.NewServeMux()

		mux.HandleFunc("POST /sessions", func(writer http.ResponseWriter, req *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"session-token": "test-token-broker",
					"user": map[string]any{
						"external-id": "user-broker",
					},
				},
			})
		})

		mux.HandleFunc("GET /customers/me/accounts", func(writer http.ResponseWriter, req *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{
						{
							"account": map[string]any{
								"account-number": "ACCT-001",
							},
						},
					},
				},
			})
		})

		if extraRoutes != nil {
			extraRoutes(mux)
		}

		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		ttBroker := tastytrade.New()
		tastytrade.SetClientBaseURLForTest(ttBroker, server.URL)
		tastytrade.AuthenticateClientForTest(ttBroker, ctx)

		return ttBroker
	}

	Describe("Constructor and options", func() {
		It("creates a broker with a non-nil fills channel", func() {
			ttBroker := tastytrade.New()
			Expect(ttBroker.Fills()).ToNot(BeNil())
		})

		It("creates a broker with sandbox mode", func() {
			ttBroker := tastytrade.New(tastytrade.WithSandbox())
			Expect(ttBroker).ToNot(BeNil())
			Expect(ttBroker.Fills()).ToNot(BeNil())
		})
	})

	Describe("Connect", Label("auth"), func() {
		It("returns ErrMissingCredentials when env vars are not set", func() {
			originalUser := os.Getenv("TASTYTRADE_USERNAME")
			originalPass := os.Getenv("TASTYTRADE_PASSWORD")
			os.Unsetenv("TASTYTRADE_USERNAME")
			os.Unsetenv("TASTYTRADE_PASSWORD")
			DeferCleanup(func() {
				if originalUser != "" {
					os.Setenv("TASTYTRADE_USERNAME", originalUser)
				}
				if originalPass != "" {
					os.Setenv("TASTYTRADE_PASSWORD", originalPass)
				}
			})

			ttBroker := tastytrade.New()
			err := ttBroker.Connect(ctx)
			Expect(err).To(MatchError(tastytrade.ErrMissingCredentials))
		})
	})

	Describe("Submit", Label("orders"), func() {
		It("submits a qty-based order", func() {
			var submitCalled atomic.Int32
			var receivedBody map[string]any

			ttBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /accounts/ACCT-001/orders", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)
					json.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"order": map[string]any{
								"id":     "ORD-QTY-1",
								"status": "Received",
							},
						},
					})
				})
			})

			err := ttBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         25,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(submitCalled.Load()).To(Equal(int32(1)))

			legs, ok := receivedBody["legs"].([]any)
			Expect(ok).To(BeTrue())
			Expect(legs).To(HaveLen(1))

			firstLeg := legs[0].(map[string]any)
			Expect(firstLeg["symbol"]).To(Equal("AAPL"))
			Expect(firstLeg["quantity"]).To(BeNumerically("==", 25))
		})

		It("converts dollar-amount orders to share quantity", func() {
			var submittedQty float64

			ttBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /market-data/TSLA/quotes", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"items": []map[string]any{
								{
									"symbol": "TSLA",
									"last":   100.0,
								},
							},
						},
					})
				})

				mux.HandleFunc("POST /accounts/ACCT-001/orders", func(writer http.ResponseWriter, req *http.Request) {
					var body map[string]any
					json.NewDecoder(req.Body).Decode(&body)

					legs := body["legs"].([]any)
					firstLeg := legs[0].(map[string]any)
					submittedQty = firstLeg["quantity"].(float64)

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"order": map[string]any{
								"id":     "ORD-AMT-1",
								"status": "Received",
							},
						},
					})
				})
			})

			err := ttBroker.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "TSLA"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      5000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(submittedQty).To(Equal(50.0)) // floor(5000 / 100) = 50
		})

		It("returns nil without submitting when dollar amount yields zero shares", func() {
			var submitCalled atomic.Int32

			ttBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /market-data/BRK.A/quotes", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"items": []map[string]any{
								{
									"symbol": "BRK.A",
									"last":   100.0,
								},
							},
						},
					})
				})

				mux.HandleFunc("POST /accounts/ACCT-001/orders", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)
					writer.WriteHeader(http.StatusOK)
				})
			})

			err := ttBroker.Submit(ctx, broker.Order{
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
		It("delegates cancellation to the client", func() {
			var cancelCalled atomic.Int32

			ttBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("DELETE /accounts/ACCT-001/orders/ORD-CANCEL-1", func(writer http.ResponseWriter, req *http.Request) {
					cancelCalled.Add(1)
					writer.WriteHeader(http.StatusOK)
				})
			})

			err := ttBroker.Cancel(ctx, "ORD-CANCEL-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(cancelCalled.Load()).To(Equal(int32(1)))
		})
	})

	Describe("Replace", Label("orders"), func() {
		It("delegates replacement to the client", func() {
			var replaceCalled atomic.Int32

			ttBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("PUT /accounts/ACCT-001/orders/ORD-REPLACE-1", func(writer http.ResponseWriter, req *http.Request) {
					replaceCalled.Add(1)
					writer.WriteHeader(http.StatusOK)
				})
			})

			err := ttBroker.Replace(ctx, "ORD-REPLACE-1", broker.Order{
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Limit,
				LimitPrice:  400.0,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(replaceCalled.Load()).To(Equal(int32(1)))
		})
	})

	Describe("Orders", func() {
		It("retrieves orders through the broker", func() {
			ttBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/ACCT-001/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"items": []map[string]any{
								{
									"id":            "ORD-LIST-1",
									"status":        "Live",
									"order-type":    "Market",
									"time-in-force": "Day",
									"legs": []map[string]any{
										{
											"symbol":          "GOOG",
											"instrument-type": "Equity",
											"action":          "Buy to Open",
											"quantity":        15,
										},
									},
								},
							},
						},
					})
				})
			})

			orders, err := ttBroker.Orders(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].ID).To(Equal("ORD-LIST-1"))
			Expect(orders[0].Asset.Ticker).To(Equal("GOOG"))
			Expect(orders[0].Qty).To(Equal(15.0))
			Expect(orders[0].Status).To(Equal(broker.OrderOpen))
		})
	})

	Describe("Positions", func() {
		It("retrieves positions through the broker", func() {
			ttBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/ACCT-001/positions", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"items": []map[string]any{
								{
									"symbol":                   "NVDA",
									"quantity":                  200.0,
									"average-open-price":        450.0,
									"mark-price":                475.0,
									"realized-day-gain-effect":  1250.0,
								},
							},
						},
					})
				})
			})

			positions, err := ttBroker.Positions(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Asset.Ticker).To(Equal("NVDA"))
			Expect(positions[0].Qty).To(Equal(200.0))
			Expect(positions[0].AvgOpenPrice).To(Equal(450.0))
			Expect(positions[0].MarkPrice).To(Equal(475.0))
		})
	})

	Describe("Balance", func() {
		It("retrieves balance through the broker", func() {
			ttBroker := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/ACCT-001/balances", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"cash-balance":            30000.0,
							"net-liquidating-value":   75000.0,
							"equity-buying-power":     60000.0,
							"maintenance-requirement": 10000.0,
						},
					})
				})
			})

			balance, err := ttBroker.Balance(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(Equal(30000.0))
			Expect(balance.NetLiquidatingValue).To(Equal(75000.0))
			Expect(balance.EquityBuyingPower).To(Equal(60000.0))
			Expect(balance.MaintenanceReq).To(Equal(10000.0))
		})
	})

	Describe("Close", func() {
		It("closes without error when no streamer is connected", func() {
			ttBroker := tastytrade.New()
			err := ttBroker.Close()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
