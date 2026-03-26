# Broker Batch Prefetch Design

## Problem

When the simulated broker fills orders after a strategy's Compute step, it fetches price data one asset at a time. Each `Submit` call triggers `Engine.Prices()` for a single asset, which calls `FetchAt` -> `fetchRange`. The cache is keyed per (asset, metric, year-chunk) and works correctly -- but because `Submit` is called per-order with one asset, `fetchRange` only ever sees 1 asset per call. Each call triggers a separate year-chunk provider fetch for that single asset.

For a 3-year weekly backtest with 50 holdings, this produces ~6,000 individual provider queries instead of ~3 (one per year chunk).

A secondary issue: `Engine.Prices()` does not include `Volume` in its metric list, so the `MarketImpact` fill adjuster receives NaN for volume.

## Design

### 1. Add Volume to `Engine.Prices()`

Add `data.Volume` to the metric list in `Engine.Prices()` (`engine/engine.go:634`). This fixes the MarketImpact adjuster bug.

### 2. Engine prefetches before ExecuteBatch

In the backtest loop (`engine/backtest.go`), insert a batched `e.Prices()` call between `strategy.Compute` and `acct.ExecuteBatch`. The engine collects all unique assets from the batch's orders and calls `Prices` once with all of them:

```go
e.Prices(ctx, allOrderAssets...)
```

This single call populates the year-chunk cache for every order asset. All subsequent per-order `Prices()` calls in `Submit` become cache hits.

The same pattern applies to child strategy batches in the backtest loop.

If the batch has no orders, skip the prefetch.

### 3. Batch-fetch in evaluatePartialRemainders

Refactor `SimulatedBroker.evaluatePartialRemainders()` to collect all unique assets from pending remainders and make one `Prices` call with all of them, then iterate over the shared DataFrame. This matches the pattern `EvaluatePending` already uses.

## Files Changed

| File | Change |
|------|--------|
| `engine/engine.go` | Add `Volume` to `Prices()` metric list |
| `engine/backtest.go` | Add prefetch before `ExecuteBatch` (parent + child paths) |
| `engine/simulated_broker.go` | Refactor `evaluatePartialRemainders` to batch |

## What Does Not Change

- No new interfaces or types in the broker package.
- `SimulatedBroker.Submit` is unchanged -- it still calls `Prices` per order, but now those calls are cache hits.
- Live brokers (Alpaca, Schwab, IBKR) are unaffected -- they fetch their own prices from brokerage APIs.
- `EvaluatePending` already batches correctly and is unchanged.
- The cache logic in `fetchRange` is correct and unchanged.

## Testing

- Existing tests continue to pass (the prefetch is transparent to the broker).
- Add a test verifying that the engine's prefetch call covers all order assets before individual submits.
- Add a test verifying `evaluatePartialRemainders` makes one batched `Prices` call, not N individual calls.
