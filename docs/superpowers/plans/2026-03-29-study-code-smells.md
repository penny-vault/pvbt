# Study Package Code Smells Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix five code smells in the study and optimize packages: floating-point accumulation, channel buffer sizing, placeholder equity curves, and a type-system redesign replacing `study.Metric` with `portfolio.Rankable`.

**Architecture:** Delete `study.Metric` enum entirely; add a `Rankable` interface to the portfolio package embedding `PerformanceMetric` + `HigherIsBetter()`. Fix `SweepRange` float precision with index-based computation. Normalize progress channel buffers. Wire real equity curves through the optimizer analysis pipeline.

**Tech Stack:** Go, Ginkgo/Gomega, portfolio/study packages

---

### Task 1: Add `Rankable` Interface to Portfolio Package

**Files:**
- Modify: `portfolio/metric_query.go:67-70` (add interface after `BenchmarkTargetable`)
- Modify: `portfolio/sharpe.go` (add `HigherIsBetter` method)
- Modify: `portfolio/cagr_metric.go` (add `HigherIsBetter` method)
- Modify: `portfolio/max_drawdown.go` (add `HigherIsBetter` method)
- Modify: `portfolio/sortino.go` (add `HigherIsBetter` method)
- Modify: `portfolio/calmar.go` (add `HigherIsBetter` method)

- [ ] **Step 1: Write the failing test**

Create `portfolio/rankable_test.go`:

```go
package portfolio_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Rankable", func() {
	It("Sharpe implements Rankable with HigherIsBetter true", func() {
		rankable, ok := portfolio.Sharpe.(portfolio.Rankable)
		Expect(ok).To(BeTrue(), "Sharpe should implement Rankable")
		Expect(rankable.HigherIsBetter()).To(BeTrue())
	})

	It("CAGR implements Rankable with HigherIsBetter true", func() {
		rankable, ok := portfolio.CAGR.(portfolio.Rankable)
		Expect(ok).To(BeTrue(), "CAGR should implement Rankable")
		Expect(rankable.HigherIsBetter()).To(BeTrue())
	})

	It("MaxDrawdown implements Rankable with HigherIsBetter true", func() {
		rankable, ok := portfolio.MaxDrawdown.(portfolio.Rankable)
		Expect(ok).To(BeTrue(), "MaxDrawdown should implement Rankable")
		Expect(rankable.HigherIsBetter()).To(BeTrue())
	})

	It("Sortino implements Rankable with HigherIsBetter true", func() {
		rankable, ok := portfolio.Sortino.(portfolio.Rankable)
		Expect(ok).To(BeTrue(), "Sortino should implement Rankable")
		Expect(rankable.HigherIsBetter()).To(BeTrue())
	})

	It("Calmar implements Rankable with HigherIsBetter true", func() {
		rankable, ok := portfolio.Calmar.(portfolio.Rankable)
		Expect(ok).To(BeTrue(), "Calmar should implement Rankable")
		Expect(rankable.HigherIsBetter()).To(BeTrue())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./portfolio/ --focus "Rankable"`
Expected: FAIL -- `portfolio.Rankable` is undefined

- [ ] **Step 3: Add `Rankable` interface to `metric_query.go`**

Add after the `BenchmarkTargetable` interface (after line 70):

```go
// Rankable extends PerformanceMetric with sort-direction metadata for
// optimization ranking. Only metrics that implement Rankable can be used
// as optimization objectives. This forces any new rankable metric to
// explicitly declare its sort direction, preventing silent sorting bugs.
type Rankable interface {
	PerformanceMetric
	HigherIsBetter() bool
}
```

- [ ] **Step 4: Add `HigherIsBetter` to the five metric structs**

In `portfolio/sharpe.go`, add after `BenchmarkTargetable()`:
```go
func (sharpe) HigherIsBetter() bool { return true }
```

In `portfolio/cagr_metric.go`, add after `BenchmarkTargetable()`:
```go
func (cagrMetric) HigherIsBetter() bool { return true }
```

In `portfolio/max_drawdown.go`, add after `BenchmarkTargetable()`:
```go
func (maxDrawdown) HigherIsBetter() bool { return true }
```

In `portfolio/sortino.go`, add after `BenchmarkTargetable()`:
```go
func (sortino) HigherIsBetter() bool { return true }
```

In `portfolio/calmar.go`, add after `BenchmarkTargetable()`:
```go
func (calmar) HigherIsBetter() bool { return true }
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./portfolio/ --focus "Rankable"`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add portfolio/rankable_test.go portfolio/metric_query.go portfolio/sharpe.go portfolio/cagr_metric.go portfolio/max_drawdown.go portfolio/sortino.go portfolio/calmar.go
git commit -m "feat: add Rankable interface to portfolio package"
```

---

### Task 2: Replace `study.Metric` with `portfolio.Rankable` in Study Package

**Files:**
- Modify: `study/metric.go` (delete type/const/performanceMetric, change function signatures)
- Modify: `study/metric_test.go` (update to use `portfolio.Rankable`)
- Modify: `study/runner.go:38` (change `Objective` field type)

- [ ] **Step 1: Update `study/metric.go`**

Replace the entire file contents with:

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

package study

import (
	"math"

	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study/report"
)

// WindowedScore computes the given metric for rp restricted to the
// closed date interval [window.Start, window.End]. It returns NaN if
// the metric cannot be computed (e.g. the window contains no data).
func WindowedScore(rp report.ReportablePortfolio, window DateRange, metric portfolio.PerformanceMetric) float64 {
	val, err := rp.PerformanceMetric(metric).AbsoluteWindow(window.Start, window.End).Value()
	if err != nil {
		return math.NaN()
	}

	return val
}

// WindowedScoreExcluding computes the given metric for rp over window,
// ignoring sub-ranges listed in exclude. When exclude is empty it
// delegates directly to WindowedScore.
//
// Full exclusion filtering (splicing out date ranges from the equity curve
// before computing the metric) will be implemented when KFold end-to-end
// testing is in place. For now, only the delegation path is active.
func WindowedScoreExcluding(rp report.ReportablePortfolio, window DateRange, exclude []DateRange, metric portfolio.PerformanceMetric) float64 {
	if len(exclude) == 0 {
		return WindowedScore(rp, window, metric)
	}

	// TODO: implement actual exclusion filtering by splicing the equity curve.
	return WindowedScore(rp, window, metric)
}
```

- [ ] **Step 2: Update `study/runner.go` -- change `Objective` field type**

Change line 38 from:
```go
Objective      Metric
```
to:
```go
Objective      portfolio.Rankable
```

Add `"github.com/penny-vault/pvbt/portfolio"` to the import block. Remove the `"strconv"` import only if it becomes unused (it's still used on line 214, so keep it).

- [ ] **Step 3: Update `study/metric_test.go`**

Replace the file contents with:

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

package study_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/report"
)

// portfolioEquityAsset mirrors the sentinel used by portfolio/account.go to
// write equity values into the performance DataFrame.
var portfolioEquityAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}

// metricFakePortfolio is a minimal implementation of report.ReportablePortfolio
// that returns a fixed performance DataFrame from PerfData. The embedded
// portfolio.Account satisfies the remaining interface methods.
type metricFakePortfolio struct {
	*portfolio.Account
	perfDF *data.DataFrame
}

func (fp *metricFakePortfolio) PerfData() *data.DataFrame {
	return fp.perfDF
}

var _ report.ReportablePortfolio = (*metricFakePortfolio)(nil)

// buildMetricPerfDF constructs a minimal performance DataFrame with a
// PortfolioEquity column for the given dates and equity values.
func buildMetricPerfDF(dates []time.Time, equityValues []float64) *data.DataFrame {
	df, err := data.NewDataFrame(
		dates,
		[]asset.Asset{portfolioEquityAsset},
		[]data.Metric{data.PortfolioEquity},
		data.Daily,
		[][]float64{equityValues},
	)
	Expect(err).NotTo(HaveOccurred())

	return df
}

// buildMetricFakePortfolio creates a metricFakePortfolio backed by the given equity curve.
func buildMetricFakePortfolio(dates []time.Time, equityValues []float64) *metricFakePortfolio {
	acct := portfolio.New(portfolio.WithCash(equityValues[0], dates[0]))

	return &metricFakePortfolio{
		Account: acct,
		perfDF:  buildMetricPerfDF(dates, equityValues),
	}
}

var _ = Describe("Metric", func() {
	var (
		dates        []time.Time
		equityValues []float64
		fakePF       *metricFakePortfolio
		fullWindow   study.DateRange
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
			time.Date(2020, 8, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 11, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2020, 12, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2021, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		// A simple growing equity curve.
		equityValues = []float64{
			10_000, 10_200, 10_400, 10_600, 10_800,
			11_000, 11_200, 11_400, 11_600, 11_800,
			12_000, 12_200, 12_400,
		}
		fakePF = buildMetricFakePortfolio(dates, equityValues)
		fullWindow = study.DateRange{Start: dates[0], End: dates[len(dates)-1]}
	})

	Describe("WindowedScore", func() {
		It("returns a finite value for a known portfolio and window", func() {
			score := study.WindowedScore(fakePF, fullWindow, portfolio.CAGR)
			Expect(math.IsNaN(score)).To(BeFalse(), "expected finite CAGR score, got NaN")
		})

		It("returns a finite Sharpe value for sufficient data", func() {
			score := study.WindowedScore(fakePF, fullWindow, portfolio.Sharpe)
			Expect(math.IsNaN(score)).To(BeFalse(), "expected finite Sharpe score, got NaN")
		})

		It("returns a zero value (not NaN) for a window that contains no data", func() {
			emptyWindow := study.DateRange{
				Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			}
			score := study.WindowedScore(fakePF, emptyWindow, portfolio.CAGR)
			Expect(math.IsNaN(score)).To(BeFalse(), "expected non-NaN for empty window")
			Expect(score).To(Equal(0.0))
		})
	})

	Describe("WindowedScoreExcluding", func() {
		It("delegates to WindowedScore when exclude is nil", func() {
			direct := study.WindowedScore(fakePF, fullWindow, portfolio.CAGR)
			excluding := study.WindowedScoreExcluding(fakePF, fullWindow, nil, portfolio.CAGR)
			Expect(excluding).To(Equal(direct))
		})

		It("delegates to WindowedScore when exclude is empty", func() {
			direct := study.WindowedScore(fakePF, fullWindow, portfolio.CAGR)
			excluding := study.WindowedScoreExcluding(fakePF, fullWindow, []study.DateRange{}, portfolio.CAGR)
			Expect(excluding).To(Equal(direct))
		})

		It("returns a value when exclude ranges are provided", func() {
			excludeRange := study.DateRange{
				Start: time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC),
			}
			score := study.WindowedScoreExcluding(fakePF, fullWindow, []study.DateRange{excludeRange}, portfolio.CAGR)
			Expect(math.IsNaN(score)).To(BeFalse(), "expected a value even with exclusions, got NaN")
		})
	})
})
```

Note: the unknown metric panic test is removed because `study.Metric` no longer exists -- the type system now prevents passing an invalid metric at compile time.

- [ ] **Step 4: Verify the study package compiles (will fail until optimize is updated)**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./study/...`
Expected: compilation errors in `study/optimize` and `study/runner.go` referencing old `Metric` type. This is expected; we fix those in the next tasks.

- [ ] **Step 5: Commit**

```bash
git add study/metric.go study/metric_test.go study/runner.go
git commit -m "refactor: replace study.Metric with portfolio.PerformanceMetric in study package"
```

---

### Task 3: Update Optimize Package to Use `portfolio.Rankable`

**Files:**
- Modify: `study/optimize/optimize.go` (change field and option types)
- Modify: `study/optimize/analyze.go` (delete `higherIsBetter`, `metricName`, update signatures)
- Modify: `study/optimize/optimize_test.go` (use `portfolio.Sharpe`, `portfolio.CAGR`)
- Modify: `study/optimize/analyze_test.go` (use `portfolio.CAGR`, `portfolio.MaxDrawdown`)
- Modify: `study/optimize/integration_test.go` (use `portfolio.CAGR`)

- [ ] **Step 1: Update `study/optimize/optimize.go`**

Change the import from `"github.com/penny-vault/pvbt/study"` to also include `"github.com/penny-vault/pvbt/portfolio"`. The `study` import is still needed for `study.Study`, `study.Split`, `study.RunResult`.

Change the `objective` field (line 32):
```go
objective portfolio.Rankable
```

Change `WithObjective` (lines 40-44):
```go
func WithObjective(metric portfolio.Rankable) Option {
	return func(opt *Optimizer) {
		opt.objective = metric
	}
}
```

Change the default in `New` (line 59):
```go
objective: portfolio.Sharpe.(portfolio.Rankable),
```

- [ ] **Step 2: Update `study/optimize/analyze.go`**

Replace the import of `"github.com/penny-vault/pvbt/study"` -- keep it (still needed for `study.WindowedScore`, `study.RunResult`, `study.Split`, `study.DateRange`). Add `"github.com/penny-vault/pvbt/portfolio"`.

Delete `higherIsBetter` function (lines 40-47).

Delete `metricName` function (lines 330-345).

Update `analyzeResults` signature (lines 50-55):
```go
func analyzeResults(
	splits []study.Split,
	objective portfolio.Rankable,
	topN int,
	results []study.RunResult,
) (report.Report, error) {
```

Update `ObjectiveName` in `analyzeResults` (line 60):
```go
ObjectiveName: objective.Name(),
```

Update `groupByCombination` signature (lines 75-79):
```go
func groupByCombination(
	splits []study.Split,
	objective portfolio.PerformanceMetric,
	results []study.RunResult,
) []*comboResult {
```

Update `rankCombos` signature (line 183):
```go
func rankCombos(combos []*comboResult, objective portfolio.Rankable) {
```

Update line 184:
```go
ascending := !objective.HigherIsBetter()
```

- [ ] **Step 3: Update `study/optimize/optimize_test.go`**

Add `"github.com/penny-vault/pvbt/portfolio"` to imports.

Line 112: change `optimize.WithObjective(study.MetricCAGR)` to `optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable))`.

- [ ] **Step 4: Update `study/optimize/analyze_test.go`**

Add `"github.com/penny-vault/pvbt/portfolio"` to imports. Remove `"github.com/penny-vault/pvbt/study"` from imports only if no other `study.` references remain (there are -- `study.Split`, `study.DateRange`, `study.RunResult`, `study.TrainTest` -- so keep it).

Replace all occurrences:
- `optimize.WithObjective(study.MetricCAGR)` -> `optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable))`
- `optimize.WithObjective(study.MetricMaxDrawdown)` -> `optimize.WithObjective(portfolio.MaxDrawdown.(portfolio.Rankable))`

These appear on lines 217, 257, 266, 283, 293, 313, 356.

- [ ] **Step 5: Update `study/optimize/integration_test.go`**

Add `"github.com/penny-vault/pvbt/portfolio"` to imports.

Replace all occurrences:
- `optimize.WithObjective(study.MetricCAGR)` -> `optimize.WithObjective(portfolio.CAGR.(portfolio.Rankable))`

These appear on lines 65, 132.

- [ ] **Step 6: Run all optimize tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/optimize/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add study/optimize/optimize.go study/optimize/analyze.go study/optimize/optimize_test.go study/optimize/analyze_test.go study/optimize/integration_test.go
git commit -m "refactor: use portfolio.Rankable in optimize package, delete higherIsBetter and metricName"
```

---

### Task 4: Update CLI and Runner References

**Files:**
- Modify: `cli/study.go:213-229` (change `parseMetric` return type)
- Modify: `study/runner.go` (update `scoreBatch` calls to `WindowedScore`)

- [ ] **Step 1: Update `cli/study.go` `parseMetric`**

Change the function (lines 213-229) to:

```go
// parseMetric maps a flag string to the corresponding portfolio.Rankable metric.
func parseMetric(metricStr string) (portfolio.Rankable, error) {
	switch strings.ToLower(strings.TrimSpace(metricStr)) {
	case "sharpe":
		return portfolio.Sharpe.(portfolio.Rankable), nil
	case "cagr":
		return portfolio.CAGR.(portfolio.Rankable), nil
	case "max-drawdown", "maxdrawdown":
		return portfolio.MaxDrawdown.(portfolio.Rankable), nil
	case "sortino":
		return portfolio.Sortino.(portfolio.Rankable), nil
	case "calmar":
		return portfolio.Calmar.(portfolio.Rankable), nil
	default:
		return nil, fmt.Errorf("unknown metric %q: choose from sharpe, cagr, max-drawdown, sortino, calmar", metricStr)
	}
}
```

Add `"github.com/penny-vault/pvbt/portfolio"` to imports. Remove the `"github.com/penny-vault/pvbt/study"` import only if unused (it likely still has other references -- check and keep if needed).

- [ ] **Step 2: Verify the runner compiles**

The `Runner.Objective` field is now `portfolio.Rankable`. Check that `runner.go` lines 303 and 306 call `WindowedScore` with `runner.Objective` -- since `Rankable` embeds `PerformanceMetric`, and `WindowedScore` now accepts `portfolio.PerformanceMetric`, this should work without changes.

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./...`
Expected: PASS (full project compiles)

- [ ] **Step 3: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cli/study.go study/runner.go
git commit -m "refactor: update CLI parseMetric and runner to use portfolio.Rankable"
```

---

### Task 5: Fix SweepRange Floating-Point Accumulation

**Files:**
- Modify: `study/sweep.go:55-61`
- Modify: `study/sweep_test.go` (add precision test)

- [ ] **Step 1: Write a failing test for float precision**

Add to `study/sweep_test.go` inside the `SweepRange` Describe block:

```go
It("avoids floating-point accumulation error for small steps", func() {
	sweep := study.SweepRange("threshold", 0.0, 1.0, 0.1)
	values := sweep.Values()
	Expect(values).To(HaveLen(11)) // 0.0, 0.1, 0.2, ..., 1.0
	Expect(values[len(values)-1]).To(Equal("1"))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/ --focus "avoids floating-point"`
Expected: FAIL -- accumulated error causes the last value to be slightly less than 1.0, so `val <= max` fails one iteration early, producing 10 values instead of 11.

- [ ] **Step 3: Fix `SweepRange` to use index-based computation**

In `study/sweep.go`, replace lines 55-62:

```go
// SweepRange generates values from min to max (inclusive) with the given step.
func SweepRange[T Numeric](field string, min, max, step T) ParamSweep {
	var values []string
	for ii := 0; ; ii++ {
		val := min + T(ii)*step
		if val > max {
			break
		}
		values = append(values, fmt.Sprintf("%v", val))
	}

	return ParamSweep{field: field, values: values, min: fmt.Sprintf("%v", min), max: fmt.Sprintf("%v", max)}
}
```

- [ ] **Step 4: Run tests to verify all sweep tests pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/ --focus "SweepRange|ParamSweep"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add study/sweep.go study/sweep_test.go
git commit -m "fix: use index-based computation in SweepRange to avoid float accumulation error"
```

---

### Task 6: Normalize Progress Channel Buffer Sizing

**Files:**
- Modify: `study/runner.go:65`

- [ ] **Step 1: Change sweep-path buffer to fixed 64**

In `study/runner.go`, change line 65 from:
```go
progressCh := make(chan Progress, len(configs)*2)
```
to:
```go
progressCh := make(chan Progress, 64)
```

- [ ] **Step 2: Run runner tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/ --focus "Runner"`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add study/runner.go
git commit -m "fix: use consistent fixed-size progress channel buffer on both runner paths"
```

---

### Task 7: Wire Real Equity Curves Through Optimizer Analysis

**Files:**
- Modify: `study/optimize/analyze.go` (`comboResult` struct, `groupByCombination`, `computeEquityCurves`)
- Modify: `study/optimize/analyze_test.go` (update equity curve test assertions)

- [ ] **Step 1: Write a failing test for real equity curve data**

In `study/optimize/analyze_test.go`, update the existing equity curves test (around line 301) to assert that curves contain actual data:

```go
It("produces equity curves with real data for the top combos", func() {
	opt := optimize.New(splits, optimize.WithTopN(1))
	rpt, err := opt.Analyze(results)
	Expect(err).NotTo(HaveOccurred())

	rptData := decodeOptReport(rpt)
	Expect(rptData.EquityCurves).To(HaveLen(1))
	Expect(rptData.EquityCurves[0].Times).NotTo(BeEmpty(), "equity curve should have timestamps")
	Expect(rptData.EquityCurves[0].Values).NotTo(BeEmpty(), "equity curve should have values")
})
```

Update `optReportData.EquityCurves` struct (around line 161) to include the new fields:

```go
EquityCurves []struct {
	Name   string      `json:"name"`
	Times  []time.Time `json:"times"`
	Values []float64   `json:"values"`
} `json:"equityCurves"`
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/optimize/ --focus "produces equity curves with real data"`
Expected: FAIL -- Times and Values are empty

- [ ] **Step 3: Add equity curve storage to `comboResult`**

In `study/optimize/analyze.go`, add a field to `comboResult`:

```go
type comboResult struct {
	comboID      string
	preset       string
	params       map[string]string
	oosScores    []float64 // one per split
	isScores     []float64 // one per split
	equityTimes  []time.Time
	equityValues []float64
}
```

Add `"time"` to the imports if not already present.

- [ ] **Step 4: Extract equity curves during grouping**

In `groupByCombination`, after computing OOS/IS scores (after line 124), extract the equity curve from the portfolio's PerfData for the test window and store it on the comboResult. Add this code after line 124:

```go
// Extract equity curve for this split's test window.
perfData := rr.Portfolio.PerfDataView(context.Background())
if perfData != nil {
	eqWindow := perfData.Between(sp.Test.Start, sp.Test.End)
	if eqWindow != nil {
		portfolioAsset := asset.Asset{
			CompositeFigi: "_PORTFOLIO_",
			Ticker:        "_PORTFOLIO_",
		}
		eqCol := eqWindow.Column(portfolioAsset, data.PortfolioEquity)
		eqTimes := eqWindow.Times()
		cr.equityTimes = append(cr.equityTimes, eqTimes...)
		cr.equityValues = append(cr.equityValues, eqCol...)
	}
}
```

Add `"context"`, `"github.com/penny-vault/pvbt/asset"`, and `"github.com/penny-vault/pvbt/data"` to the imports (note: `"context"` may already be present from other changes).

- [ ] **Step 5: Update `computeEquityCurves` to use real data**

Replace the `computeEquityCurves` function:

```go
// computeEquityCurves builds equity curve series for the top N
// combinations from their stored equity data.
func computeEquityCurves(combos []*comboResult, topN int) []equityCurveSeries {
	limit := topN
	if limit > len(combos) {
		limit = len(combos)
	}

	curves := make([]equityCurveSeries, limit)

	for idx := range limit {
		curves[idx] = equityCurveSeries{
			Name:   paramsLabel(combos[idx]),
			Times:  combos[idx].equityTimes,
			Values: combos[idx].equityValues,
		}
	}

	return curves
}
```

- [ ] **Step 6: Discard equity data for non-top-N combos after ranking**

In `analyzeResults`, after `rankCombos` and before building the report, clear equity data from combos beyond topN:

```go
rankCombos(combos, objective)

// Discard equity data for combos outside the top N to bound memory.
for idx := topN; idx < len(combos); idx++ {
	combos[idx].equityTimes = nil
	combos[idx].equityValues = nil
}
```

- [ ] **Step 7: Run all optimize tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/optimize/`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add study/optimize/analyze.go study/optimize/analyze_test.go
git commit -m "feat: populate real equity curve data in optimizer report for top N combos"
```

---

### Task 8: Run Full Suite and Lint

**Files:** None (validation only)

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./...`
Expected: PASS

- [ ] **Step 2: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run`
Expected: PASS (no new lint issues)

- [ ] **Step 3: Fix any lint issues found**

If golangci-lint reports issues, fix them. Common issues to watch for:
- Unused imports from the old `study.Metric` references
- Variable name length violations (varnamelen)
- Unused parameters

- [ ] **Step 4: Commit any lint fixes**

```bash
git add -A
git commit -m "fix: resolve lint issues from study.Metric removal"
```
