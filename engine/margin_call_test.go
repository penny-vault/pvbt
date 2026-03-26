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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// marginShortStrategy opens a large short position on the first Compute
// call and does nothing afterward. The price data is arranged so the
// stock rises sharply, triggering a margin call.
type marginShortStrategy struct {
	target    asset.Asset
	qty       float64
	callCount int
}

func (s *marginShortStrategy) Name() string           { return "marginShort" }
func (s *marginShortStrategy) Setup(_ *engine.Engine) {}
func (s *marginShortStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *marginShortStrategy) Compute(ctx context.Context, _ *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	s.callCount++
	if s.callCount == 1 {
		batch.Order(ctx, s.target, portfolio.Sell, s.qty)
	}

	return nil
}

// marginCallHandlerStrategy implements MarginCallHandler. It covers all
// short positions when OnMarginCall is called.
type marginCallHandlerStrategy struct {
	target          asset.Asset
	qty             float64
	callCount       int
	marginCallCount int
}

func (s *marginCallHandlerStrategy) Name() string           { return "marginCallHandler" }
func (s *marginCallHandlerStrategy) Setup(_ *engine.Engine) {}
func (s *marginCallHandlerStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *marginCallHandlerStrategy) Compute(ctx context.Context, _ *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	s.callCount++
	if s.callCount == 1 {
		batch.Order(ctx, s.target, portfolio.Sell, s.qty)
	}

	return nil
}

func (s *marginCallHandlerStrategy) OnMarginCall(ctx context.Context, _ *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
	s.marginCallCount++

	// Cover all shorts.
	port.Holdings(func(held asset.Asset, qty float64) {
		if qty < 0 {
			batch.Order(ctx, held, portfolio.Buy, -qty)
		}
	})

	return nil
}

var _ = Describe("Margin Call", func() {
	var (
		testStock     asset.Asset
		assetProvider *mockAssetProvider
		allMetrics    []data.Metric
	)

	BeforeEach(func() {
		testStock = asset.Asset{CompositeFigi: "FIGI-MRGN", Ticker: "MRGN"}
		assetProvider = &mockAssetProvider{assets: []asset.Asset{testStock}}
		allMetrics = []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume}
	})

	// makeMarginTestData creates a DataFrame where the stock starts at
	// startPrice and rises to endPrice over nDays. This simulates a
	// scenario where a short seller faces mounting losses.
	makeMarginTestData := func(startPrice, endPrice float64, nDays int, dataStart time.Time) *data.DataFrame {
		times := make([]time.Time, nDays)
		for idx := range times {
			day := dataStart.AddDate(0, 0, idx)
			times[idx] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
		}

		priceStep := (endPrice - startPrice) / float64(nDays-1)
		vals := make([]float64, nDays*1*len(allMetrics))

		for dayIdx := 0; dayIdx < nDays; dayIdx++ {
			price := startPrice + priceStep*float64(dayIdx)
			vals[0*nDays+dayIdx] = price       // Close
			vals[1*nDays+dayIdx] = price       // AdjClose
			vals[2*nDays+dayIdx] = 0.0         // Dividend
			vals[3*nDays+dayIdx] = price + 2.0 // High
			vals[4*nDays+dayIdx] = price - 2.0 // Low
			vals[5*nDays+dayIdx] = 1.0         // SplitFactor
			vals[6*nDays+dayIdx] = 1_000_000.0 // Volume
		}

		numCols := 1 * len(allMetrics)
		testDF, err := data.NewDataFrame(times, []asset.Asset{testStock}, allMetrics, data.Daily, data.SlabToColumns(vals, numCols, nDays))
		Expect(err).NotTo(HaveOccurred())

		return testDF
	}

	Context("auto-liquidation", func() {
		It("covers short positions proportionally when margin is breached", func() {
			// Setup: $20,000 cash, short 200 shares at $50 (day 1).
			// Short proceeds: 200 * $50 = $10,000 -> cash becomes $30,000.
			// Equity at entry: $30,000 - $10,000 = $20,000.
			// Maintenance margin: 30% of SMV.
			//
			// Price rises to $150 over 30 days.
			// At $150: SMV = 200 * $150 = $30,000.
			// Equity = $30,000 - $30,000 = $0.
			// Required = $30,000 * 0.30 = $9,000.
			// Deficiency = $9,000 - $0 = $9,000. Margin call!
			//
			// The auto-liquidation should cover some shares to restore margin.
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			testDF := makeMarginTestData(50.0, 150.0, 30, dataStart)
			provider := data.NewTestProvider(allMetrics, testDF)

			strategy := &marginShortStrategy{target: testStock, qty: 200}
			acct := portfolio.New(
				portfolio.WithCash(20_000, time.Time{}),
				portfolio.WithBorrowRate(0.0), // no borrow fees for simpler math
			)

			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithAccount(acct),
			)

			btStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			btEnd := time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), btStart, btEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			// The auto-liquidation should have reduced the short position.
			// The final position should be less negative than -200 (some
			// shares were covered).
			finalPosition := fund.Position(testStock)
			Expect(finalPosition).To(BeNumerically(">", -200),
				"expected auto-liquidation to cover some shares, got position %v", finalPosition)

			// Verify that buy transactions with "margin call auto-liquidation"
			// justification exist.
			txns := fund.Transactions()
			hasMarginCover := false

			for _, txn := range txns {
				if txn.Type == asset.BuyTransaction && txn.Justification == "margin call auto-liquidation" {
					hasMarginCover = true

					break
				}
			}

			Expect(hasMarginCover).To(BeTrue(), "expected margin call auto-liquidation buy transactions")
		})
	})

	Context("MarginCallHandler", func() {
		It("calls OnMarginCall when the strategy implements MarginCallHandler", func() {
			// Same setup as auto-liquidation but with a strategy that
			// implements MarginCallHandler.
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			testDF := makeMarginTestData(50.0, 150.0, 30, dataStart)
			provider := data.NewTestProvider(allMetrics, testDF)

			strategy := &marginCallHandlerStrategy{target: testStock, qty: 200}
			acct := portfolio.New(
				portfolio.WithCash(20_000, time.Time{}),
				portfolio.WithBorrowRate(0.0),
			)

			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithAccount(acct),
			)

			btStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			btEnd := time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), btStart, btEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			// The handler should have been called at least once.
			Expect(strategy.marginCallCount).To(BeNumerically(">=", 1),
				"expected OnMarginCall to be called at least once")

			// The handler covers all shorts, so after it runs the position
			// should be 0 (no remaining short).
			finalPosition := fund.Position(testStock)
			Expect(finalPosition).To(BeNumerically(">=", 0),
				"expected handler to fully cover shorts, got position %v", finalPosition)
		})
	})
})
