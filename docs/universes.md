# Universes

A universe is a collection of assets that a strategy operates on. It defines the investable space -- which instruments the strategy can see and trade.

Universes change over time. The S&P 500 adds and removes companies. ETFs get created and delisted. When the engine runs a strategy at a historical point in time, it resolves each universe to whatever its membership was on that date. This prevents survivorship bias -- you never accidentally trade a stock that didn't exist yet or had already been delisted.

## The Universe interface

```go
type Universe interface {
    Assets(t time.Time) []asset.Asset
    Window(ctx context.Context, lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error)
    At(ctx context.Context, metrics ...data.Metric) (*data.DataFrame, error)
    CurrentDate() time.Time
}
```

`Assets` returns the list of instruments in the universe at time `t`. The engine calls this at each computation step to resolve membership for the current simulation date.

`Window` fetches a DataFrame for the universe's current assets over a lookback period from the current date. This is the primary way signals get data for the universe.

`At` fetches a single-row DataFrame for the universe's assets at the current simulation date.

`CurrentDate` returns the simulation date the universe is currently positioned at.

## Choosing a universe

Most strategies that trade a broad US equity opportunity set should start with `universe.USTradable(p)`. It is a daily-refreshed set of liquid US stocks that meet standard tradability criteria: market cap above the 25th percentile of US-listed common stocks, $2.5M median dollar volume over the trailing 200 days, prior close of at least $5, and 200 days of contiguous price history. Recent IPOs, ADRs, limited partnerships, ETFs, and OTC stocks are excluded, and for companies with multiple share classes only the most liquid common share is kept.

Use `SP500` or `Nasdaq100` only when you specifically want to track those indexes. Use `NewStatic` for fixed asset lists like ETF rotations.

## Creating universes

There are three ways to create a universe, depending on where the assets come from.

### From struct tags

The most common case. The strategy declares exported `universe.Universe` fields with `default` tags containing comma-separated tickers:

```go
type ADM struct {
    RiskOn  universe.Universe `pvbt:"riskOn"  desc:"ETFs to invest in"   default:"VOO,SCZ"`
    RiskOff universe.Universe `pvbt:"riskOff" desc:"Out-of-market asset" default:"TLT"`
}
```

The engine uses reflection to find exported fields with `default` tags, parses the comma-separated tickers, builds a `StaticUniverse`, and registers it with the engine automatically before calling Setup. The `pvbt` tag controls the CLI flag name and the `desc` tag provides help text. This is the preferred approach when the asset list should be user-configurable.

### From explicit tickers

When a strategy needs a fixed set of assets that isn't user-configurable:

```go
s.hedges = universe.NewStatic("GLD", "TLT")
```

This creates a `StaticUniverse` -- membership does not change over time.

Tickers can include a namespace prefix to specify the data source:

```go
s.rates = universe.NewStatic("FRED:DGS3MO", "FRED:DGS10")
```

### From a predefined index

For broad equity strategies, the recommended starting point:

```go
s.stocks = universe.USTradable(indexProvider)
```

For well-known indexes whose membership changes over time:

```go
s.stocks = universe.SP500(indexProvider)
s.tech = universe.Nasdaq100(indexProvider)
```

These constructors take a `data.IndexProvider` -- a provider that knows how to supply historical index membership. The database provider implements this interface alongside `BatchProvider`. The returned universe's membership varies by date. If you backtest a strategy that uses `universe.SP500(p)` starting in 2010, the universe in January 2010 will contain whatever companies were in the S&P 500 at that time -- not today's list.

Index universes delegate to the data provider, which loads all snapshot and changelog data on the first call and advances as time progresses. The returned membership slice is borrowed and only valid for the current engine step.

## Getting data for a universe

The primary use of a universe is to get a DataFrame for its assets. The engine resolves `u.Assets(t)` into a `DataRequest`, fetches the data from providers, and hands the strategy a DataFrame. From there, the strategy operates on the DataFrame:

```go
func (s *ADM) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    mom1 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(1))
    mom3 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(3))
    mom6 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(6))

    momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
    // ...
    return nil
}
```

Filtering, ranking, and selection all happen through the DataFrame API. See [Data](data.md) for details.

## Strategies as assets

A strategy's equity curve is itself a time series. The engine can expose it as a synthetic asset, which means one strategy can be included in another strategy's universe:

```go
func (s *Blend) Setup(e *engine.Engine) {
    s.strategies = universe.NewStatic("strategy:ADM", "strategy:Value", "strategy:Growth")
}
```

The `strategy:` prefix tells the engine to run that strategy first and expose its equity curve as the price series. From the meta-strategy's perspective these are just assets with price data in a DataFrame.

> **Note:** The `strategy:` prefix is a planned feature and may not be fully implemented yet.

## Universe membership and time

A subtle but important point: universe membership is resolved at each computation step, not at setup time. During setup you declare which universe you want. During computation the engine calls `Assets(t)` to determine what's in it right now.

This means a strategy that uses `universe.SP500(p)` might see 498 stocks on one date and 503 on another, depending on index changes. The strategy doesn't need to care about this -- it operates on whatever assets are currently available.

For universes created from explicit tickers or configuration, membership is fixed. A universe created with `universe.NewStatic("SPY", "TLT")` always contains those two assets. But the engine still checks that each asset has valid data at each point in time -- if an ETF hadn't been created yet, it won't appear in the universe during that period.
