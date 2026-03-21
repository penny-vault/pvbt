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

var _ = Describe("ProbabilisticSharpe", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountWithRF(
			[]float64{100, 105, 98, 103, 97, 110},
			[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
		)
		s, err := portfolio.ProbabilisticSharpe.ComputeSeries(context.Background(), a, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeNil())
	})

	It("returns probability between 0 and 1 for mixed returns", func() {
		a := buildAccountWithRF(
			[]float64{100, 105, 98, 103, 97, 110},
			[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
		)

		v, err := a.PerformanceMetric(portfolio.ProbabilisticSharpe).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically(">", 0.0))
		Expect(v).To(BeNumerically("<=", 1.0))
	})

	It("returns above 0.5 for positive Sharpe with varied returns", func() {
		a := buildAccountWithRF(
			[]float64{100, 105, 103, 108, 106, 112, 109, 115, 113, 120,
				117, 124, 121, 128, 125, 133, 130, 138, 135, 143, 140},
			[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05, 100.06,
				100.07, 100.08, 100.09, 100.10, 100.11, 100.12, 100.13,
				100.14, 100.15, 100.16, 100.17, 100.18, 100.19, 100.20},
		)

		v, err := a.PerformanceMetric(portfolio.ProbabilisticSharpe).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically(">", 0.5))
		Expect(v).To(BeNumerically("<=", 1.0))
	})

	It("returns 0 for fewer than 4 excess returns", func() {
		a := buildAccountWithRF(
			[]float64{100, 105, 98, 103},
			[]float64{100, 100.01, 100.02, 100.03},
		)
		v, err := a.PerformanceMetric(portfolio.ProbabilisticSharpe).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for single data point", func() {
		a := buildAccountWithRF([]float64{100}, []float64{100})
		v, err := a.PerformanceMetric(portfolio.ProbabilisticSharpe).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for constant prices", func() {
		a := buildAccountWithRF(
			[]float64{100, 100, 100, 100, 100, 100},
			[]float64{100, 100, 100, 100, 100, 100},
		)
		v, err := a.PerformanceMetric(portfolio.ProbabilisticSharpe).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns below 0.5 for negative Sharpe", func() {
		a := buildAccountWithRF(
			[]float64{100, 95, 90, 85, 80, 75},
			[]float64{100, 101, 102, 103, 104, 105},
		)
		v, err := a.PerformanceMetric(portfolio.ProbabilisticSharpe).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("<", 0.5))
	})
})
