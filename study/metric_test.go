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

package study_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/report"
)

// portfolioEquityAsset mirrors the sentinel used by portfolio/account.go to
// write equity values into the performance DataFrame.
var portfolioEquityAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}

// metricFakePortfolio is a minimal implementation of report.ReportablePortfolio
// that returns a fixed performance DataFrame from PerfData. The embedded
// portfolio.Account satisfies the remaining interface methods.
type metricFakePortfolio struct {
	*portfolio.Account
	perfDF *data.DataFrame
}

func (fp *metricFakePortfolio) PerfData() *data.DataFrame {
	return fp.perfDF
}

var _ report.ReportablePortfolio = (*metricFakePortfolio)(nil)

// buildMetricPerfDF constructs a minimal performance DataFrame with a
// PortfolioEquity column for the given dates and equity values.
func buildMetricPerfDF(dates []time.Time, equityValues []float64) *data.DataFrame {
	df, err := data.NewDataFrame(
		dates,
		[]asset.Asset{portfolioEquityAsset},
		[]data.Metric{data.PortfolioEquity},
		data.Daily,
		[][]float64{equityValues},
	)
	Expect(err).NotTo(HaveOccurred())

	return df
}

// buildMetricFakePortfolio creates a metricFakePortfolio backed by the given equity curve.
func buildMetricFakePortfolio(dates []time.Time, equityValues []float64) *metricFakePortfolio {
	acct := portfolio.New(portfolio.WithCash(equityValues[0], dates[0]))
	perfDF := buildMetricPerfDF(dates, equityValues)
	acct.SetPerfData(perfDF)

	return &metricFakePortfolio{
		Account: acct,
		perfDF:  perfDF,
	}
}

var _ = Describe("Metric", func() {
	var (
		dates        []time.Time
		equityValues []float64
		fakePF       *metricFakePortfolio
		fullWindow   study.DateRange
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
			time.Date(2020, 8, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 11, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 12, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2021, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		// A simple growing equity curve.
		equityValues = []float64{
			10_000, 10_200, 10_400, 10_600, 10_800,
			11_000, 11_200, 11_400, 11_600, 11_800,
			12_000, 12_200, 12_400,
		}
		fakePF = buildMetricFakePortfolio(dates, equityValues)
		fullWindow = study.DateRange{Start: dates[0], End: dates[len(dates)-1]}
	})

	Describe("WindowedScore", func() {
		It("returns a finite value for a known portfolio and window", func() {
			score := study.WindowedScore(fakePF, fullWindow, study.MetricCAGR)
			Expect(math.IsNaN(score)).To(BeFalse(), "expected finite CAGR score, got NaN")
		})

		It("returns NaN for Sharpe when risk-free rate data is absent", func() {
			// The fake portfolio has equity data but no risk-free rate column,
			// so Sharpe correctly returns NaN via ErrNoRiskFreeRate.
			score := study.WindowedScore(fakePF, fullWindow, study.MetricSharpe)
			Expect(math.IsNaN(score)).To(BeTrue(), "expected NaN Sharpe when risk-free data is missing")
		})

		It("returns a zero value (not NaN) for a window that contains no data", func() {
			// CAGR (and most metrics) return 0, nil when the windowed data is
			// empty rather than returning an error. WindowedScore therefore
			// returns 0 rather than NaN in this case.
			emptyWindow := study.DateRange{
				Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			}
			score := study.WindowedScore(fakePF, emptyWindow, study.MetricCAGR)
			Expect(math.IsNaN(score)).To(BeFalse(), "expected non-NaN for empty window")
			Expect(score).To(Equal(0.0))
		})
	})

	Describe("WindowedScoreExcluding", func() {
		It("delegates to WindowedScore when exclude is nil", func() {
			direct := study.WindowedScore(fakePF, fullWindow, study.MetricCAGR)
			excluding := study.WindowedScoreExcluding(fakePF, fullWindow, nil, study.MetricCAGR)
			Expect(excluding).To(Equal(direct))
		})

		It("delegates to WindowedScore when exclude is empty", func() {
			direct := study.WindowedScore(fakePF, fullWindow, study.MetricCAGR)
			excluding := study.WindowedScoreExcluding(fakePF, fullWindow, []study.DateRange{}, study.MetricCAGR)
			Expect(excluding).To(Equal(direct))
		})

		It("produces a different score when a middle segment is excluded", func() {
			fullScore := study.WindowedScore(fakePF, fullWindow, study.MetricCAGR)

			excludeRange := study.DateRange{
				Start: time.Date(2020, 4, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2020, 8, 1, 0, 0, 0, 0, time.UTC),
			}
			excludedScore := study.WindowedScoreExcluding(
				fakePF, fullWindow, []study.DateRange{excludeRange}, study.MetricCAGR,
			)

			Expect(math.IsNaN(excludedScore)).To(BeFalse(), "excluded score should not be NaN")
			Expect(excludedScore).NotTo(Equal(fullScore),
				"excluding a middle segment should change the score")
		})

		It("returns NaN when the exclusion covers the entire window", func() {
			score := study.WindowedScoreExcluding(
				fakePF, fullWindow, []study.DateRange{fullWindow}, study.MetricCAGR,
			)
			Expect(math.IsNaN(score)).To(BeTrue(),
				"full exclusion should return NaN")
		})
	})

	Describe("Metric.performanceMetric panic", func() {
		It("panics on an unknown metric value", func() {
			unknownMetric := study.Metric(9999)
			Expect(func() {
				study.WindowedScore(fakePF, fullWindow, unknownMetric)
			}).To(Panic())
		})
	})
})
