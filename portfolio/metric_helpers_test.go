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

package portfolio

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metric Helpers", func() {
	Describe("returns", func() {
		It("computes period-over-period returns from a known price series", func() {
			prices := []float64{100, 110, 105, 120}
			r := returns(prices)
			Expect(r).To(HaveLen(3))
			Expect(r[0]).To(BeNumerically("~", 0.10, 1e-10))
			Expect(r[1]).To(BeNumerically("~", -5.0/110.0, 1e-10))
			Expect(r[2]).To(BeNumerically("~", 15.0/105.0, 1e-10))
		})

		It("returns empty slice for single-element input", func() {
			r := returns([]float64{100})
			Expect(r).To(BeEmpty())
		})
	})

	Describe("excessReturns", func() {
		It("subtracts risk-free returns element-wise", func() {
			r := []float64{0.10, 0.05, 0.08}
			rf := []float64{0.02, 0.02, 0.02}
			er := excessReturns(r, rf)
			Expect(er).To(HaveLen(3))
			Expect(er[0]).To(BeNumerically("~", 0.08, 1e-10))
			Expect(er[1]).To(BeNumerically("~", 0.03, 1e-10))
			Expect(er[2]).To(BeNumerically("~", 0.06, 1e-10))
		})
	})

	Describe("windowSlice", func() {
		It("trims to trailing 2 months", func() {
			times := []time.Time{
				time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 4, 15, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 5, 15, 0, 0, 0, 0, time.UTC),
			}
			series := []float64{1, 2, 3, 4, 5}
			w := Months(2)
			result := windowSlice(series, times, &w)
			// cutoff = 2025-05-15 minus 2 months = 2025-03-15
			// elements at index 2,3,4 have times >= cutoff
			Expect(result).To(Equal([]float64{3, 4, 5}))
		})

		It("returns full series when window is nil", func() {
			times := []time.Time{
				time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			}
			series := []float64{10, 20}
			result := windowSlice(series, times, nil)
			Expect(result).To(Equal([]float64{10, 20}))
		})
	})

	Describe("mean", func() {
		It("computes the arithmetic mean", func() {
			Expect(mean([]float64{2, 4, 6})).To(BeNumerically("~", 4.0, 1e-10))
		})
	})

	Describe("stddev", func() {
		It("computes the sample standard deviation", func() {
			// values: 2, 4, 4, 4, 5, 5, 7, 9; mean=5, var=32/7
			x := []float64{2, 4, 4, 4, 5, 5, 7, 9}
			sd := stddev(x)
			Expect(sd).To(BeNumerically("~", math.Sqrt(32.0/7.0), 1e-10))
		})
	})

	Describe("variance", func() {
		It("computes the sample variance", func() {
			x := []float64{2, 4, 4, 4, 5, 5, 7, 9}
			v := variance(x)
			Expect(v).To(BeNumerically("~", 32.0/7.0, 1e-10))
		})
	})

	Describe("covariance", func() {
		It("computes the sample covariance of perfectly correlated series", func() {
			x := []float64{1, 2, 3, 4, 5}
			y := []float64{2, 4, 6, 8, 10} // y = 2*x
			c := covariance(x, y)
			// cov(x, 2x) = 2 * var(x) = 2 * 2.5 = 5.0
			Expect(c).To(BeNumerically("~", 5.0, 1e-10))
		})
	})

	Describe("cagr", func() {
		It("computes CAGR for 100 -> 200 over 3 years", func() {
			result := cagr(100, 200, 3)
			expected := math.Pow(2.0, 1.0/3.0) - 1
			Expect(result).To(BeNumerically("~", expected, 1e-10))
		})
	})

	Describe("drawdownSeries", func() {
		It("computes drawdowns from a known equity curve", func() {
			equity := []float64{100, 110, 105, 120, 90}
			dd := drawdownSeries(equity)
			Expect(dd).To(HaveLen(5))
			Expect(dd[0]).To(BeNumerically("~", 0.0, 1e-10))      // peak=100
			Expect(dd[1]).To(BeNumerically("~", 0.0, 1e-10))      // peak=110
			Expect(dd[2]).To(BeNumerically("~", -5.0/110.0, 1e-10))  // (105-110)/110
			Expect(dd[3]).To(BeNumerically("~", 0.0, 1e-10))      // peak=120
			Expect(dd[4]).To(BeNumerically("~", -30.0/120.0, 1e-10)) // (90-120)/120
		})
	})
})
