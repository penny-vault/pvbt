# More Weighting Strategies

GitHub issue: #4

## Overview

Add four new portfolio weighting strategies to the `portfolio` package: inverse volatility, market-cap weighted, risk parity (iterative), and risk parity (fast approximation). These complement the existing EqualWeight and WeightedBySignal functions.

A prerequisite change moves the `DataSource` interface from `universe` to `data` and adds a source reference to `DataFrame`, enabling weighting functions to fetch additional data when needed.

## Prerequisite: DataFrame DataSource Reference

### Interface relocation

Move the `DataSource` interface from `universe/data_source.go` to the `data` package. The interface depends only on types already in `data` (`Period`, `Metric`, `*DataFrame`) plus `asset.Asset`, which `data` already imports. The `portfolio.Period` type alias resolves to `data.Period`, so the engine's existing `Fetch(ctx, []asset.Asset, portfolio.Period, []data.Metric)` signature already satisfies the interface without changes.

```go
// in data package
type DataSource interface {
    Fetch(ctx context.Context, assets []asset.Asset, lookback Period,
        metrics []Metric) (*DataFrame, error)
    FetchAt(ctx context.Context, assets []asset.Asset, t time.Time,
        metrics []Metric) (*DataFrame, error)
    CurrentDate() time.Time
}
```

### DataFrame changes

Add a `source DataSource` field to the `DataFrame` struct with:
- `Source() DataSource` accessor
- `SetSource(DataSource)` setter

The engine sets itself as the source when creating DataFrames via `Fetch` and `FetchAt`.

### Update dependent packages

Update `universe/data_source.go` to re-export or alias `data.DataSource` instead of defining its own interface. Update all references in `universe`, `signal`, and their tests that use the old `universe.DataSource` type.

## Prerequisite: DataFrame Correlation Method

Add a `Correlation` method to `data.DataFrame` alongside the existing `Covariance` and `Std` methods. The method computes Pearson correlation between asset pairs for cross-asset correlation, or between metric pairs for single-asset cross-metric correlation. This follows the same pattern as `Covariance` but normalizes by the product of standard deviations.

```go
func (df *DataFrame) Correlation(assets ...asset.Asset) *DataFrame
```

This makes the DataFrame's statistical toolkit complete (mean, std, covariance, correlation) and keeps the weighting functions from reimplementing correlation math.

## Weighting Functions

All functions live in the `portfolio` package. All require the Selected metric column and return `(PortfolioPlan, error)`. The new functions accept `context.Context` because they may need to fetch data; the existing EqualWeight and WeightedBySignal do not need context because they only read from the DataFrame already provided.

All weights are constrained to be non-negative.

### Lookback parameter convention

The `lookback` parameter is a `data.Period`. Passing a zero-value `data.Period{}` means "use the default" (60 calendar days via `data.Days(60)`). Calendar days are used because `DataSource.Fetch` operates in calendar days; 60 calendar days provides roughly 42 trading days of observations, which is sufficient for stable volatility/covariance estimation.

### Data access pattern

At each timestep in the DataFrame, the weighting function needs a trailing window of price history. The approach:

1. At the start, call `df.Source().Fetch(ctx, selectedAssets, lookback, []data.Metric{data.AdjClose})` to get a DataFrame with sufficient history.
2. For each timestep, extract the trailing window from the fetched DataFrame using existing methods (`Between`, `ValueAt`).
3. The fetch happens once per call to the weighting function, not per timestep. The returned DataFrame covers the full range needed.

If the DataFrame already contains enough history (its time range extends at least `lookback` before the first allocation date), no fetch is needed. A nil source is only an error when a fetch is actually required.

### InverseVolatility

```go
func InverseVolatility(ctx context.Context, df *data.DataFrame, lookback data.Period) (PortfolioPlan, error)
```

- Computes trailing standard deviation of daily returns (from AdjClose) for each selected asset over the lookback window.
- Weights each asset inversely proportional to its volatility: `w_i = (1/sigma_i) / sum(1/sigma_j)`.
- Falls back to equal weight if all selected assets have zero or NaN volatility.

### MarketCapWeighted

```go
func MarketCapWeighted(ctx context.Context, df *data.DataFrame) (PortfolioPlan, error)
```

- Weights selected assets proportionally to their MarketCap metric values at each timestep.
- If MarketCap is not present in the DataFrame, fetches it via `df.Source().FetchAt(ctx, selectedAssets, df.Source().CurrentDate(), []data.Metric{data.MarketCap})`.
- Falls back to equal weight if all MarketCap values are zero or NaN.

### RiskParityFast

```go
func RiskParityFast(ctx context.Context, df *data.DataFrame, lookback data.Period) (PortfolioPlan, error)
```

Approximation of equal risk contribution using the "naive risk parity" formula:

1. Compute the covariance matrix `C` and volatilities `sigma_i` over the lookback window.
2. Start with inverse volatility weights: `w_i = 1/sigma_i`.
3. Adjust for correlations: `w_i = w_i / (C @ w)_i`, where `(C @ w)_i` is asset i's marginal risk contribution.
4. Normalize: `w_i = w_i / sum(w_j)`.

This is a single-pass adjustment, not iterative. It produces better risk balance than pure inverse volatility when assets are correlated, but does not guarantee exact equal risk contribution.

Same data requirements as InverseVolatility. Falls back to equal weight when math degenerates.

### RiskParity

```go
func RiskParity(ctx context.Context, df *data.DataFrame, lookback data.Period) (PortfolioPlan, error)
```

Full iterative optimization using multiplicative fixed-point iteration:

1. Compute the covariance matrix `C` over the lookback window using DataFrame's `Covariance` method.
2. Initialize weights to `1/N` (equal weight).
3. At each iteration, compute each asset's risk contribution: `rc_i = w_i * (C @ w)_i / (w^T @ C @ w)`.
4. Update weights: `w_i = w_i * (target_rc / rc_i)^step_size`, then normalize to simplex.
5. Converged when `max(|rc_i - 1/N|) < 1e-10`.
6. Maximum 1000 iterations. Returns best result found after hitting the cap and logs a warning via zerolog if not converged.

All weights are constrained to be non-negative.

Same data requirements as InverseVolatility. Falls back to equal weight when math degenerates (e.g., singular covariance matrix).

### Insufficient history

If the lookback extends before the start of available market data (e.g., early in a backtest), the fetch returns whatever data is available. The weighting function uses what it gets. If fewer than 2 observations are available for an asset, its volatility is treated as NaN and the standard fallback logic applies.

## Error Handling

| Condition | Behavior |
|---|---|
| Missing Selected column | Return error |
| nil source when fetch is needed | Return error: `"<FuncName>: DataFrame has no DataSource; cannot fetch <data>"` |
| nil source when fetch is not needed | No error; proceed normally |
| Fetch call fails | Propagate the error |
| All selected assets have zero/NaN volatility or MarketCap | Fall back to equal weight |
| Singular covariance matrix | Fall back to equal weight |
| Fewer than 2 price observations for an asset | Treat volatility as NaN (standard fallback applies) |
| RiskParity non-convergence | Return best result found, log warning via zerolog |

No silent failures.

## Testing

Tests go in `portfolio/weighting_test.go` alongside existing EqualWeight and WeightedBySignal tests, using Ginkgo/Gomega. A mock `data.DataSource` is used for tests that verify fetch behavior.

### Coverage for all four functions

- Basic weight computation (correct proportions)
- Weight normalization (sums to 1.0)
- Single asset (gets 100%)
- NaN/zero handling in input data
- Missing Selected column (returns error)
- Fallback to equal weight when math degenerates
- Data fetching via source when data is missing (mock DataSource)
- nil source with sufficient data in DataFrame (no error)
- Per-timestep independence (weights change as inputs change)

### RiskParity-specific

- Convergence on a simple 2-asset case with analytically known answer
- Non-convergence behavior (returns best result, logs warning)

### RiskParityFast-specific

- Produces different weights than plain inverse volatility when assets are correlated
- Single-pass result is close to (but not identical to) iterative RiskParity result
