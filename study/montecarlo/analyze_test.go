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
	"bytes"
	"context"
	"errors"
	"github.com/bytedance/sonic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/montecarlo"
	"github.com/penny-vault/pvbt/study/report"
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
func (fp *fakePortfolio) Holdings() map[asset.Asset]float64     { return nil }
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
func (fp *fakePortfolio) FactorAnalysis(_ *data.DataFrame) (*portfolio.FactorRegression, error) {
	return nil, nil
}
func (fp *fakePortfolio) StepwiseFactorAnalysis(_ *data.DataFrame) (*portfolio.StepwiseResult, error) {
	return nil, nil
}

func (fp *fakePortfolio) View(_, _ time.Time) portfolio.Portfolio { return fp }

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

// mcReportData mirrors the JSON structure of the monteCarloReport for
// test deserialization. Only fields that the tests inspect are included.
type mcReportData struct {
	FanChart struct {
		P5     []float64 `json:"p5"`
		P25    []float64 `json:"p25"`
		P50    []float64 `json:"p50"`
		P75    []float64 `json:"p75"`
		P95    []float64 `json:"p95"`
		Actual *struct {
			Values []float64 `json:"values"`
		} `json:"actual,omitempty"`
	} `json:"fanChart"`

	TerminalWealth []struct {
		Label string  `json:"label"`
		Value float64 `json:"value"`
	} `json:"terminalWealth"`

	ConfidenceIntervals []struct {
		Metric string `json:"metric"`
	} `json:"confidenceIntervals"`

	Ruin struct {
		Probability float64 `json:"probability"`
		Threshold   float64 `json:"threshold"`
	} `json:"ruin"`

	HistoricalRank *struct {
		TerminalValuePercentile float64 `json:"terminalValuePercentile"`
		TWRRPercentile          float64 `json:"twrrPercentile"`
		MaxDrawdownPercentile   float64 `json:"maxDrawdownPercentile"`
		SharpePercentile        float64 `json:"sharpePercentile"`
	} `json:"historicalRank,omitempty"`

	Summary string `json:"summary"`
}

// decodeReport calls Data on the report.Report and unmarshals the JSON.
func decodeReport(rpt report.Report) mcReportData {
	var buf bytes.Buffer
	Expect(rpt.Data(&buf)).To(Succeed())

	var result mcReportData
	Expect(sonic.Unmarshal(buf.Bytes(), &result)).To(Succeed())

	return result
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
		var rptData mcReportData

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
			rptData = decodeReport(rpt)
		})

		It("has the correct component name", func() {
			Expect(rpt.Name()).To(Equal("MonteCarlo"))
		})

		It("has fan chart data without historical result", func() {
			Expect(rptData.FanChart.P50).NotTo(BeEmpty())
			Expect(rptData.FanChart.Actual).To(BeNil())
		})

		It("has terminal wealth distribution", func() {
			Expect(rptData.TerminalWealth).NotTo(BeEmpty())
		})

		It("has confidence intervals for TWRR, Max Drawdown, and Sharpe", func() {
			Expect(rptData.ConfidenceIntervals).To(HaveLen(3))
			Expect(rptData.ConfidenceIntervals[0].Metric).To(Equal("TWRR"))
			Expect(rptData.ConfidenceIntervals[1].Metric).To(Equal("Max Drawdown"))
			Expect(rptData.ConfidenceIntervals[2].Metric).To(Equal("Sharpe"))
		})

		It("has ruin probability data", func() {
			Expect(rptData.Ruin.Threshold).To(Equal(-0.30))
		})

		It("has no historical rank without historical result", func() {
			Expect(rptData.HistoricalRank).To(BeNil())
		})

		It("has a summary narrative", func() {
			Expect(rptData.Summary).NotTo(BeEmpty())
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
			rptData := decodeReport(rpt)

			// Find the P50 entry.
			var medianValue float64
			for _, stat := range rptData.TerminalWealth {
				if stat.Label == "P50" {
					medianValue = stat.Value
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
			rptData := decodeReport(rpt)

			// 1 of 3 paths has drawdown < -0.30 threshold.
			Expect(rptData.Ruin.Probability).To(BeNumerically("~", 1.0/3.0, 0.001))
		})
	})

	Context("historical rank", func() {
		It("includes historical rank when historical result is provided", func() {
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
			rptData := decodeReport(rpt)

			Expect(rptData.HistoricalRank).NotTo(BeNil())
		})

		It("omits historical rank when historical result is nil", func() {
			path := newFakePortfolio(baseDate, []float64{100, 110}, portfolio.Summary{
				TWRR: 0.10, MaxDrawdown: -0.05, Sharpe: 1.0,
			})

			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Path 1"}, Portfolio: path},
			}

			rpt, err := analyzeViaStudy(results, nil, -0.30)
			Expect(err).NotTo(HaveOccurred())
			rptData := decodeReport(rpt)

			Expect(rptData.HistoricalRank).To(BeNil())
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
			rptData := decodeReport(rpt)

			// Summary text should mention the failed path.
			Expect(rptData.Summary).To(ContainSubstring("1 of 2 paths succeeded"))
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

			// Report should still produce valid JSON.
			rptData := decodeReport(rpt)
			Expect(rptData.Summary).NotTo(BeEmpty())
		})

		It("returns a placeholder report when all runs fail", func() {
			results := []study.RunResult{
				{Config: study.RunConfig{Name: "Bad"}, Err: errors.New("failed")},
			}

			rpt, err := analyzeViaStudy(results, nil, -0.30)
			Expect(err).NotTo(HaveOccurred())

			Expect(rpt.Name()).To(Equal("MonteCarlo"))

			rptData := decodeReport(rpt)
			Expect(rptData.Summary).To(ContainSubstring("No successful"))
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
			rptData := decodeReport(rpt)

			Expect(rptData.FanChart.Actual).NotTo(BeNil())
			Expect(rptData.FanChart.Actual.Values).NotTo(BeEmpty())
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
			rptData := decodeReport(rpt)

			Expect(rptData.ConfidenceIntervals).To(HaveLen(3))
			Expect(rptData.ConfidenceIntervals[0].Metric).To(Equal("TWRR"))
			Expect(rptData.ConfidenceIntervals[1].Metric).To(Equal("Max Drawdown"))
			Expect(rptData.ConfidenceIntervals[2].Metric).To(Equal("Sharpe"))
		})
	})
})
