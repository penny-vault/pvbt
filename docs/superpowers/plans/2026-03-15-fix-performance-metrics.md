# Fix Performance Metrics Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix incorrect Sharpe, Sortino, Alpha, and withdrawal rate metrics by recording daily equity (mark-to-market every trading day) and computing the annualization factor from actual data.

**Architecture:** The root cause is that the engine only records equity on strategy schedule dates. A monthly strategy produces 28 monthly data points, making annualization fragile and withdrawal metrics nonsensical. The fix: (1) the backtest loop walks a daily `@close * * *` tradecron schedule alongside the strategy schedule, recording equity every trading day; (2) `annualizationFactor` computes from actual observation count rather than hardcoding 252/12; (3) Alpha switches to mean periodic excess returns; (4) the live engine adds the same daily recording with retry for delayed prices (mutual fund NAVs).

**Tech Stack:** Go, Ginkgo/Gomega, gonum/stat, tradecron

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `engine/backtest.go` | Modify | Interleave daily equity recording with strategy schedule |
| `engine/live.go` | Modify | Add daily equity recording with retry for delayed prices |
| `portfolio/metric_helpers.go` | Modify | Rewrite `annualizationFactor` to compute from actual data |
| `portfolio/alpha.go` | Modify | Use mean periodic excess returns + annualize |
| `portfolio/benchmark_metrics_test.go` | Modify | Update Alpha expected values |
| `portfolio/risk_adjusted_metrics_test.go` | Modify | Update Sharpe/Sortino/StdDev expected values for new AF |
| `engine/backtest_test.go` | Modify | Add test verifying daily equity recording for non-daily strategies |

---

## Chunk 1: Daily Equity Recording in Backtest

### Task 1: Modify the backtest loop to record equity every trading day

The backtest loop currently walks only the strategy schedule. Restructure it to walk all trading days via a daily `@close * * *` tradecron schedule, running strategy Compute only on strategy-schedule dates.

**Files:**
- Modify: `engine/backtest.go`
- Modify: `engine/backtest_test.go`

- [ ] **Step 1: Add a test for daily equity recording with a non-daily strategy**

Add to `engine/backtest_test.go`:

```go
// monthlyStrategy trades once per month at month-end but the engine
// should still record daily equity values.
type monthlyStrategy struct {
	assets []asset.Asset
}

func (s *monthlyStrategy) Name() string { return "monthlyStrategy" }

func (s *monthlyStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("@close @monthend", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("monthlyStrategy.Setup: %v", err))
	}
	eng.Schedule(tc)
}

func (s *monthlyStrategy) Compute(ctx context.Context, eng *engine.Engine, fund portfolio.Portfolio) error {
	if len(s.assets) == 0 {
		return nil
	}
	priceDF, err := eng.FetchAt(ctx, s.assets, eng.CurrentDate(), []data.Metric{data.MetricClose})
	if err != nil || priceDF == nil {
		return nil
	}
	// Buy 1 share of each asset on first compute if not already held.
	for _, target := range s.assets {
		if fund.Position(target) == 0 {
			fund.Order(ctx, target, portfolio.Buy, 1)
		}
	}
	return nil
}
```

Add test:

```go
Context("daily equity recording", func() {
	It("records equity every trading day even for a monthly strategy", func() {
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		df := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, df)

		strategy := &monthlyStrategy{assets: testAssets}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

		fund, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		// A monthly strategy over ~2 months would only have ~2 strategy dates.
		// But daily equity recording should give us ~40+ trading days of data.
		perfA := asset.Asset{CompositeFigi: "_PORTFOLIO_", Ticker: "_PORTFOLIO_"}
		equityCol := fund.PerfData().Column(perfA, data.PortfolioEquity)
		Expect(len(equityCol)).To(BeNumerically(">=", 30),
			"expected daily equity data, got %d points", len(equityCol))
	})
})
```

Run to confirm it fails:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -run "daily equity recording" -v -count=1
```

- [ ] **Step 2: Refactor backtest.go to walk daily dates**

Replace the date enumeration and step loop in `engine/backtest.go`. The new Phase 2 and Phase 3:

```go
// PHASE 2: DATE ENUMERATION

// Create a daily schedule for equity recording on every trading day.
dailySchedule, dailyErr := tradecron.New("@close * * *", tradecron.RegularHours)
if dailyErr != nil {
	return nil, fmt.Errorf("engine: creating daily equity schedule: %w", dailyErr)
}

// Collect strategy dates by calendar date.
strategyCalDates := make(map[string]bool)
cur := e.schedule.Next(start.Add(-time.Nanosecond))
for !cur.After(end) {
	strategyCalDates[cur.Format("2006-01-02")] = true
	cur = e.schedule.Next(cur.Add(time.Nanosecond))
}

// Walk all trading days.
type backtestStep struct {
	date       time.Time
	isStrategy bool
}

var steps []backtestStep
cur = dailySchedule.Next(start.Add(-time.Nanosecond))
for !cur.After(end) {
	calKey := cur.Format("2006-01-02")
	steps = append(steps, backtestStep{
		date:       cur,
		isStrategy: strategyCalDates[calKey],
	})
	cur = dailySchedule.Next(cur.Add(time.Nanosecond))
}

// PHASE 3: STEP LOOP

housekeepMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
priceMetrics := []data.Metric{data.MetricClose, data.AdjClose}

for stepIdx, step := range steps {
	// 10. Check context cancellation.
	if err := ctx.Err(); err != nil {
		return acct, err
	}

	date := step.date

	// 11. Set current date.
	e.currentDate = date

	// 12. Build step context with zerolog.
	stepLogger := zerolog.Ctx(ctx).With().
		Str("strategy", e.strategy.Name()).
		Time("date", date).
		Int("step", stepIdx+1).
		Int("total", len(steps)).
		Bool("strategy_day", step.isStrategy).
		Logger()
	stepCtx := stepLogger.WithContext(ctx)

	// 13. Fetch housekeeping data for held assets.
	var heldAssets []asset.Asset

	acct.Holdings(func(a asset.Asset, _ float64) {
		heldAssets = append(heldAssets, a)
	})

	var housekeepAssets []asset.Asset

	housekeepAssets = append(housekeepAssets, heldAssets...)
	if e.benchmark != (asset.Asset{}) {
		housekeepAssets = append(housekeepAssets, e.benchmark)
	}

	if e.riskFree != (asset.Asset{}) {
		housekeepAssets = append(housekeepAssets, e.riskFree)
	}

	var housekeepDF *data.DataFrame

	if len(housekeepAssets) > 0 {
		var fetchErr error

		housekeepDF, fetchErr = e.Fetch(stepCtx, housekeepAssets, portfolio.Days(1), housekeepMetrics)
		if fetchErr != nil {
			return nil, fmt.Errorf("engine: housekeeping fetch on %v: %w", date, fetchErr)
		}
	}

	// 14. Record dividends for held assets.
	if housekeepDF != nil {
		for _, heldAsset := range heldAssets {
			qty := acct.Position(heldAsset)
			if qty <= 0 {
				continue
			}

			divPerShare := housekeepDF.ValueAt(heldAsset, data.Dividend, date)
			if !math.IsNaN(divPerShare) && divPerShare > 0 {
				acct.Record(portfolio.Transaction{
					Date:   date,
					Asset:  heldAsset,
					Type:   portfolio.DividendTransaction,
					Amount: divPerShare * qty,
					Qty:    qty,
					Price:  divPerShare,
				})
			}
		}
	}

	// 15-16. Run strategy only on strategy-schedule dates.
	if step.isStrategy {
		// 15. Update simulated broker with price provider and date.
		if sb, ok := e.broker.(*SimulatedBroker); ok {
			sb.SetPriceProvider(e, date)
		}

		// 16. Call strategy.Compute.
		if err := e.strategy.Compute(stepCtx, e, acct); err != nil {
			return nil, fmt.Errorf("engine: strategy %q compute on %v: %w",
				e.strategy.Name(), date, err)
		}
	}

	// 17. Build price DataFrame for all held assets (including any
	// newly acquired positions from Compute).
	var priceAssets []asset.Asset

	acct.Holdings(func(a asset.Asset, _ float64) {
		priceAssets = append(priceAssets, a)
	})

	if e.benchmark != (asset.Asset{}) {
		priceAssets = append(priceAssets, e.benchmark)
	}

	if e.riskFree != (asset.Asset{}) {
		priceAssets = append(priceAssets, e.riskFree)
	}

	if len(priceAssets) > 0 {
		priceDF, err := e.FetchAt(stepCtx, priceAssets, date, priceMetrics)
		if err != nil {
			return nil, fmt.Errorf("engine: price fetch on %v: %w", date, err)
		}

		// 18. Record equity.
		acct.UpdatePrices(priceDF)
	}

	// 18b. Compute registered metrics only on strategy dates.
	if step.isStrategy {
		computeMetrics(acct, date)
	}

	// 19. Evict old cache data.
	e.cache.evictBefore(date)
}
```

- [ ] **Step 3: Run backtest tests to verify**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -run "Backtest" -v -count=1
```

Expected: All tests pass, including the new daily equity recording test.

- [ ] **Step 4: Commit**

```bash
git add engine/backtest.go engine/backtest_test.go
git commit -m "feat: record daily equity on every trading day regardless of strategy schedule"
```

---

## Chunk 2: Fix annualizationFactor

### Task 2: Compute annualization factor from actual data

Replace the hardcoded 252/12 binary with `(N-1) / calendar_years`. With daily equity guaranteed from Chunk 1, this gives the actual trading days per year.

**Files:**
- Modify: `portfolio/metric_helpers.go:23-34`
- Modify: `portfolio/risk_adjusted_metrics_test.go` (update expected values that depend on AF)
- Modify: `portfolio/benchmark_metrics_test.go` (update expected values that depend on AF)

- [ ] **Step 1: Compute expected values under new AF for existing tests**

The existing risk-adjusted tests use `daySeq(time.Date(2025, 1, 2, ...), 6)` which produces:
Jan 2 (Thu), Jan 3 (Fri), Jan 6 (Mon), Jan 7 (Tue), Jan 8 (Wed), Jan 9 (Thu).

Old AF = 252. New AF = (6-1) / ((Jan 9 - Jan 2) in years) = 5 / (7/365.25) = 5 / 0.01916 = 260.89.

The expected values change proportionally:
- StdDev (old): `stddev(r) * sqrt(252) = 0.08438 * 15.875 = 1.3394`
  StdDev (new): `0.08438 * sqrt(260.89) = 0.08438 * 16.153 = 1.3629`
- Sharpe (old): `mean(er)/stddev(er) * sqrt(252) = 4.1249`
  Sharpe (new): `mean(er)/stddev(er) * sqrt(260.89) = 4.1249 * sqrt(260.89)/sqrt(252) = 4.1249 * 1.0175 = 4.197`
- Sortino (old): `8.778`
  Sortino (new): `8.778 * sqrt(260.89)/sqrt(252) = 8.778 * 1.0175 = 8.932`
- DownsideDeviation (old): `0.09445`
  DownsideDeviation (new): `0.09445 * sqrt(260.89)/sqrt(252) = 0.09445 * 1.0175 = 0.09610`
- Calmar: Uses CAGR/MaxDD. CAGR uses duration directly, not AF. MaxDD doesn't use AF. No change.

Benchmark metrics tests (TrackingError, InformationRatio):
- daySeq Jan 2 2025 with 6 points: same AF=260.89
- TrackingError (old): `0.004541... * sqrt(252) = 0.072094`
  TrackingError (new): `0.004541... * sqrt(260.89) = 0.073353`
- InformationRatio (old): `6.6455`
  InformationRatio (new): `6.6455 * sqrt(260.89)/sqrt(252) = 6.6455 * 1.0175 = 6.762`

Alpha: will be changed in Chunk 3 (separate task).

Update the test expectations in `risk_adjusted_metrics_test.go` and `benchmark_metrics_test.go`. Use tolerances wide enough to account for the exact AF calculation.

**Important**: Rather than hardcoding these numbers, compute the expected AF programmatically in the test comments so future readers can verify. The tolerance should be `0.05` (relative) to account for the approximate AF.

- [ ] **Step 2: Rewrite annualizationFactor in metric_helpers.go**

Replace `portfolio/metric_helpers.go:23-34`:

```go
// annualizationFactor computes the number of observation periods per year
// from the actual timestamps. This avoids hardcoding 252 or 12 and correctly
// handles any schedule frequency, market closures, and holidays.
func annualizationFactor(times []time.Time) float64 {
	if len(times) < 2 {
		return 1
	}

	calendarDays := times[len(times)-1].Sub(times[0]).Hours() / 24
	if calendarDays <= 0 {
		return 1
	}

	years := calendarDays / 365.25

	return float64(len(times)-1) / years
}
```

- [ ] **Step 3: Run tests and adjust expected values**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Risk-Adjusted|Benchmark" -v -count=1
```

Adjust expected values to match the new AF. The changes should be small (within a few percent of old values since the test data spans only a few days, making AF slightly different from 252).

- [ ] **Step 4: Commit**

```bash
git add portfolio/metric_helpers.go portfolio/risk_adjusted_metrics_test.go portfolio/benchmark_metrics_test.go
git commit -m "fix: compute annualization factor from actual observation frequency"
```

---

## Chunk 3: Fix Alpha Calculation

### Task 3: Rewrite Alpha to use mean periodic excess returns

The current implementation (`portfolio/alpha.go:54-63`) uses total cumulative returns. Standard Jensen's alpha uses mean periodic excess returns, annualized.

**Files:**
- Modify: `portfolio/alpha.go`
- Modify: `portfolio/benchmark_metrics_test.go`

- [ ] **Step 1: Update Alpha test expectations**

The new Alpha formula is:
```
alpha = (mean(R_p - R_f) - beta * mean(R_m - R_f)) * AF
```

For the "divergent portfolio" test (6 daily points, 5 returns):
```
Portfolio returns:   [0.05, -0.028571, 0.058824, -0.018519, 0.037736]
Benchmark returns:   [0.04, -0.028846, 0.059406, -0.018692, 0.038095]
RiskFree returns:    [~0.0001, ~0.0001, ~0.0001, ~0.0001, ~0.0001]

mean(R_p - R_f) ≈ 0.019814
mean(R_m - R_f) ≈ 0.017893
Beta = 1.027594

alpha_per_period = 0.019814 - 1.027594 * 0.017893 ≈ 0.001426
AF ≈ 260.89 (5 returns over 7 calendar days)
alpha_annualized ≈ 0.001426 * 260.89 ≈ 0.372
```

For "perfect tracking" (portfolio = 10x benchmark): alpha = 0.0 (unchanged).

For "zero benchmark variance" (flat benchmark, beta=0):
```
mean(R_p - R_f) ≈ 0.08769
alpha = 0.08769 * AF
```
With 5 daily points (4 returns), daySeq: Jan 2,3,6,7,8. span=6 days.
AF = 4/(6/365.25) = 243.5.
alpha ≈ 0.08769 * 243.5 ≈ 21.35.

For "2-point equity curve": only 1 return, require at least 2 returns for meaningful alpha. Return 0.

Update test expectations accordingly.

- [ ] **Step 2: Rewrite alpha.go**

Replace the Compute method in `portfolio/alpha.go`:

```go
func (alpha) Compute(acct *Account, window *Period) (float64, error) {
	pd := acct.PerfData()
	if pd == nil {
		return 0, nil
	}

	rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
	if len(rfCol) == 0 || rfCol[0] == 0 {
		return 0, ErrNoRiskFreeRate
	}

	bmCol := pd.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmCol) == 0 || bmCol[0] == 0 {
		return 0, ErrNoBenchmark
	}

	perfDF := pd.Window(window)
	returns := perfDF.Pct().Drop(math.NaN())
	if returns.Len() < 2 {
		return 0, nil
	}

	pCol := returns.Column(portfolioAsset, data.PortfolioEquity)
	bCol := returns.Column(portfolioAsset, data.PortfolioBenchmark)
	rCol := returns.Column(portfolioAsset, data.PortfolioRiskFree)

	if len(pCol) < 2 || len(bCol) < 2 || len(rCol) < 2 {
		return 0, nil
	}

	// Compute mean excess returns.
	excessPortfolio := make([]float64, len(pCol))
	excessBenchmark := make([]float64, len(bCol))
	for idx := range pCol {
		excessPortfolio[idx] = pCol[idx] - rCol[idx]
		excessBenchmark[idx] = bCol[idx] - rCol[idx]
	}

	meanExcessPortfolio := stat.Mean(excessPortfolio, nil)
	meanExcessBenchmark := stat.Mean(excessBenchmark, nil)

	betaVal, err := Beta.Compute(acct, window)
	if err != nil {
		return 0, err
	}

	af := annualizationFactor(perfDF.Times())

	return (meanExcessPortfolio - betaVal*meanExcessBenchmark) * af, nil
}
```

Add `"math"` and `"gonum.org/v1/gonum/stat"` to the import block.

- [ ] **Step 3: Run tests**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Benchmark Metrics" -v -count=1
```

- [ ] **Step 4: Commit**

```bash
git add portfolio/alpha.go portfolio/benchmark_metrics_test.go
git commit -m "fix: compute Jensen's alpha from mean periodic excess returns and annualize"
```

---

## Chunk 4: Daily Equity Recording in Live Engine

### Task 4: Add daily equity recording to the live engine

The live engine goroutine currently only fires on the strategy schedule. Add a parallel daily schedule that records equity. When prices are unavailable (mutual fund NAVs delayed until 1-3 AM), retry after one hour.

**Files:**
- Modify: `engine/live.go`

- [ ] **Step 1: Restructure the live goroutine**

The live engine needs to handle two schedules:
1. Daily equity schedule (`@close * * *`) -- mark-to-market
2. Strategy schedule -- run Compute

Replace the goroutine body in `engine/live.go:90-215`. The approach: compute the next time for both schedules, sleep until whichever is sooner, then execute the appropriate step.

```go
go func() {
	defer close(portfolioCh)

	dailySchedule, dailyErr := tradecron.New("@close * * *", tradecron.RegularHours)
	if dailyErr != nil {
		zerolog.Ctx(ctx).Error().Err(dailyErr).Msg("failed to create daily equity schedule")
		return
	}

	housekeepMetrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
	priceMetrics := []data.Metric{data.MetricClose, data.AdjClose}
	step := 0

	for {
		// Compute next fire time for both schedules.
		now := time.Now()
		nextStrategy := e.schedule.Next(now)
		nextDaily := dailySchedule.Next(now)

		// Pick whichever is sooner.
		nextTime := nextDaily
		isStrategy := false
		if !nextStrategy.After(nextDaily) {
			nextTime = nextStrategy
			isStrategy = true
		}
		// If they fall on the same calendar day, treat as strategy day.
		if nextStrategy.Format("2006-01-02") == nextDaily.Format("2006-01-02") {
			isStrategy = true
			// Use the later timestamp (close) for equity recording.
			if nextDaily.After(nextStrategy) {
				nextTime = nextDaily
			}
		}

		wait := time.Until(nextTime)

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}

		step++
		e.currentDate = time.Now()

		stepLogger := zerolog.Ctx(ctx).With().
			Str("strategy", e.strategy.Name()).
			Time("date", e.currentDate).
			Int("step", step).
			Bool("strategy_day", isStrategy).
			Logger()
		stepCtx := stepLogger.WithContext(ctx)

		// Housekeeping: fetch prices and dividends.
		var heldAssets []asset.Asset
		acct.Holdings(func(a asset.Asset, _ float64) {
			heldAssets = append(heldAssets, a)
		})

		var housekeepAssets []asset.Asset
		housekeepAssets = append(housekeepAssets, heldAssets...)
		if e.benchmark != (asset.Asset{}) {
			housekeepAssets = append(housekeepAssets, e.benchmark)
		}
		if e.riskFree != (asset.Asset{}) {
			housekeepAssets = append(housekeepAssets, e.riskFree)
		}

		var housekeepDF *data.DataFrame
		if len(housekeepAssets) > 0 {
			var fetchErr error
			housekeepDF, fetchErr = e.Fetch(stepCtx, housekeepAssets, portfolio.Days(1), housekeepMetrics)
			if fetchErr != nil {
				zerolog.Ctx(stepCtx).Error().Err(fetchErr).Msg("housekeeping fetch failed")
				continue
			}
		}

		// Record dividends.
		if housekeepDF != nil {
			for _, heldAsset := range heldAssets {
				qty := acct.Position(heldAsset)
				if qty <= 0 {
					continue
				}
				divPerShare := housekeepDF.ValueAt(heldAsset, data.Dividend, e.currentDate)
				if !math.IsNaN(divPerShare) && divPerShare > 0 {
					acct.Record(portfolio.Transaction{
						Date:   e.currentDate,
						Asset:  heldAsset,
						Type:   portfolio.DividendTransaction,
						Amount: divPerShare * qty,
						Qty:    qty,
						Price:  divPerShare,
					})
				}
			}
		}

		// Run strategy only on strategy days.
		if isStrategy {
			if sb, ok := e.broker.(*SimulatedBroker); ok {
				sb.SetPriceProvider(e, e.currentDate)
			}

			if err := e.strategy.Compute(stepCtx, e, acct); err != nil {
				zerolog.Ctx(stepCtx).Error().Err(err).Msg("strategy compute failed")
				continue
			}
		}

		// Mark-to-market: fetch prices and record equity.
		// Retry up to 18 times with 1-hour waits for delayed prices (mutual fund NAVs).
		var priceAssets []asset.Asset
		acct.Holdings(func(a asset.Asset, _ float64) {
			priceAssets = append(priceAssets, a)
		})
		if e.benchmark != (asset.Asset{}) {
			priceAssets = append(priceAssets, e.benchmark)
		}
		if e.riskFree != (asset.Asset{}) {
			priceAssets = append(priceAssets, e.riskFree)
		}

		if len(priceAssets) > 0 {
			var priceDF *data.DataFrame
			var fetchErr error

			for attempt := range 18 {
				priceDF, fetchErr = e.FetchAt(stepCtx, priceAssets, e.currentDate, priceMetrics)
				if fetchErr == nil {
					break
				}

				zerolog.Ctx(stepCtx).Warn().
					Err(fetchErr).
					Int("attempt", attempt+1).
					Msg("price fetch failed, retrying in 1 hour")

				select {
				case <-time.After(time.Hour):
				case <-ctx.Done():
					return
				}
			}

			if fetchErr != nil {
				zerolog.Ctx(stepCtx).Error().Err(fetchErr).Msg("price fetch failed after retries")
			} else {
				acct.UpdatePrices(priceDF)
			}
		}

		// Send updated portfolio.
		select {
		case portfolioCh <- acct:
		default:
		}
	}
}()
```

- [ ] **Step 2: Run live tests**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -run "RunLive" -v -count=1
```

- [ ] **Step 3: Commit**

```bash
git add engine/live.go
git commit -m "feat: add daily equity recording to live engine with retry for delayed prices"
```

---

## Chunk 5: Full Verification

### Task 5: Run complete test suite

- [ ] **Step 1: Run portfolio tests**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -count=1
```

- [ ] **Step 2: Run engine tests**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -count=1
```

- [ ] **Step 3: Run full project tests**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -count=1
```

Expected: All pass (or only pre-existing failures unrelated to these changes).
