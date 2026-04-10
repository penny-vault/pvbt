# Point-in-Time Fundamental Data Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix look-ahead bias and sparse-data failures in fundamental data queries by using filing-date semantics, forward-filling fundamental columns onto the daily grid, and letting strategies configure the fundamental dimension.

**Architecture:** The provider query adds `date_key` to SELECT. The engine forward-fills fundamental metric columns in the slab before trimming with `Between`. A new `SetFundamentalDimension` method on Engine flows through to the provider. The snapshot schema gains `date_key` and `report_period` columns.

**Tech Stack:** Go, PostgreSQL (pgx), SQLite (modernc.org/sqlite), Ginkgo/Gomega

**Spec:** `docs/superpowers/specs/2026-04-10-fundamental-point-in-time-design.md`

---

### Task 1: Add `IsFundamental` classification function

**Files:**
- Modify: `data/pvdata_provider.go:816-845` (metricView map area)
- Create: `data/metric_view_test.go`

- [ ] **Step 1: Write the failing test**

Create `data/metric_view_test.go`:

```go
package data_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("IsFundamental", func() {
	It("returns true for fundamental metrics", func() {
		Expect(data.IsFundamental(data.Revenue)).To(BeTrue())
		Expect(data.IsFundamental(data.WorkingCapital)).To(BeTrue())
		Expect(data.IsFundamental(data.NetIncome)).To(BeTrue())
	})

	It("returns false for eod metrics", func() {
		Expect(data.IsFundamental(data.MetricClose)).To(BeFalse())
		Expect(data.IsFundamental(data.Volume)).To(BeFalse())
	})

	It("returns false for derived metrics", func() {
		Expect(data.IsFundamental(data.MarketCap)).To(BeFalse())
		Expect(data.IsFundamental(data.PE)).To(BeFalse())
	})

	It("returns false for unknown metrics", func() {
		Expect(data.IsFundamental(data.Metric("nonexistent"))).To(BeFalse())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race --focus "IsFundamental" ./data/`
Expected: FAIL -- `IsFundamental` not defined

- [ ] **Step 3: Write minimal implementation**

Add to `data/pvdata_provider.go` after the `metricView` map (around line 933):

```go
// IsFundamental reports whether the given metric is sourced from the
// fundamentals table. Fundamental metrics are sparse (quarterly) and
// require forward-fill when merged with daily price data.
func IsFundamental(metric Metric) bool {
	return metricView[metric] == "fundamentals"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `ginkgo run -race --focus "IsFundamental" ./data/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add data/pvdata_provider.go data/metric_view_test.go
git commit -m "feat: add IsFundamental classifier for metric view routing"
```

---

### Task 2: Add `date_key` to `fetchFundamentals` SELECT

**Files:**
- Modify: `data/pvdata_provider.go:574-648` (fetchFundamentals)

This task changes the SQL query to also read `date_key`. The column is scanned but not yet used for anything beyond being available in future work. The time axis and WHERE clause stay on `event_date`.

- [ ] **Step 1: Write the failing test**

This is an internal query change that doesn't affect the provider's public API yet. Add an integration-style test that verifies `date_key` is readable. However, since the existing tests use `TestProvider` (no database), and the real database query is an internal detail, we verify this indirectly in Task 5. For now, no new test -- move to the implementation.

- [ ] **Step 2: Update the SQL query and scan**

In `data/pvdata_provider.go`, change `fetchFundamentals` (starting at line 603):

Replace the query and scan block:

```go
	query := fmt.Sprintf(
		`SELECT composite_figi, event_date, date_key, %s
		 FROM fundamentals
		 WHERE composite_figi = ANY($1) AND event_date BETWEEN $2::date AND $3::date AND dimension = $4
		 ORDER BY event_date`,
		strings.Join(sqlCols, ", "),
	)

	rows, err := conn.Query(ctx, query, figis, start, end, p.dimension)
	if err != nil {
		return fmt.Errorf("pvdata: query fundamentals: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			figi      string
			eventDate time.Time
			dateKey   time.Time
		)

		vals := make([]any, len(sqlCols)+3)
		vals[0] = &figi
		vals[1] = &eventDate
		vals[2] = &dateKey

		floatVals := make([]*float64, len(sqlCols))
		for idx := range sqlCols {
			vals[idx+3] = &floatVals[idx]
		}

		if err := rows.Scan(vals...); err != nil {
			return fmt.Errorf("pvdata: scan fundamentals row: %w", err)
		}

		eventDate = eodTimestamp(eventDate)
		sec := eventDate.Unix()
		timeSet[sec] = eventDate

		for idx, m := range metricOrder {
			if floatVals[idx] != nil {
				ensureCol(figi, m)[sec] = *floatVals[idx]
			}
		}
	}
```

Note: `dateKey` is scanned but not yet used. It will be exposed as DataFrame metadata in a future change.

- [ ] **Step 3: Run existing tests to verify nothing broke**

Run: `ginkgo run -race ./data/`
Expected: PASS (existing tests use TestProvider, not the real database)

- [ ] **Step 4: Commit**

```bash
git add data/pvdata_provider.go
git commit -m "feat: read date_key column in fetchFundamentals query"
```

---

### Task 3: Forward-fill fundamental columns in `fetchRange`

**Files:**
- Modify: `engine/engine.go:558-598` (slab assembly in fetchRange)
- Modify: `engine/fetch_test.go`

- [ ] **Step 1: Write the failing test**

Add to `engine/fetch_test.go`. This test creates a provider with both daily price data and sparse fundamental data (only a few dates have values), runs a `FetchAt` on a date with no fundamental row, and verifies the fundamental value is forward-filled from the most recent prior entry.

```go
Context("with sparse fundamental data", func() {
	It("forward-fills fundamental metrics to the simulation date", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		fundamentalAssets := []asset.Asset{spy}
		assetProvider = &mockAssetProvider{assets: fundamentalAssets}

		// Daily close prices: Jan 1 - Mar 31 2024
		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 90, fundamentalAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		// Sparse fundamental data: only two filing dates
		filingDate1 := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC) // Q3 filing
		filingDate2 := time.Date(2024, 2, 20, 16, 0, 0, 0, time.UTC) // Q4 filing

		fundTimes := []time.Time{filingDate1, filingDate2}
		fundVals := [][]float64{{100_000_000}, {120_000_000}} // WorkingCapital
		fundDF, err := data.NewDataFrame(fundTimes, fundamentalAssets,
			[]data.Metric{data.WorkingCapital}, data.Daily, fundVals)
		Expect(err).NotTo(HaveOccurred())
		fundProvider := data.NewTestProvider([]data.Metric{data.WorkingCapital}, fundDF)

		// Strategy fetches both Close and WorkingCapital via FetchAt
		strategy := &fetchAtStrategy{
			metrics: []data.Metric{data.MetricClose, data.WorkingCapital},
			assets:  fundamentalAssets,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		// Run on Feb 28 -- no fundamental filing on this date.
		// Should forward-fill from the Feb 20 filing (120M).
		simDate := time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC)
		_, btErr := eng.Backtest(context.Background(), simDate, simDate)
		Expect(btErr).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).NotTo(HaveOccurred())
		Expect(strategy.fetched).NotTo(BeNil())

		wc := strategy.fetched.Column(spy, data.WorkingCapital)
		Expect(wc).To(HaveLen(1))
		Expect(wc[0]).To(BeNumerically("==", 120_000_000))
	})

	It("forward-fills fundamentals in Fetch with a lookback range", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		fundamentalAssets := []asset.Asset{spy}
		assetProvider = &mockAssetProvider{assets: fundamentalAssets}

		// Daily close prices: Jan 1 - Mar 31 2024
		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 90, fundamentalAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		// Single fundamental filing on Feb 1
		filingDate := time.Date(2024, 2, 1, 16, 0, 0, 0, time.UTC)
		fundDF, err := data.NewDataFrame(
			[]time.Time{filingDate},
			fundamentalAssets,
			[]data.Metric{data.Revenue},
			data.Daily,
			[][]float64{{500_000_000}},
		)
		Expect(err).NotTo(HaveOccurred())
		fundProvider := data.NewTestProvider([]data.Metric{data.Revenue}, fundDF)

		strategy := &fetchStrategy{
			lookback: portfolio.Days(30),
			metrics:  []data.Metric{data.MetricClose, data.Revenue},
			assets:   fundamentalAssets,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		// Run on Mar 1. Lookback 30 days covers Feb 1 - Mar 1.
		// Revenue should be filled forward from Feb 1 across all days.
		simDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
		_, btErr := eng.Backtest(context.Background(), simDate, simDate)
		Expect(btErr).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).NotTo(HaveOccurred())

		revCol := strategy.fetched.Column(spy, data.Revenue)
		Expect(len(revCol)).To(BeNumerically(">", 1))

		// Every value from the filing date onward should be 500M.
		for _, val := range revCol {
			Expect(val).To(BeNumerically("==", 500_000_000))
		}
	})

	It("does not forward-fill price metrics", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		fundamentalAssets := []asset.Asset{spy}
		assetProvider = &mockAssetProvider{assets: fundamentalAssets}

		// Sparse close prices: only 2 days, with a gap
		day1 := time.Date(2024, 2, 1, 16, 0, 0, 0, time.UTC)
		day2 := time.Date(2024, 2, 5, 16, 0, 0, 0, time.UTC)
		priceDF, err := data.NewDataFrame(
			[]time.Time{day1, day2},
			fundamentalAssets,
			[]data.Metric{data.MetricClose},
			data.Daily,
			[][]float64{{100.0}, {105.0}},
		)
		Expect(err).NotTo(HaveOccurred())
		priceProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, priceDF)

		strategy := &fetchStrategy{
			lookback: portfolio.Days(5),
			metrics:  []data.Metric{data.MetricClose},
			assets:   fundamentalAssets,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(priceProvider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		)

		simDate := time.Date(2024, 2, 5, 0, 0, 0, 0, time.UTC)
		_, btErr := eng.Backtest(context.Background(), simDate, simDate)
		Expect(btErr).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).NotTo(HaveOccurred())

		closeCol := strategy.fetched.Column(spy, data.MetricClose)
		// Should have exactly 2 values (the 2 actual price days), not forward-filled
		Expect(closeCol).To(HaveLen(2))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `ginkgo run -race --focus "sparse fundamental" ./engine/`
Expected: FAIL -- the `FetchAt` test gets NaN for WorkingCapital; the `Fetch` test gets NaN for Revenue on non-filing days.

- [ ] **Step 3: Implement forward-fill in fetchRange**

In `engine/engine.go`, add the forward-fill logic after the slab scatter loop (after line 590) and before the `NewDataFrame` call (line 592). Import `data.IsFundamental`.

Insert between the scatter loop closing brace (line 590) and the `assembled, err :=` line (line 592):

```go
	// Forward-fill fundamental metric columns. Fundamentals are sparse
	// (quarterly filing dates) but represent step-function data: the
	// value holds until the next filing supersedes it.
	for mIdx, metric := range metrics {
		if !data.IsFundamental(metric) {
			continue
		}

		for aIdx := range assets {
			colStart := (aIdx*numMetrics + mIdx) * numTimes

			for ti := 1; ti < numTimes; ti++ {
				if math.IsNaN(slab[colStart+ti]) && !math.IsNaN(slab[colStart+ti-1]) {
					slab[colStart+ti] = slab[colStart+ti-1]
				}
			}
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `ginkgo run -race --focus "sparse fundamental" ./engine/`
Expected: PASS

- [ ] **Step 5: Run full engine test suite**

Run: `ginkgo run -race ./engine/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add engine/engine.go engine/fetch_test.go
git commit -m "feat: forward-fill fundamental metrics onto the daily grid in fetchRange"
```

---

### Task 4: Add `SetFundamentalDimension` on Engine and `SetDimension` on PVDataProvider

**Files:**
- Modify: `data/pvdata_provider.go:49-56` (PVDataProvider struct area)
- Modify: `engine/engine.go:50-77` (Engine struct area)
- Create: `engine/dimension_test.go`

- [ ] **Step 1: Write the failing test**

Create `engine/dimension_test.go`:

```go
package engine_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

type dimensionStrategy struct {
	dimensionToSet string
	metrics        []data.Metric
	assets         []asset.Asset
	fetched        *data.DataFrame
	fetchErr       error
}

func (s *dimensionStrategy) Name() string { return "dimensionStrategy" }

func (s *dimensionStrategy) Setup(eng *engine.Engine) {
	if s.dimensionToSet != "" {
		eng.SetFundamentalDimension(s.dimensionToSet)
	}
}

func (s *dimensionStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *dimensionStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	s.fetched, s.fetchErr = eng.FetchAt(ctx, s.assets, eng.CurrentDate(), s.metrics)
	return nil
}

var _ = Describe("SetFundamentalDimension", func() {
	It("accepts valid dimensions", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}

		closeDF := makeDailyDF(
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			90, testAssets, []data.Metric{data.MetricClose},
		)
		provider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)
		assetProv := &mockAssetProvider{assets: testAssets}

		strategy := &dimensionStrategy{
			dimensionToSet: "MRQ",
			metrics:        []data.Metric{data.MetricClose},
			assets:         testAssets,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 2, 5, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects invalid dimensions", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}

		closeDF := makeDailyDF(
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			90, testAssets, []data.Metric{data.MetricClose},
		)
		provider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)
		assetProv := &mockAssetProvider{assets: testAssets}

		strategy := &dimensionStrategy{
			dimensionToSet: "INVALID",
			metrics:        []data.Metric{data.MetricClose},
			assets:         testAssets,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 2, 5, 23, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid fundamental dimension"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race --focus "SetFundamentalDimension" ./engine/`
Expected: FAIL -- `SetFundamentalDimension` not defined

- [ ] **Step 3: Add `SetDimension` on PVDataProvider**

In `data/pvdata_provider.go`, add after the `WithConfigFile` function (after line 74):

```go
// SetDimension updates the fundamental dimension filter at runtime.
// Valid values: "ARQ", "ARY", "ART", "MRQ", "MRY", "MRT".
func (p *PVDataProvider) SetDimension(dim string) {
	p.dimension = dim
}
```

- [ ] **Step 4: Add `SetFundamentalDimension` on Engine**

In `engine/engine.go`, add a field to the Engine struct (around line 73, after `predicting`):

```go
	fundamentalDimension string
```

Add validation set:

```go
// validDimensions lists the accepted fundamental dimension codes.
var validDimensions = map[string]bool{
	"ARQ": true, "ARY": true, "ART": true,
	"MRQ": true, "MRY": true, "MRT": true,
}
```

Add the public method:

```go
// SetFundamentalDimension configures the fundamental data dimension used
// by this engine. Valid values: "ARQ", "ARY", "ART", "MRQ", "MRY", "MRT".
// Call this from Strategy.Setup. If not called, defaults to "ARQ".
func (e *Engine) SetFundamentalDimension(dim string) {
	e.fundamentalDimension = dim
}
```

- [ ] **Step 5: Wire dimension into engine initialization**

Find the engine initialization code that calls `buildProviderRouting` (or the backtest/live run setup). After `Setup` is called on the strategy and before the first data fetch, validate and apply the dimension.

In the engine's initialization sequence, after the strategy `Setup` call, add:

```go
	if e.fundamentalDimension != "" {
		if !validDimensions[e.fundamentalDimension] {
			return nil, fmt.Errorf("invalid fundamental dimension %q; valid values: ARQ, ARY, ART, MRQ, MRY, MRT", e.fundamentalDimension)
		}

		for _, provider := range e.providers {
			if pvProvider, ok := provider.(interface{ SetDimension(string) }); ok {
				pvProvider.SetDimension(e.fundamentalDimension)
			}
		}
	}
```

Use the `interface{ SetDimension(string) }` type assertion so this works without coupling the engine to `PVDataProvider` directly.

- [ ] **Step 6: Run tests to verify they pass**

Run: `ginkgo run -race --focus "SetFundamentalDimension" ./engine/`
Expected: PASS

- [ ] **Step 7: Run full test suite**

Run: `ginkgo run -race ./engine/`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add data/pvdata_provider.go engine/engine.go engine/dimension_test.go
git commit -m "feat: add SetFundamentalDimension for strategy-level dimension control"
```

---

### Task 5: Update snapshot schema with `date_key` and `report_period`

**Files:**
- Modify: `data/snapshot_schema.go:51-57`
- Modify: `data/snapshot_recorder.go:404-483`
- Modify: `data/snapshot_provider.go:529-540`
- Modify: `data/snapshot_recorder_test.go` (or `data/snapshot_provider_test.go`)

- [ ] **Step 1: Write the failing test**

Add a test in the existing snapshot test file that verifies `date_key` and `report_period` survive a round-trip through snapshot write/read. Since the snapshot recorder writes fundamental data and the snapshot provider reads it, check that the new columns are present in the schema.

In `data/snapshot_recorder_test.go`, add a test that creates a snapshot database, verifies the fundamentals table has the `date_key` and `report_period` columns:

```go
It("creates fundamentals table with date_key and report_period columns", func() {
	dbPath := filepath.Join(tmpDir, "schema_check.db")
	db, err := sql.Open("sqlite", dbPath)
	Expect(err).NotTo(HaveOccurred())
	defer db.Close()

	err = data.CreateSnapshotSchema(db)
	Expect(err).NotTo(HaveOccurred())

	// Verify columns exist by inserting a row with date_key and report_period.
	_, err = db.Exec(
		"INSERT INTO assets (composite_figi, ticker) VALUES (?, ?)",
		"TEST-FIGI", "TEST",
	)
	Expect(err).NotTo(HaveOccurred())

	_, err = db.Exec(
		"INSERT INTO fundamentals (composite_figi, event_date, date_key, report_period, dimension) VALUES (?, ?, ?, ?, ?)",
		"TEST-FIGI", "2024-06-30", "2024-03-31", "2024-03-29", "ARQ",
	)
	Expect(err).NotTo(HaveOccurred())

	var dateKey, reportPeriod string
	err = db.QueryRow(
		"SELECT date_key, report_period FROM fundamentals WHERE composite_figi = ?",
		"TEST-FIGI",
	).Scan(&dateKey, &reportPeriod)
	Expect(err).NotTo(HaveOccurred())
	Expect(dateKey).To(Equal("2024-03-31"))
	Expect(reportPeriod).To(Equal("2024-03-29"))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race --focus "date_key and report_period" ./data/`
Expected: FAIL -- columns don't exist in schema

- [ ] **Step 3: Update `CreateSnapshotSchema`**

In `data/snapshot_schema.go`, change the fundamentals CREATE TABLE (lines 51-57) to include the new columns:

```go
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS fundamentals (
			composite_figi TEXT NOT NULL REFERENCES assets(composite_figi),
			event_date TEXT NOT NULL,
			date_key TEXT,
			report_period TEXT,
			dimension TEXT NOT NULL,
			%s,
			PRIMARY KEY (composite_figi, event_date, dimension)
		)`, fundamentalCols),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `ginkgo run -race --focus "date_key and report_period" ./data/`
Expected: PASS

- [ ] **Step 5: Update snapshot recorder**

In `data/snapshot_recorder.go`, update `recordFundamentals` to write `date_key` and `report_period`. These columns are not yet populated from the DataFrame (that depends on the deferred metadata work), so write them as NULL for now. Change the INSERT statement (around line 447-448):

Update the placeholders and query to include the two new columns:

```go
	placeholders := make([]string, 5+len(colNames))
	placeholders[0] = "?" // composite_figi
	placeholders[1] = "?" // event_date
	placeholders[2] = "?" // date_key (NULL for now)
	placeholders[3] = "?" // report_period (NULL for now)
	placeholders[4] = "?" // dimension

	for idx := range colNames {
		placeholders[5+idx] = "?"
	}
```

```go
	query := fmt.Sprintf(
		"INSERT INTO fundamentals (composite_figi, event_date, date_key, report_period, dimension, %s) VALUES (%s) ON CONFLICT(composite_figi, event_date, dimension) DO UPDATE SET %s",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "),
		strings.Join(upsertCols, ", "),
	)
```

Update the args construction in the row loop:

```go
			args := make([]any, 5+len(colMetrics))
			args[0] = a.CompositeFigi
			args[1] = dateStr
			args[2] = nil // date_key: populated when DataFrame metadata is available
			args[3] = nil // report_period: populated when DataFrame metadata is available
			args[4] = "ARQ" // default dimension
```

And update the metric indexing to offset by 5 instead of 3:

```go
			for idx, metric := range colMetrics {
				mi, ok := mIdx[metric]
				if !ok {
					args[5+idx] = nil
					continue
				}

				val := df.columns[assetIdx*numDFMetrics+mi][timeIdx]
				if math.IsNaN(val) {
					args[5+idx] = nil
				} else {
					args[5+idx] = val
				}
			}
```

- [ ] **Step 6: Update snapshot provider**

In `data/snapshot_provider.go`, the `fetchFundamentals` query (around line 532-539) should also read `date_key` if present but the snapshot provider doesn't need to use it yet. For forward compatibility, just leave the query as-is -- it selects specific metric columns by name and won't break with new columns in the table. No change needed here.

- [ ] **Step 7: Run full data test suite**

Run: `ginkgo run -race ./data/`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add data/snapshot_schema.go data/snapshot_recorder.go data/snapshot_recorder_test.go
git commit -m "feat: add date_key and report_period columns to snapshot fundamentals table"
```

---

### Task 6: Update documentation

**Files:**
- Modify: `docs/data.md`
- Modify: `docs/strategy-guide.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update `docs/data.md`**

Add a new section after the fundamentals metrics listing (or wherever fundamental data is described). Add a section titled "Fundamental data semantics":

```markdown
### Fundamental data semantics

Fundamental metrics (revenue, earnings, balance sheet items, etc.) are sourced
from SEC filings and stored with three date fields:

| Field | Meaning |
|-------|---------|
| `event_date` | Filing date -- when the data became publicly available (AR dimensions) or fiscal period end (MR dimensions). This is the temporal index used for queries. |
| `date_key` | Normalized calendar quarter/year boundary. Used for cross-company comparison (e.g., aligning all companies' Q2 data). |
| `report_period` | The actual end date of the company's fiscal period as stated in filings. |

The engine automatically forward-fills fundamental values onto the daily time
grid. Once a filing becomes public, its values are treated as current until the
next filing supersedes them. This means `Fetch` and `FetchAt` return dense data
for fundamental metrics -- no NaN gaps between quarterly filings.

Fundamental queries filter by dimension (default `"ARQ"` -- As Reported
Quarterly). See `SetFundamentalDimension` in the strategy guide for how to
change this.
```

- [ ] **Step 2: Update `docs/strategy-guide.md`**

Add documentation for `SetFundamentalDimension` near the `Setup` method documentation:

```markdown
### Fundamental dimension

Strategies that use fundamental data can configure which reporting dimension
to query. Call `SetFundamentalDimension` during `Setup`:

```go
func (s *MyStrategy) Setup(eng *engine.Engine) {
    eng.SetFundamentalDimension("ARQ")
}
```

If not called, defaults to `"ARQ"`.

| Dimension | Description |
|-----------|-------------|
| `ARQ` | As Reported, Quarterly. Point-in-time (indexed to SEC filing date). Excludes restatements. Recommended for backtesting. |
| `ARY` | As Reported, Annual. Same as ARQ but annual observations. |
| `ART` | As Reported, Trailing Twelve Months. Quarterly observations of one-year duration. |
| `MRQ` | Most Recent Reported, Quarterly. Indexed to fiscal period end. Includes restatements. Suitable for business performance analysis. |
| `MRY` | Most Recent Reported, Annual. |
| `MRT` | Most Recent Reported, Trailing Twelve Months. |

AR dimensions are suitable for backtesting because they are indexed to the SEC
filing date, preventing look-ahead bias. MR dimensions include restatements
and are indexed to the fiscal period end, so they may introduce look-ahead bias
in backtests.
```

- [ ] **Step 3: Update `CHANGELOG.md`**

Add entries under `[Unreleased]`:

Under `### Added`:
```markdown
- Strategies can configure the fundamental data dimension (ARQ, MRQ, ARY, MRY, ART, MRT) via `SetFundamentalDimension` in `Setup`. AR dimensions use SEC filing dates for point-in-time correctness; MR dimensions include restatements and are indexed to the fiscal period. Defaults to ARQ.
```

Under `### Fixed`:
```markdown
- Fundamental metrics (revenue, working capital, etc.) are now forward-filled onto the daily time grid. Previously, `FetchAt` returned NaN when the simulation date did not exactly match a filing date, and `Fetch` returned sparse data with NaN gaps between quarterly filings.
```

- [ ] **Step 4: Commit**

```bash
git add docs/data.md docs/strategy-guide.md CHANGELOG.md
git commit -m "docs: document point-in-time fundamental semantics and SetFundamentalDimension"
```

---

### Task 7: Run full test suite and lint

**Files:** None (verification only)

- [ ] **Step 1: Run full test suite**

Run: `ginkgo run -race ./...`
Expected: PASS

- [ ] **Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: 0 issues

- [ ] **Step 3: Fix any issues found**

If any tests fail or lint issues appear, fix them before proceeding.

- [ ] **Step 4: Final commit if any fixes were needed**

Only if step 3 produced changes:

```bash
git add -A
git commit -m "fix: address lint and test issues from fundamental point-in-time changes"
```
