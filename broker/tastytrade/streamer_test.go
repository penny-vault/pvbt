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
			filledAt := time.Date(2026, 3, 20, 14, 30, 0, 0, time.UTC)

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				event := tastytrade.FillEvent{
					OrderID:  "ORD-100",
					FillID:   "FILL-1",
					Price:    150.25,
					Quantity: 50,
					FilledAt: filledAt,
				}

				payload, marshalErr := json.Marshal(event)
				Expect(marshalErr).ToNot(HaveOccurred())
				conn.WriteMessage(websocket.TextMessage, payload)

				// Keep connection open until test completes.
				<-ctx.Done()
			}))
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
			Expect(received.FilledAt).To(Equal(filledAt))
		})
	})

	Describe("Deduplication", Label("streaming"), func() {
		It("delivers only one fill when the same fill ID is sent twice", func() {
			filledAt := time.Date(2026, 3, 20, 14, 30, 0, 0, time.UTC)

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				event := tastytrade.FillEvent{
					OrderID:  "ORD-200",
					FillID:   "FILL-DUP",
					Price:    99.50,
					Quantity: 10,
					FilledAt: filledAt,
				}

				payload, marshalErr := json.Marshal(event)
				Expect(marshalErr).ToNot(HaveOccurred())

				// Send the same fill twice.
				conn.WriteMessage(websocket.TextMessage, payload)
				time.Sleep(50 * time.Millisecond)
				conn.WriteMessage(websocket.TextMessage, payload)

				<-ctx.Done()
			}))
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
			filledAt := time.Date(2026, 3, 20, 15, 0, 0, 0, time.UTC)

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				firstPartial := tastytrade.FillEvent{
					OrderID:  "ORD-300",
					FillID:   "FILL-A",
					Price:    200.00,
					Quantity: 30,
					FilledAt: filledAt,
				}

				secondPartial := tastytrade.FillEvent{
					OrderID:  "ORD-300",
					FillID:   "FILL-B",
					Price:    200.10,
					Quantity: 70,
					FilledAt: filledAt.Add(time.Second),
				}

				firstPayload, _ := json.Marshal(firstPartial)
				secondPayload, _ := json.Marshal(secondPartial)

				conn.WriteMessage(websocket.TextMessage, firstPayload)
				time.Sleep(50 * time.Millisecond)
				conn.WriteMessage(websocket.TextMessage, secondPayload)

				<-ctx.Done()
			}))
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

	Describe("Shutdown", Label("streaming"), func() {
		It("stops the goroutine when CloseStreamer is called", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				<-ctx.Done()
			}))
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
