# Fix TWRR to Eliminate Cash Flow Effects

## Problem

The TWRR (Time-Weighted Rate of Return) metric in `portfolio/twrr.go` does not
adjust for external cash flows. It compounds raw equity curve percentage
changes via `Pct()`, treating deposit- and withdrawal-driven equity movements
as investment returns. This violates TWRR's core purpose: measuring portfolio
performance independent of the timing and size of cash flows.

Example: an investor deposits 10000, then 5500, then 1000 with zero market
growth. Terminal equity equals total deposits (16500). The current
implementation reports TWRR = 0.65 (65%); the correct answer is 0.0.

MWRR (Money-Weighted Rate of Return) is correctly implemented and is not
changed by this work.

## Approach

Use Approach 1 from brainstorming: adjust sub-period returns using the
transaction log. The fix is self-contained in `twrr.go` with no changes to
shared interfaces or the equity curve.

## Algorithm

Both `Compute` and `ComputeSeries` switch from `Returns()` (raw `Pct()` on
equity) to `EquitySeries()` + `TransactionsView()`.

### Data Sources

- **Equity curve:** `stats.EquitySeries(ctx, window)` returns a DataFrame with
  timestamps and `PortfolioEquity` values. Each value is `cash + sum(holdings *
  price)`, recorded by `Account.UpdatePrices`.
- **Transactions:** `stats.TransactionsView(ctx)` returns the full transaction
  log. Only `DepositTransaction` and `WithdrawalTransaction` are relevant.
  When a `window` is active, filter the flow map to only include transactions
  whose dates fall within the equity series date range.

### Flow Map Construction

Build a `map[time.Time]float64` of net external flows per date:

- `DepositTransaction`: add `txn.Amount` (positive in the transaction log)
- `WithdrawalTransaction`: add `txn.Amount` (negative in the transaction log)

Multiple flows on the same date are summed.

### Sub-Period Return Calculation

Walk the equity curve sequentially. Maintain `subPeriodStart`, initialized to
`equity[0]`.

At each subsequent timestamp `t` with equity value `e`:

1. Look up `flow = flowMap[t]` (zero if no flow on this date).
2. Compute `preFlowEquity = e - flow`. This is the portfolio value from market
   movements alone, before the external flow changed it. When `flow` is zero,
   `preFlowEquity == e`.
3. Compute sub-period return: `r = preFlowEquity / subPeriodStart - 1`.
4. Update `subPeriodStart = e`. On flow days this resets the base to
   post-flow equity; on non-flow days it advances to the current equity
   so the next day's return is a single-period return, not cumulative.

### Compute (Scalar)

Chain all sub-period returns: `product(1 + r_i) - 1`.

When there are zero external flows, every `flow` lookup returns zero, so
`preFlowEquity == e` at every step. The algorithm degenerates to the current
behavior (single sub-period over the entire curve).

### ComputeSeries (Cumulative Curve)

Same sub-period logic, but emit the running product minus 1 at every equity
timestamp, not just at flow boundaries. The result has length `len(equity) - 1`
(first equity point is the reference, not a return).

## Files Changed

| File | Change |
|------|--------|
| `portfolio/twrr.go` | Rewrite `Compute` and `ComputeSeries` to use the sub-period algorithm above. Add a helper to build the flow map from transactions. Remove dependency on `Returns()`. |
| `portfolio/return_metrics_test.go` | Refactor `buildReturnAccount` so it does not record deposit/withdrawal transactions for pure market-movement tests. Update deposit-only test. Add new tests for mixed growth + deposits, withdrawals, and multiple same-day flows. |

## Test Helper Refactoring

The existing `buildReturnAccount` helper records a `DepositTransaction` or
`WithdrawalTransaction` for every equity change between consecutive points.
This was harmless when TWRR ignored transactions, but the new algorithm reads
`TransactionsView` and would treat these synthetic flows as real external cash
flows, breaking all existing tests.

**Fix:** Change `buildReturnAccount` to use `DividendTransaction` (positive
changes) and `FeeTransaction` (negative changes) instead of
deposit/withdrawal. These transaction types are not external cash flows -- they
represent organic portfolio events. The TWRR algorithm only looks at
`DepositTransaction` and `WithdrawalTransaction`, so dividend/fee transactions
are invisible to it. The equity curve is unchanged because `UpdatePrices`
computes equity from `cash + holdings`, and these transactions still adjust
cash the same way.

Tests that specifically test cash flow behavior (the deposit-only test, new
mixed-flow tests) build their accounts manually without using
`buildReturnAccount`.

## Convention

TWRR continues to return a **cumulative** (non-annualized) return. MWRR
continues to return an **annualized** rate (XIRR). These are standard
conventions for each metric.

## Edge Cases

- **No external flows:** Algorithm degenerates to current behavior. No
  performance regression for pure backtests.
- **Flow on first date:** The initial deposit is a flow. `subPeriodStart` is
  set to `equity[0]` which already includes the initial deposit. No sub-period
  return is computed for the first point (it is the reference).
- **Flow on last date:** A sub-period return is computed using `preFlowEquity`.
  The flow itself does not contribute to the return.
- **Multiple flows on same date:** Summed into a single net flow. One
  sub-period boundary.
- **Single data point:** Returns 0 (no sub-periods to chain).
- **Zero equity at sub-period start:** That sub-period return is treated as 0
  (skip the division). Subsequent sub-periods continue to be computed and
  chained normally. This handles the case where an account starts at zero and
  then receives a deposit.

## Test Plan

1. **Existing no-flow tests pass unchanged** -- 5-point equity curve (TWRR =
   0.20), declining curve (TWRR = -0.20), single point (TWRR = 0). These use
   the refactored `buildReturnAccount` which no longer records
   deposit/withdrawal transactions.
2. **Deposit-only test corrected** -- equity [10000, 15500, 16500] with
   deposits [5500, 1000]: TWRR = 0.0 (was incorrectly 0.65).
3. **New: growth plus deposit** -- equity [10000, 11000, 17000] where day 1 has
   $1000 organic growth and day 2 has $5000 deposit + $1000 organic growth.
   Sub-period 1: preFlowEquity = 11000, return = 11000/10000 - 1 = 0.10.
   Sub-period 2: preFlowEquity = 17000 - 5000 = 12000, return = 12000/11000.
   TWRR = (11000/10000) * (12000/11000) - 1 = 0.20.
4. **New: withdrawal** -- equity [10000, 9000, 7000] where day 2 has a $3000
   withdrawal and $1000 market growth. Sub-period 1: preFlowEquity = 9000,
   return = 9000/10000 - 1 = -0.10. Sub-period 2: flow = -3000,
   preFlowEquity = 7000 - (-3000) = 10000, return = 10000/9000 - 1.
   TWRR = (9000/10000) * (10000/9000) - 1 = 0.0.
5. **New: ComputeSeries with flows** -- use the growth-plus-deposit scenario
   from test 3. Verify cumulative curve: cum[0] = 0.10 (market growth day 1),
   cum[1] = 0.20 (market growth day 1 + day 2, deposit stripped out).
6. **New: TWRR vs MWRR divergence** -- investor deposits 10000, market grows
   to 12000 (+20%), then deposits another 10000 (equity = 22000), market drops
   10% to 19800. TWRR = (12000/10000) * (19800/22000) - 1 = 1.2 * 0.9 - 1 =
   0.08. MWRR will be lower because more money was invested before the loss.
