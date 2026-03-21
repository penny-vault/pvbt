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

var _ = Describe("RecoveryFactor", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountFromEquity([]float64{100, 90, 120})
		s, err := portfolio.RecoveryFactor.ComputeSeries(context.Background(), a, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeNil())
	})

	It("computes total return / abs(max drawdown)", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
		v, err := a.PerformanceMetric(portfolio.RecoveryFactor).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("~", 4.107, 1e-2))
	})

	It("returns 0 for single data point", func() {
		a := buildAccountFromEquity([]float64{100})
		v, err := a.PerformanceMetric(portfolio.RecoveryFactor).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for constant prices (no drawdown)", func() {
		a := buildAccountFromEquity([]float64{100, 100, 100, 100})
		v, err := a.PerformanceMetric(portfolio.RecoveryFactor).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns 0 for monotonically rising equity (no drawdown)", func() {
		a := buildAccountFromEquity([]float64{100, 110, 121, 133})
		v, err := a.PerformanceMetric(portfolio.RecoveryFactor).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns negative value for net loss with drawdown", func() {
		a := buildAccountFromEquity([]float64{100, 120, 80, 90})
		v, err := a.PerformanceMetric(portfolio.RecoveryFactor).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("~", -0.3, 1e-1))
	})
})
