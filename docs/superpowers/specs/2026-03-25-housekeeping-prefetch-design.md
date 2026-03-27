# Housekeeping Prefetch and Holdings API Cleanup

## Problem

The backtest loop makes three separate `FetchAt` calls per step for held assets, each requesting different metrics:

1. `housekeepAccount -> Transactions` -- Close, High, Low, Volume, Dividend, SplitFactor
2. `setMarginPrices` -- Close
3. `updateAccountPrices` -- Close, AdjClose, High, Low

These all target the same assets (current holdings) but run at different points, causing separate cache miss cycles. Together with Submit, they account for 26% of all FetchAt CPU time.

Additionally, `Holdings()` uses a callback signature `Holdings(fn func(asset.Asset, float64))` that every caller wraps in the same boilerplate to collect into a slice or map. No caller uses the callback for anything else.

## Design

### 1. Change `Holdings` to return `map[asset.Asset]float64`

Change the `Portfolio` interface method from:

```go
Holdings(fn func(asset.Asset, float64))
```

to:

```go
Holdings() map[asset.Asset]float64
```

The `Account` implementation continues to handle substitution aggregation internally. All callers are updated to use the returned map directly.

This is a **breaking public API change** (strategy authors use `Portfolio.Holdings`) and must appear in the changelog.

### 2. Add `prefetchHousekeepingPrices` to the engine

At the start of each backtest step, before `housekeepAccount`, the engine collects all held assets from the account and calls `FetchAt` once with the union of all housekeeping metrics:

```go
[Close, AdjClose, High, Low, Volume, Dividend, SplitFactor]
```

This single call warms the year-chunk cache for all three subsequent callers. Same for child accounts before their housekeeping.

### 3. Simplify callers of Holdings

With the new return type, callers that previously collected assets via callback can iterate the map directly. Callers that only need assets (not quantities) iterate keys. Callers that need both (like Transactions, tax_loss_harvester, drawdown_circuit_breaker) use the map entries.

## Files Changed

| File | Change |
|------|--------|
| `portfolio/portfolio.go` | Change `Holdings` signature in `Portfolio` interface |
| `portfolio/account.go` | Change `Holdings` implementation to return `map[asset.Asset]float64` |
| `engine/engine.go` | Add `prefetchHousekeepingPrices` method |
| `engine/backtest.go` | Call prefetch before housekeeping; update `updateAccountPrices` caller |
| `engine/simulated_broker.go` | Update `Transactions` Holdings caller |
| `engine/margin_call.go` | Update `setMarginPrices` and `checkAndHandleMarginCall` callers |
| `engine/child_allocations.go` | Update Holdings caller |
| `engine/live.go` | Update Holdings caller |
| `engine/middleware/risk/drawdown_circuit_breaker.go` | Update Holdings caller |
| `engine/middleware/tax/tax_loss_harvester.go` | Update Holdings caller |
| `portfolio/batch.go` | Update Holdings callers |
| `portfolio/snapshot.go` | Update Holdings caller |
| `docs/portfolio.md` | Update Holdings documentation |
| `docs/strategy-guide.md` | Update Holdings examples |
| Test files | Update all test callers |

## What Does Not Change

- The substitution aggregation logic inside `Account.Holdings` is unchanged -- it still maps real assets to logical assets before returning.
- `Transactions`, `setMarginPrices`, and `updateAccountPrices` are not otherwise modified -- they just get warm cache hits from the prefetch.
- The live loop is not modified for prefetching (live brokers fetch from brokerage APIs).
