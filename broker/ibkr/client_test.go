package ibkr_test

import (
	"context"
	"errors"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/ibkr"
)

var _ = Describe("Client", func() {
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

	Describe("resolveAccount", func() {
		It("returns the first account ID", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /iserver/accounts", func(writer http.ResponseWriter, req *http.Request) {
				sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
					"accounts": []string{"U1234567"},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			accountID, resolveErr := client.ResolveAccount(ctx)
			Expect(resolveErr).ToNot(HaveOccurred())
			Expect(accountID).To(Equal("U1234567"))
		})
	})

	Describe("submitOrder", func() {
		It("posts order array and parses reply", func() {
			var capturedBody []map[string]any
			mux := http.NewServeMux()
			mux.HandleFunc("POST /iserver/account/U123/orders", func(writer http.ResponseWriter, req *http.Request) {
				sonic.ConfigDefault.NewDecoder(req.Body).Decode(&capturedBody)
				sonic.ConfigDefault.NewEncoder(writer).Encode([]map[string]any{
					{"order_id": "resp-1", "order_status": "PreSubmitted"},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			orders := []ibkr.IBOrderRequest{{Conid: 265598, OrderType: "MKT", Side: "BUY", Quantity: 100, Tif: "DAY"}}
			replies, submitErr := client.SubmitOrder(ctx, "U123", orders)
			Expect(submitErr).ToNot(HaveOccurred())
			Expect(replies).To(HaveLen(1))
			Expect(capturedBody).To(HaveLen(1))
			Expect(capturedBody[0]["side"]).To(Equal("BUY"))
		})
	})

	Describe("cancelOrder", func() {
		It("sends DELETE for the order", func() {
			var deletedPath string
			mux := http.NewServeMux()
			mux.HandleFunc("DELETE /iserver/account/U123/order/", func(writer http.ResponseWriter, req *http.Request) {
				deletedPath = req.URL.Path
				sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{"msg": "cancelled"})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			Expect(client.CancelOrder(ctx, "U123", "order-42")).To(Succeed())
			Expect(deletedPath).To(ContainSubstring("order-42"))
		})
	})

	Describe("getPositions", func() {
		It("fetches positions from portfolio endpoint", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /portfolio/U123/positions/0", func(writer http.ResponseWriter, req *http.Request) {
				sonic.ConfigDefault.NewEncoder(writer).Encode([]map[string]any{
					{"contractId": 265598, "position": 100.0, "avgCost": 150.50, "mktPrice": 155.0, "ticker": "AAPL", "currency": "USD"},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			positions, posErr := client.GetPositions(ctx, "U123")
			Expect(posErr).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Position).To(BeNumerically("==", 100))
		})
	})

	Describe("getBalance", func() {
		It("fetches summary from portfolio endpoint", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /portfolio/U123/summary", func(writer http.ResponseWriter, req *http.Request) {
				sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
					"cashbalance":    map[string]any{"amount": 50000.0},
					"netliquidation": map[string]any{"amount": 150000.0},
					"buyingpower":    map[string]any{"amount": 200000.0},
					"maintmarginreq": map[string]any{"amount": 75000.0},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			summary, balErr := client.GetBalance(ctx, "U123")
			Expect(balErr).ToNot(HaveOccurred())
			Expect(summary.CashBalance.Amount).To(BeNumerically("==", 50000))
		})
	})

	Describe("searchSecdef", func() {
		It("resolves ticker to conid", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
				var body map[string]string
				sonic.ConfigDefault.NewDecoder(req.Body).Decode(&body)
				Expect(body["symbol"]).To(Equal("AAPL"))
				sonic.ConfigDefault.NewEncoder(writer).Encode([]map[string]any{
					{"conid": 265598, "companyName": "APPLE INC", "ticker": "AAPL"},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			results, searchErr := client.SearchSecdef(ctx, "AAPL")
			Expect(searchErr).ToNot(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results[0].Conid).To(Equal(int64(265598)))
		})
	})

	Describe("getSnapshot", func() {
		It("fetches last price for conid", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /iserver/marketdata/snapshot", func(writer http.ResponseWriter, req *http.Request) {
				Expect(req.URL.Query().Get("conids")).To(Equal("265598"))
				Expect(req.URL.Query().Get("fields")).To(Equal("31"))
				sonic.ConfigDefault.NewEncoder(writer).Encode([]map[string]any{
					{"conid": 265598, "31": "155.25"},
				})
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			lastPrice, snapErr := client.GetSnapshot(ctx, 265598)
			Expect(snapErr).ToNot(HaveOccurred())
			Expect(lastPrice).To(BeNumerically("==", 155.25))
		})
	})

	Describe("error handling", func() {
		It("returns ErrRateLimited on HTTP 429", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /iserver/accounts", func(writer http.ResponseWriter, req *http.Request) {
				writer.WriteHeader(http.StatusTooManyRequests)
				writer.Write([]byte(`{"error": "rate limited"}`))
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			_, resolveErr := client.ResolveAccount(ctx)
			Expect(resolveErr).To(MatchError(broker.ErrRateLimited))
		})

		It("returns HTTPError on HTTP 500", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /iserver/accounts", func(writer http.ResponseWriter, req *http.Request) {
				writer.WriteHeader(http.StatusInternalServerError)
				writer.Write([]byte(`{"error": "internal"}`))
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client := ibkr.NewAPIClientForTest(server.URL)
			_, resolveErr := client.ResolveAccount(ctx)
			var httpErr *ibkr.HTTPError
			Expect(errors.As(resolveErr, &httpErr)).To(BeTrue())
			Expect(httpErr.StatusCode).To(Equal(500))
		})
	})
})
