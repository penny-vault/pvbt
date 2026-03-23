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

package portfolio_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("AbsoluteWindow", func() {
	// buildAccountFromEquity starts at 2024-01-02 (Tuesday) and adds weekdays.
	// 7 equity values = 7 trading days.
	// dates: 2024-01-02, 01-03, 01-04, 01-05, 01-08, 01-09, 01-10
	var acct *portfolio.Account

	BeforeEach(func() {
		acct = buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
	})

	Describe("AbsoluteWindow restricts metric computation", func() {
		It("computes CAGR over the full range without AbsoluteWindow", func() {
			fullCAGR, err := acct.PerformanceMetric(portfolio.CAGR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(math.IsNaN(fullCAGR)).To(BeFalse())
			Expect(fullCAGR).To(BeNumerically(">", 0.0))
		})

		It("computes a different CAGR when restricted to a sub-window", func() {
			fullCAGR, err := acct.PerformanceMetric(portfolio.CAGR).Value()
			Expect(err).NotTo(HaveOccurred())

			// Use only the first 3 trading days: 2024-01-02 through 2024-01-04
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)

			windowedCAGR, err := acct.PerformanceMetric(portfolio.CAGR).
				AbsoluteWindow(start, end).
				Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(math.IsNaN(windowedCAGR)).To(BeFalse())
			Expect(windowedCAGR).NotTo(BeNumerically("~", fullCAGR, 1e-9))
		})

		It("returns 0 when the window contains only one data point", func() {
			// A single day produces one data point: insufficient for CAGR.
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

			val, err := acct.PerformanceMetric(portfolio.CAGR).
				AbsoluteWindow(start, end).
				Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal(0.0))
		})

		It("returns 0 when the window contains no data", func() {
			// A window entirely before the data range.
			start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)

			val, err := acct.PerformanceMetric(portfolio.CAGR).
				AbsoluteWindow(start, end).
				Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal(0.0))
		})

		It("computes the correct CAGR for a known sub-range", func() {
			// Sub-range: 2024-01-02 to 2024-01-05 (4 points: 100, 110, 105, 115)
			// years = (Jan 5 - Jan 2) / 365.25 = 3/365.25
			// expected = (115/100)^(365.25/3) - 1
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)

			windowedCAGR, err := acct.PerformanceMetric(portfolio.CAGR).
				AbsoluteWindow(start, end).
				Value()
			Expect(err).NotTo(HaveOccurred())

			years := 3.0 / 365.25
			expected := math.Pow(115.0/100.0, 1.0/years) - 1
			Expect(windowedCAGR).To(BeNumerically("~", expected, 1e-6))
		})
	})

	Describe("AbsoluteWindow composability", func() {
		It("can be chained with Benchmark() -- benchmark not set so returns error", func() {
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)

			// buildAccountFromEquity has no benchmark configured.
			_, err := acct.PerformanceMetric(portfolio.TWRR).
				Benchmark().
				AbsoluteWindow(start, end).
				Value()
			Expect(err).To(MatchError(portfolio.ErrNoBenchmark))
		})

		It("AbsoluteWindow and Window can coexist: AbsoluteWindow takes precedence for slicing", func() {
			// When both are set the absolute window filters first via
			// windowedStats.Between; the relative Window *Period is still
			// passed to underlying Account methods for their own windowing.
			// In practice the windowedStats restricts the data, so the
			// effective range is determined by AbsoluteWindow.
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			end := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)

			windowedCAGR, err := acct.PerformanceMetric(portfolio.CAGR).
				AbsoluteWindow(start, end).
				Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(math.IsNaN(windowedCAGR)).To(BeFalse())
		})
	})
})
