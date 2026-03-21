# PortfolioStats Interface and Metric Caching

## Problem

After the DataFrame column-slice view refactor, profiling shows 10.8 GB total
allocations with GC consuming ~47% of CPU. The dominant allocators are
`sliceByIndices` (2.9 GB from `Drop(NaN)`) and `Apply` (2.0 GB from `Pct()`).

The root cause: 26+ metrics each independently compute the same DataFrame chain
`pd.Window(window).Metrics(PortfolioEquity).Pct().Drop(NaN())`. With 7 windows,
that's 182 redundant computations per backtest day. Similarly, 7 metrics compute
the same drawdown chain and 7 compute the same benchmark returns chain.

## Solution

Introduce a `PortfolioStats` interface that provides lazily cached derived
series. Account implements it, computing each derived series once and inserting
it as a new column on its internal perfData DataFrame. Metrics receive
`PortfolioStats` instead of `*Account` and extract pre-computed data via
zero-copy windowed views.

## PortfolioStats Interface

```go
type PortfolioStats interface {
    Returns(ctx context.Context, window *Period) *data.DataFrame
    ExcessReturns(ctx context.Context, window *Period) *data.DataFrame
    Drawdown(ctx context.Context, window *Period) *data.DataFrame
    BenchmarkReturns(ctx context.Context, window *Period) *data.DataFrame
    Equity(ctx context.Context, window *Period) *data.DataFrame
    Transactions(ctx context.Context) []Transaction
    TradeDetails(ctx context.Context) []TradeDetail
}
```

Each time-series method returns a windowed view of the perfData DataFrame
containing the requested derived column. The first call to a method computes
the derived series and inserts it as a new column on perfData. Subsequent calls
find the column already present and return a zero-copy windowed view.

## PerformanceMetric Interface

```go
type PerformanceMetric interface {
    Name() string
    Description() string
    Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error)
    ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period) (*data.DataFrame, error)
}
```

Changes from the current interface:
- `*Account` replaced with `PortfolioStats` (interface, not concrete type)
- `context.Context` added as first parameter
- `ComputeSeries` returns `*data.DataFrame` instead of `[]float64`

## New Metric Constants

New `data.Metric` constants for derived series stored on perfData:

```go
const (
    PortfolioReturns        Metric = "portfolio_returns"
    PortfolioExcessReturns  Metric = "portfolio_excess_returns"
    PortfolioDrawdown       Metric = "portfolio_drawdown"
    PortfolioBenchReturns   Metric = "portfolio_bench_returns"
)
```

These are inserted as columns on perfData alongside the existing
`PortfolioEquity`, `PortfolioBenchmark`, and `PortfolioRiskFree` columns.

## Account Implementation

Account implements `PortfolioStats`. It maintains a `computed map[data.Metric]bool`
to track which derived columns have been inserted on perfData (since
`metricIndex` is unexported on DataFrame). Each method checks the map, computes
and inserts the column if absent, then returns a windowed view:

```go
// Account gains a new field:
//   computed map[data.Metric]bool

func (a *Account) ensureComputed(metric data.Metric, compute func()) {
    if a.computed == nil {
        a.computed = make(map[data.Metric]bool)
    }
    if !a.computed[metric] {
        compute()
        a.computed[metric] = true
    }
}

func (a *Account) Returns(ctx context.Context, window *Period) *data.DataFrame {
    a.ensureComputed(data.PortfolioReturns, func() {
        eq := a.perfData.Metrics(data.PortfolioEquity)
        returns := eq.Pct()
        a.perfData.Insert(portfolioAsset, data.PortfolioReturns,
            returns.Column(portfolioAsset, data.PortfolioEquity))
    })
    return a.perfData.Window(window)
}

func (a *Account) ExcessReturns(ctx context.Context, window *Period) *data.DataFrame {
    a.Returns(ctx, nil) // ensure returns exist first
    a.ensureComputed(data.PortfolioExcessReturns, func() {
        retCol := a.perfData.Column(portfolioAsset, data.PortfolioReturns)
        rfCol := a.perfData.Column(portfolioAsset, data.PortfolioRiskFree)
        excessCol := make([]float64, len(retCol))
        for i := range retCol {
            excessCol[i] = retCol[i] - rfCol[i]
        }
        a.perfData.Insert(portfolioAsset, data.PortfolioExcessReturns, excessCol)
    })
    return a.perfData.Window(window)
}

func (a *Account) Drawdown(ctx context.Context, window *Period) *data.DataFrame {
    a.ensureComputed(data.PortfolioDrawdown, func() {
        eq := a.perfData.Metrics(data.PortfolioEquity)
        peak := eq.CumMax()
        dd := eq.Sub(peak).Div(peak)
        a.perfData.Insert(portfolioAsset, data.PortfolioDrawdown,
            dd.Column(portfolioAsset, data.PortfolioEquity))
    })
    return a.perfData.Window(window)
}

func (a *Account) BenchmarkReturns(ctx context.Context, window *Period) *data.DataFrame {
    a.ensureComputed(data.PortfolioBenchReturns, func() {
        bm := a.perfData.Metrics(data.PortfolioBenchmark)
        returns := bm.Pct()
        a.perfData.Insert(portfolioAsset, data.PortfolioBenchReturns,
            returns.Column(portfolioAsset, data.PortfolioBenchmark))
    })
    return a.perfData.Window(window)
}

func (a *Account) Equity(ctx context.Context, window *Period) *data.DataFrame {
    return a.perfData.Window(window)
}

func (a *Account) Transactions(ctx context.Context) []Transaction {
    return a.transactions
}

func (a *Account) TradeDetails(ctx context.Context) []TradeDetail {
    return a.tradeDetails
}
```

When `AppendRow` is called on perfData (daily price update), the `computed` map
is cleared so derived columns are recomputed on next access.

## BenchmarkView and PortfolioStats

The existing `benchmarkView()` creates a synthetic Account with the benchmark
equity curve swapped into the portfolio equity position. This Account also
implements `PortfolioStats` with its own `computed` map, so cached columns on
the original Account are not reused (correctly -- the benchmark view has
different equity data).

The `PerformanceMetricQuery` adapter:

```go
func (q PerformanceMetricQuery) resolveStats() (PortfolioStats, error) {
    if !q.benchmark {
        return q.account, nil  // Account implements PortfolioStats
    }
    if _, ok := q.metric.(BenchmarkTargetable); !ok {
        return nil, ErrBenchmarkNotSupported
    }
    return q.account.benchmarkView()  // returns *Account which implements PortfolioStats
}
```

## NaN Handling

`Pct()` produces a leading NaN at index 0. This NaN stays in the column -- the
time axis is not altered. Metrics that previously called `Drop(NaN())` on a
DataFrame now extract a `[]float64` via `Column()` and use a NaN-skipping
helper:

```go
func removeNaN(col []float64) []float64 {
    clean := make([]float64, 0, len(col))
    for _, v := range col {
        if !math.IsNaN(v) {
            clean = append(clean, v)
        }
    }
    return clean
}
```

This allocates ~200 bytes per metric call (one `[]float64`) instead of copying
the entire DataFrame via `sliceByIndices` (which was 2.9 GB total).

`removeNaN` loses index correspondence with the time axis. This is acceptable
for aggregate statistics (mean, std, variance, sort-based metrics) where time
alignment is not needed. Metrics that need time-aligned data should skip NaN
values inline rather than using `removeNaN`.

Metrics must preserve their existing guard clauses during migration (e.g.,
Sharpe checking for risk-free rate availability, Beta checking for benchmark
data). The interface change does not eliminate the need for validation.

## computeMetrics Changes

```go
func computeMetrics(stats portfolio.PortfolioStats, date time.Time,
    metrics []portfolio.PerformanceMetric, appendMetric func(portfolio.MetricRow)) {

    ctx := context.Background()

    for _, metric := range metrics {
        val, err := metric.Compute(ctx, stats, nil)
        if err == nil {
            appendMetric(portfolio.MetricRow{
                Date:   date,
                Name:   metric.Name(),
                Window: "since_inception",
                Value:  val,
            })
        }

        for _, window := range standardWindows() {
            wCopy := window
            val, err := metric.Compute(ctx, stats, &wCopy)
            if err == nil {
                appendMetric(portfolio.MetricRow{
                    Date:   date,
                    Name:   metric.Name(),
                    Window: windowLabel(window),
                    Value:  val,
                })
            }
        }
    }
}
```

The engine calls this with `acct` (which implements `PortfolioStats`),
`acct.RegisteredMetrics()`, and `acct.AppendMetric`.

## PerformanceMetricQuery Adapter

The existing `PerformanceMetricQuery` builder adapts to the new interface.
`Value()` and `Series()` call through to the new `Compute` / `ComputeSeries`
signatures:

```go
func (q PerformanceMetricQuery) Value() (float64, error) {
    stats, err := q.resolveStats()
    if err != nil {
        return 0, err
    }
    return q.metric.Compute(context.Background(), stats, q.window)
}

func (q PerformanceMetricQuery) Series() (*data.DataFrame, error) {
    stats, err := q.resolveStats()
    if err != nil {
        return nil, err
    }
    return q.metric.ComputeSeries(context.Background(), stats, q.window)
}
```

## Example Metric: Sharpe

Before:
```go
func (sharpe) Compute(a *Account, window *Period) (float64, error) {
    pd := a.PerfData()
    if pd == nil {
        return 0, nil
    }
    perfDF := pd.Window(window)
    returns := perfDF.Metrics(data.PortfolioEquity, data.PortfolioRiskFree).Pct().Drop(math.NaN())
    er := returns.Metrics(data.PortfolioEquity).Sub(returns, data.PortfolioRiskFree)
    col := er.Column(portfolioAsset, data.PortfolioEquity)
    // ... stat computations
}
```

After:
```go
func (sharpe) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
    er := stats.ExcessReturns(ctx, window)
    col := er.Column(portfolioAsset, data.PortfolioExcessReturns)
    times := er.Times()
    if len(col) < 2 {
        return 0, nil
    }
    clean := removeNaN(col)
    mean := stat.Mean(clean, nil)
    std := stat.StdDev(clean, nil)
    af := annualizationFactor(times)
    return mean / std * math.Sqrt(af), nil
}
```

## Metric Categories and Migration

**Pattern A (26 metrics): Returns-based**
StdDev, Sharpe, Sortino, DownsideDeviation, TWRR, KRatio, etc.
Change: Replace `pd.Window(window).Metrics(...).Pct().Drop(NaN())` with
`stats.Returns(ctx, window)` or `stats.ExcessReturns(ctx, window)`.

**Pattern B (6 metrics): Drawdown-based**
MaxDrawdown, AvgDrawdown, AvgDrawdownDays, Calmar, RecoveryFactor, KellerRatio.
Change: Replace `equity.CumMax().Sub().Div()` chain with
`stats.Drawdown(ctx, window)`.

**Pattern C (7 metrics): Benchmark-based**
Beta, Alpha, TrackingError, InformationRatio, RSquared, UpsideCapture,
DownsideCapture.
Change: Use `stats.Returns(ctx, window)` which includes benchmark columns.

**Pattern D (3 metrics): Equity-value-based**
CAGR, withdrawal rate metrics.
Change: Use `stats.Equity(ctx, window)`.

**Pattern E (24 metrics): Trade/transaction-based**
WinRate, AverageLoss, LTCG, etc.
Change: Use `stats.Transactions(ctx)` or `stats.TradeDetails(ctx)`. These
metrics do no DataFrame work and are unaffected by the caching.

## Expected Performance Impact

| Allocator | Before | After | Reduction |
|-----------|--------|-------|-----------|
| Apply (Pct) | 2.0 GB | ~80 MB | 96% |
| sliceByIndices (Drop) | 2.9 GB | ~0 | 100% |
| Total alloc | 10.8 GB | ~6 GB | 44% |
| GC CPU | ~47% | ~25% | ~halved |

Estimated runtime improvement: 4.9s to ~3-3.5s.

## Files Requiring Changes

| File | Change |
|------|--------|
| `portfolio/metric_query.go` | New `PortfolioStats` interface, update `PerformanceMetric` interface, adapt query builder |
| `portfolio/account.go` | Implement `PortfolioStats` methods with lazy insert |
| `portfolio/*.go` (69 metric files) | Update `Compute`/`ComputeSeries` signatures and bodies |
| `portfolio/*_test.go` | Update test calls for new signatures |
| `engine/metrics.go` | Update `computeMetrics` to use `PortfolioStats` |
| `data/metric.go` | Add new metric constants |
| `portfolio/metric_helpers.go` | Add `removeNaN` helper |
