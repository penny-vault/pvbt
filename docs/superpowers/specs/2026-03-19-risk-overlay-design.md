# Portfolio Middleware and Risk Management Overlay Design

## Problem

pvbt has no mechanism to enforce risk discipline independently from strategy
logic. Strategies directly mutate the portfolio through `RebalanceTo()` and
`Order()`, and there is no interception point between strategy decisions and
order execution. This means:

- No way to cap position sizes, enforce drawdown limits, or scale by volatility
- No way to prevent a misbehaving strategy from concentrating into a single position
- The portfolio is directly mutable by strategy code during `Compute()`
- Orders execute immediately with no opportunity for review or modification

## Solution

Introduce a **portfolio middleware** system and a **first-class Batch type**
that together decouple strategy intent from order execution. The middleware
system is general-purpose; a **risk management overlay** is the first
application built on top of it.

## Terminology

These terms are used precisely throughout the codebase and this document:

- **Step**: A single engine loop iteration at a specific timestamp. Every step
  drains broker fills, records dividends, and updates the equity curve.
- **Frame**: A step where the strategy is scheduled to run. Every frame is a
  step, but not every step is a frame. On a frame the engine also cancels open
  orders, runs `Compute()`, processes the batch through middleware, and submits
  orders.
- **Batch**: The first-class type that holds all proposed orders and annotations
  produced during a single frame. The strategy writes to the batch, middleware
  processes it, and the portfolio executes the final result.

## Design

### 1. Batch: first-class type for strategy output

All strategy actions during a frame flow through a Batch. The strategy no
longer writes directly to the portfolio.

```go
type Batch struct {
    Timestamp   time.Time
    Orders      []broker.Order
    Annotations map[string]string
    portfolio   Portfolio  // read-only reference, set at creation
}

func (b *Batch) RebalanceTo(ctx context.Context, alloc ...Allocation) error
func (b *Batch) Order(ctx context.Context, a asset.Asset, side Side, qty float64, mods ...OrderModifier) error
func (b *Batch) Annotate(key, value string)

func (b *Batch) ProjectedHoldings() map[asset.Asset]float64
func (b *Batch) ProjectedValue() float64
func (b *Batch) ProjectedWeights() map[asset.Asset]float64
```

`RebalanceTo()` and `Order()` on the Batch accumulate orders instead of
executing them. `Annotate()` writes to the batch's annotation map using the
batch's timestamp -- no timestamp parameter needed.

`ProjectedHoldings()`, `ProjectedValue()`, and `ProjectedWeights()` compute
what the portfolio would look like if all orders in the batch were executed at
the last known prices from the portfolio. Middleware uses these to evaluate
constraints.

### 2. Portfolio becomes read-only

`RebalanceTo()`, `Order()`, and `Annotate()` move from the `Portfolio`
interface to the `Batch` type. The `Portfolio` interface retains only query
methods:

```go
type Portfolio interface {
    Cash() float64
    Value() float64
    Position(a asset.Asset) float64
    PositionValue(a asset.Asset) float64
    Holdings(fn func(asset.Asset, float64))
    Transactions() []Transaction
    PerfData() *data.DataFrame
    PerformanceMetric(m PerformanceMetric) PerformanceMetricQuery
    Summary() (Summary, error)
    RiskMetrics() (RiskMetrics, error)
    TaxMetrics() (TaxMetrics, error)
    TradeMetrics() (TradeMetrics, error)
    WithdrawalMetrics() (WithdrawalMetrics, error)
    SetMetadata(key, value string)
    GetMetadata(key string) string
    Annotations() []Annotation
}
```

`SetMetadata()` remains on `Portfolio` because metadata is run-level
configuration (strategy name, parameters, run ID) set during `Setup()`, not
trade-level activity that should flow through middleware.

The portfolio's trade state (holdings, cash, positions) can only be mutated
through batch execution. This prevents strategy code from bypassing middleware
and directly altering portfolio state.

### 3. Middleware interface

A middleware processes a batch, potentially modifying its orders and adding
annotations. Middleware is general-purpose -- risk management is one
application, but the same interface supports transaction cost modeling,
slippage simulation, position rounding, and logging.

```go
type Middleware interface {
    Process(ctx context.Context, batch *Batch) error
}
```

A middleware can:

- **Remove orders** from the batch (reduce exposure)
- **Modify order quantities** (cap position sizes)
- **Add new orders** (force-liquidation)
- **Add annotations** explaining what it changed and why

When a middleware reduces exposure, excess goes to cash. Middleware never
amplifies exposure or redistributes to other positions.

If a middleware returns an error, `ExecuteBatch` aborts -- no further
middleware runs and no orders are submitted. The batch is discarded and the
error propagates to the engine.

### 4. Middleware registration and chain execution

Middleware is configured on the account at the runner level, not by the
strategy. This separates investment logic from risk discipline.

```go
// individual middleware
acct.Use(risk.MaxPositionSize(0.25))

// convenience profiles expand to multiple middleware
acct.Use(risk.Conservative(engine)...)
```

The `Account` holds an ordered `[]Middleware`. The chain executes in order --
each middleware receives the output of the previous one. Order matters:
a position size cap after a volatility scaler enforces hard limits on the
scaler's output.

### 5. PortfolioManager changes

`PortfolioManager` gains batch lifecycle methods. `Use()` also lives on
`PortfolioManager` so the engine can configure middleware through the
interface. The concrete `Account` type implements both `Portfolio` and
`PortfolioManager`, as it does today.

```go
type PortfolioManager interface {
    Record(tx Transaction)
    UpdatePrices(df *data.DataFrame)
    SetBroker(b broker.Broker)

    Use(middleware ...Middleware)
    NewBatch(timestamp time.Time) *Batch
    ExecuteBatch(ctx context.Context, batch *Batch) error
    DrainFills(ctx context.Context) error
    CancelOpenOrders(ctx context.Context) error
}
```

- `Use()` appends middleware to the ordered chain.
- `NewBatch()` creates a batch with a reference to the portfolio.
- `ExecuteBatch()` runs the middleware chain over the batch, then submits each
  final order to the broker. After all orders are submitted, it drains the
  broker's fill channel to record any immediate fills as transactions. For the
  simulated broker, all fills arrive immediately so this captures everything.
  For a live broker, some fills may arrive later and will be picked up by
  `DrainFills()` at the next step.
- `DrainFills()` reads all available fills from the broker's fill channel and
  records them as transactions.
- `CancelOpenOrders()` cancels any orders still open at the broker from a
  previous frame.

### 6. Strategy interface change

`Compute()` receives the portfolio (read-only) and the batch (write) as
separate arguments:

```go
type Strategy interface {
    Name() string
    Setup(eng *Engine)
    Compute(ctx context.Context, eng *Engine, portfolio portfolio.Portfolio, batch *portfolio.Batch) error
}
```

The `portfolio` argument and the batch's internal portfolio reference are the
same `Account` instance. The engine passes `acct` as the `Portfolio` and
creates the batch via `acct.NewBatch(timestamp)`, which stores a reference
back to the same account. This ensures `batch.ProjectedHoldings()` reads from
the same state the strategy sees through `portfolio`.

### 7. Broker changes: non-blocking submit and fill channel

`Submit()` becomes fire-and-forget. All fills -- whether immediate or
delayed -- arrive through a buffered channel.

```go
type Broker interface {
    Connect(ctx context.Context) error
    Close() error
    Submit(ctx context.Context, order Order) error
    Cancel(ctx context.Context, orderID string) error
    Replace(ctx context.Context, orderID string, order Order) error
    Orders(ctx context.Context) ([]Order, error)
    Positions(ctx context.Context) ([]Position, error)
    Balance(ctx context.Context) (Balance, error)
    Fills() <-chan Fill
}
```

Both `Submit()` and `Replace()` change from returning `([]Fill, error)` to
returning `error`. All fills from any source -- `Submit`, `Replace`, or
asynchronous brokerage events -- arrive through the `Fills()` channel.

The simulated broker writes fills to the channel immediately on `Submit()`.
A live broker writes fills as the brokerage reports them asynchronously.
The channel is created by the broker implementation with a fixed buffer size
(1024 fills). If the buffer fills because the consumer has not drained it,
the broker blocks on send. This is acceptable: a backed-up fill channel
indicates the engine is not keeping up, and blocking is safer than dropping
fills silently.

### 8. Engine loop

The engine loop distinguishes steps (every iteration) from frames (strategy
schedule). The ordering within each iteration:

**Every step:**

1. Drain fills from the broker channel -- record as transactions
2. Record dividends for held assets
3. Update prices / equity curve

**Frames additionally, after the above:**

4. Cancel any unfilled orders from the previous frame
5. Create a new batch: `batch := acct.NewBatch(timestamp)`
6. Run `strategy.Compute(ctx, eng, acct, batch)`
7. Run middleware chain and submit: `acct.ExecuteBatch(ctx, batch)`
   - Each middleware in order calls `Process(ctx, batch)`
   - Final orders submitted to broker
   - Drain fill channel to record immediate fills as transactions

### 9. Built-in risk middleware

**Declarative constraints:**

- `MaxPositionSize(weight float64)` -- cap any single position at a percentage
  of portfolio value. Excess goes to cash.
- `DrawdownCircuitBreaker(threshold float64)` -- when portfolio drawdown from
  peak exceeds threshold, force-liquidate all equity positions to cash.
- `MaxPositionCount(n int)` -- limit the number of concurrent positions.
  Smallest positions are dropped first; excess goes to cash.

**Algorithmic:**

- `VolatilityScaler(ds DataSource, lookback int)` -- scale position sizes
  inversely to trailing realized volatility. `lookback` is in trading days.
  Each position's weight is scaled by `(1/vol_i) / sum(1/vol_j)` where
  `vol_i` is the annualized realized volatility of asset `i` over the
  lookback window. This produces an inverse-volatility-weighted portfolio
  where lower-volatility assets receive larger allocations. If volatility
  data is unavailable for an asset, its weight is unchanged.

**Convenience profiles:**

- `Conservative(ds DataSource)` -- 20% max position, 10% drawdown breaker,
  volatility scaling with 60 trading day lookback
- `Moderate(ds DataSource)` -- 25% max position, 15% drawdown breaker
- `Aggressive(ds DataSource)` -- 35% max position, 25% drawdown breaker

Algorithmic middleware and profiles that need market data receive a
`DataSource` interface at construction time. This is the same `DataSource`
interface used by strategies -- currently in the `universe` package but
moving to the `data` package per the weighting-strategies design spec.
This work depends on that move landing first. `DataSource` provides
`Fetch()`, `FetchAt()`, and `CurrentDate()`. Declarative constraints that
only need portfolio state take simple parameters.

### 10. Annotation conventions for risk middleware

When a middleware modifies the batch, it annotates with:

- **Key**: `risk:<middleware-name>` (e.g., `risk:max-position-size`)
- **Value**: human-readable description of the change (e.g.,
  `"capped AAPL from 38.2% to 25.0%, $13,200 moved to cash"`)

This provides a complete audit trail of every risk adjustment in the
portfolio's annotation log.

## Design decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Where middleware sits | Portfolio level, not broker | Portfolio is where orders originate; catches both `RebalanceTo` and `Order` paths |
| Excess allocation | Goes to cash | Overlay only reduces exposure, never amplifies |
| Middleware composition | User-defined ordered pipeline | Declarative and algorithmic are the same abstraction; runner controls order |
| Force-liquidation | Middleware can inject sell orders | Required for circuit breakers and exposure caps to be meaningful |
| Strategy data access for middleware | Via `DataSource` at construction | Keeps the `Middleware` interface simple; data needs are a construction concern |
| Risk configuration | Runner level, not strategy | Separates investment logic from risk discipline |
| Fill delivery | Buffered channel on broker | Non-blocking, works for both simulated and live brokers, any strategy frequency |
| Open order lifecycle | Cancelled at start of each frame | Each frame starts clean; strategy re-proposes if it still wants a limit order |
| Portfolio mutability | Only through batch execution | Prevents strategies from bypassing middleware |
| Annotation timestamps | `time.Time` on Batch, aligned with existing `Annotation` type | Current `Annotate()` uses `int64` timestamps; migrate to `time.Time` for consistency with `Batch.Timestamp`. This migration is in scope for this work. |
