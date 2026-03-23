package tradier_test

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
	"github.com/penny-vault/pvbt/broker/tradier"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(req *http.Request) bool { return true },
}

func wsURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

var _ = Describe("accountStreamer", Label("streaming"), func() {
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

	Describe("WebSocket connection", func() {
		It("connects to the URL and sends a subscription JSON with session ID", func() {
			subReceived := make(chan map[string]any, 1)
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				_, msgData, readErr := conn.ReadMessage()
				Expect(readErr).ToNot(HaveOccurred())

				var sub map[string]any
				Expect(json.Unmarshal(msgData, &sub)).To(Succeed())
				subReceived <- sub

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradier.NewAPIClientForTest(server.URL, "test-token", "acct-1")
			streamer := tradier.NewAccountStreamerForTest(client, fills, wsURL(server), "sess-abc", false)

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var sub map[string]any
			Eventually(subReceived, 3*time.Second).Should(Receive(&sub))
			Expect(sub["sessionid"]).To(Equal("sess-abc"))
			events, ok := sub["events"].([]any)
			Expect(ok).To(BeTrue())
			Expect(events).To(ContainElement("order"))
		})
	})

	Describe("Fill detection", func() {
		buildServer := func(eventJSON []byte) (*httptest.Server, chan struct{}) {
			handlerDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				// Consume subscription message.
				conn.ReadMessage()

				conn.WriteMessage(websocket.TextMessage, eventJSON)
				<-handlerDone
			}))
			return server, handlerDone
		}

		It("delivers a broker.Fill when status=filled", func() {
			event := tradierAccountEvent{
				ID:               101,
				Event:            "order",
				Status:           "filled",
				AvgFillPrice:     150.50,
				LastFillPrice:    150.50,
				LastFillQuantity: 10.0,
				TransactionDate:  "2026-03-22T15:30:00Z",
			}
			eventJSON, _ := json.Marshal(event)
			server, handlerDone := buildServer(eventJSON)
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradier.NewAPIClientForTest(server.URL, "test-token", "acct-1")
			streamer := tradier.NewAccountStreamerForTest(client, fills, wsURL(server), "sess-1", false)

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var fill broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("101"))
			Expect(fill.Price).To(Equal(150.50))
			Expect(fill.Qty).To(Equal(10.0))
		})

		It("delivers a broker.Fill when status=partially_filled", func() {
			event := tradierAccountEvent{
				ID:               202,
				Event:            "order",
				Status:           "partially_filled",
				AvgFillPrice:     99.00,
				LastFillPrice:    99.00,
				LastFillQuantity: 5.0,
				TransactionDate:  "2026-03-22T16:00:00Z",
			}
			eventJSON, _ := json.Marshal(event)
			server, handlerDone := buildServer(eventJSON)
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradier.NewAPIClientForTest(server.URL, "test-token", "acct-1")
			streamer := tradier.NewAccountStreamerForTest(client, fills, wsURL(server), "sess-2", false)

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var fill broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("202"))
		})

		It("does not deliver a fill when status=open", func() {
			event := tradierAccountEvent{
				ID:     303,
				Event:  "order",
				Status: "open",
			}
			eventJSON, _ := json.Marshal(event)
			server, handlerDone := buildServer(eventJSON)
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradier.NewAPIClientForTest(server.URL, "test-token", "acct-1")
			streamer := tradier.NewAccountStreamerForTest(client, fills, wsURL(server), "sess-3", false)

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			Consistently(fills, 500*time.Millisecond).ShouldNot(Receive())
		})
	})

	Describe("Deduplication", func() {
		It("delivers only one fill when the same fill event is received twice", func() {
			handlerDone := make(chan struct{})

			event := tradierAccountEvent{
				ID:               404,
				Event:            "order",
				Status:           "filled",
				AvgFillPrice:     200.00,
				LastFillQuantity: 20.0,
				TransactionDate:  "2026-03-22T17:00:00Z",
			}
			eventJSON, _ := json.Marshal(event)

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				conn.ReadMessage()

				conn.WriteMessage(websocket.TextMessage, eventJSON)
				time.Sleep(50 * time.Millisecond)
				conn.WriteMessage(websocket.TextMessage, eventJSON)

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradier.NewAPIClientForTest(server.URL, "test-token", "acct-1")
			streamer := tradier.NewAccountStreamerForTest(client, fills, wsURL(server), "sess-4", false)

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var firstFill broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&firstFill))
			Expect(firstFill.OrderID).To(Equal("404"))

			Consistently(fills, 1*time.Second).ShouldNot(Receive())
		})
	})

	Describe("Close", func() {
		It("stops the read loop and returns without error", func() {
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				conn.ReadMessage()
				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradier.NewAPIClientForTest(server.URL, "test-token", "acct-1")
			streamer := tradier.NewAccountStreamerForTest(client, fills, wsURL(server), "sess-5", false)

			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())

			closeDone := make(chan error, 1)
			go func() {
				closeDone <- streamer.CloseStreamer()
			}()

			Eventually(closeDone, 3*time.Second).Should(Receive(Not(HaveOccurred())))
		})
	})

	Describe("Sandbox polling", func() {
		It("detects new fills by comparing against previous order state and delivers fills", func() {
			callCount := 0

			// First call: order is open. Second call: order is filled.
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")

				callCount++

				var orders []tradierOrderResponse
				if callCount >= 2 {
					orders = []tradierOrderResponse{
						{
							ID:               999,
							Status:           "filled",
							AvgFillPrice:     55.50,
							ExecQuantity:     7.0,
							LastFillPrice:    55.50,
							LastFillQuantity: 7.0,
							TransactionDate:  "2026-03-22T18:00:00Z",
						},
					}
				} else {
					orders = []tradierOrderResponse{
						{
							ID:     999,
							Status: "open",
						},
					}
				}

				resp := map[string]any{
					"orders": map[string]any{
						"order": orders,
					},
				}
				json.NewEncoder(writer).Encode(resp)
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := tradier.NewAPIClientForTest(server.URL, "test-token", "acct-1")
			streamer := tradier.NewAccountStreamerForTest(client, fills, "", "sess-sandbox", true)

			streamer.StartPollingForTest(ctx)
			DeferCleanup(func() { streamer.CloseStreamer() })

			var fill broker.Fill
			Eventually(fills, 5*time.Second).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("999"))
			Expect(fill.Price).To(Equal(55.50))
			Expect(fill.Qty).To(Equal(7.0))
		})
	})
})

// tradierAccountEvent is a local duplicate for use in test JSON construction.
// The real type is unexported in the production package.
type tradierAccountEvent struct {
	ID               int64   `json:"id"`
	Event            string  `json:"event"`
	Status           string  `json:"status"`
	AvgFillPrice     float64 `json:"avg_fill_price"`
	LastFillQuantity float64 `json:"last_fill_quantity"`
	LastFillPrice    float64 `json:"last_fill_price"`
	TransactionDate  string  `json:"transaction_date"`
}

// tradierOrderResponse is a local duplicate for use in test JSON construction.
type tradierOrderResponse struct {
	ID                int64   `json:"id"`
	Status            string  `json:"status"`
	AvgFillPrice      float64 `json:"avg_fill_price"`
	ExecQuantity      float64 `json:"exec_quantity"`
	LastFillPrice     float64 `json:"last_fill_price"`
	LastFillQuantity  float64 `json:"last_fill_quantity"`
	TransactionDate   string  `json:"transaction_date"`
}
