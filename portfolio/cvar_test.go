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

var _ = Describe("CVaR", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
		Expect(portfolio.CVaR.ComputeSeries(a, nil)).To(BeNil())
	})

	It("computes the average of the worst 5% of returns", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
		v := a.PerformanceMetric(portfolio.CVaR).Value()
		Expect(v).To(BeNumerically("~", -0.060869565217391, 1e-9))
	})

	It("averages multiple tail returns for 20+ returns", func() {
		equity := []float64{
			100, 102, 99, 103, 101, 105, 100, 106, 104, 108,
			103, 109, 107, 111, 106, 112, 110, 114, 109, 115, 113,
		}
		a := buildAccountFromEquity(equity)
		v := a.PerformanceMetric(portfolio.CVaR).Value()
		Expect(v).To(BeNumerically("~", -0.04762, 1e-4))
	})

	It("returns 0 for empty equity curve (single data point)", func() {
		a := buildAccountFromEquity([]float64{100})
		v := a.PerformanceMetric(portfolio.CVaR).Value()
		Expect(v).To(Equal(0.0))
	})

	It("returns the single return for a two-point curve", func() {
		a := buildAccountFromEquity([]float64{100, 90})
		v := a.PerformanceMetric(portfolio.CVaR).Value()
		Expect(v).To(BeNumerically("~", -0.1, 1e-9))
	})

	It("returns 0 for constant prices", func() {
		a := buildAccountFromEquity([]float64{100, 100, 100, 100, 100})
		v := a.PerformanceMetric(portfolio.CVaR).Value()
		Expect(v).To(Equal(0.0))
	})

	It("returns a negative value for all-negative returns", func() {
		a := buildAccountFromEquity([]float64{100, 95, 90, 85, 80})
		v := a.PerformanceMetric(portfolio.CVaR).Value()
		Expect(v).To(BeNumerically("<", 0.0))
	})
})
