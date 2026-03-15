# Snapshot-Based Testing Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Capture real market data during a strategy run into a SQLite file and replay it deterministically in unit tests.

**Architecture:** A `SnapshotRecorder` wraps real providers, records all data access to SQLite during a backtest, and a `SnapshotProvider` reads that file back for replay. A `snapshot` CLI command orchestrates capture. All snapshot code lives in the `data` package; the CLI command lives in `cli`.

**Tech Stack:** Go, SQLite via `modernc.org/sqlite`, Ginkgo/Gomega for tests, Cobra for CLI.

**Spec:** `docs/superpowers/specs/2026-03-15-snapshot-testing-design.md`

---

## Chunk 1: Schema and Dependencies

### Task 1: Add SQLite dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add modernc.org/sqlite dependency**

Run: `go get modernc.org/sqlite`

- [ ] **Step 2: Tidy modules**

Run: `go mod tidy`

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add modernc.org/sqlite for snapshot testing"
```

---

### Task 2: Create snapshot schema DDL

**Files:**
- Create: `data/snapshot_schema.go`
- Test: `data/snapshot_schema_test.go`

The schema file defines the SQLite DDL and a helper that creates the tables in a given `*sql.DB`. The fundamental columns are derived from the `metricColumn` map in `pvdata_provider.go`.

- [ ] **Step 1: Write the failing test**

Create `data/snapshot_schema_test.go`:

```go
package data_test

import (
	"database/sql"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/data"

	_ "modernc.org/sqlite"
)

var _ = Describe("SnapshotSchema", func() {
	var db *sql.DB

	BeforeEach(func() {
		var err error
		db, err = sql.Open("sqlite", ":memory:")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		db.Close()
	})

	It("creates all expected tables", func() {
		err := data.CreateSnapshotSchema(db)
		Expect(err).NotTo(HaveOccurred())

		tables := []string{"assets", "eod", "metrics", "fundamentals", "ratings", "index_members"}
		for _, table := range tables {
			var count int
			err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count)
			Expect(err).NotTo(HaveOccurred(), "table %s should exist", table)
			Expect(count).To(Equal(0))
		}
	})

	It("creates the fundamentals table with all metricColumn entries", func() {
		err := data.CreateSnapshotSchema(db)
		Expect(err).NotTo(HaveOccurred())

		// Insert a row with just the required columns to verify the table accepts them.
		_, err = db.Exec("INSERT INTO fundamentals (composite_figi, event_date, dimension) VALUES ('TEST', '2024-01-02', 'ARQ')")
		Expect(err).NotTo(HaveOccurred())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "SnapshotSchema" -v`
Expected: FAIL -- `data.CreateSnapshotSchema` does not exist.

- [ ] **Step 3: Write the implementation**

Create `data/snapshot_schema.go`:

```go
package data

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// CreateSnapshotSchema creates all tables needed by a snapshot database.
func CreateSnapshotSchema(db *sql.DB) error {
	fundamentalCols := buildFundamentalColumns()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS assets (
			composite_figi TEXT PRIMARY KEY,
			ticker TEXT NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS eod (
			composite_figi TEXT NOT NULL REFERENCES assets(composite_figi),
			event_date TEXT NOT NULL,
			open REAL,
			high REAL,
			low REAL,
			close REAL,
			adj_close REAL,
			volume REAL,
			dividend REAL,
			split_factor REAL,
			PRIMARY KEY (composite_figi, event_date)
		)`,

		`CREATE TABLE IF NOT EXISTS metrics (
			composite_figi TEXT NOT NULL REFERENCES assets(composite_figi),
			event_date TEXT NOT NULL,
			market_cap INTEGER,
			ev INTEGER,
			pe REAL,
			pb REAL,
			ps REAL,
			ev_ebit REAL,
			ev_ebitda REAL,
			pe_forward REAL,
			peg REAL,
			price_to_cash_flow REAL,
			beta REAL,
			PRIMARY KEY (composite_figi, event_date)
		)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS fundamentals (
			composite_figi TEXT NOT NULL REFERENCES assets(composite_figi),
			event_date TEXT NOT NULL,
			dimension TEXT NOT NULL,
			%s,
			PRIMARY KEY (composite_figi, event_date, dimension)
		)`, fundamentalCols),

		`CREATE TABLE IF NOT EXISTS ratings (
			analyst TEXT NOT NULL,
			filter_values TEXT NOT NULL,
			event_date TEXT NOT NULL,
			composite_figi TEXT NOT NULL,
			ticker TEXT NOT NULL,
			PRIMARY KEY (analyst, filter_values, event_date, composite_figi)
		)`,

		`CREATE TABLE IF NOT EXISTS index_members (
			index_name TEXT NOT NULL,
			event_date TEXT NOT NULL,
			composite_figi TEXT NOT NULL REFERENCES assets(composite_figi),
			ticker TEXT NOT NULL,
			PRIMARY KEY (index_name, event_date, composite_figi)
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("snapshot schema: %w\nSQL: %s", err, stmt)
		}
	}

	return nil
}

// buildFundamentalColumns generates the SQL column definitions for the
// fundamentals table from the metricColumn map. Columns are sorted
// alphabetically for deterministic DDL output.
func buildFundamentalColumns() string {
	seen := make(map[string]bool)
	var cols []string

	for _, colName := range metricColumn {
		if seen[colName] {
			continue
		}
		seen[colName] = true
		cols = append(cols, colName)
	}

	sort.Strings(cols)

	defs := make([]string, len(cols))
	for idx, col := range cols {
		defs[idx] = fmt.Sprintf("%s REAL", col)
	}

	return strings.Join(defs, ",\n\t\t\t")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "SnapshotSchema" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add data/snapshot_schema.go data/snapshot_schema_test.go
git commit -m "feat: add SQLite schema for snapshot testing"
```

---

## Chunk 2: SnapshotRecorder

### Task 3: SnapshotRecorder -- asset recording

**Files:**
- Create: `data/snapshot_recorder.go`
- Test: `data/snapshot_recorder_test.go`

Start with the constructor and AssetProvider recording, since assets are the foundation for all other tables.

- [ ] **Step 1: Write the failing test**

Create `data/snapshot_recorder_test.go`:

```go
package data_test

import (
	"context"
	"database/sql"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"

	_ "modernc.org/sqlite"
)

var _ = Describe("SnapshotRecorder", func() {
	var (
		ctx      context.Context
		recorder *data.SnapshotRecorder
		dbPath   string
	)

	BeforeEach(func() {
		ctx = context.Background()
		tmpDir := GinkgoT().TempDir()
		dbPath = tmpDir + "/test-snapshot.db"
	})

	AfterEach(func() {
		if recorder != nil {
			Expect(recorder.Close()).To(Succeed())
		}
	})

	Describe("asset recording", func() {
		It("records assets from Assets() call", func() {
			stubAssets := []asset.Asset{
				{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"},
				{CompositeFigi: "BBG000BHTK15", Ticker: "TLT"},
			}
			stub := &stubAssetProvider{assets: stubAssets}

			var err error
			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				AssetProvider: stub,
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := recorder.Assets(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(stubAssets))

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			// Verify data was written to SQLite.
			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var count int
			Expect(db.QueryRow("SELECT count(*) FROM assets").Scan(&count)).To(Succeed())
			Expect(count).To(Equal(2))
		})

		It("records asset from LookupAsset() call", func() {
			expected := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			stub := &stubAssetProvider{lookupResult: expected}

			var err error
			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				AssetProvider: stub,
			})
			Expect(err).NotTo(HaveOccurred())

			result, err := recorder.LookupAsset(ctx, "SPY")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(expected))

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var count int
			Expect(db.QueryRow("SELECT count(*) FROM assets").Scan(&count)).To(Succeed())
			Expect(count).To(Equal(1))
		})
	})
})

// -- stubs --

type stubAssetProvider struct {
	assets       []asset.Asset
	lookupResult asset.Asset
}

func (s *stubAssetProvider) Assets(ctx context.Context) ([]asset.Asset, error) {
	return s.assets, nil
}

func (s *stubAssetProvider) LookupAsset(ctx context.Context, ticker string) (asset.Asset, error) {
	return s.lookupResult, nil
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "SnapshotRecorder" -v`
Expected: FAIL -- `data.NewSnapshotRecorder` does not exist.

- [ ] **Step 3: Write the implementation**

Create `data/snapshot_recorder.go`:

```go
package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"

	_ "modernc.org/sqlite"
)

// Compile-time interface checks.
var (
	_ BatchProvider  = (*SnapshotRecorder)(nil)
	_ AssetProvider  = (*SnapshotRecorder)(nil)
	_ IndexProvider  = (*SnapshotRecorder)(nil)
	_ RatingProvider = (*SnapshotRecorder)(nil)
)

// SnapshotRecorderConfig holds the providers to wrap.
type SnapshotRecorderConfig struct {
	BatchProvider  BatchProvider
	AssetProvider  AssetProvider
	IndexProvider  IndexProvider  // optional
	RatingProvider RatingProvider // optional
}

// SnapshotRecorder wraps real data providers, delegates every call, and
// writes the results to a SQLite snapshot database.
type SnapshotRecorder struct {
	db             *sql.DB
	batchProvider  BatchProvider
	assetProvider  AssetProvider
	indexProvider  IndexProvider
	ratingProvider RatingProvider
}

// NewSnapshotRecorder opens (or creates) the SQLite file at path, creates
// the snapshot schema, and returns a recorder ready to wrap provider calls.
func NewSnapshotRecorder(path string, cfg SnapshotRecorderConfig) (*SnapshotRecorder, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("snapshot recorder: open database: %w", err)
	}

	if err := CreateSnapshotSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("snapshot recorder: create schema: %w", err)
	}

	return &SnapshotRecorder{
		db:             db,
		batchProvider:  cfg.BatchProvider,
		assetProvider:  cfg.AssetProvider,
		indexProvider:  cfg.IndexProvider,
		ratingProvider: cfg.RatingProvider,
	}, nil
}

// Close closes the underlying SQLite database.
func (r *SnapshotRecorder) Close() error {
	return r.db.Close()
}

// -- AssetProvider --

// Assets delegates to the inner AssetProvider and records the results.
func (r *SnapshotRecorder) Assets(ctx context.Context) ([]asset.Asset, error) {
	if r.assetProvider == nil {
		return nil, nil
	}

	assets, err := r.assetProvider.Assets(ctx)
	if err != nil {
		return nil, err
	}

	if err := r.recordAssets(assets); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record assets: %w", err)
	}

	return assets, nil
}

// LookupAsset delegates to the inner AssetProvider and records the result.
func (r *SnapshotRecorder) LookupAsset(ctx context.Context, ticker string) (asset.Asset, error) {
	if r.assetProvider == nil {
		return asset.Asset{}, fmt.Errorf("snapshot recorder: no asset provider configured")
	}

	result, err := r.assetProvider.LookupAsset(ctx, ticker)
	if err != nil {
		return asset.Asset{}, err
	}

	if err := r.recordAssets([]asset.Asset{result}); err != nil {
		return asset.Asset{}, fmt.Errorf("snapshot recorder: record asset: %w", err)
	}

	return result, nil
}

func (r *SnapshotRecorder) recordAssets(assets []asset.Asset) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO assets (composite_figi, ticker) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range assets {
		if _, err := stmt.Exec(a.CompositeFigi, a.Ticker); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// -- BatchProvider --

// Provides delegates to the inner BatchProvider.
func (r *SnapshotRecorder) Provides() []Metric {
	if r.batchProvider == nil {
		return nil
	}

	return r.batchProvider.Provides()
}

// Fetch delegates to the inner BatchProvider and records the results.
func (r *SnapshotRecorder) Fetch(ctx context.Context, req DataRequest) (*DataFrame, error) {
	if r.batchProvider == nil {
		return nil, fmt.Errorf("snapshot recorder: no batch provider configured")
	}

	df, err := r.batchProvider.Fetch(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := r.recordDataFrame(df, req.Metrics); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record data: %w", err)
	}

	return df, nil
}

func (r *SnapshotRecorder) recordDataFrame(df *DataFrame, requestedMetrics []Metric) error {
	if df == nil || len(df.times) == 0 {
		return nil
	}

	// Ensure all assets in the frame are in the assets table.
	if err := r.recordAssets(df.assets); err != nil {
		return err
	}

	// Group requested metrics by view.
	viewMetrics := make(map[string][]Metric)
	for _, metric := range requestedMetrics {
		view, ok := metricView[metric]
		if !ok {
			continue
		}
		viewMetrics[view] = append(viewMetrics[view], metric)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if metrics, ok := viewMetrics["eod"]; ok {
		if err := r.recordEod(tx, df, metrics); err != nil {
			return err
		}
	}

	if metrics, ok := viewMetrics["metrics"]; ok {
		if err := r.recordMetrics(tx, df, metrics); err != nil {
			return err
		}
	}

	if metrics, ok := viewMetrics["fundamentals"]; ok {
		if err := r.recordFundamentals(tx, df, metrics); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *SnapshotRecorder) recordEod(tx *sql.Tx, df *DataFrame, metrics []Metric) error {
	want := metricSet(metrics)
	numTimes := len(df.times)
	numMetrics := len(df.metrics)

	// Build metric index for the DataFrame.
	mIdx := make(map[Metric]int, len(df.metrics))
	for idx, metric := range df.metrics {
		mIdx[metric] = idx
	}

	// getValue returns the value for a metric, or nil if the metric was
	// not requested or the value is NaN. This ensures unrequested metrics
	// are stored as NULL rather than zero.
	type nullableFloat = interface{}

	for assetIdx, a := range df.assets {
		for timeIdx, timestamp := range df.times {
			dateStr := timestamp.Format(time.RFC3339)

			getValue := func(metric Metric) nullableFloat {
				if !want[metric] {
					return nil
				}
				mi, ok := mIdx[metric]
				if !ok {
					return nil
				}
				colStart := (assetIdx*numMetrics + mi) * numTimes
				val := df.data[colStart+timeIdx]
				if math.IsNaN(val) {
					return nil
				}
				return val
			}

			_, err := tx.Exec(
				`INSERT OR REPLACE INTO eod
				 (composite_figi, event_date, open, high, low, close, adj_close, volume, dividend, split_factor)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				a.CompositeFigi, dateStr,
				getValue(MetricOpen), getValue(MetricHigh), getValue(MetricLow),
				getValue(MetricClose), getValue(AdjClose), getValue(Volume),
				getValue(Dividend), getValue(SplitFactor),
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *SnapshotRecorder) recordMetrics(tx *sql.Tx, df *DataFrame, metrics []Metric) error {
	numTimes := len(df.times)
	numMetrics := len(df.metrics)

	mIdx := make(map[Metric]int, len(df.metrics))
	for idx, metric := range df.metrics {
		mIdx[metric] = idx
	}

	for assetIdx, a := range df.assets {
		for timeIdx, timestamp := range df.times {
			dateStr := timestamp.Format(time.RFC3339)

			getValue := func(metric Metric) interface{} {
				mi, ok := mIdx[metric]
				if !ok {
					return nil
				}
				colStart := (assetIdx*numMetrics + mi) * numTimes
				val := df.data[colStart+timeIdx]
				if math.IsNaN(val) {
					return nil
				}
				return val
			}

			_, err := tx.Exec(
				`INSERT OR REPLACE INTO metrics
				 (composite_figi, event_date, market_cap, ev, pe, pb, ps, ev_ebit, ev_ebitda, pe_forward, peg, price_to_cash_flow, beta)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				a.CompositeFigi, dateStr,
				getValue(MarketCap), getValue(EnterpriseValue),
				getValue(PE), getValue(PB), getValue(PS),
				getValue(EVtoEBIT), getValue(EVtoEBITDA),
				getValue(ForwardPE), getValue(PEG),
				getValue(PriceToCashFlow), getValue(Beta),
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *SnapshotRecorder) recordFundamentals(tx *sql.Tx, df *DataFrame, metrics []Metric) error {
	numTimes := len(df.times)
	numDFMetrics := len(df.metrics)

	mIdx := make(map[Metric]int, len(df.metrics))
	for idx, metric := range df.metrics {
		mIdx[metric] = idx
	}

	// Build sorted column list from the metrics we have data for.
	var colNames []string
	var colMetrics []Metric

	for _, metric := range metrics {
		colName, ok := metricColumn[metric]
		if !ok {
			continue
		}
		colNames = append(colNames, colName)
		colMetrics = append(colMetrics, metric)
	}

	if len(colNames) == 0 {
		return nil
	}

	placeholders := make([]string, 3+len(colNames))
	placeholders[0] = "?"
	placeholders[1] = "?"
	placeholders[2] = "?"
	for idx := range colNames {
		placeholders[3+idx] = "?"
	}

	query := fmt.Sprintf(
		"INSERT OR REPLACE INTO fundamentals (composite_figi, event_date, dimension, %s) VALUES (%s)",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "),
	)

	for assetIdx, a := range df.assets {
		for timeIdx, timestamp := range df.times {
			dateStr := timestamp.Format(time.RFC3339)

			args := make([]interface{}, 3+len(colMetrics))
			args[0] = a.CompositeFigi
			args[1] = dateStr
			args[2] = "ARQ" // default dimension

			for idx, metric := range colMetrics {
				mi, ok := mIdx[metric]
				if !ok {
					args[3+idx] = nil
					continue
				}
				colStart := (assetIdx*numDFMetrics + mi) * numTimes
				val := df.data[colStart+timeIdx]
				if math.IsNaN(val) {
					args[3+idx] = nil
				} else {
					args[3+idx] = val
				}
			}

			if _, err := tx.Exec(query, args...); err != nil {
				return err
			}
		}
	}

	return nil
}

// -- IndexProvider --

// IndexMembers delegates to the inner IndexProvider and records the results.
func (r *SnapshotRecorder) IndexMembers(ctx context.Context, index string, t time.Time) ([]asset.Asset, error) {
	if r.indexProvider == nil {
		return nil, nil
	}

	members, err := r.indexProvider.IndexMembers(ctx, index, t)
	if err != nil {
		return nil, err
	}

	if err := r.recordAssets(members); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record index member assets: %w", err)
	}

	if err := r.recordIndexMembers(index, t, members); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record index members: %w", err)
	}

	return members, nil
}

func (r *SnapshotRecorder) recordIndexMembers(index string, t time.Time, members []asset.Asset) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO index_members (index_name, event_date, composite_figi, ticker) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	dateStr := t.Format(time.RFC3339)
	for _, a := range members {
		if _, err := stmt.Exec(index, dateStr, a.CompositeFigi, a.Ticker); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// -- RatingProvider --

// RatedAssets delegates to the inner RatingProvider and records the results.
func (r *SnapshotRecorder) RatedAssets(ctx context.Context, analyst string, filter RatingFilter, t time.Time) ([]asset.Asset, error) {
	if r.ratingProvider == nil {
		return nil, nil
	}

	assets, err := r.ratingProvider.RatedAssets(ctx, analyst, filter, t)
	if err != nil {
		return nil, err
	}

	if err := r.recordAssets(assets); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record rated assets: %w", err)
	}

	if err := r.recordRatedAssets(analyst, filter, t, assets); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record ratings: %w", err)
	}

	return assets, nil
}

func (r *SnapshotRecorder) recordRatedAssets(analyst string, filter RatingFilter, t time.Time, assets []asset.Asset) error {
	filterJSON, err := json.Marshal(filter.Values)
	if err != nil {
		return fmt.Errorf("marshal filter values: %w", err)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO ratings (analyst, filter_values, event_date, composite_figi, ticker) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	dateStr := t.Format(time.RFC3339)
	for _, a := range assets {
		if _, err := stmt.Exec(analyst, string(filterJSON), dateStr, a.CompositeFigi, a.Ticker); err != nil {
			return err
		}
	}

	return tx.Commit()
}
```

Note: The fields `df.times`, `df.assets`, `df.metrics`, `df.data` are unexported but accessible within the `data` package since `snapshot_recorder.go` lives in `package data`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "SnapshotRecorder" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add data/snapshot_recorder.go data/snapshot_recorder_test.go
git commit -m "feat: add SnapshotRecorder with asset recording"
```

---

### Task 4: SnapshotRecorder -- batch data recording

**Files:**
- Modify: `data/snapshot_recorder_test.go`

Add tests for `Fetch` recording across eod, metrics, and fundamentals tables.

- [ ] **Step 1: Write the failing test for eod recording**

Add to `data/snapshot_recorder_test.go`:

```go
Describe("batch data recording", func() {
	It("records eod data from Fetch() call", func() {
		// Build a DataFrame with 2 assets, 2 dates, 2 eod metrics.
		spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
		tlt := asset.Asset{CompositeFigi: "BBG000BHTK15", Ticker: "TLT"}
		assets := []asset.Asset{spy, tlt}
		metrics := []data.Metric{data.MetricClose, data.AdjClose}

		nyc, err := time.LoadLocation("America/New_York")
		Expect(err).NotTo(HaveOccurred())

		times := []time.Time{
			time.Date(2024, 1, 2, 16, 0, 0, 0, nyc),
			time.Date(2024, 1, 3, 16, 0, 0, 0, nyc),
		}

		// Column-major layout: [spy_close_t0, spy_close_t1, spy_adjclose_t0, spy_adjclose_t1,
		//                       tlt_close_t0, tlt_close_t1, tlt_adjclose_t0, tlt_adjclose_t1]
		values := []float64{
			100.0, 101.0, 99.0, 100.0,
			50.0, 51.0, 49.0, 50.0,
		}

		df, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
		Expect(err).NotTo(HaveOccurred())

		stub := data.NewTestProvider(metrics, df)

		recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
			BatchProvider: stub,
			AssetProvider: &stubAssetProvider{assets: assets},
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := recorder.Fetch(ctx, data.DataRequest{
			Assets:    assets,
			Metrics:   metrics,
			Start:     times[0],
			End:       times[1],
			Frequency: data.Daily,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		Expect(recorder.Close()).To(Succeed())
		recorder = nil

		db, err := sql.Open("sqlite", dbPath)
		Expect(err).NotTo(HaveOccurred())
		defer db.Close()

		var count int
		Expect(db.QueryRow("SELECT count(*) FROM eod").Scan(&count)).To(Succeed())
		Expect(count).To(Equal(4)) // 2 assets * 2 dates

		Expect(db.QueryRow("SELECT count(*) FROM assets").Scan(&count)).To(Succeed())
		Expect(count).To(Equal(2))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "batch data recording" -v`
Expected: FAIL -- recording logic may not compile or may fail.

- [ ] **Step 3: Fix any issues and make the test pass**

The implementation from Task 3 should handle this. If DataFrame field access needs adjustment, fix it now. The key issue is likely that `df.times`, `df.assets`, `df.metrics`, `df.data` are unexported. Use public accessors or add them as needed.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "SnapshotRecorder" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add data/snapshot_recorder.go data/snapshot_recorder_test.go
git commit -m "feat: add batch data recording to SnapshotRecorder"
```

---

### Task 5: SnapshotRecorder -- index and rating recording

**Files:**
- Modify: `data/snapshot_recorder_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `data/snapshot_recorder_test.go`:

```go
Describe("index member recording", func() {
	It("records index members from IndexMembers() call", func() {
		members := []asset.Asset{
			{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"},
		}

		nyc, _ := time.LoadLocation("America/New_York")
		date := time.Date(2024, 1, 2, 16, 0, 0, 0, nyc)

		stub := &stubIndexProvider{members: members}

		var err error
		recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
			IndexProvider: stub,
			AssetProvider: &stubAssetProvider{assets: members},
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := recorder.IndexMembers(ctx, "SP500", date)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(members))

		Expect(recorder.Close()).To(Succeed())
		recorder = nil

		db, err := sql.Open("sqlite", dbPath)
		Expect(err).NotTo(HaveOccurred())
		defer db.Close()

		var count int
		Expect(db.QueryRow("SELECT count(*) FROM index_members").Scan(&count)).To(Succeed())
		Expect(count).To(Equal(1))
	})
})

Describe("rating recording", func() {
	It("records rated assets from RatedAssets() call", func() {
		rated := []asset.Asset{
			{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"},
		}

		nyc, _ := time.LoadLocation("America/New_York")
		date := time.Date(2024, 1, 2, 16, 0, 0, 0, nyc)

		stub := &stubRatingProvider{assets: rated}

		var err error
		recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
			RatingProvider: stub,
			AssetProvider:  &stubAssetProvider{assets: rated},
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := recorder.RatedAssets(ctx, "morningstar", data.RatingEq(5), date)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(rated))

		Expect(recorder.Close()).To(Succeed())
		recorder = nil

		db, err := sql.Open("sqlite", dbPath)
		Expect(err).NotTo(HaveOccurred())
		defer db.Close()

		var count int
		Expect(db.QueryRow("SELECT count(*) FROM ratings").Scan(&count)).To(Succeed())
		Expect(count).To(Equal(1))
	})
})
```

Also add stub types at the bottom of the test file:

```go
type stubIndexProvider struct {
	members []asset.Asset
}

func (s *stubIndexProvider) IndexMembers(ctx context.Context, index string, t time.Time) ([]asset.Asset, error) {
	return s.members, nil
}

type stubRatingProvider struct {
	assets []asset.Asset
}

func (s *stubRatingProvider) RatedAssets(ctx context.Context, analyst string, filter data.RatingFilter, t time.Time) ([]asset.Asset, error) {
	return s.assets, nil
}
```

- [ ] **Step 2: Run tests to verify they fail then pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "SnapshotRecorder" -v`
Expected: PASS (implementation from Task 3 already handles these).

- [ ] **Step 3: Commit**

```bash
git add data/snapshot_recorder_test.go
git commit -m "test: add index and rating recording tests for SnapshotRecorder"
```

---

### Task 6: SnapshotRecorder -- nil provider handling

**Files:**
- Modify: `data/snapshot_recorder_test.go`

- [ ] **Step 1: Write tests for nil providers**

```go
Describe("nil provider handling", func() {
	It("returns empty slice for IndexMembers when no IndexProvider", func() {
		var err error
		recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
			AssetProvider: &stubAssetProvider{},
		})
		Expect(err).NotTo(HaveOccurred())

		nyc, _ := time.LoadLocation("America/New_York")
		result, err := recorder.IndexMembers(ctx, "SP500", time.Date(2024, 1, 2, 16, 0, 0, 0, nyc))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})

	It("returns empty slice for RatedAssets when no RatingProvider", func() {
		var err error
		recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
			AssetProvider: &stubAssetProvider{},
		})
		Expect(err).NotTo(HaveOccurred())

		nyc, _ := time.LoadLocation("America/New_York")
		result, err := recorder.RatedAssets(ctx, "morningstar", data.RatingEq(5), time.Date(2024, 1, 2, 16, 0, 0, 0, nyc))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})
})
```

- [ ] **Step 2: Run and verify**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "nil provider" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add data/snapshot_recorder_test.go
git commit -m "test: add nil provider handling tests for SnapshotRecorder"
```

---

## Chunk 3: SnapshotProvider

### Task 7: SnapshotProvider -- asset replay

**Files:**
- Create: `data/snapshot_provider.go`
- Test: `data/snapshot_provider_test.go`

- [ ] **Step 1: Write the failing test**

Create `data/snapshot_provider_test.go`:

```go
package data_test

import (
	"context"
	"database/sql"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"

	_ "modernc.org/sqlite"
)

var _ = Describe("SnapshotProvider", func() {
	var (
		ctx    context.Context
		dbPath string
	)

	BeforeEach(func() {
		ctx = context.Background()
		dbPath = GinkgoT().TempDir() + "/test-snapshot.db"
	})

	// Helper: seed a snapshot database with known data.
	seedDB := func() {
		db, err := sql.Open("sqlite", dbPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(data.CreateSnapshotSchema(db)).To(Succeed())

		_, err = db.Exec("INSERT INTO assets (composite_figi, ticker) VALUES ('BBG000BLNNH6', 'SPY'), ('BBG000BHTK15', 'TLT')")
		Expect(err).NotTo(HaveOccurred())
		db.Close()
	}

	Describe("Assets", func() {
		It("returns all assets from the snapshot", func() {
			seedDB()

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			assets, err := snap.Assets(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(assets).To(HaveLen(2))
			Expect(assets[0].Ticker).To(Equal("SPY"))
		})
	})

	Describe("LookupAsset", func() {
		It("finds an asset by ticker", func() {
			seedDB()

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			result, err := snap.LookupAsset(ctx, "SPY")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.CompositeFigi).To(Equal("BBG000BLNNH6"))
		})

		It("returns error for unknown ticker", func() {
			seedDB()

			snap, err := data.NewSnapshotProvider(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer snap.Close()

			_, err = snap.LookupAsset(ctx, "NOPE")
			Expect(err).To(HaveOccurred())
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "SnapshotProvider" -v`
Expected: FAIL -- `data.NewSnapshotProvider` does not exist.

- [ ] **Step 3: Write the implementation**

Create `data/snapshot_provider.go`:

```go
package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"

	_ "modernc.org/sqlite"
)

// Compile-time interface checks.
var (
	_ BatchProvider  = (*SnapshotProvider)(nil)
	_ AssetProvider  = (*SnapshotProvider)(nil)
	_ IndexProvider  = (*SnapshotProvider)(nil)
	_ RatingProvider = (*SnapshotProvider)(nil)
)

// SnapshotProvider replays data from a snapshot SQLite database.
type SnapshotProvider struct {
	db *sql.DB
}

// NewSnapshotProvider opens the snapshot database at path in read-only mode.
func NewSnapshotProvider(path string) (*SnapshotProvider, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: open database: %w", err)
	}

	// Set read-only mode via pragma (modernc.org/sqlite does not support
	// ?mode=ro in the DSN).
	if _, err := db.Exec("PRAGMA query_only = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("snapshot provider: set read-only: %w", err)
	}

	return &SnapshotProvider{db: db}, nil
}

// Close closes the database connection.
func (p *SnapshotProvider) Close() error {
	return p.db.Close()
}

// -- AssetProvider --

func (p *SnapshotProvider) Assets(ctx context.Context) ([]asset.Asset, error) {
	rows, err := p.db.QueryContext(ctx, "SELECT composite_figi, ticker FROM assets ORDER BY ticker")
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: query assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset
	for rows.Next() {
		var a asset.Asset
		if err := rows.Scan(&a.CompositeFigi, &a.Ticker); err != nil {
			return nil, fmt.Errorf("snapshot provider: scan asset: %w", err)
		}
		assets = append(assets, a)
	}

	return assets, rows.Err()
}

func (p *SnapshotProvider) LookupAsset(ctx context.Context, ticker string) (asset.Asset, error) {
	var a asset.Asset
	err := p.db.QueryRowContext(ctx,
		"SELECT composite_figi, ticker FROM assets WHERE ticker = ? LIMIT 1", ticker,
	).Scan(&a.CompositeFigi, &a.Ticker)
	if err != nil {
		return asset.Asset{}, fmt.Errorf("snapshot provider: lookup asset %q: %w", ticker, err)
	}

	return a, nil
}

// -- BatchProvider --

func (p *SnapshotProvider) Provides() []Metric {
	tables := []string{"eod", "metrics", "fundamentals"}
	var result []Metric

	for _, table := range tables {
		var count int
		if err := p.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count == 0 {
			continue
		}

		for metric, view := range metricView {
			if view == table {
				result = append(result, metric)
			}
		}
	}

	// Sort for deterministic output.
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })

	return result
}

func (p *SnapshotProvider) Fetch(ctx context.Context, req DataRequest) (*DataFrame, error) {
	// Group requested metrics by view.
	viewMetrics := make(map[string][]Metric)
	for _, metric := range req.Metrics {
		view, ok := metricView[metric]
		if !ok {
			continue
		}
		viewMetrics[view] = append(viewMetrics[view], metric)
	}

	figis := make([]string, len(req.Assets))
	for idx, a := range req.Assets {
		figis[idx] = a.CompositeFigi
	}

	type colKey struct {
		figi   string
		metric Metric
	}

	colData := make(map[colKey]map[int64]float64)
	timeSet := make(map[int64]time.Time)

	ensureCol := func(figi string, m Metric) map[int64]float64 {
		key := colKey{figi, m}
		if c, ok := colData[key]; ok {
			return c
		}
		c := make(map[int64]float64)
		colData[key] = c
		return c
	}

	startStr := req.Start.Format(time.RFC3339)
	endStr := req.End.Format(time.RFC3339)

	if metrics, ok := viewMetrics["eod"]; ok {
		if err := p.fetchEod(ctx, figis, startStr, endStr, metrics, ensureCol, timeSet); err != nil {
			return nil, err
		}
	}

	if metrics, ok := viewMetrics["metrics"]; ok {
		if err := p.fetchMetrics(ctx, figis, startStr, endStr, metrics, ensureCol, timeSet); err != nil {
			return nil, err
		}
	}

	if metrics, ok := viewMetrics["fundamentals"]; ok {
		if err := p.fetchFundamentals(ctx, figis, startStr, endStr, metrics, ensureCol, timeSet); err != nil {
			return nil, err
		}
	}

	// Build sorted time axis.
	times := make([]time.Time, 0, len(timeSet))
	for _, t := range timeSet {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	if len(times) == 0 {
		return NewDataFrame(nil, nil, nil, req.Frequency, nil)
	}

	timeIdx := make(map[int64]int, len(times))
	for idx, t := range times {
		timeIdx[t.Unix()] = idx
	}

	numTimes := len(times)
	numMetrics := len(req.Metrics)

	slab := make([]float64, numTimes*len(req.Assets)*numMetrics)
	for idx := range slab {
		slab[idx] = math.NaN()
	}

	aIdx := make(map[string]int, len(req.Assets))
	for idx, a := range req.Assets {
		aIdx[a.CompositeFigi] = idx
	}

	mIdx := make(map[Metric]int, numMetrics)
	for idx, m := range req.Metrics {
		mIdx[m] = idx
	}

	for key, vals := range colData {
		ai, ok := aIdx[key.figi]
		if !ok {
			continue
		}
		mi, ok := mIdx[key.metric]
		if !ok {
			continue
		}
		colStart := (ai*numMetrics + mi) * numTimes
		for sec, val := range vals {
			ti := timeIdx[sec]
			slab[colStart+ti] = val
		}
	}

	return NewDataFrame(times, req.Assets, req.Metrics, req.Frequency, slab)
}

func (p *SnapshotProvider) fetchEod(
	ctx context.Context,
	figis []string,
	startStr, endStr string,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	placeholders := make([]string, len(figis))
	args := make([]interface{}, len(figis)+2)
	for idx, figi := range figis {
		placeholders[idx] = "?"
		args[idx] = figi
	}
	args[len(figis)] = startStr
	args[len(figis)+1] = endStr

	query := fmt.Sprintf(
		`SELECT composite_figi, event_date, open, high, low, close, adj_close, volume, dividend, split_factor
		 FROM eod
		 WHERE composite_figi IN (%s) AND event_date BETWEEN ? AND ?
		 ORDER BY event_date`,
		strings.Join(placeholders, ","),
	)

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("snapshot provider: query eod: %w", err)
	}
	defer rows.Close()

	want := metricSet(metrics)

	type eodCol struct {
		metric Metric
	}

	columns := []eodCol{
		{MetricOpen}, {MetricHigh}, {MetricLow}, {MetricClose},
		{AdjClose}, {Volume}, {Dividend}, {SplitFactor},
	}

	vals := make([]sql.NullFloat64, len(columns))

	for rows.Next() {
		var (
			figi    string
			dateStr string
		)

		scanArgs := make([]interface{}, 0, 2+len(columns))
		scanArgs = append(scanArgs, &figi, &dateStr)
		for idx := range columns {
			scanArgs = append(scanArgs, &vals[idx])
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return fmt.Errorf("snapshot provider: scan eod: %w", err)
		}

		t, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			return fmt.Errorf("snapshot provider: parse eod date: %w", err)
		}

		sec := t.Unix()
		timeSet[sec] = t

		for idx, col := range columns {
			if !want[col.metric] {
				continue
			}
			if vals[idx].Valid {
				ensureCol(figi, col.metric)[sec] = vals[idx].Float64
			}
		}
	}

	return rows.Err()
}

// fetchMetrics and fetchFundamentals follow the same pattern as fetchEod
// but read from the metrics and fundamentals tables respectively.
// (Full implementations follow the same structure as PVDataProvider.)

func (p *SnapshotProvider) fetchMetrics(
	ctx context.Context,
	figis []string,
	startStr, endStr string,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	placeholders := make([]string, len(figis))
	args := make([]interface{}, len(figis)+2)
	for idx, figi := range figis {
		placeholders[idx] = "?"
		args[idx] = figi
	}
	args[len(figis)] = startStr
	args[len(figis)+1] = endStr

	query := fmt.Sprintf(
		`SELECT composite_figi, event_date,
		        market_cap, ev, pe, pb, ps, ev_ebit, ev_ebitda,
		        pe_forward, peg, price_to_cash_flow, beta
		 FROM metrics
		 WHERE composite_figi IN (%s) AND event_date BETWEEN ? AND ?
		 ORDER BY event_date`,
		strings.Join(placeholders, ","),
	)

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("snapshot provider: query metrics: %w", err)
	}
	defer rows.Close()

	want := metricSet(metrics)

	type metricsCol struct {
		metric Metric
		intCol bool
	}

	columns := []metricsCol{
		{MarketCap, true}, {EnterpriseValue, true},
		{PE, false}, {PB, false}, {PS, false},
		{EVtoEBIT, false}, {EVtoEBITDA, false},
		{ForwardPE, false}, {PEG, false},
		{PriceToCashFlow, false}, {Beta, false},
	}

	intVals := make([]sql.NullInt64, len(columns))
	floatVals := make([]sql.NullFloat64, len(columns))

	for rows.Next() {
		var (
			figi    string
			dateStr string
		)

		scanArgs := make([]interface{}, 0, 2+len(columns))
		scanArgs = append(scanArgs, &figi, &dateStr)

		for idx, col := range columns {
			if col.intCol {
				scanArgs = append(scanArgs, &intVals[idx])
			} else {
				scanArgs = append(scanArgs, &floatVals[idx])
			}
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return fmt.Errorf("snapshot provider: scan metrics: %w", err)
		}

		t, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			return fmt.Errorf("snapshot provider: parse metrics date: %w", err)
		}

		sec := t.Unix()
		timeSet[sec] = t

		for idx, col := range columns {
			if !want[col.metric] {
				continue
			}
			if col.intCol {
				if intVals[idx].Valid {
					ensureCol(figi, col.metric)[sec] = float64(intVals[idx].Int64)
				}
			} else {
				if floatVals[idx].Valid {
					ensureCol(figi, col.metric)[sec] = floatVals[idx].Float64
				}
			}
		}
	}

	return rows.Err()
}

func (p *SnapshotProvider) fetchFundamentals(
	ctx context.Context,
	figis []string,
	startStr, endStr string,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	var sqlCols []string
	var metricOrder []Metric

	for _, metric := range metrics {
		col, ok := metricColumn[metric]
		if !ok {
			continue
		}
		sqlCols = append(sqlCols, col)
		metricOrder = append(metricOrder, metric)
	}

	if len(sqlCols) == 0 {
		return nil
	}

	placeholders := make([]string, len(figis))
	args := make([]interface{}, len(figis)+2)
	for idx, figi := range figis {
		placeholders[idx] = "?"
		args[idx] = figi
	}
	args[len(figis)] = startStr
	args[len(figis)+1] = endStr

	// Add dimension filter -- the recorder stores "ARQ" as the default.
	args = append(args, "ARQ")

	query := fmt.Sprintf(
		`SELECT composite_figi, event_date, %s
		 FROM fundamentals
		 WHERE composite_figi IN (%s) AND event_date BETWEEN ? AND ? AND dimension = ?
		 ORDER BY event_date`,
		strings.Join(sqlCols, ", "),
		strings.Join(placeholders, ","),
	)

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("snapshot provider: query fundamentals: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			figi    string
			dateStr string
		)

		vals := make([]interface{}, len(sqlCols)+2)
		vals[0] = &figi
		vals[1] = &dateStr

		floatVals := make([]sql.NullFloat64, len(sqlCols))
		for idx := range sqlCols {
			vals[idx+2] = &floatVals[idx]
		}

		if err := rows.Scan(vals...); err != nil {
			return fmt.Errorf("snapshot provider: scan fundamentals: %w", err)
		}

		t, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			return fmt.Errorf("snapshot provider: parse fundamentals date: %w", err)
		}

		sec := t.Unix()
		timeSet[sec] = t

		for idx, metric := range metricOrder {
			if floatVals[idx].Valid {
				ensureCol(figi, metric)[sec] = floatVals[idx].Float64
			}
		}
	}

	return rows.Err()
}

// -- IndexProvider --

func (p *SnapshotProvider) IndexMembers(ctx context.Context, index string, t time.Time) ([]asset.Asset, error) {
	dateStr := t.Format(time.RFC3339)

	rows, err := p.db.QueryContext(ctx,
		"SELECT composite_figi, ticker FROM index_members WHERE index_name = ? AND event_date = ?",
		index, dateStr,
	)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: query index members: %w", err)
	}
	defer rows.Close()

	var members []asset.Asset
	for rows.Next() {
		var a asset.Asset
		if err := rows.Scan(&a.CompositeFigi, &a.Ticker); err != nil {
			return nil, fmt.Errorf("snapshot provider: scan index member: %w", err)
		}
		members = append(members, a)
	}

	return members, rows.Err()
}

// -- RatingProvider --

func (p *SnapshotProvider) RatedAssets(ctx context.Context, analyst string, filter RatingFilter, t time.Time) ([]asset.Asset, error) {
	filterJSON, err := json.Marshal(filter.Values)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: marshal filter: %w", err)
	}

	dateStr := t.Format(time.RFC3339)

	rows, err := p.db.QueryContext(ctx,
		"SELECT composite_figi, ticker FROM ratings WHERE analyst = ? AND filter_values = ? AND event_date = ?",
		analyst, string(filterJSON), dateStr,
	)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: query ratings: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset
	for rows.Next() {
		var a asset.Asset
		if err := rows.Scan(&a.CompositeFigi, &a.Ticker); err != nil {
			return nil, fmt.Errorf("snapshot provider: scan rated asset: %w", err)
		}
		assets = append(assets, a)
	}

	return assets, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "SnapshotProvider" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add data/snapshot_provider.go data/snapshot_provider_test.go
git commit -m "feat: add SnapshotProvider for replaying snapshot data"
```

---

### Task 8: SnapshotProvider -- Fetch (eod) replay

**Files:**
- Modify: `data/snapshot_provider_test.go`

- [ ] **Step 1: Write the failing test**

Add to `data/snapshot_provider_test.go`:

```go
Describe("Fetch", func() {
	It("replays eod data as a DataFrame", func() {
		// Seed a snapshot with eod data via the recorder.
		spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
		assets := []asset.Asset{spy}
		metrics := []data.Metric{data.MetricClose, data.AdjClose}

		nyc, err := time.LoadLocation("America/New_York")
		Expect(err).NotTo(HaveOccurred())

		times := []time.Time{
			time.Date(2024, 1, 2, 16, 0, 0, 0, nyc),
			time.Date(2024, 1, 3, 16, 0, 0, 0, nyc),
		}

		values := []float64{100.0, 101.0, 99.0, 100.0}

		df, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
		Expect(err).NotTo(HaveOccurred())

		stub := data.NewTestProvider(metrics, df)
		recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
			BatchProvider: stub,
			AssetProvider: &stubAssetProvider{assets: assets},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = recorder.Fetch(ctx, data.DataRequest{
			Assets: assets, Metrics: metrics, Start: times[0], End: times[1],
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(recorder.Close()).To(Succeed())

		// Now replay.
		snap, err := data.NewSnapshotProvider(dbPath)
		Expect(err).NotTo(HaveOccurred())
		defer snap.Close()

		result, err := snap.Fetch(ctx, data.DataRequest{
			Assets:    assets,
			Metrics:   metrics,
			Start:     times[0],
			End:       times[1],
			Frequency: data.Daily,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		// Verify the replayed data matches the original.
		// Access the underlying data slab (within data_test package,
		// use exported accessors or test helpers).
		// SPY close at t0=100.0, t1=101.0; adj_close t0=99.0, t1=100.0
		spyClose := result.Column(spy, data.MetricClose)
		Expect(spyClose[0]).To(BeNumerically("~", 100.0, 0.001))
		Expect(spyClose[1]).To(BeNumerically("~", 101.0, 0.001))

		spyAdj := result.Column(spy, data.AdjClose)
		Expect(spyAdj[0]).To(BeNumerically("~", 99.0, 0.001))
		Expect(spyAdj[1]).To(BeNumerically("~", 100.0, 0.001))
	})
})
```

Note to implementer: adjust the value-checking assertions based on the actual DataFrame accessor API (e.g., `df.Column(asset, metric)` returns a `[]float64` slice).

- [ ] **Step 2: Run test and make it pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "SnapshotProvider/Fetch" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add data/snapshot_provider_test.go
git commit -m "test: add eod Fetch replay test for SnapshotProvider"
```

---

### Task 9: SnapshotProvider -- Provides()

**Files:**
- Modify: `data/snapshot_provider_test.go`

- [ ] **Step 1: Write the test**

```go
Describe("Provides", func() {
	It("returns metrics for tables that have data", func() {
		// Seed with eod data only.
		db, err := sql.Open("sqlite", dbPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(data.CreateSnapshotSchema(db)).To(Succeed())

		_, err = db.Exec("INSERT INTO assets (composite_figi, ticker) VALUES ('BBG000BLNNH6', 'SPY')")
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec("INSERT INTO eod (composite_figi, event_date, close) VALUES ('BBG000BLNNH6', '2024-01-02T16:00:00-05:00', 100.0)")
		Expect(err).NotTo(HaveOccurred())
		db.Close()

		snap, err := data.NewSnapshotProvider(dbPath)
		Expect(err).NotTo(HaveOccurred())
		defer snap.Close()

		provided := snap.Provides()
		Expect(provided).To(ContainElement(data.MetricClose))
		Expect(provided).To(ContainElement(data.MetricOpen))
		Expect(provided).NotTo(ContainElement(data.PE)) // metrics table is empty
	})

	It("returns empty when no tables have data", func() {
		db, err := sql.Open("sqlite", dbPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(data.CreateSnapshotSchema(db)).To(Succeed())
		db.Close()

		snap, err := data.NewSnapshotProvider(dbPath)
		Expect(err).NotTo(HaveOccurred())
		defer snap.Close()

		Expect(snap.Provides()).To(BeEmpty())
	})
})
```

- [ ] **Step 2: Run test**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "Provides" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add data/snapshot_provider_test.go
git commit -m "test: add Provides() tests for SnapshotProvider"
```

---

### Task 10: SnapshotProvider -- index and rating replay

**Files:**
- Modify: `data/snapshot_provider_test.go`

- [ ] **Step 1: Write the tests**

```go
Describe("IndexMembers", func() {
	It("replays recorded index members", func() {
		// Seed via recorder.
		members := []asset.Asset{{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}}
		nyc, _ := time.LoadLocation("America/New_York")
		date := time.Date(2024, 1, 2, 16, 0, 0, 0, nyc)

		recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
			IndexProvider: &stubIndexProvider{members: members},
			AssetProvider: &stubAssetProvider{assets: members},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = recorder.IndexMembers(ctx, "SP500", date)
		Expect(err).NotTo(HaveOccurred())
		Expect(recorder.Close()).To(Succeed())

		snap, err := data.NewSnapshotProvider(dbPath)
		Expect(err).NotTo(HaveOccurred())
		defer snap.Close()

		result, err := snap.IndexMembers(ctx, "SP500", date)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveLen(1))
		Expect(result[0].Ticker).To(Equal("SPY"))
	})
})

Describe("RatedAssets", func() {
	It("replays recorded rated assets", func() {
		rated := []asset.Asset{{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}}
		nyc, _ := time.LoadLocation("America/New_York")
		date := time.Date(2024, 1, 2, 16, 0, 0, 0, nyc)

		recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
			RatingProvider: &stubRatingProvider{assets: rated},
			AssetProvider:  &stubAssetProvider{assets: rated},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = recorder.RatedAssets(ctx, "morningstar", data.RatingEq(5), date)
		Expect(err).NotTo(HaveOccurred())
		Expect(recorder.Close()).To(Succeed())

		snap, err := data.NewSnapshotProvider(dbPath)
		Expect(err).NotTo(HaveOccurred())
		defer snap.Close()

		result, err := snap.RatedAssets(ctx, "morningstar", data.RatingEq(5), date)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveLen(1))
		Expect(result[0].Ticker).To(Equal("SPY"))
	})
})
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run "SnapshotProvider" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add data/snapshot_provider_test.go
git commit -m "test: add index and rating replay tests for SnapshotProvider"
```

---

## Chunk 4: CLI Command

### Task 11: Add snapshot CLI command

**Files:**
- Create: `cli/snapshot.go`
- Modify: `cli/run.go:42` (add `rootCmd.AddCommand(newSnapshotCmd(strategy))` after existing AddCommand calls)

- [ ] **Step 1: Create the snapshot command**

Create `cli/snapshot.go`:

```go
package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	_ "modernc.org/sqlite"
)

func newSnapshotCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Run a backtest and capture all data access to a snapshot file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSnapshot(strategy)
		},
	}

	now := time.Now()
	fiveYearsAgo := now.AddDate(-5, 0, 0)

	cmd.Flags().String("start", fiveYearsAgo.Format("2006-01-02"), "Backtest start date (YYYY-MM-DD)")
	cmd.Flags().String("end", now.Format("2006-01-02"), "Backtest end date (YYYY-MM-DD)")
	cmd.Flags().Float64("cash", 100000, "Initial cash balance")
	cmd.Flags().String("output", "", "Snapshot output path (default: pv-data-snapshot-{strategy}-{start}-{end}.db)")

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		log.Fatal().Err(err).Msg("failed to bind snapshot flags")
	}

	registerStrategyFlags(cmd, strategy)

	return cmd
}

func defaultSnapshotPath(strategyName string, start, end time.Time) string {
	return fmt.Sprintf("pv-data-snapshot-%s-%s-%s.db",
		strings.ToLower(strategyName),
		start.Format("20060102"),
		end.Format("20060102"),
	)
}

func runSnapshot(strategy engine.Strategy) error {
	ctx := log.Logger.WithContext(context.Background())

	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return fmt.Errorf("load America/New_York timezone: %w", err)
	}

	start, err := time.ParseInLocation("2006-01-02", viper.GetString("start"), nyc)
	if err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}

	end, err := time.ParseInLocation("2006-01-02", viper.GetString("end"), nyc)
	if err != nil {
		return fmt.Errorf("invalid end date: %w", err)
	}

	cash := viper.GetFloat64("cash")

	outputPath := viper.GetString("output")
	if outputPath == "" {
		outputPath = defaultSnapshotPath(strategy.Name(), start, end)
	}

	log.Info().
		Str("strategy", strategy.Name()).
		Time("start", start).
		Time("end", end).
		Str("output", outputPath).
		Msg("starting snapshot capture")

	applyStrategyFlags(strategy)

	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	recorder, err := data.NewSnapshotRecorder(outputPath, data.SnapshotRecorderConfig{
		BatchProvider: provider,
		AssetProvider: provider,
		// IndexProvider and RatingProvider are nil unless PVDataProvider
		// implements them in the future or the strategy registers its own.
	})
	if err != nil {
		provider.Close()
		return fmt.Errorf("create snapshot recorder: %w", err)
	}

	acct := portfolio.New(
		portfolio.WithCash(cash, start),
		portfolio.WithAllMetrics(),
	)

	eng := engine.New(strategy,
		engine.WithDataProvider(recorder),
		engine.WithAssetProvider(recorder),
		engine.WithAccount(acct),
	)

	// Do NOT defer eng.Close() here -- the engine would close the recorder
	// (its registered provider), causing a double-close when we also close
	// the recorder and the underlying provider below.

	_, err = eng.Backtest(ctx, start, end)
	if err != nil {
		// Engine.Close() only closes registered providers, and the recorder is
		// the only registered provider. Close recorder and underlying provider
		// directly -- no need to also close the engine.
		recorder.Close()
		provider.Close()
		return fmt.Errorf("backtest failed: %w", err)
	}

	// Close the recorder first (flushes SQLite), then the underlying provider
	// (releases the pgxpool). The engine does not own these lifetimes.
	if err := recorder.Close(); err != nil {
		provider.Close()
		return fmt.Errorf("close snapshot recorder: %w", err)
	}

	if err := provider.Close(); err != nil {
		return fmt.Errorf("close data provider: %w", err)
	}

	// Print summary with row counts per table.
	summaryDB, err := sql.Open("sqlite", outputPath)
	if err != nil {
		return fmt.Errorf("open snapshot for summary: %w", err)
	}
	defer summaryDB.Close()

	tables := []string{"assets", "eod", "metrics", "fundamentals", "ratings", "index_members"}
	for _, table := range tables {
		var count int
		if err := summaryDB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
			log.Warn().Err(err).Str("table", table).Msg("could not count rows")
			continue
		}
		if count > 0 {
			log.Info().Str("table", table).Int("rows", count).Msg("snapshot table")
		}
	}

	log.Info().Str("path", outputPath).Msg("snapshot written")

	return nil
}
```

- [ ] **Step 2: Register the command in cli/run.go**

Add `rootCmd.AddCommand(newSnapshotCmd(strategy))` after the existing `AddCommand` calls in `cli/run.go:42`.

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add cli/snapshot.go cli/run.go
git commit -m "feat: add snapshot CLI command for capturing test data"
```

---

## Chunk 5: Round-Trip Integration Test

### Task 12: End-to-end record-and-replay test

**Files:**
- Modify: `data/snapshot_provider_test.go`

This test records data through the recorder and replays it through the provider, verifying the full round trip produces identical DataFrames.

- [ ] **Step 1: Write the integration test**

Add to `data/snapshot_provider_test.go`:

```go
Describe("round-trip", func() {
	It("record then replay produces identical data", func() {
		spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
		tlt := asset.Asset{CompositeFigi: "BBG000BHTK15", Ticker: "TLT"}
		assets := []asset.Asset{spy, tlt}
		metrics := []data.Metric{data.MetricClose, data.AdjClose}

		nyc, err := time.LoadLocation("America/New_York")
		Expect(err).NotTo(HaveOccurred())

		times := []time.Time{
			time.Date(2024, 1, 2, 16, 0, 0, 0, nyc),
			time.Date(2024, 1, 3, 16, 0, 0, 0, nyc),
			time.Date(2024, 1, 4, 16, 0, 0, 0, nyc),
		}

		// 2 assets * 2 metrics * 3 times = 12 values
		values := []float64{
			100.0, 101.0, 102.0, // SPY close
			99.0, 100.0, 101.0,  // SPY adj_close
			50.0, 51.0, 52.0,    // TLT close
			49.0, 50.0, 51.0,    // TLT adj_close
		}

		originalDF, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
		Expect(err).NotTo(HaveOccurred())

		stub := data.NewTestProvider(metrics, originalDF)
		recorder, err := data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
			BatchProvider: stub,
			AssetProvider: &stubAssetProvider{assets: assets},
		})
		Expect(err).NotTo(HaveOccurred())

		req := data.DataRequest{
			Assets: assets, Metrics: metrics,
			Start: times[0], End: times[2], Frequency: data.Daily,
		}

		_, err = recorder.Fetch(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(recorder.Close()).To(Succeed())

		// Replay.
		snap, err := data.NewSnapshotProvider(dbPath)
		Expect(err).NotTo(HaveOccurred())
		defer snap.Close()

		replayedDF, err := snap.Fetch(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(replayedDF).NotTo(BeNil())

		// Compare all values element-by-element.
		for _, a := range assets {
			for _, m := range metrics {
				original := originalDF.Column(a, m)
				replayed := replayedDF.Column(a, m)
				Expect(len(replayed)).To(Equal(len(original)))
				for idx := range original {
					Expect(replayed[idx]).To(BeNumerically("~", original[idx], 0.001),
						"mismatch at %s/%s index %d", a.Ticker, m, idx)
				}
			}
		}
	})
})
```

Note to implementer: use the actual DataFrame accessor API to compare values element-by-element. If `DataFrame` exposes a `DataSlice()` or similar method, compare the full slabs. Otherwise iterate via `Column(asset, metric)`.

- [ ] **Step 2: Run all data package tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add data/snapshot_provider_test.go
git commit -m "test: add round-trip integration test for snapshot record/replay"
```

---

### Task 13: Run full test suite

- [ ] **Step 1: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./...`
Expected: ALL PASS

- [ ] **Step 2: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address issues found during full test suite run"
```
