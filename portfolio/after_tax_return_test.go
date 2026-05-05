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
	"errors"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("AfterTax return metrics", func() {
	var spy asset.Asset

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	})

	// buildSTCGAccount realizes a single STCG sell so the tax-adjusted curve
	// has a known shape. Buy 100 SPY @ $100, mark @ $120, sell all @ $120.
	// Pre-tax equity progression on three trading days:
	//   day0: cash 50000 (50000 - 10000 buy + 10000 stock)
	//   day1: stock revalues to $120 -> cash 40000 + holdings 12000 = 52000
	//   day2: sell at $120 -> cash 52000 + holdings 0 = 52000
	// STCG = 100 * 20 = 2000; tax = 0.25 * 2000 = 500.
	// After-tax curve: [50000, 52000, 51500].
	buildSTCGAccount := func() (*portfolio.Account, []time.Time) {
		dates := daySeq(time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC), 3)

		acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
		acct.Record(portfolio.Transaction{
			Date:   dates[0],
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    100,
			Price:  100.0,
			Amount: -10_000.0,
		})
		acct.UpdatePrices(buildDF(dates[0], []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))

		acct.UpdatePrices(buildDF(dates[1], []asset.Asset{spy}, []float64{120.0}, []float64{120.0}))

		acct.Record(portfolio.Transaction{
			Date:   dates[2],
			Asset:  spy,
			Type:   asset.SellTransaction,
			Qty:    100,
			Price:  120.0,
			Amount: 12_000.0,
		})
		acct.UpdatePrices(buildDF(dates[2], []asset.Asset{spy}, []float64{120.0}, []float64{120.0}))

		return acct, dates
	}

	Describe("AfterTaxTWRR", func() {
		It("subtracts realized STCG tax from the equity curve before compounding", func() {
			acct, _ := buildSTCGAccount()

			twrr, err := acct.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			afterTax, err := acct.PerformanceMetric(portfolio.AfterTaxTWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			// Pre-tax: product(52000/50000, 52000/52000) - 1 = 0.04
			// After-tax curve [50000, 52000, 51500]:
			//   product(52000/50000, 51500/52000) - 1 = 51500/50000 - 1 = 0.03
			Expect(twrr).To(BeNumerically("~", 0.04, 1e-9))
			Expect(afterTax).To(BeNumerically("~", 0.03, 1e-9))
		})

		It("equals TWRR when there are no realized gains in the window", func() {
			// Buy and hold; never sell. Tax stream is empty so the after-tax
			// curve matches the pre-tax curve exactly.
			dates := daySeq(time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC), 3)

			acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
			acct.Record(portfolio.Transaction{
				Date:   dates[0],
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})
			acct.UpdatePrices(buildDF(dates[0], []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))
			acct.UpdatePrices(buildDF(dates[1], []asset.Asset{spy}, []float64{110.0}, []float64{110.0}))
			acct.UpdatePrices(buildDF(dates[2], []asset.Asset{spy}, []float64{120.0}, []float64{120.0}))

			twrr, err := acct.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			afterTax, err := acct.PerformanceMetric(portfolio.AfterTaxTWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			Expect(afterTax).To(BeNumerically("~", twrr, 1e-12))
		})

		It("is unaffected by realized losses (tax floor is zero per category)", func() {
			// Sell at a loss => STCG is negative; tax adjustment is zero.
			dates := daySeq(time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC), 3)

			acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
			acct.Record(portfolio.Transaction{
				Date:   dates[0],
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})
			acct.UpdatePrices(buildDF(dates[0], []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))
			acct.UpdatePrices(buildDF(dates[1], []asset.Asset{spy}, []float64{80.0}, []float64{80.0}))

			acct.Record(portfolio.Transaction{
				Date:   dates[2],
				Asset:  spy,
				Type:   asset.SellTransaction,
				Qty:    100,
				Price:  80.0,
				Amount: 8_000.0,
			})
			acct.UpdatePrices(buildDF(dates[2], []asset.Asset{spy}, []float64{80.0}, []float64{80.0}))

			twrr, err := acct.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			afterTax, err := acct.PerformanceMetric(portfolio.AfterTaxTWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			Expect(afterTax).To(BeNumerically("~", twrr, 1e-12))
		})

		It("returns ErrInsufficientData when the window has fewer than two equity points", func() {
			acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
			date := time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC)
			acct.UpdatePrices(buildDF(date, []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))

			_, err := acct.PerformanceMetric(portfolio.AfterTaxTWRR).Value()
			Expect(errors.Is(err, portfolio.ErrInsufficientData)).To(BeTrue())
		})
	})

	Describe("AfterTaxCAGR", func() {
		It("annualizes the after-tax return over the window", func() {
			// Buy 100 SPY @ $100 in early 2023, hold for ~2 years, sell at $200
			// after Jan 2024 so the gain is LTCG (15% rate). Hand-compute:
			//   starting equity = 50000 (cash) - 10000 (buy) + 10000 (mark) = 50000
			//   end equity (mark @ $200, sell day): cash 60000 + 0 holdings = 60000
			//   STCG = 0; LTCG = 100 * 100 = 10000; tax = 0.15 * 10000 = 1500
			//   after-tax end = 60000 - 1500 = 58500
			//   ratio = 58500 / 50000 = 1.17
			//   years = exact diff / 365.25 (computed below)
			//   AfterTaxCAGR = 1.17^(1/years) - 1
			start := time.Date(2023, 1, 3, 0, 0, 0, 0, time.UTC)
			midMark := time.Date(2023, 7, 3, 0, 0, 0, 0, time.UTC)
			end := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)

			acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
			acct.Record(portfolio.Transaction{
				Date: start, Asset: spy, Type: asset.BuyTransaction,
				Qty: 100, Price: 100.0, Amount: -10_000.0,
			})
			acct.UpdatePrices(buildDF(start, []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))
			acct.UpdatePrices(buildDF(midMark, []asset.Asset{spy}, []float64{150.0}, []float64{150.0}))

			acct.Record(portfolio.Transaction{
				Date: end, Asset: spy, Type: asset.SellTransaction,
				Qty: 100, Price: 200.0, Amount: 20_000.0,
			})
			acct.UpdatePrices(buildDF(end, []asset.Asset{spy}, []float64{200.0}, []float64{200.0}))

			years := end.Sub(start).Hours() / 24 / 365.25
			expected := math.Pow(58_500.0/50_000.0, 1.0/years) - 1

			cagr, err := acct.PerformanceMetric(portfolio.AfterTaxCAGR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(cagr).To(BeNumerically("~", expected, 1e-9))
		})

		It("returns ErrInsufficientData when the window cannot be annualized", func() {
			acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
			date := time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC)
			acct.UpdatePrices(buildDF(date, []asset.Asset{spy}, []float64{100.0}, []float64{100.0}))

			_, err := acct.PerformanceMetric(portfolio.AfterTaxCAGR).Value()
			Expect(errors.Is(err, portfolio.ErrInsufficientData)).To(BeTrue())
		})
	})
})
