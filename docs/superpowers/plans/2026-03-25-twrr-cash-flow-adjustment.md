# TWRR Cash Flow Adjustment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the TWRR metric to properly eliminate external cash flow effects by splitting the equity curve into sub-periods at each deposit/withdrawal.

**Architecture:** Rewrite `twrr.Compute` and `twrr.ComputeSeries` to use `EquitySeries()` + `TransactionsView()` instead of `Returns()`. A helper function builds a flow map from transactions; the main loop walks the equity curve, subtracts flows to isolate market returns, and chains sub-period returns geometrically.

**Tech Stack:** Go, Ginkgo/Gomega for tests.

**Spec:** `docs/superpowers/specs/2026-03-24-twrr-cash-flow-adjustment-design.md`

---

### Task 1: Refactor `buildReturnAccount` test helper

The existing helper records `DepositTransaction`/`WithdrawalTransaction` for every equity change. The new TWRR algorithm reads these transaction types, so the helper must use transaction types that are invisible to TWRR.

**Files:**
- Modify: `portfolio/return_metrics_test.go:26-52`

- [ ] **Step 1: Change transaction types in buildReturnAccount**

Replace `DepositTransaction` with `DividendTransaction` and `WithdrawalTransaction` with `FeeTransaction`. These still adjust `cash` via `Record()` (line 774 of account.go: `a.cash += txn.Amount`), so the equity curve is unchanged. TWRR only filters for deposit/withdrawal, so these become invisible.

```go
// buildReturnAccount creates an account with the given equity curve.
// It uses DividendTransaction (positive) and FeeTransaction (negative)
// to move cash so equity = cash. These types are invisible to TWRR's
// cash flow adjustment (which only considers deposits/withdrawals).
buildReturnAccount := func(dates []time.Time, equity []float64) *portfolio.Account {
    a := portfolio.New(portfolio.WithCash(equity[0], time.Time{}))
    for ii, dd := range dates {
        if ii > 0 {
            diff := equity[ii] - equity[ii-1]
            if diff > 0 {
                a.Record(portfolio.Transaction{
                    Date:   dd,
                    Type:   asset.DividendTransaction,
                    Amount: diff,
                })
            } else if diff < 0 {
                a.Record(portfolio.Transaction{
                    Date:   dd,
                    Type:   asset.FeeTransaction,
                    Amount: diff,
                })
            }
        }
        df := buildDF(dd, []asset.Asset{spy}, []float64{100}, []float64{100})
        a.UpdatePrices(df)
    }
    return a
}
```

- [ ] **Step 2: Run existing tests to verify nothing breaks yet**

Run: `ginkgo run -race ./portfolio/ --focus="TWRR|MWRR"`
Expected: All existing tests PASS (TWRR still uses the old algorithm, but the helper change is transparent to it).

- [ ] **Step 3: Commit**

```
git add portfolio/return_metrics_test.go
git commit -m "test: use DividendTransaction/FeeTransaction in buildReturnAccount

The TWRR fix will read TransactionsView for deposit/withdrawal events.
Switch the test helper to use transaction types invisible to that filter
so existing pure-equity tests are unaffected."
```

---

### Task 2: Implement the flow-adjusted TWRR algorithm

Rewrite `twrr.Compute` and `twrr.ComputeSeries` to use the sub-period algorithm from the spec. Add a `buildFlowMap` helper.

**Files:**
- Modify: `portfolio/twrr.go` (full rewrite of lines 33-88)

- [ ] **Step 1: Write the `buildFlowMap` helper and updated `Compute`**

Replace the entire contents of `portfolio/twrr.go` (below the copyright header and package declaration):

```go
package portfolio

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

type twrr struct{}

func (twrr) Name() string { return "TWRR" }

func (twrr) Description() string {
	return "Time-weighted rate of return. Measures portfolio performance independent of the timing and size of cash flows. Computed by geometrically linking sub-period returns. The standard measure for comparing investment manager performance."
}

// buildFlowMap returns a map from date to net external cash flow amount.
// Only DepositTransaction and WithdrawalTransaction are considered.
// Transaction.Amount is positive for deposits, negative for withdrawals.
func buildFlowMap(transactions []Transaction, start, end time.Time) map[time.Time]float64 {
	flows := make(map[time.Time]float64)
	for _, txn := range transactions {
		if txn.Type != asset.DepositTransaction && txn.Type != asset.WithdrawalTransaction {
			continue
		}
		if txn.Date.Before(start) || txn.Date.After(end) {
			continue
		}
		flows[txn.Date] += txn.Amount
	}
	return flows
}

// subPeriodReturns walks the equity curve and computes flow-adjusted
// sub-period returns. It returns one return value per equity point after
// the first (len = len(equity) - 1).
func subPeriodReturns(equity []float64, times []time.Time, flowMap map[time.Time]float64) []float64 {
	returns := make([]float64, len(equity)-1)
	subPeriodStart := equity[0]

	for ii := 1; ii < len(equity); ii++ {
		ev := equity[ii]
		flow := flowMap[times[ii]]
		preFlowEquity := ev - flow

		ri := 0.0
		if subPeriodStart != 0 {
			ri = preFlowEquity/subPeriodStart - 1
		}
		returns[ii-1] = ri

		// Always advance subPeriodStart. On flow days this resets the
		// base to post-flow equity; on non-flow days it advances so
		// the next return is a single-period return, not cumulative.
		subPeriodStart = ev
	}

	return returns
}

// Compute returns the total time-weighted return over the window (or full
// history when window is nil). It splits the equity curve into sub-periods
// at each external cash flow, computes market-only returns per sub-period,
// and chains them geometrically: product(1 + r_i) - 1.
func (twrr) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return 0, nil
	}

	equity := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(equity) < 2 {
		return 0, nil
	}

	times := df.Times()
	flowMap := buildFlowMap(stats.TransactionsView(ctx), times[0], times[len(times)-1])
	returns := subPeriodReturns(equity, times, flowMap)

	product := 1.0
	for _, ri := range returns {
		product *= (1 + ri)
	}

	return product - 1, nil
}

// ComputeSeries returns the cumulative return at each point: the running
// product of (1 + r_i) minus 1. The result has length len(equity)-1.
func (twrr) ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period) (*data.DataFrame, error) {
	df := stats.EquitySeries(ctx, window)
	if df == nil {
		return nil, nil
	}

	equity := df.Column(portfolioAsset, data.PortfolioEquity)
	if len(equity) < 2 {
		return nil, nil
	}

	times := df.Times()
	flowMap := buildFlowMap(stats.TransactionsView(ctx), times[0], times[len(times)-1])
	returns := subPeriodReturns(equity, times, flowMap)

	cum := make([]float64, len(returns))
	product := 1.0
	for idx, ri := range returns {
		product *= (1 + ri)
		cum[idx] = product - 1
	}

	return data.NewDataFrame(
		times[1:],
		[]asset.Asset{portfolioAsset},
		[]data.Metric{data.PortfolioEquity},
		df.Frequency(),
		[][]float64{cum},
	)
}

func (twrr) BenchmarkTargetable() {}

// TWRR is the time-weighted rate of return, which eliminates the effect
// of cash flows (deposits/withdrawals) on portfolio returns.
var TWRR PerformanceMetric = twrr{}
```

- [ ] **Step 2: Verify existing no-flow tests still pass**

Run: `ginkgo run -race ./portfolio/ --focus="TWRR"`
Expected: The 4 existing TWRR tests PASS. The `buildReturnAccount` helper (refactored in Task 1) uses DividendTransaction/FeeTransaction, so `buildFlowMap` returns an empty map and the algorithm degenerates to the current behavior.

- [ ] **Step 3: Commit**

```
git add portfolio/twrr.go
git commit -m "fix: adjust TWRR for external cash flows (deposits/withdrawals)

TWRR now splits the equity curve into sub-periods at each deposit or
withdrawal, computes market-only returns per sub-period, and chains
them geometrically. Previously it compounded raw equity curve changes,
treating deposit-driven growth as investment return."
```

---

### Task 3: Fix the deposit-only test expectation

The existing test at `return_metrics_test.go:144-216` asserts TWRR = 0.65 for a scenario where all equity changes come from deposits. The correct answer is 0.0.

**Files:**
- Modify: `portfolio/return_metrics_test.go:144-216`

- [ ] **Step 1: Update the expected TWRR value and related assertions**

In the test `"differs from TWRR when there is a mid-stream deposit"` (around line 205-215), change:

```go
// TWRR = (15500/10000)*(16500/15500) - 1 = 1.65 - 1 = 0.65
// TWRR naively treats deposit-driven equity growth as returns.
Expect(twrrResult).To(BeNumerically("~", 0.65, 1e-9))

// MWRR = 0.0 because total deposits (16500) equal terminal value (16500).
// The investor put in exactly what they got out -- zero investment return.
Expect(result).To(BeNumerically("~", 0.0, 1e-9))
Expect(result).NotTo(BeNumerically("~", twrrResult, 0.01))
```

to:

```go
// TWRR = 0.0 because deposits are stripped from the equity changes.
// preFlowEquity at each step equals the previous equity (no market
// growth), so every sub-period return is 0.
Expect(twrrResult).To(BeNumerically("~", 0.0, 1e-9))

// MWRR = 0.0 because total deposits (16500) equal terminal value (16500).
// Both metrics agree: zero investment return.
Expect(result).To(BeNumerically("~", 0.0, 1e-9))
```

Also rename the test from `"differs from TWRR when there is a mid-stream deposit"` to `"agrees with MWRR when deposits produce no market growth"`, since both metrics now correctly report 0.0 for this all-deposit scenario. The TWRR vs MWRR divergence test (Task 7) covers the case where they differ.

- [ ] **Step 2: Run the test**

Run: `ginkgo run -race ./portfolio/ --focus="agrees with MWRR"`
Expected: PASS

- [ ] **Step 3: Commit**

```
git add portfolio/return_metrics_test.go
git commit -m "test: correct deposit-only TWRR expectation from 0.65 to 0.0"
```

---

### Task 4: Add test for growth plus deposit

Verify TWRR correctly isolates market growth from deposit-driven equity changes.

**Files:**
- Modify: `portfolio/return_metrics_test.go` (add new `It` block inside the `TWRR` Describe)

- [ ] **Step 1: Write the test**

Add after the `"returns 0 for a single data point"` test (around line 108):

```go
It("isolates market growth from deposit-driven equity changes", func() {
    // Day 0: deposit 10000, equity = 10000
    // Day 1: organic growth +1000, equity = 11000 (no deposit)
    // Day 2: deposit 5000 + organic growth +1000, equity = 17000
    //
    // Sub-period 1 (day 0->1): no flow
    //   preFlowEquity = 11000, return = 11000/10000 - 1 = 0.10
    // Sub-period 2 (day 1->2): flow = 5000 (deposit)
    //   preFlowEquity = 17000 - 5000 = 12000, return = 12000/11000 - 1
    //   subPeriodStart resets to 17000
    // TWRR = (11000/10000) * (12000/11000) - 1 = 12000/10000 - 1 = 0.20
    dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)

    aa := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
    df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df0)

    // Day 1: organic growth (dividend, invisible to TWRR flow map)
    aa.Record(portfolio.Transaction{
        Date:   dates[1],
        Type:   asset.DividendTransaction,
        Amount: 1000,
    })
    df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df1)

    // Day 2: deposit 5000 (visible to TWRR) + organic growth 1000
    aa.Record(portfolio.Transaction{
        Date:   dates[2],
        Type:   asset.DepositTransaction,
        Amount: 5000,
    })
    aa.Record(portfolio.Transaction{
        Date:   dates[2],
        Type:   asset.DividendTransaction,
        Amount: 1000,
    })
    df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df2)

    result, err := aa.PerformanceMetric(portfolio.TWRR).Value()
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(BeNumerically("~", 0.20, 1e-9))
})
```

- [ ] **Step 2: Run the test**

Run: `ginkgo run -race ./portfolio/ --focus="isolates market growth"`
Expected: PASS

- [ ] **Step 3: Commit**

```
git add portfolio/return_metrics_test.go
git commit -m "test: add TWRR test for mixed organic growth and deposit"
```

---

### Task 5: Add test for withdrawal

Verify TWRR handles withdrawals correctly (flow has negative Amount).

**Files:**
- Modify: `portfolio/return_metrics_test.go` (add new `It` block inside the `TWRR` Describe)

- [ ] **Step 1: Write the test**

Add after the growth-plus-deposit test:

```go
It("handles withdrawals correctly", func() {
    // Day 0: deposit 10000, equity = 10000
    // Day 1: market loss -1000, equity = 9000
    // Day 2: withdrawal 3000 + market gain +1000, equity = 7000
    //
    // Sub-period 1 (day 0->1): no flow
    //   preFlowEquity = 9000, return = 9000/10000 - 1 = -0.10
    // Sub-period 2 (day 1->2): flow = -3000 (withdrawal)
    //   preFlowEquity = 7000 - (-3000) = 10000, return = 10000/9000 - 1
    //   subPeriodStart resets to 7000
    // TWRR = (9000/10000) * (10000/9000) - 1 = 10000/10000 - 1 = 0.0
    dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)

    aa := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
    df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df0)

    // Day 1: market loss (fee, invisible to TWRR)
    aa.Record(portfolio.Transaction{
        Date:   dates[1],
        Type:   asset.FeeTransaction,
        Amount: -1000,
    })
    df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df1)

    // Day 2: withdrawal 3000 (visible) + market gain 1000 (dividend, invisible)
    aa.Record(portfolio.Transaction{
        Date:   dates[2],
        Type:   asset.WithdrawalTransaction,
        Amount: -3000,
    })
    aa.Record(portfolio.Transaction{
        Date:   dates[2],
        Type:   asset.DividendTransaction,
        Amount: 1000,
    })
    df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df2)

    result, err := aa.PerformanceMetric(portfolio.TWRR).Value()
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(BeNumerically("~", 0.0, 1e-9))
})
```

- [ ] **Step 2: Run the test**

Run: `ginkgo run -race ./portfolio/ --focus="handles withdrawals correctly"`
Expected: PASS

- [ ] **Step 3: Commit**

```
git add portfolio/return_metrics_test.go
git commit -m "test: add TWRR test for withdrawal scenario"
```

---

### Task 6: Add ComputeSeries test with flows

Verify the cumulative return curve is correct when cash flows are present.

**Files:**
- Modify: `portfolio/return_metrics_test.go` (add new `It` block inside the `TWRR` Describe)

- [ ] **Step 1: Write the test**

Reuse the growth-plus-deposit scenario from Task 4:

```go
It("computes cumulative series with cash flows stripped", func() {
    // Same scenario as "isolates market growth" test:
    // Day 0: equity 10000, Day 1: equity 11000 (organic), Day 2: equity 17000 (deposit 5000 + organic 1000)
    // Sub-period returns: r0 = 0.10, r1 = 12000/11000 - 1
    // cum[0] = 1.10 - 1 = 0.10
    // cum[1] = 1.10 * (12000/11000) - 1 = 12000/10000 - 1 = 0.20
    dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)

    aa := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
    df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df0)

    aa.Record(portfolio.Transaction{
        Date:   dates[1],
        Type:   asset.DividendTransaction,
        Amount: 1000,
    })
    df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df1)

    aa.Record(portfolio.Transaction{
        Date:   dates[2],
        Type:   asset.DepositTransaction,
        Amount: 5000,
    })
    aa.Record(portfolio.Transaction{
        Date:   dates[2],
        Type:   asset.DividendTransaction,
        Amount: 1000,
    })
    df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df2)

    seriesDF, err := aa.PerformanceMetric(portfolio.TWRR).Series()
    Expect(err).NotTo(HaveOccurred())
    Expect(seriesDF.Len()).To(Equal(2))
    series := seriesDF.Column(perfAsset, data.PortfolioEquity)
    Expect(series[0]).To(BeNumerically("~", 0.10, 1e-9))
    Expect(series[1]).To(BeNumerically("~", 0.20, 1e-9))
})
```

- [ ] **Step 2: Run the test**

Run: `ginkgo run -race ./portfolio/ --focus="cumulative series with cash flows"`
Expected: PASS

- [ ] **Step 3: Commit**

```
git add portfolio/return_metrics_test.go
git commit -m "test: add TWRR ComputeSeries test with cash flows"
```

---

### Task 7: Add TWRR vs MWRR divergence test

Demonstrate that TWRR and MWRR produce different results when the timing of deposits matters.

**Files:**
- Modify: `portfolio/return_metrics_test.go` (add new `It` block inside the `TWRR` Describe)

- [ ] **Step 1: Write the test**

```go
It("diverges from MWRR when deposit timing matters", func() {
    // Day 0: deposit 10000, equity = 10000
    // Day 183: market grew 20%, equity = 12000, then deposit 10000, equity = 22000
    // Day 367: market dropped 10%, equity = 19800
    //
    // TWRR sub-periods:
    //   period 1: preFlowEquity = 12000, return = 12000/10000 - 1 = 0.20
    //             subPeriodStart resets to 22000
    //   period 2: preFlowEquity = 19800, return = 19800/22000 - 1 = -0.10
    // TWRR = 1.20 * 0.90 - 1 = 0.08
    //
    // MWRR will be lower: more money was exposed to the 10% loss than
    // to the 20% gain (20000 vs 10000), so the investor's actual
    // experience is worse than what TWRR reports.
    dates := []time.Time{
        time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
        time.Date(2024, 7, 3, 0, 0, 0, 0, time.UTC),
        time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
    }

    aa := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
    df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df0)

    // Day 183: market grew 20% (organic), then deposit 10000
    aa.Record(portfolio.Transaction{
        Date:   dates[1],
        Type:   asset.DividendTransaction,
        Amount: 2000,
    })
    aa.Record(portfolio.Transaction{
        Date:   dates[1],
        Type:   asset.DepositTransaction,
        Amount: 10_000,
    })
    df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df1)

    // Day 367: market dropped 10% on 22000 = loss of 2200
    aa.Record(portfolio.Transaction{
        Date:   dates[2],
        Type:   asset.FeeTransaction,
        Amount: -2200,
    })
    df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
    aa.UpdatePrices(df2)

    twrrResult, err := aa.PerformanceMetric(portfolio.TWRR).Value()
    Expect(err).NotTo(HaveOccurred())
    Expect(twrrResult).To(BeNumerically("~", 0.08, 1e-9))

    mwrrResult, err := aa.PerformanceMetric(portfolio.MWRR).Value()
    Expect(err).NotTo(HaveOccurred())
    // MWRR should be lower than TWRR because more capital was at risk
    // during the loss period.
    Expect(mwrrResult).To(BeNumerically("<", twrrResult))
})
```

- [ ] **Step 2: Run the test**

Run: `ginkgo run -race ./portfolio/ --focus="diverges from MWRR"`
Expected: PASS

- [ ] **Step 3: Commit**

```
git add portfolio/return_metrics_test.go
git commit -m "test: add TWRR vs MWRR divergence test"
```

---

### Task 8: Run full test suite and lint

Verify nothing is broken across the entire codebase.

**Files:** None (validation only)

- [ ] **Step 1: Run full test suite**

Run: `make test`
Expected: All 22 suites PASS

- [ ] **Step 2: Run linter**

Run: `make lint`
Expected: 0 issues

- [ ] **Step 3: Fix any failures**

If any test or lint issue is found, fix it before proceeding.
