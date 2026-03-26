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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// mockIndexProvider returns assets keyed by t.Unix().
type mockIndexProvider struct {
	results map[int64][]asset.Asset
}

func (m *mockIndexProvider) IndexMembers(_ context.Context, _ string, t time.Time) ([]asset.Asset, error) {
	if assets, ok := m.results[t.Unix()]; ok {
		return assets, nil
	}
	return nil, nil
}

// countingIndexProvider wraps mockIndexProvider and counts calls.
type countingIndexProvider struct {
	inner     *mockIndexProvider
	callCount *int
}

func (c *countingIndexProvider) IndexMembers(ctx context.Context, index string, t time.Time) ([]asset.Asset, error) {
	*c.callCount++
	return c.inner.IndexMembers(ctx, index, t)
}

// errorIndexProvider always returns an error.
type errorIndexProvider struct{}

func (e *errorIndexProvider) IndexMembers(_ context.Context, _ string, _ time.Time) ([]asset.Asset, error) {
	return nil, fmt.Errorf("provider error")
}

var _ = Describe("Index Universe", func() {
	var (
		aapl    asset.Asset
		goog    asset.Asset
		msft    asset.Asset
		now     time.Time
		emptyDF *data.DataFrame
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
		emptyDF, _ = data.NewDataFrame(nil, nil, nil, data.Daily, nil)
	})

	Describe("Assets", func() {
		It("returns assets from the index provider sorted by ticker", func() {
			provider := &mockIndexProvider{
				results: map[int64][]asset.Asset{
					now.Unix(): {goog, aapl, msft},
				},
			}
			u := universe.NewIndex(provider, "SP500")
			assets := u.Assets(now)
			Expect(assets).To(HaveLen(3))
			Expect(assets[0].Ticker).To(Equal("AAPL"))
			Expect(assets[1].Ticker).To(Equal("GOOG"))
			Expect(assets[2].Ticker).To(Equal("MSFT"))
		})

		It("caches results for the same date", func() {
			callCount := 0
			provider := &countingIndexProvider{
				inner: &mockIndexProvider{
					results: map[int64][]asset.Asset{
						now.Unix(): {aapl, goog},
					},
				},
				callCount: &callCount,
			}
			u := universe.NewIndex(provider, "SP500")
			u.Assets(now)
			u.Assets(now)
			Expect(callCount).To(Equal(1))
		})

		It("returns nil when provider returns no assets", func() {
			provider := &mockIndexProvider{
				results: map[int64][]asset.Asset{},
			}
			u := universe.NewIndex(provider, "SP500")
			assets := u.Assets(now)
			Expect(assets).To(BeNil())
		})

		It("returns nil when the provider returns an error", func() {
			u := universe.NewIndex(&errorIndexProvider{}, "SP500")
			assets := u.Assets(now)
			Expect(assets).To(BeNil())
		})
	})

	Describe("Window", func() {
		It("delegates to the data source with resolved assets", func() {
			provider := &mockIndexProvider{
				results: map[int64][]asset.Asset{
					now.Unix(): {aapl, goog},
				},
			}
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			u := universe.NewIndex(provider, "SP500")
			u.SetDataSource(ds)

			result, err := u.Window(context.Background(), portfolio.Months(3), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())
			Expect(ds.fetchCalled).To(BeTrue())
			Expect(ds.fetchAssets).To(ConsistOf(aapl, goog))
			Expect(ds.fetchPeriod).To(Equal(portfolio.Months(3)))
			Expect(result).To(BeIdenticalTo(emptyDF))
		})

		It("returns an error when no data source is set", func() {
			provider := &mockIndexProvider{results: map[int64][]asset.Asset{}}
			u := universe.NewIndex(provider, "SP500")
			_, err := u.Window(context.Background(), portfolio.Months(3), data.MetricClose)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("At", func() {
		It("delegates to the data source with resolved assets", func() {
			provider := &mockIndexProvider{
				results: map[int64][]asset.Asset{
					now.Unix(): {aapl},
				},
			}
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			u := universe.NewIndex(provider, "SP500")
			u.SetDataSource(ds)

			result, err := u.At(context.Background(), now, data.MetricClose)
			Expect(err).NotTo(HaveOccurred())
			Expect(ds.fetchCalled).To(BeTrue())
			Expect(ds.fetchAssets).To(ConsistOf(aapl))
			Expect(result).To(BeIdenticalTo(emptyDF))
		})

		It("returns an error when no data source is set", func() {
			provider := &mockIndexProvider{results: map[int64][]asset.Asset{}}
			u := universe.NewIndex(provider, "SP500")
			_, err := u.At(context.Background(), now, data.MetricClose)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CurrentDate", func() {
		It("delegates to the data source", func() {
			provider := &mockIndexProvider{results: map[int64][]asset.Asset{}}
			ds := &mockDataSource{currentDate: now}
			u := universe.NewIndex(provider, "SP500")
			u.SetDataSource(ds)

			Expect(u.CurrentDate()).To(Equal(now))
		})

		It("returns zero time when no data source is set", func() {
			provider := &mockIndexProvider{results: map[int64][]asset.Asset{}}
			u := universe.NewIndex(provider, "SP500")
			Expect(u.CurrentDate()).To(Equal(time.Time{}))
		})
	})

	Describe("Prefetch", func() {
		It("pre-populates the cache so Assets skips the provider", func() {
			day1 := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
			day2 := time.Date(2025, 6, 16, 0, 0, 0, 0, time.UTC)

			callCount := 0
			inner := &mockIndexProvider{
				results: map[int64][]asset.Asset{
					day1.Unix(): {goog, aapl},
					day2.Unix(): {msft},
				},
			}
			provider := &countingIndexProvider{
				inner:     inner,
				callCount: &callCount,
			}
			u := universe.NewIndex(provider, "SP500")

			err := u.Prefetch(context.Background(), day1, day2)
			Expect(err).NotTo(HaveOccurred())

			assets := u.Assets(day1)
			Expect(assets).To(HaveLen(2))
			Expect(assets[0].Ticker).To(Equal("AAPL"))
			Expect(assets[1].Ticker).To(Equal("GOOG"))

			beforeCount := callCount
			u.Assets(day1)
			u.Assets(day2)
			Expect(callCount).To(Equal(beforeCount))
		})
	})

	Describe("SP500 and Nasdaq100 convenience constructors", func() {
		It("SP500 returns a universe that queries 'SP500'", func() {
			provider := &mockIndexProvider{
				results: map[int64][]asset.Asset{
					now.Unix(): {aapl},
				},
			}
			u := universe.SP500(provider)
			assets := u.Assets(now)
			Expect(assets).To(ConsistOf(aapl))
		})

		It("Nasdaq100 returns a universe that queries 'NASDAQ100'", func() {
			provider := &mockIndexProvider{
				results: map[int64][]asset.Asset{
					now.Unix(): {goog},
				},
			}
			u := universe.Nasdaq100(provider)
			assets := u.Assets(now)
			Expect(assets).To(ConsistOf(goog))
		})
	})
})
