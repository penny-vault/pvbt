# Monte Carlo Simulation Design

**Issue:** #6
**Date:** 2026-03-22
**Dependency:** #35 (Study runner framework)

## Problem

A single backtest produces a single outcome shaped by the specific sequence of historical events. Monte Carlo simulation answers whether the strategy is genuinely robust or whether the result depended on a lucky ordering of returns. By running the strategy against thousands of synthetic price series constructed from resampled historical data, we produce confidence intervals, probability of ruin, and a percentile rank for the historical result.

## Approach

The Monte Carlo study runs real engine backtests against synthetic data. A resampling data provider wraps any existing provider and produces synthetic price series by resampling historical returns. The study implements the `Study` interface and uses a new `EngineCustomizer` optional interface to inject the resampling provider per run. Each simulation path is a full engine backtest -- the strategy logic is exercised on every path, not just the returns.

## Components

### 1. Resampler Interface

Lives in the `data` package. Defines the contract for resampling methods:

```go
type Resampler interface {
    Resample(returns [][]float64, targetLen int, rng *rand.Rand) [][]float64
}
```

Input is a 2D slice of historical returns (assets x time steps). Output is a synthetic 2D slice (assets x targetLen). All methods operate on returns, not prices, and synchronize cross-asset indices to preserve correlations.

Three implementations:

**Block Bootstrap** (default) -- Picks random start indices and copies contiguous blocks of `blockSize` returns across all assets simultaneously. Concatenates blocks until `targetLen` is reached, truncating the last block if needed. Preserves short-term autocorrelation and cross-asset correlations within blocks.

**Return-Level Bootstrap** -- For each time step in the output, picks a random historical time step and copies the returns for all assets at that step. Sampling with replacement. Preserves cross-asset correlations at each point but destroys temporal structure.

**Permutation** -- Randomly shuffles the time indices of the historical return series. All assets permuted with the same index mapping. Without replacement, so the marginal distribution is exactly preserved. Destroys temporal structure while keeping cross-asset correlations.

### 2. Resampling Data Provider

Lives in the `data` package. Implements the existing data provider interface by wrapping any concrete provider.

Construction parameters:
- Underlying data provider
- Resampler implementation
- Random seed

Behavior:
- On construction, receives the underlying provider, a `Resampler`, a random seed, and a pre-fetched historical `DataFrame` (shared across all paths -- see Section 5 for how this is managed)
- On data request, converts the cached historical prices to daily returns
- Passes returns through the configured `Resampler` to produce synthetic returns
- Reconstructs prices from synthetic returns
- Zeroes out dividends, splits, and all other corporate actions (these are meaningless in a resampled timeline)
- Each simulation path receives a different seed (base seed + path index) for reproducibility

### 3. EngineCustomizer Interface

Lives in the `study` package. An optional interface that studies can implement to customize per-run engine construction:

```go
type EngineCustomizer interface {
    EngineOptions(cfg RunConfig) []engine.Option
}
```

The runner checks for this via type assertion. If the study implements it, `EngineOptions(cfg)` is called and the returned options are appended to the base options before constructing the engine. The existing `Study` interface is unchanged. Studies that do not implement `EngineCustomizer` are unaffected.

### 4. Runner Changes

In `runSingle`, after copying base options and before constructing the engine, the runner checks:

```go
if customizer, ok := runner.Study.(EngineCustomizer); ok {
    opts = append(opts, customizer.EngineOptions(cfg)...)
}
```

This is the only change to the runner.

### 5. Monte Carlo Study

Lives in `study/montecarlo`. Implements `Study` and `EngineCustomizer`.

**Construction:** Receives the base data provider at construction time. Before generating configurations, the study pre-fetches the full historical dataset from the base provider once and caches it. This cached dataset is shared (read-only) across all simulation paths, avoiding 1000 redundant fetches of the same data.

**Configuration:**

| Parameter | Default | Description |
|---|---|---|
| Simulations | 1000 | Number of synthetic paths |
| Method | Block Bootstrap | Resampling method |
| BlockSize | 20 | Trading days per block (block bootstrap only) |
| Seed | 42 | Base random seed for reproducibility |
| RuinThreshold | -0.30 | Max drawdown level considered "ruin" |

**`Configurations()`** returns N `RunConfig` entries, one per simulation path. Each config is named "Path 1", "Path 2", etc. and carries `simulation_seed` in its metadata.

**`EngineOptions(cfg)`** reads the seed from the config's metadata and constructs a `ResamplingProvider` using the pre-cached historical data, the study's resampler, and the per-path seed. Returns a `WithDataProvider` option wrapping this provider.

**Historical result:** Optionally provided at construction. If present, it is included in the analysis for percentile ranking. It is not re-run -- just compared against the simulated paths.

**`Analyze(results)`** collects all successful run results and composes the report.

### 6. Report Output

`Analyze()` produces a report with these sections:

**Fan Chart** (TimeSeries) -- Equity curve percentile bands across all paths at each time step. Series: P5, P25, P50 (median), P75, P95. If the historical result is present, added as a 6th "Historical" series.

**Terminal Wealth Distribution** (Table) -- Percentile table of final portfolio values: P1, P5, P10, P25, P50, P75, P90, P95, P99. Includes mean, standard deviation, and historical result's percentile rank if available.

**Confidence Intervals on Key Metrics** (Table) -- For TWRR, max drawdown, and Sharpe ratio: P5, P25, P50, P75, P95 across all paths. Historical value and its percentile rank if available. Metrics extracted via portfolio's `Summary()` API.

**Probability of Ruin** (MetricPairs) -- Percentage of paths where max drawdown exceeded the ruin threshold. Median max drawdown across all paths.

**Historical Rank** (MetricPairs) -- Historical backtest's terminal value, TWRR, max drawdown, and Sharpe expressed as percentile rank among simulated paths. Skipped if no historical result provided.

**Summary Narrative** (Text) -- Brief interpretation, e.g., "85% of simulated paths were profitable. The historical result ranked in the 72nd percentile of terminal wealth, suggesting the strategy's performance was moderately above its expected range."
