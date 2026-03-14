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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
)

// fetchStrategy captures data fetched during Compute for test assertions.
type fetchStrategy struct {
	lookback portfolio.Period
	metrics  []data.Metric
	assets   []asset.Asset
	fetched  *data.DataFrame
	fetchErr error
}

func (s *fetchStrategy) Name() string { return "fetchStrategy" }

func (s *fetchStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("fetchStrategy.Setup: %v", err))
	}
	eng.Schedule(tc)
}

func (s *fetchStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio) {
	s.fetched, s.fetchErr = eng.Fetch(ctx, s.assets, s.lookback, s.metrics)
}

// fetchAtStrategy calls FetchAt during Compute.
type fetchAtStrategy struct {
	metrics  []data.Metric
	assets   []asset.Asset
	fetched  *data.DataFrame
	fetchErr error
}

func (s *fetchAtStrategy) Name() string { return "fetchAtStrategy" }

func (s *fetchAtStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("fetchAtStrategy.Setup: %v", err))
	}
	eng.Schedule(tc)
}

func (s *fetchAtStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio) {
	s.fetched, s.fetchErr = eng.FetchAt(ctx, s.assets, eng.CurrentDate(), s.metrics)
}

// doubleFetchStrategy calls Fetch twice per Compute with overlapping data.
type doubleFetchStrategy struct {
	assets1, assets2     []asset.Asset
	lookback             portfolio.Period
	metrics              []data.Metric
	fetched1, fetched2   *data.DataFrame
	fetchErr1, fetchErr2 error
}

func (s *doubleFetchStrategy) Name() string { return "doubleFetchStrategy" }

func (s *doubleFetchStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("doubleFetchStrategy.Setup: %v", err))
	}
	eng.Schedule(tc)
}

func (s *doubleFetchStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio) {
	s.fetched1, s.fetchErr1 = eng.Fetch(ctx, s.assets1, s.lookback, s.metrics)
	s.fetched2, s.fetchErr2 = eng.Fetch(ctx, s.assets2, s.lookback, s.metrics)
}

// fetchThenFetchAtStrategy calls Fetch then FetchAt in the same Compute.
type fetchThenFetchAtStrategy struct {
	assets        []asset.Asset
	lookback      portfolio.Period
	metrics       []data.Metric
	fetchAtResult *data.DataFrame
	fetchAtErr    error
}

func (s *fetchThenFetchAtStrategy) Name() string { return "fetchThenFetchAtStrategy" }

func (s *fetchThenFetchAtStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("fetchThenFetchAtStrategy.Setup: %v", err))
	}
	eng.Schedule(tc)
}

func (s *fetchThenFetchAtStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio) {
	// First call populates the cache.
	_, _ = eng.Fetch(ctx, s.assets, s.lookback, s.metrics)
	// Second call should hit the cache.
	s.fetchAtResult, s.fetchAtErr = eng.FetchAt(ctx, s.assets, eng.CurrentDate(), s.metrics)
}

// futureFetchAtStrategy calls FetchAt with a future date during Compute.
type futureFetchAtStrategy struct {
	metrics  []data.Metric
	assets   []asset.Asset
	fetched  *data.DataFrame
	fetchErr error
}

func (s *futureFetchAtStrategy) Name() string { return "futureFetchAtStrategy" }

func (s *futureFetchAtStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("futureFetchAtStrategy.Setup: %v", err))
	}
	eng.Schedule(tc)
}

func (s *futureFetchAtStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio) {
	futureDate := eng.CurrentDate().AddDate(0, 0, 30)
	s.fetched, s.fetchErr = eng.FetchAt(ctx, s.assets, futureDate, s.metrics)
}

// countingProvider wraps a TestProvider and counts Fetch calls.
type countingProvider struct {
	data.DataProvider
	inner     *data.TestProvider
	fetchCount int
}

func newCountingProvider(metrics []data.Metric, df *data.DataFrame) *countingProvider {
	inner := data.NewTestProvider(metrics, df)
	return &countingProvider{DataProvider: inner, inner: inner}
}

func (cp *countingProvider) Fetch(ctx context.Context, req data.DataRequest) (*data.DataFrame, error) {
	cp.fetchCount++
	return cp.inner.Fetch(ctx, req)
}

// makeDailyDF creates a DataFrame with daily timestamps at 16:00 UTC.
func makeDailyDF(start time.Time, nDays int, testAssets []asset.Asset, metrics []data.Metric) *data.DataFrame {
	times := make([]time.Time, nDays)
	for i := range times {
		day := start.AddDate(0, 0, i)
		times[i] = time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, time.UTC)
	}
	vals := make([]float64, nDays*len(testAssets)*len(metrics))
	for i := range vals {
		vals[i] = float64(i + 1)
	}
	df, err := data.NewDataFrame(times, testAssets, metrics, vals)
	Expect(err).NotTo(HaveOccurred())
	return df
}

var _ = Describe("Fetch", func() {
	var (
		aapl          asset.Asset
		testAssets    []asset.Asset
		assetProvider *mockAssetProvider
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		testAssets = []asset.Asset{aapl}
		assetProvider = &mockAssetProvider{assets: testAssets}
	})

	Context("with a lookback period", func() {
		It("returns data for the requested window", func() {
			metrics := []data.Metric{data.MetricClose}
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyDF(dataStart, 90, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &fetchStrategy{
				lookback: portfolio.Days(10),
				metrics:  metrics,
				assets:   testAssets,
			}

			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 5, 23, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.fetchErr).NotTo(HaveOccurred())
			Expect(strategy.fetched).NotTo(BeNil())
			Expect(strategy.fetched.Len()).To(BeNumerically(">", 0))
		})
	})

	Context("with multiple providers", func() {
		It("merges metrics from different providers", func() {
			goog := asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
			multiAssets := []asset.Asset{aapl, goog}
			assetProvider = &mockAssetProvider{assets: multiAssets}

			closeMetrics := []data.Metric{data.MetricClose}
			volumeMetrics := []data.Metric{data.Volume}

			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			closeDF := makeDailyDF(dataStart, 90, multiAssets, closeMetrics)
			volumeDF := makeDailyDF(dataStart, 90, multiAssets, volumeMetrics)

			closeProvider := data.NewTestProvider(closeMetrics, closeDF)
			volumeProvider := data.NewTestProvider(volumeMetrics, volumeDF)

			allMetrics := []data.Metric{data.MetricClose, data.Volume}
			strategy := &fetchStrategy{
				lookback: portfolio.Days(5),
				metrics:  allMetrics,
				assets:   multiAssets,
			}

			eng := engine.New(strategy,
				engine.WithDataProvider(closeProvider, volumeProvider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 5, 23, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.fetchErr).NotTo(HaveOccurred())
			Expect(strategy.fetched).NotTo(BeNil())

			metricList := strategy.fetched.MetricList()
			hasClose := false
			hasVolume := false
			for _, metric := range metricList {
				if metric == data.MetricClose {
					hasClose = true
				}
				if metric == data.Volume {
					hasVolume = true
				}
			}
			Expect(hasClose).To(BeTrue(), "merged result should contain MetricClose")
			Expect(hasVolume).To(BeTrue(), "merged result should contain Volume")
		})
	})

	Context("FetchAt", func() {
		It("returns data for a single point in time", func() {
			metrics := []data.Metric{data.MetricClose}
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyDF(dataStart, 90, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			strategy := &fetchAtStrategy{
				metrics: metrics,
				assets:  testAssets,
			}

			eng := engine.New(strategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 5, 23, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.fetchErr).NotTo(HaveOccurred())
			Expect(strategy.fetched).NotTo(BeNil())
			Expect(strategy.fetched.Len()).To(Equal(1))
		})
	})

	Context("cache reuse", func() {
		It("does not re-fetch data already in the cache", func() {
			metrics := []data.Metric{data.MetricClose}
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyDF(dataStart, 90, testAssets, metrics)
			cp := newCountingProvider(metrics, df)

			strategy := &doubleFetchStrategy{
				assets1:  testAssets,
				assets2:  testAssets[:1],
				lookback: portfolio.Days(5),
				metrics:  metrics,
			}

			eng := engine.New(strategy,
				engine.WithDataProvider(cp),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 3, 23, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.fetchErr1).NotTo(HaveOccurred())
			Expect(strategy.fetchErr2).NotTo(HaveOccurred())

			Expect(cp.fetchCount).To(BeNumerically("<=", 3),
				"expected cache reuse to reduce provider calls")
		})
	})

	Context("FetchAt cache", func() {
		It("does not call the provider when data is already cached", func() {
			metrics := []data.Metric{data.MetricClose}
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyDF(dataStart, 90, testAssets, metrics)
			cp := newCountingProvider(metrics, df)

			strategy := &fetchThenFetchAtStrategy{
				assets:   testAssets,
				lookback: portfolio.Days(5),
				metrics:  metrics,
			}

			eng := engine.New(strategy,
				engine.WithDataProvider(cp),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 3, 23, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.fetchAtErr).NotTo(HaveOccurred())
			Expect(strategy.fetchAtResult).NotTo(BeNil())
			Expect(strategy.fetchAtResult.Len()).To(Equal(1))

			Expect(cp.fetchCount).To(BeNumerically("<=", 2),
				"FetchAt should use the cache after Fetch populates it")
		})
	})

	Context("look-ahead guard", func() {
		It("FetchAt rejects a future date", func() {
			metrics := []data.Metric{data.MetricClose}
			dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyDF(dataStart, 90, testAssets, metrics)
			provider := data.NewTestProvider(metrics, df)

			futureStrategy := &futureFetchAtStrategy{
				metrics: metrics,
				assets:  testAssets,
			}

			eng := engine.New(futureStrategy,
				engine.WithDataProvider(provider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 2, 3, 23, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(futureStrategy.fetchErr).To(HaveOccurred())
			Expect(futureStrategy.fetchErr.Error()).To(ContainSubstring("future"))
		})
	})

	Context("cross-year fetch", func() {
		It("fetches and assembles data spanning two calendar years", func() {
			metrics := []data.Metric{data.MetricClose}
			dataStart := time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC)
			df := makeDailyDF(dataStart, 90, testAssets, metrics)
			cp := newCountingProvider(metrics, df)

			strategy := &fetchStrategy{
				lookback: portfolio.Days(30),
				metrics:  metrics,
				assets:   testAssets,
			}

			eng := engine.New(strategy,
				engine.WithDataProvider(cp),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			start := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 1, 17, 23, 0, 0, 0, time.UTC)

			_, err := eng.Backtest(context.Background(), start, end)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.fetchErr).NotTo(HaveOccurred())
			Expect(strategy.fetched).NotTo(BeNil())
			Expect(strategy.fetched.Len()).To(BeNumerically(">", 10))
		})
	})
})
