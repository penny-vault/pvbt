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

package tax_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tax"
)

// buyLot records a buy transaction for the given asset.
func buyLot(acct *portfolio.Account, ast asset.Asset, date time.Time, price, qty float64) {
	acct.Record(portfolio.Transaction{
		Date:   date,
		Asset:  ast,
		Type:   portfolio.BuyTransaction,
		Qty:    qty,
		Price:  price,
		Amount: -(price * qty),
	})
}

// sellLot records a sell transaction for the given asset.
func sellLot(acct *portfolio.Account, ast asset.Asset, date time.Time, price, qty float64) {
	acct.Record(portfolio.Transaction{
		Date:   date,
		Asset:  ast,
		Type:   portfolio.SellTransaction,
		Qty:    qty,
		Price:  price,
		Amount: price * qty,
	})
}

// ordersWithSide filters a slice of broker orders by side.
func ordersWithSide(orders []broker.Order, side broker.Side) []broker.Order {
	var result []broker.Order

	for _, ord := range orders {
		if ord.Side == side {
			result = append(result, ord)
		}
	}

	return result
}

// buildAccount creates an Account with the given cash, records a buy for the
// asset at the given price and quantity, and loads a price DataFrame so the
// batch can compute current market values.
func buildAccount(cash float64, ast asset.Asset, buyPrice, currentPrice, qty float64, priceDate time.Time) *portfolio.Account {
	acct := portfolio.New(portfolio.WithCash(cash+buyPrice*qty, time.Time{}))

	// Record the buy at the original price.
	buyDate := priceDate.AddDate(0, -1, 0)
	buyLot(acct, ast, buyDate, buyPrice, qty)

	// Load current prices so the portfolio reports the current market value.
	df, err := data.NewDataFrame(
		[]time.Time{priceDate},
		[]asset.Asset{ast},
		[]data.Metric{data.MetricClose},
		data.Daily,
		[][]float64{{currentPrice}},
	)
	Expect(err).NotTo(HaveOccurred())

	acct.UpdatePrices(df)

	return acct
}

var _ = Describe("TaxLossHarvester", func() {
	var (
		spy asset.Asset
		voo asset.Asset
		ctx context.Context
		ts  time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		voo = asset.Asset{CompositeFigi: "VOO001", Ticker: "VOO"}
		ctx = context.Background()
		ts = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	})

	Describe("Process", func() {
		It("harvests loss exceeding threshold", func() {
			// Buy 100 shares at $100, current price $90 => 10% loss, threshold 5%.
			acct := buildAccount(5_000, spy, 100.0, 90.0, 100, ts)
			batch := portfolio.NewBatch(ts, acct)

			harvester := tax.NewTaxLossHarvester(tax.HarvesterConfig{
				LossThreshold: 0.05,
			})
			Expect(harvester.Process(ctx, batch)).To(Succeed())

			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).To(HaveLen(1))
			Expect(sells[0].Asset).To(Equal(spy))
			Expect(sells[0].Qty).To(BeNumerically("~", 100.0, 1e-6))
			Expect(sells[0].LotSelection).To(Equal(int(portfolio.LotHighestCost)))
			Expect(sells[0].Justification).To(ContainSubstring("tax-loss harvest"))
			Expect(sells[0].Justification).To(ContainSubstring("10.0%"))
		})

		It("skips position below threshold", func() {
			// Buy 100 shares at $100, current price $97 => 3% loss, threshold 5%.
			acct := buildAccount(5_000, spy, 100.0, 97.0, 100, ts)
			batch := portfolio.NewBatch(ts, acct)

			harvester := tax.NewTaxLossHarvester(tax.HarvesterConfig{
				LossThreshold: 0.05,
			})
			Expect(harvester.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
		})

		It("gain-offset mode skips when no gains", func() {
			// Position has a loss but GainOffsetOnly is true and no gains exist.
			acct := buildAccount(5_000, spy, 100.0, 90.0, 100, ts)
			batch := portfolio.NewBatch(ts, acct)

			harvester := tax.NewTaxLossHarvester(tax.HarvesterConfig{
				LossThreshold:  0.05,
				GainOffsetOnly: true,
			})
			Expect(harvester.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
		})

		It("gain-offset mode harvests when gains exist", func() {
			// Create an account with a realized gain, then a position at a loss.
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy and sell QQQ at a gain to create realized gains.
			qqq := asset.Asset{CompositeFigi: "QQQ001", Ticker: "QQQ"}
			gainBuyDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
			buyLot(acct, qqq, gainBuyDate, 50.0, 100) // buy 100 @ $50
			gainSellDate := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
			sellLot(acct, qqq, gainSellDate, 70.0, 100) // sell 100 @ $70, $2000 gain

			// Buy SPY and let it decline.
			spyBuyDate := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, spyBuyDate, 100.0, 50)

			// Load current SPY price at $88 (12% loss).
			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[][]float64{{88.0}},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(df)

			batch := portfolio.NewBatch(ts, acct)

			harvester := tax.NewTaxLossHarvester(tax.HarvesterConfig{
				LossThreshold:  0.05,
				GainOffsetOnly: true,
			})
			Expect(harvester.Process(ctx, batch)).To(Succeed())

			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).To(HaveLen(1))
			Expect(sells[0].Asset).To(Equal(spy))
		})

		It("respects wash sale window", func() {
			// Create a wash sale scenario: buy, sell at loss, rebuy within 30 days.
			// Then verify the harvester does NOT try to harvest again (no substitute).
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy SPY at $100.
			buyDate := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, buyDate, 100.0, 50)

			// Sell at $80 (loss).
			sellDate := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			sellLot(acct, spy, sellDate, 80.0, 50)

			// Rebuy within 30 days -- triggers wash sale detection.
			rebuyDate := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, rebuyDate, 85.0, 50)

			// Verify wash sale was recorded.
			washRecords := acct.WashSaleWindow(spy)
			Expect(washRecords).NotTo(BeEmpty(), "expected wash sale records after loss sale + rebuy")

			// Now the position is at a further loss -- current price $75.
			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[][]float64{{75.0}},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(df)

			batch := portfolio.NewBatch(ts, acct)

			harvester := tax.NewTaxLossHarvester(tax.HarvesterConfig{
				LossThreshold: 0.05,
			})
			Expect(harvester.Process(ctx, batch)).To(Succeed())

			// Should NOT harvest because of wash sale risk and no substitute.
			Expect(batch.Orders).To(BeEmpty())
		})

		It("harvests despite wash sale when substitute configured", func() {
			// Same wash sale scenario but with a substitute configured.
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

			// Buy SPY at $100.
			buyDate := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, buyDate, 100.0, 50)

			// Sell at $80 (loss).
			sellDate := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			sellLot(acct, spy, sellDate, 80.0, 50)

			// Rebuy within 30 days -- triggers wash sale.
			rebuyDate := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
			buyLot(acct, spy, rebuyDate, 85.0, 50)

			// Current price $75 -- still at a loss.
			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy, voo},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[][]float64{{75.0}, {80.0}},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(df)

			batch := portfolio.NewBatch(ts, acct)

			harvester := tax.NewTaxLossHarvester(tax.HarvesterConfig{
				LossThreshold: 0.05,
				Substitutes:   map[asset.Asset]asset.Asset{spy: voo},
			})
			Expect(harvester.Process(ctx, batch)).To(Succeed())

			// Should harvest -- sell SPY and buy VOO.
			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).To(HaveLen(1))
			Expect(sells[0].Asset).To(Equal(spy))

			buys := ordersWithSide(batch.Orders, broker.Buy)
			Expect(buys).To(HaveLen(1))
			Expect(buys[0].Asset).To(Equal(voo))
		})

		It("silent when nothing harvestable", func() {
			// All positions at a gain -- nothing to harvest.
			acct := buildAccount(5_000, spy, 100.0, 110.0, 100, ts)
			batch := portfolio.NewBatch(ts, acct)

			originalOrderCount := len(batch.Orders)

			harvester := tax.NewTaxLossHarvester(tax.HarvesterConfig{
				LossThreshold: 0.05,
			})
			Expect(harvester.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(HaveLen(originalOrderCount))
			// No annotations should be added.
			Expect(batch.Annotations).To(BeEmpty())
		})

		It("substitute buy matches sold lots dollar value", func() {
			// Buy 50 shares at $100, current price $88 => position value = $4400.
			// The substitute buy should be for $4400, not the cost basis.
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			spyBuyDate := ts.AddDate(0, -1, 0)
			buyLot(acct, spy, spyBuyDate, 100.0, 50)

			// Load SPY and VOO prices together so UpdatePrices is called once.
			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy, voo},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[][]float64{{88.0}, {95.0}},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(df)

			batch := portfolio.NewBatch(ts, acct)

			harvester := tax.NewTaxLossHarvester(tax.HarvesterConfig{
				LossThreshold: 0.05,
				Substitutes:   map[asset.Asset]asset.Asset{spy: voo},
			})
			Expect(harvester.Process(ctx, batch)).To(Succeed())

			buys := ordersWithSide(batch.Orders, broker.Buy)
			Expect(buys).To(HaveLen(1))
			Expect(buys[0].Asset).To(Equal(voo))

			// Substitute buy should match the sold position's current value.
			// 50 shares * $88 = $4400.
			Expect(buys[0].Amount).To(BeNumerically("~", 4_400.0, 1e-6))
			Expect(buys[0].Qty).To(BeNumerically("~", 0.0, 1e-6))
		})
	})
})
