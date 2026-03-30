# Snapshot command does not capture ratings data

**Issue:** penny-vault/pvbt#128
**Date:** 2026-03-29

## Problem

The `snapshot` command does not wire `PVDataProvider` as a `RatingProvider` when constructing the `SnapshotRecorder`. This means:

1. `SnapshotRecorder.ratingProvider` is nil
2. `SnapshotRecorder.RatedAssets()` returns nil without delegating or recording
3. Strategies using `eng.RatedUniverse()` get an empty universe
4. With no rated assets, downstream `Fetch` calls for metrics like `MarketCap` have no assets to query, so the `metrics` table is also empty
5. The strategy falls back to the out-of-market ticker on every rebalance date

The metrics recording path itself works correctly -- the empty metrics table is a consequence of the empty ratings, not a separate bug.

## Fix

In `cli/snapshot.go`, pass `provider` (which implements `RatingProvider`) to the `SnapshotRecorderConfig`:

```go
recorder, err := data.NewSnapshotRecorder(outputPath, data.SnapshotRecorderConfig{
    BatchProvider:  provider,
    AssetProvider:  provider,
    RatingProvider: provider,
})
```

The `SnapshotRecorder` already has full plumbing for recording and replaying ratings data (`RatedAssets`, `recordRatedAssets`, and the `ratings` table schema). No changes needed in the recorder or provider.

## Testing

The recorder-level tests already exist in `data/snapshot_recorder_test.go`:

- Happy path: creates recorder with `stubRatingProvider`, calls `RatedAssets`, verifies `ratings` table has the expected row (line 198)
- Nil provider: verifies `RatedAssets` returns nil when no `RatingProvider` is configured (line 238)

No new recorder tests are needed. The fix is in the CLI wiring layer (`cli/snapshot.go`), which is exercised by running the snapshot command against a live database -- not unit-testable without a PostgreSQL instance.

## Scope

- One-line change in `cli/snapshot.go`
- One new test case
- `IndexProvider` is not wired because `PVDataProvider` does not implement it (tracked in #130)
