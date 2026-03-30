package webull_test

import (
	"context"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/webull"
)

// Compile-time interface check.
var _ broker.Broker = (*webull.WebullBroker)(nil)

var _ = Describe("WebullBroker", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// authenticatedBroker builds a WebullBroker backed by an httptest.Server.
	authenticatedBroker := func(extraRoutes func(mux *http.ServeMux), opts ...webull.Option) *webull.WebullBroker {
		mux := http.NewServeMux()
		if extraRoutes != nil {
			extraRoutes(mux)
		}

		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		wb := webull.New(opts...)
		webull.SetClientForTest(wb, server.URL, "test-key", "test-secret")

		return wb
	}

	Describe("Connect", Label("auth"), func() {
		It("returns ErrMissingCredentials when no env vars are set", func() {
			originalAppKey := os.Getenv("WEBULL_APP_KEY")
			originalAppSecret := os.Getenv("WEBULL_APP_SECRET")
			originalClientID := os.Getenv("WEBULL_CLIENT_ID")
			originalClientSecret := os.Getenv("WEBULL_CLIENT_SECRET")
			os.Unsetenv("WEBULL_APP_KEY")
			os.Unsetenv("WEBULL_APP_SECRET")
			os.Unsetenv("WEBULL_CLIENT_ID")
			os.Unsetenv("WEBULL_CLIENT_SECRET")
			DeferCleanup(func() {
				restoreEnv("WEBULL_APP_KEY", originalAppKey)
				restoreEnv("WEBULL_APP_SECRET", originalAppSecret)
				restoreEnv("WEBULL_CLIENT_ID", originalClientID)
				restoreEnv("WEBULL_CLIENT_SECRET", originalClientSecret)
			})

			wb := webull.New()
			err := wb.Connect(ctx)
			Expect(err).To(MatchError(webull.ErrMissingCredentials))
		})
	})

	Describe("Close", func() {
		It("closes without error and closes the fills channel", func() {
			wb := authenticatedBroker(nil)

			err := wb.Close()
			Expect(err).ToNot(HaveOccurred())

			fills := wb.Fills()
			_, open := <-fills
			Expect(open).To(BeFalse())
		})
	})

	Describe("Submit", Label("orders"), func() {
		It("rejects unsupported time-in-force", func() {
			wb := authenticatedBroker(nil)

			err := wb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.IOC,
			})

			Expect(err).To(MatchError(webull.ErrUnsupportedTimeInForce))
		})

		It("rejects dollar-amount orders without WithFractionalShares", func() {
			wb := authenticatedBroker(nil)

			err := wb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      1000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).To(MatchError(webull.ErrFractionalNotEnabled))
		})

		It("rejects non-market dollar-amount orders", func() {
			wb := authenticatedBroker(nil, webull.WithFractionalShares())

			err := wb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      1000,
				OrderType:   broker.Limit,
				LimitPrice:  150.0,
				TimeInForce: broker.Day,
			})

			Expect(err).To(MatchError(webull.ErrFractionalNotMarket))
		})

		It("succeeds with a valid market order", func() {
			var submitCalled atomic.Int32

			wb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /api/trade/order/place", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"order_id": "ORD-001",
					})
				})
			})

			err := wb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(submitCalled.Load()).To(Equal(int32(1)))
		})

		It("succeeds with a fractional dollar-amount order", func() {
			var receivedBody map[string]any

			wb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /api/trade/order/place", func(writer http.ResponseWriter, req *http.Request) {
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&receivedBody)

					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"order_id": "ORD-002",
					})
				})
			}, webull.WithFractionalShares())

			err := wb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      500,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(receivedBody["notional"]).To(Equal("500"))
		})
	})

	Describe("Cancel", Label("orders"), func() {
		It("sends cancel request to the correct endpoint", func() {
			var cancelCalled atomic.Int32

			wb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /api/trade/order/cancel", func(writer http.ResponseWriter, req *http.Request) {
					cancelCalled.Add(1)

					writer.Header().Set("Content-Type", "application/json")
					writer.WriteHeader(http.StatusOK)
				})
			})

			err := wb.Cancel(ctx, "ORD-123")
			Expect(err).ToNot(HaveOccurred())
			Expect(cancelCalled.Load()).To(Equal(int32(1)))
		})
	})

	Describe("Replace", Label("orders"), func() {
		It("rejects when side differs from original", func() {
			wb := authenticatedBroker(func(mux *http.ServeMux) {
				// First submit an order so it is tracked.
				mux.HandleFunc("POST /api/trade/order/place", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"order_id": "ORD-100",
					})
				})
			})

			// Submit the original order.
			submitErr := wb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})
			Expect(submitErr).ToNot(HaveOccurred())

			// Try to replace with a different side.
			err := wb.Replace(ctx, "ORD-100", broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Sell,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(err).To(MatchError(webull.ErrReplaceFieldNotAllowed))
		})

		It("succeeds when only qty and price change", func() {
			var replaceCalled atomic.Int32

			wb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /api/trade/order/place", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"order_id": "ORD-200",
					})
				})

				mux.HandleFunc("POST /api/trade/order/replace", func(writer http.ResponseWriter, req *http.Request) {
					replaceCalled.Add(1)

					writer.Header().Set("Content-Type", "application/json")
					writer.WriteHeader(http.StatusOK)
				})
			})

			// Submit the original order.
			submitErr := wb.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Limit,
				LimitPrice:  150.0,
				TimeInForce: broker.Day,
			})
			Expect(submitErr).ToNot(HaveOccurred())

			// Replace with new qty and price.
			err := wb.Replace(ctx, "ORD-200", broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         20,
				OrderType:   broker.Limit,
				LimitPrice:  155.0,
				TimeInForce: broker.Day,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(replaceCalled.Load()).To(Equal(int32(1)))
		})
	})

	Describe("Orders", func() {
		It("returns mapped broker orders", func() {
			wb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /api/trade/order/list", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"orders": []map[string]any{
							{
								"order_id":         "ORD-300",
								"symbol":           "GOOG",
								"side":             "BUY",
								"order_status":     "FILLED",
								"order_type":       "LIMIT",
								"qty":              "5",
								"filled_qty":       "5",
								"filled_avg_price": "140.50",
								"limit_price":      "141",
								"stop_price":       "",
							},
						},
					})
				})
			})

			orders, err := wb.Orders(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].ID).To(Equal("ORD-300"))
			Expect(orders[0].Asset.Ticker).To(Equal("GOOG"))
			Expect(orders[0].Side).To(Equal(broker.Buy))
			Expect(orders[0].Status).To(Equal(broker.OrderFilled))
			Expect(orders[0].OrderType).To(Equal(broker.Limit))
			Expect(orders[0].Qty).To(Equal(5.0))
			Expect(orders[0].LimitPrice).To(Equal(141.0))
		})
	})

	Describe("Positions", func() {
		It("returns mapped positions", func() {
			wb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /api/trade/account/positions", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"positions": []map[string]any{
							{
								"symbol":        "NVDA",
								"qty":           "100",
								"avg_cost":      "400",
								"market_value":  "50000",
								"unrealized_pl": "10000",
							},
						},
					})
				})
			})

			positions, err := wb.Positions(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Asset.Ticker).To(Equal("NVDA"))
			Expect(positions[0].Qty).To(Equal(100.0))
			Expect(positions[0].AvgOpenPrice).To(Equal(400.0))
			Expect(positions[0].MarkPrice).To(Equal(500.0))
		})
	})

	Describe("Balance", func() {
		It("returns mapped balance", func() {
			wb := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /api/trade/account/detail", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"account_id":      "test-account",
						"net_liquidation": "100000",
						"cash_balance":    "25000",
						"buying_power":    "50000",
						"maintenance_req": "10000",
					})
				})
			})

			balance, err := wb.Balance(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(balance.NetLiquidatingValue).To(Equal(100000.0))
			Expect(balance.CashBalance).To(Equal(25000.0))
			Expect(balance.EquityBuyingPower).To(Equal(50000.0))
			Expect(balance.MaintenanceReq).To(Equal(10000.0))
		})
	})

	Describe("Transactions", func() {
		It("returns an empty slice", func() {
			wb := authenticatedBroker(nil)

			transactions, err := wb.Transactions(ctx, time.Time{})
			Expect(err).ToNot(HaveOccurred())
			Expect(transactions).To(BeNil())
		})
	})

	Describe("Fills", func() {
		It("returns a non-nil channel", func() {
			wb := webull.New()
			Expect(wb.Fills()).ToNot(BeNil())
		})
	})
})

// restoreEnv sets the environment variable back if it was previously non-empty,
// or unsets it if it was empty.
func restoreEnv(key, value string) {
	if value != "" {
		os.Setenv(key, value)
	} else {
		os.Unsetenv(key)
	}
}
