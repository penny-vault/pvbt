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
	"github.com/rs/zerolog/log"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
)

func tz() *time.Location {
	return common.GetTimezone()
}

type segment struct {
	start int
	end   int
	vals  []float64
}

var _ = Describe("Cache", func() {
	Describe("when the cache is initialized ", func() {
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
					err := cache.Set(security, data.MetricAdjustedClose, begin, end, vals)
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
							Expect(len(res.Vals)).To(Equal(0), "number of columns in df should be zero")
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
				Entry("range ends on sunday", 3, 6, []float64{0, 1, 2}, nil),
				Entry("when range is a single date", 4, 4, []float64{1}, nil),
			)

			DescribeTable("test merging of values",
				func(segments []segment, expectedItemCount, a, b int, expectedVals []float64) {
					// set each segment on the cache, this should cause merging
					for _, seg := range segments {
						log.Debug().Int("Start", seg.start).Int("End", seg.end).Floats64("Floats", seg.vals).Msg("adding segment")
						begin := time.Date(2022, 8, seg.start, 0, 0, 0, 0, tz())
						end := time.Date(2022, 8, seg.end, 0, 0, 0, 0, tz())
						err := cache.Set(security, data.MetricAdjustedClose, begin, end, seg.vals)
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
				Entry("when segments are non contiguous", []segment{{1, 2, []float64{1, 2}}, {4, 5, []float64{4, 5}}}, 2, 4, 5, []float64{4, 5}),
				Entry("when segments are left contiguous", []segment{{3, 4, []float64{3, 4}}, {1, 2, []float64{1, 2}}}, 1, 1, 4, []float64{1, 2, 3, 4}),
				Entry("when segments are right contiguous", []segment{{1, 2, []float64{1, 2}}, {3, 4, []float64{3, 4}}}, 1, 1, 4, []float64{1, 2, 3, 4}),
				Entry("when segments are left contiguous (single)", []segment{{2, 2, []float64{2}}, {1, 1, []float64{1}}}, 1, 1, 2, []float64{1, 2}),
				Entry("when segments are right contiguous (single)", []segment{{1, 1, []float64{1}}, {2, 2, []float64{2}}}, 1, 1, 2, []float64{1, 2}),
				Entry("when segments are equal", []segment{{4, 5, []float64{4, 5}}, {4, 5, []float64{4, 5}}}, 1, 4, 5, []float64{4, 5}),
				Entry("when segment A is a subset of segment B", []segment{{3, 4, []float64{3, 4}}, {1, 5, []float64{1, 2, 3, 4, 5}}}, 1, 1, 5, []float64{1, 2, 3, 4, 5}),
				Entry("when segment B is a subset of segment A", []segment{{1, 5, []float64{1, 2, 3, 4, 5}}, {3, 4, []float64{3, 4}}}, 1, 1, 5, []float64{1, 2, 3, 4, 5}),
				Entry("when segments are left contiguous (weekend)", []segment{{8, 9, []float64{8, 9}}, {4, 5, []float64{4, 5}}}, 1, 4, 9, []float64{4, 5, 8, 9}),
				Entry("when segments are right contiguous (weekend)", []segment{{4, 5, []float64{4, 5}}, {8, 9, []float64{8, 9}}}, 1, 4, 9, []float64{4, 5, 8, 9}),
				Entry("when segments are right contiguous with overlap", []segment{{1, 2, []float64{1, 2}}, {2, 3, []float64{2, 3}}}, 1, 1, 3, []float64{1, 2, 3}),
				Entry("when segments are left contiguous with overlap", []segment{{2, 3, []float64{2, 3}}, {1, 2, []float64{1, 2}}}, 1, 1, 3, []float64{1, 2, 3}),
			)

			It("should collapse multiple overlapping cache items", func() {
				// add two single item entries into the cache that are separated by a day
				err := cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), []float64{1})
				Expect(err).To(BeNil(), "error during first cache set")
				err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), []float64{3})
				Expect(err).To(BeNil(), "error during second cache set")

				// make sure there are 2 entries in the []*CacheItem list
				Expect(cache.ItemCount(security, data.MetricAdjustedClose)).To(Equal(2), "item count after 2 sets")

				// now add a *CacheItem that connects the previous two entries
				err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 2, 0, 0, 0, 0, tz()), time.Date(2022, 8, 2, 0, 0, 0, 0, tz()), []float64{2})
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
				err := cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), []float64{1})
				Expect(err).To(BeNil(), "error during first cache set")
				err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), []float64{3})
				Expect(err).To(BeNil(), "error during second cache set")

				// make sure there are 2 entries in the []*CacheItem list
				Expect(cache.ItemCount(security, data.MetricAdjustedClose)).To(Equal(2), "item count after 2 sets")

				// now add a *CacheItem that connects the previous two entries
				err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 4, 0, 0, 0, 0, tz()), time.Date(2022, 8, 4, 0, 0, 0, 0, tz()), []float64{4})
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

				cache.Set(security, data.MetricAdjustedClose, begin, end, vals)

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

				err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end, localDates, vals)
				Expect(err).To(BeNil())

				Expect(cache.Count()).To(Equal(1), "number of metrics stored")
				Expect(cache.Size()).To(BeNumerically("==", 16), "cache size after set")

				items := cache.Items(security, data.MetricAdjustedClose)
				Expect(items[0].IsLocalDateIndex()).To(BeTrue(), "check if using a local date index")
			})

			It("should refuse to set values with unequal length and date and val arrays", func() {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
				localDates := []time.Time{time.Date(2022, 8, 4, 0, 0, 0, 0, tz()), time.Date(2022, 8, 6, 0, 0, 0, 0, tz())}
				vals := []float64{4}

				err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end, localDates, vals)
				Expect(errors.Is(err, data.ErrDateLengthDoesNotMatch)).To(BeTrue(), "check if set caused an error")
			})

			DescribeTable("check various time ranges",
				func(a, b int, expectedPresent bool, expectedInterval []*data.Interval) {
					begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
					end := time.Date(2022, 8, 8, 0, 0, 0, 0, tz())
					dt := []time.Time{time.Date(2022, 8, 4, 0, 0, 0, 0, tz())}
					vals := []float64{5.2}
					err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end, dt, vals)
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
					err := cache.SetWithLocalDates(security, data.MetricAdjustedClose, begin, end, dt, vals)
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

			PDescribeTable("test merging of values",
				func(segments []segment, expectedItemCount, a, b int, expectedVals []float64) {
					// set each segment on the cache, this should cause merging
					for _, seg := range segments {
						log.Debug().Int("Start", seg.start).Int("End", seg.end).Floats64("Floats", seg.vals).Msg("adding segment")
						begin := time.Date(2022, 8, seg.start, 0, 0, 0, 0, tz())
						end := time.Date(2022, 8, seg.end, 0, 0, 0, 0, tz())
						err := cache.Set(security, data.MetricAdjustedClose, begin, end, seg.vals)
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
				Entry("when segments are non contiguous", []segment{{1, 2, []float64{1, 2}}, {4, 5, []float64{4, 5}}}, 2, 4, 5, []float64{4, 5}),
				Entry("when segments are left contiguous", []segment{{3, 4, []float64{3, 4}}, {1, 2, []float64{1, 2}}}, 1, 1, 4, []float64{1, 2, 3, 4}),
				Entry("when segments are right contiguous", []segment{{1, 2, []float64{1, 2}}, {3, 4, []float64{3, 4}}}, 1, 1, 4, []float64{1, 2, 3, 4}),
				Entry("when segments are left contiguous (single)", []segment{{2, 2, []float64{2}}, {1, 1, []float64{1}}}, 1, 1, 2, []float64{1, 2}),
				Entry("when segments are right contiguous (single)", []segment{{1, 1, []float64{1}}, {2, 2, []float64{2}}}, 1, 1, 2, []float64{1, 2}),
				Entry("when segments are equal", []segment{{4, 5, []float64{4, 5}}, {4, 5, []float64{4, 5}}}, 1, 4, 5, []float64{4, 5}),
				Entry("when segment A is a subset of segment B", []segment{{3, 4, []float64{3, 4}}, {1, 5, []float64{1, 2, 3, 4, 5}}}, 1, 1, 5, []float64{1, 2, 3, 4, 5}),
				Entry("when segment B is a subset of segment A", []segment{{1, 5, []float64{1, 2, 3, 4, 5}}, {3, 4, []float64{3, 4}}}, 1, 1, 5, []float64{1, 2, 3, 4, 5}),
				Entry("when segments are left contiguous (weekend)", []segment{{8, 9, []float64{8, 9}}, {4, 5, []float64{4, 5}}}, 1, 4, 9, []float64{4, 5, 8, 9}),
				Entry("when segments are right contiguous (weekend)", []segment{{4, 5, []float64{4, 5}}, {8, 9, []float64{8, 9}}}, 1, 4, 9, []float64{4, 5, 8, 9}),
				Entry("when segments are right contiguous with overlap", []segment{{1, 2, []float64{1, 2}}, {2, 3, []float64{2, 3}}}, 1, 1, 3, []float64{1, 2, 3}),
				Entry("when segments are left contiguous with overlap", []segment{{2, 3, []float64{2, 3}}, {1, 2, []float64{1, 2}}}, 1, 1, 3, []float64{1, 2, 3}),
			)

			PIt("should collapse multiple overlapping cache items", func() {
				// add two single item entries into the cache that are separated by a day
				err := cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), []float64{1})
				Expect(err).To(BeNil(), "error during first cache set")
				err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), []float64{3})
				Expect(err).To(BeNil(), "error during second cache set")

				// make sure there are 2 entries in the []*CacheItem list
				Expect(cache.ItemCount(security, data.MetricAdjustedClose)).To(Equal(2), "item count after 2 sets")

				// now add a *CacheItem that connects the previous two entries
				err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 2, 0, 0, 0, 0, tz()), time.Date(2022, 8, 2, 0, 0, 0, 0, tz()), []float64{2})
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

			PIt("should not collapse if items don't overlap", func() {
				// add two single item entries into the cache that are separated by a day
				err := cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), time.Date(2022, 8, 1, 0, 0, 0, 0, tz()), []float64{1})
				Expect(err).To(BeNil(), "error during first cache set")
				err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), time.Date(2022, 8, 3, 0, 0, 0, 0, tz()), []float64{3})
				Expect(err).To(BeNil(), "error during second cache set")

				// make sure there are 2 entries in the []*CacheItem list
				Expect(cache.ItemCount(security, data.MetricAdjustedClose)).To(Equal(2), "item count after 2 sets")

				// now add a *CacheItem that connects the previous two entries
				err = cache.Set(security, data.MetricAdjustedClose, time.Date(2022, 8, 4, 0, 0, 0, 0, tz()), time.Date(2022, 8, 4, 0, 0, 0, 0, tz()), []float64{4})
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

			PIt("should successfully get values", func() {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
				vals := []float64{0, 1, 2, 3, 4}

				cache.Set(security, data.MetricAdjustedClose, begin, end, vals)

				result, err := cache.Get(security, data.MetricAdjustedClose, begin, end)
				Expect(err).To(BeNil())
				Expect(result.Vals[0]).To(Equal(vals))
			})

			PIt("should error for metric that doesn't exist", func() {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
				_, err := cache.Get(security, data.MetricClose, begin, end)
				Expect(errors.Is(err, data.ErrRangeDoesNotExist)).To(BeTrue(), "error should be metric does not exist")
			})

			PIt("should error for security that doesn't exist", func() {
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
				security2 := &data.Security{
					CompositeFigi: "second",
					Ticker:        "second",
				}
				_, err := cache.Get(security2, data.MetricClose, begin, end)
				Expect(errors.Is(err, data.ErrRangeDoesNotExist)).To(BeTrue(), "error should be security does not exist")
			})

			PIt("should error when Set size is greater than total size of cache", func() {
				cache = data.NewSecurityMetricCache(2, dates)
				begin := time.Date(2022, 8, 3, 0, 0, 0, 0, tz())
				end := time.Date(2022, 8, 9, 0, 0, 0, 0, tz())
				vals := []float64{0, 1, 2, 3, 4}

				err := cache.Set(security, data.MetricAdjustedClose, begin, end, vals)
				Expect(errors.Is(err, data.ErrDataLargerThanCache)).To(BeTrue(), "error should be data larger than cache")
			})

			PIt("should evict old data when cumulative sets exceed the configured memory", func() {
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
				Expect(result.Vals[0]).To(Equal(vals2))
			})

		})

	})
})
