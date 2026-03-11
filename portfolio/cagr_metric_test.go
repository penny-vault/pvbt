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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("CAGR", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountFromEquity([]float64{100, 110, 120})
		Expect(portfolio.CAGR.ComputeSeries(a, nil)).To(BeNil())
	})

	It("computes annualized compound growth rate", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
		v := a.PerformanceMetric(portfolio.CAGR).Value()

		years := 8.0 / 365.25
		expected := math.Pow(125.0/100.0, 1.0/years) - 1
		Expect(v).To(BeNumerically("~", expected, 1e-6))
	})

	It("returns 0 for single data point", func() {
		a := buildAccountFromEquity([]float64{100})
		v := a.PerformanceMetric(portfolio.CAGR).Value()
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for constant prices (start == end)", func() {
		a := buildAccountFromEquity([]float64{100, 100, 100, 100})
		v := a.PerformanceMetric(portfolio.CAGR).Value()
		Expect(v).To(Equal(0.0))
	})

	It("returns negative value for declining equity", func() {
		a := buildAccountFromEquity([]float64{100, 95, 90, 85})
		v := a.PerformanceMetric(portfolio.CAGR).Value()
		Expect(v).To(BeNumerically("<", 0.0))
	})

	It("returns positive value for growing equity", func() {
		a := buildAccountFromEquity([]float64{100, 110, 121, 133})
		v := a.PerformanceMetric(portfolio.CAGR).Value()
		Expect(v).To(BeNumerically(">", 0.0))
	})
})
