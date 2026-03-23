package ibkr_test

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
	"github.com/penny-vault/pvbt/broker/ibkr"
)

var _ = Describe("Auth", func() {
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

	Describe("GatewayAuthenticator", func() {
		It("verifies an active session on Init", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /iserver/auth/status", func(writer http.ResponseWriter, req *http.Request) {
				json.NewEncoder(writer).Encode(map[string]any{
					"authenticated": true,
					"connected":     true,
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			auth := ibkr.NewGatewayAuthenticatorForTest(server.URL)
			Expect(auth.InitAuth(ctx)).To(Succeed())
		})

		It("calls reauthenticate when session is not active", func() {
			var reauthCalled atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("POST /iserver/auth/status", func(writer http.ResponseWriter, req *http.Request) {
				json.NewEncoder(writer).Encode(map[string]any{
					"authenticated": false,
					"connected":     false,
				})
			})
			mux.HandleFunc("POST /iserver/reauthenticate", func(writer http.ResponseWriter, req *http.Request) {
				reauthCalled.Add(1)
				json.NewEncoder(writer).Encode(map[string]any{"message": "triggered"})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			auth := ibkr.NewGatewayAuthenticatorForTest(server.URL)
			initErr := auth.InitAuth(ctx)
			Expect(initErr).To(MatchError(broker.ErrNotAuthenticated))
			Expect(reauthCalled.Load()).To(Equal(int32(1)))
		})

		It("Decorate is a no-op", func() {
			auth := ibkr.NewGatewayAuthenticatorForTest("http://localhost:5000")
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			Expect(auth.DecorateRequest(req)).To(Succeed())
			Expect(req.Header.Get("Authorization")).To(BeEmpty())
		})

		It("sends POST /tickle periodically during Keepalive", func() {
			var tickleCalls atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("POST /iserver/auth/status", func(writer http.ResponseWriter, req *http.Request) {
				json.NewEncoder(writer).Encode(map[string]any{"authenticated": true, "connected": true})
			})
			mux.HandleFunc("POST /tickle", func(writer http.ResponseWriter, req *http.Request) {
				tickleCalls.Add(1)
				writer.WriteHeader(http.StatusOK)
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			auth := ibkr.NewGatewayAuthenticatorForTest(server.URL)
			ibkr.SetGatewayTickleIntervalForTest(auth, 100*time.Millisecond)
			Expect(auth.InitAuth(ctx)).To(Succeed())

			keepaliveCtx, keepaliveCancel := context.WithCancel(ctx)
			go auth.RunKeepalive(keepaliveCtx)
			DeferCleanup(keepaliveCancel)

			Eventually(func() int32 { return tickleCalls.Load() }, 2*time.Second).Should(BeNumerically(">=", 2))
		})
	})
})
