# DataFrame Column-Slice View Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat `data []float64` slab with `columns [][]float64` so time/metric/asset slicing becomes zero-copy.

**Architecture:** Each (asset, metric) column becomes its own `[]float64`. View operations (Between, Window, Metrics, Assets) sub-slice or select column references instead of copying data. Transform operations (Apply, Pct, Drop) still allocate new column data. Three-index slice expressions cap view capacity to prevent AppendRow aliasing.

**Tech Stack:** Go 1.24, gonum/floats, Ginkgo/Gomega test framework

**Spec:** `docs/superpowers/specs/2026-03-21-dataframe-column-views-design.md`

---

### Task 1: Core struct, constructors, and SlabToColumns helper

**Files:**
- Modify: `data/data_frame.go:40-77` (struct definition)
- Modify: `data/data_frame.go:144-177` (NewDataFrame, mustNewDataFrame)
- Create helper: `SlabToColumns` in `data/data_frame.go`

- [ ] **Step 1: Change DataFrame struct**

Replace `data []float64` with `columns [][]float64` in the struct definition:

```go
type DataFrame struct {
	columns [][]float64

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

- [ ] **Step 2: Update NewDataFrame signature**

Change the last parameter from `data []float64` to `columns [][]float64`. Update validation to check `len(columns) == A*M` and each column length equals `len(times)`. Handle the nil/empty case (for empty DataFrames where all dimensions are 0):

```go
func NewDataFrame(times []time.Time, assets []asset.Asset, metrics []Metric, freq Frequency, columns [][]float64) (*DataFrame, error) {
	expected := len(assets) * len(metrics)
	if len(columns) != expected {
		return nil, fmt.Errorf("columns count %d does not match dimensions %d (assets=%d, metrics=%d)",
			len(columns), expected, len(assets), len(metrics))
	}

	for i, col := range columns {
		if len(col) != len(times) {
			return nil, fmt.Errorf("column %d length %d does not match time axis length %d",
				i, len(col), len(times))
		}
	}

	idx := make(map[string]int, len(assets))
	for i, a := range assets {
		idx[a.CompositeFigi] = i
	}

	return &DataFrame{
		columns:    columns,
		times:      times,
		assets:     assets,
		metrics:    metrics,
		assetIndex: idx,
		freq:       freq,
	}, nil
}
```

- [ ] **Step 3: Update mustNewDataFrame**

Same signature change. Passes `columns` through to `NewDataFrame`.

- [ ] **Step 4: Add SlabToColumns helper**

Add immediately after `NewDataFrame`:

```go
func SlabToColumns(slab []float64, numCols, colLen int) [][]float64 {
	cols := make([][]float64, numCols)
	for i := range cols {
		cols[i] = slab[i*colLen : (i+1)*colLen : (i+1)*colLen]
	}
	return cols
}
```

- [ ] **Step 5: Commit**

```
git add data/data_frame.go
git commit -m "refactor: change DataFrame storage from flat slab to per-column slices

Update struct, NewDataFrame, mustNewDataFrame signatures. Add
SlabToColumns migration helper. Code does not compile yet -- remaining
methods still reference df.data and colOffset."
```

Note: The project will NOT compile after this task. That is expected. Tasks 2-6 fix all internal references before external callers are updated.

---

### Task 2: Column access helpers and point access methods

**Files:**
- Modify: `data/data_frame.go:195-236` (colOffset, colSlice)
- Modify: `data/data_frame.go:310-359` (Value, ValueAt, Column)

- [ ] **Step 1: Replace colOffset with colIdx**

Delete `colOffset`. Add `colIdx`:

```go
func (df *DataFrame) colIdx(aIdx, mIdx int) int {
	return aIdx*len(df.metrics) + mIdx
}
```

- [ ] **Step 2: Update colSlice**

```go
func (df *DataFrame) colSlice(aIdx, mIdx int) []float64 {
	return df.columns[df.colIdx(aIdx, mIdx)]
}
```

- [ ] **Step 3: Update Value**

Change from `df.data[off+len(df.times)-1]` to:

```go
col := df.colSlice(aIdx, mIdx)
return col[len(col)-1]
```

- [ ] **Step 4: Update ValueAt**

Change from `df.data[off+tIdx]` to:

```go
return df.columns[df.colIdx(aIdx, mIdx)][tIdx]
```

- [ ] **Step 5: Update Column**

Change to return `df.columns[df.colIdx(aIdx, mIdx)]` directly.

- [ ] **Step 6: Commit**

```
git add data/data_frame.go
git commit -m "refactor: replace colOffset with colIdx for column access"
```

---

### Task 3: Copy, At, and Table methods

**Files:**
- Modify: `data/data_frame.go` -- Copy (~line 431), At (~line 379), Table (~line 466)

- [ ] **Step 1: Update Copy**

Replace single slab copy with per-column deep copy:

```go
func (df *DataFrame) Copy() *DataFrame {
	cols := make([][]float64, len(df.columns))
	for i, col := range df.columns {
		dst := make([]float64, len(col))
		copy(dst, col)
		cols[i] = dst
	}

	times := make([]time.Time, len(df.times))
	copy(times, df.times)

	assets := make([]asset.Asset, len(df.assets))
	copy(assets, df.assets)

	metrics := make([]Metric, len(df.metrics))
	copy(metrics, df.metrics)

	result := mustNewDataFrame(times, assets, metrics, df.freq, cols)
	// copy riskFreeRates, dateKeys as before
}
```

- [ ] **Step 2: Update At**

Build a single-row DataFrame with one-element column slices:

```go
cols := make([][]float64, len(df.columns))
for i, col := range df.columns {
	cols[i] = []float64{col[tIdx]}
}
```

- [ ] **Step 3: Update Table**

Change `df.data[off]` references to `df.columns[df.colIdx(aIdx, mIdx)][tIdx]`.

- [ ] **Step 4: Commit**

```
git add data/data_frame.go
git commit -m "refactor: update Copy, At, Table for per-column storage"
```

---

### Task 4: Time-axis slicing (zero-copy views)

**Files:**
- Modify: `data/data_frame.go` -- sliceByTimeIndices (~line 627), Window (~line 1893)

- [ ] **Step 1: Update sliceByTimeIndices to zero-copy**

Replace data-copying loop with column sub-slicing using three-index expressions:

```go
func (df *DataFrame) sliceByTimeIndices(startIdx, endIdx int) *DataFrame {
	cols := make([][]float64, len(df.columns))
	for i, col := range df.columns {
		cols[i] = col[startIdx:endIdx:endIdx]
	}

	times := df.times[startIdx:endIdx:endIdx]

	assets := make([]asset.Asset, len(df.assets))
	copy(assets, df.assets)

	metrics := make([]Metric, len(df.metrics))
	copy(metrics, df.metrics)

	result := mustNewDataFrame(times, assets, metrics, df.freq, cols)

	if df.riskFreeRates != nil {
		result.riskFreeRates = df.riskFreeRates[startIdx:endIdx:endIdx]
	}

	if df.dateKeys != nil {
		result.dateKeys = df.dateKeys[startIdx:endIdx:endIdx]
	}

	return result
}
```

- [ ] **Step 2: Update Window(nil) to return view instead of Copy**

Change `Window` to create a view via `sliceByTimeIndices(0, len(df.times))` for nil period and full-range cases, instead of calling `Copy()`:

```go
if period == nil {
	return df.sliceByTimeIndices(0, len(df.times))
}
// ...
if !start.After(df.Start()) {
	return df.sliceByTimeIndices(0, len(df.times))
}
```

- [ ] **Step 3: Commit**

```
git add data/data_frame.go
git commit -m "perf: make time-axis slicing zero-copy via column sub-slices"
```

---

### Task 5: Metric/asset axis views and sliceByIndices

**Files:**
- Modify: `data/data_frame.go` -- Metrics (~line 552), Assets (~line 508), sliceByIndices (~line 702)

- [ ] **Step 1: Update Metrics to zero-copy**

Select matching column slices by reference instead of copying data:

```go
cols := make([][]float64, len(df.assets)*newMetricLen)
for aIdx := range df.assets {
	for newMIdx, oldMIdx := range matchedIdx {
		cols[aIdx*newMetricLen+newMIdx] = df.columns[df.colIdx(aIdx, oldMIdx)]
	}
}
```

Copy `times` (shared sub-slice), `assets` (copy), `matched` metrics. Call `propagateAux`.

- [ ] **Step 2: Update Assets to zero-copy**

Same pattern: select column slices for matched assets:

```go
cols := make([][]float64, len(matched)*len(df.metrics))
for newAIdx, oldAIdx := range matchedIdx {
	for mIdx := range df.metrics {
		cols[newAIdx*len(df.metrics)+mIdx] = df.columns[df.colIdx(oldAIdx, mIdx)]
	}
}
```

- [ ] **Step 3: Update sliceByIndices**

Build new columns by gathering non-contiguous indices:

```go
cols := make([][]float64, len(df.columns))
for i, srcCol := range df.columns {
	dst := make([]float64, len(indices))
	for newIdx, oldIdx := range indices {
		dst[newIdx] = srcCol[oldIdx]
	}
	cols[i] = dst
}
```

Gather riskFreeRates, dateKeys per index as before.

- [ ] **Step 4: Commit**

```
git add data/data_frame.go
git commit -m "perf: make Metrics/Assets zero-copy views; update sliceByIndices"
```

---

### Task 6: Filter, Drop, dropNaN, and iteration methods

**Files:**
- Modify: `data/data_frame.go` -- Filter (~line 659), Drop (~line 764), dropNaN (~line 791)
- Modify: `data/data_frame.go` -- MaxAcrossAssets, MinAcrossAssets, CountWhere, IdxMaxAcrossAssets

- [ ] **Step 1: Update Filter**

Build reusable single-row DataFrame with per-column buffers:

```go
rowCols := make([][]float64, len(df.columns))
buffers := make([]float64, len(df.columns))
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

- [ ] **Step 2: Update Drop (non-NaN path)**

The predicate currently iterates `row.data`. Change to iterate `row.columns`:

```go
return df.Filter(func(_ time.Time, row *DataFrame) bool {
	for _, col := range row.columns {
		if col[0] == val {
			return false
		}
	}
	return true
})
```

- [ ] **Step 3: Update dropNaN**

Change from `colSlice` (already correct) -- verify it uses `df.colSlice(aIdx, mIdx)` which now returns `df.columns[idx]`. The `hasNaN` loop iterating `col` values should work unchanged.

- [ ] **Step 4: Update MaxAcrossAssets, MinAcrossAssets, CountWhere, IdxMaxAcrossAssets**

Change all `df.data[df.colOffset(aIdx, mIdx)+tIdx]` to `df.columns[df.colIdx(aIdx, mIdx)][tIdx]`. Update result construction from flat slab to `[][]float64`.

- [ ] **Step 5: Commit**

```
git add data/data_frame.go
git commit -m "refactor: update Filter, Drop, iteration methods for per-column storage"
```

---

### Task 7: Apply, arithmetic, and aggregation methods

**Files:**
- Modify: `data/data_frame.go` -- Apply (~line 1925), elemWiseOp (~line 977), findIntersection (~line 931), broadcastOp (~line 1081), AddScalar (~line 1131), MulScalar (~line 1152), Reduce (~line 2012)

- [ ] **Step 1: Update Apply**

Build new columns by transforming each column individually:

```go
cols := make([][]float64, len(df.columns))
for i, col := range df.columns {
	cols[i] = transform(col)
}
```

- [ ] **Step 2: Update findIntersection and elemWiseOp**

Change `colPair` to store column indices instead of slab offsets. Update `elemWiseOp` to operate per-column:

```go
type colPair struct {
	selfIdx  int
	otherIdx int
}
```

Build result `[][]float64` with one new column per pair.

- [ ] **Step 3: Update broadcastOp**

Replace `result.colOffset(aIdx, rMIdx)` with `result.columns[colIdx]`. Replace `result.data[off:off+timeLen]` with column slice access. Replace `other.colSlice(otherAIdx, mIdx)` call (already returns column slice -- verify it works).

- [ ] **Step 4: Update AddScalar and MulScalar**

Replace `floats.AddConst(scalar, result.data)` with per-column loop:

```go
for _, col := range result.columns {
	floats.AddConst(scalar, col)
}
```

Same for `floats.Scale` in MulScalar.

- [ ] **Step 5: Update Reduce**

Build result `[][]float64` with one single-element column per (asset, metric):

```go
cols := make([][]float64, len(df.columns))
for i, col := range df.columns {
	cols[i] = []float64{reducer(col)}
}
```

- [ ] **Step 6: Update Covariance and Correlation**

These four methods (`crossMetricCovariance`, `crossAssetCovariance`, `crossMetricCorrelation`, `crossAssetCorrelation`) use `colSlice()` for reading (already updated) and build result DataFrames with `append(pairData, value)`. Change from flat `pairData []float64` to `pairCols [][]float64`:

```go
// Before (crossMetricCovariance):
pairData = append(pairData, sampleCov(df.colSlice(aIdx, i), df.colSlice(aIdx, j)))
// ...
return mustNewDataFrame(lastTime, []asset.Asset{targetAsset}, pairMetrics, df.freq, pairData)

// After:
pairCols = append(pairCols, []float64{sampleCov(df.colSlice(aIdx, i), df.colSlice(aIdx, j))})
// ...
return mustNewDataFrame(lastTime, []asset.Asset{targetAsset}, pairMetrics, df.freq, pairCols)
```

Same pattern for all four methods. The column ordering is preserved because `append` order matches the original.

**Methods that require no changes (transitive correctness):**
- `RenameMetric` -- delegates to `Copy()`, works correctly after Copy is updated
- `Last` -- delegates to `At()`, works correctly after At is updated
- `Annotate` -- uses `ValueAt()`, works correctly after ValueAt is updated
- `SubScalar`/`DivScalar` -- delegate to `AddScalar`/`MulScalar`
- `Mean`/`Sum`/`Max`/`Min`/`Variance`/`Std` -- delegate to `Reduce`
- `Pct`/`RiskAdjustedPct`/`Diff`/`Log`/`CumSum`/`CumMax`/`Shift` -- delegate to `Apply`

**Note on `propagateAux`:** This method shares `riskFreeRates` and `dateKeys` by reference. This is safe for same-time-axis operations (Metrics, Assets, Apply) because the time axis is identical. For time-sliced views, `sliceByTimeIndices` handles sub-slicing directly (not through propagateAux). This is the existing behavior, preserved by the refactor.

- [ ] **Step 7: Commit**

```
git add data/data_frame.go
git commit -m "refactor: update Apply, arithmetic, aggregation for per-column storage"
```

---

### Task 8: Mutation methods (AppendRow, Insert)

**Files:**
- Modify: `data/data_frame.go` -- AppendRow (~line 1965), Insert (~line 860)

- [ ] **Step 1: Update AppendRow**

Replace full slab rebuild with per-column append:

```go
func (df *DataFrame) AppendRow(timestamp time.Time, values []float64) error {
	colCount := len(df.assets) * len(df.metrics)
	if len(values) != colCount {
		return fmt.Errorf("AppendRow: values length %d does not match column count %d", len(values), colCount)
	}

	if len(df.times) > 0 && !timestamp.After(df.End()) {
		return fmt.Errorf("AppendRow: timestamp %s is not after current End() %s",
			timestamp.Format(time.RFC3339), df.End().Format(time.RFC3339))
	}

	for i, val := range values {
		df.columns[i] = append(df.columns[i], val)
	}

	df.times = append(df.times, timestamp)

	if df.dateKeys != nil {
		df.dateKeys = append(df.dateKeys, dateKey(timestamp))
	}

	df.riskFreeRates = nil

	return nil
}
```

- [ ] **Step 2: Update Insert**

Currently rebuilds the entire slab when adding a new column because column offsets shift when metrics or assets are added. With per-column storage, adding a metric still changes the `colIdx` formula (`aIdx*len(metrics)+mIdx`) because `len(metrics)` increases.

When the metric set or asset set changes, rebuild the `columns` slice to maintain the `aIdx*M+mIdx` invariant:

```go
if !assetExists || !metricExists {
	oldAssetLen, oldMetricLen := assetLen, metricLen
	if !assetExists {
		oldAssetLen = assetLen - 1
	}
	if !metricExists {
		oldMetricLen = metricLen - 1
	}

	newCols := make([][]float64, assetLen*metricLen)
	// Initialize all columns as zero-filled
	for i := range newCols {
		newCols[i] = make([]float64, len(df.times))
	}
	// Copy existing data into the correct new positions
	for oldAIdx := range oldAssetLen {
		for oldMIdx := range oldMetricLen {
			oldIdx := oldAIdx*oldMetricLen + oldMIdx
			newIdx := oldAIdx*metricLen + oldMIdx
			newCols[newIdx] = df.columns[oldIdx]
		}
	}
	df.columns = newCols
}

// Write the values into the correct column.
aIdx := df.assetIndex[targetAsset.CompositeFigi]
mIdx, _ := df.metricIndex(metric)
df.columns[aIdx*metricLen+mIdx] = values
```

This preserves the `colIdx` invariant after dimension changes. Existing columns are moved (not copied) to their new positions via slice reference assignment.

- [ ] **Step 3: Commit**

```
git add data/data_frame.go
git commit -m "refactor: simplify AppendRow and Insert for per-column storage"
```

---

### Task 9: Satellite types (downsample, upsample, merge, snapshot_recorder)

**Files:**
- Modify: `data/downsample.go:36-82`
- Modify: `data/upsample.go:69-219`
- Modify: `data/merge.go:30-151`
- Modify: `data/snapshot_recorder.go:312-479`

- [ ] **Step 1: Update DownsampledDataFrame.aggregate**

Read side: change `d.df.data[srcOff+timeGroup.start : srcOff+timeGroup.end]` to `d.df.columns[colIdx][timeGroup.start:timeGroup.end]` where `colIdx = aIdx*metricLen + mIdx`.

Write side: build `[][]float64` output instead of flat `newData` slab.

- [ ] **Step 2: Update UpsampledDataFrame (ForwardFill, BackFill, Interpolate)**

Read side: already uses `u.df.colSlice(aIdx, mIdx)` which is updated.

Write side: **all three methods** (`ForwardFill`, `BackFill`, `Interpolate`) have independent write-side code that computes `dstOff := (aIdx*metricLen + mIdx) * len(newTimes)` and writes `newData[dstOff+idx]`. Each must be converted separately to build per-column `[]float64` slices. The fill logic differs per method, so each needs individual attention.

- [ ] **Step 3: Update merge.go**

`MergeColumns`: update to work with per-column storage. Copy column slices from source DataFrames.

`MergeTimes`: build `[][]float64` by concatenating corresponding columns from each frame:

```go
numCols := len(assets) * len(metrics)
cols := make([][]float64, numCols)
for i := range cols {
	cols[i] = make([]float64, 0, totalLen)
}

for _, f := range sorted {
	for aIdx, a := range assets {
		for mIdx, m := range metrics {
			col := f.Column(a, m)
			if col == nil {
				// fill with zeros for this frame's time range
				cols[aIdx*len(metrics)+mIdx] = append(cols[aIdx*len(metrics)+mIdx],
					make([]float64, len(f.times))...)
			} else {
				cols[aIdx*len(metrics)+mIdx] = append(cols[aIdx*len(metrics)+mIdx], col...)
			}
		}
	}
}

result, err := NewDataFrame(allTimes, assets, metrics, sorted[0].freq, cols)
```

- [ ] **Step 4: Update snapshot_recorder.go**

Three locations access `df.data[colStart+timeIdx]` with `colStart := (assetIdx*numMetrics + mi) * numTimes`. Change all to `df.columns[assetIdx*numMetrics+mi][timeIdx]`.

- [ ] **Step 5: Update example_data.go**

Change flat slab construction to `[][]float64` in `ExampleData()`.

- [ ] **Step 6: Verify data package compiles**

Run: `go build ./data/...`
Expected: Clean compilation (no errors)

- [ ] **Step 7: Commit**

```
git add data/
git commit -m "refactor: update satellite types for per-column DataFrame storage"
```

---

### Task 10: External callers (engine, portfolio, signal)

**Files:**
- Modify: `engine/engine.go:273,531,584`
- Modify: `engine/backtest.go:561`
- Modify: `engine/live.go:429`
- Modify: `portfolio/account.go:1647`
- Modify: `portfolio/sqlite.go:645`
- Modify: `signal/earnings_yield.go:64`
- Modify: `data/pvdata_provider.go:271,326`
- Modify: `data/snapshot_provider.go:255,300`

- [ ] **Step 1: Update engine callers**

`engine.go:273,531`: Empty DataFrames -- change `nil` data to `nil` columns:
```go
return data.NewDataFrame(nil, nil, nil, data.Daily, nil)
```
(This should still work since `len(nil) == 0` for both `[]float64` and `[][]float64`)

`engine.go:584`: Main assembly -- use `SlabToColumns`:
```go
assembled, err := data.NewDataFrame(unionTimes, assets, metrics, data.Daily,
	data.SlabToColumns(slab, len(assets)*len(metrics), len(unionTimes)))
```

`backtest.go:561` and `live.go:429`: Cash-only frames with nil data -- same nil pattern.

- [ ] **Step 2: Update portfolio callers**

`account.go:1647`: Single-row frame:
```go
row, err := data.NewDataFrame(timestamps, assets, metrics, data.Daily,
	[][]float64{{total}, {benchVal}, {rfVal}})
```

`sqlite.go:645`: Use `SlabToColumns`:
```go
df, err := data.NewDataFrame(times, assets, metrics, perfFreq,
	data.SlabToColumns(vals, len(assets)*len(metrics), len(times)))
```

- [ ] **Step 3: Update signal callers**

`earnings_yield.go:64`: Use `SlabToColumns` or build columns directly.

- [ ] **Step 4: Update data package providers**

`pvdata_provider.go:271`: Empty frame -- nil pattern.
`pvdata_provider.go:326`: Use `SlabToColumns`.
`snapshot_provider.go:255`: Empty frame -- nil pattern.
`snapshot_provider.go:300`: Use `SlabToColumns`.

- [ ] **Step 5: Verify each package compiles**

Run after each step to catch errors early:
```
go build ./engine/...
go build ./portfolio/...
go build ./signal/...
go build ./data/...
go build ./...
```
Expected: Clean compilation for all

- [ ] **Step 6: Commit**

```
git add engine/ portfolio/ signal/ data/pvdata_provider.go data/snapshot_provider.go
git commit -m "refactor: update all external NewDataFrame callers for column slices"
```

---

### Task 11: Test migration

**Files:**
- Modify: `data/data_frame_test.go` (largest -- 325+ NewDataFrame calls)
- Modify: `data/merge_test.go`
- Modify: `data/snapshot_provider_test.go`
- Modify: `data/snapshot_recorder_test.go`
- Modify: `data/period_test.go`
- Modify: `data/test_provider_test.go`
- Modify: `engine/*_test.go`
- Modify: `portfolio/*_test.go`
- Modify: `signal/*_test.go`
- Modify: `risk/*_test.go`
- Modify: `tax/*_test.go`
- Modify: `universe/*_test.go`
- Modify: `report/*_test.go`

Strategy: Use `SlabToColumns` for bulk migration of test calls, then optimize hot spots to build `[][]float64` directly where it improves readability. There are 325+ test `NewDataFrame` calls across all packages.

- [ ] **Step 1: Update data package tests**

This is the largest batch. For each `NewDataFrame(..., []float64{...})` call, convert to `NewDataFrame(..., [][]float64{{...}, {...}})`. For tests with many columns, use `SlabToColumns`.

Run: `go test ./data/... -count=1`
Expected: All tests pass (except copy-semantics tests that need updating -- see Task 12)

- [ ] **Step 2: Update engine test files**

Grep for `data.NewDataFrame` in `engine/*_test.go` and update each call.

Run: `go test ./engine/... -count=1`
Expected: All tests pass

- [ ] **Step 3: Update portfolio test files**

Grep for `data.NewDataFrame` in `portfolio/*_test.go` and update each call.

Run: `go test ./portfolio/... -count=1`
Expected: All tests pass

- [ ] **Step 4: Update remaining test files**

Update `signal/`, `risk/`, `tax/`, `universe/`, `report/`, `cli/` test files.

Run: `go test ./... -count=1`
Expected: All tests pass

- [ ] **Step 5: Run lint**

Run: `golangci-lint run ./...`
Expected: No new lint issues

- [ ] **Step 6: Commit**

```
git add -A
git commit -m "test: migrate all test NewDataFrame calls to column-slice API"
```

---

### Task 12: Update copy-semantics tests and add view tests

**Files:**
- Modify: `data/data_frame_test.go`

- [ ] **Step 1: Update "Between produces independent copy" test**

This test currently verifies that modifying the Between result doesn't affect the original. With views, column data IS shared. Update to verify:
- Structural independence (different time range)
- Column data sharing (mutating via Column() on view is visible in parent)

- [ ] **Step 2: Update "Assets/Metrics produces independent copy" tests**

Same pattern: verify structural independence + column data sharing.

- [ ] **Step 3: Add view aliasing tests**

New test: "Between view shares column data with parent":
```go
It("Between view shares column data with parent", func() {
	// Create DataFrame, take Between view, mutate via Column()
	// Verify parent sees the mutation
})
```

New test: "Metrics view shares column data with parent":
```go
It("Metrics view shares column data with parent", func() {
	// Create DataFrame, take Metrics view, mutate via Column()
	// Verify parent sees the mutation
})
```

- [ ] **Step 4: Add AppendRow isolation test for views**

```go
It("AppendRow does not affect views created via Between", func() {
	// Create DataFrame, take Between view
	// AppendRow on parent
	// Verify view is unaffected (length, values)
})
```

- [ ] **Step 5: Add Insert isolation test for views**

```go
It("Insert does not affect views", func() {
	// Create DataFrame, take Metrics view
	// Insert new column on parent
	// Verify view is unaffected
})
```

- [ ] **Step 6: Verify "does not affect prior Window snapshots" still passes**

This test now exercises the view path instead of Copy. Run it explicitly:

Run: `go test ./data/... -count=1 --ginkgo.focus="does not affect prior Window snapshots"`
Expected: PASS

- [ ] **Step 7: Run full test suite**

Run: `go test ./... -count=1 -timeout 300s`
Expected: All tests pass

- [ ] **Step 8: Run lint**

Run: `golangci-lint run ./...`
Expected: No issues

- [ ] **Step 9: Commit**

```
git add data/data_frame_test.go
git commit -m "test: update copy-semantics tests for view behavior; add view aliasing tests"
```

---

### Task 13: Final verification and cleanup

**Files:**
- All files touched in previous tasks

- [ ] **Step 1: Verify no remaining references to old API**

Search for any remaining `df.data`, `colOffset`, or flat-slab `NewDataFrame` patterns:

Run: `grep -rn '\.data\[' data/data_frame.go data/downsample.go data/upsample.go data/merge.go data/snapshot_recorder.go`
Expected: No matches

Run: `grep -rn 'colOffset' data/`
Expected: No matches

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -count=1 -timeout 300s`
Expected: All tests pass

- [ ] **Step 3: Run lint**

Run: `golangci-lint run ./...`
Expected: No issues

- [ ] **Step 4: Commit any cleanup**

If any cleanup was needed, commit it:
```
git commit -m "chore: remove remaining flat-slab references"
```
