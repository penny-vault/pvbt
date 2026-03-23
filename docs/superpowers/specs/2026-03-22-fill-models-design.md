# Fill Models Design Spec

**Issue:** #8 -- Additional simulated broker implementations
**Date:** 2026-03-22

## Problem

The current `SimulatedBroker` fills every market order at the closing price. Real trading involves slippage, bid-ask spreads, and market impact from large orders. Strategy authors need configurable fill realism for more accurate backtesting.

## Solution

A composable `FillModel` abstraction plugged into the existing `SimulatedBroker`. Fill models are split into two categories:

- **Base price models** produce the starting execution price (e.g., close, VWAP)
- **Adjusters** modify that price (e.g., spread cost, market impact, slippage)

Strategy authors compose them in a pipeline:

```go
engine.NewSimulatedBroker(
    broker.WithFillModel(
        fill.VWAP(),
        fill.SpreadAware(fill.SpreadBPS(10)),
        fill.MarketImpact(fill.SmallCap),
    ),
)
```

If no fill model is configured, behavior is unchanged (fill at close).

## Core Types

### FillResult

```go
type FillResult struct {
    Price    float64
    Partial  bool
    Quantity float64
}
```

`Partial` is true when a model (e.g., market impact) determines the order cannot be fully filled. `Quantity` is the actual filled amount, which may be less than requested.

### FillModel Interface

```go
type FillModel interface {
    Fill(order broker.Order, bar *data.DataFrame) (FillResult, error)
}
```

### Pipeline

The pipeline is itself a `FillModel`. It calls the base model first, then feeds its `FillResult` through each adjuster in sequence. Each adjuster receives the current result and the original bar data. If any model returns an error, the order is rejected (not silently degraded).

### DataFetcher

Models that need additional data beyond the current bar (e.g., VWAP requesting intraday bars) use a `DataFetcher` interface:

```go
type DataFetcher interface {
    FetchAt(ctx context.Context, assets []asset.Asset,
        timestamp time.Time, metrics []data.Metric) (*data.DataFrame, error)
}
```

The `SimulatedBroker` injects this when it sets the price provider. Models that don't need it ignore it.

## Base Price Models

### CloseFill (default)

Fills at the bar's close price. Equivalent to current behavior.

### VWAPFill

Estimates volume-weighted average price. Data source priority:

1. Intraday bars via `DataFetcher` if available -- computes true VWAP from intraday OHLCV
2. Approximates as `(High + Low + Close) / 3` (typical price) from the daily bar when intraday data is unavailable

## Adjusters

### SpreadAware

Applies half-spread cost directionally: buys fill at `price + halfSpread`, sells at `price - halfSpread`.

Spread source priority:
1. Real bid/ask from the bar (`MetricBid` / `MetricAsk`) if present
2. Configured estimate via `SpreadBPS(bps int)` option
3. Error if neither is available

### MarketImpact

Square-root impact model: `impact = coefficient * sqrt(orderShares / barVolume)`.

- Buys: `price * (1 + impact)`
- Sells: `price * (1 - impact)`
- If order volume exceeds a threshold relative to bar volume, the fill is partial

Named presets with bundled coefficient and volume threshold:
- `LargeCap` -- low coefficient, high volume threshold
- `SmallCap` -- moderate coefficient, moderate threshold
- `MicroCap` -- high coefficient, low threshold

### SlippageFill

Configurable fixed or percentage slippage, applied directionally (increases cost for buys, decreases proceeds for sells).

- `Slippage(fill.Percent(0.1))` -- percentage-based
- `Slippage(fill.Fixed(0.05))` -- fixed dollar amount

## Package Layout

New package `fill` at the project root. Keeps fill logic separate from `broker` (interface definitions) and `engine` (SimulatedBroker implementation).

```
fill/
    fill.go          # FillModel, FillResult, Pipeline, DataFetcher
    close.go         # CloseFill
    vwap.go          # VWAPFill
    spread.go        # SpreadAware
    impact.go        # MarketImpact with presets
    slippage.go      # SlippageFill
```

## Changes to SimulatedBroker

1. Add `fillModel FillModel` field, defaulting to `fill.Close()`
2. Add `WithFillModel(base FillModel, adjusters ...FillModel) Option` constructor option
3. In `Submit`, replace hardcoded close-price lookup with `fillModel.Fill(order, bar)`
4. Handle partial fills from `FillResult`: emit a partial fill event and keep the remainder as a pending order
5. After the price provider is set, call `SetDataFetcher` on the pipeline so VWAP and other models can request additional data

## Error Handling

- If a fill model returns an error, the order is rejected. Errors are never silently swallowed.
- SpreadAware with no bid/ask data and no configured fallback returns an error.
- MarketImpact with no volume data returns an error.
- VWAP gracefully degrades to typical price when intraday data is unavailable (this is an approximation, not a silent failure -- the data is still valid daily OHLC).
