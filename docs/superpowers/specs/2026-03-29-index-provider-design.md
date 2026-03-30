# IndexProvider Implementation Design

Issue: #130

## Problem

The `IndexProvider` interface exists and the snapshot recorder/provider fully support it, but no real data provider implements it. `PVDataProvider` has no `IndexMembers` method, and `Engine.IndexUniverse()` panics when called. The pv-data ETL framework already populates `indices_snapshot` and `indices_changelog` tables in PostgreSQL.

## Design

### PVDataProvider.IndexMembers

Implement `IndexMembers(ctx context.Context, index string, forDate time.Time) ([]asset.Asset, error)` on `PVDataProvider`. Add the compile-time check `var _ IndexProvider = (*PVDataProvider)(nil)`.

#### Per-index state

On first call for a given index name, lazily initialize:

- **snapshotStack**: All rows from `indices_snapshot` where `index_name` matches, sorted by `snapshot_date`, earliest on top.
- **changelogStack**: All rows from `indices_changelog` where `index_name` matches, sorted by `event_date`, earliest on top.
- **members**: `[]asset.Asset` representing the current constituents of the index.

#### Algorithm

Given a requested date T (dates must be monotonically increasing across calls):

1. Pop snapshots from `snapshotStack` while the top entry's date <= T. For each popped snapshot date, replace `members` with that snapshot's full constituent list. Discard any changelog entries with dates <= the snapshot date (they are superseded).
2. Pop changelog entries from `changelogStack` while the top entry's date <= T. For each entry, "add" appends to `members`, "remove" scans and swap-removes from `members`.
3. Return `members` directly (zero copy).

The returned slice is borrowed -- it is only valid for the current engine step. If a strategy needs the data across steps, it must copy. The provider mutates `members` in place on subsequent calls.

#### Monotonically increasing constraint

Dates passed to `IndexMembers` must be monotonically increasing. The stacks are consumed as time advances and cannot be rewound. This constraint must be documented on the method. It holds for the engine's backtest loop, which is the only production caller.

### Simplify indexUniverse

The `indexUniverse` type in `universe/index.go` currently maintains a cache and mutex that are redundant now that the provider owns the state.

Remove:
- The `cache map[int64][]asset.Asset` field
- The `sync.Mutex` field
- The `Prefetch` method

`Assets()` becomes a direct call-through to `provider.IndexMembers()`.

Remove the `sort.Slice` call in `Assets()`. The provider does not guarantee member ordering, and consumers should not depend on it. Sorting the borrowed slice would mutate the provider's internal state.

### Universe interface changes

#### Remove Prefetch

Remove `Prefetch(ctx context.Context, start, end time.Time) error` from the `Universe` interface. The engine never calls it (only tests do). Pre-fetching now happens inside the data provider.

Update all implementors:
- `indexUniverse`: remove `Prefetch` method
- `ratedUniverse`: remove `Prefetch` method and its cache/mutex
- `StaticUniverse`: remove no-op `Prefetch`

Update documentation: `docs/universes.md`, `universe/doc.go`.

#### Remove date parameter from At

Change `At` from:

```go
At(ctx context.Context, t time.Time, metrics ...data.Metric) (*data.DataFrame, error)
```

to:

```go
At(ctx context.Context, metrics ...data.Metric) (*data.DataFrame, error)
```

`At` uses `CurrentDate()` internally, the same as `Window`. This eliminates the possibility of calling `Assets()` with an out-of-order date. All existing strategies in `../strategies` pass `eng.CurrentDate()` and simply drop that argument.

### Wiring

In `cli/snapshot.go`, add `IndexProvider: provider` to the `SnapshotRecorderConfig` struct literal. Same one-line pattern as the #128 ratings fix.

### Index name constants

Update `universe/index.go` to use pv-data's canonical names:

- `SP500()`: `"SP500"` -> `"sp500"`
- `Nasdaq100()`: `"NASDAQ100"` -> `"ndx100"`

## Public API changes

All three are breaking changes requiring changelog entries:

1. `Prefetch` removed from `Universe` interface
2. `At` date parameter removed from `Universe` interface
3. `SP500()` and `Nasdaq100()` use pv-data canonical names (`"sp500"`, `"ndx100"`)

## Out of scope

- `ratedUniverse` has the same cache/sort/Prefetch structure as `indexUniverse`. Removing Prefetch from the interface forces removing the method, but further simplification of `ratedUniverse` internals (cache removal, sort removal) is deferred unless the same zero-copy borrow pattern is applied to `RatingProvider`.
