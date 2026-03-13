# Engine Package Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the engine package that orchestrates backtesting and live trading -- data fetching, caching, scheduling, simulated broker, strategy field hydration, and parameter discovery.

**Architecture:** The engine owns the simulation loop. Strategies interact with data through universe methods (`Window`, `At`) which delegate to the engine via a `DataSource` interface, breaking the circular dependency. The engine manages a sliding-window data cache, routes metric requests to the appropriate providers, and provides a simulated broker that fills at close price.

**Tech Stack:** Go, zerolog, tradecron, gonum, ginkgo/gomega for tests

**Spec:** `docs/plans/2026-03-10-engine-design.md`

---

## Chunk 0: Prerequisites in the data package

### Task 0a: AssetProvider interface and PVDataProvider.Assets

**Files:**
- Create: `data/asset_provider.go`
- Modify: `data/pvdata_provider.go` (add `Assets(ctx)` method)

The existing `PVDataProvider` already has `LookupAsset` but does not have `Assets`.
We need to add it so `PVDataProvider` can serve as an `AssetProvider`.

- [ ] **Step 1: Create the AssetProvider interface**

Create `data/asset_provider.go`:

```go
package data

import (
    "context"

    "github.com/penny-vault/pvbt/asset"
)

// AssetProvider supplies asset metadata. The engine bulk-loads all known
// assets at startup and uses LookupAsset as a fallback for cache misses.
type AssetProvider interface {
    // Assets returns all known assets.
    Assets(ctx context.Context) ([]asset.Asset, error)

    // LookupAsset resolves a single ticker to its full Asset (including
    // CompositeFigi). Returns an error if the ticker is unknown.
    LookupAsset(ctx context.Context, ticker string) (asset.Asset, error)
}
```

- [ ] **Step 2: Add Assets method to PVDataProvider**

Add to `data/pvdata_provider.go`:

```go
// Assets returns all known assets from the database.
func (p *PVDataProvider) Assets(ctx context.Context) ([]asset.Asset, error) {
    rows, err := p.pool.Query(ctx,
        `SELECT composite_figi, ticker FROM assets ORDER BY ticker`)
    if err != nil {
        return nil, fmt.Errorf("query assets: %w", err)
    }
    defer rows.Close()

    var assets []asset.Asset
    for rows.Next() {
        var a asset.Asset
        if err := rows.Scan(&a.CompositeFigi, &a.Ticker); err != nil {
            return nil, fmt.Errorf("scan asset: %w", err)
        }
        assets = append(assets, a)
    }
    return assets, rows.Err()
}
```

Add compile-time check:

```go
var _ AssetProvider = (*PVDataProvider)(nil)
```

- [ ] **Step 3: Commit**

```
git add data/asset_provider.go data/pvdata_provider.go
git commit -m "feat(data): add AssetProvider interface, implement on PVDataProvider"
```

---

### Task 0b: DataFrame merge functions

**Files:**
- Create: `data/merge.go`
- Create: `data/merge_test.go`

The engine needs to merge DataFrames from multiple providers (different metrics,
same timestamps) and from multiple cache chunks (same metrics, different time
ranges).

- [ ] **Step 1: Write failing tests**

Create `data/merge_test.go` with tests:

1. `TestMergeColumns` -- two DataFrames with same timestamps and assets but different
   metrics. Verify merged DataFrame has all metrics.
2. `TestMergeColumnsEmpty` -- one or both frames empty.
3. `TestMergeTimes` -- two DataFrames with same assets and metrics but sequential
   time ranges. Verify merged DataFrame has all timestamps in order.
4. `TestMergeTimesOverlap` -- overlapping time ranges should return error.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./data/ -run TestMerge -v`

- [ ] **Step 3: Implement MergeColumns and MergeTimes**

Create `data/merge.go`:

```go
package data

import (
    "fmt"
    "sort"
    "time"

    "github.com/penny-vault/pvbt/asset"
)

// MergeColumns combines DataFrames with the same timestamps but different
// metrics or assets. Used for multi-provider routing.
func MergeColumns(frames ...*DataFrame) (*DataFrame, error) {
    if len(frames) == 0 {
        return mustNewDataFrame(nil, nil, nil, nil), nil
    }
    if len(frames) == 1 {
        return frames[0], nil
    }

    // Verify all frames have the same timestamps.
    base := frames[0]
    for i := 1; i < len(frames); i++ {
        if len(frames[i].times) != len(base.times) {
            return nil, fmt.Errorf("MergeColumns: timestamp count mismatch: %d vs %d",
                len(base.times), len(frames[i].times))
        }
        for j := range base.times {
            if !base.times[j].Equal(frames[i].times[j]) {
                return nil, fmt.Errorf("MergeColumns: timestamp mismatch at index %d", j)
            }
        }
    }

    // Collect all unique (asset, metric) pairs.
    // Start with base, insert columns from subsequent frames.
    result := base.Copy()
    for i := 1; i < len(frames); i++ {
        f := frames[i]
        for _, a := range f.assets {
            for _, m := range f.metrics {
                col := f.Column(a, m)
                if col != nil {
                    colCopy := make([]float64, len(col))
                    copy(colCopy, col)
                    if err := result.Insert(a, m, colCopy); err != nil {
                        return nil, fmt.Errorf("MergeColumns: insert: %w", err)
                    }
                }
            }
        }
    }
    return result, nil
}

// MergeTimes combines DataFrames with the same assets and metrics but
// different, non-overlapping time ranges. Timestamps must not overlap.
func MergeTimes(frames ...*DataFrame) (*DataFrame, error) {
    if len(frames) == 0 {
        return mustNewDataFrame(nil, nil, nil, nil), nil
    }
    if len(frames) == 1 {
        return frames[0], nil
    }

    // Sort frames by start time.
    sorted := make([]*DataFrame, len(frames))
    copy(sorted, frames)
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i].Start().Before(sorted[j].Start())
    })

    // Verify no overlap and same assets/metrics.
    for i := 1; i < len(sorted); i++ {
        if !sorted[i].Start().After(sorted[i-1].End()) {
            return nil, fmt.Errorf("MergeTimes: overlapping time ranges")
        }
    }

    // Collect all times.
    var allTimes []time.Time
    for _, f := range sorted {
        allTimes = append(allTimes, f.times...)
    }

    // Use the first frame's assets and metrics as the schema.
    assets := make([]asset.Asset, len(sorted[0].assets))
    copy(assets, sorted[0].assets)
    metrics := make([]Metric, len(sorted[0].metrics))
    copy(metrics, sorted[0].metrics)

    totalLen := len(allTimes)
    newData := make([]float64, len(assets)*len(metrics)*totalLen)

    tOffset := 0
    for _, f := range sorted {
        fTimeLen := len(f.times)
        for aIdx, a := range assets {
            for mIdx, m := range metrics {
                col := f.Column(a, m)
                if col == nil {
                    continue
                }
                dstOff := (aIdx*len(metrics) + mIdx) * totalLen + tOffset
                copy(newData[dstOff:dstOff+fTimeLen], col)
            }
        }
        tOffset += fTimeLen
    }

    return NewDataFrame(allTimes, assets, metrics, newData)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./data/ -run TestMerge -v`

- [ ] **Step 5: Commit**

```
git add data/merge.go data/merge_test.go
git commit -m "feat(data): add MergeColumns and MergeTimes for DataFrame merging"
```

---

## Chunk 1: Foundation Types and Interfaces

### Task 1: DataSource interface (universe package)

**Files:**
- Create: `universe/data_source.go`

- [ ] **Step 1: Create the DataSource interface**

Create `universe/data_source.go`:

```go
package universe

import (
    "context"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "github.com/penny-vault/pvbt/data"
    "github.com/penny-vault/pvbt/portfolio"
)

// DataSource provides data fetching capabilities to universe implementations.
// The engine implements this interface; universes hold a reference to it.
// This breaks the circular dependency between engine and universe.
type DataSource interface {
    // Fetch returns a DataFrame covering [currentDate - lookback, currentDate]
    // for the given assets and metrics.
    Fetch(ctx context.Context, assets []asset.Asset, lookback portfolio.Period,
        metrics []data.Metric) (*data.DataFrame, error)

    // FetchAt returns a single-row DataFrame at the given time for the given
    // assets and metrics.
    FetchAt(ctx context.Context, assets []asset.Asset, t time.Time,
        metrics []data.Metric) (*data.DataFrame, error)

    // CurrentDate returns the current simulation date.
    CurrentDate() time.Time
}
```

- [ ] **Step 2: Commit**

```
git add universe/data_source.go
git commit -m "feat(universe): add DataSource interface for engine-universe decoupling"
```

---

### Task 2: Update Universe interface with data methods

**Files:**
- Modify: `universe/universe.go`
- Modify: `universe/static.go`

- [ ] **Step 1: Write failing test for StaticUniverse.Window**

Create `universe/universe_test.go`:

```go
package universe_test

import (
    "context"
    "testing"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "github.com/penny-vault/pvbt/data"
    "github.com/penny-vault/pvbt/portfolio"
    "github.com/penny-vault/pvbt/universe"
)

// mockDataSource implements universe.DataSource for testing.
type mockDataSource struct {
    currentDate time.Time
    fetchCalled bool
    fetchAssets []asset.Asset
    fetchPeriod portfolio.Period
    fetchResult *data.DataFrame
}

func (m *mockDataSource) Fetch(_ context.Context, assets []asset.Asset, lookback portfolio.Period, _ []data.Metric) (*data.DataFrame, error) {
    m.fetchCalled = true
    m.fetchAssets = assets
    m.fetchPeriod = lookback
    return m.fetchResult, nil
}

func (m *mockDataSource) FetchAt(_ context.Context, assets []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
    m.fetchCalled = true
    m.fetchAssets = assets
    return m.fetchResult, nil
}

func (m *mockDataSource) CurrentDate() time.Time {
    return m.currentDate
}

func TestStaticUniverseWindow(t *testing.T) {
    aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
    goog := asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}

    now := time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
    emptyDF, _ := data.NewDataFrame(nil, nil, nil, nil)

    ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
    u := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

    _, err := u.Window(portfolio.Months(3), data.MetricClose)
    if err != nil {
        t.Fatalf("Window returned error: %v", err)
    }

    if !ds.fetchCalled {
        t.Fatal("expected Fetch to be called")
    }
    if len(ds.fetchAssets) != 2 {
        t.Fatalf("expected 2 assets, got %d", len(ds.fetchAssets))
    }
    if ds.fetchPeriod != portfolio.Months(3) {
        t.Fatalf("expected Months(3), got %+v", ds.fetchPeriod)
    }
}

func TestStaticUniverseAt(t *testing.T) {
    aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}

    now := time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
    emptyDF, _ := data.NewDataFrame(nil, nil, nil, nil)

    ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
    u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

    _, err := u.At(now, data.MetricClose)
    if err != nil {
        t.Fatalf("At returned error: %v", err)
    }

    if !ds.fetchCalled {
        t.Fatal("expected FetchAt to be called")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./universe/ -run TestStaticUniverse -v`
Expected: compilation error -- `Window`, `At`, `NewStaticWithSource` don't exist.

- [ ] **Step 3: Update Universe interface and StaticUniverse**

Update `universe/universe.go`:

```go
package universe

import (
    "context"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "github.com/penny-vault/pvbt/data"
    "github.com/penny-vault/pvbt/portfolio"
)

// Universe provides time-varying membership of tradeable instruments
// and data access for strategies.
type Universe interface {
    // Assets returns the members of the universe at time t.
    Assets(t time.Time) []asset.Asset

    // Prefetch tells the universe what time range the engine will
    // operate over so it can load membership data in bulk.
    Prefetch(ctx context.Context, start, end time.Time) error

    // Window returns a DataFrame covering [currentDate - lookback, currentDate]
    // for the requested metrics, using the universe's current membership.
    Window(lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error)

    // At returns a single-row DataFrame at time t for the requested metrics.
    At(t time.Time, metrics ...data.Metric) (*data.DataFrame, error)
}
```

Update `universe/static.go`:

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

// StaticUniverse is a fixed set of assets that does not change over time.
type StaticUniverse struct {
    members []asset.Asset
    ds      DataSource
}

func (u *StaticUniverse) Assets(_ time.Time) []asset.Asset { return u.members }

func (u *StaticUniverse) Prefetch(_ context.Context, _, _ time.Time) error { return nil }

func (u *StaticUniverse) Window(lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error) {
    if u.ds == nil {
        return nil, fmt.Errorf("universe has no data source; was it created via engine.Universe()?")
    }
    return u.ds.Fetch(context.TODO(), u.members, lookback, metrics)
}

func (u *StaticUniverse) At(t time.Time, metrics ...data.Metric) (*data.DataFrame, error) {
    if u.ds == nil {
        return nil, fmt.Errorf("universe has no data source; was it created via engine.Universe()?")
    }
    return u.ds.FetchAt(context.TODO(), u.members, t, metrics)
}

// NewStatic creates a static universe from explicit ticker symbols.
// The universe has no data source until SetDataSource is called (typically
// by the engine during field hydration).
func NewStatic(tickers ...string) *StaticUniverse {
    members := make([]asset.Asset, len(tickers))
    for i, t := range tickers {
        members[i] = asset.Asset{Ticker: t}
    }
    return &StaticUniverse{members: members}
}

// NewStaticWithSource creates a static universe wired to a data source.
func NewStaticWithSource(assets []asset.Asset, ds DataSource) *StaticUniverse {
    return &StaticUniverse{members: assets, ds: ds}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./universe/ -run TestStaticUniverse -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add universe/universe.go universe/static.go universe/data_source.go universe/universe_test.go
git commit -m "feat(universe): add Window/At data methods and DataSource interface"
```

---

### Task 3: SimulatedBroker

**Files:**
- Create: `engine/simulated_broker.go`
- Create: `engine/simulated_broker_test.go`

- [ ] **Step 1: Write failing tests**

Create `engine/simulated_broker_test.go`:

```go
package engine_test

import (
    "testing"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "github.com/penny-vault/pvbt/broker"
    "github.com/penny-vault/pvbt/engine"
)

func TestSimulatedBrokerSubmitMarketOrder(t *testing.T) {
    aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
    date := time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)

    sb := engine.NewSimulatedBroker()
    sb.SetPrices(func(a asset.Asset) (float64, bool) {
        if a.CompositeFigi == "FIGI-AAPL" {
            return 150.0, true
        }
        return 0, false
    }, date)

    fills, err := sb.Submit(broker.Order{
        Asset:     aapl,
        Side:      broker.Buy,
        Qty:       100,
        OrderType: broker.Market,
    })

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(fills) != 1 {
        t.Fatalf("expected 1 fill, got %d", len(fills))
    }
    if fills[0].Price != 150.0 {
        t.Errorf("expected price 150.0, got %f", fills[0].Price)
    }
    if fills[0].Qty != 100 {
        t.Errorf("expected qty 100, got %f", fills[0].Qty)
    }
    if !fills[0].FilledAt.Equal(date) {
        t.Errorf("expected fill at %v, got %v", date, fills[0].FilledAt)
    }
}

func TestSimulatedBrokerSubmitUnknownAsset(t *testing.T) {
    unknown := asset.Asset{CompositeFigi: "FIGI-UNKNOWN", Ticker: "UNKNOWN"}
    date := time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)

    sb := engine.NewSimulatedBroker()
    sb.SetPrices(func(_ asset.Asset) (float64, bool) {
        return 0, false
    }, date)

    _, err := sb.Submit(broker.Order{
        Asset:     unknown,
        Side:      broker.Buy,
        Qty:       100,
        OrderType: broker.Market,
    })

    if err == nil {
        t.Fatal("expected error for unknown asset")
    }
}

func TestSimulatedBrokerConnectClose(t *testing.T) {
    sb := engine.NewSimulatedBroker()
    if err := sb.Connect(nil); err != nil {
        t.Fatalf("Connect should succeed: %v", err)
    }
    if err := sb.Close(); err != nil {
        t.Fatalf("Close should succeed: %v", err)
    }
}

// Compile-time interface check.
var _ broker.Broker = (*engine.SimulatedBroker)(nil)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run TestSimulatedBroker -v`
Expected: compilation error -- `SimulatedBroker`, `NewSimulatedBroker`, `SetPrices` don't exist.

- [ ] **Step 3: Implement SimulatedBroker**

Create `engine/simulated_broker.go`:

```go
package engine

import (
    "context"
    "fmt"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "github.com/penny-vault/pvbt/broker"
)

// SimulatedBroker fills all orders at the close price for backtesting.
// The engine updates the price function and date before each Compute step.
type SimulatedBroker struct {
    priceFn func(asset.Asset) (float64, bool)
    date    time.Time
}

// NewSimulatedBroker creates a SimulatedBroker with no prices set.
func NewSimulatedBroker() *SimulatedBroker {
    return &SimulatedBroker{
        priceFn: func(_ asset.Asset) (float64, bool) { return 0, false },
    }
}

// SetPrices updates the price lookup function and simulation date.
func (b *SimulatedBroker) SetPrices(fn func(asset.Asset) (float64, bool), date time.Time) {
    b.priceFn = fn
    b.date = date
}

func (b *SimulatedBroker) Connect(_ context.Context) error { return nil }
func (b *SimulatedBroker) Close() error                    { return nil }

func (b *SimulatedBroker) Submit(order broker.Order) ([]broker.Fill, error) {
    price, ok := b.priceFn(order.Asset)
    if !ok {
        return nil, fmt.Errorf("simulated broker: no price for %s (%s)",
            order.Asset.Ticker, order.Asset.CompositeFigi)
    }

    return []broker.Fill{{
        OrderID:  order.ID,
        Price:    price,
        Qty:      order.Qty,
        FilledAt: b.date,
    }}, nil
}

func (b *SimulatedBroker) Cancel(_ string) error {
    return fmt.Errorf("simulated broker: cancel not supported")
}

func (b *SimulatedBroker) Replace(_ string, _ broker.Order) ([]broker.Fill, error) {
    return nil, fmt.Errorf("simulated broker: replace not supported")
}

func (b *SimulatedBroker) Orders() ([]broker.Order, error)       { return nil, nil }
func (b *SimulatedBroker) Positions() ([]broker.Position, error) { return nil, nil }
func (b *SimulatedBroker) Balance() (broker.Balance, error)      { return broker.Balance{}, nil }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./engine/ -run TestSimulatedBroker -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add engine/simulated_broker.go engine/simulated_broker_test.go
git commit -m "feat(engine): add SimulatedBroker that fills at close price"
```

---

### Task 4: Parameter discovery

**Files:**
- Create: `engine/parameter.go`
- Create: `engine/parameter_test.go`

- [ ] **Step 1: Write failing test**

Create `engine/parameter_test.go`:

```go
package engine_test

import (
    "reflect"
    "testing"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "github.com/penny-vault/pvbt/engine"
    "github.com/penny-vault/pvbt/universe"
)

type testStrategy struct {
    Lookback  float64           `pvbt:"lookback" desc:"Lookback in months" default:"6.0"`
    Ticker    asset.Asset       `pvbt:"ticker" desc:"Primary ticker" default:"SPY"`
    RiskOn    universe.Universe `pvbt:"riskOn" desc:"Risk-on universe" default:"VFINX,PRIDX"`
    Name_     string            // unexported-ish but with underscore; won't match
    hidden    int               // unexported, should be skipped
    Duration  time.Duration     `pvbt:"dur" desc:"Interval" default:"5m"`
    Enabled   bool              `pvbt:"enabled" desc:"Enable feature" default:"true"`
    Count     int               `pvbt:"count" desc:"Number of items" default:"10"`
    Label     string            `pvbt:"label" desc:"Display label" default:"hello"`
}

func (s *testStrategy) Name() string                                                        { return "test" }
func (s *testStrategy) Setup(_ *engine.Engine)                                              {}
func (s *testStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio)  {}

func TestStrategyParameters(t *testing.T) {
    s := &testStrategy{}
    params := engine.StrategyParameters(s)

    // Should only include exported fields: Lookback, Ticker, RiskOn, Name_, Duration, Enabled, Count, Label
    // hidden is unexported, skipped
    if len(params) != 8 {
        t.Fatalf("expected 8 parameters, got %d", len(params))
    }

    // Check first param
    p := findParam(params, "lookback")
    if p == nil {
        t.Fatal("expected parameter 'lookback'")
    }
    if p.Description != "Lookback in months" {
        t.Errorf("expected desc 'Lookback in months', got %q", p.Description)
    }
    if p.Default != "6.0" {
        t.Errorf("expected default '6.0', got %q", p.Default)
    }
    if p.GoType != reflect.TypeOf(float64(0)) {
        t.Errorf("expected float64 type, got %v", p.GoType)
    }
}

func findParam(params []engine.Parameter, name string) *engine.Parameter {
    for i := range params {
        if params[i].Name == name {
            return &params[i]
        }
    }
    return nil
}
```

Note: This test has an import issue -- `portfolio.Portfolio` is used in the interface method but not imported. The actual test file will need the correct import. The `testStrategy` also needs to import `context`. Fix the imports when writing the actual file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run TestStrategyParameters -v`
Expected: compilation error -- `Parameter`, `StrategyParameters` don't exist.

- [ ] **Step 3: Implement Parameter and StrategyParameters**

Create `engine/parameter.go`:

```go
package engine

import (
    "reflect"
    "strings"
)

// Parameter describes a single configurable field on a strategy struct.
type Parameter struct {
    Name        string       // from pvbt tag, or field name lowercased
    FieldName   string       // Go struct field name
    Description string       // from desc tag
    GoType      reflect.Type // field's Go type
    Default     string       // from default tag
}

// StrategyParameters reflects over the strategy struct and returns metadata
// for each exported field. Used by the CLI to generate flags and by UIs to
// build configuration forms.
func StrategyParameters(s Strategy) []Parameter {
    v := reflect.ValueOf(s)
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
    }
    t := v.Type()
    if t.Kind() != reflect.Struct {
        return nil
    }

    var params []Parameter
    for i := 0; i < t.NumField(); i++ {
        field := t.Field(i)
        if !field.IsExported() {
            continue
        }

        name := field.Tag.Get("pvbt")
        if name == "" {
            name = strings.ToLower(field.Name)
        }

        params = append(params, Parameter{
            Name:        name,
            FieldName:   field.Name,
            Description: field.Tag.Get("desc"),
            GoType:      field.Type,
            Default:     field.Tag.Get("default"),
        })
    }

    return params
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./engine/ -run TestStrategyParameters -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add engine/parameter.go engine/parameter_test.go
git commit -m "feat(engine): add StrategyParameters for parameter discovery"
```

---

## Chunk 2: Engine Core

### Task 5: Update Strategy interface and Engine struct

**Files:**
- Modify: `engine/strategy.go`
- Modify: `engine/engine.go`
- Modify: `engine/option.go`
- Remove: `engine/config.go`

- [ ] **Step 1: Update Strategy interface**

Rewrite `engine/strategy.go`:

```go
package engine

import (
    "context"

    "github.com/penny-vault/pvbt/portfolio"
)

// Strategy is the interface that all strategies must implement.
type Strategy interface {
    Name() string
    Setup(e *Engine)
    Compute(ctx context.Context, e *Engine, p portfolio.Portfolio)
}
```

- [ ] **Step 2: Rewrite Engine struct and public methods**

Rewrite `engine/engine.go`. Remove the `Register` method, `DataFrame` method, and
`Config` parameter from `Setup`. Add `SetBenchmark`, `Universe` factory, `CurrentDate`.
Remove the `config.go` import. The `Backtest` and `RunLive` methods remain as stubs
in this task -- they will be implemented in Tasks 8 and 9.

Key changes:
- Remove `universes []universe.Universe` field (no Register)
- Remove `DataFrame` method
- Add `benchmark asset.Asset` field
- Add `assets map[string]asset.Asset` field
- Add `cache *dataCache` field
- Add `currentDate time.Time` field
- Add `start, end time.Time` fields
- Add `metricProvider map[data.Metric]data.BatchProvider` field
- Add `assetProvider data.AssetProvider` field
- Add `SetBenchmark` method
- Add `Universe` factory method
- Add `CurrentDate` method
- Implement `universe.DataSource` interface (Fetch, FetchAt, CurrentDate)
- Update `Asset` to use registry lookup
- Keep `Schedule`, `RiskFreeAsset`, `Close` as-is

```go
package engine

import (
    "context"
    "fmt"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "github.com/penny-vault/pvbt/data"
    "github.com/penny-vault/pvbt/portfolio"
    "github.com/penny-vault/pvbt/tradecron"
    "github.com/penny-vault/pvbt/universe"
)

// Engine orchestrates data access, computation scheduling, and portfolio
// management for both backtesting and live trading.
type Engine struct {
    strategy      Strategy
    providers     []data.DataProvider
    assetProvider data.AssetProvider
    schedule      *tradecron.TradeCron
    riskFree      asset.Asset
    benchmark     asset.Asset

    // configuration (set via options, used during init)
    cacheMaxBytes  int64
    cacheChunkSize time.Duration

    // populated during initialization
    assets         map[string]asset.Asset
    cache          *dataCache
    currentDate    time.Time
    start          time.Time
    end            time.Time
    metricProvider map[data.Metric]data.BatchProvider
}

// New creates a new engine for the given strategy.
func New(strategy Strategy, opts ...Option) *Engine {
    e := &Engine{
        strategy: strategy,
        assets:   make(map[string]asset.Asset),
    }
    for _, opt := range opts {
        opt(e)
    }
    return e
}

// Schedule sets the trading schedule for the engine. Called by the
// strategy during Setup.
func (e *Engine) Schedule(s *tradecron.TradeCron) {
    e.schedule = s
}

// SetBenchmark sets the benchmark asset. Called by the strategy during Setup.
func (e *Engine) SetBenchmark(a asset.Asset) {
    e.benchmark = a
}

// RiskFreeAsset sets the risk-free asset. Called by the strategy during Setup.
func (e *Engine) RiskFreeAsset(a asset.Asset) {
    e.riskFree = a
}

// Asset looks up an asset by ticker from the pre-loaded registry.
// Panics if the ticker cannot be resolved.
func (e *Engine) Asset(ticker string) asset.Asset {
    if a, ok := e.assets[ticker]; ok {
        return a
    }

    if e.assetProvider != nil {
        a, err := e.assetProvider.LookupAsset(context.Background(), ticker)
        if err == nil {
            e.assets[ticker] = a
            return a
        }
    }

    panic(fmt.Sprintf("engine: unknown asset ticker %q", ticker))
}

// Universe creates a static universe wired to this engine for data fetching.
func (e *Engine) Universe(assets ...asset.Asset) universe.Universe {
    return universe.NewStaticWithSource(assets, e)
}

// CurrentDate returns the current simulation date.
func (e *Engine) CurrentDate() time.Time {
    return e.currentDate
}

// Fetch implements universe.DataSource.
func (e *Engine) Fetch(ctx context.Context, assets []asset.Asset, lookback portfolio.Period, metrics []data.Metric) (*data.DataFrame, error) {
    // Implemented in data_cache.go / backtest.go -- stub for now.
    return nil, fmt.Errorf("engine: Fetch not yet implemented")
}

// FetchAt implements universe.DataSource.
func (e *Engine) FetchAt(ctx context.Context, assets []asset.Asset, t time.Time, metrics []data.Metric) (*data.DataFrame, error) {
    // Implemented in data_cache.go / backtest.go -- stub for now.
    return nil, fmt.Errorf("engine: FetchAt not yet implemented")
}

// Close releases all resources held by the engine, including closing
// all registered data providers.
func (e *Engine) Close() error {
    var firstErr error
    for _, p := range e.providers {
        if err := p.Close(); err != nil && firstErr == nil {
            firstErr = err
        }
    }
    return firstErr
}

// Compile-time check that Engine implements universe.DataSource.
var _ universe.DataSource = (*Engine)(nil)

// Run is a deprecated alias for Backtest. It maintains compilation of existing
// callers (e.g., cli/backtest.go) until they are updated in Task 12.
// Deprecated: Use Backtest instead.
func (e *Engine) Run(ctx context.Context, acct *portfolio.Account, start, end time.Time) (*portfolio.Account, error) {
    return e.Backtest(ctx, acct, start, end)
}
```

- [ ] **Step 3: Update options**

Rewrite `engine/option.go`:

```go
package engine

import (
    "time"

    "github.com/penny-vault/pvbt/data"
)

// Option configures the engine.
type Option func(*Engine)

// WithDataProvider registers one or more data providers with the engine.
func WithDataProvider(providers ...data.DataProvider) Option {
    return func(e *Engine) {
        e.providers = append(e.providers, providers...)
    }
}

// WithAssetProvider sets the asset provider for ticker resolution.
func WithAssetProvider(p data.AssetProvider) Option {
    return func(e *Engine) {
        e.assetProvider = p
    }
}

// WithCacheMaxBytes sets the maximum memory for the data cache.
// Default is 512MB.
func WithCacheMaxBytes(n int64) Option {
    return func(e *Engine) {
        if e.cache != nil {
            e.cache.maxBytes = n
        }
        // Store for later initialization if cache not yet created.
        e.cacheMaxBytes = n
    }
}

// WithChunkSize sets the time duration of each data cache chunk.
// Default is 1 year.
func WithChunkSize(d time.Duration) Option {
    return func(e *Engine) {
        if e.cache != nil {
            e.cache.chunkSize = d
        }
        e.cacheChunkSize = d
    }
}
```

Note: `cacheMaxBytes` and `cacheChunkSize` need to be added to the Engine struct as
configuration fields that are used when the cache is initialized during Backtest/RunLive.
Update the Engine struct to include:

```go
    // configuration (set via options, used during init)
    cacheMaxBytes  int64
    cacheChunkSize time.Duration
```

- [ ] **Step 4: Delete config.go**

Remove `engine/config.go` -- the `Config` type is no longer used.

- [ ] **Step 5: Verify compilation and Universe factory**

Run: `go build ./engine/`
Expected: compiles successfully.

Add a quick test in `engine/engine_test.go`:

```go
func TestEngineUniverseFactory(t *testing.T) {
    aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
    e := New(&noopStrategy{}, WithAssetProvider(nil))
    e.assets["AAPL"] = aapl

    u := e.Universe(aapl)
    members := u.Assets(time.Now())
    if len(members) != 1 || members[0].CompositeFigi != "FIGI-AAPL" {
        t.Fatalf("expected 1 member FIGI-AAPL, got %v", members)
    }
}
```

Where `noopStrategy` is a minimal Strategy implementation for testing.

- [ ] **Step 6: Commit**

```
git add engine/strategy.go engine/engine.go engine/option.go
git rm engine/config.go
git commit -m "feat(engine): update Strategy interface, Engine struct, remove Config"
```

---

### Task 6: Strategy field hydration

**Files:**
- Create: `engine/hydrate.go`
- Create: `engine/hydrate_test.go`

- [ ] **Step 1: Write failing tests**

Create `engine/hydrate_test.go` with tests for each supported type:

```go
package engine

import (
    "testing"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "github.com/penny-vault/pvbt/universe"
)

type hydrateTarget struct {
    FloatVal    float64           `default:"3.14"`
    IntVal      int               `default:"42"`
    StringVal   string            `default:"hello"`
    BoolVal     bool              `default:"true"`
    DurationVal time.Duration     `default:"5m"`
    AssetVal    asset.Asset       `default:"AAPL"`
    UniverseVal universe.Universe `default:"AAPL,GOOG"`
    NoDefault   float64
    PreSet      float64           `default:"99.0"`
}

func TestHydrateScalarFields(t *testing.T) {
    target := &hydrateTarget{PreSet: 1.0}

    // Create a minimal engine with a mock asset provider.
    e := &Engine{
        assets: map[string]asset.Asset{
            "AAPL": {CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"},
            "GOOG": {CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"},
        },
    }

    hydrateFields(e, target)

    if target.FloatVal != 3.14 {
        t.Errorf("FloatVal: expected 3.14, got %f", target.FloatVal)
    }
    if target.IntVal != 42 {
        t.Errorf("IntVal: expected 42, got %d", target.IntVal)
    }
    if target.StringVal != "hello" {
        t.Errorf("StringVal: expected 'hello', got %q", target.StringVal)
    }
    if target.BoolVal != true {
        t.Errorf("BoolVal: expected true, got %v", target.BoolVal)
    }
    if target.DurationVal != 5*time.Minute {
        t.Errorf("DurationVal: expected 5m, got %v", target.DurationVal)
    }
    if target.NoDefault != 0 {
        t.Errorf("NoDefault: expected 0, got %f", target.NoDefault)
    }
    // PreSet should NOT be overwritten.
    if target.PreSet != 1.0 {
        t.Errorf("PreSet: expected 1.0 (not overwritten), got %f", target.PreSet)
    }
}

func TestHydrateAssetField(t *testing.T) {
    target := &hydrateTarget{}
    e := &Engine{
        assets: map[string]asset.Asset{
            "AAPL": {CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"},
            "GOOG": {CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"},
        },
    }

    hydrateFields(e, target)

    if target.AssetVal.CompositeFigi != "FIGI-AAPL" {
        t.Errorf("AssetVal: expected FIGI-AAPL, got %q", target.AssetVal.CompositeFigi)
    }
}

func TestHydrateUniverseField(t *testing.T) {
    target := &hydrateTarget{}
    e := &Engine{
        assets: map[string]asset.Asset{
            "AAPL": {CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"},
            "GOOG": {CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"},
        },
    }

    hydrateFields(e, target)

    if target.UniverseVal == nil {
        t.Fatal("UniverseVal should not be nil")
    }
    members := target.UniverseVal.Assets(time.Now())
    if len(members) != 2 {
        t.Fatalf("expected 2 members, got %d", len(members))
    }
    if members[0].CompositeFigi != "FIGI-AAPL" {
        t.Errorf("first member: expected FIGI-AAPL, got %q", members[0].CompositeFigi)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run TestHydrate -v`
Expected: compilation error -- `hydrateFields` doesn't exist.

- [ ] **Step 3: Implement hydrateFields**

Create `engine/hydrate.go`:

```go
package engine

import (
    "reflect"
    "strconv"
    "strings"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "github.com/penny-vault/pvbt/universe"
)

var (
    assetType    = reflect.TypeOf(asset.Asset{})
    universeType = reflect.TypeOf((*universe.Universe)(nil)).Elem()
    durationType = reflect.TypeOf(time.Duration(0))
)

// hydrateFields reflects over the target struct and populates exported fields
// from their `default` tags. Fields with non-zero values are not overwritten.
// asset.Asset fields are resolved via the engine's asset registry.
// universe.Universe fields are built from comma-separated tickers via e.Universe.
func hydrateFields(e *Engine, target interface{}) {
    v := reflect.ValueOf(target)
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
    }
    t := v.Type()
    if t.Kind() != reflect.Struct {
        return
    }

    for i := 0; i < t.NumField(); i++ {
        field := t.Field(i)
        if !field.IsExported() {
            continue
        }

        defaultVal := field.Tag.Get("default")
        if defaultVal == "" {
            continue
        }

        fv := v.Field(i)
        if !fv.CanSet() {
            continue
        }

        // Skip non-zero fields (caller may have pre-set them).
        if !fv.IsZero() {
            continue
        }

        switch {
        case field.Type == assetType:
            a := e.Asset(defaultVal)
            fv.Set(reflect.ValueOf(a))

        case field.Type.Implements(universeType):
            tickers := strings.Split(defaultVal, ",")
            assets := make([]asset.Asset, len(tickers))
            for j, ticker := range tickers {
                assets[j] = e.Asset(strings.TrimSpace(ticker))
            }
            u := e.Universe(assets...)
            fv.Set(reflect.ValueOf(u))

        case field.Type == durationType:
            if d, err := time.ParseDuration(defaultVal); err == nil {
                fv.Set(reflect.ValueOf(d))
            }

        default:
            switch field.Type.Kind() {
            case reflect.Float64:
                if f, err := strconv.ParseFloat(defaultVal, 64); err == nil {
                    fv.SetFloat(f)
                }
            case reflect.Int:
                if n, err := strconv.Atoi(defaultVal); err == nil {
                    fv.SetInt(int64(n))
                }
            case reflect.String:
                fv.SetString(defaultVal)
            case reflect.Bool:
                if b, err := strconv.ParseBool(defaultVal); err == nil {
                    fv.SetBool(b)
                }
            }
        }
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./engine/ -run TestHydrate -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add engine/hydrate.go engine/hydrate_test.go
git commit -m "feat(engine): add strategy field hydration from default tags"
```

---

## Chunk 3: Data Cache and Backtest Loop

### Task 7: Data cache

**Files:**
- Create: `engine/data_cache.go`
- Create: `engine/data_cache_test.go`

- [ ] **Step 1: Write failing tests for cache basics**

Create `engine/data_cache_test.go` with tests covering:

1. `TestCachePutGet` -- store and retrieve a DataFrame by key
2. `TestCacheEviction` -- verify that chunks behind the sliding window are evicted
3. `TestCacheSizeEstimation` -- verify byte calculation matches `T * A * M * 8`
4. `TestCacheChunkBoundaries` -- verify calendar-aligned chunk boundary computation

Each test should construct a small DataFrame via `data.NewDataFrame` with known
dimensions, put it in the cache, and verify retrieval.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./engine/ -run TestCache -v`
Expected: compilation error.

- [ ] **Step 3: Implement dataCache**

Create `engine/data_cache.go`:

The cache stores DataFrames keyed by `dataCacheKey`. It tracks total memory usage
and evicts chunks that fall behind the sliding window.

Key methods:
- `newDataCache(maxBytes int64, chunkSize time.Duration) *dataCache`
- `get(key dataCacheKey) (*data.DataFrame, bool)`
- `put(key dataCacheKey, df *data.DataFrame)`
- `evictBefore(t time.Time)` -- remove all chunks with `chunkStart + chunkSize < t`
- `chunkBoundaries(start, end time.Time) []time.Time` -- return chunk start times
- `estimateBytes(df *data.DataFrame) int64`
- `hashAssets(assets []asset.Asset) uint64`
- `hashMetrics(metrics []data.Metric) uint64`

Use `hash/fnv` for the hash functions. Sort assets by CompositeFigi and metrics
alphabetically before hashing.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./engine/ -run TestCache -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add engine/data_cache.go engine/data_cache_test.go
git commit -m "feat(engine): add sliding-window data cache"
```

---

### Task 8: Implement Engine.Fetch and Engine.FetchAt

**Files:**
- Modify: `engine/engine.go` (replace Fetch/FetchAt stubs)

- [ ] **Step 1: Write failing tests**

Add to `engine/engine_test.go` (or create it):

Test `Engine.Fetch` with a `data.TestProvider` as the BatchProvider:
1. Create a TestProvider with known data (5 assets, 2 metrics, 100 timestamps).
2. Create an Engine with the provider and a mock AssetProvider.
3. Set `e.currentDate`, `e.start`, `e.end`.
4. Initialize the cache.
5. Call `e.Fetch(ctx, assets, Months(3), metrics)`.
6. Verify the returned DataFrame has the correct time window (up to currentDate).
7. Verify the cache was populated.
8. Call again -- verify it returns from cache (provider's Fetch should not be called twice for the same chunk).

Test `Engine.FetchAt`:
1. Same setup.
2. Call `e.FetchAt(ctx, assets, specificDate, metrics)`.
3. Verify single-row DataFrame at that date.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./engine/ -run TestEngineFetch -v`
Expected: FAIL (stubs return errors).

- [ ] **Step 3: Implement Fetch and FetchAt**

In `engine/engine.go`, replace the stub implementations:

`Fetch`:
1. Compute `rangeStart` from `currentDate - lookback` using `portfolio.Period` (subtract N days/months/years from `currentDate`).
2. Compute chunk boundaries for `[rangeStart, e.end]`.
3. For each chunk in `[rangeStart, currentDate]`:
   a. Build cache key from `hashAssets(assets)`, `hashMetrics(metrics)`, `chunkStart`.
   b. Check cache. If hit, continue.
   c. Route metrics to providers. For each provider:
      - Build `DataRequest{Assets, Metrics, Start: chunkStart, End: chunkEnd, Frequency: Daily}`.
      - Call `provider.Fetch(ctx, req)`.
   d. If multiple providers, merge DataFrames.
   e. Put in cache.
4. Collect all cached chunks covering `[rangeStart, currentDate]`.
5. Merge into a single DataFrame.
6. Window to `[rangeStart, currentDate]` using `df.Between(rangeStart, currentDate)`.
7. Return.

Also add helper:

```go
// periodToTime subtracts a Period from a base time.
func periodToTime(base time.Time, p portfolio.Period) time.Time {
    switch p.Unit {
    case portfolio.UnitDay:
        return base.AddDate(0, 0, -p.N)
    case portfolio.UnitMonth:
        return base.AddDate(0, -p.N, 0)
    case portfolio.UnitYear:
        return base.AddDate(-p.N, 0, 0)
    default:
        return base
    }
}
```

`FetchAt`:
1. Call `Fetch` with a `Days(0)` lookback or equivalent.
2. Return `df.At(t)`.
Or: build a DataRequest for just that date, fetch, and return.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./engine/ -run TestEngineFetch -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add engine/engine.go
git commit -m "feat(engine): implement Fetch and FetchAt with cache and provider routing"
```

---

### Task 9: Provider routing

**Files:**
- Modify: `engine/engine.go` (add `buildProviderRouting` method)

- [ ] **Step 1: Write failing test**

Test that when two providers are registered (one provides MetricClose, the other
provides Volume), a Fetch call with both metrics routes to the correct providers
and merges the results.

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Implement buildProviderRouting**

Add method `buildProviderRouting() error` to Engine. Called during initialization.
Iterates providers, calls `Provides()`, builds `metricProvider` map. Returns error
if a required provider doesn't implement `BatchProvider`.

Add method `fetchFromProviders(ctx, assets, metrics, start, end)` that splits
metrics by provider, fetches from each, and merges the results.

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```
git add engine/engine.go
git commit -m "feat(engine): add provider routing by metric"
```

---

### Task 10: Backtest method

**Files:**
- Create: `engine/backtest.go`
- Create: `engine/backtest_test.go`
- Modify: `portfolio/account.go` (add `SetBenchmark`, `SetRiskFree` methods)

- [ ] **Step 1: Write failing integration test**

Create `engine/backtest_test.go`:

Build a complete but minimal end-to-end test:
1. Create a `TestProvider` with 2 assets, daily close + dividend data, 1 year of data.
2. Create a mock `AssetProvider` returning those 2 assets.
3. Create a simple strategy that:
   - In Setup: sets schedule to `@monthend`, sets benchmark.
   - In Compute: calls `s.RiskOn.Window(Months(1), data.MetricClose)` and
     rebalances to equal weight.
4. Call `eng.Backtest(ctx, acct, start, end)`.
5. Verify:
   - Account has transactions (buys).
   - Equity curve has entries (one per scheduled date).
   - No panics or errors.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run TestBacktest -v`
Expected: FAIL.

- [ ] **Step 3: Implement Backtest**

Create `engine/backtest.go` implementing the full flow from the design spec:
initialization, date enumeration, step loop with dividend recording, Compute
dispatch, and price updates. Follow the exact ordering from the design.

Key implementation notes:
- Call `hydrateFields(e, e.strategy)` before Setup.
- Create `SimulatedBroker` and attach to account via `acct.SetBroker(simBroker)`.
- Wire benchmark and risk-free to account after Setup:
  ```go
  if e.benchmark != (asset.Asset{}) {
      acct.SetBenchmark(e.benchmark)  // uses portfolio.WithBenchmark equivalent
  }
  if e.riskFree != (asset.Asset{}) {
      acct.SetRiskFree(e.riskFree)
  }
  ```
  Note: Account currently only has `WithBenchmark`/`WithRiskFree` as constructor options.
  Add `SetBenchmark(a asset.Asset)` and `SetRiskFree(a asset.Asset)` methods to Account
  (2 one-liner methods) so the engine can set them after construction.
- Build step context with `zerolog.New(...).With().Str("strategy", ...).Time("date", ...).Int("step", ...).Logger()`
- Use `logger.WithContext(ctx)` to attach to context.
- Enumerate dates: start from `start`, call `tc.Next()` until past `end`.
- Housekeeping fetch: collect held assets via `acct.Holdings`, add benchmark + risk-free, fetch `{MetricClose, AdjClose, Dividend}`.
- Dividend recording: for each held asset, check Dividend value. Use `df.ValueAt(asset, data.Dividend, currentDate)`.
- Update simulated broker: set priceFn to closure over cache.
- After Compute: build price DataFrame for all relevant assets, call `acct.UpdatePrices`.

Before implementing Backtest, add two setter methods to `portfolio/account.go`:

```go
// SetBenchmark sets the benchmark asset after construction.
func (a *Account) SetBenchmark(b asset.Asset) { a.benchmark = b }

// SetRiskFree sets the risk-free asset after construction.
func (a *Account) SetRiskFree(rf asset.Asset) { a.riskFree = rf }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./engine/ -run TestBacktest -v`
Expected: PASS

- [ ] **Step 5: Run all tests to check for regressions**

Run: `go test ./...`
Expected: All pass.

- [ ] **Step 6: Commit**

```
git add engine/backtest.go engine/backtest_test.go portfolio/account.go
git commit -m "feat(engine): implement Backtest method"
```

---

## Chunk 4: RunLive, CLI Updates, and Docs

### Task 11: RunLive method

**Files:**
- Create: `engine/live.go`
- Create: `engine/live_test.go`

- [ ] **Step 1: Write failing test**

Test RunLive with a short context timeout:
1. Create a strategy, simulated broker, mock provider.
2. Attach the simulated broker to the account.
3. Create a context with a short deadline (100ms).
4. Call `eng.RunLive(ctx, acct)`.
5. Verify: returns a channel, no error.
6. Wait for channel to close (context cancellation).
7. Verify no panics.

Test RunLive with no broker:
1. Create account with no broker.
2. Call `eng.RunLive(ctx, acct)`.
3. Verify: returns error.

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Implement RunLive**

Create `engine/live.go` following the design spec. Key differences from Backtest:
- Validates broker is set on account (returns error if nil).
- Does NOT create simulated broker.
- Runs in a goroutine: computes next schedule time, sleeps, fetches, computes, sends on channel.
- Closes channel when context is cancelled.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./engine/ -run TestRunLive -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add engine/live.go engine/live_test.go
git commit -m "feat(engine): implement RunLive method"
```

---

### Task 12: Update CLI

**Files:**
- Modify: `cli/backtest.go`
- Modify: `cli/flags.go`
- Modify: `cli/live.go`

- [ ] **Step 1: Update flags.go to use engine.StrategyParameters**

Rewrite `registerStrategyFlags` to call `engine.StrategyParameters(strategy)` and
iterate the returned `[]Parameter` to register cobra flags. The logic is similar
to what exists but driven by `Parameter` metadata instead of raw reflection.

Rewrite `applyStrategyFlags` similarly -- iterate parameters, read from viper, set
on the strategy struct via reflection.

- [ ] **Step 2: Update backtest.go**

Change `eng.Run` to `eng.Backtest` (the deprecated `Run` alias from Task 5 kept
the CLI compiling; now switch to the real method). Remove the direct `applyStrategyFlags` call
before `eng.Backtest` since the engine now hydrates defaults (CLI flags should
be applied after engine hydration, overriding defaults).

Update the flow:
1. Create engine: `eng := engine.New(strategy, engine.WithDataProvider(provider), engine.WithAssetProvider(assetProvider))`.
   The `assetProvider` is the same `PVDataProvider` that serves as the data provider --
   it also implements `data.AssetProvider`. Pass it to both `WithDataProvider` and
   `WithAssetProvider`.
2. Apply CLI flag overrides to strategy struct.
3. Call `eng.Backtest(ctx, acct, start, end)`.

- [ ] **Step 3: Update live.go**

Implement the live command to call `eng.RunLive`, read from the returned channel,
and print account state. Basic implementation -- this can be enhanced later.

- [ ] **Step 4: Verify CLI tests pass**

Run: `go test ./cli/ -v`
Expected: PASS (may need to update existing tests for interface changes).

- [ ] **Step 5: Commit**

```
git add cli/backtest.go cli/flags.go cli/live.go
git commit -m "feat(cli): update to use engine.Backtest and StrategyParameters"
```

---

### Task 13: Update documentation

**Files:**
- Modify: `docs/overview.md`
- Modify: `docs/configuration.md`
- Modify: `docs/data.md`
- Modify: `docs/portfolio.md`

- [ ] **Step 1: Update overview.md**

Replace the TOML example with struct tags. Update the ADM strategy code to use the
new interface (`Setup(e)`, `Compute(ctx, e, p)`). Update `main()` to use `Backtest`.
Update the walkthrough sections to match.

- [ ] **Step 2: Update configuration.md**

Rewrite to describe struct tags (`pvbt`, `desc`, `default`) instead of TOML.
Show how `engine.StrategyParameters` works. Show the type mapping table.
Remove TOML-specific sections.

- [ ] **Step 3: Update data.md**

Add `AssetProvider` section. Fix the metric table to reference actual constant names
from `data/metric.go` (e.g., `MetricClose` not `Price`). Add `DataSource` explanation.

- [ ] **Step 4: Update portfolio.md**

Change `e.Run` to `eng.Backtest` in code examples.

- [ ] **Step 5: Commit**

```
git add docs/overview.md docs/configuration.md docs/data.md docs/portfolio.md
git commit -m "docs: update for new engine interface and struct tag configuration"
```

---

### Task 14: Final verification

- [ ] **Step 1: Remove deprecated Run alias**

Delete the `Run` method from `engine/engine.go` (added in Task 5 as a temporary
bridge). All callers now use `Backtest` directly. Verify with `go build ./...`.

- [ ] **Step 2: Run all tests**

Run: `go test ./... -v`
Expected: All pass.

- [ ] **Step 3: Run go vet**

Run: `go vet ./...`
Expected: No issues.

- [ ] **Step 4: Check for unused imports or dead code**

Run: `go build ./...`
Expected: Clean build.

- [ ] **Step 4: Verify the ADM example compiles**

If there is an example strategy in the repo, verify it compiles against the new
interface. If not, consider adding a minimal example in `examples/adm/` for
validation.

- [ ] **Step 5: Final commit if any cleanup needed**

```
git add -A
git commit -m "chore: final cleanup for engine package implementation"
```
