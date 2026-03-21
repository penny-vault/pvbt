// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package portfolio_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Batch", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		ts   time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL001", Ticker: "AAPL"}
		ts = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	})

	// buildPricedAccount creates an Account with the given cash and a price
	// DataFrame so that PositionValue works correctly.
	buildPricedAccount := func(cash float64, assets []asset.Asset, prices []float64) *portfolio.Account {
		acct := portfolio.New(portfolio.WithCash(cash, time.Time{}))
		df, err := data.NewDataFrame(
			[]time.Time{ts},
			assets,
			[]data.Metric{data.MetricClose},
			data.Daily,
			prices,
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdatePrices(df)

		return acct
	}

	Describe("NewBatch", func() {
		It("creates a batch with empty orders and annotations", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			Expect(batch.Timestamp).To(Equal(ts))
			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).To(BeEmpty())
			Expect(batch.Portfolio()).To(Equal(portfolio.Portfolio(acct)))
		})
	})

	Describe("Annotate", func() {
		It("stores a key-value pair in the annotations map", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			batch.Annotate("signal", "0.75")

			Expect(batch.Annotations).To(HaveKeyWithValue("signal", "0.75"))
		})

		It("overwrites the previous value when called again with the same key", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			batch.Annotate("signal", "0.75")
			batch.Annotate("signal", "0.90")

			Expect(batch.Annotations).To(HaveKeyWithValue("signal", "0.90"))
			Expect(batch.Annotations).To(HaveLen(1))
		})

		It("accumulates multiple distinct keys", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			batch.Annotate("alpha", "1.0")
			batch.Annotate("beta", "0.5")

			Expect(batch.Annotations).To(HaveLen(2))
			Expect(batch.Annotations["alpha"]).To(Equal("1.0"))
			Expect(batch.Annotations["beta"]).To(Equal("0.5"))
		})
	})

	Describe("Order", func() {
		It("accumulates a buy order without executing it", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			err := batch.Order(context.Background(), spy, portfolio.Buy, 10)
			Expect(err).NotTo(HaveOccurred())

			// Order was appended to the batch.
			Expect(batch.Orders).To(HaveLen(1))
			ord := batch.Orders[0]
			Expect(ord.Asset).To(Equal(spy))
			Expect(ord.Side).To(Equal(broker.Buy))
			Expect(ord.Qty).To(Equal(10.0))
			Expect(ord.OrderType).To(Equal(broker.Market))
			Expect(ord.TimeInForce).To(Equal(broker.Day))

			// Portfolio is unchanged: cash and position are the same.
			Expect(acct.Cash()).To(Equal(10_000.0))
			Expect(acct.Position(spy)).To(Equal(0.0))
		})

		It("accumulates a sell order", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			err := batch.Order(context.Background(), spy, portfolio.Sell, 5)
			Expect(err).NotTo(HaveOccurred())

			Expect(batch.Orders).To(HaveLen(1))
			Expect(batch.Orders[0].Side).To(Equal(broker.Sell))
		})

		It("accumulates multiple orders", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy, aapl}, []float64{100, 200})
			batch := portfolio.NewBatch(ts, acct)

			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 10)).To(Succeed())
			Expect(batch.Order(context.Background(), aapl, portfolio.Buy, 5)).To(Succeed())

			Expect(batch.Orders).To(HaveLen(2))
		})

		It("applies Limit modifier", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 5, portfolio.Limit(95.0))).To(Succeed())

			Expect(batch.Orders[0].OrderType).To(Equal(broker.Limit))
			Expect(batch.Orders[0].LimitPrice).To(Equal(95.0))
		})

		It("applies Stop modifier", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 5, portfolio.Stop(90.0))).To(Succeed())

			Expect(batch.Orders[0].OrderType).To(Equal(broker.Stop))
			Expect(batch.Orders[0].StopPrice).To(Equal(90.0))
		})

		It("applies StopLimit when both Limit and Stop are set", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 5,
				portfolio.Limit(95.0), portfolio.Stop(90.0))).To(Succeed())

			Expect(batch.Orders[0].OrderType).To(Equal(broker.StopLimit))
		})

		It("applies GoodTilCancel modifier", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 5, portfolio.GoodTilCancel)).To(Succeed())

			Expect(batch.Orders[0].TimeInForce).To(Equal(broker.GTC))
		})

		It("applies GoodTilDate modifier", func() {
			gtdDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 5, portfolio.GoodTilDate(gtdDate))).To(Succeed())

			Expect(batch.Orders[0].TimeInForce).To(Equal(broker.GTD))
			Expect(batch.Orders[0].GTDDate).To(Equal(gtdDate))
		})

		It("does not execute orders against the portfolio", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			initialCash := acct.Cash()
			initialPos := acct.Position(spy)

			batch := portfolio.NewBatch(ts, acct)
			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 50)).To(Succeed())
			Expect(batch.Order(context.Background(), spy, portfolio.Sell, 10)).To(Succeed())

			// Portfolio state is completely unchanged.
			Expect(acct.Cash()).To(Equal(initialCash))
			Expect(acct.Position(spy)).To(Equal(initialPos))
			Expect(acct.Transactions()).To(HaveLen(1)) // only the initial deposit
		})
	})

	Describe("ProjectedHoldings", func() {
		It("returns current holdings when there are no orders", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100,
				Amount: -1_000,
			})

			batch := portfolio.NewBatch(ts, acct)

			holdings := batch.ProjectedHoldings()
			Expect(holdings[spy]).To(Equal(10.0))
		})

		It("reflects a buy order added to existing holdings", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100,
				Amount: -1_000,
			})

			batch := portfolio.NewBatch(ts, acct)
			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 5)).To(Succeed())

			holdings := batch.ProjectedHoldings()
			// Should see 10 (held) + 5 (batch buy) = 15
			Expect(holdings[spy]).To(Equal(15.0))
		})

		It("reflects a buy order for an asset with no existing position", func() {
			// SPY is held and priced; AAPL is priced but not held.
			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy, aapl},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[]float64{100, 200},
			)
			Expect(err).NotTo(HaveOccurred())

			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			acct.UpdatePrices(df)
			// Only buy SPY so we have a price for it via Position/PositionValue.
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100,
				Amount: -1_000,
			})

			batch := portfolio.NewBatch(ts, acct)
			// Buy AAPL by qty (not dollar amount) -- should be reflected.
			Expect(batch.Order(context.Background(), aapl, portfolio.Buy, 3)).To(Succeed())

			holdings := batch.ProjectedHoldings()
			Expect(holdings[spy]).To(Equal(10.0))
			Expect(holdings[aapl]).To(Equal(3.0))
		})

		It("reflects a sell order reducing an existing position", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			// Set up holdings via Record (simulating prior execution).
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  100,
				Amount: -2_000,
			})

			batch := portfolio.NewBatch(ts, acct)
			Expect(batch.Order(context.Background(), spy, portfolio.Sell, 8)).To(Succeed())

			holdings := batch.ProjectedHoldings()
			Expect(holdings[spy]).To(Equal(12.0))
		})

		It("removes asset from projected holdings when fully sold", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100,
				Amount: -1_000,
			})

			batch := portfolio.NewBatch(ts, acct)
			Expect(batch.Order(context.Background(), spy, portfolio.Sell, 10)).To(Succeed())

			holdings := batch.ProjectedHoldings()
			Expect(holdings).NotTo(HaveKey(spy))
		})

		It("reflects a dollar-amount buy for an unheld asset after RebalanceTo", func() {
			// SPY is held at $100/share. AAPL is priced at $200 but not held.
			// Total value = $10000 (cash only, no holdings to start).
			// RebalanceTo 100% AAPL should create a dollar-amount buy for AAPL.
			// ProjectedHoldings must convert that to shares using the price fallback:
			//   floor(10000 / 200) = 50 shares of AAPL.
			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy, aapl},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[]float64{100, 200},
			)
			Expect(err).NotTo(HaveOccurred())

			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			acct.UpdatePrices(df)
			// No holdings; AAPL price is only available via the price DataFrame.

			batch := portfolio.NewBatch(ts, acct)
			err = batch.RebalanceTo(context.Background(), portfolio.Allocation{
				Date:    ts,
				Members: map[asset.Asset]float64{aapl: 1.0},
			})
			Expect(err).NotTo(HaveOccurred())

			holdings := batch.ProjectedHoldings()
			// floor(10000 / 200) = 50 shares
			Expect(holdings[aapl]).To(Equal(50.0))
		})

		It("converts dollar-amount buy to shares using math.Floor", func() {
			// SPY at $100: a $350 buy should give floor(350/100) = 3 shares.
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    1,
				Price:  100,
				Amount: -100,
			})

			batch := portfolio.NewBatch(ts, acct)
			// Simulate what RebalanceTo does: append a dollar-amount buy.
			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 0)).To(Succeed())

			// Manually append a dollar-amount order to test conversion logic.
			batch.Orders[0].Amount = 350
			batch.Orders[0].Qty = 0

			holdings := batch.ProjectedHoldings()
			// 1 existing + floor(350/100) = 3 batch = 4
			Expect(holdings[spy]).To(Equal(4.0))
		})
	})

	Describe("ProjectedValue", func() {
		It("returns cash-only value when no holdings and no orders", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			Expect(batch.ProjectedValue()).To(BeNumerically("~", 10_000.0, 1e-9))
		})

		It("includes existing holdings marked to current prices", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{200})
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  200,
				Amount: -2_000,
			})
			// cash = 8000, 10 SPY @ 200 = 2000, total = 10000

			batch := portfolio.NewBatch(ts, acct)
			Expect(batch.ProjectedValue()).To(BeNumerically("~", 10_000.0, 1e-9))
		})

		It("adjusts value for pending buy orders", func() {
			// After buying 5 more shares at $200: cash drops, holdings go up.
			// Net value should not change (ignoring slippage).
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{200})
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  200,
				Amount: -2_000,
			})
			// cash = 8000, 10 SPY @ 200 = 2000

			batch := portfolio.NewBatch(ts, acct)
			// Buy 5 more by qty.
			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 5)).To(Succeed())

			// cash projected: 8000 - 5*200 = 7000
			// holdings projected: 15 * 200 = 3000
			// total = 10000
			Expect(batch.ProjectedValue()).To(BeNumerically("~", 10_000.0, 1e-9))
		})
	})

	Describe("ProjectedWeights", func() {
		It("returns empty map when projected value is zero", func() {
			acct := portfolio.New(portfolio.WithCash(0, time.Time{}))
			batch := portfolio.NewBatch(ts, acct)

			weights := batch.ProjectedWeights()
			Expect(weights).To(BeEmpty())
		})

		It("returns correct weights for cash-only portfolio", func() {
			// No holdings, so no asset weights; only cash.
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			weights := batch.ProjectedWeights()
			Expect(weights).To(BeEmpty())
		})

		It("returns correct weights after a buy order", func() {
			// Buy 50 shares at $100: total value stays 10000.
			// SPY weight = (50*100)/10000 = 0.5.
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    50,
				Price:  100,
				Amount: -5_000,
			})
			// cash = 5000, 50 SPY @ 100 = 5000

			batch := portfolio.NewBatch(ts, acct)

			weights := batch.ProjectedWeights()
			Expect(weights[spy]).To(BeNumerically("~", 0.5, 1e-9))
		})

		It("returns correct weights for two held assets", func() {
			// SPY: 40 shares @ $100 = $4000 (40%)
			// AAPL: 10 shares @ $200 = $2000 (20%)
			// cash: $4000 (40%, not in weights map)
			// total = $10000
			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy, aapl},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[]float64{100, 200},
			)
			Expect(err).NotTo(HaveOccurred())

			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			acct.UpdatePrices(df)
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    40,
				Price:  100,
				Amount: -4_000,
			})
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  aapl,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  200,
				Amount: -2_000,
			})
			// cash = 4000, 40 SPY @ 100 = 4000, 10 AAPL @ 200 = 2000, total = 10000

			batch := portfolio.NewBatch(ts, acct)
			weights := batch.ProjectedWeights()

			Expect(weights[spy]).To(BeNumerically("~", 0.4, 1e-9))
			Expect(weights[aapl]).To(BeNumerically("~", 0.2, 1e-9))
			// Weights sum to 0.6 (cash portion not included).
			total := weights[spy] + weights[aapl]
			Expect(total).To(BeNumerically("~", 0.6, 1e-9))
		})
	})

	Describe("OCO modifier", func() {
		It("expands into 2 orders with correct types, prices, GroupRole, and shared GroupID", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			err := batch.Order(context.Background(), spy, portfolio.Sell, 10,
				portfolio.OCO(portfolio.StopLeg(90.0), portfolio.LimitLeg(115.0)))
			Expect(err).NotTo(HaveOccurred())

			Expect(batch.Orders).To(HaveLen(2))

			legA := batch.Orders[0]
			Expect(legA.Asset).To(Equal(spy))
			Expect(legA.Side).To(Equal(broker.Sell))
			Expect(legA.Qty).To(Equal(10.0))
			Expect(legA.OrderType).To(Equal(broker.Stop))
			Expect(legA.StopPrice).To(Equal(90.0))
			Expect(legA.GroupRole).To(Equal(broker.RoleStopLoss))
			Expect(legA.GroupID).NotTo(BeEmpty())

			legB := batch.Orders[1]
			Expect(legB.Asset).To(Equal(spy))
			Expect(legB.Side).To(Equal(broker.Sell))
			Expect(legB.Qty).To(Equal(10.0))
			Expect(legB.OrderType).To(Equal(broker.Limit))
			Expect(legB.LimitPrice).To(Equal(115.0))
			Expect(legB.GroupRole).To(Equal(broker.RoleTakeProfit))
			Expect(legB.GroupID).To(Equal(legA.GroupID))
		})

		It("records in Groups() with Type=GroupOCO and EntryIndex=-1", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			err := batch.Order(context.Background(), spy, portfolio.Sell, 10,
				portfolio.OCO(portfolio.StopLeg(90.0), portfolio.LimitLeg(115.0)))
			Expect(err).NotTo(HaveOccurred())

			groups := batch.Groups()
			Expect(groups).To(HaveLen(1))
			Expect(groups[0].Type).To(Equal(broker.GroupOCO))
			Expect(groups[0].EntryIndex).To(Equal(-1))
			Expect(groups[0].GroupID).NotTo(BeEmpty())
			Expect(groups[0].GroupID).To(Equal(batch.Orders[0].GroupID))
		})

		It("creates separate GroupIDs for two OCO calls", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			Expect(batch.Order(context.Background(), spy, portfolio.Sell, 10,
				portfolio.OCO(portfolio.StopLeg(90.0), portfolio.LimitLeg(115.0)))).To(Succeed())
			Expect(batch.Order(context.Background(), aapl, portfolio.Sell, 5,
				portfolio.OCO(portfolio.StopLeg(180.0), portfolio.LimitLeg(220.0)))).To(Succeed())

			groups := batch.Groups()
			Expect(groups).To(HaveLen(2))
			Expect(groups[0].GroupID).NotTo(Equal(groups[1].GroupID))
		})
	})

	Describe("WithBracket modifier", func() {
		It("adds 1 entry order with GroupRole=RoleEntry and non-empty GroupID", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			err := batch.Order(context.Background(), spy, portfolio.Buy, 10,
				portfolio.WithBracket(portfolio.StopLossPrice(90.0), portfolio.TakeProfitPrice(115.0)))
			Expect(err).NotTo(HaveOccurred())

			Expect(batch.Orders).To(HaveLen(1))

			entry := batch.Orders[0]
			Expect(entry.Asset).To(Equal(spy))
			Expect(entry.Side).To(Equal(broker.Buy))
			Expect(entry.Qty).To(Equal(10.0))
			Expect(entry.GroupRole).To(Equal(broker.RoleEntry))
			Expect(entry.GroupID).NotTo(BeEmpty())
		})

		It("records in Groups() with Type=GroupBracket, correct EntryIndex, and exit targets", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			batch := portfolio.NewBatch(ts, acct)

			stopLoss := portfolio.StopLossPrice(90.0)
			takeProfit := portfolio.TakeProfitPrice(115.0)

			err := batch.Order(context.Background(), spy, portfolio.Buy, 10,
				portfolio.WithBracket(stopLoss, takeProfit))
			Expect(err).NotTo(HaveOccurred())

			groups := batch.Groups()
			Expect(groups).To(HaveLen(1))
			Expect(groups[0].Type).To(Equal(broker.GroupBracket))
			Expect(groups[0].EntryIndex).To(Equal(0))
			Expect(groups[0].GroupID).NotTo(BeEmpty())
			Expect(groups[0].GroupID).To(Equal(batch.Orders[0].GroupID))
			Expect(groups[0].StopLoss).To(Equal(stopLoss))
			Expect(groups[0].TakeProfit).To(Equal(takeProfit))
		})

		It("records EntryIndex pointing to the correct order when preceding orders exist", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy, aapl}, []float64{100, 200})
			batch := portfolio.NewBatch(ts, acct)

			// Add a plain order first (index 0).
			Expect(batch.Order(context.Background(), aapl, portfolio.Buy, 5)).To(Succeed())

			// Bracket order should land at index 1.
			Expect(batch.Order(context.Background(), spy, portfolio.Buy, 10,
				portfolio.WithBracket(portfolio.StopLossPrice(90.0), portfolio.TakeProfitPrice(115.0)))).To(Succeed())

			Expect(batch.Orders).To(HaveLen(2))
			groups := batch.Groups()
			Expect(groups).To(HaveLen(1))
			Expect(groups[0].EntryIndex).To(Equal(1))
			Expect(batch.Orders[1].GroupRole).To(Equal(broker.RoleEntry))
		})
	})

	Describe("RebalanceTo", func() {
		It("appends sell and buy orders to reach target allocation", func() {
			// Account: 10 SPY @ 100, cash = 0, total = 1000.
			// Target: 50% SPY, 50% cash (i.e. only SPY in members at 0.5).
			// Expected: sell 5 SPY.
			acct := buildPricedAccount(1_000, []asset.Asset{spy}, []float64{100})
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100,
				Amount: -1_000,
			})
			// cash = 0, 10 SPY @ 100 = 1000

			batch := portfolio.NewBatch(ts, acct)
			err := batch.RebalanceTo(context.Background(), portfolio.Allocation{
				Date: ts,
				Members: map[asset.Asset]float64{
					spy: 0.5,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Should have appended a sell order for the overweight portion.
			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).NotTo(BeEmpty())

			// No orders are submitted to a broker.
			Expect(acct.Position(spy)).To(Equal(10.0))
		})

		It("appends buy orders for underweight positions", func() {
			// Account: 0 SPY, cash = 10000, total = 10000.
			// Target: 100% SPY.
			// Expected: buy order for the full value.
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})

			batch := portfolio.NewBatch(ts, acct)
			err := batch.RebalanceTo(context.Background(), portfolio.Allocation{
				Date:    ts,
				Members: map[asset.Asset]float64{spy: 1.0},
			})
			Expect(err).NotTo(HaveOccurred())

			buys := ordersWithSide(batch.Orders, broker.Buy)
			Expect(buys).NotTo(BeEmpty())
			Expect(buys[0].Asset).To(Equal(spy))
		})

		It("appends sell orders for assets not in target", func() {
			// Account: 10 SPY @ 100. Target: only AAPL.
			// Expected: sell all SPY, then buy AAPL.
			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy, aapl},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[]float64{100, 200},
			)
			Expect(err).NotTo(HaveOccurred())

			acct := portfolio.New(portfolio.WithCash(1_000, time.Time{}))
			acct.UpdatePrices(df)
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  spy,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100,
				Amount: -1_000,
			})
			// cash = 0, 10 SPY @ 100 = 1000

			batch := portfolio.NewBatch(ts, acct)
			err = batch.RebalanceTo(context.Background(), portfolio.Allocation{
				Date:    ts,
				Members: map[asset.Asset]float64{aapl: 1.0},
			})
			Expect(err).NotTo(HaveOccurred())

			sells := ordersWithSide(batch.Orders, broker.Sell)
			spySells := ordersForAsset(sells, spy)
			Expect(spySells).NotTo(BeEmpty())
		})

		It("does not execute orders against the portfolio", func() {
			acct := buildPricedAccount(10_000, []asset.Asset{spy}, []float64{100})
			initialCash := acct.Cash()

			batch := portfolio.NewBatch(ts, acct)
			Expect(batch.RebalanceTo(context.Background(), portfolio.Allocation{
				Date:    ts,
				Members: map[asset.Asset]float64{spy: 1.0},
			})).To(Succeed())

			// Portfolio cash is unchanged.
			Expect(acct.Cash()).To(Equal(initialCash))
		})
	})
})

// ordersWithSide filters a slice of orders by side.
func ordersWithSide(orders []broker.Order, side broker.Side) []broker.Order {
	var result []broker.Order

	for _, ord := range orders {
		if ord.Side == side {
			result = append(result, ord)
		}
	}

	return result
}

// ordersForAsset filters a slice of orders by asset.
func ordersForAsset(orders []broker.Order, ast asset.Asset) []broker.Order {
	var result []broker.Order

	for _, ord := range orders {
		if ord.Asset == ast {
			result = append(result, ord)
		}
	}

	return result
}
