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

`study/stress/` becomes a consumer: its `Scenario` type alias or direct import references `study.Scenario`. The `DefaultScenarios()` function in stress moves to `study.AllScenarios()`. The stress test's `New()` accepts `[]study.Scenario`.

## Validation Schemes

A validation scheme produces a list of splits. Each split has training periods (possibly non-contiguous) and a single test period. All schemes produce the same `Split` type.

```go
package study

type Period struct {
    Start time.Time
    End   time.Time
}

type Split struct {
    Name  string   // human-readable label, e.g. "Fold 3" or "Walk 2010-2012"
    Train []Period // one or more training windows
    Test  Period   // single test window
}
```

### TrainTest

Simple single split. Everything before a cutoff date is training, everything after is test.

```go
func TrainTest(start, cutoff, end time.Time) []Split
```

Returns one `Split` with `Train: [{start, cutoff}]` and `Test: {cutoff, end}`.

### KFold

Divides a date range into k equal windows and rotates which is held out as test.

```go
func KFold(start, end time.Time, folds int) []Split
```

Returns k splits. Each split's `Train` contains k-1 contiguous windows (as separate `Period` entries since they may not be adjacent after removing the test fold). The test window is the held-out fold.

### WalkForward

Expanding (or sliding) training window with a fixed-size test window that advances through time.

```go
func WalkForward(start, end time.Time, minTrain, testLen, step time.Duration) []Split
```

Produces splits in chronological order. The first split trains on `[start, start+minTrain)` and tests on `[start+minTrain, start+minTrain+testLen)`. Each subsequent split advances the test window by `step`. Training expands to include all data before the test window.

An option for sliding (fixed-size) vs expanding training windows may be added later. The default is expanding.

### ScenarioLeaveNOut

Uses named scenarios from the shared scenario library. Each split holds out one (or N) scenarios as the test set and trains on everything else within a bounding date range.

```go
func ScenarioLeaveNOut(scenarios []Scenario, boundStart, boundEnd time.Time, holdOut int) []Split
```

When `holdOut` is 1, this is leave-one-out cross-validation on named scenarios. The training periods are the portions of `[boundStart, boundEnd]` that do not overlap with the held-out scenario(s). Since scenarios may be non-contiguous, the training periods may be multiple `Period` entries.

## Search Strategies

A `SearchStrategy` decides which parameter combinations to evaluate. The Runner calls it in a loop.

```go
package study

type SearchStrategy interface {
    Next(results []RunResult) (configs []RunConfig, done bool)
}
```

The first call passes `nil` results. The strategy returns the initial batch of configs. The Runner executes them, then calls `Next()` with the completed results. This repeats until `done` is true.

For validation, each "parameter combination" produces multiple runs (one per split). The Runner runs all splits for a batch, then aggregates results per combination before feeding scores back to the search strategy.

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

For `SweepRange` inputs, samples uniformly within `[min, max]` rather than only at step boundaries. For `SweepValues` inputs, samples uniformly from the value list.

### Bayesian

Guided search using a Gaussian process surrogate model and an acquisition function. Returns small batches and iteratively refines.

```go
func NewBayesian(sweeps []ParamSweep, opts ...BayesianOption) SearchStrategy
```

Options:

- `WithObjective(func(RunResult) float64)` -- extracts a scalar score from a run result. Required.
- `WithBatchSize(int)` -- number of candidates per iteration. Default 10.
- `WithMaxIterations(int)` -- budget cap. Default 20.
- `WithInitialSamples(int)` -- size of the random initial batch before the surrogate kicks in. Default equal to batch size.

The first `Next(nil)` call returns `initialSamples` random points (Latin hypercube or uniform). Subsequent calls fit the surrogate to observed (params, score) pairs and use Expected Improvement to select the next batch. Returns `done=true` when `maxIterations` is reached or improvement stalls.

The objective function receives aggregated results -- the mean score across validation folds for a parameter combination. The Runner handles this aggregation before calling `Next()`.

## Runner Changes

The Runner gains a `SearchStrategy` field. When set, it replaces the existing `Sweeps`-based execution.

```go
type Runner struct {
    Study          Study
    NewStrategy    func() engine.Strategy
    Options        []engine.Option
    Workers        int
    Sweeps         []ParamSweep       // batch mode (existing)
    SearchStrategy SearchStrategy     // iterative mode (new)
    Splits         []Split            // validation splits (new)
    Objective      func(RunResult) float64  // scoring function (new)
}
```

**Execution flow when `SearchStrategy` is set:**

1. Call `Study.Configurations(ctx)` to get base configs.
2. Call `SearchStrategy.Next(nil)` to get the first batch of parameter combinations.
3. For each parameter combination in the batch, cross-product with `Splits` to produce run configs. Each run gets metadata tagging its combination ID and split index.
4. Execute all runs for the batch concurrently (existing worker pool).
5. Group results by combination ID. For each combination, compute the mean objective score across its splits (using the `Objective` function on each split's result).
6. Call `SearchStrategy.Next(aggregatedResults)` with per-combination results. The aggregated score is attached to the results so the strategy can use it.
7. Repeat until `done`.
8. Collect all results and call `Study.Analyze(allResults)`.

**When `Splits` is empty**, each combination runs once against the study's base configs (no validation splitting). This is backward-compatible with the existing behavior.

**When `SearchStrategy` is nil**, the existing `Sweeps` + `CrossProduct` path runs unchanged. The two fields are mutually exclusive; setting both is an error.

**Progress reporting** adapts to iterative execution. `TotalRuns` is unknown upfront for Bayesian search, so `Progress` gains a `BatchIndex` and `BatchTotal` field to report per-batch progress. Grid and random report accurate totals since they complete in one batch.

## Objective Functions

Common metric extractors shipped with the framework:

```go
package study

func ObjectiveSharpe(result RunResult) float64
func ObjectiveCAGR(result RunResult) float64
func ObjectiveMaxDrawdown(result RunResult) float64
func ObjectiveSortino(result RunResult) float64
func ObjectiveCalmar(result RunResult) float64
```

Each extracts the metric from `result.Portfolio` using the existing performance metric system. Users can provide custom functions with the same signature.

For validation, the Runner computes `mean(objective(split_result) for each split)` per combination and uses that as the combination's score. The optimization report shows both the mean and per-fold scores.

## Parameter Optimization Study

`study/optimize/` implements `study.Study`. It manages validation splits and produces an optimization-focused report.

```go
package optimize

type Optimizer struct {
    Splits    []study.Split
    Objective func(study.RunResult) float64
    TopN      int // number of top results to feature in the report (default 10)
}

func New(splits []study.Split, opts ...Option) *Optimizer
```

Options: `WithObjective`, `WithTopN`.

### Configurations

`Configurations()` returns base configs derived from the validation splits. For each split, it produces train and test configs. The split metadata (combination ID, split index, train/test role) is embedded in `RunConfig.Metadata`.

### Analyze

`Analyze()` receives all run results across all combinations and splits. It:

1. Groups results by combination ID.
2. For each combination, separates train and test results.
3. Computes mean in-sample and out-of-sample scores per combination.
4. Ranks combinations by out-of-sample score.
5. Composes the report.

### Report Sections

- **Rankings table** -- top N parameter combinations ranked by mean out-of-sample score. Columns: rank, parameter values, mean OOS score, mean in-sample score, score standard deviation across folds.
- **Best combination detail** -- per-fold breakdown for the top combination: in-sample score, out-of-sample score, and key metrics (CAGR, max drawdown, Sharpe) for each fold.
- **Parameter sensitivity** -- for each parameter, how the objective varies as that parameter changes (holding others at their best values). Helps identify which parameters matter most.
- **Overfitting check** -- scatter or table comparing in-sample vs out-of-sample scores across all combinations. A large gap indicates overfitting.
- **Equity curves** -- time series for the top N combinations on their out-of-sample folds.

## CLI Integration

```
./adm study optimize [flags]
```

Flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--search` | Search strategy: `grid`, `random`, `bayesian` | `grid` |
| `--metric` | Objective metric: `sharpe`, `cagr`, `max-drawdown`, `sortino`, `calmar` | `sharpe` |
| `--validation` | Validation scheme: `train-test`, `kfold`, `walk-forward`, `scenario` | `train-test` |
| `--folds` | Number of folds (k-fold) | `5` |
| `--train-end` | Cutoff date for train/test split | |
| `--test-start` | Start of test period (walk-forward) | |
| `--min-train` | Minimum training period (walk-forward) | `5y` |
| `--test-len` | Test window length (walk-forward) | `2y` |
| `--step` | Step size (walk-forward) | `1y` |
| `--samples` | Number of random samples | `100` |
| `--batch-size` | Bayesian batch size | `10` |
| `--max-iter` | Bayesian max iterations | `20` |
| `--workers` | Concurrent workers | `GOMAXPROCS` |
| `--top` | Number of top results to show | `10` |
| `--format` | Output format: `text`, `json` | `text` |

Parameter sweep ranges are specified via strategy parameter flags with range syntax. The exact syntax (e.g., `--lookback=3:24:1` for min:max:step) will be determined during implementation but must be unambiguous with single-value parameter flags.

## Stress Test Migration

The stress test study moves `Scenario` and `DefaultScenarios()` up to the `study` package. The `stress.StressTest` struct changes to accept `[]study.Scenario`. The `stress.Scenario` type is removed. No behavioral changes -- the stress test works exactly as before, just imports the scenario type from its parent package.

## Dependencies

No external dependencies for grid or random search. Bayesian optimization requires a Gaussian process implementation. Options:

1. Implement a minimal GP in-house (the parameter spaces are low-dimensional, typically <10 parameters)
2. Use an existing Go library if one meets quality standards

This decision is deferred to implementation. The `SearchStrategy` interface isolates the choice -- the Runner doesn't care what's behind `Next()`.

## Implementation Order

1. Extract scenarios from `study/stress/` into `study/`
2. Add `Period`, `Split`, and validation scheme functions to `study/`
3. Add `SearchStrategy` interface and Grid/Random implementations to `study/`
4. Update Runner to support `SearchStrategy` and `Splits`
5. Implement `study/optimize/` with `Analyze()` and report composition
6. Add CLI `optimize` subcommand
7. Implement Bayesian search strategy
8. Update stress test to use shared scenarios
