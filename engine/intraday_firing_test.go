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

// intradayMarkStrategy buys a fixed quantity of SPY on its first firing and
// thereafter records the prices the engine marks the account to during each
// firing: the price DataFrame close, the position value, and the total
// portfolio value. Once it holds the position it also issues an Allocate to a
// target weight and records the resulting order amount, so the test can prove
// Allocate sizes off the live eng.Now() mark rather than the prior EOD close.
type intradayMarkStrategy struct {
	mu           sync.Mutex
	spy          asset.Asset
	bought       bool
	doAllocate   bool
	allocated    bool
	priceSeen    []float64
	posValueSeen []float64
	valueSeen    []float64
	allocAmounts []float64
	schedule     string
}

func (s *intradayMarkStrategy) Name() string           { return "intradayMark" }
func (s *intradayMarkStrategy) Setup(_ *engine.Engine) {}

func (s *intradayMarkStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "intraday-mark", Schedule: s.schedule}
}

func (s *intradayMarkStrategy) Compute(
	_ context.Context,
	_ *engine.Engine,
	port portfolio.Portfolio,
	batch *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	price := math.NaN()
	if prices := port.Prices(); prices != nil {
		price = prices.Value(s.spy, data.MetricClose)
	}

	s.priceSeen = append(s.priceSeen, price)
	s.posValueSeen = append(s.posValueSeen, port.PositionValue(s.spy))
	s.valueSeen = append(s.valueSeen, port.Value())

	if !s.bought {
		s.bought = true

		return batch.Order(context.Background(), s.spy, portfolio.Buy, 10)
	}

	// When enabled, exercise Allocate end-to-end exactly once on the day-2
	// 10:00 firing (the third firing) and capture the order it sizes off the
	// live projected value. Restricting to a single firing keeps the held
	// quantity fixed at the day-1 purchase so the expected amount is exact;
	// fills drain synchronously inside ExecuteBatch.
	if s.doAllocate && !s.allocated && len(s.priceSeen) == 3 {
		s.allocated = true

		before := len(batch.Orders)
		if err := batch.Allocate(context.Background(), s.spy, 0.5); err != nil {
			return err
		}

		if len(batch.Orders) > before {
			s.allocAmounts = append(s.allocAmounts, batch.Orders[before].Amount)
		}
	}

	return nil
}

var _ = Describe("intra-day account marking", func() {
	var (
		nyc      *time.Location
		start    time.Time
		end      time.Time
		spy      asset.Asset
		eng      *engine.Engine
		strategy *intradayMarkStrategy
	)

	BeforeEach(func() {
		var locErr error

		nyc, locErr = time.LoadLocation("America/New_York")
		Expect(locErr).NotTo(HaveOccurred())

		start = time.Date(2026, 5, 11, 0, 0, 0, 0, nyc)
		end = time.Date(2026, 5, 12, 23, 59, 59, 0, nyc)

		spy = asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BDTBL9"}

		dailyTimes := []time.Time{
			time.Date(2026, 5, 11, 16, 0, 0, 0, nyc),
			time.Date(2026, 5, 12, 16, 0, 0, 0, nyc),
		}

		dailyDF, dailyErr := data.NewDataFrame(dailyTimes,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow, data.Volume, data.Dividend, data.SplitFactor},
			data.Daily,
			[][]float64{
				{100, 101},             // close (EOD)
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

		// Day-2 minute closes (105, 106) are deliberately well above the
		// day-2 EOD close (101) so the live mark is unambiguously distinct
		// from the EOD mark the engine would otherwise use.
		minuteDF, minuteErr := data.NewDataFrame(minuteTimes,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.MetricHigh, data.MetricLow, data.Volume, data.AdjClose},
			data.Tick,
			[][]float64{
				{100.1, 100.2, 100.3, 100.4, 105.0, 105.1, 106.0, 106.1}, // close
				{100.5, 100.6, 100.7, 100.8, 105.5, 105.6, 106.5, 106.6}, // high
				{99.5, 99.6, 99.7, 99.8, 104.5, 104.6, 105.5, 105.6},     // low
				{50_000, 50_000, 50_000, 50_000, 50_000, 50_000, 50_000, 50_000},
				{100.1, 100.2, 100.3, 100.4, 105.0, 105.1, 106.0, 106.1}, // adj close
			})
		Expect(minuteErr).NotTo(HaveOccurred())

		intradayProvider := data.NewIntradayTestProvider(minuteDF)

		strategy = &intradayMarkStrategy{spy: spy, schedule: "0 10,14 * * MON-FRI"}

		eng = engine.New(strategy,
			engine.WithAssetProvider(&mockAssetProvider{assets: []asset.Asset{spy}}),
			engine.WithDataProvider(dailyProvider),
			engine.WithDataProvider(intradayProvider),
			engine.WithInitialDeposit(10000),
		)
	})

	It("marks held positions to the eng.Now() minute bar, not the prior EOD close", func() {
		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		strategy.mu.Lock()
		defer strategy.mu.Unlock()

		Expect(strategy.priceSeen).To(HaveLen(4)) // 2 days * 2 firings

		// Day 1 buys 10 SPY at 10:00; the fill drains into the account
		// before day 2's firings. On day 2 the account is marked to the
		// day-2 minute bars (close 105 at 10:00, 106 at 14:00), not the
		// day-1 EOD close (100) nor the day-2 EOD close (101).
		Expect(strategy.priceSeen[2]).To(BeNumerically("~", 105.0, 1e-9))
		Expect(strategy.priceSeen[3]).To(BeNumerically("~", 106.0, 1e-9))

		Expect(strategy.posValueSeen[2]).To(BeNumerically("~", 10*105.0, 1e-6))
		Expect(strategy.posValueSeen[3]).To(BeNumerically("~", 10*106.0, 1e-6))

		// Total value reflects the live position mark. Cash after the
		// fill is 10000 - 10*100.2 (filled at the day-1 10:01 bar) = 8998.
		Expect(strategy.valueSeen[2]).To(BeNumerically("~", 8998+10*105.0, 1e-6))
	})

	It("sizes Allocate off the live intraday value, not the prior EOD close", func() {
		strategy.doAllocate = true

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		strategy.mu.Lock()
		defer strategy.mu.Unlock()

		// Allocate runs once, on the day-2 10:00 firing, holding the 10 SPY
		// bought on day 1 (cash 8998). The live projected value is
		// 8998 + 10*105 = 10048; targeting 0.5 against a current position
		// worth 10*105 = 1050 yields a buy of 0.5*10048 - 1050 = 3974.
		// Sized off the day-2 EOD close (101) it would instead be
		// 0.5*(8998+1010) - 1010 = 3994, so the ~20 difference pins the
		// live mark.
		Expect(strategy.allocAmounts).To(HaveLen(1))
		Expect(strategy.allocAmounts[0]).To(BeNumerically("~", 3974.0, 1e-6))
	})
})

// intradayMarginStrategy shorts a large SPY position on its first firing,
// then on the day-2 10:00 firing attempts to extend the short. The attempt is
// gated by the Reg-T initial-margin check, which depends on account equity --
// and therefore on the price the engine marks the existing short to.
type intradayMarginStrategy struct {
	mu        sync.Mutex
	spy       asset.Asset
	firings   int
	shorted   bool
	triedMore bool
	schedule  string
}

func (s *intradayMarginStrategy) Name() string           { return "intradayMargin" }
func (s *intradayMarginStrategy) Setup(_ *engine.Engine) {}

func (s *intradayMarginStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "intraday-margin", Schedule: s.schedule}
}

func (s *intradayMarginStrategy) Compute(
	_ context.Context,
	_ *engine.Engine,
	_ portfolio.Portfolio,
	batch *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.firings++

	if !s.shorted {
		s.shorted = true

		return batch.Order(context.Background(), s.spy, portfolio.Sell, 120)
	}

	// Day-2 10:00 firing: try to extend the short.
	if !s.triedMore && s.firings == 3 {
		s.triedMore = true

		return batch.Order(context.Background(), s.spy, portfolio.Sell, 10)
	}

	return nil
}

var _ = Describe("intra-day margin checks", func() {
	var (
		nyc      *time.Location
		start    time.Time
		end      time.Time
		spy      asset.Asset
		eng      *engine.Engine
		strategy *intradayMarginStrategy
	)

	BeforeEach(func() {
		var locErr error

		nyc, locErr = time.LoadLocation("America/New_York")
		Expect(locErr).NotTo(HaveOccurred())

		start = time.Date(2026, 5, 11, 0, 0, 0, 0, nyc)
		end = time.Date(2026, 5, 12, 23, 59, 59, 0, nyc)

		spy = asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BDTBL9"}

		dailyTimes := []time.Time{
			time.Date(2026, 5, 11, 16, 0, 0, 0, nyc),
			time.Date(2026, 5, 12, 16, 0, 0, 0, nyc),
		}

		dailyDF, dailyErr := data.NewDataFrame(dailyTimes,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow, data.Volume, data.Dividend, data.SplitFactor},
			data.Daily,
			[][]float64{
				{100, 101},             // close (EOD): day-2 short still well-collateralized
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

		// The day-2 minute price (130) is far above the day-2 EOD close
		// (101): the short has moved sharply against the account intraday,
		// cutting live equity well below what the EOD mark would show.
		minuteDF, minuteErr := data.NewDataFrame(minuteTimes,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.MetricHigh, data.MetricLow, data.Volume, data.AdjClose},
			data.Tick,
			[][]float64{
				{100.1, 100.2, 100.3, 100.4, 130.0, 130.1, 131.0, 131.1}, // close
				{100.5, 100.6, 100.7, 100.8, 130.5, 130.6, 131.5, 131.6}, // high
				{99.5, 99.6, 99.7, 99.8, 129.5, 129.6, 130.5, 130.6},     // low
				{50_000, 50_000, 50_000, 50_000, 50_000, 50_000, 50_000, 50_000},
				{100.1, 100.2, 100.3, 100.4, 130.0, 130.1, 131.0, 131.1}, // adj close
			})
		Expect(minuteErr).NotTo(HaveOccurred())

		intradayProvider := data.NewIntradayTestProvider(minuteDF)

		strategy = &intradayMarginStrategy{spy: spy, schedule: "0 10,14 * * MON-FRI"}

		eng = engine.New(strategy,
			engine.WithAssetProvider(&mockAssetProvider{assets: []asset.Asset{spy}}),
			engine.WithDataProvider(dailyProvider),
			engine.WithDataProvider(intradayProvider),
			engine.WithInitialDeposit(10000),
		)
	})

	It("rejects an order that breaches initial margin under the live intraday mark", func() {
		acct, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		// Day 1 shorts 120 SPY (fills at 100.2, proceeds 12024, cash 22024).
		// On day-2 10:00 the live mark is 130, so the short is worth
		// 120*130 = 15600 and equity is 22024 - 15600 = 6424. Extending the
		// short by 10 (filling at 130.1) needs equity/newShortValue =
		// 6424/16901 = 0.38 < 0.50 initial margin, so it is rejected and the
		// position stays at -120. Marked to the day-2 EOD close (101) instead,
		// equity would be 9904 and the ratio 9904/13421 = 0.74 >= 0.50, which
		// would have let the extension through to -130. The -120 result
		// therefore can only happen if the live intraday mark was used.
		Expect(acct.Holdings()[spy]).To(BeNumerically("~", -120.0, 1e-9))
	})
})

// intradayMultiMarkStrategy buys two assets on its first firing, then records
// each asset's position value on every firing. It exercises the multi-asset
// marking path, including held assets whose latest minute bar is older than
// the firing minute.
type intradayMultiMarkStrategy struct {
	mu        sync.Mutex
	spy       asset.Asset
	tlt       asset.Asset
	bought    bool
	spyValues []float64
	tltValues []float64
	schedule  string
}

func (s *intradayMultiMarkStrategy) Name() string           { return "intradayMultiMark" }
func (s *intradayMultiMarkStrategy) Setup(_ *engine.Engine) {}

func (s *intradayMultiMarkStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "intraday-multi-mark", Schedule: s.schedule}
}

func (s *intradayMultiMarkStrategy) Compute(
	_ context.Context,
	_ *engine.Engine,
	port portfolio.Portfolio,
	batch *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.spyValues = append(s.spyValues, port.PositionValue(s.spy))
	s.tltValues = append(s.tltValues, port.PositionValue(s.tlt))

	if !s.bought {
		s.bought = true

		if err := batch.Order(context.Background(), s.spy, portfolio.Buy, 10); err != nil {
			return err
		}

		return batch.Order(context.Background(), s.tlt, portfolio.Buy, 5)
	}

	return nil
}

var _ = Describe("intra-day marking with multiple held assets", func() {
	var (
		nyc      *time.Location
		start    time.Time
		end      time.Time
		spy      asset.Asset
		tlt      asset.Asset
		eng      *engine.Engine
		strategy *intradayMultiMarkStrategy
	)

	BeforeEach(func() {
		var locErr error

		nyc, locErr = time.LoadLocation("America/New_York")
		Expect(locErr).NotTo(HaveOccurred())

		start = time.Date(2026, 5, 11, 0, 0, 0, 0, nyc)
		end = time.Date(2026, 5, 12, 23, 59, 59, 0, nyc)

		spy = asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BDTBL9"}
		tlt = asset.Asset{Ticker: "TLT", CompositeFigi: "BBG000BDTBL0"}

		dailyTimes := []time.Time{
			time.Date(2026, 5, 11, 16, 0, 0, 0, nyc),
			time.Date(2026, 5, 12, 16, 0, 0, 0, nyc),
		}

		dailyMetrics := []data.Metric{
			data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow,
			data.Volume, data.Dividend, data.SplitFactor,
		}

		dailyDF, dailyErr := data.NewDataFrame(dailyTimes,
			[]asset.Asset{spy, tlt},
			dailyMetrics,
			data.Daily,
			[][]float64{
				// SPY
				{100, 101}, {100, 101}, {102, 103}, {99, 100}, {1_000_000, 1_000_000}, {0, 0}, {1, 1},
				// TLT
				{50, 51}, {50, 51}, {52, 53}, {49, 50}, {1_000_000, 1_000_000}, {0, 0}, {1, 1},
			})
		Expect(dailyErr).NotTo(HaveOccurred())

		dailyProvider := data.NewTestProvider(dailyMetrics, dailyDF)

		// Time grid is the union across both assets. SPY trades at the firing
		// minute on day 2; TLT does not -- its latest day-2 morning bar is at
		// 09:58 (NaN at 10:00), and it has no bar at all in the 13:54-14:00
		// window before the 14:00 firing.
		minuteTimes := []time.Time{
			time.Date(2026, 5, 11, 10, 0, 0, 0, nyc), // 0
			time.Date(2026, 5, 11, 10, 1, 0, 0, nyc), // 1 (day-1 fill bar)
			time.Date(2026, 5, 11, 14, 0, 0, 0, nyc), // 2
			time.Date(2026, 5, 11, 14, 1, 0, 0, nyc), // 3
			time.Date(2026, 5, 12, 9, 58, 0, 0, nyc), // 4 (TLT only)
			time.Date(2026, 5, 12, 10, 0, 0, 0, nyc), // 5 (SPY only)
			time.Date(2026, 5, 12, 10, 1, 0, 0, nyc), // 6
			time.Date(2026, 5, 12, 14, 0, 0, 0, nyc), // 7 (SPY only)
			time.Date(2026, 5, 12, 14, 1, 0, 0, nyc), // 8
		}

		nan := math.NaN()

		minuteMetrics := []data.Metric{data.MetricClose, data.MetricHigh, data.MetricLow, data.Volume, data.AdjClose}

		minuteDF, minuteErr := data.NewDataFrame(minuteTimes,
			[]asset.Asset{spy, tlt},
			minuteMetrics,
			data.Tick,
			[][]float64{
				// SPY close / high / low / volume / adjclose
				{100.1, 100.2, 100.3, 100.4, nan, 105.0, 105.1, 106.0, 106.1},
				{100.5, 100.6, 100.7, 100.8, nan, 105.5, 105.6, 106.5, 106.6},
				{99.5, 99.6, 99.7, 99.8, nan, 104.5, 104.6, 105.5, 105.6},
				{50_000, 50_000, 50_000, 50_000, nan, 50_000, 50_000, 50_000, 50_000},
				{100.1, 100.2, 100.3, 100.4, nan, 105.0, 105.1, 106.0, 106.1},
				// TLT close / high / low / volume / adjclose
				{50.0, 50.0, 50.0, 50.0, 50.0, nan, 50.1, nan, nan},
				{50.2, 50.2, 50.2, 50.2, 50.2, nan, 50.3, nan, nan},
				{49.8, 49.8, 49.8, 49.8, 49.8, nan, 49.9, nan, nan},
				{50_000, 50_000, 50_000, 50_000, 50_000, nan, 50_000, nan, nan},
				{50.0, 50.0, 50.0, 50.0, 50.0, nan, 50.1, nan, nan},
			})
		Expect(minuteErr).NotTo(HaveOccurred())

		intradayProvider := data.NewIntradayTestProvider(minuteDF)

		strategy = &intradayMultiMarkStrategy{spy: spy, tlt: tlt, schedule: "0 10,14 * * MON-FRI"}

		eng = engine.New(strategy,
			engine.WithAssetProvider(&mockAssetProvider{assets: []asset.Asset{spy, tlt}}),
			engine.WithDataProvider(dailyProvider),
			engine.WithDataProvider(intradayProvider),
			engine.WithInitialDeposit(10000),
		)
	})

	It("marks each held asset to its own latest bar and never drops one missing the firing minute", func() {
		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		strategy.mu.Lock()
		defer strategy.mu.Unlock()

		Expect(strategy.spyValues).To(HaveLen(4))

		// Day-2 10:00 firing (index 2): SPY trades at 10:00 (105) while TLT's
		// latest bar in the 09:54-10:00 window is 09:58 (50). TLT must be
		// marked to 50, not dropped to 0 because it lacks a 10:00 bar.
		Expect(strategy.spyValues[2]).To(BeNumerically("~", 10*105.0, 1e-6))
		Expect(strategy.tltValues[2]).To(BeNumerically("~", 5*50.0, 1e-6))

		// Day-2 14:00 firing (index 3): TLT has no bar in the window at all,
		// so it carries forward its prior mark (50) rather than dropping to 0.
		Expect(strategy.spyValues[3]).To(BeNumerically("~", 10*106.0, 1e-6))
		Expect(strategy.tltValues[3]).To(BeNumerically("~", 5*50.0, 1e-6))
	})
})

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
