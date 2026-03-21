# MFE/MAE Trade Quality Analytics Design

## Problem

pvbt computes aggregate trade metrics (win rate, profit factor, average
holding period) but provides no insight into how well a strategy exits its
trades. A strategy might have a reasonable win rate yet consistently leave
money on the table by closing winners too early, or it might hold losers too
long before stopping out. Without per-trade excursion data there is no way
to diagnose these exit quality problems.

## Solution

Add Maximum Favorable Excursion (MFE) and Maximum Adverse Excursion (MAE)
analytics that track the best and worst price movement for every trade
during its holding period. MFE is the furthest a trade moved in your favor
before you closed it; MAE is the furthest it moved against you. Together
they answer: "given what the market offered, how good were my exits?"

Excursions are tracked live during the backtest using daily High/Low prices,
expressed as percentages from entry price, and exposed both as per-trade
detail and as summary statistics.

## Design

### 1. Excursion tracker

A new `ExcursionRecord` type tracks the running price extremes for each open
position. The tracker state lives in the `Account` struct as a map keyed by
asset.

```go
type ExcursionRecord struct {
    EntryPrice float64
    HighPrice  float64 // running max of daily High
    LowPrice   float64 // running min of daily Low
}
```

The type is exported because it appears in the `PortfolioSnapshot` interface
(see Section 3).

Lifecycle:

- **On BuyTransaction**: initialize with `EntryPrice` from the fill,
  `HighPrice = EntryPrice`, `LowPrice = EntryPrice`. If an excursion record
  already exists for the asset (adding to an existing position), keep the
  existing record -- excursions track the full position lifecycle, not
  individual lots. This means the `EntryPrice` reflects the first buy, not a
  weighted average. For scaled-in positions this is a simplification: MFE/MAE
  percentages are relative to the original entry, not later adds. This
  trade-off favors simplicity; per-lot excursion tracking would require
  matching High/Low updates to individual lots, adding significant complexity
  for a niche case. All `TradeDetail` entries produced from partial closes of
  the same position share the same MFE/MAE values, reflecting the excursion
  of the entire position from first entry to each exit.
- **On UpdateExcursions**: for each open position, read the daily High and
  Low from the price DataFrame: `HighPrice = max(HighPrice, dailyHigh)` and
  `LowPrice = min(LowPrice, dailyLow)`. If High or Low is NaN (missing data
  for that asset on that day), skip the update for that asset.
- **On SellTransaction (full close)**: finalize the excursion record,
  compute MFE/MAE percentages, store them on the completed `TradeDetail`,
  and remove the record from the tracker. A subsequent buy of the same asset
  starts a fresh excursion record.
- **On SellTransaction (partial close)**: compute MFE/MAE from the current
  excursion record and attach to the `TradeDetail` for the closed lots, but
  keep the excursion record in the tracker for the remaining position.

MFE and MAE are computed as percentages from entry price:

```
MFE = (HighPrice - EntryPrice) / EntryPrice
MAE = (LowPrice - EntryPrice) / EntryPrice
```

MFE is always >= 0 (the best case is at least flat). MAE is always <= 0
(the worst case is at least flat).

**Same-day open/close**: if a position is opened and closed on the same day,
`UpdateExcursions` has not yet been called for that day (it runs after
strategy execution). The excursion record will have `HighPrice = EntryPrice`
and `LowPrice = EntryPrice`, resulting in MFE = 0 and MAE = 0. This is a
known limitation of daily-bar tracking.

### 2. Enriched round trips and public TradeDetail

The private `roundTrip` struct gains MFE/MAE fields:

```go
type roundTrip struct {
    pnl      float64
    holdDays float64
    mfe      float64 // percentage from entry price
    mae      float64 // percentage from entry price (negative)
}
```

A new public type exposes per-trade data for analysis:

```go
type TradeDetail struct {
    Asset      asset.Asset
    EntryDate  time.Time
    ExitDate   time.Time
    EntryPrice float64
    ExitPrice  float64
    Qty        float64
    PnL        float64
    HoldDays   float64
    MFE        float64 // max favorable excursion as % of entry price
    MAE        float64 // max adverse excursion as % of entry price
}
```

A new method on `Account` exposes completed trade details:

```go
func (a *Account) TradeDetails() []TradeDetail
```

The existing `roundTrips()` function and all trade metrics that use it
(`WinRate`, `AverageWin`, `AverageLoss`, `ProfitFactor`,
`AverageHoldingPeriod`, `Turnover`, `TradeGainLossRatio`) are migrated to
compute from `TradeDetails()`. This eliminates the parallel FIFO matching
code path so there is a single source of truth for round-trip data.

### 3. Serialization

Both completed trade details and open excursion records must survive
snapshot round-trips. The `PortfolioSnapshot` interface gains two methods:

```go
type PortfolioSnapshot interface {
    // ... existing methods ...
    TradeDetails() []TradeDetail
    Excursions() map[asset.Asset]ExcursionRecord
}
```

`WithPortfolioSnapshot` restores both when creating an Account from a
snapshot. `Clone()` deep-copies both the `tradeDetails` slice and the
`excursions` map.

### 4. Interface and engine changes

The `Portfolio` interface (read-only, strategy-facing) gains:

```go
TradeDetails() []TradeDetail
```

This is intentional: the `Portfolio` interface already exposes
`Transactions()` (the raw transaction log), so per-trade detail is not a new
category of exposure. Strategies may use `TradeDetails()` during `Compute()`
to adapt behavior based on past trade quality, similar to how they can
already use `PerformanceMetric(MaxDrawdown).Value()` to react to drawdowns.

The `PortfolioManager` interface (engine-facing mutation methods) gains:

```go
UpdateExcursions(df *data.DataFrame)
```

Engine backtest loop changes:

- **Step 17**: expand `priceMetrics` to include `MetricHigh` and `MetricLow`
  alongside the existing `MetricClose` and `AdjClose`. This doubles the
  per-step price data fetch. The additional data is small (two extra columns
  per asset per day) and is required for accurate excursion tracking.
  `housekeepMetrics` (step 13) does not need High/Low since it is used only
  for dividends and close prices.
- **Step 18**: after calling `acct.UpdatePrices(priceDF)`, call
  `acct.UpdateExcursions(priceDF)`. The engine passes data through; all
  excursion logic lives in the `portfolio` package.

### 5. Summary statistics in TradeMetrics

Six new fields are added to the `TradeMetrics` struct:

```go
type TradeMetrics struct {
    // ... existing fields ...
    AverageMFE   float64 // mean MFE across all round trips
    AverageMAE   float64 // mean MAE across all round trips
    MedianMFE    float64 // median MFE across all round trips
    MedianMAE    float64 // median MAE across all round trips
    EdgeRatio    float64 // average MFE / abs(average MAE)
    CaptureRatio float64 // mean realized return / mean MFE
}
```

`CaptureRatio` is defined precisely as:
`mean((exitPrice - entryPrice) / entryPrice) / mean(MFE)` across all
completed trades. A value of 1.0 means the strategy captures all available
favorable movement on average; lower values indicate premature exits.

Each metric gets its own implementation file following the existing pattern:
`average_mfe.go`, `average_mae.go`, `median_mfe.go`, `median_mae.go`,
`edge_ratio.go`, `capture_ratio.go`. Each file defines an unexported struct
implementing `PerformanceMetric`. All six are registered in
`WithTradeMetrics()` and `WithAllMetrics()`.

The metrics compute from `TradeDetails()`, which already has the MFE/MAE
values. The `TradeMetrics()` method on `Account` is updated to query and
populate all six new fields, following the same `PerformanceMetric().Value()`
pattern used by the existing fields.

Note: `realizedGains()` in `metric_helpers.go` performs its own FIFO
matching for tax-lot-specific data (LTCG/STCG classification). It remains
separate from `TradeDetails()` because it tracks tax-specific state
(holding period qualification) that is orthogonal to excursion tracking.

### 6. Documentation

**Code documentation:**

- Doc comments on all new public types (`TradeDetail`, `ExcursionRecord`)
  and methods (`TradeDetails()`, `UpdateExcursions()`)
- Update doc comments on `TradeMetrics`, `Portfolio`, and
  `PortfolioManager` to reflect the new additions

**New `docs/performance-metrics.md`:**

- Extract the entire performance measurement content from `portfolio.md`
  into this new file (metric tables, custom metrics, bundles, usage during
  computation)
- Add a new "Trade quality: MFE and MAE" section explaining the concepts,
  why they matter, and how to interpret the results:
  - High MFE vs low realized profit means exits are too early
  - Deep MAE vs realized loss means stops are too loose
  - Edge ratio measures favorable vs adverse movement balance
  - Capture ratio measures how much available profit is being captured
- Document `TradeDetails()` with usage examples
- Add the six new metrics to the TradeMetrics table and Available metrics
  listing
- `portfolio.md` retains only a brief summary of the metrics system with a
  link to the new file

**Changelog:**

- Entry describing the new MFE/MAE trade quality analytics feature

## Data flow

```
Engine daily loop:
  1. Fetch High/Low/Close/AdjClose for held assets
  2. Record dividends, drain fills
  3. Run strategy Compute (may produce Buy/Sell transactions)
     -> On BuyTransaction: Account.Record() initializes ExcursionRecord
     -> On SellTransaction: Account.Record() finalizes ExcursionRecord,
        computes MFE/MAE, appends TradeDetail
  4. UpdatePrices(priceDF)    -- equity curve
  5. UpdateExcursions(priceDF) -- running high/low for open positions

After backtest:
  p.TradeDetails()  -> []TradeDetail with per-trade MFE/MAE
  p.TradeMetrics()  -> TradeMetrics with summary stats
```

## Testing

- Unit tests for `ExcursionRecord` update logic
- Unit tests for MFE/MAE percentage calculation
- Unit tests for each new `PerformanceMetric` implementation
- Integration test with a multi-trade backtest verifying per-trade
  MFE/MAE values against known High/Low sequences
- Test that `Clone()` and `WithPortfolioSnapshot` correctly preserve
  excursion state
- Test edge cases: same-day open/close (MFE=0, MAE=0), position adds
  (buying more of a held asset keeps existing excursion record), partial
  closes (MFE/MAE from full position attached to each lot's TradeDetail)
