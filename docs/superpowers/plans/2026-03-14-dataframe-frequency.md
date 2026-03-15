# DataFrame Frequency Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `Frequency` field to `DataFrame` so it knows its own data resolution (daily, minute, tick, etc.).

**Architecture:** Add a `freq Frequency` field to the DataFrame struct, a parameter to `NewDataFrame` and `mustNewDataFrame`, and propagate through all derived DataFrame methods. Update all callers across the codebase. Add `ParseFrequency` for SQLite round-trip.

**Tech Stack:** Go, Ginkgo/Gomega

**Spec:** `docs/superpowers/specs/2026-03-14-dataframe-frequency-design.md`

---

## Chunk 1: Core DataFrame and frequency package changes

### Task 1: Add ParseFrequency and Frequency accessor

**Files:**
- Modify: `data/frequency.go`

- [ ] **Step 1: Add ParseFrequency function**

In `data/frequency.go`, add after the `String()` method:

```go
// ParseFrequency converts a string back to a Frequency constant.
// It is the inverse of Frequency.String().
func ParseFrequency(s string) (Frequency, error) {
	switch s {
	case "Tick":
		return Tick, nil
	case "Daily":
		return Daily, nil
	case "Weekly":
		return Weekly, nil
	case "Monthly":
		return Monthly, nil
	case "Quarterly":
		return Quarterly, nil
	case "Yearly":
		return Yearly, nil
	default:
		return 0, fmt.Errorf("unknown frequency: %q", s)
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add data/frequency.go
git commit -m "feat: add ParseFrequency for string-to-Frequency conversion"
```

### Task 2: Add freq field to DataFrame, update constructors

**Files:**
- Modify: `data/data_frame.go:39-104`

- [ ] **Step 1: Add freq field to DataFrame struct**

In `data/data_frame.go`, add after the `err` field (line 58):

```go
	// freq is the data publication frequency (Daily, Tick, etc.).
	freq Frequency
```

- [ ] **Step 2: Add Frequency accessor**

Add after the `Err()` method (around line 61):

```go
// Frequency returns the data publication frequency.
func (df *DataFrame) Frequency() Frequency { return df.freq }
```

- [ ] **Step 3: Update NewDataFrame signature**

Change line 74 from:

```go
func NewDataFrame(times []time.Time, assets []asset.Asset, metrics []Metric, data []float64) (*DataFrame, error) {
```

to:

```go
func NewDataFrame(times []time.Time, assets []asset.Asset, metrics []Metric, freq Frequency, data []float64) (*DataFrame, error) {
```

And add `freq: freq,` to the returned struct literal (around line 86):

```go
	return &DataFrame{
		data:       data,
		times:      times,
		assets:     assets,
		metrics:    metrics,
		assetIndex: idx,
		freq:       freq,
	}, nil
```

- [ ] **Step 4: Update mustNewDataFrame signature**

Change line 97 from:

```go
func mustNewDataFrame(times []time.Time, assets []asset.Asset, metrics []Metric, data []float64) *DataFrame {
```

to:

```go
func mustNewDataFrame(times []time.Time, assets []asset.Asset, metrics []Metric, freq Frequency, data []float64) *DataFrame {
```

And update the call to `NewDataFrame` inside it:

```go
	df, err := NewDataFrame(times, assets, metrics, freq, data)
```

- [ ] **Step 5: Update all mustNewDataFrame calls in data_frame.go**

Every call to `mustNewDataFrame` in `data/data_frame.go` must pass `df.freq` as the new parameter. There are approximately 28 calls. The pattern is:

For calls with real data: `mustNewDataFrame(times, assets, metrics, newData)` becomes `mustNewDataFrame(times, assets, metrics, df.freq, newData)`

For nil/empty calls: `mustNewDataFrame(nil, nil, nil, nil)` becomes `mustNewDataFrame(nil, nil, nil, 0, nil)` (frequency 0 is Tick, acceptable for empty DataFrames).

Use find-and-replace:
- Replace `mustNewDataFrame(nil, nil, nil, nil)` with `mustNewDataFrame(nil, nil, nil, 0, nil)` globally in the file
- Then manually update each non-nil call to insert `df.freq` before the data argument

- [ ] **Step 6: Update internal NewDataFrame calls in data_frame.go**

Search for `NewDataFrame(` within `data/data_frame.go` and update each call to include the frequency parameter.

- [ ] **Step 7: Commit**

Note: The codebase will NOT compile at this point. All external callers of `NewDataFrame` and `mustNewDataFrame` still use the old signature. This is expected -- subsequent tasks fix them.

```bash
git add data/data_frame.go
git commit -m "feat: add freq field to DataFrame, update constructors"
```

### Task 3: Update data package internal files

**Files:**
- Modify: `data/downsample.go`
- Modify: `data/upsample.go`
- Modify: `data/merge.go`
- Modify: `data/pvdata_provider.go`
- Modify: `data/test_provider.go`
- Modify: `data/example_data.go`
- Modify: `signal/earnings_yield.go`

- [ ] **Step 1: Update downsample.go**

Two `mustNewDataFrame` calls. The empty one (line 36) gets `0` for freq. The aggregate result (line 82) gets `d.freq` (the target frequency):

```go
// line 36:
return mustNewDataFrame(nil, nil, nil, 0, nil)

// line 82:
return mustNewDataFrame(newTimes, assets, metrics, d.freq, newData)
```

- [ ] **Step 2: Update upsample.go**

Six `mustNewDataFrame` calls. Empty ones get `0`. Result ones get `u.freq` (the target frequency):

Update all `mustNewDataFrame(nil, nil, nil, nil)` to `mustNewDataFrame(nil, nil, nil, 0, nil)`.

Update all result calls: `mustNewDataFrame(newTimes, assets, metrics, newData)` to `mustNewDataFrame(newTimes, assets, metrics, u.freq, newData)`.

- [ ] **Step 3: Update merge.go**

Four `mustNewDataFrame` calls (lines 30, 79, 95) and one `NewDataFrame` call (line 151).

Empty calls get `0`. The `MergeColumns` result should use the frequency of the base DataFrame. The `MergeTimes` `NewDataFrame` call should use the frequency of the first input frame.

For `MergeTimes`, add frequency validation at the start: if input frames have different frequencies, return an error.

- [ ] **Step 4: Update pvdata_provider.go**

Two `NewDataFrame` calls. Pass `req.Frequency` from the `DataRequest`:

Find the `NewDataFrame` calls and add `req.Frequency` before the data argument.

- [ ] **Step 5: Update test_provider.go**

Add a `freq Frequency` field to `TestProvider` struct. Update the constructor to accept it (or default to `Daily`). Pass it in the `Fetch` method's `NewDataFrame` call.

- [ ] **Step 6: Update example_data.go**

Pass `Daily` to the `NewDataFrame` call.

- [ ] **Step 7: Update signal/earnings_yield.go**

The `NewDataFrame` call at line 64 constructs derived signal data. Pass `Daily` (or the frequency of the input DataFrame if available).

- [ ] **Step 8: Update engine/engine.go**

Three `NewDataFrame` calls. Pass the frequency from the request or `Daily` as appropriate.

- [ ] **Step 9: Update portfolio/account.go**

One `NewDataFrame` call in `UpdatePrices` for perfData. Pass `Daily`.

- [ ] **Step 10: Update portfolio/sqlite.go**

In `ToSQLite`: persist `a.perfData.Frequency().String()` as metadata key `"perf_data_frequency"`.

In `FromSQLite`: read the `"perf_data_frequency"` key, parse with `ParseFrequency`, default to `Daily` if missing. Pass to `NewDataFrame` when reconstructing perfData.

- [ ] **Step 11: Commit**

```bash
git add data/downsample.go data/upsample.go data/merge.go \
  data/pvdata_provider.go data/test_provider.go data/example_data.go \
  signal/earnings_yield.go engine/engine.go \
  portfolio/account.go portfolio/sqlite.go
git commit -m "feat: propagate frequency through all internal NewDataFrame callers"
```

### Task 4: Update all test files

**Files:**
- Modify: `data/data_frame_test.go` (~77 calls)
- Modify: `data/merge_test.go` (~6 calls)
- Modify: `data/rolling_data_frame_test.go` (~4 calls)
- Modify: `data/test_provider_test.go` (~1 call)
- Modify: `portfolio/testutil_test.go` (~2 calls)
- Modify: `portfolio/rebalance_test.go` (~7 calls)
- Modify: `portfolio/sqlite_test.go` (~1 call in existing tests)
- Modify: `portfolio/annotation_test.go`
- Modify: `portfolio/selector_test.go` (~20 calls)
- Modify: `portfolio/weighting_test.go` (~26 calls)
- Modify: `portfolio/trade_metrics_test.go` (~3 calls)
- Modify: `portfolio/account_test.go` (~3 calls)
- Modify: `engine/backtest_test.go` (~1 call)
- Modify: `engine/fetch_test.go` (~1 call)
- Modify: `engine/example_test.go`
- Modify: `engine/simulated_broker_test.go` (~1 call)
- Modify: `engine/rated_universe_test.go` (~1 call)
- Modify: `signal/momentum_test.go` (~3 calls)
- Modify: `signal/earnings_yield_test.go` (~4 calls)
- Modify: `signal/volatility_test.go` (~3 calls)
- Modify: `universe/universe_test.go` (~1 call)
- Modify: `universe/rated_test.go` (~1 call)

All test calls to `NewDataFrame` must add `data.Daily` as the new frequency parameter (inserted between `metrics` and `data` arguments).

The pattern for every call is the same:

```go
// Before:
data.NewDataFrame(times, assets, metrics, vals)

// After:
data.NewDataFrame(times, assets, metrics, data.Daily, vals)
```

And for nil calls:

```go
// Before:
data.NewDataFrame(nil, nil, nil, nil)

// After:
data.NewDataFrame(nil, nil, nil, data.Daily, nil)
```

- [ ] **Step 1: Update data package test files**

Update `data/data_frame_test.go`, `data/merge_test.go`, `data/rolling_data_frame_test.go`, `data/test_provider_test.go`.

- [ ] **Step 2: Update portfolio package test files**

Update `portfolio/testutil_test.go`, `portfolio/rebalance_test.go`, `portfolio/sqlite_test.go`, `portfolio/annotation_test.go`, `portfolio/selector_test.go`, `portfolio/weighting_test.go`, `portfolio/trade_metrics_test.go`, `portfolio/account_test.go`.

- [ ] **Step 3: Update engine package test files**

Update `engine/backtest_test.go`, `engine/fetch_test.go`, `engine/example_test.go`, `engine/simulated_broker_test.go`, `engine/rated_universe_test.go`.

- [ ] **Step 4: Update signal package test files**

Update `signal/momentum_test.go`, `signal/earnings_yield_test.go`, `signal/volatility_test.go`.

- [ ] **Step 5: Update universe package test files**

Update `universe/universe_test.go`, `universe/rated_test.go`.

- [ ] **Step 6: Run build and tests**

Run: `go build ./... && go test ./...`
Expected: All pass.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: update all test NewDataFrame calls with frequency parameter"
```

## Chunk 2: Tests and validation

### Task 5: Add tests for frequency propagation

**Files:**
- Modify: `data/data_frame_test.go`

- [ ] **Step 1: Write test for Frequency accessor**

```go
It("returns the frequency set at construction", func() {
    df, err := data.NewDataFrame(
        []time.Time{time.Now()},
        []asset.Asset{{CompositeFigi: "SPY001", Ticker: "SPY"}},
        []data.Metric{data.MetricClose},
        data.Daily,
        []float64{100.0},
    )
    Expect(err).NotTo(HaveOccurred())
    Expect(df.Frequency()).To(Equal(data.Daily))
})
```

- [ ] **Step 2: Write test for frequency propagation through derived methods**

```go
It("propagates frequency through Assets narrowing", func() {
    spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
    df, err := data.NewDataFrame(
        []time.Time{time.Now()},
        []asset.Asset{spy},
        []data.Metric{data.MetricClose},
        data.Monthly,
        []float64{100.0},
    )
    Expect(err).NotTo(HaveOccurred())

    narrowed := df.Assets(spy)
    Expect(narrowed.Frequency()).To(Equal(data.Monthly))
})
```

- [ ] **Step 3: Write test for ParseFrequency**

```go
It("round-trips all frequency constants through String/Parse", func() {
    for _, freq := range []data.Frequency{data.Tick, data.Daily, data.Weekly, data.Monthly, data.Quarterly, data.Yearly} {
        parsed, err := data.ParseFrequency(freq.String())
        Expect(err).NotTo(HaveOccurred())
        Expect(parsed).To(Equal(freq))
    }
})

It("returns error for unknown frequency string", func() {
    _, err := data.ParseFrequency("bogus")
    Expect(err).To(HaveOccurred())
})
```

- [ ] **Step 4: Write test for Downsample frequency**

```go
It("sets target frequency on downsampled result", func() {
    spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
    times := []time.Time{
        time.Date(2024, 1, 2, 16, 0, 0, 0, time.UTC),
        time.Date(2024, 1, 3, 16, 0, 0, 0, time.UTC),
        time.Date(2024, 2, 1, 16, 0, 0, 0, time.UTC),
    }
    df, err := data.NewDataFrame(times, []asset.Asset{spy},
        []data.Metric{data.MetricClose}, data.Daily,
        []float64{100, 101, 110},
    )
    Expect(err).NotTo(HaveOccurred())

    monthly := df.Downsample(data.Monthly).Last()
    Expect(monthly.Frequency()).To(Equal(data.Monthly))
})
```

- [ ] **Step 5: Run tests**

Run: `go test ./data/ -v -count=1`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add data/data_frame_test.go
git commit -m "test: add frequency propagation and ParseFrequency tests"
```

### Task 6: Add SQLite frequency round-trip test

**Files:**
- Modify: `portfolio/sqlite_test.go`

- [ ] **Step 1: Write round-trip test**

Add inside the existing `Describe("round-trip", ...)` block:

```go
It("round-trips perfData frequency", func() {
    spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}

    acct := portfolio.New(
        portfolio.WithCash(10000, time.Time{}),
        portfolio.WithBenchmark(spy),
    )

    // Trigger perfData creation by updating prices.
    priceTime := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
    priceDF, err := data.NewDataFrame(
        []time.Time{priceTime},
        []asset.Asset{spy},
        []data.Metric{data.MetricClose, data.AdjClose},
        data.Daily,
        []float64{500, 500},
    )
    Expect(err).NotTo(HaveOccurred())
    acct.UpdatePrices(priceDF)

    path := filepath.Join(tmpDir, "freq.db")
    Expect(acct.ToSQLite(path)).To(Succeed())

    restored, err := portfolio.FromSQLite(path)
    Expect(err).NotTo(HaveOccurred())

    perfData := restored.PerfData()
    Expect(perfData).NotTo(BeNil())
    Expect(perfData.Frequency()).To(Equal(data.Daily))
})
```

- [ ] **Step 2: Run tests**

Run: `go test ./portfolio/ -v -count=1`
Expected: All pass

- [ ] **Step 3: Run full test suite**

Run: `go build ./... && go test ./...`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add portfolio/sqlite_test.go
git commit -m "test: add SQLite frequency round-trip test"
```
