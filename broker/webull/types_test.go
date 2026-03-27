package webull_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/webull"
)

var _ = Describe("Types", func() {
	Describe("toWebullOrder", func() {
		It("maps a market buy order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			req := webull.ToWebullOrder(order, false)
			Expect(req.Symbol).To(Equal("AAPL"))
			Expect(req.Side).To(Equal("BUY"))
			Expect(req.OrderType).To(Equal("MARKET"))
			Expect(req.TimeInForce).To(Equal("DAY"))
			Expect(req.Qty).To(Equal("10"))
		})

		It("maps a limit sell order with GTC", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Sell,
				Qty:         5,
				OrderType:   broker.Limit,
				LimitPrice:  350.50,
				TimeInForce: broker.GTC,
			}
			req := webull.ToWebullOrder(order, false)
			Expect(req.Side).To(Equal("SELL"))
			Expect(req.OrderType).To(Equal("LIMIT"))
			Expect(req.TimeInForce).To(Equal("GTC"))
			Expect(req.LimitPrice).To(Equal("350.5"))
		})

		It("maps a stop-limit order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "GOOG"},
				Side:        broker.Sell,
				Qty:         20,
				OrderType:   broker.StopLimit,
				LimitPrice:  140,
				StopPrice:   138,
				TimeInForce: broker.Day,
			}
			req := webull.ToWebullOrder(order, false)
			Expect(req.OrderType).To(Equal("STOP_LOSS_LIMIT"))
			Expect(req.LimitPrice).To(Equal("140"))
			Expect(req.StopPrice).To(Equal("138"))
		})

		It("uses notional for fractional dollar-amount orders", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Amount:      500,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			req := webull.ToWebullOrder(order, true)
			Expect(req.Qty).To(BeEmpty())
			Expect(req.Notional).To(Equal("500"))
		})

		It("uses qty when fractional is false even with Amount set", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				Amount:      500,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			req := webull.ToWebullOrder(order, false)
			Expect(req.Qty).To(Equal("10"))
			Expect(req.Notional).To(BeEmpty())
		})
	})

	Describe("toBrokerOrder", func() {
		It("maps a Webull order response to broker.Order", func() {
			resp := webull.OrderResponseExport{
				ID:         "order-123",
				Symbol:     "AAPL",
				Side:       "BUY",
				Status:     "FILLED",
				OrderType:  "LIMIT",
				Qty:        "10",
				FilledQty:  "10",
				LimitPrice: "150.25",
			}
			order := webull.ToBrokerOrder(resp)
			Expect(order.ID).To(Equal("order-123"))
			Expect(order.Asset.Ticker).To(Equal("AAPL"))
			Expect(order.Side).To(Equal(broker.Buy))
			Expect(order.Status).To(Equal(broker.OrderFilled))
			Expect(order.OrderType).To(Equal(broker.Limit))
			Expect(order.Qty).To(Equal(10.0))
			Expect(order.LimitPrice).To(Equal(150.25))
		})
	})

	Describe("toBrokerPosition", func() {
		It("maps a Webull position response to broker.Position", func() {
			resp := webull.PositionResponseExport{
				Symbol:  "MSFT",
				Qty:     "25",
				AvgCost: "320.50",
			}
			pos := webull.ToBrokerPosition(resp)
			Expect(pos.Asset.Ticker).To(Equal("MSFT"))
			Expect(pos.Qty).To(Equal(25.0))
			Expect(pos.AvgOpenPrice).To(Equal(320.50))
		})
	})

	Describe("toBrokerBalance", func() {
		It("maps a Webull account response to broker.Balance", func() {
			resp := webull.AccountResponseExport{
				NetLiquidation: "100000.50",
				CashBalance:    "25000.00",
				BuyingPower:    "50000.00",
				MaintenanceReq: "15000.00",
			}
			bal := webull.ToBrokerBalance(resp)
			Expect(bal.NetLiquidatingValue).To(Equal(100000.50))
			Expect(bal.CashBalance).To(Equal(25000.00))
			Expect(bal.EquityBuyingPower).To(Equal(50000.00))
			Expect(bal.MaintenanceReq).To(Equal(15000.00))
		})
	})
})
