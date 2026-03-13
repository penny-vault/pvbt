# Metric Helpers Refactor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace unexported metric helpers with proper PerformanceMetric implementations, consolidate Account's raw slices into a DataFrame, and move pure math to the data package.

**Architecture:** Account stores equity/benchmark/risk-free data in a single `perfData` DataFrame. PerformanceMetrics compose with each other (Returns, DrawdownSeries, ExcessReturns) instead of calling unexported helpers. Pure statistical functions (SliceMean, Variance, Stddev, Covariance) live in `data/stats.go`.

**Tech Stack:** Go, ginkgo v2 + gomega for tests, gonum for float operations.

**Spec:** `docs/superpowers/specs/2026-03-12-metric-helpers-refactor-design.md`

---

## Chunk 1: Foundation (data package + Account internals)

### Task 1: Add portfolio metric constants to data package

**Files:**
- Modify: `data/metric.go`

- [ ] **Step 1: Add the constants**

Add to `data/metric.go` after the existing metric groups:

```go
// Portfolio performance tracking metrics.
const (
	PortfolioEquity    Metric = "PortfolioEquity"
	PortfolioBenchmark Metric = "PortfolioBenchmark"
	PortfolioRiskFree  Metric = "PortfolioRiskFree"
)
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./data/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```
git add data/metric.go
git commit -m "feat(data): add PortfolioEquity, PortfolioBenchmark, PortfolioRiskFree metric constants"
```

---

### Task 2: Create data/stats.go with pure math functions

**Files:**
- Create: `data/stats.go`
- Create: `data/stats_test.go`

- [ ] **Step 1: Write failing tests for SliceMean**

Create `data/stats_test.go` with black-box ginkgo tests (`package data_test`):

```go
package data_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("Stats", func() {
	Describe("SliceMean", func() {
		It("returns 0 for empty slice", func() {
			Expect(data.SliceMean([]float64{})).To(BeNumerically("==", 0))
		})

		It("returns 0 for nil slice", func() {
			Expect(data.SliceMean(nil)).To(BeNumerically("==", 0))
		})

		It("returns the single value for one-element slice", func() {
			Expect(data.SliceMean([]float64{7.5})).To(BeNumerically("~", 7.5, 1e-12))
		})

		It("computes arithmetic mean for multiple elements", func() {
			Expect(data.SliceMean([]float64{1, 2, 3, 4})).To(BeNumerically("~", 2.5, 1e-12))
		})
	})

	Describe("Variance", func() {
		It("returns 0 for empty input", func() {
			Expect(data.Variance([]float64{})).To(BeNumerically("==", 0))
		})

		It("returns 0 for single element", func() {
			Expect(data.Variance([]float64{42})).To(BeNumerically("==", 0))
		})

		It("computes correct sample variance", func() {
			// [1, 2, 3], mean=2, sum of sq diffs = 2, var = 2/2 = 1.0
			Expect(data.Variance([]float64{1, 2, 3})).To(BeNumerically("~", 1.0, 1e-12))
		})

		It("returns 0 for identical values", func() {
			Expect(data.Variance([]float64{5, 5, 5, 5})).To(BeNumerically("==", 0))
		})
	})

	Describe("Stddev", func() {
		It("returns 0 for empty input", func() {
			Expect(data.Stddev([]float64{})).To(BeNumerically("==", 0))
		})

		It("is the square root of the variance", func() {
			Expect(data.Stddev([]float64{1, 2, 3})).To(BeNumerically("~", 1.0, 1e-12))
		})
	})

	Describe("Covariance", func() {
		It("returns 0 for empty inputs", func() {
			Expect(data.Covariance([]float64{}, []float64{})).To(BeNumerically("==", 0))
		})

		It("returns 0 for single-element inputs", func() {
			Expect(data.Covariance([]float64{5}, []float64{10})).To(BeNumerically("==", 0))
		})

		It("trims to the shorter array", func() {
			x := []float64{1, 2, 3}
			y := []float64{2, 4}
			Expect(data.Covariance(x, y)).To(BeNumerically("~", 1.0, 1e-12))
		})

		It("computes correct sample covariance for perfect linear relationship", func() {
			x := []float64{1, 2, 3, 4, 5}
			y := []float64{2, 4, 6, 8, 10}
			Expect(data.Covariance(x, y)).To(BeNumerically("~", 5.0, 1e-12))
		})
	})

	Describe("AnnualizationFactor", func() {
		It("returns 252 for daily timestamps", func() {
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, 10)
			for i := range times {
				times[i] = start.AddDate(0, 0, i)
			}
			Expect(data.AnnualizationFactor(times)).To(BeNumerically("==", 252))
		})

		It("returns 12 for monthly timestamps", func() {
			start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, 6)
			for i := range times {
				times[i] = start.AddDate(0, i, 0)
			}
			Expect(data.AnnualizationFactor(times)).To(BeNumerically("==", 12))
		})

		It("defaults to 252 for a single timestamp", func() {
			times := []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
			Expect(data.AnnualizationFactor(times)).To(BeNumerically("==", 252))
		})

		It("defaults to 252 for empty slice", func() {
			Expect(data.AnnualizationFactor(nil)).To(BeNumerically("==", 252))
		})

		It("returns 252 for gap of exactly 20 days (boundary)", func() {
			t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t1 := time.Date(2024, 1, 21, 0, 0, 0, 0, time.UTC)
			Expect(data.AnnualizationFactor([]time.Time{t0, t1})).To(BeNumerically("==", 252))
		})

		It("returns 12 for gap of 21 days", func() {
			t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t1 := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
			Expect(data.AnnualizationFactor([]time.Time{t0, t1})).To(BeNumerically("==", 12))
		})
	})

	Describe("PeriodsReturns", func() {
		It("returns empty slice for single element", func() {
			Expect(data.PeriodsReturns([]float64{100})).To(HaveLen(0))
		})

		It("returns empty slice for empty input", func() {
			Expect(data.PeriodsReturns([]float64{})).To(HaveLen(0))
		})

		It("computes period-over-period returns", func() {
			r := data.PeriodsReturns([]float64{100, 110, 99})
			Expect(r).To(HaveLen(2))
			Expect(r[0]).To(BeNumerically("~", 0.10, 1e-12))
			Expect(r[1]).To(BeNumerically("~", -0.1, 1e-12))
		})

		It("handles zero values safely", func() {
			r := data.PeriodsReturns([]float64{100, 0})
			Expect(r).To(HaveLen(1))
			Expect(r[0]).To(BeNumerically("~", -1.0, 1e-12))
		})

		It("returns +Inf for zero-to-positive", func() {
			r := data.PeriodsReturns([]float64{0, 100})
			Expect(r).To(HaveLen(1))
			Expect(math.IsInf(r[0], 1)).To(BeTrue())
		})
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `ginkgo run ./data/...`
Expected: FAIL -- `SliceMean`, `Variance`, etc. not defined

- [ ] **Step 3: Write the implementations**

Create `data/stats.go`:

```go
package data

import (
	"math"
	"time"
)

// SliceMean computes the arithmetic mean of a float64 slice.
// Returns 0 for empty or nil input.
func SliceMean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range x {
		sum += v
	}
	return sum / float64(len(x))
}

// Variance computes the sample variance (N-1 denominator).
// Returns 0 for fewer than 2 elements.
func Variance(x []float64) float64 {
	n := len(x)
	if n < 2 {
		return 0
	}
	m := SliceMean(x)
	sum := 0.0
	for _, v := range x {
		d := v - m
		sum += d * d
	}
	return sum / float64(n-1)
}

// Stddev computes the sample standard deviation (N-1 denominator).
func Stddev(x []float64) float64 {
	return math.Sqrt(Variance(x))
}

// Covariance computes the sample covariance between x and y.
// Trims to the shorter of the two slices. Returns 0 for fewer than 2 pairs.
func Covariance(x, y []float64) float64 {
	n := len(x)
	if len(y) < n {
		n = len(y)
	}
	if n < 2 {
		return 0
	}
	mx := SliceMean(x[:n])
	my := SliceMean(y[:n])
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += (x[i] - mx) * (y[i] - my)
	}
	return sum / float64(n-1)
}

// AnnualizationFactor estimates periods-per-year from timestamps.
// If the average gap exceeds 20 calendar days, returns 12 (monthly);
// otherwise returns 252 (daily). Defaults to 252 for fewer than 2 timestamps.
func AnnualizationFactor(times []time.Time) float64 {
	if len(times) < 2 {
		return 252
	}
	avgDays := times[len(times)-1].Sub(times[0]).Hours() / 24 / float64(len(times)-1)
	if avgDays > 20 {
		return 12
	}
	return 252
}

// PeriodsReturns computes period-over-period returns from a price series.
// Returns a slice of length len(prices)-1. Returns empty slice for fewer than 2 prices.
func PeriodsReturns(prices []float64) []float64 {
	if len(prices) < 2 {
		return []float64{}
	}
	r := make([]float64, len(prices)-1)
	for i := 0; i < len(prices)-1; i++ {
		r[i] = (prices[i+1] - prices[i]) / prices[i]
	}
	return r
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `ginkgo run ./data/...`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```
git add data/stats.go data/stats_test.go
git commit -m "feat(data): add SliceMean, Variance, Stddev, Covariance, AnnualizationFactor, PeriodsReturns"
```

---

### Task 3: Refactor Account to use perfData DataFrame

**Files:**
- Modify: `portfolio/account.go`
- Modify: `portfolio/snapshot.go`

This task replaces the four separate slices (`equityCurve`, `equityTimes`, `benchmarkPrices`, `riskFreePrices`) with a single `perfData *data.DataFrame` and rewrites `UpdatePrices` to build single-row DataFrames and merge them.

- [ ] **Step 1: Add portfolioAsset and PerfData accessor**

In `portfolio/account.go`, add the synthetic asset variable and the `PerfData()` method. Replace the four slice fields with `perfData`:

```go
// portfolioAsset is a synthetic asset used as the key in the perfData DataFrame.
var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}
```

In the `Account` struct, replace:
```go
equityCurve     []float64
equityTimes     []time.Time
benchmarkPrices []float64
riskFreePrices  []float64
```
with:
```go
perfData *data.DataFrame
```

Add accessor:
```go
// PerfData returns the performance tracking DataFrame containing
// PortfolioEquity, PortfolioBenchmark, and PortfolioRiskFree columns.
func (a *Account) PerfData() *data.DataFrame { return a.perfData }
```

- [ ] **Step 2: Add windowedPerfData method**

```go
// windowedPerfData returns the perfData DataFrame trimmed to the given window.
// If window is nil, returns the full perfData.
func (a *Account) windowedPerfData(window *Period) *data.DataFrame {
	if window == nil || a.perfData == nil {
		return a.perfData
	}
	end := a.perfData.End()
	var start time.Time
	switch window.Unit {
	case UnitDay:
		start = end.AddDate(0, 0, -window.N)
	case UnitMonth:
		start = end.AddDate(0, -window.N, 0)
	case UnitYear:
		start = end.AddDate(-window.N, 0, 0)
	}
	return a.perfData.Between(start, end)
}
```

- [ ] **Step 3: Rewrite UpdatePrices**

Replace the existing `UpdatePrices` method with:

```go
func (a *Account) UpdatePrices(df *data.DataFrame) {
	a.prices = df

	total := a.cash
	for ast, qty := range a.holdings {
		v := df.Value(ast, data.MetricClose)
		if !math.IsNaN(v) {
			total += qty * v
		}
	}

	var benchVal, rfVal float64
	if a.benchmark != (asset.Asset{}) {
		v := df.Value(a.benchmark, data.AdjClose)
		if !math.IsNaN(v) {
			benchVal = v
		}
	}
	if a.riskFree != (asset.Asset{}) {
		v := df.Value(a.riskFree, data.AdjClose)
		if !math.IsNaN(v) {
			rfVal = v
		}
	}

	t := []time.Time{df.End()}
	assets := []asset.Asset{portfolioAsset}
	metrics := []data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree}
	row, err := data.NewDataFrame(t, assets, metrics, []float64{total, benchVal, rfVal})
	if err != nil {
		return
	}

	if a.perfData == nil {
		a.perfData = row
	} else {
		merged, err := data.MergeTimes(a.perfData, row)
		if err != nil {
			return
		}
		a.perfData = merged
	}
}
```

- [ ] **Step 4: Remove old accessor methods**

Delete these methods from `account.go`:
- `EquityCurve() []float64`
- `EquityTimes() []time.Time`
- `BenchmarkPrices() []float64`
- `RiskFreePrices() []float64`

- [ ] **Step 5: Update PortfolioSnapshot interface and WithPortfolioSnapshot**

In `portfolio/snapshot.go`, change the interface from:
```go
EquityCurve() []float64
EquityTimes() []time.Time
BenchmarkPrices() []float64
RiskFreePrices() []float64
```
to:
```go
PerfData() *data.DataFrame
```

Update `WithPortfolioSnapshot`:
```go
func WithPortfolioSnapshot(snap PortfolioSnapshot) Option {
	return func(a *Account) {
		a.cash = snap.Cash()
		snap.Holdings(func(ast asset.Asset, qty float64) {
			a.holdings[ast] = qty
		})
		a.transactions = append(a.transactions, snap.Transactions()...)
		a.perfData = snap.PerfData()
		for ast, lots := range snap.TaxLots() {
			a.taxLots[ast] = append(a.taxLots[ast], lots...)
		}
	}
}
```

- [ ] **Step 6: Verify compilation**

Run: `go build ./portfolio/...`
Expected: FAIL -- many metric files still reference removed methods. This is expected; those files are updated in subsequent tasks.

- [ ] **Step 7: Commit (with --no-verify if build fails due to downstream)**

Do NOT commit yet -- wait until Task 4-8 are done so the tree compiles.

---

### Task 4: Create Returns PerformanceMetric

**Files:**
- Create: `portfolio/returns.go`

- [ ] **Step 1: Create returns.go**

```go
package portfolio

import (
	"github.com/penny-vault/pvbt/data"
)

type returnsMetric struct{}

func (returnsMetric) Name() string { return "Returns" }

func (returnsMetric) Description() string {
	return "Period-over-period returns derived from the equity curve."
}

func (returnsMetric) Compute(a *Account, window *Period) (float64, error) {
	perfDF := a.windowedPerfData(window)
	eq := perfDF.Column(portfolioAsset, data.PortfolioEquity)
	if len(eq) < 2 {
		return 0, nil
	}
	return (eq[len(eq)-1] - eq[0]) / eq[0], nil
}

func (returnsMetric) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	perfDF := a.windowedPerfData(window)
	eq := perfDF.Column(portfolioAsset, data.PortfolioEquity)
	if len(eq) < 2 {
		return nil, nil
	}
	r := make([]float64, len(eq)-1)
	for i := range r {
		r[i] = (eq[i+1] - eq[i]) / eq[i]
	}
	return r, nil
}

// Returns computes period-over-period returns from the portfolio equity curve.
var Returns PerformanceMetric = returnsMetric{}
```

- [ ] **Step 2: Verify it compiles in isolation**

Run: `go vet ./portfolio/returns.go` (may fail due to other broken files -- that's OK)

---

### Task 5: Create DrawdownSeries PerformanceMetric

**Files:**
- Create: `portfolio/drawdown_series.go`

- [ ] **Step 1: Create drawdown_series.go**

```go
package portfolio

import (
	"math"

	"github.com/penny-vault/pvbt/data"
)

type drawdownSeriesMetric struct{}

func (drawdownSeriesMetric) Name() string { return "DrawdownSeries" }

func (drawdownSeriesMetric) Description() string {
	return "Drawdown at each point in the equity curve, as negative fractions from peak."
}

func (d drawdownSeriesMetric) Compute(a *Account, window *Period) (float64, error) {
	series, err := d.ComputeSeries(a, window)
	if err != nil || len(series) == 0 {
		return 0, err
	}
	return series[len(series)-1], nil
}

func (drawdownSeriesMetric) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	perfDF := a.windowedPerfData(window)
	eq := perfDF.Column(portfolioAsset, data.PortfolioEquity)
	dd := make([]float64, len(eq))
	peak := math.Inf(-1)
	for i, v := range eq {
		if v > peak {
			peak = v
		}
		dd[i] = (v - peak) / peak
	}
	return dd, nil
}

// DrawdownSeries computes the drawdown at each point in the equity curve.
var DrawdownSeries PerformanceMetric = drawdownSeriesMetric{}
```

---

### Task 6: Create ExcessReturns PerformanceMetric

**Files:**
- Create: `portfolio/excess_returns.go`

- [ ] **Step 1: Create excess_returns.go**

```go
package portfolio

import (
	"github.com/penny-vault/pvbt/data"
)

type excessReturnsMetric struct{}

func (excessReturnsMetric) Name() string { return "ExcessReturns" }

func (excessReturnsMetric) Description() string {
	return "Portfolio returns minus risk-free returns, element-wise."
}

func (e excessReturnsMetric) Compute(a *Account, window *Period) (float64, error) {
	series, err := e.ComputeSeries(a, window)
	if err != nil || len(series) == 0 {
		return 0, err
	}
	return data.SliceMean(series), nil
}

func (excessReturnsMetric) ComputeSeries(a *Account, window *Period) ([]float64, error) {
	perfDF := a.windowedPerfData(window)
	eq := perfDF.Column(portfolioAsset, data.PortfolioEquity)
	rf := perfDF.Column(portfolioAsset, data.PortfolioRiskFree)

	r := data.PeriodsReturns(eq)
	rfr := data.PeriodsReturns(rf)

	n := len(r)
	if len(rfr) < n {
		n = len(rfr)
	}
	er := make([]float64, n)
	for i := 0; i < n; i++ {
		er[i] = r[i] - rfr[i]
	}
	return er, nil
}

// ExcessReturns computes portfolio returns minus risk-free returns.
var ExcessReturns PerformanceMetric = excessReturnsMetric{}
```

---

### Task 7: Strip metric_helpers.go down to transaction helpers only

**Files:**
- Modify: `portfolio/metric_helpers.go`

- [ ] **Step 1: Remove all functions except roundTrips and realizedGains**

Delete these functions from `metric_helpers.go`:
- `returns`
- `excessReturns`
- `windowSlice`
- `windowSliceTimes`
- `annualizationFactor`
- `drawdownSeries`
- `cagr`
- `mean`
- `variance`
- `stddev`
- `covariance`

Keep:
- `roundTrip` type
- `roundTrips` function
- `realizedGains` function

Remove unused imports (the `math` and `time` imports that were needed by the deleted functions may no longer be needed -- check).

---

### Task 8: Delete export shims and internal tests

**Files:**
- Delete: `portfolio/export_test.go`
- Delete: `portfolio/metric_helpers_test.go`

- [ ] **Step 1: Delete both files**

```
rm portfolio/export_test.go portfolio/metric_helpers_test.go
```

---

## Chunk 2: Migrate all ~45 metric implementations

Each metric file needs the same pattern applied. The migration depends on which helpers the file uses. Read each file, apply the transformation, and move on.

**Common patterns:**

Pattern A -- files that used `windowSlice(a.EquityCurve(), a.EquityTimes(), window)` + `returns(eq)`:
```go
// Before:
eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
r := returns(eq)

// After:
r, err := Returns.ComputeSeries(a, window)
if err != nil { return 0, err }
```

Pattern B -- files that used `excessReturns`:
```go
// Before:
eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
r := returns(eq)
rf := returns(windowSlice(a.RiskFreePrices(), a.EquityTimes(), window))
er := excessReturns(r, rf)

// After:
er, err := ExcessReturns.ComputeSeries(a, window)
if err != nil { return 0, err }
```

Pattern C -- files that used `drawdownSeries`:
```go
// Before:
eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
dd := drawdownSeries(eq)

// After:
dd, err := DrawdownSeries.ComputeSeries(a, window)
if err != nil { return 0, err }
```

Pattern D -- files that used `mean`, `stddev`, `variance`, `covariance`:
```go
// Before:           // After:
mean(x)              data.SliceMean(x)
stddev(x)            data.Stddev(x)
variance(x)          data.Variance(x)
covariance(x, y)     data.Covariance(x, y)
```

Pattern E -- files that used `annualizationFactor`:
```go
// Before:
af := annualizationFactor(a.EquityTimes())

// After:
perfDF := a.windowedPerfData(window)
af := data.AnnualizationFactor(perfDF.Times())
```

Pattern F -- files that used `windowSlice` on equity directly (without returns):
```go
// Before:
equity := windowSlice(a.EquityCurve(), a.EquityTimes(), window)

// After:
perfDF := a.windowedPerfData(window)
equity := perfDF.Column(portfolioAsset, data.PortfolioEquity)
```

Pattern G -- files that used benchmark data:
```go
// Before:
if len(a.BenchmarkPrices()) == 0 { return 0, nil }
bm := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)
bReturns := returns(bm)

// After:
perfDF := a.windowedPerfData(window)
bm := perfDF.Column(portfolioAsset, data.PortfolioBenchmark)
if len(bm) == 0 { return 0, nil }
bReturns := data.PeriodsReturns(bm)
```

Pattern H -- files that used risk-free data directly (not via excessReturns):
```go
// Before:
if len(a.RiskFreePrices()) == 0 { return 0, nil }
rf := windowSlice(a.RiskFreePrices(), a.EquityTimes(), window)

// After:
perfDF := a.windowedPerfData(window)
rf := perfDF.Column(portfolioAsset, data.PortfolioRiskFree)
if len(rf) == 0 { return 0, nil }
```

### Task 9: Migrate ExcessReturns-based metrics (8 files)

**Files:** `portfolio/sharpe.go`, `portfolio/sortino.go`, `portfolio/smart_sharpe.go`, `portfolio/smart_sortino.go`, `portfolio/probabilistic_sharpe.go`, `portfolio/downside_deviation.go`, `portfolio/information_ratio.go`, `portfolio/tracking_error.go`

These files use patterns B + D + E. Read each file, apply the patterns. Note that `information_ratio.go` and `tracking_error.go` use `excessReturns(pReturns, bReturns)` for active returns (portfolio vs benchmark, not portfolio vs risk-free), so they need `data.PeriodsReturns` on both series and inline subtraction, not `ExcessReturns.ComputeSeries`.

- [ ] **Step 1: Read and migrate each file**

For each file:
1. Read the current implementation
2. Replace `windowSlice`/`returns`/`excessReturns`/`mean`/`stddev`/`annualizationFactor`/`EquityCurve`/`EquityTimes`/`RiskFreePrices`/`BenchmarkPrices` calls per the patterns above
3. Update imports: add `"github.com/penny-vault/pvbt/data"`, remove unused imports

- [ ] **Step 2: Verify compilation**

Run: `go build ./portfolio/...`
Expected: may still fail if other files aren't migrated yet

---

### Task 10: Migrate DrawdownSeries-based metrics (6 files)

**Files:** `portfolio/max_drawdown.go`, `portfolio/avg_drawdown.go`, `portfolio/avg_drawdown_days.go`, `portfolio/calmar.go`, `portfolio/keller_ratio.go`, `portfolio/recovery_factor.go`

These use pattern C + possibly D. `calmar.go` also uses `cagr()` -- inline the computation (see spec section 6).

- [ ] **Step 1: Read and migrate each file**

For each file, apply patterns C, D, F as needed. For `calmar.go`, replace `cagr(eq[0], eq[len(eq)-1], years)` with inline `math.Pow(eq[len(eq)-1]/eq[0], 1.0/years) - 1`.

- [ ] **Step 2: Migrate cagr_metric.go**

Replace `cagr()` call with inline computation per spec section 6.

---

### Task 11: Migrate simple windowSlice+returns metrics (21 files)

**Files:** `portfolio/twrr.go`, `portfolio/std_dev.go`, `portfolio/consecutive_wins.go`, `portfolio/consecutive_losses.go`, `portfolio/cvar.go`, `portfolio/excess_kurtosis.go`, `portfolio/exposure.go`, `portfolio/gain_loss_ratio.go`, `portfolio/gain_to_pain.go`, `portfolio/k_ratio.go`, `portfolio/kelly_criterion.go`, `portfolio/n_positive_periods.go`, `portfolio/omega_ratio.go`, `portfolio/skewness.go`, `portfolio/tail_ratio.go`, `portfolio/value_at_risk.go`, `portfolio/dynamic_withdrawal_rate.go`, `portfolio/perpetual_withdrawal_rate.go`, `portfolio/safe_withdrawal_rate.go`, `portfolio/ulcer_index.go`, `portfolio/treynor.go`

These use patterns A + D + F + possibly H. Apply the transformations.

- [ ] **Step 1: Read and migrate each file**

For each file:
1. Replace `windowSlice(a.EquityCurve(), a.EquityTimes(), window)` + `returns()` with `Returns.ComputeSeries(a, window)` or `a.windowedPerfData(window)` + `perfDF.Column(...)` as appropriate
2. Replace `mean`/`stddev`/`variance`/`covariance` with `data.SliceMean`/`data.Stddev`/`data.Variance`/`data.Covariance`
3. Files that need the raw equity (not returns) like `ulcer_index.go`, `dynamic_withdrawal_rate.go`, `perpetual_withdrawal_rate.go`, `safe_withdrawal_rate.go` use pattern F
4. `treynor.go` uses risk-free data directly -- use pattern H
5. Update imports

---

### Task 12: Migrate benchmark-based metrics (8 files)

**Files:** `portfolio/active_return.go`, `portfolio/alpha.go`, `portfolio/beta.go`, `portfolio/r_squared.go`, `portfolio/downside_capture.go`, `portfolio/upside_capture.go`

Note: `information_ratio.go` and `tracking_error.go` were already migrated in Task 9.

These use patterns G + D + possibly A. Apply the transformations.

- [ ] **Step 1: Read and migrate each file**

For each file:
1. Replace `a.BenchmarkPrices()` checks and `windowSlice(a.BenchmarkPrices(), ...)` with `perfDF.Column(portfolioAsset, data.PortfolioBenchmark)`
2. Replace `returns(bm)` with `data.PeriodsReturns(bm)`
3. Replace `variance`/`covariance`/`stddev`/`mean` with `data.` equivalents
4. `alpha.go` also uses risk-free data -- use pattern H
5. Update imports

---

### Task 13: Verify full compilation and run tests

- [ ] **Step 1: Build the portfolio package**

Run: `go build ./portfolio/...`
Expected: SUCCESS

- [ ] **Step 2: Run all portfolio tests**

Run: `ginkgo run ./portfolio/...`
Expected: ALL PASS (some tests may need updates in Task 14)

- [ ] **Step 3: Build and test full project**

Run: `go build ./...`
Run: `ginkgo run ./...`
Expected: ALL PASS

- [ ] **Step 4: Commit all changes**

```
git add -A
git commit -m "refactor(portfolio): replace metric helpers with PerformanceMetric composition and data.stats

- Account stores perfData DataFrame instead of separate equityCurve/benchmarkPrices/riskFreePrices slices
- Returns, DrawdownSeries, ExcessReturns promoted to PerformanceMetric implementations
- Pure math (SliceMean, Variance, Stddev, Covariance, AnnualizationFactor) moved to data/stats.go
- Deleted cagr helper; CAGR metric computes inline
- Deleted export_test.go and metric_helpers_test.go (violate black-box testing)
- All ~45 metric files migrated to use composition and data.stats functions"
```

---

## Chunk 3: Update tests and callers

### Task 14: Update account_test.go

**Files:**
- Modify: `portfolio/account_test.go`

- [ ] **Step 1: Read account_test.go and identify all references to removed methods**

Search for: `EquityCurve()`, `EquityTimes()`, `BenchmarkPrices()`, `RiskFreePrices()`

- [ ] **Step 2: Replace with PerfData() equivalents**

```go
// Before:
a.EquityCurve()
a.EquityTimes()
a.BenchmarkPrices()
a.RiskFreePrices()

// After:
a.PerfData().Column(portfolioAsset, data.PortfolioEquity)
a.PerfData().Times()
a.PerfData().Column(portfolioAsset, data.PortfolioBenchmark)
a.PerfData().Column(portfolioAsset, data.PortfolioRiskFree)
```

Note: `portfolioAsset` is unexported, so test code in `package portfolio_test` cannot access it directly. The tests should use `PerfData()` and access columns via the exported metric constants. Check if the test needs to reference `portfolioAsset` -- if so, the test may need to assert on the DataFrame's column list or use `PerfData().Column()` with a locally constructed asset matching the CompositeFigi.

Alternative: export `PortfolioAsset` as a public var, or provide `EquityCurve()` as a convenience wrapper. Check what the tests actually need.

- [ ] **Step 3: Run portfolio tests**

Run: `ginkgo run ./portfolio/...`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```
git add portfolio/account_test.go
git commit -m "test(portfolio): update account tests to use PerfData DataFrame"
```

---

### Task 15: Add tests for new PerformanceMetrics

**Files:**
- Modify: `portfolio/return_metrics_test.go` (or create new test file)

- [ ] **Step 1: Add Returns tests**

Add to an appropriate test file (e.g., `portfolio/return_metrics_test.go`):

```go
Describe("Returns", func() {
	It("computes total return over full history", func() {
		// Build account with known equity curve
		// Assert Returns.Compute gives expected total return
	})

	It("computes period-over-period return series", func() {
		// Assert Returns.ComputeSeries gives expected values
	})
})
```

Build the Account using `portfolio.New()` + `UpdatePrices()` calls to create a known equity curve.

- [ ] **Step 2: Add DrawdownSeries tests**

```go
Describe("DrawdownSeries", func() {
	It("returns all zeros for rising equity", func() { ... })
	It("computes correct drawdown for peak-trough pattern", func() { ... })
})
```

- [ ] **Step 3: Add ExcessReturns tests**

```go
Describe("ExcessReturns", func() {
	It("subtracts risk-free returns from portfolio returns", func() { ... })
})
```

- [ ] **Step 4: Run tests**

Run: `ginkgo run ./portfolio/...`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```
git add portfolio/return_metrics_test.go
git commit -m "test(portfolio): add black-box tests for Returns, DrawdownSeries, ExcessReturns"
```

---

### Task 16: Final verification

- [ ] **Step 1: Run full test suite**

Run: `ginkgo run ./...`
Expected: ALL PASS

- [ ] **Step 2: Verify no remaining references to deleted helpers**

Run: `grep -rn 'windowSlice\|windowSliceTimes\|annualizationFactor\|excessReturns\|drawdownSeries\| cagr(' portfolio/*.go | grep -v _test.go`
Expected: NO OUTPUT (all references removed)

Run: `grep -rn 'EquityCurve()\|EquityTimes()\|BenchmarkPrices()\|RiskFreePrices()' portfolio/*.go cli/*.go`
Expected: NO OUTPUT (all references removed)

- [ ] **Step 3: Verify export_test.go is gone**

Run: `ls portfolio/export_test.go`
Expected: "No such file or directory"

- [ ] **Step 4: Final commit if any stragglers**

```
git add -A
git commit -m "chore: clean up remaining references after metric helpers refactor"
```
