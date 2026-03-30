package alpaca_test

import (
	"context"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/alpaca"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(req *http.Request) bool { return true },
}

// wsServerURL converts an httptest.Server URL from http:// to ws://.
func wsServerURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

// newAuthedServer creates a test WebSocket server that performs the full
// Alpaca auth + listen handshake, then calls onReady with the connection.
// It blocks until handlerDone is closed.
func newAuthedServer(authReceived chan map[string]any, listenReceived chan map[string]any, onReady func(conn *websocket.Conn), handlerDone chan struct{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
		Expect(upgradeErr).ToNot(HaveOccurred())
		defer conn.Close()

		// Read auth message.
		_, authData, readErr := conn.ReadMessage()
		Expect(readErr).ToNot(HaveOccurred())

		var authMsg map[string]any
		Expect(sonic.Unmarshal(authData, &authMsg)).To(Succeed())

		if authReceived != nil {
			authReceived <- authMsg
		}

		// Send authorized response.
		authResp := map[string]any{
			"stream": "authorization",
			"data": map[string]any{
				"status": "authorized",
				"action": "authenticate",
			},
		}

		payload, marshalErr := sonic.Marshal(authResp)
		Expect(marshalErr).ToNot(HaveOccurred())
		Expect(conn.WriteMessage(websocket.TextMessage, payload)).To(Succeed())

		// Read listen message.
		_, listenData, listenErr := conn.ReadMessage()
		Expect(listenErr).ToNot(HaveOccurred())

		var listenMsg map[string]any
		Expect(sonic.Unmarshal(listenData, &listenMsg)).To(Succeed())

		if listenReceived != nil {
			listenReceived <- listenMsg
		}

		// Send listen ack.
		listenAck := map[string]any{
			"stream": "listening",
			"data": map[string]any{
				"streams": []string{"trade_updates"},
			},
		}

		ackPayload, ackErr := sonic.Marshal(listenAck)
		Expect(ackErr).ToNot(HaveOccurred())
		Expect(conn.WriteMessage(websocket.TextMessage, ackPayload)).To(Succeed())

		if onReady != nil {
			onReady(conn)
		}

		<-handlerDone
	}))
}

func sendTradeUpdate(conn *websocket.Conn, event, executionID, orderID, price, qty, timestamp string) {
	tradeUpdate := map[string]any{
		"stream": "trade_updates",
		"data": map[string]any{
			"event":        event,
			"execution_id": executionID,
			"price":        price,
			"qty":          qty,
			"timestamp":    timestamp,
			"order": map[string]any{
				"id": orderID,
			},
		},
	}

	payload, marshalErr := sonic.Marshal(tradeUpdate)
	Expect(marshalErr).ToNot(HaveOccurred())
	Expect(conn.WriteMessage(websocket.TextMessage, payload)).To(Succeed())
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

	Describe("Auth flow", Label("streaming"), func() {
		It("sends auth with correct key and secret", func() {
			authReceived := make(chan map[string]any, 1)
			handlerDone := make(chan struct{})

			server := newAuthedServer(authReceived, nil, nil, handlerDone)
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := alpaca.NewAPIClientForTest("http://unused.test", "test-key", "test-secret")
			streamer := alpaca.NewFillStreamerForTest(client, fills, wsServerURL(server), "test-key", "test-secret")

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var msg map[string]any
			Eventually(authReceived, 3*time.Second).Should(Receive(&msg))
			Expect(msg["action"]).To(Equal("auth"))
			Expect(msg["key"]).To(Equal("test-key"))
			Expect(msg["secret"]).To(Equal("test-secret"))
		})

		It("sends listen after auth", func() {
			listenReceived := make(chan map[string]any, 1)
			handlerDone := make(chan struct{})

			server := newAuthedServer(nil, listenReceived, nil, handlerDone)
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := alpaca.NewAPIClientForTest("http://unused.test", "test-key", "test-secret")
			streamer := alpaca.NewFillStreamerForTest(client, fills, wsServerURL(server), "test-key", "test-secret")

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var msg map[string]any
			Eventually(listenReceived, 3*time.Second).Should(Receive(&msg))
			Expect(msg["action"]).To(Equal("listen"))

			dataMap, ok := msg["data"].(map[string]any)
			Expect(ok).To(BeTrue())

			streams, ok := dataMap["streams"].([]any)
			Expect(ok).To(BeTrue())
			Expect(streams).To(ContainElement("trade_updates"))
		})

		It("returns ErrNotAuthenticated on unauthorized response", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				// Read auth message.
				conn.ReadMessage()

				// Send unauthorized response.
				unauthorizedResp := map[string]any{
					"stream": "authorization",
					"data": map[string]any{
						"status": "unauthorized",
						"action": "authenticate",
					},
				}

				payload, marshalErr := sonic.Marshal(unauthorizedResp)
				Expect(marshalErr).ToNot(HaveOccurred())
				conn.WriteMessage(websocket.TextMessage, payload)
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := alpaca.NewAPIClientForTest("http://unused.test", "bad-key", "bad-secret")
			streamer := alpaca.NewFillStreamerForTest(client, fills, wsServerURL(server), "bad-key", "bad-secret")

			err := streamer.ConnectStreamer(ctx)
			Expect(err).To(MatchError(broker.ErrNotAuthenticated))
		})
	})

	Describe("Fill delivery", Label("streaming"), func() {
		It("emits a broker.Fill with correct fields when a fill event arrives", func() {
			handlerDone := make(chan struct{})

			server := newAuthedServer(nil, nil, func(conn *websocket.Conn) {
				sendTradeUpdate(conn, "fill", "exec-1", "ORD-100", "150.25", "50", "2026-03-20T14:30:00Z")
			}, handlerDone)
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := alpaca.NewAPIClientForTest("http://unused.test", "test-key", "test-secret")
			streamer := alpaca.NewFillStreamerForTest(client, fills, wsServerURL(server), "test-key", "test-secret")

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
		It("delivers only one fill when the same execution_id is sent twice", func() {
			handlerDone := make(chan struct{})

			server := newAuthedServer(nil, nil, func(conn *websocket.Conn) {
				sendTradeUpdate(conn, "fill", "exec-dup", "ORD-200", "99.50", "10", "2026-03-20T14:30:00Z")
				time.Sleep(50 * time.Millisecond)
				sendTradeUpdate(conn, "fill", "exec-dup", "ORD-200", "99.50", "10", "2026-03-20T14:30:00Z")
			}, handlerDone)
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := alpaca.NewAPIClientForTest("http://unused.test", "test-key", "test-secret")
			streamer := alpaca.NewFillStreamerForTest(client, fills, wsServerURL(server), "test-key", "test-secret")

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
		It("delivers both fills when they share an order ID but have different execution_ids", func() {
			handlerDone := make(chan struct{})

			server := newAuthedServer(nil, nil, func(conn *websocket.Conn) {
				sendTradeUpdate(conn, "partial_fill", "exec-A", "ORD-300", "200.00", "30", "2026-03-20T15:00:00Z")
				time.Sleep(50 * time.Millisecond)
				sendTradeUpdate(conn, "fill", "exec-B", "ORD-300", "200.10", "70", "2026-03-20T15:00:01Z")
			}, handlerDone)
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := alpaca.NewAPIClientForTest("http://unused.test", "test-key", "test-secret")
			streamer := alpaca.NewFillStreamerForTest(client, fills, wsServerURL(server), "test-key", "test-secret")

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

	Describe("Shutdown", Label("streaming"), func() {
		It("returns promptly when CloseStreamer is called", func() {
			handlerDone := make(chan struct{})

			server := newAuthedServer(nil, nil, nil, handlerDone)
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := alpaca.NewAPIClientForTest("http://unused.test", "test-key", "test-secret")
			streamer := alpaca.NewFillStreamerForTest(client, fills, wsServerURL(server), "test-key", "test-secret")

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())

			closeDone := make(chan error, 1)
			go func() {
				closeDone <- streamer.CloseStreamer()
			}()

			Eventually(closeDone, 3*time.Second).Should(Receive(Not(HaveOccurred())))
		})
	})
})
