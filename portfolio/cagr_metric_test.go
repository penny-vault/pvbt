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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("CAGR", func() {
	It("returns nil from ComputeSeries", func() {
		a := buildAccountFromEquity([]float64{100, 110, 120})
		s, err := portfolio.CAGR.ComputeSeries(context.Background(), a, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(s).To(BeNil())
	})

	It("computes annualized compound growth rate", func() {
		a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
		v, err := a.PerformanceMetric(portfolio.CAGR).Value()
		Expect(err).NotTo(HaveOccurred())

		years := 8.0 / 365.25
		expected := math.Pow(125.0/100.0, 1.0/years) - 1
		Expect(v).To(BeNumerically("~", expected, 1e-6))
	})

	It("returns ErrInsufficientData for single data point", func() {
		a := buildAccountFromEquity([]float64{100})
		_, err := a.PerformanceMetric(portfolio.CAGR).Value()
		Expect(err).To(MatchError(portfolio.ErrInsufficientData))
	})

	It("returns 0 for constant prices (start == end)", func() {
		a := buildAccountFromEquity([]float64{100, 100, 100, 100})
		v, err := a.PerformanceMetric(portfolio.CAGR).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(0.0))
	})

	It("returns negative value for declining equity", func() {
		a := buildAccountFromEquity([]float64{100, 95, 90, 85})
		v, err := a.PerformanceMetric(portfolio.CAGR).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically("<", 0.0))
	})

	It("returns positive value for growing equity", func() {
		a := buildAccountFromEquity([]float64{100, 110, 121, 133})
		v, err := a.PerformanceMetric(portfolio.CAGR).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(BeNumerically(">", 0.0))
	})

	It("does not treat deposits as growth", func() {
		// Same scenario the TWRR test uses to verify flow adjustment:
		//   day 0: starting cash 10000.
		//   day 1: organic growth 1000 -> equity 11000.
		//   day 2: deposit 5000 + organic growth 1000 -> equity 17000.
		//
		// True annualized growth rate matches TWRR annualized:
		//   TWRR = (11000/10000) * (12000/11000) - 1 = 0.20 over 2 days
		//   years = 2/365.25
		//   CAGR = (1.20)^(1/years) - 1
		//
		// A naive CAGR (end/start without flow adjustment) would compute
		// 17000/10000 ratio and treat the 5000 deposit as growth, producing
		// a wildly inflated rate.
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)

		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
		acct.UpdatePrices(buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100}))

		acct.Record(portfolio.Transaction{
			Date: dates[1], Type: asset.DividendTransaction, Amount: 1000,
		})
		acct.UpdatePrices(buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100}))

		acct.Record(portfolio.Transaction{
			Date: dates[2], Type: asset.DepositTransaction, Amount: 5000,
		})
		acct.Record(portfolio.Transaction{
			Date: dates[2], Type: asset.DividendTransaction, Amount: 1000,
		})
		acct.UpdatePrices(buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100}))

		years := dates[len(dates)-1].Sub(dates[0]).Hours() / 24 / 365.25
		expected := math.Pow(1.20, 1.0/years) - 1

		val, err := acct.PerformanceMetric(portfolio.CAGR).Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(val).To(BeNumerically("~", expected, 1e-6))
	})
})
