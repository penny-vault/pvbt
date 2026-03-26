# Mean Reversion Signals Design

## Summary

Add five mean reversion signals to the `signal/` package: Z-Score, Hurst exponent (R/S), Hurst exponent (DFA), pairs residual, and pairs ratio. These signals help identify stretched conditions where prices are likely to revert to typical behavior. Addresses issue #27.

## Signals

### Z-Score

**Function:** `signal.ZScore(ctx, universe, period, metrics ...data.Metric) *data.DataFrame`

Computes the z-score of each asset's current value relative to its rolling mean and standard deviation over the lookback period. Default metric is `Close`.

**Computation:**

1. Fetch window of data via `universe.Window(ctx, period, metric)`
2. For each asset: `z = (current - mean) / stddev`
3. Return single-row DataFrame with metric `ZScore`

**Output interpretation:** Positive values mean the asset is above its rolling mean; negative values mean it is below. Values beyond +/-2 indicate significant deviation.

**Error cases:** Insufficient data (< 2 points). Zero standard deviation (constant price series) returns an error rather than NaN/Inf.

### Hurst Exponent (R/S)

**Function:** `signal.HurstRS(ctx, universe, period) *data.DataFrame`

Rescaled Range (R/S) analysis of the Hurst exponent.

**Computation:**

1. Fetch close prices over the lookback period
2. Compute returns series
3. For multiple sub-period sizes `n`, compute the R/S statistic: `R(n)/S(n)` where R is the range of cumulative deviations from the mean and S is the standard deviation
4. Regress `log(R/S)` on `log(n)` -- the slope is the Hurst exponent
5. Return single-row DataFrame with metric `HurstRS`

**Output interpretation:** H < 0.5 = mean-reverting, H = 0.5 = random walk, H > 0.5 = trending. Values typically range 0 to 1.

**Error cases:** Insufficient data to form enough sub-periods for a meaningful regression (need 20+ data points).

### Hurst Exponent (DFA)

**Function:** `signal.HurstDFA(ctx, universe, period) *data.DataFrame`

Detrended Fluctuation Analysis of the Hurst exponent.

**Computation:**

1. Fetch close prices, compute returns, build the cumulative deviation (profile) series
2. For multiple window sizes `n`, divide the profile into non-overlapping segments
3. In each segment, fit a linear trend and compute the RMS of residuals
4. The fluctuation function `F(n)` is the average RMS across segments
5. Regress `log(F(n))` on `log(n)` -- the slope is the DFA exponent (equivalent to Hurst)
6. Return single-row DataFrame with metric `HurstDFA`

**Output interpretation:** Same as R/S -- H < 0.5 = mean-reverting, H = 0.5 = random walk, H > 0.5 = trending.

**Error cases:** Insufficient data to form enough sub-periods for a meaningful regression (need 20+ data points).

### Pairs Residual

**Function:** `signal.PairsResidual(ctx, universe, period, referenceUniverse) *data.DataFrame`

OLS regression residual z-score for pairs trading. The reference universe is a collection of one or more assets to compute pairs against. Each reference asset produces a separate output metric.

**Computation:**

1. Fetch close prices for both the primary universe and reference universe over the lookback period
2. For each (asset, reference) pair, regress asset returns on reference returns via OLS to get slope (beta) and intercept (alpha)
3. Compute the residual: `residual = asset_return - (alpha + beta * reference_return)` for each bar
4. Take the z-score of the residual series; the last value is the output
5. Output metric is `PairsResidual_{ticker}` where `{ticker}` is the reference asset's ticker

**Output interpretation:** Positive = asset is rich relative to the reference. Negative = asset is cheap relative to the reference. Magnitude indicates standard deviations from the mean relationship.

**Output shape:** If the reference universe has N assets, the output DataFrame has N metrics per primary asset.

**Error cases:** Insufficient data, reference asset not found in fetched data, zero variance in reference series.

### Pairs Ratio

**Function:** `signal.PairsRatio(ctx, universe, period, referenceUniverse) *data.DataFrame`

Price ratio z-score for pairs trading. Same reference universe approach as PairsResidual.

**Computation:**

1. Fetch close prices for both universes over the lookback period
2. Compute the rolling ratio: `ratio = asset_price / reference_price`
3. Take the z-score of the ratio series; the last value is the output
4. Output metric is `PairsRatio_{ticker}` where `{ticker}` is the reference asset's ticker

**Output interpretation:** Same as PairsResidual -- positive = rich, negative = cheap relative to reference.

**Output shape:** Same as PairsResidual -- N metrics per primary asset for N reference assets.

**Error cases:** Insufficient data, zero-price reference values.

## Architecture

All five signals follow the existing function-based signal pattern. No new interfaces, registries, or abstractions are introduced.

- Z-Score and Hurst variants are standard single-universe signals matching the existing calling convention
- Pairs signals add a second `universe.Universe` parameter for the reference assets, which is the only deviation from the standard signature
- Shared math (z-score computation used by Z-Score, PairsResidual, and PairsRatio) is extracted into an unexported helper function

## Files

| File | Purpose |
|------|---------|
| `signal/zscore.go` | Z-Score signal |
| `signal/zscore_test.go` | Z-Score tests |
| `signal/hurst_rs.go` | Hurst R/S signal |
| `signal/hurst_rs_test.go` | Hurst R/S tests |
| `signal/hurst_dfa.go` | Hurst DFA signal |
| `signal/hurst_dfa_test.go` | Hurst DFA tests |
| `signal/pairs_residual.go` | Pairs residual signal |
| `signal/pairs_residual_test.go` | Pairs residual tests |
| `signal/pairs_ratio.go` | Pairs ratio signal |
| `signal/pairs_ratio_test.go` | Pairs ratio tests |

Existing files updated:
- `signal/doc.go` -- add new signals to the catalog
- Changelog and reference docs

## Testing Strategy

Each signal gets a dedicated test file using Ginkgo/Gomega following established patterns:

- **Correctness:** Hand-calculated values against known inputs
- **Multi-asset independence:** Results for one asset do not change when another is added
- **Edge cases:** Minimum viable data length, constant price series, all-positive/all-negative returns
- **Error propagation:** Insufficient data and fetch errors via `errorDataSource`
- **Hurst-specific:** Synthetic trending series should produce H > 0.5; synthetic mean-reverting series should produce H < 0.5
- **Pairs-specific:** Multi-reference output shape validation (correct number of metrics, correct metric naming convention)
