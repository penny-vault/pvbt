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

package study_test

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/montecarlo"
	"github.com/penny-vault/pvbt/study/stress"
)

// integrationAssetProvider implements data.AssetProvider for integration tests.
type integrationAssetProvider struct {
	assets []asset.Asset
}

func (provider *integrationAssetProvider) Assets(_ context.Context) ([]asset.Asset, error) {
	return provider.assets, nil
}

func (provider *integrationAssetProvider) LookupAsset(_ context.Context, ticker string) (asset.Asset, error) {
	for _, item := range provider.assets {
		if item.Ticker == ticker {
			return item, nil
		}
	}

	return asset.Asset{}, fmt.Errorf("asset not found: %s", ticker)
}

// buyAndHoldStrategy buys equal weight of all assets on the first Compute call
// and then holds for the remainder of the backtest.
type buyAndHoldStrategy struct {
	targetAssets []asset.Asset
	purchased    bool
}

func (strategy *buyAndHoldStrategy) Name() string { return "BuyAndHold" }

func (strategy *buyAndHoldStrategy) Setup(_ *engine.Engine) {}

func (strategy *buyAndHoldStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (strategy *buyAndHoldStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio, batch *portfolio.Batch) error {
	if strategy.purchased {
		return nil
	}

	strategy.purchased = true

	priceDF, fetchErr := eng.FetchAt(ctx, strategy.targetAssets, eng.CurrentDate(), []data.Metric{data.MetricClose})
	if fetchErr != nil || priceDF == nil {
		return fetchErr
	}

	weight := 1.0 / float64(len(strategy.targetAssets))
	totalCash := fund.Cash()

	for _, target := range strategy.targetAssets {
		price := priceDF.ValueAt(target, data.MetricClose, eng.CurrentDate())
		if math.IsNaN(price) || price <= 0 {
			continue
		}

		shares := math.Floor(weight * totalCash / price)
		if shares > 0 {
			batch.Order(ctx, target, portfolio.Buy, shares)
		}
	}

	return nil
}

// makeSyntheticDailyData creates a DataFrame with daily prices for the given
// assets spanning numDays starting at startDate. Prices increase linearly to
// ensure non-zero returns. The data includes all metrics required by the engine.
func makeSyntheticDailyData(startDate time.Time, numDays int, testAssets []asset.Asset, metrics []data.Metric) *data.DataFrame {
	times := make([]time.Time, numDays)
	for dayIdx := range times {
		day := startDate.AddDate(0, 0, dayIdx)
		times[dayIdx] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
	}

	numCols := len(testAssets) * len(metrics)
	vals := make([]float64, numDays*numCols)

	for assetIdx := range testAssets {
		for metricIdx, metric := range metrics {
			colStart := (assetIdx*len(metrics) + metricIdx) * numDays
			for dayIdx := 0; dayIdx < numDays; dayIdx++ {
				basePrice := 100.0 + float64(dayIdx)*0.5 + float64(assetIdx)*10.0

				switch metric {
				case data.SplitFactor:
					vals[colStart+dayIdx] = 1.0
				case data.Dividend:
					vals[colStart+dayIdx] = 0.0
				case data.MetricHigh:
					vals[colStart+dayIdx] = basePrice + 1.0
				case data.MetricLow:
					vals[colStart+dayIdx] = basePrice - 1.0
				default:
					// MetricClose and AdjClose get the same base price.
					vals[colStart+dayIdx] = basePrice
				}
			}
		}
	}

	columns := data.SlabToColumns(vals, numCols, numDays)
	dataFrame, err := data.NewDataFrame(times, testAssets, metrics, data.Daily, columns)
	Expect(err).NotTo(HaveOccurred())

	return dataFrame
}

// integrationStressStudy wraps stress.StressTest with custom short scenarios
// so data requirements stay small.
func integrationStressStudy() *stress.StressTest {
	scenarios := []study.Scenario{
		{
			Name:        "Short Drawdown",
			Description: "A short custom scenario for integration testing",
			Start:       time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			End:         time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	return stress.New(scenarios)
}

// customizableStudy wraps an existing Study and also implements EngineCustomizer
// so the runner integration test can verify EngineOptions is called per run.
type customizableStudy struct {
	study.Study
	callCount int
}

var _ study.EngineCustomizer = (*customizableStudy)(nil)

func (cs *customizableStudy) EngineOptions(_ study.RunConfig) []engine.Option {
	cs.callCount++
	return nil
}

var _ = Describe("Integration", func() {
	It("stress test satisfies the Study interface", func() {
		var iface study.Study = stress.New(nil)
		Expect(iface.Name()).To(Equal("Stress Test"))
	})

	It("stress test Configurations returns valid configs", func() {
		stressStudy := stress.New(nil)
		configs, err := stressStudy.Configurations(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(configs).To(HaveLen(1))
		Expect(configs[0].Start).NotTo(BeZero())
		Expect(configs[0].End).NotTo(BeZero())
		Expect(configs[0].End.After(configs[0].Start)).To(BeTrue())
	})

	It("runs a real strategy through the engine and stress analysis pipeline", func() {
		assetAlpha := asset.Asset{CompositeFigi: "FIGI-ALPHA", Ticker: "ALPHA"}
		assetBeta := asset.Asset{CompositeFigi: "FIGI-BETA", Ticker: "BETA"}
		testAssets := []asset.Asset{assetAlpha, assetBeta}

		metrics := []data.Metric{
			data.MetricClose,
			data.AdjClose,
			data.Dividend,
			data.MetricHigh,
			data.MetricLow,
			data.SplitFactor,
		}

		// Create synthetic daily data from 2024-01-01 through ~2024-04-30 (120 days).
		// This covers the custom stress scenario (Jan 15 - Mar 15).
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		syntheticData := makeSyntheticDailyData(dataStart, 120, testAssets, metrics)
		testProvider := data.NewTestProvider(metrics, syntheticData)
		assetProvider := &integrationAssetProvider{assets: testAssets}

		stressStudy := integrationStressStudy()

		runner := &study.Runner{
			Study: stressStudy,
			NewStrategy: func() engine.Strategy {
				return &buyAndHoldStrategy{targetAssets: testAssets}
			},
			Options: []engine.Option{
				engine.WithDataProvider(testProvider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			},
			Workers: 1,
		}

		progressCh, resultCh, runErr := runner.Run(context.Background())
		Expect(runErr).NotTo(HaveOccurred())
		Expect(progressCh).NotTo(BeNil())
		Expect(resultCh).NotTo(BeNil())

		// Drain progress channel and verify we see started+completed for one run.
		var progressEvents []study.Progress
		for event := range progressCh {
			progressEvents = append(progressEvents, event)
		}

		Expect(len(progressEvents)).To(BeNumerically(">=", 2), "expected at least started and completed progress events")

		// Read the final result.
		result := <-resultCh
		Expect(result.Err).NotTo(HaveOccurred())

		// Verify that the individual run succeeded.
		Expect(result.Runs).To(HaveLen(1))
		Expect(result.Runs[0].Err).NotTo(HaveOccurred())
		Expect(result.Runs[0].Portfolio).NotTo(BeNil())

		// Verify report has sections.
		Expect(result.Report.Title).To(Equal("Stress Test Analysis"))
		Expect(result.Report.Sections).NotTo(BeEmpty())

		// Render in text format.
		var textBuffer bytes.Buffer
		Expect(result.Report.Render(report.FormatText, &textBuffer)).To(Succeed())
		Expect(textBuffer.Len()).To(BeNumerically(">", 0))

		// Render in JSON format.
		var jsonBuffer bytes.Buffer
		Expect(result.Report.Render(report.FormatJSON, &jsonBuffer)).To(Succeed())
		Expect(jsonBuffer.String()).To(ContainSubstring(`"title"`))

		// Verify the stress analysis produced meaningful metrics.
		// The ranking table is the first section; find the scenario metrics section.
		foundScenarioSection := false
		for _, section := range result.Report.Sections {
			metricPairs, isMetricPairs := section.(*report.MetricPairs)
			if !isMetricPairs {
				continue
			}

			foundScenarioSection = true

			// There should be metrics for the single run.
			Expect(metricPairs.Metrics).NotTo(BeEmpty())

			for _, pair := range metricPairs.Metrics {
				if pair.Format == "percent" {
					// With rising synthetic prices, total return should be positive
					// and drawdown should be zero or negative. Either way, the value
					// should be a real number (not NaN).
					Expect(math.IsNaN(pair.Value)).To(BeFalse(),
						"expected non-NaN metric for %s", pair.Label)
				}
			}
		}

		Expect(foundScenarioSection).To(BeTrue(), "expected at least one MetricPairs scenario section")
	})

	It("runs a Monte Carlo simulation through the full runner pipeline", func() {
		assetAlpha := asset.Asset{CompositeFigi: "FIGI-ALPHA", Ticker: "ALPHA"}
		assetBeta := asset.Asset{CompositeFigi: "FIGI-BETA", Ticker: "BETA"}
		testAssets := []asset.Asset{assetAlpha, assetBeta}

		metrics := []data.Metric{
			data.MetricClose,
			data.AdjClose,
			data.Dividend,
			data.MetricHigh,
			data.MetricLow,
			data.SplitFactor,
		}

		// Create 60 days of synthetic daily data as the historical source for resampling.
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		numDays := 60
		syntheticData := makeSyntheticDailyData(dataStart, numDays, testAssets, metrics)
		assetProvider := &integrationAssetProvider{assets: testAssets}

		endDate := dataStart.AddDate(0, 0, numDays-1)

		mcStudy := montecarlo.New(syntheticData, metrics)
		mcStudy.Simulations = 5
		mcStudy.StartDate = dataStart
		mcStudy.EndDate = endDate
		mcStudy.InitialDeposit = 100_000.0

		runner := &study.Runner{
			Study: mcStudy,
			NewStrategy: func() engine.Strategy {
				return &buyAndHoldStrategy{targetAssets: testAssets}
			},
			Options: []engine.Option{
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			},
			Workers: 2,
		}

		progressCh, resultCh, runErr := runner.Run(context.Background())
		Expect(runErr).NotTo(HaveOccurred())
		Expect(progressCh).NotTo(BeNil())
		Expect(resultCh).NotTo(BeNil())

		// Drain progress channel.
		for range progressCh {
		}

		result := <-resultCh
		Expect(result.Err).NotTo(HaveOccurred())

		// All 5 runs should have succeeded with non-nil portfolios.
		Expect(result.Runs).To(HaveLen(5))

		for idx, run := range result.Runs {
			Expect(run.Err).NotTo(HaveOccurred(), "run %d had an error", idx)
			Expect(run.Portfolio).NotTo(BeNil(), "run %d has nil portfolio", idx)
		}

		// Verify report title and sections.
		Expect(result.Report.Title).To(Equal("Monte Carlo Simulation"))
		Expect(result.Report.Sections).NotTo(BeEmpty())

		// Render in text format.
		var textBuffer bytes.Buffer
		Expect(result.Report.Render(report.FormatText, &textBuffer)).To(Succeed())
		Expect(textBuffer.Len()).To(BeNumerically(">", 0))

		// Render in JSON format.
		var jsonBuffer bytes.Buffer
		Expect(result.Report.Render(report.FormatJSON, &jsonBuffer)).To(Succeed())
		Expect(jsonBuffer.String()).To(ContainSubstring(`"title"`))
	})

	It("calls EngineCustomizer.EngineOptions once per run config", func() {
		assetAlpha := asset.Asset{CompositeFigi: "FIGI-ALPHA", Ticker: "ALPHA"}
		assetBeta := asset.Asset{CompositeFigi: "FIGI-BETA", Ticker: "BETA"}
		testAssets := []asset.Asset{assetAlpha, assetBeta}

		metrics := []data.Metric{
			data.MetricClose,
			data.AdjClose,
			data.Dividend,
			data.MetricHigh,
			data.MetricLow,
			data.SplitFactor,
		}

		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		syntheticData := makeSyntheticDailyData(dataStart, 120, testAssets, metrics)
		testProvider := data.NewTestProvider(metrics, syntheticData)
		assetProvider := &integrationAssetProvider{assets: testAssets}

		inner := integrationStressStudy()
		wrapped := &customizableStudy{Study: inner}

		runner := &study.Runner{
			Study: wrapped,
			NewStrategy: func() engine.Strategy {
				return &buyAndHoldStrategy{targetAssets: testAssets}
			},
			Options: []engine.Option{
				engine.WithDataProvider(testProvider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			},
			Workers: 1,
		}

		progressCh, resultCh, runErr := runner.Run(context.Background())
		Expect(runErr).NotTo(HaveOccurred())

		// Drain progress channel.
		for range progressCh {
		}

		result := <-resultCh
		Expect(result.Err).NotTo(HaveOccurred())
		Expect(result.Runs).To(HaveLen(1))
		Expect(result.Runs[0].Err).NotTo(HaveOccurred())

		// integrationStressStudy has one scenario, so EngineOptions should be called once.
		Expect(wrapped.callCount).To(Equal(1))
	})
})
