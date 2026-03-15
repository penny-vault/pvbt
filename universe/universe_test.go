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

// mockDataSource implements universe.DataSource for testing.
type mockDataSource struct {
	currentDate time.Time
	fetchCalled bool
	fetchAssets []asset.Asset
	fetchPeriod portfolio.Period
	fetchResult *data.DataFrame
}

func (m *mockDataSource) Fetch(_ context.Context, assets []asset.Asset, lookback portfolio.Period, _ []data.Metric) (*data.DataFrame, error) {
	m.fetchCalled = true
	m.fetchAssets = assets
	m.fetchPeriod = lookback
	return m.fetchResult, nil
}

func (m *mockDataSource) FetchAt(_ context.Context, assets []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	m.fetchCalled = true
	m.fetchAssets = assets
	return m.fetchResult, nil
}

func (m *mockDataSource) CurrentDate() time.Time {
	return m.currentDate
}

var _ = Describe("Static Universe", func() {
	var (
		aapl    asset.Asset
		goog    asset.Asset
		now     time.Time
		emptyDF *data.DataFrame
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
		emptyDF, _ = data.NewDataFrame(nil, nil, nil, data.Daily, nil)
	})

	Describe("Window", func() {
		It("delegates to the data source Fetch method", func() {
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			staticUniverse := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

			_, err := staticUniverse.Window(context.Background(), portfolio.Months(3), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())

			Expect(ds.fetchCalled).To(BeTrue())
			Expect(ds.fetchAssets).To(HaveLen(2))
			Expect(ds.fetchPeriod).To(Equal(portfolio.Months(3)))
		})

		It("returns an error when no data source is set", func() {
			staticUniverse := universe.NewStatic("AAPL")
			_, err := staticUniverse.Window(context.Background(), portfolio.Months(3), data.MetricClose)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CurrentDate", func() {
		It("delegates to the data source", func() {
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			staticUniverse := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			Expect(staticUniverse.CurrentDate()).To(Equal(now))
		})

		It("returns zero time when no data source is set", func() {
			staticUniverse := universe.NewStatic("AAPL")

			Expect(staticUniverse.CurrentDate()).To(Equal(time.Time{}))
		})
	})

	Describe("At", func() {
		It("delegates to the data source FetchAt method", func() {
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			staticUniverse := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			_, err := staticUniverse.At(context.Background(), now, data.MetricClose)
			Expect(err).NotTo(HaveOccurred())

			Expect(ds.fetchCalled).To(BeTrue())
		})
	})
})
