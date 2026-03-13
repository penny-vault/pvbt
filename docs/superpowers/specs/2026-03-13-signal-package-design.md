# Signal Package Implementation Design

**Date:** 2026-03-13
**Status:** Draft

## Overview

Implement the `signal` package: three reusable signal functions (Momentum, Volatility, EarningsYield) that fetch data from a universe and return computed DataFrames. This requires supporting changes to DataFrame (error accumulation for chaining), the engine (look-ahead guard), and the universe interface (CurrentDate).

## 1. DataFrame Error Accumulation

### Problem

`Add`/`Sub`/`Mul`/`Div` return `(*DataFrame, error)`, preventing fluent chaining like `mom1.Add(mom3).Add(mom6).DivScalar(3)`.

### Design

Add an `err error` field to `DataFrame` following the `bufio.Scanner` pattern:

```go
type DataFrame struct {
    // ... existing fields ...
    err error
}

func (df *DataFrame) Err() error { return df.err }
```

**Behavioral rules:**

- `Add`, `Sub`, `Mul`, `Div` change from returning `(*DataFrame, error)` to returning `*DataFrame`. On failure, the returned DataFrame carries the error in `.err`.
- Every chainable method (`Add`, `Sub`, `Mul`, `Div`, `Pct`, `Apply`, `Rolling`, `Metrics`, `Assets`, `DivScalar`, `MulScalar`, `AddScalar`, `SubScalar`, etc.) checks `df.err` at entry. If set, it returns a new DataFrame carrying the same error without doing work.
- `RollingDataFrame` methods (`Std`, `Mean`, `Sum`, `Max`, `Min`, `Variance`, `Percentile`) check the underlying DataFrame's `err` field at entry. If set, they return a DataFrame carrying the same error without computing.
- `NewDataFrame` continues to return `(*DataFrame, error)` for non-chained construction use.
- A `withErr(err error) *DataFrame` private helper returns `&DataFrame{err: err}` (zero-value struct with error set). All accessor methods (`Len()`, `AssetList()`, etc.) return safe zero values on this form.

**Caller updates:**

All existing callers of `Add`/`Sub`/`Mul`/`Div` that destructure `(df, err)` must be updated to use the new single-return signature and check `.Err()` where needed.

## 2. Engine Look-Ahead Guard

### Problem

Nothing prevents strategies from requesting data beyond the current simulation date, allowing look-ahead bias.

### Design

**`Fetch` (range-based):** No guard needed. `Fetch` already sets `rangeEnd := e.currentDate`, so the range cannot exceed the current date by construction.

**`FetchAt` (point-in-time):** If the requested `time.Time` is after `e.currentDate`, return an error. This is a clear programmer mistake.

The guard lives in `engine/engine.go`. This protects point-in-time data access since `Universe.At` delegates through `DataSource.FetchAt`.

## 3. Universe Interface Changes

### Problem

`EarningsYield` needs the current simulation date when no explicit time is provided. The universe has access to it via `DataSource.CurrentDate()` but does not expose it.

### Design

Add `CurrentDate() time.Time` to the `Universe` interface:

```go
type Universe interface {
    Assets(t time.Time) []asset.Asset
    Prefetch(ctx context.Context, start, end time.Time) error
    Window(ctx context.Context, lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error)
    At(ctx context.Context, t time.Time, metrics ...data.Metric) (*data.DataFrame, error)
    CurrentDate() time.Time
}
```

This expands the interface to 5 methods (above the project's 1-4 method convention). This is justified because `CurrentDate` is inherent to a universe's simulation context and avoids threading `time.Time` through every signal that needs it.

Existing implementations (e.g., `StaticUniverse`) delegate to `u.dataSource.CurrentDate()`.

## 4. DataFrame RenameMetric

### Problem

Signals need to rename the output metric (e.g., `MetricClose` becomes `"Momentum"`). No rename method exists.

### Design

Add `RenameMetric(old, new Metric) *DataFrame` to DataFrame. Returns a new DataFrame with the metric name replaced. Participates in error accumulation (short-circuits if `df.err` is set). If `new` already exists in the DataFrame, returns a DataFrame with `.Err()` set (duplicate metrics would be a bug). If `old` is not found, returns a DataFrame with `.Err()` set.

## 5. Signal Functions

### Package

`signal` (existing package at `signal/`)

### Signal Constants

Signal output names are defined in the signal package, not in `data/metric.go`, to keep the boundary between raw data and computed signals clear. They use a `Signal` suffix to avoid colliding with the function names:

```go
const (
    MomentumSignal      data.Metric = "Momentum"
    VolatilitySignal    data.Metric = "Volatility"
    EarningsYieldSignal data.Metric = "EarningsYield"
)
```

### Signatures

```go
func Momentum(ctx context.Context, u universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame

func Volatility(ctx context.Context, u universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame

func EarningsYield(ctx context.Context, u universe.Universe, t ...time.Time) *data.DataFrame
```

### Momentum

1. Resolve metric: use first of `metrics`, default `MetricClose`.
2. Fetch: `u.Window(ctx, period, metric)`.
3. On fetch error: return `withErr(err)`.
4. Validate: if `df.Len() < 2`, return `withErr` -- not enough data to compute momentum.
5. Compute: `df.Pct(df.Len() - 1).Last()` -- percent change over the full window, reduced to the final row. Note: `Pct(n)` produces NaN for the first `n` rows; only the last row has a valid value, which `.Last()` extracts.
6. Rename output metric to `MomentumSignal`.
7. Return result; any chaining errors propagate via `.Err()`.

### Volatility

1. Resolve metric: use first of `metrics`, default `MetricClose`.
2. Fetch: `u.Window(ctx, period, metric)`.
3. On fetch error: return `withErr(err)`.
4. Validate: if `df.Len() < 3`, return `withErr` -- need at least 3 prices to produce 2 returns for a meaningful std dev.
5. Compute step by step (not a single chain, to handle the NaN from `Pct`):
   - `returns := df.Pct(1).Drop(math.NaN())` -- daily returns with leading NaN removed.
   - `result := returns.Rolling(returns.Len()).Std().Last()` -- rolling std dev over all returns, reduced to final row.
6. Rename output metric to `VolatilitySignal`.
7. Return result.

### EarningsYield

1. Resolve time: use first of `t`, default `u.CurrentDate()`.
2. Fetch: `u.At(ctx, resolvedTime, data.EarningsPerShare, data.Price)`.
3. On fetch error: return `withErr(err)`.
4. Validate: check that both `EarningsPerShare` and `Price` metrics are present in the fetched DataFrame (via `ColCount` or similar). If either is missing, return `withErr` with a descriptive error.
5. Compute manually: for each asset, extract the `EarningsPerShare` and `Price` values, divide, and build a new DataFrame with the results. This avoids the `Div` metric-intersection problem (two DataFrames with different metrics would produce an empty intersection).
6. Output DataFrame has metric `EarningsYieldSignal` and the same assets/timestamps.
7. Return result.

## 6. Testing

### DataFrame error accumulation (`data/data_frame_test.go`)

- `Add`/`Sub`/`Mul`/`Div` return correct values on success (update existing tests for new signature).
- Error propagation: once `.Err()` is set, subsequent operations preserve it.
- End-to-end chaining: `a.Add(b).MulScalar(2).Err()` is nil on success, non-nil on failure.
- `RollingDataFrame` propagates error from underlying DataFrame.

### DataFrame RenameMetric (`data/data_frame_test.go`)

- Renames a metric successfully.
- Returns error when old metric not found.
- Returns error when new metric already exists.
- Propagates existing `.Err()`.

### Engine look-ahead guard (`engine/` test files)

- `FetchAt` with a future date returns an error.
- `Fetch` with a range extending past `currentDate` clamps to `currentDate`.

### Universe CurrentDate (`universe/universe_test.go`)

- Verify `CurrentDate()` delegates to the data source.

### Signal tests (one file per signal)

**`signal/momentum_test.go`:**
- Hand-computed percent change against known price data.
- Default metric (`MetricClose`) vs custom metric override.
- Degenerate window (fewer than 2 rows) sets `.Err()`.
- Fetch error propagates to `.Err()`.

**`signal/volatility_test.go`:**
- Hand-computed rolling std dev against known return data.
- Default metric vs custom metric override.
- Degenerate window (fewer than 3 rows) sets `.Err()`.
- Fetch error propagates to `.Err()`.

**`signal/earnings_yield_test.go`:**
- Hand-computed EPS/Price division.
- Default time (current date) vs explicit time.
- Missing `EarningsPerShare` or `Price` metric sets `.Err()`.
- Fetch error propagates to `.Err()`.

All tests use Ginkgo/Gomega.

## 7. Documentation Updates

Update `docs/overview.md`:
- Fix the `Compute` example to pass `ctx` to signal functions.
- Fix the `Select` call direction to match actual code: `portfolio.MaxAboveZero(s.RiskOff).Select(momentum)`.
- Show `.Err()` checking after signal chains.

Update `docs/data.md`:
- Signal examples to include `ctx` parameter.
- Show `.Err()` checking pattern.
