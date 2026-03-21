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

A new `excursionRecord` type tracks the running price extremes for each open
position. The tracker state lives in the `Account` struct as a map keyed by
asset.

```go
type excursionRecord struct {
    entryPrice float64
    highPrice  float64 // running max of daily High (long) or min of daily Low (short)
    lowPrice   float64 // running min of daily Low (long) or max of daily High (short)
    direction  int     // +1 long, -1 short
}
```

Lifecycle:

- **On BuyTransaction**: initialize with `entryPrice` from the fill,
  `highPrice = entryPrice`, `lowPrice = entryPrice`, `direction = +1`. If an
  excursion record already exists for the asset (adding to an existing
  position), keep the existing record -- excursions track the full position
  lifecycle, not individual lots.
- **On SellShort**: same as above but with `direction = -1`.
- **On UpdateExcursions**: for each open position, read the daily High and
  Low from the price DataFrame. For long positions: `highPrice = max(highPrice,
  dailyHigh)` and `lowPrice = min(lowPrice, dailyLow)`. For short positions
  the logic is inverted: favorable movement is price going down, adverse is
  price going up.
- **On SellTransaction (closing a long) or BuyToCover (closing a short)**:
  finalize the excursion record, compute MFE/MAE percentages, and store them
  on the completed `TradeDetail`. Remove the record from the tracker.

MFE and MAE are computed as percentages from entry price:

- Long: `MFE = (highPrice - entryPrice) / entryPrice`,
  `MAE = (lowPrice - entryPrice) / entryPrice`
- Short: `MFE = (entryPrice - lowPrice) / entryPrice`,
  `MAE = (entryPrice - highPrice) / entryPrice`

MFE is always >= 0 (the best case is at least flat). MAE is always <= 0
(the worst case is at least flat).

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
    Direction  int     // +1 long, -1 short
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

The existing `roundTrips()` function is refactored to share the same FIFO
matching logic with `TradeDetails()`, so both use a single code path.

### 3. Serialization

Both completed trade details and open excursion records must survive
snapshot round-trips. The `PortfolioSnapshot` interface gains two methods:

```go
type PortfolioSnapshot interface {
    // ... existing methods ...
    TradeDetails() []TradeDetail
    Excursions() map[asset.Asset]excursionRecord
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

The `PortfolioManager` interface (engine-facing mutation methods) gains:

```go
UpdateExcursions(df *data.DataFrame)
```

Engine backtest loop changes:

- **Step 17**: expand `priceMetrics` to include `MetricHigh` and `MetricLow`
  alongside the existing `MetricClose` and `AdjClose`.
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
    CaptureRatio float64 // average profit pct / average MFE
}
```

Each metric gets its own implementation file following the existing pattern
(e.g., `average_mfe.go` with an unexported struct implementing
`PerformanceMetric`). All six are registered in `WithTradeMetrics()` and
`WithAllMetrics()`.

The metrics compute from `TradeDetails()` rather than from `roundTrips()`,
since `TradeDetails` already has the MFE/MAE values.

### 6. Documentation

**Code documentation:**

- Doc comments on all new public types (`TradeDetail`, `excursionRecord`)
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
     -> On BuyTransaction: Account.Record() initializes excursionRecord
     -> On SellTransaction: Account.Record() finalizes excursionRecord,
        computes MFE/MAE, appends TradeDetail
  4. UpdatePrices(priceDF)    -- equity curve
  5. UpdateExcursions(priceDF) -- running high/low for open positions

After backtest:
  p.TradeDetails()  -> []TradeDetail with per-trade MFE/MAE
  p.TradeMetrics()  -> TradeMetrics with summary stats
```

## Testing

- Unit tests for `excursionRecord` update logic (long and short positions)
- Unit tests for MFE/MAE percentage calculation (long and short)
- Unit tests for each new `PerformanceMetric` implementation
- Integration test with a multi-trade backtest verifying per-trade
  MFE/MAE values against known High/Low sequences
- Test that `Clone()` and `WithPortfolioSnapshot` correctly preserve
  excursion state
- Test edge cases: same-day open/close, position adds (buying more of a
  held asset), partial closes
