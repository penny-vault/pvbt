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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("KellerRatio", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105, 115})
		s, err := portfolio.KellerRatio.ComputeSeries(a, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeNil())
	})

	It("computes K = R * (1 - D/(1-D)) for positive return with moderate drawdown", func() {
		// equity: 100 -> 110 -> 95 -> 120
		// total return R = 0.20
		// peak sequence: 100, 110, 110, 120
		// drawdown at point 2: (95-110)/110 = -0.13636...
		// max drawdown D = 0.13636...
		// K = 0.20 * (1 - 0.13636/(1-0.13636))
		//   = 0.20 * (1 - 0.13636/0.86364)
		//   = 0.20 * (1 - 0.15789)
		//   = 0.20 * 0.84211
		//   = 0.16842
		a := buildAccountFromEquity([]float64{100, 110, 95, 120})
		v, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("~", 0.16842, 1e-3))
	})

	It("returns 0 for negative total return", func() {
		a := buildAccountFromEquity([]float64{100, 110, 90, 85})
		v, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 when max drawdown exceeds 50%", func() {
		a := buildAccountFromEquity([]float64{100, 120, 40, 110})
		v, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns full total return when there is no drawdown", func() {
		// monotonically rising: no drawdown, D=0
		// K = R * (1 - 0/(1-0)) = R * 1 = R
		a := buildAccountFromEquity([]float64{100, 110, 121, 133})
		v, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("~", 0.33, 1e-2))
	})

	It("returns 0 for single data point", func() {
		a := buildAccountFromEquity([]float64{100})
		v, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for constant prices", func() {
		a := buildAccountFromEquity([]float64{100, 100, 100, 100})
		v, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("penalizes larger drawdowns more heavily", func() {
		// Both have same total return (25%) but different drawdowns
		smallDD := buildAccountFromEquity([]float64{100, 110, 105, 125})
		largeDD := buildAccountFromEquity([]float64{100, 130, 90, 125})

		smallV, err := smallDD.PerformanceMetric(portfolio.KellerRatio).Value()
		Expect(err).NotTo(HaveOccurred())

		largeV, err := largeDD.PerformanceMetric(portfolio.KellerRatio).Value()
		Expect(err).NotTo(HaveOccurred())

		Expect(smallV).To(BeNumerically(">", largeV))
	})
})
