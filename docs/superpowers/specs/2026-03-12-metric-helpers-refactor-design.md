# Metric Helpers Refactor

## Problem

`portfolio/metric_helpers.go` contains unexported helper functions that violate the project's design principles:

1. **Bypassing DataFrame operations** -- `returns()`, `excessReturns()`, `mean()`, `variance()`, `stddev()`, `covariance()`, `drawdownSeries()`, and `cagr()` manually operate on `[]float64` slices. DataFrame already provides `Pct()`, `Mean()`, `Std()`, `Variance()`, `Covariance()`, `Sub()`, and `CumSum()` as chainable methods. The helpers bypass the consistent DataFrame-centric system.

2. **Reimplementing DataFrame windowing** -- `windowSlice()` and `windowSliceTimes()` manually trim `[]float64` and `[]time.Time` slices by walking timestamps. DataFrame provides `Between()` and will provide `Window()` for this.

3. **Scattered raw slices instead of DataFrame** -- Account stores `equityCurve []float64`, `equityTimes []time.Time`, `benchmarkPrices []float64`, and `riskFreePrices []float64` as four parallel fields. These are time-indexed series -- exactly what a DataFrame is for.

4. **Untestable without export shims** -- `portfolio/export_test.go` exposes internals to test these helpers, violating the project's strict black-box testing rule.

## Design

### 1. Move Period to data package

`Period`, `Days()`, `Months()`, `Years()`, `YTD()`, `MTD()`, `WTD()` move from `portfolio/period.go` to `data/period.go`. This allows DataFrame to accept Period directly for windowing.

Add a method to Period that computes the start date for a trailing window:

```go
func (p Period) Before(ref time.Time) time.Time
```

This absorbs the calendar logic currently in the unexported `periodCutoff` function: day/month/year subtraction via `AddDate`, YTD returns Jan 1, MTD returns 1st of month, WTD returns most recent Monday.

The portfolio package uses type aliases for API compatibility:

```go
type Period = data.Period

var (
    Days   = data.Days
    Months = data.Months
    Years  = data.Years
    YTD    = data.YTD
    MTD    = data.MTD
    WTD    = data.WTD
)
```

The `UnitDay`, `UnitMonth`, `UnitYear`, `UnitYTD`, `UnitMTD`, `UnitWTD` constants also move to `data`.

### 2. Add DataFrame.Window

```go
func (df *DataFrame) Window(p *Period) *DataFrame
```

Accepts `*Period` (pointer, matching the existing `Compute` signature). When nil, returns the full DataFrame unchanged. When `p.Before(df.End())` returns a time before `df.Start()`, returns the full DataFrame (same as nil -- the window exceeds available data). Otherwise delegates to `Between()`.

Follows the standard DataFrame error model: if `df.err != nil`, returns `WithErr(df.err)`.

This replaces `windowSlice` and `windowSliceTimes`. Performance metrics call `a.perfData.Window(window)` directly.

### 3. Add DataFrame.CumMax

```go
func (df *DataFrame) CumMax() *DataFrame
```

Running maximum per column. Same family as `CumSum()`. Follows the standard error model (`df.err` propagation). Performance metrics use this to compute drawdown without a dedicated helper:

```go
peak := eq.CumMax()
dd := eq.Sub(peak).Div(peak)
```

### 4. Extend Sub/Add/Mul/Div with variadic metric parameter

Current signature:

```go
func (df *DataFrame) Sub(other *DataFrame) *DataFrame
```

Extended signature:

```go
func (df *DataFrame) Sub(other *DataFrame, metrics ...Metric) *DataFrame
```

**Broadcast semantics**: when metrics are specified, select those metric columns from `other` (across all assets in `other`). For each specified metric column, subtract its values from every column in `df`, regardless of `df`'s metric names. This is a broadcast: one column from `other` is applied to all columns in `df`. **The result retains the column structure (assets and metric names) of the receiver `df`**, not `other`.

Multiple metrics chain sequentially with accumulated results: `df.Sub(other, m1, m2)` computes `(df - other[m1]) - other[m2]`.

When no metrics are specified, behavior is unchanged (element-wise on matching (asset, metric) pairs via `findIntersection`).

Same extension applies to `Add`, `Mul`, `Div`.

All variants follow the standard DataFrame error model: if either `df.err` or `other.err` is non-nil, returns `WithErr(err)`.

**Why this is needed**: after filtering a DataFrame to a single metric (e.g., `perfDF.Metrics(PortfolioEquity)`), the result carries that metric name. A second DataFrame filtered to a different metric (e.g., `PortfolioRiskFree`) has non-matching metric names. The default `Sub()` with no variadic args would find no intersection and return an empty DataFrame. The variadic form bypasses metric name matching by explicitly selecting from `other` and broadcasting.

### 5. Account data consolidation

**Remove** from Account struct:
- `equityCurve []float64`
- `equityTimes []time.Time`
- `benchmarkPrices []float64`
- `riskFreePrices []float64`

**Keep**:
- `prices *data.DataFrame` -- transient single-step snapshot from the engine, used by `Value()` and `PositionValue()` to mark holdings to market, overwritten each step, not serialized

**Add**:
- `perfData *data.DataFrame` -- accumulated time series, serialized to SQLite

The metric constants `PortfolioEquity`, `PortfolioBenchmark`, `PortfolioRiskFree` already exist in `data/metric.go`.

A synthetic asset represents the portfolio itself:

```go
var portfolioAsset = asset.Asset{
    CompositeFigi: "_PORTFOLIO_",
    Ticker:        "_PORTFOLIO_",
}
```

The `_PORTFOLIO_` CompositeFigi is a reserved value that cannot collide with real FIGI identifiers (real FIGIs follow the format `BBG...`). This synthetic asset never leaves the Account -- it is not passed to data providers or the broker.

`perfData` has one asset (`portfolioAsset`) with three metrics (`PortfolioEquity`, `PortfolioBenchmark`, `PortfolioRiskFree`). It grows one row per trading step.

#### Efficient row appending

The old `append()` to `[]float64` was O(1) amortized. Calling `MergeTimes` on every step would be O(n^2) over a full backtest (each merge copies the entire growing DataFrame). To avoid this, add an `AppendRow` method to DataFrame:

```go
func (df *DataFrame) AppendRow(t time.Time, values []float64) error
```

`AppendRow` appends a single timestamp and its column values to the existing DataFrame in place, growing the underlying slices with Go's amortized-doubling `append`. The `values` slice must have length `len(assets) * len(metrics)`, ordered as `[asset0_metric0, asset0_metric1, ..., asset1_metric0, ...]` (matching the column-major layout). Returns an error if the timestamp is not after the DataFrame's current `End()` (enforces chronological order) or if the values length is wrong.

**Mutation safety**: `AppendRow` is the only DataFrame method that mutates in place (all other methods return new DataFrames). This is safe because all read methods (`Window`, `Between`, `Metrics`, `Assets`, etc.) produce independent copies of the underlying data via `make` + `copy`. A `Window()` result taken before an `AppendRow` call is not affected by the mutation.

`UpdatePrices` uses `AppendRow` instead of `MergeTimes`:

```go
func (a *Account) UpdatePrices(df *data.DataFrame) {
    a.prices = df

    total := a.cash
    for ast, qty := range a.holdings {
        v := df.Value(ast, data.MetricClose)
        if !math.IsNaN(v) {
            total += qty * v
        }
    }

    var benchVal, rfVal float64
    if a.benchmark != (asset.Asset{}) {
        v := df.Value(a.benchmark, data.AdjClose)
        if math.IsNaN(v) || v == 0 {
            v = df.Value(a.benchmark, data.MetricClose)
        }
        benchVal = v
    }
    if a.riskFree != (asset.Asset{}) {
        v := df.Value(a.riskFree, data.AdjClose)
        if math.IsNaN(v) || v == 0 {
            v = df.Value(a.riskFree, data.MetricClose)
        }
        rfVal = v
    }

    if a.perfData == nil {
        t := []time.Time{df.End()}
        assets := []asset.Asset{portfolioAsset}
        metrics := []data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree}
        row, err := data.NewDataFrame(t, assets, metrics, []float64{total, benchVal, rfVal})
        if err != nil {
            log.Error().Err(err).Msg("UpdatePrices: failed to create perfData")
            return
        }
        a.perfData = row
    } else {
        if err := a.perfData.AppendRow(df.End(), []float64{total, benchVal, rfVal}); err != nil {
            log.Error().Err(err).Msg("UpdatePrices: failed to append to perfData")
            return
        }
    }
}
```

Note: `UpdatePrices` has no error return (constrained by the `PortfolioManager` interface). Errors are logged but not propagated. This is a known limitation; changing the interface is out of scope for this refactor.

**Remove accessors**: `EquityCurve()`, `EquityTimes()`, `BenchmarkPrices()`, `RiskFreePrices()`. Replace with `PerfData() *data.DataFrame`.

### 6. AnnualizationFactor stays in portfolio

`annualizationFactor` is a financial concept (252 trading days per year, 12 months per year). It does not belong in the generic `data` package. It remains as an unexported helper in `portfolio/metric_helpers.go`. The existing exported `data.AnnualizationFactor` in `data/stats.go` is deleted along with its tests in `data/stats_test.go`.

Note: the `2026-03-12-dataframe-stats-api-design.md` spec says `AnnualizationFactor` stays in `data/stats.go`. This spec supersedes that decision -- the function moves to portfolio.

### 7. Helper deletion and migration

**Delete from metric_helpers.go**: `returns`, `excessReturns`, `windowSlice`, `windowSliceTimes`, `mean`, `variance`, `stddev`, `covariance`, `cagr`, `drawdownSeries`, `periodCutoff`

**Keep in metric_helpers.go**: `roundTrips`, `realizedGains`, `roundTrip` struct, `annualizationFactor`

`roundTrips` and `realizedGains` are FIFO lot-matching algorithms that operate on `[]Transaction`. They return multiple values from a single pass and cannot be expressed as DataFrame chains or single-value PerformanceMetrics. They stay as unexported helpers used by the trade and tax metrics respectively.

**Migration mapping** -- what metrics use instead of the deleted helpers:

| Old helper call | New DataFrame chain |
|---|---|
| `returns(eq)` | `eq.Pct().Drop(math.NaN())` |
| `windowSlice(series, times, window)` | `a.perfData.Window(window)` |
| `windowSliceTimes(times, window)` | `a.perfData.Window(window).Times()` |
| `excessReturns(r, rf)` | `eqReturns.Sub(rfReturns, data.PortfolioRiskFree)` |
| `mean(x)` | `df.Mean().Value(portfolioAsset, metric)` |
| `variance(x)` | `df.Variance().Value(portfolioAsset, metric)` |
| `stddev(x)` | `df.Std().Value(portfolioAsset, metric)` |
| `covariance(x, y)` | `df.Covariance(assets...)` |
| `cagr(start, end, years)` | inline in CAGR metric |
| `drawdownSeries(eq)` | `peak := eq.CumMax(); eq.Sub(peak).Div(peak)` |
| `periodCutoff(last, window)` | `window.Before(last)` (Period method in data package) |

**NaN handling**: `Pct()` returns a DataFrame of the same length with NaN in position 0 (unlike the old `returns()` which returned a slice one element shorter). Downstream aggregations (`Mean()`, `Std()`, `Variance()`) use gonum functions that propagate NaN. Callers must chain `.Drop(math.NaN())` after `Pct()` to remove the leading NaN row. The existing `Drop()` method already handles NaN correctly via `floats.HasNaN`.

**Division by zero**: `Pct()` computes `(col[i] - col[i-1]) / col[i-1]`. When `col[i-1]` is zero, this produces `+Inf` or `NaN`. `PortfolioEquity` is never zero (initial deposit is nonzero). `PortfolioBenchmark` and `PortfolioRiskFree` are zero when those assets are not configured, but metrics that use them guard with `ErrNoRiskFreeRate` / `ErrNoBenchmark` before reaching `Pct()`.

### 8. DataFrame error model in metric chains

DataFrame uses a monadic error model: each DataFrame carries an `err` field. When a method encounters an error, it returns `WithErr(err)` -- a zero-value DataFrame carrying the error. All subsequent chained methods short-circuit immediately, returning `WithErr(df.err)`. Terminal accessors return safe defaults: `Value()` returns `math.NaN()`, `Column()` returns `nil`, `Times()` returns `nil`, `Len()` returns `0`.

Callers check `df.Err()` at the end of a chain. For performance metrics, an error DataFrame from the chain produces NaN from `Value()`, which the metric should detect and return as an error:

```go
// Example: checking for chain errors
sd := er.Std().Value(portfolioAsset, data.PortfolioEquity)
if math.IsNaN(sd) {
    if err := er.Err(); err != nil {
        return 0, err
    }
}
```

The new methods (`Window`, `CumMax`, broadcast `Sub`) all follow this same pattern: check `df.err` first, propagate via `WithErr`.

### 9. How existing metrics change

Representative example -- Sharpe ratio.

Before:
```go
func (sharpe) Compute(a *Account, window *Period) (float64, error) {
    if len(a.RiskFreePrices()) == 0 {
        return 0, ErrNoRiskFreeRate
    }
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

After:
```go
func (sharpe) Compute(a *Account, window *Period) (float64, error) {
    pd := a.PerfData()
    if pd == nil {
        return 0, nil
    }

    // Guard: check risk-free is configured. PortfolioRiskFree is always
    // present as a column but its values are zero when no risk-free asset
    // is set. A risk-free instrument (money market, T-bills) never has a
    // price of zero, so rfCol[0] == 0 reliably detects "not configured".
    rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
    if len(rfCol) == 0 || rfCol[0] == 0 {
        return 0, ErrNoRiskFreeRate
    }

    perfDF := pd.Window(window)
    returns := perfDF.Pct().Drop(math.NaN())

    er := returns.Metrics(data.PortfolioEquity).Sub(returns, data.PortfolioRiskFree)
    if err := er.Err(); err != nil {
        return 0, err
    }

    sd := er.Std().Value(portfolioAsset, data.PortfolioEquity)
    if sd == 0 {
        return 0, nil
    }

    // Intentional: use windowed times for annualization factor, not full
    // history. The annualization factor should reflect the frequency of
    // the data being analyzed, which may differ in a windowed subset.
    af := annualizationFactor(perfDF.Times())
    m := er.Mean().Value(portfolioAsset, data.PortfolioEquity)
    return m / sd * math.Sqrt(af), nil
}
```

Metrics use the `PerfData()` accessor (not the unexported `perfData` field) so the same pattern works for both internal and external `PerformanceMetric` implementations.

Key differences from old pattern:
- Guard checks `PerfData()` for risk-free data instead of `len(a.RiskFreePrices())`
- `Window(window)` replaces `windowSlice`
- `Pct().Drop(math.NaN())` replaces `returns()`
- Variadic `Sub(returns, data.PortfolioRiskFree)` handles metric name mismatch (PortfolioEquity vs PortfolioRiskFree)
- `Mean().Value(...)` and `Std().Value(...)` extract scalars from aggregate DataFrames
- `er.Err()` checks for DataFrame chain errors before using results
- `annualizationFactor` uses windowed times (intentional behavioral change from old code which used full history)

All metric files that reference `EquityCurve()`, `EquityTimes()`, `BenchmarkPrices()`, or `RiskFreePrices()` follow this pattern. Metrics that use only `roundTrips` or `realizedGains` (trade and tax metrics like `win_rate.go`, `stcg.go`, `ltcg.go`) do not need DataFrame chain changes -- they only need to stop calling `EquityCurve()`/`EquityTimes()` if they do.

### 10. Interface changes

**PortfolioSnapshot** (`portfolio/snapshot.go`): Replace `EquityCurve()`, `EquityTimes()`, `BenchmarkPrices()`, `RiskFreePrices()` with `PerfData() *data.DataFrame`. The remaining methods (`Cash()`, `Holdings()`, `Transactions()`, `TaxLots()`, `Metrics()`, `AllMetadata()`) are unchanged.

`WithPortfolioSnapshot` deep-copies the snapshot's perfData via `perfData.Copy()` to prevent shared mutation between the snapshot source and the new Account.

**Portfolio** (`portfolio/portfolio.go`): Replace `EquityCurve()`, `EquityTimes()` with `PerfData() *data.DataFrame`. This is a breaking change to the public `Portfolio` interface. Since `Account` is the only implementor and all callers are internal to this repo, the impact is contained.

### 11. SQLite serialization

The `equity_curve` and `price_series` tables are replaced by a single `perf_data` table:

```sql
CREATE TABLE perf_data (
    date      TEXT NOT NULL,
    metric    TEXT NOT NULL,
    value     REAL NOT NULL
);
CREATE INDEX idx_perf_data_date_metric ON perf_data(date, metric);
```

The asset is always `portfolioAsset` so it is not stored. Each row is a (date, metric, value) triple. Metrics are `PortfolioEquity`, `PortfolioBenchmark`, `PortfolioRiskFree`.

`ToSQLite` writes `perfData` by iterating timestamps and columns. `FromSQLite` queries ordered by `(date, metric)`, collects all timestamps and values, and reconstructs `perfData` as a DataFrame with `portfolioAsset` and the three metrics.

Schema version bumps to `"2"`. `FromSQLite` rejects version `"1"` databases (no automatic migration -- existing version "1" files can be regenerated by re-running the backtest).

The transient `prices` field is not serialized.

### 12. Files deleted

| File | Reason |
|------|--------|
| `portfolio/export_test.go` | Exposes internals; violates black-box testing |
| `portfolio/metric_helpers_test.go` | Tests internals via export shim; behavior tested through PerformanceMetric tests |

### 13. Files modified

| File | Change |
|------|--------|
| `portfolio/period.go` | Replace contents with type alias and constructor aliases pointing to `data` package |
| `data/period.go` | New file: `Period` type, `Days()`, `Months()`, `Years()`, `YTD()`, `MTD()`, `WTD()` constructors, `Period.Before()` method, unit constants |
| `data/data_frame.go` | Add `Window(*Period)`, `CumMax()`, `AppendRow()`; extend `Sub`/`Add`/`Mul`/`Div` with variadic `metrics ...Metric` |
| `data/stats.go` | Delete `AnnualizationFactor` (file becomes empty and can be deleted) |
| `data/stats_test.go` | Delete `AnnualizationFactor` tests (file becomes empty and can be deleted) |
| `portfolio/account.go` | Replace four slices with `perfData *data.DataFrame`; add `PerfData()`; add `portfolioAsset` var; rewrite `UpdatePrices` with `AppendRow`; remove `EquityCurve()`, `EquityTimes()`, `BenchmarkPrices()`, `RiskFreePrices()` |
| `portfolio/snapshot.go` | Change `PortfolioSnapshot` interface to use `PerfData()`; deep-copy in `WithPortfolioSnapshot` |
| `portfolio/portfolio.go` | Change `Portfolio` interface to use `PerfData()` |
| `portfolio/metric_helpers.go` | Delete all helpers except `roundTrips`, `realizedGains`, `annualizationFactor` |
| `portfolio/metric_query.go` | Update `PerformanceMetric` interface doc to reference `perfData` instead of equity curve |
| `portfolio/sqlite.go` | Replace `equity_curve`/`price_series` with `perf_data` table; bump schema to "2" |
| `portfolio/sharpe.go` | Use DataFrame chains (representative of pattern) |
| `portfolio/sortino.go` | Use DataFrame chains |
| `portfolio/max_drawdown.go` | Use `CumMax()` for drawdown |
| All other metric files referencing old accessors | Replace helper calls with DataFrame chains |
| All corresponding test files for modified metrics | Update assertions from old accessors to `PerfData()` |
| `portfolio/account_test.go` | Update to use `PerfData()` |
| `portfolio/sqlite_test.go` | Update for new schema |
| `portfolio/return_metrics_test.go` | Update `EquityCurve()` assertions to `PerfData()` |
| `engine/backtest_test.go` | Update `EquityCurve()` assertion to `PerfData()` |

### 14. Testing strategy

All changes tested through existing PerformanceMetric black-box ginkgo tests. New DataFrame methods (`Window`, `CumMax`, `AppendRow`, extended `Sub`) get tests in `data/data_frame_test.go`, including error propagation tests verifying the monadic error model. `Period.Before` gets tests in `data/period_test.go`.
