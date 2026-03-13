# Signal Package Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement three signal functions (Momentum, Volatility, EarningsYield) with supporting DataFrame error accumulation, engine look-ahead guard, and universe CurrentDate.

**Architecture:** Signals are pure functions in the `signal` package that fetch data from a `universe.Universe` and return a `*data.DataFrame` with computed scores. DataFrame gains an `err` field for fluent chaining (bufio.Scanner pattern). The engine guards against look-ahead bias.

**Tech Stack:** Go, Ginkgo/Gomega, zerolog, gonum

**Spec:** `docs/superpowers/specs/2026-03-13-signal-package-design.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `data/data_frame.go` | Add `err` field, `Err()`, `WithErr()`, error short-circuits on ALL methods, change Add/Sub/Mul/Div signatures, add `RenameMetric` |
| Modify | `data/rolling_data_frame.go` | Error propagation in all RollingDataFrame methods |
| Modify | `data/data_frame_test.go` | Update existing arithmetic tests for new signatures, add error accumulation + RenameMetric tests |
| Modify | `data/rolling_data_frame_test.go` | Add error propagation test for RollingDataFrame |
| Modify | `engine/engine.go` | Look-ahead guard in FetchAt |
| Modify | `engine/fetch_test.go` | Add look-ahead guard tests |
| Modify | `universe/universe.go` | Add `CurrentDate()` to Universe interface |
| Modify | `universe/static.go` | Implement `CurrentDate()` on StaticUniverse |
| Modify | `universe/universe_test.go` | Add CurrentDate test |
| Modify | `signal/signal.go` | Add signal constants |
| Rewrite | `signal/momentum.go` | Implement Momentum |
| Rewrite | `signal/volatility.go` | Implement Volatility |
| Rewrite | `signal/earnings_yield.go` | Implement EarningsYield |
| Create | `signal/signal_suite_test.go` | Ginkgo test suite bootstrap |
| Create | `signal/helpers_test.go` | Shared test mock types (mockDataSource, errorDataSource) |
| Create | `signal/momentum_test.go` | Momentum tests |
| Create | `signal/volatility_test.go` | Volatility tests |
| Create | `signal/earnings_yield_test.go` | EarningsYield tests |
| Modify | `docs/overview.md` | Fix signal examples |
| Modify | `docs/data.md` | Fix signal section |

---

## Chunk 1: DataFrame Error Accumulation

### Task 1: Add err field and WithErr helper to DataFrame

**Files:**
- Modify: `data/data_frame.go:38-55` (DataFrame struct)

- [ ] **Step 1: Add the err field to the DataFrame struct**

In `data/data_frame.go`, add `err error` to the struct:

```go
type DataFrame struct {
	data       []float64
	times      []time.Time
	assets     []asset.Asset
	metrics    []Metric
	assetIndex map[string]int
	err        error
}
```

- [ ] **Step 2: Add Err() accessor and exported WithErr helper**

Below the struct in `data/data_frame.go`, add:

```go
// Err returns the first error encountered during chained operations.
func (df *DataFrame) Err() error { return df.err }

// WithErr returns a zero-value DataFrame carrying the given error.
// All accessor methods (Len, AssetList, etc.) return safe defaults on this form.
// Exported so that packages like signal can create error DataFrames.
func WithErr(err error) *DataFrame {
	return &DataFrame{err: err}
}
```

Note: exported from the start so the signal package can use it.

- [ ] **Step 3: Verify the project compiles**

Run: `go build ./...`
Expected: compiles with no errors (new field is zero-valued by default, new methods are additive).

- [ ] **Step 4: Commit**

```bash
git add data/data_frame.go
git commit -m "feat(data): add err field, Err() accessor, and WithErr helper to DataFrame"
```

---

### Task 2: Add error short-circuits to ALL DataFrame methods

**Files:**
- Modify: `data/data_frame.go`

Every method that accesses `df.data`, `df.times`, `df.assets`, `df.metrics`, or `df.assetIndex` needs a guard. The pattern for methods returning `*DataFrame` is:

```go
func (df *DataFrame) SomeMethod(...) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}
	// ... existing logic ...
}
```

- [ ] **Step 1: Add short-circuits to all methods returning `*DataFrame`**

Add `if df.err != nil { return WithErr(df.err) }` at the top of each:

- `At`
- `Last`
- `Copy`
- `Assets`
- `Metrics`
- `Between`
- `Filter`
- `Drop`
- `AddScalar`
- `SubScalar`
- `MulScalar`
- `DivScalar`
- `MaxAcrossAssets`
- `MinAcrossAssets`
- `Mean`
- `Sum`
- `Max`
- `Min`
- `Variance`
- `Std`
- `Covariance`
- `Pct`
- `Diff`
- `Log`
- `CumSum`
- `Shift`
- `Apply`
- `Reduce`

- [ ] **Step 2: Add short-circuits to accessor methods that access nil-unsafe fields**

For `Value` and `ValueAt` (which access `df.assetIndex` map -- nil map panics on read):

```go
func (df *DataFrame) Value(a asset.Asset, m Metric) float64 {
	if df.err != nil {
		return math.NaN()
	}
	// ... existing ...
}

func (df *DataFrame) ValueAt(a asset.Asset, m Metric, t time.Time) float64 {
	if df.err != nil {
		return math.NaN()
	}
	// ... existing ...
}
```

For `Column` (accesses `df.assetIndex`):

```go
func (df *DataFrame) Column(a asset.Asset, m Metric) []float64 {
	if df.err != nil {
		return nil
	}
	// ... existing ...
}
```

For `IdxMaxAcrossAssets`:

```go
func (df *DataFrame) IdxMaxAcrossAssets() []asset.Asset {
	if df.err != nil {
		return nil
	}
	// ... existing ...
}
```

For `Table`:

```go
func (df *DataFrame) Table() string {
	if df.err != nil {
		return "(error DataFrame)"
	}
	// ... existing ...
}
```

Note: `Len()`, `ColCount()`, `Start()`, `End()`, `Duration()`, `Times()`, `AssetList()`, `MetricList()` are safe on nil slices (len(nil) == 0, range over nil is empty, copy from nil is fine). No guards needed.

- [ ] **Step 3: Verify the project compiles**

Run: `go build ./...`

- [ ] **Step 4: Run existing tests to ensure nothing broke**

Run: `go test ./data/... -v`
Expected: all existing tests pass (error field is nil by default).

- [ ] **Step 5: Commit**

```bash
git add data/data_frame.go
git commit -m "feat(data): add error short-circuits to all DataFrame methods"
```

---

### Task 3: Add error propagation to RollingDataFrame

**Files:**
- Modify: `data/rolling_data_frame.go`

- [ ] **Step 1: Add error guard to all RollingDataFrame methods**

Add `if r.df.err != nil { return WithErr(r.df.err) }` at the top of each method in `data/rolling_data_frame.go`:

- `Mean`
- `Sum`
- `Max`
- `Min`
- `Std`
- `Variance`
- `Percentile`

Note: these call `WithErr` from the `data` package (same package, so just `WithErr`).

- [ ] **Step 2: Verify the project compiles**

Run: `go build ./...`

- [ ] **Step 3: Commit**

```bash
git add data/rolling_data_frame.go
git commit -m "feat(data): propagate DataFrame errors through RollingDataFrame methods"
```

---

### Task 4: Change Add/Sub/Mul/Div signatures

**Files:**
- Modify: `data/data_frame.go:638-705`
- Modify: `data/data_frame_test.go:520-630`

- [ ] **Step 1: Change elemWiseOp to return `*DataFrame` instead of `(*DataFrame, error)`**

In `data/data_frame.go`, change `elemWiseOp`:

```go
func (df *DataFrame) elemWiseOp(other *DataFrame, apply func(dst, s, t []float64) []float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}
	if other.err != nil {
		return WithErr(other.err)
	}

	timeLen := len(df.times)
	if len(other.times) != timeLen {
		return WithErr(fmt.Errorf("elemWiseOp: timestamp count mismatch: %d vs %d", timeLen, len(other.times)))
	}

	for i := 0; i < timeLen; i++ {
		if !df.times[i].Equal(other.times[i]) {
			return WithErr(fmt.Errorf("elemWiseOp: timestamp mismatch at index %d: %s vs %s",
				i, df.times[i].Format(time.RFC3339), other.times[i].Format(time.RFC3339)))
		}
	}

	pairs, resAssets, resMetrics := df.findIntersection(other)
	if len(pairs) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	resIdx := make(map[string]int, len(resAssets))
	for i, a := range resAssets {
		resIdx[a.CompositeFigi] = i
	}

	resMIdx := make(map[Metric]int, len(resMetrics))
	for i, m := range resMetrics {
		resMIdx[m] = i
	}

	resMetricLen := len(resMetrics)
	newData := make([]float64, len(resAssets)*resMetricLen*timeLen)

	for _, p := range pairs {
		raIdx := resIdx[p.a.CompositeFigi]
		rmIdx := resMIdx[p.m]
		dstOff := (raIdx*resMetricLen + rmIdx) * timeLen
		dst := newData[dstOff : dstOff+timeLen]
		s := df.data[p.selfOff : p.selfOff+timeLen]
		t := other.data[p.otherOff : p.otherOff+timeLen]
		apply(dst, s, t)
	}

	times := make([]time.Time, timeLen)
	copy(times, df.times)

	return mustNewDataFrame(times, resAssets, resMetrics, newData)
}
```

- [ ] **Step 2: Change Add/Sub/Mul/Div to return `*DataFrame`**

```go
func (df *DataFrame) Add(other *DataFrame) *DataFrame {
	return df.elemWiseOp(other, floats.AddTo)
}

func (df *DataFrame) Sub(other *DataFrame) *DataFrame {
	return df.elemWiseOp(other, floats.SubTo)
}

func (df *DataFrame) Mul(other *DataFrame) *DataFrame {
	return df.elemWiseOp(other, floats.MulTo)
}

func (df *DataFrame) Div(other *DataFrame) *DataFrame {
	return df.elemWiseOp(other, floats.DivTo)
}
```

- [ ] **Step 3: Update all test callers in data/data_frame_test.go**

Change every `result, err := df.Add(other)` pattern to `result := df.Add(other)` and replace `Expect(err).NotTo(HaveOccurred())` with `Expect(result.Err()).NotTo(HaveOccurred())`. For error tests, change `_, err = df.Add(short)` / `Expect(err).To(HaveOccurred())` to `result = df.Add(short)` / `Expect(result.Err()).To(HaveOccurred())`.

Note: verified that no callers of `Add`/`Sub`/`Mul`/`Div` on DataFrame exist outside `data/data_frame_test.go`. Only `time.Add`/`time.Sub` calls appear elsewhere.

The tests to update (all in `data/data_frame_test.go`):
- Line 521: `result, err := df.Add(other)` and similar for Sub/Mul/Div
- Line 549, 561: Div tests
- Line 571, 583: partial overlap and no overlap tests
- Line 592, 604: error-expecting tests (timestamp mismatch)
- Line 612: immutability test `_, _ = df.Add(other)`
- Line 627: NaN propagation test

- [ ] **Step 4: Run tests**

Run: `go test ./data/... -v`
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add data/data_frame.go data/data_frame_test.go
git commit -m "refactor(data): change Add/Sub/Mul/Div to return *DataFrame with error accumulation"
```

---

### Task 5: Write error accumulation tests

**Files:**
- Modify: `data/data_frame_test.go`

- [ ] **Step 1: Write the tests**

Add a new `Describe("Error accumulation", ...)` block in `data/data_frame_test.go`:

```go
Describe("Error accumulation", func() {
	It("Err returns nil on a healthy DataFrame", func() {
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Err()).To(BeNil())
	})

	It("propagates error through Add chain", func() {
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})
		short, _ := data.NewDataFrame(times[:3], []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3})

		result := df.Add(short).AddScalar(1)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("timestamp count mismatch"))
	})

	It("propagates error through mixed chain", func() {
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})
		short, _ := data.NewDataFrame(times[:3], []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3})

		result := df.Add(short).MulScalar(2).Pct().Last()
		Expect(result.Err()).To(HaveOccurred())
	})

	It("successful chain has nil Err", func() {
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})
		other, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{10, 20, 30, 40, 50})

		result := df.Add(other).MulScalar(2)
		Expect(result.Err()).To(BeNil())
		Expect(result.Value(aapl, data.Price)).To(Equal(110.0)) // (5+50)*2
	})

	It("propagates error through Rolling", func() {
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})
		short, _ := data.NewDataFrame(times[:3], []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3})

		result := df.Add(short).Rolling(3).Mean()
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates other's error through Add", func() {
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})
		short, _ := data.NewDataFrame(times[:3], []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3})

		errDF := df.Add(short) // has error
		good, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})

		result := good.Add(errDF) // other has error
		Expect(result.Err()).To(HaveOccurred())
	})

	It("returns NaN from Value on error DataFrame", func() {
		errDF := data.WithErr(fmt.Errorf("test error"))
		Expect(math.IsNaN(errDF.Value(aapl, data.Price))).To(BeTrue())
	})

	It("returns nil from Column on error DataFrame", func() {
		errDF := data.WithErr(fmt.Errorf("test error"))
		Expect(errDF.Column(aapl, data.Price)).To(BeNil())
	})

	It("returns 0 from Len on error DataFrame", func() {
		errDF := data.WithErr(fmt.Errorf("test error"))
		Expect(errDF.Len()).To(Equal(0))
	})
})
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./data/... -v -run "Error accumulation"`
Expected: PASS (error propagation already implemented in prior tasks).

- [ ] **Step 3: Commit**

```bash
git add data/data_frame_test.go
git commit -m "test(data): add error accumulation tests for DataFrame chaining"
```

---

### Task 6: Add RenameMetric to DataFrame

**Files:**
- Modify: `data/data_frame.go`
- Modify: `data/data_frame_test.go`

- [ ] **Step 1: Write the failing tests**

Add a `Describe("RenameMetric", ...)` block in `data/data_frame_test.go`:

```go
Describe("RenameMetric", func() {
	It("renames a metric successfully", func() {
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})
		Expect(err).NotTo(HaveOccurred())

		result := df.RenameMetric(data.Price, data.Metric("Signal"))
		Expect(result.Err()).To(BeNil())
		Expect(result.MetricList()).To(Equal([]data.Metric{data.Metric("Signal")}))
		Expect(result.Value(aapl, data.Metric("Signal"))).To(Equal(5.0))
	})

	It("returns error when old metric not found", func() {
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})

		result := df.RenameMetric(data.Volume, data.Metric("Signal"))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("Volume"))
	})

	It("returns error when new metric already exists", func() {
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price, data.Volume},
			[]float64{1, 2, 3, 4, 5, 10, 20, 30, 40, 50})

		result := df.RenameMetric(data.Price, data.Volume)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("already exists"))
	})

	It("propagates existing error", func() {
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3, 4, 5})
		short, _ := data.NewDataFrame(times[:3], []asset.Asset{aapl}, []data.Metric{data.Price}, []float64{1, 2, 3})

		result := df.Add(short).RenameMetric(data.Price, data.Metric("Signal"))
		Expect(result.Err()).To(HaveOccurred())
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./data/... -v -run "RenameMetric"`
Expected: FAIL (RenameMetric not defined).

- [ ] **Step 3: Implement RenameMetric**

Add to `data/data_frame.go`:

```go
// RenameMetric returns a new DataFrame with metric old replaced by new.
// Returns a DataFrame with Err set if old is not found or new already exists.
func (df *DataFrame) RenameMetric(old, new Metric) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	oldIdx := -1
	for i, m := range df.metrics {
		if m == old {
			oldIdx = i
		}
		if m == new {
			return WithErr(fmt.Errorf("RenameMetric: metric %q already exists", new))
		}
	}
	if oldIdx == -1 {
		return WithErr(fmt.Errorf("RenameMetric: metric %q not found", old))
	}

	result := df.Copy()
	result.metrics[oldIdx] = new
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./data/... -v -run "RenameMetric"`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add RenameMetric method to DataFrame"
```

---

## Chunk 2: Engine Look-Ahead Guard and Universe CurrentDate

### Task 7: Add look-ahead guard to engine FetchAt

**Files:**
- Modify: `engine/engine.go:253-259`
- Modify: `engine/fetch_test.go`

Note: `Fetch` already sets `rangeEnd := e.currentDate` (line 233), so it cannot exceed the current date by construction. Only `FetchAt` needs a guard since it takes a caller-supplied `time.Time`.

- [ ] **Step 1: Write the failing test**

Add `futureFetchAtStrategy` type to `engine/fetch_test.go`:

```go
type futureFetchAtStrategy struct {
	metrics  []data.Metric
	assets   []asset.Asset
	fetched  *data.DataFrame
	fetchErr error
}

func (s *futureFetchAtStrategy) Name() string { return "futureFetchAtStrategy" }

func (s *futureFetchAtStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("futureFetchAtStrategy.Setup: %v", err))
	}
	eng.Schedule(tc)
}

func (s *futureFetchAtStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio) {
	futureDate := eng.CurrentDate().AddDate(0, 0, 30)
	s.fetched, s.fetchErr = eng.FetchAt(ctx, s.assets, futureDate, s.metrics)
}
```

Add the test inside the existing `Describe("Fetch", ...)`:

```go
Context("look-ahead guard", func() {
	It("FetchAt rejects a future date", func() {
		metrics := []data.Metric{data.MetricClose}
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		df := makeDailyDF(dataStart, 90, testAssets, metrics)
		provider := data.NewTestProvider(metrics, df)

		futureStrategy := &futureFetchAtStrategy{
			metrics: metrics,
			assets:  testAssets,
		}

		eng := engine.New(futureStrategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 2, 3, 23, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())
		Expect(futureStrategy.fetchErr).To(HaveOccurred())
		Expect(futureStrategy.fetchErr.Error()).To(ContainSubstring("future"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./engine/... -v -run "look-ahead"`
Expected: FAIL (FetchAt does not reject future dates yet).

- [ ] **Step 3: Implement the look-ahead guard in FetchAt**

In `engine/engine.go`, modify `FetchAt` (around line 254):

```go
func (e *Engine) FetchAt(ctx context.Context, assets []asset.Asset, t time.Time, metrics []data.Metric) (*data.DataFrame, error) {
	if !e.currentDate.IsZero() && t.After(e.currentDate) {
		return nil, fmt.Errorf("FetchAt: requested future date %s (current simulation date is %s)",
			t.Format("2006-01-02"), e.currentDate.Format("2006-01-02"))
	}

	if e.cache == nil {
		e.cache = newDataCache(e.cacheMaxBytes)
	}
	return e.fetchRange(ctx, assets, metrics, t, t)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./engine/... -v -run "look-ahead"`
Expected: PASS.

- [ ] **Step 5: Run all engine tests to verify nothing broke**

Run: `go test ./engine/... -v`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add engine/engine.go engine/fetch_test.go
git commit -m "feat(engine): add look-ahead guard to FetchAt"
```

---

### Task 8: Add CurrentDate to Universe interface and StaticUniverse

**Files:**
- Modify: `universe/universe.go:30-44`
- Modify: `universe/static.go`
- Modify: `universe/universe_test.go`

- [ ] **Step 1: Write the failing test**

Add to `universe/universe_test.go`, inside the existing `Describe("Static Universe", ...)`:

```go
Describe("CurrentDate", func() {
	It("delegates to the data source", func() {
		ds := &mockDataSource{currentDate: now, fetchResult: emptyDF}
		staticUniverse := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		Expect(staticUniverse.CurrentDate()).To(Equal(now))
	})

	It("returns zero time when no data source is set", func() {
		staticUniverse := universe.NewStatic("AAPL")

		Expect(staticUniverse.CurrentDate()).To(Equal(time.Time{}))
	})
})
```

Note: the existing `mockDataSource` in `universe_test.go` already has `CurrentDate() time.Time` so it satisfies the updated interface.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./universe/... -v -run "CurrentDate"`
Expected: FAIL (CurrentDate not defined on StaticUniverse).

- [ ] **Step 3: Add CurrentDate to Universe interface**

In `universe/universe.go`, add to the interface:

```go
type Universe interface {
	Assets(t time.Time) []asset.Asset
	Prefetch(ctx context.Context, start, end time.Time) error
	Window(ctx context.Context, lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error)
	At(ctx context.Context, t time.Time, metrics ...data.Metric) (*data.DataFrame, error)
	CurrentDate() time.Time
}
```

- [ ] **Step 4: Implement CurrentDate on StaticUniverse**

In `universe/static.go`, add:

```go
func (u *StaticUniverse) CurrentDate() time.Time {
	if u.ds == nil {
		return time.Time{}
	}
	return u.ds.CurrentDate()
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./universe/... -v`
Expected: PASS.

- [ ] **Step 6: Verify the full project compiles (catch any other Universe implementors)**

Run: `go build ./...`
Expected: compiles. If any other type implements Universe and is missing CurrentDate, the build will catch it.

- [ ] **Step 7: Commit**

```bash
git add universe/universe.go universe/static.go universe/universe_test.go
git commit -m "feat(universe): add CurrentDate to Universe interface"
```

---

## Chunk 3: Signal Functions

### Task 9: Add signal constants, test suite bootstrap, and shared test helpers

**Files:**
- Modify: `signal/signal.go`
- Create: `signal/signal_suite_test.go`
- Create: `signal/helpers_test.go`

- [ ] **Step 1: Update signal.go with constants**

Replace the contents of `signal/signal.go` (keep the copyright header):

```go
// Package signal provides reusable computations that derive new time
// series from DataFrame metrics. Each signal is a plain function that
// takes a DataFrame and returns a new DataFrame with one column per
// asset containing the computed signal score.
package signal

import "github.com/penny-vault/pvbt/data"

// Signal output names. These are typed as data.Metric so they can be
// used in DataFrame operations, but they represent computed signals,
// not raw market data. The Signal suffix avoids collision with the
// function names in this package.
const (
	MomentumSignal      data.Metric = "Momentum"
	VolatilitySignal    data.Metric = "Volatility"
	EarningsYieldSignal data.Metric = "EarningsYield"
)
```

- [ ] **Step 2: Create the Ginkgo test suite bootstrap**

Create `signal/signal_suite_test.go`:

```go
package signal_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSignal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Signal Suite")
}
```

- [ ] **Step 3: Create shared test helpers**

Create `signal/helpers_test.go`:

```go
package signal_test

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// mockDataSource implements universe.DataSource for signal tests.
type mockDataSource struct {
	currentDate time.Time
	fetchResult *data.DataFrame
}

func (m *mockDataSource) Fetch(_ context.Context, _ []asset.Asset, _ portfolio.Period, _ []data.Metric) (*data.DataFrame, error) {
	return m.fetchResult, nil
}

func (m *mockDataSource) FetchAt(_ context.Context, _ []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	return m.fetchResult, nil
}

func (m *mockDataSource) CurrentDate() time.Time { return m.currentDate }

// Compile-time check.
var _ universe.DataSource = (*mockDataSource)(nil)

// errorDataSource always returns an error.
type errorDataSource struct {
	err error
}

func (e *errorDataSource) Fetch(_ context.Context, _ []asset.Asset, _ portfolio.Period, _ []data.Metric) (*data.DataFrame, error) {
	return nil, e.err
}

func (e *errorDataSource) FetchAt(_ context.Context, _ []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	return nil, e.err
}

func (e *errorDataSource) CurrentDate() time.Time { return time.Time{} }

// Compile-time check.
var _ universe.DataSource = (*errorDataSource)(nil)
```

- [ ] **Step 4: Verify it compiles and the empty suite runs**

Run: `go test ./signal/... -v`
Expected: PASS (0 specs).

- [ ] **Step 5: Commit**

```bash
git add signal/signal.go signal/signal_suite_test.go signal/helpers_test.go
git commit -m "feat(signal): add signal constants, test suite bootstrap, and shared test helpers"
```

---

### Task 10: Implement Momentum signal

**Files:**
- Rewrite: `signal/momentum.go`
- Create: `signal/momentum_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/momentum_test.go`:

```go
package signal_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("Momentum", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		goog asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("computes percent change over the full window", func() {
		// AAPL prices: 100, 105, 110, 115, 120 => (120-100)/100 = 0.20
		// GOOG prices: 200, 190, 180, 170, 160 => (160-200)/200 = -0.20
		times := make([]time.Time, 5)
		for i := range times {
			times[i] = now.AddDate(0, 0, i-4)
		}
		vals := []float64{
			100, 105, 110, 115, 120, // AAPL/Close
			200, 190, 180, 170, 160, // GOOG/Close
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.MetricClose}, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.Momentum(ctx, u, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.MomentumSignal}))
		Expect(result.Value(aapl, signal.MomentumSignal)).To(BeNumerically("~", 0.20, 1e-10))
		Expect(result.Value(goog, signal.MomentumSignal)).To(BeNumerically("~", -0.20, 1e-10))
	})

	It("uses custom metric when provided", func() {
		times := make([]time.Time, 3)
		for i := range times {
			times[i] = now.AddDate(0, 0, i-2)
		}
		vals := []float64{50, 60, 75} // single asset AdjClose: (75-50)/50 = 0.50
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Momentum(ctx, u, portfolio.Days(2), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MomentumSignal)).To(BeNumerically("~", 0.50, 1e-10))
	})

	It("returns error on degenerate window (fewer than 2 rows)", func() {
		times := []time.Time{now}
		vals := []float64{100}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Momentum(ctx, u, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Momentum(ctx, u, portfolio.Days(10))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./signal/... -v -run "Momentum"`
Expected: FAIL (Momentum returns nil).

- [ ] **Step 3: Implement Momentum**

Rewrite `signal/momentum.go`:

```go
package signal

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// Momentum computes the percent change over the given period for each
// asset in the universe. Returns a single-row DataFrame with one column
// per asset containing the momentum score.
func Momentum(ctx context.Context, u universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := u.Window(ctx, period, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("Momentum: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("Momentum: need at least 2 data points, got %d", df.Len()))
	}

	return df.Pct(df.Len() - 1).Last().RenameMetric(metric, MomentumSignal)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./signal/... -v -run "Momentum"`
Expected: PASS.

- [ ] **Step 5: Run all tests to verify nothing broke**

Run: `go test ./... -count=1`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add signal/momentum.go signal/momentum_test.go
git commit -m "feat(signal): implement Momentum signal function"
```

---

### Task 11: Implement Volatility signal

**Files:**
- Rewrite: `signal/volatility.go`
- Create: `signal/volatility_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/volatility_test.go`:

```go
package signal_test

import (
	"context"
	"errors"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("Volatility", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("computes rolling std dev of returns", func() {
		// Prices: 100, 110, 105, 115, 120
		// Returns (Pct(1)): NaN, 0.10, -0.04545, 0.09524, 0.04348
		// After Drop(NaN): 0.10, -0.04545, 0.09524, 0.04348
		// Std of those 4 returns (sample, N-1)
		times := make([]time.Time, 5)
		for i := range times {
			times[i] = now.AddDate(0, 0, i-4)
		}
		vals := []float64{100, 110, 105, 115, 120}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Volatility(ctx, u, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.VolatilitySignal}))

		// Hand-compute expected std dev.
		returns := []float64{
			(110.0 - 100.0) / 100.0,
			(105.0 - 110.0) / 110.0,
			(115.0 - 105.0) / 105.0,
			(120.0 - 115.0) / 115.0,
		}
		mean := 0.0
		for _, r := range returns {
			mean += r
		}
		mean /= float64(len(returns))
		variance := 0.0
		for _, r := range returns {
			d := r - mean
			variance += d * d
		}
		variance /= float64(len(returns) - 1)
		expectedStd := math.Sqrt(variance)

		Expect(result.Value(aapl, signal.VolatilitySignal)).To(BeNumerically("~", expectedStd, 1e-10))
	})

	It("uses custom metric when provided", func() {
		times := make([]time.Time, 4)
		for i := range times {
			times[i] = now.AddDate(0, 0, i-3)
		}
		vals := []float64{50, 55, 52, 58}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Volatility(ctx, u, portfolio.Days(3), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.VolatilitySignal}))
	})

	It("returns error on degenerate window (fewer than 3 rows)", func() {
		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := []float64{100, 110}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Volatility(ctx, u, portfolio.Days(1))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("timeout")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Volatility(ctx, u, portfolio.Days(10))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("timeout"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./signal/... -v -run "Volatility"`
Expected: FAIL.

- [ ] **Step 3: Implement Volatility**

Rewrite `signal/volatility.go`:

```go
package signal

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// Volatility computes the rolling standard deviation of returns over
// the given period for each asset in the universe. Returns a single-row
// DataFrame with one column per asset containing the volatility score.
func Volatility(ctx context.Context, u universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := u.Window(ctx, period, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("Volatility: %w", err))
	}

	if df.Len() < 3 {
		return data.WithErr(fmt.Errorf("Volatility: need at least 3 data points, got %d", df.Len()))
	}

	// Compute daily returns, drop leading NaN, then rolling std over all returns.
	returns := df.Pct(1).Drop(math.NaN())
	result := returns.Rolling(returns.Len()).Std().Last().RenameMetric(metric, VolatilitySignal)
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./signal/... -v -run "Volatility"`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add signal/volatility.go signal/volatility_test.go
git commit -m "feat(signal): implement Volatility signal function"
```

---

### Task 12: Implement EarningsYield signal

**Files:**
- Rewrite: `signal/earnings_yield.go`
- Create: `signal/earnings_yield_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/earnings_yield_test.go`:

```go
package signal_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("EarningsYield", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		goog asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("computes EPS divided by Price", func() {
		// AAPL: EPS=5, Price=100 => yield = 0.05
		// GOOG: EPS=10, Price=200 => yield = 0.05
		times := []time.Time{now}
		vals := []float64{
			5,   // AAPL/EarningsPerShare
			100, // AAPL/Price
			10,  // GOOG/EarningsPerShare
			200, // GOOG/Price
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog},
			[]data.Metric{data.EarningsPerShare, data.Price}, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.EarningsYield(ctx, u)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.EarningsYieldSignal}))
		Expect(result.Value(aapl, signal.EarningsYieldSignal)).To(BeNumerically("~", 0.05, 1e-10))
		Expect(result.Value(goog, signal.EarningsYieldSignal)).To(BeNumerically("~", 0.05, 1e-10))
	})

	It("uses explicit time when provided", func() {
		explicitTime := time.Date(2025, 3, 1, 16, 0, 0, 0, time.UTC)
		times := []time.Time{explicitTime}
		vals := []float64{8, 160} // EPS=8, Price=160 => 0.05
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.EarningsPerShare, data.Price}, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.EarningsYield(ctx, u, explicitTime)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.EarningsYieldSignal)).To(BeNumerically("~", 0.05, 1e-10))
	})

	It("returns error when EarningsPerShare metric is missing", func() {
		times := []time.Time{now}
		vals := []float64{100} // only Price, no EPS
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.EarningsYield(ctx, u)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("EarningsPerShare"))
	})

	It("returns error when Price metric is missing", func() {
		times := []time.Time{now}
		vals := []float64{5} // only EPS, no Price
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.EarningsPerShare}, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.EarningsYield(ctx, u)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("Price"))
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("db down")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.EarningsYield(ctx, u)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("db down"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./signal/... -v -run "EarningsYield"`
Expected: FAIL.

- [ ] **Step 3: Implement EarningsYield**

Rewrite `signal/earnings_yield.go`:

```go
package signal

import (
	"context"
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/universe"
)

// EarningsYield computes earnings per share divided by price for each
// asset in the universe. Returns a single-row DataFrame with one column
// per asset containing the earnings yield.
func EarningsYield(ctx context.Context, u universe.Universe, t ...time.Time) *data.DataFrame {
	fetchTime := u.CurrentDate()
	if len(t) > 0 {
		fetchTime = t[0]
	}

	df, err := u.At(ctx, fetchTime, data.EarningsPerShare, data.Price)
	if err != nil {
		return data.WithErr(fmt.Errorf("EarningsYield: %w", err))
	}

	// Validate required metrics are present.
	epsDF := df.Metrics(data.EarningsPerShare)
	if epsDF.ColCount() == 0 {
		return data.WithErr(fmt.Errorf("EarningsYield: missing EarningsPerShare metric"))
	}

	priceDF := df.Metrics(data.Price)
	if priceDF.ColCount() == 0 {
		return data.WithErr(fmt.Errorf("EarningsYield: missing Price metric"))
	}

	// Build result manually to avoid the Div metric-intersection problem.
	// (Two DataFrames with different metrics would have an empty intersection.)
	assets := df.AssetList()
	times := df.Times()
	resultData := make([]float64, len(assets))

	for i, a := range assets {
		eps := df.ValueAt(a, data.EarningsPerShare, times[0])
		price := df.ValueAt(a, data.Price, times[0])
		resultData[i] = eps / price
	}

	result, buildErr := data.NewDataFrame(times, assets, []data.Metric{EarningsYieldSignal}, resultData)
	if buildErr != nil {
		return data.WithErr(fmt.Errorf("EarningsYield: %w", buildErr))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./signal/... -v -run "EarningsYield"`
Expected: PASS.

- [ ] **Step 5: Run all signal tests**

Run: `go test ./signal/... -v`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add signal/earnings_yield.go signal/earnings_yield_test.go
git commit -m "feat(signal): implement EarningsYield signal function"
```

---

## Chunk 4: Documentation Updates and Final Verification

### Task 13: Update documentation

**Files:**
- Modify: `docs/overview.md`
- Modify: `docs/data.md`

- [ ] **Step 1: Update docs/overview.md**

In `docs/overview.md`, the Compute example (around lines 50-58 and 118-128) currently shows:

```go
mom1 := signal.Momentum(s.RiskOn, Months(1))
```

Change all signal calls to include `ctx`:

```go
mom1 := signal.Momentum(ctx, s.RiskOn, Months(1))
mom3 := signal.Momentum(ctx, s.RiskOn, Months(3))
mom6 := signal.Momentum(ctx, s.RiskOn, Months(6))
```

The chain `momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)` is already correct with the new single-return `Add`. Add an error check:

```go
momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
if err := momentum.Err(); err != nil {
    log.Error().Err(err).Msg("signal computation failed")
    return
}
```

Fix the Select call to match actual code:

```go
symbols := portfolio.MaxAboveZero(s.RiskOff).Select(momentum)
```

- [ ] **Step 2: Update docs/data.md signals section**

In `docs/data.md` (around lines 305-340), the signal examples currently show:

```go
mom3 := signal.Momentum(df, 3)
```

Update to match the new signatures:

```go
mom3 := signal.Momentum(ctx, riskOn, portfolio.Months(3))
mom6 := signal.Momentum(ctx, riskOn, portfolio.Months(6))
mom12 := signal.Momentum(ctx, riskOn, portfolio.Months(12))

composite := mom3.Add(mom6).Add(mom12).DivScalar(3)
if err := composite.Err(); err != nil {
    // handle error
}
```

Update the custom signal example:

```go
func BookToPrice(ctx context.Context, u universe.Universe) *data.DataFrame {
    df, err := u.At(ctx, u.CurrentDate(), data.BookValue, data.Price)
    if err != nil {
        return data.WithErr(err)
    }
    // ... manual computation like EarningsYield ...
}
```

Also update the sentence "If a required metric is missing, the signal panics" to "If a required metric is missing, the signal returns a DataFrame with `.Err()` set."

- [ ] **Step 3: Commit**

```bash
git add docs/overview.md docs/data.md
git commit -m "docs: update signal examples with ctx parameter and Err() checking"
```

---

### Task 14: Full test suite verification

- [ ] **Step 1: Run the full test suite**

Run: `go test ./... -count=1`
Expected: all tests pass.

- [ ] **Step 2: Run go vet**

Run: `go vet ./...`
Expected: no issues.

- [ ] **Step 3: Build the example to verify it still compiles**

Run: `go build ./examples/momentum-rotation/...`
Expected: compiles (this example does not use the signal package directly yet, but import paths must resolve).
