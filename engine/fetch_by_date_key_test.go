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
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

type fetchByDateKeyStrategy struct {
	assets          []asset.Asset
	metrics         []data.Metric
	dateKey         time.Time
	opts            []engine.FundamentalsByDateKeyOption
	fetched         *data.DataFrame
	fetchErr        error
	capturedCurrent time.Time
}

func (s *fetchByDateKeyStrategy) Name() string { return "fetchByDateKeyStrategy" }

func (s *fetchByDateKeyStrategy) Setup(*engine.Engine) {}

func (s *fetchByDateKeyStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *fetchByDateKeyStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	s.capturedCurrent = eng.CurrentDate()
	s.fetched, s.fetchErr = eng.FetchFundamentalsByDateKey(ctx, s.assets, s.metrics, s.dateKey, s.opts...)
	return nil
}

// fakeByDateKeyProvider is a small in-memory FundamentalsByDateKeyProvider.
type fakeByDateKeyProvider struct {
	*data.TestProvider
	rows         map[string]map[time.Time]map[data.Metric]float64 // figi -> dateKey -> metric -> value
	lastMaxEvent time.Time
}

func (p *fakeByDateKeyProvider) FetchFundamentalsByDateKey(
	_ context.Context,
	assets []asset.Asset,
	metrics []data.Metric,
	dateKey time.Time,
	_ string,
	maxEventDate time.Time,
) (*data.DataFrame, error) {
	p.lastMaxEvent = maxEventDate
	times := []time.Time{dateKey}
	columns := make([][]float64, len(assets)*len(metrics))

	for aIdx, aa := range assets {
		assetRows := p.rows[aa.CompositeFigi]
		for mIdx, mm := range metrics {
			val := math.NaN()
			if assetRows != nil {
				if dayMetrics, ok := assetRows[dateKey]; ok {
					if vv, ok := dayMetrics[mm]; ok {
						val = vv
					}
				}
			}

			columns[aIdx*len(metrics)+mIdx] = []float64{val}
		}
	}

	return data.NewDataFrame(times, assets, metrics, data.Daily, columns)
}

var _ = Describe("Engine.FetchFundamentalsByDateKey", func() {
	var assetProv *mockAssetProvider

	BeforeEach(func() {
		assetProv = &mockAssetProvider{}
	})

	It("returns the requested fundamentals at the date_key", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		testAssets := []asset.Asset{spy, msft}
		assetProv.assets = testAssets

		q1 := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 180, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		fundProvider := &fakeByDateKeyProvider{
			TestProvider: data.NewTestProvider(
				[]data.Metric{data.WorkingCapital, data.FundamentalsDateKey},
				mustEmptyFundDF(testAssets),
			),
			rows: map[string]map[time.Time]map[data.Metric]float64{
				spy.CompositeFigi: {
					q1: {
						data.WorkingCapital:      120_000_000,
						data.FundamentalsDateKey: float64(q1.Unix()),
					},
				},
				msft.CompositeFigi: {
					q1: {
						data.WorkingCapital:      80_000_000,
						data.FundamentalsDateKey: float64(q1.Unix()),
					},
				},
			},
		}

		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital, data.FundamentalsDateKey},
			dateKey: q1,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		simStart := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
		simEnd := time.Date(2024, 6, 17, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simStart, simEnd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).NotTo(HaveOccurred())

		spyWC := strategy.fetched.Column(spy, data.WorkingCapital)
		Expect(spyWC).To(HaveLen(1))
		Expect(spyWC[0]).To(BeNumerically("==", 120_000_000))

		msftWC := strategy.fetched.Column(msft, data.WorkingCapital)
		Expect(msftWC).To(HaveLen(1))
		Expect(msftWC[0]).To(BeNumerically("==", 80_000_000))

		spyDK := strategy.fetched.Column(spy, data.FundamentalsDateKey)
		Expect(time.Unix(int64(spyDK[0]), 0).UTC()).To(Equal(q1))
	})

	It("returns NaN for assets with no filing at the date_key", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		testAssets := []asset.Asset{spy, msft}
		assetProv.assets = testAssets

		q1 := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 180, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		fundProvider := &fakeByDateKeyProvider{
			TestProvider: data.NewTestProvider(
				[]data.Metric{data.WorkingCapital},
				mustEmptyFundDF(testAssets),
			),
			rows: map[string]map[time.Time]map[data.Metric]float64{
				spy.CompositeFigi: {
					q1: {data.WorkingCapital: 120_000_000},
				},
				// MSFT has no Q1 row.
			},
		}

		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital},
			dateKey: q1,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		simStart := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
		simEnd := time.Date(2024, 6, 17, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simStart, simEnd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).NotTo(HaveOccurred())

		msftWC := strategy.fetched.Column(msft, data.WorkingCapital)
		Expect(math.IsNaN(msftWC[0])).To(BeTrue())
	})

	It("errors when a non-fundamental metric is requested", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}
		assetProv.assets = testAssets

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 30, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		fundProvider := &fakeByDateKeyProvider{
			TestProvider: data.NewTestProvider(
				[]data.Metric{data.WorkingCapital},
				mustEmptyFundDF(testAssets),
			),
			rows: map[string]map[time.Time]map[data.Metric]float64{},
		}

		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital, data.MetricClose},
			dateKey: time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC),
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		// Jan 22 2024 is a Monday; 30-day closeDF covers through Jan 31.
		simStart := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
		simEnd := time.Date(2024, 1, 22, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simStart, simEnd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).To(HaveOccurred())
		Expect(strategy.fetchErr.Error()).To(ContainSubstring("not a fundamental metric"))
	})

	It("forwards e.CurrentDate() as the event-date cap when no option is set", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}
		assetProv.assets = testAssets

		q1 := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 180, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		fundProvider := &fakeByDateKeyProvider{
			TestProvider: data.NewTestProvider(
				[]data.Metric{data.WorkingCapital},
				mustEmptyFundDF(testAssets),
			),
			rows: map[string]map[time.Time]map[data.Metric]float64{
				spy.CompositeFigi: {q1: {data.WorkingCapital: 120_000_000}},
			},
		}

		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital},
			dateKey: q1,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		simStart := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
		simEnd := time.Date(2024, 6, 17, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simStart, simEnd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).NotTo(HaveOccurred())
		Expect(fundProvider.lastMaxEvent).To(Equal(strategy.capturedCurrent))
	})

	It("forwards the WithAsOfDate value as the event-date cap", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}
		assetProv.assets = testAssets

		q4 := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 180, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		fundProvider := &fakeByDateKeyProvider{
			TestProvider: data.NewTestProvider(
				[]data.Metric{data.WorkingCapital},
				mustEmptyFundDF(testAssets),
			),
			rows: map[string]map[time.Time]map[data.Metric]float64{
				spy.CompositeFigi: {q4: {data.WorkingCapital: 50_000_000}},
			},
		}

		formationDate := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)
		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital},
			dateKey: q4,
			opts:    []engine.FundamentalsByDateKeyOption{engine.WithAsOfDate(formationDate)},
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		simStart := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
		simEnd := time.Date(2024, 6, 17, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simStart, simEnd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).NotTo(HaveOccurred())
		Expect(fundProvider.lastMaxEvent).To(Equal(formationDate))
		Expect(strategy.capturedCurrent.After(formationDate)).To(BeTrue(),
			"test precondition: currentDate should be later than formationDate")
	})

	It("rejects WithAsOfDate later than CurrentDate() to prevent look-ahead", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}
		assetProv.assets = testAssets

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 180, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		fundProvider := &fakeByDateKeyProvider{
			TestProvider: data.NewTestProvider(
				[]data.Metric{data.WorkingCapital},
				mustEmptyFundDF(testAssets),
			),
			rows: map[string]map[time.Time]map[data.Metric]float64{},
		}

		future := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital},
			dateKey: time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC),
			opts:    []engine.FundamentalsByDateKeyOption{engine.WithAsOfDate(future)},
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		simStart := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
		simEnd := time.Date(2024, 6, 17, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simStart, simEnd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).To(HaveOccurred())
		Expect(strategy.fetchErr.Error()).To(ContainSubstring("as-of date"))
	})

	It("rejects WithAsOfDate with a zero time", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}
		assetProv.assets = testAssets

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 180, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		fundProvider := &fakeByDateKeyProvider{
			TestProvider: data.NewTestProvider(
				[]data.Metric{data.WorkingCapital},
				mustEmptyFundDF(testAssets),
			),
			rows: map[string]map[time.Time]map[data.Metric]float64{},
		}

		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital},
			dateKey: time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC),
			opts:    []engine.FundamentalsByDateKeyOption{engine.WithAsOfDate(time.Time{})},
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		simStart := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
		simEnd := time.Date(2024, 6, 17, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simStart, simEnd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).To(HaveOccurred())
		Expect(strategy.fetchErr.Error()).To(ContainSubstring("as-of date"))
	})

	It("errors when no provider supports FundamentalsByDateKey", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}
		assetProv.assets = testAssets

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 30, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)
		fundProvider := data.NewTestProvider(
			[]data.Metric{data.WorkingCapital},
			mustEmptyFundDF(testAssets),
		)

		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital},
			dateKey: time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC),
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		// Jan 22 2024 is a Monday; 30-day closeDF covers through Jan 31.
		simStart := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
		simEnd := time.Date(2024, 1, 22, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simStart, simEnd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).To(HaveOccurred())
		Expect(strategy.fetchErr.Error()).To(ContainSubstring("no provider supports"))
	})
})

// mustEmptyFundDF returns an empty fundamentals DataFrame for use as a
// TestProvider seed. Real values come from fakeByDateKeyProvider.rows.
func mustEmptyFundDF(assets []asset.Asset) *data.DataFrame {
	df, err := data.NewDataFrame(
		nil,
		assets,
		[]data.Metric{data.WorkingCapital, data.FundamentalsDateKey},
		data.Daily,
		nil,
	)
	if err != nil {
		panic(err)
	}

	return df
}
