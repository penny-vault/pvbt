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

package optimize_test

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/bytedance/sonic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/optimize"
	"github.com/penny-vault/pvbt/study/report"
)

// priceAsset is used to build single-row DataFrames for UpdatePrices.
var priceAsset = asset.Asset{
	CompositeFigi: "SPY",
	Ticker:        "SPY",
}

// buildPriceDF builds a single-timestamp DataFrame with MetricClose and
// AdjClose for the price asset.
func buildPriceDF(ts time.Time, price float64) *data.DataFrame {
	df, err := data.NewDataFrame(
		[]time.Time{ts},
		[]asset.Asset{priceAsset},
		[]data.Metric{data.MetricClose, data.AdjClose},
		data.Daily,
		[][]float64{{price}, {price}},
	)
	Expect(err).NotTo(HaveOccurred())

	return df
}

// buildAccountFromEquity creates an Account whose perfData equity curve
// matches the given values. It uses dividend/fee transactions to adjust
// cash between UpdatePrices calls; these are invisible to TWRR's and
// CAGR's flow filter (which only looks at deposits/withdrawals), so
// each step's change registers as market-driven growth rather than an
// external flow.
func buildAccountFromEquity(dates []time.Time, equityValues []float64) *portfolio.Account {
	acct := portfolio.New(portfolio.WithCash(equityValues[0], time.Time{}))

	for idx, val := range equityValues {
		if idx > 0 {
			diff := val - equityValues[idx-1]
			if diff > 0 {
				acct.Record(portfolio.Transaction{
					Date:   dates[idx],
					Type:   asset.DividendTransaction,
					Amount: diff,
				})
			} else if diff < 0 {
				acct.Record(portfolio.Transaction{
					Date:   dates[idx],
					Type:   asset.FeeTransaction,
					Amount: diff,
				})
			}
		}

		acct.UpdatePrices(buildPriceDF(dates[idx], 100))
	}

	return acct
}

// makeResult creates a RunResult with the given combination ID, split index,
// params, and portfolio.
func makeResult(comboID string, splitIdx int, params map[string]string, pf report.ReportablePortfolio) study.RunResult {
	return study.RunResult{
		Config: study.RunConfig{
			Name:   comboID,
			Params: params,
			Metadata: map[string]string{
				"_combination_id": comboID,
				"_split_index":    itoa(splitIdx),
			},
		},
		Portfolio: pf,
	}
}

// itoa converts an int to its decimal string representation.
func itoa(val int) string {
	if val == 0 {
		return "0"
	}

	negative := val < 0
	if negative {
		val = -val
	}

	digits := make([]byte, 0, 10)

	for val > 0 {
		digits = append(digits, byte('0'+val%10))
		val /= 10
	}

	// Reverse.
	for left, right := 0, len(digits)-1; left < right; left, right = left+1, right-1 {
		digits[left], digits[right] = digits[right], digits[left]
	}

	if negative {
		return "-" + string(digits)
	}

	return string(digits)
}

// optReportData mirrors the JSON structure of the optimizerReport for
// test deserialization. Only fields that the tests inspect are included.
type optReportData struct {
	ObjectiveName string `json:"objectiveName"`
	Warning       string `json:"warning,omitempty"`

	Rankings []struct {
		Rank       int     `json:"rank"`
		Parameters string  `json:"parameters"`
		MeanOOS    float64 `json:"meanOOS"`
		MeanIS     float64 `json:"meanIS"`
		OOSStdDev  float64 `json:"oosStdDev"`
	} `json:"rankings"`

	BestDetail *struct {
		Parameters string `json:"parameters"`
		Folds      []struct {
			FoldName string  `json:"foldName"`
			ISScore  float64 `json:"isScore"`
			OOSScore float64 `json:"oosScore"`
		} `json:"folds"`
	} `json:"bestDetail,omitempty"`

	Overfitting []struct {
		Parameters  string  `json:"parameters"`
		MeanIS      float64 `json:"meanIS"`
		MeanOOS     float64 `json:"meanOOS"`
		Degradation float64 `json:"degradation"`
	} `json:"overfitting"`

	EquityCurves []struct {
		Name   string      `json:"name"`
		Times  []time.Time `json:"times"`
		Values []float64   `json:"values"`
	} `json:"equityCurves"`
}

// decodeOptReport calls Data on the report and unmarshals the JSON.
func decodeOptReport(rpt report.Report) optReportData {
	var buf bytes.Buffer
	Expect(rpt.Data(&buf)).To(Succeed())

	var result optReportData
	Expect(sonic.Unmarshal(buf.Bytes(), &result)).To(Succeed())

	return result
}

var _ = Describe("Analyze", func() {
	var splits []study.Split

	BeforeEach(func() {
		splits = []study.Split{
			{
				Name: "fold 1/2",
				FullRange: study.DateRange{
					Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				Train: study.DateRange{
					Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC),
				},
				Test: study.DateRange{
					Start: time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			{
				Name: "fold 2/2",
				FullRange: study.DateRange{
					Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				Train: study.DateRange{
					Start: time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				Test: study.DateRange{
					Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		}
	})

	Describe("with empty results", func() {
		It("returns a valid report with expected fields", func() {
			opt := optimize.New(splits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze([]study.RunResult{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rpt.Name()).NotTo(BeEmpty())

			rptData := decodeOptReport(rpt)
			Expect(rptData.Rankings).NotTo(BeNil())
			Expect(rptData.EquityCurves).NotTo(BeNil())
		})
	})

	Describe("with multiple combinations", func() {
		var results []study.RunResult

		BeforeEach(func() {
			dates := makeDailyDates(
				time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
			)

			// Combo A: strong equity growth (should rank higher for Sharpe).
			equityA := makeLinearEquity(dates, 10000, 20000)
			acctA := buildAccountFromEquity(dates, equityA)

			// Combo B: flat equity (should rank lower for Sharpe).
			equityB := makeLinearEquity(dates, 10000, 10100)
			acctB := buildAccountFromEquity(dates, equityB)

			paramsA := map[string]string{"lookback": "20"}
			paramsB := map[string]string{"lookback": "50"}

			results = []study.RunResult{
				makeResult("combo-a", 0, paramsA, acctA),
				makeResult("combo-a", 1, paramsA, acctA),
				makeResult("combo-b", 0, paramsB, acctB),
				makeResult("combo-b", 1, paramsB, acctB),
			}
		})

		It("groups by combination ID", func() {
			opt := optimize.New(splits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.Rankings).To(HaveLen(2))
		})

		It("ranks combos with better OOS scores first", func() {
			opt := optimize.New(splits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.Rankings).To(HaveLen(2))

			// First row should be rank 1.
			firstRow := rptData.Rankings[0]
			Expect(firstRow.Rank).To(Equal(1))

			// Combo A (lookback=20, strong growth) should rank above
			// combo B (lookback=50, flat).
			Expect(firstRow.Parameters).To(ContainSubstring("lookback=20"))
		})

		It("produces a best combo detail", func() {
			opt := optimize.New(splits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.BestDetail).NotTo(BeNil())
			Expect(rptData.BestDetail.Folds).To(HaveLen(len(splits)))
		})

		It("produces an overfitting check", func() {
			opt := optimize.New(splits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.Overfitting).To(HaveLen(2))
		})

		It("produces equity curves with real data for the top combos", func() {
			opt := optimize.New(splits, optimize.WithTopN(1))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.EquityCurves).To(HaveLen(1))
			Expect(rptData.EquityCurves[0].Times).NotTo(BeEmpty(), "equity curve should have timestamps")
			Expect(rptData.EquityCurves[0].Values).NotTo(BeEmpty(), "equity curve should have values")
		})
	})

	Describe("with results missing metadata", func() {
		It("ignores results without _combination_id", func() {
			opt := optimize.New(splits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			results := []study.RunResult{
				{
					Config: study.RunConfig{
						Name:     "orphan",
						Metadata: map[string]string{},
					},
				},
			}

			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.Rankings).To(BeEmpty())
		})
	})

	Describe("degenerate ranking detection", func() {
		var trainTestSplits []study.Split

		BeforeEach(func() {
			start := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
			cutoff := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)

			built, err := study.TrainTest(start, cutoff, end)
			Expect(err).NotTo(HaveOccurred())
			trainTestSplits = built
		})

		It("emits a warning when every combo produces an identical OOS score", func() {
			dates := makeDailyDates(
				time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
			)

			// Strategy that never trades: equity stays at the initial cash.
			// CAGR/Sharpe/Sortino/Calmar all collapse to the same value across
			// every parameter combination, so the optimizer cannot distinguish
			// between them.
			flatEquity := makeLinearEquity(dates, 10_000, 10_000)

			var results []study.RunResult
			for idx := range 4 {
				acct := buildAccountFromEquity(dates, flatEquity)
				params := map[string]string{"lookback": fmt.Sprintf("%d", 10+idx*5)}
				results = append(results, makeResult(fmt.Sprintf("combo-%d", idx), 0, params, acct))
			}

			opt := optimize.New(trainTestSplits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.Warning).NotTo(BeEmpty(),
				"expected a warning when every combo produces the same CAGR score")
			Expect(rptData.Warning).To(ContainSubstring("CAGR"))
			Expect(rptData.Warning).To(ContainSubstring("insertion order"))
		})

		It("emits a warning when every combo's score is undefined (NaN)", func() {
			// Out-of-range _split_index keeps every combo's score at the
			// initial NaN, simulating a path where the metric could not
			// be computed for any combination + split pair.
			dates := makeDailyDates(
				time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
			)
			acct := buildAccountFromEquity(dates, makeLinearEquity(dates, 10_000, 12_000))

			makeNoScoreResult := func(comboID string, params map[string]string) study.RunResult {
				return study.RunResult{
					Config: study.RunConfig{
						Name:   comboID,
						Params: params,
						Metadata: map[string]string{
							"_combination_id": comboID,
							"_split_index":    "9999", // out of range
						},
					},
					Portfolio: acct,
				}
			}

			results := []study.RunResult{
				makeNoScoreResult("combo-a", map[string]string{"lookback": "10"}),
				makeNoScoreResult("combo-b", map[string]string{"lookback": "20"}),
				makeNoScoreResult("combo-c", map[string]string{"lookback": "30"}),
			}

			opt := optimize.New(trainTestSplits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.Warning).NotTo(BeEmpty(),
				"expected a warning when every combo produces NaN scores")
			Expect(rptData.Warning).To(ContainSubstring("undefined"))
		})

		It("does not emit a warning when combos produce different scores", func() {
			dates := makeDailyDates(
				time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
			)

			results := []study.RunResult{
				makeResult("combo-a", 0, map[string]string{"lookback": "10"},
					buildAccountFromEquity(dates, makeLinearEquity(dates, 10_000, 25_000))),
				makeResult("combo-b", 0, map[string]string{"lookback": "20"},
					buildAccountFromEquity(dates, makeLinearEquity(dates, 10_000, 12_000))),
				makeResult("combo-c", 0, map[string]string{"lookback": "30"},
					buildAccountFromEquity(dates, makeLinearEquity(dates, 10_000, 10_500))),
			}

			opt := optimize.New(trainTestSplits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.Warning).To(BeEmpty(),
				"did not expect a warning when combos produce distinct CAGR scores")
		})

		It("does not emit a warning for a single combo", func() {
			dates := makeDailyDates(
				time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
			)

			results := []study.RunResult{
				makeResult("combo-a", 0, map[string]string{"lookback": "10"},
					buildAccountFromEquity(dates, makeLinearEquity(dates, 10_000, 10_000))),
			}

			opt := optimize.New(trainTestSplits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.Warning).To(BeEmpty(),
				"single-combo runs are trivially degenerate and should not warn")
		})
	})

	Describe("Text rendering", func() {
		var trainTestSplits []study.Split

		BeforeEach(func() {
			start := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
			cutoff := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)

			built, err := study.TrainTest(start, cutoff, end)
			Expect(err).NotTo(HaveOccurred())
			trainTestSplits = built
		})

		It("produces a tabular header, rows, and best block", func() {
			dates := makeDailyDates(
				time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
			)

			results := []study.RunResult{
				makeResult("combo-a", 0, map[string]string{"lookback": "10"},
					buildAccountFromEquity(dates, makeLinearEquity(dates, 10_000, 25_000))),
				makeResult("combo-b", 0, map[string]string{"lookback": "30"},
					buildAccountFromEquity(dates, makeLinearEquity(dates, 10_000, 12_000))),
			}

			opt := optimize.New(trainTestSplits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			textReport, ok := rpt.(interface{ Text(io.Writer) error })
			Expect(ok).To(BeTrue(), "report must implement Text(io.Writer) error")

			var buf bytes.Buffer
			Expect(textReport.Text(&buf)).To(Succeed())
			out := buf.String()

			Expect(out).To(ContainSubstring("Optimization: CAGR (2 combos)"))
			Expect(out).To(ContainSubstring("Rank"))
			Expect(out).To(ContainSubstring("Parameters"))
			Expect(out).To(ContainSubstring("OOS"))
			Expect(out).To(ContainSubstring("lookback=10"))
			Expect(out).To(ContainSubstring("lookback=30"))
			Expect(out).To(ContainSubstring("Best:"))
		})

		It("includes the warning text when the ranking is degenerate", func() {
			dates := makeDailyDates(
				time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
			)

			flat := makeLinearEquity(dates, 10_000, 10_000)

			var results []study.RunResult
			for idx := range 3 {
				results = append(results, makeResult(
					fmt.Sprintf("combo-%d", idx),
					0,
					map[string]string{"lookback": fmt.Sprintf("%d", 10+idx*5)},
					buildAccountFromEquity(dates, flat),
				))
			}

			opt := optimize.New(trainTestSplits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			textReport := rpt.(interface{ Text(io.Writer) error })

			var buf bytes.Buffer
			Expect(textReport.Text(&buf)).To(Succeed())

			Expect(buf.String()).To(ContainSubstring("Warning:"))
			Expect(buf.String()).To(ContainSubstring("insertion order"))
		})

		It("renders NaN scores as n/a", func() {
			// Out-of-range _split_index leaves every score at NaN.
			acct := buildAccountFromEquity(
				makeDailyDates(
					time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
				),
				makeLinearEquity(
					makeDailyDates(
						time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
						time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
					),
					10_000, 12_000,
				),
			)

			results := []study.RunResult{
				{
					Config: study.RunConfig{
						Name: "orphan",
						Metadata: map[string]string{
							"_combination_id": "orphan",
							"_split_index":    "9999",
						},
					},
					Portfolio: acct,
				},
			}

			opt := optimize.New(trainTestSplits, optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			textReport := rpt.(interface{ Text(io.Writer) error })

			var buf bytes.Buffer
			Expect(textReport.Text(&buf)).To(Succeed())

			Expect(buf.String()).To(ContainSubstring("n/a"))
		})
	})

	Describe("with MaxDrawdown objective", func() {
		It("ranks lower (less negative) drawdown first", func() {
			dates := makeDailyDates(
				time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
			)

			// Combo A: V-shape equity (large drawdown, bad).
			equityA := makeVShapeEquity(dates, 10000, 5000, 10000)
			acctA := buildAccountFromEquity(dates, equityA)

			// Combo B: linear growth (no drawdown, good).
			equityB := makeLinearEquity(dates, 10000, 12000)
			acctB := buildAccountFromEquity(dates, equityB)

			paramsA := map[string]string{"lookback": "20"}
			paramsB := map[string]string{"lookback": "50"}

			results := []study.RunResult{
				makeResult("combo-a", 0, paramsA, acctA),
				makeResult("combo-a", 1, paramsA, acctA),
				makeResult("combo-b", 0, paramsB, acctB),
				makeResult("combo-b", 1, paramsB, acctB),
			}

			opt := optimize.New(splits, optimize.WithObjective(portfolio.MaxDrawdown.(portfolio.Rankable)))
			rpt, err := opt.Analyze(results)
			Expect(err).NotTo(HaveOccurred())

			rptData := decodeOptReport(rpt)
			Expect(rptData.Rankings).To(HaveLen(2))

			// Combo B (no drawdown, score=0) should rank above combo A
			// (large negative drawdown) since higher is better.
			Expect(rptData.Rankings[0].Parameters).To(ContainSubstring("lookback=50"))
		})
	})
})

// makeDailyDates creates a slice of daily dates from start (inclusive)
// to end (exclusive).
func makeDailyDates(start, end time.Time) []time.Time {
	var dates []time.Time

	current := start
	for current.Before(end) {
		dates = append(dates, current)
		current = current.AddDate(0, 0, 1)
	}

	return dates
}

// makeLinearEquity creates an equity curve that grows linearly from
// startVal to endVal over the given number of dates.
func makeLinearEquity(dates []time.Time, startVal, endVal float64) []float64 {
	count := len(dates)
	values := make([]float64, count)

	for idx := range dates {
		fraction := float64(idx) / float64(count-1)
		values[idx] = startVal + fraction*(endVal-startVal)
	}

	return values
}

// makeVShapeEquity creates an equity curve that drops from startVal to
// troughVal at the midpoint then recovers to endVal.
func makeVShapeEquity(dates []time.Time, startVal, troughVal, endVal float64) []float64 {
	count := len(dates)
	mid := count / 2
	values := make([]float64, count)

	for idx := range mid {
		fraction := float64(idx) / float64(mid)
		values[idx] = startVal + fraction*(troughVal-startVal)
	}

	for idx := mid; idx < count; idx++ {
		fraction := float64(idx-mid) / float64(count-1-mid)
		values[idx] = troughVal + fraction*(endVal-troughVal)
	}

	return values
}
