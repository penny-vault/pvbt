# Point-in-Time Fundamental Data Access

## Problem

The engine's fundamental data queries use `event_date BETWEEN $start AND $end` where `event_date` is about to become the SEC filing date (AR dimensions) or the fiscal period end (MR dimensions). Two bugs exist:

1. **Look-ahead bias.** Before the column swap, `event_date` was the normalized calendar quarter boundary, not the filing date. The query returned data on the quarter boundary date even though the filing hadn't happened yet. For example, AAPL Q2 2024 data appeared at June 30 but wasn't filed until August 2.

2. **Weekend alignment failure.** `FetchAt` passes `(timestamp, timestamp)` to `fetchRange`, which trims with `Between(timestamp, timestamp)`. Fundamental filing dates are sparse -- they rarely land on the simulation date. For daily price data this works because there's a row on every trading day. For quarterly fundamentals, exact-match almost never succeeds.

After the ongoing column swap (`event_date` becomes the filing date, `date_key` becomes the normalized quarter boundary), problem 1 is resolved at the data level. Problem 2 remains: `FetchAt` on a simulation date won't match a filing date, and `Fetch` over a date range returns fundamentals only on their filing dates with NaN gaps everywhere else.

Additionally, strategies cannot configure which fundamental dimension (ARQ, MRQ, etc.) to use. The dimension is hardcoded to `"ARQ"` in the provider.

## Design

### 1. Provider: add date_key to SELECT

In `PVDataProvider.fetchFundamentals` (`data/pvdata_provider.go`), add `date_key` to the SELECT clause:

```sql
SELECT composite_figi, event_date, date_key, {metric columns}
FROM fundamentals
WHERE composite_figi = ANY($1)
  AND event_date BETWEEN $2::date AND $3::date
  AND dimension = $4
ORDER BY event_date
```

The WHERE clause stays on `event_date` (filing date post-swap). The `date_key` (normalized quarter boundary post-swap) is read but not used for filtering. It is stored alongside the metric values in the cache so downstream consumers can access it in the future.

The `date_key` value is stored as a new `DateKey` field on a per-row basis in the cache entry. The exact representation is an implementation detail, but one approach is a parallel `[]time.Time` in `colCacheEntry` or a separate metadata structure keyed by `(figi, event_date_unix)`.

### 2. Engine: forward-fill fundamentals in fetchRange

In `Engine.fetchRange` (`engine/engine.go`), after scattering cached values into the NaN-filled slab (around line 590) and before the `Between` trim (line 598):

1. Identify which metrics are fundamentals. Use the existing `metricView` map: any metric where `metricView[metric] == "fundamentals"` is a fundamental metric. This classification needs to be accessible from the engine layer. Add a `data.IsFundamental(metric)` function that checks `metricView`.

2. For each fundamental metric column in the slab, forward-fill NaN gaps. Walk the column chronologically; when a NaN is encountered, replace it with the most recent non-NaN value. This implements step-function semantics: once a filing is public, its values are current until the next filing.

3. The `Between(rangeStart, rangeEnd)` trim then works correctly for both `Fetch` (range) and `FetchAt` (single date). Fundamental values are dense on the daily grid, so `Between(timestamp, timestamp)` finds a value.

Forward-fill only runs on fundamental columns. Price and derived metric columns are left unchanged.

### 3. Engine: SetFundamentalDimension

Add a method on `*Engine`:

```go
func (e *Engine) SetFundamentalDimension(dim string)
```

Valid values: `"ARQ"`, `"ARY"`, `"ART"`, `"MRQ"`, `"MRY"`, `"MRT"`. The engine validates the value during initialization (in `buildProviderRouting` or the run loop setup). Invalid values produce an error before the backtest starts.

The engine passes the dimension to the provider. Since the provider is created before the engine in the CLI commands, the engine calls a method on the provider to update the dimension, or the engine re-creates the provider with the new dimension. The simplest path: add a `SetDimension(dim string)` method on `PVDataProvider` that updates `p.dimension`.

Strategy authors call this in `Setup`:

```go
func (s *MyStrategy) Setup(eng *engine.Engine) {
    eng.SetFundamentalDimension("ARQ") // default; explicit for clarity
}
```

If not called, defaults to `"ARQ"`.

This is a public API addition (strategy author surface).

### 4. Snapshot schema

Update `CreateSnapshotSchema` in `data/snapshot_schema.go` to add `date_key` and `report_period` columns to the fundamentals table:

```sql
CREATE TABLE IF NOT EXISTS fundamentals (
    composite_figi TEXT NOT NULL REFERENCES assets(composite_figi),
    event_date TEXT NOT NULL,
    date_key TEXT,
    report_period TEXT,
    dimension TEXT NOT NULL,
    {metric columns},
    PRIMARY KEY (composite_figi, event_date, dimension)
)
```

Update `recordFundamentals` in `data/snapshot_recorder.go` to write these columns. Update the snapshot reader to read them back.

### 5. Documentation

- `docs/data.md`: Document the fundamental date field semantics. After the column swap: `event_date` is the filing date (when data became publicly available for AR dimensions), `date_key` is the normalized calendar quarter boundary (for cross-company comparison), `report_period` is the actual fiscal period end. Explain that the engine automatically applies point-in-time filtering and forward-fill for fundamental metrics.

- `docs/strategy-guide.md`: Document `SetFundamentalDimension`. Explain the dimension choices (AR vs MR, Q vs Y vs T) and their implications. AR dimensions are suitable for backtesting (point-in-time indexed to SEC filing). MR dimensions include restatements and are indexed to the fiscal period end.

- `CHANGELOG.md`: Entries under appropriate headings for the point-in-time fix, dimension API, and snapshot schema change.

### 6. Deferred: DataFrame metadata for date_key

A future change will expose `date_key` (normalized quarter boundary) as DataFrame metadata so strategies can group fundamentals by reporting period for cross-company alignment. This is not needed for the initial fix -- the NCAVE strategy and similar "most recent available" patterns work without it.

## What does not change

- The `DataSource` interface (`Fetch`, `FetchAt`, `CurrentDate`) -- signatures are unchanged.
- The `DataRequest` struct -- no new fields needed.
- The `BatchProvider` interface.
- `fetchEod` and `fetchMetrics` queries -- daily data, `event_date` is already correct for those tables.
- The cache key structure (`colCacheKey`) -- fundamental columns are cached the same way, just with forward-fill applied during assembly.
