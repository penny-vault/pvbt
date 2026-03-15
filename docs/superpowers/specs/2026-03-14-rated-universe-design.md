# Rated Universe Design

## Problem

Strategy authors need a way to select stocks based on analyst ratings stored in the pv-data `ratings` view. The immediate use case is MDEP, which needs "all assets where analyst = 'zacks-rank' AND rating = 1" at each simulation date. Future strategies will filter by other analysts and rating ranges.

The ratings table schema is generic: `(ticker, composite_figi, event_date, analyst, rating)` where rating is an integer 1-5. The `analyst` field identifies the rating source (e.g., "zacks-rank", "zacks-value", "zacks-growth").

Currently, pvbt has no awareness of the ratings view. The `PVDataProvider` only knows about `eod`, `metrics`, and `fundamentals` views. Universes are either static (fixed ticker list) or index-based (historical index membership). There is no way to define a universe whose membership is determined by rating criteria.

## Approach

Add a `RatedUniverse` -- a new `Universe` implementation whose membership at each date is resolved by querying analyst ratings. This follows the same architectural pattern as `IndexUniverse` (provider-backed, cached, wired with a data source by the engine) but driven by rating criteria instead of index membership.

The engine gets a new `RatedUniverse()` method so strategies can create fully wired rated universes without touching providers directly.

## Design

### 1. RatingFilter (data package)

`RatingFilter` specifies which integer rating values qualify for universe membership.

```go
// RatingFilter specifies which rating values qualify for membership.
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

`PVDataProvider` gains a `RatedAssets` method. The SQL query retrieves the most recent rating per asset on or before the requested date:

```sql
SELECT DISTINCT ON (composite_figi) composite_figi, ticker, rating
FROM ratings
WHERE analyst = $1 AND event_date <= $2
ORDER BY composite_figi, event_date DESC
```

Go code then filters each row with `filter.Matches(rating)` and returns the matching assets.

A compile-time interface check is added:

```go
var _ RatingProvider = (*PVDataProvider)(nil)
```

### 4. ratedUniverse (universe package)

A new unexported struct implementing `Universe` and `DataSourceSetter`:

```go
type ratedUniverse struct {
    provider data.RatingProvider
    analyst  string
    filter   data.RatingFilter
    ds       DataSource

    mu    sync.Mutex
    cache map[time.Time][]asset.Asset
}
```

Constructor:

```go
// NewRated creates a universe whose membership is determined by analyst ratings.
// The universe has no data source until wired via SetDataSource.
func NewRated(provider data.RatingProvider, analyst string, filter data.RatingFilter) *ratedUniverse
```

Behavior:

- **Assets(t)** -- calls `provider.RatedAssets(ctx, analyst, filter, t)`, caches the result per date, sorts by ticker for determinism.
- **Prefetch(ctx, start, end)** -- walks each day in the range and pre-populates the cache (same pattern as `indexUniverse`).
- **Window(ctx, lookback, metrics...)** -- resolves `Assets(ds.CurrentDate())`, then calls `ds.Fetch(ctx, assets, lookback, metrics)`.
- **At(ctx, t, metrics...)** -- resolves `Assets(t)`, then calls `ds.FetchAt(ctx, assets, t, metrics)`.
- **CurrentDate()** -- delegates to `ds.CurrentDate()`.
- **SetDataSource(ds)** -- stores the data source reference.

### 5. DataSourceSetter interface (universe package)

```go
// DataSourceSetter is implemented by universes that need a DataSource
// wired after construction.
type DataSourceSetter interface {
    Universe
    SetDataSource(ds DataSource)
}
```

`StaticUniverse` also implements this (it already has the `ds` field; this just adds the setter method).

### 6. Engine.RatedUniverse() (engine package)

```go
// RatedUniverse creates a universe whose membership is determined by analyst
// ratings. The engine finds a RatingProvider from its registered providers,
// creates the universe, and wires it with the engine's data source.
func (e *Engine) RatedUniverse(analyst string, filter data.RatingFilter) universe.Universe
```

Implementation:

1. Iterate `e.providers`, type-assert each to `data.RatingProvider`.
2. Use the first match to create `universe.NewRated(provider, analyst, filter)`.
3. Call `SetDataSource(e)` on the result.
4. Return the wired universe.
5. Panic if no provider implements `RatingProvider` (same convention as `e.Asset()` for missing tickers).

### 7. Strategy usage (not in scope for implementation)

Once the above is in place, a strategy author can write:

```go
func (s *MDEP) Setup(e *engine.Engine) {
    s.zacksUniverse = e.RatedUniverse("zacks-rank", data.RatingEq(1))
    // ...
}

func (s *MDEP) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {
    df, err := s.zacksUniverse.At(ctx, e.CurrentDate(), data.MarketCap)
    // sort by MarketCap, take top N, equal-weight, rebalance
}
```

## Files Changed

| File | Change |
|------|--------|
| `data/rating_filter.go` | New file: `RatingFilter`, `RatingEq`, `RatingIn`, `RatingLTE` |
| `data/rating_provider.go` | New file: `RatingProvider` interface |
| `data/pvdata_provider.go` | Add `RatedAssets` method, compile-time check |
| `universe/data_source.go` | Add `DataSourceSetter` interface |
| `universe/rated.go` | New file: `ratedUniverse`, `NewRated` constructor |
| `universe/static.go` | Add `SetDataSource` method to `StaticUniverse` |
| `engine/engine.go` | Add `RatedUniverse` method |

## Out of Scope

- Modifying the MDEP strategy (strategy author will do this)
- Changes to the ratings database schema or pv-data
- Adding `Engine.IndexUniverse()` (could follow same pattern later)
