# DataFrame Stats API and Resample Builder

## Problem

The `data` package has three interrelated design issues:

1. **Aggregation constants collide with desired method names.** `Mean`, `Max`, `Min`, `Sum` are `Aggregation` iota constants used by `Resample()`. These names should be DataFrame methods, not enum values.

2. **Statistical operations are standalone functions on `[]float64` instead of DataFrame methods.** `SliceMean`, `Variance`, `Stddev`, `Covariance`, `PeriodsReturns` in `data/stats.go` operate on raw slices. Since DataFrame is the central abstraction, these should be methods on it.

3. **`Max()` and `Min()` axis inconsistency.** The existing `Max()` and `Min()` methods reduce across assets (per timestamp), but `Rolling(n).Max()` and `Rolling(n).Min()` reduce across time (per column). The new `Mean()`, `Std()`, `Variance()` methods reduce time. The cross-asset `Max()`/`Min()` need different names.

## Design

### 1. Downsample and Upsample replace Resample

`Resample(freq, agg)` is replaced by two separate builders that make invalid states unrepresentable at the type level. This matches the existing `Rolling(n)` builder pattern.

```go
type DownsampledDataFrame struct {
    df   *DataFrame
    freq Frequency
}

type UpsampledDataFrame struct {
    df   *DataFrame
    freq Frequency
}

func (df *DataFrame) Downsample(freq Frequency) *DownsampledDataFrame
func (df *DataFrame) Upsample(freq Frequency) *UpsampledDataFrame
```

**Downsample methods** (aggregate values within each period):

```go
func (d *DownsampledDataFrame) Mean() *DataFrame
func (d *DownsampledDataFrame) Sum() *DataFrame
func (d *DownsampledDataFrame) Max() *DataFrame
func (d *DownsampledDataFrame) Min() *DataFrame
func (d *DownsampledDataFrame) Std() *DataFrame
func (d *DownsampledDataFrame) Variance() *DataFrame
func (d *DownsampledDataFrame) First() *DataFrame
func (d *DownsampledDataFrame) Last() *DataFrame
```

**Upsample methods** (fill gaps when increasing frequency):

```go
func (u *UpsampledDataFrame) ForwardFill() *DataFrame
func (u *UpsampledDataFrame) BackFill() *DataFrame
func (u *UpsampledDataFrame) Interpolate() *DataFrame
```

The compiler prevents calling `Mean()` on an upsampler or `ForwardFill()` on a downsampler -- no runtime error or panic needed.

**Frequency inference:** The builders do not infer source frequency. The caller specifies the target frequency. The builder groups timestamps into periods of the target frequency and applies the method. For downsampling, if a period contains only one value, that value is returned as-is. For upsampling, the builder generates timestamps at the target frequency between existing timestamps and fills using the chosen method.

The old `Resample(freq, agg)` method, the `Aggregation` type, its constants (`Last`, `First`, `Sum`, `Mean`, `Max`, `Min`), the OHLC aliases (`Close`, `Open`, `High`, `Low`), and the internal `aggregate()` helper function are all deleted.

### 2. DataFrame column-wise stats methods

These methods reduce the time dimension, returning a single-row DataFrame with the same asset and metric dimensions. All use **sample** statistics (N-1 denominator) for variance and standard deviation.

```go
func (df *DataFrame) Mean() *DataFrame
func (df *DataFrame) Std() *DataFrame
func (df *DataFrame) Variance() *DataFrame
func (df *DataFrame) Sum() *DataFrame
func (df *DataFrame) Max() *DataFrame
func (df *DataFrame) Min() *DataFrame
```

For example, a DataFrame with assets SPY, EFA and metrics Price, Volume produces a single-row DataFrame with 4 values: Mean(SPY.Price), Mean(SPY.Volume), Mean(EFA.Price), Mean(EFA.Volume).

These are consistent with `Rolling(n).Mean()`, `Rolling(n).Std()`, etc. -- same axis, same names. The difference is that Rolling uses a sliding window while these use the full time range.

**Edge cases:**
- Empty DataFrame: returns empty DataFrame.
- Single timestamp: `Mean()`, `Sum()`, `Max()`, `Min()` return the values as-is. `Variance()` and `Std()` return 0 (fewer than 2 values).
- NaN values: propagate through computations (a column containing NaN produces NaN in the result). Callers can use `Drop(math.NaN())` to filter before computing.

### 3. Rename existing cross-asset Max/Min/IdxMax

The existing `Max()`, `Min()`, and `IdxMax()` methods reduce across the asset dimension (per timestamp). They are renamed to clarify the axis:

| Old | New |
|-----|-----|
| `Max()` | `MaxAcrossAssets()` |
| `Min()` | `MinAcrossAssets()` |
| `IdxMax()` | `IdxMaxAcrossAssets()` |

These return a DataFrame with one synthetic asset and the same time/metric dimensions. Their behavior does not change.

### 4. Covariance

`Covariance` is a method on DataFrame that computes sample covariance (N-1 denominator) between columns. Its behavior depends on the number of assets provided:

**Single asset -- cross-metric covariance:**

```go
df.Covariance(spy)
// Returns: asset=spy, composite metrics Price:Volume, Price:Dividend, Volume:Dividend
// Each value is the covariance between those two metric columns for spy.
```

Result shape: single-row DataFrame with the original asset and M*(M-1)/2 composite metric keys (where M is the number of metrics).

**Two assets -- per-metric covariance:**

```go
df.Covariance(portfolioAsset, spy)
// Returns: composite asset=_PORTFOLIO_:SPY, metrics Price, Volume
// Each value is the covariance between the two assets for that metric.
```

Result shape: single-row DataFrame with one composite asset key and the original metrics.

**Three+ assets -- all unique pairs, per metric:**

```go
df.Covariance(spy, efa, voo)
// Returns: composite assets SPY:EFA, SPY:VOO, EFA:VOO, metrics Price, Volume
// Each value is the covariance between that asset pair for that metric.
```

Result shape: single-row DataFrame with N*(N-1)/2 composite asset keys and the original metrics.

Signature:

```go
func (df *DataFrame) Covariance(assets ...asset.Asset) *DataFrame
```

**Edge cases:**
- Zero assets: returns empty DataFrame.
- Asset not found in DataFrame: that asset's columns are treated as missing; the result excludes pairs involving it.
- Fewer than 2 timestamps: returns 0 for all covariance values (insufficient data for sample covariance).

### 5. Composite keys

Composite keys represent pairs for covariance results. The separator is `:`.

**Composite asset keys:**

```go
// CompositeAsset creates an asset representing a pair.
func CompositeAsset(a, b asset.Asset) asset.Asset {
    return asset.Asset{
        CompositeFigi: a.CompositeFigi + ":" + b.CompositeFigi,
        Ticker:        a.Ticker + ":" + b.Ticker,
    }
}
```

**Composite metric keys:**

```go
// CompositeMetric creates a metric representing a pair.
func CompositeMetric(a, b Metric) Metric {
    return Metric(string(a) + ":" + string(b))
}
```

Pairs are ordered by their position in the DataFrame's asset or metric list (first asset/metric comes first in the composite key). This ensures deterministic ordering.

### 6. Rolling updates

Add `Variance()` to `RollingDataFrame` (currently missing). Fix `Std()` to use **sample** standard deviation (N-1 denominator) instead of the current population standard deviation (N denominator).

The full Rolling method set becomes:

```go
func (r *RollingDataFrame) Mean() *DataFrame
func (r *RollingDataFrame) Sum() *DataFrame
func (r *RollingDataFrame) Max() *DataFrame
func (r *RollingDataFrame) Min() *DataFrame
func (r *RollingDataFrame) Std() *DataFrame          // fix: N-1 denominator
func (r *RollingDataFrame) Variance() *DataFrame      // new
func (r *RollingDataFrame) Percentile(p float64) *DataFrame
```

The Downsample builder methods also use sample (N-1) for `Std()` and `Variance()`.

### 7. PeriodsReturns replaced by Pct

`DataFrame.Pct()` already computes period-over-period returns (percent change). The standalone `PeriodsReturns` function in `data/stats.go` duplicates this and is deleted. Callers use `df.Pct()` instead.

### 8. AnnualizationFactor

**SUPERSEDED**: The metric-helpers-refactor spec (`2026-03-12-metric-helpers-refactor-design.md`) moves `AnnualizationFactor` to the `portfolio` package as an unexported helper. It is a financial concept (252/12) that does not belong in the generic `data` package. `data/stats.go` and `data/stats_test.go` are deleted.

### 9. Deleted code

| Item | Reason |
|------|--------|
| `Aggregation` type and all constants | Replaced by Downsample/Upsample builder methods |
| OHLC aliases (`Close`, `Open`, `High`, `Low`) | Removed with Aggregation type |
| `aggregate()` helper in `data_frame.go` | Removed with Aggregation type |
| `Resample()` method | Replaced by `Downsample()` and `Upsample()` |
| `data/stats.go`: `SliceMean` | Replaced by `DataFrame.Mean()` |
| `data/stats.go`: `Variance` | Replaced by `DataFrame.Variance()` |
| `data/stats.go`: `Stddev` | Replaced by `DataFrame.Std()` |
| `data/stats.go`: `Covariance` | Replaced by `DataFrame.Covariance()` |
| `data/stats.go`: `PeriodsReturns` | Replaced by `DataFrame.Pct()` |

`AnnualizationFactor` is also deleted (see Section 8 -- superseded by metric-helpers-refactor spec). `data/stats.go` becomes empty and is deleted.

### 10. Impact on metric helpers refactor

The existing metric helpers refactor spec (`2026-03-12-metric-helpers-refactor-design.md`) references `data.SliceMean(x)`, `data.Stddev(x)`, `data.Variance(x)`, `data.Covariance(x, y)`, `data.PeriodsReturns(prices)`. These all change:

| Old (from refactor spec) | New |
|---|---|
| `data.SliceMean(er)` | `erDF.Mean().Value(asset, metric)` |
| `data.Stddev(er)` | `erDF.Std().Value(asset, metric)` |
| `data.Variance(bm)` | `bmDF.Variance().Value(asset, metric)` |
| `data.Covariance(r, bm)` | `df.Covariance(portfolioAsset, benchmarkAsset).Value(...)` |
| `data.PeriodsReturns(eq)` | `eqDF.Pct()` |

PerformanceMetric implementations should keep data in DataFrame form throughout rather than extracting to `[]float64` with `Column()`. The metric helpers refactor spec and plan need updating after this spec is approved.

### 11. Files modified

| File | Change |
|------|--------|
| `data/data_frame.go` | Add `Mean`, `Std`, `Variance`, `Sum`, `Max`, `Min`, `Covariance`; rename existing `Max`/`Min`/`IdxMax` to `*AcrossAssets`; remove `Resample` method and `aggregate` helper; add `Downsample`/`Upsample` constructors; add `CompositeAsset`/`CompositeMetric` helpers |
| `data/downsample.go` | New file: `DownsampledDataFrame` type with aggregation methods |
| `data/upsample.go` | New file: `UpsampledDataFrame` type with fill methods |
| `data/rolling_data_frame.go` | Add `Variance()` method; fix `Std()` to use N-1 denominator |
| `data/aggregation.go` | Delete file |
| `data/stats.go` | Remove all functions except `AnnualizationFactor` |
| `data/stats_test.go` | Remove tests for deleted functions; keep `AnnualizationFactor` tests |
| `data/data_frame_test.go` | Update resample tests to Downsample/Upsample pattern; update `Max`/`Min` tests for rename; add tests for new stats methods and Covariance |
| `data/rolling_data_frame_test.go` | Add `Variance` tests; update `Std` tests for N-1 behavior |

### 12. Testing strategy

All new methods get black-box ginkgo tests:

- `Mean`, `Std`, `Variance`, `Sum`, `Max`, `Min`: verify single-row result with known input, verify asset/metric dimensions preserved, verify edge cases (empty, single timestamp, NaN)
- `Covariance`: test 1-asset (cross-metric), 2-asset (per-metric), 3+-asset (all pairs) cases; verify composite key format; verify edge cases (missing asset, fewer than 2 timestamps)
- `DownsampledDataFrame`: test each method (Mean, Sum, Max, Min, Std, Variance, First, Last) with known weekly/monthly data
- `UpsampledDataFrame`: test ForwardFill, BackFill, Interpolate with known gaps
- `Rolling.Variance()`: verify against known values
- `Rolling.Std()`: verify N-1 denominator (breaking change from current N denominator)
- Composite keys: verify `CompositeAsset` and `CompositeMetric` produce expected formats
