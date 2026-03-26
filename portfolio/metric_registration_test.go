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

var _ = Describe("MetricRegistration", func() {
	It("registers all metrics by default when none specified", func() {
		acct := portfolio.New()
		names := metricNames(acct.RegisteredMetrics())
		Expect(names).To(ContainElements(
			"TWRR", "MWRR", "Sharpe", "Sortino", "Calmar", "MaxDrawdown", "StdDev",
			"Beta", "Alpha", "CAGR", "WinRate", "ProfitFactor", "ValueAtRisk",
		))
	})

	It("registers an individual metric", func() {
		acct := portfolio.New(portfolio.WithMetric(portfolio.Sharpe))
		Expect(acct.RegisteredMetrics()).To(HaveLen(1))
		Expect(acct.RegisteredMetrics()[0].Name()).To(Equal("Sharpe"))
	})

	It("registers summary metrics group", func() {
		acct := portfolio.New(portfolio.WithSummaryMetrics())
		names := metricNames(acct.RegisteredMetrics())
		Expect(names).To(ContainElements("TWRR", "MWRR", "Sharpe", "Sortino", "Calmar", "MaxDrawdown", "StdDev"))
	})

	It("registers all metrics", func() {
		acct := portfolio.New(portfolio.WithAllMetrics())
		Expect(len(acct.RegisteredMetrics())).To(BeNumerically(">", 30))
	})

	It("deduplicates metrics", func() {
		acct := portfolio.New(
			portfolio.WithMetric(portfolio.Sharpe),
			portfolio.WithMetric(portfolio.Sharpe),
		)
		Expect(acct.RegisteredMetrics()).To(HaveLen(1))
	})
})

func metricNames(metrics []portfolio.PerformanceMetric) []string {
	names := make([]string, len(metrics))
	for i, m := range metrics {
		names[i] = m.Name()
	}
	return names
}
