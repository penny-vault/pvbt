package tradier_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/tradier"
)

var _ = Describe("apiClient", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// authenticatedClient sets up a test server with the given routes and returns
	// an apiClient pointed at it.
	authenticatedClient := func(extraRoutes func(mux *http.ServeMux)) *tradier.APIClientForTest {
		mux := http.NewServeMux()
		if extraRoutes != nil {
			extraRoutes(mux)
		}
		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)
		return tradier.NewAPIClientForTest(server.URL, "test-token", "TEST-ACCT")
	}

	Describe("submitOrder", func() {
		It("POSTs to the correct path with form-encoded body and returns the order ID", func() {
			var capturedMethod, capturedPath, capturedBody, capturedAuth string

			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					capturedMethod = req.Method
					capturedPath = req.URL.Path
					capturedAuth = req.Header.Get("Authorization")
					if parseErr := req.ParseForm(); parseErr == nil {
						capturedBody = req.Form.Encode()
					}

					writer.Header().Set("Content-Type", "application/json")
					writer.WriteHeader(http.StatusOK)
					fmt.Fprint(writer, `{"order":{"id":98765,"status":"ok"}}`)
				})
			})

			params := url.Values{}
			params.Set("class", "equity")
			params.Set("symbol", "AAPL")
			params.Set("side", "buy")
			params.Set("quantity", "10")
			params.Set("type", "market")
			params.Set("duration", "day")

			orderID, submitErr := client.SubmitOrder(ctx, params)
			Expect(submitErr).ToNot(HaveOccurred())
			Expect(orderID).To(Equal("98765"))
			Expect(capturedMethod).To(Equal("POST"))
			Expect(capturedPath).To(Equal("/accounts/TEST-ACCT/orders"))
			Expect(capturedAuth).To(Equal("Bearer test-token"))
			Expect(capturedBody).To(ContainSubstring("symbol=AAPL"))
		})

		It("returns a wrapped HTTPError on HTTP 400", func() {
			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(writer, `{"fault":{"description":"invalid order"}}`)
				})
			})

			params := url.Values{}
			params.Set("class", "equity")

			_, submitErr := client.SubmitOrder(ctx, params)
			Expect(submitErr).To(HaveOccurred())

			var httpErr *tradier.HTTPError
			Expect(submitErr).To(BeAssignableToTypeOf(httpErr))

			httpErr, _ = submitErr.(*tradier.HTTPError)
			Expect(httpErr.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("cancelOrder", func() {
		It("sends DELETE to the correct path and returns nil on success", func() {
			var capturedMethod, capturedPath string

			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/orders/12345", func(writer http.ResponseWriter, req *http.Request) {
					capturedMethod = req.Method
					capturedPath = req.URL.Path
					writer.WriteHeader(http.StatusOK)
					fmt.Fprint(writer, `{"order":{"id":12345,"status":"canceled"}}`)
				})
			})

			cancelErr := client.CancelOrder(ctx, "12345")
			Expect(cancelErr).ToNot(HaveOccurred())
			Expect(capturedMethod).To(Equal("DELETE"))
			Expect(capturedPath).To(Equal("/accounts/TEST-ACCT/orders/12345"))
		})

		It("returns an HTTPError when the server responds with 404", func() {
			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/orders/MISSING", func(writer http.ResponseWriter, req *http.Request) {
					writer.WriteHeader(http.StatusNotFound)
					fmt.Fprint(writer, `{"fault":{"description":"order not found"}}`)
				})
			})

			cancelErr := client.CancelOrder(ctx, "MISSING")
			Expect(cancelErr).To(HaveOccurred())

			httpErr, ok := cancelErr.(*tradier.HTTPError)
			Expect(ok).To(BeTrue())
			Expect(httpErr.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("modifyOrder", func() {
		It("sends PUT with form params for price/stop/type/duration and returns nil", func() {
			var capturedMethod, capturedPath string
			var capturedForm url.Values

			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/orders/77777", func(writer http.ResponseWriter, req *http.Request) {
					capturedMethod = req.Method
					capturedPath = req.URL.Path
					if parseErr := req.ParseForm(); parseErr == nil {
						capturedForm = req.Form
					}

					writer.WriteHeader(http.StatusOK)
					fmt.Fprint(writer, `{"order":{"id":77777,"status":"ok"}}`)
				})
			})

			params := url.Values{}
			params.Set("type", "limit")
			params.Set("price", "155.50")
			params.Set("duration", "day")

			modifyErr := client.ModifyOrder(ctx, "77777", params)
			Expect(modifyErr).ToNot(HaveOccurred())
			Expect(capturedMethod).To(Equal("PUT"))
			Expect(capturedPath).To(Equal("/accounts/TEST-ACCT/orders/77777"))
			Expect(capturedForm.Get("type")).To(Equal("limit"))
			Expect(capturedForm.Get("price")).To(Equal("155.50"))
			Expect(capturedForm.Get("duration")).To(Equal("day"))
		})
	})

	Describe("getOrders", func() {
		It("GET returns parsed orders when the response is an array", func() {
			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					Expect(req.Method).To(Equal("GET"))
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{
						"orders": {
							"order": [
								{"id":1001,"symbol":"AAPL","side":"buy","quantity":10,"type":"market","status":"filled","duration":"day"},
								{"id":1002,"symbol":"GOOG","side":"sell","quantity":5,"type":"limit","status":"open","duration":"gtc","price":150.0}
							]
						}
					}`)
				})
			})

			orders, getErr := client.GetOrders(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(2))
			Expect(orders[0].ID).To(Equal(int64(1001)))
			Expect(orders[0].Symbol).To(Equal("AAPL"))
			Expect(orders[0].Side).To(Equal("buy"))
			Expect(orders[1].ID).To(Equal(int64(1002)))
			Expect(orders[1].Symbol).To(Equal("GOOG"))
			Expect(orders[1].Price).To(Equal(150.0))
		})

		It("GET returns a single order as a slice when the response is an object", func() {
			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{
						"orders": {
							"order": {"id":2001,"symbol":"TSLA","side":"buy","quantity":3,"type":"limit","status":"open","duration":"day","price":200.0}
						}
					}`)
				})
			})

			orders, getErr := client.GetOrders(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].ID).To(Equal(int64(2001)))
			Expect(orders[0].Symbol).To(Equal("TSLA"))
		})
	})

	Describe("getPositions", func() {
		It("GET returns parsed positions when the response is an array", func() {
			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/positions", func(writer http.ResponseWriter, req *http.Request) {
					Expect(req.Method).To(Equal("GET"))
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{
						"positions": {
							"position": [
								{"id":101,"symbol":"AAPL","quantity":50,"cost_basis":7500.0,"date_acquired":"2024-01-15T00:00:00.000Z"},
								{"id":102,"symbol":"MSFT","quantity":20,"cost_basis":6000.0,"date_acquired":"2024-02-10T00:00:00.000Z"}
							]
						}
					}`)
				})
			})

			positions, getErr := client.GetPositions(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(2))
			Expect(positions[0].Symbol).To(Equal("AAPL"))
			Expect(positions[0].Quantity).To(Equal(50.0))
			Expect(positions[0].CostBasis).To(Equal(7500.0))
			Expect(positions[1].Symbol).To(Equal("MSFT"))
		})

		It("GET returns a single position as a slice when the response is an object", func() {
			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/positions", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{
						"positions": {
							"position": {"id":201,"symbol":"NVDA","quantity":15,"cost_basis":9000.0,"date_acquired":"2024-03-01T00:00:00.000Z"}
						}
					}`)
				})
			})

			positions, getErr := client.GetPositions(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Symbol).To(Equal("NVDA"))
			Expect(positions[0].Quantity).To(Equal(15.0))
		})
	})

	Describe("getBalance", func() {
		It("GET returns parsed balance for a margin account", func() {
			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/balances", func(writer http.ResponseWriter, req *http.Request) {
					Expect(req.Method).To(Equal("GET"))
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{
						"balances": {
							"account_number": "TEST-ACCT",
							"account_type": "margin",
							"total_equity": 100000.0,
							"total_cash": 25000.0,
							"market_value": 75000.0,
							"margin": {
								"stock_buying_power": 50000.0,
								"current_requirement": 10000.0
							},
							"cash": {
								"cash_available": 0.0,
								"unsettled_funds": 0.0
							}
						}
					}`)
				})
			})

			balance, getErr := client.GetBalance(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(balance.AccountNumber).To(Equal("TEST-ACCT"))
			Expect(balance.AccountType).To(Equal("margin"))
			Expect(balance.TotalEquity).To(Equal(100000.0))
			Expect(balance.TotalCash).To(Equal(25000.0))
			Expect(balance.Margin.StockBuyingPower).To(Equal(50000.0))
			Expect(balance.Margin.CurrentRequirement).To(Equal(10000.0))
		})

		It("GET returns parsed balance for a cash account", func() {
			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/TEST-ACCT/balances", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{
						"balances": {
							"account_number": "TEST-ACCT",
							"account_type": "cash",
							"total_equity": 30000.0,
							"total_cash": 30000.0,
							"market_value": 0.0,
							"margin": {
								"stock_buying_power": 0.0,
								"current_requirement": 0.0
							},
							"cash": {
								"cash_available": 28000.0,
								"unsettled_funds": 2000.0
							}
						}
					}`)
				})
			})

			balance, getErr := client.GetBalance(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(balance.AccountType).To(Equal("cash"))
			Expect(balance.TotalEquity).To(Equal(30000.0))
			Expect(balance.Cash.CashAvailable).To(Equal(28000.0))
			Expect(balance.Cash.UnsettledFunds).To(Equal(2000.0))
		})
	})

	Describe("getQuote", func() {
		It("GET returns the last price for a symbol", func() {
			var capturedSymbol string

			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/markets/quotes", func(writer http.ResponseWriter, req *http.Request) {
					Expect(req.Method).To(Equal("GET"))
					capturedSymbol = req.URL.Query().Get("symbols")
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{
						"quotes": {
							"quote": {
								"last": 178.25
							}
						}
					}`)
				})
			})

			price, getErr := client.GetQuote(ctx, "AAPL")
			Expect(getErr).ToNot(HaveOccurred())
			Expect(price).To(Equal(178.25))
			Expect(capturedSymbol).To(Equal("AAPL"))
		})
	})

	Describe("createStreamSession", func() {
		It("POST returns the session ID string", func() {
			var capturedMethod, capturedPath string

			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/events/session", func(writer http.ResponseWriter, req *http.Request) {
					capturedMethod = req.Method
					capturedPath = req.URL.Path
					writer.Header().Set("Content-Type", "application/json")
					fmt.Fprint(writer, `{
						"stream": {
							"sessionid": "abc-session-123",
							"url": "wss://stream.tradier.com/v1/markets/events"
						}
					}`)
				})
			})

			sessionID, sessionErr := client.CreateStreamSession(ctx)
			Expect(sessionErr).ToNot(HaveOccurred())
			Expect(sessionID).To(Equal("abc-session-123"))
			Expect(capturedMethod).To(Equal("POST"))
			Expect(capturedPath).To(Equal("/accounts/events/session"))
		})

		It("returns an HTTPError when the server responds with an error", func() {
			client := authenticatedClient(func(mux *http.ServeMux) {
				mux.HandleFunc("/accounts/events/session", func(writer http.ResponseWriter, req *http.Request) {
					writer.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(writer, `{"fault":{"description":"invalid token"}}`)
				})
			})

			_, sessionErr := client.CreateStreamSession(ctx)
			Expect(sessionErr).To(HaveOccurred())

			httpErr, ok := sessionErr.(*tradier.HTTPError)
			Expect(ok).To(BeTrue())
			Expect(httpErr.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})
})
