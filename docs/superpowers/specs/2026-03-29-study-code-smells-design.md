# Study Package: Code Smells and Style Fixes

**Issue:** #107
**Date:** 2026-03-29

## Overview

Five localized fixes to the study and optimize packages addressing style issues, floating-point correctness, resource sizing, placeholder data, and a type-system redesign to prevent silent sorting bugs.

## Item 1: Metric Iota Style

`study/metric.go` repeats `= iota` on every constant in the `Metric` const block. Idiomatic Go uses `= iota` only on the first constant; subsequent constants inherit the incrementing behavior. Remove the redundant assignments.

This const block is deleted entirely by Item 5 (below), so this fix only applies if other iota blocks in the codebase have the same problem.

## Item 2: SweepRange Floating-Point Accumulation

`study/sweep.go:55-62` -- `SweepRange` accumulates floating-point error by iterating `val += step`. For float64 steps over wide ranges, this drifts.

**Fix:** Compute each value from its index:

```go
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

Single multiplication from a known origin eliminates accumulated error.

## Item 3: Progress Channel Buffer Sizing

`study/runner.go` uses two different buffer strategies:

- Standard sweep path (line 65): `make(chan Progress, len(configs)*2)` -- scales unboundedly with workload
- Search path (line 153): `make(chan Progress, 64)` -- fixed

The consumer (`cli/study.go:102-112`) is a simple logging loop that drains the channel. A large buffer just delays backpressure without adding throughput.

**Fix:** Use a consistent small fixed buffer (64) on both paths. The consumer reads fast enough that this won't bottleneck, and if it does, backpressure is the correct behavior.

## Item 4: Equity Curve Placeholders

`optimize/analyze.go:312-327` -- `computeEquityCurves` creates empty `equityCurveSeries` objects with only a name. The portfolio's equity curve data (`PerfData`) is available on each `RunResult.Portfolio` but is not retained through the analysis pipeline.

**Fix:** The full `RunResult` array (including each portfolio's `PerfData()`) is already in memory during analysis. The current code just never extracts the equity curves from it. The fix happens in two phases:

1. During `groupByCombination`, when iterating RunResults to compute OOS/IS scores, also extract the equity column from `rr.Portfolio.PerfData()` for each split's test window. Store these per-split equity slices on `comboResult`.
2. After `rankCombos` identifies the top 10, discard equity data from all other combinations. `computeEquityCurves` reads directly from the top 10 combo results instead of creating empty placeholders.

The `comboResult` struct gains a field for per-split equity curve data (timestamps + values). Memory is bounded because only the top 10 retain this data after ranking.

## Item 5: Eliminate study.Metric, Add portfolio.Rankable

`study.Metric` is an int enum that maps 1:1 to `portfolio.PerformanceMetric` implementations via a switch statement (`performanceMetric()`). This duplication means adding a new metric requires updating both packages, and the standalone `higherIsBetter()` function in `optimize/analyze.go` always returns `true` regardless of input -- a silent bug waiting to happen if a lower-is-better metric is added.

**Fix:** Delete `study.Metric` entirely. Add a `Rankable` interface to the portfolio package:

```go
type Rankable interface {
    PerformanceMetric
    HigherIsBetter() bool
}
```

The five optimization-eligible metrics (Sharpe, CAGR, MaxDrawdown, Sortino, Calmar) implement `Rankable` by adding a `HigherIsBetter()` method to their existing unexported structs. All return `true` (MaxDrawdown returns negative values, so numerically higher is better).

### Changes required

**portfolio package:**
- Add `Rankable` interface to `metric_query.go`
- Add `HigherIsBetter() bool` method to `sharpe`, `cagrMetric`, `maxDrawdown`, `sortino`, `calmar`

**study package:**
- Delete `study/metric.go` type definition, const block, and `performanceMetric()` method
- Change `WindowedScore` and `WindowedScoreExcluding` signatures from `metric Metric` to `metric portfolio.PerformanceMetric`
- Delete `metricName()` in `optimize/analyze.go` (use `metric.Name()` from the interface)
- Delete `higherIsBetter()` in `optimize/analyze.go` (use `metric.HigherIsBetter()` from `Rankable`)

**study/optimize package:**
- Change `Optimizer.objective` field from `study.Metric` to `portfolio.Rankable`
- Change `WithObjective` option signature accordingly
- Change default from `study.MetricSharpe` to `portfolio.Sharpe` (which implements `Rankable`)
- Update `analyzeResults`, `groupByCombination`, `rankCombos` signatures

**cli package:**
- `parseMetric` in `cli/study.go` returns `portfolio.Rankable` instead of `study.Metric`
- Map flag strings ("sharpe", "cagr", etc.) directly to `portfolio.Sharpe`, `portfolio.CAGR`, etc.

**tests:**
- Replace all `study.MetricCAGR`, `study.MetricSharpe`, etc. with `portfolio.CAGR`, `portfolio.Sharpe`, etc.
- Update unknown-metric test to verify that a non-`Rankable` metric is rejected
