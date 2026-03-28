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

package summary

import (
	"fmt"
	"math"
	"strings"

	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

const colWidth = 16

func buildRecentReturns(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) returnTable {
	oneDay := portfolio.Days(1)
	oneWeek := portfolio.Days(7)
	oneMonth := portfolio.Months(1)
	wtd := portfolio.WTD()
	mtd := portfolio.MTD()
	ytd := portfolio.YTD()

	type periodDef struct {
		label  string
		window portfolio.Period
	}

	defs := []periodDef{
		{"1D", oneDay},
		{"1W", oneWeek},
		{"1M", oneMonth},
		{"WTD", wtd},
		{"MTD", mtd},
		{"YTD", ytd},
	}

	pd := acct.PerfData()

	result := returnTable{
		periods:   make([]string, len(defs)),
		strategy:  make([]float64, len(defs)),
		benchmark: make([]float64, len(defs)),
	}

	if pd != nil && pd.Len() > 0 {
		result.asOf = pd.End()
	}

	for idx, def := range defs {
		result.periods[idx] = def.label
		result.strategy[idx] = metricValWindow(acct, portfolio.TWRR, def.window, warnings)

		if hasBenchmark {
			result.benchmark[idx] = metricValBenchmarkWindow(acct, portfolio.TWRR, def.window, warnings)
		} else {
			result.benchmark[idx] = math.NaN()
		}
	}

	return result
}

func buildReturns(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) returnTable {
	perfData := acct.PerfData()

	var backtestStart, backtestEnd time.Time

	if perfData != nil && perfData.Len() > 0 {
		backtestStart = perfData.Start()
		backtestEnd = perfData.End()
	}

	backtestYears := backtestEnd.Sub(backtestStart).Hours() / 24 / 365.25

	type periodDef struct {
		label        string
		window       *portfolio.Period
		nominalYears float64
	}

	oneYear := portfolio.Years(1)
	threeYears := portfolio.Years(3)
	fiveYears := portfolio.Years(5)
	tenYears := portfolio.Years(10)

	defs := []periodDef{
		{"1Y", &oneYear, 1},
		{"3Y", &threeYears, 3},
		{"5Y", &fiveYears, 5},
		{"10Y", &tenYears, 10},
		{"Since Inception", nil, 0},
	}

	result := returnTable{
		asOf:      backtestEnd,
		periods:   make([]string, len(defs)),
		strategy:  make([]float64, len(defs)),
		benchmark: make([]float64, len(defs)),
	}

	for idx, def := range defs {
		result.periods[idx] = def.label

		if def.window != nil {
			windowStart := def.window.Before(backtestEnd)
			if windowStart.Before(backtestStart) {
				result.strategy[idx] = math.NaN()
				result.benchmark[idx] = math.NaN()

				continue
			}
		}

		var stratTWRR, benchTWRR float64

		if def.window != nil {
			stratTWRR = metricValWindow(acct, portfolio.TWRR, *def.window, warnings)
		} else {
			stratTWRR = metricVal(acct, portfolio.TWRR, warnings)
		}

		if hasBenchmark {
			if def.window != nil {
				benchTWRR = metricValBenchmarkWindow(acct, portfolio.TWRR, *def.window, warnings)
			} else {
				benchTWRR = metricValBenchmark(acct, portfolio.TWRR, warnings)
			}
		} else {
			benchTWRR = math.NaN()
		}

		years := def.nominalYears
		if years == 0 {
			years = backtestYears
		}

		if def.window == nil && backtestYears < 1.0 {
			result.strategy[idx] = stratTWRR
			result.benchmark[idx] = benchTWRR
		} else {
			result.strategy[idx] = annualizeTWRR(stratTWRR, years)
			result.benchmark[idx] = annualizeTWRR(benchTWRR, years)
		}
	}

	return result
}

func buildAnnualReturns(acct ReportablePortfolio, hasBenchmark bool, warnings *[]string) annualReturns {
	years, stratReturns, err := acct.AnnualReturns(data.PortfolioEquity)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("annual returns (strategy): %v", err))
		return annualReturns{}
	}

	result := annualReturns{
		years:    years,
		strategy: stratReturns,
	}

	if hasBenchmark {
		_, benchReturns, benchErr := acct.AnnualReturns(data.PortfolioBenchmark)
		if benchErr != nil {
			*warnings = append(*warnings, fmt.Sprintf("annual returns (benchmark): %v", benchErr))
		} else {
			result.benchmark = benchReturns
		}
	}

	return result
}

func annualizeTWRR(twrr float64, years float64) float64 {
	if math.IsNaN(twrr) || years <= 0 {
		return math.NaN()
	}

	return math.Pow(1+twrr, 1.0/years) - 1
}

func renderRecentReturns(builder *strings.Builder, table returnTable, hasBenchmark bool) {
	title := "Recent Returns"
	if !table.asOf.IsZero() {
		title = fmt.Sprintf("Recent Returns (as of %s)", table.asOf.Format("2006-01-02"))
	}

	renderReturnTable(title, builder, table, hasBenchmark)
}

func renderReturns(builder *strings.Builder, table returnTable, hasBenchmark bool) {
	renderReturnTable("Returns", builder, table, hasBenchmark)
}

func renderReturnTable(title string, builder *strings.Builder, table returnTable, hasBenchmark bool) {
	if len(table.periods) == 0 {
		return
	}

	builder.WriteString(sectionTitleStyle.Render(title))
	builder.WriteString("\n")

	hdr := padRight(labelStyle.Render(""), colWidth)
	for _, period := range table.periods {
		hdr += padLeft(tableHeaderStyle.Render(period), colWidth)
	}

	builder.WriteString("  " + hdr + "\n")

	stratRow := padRight(labelStyle.Render("Strategy"), colWidth)
	for _, val := range table.strategy {
		stratRow += padLeft(fmtPct(val), colWidth)
	}

	builder.WriteString("  " + stratRow + "\n")

	if hasBenchmark {
		benchRow := padRight(labelStyle.Render("Benchmark"), colWidth)
		for _, val := range table.benchmark {
			benchRow += padLeft(fmtPct(val), colWidth)
		}

		builder.WriteString("  " + benchRow + "\n")

		diffRow := padRight(labelStyle.Render("+/-"), colWidth)

		for idx := range table.strategy {
			diff := table.strategy[idx] - table.benchmark[idx]
			if math.IsNaN(table.strategy[idx]) || math.IsNaN(table.benchmark[idx]) {
				diff = math.NaN()
			}

			diffRow += padLeft(fmtPctDiff(diff), colWidth)
		}

		builder.WriteString("  " + diffRow + "\n")
	}
}

// renderAnnualReturns writes the annual returns table with years as rows
// so that it remains readable regardless of how many years are present.
func renderAnnualReturns(builder *strings.Builder, annual annualReturns, hasBenchmark bool) {
	if len(annual.years) == 0 {
		return
	}

	builder.WriteString(sectionTitleStyle.Render("Annual Returns"))
	builder.WriteString("\n")

	// Column header row.
	hdr := padRight(labelStyle.Render(""), colWidth)
	hdr += padLeft(tableHeaderStyle.Render("Strategy"), colWidth)

	if hasBenchmark && len(annual.benchmark) > 0 {
		hdr += padLeft(tableHeaderStyle.Render("Benchmark"), colWidth)
		hdr += padLeft(tableHeaderStyle.Render("+/-"), colWidth)
	}

	builder.WriteString("  " + hdr + "\n")

	// One row per year.
	for idx, year := range annual.years {
		row := padRight(tableHeaderStyle.Render(fmt.Sprintf("%d", year)), colWidth)

		stratVal := math.NaN()
		if idx < len(annual.strategy) {
			stratVal = annual.strategy[idx]
		}

		row += padLeft(fmtPct(stratVal), colWidth)

		if hasBenchmark && len(annual.benchmark) > 0 {
			benchVal := math.NaN()
			if idx < len(annual.benchmark) {
				benchVal = annual.benchmark[idx]
			}

			row += padLeft(fmtPct(benchVal), colWidth)

			diff := math.NaN()
			if !math.IsNaN(stratVal) && !math.IsNaN(benchVal) {
				diff = stratVal - benchVal
			}

			row += padLeft(fmtPctDiff(diff), colWidth)
		}

		builder.WriteString("  " + row + "\n")
	}
}
