# Short Selling Design

## Overview

Add support for short selling positions in strategy simulations. A short sale borrows shares and sells them at the current market price, profiting when the price falls. The design covers the full lifecycle: opening shorts, tracking cost basis via short tax lots, margin accounting, borrow fees, dividend obligations, margin calls, split handling, risk middleware updates, and long/short P&L reporting.

The implementation follows a bottom-up approach, building from the data model through accounting, engine housekeeping, margin mechanics, and finally reporting.

## 1. Short Tax Lots and Cost Basis

### portfolio/account.go

Short lots mirror long tax lots with inverted entry/exit semantics.

**Opening a short position:** `Record()` must be refactored to add short-aware routing. Currently, sells always consume long lots and buys always create long lots. The new behavior: when `Record()` processes a sell transaction, it first tries to consume existing long lots (closing a long). If no long lots exist or quantity exceeds long holdings, the remainder creates short lots. Each short lot records:

```go
type TaxLot struct {
    ID    string
    Date  time.Time // entry date (when the short was opened)
    Qty   float64   // shares shorted
    Price float64   // entry price (the sell price)
}
```

Short lots are stored in a separate map from long lots to avoid ambiguity:

```go
shortLots map[asset.Asset][]TaxLot
```

**Covering a short:** Similarly, the buy path in `Record()` must be extended. When `Record()` processes a buy transaction and short lots exist for the asset, it consumes short lots before creating long lots. Lot selection uses the same methods as longs (FIFO, LIFO, HighestCost).

**Realized P&L on cover:**
- P&L = (entry price - cover price) x quantity
- Holding period = cover date - entry date
- LTCG if held > 1 year, STCG otherwise

**Wash sales:** Apply across long and short positions in the same asset. Closing a short at a loss and opening a new short (or going long) within 30 days defers the loss, using the same wash sale logic that exists for longs.

## 2. Split Handling

### engine/backtest.go, engine/live.go

A new daily housekeeping step that adjusts positions and tax lots when stock splits occur. Runs for all held positions, both long and short.

**When to run:** Inside `housekeepAccount`, before dividend recording. The current engine loop runs housekeeping (step 13-14b) before `Compute` (step 15-16). Within `housekeepAccount`, the order is: drain broker fills, apply splits, then record dividends. Splits must run before dividends because dividend per share values are post-split. The `SplitFactor` metric must be added to the `housekeepMetrics` slice so it is fetched alongside close, adjusted close, and dividend data.

**Position adjustment:**
- New quantity = old quantity x split factor
- Applies to both positive (long) and negative (short) holdings

**Tax lot adjustment (long and short lots):**
- Lot quantity = lot quantity x split factor
- Lot price = lot price / split factor
- Total cost basis is preserved

**Transaction recording:** A new `SplitTransaction` type records the event:

```go
const (
    // ... existing transaction types ...
    SplitTransaction
)
```

The transaction captures: asset, date, split factor, old quantity, new quantity.

**Sentinel handling:** A NaN or missing split factor is treated as 1.0 (no split). A split factor of 0 is an error and must be rejected to avoid destroying position data.

**Data source:** The `SplitFactor` metric is already fetched and stored by data providers. The engine reads it from the DataFrame each day.

## 3. Margin Accounting

### portfolio/account.go

Margin state is tracked on `Account` and recalculated daily after price updates.

**Core calculations:**
- **Short market value** = sum of abs(qty) x price for all short positions
- **Long market value** = sum of qty x price for all long positions
- **Equity** = cash + long market value - short market value
- **Margin ratio** = equity / short market value (NaN when no shorts exist)
- **Buying power** = cash available for new positions, accounting for margin requirements on existing shorts

**Configurable parameters:**
- Initial margin rate: default 0.50 (Reg T -- must deposit 50% of short value)
- Maintenance margin rate: default 0.30 (Reg T -- 30% equity requirement)

These are set on the Account and can be configured via strategy description or engine options.

### Portfolio interface additions

```go
MarginRatio() float64        // current margin ratio; NaN if no shorts
MarginDeficiency() float64   // dollars needed to restore maintenance margin; 0 if not breached
ShortMarketValue() float64   // total absolute value of short positions
BuyingPower() float64        // cash available accounting for margin requirements
```

These are read-only and available to strategies during `Compute`.

## 4. Borrow Fees and Dividend Debits

### engine/backtest.go, engine/live.go

Both are daily housekeeping steps in the engine loop.

**Borrow fees:**
- Configurable annualized rate, default 0.50%
- Applied daily per short position: fee = abs(qty) x price x (annual rate / 252)
- Debited from cash as a `FeeTransaction` per asset for visibility in the transaction log

**Dividend debits on shorts:**
- When dividend data appears for an asset with a short position, debit cash: dividend per share x abs(short quantity)
- Recorded as a `DividendTransaction` with negative amount
- Justification field populated with context, e.g., "short dividend obligation: MSFT ex-date 2026-03-20"
- Runs in the same housekeeping step as existing long dividend recording
- Note: the current dividend recording code in `housekeepAccount` already skips positions where `qty <= 0`. The implementation must change this guard to handle negative quantities (short positions) by debiting cash instead of skipping them

## 5. Margin Calls and MarginCallHandler

### engine/backtest.go, engine/live.go

Daily margin check in the engine loop, after housekeeping (splits, borrow fees, dividends) but before `Compute`. In the current backtest loop, this is a new step between step 13-14b (housekeeping) and step 15-16 (strategy compute). The check runs on every trading day, not just strategy-schedule days, so it fires even for strategies with weekly or monthly schedules.

**Detection:** If margin ratio < maintenance margin rate, a margin call is active.

**Optional strategy interface (defined in engine package):**

```go
type MarginCallHandler interface {
    OnMarginCall(ctx context.Context, eng *Engine, port portfolio.Portfolio, batch *portfolio.Batch) error
}
```

**Response flow:**
1. Engine checks if strategy implements `MarginCallHandler` (type assertion)
2. If yes: engine creates a batch, calls `OnMarginCall`, strategy writes orders to address the shortfall, engine executes the batch. The batch bypasses risk middleware since the strategy is responding to an emergency and risk limits should not block the response.
3. If no: engine auto-liquidates by covering short positions proportionally until margin is restored
4. After either path: re-check margin. If still breached, force-liquidate proportionally

## 6. Risk Middleware Updates

### risk/

Extend existing risk middleware to understand short positions.

**Position size limits:**
- Apply to both long and short positions symmetrically
- A short position's size = abs(market value) as a percentage of portfolio value
- A 10% limit means no single position (long or short) exceeds 10%
- Middleware must query current portfolio holdings to determine whether a sell order opens a new short vs closes a long, since the order itself does not carry this distinction. The middleware already receives the `Portfolio` interface (via `Batch`) which provides `Position(asset)` for this check. A sell order that would result in a net negative position is a short-opening order; a sell that reduces a positive position is a long-closing order.

**Gross and net exposure limits (new):**
- Gross exposure = (long market value + short market value) / equity
- Net exposure = (long market value - short market value) / equity
- Both configurable with no default limit (opt-in)

**Drawdown circuit breaker:**
- Already works off portfolio value, which naturally includes shorts via negative quantities
- No change needed

**Volatility scaling:**
- Handle short positions symmetrically -- scale down short position sizes in high-vol names the same as longs

## 7. Simulated Broker Updates

### engine/simulated_broker.go

**Initial margin enforcement:**
- Before filling a short order, verify that post-trade equity / post-trade short market value >= initial margin rate
- If insufficient margin, reject the order (no fill sent, rejection logged)
- Requires read access to current portfolio state for the margin check

**Borrow availability:**
- All securities borrowable at the configured flat rate
- No order rejection on borrow grounds
- The interface accommodates a future borrow availability check without structural change

**Order side semantics:**
- No change needed to fill mechanics -- the portfolio layer determines whether a sell opens a short or closes a long based on current holdings

## 8. P&L Reporting and Performance Metrics

### portfolio/

**TradeDetail changes:**

Add a direction indicator to `TradeDetail`:

```go
type TradeDirection int
const (
    TradeLong TradeDirection = iota
    TradeShort
)
```

New field on `TradeDetail`:

```go
Direction TradeDirection
```

For short trades:
- Entry price = sell price, exit price = cover price
- P&L = (entry price - exit price) x quantity
- MFE = (entry price - period low) / entry price
- MAE = (entry price - period high) / entry price (a negative move for shorts is price going up)

**Long/short metric breakdown:**

Key metrics computed three ways (combined, long-only, short-only):
- Win rate
- Profit factor
- Average P&L
- Average holding period
- Average MFE / MAE

Exposed as additional registered performance metrics: `LongWinRate`, `ShortWinRate`, `LongProfitFactor`, `ShortProfitFactor`, etc.

**Unrealized P&L:**
- Long positions: (current price - avg cost basis) x quantity
- Short positions: (entry price - current price) x abs(quantity)
- LTCG/STCG classification based on holding period of open short lots

**Equity curve:** No change needed. `Value()` already handles negative quantities correctly.

## Negative Weights in Allocations

### portfolio/account.go, portfolio/batch.go

`RebalanceTo` is defined on both `Account` (account.go) and `Batch` (batch.go). Both must accept negative weights to indicate short targets. A weight of -0.50 means "short 50% of portfolio value in this asset."

The delta math extends naturally:
- Target position = portfolio value x weight / current price (negative weight produces negative target)
- Delta = target position - current position
- Positive delta generates buy orders, negative delta generates sell orders

No new types needed. The sign convention (negative = short) is the industry standard used by Zipline, QuantConnect, Backtrader, and others.

Helper functions like `EqualWeight` default to long (positive weights). Strategy authors use negative weights explicitly when constructing short allocations.

**Liquidation of unlisted positions:** The current `RebalanceTo` implementation liquidates positions not present in the target allocation, but only checks `qty > 0` (long positions). This must be updated to also cover (buy back) short positions not in the target allocation.

**Live engine parity:** The live engine (`engine/live.go`) has the same housekeeping structure as the backtest engine. All housekeeping changes (splits, borrow fees, dividend debits, margin checks) apply to both engines identically.

## Implementation Order

Bottom-up, each layer testable before the next builds on it:

1. Short tax lots and cost basis tracking (`portfolio/`)
2. Split handling (`engine/`)
3. Margin accounting (`portfolio/`)
4. Borrow fees and dividend debits (`engine/`)
5. Margin call detection and `MarginCallHandler` (`engine/`)
6. Risk middleware updates (`risk/`)
7. Simulated broker updates (`engine/`)
8. P&L reporting and metrics (`portfolio/`)
