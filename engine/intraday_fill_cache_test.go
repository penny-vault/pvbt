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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// intradayBasketStrategy buys one share of each asset in its basket on every
// scheduled firing. A multi-order firing routes one order-fill price lookup per
// asset through the intraday path, so it exercises the per-firing fill cache.
type intradayBasketStrategy struct {
	mu       sync.Mutex
	basket   []asset.Asset
	schedule string
}

func (s *intradayBasketStrategy) Name() string           { return "intradayBasket" }
func (s *intradayBasketStrategy) Setup(_ *engine.Engine) {}

func (s *intradayBasketStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "intraday-basket", Schedule: s.schedule}
}

func (s *intradayBasketStrategy) Compute(
	ctx context.Context,
	_ *engine.Engine,
	_ portfolio.Portfolio,
	batch *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, held := range s.basket {
		if err := batch.Order(ctx, held, portfolio.Buy, 1); err != nil {
			return err
		}
	}

	return nil
}

// intradayProbeStrategy queries Engine.Prices for assetX, then assetY, then
// assetX again on its single firing, placing no orders. It exercises the
// fill-window cache directly: the assetY lookup is a miss against a cache that
// holds only assetX, and the repeat assetX lookup must still be a hit.
type intradayProbeStrategy struct {
	mu       sync.Mutex
	assetX   asset.Asset
	assetY   asset.Asset
	schedule string
}

func (s *intradayProbeStrategy) Name() string           { return "intradayProbe" }
func (s *intradayProbeStrategy) Setup(_ *engine.Engine) {}

func (s *intradayProbeStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "intraday-probe", Schedule: s.schedule}
}

func (s *intradayProbeStrategy) Compute(
	ctx context.Context,
	eng *engine.Engine,
	_ portfolio.Portfolio,
	_ *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, want := range []asset.Asset{s.assetX, s.assetY, s.assetX} {
		if _, err := eng.Prices(ctx, want); err != nil {
			return err
		}
	}

	return nil
}

var _ = Describe("intra-day fill window caching", func() {
	var (
		nyc    *time.Location
		basket []asset.Asset
	)

	// buildEngine wires an engine for the given strategy whose intraday source
	// is a counting provider, over a basket of three assets that all trade at
	// 10:00 and 10:01 on both 2026-05-11 and 2026-05-12.
	buildEngine := func(strategy engine.Strategy) (*engine.Engine, *countingIntradayProvider) {
		dailyMetrics := []data.Metric{
			data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow,
			data.Volume, data.Dividend, data.SplitFactor,
		}

		dailyTimes := []time.Time{
			time.Date(2026, 5, 11, 16, 0, 0, 0, nyc),
			time.Date(2026, 5, 12, 16, 0, 0, 0, nyc),
		}

		// Three assets with distinct, easily-checked EOD closes.
		dailyCols := [][]float64{
			// AAA
			{10, 11}, {10, 11}, {12, 13}, {9, 10}, {1_000_000, 1_000_000}, {0, 0}, {1, 1},
			// BBB
			{20, 21}, {20, 21}, {22, 23}, {19, 20}, {1_000_000, 1_000_000}, {0, 0}, {1, 1},
			// CCC
			{30, 31}, {30, 31}, {32, 33}, {29, 30}, {1_000_000, 1_000_000}, {0, 0}, {1, 1},
		}

		dailyDF, dailyErr := data.NewDataFrame(dailyTimes, basket, dailyMetrics, data.Daily, dailyCols)
		Expect(dailyErr).NotTo(HaveOccurred())

		dailyProvider := data.NewTestProvider(dailyMetrics, dailyDF)

		minuteTimes := []time.Time{
			time.Date(2026, 5, 11, 10, 0, 0, 0, nyc),
			time.Date(2026, 5, 11, 10, 1, 0, 0, nyc),
			time.Date(2026, 5, 12, 10, 0, 0, 0, nyc),
			time.Date(2026, 5, 12, 10, 1, 0, 0, nyc),
		}

		minuteMetrics := []data.Metric{data.MetricClose, data.MetricHigh, data.MetricLow, data.Volume}

		// Per asset: close at 10:00 / 10:01 across the two days. The 10:01 bar
		// is the next-minute-bar that fills a 10:00 firing.
		minuteCols := [][]float64{
			// AAA close / high / low / volume
			{10.0, 10.05, 11.0, 11.05},
			{10.1, 10.15, 11.1, 11.15},
			{9.9, 9.95, 10.9, 10.95},
			{50_000, 50_000, 50_000, 50_000},
			// BBB
			{20.0, 20.05, 21.0, 21.05},
			{20.1, 20.15, 21.1, 21.15},
			{19.9, 19.95, 20.9, 20.95},
			{50_000, 50_000, 50_000, 50_000},
			// CCC
			{30.0, 30.05, 31.0, 31.05},
			{30.1, 30.15, 31.1, 31.15},
			{29.9, 29.95, 30.9, 30.95},
			{50_000, 50_000, 50_000, 50_000},
		}

		minuteDF, minuteErr := data.NewDataFrame(minuteTimes, basket, minuteMetrics, data.Tick, minuteCols)
		Expect(minuteErr).NotTo(HaveOccurred())

		intraday := &countingIntradayProvider{IntradayTestProvider: data.NewIntradayTestProvider(minuteDF)}

		eng := engine.New(strategy,
			engine.WithAssetProvider(&mockAssetProvider{assets: basket}),
			engine.WithDataProvider(dailyProvider),
			engine.WithDataProvider(intraday),
			engine.WithInitialDeposit(10000),
		)

		return eng, intraday
	}

	BeforeEach(func() {
		var locErr error

		nyc, locErr = time.LoadLocation("America/New_York")
		Expect(locErr).NotTo(HaveOccurred())

		basket = []asset.Asset{
			{Ticker: "AAA", CompositeFigi: "BBG000000001"},
			{Ticker: "BBB", CompositeFigi: "BBG000000002"},
			{Ticker: "CCC", CompositeFigi: "BBG000000003"},
		}
	})

	It("serves every per-order fill in a firing from a single IntradayFetch", func() {
		// One trading day, one 10:00 firing buying all three assets. The
		// prefetch issues one IntradayFetch; the three per-order Submit price
		// lookups are cache hits. No prior holdings means no mark-bar fetch.
		start := time.Date(2026, 5, 11, 0, 0, 0, 0, nyc)
		end := time.Date(2026, 5, 11, 23, 59, 59, 0, nyc)

		eng, intraday := buildEngine(&intradayBasketStrategy{basket: basket, schedule: "0 10 * * MON-FRI"})

		fund, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		Expect(intraday.callCount()).To(Equal(1),
			"a 3-order firing must fetch the next-minute-bar window once, not once per order")

		// Each order fills at its asset's 10:01 close, proving the cached
		// window serves correct per-asset bars.
		fills := map[string]float64{}
		for _, txn := range fund.Transactions() {
			if txn.Type == asset.BuyTransaction {
				fills[txn.Asset.Ticker] = txn.Price
			}
		}

		Expect(fills["AAA"]).To(BeNumerically("~", 10.05, 1e-9))
		Expect(fills["BBB"]).To(BeNumerically("~", 20.05, 1e-9))
		Expect(fills["CCC"]).To(BeNumerically("~", 30.05, 1e-9))
	})

	It("re-fetches the window on the next firing rather than serving a stale one", func() {
		// Two trading days, one 10:00 firing each buying all three assets.
		// Day 1: no holdings, so prefetch issues 1 fetch and the 3 fills hit
		// the cache. Day 2: holdings exist, so mark-bar issues 1 fetch; the
		// cache from day 1 is invalid at the new firing time, so the prefetch
		// issues a fresh fetch and the 3 fills hit it. Total: 1 + 2 = 3.
		// A stale cache that survived across firings would total 2; no caching
		// at all would total 9.
		start := time.Date(2026, 5, 11, 0, 0, 0, 0, nyc)
		end := time.Date(2026, 5, 12, 23, 59, 59, 0, nyc)

		eng, intraday := buildEngine(&intradayBasketStrategy{basket: basket, schedule: "0 10 * * MON-FRI"})

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		Expect(intraday.callCount()).To(Equal(3))
	})

	It("fills each asset at its own earliest bar when bars are staggered in the window", func() {
		// The riskiest equivalence case: within the shared next-minute-bar
		// window the three assets first trade at different minutes. A direct
		// per-asset fetch builds its time axis from only that asset's bars, so
		// each fills at its own first bar. Serving from a single multi-asset
		// window (which carries the union time axis) must reproduce that: AAA
		// and BBB fill at 10:01, while CCC -- absent at 10:01 -- must fill at
		// its 10:03 bar, not be valued NaN at 10:01 and dropped.
		nan := math.NaN()

		start := time.Date(2026, 5, 11, 0, 0, 0, 0, nyc)
		end := time.Date(2026, 5, 11, 23, 59, 59, 0, nyc)

		dailyMetrics := []data.Metric{
			data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow,
			data.Volume, data.Dividend, data.SplitFactor,
		}

		dailyTimes := []time.Time{time.Date(2026, 5, 11, 16, 0, 0, 0, nyc)}

		dailyDF, dailyErr := data.NewDataFrame(dailyTimes, basket, dailyMetrics, data.Daily,
			[][]float64{
				{10}, {10}, {12}, {9}, {1_000_000}, {0}, {1}, // AAA
				{20}, {20}, {22}, {19}, {1_000_000}, {0}, {1}, // BBB
				{30}, {30}, {32}, {29}, {1_000_000}, {0}, {1}, // CCC
			})
		Expect(dailyErr).NotTo(HaveOccurred())

		dailyProvider := data.NewTestProvider(dailyMetrics, dailyDF)

		// Union time axis [10:00, 10:01, 10:03]. CCC has no 10:01 bar; AAA and
		// BBB have no 10:03 bar.
		minuteTimes := []time.Time{
			time.Date(2026, 5, 11, 10, 0, 0, 0, nyc),
			time.Date(2026, 5, 11, 10, 1, 0, 0, nyc),
			time.Date(2026, 5, 11, 10, 3, 0, 0, nyc),
		}

		minuteMetrics := []data.Metric{data.MetricClose, data.MetricHigh, data.MetricLow, data.Volume}

		minuteDF, minuteErr := data.NewDataFrame(minuteTimes, basket, minuteMetrics, data.Tick,
			[][]float64{
				// AAA close / high / low / volume -- no 10:03 bar
				{10.0, 10.05, nan},
				{10.0, 10.06, nan},
				{10.0, 9.95, nan},
				{50_000, 50_000, nan},
				// BBB -- no 10:03 bar
				{20.0, 20.05, nan},
				{20.0, 20.06, nan},
				{20.0, 19.95, nan},
				{50_000, 50_000, nan},
				// CCC -- no 10:01 bar, first window bar at 10:03
				{30.0, nan, 30.3},
				{30.0, nan, 30.4},
				{30.0, nan, 30.2},
				{50_000, nan, 50_000},
			})
		Expect(minuteErr).NotTo(HaveOccurred())

		intraday := &countingIntradayProvider{IntradayTestProvider: data.NewIntradayTestProvider(minuteDF)}

		strategy := &intradayBasketStrategy{basket: basket, schedule: "0 10 * * MON-FRI"}

		eng := engine.New(strategy,
			engine.WithAssetProvider(&mockAssetProvider{assets: basket}),
			engine.WithDataProvider(dailyProvider),
			engine.WithDataProvider(intraday),
			engine.WithInitialDeposit(10000),
		)

		fund, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		Expect(intraday.callCount()).To(Equal(1))

		fills := map[string]float64{}
		for _, txn := range fund.Transactions() {
			if txn.Type == asset.BuyTransaction {
				fills[txn.Asset.Ticker] = txn.Price
			}
		}

		Expect(fills["AAA"]).To(BeNumerically("~", 10.05, 1e-9))
		Expect(fills["BBB"]).To(BeNumerically("~", 20.05, 1e-9))
		Expect(fills).To(HaveKey("CCC"), "CCC must fill from its 10:03 bar, not be dropped for lacking a 10:01 bar")
		Expect(fills["CCC"]).To(BeNumerically("~", 30.3, 1e-9))
	})

	It("widens the cached window on a miss rather than evicting earlier assets", func() {
		// One firing queries Prices for AAA, then BBB, then AAA again. AAA is a
		// cold miss (1 fetch). BBB is not covered by the AAA-only cache, so the
		// window widens to {AAA, BBB} (1 fetch). The repeat AAA query must hit
		// the widened window. Total: 2 fetches. A cache that replaced rather
		// than widened would evict AAA on the BBB miss, making the repeat AAA
		// query a third fetch.
		start := time.Date(2026, 5, 11, 0, 0, 0, 0, nyc)
		end := time.Date(2026, 5, 11, 23, 59, 59, 0, nyc)

		probe := &intradayProbeStrategy{assetX: basket[0], assetY: basket[1], schedule: "0 10 * * MON-FRI"}

		eng, intraday := buildEngine(probe)

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		Expect(intraday.callCount()).To(Equal(2))
	})
})
