# Data

The data layer has three core concepts: metrics, data providers, and DataFrames.

A **metric** is any externally-sourced measurement about an asset or the economy -- price, volume, market capitalization, unemployment rate. Metrics are what data providers supply.

A **data provider** connects to an external source (database, API, file) and fetches metric values. Providers can deliver data in bulk for backtesting or stream it in real-time for live trading.

A **DataFrame** is the primary type for working with time-series data. It stores values indexed by time, asset, and metric, with a rich API for filtering, arithmetic, transforms, and windowed operations. DataFrames are directly compatible with gonum.

## Metrics

A metric identifies what a data provider can supply. It's a named type backed by a string:

```go
type Metric string
```

The `data` package defines well-known metrics:

| Metric | Description |
|--------|-------------|
| `MetricOpen` | Opening price |
| `MetricHigh` | High price |
| `MetricLow` | Low price |
| `MetricClose` | Closing price |
| `AdjClose` | Split/dividend-adjusted close |
| `Volume` | Trade volume |
| `Dividend` | Dividend per share |
| `SplitFactor` | Split adjustment factor |
| `Revenue` | Total revenue |
| `NetIncome` | Net income |
| `EarningsPerShare` | Diluted EPS |
| `TotalDebt` | Total debt |
| `TotalAssets` | Total assets |
| `FreeCashFlow` | Free cash flow |
| `BookValue` | Book value per share |
| `MarketCap` | Market capitalization |

For live trading, additional metrics are available:

| Metric | Description |
|--------|-------------|
| `Price` | Current market price (streaming) |
| `Bid` | Current bid price (streaming) |
| `Ask` | Current ask price (streaming) |

Custom metrics can be defined anywhere:

```go
const CPI data.Metric = "CPI"
const FedFundsRate data.Metric = "FedFundsRate"
```

### Economic indicators

Economic indicators like unemployment and CPI are not tied to a specific asset. They use the sentinel `asset.EconomicIndicator` in requests and DataFrames. From the DataFrame's perspective they look like any other asset -- the data layout stays uniform.

## Data providers

A data provider supplies metric values from an external source. All providers implement the base interface:

```go
type DataProvider interface {
    Provides() []Metric
    Close() error
}
```

`Provides` returns the metrics the provider can supply. `Close` releases resources (database connections, open files, etc.) when the engine is finished.

### Batch providers

Batch providers fetch historical data in bulk. Used during backtesting where the engine requests a complete time range upfront:

```go
type BatchProvider interface {
    DataProvider
    Fetch(ctx context.Context, req DataRequest) (*DataFrame, error)
}
```

### Stream providers

Stream providers deliver data in real-time. Used during live trading where the engine reacts to incoming market data:

```go
type StreamProvider interface {
    DataProvider
    Subscribe(ctx context.Context, req DataRequest) (<-chan DataPoint, error)
}
```

`Subscribe` opens a data stream. Each `DataPoint` is delivered on the returned channel. The provider closes the channel when the context is cancelled. The engine manages subscriptions by cancelling the context and re-subscribing when the requested assets or metrics change.

### Provider lifecycle

Providers are constructed by the caller and registered with the engine:

```go
db := postgres.New("postgres://localhost/market_data")
fred := fred.New(os.Getenv("FRED_API_KEY"))

e := engine.New(&ADM{},
    engine.WithDataProvider(db, fred),
)
```

The caller handles construction and authentication. The engine calls `Provides()` to discover what each provider can supply, routes data requests to the right provider, and calls `Close()` when finished.

A provider can implement both `BatchProvider` and `StreamProvider` if it supports both access patterns.

### Asset providers

An `AssetProvider` resolves ticker symbols to full `asset.Asset` values (including CompositeFigi identifiers). The engine requires an asset provider to build its internal asset registry:

```go
type AssetProvider interface {
    Assets(ctx context.Context) ([]asset.Asset, error)
    LookupAsset(ctx context.Context, ticker string) (asset.Asset, error)
}
```

`Assets` returns all known assets (used during initialization to build the registry). `LookupAsset` resolves a single ticker on demand (used by `e.Asset()`). A database provider typically implements `AssetProvider` alongside `BatchProvider`:

```go
eng := engine.New(&ADM{},
    engine.WithDataProvider(provider),
    engine.WithAssetProvider(provider),
)
```

### DataSource interface

The `data.DataSource` interface decouples data fetching from the engine, preventing circular dependencies between the engine and other packages:

```go
type DataSource interface {
    Fetch(ctx context.Context, assets []asset.Asset, lookback Period,
        metrics []Metric) (*DataFrame, error)
    FetchAt(ctx context.Context, assets []asset.Asset, t time.Time,
        metrics []Metric) (*DataFrame, error)
    CurrentDate() time.Time
}
```

The engine implements `DataSource`. Every DataFrame created by the engine carries a reference to it via `Source()`, so downstream consumers like weighting functions can fetch additional data on demand. Universes also hold a `DataSource` reference for the same purpose. A backward-compatible type alias `universe.DataSource` is provided.

### Index providers

Index providers supply historical index membership data (e.g. which companies were in the S&P 500 on a given date). The `universe` package uses this to build time-varying universes:

```go
type IndexProvider interface {
    IndexMembers(ctx context.Context, index string, t time.Time) ([]asset.Asset, error)
}
```

A database provider typically implements `IndexProvider` alongside `BatchProvider`, since both use the same database connection.

### Rating filters

Rating filters select assets by analyst rating for use with `eng.RatedUniverse`:

```go
data.RatingEq(5)           // exactly 5-star
data.RatingLTE(3)          // 1, 2, or 3 star
data.RatingIn(1, 2, 5)     // any of the listed values
```

### Data requests

A `DataRequest` describes what data to fetch:

```go
type DataRequest struct {
    Assets    []asset.Asset
    Metrics   []Metric
    Start     time.Time
    End       time.Time
    Frequency Frequency
}
```

The engine optimizes requests -- it coalesces what the strategy needs, batches requests across providers, and pre-loads data for all computation steps.

## DataFrame

DataFrame is the primary type for working with time-series data. It stores values indexed by time, asset, and metric in column-major order. Each (asset, metric) pair is a contiguous `[]float64` slice, making it directly compatible with gonum and SIMD-friendly.

### Accessors

```go
df.Start()                          // earliest timestamp
df.End()                            // latest timestamp
df.Duration()                       // time span
df.Len()                            // number of timestamps
df.ColCount()                       // number of columns (assets * metrics)
df.Frequency()                      // data resolution (Daily, Weekly, etc.)
df.Times()                          // copy of all timestamps as []time.Time
df.AssetList()                      // copy of all assets as []asset.Asset
df.MetricList()                     // copy of all metrics as []Metric
df.Value(aapl, data.Price)          // most recent value
df.ValueAt(aapl, data.Price, t)     // value at time t
df.Column(aapl, data.Price)         // contiguous []float64, gonum-compatible
df.At(t)                            // single-row DataFrame at time t
df.Last()                           // single-row DataFrame, most recent
df.Copy()                           // deep copy
df.Table()                          // ASCII table for debugging
df.Err()                            // error from any chained operation (nil if OK)
df.Source()                         // the DataSource that created this DataFrame
```

### Narrowing and filtering

```go
df.Assets(aapl, goog)               // only these assets
df.Metrics(data.Price)              // only this metric
df.Between(start, end)              // only this time range
df.Drop(math.NaN())                 // remove timestamps with NaN values

df.Filter(func(t time.Time, row *DataFrame) bool {
    return row.Value(aapl, data.Volume) > 1_000_000
})
```

`Filter` receives a single-row DataFrame for each timestamp, giving full access to all assets and metrics at that point.

### Arithmetic

DataFrame arithmetic aligns by asset and metric automatically:

```go
// DataFrame-to-DataFrame (element-wise, aligned)
result := df1.Add(df2)
result := df1.Sub(df2)
result := df1.Mul(df2)
result := df1.Div(df2)

// Scalar
result := df.AddScalar(1.0)
result := df.SubScalar(0.5)
result := df.MulScalar(0.5)
result := df.DivScalar(100.0)
```

### Per-column aggregation

These collapse the time dimension, producing one value per column:

```go
df.Mean()                           // mean of each column over time
df.Sum()                            // sum of each column over time
df.Max()                            // max of each column over time
df.Min()                            // min of each column over time
df.Variance()                       // sample variance of each column
df.Std()                            // sample standard deviation of each column
df.Covariance()                     // covariance matrix
df.Correlation()                    // Pearson correlation matrix
```

### Aggregation across assets

These collapse the asset dimension, producing one value per timestamp per metric:

```go
df.MaxAcrossAssets()                // max across all assets per timestamp
df.MinAcrossAssets()                // min across all assets per timestamp
df.IdxMaxAcrossAssets()             // which asset has the max (returns []asset.Asset)
```

### Common transforms

```go
df.Pct()                            // percent change, 1-period default
df.Pct(5)                           // 5-period percent change
df.RiskAdjustedPct()                // percent change minus risk-free return, 1-period
df.RiskAdjustedPct(5)               // 5-period risk-adjusted percent change
df.Diff()                           // first difference
df.Log()                            // natural logarithm
df.CumSum()                         // cumulative sum
df.CumMax()                         // running maximum
df.Shift(1)                         // shift forward by 1 period
df.Shift(-1)                        // shift backward by 1 period
```

### Resampling

Downsampling reduces frequency by aggregating values within each period. It returns a builder -- call an aggregation method to get the result:

```go
df.Downsample(data.Weekly).Last()    // weekly close
df.Downsample(data.Weekly).Sum()     // weekly total volume
df.Downsample(data.Monthly).Max()    // monthly high
df.Downsample(data.Monthly).First()  // monthly open
```

Upsampling increases frequency by filling gaps:

```go
df.Upsample(data.Daily).ForwardFill()  // fill gaps with previous value
df.Upsample(data.Daily).BackFill()     // fill gaps with next value
df.Upsample(data.Daily).Interpolate()  // linear interpolation
```

OHLC bars are not a primitive. They are a pattern of downsamples: Open is `First()`, High is `Max()`, Low is `Min()`, Close is `Last()`, Volume is `Sum()`.

### Rolling window operations

```go
df.Rolling(20).Mean()               // 20-period rolling mean (SMA)
df.Rolling(20).Sum()                // rolling sum
df.Rolling(20).Max()                // rolling max
df.Rolling(20).Min()                // rolling min
df.Rolling(20).Std()                // rolling standard deviation
df.Rolling(20).Variance()           // rolling variance
df.Rolling(20).Percentile(0.9)      // rolling 90th percentile
```

Combine with `Metrics` to apply to a specific metric:

```go
sma := df.Metrics(data.Price).Rolling(20).Mean()
```

### Extensibility

For operations not built into DataFrame, use `Apply` and `Reduce`:

```go
// Apply transforms each column, returning a new DataFrame
result := df.Apply(func(col []float64) []float64 {
    out := make([]float64, len(col))
    floats.CumProd(out, col)
    return out
})

// Reduce collapses each column to a single value (single-row DataFrame)
stds := df.Reduce(func(col []float64) float64 {
    return stat.StdDev(col, nil)
})
```

Since `Column` returns `[]float64`, you can also use gonum directly on individual columns:

```go
prices := df.Column(aapl, data.Price)
mean := stat.Mean(prices, nil)
std := stat.StdDev(prices, nil)
```

### Mutation

DataFrames are normally immutable (operations return new DataFrames), but a few methods mutate in place for construction and labeling:

```go
df.AppendRow(timestamp, values)     // append a row of values at timestamp
df.Insert(asset, metric, values)    // insert a new column
df.RenameMetric(old, new)           // rename a metric column (returns df for chaining)
```

### CountWhere

`CountWhere` counts how many assets match a predicate for a given metric at each timestep. It returns a single-asset DataFrame (Ticker `"COUNT"`) with a `Count` metric:

```go
badCanary := df.CountWhere(data.AdjClose, func(v float64) bool {
    return math.IsNaN(v) || v <= 0
})
```

### Risk-free rates

When the engine resolves DGS3MO, it attaches cumulative risk-free rate data to DataFrames. This enables `RiskAdjustedPct`:

```go
df.RiskFreeRates()                  // []float64 of cumulative risk-free values
df.SetRiskFreeRates(rates)          // attach risk-free rates (engine does this automatically)
```

### Error-only DataFrames

`data.WithErr` creates a DataFrame that carries only an error. Useful for signal functions that fail early:

```go
func MySignal(ctx context.Context, u universe.Universe) *data.DataFrame {
    df, err := u.At(ctx, u.CurrentDate(), data.MetricClose)
    if err != nil {
        return data.WithErr(err)
    }
    // ...
}
```

### Chaining

DataFrame operations return new DataFrames, so they chain naturally:

```go
// 20-day rolling average of 5-period percent change for AAPL price
result := df.Assets(aapl).Metrics(data.Price).Pct(5).Rolling(20).Mean()

// Risk-adjusted momentum as a percentage
momentum := prices.RiskAdjustedPct(1).MulScalar(100)
```

`RiskAdjustedPct` subtracts the risk-free return over the same period from each column's percent change. The engine automatically attaches cumulative risk-free rate data (DGS3MO) to DataFrames returned by `Fetch`/`FetchAt` when a risk-free asset is configured. If no risk-free data is attached, `RiskAdjustedPct` sets an error on the returned DataFrame.

## Signals

Signals are reusable computations that derive new time series from market data -- prices, volume, fundamentals, economic indicators. They live in the `signal` package as plain functions. Each signal takes a universe and returns a new DataFrame with one column per asset containing the computed score.

Signals operate on market data, not portfolio state. A signal like `signal.Momentum(ctx, u, portfolio.Months(12))` produces the same output regardless of what the portfolio holds or how it has traded. This is the key distinction from portfolio metrics (see [Portfolio - Performance measurement](portfolio.md#performance-measurement)), which operate on the returns and transactions of a specific portfolio and change based on the portfolio's particular trading history.

```go
mom3 := signal.Momentum(ctx, riskOn, portfolio.Months(3))
mom6 := signal.Momentum(ctx, riskOn, portfolio.Months(6))
mom12 := signal.Momentum(ctx, riskOn, portfolio.Months(12))

composite := mom3.Add(mom6).Add(mom12).DivScalar(3)
if err := composite.Err(); err != nil {
    // handle error
}
```

Signals name a concept. `signal.EarningsYield(ctx, u)` is clearer than `df.Metrics(data.EPS).Div(df.Metrics(data.Price))`, even though they compute the same thing. Because signals return DataFrames, they compose through DataFrame arithmetic.

The input DataFrame must contain the metrics the signal needs. `Momentum` needs price data. `EarningsYield` needs EPS and price. If a required metric is missing, the signal returns a DataFrame with `.Err()` set.

### Built-in signals

| Signal | Description |
|--------|-------------|
| `Momentum(ctx, u, period, metrics...)` | Percent change over a lookback period |
| `EarningsYield(ctx, u, t...)` | Earnings per share divided by price |
| `Volatility(ctx, u, period, metrics...)` | Rolling standard deviation of returns |

### Custom signals

A signal is any function that takes a universe and returns a DataFrame:

```go
func BookToPrice(ctx context.Context, u universe.Universe) *data.DataFrame {
    df, err := u.At(ctx, u.CurrentDate(), data.BookValue, data.Price)
    if err != nil {
        return data.WithErr(err)
    }
    // ... manual computation like EarningsYield ...
}
```

## Frequency and aggregation

### Frequency

```go
type Frequency int

const (
    Tick Frequency = iota
    Daily
    Weekly
    Monthly
    Quarterly
    Yearly
)
```

### Aggregation

Aggregation is expressed as methods on `DownsampledDataFrame` rather than a top-level enum. The available aggregations are: `Mean()`, `Sum()`, `Max()`, `Min()`, `First()`, `Last()`, `Std()`, `Variance()`.
