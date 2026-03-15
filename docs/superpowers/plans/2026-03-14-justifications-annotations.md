# Justifications and Annotations Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two features: step-level annotations for capturing strategy reasoning, and transaction-level justifications for explaining individual trades.

**Architecture:** Annotations are an append-only `[]Annotation` log on Account, with a `DataFrame.Annotate` method that decomposes DataFrames into per-cell entries via a narrow `Annotator` interface in the data package. Justifications are a string field on `Allocation` and `Transaction`, threaded through `submitAndRecord`. Both are serialized to SQLite.

**Tech Stack:** Go, Ginkgo/Gomega, SQLite (modernc.org/sqlite)

**Spec:** `docs/superpowers/specs/2026-03-14-justifications-annotations-design.md`

---

## Chunk 1: Annotations

### Task 1: Annotation struct and Account storage

**Files:**
- Create: `portfolio/annotation.go`
- Modify: `portfolio/portfolio.go:31-121`
- Modify: `portfolio/account.go:44-57,60-76`

- [ ] **Step 1: Create the Annotation struct**

Create `portfolio/annotation.go`:

```go
package portfolio

// Annotation is a single key-value entry recorded by a strategy to explain
// its reasoning at a point in time.
type Annotation struct {
	Timestamp int64
	Key       string
	Value     string
}
```

- [ ] **Step 2: Add Annotate and Annotations to the Portfolio interface**

In `portfolio/portfolio.go`, add these methods to the `Portfolio` interface, after `GetMetadata`:

```go
	// Annotate records a key-value annotation for the given timestamp.
	// Call this during Compute to capture intermediate computations
	// that explain why the strategy made its decisions. Multiple calls
	// accumulate entries.
	Annotate(timestamp int64, key, value string)

	// Annotations returns the full annotation log in the order entries
	// were recorded.
	Annotations() []Annotation
```

- [ ] **Step 3: Add annotations field to Account struct**

In `portfolio/account.go`, add `annotations` to the Account struct (after `metrics`):

```go
	annotations       []Annotation
```

- [ ] **Step 4: Implement Annotate and Annotations on Account**

In `portfolio/account.go`, add after the `SetMetadata`/`GetMetadata` methods (or near the bottom of the Portfolio interface section):

```go
// Annotate records a key-value annotation for the given timestamp.
func (a *Account) Annotate(timestamp int64, key, value string) {
	a.annotations = append(a.annotations, Annotation{
		Timestamp: timestamp,
		Key:       key,
		Value:     value,
	})
}

// Annotations returns the full annotation log in the order entries
// were recorded.
func (a *Account) Annotations() []Annotation {
	return a.annotations
}
```

- [ ] **Step 5: Run build to verify compilation**

Run: `go build ./...`
Expected: Pass (but tests may fail if any mock implementations of Portfolio exist that need the new methods).

- [ ] **Step 6: Check for and fix any mock Portfolio implementations**

Search for other implementations of the Portfolio interface in test files. If any exist, add stub `Annotate` and `Annotations` methods.

- [ ] **Step 7: Write tests for Account.Annotate and Annotations**

Create `portfolio/annotation_test.go`:

```go
package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Annotations", func() {
	It("records and returns annotations in order", func() {
		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

		ts := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC).Unix()
		acct.Annotate(ts, "SPY/Momentum", "0.87")
		acct.Annotate(ts, "bond_fraction", "0.3")

		annotations := acct.Annotations()
		Expect(annotations).To(HaveLen(2))
		Expect(annotations[0].Timestamp).To(Equal(ts))
		Expect(annotations[0].Key).To(Equal("SPY/Momentum"))
		Expect(annotations[0].Value).To(Equal("0.87"))
		Expect(annotations[1].Key).To(Equal("bond_fraction"))
		Expect(annotations[1].Value).To(Equal("0.3"))
	})

	It("returns nil when no annotations have been recorded", func() {
		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
		Expect(acct.Annotations()).To(BeNil())
	})
})
```

- [ ] **Step 8: Run tests**

Run: `go test ./portfolio/ -run Annotations -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add portfolio/annotation.go portfolio/portfolio.go portfolio/account.go portfolio/annotation_test.go
git commit -m "feat: add Annotation struct and Portfolio.Annotate/Annotations methods"
```

### Task 2: Annotator interface and DataFrame.Annotate

**Files:**
- Create: `data/annotator.go`
- Modify: `data/data_frame.go`
- Test: `data/data_frame_test.go`

- [ ] **Step 1: Create the Annotator interface**

Create `data/annotator.go`:

```go
package data

// Annotator receives key-value annotations. Portfolio satisfies this
// interface, allowing DataFrame.Annotate to push entries without
// depending on the portfolio package.
type Annotator interface {
	Annotate(timestamp int64, key, value string)
}
```

- [ ] **Step 2: Write the failing test for DataFrame.Annotate**

Add to `data/data_frame_test.go`:

```go
type mockAnnotator struct {
	entries []struct {
		timestamp int64
		key       string
		value     string
	}
}

func (m *mockAnnotator) Annotate(timestamp int64, key, value string) {
	m.entries = append(m.entries, struct {
		timestamp int64
		key       string
		value     string
	}{timestamp, key, value})
}

var _ = Describe("DataFrame.Annotate", func() {
	It("pushes non-NaN cells as annotations with TICKER/Metric keys", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		efa := asset.Asset{CompositeFigi: "EFA001", Ticker: "EFA"}
		t1 := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, efa},
			[]data.Metric{data.MetricClose},
			[]float64{150.5, 75.25},
		)
		Expect(err).NotTo(HaveOccurred())

		dest := &mockAnnotator{}
		result := df.Annotate(dest)
		Expect(result.Err()).NotTo(HaveOccurred())

		Expect(dest.entries).To(HaveLen(2))

		// Sort by key to make assertion order-independent.
		keys := make(map[string]string)
		for _, entry := range dest.entries {
			Expect(entry.timestamp).To(Equal(t1.Unix()))
			keys[entry.key] = entry.value
		}

		Expect(keys).To(HaveKey("SPY/Close"))
		Expect(keys).To(HaveKey("EFA/Close"))
	})

	It("skips NaN values", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		t1 := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{math.NaN()},
		)
		Expect(err).NotTo(HaveOccurred())

		dest := &mockAnnotator{}
		df.Annotate(dest)
		Expect(dest.entries).To(BeEmpty())
	})

	It("is a no-op when DataFrame has an error", func() {
		errDF := data.WithErr(fmt.Errorf("test error"))

		dest := &mockAnnotator{}
		errDF.Annotate(dest)
		Expect(dest.entries).To(BeEmpty())
	})

	It("handles multiple rows and metrics", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		t1 := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		t2 := time.Date(2024, 1, 16, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose, data.AdjClose},
			[]float64{150.0, 151.0, 149.5, 150.5},
		)
		Expect(err).NotTo(HaveOccurred())

		dest := &mockAnnotator{}
		df.Annotate(dest)
		Expect(dest.entries).To(HaveLen(4))
	})
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./data/ -run "DataFrame.Annotate" -v`
Expected: FAIL (method does not exist)

- [ ] **Step 4: Implement DataFrame.Annotate**

Add to `data/data_frame.go`:

```go
// Annotate pushes every non-NaN cell in the DataFrame as a key-value
// annotation to the destination. Keys are formatted as "TICKER/Metric".
// Values are the float formatted with strconv.FormatFloat(v, 'f', -1, 64).
// Returns the DataFrame for chaining. If the DataFrame has an error,
// this is a no-op.
func (df *DataFrame) Annotate(dest Annotator) *DataFrame {
	if df.err != nil {
		return df
	}

	times := df.Times()
	assets := df.AssetList()
	metrics := df.MetricList()

	for _, t := range times {
		ts := t.Unix()
		for _, a := range assets {
			for _, m := range metrics {
				v := df.ValueAt(a, m, t)
				if !math.IsNaN(v) {
					dest.Annotate(ts, a.Ticker+"/"+string(m), strconv.FormatFloat(v, 'f', -1, 64))
				}
			}
		}
	}

	return df
}
```

Add `"strconv"` to the import block in `data/data_frame.go` (`math` is already imported).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./data/ -run "DataFrame.Annotate" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add data/annotator.go data/data_frame.go data/data_frame_test.go
git commit -m "feat: add Annotator interface and DataFrame.Annotate method"
```

## Chunk 2: Justifications

### Task 3: Justification field on Transaction and Allocation

**Files:**
- Modify: `portfolio/transaction.go:72-102`
- Modify: `portfolio/allocation.go:26-29`

- [ ] **Step 1: Add Justification field to Transaction**

In `portfolio/transaction.go`, add after the `Qualified` field (line 101):

```go
	// Justification is an optional explanation of why this trade was made.
	// Set automatically from the Allocation's Justification field during
	// RebalanceTo, or from the WithJustification OrderModifier during Order.
	Justification string
```

- [ ] **Step 2: Add Justification field to Allocation**

In `portfolio/allocation.go`, add after the `Members` field (line 28):

```go
	// Justification is an optional explanation of why this allocation
	// was chosen. Copied onto every Transaction generated by RebalanceTo.
	Justification string
```

- [ ] **Step 3: Run build**

Run: `go build ./...`
Expected: Pass

- [ ] **Step 4: Commit**

```bash
git add portfolio/transaction.go portfolio/allocation.go
git commit -m "feat: add Justification field to Transaction and Allocation"
```

### Task 4: Thread justification through submitAndRecord

**Files:**
- Modify: `portfolio/account.go:125-202` (RebalanceTo)
- Modify: `portfolio/account.go:203-258` (Order)
- Modify: `portfolio/account.go:260-294` (submitAndRecord)
- Modify: `portfolio/order.go`
- Test: `portfolio/rebalance_test.go`

- [ ] **Step 1: Add WithJustification OrderModifier**

In `portfolio/order.go`, add after the `GoodTilDate` function (end of file):

```go
type justificationModifier struct{ reason string }

func (justificationModifier) orderModifier() {}

// WithJustification attaches an explanation to the resulting transaction.
func WithJustification(reason string) OrderModifier {
	return justificationModifier{reason: reason}
}
```

- [ ] **Step 2: Add justification parameter to submitAndRecord**

In `portfolio/account.go`, change `submitAndRecord` signature from:

```go
func (a *Account) submitAndRecord(ctx context.Context, ast asset.Asset, side Side, order broker.Order) error {
```

to:

```go
func (a *Account) submitAndRecord(ctx context.Context, ast asset.Asset, side Side, order broker.Order, justification string) error {
```

And in the Transaction construction inside the for loop (around line 283), add the Justification field:

```go
		a.Record(Transaction{
			Date:          fill.FilledAt,
			Asset:         ast,
			Type:          txType,
			Qty:           fill.Qty,
			Price:         fill.Price,
			Amount:        amount,
			Justification: justification,
		})
```

- [ ] **Step 3: Update RebalanceTo to pass alloc.Justification**

In `portfolio/account.go`, update both `submitAndRecord` calls in `RebalanceTo` (around lines 166 and 195) from:

```go
if err := a.submitAndRecord(ctx, o.asset, Sell, order); err != nil {
```

to:

```go
if err := a.submitAndRecord(ctx, o.asset, Sell, order, alloc.Justification); err != nil {
```

And similarly for the buy call:

```go
if err := a.submitAndRecord(ctx, o.asset, Buy, order, alloc.Justification); err != nil {
```

- [ ] **Step 4: Update Order to extract and pass justification**

In `portfolio/account.go`, in the `Order` method, add a `justification` variable and extract it from the modifier list. Add a case to the type switch (around line 223):

```go
		case justificationModifier:
			justification = m.reason
```

Declare `var justification string` before the modifier loop (around line 220, after `var hasLimit, hasStop bool`):

```go
	var hasLimit, hasStop bool
	var justification string
```

And update the final `submitAndRecord` call (around line 257) from:

```go
	return a.submitAndRecord(ctx, ast, side, order)
```

to:

```go
	return a.submitAndRecord(ctx, ast, side, order, justification)
```

- [ ] **Step 5: Run build**

Run: `go build ./...`
Expected: Pass

- [ ] **Step 6: Write test for justification propagation via RebalanceTo**

Add to `portfolio/rebalance_test.go`, inside the existing `Describe("RebalanceTo", ...)` block:

```go
	It("copies Allocation.Justification onto generated transactions", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{500},
		)
		Expect(err).NotTo(HaveOccurred())

		mb := &mockBroker{}
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy: {{Price: 500.0, Qty: 100, FilledAt: fill}},
		}

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mb))
		acct.UpdatePrices(df)

		err = acct.RebalanceTo(context.Background(), portfolio.Allocation{
			Date:          t1,
			Members:       map[asset.Asset]float64{spy: 1.0},
			Justification: "momentum crossover signal",
		})
		Expect(err).NotTo(HaveOccurred())

		txns := acct.Transactions()
		// First transaction is the initial deposit, second is the buy.
		buyTxns := filterTransactions(txns, portfolio.BuyTransaction)
		Expect(buyTxns).NotTo(BeEmpty())
		Expect(buyTxns[0].Justification).To(Equal("momentum crossover signal"))
	})

	It("leaves Justification empty when Allocation has no justification", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{500},
		)
		Expect(err).NotTo(HaveOccurred())

		mb := &mockBroker{}
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy: {{Price: 500.0, Qty: 100, FilledAt: fill}},
		}

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mb))
		acct.UpdatePrices(df)

		err = acct.RebalanceTo(context.Background(), portfolio.Allocation{
			Date:    t1,
			Members: map[asset.Asset]float64{spy: 1.0},
		})
		Expect(err).NotTo(HaveOccurred())

		txns := acct.Transactions()
		buyTxns := filterTransactions(txns, portfolio.BuyTransaction)
		Expect(buyTxns).NotTo(BeEmpty())
		Expect(buyTxns[0].Justification).To(BeEmpty())
	})
```

Also add the helper function `filterTransactions` near the top of the test file (after imports):

```go
func filterTransactions(txns []portfolio.Transaction, txType portfolio.TransactionType) []portfolio.Transaction {
	var result []portfolio.Transaction
	for _, tx := range txns {
		if tx.Type == txType {
			result = append(result, tx)
		}
	}
	return result
}
```

- [ ] **Step 7: Write test for WithJustification OrderModifier**

Add to `portfolio/rebalance_test.go` (or a separate Describe block in the same file):

```go
var _ = Describe("Order WithJustification", func() {
	It("attaches justification to the resulting transaction", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		fill := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{500},
		)
		Expect(err).NotTo(HaveOccurred())

		mb := &mockBroker{}
		mb.fillsByAsset = map[asset.Asset][]broker.Fill{
			spy: {{Price: 500.0, Qty: 10, FilledAt: fill}},
		}

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithBroker(mb))
		acct.UpdatePrices(df)

		err = acct.Order(context.Background(), spy, portfolio.Buy, 10,
			portfolio.WithJustification("price below 200-day MA"),
		)
		Expect(err).NotTo(HaveOccurred())

		txns := acct.Transactions()
		buyTxns := filterTransactions(txns, portfolio.BuyTransaction)
		Expect(buyTxns).NotTo(BeEmpty())
		Expect(buyTxns[0].Justification).To(Equal("price below 200-day MA"))
	})
})
```

- [ ] **Step 8: Run tests**

Run: `go test ./portfolio/ -run "Justification|WithJustification" -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add portfolio/order.go portfolio/account.go portfolio/rebalance_test.go
git commit -m "feat: thread justification through RebalanceTo, Order, and submitAndRecord"
```

## Chunk 3: SQLite Serialization

### Task 5: SQLite schema and serialization updates

**Files:**
- Modify: `portfolio/sqlite.go`
- Test: `portfolio/sqlite_test.go`

- [ ] **Step 1: Bump schema version and update createSchema**

In `portfolio/sqlite.go`, change:

```go
const schemaVersion = "2"
```

to:

```go
const schemaVersion = "3"
```

In the `createSchema` const, add the annotations table after the metrics table and its index:

```sql
CREATE TABLE annotations (
    timestamp INTEGER NOT NULL,
    key       TEXT NOT NULL,
    value     TEXT NOT NULL
);

CREATE INDEX idx_annotations_timestamp ON annotations(timestamp);
```

Update the transactions table in `createSchema` to add the `justification` column. Change:

```sql
CREATE TABLE transactions (
    date      TEXT NOT NULL,
    type      TEXT NOT NULL,
    ticker    TEXT,
    figi      TEXT,
    quantity  REAL,
    price     REAL,
    amount    REAL,
    qualified INTEGER
);
```

to:

```sql
CREATE TABLE transactions (
    date          TEXT NOT NULL,
    type          TEXT NOT NULL,
    ticker        TEXT,
    figi          TEXT,
    quantity      REAL,
    price         REAL,
    amount        REAL,
    qualified     INTEGER,
    justification TEXT
);
```

- [ ] **Step 2: Add writeAnnotations method**

Add to `portfolio/sqlite.go`, following the pattern of `writeMetrics`:

```go
func (a *Account) writeAnnotations(tx *sql.Tx) error {
	if len(a.annotations) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO annotations (timestamp, key, value) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare annotations: %w", err)
	}
	defer stmt.Close()

	for _, ann := range a.annotations {
		if _, err := stmt.Exec(ann.Timestamp, ann.Key, ann.Value); err != nil {
			return fmt.Errorf("insert annotation: %w", err)
		}
	}

	return nil
}
```

- [ ] **Step 3: Add readAnnotations method**

Add to `portfolio/sqlite.go`:

```go
func (a *Account) readAnnotations(db *sql.DB) error {
	rows, err := db.Query("SELECT timestamp, key, value FROM annotations ORDER BY timestamp, key")
	if err != nil {
		return fmt.Errorf("query annotations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ann Annotation
		if err := rows.Scan(&ann.Timestamp, &ann.Key, &ann.Value); err != nil {
			return fmt.Errorf("scan annotation: %w", err)
		}
		a.annotations = append(a.annotations, ann)
	}

	return rows.Err()
}
```

- [ ] **Step 4: Update writeTransactions to include justification**

In `portfolio/sqlite.go`, update the `writeTransactions` method. Change the prepared statement from:

```go
stmt, err := tx.Prepare("INSERT INTO transactions (date, type, ticker, figi, quantity, price, amount, qualified) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
```

to:

```go
stmt, err := tx.Prepare("INSERT INTO transactions (date, type, ticker, figi, quantity, price, amount, qualified, justification) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)")
```

And update the `stmt.Exec` call to include `t.Justification`:

```go
if _, err := stmt.Exec(d, typStr, t.Asset.Ticker, t.Asset.CompositeFigi, t.Qty, t.Price, t.Amount, qualified, sql.NullString{String: t.Justification, Valid: t.Justification != ""}); err != nil {
```

- [ ] **Step 5: Update readTransactions to scan justification**

In `portfolio/sqlite.go`, in `readTransactions`, add `justification` to the SELECT query:

```go
rows, err := db.Query("SELECT date, type, ticker, figi, quantity, price, amount, qualified, justification FROM transactions ORDER BY date")
```

Add a variable for scanning:

```go
var justification sql.NullString
```

Update `rows.Scan` to include `&justification`:

```go
if err := rows.Scan(&dateStr, &typStr, &ticker, &figi, &qty, &price, &amount, &qualified, &justification); err != nil {
```

And set the field on the Transaction:

```go
if justification.Valid {
    tx.Justification = justification.String
}
```

- [ ] **Step 6: Wire writeAnnotations and readAnnotations into ToSQLite/FromSQLite**

In `ToSQLite`, add after `writeMetrics`:

```go
	// Write annotations.
	if err := a.writeAnnotations(tx); err != nil {
		return err
	}
```

In `FromSQLite`, add after `readMetrics`:

```go
	// Read annotations.
	if err := a.readAnnotations(db); err != nil {
		return nil, err
	}
```

- [ ] **Step 7: Update FromSQLite docstring and schema version check**

In `portfolio/sqlite.go`, update the `FromSQLite` docstring (line 380) from `schema_version "2"` to `schema_version "3"`. The schema version check itself compares against the `schemaVersion` const, so the const update in Step 1 handles the logic.

- [ ] **Step 8: Write round-trip test for annotations**

Add inside the existing `Describe("round-trip", ...)` block in `portfolio/sqlite_test.go`
(which has `tmpDir` available from `BeforeEach`):

```go
It("round-trips annotations", func() {
    acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

    ts1 := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC).Unix()
    ts2 := time.Date(2024, 2, 15, 16, 0, 0, 0, time.UTC).Unix()
    acct.Annotate(ts1, "SPY/Momentum", "0.87")
    acct.Annotate(ts1, "bond_fraction", "0.3")
    acct.Annotate(ts2, "SPY/Momentum", "0.92")

    path := filepath.Join(tmpDir, "annotations.db")
    Expect(acct.ToSQLite(path)).To(Succeed())

    restored, err := portfolio.FromSQLite(path)
    Expect(err).NotTo(HaveOccurred())

    annotations := restored.Annotations()
    Expect(annotations).To(HaveLen(3))
    Expect(annotations[0].Timestamp).To(Equal(ts1))
    Expect(annotations[0].Key).To(Equal("SPY/Momentum"))
    Expect(annotations[0].Value).To(Equal("0.87"))
    Expect(annotations[2].Timestamp).To(Equal(ts2))
})
```

- [ ] **Step 9: Write round-trip test for transaction justification**

Add inside the same `Describe("round-trip", ...)` block:

```go
It("round-trips transaction justification", func() {
    acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

    acct.Record(portfolio.Transaction{
        Date:          time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
        Asset:         asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"},
        Type:          portfolio.BuyTransaction,
        Qty:           10,
        Price:         500,
        Amount:        -5000,
        Justification: "momentum crossover",
    })

    // Also record a transaction without justification.
    acct.Record(portfolio.Transaction{
        Date:   time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
        Asset:  asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"},
        Type:   portfolio.SellTransaction,
        Qty:    5,
        Price:  510,
        Amount: 2550,
    })

    path := filepath.Join(tmpDir, "justification.db")
    Expect(acct.ToSQLite(path)).To(Succeed())

    restored, err := portfolio.FromSQLite(path)
    Expect(err).NotTo(HaveOccurred())

    txns := restored.Transactions()
    // First is the deposit from WithCash, then our two trades.
    Expect(txns).To(HaveLen(3))
    Expect(txns[1].Justification).To(Equal("momentum crossover"))
    Expect(txns[2].Justification).To(BeEmpty())
})
```

- [ ] **Step 10: Run all tests**

Run: `go test ./portfolio/ -v`
Expected: All pass

- [ ] **Step 11: Run full test suite**

Run: `go build ./... && go test ./...`
Expected: All pass

- [ ] **Step 12: Commit**

```bash
git add portfolio/sqlite.go portfolio/sqlite_test.go
git commit -m "feat: serialize annotations and justifications to SQLite"
```
