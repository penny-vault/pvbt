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

### Interfaces

Base price models and adjusters have distinct interfaces reflecting their roles:

```go
type BaseModel interface {
    Fill(ctx context.Context, order broker.Order, bar *data.DataFrame) (FillResult, error)
}

type Adjuster interface {
    Adjust(ctx context.Context, order broker.Order, bar *data.DataFrame, current FillResult) (FillResult, error)
}
```

`BaseModel` produces the initial fill price and quantity. `Adjuster` receives the upstream result and modifies it. Both take `context.Context` for cancellation propagation and downstream data fetches.

### Pipeline

The pipeline composes a `BaseModel` with zero or more `Adjuster`s. It calls the base model first, then feeds its `FillResult` through each adjuster in sequence. If any step returns an error, the order is rejected (not silently degraded).

### DataFetcher

Models that need additional data beyond the current bar (e.g., VWAP requesting intraday bars) accept a `DataFetcher` at construction time. This is the same narrow interface used elsewhere in the codebase for on-demand data access; the `fill` package defines its own copy to avoid importing `risk` or `engine`:

```go
type DataFetcher interface {
    FetchAt(ctx context.Context, assets []asset.Asset,
        timestamp time.Time, metrics []data.Metric) (*data.DataFrame, error)
}
```

The `SimulatedBroker` passes its `DataFetcher` to the pipeline via a `SetDataFetcher(DataFetcher)` method on the `Pipeline` type. The pipeline propagates it to any child model that implements the optional `DataFetcherAware` interface:

```go
type DataFetcherAware interface {
    SetDataFetcher(DataFetcher)
}
```

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
1. Real bid/ask from the bar (`data.Bid` / `data.Ask`) if present
2. Configured estimate via `SpreadBPS(bps int)` option
3. Error if neither is available

### MarketImpact

Square-root impact model: `impact = coefficient * sqrt(orderShares / barVolume)`.

- Buys: `price * (1 + impact)`
- Sells: `price * (1 - impact)`
- If order volume exceeds a threshold relative to bar volume, the fill is partial

Named presets with bundled coefficient and volume threshold:
- `LargeCap` -- coefficient 0.1, partial fill above 5% of daily volume
- `SmallCap` -- coefficient 0.3, partial fill above 2% of daily volume
- `MicroCap` -- coefficient 0.5, partial fill above 1% of daily volume

### SlippageFill

Configurable fixed or percentage slippage, applied directionally (increases cost for buys, decreases proceeds for sells).

- `Slippage(fill.Percent(0.1))` -- percentage-based
- `Slippage(fill.Fixed(0.05))` -- fixed dollar amount

## Scope

Fill models apply to **market orders only**. Stop-loss and take-profit orders handled by `EvaluatePending` continue to use their existing trigger-price logic (high/low evaluation). This avoids conflating two distinct concerns: fill realism for discretionary orders vs. bracket-order trigger mechanics.

## Dollar-Amount Orders

The `SimulatedBroker` currently converts dollar-amount orders (`Amount > 0, Qty == 0`) to share quantities using the close price. With fill models, quantity conversion moves **after** the base model runs: the base model determines the execution price, then the broker divides the dollar amount by that price to get the share quantity. The resulting quantity is then passed through any adjusters (e.g., market impact may further reduce it via partial fill).

## Partial Fill Lifecycle

When a fill model returns a partial fill:

1. The broker emits a fill event for the executed portion at the fill price.
2. The unfilled remainder becomes a pending order for the **next bar only**. On the next bar, the fill model runs again with the reduced quantity (market impact recalculates based on the smaller remaining size).
3. If the remainder is still not fully filled after the second bar, it is cancelled. This prevents infinite pending orders. The cancellation is reported as a standard order cancellation event.

## Package Layout

New package `fill` at the project root. Keeps fill logic separate from `broker` (interface definitions) and `engine` (SimulatedBroker implementation).

```
fill/
    fill.go          # BaseModel, Adjuster, FillResult, Pipeline, DataFetcher
    close.go         # CloseFill
    vwap.go          # VWAPFill
    spread.go        # SpreadAware
    impact.go        # MarketImpact with presets
    slippage.go      # SlippageFill
```

## Changes to SimulatedBroker

1. Add `fillPipeline *fill.Pipeline` field, defaulting to a pipeline with `fill.Close()` and no adjusters
2. Add `WithFillModel(base fill.BaseModel, adjusters ...fill.Adjuster) Option` constructor option
3. In `Submit`, replace hardcoded close-price lookup with pipeline execution for market orders
4. Handle partial fills from `FillResult`: emit a partial fill event and queue the remainder for the next bar
5. After the price provider is set, call `pipeline.SetDataFetcher(...)` to propagate the fetcher to models that need it
6. Move dollar-amount quantity conversion to after the base model determines the fill price

This is a public API addition (`WithFillModel` option) that must appear in the changelog.

## Error Handling

- If a fill model returns an error, the order is rejected. Errors are never silently swallowed.
- SpreadAware with no bid/ask data and no configured fallback returns an error.
- MarketImpact with no volume data returns an error.
- VWAP gracefully degrades to typical price when intraday data is unavailable (this is an approximation, not a silent failure -- the data is still valid daily OHLC).
