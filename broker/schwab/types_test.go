package schwab

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("Types", Label("translation"), func() {
	Describe("toSchwabOrder", func() {
		It("translates a market buy order", func() {
			order := broker.Order{
				ID:          "order-1",
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}

			result, translateErr := toSchwabOrder(order)
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.OrderType).To(Equal("MARKET"))
			Expect(result.Session).To(Equal("NORMAL"))
			Expect(result.Duration).To(Equal("DAY"))
			Expect(result.OrderStrategyType).To(Equal("SINGLE"))
			Expect(result.OrderLegCollection).To(HaveLen(1))
			Expect(result.OrderLegCollection[0].Instruction).To(Equal("BUY"))
			Expect(result.OrderLegCollection[0].Quantity).To(Equal(100.0))
			Expect(result.OrderLegCollection[0].Instrument.Symbol).To(Equal("AAPL"))
			Expect(result.OrderLegCollection[0].Instrument.AssetType).To(Equal("EQUITY"))
			Expect(result.Price).To(BeZero())
			Expect(result.StopPrice).To(BeZero())
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

			result, translateErr := toSchwabOrder(order)
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.OrderType).To(Equal("LIMIT"))
			Expect(result.Price).To(Equal(350.0))
			Expect(result.Duration).To(Equal("GOOD_TILL_CANCEL"))
			Expect(result.OrderLegCollection[0].Instruction).To(Equal("SELL"))
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

			result, translateErr := toSchwabOrder(order)
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.OrderType).To(Equal("STOP"))
			Expect(result.StopPrice).To(Equal(200.0))
			Expect(result.Price).To(BeZero())
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

			result, translateErr := toSchwabOrder(order)
			Expect(translateErr).ToNot(HaveOccurred())

			Expect(result.OrderType).To(Equal("STOP_LIMIT"))
			Expect(result.StopPrice).To(Equal(150.0))
			Expect(result.Price).To(Equal(155.0))
		})

		It("maps LotSelection to taxLotMethod", func() {
			for _, testCase := range []struct {
				lotSelection int
				expected     string
			}{
				{0, "FIFO"},
				{1, "LIFO"},
				{2, "HIGH_COST"},
				{3, "SPECIFIC_LOT"},
			} {
				order := broker.Order{
					Asset:        asset.Asset{Ticker: "X"},
					Side:         broker.Sell,
					Qty:          1,
					OrderType:    broker.Market,
					TimeInForce:  broker.Day,
					LotSelection: testCase.lotSelection,
				}
				result, translateErr := toSchwabOrder(order)
				Expect(translateErr).ToNot(HaveOccurred())
				Expect(result.TaxLotMethod).To(Equal(testCase.expected), "for LotSelection %d", testCase.lotSelection)
			}
		})

		It("maps supported time-in-force values", func() {
			for _, testCase := range []struct {
				tif    broker.TimeInForce
				expect string
			}{
				{broker.Day, "DAY"},
				{broker.GTC, "GOOD_TILL_CANCEL"},
				{broker.IOC, "IMMEDIATE_OR_CANCEL"},
				{broker.FOK, "FILL_OR_KILL"},
			} {
				order := broker.Order{
					Asset:       asset.Asset{Ticker: "X"},
					Side:        broker.Buy,
					Qty:         1,
					OrderType:   broker.Market,
					TimeInForce: testCase.tif,
				}
				result, translateErr := toSchwabOrder(order)
				Expect(translateErr).ToNot(HaveOccurred())
				Expect(result.Duration).To(Equal(testCase.expect), "for TIF %d", testCase.tif)
			}
		})

		It("returns an error for unsupported time-in-force values", func() {
			for _, tif := range []broker.TimeInForce{broker.OnOpen, broker.OnClose, broker.GTD} {
				order := broker.Order{
					Asset:       asset.Asset{Ticker: "X"},
					Side:        broker.Buy,
					Qty:         1,
					OrderType:   broker.Market,
					TimeInForce: tif,
				}
				_, translateErr := toSchwabOrder(order)
				Expect(translateErr).To(HaveOccurred(), "for TIF %d", tif)
			}
		})
	})

	Describe("toBrokerOrder", func() {
		It("translates a Schwab order response to broker.Order", func() {
			resp := schwabOrderResponse{
				OrderID:           123456,
				Status:            "WORKING",
				OrderType:         "LIMIT",
				Price:             150.0,
				Duration:          "DAY",
				OrderStrategyType: "SINGLE",
				OrderLegCollection: []schwabOrderLeg{
					{
						Instruction: "BUY",
						Quantity:    100,
						Instrument: schwabInstrument{
							Symbol:    "AAPL",
							AssetType: "EQUITY",
						},
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

		It("maps all Schwab statuses correctly", func() {
			for _, testCase := range []struct {
				schwabStatus string
				expected     broker.OrderStatus
			}{
				{"NEW", broker.OrderSubmitted},
				{"AWAITING_PARENT_ORDER", broker.OrderSubmitted},
				{"AWAITING_CONDITION", broker.OrderSubmitted},
				{"AWAITING_STOP_CONDITION", broker.OrderSubmitted},
				{"AWAITING_MANUAL_REVIEW", broker.OrderSubmitted},
				{"ACCEPTED", broker.OrderSubmitted},
				{"PENDING_ACTIVATION", broker.OrderSubmitted},
				{"QUEUED", broker.OrderSubmitted},
				{"PENDING_ACKNOWLEDGEMENT", broker.OrderSubmitted},
				{"WORKING", broker.OrderOpen},
				{"FILLED", broker.OrderFilled},
				{"PENDING_CANCEL", broker.OrderCancelled},
				{"CANCELED", broker.OrderCancelled},
				{"REJECTED", broker.OrderCancelled},
				{"EXPIRED", broker.OrderCancelled},
				{"REPLACED", broker.OrderCancelled},
				{"PENDING_REPLACE", broker.OrderCancelled},
				{"PENDING_RECALL", broker.OrderCancelled},
			} {
				resp := schwabOrderResponse{Status: testCase.schwabStatus}
				result := toBrokerOrder(resp)
				Expect(result.Status).To(Equal(testCase.expected), "for status %q", testCase.schwabStatus)
			}
		})
	})

	Describe("toBrokerPosition", func() {
		It("translates a long position", func() {
			resp := schwabPositionEntry{
				Instrument: schwabInstrument{
					Symbol:    "AAPL",
					AssetType: "EQUITY",
				},
				LongQuantity:         100,
				ShortQuantity:        0,
				AveragePrice:         150.0,
				MarketValue:          15500.0,
				CurrentDayProfitLoss: 200.0,
			}

			result := toBrokerPosition(resp)

			Expect(result.Asset.Ticker).To(Equal("AAPL"))
			Expect(result.Qty).To(Equal(100.0))
			Expect(result.AvgOpenPrice).To(Equal(150.0))
			Expect(result.MarkPrice).To(Equal(155.0))
			Expect(result.RealizedDayPL).To(Equal(200.0))
		})

		It("translates a short position with negative qty", func() {
			resp := schwabPositionEntry{
				Instrument: schwabInstrument{
					Symbol:    "TSLA",
					AssetType: "EQUITY",
				},
				LongQuantity:         0,
				ShortQuantity:        50,
				AveragePrice:         200.0,
				MarketValue:          9500.0,
				CurrentDayProfitLoss: -100.0,
			}

			result := toBrokerPosition(resp)

			Expect(result.Asset.Ticker).To(Equal("TSLA"))
			Expect(result.Qty).To(Equal(-50.0))
			Expect(result.MarkPrice).To(Equal(-190.0))
		})
	})

	Describe("toBrokerBalance", func() {
		It("translates a margin account balance", func() {
			resp := schwabAccountResponse{
				SecuritiesAccount: schwabSecuritiesAccount{
					AccountNumber: "12345678",
					Type:          "MARGIN",
					CurrentBalances: schwabBalances{
						CashBalance:            10000.0,
						Equity:                 25000.0,
						BuyingPower:            50000.0,
						MaintenanceRequirement: 5000.0,
					},
				},
			}

			result := toBrokerBalance(resp)

			Expect(result.CashBalance).To(Equal(10000.0))
			Expect(result.NetLiquidatingValue).To(Equal(25000.0))
			Expect(result.EquityBuyingPower).To(Equal(50000.0))
			Expect(result.MaintenanceReq).To(Equal(5000.0))
		})
	})

	Describe("mapOrderType", func() {
		It("maps all broker order types to Schwab strings", func() {
			Expect(mapOrderType(broker.Market)).To(Equal("MARKET"))
			Expect(mapOrderType(broker.Limit)).To(Equal("LIMIT"))
			Expect(mapOrderType(broker.Stop)).To(Equal("STOP"))
			Expect(mapOrderType(broker.StopLimit)).To(Equal("STOP_LIMIT"))
		})
	})

	Describe("mapSchwabOrderType", func() {
		It("maps all Schwab order type strings to broker types", func() {
			Expect(mapSchwabOrderType("MARKET")).To(Equal(broker.Market))
			Expect(mapSchwabOrderType("LIMIT")).To(Equal(broker.Limit))
			Expect(mapSchwabOrderType("STOP")).To(Equal(broker.Stop))
			Expect(mapSchwabOrderType("STOP_LIMIT")).To(Equal(broker.StopLimit))
			Expect(mapSchwabOrderType("UNKNOWN")).To(Equal(broker.Market))
		})
	})

	Describe("mapSide", func() {
		It("maps broker sides to Schwab instructions", func() {
			Expect(mapSide(broker.Buy)).To(Equal("BUY"))
			Expect(mapSide(broker.Sell)).To(Equal("SELL"))
		})
	})

	Describe("mapSchwabSide", func() {
		It("maps Schwab instructions to broker sides", func() {
			Expect(mapSchwabSide("BUY")).To(Equal(broker.Buy))
			Expect(mapSchwabSide("SELL")).To(Equal(broker.Sell))
			Expect(mapSchwabSide("SELL_SHORT")).To(Equal(broker.Sell))
			Expect(mapSchwabSide("BUY_TO_COVER")).To(Equal(broker.Buy))
		})
	})

	Describe("mapTimeInForce", func() {
		It("maps supported broker TIF values", func() {
			val, err := mapTimeInForce(broker.Day)
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal("DAY"))

			val, err = mapTimeInForce(broker.GTC)
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal("GOOD_TILL_CANCEL"))

			val, err = mapTimeInForce(broker.IOC)
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal("IMMEDIATE_OR_CANCEL"))

			val, err = mapTimeInForce(broker.FOK)
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal("FILL_OR_KILL"))
		})

		It("returns an error for GTD", func() {
			_, err := mapTimeInForce(broker.GTD)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("GTD"))
		})

		It("returns an error for OnOpen", func() {
			_, err := mapTimeInForce(broker.OnOpen)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("OnOpen"))
		})

		It("returns an error for OnClose", func() {
			_, err := mapTimeInForce(broker.OnClose)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("OnClose"))
		})
	})

	Describe("mapSchwabStatus", func() {
		It("returns OrderOpen for WORKING", func() {
			Expect(mapSchwabStatus("WORKING")).To(Equal(broker.OrderOpen))
		})

		It("returns OrderFilled for FILLED", func() {
			Expect(mapSchwabStatus("FILLED")).To(Equal(broker.OrderFilled))
		})

		It("returns OrderSubmitted for NEW", func() {
			Expect(mapSchwabStatus("NEW")).To(Equal(broker.OrderSubmitted))
		})

		It("returns OrderCancelled for CANCELED", func() {
			Expect(mapSchwabStatus("CANCELED")).To(Equal(broker.OrderCancelled))
		})

		It("defaults to OrderOpen for unknown statuses", func() {
			Expect(mapSchwabStatus("SOMETHING_NEW")).To(Equal(broker.OrderOpen))
		})
	})

	Describe("mapLotSelection", func() {
		It("maps lot selection integers to Schwab tax lot methods", func() {
			Expect(mapLotSelection(0)).To(Equal("FIFO"))
			Expect(mapLotSelection(1)).To(Equal("LIFO"))
			Expect(mapLotSelection(2)).To(Equal("HIGH_COST"))
			Expect(mapLotSelection(3)).To(Equal("SPECIFIC_LOT"))
			Expect(mapLotSelection(99)).To(Equal("FIFO"))
		})
	})

	Describe("buildBracketOrder", func() {
		It("builds a TRIGGER order with nested OCO children", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Stop, StopPrice: 430.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}

			result, buildErr := buildBracketOrder(orders)

			Expect(buildErr).ToNot(HaveOccurred())
			Expect(result.OrderStrategyType).To(Equal("TRIGGER"))
			Expect(result.OrderType).To(Equal("MARKET"))
			Expect(result.OrderLegCollection[0].Instruction).To(Equal("BUY"))
			Expect(result.ChildOrderStrategies).To(HaveLen(1))

			ocoChild := result.ChildOrderStrategies[0]
			Expect(ocoChild.OrderStrategyType).To(Equal("OCO"))
			Expect(ocoChild.ChildOrderStrategies).To(HaveLen(2))
		})

		It("returns ErrNoEntryOrder when no entry is present", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Limit, LimitPrice: 460.0, TimeInForce: broker.GTC, GroupRole: broker.RoleTakeProfit},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Sell, Qty: 5, OrderType: broker.Stop, StopPrice: 430.0, TimeInForce: broker.GTC, GroupRole: broker.RoleStopLoss},
			}

			_, buildErr := buildBracketOrder(orders)
			Expect(buildErr).To(MatchError(broker.ErrNoEntryOrder))
		})

		It("returns ErrMultipleEntryOrders when two entries are present", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
				{Asset: asset.Asset{Ticker: "SPY"}, Side: broker.Buy, Qty: 5, OrderType: broker.Market, TimeInForce: broker.Day, GroupRole: broker.RoleEntry},
			}

			_, buildErr := buildBracketOrder(orders)
			Expect(buildErr).To(MatchError(broker.ErrMultipleEntryOrders))
		})
	})

	Describe("buildOCOOrder", func() {
		It("builds an OCO order with SINGLE children", func() {
			orders := []broker.Order{
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Buy, Qty: 10, OrderType: broker.Limit, LimitPrice: 150.0, TimeInForce: broker.Day},
				{Asset: asset.Asset{Ticker: "AAPL"}, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 160.0, TimeInForce: broker.GTC},
			}

			result, buildErr := buildOCOOrder(orders)
			Expect(buildErr).ToNot(HaveOccurred())

			Expect(result.OrderStrategyType).To(Equal("OCO"))
			Expect(result.ChildOrderStrategies).To(HaveLen(2))
			Expect(result.ChildOrderStrategies[0].OrderStrategyType).To(Equal("SINGLE"))
			Expect(result.ChildOrderStrategies[1].OrderStrategyType).To(Equal("SINGLE"))
		})
	})
})
