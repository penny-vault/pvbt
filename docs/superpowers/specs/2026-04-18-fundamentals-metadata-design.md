# Fundamentals Metadata and Date-Key Queries

## Problem

The 2026-04-10 point-in-time work deferred two pieces:

1. **Metadata exposure.** The `fundamentals` table has three date columns -- `event_date` (filing date), `date_key` (normalized calendar quarter boundary), and `report_period` (actual fiscal period end). Only `event_date` is exposed to strategies (it lives on the DataFrame's time axis). `date_key` is read from SQL by `fetchFundamentals` but discarded after scan. `report_period` is not selected at all. The snapshot recorder has columns for both but writes NULL.

2. **Reporting-period queries.** Strategies cannot ask for "Q1 working capital." After forward-fill, every daily timestamp carries the most recent fundamental value, but the strategy has no way to know which fiscal period that value belongs to, and no way to filter to a specific period. The first concrete need is the NCAVE strategy: it requires Q1 working capital, which is the most recent filing whose `date_key` matches Q1 of the current fiscal year (subject to the filing being public as of the simulation date).

## Design

### 1. Two new metrics

Add `data.FundamentalsDateKey` and `data.FundamentalsReportPeriod` as ordinary `Metric` values alongside the other fundamentals (`data/metric.go`). Names use the `Fundamentals` prefix to avoid collision with the existing `data.DateKey()` helper that converts `time.Time` to YYYYMMDD.

Both are routed through `metricView` to the `"fundamentals"` view. `IsFundamental` returns true for them. The engine's existing forward-fill applies, so the metadata travels with the value: at any daily timestamp, the value of `FundamentalsDateKey` for asset A is the date_key of the most recent filing for A as of that timestamp.

**Encoding.** Stored as `float64(t.Unix())`. Strategies round-trip with `time.Unix(int64(df.Value(asset, data.FundamentalsDateKey, t)), 0)`. No custom helper. NaN means no filing has been observed.

### 2. Provider changes

Add `report_period` to the `fetchFundamentals` SELECT, and register both new metrics in `metricColumn`:

```go
metricColumn[FundamentalsDateKey]      = "date_key"
metricColumn[FundamentalsReportPeriod] = "report_period"
```

`fetchFundamentals` already special-cases `event_date` and `date_key` outside the metric loop. Refactor so all three date columns are scanned generically: when a request includes `FundamentalsDateKey` or `FundamentalsReportPeriod`, the date column is converted to `float64(t.Unix())` and stored in the cache exactly like a regular metric value. When the request does not include them, no extra cost.

### 3. Engine: `FetchFundamentalsByDateKey`

```go
func (e *Engine) FetchFundamentalsByDateKey(
    ctx context.Context,
    assets []asset.Asset,
    metrics []data.Metric,
    dateKey time.Time,
) (*data.DataFrame, error)
```

Behavior:

- Validates that every metric in `metrics` is fundamental (`data.IsFundamental(m)`). Returns `fmt.Errorf("FetchFundamentalsByDateKey: metric %q is not a fundamental metric", m)` otherwise.
- Uses the engine's configured dimension (set via `SetFundamentalDimension`).
- Returns a DataFrame with a single time-axis row at `dateKey`. One value per asset per metric.
- SQL filter: `dimension = $1 AND date_key = $2 AND event_date <= $3`, where `$3` is `eng.CurrentDate()`. For MR dimensions a single asset can have multiple matching rows (restatements); take the row with the maximum `event_date` per asset (`DISTINCT ON (composite_figi)` with `ORDER BY composite_figi, event_date DESC` on PostgreSQL; equivalent window function on SQLite for snapshot replay).
- Assets that have not filed for `dateKey` as of `CurrentDate()` get NaN values. The strategy filters with `math.IsNaN`.

`FetchFundamentalsByDateKey` reuses the existing column cache: the cache key gains a `dateKey` discriminator alongside the existing `(figi, metric, dimension)` tuple so that range queries and date-key queries do not collide.

### 4. Existing `Fetch` and `FetchAt`

No new methods or arguments. `Fetch`/`FetchAt` callers gain access to `FundamentalsDateKey` and `FundamentalsReportPeriod` simply by including them in the metric list. They flow through the same forward-fill path as Revenue, WorkingCapital, etc.

### 5. Snapshot recorder

`recordFundamentals` (`data/snapshot_recorder.go`) currently writes `nil` for `date_key` and `report_period`. Change it to read those values from the DataFrame's `FundamentalsDateKey`/`FundamentalsReportPeriod` columns when present:

- If the DataFrame includes `FundamentalsDateKey`, read the float for the (asset, time) cell, convert to `time.Time` via `time.Unix`, format as `2006-01-02`, write to the `date_key` column. NaN -> NULL.
- Same for `FundamentalsReportPeriod`.
- If the DataFrame does not include those columns, write NULL (current behavior). Snapshots taken without explicitly requesting metadata stay backward-compatible.

The `dimension` column written by the recorder also stops being hardcoded `"ARQ"`. The recorder reads it from the wrapped batch provider via a `Dimension() string` interface (`PVDataProvider` implements it; the existing `SetDimension` already mutates the same field). Providers that do not implement the interface fall back to `"ARQ"` so existing code paths still work.

### 6. Documentation

- `docs/data.md`: document `FundamentalsDateKey` and `FundamentalsReportPeriod` as metrics, including the Unix-seconds encoding and the `time.Unix(int64(...), 0)` round-trip pattern.
- `docs/strategy-guide.md`: document `FetchFundamentalsByDateKey` with a worked NCAVE-style example (compute Q1 date_key for `eng.CurrentDate()`, fetch, filter NaN, use values).
- `CHANGELOG.md`: entries under `Added` for the new metrics and `FetchFundamentalsByDateKey`.

### 7. Testing

- **Provider:** request `FundamentalsDateKey`/`FundamentalsReportPeriod` alongside Revenue; verify cells contain the expected `float64(t.Unix())`. Verify that requests omitting the metadata metrics still work and do not pay for extra columns.
- **Engine forward-fill:** sparse fundamental input on filing dates D1, D2, D3. Fetch a daily range covering all three. Verify `FundamentalsDateKey` forward-fills the same way Revenue does; the value at any non-filing day equals the previous filing's date_key.
- **`FetchFundamentalsByDateKey`:**
  - Valid request returns one time-axis row at `dateKey`, expected values per asset.
  - Non-fundamental metric in the list returns the documented error.
  - Asset with no filing for that `dateKey` gets NaN.
  - MR dimension with restatement: most recent `event_date` <= `CurrentDate()` wins.
  - Filing whose `event_date` is strictly after `CurrentDate()` is excluded (point-in-time correctness).
- **Snapshot:** record a fundamentals DataFrame that includes `FundamentalsDateKey` and `FundamentalsReportPeriod`; read it back via the snapshot provider; verify non-NULL values match.

## What does not change

- DataFrame struct -- no new fields. Metadata is just two more value columns.
- `DataSource` interface signatures.
- `DataRequest` struct -- `FetchFundamentalsByDateKey` lives on `*Engine`, not on the provider interface. It builds an internal request with the necessary filter.
- Cache key shape beyond the added `dateKey` discriminator.
- `SetFundamentalDimension` -- still a strategy-level setting in `Setup`.
- `fetchEod` and `fetchMetrics` queries.
