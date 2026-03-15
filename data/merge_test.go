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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("Merge", func() {
	var (
		a1 asset.Asset
		a2 asset.Asset
	)

	BeforeEach(func() {
		a1 = asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "IBM"}
		a2 = asset.Asset{CompositeFigi: "BBG000BVPV84", Ticker: "AMZN"}
	})

	Describe("MergeColumns", func() {
		It("merges frames with different metrics for the same assets and times", func() {
			times := []time.Time{
				time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			df1, err := data.NewDataFrame(times, []asset.Asset{a1, a2}, []data.Metric{data.MetricClose}, data.Daily, []float64{
				100, 101, 102, // a1 Close
				200, 201, 202, // a2 Close
			})
			Expect(err).NotTo(HaveOccurred())

			df2, err := data.NewDataFrame(times, []asset.Asset{a1, a2}, []data.Metric{data.MetricOpen}, data.Daily, []float64{
				99, 100, 101, // a1 Open
				199, 200, 201, // a2 Open
			})
			Expect(err).NotTo(HaveOccurred())

			merged, err := data.MergeColumns(df1, df2)
			Expect(err).NotTo(HaveOccurred())

			closeCol := merged.Column(a1, data.MetricClose)
			Expect(closeCol).NotTo(BeNil())
			Expect(closeCol).To(Equal([]float64{100, 101, 102}))

			openCol := merged.Column(a1, data.MetricOpen)
			Expect(openCol).NotTo(BeNil())
			Expect(openCol).To(Equal([]float64{99, 100, 101}))

			closeCol2 := merged.Column(a2, data.MetricClose)
			Expect(closeCol2).NotTo(BeNil())
			Expect(closeCol2).To(Equal([]float64{200, 201, 202}))

			openCol2 := merged.Column(a2, data.MetricOpen)
			Expect(openCol2).NotTo(BeNil())
			Expect(openCol2).To(Equal([]float64{199, 200, 201}))
		})

		It("returns an empty frame when called with no arguments", func() {
			merged, err := data.MergeColumns()
			Expect(err).NotTo(HaveOccurred())
			Expect(merged.Len()).To(Equal(0))
		})
	})

	Describe("MergeTimes", func() {
		It("concatenates frames with non-overlapping time ranges", func() {
			times1 := []time.Time{
				time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			}
			times2 := []time.Time{
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
			}

			df1, err := data.NewDataFrame(times1, []asset.Asset{a1}, []data.Metric{data.MetricClose}, data.Daily, []float64{
				100, 101,
			})
			Expect(err).NotTo(HaveOccurred())

			df2, err := data.NewDataFrame(times2, []asset.Asset{a1}, []data.Metric{data.MetricClose}, data.Daily, []float64{
				102, 103,
			})
			Expect(err).NotTo(HaveOccurred())

			merged, err := data.MergeTimes(df1, df2)
			Expect(err).NotTo(HaveOccurred())
			Expect(merged.Len()).To(Equal(4))

			col := merged.Column(a1, data.MetricClose)
			Expect(col).NotTo(BeNil())
			Expect(col).To(Equal([]float64{100, 101, 102, 103}))

			mergedTimes := merged.Times()
			expectedTimes := append(times1, times2...)
			for idx, tm := range expectedTimes {
				Expect(mergedTimes[idx].Equal(tm)).To(BeTrue(), "time[%d] mismatch", idx)
			}
		})

		It("returns an error for overlapping time ranges", func() {
			times1 := []time.Time{
				time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			}
			times2 := []time.Time{
				time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			df1, err := data.NewDataFrame(times1, []asset.Asset{a1}, []data.Metric{data.MetricClose}, data.Daily, []float64{
				100, 101,
			})
			Expect(err).NotTo(HaveOccurred())

			df2, err := data.NewDataFrame(times2, []asset.Asset{a1}, []data.Metric{data.MetricClose}, data.Daily, []float64{
				101, 102,
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = data.MergeTimes(df1, df2)
			Expect(err).To(HaveOccurred())
		})
	})
})
