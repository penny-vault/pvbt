# PortfolioStats Interface and Metric Caching Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate redundant DataFrame allocations by caching derived series (returns, excess returns, drawdown) on a `PortfolioStats` interface, reducing total allocations from 10.8 GB to ~6 GB and GC CPU from ~47% to ~25%.

**Architecture:** `PortfolioStats` interface provides lazily cached derived series via DataFrame column insertion. Account implements it. Metrics receive `PortfolioStats` instead of `*Account`. The `PerformanceMetric.Compute` signature changes to `(ctx, stats, window)`. All 69 metrics are updated.

**Tech Stack:** Go 1.24, gonum/stat, Ginkgo/Gomega test framework

**Spec:** `docs/superpowers/specs/2026-03-21-metric-portfolio-stats-design.md`

---

### Task 1: Add metric constants and removeNaN helper

**Files:**
- Modify: `data/metric.go` -- add new constants
- Modify: `portfolio/metric_helpers.go` -- add `removeNaN`

- [ ] **Step 1: Add metric constants**

In `data/metric.go`, add the derived series constants alongside existing portfolio constants:

```go
const (
    PortfolioReturns       Metric = "portfolio_returns"
    PortfolioExcessReturns Metric = "portfolio_excess_returns"
    PortfolioDrawdown      Metric = "portfolio_drawdown"
    PortfolioBenchReturns  Metric = "portfolio_bench_returns"
)
```

- [ ] **Step 2: Add removeNaN helper**

In `portfolio/metric_helpers.go`, add:

```go
func removeNaN(col []float64) []float64 {
    clean := make([]float64, 0, len(col))
    for _, v := range col {
        if !math.IsNaN(v) {
            clean = append(clean, v)
        }
    }
    return clean
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./data/... ./portfolio/...`

- [ ] **Step 4: Commit**

```
git commit -m "feat: add derived metric constants and removeNaN helper"
```

---

### Task 2: Define PortfolioStats interface and update PerformanceMetric

**Files:**
- Modify: `portfolio/metric_query.go` -- new interface, update PerformanceMetric

- [ ] **Step 1: Add PortfolioStats interface**

The interface must cover all Account methods used by any metric:

```go
type PortfolioStats interface {
    Returns(ctx context.Context, window *Period) *data.DataFrame
    ExcessReturns(ctx context.Context, window *Period) *data.DataFrame
    Drawdown(ctx context.Context, window *Period) *data.DataFrame
    BenchmarkReturns(ctx context.Context, window *Period) *data.DataFrame
    Equity(ctx context.Context, window *Period) *data.DataFrame
    Transactions(ctx context.Context) []Transaction
    TradeDetails(ctx context.Context) []TradeDetail
    Prices(ctx context.Context) *data.DataFrame
    TaxLots(ctx context.Context) map[asset.Asset][]TaxLot
    ShortLots(ctx context.Context, fn func(asset.Asset, []TaxLot))
    PerfData(ctx context.Context) *data.DataFrame
}
```

Add `"context"` and `"github.com/penny-vault/pvbt/asset"` to imports.

- [ ] **Step 2: Update PerformanceMetric interface**

```go
type PerformanceMetric interface {
    Name() string
    Description() string
    Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error)
    ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period) (*data.DataFrame, error)
}
```

- [ ] **Step 3: Update PerformanceMetricQuery**

Change `resolveAccount` to `resolveStats`:

```go
func (q PerformanceMetricQuery) Value() (float64, error) {
    stats, err := q.resolveStats()
    if err != nil {
        return 0, err
    }
    return q.metric.Compute(context.Background(), stats, q.window)
}

func (q PerformanceMetricQuery) Series() (*data.DataFrame, error) {
    stats, err := q.resolveStats()
    if err != nil {
        return nil, err
    }
    return q.metric.ComputeSeries(context.Background(), stats, q.window)
}

func (q PerformanceMetricQuery) resolveStats() (PortfolioStats, error) {
    if !q.benchmark {
        return q.account, nil
    }
    if _, ok := q.metric.(BenchmarkTargetable); !ok {
        return nil, ErrBenchmarkNotSupported
    }
    return q.account.benchmarkView()
}
```

Note: This won't compile yet because Account doesn't implement PortfolioStats.

- [ ] **Step 4: Commit**

```
git commit -m "feat: define PortfolioStats interface and update PerformanceMetric signature"
```

---

### Task 3: Implement PortfolioStats on Account

**Files:**
- Modify: `portfolio/account.go` -- add `computed` field, implement interface methods

- [ ] **Step 1: Add computed field and ensureComputed helper**

Add to Account struct:

```go
computed map[data.Metric]bool
```

Add helper method:

```go
func (a *Account) ensureComputed(metric data.Metric, compute func()) {
    if a.computed == nil {
        a.computed = make(map[data.Metric]bool)
    }
    if !a.computed[metric] {
        compute()
        a.computed[metric] = true
    }
}
```

- [ ] **Step 2: Clear computed map when perfData changes**

In `UpdatePrices` (the method that calls `perfData.AppendRow`), after the
AppendRow call, clear the computed map:

```go
a.computed = nil
```

- [ ] **Step 3: Implement Returns**

```go
func (a *Account) Returns(ctx context.Context, window *Period) *data.DataFrame {
    if a.perfData == nil {
        return nil
    }
    a.ensureComputed(data.PortfolioReturns, func() {
        eq := a.perfData.Metrics(data.PortfolioEquity)
        returns := eq.Pct()
        a.perfData.Insert(portfolioAsset, data.PortfolioReturns,
            returns.Column(portfolioAsset, data.PortfolioEquity))
    })
    return a.perfData.Window(window)
}
```

- [ ] **Step 4: Implement ExcessReturns**

```go
func (a *Account) ExcessReturns(ctx context.Context, window *Period) *data.DataFrame {
    if a.perfData == nil {
        return nil
    }
    a.Returns(ctx, nil)
    a.ensureComputed(data.PortfolioExcessReturns, func() {
        retCol := a.perfData.Column(portfolioAsset, data.PortfolioReturns)
        rfCol := a.perfData.Column(portfolioAsset, data.PortfolioRiskFree)
        excessCol := make([]float64, len(retCol))
        for i := range retCol {
            excessCol[i] = retCol[i] - rfCol[i]
        }
        a.perfData.Insert(portfolioAsset, data.PortfolioExcessReturns, excessCol)
    })
    return a.perfData.Window(window)
}
```

- [ ] **Step 5: Implement Drawdown**

```go
func (a *Account) Drawdown(ctx context.Context, window *Period) *data.DataFrame {
    if a.perfData == nil {
        return nil
    }
    a.ensureComputed(data.PortfolioDrawdown, func() {
        eq := a.perfData.Metrics(data.PortfolioEquity)
        peak := eq.CumMax()
        dd := eq.Sub(peak).Div(peak)
        a.perfData.Insert(portfolioAsset, data.PortfolioDrawdown,
            dd.Column(portfolioAsset, data.PortfolioEquity))
    })
    return a.perfData.Window(window)
}
```

- [ ] **Step 6: Implement BenchmarkReturns**

```go
func (a *Account) BenchmarkReturns(ctx context.Context, window *Period) *data.DataFrame {
    if a.perfData == nil {
        return nil
    }
    a.ensureComputed(data.PortfolioBenchReturns, func() {
        bm := a.perfData.Metrics(data.PortfolioBenchmark)
        returns := bm.Pct()
        a.perfData.Insert(portfolioAsset, data.PortfolioBenchReturns,
            returns.Column(portfolioAsset, data.PortfolioBenchmark))
    })
    return a.perfData.Window(window)
}
```

- [ ] **Step 7: Implement Equity, Transactions, TradeDetails, Prices, TaxLots, ShortLots, PerfData**

These are thin wrappers that delegate to existing Account fields/methods. The
existing methods `PerfData()`, `Transactions()`, `TradeDetails()` already exist
on Account but need ctx parameter added:

```go
func (a *Account) Equity(ctx context.Context, window *Period) *data.DataFrame {
    if a.perfData == nil {
        return nil
    }
    return a.perfData.Window(window)
}

// Rename existing PerfData() or add the ctx variant.
// The PortfolioStats interface version:
func (a *Account) PerfDataStats(ctx context.Context) *data.DataFrame {
    return a.perfData
}
```

Note: The existing `PerfData()` (no ctx) is called from many places. The
PortfolioStats interface method name must match. If naming conflicts arise,
the interface method can be named differently (e.g., `PerfDataView`) or
the existing method can be updated to take ctx. Choose the approach that
minimizes churn.

- [ ] **Step 8: Verify compilation of portfolio package**

Run: `go build ./portfolio/...`
Expected: Will fail because metric Compute signatures haven't been updated yet.

- [ ] **Step 9: Commit**

```
git commit -m "feat: implement PortfolioStats on Account with lazy caching"
```

---

### Task 4: Update computeMetrics in engine

**Files:**
- Modify: `engine/metrics.go`

- [ ] **Step 1: Update computeMetrics signature and body**

```go
func computeMetrics(stats portfolio.PortfolioStats, date time.Time,
    metrics []portfolio.PerformanceMetric, appendMetric func(portfolio.MetricRow)) {

    ctx := context.Background()

    for _, metric := range metrics {
        val, err := metric.Compute(ctx, stats, nil)
        if err == nil {
            appendMetric(portfolio.MetricRow{
                Date:   date,
                Name:   metric.Name(),
                Window: "since_inception",
                Value:  val,
            })
        }

        for _, window := range standardWindows() {
            wCopy := window
            val, err := metric.Compute(ctx, stats, &wCopy)
            if err == nil {
                appendMetric(portfolio.MetricRow{
                    Date:   date,
                    Name:   metric.Name(),
                    Window: windowLabel(window),
                    Value:  val,
                })
            }
        }
    }
}
```

- [ ] **Step 2: Update call sites**

Find all calls to `computeMetrics` in the engine package and update them from
`computeMetrics(acct, date)` to
`computeMetrics(acct, date, acct.RegisteredMetrics(), acct.AppendMetric)`.

- [ ] **Step 3: Commit**

```
git commit -m "refactor: update computeMetrics to use PortfolioStats interface"
```

---

### Task 5: Migrate Pattern E metrics (trade/transaction-based, 17 metrics)

These are the simplest -- they don't use PerfData for the main computation.
The signature changes but the body only changes `a.TradeDetails()` to
`stats.TradeDetails(ctx)` and `a.Transactions()` to `stats.Transactions(ctx)`.

**Files:** 17 metric files in `portfolio/`

- [ ] **Step 1: Update all Pattern E metric signatures and bodies**

For each of these 17 files, change:
- `Compute(a *Account, window *Period)` to `Compute(ctx context.Context, stats PortfolioStats, window *Period)`
- `ComputeSeries(a *Account, window *Period)` to `ComputeSeries(ctx context.Context, stats PortfolioStats, window *Period)`
- `a.TradeDetails()` to `stats.TradeDetails(ctx)`
- `a.Transactions()` to `stats.Transactions(ctx)`
- `roundTrips(a.TradeDetails(), a.Transactions())` to `roundTrips(stats.TradeDetails(ctx), stats.Transactions(ctx))`

Files: average_holding_period.go, average_loss.go, average_mae.go,
average_mfe.go, average_win.go, edge_ratio.go, long_profit_factor.go,
long_win_rate.go, median_mae.go, median_mfe.go, profit_factor_metric.go,
short_profit_factor.go, short_win_rate.go, trade_capture_ratio.go,
trade_gain_loss_ratio.go, win_rate.go, turnover.go

Note: turnover.go also uses `a.PerfData()` -- update to
`stats.PerfData(ctx)`.

- [ ] **Step 2: Commit**

```
git commit -m "refactor: migrate trade/transaction metrics to PortfolioStats"
```

---

### Task 6: Migrate Pattern G metrics (tax/gains-based, 8 metrics)

These use Transactions, TaxLots, ShortLots, Prices, and/or PerfData.

**Files:** 8 metric files in `portfolio/`

- [ ] **Step 1: Update all Pattern G metric signatures and bodies**

For each file, change the signature and update Account method calls:
- `a.Transactions()` to `stats.Transactions(ctx)`
- `a.PerfData()` to `stats.PerfData(ctx)`
- `a.Prices()` to `stats.Prices(ctx)`
- `a.TaxLots()` to `stats.TaxLots(ctx)`
- `a.ShortLots(fn)` to `stats.ShortLots(ctx, fn)`

Files: ltcg.go, stcg.go, qualified_dividends.go, non_qualified_income.go,
tax_cost_ratio.go, tax_drag.go, unrealized_ltcg.go, unrealized_stcg.go

- [ ] **Step 2: Commit**

```
git commit -m "refactor: migrate tax/gains metrics to PortfolioStats"
```

---

### Task 7: Migrate Pattern A metrics (returns-based, 22 metrics)

These are the main performance target -- they currently each compute
`Pct().Drop(NaN())` independently. After migration, they call
`stats.Returns(ctx, window)` or `stats.ExcessReturns(ctx, window)`.

**Files:** 22 metric files in `portfolio/`

- [ ] **Step 1: Migrate metrics that use plain returns**

For metrics that only need `Returns`: twrr, std_dev, n_positive_periods,
exposure, value_at_risk, cvar, tail_ratio, kelly_criterion, gain_loss_ratio,
gain_to_pain, consecutive_wins, consecutive_losses, excess_kurtosis, skewness,
k_ratio, omega_ratio.

Change pattern:
```go
// Before:
pd := a.PerfData()
if pd == nil { return 0, nil }
r := pd.Window(window).Metrics(data.PortfolioEquity).Pct().Drop(math.NaN())
col := r.Column(portfolioAsset, data.PortfolioEquity)

// After:
df := stats.Returns(ctx, window)
if df == nil { return 0, nil }
col := removeNaN(df.Column(portfolioAsset, data.PortfolioReturns))
```

- [ ] **Step 2: Migrate metrics that use excess returns**

For metrics that subtract risk-free: sharpe, sortino, downside_deviation,
smart_sharpe, smart_sortino, probabilistic_sharpe.

Change pattern:
```go
// Before:
returns := perfDF.Metrics(data.PortfolioEquity, data.PortfolioRiskFree).Pct().Drop(math.NaN())
er := returns.Metrics(data.PortfolioEquity).Sub(returns, data.PortfolioRiskFree)
col := er.Column(portfolioAsset, data.PortfolioEquity)

// After:
df := stats.ExcessReturns(ctx, window)
if df == nil { return 0, nil }
col := removeNaN(df.Column(portfolioAsset, data.PortfolioExcessReturns))
```

- [ ] **Step 3: Migrate alpha**

Alpha calls `Beta.Compute(a, window)` internally. Update to
`Beta.Compute(ctx, stats, window)`.

- [ ] **Step 4: Commit**

```
git commit -m "refactor: migrate returns-based metrics to PortfolioStats"
```

---

### Task 8: Migrate Pattern B metrics (drawdown-based, 6 metrics)

**Files:** 6 metric files in `portfolio/`

- [ ] **Step 1: Update drawdown metrics**

For max_drawdown, avg_drawdown, avg_drawdown_days, keller_ratio:

```go
// Before:
equity := pd.Window(window).Metrics(data.PortfolioEquity)
peak := equity.CumMax()
dd := equity.Sub(peak).Div(peak)
col := dd.Column(portfolioAsset, data.PortfolioEquity)

// After:
df := stats.Drawdown(ctx, window)
if df == nil { return 0, nil }
col := df.Column(portfolioAsset, data.PortfolioDrawdown)
```

For calmar and recovery_factor which use BOTH drawdown and equity:

```go
// Use stats.Drawdown(ctx, window) for drawdown
// Use stats.Equity(ctx, window) for equity values
```

- [ ] **Step 2: Commit**

```
git commit -m "refactor: migrate drawdown-based metrics to PortfolioStats"
```

---

### Task 9: Migrate Pattern C metrics (benchmark-based, 8 metrics)

**Files:** 8 metric files in `portfolio/`

- [ ] **Step 1: Update benchmark metrics**

For beta, r_squared, tracking_error, information_ratio, upside_capture,
downside_capture, active_return:

These need both portfolio returns and benchmark returns. Use
`stats.Returns(ctx, window)` for portfolio returns and
`stats.BenchmarkReturns(ctx, window)` for benchmark returns.

```go
// Before:
returns := perfDF.Metrics(data.PortfolioEquity, data.PortfolioBenchmark).Pct().Drop(math.NaN())
pCol := returns.Column(portfolioAsset, data.PortfolioEquity)
bCol := returns.Column(portfolioAsset, data.PortfolioBenchmark)

// After:
rdf := stats.Returns(ctx, window)
bdf := stats.BenchmarkReturns(ctx, window)
if rdf == nil || bdf == nil { return 0, nil }
pCol := removeNaN(rdf.Column(portfolioAsset, data.PortfolioReturns))
bCol := removeNaN(bdf.Column(portfolioAsset, data.PortfolioBenchReturns))
```

Note: Beta and benchmark metrics need both columns to have the same length
after NaN removal. Since both Pct() columns have NaN at index 0, removing
NaN from each independently preserves alignment.

For alpha and treynor which call `Beta.Compute(a, window)`:
update to `Beta.Compute(ctx, stats, window)`.

- [ ] **Step 2: Commit**

```
git commit -m "refactor: migrate benchmark-based metrics to PortfolioStats"
```

---

### Task 10: Migrate Pattern D and F metrics (equity + withdrawal, 7 metrics)

**Files:** 7 metric files in `portfolio/`

- [ ] **Step 1: Update equity-value metrics**

For cagr_metric, mwrr, ulcer_index, treynor:

```go
// Before:
pd := a.PerfData()
col := pd.Window(window).Column(portfolioAsset, data.PortfolioEquity)

// After:
df := stats.Equity(ctx, window)
if df == nil { return 0, nil }
col := df.Column(portfolioAsset, data.PortfolioEquity)
```

Treynor also calls `Beta.Compute(a, window)` -- update to
`Beta.Compute(ctx, stats, window)`.

- [ ] **Step 2: Update withdrawal metrics**

For safe_withdrawal_rate, perpetual_withdrawal_rate, dynamic_withdrawal_rate:

```go
// Before:
pd := a.PerfData()
eq := pd.Window(window).Metrics(data.PortfolioEquity)
col := eq.Column(portfolioAsset, data.PortfolioEquity)
times := eq.Times()

// After:
df := stats.Equity(ctx, window)
if df == nil { return 0, nil }
col := df.Column(portfolioAsset, data.PortfolioEquity)
times := df.Times()
```

- [ ] **Step 3: Commit**

```
git commit -m "refactor: migrate equity and withdrawal metrics to PortfolioStats"
```

---

### Task 11: Update tests

**Files:** All `portfolio/*_test.go` files that test metrics

- [ ] **Step 1: Update test helpers**

Tests that call `metric.Compute(acct, nil)` need to change to
`metric.Compute(context.Background(), acct, nil)` since Account implements
PortfolioStats. This is a mechanical find-and-replace.

Tests that call `metric.ComputeSeries(acct, nil)` need the same treatment,
plus handling the return type change from `[]float64` to `*data.DataFrame`.
Series tests need to extract the column from the returned DataFrame.

- [ ] **Step 2: Update PerformanceMetricQuery tests**

Tests using `acct.PerformanceMetric(X).Value()` and `.Series()` should still
work since the query builder adapts internally. Verify `Series()` callers
handle `*data.DataFrame` return.

- [ ] **Step 3: Run data and portfolio tests**

Run: `go test ./data/... ./portfolio/... -count=1 -timeout 120s`

- [ ] **Step 4: Run full test suite**

Run: `go test ./... -count=1 -timeout 300s`

- [ ] **Step 5: Run lint**

Run: `golangci-lint run ./...`

- [ ] **Step 6: Commit**

```
git commit -m "test: update all metric tests for PortfolioStats interface"
```

---

### Task 12: Update remaining callers and final verification

**Files:**
- Modify: `engine/backtest.go`, `engine/live.go` -- update computeMetrics call sites
- Modify: any other files that reference old PerformanceMetric.Compute signature

- [ ] **Step 1: Find and fix remaining compilation errors**

Run: `go build ./...`
Fix any remaining references to the old Compute/ComputeSeries signatures.

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -count=1 -timeout 300s`
Expected: All tests pass

- [ ] **Step 3: Run lint**

Run: `golangci-lint run ./...`
Expected: No issues

- [ ] **Step 4: Commit**

```
git commit -m "refactor: fix remaining callers for PortfolioStats migration"
```
