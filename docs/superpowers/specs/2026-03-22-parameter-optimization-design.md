# Parameter Optimization and Walk-Forward Validation

Covers issues #22 (parameter optimization) and #5 (walk-forward validation).

## Overview

Systematic parameter search with out-of-sample validation. The user defines parameter ranges, picks a search strategy (grid, random, or Bayesian), and chooses a validation scheme (train/test split, k-fold, or walk-forward). The framework evaluates each parameter combination across validation folds, ranks them by a configurable metric, and produces a report showing the best parameters and their robustness.

## Package Layout

All new code lives under the existing `study` package hierarchy:

- `study/` -- `SearchStrategy` interface, Grid, Random, and Bayesian implementations; `Scenario`, `Split`, and validation scheme types; existing Runner, ParamSweep, and CrossProduct
- `study/optimize/` -- parameter optimization Study implementation (peer to `study/stress/`, `study/montecarlo/`)

## Shared Scenario Library

Named date ranges are extracted from `study/stress/` into `study/` so all study types can use them.

```go
package study

type Scenario struct {
    Name        string
    Description string
    Start       time.Time
    End         time.Time
}

func AllScenarios() []Scenario
func ScenariosByName(names ...string) ([]Scenario, error)
```

`study/stress/` becomes a consumer: it imports `study.Scenario` directly (no type alias). The `DefaultScenarios()` function in stress moves to `study.AllScenarios()`. The stress test's `New()` accepts `[]study.Scenario`.

## Validation Schemes

A validation scheme produces a list of splits. Each split defines a training date range and a test date range. The engine runs a single backtest over the full bounding range for each split; the `Analyze()` method slices the equity curve into train and test windows for scoring. This follows the same pattern the stress test uses -- one engine run, multiple analysis windows.

```go
package study

type Period struct {
    Start time.Time
    End   time.Time
}

type Split struct {
    Name     string // human-readable label, e.g. "Fold 3" or "Walk 2010-2012"
    FullRange Period // bounding range for the engine run (Train.Start to Test.End)
    Train    Period // training evaluation window (may overlap FullRange for KFold)
    Test     Period // test evaluation window
    Exclude  []Period // date ranges to exclude from train scoring (KFold test folds, overlapping scenarios)
}
```

Each split produces one backtest run spanning `FullRange`. The `Analyze()` method evaluates performance separately on the train and test windows within that single run, excluding any `Exclude` periods from in-sample scoring. This avoids the problem of non-contiguous training periods and eliminates the need for multiple backtests per split.

The `Exclude` field handles two cases: (1) KFold, where the training range spans `[start, end]` but the test fold must be excluded from in-sample scoring, and (2) ScenarioLeaveNOut, where overlapping scenarios must be excluded from training evaluation.

### TrainTest

Simple single split. Everything before a cutoff date is training, everything after is test.

```go
func TrainTest(start, cutoff, end time.Time) []Split
```

Returns one `Split` with `Train: {start, cutoff}` and `Test: {cutoff, end}`.

### KFold

Divides a date range into k equal windows and rotates which is held out as test.

```go
func KFold(start, end time.Time, folds int) []Split
```

Returns k splits. Each split's `FullRange` is `[start, end]`, `Test` is the held-out fold, `Train` is `[start, end]`, and `Exclude` contains the test fold's period. The engine runs the full range; `Analyze()` evaluates in-sample performance over `Train` minus `Exclude` dates.

### WalkForward

Expanding (or sliding) training window with a fixed-size test window that advances through time.

```go
func WalkForward(start, end time.Time, minTrain, testLen, step time.Duration) []Split
```

Produces splits in chronological order. The first split trains on `[start, start+minTrain)` and tests on `[start+minTrain, start+minTrain+testLen)`. Each subsequent split advances the test window by `step`. Training expands to include all data before the test window.

An option for sliding (fixed-size) vs expanding training windows may be added later. The default is expanding.

### ScenarioLeaveNOut

Uses named scenarios from the shared scenario library. Each split holds out one (or N) scenarios as the test set and evaluates training on everything else within a bounding date range.

```go
func ScenarioLeaveNOut(scenarios []Scenario, boundStart, boundEnd time.Time, holdOut int) []Split
```

When `holdOut` is 1, this is leave-one-out cross-validation on named scenarios. `FullRange` is `[boundStart, boundEnd]`. The held-out scenario defines `Test`. `Train` spans `[boundStart, boundEnd]`. `Exclude` contains the held-out scenario period (and any other scenarios that overlap with it), so `Analyze()` can cleanly separate in-sample from out-of-sample scoring.

Overlapping scenarios (e.g., "Dot-com Bubble" 1998-2000 overlaps with "1998 LTCM / Russian Crisis") are handled by adding all overlapping scenario periods to `Exclude` when one of them is held out. This prevents information leakage from test-adjacent events into the training score.

**Error handling for all validation scheme constructors:** invalid inputs return errors. `KFold` returns an error if `folds < 2`. `WalkForward` returns an error if `minTrain + testLen` exceeds the date range. `TrainTest` returns an error if the cutoff is outside `[start, end]`. `ScenarioLeaveNOut` returns an error if `holdOut` exceeds the number of scenarios.

## Search Strategies

A `SearchStrategy` decides which parameter combinations to evaluate. The Runner calls it in a loop.

```go
package study

// CombinationScore reports the aggregated score for a single parameter combination
// across all validation folds.
type CombinationScore struct {
    Params map[string]string // parameter values for this combination
    Score  float64           // mean objective score across folds
    Runs   []RunResult       // individual per-fold results
}

type SearchStrategy interface {
    Next(scores []CombinationScore) (configs []RunConfig, done bool)
}
```

The first call passes `nil` scores. The strategy returns the initial batch of configs (one per parameter combination; the Runner handles split expansion). The Runner executes all runs for the batch, aggregates per-combination scores, and calls `Next()` with the results. This repeats until `done` is true.

### Grid

Exhaustive cross-product of all sweep dimensions. Returns everything from the first `Next(nil)` call and immediately returns `done=true`.

```go
func NewGrid(sweeps ...ParamSweep) SearchStrategy
```

Wraps the existing `CrossProduct` logic.

### Random

Samples N parameter combinations uniformly from the sweep ranges. Returns everything from the first call, `done=true`.

```go
func NewRandom(sweeps []ParamSweep, samples int, seed int64) SearchStrategy
```

For continuous sampling, `ParamSweep` gains exported `Min()`, `Max()` methods that return the range bounds (empty strings for `SweepValues`/`SweepPresets`). When bounds are available, Random samples uniformly within `[min, max]`. When only discrete values exist, it samples uniformly from the value list.

### Bayesian

Guided search using a Gaussian process surrogate model and an acquisition function. Returns small batches and iteratively refines.

```go
func NewBayesian(sweeps []ParamSweep, opts ...BayesianOption) SearchStrategy
```

Options:

- `WithBatchSize(int)` -- number of candidates per iteration. Default 10.
- `WithMaxIterations(int)` -- budget cap. Default 20.
- `WithInitialSamples(int)` -- size of the random initial batch before the surrogate kicks in. Default equal to batch size.

The first `Next(nil)` call returns `initialSamples` random points (Latin hypercube or uniform). Subsequent calls fit the surrogate to observed (params, score) pairs from `CombinationScore` and use Expected Improvement to select the next batch. Returns `done=true` when `maxIterations` is reached or improvement stalls.

**Categorical and discrete parameters:** The GP operates on a continuous numeric encoding. Categorical parameters (from `SweepValues` and `SweepPresets`) are integer-encoded (0, 1, 2, ...) and rounded to the nearest valid index when the GP proposes a point. Discrete integer parameters (from `SweepRange` with integer type) are rounded to the nearest integer. This is the standard approach used by Optuna (TPE) and scikit-optimize.

## Runner Changes

The Runner gains `SearchStrategy`, `Splits`, and `Objective` fields.

```go
type Runner struct {
    Study          Study
    NewStrategy    func() engine.Strategy
    Options        []engine.Option
    Workers        int
    Sweeps         []ParamSweep            // batch mode (existing)
    SearchStrategy SearchStrategy          // iterative mode (new)
    Splits         []Split                 // validation splits (new)
    Objective      Metric                  // scoring metric (new)
}
```

The `Objective` metric is the single source of truth for scoring. It lives on the Runner. The search strategy receives scores via `CombinationScore`; the optimization study receives all `RunResult` values and uses the same metric for its analysis.

**Execution flow when `SearchStrategy` is set:**

1. Call `Study.Configurations(ctx)` to get base configs.
2. Call `SearchStrategy.Next(nil)` to get the first batch of parameter combinations (as `[]RunConfig` with `Params` or `Preset` populated).
3. For each parameter combination in the batch, cross-product with `Splits` to produce run configs. Each combination gets a sequential integer combination ID in `RunConfig.Metadata["_combination_id"]` (e.g., "0", "1", "2"). Each split adds `RunConfig.Metadata["_split_index"]` and `RunConfig.Metadata["_split_name"]`. The run's `Start` and `End` are set from `split.FullRange`.
4. Execute all runs for the batch concurrently (existing worker pool).
5. Group results by combination ID. For each combination, compute the test-window score per fold via `WindowedScore(result.Portfolio, split.Test, runner.Objective)`. Average the non-NaN scores to get the combination's aggregate score. For failed runs (`RunResult.Err != nil`), the fold score is `math.NaN()`. Combinations where all folds are NaN are excluded from ranking but still passed to `Analyze()`.
6. Call `SearchStrategy.Next(combinationScores)` with per-combination aggregated results.
7. Repeat until `done`.
8. Collect all results across all batches and call `Study.Analyze(allResults)`.

**Preset sweeps in search strategies:** `SweepPresets` values are preset names. Grid handles these via existing `CrossProduct` logic (sets `RunConfig.Preset`). Random samples uniformly from the preset name list and sets `RunConfig.Preset`. Bayesian integer-encodes preset names (0, 1, 2, ...) and maps back to names when emitting configs. All three strategies set `RunConfig.Preset` for preset sweeps and `RunConfig.Params` for field sweeps, matching the existing `CrossProduct` behavior.

**When `Splits` is empty**, each combination runs once against the study's base configs (no validation splitting). This is backward-compatible with existing behavior.

**When `SearchStrategy` is nil**, the existing `Sweeps` + `CrossProduct` path runs unchanged. `Sweeps` and `SearchStrategy` are mutually exclusive; setting both is an error.

**Progress reporting** adapts to iterative execution. `Progress` gains two fields:

```go
BatchIndex int // which batch this run belongs to (0-indexed)
BatchSize  int // number of runs in this batch
```

For Bayesian search, `TotalRuns` reflects the current batch only (the total is unknown upfront). Grid and random complete in one batch and report accurate totals.

**Result contract:** `Run()` still returns `(<-chan Progress, <-chan Result, error)`. The result channel emits a single `Result` when all batches are complete and `Analyze()` has run. Intermediate batch results are not emitted -- the search loop is internal to the Runner. Callers see the same contract as today.

## Windowed Scoring

The portfolio performance metric system (`PerformanceMetric()`) computes over the full equity curve. For validation, scoring must be restricted to specific date windows (train or test). The framework provides a `WindowedScore` function that:

1. Extracts the equity curve DataFrame from `RunResult.Portfolio` via `PerfData()`.
2. Slices it to the target date range using `DataFrame.Between(start, end)`.
3. Computes the requested metric directly from the sliced equity values.

This follows the same approach the stress test uses in `study/stress/analyze.go` -- direct computation from raw equity data rather than the `PerformanceMetric()` API. The computation logic is shared in a `study` package helper so both the stress test and optimization study can use it.

```go
package study

// WindowedScore computes a metric over a date range within a portfolio's equity curve.
func WindowedScore(portfolio report.ReportablePortfolio, window Period, metric Metric) float64

type Metric int

const (
    MetricSharpe Metric = iota
    MetricCAGR
    MetricMaxDrawdown
    MetricSortino
    MetricCalmar
)
```

If the window contains insufficient data for the metric (e.g., fewer than 2 data points), `WindowedScore` returns `math.NaN()`.

## Objective Functions and Runner Scoring

The Runner's `Objective` field specifies which metric to optimize:

```go
// On the Runner struct:
Objective Metric
```

During the search loop, the Runner scores each run by calling `WindowedScore(result.Portfolio, split.Test, runner.Objective)` -- scoring only the **test window**, not the full run. This is critical: the search strategy must optimize on out-of-sample performance to avoid overfitting.

For aggregation across folds, the Runner computes `mean(score for each non-NaN fold)` per combination. Combinations where all folds are `NaN` are excluded from the search strategy's ranking but still passed to `Analyze()`.

The `Analyze()` method computes both in-sample and out-of-sample scores independently using `WindowedScore` with the appropriate windows, for the full report.

## Parameter Optimization Study

`study/optimize/` implements `study.Study`. It produces an optimization-focused report.

```go
package optimize

type Optimizer struct {
    splits    []study.Split
    objective study.Metric
    topN      int
}

func New(splits []study.Split, opts ...Option) *Optimizer
```

Options: `WithObjective(study.Metric)`, `WithTopN(int)`.

### Configurations

`Configurations()` returns a single base config. The date range spans the full bounding range of all splits (earliest `Train.Start` to latest `Test.End`). The Runner handles parameter combination expansion and split cross-producting -- the Optimizer does not duplicate that responsibility.

### Analyze

`Analyze()` receives all run results across all batches, combinations, and splits. It:

1. Groups results by combination ID (from `RunConfig.Metadata["_combination_id"]`).
2. For each combination and split, slices the portfolio's equity curve into train and test windows using the split's `Train` and `Test` periods.
3. Computes in-sample (train window) and out-of-sample (test window) scores per combination per fold using the objective function.
4. Ranks combinations by mean out-of-sample score.
5. Composes the report.

### Report Sections

- **Rankings table** -- top N parameter combinations ranked by mean out-of-sample score. Columns: rank, parameter values, mean OOS score, mean in-sample score, score standard deviation across folds.
- **Best combination detail** -- per-fold breakdown for the top combination: in-sample score, out-of-sample score, and key metrics (CAGR, max drawdown, Sharpe) for each fold.
- **Parameter sensitivity** -- for each parameter, how the objective varies as that parameter changes (holding others at their best values). Helps identify which parameters matter most.
- **Overfitting check** -- table comparing in-sample vs out-of-sample scores across all evaluated combinations. A large gap indicates overfitting.
- **Equity curves** -- time series for the top N combinations on their out-of-sample folds.

## CLI Integration

From a compiled strategy binary (e.g., `adm`):

```
./adm study optimize [flags]
```

From pvbt:

```
pvbt study optimize --strategy=adm [flags]
```

Flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--search` | Search strategy: `grid`, `random`, `bayesian` | `grid` |
| `--metric` | Objective metric: `sharpe`, `cagr`, `max-drawdown`, `sortino`, `calmar` | `sharpe` |
| `--validation` | Validation scheme: `train-test`, `kfold`, `walk-forward`, `scenario` | `train-test` |
| `--folds` | Number of folds (k-fold) | `5` |
| `--train-end` | Cutoff date for train/test split | |
| `--min-train` | Minimum training period (walk-forward) | `5y` |
| `--test-len` | Test window length (walk-forward) | `2y` |
| `--step` | Step size (walk-forward) | `1y` |
| `--samples` | Number of random samples | `100` |
| `--batch-size` | Bayesian batch size | `10` |
| `--max-iter` | Bayesian max iterations | `20` |
| `--workers` | Concurrent workers | `GOMAXPROCS` |
| `--top` | Number of top results to show | `10` |
| `--format` | Output format: `text`, `json` | `text` |

Parameter sweep ranges are specified via strategy parameter flags with a range syntax extension. When a parameter flag value contains colons, it is interpreted as `min:max:step` (e.g., `--lookback=3:24:1`). When it contains no colons, it is a single value as today. This is unambiguous because single parameter values do not contain colons in any supported type (float, int, string identifiers, durations use `h`/`m`/`s`).

## Stress Test Migration

The stress test study moves `Scenario` and `DefaultScenarios()` up to the `study` package as `study.Scenario` and `study.AllScenarios()`. The `stress.StressTest` struct changes to accept `[]study.Scenario`. The `stress.Scenario` type is removed. No behavioral changes -- the stress test works exactly as before, just imports the scenario type from its parent package.

## ParamSweep Extensions

`ParamSweep` gains methods to expose range metadata for continuous sampling:

```go
func (ps ParamSweep) Min() string  // empty for SweepValues/SweepPresets
func (ps ParamSweep) Max() string  // empty for SweepValues/SweepPresets
```

`SweepRange` and `SweepDuration` store their original min/max values. `Min()` and `Max()` return these as strings. `SweepValues` and `SweepPresets` return empty strings (sampling falls back to the materialized value list).

## Dependencies

No external dependencies for grid or random search. Bayesian optimization requires a Gaussian process implementation. Options:

1. Implement a minimal GP in-house (the parameter spaces are low-dimensional, typically <10 parameters)
2. Use an existing Go library if one meets quality standards

This decision is deferred to implementation. The `SearchStrategy` interface isolates the choice -- the Runner does not care what is behind `Next()`.

## Implementation Order

1. Extract scenarios from `study/stress/` into `study/`
2. Update stress test to use `study.Scenario`
3. Add `Period`, `Split`, and validation scheme functions to `study/`
4. Extend `ParamSweep` with `Min()`/`Max()` methods
5. Add `CombinationScore` type and `SearchStrategy` interface to `study/`
6. Implement Grid and Random search strategies
7. Update Runner to support `SearchStrategy`, `Splits`, and `Objective`
8. Implement `study/optimize/` with `Analyze()` and report composition
9. Add CLI `optimize` subcommand
10. Implement Bayesian search strategy
