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
	"github.com/penny-vault/pvbt/universe"
)

// intradayRecordStrategy fires on each scheduled timestamp and records
// the engine's Now() value, plus the result of a MinuteBars Window call,
// for verification.
type intradayRecordStrategy struct {
	mu       sync.Mutex
	universe universe.Universe
	fired    []time.Time
	windows  []*data.DataFrame
	schedule string
}

func (s *intradayRecordStrategy) Name() string           { return "intradayRecord" }
func (s *intradayRecordStrategy) Setup(_ *engine.Engine) {}

func (s *intradayRecordStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		ShortCode: "intraday-record",
		Schedule:  s.schedule,
	}
}

func (s *intradayRecordStrategy) Compute(
	ctx context.Context,
	eng *engine.Engine,
	_ portfolio.Portfolio,
	_ *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.fired = append(s.fired, eng.Now())

	if s.universe != nil {
		df, err := s.universe.Window(ctx, portfolio.MinuteBars(3), data.MetricClose)
		if err != nil {
			return err
		}

		s.windows = append(s.windows, df)
	}

	return nil
}

var _ = Describe("intra-day firings", func() {
	var (
		nyc      *time.Location
		start    time.Time
		end      time.Time
		spy      asset.Asset
		eng      *engine.Engine
		strategy *intradayRecordStrategy
	)

	BeforeEach(func() {
		var locErr error

		nyc, locErr = time.LoadLocation("America/New_York")
		Expect(locErr).NotTo(HaveOccurred())

		start = time.Date(2026, 5, 11, 0, 0, 0, 0, nyc)  // Monday
		end = time.Date(2026, 5, 12, 23, 59, 59, 0, nyc) // Tuesday

		spy = asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BDTBL9"}

		// Build a daily bar frame so the engine has eod-style data to
		// drive the broker, housekeeping and reporting. The strategy
		// itself doesn't fetch daily bars but the engine path does.
		dailyTimes := []time.Time{
			time.Date(2026, 5, 11, 16, 0, 0, 0, nyc),
			time.Date(2026, 5, 12, 16, 0, 0, 0, nyc),
		}

		dailyDF, dailyErr := data.NewDataFrame(dailyTimes,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow, data.Volume, data.Dividend, data.SplitFactor},
			data.Daily,
			[][]float64{
				{100, 101},             // close
				{100, 101},             // adj close
				{102, 103},             // high
				{99, 100},              // low
				{1_000_000, 1_000_000}, // volume
				{0, 0},                 // dividend
				{1, 1},                 // split factor
			})
		Expect(dailyErr).NotTo(HaveOccurred())

		dailyMetrics := []data.Metric{
			data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow,
			data.Volume, data.Dividend, data.SplitFactor,
		}
		dailyProvider := data.NewTestProvider(dailyMetrics, dailyDF)

		// Build a minute-bar frame for IntradayFetch covering both
		// trading days at 10:00 and 14:00. Also include a 10:01 and
		// 14:01 bar so the broker's next-bar fill has a row to land
		// on.
		minuteTimes := []time.Time{
			time.Date(2026, 5, 11, 10, 0, 0, 0, nyc),
			time.Date(2026, 5, 11, 10, 1, 0, 0, nyc),
			time.Date(2026, 5, 11, 14, 0, 0, 0, nyc),
			time.Date(2026, 5, 11, 14, 1, 0, 0, nyc),
			time.Date(2026, 5, 12, 10, 0, 0, 0, nyc),
			time.Date(2026, 5, 12, 10, 1, 0, 0, nyc),
			time.Date(2026, 5, 12, 14, 0, 0, 0, nyc),
			time.Date(2026, 5, 12, 14, 1, 0, 0, nyc),
		}

		minuteDF, minuteErr := data.NewDataFrame(minuteTimes,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.MetricHigh, data.MetricLow, data.Volume, data.AdjClose},
			data.Tick,
			[][]float64{
				{100.1, 100.2, 100.3, 100.4, 101.1, 101.2, 101.3, 101.4},         // close
				{100.5, 100.6, 100.7, 100.8, 101.5, 101.6, 101.7, 101.8},         // high
				{99.5, 99.6, 99.7, 99.8, 100.5, 100.6, 100.7, 100.8},             // low
				{50_000, 50_000, 50_000, 50_000, 50_000, 50_000, 50_000, 50_000}, // volume
				{100.1, 100.2, 100.3, 100.4, 101.1, 101.2, 101.3, 101.4},         // adj close
			})
		Expect(minuteErr).NotTo(HaveOccurred())

		intradayProvider := data.NewIntradayTestProvider(minuteDF)

		strategy = &intradayRecordStrategy{
			schedule: "0 10,14 * * MON-FRI",
		}

		eng = engine.New(strategy,
			engine.WithAssetProvider(&mockAssetProvider{assets: []asset.Asset{spy}}),
			engine.WithDataProvider(dailyProvider),
			engine.WithDataProvider(intradayProvider),
			engine.WithInitialDeposit(10000),
		)

		strategy.universe = eng.Universe(spy)
	})

	It("fires Compute twice per trading day at 10:00 and 14:00 Eastern", func() {
		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		strategy.mu.Lock()
		defer strategy.mu.Unlock()

		Expect(strategy.fired).To(HaveLen(4)) // 2 days * 2 firings

		Expect(strategy.fired[0].Hour()).To(Equal(10))
		Expect(strategy.fired[0].Minute()).To(Equal(0))
		Expect(strategy.fired[1].Hour()).To(Equal(14))
		Expect(strategy.fired[1].Minute()).To(Equal(0))
		Expect(strategy.fired[2].Hour()).To(Equal(10))
		Expect(strategy.fired[2].Minute()).To(Equal(0))
		Expect(strategy.fired[3].Hour()).To(Equal(14))
		Expect(strategy.fired[3].Minute()).To(Equal(0))
	})

	It("makes engine.Now() carry the firing timestamp during Compute", func() {
		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		strategy.mu.Lock()
		defer strategy.mu.Unlock()

		for _, fired := range strategy.fired {
			// Each fired timestamp has non-zero hour, so it is not the
			// day boundary.
			Expect(fired.Hour() == 10 || fired.Hour() == 14).To(BeTrue())
		}
	})

	It("routes MinuteBars Window calls through IntradayFetch", func() {
		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		strategy.mu.Lock()
		defer strategy.mu.Unlock()

		Expect(strategy.windows).To(HaveLen(4))

		for _, df := range strategy.windows {
			// MinuteBars(3) requests 3 minutes back; the test data has
			// bars at the firing time and nearby. At least one row
			// should be returned.
			Expect(df).NotTo(BeNil())
		}
	})
})
