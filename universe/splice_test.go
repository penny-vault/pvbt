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

package universe_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// spliceMockDataSource serves a per-asset DataFrame on every Fetch/FetchAt
// call, recording which assets were requested. Unlike mockDataSource above,
// it supports multiple Fetch calls with different asset lists -- splice needs
// one Fetch per segment.
type spliceMockDataSource struct {
	currentDate time.Time
	frames      map[string]*data.DataFrame // keyed by Ticker
	fetchCalls  []string                   // tickers requested, in order
}

func (m *spliceMockDataSource) Fetch(_ context.Context, assets []asset.Asset, _ portfolio.Period, _ []data.Metric) (*data.DataFrame, error) {
	m.fetchCalls = append(m.fetchCalls, assets[0].Ticker)
	return m.frames[assets[0].Ticker], nil
}

func (m *spliceMockDataSource) FetchAt(_ context.Context, assets []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	m.fetchCalls = append(m.fetchCalls, assets[0].Ticker)
	return m.frames[assets[0].Ticker], nil
}

func (m *spliceMockDataSource) CurrentDate() time.Time { return m.currentDate }

// makeDailyFrame builds a single-asset, single-metric DataFrame with one
// daily row per day in [start, start+nDays). Each row's value equals the
// (1-indexed) row number for easy assertions.
func makeDailyFrame(a asset.Asset, metric data.Metric, start time.Time, nDays int) *data.DataFrame {
	times := make([]time.Time, nDays)
	col := make([]float64, nDays)
	for ii := range nDays {
		times[ii] = start.AddDate(0, 0, ii)
		col[ii] = float64(ii + 1)
	}

	df, err := data.NewDataFrame(times, []asset.Asset{a}, []data.Metric{metric}, data.Daily, [][]float64{col})
	if err != nil {
		panic(err)
	}

	return df
}

var _ = Describe("Splice Universe", func() {
	var (
		tqqq   asset.Asset
		qld    asset.Asset
		cutoff time.Time
	)

	BeforeEach(func() {
		tqqq = asset.Asset{CompositeFigi: "FIGI-TQQQ", Ticker: "TQQQ"}
		qld = asset.Asset{CompositeFigi: "FIGI-QLD", Ticker: "QLD"}
		cutoff = time.Date(2010, 2, 11, 0, 0, 0, 0, time.UTC)
	})

	resolveAssets := func(u *universe.SpliceUniverse) {
		u.Resolve(func(ticker string) asset.Asset {
			switch ticker {
			case "TQQQ":
				return tqqq
			case "QLD":
				return qld
			}
			return asset.Asset{Ticker: ticker}
		})
	}

	Describe("Assets", func() {
		It("returns the fallback before the cutoff and the primary on or after", func() {
			u := universe.NewSplice("TQQQ", universe.SplicePeriod{Ticker: "QLD", Before: cutoff})
			resolveAssets(u)

			before := time.Date(2008, 6, 1, 0, 0, 0, 0, time.UTC)
			Expect(u.Assets(before)).To(ConsistOf(qld))

			at := cutoff
			Expect(u.Assets(at)).To(ConsistOf(tqqq))

			after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			Expect(u.Assets(after)).To(ConsistOf(tqqq))
		})

		It("walks multiple chronological fallbacks regardless of input order", func() {
			early := asset.Asset{CompositeFigi: "FIGI-EARLY", Ticker: "EARLY"}
			mid := asset.Asset{CompositeFigi: "FIGI-MID", Ticker: "MID"}
			cutoffEarly := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
			cutoffMid := cutoff

			// Pass fallbacks out of chronological order to confirm sorting.
			u := universe.NewSplice("TQQQ",
				universe.SplicePeriod{Ticker: "MID", Before: cutoffMid},
				universe.SplicePeriod{Ticker: "EARLY", Before: cutoffEarly},
			)
			u.Resolve(func(ticker string) asset.Asset {
				switch ticker {
				case "TQQQ":
					return tqqq
				case "MID":
					return mid
				case "EARLY":
					return early
				}
				return asset.Asset{Ticker: ticker}
			})

			Expect(u.Assets(time.Date(2003, 6, 1, 0, 0, 0, 0, time.UTC))).To(ConsistOf(early))
			Expect(u.Assets(time.Date(2007, 6, 1, 0, 0, 0, 0, time.UTC))).To(ConsistOf(mid))
			Expect(u.Assets(time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC))).To(ConsistOf(tqqq))
		})
	})

	Describe("At", func() {
		It("fetches the active ticker for the current simulation date", func() {
			before := time.Date(2008, 6, 1, 0, 0, 0, 0, time.UTC)
			ds := &spliceMockDataSource{
				currentDate: before,
				frames: map[string]*data.DataFrame{
					"QLD": makeDailyFrame(qld, data.MetricClose, before, 1),
				},
			}

			u := universe.NewSplice("TQQQ", universe.SplicePeriod{Ticker: "QLD", Before: cutoff})
			resolveAssets(u)
			u.SetDataSource(ds)

			df, err := u.At(context.Background(), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())
			Expect(df.AssetList()).To(ConsistOf(qld))
			Expect(ds.fetchCalls).To(Equal([]string{"QLD"}))
		})

		It("fetches the primary on the cutoff day and after", func() {
			at := cutoff
			ds := &spliceMockDataSource{
				currentDate: at,
				frames: map[string]*data.DataFrame{
					"TQQQ": makeDailyFrame(tqqq, data.MetricClose, at, 1),
				},
			}

			u := universe.NewSplice("TQQQ", universe.SplicePeriod{Ticker: "QLD", Before: cutoff})
			resolveAssets(u)
			u.SetDataSource(ds)

			_, err := u.At(context.Background(), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())
			Expect(ds.fetchCalls).To(Equal([]string{"TQQQ"}))
		})

		It("returns an error when no data source is set", func() {
			u := universe.NewSplice("TQQQ", universe.SplicePeriod{Ticker: "QLD", Before: cutoff})
			resolveAssets(u)

			_, err := u.At(context.Background(), data.MetricClose)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Window", func() {
		It("returns only primary data when the lookback is fully after the cutoff", func() {
			now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
			fetchStart := portfolio.Months(3).Before(now)

			ds := &spliceMockDataSource{
				currentDate: now,
				frames: map[string]*data.DataFrame{
					"TQQQ": makeDailyFrame(tqqq, data.MetricClose, fetchStart, 100),
					"QLD":  makeDailyFrame(qld, data.MetricClose, fetchStart, 100),
				},
			}

			u := universe.NewSplice("TQQQ", universe.SplicePeriod{Ticker: "QLD", Before: cutoff})
			resolveAssets(u)
			u.SetDataSource(ds)

			df, err := u.Window(context.Background(), portfolio.Months(3), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())

			// QLD was never fetched because its segment doesn't intersect the window.
			Expect(ds.fetchCalls).To(Equal([]string{"TQQQ"}))
			Expect(df.AssetList()).To(ConsistOf(tqqq))
			Expect(df.Len()).To(BeNumerically(">", 0))
		})

		It("returns only fallback data when the lookback is fully before the cutoff", func() {
			now := time.Date(2008, 6, 1, 0, 0, 0, 0, time.UTC)
			fetchStart := portfolio.Months(3).Before(now)

			ds := &spliceMockDataSource{
				currentDate: now,
				frames: map[string]*data.DataFrame{
					"QLD": makeDailyFrame(qld, data.MetricClose, fetchStart, 100),
				},
			}

			u := universe.NewSplice("TQQQ", universe.SplicePeriod{Ticker: "QLD", Before: cutoff})
			resolveAssets(u)
			u.SetDataSource(ds)

			df, err := u.Window(context.Background(), portfolio.Months(3), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())

			Expect(ds.fetchCalls).To(Equal([]string{"QLD"}))
			// All rows come from QLD, but the column identity is QLD itself
			// because that is the active ticker on the current sim date.
			Expect(df.AssetList()).To(ConsistOf(qld))
			Expect(df.Len()).To(BeNumerically(">", 0))
		})

		It("splices QLD and TQQQ rows when the window crosses the cutoff", func() {
			now := time.Date(2010, 4, 1, 0, 0, 0, 0, time.UTC)
			fetchStart := portfolio.Months(3).Before(now)

			ds := &spliceMockDataSource{
				currentDate: now,
				frames: map[string]*data.DataFrame{
					"QLD":  makeDailyFrame(qld, data.MetricClose, fetchStart, 120),
					"TQQQ": makeDailyFrame(tqqq, data.MetricClose, fetchStart, 120),
				},
			}

			u := universe.NewSplice("TQQQ", universe.SplicePeriod{Ticker: "QLD", Before: cutoff})
			resolveAssets(u)
			u.SetDataSource(ds)

			df, err := u.Window(context.Background(), portfolio.Months(3), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())

			// Both segments intersect the window, so both got fetched.
			Expect(ds.fetchCalls).To(ConsistOf("QLD", "TQQQ"))

			// Result is a single column under the active-today asset (TQQQ),
			// even though pre-cutoff bars are sourced from QLD.
			Expect(df.AssetList()).To(ConsistOf(tqqq))

			// Times are strictly increasing across the splice with no
			// duplicates at the cutoff boundary.
			times := df.Times()
			Expect(len(times)).To(BeNumerically(">", 1))
			for ii := 1; ii < len(times); ii++ {
				Expect(times[ii].After(times[ii-1])).To(BeTrue(),
					"timestamps must strictly increase across the splice; got %s then %s",
					times[ii-1], times[ii])
			}

			// The cutoff itself should be the first post-cutoff row.
			cutoffIdx := -1
			for ii, t := range times {
				if !t.Before(cutoff) {
					cutoffIdx = ii
					break
				}
			}
			Expect(cutoffIdx).To(BeNumerically(">", 0),
				"window must contain at least one pre-cutoff and one post-cutoff row")
		})

		It("returns an error when no data source is set", func() {
			u := universe.NewSplice("TQQQ", universe.SplicePeriod{Ticker: "QLD", Before: cutoff})
			resolveAssets(u)

			_, err := u.Window(context.Background(), portfolio.Months(3), data.MetricClose)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CurrentDate", func() {
		It("delegates to the data source", func() {
			now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			ds := &spliceMockDataSource{currentDate: now}

			u := universe.NewSplice("TQQQ", universe.SplicePeriod{Ticker: "QLD", Before: cutoff})
			resolveAssets(u)
			u.SetDataSource(ds)

			Expect(u.CurrentDate()).To(Equal(now))
		})

		It("returns zero time when no data source is set", func() {
			u := universe.NewSplice("TQQQ", universe.SplicePeriod{Ticker: "QLD", Before: cutoff})
			Expect(u.CurrentDate()).To(Equal(time.Time{}))
		})
	})
})
