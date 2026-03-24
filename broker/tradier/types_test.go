package tradier_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/tradier"
)

var _ = Describe("Types", Label("translation"), func() {
	Describe("unmarshalFlexible", func() {
		type testItem struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}

		It("handles a single object (not wrapped in array)", func() {
			raw := json.RawMessage(`{"name":"alpha","value":1}`)
			result, err := tradier.UnmarshalFlexible[testItem](raw)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("alpha"))
			Expect(result[0].Value).To(Equal(1))
		})

		It("handles an array with multiple objects", func() {
			raw := json.RawMessage(`[{"name":"alpha","value":1},{"name":"beta","value":2}]`)
			result, err := tradier.UnmarshalFlexible[testItem](raw)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(result[0].Name).To(Equal("alpha"))
			Expect(result[1].Name).To(Equal("beta"))
		})

		It("returns empty slice for null", func() {
			raw := json.RawMessage(`null`)
			result, err := tradier.UnmarshalFlexible[testItem](raw)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("returns empty slice for empty RawMessage", func() {
			raw := json.RawMessage(nil)
			result, err := tradier.UnmarshalFlexible[testItem](raw)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Describe("toTradierOrderParams", func() {
		It("translates a Market/Buy/Day order to correct form params", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			params, err := tradier.ToTradierOrderParams(order)
			Expect(err).ToNot(HaveOccurred())
			Expect(params.Get("symbol")).To(Equal("AAPL"))
			Expect(params.Get("side")).To(Equal("buy"))
			Expect(params.Get("quantity")).To(Equal("10"))
			Expect(params.Get("type")).To(Equal("market"))
			Expect(params.Get("duration")).To(Equal("day"))
			Expect(params.Get("price")).To(BeEmpty())
			Expect(params.Get("stop")).To(BeEmpty())
		})

		It("translates a Limit order and includes price param", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Sell,
				Qty:         5,
				OrderType:   broker.Limit,
				LimitPrice:  300.0,
				TimeInForce: broker.GTC,
			}

			params, err := tradier.ToTradierOrderParams(order)
			Expect(err).ToNot(HaveOccurred())
			Expect(params.Get("type")).To(Equal("limit"))
			Expect(params.Get("price")).To(Equal("300"))
			Expect(params.Get("stop")).To(BeEmpty())
			Expect(params.Get("duration")).To(Equal("gtc"))
		})

		It("translates a Stop order and includes stop param", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "TSLA"},
				Side:        broker.Sell,
				Qty:         2,
				OrderType:   broker.Stop,
				StopPrice:   250.0,
				TimeInForce: broker.Day,
			}

			params, err := tradier.ToTradierOrderParams(order)
			Expect(err).ToNot(HaveOccurred())
			Expect(params.Get("type")).To(Equal("stop"))
			Expect(params.Get("stop")).To(Equal("250"))
			Expect(params.Get("price")).To(BeEmpty())
		})

		It("translates a StopLimit order and includes both price and stop params", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "GOOG"},
				Side:        broker.Buy,
				Qty:         3,
				OrderType:   broker.StopLimit,
				StopPrice:   140.0,
				LimitPrice:  145.0,
				TimeInForce: broker.Day,
			}

			params, err := tradier.ToTradierOrderParams(order)
			Expect(err).ToNot(HaveOccurred())
			Expect(params.Get("type")).To(Equal("stop_limit"))
			Expect(params.Get("stop")).To(Equal("140"))
			Expect(params.Get("price")).To(Equal("145"))
		})

		It("returns error for IOC time-in-force", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.IOC,
			}
			_, err := tradier.ToTradierOrderParams(order)
			Expect(err).To(HaveOccurred())
		})

		It("returns error for FOK time-in-force", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.FOK,
			}
			_, err := tradier.ToTradierOrderParams(order)
			Expect(err).To(HaveOccurred())
		})

		It("returns error for GTD time-in-force", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.GTD,
			}
			_, err := tradier.ToTradierOrderParams(order)
			Expect(err).To(HaveOccurred())
		})

		It("returns error for OnOpen time-in-force", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.OnOpen,
			}
			_, err := tradier.ToTradierOrderParams(order)
			Expect(err).To(HaveOccurred())
		})

		It("returns error for OnClose time-in-force", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.OnClose,
			}
			_, err := tradier.ToTradierOrderParams(order)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("toBrokerOrder", func() {
		It("maps all Tradier statuses correctly", func() {
			for _, tc := range []struct {
				status   string
				expected broker.OrderStatus
			}{
				{"pending", broker.OrderSubmitted},
				{"open", broker.OrderOpen},
				{"partially_filled", broker.OrderPartiallyFilled},
				{"filled", broker.OrderFilled},
				{"expired", broker.OrderCancelled},
				{"canceled", broker.OrderCancelled},
				{"rejected", broker.OrderCancelled},
			} {
				resp := tradier.TradierOrderResponse{Status: tc.status}
				result := tradier.ToBrokerOrder(resp)
				Expect(result.Status).To(Equal(tc.expected), "for status %q", tc.status)
			}
		})

		It("maps fields from a filled order response", func() {
			resp := tradier.TradierOrderResponse{
				ID:           987654,
				Type:         "limit",
				Symbol:       "AAPL",
				Side:         "buy",
				Quantity:     100,
				Status:       "filled",
				Duration:     "day",
				AvgFillPrice: 152.50,
				Price:        153.00,
			}

			result := tradier.ToBrokerOrder(resp)
			Expect(result.ID).To(Equal("987654"))
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Side).To(Equal(broker.Buy))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.Status).To(Equal(broker.OrderFilled))
			Expect(result.OrderType).To(Equal(broker.Limit))
			Expect(result.LimitPrice).To(Equal(153.00))
		})
	})

	Describe("toBrokerPosition", func() {
		It("maps a long position correctly", func() {
			resp := tradier.TradierPositionResponse{
				ID:        1001,
				Symbol:    "AAPL",
				Quantity:  50,
				CostBasis: 7500.0,
			}

			result := tradier.ToBrokerPosition(resp)
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Qty).To(Equal(50.0))
			Expect(result.AvgOpenPrice).To(Equal(150.0))
			Expect(result.MarkPrice).To(BeZero())
			Expect(result.RealizedDayPL).To(BeZero())
		})

		It("maps a short position with negative quantity", func() {
			resp := tradier.TradierPositionResponse{
				ID:        1002,
				Symbol:    "TSLA",
				Quantity:  -25,
				CostBasis: -5000.0,
			}

			result := tradier.ToBrokerPosition(resp)
			Expect(result.Asset.Ticker).To(Equal("TSLA"))
			Expect(result.Qty).To(Equal(-25.0))
			Expect(result.AvgOpenPrice).To(Equal(200.0))
			Expect(result.MarkPrice).To(BeZero())
		})

		It("sets MarkPrice to 0 (populated later by Positions())", func() {
			resp := tradier.TradierPositionResponse{
				ID:        1003,
				Symbol:    "SPY",
				Quantity:  10,
				CostBasis: 4500.0,
			}

			result := tradier.ToBrokerPosition(resp)
			Expect(result.MarkPrice).To(BeZero())
		})

		It("computes AvgOpenPrice as CostBasis / abs(Quantity)", func() {
			resp := tradier.TradierPositionResponse{
				ID:        1004,
				Symbol:    "QQQ",
				Quantity:  -20,
				CostBasis: -7000.0,
			}

			result := tradier.ToBrokerPosition(resp)
			Expect(result.AvgOpenPrice).To(Equal(350.0))
		})
	})

	Describe("toBrokerBalance", func() {
		It("maps a margin account balance", func() {
			resp := tradier.TradierBalanceResponse{
				AccountNumber: "12345678",
				AccountType:   "margin",
				TotalEquity:   50000.0,
				TotalCash:     10000.0,
				MarketValue:   40000.0,
				Margin: tradier.TradierMarginBalance{
					StockBuyingPower:    80000.0,
					CurrentRequirement: 5000.0,
				},
			}

			result := tradier.ToBrokerBalance(resp)
			Expect(result.CashBalance).To(Equal(10000.0))
			Expect(result.NetLiquidatingValue).To(Equal(50000.0))
			Expect(result.EquityBuyingPower).To(Equal(80000.0))
			Expect(result.MaintenanceReq).To(Equal(5000.0))
		})

		It("maps a cash account balance", func() {
			resp := tradier.TradierBalanceResponse{
				AccountNumber: "87654321",
				AccountType:   "cash",
				TotalEquity:   20000.0,
				TotalCash:     15000.0,
				MarketValue:   5000.0,
				Cash: tradier.TradierCashBalance{
					CashAvailable:  15000.0,
					UnsettledFunds: 2000.0,
				},
			}

			result := tradier.ToBrokerBalance(resp)
			Expect(result.CashBalance).To(Equal(15000.0))
			Expect(result.NetLiquidatingValue).To(Equal(20000.0))
			Expect(result.EquityBuyingPower).To(Equal(15000.0))
			Expect(result.MaintenanceReq).To(BeZero())
		})
	})

	Describe("mapTradierSide", func() {
		It("maps buy to Buy", func() {
			Expect(tradier.MapTradierSide("buy")).To(Equal(broker.Buy))
		})

		It("maps sell to Sell", func() {
			Expect(tradier.MapTradierSide("sell")).To(Equal(broker.Sell))
		})

		It("maps sell_short to Sell", func() {
			Expect(tradier.MapTradierSide("sell_short")).To(Equal(broker.Sell))
		})

		It("maps buy_to_cover to Buy", func() {
			Expect(tradier.MapTradierSide("buy_to_cover")).To(Equal(broker.Buy))
		})
	})
})
