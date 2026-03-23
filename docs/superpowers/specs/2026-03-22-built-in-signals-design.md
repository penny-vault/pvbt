# Built-in Signals Design

**Issue:** #21
**Date:** 2026-03-22

## Summary

Add five technical indicator signals (RSI, MACD, Bollinger Bands, moving average crossovers, ATR) to the `signal` package as thin wrappers over DataFrame operations. Add EMA as a new public method on `RollingDataFrame` to support these indicators.

## EMA on RollingDataFrame

`RollingDataFrame` gains one new method:

```go
func (r *RollingDataFrame) EMA() *DataFrame
```

Smoothing factor: `alpha = 2 / (n + 1)` where `n` is the window size from `Rolling(n)`. The first `n-1` rows are NaN, matching the behavior of `Rolling(n).Mean()`. Seeded with the SMA of the first `n` values. Each column is computed independently via `Apply()`.

## Signal Functions

All signals follow the existing pattern: plain functions in the `signal` package that take `context.Context`, `universe.Universe`, and signal-specific parameters, returning `*data.DataFrame`. Errors are propagated via `data.WithErr()`.

### RSI

```go
func RSI(ctx context.Context, u universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame
```

Computes percentage changes, separates gains and losses, applies Wilder's smoothing (`alpha = 1/n`, not standard EMA) via `Apply()` over `period`, returns values 0-100. Output metric: `RSISignal`.

### MACD

```go
func MACD(ctx context.Context, u universe.Universe, fast, slow, signalPeriod portfolio.Period, metrics ...data.Metric) *data.DataFrame
```

Fast EMA minus slow EMA produces MACD line. EMA of MACD line produces signal line. Difference produces histogram. Output metrics: `MACDLineSignal`, `MACDSignalLineSignal`, `MACDHistogramSignal`.

### Bollinger Bands

```go
func BollingerBands(ctx context.Context, u universe.Universe, period portfolio.Period, numStdDev float64, metrics ...data.Metric) *data.DataFrame
```

Middle band = `Rolling(n).Mean()`. Upper/lower = middle +/- `numStdDev * Rolling(n).Std()` where `n` is the number of rows in the window returned by `u.Window(ctx, period)`. Output metrics: `BollingerUpperSignal`, `BollingerMiddleSignal`, `BollingerLowerSignal`.

### Crossover

```go
func Crossover(ctx context.Context, u universe.Universe, fastPeriod, slowPeriod portfolio.Period, metrics ...data.Metric) *data.DataFrame
```

Computes SMA for both periods. Crossover indicator: +1 when fast > slow, -1 when fast < slow. Output metrics: `CrossoverFastSignal`, `CrossoverSlowSignal`, `CrossoverSignal`.

### ATR

```go
func ATR(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame
```

No `metrics` parameter -- always uses High, Low, Close. True Range = max(high-low, |high-prevClose|, |low-prevClose|). Smoothed with Wilder's method (`alpha = 1/n`) via full-column `Apply()`, same as RSI. Output metric: `ATRSignal`.

## Constants

Added to `signal/signal.go`:

```go
RSISignal             data.Metric = "RSI"
ATRSignal             data.Metric = "ATR"
MACDLineSignal        data.Metric = "MACDLine"
MACDSignalLineSignal  data.Metric = "MACDSignalLine"
MACDHistogramSignal   data.Metric = "MACDHistogram"
BollingerUpperSignal  data.Metric = "BollingerUpper"
BollingerMiddleSignal data.Metric = "BollingerMiddle"
BollingerLowerSignal  data.Metric = "BollingerLower"
CrossoverFastSignal   data.Metric = "CrossoverFast"
CrossoverSlowSignal   data.Metric = "CrossoverSlow"
CrossoverSignal       data.Metric = "Crossover"
```

## File Organization

Each signal gets its own file following the existing pattern:

- `signal/rsi.go` + `signal/rsi_test.go`
- `signal/macd.go` + `signal/macd_test.go`
- `signal/bollinger.go` + `signal/bollinger_test.go`
- `signal/crossover.go` + `signal/crossover_test.go`
- `signal/atr.go` + `signal/atr_test.go`

EMA is added to `data/rolling_data_frame.go` with tests in `data/rolling_data_frame_test.go`.

`signal/doc.go` is updated to list all eight built-in signals.

## Error Handling

Each function calls `u.Window()` to fetch the required lookback. If the returned DataFrame has fewer rows than needed or `df.Err()` is set, the function returns `data.WithErr(fmt.Errorf("SignalName: %w", err))`.

ATR fetches three metrics (High, Low, Close) in its `Window()` call. All others fetch one metric (defaulting to Close).

## Testing

Tests use Ginkgo/Gomega with `mockDataSource` from `helpers_test.go`.

**Each signal tests:**
1. Correctness against hand-computed values from known input data
2. Custom metric override (except ATR which always uses High/Low/Close)
3. Insufficient data returns error via `df.Err()`
4. Data fetch error propagation using `errorDataSource`

**ATR additionally tests** that True Range handles each of the three TR cases (high-low dominant, |high-prevClose| dominant, |low-prevClose| dominant).

**EMA tests** (in `data/rolling_data_frame_test.go`):
1. Values against hand-computed results
2. First `n-1` rows are NaN
3. Single-column and multi-column DataFrames
4. Window size of 1 (EMA equals the input)

## Design Decisions

- **`portfolio.Period` for all lookback parameters:** Matches the existing signal convention. `Window()` handles calendar-to-bar conversion; signals use the row count of the returned DataFrame for rolling window sizes.
- **Textbook defaults only:** RSI uses Wilder's smoothing (`alpha = 1/n`), MACD uses standard EMA (`alpha = 2/(n+1)`), Bollinger uses SMA. No option to swap. Non-standard variants can be composed from DataFrame primitives.
- **RSI and ATR use Wilder's smoothing, not `Rolling(n).EMA()`:** Wilder's smoothing factor (`alpha = 1/n`) differs from standard EMA (`alpha = 2/(n+1)`). Both RSI and ATR compute it via full-column `Apply()` with internal running state, rather than calling the `EMA()` method.
- **Explicit parameters, no hidden defaults:** All lookback periods are required arguments.
- **Multi-output signals return multi-metric DataFrames:** MACD, Bollinger Bands, and Crossover each return a single DataFrame with multiple metric columns rather than separate functions.
- **EMA is a public DataFrame operation:** Added to `RollingDataFrame` so strategy authors can use it directly, keeping signal implementations thin.
