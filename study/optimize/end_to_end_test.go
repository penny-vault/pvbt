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

package optimize_test

import (
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
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/optimize"
	"github.com/penny-vault/pvbt/tradecron"
)

// e2eAssetProvider is a minimal AssetProvider for end-to-end tests.
type e2eAssetProvider struct {
	assets []asset.Asset
}

func (provider *e2eAssetProvider) Assets(_ context.Context) ([]asset.Asset, error) {
	return provider.assets, nil
}

func (provider *e2eAssetProvider) LookupAsset(_ context.Context, ticker string) (asset.Asset, error) {
	for _, item := range provider.assets {
		if item.Ticker == ticker {
			return item, nil
		}
	}

	return asset.Asset{}, fmt.Errorf("asset not found: %s", ticker)
}

// fixedFractionStrategy buys a fixed fraction of available assets on the
// first Compute call and holds. The Lookback parameter is exposed as an
// int parameter so the optimizer has something to sweep over, but it does
// not influence trading behavior beyond being applied via reflection. This
// keeps every combo's equity curve identical so we can isolate the
// optimizer/scoring path.
type fixedFractionStrategy struct {
	Targets  []asset.Asset
	Lookback int `default:"10"`

	purchased bool
}

func (strategy *fixedFractionStrategy) Name() string { return "FixedFraction" }

func (strategy *fixedFractionStrategy) Setup(_ *engine.Engine) {}

func (strategy *fixedFractionStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (strategy *fixedFractionStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio, batch *portfolio.Batch) error {
	if strategy.purchased {
		return nil
	}

	strategy.purchased = true

	if len(strategy.Targets) == 0 {
		return nil
	}

	priceDF, fetchErr := eng.FetchAt(ctx, strategy.Targets, eng.CurrentDate(), []data.Metric{data.MetricClose})
	if fetchErr != nil || priceDF == nil {
		return fetchErr
	}

	weight := 1.0 / float64(len(strategy.Targets))
	totalCash := fund.Cash()

	for _, target := range strategy.Targets {
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

// makeGrowingDailyData builds a synthetic price DataFrame whose AdjClose and
// MetricClose grow linearly from startPrice to endPrice over numDays.
// Returns include all metrics the engine needs.
func makeGrowingDailyData(startDate time.Time, numDays int, testAssets []asset.Asset, startPrice, endPrice float64) *data.DataFrame {
	metrics := []data.Metric{
		data.MetricClose,
		data.AdjClose,
		data.Dividend,
		data.MetricHigh,
		data.MetricLow,
		data.Volume,
		data.SplitFactor,
	}

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
			for dayIdx := range numDays {
				progress := float64(dayIdx) / float64(numDays-1)
				basePrice := startPrice + progress*(endPrice-startPrice)

				switch metric {
				case data.SplitFactor:
					vals[colStart+dayIdx] = 1.0
				case data.Dividend:
					vals[colStart+dayIdx] = 0.0
				case data.MetricHigh:
					vals[colStart+dayIdx] = basePrice + 0.5
				case data.MetricLow:
					vals[colStart+dayIdx] = basePrice - 0.5
				case data.Volume:
					vals[colStart+dayIdx] = 1_000_000
				default:
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

var _ = Describe("End-to-end optimize through the engine", func() {
	BeforeEach(func() {
		// Initialize an empty market-holidays table; the engine refuses to run
		// without one. Empty is fine for synthetic Mon-Fri data.
		tradecron.SetMarketHolidays(nil)
	})

	It("computes non-zero CAGR for each combo when the strategy actually trades", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}

		// 600 trading days of growing prices: 100 -> 200 (doubles), spanning
		// roughly 2.4 years. This is wide enough to comfortably bracket a
		// TrainTest cutoff.
		dataStart := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		syntheticData := makeGrowingDailyData(dataStart, 600, testAssets, 100.0, 200.0)
		testProvider := data.NewTestProvider(
			[]data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.Volume, data.SplitFactor},
			syntheticData,
		)
		assetProvider := &e2eAssetProvider{assets: testAssets}

		// TrainTest with a cutoff well inside the available data so both
		// halves have many trading days.
		start := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
		cutoff := time.Date(2021, 1, 4, 0, 0, 0, 0, time.UTC)
		end := time.Date(2022, 1, 3, 0, 0, 0, 0, time.UTC)

		splits, err := study.TrainTest(start, cutoff, end)
		Expect(err).NotTo(HaveOccurred())

		opt := optimize.New(splits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))

		runner := &study.Runner{
			Study: opt,
			NewStrategy: func() engine.Strategy {
				return &fixedFractionStrategy{Targets: testAssets}
			},
			Options: []engine.Option{
				engine.WithDataProvider(testProvider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			},
			Workers:        1,
			SearchStrategy: study.NewGrid(study.SweepValues("lookback", "5", "10", "20")),
			Splits:         splits,
			Objective:      portfolio.CAGR.(portfolio.Rankable),
		}

		progressCh, resultCh, runErr := runner.Run(context.Background())
		Expect(runErr).NotTo(HaveOccurred())

		for range progressCh {
		}

		result := <-resultCh
		Expect(result.Err).NotTo(HaveOccurred())

		for _, run := range result.Runs {
			Expect(run.Err).NotTo(HaveOccurred())
			Expect(run.Portfolio).NotTo(BeNil())
		}

		rptData := decodeOptReport(result.Report)
		Expect(rptData.Rankings).To(HaveLen(3),
			"expected one ranking row per combo (lookback=5,10,20)")

		// The strategy buys SPY on day 1 and holds. Prices grow linearly,
		// so CAGR over the test window must be strictly positive for every
		// combo. If any combo's MeanOOS is exactly zero, the metric path
		// is broken end-to-end.
		for idx, row := range rptData.Rankings {
			GinkgoWriter.Printf("rank=%d params=%q meanOOS=%g meanIS=%g\n",
				row.Rank, row.Parameters, row.MeanOOS, row.MeanIS)
			Expect(row.MeanOOS).NotTo(Equal(0.0),
				"row %d (params=%q) MeanOOS should not be zero — backtests grew from 100 to ~200, CAGR must be positive",
				idx, row.Parameters)
			Expect(row.MeanIS).NotTo(Equal(0.0),
				"row %d (params=%q) MeanIS should not be zero",
				idx, row.Parameters)
		}

		Expect(rptData.BestDetail).NotTo(BeNil())
		for _, fold := range rptData.BestDetail.Folds {
			Expect(fold.OOSScore).NotTo(Equal(0.0),
				"best detail fold %q OOSScore should not be zero", fold.FoldName)
			Expect(fold.ISScore).NotTo(Equal(0.0),
				"best detail fold %q ISScore should not be zero", fold.FoldName)
		}
	})

	It("fails loudly when no initial deposit is configured", func() {
		// Regression guard for the original bug: the `study optimize` CLI
		// previously did not pass engine.WithInitialDeposit, so the engine
		// constructed an account with $0 starting cash. The strategy could
		// not trade, equity stayed at $0 throughout the run, and CAGR /
		// Sharpe / Sortino / Calmar all silently returned 0 -- producing a
		// rankings list ordered by insertion. The engine now refuses to
		// run instead of producing meaningless zero scores.
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}

		dataStart := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		syntheticData := makeGrowingDailyData(dataStart, 600, testAssets, 100.0, 200.0)
		testProvider := data.NewTestProvider(
			[]data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow, data.Volume, data.SplitFactor},
			syntheticData,
		)
		assetProvider := &e2eAssetProvider{assets: testAssets}

		start := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
		cutoff := time.Date(2021, 1, 4, 0, 0, 0, 0, time.UTC)
		end := time.Date(2022, 1, 3, 0, 0, 0, 0, time.UTC)

		splits, err := study.TrainTest(start, cutoff, end)
		Expect(err).NotTo(HaveOccurred())

		opt := optimize.New(splits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))

		runner := &study.Runner{
			Study: opt,
			NewStrategy: func() engine.Strategy {
				return &fixedFractionStrategy{Targets: testAssets}
			},
			Options: []engine.Option{
				engine.WithDataProvider(testProvider),
				engine.WithAssetProvider(assetProvider),
				// Deliberately no engine.WithInitialDeposit.
			},
			Workers:        1,
			SearchStrategy: study.NewGrid(study.SweepValues("lookback", "5", "10", "20")),
			Splits:         splits,
			Objective:      portfolio.CAGR.(portfolio.Rankable),
		}

		progressCh, resultCh, runErr := runner.Run(context.Background())
		Expect(runErr).NotTo(HaveOccurred())

		for range progressCh {
		}

		result := <-resultCh
		Expect(result.Runs).NotTo(BeEmpty())

		for _, run := range result.Runs {
			Expect(run.Err).To(HaveOccurred(),
				"the engine should refuse to run with no initial deposit")
			Expect(run.Err.Error()).To(ContainSubstring("initial deposit must be positive"))
		}
	})
})
