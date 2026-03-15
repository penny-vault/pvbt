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

`WithErr` creates a DataFrame with only an error. Its frequency is zero-valued
(undefined), which is acceptable since the DataFrame cannot be used for anything
other than error propagation.

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

Note: This does NOT require a schema version bump. The frequency is stored
in the existing metadata table as a key-value pair.

## Files Changed

### Source

- `data/data_frame.go` -- add `freq` field, update `NewDataFrame` and
  `mustNewDataFrame` signatures, add `Frequency()` accessor, propagate freq
  through all derived DataFrame methods
- `data/downsample.go` -- pass target `d.freq` to `mustNewDataFrame` in
  `aggregate` method
- `data/upsample.go` -- pass target `u.freq` to `mustNewDataFrame` in
  `ForwardFill`, `BackFill`, and `Interpolate` methods
- `data/pvdata_provider.go` -- pass `req.Frequency` to `NewDataFrame`
- `data/test_provider.go` -- accept or default frequency, pass to `NewDataFrame`
- `data/example_data.go` -- pass `Daily` to `NewDataFrame` (if this file exists)
- `portfolio/account.go` -- pass `Daily` when constructing perfData
- `portfolio/sqlite.go` -- persist and restore frequency via metadata table

### Tests

Every test file that calls `NewDataFrame` must be updated with the new
parameter. Key files:

- `data/data_frame_test.go`
- `data/data_suite_test.go` (if it has helpers)
- `portfolio/rebalance_test.go`
- `portfolio/sqlite_test.go`
- `portfolio/annotation_test.go`
- `engine/backtest_test.go`
- `engine/fetch_test.go`
- `engine/example_test.go`
- `signal/` test files
- `universe/` test files

All test calls should use `data.Daily` unless testing a specific frequency.
