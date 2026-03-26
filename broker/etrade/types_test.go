package etrade_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/etrade"
)

var _ = Describe("Types", Label("translation"), func() {

	Describe("mapPriceType", func() {
		DescribeTable("maps broker.OrderType to E*TRADE priceType string",
			func(orderType broker.OrderType, expected string) {
				Expect(etrade.MapPriceType(orderType)).To(Equal(expected))
			},
			Entry("Market -> MARKET", broker.Market, "MARKET"),
			Entry("Limit -> LIMIT", broker.Limit, "LIMIT"),
			Entry("Stop -> STOP", broker.Stop, "STOP"),
			Entry("StopLimit -> STOP_LIMIT", broker.StopLimit, "STOP_LIMIT"),
		)

		It("returns MARKET for unknown order type", func() {
			Expect(etrade.MapPriceType(broker.OrderType(99))).To(Equal("MARKET"))
		})
	})

	Describe("unmapPriceType", func() {
		DescribeTable("maps E*TRADE priceType string to broker.OrderType",
			func(priceType string, expected broker.OrderType) {
				Expect(etrade.UnmapPriceType(priceType)).To(Equal(expected))
			},
			Entry("MARKET -> Market", "MARKET", broker.Market),
			Entry("LIMIT -> Limit", "LIMIT", broker.Limit),
			Entry("STOP -> Stop", "STOP", broker.Stop),
			Entry("STOP_LIMIT -> StopLimit", "STOP_LIMIT", broker.StopLimit),
		)

		It("returns Market for unknown price type", func() {
			Expect(etrade.UnmapPriceType("UNKNOWN")).To(Equal(broker.Market))
		})
	})

	Describe("mapOrderTerm", func() {
		DescribeTable("maps broker.TimeInForce to E*TRADE orderTerm string",
			func(tif broker.TimeInForce, expected string) {
				result, err := etrade.MapOrderTerm(tif)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(expected))
			},
			Entry("Day -> GOOD_FOR_DAY", broker.Day, "GOOD_FOR_DAY"),
			Entry("GTC -> GOOD_UNTIL_CANCEL", broker.GTC, "GOOD_UNTIL_CANCEL"),
			Entry("GTD -> GOOD_TILL_DATE", broker.GTD, "GOOD_TILL_DATE"),
			Entry("IOC -> IMMEDIATE_OR_CANCEL", broker.IOC, "IMMEDIATE_OR_CANCEL"),
			Entry("FOK -> FILL_OR_KILL", broker.FOK, "FILL_OR_KILL"),
		)

		It("returns error for OnOpen", func() {
			_, err := etrade.MapOrderTerm(broker.OnOpen)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("OnOpen"))
		})

		It("returns error for OnClose", func() {
			_, err := etrade.MapOrderTerm(broker.OnClose)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("OnClose"))
		})

		It("returns GOOD_FOR_DAY for unknown time-in-force", func() {
			result, err := etrade.MapOrderTerm(broker.TimeInForce(99))
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal("GOOD_FOR_DAY"))
		})
	})

	Describe("mapOrderAction", func() {
		DescribeTable("maps broker.Side to E*TRADE orderAction string",
			func(side broker.Side, expected string) {
				Expect(etrade.MapOrderAction(side)).To(Equal(expected))
			},
			Entry("Buy -> BUY", broker.Buy, "BUY"),
			Entry("Sell -> SELL", broker.Sell, "SELL"),
		)

		It("returns BUY for unknown side", func() {
			Expect(etrade.MapOrderAction(broker.Side(99))).To(Equal("BUY"))
		})
	})

	Describe("unmapOrderAction", func() {
		DescribeTable("maps E*TRADE orderAction string to broker.Side",
			func(action string, expected broker.Side) {
				Expect(etrade.UnmapOrderAction(action)).To(Equal(expected))
			},
			Entry("BUY -> Buy", "BUY", broker.Buy),
			Entry("BUY_TO_COVER -> Buy", "BUY_TO_COVER", broker.Buy),
			Entry("SELL -> Sell", "SELL", broker.Sell),
			Entry("SELL_SHORT -> Sell", "SELL_SHORT", broker.Sell),
		)

		It("returns Buy for unknown action", func() {
			Expect(etrade.UnmapOrderAction("UNKNOWN")).To(Equal(broker.Buy))
		})
	})

	Describe("mapOrderStatus", func() {
		DescribeTable("maps E*TRADE status string to broker.OrderStatus",
			func(status string, expected broker.OrderStatus) {
				Expect(etrade.MapOrderStatus(status)).To(Equal(expected))
			},
			Entry("OPEN -> OrderOpen", "OPEN", broker.OrderOpen),
			Entry("EXECUTED -> OrderFilled", "EXECUTED", broker.OrderFilled),
			Entry("CANCELLED -> OrderCancelled", "CANCELLED", broker.OrderCancelled),
			Entry("CANCEL_REQUESTED -> OrderCancelled", "CANCEL_REQUESTED", broker.OrderCancelled),
			Entry("PARTIAL -> OrderPartiallyFilled", "PARTIAL", broker.OrderPartiallyFilled),
			Entry("INDIVIDUAL_FILLS -> OrderPartiallyFilled", "INDIVIDUAL_FILLS", broker.OrderPartiallyFilled),
		)

		It("returns OrderSubmitted for unknown status", func() {
			Expect(etrade.MapOrderStatus("SOMETHING_ELSE")).To(Equal(broker.OrderSubmitted))
		})
	})

	Describe("formatDate and parseDate", func() {
		It("formats a date as MMDDYYYY", func() {
			tt := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
			Expect(etrade.FormatDate(tt)).To(Equal("03152024"))
		})

		It("parses MMDDYYYY string to time.Time", func() {
			tt, err := etrade.ParseDate("03152024")
			Expect(err).ToNot(HaveOccurred())
			Expect(tt.Year()).To(Equal(2024))
			Expect(tt.Month()).To(Equal(time.March))
			Expect(tt.Day()).To(Equal(15))
		})

		It("returns error for invalid date string", func() {
			_, err := etrade.ParseDate("notadate")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("etrade"))
		})

		It("round-trips a date through format and parse", func() {
			original := time.Date(2023, 11, 7, 0, 0, 0, 0, time.UTC)
			formatted := etrade.FormatDate(original)
			parsed, err := etrade.ParseDate(formatted)
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed.Year()).To(Equal(original.Year()))
			Expect(parsed.Month()).To(Equal(original.Month()))
			Expect(parsed.Day()).To(Equal(original.Day()))
		})

		It("handles single-digit month and day with zero padding", func() {
			tt := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)
			Expect(etrade.FormatDate(tt)).To(Equal("01052025"))
		})
	})

	Describe("toBrokerOrder", func() {
		It("maps a filled limit buy order correctly", func() {
			detail := etrade.EtradeOrderDetail{
				OrderID:   12345,
				OrderType: "EQ",
				Status:    "EXECUTED",
				OrderList: []etrade.EtradeOrderLeg{
					{
						PriceType:  "LIMIT",
						OrderTerm:  "GOOD_FOR_DAY",
						LimitPrice: 150.50,
						Instrument: []etrade.EtradeInstrument{
							{
								OrderAction: "BUY",
								Quantity:    100,
							},
						},
					},
				},
			}
			detail.OrderList[0].Instrument[0].Product.Symbol = "AAPL"
			detail.OrderList[0].Instrument[0].Product.SecurityType = "EQ"

			result := etrade.ToBrokerOrder(detail)
			Expect(result.ID).To(Equal("12345"))
			Expect(result.Status).To(Equal(broker.OrderFilled))
			Expect(result.OrderType).To(Equal(broker.Limit))
			Expect(result.LimitPrice).To(Equal(150.50))
			Expect(result.Side).To(Equal(broker.Buy))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
		})

		It("maps a stop sell order correctly", func() {
			detail := etrade.EtradeOrderDetail{
				OrderID: 99,
				Status:  "OPEN",
				OrderList: []etrade.EtradeOrderLeg{
					{
						PriceType: "STOP",
						StopPrice: 200.0,
						Instrument: []etrade.EtradeInstrument{
							{
								OrderAction: "SELL",
								Quantity:    50,
							},
						},
					},
				},
			}
			detail.OrderList[0].Instrument[0].Product.Symbol = "TSLA"

			result := etrade.ToBrokerOrder(detail)
			Expect(result.ID).To(Equal("99"))
			Expect(result.Status).To(Equal(broker.OrderOpen))
			Expect(result.OrderType).To(Equal(broker.Stop))
			Expect(result.StopPrice).To(Equal(200.0))
			Expect(result.Side).To(Equal(broker.Sell))
			Expect(result.Asset.Ticker).To(Equal("TSLA"))
		})

		It("handles an order with no legs gracefully", func() {
			detail := etrade.EtradeOrderDetail{
				OrderID:   77,
				Status:    "PARTIAL",
				OrderList: []etrade.EtradeOrderLeg{},
			}

			result := etrade.ToBrokerOrder(detail)
			Expect(result.ID).To(Equal("77"))
			Expect(result.Status).To(Equal(broker.OrderPartiallyFilled))
			Expect(result.Asset.Ticker).To(BeEmpty())
		})

		It("handles a leg with no instruments gracefully", func() {
			detail := etrade.EtradeOrderDetail{
				OrderID: 88,
				Status:  "OPEN",
				OrderList: []etrade.EtradeOrderLeg{
					{
						PriceType:  "MARKET",
						Instrument: []etrade.EtradeInstrument{},
					},
				},
			}

			result := etrade.ToBrokerOrder(detail)
			Expect(result.ID).To(Equal("88"))
			Expect(result.OrderType).To(Equal(broker.Market))
			Expect(result.Asset.Ticker).To(BeEmpty())
		})
	})

	Describe("toBrokerPosition", func() {
		It("maps a long position correctly", func() {
			pos := etrade.EtradePosition{
				PositionID:   1001,
				Quantity:     50,
				CostPerShare: 150.0,
				MarketValue:  8000.0,
			}
			pos.Product.Symbol = "AAPL"

			result := etrade.ToBrokerPosition(pos)
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Qty).To(Equal(50.0))
			Expect(result.AvgOpenPrice).To(Equal(150.0))
			Expect(result.MarkPrice).To(Equal(160.0))
			Expect(result.RealizedDayPL).To(BeZero())
		})

		It("maps a short position with negative quantity", func() {
			pos := etrade.EtradePosition{
				Quantity:     -25,
				CostPerShare: 200.0,
				MarketValue:  -5500.0,
			}
			pos.Product.Symbol = "TSLA"

			result := etrade.ToBrokerPosition(pos)
			Expect(result.Asset.Ticker).To(Equal("TSLA"))
			Expect(result.Qty).To(Equal(-25.0))
			Expect(result.AvgOpenPrice).To(Equal(200.0))
		})

		It("uses Product.Symbol for ticker", func() {
			pos := etrade.EtradePosition{
				Quantity:    10,
				MarketValue: 4000.0,
			}
			pos.Product.Symbol = "SPY"

			result := etrade.ToBrokerPosition(pos)
			Expect(result.Asset.Ticker).To(Equal("SPY"))
		})
	})

	Describe("toBrokerBalance", func() {
		It("maps a margin account using MarginBuyingPower", func() {
			resp := etrade.EtradeBalanceResponse{}
			resp.BalanceResponse.AccountType = "MARGIN"
			resp.BalanceResponse.Computed.CashAvailableForInvestment = 5000.0
			resp.BalanceResponse.Computed.MarginBuyingPower = 20000.0
			resp.BalanceResponse.Computed.CashBuyingPower = 5000.0
			resp.BalanceResponse.Computed.MaintenanceReq = 3000.0
			resp.BalanceResponse.Computed.RealTimeValues.TotalAccountValue = 50000.0

			result := etrade.ToBrokerBalance(resp)
			Expect(result.CashBalance).To(Equal(5000.0))
			Expect(result.NetLiquidatingValue).To(Equal(50000.0))
			Expect(result.EquityBuyingPower).To(Equal(20000.0))
			Expect(result.MaintenanceReq).To(Equal(3000.0))
		})

		It("falls back to CashBuyingPower when MarginBuyingPower is zero", func() {
			resp := etrade.EtradeBalanceResponse{}
			resp.BalanceResponse.Computed.CashAvailableForInvestment = 8000.0
			resp.BalanceResponse.Computed.MarginBuyingPower = 0.0
			resp.BalanceResponse.Computed.CashBuyingPower = 8000.0
			resp.BalanceResponse.Computed.RealTimeValues.TotalAccountValue = 20000.0

			result := etrade.ToBrokerBalance(resp)
			Expect(result.EquityBuyingPower).To(Equal(8000.0))
			Expect(result.CashBalance).To(Equal(8000.0))
			Expect(result.NetLiquidatingValue).To(Equal(20000.0))
		})

		It("uses RealTimeValues.TotalAccountValue for NetLiquidatingValue", func() {
			resp := etrade.EtradeBalanceResponse{}
			resp.BalanceResponse.Computed.RealTimeValues.TotalAccountValue = 75000.0

			result := etrade.ToBrokerBalance(resp)
			Expect(result.NetLiquidatingValue).To(Equal(75000.0))
		})
	})

	Describe("toBrokerTransaction", func() {
		It("maps a buy transaction correctly", func() {
			txn := etrade.EtradeTransaction{
				TransactionID:   555,
				TransactionDate: "03152024",
				Amount:          -15050.0,
				Description:     "Bought 100 AAPL",
			}
			txn.Brokerage.Product.Symbol = "AAPL"
			txn.Brokerage.Quantity = 100
			txn.Brokerage.Price = 150.50

			result := etrade.ToBrokerTransaction(txn)
			Expect(result.ID).To(Equal("555"))
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Type).To(Equal(asset.BuyTransaction))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.Price).To(Equal(150.50))
			Expect(result.Amount).To(Equal(-15050.0))
			Expect(result.Date.Year()).To(Equal(2024))
			Expect(result.Date.Month()).To(Equal(time.March))
			Expect(result.Date.Day()).To(Equal(15))
			Expect(result.Justification).To(Equal("Bought 100 AAPL"))
		})

		It("maps a dividend transaction correctly", func() {
			txn := etrade.EtradeTransaction{
				TransactionID:   556,
				TransactionDate: "06012024",
				Amount:          50.0,
				Description:     "Dividend received MSFT",
			}
			txn.Brokerage.Product.Symbol = "MSFT"

			result := etrade.ToBrokerTransaction(txn)
			Expect(result.Type).To(Equal(asset.DividendTransaction))
			Expect(result.Amount).To(Equal(50.0))
		})

		It("maps a sell transaction correctly", func() {
			txn := etrade.EtradeTransaction{
				TransactionID:   557,
				TransactionDate: "07102024",
				Amount:          5000.0,
				Description:     "Sold 20 SPY",
			}
			txn.Brokerage.Product.Symbol = "SPY"
			txn.Brokerage.Quantity = 20
			txn.Brokerage.Price = 250.0

			result := etrade.ToBrokerTransaction(txn)
			Expect(result.Type).To(Equal(asset.SellTransaction))
		})

		It("classifies unknown description as JournalTransaction", func() {
			txn := etrade.EtradeTransaction{
				TransactionID:   558,
				TransactionDate: "01012024",
				Amount:          0.0,
				Description:     "Some unknown event",
			}

			result := etrade.ToBrokerTransaction(txn)
			Expect(result.Type).To(Equal(asset.JournalTransaction))
		})

		It("handles an invalid date gracefully (zero time)", func() {
			txn := etrade.EtradeTransaction{
				TransactionID:   559,
				TransactionDate: "invalid",
				Description:     "Fee charged",
			}

			result := etrade.ToBrokerTransaction(txn)
			Expect(result.Date.IsZero()).To(BeTrue())
		})
	})

	Describe("toEtradeOrderRequest", func() {
		It("creates a valid preview request for a Market/Buy/Day order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			req, err := etrade.ToEtradeOrderRequest(order)
			Expect(err).ToNot(HaveOccurred())
			Expect(req.OrderType).To(Equal("EQ"))
			Expect(req.ClientOrderID).To(HaveLen(20))
			Expect(req.Order).To(HaveLen(1))

			leg := req.Order[0]
			Expect(leg.PriceType).To(Equal("MARKET"))
			Expect(leg.OrderTerm).To(Equal("GOOD_FOR_DAY"))
			Expect(leg.MarketSession).To(Equal("REGULAR"))
			Expect(leg.AllOrNone).To(BeFalse())
			Expect(leg.Instrument).To(HaveLen(1))

			instr := leg.Instrument[0]
			Expect(instr.Product.Symbol).To(Equal("AAPL"))
			Expect(instr.Product.SecurityType).To(Equal("EQ"))
			Expect(instr.OrderAction).To(Equal("BUY"))
			Expect(instr.QuantityType).To(Equal("QUANTITY"))
			Expect(instr.Quantity).To(Equal(10.0))
		})

		It("creates a limit sell GTC order with limit price", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "TSLA"},
				Side:        broker.Sell,
				Qty:         5,
				OrderType:   broker.Limit,
				LimitPrice:  300.0,
				TimeInForce: broker.GTC,
			}

			req, err := etrade.ToEtradeOrderRequest(order)
			Expect(err).ToNot(HaveOccurred())
			leg := req.Order[0]
			Expect(leg.PriceType).To(Equal("LIMIT"))
			Expect(leg.OrderTerm).To(Equal("GOOD_UNTIL_CANCEL"))
			Expect(leg.LimitPrice).To(Equal(300.0))
			Expect(leg.Instrument[0].OrderAction).To(Equal("SELL"))
		})

		It("creates a stop limit order with both prices", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "GOOG"},
				Side:        broker.Buy,
				Qty:         3,
				OrderType:   broker.StopLimit,
				LimitPrice:  145.0,
				StopPrice:   140.0,
				TimeInForce: broker.Day,
			}

			req, err := etrade.ToEtradeOrderRequest(order)
			Expect(err).ToNot(HaveOccurred())
			leg := req.Order[0]
			Expect(leg.PriceType).To(Equal("STOP_LIMIT"))
			Expect(leg.LimitPrice).To(Equal(145.0))
			Expect(leg.StopPrice).To(Equal(140.0))
		})

		It("returns error for OnOpen time-in-force", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.OnOpen,
			}
			_, err := etrade.ToEtradeOrderRequest(order)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("etrade"))
		})

		It("returns error for OnClose time-in-force", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.OnClose,
			}
			_, err := etrade.ToEtradeOrderRequest(order)
			Expect(err).To(HaveOccurred())
		})

		It("generates a unique clientOrderId each call", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			req1, err1 := etrade.ToEtradeOrderRequest(order)
			req2, err2 := etrade.ToEtradeOrderRequest(order)
			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())
			Expect(req1.ClientOrderID).ToNot(Equal(req2.ClientOrderID))
		})

		It("supports IOC time-in-force", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.IOC,
			}
			req, err := etrade.ToEtradeOrderRequest(order)
			Expect(err).ToNot(HaveOccurred())
			Expect(req.Order[0].OrderTerm).To(Equal("IMMEDIATE_OR_CANCEL"))
		})

		It("supports FOK time-in-force", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.FOK,
			}
			req, err := etrade.ToEtradeOrderRequest(order)
			Expect(err).ToNot(HaveOccurred())
			Expect(req.Order[0].OrderTerm).To(Equal("FILL_OR_KILL"))
		})

		It("supports GTD time-in-force", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "SPY"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Limit,
				LimitPrice:  400.0,
				TimeInForce: broker.GTD,
			}
			req, err := etrade.ToEtradeOrderRequest(order)
			Expect(err).ToNot(HaveOccurred())
			Expect(req.Order[0].OrderTerm).To(Equal("GOOD_TILL_DATE"))
		})
	})
})
