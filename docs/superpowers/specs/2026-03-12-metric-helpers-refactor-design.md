# Metric Helpers Refactor

## Problem

`portfolio/metric_helpers.go` contains unexported helper functions that violate the project's design principles:

1. **Bypassing PerformanceMetric pattern** -- `returns()`, `drawdownSeries()`, and `cagr()` perform computations that should flow through the PerformanceMetric interface. Other metrics call these helpers directly instead of composing with PerformanceMetric implementations.

2. **Reimplementing DataFrame operations** -- `windowSlice()`, `windowSliceTimes()`, and `annualizationFactor()` manually trim and analyze `[]float64` and `[]time.Time` slices. These operations duplicate what DataFrame already provides via `Between()` and its time axis.

3. **Scattered raw slices instead of DataFrame** -- Account stores `equityCurve []float64`, `equityTimes []time.Time`, `benchmarkPrices []float64`, and `riskFreePrices []float64` as separate fields. These are time-indexed series -- exactly what a DataFrame is for.

4. **Untestable without export shims** -- `portfolio/export_test.go` exposes internals to test these helpers, violating the project's strict black-box testing rule.

## Design

### 1. Account stores a performance DataFrame

Replace the four separate slices in Account with a single DataFrame:

```go
type Account struct {
    // ... existing fields ...
    // Remove: equityCurve, equityTimes, benchmarkPrices, riskFreePrices
    perfData *data.DataFrame  // columns: PortfolioEquity, PortfolioBenchmark, PortfolioRiskFree
}
```

New metric constants in the `data` package (in the "Portfolio performance metrics" group):

```go
const (
    PortfolioEquity    Metric = "PortfolioEquity"
    PortfolioBenchmark Metric = "PortfolioBenchmark"
    PortfolioRiskFree  Metric = "PortfolioRiskFree"
)
```

These are distinct from the existing `Equity` balance sheet metric.

A synthetic asset represents the portfolio itself:

```go
// in portfolio package
var portfolioAsset = asset.Asset{
    CompositeFigi: "_PORTFOLIO_",
    Ticker:        "_PORTFOLIO_",
}
```

`UpdatePrices` builds a single-row DataFrame and merges it:

```go
func (a *Account) UpdatePrices(df *data.DataFrame) {
    a.prices = df

    // Compute total portfolio value.
    total := a.cash
    for ast, qty := range a.holdings {
        v := df.Value(ast, data.MetricClose)
        if !math.IsNaN(v) {
            total += qty * v
        }
    }

    // Extract benchmark and risk-free values.
    var benchVal, rfVal float64
    if a.benchmark != (asset.Asset{}) {
        v := df.Value(a.benchmark, data.AdjClose)
        if !math.IsNaN(v) {
            benchVal = v
        }
    }
    if a.riskFree != (asset.Asset{}) {
        v := df.Value(a.riskFree, data.AdjClose)
        if !math.IsNaN(v) {
            rfVal = v
        }
    }

    // Build single-row DataFrame.
    t := []time.Time{df.End()}
    assets := []asset.Asset{portfolioAsset}
    metrics := []data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree}
    row, err := data.NewDataFrame(t, assets, metrics, []float64{total, benchVal, rfVal})
    if err != nil {
        return // should not happen with valid inputs
    }

    if a.perfData == nil {
        a.perfData = row
    } else {
        merged, err := data.MergeTimes(a.perfData, row)
        if err != nil {
            return // duplicate timestamp -- engine guarantees one call per timestamp
        }
        a.perfData = merged
    }
}
```

Note: `MergeTimes` rejects overlapping timestamps. The engine guarantees `UpdatePrices` is called exactly once per trading date, so this is safe. The error is handled explicitly (not silently discarded) to avoid nil `perfData`.

Account accessor methods change:

| Old | New |
|-----|-----|
| `EquityCurve() []float64` | `PerfData() *data.DataFrame` |
| `EquityTimes() []time.Time` | (use `PerfData().Times()`) |
| `BenchmarkPrices() []float64` | (use `PerfData().Column(portfolioAsset, data.PortfolioBenchmark)`) |
| `RiskFreePrices() []float64` | (use `PerfData().Column(portfolioAsset, data.PortfolioRiskFree)`) |

Since all callers of the old methods are PerformanceMetric implementations (internal to the package), we remove them and have metrics use `PerfData()` directly.

### 2. New PerformanceMetrics

#### Returns

File: `portfolio/returns.go`

```go
type returnsMetric struct{}

func (returnsMetric) Name() string { return "Returns" }
func (returnsMetric) Description() string {
    return "Period-over-period returns derived from the equity curve."
}

func (returnsMetric) Compute(a *Account, window *Period) (float64, error) {
    perfDF := a.windowedPerfData(window)
    eq := perfDF.Column(portfolioAsset, data.PortfolioEquity)
    if len(eq) < 2 {
        return 0, nil
    }
    return (eq[len(eq)-1] - eq[0]) / eq[0], nil
}

func (returnsMetric) ComputeSeries(a *Account, window *Period) ([]float64, error) {
    perfDF := a.windowedPerfData(window)
    eq := perfDF.Column(portfolioAsset, data.PortfolioEquity)
    if len(eq) < 2 {
        return nil, nil
    }
    r := make([]float64, len(eq)-1)
    for i := range r {
        r[i] = (eq[i+1] - eq[i]) / eq[i]
    }
    return r, nil
}

var Returns PerformanceMetric = returnsMetric{}
```

#### DrawdownSeries

File: `portfolio/drawdown_series.go`

```go
type drawdownSeriesMetric struct{}

func (drawdownSeriesMetric) Name() string { return "DrawdownSeries" }
func (drawdownSeriesMetric) Description() string {
    return "Drawdown at each point in the equity curve, as negative fractions from peak."
}

func (d drawdownSeriesMetric) Compute(a *Account, window *Period) (float64, error) {
    series, err := d.ComputeSeries(a, window)
    if err != nil || len(series) == 0 {
        return 0, err
    }
    return series[len(series)-1], nil
}

func (drawdownSeriesMetric) ComputeSeries(a *Account, window *Period) ([]float64, error) {
    perfDF := a.windowedPerfData(window)
    eq := perfDF.Column(portfolioAsset, data.PortfolioEquity)
    dd := make([]float64, len(eq))
    peak := math.Inf(-1)
    for i, v := range eq {
        if v > peak {
            peak = v
        }
        dd[i] = (v - peak) / peak
    }
    return dd, nil
}

var DrawdownSeries PerformanceMetric = drawdownSeriesMetric{}
```

#### ExcessReturns

File: `portfolio/excess_returns.go`

```go
type excessReturnsMetric struct{}

func (excessReturnsMetric) Name() string { return "ExcessReturns" }
func (excessReturnsMetric) Description() string {
    return "Portfolio returns minus risk-free returns, element-wise."
}

func (e excessReturnsMetric) Compute(a *Account, window *Period) (float64, error) {
    series, err := e.ComputeSeries(a, window)
    if err != nil || len(series) == 0 {
        return 0, err
    }
    return data.SliceMean(series), nil
}

func (excessReturnsMetric) ComputeSeries(a *Account, window *Period) ([]float64, error) {
    perfDF := a.windowedPerfData(window)
    eq := perfDF.Column(portfolioAsset, data.PortfolioEquity)
    rf := perfDF.Column(portfolioAsset, data.PortfolioRiskFree)

    // Compute portfolio returns from equity curve.
    r := periodsReturns(eq)

    // Compute risk-free returns from risk-free price series.
    // This uses the same formula as Returns but on the risk-free column.
    rfr := periodsReturns(rf)

    n := len(r)
    if len(rfr) < n {
        n = len(rfr)
    }
    er := make([]float64, n)
    for i := 0; i < n; i++ {
        er[i] = r[i] - rfr[i]
    }
    return er, nil
}

var ExcessReturns PerformanceMetric = excessReturnsMetric{}
```

Note: `periodsReturns` is an unexported function in `excess_returns.go` that computes `(p[i+1]-p[i])/p[i]` on a raw `[]float64`. This is the same arithmetic as `Returns.ComputeSeries`, but `Returns` operates through `Account.windowedPerfData` which always extracts the equity curve. ExcessReturns needs returns on the risk-free column directly, so it uses a shared helper. This helper is purely arithmetic on `[]float64` with no Account dependency -- it could also live in `data/stats.go` as an exported function if other callers need it.

### 3. Account windowing helper

A single unexported method on Account replaces `windowSlice` and `windowSliceTimes`:

```go
func (a *Account) windowedPerfData(window *Period) *data.DataFrame {
    if window == nil || a.perfData == nil {
        return a.perfData
    }
    end := a.perfData.End()
    var start time.Time
    switch window.Unit {
    case UnitDay:
        start = end.AddDate(0, 0, -window.N)
    case UnitMonth:
        start = end.AddDate(0, -window.N, 0)
    case UnitYear:
        start = end.AddDate(-window.N, 0, 0)
    }
    return a.perfData.Between(start, end)
}
```

This is not a PerformanceMetric -- it's an Account method that translates a Period into a `DataFrame.Between()` call. Every PerformanceMetric calls `a.windowedPerfData(window)` as its first step.

Behavioral note: `Between` returns an empty DataFrame if the window extends beyond available data, whereas the old `windowSlice` returned the full series. Callers already guard against empty data (checking `len(eq) < 2`), so this change is safe.

### 4. Pure math moves to data package

Export these as package-level functions in `data/stats.go`:

```go
func SliceMean(x []float64) float64
func Variance(x []float64) float64
func Stddev(x []float64) float64
func Covariance(x, y []float64) float64
```

`SliceMean` (not `Mean`) avoids collision with the existing `data.Mean` aggregation constant in `data/aggregation.go`.

These operate on `[]float64` and have no dependency on DataFrame or Account. They are general-purpose statistical functions compatible with gonum column slices.

### 5. Helpers that stay in portfolio

`roundTrips` and `realizedGains` operate on `[]Transaction`, not on `[]float64` columns. They are FIFO lot-matching algorithms specific to the portfolio domain. They stay as unexported helpers in `portfolio/metric_helpers.go`.

### 6. CAGR metric

The unexported `cagr()` helper is deleted. The CAGR PerformanceMetric computes directly:

```go
func (cagrMetric) Compute(a *Account, window *Period) (float64, error) {
    perfDF := a.windowedPerfData(window)
    eq := perfDF.Column(portfolioAsset, data.PortfolioEquity)
    times := perfDF.Times()
    if len(eq) < 2 || len(times) < 2 {
        return 0, nil
    }
    years := times[len(times)-1].Sub(times[0]).Hours() / 24 / 365.25
    if years <= 0 || eq[0] <= 0 || eq[len(eq)-1] <= 0 {
        return 0, nil
    }
    return math.Pow(eq[len(eq)-1]/eq[0], 1.0/years) - 1, nil
}
```

### 7. AnnualizationFactor

`annualizationFactor` becomes a package-level function in `data/stats.go`:

```go
func AnnualizationFactor(times []time.Time) float64
```

It estimates periods-per-year from the time axis: if the average gap between timestamps exceeds 20 calendar days, return 12 (monthly); otherwise return 252 (daily). Defaults to 252 for fewer than 2 timestamps.

This is a package function, not a DataFrame method, since it only needs a `[]time.Time` and has no DataFrame dependency. Callers pass `perfDF.Times()`.

### 8. How existing metrics change

Existing PerformanceMetric implementations (Sharpe, Sortino, MaxDrawdown, etc.) change from:

```go
// Before
func (sharpe) Compute(a *Account, window *Period) (float64, error) {
    eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
    r := returns(eq)
    rf := returns(windowSlice(a.RiskFreePrices(), a.EquityTimes(), window))
    er := excessReturns(r, rf)
    sd := stddev(er)
    if sd == 0 { return 0, nil }
    af := annualizationFactor(a.EquityTimes())
    return mean(er) / sd * math.Sqrt(af), nil
}
```

To:

```go
// After
func (sharpe) Compute(a *Account, window *Period) (float64, error) {
    er, err := ExcessReturns.ComputeSeries(a, window)
    if err != nil { return 0, err }
    sd := data.Stddev(er)
    if sd == 0 { return 0, nil }
    perfDF := a.windowedPerfData(window)
    af := data.AnnualizationFactor(perfDF.Times())
    return data.SliceMean(er) / sd * math.Sqrt(af), nil
}
```

All ~45 metric files follow this pattern: replace helper calls with PerformanceMetric composition and `data.` stats calls.

### 9. PortfolioSnapshot interface change

`PortfolioSnapshot` in `portfolio/snapshot.go` is a public interface that currently exposes:

```go
EquityCurve() []float64
EquityTimes() []time.Time
BenchmarkPrices() []float64
RiskFreePrices() []float64
```

These change to a single method:

```go
PerfData() *data.DataFrame
```

`WithPortfolioSnapshot` reconstructs `perfData` from the snapshot's DataFrame. This is a breaking change to the `PortfolioSnapshot` interface. Since `Account` is the only known implementor, the impact is contained.

### 10. Files deleted

| File | Reason |
|------|--------|
| `portfolio/export_test.go` | Exposes internals; violates black-box testing |
| `portfolio/metric_helpers_test.go` | Tests internals via export shim; behavior tested through PerformanceMetric tests |

### 11. Files created

| File | Contents |
|------|----------|
| `data/stats.go` | `SliceMean`, `Variance`, `Stddev`, `Covariance`, `AnnualizationFactor` |
| `data/stats_test.go` | Black-box ginkgo tests for stats functions |
| `portfolio/returns.go` | `Returns` PerformanceMetric |
| `portfolio/drawdown_series.go` | `DrawdownSeries` PerformanceMetric |
| `portfolio/excess_returns.go` | `ExcessReturns` PerformanceMetric + `periodsReturns` helper |

### 12. Files modified

| File | Change |
|------|--------|
| `data/metric.go` | Add `PortfolioEquity`, `PortfolioBenchmark`, `PortfolioRiskFree` constants |
| `portfolio/account.go` | Replace four slices with `perfData *data.DataFrame`; add `windowedPerfData()` and `PerfData()`; add `portfolioAsset` var; rewrite `UpdatePrices`; remove `EquityCurve()`, `EquityTimes()`, `BenchmarkPrices()`, `RiskFreePrices()` |
| `portfolio/snapshot.go` | Change `PortfolioSnapshot` interface; update `WithPortfolioSnapshot` to restore `perfData` |
| `portfolio/metric_helpers.go` | Remove `returns`, `excessReturns`, `windowSlice`, `windowSliceTimes`, `annualizationFactor`, `drawdownSeries`, `cagr`, `mean`, `variance`, `stddev`, `covariance`; keep `roundTrips`, `realizedGains` |
| `portfolio/cagr_metric.go` | Inline computation, no helper |
| `portfolio/sharpe.go` | Use `ExcessReturns.ComputeSeries()`, `data.Stddev()`, `data.SliceMean()` |
| `portfolio/sortino.go` | Same pattern as Sharpe |
| `portfolio/max_drawdown.go` | Use `DrawdownSeries.ComputeSeries()` |
| ~40 other metric files | Replace helper calls with PerformanceMetric composition and `data.` stats calls |
| `portfolio/account_test.go` | Update references from `EquityCurve()` etc. to `PerfData()` |
| `portfolio/return_metrics_test.go` | Add tests for Returns, ExcessReturns PerformanceMetrics |
| `cli/output.go`, `cli/output_jsonl.go`, `cli/output_parquet.go` | Migrate from `EquityCurve()`/`BenchmarkPrices()` to `PerfData()` |

### 13. Testing strategy

All new PerformanceMetrics (Returns, DrawdownSeries, ExcessReturns) get black-box ginkgo tests. Tests construct an Account, call `UpdatePrices` to build the performance DataFrame, then assert via `PerformanceMetric(Returns).Series()`.

The `data/stats.go` functions get their own ginkgo tests in `data/stats_test.go`.

`AnnualizationFactor` is tested in `data/stats_test.go`.
