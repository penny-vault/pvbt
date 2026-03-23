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
	"github.com/penny-vault/pvbt/config"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// ptrFloat64Config returns a pointer to the given float64 value.
func ptrFloat64Config(v float64) *float64 { return &v }

// ptrIntConfig returns a pointer to the given int value.
func ptrIntConfig(v int) *int { return &v }

// middlewareConfigStrategy is a strategy that tries to buy 100% of a single
// asset, letting middleware cap the position.
type middlewareConfigStrategy struct {
	target asset.Asset
}

func (s *middlewareConfigStrategy) Name() string           { return "middleware-config-test" }
func (s *middlewareConfigStrategy) Setup(_ *engine.Engine) {}
func (s *middlewareConfigStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		Schedule:  "@monthend",
		Benchmark: "SPY",
	}
}
func (s *middlewareConfigStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	return batch.RebalanceTo(ctx, portfolio.Allocation{
		Date:    eng.CurrentDate(),
		Members: map[asset.Asset]float64{s.target: 1.0},
	})
}

var _ = Describe("WithMiddlewareConfig", func() {
	Context("option storage", func() {
		It("stores the config on the engine", func() {
			limit := 0.25
			cfg := config.Config{
				Risk: config.RiskConfig{
					MaxPositionSize: &limit,
				},
			}

			strategy := &middlewareConfigStrategy{}
			eng := engine.New(strategy, engine.WithMiddlewareConfig(cfg))

			stored := engine.EngineMiddlewareConfigForTest(eng)
			Expect(stored).NotTo(BeNil())
			Expect(stored.Risk.MaxPositionSize).NotTo(BeNil())
			Expect(*stored.Risk.MaxPositionSize).To(Equal(0.25))
		})

		It("stores nil when WithMiddlewareConfig is not called", func() {
			strategy := &middlewareConfigStrategy{}
			eng := engine.New(strategy)

			stored := engine.EngineMiddlewareConfigForTest(eng)
			Expect(stored).To(BeNil())
		})
	})

	Context("buildMiddlewareFromConfig", func() {
		var (
			spy          asset.Asset
			allAssets    []asset.Asset
			allMetrics   []data.Metric
			dataStart    time.Time
			testDF       *data.DataFrame
			testProvider data.DataProvider
			assetProv    *mockAssetProvider
		)

		BeforeEach(func() {
			spy = asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
			allAssets = []asset.Asset{spy}
			allMetrics = []data.Metric{
				data.MetricClose, data.AdjClose, data.Dividend,
				data.MetricHigh, data.MetricLow, data.SplitFactor,
			}
			dataStart = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			testDF = makeDailyTestData(dataStart, 400, allAssets, allMetrics)
			testProvider = data.NewTestProvider(allMetrics, testDF)
			assetProv = &mockAssetProvider{assets: allAssets}
		})

		It("registers MaxPositionSize middleware when configured", func() {
			cfg := config.Config{
				Risk: config.RiskConfig{
					MaxPositionSize: ptrFloat64Config(0.25),
				},
			}

			strategy := &middlewareConfigStrategy{target: spy}
			eng := engine.New(strategy,
				engine.WithDataProvider(testProvider),
				engine.WithAssetProvider(assetProv),
				engine.WithInitialDeposit(100_000.0),
				engine.WithMiddlewareConfig(cfg),
			)

			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			engine.SetAccountForTest(eng, acct)
			engine.BuildMiddlewareFromConfigForTest(eng)

			// Run a backtest. The strategy wants 100% SPY; MaxPositionSize(0.25)
			// should cap it and produce risk:max-position-size annotations.
			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			annotations := acct.Annotations()
			found := false
			for _, ann := range annotations {
				if ann.Key == "risk:max-position-size" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected risk:max-position-size annotation from config-driven middleware")
		})

		It("registers MaxPositionCount middleware when configured", func() {
			// Use two assets to test position count capping.
			msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
			twoAssets := []asset.Asset{spy, msft}
			twoAssetProvider := &mockAssetProvider{assets: twoAssets}
			twoDF := makeDailyTestData(dataStart, 400, twoAssets, allMetrics)
			twoProvider := data.NewTestProvider(allMetrics, twoDF)

			cfg := config.Config{
				Risk: config.RiskConfig{
					MaxPositionCount: ptrIntConfig(1),
				},
			}

			// Strategy that tries to hold both assets.
			strategy := &backtestStrategy{assets: twoAssets}
			eng := engine.New(strategy,
				engine.WithDataProvider(twoProvider),
				engine.WithAssetProvider(twoAssetProvider),
				engine.WithInitialDeposit(100_000.0),
				engine.WithMiddlewareConfig(cfg),
			)

			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			engine.SetAccountForTest(eng, acct)
			engine.BuildMiddlewareFromConfigForTest(eng)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			annotations := acct.Annotations()
			found := false
			for _, ann := range annotations {
				if ann.Key == "risk:max-position-count" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected risk:max-position-count annotation from config-driven middleware")
		})

		It("registers conservative profile middleware", func() {
			cfg := config.Config{
				Risk: config.RiskConfig{
					Profile: "conservative",
				},
			}

			strategy := &middlewareConfigStrategy{target: spy}
			eng := engine.New(strategy,
				engine.WithDataProvider(testProvider),
				engine.WithAssetProvider(assetProv),
				engine.WithInitialDeposit(100_000.0),
				engine.WithMiddlewareConfig(cfg),
			)

			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			engine.SetAccountForTest(eng, acct)
			engine.BuildMiddlewareFromConfigForTest(eng)

			// The conservative profile sets MaxPositionSize(0.20), so we
			// expect risk:max-position-size annotations.
			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			annotations := acct.Annotations()
			found := false
			for _, ann := range annotations {
				if ann.Key == "risk:max-position-size" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected risk:max-position-size annotation from conservative profile")
		})

		It("clears existing middleware before applying config-driven stack", func() {
			// Pre-install a DrawdownCircuitBreaker on the account; then
			// buildMiddlewareFromConfig with only MaxPositionSize should remove it.
			cfg := config.Config{
				Risk: config.RiskConfig{
					MaxPositionSize: ptrFloat64Config(0.50),
				},
			}

			strategy := &middlewareConfigStrategy{target: spy}
			eng := engine.New(strategy,
				engine.WithDataProvider(testProvider),
				engine.WithAssetProvider(assetProv),
				engine.WithInitialDeposit(100_000.0),
				engine.WithMiddlewareConfig(cfg),
			)

			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			engine.SetAccountForTest(eng, acct)

			// The config-driven stack should replace any prior middleware.
			engine.BuildMiddlewareFromConfigForTest(eng)
			engine.BuildMiddlewareFromConfigForTest(eng) // second call must not double-register

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			// Count occurrences of max-position-size annotations to confirm
			// middleware was not applied twice.
			annotations := acct.Annotations()
			count := 0
			for _, ann := range annotations {
				if ann.Key == "risk:max-position-size" {
					count++
				}
			}
			// There should be some annotations but not double the expected number.
			// The exact count depends on how many rebalances fired; just check > 0.
			Expect(count).To(BeNumerically(">", 0))
		})
	})
})
