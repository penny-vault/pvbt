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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/optimize"
)

var _ = Describe("Integration", func() {
	It("produces a complete optimization report from multiple combos and splits", func() {
		// Build a TrainTest split covering 2020.
		start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		cutoff := time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

		splits, err := study.TrainTest(start, cutoff, end)
		Expect(err).NotTo(HaveOccurred())
		Expect(splits).To(HaveLen(1))

		// Build daily equity dates spanning the full range.
		dates := makeDailyDates(start, end)

		// Combo A: strong linear growth (best performer).
		equityA := makeLinearEquity(dates, 10_000, 25_000)
		acctA := buildAccountFromEquity(dates, equityA)

		// Combo B: modest growth.
		equityB := makeLinearEquity(dates, 10_000, 13_000)
		acctB := buildAccountFromEquity(dates, equityB)

		// Combo C: near-flat equity (worst performer).
		equityC := makeLinearEquity(dates, 10_000, 10_100)
		acctC := buildAccountFromEquity(dates, equityC)

		paramsA := map[string]string{"lookback": "10"}
		paramsB := map[string]string{"lookback": "30"}
		paramsC := map[string]string{"lookback": "60"}

		// Build one RunResult per combination for split index 0.
		results := []study.RunResult{
			makeResult("combo-a", 0, paramsA, acctA),
			makeResult("combo-b", 0, paramsB, acctB),
			makeResult("combo-c", 0, paramsC, acctC),
		}

		opt := optimize.New(splits, optimize.WithObjective(study.MetricCAGR))
		rpt, analyzeErr := opt.Analyze(results)

		Expect(analyzeErr).NotTo(HaveOccurred())
		Expect(rpt.Title).NotTo(BeEmpty())

		// The report must contain at least four sections.
		Expect(rpt.Sections).To(HaveLen(4))

		// Verify Rankings table: 3 combos, best first.
		rankingsTable := findTable(rpt, "Rankings")
		Expect(rankingsTable).NotTo(BeNil(), "expected a Rankings table section")
		Expect(rankingsTable.Rows).To(HaveLen(3))

		// First row should be rank 1 and identify combo A (lookback=10).
		firstRank, ok := rankingsTable.Rows[0][0].(int)
		Expect(ok).To(BeTrue())
		Expect(firstRank).To(Equal(1))

		firstParams, paramOK := rankingsTable.Rows[0][1].(string)
		Expect(paramOK).To(BeTrue())
		Expect(firstParams).To(ContainSubstring("lookback=10"))

		// Verify Best Combination Detail table exists.
		detailTable := findTable(rpt, "Best Combination")
		Expect(detailTable).NotTo(BeNil(), "expected a Best Combination Detail table section")
		Expect(detailTable.Rows).To(HaveLen(len(splits)))

		// Verify Overfitting table exists with one row per combo.
		overfitTable := findTable(rpt, "Overfitting")
		Expect(overfitTable).NotTo(BeNil(), "expected an Overfitting Check table section")
		Expect(overfitTable.Rows).To(HaveLen(3))

		// Verify equity curves time_series section is present.
		var tsCount int
		for _, section := range rpt.Sections {
			if section.Type() == "time_series" {
				tsCount++
			}
		}

		Expect(tsCount).To(Equal(1), "expected exactly one time_series section")
	})

	It("ranks parameter combinations correctly across two TrainTest splits", func() {
		// Build two sequential TrainTest splits to verify multi-split aggregation.
		firstStart := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
		firstCutoff := time.Date(2019, 7, 1, 0, 0, 0, 0, time.UTC)
		firstEnd := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

		secondStart := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		secondCutoff := time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC)
		secondEnd := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

		splits1, err := study.TrainTest(firstStart, firstCutoff, firstEnd)
		Expect(err).NotTo(HaveOccurred())

		splits2, err := study.TrainTest(secondStart, secondCutoff, secondEnd)
		Expect(err).NotTo(HaveOccurred())

		// Combine both splits into a two-split slice.
		allSplits := append(splits1, splits2...)
		Expect(allSplits).To(HaveLen(2))

		dates1 := makeDailyDates(firstStart, firstEnd)
		dates2 := makeDailyDates(secondStart, secondEnd)

		// Combo A wins on both splits.
		acctA1 := buildAccountFromEquity(dates1, makeLinearEquity(dates1, 10_000, 20_000))
		acctA2 := buildAccountFromEquity(dates2, makeLinearEquity(dates2, 20_000, 40_000))

		// Combo B loses on both splits.
		acctB1 := buildAccountFromEquity(dates1, makeLinearEquity(dates1, 10_000, 10_050))
		acctB2 := buildAccountFromEquity(dates2, makeLinearEquity(dates2, 10_050, 10_100))

		paramsA := map[string]string{"window": "5"}
		paramsB := map[string]string{"window": "200"}

		results := []study.RunResult{
			makeResult("combo-a", 0, paramsA, acctA1),
			makeResult("combo-a", 1, paramsA, acctA2),
			makeResult("combo-b", 0, paramsB, acctB1),
			makeResult("combo-b", 1, paramsB, acctB2),
		}

		opt := optimize.New(allSplits, optimize.WithObjective(study.MetricCAGR))
		rpt, analyzeErr := opt.Analyze(results)

		Expect(analyzeErr).NotTo(HaveOccurred())

		rankingsTable := findTable(rpt, "Rankings")
		Expect(rankingsTable).NotTo(BeNil())
		Expect(rankingsTable.Rows).To(HaveLen(2))

		// Combo A (window=5) must rank first.
		topParams, paramOK := rankingsTable.Rows[0][1].(string)
		Expect(paramOK).To(BeTrue())
		Expect(topParams).To(ContainSubstring("window=5"))
	})
})
