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
	"fmt"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

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
		values := []float64{
			// AAPL Price
			100, 101, 102, 103, 104,
			// AAPL Volume
			1000, 1100, 1200, 1300, 1400,
			// GOOG Price
			200, 202, 204, 206, 208,
			// GOOG Volume
			2000, 2200, 2400, 2600, 2800,
		}

		df = data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, values)
	})

	Describe("NewDataFrame", func() {
		It("builds correct assetIndex", func() {
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(df.Value(goog, data.Price)).To(Equal(208.0))
		})

		It("panics when data length mismatches dimensions", func() {
			Expect(func() {
				data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2})
			}).To(Panic())
		})

		It("accepts empty dimensions with nil data", func() {
			empty := data.NewDataFrame(nil, nil, nil, nil)
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
			empty := data.NewDataFrame(nil, nil, nil, nil)
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
			empty := data.NewDataFrame(nil, nil, nil, nil)
			Expect(empty.Table()).To(Equal("(empty DataFrame)"))
		})

		Context("empty frame", func() {
			var empty *data.DataFrame

			BeforeEach(func() {
				empty = data.NewDataFrame(nil, nil, nil, nil)
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
				single := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{42.0})
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
			vals := []float64{100.0, 101.0}
			df2 = data.NewDataFrame(t, assets, metrics, vals)
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

		It("Assets produces independent copy", func() {
			narrowed := df.Assets(aapl)
			col := narrowed.Column(aapl, data.Price)
			col[4] = 999.0
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
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

		It("Metrics produces independent copy", func() {
			narrowed := df.Metrics(data.Price)
			col := narrowed.Column(aapl, data.Price)
			col[4] = 999.0
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
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

		It("Between produces independent copy", func() {
			sub := df.Between(times[0], times[4])
			col := sub.Column(aapl, data.Price)
			col[0] = 999.0
			Expect(df.ValueAt(aapl, data.Price, times[0])).To(Equal(100.0))
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
			vals := []float64{1, math.NaN(), 3}
			t := []time.Time{times[0], times[1], times[2]}
			small := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)

			cleaned := small.Drop(math.NaN())
			Expect(cleaned.Len()).To(Equal(2))
		})

		It("Drop with non-NaN sentinel removes matching rows", func() {
			vals := []float64{1, -999, 3, -999, 5}
			small := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
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
			vals := []float64{
				1, math.NaN(), 3, // AAPL Price
				10, 20, 30, // GOOG Price
			}
			t := []time.Time{times[0], times[1], times[2]}
			multi := data.NewDataFrame(t, []asset.Asset{aapl, goog}, []data.Metric{data.Price}, vals)
			cleaned := multi.Drop(math.NaN())
			Expect(cleaned.Len()).To(Equal(2))
		})
	})

	Describe("Mutation", func() {
		It("Insert adds new column for existing asset, new metric", func() {
			newMetric := data.Metric("Beta")
			vals := []float64{0.9, 0.91, 0.92, 0.93, 0.94}
			df.Insert(aapl, newMetric, vals)
			Expect(df.Value(aapl, newMetric)).To(Equal(0.94))
			// Existing data should be intact.
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(df.Value(goog, data.Price)).To(Equal(208.0))
			Expect(df.Value(goog, data.Volume)).To(Equal(2800.0))
		})

		It("Insert adds new column for new asset", func() {
			msft := asset.Asset{CompositeFigi: "MSFT", Ticker: "MSFT"}
			vals := []float64{300, 301, 302, 303, 304}
			df.Insert(msft, data.Price, vals)
			Expect(df.Value(msft, data.Price)).To(Equal(304.0))
			// Existing data should be intact.
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(df.Value(goog, data.Volume)).To(Equal(2800.0))
		})

		It("Insert with new asset AND new metric simultaneously", func() {
			msft := asset.Asset{CompositeFigi: "MSFT", Ticker: "MSFT"}
			newMetric := data.Metric("Beta")
			vals := []float64{1.1, 1.2, 1.3, 1.4, 1.5}
			df.Insert(msft, newMetric, vals)
			Expect(df.Value(msft, newMetric)).To(Equal(1.5))
			// All original data intact.
			Expect(df.Value(aapl, data.Price)).To(Equal(104.0))
			Expect(df.Value(aapl, data.Volume)).To(Equal(1400.0))
			Expect(df.Value(goog, data.Price)).To(Equal(208.0))
			Expect(df.Value(goog, data.Volume)).To(Equal(2800.0))
		})

		It("Insert overwrites an existing column", func() {
			vals := []float64{500, 501, 502, 503, 504}
			df.Insert(aapl, data.Price, vals)
			Expect(df.Value(aapl, data.Price)).To(Equal(504.0))
			col := df.Column(aapl, data.Price)
			Expect(col).To(Equal([]float64{500, 501, 502, 503, 504}))
			// Other columns unaffected.
			Expect(df.Value(aapl, data.Volume)).To(Equal(1400.0))
		})

		It("Insert panics when values length does not match Len()", func() {
			Expect(func() {
				df.Insert(aapl, data.Price, []float64{1, 2})
			}).To(Panic())
		})
	})

	Describe("DataFrame arithmetic", func() {
		var other *data.DataFrame

		BeforeEach(func() {
			otherVals := []float64{
				10, 10, 10, 10, 10,
				100, 100, 100, 100, 100,
				20, 20, 20, 20, 20,
				200, 200, 200, 200, 200,
			}
			other = data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, otherVals)
		})

		It("Add performs element-wise addition", func() {
			result := df.Add(other)
			Expect(result.Value(aapl, data.Price)).To(Equal(114.0))
			Expect(result.Value(goog, data.Volume)).To(Equal(3000.0))
		})

		It("Sub performs element-wise subtraction", func() {
			result := df.Sub(other)
			Expect(result.Value(aapl, data.Price)).To(Equal(94.0))
		})

		It("Mul performs element-wise multiplication", func() {
			result := df.Mul(other)
			Expect(result.Value(aapl, data.Price)).To(Equal(1040.0))
		})

		It("Div performs element-wise division", func() {
			result := df.Div(other)
			Expect(result.Value(aapl, data.Price)).To(Equal(10.4))
		})

		It("Div by zero produces Inf", func() {
			zeroVals := make([]float64, 20)
			zero := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, zeroVals)
			result := df.Div(zero)
			v := result.Value(aapl, data.Price)
			Expect(math.IsInf(v, 1)).To(BeTrue())
		})

		It("Div zero by zero produces NaN", func() {
			zeroVals := make([]float64, 20)
			zeroA := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, zeroVals)
			zeroB := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price, data.Volume}, make([]float64, 20))
			result := zeroA.Div(zeroB)
			Expect(math.IsNaN(result.Value(aapl, data.Price))).To(BeTrue())
		})

		It("arithmetic with partial overlap returns intersection only", func() {
			// other has only AAPL with only Price.
			partialVals := []float64{1, 1, 1, 1, 1}
			partial := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, partialVals)
			result := df.Add(partial)
			Expect(result.ColCount()).To(Equal(1)) // only AAPL/Price
			Expect(result.Value(aapl, data.Price)).To(Equal(105.0))
			// GOOG should not be present.
			Expect(math.IsNaN(result.Value(goog, data.Price))).To(BeTrue())
		})

		It("arithmetic with no overlap returns empty frame", func() {
			msft := asset.Asset{CompositeFigi: "MSFT", Ticker: "MSFT"}
			noOverlap := data.NewDataFrame(times, []asset.Asset{msft}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})
			result := df.Add(noOverlap)
			Expect(result.Len()).To(Equal(0))
		})

		It("arithmetic panics on timestamp count mismatch", func() {
			shortTimes := times[:3]
			short := data.NewDataFrame(shortTimes, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3})
			Expect(func() {
				df.Add(short)
			}).To(Panic())
		})

		It("arithmetic does not modify original frames", func() {
			origAAPL := df.Value(aapl, data.Price)
			origOther := other.Value(aapl, data.Price)
			_ = df.Add(other)
			Expect(df.Value(aapl, data.Price)).To(Equal(origAAPL))
			Expect(other.Value(aapl, data.Price)).To(Equal(origOther))
		})

		It("NaN propagates through element-wise addition", func() {
			nanVals := []float64{
				1, math.NaN(), 3, 4, 5,
				10, 20, 30, 40, 50,
				100, 200, 300, 400, 500,
				1000, 2000, 3000, 4000, 5000,
			}
			nanFrame := data.NewDataFrame(times, []asset.Asset{aapl, goog},
				[]data.Metric{data.Price, data.Volume}, nanVals)
			result := df.Add(nanFrame)
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
		It("Max returns max across assets per timestamp", func() {
			maxDF := df.Max()
			col := maxDF.Column(asset.Asset{Ticker: "MAX"}, data.Price)
			Expect(col).To(Equal([]float64{200, 202, 204, 206, 208}))
		})

		It("Min returns min across assets per timestamp", func() {
			minDF := df.Min()
			col := minDF.Column(asset.Asset{Ticker: "MIN"}, data.Price)
			Expect(col).To(Equal([]float64{100, 101, 102, 103, 104}))
		})

		It("Max on single-asset frame returns same values", func() {
			single := df.Assets(aapl)
			maxDF := single.Max()
			Expect(maxDF.Len()).To(Equal(5))
			col := maxDF.Column(asset.Asset{Ticker: "MAX"}, data.Price)
			Expect(col).To(Equal([]float64{100, 101, 102, 103, 104}))
		})

		It("Min on single-asset frame returns same values", func() {
			single := df.Assets(aapl)
			minDF := single.Min()
			col := minDF.Column(asset.Asset{Ticker: "MIN"}, data.Price)
			Expect(col).To(Equal([]float64{100, 101, 102, 103, 104}))
		})

		It("Max/Min aggregates across all metrics", func() {
			maxDF := df.Max()
			volCol := maxDF.Column(asset.Asset{Ticker: "MAX"}, data.Volume)
			Expect(volCol).To(Equal([]float64{2000, 2200, 2400, 2600, 2800}))
		})

		It("IdxMax returns asset with max value per timestamp", func() {
			result := df.IdxMax()
			Expect(result).To(HaveLen(5))
			for _, a := range result {
				Expect(a.CompositeFigi).To(Equal("GOOG"))
			}
		})

		It("IdxMax with alternating maxes", func() {
			// AAPL > GOOG on even indices, GOOG > AAPL on odd.
			vals := []float64{
				10, 1, 10, 1, 10, // AAPL Price
				1, 10, 1, 10, 1, // GOOG Price
			}
			altDF := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.Price}, vals)
			result := altDF.IdxMax()
			Expect(result[0].CompositeFigi).To(Equal("AAPL"))
			Expect(result[1].CompositeFigi).To(Equal("GOOG"))
			Expect(result[2].CompositeFigi).To(Equal("AAPL"))
			Expect(result[3].CompositeFigi).To(Equal("GOOG"))
			Expect(result[4].CompositeFigi).To(Equal("AAPL"))
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
			vals := []float64{0, 1, 2, 3, 4}
			zdf := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
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
			single := data.NewDataFrame(times[:1], []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{42.0})
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
			vals := []float64{0, 1, 2, 3, 4}
			zdf := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
			result := zdf.Log()
			col := result.Column(aapl, data.Price)
			Expect(math.IsInf(col[0], -1)).To(BeTrue())
		})

		It("Log of negative value produces NaN", func() {
			vals := []float64{-1, 1, 2, 3, 4}
			zdf := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
			result := zdf.Log()
			col := result.Column(aapl, data.Price)
			Expect(math.IsNaN(col[0])).To(BeTrue())
		})

		It("Diff on frame with zero timestamps does not panic", func() {
			// A frame with assets and metrics but no timestamps has T=0.
			noTime := data.NewDataFrame(nil, []asset.Asset{aapl}, []data.Metric{data.Price}, nil)
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

	Describe("Resampling", func() {
		var weeklyDF *data.DataFrame

		BeforeEach(func() {
			// Create data spanning multiple weeks: 10 daily points.
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // Monday
			t := make([]time.Time, 10)
			vals := make([]float64, 10)
			for i := range t {
				t[i] = base.AddDate(0, 0, i)
				vals[i] = float64(i + 1)
			}

			weeklyDF = data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
		})

		It("Resample Weekly Last picks last value per week", func() {
			result := weeklyDF.Resample(data.Weekly, data.Last)
			Expect(result.Len()).To(Equal(2))
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(7.0))
			Expect(col[1]).To(Equal(10.0))
		})

		It("Resample Weekly First picks first value per week", func() {
			result := weeklyDF.Resample(data.Weekly, data.First)
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(1.0))
			Expect(col[1]).To(Equal(8.0))
		})

		It("Resample Weekly Mean computes mean per week", func() {
			result := weeklyDF.Resample(data.Weekly, data.Mean)
			col := result.Column(aapl, data.Price)
			// Week 1: mean(1..7) = 4, Week 2: mean(8,9,10) = 9
			Expect(col[0]).To(Equal(4.0))
			Expect(col[1]).To(Equal(9.0))
		})

		It("Resample Weekly Max picks max per week", func() {
			result := weeklyDF.Resample(data.Weekly, data.Max)
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(7.0))
			Expect(col[1]).To(Equal(10.0))
		})

		It("Resample Weekly Min picks min per week", func() {
			result := weeklyDF.Resample(data.Weekly, data.Min)
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(1.0))
			Expect(col[1]).To(Equal(8.0))
		})

		It("Resample Monthly Sum sums values per month", func() {
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t := make([]time.Time, 60)
			vals := make([]float64, 60)
			for i := range t {
				t[i] = base.AddDate(0, 0, i)
				vals[i] = 1.0
			}
			monthDF := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
			result := monthDF.Resample(data.Monthly, data.Sum)
			Expect(result.Len()).To(Equal(2)) // Jan and Feb
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(31.0))
			Expect(col[1]).To(Equal(29.0)) // 2024 is leap year
		})

		It("Resample Quarterly groups by quarter", func() {
			// 365 days spanning 4 quarters of 2024.
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t := make([]time.Time, 365)
			vals := make([]float64, 365)
			for i := range t {
				t[i] = base.AddDate(0, 0, i)
				vals[i] = 1.0
			}
			yearDF := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
			result := yearDF.Resample(data.Quarterly, data.Sum)
			// Q1: Jan(31)+Feb(29)+Mar(31)=91, Q2: Apr(30)+May(31)+Jun(30)=91,
			// Q3: Jul(31)+Aug(31)+Sep(30)=92, Q4: Oct(31)+Nov(30)+Dec(1)=62
			// Actually 365 days from Jan 1 to Dec 31 covers all 4 quarters.
			Expect(result.Len()).To(Equal(4))
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(91.0)) // Q1
			Expect(col[1]).To(Equal(91.0)) // Q2
			Expect(col[2]).To(Equal(92.0)) // Q3
		})

		It("Resample Yearly groups by year", func() {
			// 3 timestamps across 2 years.
			t := []time.Time{
				time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			}
			vals := []float64{10, 20, 30}
			yearDF := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
			result := yearDF.Resample(data.Yearly, data.Sum)
			Expect(result.Len()).To(Equal(2))
			col := result.Column(aapl, data.Price)
			Expect(col[0]).To(Equal(30.0)) // 2024: 10+20
			Expect(col[1]).To(Equal(30.0)) // 2025: 30
		})

		It("Resample on empty frame returns empty", func() {
			empty := data.NewDataFrame(nil, nil, nil, nil)
			result := empty.Resample(data.Weekly, data.Last)
			Expect(result.Len()).To(Equal(0))
		})

		It("Resample when all data in single period returns single row", func() {
			// All 5 timestamps are in the same week.
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t := make([]time.Time, 3)
			vals := make([]float64, 3)
			for i := range t {
				t[i] = base.AddDate(0, 0, i)
				vals[i] = float64(i + 1)
			}
			small := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
			result := small.Resample(data.Weekly, data.Sum)
			Expect(result.Len()).To(Equal(1))
			Expect(result.Column(aapl, data.Price)[0]).To(Equal(6.0))
		})

		It("Resample Daily treats every row as its own period", func() {
			// Daily (and Tick) frequency hits the periodChanged default branch
			// which always returns true, so each timestamp becomes its own group.
			result := weeklyDF.Resample(data.Daily, data.Last)
			Expect(result.Len()).To(Equal(weeklyDF.Len()))
			col := result.Column(aapl, data.Price)
			for i := 0; i < weeklyDF.Len(); i++ {
				Expect(col[i]).To(Equal(float64(i + 1)))
			}
		})

		It("Resample with unknown Aggregation produces NaN", func() {
			unknownAgg := data.Aggregation(99)
			result := weeklyDF.Resample(data.Weekly, unknownAgg)
			col := result.Column(aapl, data.Price)
			for _, v := range col {
				Expect(math.IsNaN(v)).To(BeTrue())
			}
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
			empty := data.NewDataFrame(nil, nil, nil, nil)
			result := empty.Reduce(func(col []float64) float64 { return 0 })
			Expect(result.Len()).To(Equal(0))
			Expect(result.ColCount()).To(Equal(0))
		})

		It("Apply on empty frame returns empty frame", func() {
			empty := data.NewDataFrame(nil, nil, nil, nil)
			result := empty.Apply(func(col []float64) []float64 {
				return col
			})
			Expect(result.Len()).To(Equal(0))
			Expect(result.ColCount()).To(Equal(0))
		})
	})
})

var _ = Describe("Aggregation", func() {
	It("String returns correct names for all known values", func() {
		Expect(data.Last.String()).To(Equal("Last"))
		Expect(data.First.String()).To(Equal("First"))
		Expect(data.Sum.String()).To(Equal("Sum"))
		Expect(data.Mean.String()).To(Equal("Mean"))
		Expect(data.Max.String()).To(Equal("Max"))
		Expect(data.Min.String()).To(Equal("Min"))
	})

	It("String returns formatted fallback for unknown value", func() {
		unknown := data.Aggregation(99)
		Expect(unknown.String()).To(Equal(fmt.Sprintf("Aggregation(%d)", 99)))
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
