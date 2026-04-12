# Fetch Hot-Path Optimization

## Goal

Reduce wall-clock time for long-horizon backtests by cutting CPU overhead in
the per-day `FetchAt` path and reducing bytes transferred from Postgres for
sparse-access metric queries.

Baseline: `ncave` backtest, 2010-01-01 through 2026-01-01.

- Wall: 33.43s
- On-CPU: 14.15s (42.3% of wall)
- Off-CPU / I/O wait: ~19s (dominated by Postgres reads)

Target: ~20-25% wall-time improvement (26-27s) without public API changes.

## Profile Summary

Dominant costs from `cpu.prof`:

| Cost                                     | Flat  | Cum    | Source                               |
| ---------------------------------------- | ----- | ------ | ------------------------------------ |
| `syscall.rawsyscalln` (pgx reads)        | 6.05s | 6.05s  | `data.(*PVDataProvider).Fetch`       |
| `runtime.mapaccess2_fast64`              | 1.40s | ~1.76s | `engine.fetchRange` reassembly       |
| `runtime.mapassign_fast64`               | 0.82s | ~1.12s | `engine.fetchRange` reassembly       |
| `fetchRange` body                        | 0.41s | 10.04s | cache-hit point-in-time rebuilds    |
| `ncave.Compute` fundamentals FetchAt     | -     | 6.38s  | 16 annual scans of ~3 000 assets     |

Two distinct problems:

1. **Per-day reassembly on cache hits.** Every `FetchAt` rebuilds a union
   time axis across cached year chunks, allocates and NaN-fills a slab,
   scatters values, then slices down to the requested day via `Between()`.
   For 30 held assets x 7 metrics x 2 000+ trading days, the map dedup and
   scatter dominate on-CPU time even though no DB query fires.
2. **Over-fetching for sparse-access strategies.** `ncave.Compute` issues
   `FetchAt` for `MarketCap` + `WorkingCapital` across ~3 000 assets at
   March 31 of each year. The year-chunk cache forces a full year of daily
   rows from the `metrics` table (~756k rows/year x 16 years = 12M rows),
   and `fetchMetrics` `SELECT`s all 11 metric columns even when only one
   was requested.

## Non-Goals

- Changing cache chunk granularity or policy.
- New strategy-author API hints for access patterns.
- Parallelizing `fetchEod` / `fetchMetrics` / `fetchFundamentals` calls.
- GC tuning, slab pooling, or any `sync.Pool` work.
- Public CLI, `Strategy`, `Portfolio`, `DataFrame`, or provider API changes.

These are all deliberately reserved as a potential phase 2.

## Design

Three independent changes, in order of ROI.

### 1. Point-in-time fast path in `fetchRange`

`engine.fetchRange` is the shared implementation behind `Fetch` and
`FetchAt`. When called from `FetchAt`, `rangeStart` and `rangeEnd` are
equal. In that branch we can skip the union-axis / slab / scatter loop
entirely.

**Current behavior (engine/engine.go:387-672)**: for every call, whether
point-in-time or range, `fetchRange`:

1. Identifies cache misses and bulk-fetches them by year chunk.
2. Walks `(year, asset, metric)` triples to collect `timeSet[unix] = t`.
3. Sorts `unionTimes`, builds `timeIdx[unix] = i`.
4. Allocates a dense slab `numTimes * numAssets * numMetrics` and NaN-fills.
5. Re-walks cache entries and scatters values into the slab via `timeIdx`.
6. Forward-fills fundamental columns.
7. Constructs a `DataFrame` and calls `Between(rangeStart, rangeEnd)`.

**New behavior for point-in-time queries**: after the miss-fetch phase
(steps 1 and 6 stay), skip straight to a direct assembly:

```
if rangeStart.Equal(rangeEnd) {
    return e.assemblePointInTime(assets, metrics, rangeEnd)
}
```

`assemblePointInTime` MUST match the slab-path semantics exactly. Both
paths consider only the chunks returned by `chunkYears(rangeEnd,
rangeEnd)`, which for point-in-time is always the single calendar year
containing `rangeEnd`. No cross-year forward-fill is introduced.

- `hasFundamental` is computed the same way as in the slab path.
- For each `(asset, metric)`:
  - Look up the single `colCacheKey{figi, metric, chunkStart}` for the
    year containing `rangeEnd`. Continue if not present (matches the
    cache-miss-sentinel behavior in the slab path).
  - For non-fundamental metrics, binary-search `entry.times` for an entry
    whose `Unix()` equals `rangeEnd.Unix()`. If found, write
    `entry.values[i]` into the row. Otherwise leave it as `NaN`. This
    matches what the slab path produces for the FetchAt day after
    `Between(rangeEnd, rangeEnd)` trims the union frame.
  - For fundamental metrics, binary-search `entry.times` for the greatest
    index `i` with `entry.times[i].Before(rangeEnd) || equal`. If found,
    write `entry.values[i]` (the forward-filled value). Otherwise leave
    it as `NaN`. This matches the slab path's within-chunk forward-fill
    which operates only within the current year's union axis.
- Return-shape selection, matching slab-path behavior:
  - If `hasFundamental` and `rangeStart == rangeEnd`, return a one-row
    frame anchored at `rangeEnd` (even if all columns are NaN). This
    mirrors the `timeSet[rangeEnd.Unix()] = rangeEnd` injection at
    `engine.go:581-583`.
  - Otherwise, return a one-row frame only if any entry.times in the year
    chunks contains a time whose `Unix()` equals `rangeEnd.Unix()`.
    Otherwise return an empty frame (`Len() == 0`) with the requested
    asset/metric schema. This mirrors what `Between(rangeEnd, rangeEnd)`
    produces when the union axis contains only non-matching times.

Note: the miss-fetch path in steps 1 and 6 is unchanged, so cache
population, provider routing, and forward-fill semantics remain identical;
only the *assembly* of the result is bypassed.

**Data-layer constraints.** The fast path constructs a one-row `DataFrame`
using `data.NewDataFrame` with a single-timestamp slice. `NewDataFrame`
already accepts that shape. No new exported constructor is needed.

**Risks.** Behavioral divergence between slab-assembly and fast-path
results. Mitigation: existing Ginkgo tests in `engine/fetch_test.go`
already cover `FetchAt`, `FetchAt cache`, forward-fill on non-trading
days, past-date forward-fill, and future-date rejection. We'll add
targeted cases for:

- `FetchAt` on a trading day where no value exists (empty asset row).
- `FetchAt` on a non-trading day where the latest fundamental value comes
  from the previous year's chunk.
- `FetchAt` for mixed fundamental + daily metrics in the same call.

### 2. Column-trimmed query in `fetchMetrics`

**Current behavior (data/pvdata_provider.go:544-643)**: the SQL is fixed:

```sql
SELECT composite_figi, event_date,
       market_cap, ev, pe, pb, ps, ev_ebit, ev_ebitda,
       pe_forward, peg, price_to_cash_flow, beta
FROM metrics
WHERE composite_figi = ANY($1) AND event_date BETWEEN $2 AND $3
ORDER BY event_date
```

Every call transfers all 11 float/int columns regardless of what the
caller asked for. The row scan loop also allocates `scanArgs := make([]any,
0, 2+len(columns))` once per row.

**New behavior**: build a per-call SELECT from the requested metrics,
mirroring how `fetchFundamentals` uses `metricColumn` to generate its
column list.

- Introduce a `metricsColumn` map (or extend the existing `columns` table)
  that gives the SQL column name *and* int-vs-float classification for
  each metric served by the `metrics` view.
- At the start of `fetchMetrics`, build `sqlCols` and a parallel
  `metricOrder` slice from the caller's `metrics` argument, in request
  order.
- Emit the SELECT as `fmt.Sprintf` with `strings.Join(sqlCols, ", ")`.
- Hoist `scanArgs`, `intVals`, `floatVals` out of the row loop. Reset
  them in-place each iteration.

For ncave's `MarketCap`-only fetch, this drops each row from 13 values
(`figi`, `event_date`, 11 metric columns) to 3. Bytes transferred per row
drop ~4x and pgx scan work drops proportionally.

**Risks.** An unknown metric in `req.Metrics` that belongs to the
`metrics` view must still error out rather than silently dropping.
`fetchMetrics` is only reachable from `PVDataProvider.Fetch`, which groups
by view via `metricView`; any metric with `metricView[m] == "metrics"`
that isn't in the new column map indicates a registration bug and must
panic or return an error.

**Tests.** `TestPVDataProvider` already exercises `fetchMetrics`. Add
cases for:

- `Fetch` with only `MarketCap` (verify the SQL actually trims columns:
  check via a sqlmock or by asserting the returned DataFrame has only the
  `MarketCap` column populated).
- `Fetch` with a mix of `metrics`-view metrics.
- `Fetch` with a metric that is NOT registered (expect an explicit error).

### 3. Per-row allocation hoist in `fetchFundamentals`

**Current behavior (data/pvdata_provider.go:688-720)**:

```go
for rows.Next() {
    var figi string
    var eventDate, dateKey time.Time

    vals := make([]any, len(sqlCols)+3)          // allocated per row
    vals[0] = &figi
    vals[1] = &eventDate
    vals[2] = &dateKey

    floatVals := make([]*float64, len(sqlCols))  // allocated per row
    for idx := range sqlCols {
        vals[idx+3] = &floatVals[idx]
    }
    ...
}
```

**New behavior**: move both `vals` and `floatVals` allocation outside the
loop, reset per row.

```go
vals := make([]any, len(sqlCols)+3)
floatVals := make([]*float64, len(sqlCols))
// wire pointers once

for rows.Next() {
    // reset floatVals to nil (so SQL NULLs remain nil)
    for idx := range floatVals {
        floatVals[idx] = nil
    }
    ...
}
```

Small diff, trivially correct, matches the already-hoisted pattern in
`fetchMetrics`. Measurable only under the fundamentals-heavy ncave path.

## Interactions

- Change 1 must *not* bypass the miss-fetch phase: the fast path still
  runs the cache-population loop before assembly, so provider calls,
  routing, and error paths are unchanged.
- Change 1 and the existing slab path stay in the same function so that
  `Fetch` (range queries) continues to exercise the slab assembly. Only
  point-in-time queries take the fast path.
- Changes 2 and 3 are independent of Change 1 and of each other; they can
  land in any order. Change 2 requires a new metric-to-SQL-column mapping
  that is analogous to `metricColumn` but for the `metrics` view.

## Testing

- Run `make test` after each change; Ginkgo suites cover the affected
  packages (`engine`, `data`).
- New Ginkgo cases added to `engine/fetch_test.go` and
  `data/pvdata_provider*_test.go` as listed above.
- Re-run the ncave backtest with CPU profiling and compare cumulative
  times in `fetchRange`, `fetchMetrics`, and `mapaccess2_fast64` against
  baseline.

## Expected Impact

- **Change 1**: ~4s on-CPU reduction, ~2-3s wall reduction. Eliminates
  ~2.9s of map ops + ~1s slab/NaN overhead per day across ~12k FetchAt
  calls.
- **Change 2**: ~3-4s wall reduction on the ncave-style annual scan path
  by trimming bytes per row ~4x and cutting pgx row-parse CPU
  proportionally.
- **Change 3**: ~100-200ms, below the noise floor but free.

Total expected wall-time: 33.4s -> ~26-27s (20-25% faster).

## Out of Scope / Phase 2 Candidates

If the above does not meet the target, consider:

- Cache policy changes so that sparse-access strategies can issue narrow
  date-filtered queries instead of pulling whole year chunks.
- Parallelizing the three per-view provider calls inside
  `PVDataProvider.Fetch`.
- Using `sync.Pool` for the slab buffer in the range-query (`Fetch`) path
  if GC pressure remains visible.
- Widening cache chunks (multi-year) for metrics the engine observes
  being accessed sparsely.
