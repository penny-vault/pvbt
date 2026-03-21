# Tax Optimization Design Spec

**Issue:** #10
**Date:** 2026-03-20

## Overview

Add active tax optimization to pvbt, building on the existing FIFO tax lot tracking, capital gains classification, and dividend qualification. The strategy decides what to hold; the tax optimizer adjusts timing and execution to minimize tax impact. Strategy authors do not need to think about wash sale windows or loss harvesting -- the engine handles it.

## Architecture: Two-Layer Design

Tax optimization spans two layers:

1. **Account layer (correctness)** -- Wash sale tracking, configurable lot selection, and substitution mapping live in the portfolio system. These are always-on regardless of whether tax middleware is active.
2. **Tax middleware (optimization)** -- A `TaxLossHarvester` middleware actively identifies and executes loss harvesting opportunities. Uses the same `portfolio.Middleware` interface as the risk overlay.
3. **Tax drag metric (reporting)** -- A new metric computed from the transaction log measuring return lost to trading-related taxes.

## Account Layer Changes

### Lot Selection

A new `LotSelection` type with four methods:

- `FIFO` -- first in, first out (current behavior, remains the default)
- `LIFO` -- last in, first out
- `HighestCost` -- sell the lot with the highest cost basis first (produces the largest realized loss when the position is underwater)
- `SpecificID` -- sell a specific lot by reference (requires adding an ID field to `TaxLot`)

The account holds a default lot selection method set at construction time (defaults to `FIFO` for backwards compatibility). `broker.Order` gains an optional `LotSelection` field; when set, it overrides the account default for that order. A corresponding `WithLotSelection(method)` `OrderModifier` is added to `portfolio/order.go` so middleware can set lot selection through the standard `batch.Order()` API. The account's sell-recording logic switches on the method to determine which lots to consume.

### Wash Sale Tracking

IRS wash sale rules apply to a 61-day window: 30 days before through 30 days after a loss sale. In pvbt's chronological processing model, both directions are handled:

- **Buy after loss sale:** When `Record()` processes a buy, it checks whether the same asset had a loss-generating sell within the prior 30 calendar days.
- **Loss sale after buy:** When `Record()` processes a sell at a loss, it checks whether the same asset was bought within the prior 30 calendar days.

In either case, if a wash sale is detected:

- The disallowed loss is added to the replacement lot's cost basis
- A `WashSaleRecord` is stored (original loss amount, disallowed amount, adjusted lot reference, dates)

This is always-on. The wash sale window is tracked as a slice of recent loss sales and recent buys per asset, pruned on each transaction.

### TaxAware Interface

A new interface in the `portfolio` package, separate from `Portfolio`:

```go
type TaxAware interface {
    WashSaleWindow(asset asset.Asset) []WashSaleRecord
    UnrealizedLots(asset asset.Asset) []TaxLot
    RealizedGainsYTD() (ltcg, stcg float64)
    RegisterSubstitution(original, substitute asset.Asset, until time.Time)
    ActiveSubstitutions() map[asset.Asset]Substitution
}
```

Note: `SetLotSelection` is not on this interface. The account-wide default is set at construction via `WithLotSelection()`, and per-order overrides use the `WithLotSelection` `OrderModifier` on individual orders. There is no need for middleware to mutate the account-wide default at runtime.

`Account` implements both `Portfolio` and `TaxAware`. The tax middleware type-asserts the batch's portfolio reference to `TaxAware` to access tax-specific capabilities. Strategies and risk middleware only see `Portfolio`.

### Substitution Mapping

When the tax middleware swaps an asset for a correlated substitute (e.g., sell SPY, buy IVV to avoid a wash sale), it calls `RegisterSubstitution(original, substitute, until)` on `TaxAware`.

- `Holdings()`, `ProjectedHoldings()`, and `ProjectedWeights()` return the logical view: the strategy sees SPY, not IVV
- The transaction log, tax lots, and cost basis reflect reality: the actual asset traded (IVV)
- `Value()` is unaffected -- it returns a dollar total regardless of naming
- `ActiveSubstitutions()` allows the middleware (or anything that type-asserts to `TaxAware`) to see through the mapping

**Batch projection consistency:** Since `Batch.ProjectedHoldings()` starts from `Portfolio.Holdings()` (which returns the logical view) and then applies pending orders, tax-injected orders that reference the real substitute asset (IVV) could cause double-counting. To prevent this, orders injected by the tax middleware for substituted assets are tagged so that `ProjectedHoldings()` maps them through the substitution table. The substitute buy order for IVV is projected as SPY in the logical view.

**Swap-back timing:** When the 30-day window expires, the middleware injects swap-back orders on the next batch it processes. The engine creates and processes batches through the middleware chain on every trading step, even when the strategy produces no orders, so expired substitutions are handled promptly.

## Tax Loss Harvester Middleware

A new `tax` package at `pvbt/tax/`, parallel to `risk/`. The `TaxLossHarvester` implements `portfolio.Middleware`.

### Configuration

```go
type HarvesterConfig struct {
    LossThreshold    float64                      // minimum unrealized loss % to harvest
    GainOffsetOnly   bool                         // only harvest when realized gains exist to offset
    Substitutes      map[asset.Asset]asset.Asset   // optional asset-to-substitute mapping
    DataSource       DataSource                    // market data for current prices
}
```

### Process() Logic

1. Type-assert batch's portfolio to `TaxAware`
2. If `GainOffsetOnly` is true, call `RealizedGainsYTD()` to check whether gains exist to offset; if no gains, return early
3. For each position, call `UnrealizedLots()` to find lots with losses exceeding `LossThreshold`
4. Call `WashSaleWindow()` to check whether selling would be pointless (repurchase within 30 days would trigger a wash sale) -- if a substitute is configured, proceed anyway
5. Inject a sell order via `batch.Order()` with `WithLotSelection(HighestCost)` and `WithJustification` explaining the harvest (e.g., "tax-loss harvest: SPY down 8%, realized $2,400 loss")
6. If a substitute is configured for the sold asset, inject a buy order for the substitute matching the dollar value of the lots actually sold (not the full position), and call `RegisterSubstitution(original, substitute, until)` with a 30-day expiry
7. After 30 days, inject swap-back orders (sell substitute, buy original) and unregister the substitution

If nothing is harvestable, the middleware does nothing silently -- no annotations, no justifications.

### Middleware Chain Ordering

Tax middleware runs before risk middleware. The risk overlay may further adjust positions but should not override tax-motivated trades. Ordering is the caller's responsibility via `Use()` call order.

Recommended chain: `TaxLossHarvester -> VolatilityScaler -> MaxPositionSize -> DrawdownCircuitBreaker`

## Tax Drag Metric

Tax drag measures the percentage of pre-tax return consumed by taxes from trading activity. It isolates the cost of buying and selling, excluding dividend taxation.

**Formula:** `TaxDrag = EstimatedTaxFromTurnover / PreTaxReturn`

Where estimated tax from turnover = `(0.25 * STCG) + (0.15 * LTCG)`. These are the same rate assumptions used by `TaxCostRatio`; both metrics share the hardcoded rates and should be updated together if rates ever become configurable.

Implemented as a new field on `TaxMetrics` and a corresponding metric computation function following the same pattern as `ltcg.go`, `stcg.go`, etc. Computed from the transaction log via `realizedGains()`.

## Package Structure

### New `tax` package (`pvbt/tax/`)

- `tax.go` -- `DataSource` interface (same as risk's), configuration types
- `tax_loss_harvester.go` -- the middleware implementation
- `profiles.go` -- convenience constructors returning `[]portfolio.Middleware` for consistency with the risk package (e.g., `tax.TaxEfficient(config)` returns a single-element slice today but allows future composition)

### Account layer changes (`pvbt/portfolio/`)

- `lot_selection.go` -- `LotSelection` type and constants
- `wash_sale.go` -- `WashSaleRecord` type, wash sale detection logic, 30-day window tracking
- `tax_aware.go` -- `TaxAware` interface definition
- `substitution.go` -- substitution registration and Holdings/ProjectedHoldings/ProjectedWeights mapping
- `tax_drag.go` -- new metric
- Modifications to `account.go` -- implement `TaxAware`, lot selection in sell path, wash sale checks in buy path
- Modifications to `broker/order.go` -- optional `LotSelection` field on `Order`
- Addition of `TaxDrag` field to `TaxMetrics`

## Usage Example

```go
acct := portfolio.New(
    portfolio.WithCash(100_000, start),
    portfolio.WithLotSelection(portfolio.HighestCost),
)

harvester := tax.LossHarvester(tax.HarvesterConfig{
    LossThreshold:  0.05,           // harvest losses > 5%
    GainOffsetOnly: false,          // harvest proactively
    Substitutes:    map[asset.Asset]asset.Asset{spy: ivv},
    DataSource:     engine,
})

acct.Use(harvester)
acct.Use(risk.Conservative(engine)...)
```
