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
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
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

func (s *backtestStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("backtestStrategy.Setup: tradecron.New: %v", err))
	}
	eng.Schedule(tc)
}

func (s *backtestStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio) error {
	if len(s.assets) == 0 {
		return nil
	}
	priceDF, err := eng.FetchAt(ctx, s.assets, eng.CurrentDate(), []data.Metric{data.MetricClose})
	if err != nil || priceDF == nil {
		return nil
	}

	weight := 1.0 / float64(len(s.assets))
	totalValue := fund.Cash()
	fund.Holdings(func(held asset.Asset, qty float64) {
		price := priceDF.ValueAt(held, data.MetricClose, eng.CurrentDate())
		if !math.IsNaN(price) {
			totalValue += qty * price
		}
	})

	// Sell assets not in target.
	fund.Holdings(func(held asset.Asset, qty float64) {
		inTarget := false
		for _, target := range s.assets {
			if target == held {
				inTarget = true
				break
			}
		}
		if !inTarget && qty > 0 {
			fund.Order(ctx, held, portfolio.Sell, qty)
		}
	})

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
			fund.Order(ctx, target, portfolio.Buy, diff)
		} else if diff < 0 {
			fund.Order(ctx, target, portfolio.Sell, -diff)
		}
	}
	return nil
}

// noScheduleStrategy omits calling e.Schedule in Setup.
type noScheduleStrategy struct{}

func (s *noScheduleStrategy) Name() string { return "noSchedule" }
func (s *noScheduleStrategy) Setup(_ *engine.Engine) {}
func (s *noScheduleStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error {
	return nil
}

// failingStrategy always returns an error from Compute.
type failingStrategy struct{}

func (s *failingStrategy) Name() string { return "failing" }

func (s *failingStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("failingStrategy.Setup: tradecron.New: %v", err))
	}
	eng.Schedule(tc)
}

func (s *failingStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error {
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
	df, err := data.NewDataFrame(times, testAssets, metrics, data.Daily, vals)
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
		metrics = []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
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
				if tx.Type == portfolio.BuyTransaction {
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
})
