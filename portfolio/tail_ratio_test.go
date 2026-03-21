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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("TailRatio", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105})
		s, err := portfolio.TailRatio.ComputeSeries(context.Background(), a, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeNil())
	})

	It("computes 95th percentile / abs(5th percentile) ratio", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
		v, err := a.PerformanceMetric(portfolio.TailRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("~", 1.8254, 1e-3))
	})

	It("returns 0 for empty returns", func() {
		a := buildAccountFromEquity([]float64{100})
		v, err := a.PerformanceMetric(portfolio.TailRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for constant prices (p5 = 0)", func() {
		a := buildAccountFromEquity([]float64{100, 100, 100, 100})
		v, err := a.PerformanceMetric(portfolio.TailRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("handles all positive returns", func() {
		a := buildAccountFromEquity([]float64{100, 110, 121, 133, 146})
		v, err := a.PerformanceMetric(portfolio.TailRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically(">", 0.0))
	})

	It("handles single return", func() {
		a := buildAccountFromEquity([]float64{100, 90})
		v, err := a.PerformanceMetric(portfolio.TailRatio).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("~", -1.0, 1e-9))
	})
})
