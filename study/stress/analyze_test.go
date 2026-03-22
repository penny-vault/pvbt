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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
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

var _ = Describe("Analyze", func() {
	var (
		scenarios  []stress.Scenario
		stressTest *stress.StressTest
	)

	BeforeEach(func() {
		scenarios = []stress.Scenario{
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
		It("returns a valid report with the expected section types", func() {
			rpt, err := stressTest.Analyze([]study.RunResult{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rpt.Title).NotTo(BeEmpty())

			sectionTypes := make([]string, len(rpt.Sections))
			for idx, section := range rpt.Sections {
				sectionTypes[idx] = section.Type()
			}

			Expect(sectionTypes).To(ContainElement("table"))
			Expect(sectionTypes).To(ContainElement("text"))
		})
	})

	Describe("with failed results", func() {
		It("includes failed runs in the table with error information", func() {
			results := []study.RunResult{
				{
					Config: study.RunConfig{Name: "FailedRun"},
					Err:    errors.New("backtest engine error"),
				},
			}

			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			var rankingTable *report.Table

			for _, section := range rpt.Sections {
				if section.Type() == "table" {
					rankingTable = section.(*report.Table)

					break
				}
			}

			Expect(rankingTable).NotTo(BeNil())
			Expect(rankingTable.Rows).NotTo(BeEmpty())

			found := false

			for _, row := range rankingTable.Rows {
				for _, cell := range row {
					if cellStr, ok := cell.(string); ok && cellStr == "backtest engine error" {
						found = true

						break
					}
				}
			}

			Expect(found).To(BeTrue(), "expected error message in ranking table rows")
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
			Expect(rpt.Title).NotTo(BeEmpty())
		})

		It("produces a ranking table section", func() {
			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			sectionTypes := make([]string, len(rpt.Sections))
			for idx, section := range rpt.Sections {
				sectionTypes[idx] = section.Type()
			}

			Expect(sectionTypes).To(ContainElement("table"))
		})

		It("produces one metric_pairs section per scenario", func() {
			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			metricPairCount := 0

			for _, section := range rpt.Sections {
				if section.Type() == "metric_pairs" {
					metricPairCount++
				}
			}

			Expect(metricPairCount).To(Equal(len(scenarios)))
		})

		It("produces a text summary section", func() {
			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			found := false

			for _, section := range rpt.Sections {
				if section.Type() == "text" {
					found = true

					break
				}
			}

			Expect(found).To(BeTrue())
		})

		It("names MetricPairs sections after their scenario", func() {
			rpt, err := stressTest.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			sectionNames := make([]string, 0)

			for _, section := range rpt.Sections {
				if section.Type() == "metric_pairs" {
					sectionNames = append(sectionNames, section.Name())
				}
			}

			for _, scenario := range scenarios {
				foundScenario := false

				for _, sectionName := range sectionNames {
					if len(sectionName) >= len(scenario.Name) && sectionName[:len(scenario.Name)] == scenario.Name {
						foundScenario = true

						break
					}
				}

				Expect(foundScenario).To(BeTrue(),
					"expected a metric_pairs section starting with scenario name %q", scenario.Name)
			}
		})
	})

	Describe("with mixed results (some failed, some successful)", func() {
		It("processes successful runs while including failed ones in the table", func() {
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

			sectionTypes := make([]string, len(rpt.Sections))
			for idx, section := range rpt.Sections {
				sectionTypes[idx] = section.Type()
			}

			Expect(sectionTypes).To(ContainElement("table"))
			Expect(sectionTypes).To(ContainElement("metric_pairs"))
			Expect(sectionTypes).To(ContainElement("text"))
		})
	})
})
