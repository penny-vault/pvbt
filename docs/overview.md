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

To write a new strategy you bring 3 things:

1) A configuration file that describes what parameters users can pass to your
   strategy.
2) A description of the strategy (optional)
3) The strategy code

Here's an example strategy for Accelerating Dual Momentum:

```toml
name = "Accelerating Dual Momentum"
shortcode = "adm"
description = "A market timing strategy that uses a 1-, 3-, and 6-month momentum score to select assets."
source = "https://engineeredportfolio.com/2018/05/02/accelerating-dual-momentum-investing/"
version = "1.1.0"
benchmark = "VFINX"
schedule = "@monthend"

[arguments.riskOn]
name = "Tickers"
description = "List of ETF, Mutual Fund, or Stock tickers to invest in"
typecode = "[]stock"
default = ["VOO", "SCZ"]

[arguments.riskOff]
name = "Out-of-Market Tickers"
description = "Ticker to use when model scores are all below 0"
typecode = "stock"
default = "TLT"

[suggested."Engineered Portfolio"]
riskOn = ["VFINX", "PRIDX"]
riskOff = ["VUSTX"]
```

This configuration sets some metadata and lets the strategy author define
arguments that users of the strategy can change. This information is all used
by the UI to make the user experience better.

On to the strategy itself:

```go
package main

import (
    "context"
    "time"

    "github.com/penny-vault/pvbt/data"
    "github.com/penny-vault/pvbt/engine"
    "github.com/penny-vault/pvbt/portfolio"
    "github.com/penny-vault/pvbt/universe"
)

type ADM struct {
    riskOn  universe.Universe
    riskOff universe.Universe
}

func (s *ADM) Name() string { return "adm" }

func (s *ADM) Setup(e *engine.Engine, config engine.Config) {
    e.RiskFreeAsset(e.Asset("DGS3MO"))
}

func (s *ADM) Compute(ctx context.Context, p portfolio.Portfolio) {
    mom1 := signal.Momentum(df, 1)
    mom3 := signal.Momentum(df, 3)
    mom6 := signal.Momentum(df, 6)

    momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
    symbols := momentum.Select(portfolio.MaxAboveZero(s.riskOff))

    p.RebalanceTo(portfolio.EqualWeight(symbols)...)
}

func main() {
    acct := portfolio.New(portfolio.WithCash(10_000))
    e := engine.New(&ADM{})
    defer e.Close()

    ctx := context.Background()
    start := time.Date(2005, time.January, 1, 0, 0, 0, 0, time.UTC)
    end := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

    acct, err := e.Run(ctx, acct, start, end)
}
```

The rest of this page walks through what each piece does and what the engine does when it runs.

## Walking through the code

### The struct and Name

```go
type ADM struct {
    riskOn  universe.Universe
    riskOff universe.Universe
}

func (s *ADM) Name() string { return "adm" }
```

A strategy is any type that implements three methods: `Name`, `Setup`, and `Compute`. The struct holds whatever state the strategy needs between computation steps. ADM needs two **universes** -- named collections of assets. `riskOn` holds the equity funds that the strategy picks from (VFINX, PRIDX by default). `riskOff` holds the safe-haven asset it retreats to (VUSTX, long-term treasuries).

The field names match the argument names in the TOML file. Before calling Setup, the engine uses reflection to find these fields, parses the TOML argument values, and populates them automatically. A `universe.Universe` field with a `[]stock` typecode becomes a `StaticUniverse` built from the configured tickers and is automatically registered with the engine. If the field type doesn't match the typecode (e.g., a `float64` field for a `[]stock` argument), the engine panics at startup with a clear error.

If a field name doesn't match the TOML argument name, use a struct tag: `pvbt:"tomlName"`.

### Setup

```go
func (s *ADM) Setup(e *engine.Engine, config engine.Config) {
    e.RiskFreeAsset(e.Asset("DGS3MO"))
}
```

Setup runs once, after the engine has populated the strategy's fields from the TOML and registered any universe fields.

`e.RiskFreeAsset` tells the engine which asset to use as the risk-free rate. The engine fetches its data alongside the universe members, and signals like `Momentum` automatically subtract it from returns. `e.Asset("DGS3MO")` looks up the 3-month treasury yield by ticker.

Setup is also where a strategy would register universes it creates itself (e.g., `universe.SP500(provider)`), override the schedule, or do any other one-time initialization. The schedule is set in the TOML file (`schedule = "@monthend"`). The tradecron expression `@monthend` means "the last trading day of each month." Tradecron understands market holidays and trading hours -- it will never fire on Christmas or a weekend.

### Compute

```go
func (s *ADM) Compute(ctx context.Context, p portfolio.Portfolio) {
    mom1 := signal.Momentum(df, 1)
    mom3 := signal.Momentum(df, 3)
    mom6 := signal.Momentum(df, 6)

    momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
    symbols := momentum.Select(portfolio.MaxAboveZero(s.riskOff))

    p.RebalanceTo(portfolio.EqualWeight(symbols)...)
}
```

Compute runs at each scheduled step -- once per month for ADM. It receives a context (which carries the logger via `zerolog.Ctx(ctx)`) and the **portfolio** (the strategy's holdings, exposed as the `Portfolio` interface).

The first three lines compute momentum at 1-, 3-, and 6-month lookbacks using the `signal.Momentum` function. Each call takes a DataFrame and a period count, returning a new DataFrame of momentum scores. DataFrame arithmetic is element-wise: `mom1.Add(mom3).Add(mom6).DivScalar(3)` averages the three scores across all assets in one expression.

`momentum.Select(portfolio.MaxAboveZero(s.riskOff))` is the selection step. It filters the DataFrame to the asset with the highest momentum, but only if that score is above zero. If no risk-on asset has positive momentum, the selection falls back to the risk-off universe. The result is a filtered DataFrame.

`portfolio.EqualWeight(symbols)` is the weighting step. It takes the filtered DataFrame and builds a `PortfolioPlan` with equal weights at each timestep. Since ADM typically selects a single asset, this means 100% in that asset.

`p.RebalanceTo(plan...)` applies the plan. The portfolio diffs current holdings against the target and generates the necessary trades, applying commission and slippage, respecting any risk controls the operator has configured.

### main

```go
func main() {
    acct := portfolio.New(portfolio.WithCash(10_000))
    e := engine.New(&ADM{})
    defer e.Close()

    ctx := context.Background()
    start := time.Date(2005, time.January, 1, 0, 0, 0, 0, time.UTC)
    end := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

    acct, err := e.Run(ctx, acct, start, end)
}
```

The portfolio is created with initial cash via `portfolio.New(portfolio.WithCash(10_000))`. The engine is created with the strategy via `engine.New`. `Run` takes a context (for cancellation), the portfolio, and start/end dates. The portfolio is returned with its full history after the run. `Close` releases all resources including data provider connections. The same strategy code works for any time range, any starting capital, and -- critically -- in production with live data via `RunLive`.

## What the engine does when you run this

A backtest proceeds in three phases.

### Setup

Before calling Setup, the engine parses the strategy TOML and populates the strategy struct's fields via reflection. It matches fields to TOML arguments by name (or `pvbt` struct tag), validates that Go types are compatible with typecodes, and parses the string values into the correct types. Any `universe.Universe` fields are automatically registered with the engine.

When the engine then calls `ADM.Setup`, the strategy can do any additional initialization. For ADM, Setup sets the risk-free asset to the 3-month treasury yield. The universes came from configuration and were registered automatically. The engine reads the schedule from the TOML and configures tradecron.

### Computation

The engine prefetches all registered universes for the backtest time range, then steps through time according to the schedule -- the last trading day of each month from 2005 to 2024, roughly 240 steps. At each step it resolves universe membership (this matters for index-based universes like the S&P 500 where companies enter and leave over time -- for ADM's fixed ticker lists it's straightforward), makes metric data available as DataFrames for the current period, and calls `ADM.Compute`.

The strategy computes its momentum scores, selects an asset, and tells the portfolio to rebalance. The engine then processes the resulting trades. If the strategy was holding VFINX and now wants PRIDX, the engine sells the VFINX position and buys PRIDX, applying commission and slippage models configured by the operator.

### Results

After the final step, the portfolio contains the full transaction log and can compute performance metrics. It provides access to the equity curve, every trade via `Transactions()`, individual metrics via `PerformanceMetric()`, and convenient bundles like `Summary()` and `RiskMetrics()`. Series data is returned as `[]float64`, compatible with gonum for further analysis.

## Logging

Strategies use [zerolog](https://github.com/rs/zerolog) for structured logging. The logger is carried on the context passed to `Compute`:

```go
func (s *ADM) Compute(ctx context.Context, p portfolio.Portfolio) {
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
- [Configuration](configuration.md) explains the TOML file format and how to parameterize your strategy.
