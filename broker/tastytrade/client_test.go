package tastytrade_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/tastytrade"
)

var _ = Describe("apiClient", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// newAuthenticatedClient creates an httptest server with session and accounts
	// handlers, authenticates the client, and returns the client plus a mux
	// that callers can add more routes to. The server is closed in AfterEach
	// via DeferCleanup.
	newAuthenticatedClient := func(extraRoutes func(mux *http.ServeMux)) (*tastytrade.APIClientForTestType, *httptest.Server) {
		mux := http.NewServeMux()

		mux.HandleFunc("POST /sessions", func(writer http.ResponseWriter, req *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"session-token": "test-token-abc",
					"user": map[string]any{
						"external-id": "user-123",
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

		client := tastytrade.NewAPIClientForTest(server.URL)
		Expect(client.Authenticate(ctx, "user@test.com", "secret")).To(Succeed())

		return client, server
	}

	Describe("Authentication", Label("auth"), func() {
		It("stores the session token and account ID after login", func() {
			mux := http.NewServeMux()

			mux.HandleFunc("POST /sessions", func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"data": map[string]any{
						"session-token": "tok-999",
						"user": map[string]any{
							"external-id": "u1",
						},
					},
				})
			})

			mux.HandleFunc("GET /customers/me/accounts", func(writer http.ResponseWriter, req *http.Request) {
				// Verify that the auth token is sent.
				Expect(req.Header.Get("Authorization")).To(Equal("Bearer tok-999"))

				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"data": map[string]any{
						"items": []map[string]any{
							{
								"account": map[string]any{
									"account-number": "MY-ACCT",
								},
							},
						},
					},
				})
			})

			server := httptest.NewServer(mux)
			defer server.Close()

			client := tastytrade.NewAPIClientForTest(server.URL)
			Expect(client.Authenticate(ctx, "user@test.com", "pass")).To(Succeed())
			Expect(client.AccountID()).To(Equal("MY-ACCT"))
		})

		It("returns an error when no accounts are found", func() {
			mux := http.NewServeMux()

			mux.HandleFunc("POST /sessions", func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"data": map[string]any{
						"session-token": "tok-empty",
						"user":          map[string]any{},
					},
				})
			})

			mux.HandleFunc("GET /customers/me/accounts", func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"data": map[string]any{
						"items": []map[string]any{},
					},
				})
			})

			server := httptest.NewServer(mux)
			defer server.Close()

			client := tastytrade.NewAPIClientForTest(server.URL)
			err := client.Authenticate(ctx, "user@test.com", "pass")
			Expect(err).To(MatchError(tastytrade.ErrAccountNotFound))
		})

		It("returns an HTTPError on authentication failure", func() {
			mux := http.NewServeMux()

			mux.HandleFunc("POST /sessions", func(writer http.ResponseWriter, req *http.Request) {
				writer.WriteHeader(http.StatusUnauthorized)
				writer.Write([]byte(`{"error":"invalid credentials"}`))
			})

			server := httptest.NewServer(mux)
			defer server.Close()

			client := tastytrade.NewAPIClientForTest(server.URL)
			err := client.Authenticate(ctx, "bad@test.com", "wrong")
			Expect(err).To(HaveOccurred())

			var httpErr *tastytrade.HTTPError
			Expect(err).To(BeAssignableToTypeOf(httpErr))
		})
	})

	Describe("Orders", Label("orders"), func() {
		It("submits an order and returns the order ID", func() {
			client, _ := newAuthenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /accounts/ACCT-001/orders", func(writer http.ResponseWriter, req *http.Request) {
					var body map[string]any
					json.NewDecoder(req.Body).Decode(&body)
					Expect(body["order-type"]).To(Equal("Limit"))

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"order": map[string]any{
								"id":     "ORD-123",
								"status": "Received",
							},
						},
					})
				})
			})

			order := tastytrade.OrderRequest{
				OrderType:   "Limit",
				TimeInForce: "Day",
				Price:       150.50,
				Legs: []tastytrade.OrderLeg{
					{
						InstrumentType: "Equity",
						Symbol:         "AAPL",
						Action:         "Buy to Open",
						Quantity:       10,
					},
				},
			}

			orderID, err := client.SubmitOrder(ctx, order)
			Expect(err).ToNot(HaveOccurred())
			Expect(orderID).To(Equal("ORD-123"))
		})

		It("cancels an order at the correct URL", func() {
			var cancelPath string

			client, _ := newAuthenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("DELETE /accounts/ACCT-001/orders/ORD-456", func(writer http.ResponseWriter, req *http.Request) {
					cancelPath = req.URL.Path
					writer.WriteHeader(http.StatusOK)
				})
			})

			err := client.CancelOrder(ctx, "ORD-456")
			Expect(err).ToNot(HaveOccurred())
			Expect(cancelPath).To(Equal("/accounts/ACCT-001/orders/ORD-456"))
		})

		It("replaces an order with a PUT to the correct URL", func() {
			var replacePath string
			var replaceBody map[string]any

			client, _ := newAuthenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("PUT /accounts/ACCT-001/orders/ORD-789", func(writer http.ResponseWriter, req *http.Request) {
					replacePath = req.URL.Path
					json.NewDecoder(req.Body).Decode(&replaceBody)
					writer.WriteHeader(http.StatusOK)
				})
			})

			order := tastytrade.OrderRequest{
				OrderType:   "Limit",
				TimeInForce: "Day",
				Price:       155.00,
				Legs: []tastytrade.OrderLeg{
					{
						InstrumentType: "Equity",
						Symbol:         "AAPL",
						Action:         "Buy to Open",
						Quantity:       5,
					},
				},
			}

			err := client.ReplaceOrder(ctx, "ORD-789", order)
			Expect(err).ToNot(HaveOccurred())
			Expect(replacePath).To(Equal("/accounts/ACCT-001/orders/ORD-789"))
			Expect(replaceBody["order-type"]).To(Equal("Limit"))
		})

		It("retrieves and parses an order list", func() {
			client, _ := newAuthenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/ACCT-001/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"items": []map[string]any{
								{
									"id":            "ORD-A",
									"status":        "Live",
									"order-type":    "Market",
									"time-in-force": "Day",
								},
								{
									"id":            "ORD-B",
									"status":        "Filled",
									"order-type":    "Limit",
									"time-in-force": "GTC",
									"price":         100.0,
								},
							},
						},
					})
				})
			})

			orders, err := client.GetOrders(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(2))
			Expect(orders[0].ID).To(Equal("ORD-A"))
			Expect(orders[1].Status).To(Equal("Filled"))
			Expect(orders[1].Price).To(Equal(100.0))
		})

		It("paginates through multiple pages of orders", func() {
			var requestCount atomic.Int32

			client, _ := newAuthenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/ACCT-001/orders", func(writer http.ResponseWriter, req *http.Request) {
					page := requestCount.Add(1)

					Expect(req.URL.Query().Get("per-page")).To(Equal("200"))
					if page == 1 {
						Expect(req.URL.Query().Get("page-offset")).To(Equal("0"))
					} else {
						Expect(req.URL.Query().Get("page-offset")).To(Equal("200"))
					}

					writer.Header().Set("Content-Type", "application/json")

					if page == 1 {
						items := make([]map[string]any, 200)
						for idx := range items {
							items[idx] = map[string]any{
								"id":     fmt.Sprintf("ORD-P1-%d", idx),
								"status": "Live",
							}
						}
						json.NewEncoder(writer).Encode(map[string]any{
							"data": map[string]any{"items": items},
						})
					} else {
						json.NewEncoder(writer).Encode(map[string]any{
							"data": map[string]any{
								"items": []map[string]any{
									{"id": "ORD-P2-0", "status": "Filled"},
								},
							},
						})
					}
				})
			})

			orders, err := client.GetOrders(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(201))
			Expect(requestCount.Load()).To(Equal(int32(2)))
		})
	})

	Describe("Positions", func() {
		It("retrieves and parses positions", func() {
			client, _ := newAuthenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/ACCT-001/positions", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"items": []map[string]any{
								{
									"symbol":                  "AAPL",
									"quantity":                100.0,
									"average-open-price":      150.25,
									"mark-price":              155.00,
									"realized-day-gain-effect": 50.00,
								},
							},
						},
					})
				})
			})

			positions, err := client.GetPositions(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Symbol).To(Equal("AAPL"))
			Expect(positions[0].Quantity).To(Equal(100.0))
			Expect(positions[0].AveragePrice).To(Equal(150.25))
		})
	})

	Describe("Balance", func() {
		It("retrieves and unwraps the balance envelope", func() {
			client, _ := newAuthenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/ACCT-001/balances", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"cash-balance":             25000.50,
							"net-liquidating-value":    50000.00,
							"equity-buying-power":      45000.00,
							"maintenance-requirement":  5000.00,
						},
					})
				})
			})

			balance, err := client.GetBalance(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(Equal(25000.50))
			Expect(balance.NetLiquidatingValue).To(Equal(50000.00))
			Expect(balance.EquityBuyingPower).To(Equal(45000.00))
			Expect(balance.MaintenanceReq).To(Equal(5000.00))
		})
	})

	Describe("Quotes", func() {
		It("retrieves the last price using the by-type endpoint", func() {
			client, _ := newAuthenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /market-data/by-type", func(writer http.ResponseWriter, req *http.Request) {
					Expect(req.URL.Query().Get("equity")).To(Equal("TSLA"))

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"items": []map[string]any{
								{
									"symbol": "TSLA",
									"last":   245.50,
								},
							},
						},
					})
				})
			})

			price, err := client.GetQuote(ctx, "TSLA")
			Expect(err).ToNot(HaveOccurred())
			Expect(price).To(Equal(245.50))
		})
	})

	Describe("Retry", Label("auth"), func() {
		It("retries on 500 and eventually succeeds", func() {
			var requestCount int64

			client, _ := newAuthenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /accounts/ACCT-001/positions", func(writer http.ResponseWriter, req *http.Request) {
					count := atomic.AddInt64(&requestCount, 1)
					if count <= 2 {
						writer.WriteHeader(http.StatusInternalServerError)
						writer.Write([]byte(`{"error":"server error"}`))
						return
					}

					writer.Header().Set("Content-Type", "application/json")
					json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"items": []map[string]any{
								{
									"symbol":   "GOOG",
									"quantity": 50.0,
								},
							},
						},
					})
				})
			})

			positions, err := client.GetPositions(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Symbol).To(Equal("GOOG"))
			Expect(atomic.LoadInt64(&requestCount)).To(BeNumerically(">=", 3))
		})
	})
})
