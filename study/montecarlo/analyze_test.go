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

package montecarlo_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/montecarlo"
)

// fakePortfolio implements report.ReportablePortfolio with predetermined values
// for testing the analysis pipeline.
type fakePortfolio struct {
	perfData     *data.DataFrame
	summary      portfolio.Summary
	summaryErr   error
	portfolioVal float64
}

// Portfolio interface methods.
func (fp *fakePortfolio) Cash() float64                         { return 0 }
func (fp *fakePortfolio) Value() float64                        { return fp.portfolioVal }
func (fp *fakePortfolio) Position(_ asset.Asset) float64        { return 0 }
func (fp *fakePortfolio) PositionValue(_ asset.Asset) float64   { return 0 }
func (fp *fakePortfolio) Holdings(_ func(asset.Asset, float64)) {}
func (fp *fakePortfolio) Transactions() []portfolio.Transaction { return nil }
func (fp *fakePortfolio) Prices() *data.DataFrame               { return nil }
func (fp *fakePortfolio) PerfData() *data.DataFrame             { return fp.perfData }
func (fp *fakePortfolio) PerformanceMetric(_ portfolio.PerformanceMetric) portfolio.PerformanceMetricQuery {
	return portfolio.PerformanceMetricQuery{}
}
func (fp *fakePortfolio) Summary() (portfolio.Summary, error) { return fp.summary, fp.summaryErr }
func (fp *fakePortfolio) RiskMetrics() (portfolio.RiskMetrics, error) {
	return portfolio.RiskMetrics{}, nil
}
func (fp *fakePortfolio) TaxMetrics() (portfolio.TaxMetrics, error) {
	return portfolio.TaxMetrics{}, nil
}
func (fp *fakePortfolio) TradeMetrics() (portfolio.TradeMetrics, error) {
	return portfolio.TradeMetrics{}, nil
}
func (fp *fakePortfolio) WithdrawalMetrics() (portfolio.WithdrawalMetrics, error) {
	return portfolio.WithdrawalMetrics{}, nil
}
func (fp *fakePortfolio) SetMetadata(_, _ string)               {}
func (fp *fakePortfolio) GetMetadata(_ string) string           { return "" }
func (fp *fakePortfolio) Annotations() []portfolio.Annotation   { return nil }
func (fp *fakePortfolio) TradeDetails() []portfolio.TradeDetail { return nil }
func (fp *fakePortfolio) Equity() float64                       { return 0 }
func (fp *fakePortfolio) LongMarketValue() float64              { return 0 }
func (fp *fakePortfolio) ShortMarketValue() float64             { return 0 }
func (fp *fakePortfolio) MarginRatio() float64                  { return 0 }
func (fp *fakePortfolio) MarginDeficiency() float64             { return 0 }
func (fp *fakePortfolio) BuyingPower() float64                  { return 0 }
func (fp *fakePortfolio) Benchmark() asset.Asset                { return asset.Asset{} }

// PortfolioStats interface methods.
func (fp *fakePortfolio) Returns(_ context.Context, _ *portfolio.Period) *data.DataFrame { return nil }
func (fp *fakePortfolio) ExcessReturns(_ context.Context, _ *portfolio.Period) *data.DataFrame {
	return nil
}
func (fp *fakePortfolio) Drawdown(_ context.Context, _ *portfolio.Period) *data.DataFrame {
	return nil
}
func (fp *fakePortfolio) BenchmarkReturns(_ context.Context, _ *portfolio.Period) *data.DataFrame {
	return nil
}
func (fp *fakePortfolio) EquitySeries(_ context.Context, _ *portfolio.Period) *data.DataFrame {
	return nil
}
func (fp *fakePortfolio) TransactionsView(_ context.Context) []portfolio.Transaction { return nil }
func (fp *fakePortfolio) TradeDetailsView(_ context.Context) []portfolio.TradeDetail { return nil }
func (fp *fakePortfolio) PricesView(_ context.Context) *data.DataFrame               { return nil }
func (fp *fakePortfolio) TaxLotsView(_ context.Context) map[asset.Asset][]portfolio.TaxLot {
	return nil
}
func (fp *fakePortfolio) ShortLotsView(_ context.Context, _ func(asset.Asset, []portfolio.TaxLot)) {}
func (fp *fakePortfolio) PerfDataView(_ context.Context) *data.DataFrame                           { return nil }
func (fp *fakePortfolio) AnnualReturns(_ data.Metric) ([]int, []float64, error) {
	return nil, nil, nil
}
func (fp *fakePortfolio) DrawdownDetails(_ int) ([]portfolio.DrawdownDetail, error) {
	return nil, nil
}
func (fp *fakePortfolio) MonthlyReturns(_ data.Metric) ([]int, [][]float64, error) {
	return nil, nil, nil
}

// Compile-time check that fakePortfolio implements report.ReportablePortfolio.
var _ report.ReportablePortfolio = (*fakePortfolio)(nil)

// portfolioAsset mirrors the sentinel used in the production code.
var testPortfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}

// newFakePortfolio creates a fakePortfolio with an equity curve derived from
// the given values over consecutive days starting from baseDate.
func newFakePortfolio(baseDate time.Time, equityValues []float64, summary portfolio.Summary) *fakePortfolio {
	times := make([]time.Time, len(equityValues))
	for idx := range equityValues {
		times[idx] = baseDate.AddDate(0, 0, idx)
	}

	perfData, err := data.NewDataFrame(
		times,
		[]asset.Asset{testPortfolioAsset},
		[]data.Metric{data.PortfolioEquity},
		data.Daily,
		[][]float64{equityValues},
	)
	if err != nil {
		panic(err)
	}

	finalValue := 0.0
	if len(equityValues) > 0 {
		finalValue = equityValues[len(equityValues)-1]
	}

	return &fakePortfolio{
		perfData:     perfData,
		summary:      summary,
		portfolioVal: finalValue,
	}
}

var _ = Describe("analyzeResults", func() {
	baseDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create a MonteCarloStudy and use its Analyze method to access the
	// internal analyzeResults function.
	analyzeViaStudy := func(results []study.RunResult, historical report.ReportablePortfolio, ruinThreshold float64) (report.Report, error) {
		mcs := montecarlo.New(nil, nil)
		mcs.RuinThreshold = ruinThreshold
		mcs.HistoricalResult = historical

		return mcs.Analyze(results)
	}

	Context("report structure", func() {
		var rpt report.Report

		BeforeEach(func() {
			// Three paths with different equity curves.
			path1 := newFakePortfolio(baseDate, []float64{100, 110, 120}, portfolio.Summary{
				TWRR: 0.20, MaxDrawdown: -0.05, Sharpe: 1.5,
			})
			path2 := newFakePortfolio(baseDate, []float64{100, 90, 130}, portfolio.Summary{
				TWRR: 0.30, MaxDrawdown: -0.10, Sharpe: 1.2,
			})
			path3 := newFakePortfolio(baseDate, []float64{100, 105, 110}, portfolio.Summary{
				TWRR: 0.10, MaxDrawdown: -0.02, Sharpe: 0.8,
			})

			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Path 1"}, Portfolio: path1},
				{Config: study.RunConfig{Name: "Path 2"}, Portfolio: path2},
				{Config: study.RunConfig{Name: "Path 3"}, Portfolio: path3},
			}

			var err error
			rpt, err = analyzeViaStudy(results, nil, -0.30)
			Expect(err).NotTo(HaveOccurred())
		})

		It("has the correct title", func() {
			Expect(rpt.Title).To(Equal("Monte Carlo Simulation"))
		})

		It("has the expected number of sections without historical result", func() {
			// Fan chart, terminal wealth, confidence intervals, ruin, summary = 5
			Expect(rpt.Sections).To(HaveLen(5))
		})

		It("has sections in the correct order", func() {
			Expect(rpt.Sections[0].Type()).To(Equal("time_series"))
			Expect(rpt.Sections[0].Name()).To(Equal("Equity Curve Distribution"))

			Expect(rpt.Sections[1].Type()).To(Equal("table"))
			Expect(rpt.Sections[1].Name()).To(Equal("Terminal Wealth Distribution"))

			Expect(rpt.Sections[2].Type()).To(Equal("table"))
			Expect(rpt.Sections[2].Name()).To(Equal("Confidence Intervals"))

			Expect(rpt.Sections[3].Type()).To(Equal("metric_pairs"))
			Expect(rpt.Sections[3].Name()).To(Equal("Probability of Ruin"))

			Expect(rpt.Sections[4].Type()).To(Equal("text"))
			Expect(rpt.Sections[4].Name()).To(Equal("Summary"))
		})
	})

	Context("percentile computation", func() {
		It("computes correct percentiles for known inputs", func() {
			// Use 10 paths with known terminal values: 100..1000 step 100.
			paths := make([]study.RunResult, 10)
			for idx := range 10 {
				termVal := float64((idx + 1) * 100)
				paths[idx] = study.RunResult{
					Config: study.RunConfig{Name: "Path"},
					Portfolio: newFakePortfolio(baseDate, []float64{100, termVal}, portfolio.Summary{
						TWRR: (termVal - 100) / 100, MaxDrawdown: -0.05, Sharpe: 1.0,
					}),
				}
			}

			rpt, err := analyzeViaStudy(paths, nil, -0.50)
			Expect(err).NotTo(HaveOccurred())

			// Check terminal wealth table (section index 1).
			termTable, ok := rpt.Sections[1].(*report.Table)
			Expect(ok).To(BeTrue())

			// Find the P50 row.
			var medianValue float64
			for _, row := range termTable.Rows {
				if row[0] == "P50" {
					medianValue = row[1].(float64)
				}
			}

			// With values 100..1000, P50 should be around 550.
			Expect(medianValue).To(BeNumerically("~", 550, 50))
		})
	})

	Context("probability of ruin", func() {
		It("correctly calculates ruin percentage", func() {
			// 3 paths: two with mild drawdowns, one with severe drawdown.
			mildPath := newFakePortfolio(baseDate, []float64{100, 110}, portfolio.Summary{
				TWRR: 0.10, MaxDrawdown: -0.05, Sharpe: 1.0,
			})
			severePath := newFakePortfolio(baseDate, []float64{100, 60}, portfolio.Summary{
				TWRR: -0.40, MaxDrawdown: -0.40, Sharpe: -0.5,
			})

			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Path 1"}, Portfolio: mildPath},
				{Config: study.RunConfig{Name: "Path 2"}, Portfolio: mildPath},
				{Config: study.RunConfig{Name: "Path 3"}, Portfolio: severePath},
			}

			rpt, err := analyzeViaStudy(results, nil, -0.30)
			Expect(err).NotTo(HaveOccurred())

			// Ruin section is index 3.
			ruinSection, ok := rpt.Sections[3].(*report.MetricPairs)
			Expect(ok).To(BeTrue())

			// 1 of 3 paths has drawdown < -0.30 threshold.
			ruinPct := ruinSection.Metrics[0].Value
			Expect(ruinPct).To(BeNumerically("~", 1.0/3.0, 0.001))
		})
	})

	Context("historical rank", func() {
		It("includes historical rank section when historical result is provided", func() {
			path1 := newFakePortfolio(baseDate, []float64{100, 110, 120}, portfolio.Summary{
				TWRR: 0.20, MaxDrawdown: -0.05, Sharpe: 1.5,
			})
			path2 := newFakePortfolio(baseDate, []float64{100, 90, 80}, portfolio.Summary{
				TWRR: -0.20, MaxDrawdown: -0.20, Sharpe: -0.5,
			})

			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Path 1"}, Portfolio: path1},
				{Config: study.RunConfig{Name: "Path 2"}, Portfolio: path2},
			}

			historical := newFakePortfolio(baseDate, []float64{100, 105, 115}, portfolio.Summary{
				TWRR: 0.15, MaxDrawdown: -0.03, Sharpe: 1.2,
			})

			rpt, err := analyzeViaStudy(results, historical, -0.30)
			Expect(err).NotTo(HaveOccurred())

			// With historical, we should have 6 sections.
			Expect(rpt.Sections).To(HaveLen(6))

			// Historical rank is section index 4.
			rankSection, ok := rpt.Sections[4].(*report.MetricPairs)
			Expect(ok).To(BeTrue())
			Expect(rankSection.Name()).To(Equal("Historical Rank"))
			Expect(rankSection.Metrics).To(HaveLen(4))
		})

		It("omits historical rank section when historical result is nil", func() {
			path := newFakePortfolio(baseDate, []float64{100, 110}, portfolio.Summary{
				TWRR: 0.10, MaxDrawdown: -0.05, Sharpe: 1.0,
			})

			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Path 1"}, Portfolio: path},
			}

			rpt, err := analyzeViaStudy(results, nil, -0.30)
			Expect(err).NotTo(HaveOccurred())

			// Without historical, we should have 5 sections.
			Expect(rpt.Sections).To(HaveLen(5))

			// No section should be named "Historical Rank".
			for _, section := range rpt.Sections {
				Expect(section.Name()).NotTo(Equal("Historical Rank"))
			}
		})
	})

	Context("failed run filtering", func() {
		It("filters out runs with errors", func() {
			goodPath := newFakePortfolio(baseDate, []float64{100, 110, 120}, portfolio.Summary{
				TWRR: 0.20, MaxDrawdown: -0.05, Sharpe: 1.5,
			})

			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Good"}, Portfolio: goodPath},
				{Config: study.RunConfig{Name: "Bad"}, Portfolio: nil, Err: errors.New("simulation failed")},
			}

			rpt, err := analyzeViaStudy(results, nil, -0.30)
			Expect(err).NotTo(HaveOccurred())

			// Report should still be valid with 5 sections.
			Expect(rpt.Sections).To(HaveLen(5))
			Expect(rpt.Title).To(Equal("Monte Carlo Simulation"))

			// Summary text should mention the failed path.
			summarySection, ok := rpt.Sections[4].(*report.Text)
			Expect(ok).To(BeTrue())
			Expect(summarySection.Body).To(ContainSubstring("1 of 2 paths succeeded"))
		})

		It("filters out runs with nil portfolios", func() {
			goodPath := newFakePortfolio(baseDate, []float64{100, 110}, portfolio.Summary{
				TWRR: 0.10, MaxDrawdown: -0.05, Sharpe: 1.0,
			})

			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Good"}, Portfolio: goodPath},
				{Config: study.RunConfig{Name: "Nil Portfolio"}, Portfolio: nil},
			}

			rpt, err := analyzeViaStudy(results, nil, -0.30)
			Expect(err).NotTo(HaveOccurred())

			Expect(rpt.Sections).To(HaveLen(5))
		})

		It("returns a placeholder report when all runs fail", func() {
			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Bad"}, Err: errors.New("failed")},
			}

			rpt, err := analyzeViaStudy(results, nil, -0.30)
			Expect(err).NotTo(HaveOccurred())

			Expect(rpt.Title).To(Equal("Monte Carlo Simulation"))
			Expect(rpt.Sections).To(HaveLen(1))

			textSection, ok := rpt.Sections[0].(*report.Text)
			Expect(ok).To(BeTrue())
			Expect(textSection.Body).To(ContainSubstring("No successful"))
		})
	})

	Context("fan chart", func() {
		It("includes historical series in fan chart when provided", func() {
			path := newFakePortfolio(baseDate, []float64{100, 110}, portfolio.Summary{
				TWRR: 0.10, MaxDrawdown: -0.05, Sharpe: 1.0,
			})
			historical := newFakePortfolio(baseDate, []float64{100, 105}, portfolio.Summary{
				TWRR: 0.05, MaxDrawdown: -0.01, Sharpe: 0.5,
			})

			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Path 1"}, Portfolio: path},
			}

			rpt, err := analyzeViaStudy(results, historical, -0.30)
			Expect(err).NotTo(HaveOccurred())

			fanChart, ok := rpt.Sections[0].(*report.TimeSeries)
			Expect(ok).To(BeTrue())

			// Should have 6 series: P5, P25, P50, P75, P95, Historical.
			Expect(fanChart.Series).To(HaveLen(6))
			Expect(fanChart.Series[5].Name).To(Equal("Historical"))
		})
	})

	Context("confidence intervals table", func() {
		It("contains TWRR, Max Drawdown, and Sharpe rows", func() {
			path := newFakePortfolio(baseDate, []float64{100, 110}, portfolio.Summary{
				TWRR: 0.10, MaxDrawdown: -0.05, Sharpe: 1.0,
			})

			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Path 1"}, Portfolio: path},
			}

			rpt, err := analyzeViaStudy(results, nil, -0.30)
			Expect(err).NotTo(HaveOccurred())

			ciTable, ok := rpt.Sections[2].(*report.Table)
			Expect(ok).To(BeTrue())
			Expect(ciTable.Rows).To(HaveLen(3))
			Expect(ciTable.Rows[0][0]).To(Equal("TWRR"))
			Expect(ciTable.Rows[1][0]).To(Equal("Max Drawdown"))
			Expect(ciTable.Rows[2][0]).To(Equal("Sharpe"))
		})
	})
})
