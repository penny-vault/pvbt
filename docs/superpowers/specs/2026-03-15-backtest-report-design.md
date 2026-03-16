# Backtest Report Redesign

## Overview

Replace the current flat metric dump in `cli/summary.go` with a rich, structured
backtest report. The report package lives at the top level (`report/`) and produces
a report model (view model) that renderers consume. The first renderer targets the
terminal (lipgloss + ntcharts). Future renderers (PDF, HTML) will consume the same
model.

The architecture follows an **MVVM** (Model-View-ViewModel) pattern:

- **Model**: `portfolio.Account` with its performance metrics -- the domain truth
- **ViewModel**: `report.Report` -- a plain data struct shaped for display, no behavior
- **View**: `report/terminal` -- formats the view model for output
- **Builder**: `report.Build()` -- maps the domain model to the view model

Data flows one direction: domain model -> view model -> rendered output.

## Architecture

```
portfolio.Portfolio  (domain model)
        |
        v
  report.Build(portfolio, StrategyInfo, RunMeta) -> report.Report  (view model)
        |
        v
  report.Report  (structured data -- no formatting, no behavior)
        |
        +---> report/terminal.Render(Report, io.Writer)   [now]   (view)
        +---> report/pdf.Render(Report, io.Writer)        [future]
        +---> report/html.Render(Report, io.Writer)       [future]
```

### report.Report Model

A plain struct tree holding every value the report displays. No formatting, no
ANSI codes, no layout. Each section is its own substruct:

```go
type Report struct {
    Header          Header
    HasBenchmark    bool
    EquityCurve     EquityCurve
    TrailingReturns TrailingReturns
    AnnualReturns   AnnualReturns
    Risk            Risk
    RiskVsBenchmark RiskVsBenchmark  // zero value when HasBenchmark is false
    Drawdowns       Drawdowns
    MonthlyReturns  MonthlyReturns
    Trades          Trades
    Warnings        []string         // non-fatal issues encountered during Build
}
```

### Key Types

```go
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

type EquityCurve struct {
    Times           []time.Time
    StrategyValues  []float64   // dollar values
    BenchmarkValues []float64   // normalized to InitialCash so both lines start equal
}

type TrailingReturns struct {
    Periods   []string  // "1M", "3M", "6M", "YTD", "1Y", "Since Inception"
    Strategy  []float64
    Benchmark []float64
}

type AnnualReturns struct {
    Years     []int
    Strategy  []float64
    Benchmark []float64
}

type Risk struct {
    // Each field is a pair: {Strategy, Benchmark}
    MaxDrawdown        [2]float64
    Volatility         [2]float64
    DownsideDeviation  [2]float64
    Sharpe             [2]float64
    Sortino            [2]float64
    Calmar             [2]float64
    UlcerIndex         [2]float64
    ValueAtRisk        [2]float64
    Skewness           [2]float64
    ExcessKurtosis     [2]float64
}

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

type DrawdownEntry struct {
    Start    time.Time
    End      time.Time
    Recovery time.Time   // zero if not recovered
    Depth    float64
    Days     int         // trading days (data points) from start to recovery or end of data
}

type Drawdowns struct {
    Entries []DrawdownEntry // top 5 by depth
}

type MonthlyReturns struct {
    Years  []int
    // Values[yearIdx][monthIdx] -- NaN for months outside range
    Values [][]float64
}

type TradeEntry struct {
    Date   time.Time
    Action string   // "BUY", "SELL", "HOLD"
    Ticker string
    Shares float64
    Price  float64
    Amount float64
    PL     float64  // NaN if no P/L (buys, holds)
}

type Trades struct {
    TotalTransactions int
    RoundTrips        int
    WinRate           float64
    AvgHolding        float64 // days
    AvgWin            float64
    AvgLoss           float64
    ProfitFactor      float64
    GainLossRatio     float64
    Turnover          float64
    PositivePeriods   float64
    Trades            []TradeEntry // all trades; renderer truncates as needed
}
```

### report.Build()

```go
func Build(acct portfolio.Portfolio, info engine.StrategyInfo, meta RunMeta) (Report, error)
```

`RunMeta` carries elapsed time, step count, and initial cash -- values that come
from `runBacktest`, not from the portfolio itself.

```go
type RunMeta struct {
    Elapsed     time.Duration
    Steps       int
    InitialCash float64
}
```

`Build` calls `acct.PerformanceMetric(...)` with appropriate windows and the new
`.Benchmark()` selector to populate every field. It does zero math -- all values
come from the portfolio package (the PerformanceMetric system for scalar metrics,
and Account methods like MonthlyReturns/AnnualReturns/DrawdownDetails for
structured data that does not fit the scalar metric interface).

### Edge Cases

**No benchmark configured:** `Build` sets `Report.HasBenchmark = false`. All
benchmark-dependent fields use `math.NaN()`. The terminal renderer checks
`HasBenchmark` and hides benchmark columns or shows "N/A". The `RiskVsBenchmark`
section and equity curve benchmark line are skipped entirely.

**Trailing return periods exceeding data range:** If the backtest covers 2 months,
the 3M/6M/1Y trailing return values are `math.NaN()`. The renderer displays "N/A".

**Insufficient data:** If `perfData.Len() < 2`, `Build` returns a Report with only
the `Header` populated and a warning in `Report.Warnings`. The renderer displays
the header and the warning, skipping all other sections.

**Partial metric failures:** Follow the existing `errors.Join` pattern. Individual
metric failures populate their fields with `math.NaN()` and add a warning to
`Report.Warnings`. `Build` returns a usable Report alongside the joined error.
The renderer displays what it can.

## Performance Metric Changes

### 1. Benchmark Targeting on PerformanceMetricQuery

Add a `.Benchmark()` method to the query builder that tells the metric to compute
against the benchmark equity curve instead of the portfolio equity curve.

```go
// Current usage (unchanged):
acct.PerformanceMetric(Sharpe).Value()

// New: compute against benchmark curve:
acct.PerformanceMetric(Sharpe).Benchmark().Value()
```

**Implementation:** Add a `benchmark bool` field to `PerformanceMetricQuery`.
Thread it into `Compute` via a new `ComputeOptions` struct or by extending the
`*Account` method set to expose a benchmark-targeted perfData view.

The cleanest approach: add a method to `Account` that returns a "view" DataFrame
where `PortfolioEquity` is swapped with `PortfolioBenchmark`. Metrics already read
from `a.PerfData()` -- if the query builder substitutes a benchmark view, every
existing metric works against the benchmark without any per-metric changes.

```go
// In account.go:
func (a *Account) BenchmarkPerfData() *data.DataFrame {
    // Returns a DataFrame where PortfolioEquity column contains
    // the benchmark values (normalized to starting portfolio value).
    // PortfolioBenchmark and PortfolioRiskFree remain unchanged.
}
```

The query builder creates a thin wrapper Account that overrides `PerfData()` to
return the benchmark view. This wrapper only needs to satisfy the internal metric
computation interface -- it is not exposed to strategy code.

**Constraints on `.Benchmark()`:**

`.Benchmark()` is only meaningful for **single-series metrics** that read
`PortfolioEquity` from perfData (Sharpe, Sortino, Calmar, MaxDrawdown, StdDev,
TWRR, Skewness, ExcessKurtosis, ValueAtRisk, UlcerIndex, DownsideDeviation).

It must NOT be used on:

- **Relational metrics** that read both `PortfolioEquity` and `PortfolioBenchmark`
  (Beta, Alpha, R-Squared, TrackingError, InformationRatio, Treynor,
  UpsideCapture, DownsideCapture). These would compute benchmark-vs-benchmark,
  yielding nonsense (e.g., Beta = 1.0 always).

- **Transaction-based metrics** that derive values from the transaction log
  (WinRate, AverageWin, AverageLoss, ProfitFactor, AverageHoldingPeriod,
  Turnover, tax metrics). Swapping the equity column has no effect on these.

The query builder should return `ErrBenchmarkNotSupported` if `.Benchmark()` is
called on a metric that does not support it. Each metric implementation will need
a way to declare whether it is benchmark-targetable (e.g., an optional interface
`BenchmarkTargetable` that single-series metrics implement).

### 2. New Account Method: MonthlyReturns

Returns a structured result of monthly returns by year. Since the
`PerformanceMetric` interface returns `(float64, error)` or `([]float64, error)`,
monthly returns need a different approach.

Add a method directly to `Account` that accepts a `data.Metric` parameter to
select which curve to compute against:

```go
func (a *Account) MonthlyReturns(metric data.Metric) (years []int, values [][]float64, err error)
```

Called as `a.MonthlyReturns(data.PortfolioEquity)` for strategy returns or
`a.MonthlyReturns(data.PortfolioBenchmark)` for benchmark returns. This avoids
duplicating the method for each curve and mirrors the `Column(asset, metric)`
pattern used by DataFrame.

Computes month-over-month returns from the equity curve using the existing
`Downsample(Monthly).Last()` operation, then calculates percentage changes.

### 3. New Account Method: AnnualReturns

Same parameterized pattern:

```go
func (a *Account) AnnualReturns(metric data.Metric) (years []int, returns []float64, err error)
```

Called as `a.AnnualReturns(data.PortfolioEquity)` or
`a.AnnualReturns(data.PortfolioBenchmark)`.

### 4. New Metric: DrawdownDetails

Returns the top N drawdown periods with full metadata:

```go
type DrawdownDetail struct {
    Start    time.Time
    Trough   time.Time
    Recovery time.Time // zero value if still in drawdown
    Depth    float64   // negative fraction, e.g. -0.0619
    Duration int       // calendar days from start to recovery (or end of data)
}

func (a *Account) DrawdownDetails(topN int) ([]DrawdownDetail, error)
```

Walks the equity curve to identify distinct drawdown periods (peak to recovery),
sorts by depth, returns the top N.

## Terminal Renderer

`report/terminal/renderer.go` -- takes a `report.Report` and writes styled
terminal output to an `io.Writer`.

### Sections

Each section is a function that renders its portion:

- `renderHeader` -- strategy name, version, benchmark, dates, initial/final, elapsed
- `renderEquityCurve` -- ntcharts line chart, strategy + benchmark, normalized benchmark, legend
- `renderTrailingReturns` -- table with 1M/3M/6M/YTD/1Y/SI columns, strategy/benchmark/+/- rows
- `renderAnnualReturns` -- table with year columns, strategy/benchmark/+/- rows
- `renderRisk` -- two-column table (strategy vs benchmark)
- `renderRiskVsBenchmark` -- 2x4 grid layout (Beta/Alpha/R-Squared/Tracking Error, Info Ratio/Treynor/Upside Capture/Downside Capture)
- `renderDrawdowns` -- table with # / Start / End / Recovery / Depth / Duration columns
- `renderMonthlyReturns` -- grid with Jan-Dec columns, year rows, color-coded positive/negative
- `renderTrades` -- summary stats + recent transaction table

### Styling

Use lipgloss throughout:

- Section headers: bold, colored
- Positive values: green
- Negative values: red
- Table borders: dim
- Column alignment: right-aligned for numbers, left for labels
- Equity curve: ntcharts canvas with lipgloss-styled series

### Width

Target 80 columns as minimum viable width. Detect terminal width and expand
tables to fill when wider. The equity curve chart scales to available width.

## Integration with runBacktest

### Timing

Wrap the `eng.Backtest()` call with timing:

```go
startTime := time.Now()
result, err := eng.Backtest(ctx, start, end)
elapsed := time.Since(startTime)
```

### Step Count

Derived from `result.PerfData().Len()` -- no new engine API needed.

### StrategyInfo

Call `engine.DescribeStrategy(eng)` after `Backtest()` returns (engine is still
alive at that point, before `defer eng.Close()`).

### Replacing printSummary

Replace the `printSummary(result)` call with:

```go
info := engine.DescribeStrategy(eng)
meta := report.RunMeta{
    Elapsed:     elapsed,
    Steps:       result.PerfData().Len(),
    InitialCash: cash,
}

rpt, err := report.Build(result, info, meta)
if err != nil {
    return fmt.Errorf("building report: %w", err)
}

terminal.Render(rpt, os.Stdout)
```

The old `cli/summary.go` is deleted.

## File Layout

```
report/
    report.go          -- Report struct, Build(), RunMeta, all view model types
    terminal/
        renderer.go    -- Render(Report, io.Writer)
        header.go      -- renderHeader
        chart.go       -- renderEquityCurve (ntcharts)
        returns.go     -- renderTrailingReturns, renderAnnualReturns
        risk.go        -- renderRisk, renderRiskVsBenchmark
        drawdowns.go   -- renderDrawdowns
        monthly.go     -- renderMonthlyReturns
        trades.go      -- renderTrades
        style.go       -- shared lipgloss styles
portfolio/
    benchmark_view.go  -- BenchmarkPerfData, benchmarkAccount wrapper
    monthly_returns.go -- MonthlyReturns(metric)
    annual_returns.go  -- AnnualReturns(metric)
    drawdown_detail.go -- DrawdownDetails(topN)
cli/
    backtest.go        -- timing, DescribeStrategy, report.Build + terminal.Render
    summary.go         -- DELETED
```

## Relationship to Study Runner Framework (Issue #35)

The study runner framework (issue #35) will orchestrate multiple backtest runs for
stress tests, capacity analysis, parameter optimization, etc. Each study produces
its own results that need reporting.

This design is compatible because:

1. **The report model is data, not presentation.** A study that includes a standard
   backtest as one component can embed `report.Report` in its own result struct.

2. **Renderers are separate from models.** Study-specific renderers can call
   `terminal.RenderSection(...)` to include standard backtest sections within a
   larger study report, or render their own sections alongside.

3. **The terminal renderer functions are per-section.** A stress test report could
   render its scenario comparison table, then call `renderDrawdowns` and
   `renderRisk` from the terminal package for each scenario's backtest.

No special accommodation is needed now. The clean separation between model and
renderer means study reports can compose backtest report sections naturally.

## Out of Scope

- PDF renderer
- HTML renderer
- Tax metrics section in report (can be added later as a section)
- Withdrawal metrics section in report (can be added later as a section)
- Interactive TUI report (the existing `--tui` flag is a separate feature)
