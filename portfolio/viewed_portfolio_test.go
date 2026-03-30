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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("View", func() {
	var (
		acct   *portfolio.Account
		dates  []time.Time
		equity []float64
	)

	BeforeEach(func() {
		dates = []time.Time{
			time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 2, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 3, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 4, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 8, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 11, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 12, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2021, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		equity = []float64{
			10_000, 10_200, 10_400, 10_600, 10_800,
			11_000, 11_200, 11_400, 11_600, 11_800,
			12_000, 12_200, 12_400,
		}

		portfolioAsset := asset.Asset{
			CompositeFigi: "_PORTFOLIO_",
			Ticker:        "_PORTFOLIO_",
		}

		perfDF, err := data.NewDataFrame(
			dates,
			[]asset.Asset{portfolioAsset},
			[]data.Metric{data.PortfolioEquity},
			data.Daily,
			[][]float64{equity},
		)
		Expect(err).NotTo(HaveOccurred())

		acct = portfolio.New(portfolio.WithCash(equity[0], dates[0]))
		acct.SetPerfData(perfDF)
	})

	It("returns a Portfolio whose metric matches AbsoluteWindow", func() {
		viewStart := dates[2] // 2020-03-02
		viewEnd := dates[8]   // 2020-09-01

		// Compute via AbsoluteWindow (existing API).
		expected, err := acct.PerformanceMetric(portfolio.CAGR).AbsoluteWindow(viewStart, viewEnd).Value()
		Expect(err).NotTo(HaveOccurred())

		// Compute via View (new API).
		viewed := acct.View(viewStart, viewEnd)
		actual, err := viewed.PerformanceMetric(portfolio.CAGR).Value()
		Expect(err).NotTo(HaveOccurred())

		Expect(actual).To(BeNumerically("~", expected, 1e-12))
	})

	It("satisfies the PortfolioStats interface", func() {
		viewed := acct.View(dates[0], dates[len(dates)-1])
		_, ok := viewed.(portfolio.PortfolioStats)
		Expect(ok).To(BeTrue(), "viewed portfolio should satisfy PortfolioStats")
	})

	It("passes through point-in-time methods from the account", func() {
		viewed := acct.View(dates[0], dates[len(dates)-1])
		Expect(viewed.Cash()).To(Equal(acct.Cash()))
		Expect(viewed.Benchmark()).To(Equal(acct.Benchmark()))
	})
})
