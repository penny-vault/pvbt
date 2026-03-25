# Oscillator Signals Design Spec

**Issue:** #23 -- Oscillator signals
**Date:** 2026-03-24

## Goal

Add three built-in oscillator signals (Stochastic Oscillator, Williams %R, CCI) to the `signal` package, create a comprehensive signal reference doc, and update existing documentation.

Rate of Change is excluded because it duplicates the existing `signal.Momentum` function.

## New Signal Functions

All oscillator signals follow the established pattern: accept `context.Context`, `universe.Universe`, and configuration parameters; fetch data via `u.Window()`; return a single-row `*data.DataFrame`.

All three oscillators require High, Low, and Close data (same multi-metric fetch pattern as ATR).

### StochasticFast

```go
func StochasticFast(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame
```

Computes the Fast Stochastic Oscillator for each asset.

- %K = (Close - Lowest Low over period) / (Highest High over period - Lowest Low over period) * 100
- %D = 3-period SMA of %K (the 3-period smoothing is the universal convention and is hardcoded)

Output metrics: `StochasticKSignal`, `StochasticDSignal`. Range: 0-100. Division by zero (Highest High == Lowest Low) produces NaN.

Uses `data.MergeColumns` for the two-metric output, following the MACD/Bollinger pattern.

**Data requirements:** Computing %D requires %K values for 3 bars, so the function internally fetches `period.N + 2` bars of data to produce 3 rolling %K values for the SMA. The `period` parameter controls the lookback for %K itself. Minimum data: `period.N + 2` rows.

### StochasticSlow

```go
func StochasticSlow(ctx context.Context, assetUniverse universe.Universe, period, smoothing portfolio.Period) *data.DataFrame
```

Computes the Slow Stochastic Oscillator for each asset.

- Raw %K = same formula as Fast %K
- Slow %K = SMA of Raw %K over `smoothing` bars
- Slow %D = 3-period SMA of Slow %K (hardcoded, same convention as Fast)

Output metrics: `StochasticSlowKSignal`, `StochasticSlowDSignal`. Range: 0-100. Division by zero produces NaN.

The `smoothing` parameter controls the %K smoothing window (commonly `data.Days(3)`). Typed as `portfolio.Period` for consistency with other signal parameters.

**Data requirements:** The function internally fetches `period.N + smoothing.N + 2` bars to produce enough Slow %K values for the 3-period %D SMA. Minimum data: `period.N + smoothing.N + 2` rows.

### WilliamsR

```go
func WilliamsR(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame
```

Computes Williams %R for each asset.

- %R = (Highest High over period - Close) / (Highest High over period - Lowest Low over period) * -100

Output metric: `WilliamsRSignal`. Range: -100 to 0. Division by zero produces NaN.

Fully independent implementation from Stochastic despite the mathematical relationship.

### CCI

```go
func CCI(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame
```

Computes the Commodity Channel Index for each asset.

- Typical Price = (High + Low + Close) / 3
- SMA = simple moving average of Typical Price over period
- Mean Deviation = mean of |Typical Price - SMA| over period
- CCI = (Typical Price - SMA) / (0.015 * Mean Deviation)

Output metric: `CCISignal`. Range: unbounded. The constant 0.015 is hardcoded (Lambert's convention). Division by zero (Mean Deviation == 0) produces NaN.

## New Metric Constants

Added to `signal/signal.go`:

```go
StochasticKSignal     data.Metric = "StochasticK"
StochasticDSignal     data.Metric = "StochasticD"
StochasticSlowKSignal data.Metric = "StochasticSlowK"
StochasticSlowDSignal data.Metric = "StochasticSlowD"
WilliamsRSignal       data.Metric = "WilliamsR"
CCISignal             data.Metric = "CCI"
```

## Implementation Approach

Each signal follows the ATR pattern for multi-metric data access:

1. Fetch High, Low, Close via `assetUniverse.Window(ctx, adjustedPeriod, data.MetricHigh, data.MetricLow, data.MetricClose)` where `adjustedPeriod` accounts for the extra bars needed by SMA smoothing (see per-signal data requirements above)
2. Validate minimum data points (per-signal minimum: Williams %R and CCI need `period.N` rows; StochasticFast needs `period.N + 2`; StochasticSlow needs `period.N + smoothing.N + 2`)
3. Iterate over assets using `df.Column(asset, metric)` for direct `[]float64` access
4. Compute per-asset, store results in column slices
5. Construct result DataFrame via `data.NewDataFrame` for single-metric signals or `data.MergeColumns` for multi-metric signals (Stochastic)

This matches ATR's approach rather than the `df.Apply()` approach used by RSI, because these oscillators need simultaneous access to High, Low, and Close columns per asset.

## Error Handling

- Wrap all errors with signal name context: `fmt.Errorf("SignalName: %w", err)`
- Return `data.WithErr(err)` for immediate errors (window fetch failure, insufficient data)
- Division by zero produces NaN (not an error), consistent with standard floating-point behavior

## File Layout

### New Files

| File | Contents |
|------|----------|
| `signal/stochastic.go` | `StochasticFast` and `StochasticSlow` |
| `signal/williams_r.go` | `WilliamsR` |
| `signal/cci.go` | `CCI` |
| `signal/stochastic_test.go` | Tests for both Stochastic variants |
| `signal/williams_r_test.go` | Tests for Williams %R |
| `signal/cci_test.go` | Tests for CCI |
| `docs/signals.md` | Comprehensive signal reference (all signals) |

### Modified Files

| File | Change |
|------|--------|
| `signal/signal.go` | Add 6 new metric constants |
| `signal/doc.go` | Add new signals to the package doc list |
| `docs/data.md` | Remove signals section (lines 410-456), replace with link to `docs/signals.md` |
| `CHANGELOG.md` | Entry under "Added" |

## Testing

Each test file uses Ginkgo/Gomega following the existing signal test pattern:

1. Construct a DataFrame with known High, Low, Close prices
2. Create a static universe via `universe.NewStaticWithSource`
3. Call the signal function
4. Assert output values against hand-calculated expected results using `Expect(...).To(BeNumerically("~", expected, tolerance))`

Test cases per signal:

- **Happy path:** known input prices with hand-verified expected output
- **Minimum data:** exactly 2 data points
- **Insufficient data:** fewer than 2 data points returns error
- **Flat market:** all prices identical (division-by-zero case produces NaN for Stochastic, Williams %R, and CCI)
- **Multi-asset:** verify per-asset independence
- **StochasticSlow with non-default smoothing:** verify correctness with smoothing values other than 3 (e.g., 5)

## Documentation

### `docs/signals.md` (new)

Comprehensive signal reference covering all signals (existing and new):

- Overview of what signals are and how they work
- Table of all built-in signals with signatures
- Per-signal reference sections: description, parameters, output metrics, value range, typical usage
- Composing and combining signals
- Writing custom signals

Content for existing signals is moved/adapted from `docs/data.md` and `signal/doc.go`.

### `docs/data.md` (modified)

Remove the "Signals" section (lines 410-456) and replace with a brief paragraph linking to `docs/signals.md`.

### Godoc comments

All new exported functions and constants get proper godoc comments following the style of existing signals (see RSI, ATR, BollingerBands for examples).

### `signal/doc.go` (modified)

Add new signals to the "Built-in Signals" list in the package doc comment.

## CHANGELOG

Entry under "Added":

> Stochastic Oscillator (fast and slow), Williams %R, and CCI signals are now available in the signal package for identifying overbought/oversold conditions.
