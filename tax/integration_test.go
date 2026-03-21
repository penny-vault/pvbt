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

// echoFillBroker is a minimal broker that immediately fills every submitted
// order at the close price from a configurable price map. It echoes back
// the order ID so that Account.drainFillsFromChannel can match the fill
// to the pending order.
type echoFillBroker struct {
	prices map[asset.Asset]float64
	fillCh chan broker.Fill
}

func newEchoFillBroker(prices map[asset.Asset]float64) *echoFillBroker {
	return &echoFillBroker{
		prices: prices,
		fillCh: make(chan broker.Fill, 64),
	}
}

func (b *echoFillBroker) Connect(_ context.Context) error { return nil }
func (b *echoFillBroker) Close() error                    { return nil }
func (b *echoFillBroker) Fills() <-chan broker.Fill        { return b.fillCh }

func (b *echoFillBroker) Submit(_ context.Context, order broker.Order) error {
	price, ok := b.prices[order.Asset]
	if !ok {
		return nil
	}

	qty := order.Qty
	if qty == 0 && order.Amount > 0 {
		qty = float64(int(order.Amount / price))
	}

	if qty == 0 {
		return nil
	}

	b.fillCh <- broker.Fill{
		OrderID:  order.ID,
		Price:    price,
		Qty:      qty,
		FilledAt: time.Now(),
	}

	return nil
}

func (b *echoFillBroker) Cancel(_ context.Context, _ string) error   { return nil }
func (b *echoFillBroker) Replace(_ context.Context, _ string, _ broker.Order) error {
	return nil
}
func (b *echoFillBroker) Orders(_ context.Context) ([]broker.Order, error) { return nil, nil }
func (b *echoFillBroker) Positions(_ context.Context) ([]broker.Position, error) {
	return nil, nil
}
func (b *echoFillBroker) Balance(_ context.Context) (broker.Balance, error) {
	return broker.Balance{}, nil
}

var _ broker.Broker = (*echoFillBroker)(nil)

var _ = Describe("Integration: full tax optimization flow", func() {
	var (
		spy asset.Asset
		ivv asset.Asset
		ctx context.Context
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		ivv = asset.Asset{CompositeFigi: "IVV001", Ticker: "IVV"}
		ctx = context.Background()
	})

	It("harvests losses, substitutes, and swaps back after the wash window", func() {
		// ---- Phase 1: Setup account with multiple lots of SPY ----

		// Start with $100,000 plus enough to buy lots.
		lot1Date := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		lot2Date := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
		lot3Date := time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)

		// We buy:
		//   Lot 1: 50 shares @ $200 = $10,000
		//   Lot 2: 30 shares @ $210 = $ 6,300
		//   Lot 3: 20 shares @ $220 = $ 4,400
		// Total cost basis = $20,700 for 100 shares
		// Starting cash must cover lots + leave headroom.
		startCash := 100_000.0 + 200.0*50 + 210.0*30 + 220.0*20

		acct := portfolio.New(
			portfolio.WithCash(startCash, time.Time{}),
			portfolio.WithDefaultLotSelection(portfolio.LotHighestCost),
		)

		buyLot(acct, spy, lot1Date, 200.0, 50)
		buyLot(acct, spy, lot2Date, 210.0, 30)
		buyLot(acct, spy, lot3Date, 220.0, 20)

		// Verify starting position.
		Expect(acct.Position(spy)).To(Equal(100.0))

		// ---- Phase 2: Configure harvester middleware ----

		harvester := tax.NewTaxLossHarvester(tax.HarvesterConfig{
			LossThreshold: 0.05,
			Substitutes:   map[asset.Asset]asset.Asset{spy: ivv},
		})
		acct.Use(harvester)

		// ---- Phase 3: Update prices so SPY drops > 5% ----

		harvestDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		// SPY at $190 is down from weighted avg cost ($207) by ~8.2%.
		spyDropPrice := 190.0
		ivvPrice := 185.0

		mockBroker := newEchoFillBroker(map[asset.Asset]float64{
			spy: spyDropPrice,
			ivv: ivvPrice,
		})
		acct.SetBroker(mockBroker)

		df, err := data.NewDataFrame(
			[]time.Time{harvestDate},
			[]asset.Asset{spy, ivv},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{spyDropPrice, ivvPrice},
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdatePrices(df)

		// ---- Phase 4: Execute batch -- middleware should inject harvest orders ----

		batch := acct.NewBatch(harvestDate)
		err = acct.ExecuteBatch(ctx, batch)
		Expect(err).NotTo(HaveOccurred())

		// Verify: sell order for SPY was injected with justification.
		sells := ordersWithSide(batch.Orders, broker.Sell)
		Expect(sells).To(HaveLen(1), "expected one sell order for SPY")
		Expect(sells[0].Asset).To(Equal(spy))
		Expect(sells[0].Justification).To(ContainSubstring("tax-loss harvest"))

		// Verify: buy order for IVV injected (substitute).
		buys := ordersWithSide(batch.Orders, broker.Buy)
		Expect(buys).To(HaveLen(1), "expected one buy order for IVV substitute")
		Expect(buys[0].Asset).To(Equal(ivv))
		Expect(buys[0].Justification).To(ContainSubstring("substitute"))

		// Verify: substitution registered -- ActiveSubstitutions shows SPY->IVV.
		subs := acct.ActiveSubstitutions()
		Expect(subs).To(HaveKey(spy))
		Expect(subs[spy].Substitute).To(Equal(ivv))
		Expect(subs[spy].Original).To(Equal(spy))

		// Verify: after fills are processed, SPY is sold and IVV is held.
		// The real holdings should show IVV, but the logical Holdings()
		// should show SPY (because substitution maps IVV -> SPY).
		Expect(acct.Position(spy)).To(Equal(0.0), "SPY should be sold at physical level")
		Expect(acct.Position(ivv)).To(BeNumerically(">", 0.0), "IVV should be held as substitute")

		// Logical view via Holdings() should report SPY, not IVV.
		logicalHoldings := make(map[asset.Asset]float64)
		acct.Holdings(func(ast asset.Asset, qty float64) {
			logicalHoldings[ast] = qty
		})
		Expect(logicalHoldings).To(HaveKey(spy), "logical view should show SPY")
		Expect(logicalHoldings).NotTo(HaveKey(ivv), "logical view should not show IVV")

		// Verify: wash sale records are empty (first harvest, no prior loss sale
		// that would trigger a wash sale on rebuy).
		washRecords := acct.WashSaleWindow(spy)
		Expect(washRecords).To(BeEmpty(), "no wash sales expected on first harvest")

		// ---- Phase 5: Swap-back after wash sale window expires ----

		// Advance 31 days past the harvest date.
		swapBackDate := harvestDate.AddDate(0, 0, 31)
		spySwapBackPrice := 195.0
		ivvSwapBackPrice := 190.0

		// Update broker prices for the swap-back.
		swapBackBroker := newEchoFillBroker(map[asset.Asset]float64{
			spy: spySwapBackPrice,
			ivv: ivvSwapBackPrice,
		})
		acct.SetBroker(swapBackBroker)

		dfSwap, err := data.NewDataFrame(
			[]time.Time{swapBackDate},
			[]asset.Asset{spy, ivv},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{spySwapBackPrice, ivvSwapBackPrice},
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdatePrices(dfSwap)

		// Execute a new batch -- middleware should inject swap-back orders.
		swapBatch := acct.NewBatch(swapBackDate)
		err = acct.ExecuteBatch(ctx, swapBatch)
		Expect(err).NotTo(HaveOccurred())

		// Verify: swap-back orders injected (sell IVV, buy SPY).
		swapSells := ordersWithSide(swapBatch.Orders, broker.Sell)
		Expect(swapSells).To(HaveLen(1), "expected sell IVV in swap-back")
		Expect(swapSells[0].Asset).To(Equal(ivv))
		Expect(swapSells[0].Justification).To(ContainSubstring("swap-back"))

		swapBuys := ordersWithSide(swapBatch.Orders, broker.Buy)
		Expect(swapBuys).To(HaveLen(1), "expected buy SPY in swap-back")
		Expect(swapBuys[0].Asset).To(Equal(spy))
		Expect(swapBuys[0].Justification).To(ContainSubstring("swap-back"))

		// After swap-back, SPY should be held again.
		Expect(acct.Position(spy)).To(BeNumerically(">", 0.0), "SPY should be held after swap-back")
		Expect(acct.Position(ivv)).To(Equal(0.0), "IVV should be fully sold after swap-back")
	})
})
