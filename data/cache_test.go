// Copyright 2021-2023
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
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/rs/zerolog/log"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/dataframe"
)

func tz() *time.Location {
	return common.GetTimezone()
}

type segment struct {
	begin int
	end   int
	df    *dataframe.DataFrame[time.Time]
}

var _ = Describe("Cache", func() {

	Context("with long date period with non-equal period and covered period", func() {
		var (
			cache    *data.SecurityMetricCache
			dates    []time.Time
			security *data.Security
		)

		BeforeEach(func() {
			dates = make([]time.Time, 365)
			dt := time.Date(2021, time.October, 31, 0, 0, 0, 0, tz())
			for idx := range dates {
				dates[idx] = dt
				dt = dt.AddDate(0, 0, 1)
			}

			cache = data.NewSecurityMetricCache(1024, dates)
			security = &data.Security{
				CompositeFigi: "test",
				Ticker:        "test",
			}
		})

		It("can get all values available", func() {
			df := &dataframe.DataFrame[time.Time]{
				Index: []time.Time{
					time.Date(2022, 8, 7, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
				},
				Vals: [][]float64{{7, 8, 9}},
			}

			err := cache.Set(security, data.MetricAdjustedClose, dates[125], dates[len(dates)-1], df)
			Expect(err).To(BeNil(), "set does not return error")
			df2, err := cache.Get(security, data.MetricAdjustedClose, dates[125], dates[len(dates)-1])
			Expect(err).To(BeNil(), "set does not return error")
			Expect(df2.Len()).To(BeNumerically("==", 3))
			Expect(df2.Index).To(Equal([]time.Time{
				time.Date(2022, 8, 7, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
			}))
			Expect(df2.Vals).To(Equal([][]float64{{7, 8, 9}}))
		})

		It("can get subset of values available (left)", func() {
			df := &dataframe.DataFrame[time.Time]{
				Index: []time.Time{
					time.Date(2022, 8, 7, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
				},
				Vals: [][]float64{{7, 8, 9}},
			}

			err := cache.Set(security, data.MetricAdjustedClose, dates[125], dates[len(dates)-1], df)
			Expect(err).To(BeNil(), "set does not return error")
			df2, err := cache.Get(security, data.MetricAdjustedClose, dates[125], time.Date(2022, 8, 8, 0, 0, 0, 0, tz()))
			Expect(err).To(BeNil(), "set does not return error")
			Expect(df2.Len()).To(BeNumerically("==", 2))
			Expect(df2.Index).To(Equal([]time.Time{
				time.Date(2022, 8, 7, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}))
			Expect(df2.Vals).To(Equal([][]float64{{7, 8}}))
		})

		It("can get a subset of values available (right)", func() {
			df := &dataframe.DataFrame[time.Time]{
				Index: []time.Time{
					time.Date(2022, 8, 7, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
				},
				Vals: [][]float64{{7, 8, 9}},
			}

			err := cache.Set(security, data.MetricAdjustedClose, dates[125], dates[len(dates)-1], df)
			Expect(err).To(BeNil(), "set does not return error")
			df2, err := cache.Get(security, data.MetricAdjustedClose, time.Date(2022, 8, 9, 0, 0, 0, 0, tz()), dates[len(dates)-1])
			Expect(err).To(BeNil(), "set does not return error")
			Expect(df2.Len()).To(BeNumerically("==", 1))
			Expect(df2.Index).To(Equal([]time.Time{
				time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
			}))
			Expect(df2.Vals).To(Equal([][]float64{{9}}))
		})

		It("can get a subset of values", func() {
			df := &dataframe.DataFrame[time.Time]{
				Index: []time.Time{
					time.Date(2022, 8, 7, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
				},
				Vals: [][]float64{{7, 8, 9}},
			}

			err := cache.Set(security, data.MetricAdjustedClose, dates[125], dates[len(dates)-1], df)
			Expect(err).To(BeNil(), "set does not return error")
			df2, err := cache.Get(security, data.MetricAdjustedClose, time.Date(2022, 8, 8, 0, 0, 0, 0, tz()), time.Date(2022, 8, 8, 0, 0, 0, 0, tz()))
			Expect(err).To(BeNil(), "set does not return error")
			Expect(df2.Len()).To(BeNumerically("==", 1))
			Expect(df2.Index).To(Equal([]time.Time{
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}))
			Expect(df2.Vals).To(Equal([][]float64{{8}}))
		})

		It("should merge overlapping cache items with local dates", func() {
			err := cache.SetWithLocalDates(security, data.MetricDividendCash,
				time.Date(2022, 6, 14, 0, 0, 0, 0, tz()), time.Date(2022, 11, 21, 0, 0, 0, 0, tz()),
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 6, 27, 0, 0, 0, 0, tz()), time.Date(2022, 7, 12, 0, 0, 0, 0, tz()), time.Date(2022, 7, 28, 0, 0, 0, 0, tz()), time.Date(2022, 11, 3, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{0.4, 0.4, 0.4, 0.4}},
				})
			Expect(err).To(BeNil(), "error during first cache set")

			err = cache.SetWithLocalDates(security, data.MetricDividendCash,
				time.Date(2022, 1, 20, 0, 0, 0, 0, tz()), time.Date(2022, 8, 21, 0, 0, 0, 0, tz()),
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 1, 4, 0, 0, 0, 0, tz()), time.Date(2022, 6, 27, 0, 0, 0, 0, tz()), time.Date(2022, 7, 12, 0, 0, 0, 0, tz()), time.Date(2022, 7, 28, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{0.39, 0.4, 0.4, 0.4}},
				})
			Expect(err).To(BeNil(), "error during first cache set")

			contains, _ := cache.Check(security, data.MetricDividendCash, time.Date(2022, 6, 27, 0, 0, 0, 0, tz()), time.Date(2022, 6, 27, 0, 0, 0, 0, tz()))
			Expect(contains).To(BeTrue(), "cache contains expected date")

			items := cache.Items(security, data.MetricDividendCash)
			Expect(items).To(HaveLen(1), "there should be only 1 CacheItem")
			Expect(items[0].Values).To(HaveLen(5), "Values should have length 5")
			Expect(items[0].Values).To(Equal([]float64{0.39, 0.4, 0.4, 0.4, 0.4}))
			Expect(items[0].LocalDateIndex()).To(HaveLen(5), "there should be 5 local dates")
			Expect(items[0].LocalDateIndex()).To(Equal([]time.Time{time.Date(2022, 1, 4, 0, 0, 0, 0, tz()), time.Date(2022, 6, 27, 0, 0, 0, 0, tz()), time.Date(2022, 7, 12, 0, 0, 0, 0, tz()), time.Date(2022, 7, 28, 0, 0, 0, 0, tz()), time.Date(2022, 11, 3, 0, 0, 0, 0, tz())}))
		})

		It("should merge overlapping cache items when only extending the covered period (left)", func() {
			err := cache.SetWithLocalDates(security, data.MetricDividendCash,
				time.Date(2022, 6, 14, 0, 0, 0, 0, tz()), time.Date(2022, 11, 21, 0, 0, 0, 0, tz()),
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 6, 27, 0, 0, 0, 0, tz()), time.Date(2022, 7, 12, 0, 0, 0, 0, tz()), time.Date(2022, 7, 28, 0, 0, 0, 0, tz()), time.Date(2022, 11, 3, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{0.4, 0.4, 0.4, 0.4}},
				})
			Expect(err).To(BeNil(), "error during first cache set")

			// second cache period is not a superset but has more dates covered on the left side
			err = cache.SetWithLocalDates(security, data.MetricDividendCash,
				time.Date(2022, 4, 14, 0, 0, 0, 0, tz()), time.Date(2022, 11, 18, 0, 0, 0, 0, tz()),
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 6, 27, 0, 0, 0, 0, tz()), time.Date(2022, 7, 12, 0, 0, 0, 0, tz()), time.Date(2022, 7, 28, 0, 0, 0, 0, tz()), time.Date(2022, 11, 3, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{0.4, 0.4, 0.4, 0.4}},
				})
			Expect(err).To(BeNil(), "error during first cache set")

			contains, _ := cache.Check(security, data.MetricDividendCash, time.Date(2022, 4, 27, 0, 0, 0, 0, tz()), time.Date(2022, 4, 27, 0, 0, 0, 0, tz()))
			Expect(contains).To(BeTrue(), "cache contains expected date")

			items := cache.Items(security, data.MetricDividendCash)
			Expect(items).To(HaveLen(1), "there should be only 1 CacheItem")
			Expect(items[0].Values).To(HaveLen(4), "Values should have length 4")
			Expect(items[0].Values).To(Equal([]float64{0.4, 0.4, 0.4, 0.4}))
			Expect(items[0].LocalDateIndex()).To(HaveLen(4), "there should be 4 local dates")
			Expect(items[0].LocalDateIndex()).To(Equal([]time.Time{time.Date(2022, 6, 27, 0, 0, 0, 0, tz()), time.Date(2022, 7, 12, 0, 0, 0, 0, tz()), time.Date(2022, 7, 28, 0, 0, 0, 0, tz()), time.Date(2022, 11, 3, 0, 0, 0, 0, tz())}))
		})

		It("should merge overlapping cache items when only extending the covered period (right)", func() {
			err := cache.SetWithLocalDates(security, data.MetricDividendCash,
				time.Date(2022, 6, 14, 0, 0, 0, 0, tz()), time.Date(2022, 11, 21, 0, 0, 0, 0, tz()),
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 6, 27, 0, 0, 0, 0, tz()), time.Date(2022, 7, 12, 0, 0, 0, 0, tz()), time.Date(2022, 7, 28, 0, 0, 0, 0, tz()), time.Date(2022, 11, 3, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{0.4, 0.4, 0.4, 0.4}},
				})
			Expect(err).To(BeNil(), "error during first cache set")

			// second cache period is not a superset but has more dates covered on the left side
			err = cache.SetWithLocalDates(security, data.MetricDividendCash,
				time.Date(2022, 6, 14, 0, 0, 0, 0, tz()), time.Date(2022, 12, 18, 0, 0, 0, 0, tz()),
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 6, 27, 0, 0, 0, 0, tz()), time.Date(2022, 7, 12, 0, 0, 0, 0, tz()), time.Date(2022, 7, 28, 0, 0, 0, 0, tz()), time.Date(2022, 11, 3, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{0.4, 0.4, 0.4, 0.4}},
				})
			Expect(err).To(BeNil(), "error during first cache set")

			contains, _ := cache.Check(security, data.MetricDividendCash, time.Date(2022, 12, 1, 0, 0, 0, 0, tz()), time.Date(2022, 12, 1, 0, 0, 0, 0, tz()))
			Expect(contains).To(BeTrue(), "cache contains expected date")

			items := cache.Items(security, data.MetricDividendCash)
			Expect(items).To(HaveLen(1), "there should be only 1 CacheItem")
			Expect(items[0].Values).To(HaveLen(4), "Values should have length 4")
			Expect(items[0].Values).To(Equal([]float64{0.4, 0.4, 0.4, 0.4}))
			Expect(items[0].LocalDateIndex()).To(HaveLen(4), "there should be 4 local dates")
			Expect(items[0].LocalDateIndex()).To(Equal([]time.Time{time.Date(2022, 6, 27, 0, 0, 0, 0, tz()), time.Date(2022, 7, 12, 0, 0, 0, 0, tz()), time.Date(2022, 7, 28, 0, 0, 0, 0, tz()), time.Date(2022, 11, 3, 0, 0, 0, 0, tz())}))
		})
	})

	Context("with ranges where covered period does not equal the whole period", func() {
		var (
			cache    *data.SecurityMetricCache
			dates    []time.Time
			security *data.Security
		)

		BeforeEach(func() {
			dates = []time.Time{
				time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 10, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 11, 0, 0, 0, 0, tz()),
			}

			cache = data.NewSecurityMetricCache(1024, dates)
			security = &data.Security{
				CompositeFigi: "test",
				Ticker:        "test",
			}
		})

		It("can set periods where the covered period and overall period are not equal", func() {
			df := &dataframe.DataFrame[time.Time]{
				Index: []time.Time{
					time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
				},
				Vals: [][]float64{{5, 8, 9}},
			}
			err := cache.Set(security, data.MetricAdjustedClose, dates[0], dates[6], df)
			Expect(err).To(BeNil(), "set does not return error")
			cacheItems := cache.Items(security, data.MetricAdjustedClose)
			Expect(cacheItems).To(HaveLen(1), "cache items has length 1")
			item := cacheItems[0]
			Expect(item.Period.Begin).To(Equal(dates[0]), "has correct period begin")
			Expect(item.Period.End).To(Equal(dates[6]), "has correct period end")
			Expect(item.CoveredPeriod.Begin).To(Equal(df.Index[0]), "has correct covered period begin")
			Expect(item.CoveredPeriod.End).To(Equal(df.Index[2]), "has correct covered period end")
		})

		DescribeTable("with different types of merges",
			func(segments []segment, expectedItemCount int, expectedSegs []segment) {
				for _, seg := range segments {
					err := cache.Set(
						security,
						data.MetricAdjustedClose,
						time.Date(2022, 8, seg.begin, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, seg.end, 0, 0, 0, 0, tz()),
						seg.df,
					)
					Expect(err).To(BeNil())
				}

				items := cache.Items(security, data.MetricAdjustedClose)

				for _, item := range items {
					log.Debug().Object("item", item).Msg("segments")
				}

				Expect(items).To(HaveLen(expectedItemCount), "item count")
				for idx := range items {
					Expect(items[idx].Period.Begin).To(Equal(time.Date(2022, 8, expectedSegs[idx].begin, 0, 0, 0, 0, tz())), fmt.Sprintf("item %d period begin", idx))
					Expect(items[idx].Period.End).To(Equal(time.Date(2022, 8, expectedSegs[idx].end, 0, 0, 0, 0, tz())), fmt.Sprintf("item %d period end", idx))
					Expect(items[idx].CoveredPeriod.Begin).To(Equal(expectedSegs[idx].df.Index[0]), fmt.Sprintf("item %d covered begin", idx))
					Expect(items[idx].CoveredPeriod.End).To(Equal(expectedSegs[idx].df.Index[len(expectedSegs[idx].df.Index)-1]), fmt.Sprintf("item %d covered end", idx))
					Expect(items[idx].Values).To(Equal(expectedSegs[idx].df.Vals[0]), fmt.Sprintf("item %d values", idx))
				}
			},
			Entry("touching covered period on left->right (both partial)",
				[]segment{
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2}}}},
					{3, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4}}}},
				},
				1, []segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2, 3, 4}}}},
				}),
			Entry("touching covered period on left->right (right partial)",
				[]segment{
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2}}}},
					{3, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4}}}},
				},
				1, []segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2, 3, 4}}}},
				}),
			Entry("touching covered period on left->right (left partial)",
				[]segment{
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2}}}},
					{3, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4, 5}}}},
				},
				1, []segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2, 3, 4, 5}}}},
				}),
			Entry("touching covered period on right->left (both partial)",
				[]segment{
					{3, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4}}}},
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2}}}},
				},
				1, []segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2, 3, 4}}}},
				}),
			Entry("touching covered period on right->left (right partial)",
				[]segment{
					{3, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4}}}},
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2}}}},
				},
				1, []segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2, 3, 4}}}},
				}),
			Entry("touching covered period on right->left (left partial)",
				[]segment{
					{3, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4, 5}}}},
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2}}}},
				},
				1, []segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2, 3, 4, 5}}}},
				}),
			Entry("touching covered period on left->left and right->right",
				[]segment{
					{3, 3, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3}}}},
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2}}}},
					{4, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4}}}},
				},
				1, []segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2, 3, 4}}}},
				}),
			Entry("non-touching covered period on left",
				[]segment{
					{3, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2}}}},
				},
				2, []segment{
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2}}}},
					{3, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
				}),
			Entry("converts a single segment with gaps into multiple segments",
				[]segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2, 4, 5}}}},
				},
				2, []segment{
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2}}}},
					{3, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
				}),
			Entry("properly merges overlapping segments",
				[]segment{
					{1, 4, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4}}}},
					{4, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
				},
				1, []segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
				},
			),
			Entry("overlapping segments 2",
				[]segment{
					{1, 4, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4}}}},
					{4, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
				},
				1, []segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4, 5}}}},
				},
			),
		)
	})

	Context("with values for a single security and metric using the cache date index", func() {
		var (
			cache    *data.SecurityMetricCache
			dates    []time.Time
			security *data.Security
		)

		BeforeEach(func() {
			dates = []time.Time{
				time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
			}

			cache = data.NewSecurityMetricCache(1024, dates)
			security = &data.Security{
				CompositeFigi: "test",
				Ticker:        "test",
			}
		})

		It("benchmarks performance", func() {
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
			dateIdx := []time.Time{
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
			}
			vals := []float64{0, 1, 2, 3, 4}

			experiment := gmeasure.NewExperiment("cache set")
			AddReportEntry(experiment.Name, experiment)

			experiment.SampleDuration("simple set", func(_ int) {
				cache = data.NewSecurityMetricCache(1024, dates)
				err := cache.Set(security, data.MetricAdjustedClose, begin, end, &dataframe.DataFrame[time.Time]{
					Index:    dateIdx,
					Vals:     [][]float64{vals},
					ColNames: []string{"vals"},
				})
				Expect(err).To(BeNil())
			}, gmeasure.SamplingConfig{N: 1000})

			cache = data.NewSecurityMetricCache(1024, dates)
			err := cache.Set(security, data.MetricAdjustedClose, begin, end, &dataframe.DataFrame[time.Time]{
				Index:    dateIdx,
				Vals:     [][]float64{vals},
				ColNames: []string{"vals"},
			})
			Expect(err).To(BeNil())

			beginSubset := time.Date(2022, 8, 4, 0, 0, 0, 0, tz())
			endSubset := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())

			experiment.SampleDuration("simple get", func(_ int) {
				val, err := cache.Get(security, data.MetricAdjustedClose, beginSubset, endSubset)
				Expect(err).To(BeNil())
				Expect(val.Vals[0]).To(Equal([]float64{1, 2, 3}))
			}, gmeasure.SamplingConfig{N: 1000})
		})

		It("should have default count of 0", func() {
			Expect(cache.Count()).To(Equal(0))
		})

		It("should successfully set values", func() {
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
			vals := []float64{0, 1, 2, 3, 4}
			dateIdx := []time.Time{
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
			}

			df := &dataframe.DataFrame[time.Time]{
				Index:    dateIdx,
				Vals:     [][]float64{vals},
				ColNames: []string{"vals"},
			}

			err := cache.Set(security, data.MetricAdjustedClose, begin, end, df)
			Expect(err).To(BeNil())

			Expect(cache.Count()).To(Equal(1), "number of metrics stored")
			Expect(cache.Size()).To(BeNumerically("==", 40), "cache size after set")
		})

		DescribeTable("check various time ranges",
			func(a, b int, expectedPresent bool, expectedInterval []*data.Interval) {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())
				vals := []float64{0, 1, 2, 3}
				dateIdx := []time.Time{
					time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}

				df := &dataframe.DataFrame[time.Time]{
					Index:    dateIdx,
					Vals:     [][]float64{vals},
					ColNames: []string{"vals"},
				}

				err := cache.Set(security, data.MetricAdjustedClose, begin, end, df)
				Expect(err).To(BeNil())

				// try to retrieve the data
				rangeA := time.Date(2022, 8, a, 0, 0, 0, 0, tz())
				rangeB := time.Date(2022, 8, b, 0, 0, 0, 0, tz())
				present, intervals := cache.Check(security, data.MetricAdjustedClose, rangeA, rangeB)

				Expect(present).To(Equal(expectedPresent))
				Expect(intervals).To(Equal(expectedInterval))
			},
			Entry("when range is completely outside of data interval (before start)", 1, 1, false, []*data.Interval{}),
			Entry("when range is before start and ends within interval", 2, 3, false, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range is before start and ends at interval end", 2, 8, false, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range is before start and ends after end", 2, 9, false, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range is after start and ends after end", 4, 9, false, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range is completely outside of data interval (after end)", 9, 9, false, []*data.Interval{}),
			Entry("when range is completely outside of covered interval (after end)", 10, 12, false, []*data.Interval{}),
			Entry("when range is invalid (start after end)", 5, 3, false, []*data.Interval{}),
			Entry("when range is covered but days only on weekend", 6, 7, true, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range completely covers available data", 3, 8, true, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range is a single date", 4, 4, true, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
		)

		DescribeTable("get various time ranges",
			func(a, b int, expected []float64, expectedError error) {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())
				vals := []float64{0, 1, 2, 3}
				dateIdx := []time.Time{
					time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}

				df := &dataframe.DataFrame[time.Time]{
					Index:    dateIdx,
					Vals:     [][]float64{vals},
					ColNames: []string{"vals"},
				}

				err := cache.Set(security, data.MetricAdjustedClose, begin, end, df)
				Expect(err).To(BeNil())

				// try to retrieve the data
				rangeA := time.Date(2022, 8, a, 0, 0, 0, 0, tz())
				rangeB := time.Date(2022, 8, b, 0, 0, 0, 0, tz())
				res, err := cache.Get(security, data.MetricAdjustedClose, rangeA, rangeB)

				if expectedError == nil {
					Expect(err).To(BeNil())

					if len(expected) > 0 {
						Expect(len(res.Vals)).To(Equal(1), "number of columns in df should be one")
						if len(res.Vals) > 0 {
							Expect(res.Vals[0]).To(Equal(expected))
						}
					} else {
						Expect(len(res.Vals)).To(Equal(1), "number of columns in df should be one")
						Expect(len(res.Vals[0])).To(Equal(0), "number of values should be zero")
					}
				} else {
					Expect(errors.Is(err, expectedError)).To(BeTrue())
				}
			},
			Entry("when range is completely outside of data interval (before start)", 1, 1, nil, data.ErrRangeDoesNotExist),
			Entry("when range is before start and ends within interval", 2, 3, nil, data.ErrRangeDoesNotExist),
			Entry("when range is before start and ends at interval end", 2, 8, nil, data.ErrRangeDoesNotExist),
			Entry("when range is before start and ends after end", 2, 9, nil, data.ErrRangeDoesNotExist),
			Entry("when range is after start and ends after end", 4, 9, nil, data.ErrRangeDoesNotExist),
			Entry("when range is completely outside of data interval (after end)", 9, 9, nil, data.ErrRangeDoesNotExist),
			Entry("when range is completely outside of covered interval (after end)", 10, 12, nil, data.ErrRangeDoesNotExist),
			Entry("when range is invalid (start after end)", 5, 3, nil, data.ErrInvalidTimeRange),
			Entry("when range is covered but days only on weekend", 6, 7, []float64{}, nil),
			Entry("when range is covered but single only on weekend", 6, 6, []float64{}, nil),
			Entry("when range is a subset of covered period", 4, 5, []float64{1, 2}, nil),
			Entry("range touches left extremity", 3, 5, []float64{0, 1, 2}, nil),
			Entry("range touches right extremity", 5, 8, []float64{2, 3}, nil),
			Entry("range covers full period", 3, 8, []float64{0, 1, 2, 3}, nil),
			Entry("range covers a single day 0", 3, 3, []float64{0}, nil),
			Entry("range covers a single day 1", 4, 4, []float64{1}, nil),
			Entry("range covers a single day 2", 5, 5, []float64{2}, nil),
			Entry("range covers a single day 3", 8, 8, []float64{3}, nil),
			Entry("range begins on weekend", 6, 8, []float64{3}, nil),
			Entry("range ends on saturday", 3, 6, []float64{0, 1, 2}, nil),
			Entry("range ends on sunday", 3, 7, []float64{0, 1, 2}, nil),
			Entry("when range is a single date", 4, 4, []float64{1}, nil),
		)

		DescribeTable("get partial for various time ranges",
			func(a, b int, expected []float64) {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())
				vals := []float64{3, 4, 5, 8}
				dateIdx := []time.Time{
					time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}

				df := &dataframe.DataFrame[time.Time]{
					Index:    dateIdx,
					Vals:     [][]float64{vals},
					ColNames: []string{"vals"},
				}

				err := cache.Set(security, data.MetricAdjustedClose, begin, end, df)
				Expect(err).To(BeNil())

				// try to retrieve the data
				rangeA := time.Date(2022, 8, a, 0, 0, 0, 0, tz())
				rangeB := time.Date(2022, 8, b, 0, 0, 0, 0, tz())
				res := cache.GetPartial(security, data.MetricAdjustedClose, rangeA, rangeB)

				if len(expected) > 0 {
					Expect(len(res.Vals)).To(Equal(1), "number of columns in df should be one")
					if len(res.Vals) > 0 {
						Expect(res.Vals[0]).To(Equal(expected))
					}
				} else {
					Expect(len(res.Vals)).To(Equal(1), "number of columns in df should be zero")
					Expect(len(res.Vals[0])).To(Equal(0), "number of values should be zero")
				}
			},
			Entry("when range is completely outside of data interval (before start)", 1, 1, []float64{}),
			Entry("when range is before start and ends within interval", 2, 3, []float64{3}),
			Entry("when range is before start and ends at interval end", 2, 8, []float64{3, 4, 5, 8}),
			Entry("when range is before start and ends after end", 2, 9, []float64{3, 4, 5, 8}),
			Entry("when range is after start and ends after end", 4, 9, []float64{4, 5, 8}),
			Entry("when range is completely outside of data interval (after end)", 9, 9, []float64{}),
			Entry("when range is completely outside of covered interval (after end)", 10, 12, []float64{}),
			Entry("when range is invalid (start after end)", 5, 3, []float64{}),
			Entry("when range is covered but days only on weekend", 6, 7, []float64{}),
			Entry("when range is covered but single only on weekend", 6, 6, []float64{}),
			Entry("when range is a subset of covered period", 4, 5, []float64{4, 5}),
			Entry("range touches left extremity", 3, 5, []float64{3, 4, 5}),
			Entry("range touches right extremity", 5, 8, []float64{5, 8}),
			Entry("range covers full period", 3, 8, []float64{3, 4, 5, 8}),
			Entry("range covers a single day 0", 3, 3, []float64{3}),
			Entry("range covers a single day 1", 4, 4, []float64{4}),
			Entry("range covers a single day 2", 5, 5, []float64{5}),
			Entry("range covers a single day 3", 8, 8, []float64{8}),
			Entry("range begins on weekend", 6, 8, []float64{8}),
			Entry("range ends on saturday", 3, 6, []float64{3, 4, 5}),
			Entry("range ends on sunday", 3, 7, []float64{3, 4, 5}),
			Entry("when range is a single date", 4, 4, []float64{4}),
		)

		DescribeTable("test merging of values",
			func(segments []segment, expectedItemCount, a, b int, expectedVals []float64) {
				// set each segment on the cache, this should cause merging
				for _, seg := range segments {
					begin := time.Date(2022, 8, seg.begin, 0, 0, 0, 0, tz())
					end := time.Date(2022, 8, seg.end, 0, 0, 0, 0, tz())
					err := cache.Set(security, data.MetricAdjustedClose, begin, end, seg.df)
					Expect(err).To(BeNil())
				}

				// check the number of items for the security / metric pair
				itemCount := cache.ItemCount(security, data.MetricAdjustedClose)
				Expect(itemCount).To(Equal(expectedItemCount), "item count")

				// validate that multiple items are properly sorted
				items := cache.Items(security, data.MetricAdjustedClose)
				if len(items) > 1 {
					lastItem := items[0]
					log.Debug().Time("PeriodBegin", lastItem.Period.Begin).Time("PeriodEnd", lastItem.Period.End).Floats64("Vals", lastItem.Values).Msg("cache item")
					for _, item := range items[1:] {
						log.Debug().Time("PeriodBegin", item.Period.Begin).Time("PeriodEnd", item.Period.End).Floats64("Vals", item.Values).Msg("cache item")
						Expect(item.Period.Begin.Before(lastItem.Period.End)).To(BeFalse(), "item period begin should be after the previous end")
						lastItem = item
					}
				} else if len(items) == 1 {
					item := items[0]
					log.Debug().Time("PeriodBegin", item.Period.Begin).Time("PeriodEnd", item.Period.End).Floats64("Vals", item.Values).Msg("cache item")
				}

				// try to retrieve the data
				rangeA := time.Date(2022, 8, a, 0, 0, 0, 0, tz())
				rangeB := time.Date(2022, 8, b, 0, 0, 0, 0, tz())
				res, err := cache.Get(security, data.MetricAdjustedClose, rangeA, rangeB)
				Expect(err).To(BeNil())
				Expect(res.Vals[0]).To(Equal(expectedVals))
			},
			Entry("when segments are non contiguous",
				[]segment{
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2}}}},
					{4, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
				},
				2, 4, 5, []float64{4, 5}),
			Entry("when segments are left contiguous",
				[]segment{
					{3, 4, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4}}}},
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2}}}},
				},
				1, 1, 4, []float64{1, 2, 3, 4}),
			Entry("when segments are right contiguous",
				[]segment{
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2}}}},
					{3, 4, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4}}}},
				},
				1, 1, 4, []float64{1, 2, 3, 4}),
			Entry("when segments are left contiguous (single)",
				[]segment{
					{2, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2}}}},
					{1, 1, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1}}}},
				}, 1, 1, 2, []float64{1, 2}),
			Entry("when segments are right contiguous (single)",
				[]segment{
					{1, 1, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1}}}},
					{2, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2}}}},
				}, 1, 1, 2, []float64{1, 2}),
			Entry("when segments are equal",
				[]segment{
					{4, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
					{4, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
				}, 1, 4, 5, []float64{4, 5}),
			Entry("when segment A is a subset of segment B",
				[]segment{
					{3, 4, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4}}}},
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2, 3, 4, 5}}}},
				}, 1, 1, 5, []float64{1, 2, 3, 4, 5}),
			Entry("when segment B is a subset of segment A",
				[]segment{
					{1, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2, 3, 4, 5}}}},
					{3, 4, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{3, 4}}}},
				}, 1, 1, 5, []float64{1, 2, 3, 4, 5}),
			Entry("when segments are left contiguous (weekend)",
				[]segment{
					{8, 9, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{8, 9}}}},
					{4, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
				}, 1, 4, 9, []float64{4, 5, 8, 9}),
			Entry("when segments are right contiguous (weekend)",
				[]segment{
					{4, 5, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{4, 5}}}},
					{8, 9, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{8, 9}}}},
				}, 1, 4, 9, []float64{4, 5, 8, 9}),
			Entry("when segments are right contiguous with overlap",
				[]segment{
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2}}}},
					{2, 3, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2, 3}}}},
				}, 1, 1, 3, []float64{1, 2, 3}),
			Entry("when segments are left contiguous with overlap",
				[]segment{
					{2, 3, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{2, 3}}}},
					{1, 2, &dataframe.DataFrame[time.Time]{
						Index: []time.Time{
							time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
							time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						},
						Vals: [][]float64{{1, 2}}}},
				}, 1, 1, 3, []float64{1, 2, 3}),
		)

		It("should collapse multiple overlapping cache items", func() {
			// add two single item entries into the cache that are separated by a day
			err := cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), &dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 1, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{1}},
				})
			Expect(err).To(BeNil(), "error during first cache set")
			err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), &dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 3, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{3}},
				})
			Expect(err).To(BeNil(), "error during second cache set")

			// make sure there are 2 entries in the []*CacheItem list
			Expect(cache.ItemCount(security, data.MetricAdjustedClose)).To(Equal(2), "item count after 2 sets")

			// now add a *CacheItem that connects the previous two entries
			err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 2, 0, 0, 0, 0, tz()), &dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 2, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{2}},
				})
			Expect(err).To(BeNil(), "error during second cache set")

			// check the number of items for the security / metric pair
			itemCount := cache.ItemCount(security, data.MetricAdjustedClose)
			Expect(itemCount).To(Equal(1), "item count after 3 sets")

			// validate that multiple items are properly sorted
			rangeA := time.Date(2022, 8, 1, 0, 0, 0, 0, tz())
			rangeB := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			item := cache.Items(security, data.MetricAdjustedClose)[0]
			Expect(item.Period.Begin).To(Equal(rangeA))
			Expect(item.Period.End).To(Equal(rangeB))

			// try to retrieve the data
			res, err := cache.Get(security, data.MetricAdjustedClose, rangeA, rangeB)
			Expect(err).To(BeNil())
			Expect(res.Vals[0]).To(Equal([]float64{1, 2, 3}), "value for security/metric in cache")
		})

		It("should not collapse if items don't overlap", func() {
			// add two single item entries into the cache that are separated by a day
			err := cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), &dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 1, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{1}},
				})
			Expect(err).To(BeNil(), "error during first cache set")
			err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), &dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 3, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{3}},
				})
			Expect(err).To(BeNil(), "error during second cache set")

			// make sure there are 2 entries in the []*CacheItem list
			Expect(cache.ItemCount(security, data.MetricAdjustedClose)).To(Equal(2), "item count after 2 sets")

			// now add a *CacheItem that connects the previous two entries
			err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 4, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{4}},
				})
			Expect(err).To(BeNil(), "error during second cache set")

			// check the number of items for the security / metric pair
			itemCount := cache.ItemCount(security, data.MetricAdjustedClose)
			Expect(itemCount).To(Equal(2), "item count after 3 sets")

			// validate that multiple items are properly sorted
			rangeA := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			rangeB := time.Date(2022, 8, 4, 0, 0, 0, 0, tz())
			item := cache.Items(security, data.MetricAdjustedClose)[1]
			Expect(item.Period.Begin).To(Equal(rangeA))
			Expect(item.Period.End).To(Equal(rangeB))

			// try to retrieve the data
			res, err := cache.Get(security, data.MetricAdjustedClose, rangeA, rangeB)
			Expect(err).To(BeNil())
			Expect(res.Vals[0]).To(Equal([]float64{3, 4}), "value for security/metric in cache")
		})

		It("should successfully get values", func() {
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
			vals := []float64{0, 1, 2, 3, 4}
			dateIdx := []time.Time{
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
			}

			df := &dataframe.DataFrame[time.Time]{
				Index:    dateIdx,
				Vals:     [][]float64{vals},
				ColNames: []string{"vals"},
			}

			err := cache.Set(security, data.MetricAdjustedClose, begin, end, df)
			Expect(err).To(BeNil(), "cache set")

			result, err := cache.Get(security, data.MetricAdjustedClose, begin, end)
			Expect(err).To(BeNil())
			Expect(result.Vals[0]).To(Equal(vals))
		})

		It("should error for metric that doesn't exist", func() {
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
			_, err := cache.Get(security, data.MetricClose, begin, end)
			Expect(errors.Is(err, data.ErrRangeDoesNotExist)).To(BeTrue(), "error should be metric does not exist")
		})

		It("should error for security that doesn't exist", func() {
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
			security2 := &data.Security{
				CompositeFigi: "second",
				Ticker:        "second",
			}
			_, err := cache.Get(security2, data.MetricClose, begin, end)
			Expect(errors.Is(err, data.ErrRangeDoesNotExist)).To(BeTrue(), "error should be security does not exist")
		})

		It("should error when Set size is greater than total size of cache", func() {
			cache = data.NewSecurityMetricCache(2, dates)
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
			vals := []float64{0, 1, 2, 3, 4}

			dateIdx := []time.Time{
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
			}

			df := &dataframe.DataFrame[time.Time]{
				Index:    dateIdx,
				Vals:     [][]float64{vals},
				ColNames: []string{"vals"},
			}

			err := cache.Set(security, data.MetricAdjustedClose, begin, end, df)
			Expect(errors.Is(err, data.ErrDataLargerThanCache)).To(BeTrue(), "error should be data larger than cache")
		})

		It("should evict old data when cumulative sets exceed the configured memory", func() {
			cache = data.NewSecurityMetricCache(16, dates)
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 4, 0, 0, 0, 0, tz())
			vals := []float64{0, 1}

			dateIdx := []time.Time{
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
			}

			df := &dataframe.DataFrame[time.Time]{
				Index:    dateIdx,
				Vals:     [][]float64{vals},
				ColNames: []string{"vals"},
			}

			err := cache.Set(security, data.MetricAdjustedClose, begin, end, df)
			Expect(err).To(BeNil())

			begin2 := time.Date(2022, 8, 5, 0, 0, 0, 0, tz())
			end2 := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())
			vals2 := []float64{2, 3}

			dateIdx2 := []time.Time{
				time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}

			df2 := &dataframe.DataFrame[time.Time]{
				Index:    dateIdx2,
				Vals:     [][]float64{vals2},
				ColNames: []string{"vals"},
			}

			err = cache.Set(security, data.MetricAdjustedClose, begin2, end2, df2)
			Expect(err).To(BeNil())

			_, err = cache.Get(security, data.MetricAdjustedClose, begin, end)
			Expect(errors.Is(err, data.ErrRangeDoesNotExist)).To(BeTrue(), "error should be range does not exist")

			result, err := cache.Get(security, data.MetricAdjustedClose, begin2, end2)
			Expect(err).To(BeNil())
			Expect(result.Vals[0]).To(Equal(vals2))
		})

	})

	Context("with values for a single security and metric using a local date index", func() {
		var (
			cache    *data.SecurityMetricCache
			dates    []time.Time
			security *data.Security
		)

		BeforeEach(func() {
			dates = []time.Time{
				time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
			}

			cache = data.NewSecurityMetricCache(1024, dates)
			security = &data.Security{
				CompositeFigi: "test",
				Ticker:        "test",
			}
		})

		It("should successfully set values", func() {
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
			localDates := []time.Time{time.Date(2022, 8, 4, 0, 0, 0, 0, tz()), time.Date(2022, 8, 6, 0, 0, 0, 0, tz())}
			vals := []float64{4, 5}

			df := &dataframe.DataFrame[time.Time]{
				Index:    localDates,
				Vals:     [][]float64{vals},
				ColNames: []string{"vals"},
			}

			err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end, df)
			Expect(err).To(BeNil())

			Expect(cache.Count()).To(Equal(1), "number of metrics stored")
			Expect(cache.Size()).To(BeNumerically("==", 16), "cache size after set")

			items := cache.Items(security, data.MetricAdjustedClose)
			Expect(items[0].IsLocalDateIndex()).To(BeTrue(), "check if using a local date index")
		})

		It("should refuse to set values with unequal length date and val arrays", func() {
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
			localDates := []time.Time{time.Date(2022, 8, 4, 0, 0, 0, 0, tz()), time.Date(2022, 8, 6, 0, 0, 0, 0, tz())}
			vals := []float64{4}

			df := &dataframe.DataFrame[time.Time]{
				Index:    localDates,
				Vals:     [][]float64{vals},
				ColNames: []string{"vals"},
			}

			err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end, df)
			Expect(errors.Is(err, data.ErrDateLengthDoesNotMatch)).To(BeTrue(), "check if set caused an error")
		})

		DescribeTable("check various time ranges",
			func(a, b int, expectedPresent bool, expectedInterval []*data.Interval) {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())
				dt := []time.Time{time.Date(2022, 8, 4, 0, 0, 0, 0, tz())}
				vals := []float64{5.2}

				df := &dataframe.DataFrame[time.Time]{
					Index:    dt,
					Vals:     [][]float64{vals},
					ColNames: []string{"vals"},
				}

				err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end, df)
				Expect(err).To(BeNil())

				// try to retrieve the data
				rangeA := time.Date(2022, 8, a, 0, 0, 0, 0, tz())
				rangeB := time.Date(2022, 8, b, 0, 0, 0, 0, tz())
				present, intervals := cache.Check(security, data.MetricAdjustedClose, rangeA, rangeB)

				Expect(present).To(Equal(expectedPresent))
				Expect(intervals).To(Equal(expectedInterval))
			},
			Entry("when range is completely outside of data interval (before start)", 1, 1, false, []*data.Interval{}),
			Entry("when range is before start and ends within interval", 2, 3, false, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range is before start and ends at interval end", 2, 8, false, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range is before start and ends after end", 2, 9, false, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range is after start and ends after end", 4, 9, false, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range is completely outside of data interval (after end)", 9, 9, false, []*data.Interval{}),
			Entry("when range is completely outside of covered interval (after end)", 10, 12, false, []*data.Interval{}),
			Entry("when range is invalid (start after end)", 5, 3, false, []*data.Interval{}),
			Entry("when range is covered but days only on weekend", 6, 7, true, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range completely covers available data", 3, 8, true, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
			Entry("when range is a single date", 4, 4, true, []*data.Interval{{
				Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
			}}),
		)

		DescribeTable("get various time ranges",
			func(a, b int, expected []float64, expectedError error) {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())
				dt := []time.Time{time.Date(2022, 8, 4, 0, 0, 0, 0, tz())}
				vals := []float64{2}

				df := &dataframe.DataFrame[time.Time]{
					Index:    dt,
					Vals:     [][]float64{vals},
					ColNames: []string{"vals"},
				}

				err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end, df)
				Expect(err).To(BeNil())

				// try to retrieve the data
				rangeA := time.Date(2022, 8, a, 0, 0, 0, 0, tz())
				rangeB := time.Date(2022, 8, b, 0, 0, 0, 0, tz())
				res, err := cache.Get(security, data.MetricAdjustedClose, rangeA, rangeB)

				if expectedError == nil {
					Expect(err).To(BeNil())

					Expect(len(res.Vals)).To(Equal(1), "number of columns in df should be one")
					if len(res.Vals) > 0 {
						Expect(res.Vals[0]).To(Equal(expected))
					}
				} else {
					Expect(errors.Is(err, expectedError)).To(BeTrue())
				}
			},
			Entry("when range is completely outside of data interval (before start)", 1, 1, nil, data.ErrRangeDoesNotExist),
			Entry("when range is before start and ends within interval", 2, 3, nil, data.ErrRangeDoesNotExist),
			Entry("when range is before start and ends at interval end", 2, 8, nil, data.ErrRangeDoesNotExist),
			Entry("when range is before start and ends after end", 2, 9, nil, data.ErrRangeDoesNotExist),
			Entry("when range is after start and ends after end", 4, 9, nil, data.ErrRangeDoesNotExist),
			Entry("when range is completely outside of data interval (after end)", 9, 9, nil, data.ErrRangeDoesNotExist),
			Entry("when range is completely outside of covered interval (after end)", 10, 12, nil, data.ErrRangeDoesNotExist),
			Entry("when range is invalid (start after end)", 5, 3, nil, data.ErrInvalidTimeRange),
			Entry("when range is covered but days only on weekend", 6, 7, []float64{}, nil),
			Entry("when range is a subset of covered period", 4, 5, []float64{2}, nil),
			Entry("range touches left extremity", 3, 5, []float64{2}, nil),
			Entry("range touches right extremity", 5, 8, []float64{}, nil),
			Entry("range covers full period", 3, 8, []float64{2}, nil),
			Entry("range covers a single day 0", 3, 3, []float64{}, nil),
			Entry("range covers a single day 1", 4, 4, []float64{2}, nil),
			Entry("range covers a single day 2", 5, 5, []float64{}, nil),
			Entry("range covers a single day 3", 8, 8, []float64{}, nil),
			Entry("range begins on weekend", 6, 8, []float64{}, nil),
			Entry("range ends on saturday", 3, 6, []float64{2}, nil),
			Entry("range ends on sunday", 3, 6, []float64{2}, nil),
			Entry("when range is a single date", 4, 4, []float64{2}, nil),
		)

		DescribeTable("test merging of values",
			func(segments []segment, expectedItemCount, a, b int, expectedVals []float64) {
				// set each segment on the cache, this should cause merging
				for _, seg := range segments {
					begin := time.Date(2022, 8, seg.begin, 0, 0, 0, 0, tz())
					end := time.Date(2022, 8, seg.end, 0, 0, 0, 0, tz())
					err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end, seg.df)
					Expect(err).To(BeNil())
				}

				// check the number of items for the security / metric pair
				itemCount := cache.ItemCount(security, data.MetricAdjustedClose)
				Expect(itemCount).To(Equal(expectedItemCount), "item count")

				// validate that multiple items are properly sorted
				items := cache.Items(security, data.MetricAdjustedClose)
				if len(items) > 1 {
					lastItem := items[0]
					log.Debug().Time("PeriodBegin", lastItem.Period.Begin).Time("PeriodEnd", lastItem.Period.End).Floats64("Vals", lastItem.Values).Msg("cache item")
					for _, item := range items[1:] {
						log.Debug().Time("PeriodBegin", item.Period.Begin).Time("PeriodEnd", item.Period.End).Floats64("Vals", item.Values).Msg("cache item")
						Expect(item.Period.Begin.Before(lastItem.Period.End)).To(BeFalse(), "item period begin should be after the previous end")
						lastItem = item
					}
				} else if len(items) == 1 {
					item := items[0]
					log.Debug().Time("PeriodBegin", item.Period.Begin).Time("PeriodEnd", item.Period.End).Floats64("Vals", item.Values).Msg("cache item")
				}

				// try to retrieve the data
				rangeA := time.Date(2022, 8, a, 0, 0, 0, 0, tz())
				rangeB := time.Date(2022, 8, b, 0, 0, 0, 0, tz())
				res, err := cache.Get(security, data.MetricAdjustedClose, rangeA, rangeB)
				Expect(err).To(BeNil())
				Expect(res.Vals[0]).To(Equal(expectedVals))
			},
			Entry("when segments are non contiguous", []segment{
				{1, 2, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1, 2}},
				}},
				{4, 5, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{4, 5}},
				}},
			}, 2, 4, 5, []float64{4, 5}),
			Entry("when segments are left contiguous", []segment{
				{3, 4, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{3, 4}},
				}},
				{1, 2, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1, 2}},
				}},
			}, 1, 1, 4, []float64{1, 2, 3, 4}),
			Entry("when segments are right contiguous", []segment{
				{1, 2, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1, 2}},
				}},
				{3, 4, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{3, 4}},
				}},
			}, 1, 1, 4, []float64{1, 2, 3, 4}),
			Entry("when segments are left contiguous (single)", []segment{
				{2, 2, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{2}},
				}},
				{1, 1, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1}},
				}},
			}, 1, 1, 2, []float64{1, 2}),
			Entry("when segments are right contiguous (single)", []segment{
				{1, 1, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1}},
				}},
				{2, 2, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{2}},
				}},
			}, 1, 1, 2, []float64{1, 2}),
			Entry("when segments are equal", []segment{
				{4, 5, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{4, 5}},
				}},
				{4, 5, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{4, 5}},
				}},
			}, 1, 4, 5, []float64{4, 5}),
			Entry("when segment A is a subset of segment B", []segment{
				{3, 4, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{3, 4}},
				}},
				{1, 5, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1, 2, 3, 4, 5}},
				}},
			}, 1, 1, 5, []float64{1, 2, 3, 4, 5}),
			Entry("when segment B is a subset of segment A", []segment{
				{1, 5, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1, 2, 3, 4, 5}},
				}},
				{3, 4, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{3, 4}},
				}},
			}, 1, 1, 5, []float64{1, 2, 3, 4, 5}),
			Entry("when segments are left contiguous (weekend)", []segment{
				{8, 9, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{8, 9}},
				}},
				{4, 5, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{4, 5}},
				}},
			}, 1, 4, 9, []float64{4, 5, 8, 9}),
			Entry("when segments are right contiguous (weekend)", []segment{
				{4, 5, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{4, 5}},
				}},
				{8, 9, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 9, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{8, 9}},
				}},
			}, 1, 4, 9, []float64{4, 5, 8, 9}),
			Entry("when segments are right contiguous with overlap", []segment{
				{1, 2, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1, 2}},
				}},
				{2, 3, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{2, 3}},
				}},
			}, 1, 1, 3, []float64{1, 2, 3}),
			Entry("when segments overlap by 2 items on the right", []segment{
				{1, 3, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1, 2, 3}},
				}},
				{2, 4, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{2, 3, 4}},
				}},
			}, 1, 1, 4, []float64{1, 2, 3, 4}),
			Entry("when segments are left contiguous with overlap", []segment{
				{2, 3, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{2, 3}},
				}},
				{1, 2, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1, 2}},
				}},
			}, 1, 1, 3, []float64{1, 2, 3}),
			Entry("when segments are left contiguous with overlap (more items)", []segment{
				{3, 5, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 5, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{3, 4, 5}},
				}},
				{1, 3, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1, 2, 3}},
				}},
			}, 1, 1, 5, []float64{1, 2, 3, 4, 5}),
			Entry("when segments are sparse but they overlap", []segment{
				{1, 3, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{1, 3}},
				}},
				{3, 4, &dataframe.DataFrame[time.Time]{
					Index: []time.Time{
						time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
						time.Date(2022, 8, 4, 0, 0, 0, 0, tz()),
					},
					Vals: [][]float64{{3, 4}},
				}},
			}, 1, 1, 4, []float64{1, 3, 4}),
		)
		It("should collapse multiple overlapping cache items", func() {
			// add two single item entries into the cache that are separated by a day
			err := cache.SetWithLocalDates(security, data.MetricAdjustedClose,
				time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), time.Date(2022, 8, 1, 0, 0, 0, 0, tz()),
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 1, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{1}},
				})
			Expect(err).To(BeNil(), "error during first cache set")
			err = cache.SetWithLocalDates(security, data.MetricAdjustedClose,
				time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 3, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{3}},
				})
			Expect(err).To(BeNil(), "error during second cache set")

			// make sure there are 2 entries in the []*CacheItem list
			Expect(cache.ItemCount(security, data.MetricAdjustedClose)).To(Equal(2), "item count after 2 sets")

			// now add a *CacheItem that connects the previous two entries
			err = cache.SetWithLocalDates(security, data.MetricAdjustedClose,
				time.Date(2022, 8, 2, 0, 0, 0, 0, tz()), time.Date(2022, 8, 2, 0, 0, 0, 0, tz()),
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 2, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{{2}},
				})
			Expect(err).To(BeNil(), "error during second cache set")

			// check the number of items for the security / metric pair
			itemCount := cache.ItemCount(security, data.MetricAdjustedClose)
			Expect(itemCount).To(Equal(1), "item count after 3 sets")

			// validate that multiple items are properly sorted
			rangeA := time.Date(2022, 8, 1, 0, 0, 0, 0, tz())
			rangeB := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			item := cache.Items(security, data.MetricAdjustedClose)[0]
			Expect(item.Period.Begin).To(Equal(rangeA))
			Expect(item.Period.End).To(Equal(rangeB))

			// try to retrieve the data
			res, err := cache.Get(security, data.MetricAdjustedClose, rangeA, rangeB)
			Expect(err).To(BeNil())
			Expect(res.Vals[0]).To(Equal([]float64{1, 2, 3}), "value for security/metric in cache")

			// confirm that the local date index is properly merged
			Expect(item.LocalDateIndex()).To(Equal([]time.Time{time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), time.Date(2022, 8, 2, 0, 0, 0, 0, tz()), time.Date(2022, 8, 3, 0, 0, 0, 0, tz())}))
		})

		It("should successfully get values", func() {
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
			vals := []float64{4, 5}

			err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end,
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 4, 0, 0, 0, 0, tz()), time.Date(2022, 8, 5, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{vals},
				})
			Expect(err).To(BeNil(), "SetWithLocalDates error check")

			result, err := cache.Get(security, data.MetricAdjustedClose, begin, end)
			Expect(err).To(BeNil())
			Expect(result.Vals[0]).To(Equal(vals))
		})

		It("should error when Set size is greater than total size of cache", func() {
			cache = data.NewSecurityMetricCache(2, dates)
			begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
			end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
			vals := []float64{0, 1, 2}

			err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end,
				&dataframe.DataFrame[time.Time]{
					ColNames: []string{"vals"},
					Index:    []time.Time{time.Date(2022, 8, 4, 0, 0, 0, 0, tz()), time.Date(2022, 8, 5, 0, 0, 0, 0, tz()), time.Date(2022, 8, 6, 0, 0, 0, 0, tz())},
					Vals:     [][]float64{vals},
				})
			Expect(errors.Is(err, data.ErrDataLargerThanCache)).To(BeTrue(), "error should be data larger than cache")
		})
	})
})
