# Factor Analysis Design

GitHub issue: #33

## Goal

Decompose portfolio returns into exposures to known market factors via OLS regression. Reveals whether a strategy generates genuine alpha or just loads on well-known risk premia like value, momentum, or size.

## Factor data

Factor return series (SMB, HML, MktRF, etc.) live in the pv-data database and are fetched through the existing `PVDataProvider` / `BatchProvider.Fetch()`. No new provider is needed.

Factors use a new sentinel asset:

```go
var Factor = Asset{Ticker: "$FACTOR", CompositeFigi: "$FACTOR"}
```

Each factor name is a metric. Fetching factors looks like any other data request:

```go
req := data.DataRequest{
    Assets:  []asset.Asset{asset.Factor},
    Metrics: []data.Metric{data.Metric("SMB"), data.Metric("HML"), data.Metric("MktRF")},
    Start:   start,
    End:     end,
}
factorDF, err := provider.Fetch(ctx, req)
```

This follows the same pattern as economic indicators (`asset.EconomicIndicator` with indicator names as metrics), but with a distinct sentinel so factors and indicators don't collide.

## API

Two methods on the `Portfolio` interface, implemented on `Account`:

```go
FactorAnalysis(factors *data.DataFrame) (FactorRegression, error)
StepwiseFactorAnalysis(factors *data.DataFrame) (StepwiseResult, error)
```

`FactorAnalysis` regresses portfolio excess returns against every factor in the DataFrame. Use this when you want a specific model (e.g., Fama-French 3) applied consistently across strategies for comparability.

`StepwiseFactorAnalysis` uses forward stepwise AIC selection to find the best factor subset from the candidates. Use this when you have a large pool of factors and want the algorithm to determine which ones matter.

### Input

A `*data.DataFrame` with `asset.Factor` as the asset and one metric per factor (each metric column is a factor return series).

The time axis must overlap with the portfolio's time axis. The method intersects the two time axes and regresses only over the overlapping dates. Returns an error if the overlap is too short for a meaningful regression (fewer than 12 observations).

### Output

```go
type FactorRegression struct {
    Alpha    float64            // intercept: return not explained by factors
    RSquared float64            // fraction of variance explained by the model
    AIC      float64            // Akaike Information Criterion
    Betas    map[string]float64 // factor metric name -> coefficient
}

type StepwiseResult struct {
    Best  FactorRegression   // the selected model
    Steps []FactorRegression // one per round of selection
}
```

`Betas` is keyed by metric name from the input DataFrame (e.g., "SMB", "HML", "MktRF").

### Stepwise algorithm

Following the approach described by Quantpedia:

1. Try each factor individually, pick the one with the lowest AIC
2. Try adding each remaining factor to the current model, keep it if AIC improves
3. Stop when no additional factor improves the model

Each round is recorded in `Steps`.

## Regression

OLS regression of portfolio excess returns (returns minus risk-free rate) against the factor return series. The portfolio's excess returns come from `PortfolioStats.ExcessReturns()`. Factor returns come from the input DataFrame, aligned on dates.

Uses `gonum/mat` for the matrix math (the project already depends on gonum).

## What this is not

- **Not a study.** Studies orchestrate multiple backtest runs (stress tests, optimization, Monte Carlo). Factor analysis computes on a single portfolio's returns. It is a method on Portfolio.
- **Not a PerformanceMetric.** The `PerformanceMetric` interface returns `(float64, error)` from `Compute` and `(*DataFrame, error)` from `ComputeSeries`. Factor analysis returns a structured result with multiple values. Forcing it into a DataFrame would mean cramming betas, alpha, R-squared, and AIC into a layout that doesn't naturally represent them.
- **Not a new package.** The types and methods live in the `portfolio` package alongside the existing alpha, beta, and R-squared implementations.

## Files

- `asset/asset.go` -- add `Factor` sentinel
- `portfolio/factor_regression.go` -- types (`FactorRegression`, `StepwiseResult`) and the OLS regression implementation
- `portfolio/factor_regression_test.go` -- tests with known factor data and expected regression outputs
