# Fix WindowedScoreExcluding: Portfolio.View Design

**Issue:** #102 -- `WindowedScoreExcluding` ignores its `exclude` parameter. KFold IS scores include OOS data, breaking the overfitting diagnostic.

**Approach:** Add a `View(start, end)` method to Portfolio that returns a windowed Portfolio. The study package computes which date ranges are IS vs OOS and calls `View` for each. No exclusion concept at the portfolio level -- just windowed views.

## Changes

### 1. `portfolio/viewed_portfolio.go` (new file)

A `viewedPortfolio` struct that wraps `*Account` and a `windowedStats`. Implements `Portfolio` and `PortfolioStats` (therefore satisfies `ReportablePortfolio`).

- `PerformanceMetric(m)` delegates to the account but pre-applies `AbsoluteWindow(start, end)`.
- All `PortfolioStats` methods (Returns, ExcessReturns, Drawdown, etc.) delegate to the inner `windowedStats`.
- Point-in-time methods (Cash, Value, Holdings, etc.) pass through to the account.

### 2. `portfolio/account.go`

Add one method:

```go
func (acct *Account) View(start, end time.Time) Portfolio {
    return &viewedPortfolio{
        acct:  acct,
        stats: newWindowedStats(acct, start, end),
        start: start,
        end:   end,
    }
}
```

This is a public API addition to Portfolio. It parallels `AbsoluteWindow` on the metric query but operates at the portfolio level and returns a first-class Portfolio.

### 3. `study/split.go`

Add a `subtractRanges` function:

```go
func subtractRanges(window DateRange, exclude []DateRange) []DateRange
```

Returns the portions of `window` not covered by any range in `exclude`. For KFold fold 2/3 on [Jan, Dec] with test = [May, Aug], returns `[Jan, Apr]` and `[Sep, Dec]`.

### 4. `study/metric.go`

**`WindowedScore`** simplifies to:

```go
func WindowedScore(rp report.ReportablePortfolio, window DateRange, metric portfolio.PerformanceMetric) float64 {
    val, err := rp.View(window.Start, window.End).PerformanceMetric(metric).Value()
    if err != nil {
        return math.NaN()
    }
    return val
}
```

**`WindowedScoreExcluding`** computes IS segments and averages:

```go
func WindowedScoreExcluding(rp report.ReportablePortfolio, window DateRange, exclude []DateRange, metric portfolio.PerformanceMetric) float64 {
    if len(exclude) == 0 {
        return WindowedScore(rp, window, metric)
    }

    segments := subtractRanges(window, exclude)

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

For KFold interior folds where exclusion creates two disjoint IS segments, this computes the metric on each segment via `View` and takes a duration-weighted average. This is standard practice for cross-validated metric comparison.

### 5. Tests

- `portfolio/viewed_portfolio_test.go`: verify `View` returns a Portfolio whose metrics match `AbsoluteWindow` results for the same range.
- `study/split_test.go`: `subtractRanges` cases -- single exclusion at start/middle/end, multiple exclusions, full coverage returns empty.
- `study/metric_test.go`: update `WindowedScoreExcluding` tests to assert the excluded score differs from the full-window score. Add a KFold-style test with an interior fold.

### 6. `ReportablePortfolio` interface

`View` is added to the `Portfolio` interface, which `ReportablePortfolio` composes. The `viewedPortfolio` type itself satisfies `ReportablePortfolio`, so views can be passed anywhere a `ReportablePortfolio` is accepted.

## What doesn't change

- `windowedStats` -- reused internally by `viewedPortfolio`, untouched.
- `DataFrame` -- no new methods needed.
- `PerformanceMetricQuery` / `AbsoluteWindow` -- untouched.
- `optimize/analyze.go` -- already calls `WindowedScoreExcluding`, gets correct results automatically.

## Interaction with ongoing refactoring

The study-code-smells plan (Tasks 1-4) replaces `study.Metric` with `portfolio.Rankable`. This fix is compatible: `WindowedScore` and `WindowedScoreExcluding` already accept `portfolio.PerformanceMetric` after that refactoring. If the refactoring lands first, this fix uses `portfolio.PerformanceMetric` in signatures. If this fix lands first, the refactoring updates the signatures as planned. No conflicts either way.
