# PerformanceMetric Error Returns Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change `PerformanceMetric.Compute` and `ComputeSeries` to return `(T, error)` so metrics can report missing configuration (risk-free rate, benchmark).

**Architecture:** Update the interface, add sentinel errors, update all ~57 implementations (most mechanically add `, nil`), update query builder and aggregate methods to propagate errors, update CLI callers to log warnings on error.

**Tech Stack:** Go, Ginkgo/Gomega tests

---

## Chunk 1: Interface, Sentinel Errors, Query Builder

### Task 1: Add sentinel errors and update PerformanceMetric interface

**Files:**
- Modify: `portfolio/metric_query.go`

- [ ] **Step 1: Add sentinel error variables and update interface**

Add `"errors"` to imports. Add sentinel errors before the interface. Change `Compute` and `ComputeSeries` return types:

```go
import "errors"

var (
	ErrNoRiskFreeRate = errors.New("risk-free rate not configured")
	ErrNoBenchmark    = errors.New("benchmark not configured")
)

type PerformanceMetric interface {
	Name() string
	Description() string
	Compute(a *Account, window *Period) (float64, error)
	ComputeSeries(a *Account, window *Period) ([]float64, error)
}
```

- [ ] **Step 2: Update PerformanceMetricQuery.Value and Series**

```go
func (q PerformanceMetricQuery) Value() (float64, error) {
	return q.metric.Compute(q.account, q.window)
}

func (q PerformanceMetricQuery) Series() ([]float64, error) {
	return q.metric.ComputeSeries(q.account, q.window)
}
```

- [ ] **Step 3: Commit**

```bash
git add portfolio/metric_query.go
git commit -m "refactor(portfolio): change PerformanceMetric interface to return errors"
```

Note: the project will not compile after this commit until all implementations are updated. That is expected.

---

## Chunk 2: Risk-Free-Rate Metrics

These metrics use `a.RiskFreePrices()` and must return `ErrNoRiskFreeRate` when it is empty.

### Task 2: Update Sharpe

**Files:**
- Modify: `portfolio/sharpe.go`

- [ ] **Step 1: Update Compute signature and add error check**

```go
func (sharpe) Compute(a *Account, window *Period) (float64, error) {
	if len(a.RiskFreePrices()) == 0 {
		return 0, ErrNoRiskFreeRate
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	r := returns(eq)
	rf := returns(windowSlice(a.RiskFreePrices(), a.EquityTimes(), window))
	er := excessReturns(r, rf)

	sd := stddev(er)
	if sd == 0 {
		return 0, nil
	}

	af := annualizationFactor(a.EquityTimes())
	return mean(er) / sd * math.Sqrt(af), nil
}
```

- [ ] **Step 2: Update ComputeSeries signature**

```go
func (sharpe) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }
```

### Task 3: Update Sortino

**Files:**
- Modify: `portfolio/sortino.go`

- [ ] **Step 1: Update Compute -- add `ErrNoRiskFreeRate` check at top, return `(float64, error)`**

Same pattern as Sharpe: check `len(a.RiskFreePrices()) == 0` at top, add `, nil` to all other returns, add `, nil` to final return.

- [ ] **Step 2: Update ComputeSeries signature to return `([]float64, error)`**

### Task 4: Update SmartSharpe

**Files:**
- Modify: `portfolio/smart_sharpe.go`

- [ ] **Step 1: Same pattern -- `ErrNoRiskFreeRate` check, update returns**

### Task 5: Update SmartSortino

**Files:**
- Modify: `portfolio/smart_sortino.go`

- [ ] **Step 1: Same pattern -- `ErrNoRiskFreeRate` check, update returns**

### Task 6: Update ProbabilisticSharpe

**Files:**
- Modify: `portfolio/probabilistic_sharpe.go`

- [ ] **Step 1: Same pattern -- `ErrNoRiskFreeRate` check, update returns**

### Task 7: Update DownsideDeviation

**Files:**
- Modify: `portfolio/downside_deviation.go`

- [ ] **Step 1: Same pattern -- `ErrNoRiskFreeRate` check, update returns**

### Task 8: Commit risk-free metrics

```bash
git add portfolio/sharpe.go portfolio/sortino.go portfolio/smart_sharpe.go portfolio/smart_sortino.go portfolio/probabilistic_sharpe.go portfolio/downside_deviation.go
git commit -m "refactor(portfolio): add ErrNoRiskFreeRate to risk-free-dependent metrics"
```

---

## Chunk 3: Benchmark Metrics

These metrics use `a.BenchmarkPrices()` and must return `ErrNoBenchmark` when it is empty.

### Task 9: Update Beta

**Files:**
- Modify: `portfolio/beta.go`

- [ ] **Step 1: Update Compute -- add `ErrNoBenchmark` check at top**

```go
func (beta) Compute(a *Account, window *Period) (float64, error) {
	if len(a.BenchmarkPrices()) == 0 {
		return 0, ErrNoBenchmark
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	bm := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)

	pReturns := returns(eq)
	bReturns := returns(bm)

	if len(pReturns) == 0 || len(bReturns) == 0 {
		return 0, nil
	}

	v := variance(bReturns)
	if v == 0 {
		return 0, nil
	}

	return covariance(pReturns, bReturns) / v, nil
}
```

- [ ] **Step 2: Update ComputeSeries signature**

### Task 10: Update TrackingError

**Files:**
- Modify: `portfolio/tracking_error.go`

- [ ] **Step 1: Same pattern -- `ErrNoBenchmark` check, update returns**

### Task 11: Update InformationRatio

**Files:**
- Modify: `portfolio/information_ratio.go`

- [ ] **Step 1: Same pattern -- `ErrNoBenchmark` check, update returns**

### Task 12: Update RSquared

**Files:**
- Modify: `portfolio/r_squared.go`

- [ ] **Step 1: Same pattern -- `ErrNoBenchmark` check, update returns**

### Task 13: Update UpsideCaptureRatio

**Files:**
- Modify: `portfolio/upside_capture.go`

- [ ] **Step 1: Same pattern -- `ErrNoBenchmark` check, update returns**

### Task 14: Update DownsideCaptureRatio

**Files:**
- Modify: `portfolio/downside_capture.go`

- [ ] **Step 1: Same pattern -- `ErrNoBenchmark` check, update returns**

### Task 15: Update ActiveReturn

**Files:**
- Modify: `portfolio/active_return.go`

- [ ] **Step 1: Same pattern -- `ErrNoBenchmark` check, update both Compute and ComputeSeries returns**

### Task 16: Commit benchmark metrics

```bash
git add portfolio/beta.go portfolio/tracking_error.go portfolio/information_ratio.go portfolio/r_squared.go portfolio/upside_capture.go portfolio/downside_capture.go portfolio/active_return.go
git commit -m "refactor(portfolio): add ErrNoBenchmark to benchmark-dependent metrics"
```

---

## Chunk 4: Composite Metrics (need both or propagate errors)

### Task 17: Update Alpha (needs risk-free AND benchmark, composes Beta)

**Files:**
- Modify: `portfolio/alpha.go`

- [ ] **Step 1: Update Compute -- check both, propagate Beta error**

```go
func (alpha) Compute(a *Account, window *Period) (float64, error) {
	if len(a.RiskFreePrices()) == 0 {
		return 0, ErrNoRiskFreeRate
	}
	if len(a.BenchmarkPrices()) == 0 {
		return 0, ErrNoBenchmark
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	bm := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)
	rf := windowSlice(a.RiskFreePrices(), a.EquityTimes(), window)

	if len(eq) < 2 || len(bm) < 2 || len(rf) < 2 {
		return 0, nil
	}

	portfolioReturn := (eq[len(eq)-1] / eq[0]) - 1
	benchmarkReturn := (bm[len(bm)-1] / bm[0]) - 1
	riskFreeReturn := (rf[len(rf)-1] / rf[0]) - 1

	b, err := Beta.Compute(a, window)
	if err != nil {
		return 0, err
	}

	return portfolioReturn - (riskFreeReturn + b*(benchmarkReturn-riskFreeReturn)), nil
}
```

- [ ] **Step 2: Update ComputeSeries signature**

### Task 18: Update Treynor (needs risk-free, composes Beta)

**Files:**
- Modify: `portfolio/treynor.go`

- [ ] **Step 1: Update Compute -- check risk-free, propagate Beta error**

```go
func (treynor) Compute(a *Account, window *Period) (float64, error) {
	if len(a.RiskFreePrices()) == 0 {
		return 0, ErrNoRiskFreeRate
	}

	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	rf := windowSlice(a.RiskFreePrices(), a.EquityTimes(), window)

	if len(eq) < 2 || len(rf) < 2 {
		return 0, nil
	}

	portfolioReturn := (eq[len(eq)-1] / eq[0]) - 1
	riskFreeReturn := (rf[len(rf)-1] / rf[0]) - 1

	b, err := Beta.Compute(a, window)
	if err != nil {
		return 0, err
	}
	if b == 0 {
		return 0, nil
	}

	return (portfolioReturn - riskFreeReturn) / b, nil
}
```

- [ ] **Step 2: Update ComputeSeries signature**

### Task 19: Commit composite metrics

```bash
git add portfolio/alpha.go portfolio/treynor.go
git commit -m "refactor(portfolio): add error propagation to Alpha and Treynor"
```

---

## Chunk 5: Mechanical Updates (no-error metrics)

All remaining metrics just need `(float64, error)` / `([]float64, error)` signatures with `, nil` appended to every return statement. No new error conditions.

### Task 20: Update all remaining metric files

**Files (37 files):**
- Modify: `portfolio/avg_drawdown_days.go`
- Modify: `portfolio/avg_drawdown.go`
- Modify: `portfolio/cagr_metric.go`
- Modify: `portfolio/calmar.go`
- Modify: `portfolio/consecutive_losses.go`
- Modify: `portfolio/consecutive_wins.go`
- Modify: `portfolio/cvar.go`
- Modify: `portfolio/dynamic_withdrawal_rate.go`
- Modify: `portfolio/excess_kurtosis.go`
- Modify: `portfolio/exposure.go`
- Modify: `portfolio/gain_loss_ratio.go`
- Modify: `portfolio/gain_to_pain.go`
- Modify: `portfolio/k_ratio.go`
- Modify: `portfolio/keller_ratio.go`
- Modify: `portfolio/kelly_criterion.go`
- Modify: `portfolio/ltcg.go`
- Modify: `portfolio/max_drawdown.go`
- Modify: `portfolio/mwrr.go`
- Modify: `portfolio/n_positive_periods.go`
- Modify: `portfolio/non_qualified_income.go`
- Modify: `portfolio/omega_ratio.go`
- Modify: `portfolio/perpetual_withdrawal_rate.go`
- Modify: `portfolio/profit_factor_metric.go`
- Modify: `portfolio/qualified_dividends.go`
- Modify: `portfolio/recovery_factor.go`
- Modify: `portfolio/safe_withdrawal_rate.go`
- Modify: `portfolio/skewness.go`
- Modify: `portfolio/stcg.go`
- Modify: `portfolio/std_dev.go`
- Modify: `portfolio/tail_ratio.go`
- Modify: `portfolio/tax_cost_ratio.go`
- Modify: `portfolio/trade_gain_loss_ratio.go`
- Modify: `portfolio/turnover.go`
- Modify: `portfolio/twrr.go`
- Modify: `portfolio/ulcer_index.go`
- Modify: `portfolio/unrealized_ltcg.go`
- Modify: `portfolio/unrealized_stcg.go`
- Modify: `portfolio/value_at_risk.go`
- Modify: `portfolio/win_rate.go`
- Modify: `portfolio/average_holding_period.go`
- Modify: `portfolio/average_loss.go`
- Modify: `portfolio/average_win.go`

For each file, the change is mechanical:
1. `Compute(a *Account, window *Period) float64` becomes `Compute(a *Account, window *Period) (float64, error)`
2. `ComputeSeries(a *Account, window *Period) []float64` becomes `ComputeSeries(a *Account, window *Period) ([]float64, error)`
3. Every `return X` in Compute becomes `return X, nil`
4. Every `return X` in ComputeSeries becomes `return X, nil`

- [ ] **Step 1: Update all 42 files with mechanical signature changes**

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./portfolio/...`
Expected: may still fail due to callers, but all metric files should be clean.

- [ ] **Step 3: Commit**

```bash
git add portfolio/
git commit -m "refactor(portfolio): update remaining metrics to return (T, error)"
```

---

## Chunk 6: Aggregate Methods and Portfolio Interface

### Task 21: Update aggregate methods on Account

**Files:**
- Modify: `portfolio/account.go`
- Modify: `portfolio/portfolio.go`
- Modify: `portfolio/summary.go` (if Summary/RiskMetrics/etc. structs need changes -- they don't, just the methods)

- [ ] **Step 1: Update Portfolio interface in `portfolio/portfolio.go`**

Change the five aggregate method signatures:

```go
Summary() (Summary, error)
RiskMetrics() (RiskMetrics, error)
TaxMetrics() (TaxMetrics, error)
TradeMetrics() (TradeMetrics, error)
WithdrawalMetrics() (WithdrawalMetrics, error)
```

Also update `PerformanceMetric` method doc to note that `Value()` now returns error:

```go
//   v, err := p.PerformanceMetric(Sharpe).Window(Months(36)).Value()
```

- [ ] **Step 2: Update Account.Summary() in `portfolio/account.go`**

```go
func (a *Account) Summary() (Summary, error) {
	var errs []error
	s := Summary{}
	var err error

	s.TWRR, err = a.PerformanceMetric(TWRR).Value()
	if err != nil { errs = append(errs, err) }

	s.MWRR, err = a.PerformanceMetric(MWRR).Value()
	if err != nil { errs = append(errs, err) }

	s.Sharpe, err = a.PerformanceMetric(Sharpe).Value()
	if err != nil { errs = append(errs, err) }

	s.Sortino, err = a.PerformanceMetric(Sortino).Value()
	if err != nil { errs = append(errs, err) }

	s.Calmar, err = a.PerformanceMetric(Calmar).Value()
	if err != nil { errs = append(errs, err) }

	s.MaxDrawdown, err = a.PerformanceMetric(MaxDrawdown).Value()
	if err != nil { errs = append(errs, err) }

	s.StdDev, err = a.PerformanceMetric(StdDev).Value()
	if err != nil { errs = append(errs, err) }

	return s, errors.Join(errs...)
}
```

- [ ] **Step 3: Update Account.RiskMetrics() -- same pattern with `errors.Join`**

- [ ] **Step 4: Update Account.TaxMetrics() -- same pattern**

- [ ] **Step 5: Update Account.TradeMetrics() -- same pattern**

- [ ] **Step 6: Update Account.WithdrawalMetrics() -- same pattern**

- [ ] **Step 7: Add `"errors"` to imports in `portfolio/account.go`**

- [ ] **Step 8: Commit**

```bash
git add portfolio/account.go portfolio/portfolio.go
git commit -m "refactor(portfolio): update aggregate methods and Portfolio interface to return errors"
```

---

## Chunk 7: CLI Callers

### Task 22: Update CLI summary and output code

**Files:**
- Modify: `cli/summary.go`
- Modify: `cli/output_jsonl.go`
- Modify: `cli/output_parquet.go`

- [ ] **Step 1: Update `cli/summary.go`**

Add `"github.com/rs/zerolog/log"` import. Handle errors from aggregate methods by logging warnings:

```go
func printSummary(acct portfolio.Portfolio) {
	s, err := acct.Summary()
	if err != nil {
		log.Warn().Err(err).Msg("some summary metrics unavailable")
	}
	r, err := acct.RiskMetrics()
	if err != nil {
		log.Warn().Err(err).Msg("some risk metrics unavailable")
	}
	t, err := acct.TradeMetrics()
	if err != nil {
		log.Warn().Err(err).Msg("some trade metrics unavailable")
	}
	w, err := acct.WithdrawalMetrics()
	if err != nil {
		log.Warn().Err(err).Msg("some withdrawal metrics unavailable")
	}
	// ... rest unchanged, struct fields are zero when metric errored
```

- [ ] **Step 2: Update `cli/output_jsonl.go` -- same pattern for writeMetrics function**

- [ ] **Step 3: Update `cli/output_parquet.go` -- same pattern if it calls aggregate methods**

- [ ] **Step 4: Verify compilation**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./...`
Expected: PASS (all callers updated)

- [ ] **Step 5: Commit**

```bash
git add cli/
git commit -m "refactor(cli): handle metric errors with log warnings"
```

---

## Chunk 8: Test Updates

### Task 23: Update test files that call .Value()

**Files (22 test files):**
- Modify: `portfolio/recovery_factor_test.go`
- Modify: `portfolio/avg_drawdown_days_test.go`
- Modify: `portfolio/smart_sharpe_test.go`
- Modify: `portfolio/benchmark_metrics_test.go`
- Modify: `portfolio/account_test.go`
- Modify: `portfolio/tail_ratio_test.go`
- Modify: `portfolio/cagr_metric_test.go`
- Modify: `portfolio/omega_ratio_test.go`
- Modify: `portfolio/kelly_criterion_test.go`
- Modify: `portfolio/specialized_metrics_test.go`
- Modify: `portfolio/smart_sortino_test.go`
- Modify: `portfolio/capture_drawdown_metrics_test.go`
- Modify: `portfolio/consecutive_losses_test.go`
- Modify: `portfolio/exposure_test.go`
- Modify: `portfolio/gain_to_pain_test.go`
- Modify: `portfolio/rebalance_test.go`
- Modify: `portfolio/cvar_test.go`
- Modify: `portfolio/distribution_metrics_test.go`
- Modify: `portfolio/return_metrics_test.go`
- Modify: `portfolio/risk_adjusted_metrics_test.go`
- Modify: `portfolio/consecutive_wins_test.go`
- Modify: `portfolio/probabilistic_sharpe_test.go`
- Modify: `portfolio/withdrawal_metrics_test.go`

- [ ] **Step 1: Update all `.Value()` calls in tests**

Pattern: change `v := a.PerformanceMetric(M).Value()` to:

```go
v, err := a.PerformanceMetric(M).Value()
Expect(err).NotTo(HaveOccurred())
```

For tests calling `.Compute()` directly (withdrawal_metrics_test.go), change:

```go
Expect(portfolio.SafeWithdrawalRate.Compute(a, nil)).To(Equal(0.0))
```

to:

```go
v, err := portfolio.SafeWithdrawalRate.Compute(a, nil)
Expect(err).NotTo(HaveOccurred())
Expect(v).To(Equal(0.0))
```

- [ ] **Step 2: Update tests calling `.Summary()`, `.RiskMetrics()`, etc.**

Add `err` handling for aggregate method calls in `portfolio/account_test.go`.

- [ ] **Step 3: Run tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/... -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add portfolio/*_test.go
git commit -m "test(portfolio): update tests for error-returning metric interface"
```

### Task 24: Update engine and CLI test files

**Files:**
- Modify: `engine/backtest_test.go` (if it references metrics)
- Modify: `engine/live_test.go` (if it references metrics)
- Modify: `cli/cli_test.go` (if it references metrics)

- [ ] **Step 1: Check and update any engine/CLI tests that call .Value() or .Compute()**

- [ ] **Step 2: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./... -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add engine/ cli/
git commit -m "test: update engine and CLI tests for metric error returns"
```

---

## Chunk 9: Error Path Tests

### Task 25: Add tests for missing risk-free rate errors

**Files:**
- Modify: `portfolio/risk_adjusted_metrics_test.go` (or whichever file tests Sharpe/Sortino)

- [ ] **Step 1: Add test cases for ErrNoRiskFreeRate**

Create an Account with no risk-free data configured. Assert that Sharpe, Sortino, SmartSharpe, SmartSortino, ProbabilisticSharpe, DownsideDeviation, Treynor, and Alpha all return `ErrNoRiskFreeRate`:

```go
It("returns ErrNoRiskFreeRate when risk-free rate is not configured", func() {
    a := buildAccountWithoutRiskFree() // helper that sets up equity curve but no risk-free prices
    for _, m := range []portfolio.PerformanceMetric{
        portfolio.Sharpe,
        portfolio.Sortino,
        portfolio.SmartSharpe,
        portfolio.SmartSortino,
        portfolio.ProbabilisticSharpe,
        portfolio.DownsideDeviation,
        portfolio.Treynor,
        portfolio.Alpha,
    } {
        v, err := m.Compute(a, nil)
        Expect(err).To(MatchError(portfolio.ErrNoRiskFreeRate), m.Name())
        Expect(v).To(Equal(0.0))
    }
})
```

- [ ] **Step 2: Run test**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/... -run ErrNoRiskFreeRate -v`
Expected: PASS

### Task 26: Add tests for missing benchmark errors

- [ ] **Step 1: Add test cases for ErrNoBenchmark**

Same pattern: Account with no benchmark data. Assert Beta, TrackingError, InformationRatio, RSquared, UpsideCaptureRatio, DownsideCaptureRatio, ActiveReturn, and Alpha all return `ErrNoBenchmark`:

```go
It("returns ErrNoBenchmark when benchmark is not configured", func() {
    a := buildAccountWithoutBenchmark()
    for _, m := range []portfolio.PerformanceMetric{
        portfolio.Beta,
        portfolio.TrackingError,
        portfolio.InformationRatio,
        portfolio.RSquared,
        portfolio.UpsideCaptureRatio,
        portfolio.DownsideCaptureRatio,
        portfolio.ActiveReturn,
    } {
        v, err := m.Compute(a, nil)
        Expect(err).To(MatchError(portfolio.ErrNoBenchmark), m.Name())
        Expect(v).To(Equal(0.0))
    }
})
```

Note: Alpha should be tested separately since it checks risk-free first.

### Task 27: Add test for aggregate method error collection

- [ ] **Step 1: Test that Summary() returns partial results with joined error**

```go
It("returns partial Summary with joined errors when risk-free is missing", func() {
    a := buildAccountWithoutRiskFree()
    s, err := a.Summary()
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("risk-free rate not configured"))
    // TWRR, MWRR, Calmar, MaxDrawdown, StdDev should still have values
    Expect(s.TWRR).NotTo(Equal(0.0))
    // Sharpe, Sortino should be zero
    Expect(s.Sharpe).To(Equal(0.0))
    Expect(s.Sortino).To(Equal(0.0))
})
```

- [ ] **Step 2: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./... -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add portfolio/
git commit -m "test(portfolio): add error path tests for missing risk-free and benchmark"
```
