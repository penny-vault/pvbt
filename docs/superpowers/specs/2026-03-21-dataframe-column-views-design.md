# DataFrame Column-Slice View Refactor

## Problem

The DataFrame's column-major flat slab layout (`data []float64`) forces every
time-axis, metric-axis, or asset-axis operation to allocate and copy the entire
data slab. Profiling the backtest engine shows this dominates both CPU and memory:

- `NewDataFrame` allocated 28.3 GB (49% of 57.5 GB total) in a single run
- `DataFrame.Copy` allocated 3.5 GB
- `DataFrame.At` allocated 6.3 GB
- The hot path `Window -> Metrics -> Pct -> Drop -> Column` runs ~2,400 times
  per backtest day (85+ metrics x 7 windows each), creating 4 intermediate
  DataFrames per invocation

The root cause is the flat slab layout: column offsets are computed as
`(aIdx*M + mIdx) * T`, coupling every column to the total timestamp count.
Slicing the time axis requires copying every column into a new slab with a
different stride.

## Solution

Replace the flat `data []float64` slab with `columns [][]float64`, where each
(asset, metric) pair owns its own contiguous `[]float64`. Column index
`aIdx*len(metrics) + mIdx` maps directly to the slice.

This makes time-axis, metric-axis, and asset-axis views zero-copy operations:
sub-slice or select column slices without allocating or copying data.

## Data Structure

```go
type DataFrame struct {
    columns [][]float64  // one []float64 per (asset, metric) pair

    times      []time.Time
    dateKeys   []int32
    assets     []asset.Asset
    metrics    []Metric
    assetIndex map[string]int

    err           error
    freq          Frequency
    riskFreeRates []float64
    source        DataSource
}
```

Column access simplifies from offset arithmetic to direct indexing:

```go
func (df *DataFrame) colIdx(aIdx, mIdx int) int {
    return aIdx*len(df.metrics) + mIdx
}

func (df *DataFrame) colSlice(aIdx, mIdx int) []float64 {
    return df.columns[df.colIdx(aIdx, mIdx)]
}
```

`colOffset` is eliminated entirely. `Len()` continues to return `len(df.times)`.

## Construction API

`NewDataFrame` changes its signature to accept `[][]float64`:

```go
func NewDataFrame(
    times []time.Time,
    assets []asset.Asset,
    metrics []Metric,
    freq Frequency,
    columns [][]float64,
) (*DataFrame, error)
```

Validation checks that `len(columns) == A*M` and each column has length
`len(times)`.

Internal construction uses the same signature via `mustNewDataFrame`. Methods
that build child DataFrames construct `[][]float64` directly instead of packing
into a flat slab.

This is a breaking API change. All callers of `NewDataFrame` (internal and
external) must update from flat `[]float64` to `[][]float64`.

To reduce migration risk, provide a helper for mechanical conversion of existing
flat-slab callers:

```go
func SlabToColumns(slab []float64, numCols, colLen int) [][]float64 {
    cols := make([][]float64, numCols)
    for i := range cols {
        cols[i] = slab[i*colLen : (i+1)*colLen : (i+1)*colLen]
    }
    return cols
}
```

This allows callers to wrap their existing flat slab with
`SlabToColumns(data, A*M, T)` as a first step, then optimize to build columns
directly in a follow-up pass.

## View Operations

### Time-axis views (Between, Window, sliceByTimeIndices)

Zero-copy. Sub-slice each column and `df.times` using three-index slice
expressions to cap capacity and prevent aliasing from AppendRow:

```go
func (df *DataFrame) sliceByTimeIndices(startIdx, endIdx int) *DataFrame {
    cols := make([][]float64, len(df.columns))
    for i, col := range df.columns {
        cols[i] = col[startIdx:endIdx:endIdx]  // cap prevents capacity leakage
    }
    // times, dateKeys, riskFreeRates also sub-sliced with capped capacity
    // assets, metrics, assetIndex copied (see Ownership Model)
}
```

Allocates only the `[][]float64` header (A*M slice headers, typically 6-15
pointers). No data is copied.

### Metric-axis views (Metrics)

Zero-copy. Select matching column slices:

```go
func (df *DataFrame) Metrics(metrics ...Metric) *DataFrame {
    cols := make([][]float64, len(df.assets)*len(matched))
    for aIdx := range df.assets {
        for newMIdx, oldMIdx := range matchedIdx {
            cols[aIdx*len(matched)+newMIdx] = df.columns[df.colIdx(aIdx, oldMIdx)]
        }
    }
    // times shared, new metrics slice with matched entries
}
```

### Asset-axis views (Assets)

Same pattern: select column slices for matched assets.

### Window(nil)

Returns a view instead of `Copy()`. The view has its own `columns` slice header
but shares the underlying column data. Three-index slicing ensures that
`AppendRow` on the parent cannot write into the view's capacity region.

### Transform operations (Apply, Pct, CumMax, CumSum, Diff, Log, Shift)

Allocate new column data (transforms produce new values):

```go
func (df *DataFrame) Apply(transform func([]float64) []float64) *DataFrame {
    cols := make([][]float64, len(df.columns))
    for i, col := range df.columns {
        cols[i] = transform(col)
    }
}
```

### Non-contiguous selection (Drop, Filter, sliceByIndices)

Allocate new column data (arbitrary index selection cannot be a sub-slice):

```go
func (df *DataFrame) sliceByIndices(indices []int) *DataFrame {
    cols := make([][]float64, len(df.columns))
    for i, srcCol := range df.columns {
        dst := make([]float64, len(indices))
        for newIdx, oldIdx := range indices {
            dst[newIdx] = srcCol[oldIdx]
        }
        cols[i] = dst
    }
}
```

### Arithmetic (Add, Sub, Mul, Div)

Allocate new column data (element-wise operations produce new values).

**`elemWiseOp`** uses a `colPair` struct storing flat-slab offsets (`selfOff`,
`otherOff`). These change to store column indices instead, and the operation
applies per-column using `df.columns[idx]`:

```go
// Before: dst := newData[dstOff : dstOff+timeLen]
// After:
dst := make([]float64, timeLen)
apply(dst, df.columns[selfIdx], other.columns[otherIdx])
cols[resultIdx] = dst
```

**`broadcastOp`** does not use `colPair`. It currently uses
`result.colOffset(aIdx, rMIdx)` for writing and `other.colSlice(otherAIdx, mIdx)`
for reading. Both change to direct column access: `result.columns[colIdx]` and
`other.columns[otherColIdx]`.

### Scalar arithmetic (AddScalar, MulScalar)

Currently call `floats.AddConst(scalar, result.data)` and
`floats.Scale(scalar, result.data)` on the single flat slab. With per-column
storage, these must iterate over columns:

```go
for _, col := range result.columns {
    floats.AddConst(scalar, col)
}
```

This loses the single-call gonum optimization over a contiguous array but is
negligible for typical column counts (6-15).

### Copy

Deep copy: allocates new slices for each column.

### Reduce, Covariance, Correlation

Aggregations that produce single-row or pair DataFrames. Allocate new column
data. These use `colSlice()` for reading (which is updated) and build result
`[][]float64` directly.

## Point Access Methods

The following methods use `colOffset` and `df.data[off+...]` for element access.
All change to `df.columns[colIdx][tIdx]`:

- **`Value(asset, metric)`** -- returns last value: `df.columns[colIdx][len(col)-1]`
- **`ValueAt(asset, metric, timestamp)`** -- looks up time index, then
  `df.columns[colIdx][tIdx]`
- **`Column(asset, metric)`** -- returns `df.columns[colIdx]` (shared slice,
  same contract as today)

## Iteration Methods

These methods iterate over the data slab using `colOffset(aIdx, mIdx) + tIdx`.
All change to `df.columns[colIdx][tIdx]`:

- **`Table()`** -- debug string formatting
- **`MaxAcrossAssets()`**, **`MinAcrossAssets()`** -- cross-asset aggregation
  per timestamp
- **`CountWhere(metric, predicate)`** -- count matching values per timestamp
- **`IdxMaxAcrossAssets()`** -- find asset with max value per timestamp

## Filter and Drop

**Filter** currently builds a reusable single-row DataFrame with a flat
`rowData` slab, populating it via `df.data[df.colOffset(aIdx, mIdx)+tIdx]`.
With per-column storage, the row DataFrame needs `[][]float64` with one
single-element slice per column. To avoid per-row allocation, pre-allocate
the column slices and repoint them each iteration:

```go
rowCols := make([][]float64, len(df.columns))
buffers := make([]float64, len(df.columns))  // one element per column
for i := range rowCols {
    rowCols[i] = buffers[i : i+1 : i+1]
}
rowDF := mustNewDataFrame([]time.Time{{}}, df.assets, df.metrics, df.freq, rowCols)

for tIdx, timestamp := range df.times {
    for i, col := range df.columns {
        buffers[i] = col[tIdx]
    }
    rowDF.times[0] = timestamp
    if predicate(timestamp, rowDF) {
        indices = append(indices, tIdx)
    }
}
```

**Drop (non-NaN path)** delegates to Filter, which scans `row.data` for the
sentinel value. With per-column storage, the predicate iterates `row.columns`
instead. The NaN fast path (`dropNaN`) iterates `df.columns` directly.

## Mutation Methods

### AppendRow

Per-column `append()` replaces the full slab rebuild:

```go
func (df *DataFrame) AppendRow(timestamp time.Time, values []float64) error {
    for i, val := range values {
        df.columns[i] = append(df.columns[i], val)
    }
    df.times = append(df.times, timestamp)
}
```

O(1) amortized instead of O(A*M*T). Views holding sub-slices of the old columns
are unaffected because each view holds its own slice header, and three-index
slicing caps the view's capacity so parent appends never write into the view's
backing array region.

### Insert

`Insert` mutates `df.columns` (appending a new entry), `df.assets`, `df.metrics`,
and `df.assetIndex` in place. Because views copy `assets` and `metrics` slices
(see Ownership Model), and hold their own `columns` header, Insert on the parent
does not affect views. The shared `assetIndex` map would be mutated, but views
do not retain a reference to the parent's map (they build or receive their own).

## Ownership Model

**Shared by reference:**

- Column data (`[]float64` entries in `df.columns`) -- sub-slices of parent data
- `df.times` -- sub-slice of parent times (three-index capped)
- `df.dateKeys` -- sub-slice of parent keys (three-index capped)
- `df.riskFreeRates` -- sub-slice of parent rates (three-index capped)
- `df.source` -- interface reference, shared; no method mutates it

**Copied per view (small, negligible cost):**

- `df.columns` -- each view allocates its own `[][]float64` header
- `df.assets` -- copied via `make + copy` (typically 1-3 elements)
- `df.metrics` -- copied via `make + copy` (typically 3-5 elements)
- `df.assetIndex` -- each view builds its own map or copies the parent's

Copying `assets`, `metrics`, and `assetIndex` prevents mutation leakage from
`Insert` and ensures views are structurally independent. The cost is negligible
since these slices/maps have single-digit element counts.

**Owned per DataFrame:**

- `df.err`, `df.freq` -- value types

**Column() contract preserved:** Returns a shared slice into the column data.
Callers can mutate the returned slice and it affects the DataFrame. This is the
existing documented behavior.

**ensureDateKeys() on views:** When a view shares `dateKeys` from a parent via
sub-slicing, `ensureDateKeys()` checks `if df.dateKeys != nil` and returns early.
If a view was created from a parent that had not yet built dateKeys, the view
builds its own from its `df.times` sub-slice. This means views and parents may
have independently-allocated dateKey arrays rather than shared ones, which is
correct.

## DownsampledDataFrame and UpsampledDataFrame

Both types in the `data` package directly access DataFrame fields and need
updating on both read and write sides.

**DownsampledDataFrame** read side changes from
`d.df.data[srcOff+start : srcOff+end]` to `d.df.columns[colIdx][start:end]`.
Write side changes from building a flat `newData` slab to building `[][]float64`.

**UpsampledDataFrame** read side uses `colSlice()` which is updated
transparently. Write side (ForwardFill, BackFill, Interpolate) currently
computes `dstOff := (aIdx*metricLen + mIdx) * len(newTimes)` and writes into a
flat `newData` slab. All three methods change to build `[][]float64` output.

## Test Strategy

**Existing tests that continue to pass (updated for new API):**

- "Copy returns independent deep copy"
- "does not affect prior Window snapshots" (now tests view path instead of Copy)
- "Column returns contiguous slice sharing underlying data"
- "Between produces independent copy" -- updated to verify view semantics: column
  data is shared, structural independence preserved
- "Assets produces independent copy" -- same
- "Metrics produces independent copy" -- same
- All arithmetic, transform, and aggregation tests (assertions unchanged)

**New tests:**

- View aliasing: `Between` result shares column data with parent; mutating via
  `Column()` on the view is visible through the parent
- `Metrics` view shares column data with parent
- `Assets` view shares column data with parent
- AppendRow on parent does not affect existing views (covers three-index capping)
- Insert on parent does not affect existing views

**Tests that need semantic updates:**

- "Between produces independent copy" and "Assets/Metrics produces independent
  copy" currently verify that modifying the result does not affect the original.
  With views sharing data, modifying column data WILL affect the parent. These
  tests should be updated to verify that structural changes (different time range,
  different metric set) are independent while column data is shared.

## Expected Impact

For a typical metric with 1 asset and 3 metrics over T=5000 timestamps:

| Step | Before | After |
|------|--------|-------|
| Window/Between | Copy 3*5000*8 = 120KB | 3 slice headers = 72 bytes |
| Metrics | Copy selected columns ~40KB | Pick slice refs = 24 bytes |
| Pct (Apply) | Alloc new slab ~120KB | Alloc 3 columns ~120KB (same) |
| Drop(NaN) | Alloc new slab ~120KB | Alloc 3 columns ~120KB (same) |
| Window(nil) | Full Copy ~120KB | 3 slice headers = 72 bytes |

Across 85+ metrics x 7 windows x many backtest days, Window and Metrics going
from ~120KB to ~72 bytes each eliminates the dominant allocation source.

AppendRow goes from O(A*M*T) slab rebuild to O(A*M) amortized per-column append.

## Files Requiring Changes

| File | Nature of change |
|------|------------------|
| `data/data_frame.go` | Core refactor: struct, all methods |
| `data/data_frame_test.go` | API updates, new view tests, updated copy semantics |
| `data/downsample.go` | Replace slab access with column indexing (read and write) |
| `data/upsample.go` | Update output construction to `[][]float64` |
| `data/merge.go` | Update MergeColumns/MergeTimes for column slices |
| `data/rolling_data_frame.go` | Uses Apply, likely no changes |
| `engine/engine.go` | Update ForwardFillTo, NewDataFrame calls |
| `engine/forward_fill_test.go` | Update NewDataFrame calls |
| `engine/*_test.go` | Update NewDataFrame calls in test fixtures |
| `portfolio/account.go` | Update NewDataFrame call in UpdatePrices |
| `portfolio/snapshot.go` | Update if it constructs DataFrames |
| `portfolio/*_test.go` | Update NewDataFrame calls in test fixtures |
| `signal/*.go` | Unlikely direct DataFrame construction |
| `report/*.go` | Update if constructing DataFrames |
| `data/sqlite.go` | Update DataFrame construction from DB rows |
