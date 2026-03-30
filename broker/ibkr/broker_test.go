package ibkr_test

import (
	"context"
	"github.com/bytedance/sonic"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/ibkr"
)

// Compile-time interface checks.
var _ broker.Broker = (*ibkr.IBBroker)(nil)
var _ broker.GroupSubmitter = (*ibkr.IBBroker)(nil)

var _ = Describe("IBBroker", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	authenticatedBroker := func(extraRoutes func(mux *http.ServeMux)) *ibkr.IBBroker {
		mux := http.NewServeMux()
		if extraRoutes != nil {
			extraRoutes(mux)
		}

		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)

		ib := ibkr.New(ibkr.WithGateway(server.URL))
		client := ibkr.NewAPIClientForTest(server.URL)
		ibkr.SetClientForTest(ib, client)
		ibkr.SetAccountIDForTest(ib, "U1234567")

		return ib
	}

	Describe("Submit", Label("orders"), func() {
		It("resolves conid via secdef search and submits order", func() {
			var submitCalled atomic.Int32
			var receivedBody []map[string]any

			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBSecdefResult{
						{Conid: 265598, CompanyName: "Apple Inc", Ticker: "AAPL"},
					})
				})

				mux.HandleFunc("POST /iserver/account/U1234567/orders", func(writer http.ResponseWriter, req *http.Request) {
					submitCalled.Add(1)
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBOrderReply{
						{OrderID: "ORD-1", OrderStatus: "PreSubmitted"},
					})
				})
			})

			submitErr := ib.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(submitCalled.Load()).To(Equal(int32(1)))
			Expect(receivedBody).To(HaveLen(1))
			Expect(receivedBody[0]["conid"]).To(BeNumerically("==", 265598))
			Expect(receivedBody[0]["side"]).To(Equal("BUY"))
			Expect(receivedBody[0]["orderType"]).To(Equal("MKT"))
		})

		It("caches conid across calls", func() {
			var secdefCalls atomic.Int32

			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
					secdefCalls.Add(1)
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBSecdefResult{
						{Conid: 265598, CompanyName: "Apple Inc", Ticker: "AAPL"},
					})
				})

				mux.HandleFunc("POST /iserver/account/U1234567/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBOrderReply{
						{OrderID: "ORD-1", OrderStatus: "PreSubmitted"},
					})
				})
			})

			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			Expect(ib.Submit(ctx, order)).To(Succeed())
			Expect(ib.Submit(ctx, order)).To(Succeed())
			Expect(secdefCalls.Load()).To(Equal(int32(1)))
		})

		It("returns ErrConidNotFound for unknown symbol", func() {
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBSecdefResult{})
				})
			})

			submitErr := ib.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "ZZZZZ"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(submitErr).To(MatchError(ibkr.ErrConidNotFound))
		})

		It("returns error for unsupported GTD time-in-force", func() {
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBSecdefResult{
						{Conid: 265598, CompanyName: "Apple Inc", Ticker: "AAPL"},
					})
				})
			})

			submitErr := ib.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.GTD,
			})

			Expect(submitErr).To(HaveOccurred())
			Expect(submitErr).To(MatchError(ContainSubstring("unsupported time-in-force")))
		})

		It("converts dollar-amount orders to share quantity", func() {
			var submittedQty float64

			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBSecdefResult{
						{Conid: 76792991, CompanyName: "Tesla Inc", Ticker: "TSLA"},
					})
				})

				mux.HandleFunc("GET /iserver/marketdata/snapshot", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]map[string]any{
						{"conid": 76792991, "31": "200.00"},
					})
				})

				mux.HandleFunc("POST /iserver/account/U1234567/orders", func(writer http.ResponseWriter, req *http.Request) {
					var body []map[string]any
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&body)
					submittedQty = body[0]["quantity"].(float64)
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBOrderReply{
						{OrderID: "ORD-AMT-1", OrderStatus: "PreSubmitted"},
					})
				})
			})

			submitErr := ib.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "TSLA"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      5000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(submittedQty).To(Equal(25.0)) // floor(5000 / 200) = 25
		})

		It("auto-confirms when orders endpoint returns a reply ID", func() {
			var confirmCalled atomic.Int32

			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBSecdefResult{
						{Conid: 265598, CompanyName: "Apple Inc", Ticker: "AAPL"},
					})
				})

				mux.HandleFunc("POST /iserver/account/U1234567/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBOrderReply{
						{ReplyID: "reply-abc-123", Message: []string{"Are you sure?"}},
					})
				})

				mux.HandleFunc("POST /iserver/reply/reply-abc-123", func(writer http.ResponseWriter, req *http.Request) {
					confirmCalled.Add(1)
					var body map[string]bool
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&body)
					Expect(body["confirmed"]).To(BeTrue())
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBOrderReply{
						{OrderID: "ORD-CONFIRMED", OrderStatus: "PreSubmitted"},
					})
				})
			})

			submitErr := ib.Submit(ctx, broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(confirmCalled.Load()).To(Equal(int32(1)))
		})
	})

	Describe("Cancel", Label("orders"), func() {
		It("sends DELETE to cancel the order", func() {
			var cancelCalled atomic.Int32

			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("DELETE /iserver/account/U1234567/order/ORD-CANCEL-1", func(writer http.ResponseWriter, req *http.Request) {
					cancelCalled.Add(1)
					writer.WriteHeader(http.StatusOK)
				})
			})

			cancelErr := ib.Cancel(ctx, "ORD-CANCEL-1")
			Expect(cancelErr).ToNot(HaveOccurred())
			Expect(cancelCalled.Load()).To(Equal(int32(1)))
		})
	})

	Describe("Orders", func() {
		It("returns mapped orders", func() {
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /iserver/account/orders", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(map[string]any{
						"orders": []map[string]any{
							{
								"orderId":           "ORD-100",
								"status":            "Submitted",
								"side":              "BUY",
								"orderType":         "LMT",
								"ticker":            "GOOG",
								"conid":             208813720,
								"filledQuantity":    0.0,
								"remainingQuantity": 15.0,
								"totalQuantity":     15.0,
							},
						},
					})
				})
			})

			orders, getErr := ib.Orders(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(orders).To(HaveLen(1))
			Expect(orders[0].ID).To(Equal("ORD-100"))
			Expect(orders[0].Asset.Ticker).To(Equal("GOOG"))
			Expect(orders[0].Qty).To(Equal(15.0))
			Expect(orders[0].Status).To(Equal(broker.OrderOpen))
			Expect(orders[0].OrderType).To(Equal(broker.Limit))
		})
	})

	Describe("Positions", func() {
		It("returns mapped positions", func() {
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /portfolio/U1234567/positions/0", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBPositionEntry{
						{
							ContractId: 265598,
							Position:   200,
							AvgCost:    150.50,
							MktPrice:   155.25,
							Ticker:     "AAPL",
							Currency:   "USD",
						},
					})
				})
			})

			positions, getErr := ib.Positions(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(positions).To(HaveLen(1))
			Expect(positions[0].Asset.Ticker).To(Equal("AAPL"))
			Expect(positions[0].Qty).To(Equal(200.0))
			Expect(positions[0].AvgOpenPrice).To(Equal(150.50))
			Expect(positions[0].MarkPrice).To(Equal(155.25))
		})
	})

	Describe("Balance", func() {
		It("returns mapped balance", func() {
			ib := authenticatedBroker(func(mux *http.ServeMux) {
				mux.HandleFunc("GET /portfolio/U1234567/summary", func(writer http.ResponseWriter, req *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode(ibkr.IBAccountSummary{
						CashBalance:    ibkr.SummaryValue{Amount: 50000.0},
						NetLiquidation: ibkr.SummaryValue{Amount: 125000.0},
						BuyingPower:    ibkr.SummaryValue{Amount: 100000.0},
						MaintMarginReq: ibkr.SummaryValue{Amount: 25000.0},
					})
				})
			})

			balance, getErr := ib.Balance(ctx)
			Expect(getErr).ToNot(HaveOccurred())
			Expect(balance.CashBalance).To(Equal(50000.0))
			Expect(balance.NetLiquidatingValue).To(Equal(125000.0))
			Expect(balance.EquityBuyingPower).To(Equal(100000.0))
			Expect(balance.MaintenanceReq).To(Equal(25000.0))
		})
	})

	Describe("SubmitGroup", Label("orders"), func() {
		secdefHandler := func(mux *http.ServeMux) {
			mux.HandleFunc("POST /iserver/secdef/search", func(writer http.ResponseWriter, req *http.Request) {
				var body map[string]string
				sonic.ConfigDefault.NewDecoder(req.Body).Decode(&body)
				writer.Header().Set("Content-Type", "application/json")
				sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBSecdefResult{
					{Conid: 756733, CompanyName: "SPDR S&P 500", Ticker: body["symbol"]},
				})
			})
		}

		It("submits a bracket order with parentId linking", func() {
			var receivedBody []map[string]any

			ib := authenticatedBroker(func(mux *http.ServeMux) {
				secdefHandler(mux)

				mux.HandleFunc("POST /iserver/account/U1234567/orders", func(writer http.ResponseWriter, req *http.Request) {
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBOrderReply{
						{OrderID: "BRACKET-1", OrderStatus: "PreSubmitted"},
					})
				})
			})

			submitErr := ib.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Stop, StopPrice: 430.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}, broker.GroupBracket)

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(receivedBody).To(HaveLen(3))

			// Entry order has a cOID set.
			entryCOID, hasCOID := receivedBody[0]["cOID"].(string)
			Expect(hasCOID).To(BeTrue())
			Expect(entryCOID).ToNot(BeEmpty())

			// Children reference entry via parentId.
			Expect(receivedBody[1]["parentId"]).To(Equal(entryCOID))
			Expect(receivedBody[2]["parentId"]).To(Equal(entryCOID))
		})

		It("submits an OCA order with shared ocaGroup", func() {
			var receivedBody []map[string]any

			ib := authenticatedBroker(func(mux *http.ServeMux) {
				secdefHandler(mux)

				mux.HandleFunc("POST /iserver/account/U1234567/orders", func(writer http.ResponseWriter, req *http.Request) {
					sonic.ConfigDefault.NewDecoder(req.Body).Decode(&receivedBody)
					writer.Header().Set("Content-Type", "application/json")
					sonic.ConfigDefault.NewEncoder(writer).Encode([]ibkr.IBOrderReply{
						{OrderID: "OCA-1", OrderStatus: "PreSubmitted"},
					})
				})
			})

			submitErr := ib.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 10, OrderType: broker.Limit, LimitPrice: 440.0, TimeInForce: broker.Day},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC},
			}, broker.GroupOCO)

			Expect(submitErr).ToNot(HaveOccurred())
			Expect(receivedBody).To(HaveLen(2))

			// Both orders share the same ocaGroup.
			ocaGroup0, hasOCA0 := receivedBody[0]["ocaGroup"].(string)
			ocaGroup1, hasOCA1 := receivedBody[1]["ocaGroup"].(string)
			Expect(hasOCA0).To(BeTrue())
			Expect(hasOCA1).To(BeTrue())
			Expect(ocaGroup0).To(Equal(ocaGroup1))
			Expect(ocaGroup0).ToNot(BeEmpty())

			// ocaType should be 1 (cancel-remaining).
			Expect(receivedBody[0]["ocaType"]).To(BeNumerically("==", 1))
			Expect(receivedBody[1]["ocaType"]).To(BeNumerically("==", 1))
		})

		It("returns ErrEmptyOrderGroup for empty slice", func() {
			ib := authenticatedBroker(nil)
			submitErr := ib.SubmitGroup(ctx, []broker.Order{}, broker.GroupOCO)
			Expect(submitErr).To(MatchError(broker.ErrEmptyOrderGroup))
		})

		It("returns ErrNoEntryOrder when bracket has no entry", func() {
			ib := authenticatedBroker(nil)
			submitErr := ib.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}, broker.GroupBracket)
			Expect(submitErr).To(MatchError(broker.ErrNoEntryOrder))
		})

		It("returns ErrMultipleEntryOrders when bracket has two entries", func() {
			ib := authenticatedBroker(nil)
			submitErr := ib.SubmitGroup(ctx, []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
			}, broker.GroupBracket)
			Expect(submitErr).To(MatchError(broker.ErrMultipleEntryOrders))
		})
	})

	Describe("Close", func() {
		It("closes without error when no streamer is connected", func() {
			ib := ibkr.New()
			closeErr := ib.Close()
			Expect(closeErr).ToNot(HaveOccurred())
		})
	})
})
