# IndexProvider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `IndexMembers` on `PVDataProvider` so strategies can use time-varying index universes backed by pv-data's `indices_snapshot` and `indices_changelog` tables.

**Architecture:** A stack-based stateful provider loads all snapshot and changelog data lazily on first call, then pops events as time advances. The `Universe` interface is simplified by removing `Prefetch` (the provider pre-fetches internally) and removing the date parameter from `At` (always uses `CurrentDate()`). The `indexUniverse` drops its cache, mutex, and sort since the provider owns the state and returns a borrowed slice.

**Tech Stack:** Go, PostgreSQL (pgx), Ginkgo/Gomega tests

---

### Task 1: Remove `Prefetch` from the `Universe` interface

**Files:**
- Modify: `universe/universe.go:29-46`
- Modify: `universe/static.go:39`
- Modify: `universe/rated.go:87-112`
- Modify: `universe/index.go:84-108`
- Modify: `universe/index_test.go:196-227`
- Modify: `universe/rated_test.go:198-229`

- [ ] **Step 1: Remove Prefetch from the interface definition**

In `universe/universe.go`, remove the Prefetch method from the `Universe` interface:

```go
type Universe interface {
	// Assets returns the members of the universe at time t.
	Assets(t time.Time) []asset.Asset

	// Window returns a DataFrame covering [currentDate - lookback, currentDate]
	// for the requested metrics, using the universe's current membership.
	Window(ctx context.Context, lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error)

	// At returns a single-row DataFrame at time t for the requested metrics.
	At(ctx context.Context, t time.Time, metrics ...data.Metric) (*data.DataFrame, error)

	// CurrentDate returns the current simulation date from the data source.
	CurrentDate() time.Time
}
```

The `context` import is still needed by `Window` and `At`.

- [ ] **Step 2: Remove Prefetch from StaticUniverse**

In `universe/static.go`, delete line 39:

```go
func (u *StaticUniverse) Prefetch(_ context.Context, _, _ time.Time) error { return nil }
```

- [ ] **Step 3: Remove Prefetch from ratedUniverse**

In `universe/rated.go`, delete the `Prefetch` method (lines 87-112). Leave the cache and sort in place -- ratedUniverse simplification is out of scope.

- [ ] **Step 4: Remove Prefetch from indexUniverse**

In `universe/index.go`, delete the `Prefetch` method (lines 84-108).

- [ ] **Step 5: Delete Prefetch tests**

In `universe/index_test.go`, delete the entire `Describe("Prefetch", ...)` block (lines 196-227).

In `universe/rated_test.go`, delete the entire `Describe("Prefetch", ...)` block (lines 198-229).

- [ ] **Step 6: Remove unused imports**

In `universe/rated.go`, remove `"sort"` and `"sync"` from the import block if Prefetch was the only user. Check: `Assets` still uses both (cache + sort), so they stay. No import changes needed for rated.go.

In `universe/index.go`, the `"context"` import is still needed by `Assets` (calls `context.Background()`). No import changes yet -- Task 3 handles the full rewrite.

- [ ] **Step 7: Verify compilation**

Run: `go build ./...`
Expected: compiles cleanly.

- [ ] **Step 8: Run tests**

Run: `ginkgo run -race ./universe/...`
Expected: all pass.

- [ ] **Step 9: Commit**

```bash
git add universe/
git commit -m "Remove Prefetch from Universe interface

The engine never called Prefetch -- pre-fetching now happens inside
data providers. Removes the method from the interface and all
implementors."
```

---

### Task 2: Remove date parameter from `Universe.At`

**Files:**
- Modify: `universe/universe.go:29-46`
- Modify: `universe/static.go:49-55`
- Modify: `universe/rated.go:126-136` (line numbers after Task 1 deletions)
- Modify: `universe/index.go:110-132` (line numbers after Task 1 deletions)
- Modify: `universe/universe_test.go:108-119`
- Modify: `universe/index_test.go:153-176` (line numbers after Task 1 deletions)
- Modify: `universe/rated_test.go:155-178` (line numbers after Task 1 deletions)
- Modify: `signal/earnings_yield.go:34-40`
- Modify: `signal/earnings_yield_test.go`
- Modify: `engine/example_test.go:89`
- Modify: `engine/doc.go:92`
- Modify: `universe/doc.go:37-38,74`
- Modify: `examples/momentum-rotation/main.go:45`

- [ ] **Step 1: Change At signature in the interface**

In `universe/universe.go`, change the `At` method:

```go
	// At returns a single-row DataFrame at the current simulation date for the
	// requested metrics, using the universe's current membership.
	At(ctx context.Context, metrics ...data.Metric) (*data.DataFrame, error)
```

- [ ] **Step 2: Update StaticUniverse.At**

In `universe/static.go`, change `At` to use `CurrentDate()`:

```go
func (u *StaticUniverse) At(ctx context.Context, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.Universe()?")
	}

	now := u.ds.CurrentDate()

	return u.ds.FetchAt(ctx, u.members, now, metrics)
}
```

- [ ] **Step 3: Update ratedUniverse.At**

In `universe/rated.go`, change `At` to use `CurrentDate()`:

```go
func (u *ratedUniverse) At(ctx context.Context, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.RatedUniverse()?")
	}

	now := u.ds.CurrentDate()
	members := u.Assets(now)

	return u.ds.FetchAt(ctx, members, now, metrics)
}
```

- [ ] **Step 4: Update indexUniverse.At**

In `universe/index.go`, change `At` to use `CurrentDate()`:

```go
func (u *indexUniverse) At(ctx context.Context, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.IndexUniverse()?")
	}

	now := u.ds.CurrentDate()
	members := u.Assets(now)

	return u.ds.FetchAt(ctx, members, now, metrics)
}
```

- [ ] **Step 5: Update signal.EarningsYield**

In `signal/earnings_yield.go`, remove the optional time parameter. The function always uses `CurrentDate()` via `At`:

```go
func EarningsYield(ctx context.Context, assetUniverse universe.Universe) *data.DataFrame {
	df, err := assetUniverse.At(ctx, data.EarningsPerShare, data.Price)
```

- [ ] **Step 6: Update all test callers of At**

In `universe/universe_test.go`, change the At test (around line 108):

```go
	Describe("At", func() {
		It("delegates to the data source FetchAt method", func() {
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			staticUniverse := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

			result, err := staticUniverse.At(context.Background(), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())

			Expect(ds.fetchCalled).To(BeTrue())
			Expect(ds.fetchAssets).To(ConsistOf(aapl))
			Expect(result).To(BeIdenticalTo(emptyDF))
		})
	})
```

In `universe/index_test.go`, update the `Describe("At", ...)` block:

```go
	Describe("At", func() {
		It("delegates to the data source with resolved assets", func() {
			provider := &mockIndexProvider{
				results: map[int64][]asset.Asset{
					now.Unix(): {aapl},
				},
			}
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			u := universe.NewIndex(provider, "SP500")
			u.SetDataSource(ds)

			result, err := u.At(context.Background(), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())
			Expect(ds.fetchCalled).To(BeTrue())
			Expect(ds.fetchAssets).To(ConsistOf(aapl))
			Expect(result).To(BeIdenticalTo(emptyDF))
		})

		It("returns an error when no data source is set", func() {
			provider := &mockIndexProvider{results: map[int64][]asset.Asset{}}
			u := universe.NewIndex(provider, "SP500")
			_, err := u.At(context.Background(), data.MetricClose)
			Expect(err).To(HaveOccurred())
		})
	})
```

In `universe/rated_test.go`, update the `Describe("At", ...)` block:

```go
	Describe("At", func() {
		It("delegates to the data source with resolved assets", func() {
			provider := &mockRatingProvider{
				results: map[int64][]asset.Asset{
					now.Unix(): {aapl},
				},
			}
			ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
			u := universe.NewRated(provider, "analyst1", filter)
			u.SetDataSource(ds)

			result, err := u.At(context.Background(), data.MetricClose)
			Expect(err).NotTo(HaveOccurred())
			Expect(ds.fetchCalled).To(BeTrue())
			Expect(ds.fetchAssets).To(ConsistOf(aapl))
			Expect(result).To(BeIdenticalTo(emptyDF))
		})

		It("returns an error when no data source is set", func() {
			provider := &mockRatingProvider{results: map[int64][]asset.Asset{}}
			u := universe.NewRated(provider, "analyst1", filter)
			_, err := u.At(context.Background(), data.MetricClose)
			Expect(err).To(HaveOccurred())
		})
	})
```

In `signal/earnings_yield_test.go`, update the test that passes an explicit time. The explicit-time test should be removed since the function no longer accepts one. Update all `signal.EarningsYield(ctx, u, explicitTime)` calls to `signal.EarningsYield(ctx, u)`.

- [ ] **Step 7: Update examples and doc comments**

In `examples/momentum-rotation/main.go` line 45, change:

```go
riskOffDF, err := s.RiskOff.At(ctx, eng.CurrentDate(), data.MetricClose)
```
to:
```go
riskOffDF, err := s.RiskOff.At(ctx, data.MetricClose)
```

In `engine/example_test.go` line 89, change:

```go
riskOffDF, err := s.RiskOff.At(ctx, e.CurrentDate(), data.MetricClose)
```
to:
```go
riskOffDF, err := s.RiskOff.At(ctx, data.MetricClose)
```

In `engine/doc.go` line 92, change:

```go
//		riskOffDF, err := s.RiskOff.At(ctx, eng.CurrentDate(), data.MetricClose)
```
to:
```go
//		riskOffDF, err := s.RiskOff.At(ctx, data.MetricClose)
```

In `universe/doc.go`, update the interface listing (line 37-38) and the example (line 74):

Line 37-38: change from `At(ctx, t, metrics...)` to `At(ctx, metrics...)`.
Line 74: change from `u.At(ctx, today, data.Close)` to `u.At(ctx, data.Close)`.

- [ ] **Step 8: Verify compilation**

Run: `go build ./...`
Expected: compiles cleanly.

- [ ] **Step 9: Run tests**

Run: `ginkgo run -race ./...`
Expected: all pass.

- [ ] **Step 10: Commit**

```bash
git add universe/ signal/ engine/ examples/ docs/
git commit -m "Remove date parameter from Universe.At

At now always uses CurrentDate() internally, matching Window's behavior.
This eliminates the possibility of calling Assets() with an out-of-order
date, which would break stateful providers."
```

---

### Task 3: Simplify `indexUniverse`

**Files:**
- Modify: `universe/index.go`
- Modify: `universe/index_test.go`

- [ ] **Step 1: Update tests for the simplified behavior**

In `universe/index_test.go`, update the `Describe("Assets", ...)` block. Remove the "sorted by ticker" expectation and the "caches results" test (provider owns caching now):

The "returns assets from the index provider sorted by ticker" test becomes "returns assets from the index provider":

```go
		It("returns assets from the index provider", func() {
			provider := &mockIndexProvider{
				results: map[int64][]asset.Asset{
					now.Unix(): {goog, aapl, msft},
				},
			}
			u := universe.NewIndex(provider, "SP500")
			assets := u.Assets(now)
			Expect(assets).To(ConsistOf(goog, aapl, msft))
		})
```

Delete the "caches results for the same date" test entirely. Delete the `countingIndexProvider` type from the test file since it's no longer used.

- [ ] **Step 2: Run tests to see them fail**

Run: `ginkgo run -race ./universe/...`
Expected: the updated "returns assets from the index provider" test may fail because `Assets()` currently sorts the result. It should pass with `ConsistOf` since that's order-independent. The deleted tests should no longer appear.

- [ ] **Step 3: Simplify indexUniverse struct and Assets method**

In `universe/index.go`, replace the struct and `NewIndex`, `Assets` method. Remove the cache, mutex, sort, and the `"sort"` and `"sync"` imports:

```go
package universe

import (
	"context"
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// compile-time check
var _ Universe = (*indexUniverse)(nil)

// indexUniverse resolves index membership from an IndexProvider. The provider
// owns the membership state; this type is a thin wrapper that delegates
// Assets calls and provides Window/At data access.
type indexUniverse struct {
	provider  data.IndexProvider
	indexName string
	ds        DataSource
}

// NewIndex creates an index universe backed by the given provider. The universe
// has no data source until SetDataSource is called (or it is created via
// engine.IndexUniverse()).
func NewIndex(provider data.IndexProvider, indexName string) *indexUniverse {
	return &indexUniverse{
		provider:  provider,
		indexName: indexName,
	}
}

// SetDataSource wires the universe to a data source.
func (u *indexUniverse) SetDataSource(ds DataSource) {
	u.ds = ds
}

// Assets returns the index members at the given date. The returned slice is
// borrowed from the provider and is only valid for the current engine step.
// Callers that need data across steps must copy. Dates must be monotonically
// increasing across calls.
func (u *indexUniverse) Assets(asOfDate time.Time) []asset.Asset {
	members, err := u.provider.IndexMembers(context.Background(), u.indexName, asOfDate)
	if err != nil || len(members) == 0 {
		return nil
	}

	return members
}
```

Keep `Window`, `At`, `CurrentDate`, `SP500`, `Nasdaq100` as they are (with the At changes from Task 2).

- [ ] **Step 4: Run tests**

Run: `ginkgo run -race ./universe/...`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add universe/
git commit -m "Simplify indexUniverse to a thin provider wrapper

Remove cache, mutex, and sort. The provider owns membership state and
returns a borrowed slice. Sorting the borrowed slice would mutate the
provider's internal state."
```

---

### Task 4: Update index name constants

**Files:**
- Modify: `universe/index.go`
- Modify: `universe/index_test.go`

- [ ] **Step 1: Update tests for new names**

In `universe/index_test.go`, update the `Describe("SP500 and Nasdaq100 convenience constructors", ...)` block. The tests currently use `mockIndexProvider` which ignores the index name, so the tests pass regardless. Update the test descriptions:

```go
	Describe("SP500 and Nasdaq100 convenience constructors", func() {
		It("SP500 returns a universe that queries 'sp500'", func() {
			provider := &mockIndexProvider{
				results: map[int64][]asset.Asset{
					now.Unix(): {aapl},
				},
			}
			u := universe.SP500(provider)
			assets := u.Assets(now)
			Expect(assets).To(ConsistOf(aapl))
		})

		It("Nasdaq100 returns a universe that queries 'ndx100'", func() {
			provider := &mockIndexProvider{
				results: map[int64][]asset.Asset{
					now.Unix(): {goog},
				},
			}
			u := universe.Nasdaq100(provider)
			assets := u.Assets(now)
			Expect(assets).To(ConsistOf(goog))
		})
	})
```

- [ ] **Step 2: Update the constants**

In `universe/index.go`, change:

```go
func SP500(p data.IndexProvider) *indexUniverse {
	return NewIndex(p, "sp500")
}

func Nasdaq100(p data.IndexProvider) *indexUniverse {
	return NewIndex(p, "ndx100")
}
```

- [ ] **Step 3: Run tests**

Run: `ginkgo run -race ./universe/...`
Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add universe/
git commit -m "Use pv-data canonical index names: sp500, ndx100

Aligns with the names used by pv-data's ETL providers (iShares,
Nasdaq scrapers)."
```

---

### Task 5: Implement `PVDataProvider.IndexMembers`

**Files:**
- Create: `data/index_state.go`
- Modify: `data/pvdata_provider.go:35-53`
- Create: `data/pvdata_index_members_test.go`

- [ ] **Step 1: Write the index state types**

Create `data/index_state.go`:

```go
package data

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// indexSnapshot is a point-in-time capture of all constituents for one index.
type indexSnapshot struct {
	date    time.Time
	members []asset.Asset
}

// indexChange is a single add or remove event from the changelog.
type indexChange struct {
	date          time.Time
	compositeFigi string
	ticker        string
	action        string // "add" or "remove"
}

// indexState holds the stacks and current membership for one index.
// Dates passed to advance must be monotonically increasing.
type indexState struct {
	snapshots []indexSnapshot // sorted by date, earliest first (index 0 = earliest)
	changelog []indexChange  // sorted by date, earliest first
	snapIdx   int            // next unprocessed snapshot
	logIdx    int            // next unprocessed changelog entry
	members   []asset.Asset  // current constituents; mutated in place
}

// advance pops snapshots and changelog entries up through forDate,
// updating members in place. Returns the current members slice.
func (s *indexState) advance(forDate time.Time) []asset.Asset {
	// Pop snapshots: each one resets members entirely.
	for s.snapIdx < len(s.snapshots) && !s.snapshots[s.snapIdx].date.After(forDate) {
		snap := s.snapshots[s.snapIdx]
		s.snapIdx++

		// Replace members with this snapshot's constituents.
		s.members = s.members[:0]
		s.members = append(s.members, snap.members...)

		// Discard changelog entries at or before this snapshot date.
		for s.logIdx < len(s.changelog) && !s.changelog[s.logIdx].date.After(snap.date) {
			s.logIdx++
		}
	}

	// Apply changelog entries up through forDate.
	for s.logIdx < len(s.changelog) && !s.changelog[s.logIdx].date.After(forDate) {
		ch := s.changelog[s.logIdx]
		s.logIdx++

		switch ch.action {
		case "add":
			s.members = append(s.members, asset.Asset{
				CompositeFigi: ch.compositeFigi,
				Ticker:        ch.ticker,
			})
		case "remove":
			for ii := range s.members {
				if s.members[ii].CompositeFigi == ch.compositeFigi {
					last := len(s.members) - 1
					s.members[ii] = s.members[last]
					s.members = s.members[:last]

					break
				}
			}
		}
	}

	return s.members
}
```

- [ ] **Step 2: Write tests for indexState.advance**

Create `data/pvdata_index_members_test.go`:

```go
package data_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("IndexState", func() {
	var (
		aapl asset.Asset
		goog asset.Asset
		msft asset.Asset
		day1 time.Time
		day2 time.Time
		day3 time.Time
		day4 time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		day1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		day2 = time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
		day3 = time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		day4 = time.Date(2025, 1, 7, 0, 0, 0, 0, time.UTC)
	})

	It("returns snapshot members at the snapshot date", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl, goog}}},
			nil,
		)
		members := state.Advance(day1)
		Expect(members).To(ConsistOf(aapl, goog))
	})

	It("carries forward snapshot members to later dates", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl}}},
			nil,
		)
		state.Advance(day1)
		members := state.Advance(day2)
		Expect(members).To(ConsistOf(aapl))
	})

	It("applies changelog add between snapshots", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl}}},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "add"}},
		)
		state.Advance(day1)
		members := state.Advance(day2)
		Expect(members).To(ConsistOf(aapl, goog))
	})

	It("applies changelog remove between snapshots", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl, goog}}},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "remove"}},
		)
		state.Advance(day1)
		members := state.Advance(day2)
		Expect(members).To(ConsistOf(aapl))
	})

	It("resets members when a new snapshot arrives", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{
				{Date: day1, Members: []asset.Asset{aapl, goog}},
				{Date: day3, Members: []asset.Asset{aapl, msft}},
			},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "remove"}},
		)
		state.Advance(day1)
		members := state.Advance(day3)
		Expect(members).To(ConsistOf(aapl, msft))
	})

	It("discards changelog entries superseded by a snapshot", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{
				{Date: day1, Members: []asset.Asset{aapl}},
				{Date: day2, Members: []asset.Asset{aapl, goog}},
			},
			// This add is on day2, same as the snapshot -- should be ignored.
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-MSFT", Ticker: "MSFT", Action: "add"}},
		)
		members := state.Advance(day2)
		// Snapshot says {aapl, goog}. The changelog add of MSFT on day2 is
		// superseded by the snapshot on day2.
		Expect(members).To(ConsistOf(aapl, goog))
	})

	It("returns nil when no data exists before the requested date", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day3, Members: []asset.Asset{aapl}}},
			nil,
		)
		members := state.Advance(day1)
		Expect(members).To(BeNil())
	})

	It("returns the borrowed slice without copying", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl, goog}}},
			nil,
		)
		first := state.Advance(day1)
		second := state.Advance(day2)
		// Same underlying slice -- the provider mutates in place.
		Expect(&first[0]).To(BeIdenticalTo(&second[0]))
	})

	It("handles multiple changelog entries on the same date", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl}}},
			[]data.IndexChangeEntry{
				{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "add"},
				{Date: day2, CompositeFigi: "FIGI-MSFT", Ticker: "MSFT", Action: "add"},
			},
		)
		state.Advance(day1)
		members := state.Advance(day2)
		Expect(members).To(ConsistOf(aapl, goog, msft))
	})

	It("handles changelog add then remove across dates", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl}}},
			[]data.IndexChangeEntry{
				{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "add"},
				{Date: day3, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "remove"},
			},
		)
		state.Advance(day1)
		state.Advance(day2)
		members := state.Advance(day3)
		Expect(members).To(ConsistOf(aapl))
	})
})
```

- [ ] **Step 3: Export types needed by tests**

The test file is in `data_test` (external test package), so it needs exported constructors and types. Update `data/index_state.go` to export the constructor and entry types:

```go
// IndexSnapshotEntry is a snapshot used to construct an indexState.
type IndexSnapshotEntry struct {
	Date    time.Time
	Members []asset.Asset
}

// IndexChangeEntry is a changelog event used to construct an indexState.
type IndexChangeEntry struct {
	Date          time.Time
	CompositeFigi string
	Ticker        string
	Action        string
}

// NewIndexState creates an indexState from pre-sorted snapshots and changelog
// entries (earliest first). Used by PVDataProvider and tests.
func NewIndexState(snapshots []IndexSnapshotEntry, changelog []IndexChangeEntry) *indexState {
	ss := make([]indexSnapshot, len(snapshots))
	for ii, s := range snapshots {
		ss[ii] = indexSnapshot{date: s.Date, members: s.Members}
	}

	cc := make([]indexChange, len(changelog))
	for ii, c := range changelog {
		cc[ii] = indexChange{
			date:          c.Date,
			compositeFigi: c.CompositeFigi,
			ticker:        c.Ticker,
			action:        c.Action,
		}
	}

	return &indexState{snapshots: ss, changelog: cc}
}

// Advance is the exported wrapper for advance.
func (s *indexState) Advance(forDate time.Time) []asset.Asset {
	return s.advance(forDate)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `ginkgo run -race ./data/... -focus "IndexState"`
Expected: all pass.

- [ ] **Step 5: Implement IndexMembers on PVDataProvider**

In `data/pvdata_provider.go`, add the compile-time check and the method. Add a `indexes` field to the struct:

Add to the compile-time checks (line 35-38):

```go
var _ IndexProvider = (*PVDataProvider)(nil)
```

Add a field to `PVDataProvider` (line 49-53):

```go
type PVDataProvider struct {
	pool      *pgxpool.Pool
	ownsPool  bool
	dimension string
	indexes   map[string]*indexState
}
```

Add the `IndexMembers` method after `RatedAssets`:

```go
// IndexMembers returns the constituents of the named index at forDate. The
// returned slice is borrowed -- it is only valid for the current engine step.
// Callers that need data across steps must copy.
//
// Dates must be monotonically increasing across calls for a given index.
// The provider loads all snapshot and changelog data on the first call and
// advances an internal cursor as time progresses.
func (p *PVDataProvider) IndexMembers(ctx context.Context, index string, forDate time.Time) ([]asset.Asset, error) {
	if p.indexes == nil {
		p.indexes = make(map[string]*indexState)
	}

	state, ok := p.indexes[index]
	if !ok {
		var err error

		state, err = p.loadIndexState(ctx, index)
		if err != nil {
			return nil, fmt.Errorf("pvdata: load index state for %q: %w", index, err)
		}

		p.indexes[index] = state
	}

	members := state.Advance(forDate)

	return members, nil
}

func (p *PVDataProvider) loadIndexState(ctx context.Context, index string) (*indexState, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	// Load snapshots grouped by date.
	snapRows, err := conn.Query(ctx,
		`SELECT snapshot_date, composite_figi, ticker
		 FROM indices_snapshot
		 WHERE index_name = $1
		 ORDER BY snapshot_date, composite_figi`,
		index,
	)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer snapRows.Close()

	var snapshots []IndexSnapshotEntry
	var current *IndexSnapshotEntry

	for snapRows.Next() {
		var dt time.Time
		var figi, ticker string

		if err := snapRows.Scan(&dt, &figi, &ticker); err != nil {
			return nil, fmt.Errorf("scan snapshot row: %w", err)
		}

		if current == nil || !current.Date.Equal(dt) {
			if current != nil {
				snapshots = append(snapshots, *current)
			}

			current = &IndexSnapshotEntry{Date: dt}
		}

		current.Members = append(current.Members, asset.Asset{
			CompositeFigi: figi,
			Ticker:        ticker,
		})
	}

	if current != nil {
		snapshots = append(snapshots, *current)
	}

	if err := snapRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate snapshots: %w", err)
	}

	// Load changelog.
	logRows, err := conn.Query(ctx,
		`SELECT event_date, composite_figi, ticker, action
		 FROM indices_changelog
		 WHERE index_name = $1
		 ORDER BY event_date, composite_figi`,
		index,
	)
	if err != nil {
		return nil, fmt.Errorf("query changelog: %w", err)
	}
	defer logRows.Close()

	var changelog []IndexChangeEntry

	for logRows.Next() {
		var entry IndexChangeEntry

		if err := logRows.Scan(&entry.Date, &entry.CompositeFigi, &entry.Ticker, &entry.Action); err != nil {
			return nil, fmt.Errorf("scan changelog row: %w", err)
		}

		changelog = append(changelog, entry)
	}

	if err := logRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate changelog: %w", err)
	}

	return NewIndexState(snapshots, changelog), nil
}
```

- [ ] **Step 6: Verify compilation**

Run: `go build ./...`
Expected: compiles cleanly.

- [ ] **Step 7: Run tests**

Run: `ginkgo run -race ./data/... -focus "IndexState"`
Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add data/index_state.go data/pvdata_provider.go data/pvdata_index_members_test.go
git commit -m "Implement IndexMembers on PVDataProvider (#130)

Queries indices_snapshot and indices_changelog from pv-data on first
call, then advances a stack-based cursor as time progresses. Returns
a borrowed slice that is mutated in place on subsequent calls."
```

---

### Task 6: Wire IndexProvider in snapshot command

**Files:**
- Modify: `cli/snapshot.go:111-115`

- [ ] **Step 1: Add IndexProvider to the recorder config**

In `cli/snapshot.go`, change the `SnapshotRecorderConfig` literal (around line 111):

```go
		recorder, err := data.NewSnapshotRecorder(outputPath, data.SnapshotRecorderConfig{
			BatchProvider:  provider,
			AssetProvider:  provider,
			RatingProvider: provider,
			IndexProvider:  provider,
		})
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./...`
Expected: compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add cli/snapshot.go
git commit -m "Wire PVDataProvider as IndexProvider in snapshot command"
```

---

### Task 7: Update docs and changelog

**Files:**
- Modify: `docs/universes.md`
- Modify: `universe/doc.go`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update docs/universes.md**

Remove the `Prefetch` line from the interface listing and its description paragraph. Change the `At` signature. Update the description of index universe caching. Remove the `Prefetch` mention from the index universe section.

The interface block becomes:

```go
type Universe interface {
    Assets(t time.Time) []asset.Asset
    Window(ctx context.Context, lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error)
    At(ctx context.Context, metrics ...data.Metric) (*data.DataFrame, error)
    CurrentDate() time.Time
}
```

Remove the paragraph that starts with "`Prefetch` allows the engine to tell the universe...".

Change the `At` description to: "`At` fetches a single-row DataFrame for the universe's assets at the current simulation date."

Replace the paragraph "Index universes cache membership in memory..." (line 73) with: "Index universes delegate to the data provider, which loads all snapshot and changelog data on the first call and advances as time progresses. The returned membership slice is borrowed and only valid for the current engine step."

- [ ] **Step 2: Update universe/doc.go**

Update the interface listing to remove `Prefetch` and change `At`:

```go
//   - Assets(t time.Time) []asset.Asset -- returns the members at time t.
//   - Window(ctx, lookback, metrics...) (*data.DataFrame, error) -- returns a
//     DataFrame covering the lookback period ending at the current simulation
//     date.
//   - At(ctx, metrics...) (*data.DataFrame, error) -- returns a single-row
//     DataFrame at the current simulation date.
//   - CurrentDate() time.Time -- returns the current simulation date.
```

Update the doc example (line 74):

```go
//	row, err := u.At(ctx, data.Close)
```

Update the sentence about index universes and caching (around lines 58-63):

```go
// From predefined indexes: use SP500 or Nasdaq100 with an IndexProvider.
// These universes resolve time-varying membership from the data provider.
// The provider loads all data on first access and advances as time progresses.
```

- [ ] **Step 3: Update CHANGELOG.md**

Add entries under `## [Unreleased]`:

Under `### Added`:

```
- Strategies can use `universe.SP500` and `universe.Nasdaq100` to trade against historical index membership sourced from pv-data.
```

Under `### Changed`:

```
- **Breaking:** `Universe.Prefetch` has been removed. Data providers now pre-fetch internally.
- **Breaking:** `Universe.At` no longer accepts a date parameter; it always uses the current simulation date. Update strategy code from `u.At(ctx, date, metrics...)` to `u.At(ctx, metrics...)`.
- **Breaking:** `universe.SP500` and `universe.Nasdaq100` now use pv-data canonical names (`"sp500"`, `"ndx100"`) instead of `"SP500"` and `"NASDAQ100"`.
```

- [ ] **Step 4: Run full test suite**

Run: `ginkgo run -race ./...`
Expected: all pass.

- [ ] **Step 5: Run linter**

Run: `make lint`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add docs/ universe/doc.go CHANGELOG.md
git commit -m "Update docs and changelog for IndexProvider and Universe API changes"
```
