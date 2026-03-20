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
| `WithPortfolioSnapshot(snap)` | Restore portfolio from a previous run's snapshot. |
| `WithAccount(acct *portfolio.Account)` | Use a pre-configured Account (overrides deposit, snapshot, and broker). |

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

1. **Initialization** -- loads assets, hydrates strategy fields from struct tags, builds the provider routing table, calls `strategy.Setup`, validates the schedule, and creates the portfolio account.
2. **Date enumeration** -- walks the tradecron schedule from start to end to build the list of trading dates.
3. **Step loop** -- for each trading date: fetches housekeeping data (dividends), records dividend income, updates the broker's price provider, calls `strategy.Compute`, fetches post-Compute prices, updates the equity curve, and computes performance metrics.
4. **Return** -- returns the portfolio with the full transaction log, equity curve, and computed metrics.

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
    Compute(ctx context.Context, eng *Engine, portfolio portfolio.Portfolio) error
}
```

**Name** returns a short identifier (e.g., `"adm"`, `"momentum-rotation"`).

**Setup** runs once after the engine populates strategy fields from their struct tags. If the strategy declares `Schedule` and `Benchmark` in `Describe()`, Setup only needs to handle other initialization. Otherwise, use Setup to set them imperatively:

```go
// Declarative approach (preferred): schedule and benchmark come from Describe().
func (s *ADM) Setup(eng *engine.Engine) {
    eng.RiskFreeAsset(eng.Asset("DGS3MO"))
}

// Imperative approach: still works, and overrides values from Describe().
func (s *ADM) Setup(eng *engine.Engine) {
    tc, _ := tradecron.New("@monthend", tradecron.RegularHours)
    eng.Schedule(tc)
    eng.SetBenchmark(eng.Asset("SPY"))
    eng.RiskFreeAsset(eng.Asset("DGS3MO"))
}
```

**Compute** runs at each scheduled trading date. It receives the engine (for data access) and the portfolio (for trading). Return an error to halt the backtest or signal a problem in live trading.

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
```

Most strategies fetch data through universes (`universe.Window`, `universe.At`) rather than calling `Fetch`/`FetchAt` directly. The universe methods are higher-level and handle membership resolution automatically.

### Schedule, benchmark, risk-free

```go
eng.Schedule(tc)                    // Set the trading schedule (required, call in Setup)
eng.SetBenchmark(eng.Asset("SPY"))  // Set the benchmark for performance comparison
eng.RiskFreeAsset(eng.Asset("SHV")) // Set the risk-free asset for Sharpe, etc.
```

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
    }
}
```

Call `engine.DescribeStrategy(strategy)` to get a `StrategyInfo` struct containing the strategy name, schedule, benchmark, parameters, and any preset suggestions. This does not require an engine or Setup to have run. The result is JSON-serializable for CLI and UI use.

## Resource cleanup

Always defer `Close` after creating an engine:

```go
eng := engine.New(strategy, opts...)
defer eng.Close()
```

This closes all registered data providers and releases resources.
