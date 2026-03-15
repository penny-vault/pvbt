# Snapshot-Based Testing with Real Data

**Issue:** #20
**Date:** 2026-03-15
**Status:** Draft

## Problem

Testing strategies against real market data catches bugs that synthetic data misses -- edge cases around splits, dividends, delistings, and holiday schedules. But unit tests should not depend on external data providers being available or returning consistent results.

## Solution

A two-part feature:

1. **Snapshot capture** -- a CLI command that wraps real data providers, records every data request and response during a strategy run, and writes the results to a SQLite file.
2. **Snapshot replay** -- a data provider that reads the SQLite file and serves the recorded data, requiring no external dependencies.

## Architecture

### SQLite Schema

The snapshot database mirrors pv-data's table structure so the recording and replay code can use nearly identical SQL.

**`assets`** -- AssetProvider data

| Column         | Type    | Constraint |
|----------------|---------|------------|
| composite_figi | TEXT    | PK         |
| ticker         | TEXT    |            |

**`eod`** -- BatchProvider end-of-day price data

| Column         | Type | Constraint                          |
|----------------|------|-------------------------------------|
| composite_figi | TEXT | FK assets(composite_figi)           |
| event_date     | TEXT | ISO 8601                            |
| open           | REAL |                                     |
| high           | REAL |                                     |
| low            | REAL |                                     |
| close          | REAL |                                     |
| adj_close      | REAL |                                     |
| volume         | REAL |                                     |
| dividend       | REAL |                                     |
| split_factor   | REAL |                                     |
| | | PK (composite_figi, event_date) |

**`metrics`** -- BatchProvider valuation metrics

| Column              | Type    | Constraint                          |
|---------------------|---------|-------------------------------------|
| composite_figi      | TEXT    | FK assets(composite_figi)           |
| event_date          | TEXT    |                                     |
| market_cap          | INTEGER |                                     |
| ev                  | INTEGER |                                     |
| pe                  | REAL    |                                     |
| pb                  | REAL    |                                     |
| ps                  | REAL    |                                     |
| ev_ebit             | REAL    |                                     |
| ev_ebitda           | REAL    |                                     |
| pe_forward          | REAL    |                                     |
| peg                 | REAL    |                                     |
| price_to_cash_flow  | REAL    |                                     |
| beta                | REAL    |                                     |
| | | PK (composite_figi, event_date) |

**`fundamentals`** -- BatchProvider fundamental data

| Column         | Type | Constraint                                    |
|----------------|------|-----------------------------------------------|
| composite_figi | TEXT | FK assets(composite_figi)                     |
| event_date     | TEXT |                                               |
| dimension      | TEXT |                                               |
| (all ~80 fundamental columns) | REAL | Same columns as pv-data fundamentals view |
| | | PK (composite_figi, event_date, dimension) |

**`ratings`** -- RatingProvider recorded call results

| Column         | Type    | Constraint |
|----------------|---------|------------|
| analyst        | TEXT    |            |
| filter_values  | TEXT    | JSON-encoded []int |
| event_date     | TEXT    |            |
| composite_figi | TEXT    |            |
| ticker         | TEXT    |            |
| | | PK (analyst, filter_values, event_date, composite_figi) |

The `ratings` table stores the results of `RatedAssets` calls (not raw rating rows). Each row represents one asset in the result set for a given (analyst, filter, date) call. On replay, the snapshot provider matches on the exact call signature and returns the stored asset list.

**`index_members`** -- IndexProvider data

| Column         | Type | Constraint                                    |
|----------------|------|-----------------------------------------------|
| index_name     | TEXT |                                               |
| event_date     | TEXT |                                               |
| composite_figi | TEXT | FK assets(composite_figi)                     |
| ticker         | TEXT |                                                |
| | | PK (index_name, event_date, composite_figi) |

### SnapshotRecorder

An exported type in the `data` package that wraps real providers and records all data access to SQLite.

```go
type SnapshotRecorder struct {
    // unexported fields: db handle, inner providers, recording wrappers
}

type SnapshotRecorderConfig struct {
    BatchProvider  BatchProvider
    AssetProvider  AssetProvider
    IndexProvider  IndexProvider  // optional
    RatingProvider RatingProvider // optional
}

func NewSnapshotRecorder(path string, cfg SnapshotRecorderConfig) (*SnapshotRecorder, error)
```

The `SnapshotRecorder` implements `BatchProvider`, `AssetProvider`, `IndexProvider`, and `RatingProvider` by delegating to the inner providers and writing results to SQLite. The CLI passes the recorder to the engine as all four provider types.

Internally, four unexported wrapper types handle the per-interface recording logic:

- `recordingBatchProvider` -- on `Fetch`, delegates to the inner provider, then writes the returned DataFrame rows into `eod`, `metrics`, and/or `fundamentals` using the `metricView` mapping to route rows to the correct table. Deduplicates on primary key (upsert).
- `recordingAssetProvider` -- on `Assets()` and `LookupAsset()`, records results to the `assets` table. Deduplicates on `composite_figi`.
- `recordingIndexProvider` -- on `IndexMembers()`, records the result set with the index name and date.
- `recordingRatingProvider` -- on `RatedAssets()`, records the call parameters (analyst, filter values as JSON, date) and the returned asset list.

`Close()` finalizes the SQLite database and closes the connection.

### SnapshotProvider

An exported type in the `data` package that reads a snapshot `.db` file and implements all four provider interfaces.

```go
type SnapshotProvider struct {
    // unexported: read-only *sql.DB
}

func NewSnapshotProvider(path string) (*SnapshotProvider, error)
```

Interface implementations:

- **`Provides()`** -- inspects the snapshot to determine which metrics are present (checks which tables have rows) and returns those metrics.
- **`Fetch(ctx, req)`** -- queries `eod`, `metrics`, and/or `fundamentals` filtered by requested assets, metrics, and date range. Assembles a DataFrame using the same column-major layout as `PVDataProvider`.
- **`Assets(ctx)`** -- returns all rows from `assets`.
- **`LookupAsset(ctx, ticker)`** -- queries `assets WHERE ticker = ?`.
- **`IndexMembers(ctx, index, t)`** -- queries `index_members WHERE index_name = ? AND event_date = ?`.
- **`RatedAssets(ctx, analyst, filter, t)`** -- queries `ratings WHERE analyst = ? AND filter_values = ? AND event_date = ?`, returns the stored assets.
- **`Close()`** -- closes the SQLite connection.

### CLI Command

A new `snapshot` subcommand added to the strategy binary via `cli.Run()`, alongside `backtest` and `live`.

**Flags:** Same as `backtest` (start, end, initial cash, strategy-specific params) plus:
- `--output` (optional) -- path for the snapshot file. Defaults to `pv-data-snapshot-{strategy}-{start_date}-{end_date}.db`.

**Execution flow:**

1. Apply strategy flags (same as `backtest`)
2. Create `PVDataProvider` and any other real providers
3. Create `SnapshotRecorder` wrapping the real providers
4. Build the engine with the recorder as all provider types
5. Run the backtest
6. Call `recorder.Close()`
7. Print the output path and summary (row counts per table)

### Test Usage

Strategy authors use the snapshot in tests:

```go
var _ = Describe("MyStrategy", func() {
    var (
        snap *data.SnapshotProvider
        eng  *engine.Engine
    )

    BeforeEach(func() {
        var err error
        snap, err = data.NewSnapshotProvider("testdata/snapshot.db")
        Expect(err).NotTo(HaveOccurred())

        eng = engine.New(myStrategy,
            engine.WithDataProvider(snap),
            engine.WithAssetProvider(snap),
        )
    })

    AfterEach(func() {
        snap.Close()
    })

    It("produces expected returns", func() {
        result, err := eng.Backtest(ctx)
        Expect(err).NotTo(HaveOccurred())
        // assertions on result
    })
})
```

## File Locations

| File | Package | Purpose |
|------|---------|---------|
| `data/snapshot_recorder.go` | data | SnapshotRecorder and recording wrappers |
| `data/snapshot_provider.go` | data | SnapshotProvider (replay) |
| `data/snapshot_schema.go` | data | SQLite DDL and shared constants |
| `cli/snapshot.go` | cli | snapshot CLI command |

## Dependencies

- `modernc.org/sqlite` -- pure-Go SQLite driver (no CGO)
