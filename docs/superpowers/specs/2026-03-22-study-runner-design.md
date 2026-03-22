# Study Runner Framework and Stress Test Study

## Overview

An orchestration layer that sits above the engine, runs it multiple times with different configurations, and synthesizes the results into a coherent report. Parameter sweeps are cross-producted with study configurations to generate the run matrix. The first concrete study type -- stress testing -- validates the framework.

This design also refactors the report package from a monolithic single-portfolio struct into a compositional system of renderable primitives, enabling both the existing summary report and new study-specific reports to share the same rendering infrastructure.

## Package: `study`

### RunConfig

Fully specifies what the engine should do for a single run.

```go
type RunConfig struct {
    Name     string            // human-readable label, e.g. "2008 Financial Crisis"
    Start    time.Time         // backtest start date
    End      time.Time         // backtest end date
    Deposit  float64           // initial deposit (0 = use base engine option)
    Preset   string            // named parameter preset, resolved before Params
    Params   map[string]string // explicit parameter overrides (take precedence over preset)
    Metadata map[string]string // arbitrary tags stored on the portfolio
}
```

When `Deposit` is zero, the runner uses whatever deposit is configured in the base engine options. If neither specifies a deposit, the engine's existing default behavior applies (which requires `WithInitialDeposit` or `WithPortfolioSnapshot`). The runner does not invent a default.

### RunResult

Pairs a config with its outcome.

```go
type RunResult struct {
    Config    RunConfig
    Portfolio report.ReportablePortfolio
    Err       error
}
```

`ReportablePortfolio` (the composition of `portfolio.Portfolio` and `portfolio.PortfolioStats`) is used instead of bare `portfolio.Portfolio` because `Analyze()` needs access to performance metrics. The declared return type of `engine.Backtest()` is `portfolio.Portfolio`, but the concrete `*Account` returned satisfies `ReportablePortfolio`. The runner performs a type assertion after each `Backtest()` call and treats assertion failure as a run error.

### Study Interface

Each study type implements this interface. It knows what runs to generate and how to interpret the collected results.

```go
type Study interface {
    Name() string
    Description() string
    Configurations(ctx context.Context) ([]RunConfig, error)
    Analyze(results []RunResult) (report.Report, error)
}
```

- `Configurations()` returns the base set of run configs. The study receives all its inputs at construction time (e.g., scenario list, date ranges). These configs are cross-producted with any parameter sweeps on the Runner to produce the final run matrix.
- `Analyze()` takes all collected run results (including failures, where `RunResult.Err` is non-nil) and composes a `report.Report` from report primitives. Each study type decides how to handle failed runs (skip, report as missing, or error).

### Progress

Sent on a channel as runs execute, enabling CLI progress display.

```go
type RunStatus int

const (
    RunStarted   RunStatus = iota
    RunCompleted
    RunFailed
)

type Progress struct {
    RunName   string
    RunIndex  int
    TotalRuns int
    Status    RunStatus
    Err       error
}
```

### Result

Sent on a channel when the study completes.

```go
type Result struct {
    Runs   []RunResult
    Report report.Report
    Err    error
}
```

`Result.Err` is set when `Analyze()` fails. Individual run failures are recorded in each `RunResult.Err`. Both can be present simultaneously. If `Configurations()` fails, `Run()` returns nil channels and the error synchronously -- no goroutines are started.

### Parameter Sweeps

Parameter sweeps describe how to vary strategy parameters across runs. Multiple sweeps are cross-producted with each other and with the study's `Configurations()` output to produce the final run matrix.

```go
type ParamSweep struct { /* expanded values stored as strings internally */ }

func SweepRange[T constraints.Integer | constraints.Float](field string, min, max, step T) ParamSweep
func SweepDuration(field string, min, max, step time.Duration) ParamSweep
func SweepValues(field string, values ...string) ParamSweep
func SweepPresets(presets ...string) ParamSweep
```

- `SweepRange` generates values from min to max (inclusive) with the given step. Handles all integer and float types via generics.
- `SweepDuration` is separate because `time.Duration` does not satisfy integer/float constraints.
- `SweepValues` provides explicit string values for a field. Works for any parameter type the engine can parse from strings, including tickers and asset identifiers.
- `SweepPresets` varies named parameter presets. Presets set multiple parameters at once (from the strategy's `suggest` tags). `SweepPresets` produces a dimension in the cross-product matrix just like field sweeps. Preset values are applied before field-level sweeps, so explicit field sweeps take precedence. If both a study's `RunConfig.Preset` and a `SweepPresets` are present, the sweep preset overrides the study's preset.

All constructors expand their inputs into string values internally, since the engine's parameter application machinery uses strings as the wire format.

### Runner

The Runner holds study configuration and executes the study.

```go
type Runner struct {
    Study       Study
    NewStrategy func() engine.Strategy
    Options     []engine.Option
    Workers     int
    Sweeps      []ParamSweep
}

func (r *Runner) Run(ctx context.Context) (<-chan Progress, <-chan Result, error)
```

`NewStrategy` is a factory function that returns a fresh strategy instance for each run. This is necessary because `Setup()` mutates strategy state (stores universe references, provider handles, etc.), making it unsafe to share a single strategy value across concurrent workers.

**Execution flow:**

1. Calls `r.Study.Configurations(ctx)` to get the base run configs.
2. Cross-products base configs with `r.Sweeps` to produce the final run matrix. Each combination gets a descriptive `Name` (e.g., "2008 Financial Crisis / lookback=5.0 / Classic").
3. Fans configs out to a worker pool of `r.Workers` goroutines. Each worker:
   a. Calls `r.NewStrategy()` to get an independent strategy instance.
   b. Constructs an engine via `engine.New(strategy, r.Options...)` with config-specific options appended (start/end, deposit).
   c. Calls `engine.ApplyParams(eng, config.Preset, config.Params)` to resolve presets and apply parameter overrides. This happens after engine construction so that asset and universe fields can be resolved via the engine's asset provider.
   d. Runs `eng.Backtest(ctx, start, end)`.
   e. Type-asserts the returned `portfolio.Portfolio` to `report.ReportablePortfolio`.
4. Sends a `Progress` value on each run state change.
5. Collects all `RunResult` values. Individual run failures do not abort remaining runs.
6. Calls `r.Study.Analyze(results)` to produce the report.
7. Sends a single `Result` on the result channel.
8. Closes both channels.

**Preset and parameter application:** The runner resolves presets and applies parameter overrides using an exported `engine.ApplyParams(eng *Engine, preset string, params map[string]string) error` function. This function must be added to the engine package. It takes an `*Engine` (not just a strategy) because resolving `asset.Asset` and `universe.Universe` fields requires the engine's asset provider. It calls `DescribeStrategy()` to get the aggregated `Suggestions` map (preset name -> field name -> value), resolves the named preset (requiring the strategy to implement the `Descriptor` interface; if it does not, preset resolution is skipped and a non-empty preset string returns an error), merges explicit params on top, and sets the strategy's exported fields via reflection (wrapping the existing `applyParamValue` and `hydrateFields` logic).

**Concurrency model:** Each worker gets its own engine instance with independent strategy, broker, and cache. The `r.Options` slice is shared read-only (options are functions, not mutable state). Data providers referenced in options must be safe for concurrent use. If a provider is not thread-safe, the caller must wrap it or provide a factory option.

**Cancellation:** Via the context. Each engine run receives the same context, so cancelling it aborts all in-flight runs.

## Package: `report` (refactored)

The existing report package is refactored from a monolithic `Report` struct into a compositional system of renderable primitives. This is a breaking change to the report package's public API.

### Migration Path

1. Define the new `Section` interface, concrete section types, and the new `Report` struct alongside the existing `Report` struct (temporarily renamed during migration).
2. Implement `report.Summary()` as a builder that produces the new `Report` from a `ReportablePortfolio`, replacing the existing `Build()` function. The new `Summary()` composes the same content (header, equity curve, returns, risk, drawdowns, trades) as sections.
3. Update the `terminal` sub-package to accept the new `Report` and iterate its sections. The existing `terminal.Render()` is replaced with a new implementation that walks sections and renders each one using lipgloss styling.
4. Remove the old `Report` struct and `Build()` function.

This can be done incrementally: the new types coexist with the old during the transition.

### Format

```go
type Format string

const (
    FormatText Format = "text"
    FormatHTML Format = "html"
    FormatJSON Format = "json"
)
```

String-typed for natural CLI flag parsing and JSON serialization.

### Section Interface

A self-contained renderable unit. Each section knows how to render itself in every supported format.

```go
type Section interface {
    Type() string // discriminator for JSON serialization, e.g. "table", "time_series"
    Name() string
    Render(format Format, w io.Writer) error
}
```

### Concrete Section Types

**Table** -- rows and columns of data.

```go
type Table struct {
    SectionName string
    Columns     []Column
    Rows        [][]any
}

type Column struct {
    Header string
    Format string // "percent", "currency", "number", "string", "date"
    Align  string // "left", "right", "center"
}
```

Used for scenario comparison tables, annual returns, trade statistics, metric grids.

**TimeSeries** -- multiple named series over time.

```go
type TimeSeries struct {
    SectionName string
    Series      []NamedSeries
}

type NamedSeries struct {
    Name   string
    Times  []time.Time
    Values []float64
}
```

Used for equity curves, drawdown charts, benchmark comparisons. When rendered as JSON, suitable for charting libraries. When rendered as text, shows summary statistics.

**MetricPairs** -- labeled metric values, optionally paired.

```go
type MetricPairs struct {
    SectionName string
    Metrics     []MetricPair
}

type MetricPair struct {
    Label      string
    Value      float64
    Comparison *float64 // nil when no comparison value
    Format     string   // "percent", "ratio", "days"
}
```

Used for risk metrics, return summaries, strategy-vs-benchmark comparisons. `Comparison` is a pointer to distinguish "no comparison" from "comparison is 0.0."

**Text** -- narrative blocks or warnings.

```go
type Text struct {
    SectionName string
    Body        string
}
```

### Report

A titled collection of sections.

```go
type Report struct {
    Title    string
    Sections []Section
}

func (r Report) Render(format Format, w io.Writer) error
```

`Render` iterates sections and calls each one's `Render` method, adding appropriate framing (section separators in text mode, wrapping JSON object, HTML document structure).

For JSON output, each section emits a `type` discriminator field, enabling frontend frameworks (e.g., Vue) to map section types to components:

```json
{
  "title": "Stress Test: MyStrategy",
  "sections": [
    {"type": "table", "name": "Scenario Comparison", "columns": [...], "rows": [...]},
    {"type": "time_series", "name": "Equity Curves", "series": [...]},
    {"type": "metric_pairs", "name": "Worst Case", "metrics": [...]}
  ]
}
```

### Builders

Convenience functions that compose reports from domain data. Builders live wherever makes sense -- the report package for common cases, study packages for study-specific reports.

- `report.Summary(p ReportablePortfolio) (Report, error)` -- the existing single-portfolio report, refactored to compose from primitives. Produces sections for: header metrics (MetricPairs), equity curve (TimeSeries), recent and period returns (Table), annual returns (Table), risk metrics (MetricPairs), risk vs benchmark (MetricPairs), drawdowns (Table), monthly returns (Table), trade statistics (MetricPairs), and warnings (Text). This replaces the existing `Build()` function.
- Study-specific builders are called from within each study's `Analyze()` method.

## Package: `study/stress`

The first concrete study type. Runs a strategy against named historical market scenarios and analyzes how the strategy behaves under the worst conditions.

### Scenario

A named historical market episode.

```go
type Scenario struct {
    Name        string
    Description string
    Start       time.Time
    End         time.Time
}
```

### Pre-defined Scenarios

| Name | Period |
|------|--------|
| 2008 Financial Crisis | Sep 2008 -- Mar 2009 |
| COVID Crash | Feb 2020 -- Mar 2020 |
| 2022 Rate Hiking Cycle | Jan 2022 -- Oct 2022 |
| Dot-com Bust | Mar 2000 -- Oct 2002 |
| 2015--2017 Low-Volatility Grind | Jan 2015 -- Dec 2017 |
| 2011 Debt Ceiling Crisis | Jul 2011 -- Oct 2011 |

Users can define their own scenarios by constructing `Scenario` values directly.

### Construction

```go
func New(scenarios []Scenario) *StressTest
```

Passing an empty or nil slice defaults to all pre-defined scenarios. Individual scenarios can be selected by name via the CLI.

### Configurations

`Configurations()` returns a single `RunConfig` spanning the full date range (earliest scenario start to latest scenario end). The engine handles warmup internally based on the strategy's declared warmup period and the `DateRangeMode` option. When sweeps are provided on the Runner, this single config gets cross-producted with the sweep matrix -- one run per parameter set.

The stress test does not need separate engine runs per scenario. A single run covers all scenarios; `Analyze()` slices the equity curve into scenario windows.

### Analysis

`Analyze()` slices each portfolio's equity curve into scenario windows using `DataFrame.Between(start, end)` to extract the absolute date range for each scenario. Performance metrics (Sharpe, max drawdown, etc.) are computed over the sliced data. The existing `PerformanceMetricQuery.Window()` API uses relative lookback periods and is not suitable for absolute date ranges; the stress test must either add an absolute-date-range query path to the metric system or compute metrics directly from the sliced DataFrames.

**Per-scenario metrics:**
- Maximum drawdown
- Drawdown velocity: max drawdown divided by the number of trading days from peak to trough, expressed as percentage per day
- Recovery time: trading days from trough until the portfolio exceeds its pre-drawdown peak. Tracks forward beyond the scenario window until recovery occurs or the run ends. Reported as "not recovered" if the portfolio never recovers within the run.
- Worst single day and worst single week return
- Turnover during the scenario: computed from transactions within the scenario window (sum of absolute trade values divided by average portfolio value over the window). This is a direct computation from transaction data, not a pre-existing windowed metric.
- Strategy return vs benchmark return (relative performance). Benchmark comes from the portfolio's metadata (`strategy.benchmark` key), which the engine sets during execution.
- Strategy Sharpe ratio during the period

**Report composition:**
- A `Table` section ranking scenarios by severity (max drawdown)
- A `TimeSeries` section with overlaid equity curves per scenario (normalized to starting value)
- A `MetricPairs` section for each scenario showing strategy vs benchmark metrics
- A `Text` section with summary narrative

When multiple parameter sets are present (from sweeps), the table includes a column per parameter set so performance across configurations can be compared within each scenario.

Failed runs are included in the table with their error message and blank metrics.

## CLI Integration

### Compiled strategy binary

```
./adm study stress-test [scenario-names...|all]
```

The binary IS the strategy. No `--strategy` flag. Scenarios are positional args. `all` runs every pre-defined scenario.

### pvbt runner

```
pvbt study stress-test --strategy=adm [scenario-names...|all]
```

Strategy is looked up by shortcode.

### Progress display

The CLI reads the `Progress` channel and drives a Bubble Tea progress display showing run status, completion count, and which scenarios are in-flight. When the study completes, it reads the `Result` channel and calls `result.Report.Render(format, os.Stdout)` for output.

## Implementation Order

1. Report primitives and new `Report` type
2. `report.Summary()` builder (replaces `Build()`)
3. Update `terminal` sub-package to use new `Report`
4. `engine.ApplyParams()` export
5. `study` package (Runner, types, sweeps, parameter cross-product)
6. `study/stress` package
7. CLI `study` subcommand

## Studies That Build on This Framework

These are separate issues that will implement additional `Study` types using this framework:

- Capacity analysis (#37)
- Tax analysis (#38)
- Factor analysis (#39)
- Regime analysis (#34)
- Walk-forward validation (#5)
- Parameter optimization (#22)
- Monte Carlo simulation (#6)
