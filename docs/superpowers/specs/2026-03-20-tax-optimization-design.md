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
- `HighestCost` -- sell the lot with the highest cost basis first (maximizes realized losses)
- `SpecificID` -- sell a specific lot by reference

The account holds a default lot selection method set at construction time (defaults to `FIFO` for backwards compatibility). `broker.Order` gains an optional `LotSelection` field; when set, it overrides the account default for that order. The account's sell-recording logic switches on the method to determine which lots to consume.

### Wash Sale Tracking

When `Record()` processes a buy transaction, it checks whether the same asset had a loss-generating sell within the prior 30 calendar days. If a wash sale is detected:

- The disallowed loss is added to the new lot's cost basis
- A `WashSaleRecord` is stored (original loss amount, disallowed amount, adjusted lot reference, dates)

This is always-on. The wash sale window is tracked as a slice of recent loss sales per asset, pruned on each transaction.

### TaxAware Interface

A new interface in the `portfolio` package, separate from `Portfolio`:

```go
type TaxAware interface {
    WashSaleWindow(asset asset.Asset) []WashSaleRecord
    UnrealizedLots(asset asset.Asset) []TaxLot
    RealizedGainsYTD() (ltcg, stcg float64)
    SetLotSelection(method LotSelection)
    RegisterSubstitution(original, substitute asset.Asset, until time.Time)
    ActiveSubstitutions() map[asset.Asset]Substitution
}
```

`Account` implements both `Portfolio` and `TaxAware`. The tax middleware type-asserts the batch's portfolio reference to `TaxAware` to access tax-specific capabilities. Strategies and risk middleware only see `Portfolio`.

### Substitution Mapping

When the tax middleware swaps an asset for a correlated substitute (e.g., sell SPY, buy IVV to avoid a wash sale), it calls `RegisterSubstitution(original, substitute, until)` on `TaxAware`.

- `Holdings()`, `ProjectedHoldings()`, and `ProjectedWeights()` return the logical view: the strategy sees SPY, not IVV
- The transaction log, tax lots, and cost basis reflect reality: the actual asset traded (IVV)
- `Value()` is unaffected -- it returns a dollar total regardless of naming
- `ActiveSubstitutions()` allows the middleware (or anything that type-asserts to `TaxAware`) to see through the mapping

When the 30-day window expires, the middleware unregisters the substitution and injects swap-back orders.

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
5. Inject a sell order with `LotSelection: HighestCost` to maximize the realized loss, with `WithJustification` explaining the harvest (e.g., "tax-loss harvest: SPY down 8%, realized $2,400 loss")
6. If a substitute is configured for the sold asset, inject a buy order for the substitute at the same dollar value, and call `RegisterSubstitution(original, substitute, until)` with a 30-day expiry
7. After 30 days, inject swap-back orders (sell substitute, buy original) and unregister the substitution

If nothing is harvestable, the middleware does nothing silently -- no annotations, no justifications.

### Middleware Chain Ordering

Tax middleware runs before risk middleware. The risk overlay may further adjust positions but should not override tax-motivated trades.

Recommended chain: `TaxLossHarvester -> VolatilityScaler -> MaxPositionSize -> DrawdownCircuitBreaker`

## Tax Drag Metric

Tax drag measures the percentage of pre-tax return consumed by taxes from trading activity. It isolates the cost of buying and selling, excluding dividend taxation.

**Formula:** `TaxDrag = EstimatedTaxFromTurnover / PreTaxReturn`

Where estimated tax from turnover = `(0.25 * STCG) + (0.15 * LTCG)`, using the same rate assumptions as the existing `TaxCostRatio`.

Implemented as a new field on `TaxMetrics` and a corresponding metric computation function following the same pattern as `ltcg.go`, `stcg.go`, etc. Computed from the transaction log via `realizedGains()`.

## Package Structure

### New `tax` package (`pvbt/tax/`)

- `tax.go` -- `DataSource` interface (same as risk's), configuration types
- `tax_loss_harvester.go` -- the middleware implementation
- `profiles.go` -- convenience constructors (e.g., `tax.TaxEfficient(config)`)

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
