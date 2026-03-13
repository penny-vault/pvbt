# Engine Package Design

## Overview

The engine orchestrates data access, computation scheduling, and portfolio management
for both backtesting and live trading. It is the central coordinator: it owns the
simulation loop, manages data caching, creates universes and resolves assets, and
provides a simulated broker for backtesting.

## Design Decisions

These decisions were made during brainstorming and are not open for revisiting:

1. **`Backtest` not `Run`** -- the backtest method is named `Backtest` for clarity.
2. **Backtest always creates a simulated broker** -- overrides whatever broker was on
   the account. The simulated broker fills all order types at the close price.
3. **`RunLive` requires a broker** -- returns an error if no broker is attached to
   the account. The caller may attach a `SimulatedBroker` for paper trading.
4. **No TOML** -- strategy parameters are declared as exported struct fields with
   struct tags (`pvbt`, `desc`, `default`). The strategy struct is the source of truth.
   `engine.StrategyParameters(s)` reflects over the struct for discovery (CLI flags,
   UI generation, TOML export).
5. **Engine hydrates strategy fields** -- the engine reflects over the strategy struct
   at startup and populates fields from their `default` tags. `asset.Asset` fields are
   resolved via the asset registry. `universe.Universe` fields are built via
   `e.Universe(assets...)` so they are wired to the engine for data fetching.
6. **Engine is the factory for universes** -- `e.Universe(assets...)` creates a
   universe with data fetching wired in. Index universes are also created through the
   engine. The `universe.Universe` interface has data access methods (`Window`, `At`)
   that delegate to the engine.
7. **No `Register` method** -- the engine discovers universes through their usage
   (universe data methods call back to the engine).
8. **Asset registry loaded from `data.AssetProvider`** -- the engine bulk-loads all
   assets at startup. `e.Asset(ticker)` does a map lookup. Falls back to
   `LookupAsset` on cache miss. Panics if unresolvable.
9. **All matching on CompositeFigi** -- asset identity throughout the system uses
   `CompositeFigi`, not ticker.
10. **Dividends from data** -- the engine includes `data.Dividend` in housekeeping
    fetches for held assets at each step. Non-zero values are recorded as
    `DividendTransaction`.
11. **Smart data fetching with sliding window** -- on the first `DataFrame`-equivalent
    call, the engine knows the lookback (from the request) and the remaining schedule
    (from the backtest range). It fetches `[currentDate - lookback, end]` in chunks
    sized to fit memory. A sliding window of `[currentDate - lookback, currentDate]`
    determines what stays resident. Data behind the window is evicted. Upcoming
    chunks are pre-fetched.
12. **Universe is the data interface for strategies** -- strategies call
    `s.riskOn.Window(Months(6), data.MetricClose)` or `s.riskOn.At(today)`. Signals
    call universe methods internally. The engine's public `DataFrame` method is
    removed; all data flows through universes.

## Strategy Interface

```go
type Strategy interface {
    Name() string
    Setup(e *Engine)
    Compute(ctx context.Context, e *Engine, p portfolio.Portfolio)
}
```

Strategy structs declare parameters as exported fields with struct tags:

```go
type ADM struct {
    RiskOn  universe.Universe `pvbt:"riskOn" desc:"Tickers to invest in" default:"VFINX,PRIDX"`
    RiskOff asset.Asset       `pvbt:"riskOff" desc:"Out-of-market ticker" default:"VUSTX"`
}

func (s *ADM) Name() string { return "adm" }

func (s *ADM) Setup(e *engine.Engine) {
    e.Schedule(tradecron.MustNew("@monthend", tradecron.RegularHours))
    e.RiskFreeAsset(e.Asset("DGS3MO"))
    e.SetBenchmark(e.Asset("VFINX"))
}

func (s *ADM) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {
    df := s.RiskOn.Window(portfolio.Months(6), data.MetricClose)
    mom1 := signal.Momentum(df, 1)
    mom3 := signal.Momentum(df, 3)
    mom6 := signal.Momentum(df, 6)
    momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
    symbols := momentum.Select(portfolio.MaxAboveZero(s.RiskOff))
    p.RebalanceTo(portfolio.EqualWeight(symbols)...)
}
```

## Engine Struct

```go
type Engine struct {
    strategy      Strategy
    providers     []data.DataProvider
    assetProvider data.AssetProvider
    schedule      *tradecron.TradeCron
    riskFree      asset.Asset
    benchmark     asset.Asset

    // configuration (set via options, used during init)
    cacheMaxBytes  int64         // default 512MB
    cacheChunkSize time.Duration // default 1 year

    // populated at Backtest/RunLive start
    assets      map[string]asset.Asset   // ticker -> full Asset
    cache       *dataCache               // sliding window, memory-based
    currentDate time.Time                // current simulation step
    ctx         context.Context          // current step context

    // backtest range
    start time.Time
    end   time.Time

    // provider routing
    metricProvider map[data.Metric]data.BatchProvider // metric -> provider (first wins)
}
```

## Options

```go
func WithDataProvider(providers ...data.DataProvider) Option
func WithAssetProvider(p data.AssetProvider) Option
func WithCacheMaxBytes(n int64) Option    // default 512MB
func WithChunkSize(d time.Duration) Option // default 1 year
```

## AssetProvider Interface (new, in data package)

```go
type AssetProvider interface {
    Assets(ctx context.Context) ([]asset.Asset, error)
    LookupAsset(ctx context.Context, ticker string) (asset.Asset, error)
}
```

Provided to the engine via `WithAssetProvider`. The engine calls `Assets(ctx)` at
startup to bulk-load the registry into a `map[string]asset.Asset` keyed by ticker.
`Engine.Asset(ticker)` does a map lookup. On miss, calls `LookupAsset` and caches.
Panics if the ticker cannot be resolved.

## DataSource Interface (new, in universe package)

Breaks the circular dependency between `engine` and `universe`. The engine implements
this interface; universe implementations hold a reference to it.

```go
type DataSource interface {
    Fetch(ctx context.Context, assets []asset.Asset, lookback portfolio.Period,
          metrics []data.Metric) (*data.DataFrame, error)
    FetchAt(ctx context.Context, assets []asset.Asset, t time.Time,
            metrics []data.Metric) (*data.DataFrame, error)
    CurrentDate() time.Time
}
```

## Universe Interface (revised)

```go
type Universe interface {
    // Assets returns the members of the universe at time t.
    Assets(t time.Time) []asset.Asset

    // Prefetch loads membership data for the given range.
    Prefetch(ctx context.Context, start, end time.Time) error

    // Window returns a DataFrame covering [currentDate - lookback, currentDate]
    // for the requested metrics. Resolves membership at currentDate.
    Window(lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error)

    // At returns a single-row DataFrame at the given time for the requested metrics.
    At(t time.Time, metrics ...data.Metric) (*data.DataFrame, error)
}
```

Universe implementations hold a `DataSource` reference. `Window` calls
`ds.Fetch(ctx, u.Assets(ds.CurrentDate()), lookback, metrics)`. `At` calls
`ds.FetchAt(ctx, u.Assets(t), t, metrics)`.

### StaticUniverse (revised)

Each universe implementation holds a `DataSource` reference and implements `Window`
and `At` directly (no shared base struct, since `Window` needs the concrete type's
`Assets()` to resolve membership).

```go
type StaticUniverse struct {
    members []asset.Asset
    ds      DataSource
}

func (u *StaticUniverse) Assets(_ time.Time) []asset.Asset { return u.members }
func (u *StaticUniverse) Prefetch(_ context.Context, _, _ time.Time) error { return nil }

func (u *StaticUniverse) Window(lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error) {
    return u.ds.Fetch(context.TODO(), u.members, lookback, metrics)
}

func (u *StaticUniverse) At(t time.Time, metrics ...data.Metric) (*data.DataFrame, error) {
    return u.ds.FetchAt(context.TODO(), u.members, t, metrics)
}
```

### Engine.Universe factory

```go
func (e *Engine) Universe(assets ...asset.Asset) universe.Universe {
    return universe.NewStaticWithSource(assets, e)  // e implements DataSource
}
```

## Parameter Discovery

```go
type Parameter struct {
    Name        string       // from pvbt tag or field name
    FieldName   string       // Go struct field name
    Description string       // from desc tag
    GoType      reflect.Type // field's Go type
    Default     string       // from default tag
}

func StrategyParameters(s Strategy) []Parameter
```

Reflects over the strategy struct's exported fields. Reads `pvbt`, `desc`, `default`
tags.

## Strategy Field Hydration

At startup, the engine reflects over the strategy struct and populates fields from
`default` tags:

| Go type | Default format | Hydration |
|---------|---------------|-----------|
| `float64` | `"0.5"` | `strconv.ParseFloat` |
| `int` | `"10"` | `strconv.Atoi` |
| `string` | `"value"` | direct |
| `bool` | `"true"` | `strconv.ParseBool` |
| `time.Duration` | `"5m"` | `time.ParseDuration` |
| `asset.Asset` | `"VFINX"` | `e.Asset(ticker)` |
| `universe.Universe` | `"VFINX,PRIDX"` | split on comma, resolve each via `e.Asset`, build via `e.Universe` |

Fields that already have non-zero values are not overwritten (the caller may have
set them before passing to `engine.New`).

## SimulatedBroker

```go
type SimulatedBroker struct {
    priceFn func(asset.Asset) (float64, bool) // returns close price, ok
    date    time.Time
}
```

Implements `broker.Broker`:

| Method | Behavior |
|--------|----------|
| `Connect` | no-op, returns nil |
| `Close` | no-op, returns nil |
| `Submit` | Look up close via `priceFn`. Not found: return error. Found: return `[]Fill{{Price: close, Qty: order.Qty, FilledAt: date}}` |
| `Cancel` | return `ErrNotSupported` |
| `Replace` | return `ErrNotSupported` |
| `Orders` | return nil, nil |
| `Positions` | return nil, nil |
| `Balance` | return `Balance{}`, nil |

The engine updates `priceFn` and `date` before each Compute step. The closure
captures the engine's cache so any asset fetched during universe data calls is
available for order fills.

## Data Cache

Sliding-window cache, memory-based. Not LRU -- eviction is deterministic based on
the window position.

```go
type dataCache struct {
    entries   map[dataCacheKey]*data.DataFrame
    curBytes  int64
    maxBytes  int64
    chunkSize time.Duration
}

type dataCacheKey struct {
    assetsHash  uint64    // hash of sorted CompositeFigi values
    metricsHash uint64    // hash of sorted metric strings
    chunkStart  time.Time // chunk boundary start
}
```

### Fetch strategy

When a universe calls the engine's `Fetch(ctx, assets, lookback, metrics)`:

1. Compute the requested range: `[currentDate - lookback, currentDate]`
2. The engine knows the full backtest range `[start, end]`. The total data needed
   for this `(assets, metrics)` combination across all remaining steps is
   `[currentDate - lookback, end]`.
3. Determine which chunks cover this range. Chunks are calendar-aligned by
   `chunkSize` (default 1 year for daily, 1 day for tick -- inferred from the
   data frequency returned by the provider, or configurable).
4. Check cache for each chunk in the requested window `[currentDate - lookback,
   currentDate]`.
5. On cache miss, fetch from the appropriate provider. The engine issues a
   `DataRequest` for the chunk's time range with the resolved assets and metrics.
6. Optionally pre-fetch the next chunk(s) in a background goroutine if memory allows.
7. Evict chunks that are entirely before `currentDate - maxLookback` (where
   `maxLookback` is the largest lookback seen so far for this assets/metrics key).
8. Merge cached chunks covering `[currentDate - lookback, currentDate]` into a
   single DataFrame and return it.

### Size estimation

```
bytes = len(times) * len(assets) * len(metrics) * 8
```

### Provider routing

At startup, the engine builds `metricProvider map[data.Metric]data.BatchProvider`
by calling `Provides()` on each provider. First provider wins for a given metric.
When a fetch request includes metrics from multiple providers, the engine splits the
request, fetches from each, and merges the resulting DataFrames.

## Backtest Flow

```
eng.Backtest(ctx, acct, start, end) (*portfolio.Account, error):

PHASE 1: INITIALIZATION
  1. Load asset registry from assetProvider.Assets(ctx)
     Build map[string]asset.Asset keyed by ticker.
     Error if assetProvider is nil or fails.

  2. Hydrate strategy fields from default tags via reflection.
     asset.Asset fields: resolve via e.Asset(ticker).
     universe.Universe fields: split default, resolve each ticker, build via e.Universe.
     Scalar fields: parse from string.
     Skip fields with non-zero values.

  3. Build provider routing table.
     For each provider, call Provides() to get supported metrics.
     Build metric -> BatchProvider map. First provider wins.
     Error if any provider is not a BatchProvider.

  4. Call strategy.Setup(e).
     Strategy calls e.Schedule(), e.RiskFreeAsset(), e.SetBenchmark(), etc.
     Strategy may call e.Asset() or e.Universe() for additional assets/universes.

  5. Validate.
     Error if e.schedule is nil.

  6. Configure account.
     Create SimulatedBroker, attach via acct.SetBroker().
     Set benchmark on account if e.benchmark is set.
     Set risk-free on account if e.riskFree is set.

  7. Initialize data cache.

  8. Store start/end on engine for use by DataSource methods.

PHASE 2: DATE ENUMERATION
  9. Walk tradecron.Next() from start until past end.
     Store as []time.Time.

PHASE 3: STEP LOOP
  For each scheduled date (step i of N):

  10. Check ctx.Err(). Return acct with partial history if cancelled.

  11. Set e.currentDate = date.

  12. Build step context.
      Attach zerolog logger with fields: strategy name, date, step number.
      Derive from parent ctx.

  13. Fetch housekeeping data for held assets.
      Collect: all held assets (acct.Holdings) + benchmark + risk-free.
      Metrics: MetricClose, AdjClose, Dividend.
      Fetch via cache (same chunked path as universe fetches).
      This ensures dividend data and broker prices are available.

  14. Record dividends.
      For each held asset with non-zero Dividend at currentDate:
        acct.Record(Transaction{
            Date: currentDate, Asset: a, Type: DividendTransaction,
            Amount: dividendPerShare * qty, Qty: qty, Price: dividendPerShare,
        })

  15. Update simulated broker.
      Set priceFn to closure that looks up MetricClose in cache.
      Set date to currentDate.

  16. Call strategy.Compute(stepCtx, e, acct).
      Strategy calls universe.Window() / universe.At() for data.
      Universe delegates to engine (DataSource interface).
      Engine fetches/caches, returns windowed DataFrame.
      Strategy calls p.Order() / p.RebalanceTo().
      Portfolio uses simulated broker for fills.

  17. Build price DataFrame for all assets seen this step.
      Held assets (including newly acquired) + benchmark + risk-free.
      Fetch any newly acquired assets not yet in cache.
      Single-row DataFrame at currentDate.

  18. Call acct.UpdatePrices(priceDF).

PHASE 4: RETURN
  19. Return acct, nil.
```

### Ordering constraints

- Step 13 before 14: need dividend data before recording.
- Step 13 before 15: broker needs prices.
- Step 15 before 16: orders during Compute need broker prices.
- Step 16 before 17: need to know all assets from trades.
- Step 17 before 18: UpdatePrices needs complete price data.

### Edge cases

- First step: no held assets, step 13 only fetches benchmark + risk-free.
- Strategy orders an asset not in cache: SimulatedBroker returns error, portfolio
  logs and skips (existing behavior in account.go).
- Context cancelled mid-loop: return acct with partial history and ctx.Err().
- Provider fetch fails: return error (abort backtest).
- Empty schedule (no trading days in [start, end]): return acct unchanged.

## RunLive Flow

```
eng.RunLive(ctx, acct) (<-chan *portfolio.Account, error):

PHASE 1: INITIALIZATION
  1-4. Same as Backtest steps 1-4.

  5. Validate.
     Error if e.schedule is nil.
     Error if acct has no broker set.

  6. Build provider routing table.
     Prefer StreamProvider where available.
     Fall back to BatchProvider.

PHASE 2: GOROUTINE
  7. Create channel: ch := make(chan *portfolio.Account, 1)

  8. Start goroutine:
     Loop:
       a. Compute next scheduled time via tradecron.Next(time.Now()).
       b. select {
            case <-time.After(until next):
            case <-ctx.Done(): close(ch); return
          }
       c. Set e.currentDate = time.Now().
       d. Build step context with zerolog logger.
       e. Fetch data for held assets + benchmark + risk-free.
       f. Record dividends.
       g. Call strategy.Compute(stepCtx, e, acct).
       h. Build price DataFrame, call acct.UpdatePrices().
       i. Non-blocking send on channel:
          select {
            case ch <- acct: // sent
            default: // dropped, consumer is slow
          }
       j. Repeat.

  9. Return ch, nil immediately.
```

## Error Handling

| Condition | Behavior |
|-----------|----------|
| No schedule set after Setup | Return error from Backtest/RunLive |
| No AssetProvider configured | Return error from Backtest/RunLive |
| No DataProvider configured | Return error from Backtest/RunLive |
| No BatchProvider for Backtest | Return error |
| No broker on account for RunLive | Return error |
| Asset ticker not found | Panic (strategy misconfiguration) |
| Provider Fetch fails | Return error (abort backtest) |
| Universe Prefetch fails | Return error |
| Context cancelled | Return acct with partial history + ctx.Err() |
| Order for unknown asset | SimBroker returns error, portfolio logs and skips |

## DataFrame Merging

The `data.DataFrame` type needs a `Merge` function to combine DataFrames from
multiple providers or cache chunks. Two variants:

```go
// MergeColumns combines DataFrames with the same timestamps but different
// assets or metrics. Used for multi-provider routing (provider A returns
// MetricClose, provider B returns Volume -- merge into one DataFrame).
func MergeColumns(frames ...*DataFrame) (*DataFrame, error)

// MergeTimes combines DataFrames with the same assets and metrics but
// different time ranges. Used for cache chunk merging (chunk 2024 + chunk
// 2025 = one contiguous DataFrame). Timestamps must not overlap.
func MergeTimes(frames ...*DataFrame) (*DataFrame, error)
```

These are added to the `data` package as part of this implementation.

## File Layout

Engine package (one type per file):

```
engine/
    engine.go             Engine struct, New, Close, Asset, Schedule, SetBenchmark,
                          RiskFreeAsset, Universe; implements universe.DataSource
    strategy.go           Strategy interface (updated)
    option.go             Option type and With* functions (updated)
    parameter.go          Parameter type and StrategyParameters function
    hydrate.go            hydrateFields -- reflection logic for default tags
    backtest.go           Backtest method
    live.go               RunLive method
    data_cache.go         dataCache, dataCacheKey, sliding window logic
    simulated_broker.go   SimulatedBroker
```

Data package (new + modified files):

```
data/
    asset_provider.go     AssetProvider interface
    merge.go              MergeColumns and MergeTimes functions
    pvdata_provider.go    Add Assets(ctx) method to implement AssetProvider
```

Universe package (modified + new):

```
universe/
    universe.go           Universe interface (updated with Window, At)
    data_source.go        DataSource interface
    static.go             StaticUniverse (updated with DataSource, Window, At)
```

CLI package (modified):

```
cli/
    backtest.go           Update to use eng.Backtest, delegate field handling to engine
    flags.go              Update to use engine.StrategyParameters
```

## CLI Updates

The CLI currently does its own reflection in `flags.go` to generate cobra flags and
populate strategy fields. This changes:

1. `registerStrategyFlags` calls `engine.StrategyParameters(strategy)` to discover
   parameters and generates cobra flags from the returned `[]Parameter`.
2. After flag parsing, `applyStrategyFlags` sets values on the strategy struct using
   the same reflection but driven by the `Parameter` metadata.
3. `runBacktest` calls `eng.Backtest` instead of `eng.Run`.
4. The engine hydrates defaults first; CLI flags override after.

## Doc Updates

- `docs/overview.md`: Update ADM example to new Strategy interface. Remove TOML.
  Show struct tags. Update Compute signature. Show `Backtest` not `Run`.
- `docs/configuration.md`: Rewrite to describe struct tags instead of TOML.
- `docs/data.md`: Add AssetProvider section. Fix metric table to match code.
- `docs/portfolio.md`: `Backtest` not `Run` in examples.
- `docs/scheduling.md`: No changes needed.
