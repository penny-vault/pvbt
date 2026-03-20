# Warmup and DateRangeMode Design

## Problem

Strategies often need historical data before their first compute date to
initialize indicators (e.g., a 200-day moving average needs 200 prior trading
days). Today the engine has no way to express or validate this requirement.
If data is missing in the lookback window, the strategy silently computes on
incomplete data.

## Solution

Add a **warmup** declaration to `StrategyDescription` and a **DateRangeMode**
engine option that controls how the engine reacts when warmup data is
unavailable.

## Design

### 1. Warmup field on StrategyDescription

Add `Warmup int` to `StrategyDescription` and `StrategyInfo`. The value is
the number of **trading days** of data the strategy needs before its first
compute date. Default is 0 (no warmup). Negative values are rejected with an
error during backtest initialization.

```go
type StrategyDescription struct {
    ShortCode   string    `json:"shortcode,omitempty"`
    Description string    `json:"description,omitempty"`
    Source      string    `json:"source,omitempty"`
    Version     string    `json:"version,omitempty"`
    VersionDate time.Time `json:"versionDate,omitzero"`
    Schedule    string    `json:"schedule,omitempty"`
    Benchmark   string    `json:"benchmark,omitempty"`
    Warmup      int       `json:"warmup,omitempty"`
}

type StrategyInfo struct {
    Name        string                       `json:"name"`
    ShortCode   string                       `json:"shortcode,omitempty"`
    Description string                       `json:"description,omitempty"`
    Source      string                       `json:"source,omitempty"`
    Version     string                       `json:"version,omitempty"`
    VersionDate time.Time                    `json:"versionDate,omitzero"`
    Schedule    string                       `json:"schedule,omitempty"`
    Benchmark   string                       `json:"benchmark,omitempty"`
    RiskFree    string                       `json:"riskFree,omitempty"`
    Warmup      int                          `json:"warmup,omitempty"`
    Parameters  []ParameterInfo              `json:"parameters"`
    Suggestions map[string]map[string]string `json:"suggestions,omitempty"`
}
```

Strategies declare warmup in `Describe()`:

```go
func (s *MomentumRotation) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        ShortCode: "ADM",
        Warmup:    126,
    }
}
```

`DescribeStrategy()` copies the value into `StrategyInfo` so it appears in
JSON `describe` output.

### 2. DateRangeMode type and engine option

A new file `engine/date_range_mode.go` defines the mode:

```go
type DateRangeMode int

const (
    DateRangeModeStrict    DateRangeMode = iota // error if warmup data unavailable
    DateRangeModePermissive                      // adjust start date forward
)
```

`DateRangeModeStrict` is the default (the zero value).

A new engine option:

```go
func WithDateRangeMode(mode DateRangeMode) Option {
    return func(e *Engine) {
        e.dateRangeMode = mode
    }
}
```

The `Engine` struct gains two fields:

- `dateRangeMode DateRangeMode` (set via option, default strict)
- `warmup int` (extracted from strategy's `Describe()` during backtest init)

### 3. Warmup validation in Backtest

The warmup check runs in `Backtest()` after step 5 (schedule validation)
and before step 6 (account creation). This ensures that strategy fields are
hydrated, `Setup()` has run, the `Describe()` fallback for schedule/benchmark
has been applied, and the schedule has been validated as non-nil. If
permissive mode adjusts the start date, the adjusted value is used for all
subsequent steps including account creation, risk-free prefetch, `e.start`
assignment, and step enumeration.

Note: `start` in the warmup context means the first scheduled trading date
on or after the user-supplied start (computed via
`schedule.Next(start.Add(-time.Nanosecond))`), not the raw `start` parameter.
This handles cases where the user passes a weekend or holiday.

If warmup is 0, the check is skipped entirely.

#### 3a. Extract warmup

Extract warmup from the strategy's `Describe()` output. If the strategy does
not implement `Descriptor`, warmup is 0. If the value is negative, return an
error.

#### 3b. Compute warmup start date

TradeCron has no backward-walking API. To find the date `warmup` trading days
before `start`:

1. Estimate a calendar-day offset: `warmup * 2` calendar days (accounts for
   weekends, holidays).
2. Create a daily schedule (`@close * * *`, `tradecron.RegularHours`) and
   walk forward from the estimated date, counting trading days until reaching
   `start`.
3. If the count exceeds `warmup`, the estimated start is correct or
   conservative. Take the trading day at position `count - warmup` from the
   collected list as the warmup start date.
4. If the count is less than `warmup` (unlikely with the 2x multiplier),
   double the offset and retry, up to 3 attempts. If all attempts fail,
   return an error.

This logic lives in a helper function in `engine/warmup.go`:
`walkBackTradingDays(from time.Time, days int) (time.Time, error)`.

#### 3c. Collect assets to validate

Reflect over the strategy struct's top-level exported fields (matching the
same flat, non-recursive pattern used by `hydrateFields`):

- For each `universe.Universe` field: if the concrete type is
  `*universe.StaticUniverse`, call `Assets(time.Time{})` to get members.
  Skip dynamic universe types (index, rated) entirely rather than calling
  `Assets` with a meaningless time.
- For each `asset.Asset` field, include it directly.
- Include the benchmark asset if set.
- Deduplicate by `CompositeFigi`.

Assets from `WithPortfolioSnapshot` are not included -- snapshot data
availability is the caller's responsibility.

#### 3d. Check data availability

Fetch `MetricClose` over `[warmupStart, start)` for all collected assets.

An asset is considered to have **insufficient warmup data** when its
`MetricClose` column contains fewer than `warmup` non-NaN values in the
fetched DataFrame. This accounts for assets that may have partial coverage
(e.g., an ETF that launched after the warmup start date).

#### 3e. Handle failures

**Strict mode**: return an error listing which assets have insufficient data
and how many trading days each is short by.

**Permissive mode**: scan forward one trading day at a time from `start`.
At each candidate date, recompute the warmup window and re-check availability
for the failing assets. Stop at the first date where all assets have
sufficient data. If the adjusted start date >= `end`, return an error
explaining that no valid start date exists within the requested range.

### 4. Implementation files

| File | Change |
|------|--------|
| `engine/descriptor.go` | Add `Warmup int` to `StrategyDescription` and `StrategyInfo`; wire through `DescribeStrategy()` |
| `engine/engine.go` | Add `dateRangeMode` and `warmup` fields to `Engine` struct |
| `engine/option.go` | Add `WithDateRangeMode()` option |
| `engine/date_range_mode.go` (new) | `DateRangeMode` type and constants |
| `engine/warmup.go` (new) | Validation logic: `walkBackTradingDays`, asset collection, data availability check, permissive-mode adjustment |
| `engine/backtest.go` | Call warmup validation after `Setup()`, before account creation; use adjusted start if permissive |

### 5. What does not change

- The step loop and `Compute()` behavior are unchanged.
- Data providers, portfolio, and universe interfaces are unchanged.
- Warmup is purely a pre-flight data availability check. The engine does not
  run a separate warmup phase or discard early trades.

### 6. Future use

The warmup field enables the future `strategy:` ticker prefix feature. When
a parent strategy includes a sub-strategy as an asset, the engine uses the
sub-strategy's declared warmup to know how far before the parent's start
date the sub-strategy must begin running to produce a usable equity curve.
