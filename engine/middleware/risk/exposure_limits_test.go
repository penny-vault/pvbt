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

package risk_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine/middleware/risk"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Exposure Limits", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		ts   time.Time
		ctx  context.Context
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL001", Ticker: "AAPL"}
		ts = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
		ctx = context.Background()
	})

	// buildLongAccount creates an account with cash and a long position.
	buildLongAccount := func(cash float64, ast asset.Asset, price float64, qty float64) *portfolio.Account {
		acct := portfolio.New(portfolio.WithCash(cash+price*qty, time.Time{}))

		df, err := data.NewDataFrame(
			[]time.Time{ts},
			[]asset.Asset{ast},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[][]float64{{price}},
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdatePrices(df)

		if qty > 0 {
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  ast,
				Type:   asset.BuyTransaction,
				Qty:    qty,
				Price:  price,
				Amount: -(price * qty),
			})
		}

		return acct
	}

	// buildLongShortAccount creates an account with a long and a short position.
	buildLongShortAccount := func(cash float64,
		longAsset asset.Asset, longPrice float64, longQty float64,
		shortAsset asset.Asset, shortPrice float64, shortQty float64,
	) *portfolio.Account {
		// Start with enough cash for the long + short proceeds.
		initialCash := cash + longPrice*longQty + shortPrice*shortQty
		acct := portfolio.New(portfolio.WithCash(initialCash, time.Time{}))

		// Record the long buy.
		acct.Record(portfolio.Transaction{
			Date:   ts,
			Asset:  longAsset,
			Type:   asset.BuyTransaction,
			Qty:    longQty,
			Price:  longPrice,
			Amount: -(longPrice * longQty),
		})

		// Record the short sell.
		acct.Record(portfolio.Transaction{
			Date:   ts,
			Asset:  shortAsset,
			Type:   asset.SellTransaction,
			Qty:    shortQty,
			Price:  shortPrice,
			Amount: 0,
		})

		df, err := data.NewDataFrame(
			[]time.Time{ts},
			[]asset.Asset{longAsset, shortAsset},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[][]float64{{longPrice}, {shortPrice}},
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdatePrices(df)

		return acct
	}

	Describe("GrossExposureLimit", func() {
		It("drops orders that would exceed the gross exposure limit", func() {
			// Start with $50000 cash, long 100 SPY @ $200 = $20000 LMV.
			// Equity = $50000, LMV = $20000, SMV = $0.
			// Gross = 20000/50000 = 0.40.
			// Adding a $20000 short would make gross = (20000+20000)/50000 = 0.80.
			// With limit of 0.60, the short order should be dropped.
			acct := buildLongAccount(30_000, spy, 200, 100)
			batch := portfolio.NewBatch(ts, acct)
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       aapl,
				Side:        broker.Sell,
				Amount:      20_000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			mw := risk.GrossExposureLimit(0.60)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).To(HaveKey("risk:gross-exposure-limit"))
		})

		It("keeps orders that stay within the gross exposure limit", func() {
			// $50000 cash, no positions. Equity = $50000.
			// Buying $10000 SPY => gross = 10000/50000 = 0.20. Under 1.0 limit.
			acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))

			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[][]float64{{200}},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(df)

			batch := portfolio.NewBatch(ts, acct)
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       spy,
				Side:        broker.Buy,
				Amount:      10_000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			mw := risk.GrossExposureLimit(1.0)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(HaveLen(1))
		})

		It("allows sells that close long positions (reducing gross exposure)", func() {
			// Long 100 SPY @ $200 = $20000 LMV. Cash = $30000. Equity = $50000.
			// Gross = 20000/50000 = 0.40.
			// Selling 50 SPY ($10000) reduces LMV to $10000.
			// New gross = 10000/50000 = 0.20. Well under limit.
			acct := buildLongAccount(30_000, spy, 200, 100)
			batch := portfolio.NewBatch(ts, acct)
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       spy,
				Side:        broker.Sell,
				Amount:      10_000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			mw := risk.GrossExposureLimit(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(HaveLen(1))
		})

		It("handles zero equity gracefully", func() {
			acct := portfolio.New(portfolio.WithCash(0, time.Time{}))
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.GrossExposureLimit(1.0)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
		})
	})

	Describe("NetExposureLimit", func() {
		It("drops orders that would push net exposure above the limit", func() {
			// Long 200 SPY @ $200 = $40000 LMV. Short 100 AAPL @ $150 = $15000 SMV.
			// Cash = initial $50000 + short proceeds.
			// Equity = cash + LMV - SMV.
			// Net = abs(LMV - SMV) / equity = abs(40000 - 15000) / equity.
			// Buying another $30000 SPY would increase net exposure significantly.
			acct := buildLongShortAccount(50_000, spy, 200, 200, aapl, 150, 100)
			equity := acct.Equity()

			batch := portfolio.NewBatch(ts, acct)
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       spy,
				Side:        broker.Buy,
				Amount:      30_000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			// Set a tight net exposure limit that the buy would breach.
			currentNet := (acct.LongMarketValue() - acct.ShortMarketValue()) / equity
			limitNet := currentNet + 0.01 // just barely above current, so $30000 buy breaches

			mw := risk.NetExposureLimit(limitNet)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).To(HaveKey("risk:net-exposure-limit"))
		})

		It("keeps orders that stay within the net exposure limit", func() {
			// Cash only portfolio with generous limit.
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[][]float64{{200}},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(df)

			batch := portfolio.NewBatch(ts, acct)
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       spy,
				Side:        broker.Buy,
				Amount:      10_000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			mw := risk.NetExposureLimit(1.0)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(HaveLen(1))
		})

		It("allows sells that close long positions to reduce net exposure", func() {
			// Long 100 SPY @ $200. Selling reduces net exposure.
			acct := buildLongAccount(30_000, spy, 200, 100)
			batch := portfolio.NewBatch(ts, acct)
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       spy,
				Side:        broker.Sell,
				Amount:      5_000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			mw := risk.NetExposureLimit(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(HaveLen(1))
		})

		It("handles zero equity gracefully", func() {
			acct := portfolio.New(portfolio.WithCash(0, time.Time{}))
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.NetExposureLimit(1.0)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
		})
	})
})
