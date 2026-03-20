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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

func filterTransactions(txns []portfolio.Transaction, txType portfolio.TransactionType) []portfolio.Transaction {
	var result []portfolio.Transaction
	for _, tx := range txns {
		if tx.Type == txType {
			result = append(result, tx)
		}
	}
	return result
}

var _ = Describe("RebalanceTo", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		goog asset.Asset
		t1   time.Time
		fill time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL001", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "GOOG001", Ticker: "GOOG"}
		t1 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		fill = time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	})

	It("buys to match a single allocation from cash", func() {
		// 60% SPY at $500, 40% AAPL at $200, $100k cash
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		// Configure mock broker to fill at correct prices (keyed by asset
		// so map iteration order doesn't matter).
		mb := newMockBroker()
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy:  {{Price: 500.0, Qty: 120, FilledAt: fill}},
			aapl: {{Price: 200.0, Qty: 200, FilledAt: fill}},
		}

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mb))
		acct.UpdatePrices(df)

		err = acct.RebalanceTo(context.Background(), portfolio.Allocation{
			Date: t1,
			Members: map[asset.Asset]float64{
				spy:  0.60,
				aapl: 0.40,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		// Target shares: SPY = floor(0.60 * 100000 / 500) = 120
		//                AAPL = floor(0.40 * 100000 / 200) = 200
		Expect(acct.Position(spy)).To(Equal(120.0))
		Expect(acct.Position(aapl)).To(Equal(200.0))

		// Cash: 100000 - 120*500 - 200*200 = 100000 - 60000 - 40000 = 0
		Expect(acct.Cash()).To(Equal(0.0))

		// All orders should be buys
		Expect(mb.submitted).To(HaveLen(2))
		for _, ord := range mb.submitted {
			Expect(ord.Side).To(Equal(broker.Buy))
		}
	})

	It("sells excess and buys new to rebalance", func() {
		// Start with 200 shares SPY, rebalance to 50/50 SPY/AAPL.
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		// Seed the account: buy 200 SPY at $500 with enough cash.
		mbSetup := newMockBroker()
		mbSetup.fills = [][]broker.Fill{
			{{Price: 500.0, Qty: 200, FilledAt: fill}},
		}
		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mbSetup))
		Expect(acct.Order(context.Background(), spy, portfolio.Buy, 200)).To(Succeed())
		// Now: 0 cash, 200 SPY worth $100k

		acct.UpdatePrices(df)
		Expect(acct.Value()).To(Equal(100_000.0))

		// Swap broker for rebalance tracking.
		mb := newMockBroker()
		// Sells happen first, then buys.
		// Sell 100 SPY (200 held, target = floor(0.5*100000/500) = 100, diff = -100)
		// Buy 250 AAPL (0 held, target = floor(0.5*100000/200) = 250, diff = +250)
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy:  {{Price: 500.0, Qty: 100, FilledAt: fill}},
			aapl: {{Price: 200.0, Qty: 250, FilledAt: fill}},
		}
		acct.SetBroker(mb)

		err = acct.RebalanceTo(context.Background(), portfolio.Allocation{
			Date: t1,
			Members: map[asset.Asset]float64{
				spy:  0.50,
				aapl: 0.50,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(acct.Position(spy)).To(Equal(100.0))
		Expect(acct.Position(aapl)).To(Equal(250.0))

		// Sells come first.
		Expect(mb.submitted).To(HaveLen(2))
		Expect(mb.submitted[0].Side).To(Equal(broker.Sell))
		Expect(mb.submitted[1].Side).To(Equal(broker.Buy))
	})

	It("handles assets not in target", func() {
		// Start with 100 SPY, rebalance to 100% GOOG.
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, goog},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500, 1000},
		)
		Expect(err).NotTo(HaveOccurred())

		mbSetup := newMockBroker()
		mbSetup.fills = [][]broker.Fill{
			{{Price: 500.0, Qty: 100, FilledAt: fill}},
		}
		acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}), portfolio.WithBroker(mbSetup))
		Expect(acct.Order(context.Background(), spy, portfolio.Buy, 100)).To(Succeed())
		// Now: 0 cash, 100 SPY worth $50k

		acct.UpdatePrices(df)
		Expect(acct.Value()).To(Equal(50_000.0))

		mb := newMockBroker()
		// Sell all SPY first, then buy GOOG.
		// target GOOG = floor(1.0 * 50000 / 1000) = 50
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy:  {{Price: 500.0, Qty: 100, FilledAt: fill}},
			goog: {{Price: 1000.0, Qty: 50, FilledAt: fill}},
		}
		acct.SetBroker(mb)

		err = acct.RebalanceTo(context.Background(), portfolio.Allocation{
			Date: t1,
			Members: map[asset.Asset]float64{
				goog: 1.0,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(acct.Position(spy)).To(Equal(0.0))
		Expect(acct.Position(goog)).To(Equal(50.0))

		// First order is sell SPY, second is buy GOOG.
		Expect(mb.submitted).To(HaveLen(2))
		Expect(mb.submitted[0].Side).To(Equal(broker.Sell))
		Expect(mb.submitted[0].Asset).To(Equal(spy))
		Expect(mb.submitted[1].Side).To(Equal(broker.Buy))
		Expect(mb.submitted[1].Asset).To(Equal(goog))
	})

	It("returns an error when broker rejects an order", func() {
		mb := newMockBroker()
		mb.submitErr = fmt.Errorf("no price for SPY")
		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mb))

		err := acct.RebalanceTo(context.Background(), portfolio.Allocation{
			Date: t1,
			Members: map[asset.Asset]float64{
				spy:  0.50,
				aapl: 0.50,
			},
		})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no price"))
	})

	It("is a no-op when allocation has empty Members and no holdings", func() {
		mb := newMockBroker()
		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mb))

		err := acct.RebalanceTo(context.Background(), portfolio.Allocation{
			Date:    t1,
			Members: map[asset.Asset]float64{},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(mb.submitted).To(BeEmpty())
		Expect(acct.Cash()).To(Equal(100_000.0))
	})

	It("copies Allocation.Justification onto generated transactions", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500},
		)
		Expect(err).NotTo(HaveOccurred())

		mb := newMockBroker()
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy: {{Price: 500.0, Qty: 100, FilledAt: fill}},
		}

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mb))
		acct.UpdatePrices(df)

		err = acct.RebalanceTo(context.Background(), portfolio.Allocation{
			Date:          t1,
			Members:       map[asset.Asset]float64{spy: 1.0},
			Justification: "momentum crossover signal",
		})
		Expect(err).NotTo(HaveOccurred())

		txns := acct.Transactions()
		buyTxns := filterTransactions(txns, portfolio.BuyTransaction)
		Expect(buyTxns).NotTo(BeEmpty())
		Expect(buyTxns[0].Justification).To(Equal("momentum crossover signal"))
	})

	It("leaves Justification empty when Allocation has no justification", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500},
		)
		Expect(err).NotTo(HaveOccurred())

		mb := newMockBroker()
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy: {{Price: 500.0, Qty: 100, FilledAt: fill}},
		}

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mb))
		acct.UpdatePrices(df)

		err = acct.RebalanceTo(context.Background(), portfolio.Allocation{
			Date:    t1,
			Members: map[asset.Asset]float64{spy: 1.0},
		})
		Expect(err).NotTo(HaveOccurred())

		txns := acct.Transactions()
		buyTxns := filterTransactions(txns, portfolio.BuyTransaction)
		Expect(buyTxns).NotTo(BeEmpty())
		Expect(buyTxns[0].Justification).To(BeEmpty())
	})

	It("processes multiple variadic allocations", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		mb := newMockBroker()
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy:  {{Price: 500.0, Qty: 120, FilledAt: fill}},
			aapl: {{Price: 200.0, Qty: 200, FilledAt: fill}},
		}

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mb))
		acct.UpdatePrices(df)

		// Pass two allocations with separate assets.
		// First allocation buys SPY, second sells SPY (not in members)
		// and buys AAPL. So we expect 3 orders total: buy SPY, sell SPY, buy AAPL.
		acct.RebalanceTo(context.Background(),
			portfolio.Allocation{
				Date: t1,
				Members: map[asset.Asset]float64{
					spy: 0.60,
				},
			},
			portfolio.Allocation{
				Date: t1,
				Members: map[asset.Asset]float64{
					aapl: 0.40,
				},
			},
		)

		// Both allocations were processed: buy SPY, sell SPY, buy AAPL.
		Expect(mb.submitted).To(HaveLen(3))
		Expect(mb.submitted[0].Side).To(Equal(broker.Buy))
		Expect(mb.submitted[0].Asset).To(Equal(spy))
		Expect(mb.submitted[1].Side).To(Equal(broker.Sell))
		Expect(mb.submitted[1].Asset).To(Equal(spy))
		Expect(mb.submitted[2].Side).To(Equal(broker.Buy))
		Expect(mb.submitted[2].Asset).To(Equal(aapl))
	})
})

var _ = Describe("Order WithJustification", func() {
	It("attaches justification to the resulting transaction", func() {
		spyAsset := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		tradeDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		fillTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{tradeDate},
			[]asset.Asset{spyAsset},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500},
		)
		Expect(err).NotTo(HaveOccurred())

		mb := newMockBroker()
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spyAsset: {{Price: 500.0, Qty: 10, FilledAt: fillTime}},
		}

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mb))
		acct.UpdatePrices(df)

		err = acct.Order(context.Background(), spyAsset, portfolio.Buy, 10,
			portfolio.WithJustification("price below 200-day MA"),
		)
		Expect(err).NotTo(HaveOccurred())

		txns := acct.Transactions()
		buyTxns := filterTransactions(txns, portfolio.BuyTransaction)
		Expect(buyTxns).NotTo(BeEmpty())
		Expect(buyTxns[0].Justification).To(Equal("price below 200-day MA"))
	})
})
