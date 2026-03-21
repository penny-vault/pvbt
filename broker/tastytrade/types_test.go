package tastytrade

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("Types", Label("translation"), func() {
	Describe("toTastytradeOrder", func() {
		It("translates a market buy order", func() {
			order := broker.Order{
				ID:          "order-1",
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result := toTastytradeOrder(order)

			Expect(result.OrderType).To(Equal("Market"))
			Expect(result.TimeInForce).To(Equal("Day"))
			Expect(result.Price).To(BeZero())
			Expect(result.StopTrigger).To(BeZero())
			Expect(result.Legs).To(HaveLen(1))
			Expect(result.Legs[0].Symbol).To(Equal("AAPL"))
			Expect(result.Legs[0].Action).To(Equal("Buy to Open"))
			Expect(result.Legs[0].Quantity).To(Equal(100.0))
			Expect(result.Legs[0].InstrumentType).To(Equal("Equity"))
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

			result := toTastytradeOrder(order)

			Expect(result.OrderType).To(Equal("Limit"))
			Expect(result.Price).To(Equal(350.0))
			Expect(result.TimeInForce).To(Equal("GTC"))
			Expect(result.Legs[0].Action).To(Equal("Sell to Close"))
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

			result := toTastytradeOrder(order)

			Expect(result.OrderType).To(Equal("Stop"))
			Expect(result.StopTrigger).To(Equal(200.0))
			Expect(result.Price).To(BeZero())
		})

		It("translates a stop limit order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "GOOG"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.StopLimit,
				StopPrice:   150.0,
				LimitPrice:  155.0,
				TimeInForce: broker.Day,
			}

			result := toTastytradeOrder(order)

			Expect(result.OrderType).To(Equal("Stop Limit"))
			Expect(result.StopTrigger).To(Equal(150.0))
			Expect(result.Price).To(Equal(155.0))
		})

		It("maps all time-in-force values", func() {
			for _, tc := range []struct {
				tif    broker.TimeInForce
				expect string
			}{
				{broker.Day, "Day"},
				{broker.GTC, "GTC"},
				{broker.GTD, "GTD"},
				{broker.IOC, "IOC"},
				{broker.FOK, "FOK"},
				{broker.OnOpen, "Day"},
				{broker.OnClose, "Day"},
			} {
				order := broker.Order{
					Asset:       asset.Asset{Ticker: "X"},
					Side:        broker.Buy,
					Qty:         1,
					OrderType:   broker.Market,
					TimeInForce: tc.tif,
				}
				result := toTastytradeOrder(order)
				Expect(result.TimeInForce).To(Equal(tc.expect), "for TIF %d", tc.tif)
			}
		})
	})

	Describe("toBrokerOrder", func() {
		It("translates an order response", func() {
			resp := orderResponse{
				ID:          "tt-order-1",
				Status:      "Live",
				OrderType:   "Limit",
				TimeInForce: "GTC",
				Price:       150.0,
				Legs: []orderLegResponse{
					{
						Symbol:         "AAPL",
						InstrumentType: "Equity",
						Action:         "Buy to Open",
						Quantity:       100,
					},
				},
			}

			result := toBrokerOrder(resp)

			Expect(result.ID).To(Equal("tt-order-1"))
			Expect(result.Status).To(Equal(broker.OrderOpen))
			Expect(result.OrderType).To(Equal(broker.Limit))
			Expect(result.LimitPrice).To(Equal(150.0))
			Expect(result.Side).To(Equal(broker.Buy))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
		})

		It("maps all tastytrade statuses", func() {
			for _, tc := range []struct {
				ttStatus string
				expected broker.OrderStatus
			}{
				{"Received", broker.OrderSubmitted},
				{"Routed", broker.OrderSubmitted},
				{"In Flight", broker.OrderSubmitted},
				{"Live", broker.OrderOpen},
				{"Filled", broker.OrderFilled},
				{"Cancelled", broker.OrderCancelled},
				{"Expired", broker.OrderCancelled},
				{"Rejected", broker.OrderCancelled},
			} {
				resp := orderResponse{Status: tc.ttStatus}
				result := toBrokerOrder(resp)
				Expect(result.Status).To(Equal(tc.expected), "for status %q", tc.ttStatus)
			}
		})
	})

	Describe("toBrokerPosition", func() {
		It("translates a position response", func() {
			resp := positionResponse{
				Symbol:        "AAPL",
				Quantity:      100,
				AveragePrice:  150.0,
				MarkPrice:     155.0,
				RealizedDayPL: 200.0,
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
		It("translates a balance response", func() {
			resp := balanceResponse{
				CashBalance:         10000.0,
				NetLiquidatingValue: 25000.0,
				EquityBuyingPower:   50000.0,
				MaintenanceReq:      5000.0,
			}

			result := toBrokerBalance(resp)

			Expect(result.CashBalance).To(Equal(10000.0))
			Expect(result.NetLiquidatingValue).To(Equal(25000.0))
			Expect(result.EquityBuyingPower).To(Equal(50000.0))
			Expect(result.MaintenanceReq).To(Equal(5000.0))
		})
	})

	Describe("toBrokerFill", func() {
		It("translates a fill event", func() {
			fillTime := time.Date(2026, 3, 20, 14, 30, 0, 0, time.UTC)
			event := fillEvent{
				OrderID:  "tt-order-1",
				FillID:   "fill-1",
				Price:    152.50,
				Quantity: 50,
				FilledAt: fillTime,
			}

			result := toBrokerFill(event)

			Expect(result.OrderID).To(Equal("tt-order-1"))
			Expect(result.Price).To(Equal(152.50))
			Expect(result.Qty).To(Equal(50.0))
			Expect(result.FilledAt).To(Equal(fillTime))
		})
	})
})
