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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

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
			[]float64{500, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		// Configure mock broker to fill at correct prices (keyed by asset
		// so map iteration order doesn't matter).
		mb := &mockBroker{}
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy:  {{Price: 500.0, Qty: 120, FilledAt: fill}},
			aapl: {{Price: 200.0, Qty: 200, FilledAt: fill}},
		}

		acct := portfolio.New(portfolio.WithCash(100_000), portfolio.WithBroker(mb))
		acct.UpdatePrices(df)

		acct.RebalanceTo(portfolio.Allocation{
			Date: t1,
			Members: map[asset.Asset]float64{
				spy:  0.60,
				aapl: 0.40,
			},
		})

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
			[]float64{500, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		// Seed the account: buy 200 SPY at $500 with enough cash.
		mbSetup := &mockBroker{
			fills: [][]broker.Fill{
				{{Price: 500.0, Qty: 200, FilledAt: fill}},
			},
		}
		acct := portfolio.New(portfolio.WithCash(100_000), portfolio.WithBroker(mbSetup))
		acct.Order(spy, portfolio.Buy, 200)
		// Now: 0 cash, 200 SPY worth $100k

		acct.UpdatePrices(df)
		Expect(acct.Value()).To(Equal(100_000.0))

		// Swap broker for rebalance tracking.
		mb := &mockBroker{}
		// Sells happen first, then buys.
		// Sell 100 SPY (200 held, target = floor(0.5*100000/500) = 100, diff = -100)
		// Buy 250 AAPL (0 held, target = floor(0.5*100000/200) = 250, diff = +250)
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy:  {{Price: 500.0, Qty: 100, FilledAt: fill}},
			aapl: {{Price: 200.0, Qty: 250, FilledAt: fill}},
		}
		acct.SetBroker(mb)

		acct.RebalanceTo(portfolio.Allocation{
			Date: t1,
			Members: map[asset.Asset]float64{
				spy:  0.50,
				aapl: 0.50,
			},
		})

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
			[]float64{500, 1000},
		)
		Expect(err).NotTo(HaveOccurred())

		mbSetup := &mockBroker{
			fills: [][]broker.Fill{
				{{Price: 500.0, Qty: 100, FilledAt: fill}},
			},
		}
		acct := portfolio.New(portfolio.WithCash(50_000), portfolio.WithBroker(mbSetup))
		acct.Order(spy, portfolio.Buy, 100)
		// Now: 0 cash, 100 SPY worth $50k

		acct.UpdatePrices(df)
		Expect(acct.Value()).To(Equal(50_000.0))

		mb := &mockBroker{}
		// Sell all SPY first, then buy GOOG.
		// target GOOG = floor(1.0 * 50000 / 1000) = 50
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy:  {{Price: 500.0, Qty: 100, FilledAt: fill}},
			goog: {{Price: 1000.0, Qty: 50, FilledAt: fill}},
		}
		acct.SetBroker(mb)

		acct.RebalanceTo(portfolio.Allocation{
			Date: t1,
			Members: map[asset.Asset]float64{
				goog: 1.0,
			},
		})

		Expect(acct.Position(spy)).To(Equal(0.0))
		Expect(acct.Position(goog)).To(Equal(50.0))

		// First order is sell SPY, second is buy GOOG.
		Expect(mb.submitted).To(HaveLen(2))
		Expect(mb.submitted[0].Side).To(Equal(broker.Sell))
		Expect(mb.submitted[0].Asset).To(Equal(spy))
		Expect(mb.submitted[1].Side).To(Equal(broker.Buy))
		Expect(mb.submitted[1].Asset).To(Equal(goog))
	})
})
