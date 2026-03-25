# Volume Signals Design Spec

**Issue:** #25
**Date:** 2026-03-25

## Summary

Add five built-in volume-based signals to the `signal` package: OBV, VWMA, Accumulation/Distribution, Chaikin Money Flow, and Money Flow Index. These signals confirm whether price moves have conviction behind them and detect accumulation/distribution patterns.

VWAP was in the original issue but is replaced by VWMA, which is meaningful across all bar frequencies (daily through intraday).

## File Organization

One file per signal, consistent with the oscillator signals (CCI, Stochastic, Williams R):

| Signal | File | Test File |
|--------|------|-----------|
| OBV | `signal/obv.go` | `signal/obv_test.go` |
| VWMA | `signal/vwma.go` | `signal/vwma_test.go` |
| Accumulation/Distribution | `signal/accumulation_distribution.go` | `signal/accumulation_distribution_test.go` |
| CMF | `signal/cmf.go` | `signal/cmf_test.go` |
| MFI | `signal/mfi.go` | `signal/mfi_test.go` |

Each file contains its own metric constant (e.g., `OBVSignal`), consistent with how `CCISignal` lives in `cci.go`.

## Signal Signatures

All signals follow the established pattern: plain functions taking a context, universe, and period, returning `*data.DataFrame`.

```go
func OBV(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame
func VWMA(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame
func AccumulationDistribution(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame
func CMF(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame
func MFI(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame
```

Each returns a single-row DataFrame (the signal value at the end of the window), one column per asset.

## Output Metrics

Each signal defines its output metric constant in its own file:

| Signal | Constant | Type | Value Range |
|--------|----------|------|-------------|
| OBV | `OBVSignal` | `data.Metric` | Unbounded (cumulative) |
| VWMA | `VWMASignal` | `data.Metric` | Price units |
| A/D | `AccumulationDistributionSignal` | `data.Metric` | Unbounded (cumulative) |
| CMF | `CMFSignal` | `data.Metric` | [-1, 1] |
| MFI | `MFISignal` | `data.Metric` | [0, 100] |

## Formulas

### OBV (On-Balance Volume)

**Data requirements:** Close, Volume over `period`

**Algorithm:**
1. Fetch Close and Volume via `u.Window(ctx, period, data.MetricClose, data.Volume)`
2. For each bar after the first: if close > prev close, add volume to running total; if close < prev close, subtract volume; if equal, add nothing
3. Return the final cumulative value

### VWMA (Volume-Weighted Moving Average)

**Data requirements:** Close, Volume over `period`

**Algorithm:**
1. Fetch Close and Volume via `u.Window(ctx, period, data.MetricClose, data.Volume)`
2. Compute Sum(Close * Volume) / Sum(Volume) over the period window
3. Return the VWMA value

**Edge case:** Zero total volume yields NaN.

### Accumulation/Distribution

**Data requirements:** High, Low, Close, Volume over `period`

**Algorithm:**
1. Fetch via `u.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume)`
2. Money Flow Multiplier = ((Close - Low) - (High - Close)) / (High - Low)
3. Money Flow Volume = MFM * Volume
4. A/D = running sum of Money Flow Volume
5. Return the final cumulative value

**Edge case:** High == Low yields MFM = 0 (avoids division by zero).

### CMF (Chaikin Money Flow)

**Data requirements:** High, Low, Close, Volume over `period`

**Algorithm:**
1. Fetch via `u.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume)`
2. Compute Money Flow Multiplier and Money Flow Volume per bar (same as A/D)
3. CMF = Sum(Money Flow Volume) / Sum(Volume) over the period
4. Return the CMF value

**Edge cases:** Zero total volume yields NaN. High == Low yields MFM = 0.

### MFI (Money Flow Index)

**Data requirements:** High, Low, Close, Volume over `period` (needs `period.N + 1` bars for TP comparison)

**Algorithm:**
1. Fetch via `u.Window(ctx, adjustedPeriod, data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume)` where the period is extended by 1 to get the prior bar
2. Typical Price = (High + Low + Close) / 3
3. Raw Money Flow = TP * Volume
4. For each bar: if TP > prev TP, add to positive flow sum; if TP < prev TP, add to negative flow sum; if equal, discard
5. Money Flow Ratio = positive sum / negative sum
6. MFI = 100 - (100 / (1 + ratio))
7. Return the MFI value

**Edge cases:** All negative flows (ratio = 0) yields MFI = 0. All positive flows (negative sum = 0) yields MFI = 100.

## Error Handling

Consistent with existing signals:

- If `u.Window()` returns an error, wrap with context (e.g., `fmt.Errorf("OBV: %w", err)`) and return via `data.WithErr()`
- If the DataFrame has fewer rows than needed (e.g., MFI needs `period.N + 1` bars, all others need at least 2), return an error via `data.WithErr()`
- Division by zero (zero volume, High == Low) produces NaN for the affected asset, not an error -- this matches how existing signals handle flat/degenerate markets

## Testing

One test file per signal, using the existing Ginkgo/Gomega setup and `mockDataSource` from `helpers_test.go`.

Standard test cases for each signal:

1. **Happy path** -- hand-calculated expected values with known data
2. **Multi-asset independence** -- two assets computed correctly in the same call
3. **Minimum data** -- exactly enough rows for computation
4. **Insufficient data** -- fewer rows than required, returns error
5. **Flat market / zero volume** -- NaN or appropriate edge case behavior
6. **Fetch error propagation** -- mock returns error, signal wraps and propagates it

## Documentation Updates

### `signal/doc.go`

Add volume signals to the Built-in Signals list:

```
//   - [OBV](ctx, u, period): On-Balance Volume (cumulative).
//   - [VWMA](ctx, u, period): Volume-Weighted Moving Average.
//   - [AccumulationDistribution](ctx, u, period): Accumulation/Distribution line (cumulative).
//   - [CMF](ctx, u, period): Chaikin Money Flow (-1 to 1).
//   - [MFI](ctx, u, period): Money Flow Index (0 to 100).
```

### `docs/signals.md`

Add a new "Volume" section after "Oscillators" with entries for all five signals following the existing format (description, code example, signature, parameters, output metric, value range).

Add the five signals to the overview table.

### `CHANGELOG.md`

Add a single entry under `[Unreleased] > Added`:

```
- Volume signals (OBV, VWMA, Accumulation/Distribution, Chaikin Money Flow, and Money Flow Index) confirm price moves and detect accumulation/distribution patterns.
```
