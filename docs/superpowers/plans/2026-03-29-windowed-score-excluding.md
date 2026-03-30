# Fix WindowedScoreExcluding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix `WindowedScoreExcluding` so KFold in-sample scores exclude the test fold, producing correct overfitting diagnostics.

**Architecture:** Add a `View(start, end)` method to `Portfolio` that returns a windowed portfolio. The study package computes IS segments via `subtractRanges`, calls `View` for each segment, and averages the scores. No exclusion concept at the portfolio level.

**Tech Stack:** Go, Ginkgo/Gomega, portfolio/study packages

---

### Task 1: Add `View` to Portfolio Interface and Implement `viewedPortfolio`

**Files:**
- Modify: `portfolio/portfolio.go:39-160` (add `View` to `Portfolio` interface)
- Create: `portfolio/viewed_portfolio.go`
- Create: `portfolio/viewed_portfolio_test.go`

- [ ] **Step 1: Write the failing test**

Create `portfolio/viewed_portfolio_test.go`:

```go
// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("View", func() {
	var (
		acct   *portfolio.Account
		dates  []time.Time
		equity []float64
	)

	BeforeEach(func() {
		dates = []time.Time{
			time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 2, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 3, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 4, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 8, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 11, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 12, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2021, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		equity = []float64{
			10_000, 10_200, 10_400, 10_600, 10_800,
			11_000, 11_200, 11_400, 11_600, 11_800,
			12_000, 12_200, 12_400,
		}

		portfolioAsset := asset.Asset{
			CompositeFigi: "_PORTFOLIO_",
			Ticker:        "_PORTFOLIO_",
		}

		perfDF, err := data.NewDataFrame(
			dates,
			[]asset.Asset{portfolioAsset},
			[]data.Metric{data.PortfolioEquity},
			data.Daily,
			[][]float64{equity},
		)
		Expect(err).NotTo(HaveOccurred())

		acct = portfolio.New(portfolio.WithCash(equity[0], dates[0]))
		acct.SetPerfData(perfDF)
	})

	It("returns a Portfolio whose metric matches AbsoluteWindow", func() {
		viewStart := dates[2]  // 2020-03-02
		viewEnd := dates[8]    // 2020-09-01

		// Compute via AbsoluteWindow (existing API).
		expected, err := acct.PerformanceMetric(portfolio.CAGR).AbsoluteWindow(viewStart, viewEnd).Value()
		Expect(err).NotTo(HaveOccurred())

		// Compute via View (new API).
		viewed := acct.View(viewStart, viewEnd)
		actual, err := viewed.PerformanceMetric(portfolio.CAGR).Value()
		Expect(err).NotTo(HaveOccurred())

		Expect(actual).To(BeNumerically("~", expected, 1e-12))
	})

	It("satisfies the PortfolioStats interface", func() {
		viewed := acct.View(dates[0], dates[len(dates)-1])
		_, ok := viewed.(portfolio.PortfolioStats)
		Expect(ok).To(BeTrue(), "viewed portfolio should satisfy PortfolioStats")
	})

	It("passes through point-in-time methods from the account", func() {
		viewed := acct.View(dates[0], dates[len(dates)-1])
		Expect(viewed.Cash()).To(Equal(acct.Cash()))
		Expect(viewed.Benchmark()).To(Equal(acct.Benchmark()))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./portfolio/ --focus "View"`
Expected: FAIL -- `acct.View` is undefined, `SetPerfData` is undefined.

- [ ] **Step 3: Add `SetPerfData` to Account**

The test needs to inject a perfData DataFrame. Check if `SetPerfData` already exists on Account. If not, add it to `portfolio/account.go` (this is a test helper -- add near the other Set methods):

```go
// SetPerfData replaces the performance DataFrame. This is intended for
// testing scenarios where a synthetic equity curve is needed.
func (a *Account) SetPerfData(df *data.DataFrame) {
	a.perfData = df
}
```

Note: `perfData` is a lowercase field, so `SetPerfData` must be on `*Account` in the same package. If `perfData` is already settable through an existing mechanism, use that instead. Check `account.go` for any existing setter before adding this.

- [ ] **Step 4: Add `View` to the `Portfolio` interface**

In `portfolio/portfolio.go`, add after the `StepwiseFactorAnalysis` method (before the closing brace of the `Portfolio` interface at line 160):

```go
	// View returns a read-only Portfolio restricted to the date range
	// [start, end]. Metrics computed on the view use only data within
	// this range.
	View(start, end time.Time) Portfolio
```

- [ ] **Step 5: Implement `viewedPortfolio`**

Create `portfolio/viewed_portfolio.go`:

```go
// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package portfolio

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// viewedPortfolio is a read-only view of an Account restricted to a date
// range. It satisfies Portfolio and PortfolioStats so callers can use it
// anywhere a ReportablePortfolio is expected.
type viewedPortfolio struct {
	acct  *Account
	stats PortfolioStats
	start time.Time
	end   time.Time
}

// Compile-time check that viewedPortfolio satisfies both interfaces.
var (
	_ Portfolio      = (*viewedPortfolio)(nil)
	_ PortfolioStats = (*viewedPortfolio)(nil)
)

// --- Portfolio methods: delegate to acct ---

func (vp *viewedPortfolio) Cash() float64                     { return vp.acct.Cash() }
func (vp *viewedPortfolio) Value() float64                    { return vp.acct.Value() }
func (vp *viewedPortfolio) Position(aa asset.Asset) float64   { return vp.acct.Position(aa) }
func (vp *viewedPortfolio) PositionValue(aa asset.Asset) float64 { return vp.acct.PositionValue(aa) }
func (vp *viewedPortfolio) Holdings() map[asset.Asset]float64 { return vp.acct.Holdings() }
func (vp *viewedPortfolio) Transactions() []Transaction       { return vp.acct.Transactions() }
func (vp *viewedPortfolio) Prices() *data.DataFrame           { return vp.acct.Prices() }
func (vp *viewedPortfolio) SetMetadata(key, value string)     { vp.acct.SetMetadata(key, value) }
func (vp *viewedPortfolio) GetMetadata(key string) string     { return vp.acct.GetMetadata(key) }
func (vp *viewedPortfolio) Annotations() []Annotation         { return vp.acct.Annotations() }
func (vp *viewedPortfolio) TradeDetails() []TradeDetail       { return vp.acct.TradeDetails() }
func (vp *viewedPortfolio) Equity() float64                   { return vp.acct.Equity() }
func (vp *viewedPortfolio) LongMarketValue() float64          { return vp.acct.LongMarketValue() }
func (vp *viewedPortfolio) ShortMarketValue() float64         { return vp.acct.ShortMarketValue() }
func (vp *viewedPortfolio) MarginRatio() float64              { return vp.acct.MarginRatio() }
func (vp *viewedPortfolio) MarginDeficiency() float64         { return vp.acct.MarginDeficiency() }
func (vp *viewedPortfolio) BuyingPower() float64              { return vp.acct.BuyingPower() }
func (vp *viewedPortfolio) Benchmark() asset.Asset            { return vp.acct.Benchmark() }

func (vp *viewedPortfolio) Summary() (Summary, error) { return vp.acct.Summary() }
func (vp *viewedPortfolio) RiskMetrics() (RiskMetrics, error) { return vp.acct.RiskMetrics() }
func (vp *viewedPortfolio) TaxMetrics() (TaxMetrics, error)   { return vp.acct.TaxMetrics() }
func (vp *viewedPortfolio) TradeMetrics() (TradeMetrics, error) { return vp.acct.TradeMetrics() }
func (vp *viewedPortfolio) WithdrawalMetrics() (WithdrawalMetrics, error) {
	return vp.acct.WithdrawalMetrics()
}

func (vp *viewedPortfolio) FactorAnalysis(factors *data.DataFrame) (*FactorRegression, error) {
	return vp.acct.FactorAnalysis(factors)
}

func (vp *viewedPortfolio) StepwiseFactorAnalysis(factors *data.DataFrame) (*StepwiseResult, error) {
	return vp.acct.StepwiseFactorAnalysis(factors)
}

// --- Overridden Portfolio methods ---

// PerfData returns the performance DataFrame restricted to the view's date range.
func (vp *viewedPortfolio) PerfData() *data.DataFrame {
	df := vp.acct.PerfData()
	if df == nil {
		return nil
	}

	return df.Between(vp.start, vp.end)
}

// PerformanceMetric returns a query with the view's date range pre-applied.
func (vp *viewedPortfolio) PerformanceMetric(mm PerformanceMetric) PerformanceMetricQuery {
	return vp.acct.PerformanceMetric(mm).AbsoluteWindow(vp.start, vp.end)
}

// View returns a new view of the underlying account for the given range.
func (vp *viewedPortfolio) View(start, end time.Time) Portfolio {
	return vp.acct.View(start, end)
}

// --- PortfolioStats methods: delegate to windowed stats ---

func (vp *viewedPortfolio) Returns(ctx context.Context, window *Period) *data.DataFrame {
	return vp.stats.Returns(ctx, window)
}

func (vp *viewedPortfolio) ExcessReturns(ctx context.Context, window *Period) *data.DataFrame {
	return vp.stats.ExcessReturns(ctx, window)
}

func (vp *viewedPortfolio) Drawdown(ctx context.Context, window *Period) *data.DataFrame {
	return vp.stats.Drawdown(ctx, window)
}

func (vp *viewedPortfolio) BenchmarkReturns(ctx context.Context, window *Period) *data.DataFrame {
	return vp.stats.BenchmarkReturns(ctx, window)
}

func (vp *viewedPortfolio) EquitySeries(ctx context.Context, window *Period) *data.DataFrame {
	return vp.stats.EquitySeries(ctx, window)
}

func (vp *viewedPortfolio) TransactionsView(ctx context.Context) []Transaction {
	return vp.stats.TransactionsView(ctx)
}

func (vp *viewedPortfolio) TradeDetailsView(ctx context.Context) []TradeDetail {
	return vp.stats.TradeDetailsView(ctx)
}

func (vp *viewedPortfolio) PricesView(ctx context.Context) *data.DataFrame {
	return vp.stats.PricesView(ctx)
}

func (vp *viewedPortfolio) TaxLotsView(ctx context.Context) map[asset.Asset][]TaxLot {
	return vp.stats.TaxLotsView(ctx)
}

func (vp *viewedPortfolio) ShortLotsView(ctx context.Context, fn func(asset.Asset, []TaxLot)) {
	vp.stats.ShortLotsView(ctx, fn)
}

func (vp *viewedPortfolio) PerfDataView(ctx context.Context) *data.DataFrame {
	return vp.stats.PerfDataView(ctx)
}

func (vp *viewedPortfolio) AnnualReturns(metric data.Metric) ([]int, []float64, error) {
	return vp.stats.AnnualReturns(metric)
}

func (vp *viewedPortfolio) DrawdownDetails(topN int) ([]DrawdownDetail, error) {
	return vp.stats.DrawdownDetails(topN)
}

func (vp *viewedPortfolio) MonthlyReturns(metric data.Metric) ([]int, [][]float64, error) {
	return vp.stats.MonthlyReturns(metric)
}
```

- [ ] **Step 6: Add `View` method to `Account`**

In `portfolio/account.go`, add after the `PerformanceMetric` method (after line 453):

```go
// View returns a read-only Portfolio restricted to the date range
// [start, end]. Metrics computed on the view use only data within
// this range.
func (a *Account) View(start, end time.Time) Portfolio {
	return &viewedPortfolio{
		acct:  a,
		stats: newWindowedStats(a, start, end),
		start: start,
		end:   end,
	}
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./portfolio/ --focus "View"`
Expected: PASS

- [ ] **Step 8: Run full portfolio suite to check for regressions**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./portfolio/`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add portfolio/portfolio.go portfolio/account.go portfolio/viewed_portfolio.go portfolio/viewed_portfolio_test.go
git commit -m "feat: add View method to Portfolio for date-range restricted views"
```

---

### Task 2: Add `subtractRanges` to Study Package

**Files:**
- Modify: `study/split.go` (add `subtractRanges` function)
- Modify: `study/split_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Add to `study/split_test.go`, inside the outer `Describe("DateRange and Split", ...)` block, after the `ScenarioLeaveNOut` Describe:

```go
	Describe("subtractRanges", func() {
		var (
			jan = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			mar = time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC)
			may = time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)
			jul = time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC)
			sep = time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC)
			dec = time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)
		)

		It("returns the full window when exclude is empty", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				nil,
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Start).To(Equal(jan))
			Expect(result[0].End).To(Equal(dec))
		})

		It("removes an exclusion from the middle", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				[]study.DateRange{{Start: may, End: jul}},
			)
			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(Equal(study.DateRange{Start: jan, End: may}))
			Expect(result[1]).To(Equal(study.DateRange{Start: jul, End: dec}))
		})

		It("removes an exclusion at the start", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				[]study.DateRange{{Start: jan, End: may}},
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(Equal(study.DateRange{Start: may, End: dec}))
		})

		It("removes an exclusion at the end", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				[]study.DateRange{{Start: sep, End: dec}},
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(Equal(study.DateRange{Start: jan, End: sep}))
		})

		It("handles multiple exclusions", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				[]study.DateRange{
					{Start: mar, End: may},
					{Start: jul, End: sep},
				},
			)
			Expect(result).To(HaveLen(3))
			Expect(result[0]).To(Equal(study.DateRange{Start: jan, End: mar}))
			Expect(result[1]).To(Equal(study.DateRange{Start: may, End: jul}))
			Expect(result[2]).To(Equal(study.DateRange{Start: sep, End: dec}))
		})

		It("returns empty when exclusion covers the full window", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				[]study.DateRange{{Start: jan, End: dec}},
			)
			Expect(result).To(BeEmpty())
		})
	})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/ --focus "subtractRanges"`
Expected: FAIL -- `study.SubtractRanges` is undefined.

- [ ] **Step 3: Implement `SubtractRanges`**

Add to `study/split.go`, after the `overlaps` function (after line 43):

```go
// SubtractRanges returns the portions of window not covered by any range in
// exclude. Exclude ranges are assumed non-overlapping. The returned slices
// share boundary timestamps with the exclusion ranges; this is acceptable
// because metric computations are insensitive to a single shared data point.
func SubtractRanges(window DateRange, exclude []DateRange) []DateRange {
	if len(exclude) == 0 {
		return []DateRange{window}
	}

	// Sort exclusions by start time.
	sorted := make([]DateRange, len(exclude))
	copy(sorted, exclude)
	sort.Slice(sorted, func(ii, jj int) bool {
		return sorted[ii].Start.Before(sorted[jj].Start)
	})

	var result []DateRange
	cursor := window.Start

	for _, ex := range sorted {
		// Clamp exclusion to the window.
		exStart := ex.Start
		if exStart.Before(window.Start) {
			exStart = window.Start
		}

		exEnd := ex.End
		if exEnd.After(window.End) {
			exEnd = window.End
		}

		// Emit the segment before this exclusion.
		if cursor.Before(exStart) {
			result = append(result, DateRange{Start: cursor, End: exStart})
		}

		// Advance cursor past the exclusion.
		if exEnd.After(cursor) {
			cursor = exEnd
		}
	}

	// Emit the segment after the last exclusion.
	if cursor.Before(window.End) {
		result = append(result, DateRange{Start: cursor, End: window.End})
	}

	return result
}
```

Add `"sort"` to the import block in `study/split.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/ --focus "subtractRanges"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add study/split.go study/split_test.go
git commit -m "feat: add SubtractRanges for computing in-sample date segments"
```

---

### Task 3: Fix `WindowedScore` and `WindowedScoreExcluding`

**Files:**
- Modify: `study/metric.go` (rewrite both functions)
- Modify: `study/metric_test.go` (update tests to verify exclusion works)

- [ ] **Step 1: Update the WindowedScoreExcluding test to assert exclusion changes the score**

In `study/metric_test.go`, replace the `WindowedScoreExcluding` Describe block (lines 137-159) with:

```go
	Describe("WindowedScoreExcluding", func() {
		It("delegates to WindowedScore when exclude is nil", func() {
			direct := study.WindowedScore(fakePF, fullWindow, study.MetricCAGR)
			excluding := study.WindowedScoreExcluding(fakePF, fullWindow, nil, study.MetricCAGR)
			Expect(excluding).To(Equal(direct))
		})

		It("delegates to WindowedScore when exclude is empty", func() {
			direct := study.WindowedScore(fakePF, fullWindow, study.MetricCAGR)
			excluding := study.WindowedScoreExcluding(fakePF, fullWindow, []study.DateRange{}, study.MetricCAGR)
			Expect(excluding).To(Equal(direct))
		})

		It("produces a different score when a middle segment is excluded", func() {
			fullScore := study.WindowedScore(fakePF, fullWindow, study.MetricCAGR)

			excludeRange := study.DateRange{
				Start: time.Date(2020, 4, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2020, 8, 1, 0, 0, 0, 0, time.UTC),
			}
			excludedScore := study.WindowedScoreExcluding(
				fakePF, fullWindow, []study.DateRange{excludeRange}, study.MetricCAGR,
			)

			Expect(math.IsNaN(excludedScore)).To(BeFalse(), "excluded score should not be NaN")
			Expect(excludedScore).NotTo(Equal(fullScore),
				"excluding a middle segment should change the score")
		})

		It("returns NaN when the exclusion covers the entire window", func() {
			score := study.WindowedScoreExcluding(
				fakePF, fullWindow, []study.DateRange{fullWindow}, study.MetricCAGR,
			)
			Expect(math.IsNaN(score)).To(BeTrue(),
				"full exclusion should return NaN")
		})
	})
```

- [ ] **Step 2: Run tests to verify the new assertions fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/ --focus "WindowedScoreExcluding"`
Expected: FAIL -- "produces a different score when a middle segment is excluded" fails because the current implementation ignores exclusions.

- [ ] **Step 3: Rewrite `study/metric.go`**

Replace the contents of `study/metric.go` (lines 63-89, the `WindowedScore` and `WindowedScoreExcluding` functions) with:

```go
// WindowedScore computes the given metric for rp restricted to the
// closed date interval [window.Start, window.End]. It returns NaN if
// the metric cannot be computed (e.g. the window contains no data).
func WindowedScore(rp report.ReportablePortfolio, window DateRange, metric Metric) float64 {
	val, err := rp.View(window.Start, window.End).PerformanceMetric(metric.performanceMetric()).Value()
	if err != nil {
		return math.NaN()
	}

	return val
}

// WindowedScoreExcluding computes the given metric for rp over window,
// ignoring sub-ranges listed in exclude. It computes the metric on each
// non-excluded segment and returns the duration-weighted average. When
// exclude is empty it delegates directly to WindowedScore.
func WindowedScoreExcluding(rp report.ReportablePortfolio, window DateRange, exclude []DateRange, metric Metric) float64 {
	if len(exclude) == 0 {
		return WindowedScore(rp, window, metric)
	}

	segments := SubtractRanges(window, exclude)
	if len(segments) == 0 {
		return math.NaN()
	}

	if len(segments) == 1 {
		return WindowedScore(rp, segments[0], metric)
	}

	var totalWeight float64
	var weightedSum float64

	for _, seg := range segments {
		score := WindowedScore(rp, seg, metric)
		if math.IsNaN(score) {
			continue
		}

		weight := seg.End.Sub(seg.Start).Seconds()
		weightedSum += score * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return math.NaN()
	}

	return weightedSum / totalWeight
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/ --focus "Metric"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add study/metric.go study/metric_test.go
git commit -m "fix: implement WindowedScoreExcluding to exclude date ranges from IS scores (#102)"
```

---

### Task 4: Run Full Suite and Lint

**Files:** None (validation only)

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./...`
Expected: PASS

- [ ] **Step 2: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run`
Expected: PASS

- [ ] **Step 3: Fix any lint issues found**

Common issues to watch for:
- Unused imports from changes
- Variable name length violations (varnamelen) -- ensure all variable names are >= 2 chars
- Any pre-existing lint issues -- fix them all

- [ ] **Step 4: Run tests again after lint fixes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./...`
Expected: PASS

- [ ] **Step 5: Commit any lint fixes**

```bash
git add -A
git commit -m "fix: resolve lint issues"
```
