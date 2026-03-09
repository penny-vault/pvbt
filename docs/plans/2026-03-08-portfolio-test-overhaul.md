# Portfolio Test Quality Overhaul Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace meaningless tests with hand-calculated assertions, convert all tests to black-box, fix test infrastructure, and add comprehensive edge case coverage.

**Architecture:** Delete metric_helpers_test.go. Create shared testutil_test.go with buildDF and buildAccount helpers. Rewrite each metric test file to use precise expected values. All tests are Ginkgo in package portfolio_test.

**Tech Stack:** Go, Ginkgo/Gomega, package portfolio_test

---

### Task 1: Create shared test infrastructure

**Files:**
- Create: `portfolio/testutil_test.go`

**Step 1: Create testutil_test.go with shared helpers and compile-time interface checks**

```go
package portfolio_test

import (
	"time"

	"github.com/penny-vault/pvbt2/assets"
	"github.com/penny-vault/pvbt2/data"
	"github.com/penny-vault/pvbt2/portfolio"

	. "github.com/onsi/gomega"
)

// Compile-time interface checks
var (
	_ portfolio.Portfolio        = (*portfolio.Account)(nil)
	_ portfolio.PortfolioManager = (*portfolio.Account)(nil)
	_ portfolio.Selector         = portfolio.MaxAboveZero(nil)
)

// buildDF builds a single-timestamp DataFrame with MetricClose and AdjClose.
func buildDF(t time.Time, assets []asset.Asset, closes, adjCloses []float64) *data.DataFrame {
	vals := make([]float64, 0, len(assets)*2)
	for i := range assets {
		vals = append(vals, closes[i])
		vals = append(vals, adjCloses[i])
	}
	df, err := data.NewDataFrame(
		[]time.Time{t},
		assets,
		[]data.Metric{data.MetricClose, data.AdjClose},
		vals,
	)
	Expect(err).NotTo(HaveOccurred())
	return df
}

// buildMultiDF builds a multi-timestamp DataFrame with MetricClose and AdjClose.
// closeSeries[i] and adjCloseSeries[i] are the values for assets at time times[i].
// Each inner slice must have len(assets) elements.
func buildMultiDF(times []time.Time, assets []asset.Asset, closeSeries, adjCloseSeries [][]float64) *data.DataFrame {
	vals := make([]float64, 0, len(times)*len(assets)*2)
	for i := range times {
		for j := range assets {
			vals = append(vals, closeSeries[i][j])
			vals = append(vals, adjCloseSeries[i][j])
		}
	}
	df, err := data.NewDataFrame(
		times,
		assets,
		[]data.Metric{data.MetricClose, data.AdjClose},
		vals,
	)
	Expect(err).NotTo(HaveOccurred())
	return df
}

// daySeq returns n timestamps starting from start, each one trading day apart (skipping weekends).
func daySeq(start time.Time, n int) []time.Time {
	times := make([]time.Time, n)
	t := start
	for i := 0; i < n; i++ {
		// skip weekends
		for t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			t = t.AddDate(0, 0, 1)
		}
		times[i] = t
		t = t.AddDate(0, 0, 1)
	}
	return times
}

// monthSeq returns n timestamps starting from start, each one month apart.
func monthSeq(start time.Time, n int) []time.Time {
	times := make([]time.Time, n)
	for i := 0; i < n; i++ {
		times[i] = start.AddDate(0, i, 0)
	}
	return times
}
```

NOTE: The exact import paths for asset and data packages must be verified against existing test files (check the import blocks in account_test.go). Adjust `assets` vs `asset` accordingly.

**Step 2: Run tests to verify compilation**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run='^$' -count=1`
Expected: compiles without error (no tests run)

**Step 3: Commit**

```
git add portfolio/testutil_test.go
git commit -m "test(portfolio): add shared test utilities and interface checks"
```

---

### Task 2: Fix mockBroker and rebalance test infrastructure

**Files:**
- Modify: `portfolio/order_test.go` (lines 45-57, mockBroker.Submit)
- Modify: `portfolio/rebalance_test.go` (lines 137-140, order assertions)

**Step 1: Fix mockBroker to fail when unconfigured fills exhausted**

In `order_test.go`, replace the fallback default fill with a Ginkgo Fail():

```go
// REPLACE lines 49-56 of Submit method with:
func (m *mockBroker) Submit(order broker.Order) ([]broker.Fill, error) {
	m.submitted = append(m.submitted, order)
	if m.callIdx < len(m.fills) {
		f := m.fills[m.callIdx]
		m.callIdx++
		return f, nil
	}
	Fail(fmt.Sprintf("mockBroker: unexpected Submit call #%d with no configured fill", m.callIdx))
	return nil, nil
}
```

**Step 2: Fix rebalance_test.go to not depend on map iteration order**

In `rebalance_test.go`, replace assertions on submitted[0] and submitted[1] with order-independent checks. For the "sells excess and buys new" test (around lines 137-140):

```go
// Instead of asserting submitted[0].Side == Sell and submitted[1].Side == Buy,
// collect sides and assert on the set:
var sells, buys int
for _, o := range mb.submitted {
    if o.Side == broker.Sell {
        sells++
    } else {
        buys++
    }
}
Expect(sells).To(Equal(1))
Expect(buys).To(Equal(1))
```

Apply similar fixes to all rebalance assertions that depend on order.

**Step 3: Run tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -count=1 -v`
Expected: all existing tests pass

**Step 4: Commit**

```
git add portfolio/order_test.go portfolio/rebalance_test.go
git commit -m "test(portfolio): fix mockBroker default fill and map iteration flakiness"
```

---

### Task 3: Delete metric_helpers_test.go and meaningless tests

**Files:**
- Delete: `portfolio/metric_helpers_test.go`
- Modify: `portfolio/benchmark_metrics_test.go` - remove all Name() tests (lines 96-97, 111-112, 126-127, 141-142, 156-157, 180-181, 356-365), remove all "returns nil for ComputeSeries" tests (lines 105-107, 120-122, 135-137, 150-152, 174-176, 189-191)
- Modify: `portfolio/risk_adjusted_metrics_test.go` - remove all Name() tests (lines 111-112, 127-128, 143-144, 154-155, 171-172, 182-183)
- Modify: `portfolio/capture_drawdown_metrics_test.go` - remove all Name() tests (lines 100-102, 139-140, 177-178), remove "returns nil" tests (lines 110-112, 148-150, 186-188)
- Modify: `portfolio/distribution_metrics_test.go` - remove all Name() tests (lines 96-97, 138-139, 179-180, 220-221), remove "returns nil" tests (lines 130-134, 171-175, 212-216, 258-262)
- Modify: `portfolio/specialized_metrics_test.go` - remove all Name() tests (lines 78-79, 115-116, 132-134, 150-151), remove "returns nil" tests (lines 107-111, 125-129, 143-147, 181-185)
- Modify: `portfolio/return_metrics_test.go` - remove Name() tests (lines 100-102, 141-142, 183-184), remove "returns nil" test (lines 175-179)
- Modify: `portfolio/withdrawal_metrics_test.go` - remove Name() tests (lines 85-86, 114-115, 137-138), remove "returns nil" tests (lines 107-110, 130-133, 159-162)

**Step 1: Delete metric_helpers_test.go**

```
rm portfolio/metric_helpers_test.go
```

**Step 2: Remove all Name() and "returns nil for ComputeSeries" It blocks from each test file**

For each file listed above, delete the `It("returns a name..."` and `It("returns nil for ComputeSeries"...)` blocks. Also delete the duplicate "Name methods" Describe block in benchmark_metrics_test.go (lines 356-365).

**Step 3: Replace directional-only assertions with hand-calculated values**

In `risk_adjusted_metrics_test.go`, the tests for StdDev, MaxDrawdown, DownsideDeviation, Sharpe, Sortino, and Calmar currently assert only on sign. Replace with computed expected values based on the 20-point equity curve defined in the BeforeEach. This requires reading the exact equity curve values from the BeforeEach setup and computing the expected metric values by hand.

Similarly for `capture_drawdown_metrics_test.go`, `distribution_metrics_test.go`, `specialized_metrics_test.go`.

**Step 4: Remove buildDF duplicates from each test file that now uses the shared one**

Delete the local `buildDF` definitions from: `return_metrics_test.go`, `risk_adjusted_metrics_test.go`, `withdrawal_metrics_test.go`, `distribution_metrics_test.go`, `tax_metrics_test.go`, `capture_drawdown_metrics_test.go`, `specialized_metrics_test.go`, `benchmark_metrics_test.go`, `account_test.go`.

Add import of the shared helper if needed (since it's in the same test package, no import needed).

**Step 5: Run tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -count=1 -v`
Expected: all remaining tests pass

**Step 6: Commit**

```
git add -u portfolio/
git commit -m "test(portfolio): remove meaningless tests and deduplicate buildDF"
```

---

### Task 4: Rewrite risk-adjusted metric tests with precise expected values

**Files:**
- Modify: `portfolio/risk_adjusted_metrics_test.go`

**Step 1: Construct a small known equity curve and hand-calculate expected values**

Use a 5-point equity curve: [100, 105, 103, 108, 110] with daily timestamps and BIL risk-free at [100, 100.01, 100.02, 100.03, 100.04].

Hand-calculate:
- Returns: [0.05, -0.01905, 0.04854, 0.01852]
- RF returns: [0.0001, 0.0001, 0.0001, 0.0001]
- Excess returns: [0.0499, -0.01915, 0.04844, 0.01842]
- Mean excess: 0.024403
- StdDev excess: 0.03116 (sample)
- Sharpe (annualized): mean/std * sqrt(252)
- Drawdown series: [0, 0, -0.01905, 0, 0]
- MaxDrawdown: -0.01905
- And so on for each metric

Write Ginkgo tests asserting `BeNumerically("~", expected, 1e-4)`.

**Step 2: Write tests for edge cases**

- StdDev with flat curve (all same values) -> 0
- Sharpe with zero excess stddev -> 0
- Sortino with no negative excess returns -> 0
- Calmar with zero drawdown -> 0
- MaxDrawdown with monotonically rising curve -> 0

**Step 3: Run tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run='RiskAdjusted' -count=1 -v`
Expected: PASS

**Step 4: Commit**

```
git add portfolio/risk_adjusted_metrics_test.go
git commit -m "test(portfolio): rewrite risk-adjusted metrics with hand-calculated values"
```

---

### Task 5: Rewrite benchmark metric tests with precise expected values

**Files:**
- Modify: `portfolio/benchmark_metrics_test.go`

**Step 1: Construct known portfolio and benchmark curves, hand-calculate**

Use a small curve (5-6 points) where portfolio and benchmark diverge in known ways.

Hand-calculate Beta, Alpha, TrackingError, InformationRatio, Treynor, RSquared from the returns. Document the calculation in comments.

**Step 2: Add edge cases**

- Beta with zero benchmark variance -> 0 or handled gracefully
- Alpha with perfect tracking -> 0
- RSquared with uncorrelated returns -> near 0
- All metrics with 2-point curves (minimum viable input)

**Step 3: Run tests, commit**

---

### Task 6: Rewrite return metric tests with precise expected values

**Files:**
- Modify: `portfolio/return_metrics_test.go`

**Step 1: TWRR with known curve**

Equity: [100, 110, 105, 115, 120]
Returns: [0.10, -0.04545, 0.09524, 0.04348]
TWRR = (1.10)(0.95455)(1.09524)(1.04348) - 1 = 0.20

Assert: `BeNumerically("~", 0.20, 1e-6)`

**Step 2: TWRR edge cases**

- Declining curve (negative return)
- Single data point (returns 0)

**Step 3: TWRR ComputeSeries test**

Verify each element of the series matches cumulative return at that point.

**Step 4: MWRR with known cash flows**

Build an Account with a mid-stream deposit. Hand-calculate XIRR and assert.

**Step 5: MWRR edge cases**

- Zero-date deposit (exercises the IsZero branch)
- Convergence edge case (if feasible to construct)

**Step 6: ActiveReturn with known benchmark**

Portfolio TWRR - Benchmark TWRR = expected ActiveReturn.

**Step 7: Run tests, commit**

---

### Task 7: Rewrite distribution metric tests with precise expected values

**Files:**
- Modify: `portfolio/distribution_metrics_test.go`

**Step 1: Hand-calculate for a small known return series**

Use returns: [0.05, -0.02, 0.03, -0.01, 0.04, -0.03, 0.02]

Hand-calculate:
- ExcessKurtosis (using sample formula)
- Skewness (using sample formula)
- NPositivePeriods = 4/7
- GainLossRatio = mean([0.05,0.03,0.04,0.02]) / |mean([-0.02,-0.01,-0.03])| = 0.035/0.02 = 1.75

**Step 2: Edge cases**

- All positive returns (NPositivePeriods = 1.0, GainLossRatio -> inf or handled)
- All negative returns
- Single return

**Step 3: Run tests, commit**

---

### Task 8: Rewrite specialized metric tests with precise expected values

**Files:**
- Modify: `portfolio/specialized_metrics_test.go`

**Step 1: Hand-calculate UlcerIndex, ValueAtRisk, KRatio, KellerRatio**

For a known 5-point equity curve, compute each metric and assert.

- UlcerIndex = sqrt(mean(drawdown^2))
- ValueAtRisk = sorted returns at 5th percentile (need enough data points)
- KRatio = slope / (N * stderr) of log(VAMI) regression
- KellerRatio = R * (1 - D/(1-D)) for R >= 0, D <= 0.5

**Step 2: Edge cases**

- KRatio with < 3 returns -> 0
- KRatio with identical returns (zero stderr)
- KellerRatio with drawdown > 50% -> 0
- KellerRatio with negative total return -> 0
- ValueAtRisk with empty returns -> 0

**Step 3: Run tests, commit**

---

### Task 9: Rewrite capture/drawdown metric tests with precise expected values

**Files:**
- Modify: `portfolio/capture_drawdown_metrics_test.go`

**Step 1: Hand-calculate UpsideCaptureRatio, DownsideCaptureRatio, AvgDrawdown**

Build portfolio and benchmark with known divergence in up/down markets.

**Step 2: Edge cases**

- Portfolio that doesn't participate in down markets
- Flat benchmark
- No drawdowns in portfolio

**Step 3: Run tests, commit**

---

### Task 10: Rewrite withdrawal metric tests with precise expected values

**Files:**
- Modify: `portfolio/withdrawal_metrics_test.go`

**Step 1: Replace range assertions with seed-specific expected values**

Since the Monte Carlo uses `rand.NewSource(42)`, the results are deterministic. Compute the exact values for the test equity curves and assert with tight tolerance.

**Step 2: Add edge cases**

- Declining equity curve (should return 0 or very low rates)
- Short equity curve (< 21 data points, not enough for monthly returns)
- Flat equity curve

**Step 3: Verify PerpetualWithdrawalRate <= SafeWithdrawalRate <= DynamicWithdrawalRate ordering for growing curves**

**Step 4: Run tests, commit**

---

### Task 11: Add missing Account edge case tests

**Files:**
- Modify: `portfolio/account_test.go`
- Modify: `portfolio/rebalance_test.go`
- Modify: `portfolio/order_test.go`

**Step 1: Order with broker error**

Configure mockBroker.Submit to return an error. Verify Account.Order() handles it (logs and returns without panicking, cash unchanged).

**Step 2: RebalanceTo edge cases**

- NaN/zero price for an asset in target allocation
- Empty Members map
- Multiple allocations (variadic)

**Step 3: Holdings with actual positions**

Create account, buy assets, call Holdings(), verify callback receives correct assets and quantities.

**Step 4: Value() and PositionValue() edge cases**

- PositionValue with nil prices and non-zero holding -> 0
- Value() with NaN price for held asset (should skip)

**Step 5: Record edge cases**

- Full position depletion (sell all shares)
- Multiple tax lots with partial FIFO consumption

**Step 6: TransactionType.String() unknown type**

```go
It("returns a formatted string for unknown types", func() {
    t := portfolio.TransactionType(99)
    Expect(t.String()).To(Equal("TransactionType(99)"))
})
```

**Step 7: WithCash(0)**

Verify no deposit transaction is recorded (or verify it is -- check the implementation).

**Step 8: Run tests, commit**

---

### Task 12: Add 0%-coverage function tests

**Files:**
- Modify: `portfolio/account_test.go` (for Summary, RiskMetrics, WithdrawalMetrics)
- Create or modify metric-specific test files (for ComputeSeries, Window, Days, Years)

**Step 1: Summary() test**

Build an Account with a known equity curve. Call Summary(). Verify each field matches the corresponding individual metric Compute() call.

**Step 2: RiskMetrics() test**

Same approach -- verify each field matches individual metric computation.

**Step 3: WithdrawalMetrics() test**

Same approach.

**Step 4: Window() / Days() / Years() tests**

```go
It("computes a windowed metric value", func() {
    // Build account with 100 data points
    // Compute TWRR with full history
    // Compute TWRR with Window(Months(3))
    // Verify windowed value differs from full and is correct
})

It("uses Days period", func() {
    val := acct.PerformanceMetric(portfolio.TWRR).Window(portfolio.Days(30)).Value()
    // verify against hand-calculated value for last 30 days
})

It("uses Years period", func() {
    val := acct.PerformanceMetric(portfolio.TWRR).Window(portfolio.Years(1)).Value()
    // verify against hand-calculated value for last year
})
```

**Step 5: ComputeSeries for Calmar, Sharpe, Sortino, DownsideDeviation**

If these should return nil (scalar-only metrics), the tests were already removed. If they should return actual series, implement and test. Check the source to confirm -- if ComputeSeries returns nil, no test needed (that's implementation, not behavior).

NOTE: Based on source analysis, Sharpe/Sortino/Calmar/DownsideDeviation all have ComputeSeries returning nil. These are intentionally scalar-only. No tests needed for "returns nil" -- that's a meaningless test. If the package later adds series support, tests should be added then.

**Step 6: Run tests, commit**

---

### Task 13: Add tax and trade metric edge case tests

**Files:**
- Modify: `portfolio/tax_metrics_test.go`
- Modify: `portfolio/trade_metrics_test.go`

**Step 1: TaxMetrics with multiple assets**

Build account with buys/sells of SPY and AAPL. Verify FIFO tracking works per-asset.

**Step 2: TaxMetrics with capital losses**

Verify STCG and LTCG can be negative. Verify TaxCostRatio handles negative gains correctly.

**Step 3: TaxMetrics partial lot consumption**

Buy 100 shares at $10, buy 50 shares at $12, sell 120 shares at $15. Verify FIFO: first lot fully consumed, second lot partially consumed. Check realized gains math.

**Step 4: TradeMetrics with only losing trades**

All sells below purchase price. Verify WinRate=0, AverageWin=0, ProfitFactor=0.

**Step 5: TradeMetrics with break-even trade**

Sell at purchase price (PnL=0). Verify classification (counted as loss per implementation).

**Step 6: TradeMetrics with multiple assets**

Verify FIFO matching works across different assets independently.

**Step 7: Run tests, commit**

---

### Task 14: Add selector and weighting edge case tests

**Files:**
- Modify: `portfolio/selector_test.go`
- Modify: `portfolio/weighting_test.go`

**Step 1: MaxAboveZero edge cases**

- All NaN values mixed with positive values (NaN should be skipped)
- All equal positive values (deterministic winner)
- No positive values with nil fallback (empty selection)
- Single asset (trivial selection)

**Step 2: WeightedBySignal edge cases**

- All negative signal values -> equal weight fallback
- Mix of positive and NaN values
- Single asset -> weight 1.0
- Zero signal sum -> equal weight fallback

**Step 3: Run tests, commit**

---

### Task 15: Stop using phantom transactions for equity curve manipulation

**Files:**
- Modify: `portfolio/benchmark_metrics_test.go`
- Modify: `portfolio/return_metrics_test.go`
- Modify: `portfolio/capture_drawdown_metrics_test.go`
- Modify: `portfolio/distribution_metrics_test.go`
- Modify: `portfolio/specialized_metrics_test.go`

**Step 1: Audit each test file for phantom dividend/fee transactions**

Identify all places where DividendTransaction or FeeTransaction are used solely to adjust cash for equity curve manipulation.

**Step 2: Replace with actual buy/sell sequences or direct construction**

Use actual market operations (buy asset, price changes, sell) to create desired equity curves. Alternatively, if Account exposes enough to set up equity curves directly, use that approach.

NOTE: This may be the hardest task. If the Account struct doesn't support direct equity curve injection, it may be acceptable to keep the cash-adjustment approach but use DepositTransaction/WithdrawalTransaction instead of Dividend/Fee to avoid contaminating tax calculations. Evaluate and choose the cleanest approach.

**Step 3: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -count=1 -v`
Expected: all tests pass

**Step 4: Commit**

```
git commit -m "test(portfolio): stop using phantom transactions for equity curve setup"
```

---

### Task 16: Final verification

**Step 1: Run full test suite with race detector**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -race -count=1 -v`
Expected: all tests pass, no race conditions

**Step 2: Check coverage**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -coverprofile=coverage.out && go tool cover -func=coverage.out | grep portfolio`
Expected: coverage should be significantly higher than 89.4%, with 0%-coverage functions now tested

**Step 3: Verify no white-box tests remain**

Run: `grep -r 'package portfolio$' portfolio/*_test.go`
Expected: no matches (only `package portfolio_test`)

**Step 4: Commit any final adjustments**
