# Engine

The engine orchestrates everything: loading data, stepping through time, calling your strategy, tracking performance. Strategy authors interact with it during Setup and Compute. Users configure it with options at construction time.

## Creating an engine

Pass your strategy and configuration options to `engine.New`:

```go
eng := engine.New(&MyStrategy{},
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(assetProvider),
    engine.WithInitialDeposit(100_000),
)
defer eng.Close()
```

### Options

| Option | Purpose |
|--------|---------|
| `WithDataProvider(providers ...data.DataProvider)` | Register data providers. Call multiple times for multiple providers. |
| `WithAssetProvider(p data.AssetProvider)` | Set the asset provider for ticker resolution. Required. |
| `WithInitialDeposit(amount float64)` | Starting cash balance. |
| `WithBroker(b broker.Broker)` | Broker for order execution. Defaults to a simulated broker. |
| `WithCacheMaxBytes(n int64)` | Maximum memory for the data cache. Defaults to 512MB. |
| `WithPortfolioSnapshot(snap)` | Restore portfolio from a previous run's snapshot. Mutually exclusive with `WithInitialDeposit`. |
| `WithAccount(acct portfolio.PortfolioManager)` | Use a pre-configured Account (overrides deposit, snapshot, and broker). |
| `WithDateRangeMode(mode DateRangeMode)` | How to handle insufficient warmup data: `DateRangeModeStrict` (default) errors, `DateRangeModePermissive` adjusts the start date forward. |
| `WithBenchmarkTicker(ticker string)` | Override the benchmark by ticker. Takes priority over the benchmark declared in `Describe()`. |
| `WithFillModel(base broker.BaseModel, adjusters ...broker.Adjuster)` | Configure the fill pipeline used by the simulated broker. Ignored when `WithBroker` supplies a non-default broker. |
| `WithMiddlewareConfig(cfg MiddlewareConfig)` | Attach risk- and tax-management middleware (see `pvbt.toml` / `--risk-profile` / `--tax`). Replaces any strategy-declared middleware. |
| `WithProgressCallback(fn ProgressCallback)` | Register a callback invoked after each simulation step with a `ProgressEvent` (step index, total steps, current date, measurement count). Must return quickly; runs inside the step loop. |

## Running a backtest

```go
result, err := eng.Backtest(ctx, start, end)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Final value: $%.2f\n", result.Value())
summary, _ := result.Summary()
fmt.Printf("Sharpe: %.2f\n", summary.Sharpe)
```

The backtest proceeds in four phases:

1. **Initialization** -- loads assets, hydrates strategy fields from struct tags, builds the provider routing table, calls `strategy.Setup`, validates the schedule, checks warmup data availability (see [Warmup](#warmup)), and creates the portfolio account.
2. **Date enumeration** -- walks the tradecron schedule from start to end to build the list of trading dates.
3. **Step loop** -- iterates every engine date (see [Steps and frames](#steps-and-frames) below). At each step: drains the broker fill channel, syncs broker-reported transactions (dividends, splits, borrow fees, delistings), runs a margin check, and updates the equity curve. On frames (dates matching the trading schedule), the engine also cancels open orders, creates a fresh batch, calls `strategy.Compute`, runs the batch through middleware, and submits the resulting orders to the broker.
4. **Return** -- returns the portfolio with the full transaction log, equity curve, and computed metrics.

## Steps and frames

The engine uses two levels of iteration:

- **Step** -- every engine iteration (typically every trading day). At each step the engine drains broker fills, records dividends, and updates the equity curve and performance metrics.
- **Frame** -- a step where the trading schedule fires. At a frame the engine cancels open orders, creates a `Batch`, calls `strategy.Compute`, executes the batch through the middleware chain, and submits orders to the broker.
- **Batch** -- the container for a single frame's orders and annotations. Created fresh at the start of each frame and discarded after submission. Strategy code writes to the batch; middleware transforms it.

### Housekeeping order within each step

At every step the engine performs housekeeping in a fixed order before updating the equity curve:

1. **Drain fills** -- consume all pending fills from the broker's `Fills()` channel and apply them to the portfolio.
2. **Apply splits** -- adjust share quantities and cost bases for any stock splits effective on the current date.
3. **Borrow fees** -- debit daily borrow fees for all open short positions (see [broker.md](broker.md)).
4. **Dividends** -- credit dividend income and adjust short positions for dividend obligations.
5. **Margin check** -- verify that maintenance margin requirements are met. The margin check runs every trading day regardless of whether the current step is a frame (i.e., whether the strategy fires). If the check fails, the `MarginCallHandler` is invoked.

### Margin calls and auto-liquidation

When a margin check fails, the engine first looks for a `MarginCallHandler` on the strategy itself. The interface is opt-in -- implement it on your strategy type to take over margin-call handling:

```go
type MarginCallHandler interface {
    OnMarginCall(ctx context.Context, eng *Engine, port portfolio.Portfolio, batch *portfolio.Batch) error
}
```

The handler receives the engine, the current portfolio (read-only), and a dedicated batch on which to queue orders. The batch bypasses middleware (it must not be further transformed by risk/tax rules) and is executed immediately after `OnMarginCall` returns. If the handler clears the deficiency the step continues; otherwise the engine falls back to automatic liquidation.

If the strategy does not implement `MarginCallHandler`, the engine auto-liquidates short positions proportionally until the deficiency is resolved. Returning an error from the handler halts the run.

### Stock split handling

When a split is effective on the current date, the engine adjusts all affected lots before any other housekeeping for that step. For long lots the share count is multiplied by the split ratio and the cost basis per share is divided by the same ratio. For short lots the liability is adjusted symmetrically: the short share count increases and the proceeds-per-share decrease, so the net position value is unchanged. Any fractional shares produced by the split are settled as cash at the post-split price.

## Live trading

```go
ch, err := eng.RunLive(ctx)
if err != nil {
    log.Fatal(err)
}

for portfolio := range ch {
    fmt.Printf("Value: $%.2f\n", portfolio.Value())
}
```

`RunLive` performs the same initialization as `Backtest`, then launches a goroutine that fires on each scheduled time. The returned channel receives the portfolio after each step. Cancel the context to stop.

## Strategy interface

Strategies implement three methods:

```go
type Strategy interface {
    Name() string
    Setup(eng *Engine)
    Compute(ctx context.Context, eng *Engine, portfolio portfolio.Portfolio, batch *portfolio.Batch) error
}
```

**Name** returns a short identifier (e.g., `"adm"`, `"momentum-rotation"`).

**Setup** runs once after the engine populates strategy fields from their struct tags. If the strategy declares `Schedule` and `Benchmark` in `Describe()`, Setup only needs to handle other initialization:

```go
// Declarative approach (preferred): schedule and benchmark come from Describe().
func (s *ADM) Setup(eng *engine.Engine) {}

func (s *ADM) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        Schedule:  "@monthend",
        Benchmark: "SPY",
    }
}

// Imperative benchmark override: SetBenchmark in Setup overrides the value from Describe().
func (s *ADM) Setup(eng *engine.Engine) {
    eng.SetBenchmark(eng.Asset("SPY"))
}
```

The risk-free rate (DGS3MO, the 3-month treasury yield) is resolved automatically by the engine during initialization when available.

**Compute** runs at each scheduled trading date. It receives the engine (for data access), the portfolio (read-only, for inspecting holdings and performance), and a batch (for accumulating orders and annotations). Return an error to halt the backtest or signal a problem in live trading.

## Engine methods for strategy authors

These methods are available to strategies during Setup and Compute:

### Asset lookup

```go
spy := eng.Asset("SPY")
```

Resolves a ticker to an `asset.Asset` from the pre-loaded registry. Panics if the ticker is unknown (fail-fast -- catch typos immediately).

### Universes

```go
// Static universe from explicit assets
riskOn := eng.Universe(eng.Asset("SPY"), eng.Asset("EFA"), eng.Asset("EEM"))

// Rated universe from analyst ratings
rated := eng.RatedUniverse("morningstar", data.RatingLTE(3))
```

Universes define the investable space. The engine wires them to its data layer so they can fetch data through `Window` and `At` methods.

### Current date

```go
date := eng.CurrentDate()
```

Returns the current simulation date during Compute. Use this for date-dependent logic.

### Data fetching

```go
// Historical window
df, err := eng.Fetch(ctx, assets, portfolio.Months(6), []data.Metric{data.MetricClose})

// Single point in time
df, err := eng.FetchAt(ctx, assets, eng.CurrentDate(), []data.Metric{data.MetricClose})

// Specific fundamentals reporting period (e.g. Q4 prior year)
df, err := eng.FetchFundamentalsByDateKey(ctx, assets, []data.Metric{data.WorkingCapital}, q4)
```

Most strategies fetch data through universes (`universe.Window`, `universe.At`) rather than calling `Fetch`/`FetchAt` directly. The universe methods are higher-level and handle membership resolution automatically.

`FetchFundamentalsByDateKey` returns one value per asset for a given `date_key`, filtered to filings available by `eng.CurrentDate()`. Pass `engine.WithAsOfDate(t)` to cap availability at an earlier formation date (must be non-zero and not later than `CurrentDate()`). See the strategy guide for a worked example.

### Fundamental dimension

```go
eng.SetFundamentalDimension("MRQ")
```

Selects the dimension used for all subsequent fundamental fetches on this engine. Valid values: `ARQ`, `ARY`, `ART` (As Reported, point-in-time on SEC filing date) and `MRQ`, `MRY`, `MRT` (Most Recent Reported, indexed to fiscal period end). Call from `Setup`; defaults to `ARQ`. See the strategy guide for the full table and guidance on backtest correctness.

### Benchmark

```go
eng.SetBenchmark(eng.Asset("SPY"))  // Set the benchmark for performance comparison
```

The benchmark can also be declared in `Describe()` via the `Benchmark` field. A value set in `Setup` overrides it. The trading schedule is set via the `Schedule` field in `Describe()`. The risk-free rate (DGS3MO) is resolved automatically by the engine.

## Previewing upcoming trades

Strategies that trade infrequently (e.g., monthly) leave users wondering what the next trade will be. `PredictedPortfolio` answers this by running the strategy's Compute against a shadow copy of the portfolio using the next scheduled trade date:

```go
predicted, err := eng.PredictedPortfolio(ctx)
if err != nil {
    log.Fatal(err)
}

for _, tx := range predicted.Transactions() {
    fmt.Printf("%s %s %.0f shares\n", tx.Type, tx.Asset.Ticker, tx.Qty)
}
```

The engine clones the current portfolio, advances the date to the next scheduled trade, and forward-fills any data gaps by copying the last available prices forward day-by-day. The strategy is completely unaware it is a prediction run.

The returned portfolio includes transactions, annotations, and justifications from the prediction run. The original portfolio is not mutated.

Call it after a backtest completes or during live operation. It works with any schedule frequency -- daily, weekly, monthly, or custom tradecron expressions.

## Strategy metadata

Strategies can optionally implement the `Descriptor` interface to provide metadata. The `Schedule` and `Benchmark` fields let the strategy declare these values declaratively, so they do not need to be set in `Setup`:

```go
func (s *ADM) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        ShortCode:   "adm",
        Description: "Accelerating Dual Momentum",
        Source:      "https://example.com/adm",
        Version:     "1.0",
        Schedule:    "@monthend",
        Benchmark:   "SPY",
        Warmup:      126, // need 126 trading days of data before first compute
    }
}
```

For meta-strategies, child strategies do not need to be listed in `Describe()`. The engine discovers `Children` automatically by scanning struct fields with `weight` tags and includes them in the `StrategyInfo` returned by `DescribeStrategy`.

Call `engine.DescribeStrategy(strategy)` to get a `StrategyInfo` struct containing the strategy name, schedule, benchmark, parameters, preset suggestions, and any discovered children. This does not require an engine or Setup to have run. The result is JSON-serializable for CLI and UI use.

## Warmup

Strategies that compute indicators over historical windows (e.g., a 200-day moving average) need data before the first compute date. The `Warmup` field on `StrategyDescription` declares how many trading days of prior data the strategy requires:

```go
func (s *ADM) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        Schedule: "@monthend",
        Warmup:   126, // 6 months of trading days
    }
}
```

During backtest initialization the engine checks that every asset in the strategy's universes and asset fields has at least `Warmup` non-NaN close prices in the window before the first scheduled trade date. The behavior depends on `DateRangeMode`:

- **Strict (default)** -- the engine returns an error listing which assets are short and by how many days.
- **Permissive** -- the engine shifts the start date forward one trading day at a time until all assets have enough data. If no valid start date exists before the end date, it returns an error.

A warmup of 0 (the default) skips the check entirely.

## Meta-strategies

A meta-strategy allocates across child strategies rather than directly buying individual assets. The engine discovers child strategies automatically from struct fields tagged with `weight`.

### Declaring children

Each child is a pointer field with a `weight` tag. Optional `preset` and `params` tags configure the child's parameters:

```go
type MyMeta struct {
    ADM *adm.ADM `weight:"0.10" preset:"Classic"`
    BAA *baa.BAA `weight:"0.40"`
    DAA *daa.DAA `weight:"0.50"`
}
```

The `weight` values are the fractional portfolio allocations (they must sum to 1.0). `preset` names a parameter preset defined in the child's `Describe()`. `params` passes individual key=value overrides as a semicolon-separated list (e.g. `params:"lookback=12;threshold=0.02"`).

### Engine initialization

During backtest initialization the engine discovers all `weight`-tagged fields, registers the children as sub-engines, and validates their combined setup. Children always run before the parent: the engine fires each child's Compute on its own schedule, then fires the parent's Compute on the parent's schedule.

### ChildAllocations

`eng.ChildAllocations()` expands each child's current holdings into a single `portfolio.Allocation` with asset weights scaled by the child's portfolio weight. Use this inside the parent's Compute to construct an allocation:

```go
func (s *MyMeta) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    alloc, err := eng.ChildAllocations()
    if err != nil {
        return err
    }
    return batch.RebalanceTo(ctx, alloc)
}
```

**Worked example.** If the parent holds ADM at 10%, BAA at 40%, and DAA at 50%, and on a given day ADM is fully in SPY, BAA holds 60% SPY / 40% TLT, and DAA holds 80% QQQ / 20% cash, then `ChildAllocations` returns:

| Asset  | Weight |
|--------|--------|
| SPY    | 0.10 × 1.00 + 0.40 × 0.60 = 0.34 |
| TLT    | 0.40 × 0.40 = 0.16 |
| QQQ    | 0.50 × 0.80 = 0.40 |
| $CASH  | 0.50 × 0.20 = 0.10 |

`$CASH` is a sentinel ticker. `RebalanceTo` leaves that fraction uninvested (as cash).

### ChildPortfolios

`eng.ChildPortfolios()` returns a map from child strategy name to its current `portfolio.Portfolio`. Inspect this for dynamic weight adjustments:

```go
portfolios := eng.ChildPortfolios()
// Inspect portfolios["adm"], portfolios["baa"], etc.
```

You can pass a weight override map to `ChildAllocations` to dynamically adjust allocations without changing the struct tags:

```go
overrides := map[string]float64{"adm": 0.05, "baa": 0.45, "daa": 0.50}
alloc, err := eng.ChildAllocations(overrides)
```

## Resource cleanup

Always defer `Close` after creating an engine:

```go
eng := engine.New(strategy, opts...)
defer eng.Close()
```

This closes all registered data providers and releases resources.
