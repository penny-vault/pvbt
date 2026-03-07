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
| `Price` | Trade price |
| `Volume` | Trade volume |
| `Bid` | Best bid price |
| `Ask` | Best ask price |
| `Revenue` | Total revenue |
| `NetIncome` | Net income |
| `EarningsPerShare` | Diluted EPS |
| `TotalDebt` | Total debt |
| `TotalAssets` | Total assets |
| `FreeCashFlow` | Free cash flow |
| `BookValue` | Book value per share |
| `MarketCap` | Market capitalization |
| `Unemployment` | Unemployment rate |

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

### Index providers

Index providers supply historical index membership data (e.g. which companies were in the S&P 500 on a given date). The `universe` package uses this to build time-varying universes:

```go
type IndexProvider interface {
    IndexMembers(ctx context.Context, index string, t time.Time) ([]asset.Asset, error)
}
```

A database provider typically implements `IndexProvider` alongside `BatchProvider`, since both use the same database connection.

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
df.Value(aapl, data.Price)          // most recent value
df.Value(aapl, data.Price, t)       // value at time t
df.Column(aapl, data.Price)         // contiguous []float64, gonum-compatible
df.At(t)                            // single-row DataFrame at time t
df.Last()                           // single-row DataFrame, most recent
df.Copy()                           // deep copy
df.Table()                          // ASCII table for debugging
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
result := df.MulScalar(0.5)
result := df.DivScalar(100.0)
```

### Aggregation across assets

These collapse the asset dimension, producing one value per timestamp per metric:

```go
df.Max()                            // max across all assets
df.Min()                            // min across all assets
df.IdxMax()                         // which asset has the max
```

### Common transforms

```go
df.Pct()                            // percent change, 1-period default
df.Pct(5)                           // 5-period percent change
df.Diff()                           // first difference
df.Log()                            // natural logarithm
df.CumSum()                         // cumulative sum
df.Shift(1)                         // shift forward by 1 period
df.Shift(-1)                        // shift backward by 1 period
```

### Resampling

```go
df.Resample(data.Weekly, data.Last)  // weekly close
df.Resample(data.Weekly, data.Sum)   // weekly total volume
df.Resample(data.Monthly, data.Max)  // monthly high
```

OHLC bars are not a primitive. They are a pattern of resamples: Open is `First`, High is `Max`, Low is `Min`, Close is `Last`, Volume is `Sum`.

### Rolling window operations

```go
df.Rolling(20).Mean()               // 20-period rolling mean (SMA)
df.Rolling(20).Sum()                // rolling sum
df.Rolling(20).Max()                // rolling max
df.Rolling(20).Min()                // rolling min
df.Rolling(20).Std()                // rolling standard deviation
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

### Chaining

DataFrame operations return new DataFrames, so they chain naturally:

```go
// 20-day rolling average of 5-period percent change for AAPL price
result := df.Assets(aapl).Metrics(data.Price).Pct(5).Rolling(20).Mean()
```

## Signals

Signals are reusable computations that derive new time series from metrics. They live in the `signal` package as plain functions. Each signal takes a DataFrame and returns a new DataFrame with one column per asset containing the computed score.

```go
mom3 := signal.Momentum(df, 3)
mom6 := signal.Momentum(df, 6)
mom12 := signal.Momentum(df, 12)

composite := mom3.Add(mom6).Add(mom12).DivScalar(3)
```

Signals name a concept. `signal.EarningsYield(df)` is clearer than `df.Metrics(data.EPS).Div(df.Metrics(data.Price))`, even though they compute the same thing. Because signals return DataFrames, they compose through DataFrame arithmetic.

The input DataFrame must contain the metrics the signal needs. `Momentum` needs price data. `EarningsYield` needs EPS and price. If a required metric is missing, the signal panics.

### Built-in signals

| Signal | Description |
|--------|-------------|
| `Momentum(df, periods)` | Percent change over a lookback period |
| `EarningsYield(df)` | Earnings per share divided by price |
| `Volatility(df, periods)` | Rolling standard deviation of returns |

### Custom signals

A signal is any function that takes a DataFrame and returns a DataFrame:

```go
func BookToPrice(df *data.DataFrame) *data.DataFrame {
    return df.Metrics(data.BookValue).Div(df.Metrics(data.Price))
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

```go
type Aggregation int

const (
    Last Aggregation = iota
    First
    Sum
    Mean
    Max
    Min
)

// OHLC aliases
const (
    Close = Last
    Open  = First
    High  = Max
    Low   = Min
)
```

## Automatic frequency alignment

When you combine data of different frequencies, the engine aligns them automatically. If you divide daily price by quarterly earnings to get a P/E ratio, the engine forward-fills the quarterly data to daily frequency behind the scenes.

Explicit resampling is for situations where you want to change how data is represented -- aggregating daily prices into weekly bars, or computing monthly returns from daily prices.
