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

package engine_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// metricsBuildDF mirrors the portfolio package's buildDF helper without
// importing test-only symbols across packages.
func metricsBuildDF(t time.Time, ast asset.Asset, close, adjClose float64) *data.DataFrame {
	df, err := data.NewDataFrame(
		[]time.Time{t},
		[]asset.Asset{ast},
		[]data.Metric{data.MetricClose, data.AdjClose},
		data.Daily,
		[][]float64{{close}, {adjClose}},
	)
	Expect(err).NotTo(HaveOccurred())
	return df
}

var _ = Describe("computeMetrics", func() {
	var (
		spy   asset.Asset
		bench asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bench = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
	})

	It("emits Benchmark<Name> rows for benchmark-targetable metrics when a benchmark is configured", func() {
		acct := portfolio.New(
			portfolio.WithCash(10_000, time.Time{}),
			portfolio.WithBenchmark(bench),
			portfolio.WithMetric(portfolio.TWRR),
		)

		dates := []time.Time{
			time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 6, 4, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 6, 5, 0, 0, 0, 0, time.UTC),
		}

		// Move the equity curve via a deposit and the benchmark via price changes.
		acct.UpdatePrices(metricsBuildDF(dates[0], bench, 100.0, 100.0))
		acct.Record(portfolio.Transaction{
			Date:   dates[1],
			Type:   asset.DepositTransaction,
			Amount: 1000.0,
		})
		acct.UpdatePrices(metricsBuildDF(dates[1], bench, 105.0, 105.0))
		acct.UpdatePrices(metricsBuildDF(dates[2], bench, 110.0, 110.0))

		var rows []portfolio.MetricRow
		count := engine.ComputeMetricsForTest(acct, dates[2], acct.RegisteredMetrics(), func(row portfolio.MetricRow) {
			rows = append(rows, row)
		})

		Expect(count).To(BeNumerically(">", 0))

		var sawTWRR, sawBenchmarkTWRR bool
		for _, row := range rows {
			if row.Name == "TWRR" && row.Window == "since_inception" {
				sawTWRR = true
			}

			if row.Name == "BenchmarkTWRR" && row.Window == "since_inception" {
				sawBenchmarkTWRR = true
			}
		}

		Expect(sawTWRR).To(BeTrue(), "expected a TWRR row from the portfolio pass")
		Expect(sawBenchmarkTWRR).To(BeTrue(), "expected a BenchmarkTWRR row from the benchmark pass")
	})

	It("skips Benchmark<Name> rows when no benchmark is configured", func() {
		acct := portfolio.New(
			portfolio.WithCash(10_000, time.Time{}),
			portfolio.WithMetric(portfolio.TWRR),
		)

		dates := []time.Time{
			time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 6, 4, 0, 0, 0, 0, time.UTC),
		}

		acct.UpdatePrices(metricsBuildDF(dates[0], spy, 100.0, 100.0))
		acct.UpdatePrices(metricsBuildDF(dates[1], spy, 105.0, 105.0))

		var rows []portfolio.MetricRow
		engine.ComputeMetricsForTest(acct, dates[1], acct.RegisteredMetrics(), func(row portfolio.MetricRow) {
			rows = append(rows, row)
		})

		for _, row := range rows {
			Expect(row.Name).NotTo(HavePrefix("Benchmark"),
				"benchmark pass should be silent when no benchmark is configured")
		}
	})

	It("omits rows whose window cannot be evaluated", func() {
		// Single equity point: every window-restricted metric returns
		// ErrInsufficientData. The since_inception pass also fails because
		// fewer than two equity points are available, so no rows are emitted.
		acct := portfolio.New(
			portfolio.WithCash(10_000, time.Time{}),
			portfolio.WithMetric(portfolio.TWRR),
		)

		date := time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC)
		acct.UpdatePrices(metricsBuildDF(date, spy, 100.0, 100.0))

		var rows []portfolio.MetricRow
		count := engine.ComputeMetricsForTest(acct, date, acct.RegisteredMetrics(), func(row portfolio.MetricRow) {
			rows = append(rows, row)
		})

		Expect(count).To(Equal(0))
		Expect(rows).To(BeEmpty())
	})
})
