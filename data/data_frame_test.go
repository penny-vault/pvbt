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

package data_test

import (
	"context"
	"fmt"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

type mockFrameDataSource struct{}

func (m *mockFrameDataSource) Fetch(_ context.Context, _ []asset.Asset, _ data.Period, _ []data.Metric) (*data.DataFrame, error) {
	return nil, nil
}
func (m *mockFrameDataSource) FetchAt(_ context.Context, _ []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	return nil, nil
}
func (m *mockFrameDataSource) CurrentDate() time.Time { return time.Time{} }

var _ = Describe("DataFrame", func() {
	var (
		df    *data.DataFrame
		aapl  asset.Asset
		goog  asset.Asset
		times []time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "GOOG", Ticker: "GOOG"}

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		times = make([]time.Time, 5)
		for i := range times {
			times[i] = base.AddDate(0, 0, i)
		}

		// 2 assets, 2 metrics (Price, Volume), 5 timestamps
		// Layout: [AAPL/Price(5), AAPL/Volume(5), GOOG/Price(5), GOOG/Volume(5)]
		values := [][]float64{
			// AAPL Price
			{100, 101, 102, 103, 104},
			// AAPL Volume
			{1000, 1100, 1200, 1300, 1400},
			// GOOG Price
			{200, 202, 204, 206, 208},
			// GOOG Volume
			{2000, 2200, 2400, 2600, 2800},
		}

		var err error
		df, err = data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, data.Daily, values)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("NewDataFrame", func() {
		It("builds correct assetIndex", func() {
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(df.Value(goog, data.Price)).To(Equal(208.0))
		})

		It("returns error when data length mismatches dimensions", func() {
			_, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2}})
			Expect(err).To(HaveOccurred())
		})

		It("accepts empty dimensions with nil data", func() {
			empty, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(empty.Len()).To(Equal(0))
			Expect(empty.ColCount()).To(Equal(0))
		})
	})

	Describe("Accessors", func() {
		It("Start returns first timestamp", func() {
			Expect(df.Start()).To(Equal(times[0]))
		})

		It("End returns last timestamp", func() {
			Expect(df.End()).To(Equal(times[4]))
		})

		It("Duration returns span", func() {
			Expect(df.Duration()).To(Equal(4 * 24 * time.Hour))
		})

		It("Len returns timestamp count", func() {
			Expect(df.Len()).To(Equal(5))
		})

		It("ColCount returns assets * metrics", func() {
			Expect(df.ColCount()).To(Equal(4))
		})

		It("Value returns most recent value for asset/metric", func() {
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(df.Value(goog, data.Volume)).To(Equal(2800.0))
		})

		It("Value returns NaN for unknown asset", func() {
			unknown := asset.Asset{CompositeFigi: "UNKNOWN", Ticker: "UNK"}
			Expect(math.IsNaN(df.Value(unknown, data.Price))).To(BeTrue())
		})

		It("Value returns NaN for unknown metric", func() {
			Expect(math.IsNaN(df.Value(aapl, data.Metric("Nonexistent")))).To(BeTrue())
		})

		It("ValueAt returns value at specific timestamp", func() {
			Expect(df.ValueAt(aapl, data.Price, times[2])).To(Equal(102.0))
			Expect(df.ValueAt(goog, data.Volume, times[0])).To(Equal(2000.0))
		})

		It("ValueAt returns NaN for missing timestamp", func() {
			missing := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			Expect(math.IsNaN(df.ValueAt(aapl, data.Price, missing))).To(BeTrue())
		})

		It("ValueAt returns NaN for unknown asset", func() {
			unknown := asset.Asset{CompositeFigi: "UNKNOWN", Ticker: "UNK"}
			Expect(math.IsNaN(df.ValueAt(unknown, data.Price, times[0]))).To(BeTrue())
		})

		It("ValueAt returns NaN for unknown metric", func() {
			Expect(math.IsNaN(df.ValueAt(aapl, data.Metric("Nonexistent"), times[0]))).To(BeTrue())
		})

		It("Column returns contiguous slice sharing underlying data", func() {
			col := df.Column(aapl, data.Price)
			Expect(col).To(HaveLen(5))
			Expect(col).To(Equal([]float64{100, 101, 102, 103, 104}))

			// Mutating the returned slice affects the original.
			// BeforeEach rebuilds df per It block, so no restoration needed.
			col[0] = 999.0
			Expect(df.ValueAt(aapl, data.Price, times[0])).To(Equal(999.0))
		})

		It("Column returns nil for unknown asset", func() {
			unknown := asset.Asset{CompositeFigi: "UNKNOWN", Ticker: "UNK"}
			Expect(df.Column(unknown, data.Price)).To(BeNil())
		})

		It("Column returns nil for unknown metric", func() {
			Expect(df.Column(aapl, data.Metric("Nonexistent"))).To(BeNil())
		})

		It("At returns single-row frame at timestamp", func() {
			row := df.At(times[2])
			Expect(row.Len()).To(Equal(1))
			Expect(row.Value(aapl, data.Price)).To(Equal(102.0))
			Expect(row.Value(goog, data.Price)).To(Equal(204.0))
		})

		It("At returns empty frame for missing timestamp", func() {
			missing := time.Date(2020, 6, 15, 0, 0, 0, 0, time.UTC)
			row := df.At(missing)
			Expect(row.Len()).To(Equal(0))
		})

		It("Last returns single-row frame at final timestamp", func() {
			last := df.Last()
			Expect(last.Len()).To(Equal(1))
			Expect(last.Value(aapl, data.Price)).To(Equal(104.0))
		})

		It("Last on empty frame returns empty frame", func() {
			empty, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
			Expect(err).NotTo(HaveOccurred())
			last := empty.Last()
			Expect(last.Len()).To(Equal(0))
		})

		It("Copy returns independent deep copy", func() {
			cp := df.Copy()
			Expect(cp.Value(aapl, data.Price)).To(Equal(104.0))

			// Modify original column; copy should be unaffected.
			origCol := df.Column(aapl, data.Price)
			origCol[4] = 999.0
			Expect(cp.Value(aapl, data.Price)).To(Equal(104.0))
		})

		It("Table returns non-empty string", func() {
			Expect(df.Table()).NotTo(BeEmpty())
		})

		It("Table on empty frame returns sentinel string", func() {
			empty, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(empty.Table()).To(Equal("(empty DataFrame)"))
		})

		Context("empty frame", func() {
			var empty *data.DataFrame

			BeforeEach(func() {
				var err error
				empty, err = data.NewDataFrame(nil, nil, nil, data.Daily, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Start returns zero time", func() {
				Expect(empty.Start()).To(Equal(time.Time{}))
			})

			It("End returns zero time", func() {
				Expect(empty.End()).To(Equal(time.Time{}))
			})

			It("Duration returns zero", func() {
				Expect(empty.Duration()).To(Equal(time.Duration(0)))
			})

			It("Len returns 0", func() {
				Expect(empty.Len()).To(Equal(0))
			})

			It("ColCount returns 0", func() {
				Expect(empty.ColCount()).To(Equal(0))
			})
		})

		Context("single-element frame", func() {
			It("works for all accessors", func() {
				t := []time.Time{times[0]}
				single, err := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{42.0}})
				Expect(err).NotTo(HaveOccurred())
				Expect(single.Len()).To(Equal(1))
				Expect(single.Start()).To(Equal(times[0]))
				Expect(single.End()).To(Equal(times[0]))
				Expect(single.Duration()).To(Equal(time.Duration(0)))
				Expect(single.Value(aapl, data.Price)).To(Equal(42.0))
				Expect(single.Last().Value(aapl, data.Price)).To(Equal(42.0))
			})
		})
	})

	Describe("DataFrame Accessors", func() {
		var df2 *data.DataFrame

		BeforeEach(func() {
			t := []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			}
			assets := []asset.Asset{
				{Ticker: "AAPL", CompositeFigi: "BBG000B9XRY4"},
			}
			metrics := []data.Metric{data.AdjClose}
			vals := [][]float64{{100.0, 101.0}}
			var err error
			df2, err = data.NewDataFrame(t, assets, metrics, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Times returns the time axis", func() {
			times := df2.Times()
			Expect(times).To(HaveLen(2))
			Expect(times[0]).To(Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)))
		})

		It("AssetList returns the assets", func() {
			assets := df2.AssetList()
			Expect(assets).To(HaveLen(1))
			Expect(assets[0].Ticker).To(Equal("AAPL"))
		})

		It("MetricList returns the metrics", func() {
			metrics := df2.MetricList()
			Expect(metrics).To(HaveLen(1))
			Expect(metrics[0]).To(Equal(data.AdjClose))
		})
	})

	Describe("Narrowing and filtering", func() {
		It("Assets narrows to requested assets", func() {
			narrowed := df.Assets(aapl)
			Expect(narrowed.ColCount()).To(Equal(2)) // 1 asset * 2 metrics
			Expect(narrowed.Value(aapl, data.Price)).To(Equal(104.0))
		})

		It("Assets with unknown asset returns empty frame", func() {
			unknown := asset.Asset{CompositeFigi: "UNKNOWN", Ticker: "UNK"}
			narrowed := df.Assets(unknown)
			Expect(narrowed.Len()).To(Equal(0))
		})

		It("Assets with mix of known and unknown returns only known", func() {
			unknown := asset.Asset{CompositeFigi: "UNKNOWN", Ticker: "UNK"}
			narrowed := df.Assets(aapl, unknown)
			Expect(narrowed.ColCount()).To(Equal(2)) // 1 asset * 2 metrics
			Expect(narrowed.Value(aapl, data.Price)).To(Equal(104.0))
		})

		It("Assets view shares column data with parent", func() {
			narrowed := df.Assets(aapl)
			col := narrowed.Column(aapl, data.Price)
			col[4] = 999.0
			// View shares underlying column data -- mutation is visible in parent.
			Expect(df.Value(aapl, data.Price)).To(Equal(999.0))
			// Restore for other tests.
			col[4] = 104.0
		})

		It("Assets view is structurally independent", func() {
			narrowed := df.Assets(aapl)
			Expect(narrowed.ColCount()).To(Equal(2)) // 1 asset * 2 metrics
			Expect(df.ColCount()).To(Equal(4))        // 2 assets * 2 metrics
		})

		It("Metrics narrows to requested metrics", func() {
			narrowed := df.Metrics(data.Price)
			Expect(narrowed.ColCount()).To(Equal(2)) // 2 assets * 1 metric
			Expect(narrowed.Value(aapl, data.Price)).To(Equal(104.0))
		})

		It("Metrics with unknown metric returns empty frame", func() {
			narrowed := df.Metrics(data.Metric("Nonexistent"))
			Expect(narrowed.Len()).To(Equal(0))
		})

		It("Metrics view shares column data with parent", func() {
			narrowed := df.Metrics(data.Price)
			col := narrowed.Column(aapl, data.Price)
			col[4] = 999.0
			// View shares underlying column data -- mutation is visible in parent.
			Expect(df.Value(aapl, data.Price)).To(Equal(999.0))
			// Restore for other tests.
			col[4] = 104.0
		})

		It("Metrics with reordered arguments preserves requested order", func() {
			narrowed := df.Metrics(data.Volume, data.Price)
			Expect(narrowed.ColCount()).To(Equal(4)) // 2 assets * 2 metrics
			// Both metrics present with correct values.
			Expect(narrowed.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(narrowed.Value(aapl, data.Volume)).To(Equal(1400.0))
			Expect(narrowed.Value(goog, data.Price)).To(Equal(208.0))
			Expect(narrowed.Value(goog, data.Volume)).To(Equal(2800.0))
		})

		It("Assets with duplicate arguments includes duplicates", func() {
			// The implementation does not deduplicate, so AAPL appears twice.
			narrowed := df.Assets(aapl, aapl)
			Expect(narrowed.ColCount()).To(Equal(4)) // 2 copies of AAPL * 2 metrics
		})

		It("Between returns inclusive time range", func() {
			sub := df.Between(times[1], times[3])
			Expect(sub.Len()).To(Equal(3))
			Expect(sub.Start()).To(Equal(times[1]))
			Expect(sub.End()).To(Equal(times[3]))
		})

		It("Between with no overlap returns empty frame", func() {
			far := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
			sub := df.Between(far, far.AddDate(0, 0, 1))
			Expect(sub.Len()).To(Equal(0))
		})

		It("Between with start == end returns single timestamp", func() {
			sub := df.Between(times[2], times[2])
			Expect(sub.Len()).To(Equal(1))
			Expect(sub.Value(aapl, data.Price)).To(Equal(102.0))
		})

		It("Between view shares column data with parent", func() {
			sub := df.Between(times[0], times[4])
			col := sub.Column(aapl, data.Price)
			col[0] = 999.0
			// View shares underlying column data -- mutation is visible in parent.
			Expect(df.ValueAt(aapl, data.Price, times[0])).To(Equal(999.0))
			// Restore for other tests.
			col[0] = 100.0
		})

		It("Between with boundaries outside the frame returns all rows", func() {
			early := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			late := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
			sub := df.Between(early, late)
			Expect(sub.Len()).To(Equal(5))
			Expect(sub.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(sub.Value(goog, data.Volume)).To(Equal(2800.0))
		})

		It("Filter keeps only matching rows", func() {
			filtered := df.Filter(func(t time.Time, _ *data.DataFrame) bool {
				return t.Day()%2 == 1 // odd days only
			})
			Expect(filtered.Len()).To(Equal(3)) // days 1, 3, 5
		})

		It("Filter rejecting all rows returns empty frame", func() {
			filtered := df.Filter(func(_ time.Time, _ *data.DataFrame) bool {
				return false
			})
			Expect(filtered.Len()).To(Equal(0))
		})

		It("Filter accepting all rows returns equivalent frame", func() {
			filtered := df.Filter(func(_ time.Time, _ *data.DataFrame) bool {
				return true
			})
			Expect(filtered.Len()).To(Equal(df.Len()))
			Expect(filtered.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(filtered.Value(goog, data.Volume)).To(Equal(2800.0))
		})

		It("Filter can inspect row values", func() {
			// Keep only rows where AAPL price > 101.
			filtered := df.Filter(func(_ time.Time, row *data.DataFrame) bool {
				return row.Value(aapl, data.Price) > 101
			})
			Expect(filtered.Len()).To(Equal(3)) // 102, 103, 104
			Expect(filtered.ValueAt(aapl, data.Price, filtered.Start())).To(Equal(102.0))
		})

		It("Drop removes rows containing NaN", func() {
			vals := [][]float64{{1, math.NaN(), 3}}
			t := []time.Time{times[0], times[1], times[2]}
			small, err := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())

			cleaned := small.Drop(math.NaN())
			Expect(cleaned.Len()).To(Equal(2))
		})

		It("Drop with non-NaN sentinel removes matching rows", func() {
			vals := [][]float64{{1, -999, 3, -999, 5}}
			small, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
			cleaned := small.Drop(-999)
			Expect(cleaned.Len()).To(Equal(3))
			col := cleaned.Column(aapl, data.Price)
			Expect(col).To(Equal([]float64{1, 3, 5}))
		})

		It("Drop with no matching values is a no-op", func() {
			cleaned := df.Drop(math.NaN())
			Expect(cleaned.Len()).To(Equal(5))
		})

		It("Drop checks all columns for NaN", func() {
			// AAPL Price has NaN at t=1, GOOG Volume has NaN at t=2.
			vals := [][]float64{
				{1, math.NaN(), 3}, // AAPL Price
				{10, 20, 30},       // GOOG Price
			}
			t := []time.Time{times[0], times[1], times[2]}
			multi, err := data.NewDataFrame(t, []asset.Asset{aapl, goog}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
			cleaned := multi.Drop(math.NaN())
			Expect(cleaned.Len()).To(Equal(2))
		})
	})

	Describe("Mutation", func() {
		It("Insert adds new column for existing asset, new metric", func() {
			newMetric := data.Metric("Beta")
			vals := []float64{0.9, 0.91, 0.92, 0.93, 0.94}
			Expect(df.Insert(aapl, newMetric, vals)).To(Succeed())
			Expect(df.Value(aapl, newMetric)).To(Equal(0.94))
			// Existing data should be intact.
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(df.Value(goog, data.Price)).To(Equal(208.0))
			Expect(df.Value(goog, data.Volume)).To(Equal(2800.0))
		})

		It("Insert adds new column for new asset", func() {
			msft := asset.Asset{CompositeFigi: "MSFT", Ticker: "MSFT"}
			vals := []float64{300, 301, 302, 303, 304}
			Expect(df.Insert(msft, data.Price, vals)).To(Succeed())
			Expect(df.Value(msft, data.Price)).To(Equal(304.0))
			// Existing data should be intact.
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(df.Value(goog, data.Volume)).To(Equal(2800.0))
		})

		It("Insert with new asset AND new metric simultaneously", func() {
			msft := asset.Asset{CompositeFigi: "MSFT", Ticker: "MSFT"}
			newMetric := data.Metric("Beta")
			vals := []float64{1.1, 1.2, 1.3, 1.4, 1.5}
			Expect(df.Insert(msft, newMetric, vals)).To(Succeed())
			Expect(df.Value(msft, newMetric)).To(Equal(1.5))
			// All original data intact.
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(df.Value(aapl, data.Volume)).To(Equal(1400.0))
			Expect(df.Value(goog, data.Price)).To(Equal(208.0))
			Expect(df.Value(goog, data.Volume)).To(Equal(2800.0))
		})

		It("Insert overwrites an existing column", func() {
			vals := []float64{500, 501, 502, 503, 504}
			Expect(df.Insert(aapl, data.Price, vals)).To(Succeed())
			Expect(df.Value(aapl, data.Price)).To(Equal(504.0))
			col := df.Column(aapl, data.Price)
			Expect(col).To(Equal([]float64{500, 501, 502, 503, 504}))
			// Other columns unaffected.
			Expect(df.Value(aapl, data.Volume)).To(Equal(1400.0))
		})

		It("Insert returns error when values length does not match Len()", func() {
			err := df.Insert(aapl, data.Price, []float64{1, 2})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("DataFrame arithmetic", func() {
		var other *data.DataFrame

		BeforeEach(func() {
			otherVals := [][]float64{
				{10, 10, 10, 10, 10},
				{100, 100, 100, 100, 100},
				{20, 20, 20, 20, 20},
				{200, 200, 200, 200, 200},
			}
			var err error
			other, err = data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, data.Daily, otherVals)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Add performs element-wise addition", func() {
			result := df.Add(other)
			Expect(result.Err()).NotTo(HaveOccurred())
			Expect(result.Value(aapl, data.Price)).To(Equal(114.0))
			Expect(result.Value(goog, data.Volume)).To(Equal(3000.0))
		})

		It("Sub performs element-wise subtraction", func() {
			result := df.Sub(other)
			Expect(result.Err()).NotTo(HaveOccurred())
			Expect(result.Value(aapl, data.Price)).To(Equal(94.0))
		})

		It("Mul performs element-wise multiplication", func() {
			result := df.Mul(other)
			Expect(result.Err()).NotTo(HaveOccurred())
			Expect(result.Value(aapl, data.Price)).To(Equal(1040.0))
		})

		It("Div performs element-wise division", func() {
			result := df.Div(other)
			Expect(result.Err()).NotTo(HaveOccurred())
			Expect(result.Value(aapl, data.Price)).To(Equal(10.4))
		})

		It("Div by zero produces Inf", func() {
			zeroVals := data.SlabToColumns(make([]float64, 20), 4, 5)
			zero, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, data.Daily, zeroVals)
			Expect(err).NotTo(HaveOccurred())
			result := df.Div(zero)
			Expect(result.Err()).NotTo(HaveOccurred())
			v := result.Value(aapl, data.Price)
			Expect(math.IsInf(v, 1)).To(BeTrue())
		})

		It("Div zero by zero produces NaN", func() {
			zeroVals := data.SlabToColumns(make([]float64, 20), 4, 5)
			zeroA, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, data.Daily, zeroVals)
			Expect(err).NotTo(HaveOccurred())
			zeroB, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, data.Daily, data.SlabToColumns(make([]float64, 20), 4, 5))
			Expect(err).NotTo(HaveOccurred())
			result := zeroA.Div(zeroB)
			Expect(result.Err()).NotTo(HaveOccurred())
			Expect(math.IsNaN(result.Value(aapl, data.Price))).To(BeTrue())
		})

		It("arithmetic with partial overlap returns intersection only", func() {
			// other has only AAPL with only Price.
			partialVals := [][]float64{{1, 1, 1, 1, 1}}
			partial, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, partialVals)
			Expect(err).NotTo(HaveOccurred())
			result := df.Add(partial)
			Expect(result.Err()).NotTo(HaveOccurred())
			Expect(result.ColCount()).To(Equal(1)) // only AAPL/Price
			Expect(result.Value(aapl, data.Price)).To(Equal(105.0))
			// GOOG should not be present.
			Expect(math.IsNaN(result.Value(goog, data.Price))).To(BeTrue())
		})

		It("arithmetic with no overlap returns empty frame", func() {
			msft := asset.Asset{CompositeFigi: "MSFT", Ticker: "MSFT"}
			noOverlap, err := data.NewDataFrame(times, []asset.Asset{msft}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
			Expect(err).NotTo(HaveOccurred())
			result := df.Add(noOverlap)
			Expect(result.Err()).NotTo(HaveOccurred())
			Expect(result.Len()).To(Equal(0))
		})

		It("arithmetic returns error on timestamp count mismatch", func() {
			shortTimes := times[:3]
			short, err := data.NewDataFrame(shortTimes, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3}})
			Expect(err).NotTo(HaveOccurred())
			result := df.Add(short)
			Expect(result.Err()).To(HaveOccurred())
		})

		It("arithmetic returns error on timestamp value mismatch", func() {
			// Same count but different dates.
			offsetTimes := make([]time.Time, 5)
			for i := range offsetTimes {
				offsetTimes[i] = times[i].AddDate(1, 0, 0) // shift by 1 year
			}
			other2, err := data.NewDataFrame(offsetTimes, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, data.Daily, data.SlabToColumns(make([]float64, 20), 4, 5))
			Expect(err).NotTo(HaveOccurred())
			result := df.Add(other2)
			Expect(result.Err()).To(HaveOccurred())
			Expect(result.Err().Error()).To(ContainSubstring("timestamp mismatch"))
		})

		It("arithmetic does not modify original frames", func() {
			origAAPL := df.Value(aapl, data.Price)
			origOther := other.Value(aapl, data.Price)
			_ = df.Add(other)
			Expect(df.Value(aapl, data.Price)).To(Equal(origAAPL))
			Expect(other.Value(aapl, data.Price)).To(Equal(origOther))
		})

		It("NaN propagates through element-wise addition", func() {
			nanVals := [][]float64{
				{1, math.NaN(), 3, 4, 5},
				{10, 20, 30, 40, 50},
				{100, 200, 300, 400, 500},
				{1000, 2000, 3000, 4000, 5000},
			}
			nanFrame, err := data.NewDataFrame(times, []asset.Asset{aapl, goog},
				[]data.Metric{data.Price, data.Volume}, data.Daily, nanVals)
			Expect(err).NotTo(HaveOccurred())
			result := df.Add(nanFrame)
			Expect(result.Err()).NotTo(HaveOccurred())
			col := result.Column(aapl, data.Price)
			// NaN + 101 = NaN at index 1.
			Expect(math.IsNaN(col[1])).To(BeTrue())
			// Non-NaN positions compute normally.
			Expect(col[0]).To(Equal(101.0)) // 100 + 1
			Expect(col[2]).To(Equal(105.0)) // 102 + 3
		})
	})

	Describe("Scalar arithmetic", func() {
		It("AddScalar adds constant to all values", func() {
			result := df.AddScalar(10)
			Expect(result.Value(aapl, data.Price)).To(Equal(114.0))
			Expect(result.Value(goog, data.Price)).To(Equal(218.0))
		})

		It("SubScalar subtracts constant from all values", func() {
			result := df.SubScalar(4)
			Expect(result.Value(aapl, data.Price)).To(Equal(100.0))
		})

		It("MulScalar multiplies all values", func() {
			result := df.MulScalar(2)
			Expect(result.Value(aapl, data.Price)).To(Equal(208.0))
		})

		It("DivScalar divides all values", func() {
			result := df.DivScalar(2)
			Expect(result.Value(aapl, data.Price)).To(Equal(52.0))
		})

		It("scalar ops do not modify original frame", func() {
			orig := df.Value(aapl, data.Price)
			_ = df.AddScalar(1000)
			Expect(df.Value(aapl, data.Price)).To(Equal(orig))
		})

		It("DivScalar by zero produces Inf", func() {
			result := df.DivScalar(0)
			Expect(math.IsInf(result.Value(aapl, data.Price), 1)).To(BeTrue())
		})
	})

	Describe("Aggregation", func() {
		It("MaxAcrossAssets returns max across assets per timestamp", func() {
			maxDF := df.MaxAcrossAssets()
			col := maxDF.Column(asset.Asset{Ticker: "MAX"}, data.Price)
			Expect(col).To(Equal([]float64{200, 202, 204, 206, 208}))
		})

		It("MinAcrossAssets returns min across assets per timestamp", func() {
			minDF := df.MinAcrossAssets()
			col := minDF.Column(asset.Asset{Ticker: "MIN"}, data.Price)
			Expect(col).To(Equal([]float64{100, 101, 102, 103, 104}))
		})

		It("MaxAcrossAssets on single-asset frame returns same values", func() {
			single := df.Assets(aapl)
			maxDF := single.MaxAcrossAssets()
			Expect(maxDF.Len()).To(Equal(5))
			col := maxDF.Column(asset.Asset{Ticker: "MAX"}, data.Price)
			Expect(col).To(Equal([]float64{100, 101, 102, 103, 104}))
		})

		It("MinAcrossAssets on single-asset frame returns same values", func() {
			single := df.Assets(aapl)
			minDF := single.MinAcrossAssets()
			col := minDF.Column(asset.Asset{Ticker: "MIN"}, data.Price)
			Expect(col).To(Equal([]float64{100, 101, 102, 103, 104}))
		})

		It("MaxAcrossAssets/MinAcrossAssets aggregates across all metrics", func() {
			maxDF := df.MaxAcrossAssets()
			volCol := maxDF.Column(asset.Asset{Ticker: "MAX"}, data.Volume)
			Expect(volCol).To(Equal([]float64{2000, 2200, 2400, 2600, 2800}))
		})

		It("IdxMaxAcrossAssets returns asset with max value per timestamp", func() {
			result := df.IdxMaxAcrossAssets()
			Expect(result).To(HaveLen(5))
			for _, a := range result {
				Expect(a.CompositeFigi).To(Equal("GOOG"))
			}
		})

		It("IdxMaxAcrossAssets with alternating maxes", func() {
			// AAPL > GOOG on even indices, GOOG > AAPL on odd.
			vals := [][]float64{
				{10, 1, 10, 1, 10}, // AAPL Price
				{1, 10, 1, 10, 1},  // GOOG Price
			}
			altDF, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
			result := altDF.IdxMaxAcrossAssets()
			Expect(result[0].CompositeFigi).To(Equal("AAPL"))
			Expect(result[1].CompositeFigi).To(Equal("GOOG"))
			Expect(result[2].CompositeFigi).To(Equal("AAPL"))
			Expect(result[3].CompositeFigi).To(Equal("GOOG"))
			Expect(result[4].CompositeFigi).To(Equal("AAPL"))
		})
	})

	Describe("CountWhere", func() {
		It("counts assets matching predicate at each timestep", func() {
			// AAPL prices: 100, 101, 102, 103, 104
			// GOOG prices: 200, 202, 204, 206, 208
			result := df.CountWhere(data.Price, func(v float64) bool {
				return v > 150
			})

			synth := asset.Asset{Ticker: "COUNT"}
			col := result.Column(synth, data.Count)
			// Only GOOG > 150 at every timestep
			Expect(col).To(Equal([]float64{1, 1, 1, 1, 1}))
		})

		It("counts all assets when all match", func() {
			result := df.CountWhere(data.Price, func(v float64) bool {
				return v > 0
			})

			synth := asset.Asset{Ticker: "COUNT"}
			col := result.Column(synth, data.Count)
			Expect(col).To(Equal([]float64{2, 2, 2, 2, 2}))
		})

		It("returns zero when no assets match", func() {
			result := df.CountWhere(data.Price, func(v float64) bool {
				return v > 1000
			})

			synth := asset.Asset{Ticker: "COUNT"}
			col := result.Column(synth, data.Count)
			Expect(col).To(Equal([]float64{0, 0, 0, 0, 0}))
		})

		It("handles NaN values in predicate", func() {
			vals := [][]float64{
				{math.NaN(), 5, 10}, // AAPL
				{3, math.NaN(), 7},  // GOOG
			}
			nanTimes := times[:3]
			nanDF, err := data.NewDataFrame(nanTimes, []asset.Asset{aapl, goog}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())

			result := nanDF.CountWhere(data.Price, func(v float64) bool {
				return math.IsNaN(v) || v <= 0
			})

			synth := asset.Asset{Ticker: "COUNT"}
			col := result.Column(synth, data.Count)
			// t0: AAPL=NaN(match), GOOG=3(no) => 1
			// t1: AAPL=5(no), GOOG=NaN(match) => 1
			// t2: AAPL=10(no), GOOG=7(no) => 0
			Expect(col).To(Equal([]float64{1, 1, 0}))
		})

		It("returns error DataFrame when metric not found", func() {
			result := df.CountWhere("nonexistent", func(v float64) bool {
				return true
			})

			Expect(result.Err()).To(HaveOccurred())
		})
	})

	Describe("Transforms", func() {
		It("Pct computes 1-period percent change by default", func() {
			result := df.Pct()
			col := result.Column(aapl, data.Price)
			Expect(math.IsNaN(col[0])).To(BeTrue())
			Expect(col[1]).To(BeNumerically("~", 0.01, 1e-10))
		})

		It("Pct(3) computes 3-period percent change", func() {
			result := df.Pct(3)
			col := result.Column(aapl, data.Price)
			for i := 0; i < 3; i++ {
				Expect(math.IsNaN(col[i])).To(BeTrue())
			}
			// (103-100)/100 = 0.03
			Expect(col[3]).To(BeNumerically("~", 0.03, 1e-10))
		})

		It("Pct with zero denominator produces Inf", func() {
			vals := [][]float64{{0, 1, 2, 3, 4}}
			zdf, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
			result := zdf.Pct()
			Expect(math.IsInf(result.Column(aapl, data.Price)[1], 1)).To(BeTrue())
		})

		It("Pct does not modify original", func() {
			orig := df.Column(aapl, data.Price)[0]
			_ = df.Pct()
			Expect(df.Column(aapl, data.Price)[0]).To(Equal(orig))
		})

		It("Diff computes first difference", func() {
			result := df.Diff()
			col := result.Column(aapl, data.Price)
			Expect(math.IsNaN(col[0])).To(BeTrue())
			Expect(col[1]).To(Equal(1.0))
			Expect(col[4]).To(Equal(1.0))
		})

		It("Diff on single-element frame", func() {
			single, err := data.NewDataFrame(times[:1], []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{42.0}})
			Expect(err).NotTo(HaveOccurred())
			result := single.Diff()
			col := result.Column(aapl, data.Price)
			Expect(math.IsNaN(col[0])).To(BeTrue())
		})

		It("Log computes natural logarithm", func() {
			result := df.Log()
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(BeNumerically("~", math.Log(100), 1e-10))
			Expect(col[4]).To(BeNumerically("~", math.Log(104), 1e-10))
		})

		It("CumSum computes cumulative sum", func() {
			result := df.CumSum()
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(100.0))
			Expect(col[1]).To(Equal(201.0))
			Expect(col[2]).To(Equal(303.0))
			Expect(col[3]).To(Equal(406.0))
			Expect(col[4]).To(Equal(510.0))
		})

		It("CumSum transforms all columns", func() {
			result := df.CumSum()
			googCol := result.Column(goog, data.Price)
			Expect(googCol[0]).To(Equal(200.0))
			Expect(googCol[1]).To(Equal(402.0))
		})

		It("Shift(1) shifts forward, fills NaN", func() {
			result := df.Shift(1)
			col := result.Column(aapl, data.Price)
			Expect(math.IsNaN(col[0])).To(BeTrue())
			Expect(col[1]).To(Equal(100.0))
			Expect(col[4]).To(Equal(103.0))
		})

		It("Shift(-1) shifts backward, fills NaN", func() {
			result := df.Shift(-1)
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(101.0))
			Expect(math.IsNaN(col[4])).To(BeTrue())
		})

		It("Shift(0) is identity", func() {
			result := df.Shift(0)
			col := result.Column(aapl, data.Price)
			Expect(col).To(Equal([]float64{100, 101, 102, 103, 104}))
		})

		It("Shift(n) where n >= Len produces all NaN", func() {
			result := df.Shift(10)
			col := result.Column(aapl, data.Price)
			for _, v := range col {
				Expect(math.IsNaN(v)).To(BeTrue())
			}
		})

		It("Shift(-n) where abs(n) >= Len produces all NaN", func() {
			result := df.Shift(-10)
			col := result.Column(aapl, data.Price)
			for _, v := range col {
				Expect(math.IsNaN(v)).To(BeTrue())
			}
		})

		It("Pct(0) computes zero change for nonzero values", func() {
			result := df.Pct(0)
			col := result.Column(aapl, data.Price)
			// (col[i] - col[i]) / col[i] = 0.0 for all nonzero values.
			for _, v := range col {
				Expect(v).To(BeNumerically("~", 0.0, 1e-10))
			}
		})

		It("Pct(n) where n >= Len produces all NaN", func() {
			result := df.Pct(10)
			col := result.Column(aapl, data.Price)
			for _, v := range col {
				Expect(math.IsNaN(v)).To(BeTrue())
			}
		})

		It("Log of zero produces -Inf", func() {
			vals := [][]float64{{0, 1, 2, 3, 4}}
			zdf, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
			result := zdf.Log()
			col := result.Column(aapl, data.Price)
			Expect(math.IsInf(col[0], -1)).To(BeTrue())
		})

		It("Log of negative value produces NaN", func() {
			vals := [][]float64{{-1, 1, 2, 3, 4}}
			zdf, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
			result := zdf.Log()
			col := result.Column(aapl, data.Price)
			Expect(math.IsNaN(col[0])).To(BeTrue())
		})

		It("Diff on frame with zero timestamps does not panic", func() {
			// A frame with assets and metrics but no timestamps has T=0.
			noTime, err := data.NewDataFrame(nil, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{}})
			Expect(err).NotTo(HaveOccurred())
			Expect(func() {
				_ = noTime.Diff()
			}).NotTo(Panic())
		})

		It("Shift does not modify original", func() {
			orig := df.Column(aapl, data.Price)[0]
			_ = df.Shift(1)
			Expect(df.Column(aapl, data.Price)[0]).To(Equal(orig))
		})

		It("CumSum does not modify original", func() {
			orig := df.Column(aapl, data.Price)[0]
			_ = df.CumSum()
			Expect(df.Column(aapl, data.Price)[0]).To(Equal(orig))
		})

		It("Diff does not modify original", func() {
			orig := df.Column(aapl, data.Price)[0]
			_ = df.Diff()
			Expect(df.Column(aapl, data.Price)[0]).To(Equal(orig))
		})

		It("Log does not modify original", func() {
			orig := df.Column(aapl, data.Price)[0]
			_ = df.Log()
			Expect(df.Column(aapl, data.Price)[0]).To(Equal(orig))
		})
	})

	Describe("Downsample", func() {
		var weeklyDF *data.DataFrame
		var aapl asset.Asset

		BeforeEach(func() {
			aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t := make([]time.Time, 10)
			col := make([]float64, 10)
			for i := range t {
				t[i] = base.AddDate(0, 0, i)
				col[i] = float64(i + 1)
			}
			var err error
			weeklyDF, err = data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{col})
			Expect(err).NotTo(HaveOccurred())
		})

		It("Last picks last value per week", func() {
			result := weeklyDF.Downsample(data.Weekly).Last()
			Expect(result.Len()).To(Equal(2))
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(7.0))
			Expect(col[1]).To(Equal(10.0))
		})

		It("First picks first value per week", func() {
			result := weeklyDF.Downsample(data.Weekly).First()
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(1.0))
			Expect(col[1]).To(Equal(8.0))
		})

		It("Mean computes mean per week", func() {
			result := weeklyDF.Downsample(data.Weekly).Mean()
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(4.0))
			Expect(col[1]).To(Equal(9.0))
		})

		It("Max picks max per week", func() {
			result := weeklyDF.Downsample(data.Weekly).Max()
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(7.0))
			Expect(col[1]).To(Equal(10.0))
		})

		It("Min picks min per week", func() {
			result := weeklyDF.Downsample(data.Weekly).Min()
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(1.0))
			Expect(col[1]).To(Equal(8.0))
		})

		It("Sum sums values per month", func() {
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t := make([]time.Time, 60)
			ones := make([]float64, 60)
			for i := range t {
				t[i] = base.AddDate(0, 0, i)
				ones[i] = 1.0
			}
			monthDF, err := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{ones})
			Expect(err).NotTo(HaveOccurred())
			result := monthDF.Downsample(data.Monthly).Sum()
			Expect(result.Len()).To(Equal(2))
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(31.0))
			Expect(col[1]).To(Equal(29.0))
		})

		It("Std uses sample (N-1) denominator", func() {
			// [1,2,3,4,5,6,7] in one week: mean=4, sum sq diffs=28, var=28/6, std=sqrt(28/6)
			result := weeklyDF.Downsample(data.Weekly).Std()
			col := result.Column(aapl, data.Price)
			expectedStd := math.Sqrt(28.0 / 6.0)
			Expect(col[0]).To(BeNumerically("~", expectedStd, 1e-12))
		})

		It("Variance uses sample (N-1) denominator", func() {
			result := weeklyDF.Downsample(data.Weekly).Variance()
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(BeNumerically("~", 28.0/6.0, 1e-12))
		})

		It("on empty frame returns empty", func() {
			empty, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
			Expect(err).NotTo(HaveOccurred())
			result := empty.Downsample(data.Weekly).Last()
			Expect(result.Len()).To(Equal(0))
		})
	})

	Describe("Upsample", func() {
		var monthlyDF *data.DataFrame
		var aapl asset.Asset

		BeforeEach(func() {
			aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
			t := []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
			}
			vals := [][]float64{{100, 200, 300}}
			var err error
			monthlyDF, err = data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("ForwardFill", func() {
			It("carries last known value forward to fill gaps", func() {
				result := monthlyDF.Upsample(data.Weekly).ForwardFill()
				// Should have weekly timestamps between Jan 1 and Mar 1
				Expect(result.Len()).To(BeNumerically(">", 3))

				// First value should be 100 (Jan 1)
				col := result.Column(aapl, data.Price)
				Expect(col[0]).To(Equal(100.0))

				// Values between Jan and Feb should be forward-filled as 100
				for i := 0; i < len(col); i++ {
					t := result.Times()[i]
					if t.Before(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)) {
						Expect(col[i]).To(Equal(100.0))
					}
				}
			})
		})

		Describe("BackFill", func() {
			It("uses next known value to fill gaps", func() {
				result := monthlyDF.Upsample(data.Weekly).BackFill()
				col := result.Column(aapl, data.Price)
				times := result.Times()

				// Values before Feb 1 (but after first) should be back-filled with 200
				for i := 1; i < len(col); i++ {
					if times[i].Before(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)) {
						Expect(col[i]).To(Equal(200.0))
					}
				}
			})
		})

		Describe("Interpolate", func() {
			It("linearly interpolates between known values", func() {
				// Use daily with simple data for easy verification
				t := []time.Time{
					time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC),
				}
				vals := [][]float64{{0, 100}}
				simple, err := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)
				Expect(err).NotTo(HaveOccurred())
				result := simple.Upsample(data.Daily).Interpolate()

				col := result.Column(aapl, data.Price)
				Expect(col[0]).To(Equal(0.0))
				Expect(col[len(col)-1]).To(Equal(100.0))
				// Middle values should be linearly interpolated
				Expect(result.Len()).To(Equal(11)) // Jan 1 through Jan 11
				Expect(col[5]).To(BeNumerically("~", 50.0, 1e-12))
			})
		})

		It("on empty frame returns empty", func() {
			empty, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
			Expect(err).NotTo(HaveOccurred())
			result := empty.Upsample(data.Daily).ForwardFill()
			Expect(result.Len()).To(Equal(0))
		})
	})

	Describe("Column-wise stats", func() {
		var statsDF *data.DataFrame
		var spy, efa asset.Asset

		BeforeEach(func() {
			spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
			efa = asset.Asset{CompositeFigi: "EFA", Ticker: "EFA"}
			t := []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
			}
			vals := [][]float64{{1, 2, 3, 4}, {10, 20, 30, 40}}
			var err error
			statsDF, err = data.NewDataFrame(t, []asset.Asset{spy, efa}, []data.Metric{data.Price}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("Mean", func() {
			It("returns single-row DataFrame with mean of each column", func() {
				result := statsDF.Mean()
				Expect(result.Len()).To(Equal(1))
				Expect(result.Value(spy, data.Price)).To(BeNumerically("~", 2.5, 1e-12))
				Expect(result.Value(efa, data.Price)).To(BeNumerically("~", 25.0, 1e-12))
			})

			It("preserves asset and metric dimensions", func() {
				result := statsDF.Mean()
				Expect(result.AssetList()).To(HaveLen(2))
				Expect(result.MetricList()).To(HaveLen(1))
			})

			It("returns empty DataFrame for empty input", func() {
				empty, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(empty.Mean().Len()).To(Equal(0))
			})
		})

		Describe("Sum", func() {
			It("returns single-row DataFrame with sum of each column", func() {
				result := statsDF.Sum()
				Expect(result.Value(spy, data.Price)).To(BeNumerically("~", 10.0, 1e-12))
				Expect(result.Value(efa, data.Price)).To(BeNumerically("~", 100.0, 1e-12))
			})
		})

		Describe("Max", func() {
			It("returns single-row DataFrame with max of each column over time", func() {
				result := statsDF.Max()
				Expect(result.Value(spy, data.Price)).To(BeNumerically("~", 4.0, 1e-12))
				Expect(result.Value(efa, data.Price)).To(BeNumerically("~", 40.0, 1e-12))
			})
		})

		Describe("Min", func() {
			It("returns single-row DataFrame with min of each column over time", func() {
				result := statsDF.Min()
				Expect(result.Value(spy, data.Price)).To(BeNumerically("~", 1.0, 1e-12))
				Expect(result.Value(efa, data.Price)).To(BeNumerically("~", 10.0, 1e-12))
			})
		})

		Describe("Variance", func() {
			It("returns single-row DataFrame with sample variance (N-1) of each column", func() {
				result := statsDF.Variance()
				// SPY: [1,2,3,4], mean=2.5, sum sq diffs=5, var=5/3
				Expect(result.Value(spy, data.Price)).To(BeNumerically("~", 5.0/3.0, 1e-12))
			})

			It("returns 0 for single timestamp", func() {
				t := []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
				single, err := data.NewDataFrame(t, []asset.Asset{spy}, []data.Metric{data.Price}, data.Daily, [][]float64{{42}})
				Expect(err).NotTo(HaveOccurred())
				Expect(single.Variance().Value(spy, data.Price)).To(BeNumerically("==", 0))
			})
		})

		Describe("Std", func() {
			It("returns single-row DataFrame with sample std (N-1) of each column", func() {
				result := statsDF.Std()
				expectedVariance := 5.0 / 3.0
				Expect(result.Value(spy, data.Price)).To(BeNumerically("~", math.Sqrt(expectedVariance), 1e-12))
			})

			It("returns 0 for single timestamp", func() {
				t := []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
				single, err := data.NewDataFrame(t, []asset.Asset{spy}, []data.Metric{data.Price}, data.Daily, [][]float64{{42}})
				Expect(err).NotTo(HaveOccurred())
				Expect(single.Std().Value(spy, data.Price)).To(BeNumerically("==", 0))
			})
		})

		Describe("NaN propagation", func() {
			It("propagates NaN through Mean", func() {
				t := []time.Time{
					time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				}
				nanDF, err := data.NewDataFrame(t, []asset.Asset{spy}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, math.NaN()}})
				Expect(err).NotTo(HaveOccurred())
				Expect(math.IsNaN(nanDF.Mean().Value(spy, data.Price))).To(BeTrue())
			})

			It("propagates NaN through Sum", func() {
				t := []time.Time{
					time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				}
				nanDF, err := data.NewDataFrame(t, []asset.Asset{spy}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, math.NaN()}})
				Expect(err).NotTo(HaveOccurred())
				Expect(math.IsNaN(nanDF.Sum().Value(spy, data.Price))).To(BeTrue())
			})

			It("propagates NaN through Variance", func() {
				t := []time.Time{
					time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				}
				nanDF, err := data.NewDataFrame(t, []asset.Asset{spy}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, math.NaN()}})
				Expect(err).NotTo(HaveOccurred())
				Expect(math.IsNaN(nanDF.Variance().Value(spy, data.Price))).To(BeTrue())
			})
		})
	})

	Describe("Extensibility", func() {
		It("Apply transforms each column", func() {
			result := df.Apply(func(col []float64) []float64 {
				out := make([]float64, len(col))
				for i, v := range col {
					out[i] = v * 2
				}
				return out
			})
			Expect(result.Value(aapl, data.Price)).To(Equal(208.0))
			Expect(result.Value(goog, data.Volume)).To(Equal(5600.0))
		})

		It("Apply transforms all columns", func() {
			result := df.Apply(func(col []float64) []float64 {
				out := make([]float64, len(col))
				for i := range col {
					out[i] = 1.0
				}
				return out
			})
			// Every value should be 1.0.
			Expect(result.Value(aapl, data.Price)).To(Equal(1.0))
			Expect(result.Value(aapl, data.Volume)).To(Equal(1.0))
			Expect(result.Value(goog, data.Price)).To(Equal(1.0))
			Expect(result.Value(goog, data.Volume)).To(Equal(1.0))
		})

		It("Apply does not modify original frame", func() {
			orig := df.Value(aapl, data.Price)
			_ = df.Apply(func(col []float64) []float64 {
				out := make([]float64, len(col))
				for i := range col {
					out[i] = 0
				}
				return out
			})
			Expect(df.Value(aapl, data.Price)).To(Equal(orig))
		})

		It("Reduce collapses each column to single value", func() {
			result := df.Reduce(func(col []float64) float64 {
				sum := 0.0
				for _, v := range col {
					sum += v
				}
				return sum
			})
			Expect(result.Len()).To(Equal(1))
			// Sum of AAPL prices: 100+101+102+103+104 = 510
			Expect(result.Value(aapl, data.Price)).To(Equal(510.0))
		})

		It("Reduce collapses all columns", func() {
			result := df.Reduce(func(col []float64) float64 {
				return col[0]
			})
			Expect(result.Value(aapl, data.Price)).To(Equal(100.0))
			Expect(result.Value(aapl, data.Volume)).To(Equal(1000.0))
			Expect(result.Value(goog, data.Price)).To(Equal(200.0))
			Expect(result.Value(goog, data.Volume)).To(Equal(2000.0))
		})

		It("Reduce timestamp is the last timestamp", func() {
			result := df.Reduce(func(col []float64) float64 { return 0 })
			Expect(result.End()).To(Equal(times[4]))
		})

		It("Reduce on empty frame returns empty frame", func() {
			empty, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
			Expect(err).NotTo(HaveOccurred())
			result := empty.Reduce(func(col []float64) float64 { return 0 })
			Expect(result.Len()).To(Equal(0))
			Expect(result.ColCount()).To(Equal(0))
		})

		It("Apply on empty frame returns empty frame", func() {
			empty, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
			Expect(err).NotTo(HaveOccurred())
			result := empty.Apply(func(col []float64) []float64 {
				return col
			})
			Expect(result.Len()).To(Equal(0))
			Expect(result.ColCount()).To(Equal(0))
		})
	})

	Describe("Composite keys", func() {
		It("CompositeAsset joins two assets with colon separator", func() {
			spyAsset := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
			efaAsset := asset.Asset{CompositeFigi: "EFA", Ticker: "EFA"}
			result := data.CompositeAsset(spyAsset, efaAsset)
			Expect(result.CompositeFigi).To(Equal("SPY:EFA"))
			Expect(result.Ticker).To(Equal("SPY:EFA"))
		})

		It("CompositeMetric joins two metrics with colon separator", func() {
			result := data.CompositeMetric(data.Price, data.Volume)
			Expect(string(result)).To(Equal("Price:Volume"))
		})
	})

	Describe("Covariance", func() {
		var covDF *data.DataFrame
		var spy, efa, voo asset.Asset

		BeforeEach(func() {
			spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
			efa = asset.Asset{CompositeFigi: "EFA", Ticker: "EFA"}
			voo = asset.Asset{CompositeFigi: "VOO", Ticker: "VOO"}
			t := []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
			}
			// SPY.Price: 1,2,3,4,5   EFA.Price: 2,4,6,8,10   VOO.Price: 10,9,8,7,6
			// SPY.Volume: 100,200,300,400,500  EFA.Volume: 50,50,50,50,50  VOO.Volume: 10,20,30,40,50
			vals := [][]float64{
				{1, 2, 3, 4, 5},           // SPY.Price
				{100, 200, 300, 400, 500},  // SPY.Volume
				{2, 4, 6, 8, 10},           // EFA.Price
				{50, 50, 50, 50, 50},       // EFA.Volume
				{10, 9, 8, 7, 6},           // VOO.Price
				{10, 20, 30, 40, 50},       // VOO.Volume
			}
			var err error
			covDF, err = data.NewDataFrame(t, []asset.Asset{spy, efa, voo}, []data.Metric{data.Price, data.Volume}, data.Daily, vals)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("two assets (per-metric covariance)", func() {
			It("computes covariance between SPY and EFA for each metric", func() {
				result := covDF.Covariance(spy, efa)
				Expect(result.Len()).To(Equal(1))

				composite := data.CompositeAsset(spy, efa)
				// SPY.Price=[1,2,3,4,5], EFA.Price=[2,4,6,8,10] => perfect linear, cov = 5.0
				Expect(result.Value(composite, data.Price)).To(BeNumerically("~", 5.0, 1e-12))

				// SPY.Volume=[100..500], EFA.Volume=[50,50,50,50,50] => cov = 0
				Expect(result.Value(composite, data.Volume)).To(BeNumerically("~", 0.0, 1e-12))
			})
		})

		Context("three assets (all unique pairs)", func() {
			It("returns N*(N-1)/2 composite assets", func() {
				result := covDF.Covariance(spy, efa, voo)
				Expect(result.AssetList()).To(HaveLen(3)) // SPY:EFA, SPY:VOO, EFA:VOO
			})

			It("computes correct covariance for each pair", func() {
				result := covDF.Covariance(spy, efa, voo)
				spyEfa := data.CompositeAsset(spy, efa)
				spyVoo := data.CompositeAsset(spy, voo)

				// SPY.Price and EFA.Price: perfect positive correlation, cov = 5.0
				Expect(result.Value(spyEfa, data.Price)).To(BeNumerically("~", 5.0, 1e-12))

				// SPY.Price=[1,2,3,4,5] and VOO.Price=[10,9,8,7,6]: perfect negative, cov = -2.5
				Expect(result.Value(spyVoo, data.Price)).To(BeNumerically("~", -2.5, 1e-12))
			})
		})

		Context("single asset (cross-metric covariance)", func() {
			It("returns composite metric keys for each metric pair", func() {
				result := covDF.Covariance(spy)
				// SPY has Price and Volume => one pair: Price:Volume
				compositeMetric := data.CompositeMetric(data.Price, data.Volume)
				Expect(result.MetricList()).To(ContainElement(compositeMetric))
			})

			It("computes covariance between metrics for that asset", func() {
				result := covDF.Covariance(spy)
				compositeMetric := data.CompositeMetric(data.Price, data.Volume)
				// SPY.Price=[1,2,3,4,5], SPY.Volume=[100,200,300,400,500]
				// Both have identical shape (linear), cov = 250.0
				Expect(result.Value(spy, compositeMetric)).To(BeNumerically("~", 250.0, 1e-12))
			})
		})

		Context("edge cases", func() {
			It("returns empty DataFrame for zero assets", func() {
				result := covDF.Covariance()
				Expect(result.Len()).To(Equal(0))
			})

			It("returns 0 for fewer than 2 timestamps", func() {
				t := []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
				short, err := data.NewDataFrame(t, []asset.Asset{spy, efa}, []data.Metric{data.Price}, data.Daily, [][]float64{{1}, {2}})
				Expect(err).NotTo(HaveOccurred())
				result := short.Covariance(spy, efa)
				composite := data.CompositeAsset(spy, efa)
				Expect(result.Value(composite, data.Price)).To(BeNumerically("==", 0))
			})

			It("excludes pairs involving missing assets", func() {
				missing := asset.Asset{CompositeFigi: "MISSING", Ticker: "MISSING"}
				result := covDF.Covariance(spy, missing, efa)
				// SPY:MISSING and MISSING:EFA excluded, only SPY:EFA remains
				Expect(result.AssetList()).To(HaveLen(1))
				composite := data.CompositeAsset(spy, efa)
				Expect(result.Value(composite, data.Price)).To(BeNumerically("~", 5.0, 1e-12))
			})
		})
	})

	Describe("Error accumulation", func() {
		It("Err returns nil on a healthy DataFrame", func() {
			df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
			Expect(err).NotTo(HaveOccurred())
			Expect(df.Err()).To(BeNil())
		})

		It("propagates error through Add chain", func() {
			df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
			short, _ := data.NewDataFrame(times[:3], []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3}})

			result := df.Add(short).AddScalar(1)
			Expect(result.Err()).To(HaveOccurred())
			Expect(result.Err().Error()).To(ContainSubstring("timestamp count mismatch"))
		})

		It("propagates error through mixed chain", func() {
			df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
			short, _ := data.NewDataFrame(times[:3], []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3}})

			result := df.Add(short).MulScalar(2).Pct().Last()
			Expect(result.Err()).To(HaveOccurred())
		})

		It("successful chain has nil Err", func() {
			df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
			other, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{10, 20, 30, 40, 50}})

			result := df.Add(other).MulScalar(2)
			Expect(result.Err()).To(BeNil())
			Expect(result.Value(aapl, data.Price)).To(Equal(110.0)) // (5+50)*2
		})

		It("propagates error through Rolling", func() {
			df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
			short, _ := data.NewDataFrame(times[:3], []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3}})

			result := df.Add(short).Rolling(3).Mean()
			Expect(result.Err()).To(HaveOccurred())
		})

		It("propagates other's error through Add", func() {
			df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
			short, _ := data.NewDataFrame(times[:3], []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3}})

			errDF := df.Add(short) // has error
			good, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})

			result := good.Add(errDF) // other has error
			Expect(result.Err()).To(HaveOccurred())
		})

		It("returns NaN from Value on error DataFrame", func() {
			errDF := data.WithErr(fmt.Errorf("test error"))
			Expect(math.IsNaN(errDF.Value(aapl, data.Price))).To(BeTrue())
		})

		It("returns nil from Column on error DataFrame", func() {
			errDF := data.WithErr(fmt.Errorf("test error"))
			Expect(errDF.Column(aapl, data.Price)).To(BeNil())
		})

		It("returns 0 from Len on error DataFrame", func() {
			errDF := data.WithErr(fmt.Errorf("test error"))
			Expect(errDF.Len()).To(Equal(0))
		})
	})

	Describe("RenameMetric", func() {
		It("renames a metric successfully", func() {
			df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
			Expect(err).NotTo(HaveOccurred())

			result := df.RenameMetric(data.Price, data.Metric("Signal"))
			Expect(result.Err()).To(BeNil())
			Expect(result.MetricList()).To(Equal([]data.Metric{data.Metric("Signal")}))
			Expect(result.Value(aapl, data.Metric("Signal"))).To(Equal(5.0))
		})

		It("returns error when old metric not found", func() {
			df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})

			result := df.RenameMetric(data.Volume, data.Metric("Signal"))
			Expect(result.Err()).To(HaveOccurred())
			Expect(result.Err().Error()).To(ContainSubstring("Volume"))
		})

		It("returns error when new metric already exists", func() {
			df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price, data.Volume},
				data.Daily, [][]float64{{1, 2, 3, 4, 5}, {10, 20, 30, 40, 50}})

			result := df.RenameMetric(data.Price, data.Volume)
			Expect(result.Err()).To(HaveOccurred())
			Expect(result.Err().Error()).To(ContainSubstring("already exists"))
		})

		It("propagates existing error", func() {
			df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
			short, _ := data.NewDataFrame(times[:3], []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3}})

			result := df.Add(short).RenameMetric(data.Price, data.Metric("Signal"))
			Expect(result.Err()).To(HaveOccurred())
		})
	})

	Describe("RiskFreeRates", func() {
		It("SetRiskFreeRates attaches rates and RiskFreeRates returns them", func() {
			rates := []float64{100.0, 100.01, 100.02, 100.03, 100.04}
			err := df.SetRiskFreeRates(rates)
			Expect(err).NotTo(HaveOccurred())
			Expect(df.RiskFreeRates()).To(Equal(rates))
		})

		It("SetRiskFreeRates returns error on length mismatch", func() {
			err := df.SetRiskFreeRates([]float64{1.0, 2.0})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("length"))
		})

		It("RiskFreeRates returns nil when not set", func() {
			fresh, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
			Expect(err).NotTo(HaveOccurred())
			Expect(fresh.RiskFreeRates()).To(BeNil())
		})

		Describe("RiskAdjustedPct", func() {
			It("computes 1-period risk-adjusted percent change by default", func() {
				// prices: 100, 101, 102, 103, 104 (AAPL Price column, Daily freq)
				// raw annualized yields: 4.5% each period
				rates := []float64{4.5, 4.5, 4.5, 4.5, 4.5}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.RiskAdjustedPct()
				col := result.Column(aapl, data.Price)
				Expect(math.IsNaN(col[0])).To(BeTrue())
				// pct = (101-100)/100 = 0.01
				// rf = rolling_sum([4.5]) / 252 / 100 = 4.5 / 252 / 100
				expected := 0.01 - 4.5/252.0/100.0
				Expect(col[1]).To(BeNumerically("~", expected, 1e-10))
			})

			It("computes 3-period risk-adjusted percent change", func() {
				// Different yields each period
				rates := []float64{4.0, 4.2, 4.4, 4.6, 4.8}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.RiskAdjustedPct(3)
				col := result.Column(aapl, data.Price)
				for i := 0; i < 3; i++ {
					Expect(math.IsNaN(col[i])).To(BeTrue())
				}
				// pct = (103-100)/100 = 0.03
				// rf = rolling_sum([4.2, 4.4, 4.6]) / 252 / 100
				rfSum := (4.2 + 4.4 + 4.6)
				expected := 0.03 - rfSum/252.0/100.0
				Expect(col[3]).To(BeNumerically("~", expected, 1e-10))
			})

			It("returns error when no risk-free rates attached", func() {
				fresh, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{{1, 2, 3, 4, 5}})
				Expect(err).NotTo(HaveOccurred())
				result := fresh.RiskAdjustedPct()
				Expect(result.Err()).To(HaveOccurred())
				Expect(result.Err().Error()).To(ContainSubstring("no risk-free rates"))
			})

			It("produces all NaN when period >= length", func() {
				rates := []float64{4.5, 4.5, 4.5, 4.5, 4.5}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.RiskAdjustedPct(500)
				col := result.Column(aapl, data.Price)
				for _, val := range col {
					Expect(math.IsNaN(val)).To(BeTrue())
				}
			})

			It("uses frequency-appropriate periods per year", func() {
				// Monthly frequency: periodsPerYear = 12
				monthlyTimes := []time.Time{
					time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				}
				monthlyDF, err := data.NewDataFrame(monthlyTimes, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Monthly, [][]float64{{100, 110, 121}})
				Expect(err).NotTo(HaveOccurred())
				Expect(monthlyDF.SetRiskFreeRates([]float64{4.5, 4.5, 4.5})).To(Succeed())

				result := monthlyDF.RiskAdjustedPct()
				col := result.Column(aapl, data.Price)
				// pct = (110-100)/100 = 0.10
				// rf = 4.5 / 12 / 100 = 0.00375
				expected := 0.10 - 4.5/12.0/100.0
				Expect(col[1]).To(BeNumerically("~", expected, 1e-10))
			})

			It("does not modify original", func() {
				rates := []float64{4.5, 4.5, 4.5, 4.5, 4.5}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())
				orig := df.Column(aapl, data.Price)[0]
				_ = df.RiskAdjustedPct()
				Expect(df.Column(aapl, data.Price)[0]).To(Equal(orig))
			})
		})

		Describe("propagation", func() {
			It("propagates through Apply-based methods (Pct, MulScalar chain)", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.Pct().MulScalar(100)
				Expect(result.RiskFreeRates()).To(Equal(rates))
			})

			It("propagates through Assets narrowing", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.Assets(aapl)
				Expect(result.RiskFreeRates()).To(Equal(rates))
			})

			It("propagates through Metrics narrowing", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.Metrics(data.Price)
				Expect(result.RiskFreeRates()).To(Equal(rates))
			})

			It("propagates through elemWiseOp (Add without metrics)", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.Add(df)
				Expect(result.RiskFreeRates()).To(Equal(rates))
			})

			It("deep-copies risk-free rates through Copy", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				copied := df.Copy()
				Expect(copied.RiskFreeRates()).To(Equal(rates))

				// Mutating the copy's rates must not affect the original.
				copied.RiskFreeRates()[0] = 999.0
				Expect(df.RiskFreeRates()[0]).To(Equal(100.0))
			})

			It("propagates through MaxAcrossAssets", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.MaxAcrossAssets()
				Expect(result.RiskFreeRates()).To(Equal(rates))
			})

			It("slices risk-free rates through Between", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.Between(times[1], times[3])
				Expect(result.RiskFreeRates()).To(Equal([]float64{100.02, 100.04, 100.06}))
			})

			It("slices risk-free rates through Filter", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.Filter(func(t time.Time, _ *data.DataFrame) bool {
					return t.Equal(times[0]) || t.Equal(times[4])
				})
				Expect(result.RiskFreeRates()).To(Equal([]float64{100.0, 100.08}))
			})

			It("slices risk-free rates through Drop", func() {
				vals := [][]float64{{math.NaN(), 101, 102, 103, 104}}
				ndf, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, vals)
				Expect(err).NotTo(HaveOccurred())

				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(ndf.SetRiskFreeRates(rates)).To(Succeed())

				result := ndf.Drop(math.NaN())
				Expect(result.Len()).To(Equal(4))
				Expect(result.RiskFreeRates()).To(Equal([]float64{100.02, 100.04, 100.06, 100.08}))
			})

			It("slices risk-free rates for At (single row)", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.At(times[2])
				Expect(result.RiskFreeRates()).To(Equal([]float64{100.04}))
			})

			It("AppendRow invalidates risk-free rates", func() {
				singleAsset, err := data.NewDataFrame(
					times[:2],
					[]asset.Asset{aapl},
					[]data.Metric{data.Price},
					data.Daily,
					[][]float64{{100, 101}},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(singleAsset.SetRiskFreeRates([]float64{100.0, 100.02})).To(Succeed())

				newTime := times[2]
				Expect(singleAsset.AppendRow(newTime, []float64{102})).To(Succeed())
				Expect(singleAsset.RiskFreeRates()).To(BeNil())
			})

			It("does not propagate through Mean aggregation", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.Mean()
				Expect(result.RiskFreeRates()).To(BeNil())
			})

			It("does not propagate through Reduce aggregation", func() {
				rates := []float64{100.0, 100.02, 100.04, 100.06, 100.08}
				Expect(df.SetRiskFreeRates(rates)).To(Succeed())

				result := df.Reduce(func(col []float64) float64 { return col[0] })
				Expect(result.RiskFreeRates()).To(BeNil())
			})

			It("MergeTimes concatenates risk-free rates", func() {
				df1, err := data.NewDataFrame(
					times[:2],
					[]asset.Asset{aapl},
					[]data.Metric{data.Price},
					data.Daily,
					[][]float64{{100, 101}},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(df1.SetRiskFreeRates([]float64{100.0, 100.02})).To(Succeed())

				df2, err := data.NewDataFrame(
					times[2:4],
					[]asset.Asset{aapl},
					[]data.Metric{data.Price},
					data.Daily,
					[][]float64{{102, 103}},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(df2.SetRiskFreeRates([]float64{100.04, 100.06})).To(Succeed())

				merged, err := data.MergeTimes(df1, df2)
				Expect(err).NotTo(HaveOccurred())
				Expect(merged.RiskFreeRates()).To(Equal([]float64{100.0, 100.02, 100.04, 100.06}))
			})

			It("Downsample preserves risk-free rates (last value per group)", func() {
				// 10 daily values, downsampled to weekly (2 groups: 7 + 3)
				base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				dsTimes := make([]time.Time, 10)
				dsVals := make([]float64, 10)
				dsRates := make([]float64, 10)

				for idx := range dsTimes {
					dsTimes[idx] = base.AddDate(0, 0, idx)
					dsVals[idx] = float64(idx + 1)
					dsRates[idx] = 100.0 + float64(idx)*0.02
				}

				dsDF, err := data.NewDataFrame(dsTimes, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, [][]float64{dsVals})
				Expect(err).NotTo(HaveOccurred())
				Expect(dsDF.SetRiskFreeRates(dsRates)).To(Succeed())

				result := dsDF.Downsample(data.Weekly).Last()
				Expect(result.Len()).To(Equal(2))
				// Last RF value of week 1 (index 6) and week 2 (index 9)
				Expect(result.RiskFreeRates()).To(Equal([]float64{dsRates[6], dsRates[9]}))
			})

			It("Upsample ForwardFill preserves risk-free rates", func() {
				usTimes := []time.Time{
					time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				}
				usVals := [][]float64{{100, 300}}
				usRates := []float64{100.0, 100.04}

				usDF, err := data.NewDataFrame(usTimes, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, usVals)
				Expect(err).NotTo(HaveOccurred())
				Expect(usDF.SetRiskFreeRates(usRates)).To(Succeed())

				result := usDF.Upsample(data.Daily).ForwardFill()
				Expect(result.Len()).To(Equal(3))
				// Jan 1 -> 100.0, Jan 2 (filled) -> 100.0, Jan 3 -> 100.04
				Expect(result.RiskFreeRates()).To(Equal([]float64{100.0, 100.0, 100.04}))
			})

			It("Upsample Interpolate interpolates risk-free rates", func() {
				usTimes := []time.Time{
					time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				}
				usVals := [][]float64{{100, 300}}
				usRates := []float64{100.0, 100.04}

				usDF, err := data.NewDataFrame(usTimes, []asset.Asset{aapl}, []data.Metric{data.Price}, data.Daily, usVals)
				Expect(err).NotTo(HaveOccurred())
				Expect(usDF.SetRiskFreeRates(usRates)).To(Succeed())

				result := usDF.Upsample(data.Daily).Interpolate()
				Expect(result.Len()).To(Equal(3))
				// Jan 1 -> 100.0, Jan 2 (interpolated) -> 100.02, Jan 3 -> 100.04
				Expect(result.RiskFreeRates()[0]).To(BeNumerically("~", 100.0, 1e-10))
				Expect(result.RiskFreeRates()[1]).To(BeNumerically("~", 100.02, 1e-10))
				Expect(result.RiskFreeRates()[2]).To(BeNumerically("~", 100.04, 1e-10))
			})

			It("MergeTimes without risk-free rates returns nil rates", func() {
				df1, err := data.NewDataFrame(
					times[:2],
					[]asset.Asset{aapl},
					[]data.Metric{data.Price},
					data.Daily,
					[][]float64{{100, 101}},
				)
				Expect(err).NotTo(HaveOccurred())

				df2, err := data.NewDataFrame(
					times[2:4],
					[]asset.Asset{aapl},
					[]data.Metric{data.Price},
					data.Daily,
					[][]float64{{102, 103}},
				)
				Expect(err).NotTo(HaveOccurred())

				merged, err := data.MergeTimes(df1, df2)
				Expect(err).NotTo(HaveOccurred())
				Expect(merged.RiskFreeRates()).To(BeNil())
			})
		})
	})
})

var _ = Describe("Window", func() {
	var df *data.DataFrame

	BeforeEach(func() {
		times := []time.Time{
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC),
		}
		assets := []asset.Asset{{CompositeFigi: "SPY", Ticker: "SPY"}}
		metrics := []data.Metric{data.MetricClose}
		vals := [][]float64{{100, 110, 120, 130, 140}}
		var err error
		df, err = data.NewDataFrame(times, assets, metrics, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns full DataFrame when window is nil", func() {
		result := df.Window(nil)
		Expect(result.Len()).To(Equal(5))
	})

	It("trims to last 2 months", func() {
		w := data.Months(2)
		result := df.Window(&w)
		// End is 2025-05-01; Months(2) gives Apr and May = 2 rows
		Expect(result.Len()).To(Equal(2))
	})

	It("returns full DataFrame when window exceeds data", func() {
		w := data.Years(10)
		result := df.Window(&w)
		Expect(result.Len()).To(Equal(5))
	})

	It("propagates error", func() {
		errDF := data.WithErr(fmt.Errorf("test error"))
		w := data.Months(1)
		result := errDF.Window(&w)
		Expect(result.Err()).To(HaveOccurred())
	})
})

var _ = Describe("CumMax", func() {
	It("computes running maximum per column", func() {
		times := []time.Time{
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		assets := []asset.Asset{spy}
		metrics := []data.Metric{data.MetricClose}
		vals := [][]float64{{100, 120, 110, 130}}
		df, err := data.NewDataFrame(times, assets, metrics, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		result := df.CumMax()
		col := result.Column(spy, data.MetricClose)
		Expect(col).To(Equal([]float64{100, 120, 120, 130}))
	})

	It("handles single value", func() {
		times := []time.Time{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{50}})
		Expect(err).NotTo(HaveOccurred())

		result := df.CumMax()
		Expect(result.Column(spy, data.MetricClose)).To(Equal([]float64{50}))
	})

	It("propagates error", func() {
		errDF := data.WithErr(fmt.Errorf("test error"))
		result := errDF.CumMax()
		Expect(result.Err()).To(HaveOccurred())
	})
})

var _ = Describe("Frequency", func() {
	It("String returns correct names for all known values", func() {
		Expect(data.Tick.String()).To(Equal("Tick"))
		Expect(data.Daily.String()).To(Equal("Daily"))
		Expect(data.Weekly.String()).To(Equal("Weekly"))
		Expect(data.Monthly.String()).To(Equal("Monthly"))
		Expect(data.Quarterly.String()).To(Equal("Quarterly"))
		Expect(data.Yearly.String()).To(Equal("Yearly"))
	})

	It("String returns formatted fallback for unknown value", func() {
		unknown := data.Frequency(99)
		Expect(unknown.String()).To(Equal(fmt.Sprintf("Frequency(%d)", 99)))
	})
})

var _ = Describe("AppendRow", func() {
	It("appends a row to a single-column DataFrame", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}},
		)
		Expect(err).NotTo(HaveOccurred())

		err = df.AppendRow(t2, []float64{110})
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Len()).To(Equal(2))
		Expect(df.Column(spy, data.MetricClose)).To(Equal([]float64{100, 110}))
		Expect(df.End()).To(Equal(t2))
	})

	It("appends rows with multiple columns", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		metrics := []data.Metric{data.MetricClose, data.PortfolioEquity}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy}, metrics, data.Daily, [][]float64{{100}, {200}},
		)
		Expect(err).NotTo(HaveOccurred())

		err = df.AppendRow(t2, []float64{110, 220})
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Column(spy, data.MetricClose)).To(Equal([]float64{100, 110}))
		Expect(df.Column(spy, data.PortfolioEquity)).To(Equal([]float64{200, 220}))
	})

	It("rejects non-chronological timestamp", func() {
		t1 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}},
		)
		Expect(err).NotTo(HaveOccurred())

		err = df.AppendRow(t0, []float64{90})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("chronological"))
	})

	It("rejects wrong values length", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}},
		)
		Expect(err).NotTo(HaveOccurred())

		err = df.AppendRow(t2, []float64{110, 220})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("values length"))
	})

	It("does not affect prior Window snapshots", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}},
		)
		Expect(err).NotTo(HaveOccurred())

		snapshot := df.Window(nil)
		Expect(snapshot.Len()).To(Equal(1))

		err = df.AppendRow(t2, []float64{110})
		Expect(err).NotTo(HaveOccurred())

		// Snapshot should be unaffected
		Expect(snapshot.Len()).To(Equal(1))
		Expect(df.Len()).To(Equal(2))
	})

	It("AppendRow does not affect views created via Between", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		t3 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, data.Daily, [][]float64{{100, 110}},
		)
		Expect(err).NotTo(HaveOccurred())

		view := df.Between(t1, t2)
		Expect(view.Len()).To(Equal(2))

		err = df.AppendRow(t3, []float64{120})
		Expect(err).NotTo(HaveOccurred())

		Expect(view.Len()).To(Equal(2))
		Expect(view.Value(spy, data.MetricClose)).To(Equal(110.0))
		Expect(df.Len()).To(Equal(3))
	})

	It("Insert does not affect views", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}},
		)
		Expect(err).NotTo(HaveOccurred())

		view := df.Metrics(data.MetricClose)
		Expect(view.ColCount()).To(Equal(1))

		err = df.Insert(spy, data.Volume, []float64{5000})
		Expect(err).NotTo(HaveOccurred())

		Expect(view.ColCount()).To(Equal(1))
		Expect(view.Value(spy, data.MetricClose)).To(Equal(100.0))
		Expect(df.ColCount()).To(Equal(2))
	})

	Describe("Broadcast Sub", func() {
		It("subtracts a selected metric column from all columns", func() {
			t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
			spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

			df, err := data.NewDataFrame(
				[]time.Time{t1, t2}, []asset.Asset{spy},
				[]data.Metric{data.PortfolioEquity}, data.Daily, [][]float64{{10, 20}},
			)
			Expect(err).NotTo(HaveOccurred())

			other, err := data.NewDataFrame(
				[]time.Time{t1, t2}, []asset.Asset{spy},
				[]data.Metric{data.PortfolioEquity, data.PortfolioRiskFree},
				data.Daily, [][]float64{{10, 20}, {1, 2}},
			)
			Expect(err).NotTo(HaveOccurred())

			result := df.Sub(other, data.PortfolioRiskFree)
			Expect(result.Err()).NotTo(HaveOccurred())
			col := result.Column(spy, data.PortfolioEquity)
			Expect(col).To(Equal([]float64{9, 18}))
		})

		It("chains multiple metrics sequentially", func() {
			t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

			df, err := data.NewDataFrame(
				[]time.Time{t1}, []asset.Asset{spy},
				[]data.Metric{data.PortfolioEquity}, data.Daily, [][]float64{{100}},
			)
			Expect(err).NotTo(HaveOccurred())

			other, err := data.NewDataFrame(
				[]time.Time{t1}, []asset.Asset{spy},
				[]data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree},
				data.Daily, [][]float64{{100}, {5}, {3}},
			)
			Expect(err).NotTo(HaveOccurred())

			// (100 - 5) - 3 = 92
			result := df.Sub(other, data.PortfolioBenchmark, data.PortfolioRiskFree)
			Expect(result.Err()).NotTo(HaveOccurred())
			Expect(result.Column(spy, data.PortfolioEquity)).To(Equal([]float64{92}))
		})

		It("falls back to intersection when no metrics specified", func() {
			t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

			a, err := data.NewDataFrame(
				[]time.Time{t1}, []asset.Asset{spy},
				[]data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}},
			)
			Expect(err).NotTo(HaveOccurred())

			b, err := data.NewDataFrame(
				[]time.Time{t1}, []asset.Asset{spy},
				[]data.Metric{data.MetricClose}, data.Daily, [][]float64{{30}},
			)
			Expect(err).NotTo(HaveOccurred())

			result := a.Sub(b)
			Expect(result.Err()).NotTo(HaveOccurred())
			Expect(result.Column(spy, data.MetricClose)).To(Equal([]float64{70}))
		})
	})
})

type mockAnnotator struct {
	entries []struct {
		timestamp time.Time
		key       string
		value     string
	}
}

func (mockAnn *mockAnnotator) Annotate(timestamp time.Time, key, value string) {
	mockAnn.entries = append(mockAnn.entries, struct {
		timestamp time.Time
		key       string
		value     string
	}{timestamp, key, value})
}

var _ = Describe("DataFrame.Annotate", func() {
	It("pushes non-NaN cells as annotations with TICKER/Metric keys", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		efa := asset.Asset{CompositeFigi: "EFA001", Ticker: "EFA"}
		t1 := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, efa},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[][]float64{{150.5}, {75.25}},
		)
		Expect(err).NotTo(HaveOccurred())

		dest := &mockAnnotator{}
		result := df.Annotate(dest)
		Expect(result.Err()).NotTo(HaveOccurred())

		Expect(dest.entries).To(HaveLen(2))

		keys := make(map[string]string)
		for _, entry := range dest.entries {
			Expect(entry.timestamp).To(Equal(t1))
			keys[entry.key] = entry.value
		}

		Expect(keys).To(HaveKey("SPY/Close"))
		Expect(keys).To(HaveKey("EFA/Close"))
	})

	It("skips NaN values", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		t1 := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[][]float64{{math.NaN()}},
		)
		Expect(err).NotTo(HaveOccurred())

		dest := &mockAnnotator{}
		df.Annotate(dest)
		Expect(dest.entries).To(BeEmpty())
	})

	It("is a no-op when DataFrame has an error", func() {
		errDF := data.WithErr(fmt.Errorf("test error"))

		dest := &mockAnnotator{}
		errDF.Annotate(dest)
		Expect(dest.entries).To(BeEmpty())
	})

	It("handles multiple rows and metrics", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		t1 := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		t2 := time.Date(2024, 1, 16, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.AdjClose},
			data.Daily,
			[][]float64{{150.0, 151.0}, {149.5, 150.5}},
		)
		Expect(err).NotTo(HaveOccurred())

		dest := &mockAnnotator{}
		df.Annotate(dest)
		Expect(dest.entries).To(HaveLen(4))
	})
})

var _ = Describe("DataFrame Frequency", func() {
	It("returns the frequency set at construction", func() {
		df, err := data.NewDataFrame(
			[]time.Time{time.Now()},
			[]asset.Asset{{CompositeFigi: "SPY001", Ticker: "SPY"}},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[][]float64{{100.0}},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Frequency()).To(Equal(data.Daily))
	})

	It("propagates frequency through Assets narrowing", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{time.Now()},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Monthly,
			[][]float64{{100.0}},
		)
		Expect(err).NotTo(HaveOccurred())

		narrowed := df.Assets(spy)
		Expect(narrowed.Frequency()).To(Equal(data.Monthly))
	})

	It("propagates frequency through Copy", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{time.Now()},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Weekly,
			[][]float64{{100.0}},
		)
		Expect(err).NotTo(HaveOccurred())

		copied := df.Copy()
		Expect(copied.Frequency()).To(Equal(data.Weekly))
	})

	It("sets target frequency on downsampled result", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		times := []time.Time{
			time.Date(2024, 1, 2, 16, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 3, 16, 0, 0, 0, time.UTC),
			time.Date(2024, 2, 1, 16, 0, 0, 0, time.UTC),
		}
		df, err := data.NewDataFrame(times, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, data.Daily,
			[][]float64{{100, 101, 110}},
		)
		Expect(err).NotTo(HaveOccurred())

		monthly := df.Downsample(data.Monthly).Last()
		Expect(monthly.Frequency()).To(Equal(data.Monthly))
	})
})

var _ = Describe("ParseFrequency", func() {
	It("round-trips all frequency constants through String/Parse", func() {
		for _, freq := range []data.Frequency{data.Tick, data.Daily, data.Weekly, data.Monthly, data.Quarterly, data.Yearly} {
			parsed, err := data.ParseFrequency(freq.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(parsed).To(Equal(freq))
		}
	})

	It("returns error for unknown frequency string", func() {
		_, err := data.ParseFrequency("bogus")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("DataFrame DataSource", func() {
	It("returns nil source by default", func() {
		df, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Source()).To(BeNil())
	})

	It("returns the source after SetSource", func() {
		df, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		Expect(err).NotTo(HaveOccurred())

		mock := &mockFrameDataSource{}
		df.SetSource(mock)
		Expect(df.Source()).To(Equal(mock))
	})
})

var _ = Describe("Correlation", func() {
	It("computes Pearson correlation between two assets", func() {
		times := []time.Time{
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

		// Perfectly correlated: both increase linearly.
		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			[][]float64{{1, 2, 3, 4}, {10, 20, 30, 40}},
		)
		Expect(err).NotTo(HaveOccurred())

		result := df.Correlation(spy, aapl)
		Expect(result.Err()).NotTo(HaveOccurred())

		composite := data.CompositeAsset(spy, aapl)
		corr := result.ValueAt(composite, data.AdjClose, result.Times()[0])
		Expect(corr).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("returns -1 for perfectly negatively correlated assets", func() {
		times := []time.Time{
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			[][]float64{{1, 2, 3, 4}, {40, 30, 20, 10}},
		)
		Expect(err).NotTo(HaveOccurred())

		result := df.Correlation(spy, aapl)
		Expect(result.Err()).NotTo(HaveOccurred())

		composite := data.CompositeAsset(spy, aapl)
		corr := result.ValueAt(composite, data.AdjClose, result.Times()[0])
		Expect(corr).To(BeNumerically("~", -1.0, 1e-9))
	})

	It("returns 0 for uncorrelated assets", func() {
		times := []time.Time{
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

		// Orthogonal: sine and cosine phase-shifted by 90 degrees have zero correlation.
		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			[][]float64{{1, 0, -1, 0}, {0, 1, 0, -1}},
		)
		Expect(err).NotTo(HaveOccurred())

		result := df.Correlation(spy, aapl)
		Expect(result.Err()).NotTo(HaveOccurred())

		composite := data.CompositeAsset(spy, aapl)
		corr := result.ValueAt(composite, data.AdjClose, result.Times()[0])
		Expect(corr).To(BeNumerically("~", 0.0, 0.1))
	})
})
