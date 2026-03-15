# Rated Universe Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable strategies to create universes whose membership is determined by analyst ratings from the pv-data `ratings` view.

**Architecture:** A `RatingProvider` interface in the `data` package lets `PVDataProvider` expose rated asset queries. A `ratedUniverse` in the `universe` package resolves membership via the provider and delegates data fetching to the engine's `DataSource`. The engine's `RatedUniverse()` method wires everything together so strategies get a fully functional universe without touching providers.

**Tech Stack:** Go, PostgreSQL (pgx), Ginkgo/Gomega test framework

**Spec:** `docs/superpowers/specs/2026-03-14-rated-universe-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `data/rating_filter.go` | New: `RatingFilter` struct, `Matches`, constructors `RatingEq`/`RatingIn`/`RatingLTE` |
| `data/rating_filter_test.go` | New: tests for `RatingFilter` |
| `data/rating_provider.go` | New: `RatingProvider` interface |
| `data/pvdata_provider.go` | Modify: add `RatedAssets` method + compile-time check |
| `universe/rated.go` | New: `ratedUniverse` struct, `NewRated` constructor, all `Universe` methods |
| `universe/rated_test.go` | New: tests for `ratedUniverse` |
| `engine/engine.go` | Modify: add `RatedUniverse` method |
| `engine/rated_universe_test.go` | New: tests for `Engine.RatedUniverse()` |

---

## Chunk 1: RatingFilter and RatingProvider

### Task 1: RatingFilter type and tests

**Files:**
- Create: `data/rating_filter.go`
- Create: `data/rating_filter_test.go`

- [ ] **Step 1: Write failing tests for RatingFilter**

Create `data/rating_filter_test.go`:

```go
package data_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("RatingFilter", func() {
	Describe("Matches", func() {
		It("returns false for the zero value filter", func() {
			f := data.RatingFilter{}
			Expect(f.Matches(1)).To(BeFalse())
			Expect(f.Matches(5)).To(BeFalse())
		})

		It("matches a single value", func() {
			f := data.RatingEq(1)
			Expect(f.Matches(1)).To(BeTrue())
			Expect(f.Matches(2)).To(BeFalse())
		})

		It("matches any value in a set", func() {
			f := data.RatingIn(1, 2)
			Expect(f.Matches(1)).To(BeTrue())
			Expect(f.Matches(2)).To(BeTrue())
			Expect(f.Matches(3)).To(BeFalse())
		})

		It("matches values from 1 through v with RatingLTE", func() {
			f := data.RatingLTE(3)
			Expect(f.Matches(1)).To(BeTrue())
			Expect(f.Matches(2)).To(BeTrue())
			Expect(f.Matches(3)).To(BeTrue())
			Expect(f.Matches(4)).To(BeFalse())
			Expect(f.Matches(0)).To(BeFalse())
		})

		It("matches nothing with RatingLTE(0)", func() {
			f := data.RatingLTE(0)
			Expect(f.Matches(1)).To(BeFalse())
		})

		It("matches nothing with RatingIn called with no arguments", func() {
			f := data.RatingIn()
			Expect(f.Matches(1)).To(BeFalse())
		})
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "RatingFilter" -v`
Expected: compilation failure -- `data.RatingFilter` undefined

- [ ] **Step 3: Write RatingFilter implementation**

Create `data/rating_filter.go`:

```go
package data

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

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "RatingFilter" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add data/rating_filter.go data/rating_filter_test.go
git commit -m "feat: add RatingFilter type with Eq, In, LTE constructors"
```

### Task 2: RatingProvider interface

**Files:**
- Create: `data/rating_provider.go`

- [ ] **Step 1: Create the RatingProvider interface**

Create `data/rating_provider.go`:

```go
package data

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// RatingProvider resolves asset membership based on analyst ratings.
type RatingProvider interface {
	// RatedAssets returns all assets whose most recent rating on or before t
	// for the given analyst matches the filter.
	RatedAssets(ctx context.Context, analyst string, filter RatingFilter, t time.Time) ([]asset.Asset, error)
}
```

- [ ] **Step 2: Verify the package compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./data/`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add data/rating_provider.go
git commit -m "feat: add RatingProvider interface"
```

### Task 3: PVDataProvider.RatedAssets

**Files:**
- Modify: `data/pvdata_provider.go`

- [ ] **Step 1: Add compile-time interface check and RatedAssets method**

Add at the top of `data/pvdata_provider.go`, near the existing compile-time checks (line 35-36):

```go
var _ RatingProvider = (*PVDataProvider)(nil)
```

Add the `RatedAssets` method at the end of the file (before the metric mappings section):

```go
// RatedAssets returns all assets whose most recent rating on or before t
// for the given analyst matches the filter. Implements RatingProvider.
func (p *PVDataProvider) RatedAssets(ctx context.Context, analyst string, filter RatingFilter, t time.Time) ([]asset.Asset, error) {
	if len(filter.Values) == 0 {
		return nil, nil
	}

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pvdata: acquire connection: %w", err)
	}
	defer conn.Release()

	rows, err := conn.Query(ctx,
		`SELECT composite_figi, ticker FROM (
			SELECT DISTINCT ON (composite_figi) composite_figi, ticker, rating
			FROM ratings
			WHERE analyst = $1 AND event_date <= $2
			ORDER BY composite_figi, event_date DESC
		) sub
		WHERE rating = ANY($3)`,
		analyst, t, filter.Values,
	)
	if err != nil {
		return nil, fmt.Errorf("pvdata: query rated assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset
	for rows.Next() {
		var a asset.Asset
		if err := rows.Scan(&a.CompositeFigi, &a.Ticker); err != nil {
			return nil, fmt.Errorf("pvdata: scan rated asset: %w", err)
		}
		assets = append(assets, a)
	}

	return assets, rows.Err()
}
```

- [ ] **Step 2: Verify the package compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./data/`
Expected: success

Note: `PVDataProvider.RatedAssets` queries a live database so it is not unit-tested here. Integration testing is out of scope per the spec.

- [ ] **Step 3: Commit**

```bash
git add data/pvdata_provider.go
git commit -m "feat: implement RatedAssets on PVDataProvider"
```

---

## Chunk 2: ratedUniverse

### Task 4: ratedUniverse type and tests

**Files:**
- Create: `universe/rated.go`
- Create: `universe/rated_test.go`

- [ ] **Step 1: Write failing tests for ratedUniverse**

Create `universe/rated_test.go`. This uses a mock `RatingProvider` (defined in the test file) and the existing `mockDataSource` from `universe_test.go`. Since `mockDataSource` is in the same package, it's available. The mock provider will also be defined here.

```go
package universe_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// mockRatingProvider implements data.RatingProvider for testing.
type mockRatingProvider struct {
	// results maps Unix seconds to the assets returned for that date.
	results map[int64][]asset.Asset
}

func (m *mockRatingProvider) RatedAssets(_ context.Context, _ string, _ data.RatingFilter, t time.Time) ([]asset.Asset, error) {
	return m.results[t.Unix()], nil
}

var _ = Describe("Rated Universe", func() {
	var (
		aapl     asset.Asset
		goog     asset.Asset
		msft     asset.Asset
		now      time.Time
		emptyDF  *data.DataFrame
		provider *mockRatingProvider
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
		emptyDF, _ = data.NewDataFrame(nil, nil, nil, nil)

		provider = &mockRatingProvider{
			results: map[int64][]asset.Asset{
				now.Unix(): {goog, aapl, msft},
			},
		}
	})

	Describe("Assets", func() {
		It("returns assets from the rating provider sorted by ticker", func() {
			u := universe.NewRated(provider, "zacks-rank", data.RatingEq(1))
			assets := u.Assets(now)

			Expect(assets).To(HaveLen(3))
			Expect(assets[0].Ticker).To(Equal("AAPL"))
			Expect(assets[1].Ticker).To(Equal("GOOG"))
			Expect(assets[2].Ticker).To(Equal("MSFT"))
		})

		It("caches results for the same date", func() {
			callCount := 0
			countingProvider := &countingRatingProvider{
				inner:     provider,
				callCount: &callCount,
			}
			u := universe.NewRated(countingProvider, "zacks-rank", data.RatingEq(1))

			u.Assets(now)
			u.Assets(now)

			Expect(callCount).To(Equal(1))
		})

		It("returns nil when provider returns no assets", func() {
			emptyProvider := &mockRatingProvider{results: map[int64][]asset.Asset{}}
			u := universe.NewRated(emptyProvider, "zacks-rank", data.RatingEq(1))

			assets := u.Assets(now)
			Expect(assets).To(BeNil())
		})

		It("returns nil when the provider returns an error", func() {
			errProvider := &errorRatingProvider{}
			u := universe.NewRated(errProvider, "zacks-rank", data.RatingEq(1))

			assets := u.Assets(now)
			Expect(assets).To(BeNil())
		})
	})

	Describe("Window", func() {
		It("delegates to the data source with resolved assets", func() {
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			u := universe.NewRated(provider, "zacks-rank", data.RatingEq(1))
			u.SetDataSource(ds)

			_, err := u.Window(context.Background(), portfolio.Months(3), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())

			Expect(ds.fetchCalled).To(BeTrue())
			Expect(ds.fetchAssets).To(HaveLen(3))
			Expect(ds.fetchPeriod).To(Equal(portfolio.Months(3)))
		})

		It("returns an error when no data source is set", func() {
			u := universe.NewRated(provider, "zacks-rank", data.RatingEq(1))
			_, err := u.Window(context.Background(), portfolio.Months(3), data.MetricClose)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("At", func() {
		It("delegates to the data source with resolved assets", func() {
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			u := universe.NewRated(provider, "zacks-rank", data.RatingEq(1))
			u.SetDataSource(ds)

			_, err := u.At(context.Background(), now, data.MetricClose)
			Expect(err).NotTo(HaveOccurred())

			Expect(ds.fetchCalled).To(BeTrue())
			Expect(ds.fetchAssets).To(HaveLen(3))
		})

		It("returns an error when no data source is set", func() {
			u := universe.NewRated(provider, "zacks-rank", data.RatingEq(1))
			_, err := u.At(context.Background(), now, data.MetricClose)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CurrentDate", func() {
		It("delegates to the data source", func() {
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			u := universe.NewRated(provider, "zacks-rank", data.RatingEq(1))
			u.SetDataSource(ds)

			Expect(u.CurrentDate()).To(Equal(now))
		})

		It("returns zero time when no data source is set", func() {
			u := universe.NewRated(provider, "zacks-rank", data.RatingEq(1))
			Expect(u.CurrentDate()).To(Equal(time.Time{}))
		})
	})

	Describe("Prefetch", func() {
		It("pre-populates the cache so Assets skips the provider", func() {
			day1 := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
			day2 := time.Date(2025, 6, 16, 0, 0, 0, 0, time.UTC)

			callCount := 0
			multiProvider := &mockRatingProvider{
				results: map[int64][]asset.Asset{
					day1.Unix(): {aapl},
					day2.Unix(): {aapl, goog},
				},
			}
			counting := &countingRatingProvider{inner: multiProvider, callCount: &callCount}
			u := universe.NewRated(counting, "zacks-rank", data.RatingEq(1))

			err := u.Prefetch(context.Background(), day1, day2)
			Expect(err).NotTo(HaveOccurred())
			prefetchCalls := callCount

			// Subsequent Assets calls should not hit the provider again.
			Expect(u.Assets(day1)).To(HaveLen(1))
			Expect(u.Assets(day2)).To(HaveLen(2))
			Expect(callCount).To(Equal(prefetchCalls))
		})
	})
})

// countingRatingProvider wraps a mockRatingProvider and counts calls.
type countingRatingProvider struct {
	inner     *mockRatingProvider
	callCount *int
}

func (c *countingRatingProvider) RatedAssets(ctx context.Context, analyst string, filter data.RatingFilter, t time.Time) ([]asset.Asset, error) {
	*c.callCount++
	return c.inner.RatedAssets(ctx, analyst, filter, t)
}

// errorRatingProvider always returns an error.
type errorRatingProvider struct{}

func (e *errorRatingProvider) RatedAssets(_ context.Context, _ string, _ data.RatingFilter, _ time.Time) ([]asset.Asset, error) {
	return nil, fmt.Errorf("provider error")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./universe/ -run "Rated Universe" -v`
Expected: compilation failure -- `universe.NewRated` undefined

- [ ] **Step 3: Write ratedUniverse implementation**

Create `universe/rated.go`:

```go
package universe

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// compile-time check
var _ Universe = (*ratedUniverse)(nil)

// ratedUniverse resolves membership from a RatingProvider and caches
// results in memory. It delegates data fetching to a DataSource wired
// by the engine.
type ratedUniverse struct {
	provider data.RatingProvider
	analyst  string
	filter   data.RatingFilter
	ds       DataSource

	mu    sync.Mutex
	cache map[int64][]asset.Asset // keyed by Unix seconds
}

// NewRated creates a universe whose membership is determined by analyst ratings.
// The universe has no data source until wired via SetDataSource.
func NewRated(provider data.RatingProvider, analyst string, filter data.RatingFilter) *ratedUniverse {
	return &ratedUniverse{
		provider: provider,
		analyst:  analyst,
		filter:   filter,
		cache:    make(map[int64][]asset.Asset),
	}
}

// SetDataSource wires the universe to a data source for metric fetching.
func (u *ratedUniverse) SetDataSource(ds DataSource) {
	u.ds = ds
}

func (u *ratedUniverse) Assets(t time.Time) []asset.Asset {
	u.mu.Lock()
	defer u.mu.Unlock()

	key := t.Unix()
	if members, ok := u.cache[key]; ok {
		return members
	}

	members, err := u.provider.RatedAssets(context.Background(), u.analyst, u.filter, t)
	if err != nil {
		return nil
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].Ticker < members[j].Ticker
	})

	u.cache[key] = members
	return members
}

func (u *ratedUniverse) Prefetch(ctx context.Context, start, end time.Time) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	for t := start; !t.After(end); t = t.AddDate(0, 0, 1) {
		key := t.Unix()
		if _, ok := u.cache[key]; ok {
			continue
		}

		members, err := u.provider.RatedAssets(ctx, u.analyst, u.filter, t)
		if err != nil {
			return err
		}

		sort.Slice(members, func(i, j int) bool {
			return members[i].Ticker < members[j].Ticker
		})

		u.cache[key] = members
	}

	return nil
}

func (u *ratedUniverse) Window(ctx context.Context, lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("ratedUniverse has no data source; was it created via engine.RatedUniverse()?")
	}
	members := u.Assets(u.ds.CurrentDate())
	return u.ds.Fetch(ctx, members, lookback, metrics)
}

func (u *ratedUniverse) At(ctx context.Context, t time.Time, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("ratedUniverse has no data source; was it created via engine.RatedUniverse()?")
	}
	members := u.Assets(t)
	return u.ds.FetchAt(ctx, members, t, metrics)
}

func (u *ratedUniverse) CurrentDate() time.Time {
	if u.ds == nil {
		return time.Time{}
	}
	return u.ds.CurrentDate()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./universe/ -run "Rated Universe" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add universe/rated.go universe/rated_test.go
git commit -m "feat: add ratedUniverse with provider-backed membership resolution"
```

---

## Chunk 3: Engine.RatedUniverse

### Task 5: Engine.RatedUniverse method and tests

**Files:**
- Modify: `engine/engine.go`
- Create: `engine/rated_universe_test.go`

- [ ] **Step 1: Write failing test for Engine.RatedUniverse**

Create `engine/rated_universe_test.go`. The test needs a mock provider that implements both `data.DataProvider` and `data.RatingProvider`. The existing `data.TestProvider` implements `DataProvider` but not `RatingProvider`, so we create a composite mock.

```go
package engine_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
)

// testRatingProvider implements both DataProvider (via BatchProvider) and RatingProvider.
type testRatingProvider struct {
	metrics []data.Metric
	assets  map[int64][]asset.Asset // keyed by Unix seconds
}

func (p *testRatingProvider) Provides() []data.Metric { return p.metrics }
func (p *testRatingProvider) Close() error             { return nil }
func (p *testRatingProvider) Fetch(_ context.Context, _ data.DataRequest) (*data.DataFrame, error) {
	return data.NewDataFrame(nil, nil, nil, nil)
}
func (p *testRatingProvider) RatedAssets(_ context.Context, _ string, _ data.RatingFilter, t time.Time) ([]asset.Asset, error) {
	return p.assets[t.Unix()], nil
}

var _ = Describe("Engine.RatedUniverse", func() {
	var (
		aapl asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("returns a universe wired with the engine's data source", func() {
		provider := &testRatingProvider{
			metrics: []data.Metric{data.MetricClose},
			assets: map[int64][]asset.Asset{
				now.Unix(): {aapl},
			},
		}

		// noScheduleStrategy is defined in backtest_test.go in this package.
		e := engine.New(&noScheduleStrategy{},
			engine.WithDataProvider(provider),
		)

		u := e.RatedUniverse("zacks-rank", data.RatingEq(1))
		Expect(u).NotTo(BeNil())

		members := u.Assets(now)
		Expect(members).To(HaveLen(1))
		Expect(members[0].Ticker).To(Equal("AAPL"))
	})

	It("panics when no provider implements RatingProvider", func() {
		// data.TestProvider does not implement RatingProvider
		base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		times := []time.Time{base}
		vals := []float64{100}
		frame, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, vals)
		Expect(err).NotTo(HaveOccurred())
		tp := data.NewTestProvider([]data.Metric{data.MetricClose}, frame)

		e := engine.New(&noScheduleStrategy{},
			engine.WithDataProvider(tp),
		)

		Expect(func() {
			e.RatedUniverse("zacks-rank", data.RatingEq(1))
		}).To(Panic())
	})
})
```

Note: `noScheduleStrategy` is already defined in `engine/backtest_test.go` (line 119) in the same `engine_test` package. It implements the `Strategy` interface with the correct `Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio)` signature.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -run "RatedUniverse" -v`
Expected: compilation failure -- `engine.Engine` has no method `RatedUniverse`

- [ ] **Step 3: Implement Engine.RatedUniverse**

Add to `engine/engine.go`, after the existing `Universe` method (around line 145):

```go
// RatedUniverse creates a universe whose membership is determined by analyst
// ratings. The engine finds a RatingProvider from its registered providers,
// creates the universe, and wires it with the engine's data source.
func (e *Engine) RatedUniverse(analyst string, filter data.RatingFilter) universe.Universe {
	for _, p := range e.providers {
		if rp, ok := p.(data.RatingProvider); ok {
			u := universe.NewRated(rp, analyst, filter)
			u.SetDataSource(e)
			return u
		}
	}
	panic(fmt.Sprintf("engine: no provider implements RatingProvider (needed for analyst %q)", analyst))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -run "RatedUniverse" -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./...`
Expected: all existing tests still pass

- [ ] **Step 6: Commit**

```bash
git add engine/engine.go engine/rated_universe_test.go
git commit -m "feat: add Engine.RatedUniverse for rating-based stock selection"
```
