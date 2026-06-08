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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// nodataEntryStrategy buys both SPY (which has minute bars) and NODATA (which
// has only daily bars, no intraday coverage) on its single intraday firing.
// It implements no Reconciler, so the NODATA order simply fails.
type nodataEntryStrategy struct {
	mu       sync.Mutex
	spy      asset.Asset
	nodata   asset.Asset
	placed   bool
	schedule string
}

func (s *nodataEntryStrategy) Name() string           { return "nodataEntry" }
func (s *nodataEntryStrategy) Setup(_ *engine.Engine) {}

func (s *nodataEntryStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "nodata-entry", Schedule: s.schedule}
}

func (s *nodataEntryStrategy) Compute(
	_ context.Context,
	_ *engine.Engine,
	_ portfolio.Portfolio,
	batch *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.placed {
		return nil
	}

	s.placed = true

	if err := batch.Order(context.Background(), s.spy, portfolio.Buy, 10); err != nil {
		return err
	}

	return batch.Order(context.Background(), s.nodata, portfolio.Buy, 10)
}

// nodataReconcileStrategy buys NODATA on its firing, then -- when Reconcile
// reports that order failed -- substitutes SPY exactly once. It records the
// tickers it saw fail and how many times Reconcile was called.
type nodataReconcileStrategy struct {
	mu             sync.Mutex
	spy            asset.Asset
	nodata         asset.Asset
	placed         bool
	substituted    bool
	reconcileCalls int
	failedTickers  []string
	schedule       string
}

func (s *nodataReconcileStrategy) Name() string           { return "nodataReconcile" }
func (s *nodataReconcileStrategy) Setup(_ *engine.Engine) {}

func (s *nodataReconcileStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "nodata-reconcile", Schedule: s.schedule}
}

func (s *nodataReconcileStrategy) Compute(
	_ context.Context,
	_ *engine.Engine,
	_ portfolio.Portfolio,
	batch *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.placed {
		return nil
	}

	s.placed = true

	return batch.Order(context.Background(), s.nodata, portfolio.Buy, 10)
}

func (s *nodataReconcileStrategy) Reconcile(
	_ context.Context,
	_ *engine.Engine,
	_ portfolio.Portfolio,
	batch *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reconcileCalls++

	for _, outcome := range batch.FailedOrders() {
		s.failedTickers = append(s.failedTickers, outcome.Order.Asset.Ticker)
	}

	// Substitute the failed NODATA entry with SPY exactly once. A second
	// pass finds SPY already filled and appends nothing, so the batch settles.
	if !s.substituted {
		s.substituted = true

		return batch.Order(context.Background(), s.spy, portfolio.Buy, 10)
	}

	return nil
}

// runawayReconcileStrategy always appends another doomed NODATA order in
// Reconcile, so the batch never settles -- exercising the pass cap.
type runawayReconcileStrategy struct {
	mu       sync.Mutex
	nodata   asset.Asset
	placed   bool
	schedule string
}

func (s *runawayReconcileStrategy) Name() string           { return "runawayReconcile" }
func (s *runawayReconcileStrategy) Setup(_ *engine.Engine) {}

func (s *runawayReconcileStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "runaway-reconcile", Schedule: s.schedule}
}

func (s *runawayReconcileStrategy) Compute(
	_ context.Context,
	_ *engine.Engine,
	_ portfolio.Portfolio,
	batch *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.placed {
		return nil
	}

	s.placed = true

	return batch.Order(context.Background(), s.nodata, portfolio.Buy, 1)
}

func (s *runawayReconcileStrategy) Reconcile(
	_ context.Context,
	_ *engine.Engine,
	_ portfolio.Portfolio,
	batch *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return batch.Order(context.Background(), s.nodata, portfolio.Buy, 1)
}

var _ = Describe("intra-day orders with no minute bar", func() {
	var (
		nyc    *time.Location
		start  time.Time
		end    time.Time
		spy    asset.Asset
		nodata asset.Asset
	)

	// newEngine wires a backtest where SPY has minute bars but NODATA has
	// only daily bars, reproducing an asset (e.g. a ticker whose intraday
	// coverage starts later) that the strategy can select off daily prices
	// but cannot be filled intraday.
	newEngine := func(strategy engine.Strategy) *engine.Engine {
		dailyTimes := []time.Time{time.Date(2026, 5, 11, 16, 0, 0, 0, nyc)}

		dailyMetrics := []data.Metric{
			data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow,
			data.Volume, data.Dividend, data.SplitFactor,
		}

		dailyDF, dailyErr := data.NewDataFrame(dailyTimes,
			[]asset.Asset{spy, nodata},
			dailyMetrics,
			data.Daily,
			[][]float64{
				// SPY
				{100}, {100}, {102}, {99}, {1_000_000}, {0}, {1},
				// NODATA: daily bars exist, so the strategy can reference it.
				{50}, {50}, {52}, {49}, {1_000_000}, {0}, {1},
			})
		Expect(dailyErr).NotTo(HaveOccurred())

		dailyProvider := data.NewTestProvider(dailyMetrics, dailyDF)

		// Minute frame contains SPY only -- NODATA has no intraday coverage.
		minuteTimes := []time.Time{
			time.Date(2026, 5, 11, 10, 0, 0, 0, nyc),
			time.Date(2026, 5, 11, 10, 1, 0, 0, nyc),
		}

		minuteDF, minuteErr := data.NewDataFrame(minuteTimes,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.MetricHigh, data.MetricLow, data.Volume, data.AdjClose},
			data.Tick,
			[][]float64{
				{100.1, 100.2},
				{100.5, 100.6},
				{99.5, 99.6},
				{50_000, 50_000},
				{100.1, 100.2},
			})
		Expect(minuteErr).NotTo(HaveOccurred())

		intradayProvider := data.NewIntradayTestProvider(minuteDF)

		return engine.New(strategy,
			engine.WithAssetProvider(&mockAssetProvider{assets: []asset.Asset{spy, nodata}}),
			engine.WithDataProvider(dailyProvider),
			engine.WithDataProvider(intradayProvider),
			engine.WithInitialDeposit(10000),
		)
	}

	BeforeEach(func() {
		var locErr error

		nyc, locErr = time.LoadLocation("America/New_York")
		Expect(locErr).NotTo(HaveOccurred())

		start = time.Date(2026, 5, 11, 0, 0, 0, 0, nyc)
		end = time.Date(2026, 5, 11, 23, 59, 59, 0, nyc)

		spy = asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BDTBL9"}
		nodata = asset.Asset{Ticker: "NODATA", CompositeFigi: "BBG000NODATA0"}
	})

	It("fails the unpriced order instead of aborting the backtest", func() {
		strategy := &nodataEntryStrategy{spy: spy, nodata: nodata, schedule: "0 10 * * MON-FRI"}
		eng := newEngine(strategy)

		acct, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		// SPY filled; NODATA could not be priced intraday, so it did not fill.
		Expect(acct.Holdings()[spy]).To(BeNumerically("~", 10.0, 1e-9))
		Expect(acct.Holdings()).NotTo(HaveKey(nodata))
	})

	It("lets a Reconciler resubmit a failed order until the batch settles", func() {
		strategy := &nodataReconcileStrategy{spy: spy, nodata: nodata, schedule: "0 10 * * MON-FRI"}
		eng := newEngine(strategy)

		acct, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		// NODATA failed; the strategy substituted SPY in Reconcile and SPY filled.
		Expect(acct.Holdings()[spy]).To(BeNumerically("~", 10.0, 1e-9))
		Expect(acct.Holdings()).NotTo(HaveKey(nodata))

		strategy.mu.Lock()
		defer strategy.mu.Unlock()

		// Reconcile saw the NODATA failure, and ran twice: once to substitute
		// SPY, once more that found nothing to do and settled the batch.
		Expect(strategy.failedTickers).To(ContainElement("NODATA"))
		Expect(strategy.reconcileCalls).To(Equal(2))
	})

	It("surfaces an error when a Reconciler never settles the batch", func() {
		strategy := &runawayReconcileStrategy{nodata: nodata, schedule: "0 10 * * MON-FRI"}
		eng := newEngine(strategy)

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("did not settle"))
	})
})
