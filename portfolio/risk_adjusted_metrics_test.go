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
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Risk-Adjusted Metrics", func() {
	var (
		acct *portfolio.Account
		spy  asset.Asset
		bil  asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}

		acct = portfolio.New(
			portfolio.WithCash(10_000),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bil),
		)

		// Simulate buying 20 shares of SPY at 400 on day 0.
		// This costs 8000, leaving 2000 cash.
		// Portfolio value will then move with SPY price.
		base := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

		// SPY prices: rise, dip, then recover above starting level.
		spyPrices := []float64{
			400, 404, 408, 412, 416,
			412, 400, 380, 360, 368,
			384, 400, 416, 432, 440,
			448, 456, 464, 472, 480,
		}
		// BIL: nearly flat risk-free proxy.
		bilPrices := []float64{
			50.00, 50.005, 50.01, 50.015, 50.02,
			50.025, 50.03, 50.035, 50.04, 50.045,
			50.05, 50.055, 50.06, 50.065, 50.07,
			50.075, 50.08, 50.085, 50.09, 50.095,
		}

		// Record buy on day 0 before first UpdatePrices.
		acct.Record(portfolio.Transaction{
			Date:   base,
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    20,
			Price:  400,
			Amount: -8_000,
		})

		for i := 0; i < 20; i++ {
			t := base.AddDate(0, 0, i)
			df := buildDF(t,
				[]asset.Asset{spy, bil},
				[]float64{spyPrices[i], bilPrices[i]},
				[]float64{spyPrices[i], bilPrices[i]},
			)
			acct.UpdatePrices(df)
		}
	})

	Describe("StdDev", func() {
		It("returns a positive value for a volatile equity curve", func() {
			val := acct.PerformanceMetric(portfolio.StdDev).Value()
			Expect(val).To(BeNumerically(">", 0))
		})

		It("returns a return series from ComputeSeries", func() {
			series := acct.PerformanceMetric(portfolio.StdDev).Series()
			Expect(series).NotTo(BeEmpty())
		})
	})

	Describe("MaxDrawdown", func() {
		It("returns a negative value for an equity curve with a dip", func() {
			val := acct.PerformanceMetric(portfolio.MaxDrawdown).Value()
			Expect(val).To(BeNumerically("<", 0))
		})

		It("returns a drawdown series from ComputeSeries", func() {
			series := acct.PerformanceMetric(portfolio.MaxDrawdown).Series()
			Expect(series).NotTo(BeEmpty())
		})
	})

	Describe("DownsideDeviation", func() {
		It("returns a non-negative value", func() {
			val := acct.PerformanceMetric(portfolio.DownsideDeviation).Value()
			Expect(val).To(BeNumerically(">=", 0))
		})
	})

	Describe("Sharpe", func() {
		It("returns a positive value for a rising equity curve", func() {
			val := acct.PerformanceMetric(portfolio.Sharpe).Value()
			Expect(val).To(BeNumerically(">", 0))
		})
	})

	Describe("Sortino", func() {
		It("returns a positive value for a rising equity curve", func() {
			val := acct.PerformanceMetric(portfolio.Sortino).Value()
			Expect(val).To(BeNumerically(">", 0))
		})
	})

	Describe("Calmar", func() {
		It("returns a positive value for a rising equity curve with drawdown", func() {
			val := acct.PerformanceMetric(portfolio.Calmar).Value()
			Expect(val).To(BeNumerically(">", 0))
		})
	})
})
