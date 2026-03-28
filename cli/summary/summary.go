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

// Package summary builds and renders backtest summary reports to a terminal.
package summary

import (
	"io"
	"strings"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

// ReportablePortfolio is the interface required by the summary renderer.
// It composes read-only portfolio access with statistical queries needed
// for the full report.
type ReportablePortfolio interface {
	portfolio.Portfolio
	portfolio.PortfolioStats
}

// Render builds a complete backtest summary report from acct and writes
// the lipgloss-styled output to writer.
func Render(acct ReportablePortfolio, writer io.Writer) error {
	var warnings []string

	perfData := acct.PerfData()

	hdr := buildHeader(acct)
	hasBenchmark := acct.Benchmark() != (asset.Asset{})

	if perfData != nil && perfData.Len() > 0 {
		hdr.startDate = perfData.Start()
		hdr.endDate = perfData.End()
		hdr.steps = perfData.Len()
	}

	var sb strings.Builder

	// Early exit for insufficient data.
	if perfData == nil || perfData.Len() < 2 {
		warnings = append(warnings, "insufficient data for full report")

		renderHeader(&sb, hdr)

		if len(warnings) > 0 {
			renderWarnings(&sb, warnings)
		}

		sb.WriteString("\n")

		_, err := io.WriteString(writer, sb.String())

		return err
	}

	ec := buildEquityCurve(perfData, hdr.initialCash)

	recentRet := buildRecentReturns(acct, hasBenchmark, &warnings)
	recentRet.sectionName = "Recent Returns"

	ret := buildReturns(acct, hasBenchmark, &warnings)
	ret.sectionName = "Returns"

	annRet := buildAnnualReturns(acct, hasBenchmark, &warnings)

	rsk := buildRisk(acct, hasBenchmark, &warnings)
	rsk.hasBenchmark = hasBenchmark

	dd := buildDrawdowns(acct, &warnings)
	mr := buildMonthlyReturns(acct, &warnings)
	tr := buildTrades(acct, &warnings)

	renderHeader(&sb, hdr)
	renderEquityCurve(&sb, ec, hasBenchmark)
	renderRecentReturns(&sb, recentRet, hasBenchmark)
	renderReturns(&sb, ret, hasBenchmark)
	renderAnnualReturns(&sb, annRet, hasBenchmark)
	renderRisk(&sb, rsk, hasBenchmark)

	if hasBenchmark {
		rvb := buildRiskVsBenchmark(acct, &warnings)
		renderRiskVsBenchmark(&sb, rvb)
	}

	renderDrawdowns(&sb, dd)
	renderMonthlyReturns(&sb, mr)
	renderTrades(&sb, tr)

	if len(warnings) > 0 {
		renderWarnings(&sb, warnings)
	}

	sb.WriteString("\n")

	_, err := io.WriteString(writer, sb.String())

	return err
}
