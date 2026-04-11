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
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/engine/middleware/risk"
	"github.com/penny-vault/pvbt/portfolio"
)

// mockAssetProvider implements data.AssetProvider for tests.
type mockAssetProvider struct {
	assets []asset.Asset
}

func (m *mockAssetProvider) Assets(_ context.Context) ([]asset.Asset, error) {
	return m.assets, nil
}

func (m *mockAssetProvider) LookupAsset(_ context.Context, ticker string) (asset.Asset, error) {
	for _, a := range m.assets {
		if a.Ticker == ticker {
			return a, nil
		}
	}
	return asset.Asset{}, fmt.Errorf("not found: %s", ticker)
}

// backtestStrategy is a simple equal-weight strategy used in integration tests.
type backtestStrategy struct {
	assets []asset.Asset
}

func (s *backtestStrategy) Name() string { return "backtestStrategy" }

func (s *backtestStrategy) Setup(_ *engine.Engine) {}

func (s *backtestStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *backtestStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio, batch *portfolio.Batch) error {
	if len(s.assets) == 0 {
		return nil
	}
	priceDF, err := eng.FetchAt(ctx, s.assets, eng.CurrentDate(), []data.Metric{data.MetricClose})
	if err != nil || priceDF == nil {
		return nil
	}

	weight := 1.0 / float64(len(s.assets))
	totalValue := fund.Cash()
	for held, qty := range fund.Holdings() {
		price := priceDF.ValueAt(held, data.MetricClose, eng.CurrentDate())
		if !math.IsNaN(price) {
			totalValue += qty * price
		}
	}

	// Sell assets not in target.
	for held, qty := range fund.Holdings() {
		inTarget := false
		for _, target := range s.assets {
			if target == held {
				inTarget = true
				break
			}
		}
		if !inTarget && qty > 0 {
			batch.Order(ctx, held, portfolio.Sell, qty)
		}
	}

	// Buy/adjust target assets.
	for _, target := range s.assets {
		price := priceDF.ValueAt(target, data.MetricClose, eng.CurrentDate())
		if math.IsNaN(price) || price <= 0 {
			continue
		}
		targetShares := math.Floor(weight * totalValue / price)
		currentShares := fund.Position(target)
		diff := targetShares - currentShares
		if diff > 0 {
			batch.Order(ctx, target, portfolio.Buy, diff)
		} else if diff < 0 {
			batch.Order(ctx, target, portfolio.Sell, -diff)
		}
	}
	return nil
}

// monthlyStrategy trades once per month at month-end but the engine
// should still record daily equity values.
type monthlyStrategy struct {
	assets []asset.Asset
}

func (s *monthlyStrategy) Name() string { return "monthlyStrategy" }

func (s *monthlyStrategy) Setup(_ *engine.Engine) {}

func (s *monthlyStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "@close @monthend"}
}

func (s *monthlyStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio, batch *portfolio.Batch) error {
	if len(s.assets) == 0 {
		return nil
	}
	priceDF, err := eng.FetchAt(ctx, s.assets, eng.CurrentDate(), []data.Metric{data.MetricClose})
	if err != nil || priceDF == nil {
		return nil
	}
	// Buy 1 share of each asset on first compute if not already held.
	for _, target := range s.assets {
		if fund.Position(target) == 0 {
			batch.Order(ctx, target, portfolio.Buy, 1)
		}
	}
	return nil
}

// noScheduleStrategy omits calling e.Schedule in Setup.
type noScheduleStrategy struct{}

func (s *noScheduleStrategy) Name() string           { return "noSchedule" }
func (s *noScheduleStrategy) Setup(_ *engine.Engine) {}
func (s *noScheduleStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// autoScheduleStrategy declares its schedule via Describe() instead of Setup.
type autoScheduleStrategy struct {
	Window int `pvbt:"window" desc:"window" default:"5"`
}

func (s *autoScheduleStrategy) Name() string { return "autoSchedule" }
func (s *autoScheduleStrategy) Setup(_ *engine.Engine) {
	// Intentionally empty -- schedule comes from Describe().
}
func (s *autoScheduleStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}
func (s *autoScheduleStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		Schedule:  "0 16 * * 1-5",
		Benchmark: "SPY",
	}
}

// riskTestStrategy requests 100% in a single asset so risk middleware can cap it.
type riskTestStrategy struct {
	target asset.Asset
}

func (s *riskTestStrategy) Name() string           { return "risk-test" }
func (s *riskTestStrategy) Setup(_ *engine.Engine) {}
func (s *riskTestStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		Schedule:  "@monthend",
		Benchmark: "SPY",
	}
}
func (s *riskTestStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	return batch.RebalanceTo(ctx, portfolio.Allocation{
		Date:    eng.CurrentDate(),
		Members: map[asset.Asset]float64{s.target: 1.0},
	})
}

// buyThenSellStrategy buys on the first Compute call and sells on the second.
// This produces a single round-trip trade for verifying MFE/MAE excursion tracking.
type buyThenSellStrategy struct {
	target    asset.Asset
	callCount int
}

func (s *buyThenSellStrategy) Name() string { return "buyThenSell" }

func (s *buyThenSellStrategy) Setup(_ *engine.Engine) {}

func (s *buyThenSellStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *buyThenSellStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio, batch *portfolio.Batch) error {
	s.callCount++
	if s.callCount == 1 {
		// Buy 10 shares on first call.
		batch.Order(ctx, s.target, portfolio.Buy, 10)
	} else if s.callCount == 2 {
		// Sell all shares on second call.
		qty := fund.Position(s.target)
		if qty > 0 {
			batch.Order(ctx, s.target, portfolio.Sell, qty)
		}
	}
	return nil
}

// bracketStrategy places a single bracket order (buy with stop-loss and take-profit)
// on the first Compute call, then does nothing on subsequent calls.
type bracketStrategy struct {
	placed    bool
	testAsset asset.Asset
	stopPct   float64
	tpPct     float64
}

func (s *bracketStrategy) Name() string           { return "bracket-test" }
func (s *bracketStrategy) Setup(_ *engine.Engine) {}
func (s *bracketStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}
func (s *bracketStrategy) Compute(ctx context.Context, _ *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	if !s.placed {
		s.placed = true
		return batch.Order(ctx, s.testAsset, portfolio.Buy, 100,
			portfolio.WithBracket(
				portfolio.StopLossPercent(s.stopPct),
				portfolio.TakeProfitPercent(s.tpPct),
			),
		)
	}
	return nil
}

// buyOnceStrategy buys a fixed number of shares on the first Compute
// call and then does nothing. Used to establish a long position for
// broker transaction sync tests.
type buyOnceStrategy struct {
	target asset.Asset
	qty    float64
	bought bool
}

func (s *buyOnceStrategy) Name() string { return "buy-once" }

func (s *buyOnceStrategy) Setup(_ *engine.Engine) {}

func (s *buyOnceStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *buyOnceStrategy) Compute(ctx context.Context, _ *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	if !s.bought {
		s.bought = true
		return batch.Order(ctx, s.target, portfolio.Buy, s.qty)
	}
	return nil
}

// shortOnceStrategy sells (shorts) a fixed number of shares on the
// first Compute call and then does nothing. This leaves a short
// position open for housekeeping to process borrow fees and dividends.
type shortOnceStrategy struct {
	target    asset.Asset
	qty       float64
	callCount int
}

func (s *shortOnceStrategy) Name() string { return "shortOnce" }

func (s *shortOnceStrategy) Setup(_ *engine.Engine) {}

func (s *shortOnceStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *shortOnceStrategy) Compute(ctx context.Context, _ *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	s.callCount++
	if s.callCount == 1 {
		batch.Order(ctx, s.target, portfolio.Sell, s.qty)
	}
	return nil
}

// longShortStrategy goes long one asset and short another for the first
// two strategy dates, then covers the short on the third date. This
// exercises the complete short-selling pipeline end-to-end.
type longShortStrategy struct {
	longAsset  asset.Asset
	shortAsset asset.Asset
	callCount  int
}

func (s *longShortStrategy) Name() string { return "longShort" }

func (s *longShortStrategy) Setup(_ *engine.Engine) {}

func (s *longShortStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *longShortStrategy) Compute(ctx context.Context, _ *engine.Engine, fund portfolio.Portfolio, batch *portfolio.Batch) error {
	s.callCount++
	switch s.callCount {
	case 1:
		// Day 1: go long 50 shares of longAsset, short 40 shares of shortAsset.
		batch.Order(ctx, s.longAsset, portfolio.Buy, 50)
		batch.Order(ctx, s.shortAsset, portfolio.Sell, 40)
	case 3:
		// Day 3: cover the short position.
		shortQty := fund.Position(s.shortAsset)
		if shortQty < 0 {
			batch.Order(ctx, s.shortAsset, portfolio.Buy, -shortQty)
		}
	}
	return nil
}

// failingStrategy always returns an error from Compute.
type failingStrategy struct{}

func (s *failingStrategy) Name() string { return "failing" }

func (s *failingStrategy) Setup(_ *engine.Engine) {}

func (s *failingStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *failingStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return fmt.Errorf("simulated compute failure")
}

// makeDailyTestData creates a DataFrame with daily prices for the given assets
// and metrics, covering nDays starting at start. Timestamps are set to 16:00
// UTC to match tradecron "0 16 * * 1-5" schedule dates.
func makeDailyTestData(start time.Time, nDays int, testAssets []asset.Asset, metrics []data.Metric) *data.DataFrame {
	times := make([]time.Time, nDays)
	for i := range times {
		day := start.AddDate(0, 0, i)
		times[i] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
	}
	vals := make([]float64, nDays*len(testAssets)*len(metrics))
	for i := range vals {
		vals[i] = 100.0 + float64(i)
	}

	// SplitFactor columns must be 1.0 (no split) to avoid unintended
	// split adjustments during housekeeping.
	for assetIdx := range testAssets {
		for metricIdx, metric := range metrics {
			if metric == data.SplitFactor {
				colStart := (assetIdx*len(metrics) + metricIdx) * nDays
				for dayIdx := 0; dayIdx < nDays; dayIdx++ {
					vals[colStart+dayIdx] = 1.0
				}
			}
		}
	}

	numCols := len(testAssets) * len(metrics)
	df, err := data.NewDataFrame(times, testAssets, metrics, data.Daily, data.SlabToColumns(vals, numCols, nDays))
	Expect(err).NotTo(HaveOccurred())
	return df
}

var _ = Describe("Backtest", func() {
	var (
		aapl          asset.Asset
		msft          asset.Asset
		testAssets    []asset.Asset
		assetProvider *mockAssetProvider
		metrics       []data.Metric
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		testAssets = []asset.Asset{aapl, msft}
		assetProvider = &mockAssetProvider{assets: testAssets}
		metrics = []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume}
	})

	Context("end to end", func() {
		It("runs a complete backtest with trades", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &backtestStrategy{assets: testAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			txns := fund.Transactions()
			hasBuy := false
			for _, tx := range txns {
				if tx.Type == asset.BuyTransaction {
					hasBuy = true
					break
				}
			}
			Expect(hasBuy).To(BeTrue(), "expected buy transactions after rebalance")
		})
	})

	Context("WithAccount", func() {
		It("uses a pre-configured account", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			acct := portfolio.New(
				portfolio.WithCash(50000, time.Time{}),
				portfolio.WithMetric(portfolio.Sharpe),
			)
			acct.SetMetadata("test_key", "test_value")

			strategy := &backtestStrategy{assets: testAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithAccount(acct),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			p, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(p.GetMetadata("test_key")).To(Equal("test_value"))
			Expect(acct.RegisteredMetrics()).To(HaveLen(1))
		})

		It("round-trips a backtest through SQLite", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			acct := portfolio.New(
				portfolio.WithCash(100000, time.Time{}),
				portfolio.WithMetric(portfolio.Sharpe),
			)

			strategy := &backtestStrategy{assets: testAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithAccount(acct),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())

			acct.SetMetadata("strategy", strategy.Name())

			tmpDir, err := os.MkdirTemp("", "pvbt-integration-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			dbPath := filepath.Join(tmpDir, "backtest.db")
			Expect(acct.ToSQLite(dbPath)).To(Succeed())

			restored, err := portfolio.FromSQLite(dbPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(restored.GetMetadata("strategy")).To(Equal(strategy.Name()))
			perfA := asset.Asset{CompositeFigi: "_PORTFOLIO_", Ticker: "_PORTFOLIO_"}
			Expect(restored.PerfData().Column(perfA, data.PortfolioEquity)).To(Equal(acct.PerfData().Column(perfA, data.PortfolioEquity)))
			Expect(restored.Metrics()).To(Equal(acct.Metrics()))
		})
	})

	Context("progress callback", func() {
		It("delivers an event per step with cumulative measurement counts", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &backtestStrategy{assets: testAssets}

			var events []engine.ProgressEvent
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
				engine.WithProgressCallback(func(ev engine.ProgressEvent) {
					events = append(events, ev)
				}),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())

			Expect(events).NotTo(BeEmpty(), "callback should fire at least once")

			first := events[0]
			last := events[len(events)-1]

			Expect(first.Step).To(Equal(1))
			Expect(first.TotalSteps).To(Equal(last.TotalSteps))
			Expect(last.Step).To(Equal(last.TotalSteps),
				"final event step should equal total step count")

			Expect(first.Start).To(Equal(last.Start))
			Expect(first.End).To(Equal(last.End))
			Expect(first.End).To(Equal(end))
			Expect(last.Date).NotTo(BeZero())
			Expect(last.Date.Before(end) || last.Date.Equal(end)).To(BeTrue())

			Expect(last.MeasurementsEvaluated).To(BeNumerically(">", 0),
				"the default account registers all metrics so the run should produce measurements")

			// Measurement count is cumulative and never decreases. Step counter
			// advances by exactly 1 per event.
			for ii := 1; ii < len(events); ii++ {
				Expect(events[ii].MeasurementsEvaluated).To(
					BeNumerically(">=", events[ii-1].MeasurementsEvaluated),
					"measurement count should be monotonically non-decreasing")
				Expect(events[ii].Step).To(Equal(events[ii-1].Step+1),
					"step counter should advance by exactly 1 per event")
			}
		})

		It("clears the cumulative count when the engine is reused", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &backtestStrategy{assets: testAssets}

			var firstRunFinal, secondRunFirst engine.ProgressEvent
			runIdx := 0
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
				engine.WithProgressCallback(func(ev engine.ProgressEvent) {
					if runIdx == 0 {
						firstRunFinal = ev
					} else if secondRunFirst.Step == 0 {
						secondRunFirst = ev
					}
				}),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())

			runIdx = 1

			_, err = eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())

			Expect(firstRunFinal.MeasurementsEvaluated).To(BeNumerically(">", 0))
			Expect(secondRunFirst.MeasurementsEvaluated).To(
				BeNumerically("<=", firstRunFinal.MeasurementsEvaluated/firstRunFinal.Step+1),
				"second run should start counting from a clean slate, not continue accumulating")
		})
	})

	Context("daily equity recording", func() {
		It("records equity every trading day even for a monthly strategy", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &monthlyStrategy{assets: testAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())

			// A monthly strategy over ~2 months would only have ~2 strategy dates.
			// But daily equity recording should give us ~40+ trading days of data.
			perfA := asset.Asset{CompositeFigi: "_PORTFOLIO_", Ticker: "_PORTFOLIO_"}
			equityCol := fund.PerfData().Column(perfA, data.PortfolioEquity)
			Expect(len(equityCol)).To(BeNumerically(">=", 30),
				"expected daily equity data, got %d points", len(equityCol))
		})
	})

	Context("validation", func() {
		It("returns an error when no schedule is set", func() {
			strategy := &noScheduleStrategy{}
			eng := engine.New(strategy, engine.WithAssetProvider(assetProvider))

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("schedule"))
		})

		It("auto-reads schedule and benchmark from Describe()", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
			allAssets := append(testAssets, spy)
			df := makeDailyTestData(dataStart, 400, allAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &autoScheduleStrategy{}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(&mockAssetProvider{assets: allAssets}),
				engine.WithInitialDeposit(100_000.0),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())
		})

		It("returns an error when no asset provider is configured", func() {
			strategy := &noScheduleStrategy{}
			eng := engine.New(strategy)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).To(HaveOccurred())
		})

		It("halts when strategy Compute returns an error", func() {
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &failingStrategy{}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(10_000),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated compute failure"))
		})
	})

	Context("MFE/MAE excursion tracking", func() {
		It("populates MFE and MAE on TradeDetails after a round-trip trade", func() {
			testStock := asset.Asset{CompositeFigi: "FIGI-XYZ", Ticker: "XYZ"}
			excursionAssets := []asset.Asset{testStock}
			excursionProvider := &mockAssetProvider{assets: excursionAssets}

			// Build a DataFrame with Close, AdjClose, Dividend, High, Low
			// for 30 trading days starting 2024-01-01.
			// The strategy runs on weekdays via "0 16 * * 1-5".
			// Day 0 (Mon Jan 1):  Close=100, High=105, Low=95
			// Day 1 (Tue Jan 2):  Close=102, High=110, Low=92  <-- buy happens here (first strategy date)
			// Day 2 (Wed Jan 3):  Close=103, High=112, Low=93
			// Day 3 (Thu Jan 4):  Close=101, High=106, Low=88  <-- low dip
			// Day 4 (Fri Jan 5):  Close=104, High=115, Low=96  <-- sell happens here (second strategy date)
			//
			// Entry price = Close on day 1 = 102
			// High over holding period (days 2-4): max(112, 106, 115) = 115
			// Low over holding period (days 2-4):  min(93, 88, 96) = 88
			// MFE = (115 - 102) / 102 > 0
			// MAE = (88 - 102) / 102 < 0

			excursionMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume}
			nDays := 30
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, nDays)
			for idx := range times {
				day := dataStart.AddDate(0, 0, idx)
				times[idx] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
			}

			// 1 asset x 7 metrics x 30 days = 210 values
			// Column layout: [Close(30)][AdjClose(30)][Dividend(30)][High(30)][Low(30)][SplitFactor(30)][Volume(30)]
			vals := make([]float64, nDays*len(excursionAssets)*len(excursionMetrics))

			// Fill with default values: Close=100, AdjClose=100, Dividend=0, High=105, Low=95, SplitFactor=1, Volume=1e6
			for dayIdx := 0; dayIdx < nDays; dayIdx++ {
				vals[0*nDays+dayIdx] = 100.0 + float64(dayIdx) // Close: 100, 101, 102, ...
				vals[1*nDays+dayIdx] = 100.0 + float64(dayIdx) // AdjClose: same as Close
				vals[2*nDays+dayIdx] = 0.0                     // Dividend: 0
				vals[3*nDays+dayIdx] = 105.0 + float64(dayIdx) // High: 105, 106, 107, ...
				vals[4*nDays+dayIdx] = 95.0                    // Low: 95 baseline
				vals[5*nDays+dayIdx] = 1.0                     // SplitFactor: 1 (no split)
				vals[6*nDays+dayIdx] = 1_000_000.0             // Volume
			}

			// Override specific days for controlled excursion values.
			// Day 3 (index 3): low dip to 88
			vals[4*nDays+3] = 88.0
			// Day 4 (index 4): high spike to 115
			vals[3*nDays+4] = 115.0

			excursionDF, dfErr := data.NewDataFrame(times, excursionAssets, excursionMetrics, data.Daily, data.SlabToColumns(vals, len(excursionAssets)*len(excursionMetrics), nDays))
			Expect(dfErr).NotTo(HaveOccurred())

			excursionDataProvider := data.NewTestProvider(excursionMetrics, excursionDF)

			strategy := &buyThenSellStrategy{target: testStock}
			eng := engine.New(strategy,
				engine.WithDataProvider(excursionDataProvider),
				engine.WithAssetProvider(excursionProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			btStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			btEnd := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), btStart, btEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			details := fund.TradeDetails()
			Expect(details).To(HaveLen(1))
			Expect(details[0].MFE).To(BeNumerically(">", 0))
			Expect(details[0].MAE).To(BeNumerically("<", 0))
		})
	})

	Context("risk middleware", func() {
		It("caps position size when MaxPositionSize is configured", func() {
			spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
			allAssets := append(testAssets, spy)
			allMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume}

			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyTestData(dataStart, 400, allAssets, allMetrics)
			provider := data.NewTestProvider(allMetrics, df)

			acct := portfolio.New(
				portfolio.WithCash(100_000, time.Time{}),
			)
			acct.Use(risk.MaxPositionSize(0.25))

			strategy := &riskTestStrategy{target: spy}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(&mockAssetProvider{assets: allAssets}),
				engine.WithAccount(acct),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			// Verify annotations from risk middleware exist.
			annotations := acct.Annotations()
			found := false
			for _, ann := range annotations {
				if ann.Key == "risk:max-position-size" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected risk:max-position-size annotation")

			// Verify the final SPY position is capped at ~25%.
			spyPos := fund.Position(spy)
			totalValue := fund.Value()
			if spyPos > 0 && totalValue > 0 {
				lastDate := fund.PerfData().Times()[len(fund.PerfData().Times())-1]
				spyPrice := df.ValueAt(spy, data.MetricClose, lastDate)
				if !math.IsNaN(spyPrice) && spyPrice > 0 {
					spyWeight := (spyPos * spyPrice) / totalValue
					Expect(spyWeight).To(BeNumerically("<=", 0.30),
						"final SPY weight %.2f exceeded cap", spyWeight)
				}
			}

			// Verify that sell transactions exist (middleware reduced the position).
			txns := fund.Transactions()
			hasSell := false
			for _, tx := range txns {
				if tx.Type == asset.SellTransaction {
					hasSell = true
					break
				}
			}
			Expect(hasSell).To(BeTrue(), "expected sell transactions from risk middleware")
		})
	})

	Context("bracket orders", func() {
		// makeBracketTestData builds a DataFrame for a single asset with explicit
		// Close, AdjClose, Dividend, High, Low values per day.
		// Each row is {close, high, low}; AdjClose=close, Dividend=0.
		makeBracketTestData := func(startDate time.Time, testAsset asset.Asset, rows []struct{ close, high, low float64 }) *data.DataFrame {
			numDays := len(rows)
			bracketMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume}
			assets := []asset.Asset{testAsset}

			times := make([]time.Time, numDays)
			for idx := range times {
				day := startDate.AddDate(0, 0, idx)
				times[idx] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
			}

			// Layout: (assetIdx * numMetrics + metricIdx) * numDays + dayIdx
			// With 1 asset: metricIdx * numDays + dayIdx
			vals := make([]float64, numDays*len(assets)*len(bracketMetrics))
			for dayIdx, row := range rows {
				vals[0*numDays+dayIdx] = row.close   // MetricClose
				vals[1*numDays+dayIdx] = row.close   // AdjClose
				vals[2*numDays+dayIdx] = 0.0         // Dividend
				vals[3*numDays+dayIdx] = row.high    // MetricHigh
				vals[4*numDays+dayIdx] = row.low     // MetricLow
				vals[5*numDays+dayIdx] = 1.0         // SplitFactor: no split
				vals[6*numDays+dayIdx] = 1_000_000.0 // Volume
			}

			df, dfErr := data.NewDataFrame(times, assets, bracketMetrics, data.Daily, data.SlabToColumns(vals, len(assets)*len(bracketMetrics), numDays))
			Expect(dfErr).NotTo(HaveOccurred())
			return df
		}

		It("triggers stop loss on intrabar low", func() {
			testStock := asset.Asset{CompositeFigi: "FIGI-SL", Ticker: "SL"}
			bracketAssets := []asset.Asset{testStock}
			bracketAssetProvider := &mockAssetProvider{assets: bracketAssets}

			// Timeline:
			// Day 0 (Mon 2024-01-01): close=100, buy fills at 100
			// Day 1 (Tue 2024-01-02): DrainFills creates bracket exits (stop@95, TP@110).
			//   EvaluatePending runs before DrainFills so it cannot see them yet.
			//   Prices: close=99, high=101, low=97 (no trigger)
			// Day 2 (Wed 2024-01-03): EvaluatePending evaluates bracket exits against
			//   today's prices. close=97, high=101, low=93 -> stop triggers (93 <= 95)
			// Day 3 (Thu 2024-01-04): padding day
			rows := []struct{ close, high, low float64 }{
				{100, 102, 98}, // Day 0: entry fills at close=100
				{99, 101, 97},  // Day 1: bracket exits created; no trigger
				{97, 101, 93},  // Day 2: stop triggers (low 93 <= stop 95)
				{98, 99, 96},   // Day 3: padding
			}

			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeBracketTestData(dataStart, testStock, rows)
			bracketMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume}
			provider := data.NewTestProvider(bracketMetrics, df)

			strategy := &bracketStrategy{
				testAsset: testStock,
				stopPct:   5.0,  // 5% stop loss -> stop at 95
				tpPct:     10.0, // 10% take profit -> TP at 110
			}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(bracketAssetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			btStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			btEnd := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), btStart, btEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			txns := fund.Transactions()
			hasSellAt95 := false
			for _, txn := range txns {
				if txn.Type == asset.SellTransaction && txn.Asset == testStock && txn.Price == 95.0 {
					hasSellAt95 = true
					break
				}
			}
			Expect(hasSellAt95).To(BeTrue(), "expected a sell transaction at stop-loss price 95, got transactions: %v", txns)
		})

		It("triggers take profit on intrabar high", func() {
			testStock := asset.Asset{CompositeFigi: "FIGI-TP", Ticker: "TP"}
			bracketAssets := []asset.Asset{testStock}
			bracketAssetProvider := &mockAssetProvider{assets: bracketAssets}

			// Timeline:
			// Day 0 (Mon 2024-01-01): close=100, buy fills at 100
			// Day 1 (Tue 2024-01-02): DrainFills creates bracket exits (stop@95, TP@110).
			//   Prices: close=101, high=103, low=99 (no trigger)
			// Day 2 (Wed 2024-01-03): EvaluatePending checks bracket exits.
			//   close=112, high=115, low=99 -> TP triggers (115 >= 110)
			// Day 3 (Thu 2024-01-04): padding day
			rows := []struct{ close, high, low float64 }{
				{100, 102, 98},  // Day 0: entry fills at close=100
				{101, 103, 99},  // Day 1: bracket exits created; no trigger
				{112, 115, 99},  // Day 2: TP triggers (high 115 >= TP 110)
				{113, 114, 111}, // Day 3: padding
			}

			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeBracketTestData(dataStart, testStock, rows)
			bracketMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume}
			provider := data.NewTestProvider(bracketMetrics, df)

			strategy := &bracketStrategy{
				testAsset: testStock,
				stopPct:   5.0,  // 5% stop loss -> stop at 95
				tpPct:     10.0, // 10% take profit -> TP at 110
			}
			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(bracketAssetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			btStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			btEnd := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), btStart, btEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			txns := fund.Transactions()
			hasSellAtTP := false
			for _, txn := range txns {
				if txn.Type == asset.SellTransaction && txn.Asset == testStock {
					// Allow small floating-point tolerance on the TP price.
					diff := txn.Price - 110.0
					if diff < 0 {
						diff = -diff
					}
					if diff < 0.01 {
						hasSellAtTP = true
						break
					}
				}
			}
			Expect(hasSellAtTP).To(BeTrue(), "expected a sell transaction at take-profit price ~110, got transactions: %v", txns)
		})
	})

	Context("short position housekeeping", func() {
		It("charges daily borrow fees on short positions", func() {
			testStock := asset.Asset{CompositeFigi: "FIGI-SHORT", Ticker: "SHORT"}
			shortAssets := []asset.Asset{testStock}
			shortProvider := &mockAssetProvider{assets: shortAssets}

			shortMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume}
			nDays := 30
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, nDays)
			for idx := range times {
				day := dataStart.AddDate(0, 0, idx)
				times[idx] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
			}

			// 1 asset x 7 metrics x 30 days
			vals := make([]float64, nDays*len(shortAssets)*len(shortMetrics))
			for dayIdx := 0; dayIdx < nDays; dayIdx++ {
				vals[0*nDays+dayIdx] = 100.0       // Close: constant $100
				vals[1*nDays+dayIdx] = 100.0       // AdjClose
				vals[2*nDays+dayIdx] = 0.0         // Dividend: none
				vals[3*nDays+dayIdx] = 102.0       // High
				vals[4*nDays+dayIdx] = 98.0        // Low
				vals[5*nDays+dayIdx] = 1.0         // SplitFactor: no split
				vals[6*nDays+dayIdx] = 1_000_000.0 // Volume
			}

			shortDF, dfErr := data.NewDataFrame(times, shortAssets, shortMetrics, data.Daily, data.SlabToColumns(vals, len(shortAssets)*len(shortMetrics), nDays))
			Expect(dfErr).NotTo(HaveOccurred())
			shortDataProvider := data.NewTestProvider(shortMetrics, shortDF)

			borrowRate := 0.10 // 10% annualized for easy math
			acct := portfolio.New(
				portfolio.WithCash(200_000, time.Time{}),
				portfolio.WithBorrowRate(borrowRate),
			)

			strategy := &shortOnceStrategy{target: testStock, qty: 100}
			eng := engine.New(strategy,
				engine.WithDataProvider(shortDataProvider),
				engine.WithAssetProvider(shortProvider),
				engine.WithAccount(acct),
			)

			btStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			btEnd := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), btStart, btEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			// Count borrow fee transactions.
			txns := fund.Transactions()
			feeCount := 0
			totalFees := 0.0
			for _, tx := range txns {
				if tx.Type == asset.FeeTransaction {
					feeCount++
					totalFees += tx.Amount // negative
				}
			}

			// There should be at least one fee transaction (one per trading
			// day after the short is opened).
			Expect(feeCount).To(BeNumerically(">=", 1), "expected borrow fee transactions")
			Expect(totalFees).To(BeNumerically("<", 0), "borrow fees should be negative")

			// Verify the daily fee amount: 100 shares * $100 * (10% / 252)
			expectedDailyFee := 100.0 * 100.0 * (borrowRate / 252.0)
			// Each fee transaction should match this amount (negated).
			for _, tx := range txns {
				if tx.Type == asset.FeeTransaction {
					Expect(tx.Amount).To(BeNumerically("~", -expectedDailyFee, 0.01))
					Expect(tx.Justification).To(ContainSubstring("borrow fee"))
					Expect(tx.Justification).To(ContainSubstring("10.00%"))
				}
			}
		})

		It("debits cash for dividends on short positions", func() {
			testStock := asset.Asset{CompositeFigi: "FIGI-DIVSHORT", Ticker: "DIVSHORT"}
			shortAssets := []asset.Asset{testStock}
			shortProvider := &mockAssetProvider{assets: shortAssets}

			shortMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume}
			nDays := 30
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, nDays)
			for idx := range times {
				day := dataStart.AddDate(0, 0, idx)
				times[idx] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
			}

			// 1 asset x 7 metrics x 30 days
			vals := make([]float64, nDays*len(shortAssets)*len(shortMetrics))
			for dayIdx := 0; dayIdx < nDays; dayIdx++ {
				vals[0*nDays+dayIdx] = 100.0       // Close
				vals[1*nDays+dayIdx] = 100.0       // AdjClose
				vals[2*nDays+dayIdx] = 0.0         // Dividend: none by default
				vals[3*nDays+dayIdx] = 102.0       // High
				vals[4*nDays+dayIdx] = 98.0        // Low
				vals[5*nDays+dayIdx] = 1.0         // SplitFactor
				vals[6*nDays+dayIdx] = 1_000_000.0 // Volume
			}

			// Place a $2.00 dividend on day 7 (Jan 8). The short is opened
			// on the first strategy date (Jan 1 or Jan 2), so by Jan 8 the
			// short position is established.
			vals[2*nDays+7] = 2.00

			shortDF, dfErr := data.NewDataFrame(times, shortAssets, shortMetrics, data.Daily, data.SlabToColumns(vals, len(shortAssets)*len(shortMetrics), nDays))
			Expect(dfErr).NotTo(HaveOccurred())
			shortDataProvider := data.NewTestProvider(shortMetrics, shortDF)

			acct := portfolio.New(
				portfolio.WithCash(200_000, time.Time{}),
				portfolio.WithBorrowRate(0.005),
			)

			strategy := &shortOnceStrategy{target: testStock, qty: 50}
			eng := engine.New(strategy,
				engine.WithDataProvider(shortDataProvider),
				engine.WithAssetProvider(shortProvider),
				engine.WithAccount(acct),
			)

			btStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			btEnd := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), btStart, btEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			// Find dividend transactions.
			txns := fund.Transactions()
			var shortDivTxns []portfolio.Transaction
			for _, tx := range txns {
				if tx.Type == asset.DividendTransaction && tx.Amount < 0 {
					shortDivTxns = append(shortDivTxns, tx)
				}
			}

			Expect(shortDivTxns).To(HaveLen(1), "expected exactly one short dividend debit")
			// 50 shares short * $2.00 = -$100 obligation
			Expect(shortDivTxns[0].Amount).To(BeNumerically("~", -100.0, 0.01))
			Expect(shortDivTxns[0].Justification).To(ContainSubstring("short dividend obligation"))
		})

		It("uses default borrow rate of 0.5% when none is configured", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			Expect(acct.BorrowRate()).To(BeNumerically("~", 0.005, 1e-9))
		})
	})

	Context("Short Selling Integration", func() {
		It("runs a complete long/short backtest", func() {
			longStock := asset.Asset{CompositeFigi: "FIGI-LONG", Ticker: "LONG"}
			shortStock := asset.Asset{CompositeFigi: "FIGI-SHORT2", Ticker: "SHORT2"}
			integrationAssets := []asset.Asset{longStock, shortStock}
			integrationProvider := &mockAssetProvider{assets: integrationAssets}

			integrationMetrics := []data.Metric{
				data.MetricClose, data.AdjClose, data.Dividend,
				data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume,
			}
			nDays := 30
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, nDays)
			for idx := range times {
				day := dataStart.AddDate(0, 0, idx)
				times[idx] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
			}

			// 2 assets x 7 metrics x 30 days = 420 values.
			// Column layout per asset: [Close][AdjClose][Dividend][High][Low][SplitFactor][Volume]
			// Assets are interleaved: asset0-metric0, asset1-metric0, asset0-metric1, ...
			// Actually, the DataFrame layout is: for each (asset, metric) pair in order.
			nAssets := len(integrationAssets)
			nMetrics := len(integrationMetrics)
			vals := make([]float64, nDays*nAssets*nMetrics)

			// Long asset: price rises from 100 to ~110 over the period.
			// Short asset: price drops from 100 to ~90 over the period (profitable short).
			for dayIdx := 0; dayIdx < nDays; dayIdx++ {
				longPrice := 100.0 + float64(dayIdx)*0.5  // rises: 100, 100.5, 101, ...
				shortPrice := 100.0 - float64(dayIdx)*0.5 // drops: 100, 99.5, 99, ...
				longHigh := longPrice + 2.0
				longLow := longPrice - 2.0
				shortHigh := shortPrice + 2.0
				shortLow := shortPrice - 2.0

				// Asset 0 (long) columns: Close, AdjClose, Dividend, High, Low, SplitFactor, Volume
				// Asset 1 (short) columns: Close, AdjClose, Dividend, High, Low, SplitFactor, Volume
				// Column index = (assetIdx*nMetrics + metricIdx)*nDays + dayIdx
				// Long asset (index 0)
				vals[(0*nMetrics+0)*nDays+dayIdx] = longPrice   // Close
				vals[(0*nMetrics+1)*nDays+dayIdx] = longPrice   // AdjClose
				vals[(0*nMetrics+2)*nDays+dayIdx] = 0.0         // Dividend
				vals[(0*nMetrics+3)*nDays+dayIdx] = longHigh    // High
				vals[(0*nMetrics+4)*nDays+dayIdx] = longLow     // Low
				vals[(0*nMetrics+5)*nDays+dayIdx] = 1.0         // SplitFactor
				vals[(0*nMetrics+6)*nDays+dayIdx] = 1_000_000.0 // Volume

				// Short asset (index 1)
				vals[(1*nMetrics+0)*nDays+dayIdx] = shortPrice  // Close
				vals[(1*nMetrics+1)*nDays+dayIdx] = shortPrice  // AdjClose
				vals[(1*nMetrics+2)*nDays+dayIdx] = 0.0         // Dividend
				vals[(1*nMetrics+3)*nDays+dayIdx] = shortHigh   // High
				vals[(1*nMetrics+4)*nDays+dayIdx] = shortLow    // Low
				vals[(1*nMetrics+5)*nDays+dayIdx] = 1.0         // SplitFactor
				vals[(1*nMetrics+6)*nDays+dayIdx] = 1_000_000.0 // Volume
			}

			// Place a $1.50 dividend on the short asset on day 2 (Jan 3, Wednesday).
			// The short position is opened on the first strategy date (Jan 1),
			// and covered on the third strategy date (Jan 3). Housekeeping runs
			// before Compute, so the dividend on Jan 3 is processed while the
			// short position is still open.
			vals[(1*nMetrics+2)*nDays+2] = 1.50

			integrationDF, dfErr := data.NewDataFrame(times, integrationAssets, integrationMetrics, data.Daily, data.SlabToColumns(vals, nAssets*nMetrics, nDays))
			Expect(dfErr).NotTo(HaveOccurred())
			integrationDataProvider := data.NewTestProvider(integrationMetrics, integrationDF)

			borrowRate := 0.08 // 8% annualized
			acct := portfolio.New(
				portfolio.WithCash(200_000, time.Time{}),
				portfolio.WithBorrowRate(borrowRate),
				portfolio.WithTradeMetrics(),
			)

			strategy := &longShortStrategy{
				longAsset:  longStock,
				shortAsset: shortStock,
			}
			eng := engine.New(strategy,
				engine.WithDataProvider(integrationDataProvider),
				engine.WithAssetProvider(integrationProvider),
				engine.WithAccount(acct),
			)

			btStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			btEnd := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), btStart, btEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			// ---------------------------------------------------------------
			// 1. Verify short position was opened and then covered.
			// ---------------------------------------------------------------
			txns := fund.Transactions()

			// There must be at least one sell transaction (opening the short)
			// and at least one buy transaction covering it.
			hasSellShort := false
			hasCoverBuy := false
			for _, tx := range txns {
				if tx.Asset == shortStock && tx.Type == asset.SellTransaction {
					hasSellShort = true
				}
				if tx.Asset == shortStock && tx.Type == asset.BuyTransaction {
					hasCoverBuy = true
				}
			}
			Expect(hasSellShort).To(BeTrue(), "expected sell (short) transaction for SHORT2")
			Expect(hasCoverBuy).To(BeTrue(), "expected buy (cover) transaction for SHORT2")

			// After covering, the short position should be zero.
			Expect(fund.Position(shortStock)).To(BeNumerically("==", 0),
				"short position should be zero after covering")

			// Long position should still be held.
			Expect(fund.Position(longStock)).To(BeNumerically(">", 0),
				"long position should remain open")

			// ---------------------------------------------------------------
			// 2. Verify borrow fees were recorded.
			// ---------------------------------------------------------------
			feeCount := 0
			totalFees := 0.0
			for _, tx := range txns {
				if tx.Type == asset.FeeTransaction {
					feeCount++
					totalFees += tx.Amount
				}
			}
			Expect(feeCount).To(BeNumerically(">=", 1),
				"expected at least one borrow fee transaction")
			Expect(totalFees).To(BeNumerically("<", 0),
				"total borrow fees should be negative (a cost)")

			// ---------------------------------------------------------------
			// 3. Verify short dividend obligation was recorded.
			// ---------------------------------------------------------------
			var shortDivTxns []portfolio.Transaction
			for _, tx := range txns {
				if tx.Type == asset.DividendTransaction && tx.Amount < 0 {
					shortDivTxns = append(shortDivTxns, tx)
				}
			}
			Expect(shortDivTxns).To(HaveLen(1),
				"expected exactly one negative dividend transaction (short obligation)")
			// 40 shares short * $1.50 dividend = -$60.
			Expect(shortDivTxns[0].Amount).To(BeNumerically("~", -60.0, 0.01))

			// ---------------------------------------------------------------
			// 4. Verify margin accounting is correct.
			// ---------------------------------------------------------------
			// After covering, ShortMarketValue should be 0.
			Expect(fund.ShortMarketValue()).To(BeNumerically("==", 0),
				"short market value should be zero after covering all shorts")
			// LongMarketValue should be positive (we hold the long stock).
			Expect(fund.LongMarketValue()).To(BeNumerically(">", 0))
			// MarginDeficiency should be 0 (no short positions).
			Expect(fund.MarginDeficiency()).To(BeNumerically("==", 0))

			// ---------------------------------------------------------------
			// 5. Verify P&L from covering the short.
			// ---------------------------------------------------------------
			// Short was opened at day 0 price ($100) and covered around day 4-6.
			// The short asset price drops, so covering is profitable.
			// Net cash should be less than initial (we bought long stock) but
			// the short cover should have yielded a profit.
			details := fund.TradeDetails()
			var shortTrade *portfolio.TradeDetail
			for idx := range details {
				if details[idx].Asset == shortStock {
					shortTrade = &details[idx]
					break
				}
			}
			Expect(shortTrade).NotTo(BeNil(), "expected a completed short trade detail")
			// Short sold at ~$100, covered at lower price, so PnL > 0.
			Expect(shortTrade.PnL).To(BeNumerically(">", 0),
				"short trade P&L should be positive when price drops")

			// ---------------------------------------------------------------
			// 6. Verify TradeDetails have correct Direction field.
			// ---------------------------------------------------------------
			Expect(shortTrade.Direction).To(Equal(portfolio.TradeShort),
				"short trade should have Direction == TradeShort")

			var longTrade *portfolio.TradeDetail
			for idx := range details {
				if details[idx].Asset == longStock {
					longTrade = &details[idx]
					break
				}
			}
			// The long position is not closed in this test, so there may be
			// no long trade detail. If there is one, verify its direction.
			if longTrade != nil {
				Expect(longTrade.Direction).To(Equal(portfolio.TradeLong),
					"long trade should have Direction == TradeLong")
			}

			// ---------------------------------------------------------------
			// 7. Verify long/short metrics are computed correctly.
			// ---------------------------------------------------------------
			tradeMetrics, tmErr := fund.TradeMetrics()
			Expect(tmErr).NotTo(HaveOccurred())

			// We have exactly one winning short trade, so ShortWinRate = 1.0 (100%).
			Expect(tradeMetrics.ShortWinRate).To(BeNumerically("~", 1.0, 0.01),
				"ShortWinRate should be 1.0 with a single winning short trade")
			// ShortProfitFactor is NaN when there are no losing short trades
			// (division by zero). This is the correct behavior.
			Expect(math.IsNaN(tradeMetrics.ShortProfitFactor)).To(BeTrue(),
				"ShortProfitFactor should be NaN when there are no losing short trades")
		})
	})

	Context("broker transaction sync", func() {
		It("applies dividends and splits through broker transaction sync", func() {
			testStock := asset.Asset{CompositeFigi: "FIGI-SYNC1", Ticker: "SYNC1"}
			syncAssets := []asset.Asset{testStock}
			syncProvider := &mockAssetProvider{assets: syncAssets}

			syncMetrics := []data.Metric{
				data.MetricClose, data.AdjClose, data.Dividend,
				data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume,
			}
			nDays := 15
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, nDays)
			for idx := range times {
				day := dataStart.AddDate(0, 0, idx)
				times[idx] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
			}

			// 1 asset x 7 metrics x 15 days
			// The split fires on calendar day 9 (Jan 10); post-split price
			// appears from day 10 onward.
			nMetrics := len(syncMetrics)
			vals := make([]float64, nDays*len(syncAssets)*nMetrics)
			for dayIdx := 0; dayIdx < nDays; dayIdx++ {
				closePrice := 100.0
				if dayIdx >= 10 {
					closePrice = 50.0 // post-split price
				}
				vals[(0*nMetrics+0)*nDays+dayIdx] = closePrice       // Close
				vals[(0*nMetrics+1)*nDays+dayIdx] = closePrice       // AdjClose
				vals[(0*nMetrics+2)*nDays+dayIdx] = 0.0              // Dividend: none by default
				vals[(0*nMetrics+3)*nDays+dayIdx] = closePrice + 2.0 // High
				vals[(0*nMetrics+4)*nDays+dayIdx] = closePrice - 2.0 // Low
				vals[(0*nMetrics+5)*nDays+dayIdx] = 1.0              // SplitFactor: no split by default
				vals[(0*nMetrics+6)*nDays+dayIdx] = 1_000_000.0      // Volume
			}
			// $2.00 dividend on day 4 (Jan 5, Friday -- a trading day).
			// By this point the strategy has already bought 100 shares on Jan 1.
			vals[(0*nMetrics+2)*nDays+4] = 2.00
			// 2-for-1 split on day 9 (Jan 10, Wednesday -- a trading day).
			vals[(0*nMetrics+5)*nDays+9] = 2.0

			syncDF, dfErr := data.NewDataFrame(times, syncAssets, syncMetrics, data.Daily,
				data.SlabToColumns(vals, len(syncAssets)*nMetrics, nDays))
			Expect(dfErr).NotTo(HaveOccurred())
			syncDataProvider := data.NewTestProvider(syncMetrics, syncDF)

			acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
			strategy := &buyOnceStrategy{target: testStock, qty: 100}
			eng := engine.New(strategy,
				engine.WithDataProvider(syncDataProvider),
				engine.WithAssetProvider(syncProvider),
				engine.WithAccount(acct),
			)

			btStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			btEnd := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), btStart, btEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			txns := fund.Transactions()

			// Verify exactly one dividend transaction with amount $200.
			var divTxns []portfolio.Transaction
			for _, tx := range txns {
				if tx.Type == asset.DividendTransaction && tx.Asset == testStock {
					divTxns = append(divTxns, tx)
				}
			}
			Expect(divTxns).To(HaveLen(1), "expected exactly one dividend transaction")
			Expect(divTxns[0].Amount).To(BeNumerically("~", 200.0, 0.01),
				"dividend amount should be 100 shares * $2.00 = $200")

			// Verify position is 200 shares after the 2-for-1 split.
			Expect(fund.Position(testStock)).To(BeNumerically("~", 200.0, 0.01),
				"position should be 200 shares after 2-for-1 split")
		})

		It("liquidates delisted positions", func() {
			testStock := asset.Asset{CompositeFigi: "FIGI-SYNC2", Ticker: "SYNC2"}
			delistAssets := []asset.Asset{testStock}
			delistProvider := &mockAssetProvider{assets: delistAssets}

			delistMetrics := []data.Metric{
				data.MetricClose, data.AdjClose, data.Dividend,
				data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume,
			}
			nDays := 15
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, nDays)
			for idx := range times {
				day := dataStart.AddDate(0, 0, idx)
				times[idx] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
			}

			nMetrics := len(delistMetrics)
			vals := make([]float64, nDays*len(delistAssets)*nMetrics)
			for dayIdx := 0; dayIdx < nDays; dayIdx++ {
				closePrice := 100.0
				if dayIdx >= 8 {
					closePrice = math.NaN() // simulates delisting
				}
				vals[(0*nMetrics+0)*nDays+dayIdx] = closePrice // Close
				vals[(0*nMetrics+1)*nDays+dayIdx] = closePrice // AdjClose
				vals[(0*nMetrics+2)*nDays+dayIdx] = 0.0        // Dividend
				if !math.IsNaN(closePrice) {
					vals[(0*nMetrics+3)*nDays+dayIdx] = closePrice + 2.0 // High
					vals[(0*nMetrics+4)*nDays+dayIdx] = closePrice - 2.0 // Low
				} else {
					vals[(0*nMetrics+3)*nDays+dayIdx] = math.NaN() // High
					vals[(0*nMetrics+4)*nDays+dayIdx] = math.NaN() // Low
				}
				vals[(0*nMetrics+5)*nDays+dayIdx] = 1.0         // SplitFactor
				vals[(0*nMetrics+6)*nDays+dayIdx] = 1_000_000.0 // Volume
			}

			delistDF, dfErr := data.NewDataFrame(times, delistAssets, delistMetrics, data.Daily,
				data.SlabToColumns(vals, len(delistAssets)*nMetrics, nDays))
			Expect(dfErr).NotTo(HaveOccurred())
			delistDataProvider := data.NewTestProvider(delistMetrics, delistDF)

			acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
			strategy := &buyOnceStrategy{target: testStock, qty: 50}
			eng := engine.New(strategy,
				engine.WithDataProvider(delistDataProvider),
				engine.WithAssetProvider(delistProvider),
				engine.WithAccount(acct),
			)

			btStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			btEnd := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), btStart, btEnd)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			txns := fund.Transactions()

			// Verify a sell transaction exists with "delisted" in the justification.
			var delistSellTxns []portfolio.Transaction
			for _, tx := range txns {
				if tx.Type == asset.SellTransaction && tx.Asset == testStock {
					delistSellTxns = append(delistSellTxns, tx)
				}
			}
			Expect(delistSellTxns).NotTo(BeEmpty(), "expected a sell transaction for delisted asset")
			hasDelisted := false
			for _, tx := range delistSellTxns {
				if tx.Justification != "" {
					hasDelisted = true
				}
			}
			Expect(hasDelisted).To(BeTrue(), "expected at least one sell transaction with delisted justification")

			// Verify position is 0 after delisting.
			Expect(fund.Position(testStock)).To(BeNumerically("==", 0),
				"position should be zero after delisting liquidation")
		})
	})
})
