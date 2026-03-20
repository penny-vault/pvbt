# Strategy Author's Guide

This guide walks through everything you need to write, run, and test a pvbt strategy.

## Your first strategy

A strategy is a Go struct that implements three methods:

```go
type Strategy interface {
    Name() string
    Setup(eng *engine.Engine)
    Compute(ctx context.Context, eng *engine.Engine, portfolio portfolio.Portfolio, batch *portfolio.Batch) error
}
```

- **Name** returns the strategy name. Used in logging and CLI output.
- **Setup** runs once at startup. Configure the trading schedule, benchmark, and risk-free asset here.
- **Compute** runs at each scheduled date. Fetch data, compute signals, and write orders and annotations to the `batch`. The portfolio is read-only -- use it to inspect holdings and performance, but place all trades through the batch.

Here is a complete strategy that rotates into whichever asset has the highest trailing momentum:

```go
package main

import (
    "context"

    "github.com/penny-vault/pvbt/cli"
    "github.com/penny-vault/pvbt/data"
    "github.com/penny-vault/pvbt/engine"
    "github.com/penny-vault/pvbt/portfolio"
    "github.com/penny-vault/pvbt/tradecron"
    "github.com/penny-vault/pvbt/universe"
    "github.com/rs/zerolog"
)

type MomentumRotation struct {
    RiskOn   universe.Universe `pvbt:"risk-on"  desc:"Assets to rotate between" default:"SPY,EFA,EEM"`
    RiskOff  universe.Universe `pvbt:"risk-off" desc:"Safe-haven asset"         default:"SHY"`
    Lookback int               `pvbt:"lookback" desc:"Momentum lookback months"  default:"6"`
}

func (s *MomentumRotation) Name() string { return "momentum-rotation" }

func (s *MomentumRotation) Setup(_ *engine.Engine) {}

func (s *MomentumRotation) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        Schedule:  "@monthend",
        Benchmark: "SPY",
        Warmup:    126, // need 6 months of data before first compute
    }
}

func (s *MomentumRotation) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    log := zerolog.Ctx(ctx)

    // Fetch close prices for the lookback period.
    df, err := s.RiskOn.Window(ctx, portfolio.Months(s.Lookback), data.MetricClose)
    if err != nil {
        log.Error().Err(err).Msg("Window fetch failed")
        return nil
    }

    if df.Len() < 2 {
        return nil
    }

    // Total return over the full window, last row only.
    momentum := df.Pct(df.Len() - 1).Last()

    // Fallback DataFrame for when no risk-on asset has positive momentum.
    riskOffDF, err := s.RiskOff.At(ctx, eng.CurrentDate(), data.MetricClose)
    if err != nil {
        log.Error().Err(err).Msg("risk-off data fetch failed")
        return nil
    }

    // Select the asset with the highest positive return; fall back to risk-off.
    portfolio.MaxAboveZero(data.MetricClose, riskOffDF).Select(momentum)

    // Convert selection to equal-weight allocation.
    plan, err := portfolio.EqualWeight(momentum)
    if err != nil {
        log.Error().Err(err).Msg("EqualWeight failed")
        return nil
    }

    // Execute the rebalance.
    if err := batch.RebalanceTo(ctx, plan...); err != nil {
        log.Error().Err(err).Msg("rebalance failed")
    }

    return nil
}

func main() {
    cli.Run(&MomentumRotation{})
}
```

The `main` function is one line: `cli.Run(&MomentumRotation{})`. This gives your strategy a full CLI with `backtest`, `live`, and `snapshot` subcommands, plus automatic flag registration from struct tags.

## Strategy parameters

Exported struct fields become CLI flags automatically. Use struct tags to customize:

| Tag | Purpose | Example |
|-----|---------|---------|
| `pvbt` | Flag name | `pvbt:"lookback"` |
| `desc` | Help text | `desc:"Momentum lookback months"` |
| `default` | Default value | `default:"6"` |

If `pvbt` is omitted, the flag name is derived from the field name in kebab-case (`RiskOn` becomes `--risk-on`).

Supported field types:

| Go type | CLI example |
|---------|-------------|
| `int` | `--lookback 6` |
| `float64` | `--threshold 0.02` |
| `string` | `--ticker SPY` |
| `bool` | `--rebalance-quarterly` |
| `time.Duration` | `--hold-period 720h` |
| `universe.Universe` | `--risk-on SPY,EFA,EEM` |

Universe fields are parsed as comma-separated ticker lists and resolved to static universes.

## Running your strategy

Build and run like any Go program:

```bash
go build -o momentum-rotation .
```

### Backtest

```bash
./momentum-rotation backtest --start 2020-01-01 --end 2024-12-31 --cash 100000
```

All dates are Eastern time. Output is a SQLite database with the equity curve, transactions, and performance metrics:

| Flag | Default | Description |
|------|---------|-------------|
| `--start` | 5 years ago | Backtest start date (YYYY-MM-DD) |
| `--end` | today | Backtest end date (YYYY-MM-DD) |
| `--cash` | 100000 | Starting cash balance |
| `--output` | auto-generated | Output file path |
| `--log-level` | info | Logging verbosity (debug, info, warn, error) |

Strategy-specific flags are appended after the built-in ones:

```bash
./momentum-rotation backtest --start 2020-01-01 --lookback 12 --risk-on SPY,QQQ,EFA
```

### Snapshot (capture data for offline testing)

```bash
./momentum-rotation snapshot --start 2023-01-01 --end 2024-01-01
```

This runs the full backtest but captures every data request and response into a SQLite file. The output defaults to `pv-data-snapshot-{strategy}-{start}-{end}.db`. Override with `--output`:

```bash
./momentum-rotation snapshot --start 2023-01-01 --end 2024-01-01 --output testdata/snapshot.db
```

See [Testing with snapshots](#testing-with-snapshots) for how to use the snapshot in tests.

## Setup

`Setup` runs once before the first `Compute` call. Use it to configure:

### Trading schedule

The schedule determines when `Compute` is called. Declare it in `Describe()` using a tradecron expression:

```go
func (s *MyStrategy) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        Schedule: "@monthend",
    }
}
```

Common schedule expressions:

| Expression | When Compute runs |
|------------|-------------------|
| `@monthend` | Last trading day of each month at market close |
| `@monthbegin` | First trading day of each month |
| `@weekbegin` | First trading day of each week |
| `@weekend` | Last trading day of each week |
| `@open * * *` | Every trading day at market open |
| `@close * * *` | Every trading day at market close |
| `0 10 * * *` | Every trading day at 10:00 AM ET |

The format is standard 5-field cron (`minute hour day-of-month month day-of-week`) with market-aware extensions. `@open` and `@close` replace the minute/hour fields. `@monthend`, `@monthbegin`, `@weekbegin`, and `@weekend` replace the day-of-month field. All times are Eastern.

### Warmup

If your strategy needs historical data before its first compute date (e.g., to calculate a moving average), declare the number of trading days in `Describe()`:

```go
func (s *MyStrategy) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        Schedule: "@monthend",
        Warmup:   252, // need 1 year of data before first trade
    }
}
```

The engine validates that all assets in your universes and asset fields have enough data covering the warmup window. In strict mode (the default) the backtest fails immediately if any asset is short. In permissive mode (`engine.WithDateRangeMode(engine.DateRangeModePermissive)`) the engine shifts the start date forward until all assets have sufficient history.

### Benchmark and risk-free assets

```go
eng.SetBenchmark(eng.Asset("SPY"))
eng.RiskFreeAsset(eng.Asset("SHV"))
```

The benchmark is used for Beta, Alpha, Tracking Error, and Information Ratio. The risk-free asset is used for Sharpe, Treynor, and `RiskAdjustedPct`. When a risk-free asset is configured, the engine pre-computes a cumulative risk-free series and attaches it to all DataFrames returned by `Fetch` and `FetchAt`, enabling `df.RiskAdjustedPct(n)` to subtract the risk-free return automatically.

### Asset lookup

`eng.Asset(ticker)` resolves a ticker to an `asset.Asset` using the registered `AssetProvider`. It panics if the ticker is not found, which is appropriate in `Setup` since a missing benchmark or risk-free asset is a fatal configuration error.

## Universes

A universe is a group of assets that can change over time. Universes are the primary way to fetch data in `Compute`.

### Static universe

Fixed set of assets. Most strategies use these, either defined as struct fields or built in Setup:

```go
// As a struct field with CLI flag:
type MyStrategy struct {
    Assets universe.Universe `pvbt:"assets" desc:"Assets to trade" default:"SPY,EFA,EEM"`
}

// Or built in Setup:
func (s *MyStrategy) Setup(eng *engine.Engine) {
    s.myUniverse = eng.Universe(eng.Asset("SPY"), eng.Asset("EFA"), eng.Asset("EEM"))
}
```

### Index universe

Tracks historical index membership. Members change over time (additions and removals), which avoids survivorship bias:

```go
func (s *MyStrategy) Setup(eng *engine.Engine) {
    s.sp500 = eng.IndexUniverse("SP500")
}
```

At each date, `s.sp500.Assets(t)` returns the index members as of that date.

### Rated universe

Selects assets by analyst rating:

```go
func (s *MyStrategy) Setup(eng *engine.Engine) {
    s.buys = eng.RatedUniverse("morningstar", data.RatingEq(5))
}
```

### Fetching data from a universe

Both methods are available in `Compute`:

```go
// Window: lookback period ending at the current simulation date.
df, err := s.Assets.Window(ctx, data.Months(6), data.MetricClose, data.AdjClose)

// At: single point in time.
df, err := s.Assets.At(ctx, eng.CurrentDate(), data.MetricClose)
```

The returned DataFrame contains one column per (asset, metric) pair and one row per trading day.

## DataFrames

DataFrames are the core data type. They store time-series values indexed by (time, asset, metric) in a column-major layout optimized for numerical operations.

### Reading values

```go
// Most recent value for an asset/metric pair.
price := df.Value(spy, data.MetricClose)

// Value at a specific timestamp.
price := df.ValueAt(spy, data.MetricClose, someDate)

// Contiguous slice for a single column (compatible with gonum).
col := df.Column(spy, data.MetricClose)
```

### Metadata

```go
df.Len()            // number of timestamps
df.Start()          // first timestamp
df.End()            // last timestamp
df.Times()          // copy of all timestamps
df.AssetList()      // copy of all assets
df.MetricList()     // copy of all metrics
```

### Slicing

```go
df.Assets(spy, tlt)                    // keep only SPY and TLT
df.Metrics(data.MetricClose)           // keep only Close
df.Between(start, end)                 // keep timestamps in range
df.Last()                              // single-row DataFrame at most recent date
df.At(someDate)                        // single-row DataFrame at specific date
df.Filter(func(t time.Time, row *data.DataFrame) bool { ... })
```

Each operation returns a new DataFrame. They can be chained:

```go
df.Assets(spy).Metrics(data.MetricClose).Last()
```

If any operation encounters an error, it propagates through the chain. Check with `df.Err()`.

### Arithmetic

Element-wise operations align by asset and metric:

```go
df.Add(other)           // element-wise addition
df.Sub(other)           // subtraction
df.Mul(other)           // multiplication
df.Div(other)           // division

df.AddScalar(1.0)       // add constant to every value
df.MulScalar(0.5)       // scale every value
```

When the second DataFrame has a single metric, it broadcasts across all metrics in the first:

```go
// Divide all columns by Close:
normalized := df.Div(priceDF, data.MetricClose)
```

### Financial calculations

```go
df.Pct()                // single-period percent change
df.Pct(n)               // n-period percent change: (current - n ago) / n ago
df.RiskAdjustedPct()    // percent change minus risk-free return
df.RiskAdjustedPct(n)   // n-period risk-adjusted percent change
df.Diff()               // first difference: current - previous
df.Log()                // natural logarithm
df.CumSum()             // cumulative sum
df.CumMax()             // running maximum
df.Shift(n)             // time-shift (positive=forward, negative=backward)
df.Covariance()         // cross-asset covariance matrix
```

### Aggregations

Aggregate across assets at each timestamp:

```go
df.Mean()               // average across assets -> synthetic "MEAN" asset
df.Sum()                // sum across assets -> "SUM"
df.MaxAcrossAssets()    // max across assets -> "MAX"
df.MinAcrossAssets()    // min across assets -> "MIN"
df.Variance()           // variance across timestamps per column
df.Std()                // standard deviation
```

### Rolling windows

```go
rolling := df.Rolling(20)   // 20-period rolling window
rolling.Mean()               // rolling average
rolling.Std()                // rolling standard deviation
rolling.Max()                // rolling maximum
rolling.Min()                // rolling minimum
```

### Resampling

```go
df.Downsample(data.Monthly)  // OHLCV aggregation to monthly
df.Upsample(data.Daily)      // forward-fill to daily
```

### Debugging

```go
fmt.Println(df.Table())     // ASCII table of the DataFrame
```

## Trading

### Declarative: RebalanceTo

The typical approach. Provide target weights and the batch generates the necessary orders:

```go
plan, err := portfolio.EqualWeight(selectedDF)
if err != nil {
    return err
}
batch.RebalanceTo(ctx, plan...)
```

The pipeline is: **select** which assets to hold, then **weight** them into an allocation, then **execute** the rebalance.

**Selection** -- A `Selector` marks chosen assets by inserting a `Selected` column into the DataFrame:

```go
// Pick the single best asset; fall back to fallbackDF if none positive.
portfolio.MaxAboveZero(data.MetricClose, fallbackDF).Select(df)

// Pick the top 3 assets by momentum score.
portfolio.TopN(3, data.MetricClose).Select(df)

// Pick the 2 cheapest assets by P/E ratio.
portfolio.BottomN(2, data.PE).Select(df)
```

`CountWhere` counts how many assets match a condition at each timestep, useful for canary-style signals:

```go
badCanary := df.CountWhere(data.AdjClose, func(v float64) bool {
    return math.IsNaN(v) || v <= 0
})
```

**Weighting** -- Converts a DataFrame with selected assets into a `PortfolioPlan`:

```go
plan, err := portfolio.EqualWeight(df)   // equal weight among selected assets
```

**Execution** -- `RebalanceTo` diffs current holdings against the target and accumulates buy/sell orders in the batch:

```go
batch.RebalanceTo(ctx, plan...)
```

An `Allocation` has target weights that sum to 1.0 and an optional justification:

```go
type Allocation struct {
    Date          time.Time
    Members       map[asset.Asset]float64   // asset -> weight
    Justification string
}
```

### Imperative: Order

For fine-grained control over individual trades:

```go
// Market buy 100 shares of SPY.
batch.Order(ctx, spy, portfolio.Buy, 100)

// Limit sell 50 shares of TLT at $95.
batch.Order(ctx, tlt, portfolio.Sell, 50, portfolio.Limit(95.0))

// Stop loss at $380.
batch.Order(ctx, spy, portfolio.Sell, 100, portfolio.Stop(380.0))
```

Available order modifiers:

| Modifier | Description |
|----------|-------------|
| `Limit(price)` | Maximum buy price or minimum sell price |
| `Stop(price)` | Trigger market order at threshold |
| `DayOrder` | Cancel at close if not filled (default) |
| `GoodTilCancel` | Keep open until filled or cancelled |
| `GoodTilDate(t)` | Keep open until specific date |
| `FillOrKill` | Fill entirely or cancel immediately |
| `ImmediateOrCancel` | Fill what you can, cancel the rest |
| `OnTheOpen` | Fill at market open price |
| `OnTheClose` | Fill at market close price |
| `WithJustification(s)` | Attach explanation to resulting transactions |

### Reading portfolio state

```go
port.Cash()                 // available cash
port.Value()                // total value (cash + holdings)
port.Position(spy)          // shares held
port.PositionValue(spy)     // market value of position

port.Holdings(func(a asset.Asset, qty float64) {
    // iterate all positions
})

port.Transactions()         // full trade log
```

### Annotations

Record why your strategy made its decisions. Annotations are stored in the output and useful for debugging:

```go
batch.Annotate("signal", fmt.Sprintf("%.4f", momentumScore))
batch.Annotate("action", "rotating to SPY")
```

## Performance metrics

After a backtest, the portfolio provides computed metrics:

```go
summary, err := port.Summary()
// summary.TWRR, summary.Sharpe, summary.Sortino, summary.Calmar,
// summary.MaxDrawdown, summary.StdDev

risk, err := port.RiskMetrics()
// risk.Beta, risk.Alpha, risk.TrackingError, risk.DownsideDeviation,
// risk.InformationRatio, risk.Treynor

trade, err := port.TradeMetrics()
// trade.WinRate, trade.ProfitFactor, trade.AverageHoldingPeriod,
// trade.Turnover, trade.ConsecutiveWins, trade.ConsecutiveLosses

tax, err := port.TaxMetrics()
// tax.LTCG, tax.STCG, tax.UnrealizedGains, tax.Dividends, tax.TaxCostRatio

withdrawal, err := port.WithdrawalMetrics()
// withdrawal.SafeWithdrawalRate, withdrawal.PerpetualWithdrawalRate,
// withdrawal.DynamicWithdrawalRate
```

Windowed queries:

```go
// Sharpe ratio over the last 3 years.
sharpe, err := port.PerformanceMetric(portfolio.Sharpe).Window(portfolio.Years(3)).Value()

// Rolling 12-month return series.
series, err := port.PerformanceMetric(portfolio.TWRR).Window(portfolio.Months(12)).Series()
```

## Strategy metadata (optional)

Implement the `Descriptor` interface to provide metadata that gets serialized into backtest output:

```go
func (s *MomentumRotation) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        ShortCode:   "momrot",
        Description: "Rotates into the asset with the highest trailing return.",
        Version:     "0.1.0",
        Warmup:      126,
    }
}
```

This is optional. Strategies that don't implement it still work.

## Periods

Periods represent calendar-aware durations for lookback windows and performance metric queries:

```go
data.Days(30)       // 30 calendar days
data.Months(6)      // 6 calendar months
data.Years(1)       // 1 calendar year
data.YTD()          // year-to-date (from January 1)
data.MTD()          // month-to-date (from the 1st)
data.WTD()          // week-to-date (from Monday)
```

These are also available as `portfolio.Days`, `portfolio.Months`, etc. for convenience.

## Available metrics

Metrics identify what data providers supply. Pass them to `Window` and `At` calls.

**Price data** (from the eod table):

`MetricOpen`, `MetricHigh`, `MetricLow`, `MetricClose`, `AdjClose`, `Volume`, `Dividend`, `SplitFactor`

**Valuation metrics** (from the metrics table):

`MarketCap`, `EnterpriseValue`, `PE`, `ForwardPE`, `PEG`, `PB`, `PS`, `PriceToCashFlow`, `EVtoEBIT`, `EVtoEBITDA`, `Beta`

**Income statement**:

`Revenue`, `CostOfRevenue`, `GrossProfit`, `OperatingExpenses`, `OperatingIncome`, `EBIT`, `EBITDA`, `EBT`, `ConsolidatedIncome`, `NetIncome`, `NetIncomeCommonStock`, `EarningsPerShare`, `EPSDiluted`, `InterestExpense`, `IncomeTaxExpense`, `RandDExpenses`, `SGAExpense`, `ShareBasedCompensation`, `DividendsPerShare`

**Balance sheet**:

`TotalAssets`, `CurrentAssets`, `AssetsNonCurrent`, `AverageAssets`, `CashAndEquivalents`, `Inventory`, `Receivables`, `Investments`, `InvestmentsCurrent`, `InvestmentsNonCur`, `Intangibles`, `PPENet`, `TaxAssets`, `TotalLiabilities`, `CurrentLiabilities`, `LiabilitiesNonCurrent`, `TotalDebt`, `DebtCurrent`, `DebtNonCurrent`, `Payables`, `DeferredRevenue`, `Deposits`, `TaxLiabilities`, `Equity`, `EquityAvg`, `AccumulatedOtherComprehensiveIncome`, `AccumulatedRetainedEarningsDeficit`

**Cash flow**:

`FreeCashFlow`, `NetCashFlow`, `NetCashFlowFromOperations`, `NetCashFlowFromInvesting`, `NetCashFlowFromFinancing`, `NetCashFlowBusiness`, `NetCashFlowCommon`, `NetCashFlowDebt`, `NetCashFlowDividend`, `NetCashFlowInvest`, `NetCashFlowFx`, `CapitalExpenditure`, `DepreciationAmortization`

**Per-share and ratios**:

`BookValue`, `FreeCashFlowPerShare`, `SalesPerShare`, `TangibleAssetsBookValuePerShare`, `ShareFactor`, `SharesBasic`, `WeightedAverageShares`, `WeightedAverageSharesDiluted`, `GrossMargin`, `EBITDAMargin`, `ProfitMargin`, `ROA`, `ROE`, `ROIC`, `ReturnOnSales`, `AssetTurnover`, `CurrentRatio`, `DebtToEquity`, `DividendYield`, `PayoutRatio`, `InvestedCapital`, `InvestedCapitalAvg`, `TangibleAssetValue`, `WorkingCapital`

All metric constants are in the `data` package.

## Testing with snapshots

Unit tests should not depend on live data sources. The snapshot system lets you capture real data once and replay it deterministically in tests.

### 1. Capture the snapshot

Run your strategy with the `snapshot` command:

```bash
./momentum-rotation snapshot \
    --start 2023-01-01 \
    --end 2024-01-01 \
    --output testdata/snapshot.db
```

This runs a full backtest and records every data access (prices, assets, index members, ratings) into a SQLite file. Commit the file to your repository as a test fixture.

### 2. Replay in tests

Use `data.NewSnapshotProvider` to load the snapshot. It implements `BatchProvider`, `AssetProvider`, `IndexProvider`, and `RatingProvider`, so the engine gets everything it needs from a single object:

```go
package mystrategy_test

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/penny-vault/pvbt/data"
    "github.com/penny-vault/pvbt/engine"
    "github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("MomentumRotation", func() {
    It("produces positive returns over the test period", func() {
        ctx := context.Background()

        snap, err := data.NewSnapshotProvider("testdata/snapshot.db")
        Expect(err).NotTo(HaveOccurred())
        defer snap.Close()

        strategy := &MomentumRotation{}

        acct := portfolio.New(
            portfolio.WithCash(100000, startDate),
            portfolio.WithAllMetrics(),
        )

        eng := engine.New(strategy,
            engine.WithDataProvider(snap),
            engine.WithAssetProvider(snap),
            engine.WithAccount(acct),
        )

        result, err := eng.Backtest(ctx, startDate, endDate)
        Expect(err).NotTo(HaveOccurred())

        summary, err := result.Summary()
        Expect(err).NotTo(HaveOccurred())
        Expect(summary.TWRR).To(BeNumerically(">", 0))
    })
})
```

The snapshot captures exactly the data your strategy accessed during the recording run. Tests replay that data without any network or database dependency.

### 3. Regenerating snapshots

If you change your strategy's data requirements (new metrics, different assets, different date range), regenerate the snapshot:

```bash
./momentum-rotation snapshot --start 2023-01-01 --end 2024-01-01 --output testdata/snapshot.db
```

Then commit the updated file.

## Engine configuration

When building the engine outside the CLI (for tests or custom runners), use option functions:

```go
eng := engine.New(strategy,
    engine.WithDataProvider(provider),      // register data providers (stackable)
    engine.WithAssetProvider(provider),      // asset metadata
    engine.WithAccount(acct),               // pre-configured account
    engine.WithCacheMaxBytes(1 << 30),      // 1 GB data cache
    engine.WithBroker(myBroker),            // custom broker for live trading
    engine.WithDateRangeMode(engine.DateRangeModePermissive), // adjust start if warmup data is missing
)
```

For backtesting the CLI handles all of this. You only need engine options when writing tests or custom entry points.

## Error handling in Compute

Return `nil` from `Compute` to continue the backtest. Return an error to halt it:

```go
func (s *MyStrategy) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    df, err := s.Assets.Window(ctx, data.Months(6), data.MetricClose)
    if err != nil {
        // Log and skip this date. The backtest continues.
        zerolog.Ctx(ctx).Error().Err(err).Msg("data fetch failed")
        return nil
    }

    // ...

    // Return an error only if something is truly unrecoverable.
    // return fmt.Errorf("fatal: %w", err)

    return nil
}
```

DataFrames propagate errors through chained operations. Always check `df.Err()` or handle errors from the initial fetch before using the DataFrame.
