// Copyright 2021-2022
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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
)

func tz() *time.Location {
	return common.GetTimezone()
}

var _ = Describe("Cache", func() {
	Describe("When the cache is initialized ", func() {
		Context("with contiguous values for a single security and metric", func() {
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
				vals := []float64{0, 1, 2, 3, 4}

				experiment := gmeasure.NewExperiment("cache set")
				AddReportEntry(experiment.Name, experiment)

				experiment.SampleDuration("simple set", func(_ int) {
					cache = data.NewSecurityMetricCache(1024, dates)
					err := cache.Set(security, data.MetricAdjustedClose, begin, end, vals)
					Expect(err).To(BeNil())
				}, gmeasure.SamplingConfig{N: 1000})

				cache = data.NewSecurityMetricCache(1024, dates)
				err := cache.Set(security, data.MetricAdjustedClose, begin, end, vals)
				Expect(err).To(BeNil())

				beginSubset := time.Date(2022, 8, 4, 0, 0, 0, 0, tz())
				endSubset := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())

				experiment.SampleDuration("simple get", func(_ int) {
					val, err := cache.Get(security, data.MetricAdjustedClose, beginSubset, endSubset)
					Expect(err).To(BeNil())
					Expect(val).To(Equal([]float64{1, 2, 3}))
				}, gmeasure.SamplingConfig{N: 1000})
			})

			It("should have default count of 0", func() {
				Expect(cache.Count()).To(Equal(0))
			})

			It("should successfully set values", func() {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
				vals := []float64{0, 1, 2, 3, 4}

				err := cache.Set(security, data.MetricAdjustedClose, begin, end, vals)
				Expect(err).To(BeNil())

				Expect(cache.Count()).To(Equal(1), "number of metrics stored")
				Expect(cache.Size()).To(BeNumerically("==", 40), "cache size after set")
			})

			DescribeTable("check various time ranges",
				func(a, b int, expectedPresent bool, expectedInterval []*data.Interval) {
					begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
					end := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())
					vals := []float64{0, 1, 2, 3}
					err := cache.Set(security, data.MetricAdjustedClose, begin, end, vals)
					Expect(err).To(BeNil())

					// try to retrieve the data
					rangeA := time.Date(2022, 8, a, 0, 0, 0, 0, tz())
					rangeB := time.Date(2022, 8, b, 0, 0, 0, 0, tz())
					present, intervals := cache.Check(security, data.MetricAdjustedClose, rangeA, rangeB)

					Expect(present).To(Equal(expectedPresent))
					Expect(intervals).To(Equal(expectedInterval))
				},
				Entry("When range is completely outside of data interval (before start)", 1, 1, false, []*data.Interval{}),
				Entry("When range is before start and ends within interval", 2, 3, false, []*data.Interval{{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}}),
				Entry("When range is before start and ends at interval end", 2, 8, false, []*data.Interval{{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}}),
				Entry("When range is before start and ends after end", 2, 9, false, []*data.Interval{{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}}),
				Entry("When range is after start and ends after end", 4, 9, false, []*data.Interval{{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}}),
				Entry("When range is completely outside of data interval (after end)", 9, 9, false, []*data.Interval{}),
				Entry("When range is completely outside of covered interval (after end)", 10, 12, false, []*data.Interval{}),
				Entry("When range is invalid (start after end)", 5, 3, false, []*data.Interval{}),
				Entry("When range is covered but days only on weekend", 6, 7, true, []*data.Interval{{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}}),
				Entry("When range completely covers available data", 3, 8, true, []*data.Interval{{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}}),
				Entry("When range is a single date", 4, 4, true, []*data.Interval{{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}}),
			)

			DescribeTable("get various time ranges",
				func(a, b int, expected []float64, expectedError error) {
					begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
					end := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())
					vals := []float64{0, 1, 2, 3}
					err := cache.Set(security, data.MetricAdjustedClose, begin, end, vals)
					Expect(err).To(BeNil())

					// try to retrieve the data
					rangeA := time.Date(2022, 8, a, 0, 0, 0, 0, tz())
					rangeB := time.Date(2022, 8, b, 0, 0, 0, 0, tz())
					res, err := cache.Get(security, data.MetricAdjustedClose, rangeA, rangeB)
					if expectedError == nil {
						Expect(err).To(BeNil())
					} else {
						Expect(errors.Is(err, expectedError)).To(BeTrue())
					}

					Expect(res).To(Equal(expected))
				},
				Entry("When range is completely outside of data interval (before start)", 1, 1, nil, data.ErrRangeDoesNotExist),
				Entry("When range is before start and ends within interval", 2, 3, nil, data.ErrRangeDoesNotExist),
				Entry("When range is before start and ends at interval end", 2, 8, nil, data.ErrRangeDoesNotExist),
				Entry("When range is before start and ends after end", 2, 9, nil, data.ErrRangeDoesNotExist),
				Entry("When range is after start and ends after end", 4, 9, nil, data.ErrRangeDoesNotExist),
				Entry("When range is completely outside of data interval (after end)", 9, 9, nil, data.ErrRangeDoesNotExist),
				Entry("When range is completely outside of covered interval (after end)", 10, 12, nil, data.ErrRangeDoesNotExist),
				Entry("When range is invalid (start after end)", 5, 3, nil, data.ErrInvalidTimeRange),
				Entry("When range is covered but days only on weekend", 6, 7, []float64{}, nil),
				Entry("When range is a subset of covered period", 4, 5, []float64{1, 2}, nil),
				Entry("Range touches left extremity", 3, 5, []float64{0, 1, 2}, nil),
				Entry("Range touches right extremity", 5, 8, []float64{2, 3}, nil),
				Entry("Range covers full period", 3, 8, []float64{0, 1, 2, 3}, nil),
				Entry("Range covers a single day 0", 3, 3, []float64{0}, nil),
				Entry("Range covers a single day 1", 4, 4, []float64{1}, nil),
				Entry("Range covers a single day 2", 5, 5, []float64{2}, nil),
				Entry("Range covers a single day 3", 8, 8, []float64{3}, nil),
				Entry("Range begins on weekend", 6, 8, []float64{3}, nil),
				Entry("Range ends on saturday", 3, 6, []float64{0, 1, 2}, nil),
				Entry("Range ends on sunday", 3, 6, []float64{0, 1, 2}, nil),
				Entry("When range is a single date", 4, 4, []float64{1}, nil),
			)

			It("should successfully get values", func() {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
				vals := []float64{0, 1, 2, 3, 4}

				cache.Set(security, data.MetricAdjustedClose, begin, end, vals)

				result, err := cache.Get(security, data.MetricAdjustedClose, begin, end)
				Expect(err).To(BeNil())
				Expect(result).To(Equal(vals))
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

				err := cache.Set(security, data.MetricAdjustedClose, begin, end, vals)
				Expect(errors.Is(err, data.ErrDataLargerThanCache)).To(BeTrue(), "error should be data larger than cache")
			})

			It("should evict old data when cumulative sets exceed the configured memory", func() {
				cache = data.NewSecurityMetricCache(16, dates)
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 4, 0, 0, 0, 0, tz())
				vals := []float64{0, 1}

				err := cache.Set(security, data.MetricAdjustedClose, begin, end, vals)
				Expect(err).To(BeNil())

				begin2 := time.Date(2022, 8, 5, 0, 0, 0, 0, tz())
				end2 := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())
				vals2 := []float64{2, 3}
				err = cache.Set(security, data.MetricAdjustedClose, begin2, end2, vals2)
				Expect(err).To(BeNil())

				_, err = cache.Get(security, data.MetricAdjustedClose, begin, end)
				Expect(errors.Is(err, data.ErrRangeDoesNotExist)).To(BeTrue(), "error should be range does not exist")

				result, err := cache.Get(security, data.MetricAdjustedClose, begin2, end2)
				Expect(err).To(BeNil())
				Expect(result).To(Equal(vals2))
			})

		})
	})
})
