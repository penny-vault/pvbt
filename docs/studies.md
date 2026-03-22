# Studies

A study runs a strategy multiple times with different configurations and synthesizes the results into a single report. The study runner handles engine construction, concurrent execution, parameter sweeps, and progress reporting. Individual study types define what configurations to generate and how to interpret the combined results.

## Running a study

### From a compiled strategy binary

```
./adm study stress-test all
./adm study stress-test "2008 Financial Crisis" "COVID Crash"
```

### From pvbt

```
pvbt study stress-test --strategy=adm all
```

### Programmatically

```go
stressStudy := stress.New(nil) // nil = all default scenarios

runner := &study.Runner{
    Study:       stressStudy,
    NewStrategy: func() engine.Strategy { return &adm.Strategy{} },
    Options: []engine.Option{
        engine.WithDataProvider(provider),
        engine.WithAssetProvider(provider),
    },
    Workers: 8,
    Sweeps: []study.ParamSweep{
        study.SweepRange("lookback", 5.0, 20.0, 5.0),
        study.SweepPresets("Classic", "Modern"),
    },
}

progressCh, resultCh, err := runner.Run(ctx)
```

## The Runner

The `Runner` struct holds the study configuration and executes it.

| Field | Purpose |
|-------|---------|
| `Study` | The study type to run (implements the `Study` interface). |
| `NewStrategy` | Factory function returning a fresh strategy instance per run. Required because `Setup()` mutates strategy state. |
| `Options` | Base engine options shared by all runs (data providers, asset provider, etc.). |
| `Workers` | Number of concurrent engine runs. Defaults to 1 if not set. |
| `Sweeps` | Parameter sweeps to cross-product with the study's configurations. |

`Run(ctx)` returns two channels and an error. If `Configurations()` fails, the error is returned synchronously and both channels are nil. Otherwise, the progress channel receives a `Progress` value for each run state change (started, completed, failed), and the result channel receives a single `Result` when the study completes.

## Parameter sweeps

Sweeps vary strategy parameters across runs. Multiple sweeps are cross-producted with each other and with the study's base configurations.

```go
// Sweep a numeric range (works with int, float32, float64, etc.)
study.SweepRange("lookback", 1.0, 30.0, 0.5)

// Sweep a duration range
study.SweepDuration("rebalancePeriod", 24*time.Hour, 720*time.Hour, 24*time.Hour)

// Sweep explicit values (any parameter type the engine can parse from strings)
study.SweepValues("universe", "SPY,TLT", "QQQ,SHY", "VTI,BND")

// Sweep named parameter presets (from strategy suggest tags)
study.SweepPresets("Classic", "Modern", "Aggressive")
```

When sweeps are combined, the total run count is the product of all sweep dimensions multiplied by the study's base configuration count. For example, a stress test (1 base config) with a 4-value lookback sweep and 2 presets produces 1 x 4 x 2 = 8 runs.

Preset values are applied before field-level sweeps, so explicit field sweeps take precedence. If both the study's `RunConfig.Preset` and a `SweepPresets` are present, the sweep preset overrides the study's preset.

## Writing a study type

A study type implements the `Study` interface:

```go
type Study interface {
    Name() string
    Description() string
    Configurations(ctx context.Context) ([]RunConfig, error)
    Analyze(results []RunResult) (report.Report, error)
}
```

`Configurations()` returns the base set of run configs. These are cross-producted with any parameter sweeps on the Runner before execution. Each `RunConfig` specifies the backtest date range, initial deposit, preset, parameter overrides, and metadata.

`Analyze()` receives all run results (including failed runs where `Err` is non-nil) and composes a `report.Report` from report primitives (Table, TimeSeries, MetricPairs, Text sections). Each study type decides how to handle failed runs.

## Stress test study

The stress test runs a strategy against named historical market scenarios and analyzes how it behaves under the worst conditions.

### Default scenarios

| Scenario | Period |
|----------|--------|
| 1973-74 Oil Embargo Bear Market | Jan 1973 -- Oct 1974 |
| Volcker Tightening | Jan 1980 -- Aug 1982 |
| 1987 Black Monday | Oct 1987 -- Dec 1987 |
| 1994 Bond Massacre | Feb 1994 -- Nov 1994 |
| 1998 LTCM / Russian Crisis | Aug 1998 -- Oct 1998 |
| Dot-com Bubble | Jan 1998 -- Mar 2000 |
| Dot-com Bust | Mar 2000 -- Oct 2002 |
| 9/11 | Sep 2001 -- Oct 2001 |
| 2008 Financial Crisis | Sep 2008 -- Mar 2009 |
| 2010 Flash Crash | May 2010 -- Jun 2010 |
| Euro Debt Crisis | Apr 2010 -- Oct 2011 |
| 2011 Debt Ceiling Crisis | Jul 2011 -- Oct 2011 |
| 2015-2017 Low-Volatility Grind | Jan 2015 -- Dec 2017 |
| 2018 Q4 Selloff | Oct 2018 -- Dec 2018 |
| COVID Crash | Feb 2020 -- Mar 2020 |
| 2022 Rate Hiking Cycle | Jan 2022 -- Oct 2022 |
| 2023 Regional Banking Crisis | Mar 2023 -- May 2023 |

The stress test runs the engine once over the full date range spanning all scenarios, then slices the equity curve into scenario windows for analysis. This avoids redundant engine runs when only the analysis window varies.

### Per-scenario metrics

- Maximum drawdown
- Drawdown velocity (percentage per trading day)
- Total strategy return
- Worst single-day return
- Strategy return vs benchmark return

### Custom scenarios

```go
scenarios := []stress.Scenario{
    {
        Name:        "My Scenario",
        Description: "Custom stress period",
        Start:       time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC),
        End:         time.Date(2018, 12, 31, 0, 0, 0, 0, time.UTC),
    },
}
stressStudy := stress.New(scenarios)
```

## Future study types

The following study types are planned to build on this framework:

- Capacity analysis -- vary initial deposit to find strategy capacity limits
- Tax analysis -- compare tax-efficient vs standard execution
- Factor analysis -- decompose returns into factor exposures
- Regime analysis -- identify market regimes and per-regime behavior
- Walk-forward validation -- rolling in-sample/out-of-sample testing
- Parameter optimization -- systematic parameter search
- Monte Carlo simulation -- randomized return sequences
