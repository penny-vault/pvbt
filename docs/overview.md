# Overview

pvbt is Penny Vault's backtesting engine. It's a library, so it's easy to
integrate into your workflow--you don't have to run in the Penny Vault
environment to use it.

Backtesting is hard. You have to find your data. Manage orders & fills.
Compute portfolio metrics. And much more. None of this is what you want to be
doing. You want to build and evaluate awesome strategies. But instead, you end
up maintaining a lot of infrastructure.

pvbt seeks to fix this. It handles the infrastructure and lets you think about
investment strategies.

To write a new strategy you bring two things:

1) A Go struct that implements the `Strategy` interface
2) A `main` function that creates the engine and runs the backtest

Parameters are defined as exported struct fields with struct tags. No external configuration files are needed.

Here's an example strategy for Accelerating Dual Momentum:

```go
package main

import (
    "context"
    "time"

    "github.com/penny-vault/pvbt/engine"
    "github.com/penny-vault/pvbt/portfolio"
    "github.com/penny-vault/pvbt/signal"
    "github.com/penny-vault/pvbt/tradecron"
    "github.com/penny-vault/pvbt/universe"
    "github.com/rs/zerolog/log"
)

type ADM struct {
    RiskOn  universe.Universe `pvbt:"riskOn"  desc:"ETFs to invest in" default:"VOO,SCZ"`
    RiskOff universe.Universe `pvbt:"riskOff" desc:"Out-of-market asset" default:"TLT"`
}

func (s *ADM) Name() string { return "adm" }

func (s *ADM) Setup(e *engine.Engine) {
    tc, _ := tradecron.New("@monthend", tradecron.RegularHours)
    e.Schedule(tc)
    e.SetBenchmark(e.Asset("VFINX"))
    e.RiskFreeAsset(e.Asset("DGS3MO"))
}

func (s *ADM) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {
    mom1 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(1))
    mom3 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(3))
    mom6 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(6))

    // Average the three momentum scores across all risk-on assets.
    momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
    if err := momentum.Err(); err != nil {
        log.Error().Err(err).Msg("signal computation failed")
        return
    }

    // Pick the risk-on asset with the highest positive momentum.
    // If none are positive, fall back to the risk-off asset (TLT).
    riskOffDF, err := s.RiskOff.At(ctx, e.CurrentDate(), data.MetricClose)
    if err != nil {
        log.Error().Err(err).Msg("risk-off data fetch failed")
        return
    }
    portfolio.MaxAboveZero(data.MetricClose, riskOffDF).Select(momentum)

    // Build an equal-weight plan and rebalance into it.
    plan, err := portfolio.EqualWeight(momentum)
    if err != nil {
        log.Error().Err(err).Msg("EqualWeight failed")
        return
    }
    p.RebalanceTo(ctx, plan...)
}

func main() {
    eng := engine.New(&ADM{},
        engine.WithInitialDeposit(10_000),
        engine.WithDataProvider(provider),
        engine.WithAssetProvider(provider),
    )
    defer eng.Close()

    ctx := context.Background()
    start := time.Date(2005, time.January, 1, 0, 0, 0, 0, time.UTC)
    end := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

    p, err := eng.Backtest(ctx, start, end)
}
```

The rest of this page walks through what each piece does and what the engine does when it runs.

## Walking through the code

### The struct and Name

```go
type ADM struct {
    RiskOn  universe.Universe `pvbt:"riskOn"  desc:"ETFs to invest in" default:"VOO,SCZ"`
    RiskOff universe.Universe `pvbt:"riskOff" desc:"Out-of-market asset" default:"TLT"`
}

func (s *ADM) Name() string { return "adm" }
```

A strategy is any type that implements three methods: `Name`, `Setup`, and `Compute`. The struct holds whatever state the strategy needs between computation steps. ADM needs two **universes** -- named collections of assets. `RiskOn` holds the equity funds that the strategy picks from (VOO, SCZ by default). `RiskOff` holds the safe-haven asset it retreats to (TLT, long-term treasuries).

Parameters are defined as exported struct fields with struct tags. Before calling Setup, the engine uses reflection to find exported fields with `default` tags and populates them automatically. A `universe.Universe` field with a comma-separated ticker list as its default becomes a `StaticUniverse` built from those tickers and registered with the engine. Supported field types include `float64`, `int`, `string`, `bool`, `time.Duration`, `asset.Asset`, and `universe.Universe`.

The `pvbt` tag controls the CLI flag name. If omitted, the lowercase field name is used. The `desc` tag provides a description for help text. The `default` tag sets the default value.

### Setup

```go
func (s *ADM) Setup(e *engine.Engine) {
    tc, _ := tradecron.New("@monthend", tradecron.RegularHours)
    e.Schedule(tc)
    e.SetBenchmark(e.Asset("VFINX"))
    e.RiskFreeAsset(e.Asset("DGS3MO"))
}
```

Setup runs once, after the engine has populated the strategy's fields from their `default` tags.

`e.Schedule` sets the trading schedule. The tradecron expression `@monthend` means "the last trading day of each month." Tradecron understands market holidays and trading hours -- it will never fire on Christmas or a weekend. The schedule is required; the engine returns an error if Setup does not set one.

`e.SetBenchmark` tells the engine which asset to compare against in performance reports. `e.RiskFreeAsset` sets the risk-free rate used by metrics like Sharpe and Sortino. `e.Asset("DGS3MO")` looks up the 3-month treasury yield by ticker.

Setup is also where a strategy would register universes it creates itself (e.g., `universe.SP500(provider)`) or do any other one-time initialization.

### Compute

```go
func (s *ADM) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {
    mom1 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(1))
    mom3 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(3))
    mom6 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(6))

    // Average the three momentum scores across all risk-on assets.
    momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
    if err := momentum.Err(); err != nil {
        log.Error().Err(err).Msg("signal computation failed")
        return
    }

    // Pick the risk-on asset with the highest positive momentum.
    // If none are positive, fall back to the risk-off asset (TLT).
    riskOffDF, err := s.RiskOff.At(ctx, e.CurrentDate(), data.MetricClose)
    if err != nil {
        log.Error().Err(err).Msg("risk-off data fetch failed")
        return
    }
    portfolio.MaxAboveZero(data.MetricClose, riskOffDF).Select(momentum)

    // Build an equal-weight plan and rebalance into it.
    plan, err := portfolio.EqualWeight(momentum)
    if err != nil {
        log.Error().Err(err).Msg("EqualWeight failed")
        return
    }
    p.RebalanceTo(ctx, plan...)
}
```

Compute runs at each scheduled step -- once per month for ADM. It receives a context (which carries the logger via `zerolog.Ctx(ctx)`), the **engine** (for data fetching via `e.Fetch` and `e.FetchAt`), and the **portfolio** (the strategy's holdings, exposed as the `Portfolio` interface).

The first three lines compute momentum at 1-, 3-, and 6-month lookbacks using the `signal.Momentum` function. Each call takes the universe and a period, returning a new DataFrame of momentum scores. DataFrame arithmetic is element-wise: `mom1.Add(mom3).Add(mom6).DivScalar(3)` averages the three scores across all assets in one expression.

The selection step fetches risk-off asset data as a fallback DataFrame, then builds a `Selector` with `MaxAboveZero`. The selector's `Select` method inserts a `Selected` column into the momentum DataFrame, marking the asset with the highest positive momentum as selected at each timestep. If nothing qualifies, the fallback assets are inserted and marked as selected instead.

`portfolio.EqualWeight(momentum)` is the weighting step. It reads the `Selected` column and builds a `PortfolioPlan` with equal weights among selected assets at each timestep. Since ADM typically selects a single asset, this means 100% in that asset.

`p.RebalanceTo(ctx, plan...)` applies the plan. The portfolio diffs current holdings against the target and generates the necessary trades, applying commission and slippage, respecting any risk controls the operator has configured.

### main

```go
func main() {
    eng := engine.New(&ADM{},
        engine.WithInitialDeposit(10_000),
        engine.WithDataProvider(provider),
        engine.WithAssetProvider(provider),
    )
    defer eng.Close()

    ctx := context.Background()
    start := time.Date(2005, time.January, 1, 0, 0, 0, 0, time.UTC)
    end := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

    p, err := eng.Backtest(ctx, start, end)
}
```

The engine is created with the strategy and options via `engine.New`. `WithInitialDeposit` sets the starting cash -- the engine creates and manages the portfolio account internally. `WithDataProvider` registers data providers for market data. `WithAssetProvider` registers the provider used to resolve tickers to full `asset.Asset` values. `Backtest` takes a context (for cancellation) and start/end dates. It returns a `Portfolio` with its full history after the run. `Close` releases all resources including data provider connections. The same strategy code works for any time range, any starting capital, and -- critically -- in production with live data via `RunLive`.

For advanced use cases, `WithAccount` lets you pass a pre-configured `*portfolio.Account` directly, and `WithPortfolioSnapshot` restores from a previous run's snapshot.

## What the engine does when you run this

A backtest proceeds in four phases.

### Phase 1: Initialization

The engine loads the asset registry from the `AssetProvider`, then uses reflection to populate exported strategy fields from their `default` struct tags. It builds a routing table mapping each data metric to its provider. Then it calls `Setup`, where the strategy sets the schedule, benchmark, risk-free asset, and does any other one-time initialization. The engine creates a portfolio account from the initial deposit (or snapshot, or pre-configured account), attaches a simulated broker (unless one was provided), and initializes the per-column data cache.

### Phase 2: Date enumeration

The engine walks the tradecron schedule from start to end, collecting every trading date. For ADM with `@monthend`, that's roughly 240 dates from 2005 to 2024.

### Phase 3: Step loop

At each date the engine:
1. Fetches housekeeping data (close, adjusted close, dividends) for held assets, the benchmark, and the risk-free asset.
2. Records dividend income for held positions.
3. Updates the simulated broker with the current price provider and date so orders can fill.
4. Calls `ADM.Compute`. The strategy fetches data via `e.Fetch`, computes signals, and tells the portfolio to rebalance.
5. Fetches post-Compute prices for all held assets (including newly acquired positions) and updates the account's equity curve.
6. Computes all registered performance metrics across standard windows (5yr, 3yr, 1yr, YTD, MTD, WTD, and since-inception).
7. Evicts stale cache entries.

### Phase 4: Return

After the final step, the portfolio contains the full transaction log and can compute performance metrics. It provides access to the equity curve, every trade via `Transactions()`, individual metrics via `PerformanceMetric()`, and convenient bundles like `Summary()` and `RiskMetrics()`. Series data is returned as `[]float64`, compatible with gonum for further analysis.

## Logging

Strategies use [zerolog](https://github.com/rs/zerolog) for structured logging. The logger is carried on the context passed to `Compute`:

```go
func (s *ADM) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {
    log := zerolog.Ctx(ctx)

    log.Info().Str("strategy", s.Name()).Msg("computing")
}
```

The engine attaches a pre-configured logger to the context before calling `Compute`. Strategies should use `zerolog.Ctx(ctx)` rather than creating their own logger -- this ensures log output is consistent and correctly scoped to the current computation step.

## Design principles

Two principles shaped the API.

**Strategies should read like their plain-English descriptions.** Accelerating Dual Momentum computes 1-, 3-, and 6-month momentum on a set of risk-on assets, averages the scores, and invests in the highest-scoring asset if it's above zero -- otherwise it moves to a risk-off asset. The code says exactly that, in roughly the same number of words.

**The same code should work in a backtest and in production.** A strategy that runs against 20 years of historical data should deploy to a live trading system without modification. The API never exposes whether you're in a simulation or operating in real time.

## What comes next

The rest of the documentation walks through each concept in detail:

- [Universes](universes.md) covers the different ways to define asset groups, including predefined indexes and using other strategies as assets.
- [Data](data.md) explains metrics, data providers, DataFrames, and signals.
- [Portfolio](portfolio.md) covers portfolio construction, order types, and performance measurement.
- [Broker](broker.md) covers the broker interface for live trading with tastytrade and other brokerages.
- [Scheduling](scheduling.md) describes tradecron syntax and schedule configuration.
- [Configuration](configuration.md) explains struct tags and how to parameterize your strategy.
