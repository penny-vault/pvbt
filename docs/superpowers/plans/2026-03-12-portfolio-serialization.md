# Portfolio Serialization Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move serialization responsibility from the CLI to the portfolio package, with SQLite as the storage format and engine-driven metric computation.

**Architecture:** The portfolio `*Account` gains metadata (key-value strings), metric registration (which `PerformanceMetric` implementations to compute), and metric storage (`[]MetricRow`). The engine computes registered metrics at each step after `UpdatePrices`. `ToSQLite`/`FromSQLite` on `*Account` handle persistence. The CLI delegates to these methods and its output files are removed.

**Tech Stack:** Go 1.25, `modernc.org/sqlite` (pure Go SQLite), Ginkgo v2 + Gomega for tests.

**Spec:** `docs/superpowers/specs/2026-03-12-portfolio-serialization-design.md`

---

## Chunk 1: Portfolio Metadata and Metric Infrastructure

### Task 1: Add SetMetadata / GetMetadata to Portfolio interface and Account

**Files:**
- Create: `portfolio/metadata.go`
- Modify: `portfolio/portfolio.go` (add interface methods)
- Modify: `portfolio/account.go` (add `metadata` field, initialize in `New()`)
- Test: `portfolio/metadata_test.go`

- [ ] **Step 1: Write the failing test**

Create `portfolio/metadata_test.go`:

```go
package portfolio_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Metadata", func() {
	var acct *portfolio.Account

	BeforeEach(func() {
		acct = portfolio.New()
	})

	It("returns empty string for unset key", func() {
		Expect(acct.GetMetadata("missing")).To(Equal(""))
	})

	It("round-trips a key-value pair", func() {
		acct.SetMetadata("run_id", "abc-123")
		Expect(acct.GetMetadata("run_id")).To(Equal("abc-123"))
	})

	It("overwrites an existing key", func() {
		acct.SetMetadata("key", "old")
		acct.SetMetadata("key", "new")
		Expect(acct.GetMetadata("key")).To(Equal("new"))
	})

	It("stores multiple keys independently", func() {
		acct.SetMetadata("a", "1")
		acct.SetMetadata("b", "2")
		Expect(acct.GetMetadata("a")).To(Equal("1"))
		Expect(acct.GetMetadata("b")).To(Equal("2"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/Metadata" -v`
Expected: Compilation error -- `SetMetadata` and `GetMetadata` not defined.

- [ ] **Step 3: Add methods to Portfolio interface**

In `portfolio/portfolio.go`, add to the `Portfolio` interface (after the `WithdrawalMetrics` method):

```go
	// SetMetadata stores a key-value string pair in the portfolio's
	// metadata map. Use this for run-level information like run ID,
	// strategy name, start/end dates, and strategy parameters.
	SetMetadata(key, value string)

	// GetMetadata retrieves a metadata value by key. Returns empty
	// string if the key has not been set.
	GetMetadata(key string) string
```

- [ ] **Step 4: Create metadata.go with Account implementation**

Create `portfolio/metadata.go`:

```go
package portfolio

// SetMetadata stores a key-value pair in the account's metadata map.
func (a *Account) SetMetadata(key, value string) {
	a.metadata[key] = value
}

// GetMetadata retrieves a metadata value by key. Returns empty string
// if the key has not been set.
func (a *Account) GetMetadata(key string) string {
	return a.metadata[key]
}

// AllMetadata returns a copy of the metadata map.
func (a *Account) AllMetadata() map[string]string {
	m := make(map[string]string, len(a.metadata))
	for k, v := range a.metadata {
		m[k] = v
	}
	return m
}
```

- [ ] **Step 5: Add metadata field to Account and initialize in New()**

In `portfolio/account.go`, add `metadata map[string]string` to the `Account` struct. In `New()`, add `metadata: make(map[string]string)` to the initialization.

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/Metadata" -v`
Expected: All 4 tests PASS.

- [ ] **Step 7: Run full portfolio test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -v`
Expected: All existing tests still pass.

- [ ] **Step 8: Commit**

```bash
git add portfolio/metadata.go portfolio/metadata_test.go portfolio/portfolio.go portfolio/account.go
git commit -m "feat(portfolio): add SetMetadata/GetMetadata to Portfolio interface"
```

---

### Task 2: Add MetricRow type and metric storage on Account

**Files:**
- Create: `portfolio/metric_row.go`
- Modify: `portfolio/account.go` (add `metrics` field and `registeredMetrics` field)
- Test: `portfolio/metric_row_test.go`

- [ ] **Step 1: Write the failing test**

Create `portfolio/metric_row_test.go`:

```go
package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("MetricRow", func() {
	var acct *portfolio.Account

	BeforeEach(func() {
		acct = portfolio.New()
	})

	It("starts with empty metrics", func() {
		Expect(acct.Metrics()).To(BeEmpty())
	})

	It("appends metric rows", func() {
		row := portfolio.MetricRow{
			Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Name:   "sharpe",
			Window: "1yr",
			Value:  1.5,
		}
		acct.AppendMetric(row)
		Expect(acct.Metrics()).To(HaveLen(1))
		Expect(acct.Metrics()[0]).To(Equal(row))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/MetricRow" -v`
Expected: Compilation error -- `MetricRow`, `Metrics()`, `AppendMetric()` not defined.

- [ ] **Step 3: Create metric_row.go**

Create `portfolio/metric_row.go`:

```go
package portfolio

import "time"

// MetricRow represents a single computed performance metric value at a
// specific date and window. The engine appends these at each step.
type MetricRow struct {
	Date   time.Time
	Name   string // e.g. "sharpe", "beta"
	Window string // e.g. "5yr", "3yr", "1yr", "ytd", "mtd", "wtd", "since_inception"
	Value  float64
}

// Metrics returns the accumulated metric rows.
func (a *Account) Metrics() []MetricRow {
	return a.metrics
}

// AppendMetric appends a MetricRow to the account's metric storage.
// Called by the engine at each step after computing metrics.
func (a *Account) AppendMetric(row MetricRow) {
	a.metrics = append(a.metrics, row)
}
```

- [ ] **Step 4: Add metrics field to Account**

In `portfolio/account.go`, add `metrics []MetricRow` to the `Account` struct.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/MetricRow" -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add portfolio/metric_row.go portfolio/metric_row_test.go portfolio/account.go
git commit -m "feat(portfolio): add MetricRow type and metric storage on Account"
```

---

### Task 3: Add metric registration options

**Files:**
- Create: `portfolio/metric_registration.go`
- Test: `portfolio/metric_registration_test.go`
- Modify: `portfolio/account.go` (add `registeredMetrics` field)

- [ ] **Step 1: Write the failing test**

Create `portfolio/metric_registration_test.go`:

```go
package portfolio_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("MetricRegistration", func() {
	It("registers all metrics by default when none specified", func() {
		acct := portfolio.New()
		Expect(len(acct.RegisteredMetrics())).To(BeNumerically(">", 30))
	})

	It("registers an individual metric", func() {
		acct := portfolio.New(portfolio.WithMetric(portfolio.Sharpe))
		Expect(acct.RegisteredMetrics()).To(HaveLen(1))
		Expect(acct.RegisteredMetrics()[0].Name()).To(Equal("Sharpe"))
	})

	It("registers summary metrics group", func() {
		acct := portfolio.New(portfolio.WithSummaryMetrics())
		names := metricNames(acct.RegisteredMetrics())
		Expect(names).To(ContainElements("TWRR", "MWRR", "Sharpe", "Sortino", "Calmar", "MaxDrawdown", "StdDev"))
	})

	It("registers all metrics", func() {
		acct := portfolio.New(portfolio.WithAllMetrics())
		Expect(len(acct.RegisteredMetrics())).To(BeNumerically(">", 30))
	})

	It("deduplicates metrics", func() {
		acct := portfolio.New(
			portfolio.WithMetric(portfolio.Sharpe),
			portfolio.WithMetric(portfolio.Sharpe),
		)
		Expect(acct.RegisteredMetrics()).To(HaveLen(1))
	})
})

func metricNames(metrics []portfolio.PerformanceMetric) []string {
	names := make([]string, len(metrics))
	for i, m := range metrics {
		names[i] = m.Name()
	}
	return names
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/MetricRegistration" -v`
Expected: Compilation error -- `WithMetric`, `RegisteredMetrics`, etc. not defined.

- [ ] **Step 3: Create metric_registration.go**

Create `portfolio/metric_registration.go`. This file contains `WithMetric`, group convenience functions, `WithAllMetrics`, and `RegisteredMetrics()`. Each group function calls `WithMetric` for every metric in that group.

The `WithMetric` option appends to a `registeredMetrics` slice on `*Account`, deduplicating by `Name()`.

The group functions use the existing package-level metric vars:
- Summary: `TWRR`, `MWRR`, `Sharpe`, `Sortino`, `Calmar`, `MaxDrawdown`, `StdDev`
- Risk: `Beta`, `Alpha`, `TrackingError`, `DownsideDeviation`, `InformationRatio`, `Treynor`, `UlcerIndex`, `ExcessKurtosis`, `Skewness`, `RSquared`, `ValueAtRisk`, `UpsideCaptureRatio`, `DownsideCaptureRatio`
- Trade: `WinRate`, `AverageWin`, `AverageLoss`, `ProfitFactor`, `AverageHoldingPeriod`, `Turnover`, `NPositivePeriods`, `TradeGainLossRatio`
- Withdrawal: `SafeWithdrawalRate`, `PerpetualWithdrawalRate`, `DynamicWithdrawalRate`
- Tax: `LTCGMetric`, `STCGMetric`, `UnrealizedLTCGMetric`, `UnrealizedSTCGMetric`, `QualifiedDividendsMetric`, `NonQualifiedIncomeMetric`, `TaxCostRatioMetric`

Also include metrics not in the above groups. Check for any additional package-level `PerformanceMetric` vars (e.g., `CAGR`, `ActiveReturn`, `SmartSharpe`, `SmartSortino`, `ProbabilisticSharpe`, `KRatio`, `KellerRatio`, `KellyCriterion`, `Omega`, `GainToPain`, `CVaR`, `TailRatio`, `RecoveryFactor`, `Exposure`, `ConsecutiveWins`, `ConsecutiveLosses`, `AvgDrawdown`, `AvgDrawdownDays`, `GainLossRatio`) and include them in `WithAllMetrics`.

```go
package portfolio

// WithMetric registers a single PerformanceMetric for computation.
func WithMetric(m PerformanceMetric) Option {
	return func(a *Account) {
		for _, existing := range a.registeredMetrics {
			if existing.Name() == m.Name() {
				return // deduplicate
			}
		}
		a.registeredMetrics = append(a.registeredMetrics, m)
	}
}

// RegisteredMetrics returns the list of metrics registered for computation.
func (a *Account) RegisteredMetrics() []PerformanceMetric {
	return a.registeredMetrics
}

// WithSummaryMetrics registers the summary metric group.
func WithSummaryMetrics() Option {
	return func(a *Account) {
		for _, m := range []PerformanceMetric{TWRR, MWRR, Sharpe, Sortino, Calmar, MaxDrawdown, StdDev} {
			WithMetric(m)(a)
		}
	}
}

// WithRiskMetrics registers the risk metric group.
func WithRiskMetrics() Option {
	return func(a *Account) {
		for _, m := range []PerformanceMetric{
			Beta, Alpha, TrackingError, DownsideDeviation,
			InformationRatio, Treynor, UlcerIndex, ExcessKurtosis,
			Skewness, RSquared, ValueAtRisk, UpsideCaptureRatio, DownsideCaptureRatio,
		} {
			WithMetric(m)(a)
		}
	}
}

// WithTradeMetrics registers the trade metric group.
func WithTradeMetrics() Option {
	return func(a *Account) {
		for _, m := range []PerformanceMetric{
			WinRate, AverageWin, AverageLoss, ProfitFactor,
			AverageHoldingPeriod, Turnover, NPositivePeriods, TradeGainLossRatio,
		} {
			WithMetric(m)(a)
		}
	}
}

// WithWithdrawalMetrics registers the withdrawal metric group.
func WithWithdrawalMetrics() Option {
	return func(a *Account) {
		for _, m := range []PerformanceMetric{SafeWithdrawalRate, PerpetualWithdrawalRate, DynamicWithdrawalRate} {
			WithMetric(m)(a)
		}
	}
}

// WithTaxMetrics registers the tax metric group.
func WithTaxMetrics() Option {
	return func(a *Account) {
		for _, m := range []PerformanceMetric{
			LTCGMetric, STCGMetric, UnrealizedLTCGMetric, UnrealizedSTCGMetric,
			QualifiedDividendsMetric, NonQualifiedIncomeMetric, TaxCostRatioMetric,
		} {
			WithMetric(m)(a)
		}
	}
}

// WithAllMetrics registers every known PerformanceMetric.
func WithAllMetrics() Option {
	return func(a *Account) {
		WithSummaryMetrics()(a)
		WithRiskMetrics()(a)
		WithTradeMetrics()(a)
		WithWithdrawalMetrics()(a)
		WithTaxMetrics()(a)
		// Additional metrics not in standard groups:
		for _, m := range []PerformanceMetric{
			CAGR, ActiveReturn, SmartSharpe, SmartSortino,
			ProbabilisticSharpe, KRatio, KellerRatio, KellyCriterion,
			OmegaRatio, GainToPainRatio, CVaR, TailRatio, RecoveryFactor,
			Exposure, ConsecutiveWins, ConsecutiveLosses,
			AvgDrawdown, AvgDrawdownDays, GainLossRatio,
		} {
			WithMetric(m)(a)
		}
	}
}
```

Note: The exact package-level var names were verified via `grep`. Key names that differ from the file names: `OmegaRatio` (not `Omega`), `GainToPainRatio` (not `GainToPain`). `GainLossRatio` is distinct from `TradeGainLossRatio` (already in trade group) -- both are included.

- [ ] **Step 4: Add registeredMetrics field to Account**

In `portfolio/account.go`, add `registeredMetrics []PerformanceMetric` to the `Account` struct.

- [ ] **Step 5: Add default-all-metrics behavior to New()**

In `portfolio/account.go`, at the end of `New()` (after all options are applied), add:

```go
	// Default: register all metrics if none were explicitly specified.
	if len(a.registeredMetrics) == 0 {
		WithAllMetrics()(a)
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/MetricRegistration" -v`
Expected: PASS.

- [ ] **Step 7: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -v`
Expected: All tests pass.

- [ ] **Step 8: Commit**

```bash
git add portfolio/metric_registration.go portfolio/metric_registration_test.go portfolio/account.go
git commit -m "feat(portfolio): add metric registration options with group conveniences"
```

---

### Task 4: Add YTD/MTD/WTD PeriodUnit values and update windowSlice

**Files:**
- Modify: `portfolio/period.go` (add new `PeriodUnit` constants and convenience functions)
- Modify: `portfolio/metric_helpers.go` (update `windowSlice` and `windowSliceTimes`)
- Test: `portfolio/metric_helpers_test.go` (add new test cases)

- [ ] **Step 1: Write the failing tests**

Add to `portfolio/metric_helpers_test.go` (or create if needed). These test the new `windowSlice` behavior for YTD/MTD/WTD:

```go
var _ = Describe("windowSlice calendar-relative windows", func() {
	var (
		times  []time.Time
		series []float64
	)

	BeforeEach(func() {
		// Daily series from 2024-12-01 to 2025-03-15
		start := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < 105; i++ {
			t := start.AddDate(0, 0, i)
			times = append(times, t)
			series = append(series, float64(i))
		}
	})

	It("YTD slices from Jan 1 of current year", func() {
		// Last date is 2025-03-15, so YTD starts 2025-01-01 = index 31
		w := portfolio.YTD()
		result := portfolio.ExportWindowSlice(series, times, &w)
		Expect(result[0]).To(Equal(float64(31))) // Jan 1, 2025
	})

	It("MTD slices from 1st of current month", func() {
		// Last date is 2025-03-15, so MTD starts 2025-03-01 = index 90
		w := portfolio.MTD()
		result := portfolio.ExportWindowSlice(series, times, &w)
		Expect(result[0]).To(Equal(float64(90))) // Mar 1, 2025
	})

	It("WTD slices from most recent Monday", func() {
		// 2025-03-15 is a Saturday, most recent Monday is 2025-03-10
		// Dec 1 = index 0, Jan 1 = index 31, Mar 1 = index 90, Mar 10 = index 99
		w := portfolio.WTD()
		result := portfolio.ExportWindowSlice(series, times, &w)
		Expect(result[0]).To(Equal(float64(99))) // Mar 10, 2025 (Monday)
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/windowSlice_calendar" -v`
Expected: Compilation error -- `YTD`, `MTD`, `WTD` not defined.

- [ ] **Step 3: Add new PeriodUnit constants and convenience functions**

In `portfolio/period.go`, add `UnitYTD`, `UnitMTD`, `UnitWTD` to the iota block:

```go
const (
	UnitDay PeriodUnit = iota
	UnitMonth
	UnitYear
	UnitYTD // year-to-date: from Jan 1 of the current year
	UnitMTD // month-to-date: from the 1st of the current month
	UnitWTD // week-to-date: from the most recent Monday
)

// YTD returns a Period representing year-to-date.
func YTD() Period { return Period{N: 0, Unit: UnitYTD} }

// MTD returns a Period representing month-to-date.
func MTD() Period { return Period{N: 0, Unit: UnitMTD} }

// WTD returns a Period representing week-to-date.
func WTD() Period { return Period{N: 0, Unit: UnitWTD} }
```

- [ ] **Step 4: Update windowSlice and windowSliceTimes in metric_helpers.go**

Add cases for the new units in the switch statement. For YTD: `cutoff = time.Date(last.Year(), 1, 1, ...)`. For MTD: `cutoff = time.Date(last.Year(), last.Month(), 1, ...)`. For WTD: compute the most recent Monday from `last`.

- [ ] **Step 5: Add ExportWindowSlice to export_test.go if not already present**

Check if `ExportWindowSlice` already exists in `portfolio/export_test.go`. It does (see line 10 in the file). No action needed.

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/windowSlice" -v`
Expected: PASS.

- [ ] **Step 7: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -v`
Expected: All tests pass.

- [ ] **Step 8: Commit**

```bash
git add portfolio/period.go portfolio/metric_helpers.go portfolio/metric_helpers_test.go
git commit -m "feat(portfolio): add YTD/MTD/WTD period units for calendar-relative metric windows"
```

---

## Chunk 2: Engine Changes

### Task 5: Add WithAccount engine option

**Files:**
- Modify: `engine/option.go` (add `WithAccount` option)
- Modify: `engine/engine.go` (add `account` field, update `createAccount`)
- Test: `engine/backtest_test.go` (add test for `WithAccount`)

- [ ] **Step 1: Write the failing test**

Add a test in `engine/backtest_test.go` that creates an Account with metadata set, passes it via `WithAccount`, runs a backtest, and verifies the metadata is preserved on the returned portfolio. Use the existing test patterns in that file.

```go
It("uses a pre-configured account when WithAccount is set", func() {
	acct := portfolio.New(
		portfolio.WithCash(50000),
		portfolio.WithSummaryMetrics(),
	)
	acct.SetMetadata("test_key", "test_value")

	eng := engine.New(strategy,
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithAccount(acct),
	)
	defer eng.Close()

	p, err := eng.Backtest(ctx, start, end)
	Expect(err).NotTo(HaveOccurred())
	Expect(p.GetMetadata("test_key")).To(Equal("test_value"))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./engine/ -run "WithAccount" -v`
Expected: Compilation error -- `WithAccount` not defined.

- [ ] **Step 3: Add WithAccount to engine/option.go**

```go
// WithAccount sets a pre-configured portfolio Account for the engine
// to use. When set, this takes priority over WithInitialDeposit,
// WithPortfolioSnapshot, and WithBroker.
func WithAccount(acct *portfolio.Account) Option {
	return func(e *Engine) {
		e.account = acct
	}
}
```

- [ ] **Step 4: Add account field to Engine struct**

In `engine/engine.go`, add `account *portfolio.Account` to the `Engine` struct.

- [ ] **Step 5: Update createAccount to use provided account**

In `engine/engine.go`, update `createAccount()`:

```go
func (e *Engine) createAccount() *portfolio.Account {
	if e.broker == nil {
		e.broker = NewSimulatedBroker()
	}

	// Use pre-configured account if provided.
	if e.account != nil {
		if e.initialDeposit != 0 || e.snapshot != nil {
			log.Warn().Msg("WithAccount set: ignoring WithInitialDeposit and WithPortfolioSnapshot")
		}
		if !e.account.HasBroker() {
			e.account.SetBroker(e.broker)
		}
		return e.account
	}

	var opts []portfolio.Option
	if e.snapshot != nil {
		opts = append(opts, portfolio.WithPortfolioSnapshot(e.snapshot))
	} else {
		opts = append(opts, portfolio.WithCash(e.initialDeposit))
	}
	opts = append(opts, portfolio.WithBroker(e.broker))

	return portfolio.New(opts...)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./engine/ -run "WithAccount" -v`
Expected: PASS.

- [ ] **Step 7: Run full engine test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./engine/ -v`
Expected: All tests pass.

- [ ] **Step 8: Commit**

```bash
git add engine/option.go engine/engine.go engine/backtest_test.go
git commit -m "feat(engine): add WithAccount option for pre-configured portfolio"
```

---

### Task 6: Add per-step metric computation in the engine backtest loop

**Files:**
- Modify: `engine/backtest.go` (add metric computation after `UpdatePrices`)
- Test: `engine/backtest_test.go` (verify metrics are populated after backtest)

- [ ] **Step 1: Write the failing test**

Add a test that runs a backtest with `WithSummaryMetrics()` registered and verifies that `acct.Metrics()` is populated after the backtest:

```go
It("computes registered metrics at each step", func() {
	acct := portfolio.New(
		portfolio.WithCash(100000),
		portfolio.WithMetric(portfolio.Sharpe),
	)

	eng := engine.New(strategy,
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithAccount(acct),
	)
	defer eng.Close()

	_, err := eng.Backtest(ctx, start, end)
	Expect(err).NotTo(HaveOccurred())

	metrics := acct.Metrics()
	Expect(metrics).NotTo(BeEmpty())

	// Each step should produce 7 rows (one per window) for Sharpe
	sharpeRows := 0
	for _, row := range metrics {
		if row.Name == "Sharpe" {
			sharpeRows++
		}
	}
	// Number of trading dates * 7 windows
	Expect(sharpeRows).To(BeNumerically(">", 0))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./engine/ -run "computes_registered_metrics" -v`
Expected: FAIL -- `acct.Metrics()` is empty.

- [ ] **Step 3: Add metric computation to the backtest loop**

In `engine/backtest.go`, after the `if len(priceAssets) > 0` block (after line 190, at the same indentation level as the cache eviction on line 193), add:

```go
		// 18b. Compute registered metrics across all standard windows.
		computeMetrics(acct, date)
```

Add a helper function `computeMetrics` in `engine/backtest.go` (or a new file `engine/metrics.go`):

```go
// standardWindows returns the fixed set of metric windows.
func standardWindows() []portfolio.Period {
	return []portfolio.Period{
		portfolio.Years(5),
		portfolio.Years(3),
		portfolio.Years(1),
		portfolio.YTD(),
		portfolio.MTD(),
		portfolio.WTD(),
	}
}

// computeMetrics computes all registered metrics on the account for
// the given date across all standard windows plus since-inception.
func computeMetrics(acct *portfolio.Account, date time.Time) {
	for _, m := range acct.RegisteredMetrics() {
		// Since inception (nil window).
		val, err := m.Compute(acct, nil)
		if err == nil {
			acct.AppendMetric(portfolio.MetricRow{
				Date:   date,
				Name:   m.Name(),
				Window: "since_inception",
				Value:  val,
			})
		}

		// Standard windows.
		for _, w := range standardWindows() {
			wCopy := w
			val, err := m.Compute(acct, &wCopy)
			if err == nil {
				acct.AppendMetric(portfolio.MetricRow{
					Date:   date,
					Name:   m.Name(),
					Window: windowLabel(w),
					Value:  val,
				})
			}
		}
	}
}

// windowLabel returns a human-readable label for a Period.
func windowLabel(p portfolio.Period) string {
	switch p.Unit {
	case portfolio.UnitYear:
		return fmt.Sprintf("%dyr", p.N)
	case portfolio.UnitMonth:
		return fmt.Sprintf("%dmo", p.N)
	case portfolio.UnitDay:
		return fmt.Sprintf("%dd", p.N)
	case portfolio.UnitYTD:
		return "ytd"
	case portfolio.UnitMTD:
		return "mtd"
	case portfolio.UnitWTD:
		return "wtd"
	default:
		return "unknown"
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./engine/ -run "computes_registered_metrics" -v`
Expected: PASS.

- [ ] **Step 5: Run full engine test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./engine/ -v`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add engine/backtest.go
git commit -m "feat(engine): compute registered metrics at each backtest step"
```

---

## Chunk 3: SQLite Serialization

### Task 7: Add modernc.org/sqlite dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go get modernc.org/sqlite && go mod tidy`

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./...`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add modernc.org/sqlite dependency"
```

---

### Task 8: Implement ToSQLite

**Files:**
- Create: `portfolio/sqlite.go`
- Test: `portfolio/sqlite_test.go`

This is a larger task. Break it into sub-steps: schema creation, metadata writing, equity curve writing, transactions, holdings, tax lots, price series, metrics.

- [ ] **Step 1: Write the failing test for ToSQLite round-trip**

Create `portfolio/sqlite_test.go` with a comprehensive test:

```go
package portfolio_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

// buildDF is defined in testutil_test.go (same package) and is available here.

var _ = Describe("SQLite serialization", func() {
	var (
		acct    *portfolio.Account
		tmpDir  string
		dbPath  string
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "pvbt-sqlite-test-*")
		Expect(err).NotTo(HaveOccurred())
		dbPath = filepath.Join(tmpDir, "test.db")

		spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BDTBL9"}
		bnd := asset.Asset{Ticker: "BND", CompositeFigi: "BBG000BBVML4"}

		acct = portfolio.New(
			portfolio.WithCash(100000),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bnd),
		)

		// Set metadata.
		acct.SetMetadata("run_id", "test-123")
		acct.SetMetadata("strategy", "momentum")

		// Record some transactions.
		acct.Record(portfolio.Transaction{
			Date:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    100,
			Price:  450.0,
			Amount: -45000.0,
		})

		// Simulate UpdatePrices by building a price DF.
		df := buildDF(
			time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			[]asset.Asset{spy, bnd},
			[]float64{450.0, 75.0},
			[]float64{450.0, 75.0},
		)
		acct.UpdatePrices(df)

		// Append a metric row.
		acct.AppendMetric(portfolio.MetricRow{
			Date:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Name:   "sharpe",
			Window: "1yr",
			Value:  1.5,
		})
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("writes and reads back a database", func() {
		err := acct.ToSQLite(dbPath)
		Expect(err).NotTo(HaveOccurred())

		restored, err := portfolio.FromSQLite(dbPath)
		Expect(err).NotTo(HaveOccurred())

		// Metadata round-trips.
		Expect(restored.GetMetadata("run_id")).To(Equal("test-123"))
		Expect(restored.GetMetadata("strategy")).To(Equal("momentum"))
		Expect(restored.GetMetadata("schema_version")).To(Equal("1"))

		// Equity curve round-trips.
		Expect(restored.EquityCurve()).To(HaveLen(len(acct.EquityCurve())))

		// Transactions round-trip.
		Expect(restored.Transactions()).To(HaveLen(len(acct.Transactions())))

		// Holdings round-trip.
		var holdingCount int
		restored.Holdings(func(_ asset.Asset, _ float64) { holdingCount++ })
		Expect(holdingCount).To(BeNumerically(">", 0))

		// Tax lots round-trip.
		Expect(restored.TaxLots()).NotTo(BeEmpty())

		// Metrics round-trip.
		Expect(restored.Metrics()).To(HaveLen(1))
		Expect(restored.Metrics()[0].Name).To(Equal("sharpe"))

		// Benchmark/risk-free identity round-trips.
		Expect(restored.Benchmark().Ticker).To(Equal("SPY"))
		Expect(restored.RiskFree().Ticker).To(Equal("BND"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/SQLite" -v`
Expected: Compilation error -- `ToSQLite`, `FromSQLite` not defined.

- [ ] **Step 3: Implement ToSQLite in portfolio/sqlite.go**

Create `portfolio/sqlite.go`. The file should:

1. Import `database/sql` and `_ "modernc.org/sqlite"`.
2. Define a `createSchema(db *sql.DB) error` function that executes all CREATE TABLE and CREATE INDEX statements from the spec.
3. Implement `func (a *Account) ToSQLite(path string) error` that:
   - Opens a SQLite database at `path` with `sql.Open("sqlite", path)`.
   - Creates the schema.
   - Writes metadata (including `schema_version=1`, benchmark/risk-free identity from `a.Benchmark()` and `a.RiskFree()`).
   - Writes equity curve (`a.EquityCurve()` and `a.EquityTimes()`).
   - Writes transactions (`a.Transactions()`), mapping `TransactionType` to string via `tx.Type.String()` -- but use lowercase (e.g., `strings.ToLower(tx.Type.String())`).
   - Writes holdings: iterate `a.Holdings()`, compute `avg_cost` from `a.TaxLots()`, compute `market_value` from last known price (`a.Prices()`).
   - Writes tax lots from `a.TaxLots()`.
   - Writes price series: `a.BenchmarkPrices()` and `a.RiskFreePrices()` with dates from `a.EquityTimes()`.
   - Writes metrics from `a.Metrics()`.
   - All writes should use transactions (`tx.Begin()` / `tx.Commit()`) for atomicity.
   - Uses prepared statements with batch inserts for performance.

- [ ] **Step 4: Run test to verify ToSQLite works (FromSQLite will still fail)**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/SQLite" -v`
Expected: FAIL on `FromSQLite` (not yet implemented), but `ToSQLite` should succeed.

- [ ] **Step 5: Commit ToSQLite**

```bash
git add portfolio/sqlite.go portfolio/sqlite_test.go
git commit -m "feat(portfolio): implement ToSQLite for Account serialization"
```

---

### Task 9: Implement FromSQLite

**Files:**
- Modify: `portfolio/sqlite.go` (add `FromSQLite`)
- Test: `portfolio/sqlite_test.go` (already written in Task 8)

- [ ] **Step 1: Implement FromSQLite**

In `portfolio/sqlite.go`, implement `func FromSQLite(path string) (*Account, error)` that:

1. Opens the database.
2. Checks `schema_version` in metadata. Returns error if not `"1"`.
3. Reads all metadata into the account's metadata map.
4. Reads equity curve (dates and values) into `equityCurve` and `equityTimes`.
5. Reads transactions, mapping string type back to `TransactionType`. Reconstructs `asset.Asset` from `ticker` + `figi`. Maps `qualified` integer to bool.
6. Replays transactions to rebuild `holdings` and `cash` (or reads holdings table for final state -- but replaying transactions is more reliable for cash). Actually: read transactions, call `New()`, then use `WithPortfolioSnapshot` pattern to set internal state directly. The simplest approach: create the Account with an empty state, then set fields directly. Since `FromSQLite` is in the `portfolio` package, it has access to unexported fields.
7. Reads tax lots.
8. Reads price series for benchmark and risk-free.
9. Reads benchmark/risk-free asset identity from metadata.
10. Reads metrics into `[]MetricRow`.

Since `FromSQLite` is in the `portfolio` package, it can set unexported fields directly on `*Account`.

- [ ] **Step 2: Run the full SQLite test**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "TestPortfolio/SQLite" -v`
Expected: PASS.

- [ ] **Step 3: Add edge case tests**

Add tests for:
- Empty portfolio (no transactions, no holdings).
- FromSQLite with nonexistent file returns error.
- FromSQLite with wrong schema version returns error.

- [ ] **Step 4: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -v`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add portfolio/sqlite.go portfolio/sqlite_test.go
git commit -m "feat(portfolio): implement FromSQLite for Account restoration"
```

---

## Chunk 4: Snapshot Update, CLI Migration, and Integration

### Task 10: Update PortfolioSnapshot to include metrics and metadata

**Files:**
- Modify: `portfolio/snapshot.go` (add `Metrics`, `AllMetadata` to interface)

- [ ] **Step 1: Check if PortfolioSnapshot needs updating**

The `PortfolioSnapshot` interface in `snapshot.go` should include methods for restoring metrics and metadata so that `WithPortfolioSnapshot` can restore a full account from `FromSQLite`. Check whether `WithPortfolioSnapshot` in `snapshot.go` needs to copy metrics and metadata.

- [ ] **Step 2: Add Metrics and AllMetadata to PortfolioSnapshot**

In `portfolio/snapshot.go`, add:

```go
type PortfolioSnapshot interface {
	// ... existing methods ...
	Metrics() []MetricRow
	AllMetadata() map[string]string
}
```

Update `WithPortfolioSnapshot` to restore metrics and metadata:

```go
a.metrics = append(a.metrics, snap.Metrics()...)
for k, v := range snap.AllMetadata() {
	a.metadata[k] = v
}
```

- [ ] **Step 3: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./... -v`
Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add portfolio/snapshot.go
git commit -m "feat(portfolio): add Metrics and AllMetadata to PortfolioSnapshot interface"
```

---

### Task 11: Update CLI backtest.go to use WithAccount and ToSQLite

**Files:**
- Modify: `cli/backtest.go`
- Modify: `cli/live.go` (if applicable)

- [ ] **Step 1: Update backtest.go**

Rewrite `runBacktest` to:
1. Create `*portfolio.Account` with `portfolio.WithCash(cash)` and `portfolio.WithAllMetrics()`.
2. Pass to engine via `engine.WithAccount(acct)`.
3. After backtest, set metadata on the returned portfolio.
4. Call `acct.ToSQLite(outputPath)` -- change default extension from `.jsonl` to `.db`.
5. Remove the `writePortfolio`, `writeTransactions`, `writeHoldings`, `writeMetrics` calls.
6. Remove the `--output-transactions`, `--output-holdings`, `--output-metrics` flags.
7. Update `defaultOutputPath` to use `.db` extension.
8. Update format validation to accept `.db`.
9. Verify `cli/summary.go` is not affected by the removal of output functions.

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./...`
Expected: No errors. (There may be unused import warnings from the old output files -- that's expected since we'll remove them next.)

- [ ] **Step 3: Commit**

```bash
git add cli/backtest.go cli/live.go
git commit -m "refactor(cli): use WithAccount and ToSQLite for backtest output"
```

---

### Task 12: Remove old CLI output files

**Files:**
- Delete: `cli/output.go`
- Delete: `cli/output_jsonl.go`
- Delete: `cli/output_parquet.go`

- [ ] **Step 1: Delete the files**

```bash
rm cli/output.go cli/output_jsonl.go cli/output_parquet.go
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./...`
Expected: No errors. If there are references to removed functions, fix them.

- [ ] **Step 3: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./... -v`
Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add -u cli/
git commit -m "refactor(cli): remove old serialization files (output.go, output_jsonl.go, output_parquet.go)"
```

---

### Task 13: Final integration test

**Files:**
- Test: `engine/backtest_test.go` (add integration test)

- [ ] **Step 1: Write integration test**

Add a test that runs a full backtest with `WithAccount`, writes to SQLite via `ToSQLite`, restores via `FromSQLite`, and verifies the restored account can be used with `WithAccount` in a new engine:

```go
It("round-trips a backtest through SQLite", func() {
	acct := portfolio.New(
		portfolio.WithCash(100000),
		portfolio.WithMetric(portfolio.Sharpe),
	)

	eng := engine.New(strategy,
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithAccount(acct),
	)
	defer eng.Close()

	_, err := eng.Backtest(ctx, start, end)
	Expect(err).NotTo(HaveOccurred())

	acct.SetMetadata("strategy", strategy.Name())

	tmpDir, err := os.MkdirTemp("", "pvbt-integration-*")
	Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "backtest.db")
	Expect(acct.ToSQLite(dbPath)).To(Succeed())

	restored, err := portfolio.FromSQLite(dbPath)
	Expect(err).NotTo(HaveOccurred())
	Expect(restored.GetMetadata("strategy")).To(Equal(strategy.Name()))
	Expect(restored.EquityCurve()).To(Equal(acct.EquityCurve()))
	Expect(restored.Metrics()).To(Equal(acct.Metrics()))
})
```

- [ ] **Step 2: Run integration test**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./engine/ -run "round-trips" -v`
Expected: PASS.

- [ ] **Step 3: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./... -v`
Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add engine/backtest_test.go
git commit -m "test(engine): add SQLite round-trip integration test"
```
