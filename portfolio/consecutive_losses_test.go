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

var _ = Describe("ConsecutiveLosses", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountFromEquity([]float64{100, 90, 95})
		s, err := portfolio.ConsecutiveLosses.ComputeSeries(context.Background(), a, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeNil())
	})

	It("finds the longest negative return streak", func() {
		a := buildAccountFromEquity([]float64{100, 90, 80, 85, 75, 65, 70})
		v, err := a.PerformanceMetric(portfolio.ConsecutiveLosses).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(2.0))
	})

	It("returns 0 for empty returns", func() {
		a := buildAccountFromEquity([]float64{100})
		v, err := a.PerformanceMetric(portfolio.ConsecutiveLosses).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for constant prices", func() {
		a := buildAccountFromEquity([]float64{100, 100, 100, 100})
		v, err := a.PerformanceMetric(portfolio.ConsecutiveLosses).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 when all returns are positive", func() {
		a := buildAccountFromEquity([]float64{100, 110, 121, 133})
		v, err := a.PerformanceMetric(portfolio.ConsecutiveLosses).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns full length when all returns are negative", func() {
		a := buildAccountFromEquity([]float64{100, 95, 90, 85, 80})
		v, err := a.PerformanceMetric(portfolio.ConsecutiveLosses).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(4.0))
	})

	It("returns 1 for alternating returns", func() {
		a := buildAccountFromEquity([]float64{100, 90, 100, 90, 100})
		v, err := a.PerformanceMetric(portfolio.ConsecutiveLosses).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(1.0))
	})
})
