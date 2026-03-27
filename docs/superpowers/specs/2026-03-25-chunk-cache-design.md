# Chunk-Level Cache Redesign

## Problem

The per-column cache in `fetchRange` uses `map[colCacheKey]*colCacheEntry` keyed by `(figi, metric, chunkYear)`. For 50 assets and 7 metrics, every `fetchRange` call does ~350 map lookups just for miss detection, then another ~350 for assembly -- plus building a union time set, sorting it, allocating a NaN slab, scattering values, and constructing a new DataFrame. The profile shows 29% of `fetchRange` CPU on map operations and 12% total CPU on GC from short-lived maps, slabs, and DataFrames created on every call.

## Design

### Replace per-column cache with per-chunk cache

Replace `map[colCacheKey]*colCacheEntry` with `map[int64]*chunkEntry` where the int64 key is the year chunk start (Unix seconds). Each `chunkEntry` holds:

```go
type chunkEntry struct {
    df       *data.DataFrame  // full year of data for all fetched assets/metrics
    columns  map[chunkCol]bool // tracks which (figi, metric) pairs are present
    bytes    int64             // estimated memory footprint
}

type chunkCol struct {
    figi   string
    metric data.Metric
}
```

### Miss detection

One map lookup per year instead of N*M. For each year, check the chunk's `columns` set for each requested (asset, metric) pair. If all are present, no fetch needed. If any are missing, collect the missing assets and metrics for that year.

### Fetching misses

Same as today: one provider call per year chunk with the missing assets and metrics. The returned DataFrame gets merged into the existing chunk entry using `DataFrame.Merge`. The chunk's `columns` set expands to include the newly fetched pairs.

When the chunk is new (no entry exists), the provider result becomes the chunk entry directly.

### Assembly

Instead of scattering cached columns into a NaN slab:

1. For each year, get the chunk's DataFrame
2. Select only the requested assets and metrics from it (if the chunk has more than requested)
3. Call `Between(rangeStart, rangeEnd)` to slice to the requested date range
4. For single-year requests (the common case), return directly -- no merge needed
5. For multi-year requests, merge the year DataFrames

### DataFrame.Merge

If a `Merge` method doesn't exist on DataFrame, add one. It combines two DataFrames that may have different assets, metrics, and overlapping or adjacent time ranges into one. For this use case, the DataFrames have non-overlapping time ranges (different years) so the merge is a concatenation along the time axis.

### DataFrame.Select

If a `Select` method doesn't exist on DataFrame, add one. It returns a view or copy containing only the specified assets and metrics. This avoids returning the entire chunk when the caller only needs a subset.

### Eviction

Same policy as today: evict chunks older than `currentYear - 1`. But now it's one `delete` per evicted year instead of iterating all column entries.

### Size tracking

Same approach: estimate bytes from the chunk DataFrame's dimensions. `len(times) * len(assets) * len(metrics) * 8` for values plus `len(times) * 24` for timestamps.

## Files Changed

| File | Change |
|------|--------|
| `engine/data_cache.go` | Replace `colCacheKey`/`colCacheEntry` with `chunkEntry`/`chunkCol`, rewrite `get`/`put`/`evictBefore` |
| `engine/engine.go` | Rewrite `fetchRange` miss detection, fetch, and assembly sections |
| `data/data_frame.go` | Add `Merge` and `Select` methods if they don't exist |
| `engine/data_cache_test.go` | Update cache tests for new structure |

## What Does Not Change

- `fetchFromProviders` -- still called with missing assets/metrics per year
- Callers of `fetchRange` (`Fetch`, `FetchAt`, `Prices`) -- same interface
- The public API -- entirely internal change
- Provider interface -- unchanged
