# Metric Helpers Refactor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace unexported metric helpers with DataFrame method chains and consolidate Account's 4 parallel slices into a single `perfData *data.DataFrame`.

**Architecture:** Move Period to `data` package, add `Window`, `CumMax`, `AppendRow` to DataFrame, extend `Sub/Add/Mul/Div` with variadic metric broadcast, replace Account's `equityCurve`/`equityTimes`/`benchmarkPrices`/`riskFreePrices` with `perfData`, migrate all ~43 metric files from helper calls to DataFrame chains, update SQLite serialization.

**Tech Stack:** Go, ginkgo v2 + gomega testing, gonum/floats, gonum/stat, modernc.org/sqlite

---

## Chunk 1: Data Package Foundation

### Task 1: Period type in data package

**Files:**
- Create: `data/period.go`
- Create: `data/period_test.go`
- Modify: `portfolio/period.go`

- [ ] **Step 1: Write failing test for Period.Before with UnitDay**

Note: The `data` package already has a test suite bootstrap in `data/data_suite_test.go`. Add these tests to the existing `data/data_frame_test.go` file (or a new `data/period_test.go` if the existing file is already large). Do NOT add another `TestXxx` bootstrap function.

```go
// Add to existing test suite in data/ package
// data/period_test.go (same package data_test, no new bootstrap needed)
package data_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("Period", func() {
	ref := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

	Describe("Before", func() {
		It("subtracts days", func() {
			p := data.Days(10)
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC)))
		})

		It("subtracts months", func() {
			p := data.Months(2)
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)))
		})

		It("subtracts years", func() {
			p := data.Years(1)
			Expect(p.Before(ref)).To(Equal(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)))
		})

		It("returns Jan 1 for YTD", func() {
			p := data.YTD()
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)))
		})

		It("returns 1st of month for MTD", func() {
			p := data.MTD()
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)))
		})

		It("returns most recent Monday for WTD", func() {
			// 2025-03-15 is a Saturday; most recent Monday is 2025-03-10
			p := data.WTD()
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)))
		})

		It("returns ref for WTD when ref is Monday", func() {
			monday := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
			p := data.WTD()
			Expect(p.Before(monday)).To(Equal(monday))
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -run Period -v`
Expected: FAIL -- `data.Days` not defined

- [ ] **Step 3: Write Period type and Before method in data/period.go**

```go
// data/period.go
package data

import "time"

// PeriodUnit identifies the calendar unit of a Period.
type PeriodUnit int

const (
	UnitDay PeriodUnit = iota
	UnitMonth
	UnitYear
	UnitYTD // year-to-date: from Jan 1 of the current year
	UnitMTD // month-to-date: from the 1st of the current month
	UnitWTD // week-to-date: from the most recent Monday
)

// Period represents a calendar-aware duration used for performance metric
// windows. Unlike time.Duration, it handles variable-length units like
// months and years correctly.
type Period struct {
	N    int
	Unit PeriodUnit
}

// Days returns a Period of n calendar days.
func Days(n int) Period { return Period{N: n, Unit: UnitDay} }

// Months returns a Period of n calendar months.
func Months(n int) Period { return Period{N: n, Unit: UnitMonth} }

// Years returns a Period of n calendar years.
func Years(n int) Period { return Period{N: n, Unit: UnitYear} }

// YTD returns a Period representing year-to-date.
func YTD() Period { return Period{N: 0, Unit: UnitYTD} }

// MTD returns a Period representing month-to-date.
func MTD() Period { return Period{N: 0, Unit: UnitMTD} }

// WTD returns a Period representing week-to-date.
func WTD() Period { return Period{N: 0, Unit: UnitWTD} }

// Before computes the start date for a trailing window ending at ref.
func (p Period) Before(ref time.Time) time.Time {
	switch p.Unit {
	case UnitDay:
		return ref.AddDate(0, 0, -p.N)
	case UnitMonth:
		return ref.AddDate(0, -p.N, 0)
	case UnitYear:
		return ref.AddDate(-p.N, 0, 0)
	case UnitYTD:
		return time.Date(ref.Year(), 1, 1, 0, 0, 0, 0, ref.Location())
	case UnitMTD:
		return time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, ref.Location())
	case UnitWTD:
		offset := int(ref.Weekday()) - int(time.Monday)
		if offset < 0 {
			offset += 7
		}
		return ref.AddDate(0, 0, -offset)
	default:
		return ref
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -run Period -v`
Expected: PASS

- [ ] **Step 5: Replace portfolio/period.go with type aliases**

Replace contents of `portfolio/period.go` with:

```go
package portfolio

import "github.com/penny-vault/pvbt/data"

// Period is an alias for data.Period to maintain API compatibility.
type Period = data.Period

// PeriodUnit is an alias for data.PeriodUnit.
type PeriodUnit = data.PeriodUnit

const (
	UnitDay   = data.UnitDay
	UnitMonth = data.UnitMonth
	UnitYear  = data.UnitYear
	UnitYTD   = data.UnitYTD
	UnitMTD   = data.UnitMTD
	UnitWTD   = data.UnitWTD
)

var (
	Days   = data.Days
	Months = data.Months
	Years  = data.Years
	YTD    = data.YTD
	MTD    = data.MTD
	WTD    = data.WTD
)
```

- [ ] **Step 6: Verify existing tests still pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ ./portfolio/ -count=1`
Expected: PASS (no behavioral change)

- [ ] **Step 7: Commit**

```bash
git add data/period.go data/period_test.go portfolio/period.go
git commit -m "feat(data): move Period type to data package with Before method"
```

---

### Task 2: DataFrame.Window

**Files:**
- Modify: `data/data_frame.go` (add Window method)
- Modify: `data/data_frame_test.go` (add Window tests)

- [ ] **Step 1: Write failing tests for Window**

Add to `data/data_frame_test.go`:

```go
Describe("Window", func() {
	var df *data.DataFrame

	BeforeEach(func() {
		times := []time.Time{
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC),
		}
		assets := []asset.Asset{{CompositeFigi: "SPY", Ticker: "SPY"}}
		metrics := []data.Metric{data.MetricClose}
		vals := []float64{100, 110, 120, 130, 140}
		var err error
		df, err = data.NewDataFrame(times, assets, metrics, vals)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns full DataFrame when window is nil", func() {
		result := df.Window(nil)
		Expect(result.Len()).To(Equal(5))
	})

	It("trims to last 2 months", func() {
		w := data.Months(2)
		result := df.Window(&w)
		// End is 2025-05-01, Before = 2025-03-01, so includes Mar, Apr, May
		Expect(result.Len()).To(Equal(3))
	})

	It("returns full DataFrame when window exceeds data", func() {
		w := data.Years(10)
		result := df.Window(&w)
		Expect(result.Len()).To(Equal(5))
	})

	It("propagates error", func() {
		errDF := data.WithErr(fmt.Errorf("test error"))
		w := data.Months(1)
		result := errDF.Window(&w)
		Expect(result.Err()).To(HaveOccurred())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -run Window -v`
Expected: FAIL -- `Window` not defined

- [ ] **Step 3: Implement DataFrame.Window**

Add to `data/data_frame.go`:

```go
// Window returns a DataFrame containing only timestamps within the
// trailing window defined by p. When p is nil, returns the full DataFrame.
// When the window exceeds the available data, returns the full DataFrame.
func (df *DataFrame) Window(p *Period) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}
	if p == nil {
		return df.Copy()
	}
	if len(df.times) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}
	start := p.Before(df.End())
	if !start.After(df.Start()) {
		return df.Copy()
	}
	return df.Between(start, df.End())
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -run Window -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add DataFrame.Window for Period-based windowing"
```

---

### Task 3: DataFrame.CumMax

**Files:**
- Modify: `data/data_frame.go` (add CumMax method)
- Modify: `data/data_frame_test.go` (add CumMax tests)

- [ ] **Step 1: Write failing test for CumMax**

Add to `data/data_frame_test.go`:

```go
Describe("CumMax", func() {
	It("computes running maximum per column", func() {
		times := []time.Time{
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		assets := []asset.Asset{spy}
		metrics := []data.Metric{data.MetricClose}
		vals := []float64{100, 120, 110, 130}
		df, err := data.NewDataFrame(times, assets, metrics, vals)
		Expect(err).NotTo(HaveOccurred())

		result := df.CumMax()
		col := result.Column(spy, data.MetricClose)
		Expect(col).To(Equal([]float64{100, 120, 120, 130}))
	})

	It("handles single value", func() {
		times := []time.Time{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, []float64{50})
		Expect(err).NotTo(HaveOccurred())

		result := df.CumMax()
		Expect(result.Column(spy, data.MetricClose)).To(Equal([]float64{50}))
	})

	It("propagates error", func() {
		errDF := data.WithErr(fmt.Errorf("test error"))
		result := errDF.CumMax()
		Expect(result.Err()).To(HaveOccurred())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -run CumMax -v`
Expected: FAIL

- [ ] **Step 3: Implement CumMax**

Add to `data/data_frame.go` near `CumSum`:

```go
// CumMax returns the running maximum along the time axis for each column.
func (df *DataFrame) CumMax() *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	return df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		if len(col) == 0 {
			return out
		}
		out[0] = col[0]
		for i := 1; i < len(col); i++ {
			if col[i] > out[i-1] {
				out[i] = col[i]
			} else {
				out[i] = out[i-1]
			}
		}
		return out
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -run CumMax -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add DataFrame.CumMax for running maximum"
```

---

### Task 4: DataFrame.AppendRow

**Files:**
- Modify: `data/data_frame.go` (add AppendRow method)
- Modify: `data/data_frame_test.go` (add AppendRow tests)

- [ ] **Step 1: Write failing tests for AppendRow**

Add to `data/data_frame_test.go`:

```go
Describe("AppendRow", func() {
	It("appends a row to a single-column DataFrame", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, []float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		err = df.AppendRow(t2, []float64{110})
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Len()).To(Equal(2))
		Expect(df.Column(spy, data.MetricClose)).To(Equal([]float64{100, 110}))
		Expect(df.End()).To(Equal(t2))
	})

	It("appends rows with multiple columns", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		metrics := []data.Metric{data.MetricClose, data.PortfolioEquity}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy}, metrics, []float64{100, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		err = df.AppendRow(t2, []float64{110, 220})
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Column(spy, data.MetricClose)).To(Equal([]float64{100, 110}))
		Expect(df.Column(spy, data.PortfolioEquity)).To(Equal([]float64{200, 220}))
	})

	It("rejects non-chronological timestamp", func() {
		t1 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, []float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		err = df.AppendRow(t0, []float64{90})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("chronological"))
	})

	It("rejects wrong values length", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, []float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		err = df.AppendRow(t2, []float64{110, 220})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("values length"))
	})

	It("does not affect prior Window snapshots", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, []float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		snapshot := df.Window(nil)
		Expect(snapshot.Len()).To(Equal(1))

		err = df.AppendRow(t2, []float64{110})
		Expect(err).NotTo(HaveOccurred())

		// Snapshot should be unaffected
		Expect(snapshot.Len()).To(Equal(1))
		Expect(df.Len()).To(Equal(2))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -run AppendRow -v`
Expected: FAIL

- [ ] **Step 3: Implement AppendRow**

Add to `data/data_frame.go`:

```go
// AppendRow appends a single timestamp and its column values to the
// DataFrame in place. The values slice must have length len(assets) *
// len(metrics), ordered as [asset0_metric0, asset0_metric1, ...,
// asset1_metric0, ...] (matching column-major layout). Returns an error
// if the timestamp is not after the current End() or if the values
// length is wrong.
//
// AppendRow is the only DataFrame method that mutates in place. This is
// safe because all read methods produce independent copies via make+copy.
func (df *DataFrame) AppendRow(t time.Time, values []float64) error {
	if df.err != nil {
		return df.err
	}

	colCount := len(df.assets) * len(df.metrics)
	if len(values) != colCount {
		return fmt.Errorf("AppendRow: values length %d does not match column count %d", len(values), colCount)
	}

	if len(df.times) > 0 && !t.After(df.End()) {
		return fmt.Errorf("AppendRow: timestamp %s is not after current End() %s (must be chronological)",
			t.Format(time.RFC3339), df.End().Format(time.RFC3339))
	}

	// The data slab is column-major: each column is contiguous.
	// To append one row, we need to insert one value at the end of each
	// column. This requires rebuilding the slab because columns shift.
	oldT := len(df.times)
	newT := oldT + 1
	metricLen := len(df.metrics)

	newData := make([]float64, colCount*newT)
	for aIdx := 0; aIdx < len(df.assets); aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			oldOff := (aIdx*metricLen + mIdx) * oldT
			newOff := (aIdx*metricLen + mIdx) * newT
			copy(newData[newOff:newOff+oldT], df.data[oldOff:oldOff+oldT])
			newData[newOff+oldT] = values[aIdx*metricLen+mIdx]
		}
	}

	df.data = newData
	df.times = append(df.times, t)

	return nil
}
```

Note: This implementation rebuilds the slab each time because the column-major layout means columns are interleaved. For a backtest of ~5000 trading days with 3 columns, each rebuild copies ~15K floats, which is fast.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -run AppendRow -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add DataFrame.AppendRow for in-place row append"
```

---

### Task 5: Broadcast Sub/Add/Mul/Div

**Files:**
- Modify: `data/data_frame.go` (extend Sub, Add, Mul, Div signatures)
- Modify: `data/data_frame_test.go` (add broadcast tests)

- [ ] **Step 1: Write failing test for broadcast Sub**

Add to `data/data_frame_test.go`:

```go
Describe("Broadcast Sub", func() {
	It("subtracts a selected metric column from all columns", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		// df has metric PortfolioEquity with values [10, 20]
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2}, []asset.Asset{spy},
			[]data.Metric{data.PortfolioEquity}, []float64{10, 20},
		)
		Expect(err).NotTo(HaveOccurred())

		// other has metrics PortfolioEquity and PortfolioRiskFree
		other, err := data.NewDataFrame(
			[]time.Time{t1, t2}, []asset.Asset{spy},
			[]data.Metric{data.PortfolioEquity, data.PortfolioRiskFree},
			[]float64{10, 20, 1, 2},
		)
		Expect(err).NotTo(HaveOccurred())

		// Broadcast: subtract PortfolioRiskFree column from all df columns
		result := df.Sub(other, data.PortfolioRiskFree)
		Expect(result.Err()).NotTo(HaveOccurred())
		col := result.Column(spy, data.PortfolioEquity)
		Expect(col).To(Equal([]float64{9, 18}))
	})

	It("chains multiple metrics sequentially", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		df, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.PortfolioEquity}, []float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		other, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree},
			[]float64{100, 5, 3},
		)
		Expect(err).NotTo(HaveOccurred())

		// (100 - 5) - 3 = 92
		result := df.Sub(other, data.PortfolioBenchmark, data.PortfolioRiskFree)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Column(spy, data.PortfolioEquity)).To(Equal([]float64{92}))
	})

	It("falls back to intersection when no metrics specified", func() {
		t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

		a, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, []float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		b, err := data.NewDataFrame(
			[]time.Time{t1}, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, []float64{30},
		)
		Expect(err).NotTo(HaveOccurred())

		result := a.Sub(b)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Column(spy, data.MetricClose)).To(Equal([]float64{70}))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -run "Broadcast Sub" -v`
Expected: FAIL -- `Sub` does not accept variadic metric args

- [ ] **Step 3: Implement broadcast extension**

Modify `Sub`, `Add`, `Mul`, `Div` in `data/data_frame.go`. Change their signatures to accept variadic metrics:

```go
// Sub returns a new DataFrame with element-wise subtraction. When metrics
// are specified, selects those metric columns from other and broadcasts
// each against all columns of df. Multiple metrics chain sequentially.
// When no metrics are specified, uses intersection matching (original behavior).
func (df *DataFrame) Sub(other *DataFrame, metrics ...Metric) *DataFrame {
	if len(metrics) == 0 {
		return df.elemWiseOp(other, floats.SubTo)
	}
	return df.broadcastOp(other, metrics, floats.SubTo)
}

// Add returns a new DataFrame with element-wise addition.
func (df *DataFrame) Add(other *DataFrame, metrics ...Metric) *DataFrame {
	if len(metrics) == 0 {
		return df.elemWiseOp(other, floats.AddTo)
	}
	return df.broadcastOp(other, metrics, floats.AddTo)
}

// Mul returns a new DataFrame with element-wise multiplication.
func (df *DataFrame) Mul(other *DataFrame, metrics ...Metric) *DataFrame {
	if len(metrics) == 0 {
		return df.elemWiseOp(other, floats.MulTo)
	}
	return df.broadcastOp(other, metrics, floats.MulTo)
}

// Div returns a new DataFrame with element-wise division.
func (df *DataFrame) Div(other *DataFrame, metrics ...Metric) *DataFrame {
	if len(metrics) == 0 {
		return df.elemWiseOp(other, floats.DivTo)
	}
	return df.broadcastOp(other, metrics, floats.DivTo)
}
```

Add `broadcastOp` helper:

```go
// broadcastOp selects specified metric columns from other and applies
// the operation against every column in df. Multiple metrics chain
// sequentially with accumulated results. Result retains df's column
// structure.
func (df *DataFrame) broadcastOp(other *DataFrame, metrics []Metric, apply func(dst, s, t []float64) []float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}
	if other.err != nil {
		return WithErr(other.err)
	}

	timeLen := len(df.times)
	if len(other.times) != timeLen {
		return WithErr(fmt.Errorf("broadcastOp: timestamp count mismatch: %d vs %d", timeLen, len(other.times)))
	}

	for i := 0; i < timeLen; i++ {
		if !df.times[i].Equal(other.times[i]) {
			return WithErr(fmt.Errorf("broadcastOp: timestamp mismatch at index %d", i))
		}
	}

	// Start with a copy of df's data.
	result := df.Copy()

	for _, m := range metrics {
		mIdx, ok := other.metricIndex(m)
		if !ok {
			continue
		}

		// For each asset in other that also exists in df, broadcast
		// that metric column against all df columns for that asset.
		for aIdx := 0; aIdx < len(result.assets); aIdx++ {
			otherAIdx, ok := other.assetIndex[result.assets[aIdx].CompositeFigi]
			if !ok {
				continue
			}

			otherCol := other.colSlice(otherAIdx, mIdx)

			for rMIdx := 0; rMIdx < len(result.metrics); rMIdx++ {
				off := result.colOffset(aIdx, rMIdx)
				dst := result.data[off : off+timeLen]
				apply(dst, dst, otherCol)
			}
		}
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -run "Broadcast Sub" -v`
Expected: PASS

- [ ] **Step 5: Run all data tests to verify no regressions**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./data/ -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): extend Sub/Add/Mul/Div with variadic metric broadcast"
```

---

## Chunk 2: Account Consolidation and Interface Changes

### Task 6: Add perfData to Account (keep old accessors temporarily)

**Files:**
- Modify: `portfolio/account.go`
- Modify: `portfolio/account_test.go`

This task adds `perfData` alongside the existing fields. Old accessors (`EquityCurve`, `EquityTimes`, `BenchmarkPrices`, `RiskFreePrices`) are kept until all consumers are migrated (Task 15). This avoids breaking compilation during the transition.

- [ ] **Step 1: Add portfolioAsset var and perfData field**

In `portfolio/account.go`, add `portfolioAsset` package-level var, `perfData` field to Account, and `PerfData()` accessor. Add `"github.com/rs/zerolog/log"` import.

```go
var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}
```

Add to Account struct:
```go
perfData *data.DataFrame
```

Add accessor:
```go
// PerfData returns the accumulated performance DataFrame, or nil if no
// prices have been recorded yet.
func (a *Account) PerfData() *data.DataFrame { return a.perfData }
```

- [ ] **Step 2: Update UpdatePrices to build perfData in addition to old fields**

Add perfData construction to UpdatePrices AFTER the existing append calls (keep old behavior for now):

```go
// ... existing appends to equityCurve, equityTimes, benchmarkPrices, riskFreePrices ...

// Build perfData in parallel with old fields.
var benchVal, rfVal float64
if a.benchmark != (asset.Asset{}) {
	benchVal = a.benchmarkPrices[len(a.benchmarkPrices)-1]
}
if a.riskFree != (asset.Asset{}) {
	rfVal = a.riskFreePrices[len(a.riskFreePrices)-1]
}

if a.perfData == nil {
	t := []time.Time{df.End()}
	assets := []asset.Asset{portfolioAsset}
	metrics := []data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree}
	row, err := data.NewDataFrame(t, assets, metrics, []float64{total, benchVal, rfVal})
	if err != nil {
		log.Error().Err(err).Msg("UpdatePrices: failed to create perfData")
		return
	}
	a.perfData = row
} else {
	if err := a.perfData.AppendRow(df.End(), []float64{total, benchVal, rfVal}); err != nil {
		log.Error().Err(err).Msg("UpdatePrices: failed to append to perfData")
		return
	}
}
```

- [ ] **Step 3: Add PerfData tests to account_test.go**

Add new test cases that verify `PerfData()` returns correct values alongside the existing assertions. Do NOT modify existing assertions yet.

```go
perfAsset := asset.Asset{CompositeFigi: "_PORTFOLIO_", Ticker: "_PORTFOLIO_"}

// After UpdatePrices:
pd := a.PerfData()
Expect(pd).NotTo(BeNil())
Expect(pd.Len()).To(Equal(1))
Expect(pd.Column(perfAsset, data.PortfolioEquity)).To(Equal([]float64{10_000.0}))
```

- [ ] **Step 4: Verify tests compile and pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run Account -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go
git commit -m "feat(portfolio): add perfData DataFrame alongside existing fields"
```

---

## Chunk 3: Metric Migration

### Task 7: Delete old helpers and cleanup

**Files:**
- Modify: `portfolio/metric_helpers.go`

- [ ] **Step 1: Trim metric_helpers.go**

Remove all functions except `roundTrips`, `realizedGains`, `roundTrip` struct, and `annualizationFactor`. Remove: `periodCutoff`, `windowSlice`, `windowSliceTimes`, `returns`, `excessReturns`, `mean`, `variance`, `stddev`, `covariance`, `cagr`, `drawdownSeries`.

```bash
rm portfolio/export_test.go portfolio/metric_helpers_test.go
```

- [ ] **Step 3: Delete data/stats.go and data/stats_test.go**

```bash
rm data/stats.go data/stats_test.go
```

- [ ] **Step 4: Do NOT commit yet** -- metrics won't compile until migrated.

---

### Task 9: Migrate equity-only metrics

These metrics use `windowSlice(a.EquityCurve(), a.EquityTimes(), window)` then `returns(eq)`.

**New pattern:**
```go
pd := a.PerfData()
if pd == nil { return 0, nil }
eq := pd.Window(window).Metrics(data.PortfolioEquity)
r := eq.Pct().Drop(math.NaN())
if r.Len() == 0 { return 0, nil }
col := r.Column(portfolioAsset, data.PortfolioEquity)
```

**Files to modify** (each follows the same transformation):
- `portfolio/twrr.go`
- `portfolio/tail_ratio.go`
- `portfolio/n_positive_periods.go`
- `portfolio/excess_kurtosis.go`
- `portfolio/skewness.go`
- `portfolio/consecutive_wins.go`
- `portfolio/consecutive_losses.go`
- `portfolio/omega_ratio.go`
- `portfolio/cvar.go`
- `portfolio/value_at_risk.go`
- `portfolio/kelly_criterion.go`
- `portfolio/gain_loss_ratio.go`
- `portfolio/gain_to_pain.go`
- `portfolio/exposure.go`
- `portfolio/k_ratio.go`

For each file:

- [ ] **Step 1: Migrate the metric to DataFrame chains**

Replace `windowSlice(a.EquityCurve(), a.EquityTimes(), window)` with `pd.Window(window).Metrics(data.PortfolioEquity)` where `pd := a.PerfData()`.

Replace `returns(eq)` with `eq.Pct().Drop(math.NaN())`.

Where the metric needs raw `[]float64` values (e.g., for sorting, iteration), extract via `col := r.Column(portfolioAsset, data.PortfolioEquity)`.

Replace `mean(x)` on raw slices with `stat.Mean(x, nil)` (from `gonum.org/v1/gonum/stat`).

Replace `stddev(x)` on raw slices with `stat.StdDev(x, nil)`.

Replace `variance(x)` on raw slices with `stat.Variance(x, nil)`.

Add `"github.com/penny-vault/pvbt/data"` and `"gonum.org/v1/gonum/stat"` imports as needed.

**Example: twrr.go Compute:**
```go
func (twrr) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}
	eq := pd.Window(window).Metrics(data.PortfolioEquity)
	r := eq.Pct().Drop(math.NaN())
	if r.Len() == 0 {
		return 0, nil
	}
	col := r.Column(portfolioAsset, data.PortfolioEquity)
	product := 1.0
	for _, v := range col {
		product *= (1 + v)
	}
	return product - 1, nil
}
```

**Example: gain_loss_ratio.go** -- splits returns into positive/negative and calls mean on each:
```go
col := r.Column(portfolioAsset, data.PortfolioEquity)
var positive, negative []float64
for _, v := range col {
	if v > 0 { positive = append(positive, v) }
	if v < 0 { negative = append(negative, v) }
}
if len(positive) == 0 || len(negative) == 0 { return 0, nil }
return stat.Mean(positive, nil) / math.Abs(stat.Mean(negative, nil)), nil
```

**Example: excess_kurtosis.go** -- uses mean and stddev on raw slices:
```go
col := r.Column(portfolioAsset, data.PortfolioEquity)
s := stat.StdDev(col, nil)
if s == 0 { return 0, nil }
m := stat.Mean(col, nil)
// ... kurtosis formula using col, m, s
```

- [ ] **Step 2: Verify these files compile**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./portfolio/...`
Expected: Some metrics still won't compile (benchmark/risk-free ones). The equity-only ones should compile.

---

### Task 10: Migrate drawdown metrics

These metrics use `drawdownSeries(eq)`.

**New pattern:**
```go
pd := a.PerfData()
if pd == nil { return 0, nil }
eq := pd.Window(window).Metrics(data.PortfolioEquity)
if eq.Len() == 0 { return 0, nil }
peak := eq.CumMax()
dd := eq.Sub(peak).Div(peak)
ddCol := dd.Column(portfolioAsset, data.PortfolioEquity)
```

**Files:**
- `portfolio/max_drawdown.go`
- `portfolio/avg_drawdown.go`
- `portfolio/avg_drawdown_days.go`
- `portfolio/recovery_factor.go`
- `portfolio/calmar.go`
- `portfolio/keller_ratio.go`
- `portfolio/ulcer_index.go`

For each file:

- [ ] **Step 1: Migrate to CumMax-based drawdown**

Replace `drawdownSeries(eq)` with the CumMax pattern above. Extract column for iteration.

For `calmar.go`, also replace `cagr(eq[0], eq[len(eq)-1], years)` with:
```go
eqCol := eq.Column(portfolioAsset, data.PortfolioEquity)
startVal := eqCol[0]
endVal := eqCol[len(eqCol)-1]
annReturn := math.Pow(endVal/startVal, 1.0/years) - 1
```

Replace `windowSliceTimes(a.EquityTimes(), window)` with `pd.Window(window).Times()`.

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./portfolio/...`

---

### Task 11: Migrate risk-free metrics

These metrics use `RiskFreePrices()`, `excessReturns()`, `annualizationFactor()`.

**New pattern:**
```go
pd := a.PerfData()
if pd == nil { return 0, nil }
rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
if len(rfCol) == 0 || rfCol[0] == 0 { return 0, ErrNoRiskFreeRate }
perfDF := pd.Window(window)
returns := perfDF.Pct().Drop(math.NaN())
er := returns.Metrics(data.PortfolioEquity).Sub(returns, data.PortfolioRiskFree)
if err := er.Err(); err != nil { return 0, err }
```

**Files:**
- `portfolio/sharpe.go` -- follow spec section 9 exactly
- `portfolio/sortino.go`
- `portfolio/smart_sharpe.go`
- `portfolio/smart_sortino.go`
- `portfolio/probabilistic_sharpe.go`
- `portfolio/downside_deviation.go`
- `portfolio/std_dev.go`

For each file:

- [ ] **Step 1: Migrate to DataFrame chains**

Follow the Sharpe pattern from spec section 9.

For `std_dev.go`: does NOT use risk-free. Pattern:
```go
pd := a.PerfData()
if pd == nil { return 0, nil }
perfDF := pd.Window(window)
eq := perfDF.Metrics(data.PortfolioEquity)
r := eq.Pct().Drop(math.NaN())
sd := r.Std().Value(portfolioAsset, data.PortfolioEquity)
af := annualizationFactor(perfDF.Times())
return sd * math.Sqrt(af), nil
```

For `sortino.go` and `smart_sortino.go`: extract excess returns column, filter negatives manually, compute stddev on negative subset using `stat.StdDev(neg, nil)`.

For `probabilistic_sharpe.go`: uses mean, stddev, kurtosis, skewness of excess returns. Extract column and compute using gonum stat functions.

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./portfolio/...`

---

### Task 12: Migrate benchmark metrics

These metrics use `BenchmarkPrices()`.

**New pattern:**
```go
pd := a.PerfData()
if pd == nil { return 0, nil }
bmCol := pd.Column(portfolioAsset, data.PortfolioBenchmark)
if len(bmCol) == 0 || bmCol[0] == 0 { return 0, ErrNoBenchmark }
perfDF := pd.Window(window)
returns := perfDF.Pct().Drop(math.NaN())
```

**Files:**
- `portfolio/beta.go`
- `portfolio/tracking_error.go`
- `portfolio/information_ratio.go`
- `portfolio/r_squared.go`
- `portfolio/upside_capture.go`
- `portfolio/downside_capture.go`
- `portfolio/active_return.go`

For each file:

- [ ] **Step 1: Migrate to DataFrame chains**

**beta.go example:**
```go
func (beta) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil { return 0, nil }
	bmCol := pd.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmCol) == 0 || bmCol[0] == 0 { return 0, ErrNoBenchmark }

	perfDF := pd.Window(window)
	returns := perfDF.Pct().Drop(math.NaN())
	if returns.Len() == 0 { return 0, nil }

	pCol := returns.Column(portfolioAsset, data.PortfolioEquity)
	bCol := returns.Column(portfolioAsset, data.PortfolioBenchmark)

	v := stat.Variance(bCol, nil)
	if v == 0 { return 0, nil }

	cov := stat.Covariance(pCol, nil, bCol, nil)
	return cov / v, nil
}
```

For `tracking_error.go` and `information_ratio.go`: compute active returns by extracting columns and subtracting element-wise, or use broadcast `Sub(returns, data.PortfolioBenchmark)` on equity-filtered returns.

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./portfolio/...`

---

### Task 13: Migrate combined and remaining metrics

**Files:**
- `portfolio/alpha.go` (uses both risk-free and benchmark)
- `portfolio/treynor.go` (uses both risk-free and benchmark)
- `portfolio/mwrr.go` (uses EquityCurve/EquityTimes directly)
- `portfolio/turnover.go` (uses EquityCurve)
- `portfolio/tax_cost_ratio.go` (uses EquityCurve)
- `portfolio/unrealized_ltcg.go` (uses EquityTimes)
- `portfolio/unrealized_stcg.go` (uses EquityTimes)
- `portfolio/cagr_metric.go` (uses cagr + windowSliceTimes)
- `portfolio/dynamic_withdrawal_rate.go`
- `portfolio/safe_withdrawal_rate.go`
- `portfolio/perpetual_withdrawal_rate.go`

For each file:

- [ ] **Step 1: Migrate to DataFrame chains**

**alpha.go** -- needs both risk-free and benchmark:
```go
pd := a.PerfData()
if pd == nil { return 0, nil }
rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
if len(rfCol) == 0 || rfCol[0] == 0 { return 0, ErrNoRiskFreeRate }
bmCol := pd.Column(portfolioAsset, data.PortfolioBenchmark)
if len(bmCol) == 0 || bmCol[0] == 0 { return 0, ErrNoBenchmark }
// ... rest using pd.Window(window).Pct().Drop(math.NaN())
```

**mwrr.go** -- uses EquityCurve() and EquityTimes() for IRR cash flow matching:
```go
pd := a.PerfData()
if pd == nil { return 0, nil }
equity := pd.Column(portfolioAsset, data.PortfolioEquity)
times := pd.Times()
```

**turnover.go** and **tax_cost_ratio.go** -- similar direct access pattern via `pd.Column(...)`.

**unrealized_ltcg.go** and **unrealized_stcg.go** -- only use timestamps:
```go
pd := a.PerfData()
if pd == nil { return 0, nil }
times := pd.Times()
```

**cagr_metric.go** -- uses windowSliceTimes:
```go
pd := a.PerfData()
if pd == nil { return 0, nil }
perfDF := pd.Window(window)
eqCol := perfDF.Column(portfolioAsset, data.PortfolioEquity)
eqTimes := perfDF.Times()
// inline cagr: math.Pow(endVal/startVal, 1.0/years) - 1
```

**withdrawal metrics** -- use `windowSlice(a.EquityCurve(), ...)`:
```go
pd := a.PerfData()
if pd == nil { return 0, nil }
equity := pd.Window(window).Column(portfolioAsset, data.PortfolioEquity)
```

- [ ] **Step 2: Verify full compilation**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./...`
Expected: PASS

- [ ] **Step 3: Commit all metric migrations together with the helper deletion**

```bash
git add portfolio/metric_helpers.go portfolio/sharpe.go portfolio/sortino.go portfolio/smart_sharpe.go portfolio/smart_sortino.go portfolio/probabilistic_sharpe.go portfolio/downside_deviation.go portfolio/std_dev.go portfolio/beta.go portfolio/tracking_error.go portfolio/information_ratio.go portfolio/r_squared.go portfolio/upside_capture.go portfolio/downside_capture.go portfolio/active_return.go portfolio/alpha.go portfolio/treynor.go portfolio/twrr.go portfolio/tail_ratio.go portfolio/n_positive_periods.go portfolio/excess_kurtosis.go portfolio/skewness.go portfolio/consecutive_wins.go portfolio/consecutive_losses.go portfolio/omega_ratio.go portfolio/cvar.go portfolio/value_at_risk.go portfolio/kelly_criterion.go portfolio/gain_loss_ratio.go portfolio/gain_to_pain.go portfolio/exposure.go portfolio/k_ratio.go portfolio/max_drawdown.go portfolio/avg_drawdown.go portfolio/avg_drawdown_days.go portfolio/recovery_factor.go portfolio/calmar.go portfolio/keller_ratio.go portfolio/ulcer_index.go portfolio/mwrr.go portfolio/turnover.go portfolio/tax_cost_ratio.go portfolio/unrealized_ltcg.go portfolio/unrealized_stcg.go portfolio/cagr_metric.go portfolio/dynamic_withdrawal_rate.go portfolio/safe_withdrawal_rate.go portfolio/perpetual_withdrawal_rate.go
git commit -m "refactor(portfolio): migrate all metrics from helpers to DataFrame chains"
```

---

## Chunk 4: Interface Changes, Test Updates, and Cleanup

### Task 14: Update test files that reference old accessors

**Files:**
- `portfolio/return_metrics_test.go`
- `portfolio/account_test.go`
- `portfolio/benchmark_metrics_test.go`
- `portfolio/risk_adjusted_metrics_test.go`
- `portfolio/capture_drawdown_metrics_test.go`
- `portfolio/distribution_metrics_test.go`
- `portfolio/sqlite_test.go`
- `engine/backtest_test.go`
- Any other test files referencing old accessors

- [ ] **Step 1: Update return_metrics_test.go**

Replace `Expect(a.EquityCurve()).To(Equal(...))` with:
```go
perfAsset := asset.Asset{CompositeFigi: "_PORTFOLIO_", Ticker: "_PORTFOLIO_"}
pd := a.PerfData()
Expect(pd).NotTo(BeNil())
eqCol := pd.Column(perfAsset, data.PortfolioEquity)
Expect(eqCol).To(Equal([]float64{...}))
```

- [ ] **Step 2: Update account_test.go**

Replace all `EquityCurve()`, `EquityTimes()`, `BenchmarkPrices()`, `RiskFreePrices()` assertions with `PerfData()` equivalents. Remove the PerfData-alongside-old-accessors tests added in Task 6 (they become the primary assertions now).

- [ ] **Step 3: Update engine/backtest_test.go**

Replace `EquityCurve()` references with `PerfData()` based assertions.

- [ ] **Step 4: Update sqlite_test.go**

Replace `EquityCurve()`, `BenchmarkPrices()`, `RiskFreePrices()` assertions with `PerfData()` equivalents.

- [ ] **Step 5: Update any remaining test files**

Search for remaining references:
```bash
grep -r "EquityCurve\|EquityTimes\|BenchmarkPrices\|RiskFreePrices" --include="*_test.go" portfolio/ engine/
```

Fix any remaining references.

- [ ] **Step 6: Verify tests pass with old accessors still present**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./... -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add portfolio/*_test.go engine/backtest_test.go
git commit -m "test: update all tests from old accessors to PerfData"
```

---

### Task 15: Remove old accessors and update interfaces

Now that all consumers (metrics and tests) use `PerfData()`, remove old fields, accessors, and interface methods.

**Files:**
- Modify: `portfolio/account.go`
- Modify: `portfolio/portfolio.go`
- Modify: `portfolio/snapshot.go`
- Modify: `portfolio/metric_query.go`
- Modify: `portfolio/sqlite.go`

- [ ] **Step 1: Rewrite UpdatePrices to use only perfData**

Remove old `equityCurve`, `equityTimes`, `benchmarkPrices`, `riskFreePrices` fields from Account struct. Remove old `EquityCurve()`, `EquityTimes()`, `BenchmarkPrices()`, `RiskFreePrices()` methods. Rewrite `UpdatePrices` to ONLY build perfData (remove the old append calls):

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
		if math.IsNaN(v) || v == 0 {
			v = df.Value(a.benchmark, data.MetricClose)
		}
		benchVal = v
	}
	if a.riskFree != (asset.Asset{}) {
		v := df.Value(a.riskFree, data.AdjClose)
		if math.IsNaN(v) || v == 0 {
			v = df.Value(a.riskFree, data.MetricClose)
		}
		rfVal = v
	}

	if a.perfData == nil {
		t := []time.Time{df.End()}
		assets := []asset.Asset{portfolioAsset}
		metrics := []data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree}
		row, err := data.NewDataFrame(t, assets, metrics, []float64{total, benchVal, rfVal})
		if err != nil {
			log.Error().Err(err).Msg("UpdatePrices: failed to create perfData")
			return
		}
		a.perfData = row
	} else {
		if err := a.perfData.AppendRow(df.End(), []float64{total, benchVal, rfVal}); err != nil {
			log.Error().Err(err).Msg("UpdatePrices: failed to append to perfData")
			return
		}
	}
}
```

- [ ] **Step 2: Update Portfolio interface**

In `portfolio/portfolio.go`, replace `EquityCurve()` and `EquityTimes()` with:
```go
PerfData() *data.DataFrame
```

- [ ] **Step 3: Update PortfolioSnapshot interface**

In `portfolio/snapshot.go`, replace `EquityCurve()`, `EquityTimes()`, `BenchmarkPrices()`, `RiskFreePrices()` with:
```go
PerfData() *data.DataFrame
```

Update `WithPortfolioSnapshot` to use `snap.PerfData().Copy()` instead of appending to old slices.

- [ ] **Step 4: Update metric_query.go doc comment**

Update `PerformanceMetric` interface doc to reference `perfData` instead of "equity curve".

- [ ] **Step 5: Update SQLite serialization**

Update `portfolio/sqlite.go`:
- Change `schemaVersion` to `"2"`
- Replace `equity_curve` and `price_series` table definitions with `perf_data` table
- Replace `writeEquityCurve`/`writePriceSeries` with `writePerfData`
- Replace `readEquityCurve`/`readPriceSeries` with `readPerfData`
- Update `FromSQLite` schema version check to `"2"`
- Add `"sort"` import

See Task 7 in the original plan for the exact `writePerfData` and `readPerfData` implementations.

- [ ] **Step 6: Delete export_test.go and metric_helpers_test.go**

```bash
rm portfolio/export_test.go portfolio/metric_helpers_test.go
```

- [ ] **Step 7: Delete data/stats.go and data/stats_test.go**

```bash
rm data/stats.go data/stats_test.go
```

- [ ] **Step 8: Verify full compilation and tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./... -count=1`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add portfolio/account.go portfolio/portfolio.go portfolio/snapshot.go portfolio/metric_query.go portfolio/sqlite.go portfolio/sqlite_test.go
git rm portfolio/export_test.go portfolio/metric_helpers_test.go data/stats.go data/stats_test.go
git commit -m "refactor(portfolio): remove old accessors, update interfaces and SQLite to schema v2"
```

---

### Task 16: Final verification

- [ ] **Step 1: Verify no references to deleted symbols**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt2
grep -r "EquityCurve\|EquityTimes\|BenchmarkPrices\|RiskFreePrices\|windowSlice\|excessReturns\|drawdownSeries\|data\.AnnualizationFactor" --include="*.go" | grep -v "vendor/"
```

Expected: No matches (except possibly comments in test files referencing math formulas).

- [ ] **Step 2: Run full test suite one final time**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./... -count=1`
Expected: PASS

- [ ] **Step 3: Run go vet**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go vet ./...`
Expected: No issues

- [ ] **Step 4: Commit any remaining fixes**

```bash
git add -A
git commit -m "chore: final cleanup after metric helpers refactor"
```
