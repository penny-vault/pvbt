package alpaca

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("Types", Label("translation"), func() {
	Describe("toAlpacaOrder", func() {
		It("translates a market buy order", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result := toAlpacaOrder(order, false)

			Expect(result.Symbol).To(Equal("AAPL"))
			Expect(result.Side).To(Equal("buy"))
			Expect(result.Type).To(Equal("market"))
			Expect(result.TimeInForce).To(Equal("day"))
			Expect(result.Qty).To(Equal("100"))
			Expect(result.LimitPrice).To(BeEmpty())
			Expect(result.StopPrice).To(BeEmpty())
			Expect(result.Notional).To(BeEmpty())
			Expect(result.ClientOrderID).NotTo(BeEmpty())
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

			result := toAlpacaOrder(order, false)

			Expect(result.Side).To(Equal("sell"))
			Expect(result.Type).To(Equal("limit"))
			Expect(result.TimeInForce).To(Equal("gtc"))
			Expect(result.LimitPrice).To(Equal("350"))
			Expect(result.Qty).To(Equal("50"))
			Expect(result.StopPrice).To(BeEmpty())
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

			result := toAlpacaOrder(order, false)

			Expect(result.Type).To(Equal("stop"))
			Expect(result.StopPrice).To(Equal("200"))
			Expect(result.LimitPrice).To(BeEmpty())
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

			result := toAlpacaOrder(order, false)

			Expect(result.Type).To(Equal("stop_limit"))
			Expect(result.StopPrice).To(Equal("150"))
			Expect(result.LimitPrice).To(Equal("155"))
		})

		It("sets expire_time for GTD orders", func() {
			expiry := time.Date(2026, 4, 15, 16, 0, 0, 0, time.UTC)
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Limit,
				LimitPrice:  150.0,
				TimeInForce: broker.GTD,
				GTDDate:     expiry,
			}

			result := toAlpacaOrder(order, false)

			Expect(result.TimeInForce).To(Equal("gtd"))
			Expect(result.ExpireTime).To(Equal("2026-04-15T16:00:00Z"))
		})

		It("maps all time-in-force values", func() {
			for _, testCase := range []struct {
				tif    broker.TimeInForce
				expect string
			}{
				{broker.Day, "day"},
				{broker.GTC, "gtc"},
				{broker.GTD, "gtd"},
				{broker.IOC, "ioc"},
				{broker.FOK, "fok"},
				{broker.OnOpen, "opg"},
				{broker.OnClose, "cls"},
			} {
				order := broker.Order{
					Asset:       asset.Asset{Ticker: "X"},
					Side:        broker.Buy,
					Qty:         1,
					OrderType:   broker.Market,
					TimeInForce: testCase.tif,
				}
				result := toAlpacaOrder(order, false)
				Expect(result.TimeInForce).To(Equal(testCase.expect), "for TIF %d", testCase.tif)
			}
		})

		It("uses notional for fractional dollar-amount orders", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      500.50,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result := toAlpacaOrder(order, true)

			Expect(result.Notional).To(Equal("500.5"))
			Expect(result.Qty).To(BeEmpty())
		})

		It("uses qty when fractional is true but qty is set", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         10,
				Amount:      500.0,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result := toAlpacaOrder(order, true)

			Expect(result.Qty).To(Equal("10"))
			Expect(result.Notional).To(BeEmpty())
		})

		It("uses qty when fractional is false even with amount set", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         0,
				Amount:      500.0,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result := toAlpacaOrder(order, false)

			Expect(result.Qty).To(Equal("0"))
			Expect(result.Notional).To(BeEmpty())
		})

		It("generates a client_order_id", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result := toAlpacaOrder(order, false)

			Expect(result.ClientOrderID).NotTo(BeEmpty())
			// UUID v4 format: 8-4-4-4-12 hex characters
			Expect(result.ClientOrderID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`))
		})

		It("generates unique client_order_ids", func() {
			order := broker.Order{
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         1,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			first := toAlpacaOrder(order, false)
			second := toAlpacaOrder(order, false)

			Expect(first.ClientOrderID).NotTo(Equal(second.ClientOrderID))
		})
	})

	Describe("toBrokerOrder", func() {
		It("translates a full order response", func() {
			resp := orderResponse{
				ID:         "alpaca-order-1",
				Status:     "filled",
				Type:       "limit",
				Side:       "buy",
				Symbol:     "AAPL",
				Qty:        "100",
				LimitPrice: "150.50",
			}

			result := toBrokerOrder(resp)

			Expect(result.ID).To(Equal("alpaca-order-1"))
			Expect(result.Status).To(Equal(broker.OrderFilled))
			Expect(result.OrderType).To(Equal(broker.Limit))
			Expect(result.Side).To(Equal(broker.Buy))
			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.LimitPrice).To(Equal(150.50))
		})

		It("parses stop price from string", func() {
			resp := orderResponse{
				ID:        "order-2",
				Status:    "new",
				Type:      "stop",
				Side:      "sell",
				Symbol:    "TSLA",
				Qty:       "25",
				StopPrice: "200.75",
			}

			result := toBrokerOrder(resp)

			Expect(result.StopPrice).To(Equal(200.75))
			Expect(result.OrderType).To(Equal(broker.Stop))
		})

		It("maps all Alpaca statuses", func() {
			for _, testCase := range []struct {
				alpacaStatus string
				expected     broker.OrderStatus
			}{
				{"new", broker.OrderSubmitted},
				{"accepted", broker.OrderSubmitted},
				{"pending_new", broker.OrderSubmitted},
				{"partially_filled", broker.OrderPartiallyFilled},
				{"filled", broker.OrderFilled},
				{"canceled", broker.OrderCancelled},
				{"expired", broker.OrderCancelled},
				{"rejected", broker.OrderCancelled},
				{"suspended", broker.OrderCancelled},
			} {
				resp := orderResponse{Status: testCase.alpacaStatus}
				result := toBrokerOrder(resp)
				Expect(result.Status).To(Equal(testCase.expected), "for status %q", testCase.alpacaStatus)
			}
		})

		It("maps all order types", func() {
			for _, testCase := range []struct {
				alpacaType string
				expected   broker.OrderType
			}{
				{"market", broker.Market},
				{"limit", broker.Limit},
				{"stop", broker.Stop},
				{"stop_limit", broker.StopLimit},
			} {
				resp := orderResponse{Type: testCase.alpacaType}
				result := toBrokerOrder(resp)
				Expect(result.OrderType).To(Equal(testCase.expected), "for type %q", testCase.alpacaType)
			}
		})

		It("maps sides correctly", func() {
			buyResp := orderResponse{Side: "buy"}
			Expect(toBrokerOrder(buyResp).Side).To(Equal(broker.Buy))

			sellResp := orderResponse{Side: "sell"}
			Expect(toBrokerOrder(sellResp).Side).To(Equal(broker.Sell))
		})
	})

	Describe("toBrokerPosition", func() {
		It("translates a position response with string parsing", func() {
			resp := positionResponse{
				Symbol:               "AAPL",
				Qty:                  "100",
				AvgEntryPrice:        "150.25",
				CurrentPrice:         "155.50",
				UnrealizedIntradayPL: "525.00",
			}

			result := toBrokerPosition(resp)

			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.AvgOpenPrice).To(Equal(150.25))
			Expect(result.MarkPrice).To(Equal(155.50))
			Expect(result.RealizedDayPL).To(Equal(525.0))
		})

		It("handles invalid numeric strings gracefully", func() {
			resp := positionResponse{
				Symbol:               "BAD",
				Qty:                  "not_a_number",
				AvgEntryPrice:        "",
				CurrentPrice:         "abc",
				UnrealizedIntradayPL: "",
			}

			result := toBrokerPosition(resp)

			Expect(result.Qty).To(Equal(0.0))
			Expect(result.AvgOpenPrice).To(Equal(0.0))
			Expect(result.MarkPrice).To(Equal(0.0))
			Expect(result.RealizedDayPL).To(Equal(0.0))
		})
	})

	Describe("toBrokerBalance", func() {
		It("translates an account response with string parsing", func() {
			resp := accountResponse{
				Cash:              "10000.50",
				Equity:            "25000.75",
				BuyingPower:       "50000.00",
				MaintenanceMargin: "5000.25",
			}

			result := toBrokerBalance(resp)

			Expect(result.CashBalance).To(Equal(10000.50))
			Expect(result.NetLiquidatingValue).To(Equal(25000.75))
			Expect(result.EquityBuyingPower).To(Equal(50000.0))
			Expect(result.MaintenanceReq).To(Equal(5000.25))
		})

		It("returns zero for invalid values", func() {
			resp := accountResponse{
				Cash:              "bad",
				Equity:            "",
				BuyingPower:       "nope",
				MaintenanceMargin: "",
			}

			result := toBrokerBalance(resp)

			Expect(result.CashBalance).To(Equal(0.0))
			Expect(result.NetLiquidatingValue).To(Equal(0.0))
			Expect(result.EquityBuyingPower).To(Equal(0.0))
			Expect(result.MaintenanceReq).To(Equal(0.0))
		})
	})

	Describe("parseFloat", func() {
		It("parses a valid integer string", func() {
			Expect(parseFloat("50")).To(Equal(50.0))
		})

		It("parses a decimal string", func() {
			Expect(parseFloat("25.5")).To(Equal(25.5))
		})

		It("returns zero for invalid input", func() {
			Expect(parseFloat("abc")).To(Equal(0.0))
		})

		It("returns zero for empty string", func() {
			Expect(parseFloat("")).To(Equal(0.0))
		})

		It("parses negative values", func() {
			Expect(parseFloat("-10.5")).To(Equal(-10.5))
		})
	})

	Describe("formatFloat", func() {
		It("formats an integer value without trailing zeros", func() {
			Expect(formatFloat(100)).To(Equal("100"))
		})

		It("formats a decimal value", func() {
			Expect(formatFloat(150.50)).To(Equal("150.5"))
		})

		It("formats zero", func() {
			Expect(formatFloat(0)).To(Equal("0"))
		})
	})
})
