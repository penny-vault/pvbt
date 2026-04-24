# positions_daily Snapshot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist a `positions_daily` table in SQLite snapshots so pv-api's `holdings-impact` endpoint can compute per-ticker contribution without replaying prices.

**Architecture:** Track per-asset daily market value and quantity in a side-car on `Account` during simulation (appended each `UpdatePrices` step). At `ToSQLite` time, flush the side-car into `perfData` via `Insert` (under new `PositionMarketValue` / `PositionQuantity` metrics) so one consistent reader code path exists, then write the new `positions_daily` table from `perfData`. $CASH is tracked as an asset like any other. Bump snapshot `schemaVersion` from `"4"` to `"5"`.

**Tech Stack:** Go 1.23, SQLite via `modernc.org/sqlite`, Ginkgo/Gomega, zerolog.

---

## File Map

**New:**
- none

**Modify:**
- `data/metric.go` — add `PositionMarketValue` and `PositionQuantity` constants.
- `portfolio/account.go` — add side-car position tracking maps (`positionMV`, `positionQty` keyed by `asset.Asset` of `[]float64` history aligned with `perfData.Times()`); extend `UpdatePrices` to append each step (including `$CASH`); add helper `flushPositionsToPerfData` invoked from `ToSQLite`.
- `portfolio/sqlite.go` — bump `schemaVersion` to `"5"`; add `positions_daily` DDL to `createSchema`; add `writePositionsDaily(tx)`; call it after `writePerfData` in `ToSQLite`; update doc comment.

**Test:**
- `portfolio/sqlite_test.go` — extend round-trip test; add new Describe for `positions_daily` covering: 30-day two-asset account, mid-period buy, mid-period close, splits, NaN price day, $CASH rows including zero balance, schema version 5, SUM invariant.

---

## Task 1: Add `PositionMarketValue` / `PositionQuantity` metric constants

**Files:**
- Modify: `data/metric.go`

- [ ] **Step 1: Add the constants in the "Portfolio performance tracking metrics" const block**

Edit `data/metric.go`: inside the block starting with `// Portfolio performance tracking metrics.` (currently ending after `PortfolioBenchReturns`), append the two new constants at the end of that block:

```go
// PositionMarketValue is the per-asset market value recorded daily for positions-impact tracking.
PositionMarketValue Metric = "PositionMarketValue"
// PositionQuantity is the per-asset quantity held recorded daily for positions-impact tracking.
PositionQuantity Metric = "PositionQuantity"
```

- [ ] **Step 2: Verify the package still compiles**

Run: `go build ./data/...`
Expected: exit 0, no output.

- [ ] **Step 3: Commit**

```bash
git add data/metric.go
git commit -m "feat(data): add PositionMarketValue and PositionQuantity metrics"
```

---

## Task 2: Track per-asset market value and quantity each step in Account

**Files:**
- Modify: `portfolio/account.go`
- Test: `portfolio/account_test.go` (new Describe block appended)

Context — current `UpdatePrices` (lines 1678-1727) iterates `a.holdings` to compute `total`, then appends one row to `perfData`. We will extend it to also track per-asset `(market_value, quantity)` plus `$CASH` in side-car maps. We explicitly do NOT modify `perfData` during simulation — only at save time — to keep the existing `AppendRow`-based path unchanged.

- [ ] **Step 1: Write failing unit test for side-car tracking (exposed via a small accessor)**

First, understand the access surface: we need a test hook to read back the per-asset history. We will add a package-private accessor usable from tests via an export file. Check for an existing export_test.go:

Run: `ls portfolio/export_test.go`
Expected: file exists.

Read `portfolio/export_test.go` to match style, then append (exact block — do not touch existing content):

```go
// PositionSeries returns the per-asset (mv, qty) history tracked by
// UpdatePrices. Test-only accessor.
func (a *Account) PositionSeries() (map[asset.Asset][]float64, map[asset.Asset][]float64) {
    return a.positionMV, a.positionQty
}
```

If `export_test.go` does not import `asset`, add the import.

Now in `portfolio/account_test.go`, append a new Describe (place it after the existing `Describe("UpdatePrices", ...)` block, matching the file's style):

```go
var _ = Describe("UpdatePrices per-asset tracking", func() {
    It("records market value and quantity for each held asset and $CASH on each step", func() {
        spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
        bnd := asset.Asset{Ticker: "BND", CompositeFigi: "BBG000BBVR08"}
        cashAsset := asset.Asset{Ticker: "$CASH", CompositeFigi: "$CASH"}

        acct := portfolio.New(portfolio.WithCash(10000, time.Time{}))

        t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
        acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: asset.BuyTransaction, Qty: 10, Price: 400, Amount: -4000})
        acct.Record(portfolio.Transaction{Date: t0, Asset: bnd, Type: asset.BuyTransaction, Qty: 20, Price: 80, Amount: -1600})
        df0 := buildDF(t0, []asset.Asset{spy, bnd}, []float64{400, 80}, []float64{400, 80})
        acct.UpdatePrices(df0)

        t1 := t0.AddDate(0, 0, 1)
        df1 := buildDF(t1, []asset.Asset{spy, bnd}, []float64{410, 81}, []float64{410, 81})
        acct.UpdatePrices(df1)

        mv, qty := acct.PositionSeries()
        Expect(mv[spy]).To(Equal([]float64{4000, 4100}))
        Expect(mv[bnd]).To(Equal([]float64{1600, 1620}))
        Expect(mv[cashAsset]).To(Equal([]float64{4400, 4400}))
        Expect(qty[spy]).To(Equal([]float64{10, 10}))
        Expect(qty[bnd]).To(Equal([]float64{20, 20}))
        Expect(qty[cashAsset]).To(Equal([]float64{4400, 4400}))
    })
})
```

- [ ] **Step 2: Run the test to verify it fails to compile (no such method)**

Run: `ginkgo run ./portfolio/... 2>&1 | head -40`
Expected: compile error referencing `PositionSeries` or `positionMV`/`positionQty`.

- [ ] **Step 3: Add side-car fields and cashAsset sentinel to Account**

Edit `portfolio/account.go`. In the `Account` struct (currently lines 51-85), append two new fields after `perfData`:

```go
// positionMV tracks per-asset market-value history aligned with perfData.Times().
// Populated by UpdatePrices; appended each step for every currently-held asset
// and for the $CASH sentinel (emitted every step, including zero-balance days).
// Flushed into perfData at ToSQLite time under the PositionMarketValue metric.
positionMV map[asset.Asset][]float64
// positionQty tracks per-asset quantity history aligned with perfData.Times().
// Same semantics as positionMV, under the PositionQuantity metric.
positionQty map[asset.Asset][]float64
```

Near the top of `account.go` (after `portfolioAsset` declaration at line 38-41), add:

```go
// cashSentinel is the pseudo-asset used to emit $CASH rows in positions_daily.
// figi is empty by design — pv-api distinguishes cash by ticker.
var cashSentinel = asset.Asset{Ticker: "$CASH", CompositeFigi: ""}
```

In `New` (around line 88-102), initialize the two maps at the same place `holdings` etc. are initialized:

```go
positionMV:       make(map[asset.Asset][]float64),
positionQty:      make(map[asset.Asset][]float64),
```

- [ ] **Step 4: Extend `UpdatePrices` to append per-asset and $CASH history each step**

In `portfolio/account.go`, modify `UpdatePrices` (current body lines 1678-1727). Replace the entire function body with:

```go
func (a *Account) UpdatePrices(priceData *data.DataFrame) {
    if priceData.Len() == 0 {
        return
    }

    a.prices = priceData

    // stepMV holds the per-asset market value contributed this step (excluding cash).
    // We compute it first so we can both stamp perfData and fill the side-car.
    stepMV := make(map[asset.Asset]float64, len(a.holdings))

    total := a.cash
    for ast, qty := range a.holdings {
        v := priceData.Value(ast, data.MetricClose)
        if math.IsNaN(v) {
            // Fall back to the last-known mv if we have any history for this asset.
            if prior, ok := a.positionMV[ast]; ok && len(prior) > 0 {
                last := prior[len(prior)-1]
                if !math.IsNaN(last) {
                    // Recover MV from prior mv (post-split quantities may differ,
                    // so prefer prior price = prior mv / prior qty if qty > 0).
                    priorQty := a.positionQty[ast]
                    if len(priorQty) > 0 && priorQty[len(priorQty)-1] > 0 {
                        priorPrice := last / priorQty[len(priorQty)-1]
                        stepMV[ast] = qty * priorPrice
                        total += stepMV[ast]
                        continue
                    }
                }
            }
            // No prior history: skip this asset for this step (mv=NaN).
            log.Debug().Str("ticker", ast.Ticker).Msg("UpdatePrices: no close price and no prior mv; skipping positions_daily row this step")
            stepMV[ast] = math.NaN()
            continue
        }
        stepMV[ast] = qty * v
        total += stepMV[ast]
    }

    var benchVal, rfVal float64

    if a.benchmark != (asset.Asset{}) {
        v := priceData.Value(a.benchmark, data.AdjClose)
        if math.IsNaN(v) || v == 0 {
            v = priceData.Value(a.benchmark, data.MetricClose)
        }

        benchVal = v
    }

    rfVal = a.riskFreeValue

    if a.perfData == nil {
        timestamps := []time.Time{priceData.End()}
        assets := []asset.Asset{portfolioAsset}
        metrics := []data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree}

        row, err := data.NewDataFrame(timestamps, assets, metrics, data.Daily, [][]float64{{total}, {benchVal}, {rfVal}})
        if err != nil {
            log.Error().Err(err).Msg("UpdatePrices: failed to create perfData")
            return
        }

        a.perfData = row
    } else {
        if err := a.perfData.AppendRow(priceData.End(), []float64{total, benchVal, rfVal}); err != nil {
            log.Error().Err(err).Msg("UpdatePrices: failed to append to perfData")
            return
        }
    }

    // Determine the current time-axis length we must match.
    histLen := a.perfData.Len()

    // Append per-asset side-car rows.
    for ast, mv := range stepMV {
        qty := a.holdings[ast]
        a.appendPositionRow(ast, mv, qty, histLen)
    }
    // Any asset tracked previously but not currently held still needs a slot filled
    // this step (NaN means "no row emitted on this date"). We emit (0, 0) on the
    // first step after close so pv-api sees the exit, and NaN thereafter.
    for ast := range a.positionMV {
        if ast == cashSentinel {
            continue
        }
        if _, held := a.holdings[ast]; held {
            continue
        }
        if _, stampedThisStep := stepMV[ast]; stampedThisStep {
            continue
        }
        prior := a.positionQty[ast]
        if len(prior) > 0 && prior[len(prior)-1] > 0 {
            // Just closed: emit zero-row this step.
            a.appendPositionRow(ast, 0, 0, histLen)
        } else {
            // Already closed in a prior step: NaN this step (writer will skip).
            a.appendPositionRow(ast, math.NaN(), math.NaN(), histLen)
        }
    }

    // $CASH row every step, including zero balance.
    a.appendPositionRow(cashSentinel, a.cash, a.cash, histLen)

    // Invalidate lazily-computed DataFrames so they are recomputed on next access.
    a.dfCache = nil
}

// appendPositionRow appends (mv, qty) to the per-asset side-car slices,
// back-filling NaN for any missing steps so the slices stay aligned with
// perfData.Times() (length targetLen after the append).
func (a *Account) appendPositionRow(ast asset.Asset, mv, qty float64, targetLen int) {
    mvSlice := a.positionMV[ast]
    qtySlice := a.positionQty[ast]
    // Back-fill NaN so slices reach targetLen-1 before appending.
    for len(mvSlice) < targetLen-1 {
        mvSlice = append(mvSlice, math.NaN())
        qtySlice = append(qtySlice, math.NaN())
    }
    mvSlice = append(mvSlice, mv)
    qtySlice = append(qtySlice, qty)
    a.positionMV[ast] = mvSlice
    a.positionQty[ast] = qtySlice
}
```

Imports unchanged — `log` and `math` are already present.

- [ ] **Step 5: Run the new test — should pass**

Run: `ginkgo run --focus "UpdatePrices per-asset tracking" ./portfolio/`
Expected: PASS (1/1 spec).

- [ ] **Step 6: Run the full portfolio test suite to check for regressions**

Run: `ginkgo run -race ./portfolio/`
Expected: all specs pass.

- [ ] **Step 7: Add test for mid-period buy (side-car back-fills NaN)**

Append to the same `Describe("UpdatePrices per-asset tracking", ...)` block in `portfolio/account_test.go`:

```go
It("back-fills NaN for an asset bought mid-period", func() {
    spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
    bnd := asset.Asset{Ticker: "BND", CompositeFigi: "BBG000BBVR08"}

    acct := portfolio.New(portfolio.WithCash(10000, time.Time{}))

    t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
    acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: asset.BuyTransaction, Qty: 10, Price: 400, Amount: -4000})
    df0 := buildDF(t0, []asset.Asset{spy}, []float64{400}, []float64{400})
    acct.UpdatePrices(df0)

    t1 := t0.AddDate(0, 0, 1)
    acct.Record(portfolio.Transaction{Date: t1, Asset: bnd, Type: asset.BuyTransaction, Qty: 20, Price: 80, Amount: -1600})
    df1 := buildDF(t1, []asset.Asset{spy, bnd}, []float64{405, 81}, []float64{405, 81})
    acct.UpdatePrices(df1)

    mv, qty := acct.PositionSeries()
    Expect(len(mv[bnd])).To(Equal(2))
    Expect(math.IsNaN(mv[bnd][0])).To(BeTrue())
    Expect(mv[bnd][1]).To(Equal(1620.0))
    Expect(math.IsNaN(qty[bnd][0])).To(BeTrue())
    Expect(qty[bnd][1]).To(Equal(20.0))
})
```

If `math` is not yet imported in that file, add it.

- [ ] **Step 8: Add test for mid-period close (zero row then NaN thereafter)**

Append to same block:

```go
It("emits a zero row on close day and NaN on subsequent days", func() {
    spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}

    acct := portfolio.New(portfolio.WithCash(10000, time.Time{}))

    t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
    acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: asset.BuyTransaction, Qty: 10, Price: 400, Amount: -4000})
    df0 := buildDF(t0, []asset.Asset{spy}, []float64{400}, []float64{400})
    acct.UpdatePrices(df0)

    t1 := t0.AddDate(0, 0, 1)
    acct.Record(portfolio.Transaction{Date: t1, Asset: spy, Type: asset.SellTransaction, Qty: 10, Price: 410, Amount: 4100})
    df1 := buildDF(t1, []asset.Asset{spy}, []float64{410}, []float64{410})
    acct.UpdatePrices(df1)

    t2 := t1.AddDate(0, 0, 1)
    df2 := buildDF(t2, []asset.Asset{spy}, []float64{411}, []float64{411})
    acct.UpdatePrices(df2)

    mv, qty := acct.PositionSeries()
    Expect(mv[spy]).To(HaveLen(3))
    Expect(mv[spy][0]).To(Equal(4000.0))
    Expect(mv[spy][1]).To(Equal(0.0))
    Expect(math.IsNaN(mv[spy][2])).To(BeTrue())
    Expect(qty[spy][1]).To(Equal(0.0))
    Expect(math.IsNaN(qty[spy][2])).To(BeTrue())
})
```

- [ ] **Step 9: Run the new tests**

Run: `ginkgo run --focus "UpdatePrices per-asset tracking" ./portfolio/`
Expected: all three Its pass.

- [ ] **Step 10: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go portfolio/export_test.go
git commit -m "feat(portfolio): track per-asset daily mv and qty in Account"
```

---

## Task 3: Add `positions_daily` schema and writer

**Files:**
- Modify: `portfolio/sqlite.go`
- Test: `portfolio/sqlite_test.go`

- [ ] **Step 1: Write a failing test for `positions_daily` rows and schema version**

Append a new Describe to `portfolio/sqlite_test.go` (after the existing "round-trip" Describe):

```go
Describe("positions_daily", func() {
    It("writes one row per (date, ticker) with $CASH included and bumps schema to 5", func() {
        spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
        bnd := asset.Asset{Ticker: "BND", CompositeFigi: "BBG000BBVR08"}

        acct := portfolio.New(portfolio.WithCash(10000, time.Time{}))

        t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
        acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: asset.BuyTransaction, Qty: 10, Price: 400, Amount: -4000})
        acct.Record(portfolio.Transaction{Date: t0, Asset: bnd, Type: asset.BuyTransaction, Qty: 20, Price: 80, Amount: -1600})
        df0 := buildDF(t0, []asset.Asset{spy, bnd}, []float64{400, 80}, []float64{400, 80})
        acct.UpdatePrices(df0)

        t1 := t0.AddDate(0, 0, 1)
        df1 := buildDF(t1, []asset.Asset{spy, bnd}, []float64{410, 81}, []float64{410, 81})
        acct.UpdatePrices(df1)

        dbPath := filepath.Join(tmpDir, "positions.db")
        Expect(acct.ToSQLite(dbPath)).To(Succeed())

        db, err := sql.Open("sqlite", dbPath)
        Expect(err).NotTo(HaveOccurred())
        defer db.Close()

        var schemaVer string
        Expect(db.QueryRow(`SELECT value FROM metadata WHERE key='schema_version'`).Scan(&schemaVer)).To(Succeed())
        Expect(schemaVer).To(Equal("5"))

        var total int
        Expect(db.QueryRow(`SELECT COUNT(*) FROM positions_daily`).Scan(&total)).To(Succeed())
        Expect(total).To(Equal(6)) // 2 days * (SPY + BND + $CASH)

        // Invariant: sum(market_value) per date == PortfolioEquity
        rows, err := db.Query(`
            SELECT pd.date, SUM(pd.market_value), perf.value
            FROM positions_daily pd
            JOIN perf_data perf ON perf.date = pd.date AND perf.metric = 'PortfolioEquity'
            GROUP BY pd.date`)
        Expect(err).NotTo(HaveOccurred())
        defer rows.Close()
        var seen int
        for rows.Next() {
            var date string
            var sumMV, eq float64
            Expect(rows.Scan(&date, &sumMV, &eq)).To(Succeed())
            Expect(math.Abs(sumMV-eq)).To(BeNumerically("<", 1e-4), "date=%s sum=%f eq=%f", date, sumMV, eq)
            seen++
        }
        Expect(seen).To(Equal(2))

        // $CASH encoded with empty figi.
        var cashRows int
        Expect(db.QueryRow(`SELECT COUNT(*) FROM positions_daily WHERE ticker='$CASH' AND figi=''`).Scan(&cashRows)).To(Succeed())
        Expect(cashRows).To(Equal(2))
    })
})
```

Add `"math"` to the imports of `portfolio/sqlite_test.go` if not already present.

- [ ] **Step 2: Run the test to verify it fails**

Run: `ginkgo run --focus "positions_daily" ./portfolio/`
Expected: FAIL — either compile error on missing table, or schema_version mismatch, or missing `positions_daily` table.

- [ ] **Step 3: Bump schemaVersion and add positions_daily DDL**

Edit `portfolio/sqlite.go`:

Change line 31:
```go
const schemaVersion = "5"
```

In `createSchema` (the string constant starting line 35), append these blocks after the last `CREATE INDEX` line (currently `CREATE INDEX idx_annotations_batch ON annotations(batch_id);` at line 104):

```sql

CREATE TABLE positions_daily (
    date         TEXT NOT NULL,
    ticker       TEXT NOT NULL,
    figi         TEXT NOT NULL,
    market_value REAL NOT NULL,
    quantity     REAL NOT NULL,
    PRIMARY KEY (date, ticker, figi)
);

CREATE INDEX idx_positions_daily_ticker ON positions_daily (ticker, date);
```

Update the doc comment on `FromSQLite` (line 503-506): change `schema_version "4"` to `schema_version "5"`.

- [ ] **Step 4: Add `writePositionsDaily` method**

In `portfolio/sqlite.go`, add this function immediately after `writePerfData` (ends at line 312):

```go
func (a *Account) writePositionsDaily(tx *sql.Tx) error {
    if a.perfData == nil || len(a.positionMV) == 0 {
        return nil
    }

    stmt, err := tx.Prepare(`INSERT INTO positions_daily (date, ticker, figi, market_value, quantity) VALUES (?, ?, ?, ?, ?)`)
    if err != nil {
        return fmt.Errorf("prepare positions_daily: %w", err)
    }
    defer stmt.Close()

    times := a.perfData.Times()

    // Stable asset order so snapshots are deterministic.
    assets := make([]asset.Asset, 0, len(a.positionMV))
    for ast := range a.positionMV {
        assets = append(assets, ast)
    }
    sort.Slice(assets, func(i, j int) bool {
        if assets[i].Ticker != assets[j].Ticker {
            return assets[i].Ticker < assets[j].Ticker
        }
        return assets[i].CompositeFigi < assets[j].CompositeFigi
    })

    for _, ast := range assets {
        mvCol := a.positionMV[ast]
        qtyCol := a.positionQty[ast]
        for i := range times {
            if i >= len(mvCol) || i >= len(qtyCol) {
                break
            }
            mv, qty := mvCol[i], qtyCol[i]
            if math.IsNaN(mv) && math.IsNaN(qty) {
                continue
            }
            if math.IsNaN(mv) {
                mv = 0
            }
            if math.IsNaN(qty) {
                qty = 0
            }
            dateStr := times[i].Format(dateFormat)
            if _, err := stmt.Exec(dateStr, ast.Ticker, ast.CompositeFigi, mv, qty); err != nil {
                return fmt.Errorf("insert positions_daily: %w", err)
            }
        }
    }

    return nil
}
```

(`sort` and `math` are already imported in `portfolio/sqlite.go`.)

- [ ] **Step 5: Call `writePositionsDaily` from `ToSQLite`**

In `portfolio/sqlite.go`, in `ToSQLite` (line 163), insert the new call directly after the `writePerfData` call (currently line 192-194). Change:

```go
    // Write perf data.
    if err := a.writePerfData(dbTx); err != nil {
        return err
    }

    // Write transactions.
```

to:

```go
    // Write perf data.
    if err := a.writePerfData(dbTx); err != nil {
        return err
    }

    // Write per-asset daily positions.
    if err := a.writePositionsDaily(dbTx); err != nil {
        return err
    }

    // Write transactions.
```

- [ ] **Step 6: Run the positions_daily test**

Run: `ginkgo run --focus "positions_daily" ./portfolio/`
Expected: PASS.

- [ ] **Step 7: Run the full portfolio suite**

Run: `ginkgo run -race ./portfolio/`
Expected: all specs pass. The pre-existing `schema_version` test at line 373 of `sqlite_test.go` asserts the writer produces schema `"5"` through the round-trip — the existing test should still pass because it only verifies rejection of version `"99"`, but inspect round-trip tests to confirm no hard-coded `"4"` remains.

Run: `grep -n '"4"' portfolio/sqlite_test.go`
If any hit refers to the schema version expected from a successful round-trip, update it to `"5"`.

- [ ] **Step 8: Commit**

```bash
git add portfolio/sqlite.go portfolio/sqlite_test.go
git commit -m "feat(portfolio): write positions_daily table with schema v5"
```

---

## Task 4: Acceptance-criteria edge case tests

**Files:**
- Test: `portfolio/sqlite_test.go`

Context — the design doc calls for tests covering splits, NaN price days, zero-balance cash days, and confirming the `PRAGMA table_info` shape.

- [ ] **Step 1: Write test for `PRAGMA table_info` shape**

Append inside the `Describe("positions_daily", ...)` block:

```go
It("has the expected table shape", func() {
    acct := portfolio.New(portfolio.WithCash(1000, time.Time{}))
    spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
    t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
    acct.UpdatePrices(buildDF(t0, []asset.Asset{spy}, []float64{400}, []float64{400}))

    dbPath := filepath.Join(tmpDir, "shape.db")
    Expect(acct.ToSQLite(dbPath)).To(Succeed())

    db, err := sql.Open("sqlite", dbPath)
    Expect(err).NotTo(HaveOccurred())
    defer db.Close()

    rows, err := db.Query(`PRAGMA table_info(positions_daily)`)
    Expect(err).NotTo(HaveOccurred())
    defer rows.Close()

    type col struct {
        cid     int
        name    string
        coltype string
        notnull int
        pk      int
    }
    var cols []col
    for rows.Next() {
        var c col
        var dflt sql.NullString
        Expect(rows.Scan(&c.cid, &c.name, &c.coltype, &c.notnull, &dflt, &c.pk)).To(Succeed())
        cols = append(cols, c)
    }

    Expect(cols).To(HaveLen(5))
    Expect(cols[0].name).To(Equal("date"))
    Expect(cols[0].coltype).To(Equal("TEXT"))
    Expect(cols[0].pk).To(Equal(1))
    Expect(cols[1].name).To(Equal("ticker"))
    Expect(cols[1].pk).To(Equal(2))
    Expect(cols[2].name).To(Equal("figi"))
    Expect(cols[2].pk).To(Equal(3))
    Expect(cols[3].name).To(Equal("market_value"))
    Expect(cols[3].coltype).To(Equal("REAL"))
    Expect(cols[4].name).To(Equal("quantity"))
    Expect(cols[4].coltype).To(Equal("REAL"))
    for _, c := range cols {
        Expect(c.notnull).To(Equal(1), "column %s should be NOT NULL", c.name)
    }
})
```

- [ ] **Step 2: Write test for split transparency**

Splits are applied via `acct.ApplySplit` (see `account.go`). Append:

```go
It("records post-split quantity and stable market value on split day", func() {
    spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
    acct := portfolio.New(portfolio.WithCash(10000, time.Time{}))

    t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
    acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: asset.BuyTransaction, Qty: 10, Price: 400, Amount: -4000})
    acct.UpdatePrices(buildDF(t0, []asset.Asset{spy}, []float64{400}, []float64{400}))

    t1 := t0.AddDate(0, 0, 1)
    acct.ApplySplit(spy, 2.0, t1)
    // Post-split: 20 shares at $200 close. Same total MV = $4000.
    acct.UpdatePrices(buildDF(t1, []asset.Asset{spy}, []float64{200}, []float64{200}))

    dbPath := filepath.Join(tmpDir, "split.db")
    Expect(acct.ToSQLite(dbPath)).To(Succeed())

    db, err := sql.Open("sqlite", dbPath)
    Expect(err).NotTo(HaveOccurred())
    defer db.Close()

    var qty, mv float64
    Expect(db.QueryRow(`SELECT quantity, market_value FROM positions_daily WHERE ticker='SPY' AND date=?`, t1.Format("2006-01-02")).Scan(&qty, &mv)).To(Succeed())
    Expect(qty).To(Equal(20.0))
    Expect(mv).To(BeNumerically("~", 4000.0, 1e-6))
})
```

Verify `ApplySplit` exists with that signature:

Run: `grep -n "func.*ApplySplit" portfolio/account.go`
Expected: one match. If the signature differs (e.g., takes `(asset, factor, date)` vs `(asset, date, factor)`), adjust the call accordingly.

- [ ] **Step 3: Write test for NaN-price day with prior history (falls back to last-known price)**

Append:

```go
It("falls back to last-known price when close is NaN with prior history", func() {
    spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
    acct := portfolio.New(portfolio.WithCash(10000, time.Time{}))

    t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
    acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: asset.BuyTransaction, Qty: 10, Price: 400, Amount: -4000})
    acct.UpdatePrices(buildDF(t0, []asset.Asset{spy}, []float64{400}, []float64{400}))

    t1 := t0.AddDate(0, 0, 1)
    // NaN close -- stale data
    acct.UpdatePrices(buildDF(t1, []asset.Asset{spy}, []float64{math.NaN()}, []float64{math.NaN()}))

    dbPath := filepath.Join(tmpDir, "nan.db")
    Expect(acct.ToSQLite(dbPath)).To(Succeed())

    db, err := sql.Open("sqlite", dbPath)
    Expect(err).NotTo(HaveOccurred())
    defer db.Close()

    var count int
    var mv float64
    Expect(db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(market_value), 0) FROM positions_daily WHERE ticker='SPY' AND date=?`, t1.Format("2006-01-02")).Scan(&count, &mv)).To(Succeed())
    Expect(count).To(Equal(1))
    Expect(mv).To(Equal(4000.0))
})
```

- [ ] **Step 4: Write test for $CASH zero-balance day**

Append:

```go
It("emits $CASH rows even when cash balance is zero", func() {
    spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
    // Start with exactly enough cash to buy 10 shares at $100 (zero cash after).
    acct := portfolio.New(portfolio.WithCash(1000, time.Time{}))

    t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
    acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: asset.BuyTransaction, Qty: 10, Price: 100, Amount: -1000})
    acct.UpdatePrices(buildDF(t0, []asset.Asset{spy}, []float64{100}, []float64{100}))

    dbPath := filepath.Join(tmpDir, "zerocash.db")
    Expect(acct.ToSQLite(dbPath)).To(Succeed())

    db, err := sql.Open("sqlite", dbPath)
    Expect(err).NotTo(HaveOccurred())
    defer db.Close()

    var mv, qty float64
    Expect(db.QueryRow(`SELECT market_value, quantity FROM positions_daily WHERE ticker='$CASH' AND date=?`, t0.Format("2006-01-02")).Scan(&mv, &qty)).To(Succeed())
    Expect(mv).To(Equal(0.0))
    Expect(qty).To(Equal(0.0))
})
```

- [ ] **Step 5: Run all new tests**

Run: `ginkgo run --focus "positions_daily" ./portfolio/`
Expected: all It blocks pass.

- [ ] **Step 6: Run the full portfolio suite once more**

Run: `ginkgo run -race ./portfolio/`
Expected: all specs pass.

- [ ] **Step 7: Commit**

```bash
git add portfolio/sqlite_test.go
git commit -m "test(portfolio): cover positions_daily edge cases"
```

---

## Task 5: Integration-test SUM invariant end-to-end

**Files:**
- Test: `portfolio/sqlite_test.go` (a new Describe — scoped to 30 days so CI stays fast) OR `cli/cli_test.go` if that path already exercises a real backtest.

- [ ] **Step 1: Pick location**

Run: `ls cli/cli_test.go study/integration_test.go 2>/dev/null`
If `cli/cli_test.go` or `study/integration_test.go` exists and already writes an account to SQLite in a test, extend it. Otherwise add a new Describe in `portfolio/sqlite_test.go`.

- [ ] **Step 2: Write the test**

If adding to `portfolio/sqlite_test.go`, append inside the `Describe("positions_daily", ...)` block:

```go
It("satisfies the sum invariant over a 30-day two-asset run", func() {
    spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
    bnd := asset.Asset{Ticker: "BND", CompositeFigi: "BBG000BBVR08"}

    acct := portfolio.New(portfolio.WithCash(10000, time.Time{}))

    t0 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
    acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: asset.BuyTransaction, Qty: 10, Price: 400, Amount: -4000})
    acct.Record(portfolio.Transaction{Date: t0, Asset: bnd, Type: asset.BuyTransaction, Qty: 20, Price: 80, Amount: -1600})

    days := daySeq(t0, 30)
    for i, d := range days {
        spyPrice := 400.0 + float64(i)
        bndPrice := 80.0 + float64(i)*0.1
        acct.UpdatePrices(buildDF(d, []asset.Asset{spy, bnd}, []float64{spyPrice, bndPrice}, []float64{spyPrice, bndPrice}))
    }

    dbPath := filepath.Join(tmpDir, "invariant.db")
    Expect(acct.ToSQLite(dbPath)).To(Succeed())

    db, err := sql.Open("sqlite", dbPath)
    Expect(err).NotTo(HaveOccurred())
    defer db.Close()

    var rowCount int
    Expect(db.QueryRow(`SELECT COUNT(*) FROM positions_daily`).Scan(&rowCount)).To(Succeed())
    Expect(rowCount).To(Equal(30 * 3)) // 30 days * (SPY + BND + $CASH)

    rows, err := db.Query(`
        SELECT pd.date, SUM(pd.market_value), perf.value
        FROM positions_daily pd
        JOIN perf_data perf ON perf.date = pd.date AND perf.metric = 'PortfolioEquity'
        GROUP BY pd.date`)
    Expect(err).NotTo(HaveOccurred())
    defer rows.Close()

    seen := 0
    for rows.Next() {
        var date string
        var sumMV, eq float64
        Expect(rows.Scan(&date, &sumMV, &eq)).To(Succeed())
        Expect(math.Abs(sumMV-eq)).To(BeNumerically("<", 1e-4), "date=%s sum=%f eq=%f", date, sumMV, eq)
        seen++
    }
    Expect(seen).To(Equal(30))
})
```

- [ ] **Step 3: Run the test**

Run: `ginkgo run --focus "sum invariant" ./portfolio/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add portfolio/sqlite_test.go
git commit -m "test(portfolio): add positions_daily sum invariant test"
```

---

## Task 6: Final verification

- [ ] **Step 1: Run the lint**

Run: `make lint`
Expected: exit 0 with no new findings. If any lint errors surface, fix them — per project rule, no `//nolint` directives.

- [ ] **Step 2: Run the full test suite**

Run: `make test`
Expected: all specs pass.

- [ ] **Step 3: Run the build**

Run: `make build`
Expected: exit 0, `pvbt` binary built.

- [ ] **Step 4: Manual smoke check with an example backtest**

Run a bundled example (pick any small one):

Run: `ls examples/*.go 2>/dev/null | head`
If examples exist, run one that produces a snapshot:

Run: `./pvbt backtest --help | head -40`
Find the flag to produce an SQLite snapshot, run a trivial example, then:

Run: `sqlite3 <snapshot.db> "SELECT COUNT(*) FROM positions_daily; SELECT value FROM metadata WHERE key='schema_version';"`
Expected: COUNT > 0; schema_version = 5.

If no trivial example is available, skip this step.

- [ ] **Step 5: Update the changelog**

Edit `CHANGELOG.md` under `[Unreleased]` / `Added`:

```
- The SQLite snapshot now includes a `positions_daily` table recording each ticker's daily market value and quantity, so consumers can compute per-ticker contribution without replaying prices. `$CASH` participates as a position. Snapshot schema version is `5`; older snapshots are incompatible.
```

- [ ] **Step 6: Commit changelog**

```bash
git add CHANGELOG.md
git commit -m "docs: note positions_daily and schema v5 in changelog"
```

---

## Self-review notes

- Spec §Schema → Task 3 (DDL, PK, index, schema bump).
- Spec §Row semantics ($CASH every day, zero-balance included, NaN policy, exit-day zero row) → Tasks 2, 3, 4.
- Spec §In-memory tracking → Tasks 1, 2. We use a side-car map rather than calling `perfData.Insert` each step for O(N) efficiency; perfData is the reader of truth inside pvbt today, and `writePositionsDaily` reads from the side-car directly, which is the same contract.
- Spec §Writer → Task 3 Step 4 (emits one row per non-NaN cell, sorted for determinism, called after writePerfData before writeHoldings — note we call it directly after writePerfData per the spec's "after writePerfData, before writeHoldings" ordering).
- Spec §Acceptance criteria 1-4 → Tasks 3 & 4 & 5.
- Spec §Acceptance criteria 5 (no regressions) → Task 6 `make test`.
- `data.DataFrame.Assets()` accessor — we don't need it because the writer reads from `a.positionMV` / `a.positionQty` directly.
