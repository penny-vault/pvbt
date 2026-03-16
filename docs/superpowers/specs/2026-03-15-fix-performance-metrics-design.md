# Fix Performance Metrics Design

## Problem

Several performance metrics produce incorrect values:

1. **Sharpe/Sortino/Alpha**: The risk-free rate is treated as a price series and Pct()'d, but DGS3MO stores annualized yields. Pct() of yields gives the change in yield, not the risk-free return.
2. **Treynor**: Uses total cumulative returns instead of annualized.
3. **Withdrawal metrics**: Monte Carlo bootstrap simulation doesn't match the standard SWR definition (deterministic, actual return path).
4. **annualizationFactor** (fixed): Was hardcoded as 252/12 binary. Now computes from actual observation frequency.
5. **Daily equity** (fixed): Engine now records equity every trading day via `@close * * *` tradecron schedule regardless of strategy schedule.

## Design

### Risk-Free Rate: System-Level, Not Strategy-Configurable

The risk-free rate is a market constant, not a strategy parameter. Every strategy backtested over the same period should use the same risk-free rate.

**Implementation:**

- Hardcode DGS3MO (3-Month Treasury Constant Maturity Rate) as the risk-free rate for all performance metrics.
- The engine resolves DGS3MO during initialization and includes it in the existing fetch asset lists. The year-chunk cache handles efficiency -- no per-day overhead.
- The engine converts the DGS3MO yield to a cumulative price-equivalent before passing it to `UpdatePrices`. This keeps `UpdatePrices` free of data-format concerns. The conversion formula:
  ```
  daily_return = (1 + yield/100)^(1/252) - 1
  cumulative[0] = 100.0
  cumulative[t] = cumulative[t-1] * (1 + daily_return)
  ```
  The `PortfolioRiskFree` column in perfData always stores this cumulative series, so downstream `Pct()` calls produce correct daily risk-free returns.
- Forward-fill DGS3MO gaps: FRED data has missing days (holidays, weekends). On days where the yield is unavailable, use the last known yield to compute the daily return.
- If DGS3MO is entirely unavailable (e.g., test environments without FRED data), fall back to rf=0%, log a warning, and store a flat cumulative series (100.0 on every day) so `Pct()` produces 0% returns.
- Deprecate `RiskFreeAsset()` on the engine: keep the method but log a deprecation warning and ignore the argument. This avoids breaking existing strategy code at compile time. Remove the method in a future major version.
- Remove `SetRiskFree()` and the `riskFree` field from Account. The engine sets the risk-free values directly.
- SQLite persistence: continue writing `risk_free_ticker` and `risk_free_figi` metadata as "DGS3MO" for all backtests. When restoring from SQLite, ignore these fields (the system always uses DGS3MO).
- Strategies that need risk-free data for their own calculations (e.g., ADM's risk-adjusted signals) fetch DGS3MO directly through `eng.Fetch()`.

### Treynor: Annualize

The current Treynor implementation uses total cumulative returns:
```
Treynor = (totalPortfolioReturn - totalRfReturn) / beta
```

For a 2-year backtest this gives ~2x the correct value. Fix to use annualized returns:
```
Treynor = (CAGR_portfolio - CAGR_riskFree) / beta
```

Where `CAGR = (endValue/startValue)^(1/years) - 1` and `years` is computed from the actual timestamp span (consistent with `annualizationFactor`).

For very short backtests (< 30 days), return 0 to avoid extreme annualized values.

### Withdrawal Metrics: Actual Return Path

The current implementation uses Monte Carlo circular block bootstrap to simulate 30 years of returns. This doesn't match the standard SWR definition and produces nonsensical results for short backtests.

**Standard SWR definition:** The maximum percentage of the original portfolio balance that can be withdrawn at the end of each year (with inflation adjustment) without the portfolio running out of money over the actual backtest period. SWR is specific to the time period and return path, making it mostly useful as a relative comparison metric.

**Implementation:**

For each candidate withdrawal rate (linear scan from 0.1% to 20% in 0.1% steps):
1. Start with the initial portfolio value.
2. Years are measured from the backtest start date at 12-month anniversary intervals. Partial final years are included (withdrawal is prorated).
3. At each year boundary, withdraw `rate * initial_balance * (1 + inflation)^year`.
4. Apply actual daily returns from the equity curve between withdrawals.
5. If the balance reaches zero, the rate is too high.
6. Return the highest rate where the portfolio survives.

**Inflation:** Hardcode 3% annually as a package-level constant (`defaultInflationRate`).

**Safe Withdrawal Rate:** Balance never reaches zero.

**Perpetual Withdrawal Rate:** Ending balance >= `starting_balance * (1 + inflation)^years`. This preserves real (inflation-adjusted) purchasing power, not just nominal value.

**Dynamic Withdrawal Rate:** Each year's withdrawal is `min(inflation-adjusted initial withdrawal, balance * rate)`. Balance must never reach zero.

For short backtests (< 5 years), the SWR will be high because there aren't enough years of withdrawals to stress the portfolio. This is correct behavior -- the metric is a relative comparison tool.

### Already Implemented

These changes are already on the `main` branch:

- **Daily equity recording**: Backtest and live engines walk a `@close * * *` tradecron schedule, recording mark-to-market equity every trading day. Strategy Compute only fires on strategy-schedule dates. Live engine retries price fetches up to 18 times (1 hour apart) for delayed mutual fund NAVs.
- **annualizationFactor**: Computes `(N-1) / calendar_years` from actual timestamps instead of hardcoding 252/12.
- **Alpha**: Uses `(mean(R_p - R_f) - beta * mean(R_m - R_f)) * AF` instead of total cumulative returns.

## Files Affected

| File | Action | Change |
|------|--------|--------|
| `engine/engine.go` | Modify | Deprecate `RiskFreeAsset()` (log warning, ignore arg); add internal DGS3MO resolution and yield-to-cumulative conversion |
| `engine/backtest.go` | Modify | Include DGS3MO in fetch lists; pass converted cumulative RF values to `UpdatePrices` |
| `engine/live.go` | Modify | Same as backtest |
| `engine/doc.go` | Modify | Update documentation to reflect non-configurable RF rate; remove `RiskFreeAsset` from examples |
| `engine/descriptor.go` | Modify | Set `RiskFree` field to "DGS3MO" unconditionally instead of reading from `eng.riskFree` |
| `engine/descriptor_test.go` | Modify | Update expectations |
| `portfolio/account.go` | Modify | Remove `riskFree` field, `SetRiskFree()`; update `Clone()` |
| `portfolio/treynor.go` | Modify | Annualize using CAGR; return 0 for very short backtests |
| `portfolio/safe_withdrawal_rate.go` | Rewrite | Deterministic actual-path simulation |
| `portfolio/perpetual_withdrawal_rate.go` | Rewrite | Same approach, inflation-adjusted success criterion |
| `portfolio/dynamic_withdrawal_rate.go` | Rewrite | Same approach with dynamic adjustment |
| `portfolio/metric_helpers.go` | Modify | Remove `monthlyReturnsFromEquity`; add yield-to-cumulative helper |
| `portfolio/sqlite.go` | Modify | Write "DGS3MO" as RF metadata; ignore RF fields on restore |
| `examples/momentum-rotation/main.go` | Modify | Remove `eng.RiskFreeAsset()` call |
| `CHANGELOG.md` | Modify | Document all metric fixes and breaking API changes |
| Various test files | Modify | Update expectations; remove RF configuration from test helpers |
