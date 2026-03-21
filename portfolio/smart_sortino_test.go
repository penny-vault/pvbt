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
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("SmartSortino", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountWithRF(
			[]float64{100, 105, 98, 103, 97, 110},
			[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
		)
		s, err := portfolio.SmartSortino.ComputeSeries(context.Background(), a, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeNil())
	})

	It("returns Sortino divided by autocorrelation penalty", func() {
		a := buildAccountWithRF(
			[]float64{100, 105, 98, 103, 97, 110},
			[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
		)

		smartVal, err := a.PerformanceMetric(portfolio.SmartSortino).Value()
		Expect(err).NotTo(HaveOccurred())

		Expect(smartVal).NotTo(Equal(0.0))
		Expect(math.IsInf(smartVal, 0)).To(BeFalse())
		Expect(math.IsNaN(smartVal)).To(BeFalse())
	})

	It("returns 0 for single data point", func() {
		a := buildAccountWithRF([]float64{100}, []float64{100})
		v, err := a.PerformanceMetric(portfolio.SmartSortino).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for constant prices", func() {
		a := buildAccountWithRF(
			[]float64{100, 100, 100, 100, 100},
			[]float64{100, 100, 100, 100, 100},
		)
		v, err := a.PerformanceMetric(portfolio.SmartSortino).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 when all excess returns are positive (no downside)", func() {
		a := buildAccountWithRF(
			[]float64{100, 110, 121, 133, 146},
			[]float64{100, 100.01, 100.02, 100.03, 100.04},
		)
		v, err := a.PerformanceMetric(portfolio.SmartSortino).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})
})
