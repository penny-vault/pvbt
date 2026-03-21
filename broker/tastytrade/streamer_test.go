package tastytrade_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/tastytrade"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(req *http.Request) bool { return true },
}

var _ = Describe("fillStreamer", Label("streaming"), func() {
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

	// wsServerURL converts an httptest.Server URL from http:// to ws://.
	wsServerURL := func(server *httptest.Server) string {
		return "ws" + strings.TrimPrefix(server.URL, "http")
	}

	Describe("Fill delivery", Label("streaming"), func() {
		It("emits a broker.Fill with correct fields when a fill event arrives", func() {
			handlerDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()
				// Consume the connect message sent by the streamer.
				conn.ReadMessage()
				envelope := map[string]any{
					"type": "Order",
					"data": map[string]any{
						"id": "ORD-100", "status": "Filled",
						"legs": []map[string]any{{
							"symbol": "AAPL", "instrument-type": "Equity", "action": "Buy to Open", "quantity": 50,
							"fills": []map[string]any{{"fill-id": "FILL-1", "fill-price": 150.25, "quantity": "50", "filled-at": "2026-03-20T14:30:00Z"}},
						}},
					},
					"timestamp": 1742480400000,
				}
				payload, marshalErr := json.Marshal(envelope)
				Expect(marshalErr).ToNot(HaveOccurred())
				conn.WriteMessage(websocket.TextMessage, payload)
				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)
			fills := make(chan broker.Fill, 10)
			client := tastytrade.NewAPIClientForTest("http://unused.test")
			streamer := tastytrade.NewFillStreamerForTest(client, fills, wsServerURL(server))
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })
			var received broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&received))
			Expect(received.OrderID).To(Equal("ORD-100"))
			Expect(received.Price).To(Equal(150.25))
			Expect(received.Qty).To(Equal(50.0))
			Expect(received.FilledAt).To(Equal(time.Date(2026, 3, 20, 14, 30, 0, 0, time.UTC)))
		})
	})

	Describe("Deduplication", Label("streaming"), func() {
		It("delivers only one fill when the same fill ID is sent twice", func() {
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()
				// Consume the connect message sent by the streamer.
				conn.ReadMessage()

				envelope := map[string]any{
					"type": "Order",
					"data": map[string]any{
						"id": "ORD-200", "status": "Filled",
						"legs": []map[string]any{{
							"symbol": "AAPL", "instrument-type": "Equity", "action": "Buy to Open", "quantity": 10,
							"fills": []map[string]any{{"fill-id": "FILL-DUP", "fill-price": 99.50, "quantity": "10", "filled-at": "2026-03-20T14:30:00Z"}},
						}},
					},
					"timestamp": 1742480400000,
				}

				payload, marshalErr := json.Marshal(envelope)
				Expect(marshalErr).ToNot(HaveOccurred())

				// Send the same fill twice.
				conn.WriteMessage(websocket.TextMessage, payload)
				time.Sleep(50 * time.Millisecond)
				conn.WriteMessage(websocket.TextMessage, payload)

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tastytrade.NewAPIClientForTest("http://unused.test")
			streamer := tastytrade.NewFillStreamerForTest(client, fills, wsServerURL(server))

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			// First fill should arrive.
			var firstFill broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&firstFill))
			Expect(firstFill.OrderID).To(Equal("ORD-200"))

			// No second fill should arrive.
			Consistently(fills, 1*time.Second).ShouldNot(Receive())
		})
	})

	Describe("Partial fills", Label("streaming"), func() {
		It("delivers both fills when they share an order ID but have different fill IDs", func() {
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()
				// Consume the connect message sent by the streamer.
				conn.ReadMessage()

				envelope := map[string]any{
					"type": "Order",
					"data": map[string]any{
						"id": "ORD-300", "status": "Filled",
						"legs": []map[string]any{{
							"symbol": "AAPL", "instrument-type": "Equity", "action": "Buy to Open", "quantity": 100,
							"fills": []map[string]any{
								{"fill-id": "FILL-A", "fill-price": 200.00, "quantity": "30", "filled-at": "2026-03-20T15:00:00Z"},
								{"fill-id": "FILL-B", "fill-price": 200.10, "quantity": "70", "filled-at": "2026-03-20T15:00:01Z"},
							},
						}},
					},
					"timestamp": 1742480400000,
				}

				payload, marshalErr := json.Marshal(envelope)
				Expect(marshalErr).ToNot(HaveOccurred())
				conn.WriteMessage(websocket.TextMessage, payload)

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tastytrade.NewAPIClientForTest("http://unused.test")
			streamer := tastytrade.NewFillStreamerForTest(client, fills, wsServerURL(server))

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var firstFill, secondFill broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&firstFill))
			Eventually(fills, 3*time.Second).Should(Receive(&secondFill))

			Expect(firstFill.OrderID).To(Equal("ORD-300"))
			Expect(firstFill.Qty).To(Equal(30.0))

			Expect(secondFill.OrderID).To(Equal("ORD-300"))
			Expect(secondFill.Qty).To(Equal(70.0))
		})
	})

	Describe("Connect message", Label("streaming"), func() {
		It("sends a connect action with auth token and account after dialing", func() {
			connectReceived := make(chan map[string]any, 1)
			handlerDone := make(chan struct{})

			mux := http.NewServeMux()
			mux.HandleFunc("POST /sessions", func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"data": map[string]any{
						"session-token": "ws-test-token",
						"user":          map[string]any{"external-id": "u1"},
					},
				})
			})
			mux.HandleFunc("GET /customers/me/accounts", func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{
					"data": map[string]any{
						"items": []map[string]any{
							{"account": map[string]any{"account-number": "WS-ACCT"}},
						},
					},
				})
			})

			restServer := httptest.NewServer(mux)
			DeferCleanup(restServer.Close)

			wsServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				_, msgData, readErr := conn.ReadMessage()
				Expect(readErr).ToNot(HaveOccurred())

				var msg map[string]any
				json.Unmarshal(msgData, &msg)
				connectReceived <- msg

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(wsServer.Close)

			client := tastytrade.NewAPIClientForTest(restServer.URL)
			Expect(client.Authenticate(ctx, "user@test.com", "secret")).To(Succeed())

			fills := make(chan broker.Fill, 10)
			streamer := tastytrade.NewFillStreamerForTest(client, fills, wsServerURL(wsServer))

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var msg map[string]any
			Eventually(connectReceived, 3*time.Second).Should(Receive(&msg))
			Expect(msg["action"]).To(Equal("connect"))
			Expect(msg["auth-token"]).To(Equal("ws-test-token"))

			valueSlice, ok := msg["value"].([]any)
			Expect(ok).To(BeTrue())
			Expect(valueSlice).To(ContainElement("WS-ACCT"))
		})
	})

	Describe("Shutdown", Label("streaming"), func() {
		It("stops the goroutine when CloseStreamer is called", func() {
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tastytrade.NewAPIClientForTest("http://unused.test")
			streamer := tastytrade.NewFillStreamerForTest(client, fills, wsServerURL(server))

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())

			// Close should return promptly, meaning the goroutine exited.
			closeDone := make(chan error, 1)
			go func() {
				closeDone <- streamer.CloseStreamer()
			}()

			Eventually(closeDone, 3*time.Second).Should(Receive(Not(HaveOccurred())))
		})
	})
})
