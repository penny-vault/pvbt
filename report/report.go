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

package report

import (
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// portfolioAsset mirrors the unexported var in portfolio/account.go so that
// we can look up columns in the perfData DataFrame.
var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}

// ---------------------------------------------------------------------------
// View model types -- plain structs, no behavior.
// ---------------------------------------------------------------------------

// Report is the top-level view model for a backtest report.
type Report struct {
	Header          Header
	HasBenchmark    bool
	EquityCurve     EquityCurve
	RecentReturns   ReturnTable
	Returns         ReturnTable
	AnnualReturns   AnnualReturns
	Risk            Risk
	RiskVsBenchmark RiskVsBenchmark
	Drawdowns       Drawdowns
	MonthlyReturns  MonthlyReturns
	Trades          Trades
	Warnings        []string
}

// RunMeta carries metadata about the backtest run itself.
type RunMeta struct {
	Elapsed     time.Duration
	Steps       int
	InitialCash float64
}

// Header contains the headline information for the report.
type Header struct {
	StrategyName    string
	StrategyVersion string
	Benchmark       string
	StartDate       time.Time
	EndDate         time.Time
	InitialCash     float64
	FinalValue      float64
	Elapsed         time.Duration
	Steps           int
}

// EquityCurve holds the time series for the strategy and benchmark equity.
type EquityCurve struct {
	Times           []time.Time
	StrategyValues  []float64
	BenchmarkValues []float64 // normalized to InitialCash
}

// ReturnTable holds return figures for named periods.
type ReturnTable struct {
	AsOf      time.Time
	Periods   []string
	Strategy  []float64
	Benchmark []float64
}

// AnnualReturns holds year-by-year return figures.
type AnnualReturns struct {
	Years     []int
	Strategy  []float64
	Benchmark []float64
}

// Risk holds paired (strategy, benchmark) risk metrics.
// Each [2]float64 is {Strategy, Benchmark}.
type Risk struct {
	MaxDrawdown       [2]float64
	Volatility        [2]float64
	DownsideDeviation [2]float64
	Sharpe            [2]float64
	Sortino           [2]float64
	Calmar            [2]float64
	UlcerIndex        [2]float64
	ValueAtRisk       [2]float64
	Skewness          [2]float64
	ExcessKurtosis    [2]float64
}

// RiskVsBenchmark holds relative-risk metrics that only make sense when
// a benchmark is configured.
type RiskVsBenchmark struct {
	Beta             float64
	Alpha            float64
	RSquared         float64
	TrackingError    float64
	InformationRatio float64
	Treynor          float64
	UpsideCapture    float64
	DownsideCapture  float64
}

// DrawdownEntry describes a single drawdown episode.
type DrawdownEntry struct {
	Start    time.Time
	End      time.Time
	Recovery time.Time
	Depth    float64
	Days     int
}

// Drawdowns holds the top drawdown episodes.
type Drawdowns struct {
	Entries []DrawdownEntry
}

// MonthlyReturns holds a year x month grid of returns.
type MonthlyReturns struct {
	Years  []int
	Values [][]float64 // [yearIdx][monthIdx], NaN for missing
}

// TradeEntry describes a single trade.
type TradeEntry struct {
	Date   time.Time
	Action string
	Ticker string
	Shares float64
	Price  float64
	Amount float64
}

// Trades holds trade analysis data.
type Trades struct {
	TotalTransactions int
	RoundTrips        int
	WinRate           float64
	AvgHolding        float64
	AvgWin            float64
	AvgLoss           float64
	ProfitFactor      float64
	GainLossRatio     float64
	Turnover          float64
	PositivePeriods   float64
	Trades            []TradeEntry
}

// ---------------------------------------------------------------------------
// Build populates a Report from a portfolio, strategy info, and run metadata.
// It does no math itself -- it delegates to portfolio metric APIs and maps
// the results into the view model structs.
// ---------------------------------------------------------------------------

// Build constructs a Report view model from the given portfolio, strategy
// description, and run metadata.
func Build(acct portfolio.Portfolio, info engine.StrategyInfo, meta RunMeta) (Report, error) {
	var warnings []string

	perfData := acct.PerfData()

	// Header -- always populated.
	header := Header{
		StrategyName:    info.Name,
		StrategyVersion: info.Version,
		Benchmark:       info.Benchmark,
		InitialCash:     meta.InitialCash,
		FinalValue:      acct.Value(),
		Elapsed:         meta.Elapsed,
		Steps:           meta.Steps,
	}

	if perfData != nil && perfData.Len() > 0 {
		header.StartDate = perfData.Start()
		header.EndDate = perfData.End()
	}

	// Determine whether a benchmark is configured.
	hasBenchmark := false
	if concrete, ok := acct.(*portfolio.Account); ok {
		hasBenchmark = concrete.Benchmark() != (asset.Asset{})
	}

	// Early exit when there is insufficient data for a full report.
	if perfData == nil || perfData.Len() < 2 {
		warnings = append(warnings, "insufficient data for full report")

		return Report{
			Header:       header,
			HasBenchmark: hasBenchmark,
			Warnings:     warnings,
		}, nil
	}

	report := Report{
		Header:       header,
		HasBenchmark: hasBenchmark,
	}

	// Equity curve.
	report.EquityCurve = buildEquityCurve(perfData, meta.InitialCash)

	// Returns.
	report.RecentReturns = buildRecentReturns(acct, hasBenchmark, &warnings)
	report.Returns = buildReturns(acct, hasBenchmark, &warnings)

	// Annual returns.
	report.AnnualReturns = buildAnnualReturns(acct, hasBenchmark, &warnings)

	// Risk metrics (strategy and benchmark).
	report.Risk = buildRisk(acct, hasBenchmark, &warnings)

	// Risk vs benchmark.
	if hasBenchmark {
		report.RiskVsBenchmark = buildRiskVsBenchmark(acct, &warnings)
	}

	// Drawdowns.
	report.Drawdowns = buildDrawdowns(acct, &warnings)

	// Monthly returns.
	report.MonthlyReturns = buildMonthlyReturns(acct, &warnings)

	// Trades.
	report.Trades = buildTrades(acct, &warnings)

	report.Warnings = warnings

	return report, nil
}

// ---------------------------------------------------------------------------
// Section builders
// ---------------------------------------------------------------------------

func buildEquityCurve(perfData *data.DataFrame, initialCash float64) EquityCurve {
	times := perfData.Times()
	strategyValues := perfData.Column(portfolioAsset, data.PortfolioEquity)
	benchmarkRaw := perfData.Column(portfolioAsset, data.PortfolioBenchmark)

	// Copy strategy values.
	stratCopy := make([]float64, len(strategyValues))
	copy(stratCopy, strategyValues)

	// Normalize benchmark to initial cash.
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

	return EquityCurve{
		Times:           timesCopy,
		StrategyValues:  stratCopy,
		BenchmarkValues: benchNorm,
	}
}

func buildRecentReturns(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) ReturnTable {
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

	result := ReturnTable{
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

func buildReturns(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) ReturnTable {
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

	result := ReturnTable{
		AsOf:      backtestEnd,
		Periods:   make([]string, len(defs)),
		Strategy:  make([]float64, len(defs)),
		Benchmark: make([]float64, len(defs)),
	}

	for idx, def := range defs {
		result.Periods[idx] = def.label

		// N/A detection: check if the backtest covers the requested period.
		if def.window != nil {
			windowStart := def.window.Before(backtestEnd)
			if windowStart.Before(backtestStart) {
				result.Strategy[idx] = math.NaN()
				result.Benchmark[idx] = math.NaN()

				continue
			}
		}

		// Compute TWRR.
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

		// Annualize.
		years := def.nominalYears
		if years == 0 {
			years = backtestYears
		}

		// For backtests shorter than 1 year, Since Inception shows raw TWRR.
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

func buildAnnualReturns(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) AnnualReturns {
	concrete, ok := acct.(*portfolio.Account)
	if !ok {
		*warnings = append(*warnings, "annual returns require *portfolio.Account")
		return AnnualReturns{}
	}

	years, stratReturns, err := concrete.AnnualReturns(data.PortfolioEquity)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("annual returns (strategy): %v", err))
		return AnnualReturns{}
	}

	result := AnnualReturns{
		Years:    years,
		Strategy: stratReturns,
	}

	if hasBenchmark {
		_, benchReturns, err := concrete.AnnualReturns(data.PortfolioBenchmark)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("annual returns (benchmark): %v", err))
		} else {
			result.Benchmark = benchReturns
		}
	}

	return result
}

func buildRisk(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) Risk {
	risk := Risk{}

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

func buildRiskVsBenchmark(acct portfolio.Portfolio, warnings *[]string) RiskVsBenchmark {
	return RiskVsBenchmark{
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

func buildDrawdowns(acct portfolio.Portfolio, warnings *[]string) Drawdowns {
	concrete, ok := acct.(*portfolio.Account)
	if !ok {
		*warnings = append(*warnings, "drawdown details require *portfolio.Account")
		return Drawdowns{}
	}

	details, err := concrete.DrawdownDetails(5)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("drawdown details: %v", err))
		return Drawdowns{}
	}

	entries := make([]DrawdownEntry, len(details))
	for idx, detail := range details {
		entries[idx] = DrawdownEntry{
			Start:    detail.Start,
			End:      detail.Trough,
			Recovery: detail.Recovery,
			Depth:    detail.Depth,
			Days:     detail.Days,
		}
	}

	return Drawdowns{Entries: entries}
}

func buildMonthlyReturns(acct portfolio.Portfolio, warnings *[]string) MonthlyReturns {
	concrete, ok := acct.(*portfolio.Account)
	if !ok {
		*warnings = append(*warnings, "monthly returns require *portfolio.Account")
		return MonthlyReturns{}
	}

	years, grid, err := concrete.MonthlyReturns(data.PortfolioEquity)
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("monthly returns: %v", err))
		return MonthlyReturns{}
	}

	return MonthlyReturns{
		Years:  years,
		Values: grid,
	}
}

func buildTrades(acct portfolio.Portfolio, warnings *[]string) Trades {
	transactions := acct.Transactions()

	var entries []TradeEntry

	totalTransactions := 0
	roundTrips := 0

	for _, txn := range transactions {
		switch txn.Type {
		case portfolio.BuyTransaction, portfolio.SellTransaction:
			totalTransactions++

			if txn.Type == portfolio.SellTransaction {
				roundTrips++
			}

			entries = append(entries, TradeEntry{
				Date:   txn.Date,
				Action: txn.Type.String(),
				Ticker: txn.Asset.Ticker,
				Shares: txn.Qty,
				Price:  txn.Price,
				Amount: txn.Amount,
			})
		}
	}

	result := Trades{
		TotalTransactions: totalTransactions,
		RoundTrips:        roundTrips,
		Trades:            entries,
	}

	// Pull aggregate trade metrics from portfolio.
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
// Metric helpers -- return NaN on error and append to warnings.
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

// annualizeTWRR converts a cumulative TWRR to an annualized rate.
func annualizeTWRR(twrr float64, years float64) float64 {
	if math.IsNaN(twrr) || years <= 0 {
		return math.NaN()
	}

	return math.Pow(1+twrr, 1.0/years) - 1
}
