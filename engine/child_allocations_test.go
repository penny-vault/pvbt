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

package engine_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// buildPricedAccount creates a portfolio.Account with the given cash balance
// and positions (asset -> dollar value), then sets prices via UpdatePrices so
// that Value() and PositionValue() return the expected values.
//
// The initial deposit is set to cashBalance + sum(positionValues) so that after
// recording all buy transactions the remaining cash equals cashBalance. Each
// position is recorded at price $1.00/share with qty = posValue, so
// PositionValue(asset) == posValue after UpdatePrices.
func buildPricedAccount(date time.Time, cashBalance float64, positions map[asset.Asset]float64) *portfolio.Account {
	totalPositionCost := 0.0
	for _, posValue := range positions {
		totalPositionCost += posValue
	}

	// Fund the account with enough cash to cover all buys and leave cashBalance remaining.
	initialDeposit := cashBalance + totalPositionCost
	acct := portfolio.New(portfolio.WithCash(initialDeposit, date))

	priceAssets := make([]asset.Asset, 0, len(positions))
	priceValues := make([]float64, 0, len(positions))

	for posAsset, posValue := range positions {
		priceAssets = append(priceAssets, posAsset)
		// Use price=1.0 and qty=posValue so PositionValue = qty*price = posValue.
		priceValues = append(priceValues, 1.0)

		// Record a buy: qty=posValue shares at $1.00 each.
		acct.Record(portfolio.Transaction{
			Date:   date,
			Asset:  posAsset,
			Type:   portfolio.BuyTransaction,
			Qty:    posValue,
			Price:  1.0,
			Amount: -posValue, // cash decreases by posValue
		})
	}

	if len(priceAssets) > 0 {
		// Build a DataFrame with a single row so UpdatePrices marks the account.
		slab := make([]float64, len(priceAssets))
		for slabIdx := range priceAssets {
			slab[slabIdx] = priceValues[slabIdx]
		}

		columns := make([][]float64, len(priceAssets))
		for colIdx := range priceAssets {
			columns[colIdx] = []float64{slab[colIdx]}
		}

		df, err := data.NewDataFrame(
			[]time.Time{date},
			priceAssets,
			[]data.Metric{data.MetricClose},
			data.Daily,
			columns,
		)
		if err != nil {
			panic("buildPricedAccount: " + err.Error())
		}

		acct.UpdatePrices(df)
	}

	return acct
}

var _ = Describe("ChildAllocations", func() {
	var (
		testDate time.Time
		spyAsset asset.Asset
		shyAsset asset.Asset
		tltAsset asset.Asset
	)

	BeforeEach(func() {
		testDate = time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		spyAsset = asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BDTBL9"}
		shyAsset = asset.Asset{Ticker: "SHY", CompositeFigi: "BBG000HLXCC4"}
		tltAsset = asset.Asset{Ticker: "TLT", CompositeFigi: "BBG000BMBF18"}
	})

	Describe("no children", func() {
		It("returns empty Allocation and no error", func() {
			eng := engine.New(&simpleChild{})
			engine.SetEngineDateForTest(eng, testDate)

			alloc, err := eng.ChildAllocations()

			Expect(err).ToNot(HaveOccurred())
			Expect(alloc.Members).To(BeEmpty())
		})
	})

	Describe("static weights with multiple holdings", func() {
		// Child A: weight=0.10, holds SPY ($60) + SHY ($40) => total $100
		//   SPY fraction: 60/100 = 0.60 => 0.10 * 0.60 = 0.06
		//   SHY fraction: 40/100 = 0.40 => 0.10 * 0.40 = 0.04
		// Child B: weight=0.40, holds TLT ($100) => total $100
		//   TLT fraction: 100/100 = 1.00 => 0.40 * 1.00 = 0.40
		It("expands holdings into flat allocation scaled by child weight", func() {
			accountA := buildPricedAccount(testDate, 0, map[asset.Asset]float64{
				spyAsset: 60.0,
				shyAsset: 40.0,
			})
			accountB := buildPricedAccount(testDate, 0, map[asset.Asset]float64{
				tltAsset: 100.0,
			})

			eng := engine.New(&simpleChild{})
			engine.SetEngineDateForTest(eng, testDate)
			engine.SetChildrenForTest(eng, []*engine.ChildEntryForTest{
				engine.NewChildEntryForTest("childA", 0.10, accountA),
				engine.NewChildEntryForTest("childB", 0.40, accountB),
			})

			alloc, err := eng.ChildAllocations()

			Expect(err).ToNot(HaveOccurred())
			Expect(alloc.Date).To(Equal(testDate))
			Expect(alloc.Members[spyAsset]).To(BeNumerically("~", 0.06, 1e-9))
			Expect(alloc.Members[shyAsset]).To(BeNumerically("~", 0.04, 1e-9))
			Expect(alloc.Members[tltAsset]).To(BeNumerically("~", 0.40, 1e-9))
		})
	})

	Describe("cash inclusion", func() {
		// Child: weight=0.50, $60 SPY + $40 cash => total $100
		//   SPY fraction: 60/100 = 0.60 => 0.50 * 0.60 = 0.30
		//   $CASH fraction: 40/100 = 0.40 => 0.50 * 0.40 = 0.20
		It("includes $CASH for child's uninvested cash", func() {
			childAccount := buildPricedAccount(testDate, 40.0, map[asset.Asset]float64{
				spyAsset: 60.0,
			})

			eng := engine.New(&simpleChild{})
			engine.SetEngineDateForTest(eng, testDate)
			engine.SetChildrenForTest(eng, []*engine.ChildEntryForTest{
				engine.NewChildEntryForTest("mixed", 0.50, childAccount),
			})

			alloc, err := eng.ChildAllocations()

			Expect(err).ToNot(HaveOccurred())
			Expect(alloc.Members[spyAsset]).To(BeNumerically("~", 0.30, 1e-9))
			Expect(alloc.Members[asset.CashAsset]).To(BeNumerically("~", 0.20, 1e-9))
		})
	})

	Describe("all cash child", func() {
		// Child: weight=0.30, no positions -- Value()==0 before UpdatePrices
		// maps the entire child weight to $CASH.
		It("maps weight to $CASH when child portfolio has zero value", func() {
			// No positions and no UpdatePrices => Value() == cash only (0)
			childAccount := portfolio.New(portfolio.WithCash(0, testDate))

			eng := engine.New(&simpleChild{})
			engine.SetEngineDateForTest(eng, testDate)
			engine.SetChildrenForTest(eng, []*engine.ChildEntryForTest{
				engine.NewChildEntryForTest("allCash", 0.30, childAccount),
			})

			alloc, err := eng.ChildAllocations()

			Expect(err).ToNot(HaveOccurred())
			Expect(alloc.Members[asset.CashAsset]).To(BeNumerically("~", 0.30, 1e-9))
		})
	})

	Describe("override weights", func() {
		It("uses overridden weight instead of declared weight", func() {
			accountA := buildPricedAccount(testDate, 0, map[asset.Asset]float64{
				spyAsset: 100.0,
			})
			accountB := buildPricedAccount(testDate, 0, map[asset.Asset]float64{
				tltAsset: 100.0,
			})

			eng := engine.New(&simpleChild{})
			engine.SetEngineDateForTest(eng, testDate)
			engine.SetChildrenForTest(eng, []*engine.ChildEntryForTest{
				engine.NewChildEntryForTest("alpha", 0.30, accountA),
				engine.NewChildEntryForTest("beta", 0.30, accountB),
			})

			overrides := map[string]float64{
				"alpha": 0.20,
				"beta":  0.50,
			}
			alloc, err := eng.ChildAllocations(overrides)

			Expect(err).ToNot(HaveOccurred())
			Expect(alloc.Members[spyAsset]).To(BeNumerically("~", 0.20, 1e-9))
			Expect(alloc.Members[tltAsset]).To(BeNumerically("~", 0.50, 1e-9))
		})
	})

	Describe("override validation", func() {
		It("returns an error when overridden weights sum to more than 1.0", func() {
			childAccount := buildPricedAccount(testDate, 0, map[asset.Asset]float64{
				spyAsset: 100.0,
			})

			eng := engine.New(&simpleChild{})
			engine.SetEngineDateForTest(eng, testDate)
			engine.SetChildrenForTest(eng, []*engine.ChildEntryForTest{
				engine.NewChildEntryForTest("only", 0.50, childAccount),
			})

			overrides := map[string]float64{"only": 1.5}
			_, err := eng.ChildAllocations(overrides)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must be at most 1.0"))
		})
	})

	Describe("overlapping assets across children", func() {
		// Both children hold SPY:
		//   Child A: weight=0.40, 100% SPY => SPY contribution 0.40
		//   Child B: weight=0.30, 100% SPY => SPY contribution 0.30
		//   Total SPY weight: 0.70
		It("sums contributions from multiple children holding the same asset", func() {
			accountA := buildPricedAccount(testDate, 0, map[asset.Asset]float64{
				spyAsset: 100.0,
			})
			accountB := buildPricedAccount(testDate, 0, map[asset.Asset]float64{
				spyAsset: 100.0,
			})

			eng := engine.New(&simpleChild{})
			engine.SetEngineDateForTest(eng, testDate)
			engine.SetChildrenForTest(eng, []*engine.ChildEntryForTest{
				engine.NewChildEntryForTest("first", 0.40, accountA),
				engine.NewChildEntryForTest("second", 0.30, accountB),
			})

			alloc, err := eng.ChildAllocations()

			Expect(err).ToNot(HaveOccurred())
			Expect(alloc.Members[spyAsset]).To(BeNumerically("~", 0.70, 1e-9))
		})
	})

	Describe("child with zero value (no UpdatePrices called)", func() {
		// Child has cash=0 and no positions -- Value()=0 -- maps entire weight to $CASH.
		It("maps entire child weight to $CASH when value is zero", func() {
			// Account has zero cash, no positions, no prices set.
			emptyAccount := portfolio.New()

			eng := engine.New(&simpleChild{})
			engine.SetEngineDateForTest(eng, testDate)
			engine.SetChildrenForTest(eng, []*engine.ChildEntryForTest{
				engine.NewChildEntryForTest("empty", 0.25, emptyAccount),
			})

			alloc, err := eng.ChildAllocations()

			Expect(err).ToNot(HaveOccurred())
			Expect(alloc.Members[asset.CashAsset]).To(BeNumerically("~", 0.25, 1e-9))
		})
	})
})

var _ = Describe("ChildPortfolios", func() {
	var testDate time.Time

	BeforeEach(func() {
		testDate = time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
	})

	It("returns an empty map when there are no children", func() {
		eng := engine.New(&simpleChild{})
		engine.SetEngineDateForTest(eng, testDate)

		portfolios := eng.ChildPortfolios()

		Expect(portfolios).To(BeEmpty())
	})

	It("returns a map keyed by child name with portfolio interface values", func() {
		spyAsset := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BDTBL9"}
		tltAsset := asset.Asset{Ticker: "TLT", CompositeFigi: "BBG000BMBF18"}

		accountA := buildPricedAccount(testDate, 50.0, map[asset.Asset]float64{
			spyAsset: 100.0,
		})
		accountB := buildPricedAccount(testDate, 0, map[asset.Asset]float64{
			tltAsset: 200.0,
		})

		eng := engine.New(&simpleChild{})
		engine.SetEngineDateForTest(eng, testDate)
		engine.SetChildrenForTest(eng, []*engine.ChildEntryForTest{
			engine.NewChildEntryForTest("equity", 0.60, accountA),
			engine.NewChildEntryForTest("bonds", 0.40, accountB),
		})

		portfolios := eng.ChildPortfolios()

		Expect(portfolios).To(HaveLen(2))
		Expect(portfolios).To(HaveKey("equity"))
		Expect(portfolios).To(HaveKey("bonds"))

		// Verify the portfolio interface values report correct state.
		Expect(portfolios["equity"].Cash()).To(BeNumerically("~", 50.0, 1e-9))
		Expect(portfolios["bonds"].PositionValue(tltAsset)).To(BeNumerically("~", 200.0, 1e-9))
	})
})
