package tradestation

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("Types", Label("translation"), func() {
	Describe("toTSOrder", func() {
		It("translates a market buy order", func() {
			order := broker.Order{
				ID:          "order-1",
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result, translateErr := toTSOrder(order, "ACCT-123")
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.AccountID).To(Equal("ACCT-123"))
			Expect(result.Symbol).To(Equal("AAPL"))
			Expect(result.Quantity).To(Equal("100"))
			Expect(result.OrderType).To(Equal("Market"))
			Expect(result.TradeAction).To(Equal("BUY"))
			Expect(result.TimeInForce.Duration).To(Equal("DAY"))
			Expect(result.Route).To(Equal("Intelligent"))
			Expect(result.LimitPrice).To(Equal(""))
			Expect(result.StopPrice).To(Equal(""))
		})

		It("translates a limit sell order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Sell,
				Qty:         50,
				OrderType:   broker.Limit,
				LimitPrice:  350.0,
				TimeInForce: broker.GTC,
			}

			result, translateErr := toTSOrder(order, "ACCT-123")
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.OrderType).To(Equal("Limit"))
			Expect(result.LimitPrice).To(Equal("350.00"))
			Expect(result.TimeInForce.Duration).To(Equal("GTC"))
			Expect(result.TradeAction).To(Equal("SELL"))
		})

		It("translates a stop order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "TSLA"},
				Side:        broker.Sell,
				Qty:         25,
				OrderType:   broker.Stop,
				StopPrice:   200.0,
				TimeInForce: broker.Day,
			}

			result, translateErr := toTSOrder(order, "ACCT-123")
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.OrderType).To(Equal("StopMarket"))
			Expect(result.StopPrice).To(Equal("200.00"))
			Expect(result.LimitPrice).To(Equal(""))
		})

		It("translates a stop-limit order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "GOOG"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.StopLimit,
				StopPrice:   150.0,
				LimitPrice:  155.0,
				TimeInForce: broker.Day,
			}

			result, translateErr := toTSOrder(order, "ACCT-123")
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.OrderType).To(Equal("StopLimit"))
			Expect(result.StopPrice).To(Equal("150.00"))
			Expect(result.LimitPrice).To(Equal("155.00"))
		})

		It("maps all supported time-in-force values", func() {
			for _, testCase := range []struct {
				tif    broker.TimeInForce
				expect string
			}{
				{broker.Day, "DAY"},
				{broker.GTC, "GTC"},
				{broker.GTD, "GTD"},
				{broker.IOC, "IOC"},
				{broker.FOK, "FOK"},
				{broker.OnOpen, "OPG"},
				{broker.OnClose, "CLO"},
			} {
				order := broker.Order{
					Asset:       asset.Asset{Ticker: "X"},
					Side:        broker.Buy,
					Qty:         1,
					OrderType:   broker.Market,
					TimeInForce: testCase.tif,
				}
				result, translateErr := toTSOrder(order, "ACCT")
				Expect(translateErr).ToNot(HaveOccurred())
				Expect(result.TimeInForce.Duration).To(Equal(testCase.expect), "for TIF %d", testCase.tif)
			}
		})

		It("includes expiration date for GTD orders", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Limit,
				LimitPrice:  450.0,
				TimeInForce: broker.GTD,
				GTDDate:     time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
			}

			result, translateErr := toTSOrder(order, "ACCT")
			Expect(translateErr).ToNot(HaveOccurred())
			Expect(result.TimeInForce.Duration).To(Equal("GTD"))
			Expect(result.TimeInForce.Expiration).To(Equal("2026-04-15"))
		})
	})

	Describe("toBrokerOrder", func() {
		It("translates a TradeStation order response to broker.Order", func() {
			resp := tsOrderResponse{
				OrderID:    "123456",
				Status:     "OPN",
				OrderType:  "Limit",
				LimitPrice: "150.00",
				StopPrice:  "",
				Duration:   "GTC",
				Legs: []tsOrderLeg{
					{
						BuySellSideCode: "1",
						Symbol:          "AAPL",
						QuantityOrdered: "100",
					},
				},
			}

			result := toBrokerOrder(resp)

			Expect(result.ID).To(Equal("123456"))
			Expect(result.Status).To(Equal(broker.OrderOpen))
			Expect(result.OrderType).To(Equal(broker.Limit))
			Expect(result.LimitPrice).To(Equal(150.0))
			Expect(result.Side).To(Equal(broker.Buy))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
		})

		It("maps all TradeStation statuses correctly", func() {
			for _, testCase := range []struct {
				tsStatus string
				expected broker.OrderStatus
			}{
				{"ACK", broker.OrderSubmitted},
				{"DON", broker.OrderSubmitted},
				{"OPN", broker.OrderOpen},
				{"FLL", broker.OrderFilled},
				{"FLP", broker.OrderPartiallyFilled},
				{"OUT", broker.OrderCancelled},
				{"CAN", broker.OrderCancelled},
				{"EXP", broker.OrderCancelled},
				{"REJ", broker.OrderCancelled},
				{"UCN", broker.OrderCancelled},
				{"BRO", broker.OrderCancelled},
			} {
				resp := tsOrderResponse{Status: testCase.tsStatus}
				result := toBrokerOrder(resp)
				Expect(result.Status).To(Equal(testCase.expected), "for status %q", testCase.tsStatus)
			}
		})
	})

	Describe("toBrokerPosition", func() {
		It("translates a long position", func() {
			resp := tsPositionEntry{
				Symbol:           "AAPL",
				Quantity:         "100",
				AveragePrice:     "150.00",
				MarketValue:      "15500.00",
				TodaysProfitLoss: "200.00",
				Last:             "155.00",
			}

			result := toBrokerPosition(resp)

			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.AvgOpenPrice).To(Equal(150.0))
			Expect(result.MarkPrice).To(Equal(155.0))
			Expect(result.RealizedDayPL).To(Equal(200.0))
		})
	})

	Describe("toBrokerBalance", func() {
		It("translates a TradeStation balance response", func() {
			resp := tsBalanceResponse{
				CashBalance: "30000.00",
				Equity:      "75000.00",
				BuyingPower: "60000.00",
				MarketValue: "45000.00",
			}

			result := toBrokerBalance(resp)

			Expect(result.CashBalance).To(Equal(30000.0))
			Expect(result.NetLiquidatingValue).To(Equal(75000.0))
			Expect(result.EquityBuyingPower).To(Equal(60000.0))
		})
	})

	Describe("mapOrderType", func() {
		It("maps all broker order types to TradeStation strings", func() {
			Expect(mapOrderType(broker.Market)).To(Equal("Market"))
			Expect(mapOrderType(broker.Limit)).To(Equal("Limit"))
			Expect(mapOrderType(broker.Stop)).To(Equal("StopMarket"))
			Expect(mapOrderType(broker.StopLimit)).To(Equal("StopLimit"))
		})
	})

	Describe("mapTSOrderType", func() {
		It("maps all TradeStation order type strings to broker types", func() {
			Expect(mapTSOrderType("Market")).To(Equal(broker.Market))
			Expect(mapTSOrderType("Limit")).To(Equal(broker.Limit))
			Expect(mapTSOrderType("StopMarket")).To(Equal(broker.Stop))
			Expect(mapTSOrderType("StopLimit")).To(Equal(broker.StopLimit))
			Expect(mapTSOrderType("Unknown")).To(Equal(broker.Market))
		})
	})

	Describe("mapSide", func() {
		It("maps broker sides to TradeStation trade actions", func() {
			Expect(mapSide(broker.Buy)).To(Equal("BUY"))
			Expect(mapSide(broker.Sell)).To(Equal("SELL"))
		})
	})

	Describe("mapTSSide", func() {
		It("maps TradeStation BuySellSideCode to broker sides", func() {
			Expect(mapTSSide("1")).To(Equal(broker.Buy))
			Expect(mapTSSide("2")).To(Equal(broker.Sell))
			Expect(mapTSSide("3")).To(Equal(broker.Sell))
			Expect(mapTSSide("4")).To(Equal(broker.Buy))
		})
	})

	Describe("mapTSStatus", func() {
		It("returns OrderOpen for OPN", func() {
			Expect(mapTSStatus("OPN")).To(Equal(broker.OrderOpen))
		})

		It("returns OrderFilled for FLL", func() {
			Expect(mapTSStatus("FLL")).To(Equal(broker.OrderFilled))
		})

		It("returns OrderSubmitted for ACK", func() {
			Expect(mapTSStatus("ACK")).To(Equal(broker.OrderSubmitted))
		})

		It("returns OrderCancelled for CAN", func() {
			Expect(mapTSStatus("CAN")).To(Equal(broker.OrderCancelled))
		})

		It("defaults to OrderOpen for unknown statuses", func() {
			Expect(mapTSStatus("SOMETHING_NEW")).To(Equal(broker.OrderOpen))
		})
	})

	Describe("mapTimeInForce", func() {
		It("maps all broker TIF values", func() {
			for _, testCase := range []struct {
				tif    broker.TimeInForce
				expect string
			}{
				{broker.Day, "DAY"},
				{broker.GTC, "GTC"},
				{broker.GTD, "GTD"},
				{broker.IOC, "IOC"},
				{broker.FOK, "FOK"},
				{broker.OnOpen, "OPG"},
				{broker.OnClose, "CLO"},
			} {
				result := mapTimeInForce(testCase.tif)
				Expect(result).To(Equal(testCase.expect), "for TIF %d", testCase.tif)
			}
		})
	})

	Describe("stripDashes", func() {
		It("removes dashes from order IDs", func() {
			Expect(stripDashes("1-2-3-456")).To(Equal("123456"))
			Expect(stripDashes("NODASHES")).To(Equal("NODASHES"))
			Expect(stripDashes("")).To(Equal(""))
		})
	})

	Describe("buildGroupOrder", func() {
		It("builds an OCO group order", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Limit, LimitPrice: 150.0, TimeInForce: broker.Day},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC},
			}

			result, buildErr := buildGroupOrder(orders, broker.GroupOCO, "ACCT-123")
			Expect(buildErr).ToNot(HaveOccurred())
			Expect(result.Type).To(Equal("OCO"))
			Expect(result.Orders).To(HaveLen(2))
		})

		It("builds a bracket group order with entry first", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Stop, StopPrice: 430.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}

			result, buildErr := buildGroupOrder(orders, broker.GroupBracket, "ACCT-123")
			Expect(buildErr).ToNot(HaveOccurred())
			Expect(result.Type).To(Equal("BRK"))
			Expect(result.Orders).To(HaveLen(3))
			// Entry must be first regardless of input order
			Expect(result.Orders[0].TradeAction).To(Equal("BUY"))
		})

		It("returns ErrNoEntryOrder when bracket has no entry", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Stop, StopPrice: 430.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}

			_, buildErr := buildGroupOrder(orders, broker.GroupBracket, "ACCT-123")
			Expect(buildErr).To(MatchError(broker.ErrNoEntryOrder))
		})

		It("returns ErrMultipleEntryOrders when bracket has two entries", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
			}

			_, buildErr := buildGroupOrder(orders, broker.GroupBracket, "ACCT-123")
			Expect(buildErr).To(MatchError(broker.ErrMultipleEntryOrders))
		})

		It("returns ErrEmptyOrderGroup for empty orders", func() {
			_, buildErr := buildGroupOrder([]broker.Order{}, broker.GroupOCO, "ACCT-123")
			Expect(buildErr).To(MatchError(broker.ErrEmptyOrderGroup))
		})
	})
})
