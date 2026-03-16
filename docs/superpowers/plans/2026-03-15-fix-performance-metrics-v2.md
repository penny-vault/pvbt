# Fix Performance Metrics Implementation Plan (v2)

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix risk-free rate handling, Treynor annualization, and withdrawal metrics by making DGS3MO the hardcoded system-level risk-free rate with proper yield-to-return conversion.

**Architecture:** The engine resolves DGS3MO during initialization, fetches the yield alongside other assets, converts it to a cumulative price-equivalent, and passes the converted value to the Account via `SetRiskFreeValue()`. The Account stores this value in perfData so downstream `Pct()` calls produce correct daily risk-free returns. Public RF configuration API is deprecated. Treynor switches to CAGR. Withdrawal metrics use deterministic actual-path simulation.

**Tech Stack:** Go, Ginkgo/Gomega, gonum/stat, tradecron

**Spec:** `docs/superpowers/specs/2026-03-15-fix-performance-metrics-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `portfolio/account.go` | Modify | Remove public RF API; add `SetRiskFreeValue()`; UpdatePrices uses stored value |
| `portfolio/account_test.go` | Modify | Update Summary/RiskMetrics tests for new RF flow |
| `portfolio/testutil_test.go` | Modify | Update helpers to use `SetRiskFreeValue()` |
| `portfolio/benchmark_metrics_test.go` | Modify | Update `benchAcct` helper |
| `portfolio/risk_adjusted_metrics_test.go` | Modify | Update `buildAccount` helper |
| `engine/engine.go` | Modify | Add DGS3MO resolution + yield conversion; deprecate `RiskFreeAsset()` |
| `engine/backtest.go` | Modify | Fetch DGS3MO in step loop; convert yield; call `SetRiskFreeValue()` |
| `engine/live.go` | Modify | Same as backtest |
| `engine/backtest_test.go` | Modify | Update test expectations |
| `engine/descriptor.go` | Modify | Hardcode "DGS3MO" in RiskFree field |
| `engine/descriptor_test.go` | Modify | Update expectations |
| `engine/doc.go` | Modify | Update documentation |
| `portfolio/treynor.go` | Modify | Annualize using CAGR |
| `portfolio/safe_withdrawal_rate.go` | Rewrite | Deterministic actual-path simulation |
| `portfolio/perpetual_withdrawal_rate.go` | Rewrite | Inflation-adjusted success criterion |
| `portfolio/dynamic_withdrawal_rate.go` | Rewrite | Dynamic adjustment |
| `portfolio/metric_helpers.go` | Modify | Add `yieldToCumulative()` helper |
| `portfolio/withdrawal_metrics_test.go` | Rewrite | Tests for actual-path SWR |
| `portfolio/sqlite.go` | Modify | Write "DGS3MO" unconditionally; ignore RF on restore |
| `examples/momentum-rotation/main.go` | Modify | Remove `RiskFreeAsset()` call |
| `CHANGELOG.md` | Modify | Document changes |

---

## Chunk 1: Account Risk-Free Refactor

### Task 1: Refactor Account to use SetRiskFreeValue instead of asset-based RF

The Account currently stores a risk-free asset and reads its price from the DataFrame in UpdatePrices. Replace this with a simple float64 value that the engine sets before each UpdatePrices call. This decouples the Account from risk-free data format concerns.

**Files:**
- Modify: `portfolio/account.go`
- Modify: `portfolio/testutil_test.go`
- Modify: `portfolio/benchmark_metrics_test.go`
- Modify: `portfolio/risk_adjusted_metrics_test.go`
- Modify: `portfolio/account_test.go`
- Modify: `portfolio/sqlite.go`

- [ ] **Step 1: Update test helpers to use SetRiskFreeValue**

In `portfolio/testutil_test.go`, update `buildAccountWithRF` to call `SetRiskFreeValue()` before each `UpdatePrices()` instead of configuring a risk-free asset:

```go
func buildAccountWithRF(spyPrices, bilPrices []float64) *portfolio.Account {
	spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	n := len(spyPrices)
	times := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), n)

	acct := portfolio.New(
		portfolio.WithCash(5*spyPrices[0], time.Time{}),
		portfolio.WithBenchmark(spy),
	)

	acct.Record(portfolio.Transaction{
		Date:   times[0],
		Asset:  spy,
		Type:   portfolio.BuyTransaction,
		Qty:    5,
		Price:  spyPrices[0],
		Amount: -5 * spyPrices[0],
	})

	for i := range n {
		acct.SetRiskFreeValue(bilPrices[i])
		df := buildDF(times[i],
			[]asset.Asset{spy},
			[]float64{spyPrices[i]},
			[]float64{spyPrices[i]},
		)
		acct.UpdatePrices(df)
	}

	return acct
}
```

Similarly update `benchAcct` in `portfolio/benchmark_metrics_test.go`:
```go
func benchAcct(eqCurve, bmPrices, rfPrices []float64) *portfolio.Account {
	bm := asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}

	a := portfolio.New(
		portfolio.WithCash(eqCurve[0], time.Time{}),
		portfolio.WithBenchmark(bm),
	)

	dates := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), len(eqCurve))

	for i := range eqCurve {
		if i > 0 {
			diff := eqCurve[i] - eqCurve[i-1]
			if diff > 0 {
				a.Record(portfolio.Transaction{
					Date:   dates[i],
					Type:   portfolio.DepositTransaction,
					Amount: diff,
				})
			} else {
				a.Record(portfolio.Transaction{
					Date:   dates[i],
					Type:   portfolio.WithdrawalTransaction,
					Amount: diff,
				})
			}
		}

		a.SetRiskFreeValue(rfPrices[i])
		df := buildDF(dates[i],
			[]asset.Asset{bm},
			[]float64{bmPrices[i]},
			[]float64{bmPrices[i]},
		)
		a.UpdatePrices(df)
	}

	return a
}
```

Update `buildAccount` in `portfolio/risk_adjusted_metrics_test.go` the same way -- remove `WithRiskFree(bil)`, call `acct.SetRiskFreeValue(bilPrices[i])` before each `UpdatePrices`.

Update `buildSummaryAcct` and `buildRiskAcct` in `portfolio/account_test.go` similarly.

- [ ] **Step 2: Refactor Account struct and UpdatePrices**

In `portfolio/account.go`:

1. Remove the `riskFree asset.Asset` field from the Account struct.
2. Add `riskFreeValue float64` field (default 0; 0 means no RF data configured).
3. Remove `WithRiskFree()` option function.
4. Remove `SetRiskFree()` method.
5. Remove `RiskFree()` getter.
6. Add `SetRiskFreeValue(v float64)` method:
   ```go
   // SetRiskFreeValue sets the risk-free cumulative value for the next
   // UpdatePrices call. The engine calls this with a yield-derived
   // cumulative series; test code can pass price-like values directly.
   func (a *Account) SetRiskFreeValue(v float64) {
   	a.riskFreeValue = v
   }
   ```
7. In `UpdatePrices`, replace the riskFree asset reading block (lines 690-697) with:
   ```go
   rfVal = a.riskFreeValue
   ```
8. Update `Clone()` to copy `riskFreeValue` instead of `riskFree`.

- [ ] **Step 3: Update sqlite.go**

In `portfolio/sqlite.go`:

1. In `writeMetadata` (around line 235): replace the conditional `if a.riskFree != (asset.Asset{})` block with unconditional writes:
   ```go
   // Risk-free identity (always DGS3MO).
   if _, err := stmt.Exec("risk_free_ticker", "DGS3MO"); err != nil {
   	return fmt.Errorf("insert risk_free_ticker: %w", err)
   }
   if _, err := stmt.Exec("risk_free_figi", ""); err != nil {
   	return fmt.Errorf("insert risk_free_figi: %w", err)
   }
   ```

2. In `FromSQLite` (around line 487): remove the block that restores `acct.riskFree` from metadata. Keep the `delete` calls to clean up the metadata map.

- [ ] **Step 4: Update ErrNoRiskFreeRate checks**

All metrics that check for the risk-free rate currently do:
```go
rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
if len(rfCol) == 0 || rfCol[0] == 0 {
    return 0, ErrNoRiskFreeRate
}
```

This check still works: if `riskFreeValue` was never set (stays 0), perfData stores 0 for the RF column, and `rfCol[0] == 0` triggers `ErrNoRiskFreeRate`. No change needed to metric consumers.

- [ ] **Step 5: Run tests and fix any remaining breakage**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -count=1
```

Fix any compilation errors or test failures from the refactor. The metric values should be identical to before since the test helpers pass the same BIL price values through `SetRiskFreeValue()`.

- [ ] **Step 6: Commit**

```bash
git add portfolio/account.go portfolio/testutil_test.go portfolio/benchmark_metrics_test.go portfolio/risk_adjusted_metrics_test.go portfolio/account_test.go portfolio/sqlite.go
git commit -m "refactor: decouple Account from risk-free asset; add SetRiskFreeValue"
```

---

## Chunk 2: Engine DGS3MO Integration

### Task 2: Engine resolves DGS3MO and converts yield to cumulative

The engine automatically resolves DGS3MO during initialization, includes it in the existing fetch lists, converts the yield to a cumulative value each step, and passes it to the Account via `SetRiskFreeValue()`.

**Files:**
- Modify: `engine/engine.go`
- Modify: `engine/backtest.go`
- Modify: `engine/live.go`
- Modify: `engine/descriptor.go`
- Modify: `engine/descriptor_test.go`
- Modify: `engine/doc.go`
- Modify: `examples/momentum-rotation/main.go`
- Modify: `portfolio/metric_helpers.go`

- [ ] **Step 1: Add yield-to-cumulative helper**

In `portfolio/metric_helpers.go`, add:

```go
// YieldToCumulative converts an annualized yield percentage to the next
// value in a cumulative price-equivalent series. For example, a yield of
// 5.25 (meaning 5.25% annual) produces a daily return of
// (1 + 0.0525)^(1/252) - 1, and the cumulative series grows by that factor.
//
// Pass prevCumulative=0 on the first call; it returns 100.0 as the
// starting value. On subsequent calls, it returns
// prevCumulative * (1 + dailyReturn).
func YieldToCumulative(annualYieldPct, prevCumulative float64) float64 {
	if annualYieldPct <= 0 {
		if prevCumulative == 0 {
			return 100.0
		}
		return prevCumulative
	}

	dailyReturn := math.Pow(1+annualYieldPct/100, 1.0/252) - 1

	if prevCumulative == 0 {
		return 100.0
	}

	return prevCumulative * (1 + dailyReturn)
}
```

Export this function since the engine package needs to call it.

- [ ] **Step 2: Add DGS3MO state to Engine struct**

In `engine/engine.go`, add fields to the Engine struct:

```go
// Risk-free rate (DGS3MO) state.
riskFreeResolved   bool
riskFreeAssetDGS   asset.Asset
riskFreeCumulative float64
```

Deprecate `RiskFreeAsset()`:
```go
// RiskFreeAsset is deprecated. The engine automatically uses DGS3MO as the
// risk-free rate. This method logs a warning and ignores the argument.
// It will be removed in a future version.
func (e *Engine) RiskFreeAsset(a asset.Asset) {
	log.Warn().Str("ticker", a.Ticker).Msg("RiskFreeAsset is deprecated; engine uses DGS3MO automatically")
}
```

Remove the `riskFree asset.Asset` field from the Engine struct.

- [ ] **Step 3: Add DGS3MO resolution to backtest init**

In `engine/backtest.go`, after `e.strategy.Setup(e)` and before account creation, add DGS3MO resolution:

```go
// Resolve DGS3MO as the system risk-free rate.
dgs3mo, rfErr := e.assetProvider.LookupAsset(ctx, "DGS3MO")
if rfErr != nil {
	zerolog.Ctx(ctx).Warn().Msg("risk-free rate data (DGS3MO) not available, using 0%")
} else {
	e.riskFreeResolved = true
	e.riskFreeAssetDGS = dgs3mo
}
e.riskFreeCumulative = 0
```

Remove the `acct.SetRiskFree(e.riskFree)` block. Remove all `e.riskFree` references in the step loop (housekeep asset list, price asset list).

In the step loop, after the housekeeping fetch and dividend recording, add DGS3MO yield conversion:

```go
// Convert DGS3MO yield to cumulative risk-free value.
if e.riskFreeResolved {
	rfDF, rfFetchErr := e.FetchAt(stepCtx, []asset.Asset{e.riskFreeAssetDGS}, date, []data.Metric{data.MetricClose})
	if rfFetchErr == nil {
		yield := rfDF.Value(e.riskFreeAssetDGS, data.MetricClose)
		if !math.IsNaN(yield) && yield > 0 {
			e.riskFreeCumulative = portfolio.YieldToCumulative(yield, e.riskFreeCumulative)
		} else {
			// Forward-fill: keep previous cumulative value.
			if e.riskFreeCumulative == 0 {
				e.riskFreeCumulative = 100.0
			}
		}
	}
}

acct.SetRiskFreeValue(e.riskFreeCumulative)
```

Place this BEFORE the `acct.UpdatePrices(priceDF)` call so the value is set before it gets stored in perfData.

- [ ] **Step 4: Apply same changes to live.go**

Mirror the DGS3MO resolution and step-loop conversion in `engine/live.go`.

- [ ] **Step 5: Update descriptor and examples**

In `engine/descriptor.go`, replace:
```go
if eng.riskFree != (asset.Asset{}) {
	info.RiskFree = eng.riskFree.Ticker
}
```
with:
```go
info.RiskFree = "DGS3MO"
```

In `engine/descriptor_test.go`, update the expectation from `"SHV"` to `"DGS3MO"`. Remove the `eng.RiskFreeAsset(asset.Asset{Ticker: "SHV"})` call from the test strategy setup.

In `examples/momentum-rotation/main.go`, remove:
```go
eng.RiskFreeAsset(eng.Asset("SHV"))
```

In `engine/doc.go`, remove `e.RiskFreeAsset(e.Asset("DGS3MO"))` from the example and add a comment explaining the risk-free rate is automatic.

- [ ] **Step 6: Run all tests**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ ./portfolio/ -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add engine/ portfolio/metric_helpers.go examples/
git commit -m "feat: hardcode DGS3MO as system risk-free rate with yield-to-cumulative conversion"
```

---

## Chunk 3: Treynor Annualization

### Task 3: Annualize Treynor using CAGR

**Files:**
- Modify: `portfolio/treynor.go`
- Modify: `portfolio/benchmark_metrics_test.go` (Treynor test expectations)
- Modify: `portfolio/account_test.go` (Treynor test expectations)

- [ ] **Step 1: Update Treynor tests with annualized expectations**

Compute expected annualized Treynor for the existing test data. The test uses `benchAcct` with equity [1000,1050,1020,1080,1060,1100] and RF prices [100,100.01,...,100.05]. The Treynor test expects `(0.10 - 0.0005) / 1.027594 = 0.096828`.

The annualized version:
```
years = 7 / 365.25 = 0.01916  (daySeq gives Jan 2-9)
CAGR_portfolio = (1100/1000)^(1/0.01916) - 1  (very large for short period)
CAGR_rf = (100.05/100)^(1/0.01916) - 1
```

For the 6-point test data this produces extreme values. The < 30 day guard should return 0. Update the test to expect 0 for this short dataset.

For the account_test.go Treynor test ("Treynor equals (TWRR - rf_return) / beta ~ 0.0995"), the same 6-point data applies -- expect 0.

- [ ] **Step 2: Rewrite treynor.go Compute method**

```go
func (treynor) Compute(acct *Account, window *Period) (float64, error) {
	pd := acct.PerfData()
	if pd == nil {
		return 0, nil
	}

	rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
	if len(rfCol) == 0 || rfCol[0] == 0 {
		return 0, ErrNoRiskFreeRate
	}

	perfDF := pd.Window(window)
	eq := perfDF.Metrics(data.PortfolioEquity)
	eqCol := eq.Column(portfolioAsset, data.PortfolioEquity)
	rfWinCol := perfDF.Column(portfolioAsset, data.PortfolioRiskFree)

	if len(eqCol) < 2 || len(rfWinCol) < 2 {
		return 0, nil
	}

	// Compute time span in years.
	times := perfDF.Times()
	calendarDays := times[len(times)-1].Sub(times[0]).Hours() / 24
	if calendarDays < 30 {
		return 0, nil
	}
	years := calendarDays / 365.25

	// Annualize returns using CAGR.
	cagrPortfolio := math.Pow(eqCol[len(eqCol)-1]/eqCol[0], 1/years) - 1
	cagrRiskFree := math.Pow(rfWinCol[len(rfWinCol)-1]/rfWinCol[0], 1/years) - 1

	betaValue, err := Beta.Compute(acct, window)
	if err != nil {
		return 0, err
	}

	if betaValue == 0 {
		return 0, nil
	}

	return (cagrPortfolio - cagrRiskFree) / betaValue, nil
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Treynor|treynor" -v -count=1
```

- [ ] **Step 4: Commit**

```bash
git add portfolio/treynor.go portfolio/benchmark_metrics_test.go portfolio/account_test.go
git commit -m "fix: annualize Treynor ratio using CAGR"
```

---

## Chunk 4: Withdrawal Metrics Rewrite

### Task 4: Replace Monte Carlo with deterministic actual-path simulation

**Files:**
- Rewrite: `portfolio/safe_withdrawal_rate.go`
- Rewrite: `portfolio/perpetual_withdrawal_rate.go`
- Rewrite: `portfolio/dynamic_withdrawal_rate.go`
- Modify: `portfolio/metric_helpers.go` (remove `monthlyReturnsFromEquity`)
- Rewrite: `portfolio/withdrawal_metrics_test.go`

- [ ] **Step 1: Write new withdrawal tests**

Replace the existing tests in `portfolio/withdrawal_metrics_test.go` with tests for the deterministic actual-path approach. Key test cases:

1. **Flat equity curve**: 0% growth over 3 years. Any withdrawal depletes the portfolio. SWR should be very low (just enough to not run out in 3 years = ~33% minus inflation adjustments).
2. **Steady growth curve**: 8% annual growth over 5 years. SWR should be moderate.
3. **Short backtest (< 1 year)**: Should return 0 (no full year to withdraw from).
4. **Ordering invariant**: PWR <= SWR <= DWR.
5. **Declining curve**: SWR should be 0.

- [ ] **Step 2: Add inflation constant and withdrawal simulation helper**

In `portfolio/metric_helpers.go`, remove `monthlyReturnsFromEquity` and add:

```go
// defaultInflationRate is the assumed annual inflation rate for withdrawal
// metric calculations.
const defaultInflationRate = 0.03

// withdrawalSimulation tests whether a given annual withdrawal rate is
// sustainable over the actual return path. It returns true if the portfolio
// survives (balance > 0) for the entire period.
//
// Parameters:
//   - equity: daily equity curve (absolute values, not returns)
//   - rate: annual withdrawal rate as a fraction (e.g., 0.04 for 4%)
//   - dynamic: if true, each year's withdrawal is min(inflated initial, balance*rate)
//   - criterion: function that checks the final balance for success
func withdrawalSimulation(
	equity []float64,
	times []time.Time,
	rate float64,
	dynamic bool,
	criterion func(startBalance, endBalance, inflationFactor float64) bool,
) bool {
	if len(equity) < 2 || len(times) < 2 {
		return false
	}

	startBalance := equity[0]
	balance := startBalance
	startDate := times[0]
	yearBoundary := startDate.AddDate(1, 0, 0)
	yearsElapsed := 0

	for dayIdx := 1; dayIdx < len(equity); dayIdx++ {
		// Apply daily return.
		dailyReturn := (equity[dayIdx] - equity[dayIdx-1]) / equity[dayIdx-1]
		balance *= (1 + dailyReturn)

		// Check for year boundary.
		if !times[dayIdx].Before(yearBoundary) {
			yearsElapsed++
			inflationFactor := math.Pow(1+defaultInflationRate, float64(yearsElapsed))
			withdrawal := rate * startBalance * inflationFactor

			if dynamic {
				currentRateWithdrawal := balance * rate
				if currentRateWithdrawal < withdrawal {
					withdrawal = currentRateWithdrawal
				}
			}

			balance -= withdrawal
			if balance <= 0 {
				return false
			}

			yearBoundary = startDate.AddDate(yearsElapsed+1, 0, 0)
		}
	}

	// Check final criterion.
	totalYears := times[len(times)-1].Sub(times[0]).Hours() / 24 / 365.25
	inflationFactor := math.Pow(1+defaultInflationRate, totalYears)
	return criterion(startBalance, balance, inflationFactor)
}
```

- [ ] **Step 3: Rewrite safe_withdrawal_rate.go**

```go
func (safeWithdrawalRate) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}

	windowed := pd.Window(window)
	equity := windowed.Column(portfolioAsset, data.PortfolioEquity)
	times := windowed.Times()

	// Need at least 1 year of data for a meaningful withdrawal rate.
	if len(equity) < 2 || len(times) < 2 {
		return 0, nil
	}
	calendarDays := times[len(times)-1].Sub(times[0]).Hours() / 24
	if calendarDays < 365 {
		return 0, nil
	}

	// Survival criterion: balance never reaches zero (checked in-loop).
	criterion := func(startBalance, endBalance, inflationFactor float64) bool {
		return true
	}

	// Linear scan from 0.1% to 20%.
	bestRate := 0.0
	for rateBps := 1; rateBps <= 200; rateBps++ {
		rate := float64(rateBps) / 1000.0
		if withdrawalSimulation(equity, times, rate, false, criterion) {
			bestRate = rate
		} else {
			break
		}
	}

	return bestRate, nil
}
```

- [ ] **Step 4: Rewrite perpetual_withdrawal_rate.go**

Same structure but with inflation-adjusted success criterion:
```go
criterion := func(startBalance, endBalance, inflationFactor float64) bool {
	return endBalance >= startBalance*inflationFactor
}
```

- [ ] **Step 5: Rewrite dynamic_withdrawal_rate.go**

Same structure but with `dynamic: true`:
```go
criterion := func(startBalance, endBalance, inflationFactor float64) bool {
	return true // survival checked in-loop
}
// ... withdrawalSimulation(equity, times, rate, true, criterion)
```

- [ ] **Step 6: Run withdrawal tests**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Withdrawal" -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add portfolio/safe_withdrawal_rate.go portfolio/perpetual_withdrawal_rate.go portfolio/dynamic_withdrawal_rate.go portfolio/metric_helpers.go portfolio/withdrawal_metrics_test.go
git commit -m "fix: rewrite withdrawal metrics to use deterministic actual-path simulation"
```

---

## Chunk 5: Documentation and Final Verification

### Task 5: Update docs, changelog, and run full test suite

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `engine/doc.go`

- [ ] **Step 1: Update CHANGELOG.md**

Add entries to the Unreleased section following the existing style (active voice, imperative mood, user-facing impact):

```markdown
### Changed
- Use DGS3MO (3-month Treasury yield) as the system risk-free rate for all
  performance metrics; the rate is no longer strategy-configurable
- Compute annualization factor from actual observation frequency instead of
  hardcoding 252 or 12
- Record daily equity on every trading day regardless of strategy schedule
- Compute Jensen's alpha from mean periodic excess returns instead of total
  cumulative returns
- Annualize Treynor ratio using CAGR instead of total returns
- Compute withdrawal rates (Safe, Perpetual, Dynamic) from the actual return
  path instead of Monte Carlo bootstrap simulation

### Deprecated
- `engine.RiskFreeAsset()` -- the engine now uses DGS3MO automatically;
  calling this method logs a warning and has no effect
```

- [ ] **Step 2: Update engine/doc.go**

Remove `e.RiskFreeAsset(e.Asset("DGS3MO"))` from the example code. Add a note explaining the risk-free rate is automatic:

```go
// The engine automatically uses the 3-Month Treasury Constant Maturity
// Rate (DGS3MO) as the risk-free rate for all performance metrics.
// Strategies do not need to configure this. If DGS3MO data is unavailable,
// metrics fall back to rf=0%.
```

- [ ] **Step 3: Run full test suite**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -count=1
```

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md engine/doc.go
git commit -m "docs: document performance metric fixes and DGS3MO risk-free rate"
```
