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

// Package summary builds backtest summary reports from a portfolio.
package summary

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study/report"
)

// portfolioAsset mirrors the unexported var in portfolio/account.go so that
// we can look up columns in the perfData DataFrame.
var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}

// Build constructs a Report from the given portfolio. The report's
// Sections are the domain types (Header, EquityCurve, ReturnTable, etc.)
// which implement Section for text/JSON rendering and can be type-asserted
// by the terminal renderer for styled output.
func Build(acct report.ReportablePortfolio) (report.Report, error) {
	var warnings []string

	perfData := acct.PerfData()

	header := buildHeaderData(acct)
	hasBenchmark := acct.Benchmark() != (asset.Asset{})

	if perfData != nil && perfData.Len() > 0 {
		header.StartDate = perfData.Start()
		header.EndDate = perfData.End()
		header.Steps = perfData.Len()
	}

	// Early exit for insufficient data.
	if perfData == nil || perfData.Len() < 2 {
		warnings = append(warnings, "insufficient data for full report")

		return report.Report{
			Title:        header.StrategyName,
			Sections:     []report.Section{&header},
			HasBenchmark: hasBenchmark,
			Warnings:     warnings,
		}, nil
	}

	equityCurve := buildEquityCurve(perfData, header.InitialCash)

	recentReturns := buildRecentReturns(acct, hasBenchmark, &warnings)
	recentReturns.SectionName = "Recent Returns"

	returns := buildReturns(acct, hasBenchmark, &warnings)
	returns.SectionName = "Returns"

	annualReturns := buildAnnualReturns(acct, hasBenchmark, &warnings)

	risk := buildRisk(acct, hasBenchmark, &warnings)
	risk.HasBenchmark = hasBenchmark

	drawdowns := buildDrawdowns(acct, &warnings)
	monthlyReturns := buildMonthlyReturns(acct, &warnings)
	trades := buildTrades(acct, &warnings)

	sections := []report.Section{
		&header,
		&equityCurve,
		&recentReturns,
		&returns,
		&annualReturns,
		&risk,
	}

	var riskVsBenchmark report.RiskVsBenchmark
	if hasBenchmark {
		riskVsBenchmark = buildRiskVsBenchmark(acct, &warnings)
		sections = append(sections, &riskVsBenchmark)
	}

	sections = append(sections, &drawdowns, &monthlyReturns, &trades)

	return report.Report{
		Title:        header.StrategyName,
		Sections:     sections,
		HasBenchmark: hasBenchmark,
		Warnings:     warnings,
	}, nil
}

// ---------------------------------------------------------------------------
// Header builder
// ---------------------------------------------------------------------------

func buildHeaderData(acct report.ReportablePortfolio) report.Header {
	header := report.Header{
		StrategyName:    acct.GetMetadata(portfolio.MetaStrategyName),
		StrategyVersion: acct.GetMetadata(portfolio.MetaStrategyVersion),
		Benchmark:       acct.GetMetadata(portfolio.MetaStrategyBenchmark),
		FinalValue:      acct.Value(),
	}

	if cashStr := acct.GetMetadata(portfolio.MetaRunInitialCash); cashStr != "" {
		if parsed, parseErr := strconv.ParseFloat(cashStr, 64); parseErr == nil {
			header.InitialCash = parsed
		}
	}

	if elapsedStr := acct.GetMetadata(portfolio.MetaRunElapsed); elapsedStr != "" {
		if parsed, parseErr := time.ParseDuration(elapsedStr); parseErr == nil {
			header.Elapsed = parsed
		}
	}

	return header
}

// ---------------------------------------------------------------------------
// Section builders
// ---------------------------------------------------------------------------

func buildEquityCurve(perfData *data.DataFrame, initialCash float64) report.EquityCurve {
	times := perfData.Times()
	strategyValues := perfData.Column(portfolioAsset, data.PortfolioEquity)
	benchmarkRaw := perfData.Column(portfolioAsset, data.PortfolioBenchmark)

	stratCopy := make([]float64, len(strategyValues))
	copy(stratCopy, strategyValues)

	var benchNorm []float64
	if len(benchmarkRaw) > 0 && benchmarkRaw[0] != 0 {
		benchNorm = make([]float64, len(benchmarkRaw))

		scale := initialCash / benchmarkRaw[0]
		for idx, val := range benchmarkRaw {
			benchNorm[idx] = val * scale
		}
	}

	timesCopy := make([]time.Time, len(times))
	copy(timesCopy, times)

	return report.EquityCurve{
		Times:           timesCopy,
		StrategyValues:  stratCopy,
		BenchmarkValues: benchNorm,
	}
}

func buildRecentReturns(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) report.ReturnTable {
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

	result := report.ReturnTable{
		Periods:   make([]string, len(defs)),
		Strategy:  make([]float64, len(defs)),
		Benchmark: make([]float64, len(defs)),
	}

	if pd != nil && pd.Len() > 0 {
		result.AsOf = pd.End()
	}

	for idx, def := range defs {
		result.Periods[idx] = def.label
		result.Strategy[idx] = metricValWindow(acct, portfolio.TWRR, def.window, warnings)

		if hasBenchmark {
			result.Benchmark[idx] = metricValBenchmarkWindow(acct, portfolio.TWRR, def.window, warnings)
		} else {
			result.Benchmark[idx] = math.NaN()
		}
	}

	return result
}

func buildReturns(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) report.ReturnTable {
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

	result := report.ReturnTable{
		AsOf:      backtestEnd,
		Periods:   make([]string, len(defs)),
		Strategy:  make([]float64, len(defs)),
		Benchmark: make([]float64, len(defs)),
	}

	for idx, def := range defs {
		result.Periods[idx] = def.label

		if def.window != nil {
			windowStart := def.window.Before(backtestEnd)
			if windowStart.Before(backtestStart) {
				result.Strategy[idx] = math.NaN()
				result.Benchmark[idx] = math.NaN()

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
			result.Strategy[idx] = stratTWRR
			result.Benchmark[idx] = benchTWRR
		} else {
			result.Strategy[idx] = annualizeTWRR(stratTWRR, years)
			result.Benchmark[idx] = annualizeTWRR(benchTWRR, years)
		}
	}

	return result
}

func buildAnnualReturns(acct report.ReportablePortfolio, hasBenchmark bool, warnings *[]string) report.AnnualReturns {
	years, stratReturns, err := acct.AnnualReturns(data.PortfolioEquity)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("annual returns (strategy): %v", err))
		return report.AnnualReturns{}
	}

	result := report.AnnualReturns{
		Years:    years,
		Strategy: stratReturns,
	}

	if hasBenchmark {
		_, benchReturns, err := acct.AnnualReturns(data.PortfolioBenchmark)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("annual returns (benchmark): %v", err))
		} else {
			result.Benchmark = benchReturns
		}
	}

	return result
}

func buildRisk(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) report.Risk {
	risk := report.Risk{}

	type riskField struct {
		target *[2]float64
		metric portfolio.PerformanceMetric
	}

	fields := []riskField{
		{&risk.MaxDrawdown, portfolio.MaxDrawdown},
		{&risk.Volatility, portfolio.StdDev},
		{&risk.DownsideDeviation, portfolio.DownsideDeviation},
		{&risk.Sharpe, portfolio.Sharpe},
		{&risk.Sortino, portfolio.Sortino},
		{&risk.Calmar, portfolio.Calmar},
		{&risk.UlcerIndex, portfolio.UlcerIndex},
		{&risk.ValueAtRisk, portfolio.ValueAtRisk},
		{&risk.Skewness, portfolio.Skewness},
		{&risk.ExcessKurtosis, portfolio.ExcessKurtosis},
	}

	for _, field := range fields {
		field.target[0] = metricVal(acct, field.metric, warnings)
		if hasBenchmark {
			field.target[1] = metricValBenchmark(acct, field.metric, warnings)
		} else {
			field.target[1] = math.NaN()
		}
	}

	return risk
}

func buildRiskVsBenchmark(acct portfolio.Portfolio, warnings *[]string) report.RiskVsBenchmark {
	return report.RiskVsBenchmark{
		Beta:             metricVal(acct, portfolio.Beta, warnings),
		Alpha:            metricVal(acct, portfolio.Alpha, warnings),
		RSquared:         metricVal(acct, portfolio.RSquared, warnings),
		TrackingError:    metricVal(acct, portfolio.TrackingError, warnings),
		InformationRatio: metricVal(acct, portfolio.InformationRatio, warnings),
		Treynor:          metricVal(acct, portfolio.Treynor, warnings),
		UpsideCapture:    metricVal(acct, portfolio.UpsideCaptureRatio, warnings),
		DownsideCapture:  metricVal(acct, portfolio.DownsideCaptureRatio, warnings),
	}
}

func buildDrawdowns(acct report.ReportablePortfolio, warnings *[]string) report.Drawdowns {
	details, err := acct.DrawdownDetails(5)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("drawdown details: %v", err))
		return report.Drawdowns{}
	}

	entries := make([]report.DrawdownEntry, len(details))
	for idx, detail := range details {
		entries[idx] = report.DrawdownEntry{
			Start:    detail.Start,
			End:      detail.Trough,
			Recovery: detail.Recovery,
			Depth:    detail.Depth,
			Days:     detail.Days,
		}
	}

	return report.Drawdowns{Entries: entries}
}

func buildMonthlyReturns(acct report.ReportablePortfolio, warnings *[]string) report.MonthlyReturns {
	years, grid, err := acct.MonthlyReturns(data.PortfolioEquity)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("monthly returns: %v", err))
		return report.MonthlyReturns{}
	}

	return report.MonthlyReturns{
		Years:  years,
		Values: grid,
	}
}

func buildTrades(acct portfolio.Portfolio, warnings *[]string) report.Trades {
	transactions := acct.Transactions()

	var entries []report.TradeEntry

	totalTransactions := 0
	roundTrips := 0

	for _, txn := range transactions {
		switch txn.Type {
		case asset.BuyTransaction, asset.SellTransaction:
			totalTransactions++

			if txn.Type == asset.SellTransaction {
				roundTrips++
			}

			entries = append(entries, report.TradeEntry{
				Date:   txn.Date,
				Action: txn.Type.String(),
				Ticker: txn.Asset.Ticker,
				Shares: txn.Qty,
				Price:  txn.Price,
				Amount: txn.Amount,
			})
		}
	}

	result := report.Trades{
		TotalTransactions: totalTransactions,
		RoundTrips:        roundTrips,
		Trades:            entries,
	}

	tradeMetrics, err := acct.TradeMetrics()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("trade metrics: %v", err))
	} else {
		result.WinRate = tradeMetrics.WinRate
		result.AvgHolding = tradeMetrics.AverageHoldingPeriod
		result.AvgWin = tradeMetrics.AverageWin
		result.AvgLoss = tradeMetrics.AverageLoss
		result.ProfitFactor = tradeMetrics.ProfitFactor
		result.GainLossRatio = tradeMetrics.GainLossRatio
		result.Turnover = tradeMetrics.Turnover
		result.PositivePeriods = tradeMetrics.NPositivePeriods
	}

	return result
}

// ---------------------------------------------------------------------------
// Metric helpers
// ---------------------------------------------------------------------------

func metricVal(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s: %v", metric.Name(), err))
		return math.NaN()
	}

	return val
}

func metricValBenchmark(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Benchmark().Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s (benchmark): %v", metric.Name(), err))
		return math.NaN()
	}

	return val
}

func metricValWindow(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, window portfolio.Period, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Window(window).Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s (window %v): %v", metric.Name(), window, err))
		return math.NaN()
	}

	return val
}

func metricValBenchmarkWindow(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, window portfolio.Period, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Benchmark().Window(window).Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s (benchmark, window %v): %v", metric.Name(), window, err))
		return math.NaN()
	}

	return val
}

func annualizeTWRR(twrr float64, years float64) float64 {
	if math.IsNaN(twrr) || years <= 0 {
		return math.NaN()
	}

	return math.Pow(1+twrr, 1.0/years) - 1
}
