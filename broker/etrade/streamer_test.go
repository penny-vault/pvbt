package etrade_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/etrade"
)

// ordersResponse builds a minimal E*TRADE orders JSON response.
func ordersResponse(orders []map[string]any) map[string]any {
	return map[string]any{
		"OrdersResponse": map[string]any{
			"Order": orders,
		},
	}
}

// openOrder returns a JSON-serialisable map for an OPEN order.
func openOrder(orderID int64, symbol string) map[string]any {
	return map[string]any{
		"orderId":     orderID,
		"orderType":   "EQ",
		"orderStatus": "OPEN",
		"OrderDetail": []map[string]any{
			{
				"priceType": "MARKET",
				"Instrument": []map[string]any{
					{
						"Product":                  map[string]any{"symbol": symbol},
						"orderAction":              "BUY",
						"orderedQuantity":          10.0,
						"filledQuantity":           0.0,
						"averageExecutionPrice":    0.0,
					},
				},
			},
		},
	}
}

// executedOrder returns a JSON-serialisable map for an EXECUTED order.
func executedOrder(orderID int64, symbol string, qty, price float64) map[string]any {
	return map[string]any{
		"orderId":     orderID,
		"orderType":   "EQ",
		"orderStatus": "EXECUTED",
		"OrderDetail": []map[string]any{
			{
				"priceType": "MARKET",
				"Instrument": []map[string]any{
					{
						"Product":               map[string]any{"symbol": symbol},
						"orderAction":           "BUY",
						"orderedQuantity":       qty,
						"filledQuantity":        qty,
						"averageExecutionPrice": price,
					},
				},
			},
		},
	}
}

// partialOrder returns a JSON-serialisable map for an INDIVIDUAL_FILLS order.
func partialOrder(orderID int64, symbol string, filledQty, price float64) map[string]any {
	return map[string]any{
		"orderId":     orderID,
		"orderType":   "EQ",
		"orderStatus": "INDIVIDUAL_FILLS",
		"OrderDetail": []map[string]any{
			{
				"priceType": "MARKET",
				"Instrument": []map[string]any{
					{
						"Product":               map[string]any{"symbol": symbol},
						"orderAction":           "BUY",
						"orderedQuantity":       100.0,
						"filledQuantity":        filledQty,
						"averageExecutionPrice": price,
					},
				},
			},
		},
	}
}

var _ = Describe("orderPoller", func() {
	var (
		server *httptest.Server
		cl     *etrade.APIClientForTest
		fills  chan broker.Fill
		poller *etrade.APIClientForTest // placeholder -- overridden per test
	)

	_ = poller // suppress unused warning

	BeforeEach(func() {
		fills = make(chan broker.Fill, 10)
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
			server = nil
		}
	})

	Describe("Poll", func() {
		It("detects a new fill when order transitions from OPEN to EXECUTED", func() {
			var callCount atomic.Int32

			server = httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				count := callCount.Add(1)

				var resp map[string]any
				if count == 1 {
					resp = ordersResponse([]map[string]any{
						openOrder(12345, "AAPL"),
					})
				} else {
					resp = ordersResponse([]map[string]any{
						executedOrder(12345, "AAPL", 10.0, 150.25),
					})
				}

				_ = json.NewEncoder(ww).Encode(resp)
			}))

			cl = etrade.NewAPIClientForTest(server.URL, testCreds(), testAccountIDKey)
			op := etrade.NewOrderPollerForTest(cl, fills)

			ctx := context.Background()

			// First poll -- order is OPEN, no fill expected.
			Expect(op.Poll(ctx)).To(Succeed())
			Consistently(fills, 50*time.Millisecond).ShouldNot(Receive())

			// Second poll -- order is EXECUTED, fill expected.
			Expect(op.Poll(ctx)).To(Succeed())

			var fill broker.Fill
			Eventually(fills, time.Second).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("12345"))
			Expect(fill.Price).To(BeNumerically("~", 150.25, 0.001))
			Expect(fill.Qty).To(BeNumerically("~", 10.0, 0.001))
		})

		It("deduplicates fills for the same executed order polled twice", func() {
			server = httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				resp := ordersResponse([]map[string]any{
					executedOrder(99001, "MSFT", 5.0, 320.00),
				})
				_ = json.NewEncoder(ww).Encode(resp)
			}))

			cl = etrade.NewAPIClientForTest(server.URL, testCreds(), testAccountIDKey)
			op := etrade.NewOrderPollerForTest(cl, fills)
			ctx := context.Background()

			Expect(op.Poll(ctx)).To(Succeed())
			Expect(op.Poll(ctx)).To(Succeed())

			var fill broker.Fill
			Eventually(fills, time.Second).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("99001"))

			// Only one fill should have been emitted.
			Consistently(fills, 100*time.Millisecond).ShouldNot(Receive())
		})

		It("handles partial fills with INDIVIDUAL_FILLS status", func() {
			server = httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				resp := ordersResponse([]map[string]any{
					partialOrder(77777, "GOOG", 30.0, 175.50),
				})
				_ = json.NewEncoder(ww).Encode(resp)
			}))

			cl = etrade.NewAPIClientForTest(server.URL, testCreds(), testAccountIDKey)
			op := etrade.NewOrderPollerForTest(cl, fills)
			ctx := context.Background()

			Expect(op.Poll(ctx)).To(Succeed())

			var fill broker.Fill
			Eventually(fills, time.Second).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("77777"))
			Expect(fill.Qty).To(BeNumerically("~", 30.0, 0.001))
			Expect(fill.Price).To(BeNumerically("~", 175.50, 0.001))
		})

		It("handles an empty orders list without errors", func() {
			server = httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"OrdersResponse": map[string]any{},
				}
				_ = json.NewEncoder(ww).Encode(resp)
			}))

			cl = etrade.NewAPIClientForTest(server.URL, testCreds(), testAccountIDKey)
			op := etrade.NewOrderPollerForTest(cl, fills)
			ctx := context.Background()

			Expect(op.Poll(ctx)).To(Succeed())
			Consistently(fills, 100*time.Millisecond).ShouldNot(Receive())
		})

		It("returns an error when the API call fails", func() {
			server = httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, _ *http.Request) {
				http.Error(ww, "internal server error", http.StatusInternalServerError)
			}))

			cl = etrade.NewAPIClientForTest(server.URL, testCreds(), testAccountIDKey)
			op := etrade.NewOrderPollerForTest(cl, fills)
			ctx := context.Background()

			// The resty client retries on 500 -- use a context with a short deadline
			// so the test doesn't block on retries indefinitely.
			pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			err := op.Poll(pollCtx)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Start/Stop", func() {
		It("starts background polling and stop cancels it", func() {
			var callCount atomic.Int32

			server = httptest.NewServer(http.HandlerFunc(func(ww http.ResponseWriter, _ *http.Request) {
				ww.Header().Set("Content-Type", "application/json")
				callCount.Add(1)
				resp := ordersResponse([]map[string]any{
					executedOrder(55555, "SPY", 100.0, 450.00),
				})
				_ = json.NewEncoder(ww).Encode(resp)
			}))

			cl = etrade.NewAPIClientForTest(server.URL, testCreds(), testAccountIDKey)
			op := etrade.NewOrderPollerForTest(cl, fills)

			// Use a very short poll interval by direct field manipulation is not
			// possible through the exported API, so we test that at least one fill
			// arrives via start, then stop.
			ctx := context.Background()
			op.Start(ctx)

			var fill broker.Fill
			Eventually(fills, 10*time.Second).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("55555"))

			op.Stop()
		})
	})
})
