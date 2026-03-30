package tradestation_test

import (
	"context"
	"fmt"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/tradestation"
)

var _ = Describe("orderStreamer", Label("streaming"), func() {
	var (
		ctx       context.Context
		cancelCtx context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancelCtx = context.WithTimeout(context.Background(), 10*time.Second)
	})

	AfterEach(func() {
		cancelCtx()
	})

	Describe("Fill delivery", func() {
		It("emits a broker.Fill when a filled order event arrives on the stream", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Path).To(Equal("/v3/brokerage/stream/accounts/ACCT-TEST/orders"))
				Expect(req.Header.Get("Authorization")).To(Equal("Bearer access-token"))

				writer.Header().Set("Content-Type", "application/vnd.tradestation.streams.v3+json")
				flusher := writer.(http.Flusher)

				event := map[string]any{
					"OrderID":        "987654",
					"Status":         "FLL",
					"OrderType":      "Market",
					"FilledQuantity": "50",
					"FilledPrice":    "150.25",
					"Legs": []map[string]any{
						{
							"Symbol":    "AAPL",
							"BuyOrSell": "1",
							"Fills": []map[string]any{
								{
									"ExecId":    "EXEC-1",
									"Quantity":  "50",
									"Price":     "150.25",
									"Timestamp": "2026-03-20T14:30:00Z",
								},
							},
						},
					},
				}

				data, _ := sonic.Marshal(event)
				fmt.Fprintf(writer, "%s\n", data)
				flusher.Flush()

				// Keep connection open until context cancelled.
				<-req.Context().Done()
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			streamer := tradestation.NewOrderStreamerForTest(client, fills, server.URL, "ACCT-TEST", "access-token")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var received broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&received))
			Expect(received.OrderID).To(Equal("987654"))
			Expect(received.Price).To(Equal(150.25))
			Expect(received.Qty).To(Equal(50.0))
		})
	})

	Describe("Deduplication", func() {
		It("delivers only one fill when the same fill is sent twice", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/vnd.tradestation.streams.v3+json")
				flusher := writer.(http.Flusher)

				event := map[string]any{
					"OrderID":        "111",
					"Status":         "FLL",
					"FilledQuantity": "10",
					"FilledPrice":    "99.50",
					"Legs": []map[string]any{
						{
							"Symbol":    "SPY",
							"BuyOrSell": "1",
							"Fills": []map[string]any{
								{
									"ExecId":    "EXEC-DUP",
									"Quantity":  "10",
									"Price":     "99.50",
									"Timestamp": "2026-03-20T14:30:00Z",
								},
							},
						},
					},
				}

				data, _ := sonic.Marshal(event)
				// Send same event twice.
				fmt.Fprintf(writer, "%s\n", data)
				flusher.Flush()
				time.Sleep(50 * time.Millisecond)
				fmt.Fprintf(writer, "%s\n", data)
				flusher.Flush()

				<-req.Context().Done()
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			streamer := tradestation.NewOrderStreamerForTest(client, fills, server.URL, "ACCT-TEST", "access-token")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var firstFill broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&firstFill))
			Expect(firstFill.OrderID).To(Equal("111"))

			Consistently(fills, 1*time.Second).ShouldNot(Receive())
		})
	})

	Describe("GoAway signal", func() {
		It("reconnects when a GoAway signal is received", func() {
			streamConnectCount := 0

			mux := http.NewServeMux()
			mux.HandleFunc("/v3/brokerage/stream/", func(writer http.ResponseWriter, req *http.Request) {
				streamConnectCount++
				writer.Header().Set("Content-Type", "application/vnd.tradestation.streams.v3+json")
				flusher := writer.(http.Flusher)

				if streamConnectCount == 1 {
					// First connection: send GoAway.
					goAway := map[string]any{"GoAway": true}
					data, _ := sonic.Marshal(goAway)
					fmt.Fprintf(writer, "%s\n", data)
					flusher.Flush()
					return
				}

				// Second connection: send a fill then stay open.
				event := map[string]any{
					"OrderID":        "222",
					"Status":         "FLL",
					"FilledQuantity": "5",
					"FilledPrice":    "100.00",
					"Legs": []map[string]any{
						{
							"Symbol":    "QQQ",
							"BuyOrSell": "1",
							"Fills": []map[string]any{
								{
									"ExecId":    "EXEC-GA",
									"Quantity":  "5",
									"Price":     "100.00",
									"Timestamp": "2026-03-20T15:00:00Z",
								},
							},
						},
					},
				}

				data, _ := sonic.Marshal(event)
				fmt.Fprintf(writer, "%s\n", data)
				flusher.Flush()

				<-req.Context().Done()
			})
			// Return empty orders for pollMissedFills.
			mux.HandleFunc("/v3/brokerage/accounts/", func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{"Orders": []any{}})
			})

			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradestation.NewAPIClientForTest(server.URL, "test-token")
			client.SetAccountID("ACCT-TEST")

			streamer := tradestation.NewOrderStreamerForTest(client, fills, server.URL, "ACCT-TEST", "access-token")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var received broker.Fill
			Eventually(fills, 5*time.Second).Should(Receive(&received))
			Expect(received.OrderID).To(Equal("222"))
		})
	})

	Describe("Shutdown", func() {
		It("stops the goroutine when CloseStreamer is called", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/vnd.tradestation.streams.v3+json")
				writer.(http.Flusher).Flush()
				<-req.Context().Done()
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradestation.NewAPIClientForTest(server.URL, "test-token")

			streamer := tradestation.NewOrderStreamerForTest(client, fills, server.URL, "ACCT-TEST", "access-token")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())

			closeDone := make(chan error, 1)
			go func() {
				closeDone <- streamer.CloseStreamer()
			}()

			Eventually(closeDone, 3*time.Second).Should(Receive(Not(HaveOccurred())))
		})
	})
})
