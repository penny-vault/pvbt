# Multi-Factor Regression Design

## Overview

Add multi-factor regression analysis to the portfolio package, following the stepwise forward selection approach described by [Quantpedia](https://quantpedia.com/a-robust-approach-to-multi-factor-analysis/). The analysis regresses portfolio excess returns against a set of factor return series and selects the best model using AIC.

This is not a single PerformanceMetric -- it is a structured analysis that produces multiple outputs. It lives in the `portfolio` package alongside the existing metric infrastructure, using the same helper patterns (raw `[]float64` slices, gonum for matrix math).

## Core Insight

A factor is a long-short strategy. SMB goes long small caps and shorts large caps. HML goes long value stocks and shorts growth stocks. Factor construction is just strategy execution -- signals, selectors, weighting, and the engine already handle all of this.

The only new code needed is the regression math. Everything else composes existing primitives.

## Types

### Factor

A named return series aligned to timestamps. This is what the regression functions consume.

```go
type Factor struct {
    Name    string
    Returns []float64
}
```

A Factor can come from two sources:
1. **Pre-computed**: downloaded from the Kenneth French Data Library or similar
2. **Strategy-derived**: run a factor strategy through the engine, extract returns from the resulting Account

### FactorRegression

The output of a regression.

```go
type FactorRegression struct {
    Alpha     float64            // annualized intercept
    Betas     map[string]float64 // factor name -> loading
    RSquared  float64
    AdjRSq    float64
    AIC       float64
    Residuals []float64
    StdErrors map[string]float64 // coefficient name -> standard error
    TStats    map[string]float64 // coefficient name -> t-statistic
    Factors   []string           // ordered factor names in the model
}
```

### StepResult

Records one step of the forward selection process.

```go
type StepResult struct {
    FactorAdded string
    AIC         float64
    RSquared    float64
}
```

## Functions

### OLS

Run ordinary least squares with all provided factors.

```go
func ols(portfolioReturns []float64, factors []Factor) FactorRegression
```

Uses `gonum.org/v1/gonum/mat` for (X'X)^-1 X'y. Unexported -- the public interface is on Account.

### Stepwise

AIC-based forward stepwise selection (the Quantpedia approach).

```go
func stepwise(portfolioReturns []float64, candidates []Factor) (FactorRegression, []StepResult)
```

Algorithm:
1. Start with no factors in the model
2. For each candidate factor, fit a univariate regression, compute AIC = n * ln(RSS/n) + 2k
3. Select the factor with the lowest AIC
4. For each remaining candidate, add it to the model, refit, compute AIC
5. If the best AIC improves over the current model, keep the factor and repeat
6. Stop when no factor improves AIC

### Rolling

Rolling-window regression for time-varying factor exposures.

```go
func rollingOLS(portfolioReturns []float64, factors []Factor, windowSize int) []FactorRegression
```

## Account Methods

```go
// FactorAnalysis runs OLS regression of portfolio excess returns against
// the provided factor return series.
func (a *Account) FactorAnalysis(factors ...Factor) FactorRegression

// StepwiseFactorAnalysis runs AIC-based forward selection across the
// provided candidate factors.
func (a *Account) StepwiseFactorAnalysis(candidates ...Factor) (FactorRegression, []StepResult)
```

These methods compute portfolio excess returns from the equity curve and risk-free prices, then delegate to the unexported regression helpers.

### FactorFromAccount

Converts a strategy's equity curve into a Factor for use in regression.

```go
func FactorFromAccount(name string, a *Account) Factor
```

This bridges the gap between "factor as strategy" and "factor as regression input." Run any strategy through the engine, then use its returns as a factor.

## Factor Data

### Primary path: pre-computed (Kenneth French Data Library)

The standard academic factors (MktRF, SMB, HML, UMD, RMW, CMA) are published as daily and monthly return series by the Kenneth French Data Library. A `factor` package provides functions to load these:

```go
package factor

// FF3 loads Fama-French 3-factor returns (MktRF, SMB, HML) for the given date range.
func FF3(start, end time.Time) []portfolio.Factor

// FF5 loads Fama-French 5-factor returns (MktRF, SMB, HML, RMW, CMA).
func FF5(start, end time.Time) []portfolio.Factor

// Carhart loads Carhart 4-factor returns (MktRF, SMB, HML, UMD).
func Carhart(start, end time.Time) []portfolio.Factor
```

Implementation: fetch CSV from the French data library, parse, cache locally, align to requested date range. This is straightforward -- the CSVs have a stable format and are freely available.

### Custom factors: strategies

Any factor not covered by published data can be constructed as a strategy. A factor strategy is no different from any other strategy -- it uses signals, selectors, and weighting to form a portfolio, and the engine computes its returns.

```go
// Run a custom factor strategy
factorAccount, _ := engine.Run(ctx, factorAcct, start, end)

// Use its returns in a regression
customFactor := portfolio.FactorFromAccount("LowVol", factorAccount)
result := p.FactorAnalysis(customFactor, mktrf, smb, hml)
```

This approach requires no new abstractions. Everything composes from existing primitives: signals compute ranking scores, selectors filter assets, weighting assigns positions, the engine runs it, and the Account holds the equity curve.

## File Layout

| File | Contents |
|------|----------|
| `portfolio/regression.go` | `ols`, `stepwise`, `rollingOLS` helpers (unexported) |
| `portfolio/factor_analysis.go` | `Factor`, `FactorRegression`, `StepResult` types; `FactorFromAccount`; Account methods |
| `factor/french.go` | Kenneth French Data Library loader (FF3, FF5, Carhart) |

## What This Is Not

- Not a new PerformanceMetric. The output is structured, not a single float64.
- Not tied to any specific factor model. Any `[]Factor` works.
- Not a new engine feature. Factor strategies are just strategies.
- Not a new type system. `FactorDef`, `Quantile`, `SignalFunc` are all unnecessary -- the existing strategy primitives already express factor construction.

## Future Extensions

- **PerformanceMetric bridge**: Thin wrapper metrics that extract individual values (e.g., factor alpha, specific beta) from a cached FactorRegression. Add only if needed.
- **Robust regression**: M-estimators or other approaches that down-weight outliers, as an alternative to OLS.
- **Value-weighted baskets**: Standard Fama-French uses value-weighted portfolios for factor construction. Strategy-based factors can implement this with `WeightedBySignal`.
