package ibkr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/ibkr"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(req *http.Request) bool { return true },
}

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

	wsServerURL := func(server *httptest.Server) string {
		return "ws" + strings.TrimPrefix(server.URL, "http")
	}

	Describe("connect and subscribe", func() {
		It("sends sor+{} upon connecting", func() {
			subscribeReceived := make(chan string, 1)
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				// Read the subscription message.
				_, msgData, readErr := conn.ReadMessage()
				Expect(readErr).ToNot(HaveOccurred())
				subscribeReceived <- string(msgData)

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var received string
			Eventually(subscribeReceived, 3*time.Second).Should(Receive(&received))
			Expect(received).To(Equal("sor+{}"))
		})
	})

	Describe("fill delivery", func() {
		It("emits a broker.Fill when a sor fill message arrives", func() {
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				// Consume the sor+{} subscription.
				conn.ReadMessage()

				// Send a fill message.
				fillMsg := map[string]any{
					"topic": "sor",
					"args": map[string]any{
						"orderId":        "ORD-123",
						"status":         "Filled",
						"filledQuantity": 50.0,
						"avgPrice":       150.25,
					},
				}
				msgBytes, marshalErr := json.Marshal(fillMsg)
				Expect(marshalErr).ToNot(HaveOccurred())
				Expect(conn.WriteMessage(websocket.TextMessage, msgBytes)).To(Succeed())

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var received broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&received))
			Expect(received.OrderID).To(Equal("ORD-123"))
			Expect(received.Qty).To(Equal(50.0))
			Expect(received.Price).To(Equal(150.25))
		})

		It("emits a fill for PartiallyFilled status", func() {
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				conn.ReadMessage()

				fillMsg := map[string]any{
					"topic": "sor",
					"args": map[string]any{
						"orderId":        "ORD-456",
						"status":         "PartiallyFilled",
						"filledQuantity": 25.0,
						"avgPrice":       99.50,
					},
				}
				msgBytes, marshalErr := json.Marshal(fillMsg)
				Expect(marshalErr).ToNot(HaveOccurred())
				Expect(conn.WriteMessage(websocket.TextMessage, msgBytes)).To(Succeed())

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var received broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&received))
			Expect(received.OrderID).To(Equal("ORD-456"))
			Expect(received.Qty).To(Equal(25.0))
			Expect(received.Price).To(Equal(99.50))
		})
	})

	Describe("deduplication", func() {
		It("delivers only one fill when the same fill is sent twice", func() {
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				conn.ReadMessage()

				fillMsg := map[string]any{
					"topic": "sor",
					"args": map[string]any{
						"orderId":        "ORD-DUP",
						"status":         "Filled",
						"filledQuantity": 10.0,
						"avgPrice":       200.00,
					},
				}
				msgBytes, marshalErr := json.Marshal(fillMsg)
				Expect(marshalErr).ToNot(HaveOccurred())

				// Send the same fill twice.
				Expect(conn.WriteMessage(websocket.TextMessage, msgBytes)).To(Succeed())
				time.Sleep(50 * time.Millisecond)
				Expect(conn.WriteMessage(websocket.TextMessage, msgBytes)).To(Succeed())

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var firstFill broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&firstFill))
			Expect(firstFill.OrderID).To(Equal("ORD-DUP"))

			Consistently(fills, 1*time.Second).ShouldNot(Receive())
		})
	})

	Describe("heartbeat", func() {
		It("sends tic messages periodically", func() {
			var ticCount atomic.Int32
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				// Consume the sor+{} subscription.
				conn.ReadMessage()

				// Read subsequent messages and count tic heartbeats.
				for {
					_, msgData, readErr := conn.ReadMessage()
					if readErr != nil {
						return
					}

					if string(msgData) == "tic" {
						ticCount.Add(1)
					}
				}
			}))
			DeferCleanup(func() {
				select {
				case <-handlerDone:
				default:
					close(handlerDone)
				}
			})
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			ibkr.SetStreamerHeartbeatForTest(streamer, 50*time.Millisecond)
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			// Wait for at least 2 heartbeats.
			Eventually(func() int32 {
				return ticCount.Load()
			}, 3*time.Second, 20*time.Millisecond).Should(BeNumerically(">=", 2))
		})
	})

	Describe("reconnection", func() {
		It("reconnects when the server closes the connection", func() {
			var connectionCount atomic.Int32
			var subscriptions atomic.Int32
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())

				connNum := connectionCount.Add(1)

				// Read subscription message.
				_, msgData, readErr := conn.ReadMessage()
				if readErr != nil {
					conn.Close()
					return
				}

				if string(msgData) == "sor+{}" {
					subscriptions.Add(1)
				}

				if connNum == 1 {
					// Close the first connection to trigger a reconnect.
					conn.Close()
					return
				}

				// Keep the second connection alive.
				defer conn.Close()
				<-handlerDone
			}))
			DeferCleanup(func() {
				select {
				case <-handlerDone:
				default:
					close(handlerDone)
				}
			})
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			streamer := ibkr.NewOrderStreamerForTest(fills, wsServerURL(server))
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			// The streamer should reconnect and subscribe again.
			Eventually(func() int32 {
				return subscriptions.Load()
			}, 5*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 2))
		})

		It("polls for missed fills on reconnect when tradesFn is set", func() {
			var connectionCount atomic.Int32
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())

				connNum := connectionCount.Add(1)

				// Read subscription message.
				conn.ReadMessage()

				if connNum == 1 {
					// Close the first connection to trigger reconnect.
					conn.Close()
					return
				}

				defer conn.Close()
				<-handlerDone
			}))
			DeferCleanup(func() {
				select {
				case <-handlerDone:
				default:
					close(handlerDone)
				}
			})
			DeferCleanup(server.Close)

			tradesFn := func(_ context.Context) ([]ibkr.IBTradeEntry, error) {
				return []ibkr.IBTradeEntry{
					{OrderID: "MISSED-1", Price: 42.50, Quantity: 100},
				}, nil
			}

			fills := make(chan broker.Fill, 10)
			streamer := ibkr.NewOrderStreamerForTestWithTrades(fills, wsServerURL(server), tradesFn)
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var received broker.Fill
			Eventually(fills, 5*time.Second).Should(Receive(&received))
			Expect(received.OrderID).To(Equal("MISSED-1"))
			Expect(received.Price).To(Equal(42.50))
			Expect(received.Qty).To(Equal(100.0))
		})
	})
})
