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

// countingIntradayProvider wraps an IntradayTestProvider and counts how many
// times IntradayFetch is invoked, so a test can assert that a given engine
// path never reaches the intraday (ClickHouse) source.
type countingIntradayProvider struct {
	*data.IntradayTestProvider

	mu    sync.Mutex
	calls int
}

func (p *countingIntradayProvider) IntradayFetch(
	ctx context.Context,
	assets []asset.Asset,
	metrics []data.Metric,
	start, end time.Time,
	timesOfDay []data.TimeOfDay,
) (*data.DataFrame, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()

	return p.IntradayTestProvider.IntradayFetch(ctx, assets, metrics, start, end, timesOfDay)
}

func (p *countingIntradayProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.calls
}

// openBuyStrategy fires at the market open and buys one share on its first
// firing. It does not request any intraday window, so the only way the
// engine could reach the intraday provider is via order-fill price lookup.
type openBuyStrategy struct {
	spy    asset.Asset
	mu     sync.Mutex
	bought bool
}

func (s *openBuyStrategy) Name() string           { return "openBuy" }
func (s *openBuyStrategy) Setup(_ *engine.Engine) {}

func (s *openBuyStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "@open * * *"}
}

func (s *openBuyStrategy) Compute(
	ctx context.Context,
	_ *engine.Engine,
	_ portfolio.Portfolio,
	batch *portfolio.Batch,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.bought {
		batch.Order(ctx, s.spy, portfolio.Buy, 1)
		s.bought = true
	}

	return nil
}

var _ = Describe("market-open firing fills", func() {
	It("fills open-firing orders from the EOD source, never the intraday source", func() {
		nyc, locErr := time.LoadLocation("America/New_York")
		Expect(locErr).NotTo(HaveOccurred())

		start := time.Date(2026, 5, 11, 0, 0, 0, 0, nyc)  // Monday
		end := time.Date(2026, 5, 12, 23, 59, 59, 0, nyc) // Tuesday

		spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BDTBL9"}

		dailyTimes := []time.Time{
			time.Date(2026, 5, 11, 16, 0, 0, 0, nyc),
			time.Date(2026, 5, 12, 16, 0, 0, 0, nyc),
		}

		dailyMetrics := []data.Metric{
			data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow,
			data.Volume, data.Dividend, data.SplitFactor,
		}

		dailyDF, dailyErr := data.NewDataFrame(dailyTimes,
			[]asset.Asset{spy},
			dailyMetrics,
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

		dailyProvider := data.NewTestProvider(dailyMetrics, dailyDF)

		// Minute bars just after the open carry a wildly different price.
		// If the open-firing order fill (incorrectly) used the intraday
		// source, the fill would land near 9999 instead of ~100.
		minuteTimes := []time.Time{
			time.Date(2026, 5, 11, 9, 31, 0, 0, nyc),
			time.Date(2026, 5, 11, 9, 32, 0, 0, nyc),
			time.Date(2026, 5, 12, 9, 31, 0, 0, nyc),
			time.Date(2026, 5, 12, 9, 32, 0, 0, nyc),
		}

		minuteDF, minuteErr := data.NewDataFrame(minuteTimes,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.MetricHigh, data.MetricLow, data.Volume, data.AdjClose},
			data.Tick,
			[][]float64{
				{9999, 9999, 9999, 9999}, // close
				{9999, 9999, 9999, 9999}, // high
				{9999, 9999, 9999, 9999}, // low
				{50_000, 50_000, 50_000, 50_000},
				{9999, 9999, 9999, 9999}, // adj close
			})
		Expect(minuteErr).NotTo(HaveOccurred())

		intraday := &countingIntradayProvider{IntradayTestProvider: data.NewIntradayTestProvider(minuteDF)}

		strategy := &openBuyStrategy{spy: spy}

		eng := engine.New(strategy,
			engine.WithAssetProvider(&mockAssetProvider{assets: []asset.Asset{spy}}),
			engine.WithDataProvider(dailyProvider),
			engine.WithDataProvider(intraday),
			engine.WithInitialDeposit(10000),
		)

		fund, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		Expect(intraday.callCount()).To(Equal(0),
			"a firing at the market open must source prices from EOD, not the intraday/ClickHouse provider")

		var (
			buyFound bool
			buyPrice float64
		)

		for _, txn := range fund.Transactions() {
			if txn.Type == asset.BuyTransaction {
				buyFound = true
				buyPrice = txn.Price

				break
			}
		}

		Expect(buyFound).To(BeTrue(), "expected the open-firing buy to fill")
		Expect(buyPrice).To(BeNumerically("<", 200),
			"fill price should come from the EOD bars (~100), not the intraday bars (9999)")
	})
})
