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

var _ = Describe("AvgDrawdownDays", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountFromEquity([]float64{100, 90, 100})
		s, err := portfolio.AvgDrawdownDays.ComputeSeries(context.Background(), a, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeNil())
	})

	It("computes mean duration of drawdown episodes", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
		v, err := a.PerformanceMetric(portfolio.AvgDrawdownDays).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("handles multi-day drawdown episodes", func() {
		a := buildAccountFromEquity([]float64{100, 120, 110, 105, 125, 115, 130})
		v, err := a.PerformanceMetric(portfolio.AvgDrawdownDays).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("~", 1.5, 1e-9))
	})

	It("returns 0 for single data point", func() {
		a := buildAccountFromEquity([]float64{100})
		v, err := a.PerformanceMetric(portfolio.AvgDrawdownDays).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for constant prices", func() {
		a := buildAccountFromEquity([]float64{100, 100, 100, 100})
		v, err := a.PerformanceMetric(portfolio.AvgDrawdownDays).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for monotonically rising equity", func() {
		a := buildAccountFromEquity([]float64{100, 110, 121, 133})
		v, err := a.PerformanceMetric(portfolio.AvgDrawdownDays).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("counts trailing drawdown that does not recover", func() {
		a := buildAccountFromEquity([]float64{100, 120, 100, 90})
		v, err := a.PerformanceMetric(portfolio.AvgDrawdownDays).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("~", 2.0, 1e-9))
	})

	It("handles continuous decline (single long episode)", func() {
		a := buildAccountFromEquity([]float64{100, 95, 90, 85, 80})
		v, err := a.PerformanceMetric(portfolio.AvgDrawdownDays).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("~", 4.0, 1e-9))
	})
})
