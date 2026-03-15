# DataFrame Frequency Design

## Status

Draft

## Problem

DataFrame does not track the frequency of its data (daily, minute, tick, etc.).
The predicted portfolio feature (planned separately) needs to forward-fill data
from the last available date to a future trade date, which requires knowing the
data frequency. More generally, storing frequency on the DataFrame enables
validation in Upsample/Downsample and eliminates the need to infer frequency
from timestamp gaps.

## Decision

Add a `Frequency` field to `DataFrame`, set at construction time. Data providers
know the frequency of the data they serve and pass it through when constructing
DataFrames.

## DataFrame struct change

Add a `freq` field to `DataFrame`:

```go
type DataFrame struct {
    data       []float64
    times      []time.Time
    assets     []asset.Asset
    metrics    []Metric
    assetIndex map[string]int
    err        error
    freq       Frequency
}
```

## Constructor change

`NewDataFrame` gains a `freq Frequency` parameter between `metrics` and `data`:

```go
func NewDataFrame(
    times []time.Time,
    assets []asset.Asset,
    metrics []Metric,
    freq Frequency,
    data []float64,
) (*DataFrame, error)
```

The internal `mustNewDataFrame` helper must also be updated with the same
parameter.

## Accessor

```go
func (df *DataFrame) Frequency() Frequency { return df.freq }
```

## WithErr

`WithErr` creates a DataFrame with only an error. Its frequency will be `Tick`
(the zero value of `Frequency`). This is acceptable since the DataFrame cannot
be used for anything other than error propagation.

## Propagation through derived DataFrames

All methods that create new DataFrames must propagate the source frequency to
the result. Most of these go through `mustNewDataFrame`, so updating that
helper covers the majority. Methods that produce derived DataFrames include:

- Narrowing: `Assets`, `Metrics`, `Between`, `Filter`, `Drop`, `At`, `Last`
- Arithmetic: `Add`, `Sub`, `Mul`, `Div`, `AddScalar`, `SubScalar`,
  `MulScalar`, `DivScalar`
- Aggregation: `Mean`, `Sum`, `Max`, `Min`, `Variance`, `Std`, `Covariance`,
  `MaxAcrossAssets`, `MinAcrossAssets`, `IdxMaxAcrossAssets`
- Transforms: `Pct`, `Diff`, `Log`, `CumSum`, `CumMax`, `Shift`
- Merge: `MergeColumns` and `MergeTimes` (in `data/merge.go`) create new
  DataFrames. When merging, all input DataFrames must have the same frequency.
  If they differ, return an error. Empty DataFrames (from `mustNewDataFrame`
  with nil args) use the frequency of the non-empty operand.
- Other: `Copy`, `RenameMetric`, `AppendRow`, `Insert`, `Window`
- Resampling: `Downsample` and `Upsample` produce wrapper types
  (`DownsampledDataFrame`, `UpsampledDataFrame`) that eventually create
  DataFrames via their aggregation/fill methods. The resulting DataFrames
  carry the *target* frequency (the one passed to `Downsample`/`Upsample`),
  not the source frequency. `DownsampledDataFrame.aggregate` calls
  `mustNewDataFrame` and must pass `d.freq` (the target). Similarly,
  `UpsampledDataFrame.ForwardFill`, `BackFill`, and `Interpolate` must
  pass `u.freq`.

## Provider updates

### PVDataProvider

`PVDataProvider.Fetch` receives a `DataRequest` which already has a `Frequency`
field. Pass `req.Frequency` to `NewDataFrame`.

### TestProvider

`TestProvider` is used in tests. It should accept a frequency at construction
or default to `Daily`.

### ExampleData

The `ExampleData()` helper that produces test data should use `Daily`.

## Account perfData

`Account.UpdatePrices` constructs a perfData DataFrame with portfolio equity
series. This is daily data -- use `Daily`.

## SQLite round-trip

The frequency must be persisted in SQLite. Add a `frequency` key to the
metadata table (alongside `schema_version`, `cash`, etc.). Store the string
representation of the Frequency constant. Read it back in `FromSQLite` and
pass it to `NewDataFrame` when reconstructing perfData.

A `ParseFrequency(string) (Frequency, error)` function must be added to
`data/frequency.go` to convert the stored string back to a `Frequency`
constant (the inverse of `Frequency.String()`).

Note: This does NOT require a schema version bump. The frequency is stored
in the existing metadata table as a key-value pair. If the key is missing
(loading a v3 database created before this change), default to `Daily`.

## Files Changed

### Source

- `data/data_frame.go` -- add `freq` field, update `NewDataFrame` and
  `mustNewDataFrame` signatures, add `Frequency()` accessor, propagate freq
  through all derived DataFrame methods
- `data/downsample.go` -- pass target `d.freq` to `mustNewDataFrame` in
  `aggregate` method
- `data/upsample.go` -- pass target `u.freq` to `mustNewDataFrame` in
  `ForwardFill`, `BackFill`, and `Interpolate` methods
- `data/merge.go` -- update `MergeColumns` and `MergeTimes` to propagate
  frequency through `mustNewDataFrame` and `NewDataFrame` calls
- `data/frequency.go` -- add `ParseFrequency(string) (Frequency, error)`
- `data/pvdata_provider.go` -- pass `req.Frequency` to `NewDataFrame`
- `data/test_provider.go` -- accept or default frequency, pass to `NewDataFrame`
- `data/example_data.go` -- pass `Daily` to `NewDataFrame`
- `signal/earnings_yield.go` -- pass appropriate frequency to `NewDataFrame`
- `engine/engine.go` -- update `NewDataFrame` calls with frequency
- `portfolio/account.go` -- pass `Daily` when constructing perfData
- `portfolio/sqlite.go` -- persist and restore frequency via metadata table

### Tests

Every test file that calls `NewDataFrame` must be updated with the new
parameter. Key files:

- `data/data_frame_test.go`
- `data/merge_test.go`
- `data/rolling_data_frame_test.go`
- `data/test_provider_test.go`
- `portfolio/rebalance_test.go`
- `portfolio/sqlite_test.go`
- `portfolio/annotation_test.go`
- `portfolio/testutil_test.go`
- `portfolio/trade_metrics_test.go`
- `portfolio/weighting_test.go`
- `portfolio/selector_test.go`
- `portfolio/account_test.go`
- `engine/backtest_test.go`
- `engine/fetch_test.go`
- `engine/example_test.go`
- `engine/simulated_broker_test.go`
- `engine/rated_universe_test.go`
- `signal/` test files
- `universe/` test files

All test calls should use `data.Daily` unless testing a specific frequency.
