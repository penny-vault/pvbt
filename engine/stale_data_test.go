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

var _ = Describe("adjustEndForExpectedEOD", func() {
	var (
		eastern    *time.Location
		testStock  asset.Asset
		dataAssets []asset.Asset
		metrics    []data.Metric
	)

	BeforeEach(func() {
		eastern = engine.NYCForTest()
		testStock = asset.Asset{CompositeFigi: "FIGI-EOD1", Ticker: "EOD1"}
		dataAssets = []asset.Asset{testStock}
		metrics = []data.Metric{
			data.MetricClose, data.AdjClose, data.Dividend,
			data.MetricHigh, data.MetricLow, data.SplitFactor, data.Volume,
		}
	})

	// buildEngine wires an engine over a fixed price series spanning
	// 2026-05-18 through 2026-05-29 (Mon-Fri across two weeks). Prices
	// are populated for all dates; tests then drive adjustEndForExpectedEOD
	// with synthetic `now` values to probe the trimming logic.
	buildEngine := func(includeLastDay bool) *engine.Engine {
		// 10 calendar dates: 2026-05-18 (Mon) through 2026-05-29 (Fri),
		// skipping the weekend 5/23-5/24.
		calendar := []time.Time{
			time.Date(2026, 5, 18, 16, 0, 0, 0, eastern),
			time.Date(2026, 5, 19, 16, 0, 0, 0, eastern),
			time.Date(2026, 5, 20, 16, 0, 0, 0, eastern),
			time.Date(2026, 5, 21, 16, 0, 0, 0, eastern),
			time.Date(2026, 5, 22, 16, 0, 0, 0, eastern),
			time.Date(2026, 5, 25, 16, 0, 0, 0, eastern),
			time.Date(2026, 5, 26, 16, 0, 0, 0, eastern),
			time.Date(2026, 5, 27, 16, 0, 0, 0, eastern),
			time.Date(2026, 5, 28, 16, 0, 0, 0, eastern),
			time.Date(2026, 5, 29, 16, 0, 0, 0, eastern),
		}

		nDays := len(calendar)
		nMetrics := len(metrics)
		vals := make([]float64, nDays*len(dataAssets)*nMetrics)

		for dayIdx := range nDays {
			closePrice := 100.0 + float64(dayIdx)
			if dayIdx == nDays-1 && !includeLastDay {
				closePrice = math.NaN()
			}

			vals[(0*nMetrics+0)*nDays+dayIdx] = closePrice
			vals[(0*nMetrics+1)*nDays+dayIdx] = closePrice
			vals[(0*nMetrics+2)*nDays+dayIdx] = 0.0
			if !math.IsNaN(closePrice) {
				vals[(0*nMetrics+3)*nDays+dayIdx] = closePrice + 1.0
				vals[(0*nMetrics+4)*nDays+dayIdx] = closePrice - 1.0
			} else {
				vals[(0*nMetrics+3)*nDays+dayIdx] = math.NaN()
				vals[(0*nMetrics+4)*nDays+dayIdx] = math.NaN()
			}
			vals[(0*nMetrics+5)*nDays+dayIdx] = 1.0
			vals[(0*nMetrics+6)*nDays+dayIdx] = 1_000_000.0
		}

		df, dfErr := data.NewDataFrame(calendar, dataAssets, metrics, data.Daily,
			data.SlabToColumns(vals, len(dataAssets)*nMetrics, nDays))
		Expect(dfErr).NotTo(HaveOccurred())

		provider := data.NewTestProvider(metrics, df)
		assetProv := &mockAssetProvider{assets: dataAssets}

		strategy := &BuyOnceExportedStrategy{Target: testStock, Qty: 10}
		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

		return engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProv),
			engine.WithAccount(acct),
		)
	}

	It("trims end to the previous trading day when wall-clock is before 4 PM ET", func() {
		eng := buildEngine(true)

		// Requested end 5/29 23:59:59 ET, wall-clock 5/28 01:22 ET (well
		// before today's close): expected last EOD is 5/27.
		requestedEnd := time.Date(2026, 5, 29, 23, 59, 59, 0, eastern)
		now := time.Date(2026, 5, 28, 1, 22, 0, 0, eastern)

		got, helperErr := engine.AdjustEndForExpectedEODForTest(eng, context.Background(), requestedEnd, now)
		Expect(helperErr).NotTo(HaveOccurred())

		Expect(got.In(eastern).Format("2006-01-02")).To(Equal("2026-05-27"))
	})

	It("keeps today when wall-clock is after 4 PM ET and today's data is present", func() {
		eng := buildEngine(true)

		requestedEnd := time.Date(2026, 5, 29, 23, 59, 59, 0, eastern)
		// 5/28 at 18:00 ET -- after today's close, today's price exists.
		now := time.Date(2026, 5, 28, 18, 0, 0, 0, eastern)

		got, helperErr := engine.AdjustEndForExpectedEODForTest(eng, context.Background(), requestedEnd, now)
		Expect(helperErr).NotTo(HaveOccurred())

		Expect(got.In(eastern).Format("2006-01-02")).To(Equal("2026-05-28"))
	})

	It("steps back one trading day when after 4 PM ET but today's data is missing", func() {
		// Engine where the last calendar date (5/29) has NaN close --
		// simulating an ingest still in flight after today's close.
		eng := buildEngine(false)

		requestedEnd := time.Date(2026, 5, 29, 23, 59, 59, 0, eastern)
		// 5/29 (Fri) at 18:00 ET -- after close, but the day's data is NaN.
		now := time.Date(2026, 5, 29, 18, 0, 0, 0, eastern)

		got, helperErr := engine.AdjustEndForExpectedEODForTest(eng, context.Background(), requestedEnd, now)
		Expect(helperErr).NotTo(HaveOccurred())

		Expect(got.In(eastern).Format("2006-01-02")).To(Equal("2026-05-28"))
	})

	It("leaves a historical end unchanged", func() {
		eng := buildEngine(true)

		// Requested end well in the past relative to wall-clock now.
		requestedEnd := time.Date(2026, 5, 20, 23, 59, 59, 0, eastern)
		now := time.Date(2026, 5, 28, 18, 0, 0, 0, eastern)

		got, helperErr := engine.AdjustEndForExpectedEODForTest(eng, context.Background(), requestedEnd, now)
		Expect(helperErr).NotTo(HaveOccurred())

		Expect(got).To(Equal(requestedEnd))
	})
})
