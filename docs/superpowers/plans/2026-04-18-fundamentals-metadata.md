# Fundamentals Metadata and Date-Key Queries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose `date_key` and `report_period` as DataFrame metrics and add `Engine.FetchFundamentalsByDateKey` so strategies can request a specific reporting period (e.g. NCAVE Q1 working capital).

**Architecture:** Two new fundamental metrics (`FundamentalsDateKey`, `FundamentalsReportPeriod`) encoded as `float64(t.Unix())`. Provider extends `fetchFundamentals` to select the metadata columns and populate them like any other metric. Engine adds `FetchFundamentalsByDateKey` via a new optional capability interface (`FundamentalsByDateKeyProvider`) implemented by `PVDataProvider` and `SnapshotProvider`. Snapshot recorder reads metadata from the DataFrame columns when present and reads the dimension from the wrapped provider via a `Dimension() string` interface.

**Tech Stack:** Go, PostgreSQL (pgx), SQLite (modernc.org/sqlite), Ginkgo/Gomega.

**Spec:** `docs/superpowers/specs/2026-04-18-fundamentals-metadata-design.md`

---

### Task 1: Add `FundamentalsDateKey` and `FundamentalsReportPeriod` metric constants

**Files:**
- Modify: `data/metric.go`
- Modify: `data/pvdata_provider.go` (`metricView`, `metricColumn`)
- Modify: `data/metric_view_test.go`

- [ ] **Step 1: Write the failing test**

Append to `data/metric_view_test.go` inside the existing `Describe("IsFundamental", ...)` block (after the closing `)` of the last `It`, before the file-end `})`):

```go
	It("classifies fundamentals metadata metrics as fundamentals", func() {
		Expect(data.IsFundamental(data.FundamentalsDateKey)).To(BeTrue())
		Expect(data.IsFundamental(data.FundamentalsReportPeriod)).To(BeTrue())
	})
```

- [ ] **Step 2: Run test to verify it fails**

```bash
ginkgo run -race --focus "classifies fundamentals metadata metrics" ./data/
```

Expected: build error or FAIL — `data.FundamentalsDateKey` undefined.

- [ ] **Step 3: Add the constants**

In `data/metric.go`, add a new const block after the "Invested capital metrics" block (after the `)` that closes the `MarketCapFundamental` const, currently at the end of that block):

```go
// Fundamentals metadata metrics (fundamentals table date columns).
const (
	// FundamentalsDateKey is the normalized calendar quarter boundary for
	// the most recent fundamental filing as of the queried timestamp.
	// Values are encoded as float64(t.Unix()); convert with
	// time.Unix(int64(val), 0). NaN means no filing has been observed.
	FundamentalsDateKey Metric = "FundamentalsDateKey"

	// FundamentalsReportPeriod is the actual fiscal-period end date for
	// the most recent fundamental filing as of the queried timestamp.
	// Values are encoded as float64(t.Unix()); convert with
	// time.Unix(int64(val), 0). NaN means no filing has been observed.
	FundamentalsReportPeriod Metric = "FundamentalsReportPeriod"
)
```

- [ ] **Step 4: Register in `metricView`**

In `data/pvdata_provider.go`, add to the `// fundamentals view` block of `metricView` (immediately before the closing `}`, around line 1010 — alongside the other `"fundamentals"` entries):

```go
	FundamentalsDateKey:      "fundamentals",
	FundamentalsReportPeriod: "fundamentals",
```

- [ ] **Step 5: Register in `metricColumn`**

In `data/pvdata_provider.go`, add to the `metricColumn` map (the map starting at line 1020 — append two new entries):

```go
	FundamentalsDateKey:      "date_key",
	FundamentalsReportPeriod: "report_period",
```

- [ ] **Step 6: Run test to verify it passes**

```bash
ginkgo run -race --focus "classifies fundamentals metadata metrics" ./data/
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add data/metric.go data/pvdata_provider.go data/metric_view_test.go
git commit -m "feat: add FundamentalsDateKey and FundamentalsReportPeriod metrics"
```

---

### Task 2: Surface `report_period` and metadata cells from `fetchFundamentals`

**Goal:** the SELECT pulls all three date columns (`event_date`, `date_key`, `report_period`); when the request includes `FundamentalsDateKey` or `FundamentalsReportPeriod`, the corresponding date is encoded as `float64(t.Unix())` and stored in the cache the same way Revenue is.

**Files:**
- Modify: `data/pvdata_provider.go` (`fetchFundamentals`, around lines 647–760)

This task has no unit test — `fetchFundamentals` requires a live PostgreSQL connection. Coverage comes via the snapshot round-trip in Task 4 and the engine integration in Task 7.

- [ ] **Step 1: Read the current implementation**

Read `data/pvdata_provider.go` lines 647–760 to confirm the current scan layout. The current code SELECTs `composite_figi, event_date, date_key, {metric columns}` and scans into `figi`, `eventDate`, `dateKey`, then per-metric float pointers (the `dateKey` value is currently discarded).

- [ ] **Step 2: Filter metadata metrics out of the SQL metric loop**

Inside `fetchFundamentals`, when building `sqlCols`/`metricOrder`, the metadata metrics map to the date columns (`date_key`, `report_period`) which are already pulled by the SELECT separately — they must not be added a second time. Replace the loop that builds `sqlCols`/`metricOrder` with one that skips metadata metrics:

```go
	for _, metric := range metrics {
		if metric == FundamentalsDateKey || metric == FundamentalsReportPeriod {
			continue
		}

		col, ok := metricColumn[metric]
		if !ok {
			continue
		}

		sqlCols = append(sqlCols, col)
		metricOrder = append(metricOrder, metric)
	}
```

Then track which metadata columns are requested:

```go
	wantDateKey := false
	wantReportPeriod := false

	for _, metric := range metrics {
		switch metric {
		case FundamentalsDateKey:
			wantDateKey = true
		case FundamentalsReportPeriod:
			wantReportPeriod = true
		}
	}
```

If `len(sqlCols) == 0 && !wantDateKey && !wantReportPeriod`, return `nil` (current early-return preserved).

- [ ] **Step 3: Add `report_period` to the SELECT**

Replace the `query := fmt.Sprintf(...)` block with:

```go
	query := fmt.Sprintf(
		`SELECT composite_figi, event_date, date_key, report_period, %s
		 FROM fundamentals
		 WHERE composite_figi = ANY($1) AND event_date BETWEEN $2::date AND $3::date AND dimension = $4
		 ORDER BY event_date`,
		strings.Join(sqlCols, ", "),
	)
```

If `sqlCols` is empty (only metadata requested), the trailing `, %s` produces `, ` — strip the trailing comma by joining differently:

```go
	cols := []string{"composite_figi", "event_date", "date_key", "report_period"}
	cols = append(cols, sqlCols...)

	query := fmt.Sprintf(
		`SELECT %s
		 FROM fundamentals
		 WHERE composite_figi = ANY($1) AND event_date BETWEEN $2::date AND $3::date AND dimension = $4
		 ORDER BY event_date`,
		strings.Join(cols, ", "),
	)
```

Use the second form.

- [ ] **Step 4: Update the scan loop**

Replace the existing scan loop body with one that captures `reportPeriod` and stores both metadata values when requested. The full updated scan loop:

```go
	for rows.Next() {
		var (
			figi         string
			eventDate    time.Time
			dateKey      sql.NullTime
			reportPeriod sql.NullTime
		)

		vals := make([]any, 4+len(sqlCols))
		vals[0] = &figi
		vals[1] = &eventDate
		vals[2] = &dateKey
		vals[3] = &reportPeriod

		floatVals := make([]*float64, len(sqlCols))
		for idx := range sqlCols {
			vals[4+idx] = &floatVals[idx]
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

		if wantDateKey && dateKey.Valid {
			ensureCol(figi, FundamentalsDateKey)[sec] = float64(dateKey.Time.Unix())
		}

		if wantReportPeriod && reportPeriod.Valid {
			ensureCol(figi, FundamentalsReportPeriod)[sec] = float64(reportPeriod.Time.Unix())
		}
	}
```

Add the `database/sql` import to the file if not already imported.

- [ ] **Step 5: Run the data package tests**

```bash
ginkgo run -race ./data/
```

Expected: PASS (no behavioral change for callers that don't request the metadata metrics).

- [ ] **Step 6: Commit**

```bash
git add data/pvdata_provider.go
git commit -m "feat: scan report_period in fetchFundamentals and populate metadata metric cells"
```

---

### Task 3: Add `Dimension()` accessor on `PVDataProvider`

**Files:**
- Modify: `data/pvdata_provider.go` (near `SetDimension`)

`NewPVDataProvider` needs a real pgxpool connection or a config file, so a unit test would require integration setup. The recorder test in Task 4 exercises the contract through `dimensionedTestProvider`. A compile-time interface assertion is enough to verify the method exists with the right signature.

- [ ] **Step 1: Implement `Dimension()`**

In `data/pvdata_provider.go`, immediately after the existing `SetDimension` method (around line 140), add:

```go
// Dimension returns the current fundamental dimension filter.
func (p *PVDataProvider) Dimension() string {
	return p.dimension
}
```

- [ ] **Step 2: Add a compile-time interface assertion**

In `data/pvdata_provider.go`, near the top of the file (alongside other `var _ ... = (*PVDataProvider)(nil)` assertions if they exist, otherwise after the package-level imports block), add:

```go
var _ interface{ Dimension() string } = (*PVDataProvider)(nil)
```

- [ ] **Step 3: Build to verify**

```bash
go build ./...
```

Expected: clean build. If the assertion fails to compile, the method signature is wrong.

- [ ] **Step 4: Commit**

```bash
git add data/pvdata_provider.go
git commit -m "feat: expose PVDataProvider.Dimension"
```

---

### Task 4: Snapshot recorder writes real `date_key`, `report_period`, and `dimension`

**Goal:** when the recorded DataFrame includes `FundamentalsDateKey`/`FundamentalsReportPeriod`, persist the actual dates instead of NULL. Replace the hardcoded `"ARQ"` dimension with the wrapped provider's `Dimension()` when available.

**Files:**
- Modify: `data/snapshot_recorder.go` (around lines 422–510, `recordFundamentals`)
- Modify: `data/snapshot_recorder_test.go`

- [ ] **Step 1: Write the failing test**

Append a new `Describe` block to `data/snapshot_recorder_test.go` (place it as a sibling to existing `Describe`s, before the file-end `})`):

```go
	Describe("fundamentals recording with metadata", func() {
		It("persists date_key, report_period, and dimension from the wrapped provider", func() {
			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			assets := []asset.Asset{spy}

			filing := time.Date(2024, 5, 2, 16, 0, 0, 0, nyc)
			dateKey := time.Date(2024, 3, 31, 0, 0, 0, 0, nyc)
			reportPeriod := time.Date(2024, 3, 30, 0, 0, 0, 0, nyc)

			times := []time.Time{filing}
			metrics := []data.Metric{
				data.WorkingCapital,
				data.FundamentalsDateKey,
				data.FundamentalsReportPeriod,
			}

			values := [][]float64{
				{120_000_000.0},                  // SPY WorkingCapital
				{float64(dateKey.Unix())},        // SPY FundamentalsDateKey
				{float64(reportPeriod.Unix())},   // SPY FundamentalsReportPeriod
			}

			df, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
			Expect(err).NotTo(HaveOccurred())

			stub := &dimensionedTestProvider{
				TestProvider: data.NewTestProvider(metrics, df),
				dimension:    "MRQ",
			}

			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.Fetch(ctx, data.DataRequest{
				Assets:    assets,
				Metrics:   metrics,
				Start:     filing,
				End:       filing,
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var (
				dateKeyStr, reportPeriodStr, dimStr string
				wc                                   float64
			)
			err = db.QueryRow(
				`SELECT date_key, report_period, dimension, working_capital
				   FROM fundamentals
				  WHERE composite_figi = ?`,
				spy.CompositeFigi,
			).Scan(&dateKeyStr, &reportPeriodStr, &dimStr, &wc)
			Expect(err).NotTo(HaveOccurred())

			Expect(dateKeyStr).To(Equal("2024-03-31"))
			Expect(reportPeriodStr).To(Equal("2024-03-30"))
			Expect(dimStr).To(Equal("MRQ"))
			Expect(wc).To(BeNumerically("==", 120_000_000.0))
		})

		It("writes NULL date_key/report_period when the DataFrame omits them", func() {
			nyc, err := time.LoadLocation("America/New_York")
			Expect(err).NotTo(HaveOccurred())

			spy := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "SPY"}
			assets := []asset.Asset{spy}

			filing := time.Date(2024, 5, 2, 16, 0, 0, 0, nyc)
			times := []time.Time{filing}
			metrics := []data.Metric{data.WorkingCapital}
			values := [][]float64{{120_000_000.0}}

			df, err := data.NewDataFrame(times, assets, metrics, data.Daily, values)
			Expect(err).NotTo(HaveOccurred())

			stub := data.NewTestProvider(metrics, df) // no Dimension() method

			recorder, err = data.NewSnapshotRecorder(dbPath, data.SnapshotRecorderConfig{
				BatchProvider: stub,
				AssetProvider: &stubAssetProvider{assets: assets},
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = recorder.Fetch(ctx, data.DataRequest{
				Assets:    assets,
				Metrics:   metrics,
				Start:     filing,
				End:       filing,
				Frequency: data.Daily,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.Close()).To(Succeed())
			recorder = nil

			db, err := sql.Open("sqlite", dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			var (
				dateKeyNull, reportPeriodNull sql.NullString
				dimStr                        string
			)
			err = db.QueryRow(
				`SELECT date_key, report_period, dimension
				   FROM fundamentals
				  WHERE composite_figi = ?`,
				spy.CompositeFigi,
			).Scan(&dateKeyNull, &reportPeriodNull, &dimStr)
			Expect(err).NotTo(HaveOccurred())

			Expect(dateKeyNull.Valid).To(BeFalse())
			Expect(reportPeriodNull.Valid).To(BeFalse())
			Expect(dimStr).To(Equal("ARQ")) // fallback default
		})
	})
```

Add a small helper type at the bottom of the same file (outside any `Describe`):

```go
type dimensionedTestProvider struct {
	*data.TestProvider
	dimension string
}

func (p *dimensionedTestProvider) Dimension() string { return p.dimension }
```

- [ ] **Step 2: Run test to verify it fails**

```bash
ginkgo run -race --focus "fundamentals recording with metadata" ./data/
```

Expected: FAIL — current recorder writes NULL for `date_key`/`report_period` and hardcodes `"ARQ"`.

- [ ] **Step 3: Update `recordFundamentals` to read metadata + dimension**

In `data/snapshot_recorder.go`, modify `recordFundamentals` (lines ~422–510). Two changes:

a) Detect metadata-metric column indices in the DataFrame (using the existing `mIdx` map):

```go
	dateKeyDFIdx, hasDateKey := mIdx[FundamentalsDateKey]
	reportPeriodDFIdx, hasReportPeriod := mIdx[FundamentalsReportPeriod]
```

(Place these immediately after `mIdx` is built, around line 428.)

b) Read the dimension from the wrapped provider; fall back to `"ARQ"`:

Replace the existing `args[4] = "ARQ"` line with:

```go
	dimension := "ARQ"
	if dp, ok := r.batchProvider.(interface{ Dimension() string }); ok {
		if dim := dp.Dimension(); dim != "" {
			dimension = dim
		}
	}
```

(Place this once outside the per-row loop — immediately before the `for assetIdx, a := range df.assets {` loop.)

c) Inside the per-row loop, replace the two `args[2] = nil` / `args[3] = nil` lines with a lookup against the DataFrame:

```go
				args[2] = nil
				if hasDateKey {
					raw := df.columns[assetIdx*numDFMetrics+dateKeyDFIdx][timeIdx]
					if !math.IsNaN(raw) {
						args[2] = time.Unix(int64(raw), 0).UTC().Format("2006-01-02")
					}
				}

				args[3] = nil
				if hasReportPeriod {
					raw := df.columns[assetIdx*numDFMetrics+reportPeriodDFIdx][timeIdx]
					if !math.IsNaN(raw) {
						args[3] = time.Unix(int64(raw), 0).UTC().Format("2006-01-02")
					}
				}

				args[4] = dimension
```

Note: `df.columns` is a package-private field. Since `recordFundamentals` is in package `data`, the access is fine.

- [ ] **Step 4: Run tests to verify they pass**

```bash
ginkgo run -race --focus "fundamentals recording with metadata" ./data/
```

Expected: PASS for both `It` blocks.

- [ ] **Step 5: Run full data tests for regressions**

```bash
ginkgo run -race ./data/
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add data/snapshot_recorder.go data/snapshot_recorder_test.go
git commit -m "feat: snapshot recorder persists date_key, report_period, and dimension"
```

---

### Task 5: Add `FundamentalsByDateKeyProvider` capability and implement on `PVDataProvider`

**Goal:** define the optional provider interface, implement the SQL on `PVDataProvider`. Engine integration is Task 7.

**Files:**
- Modify: `data/data_provider.go`
- Modify: `data/pvdata_provider.go`
- Create: no new test file (covered by integration tests in Task 7)

- [ ] **Step 1: Define the interface**

In `data/data_provider.go`, append at end of file:

```go
// FundamentalsByDateKeyProvider is implemented by providers that can
// return fundamentals filtered to a specific reporting period (date_key).
// The engine type-asserts on this interface from FetchFundamentalsByDateKey.
type FundamentalsByDateKeyProvider interface {
	// FetchFundamentalsByDateKey returns one row per asset for the given
	// date_key + dimension. Only filings with event_date <= maxEventDate
	// are included (point-in-time correctness). For dimensions where a
	// single (figi, date_key) can have multiple filings (MR restatements),
	// the row with the maximum event_date wins.
	//
	// metrics must contain only fundamental metrics. Metadata metrics
	// (FundamentalsDateKey, FundamentalsReportPeriod) populate from the
	// row's date_key/report_period columns.
	FetchFundamentalsByDateKey(
		ctx context.Context,
		assets []asset.Asset,
		metrics []Metric,
		dateKey time.Time,
		dimension string,
		maxEventDate time.Time,
	) (*DataFrame, error)
}
```

Add the necessary imports if missing (`context`, `time`, `github.com/penny-vault/pvbt/asset`).

- [ ] **Step 2: Implement on `PVDataProvider`**

In `data/pvdata_provider.go`, append at end of file:

```go
// FetchFundamentalsByDateKey implements FundamentalsByDateKeyProvider.
func (p *PVDataProvider) FetchFundamentalsByDateKey(
	ctx context.Context,
	assets []asset.Asset,
	metrics []Metric,
	dateKey time.Time,
	dimension string,
	maxEventDate time.Time,
) (*DataFrame, error) {
	figis := make([]string, len(assets))
	for idx, a := range assets {
		figis[idx] = a.CompositeFigi
	}

	var (
		sqlCols     []string
		metricOrder []Metric
	)

	wantDateKey := false
	wantReportPeriod := false

	for _, m := range metrics {
		switch m {
		case FundamentalsDateKey:
			wantDateKey = true
			continue
		case FundamentalsReportPeriod:
			wantReportPeriod = true
			continue
		}

		col, ok := metricColumn[m]
		if !ok {
			return nil, fmt.Errorf("pvdata: no SQL column for fundamental metric %q", m)
		}

		sqlCols = append(sqlCols, col)
		metricOrder = append(metricOrder, m)
	}

	cols := []string{"composite_figi", "event_date", "date_key", "report_period"}
	cols = append(cols, sqlCols...)

	// DISTINCT ON (composite_figi) keeps the most recent event_date per
	// asset, which matters for MR dimensions where a single (figi,
	// date_key) tuple may appear multiple times due to restatements.
	query := fmt.Sprintf(
		`SELECT DISTINCT ON (composite_figi) %s
		 FROM fundamentals
		 WHERE composite_figi = ANY($1)
		   AND date_key = $2::date
		   AND dimension = $3
		   AND event_date <= $4::date
		 ORDER BY composite_figi, event_date DESC`,
		strings.Join(cols, ", "),
	)

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pvdata: acquire conn for FetchFundamentalsByDateKey: %w", err)
	}
	defer conn.Release()

	rows, err := conn.Query(ctx, query, figis, dateKey, dimension, maxEventDate)
	if err != nil {
		return nil, fmt.Errorf("pvdata: query fundamentals by date_key: %w", err)
	}
	defer rows.Close()

	// Build a per-figi value map: figi -> metric -> float64.
	perFigi := make(map[string]map[Metric]float64, len(assets))

	for rows.Next() {
		var (
			figi         string
			eventDate    time.Time
			rowDateKey   time.Time
			reportPeriod sql.NullTime
		)

		vals := make([]any, 4+len(sqlCols))
		vals[0] = &figi
		vals[1] = &eventDate
		vals[2] = &rowDateKey
		vals[3] = &reportPeriod

		floatVals := make([]*float64, len(sqlCols))
		for idx := range sqlCols {
			vals[4+idx] = &floatVals[idx]
		}

		if err := rows.Scan(vals...); err != nil {
			return nil, fmt.Errorf("pvdata: scan fundamentals by date_key row: %w", err)
		}

		bucket, ok := perFigi[figi]
		if !ok {
			bucket = make(map[Metric]float64, len(metrics))
			perFigi[figi] = bucket
		}

		for idx, m := range metricOrder {
			if floatVals[idx] != nil {
				bucket[m] = *floatVals[idx]
			}
		}

		if wantDateKey {
			bucket[FundamentalsDateKey] = float64(rowDateKey.Unix())
		}

		if wantReportPeriod && reportPeriod.Valid {
			bucket[FundamentalsReportPeriod] = float64(reportPeriod.Time.Unix())
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pvdata: iterate fundamentals by date_key rows: %w", err)
	}

	// Assemble a single-row DataFrame at dateKey.
	times := []time.Time{dateKey}
	columns := make([][]float64, len(assets)*len(metrics))

	for aIdx, a := range assets {
		bucket := perFigi[a.CompositeFigi]
		for mIdx, m := range metrics {
			val := math.NaN()
			if bucket != nil {
				if v, ok := bucket[m]; ok {
					val = v
				}
			}

			columns[aIdx*len(metrics)+mIdx] = []float64{val}
		}
	}

	return NewDataFrame(times, assets, metrics, Daily, columns)
}
```

Required imports to confirm in `data/pvdata_provider.go`: `database/sql`, `math`. Add if missing.

- [ ] **Step 3: Build to verify compile**

```bash
go build ./...
```

Expected: builds cleanly.

- [ ] **Step 4: Commit**

```bash
git add data/data_provider.go data/pvdata_provider.go
git commit -m "feat: add FundamentalsByDateKeyProvider interface and PVDataProvider impl"
```

---

### Task 6: Implement `SnapshotProvider.FetchFundamentalsByDateKey` for snapshot replay

**Files:**
- Modify: `data/snapshot_provider.go`

The snapshot provider must implement the same interface so that snapshot-replayed backtests support `FetchFundamentalsByDateKey`.

- [ ] **Step 1: Append the implementation**

At the end of `data/snapshot_provider.go`, add:

```go
// FetchFundamentalsByDateKey implements FundamentalsByDateKeyProvider for
// snapshot replay. The query mirrors the PVDataProvider implementation but
// uses SQLite syntax (no DISTINCT ON; row_number window function instead).
func (p *SnapshotProvider) FetchFundamentalsByDateKey(
	ctx context.Context,
	assets []asset.Asset,
	metrics []Metric,
	dateKey time.Time,
	dimension string,
	maxEventDate time.Time,
) (*DataFrame, error) {
	figis := make([]string, len(assets))
	for idx, a := range assets {
		figis[idx] = a.CompositeFigi
	}

	var (
		sqlCols     []string
		metricOrder []Metric
	)

	wantDateKey := false
	wantReportPeriod := false

	for _, m := range metrics {
		switch m {
		case FundamentalsDateKey:
			wantDateKey = true
			continue
		case FundamentalsReportPeriod:
			wantReportPeriod = true
			continue
		}

		col, ok := metricColumn[m]
		if !ok {
			return nil, fmt.Errorf("snapshot: no SQL column for fundamental metric %q", m)
		}

		sqlCols = append(sqlCols, col)
		metricOrder = append(metricOrder, m)
	}

	cols := []string{"composite_figi", "event_date", "date_key", "report_period"}
	cols = append(cols, sqlCols...)

	placeholders := make([]string, len(figis))
	for idx := range figis {
		placeholders[idx] = "?"
	}

	dateKeyStr := dateKey.UTC().Format("2006-01-02")
	maxEventStr := maxEventDate.UTC().Format("2006-01-02")

	// SQLite ranks by event_date DESC and picks rank 1 per figi.
	query := fmt.Sprintf(
		`SELECT %s FROM (
		    SELECT %s,
		           row_number() OVER (PARTITION BY composite_figi ORDER BY event_date DESC) AS rn
		      FROM fundamentals
		     WHERE composite_figi IN (%s)
		       AND date_key = ?
		       AND dimension = ?
		       AND event_date <= ?
		 ) WHERE rn = 1`,
		strings.Join(cols, ", "),
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	args := make([]any, 0, len(figis)+3)
	for _, f := range figis {
		args = append(args, f)
	}

	args = append(args, dateKeyStr, dimension, maxEventStr)

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("snapshot: query fundamentals by date_key: %w", err)
	}
	defer rows.Close()

	perFigi := make(map[string]map[Metric]float64, len(assets))

	for rows.Next() {
		var (
			figi             string
			eventDateStr     string
			rowDateKeyStr    sql.NullString
			reportPeriodStr  sql.NullString
		)

		vals := make([]any, 4+len(sqlCols))
		vals[0] = &figi
		vals[1] = &eventDateStr
		vals[2] = &rowDateKeyStr
		vals[3] = &reportPeriodStr

		floatVals := make([]sql.NullFloat64, len(sqlCols))
		for idx := range sqlCols {
			vals[4+idx] = &floatVals[idx]
		}

		if err := rows.Scan(vals...); err != nil {
			return nil, fmt.Errorf("snapshot: scan fundamentals by date_key row: %w", err)
		}

		bucket, ok := perFigi[figi]
		if !ok {
			bucket = make(map[Metric]float64, len(metrics))
			perFigi[figi] = bucket
		}

		for idx, m := range metricOrder {
			if floatVals[idx].Valid {
				bucket[m] = floatVals[idx].Float64
			}
		}

		if wantDateKey && rowDateKeyStr.Valid {
			parsed, parseErr := parseSnapshotDate(rowDateKeyStr.String)
			if parseErr != nil {
				return nil, fmt.Errorf("snapshot: parse date_key %q: %w", rowDateKeyStr.String, parseErr)
			}

			bucket[FundamentalsDateKey] = float64(parsed.Unix())
		}

		if wantReportPeriod && reportPeriodStr.Valid {
			parsed, parseErr := parseSnapshotDate(reportPeriodStr.String)
			if parseErr != nil {
				return nil, fmt.Errorf("snapshot: parse report_period %q: %w", reportPeriodStr.String, parseErr)
			}

			bucket[FundamentalsReportPeriod] = float64(parsed.Unix())
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("snapshot: iterate fundamentals by date_key rows: %w", err)
	}

	times := []time.Time{dateKey}
	columns := make([][]float64, len(assets)*len(metrics))

	for aIdx, a := range assets {
		bucket := perFigi[a.CompositeFigi]
		for mIdx, m := range metrics {
			val := math.NaN()
			if bucket != nil {
				if v, ok := bucket[m]; ok {
					val = v
				}
			}

			columns[aIdx*len(metrics)+mIdx] = []float64{val}
		}
	}

	return NewDataFrame(times, assets, metrics, Daily, columns)
}
```

Confirm `database/sql`, `math`, `context`, `time`, `fmt`, `strings`, and `github.com/penny-vault/pvbt/asset` are all imported. The file should already have all of them from existing code.

- [ ] **Step 2: Build to verify compile**

```bash
go build ./...
```

Expected: builds cleanly.

- [ ] **Step 3: Commit**

```bash
git add data/snapshot_provider.go
git commit -m "feat: add SnapshotProvider.FetchFundamentalsByDateKey for replay"
```

---

### Task 7: Implement `Engine.FetchFundamentalsByDateKey`

**Files:**
- Modify: `engine/engine.go`
- Create: `engine/fetch_by_date_key_test.go`

- [ ] **Step 1: Write the failing test**

Create `engine/fetch_by_date_key_test.go`:

```go
package engine_test

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

type fetchByDateKeyStrategy struct {
	assets   []asset.Asset
	metrics  []data.Metric
	dateKey  time.Time
	fetched  *data.DataFrame
	fetchErr error
}

func (s *fetchByDateKeyStrategy) Name() string { return "fetchByDateKeyStrategy" }

func (s *fetchByDateKeyStrategy) Setup(*engine.Engine) {}

func (s *fetchByDateKeyStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

func (s *fetchByDateKeyStrategy) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	s.fetched, s.fetchErr = eng.FetchFundamentalsByDateKey(ctx, s.assets, s.metrics, s.dateKey)
	return nil
}

// fakeByDateKeyProvider is a small in-memory FundamentalsByDateKeyProvider.
type fakeByDateKeyProvider struct {
	*data.TestProvider
	rows map[string]map[time.Time]map[data.Metric]float64 // figi -> dateKey -> metric -> value
}

func (p *fakeByDateKeyProvider) FetchFundamentalsByDateKey(
	_ context.Context,
	assets []asset.Asset,
	metrics []data.Metric,
	dateKey time.Time,
	_ string,
	_ time.Time,
) (*data.DataFrame, error) {
	times := []time.Time{dateKey}
	columns := make([][]float64, len(assets)*len(metrics))

	for aIdx, a := range assets {
		assetRows := p.rows[a.CompositeFigi]
		for mIdx, m := range metrics {
			val := math.NaN()
			if assetRows != nil {
				if dayMetrics, ok := assetRows[dateKey]; ok {
					if v, ok := dayMetrics[m]; ok {
						val = v
					}
				}
			}

			columns[aIdx*len(metrics)+mIdx] = []float64{val}
		}
	}

	return data.NewDataFrame(times, assets, metrics, data.Daily, columns)
}

var _ = Describe("Engine.FetchFundamentalsByDateKey", func() {
	var assetProv *mockAssetProvider

	BeforeEach(func() {
		assetProv = &mockAssetProvider{}
	})

	It("returns the requested fundamentals at the date_key", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		testAssets := []asset.Asset{spy, msft}
		assetProv.assets = testAssets

		q1 := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 180, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		fundProvider := &fakeByDateKeyProvider{
			TestProvider: data.NewTestProvider(
				[]data.Metric{data.WorkingCapital, data.FundamentalsDateKey},
				mustEmptyFundDF(testAssets),
			),
			rows: map[string]map[time.Time]map[data.Metric]float64{
				spy.CompositeFigi: {
					q1: {
						data.WorkingCapital:      120_000_000,
						data.FundamentalsDateKey: float64(q1.Unix()),
					},
				},
				msft.CompositeFigi: {
					q1: {
						data.WorkingCapital:      80_000_000,
						data.FundamentalsDateKey: float64(q1.Unix()),
					},
				},
			},
		}

		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital, data.FundamentalsDateKey},
			dateKey: q1,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		simDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simDate, simDate)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).NotTo(HaveOccurred())

		spyWC := strategy.fetched.Column(spy, data.WorkingCapital)
		Expect(spyWC).To(HaveLen(1))
		Expect(spyWC[0]).To(BeNumerically("==", 120_000_000))

		msftWC := strategy.fetched.Column(msft, data.WorkingCapital)
		Expect(msftWC).To(HaveLen(1))
		Expect(msftWC[0]).To(BeNumerically("==", 80_000_000))

		spyDK := strategy.fetched.Column(spy, data.FundamentalsDateKey)
		Expect(time.Unix(int64(spyDK[0]), 0).UTC()).To(Equal(q1))
	})

	It("returns NaN for assets with no filing at the date_key", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		testAssets := []asset.Asset{spy, msft}
		assetProv.assets = testAssets

		q1 := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 180, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		fundProvider := &fakeByDateKeyProvider{
			TestProvider: data.NewTestProvider(
				[]data.Metric{data.WorkingCapital},
				mustEmptyFundDF(testAssets),
			),
			rows: map[string]map[time.Time]map[data.Metric]float64{
				spy.CompositeFigi: {
					q1: {data.WorkingCapital: 120_000_000},
				},
				// MSFT has no Q1 row.
			},
		}

		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital},
			dateKey: q1,
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		simDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simDate, simDate)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).NotTo(HaveOccurred())

		msftWC := strategy.fetched.Column(msft, data.WorkingCapital)
		Expect(math.IsNaN(msftWC[0])).To(BeTrue())
	})

	It("errors when a non-fundamental metric is requested", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}
		assetProv.assets = testAssets

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 30, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

		fundProvider := &fakeByDateKeyProvider{
			TestProvider: data.NewTestProvider(
				[]data.Metric{data.WorkingCapital},
				mustEmptyFundDF(testAssets),
			),
			rows: map[string]map[time.Time]map[data.Metric]float64{},
		}

		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital, data.MetricClose},
			dateKey: time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC),
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		simDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simDate, simDate)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).To(HaveOccurred())
		Expect(strategy.fetchErr.Error()).To(ContainSubstring("not a fundamental metric"))
	})

	It("errors when no provider supports FundamentalsByDateKey", func() {
		spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets := []asset.Asset{spy}
		assetProv.assets = testAssets

		closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		closeDF := makeDailyDF(closeStart, 30, testAssets, []data.Metric{data.MetricClose})
		closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)
		fundProvider := data.NewTestProvider(
			[]data.Metric{data.WorkingCapital},
			mustEmptyFundDF(testAssets),
		)

		strategy := &fetchByDateKeyStrategy{
			assets:  testAssets,
			metrics: []data.Metric{data.WorkingCapital},
			dateKey: time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC),
		}

		eng := engine.New(strategy,
			engine.WithDataProvider(closeProvider, fundProvider),
			engine.WithAssetProvider(assetProv),
			engine.WithInitialDeposit(100_000.0),
		)

		simDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		_, err := eng.Backtest(context.Background(), simDate, simDate)
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.fetchErr).To(HaveOccurred())
		Expect(strategy.fetchErr.Error()).To(ContainSubstring("no provider supports"))
	})
})

// mustEmptyFundDF returns an empty fundamentals DataFrame for use as a
// TestProvider seed. Real values come from fakeByDateKeyProvider.rows.
func mustEmptyFundDF(assets []asset.Asset) *data.DataFrame {
	df, err := data.NewDataFrame(
		nil,
		assets,
		[]data.Metric{data.WorkingCapital, data.FundamentalsDateKey},
		data.Daily,
		nil,
	)
	if err != nil {
		panic(err)
	}

	return df
}
```

Helpers used: `makeDailyDF` (engine test helpers, used by `dimension_test.go`), `mockAssetProvider` (`engine/backtest_test.go:37`). Both are in the same test package; no import needed.

- [ ] **Step 2: Run test to verify it fails**

```bash
ginkgo run -race --focus "Engine.FetchFundamentalsByDateKey" ./engine/
```

Expected: build error — `eng.FetchFundamentalsByDateKey` undefined.

- [ ] **Step 3: Implement `FetchFundamentalsByDateKey`**

In `engine/engine.go`, append after the `FetchAt` method (around line 351):

```go
// FetchFundamentalsByDateKey returns fundamental data for a specific reporting period.
// dateKey identifies the normalized quarter boundary (e.g. 2024-03-31 for
// Q1 2024). All metrics must be fundamentals; non-fundamental metrics
// produce an error. Subject to point-in-time correctness: only filings
// with event_date <= eng.CurrentDate() are included. Assets that have not
// filed for dateKey as of CurrentDate get NaN values.
func (e *Engine) FetchFundamentalsByDateKey(
	ctx context.Context,
	assets []asset.Asset,
	metrics []data.Metric,
	dateKey time.Time,
) (*data.DataFrame, error) {
	for _, m := range metrics {
		if !data.IsFundamental(m) {
			return nil, fmt.Errorf("FetchFundamentalsByDateKey: metric %q is not a fundamental metric", m)
		}
	}

	dimension := e.fundamentalDimension
	if dimension == "" {
		dimension = "ARQ"
	}

	for _, provider := range e.providers {
		if dp, ok := provider.(data.FundamentalsByDateKeyProvider); ok {
			df, err := dp.FetchFundamentalsByDateKey(ctx, assets, metrics, dateKey, dimension, e.currentDate)
			if err != nil {
				return nil, fmt.Errorf("FetchFundamentalsByDateKey: %w", err)
			}

			df.SetSource(e)

			return df, nil
		}
	}

	return nil, fmt.Errorf("FetchFundamentalsByDateKey: no provider supports FundamentalsByDateKeyProvider")
}
```

The engine struct's provider slice is `e.providers []data.DataProvider` (`engine/engine.go:39`).

- [ ] **Step 4: Run tests to verify they pass**

```bash
ginkgo run -race --focus "Engine.FetchFundamentalsByDateKey" ./engine/
```

Expected: PASS for all four `It` blocks.

- [ ] **Step 5: Run full engine test suite**

```bash
ginkgo run -race ./engine/
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add engine/engine.go engine/fetch_by_date_key_test.go
git commit -m "feat: add Engine.FetchFundamentalsByDateKey for reporting-period fundamentals queries"
```

---

### Task 8: Verify forward-fill carries metadata metrics through `Fetch`/`FetchAt`

**Goal:** confirm that requesting `FundamentalsDateKey` alongside Revenue in a regular `Fetch` call returns the date_key forward-filled across non-filing days. No production code change expected — the existing forward-fill in `engine/engine.go` runs on any metric for which `data.IsFundamental` is true.

**Files:**
- Modify: `engine/fetch_test.go`

- [ ] **Step 1: Write the test**

Append to `engine/fetch_test.go` inside the existing `Context("with sparse fundamental data", ...)` block (after the last `It`, before the closing `})` of the Context):

```go
		It("forward-fills FundamentalsDateKey alongside the value", func() {
			spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
			fundamentalAssets := []asset.Asset{spy}
			assetProvider = &mockAssetProvider{assets: fundamentalAssets}

			closeStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			closeDF := makeDailyDF(closeStart, 90, fundamentalAssets, []data.Metric{data.MetricClose})
			closeProvider := data.NewTestProvider([]data.Metric{data.MetricClose}, closeDF)

			filingDate := time.Date(2024, 2, 1, 16, 0, 0, 0, time.UTC)
			dateKey := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC) // Q4 2023 filing on Feb 1

			fundDF, err := data.NewDataFrame(
				[]time.Time{filingDate},
				fundamentalAssets,
				[]data.Metric{data.Revenue, data.FundamentalsDateKey},
				data.Daily,
				[][]float64{
					{500_000_000},
					{float64(dateKey.Unix())},
				},
			)
			Expect(err).NotTo(HaveOccurred())
			fundProvider := data.NewTestProvider(
				[]data.Metric{data.Revenue, data.FundamentalsDateKey},
				fundDF,
			)

			strategy := &fetchStrategy{
				lookback: portfolio.Days(30),
				metrics:  []data.Metric{data.MetricClose, data.Revenue, data.FundamentalsDateKey},
				assets:   fundamentalAssets,
			}

			eng := engine.New(strategy,
				engine.WithDataProvider(closeProvider, fundProvider),
				engine.WithAssetProvider(assetProvider),
				engine.WithInitialDeposit(100_000.0),
			)

			simDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
			_, btErr := eng.Backtest(context.Background(), simDate, simDate)
			Expect(btErr).NotTo(HaveOccurred())
			Expect(strategy.fetchErr).NotTo(HaveOccurred())

			dkCol := strategy.fetched.Column(spy, data.FundamentalsDateKey)
			Expect(len(dkCol)).To(BeNumerically(">", 1))

			expected := float64(dateKey.Unix())
			for _, v := range dkCol {
				Expect(v).To(BeNumerically("==", expected))
			}
		})
```

- [ ] **Step 2: Run the test**

```bash
ginkgo run -race --focus "forward-fills FundamentalsDateKey" ./engine/
```

Expected: PASS (no production change — Task 1 already classified the metric as fundamental, so forward-fill applies automatically).

- [ ] **Step 3: Commit**

```bash
git add engine/fetch_test.go
git commit -m "test: cover forward-fill of FundamentalsDateKey through Fetch"
```

---

### Task 9: Documentation

**Files:**
- Modify: `docs/data.md`
- Modify: `docs/strategy-guide.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update `docs/data.md`**

Find the existing "Fundamental data semantics" section (added by the prior plan). Append the following subsection at the end of that section:

```markdown
#### Reading the date metadata

`event_date` is exposed as the DataFrame's time axis for fundamental fetches.
`date_key` and `report_period` are exposed as two metrics:

- `data.FundamentalsDateKey` -- normalized calendar quarter boundary.
- `data.FundamentalsReportPeriod` -- actual fiscal period end as reported.

Both are encoded as `float64(t.Unix())`. Convert with the standard library:

```go
dk := time.Unix(int64(df.Value(spy, data.FundamentalsDateKey, t)), 0)
```

NaN means no filing has been observed for that asset as of `t`. The engine
forward-fills these metrics the same way as Revenue, WorkingCapital, etc., so
the metadata travels with whatever value the strategy reads.
```

- [ ] **Step 2: Update `docs/strategy-guide.md`**

Append a new section after the existing "Fundamental dimension" section:

```markdown
### Querying a specific reporting period

When a strategy needs values for a particular fiscal quarter (e.g. Q1
working capital across all candidates), use `Engine.FetchFundamentalsByDateKey`:

```go
func (s *NCAVE) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    q1 := mostRecentQ1(eng.CurrentDate())

    df, err := eng.FetchFundamentalsByDateKey(ctx, s.universe, []data.Metric{
        data.WorkingCapital,
        data.TotalLiabilities,
        data.FundamentalsDateKey,
    }, q1)
    if err != nil {
        return err
    }

    for _, candidate := range s.universe {
        wc := df.Value(candidate, data.WorkingCapital, q1)
        if math.IsNaN(wc) {
            continue // candidate has not filed Q1 yet as of CurrentDate
        }

        // ... use wc, df.Value(candidate, data.TotalLiabilities, q1), etc.
    }

    return nil
}
```

`FetchFundamentalsByDateKey` returns a single time-axis row at `dateKey`, one value per
asset per metric. Only filings with `event_date <= eng.CurrentDate()` are
included; assets that have not filed for `dateKey` get NaN. All metrics in
the call must be fundamentals; non-fundamental metrics return an error.

The dimension used is whatever the strategy set with `SetFundamentalDimension`
in `Setup` (defaults to `ARQ`).
```

- [ ] **Step 3: Update `CHANGELOG.md`**

Add under `[Unreleased]` -> `### Added`:

```markdown
- Strategies can request a specific fundamentals reporting period with `Engine.FetchFundamentalsByDateKey`. The call returns one row per asset for the given `date_key` (e.g. Q1 boundary), filtered to the engine's configured dimension and to filings public as of `CurrentDate()`.
- Two new fundamental metrics expose per-row date metadata: `data.FundamentalsDateKey` (normalized quarter boundary) and `data.FundamentalsReportPeriod` (actual fiscal period end). Values are encoded as Unix seconds in `float64`; convert with `time.Unix(int64(v), 0)`.
- Snapshots now persist `date_key`, `report_period`, and the configured dimension instead of writing NULL/`"ARQ"` placeholders.
```

- [ ] **Step 4: Commit**

```bash
git add docs/data.md docs/strategy-guide.md CHANGELOG.md
git commit -m "docs: document FetchFundamentalsByDateKey and fundamentals metadata metrics"
```

---

### Task 10: Final verification

**Files:** none (verification only).

- [ ] **Step 1: Full test suite**

```bash
ginkgo run -race ./...
```

Expected: PASS.

- [ ] **Step 2: Lint**

```bash
golangci-lint run ./...
```

Expected: 0 issues. Fix anything reported (no `//nolint` directives — fix the underlying issue per project convention).

- [ ] **Step 3: Build**

```bash
go build ./...
```

Expected: clean build.

- [ ] **Step 4: If any fixes were needed, commit them**

```bash
git add -A
git commit -m "fix: address lint or test issues from fundamentals metadata changes"
```
