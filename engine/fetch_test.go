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
})
