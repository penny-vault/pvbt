# Rated Universe Design

## Problem

Strategy authors need a way to select stocks based on analyst ratings stored in the pv-data `ratings` view. The immediate use case is MDEP, which needs "all assets where analyst = 'zacks-rank' AND rating = 1" at each simulation date. Future strategies will filter by other analysts and rating ranges.

The ratings table schema is generic: `(ticker, composite_figi, event_date, analyst, rating)` where rating is an integer 1-5. The `analyst` field identifies the rating source (e.g., "zacks-rank", "zacks-value", "zacks-growth").

Currently, pvbt has no awareness of the ratings view. The `PVDataProvider` only knows about `eod`, `metrics`, and `fundamentals` views. Universes are either static (fixed ticker list) or index-based (historical index membership). There is no way to define a universe whose membership is determined by rating criteria.

## Approach

Add a `ratedUniverse` -- a new `Universe` implementation whose membership at each date is resolved by querying analyst ratings. This combines two existing patterns:

- **Membership resolution and caching** from `indexUniverse` -- a provider resolves which assets belong at each date, results are cached per date.
- **DataSource wiring** from `StaticUniverse` -- the universe holds a `DataSource` reference (set by the engine) so `Window()` and `At()` can fetch metric data for the resolved members.

Note: `indexUniverse` currently lacks DataSource wiring -- its `Window()` and `At()` return errors. The `ratedUniverse` improves on this by being fully functional once wired. A future `Engine.IndexUniverse()` could follow the same approach.

The engine gets a new `RatedUniverse()` method -- the first engine method that creates a non-static, provider-backed universe. Strategies call it to get a fully wired universe without touching providers directly.

## Design

### 1. RatingFilter (data package)

`RatingFilter` specifies which integer rating values qualify for universe membership. Ratings are assumed to be positive integers (the pv-data schema uses 1-5).

The zero value (`RatingFilter{}`) matches nothing -- `RatedAssets` returns an empty list. This is intentional; callers must explicitly specify which values to match.

```go
// RatingFilter specifies which rating values qualify for membership.
// The zero value matches nothing.
type RatingFilter struct {
    Values []int // match any of these values
}

// Matches returns true if rating is accepted by the filter.
func (f RatingFilter) Matches(rating int) bool {
    for _, v := range f.Values {
        if rating == v {
            return true
        }
    }
    return false
}
```

Convenience constructors:

```go
// RatingEq creates a filter matching exactly one rating value.
func RatingEq(v int) RatingFilter {
    return RatingFilter{Values: []int{v}}
}

// RatingIn creates a filter matching any of the given rating values.
func RatingIn(vs ...int) RatingFilter {
    return RatingFilter{Values: vs}
}

// RatingLTE creates a filter matching rating values from 1 through v (inclusive).
// Assumes ratings are positive integers starting at 1.
func RatingLTE(v int) RatingFilter {
    vals := make([]int, v)
    for i := range vals {
        vals[i] = i + 1
    }
    return RatingFilter{Values: vals}
}
```

### 2. RatingProvider interface (data package)

```go
// RatingProvider resolves asset membership based on analyst ratings.
type RatingProvider interface {
    // RatedAssets returns all assets whose most recent rating on or before t
    // for the given analyst matches the filter.
    RatedAssets(ctx context.Context, analyst string, filter RatingFilter, t time.Time) ([]asset.Asset, error)
}
```

### 3. PVDataProvider implements RatingProvider (data package)

`PVDataProvider` gains a `RatedAssets` method. The SQL query retrieves the most recent rating per asset on or before the requested date and pushes the filter into the query:

```sql
SELECT composite_figi, ticker FROM (
    SELECT DISTINCT ON (composite_figi) composite_figi, ticker, rating
    FROM ratings
    WHERE analyst = $1 AND event_date <= $2
    ORDER BY composite_figi, event_date DESC
) sub
WHERE rating = ANY($3)
```

The `$3` parameter is `filter.Values` passed as a `[]int` (pgx handles this natively). This avoids transferring unneeded rows when only a subset of ratings match.

A compile-time interface check is added:

```go
var _ RatingProvider = (*PVDataProvider)(nil)
```

### 4. ratedUniverse (universe package)

A new unexported struct implementing `Universe`:

```go
type ratedUniverse struct {
    provider data.RatingProvider
    analyst  string
    filter   data.RatingFilter
    ds       DataSource

    mu    sync.Mutex
    cache map[int64][]asset.Asset // keyed by Unix seconds to avoid time.Time map key issues
}
```

The cache uses `int64` (Unix seconds) as the key rather than `time.Time`, following the pattern in `pvdata_provider.go` which notes that `time.Time` equality compares `Location` pointers, making it unsuitable as a map key for times from different `LoadLocation` calls.

Constructor:

```go
// NewRated creates a universe whose membership is determined by analyst ratings.
// The universe has no data source until wired via SetDataSource.
func NewRated(provider data.RatingProvider, analyst string, filter data.RatingFilter) *ratedUniverse
```

`NewRated` returns the unexported concrete type. The intended creation path for strategies is `Engine.RatedUniverse()`, which returns `universe.Universe`.

Behavior:

- **Assets(t)** -- calls `provider.RatedAssets(ctx, analyst, filter, t)`, caches the result keyed by `t.Unix()`, sorts by ticker for determinism. Errors are silently swallowed (returns nil), matching the `Universe.Assets()` signature which has no error return. This is the same behavior as `indexUniverse.Assets()`.
- **Prefetch(ctx, start, end)** -- walks each day in the range and pre-populates the cache. This is the same approach as `indexUniverse.Prefetch()`. Known performance concern: ratings change less frequently than daily, so a bulk query returning only change dates would be more efficient. This is acceptable for v1; can be optimized later by adding a bulk method to `RatingProvider`.
- **Window(ctx, lookback, metrics...)** -- resolves `Assets(ds.CurrentDate())`, then calls `ds.Fetch(ctx, assets, lookback, metrics)`. Returns an error if `ds` is nil.
- **At(ctx, t, metrics...)** -- resolves `Assets(t)`, then calls `ds.FetchAt(ctx, assets, t, metrics)`. Returns an error if `ds` is nil.
- **CurrentDate()** -- delegates to `ds.CurrentDate()`. Returns zero time if `ds` is nil.
- **SetDataSource(ds)** -- stores the data source reference.

### 5. Engine.RatedUniverse() (engine package)

```go
// RatedUniverse creates a universe whose membership is determined by analyst
// ratings. The engine finds a RatingProvider from its registered providers,
// creates the universe, and wires it with the engine's data source.
func (e *Engine) RatedUniverse(analyst string, filter data.RatingFilter) universe.Universe
```

Implementation:

1. Iterate `e.providers`, type-assert each to `data.RatingProvider`.
2. Use the first match to create `universe.NewRated(provider, analyst, filter)`.
3. Call `SetDataSource(e)` on the concrete `*ratedUniverse` directly (no interface needed).
4. Return the wired universe.
5. Panic if no provider implements `RatingProvider` (same convention as `e.Asset()` for missing tickers).

### 6. Strategy usage (not in scope for implementation)

Once the above is in place, a strategy author can write:

```go
func (s *MDEP) Setup(e *engine.Engine) {
    s.zacksUniverse = e.RatedUniverse("zacks-rank", data.RatingEq(1))
    // ...
}

func (s *MDEP) Compute(ctx context.Context, eng *engine.Engine, portfolio portfolio.Portfolio) error {
    df, err := s.zacksUniverse.At(ctx, eng.CurrentDate(), data.MarketCap)
    // sort by MarketCap, take top N, equal-weight, rebalance
    return nil
}
```

## Files Changed

| File | Change |
|------|--------|
| `data/rating_filter.go` | New file: `RatingFilter`, `RatingEq`, `RatingIn`, `RatingLTE` |
| `data/rating_provider.go` | New file: `RatingProvider` interface |
| `data/pvdata_provider.go` | Add `RatedAssets` method, compile-time check |
| `universe/rated.go` | New file: `ratedUniverse`, `NewRated` constructor |
| `engine/engine.go` | Add `RatedUniverse` method |

## Out of Scope

- Modifying the MDEP strategy (strategy author will do this)
- Changes to the ratings database schema or pv-data
- Adding `Engine.IndexUniverse()` (could follow same pattern later)
- `DataSourceSetter` interface or changes to `StaticUniverse`
- Bulk prefetch optimization for `ratedUniverse`
