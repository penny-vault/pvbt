package schwab_test

import (
	"context"
	"encoding/json"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/schwab"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(req *http.Request) bool { return true },
}

var _ = Describe("activityStreamer", Label("streaming"), func() {
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

	streamerInfo := schwab.SchwabStreamerInfo{
		StreamerSocketURL:      "", // will be overridden
		SchwabClientCustomerID: "cust-123",
		SchwabClientCorrelID:   "correl-456",
		SchwabClientChannel:    "channel-A",
		SchwabClientFunctionID: "func-B",
	}

	Describe("LOGIN and SUBS", func() {
		It("sends a LOGIN command followed by a SUBS command after connecting", func() {
			loginReceived := make(chan map[string]any, 1)
			subsReceived := make(chan map[string]any, 1)
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				// Read LOGIN request.
				_, msgData, readErr := conn.ReadMessage()
				Expect(readErr).ToNot(HaveOccurred())
				var loginMsg map[string]any
				sonic.Unmarshal(msgData, &loginMsg)
				loginReceived <- loginMsg

				// Send LOGIN success response.
				loginResp := map[string]any{
					"response": []map[string]any{
						{
							"service": "ADMIN",
							"command": "LOGIN",
							"content": map[string]any{
								"code": 0,
								"msg":  "server is alive",
							},
						},
					},
				}
				conn.WriteJSON(loginResp)

				// Read SUBS request.
				_, msgData, readErr = conn.ReadMessage()
				Expect(readErr).ToNot(HaveOccurred())
				var subsMsg map[string]any
				sonic.Unmarshal(msgData, &subsMsg)
				subsReceived <- subsMsg

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := schwab.NewAPIClientForTest("http://unused.test", "test-token")
			info := streamerInfo
			info.StreamerSocketURL = wsServerURL(server)

			streamer := schwab.NewActivityStreamerForTest(client, fills, wsServerURL(server), info, "HASH-TEST", "access-token-123")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var loginMsg map[string]any
			Eventually(loginReceived, 3*time.Second).Should(Receive(&loginMsg))
			requests := loginMsg["requests"].([]any)
			firstRequest := requests[0].(map[string]any)
			Expect(firstRequest["service"]).To(Equal("ADMIN"))
			Expect(firstRequest["command"]).To(Equal("LOGIN"))
			params := firstRequest["parameters"].(map[string]any)
			Expect(params["Authorization"]).To(Equal("access-token-123"))

			var subsMsg map[string]any
			Eventually(subsReceived, 3*time.Second).Should(Receive(&subsMsg))
			requests = subsMsg["requests"].([]any)
			firstRequest = requests[0].(map[string]any)
			Expect(firstRequest["service"]).To(Equal("ACCT_ACTIVITY"))
			Expect(firstRequest["command"]).To(Equal("SUBS"))
		})

		It("returns ErrLoginDenied when the server rejects LOGIN", func() {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				conn.ReadMessage()

				loginResp := map[string]any{
					"response": []map[string]any{
						{
							"service": "ADMIN",
							"command": "LOGIN",
							"content": map[string]any{
								"code": 3,
								"msg":  "LOGIN_DENIED",
							},
						},
					},
				}
				conn.WriteJSON(loginResp)
			}))
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := schwab.NewAPIClientForTest("http://unused.test", "test-token")
			info := streamerInfo
			info.StreamerSocketURL = wsServerURL(server)

			streamer := schwab.NewActivityStreamerForTest(client, fills, wsServerURL(server), info, "HASH-TEST", "bad-token")
			connectErr := streamer.ConnectStreamer(ctx)
			Expect(connectErr).To(MatchError(schwab.ErrLoginDenied))
		})
	})

	Describe("Fill delivery", func() {
		It("emits a broker.Fill when an ACCT_ACTIVITY data message arrives", func() {
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				// Consume LOGIN.
				conn.ReadMessage()
				loginResp := map[string]any{
					"response": []map[string]any{
						{"service": "ADMIN", "command": "LOGIN", "content": map[string]any{"code": 0}},
					},
				}
				conn.WriteJSON(loginResp)

				// Consume SUBS.
				conn.ReadMessage()

				// Send an ACCT_ACTIVITY data message with fill info.
				activityMsg := map[string]any{
					"data": []map[string]any{
						{
							"service":   "ACCT_ACTIVITY",
							"timestamp": 1742480400000,
							"content": []map[string]any{
								{
									"1": "OrderFill",
									"2": "ACCT-123",
									"3": json.RawMessage(`{
										"orderId": 987654,
										"status": "FILLED",
										"orderActivityCollection": [
											{
												"activityType": "EXECUTION",
												"executionType": "FILL",
												"quantity": 50,
												"executionLegs": [
													{"price": 150.25, "quantity": 50, "time": "2026-03-20T14:30:00+0000"}
												]
											}
										]
									}`),
								},
							},
						},
					},
				}
				conn.WriteJSON(activityMsg)

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := schwab.NewAPIClientForTest("http://unused.test", "test-token")
			info := streamerInfo
			info.StreamerSocketURL = wsServerURL(server)

			streamer := schwab.NewActivityStreamerForTest(client, fills, wsServerURL(server), info, "HASH-TEST", "access-token")
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
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				conn.ReadMessage()
				conn.WriteJSON(map[string]any{
					"response": []map[string]any{
						{"service": "ADMIN", "command": "LOGIN", "content": map[string]any{"code": 0}},
					},
				})
				conn.ReadMessage()

				activityMsg := map[string]any{
					"data": []map[string]any{
						{
							"service":   "ACCT_ACTIVITY",
							"timestamp": 1742480400000,
							"content": []map[string]any{
								{
									"1": "OrderFill",
									"2": "ACCT-123",
									"3": json.RawMessage(`{
										"orderId": 111,
										"status": "FILLED",
										"orderActivityCollection": [
											{
												"activityType": "EXECUTION",
												"executionType": "FILL",
												"quantity": 10,
												"executionLegs": [
													{"price": 99.50, "quantity": 10, "time": "2026-03-20T14:30:00+0000"}
												]
											}
										]
									}`),
								},
							},
						},
					},
				}

				// Send the same fill twice.
				conn.WriteJSON(activityMsg)
				time.Sleep(50 * time.Millisecond)
				conn.WriteJSON(activityMsg)

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := schwab.NewAPIClientForTest("http://unused.test", "test-token")
			info := streamerInfo
			info.StreamerSocketURL = wsServerURL(server)

			streamer := schwab.NewActivityStreamerForTest(client, fills, wsServerURL(server), info, "HASH-TEST", "access-token")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())
			DeferCleanup(func() { streamer.CloseStreamer() })

			var firstFill broker.Fill
			Eventually(fills, 3*time.Second).Should(Receive(&firstFill))
			Expect(firstFill.OrderID).To(Equal("111"))

			Consistently(fills, 1*time.Second).ShouldNot(Receive())
		})
	})

	Describe("Shutdown", func() {
		It("stops the goroutine when CloseStreamer is called", func() {
			handlerDone := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				conn, upgradeErr := wsUpgrader.Upgrade(writer, req, nil)
				Expect(upgradeErr).ToNot(HaveOccurred())
				defer conn.Close()

				conn.ReadMessage()
				conn.WriteJSON(map[string]any{
					"response": []map[string]any{
						{"service": "ADMIN", "command": "LOGIN", "content": map[string]any{"code": 0}},
					},
				})
				conn.ReadMessage()

				<-handlerDone
			}))
			DeferCleanup(func() { close(handlerDone) })
			DeferCleanup(server.Close)

			fills := make(chan broker.Fill, 10)
			client := schwab.NewAPIClientForTest("http://unused.test", "test-token")
			info := streamerInfo
			info.StreamerSocketURL = wsServerURL(server)

			streamer := schwab.NewActivityStreamerForTest(client, fills, wsServerURL(server), info, "HASH-TEST", "access-token")
			Expect(streamer.ConnectStreamer(ctx)).To(Succeed())

			closeDone := make(chan error, 1)
			go func() {
				closeDone <- streamer.CloseStreamer()
			}()

			Eventually(closeDone, 3*time.Second).Should(Receive(Not(HaveOccurred())))
		})
	})
})
