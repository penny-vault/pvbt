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
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("Stats", func() {
	Describe("SliceMean", func() {
		It("returns 0 for empty slice", func() {
			Expect(data.SliceMean([]float64{})).To(BeNumerically("==", 0))
		})

		It("returns 0 for nil slice", func() {
			Expect(data.SliceMean(nil)).To(BeNumerically("==", 0))
		})

		It("returns the single value for one-element slice", func() {
			Expect(data.SliceMean([]float64{7.5})).To(BeNumerically("~", 7.5, 1e-12))
		})

		It("computes arithmetic mean for multiple elements", func() {
			Expect(data.SliceMean([]float64{1, 2, 3, 4})).To(BeNumerically("~", 2.5, 1e-12))
		})
	})

	Describe("Variance", func() {
		It("returns 0 for empty input", func() {
			Expect(data.Variance([]float64{})).To(BeNumerically("==", 0))
		})

		It("returns 0 for single element", func() {
			Expect(data.Variance([]float64{42})).To(BeNumerically("==", 0))
		})

		It("computes correct sample variance", func() {
			Expect(data.Variance([]float64{1, 2, 3})).To(BeNumerically("~", 1.0, 1e-12))
		})

		It("returns 0 for identical values", func() {
			Expect(data.Variance([]float64{5, 5, 5, 5})).To(BeNumerically("==", 0))
		})
	})

	Describe("Stddev", func() {
		It("returns 0 for empty input", func() {
			Expect(data.Stddev([]float64{})).To(BeNumerically("==", 0))
		})

		It("is the square root of the variance", func() {
			Expect(data.Stddev([]float64{1, 2, 3})).To(BeNumerically("~", 1.0, 1e-12))
		})
	})

	Describe("Covariance", func() {
		It("returns 0 for empty inputs", func() {
			Expect(data.Covariance([]float64{}, []float64{})).To(BeNumerically("==", 0))
		})

		It("returns 0 for single-element inputs", func() {
			Expect(data.Covariance([]float64{5}, []float64{10})).To(BeNumerically("==", 0))
		})

		It("trims to the shorter array", func() {
			x := []float64{1, 2, 3}
			y := []float64{2, 4}
			Expect(data.Covariance(x, y)).To(BeNumerically("~", 1.0, 1e-12))
		})

		It("computes correct sample covariance for perfect linear relationship", func() {
			x := []float64{1, 2, 3, 4, 5}
			y := []float64{2, 4, 6, 8, 10}
			Expect(data.Covariance(x, y)).To(BeNumerically("~", 5.0, 1e-12))
		})
	})

	Describe("AnnualizationFactor", func() {
		It("returns 252 for daily timestamps", func() {
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, 10)
			for i := range times {
				times[i] = start.AddDate(0, 0, i)
			}
			Expect(data.AnnualizationFactor(times)).To(BeNumerically("==", 252))
		})

		It("returns 12 for monthly timestamps", func() {
			start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, 6)
			for i := range times {
				times[i] = start.AddDate(0, i, 0)
			}
			Expect(data.AnnualizationFactor(times)).To(BeNumerically("==", 12))
		})

		It("defaults to 252 for a single timestamp", func() {
			times := []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
			Expect(data.AnnualizationFactor(times)).To(BeNumerically("==", 252))
		})

		It("defaults to 252 for empty slice", func() {
			Expect(data.AnnualizationFactor(nil)).To(BeNumerically("==", 252))
		})

		It("returns 252 for gap of exactly 20 days (boundary)", func() {
			t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t1 := time.Date(2024, 1, 21, 0, 0, 0, 0, time.UTC)
			Expect(data.AnnualizationFactor([]time.Time{t0, t1})).To(BeNumerically("==", 252))
		})

		It("returns 12 for gap of 21 days", func() {
			t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t1 := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
			Expect(data.AnnualizationFactor([]time.Time{t0, t1})).To(BeNumerically("==", 12))
		})
	})

	Describe("PeriodsReturns", func() {
		It("returns empty slice for single element", func() {
			Expect(data.PeriodsReturns([]float64{100})).To(HaveLen(0))
		})

		It("returns empty slice for empty input", func() {
			Expect(data.PeriodsReturns([]float64{})).To(HaveLen(0))
		})

		It("computes period-over-period returns", func() {
			r := data.PeriodsReturns([]float64{100, 110, 99})
			Expect(r).To(HaveLen(2))
			Expect(r[0]).To(BeNumerically("~", 0.10, 1e-12))
			Expect(r[1]).To(BeNumerically("~", -0.1, 1e-12))
		})

		It("handles zero values safely", func() {
			r := data.PeriodsReturns([]float64{100, 0})
			Expect(r).To(HaveLen(1))
			Expect(r[0]).To(BeNumerically("~", -1.0, 1e-12))
		})

		It("returns +Inf for zero-to-positive", func() {
			r := data.PeriodsReturns([]float64{0, 100})
			Expect(r).To(HaveLen(1))
			Expect(math.IsInf(r[0], 1)).To(BeTrue())
		})
	})
})
