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

// buyLot records a buy transaction for the given asset at price and qty.
func buyLot(acct *portfolio.Account, ast asset.Asset, date time.Time, price, qty float64) {
	acct.Record(portfolio.Transaction{
		Date:   date,
		Asset:  ast,
		Type:   asset.BuyTransaction,
		Qty:    qty,
		Price:  price,
		Amount: -(price * qty),
	})
}

// sellLot records a sell transaction for the given asset at price and qty.
func sellLot(acct *portfolio.Account, ast asset.Asset, date time.Time, price, qty float64) {
	acct.Record(portfolio.Transaction{
		Date:   date,
		Asset:  ast,
		Type:   asset.SellTransaction,
		Qty:    qty,
		Price:  price,
		Amount: price * qty,
	})
}

var _ = Describe("Account lot selection", func() {
	var (
		spy   asset.Asset
		day1  time.Time
		day2  time.Time
		day3  time.Time
		day4  time.Time
		price float64
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		day1 = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		day2 = time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
		day3 = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		day4 = time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)
		price = 350.0
	})

	Describe("FIFO (default)", func() {
		It("consumes the earliest lot first", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			buyLot(acct, spy, day1, 100.0, 10) // lot 1: $100
			buyLot(acct, spy, day2, 300.0, 10) // lot 2: $300
			buyLot(acct, spy, day3, 200.0, 10) // lot 3: $200

			sellLot(acct, spy, day4, price, 10)

			lots := acct.TaxLots()[spy]
			Expect(lots).To(HaveLen(2))
			// The $100 lot should have been consumed first.
			Expect(lots[0].Price).To(Equal(300.0))
			Expect(lots[1].Price).To(Equal(200.0))
		})
	})

	Describe("LIFO", func() {
		It("consumes the most recently acquired lot first", func() {
			acct := portfolio.New(
				portfolio.WithCash(100_000, time.Time{}),
				portfolio.WithDefaultLotSelection(portfolio.LotLIFO),
			)

			buyLot(acct, spy, day1, 100.0, 10) // lot 1: $100
			buyLot(acct, spy, day2, 300.0, 10) // lot 2: $300
			buyLot(acct, spy, day3, 200.0, 10) // lot 3: $200

			sellLot(acct, spy, day4, price, 10)

			lots := acct.TaxLots()[spy]
			Expect(lots).To(HaveLen(2))
			// The $200 lot (most recent) should have been consumed.
			Expect(lots[0].Price).To(Equal(100.0))
			Expect(lots[1].Price).To(Equal(300.0))
		})
	})

	Describe("HighestCost", func() {
		It("consumes the highest cost lot first", func() {
			acct := portfolio.New(
				portfolio.WithCash(100_000, time.Time{}),
				portfolio.WithDefaultLotSelection(portfolio.LotHighestCost),
			)

			buyLot(acct, spy, day1, 100.0, 10) // lot 1: $100
			buyLot(acct, spy, day2, 300.0, 10) // lot 2: $300
			buyLot(acct, spy, day3, 200.0, 10) // lot 3: $200

			sellLot(acct, spy, day4, price, 10)

			lots := acct.TaxLots()[spy]
			Expect(lots).To(HaveLen(2))
			// The $300 lot (highest cost) should have been consumed.
			Expect(lots[0].Price).To(Equal(100.0))
			Expect(lots[1].Price).To(Equal(200.0))
		})
	})

	Describe("per-transaction LotSelection override", func() {
		It("overrides the account default when set on the transaction", func() {
			// Account default is FIFO, but transaction override is LIFO.
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			buyLot(acct, spy, day1, 100.0, 10) // lot 1: $100
			buyLot(acct, spy, day2, 300.0, 10) // lot 2: $300
			buyLot(acct, spy, day3, 200.0, 10) // lot 3: $200

			// Sell with LIFO override.
			acct.Record(portfolio.Transaction{
				Date:         day4,
				Asset:        spy,
				Type:         asset.SellTransaction,
				Qty:          10,
				Price:        price,
				Amount:       price * 10,
				LotSelection: portfolio.LotLIFO,
			})

			lots := acct.TaxLots()[spy]
			Expect(lots).To(HaveLen(2))
			// The $200 lot (most recent) should have been consumed.
			Expect(lots[0].Price).To(Equal(100.0))
			Expect(lots[1].Price).To(Equal(300.0))
		})
	})
})
