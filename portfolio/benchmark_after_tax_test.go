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
	"errors"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Benchmark after-tax return metrics", func() {
	Describe("BenchmarkAfterTaxTWRR", func() {
		It("applies LTCG tax to a benchmark gain", func() {
			// Benchmark prices: 100 -> 120 over a small window.
			// gain = 20, LTCG tax = 0.15 * 20 = 3, end value = 117.
			// after-tax cumulative return = 117/100 - 1 = 0.17.
			rfPrices := []float64{100, 100.01, 100.02, 100.03}
			eqCurve := []float64{1000, 1010, 1020, 1030}
			bmPrices := []float64{100, 110, 115, 120}

			acct := benchAcct(eqCurve, bmPrices, rfPrices)

			val, err := acct.PerformanceMetric(portfolio.BenchmarkAfterTaxTWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", 0.17, 1e-9))
		})

		It("does not tax a benchmark loss", func() {
			// gain = -20 -> no tax -> after-tax == pre-tax = -0.20.
			rfPrices := []float64{100, 100.01, 100.02, 100.03}
			eqCurve := []float64{1000, 990, 980, 970}
			bmPrices := []float64{100, 95, 88, 80}

			acct := benchAcct(eqCurve, bmPrices, rfPrices)

			val, err := acct.PerformanceMetric(portfolio.BenchmarkAfterTaxTWRR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", -0.20, 1e-9))
		})

		It("returns ErrNoBenchmark when no benchmark is configured", func() {
			acct := portfolio.New(portfolio.WithCash(1_000, time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)))
			_, err := acct.PerformanceMetric(portfolio.BenchmarkAfterTaxTWRR).Value()
			Expect(errors.Is(err, portfolio.ErrNoBenchmark)).To(BeTrue())
		})
	})

	Describe("BenchmarkAfterTaxCAGR", func() {
		It("annualizes the after-tax buy-and-hold gain", func() {
			// gain = 20 over 4 trading days; tax = 3; end value = 117.
			// years = (last - first) / 365.25; CAGR = (117/100)^(1/years) - 1.
			rfPrices := []float64{100, 100.01, 100.02, 100.03}
			eqCurve := []float64{1000, 1010, 1020, 1030}
			bmPrices := []float64{100, 110, 115, 120}

			acct := benchAcct(eqCurve, bmPrices, rfPrices)

			pd := acct.PerfData()
			times := pd.Times()
			years := times[len(times)-1].Sub(times[0]).Hours() / 24 / 365.25
			expected := math.Pow(117.0/100.0, 1.0/years) - 1

			val, err := acct.PerformanceMetric(portfolio.BenchmarkAfterTaxCAGR).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", expected, 1e-9))
		})
	})
})
