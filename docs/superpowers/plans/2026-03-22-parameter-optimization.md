# Parameter Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add parameter optimization with validation schemes (train/test, k-fold, walk-forward, scenario-based) and search strategies (grid, random, Bayesian) to the study framework.

**Architecture:** The Runner gains a SearchStrategy interface that produces parameter combinations in batches. Each combination is evaluated across validation splits (DateRange pairs). A new study type (`study/optimize/`) analyzes results and produces rankings. Windowed scoring extends the existing PerformanceMetricQuery with AbsoluteWindow support.

**Tech Stack:** Go, Ginkgo/Gomega testing, zerolog, lipgloss (report rendering)

**Spec:** `docs/superpowers/specs/2026-03-22-parameter-optimization-design.md`

---

### Task 1: Add AbsoluteWindow to PerformanceMetricQuery

**Files:**
- Modify: `portfolio/metric_query.go:101-155`
- Create: `portfolio/windowed_stats.go`
- Test: `portfolio/metric_query_test.go`

The existing `Window(period Period)` method uses a relative `data.Period` (N days/months back). We need absolute `[start, end]` windowing for validation scoring, plus an excluding variant for KFold/ScenarioLeaveNOut.

- [ ] **Step 1: Write the failing test for AbsoluteWindow**

In `portfolio/metric_query_test.go`, add a test that calls `AbsoluteWindow(start, end)` on a PerformanceMetricQuery and verifies the metric is computed only within that date range. Use a portfolio with known equity values so you can verify the CAGR or Sharpe for a sub-window.

Pattern from existing tests: the portfolio package uses Ginkgo. Check `portfolio/portfolio_suite_test.go` for suite setup and existing test helpers for constructing Account objects with known data.

```go
Describe("AbsoluteWindow", func() {
    It("restricts metric computation to the specified date range", func() {
        // Build an Account with known equity data spanning 2010-2020.
        // Compute CAGR over [2015-01-01, 2020-01-01] via AbsoluteWindow.
        // Verify it differs from full-range CAGR and matches expected value.
    })

    It("returns error when window has insufficient data", func() {
        // Build Account with data from 2010-2020.
        // Request AbsoluteWindow for a 1-day range.
        // Verify the metric computation handles this gracefully.
    })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./portfolio/... -v --focus "AbsoluteWindow"`
Expected: Compilation failure -- AbsoluteWindow method does not exist.

- [ ] **Step 3: Implement AbsoluteWindow**

In `portfolio/metric_query.go`, add two fields and a method:

```go
// Add to PerformanceMetricQuery struct:
type PerformanceMetricQuery struct {
    account       *Account
    metric        PerformanceMetric
    window        *Period
    absStart      *time.Time  // new
    absEnd        *time.Time  // new
    benchmark     bool
}

// New method:
func (query PerformanceMetricQuery) AbsoluteWindow(start, end time.Time) PerformanceMetricQuery {
    query.absStart = &start
    query.absEnd = &end
    return query
}
```

Then modify `Value()` (line 122) and `Series()` (line 132). The approach: when `absStart`/`absEnd` are set, create a `windowedStats` wrapper that intercepts all `PortfolioStats` methods and applies `DataFrame.Between(absStart, absEnd)` to the returned DataFrames. This avoids modifying any existing metric `Compute()` implementations -- they receive a `PortfolioStats` whose data is already sliced.

Create `portfolio/windowed_stats.go`:

```go
type windowedStats struct {
    inner PortfolioStats
    start time.Time
    end   time.Time
}
```

Implement all `PortfolioStats` methods by delegating to `inner` and then calling `.Between(start, end)` on the returned DataFrame. For non-DataFrame methods (`TransactionsView`, `TaxLotsView`, etc.), filter by date range.

Then in `Value()`, when `absStart` is set, wrap the resolved stats:

```go
func (query PerformanceMetricQuery) Value() (float64, error) {
    stats, err := query.resolveStats()
    if err != nil {
        return 0, err
    }
    if query.absStart != nil {
        stats = &windowedStats{inner: stats, start: *query.absStart, end: *query.absEnd}
    }
    return query.metric.Compute(context.Background(), stats, query.window)
}
```

The key data-slicing method is `DataFrame.Between(start, end)` at `data/data_frame.go:608`, which already does efficient binary-search-based absolute date range extraction. Each `PortfolioStats` method like `EquitySeries(ctx, window)` calls `a.perfData.Window(window)` -- the `windowedStats` wrapper calls `inner.EquitySeries(ctx, window).Between(ws.start, ws.end)` to apply both the relative and absolute windows.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./portfolio/... -v --focus "AbsoluteWindow"`
Expected: PASS

- [ ] **Step 5: Run full portfolio test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./portfolio/...`
Expected: All tests pass. No regressions.

- [ ] **Step 6: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && make lint`
Expected: No lint errors.

- [ ] **Step 7: Commit**

```bash
git add portfolio/metric_query.go portfolio/metric_query_test.go
git commit -m "feat: add AbsoluteWindow to PerformanceMetricQuery for date-range scoring"
```

---

### Task 2: Extract Scenarios from stress into study

**Files:**
- Create: `study/scenario.go`
- Create: `study/scenario_test.go`
- Modify: `study/stress/stress.go:28-43`
- Modify: `study/stress/scenarios.go` (delete this file after moving content)
- Modify: `study/stress/scenarios_test.go` (move to `study/scenario_test.go`)
- Modify: `study/stress/analyze.go` (update Scenario references)

Move `stress.Scenario` type and `DefaultScenarios()` to `study.Scenario` and `study.AllScenarios()`. Add `study.ScenariosByName()`.

- [ ] **Step 1: Write failing test for ScenariosByName**

Create `study/scenario_test.go`:

```go
var _ = Describe("Scenarios", func() {
    Describe("AllScenarios", func() {
        It("returns the default scenario list", func() {
            scenarios := study.AllScenarios()
            Expect(scenarios).ToNot(BeEmpty())
            Expect(scenarios[0].Name).ToNot(BeEmpty())
        })
    })

    Describe("ScenariosByName", func() {
        It("returns matching scenarios", func() {
            scenarios, err := study.ScenariosByName("COVID Crash", "2008 Financial Crisis")
            Expect(err).ToNot(HaveOccurred())
            Expect(scenarios).To(HaveLen(2))
        })

        It("returns error for unknown scenario name", func() {
            _, err := study.ScenariosByName("Nonexistent Scenario")
            Expect(err).To(HaveOccurred())
        })
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Scenarios"`
Expected: Compilation failure -- AllScenarios and ScenariosByName do not exist in study package.

- [ ] **Step 3: Create study/scenario.go**

Move the `Scenario` struct from `study/stress/scenarios.go` to `study/scenario.go`. Move all 17 default scenarios into `AllScenarios()`. Implement `ScenariosByName()` that looks up by name and returns an error for any unrecognized name.

```go
package study

import (
    "fmt"
    "time"
)

type Scenario struct {
    Name        string
    Description string
    Start       time.Time
    End         time.Time
}

func AllScenarios() []Scenario {
    return []Scenario{
        // ... all 17 scenarios from stress/scenarios.go
    }
}

func ScenariosByName(names ...string) ([]Scenario, error) {
    all := AllScenarios()
    byName := make(map[string]Scenario, len(all))
    for _, sc := range all {
        byName[sc.Name] = sc
    }

    result := make([]Scenario, 0, len(names))
    for _, name := range names {
        sc, ok := byName[name]
        if !ok {
            return nil, fmt.Errorf("unknown scenario: %q", name)
        }
        result = append(result, sc)
    }
    return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Scenarios"`
Expected: PASS

- [ ] **Step 5: Update stress test to use study.Scenario**

Modify `study/stress/stress.go`:
- Change `StressTest.scenarios` field type from `[]Scenario` to `[]study.Scenario`
- Change `New()` parameter type from `[]Scenario` to `[]study.Scenario`
- Change `New()` default from `DefaultScenarios()` to `study.AllScenarios()`
- Update `Configurations()` and `Analyze()` to use `study.Scenario`

Modify `study/stress/analyze.go`:
- Change all `Scenario` references to `study.Scenario`

Delete `study/stress/scenarios.go` (content moved to `study/scenario.go`).

Move relevant tests from `study/stress/scenarios_test.go` to `study/scenario_test.go`. Delete `study/stress/scenarios_test.go`.

- [ ] **Step 6: Update cli/study.go**

Change `resolveScenarios()` in `cli/study.go` to use `study.AllScenarios()` and return `[]study.Scenario` instead of `[]stress.Scenario`.

- [ ] **Step 7: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./...`
Expected: All tests pass. The stress test and CLI should work exactly as before.

- [ ] **Step 8: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && make lint`
Expected: No lint errors.

- [ ] **Step 9: Commit**

```bash
git add study/scenario.go study/scenario_test.go study/stress/ cli/study.go
git commit -m "refactor: extract Scenario and AllScenarios from stress into study package"
```

---

### Task 3: Add DateRange, Split, and Validation Schemes

**Files:**
- Create: `study/split.go`
- Create: `study/split_test.go`

Four validation scheme constructors: TrainTest, KFold, WalkForward, ScenarioLeaveNOut. All return `([]Split, error)`.

- [ ] **Step 1: Write failing tests for TrainTest**

Create `study/split_test.go`:

```go
var _ = Describe("Validation Schemes", func() {
    Describe("TrainTest", func() {
        It("returns a single split with train and test ranges", func() {
            start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
            cutoff := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
            end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

            splits, err := study.TrainTest(start, cutoff, end)
            Expect(err).ToNot(HaveOccurred())
            Expect(splits).To(HaveLen(1))
            Expect(splits[0].Train.Start).To(Equal(start))
            Expect(splits[0].Train.End).To(Equal(cutoff))
            Expect(splits[0].Test.Start).To(Equal(cutoff))
            Expect(splits[0].Test.End).To(Equal(end))
            Expect(splits[0].FullRange.Start).To(Equal(start))
            Expect(splits[0].FullRange.End).To(Equal(end))
            Expect(splits[0].Exclude).To(BeEmpty())
        })

        It("returns error when cutoff is before start", func() {
            start := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
            cutoff := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
            end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

            _, err := study.TrainTest(start, cutoff, end)
            Expect(err).To(HaveOccurred())
        })

        It("returns error when cutoff is after end", func() {
            start := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
            cutoff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
            end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

            _, err := study.TrainTest(start, cutoff, end)
            Expect(err).To(HaveOccurred())
        })

        It("returns error when start equals end", func() {
            ts := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)

            _, err := study.TrainTest(ts, ts, ts)
            Expect(err).To(HaveOccurred())
        })
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "TrainTest"`
Expected: Compilation failure.

- [ ] **Step 3: Implement DateRange, Split, and TrainTest**

Create `study/split.go`:

```go
package study

import (
    "fmt"
    "time"
)

type DateRange struct {
    Start time.Time
    End   time.Time
}

type Split struct {
    Name      string
    FullRange DateRange
    Train     DateRange
    Test      DateRange
    Exclude   []DateRange
}

func TrainTest(start, cutoff, end time.Time) ([]Split, error) {
    if cutoff.Before(start) || cutoff.After(end) {
        return nil, fmt.Errorf("cutoff %s must be between start %s and end %s", cutoff, start, end)
    }
    if !start.Before(end) {
        return nil, fmt.Errorf("start %s must be before end %s", start, end)
    }

    return []Split{{
        Name:      "Train/Test",
        FullRange: DateRange{Start: start, End: end},
        Train:     DateRange{Start: start, End: cutoff},
        Test:      DateRange{Start: cutoff, End: end},
    }}, nil
}
```

- [ ] **Step 4: Run TrainTest test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "TrainTest"`
Expected: PASS

- [ ] **Step 5: Write failing tests for KFold**

Add to `study/split_test.go`:

```go
Describe("KFold", func() {
    It("returns k splits with correct train/test/exclude ranges", func() {
        start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
        end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

        splits, err := study.KFold(start, end, 4)
        Expect(err).ToNot(HaveOccurred())
        Expect(splits).To(HaveLen(4))

        // Each split: FullRange=[2000,2020], Train=[2000,2020], Exclude=[test fold]
        for _, sp := range splits {
            Expect(sp.FullRange.Start).To(Equal(start))
            Expect(sp.FullRange.End).To(Equal(end))
            Expect(sp.Train.Start).To(Equal(start))
            Expect(sp.Train.End).To(Equal(end))
            Expect(sp.Exclude).To(HaveLen(1))
            Expect(sp.Exclude[0]).To(Equal(sp.Test))
        }
    })

    It("folds cover the full range without gaps or overlap", func() {
        start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
        end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

        splits, err := study.KFold(start, end, 4)
        Expect(err).ToNot(HaveOccurred())

        // First fold starts at start, last fold ends at end
        Expect(splits[0].Test.Start).To(Equal(start))
        Expect(splits[3].Test.End).To(Equal(end))

        // Each fold's test end == next fold's test start
        for ii := 0; ii < len(splits)-1; ii++ {
            Expect(splits[ii].Test.End).To(Equal(splits[ii+1].Test.Start))
        }
    })

    It("returns error when folds < 2", func() {
        start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
        end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

        _, err := study.KFold(start, end, 1)
        Expect(err).To(HaveOccurred())
    })
})
```

- [ ] **Step 6: Implement KFold and run tests**

Add `KFold` to `study/split.go`. Divide the duration `end.Sub(start)` by `folds` to get fold duration. Each fold's test window is `[start + i*foldDur, start + (i+1)*foldDur]`. FullRange and Train are both `[start, end]`. Exclude is `[]DateRange{{test fold}}`.

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "KFold"`
Expected: PASS

- [ ] **Step 7: Write failing tests for WalkForward**

Add to `study/split_test.go`:

```go
Describe("WalkForward", func() {
    It("produces splits with expanding training windows", func() {
        start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
        end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
        minTrain := 5 * 365 * 24 * time.Hour  // ~5 years
        testLen := 2 * 365 * 24 * time.Hour   // ~2 years
        step := 1 * 365 * 24 * time.Hour      // ~1 year

        splits, err := study.WalkForward(start, end, minTrain, testLen, step)
        Expect(err).ToNot(HaveOccurred())
        Expect(splits).ToNot(BeEmpty())

        // All splits: train starts at start, train end grows
        for _, sp := range splits {
            Expect(sp.Train.Start).To(Equal(start))
            Expect(sp.Train.End).To(Equal(sp.Test.Start))
        }

        // Training window expands
        if len(splits) > 1 {
            Expect(splits[1].Train.End.After(splits[0].Train.End)).To(BeTrue())
        }
    })

    It("returns error when minTrain + testLen exceeds date range", func() {
        start := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
        end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
        minTrain := 3 * 365 * 24 * time.Hour
        testLen := 2 * 365 * 24 * time.Hour
        step := 1 * 365 * 24 * time.Hour

        _, err := study.WalkForward(start, end, minTrain, testLen, step)
        Expect(err).To(HaveOccurred())
    })
})
```

- [ ] **Step 8: Implement WalkForward and run tests**

Add `WalkForward` to `study/split.go`. Loop: trainEnd starts at `start + minTrain`, testEnd at `trainEnd + testLen`. Each iteration advances trainEnd and testEnd by `step`. Stop when testEnd exceeds `end`. FullRange is `[start, testEnd]`, Train is `[start, trainEnd]`, Test is `[trainEnd, testEnd]`, Exclude is empty.

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "WalkForward"`
Expected: PASS

- [ ] **Step 9: Write failing tests for ScenarioLeaveNOut**

Add to `study/split_test.go`:

```go
Describe("ScenarioLeaveNOut", func() {
    var scenarios []study.Scenario

    BeforeEach(func() {
        scenarios = []study.Scenario{
            {Name: "A", Start: time.Date(2008, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2009, 3, 31, 0, 0, 0, 0, time.UTC)},
            {Name: "B", Start: time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2020, 3, 31, 0, 0, 0, 0, time.UTC)},
            {Name: "C", Start: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2022, 10, 31, 0, 0, 0, 0, time.UTC)},
        }
    })

    It("produces one split per scenario for leave-one-out", func() {
        boundStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
        boundEnd := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

        splits, err := study.ScenarioLeaveNOut(scenarios, boundStart, boundEnd, 1)
        Expect(err).ToNot(HaveOccurred())
        Expect(splits).To(HaveLen(3))

        // Each split's test matches one scenario
        Expect(splits[0].Test.Start).To(Equal(scenarios[0].Start))
        Expect(splits[0].Test.End).To(Equal(scenarios[0].End))
    })

    It("returns error when holdOut exceeds scenario count", func() {
        boundStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
        boundEnd := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

        _, err := study.ScenarioLeaveNOut(scenarios, boundStart, boundEnd, 4)
        Expect(err).To(HaveOccurred())
    })

    It("returns error when holdOut < 1", func() {
        boundStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
        boundEnd := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

        _, err := study.ScenarioLeaveNOut(scenarios, boundStart, boundEnd, 0)
        Expect(err).To(HaveOccurred())
    })

    It("produces correct splits for leave-two-out", func() {
        boundStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
        boundEnd := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

        splits, err := study.ScenarioLeaveNOut(scenarios, boundStart, boundEnd, 2)
        Expect(err).ToNot(HaveOccurred())
        // C(3, 2) = 3 combinations
        Expect(splits).To(HaveLen(3))
    })

    It("adds overlapping scenarios to Exclude", func() {
        overlapping := []study.Scenario{
            {Name: "Dot-com Bubble", Start: time.Date(1998, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2000, 3, 31, 0, 0, 0, 0, time.UTC)},
            {Name: "LTCM Crisis", Start: time.Date(1998, 8, 1, 0, 0, 0, 0, time.UTC), End: time.Date(1998, 10, 31, 0, 0, 0, 0, time.UTC)},
            {Name: "COVID Crash", Start: time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2020, 3, 31, 0, 0, 0, 0, time.UTC)},
        }
        boundStart := time.Date(1995, 1, 1, 0, 0, 0, 0, time.UTC)
        boundEnd := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

        splits, err := study.ScenarioLeaveNOut(overlapping, boundStart, boundEnd, 1)
        Expect(err).ToNot(HaveOccurred())

        // When LTCM is held out, Dot-com Bubble overlaps -> should be in Exclude
        ltcmSplit := splits[1] // LTCM Crisis
        Expect(ltcmSplit.Exclude).To(HaveLen(2)) // LTCM itself + overlapping Dot-com
    })
})
```

- [ ] **Step 10: Implement ScenarioLeaveNOut and run tests**

Add `ScenarioLeaveNOut` to `study/split.go`. For each combination of `holdOut` scenarios (use combinatorial selection for holdOut > 1): Test is the held-out scenario's DateRange. FullRange is `[boundStart, boundEnd]`. Train is `[boundStart, boundEnd]`. Exclude contains the held-out scenario period plus any other scenarios whose date range overlaps with the held-out one.

Two date ranges overlap when `a.Start.Before(b.End) && b.Start.Before(a.End)`.

For holdOut=1, iterate each scenario. For holdOut>1, generate all C(n, holdOut) combinations.

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "ScenarioLeaveNOut"`
Expected: PASS

- [ ] **Step 11: Run full test suite and lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./... && make lint`
Expected: All pass, no lint errors.

- [ ] **Step 12: Commit**

```bash
git add study/split.go study/split_test.go
git commit -m "feat: add DateRange, Split, and validation schemes (TrainTest, KFold, WalkForward, ScenarioLeaveNOut)"
```

---

### Task 4: Add Windowed Scoring (Metric enum, WindowedScore, WindowedScoreExcluding)

**Files:**
- Create: `study/metric.go`
- Create: `study/metric_test.go`

Bridge between the study package's `Metric` enum and the portfolio package's `PerformanceMetric` interface. `WindowedScore` and `WindowedScoreExcluding` use `AbsoluteWindow` from Task 1.

- [ ] **Step 1: Write failing tests**

Create `study/metric_test.go`:

```go
var _ = Describe("WindowedScore", func() {
    // Build a fakePortfolio with known equity data spanning 2010-2020.
    // Use the same fakePortfolio helper pattern from study/stress/analyze_test.go.

    It("computes metric over the specified date range", func() {
        // Create portfolio with known equity curve.
        // Call WindowedScore with MetricCAGR over [2015, 2020].
        // Verify the result matches expected CAGR for that sub-range.
    })

    It("returns NaN for insufficient data", func() {
        // Create portfolio with data from 2010-2020.
        // Call WindowedScore with a 0-day window.
        // Expect math.NaN().
    })
})

var _ = Describe("WindowedScoreExcluding", func() {
    It("excludes specified date ranges from computation", func() {
        // Create portfolio with known equity curve 2010-2020.
        // Call WindowedScoreExcluding over [2010, 2020] excluding [2014, 2016].
        // Verify result differs from non-excluded and matches expected value.
    })
})
```

Look at `study/stress/analyze_test.go` for the `fakePortfolio` and `buildPerfDataFrame` helpers. Reuse or factor out as needed.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "WindowedScore"`
Expected: Compilation failure.

- [ ] **Step 3: Implement Metric enum and WindowedScore**

Create `study/metric.go`:

```go
package study

import (
    "math"

    "github.com/penny-vault/pvbt/portfolio"
    "github.com/penny-vault/pvbt/report"
)

type Metric int

const (
    MetricSharpe Metric = iota
    MetricCAGR
    MetricMaxDrawdown
    MetricSortino
    MetricCalmar
)

// performanceMetric maps the study Metric enum to portfolio.PerformanceMetric values.
func (mt Metric) performanceMetric() portfolio.PerformanceMetric {
    switch mt {
    case MetricSharpe:
        return portfolio.Sharpe
    case MetricCAGR:
        return portfolio.CAGR
    case MetricMaxDrawdown:
        return portfolio.MaxDrawdown
    case MetricSortino:
        return portfolio.Sortino
    case MetricCalmar:
        return portfolio.Calmar
    default:
        panic(fmt.Sprintf("unknown metric: %d", mt))
    }
}

func WindowedScore(rp report.ReportablePortfolio, window DateRange, metric Metric) float64 {
    query := rp.PerformanceMetric(metric.performanceMetric()).AbsoluteWindow(window.Start, window.End)
    val, err := query.Value()
    if err != nil {
        return math.NaN()
    }
    return val
}

func WindowedScoreExcluding(rp report.ReportablePortfolio, window DateRange, exclude []DateRange, metric Metric) float64 {
    if len(exclude) == 0 {
        return WindowedScore(rp, window, metric)
    }

    // Use AbsoluteWindow to get a PerformanceMetricQuery scoped to the window,
    // then further filter via a excludingStats wrapper.
    //
    // The excludingStats wrapper delegates to windowedStats but additionally
    // filters returned DataFrames to remove rows whose timestamps fall within
    // any exclude range. The filter is: for each row, skip if
    // any(excl.Start <= row.Time && row.Time <= excl.End for excl in exclude).
    //
    // Add excludingStats to portfolio/windowed_stats.go alongside windowedStats.
    // It composes windowedStats and applies the additional row filter.
    //
    // Then add AbsoluteWindowExcluding(start, end, exclude) to
    // PerformanceMetricQuery that constructs excludingStats in Value().
    //
    // WindowedScoreExcluding calls:
    //   rp.PerformanceMetric(metric.performanceMetric()).
    //     AbsoluteWindowExcluding(window.Start, window.End, excludeAsPeriods).
    //     Value()
}
```

Note: the exact implementation of `WindowedScoreExcluding` depends on what `AbsoluteWindow` exposes. If the portfolio's `PerfData()` returns a DataFrame that can be sliced and filtered, use that. The stress test's `computeScenarioMetrics` in `study/stress/analyze.go:111-134` shows the pattern for manual metric computation from a sliced DataFrame.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "WindowedScore"`
Expected: PASS

- [ ] **Step 5: Run full suite and lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./... && make lint`
Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add study/metric.go study/metric_test.go
git commit -m "feat: add Metric enum, WindowedScore, and WindowedScoreExcluding for validation scoring"
```

---

### Task 5: Extend ParamSweep with Min/Max Methods

**Files:**
- Modify: `study/sweep.go:31-74`
- Modify: `study/sweep_test.go`

Add `min` and `max` string fields to `ParamSweep`. Store them in `SweepRange` and `SweepDuration`. Expose via `Min()` and `Max()` methods.

- [ ] **Step 1: Write failing tests**

Add to `study/sweep_test.go`:

```go
Describe("ParamSweep Min/Max", func() {
    It("returns min and max for SweepRange", func() {
        sweep := study.SweepRange("lookback", 3.0, 24.0, 1.0)
        Expect(sweep.Min()).To(Equal("3"))
        Expect(sweep.Max()).To(Equal("24"))
    })

    It("returns min and max for SweepDuration", func() {
        sweep := study.SweepDuration("hold", time.Hour, 24*time.Hour, time.Hour)
        Expect(sweep.Min()).To(Equal(time.Hour.String()))
        Expect(sweep.Max()).To(Equal((24 * time.Hour).String()))
    })

    It("returns empty strings for SweepValues", func() {
        sweep := study.SweepValues("universe", "SPY,TLT", "QQQ,SHY")
        Expect(sweep.Min()).To(BeEmpty())
        Expect(sweep.Max()).To(BeEmpty())
    })

    It("returns empty strings for SweepPresets", func() {
        sweep := study.SweepPresets("Classic", "Modern")
        Expect(sweep.Min()).To(BeEmpty())
        Expect(sweep.Max()).To(BeEmpty())
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Min/Max"`
Expected: Compilation failure -- Min() and Max() do not exist.

- [ ] **Step 3: Implement Min/Max on ParamSweep**

In `study/sweep.go`, add `min` and `max` fields to `ParamSweep`:

```go
type ParamSweep struct {
    field    string
    values   []string
    isPreset bool
    min      string  // new
    max      string  // new
}

func (ps ParamSweep) Min() string { return ps.min }
func (ps ParamSweep) Max() string { return ps.max }
```

Update `SweepRange` to store min/max:

```go
func SweepRange[T Numeric](field string, min, max, step T) ParamSweep {
    var values []string
    for val := min; val <= max; val += step {
        values = append(values, fmt.Sprintf("%v", val))
    }
    return ParamSweep{field: field, values: values, min: fmt.Sprintf("%v", min), max: fmt.Sprintf("%v", max)}
}
```

Update `SweepDuration` similarly.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Min/Max"`
Expected: PASS

- [ ] **Step 5: Run full suite and lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./... && make lint`
Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add study/sweep.go study/sweep_test.go
git commit -m "feat: add Min/Max methods to ParamSweep for continuous sampling"
```

---

### Task 6: Add SearchStrategy Interface, CombinationScore, Grid and Random

**Files:**
- Create: `study/search.go`
- Create: `study/search_test.go`

Define `CombinationScore`, `SearchStrategy` interface, and implement `NewGrid` and `NewRandom`.

- [ ] **Step 1: Write failing tests for Grid**

Create `study/search_test.go`:

```go
var _ = Describe("SearchStrategy", func() {
    Describe("Grid", func() {
        It("returns all combinations on first call and is immediately done", func() {
            grid := study.NewGrid(
                study.SweepRange("lookback", 1.0, 3.0, 1.0), // 3 values
                study.SweepValues("mode", "fast", "slow"),     // 2 values
            )

            configs, done := grid.Next(nil)
            Expect(configs).To(HaveLen(6)) // 3 x 2
            Expect(done).To(BeTrue())
        })

        It("returns configs with correct Params", func() {
            grid := study.NewGrid(
                study.SweepRange("lookback", 1.0, 2.0, 1.0),
            )

            configs, _ := grid.Next(nil)
            Expect(configs[0].Params["lookback"]).To(Equal("1"))
            Expect(configs[1].Params["lookback"]).To(Equal("2"))
        })

        It("handles preset sweeps by setting Preset field", func() {
            grid := study.NewGrid(
                study.SweepPresets("Classic", "Modern"),
            )

            configs, _ := grid.Next(nil)
            Expect(configs).To(HaveLen(2))
            Expect(configs[0].Preset).To(Equal("Classic"))
            Expect(configs[1].Preset).To(Equal("Modern"))
        })
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Grid"`
Expected: Compilation failure.

- [ ] **Step 3: Implement CombinationScore, SearchStrategy, and Grid**

Create `study/search.go`:

```go
package study

type CombinationScore struct {
    Params map[string]string
    Preset string
    Score  float64
    Runs   []RunResult
}

type SearchStrategy interface {
    Next(scores []CombinationScore) (configs []RunConfig, done bool)
}

type gridStrategy struct {
    sweeps []ParamSweep
    called bool
}

func NewGrid(sweeps ...ParamSweep) SearchStrategy {
    return &gridStrategy{sweeps: sweeps}
}

func (gs *gridStrategy) Next(_ []CombinationScore) ([]RunConfig, bool) {
    if gs.called {
        return nil, true
    }
    gs.called = true

    base := []RunConfig{{}} // single empty base config
    configs := CrossProduct(base, gs.sweeps)
    return configs, true
}
```

- [ ] **Step 4: Run Grid test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Grid"`
Expected: PASS

- [ ] **Step 5: Write failing tests for Random**

Add to `study/search_test.go`:

```go
Describe("Random", func() {
    It("returns the requested number of samples", func() {
        random := study.NewRandom(
            []study.ParamSweep{
                study.SweepRange("lookback", 1.0, 100.0, 1.0),
            },
            10, // 10 samples
            42, // seed
        )

        configs, done := random.Next(nil)
        Expect(configs).To(HaveLen(10))
        Expect(done).To(BeTrue())
    })

    It("samples within range bounds for continuous params", func() {
        random := study.NewRandom(
            []study.ParamSweep{
                study.SweepRange("lookback", 1.0, 10.0, 0.5),
            },
            50,
            42,
        )

        configs, _ := random.Next(nil)
        for _, cfg := range configs {
            val := cfg.Params["lookback"]
            Expect(val).ToNot(BeEmpty())
            // Values should be within [1.0, 10.0]
        }
    })

    It("is deterministic with the same seed", func() {
        mk := func() study.SearchStrategy {
            return study.NewRandom(
                []study.ParamSweep{study.SweepRange("x", 0.0, 100.0, 1.0)},
                5, 42,
            )
        }

        configs1, _ := mk().Next(nil)
        configs2, _ := mk().Next(nil)
        Expect(configs1).To(Equal(configs2))
    })

    It("samples from value lists for discrete params", func() {
        random := study.NewRandom(
            []study.ParamSweep{
                study.SweepValues("mode", "fast", "slow", "medium"),
            },
            10,
            42,
        )

        configs, _ := random.Next(nil)
        for _, cfg := range configs {
            Expect(cfg.Params["mode"]).To(BeElementOf("fast", "slow", "medium"))
        }
    })

    It("handles preset sweeps", func() {
        random := study.NewRandom(
            []study.ParamSweep{
                study.SweepPresets("Classic", "Modern", "Aggressive"),
            },
            5,
            42,
        )

        configs, _ := random.Next(nil)
        for _, cfg := range configs {
            Expect(cfg.Preset).To(BeElementOf("Classic", "Modern", "Aggressive"))
        }
    })
})
```

- [ ] **Step 6: Implement Random and run tests**

Add `randomStrategy` to `study/search.go`. For each sample:
- For sweeps with `Min()` and `Max()` (continuous): parse as float64, sample uniformly, format back to string.
- For sweeps without range bounds (discrete): pick a random index from `Values()`.
- For preset sweeps: pick a random preset name and set `RunConfig.Preset`.

Use `math/rand/v2` with the provided seed for reproducibility.

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Random"`
Expected: PASS

- [ ] **Step 7: Run full suite and lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./... && make lint`
Expected: All pass.

- [ ] **Step 8: Commit**

```bash
git add study/search.go study/search_test.go
git commit -m "feat: add SearchStrategy interface with Grid and Random implementations"
```

---

### Task 7: Update Runner for SearchStrategy, Splits, and Objective

**Files:**
- Modify: `study/runner.go:28-148`
- Modify: `study/study.go:53-59` (add BatchIndex/BatchSize to Progress)
- Create: `study/runner_search_test.go`

Add the iterative search loop to the Runner. When `SearchStrategy` is set, the Runner uses it instead of `Sweeps`.

- [ ] **Step 1: Add BatchIndex and BatchSize to Progress**

In `study/study.go`, add to the `Progress` struct:

```go
type Progress struct {
    RunName    string
    RunIndex   int
    TotalRuns  int
    Status     RunStatus
    Err        error
    BatchIndex int  // new
    BatchSize  int  // new
}
```

- [ ] **Step 2: Write failing tests for Runner with SearchStrategy**

Create `study/runner_search_test.go`. Build a minimal fake Study that returns a single base config and a simple Analyze that passes through results. Use Grid strategy with known sweeps. Verify the correct number of runs execute and Analyze receives all results.

```go
var _ = Describe("Runner with SearchStrategy", func() {
    It("executes search strategy configs and returns results", func() {
        // fakeStudy returns 1 base config, Analyze returns empty report
        // Grid strategy with 2-value sweep -> 2 total runs
        // Verify 2 RunResults passed to Analyze
    })

    It("rejects setting both Sweeps and SearchStrategy", func() {
        // Set both fields on Runner
        // Call Run()
        // Expect error
    })

    It("cross-products combinations with splits", func() {
        // Grid with 2 combos, 3 splits -> 6 total runs
        // Verify metadata has _combination_id and _split_index
    })

    It("computes combination scores from test-window objectives", func() {
        // Grid with 2 combos, 2 splits
        // Objective = MetricCAGR
        // Verify CombinationScore.Score is mean of per-split test-window scores
    })
})
```

This requires fake/mock portfolios. Use the `fakePortfolio` pattern from `study/stress/analyze_test.go`.

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Runner with SearchStrategy"`
Expected: Compilation failure -- Runner doesn't have SearchStrategy field yet.

- [ ] **Step 4: Add new fields to Runner struct**

In `study/runner.go`, add:

```go
type Runner struct {
    Study          Study
    NewStrategy    func() engine.Strategy
    Options        []engine.Option
    Workers        int
    Sweeps         []ParamSweep
    SearchStrategy SearchStrategy     // new
    Splits         []Split            // new
    Objective      Metric             // new
}
```

- [ ] **Step 5: Implement the iterative search loop in Run()**

Modify `Run()` in `study/runner.go`:

1. If both `Sweeps` and `SearchStrategy` are set, return error.
2. If `SearchStrategy` is nil, use existing `Sweeps` + `CrossProduct` path unchanged.
3. If `SearchStrategy` is set:
   a. Get base configs from `Study.Configurations(ctx)`.
   b. Call `SearchStrategy.Next(nil)` for first batch.
   c. For each combo in batch, cross-product with `Splits` (or use base configs if no splits). Tag metadata: `_combination_id`, `_split_index`, `_split_name`. Set `Start`/`End` from `split.FullRange`.
   d. Execute all runs for the batch (reuse existing `execute` logic or factor out a `runBatch` helper).
   e. Group results by `_combination_id`. Score each combo: `mean(WindowedScore(result.Portfolio, split.Test, runner.Objective))` across splits.
   f. Call `SearchStrategy.Next(combinationScores)`.
   g. Repeat until done.
   h. Collect all results, call `Study.Analyze(allResults)`.

The key refactor: extract `runBatch(ctx, configs, workers, progressCh) []RunResult` from the existing `execute()` method so both paths can reuse it.

- [ ] **Step 6: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Runner with SearchStrategy"`
Expected: PASS

- [ ] **Step 7: Run full suite and lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./... && make lint`
Expected: All pass. Existing Runner tests should still pass (no SearchStrategy = old path).

- [ ] **Step 8: Commit**

```bash
git add study/runner.go study/study.go study/runner_search_test.go
git commit -m "feat: add SearchStrategy, Splits, and Objective to Runner with iterative search loop"
```

---

### Task 8: Implement study/optimize (Optimizer study type)

**Files:**
- Create: `study/optimize/optimize.go`
- Create: `study/optimize/analyze.go`
- Create: `study/optimize/doc.go`
- Create: `study/optimize/optimize_suite_test.go`
- Create: `study/optimize/optimize_test.go`
- Create: `study/optimize/analyze_test.go`

The Optimizer is a Study implementation. `Configurations()` returns a single base config spanning all splits. `Analyze()` groups results by combination, scores train/test windows, ranks by OOS score, and composes the report.

- [ ] **Step 1: Create test suite boilerplate**

Create `study/optimize/doc.go`:

```go
// Package optimize implements the parameter optimization study type.
package optimize
```

Create `study/optimize/optimize_suite_test.go`:

```go
package optimize_test

import (
    "testing"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestOptimize(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Optimize Suite")
}
```

- [ ] **Step 2: Write failing tests for Optimizer.Configurations**

Create `study/optimize/optimize_test.go`:

```go
var _ = Describe("Optimizer", func() {
    Describe("Configurations", func() {
        It("returns a single config spanning all splits", func() {
            splits := []study.Split{
                {FullRange: study.DateRange{
                    Start: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
                    End:   time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
                }},
                {FullRange: study.DateRange{
                    Start: time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC),
                    End:   time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
                }},
            }
            opt := optimize.New(splits)
            configs, err := opt.Configurations(context.Background())
            Expect(err).ToNot(HaveOccurred())
            Expect(configs).To(HaveLen(1))
            Expect(configs[0].Start).To(Equal(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)))
            Expect(configs[0].End).To(Equal(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)))
        })
    })

    It("satisfies the study.Study interface", func() {
        var _ study.Study = (*optimize.Optimizer)(nil)
    })
})
```

- [ ] **Step 3: Implement Optimizer struct and Configurations**

Create `study/optimize/optimize.go`:

```go
package optimize

import (
    "context"
    "github.com/penny-vault/pvbt/report"
    "github.com/penny-vault/pvbt/study"
)

var _ study.Study = (*Optimizer)(nil)

type Optimizer struct {
    splits    []study.Split
    objective study.Metric
    topN      int
}

type Option func(*Optimizer)

func WithObjective(metric study.Metric) Option {
    return func(opt *Optimizer) { opt.objective = metric }
}

func WithTopN(topN int) Option {
    return func(opt *Optimizer) { opt.topN = topN }
}

func New(splits []study.Split, opts ...Option) *Optimizer {
    opt := &Optimizer{
        splits:    splits,
        objective: study.MetricSharpe,
        topN:      10,
    }
    for _, fn := range opts {
        fn(opt)
    }
    return opt
}

func (opt *Optimizer) Name() string { return "Parameter Optimization" }

func (opt *Optimizer) Description() string {
    return "Systematic parameter search with out-of-sample validation"
}

func (opt *Optimizer) Configurations(_ context.Context) ([]study.RunConfig, error) {
    earliest := opt.splits[0].FullRange.Start
    latest := opt.splits[0].FullRange.End

    for _, sp := range opt.splits[1:] {
        if sp.FullRange.Start.Before(earliest) {
            earliest = sp.FullRange.Start
        }
        if sp.FullRange.End.After(latest) {
            latest = sp.FullRange.End
        }
    }

    return []study.RunConfig{{
        Name:  "Optimization",
        Start: earliest,
        End:   latest,
        Metadata: map[string]string{
            "study": "parameter-optimization",
        },
    }}, nil
}

func (opt *Optimizer) Analyze(results []study.RunResult) (report.Report, error) {
    return analyzeResults(opt.splits, opt.objective, opt.topN, results)
}
```

- [ ] **Step 4: Run test to verify Configurations passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/optimize/... -v --focus "Configurations"`
Expected: PASS (Analyze will fail since analyzeResults doesn't exist yet -- that's OK, we're testing Configurations).

- [ ] **Step 5: Write failing tests for Analyze**

Create `study/optimize/analyze_test.go`. Build fake RunResults with known equity data, combination IDs, and split indices. Verify Analyze produces a report with the expected sections (rankings table, best combination detail, overfitting check).

```go
var _ = Describe("analyzeResults", func() {
    It("ranks combinations by mean out-of-sample score", func() {
        // Build results for 2 combinations, 2 splits each.
        // Combo 0: mediocre OOS performance
        // Combo 1: good OOS performance
        // Verify combo 1 ranks first.
    })

    It("produces a report with expected sections", func() {
        // Build results for 3 combinations, 1 split.
        // Verify report has: Rankings table, Best combination detail,
        // Parameter sensitivity, Overfitting check.
    })
})
```

- [ ] **Step 6: Implement analyzeResults**

Create `study/optimize/analyze.go`. This is the largest piece:

1. Group `[]RunResult` by `_combination_id` metadata.
2. For each combo, for each split: call `WindowedScore` on test window and train window.
3. Compute mean OOS score per combo, rank descending.
4. Build report sections:
   - Rankings table (`report.Table`)
   - Best combination detail (`report.Table` with per-fold breakdown)
   - Parameter sensitivity (`report.Table`)
   - Overfitting check (`report.Table` with IS vs OOS columns)
   - Equity curves (`report.TimeSeries`) for the top N combinations on their OOS folds -- extract the equity curve via `PerfData().Between(split.Test.Start, split.Test.End)` for each top combo's test windows

Reference `study/stress/analyze.go` for the report-building pattern. Use `report.Table`, `report.MetricPairs`, `report.Text` section types.

- [ ] **Step 7: Run Analyze tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/optimize/... -v`
Expected: PASS

- [ ] **Step 8: Run full suite and lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./... && make lint`
Expected: All pass.

- [ ] **Step 9: Commit**

```bash
git add study/optimize/
git commit -m "feat: add parameter optimization study type with rankings and overfitting analysis"
```

---

### Task 9: Add CLI optimize Subcommand

**Files:**
- Modify: `cli/study.go:34-43`
- Modify: `cli/flags.go` (add range syntax parsing)

Add `study optimize` subcommand following the pattern of `study stress-test`.

- [ ] **Step 1: Add range syntax parsing to flags**

In `cli/flags.go`, add a helper that detects `min:max:step` syntax in flag values and returns a `ParamSweep` instead of a single value. When a parameter flag value contains colons, parse it as `min:max:step` and return `SweepRange`. Otherwise, return the value as-is (existing behavior).

```go
// parseRangeOrValue checks if a flag value uses range syntax (min:max:step).
// Returns (sweep, true) if it's a range, or (nil, false) for a single value.
func parseRangeOrValue(field, value string) (study.ParamSweep, bool) {
    parts := strings.SplitN(value, ":", 3)
    if len(parts) != 3 {
        return study.ParamSweep{}, false
    }
    // Parse as float64 range
    min, err := strconv.ParseFloat(parts[0], 64)
    // ... parse max, step
    return study.SweepRange(field, min, max, step), true
}
```

- [ ] **Step 2: Write the optimize command**

Add `newOptimizeCmd()` to `cli/study.go` and register it in `newStudyCmd()`:

```go
func newStudyCmd(strategy engine.Strategy) *cobra.Command {
    cmd := &cobra.Command{Use: "study", Short: "Run analysis studies on the strategy"}
    cmd.AddCommand(newStressTestCmd(strategy))
    cmd.AddCommand(newOptimizeCmd(strategy))  // new
    return cmd
}
```

The optimize command:
1. Parses `--search`, `--metric`, `--validation` and their sub-flags.
2. Builds the validation splits using the appropriate scheme function. For `--validation=scenario`, add `--scenarios` flag (positional args or comma-separated) and `--holdout` flag (default 1). Reuse `resolveScenarios()` from the stress test CLI for scenario name lookup. Default to `study.AllScenarios()` when no scenarios specified.
3. Builds the search strategy (Grid, Random) from sweeps extracted from parameter flags.
4. Creates `optimize.New(splits, ...)`.
5. Creates Runner with Study, SearchStrategy, Splits, Objective.
6. Calls `runner.Run(ctx)`, drains progress, renders report.

Follow the `runStressTest()` pattern in `cli/study.go:61-117`.

- [ ] **Step 3: Test manually**

Verify the command registers and help text appears:

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build -o pvbt . && ./pvbt study optimize --help`
Expected: Shows optimize command with all flags listed.

- [ ] **Step 4: Run full suite and lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./... && make lint`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add cli/study.go cli/flags.go
git commit -m "feat: add 'study optimize' CLI subcommand with search, validation, and metric flags"
```

---

### Task 10: Implement Bayesian Search Strategy

**Files:**
- Create: `study/bayesian.go`
- Create: `study/bayesian_test.go`

This is the most complex task. Implement Gaussian process surrogate with Expected Improvement acquisition.

- [ ] **Step 1: Write failing tests for Bayesian strategy**

Create `study/bayesian_test.go`:

```go
var _ = Describe("Bayesian", func() {
    It("returns initial random samples on first call", func() {
        bayesian := study.NewBayesian(
            []study.ParamSweep{
                study.SweepRange("x", 0.0, 10.0, 0.1),
            },
            study.WithBatchSize(5),
            study.WithInitialSamples(10),
            study.WithMaxIterations(3),
        )

        configs, done := bayesian.Next(nil)
        Expect(configs).To(HaveLen(10))
        Expect(done).To(BeFalse())
    })

    It("returns guided samples after receiving scores", func() {
        bayesian := study.NewBayesian(
            []study.ParamSweep{
                study.SweepRange("x", 0.0, 10.0, 0.1),
            },
            study.WithBatchSize(5),
            study.WithInitialSamples(5),
            study.WithMaxIterations(3),
        )

        configs, _ := bayesian.Next(nil)
        // Build fake scores: higher score near x=5
        scores := make([]study.CombinationScore, len(configs))
        for ii, cfg := range configs {
            xVal, _ := strconv.ParseFloat(cfg.Params["x"], 64)
            scores[ii] = study.CombinationScore{
                Params: cfg.Params,
                Score:  -math.Abs(xVal - 5.0), // peak at x=5
            }
        }

        configs2, done := bayesian.Next(scores)
        Expect(configs2).To(HaveLen(5)) // batch size
        Expect(done).To(BeFalse())
    })

    It("terminates after max iterations", func() {
        bayesian := study.NewBayesian(
            []study.ParamSweep{
                study.SweepRange("x", 0.0, 10.0, 0.1),
            },
            study.WithBatchSize(2),
            study.WithInitialSamples(2),
            study.WithMaxIterations(2),
        )

        _, _ = bayesian.Next(nil) // iteration 1 (initial)
        scores := []study.CombinationScore{{Params: map[string]string{"x": "1"}, Score: 0.5}, {Params: map[string]string{"x": "2"}, Score: 0.6}}
        _, _ = bayesian.Next(scores) // iteration 2
        scores2 := []study.CombinationScore{{Params: map[string]string{"x": "3"}, Score: 0.7}, {Params: map[string]string{"x": "4"}, Score: 0.8}}
        _, done := bayesian.Next(scores2) // iteration 3 -> done
        Expect(done).To(BeTrue())
    })

    It("handles categorical parameters via integer encoding", func() {
        bayesian := study.NewBayesian(
            []study.ParamSweep{
                study.SweepValues("mode", "fast", "slow", "medium"),
            },
            study.WithBatchSize(3),
            study.WithInitialSamples(3),
            study.WithMaxIterations(2),
        )

        configs, _ := bayesian.Next(nil)
        for _, cfg := range configs {
            Expect(cfg.Params["mode"]).To(BeElementOf("fast", "slow", "medium"))
        }
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Bayesian"`
Expected: Compilation failure.

- [ ] **Step 3: Implement Bayesian search strategy**

Create `study/bayesian.go`. The implementation needs:

1. **Parameter encoding**: Map each sweep dimension to a continuous `[0, 1]` range. For numeric sweeps, linear scaling from `[min, max]`. For categorical sweeps, integer encoding `{0, 1, ..., n-1}` normalized to `[0, 1]`.

2. **GP surrogate**: Minimal Gaussian process with squared exponential (RBF) kernel. For low-dimensional parameter spaces (<10 dims), this is tractable:
   - Kernel: `k(x, x') = sigma^2 * exp(-||x - x'||^2 / (2 * l^2))`
   - Prediction: `mu(x) = K(x, X) * K(X, X)^-1 * y`, `var(x) = k(x, x) - K(x, X) * K(X, X)^-1 * K(X, x)`
   - Use Cholesky decomposition for numerical stability.

3. **Acquisition function**: Expected Improvement: `EI(x) = (mu(x) - best) * Phi(Z) + sigma(x) * phi(Z)` where `Z = (mu(x) - best) / sigma(x)`.

4. **Candidate selection**: Generate a large random candidate set, evaluate EI on each, take the top `batchSize`.

5. **Decoding**: Map selected points back to parameter values. Round categorical indices to nearest integer, clamp to valid range.

Options:
```go
type BayesianOption func(*bayesianStrategy)

func WithBatchSize(size int) BayesianOption { ... }
func WithMaxIterations(max int) BayesianOption { ... }
func WithInitialSamples(count int) BayesianOption { ... }
```

This is significant engineering. The GP implementation should be in a private helper (e.g., `study/gp.go`) to keep `bayesian.go` focused on the search strategy logic.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/... -v --focus "Bayesian"`
Expected: PASS

- [ ] **Step 5: Run full suite and lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./... && make lint`
Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add study/bayesian.go study/bayesian_test.go study/gp.go
git commit -m "feat: add Bayesian search strategy with GP surrogate and Expected Improvement"
```

---

### Task 11: Integration Test

**Files:**
- Create: `study/optimize/integration_test.go`

End-to-end test that wires up an Optimizer with a fake strategy, Grid search, TrainTest validation, and verifies the full pipeline produces a valid report.

- [ ] **Step 1: Write integration test**

```go
var _ = Describe("Integration", func() {
    It("runs a full optimization pipeline with grid search and train/test split", func() {
        // 1. Build a simple strategy with one numeric parameter (lookback).
        // 2. Create TrainTest split.
        // 3. Create Grid search with SweepRange on lookback.
        // 4. Create Optimizer study.
        // 5. Create Runner with SearchStrategy, Splits, Objective.
        // 6. Call runner.Run(ctx).
        // 7. Drain progress.
        // 8. Verify result has no error.
        // 9. Verify report has expected sections.
    })
})
```

The fake strategy should be simple enough to run quickly (e.g., a strategy that just returns constant values, or uses very short date ranges).

- [ ] **Step 2: Run integration test**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./study/optimize/... -v --focus "Integration"`
Expected: PASS

- [ ] **Step 3: Run full suite and lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./... && make lint`
Expected: All pass.

- [ ] **Step 4: Commit**

```bash
git add study/optimize/integration_test.go
git commit -m "test: add end-to-end integration test for parameter optimization pipeline"
```
