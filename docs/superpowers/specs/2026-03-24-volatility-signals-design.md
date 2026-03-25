# Volatility Signals: Keltner Channels & Donchian Channels

**Date:** 2026-03-24
**Issue:** #24

## Summary

Add two volatility channel signals to the `signal` package: Keltner Channels and Donchian Channels. Both follow the existing signal function pattern -- standalone functions returning single-row DataFrames with upper/middle/lower band metrics.

As part of this work, relocate all signal metric constants from `signal.go` to their respective signal files.

## Keltner Channels

**File:** `signal/keltner.go`

**Signature:**

```go
func KeltnerChannels(ctx context.Context, assetUniverse universe.Universe,
    period portfolio.Period, atrMultiplier float64, metrics ...data.Metric) *data.DataFrame
```

**Parameters:**
- `period` -- lookback window for both the EMA center line and ATR calculation
- `atrMultiplier` -- band width as a multiple of ATR (conventional default: 2.0)
- `metrics` -- optional price metric for the center line EMA (defaults to Close)

**Computation:**
1. Fetch High, Low, Close (plus optional custom metric) via `assetUniverse.Window`
2. Compute EMA of the price metric over the full window for the center line
3. Compute ATR inline using the same True Range + Wilder smoothing logic as `signal.ATR` (avoids a redundant data fetch)
4. Upper = center + atrMultiplier * ATR
5. Lower = center - atrMultiplier * ATR
6. Return single-row DataFrame with three metrics via `MergeColumns`

**Constants (defined in `keltner.go`):**
```go
KeltnerUpperSignal  data.Metric = "KeltnerUpper"
KeltnerMiddleSignal data.Metric = "KeltnerMiddle"
KeltnerLowerSignal  data.Metric = "KeltnerLower"
```

**Error conditions:**
- Fewer than 2 data points
- Context cancellation / data fetch errors

## Donchian Channels

**File:** `signal/donchian.go`

**Signature:**

```go
func DonchianChannels(ctx context.Context, assetUniverse universe.Universe,
    period portfolio.Period) *data.DataFrame
```

**Parameters:**
- `period` -- lookback window for the rolling max/min

No optional metrics parameter -- Donchian Channels are defined strictly on High/Low.

**Computation:**
1. Fetch High and Low via `assetUniverse.Window`
2. Upper = rolling max of High over the full window (`.Rolling(n).Max().Last()`)
3. Lower = rolling min of Low over the full window (`.Rolling(n).Min().Last()`)
4. Middle = (Upper + Lower) / 2 via DataFrame arithmetic (`.Add(lower).DivScalar(2)`)
5. Return single-row DataFrame with three metrics via `MergeColumns`

**Constants (defined in `donchian.go`):**
```go
DonchianUpperSignal  data.Metric = "DonchianUpper"
DonchianMiddleSignal data.Metric = "DonchianMiddle"
DonchianLowerSignal  data.Metric = "DonchianLower"
```

**Error conditions:**
- Fewer than 1 data point
- Context cancellation / data fetch errors

## Constant Relocation

Move all existing signal metric constants from `signal.go` to their respective files:

| Constant | Destination |
|----------|-------------|
| `MomentumSignal` | `momentum.go` |
| `VolatilitySignal` | `volatility.go` |
| `EarningsYieldSignal` | `earnings_yield.go` |
| `RSISignal` | `rsi.go` |
| `ATRSignal` | `atr.go` |
| `MACDLineSignal`, `MACDSignalLineSignal`, `MACDHistogramSignal` | `macd.go` |
| `BollingerUpperSignal`, `BollingerMiddleSignal`, `BollingerLowerSignal` | `bollinger.go` |
| `CrossoverFastSignal`, `CrossoverSlowSignal`, `CrossoverSignal` | `crossover.go` |

Delete `signal.go` after all constants are relocated (assuming no other content remains).

## Testing

**`signal/keltner_test.go`:**
- Hand-crafted price data with known High/Low/Close values
- Verify upper/middle/lower against hand-calculated EMA + ATR values
- Test custom metric parameter
- Test error case: insufficient data points

**`signal/donchian_test.go`:**
- Hand-crafted price data with known High/Low values
- Verify upper = max(High), lower = min(Low), middle = midpoint
- Test error case: insufficient data points

Both test files follow existing Ginkgo patterns with mocked data sources.

## Changelog

**Added:**
- Keltner Channel and Donchian Channel volatility signals
