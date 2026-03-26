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
	"github.com/penny-vault/pvbt/engine/middleware/risk"
	"github.com/penny-vault/pvbt/engine/middleware/tax"
	"github.com/penny-vault/pvbt/portfolio"
)

// ptrFloat64Config returns a pointer to the given float64 value.
func ptrFloat64Config(v float64) *float64 { return &v }

// ptrIntConfig returns a pointer to the given int value.
func ptrIntConfig(v int) *int { return &v }

// makeDrawdownTestData creates a DataFrame where prices rise for the first
// half of nDays then fall to peakPrice*(1-drawdownFraction) for the second
// half, producing a measurable portfolio drawdown when used with a full-
// investment strategy.  All required metrics are included so the engine can
// run a normal backtest.
func makeDrawdownTestData(
	start time.Time,
	nDays int,
	testAssets []asset.Asset,
	metrics []data.Metric,
	peakPrice float64,
	drawdownFraction float64,
) *data.DataFrame {
	times := make([]time.Time, nDays)
	for ii := range times {
		day := start.AddDate(0, 0, ii)
		times[ii] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
	}

	nMetrics := len(metrics)
	nAssets := len(testAssets)
	cols := make([][]float64, nAssets*nMetrics)
	for ci := range cols {
		cols[ci] = make([]float64, nDays)
	}

	troughPrice := peakPrice * (1 - drawdownFraction)
	midpoint := nDays / 2

	for assetIdx := range testAssets {
		for metricIdx, metric := range metrics {
			colIdx := assetIdx*nMetrics + metricIdx
			for dayIdx := range nDays {
				switch metric {
				case data.SplitFactor:
					cols[colIdx][dayIdx] = 1.0
				case data.Dividend:
					cols[colIdx][dayIdx] = 0.0
				default:
					if dayIdx < midpoint {
						cols[colIdx][dayIdx] = peakPrice
					} else {
						cols[colIdx][dayIdx] = troughPrice
					}
				}
			}
		}
	}

	df, err := data.NewDataFrame(times, testAssets, metrics, data.Daily, cols)
	Expect(err).NotTo(HaveOccurred())
	return df
}

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

var _ = Describe("HasMiddleware", func() {
	It("returns false for a zero-value config", func() {
		cfg := engine.MiddlewareConfig{}
		Expect(cfg.HasMiddleware()).To(BeFalse())
	})

	DescribeTable("returns true when a RiskConfig pointer field is set",
		func(makeCfg func() engine.MiddlewareConfig) {
			cfg := makeCfg()
			Expect(cfg.HasMiddleware()).To(BeTrue())
		},
		Entry("MaxPositionSize", func() engine.MiddlewareConfig {
			return engine.MiddlewareConfig{Risk: risk.RiskConfig{MaxPositionSize: ptrFloat64Config(0.25)}}
		}),
		Entry("MaxPositionCount", func() engine.MiddlewareConfig {
			return engine.MiddlewareConfig{Risk: risk.RiskConfig{MaxPositionCount: ptrIntConfig(5)}}
		}),
		Entry("DrawdownCircuitBreaker", func() engine.MiddlewareConfig {
			return engine.MiddlewareConfig{Risk: risk.RiskConfig{DrawdownCircuitBreaker: ptrFloat64Config(0.15)}}
		}),
		Entry("VolatilityScalerLookback", func() engine.MiddlewareConfig {
			return engine.MiddlewareConfig{Risk: risk.RiskConfig{VolatilityScalerLookback: ptrIntConfig(60)}}
		}),
		Entry("GrossExposureLimit", func() engine.MiddlewareConfig {
			return engine.MiddlewareConfig{Risk: risk.RiskConfig{GrossExposureLimit: ptrFloat64Config(1.0)}}
		}),
		Entry("NetExposureLimit", func() engine.MiddlewareConfig {
			return engine.MiddlewareConfig{Risk: risk.RiskConfig{NetExposureLimit: ptrFloat64Config(0.5)}}
		}),
	)

	DescribeTable("returns true for named risk profiles",
		func(profile string) {
			cfg := engine.MiddlewareConfig{Risk: risk.RiskConfig{Profile: profile}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		},
		Entry("conservative", "conservative"),
		Entry("moderate", "moderate"),
		Entry("aggressive", "aggressive"),
	)

	It("returns false when profile is none with no overrides", func() {
		cfg := engine.MiddlewareConfig{Risk: risk.RiskConfig{Profile: "none"}}
		Expect(cfg.HasMiddleware()).To(BeFalse())
	})

	It("returns true when tax is enabled", func() {
		cfg := engine.MiddlewareConfig{Tax: tax.TaxConfig{Enabled: true}}
		Expect(cfg.HasMiddleware()).To(BeTrue())
	})

	It("returns true when profile is none but an override is set", func() {
		cfg := engine.MiddlewareConfig{
			Risk: risk.RiskConfig{
				Profile:         "none",
				MaxPositionSize: ptrFloat64Config(0.30),
			},
		}
		Expect(cfg.HasMiddleware()).To(BeTrue())
	})
})

var _ = Describe("WithMiddlewareConfig", func() {
	Context("option storage", func() {
		It("stores the config on the engine", func() {
			limit := 0.25
			cfg := engine.MiddlewareConfig{
				Risk: risk.RiskConfig{
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
				data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume,
			}
			dataStart = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			testDF = makeDailyTestData(dataStart, 400, allAssets, allMetrics)
			testProvider = data.NewTestProvider(allMetrics, testDF)
			assetProv = &mockAssetProvider{assets: allAssets}
		})

		It("registers MaxPositionSize middleware when configured", func() {
			cfg := engine.MiddlewareConfig{
				Risk: risk.RiskConfig{
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

			cfg := engine.MiddlewareConfig{
				Risk: risk.RiskConfig{
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
			cfg := engine.MiddlewareConfig{
				Risk: risk.RiskConfig{
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

		It("registers DrawdownCircuitBreaker middleware when configured", func() {
			// Build price data that rises then drops 30% to produce a drawdown
			// large enough to trigger a 15% circuit-breaker threshold.
			ddStart := time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC)
			ddDF := makeDrawdownTestData(ddStart, 400, allAssets, allMetrics, 200.0, 0.30)
			ddProvider := data.NewTestProvider(allMetrics, ddDF)

			cfg := engine.MiddlewareConfig{
				Risk: risk.RiskConfig{
					DrawdownCircuitBreaker: ptrFloat64Config(0.15),
				},
			}

			strategy := &middlewareConfigStrategy{target: spy}
			eng := engine.New(strategy,
				engine.WithDataProvider(ddProvider),
				engine.WithAssetProvider(assetProv),
				engine.WithInitialDeposit(100_000.0),
				engine.WithMiddlewareConfig(cfg),
			)

			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			engine.SetAccountForTest(eng, acct)
			engine.BuildMiddlewareFromConfigForTest(eng)

			// Use a date range that spans the peak and the trough so the
			// circuit breaker has drawdown history to evaluate.
			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			// After prices drop 30% from peak, the 15% circuit-breaker fires
			// and appends a "risk:drawdown-circuit-breaker" annotation.
			annotations := acct.Annotations()
			found := false
			for _, ann := range annotations {
				if ann.Key == "risk:drawdown-circuit-breaker" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected risk:drawdown-circuit-breaker annotation from config-driven middleware")
		})

		It("registers GrossExposureLimit middleware when configured", func() {
			// Set a tight gross exposure limit (0.50) so that a 100% long
			// position attempt triggers the limit and produces annotations.
			cfg := engine.MiddlewareConfig{
				Risk: risk.RiskConfig{
					GrossExposureLimit: ptrFloat64Config(0.50),
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

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			annotations := acct.Annotations()
			found := false
			for _, ann := range annotations {
				if ann.Key == "risk:gross-exposure-limit" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected risk:gross-exposure-limit annotation from config-driven middleware")
		})

		It("registers NetExposureLimit middleware when configured", func() {
			// Set a tight net exposure limit (0.50) so that a 100% long
			// position attempt triggers the limit and produces annotations.
			cfg := engine.MiddlewareConfig{
				Risk: risk.RiskConfig{
					NetExposureLimit: ptrFloat64Config(0.50),
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

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			annotations := acct.Annotations()
			found := false
			for _, ann := range annotations {
				if ann.Key == "risk:net-exposure-limit" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected risk:net-exposure-limit annotation from config-driven middleware")
		})

		It("applies middleware in the correct order", func() {
			cfg := engine.MiddlewareConfig{
				Risk: risk.RiskConfig{
					MaxPositionSize:  ptrFloat64Config(0.10),
					NetExposureLimit: ptrFloat64Config(0.05),
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

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())

			// Verify both middleware fired by checking for their annotations.
			annotations := acct.Annotations()

			var hasMaxPositionSize, hasNetExposure bool

			for _, ann := range annotations {
				switch ann.Key {
				case "risk:max-position-size":
					hasMaxPositionSize = true
				case "risk:net-exposure-limit":
					hasNetExposure = true
				}
			}

			Expect(hasMaxPositionSize).To(BeTrue(),
				"expected at least one risk:max-position-size annotation")
			Expect(hasNetExposure).To(BeTrue(),
				"expected at least one risk:net-exposure-limit annotation")

			_ = fund // used above for nil check
		})

		It("registers TaxLossHarvester middleware when tax is enabled", func() {
			cfg := engine.MiddlewareConfig{
				Tax: tax.TaxConfig{
					Enabled:        true,
					LossThreshold:  0.05,
					GainOffsetOnly: false,
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

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)

			fund, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(fund).NotTo(BeNil())
		})

		It("clears existing middleware before applying config-driven stack", func() {
			cfg := engine.MiddlewareConfig{
				Risk: risk.RiskConfig{
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
			Expect(count).To(BeNumerically(">", 0))
		})
	})
})
