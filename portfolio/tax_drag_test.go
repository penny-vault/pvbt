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
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("TaxDrag", func() {
	var (
		spy asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	})

	It("computes tax drag from short-term gains (high turnover, all STCG)", func() {
		// Portfolio starts at 50_000.
		// Buy 100 shares at $100 (cash: 50000 - 10000 = 40000, equity = 50000).
		// Sell 50 at $120 (STCG = 50*(120-100) = 1000; cash: 40000+6000 = 46000)
		// Sell 50 at $130 (STCG = 50*(130-100) = 1500; cash: 46000+6500 = 52500)
		// Final equity = 52500 (all cash, no holdings); preTaxReturn = 52500 - 50000 = 2500
		// estimatedTax (STCG only, no dividends) = 0.25 * 2500 = 625
		// TaxDrag = 625 / 2500 = 0.25
		a := portfolio.New(portfolio.WithCash(50_000, time.Time{}))

		a.Record(portfolio.Transaction{
			Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  100.0,
			Amount: -10_000.0,
		})

		df1 := buildDF(
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
		)
		a.UpdatePrices(df1)

		// Sell 50 at $120 after < 1 year => STCG = 1000
		a.Record(portfolio.Transaction{
			Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.SellTransaction,
			Qty:    50,
			Price:  120.0,
			Amount: 6_000.0,
		})

		// Sell remaining 50 at $130 after < 1 year => STCG = 1500
		a.Record(portfolio.Transaction{
			Date:   time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.SellTransaction,
			Qty:    50,
			Price:  130.0,
			Amount: 6_500.0,
		})

		df2 := buildDF(
			time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy}, []float64{130.0}, []float64{130.0},
		)
		a.UpdatePrices(df2)

		// STCG = 1000 + 1500 = 2500; preTaxReturn = 52500 - 50000 = 2500
		// estimatedTax = 0.25 * 2500 = 625
		// TaxDrag = 625 / 2500 = 0.25
		tm, err := a.TaxMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(tm.TaxDrag).To(BeNumerically("~", 625.0/2500.0, 1e-9))
	})

	It("computes tax drag from long-term gains (low turnover, all LTCG)", func() {
		// Buy 100 at $100; sell 50 at $130 after > 1 year (LTCG = 1500)
		// PreTaxReturn = 56500 - 50000 = 6500
		// estimatedTax = 0.15 * 1500 = 225
		// TaxDrag = 225 / 6500
		a := portfolio.New(portfolio.WithCash(50_000, time.Time{}))

		a.Record(portfolio.Transaction{
			Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  100.0,
			Amount: -10_000.0,
		})

		df1 := buildDF(
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
		)
		a.UpdatePrices(df1)

		// Sell 50 at $130 after > 1 year => LTCG = 50 * (130 - 100) = 1500
		a.Record(portfolio.Transaction{
			Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.SellTransaction,
			Qty:    50,
			Price:  130.0,
			Amount: 6_500.0,
		})

		// Remaining 50 shares still held at $130 => unrealized value = 6500
		// Total equity = cash + holdings = (50000 - 10000 + 6500) + (50 * 130) = 46500 + 6500 = 53000
		// But wait: initial cash = 50000, bought 10000 worth, so cash left = 40000, sold for 6500, cash = 46500
		// Holdings: 50 * 130 = 6500. Total equity = 53000.
		// PreTaxReturn = 53000 - 50000 = 3000
		// LTCG = 1500; estimatedTax = 0.15 * 1500 = 225
		// TaxDrag = 225 / 3000
		df2 := buildDF(
			time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy}, []float64{130.0}, []float64{130.0},
		)
		a.UpdatePrices(df2)

		tm, err := a.TaxMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(tm.LTCG).To(Equal(1_500.0))

		preTaxReturn := 53_000.0 - 50_000.0
		estimatedTax := 0.15 * 1_500.0
		Expect(tm.TaxDrag).To(BeNumerically("~", estimatedTax/preTaxReturn, 1e-9))
	})

	It("excludes dividends from tax drag (dividends do not contribute to TaxDrag)", func() {
		// Portfolio with dividends only — no sells, no realized gains
		// TaxDrag should be 0 regardless of dividend income
		a := portfolio.New(portfolio.WithCash(50_000, time.Time{}))

		a.Record(portfolio.Transaction{
			Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  100.0,
			Amount: -10_000.0,
		})

		df1 := buildDF(
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
		)
		a.UpdatePrices(df1)

		// Qualified dividend — should be excluded from TaxDrag
		a.Record(portfolio.Transaction{
			Date:      time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC),
			Asset:     spy,
			Type:      asset.DividendTransaction,
			Amount:    500.0,
			Qualified: true,
		})

		// Non-qualified dividend — also excluded from TaxDrag
		a.Record(portfolio.Transaction{
			Date:      time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC),
			Asset:     spy,
			Type:      asset.DividendTransaction,
			Amount:    300.0,
			Qualified: false,
		})

		// Price rises — portfolio has gain from unrealized appreciation + dividends
		df2 := buildDF(
			time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy}, []float64{110.0}, []float64{110.0},
		)
		a.UpdatePrices(df2)

		tm, err := a.TaxMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(tm.TaxDrag).To(Equal(0.0))
	})

	It("returns zero TaxDrag when there is no pre-tax return", func() {
		// Portfolio with a loss: TaxDrag = 0 (preTaxReturn <= 0)
		a := portfolio.New(portfolio.WithCash(50_000, time.Time{}))

		a.Record(portfolio.Transaction{
			Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  100.0,
			Amount: -10_000.0,
		})

		df1 := buildDF(
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
		)
		a.UpdatePrices(df1)

		// Sell at same price => no gain, no loss; equity unchanged
		a.Record(portfolio.Transaction{
			Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.SellTransaction,
			Qty:    100,
			Price:  100.0,
			Amount: 10_000.0,
		})

		df2 := buildDF(
			time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
		)
		a.UpdatePrices(df2)

		tm, err := a.TaxMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(tm.TaxDrag).To(Equal(0.0))
	})

	It("returns zero TaxDrag when there are only realized losses", func() {
		// Realized losses generate no tax; TaxDrag should be 0 regardless
		a := portfolio.New(portfolio.WithCash(50_000, time.Time{}))

		a.Record(portfolio.Transaction{
			Date:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  100.0,
			Amount: -10_000.0,
		})

		df1 := buildDF(
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
		)
		a.UpdatePrices(df1)

		// Sell at a loss => STCG is negative (a loss), no tax owed
		a.Record(portfolio.Transaction{
			Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   asset.SellTransaction,
			Qty:    100,
			Price:  80.0,
			Amount: 8_000.0,
		})

		df2 := buildDF(
			time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy}, []float64{80.0}, []float64{80.0},
		)
		a.UpdatePrices(df2)

		tm, err := a.TaxMetrics()
		Expect(err).NotTo(HaveOccurred())
		// preTaxReturn = 48000 - 50000 = -2000 (negative), so TaxDrag = 0
		Expect(tm.TaxDrag).To(Equal(0.0))
	})

	Describe("window-aware tax metrics", func() {
		// Build an account that realizes gains in two distinct calendar
		// years so a sub-window can isolate one. Buy 100 SPY @ $100 in
		// 2023, sell 50 @ $130 in mid-2023 (STCG = 1500), and sell the
		// remaining 50 @ $150 in mid-2024 after > 1 year (LTCG = 2500).
		var (
			acct        *portfolio.Account
			start2023   = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
			endOf2023   = time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)
			midYear2024 = time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		)

		BeforeEach(func() {
			acct = portfolio.New(portfolio.WithCash(50_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   start2023,
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})
			acct.UpdatePrices(buildDF(
				start2023,
				[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
			))

			// In-2023 sell: 50 shares @ $130, held < 1 year => STCG = 1500.
			acct.Record(portfolio.Transaction{
				Date:   time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    50,
				Price:  130.0,
				Amount: 6_500.0,
			})
			acct.UpdatePrices(buildDF(
				time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
				[]asset.Asset{spy}, []float64{130.0}, []float64{130.0},
			))

			// Stamp the equity curve at end of 2023 so a 2023-only
			// window contains both the buy and the in-2023 sell.
			acct.UpdatePrices(buildDF(
				endOf2023,
				[]asset.Asset{spy}, []float64{140.0}, []float64{140.0},
			))

			// In-2024 sell: remaining 50 shares @ $150, held > 1 year
			// => LTCG = 50 * (150 - 100) = 2500.
			acct.Record(portfolio.Transaction{
				Date:   midYear2024,
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    50,
				Price:  150.0,
				Amount: 7_500.0,
			})
			acct.UpdatePrices(buildDF(
				midYear2024,
				[]asset.Asset{spy}, []float64{150.0}, []float64{150.0},
			))
		})

		It("STCG honors AbsoluteWindow and excludes out-of-window sells", func() {
			fullSTCG, err := acct.PerformanceMetric(portfolio.STCGMetric).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(fullSTCG).To(BeNumerically("~", 1_500.0, 1e-9))

			win2023, err := acct.PerformanceMetric(portfolio.STCGMetric).
				AbsoluteWindow(start2023, endOf2023).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(win2023).To(BeNumerically("~", 1_500.0, 1e-9))

			// 2024 window has no STCG sell.
			win2024, err := acct.PerformanceMetric(portfolio.STCGMetric).
				AbsoluteWindow(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), midYear2024).
				Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(win2024).To(Equal(0.0))
		})

		It("LTCG honors AbsoluteWindow and uses pre-window buys for cost basis", func() {
			fullLTCG, err := acct.PerformanceMetric(portfolio.LTCGMetric).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(fullLTCG).To(BeNumerically("~", 2_500.0, 1e-9))

			// 2024 window: only the LTCG sell falls inside, but the
			// 2023 buy must still seed the FIFO lot so cost basis is $100.
			win2024, err := acct.PerformanceMetric(portfolio.LTCGMetric).
				AbsoluteWindow(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), midYear2024).
				Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(win2024).To(BeNumerically("~", 2_500.0, 1e-9))

			// 2023 window has no LTCG sell.
			win2023, err := acct.PerformanceMetric(portfolio.LTCGMetric).
				AbsoluteWindow(start2023, endOf2023).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(win2023).To(Equal(0.0))
		})

		It("TaxDrag returns a different value for a sub-window than for full history", func() {
			fullDrag, err := acct.PerformanceMetric(portfolio.TaxDragMetric).Value()
			Expect(err).NotTo(HaveOccurred())

			// 2023-only window:
			//   equity start = 50000 (initial cash)
			//   equity end (Dec 31 @ $140) = cash 46500 + 50*140 = 53500
			//   preTaxReturn = 3500
			//   STCG = 1500 (sell in 2023), LTCG = 0
			//   estimatedTax = 0.25*1500 = 375
			//   TaxDrag = 375 / 3500
			win2023Drag, err := acct.PerformanceMetric(portfolio.TaxDragMetric).
				AbsoluteWindow(start2023, endOf2023).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(win2023Drag).To(BeNumerically("~", 375.0/3500.0, 1e-9))
			Expect(win2023Drag).NotTo(BeNumerically("~", fullDrag, 1e-9))
		})

		It("QualifiedDividends honors AbsoluteWindow", func() {
			// Build a fresh account that records dividends after their
			// underlying lots have aged > 60 days but before the lots
			// are sold, so isDividendQualified resolves to true.
			divAcct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
			divAcct.Record(portfolio.Transaction{
				Date:   start2023,
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})
			divAcct.UpdatePrices(buildDF(
				start2023,
				[]asset.Asset{spy}, []float64{100.0}, []float64{100.0},
			))

			divAcct.Record(portfolio.Transaction{
				Date:   time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.DividendTransaction,
				Amount: 400.0,
			})
			divAcct.UpdatePrices(buildDF(
				endOf2023,
				[]asset.Asset{spy}, []float64{120.0}, []float64{120.0},
			))

			divAcct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
				Asset:  spy,
				Type:   asset.DividendTransaction,
				Amount: 600.0,
			})
			divAcct.UpdatePrices(buildDF(
				midYear2024,
				[]asset.Asset{spy}, []float64{130.0}, []float64{130.0},
			))

			win2023, err := divAcct.PerformanceMetric(portfolio.QualifiedDividendsMetric).
				AbsoluteWindow(start2023, endOf2023).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(win2023).To(BeNumerically("~", 400.0, 1e-9))

			win2024, err := divAcct.PerformanceMetric(portfolio.QualifiedDividendsMetric).
				AbsoluteWindow(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), midYear2024).
				Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(win2024).To(BeNumerically("~", 600.0, 1e-9))
		})
	})
})
