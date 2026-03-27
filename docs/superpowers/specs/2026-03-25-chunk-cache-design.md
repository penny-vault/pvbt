# Chunk-Level Cache Redesign

## Problem

The per-column cache in `fetchRange` uses `map[colCacheKey]*colCacheEntry` keyed by `(figi, metric, chunkYear)`. For 50 assets and 7 metrics, every `fetchRange` call does ~350 map lookups for miss detection, ~350 more for assembly, builds a union time set map, sorts it, builds a time index map, allocates a NaN slab, scatters values, and constructs a new DataFrame. The profile shows 29% of `fetchRange` CPU on map operations and 12% total CPU on GC from short-lived maps, slabs, and DataFrames created on every call.

## Design

### Chunk-level cache with calendar-day-indexed slabs

Replace `map[colCacheKey]*colCacheEntry` with `map[int64]*chunkEntry` where the int64 key is the year chunk start (Unix seconds of Jan 1 00:00 Eastern). Each chunk holds a flat `[]float64` slab indexed by pure arithmetic:

```go
type chunkEntry struct {
    baseUnix  int64            // Jan 1 00:00 Eastern, Unix seconds
    assets    []asset.Asset    // ordered asset list
    metrics   []data.Metric    // ordered metric list
    assetIdx  map[string]int   // figi -> index into assets (built once per expansion)
    metricIdx map[data.Metric]int // metric -> index into metrics (built once per expansion)
    values    []float64        // [dayOffset * numAssets * numMetrics + aIdx * numMetrics + mIdx]
    bytes     int64            // estimated memory footprint
}
```

Slab dimensions: 366 days x len(assets) x len(metrics). Always 366 slots regardless of leap year -- wastes one slot in non-leap years but avoids branching. Non-trading days and missing data are NaN.

Lookup: `values[dayOffset*len(assets)*len(metrics) + aIdx*len(metrics) + mIdx]` where `dayOffset = (timestamp - baseUnix) / 86400`. No maps on the hot read path.

### Miss detection

One map lookup per year to get the chunk entry. Then check `assetIdx` and `metricIdx` for each requested (asset, metric) pair. These are small maps (50 assets, 7 metrics) checked only during miss detection, not on every value read.

### Fetching misses

Same as today: one provider call per year chunk with the missing assets and metrics. When the chunk doesn't exist, create it with the fetched dimensions. When the chunk exists but needs new assets or metrics, allocate a new slab with expanded dimensions, scatter old values into the new layout, then scatter the fetched values. This reallocation only happens a few times during the first backtest step as different callers request different metric sets, then never again.

### Assembly

For each year in the requested range:
1. Compute day offsets for `rangeStart` and `rangeEnd`
2. Walk the day range, skip days where all requested columns are NaN (weekends/holidays)
3. For each trading day, read values directly from the slab via arithmetic indexing
4. Build the result DataFrame from the collected trading days and values

For single-year requests (the common case), this is a single linear scan with no maps, no sorting, no intermediate allocations. For multi-year requests, concatenate the results.

### Eviction

Same policy as today: evict chunks older than `currentYear - 1`. One `delete` call per evicted year instead of iterating all column entries.

### Size tracking

`366 * len(assets) * len(metrics) * 8` for the values slab. Updated on expansion.

## Files Changed

| File | Change |
|------|--------|
| `engine/data_cache.go` | Replace `colCacheKey`/`colCacheEntry` with `chunkEntry`, rewrite `get`/`put`/`evictBefore`, add slab indexing methods |
| `engine/engine.go` | Rewrite `fetchRange` miss detection and assembly to use chunk slabs |
| `engine/data_cache_test.go` | Update cache tests for new structure |

## What Does Not Change

- `fetchFromProviders` -- still called with missing assets/metrics per year
- Callers of `fetchRange` (`Fetch`, `FetchAt`, `Prices`) -- same interface and return type
- The public API -- entirely internal change
- Provider interface -- unchanged
