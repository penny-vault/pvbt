# Backtest Report Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat CLI metric dump with a rich, structured MVVM backtest report using lipgloss + ntcharts.

**Architecture:** Domain model (portfolio.Account with metrics) -> ViewModel (report.Report plain struct) -> View (report/terminal renderer). The report.Build() builder maps domain to view model. Data flows one direction only.

**Tech Stack:** Go, lipgloss (styling), ntcharts (equity curve chart), Ginkgo/Gomega (tests)

**Spec:** `docs/superpowers/specs/2026-03-15-backtest-report-design.md`

---

## File Structure

### New Files

```
portfolio/benchmark_view.go       -- BenchmarkPerfData() and benchmarkAccount wrapper
portfolio/benchmark_view_test.go  -- Tests for benchmark view
portfolio/monthly_returns.go      -- MonthlyReturns(metric) Account method
portfolio/monthly_returns_test.go -- Tests for monthly returns
portfolio/annual_returns.go       -- AnnualReturns(metric) Account method
portfolio/annual_returns_test.go  -- Tests for annual returns
portfolio/drawdown_detail.go      -- DrawdownDetails(topN) Account method
portfolio/drawdown_detail_test.go -- Tests for drawdown details
report/report.go                  -- Report struct, Build(), RunMeta, all view model types
report/report_test.go             -- Tests for Build()
report/terminal/renderer.go       -- Render(Report, io.Writer) orchestrator
report/terminal/style.go          -- Shared lipgloss styles and formatting helpers
report/terminal/header.go         -- renderHeader
report/terminal/chart.go          -- renderEquityCurve (ntcharts)
report/terminal/returns.go        -- renderTrailingReturns, renderAnnualReturns
report/terminal/risk.go           -- renderRisk, renderRiskVsBenchmark
report/terminal/drawdowns.go      -- renderDrawdowns
report/terminal/monthly.go        -- renderMonthlyReturns
report/terminal/trades.go         -- renderTrades
```

### Modified Files

```
portfolio/metric_query.go:54-74   -- Add benchmark field + Benchmark() method to query builder
portfolio/metric_query.go:20-23   -- Add ErrBenchmarkNotSupported error
cli/backtest.go:58-157            -- Add timing, DescribeStrategy, replace printSummary
```

### Deleted Files

```
cli/summary.go                    -- Replaced entirely by report package
```

---

## Chunk 1: Benchmark Targeting on Performance Metrics

### Task 1: Add BenchmarkTargetable interface and Benchmark() to query builder

**Files:**
- Modify: `portfolio/metric_query.go:20-74`

- [ ] **Step 1: Write the failing test**

Create `portfolio/benchmark_view_test.go`:

```go
package portfolio_test

import (
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Benchmark targeting", func() {
	Describe("Benchmark() on query builder", func() {
		It("computes TWRR against benchmark curve", func() {
			// SPY goes 100->110 (+10%), BIL stays flat
			acct := buildAccountWithRF(
				[]float64{100, 105, 110, 115, 120},
				[]float64{50, 50, 50, 50, 50},
			)

			strategyTWRR, err := acct.PerformanceMetric(portfolio.TWRR).Value()
			Expect(err).NotTo(HaveOccurred())

			benchmarkTWRR, err := acct.PerformanceMetric(portfolio.TWRR).Benchmark().Value()
			Expect(err).NotTo(HaveOccurred())

			// Strategy and benchmark should differ since benchmark is
			// the SPY prices (which are also used as equity), but
			// portfolio equity includes cash + holdings.
			// The key test: .Benchmark() returns a value, no error.
			Expect(math.IsNaN(benchmarkTWRR)).To(BeFalse())
			Expect(strategyTWRR).NotTo(Equal(benchmarkTWRR))
		})

		It("returns ErrBenchmarkNotSupported for transaction-based metrics", func() {
			acct := buildAccountWithRF(
				[]float64{100, 105, 110},
				[]float64{50, 50, 50},
			)

			_, err := acct.PerformanceMetric(portfolio.WinRate).Benchmark().Value()
			Expect(err).To(MatchError(portfolio.ErrBenchmarkNotSupported))
		})

		It("returns ErrBenchmarkNotSupported for relational metrics", func() {
			acct := buildAccountWithRF(
				[]float64{100, 105, 110},
				[]float64{50, 50, 50},
			)

			_, err := acct.PerformanceMetric(portfolio.Beta).Benchmark().Value()
			Expect(err).To(MatchError(portfolio.ErrBenchmarkNotSupported))
		})

		It("returns ErrNoBenchmark when no benchmark configured", func() {
			acct := buildAccountFromEquity([]float64{100, 110, 120})

			_, err := acct.PerformanceMetric(portfolio.TWRR).Benchmark().Value()
			Expect(err).To(MatchError(portfolio.ErrNoBenchmark))
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Benchmark targeting" -v`
Expected: compilation errors -- `Benchmark()` method and `ErrBenchmarkNotSupported` don't exist yet.

- [ ] **Step 3: Add BenchmarkTargetable interface and error sentinel**

Add to `portfolio/metric_query.go` after line 22:

```go
var ErrBenchmarkNotSupported = errors.New("metric does not support benchmark targeting")

// BenchmarkTargetable is an optional interface that single-series metrics
// implement to indicate they can compute against the benchmark equity
// curve. Relational metrics (Beta, Alpha, etc.) and transaction-based
// metrics (WinRate, ProfitFactor, etc.) must NOT implement this.
type BenchmarkTargetable interface {
	PerformanceMetric
	// BenchmarkTargetable is a marker method.
	BenchmarkTargetable()
}
```

- [ ] **Step 4: Add benchmark field and Benchmark() method to query builder**

Modify `portfolio/metric_query.go` -- update the `PerformanceMetricQuery` struct and add methods:

```go
type PerformanceMetricQuery struct {
	account   *Account
	metric    PerformanceMetric
	window    *Period
	benchmark bool
}

// Benchmark tells the query to compute the metric against the benchmark
// equity curve instead of the portfolio equity curve. Only valid for
// metrics that implement BenchmarkTargetable.
func (q PerformanceMetricQuery) Benchmark() PerformanceMetricQuery {
	q.benchmark = true
	return q
}

func (q PerformanceMetricQuery) Value() (float64, error) {
	if q.benchmark {
		if _, ok := q.metric.(BenchmarkTargetable); !ok {
			return 0, ErrBenchmarkNotSupported
		}
		view, err := q.account.benchmarkView()
		if err != nil {
			return 0, err
		}
		return q.metric.Compute(view, q.window)
	}
	return q.metric.Compute(q.account, q.window)
}

func (q PerformanceMetricQuery) Series() ([]float64, error) {
	if q.benchmark {
		if _, ok := q.metric.(BenchmarkTargetable); !ok {
			return nil, ErrBenchmarkNotSupported
		}
		view, err := q.account.benchmarkView()
		if err != nil {
			return nil, err
		}
		return q.metric.ComputeSeries(view, q.window)
	}
	return q.metric.ComputeSeries(q.account, q.window)
}
```

- [ ] **Step 5: Implement benchmarkView() on Account**

Create `portfolio/benchmark_view.go`:

```go
package portfolio

import "github.com/penny-vault/pvbt/data"

// benchmarkView returns a shallow Account clone whose PerfData has the
// PortfolioEquity column replaced with normalized benchmark values.
// This lets any single-series metric compute against the benchmark
// without per-metric changes.
func (a *Account) benchmarkView() (*Account, error) {
	if a.benchmark == (asset.Asset{}) {
		return nil, ErrNoBenchmark
	}

	pd := a.PerfData()
	if pd == nil {
		return nil, ErrNoBenchmark
	}

	bmCol := pd.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmCol) == 0 {
		return nil, ErrNoBenchmark
	}

	// Normalize benchmark to start at the same value as portfolio equity.
	eqCol := pd.Column(portfolioAsset, data.PortfolioEquity)
	if len(eqCol) == 0 || bmCol[0] == 0 {
		return nil, ErrNoBenchmark
	}

	scale := eqCol[0] / bmCol[0]
	normalized := make([]float64, len(bmCol))
	for i, val := range bmCol {
		normalized[i] = val * scale
	}

	// Build a new DataFrame with PortfolioEquity replaced.
	viewDF := pd.Copy()
	if err := viewDF.Insert(portfolioAsset, data.PortfolioEquity, normalized); err != nil {
		return nil, err
	}

	// Return a shallow clone with swapped perfData.
	view := *a
	view.perfData = viewDF
	return &view, nil
}
```

Note: the `asset` import is already available since `account.go` uses it. Add `"github.com/penny-vault/pvbt/asset"` to the import if needed.

- [ ] **Step 6: Mark single-series metrics as BenchmarkTargetable**

Add the marker method to each benchmark-targetable metric. These are the metrics that only read `PortfolioEquity` from perfData:

For each of these files, add `func (X) BenchmarkTargetable() {}` to the metric struct:

- `portfolio/twrr.go` -- add `func (twrr) BenchmarkTargetable() {}`
- `portfolio/mwrr.go` -- add `func (mwrr) BenchmarkTargetable() {}`
- `portfolio/sharpe.go` -- add `func (sharpe) BenchmarkTargetable() {}`
- `portfolio/sortino.go` -- add `func (sortino) BenchmarkTargetable() {}`
- `portfolio/calmar.go` -- add `func (calmar) BenchmarkTargetable() {}`
- `portfolio/max_drawdown.go` -- add `func (maxDrawdown) BenchmarkTargetable() {}`
- `portfolio/std_dev.go` -- add `func (stdDev) BenchmarkTargetable() {}`
- `portfolio/downside_deviation.go` -- add `func (downsideDeviation) BenchmarkTargetable() {}`
- `portfolio/ulcer_index.go` -- add `func (ulcerIndex) BenchmarkTargetable() {}`
- `portfolio/skewness.go` -- add `func (skewness) BenchmarkTargetable() {}`
- `portfolio/excess_kurtosis.go` -- add `func (excessKurtosis) BenchmarkTargetable() {}`
- `portfolio/value_at_risk.go` -- add `func (valueAtRisk) BenchmarkTargetable() {}`
- `portfolio/cagr_metric.go` -- add `func (cagrMetric) BenchmarkTargetable() {}`

Do NOT add it to: `beta.go`, `alpha.go`, `r_squared.go`, `tracking_error.go`, `information_ratio.go`, `treynor.go`, `upside_capture.go`, `downside_capture.go`, `win_rate.go`, `average_win.go`, `average_loss.go`, `profit_factor_metric.go`, `average_holding_period.go`, `turnover.go`, or any tax/withdrawal metric.

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Benchmark targeting" -v`
Expected: all 4 tests pass.

- [ ] **Step 8: Run full portfolio test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v`
Expected: all existing tests still pass.

- [ ] **Step 9: Commit**

```bash
git add portfolio/metric_query.go portfolio/benchmark_view.go portfolio/benchmark_view_test.go portfolio/twrr.go portfolio/mwrr.go portfolio/sharpe.go portfolio/sortino.go portfolio/calmar.go portfolio/max_drawdown.go portfolio/std_dev.go portfolio/downside_deviation.go portfolio/ulcer_index.go portfolio/skewness.go portfolio/excess_kurtosis.go portfolio/value_at_risk.go portfolio/cagr_metric.go
git commit -m "feat: add benchmark targeting to performance metric query builder"
```

---

### Task 2: Add MonthlyReturns Account method

**Files:**
- Create: `portfolio/monthly_returns.go`
- Create: `portfolio/monthly_returns_test.go`

- [ ] **Step 1: Write the failing test**

Create `portfolio/monthly_returns_test.go`:

```go
package portfolio_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("MonthlyReturns", func() {
	It("returns monthly percentage returns for each year", func() {
		// Build an account spanning 3 months: Jan, Feb, Mar 2024
		// with end-of-month equity values: 100, 110, 105
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		acct := portfolio.New(portfolio.WithCash(100, time.Time{}))

		// One data point per month at end of month
		dates := []time.Time{
			time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 3, 29, 0, 0, 0, 0, time.UTC),
		}
		equities := []float64{100, 110, 105}

		for i, date := range dates {
			if i > 0 {
				diff := equities[i] - equities[i-1]
				txType := portfolio.DepositTransaction
				if diff < 0 {
					txType = portfolio.WithdrawalTransaction
				}
				acct.Record(portfolio.Transaction{
					Date:   date,
					Type:   txType,
					Amount: diff,
				})
			}
			df := buildDF(date, []asset.Asset{spy}, []float64{450}, []float64{448})
			acct.UpdatePrices(df)
		}

		years, values, err := acct.MonthlyReturns(data.PortfolioEquity)
		Expect(err).NotTo(HaveOccurred())
		Expect(years).To(Equal([]int{2024}))
		Expect(values).To(HaveLen(1))
		Expect(values[0]).To(HaveLen(12))

		// Jan: no prior month, so NaN
		Expect(math.IsNaN(values[0][0])).To(BeTrue())
		// Feb: (110-100)/100 = 0.10
		Expect(values[0][1]).To(BeNumerically("~", 0.10, 0.001))
		// Mar: (105-110)/110 = -0.0454...
		Expect(values[0][2]).To(BeNumerically("~", -0.04545, 0.001))
		// Apr-Dec: NaN (no data)
		for month := 3; month < 12; month++ {
			Expect(math.IsNaN(values[0][month])).To(BeTrue())
		}
	})

	It("returns empty results for nil perfData", func() {
		acct := portfolio.New(portfolio.WithCash(100, time.Time{}))
		years, values, err := acct.MonthlyReturns(data.PortfolioEquity)
		Expect(err).NotTo(HaveOccurred())
		Expect(years).To(BeNil())
		Expect(values).To(BeNil())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "MonthlyReturns" -v`
Expected: compilation error -- `MonthlyReturns` method doesn't exist.

- [ ] **Step 3: Implement MonthlyReturns**

Create `portfolio/monthly_returns.go`:

```go
package portfolio

import (
	"math"

	"github.com/penny-vault/pvbt/data"
)

// MonthlyReturns computes month-over-month percentage returns from the
// specified metric column (typically data.PortfolioEquity or
// data.PortfolioBenchmark). Returns years as a sorted slice and values
// as a [year][12]float64 grid where NaN marks months with no data or
// no prior month for comparison.
func (a *Account) MonthlyReturns(metric data.Metric) ([]int, [][]float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return nil, nil, nil
	}

	monthly := pd.Metrics(metric).Downsample(data.Monthly).Last()
	if monthly.Len() == 0 {
		return nil, nil, nil
	}

	times := monthly.Times()
	col := monthly.Column(portfolioAsset, metric)

	// Determine year range.
	firstYear := times[0].Year()
	lastYear := times[len(times)-1].Year()

	years := make([]int, 0, lastYear-firstYear+1)
	for yr := firstYear; yr <= lastYear; yr++ {
		years = append(years, yr)
	}

	// Build year->month grid, initialized to NaN.
	grid := make([][]float64, len(years))
	for yi := range grid {
		grid[yi] = make([]float64, 12)
		for mi := range grid[yi] {
			grid[yi][mi] = math.NaN()
		}
	}

	// Fill in returns. First data point has no prior month.
	for idx := 1; idx < len(times); idx++ {
		yr := times[idx].Year()
		mo := int(times[idx].Month()) - 1 // 0-indexed
		yi := yr - firstYear
		prev := col[idx-1]
		curr := col[idx]

		if prev != 0 && !math.IsNaN(prev) && !math.IsNaN(curr) {
			grid[yi][mo] = (curr - prev) / prev
		}
	}

	return years, grid, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "MonthlyReturns" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/monthly_returns.go portfolio/monthly_returns_test.go
git commit -m "feat: add MonthlyReturns account method for monthly return grid"
```

---

### Task 3: Add AnnualReturns Account method

**Files:**
- Create: `portfolio/annual_returns.go`
- Create: `portfolio/annual_returns_test.go`

- [ ] **Step 1: Write the failing test**

Create `portfolio/annual_returns_test.go`:

```go
package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("AnnualReturns", func() {
	It("computes year-over-year returns", func() {
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		acct := portfolio.New(portfolio.WithCash(100, time.Time{}))

		// Equity: end of 2024 = 120, end of 2025 = 150
		dates := []time.Time{
			time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
		}
		equities := []float64{100, 120, 150}

		for i, date := range dates {
			if i > 0 {
				diff := equities[i] - equities[i-1]
				txType := portfolio.DepositTransaction
				if diff < 0 {
					txType = portfolio.WithdrawalTransaction
				}
				acct.Record(portfolio.Transaction{
					Date:   date,
					Type:   txType,
					Amount: diff,
				})
			}
			df := buildDF(date, []asset.Asset{spy}, []float64{450}, []float64{448})
			acct.UpdatePrices(df)
		}

		years, returns, err := acct.AnnualReturns(data.PortfolioEquity)
		Expect(err).NotTo(HaveOccurred())
		Expect(years).To(Equal([]int{2024, 2025}))
		// 2024: (120-100)/100 = 0.20
		Expect(returns[0]).To(BeNumerically("~", 0.20, 0.001))
		// 2025: (150-120)/120 = 0.25
		Expect(returns[1]).To(BeNumerically("~", 0.25, 0.001))
	})

	It("returns empty for nil perfData", func() {
		acct := portfolio.New(portfolio.WithCash(100, time.Time{}))
		years, returns, err := acct.AnnualReturns(data.PortfolioEquity)
		Expect(err).NotTo(HaveOccurred())
		Expect(years).To(BeNil())
		Expect(returns).To(BeNil())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "AnnualReturns" -v`
Expected: compilation error.

- [ ] **Step 3: Implement AnnualReturns**

Create `portfolio/annual_returns.go`:

```go
package portfolio

import (
	"math"

	"github.com/penny-vault/pvbt/data"
)

// AnnualReturns computes year-over-year returns from the specified metric
// column. Returns sorted years and corresponding annual returns. For the
// first year, the return is computed from the first data point to the
// last data point in that year.
func (a *Account) AnnualReturns(metric data.Metric) ([]int, []float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return nil, nil, nil
	}

	yearly := pd.Metrics(metric).Downsample(data.Yearly).Last()
	if yearly.Len() < 1 {
		return nil, nil, nil
	}

	times := yearly.Times()
	col := yearly.Column(portfolioAsset, metric)

	// Get the very first equity value for the first year's return.
	allCol := pd.Column(portfolioAsset, metric)
	firstVal := allCol[0]

	years := make([]int, len(times))
	returns := make([]float64, len(times))

	for idx := range times {
		years[idx] = times[idx].Year()

		var prev float64
		if idx == 0 {
			prev = firstVal
		} else {
			prev = col[idx-1]
		}

		if prev != 0 && !math.IsNaN(prev) && !math.IsNaN(col[idx]) {
			returns[idx] = (col[idx] - prev) / prev
		} else {
			returns[idx] = math.NaN()
		}
	}

	return years, returns, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "AnnualReturns" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/annual_returns.go portfolio/annual_returns_test.go
git commit -m "feat: add AnnualReturns account method for yearly return computation"
```

---

### Task 4: Add DrawdownDetails Account method

**Files:**
- Create: `portfolio/drawdown_detail.go`
- Create: `portfolio/drawdown_detail_test.go`

- [ ] **Step 1: Write the failing test**

Create `portfolio/drawdown_detail_test.go`:

```go
package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("DrawdownDetails", func() {
	It("identifies drawdown periods sorted by depth", func() {
		// Equity curve with two distinct drawdowns:
		// Peak at 120, trough at 100 (-16.7%), recovery to 130
		// Peak at 130, trough at 120 (-7.7%), recovery to 140
		acct := buildAccountFromEquity([]float64{
			100, 110, 120, // rise to peak 1
			110, 100, // drawdown 1: -16.7%
			110, 120, 130, // recovery + rise to peak 2
			125, 120, // drawdown 2: -7.7%
			130, 140, // recovery
		})

		details, err := acct.DrawdownDetails(5)
		Expect(err).NotTo(HaveOccurred())
		Expect(details).To(HaveLen(2))

		// Sorted by depth (most negative first).
		Expect(details[0].Depth).To(BeNumerically("<", details[1].Depth))

		// First drawdown is deeper.
		Expect(details[0].Depth).To(BeNumerically("~", -0.1667, 0.01))
		Expect(details[0].Days).To(BeNumerically(">", 0))

		// Second drawdown is shallower.
		Expect(details[1].Depth).To(BeNumerically("~", -0.0769, 0.01))
	})

	It("caps results to topN", func() {
		acct := buildAccountFromEquity([]float64{
			100, 120, 110, 130, 115, 140, 125, 150,
		})

		details, err := acct.DrawdownDetails(1)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(details)).To(BeNumerically("<=", 1))
	})

	It("returns empty for nil perfData", func() {
		acct := portfolio.New(portfolio.WithCash(100, time.Time{}))
		details, err := acct.DrawdownDetails(5)
		Expect(err).NotTo(HaveOccurred())
		Expect(details).To(BeEmpty())
	})

	It("handles unrecovered drawdown at end of data", func() {
		// Drawdown that never recovers.
		acct := buildAccountFromEquity([]float64{100, 110, 105, 100})

		details, err := acct.DrawdownDetails(5)
		Expect(err).NotTo(HaveOccurred())
		Expect(details).To(HaveLen(1))
		Expect(details[0].Recovery.IsZero()).To(BeTrue())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "DrawdownDetails" -v`
Expected: compilation error.

- [ ] **Step 3: Implement DrawdownDetails**

Create `portfolio/drawdown_detail.go`:

```go
package portfolio

import (
	"sort"
	"time"

	"github.com/penny-vault/pvbt/data"
)

// DrawdownDetail describes a single drawdown period.
type DrawdownDetail struct {
	Start    time.Time // when equity first dropped below the prior peak
	Trough   time.Time // when equity hit its lowest point in this drawdown
	Recovery time.Time // when equity recovered to the prior peak (zero if unrecovered)
	Depth    float64   // maximum decline as a negative fraction (e.g. -0.0619)
	Days     int       // trading days from start to recovery (or end of data)
}

// DrawdownDetails walks the equity curve to identify distinct drawdown
// periods, sorts them by depth (most negative first), and returns the
// top N.
func (a *Account) DrawdownDetails(topN int) ([]DrawdownDetail, error) {
	pd := a.PerfData()
	if pd == nil {
		return nil, nil
	}

	col := pd.Column(portfolioAsset, data.PortfolioEquity)
	times := pd.Times()
	if len(col) < 2 {
		return nil, nil
	}

	var details []DrawdownDetail

	peak := col[0]
	peakIdx := 0
	inDrawdown := false
	var currentStart int
	var troughIdx int
	var troughVal float64

	for idx := 1; idx < len(col); idx++ {
		if col[idx] >= peak {
			if inDrawdown {
				// Recovered: record the drawdown.
				depth := (troughVal - peak) / peak
				details = append(details, DrawdownDetail{
					Start:    times[currentStart],
					Trough:   times[troughIdx],
					Recovery: times[idx],
					Depth:    depth,
					Days:     idx - currentStart,
				})
				inDrawdown = false
			}
			peak = col[idx]
			peakIdx = idx
		} else {
			if !inDrawdown {
				inDrawdown = true
				currentStart = peakIdx + 1
				if currentStart > idx {
					currentStart = idx
				}
				troughIdx = idx
				troughVal = col[idx]
			} else if col[idx] < troughVal {
				troughIdx = idx
				troughVal = col[idx]
			}
		}
	}

	// Handle unrecovered drawdown.
	if inDrawdown {
		depth := (troughVal - peak) / peak
		details = append(details, DrawdownDetail{
			Start:  times[currentStart],
			Trough: times[troughIdx],
			Depth:  depth,
			Days:   len(col) - 1 - currentStart,
		})
	}

	// Sort by depth (most negative first).
	sort.Slice(details, func(i, j int) bool {
		return details[i].Depth < details[j].Depth
	})

	if len(details) > topN {
		details = details[:topN]
	}

	return details, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "DrawdownDetails" -v`
Expected: PASS

- [ ] **Step 5: Run full portfolio test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add portfolio/drawdown_detail.go portfolio/drawdown_detail_test.go
git commit -m "feat: add DrawdownDetails account method for top-N drawdown analysis"
```

---

## Chunk 2: Report View Model and Builder

### Task 5: Create report package with view model types

**Files:**
- Create: `report/report.go`

- [ ] **Step 1: Create report directory**

Run: `mkdir -p /Users/jdf/Developer/penny-vault/pvbt/report/terminal`

- [ ] **Step 2: Write the report view model types**

Create `report/report.go` with all the view model types from the spec. This file defines the Report struct, all section structs, RunMeta, and the Build() function.

```go
package report

import (
	"errors"
	"math"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// RunMeta carries run-level values that come from the CLI, not the portfolio.
type RunMeta struct {
	Elapsed     time.Duration
	Steps       int
	InitialCash float64
}

// Report is the view model -- a plain data struct shaped for display.
// It holds every value the report renders. No formatting, no ANSI codes.
type Report struct {
	Header          Header
	HasBenchmark    bool
	EquityCurve     EquityCurve
	TrailingReturns TrailingReturns
	AnnualReturns   AnnualReturns
	Risk            Risk
	RiskVsBenchmark RiskVsBenchmark
	Drawdowns       Drawdowns
	MonthlyReturns  MonthlyReturns
	Trades          Trades
	Warnings        []string
}

type Header struct {
	StrategyName    string
	StrategyVersion string
	Benchmark       string
	StartDate       time.Time
	EndDate         time.Time
	InitialCash     float64
	FinalValue      float64
	Elapsed         time.Duration
	Steps           int
}

type EquityCurve struct {
	Times           []time.Time
	StrategyValues  []float64
	BenchmarkValues []float64
}

type TrailingReturns struct {
	Periods   []string
	Strategy  []float64
	Benchmark []float64
}

type AnnualReturns struct {
	Years     []int
	Strategy  []float64
	Benchmark []float64
}

type Risk struct {
	MaxDrawdown       [2]float64
	Volatility        [2]float64
	DownsideDeviation [2]float64
	Sharpe            [2]float64
	Sortino           [2]float64
	Calmar            [2]float64
	UlcerIndex        [2]float64
	ValueAtRisk       [2]float64
	Skewness          [2]float64
	ExcessKurtosis    [2]float64
}

type RiskVsBenchmark struct {
	Beta             float64
	Alpha            float64
	RSquared         float64
	TrackingError    float64
	InformationRatio float64
	Treynor          float64
	UpsideCapture    float64
	DownsideCapture  float64
}

type DrawdownEntry struct {
	Start    time.Time
	End      time.Time
	Recovery time.Time
	Depth    float64
	Days     int
}

type Drawdowns struct {
	Entries []DrawdownEntry
}

type MonthlyReturns struct {
	Years  []int
	Values [][]float64
}

type TradeEntry struct {
	Date   time.Time
	Action string
	Ticker string
	Shares float64
	Price  float64
	Amount float64
	PL     float64
}

type Trades struct {
	TotalTransactions int
	RoundTrips        int
	WinRate           float64
	AvgHolding        float64
	AvgWin            float64
	AvgLoss           float64
	ProfitFactor      float64
	GainLossRatio     float64
	Turnover          float64
	PositivePeriods   float64
	Trades            []TradeEntry
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./report/...`
Expected: compiles without errors. (Build() not yet implemented, but types compile.)

- [ ] **Step 4: Commit**

```bash
git add report/report.go
git commit -m "feat: add report view model types"
```

---

### Task 6: Implement report.Build()

**Files:**
- Modify: `report/report.go` (add Build function body)
- Create: `report/report_test.go`

- [ ] **Step 1: Write a test for Build()**

Create `report/report_test.go`:

```go
package report_test

import (
	"math"
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/report"
)

// buildTestAccount creates a minimal account with equity curve and benchmark.
func buildTestAccount() *portfolio.Account {
	spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	bil := asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	acct := portfolio.New(
		portfolio.WithCash(1000, time.Time{}),
		portfolio.WithBenchmark(spy),
		portfolio.WithRiskFree(bil),
	)

	// Simulate 10 days of prices.
	equities := []float64{1000, 1010, 1020, 1015, 1025, 1030, 1040, 1035, 1045, 1050}
	cur := start
	for i, eq := range equities {
		if wd := cur.Weekday(); wd == time.Saturday {
			cur = cur.AddDate(0, 0, 2)
		} else if wd == time.Sunday {
			cur = cur.AddDate(0, 0, 1)
		}

		if i > 0 {
			diff := eq - equities[i-1]
			txType := portfolio.DepositTransaction
			if diff < 0 {
				txType = portfolio.WithdrawalTransaction
			}
			acct.Record(portfolio.Transaction{
				Date:   cur,
				Type:   txType,
				Amount: diff,
			})
		}

		vals := make([]float64, 6) // spy close, spy adj, bil close, bil adj, + perf metrics
		spyPrice := 450.0 + float64(i)*2
		bilPrice := 50.0
		df, _ := data.NewDataFrame(
			[]time.Time{cur},
			[]asset.Asset{spy, bil},
			[]data.Metric{data.MetricClose, data.AdjClose},
			data.Daily,
			[]float64{spyPrice, spyPrice, bilPrice, bilPrice},
		)
		_ = vals
		acct.UpdatePrices(df)
		cur = cur.AddDate(0, 0, 1)
	}

	return acct
}

func TestBuildHeader(t *testing.T) {
	acct := buildTestAccount()
	info := engine.StrategyInfo{
		Name:    "Test Strategy",
		Version: "1.0.0",
	}
	meta := report.RunMeta{
		Elapsed:     time.Second,
		Steps:       10,
		InitialCash: 1000,
	}

	rpt, err := report.Build(acct, info, meta)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rpt.Header.StrategyName != "Test Strategy" {
		t.Errorf("expected strategy name 'Test Strategy', got %q", rpt.Header.StrategyName)
	}
	if rpt.Header.InitialCash != 1000 {
		t.Errorf("expected initial cash 1000, got %f", rpt.Header.InitialCash)
	}
	if rpt.Header.Steps != 10 {
		t.Errorf("expected 10 steps, got %d", rpt.Header.Steps)
	}
	if rpt.HasBenchmark != true {
		t.Errorf("expected HasBenchmark=true")
	}
}

func TestBuildNoBenchmark(t *testing.T) {
	acct := portfolio.New(portfolio.WithCash(100, time.Time{}))
	spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	df, _ := data.NewDataFrame(
		[]time.Time{time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		[]asset.Asset{spy},
		[]data.Metric{data.MetricClose, data.AdjClose},
		data.Daily,
		[]float64{100, 100},
	)
	acct.UpdatePrices(df)

	info := engine.StrategyInfo{Name: "Test"}
	meta := report.RunMeta{InitialCash: 100, Steps: 1}

	rpt, _ := report.Build(acct, info, meta)
	if rpt.HasBenchmark {
		t.Error("expected HasBenchmark=false when no benchmark configured")
	}
	// RiskVsBenchmark should be zero value.
	if rpt.RiskVsBenchmark.Beta != 0 {
		t.Errorf("expected zero Beta, got %f", rpt.RiskVsBenchmark.Beta)
	}
}

func TestBuildInsufficientData(t *testing.T) {
	acct := portfolio.New(portfolio.WithCash(100, time.Time{}))
	info := engine.StrategyInfo{Name: "Test"}
	meta := report.RunMeta{InitialCash: 100}

	rpt, _ := report.Build(acct, info, meta)
	if len(rpt.Warnings) == 0 {
		t.Error("expected warnings for insufficient data")
	}
}

func TestBuildTrailingReturnsNaN(t *testing.T) {
	// Short backtest: trailing returns for 1Y should be NaN.
	acct := buildTestAccount()
	info := engine.StrategyInfo{Name: "Test"}
	meta := report.RunMeta{InitialCash: 1000, Steps: 10}

	rpt, _ := report.Build(acct, info, meta)

	// Find 1Y period and check it's NaN (10 days of data < 1 year).
	for idx, period := range rpt.TrailingReturns.Periods {
		if period == "1Y" {
			if !math.IsNaN(rpt.TrailingReturns.Strategy[idx]) {
				t.Errorf("expected NaN for 1Y trailing return with 10 days of data, got %f",
					rpt.TrailingReturns.Strategy[idx])
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./report/ -v`
Expected: compilation error -- `Build` function not implemented.

- [ ] **Step 3: Implement Build()**

Add to `report/report.go`:

```go
// Build constructs a Report view model from the portfolio's metrics.
// It does no math -- all values come from the portfolio package.
func Build(acct portfolio.Portfolio, info engine.StrategyInfo, meta RunMeta) (Report, error) {
	var warnings []string

	rpt := Report{
		Header: Header{
			StrategyName:    info.Name,
			StrategyVersion: info.Version,
			Benchmark:       info.Benchmark,
			StartDate:       meta.startDate(acct),
			EndDate:         meta.endDate(acct),
			InitialCash:     meta.InitialCash,
			FinalValue:      acct.Value(),
			Elapsed:         meta.Elapsed,
			Steps:           meta.Steps,
		},
	}

	pd := acct.PerfData()
	if pd == nil || pd.Len() < 2 {
		rpt.Warnings = append(warnings, "insufficient data for full report")
		return rpt, nil
	}

	// Determine if benchmark is available.
	concrete, isAccount := acct.(*portfolio.Account)
	hasBenchmark := false
	if isAccount {
		hasBenchmark = concrete.Benchmark() != (asset.Asset{})
	}
	rpt.HasBenchmark = hasBenchmark

	// Equity curve.
	rpt.EquityCurve = buildEquityCurve(pd, hasBenchmark, meta.InitialCash)

	// Trailing returns.
	rpt.TrailingReturns = buildTrailingReturns(acct, hasBenchmark, &warnings)

	// Annual returns.
	if isAccount {
		rpt.AnnualReturns = buildAnnualReturns(concrete, hasBenchmark)
	}

	// Risk metrics.
	rpt.Risk = buildRisk(acct, hasBenchmark, &warnings)

	// Risk vs benchmark.
	if hasBenchmark {
		rpt.RiskVsBenchmark = buildRiskVsBenchmark(acct, &warnings)
	}

	// Drawdowns.
	if isAccount {
		rpt.Drawdowns = buildDrawdowns(concrete)
	}

	// Monthly returns.
	if isAccount {
		rpt.MonthlyReturns = buildMonthlyReturns(concrete, hasBenchmark)
	}

	// Trades.
	rpt.Trades = buildTrades(acct)

	rpt.Warnings = warnings

	return rpt, nil
}

func (m RunMeta) startDate(acct portfolio.Portfolio) time.Time {
	pd := acct.PerfData()
	if pd != nil && pd.Len() > 0 {
		return pd.Start()
	}
	return time.Time{}
}

func (m RunMeta) endDate(acct portfolio.Portfolio) time.Time {
	pd := acct.PerfData()
	if pd != nil && pd.Len() > 0 {
		return pd.End()
	}
	return time.Time{}
}
```

Then implement each builder helper as a separate unexported function in the same file. These are all straightforward metric queries:

```go
func buildEquityCurve(pd *data.DataFrame, hasBenchmark bool, initialCash float64) EquityCurve {
	ec := EquityCurve{
		Times:          pd.Times(),
		StrategyValues: pd.Column(portfolioAsset, data.PortfolioEquity),
	}

	if hasBenchmark {
		bmCol := pd.Column(portfolioAsset, data.PortfolioBenchmark)
		if len(bmCol) > 0 && bmCol[0] != 0 {
			scale := initialCash / bmCol[0]
			normalized := make([]float64, len(bmCol))
			for i, val := range bmCol {
				normalized[i] = val * scale
			}
			ec.BenchmarkValues = normalized
		}
	}

	return ec
}

func metricVal(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s: %v", metric.Name(), err))
		return math.NaN()
	}
	return val
}

func metricValBenchmark(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Benchmark().Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("benchmark %s: %v", metric.Name(), err))
		return math.NaN()
	}
	return val
}

func metricValWindow(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, window portfolio.Period, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Window(window).Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s (windowed): %v", metric.Name(), err))
		return math.NaN()
	}
	return val
}

func metricValBenchmarkWindow(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, window portfolio.Period, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Benchmark().Window(window).Value()
	if err != nil {
		return math.NaN()
	}
	return val
}

func buildTrailingReturns(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) TrailingReturns {
	type trailingPeriod struct {
		label  string
		window *portfolio.Period
	}

	periods := []trailingPeriod{
		{"1M", ptrPeriod(portfolio.Months(1))},
		{"3M", ptrPeriod(portfolio.Months(3))},
		{"6M", ptrPeriod(portfolio.Months(6))},
		{"YTD", ptrPeriod(portfolio.YTD())},
		{"1Y", ptrPeriod(portfolio.Years(1))},
		{"Since Inception", nil},
	}

	tr := TrailingReturns{
		Periods:   make([]string, len(periods)),
		Strategy:  make([]float64, len(periods)),
		Benchmark: make([]float64, len(periods)),
	}

	for idx, tp := range periods {
		tr.Periods[idx] = tp.label

		if tp.window != nil {
			tr.Strategy[idx] = metricValWindow(acct, portfolio.TWRR, *tp.window, warnings)
			if hasBenchmark {
				tr.Benchmark[idx] = metricValBenchmarkWindow(acct, portfolio.TWRR, *tp.window, warnings)
			} else {
				tr.Benchmark[idx] = math.NaN()
			}
		} else {
			tr.Strategy[idx] = metricVal(acct, portfolio.TWRR, warnings)
			if hasBenchmark {
				tr.Benchmark[idx] = metricValBenchmark(acct, portfolio.TWRR, warnings)
			} else {
				tr.Benchmark[idx] = math.NaN()
			}
		}
	}

	return tr
}

func ptrPeriod(p portfolio.Period) *portfolio.Period {
	return &p
}

func buildAnnualReturns(acct *portfolio.Account, hasBenchmark bool) AnnualReturns {
	years, stratReturns, err := acct.AnnualReturns(data.PortfolioEquity)
	if err != nil || years == nil {
		return AnnualReturns{}
	}

	ar := AnnualReturns{
		Years:    years,
		Strategy: stratReturns,
	}

	if hasBenchmark {
		_, bmReturns, err := acct.AnnualReturns(data.PortfolioBenchmark)
		if err == nil {
			ar.Benchmark = bmReturns
		}
	}

	return ar
}

func buildRisk(acct portfolio.Portfolio, hasBenchmark bool, warnings *[]string) Risk {
	pair := func(metric portfolio.PerformanceMetric) [2]float64 {
		stratVal := metricVal(acct, metric, warnings)
		bmVal := math.NaN()
		if hasBenchmark {
			bmVal = metricValBenchmark(acct, metric, warnings)
		}
		return [2]float64{stratVal, bmVal}
	}

	return Risk{
		MaxDrawdown:       pair(portfolio.MaxDrawdown),
		Volatility:        pair(portfolio.StdDev),
		DownsideDeviation: pair(portfolio.DownsideDeviation),
		Sharpe:            pair(portfolio.Sharpe),
		Sortino:           pair(portfolio.Sortino),
		Calmar:            pair(portfolio.Calmar),
		UlcerIndex:        pair(portfolio.UlcerIndex),
		ValueAtRisk:       pair(portfolio.ValueAtRisk),
		Skewness:          pair(portfolio.Skewness),
		ExcessKurtosis:    pair(portfolio.ExcessKurtosis),
	}
}

func buildRiskVsBenchmark(acct portfolio.Portfolio, warnings *[]string) RiskVsBenchmark {
	return RiskVsBenchmark{
		Beta:             metricVal(acct, portfolio.Beta, warnings),
		Alpha:            metricVal(acct, portfolio.Alpha, warnings),
		RSquared:         metricVal(acct, portfolio.RSquared, warnings),
		TrackingError:    metricVal(acct, portfolio.TrackingError, warnings),
		InformationRatio: metricVal(acct, portfolio.InformationRatio, warnings),
		Treynor:          metricVal(acct, portfolio.Treynor, warnings),
		UpsideCapture:    metricVal(acct, portfolio.UpsideCaptureRatio, warnings),
		DownsideCapture:  metricVal(acct, portfolio.DownsideCaptureRatio, warnings),
	}
}

func buildDrawdowns(acct *portfolio.Account) Drawdowns {
	details, err := acct.DrawdownDetails(5)
	if err != nil || details == nil {
		return Drawdowns{}
	}

	entries := make([]DrawdownEntry, len(details))
	for idx, detail := range details {
		entries[idx] = DrawdownEntry{
			Start:    detail.Start,
			End:      detail.Trough,
			Recovery: detail.Recovery,
			Depth:    detail.Depth,
			Days:     detail.Days,
		}
	}

	return Drawdowns{Entries: entries}
}

func buildMonthlyReturns(acct *portfolio.Account, hasBenchmark bool) MonthlyReturns {
	years, values, err := acct.MonthlyReturns(data.PortfolioEquity)
	if err != nil || years == nil {
		return MonthlyReturns{}
	}

	return MonthlyReturns{
		Years:  years,
		Values: values,
	}
}

func buildTrades(acct portfolio.Portfolio) Trades {
	txns := acct.Transactions()

	var tradeEntries []TradeEntry
	for _, txn := range txns {
		switch txn.Type {
		case portfolio.BuyTransaction, portfolio.SellTransaction:
			action := "BUY"
			if txn.Type == portfolio.SellTransaction {
				action = "SELL"
			}
			tradeEntries = append(tradeEntries, TradeEntry{
				Date:   txn.Date,
				Action: action,
				Ticker: txn.Asset.Ticker,
				Shares: txn.Qty,
				Price:  txn.Price,
				Amount: txn.Amount,
				PL:     math.NaN(), // P/L computed separately
			})
		}
	}

	trade, _ := acct.TradeMetrics()

	// Count round trips: a sell following a buy in the same asset.
	roundTripCount := 0
	for _, txn := range txns {
		if txn.Type == portfolio.SellTransaction {
			roundTripCount++
		}
	}

	return Trades{
		TotalTransactions: len(tradeEntries),
		RoundTrips:        roundTripCount,
		WinRate:           trade.WinRate,
		AvgHolding:        trade.AverageHoldingPeriod,
		AvgWin:            trade.AverageWin,
		AvgLoss:           trade.AverageLoss,
		ProfitFactor:      trade.ProfitFactor,
		GainLossRatio:     trade.GainLossRatio,
		Turnover:          trade.Turnover,
		PositivePeriods:   trade.NPositivePeriods,
		Trades:            tradeEntries,
	}
}
```

Note: you will need these imports at the top of `report.go`:

```go
import (
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./report/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add report/report.go report/report_test.go
git commit -m "feat: implement report.Build() to construct view model from portfolio metrics"
```

---

## Chunk 3: Terminal Renderer

### Task 7: Shared styles and formatting helpers

**Files:**
- Create: `report/terminal/style.go`

- [ ] **Step 1: Create the styles file**

```go
package terminal

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))

	subHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	sectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("12")).
				MarginTop(1).
				MarginBottom(0)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Bold(true)

	positiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10"))

	negativeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("12"))
)

func fmtPct(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}
	s := fmt.Sprintf("%.2f%%", val*100)
	if val > 0 {
		return positiveStyle.Render(s)
	} else if val < 0 {
		return negativeStyle.Render(s)
	}
	return valueStyle.Render(s)
}

func fmtPctDiff(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}
	s := fmt.Sprintf("%+.2f%%", val*100)
	if val > 0 {
		return positiveStyle.Render(s)
	} else if val < 0 {
		return negativeStyle.Render(s)
	}
	return valueStyle.Render(s)
}

func fmtRatio(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}
	return valueStyle.Render(fmt.Sprintf("%.3f", val))
}

func fmtCurrency(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}
	if val < 0 {
		return negativeStyle.Render(fmt.Sprintf("$%.2f", val))
	}
	return valueStyle.Render(fmt.Sprintf("$%.2f", val))
}

func fmtCurrencyLarge(val float64) string {
	if math.IsNaN(val) {
		return dimStyle.Render("N/A")
	}
	return valueStyle.Render(fmt.Sprintf("$%s", formatWithCommas(val)))
}

func formatWithCommas(val float64) string {
	negative := val < 0
	if negative {
		val = -val
	}
	whole := int64(val)
	frac := val - float64(whole)

	s := fmt.Sprintf("%d", whole)
	if len(s) > 3 {
		var parts []string
		for len(s) > 3 {
			parts = append([]string{s[len(s)-3:]}, parts...)
			s = s[:len(s)-3]
		}
		parts = append([]string{s}, parts...)
		s = strings.Join(parts, ",")
	}

	if frac > 0 {
		s += fmt.Sprintf(".%02d", int(frac*100+0.5))
	}

	if negative {
		s = "-" + s
	}
	return s
}

func fmtDays(days float64) string {
	if math.IsNaN(days) {
		return dimStyle.Render("N/A")
	}
	return valueStyle.Render(fmt.Sprintf("%.0f days", days))
}

func padRight(s string, width int) string {
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

func padLeft(s string, width int) string {
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return strings.Repeat(" ", width-visible) + s
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./report/terminal/...`
Expected: compiles.

- [ ] **Step 3: Commit**

```bash
git add report/terminal/style.go
git commit -m "feat: add shared lipgloss styles for terminal renderer"
```

---

### Task 8: Renderer orchestrator and header

**Files:**
- Create: `report/terminal/renderer.go`
- Create: `report/terminal/header.go`

- [ ] **Step 1: Create the renderer orchestrator**

Create `report/terminal/renderer.go`:

```go
package terminal

import (
	"fmt"
	"io"
	"strings"

	"github.com/penny-vault/pvbt/report"
)

// Render writes the full backtest report to the writer using lipgloss styling.
func Render(rpt report.Report, w io.Writer) error {
	var sb strings.Builder

	renderHeader(&sb, rpt.Header, rpt.HasBenchmark)

	if len(rpt.Warnings) > 0 && rpt.EquityCurve.Times == nil {
		// Insufficient data -- show warnings only.
		for _, warning := range rpt.Warnings {
			sb.WriteString(dimStyle.Render("  " + warning))
			sb.WriteString("\n")
		}
		_, err := fmt.Fprint(w, sb.String())
		return err
	}

	renderEquityCurve(&sb, rpt.EquityCurve, rpt.HasBenchmark)
	renderTrailingReturns(&sb, rpt.TrailingReturns, rpt.HasBenchmark)
	renderAnnualReturns(&sb, rpt.AnnualReturns, rpt.HasBenchmark)
	renderRisk(&sb, rpt.Risk, rpt.HasBenchmark)

	if rpt.HasBenchmark {
		renderRiskVsBenchmark(&sb, rpt.RiskVsBenchmark)
	}

	renderDrawdowns(&sb, rpt.Drawdowns)
	renderMonthlyReturns(&sb, rpt.MonthlyReturns)
	renderTrades(&sb, rpt.Trades)

	if len(rpt.Warnings) > 0 {
		sb.WriteString("\n")
		for _, warning := range rpt.Warnings {
			sb.WriteString(dimStyle.Render("  Warning: " + warning))
			sb.WriteString("\n")
		}
	}

	_, err := fmt.Fprint(w, sb.String())
	return err
}
```

- [ ] **Step 2: Create the header renderer**

Create `report/terminal/header.go`:

```go
package terminal

import (
	"fmt"
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderHeader(sb *strings.Builder, header report.Header, hasBenchmark bool) {
	// Line 1: Strategy name + date range
	nameStr := headerStyle.Render(header.StrategyName)
	dateRange := subHeaderStyle.Render(fmt.Sprintf("%s to %s",
		header.StartDate.Format("2006-01-02"),
		header.EndDate.Format("2006-01-02")))

	sb.WriteString(nameStr)
	// Pad between name and date range.
	nameWidth := lipgloss.Width(nameStr)
	dateWidth := lipgloss.Width(dateRange)
	gap := 80 - nameWidth - dateWidth
	if gap < 2 {
		gap = 2
	}
	sb.WriteString(strings.Repeat(" ", gap))
	sb.WriteString(dateRange)
	sb.WriteString("\n")

	// Line 2: Strategy version + Benchmark
	var line2Left, line2Right string
	if header.StrategyVersion != "" {
		line2Left = subHeaderStyle.Render(fmt.Sprintf("  Strategy: %s %s",
			header.StrategyName, header.StrategyVersion))
	} else {
		line2Left = subHeaderStyle.Render(fmt.Sprintf("  Strategy: %s", header.StrategyName))
	}
	if hasBenchmark && header.Benchmark != "" {
		line2Right = subHeaderStyle.Render(fmt.Sprintf("Benchmark: %s", header.Benchmark))
	}

	sb.WriteString(line2Left)
	if line2Right != "" {
		leftWidth := lipgloss.Width(line2Left)
		rightWidth := lipgloss.Width(line2Right)
		gap := 80 - leftWidth - rightWidth
		if gap < 2 {
			gap = 2
		}
		sb.WriteString(strings.Repeat(" ", gap))
		sb.WriteString(line2Right)
	}
	sb.WriteString("\n")

	// Line 3: Initial / Final / Elapsed
	initial := subHeaderStyle.Render(fmt.Sprintf("  Initial: %s", fmtCurrencyLarge(header.InitialCash)))
	final := valueStyle.Render(fmt.Sprintf("Final: %s", fmtCurrencyLarge(header.FinalValue)))

	elapsed := subHeaderStyle.Render(fmt.Sprintf("Elapsed: %.1fs (%d steps)",
		header.Elapsed.Seconds(), header.Steps))

	sb.WriteString(initial)
	sb.WriteString("        ")
	sb.WriteString(final)

	finalWidth := lipgloss.Width(initial) + 8 + lipgloss.Width(final)
	elapsedWidth := lipgloss.Width(elapsed)
	gap = 80 - finalWidth - elapsedWidth
	if gap < 2 {
		gap = 2
	}
	sb.WriteString(strings.Repeat(" ", gap))
	sb.WriteString(elapsed)
	sb.WriteString("\n")
}
```

The imports for `header.go` must include:
```go
import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/report"
)
```

- [ ] **Step 3: Create stub section renderers**

The orchestrator references functions that don't exist yet. Create stubs so the package compiles.

Create stubs for each section so the package compiles. Each stub is a no-op:

`report/terminal/chart.go`:
```go
package terminal

import (
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderEquityCurve(sb *strings.Builder, ec report.EquityCurve, hasBenchmark bool) {
	// TODO: implement with ntcharts
}
```

`report/terminal/returns.go`:
```go
package terminal

import (
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderTrailingReturns(sb *strings.Builder, tr report.TrailingReturns, hasBenchmark bool) {
	// TODO: implement
}

func renderAnnualReturns(sb *strings.Builder, ar report.AnnualReturns, hasBenchmark bool) {
	// TODO: implement
}
```

`report/terminal/risk.go`:
```go
package terminal

import (
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderRisk(sb *strings.Builder, risk report.Risk, hasBenchmark bool) {
	// TODO: implement
}

func renderRiskVsBenchmark(sb *strings.Builder, rvb report.RiskVsBenchmark) {
	// TODO: implement
}
```

`report/terminal/drawdowns.go`:
```go
package terminal

import (
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderDrawdowns(sb *strings.Builder, dd report.Drawdowns) {
	// TODO: implement
}
```

`report/terminal/monthly.go`:
```go
package terminal

import (
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderMonthlyReturns(sb *strings.Builder, mr report.MonthlyReturns) {
	// TODO: implement
}
```

`report/terminal/trades.go`:
```go
package terminal

import (
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderTrades(sb *strings.Builder, trades report.Trades) {
	// TODO: implement
}
```

- [ ] **Step 4: Verify everything compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./report/...`
Expected: compiles without errors.

- [ ] **Step 5: Commit**

```bash
git add report/terminal/
git commit -m "feat: add terminal renderer scaffolding with header and stubs"
```

---

### Task 9: Implement trailing returns and annual returns renderers

**Files:**
- Modify: `report/terminal/returns.go`

- [ ] **Step 1: Implement renderTrailingReturns**

Replace the stub in `report/terminal/returns.go`:

```go
package terminal

import (
	"fmt"
	"math"
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderTrailingReturns(sb *strings.Builder, tr report.TrailingReturns, hasBenchmark bool) {
	if len(tr.Periods) == 0 {
		return
	}

	sb.WriteString(sectionTitleStyle.Render("  Trailing Returns"))
	sb.WriteString("\n")

	// Column widths.
	labelWidth := 20
	colWidth := 10

	// Header row.
	sb.WriteString(padRight("", labelWidth))
	for _, period := range tr.Periods {
		sb.WriteString(padLeft(tableHeaderStyle.Render(period), colWidth))
	}
	sb.WriteString("\n")

	// Strategy row.
	sb.WriteString(padRight(labelStyle.Render("    Strategy"), labelWidth))
	for _, val := range tr.Strategy {
		sb.WriteString(padLeft(fmtPct(val), colWidth))
	}
	sb.WriteString("\n")

	// Benchmark row.
	if hasBenchmark {
		sb.WriteString(padRight(labelStyle.Render("    Benchmark"), labelWidth))
		for _, val := range tr.Benchmark {
			sb.WriteString(padLeft(fmtPct(val), colWidth))
		}
		sb.WriteString("\n")

		// +/- row.
		sb.WriteString(padRight(labelStyle.Render("    +/-"), labelWidth))
		for idx := range tr.Strategy {
			diff := math.NaN()
			if !math.IsNaN(tr.Strategy[idx]) && !math.IsNaN(tr.Benchmark[idx]) {
				diff = tr.Strategy[idx] - tr.Benchmark[idx]
			}
			sb.WriteString(padLeft(fmtPctDiff(diff), colWidth))
		}
		sb.WriteString("\n")
	}
}

func renderAnnualReturns(sb *strings.Builder, ar report.AnnualReturns, hasBenchmark bool) {
	if len(ar.Years) == 0 {
		return
	}

	sb.WriteString(sectionTitleStyle.Render("  Annual Returns"))
	sb.WriteString("\n")

	labelWidth := 20
	colWidth := 10

	// Header row.
	sb.WriteString(padRight("", labelWidth))
	for _, year := range ar.Years {
		sb.WriteString(padLeft(tableHeaderStyle.Render(fmt.Sprintf("%d", year)), colWidth))
	}
	sb.WriteString("\n")

	// Strategy row.
	sb.WriteString(padRight(labelStyle.Render("    Strategy"), labelWidth))
	for _, val := range ar.Strategy {
		sb.WriteString(padLeft(fmtPct(val), colWidth))
	}
	sb.WriteString("\n")

	// Benchmark row.
	if hasBenchmark && len(ar.Benchmark) > 0 {
		sb.WriteString(padRight(labelStyle.Render("    Benchmark"), labelWidth))
		for _, val := range ar.Benchmark {
			sb.WriteString(padLeft(fmtPct(val), colWidth))
		}
		sb.WriteString("\n")

		// +/- row.
		sb.WriteString(padRight(labelStyle.Render("    +/-"), labelWidth))
		for idx := range ar.Strategy {
			diff := math.NaN()
			if idx < len(ar.Benchmark) && !math.IsNaN(ar.Strategy[idx]) && !math.IsNaN(ar.Benchmark[idx]) {
				diff = ar.Strategy[idx] - ar.Benchmark[idx]
			}
			sb.WriteString(padLeft(fmtPctDiff(diff), colWidth))
		}
		sb.WriteString("\n")
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./report/terminal/...`

- [ ] **Step 3: Commit**

```bash
git add report/terminal/returns.go
git commit -m "feat: implement trailing and annual returns terminal renderers"
```

---

### Task 10: Implement risk section renderers

**Files:**
- Modify: `report/terminal/risk.go`

- [ ] **Step 1: Implement renderRisk and renderRiskVsBenchmark**

Replace the stub in `report/terminal/risk.go`:

```go
package terminal

import (
	"fmt"
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderRisk(sb *strings.Builder, risk report.Risk, hasBenchmark bool) {
	sb.WriteString(sectionTitleStyle.Render("  Risk"))
	sb.WriteString("\n")

	labelWidth := 24
	colWidth := 14

	// Header row.
	sb.WriteString(padRight("", labelWidth))
	sb.WriteString(padLeft(tableHeaderStyle.Render("Strategy"), colWidth))
	if hasBenchmark {
		sb.WriteString(padLeft(tableHeaderStyle.Render("Benchmark"), colWidth))
	}
	sb.WriteString("\n")

	rows := []struct {
		label string
		vals  [2]float64
		fmtFn func(float64) string
	}{
		{"    Max Drawdown", risk.MaxDrawdown, fmtPct},
		{"    Volatility", risk.Volatility, fmtPct},
		{"    Downside Deviation", risk.DownsideDeviation, fmtPct},
		{"    Sharpe", risk.Sharpe, fmtRatio},
		{"    Sortino", risk.Sortino, fmtRatio},
		{"    Calmar", risk.Calmar, fmtRatio},
		{"    Ulcer Index", risk.UlcerIndex, fmtRatio},
		{"    Value at Risk (95%)", risk.ValueAtRisk, fmtPct},
		{"    Skewness", risk.Skewness, fmtRatio},
		{"    Excess Kurtosis", risk.ExcessKurtosis, fmtRatio},
	}

	for _, row := range rows {
		sb.WriteString(padRight(labelStyle.Render(row.label), labelWidth))
		sb.WriteString(padLeft(row.fmtFn(row.vals[0]), colWidth))
		if hasBenchmark {
			sb.WriteString(padLeft(row.fmtFn(row.vals[1]), colWidth))
		}
		sb.WriteString("\n")
	}
}

func renderRiskVsBenchmark(sb *strings.Builder, rvb report.RiskVsBenchmark) {
	sb.WriteString(sectionTitleStyle.Render("  Risk vs Benchmark"))
	sb.WriteString("\n")

	// 2x4 grid layout.
	type cell struct {
		label string
		value string
	}

	leftCol := []cell{
		{"Beta", fmtRatio(rvb.Beta)},
		{"R-Squared", fmtRatio(rvb.RSquared)},
		{"Info Ratio", fmtRatio(rvb.InformationRatio)},
		{"Upside Capture", fmtPct(rvb.UpsideCapture)},
	}

	rightCol := []cell{
		{"Alpha", fmtPct(rvb.Alpha)},
		{"Tracking Error", fmtPct(rvb.TrackingError)},
		{"Treynor", fmtRatio(rvb.Treynor)},
		{"Downside Capture", fmtPct(rvb.DownsideCapture)},
	}

	labelWidth := 20
	valWidth := 14

	for idx := range leftCol {
		left := fmt.Sprintf("    %s%s",
			padRight(labelStyle.Render(leftCol[idx].label), labelWidth),
			padLeft(leftCol[idx].value, valWidth))
		right := fmt.Sprintf("    %s%s",
			padRight(labelStyle.Render(rightCol[idx].label), labelWidth),
			padLeft(rightCol[idx].value, valWidth))
		sb.WriteString(left)
		sb.WriteString(right)
		sb.WriteString("\n")
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./report/terminal/...`

- [ ] **Step 3: Commit**

```bash
git add report/terminal/risk.go
git commit -m "feat: implement risk and risk-vs-benchmark terminal renderers"
```

---

### Task 11: Implement drawdowns, monthly returns, and trades renderers

**Files:**
- Modify: `report/terminal/drawdowns.go`
- Modify: `report/terminal/monthly.go`
- Modify: `report/terminal/trades.go`

- [ ] **Step 1: Implement renderDrawdowns**

Replace stub in `report/terminal/drawdowns.go`:

```go
package terminal

import (
	"fmt"
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderDrawdowns(sb *strings.Builder, dd report.Drawdowns) {
	if len(dd.Entries) == 0 {
		return
	}

	sb.WriteString(sectionTitleStyle.Render("  Drawdowns"))
	sb.WriteString("\n")

	// Table header.
	sb.WriteString(fmt.Sprintf("    %s%s%s%s%s%s\n",
		padRight(tableHeaderStyle.Render("#"), 6),
		padRight(tableHeaderStyle.Render("Start"), 14),
		padRight(tableHeaderStyle.Render("End"), 14),
		padRight(tableHeaderStyle.Render("Recovery"), 14),
		padLeft(tableHeaderStyle.Render("Depth"), 10),
		padLeft(tableHeaderStyle.Render("Duration"), 12),
	))

	for idx, entry := range dd.Entries {
		recovery := "ongoing"
		if !entry.Recovery.IsZero() {
			recovery = entry.Recovery.Format("2006-01-02")
		}

		sb.WriteString(fmt.Sprintf("    %s%s%s%s%s%s\n",
			padRight(valueStyle.Render(fmt.Sprintf("%d", idx+1)), 6),
			padRight(valueStyle.Render(entry.Start.Format("2006-01-02")), 14),
			padRight(valueStyle.Render(entry.End.Format("2006-01-02")), 14),
			padRight(valueStyle.Render(recovery), 14),
			padLeft(fmtPct(entry.Depth), 10),
			padLeft(valueStyle.Render(fmt.Sprintf("%d days", entry.Days)), 12),
		))
	}
}
```

- [ ] **Step 2: Implement renderMonthlyReturns**

Replace stub in `report/terminal/monthly.go`:

```go
package terminal

import (
	"fmt"
	"math"
	"strings"

	"github.com/penny-vault/pvbt/report"
)

var monthNames = []string{
	"Jan", "Feb", "Mar", "Apr", "May", "Jun",
	"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
}

func renderMonthlyReturns(sb *strings.Builder, mr report.MonthlyReturns) {
	if len(mr.Years) == 0 {
		return
	}

	sb.WriteString(sectionTitleStyle.Render("  Monthly Returns (%)"))
	sb.WriteString("\n")

	yearWidth := 8
	colWidth := 7

	// Header row.
	sb.WriteString(padRight("", yearWidth))
	for _, month := range monthNames {
		sb.WriteString(padLeft(tableHeaderStyle.Render(month), colWidth))
	}
	sb.WriteString(padLeft(tableHeaderStyle.Render("Year"), colWidth))
	sb.WriteString("\n")

	// Data rows.
	for yearIdx, year := range mr.Years {
		sb.WriteString(padRight(valueStyle.Render(fmt.Sprintf("  %d", year)), yearWidth))

		yearTotal := 0.0
		hasAny := false

		for monthIdx := 0; monthIdx < 12; monthIdx++ {
			val := mr.Values[yearIdx][monthIdx]
			if math.IsNaN(val) {
				sb.WriteString(padLeft(dimStyle.Render(""), colWidth))
			} else {
				pctStr := fmt.Sprintf("%.2f", val*100)
				if val > 0 {
					sb.WriteString(padLeft(positiveStyle.Render(pctStr), colWidth))
				} else if val < 0 {
					sb.WriteString(padLeft(negativeStyle.Render(pctStr), colWidth))
				} else {
					sb.WriteString(padLeft(valueStyle.Render(pctStr), colWidth))
				}
				// Compound for year total.
				if !hasAny {
					yearTotal = 1.0
					hasAny = true
				}
				yearTotal *= (1 + val)
			}
		}

		// Year total.
		if hasAny {
			annualReturn := yearTotal - 1
			sb.WriteString(padLeft(fmtPct(annualReturn), colWidth))
		} else {
			sb.WriteString(padLeft(dimStyle.Render(""), colWidth))
		}

		sb.WriteString("\n")
	}
}
```

- [ ] **Step 3: Implement renderTrades**

Replace stub in `report/terminal/trades.go`:

```go
package terminal

import (
	"fmt"
	"strings"

	"github.com/penny-vault/pvbt/report"
)

func renderTrades(sb *strings.Builder, trades report.Trades) {
	sb.WriteString(sectionTitleStyle.Render(fmt.Sprintf(
		"  Trades (%d transactions)", trades.TotalTransactions)))
	sb.WriteString("\n")

	// Summary stats in 2-column layout.
	type statPair struct {
		leftLabel, leftVal   string
		rightLabel, rightVal string
	}

	pairs := []statPair{
		{"Win Rate", fmtPct(trades.WinRate), "Avg Holding", fmtDays(trades.AvgHolding)},
		{"Avg Win", fmtCurrency(trades.AvgWin), "Avg Loss", fmtCurrency(trades.AvgLoss)},
		{"Profit Factor", fmtRatio(trades.ProfitFactor), "Gain/Loss", fmtRatio(trades.GainLossRatio)},
		{"Turnover", fmtPct(trades.Turnover), "Positive Periods", fmtPct(trades.PositivePeriods)},
	}

	labelWidth := 20
	valWidth := 14

	for _, pair := range pairs {
		sb.WriteString(fmt.Sprintf("    %s%s    %s%s\n",
			padRight(labelStyle.Render(pair.leftLabel), labelWidth),
			padLeft(pair.leftVal, valWidth),
			padRight(labelStyle.Render(pair.rightLabel), labelWidth),
			padLeft(pair.rightVal, valWidth),
		))
	}

	// Recent trades table (last 10).
	if len(trades.Trades) == 0 {
		return
	}

	sb.WriteString("\n")

	// Table header.
	sb.WriteString(fmt.Sprintf("    %s%s%s%s%s%s\n",
		padRight(tableHeaderStyle.Render("Date"), 14),
		padRight(tableHeaderStyle.Render("Action"), 8),
		padRight(tableHeaderStyle.Render("Ticker"), 10),
		padLeft(tableHeaderStyle.Render("Shares"), 10),
		padLeft(tableHeaderStyle.Render("Price"), 12),
		padLeft(tableHeaderStyle.Render("Amount"), 14),
	))

	// Show last 10 trades.
	startIdx := 0
	if len(trades.Trades) > 10 {
		startIdx = len(trades.Trades) - 10
		sb.WriteString(dimStyle.Render(fmt.Sprintf(
			"    ... and %d earlier transactions\n", startIdx)))
	}

	for idx := startIdx; idx < len(trades.Trades); idx++ {
		trade := trades.Trades[idx]
		actionStyle := valueStyle
		if trade.Action == "BUY" {
			actionStyle = positiveStyle
		} else if trade.Action == "SELL" {
			actionStyle = negativeStyle
		}

		sb.WriteString(fmt.Sprintf("    %s%s%s%s%s%s\n",
			padRight(valueStyle.Render(trade.Date.Format("2006-01-02")), 14),
			padRight(actionStyle.Render(trade.Action), 8),
			padRight(valueStyle.Render(trade.Ticker), 10),
			padLeft(valueStyle.Render(fmt.Sprintf("%.0f", trade.Shares)), 10),
			padLeft(fmtCurrency(trade.Price), 12),
			padLeft(fmtCurrency(trade.Amount), 14),
		))
	}
}
```

- [ ] **Step 4: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./report/...`
Expected: compiles.

- [ ] **Step 5: Commit**

```bash
git add report/terminal/drawdowns.go report/terminal/monthly.go report/terminal/trades.go
git commit -m "feat: implement drawdowns, monthly returns, and trades terminal renderers"
```

---

### Task 12: Implement equity curve chart with ntcharts

**Files:**
- Modify: `report/terminal/chart.go`

- [ ] **Step 1: Implement renderEquityCurve**

Replace the stub in `report/terminal/chart.go`. Reference the existing
`cli/explore_graph.go` for ntcharts usage patterns:

```go
package terminal

import (
	"fmt"
	"math"
	"strings"

	"github.com/NimbleMarkets/ntcharts/canvas"
	"github.com/NimbleMarkets/ntcharts/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/canvas/runes"
	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/report"
)

var (
	strategyColor  = lipgloss.Color("12") // blue
	benchmarkColor = lipgloss.Color("10") // green
)

func renderEquityCurve(sb *strings.Builder, ec report.EquityCurve, hasBenchmark bool) {
	if len(ec.Times) < 2 || len(ec.StrategyValues) < 2 {
		return
	}

	sb.WriteString(sectionTitleStyle.Render("  Equity Curve"))
	sb.WriteString("\n")

	chartWidth := 60
	chartHeight := 12
	leftMargin := 10

	// Find global min/max across both series.
	minVal := math.Inf(1)
	maxVal := math.Inf(-1)

	for _, val := range ec.StrategyValues {
		if val < minVal {
			minVal = val
		}
		if val > maxVal {
			maxVal = val
		}
	}

	if hasBenchmark && len(ec.BenchmarkValues) > 0 {
		for _, val := range ec.BenchmarkValues {
			if !math.IsNaN(val) {
				if val < minVal {
					minVal = val
				}
				if val > maxVal {
					maxVal = val
				}
			}
		}
	}

	valRange := maxVal - minVal
	if valRange == 0 {
		valRange = 1
	}

	// Add 5% padding top and bottom.
	padding := valRange * 0.05
	minVal -= padding
	maxVal += padding
	valRange = maxVal - minVal

	chart := canvas.New(chartWidth, chartHeight)
	numPoints := len(ec.StrategyValues)
	xAxisRow := chartHeight - 1

	// Draw strategy line.
	strategySeqY := resampleToCanvas(ec.StrategyValues, chartWidth, chartHeight, minVal, valRange, xAxisRow)
	strategyStyle := lipgloss.NewStyle().Foreground(strategyColor)
	graph.DrawLineSequence(&chart, false, 0, strategySeqY, runes.ArcLineStyle, strategyStyle)

	// Draw benchmark line.
	if hasBenchmark && len(ec.BenchmarkValues) > 0 {
		bmSeqY := resampleToCanvas(ec.BenchmarkValues, chartWidth, chartHeight, minVal, valRange, xAxisRow)
		bmStyle := lipgloss.NewStyle().Foreground(benchmarkColor)
		graph.DrawLineSequence(&chart, false, 0, bmSeqY, runes.ArcLineStyle, bmStyle)
	}

	// Build output with Y-axis labels.
	canvasView := chart.View()
	canvasLines := strings.Split(canvasView, "\n")

	gridInterval := chartHeight / 4
	if gridInterval < 2 {
		gridInterval = 2
	}

	for row := 0; row < chartHeight && row < len(canvasLines); row++ {
		yLabel := strings.Repeat(" ", leftMargin)
		if row == 0 || row == chartHeight-1 || row%gridInterval == 0 {
			val := maxVal - float64(row)/float64(chartHeight-1)*valRange
			yLabel = fmt.Sprintf("%9s ", formatDollarAxis(val))
		}
		sb.WriteString(dimStyle.Render(yLabel))
		sb.WriteString(canvasLines[row])
		sb.WriteString("\n")
	}

	// X-axis dates.
	startDate := ec.Times[0].Format("Jan 2006")
	endDate := ec.Times[len(ec.Times)-1].Format("Jan 2006")
	xAxis := strings.Repeat(" ", leftMargin) + startDate +
		strings.Repeat(" ", max(0, chartWidth-len(startDate)-len(endDate))) + endDate
	sb.WriteString(dimStyle.Render(xAxis))
	sb.WriteString("\n")

	// Legend.
	sb.WriteString("\n")
	strategySwatch := lipgloss.NewStyle().Foreground(strategyColor).Render("--")
	sb.WriteString(fmt.Sprintf("    %s Strategy", strategySwatch))
	if hasBenchmark {
		bmSwatch := lipgloss.NewStyle().Foreground(benchmarkColor).Render("--")
		sb.WriteString(fmt.Sprintf("    %s Benchmark", bmSwatch))
	}
	sb.WriteString("\n")

	_ = numPoints
}

func resampleToCanvas(values []float64, chartWidth, chartHeight int, minVal, valRange float64, xAxisRow int) []int {
	seqY := make([]int, chartWidth)
	numPoints := len(values)

	for col := 0; col < chartWidth; col++ {
		idx := col * numPoints / chartWidth
		if idx >= numPoints {
			idx = numPoints - 1
		}
		val := values[idx]
		cartY := int((val - minVal) / valRange * float64(chartHeight-1))
		if cartY < 0 {
			cartY = 0
		}
		if cartY >= chartHeight {
			cartY = chartHeight - 1
		}
		seqY[col] = canvas.CanvasYCoordinate(xAxisRow, cartY)
	}

	return seqY
}

func formatDollarAxis(val float64) string {
	if val >= 1_000_000 {
		return fmt.Sprintf("$%.1fM", val/1_000_000)
	}
	if val >= 1_000 {
		return fmt.Sprintf("$%.0fk", val/1_000)
	}
	return fmt.Sprintf("$%.0f", val)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./report/...`
Expected: compiles.

- [ ] **Step 3: Commit**

```bash
git add report/terminal/chart.go
git commit -m "feat: implement equity curve chart renderer with ntcharts"
```

---

### Task 12b: Smoke test for terminal renderer

**Files:**
- Create: `report/terminal/renderer_test.go`

- [ ] **Step 1: Write a smoke test**

Create `report/terminal/renderer_test.go`:

```go
package terminal_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/report/terminal"
)

func TestRenderDoesNotPanic(t *testing.T) {
	rpt := report.Report{
		Header: report.Header{
			StrategyName: "Test Strategy",
			StartDate:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EndDate:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			InitialCash:  100000,
			FinalValue:   120000,
			Elapsed:      time.Second,
			Steps:        12,
		},
		HasBenchmark: false,
	}

	var buf bytes.Buffer
	err := terminal.Render(rpt, &buf)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Render produced empty output")
	}
}
```

- [ ] **Step 2: Run test**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./report/terminal/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add report/terminal/renderer_test.go
git commit -m "test: add smoke test for terminal renderer"
```

---

## Chunk 4: CLI Integration

### Task 13: Wire up report in runBacktest and delete summary.go

**Files:**
- Modify: `cli/backtest.go:58-157`
- Delete: `cli/summary.go`

- [ ] **Step 1: Update runBacktest to add timing, StrategyInfo, and report**

Modify `cli/backtest.go`. The key changes to `runBacktest`:

1. Add `time.Now()` before `eng.Backtest()` and `time.Since()` after.
2. Call `engine.DescribeStrategy(eng)` after Backtest returns.
3. Replace `printSummary(result)` with `report.Build()` + `terminal.Render()`.

The updated `runBacktest` function:

```go
func runBacktest(cmd *cobra.Command, strategy engine.Strategy) error {
	ctx := log.Logger.WithContext(context.Background())

	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return fmt.Errorf("load America/New_York timezone: %w", err)
	}

	startStr, _ := cmd.Flags().GetString("start")
	start, err := time.ParseInLocation("2006-01-02", startStr, nyc)
	if err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}

	endStr, _ := cmd.Flags().GetString("end")
	end, err := time.ParseInLocation("2006-01-02", endStr, nyc)
	if err != nil {
		return fmt.Errorf("invalid end date: %w", err)
	}

	cash, _ := cmd.Flags().GetFloat64("cash")
	fullID, shortID := runID()

	outputPath, _ := cmd.Flags().GetString("output")
	if outputPath == "" {
		outputPath = defaultOutputPath(strategy.Name(), start, end, shortID)
	}

	log.Info().
		Str("strategy", strategy.Name()).
		Time("start", start).
		Time("end", end).
		Float64("cash", cash).
		Str("output", outputPath).
		Str("run_id", fullID).
		Msg("starting backtest")

	applyStrategyFlags(strategy)

	useTUI, _ := cmd.Flags().GetBool("tui")
	if useTUI {
		return runBacktestWithTUI(strategy)
	}

	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	holidays, err := provider.FetchMarketHolidays(ctx)
	if err != nil {
		return fmt.Errorf("load market holidays: %w", err)
	}

	tradecron.SetMarketHolidays(holidays)
	log.Info().Int("holidays", len(holidays)).Msg("loaded market holidays")

	acct := portfolio.New(
		portfolio.WithCash(cash, start),
		portfolio.WithAllMetrics(),
	)

	eng := engine.New(strategy,
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithAccount(acct),
	)
	defer eng.Close()

	startTime := time.Now()

	result, err := eng.Backtest(ctx, start, end)
	if err != nil {
		return fmt.Errorf("backtest failed: %w", err)
	}

	elapsed := time.Since(startTime)

	// Set metadata on the portfolio.
	result.SetMetadata("run_id", fullID)
	result.SetMetadata("strategy", strategy.Name())
	result.SetMetadata("start", start.Format("2006-01-02"))
	result.SetMetadata("end", end.Format("2006-01-02"))

	params := strategyParams(strategy)
	for k, v := range params {
		result.SetMetadata(fmt.Sprintf("param_%s", k), fmt.Sprintf("%v", v))
	}

	if err := acct.ToSQLite(outputPath); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	log.Info().Str("path", outputPath).Msg("backtest output written")

	info := engine.DescribeStrategy(eng)

	steps := 0
	if result.PerfData() != nil {
		steps = result.PerfData().Len()
	}

	rpt, err := backtestReport.Build(result, info, backtestReport.RunMeta{
		Elapsed:     elapsed,
		Steps:       steps,
		InitialCash: cash,
	})
	if err != nil {
		log.Warn().Err(err).Msg("some report metrics failed")
	}

	if err := terminal.Render(rpt, os.Stdout); err != nil {
		return fmt.Errorf("rendering report: %w", err)
	}

	return nil
}
```

Update the imports at the top of `cli/backtest.go`:

```go
import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	backtestReport "github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/report/terminal"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)
```

- [ ] **Step 2: Delete cli/summary.go**

Run: `rm /Users/jdf/Developer/penny-vault/pvbt/cli/summary.go`

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./...`
Expected: compiles without errors.

- [ ] **Step 4: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./...`
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add cli/backtest.go report/
git rm cli/summary.go
git commit -m "feat: replace CLI summary with structured backtest report"
```

---

### Task 14: Update CHANGELOG

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Add under the `[Unreleased]` section:

```markdown
### Added

- Rich backtest report with equity curve, trailing/annual/monthly returns, risk metrics, drawdowns, and trade log
- Benchmark targeting for performance metrics via `.Benchmark()` query builder method
- MonthlyReturns, AnnualReturns, and DrawdownDetails account methods
- Terminal renderer using lipgloss and ntcharts
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add backtest report to changelog"
```

---

### Task 15: Final integration test

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -v`
Expected: all tests pass.

- [ ] **Step 2: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./...`
Expected: no errors.

- [ ] **Step 3: Verify build**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build -o /dev/null ./...`
Expected: clean build.
