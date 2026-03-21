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

var _ = Describe("Wash sale detection", func() {
	var (
		spy asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
	})

	Describe("buy after loss sale within 30 days", func() {
		It("adjusts cost basis of the new lot by the disallowed loss", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy at $100
			buyDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, buyDate, 100.0, 10)

			// Sell at $80 (loss of $20/share)
			sellDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
			sellLot(acct, spy, sellDate, 80.0, 10)

			// Rebuy within 30 days at $85
			rebuyDate := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, rebuyDate, 85.0, 10)

			// New lot basis should be $85 + $20 = $105
			lots := acct.TaxLots()[spy]
			Expect(lots).To(HaveLen(1))
			Expect(lots[0].Price).To(BeNumerically("~", 105.0, 0.001))
		})
	})

	Describe("buy after loss sale beyond 30 days", func() {
		It("does not adjust cost basis when rebuy is after 31 days", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy at $100
			buyDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, buyDate, 100.0, 10)

			// Sell at $80 (loss of $20/share)
			sellDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
			sellLot(acct, spy, sellDate, 80.0, 10)

			// Rebuy after 31 days at $85
			rebuyDate := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, rebuyDate, 85.0, 10)

			// No wash sale -- basis stays at $85
			lots := acct.TaxLots()[spy]
			Expect(lots).To(HaveLen(1))
			Expect(lots[0].Price).To(BeNumerically("~", 85.0, 0.001))
		})
	})

	Describe("loss sale after buy within 30 days", func() {
		It("adjusts the recent buy lot basis when selling at a loss", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy original lot at $100
			origBuyDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, origBuyDate, 100.0, 10)

			// Buy replacement lot at $85 (this is the "recent buy")
			recentBuyDate := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, recentBuyDate, 85.0, 10)

			// Sell original lot at $80 (loss of $20/share), within 30
			// days of the recent buy. Use FIFO to consume the original lot.
			sellDate := time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)
			sellLot(acct, spy, sellDate, 80.0, 10)

			// The recent buy lot should have its basis adjusted:
			// $85 + $20 = $105
			lots := acct.TaxLots()[spy]
			Expect(lots).To(HaveLen(1))
			Expect(lots[0].Price).To(BeNumerically("~", 105.0, 0.001))
		})
	})

	Describe("no wash sale on gain", func() {
		It("does not adjust basis when selling at a gain", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy at $100
			buyDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, buyDate, 100.0, 10)

			// Sell at $120 (gain of $20/share)
			sellDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
			sellLot(acct, spy, sellDate, 120.0, 10)

			// Rebuy within 30 days at $110
			rebuyDate := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, rebuyDate, 110.0, 10)

			// No wash sale -- basis stays at $110
			lots := acct.TaxLots()[spy]
			Expect(lots).To(HaveLen(1))
			Expect(lots[0].Price).To(BeNumerically("~", 110.0, 0.001))
		})
	})

	Describe("partial wash sale", func() {
		It("adjusts only the matching quantity of shares", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy at $100
			buyDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, buyDate, 100.0, 10)

			// Sell 10 shares at $80 (loss of $20/share on 10 shares)
			sellDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
			sellLot(acct, spy, sellDate, 80.0, 10)

			// Buy only 5 shares within 30 days at $85
			rebuyDate := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, rebuyDate, 85.0, 5)

			// Only 5 shares' worth of loss is disallowed.
			// New lot basis = $85 + $20 = $105 for 5 shares
			lots := acct.TaxLots()[spy]
			Expect(lots).To(HaveLen(1))
			Expect(lots[0].Price).To(BeNumerically("~", 105.0, 0.001))
			Expect(lots[0].Qty).To(Equal(5.0))
		})
	})

	Describe("partial wash sale (reverse direction): sell fewer shares than buy lot size", func() {
		It("adjusts only the matched shares' basis, leaving remaining shares unchanged", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy original lot at $100 first; FIFO will consume this lot first.
			origBuyDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, origBuyDate, 100.0, 10)

			// Buy 10 replacement shares at $85 within 30 days of the upcoming sell.
			recentBuyDate := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, recentBuyDate, 85.0, 10)

			// Sell only 5 shares at $80 (loss of $20/share vs the $100 original basis).
			// FIFO consumes 5 shares from the Jan-2 lot.
			// matchQty = 5, which is less than the recent buy lot size of 10.
			// DisallowedLoss = 5 * $20 = $100 total.
			// Only 5 of the 10 replacement shares should have their basis bumped.
			sellDate := time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)
			sellLot(acct, spy, sellDate, 80.0, 5)

			// After the sell the account holds:
			//   - 5 shares of the original lot that remain unsold: basis $100
			//   - 5 shares of the recent buy lot that were matched: basis $85 + $20 = $105
			//   - 5 shares of the recent buy lot that were NOT matched: basis $85
			lots := acct.TaxLots()[spy]
			Expect(lots).To(HaveLen(3))

			// Collect prices across all lots; all have qty 5.
			prices := make([]float64, 0, 3)
			for _, lot := range lots {
				Expect(lot.Qty).To(BeNumerically("~", 5.0, 0.001))
				prices = append(prices, lot.Price)
			}

			Expect(prices).To(ContainElements(
				BeNumerically("~", 105.0, 0.001), // adjusted matched portion
				BeNumerically("~", 85.0, 0.001),  // unadjusted remainder of recent buy
				BeNumerically("~", 100.0, 0.001), // remaining original lot
			))

			// Verify the wash sale record captures the correct disallowed amount.
			records := acct.WashSaleRecords()
			Expect(records).To(HaveLen(1))
			Expect(records[0].DisallowedLoss).To(BeNumerically("~", 100.0, 0.001))
		})
	})

	Describe("WashSaleRecords accessible", func() {
		It("stores and returns wash sale records", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy at $100
			buyDate := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, buyDate, 100.0, 10)

			// Sell at $80 (loss of $20/share)
			sellDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
			sellLot(acct, spy, sellDate, 80.0, 10)

			// Rebuy within 30 days at $85
			rebuyDate := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, rebuyDate, 85.0, 10)

			records := acct.WashSaleRecords()
			Expect(records).To(HaveLen(1))
			Expect(records[0].Asset).To(Equal(spy))
			Expect(records[0].SellDate).To(Equal(sellDate))
			Expect(records[0].RebuyDate).To(Equal(rebuyDate))
			Expect(records[0].DisallowedLoss).To(BeNumerically("~", 200.0, 0.001))
		})
	})
})
