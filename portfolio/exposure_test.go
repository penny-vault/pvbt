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

var _ = Describe("Exposure", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105})
		Expect(portfolio.Exposure.ComputeSeries(a, nil)).To(BeNil())
	})

	It("returns 1.0 when all returns are non-zero", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105, 115})
		v := a.PerformanceMetric(portfolio.Exposure).Value()
		Expect(v).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("returns 0 for empty returns", func() {
		a := buildAccountFromEquity([]float64{100})
		v := a.PerformanceMetric(portfolio.Exposure).Value()
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for constant prices (all returns zero)", func() {
		a := buildAccountFromEquity([]float64{100, 100, 100, 100})
		v := a.PerformanceMetric(portfolio.Exposure).Value()
		Expect(v).To(Equal(0.0))
	})

	It("computes fraction correctly with mix of zero and non-zero returns", func() {
		a := buildAccountFromEquity([]float64{100, 110, 110, 120, 120})
		v := a.PerformanceMetric(portfolio.Exposure).Value()
		Expect(v).To(BeNumerically("~", 0.5, 1e-9))
	})

	It("counts negative returns as active", func() {
		a := buildAccountFromEquity([]float64{100, 90, 90, 80})
		v := a.PerformanceMetric(portfolio.Exposure).Value()
		Expect(v).To(BeNumerically("~", 2.0/3.0, 1e-9))
	})
})
