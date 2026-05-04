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

package stress_test

import (
	"bytes"
	"errors"
	"io"
	"time"

	"github.com/bytedance/sonic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/report"
	"github.com/penny-vault/pvbt/study/stress"
)

// portfolioEquityAsset is the sentinel asset used to write equity values
// into the performance DataFrame, matching the unexported constant in
// portfolio/account.go.
var portfolioEquityAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}

// buildPerfDataFrame constructs a minimal performance DataFrame with a
// PortfolioEquity column for the given dates and equity values.
func buildPerfDataFrame(dates []time.Time, equityValues []float64) *data.DataFrame {
	columns := [][]float64{equityValues}

	df, err := data.NewDataFrame(
		dates,
		[]asset.Asset{portfolioEquityAsset},
		[]data.Metric{data.PortfolioEquity},
		data.Daily,
		columns,
	)
	Expect(err).NotTo(HaveOccurred())

	return df
}

// fakePortfolio is a minimal implementation of report.ReportablePortfolio
// that returns a fixed performance DataFrame from PerfData. The embedded
// portfolio.Account satisfies the remaining interface methods.
type fakePortfolio struct {
	*portfolio.Account
	perfDF *data.DataFrame
}

func (fp *fakePortfolio) PerfData() *data.DataFrame {
	return fp.perfDF
}

var _ report.ReportablePortfolio = (*fakePortfolio)(nil)

// buildFakePortfolio creates a fakePortfolio backed by the given equity curve.
func buildFakePortfolio(dates []time.Time, equityValues []float64) *fakePortfolio {
	acct := portfolio.New(portfolio.WithCash(equityValues[0], dates[0]))

	return &fakePortfolio{
		Account: acct,
		perfDF:  buildPerfDataFrame(dates, equityValues),
	}
}

// stressReportData mirrors the JSON structure of the stressReport for
// test deserialization. Only fields that the tests inspect are included.
type stressReportData struct {
	Rankings []struct {
		RunName      string  `json:"runName"`
		ScenarioName string  `json:"scenarioName"`
		ErrorMsg     string  `json:"errorMsg,omitempty"`
		MaxDrawdown  float64 `json:"maxDrawdown"`
		TotalReturn  float64 `json:"totalReturn"`
		WorstDay     float64 `json:"worstDay"`
	} `json:"rankings"`

	Scenarios []struct {
		Name       string `json:"name"`
		DateRange  string `json:"dateRange"`
		RunMetrics []struct {
			RunName     string  `json:"runName"`
			ErrorMsg    string  `json:"errorMsg,omitempty"`
			HasData     bool    `json:"hasData"`
			MaxDrawdown float64 `json:"maxDrawdown"`
			TotalReturn float64 `json:"totalReturn"`
			WorstDay    float64 `json:"worstDay"`
		} `json:"runMetrics"`
	} `json:"scenarios"`

	Summary string `json:"summary"`
}

// decodeStressReport calls Data on the report and unmarshals the JSON.
func decodeStressReport(rpt report.Report) stressReportData {
	var buf bytes.Buffer
	Expect(rpt.Data(&buf)).To(Succeed())

	var result stressReportData
	Expect(sonic.Unmarshal(buf.Bytes(), &result)).To(Succeed())

	return result
}

var _ = Describe("Analyze", func() {
	var (
		scenarios  []study.Scenario
		stressTest *stress.StressTest
	)

	BeforeEach(func() {
		scenarios = []study.Scenario{
			{
				Name:        "Downturn",
				Description: "Test downturn",
				Start:       time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:         time.Date(2020, 3, 31, 0, 0, 0, 0, time.UTC),
			},
			{
				Name:        "Recovery",
				Description: "Test recovery",
				Start:       time.Date(2020, 4, 1, 0, 0, 0, 0, time.UTC),
				End:         time.Date(2020, 6, 30, 0, 0, 0, 0, time.UTC),
			},
		}
		stressTest = stress.New(scenarios)
	})

	Describe("with empty results", func() {
		It("returns a valid report with the expected fields", func() {
			rpt, err := stressTest.Analyze([]study.RunResult{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rpt.Name()).NotTo(BeEmpty())

			rptData := decodeStressReport(rpt)
			Expect(rptData.Rankings).To(HaveLen(0))
			Expect(rptData.Summary).NotTo(BeEmpty())
		})
	})

	Describe("with failed results", func() {
		It("includes failed runs in the rankings with error information", func() {
			results := []study.RunResult{
				{
					Config: study.RunConfig{Name: "FailedRun"},
					Err:    errors.New("backtest engine error"),
				},
			}

			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeStressReport(rpt)
			Expect(rptData.Rankings).NotTo(BeEmpty())

			found := false
			for _, ranking := range rptData.Rankings {
				if ranking.ErrorMsg == "backtest engine error" {
					found = true

					break
				}
			}

			Expect(found).To(BeTrue(), "expected error message in rankings")
		})
	})

	Describe("with successful results", func() {
		var results []study.RunResult

		BeforeEach(func() {
			dates := []time.Time{
				time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 2, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 3, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 4, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 6, 29, 0, 0, 0, 0, time.UTC),
			}
			// Equity drops from 12k peak to 8k, then recovers above start.
			equityValues := []float64{10_000, 12_000, 11_000, 8_000, 9_000, 10_500, 11_500}
			fakePF := buildFakePortfolio(dates, equityValues)

			results = []study.RunResult{
				{
					Config:    study.RunConfig{Name: "TestRun"},
					Portfolio: fakePF,
				},
			}
		})

		It("returns a report without error", func() {
			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())
			Expect(rpt.Name()).NotTo(BeEmpty())
		})

		It("produces a rankings section", func() {
			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeStressReport(rpt)
			Expect(rptData.Rankings).NotTo(BeEmpty())
		})

		It("produces one scenario detail per scenario", func() {
			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeStressReport(rpt)
			Expect(rptData.Scenarios).To(HaveLen(len(scenarios)))
		})

		It("produces a summary", func() {
			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeStressReport(rpt)
			Expect(rptData.Summary).NotTo(BeEmpty())
		})

		It("names scenario details after their scenario", func() {
			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeStressReport(rpt)

			scenarioNames := make([]string, len(rptData.Scenarios))
			for idx, scenario := range rptData.Scenarios {
				scenarioNames[idx] = scenario.Name
			}

			for _, scenario := range scenarios {
				Expect(scenarioNames).To(ContainElement(scenario.Name))
			}
		})

		It("renders a plain-text table via the Text method", func() {
			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			textReport, ok := rpt.(interface{ Text(io.Writer) error })
			Expect(ok).To(BeTrue(), "stress report must implement Text(io.Writer) error")

			var buf bytes.Buffer
			Expect(textReport.Text(&buf)).To(Succeed())

			out := buf.String()
			Expect(out).To(ContainSubstring("Stress Test"))
			Expect(out).To(ContainSubstring("Scenario"))
			Expect(out).To(ContainSubstring("MaxDD"))
			Expect(out).To(ContainSubstring("TestRun"))
			for _, scenario := range scenarios {
				Expect(out).To(ContainSubstring(scenario.Name))
			}
		})
	})

	Describe("with mixed results (some failed, some successful)", func() {
		It("processes successful runs while including failed ones in rankings", func() {
			dates := []time.Time{
				time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 3, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2020, 6, 29, 0, 0, 0, 0, time.UTC),
			}
			equityValues := []float64{10_000, 8_000, 11_000}
			fakePF := buildFakePortfolio(dates, equityValues)

			results := []study.RunResult{
				{
					Config:    study.RunConfig{Name: "GoodRun"},
					Portfolio: fakePF,
				},
				{
					Config: study.RunConfig{Name: "BadRun"},
					Err:    errors.New("something went wrong"),
				},
			}

			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeStressReport(rpt)
			Expect(rptData.Rankings).NotTo(BeEmpty())
			Expect(rptData.Scenarios).NotTo(BeEmpty())
			Expect(rptData.Summary).NotTo(BeEmpty())
		})
	})
})
