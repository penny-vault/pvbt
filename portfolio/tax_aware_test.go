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

// Compile-time assertion that *Account satisfies TaxAware.
var _ portfolio.TaxAware = (*portfolio.Account)(nil)

var _ = Describe("TaxAware interface", func() {
	var (
		spy asset.Asset
		qqq asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		qqq = asset.Asset{CompositeFigi: "QQQ001", Ticker: "QQQ"}
	})

	Describe("WashSaleWindow", func() {
		It("returns only wash sale records for the requested asset", func() {
			acct := portfolio.New(portfolio.WithCash(200_000, time.Time{}))

			// Create a wash sale for SPY.
			spyBuy := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			spySell := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
			spyRebuy := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, spyBuy, 100.0, 10)
			sellLot(acct, spy, spySell, 80.0, 10)
			buyLot(acct, spy, spyRebuy, 85.0, 10)

			// Create a wash sale for QQQ.
			qqqBuy := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
			qqqSell := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
			qqqRebuy := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
			buyLot(acct, qqq, qqqBuy, 200.0, 5)
			sellLot(acct, qqq, qqqSell, 160.0, 5)
			buyLot(acct, qqq, qqqRebuy, 170.0, 5)

			// WashSaleWindow for SPY should return only SPY records.
			spyRecords := acct.WashSaleWindow(spy)
			Expect(spyRecords).To(HaveLen(1))
			Expect(spyRecords[0].Asset).To(Equal(spy))
			Expect(spyRecords[0].SellDate).To(Equal(spySell))

			// WashSaleWindow for QQQ should return only QQQ records.
			qqqRecords := acct.WashSaleWindow(qqq)
			Expect(qqqRecords).To(HaveLen(1))
			Expect(qqqRecords[0].Asset).To(Equal(qqq))
			Expect(qqqRecords[0].SellDate).To(Equal(qqqSell))
		})

		It("returns nil when no wash sales exist for the asset", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			buyLot(acct, spy, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 100.0, 10)

			records := acct.WashSaleWindow(qqq)
			Expect(records).To(BeNil())
		})

		It("returns a copy; mutations do not affect internal state", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			buyLot(acct, spy, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 100.0, 10)
			sellLot(acct, spy, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), 80.0, 10)
			buyLot(acct, spy, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), 85.0, 10)

			records := acct.WashSaleWindow(spy)
			Expect(records).To(HaveLen(1))
			// Mutate the returned slice.
			records[0].DisallowedLoss = 999_999.0

			// Internal state is unaffected.
			fresh := acct.WashSaleWindow(spy)
			Expect(fresh[0].DisallowedLoss).NotTo(BeNumerically("~", 999_999.0, 1))
		})
	})

	Describe("UnrealizedLots", func() {
		It("returns the open lots for the requested asset", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			buyDate1 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			buyDate2 := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, buyDate1, 100.0, 10)
			buyLot(acct, spy, buyDate2, 110.0, 5)

			lots := acct.UnrealizedLots(spy)
			Expect(lots).To(HaveLen(2))

			// Verify prices are as expected.
			prices := make([]float64, 0, 2)
			for _, lot := range lots {
				prices = append(prices, lot.Price)
			}
			Expect(prices).To(ContainElements(
				BeNumerically("~", 100.0, 0.001),
				BeNumerically("~", 110.0, 0.001),
			))
		})

		It("returns nil when no lots exist for the asset", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			lots := acct.UnrealizedLots(spy)
			Expect(lots).To(BeNil())
		})

		It("returns a copy; mutations do not affect internal state", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			buyLot(acct, spy, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 100.0, 10)

			lots := acct.UnrealizedLots(spy)
			Expect(lots).To(HaveLen(1))
			origPrice := lots[0].Price

			// Mutate the returned slice.
			lots[0].Price = 999_999.0

			// Internal state (via a second call) is unaffected.
			fresh := acct.UnrealizedLots(spy)
			Expect(fresh[0].Price).To(BeNumerically("~", origPrice, 0.001))
		})
	})

	Describe("RealizedGainsYTD", func() {
		It("returns zero when no sells have occurred", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			buyLot(acct, spy, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 100.0, 10)

			ltcg, stcg := acct.RealizedGainsYTD()
			Expect(ltcg).To(BeNumerically("~", 0.0, 0.001))
			Expect(stcg).To(BeNumerically("~", 0.0, 0.001))
		})

		It("classifies short-term gains correctly (held < 1 year)", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			buyDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			sellDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) // ~164 days later
			buyLot(acct, spy, buyDate, 100.0, 10)
			sellLot(acct, spy, sellDate, 150.0, 10) // $50/share gain

			ltcg, stcg := acct.RealizedGainsYTD()
			Expect(ltcg).To(BeNumerically("~", 0.0, 0.001))
			Expect(stcg).To(BeNumerically("~", 500.0, 0.001)) // 10 * $50
		})

		It("classifies long-term gains correctly (held > 1 year)", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			buyDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			sellDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // ~2 years later
			buyLot(acct, spy, buyDate, 100.0, 10)
			sellLot(acct, spy, sellDate, 180.0, 10) // $80/share gain

			ltcg, stcg := acct.RealizedGainsYTD()
			Expect(ltcg).To(BeNumerically("~", 800.0, 0.001)) // 10 * $80
			Expect(stcg).To(BeNumerically("~", 0.0, 0.001))
		})

		It("classifies short-term losses correctly", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			buyDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			sellDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, buyDate, 100.0, 10)
			sellLot(acct, spy, sellDate, 80.0, 10) // $20/share loss

			ltcg, stcg := acct.RealizedGainsYTD()
			Expect(ltcg).To(BeNumerically("~", 0.0, 0.001))
			Expect(stcg).To(BeNumerically("~", -200.0, 0.001)) // 10 * -$20
		})

		It("accumulates gains across multiple sells", func() {
			acct := portfolio.New(portfolio.WithCash(200_000, time.Time{}))

			// Short-term gain: buy Jan 2, sell Jun 15 (same year).
			buyLot(acct, spy, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 100.0, 10)
			sellLot(acct, spy, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), 120.0, 10) // +$200 STCG

			// Long-term gain: buy Jan 2024, sell Jan 2026.
			buyLot(acct, qqq, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 200.0, 5)
			sellLot(acct, qqq, time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC), 300.0, 5) // +$500 LTCG

			ltcg, stcg := acct.RealizedGainsYTD()
			Expect(ltcg).To(BeNumerically("~", 500.0, 0.001))
			Expect(stcg).To(BeNumerically("~", 200.0, 0.001))
		})
	})
})
