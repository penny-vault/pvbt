# Portfolio Package Test Quality Overhaul

## Problem

The portfolio package test suite has several quality issues:

1. Many tests are meaningless (Name() tests, "returns nil" tests, directional-only assertions)
2. metric_helpers_test.go uses white-box testing (package portfolio) while everything else is black-box
3. buildDF helper is duplicated across 9 test files
4. Missing edge cases across Account operations, metrics, selectors, and weightings
5. Several 0% coverage public functions (Summary, RiskMetrics, WithdrawalMetrics, Window, Days, Years, ComputeSeries for several metrics)
6. mockBroker silently returns defaults, fragile map iteration assertions, phantom transactions to manipulate equity curves

## Design

### 1. Remove Meaningless Tests

Delete all:
- `It("returns a name")` tests for every metric (they test one-line string literal returns)
- `It("returns nil for ComputeSeries")` tests (they test one-line `return nil`)
- Directional-only assertions (`> 0`, `< 0`, `>= 0`, `between X and Y`) -- replace with assertions against hand-calculated expected values

### 2. Convert to All Black-Box Testing

Delete metric_helpers_test.go entirely. Test helper edge cases through the public metrics that call them:
- Empty returns -> test a metric with a 1-element equity curve
- Division by zero in returns() -> test a metric with zero-price data point
- mean/variance/covariance with small inputs -> test metrics with 2-3 data points and verify exact values
- windowSlice with Day/Year units -> test metrics with Window(Days(n)) and Window(Years(n))
- annualizationFactor branches -> test metrics with daily-spaced and monthly-spaced timestamps

### 3. Fix Test Infrastructure

- Create `testutil_test.go` in package portfolio_test with shared buildDF helper and common test fixtures
- Fix mockBroker to panic or Fail() when unconfigured fills are exhausted
- Fix rebalance_test.go to not depend on map iteration order (sort submitted orders before asserting)
- Stop using phantom DividendTransaction/FeeTransaction to manipulate equity curves; instead build equity curves through actual buy/sell sequences or direct Account construction
- Add compile-time interface checks: `var _ Selector = ...`, `var _ PerformanceMetric = ...`

### 4. Add Missing Edge Cases

**Account/Core:**
- Order with broker error
- RebalanceTo with NaN/zero price, empty Members, multiple allocations
- Holdings iteration with actual holdings
- PositionValue with nil prices
- Value() with NaN price for held asset
- Record for full position depletion
- Record with multiple tax lots (FIFO partial consumption)
- TransactionType.String() unknown type
- WithCash zero/negative

**Metrics (all through public API with hand-calculated expected values):**
- TWRR with declining curve, with window
- MWRR with mid-stream cash flows, zero-date deposits, convergence edge cases
- Sharpe/Sortino/Calmar with zero denominator conditions
- ComputeSeries for Calmar, Sharpe, Sortino, DownsideDeviation
- All metrics with Window(Days(n)) and Window(Years(n))
- KRatio with < 3 returns, identical returns
- KellerRatio with drawdown > 50%
- ValueAtRisk with empty returns, verified against known percentile

**Convenience methods:**
- Summary(), RiskMetrics(), WithdrawalMetrics()

**Selectors/Weightings:**
- MaxAboveZero with NaN values, all equal, no positive values with nil fallback
- WeightedBySignal with negative/NaN signals, single asset, multiple timesteps

**Tax/Trade:**
- TaxMetrics with multiple assets, partial lot consumption, losses
- TradeMetrics with only losing trades, break-even trades, multiple assets

### 5. All Ginkgo

Every test file uses `package portfolio_test` with Describe/Context/It blocks. No raw testing.T tests except the Ginkgo bootstrap in portfolio_suite_test.go.

### Expected Value Strategy

For each metric, construct a small (3-5 point) equity curve and hand-calculate the expected result. Assert with `BeNumerically("~", expected, 1e-6)`. Document the calculation in a comment above the assertion.
