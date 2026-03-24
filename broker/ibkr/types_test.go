package ibkr_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/broker/ibkr"
)

var _ = Describe("Types", func() {
	Describe("toIBOrder", func() {
		It("translates a market buy order", func() {
			order := broker.Order{
				ID:          "test-1",
				Asset:       asset.Asset{Ticker: "AAPL"},
				Side:        broker.Buy,
				Qty:         100,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			result, translateErr := ibkr.ToIBOrder(order, 265598)
			Expect(translateErr).ToNot(HaveOccurred())
			Expect(result.OrderType).To(Equal("MKT"))
			Expect(result.Side).To(Equal("BUY"))
			Expect(result.Tif).To(Equal("DAY"))
			Expect(result.Quantity).To(BeNumerically("==", 100))
			Expect(result.Conid).To(Equal(int64(265598)))
		})

		It("translates a limit sell order", func() {
			order := broker.Order{
				ID:          "test-2",
				Asset:       asset.Asset{Ticker: "MSFT"},
				Side:        broker.Sell,
				Qty:         50,
				OrderType:   broker.Limit,
				LimitPrice:  150.00,
				TimeInForce: broker.GTC,
			}
			result, translateErr := ibkr.ToIBOrder(order, 272093)
			Expect(translateErr).ToNot(HaveOccurred())
			Expect(result.OrderType).To(Equal("LMT"))
			Expect(result.Side).To(Equal("SELL"))
			Expect(result.Price).To(BeNumerically("==", 150.00))
			Expect(result.Tif).To(Equal("GTC"))
		})

		It("translates a stop-limit order with both prices", func() {
			order := broker.Order{
				ID:          "test-3",
				Side:        broker.Buy,
				Qty:         25,
				OrderType:   broker.StopLimit,
				LimitPrice:  155.00,
				StopPrice:   150.00,
				TimeInForce: broker.Day,
			}
			result, translateErr := ibkr.ToIBOrder(order, 265598)
			Expect(translateErr).ToNot(HaveOccurred())
			Expect(result.OrderType).To(Equal("STP_LIMIT"))
			Expect(result.Price).To(BeNumerically("==", 155.00))
			Expect(result.AuxPrice).To(BeNumerically("==", 150.00))
		})

		It("maps all supported time-in-force values", func() {
			for _, tc := range []struct {
				input    broker.TimeInForce
				expected string
			}{
				{broker.Day, "DAY"},
				{broker.GTC, "GTC"},
				{broker.IOC, "IOC"},
				{broker.OnOpen, "OPG"},
				{broker.OnClose, "MOC"},
			} {
				order := broker.Order{
					Side: broker.Buy, Qty: 1, OrderType: broker.Market,
					TimeInForce: tc.input,
				}
				result, translateErr := ibkr.ToIBOrder(order, 1)
				Expect(translateErr).ToNot(HaveOccurred(), "for TIF %d", tc.input)
				Expect(result.Tif).To(Equal(tc.expected), "for TIF %d", tc.input)
			}
		})

		It("returns error for unsupported GTD time-in-force", func() {
			order := broker.Order{Side: broker.Buy, Qty: 1, OrderType: broker.Market, TimeInForce: broker.GTD}
			_, translateErr := ibkr.ToIBOrder(order, 1)
			Expect(translateErr).To(MatchError(ContainSubstring("unsupported")))
		})

		It("returns error for unsupported FOK time-in-force", func() {
			order := broker.Order{Side: broker.Buy, Qty: 1, OrderType: broker.Market, TimeInForce: broker.FOK}
			_, translateErr := ibkr.ToIBOrder(order, 1)
			Expect(translateErr).To(MatchError(ContainSubstring("unsupported")))
		})
	})

	Describe("toBrokerOrder", func() {
		It("maps Submitted status to OrderOpen", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "123", Status: "Submitted", Side: "BUY",
				OrderType: "LMT", Ticker: "AAPL", Conid: 265598,
				FilledQuantity: 0, RemainingQuantity: 100, TotalQuantity: 100,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderOpen))
		})

		It("maps PreSubmitted status to OrderSubmitted", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "124", Status: "PreSubmitted", Side: "SELL",
				OrderType: "MKT", Ticker: "MSFT", Conid: 272093,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderSubmitted))
		})

		It("maps Filled status to OrderFilled", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "125", Status: "Filled", Side: "BUY",
				OrderType: "LMT", Ticker: "AAPL", Conid: 265598,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderFilled))
		})

		It("maps Cancelled status to OrderCancelled", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "126", Status: "Cancelled", Side: "BUY",
				OrderType: "MKT", Ticker: "GOOG", Conid: 208813720,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderCancelled))
		})

		It("maps PartiallyFilled status to OrderPartiallyFilled", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "126b", Status: "PartiallyFilled", Side: "BUY",
				OrderType: "LMT", Ticker: "AAPL", Conid: 265598,
				FilledQuantity: 50, RemainingQuantity: 50, TotalQuantity: 100,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderPartiallyFilled))
		})

		It("maps Inactive status to OrderCancelled", func() {
			ibOrder := ibkr.IBOrderResponse{
				OrderID: "127", Status: "Inactive", Side: "BUY",
				OrderType: "MKT", Ticker: "GOOG", Conid: 208813720,
			}
			result := ibkr.ToBrokerOrder(ibOrder)
			Expect(result.Status).To(Equal(broker.OrderCancelled))
		})
	})

	Describe("toBrokerPosition", func() {
		It("maps IB position to broker position", func() {
			ibPos := ibkr.IBPositionEntry{
				ContractId: 265598, Position: 100, AvgCost: 150.50,
				MktPrice: 155.25, Ticker: "AAPL", Currency: "USD",
			}
			result := ibkr.ToBrokerPosition(ibPos)
			Expect(result.Qty).To(BeNumerically("==", 100))
			Expect(result.AvgOpenPrice).To(BeNumerically("==", 150.50))
			Expect(result.MarkPrice).To(BeNumerically("==", 155.25))
		})
	})

	Describe("toBrokerBalance", func() {
		It("maps IB summary to broker balance", func() {
			summary := ibkr.IBAccountSummary{
				CashBalance:    ibkr.SummaryValue{Amount: 50000.00},
				NetLiquidation: ibkr.SummaryValue{Amount: 150000.00},
				BuyingPower:    ibkr.SummaryValue{Amount: 200000.00},
				MaintMarginReq: ibkr.SummaryValue{Amount: 75000.00},
			}
			result := ibkr.ToBrokerBalance(summary)
			Expect(result.CashBalance).To(BeNumerically("==", 50000.00))
			Expect(result.NetLiquidatingValue).To(BeNumerically("==", 150000.00))
			Expect(result.EquityBuyingPower).To(BeNumerically("==", 200000.00))
			Expect(result.MaintenanceReq).To(BeNumerically("==", 75000.00))
		})
	})
})
