# Per-Batch History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Record a monotonic batch identifier on every transaction and annotation so a SQLite snapshot can reconstruct holdings after each batch.

**Architecture:** Each `ExecuteBatch` call assigns a new integer `BatchID` (starting at 1). The id is carried on `broker.Order`, `portfolio.Transaction`, and `portfolio.Annotation`. Fills propagate the id through the pending-order map. Non-batch transactions (deposits, stock splits, direct `Record` calls outside `ExecuteBatch`) keep `BatchID = 0`. A new `batches(batch_id, timestamp)` SQLite table indexes every batch so empty-batch entries are preserved. Schema version bumps from `"3"` to `"4"`.

**Tech Stack:** Go, Ginkgo/Gomega, SQLite (`modernc.org/sqlite` driver via `database/sql`), zerolog.

---

## File Structure

**Modified:**
- `broker/broker.go` — add `BatchID int` field to `Order`.
- `portfolio/transaction.go` — add `BatchID int` field to `Transaction`.
- `portfolio/annotation.go` — add `BatchID int` field to `Annotation`.
- `portfolio/account.go` — add `batches []batchRecord` and `currentBatchID int` fields; wire them through `New`, `ExecuteBatch`, `Annotate`, `drainFillsFromChannel`, `submitBracketExits`, `deferredExitInfo`.
- `portfolio/sqlite.go` — bump `schemaVersion`, update `createSchema`, add `writeBatches` / `readBatches`, update `writeTransactions` / `readTransactions` / `writeAnnotations` / `readAnnotations` to round-trip `batch_id`.

**New tests:**
- `portfolio/batch_history_test.go` (Ginkgo) — round-trip test that runs multiple `ExecuteBatch` calls, serializes to SQLite, reloads, and verifies that transactions/annotations/batches carry the expected `BatchID`.

---

### Task 1: Add `BatchID` field to `broker.Order`

**Files:**
- Modify: `broker/broker.go:92-115`

- [ ] **Step 1: Add the field**

Edit the `Order` struct at `broker/broker.go:92-115`, inserting after the `GroupRole` field:

```go
// BatchID identifies the portfolio batch that produced this order.
// Zero means the order did not originate from a portfolio.Batch
// (e.g., broker-internal housekeeping orders).
BatchID int
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./broker/...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add broker/broker.go
git commit -m "feat(broker): add BatchID field to Order"
```

---

### Task 2: Add `BatchID` field to `portfolio.Transaction` and `portfolio.Annotation`

**Files:**
- Modify: `portfolio/transaction.go:27-73`
- Modify: `portfolio/annotation.go:26-30`

- [ ] **Step 1: Add `BatchID` to `Transaction`**

Edit `portfolio/transaction.go`, append before the closing brace of the struct:

```go
// BatchID is the portfolio batch that produced this transaction.
// Zero means the transaction was recorded outside any batch
// (deposits, withdrawals, stock splits, manual Record calls).
// Batch IDs start at 1 and increment monotonically within an Account.
BatchID int
```

- [ ] **Step 2: Add `BatchID` to `Annotation`**

Edit `portfolio/annotation.go`, update the struct to:

```go
type Annotation struct {
	Timestamp time.Time
	Key       string
	Value     string
	// BatchID is the portfolio batch that was active when the
	// annotation was recorded. Zero means no batch was active.
	BatchID int
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./portfolio/...`
Expected: no errors. Existing `Transaction{}` and `Annotation{}` literals get `BatchID: 0` by default.

- [ ] **Step 4: Commit**

```bash
git add portfolio/transaction.go portfolio/annotation.go
git commit -m "feat(portfolio): add BatchID field to Transaction and Annotation"
```

---

### Task 3: Add batch-tracking state to `Account`

**Files:**
- Modify: `portfolio/account.go:44-76` (struct + constructor)

- [ ] **Step 1: Introduce the `batchRecord` type**

Near the top of `portfolio/account.go` (after the import block, above the existing `portfolioAsset` var), add:

```go
// batchRecord captures the timestamp of a single ExecuteBatch call.
// The index in Account.batches plus one is the batch id.
type batchRecord struct {
	BatchID   int
	Timestamp time.Time
}
```

- [ ] **Step 2: Add fields to `Account`**

In the `Account` struct (`portfolio/account.go:44-76`), add after the `seenTransactions` field:

```go
batches         []batchRecord
currentBatchID  int
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./portfolio/...`
Expected: no errors. `currentBatchID` defaults to 0 (no active batch), matching our sentinel.

- [ ] **Step 4: Commit**

```bash
git add portfolio/account.go
git commit -m "feat(portfolio): track batch history on Account"
```

---

### Task 4: Stamp `BatchID` on annotations

**Files:**
- Modify: `portfolio/account.go:1843-1859` (Annotate method)
- Test: `portfolio/annotation_test.go`

- [ ] **Step 1: Write a failing test**

Append to `portfolio/annotation_test.go` inside the existing `Describe` block:

```go
It("stamps BatchID from the account's current batch context", func() {
	acct := portfolio.New(portfolio.WithCash(1000, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	portfolio.SetAccountCurrentBatchID(acct, 7)

	acct.Annotate(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "score", "0.42")

	anns := acct.Annotations()
	Expect(anns).To(HaveLen(1))
	Expect(anns[0].BatchID).To(Equal(7))
})
```

- [ ] **Step 2: Add a test helper for access to unexported state**

Create `portfolio/export_test.go`:

```go
package portfolio

// SetAccountCurrentBatchID exposes Account.currentBatchID for tests.
func SetAccountCurrentBatchID(a *Account, id int) {
	a.currentBatchID = id
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `ginkgo run ./portfolio -focus "stamps BatchID from the account's current batch context"`
Expected: FAIL — BatchID is 0 because Annotate does not yet stamp it.

- [ ] **Step 4: Update `Annotate` to stamp the BatchID**

Edit `portfolio/account.go:1843-1858`, replacing the body of `Annotate`:

```go
func (a *Account) Annotate(timestamp time.Time, key, value string) {
	for idx := range a.annotations {
		if a.annotations[idx].Timestamp.Equal(timestamp) && a.annotations[idx].Key == key {
			a.annotations[idx].Value = value
			a.annotations[idx].BatchID = a.currentBatchID
			return
		}
	}

	a.annotations = append(a.annotations, Annotation{
		Timestamp: timestamp,
		Key:       key,
		Value:     value,
		BatchID:   a.currentBatchID,
	})
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `ginkgo run ./portfolio -focus "stamps BatchID from the account's current batch context"`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add portfolio/account.go portfolio/annotation_test.go portfolio/export_test.go
git commit -m "feat(portfolio): stamp BatchID on annotations"
```

---

### Task 5: Assign BatchID in ExecuteBatch and stamp outgoing orders

**Files:**
- Modify: `portfolio/account.go:1885-1990` (ExecuteBatch)

- [ ] **Step 1: Write a failing test**

Append to `portfolio/account_test.go` a new `Describe("batch history", ...)` block:

```go
Describe("batch history", func() {
	It("assigns sequential BatchIDs starting at 1 and stamps orders", func() {
		ctx := context.Background()
		acct := newTestAccount()                      // helper from testutil_test.go
		broker := newTestBroker(acct)                 // helper
		acct.SetBroker(broker)

		batch1 := portfolio.NewBatch(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), acct)
		Expect(batch1.Order(ctx, testAssetSPY, portfolio.Buy, 10)).To(Succeed())
		Expect(acct.ExecuteBatch(ctx, batch1)).To(Succeed())

		batch2 := portfolio.NewBatch(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), acct)
		Expect(batch2.Order(ctx, testAssetSPY, portfolio.Sell, 5)).To(Succeed())
		Expect(acct.ExecuteBatch(ctx, batch2)).To(Succeed())

		batches := portfolio.GetAccountBatches(acct)
		Expect(batches).To(HaveLen(2))
		Expect(batches[0].BatchID).To(Equal(1))
		Expect(batches[1].BatchID).To(Equal(2))
		Expect(batches[0].Timestamp).To(Equal(batch1.Timestamp))
	})

	It("resets currentBatchID to 0 after ExecuteBatch returns", func() {
		ctx := context.Background()
		acct := newTestAccount()
		broker := newTestBroker(acct)
		acct.SetBroker(broker)

		batch := portfolio.NewBatch(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), acct)
		Expect(acct.ExecuteBatch(ctx, batch)).To(Succeed())

		Expect(portfolio.GetAccountCurrentBatchID(acct)).To(BeZero())
	})
})
```

- [ ] **Step 2: Extend the test helper**

Edit `portfolio/export_test.go`, append:

```go
func GetAccountBatches(a *Account) []batchRecord {
	return a.batches
}

func GetAccountCurrentBatchID(a *Account) int {
	return a.currentBatchID
}
```

- [ ] **Step 3: Run tests to verify failure**

Run: `ginkgo run ./portfolio -focus "batch history"`
Expected: FAIL — batches slice is empty, currentBatchID never set.

- [ ] **Step 4: Update ExecuteBatch**

Edit `portfolio/account.go:1885` — at the start of `ExecuteBatch`, before middleware runs, insert:

```go
batchID := len(a.batches) + 1
a.batches = append(a.batches, batchRecord{BatchID: batchID, Timestamp: batch.Timestamp})
a.currentBatchID = batchID
defer func() { a.currentBatchID = 0 }()
```

Then, in step 4 of the existing `ExecuteBatch` body (around `portfolio/account.go:1908-1919`), where each order's ID is assigned, also stamp `BatchID`:

```go
for idx := range batch.Orders {
	order := &batch.Orders[idx]
	if order.ID == "" {
		order.ID = fmt.Sprintf("batch-%d-%d", batch.Timestamp.UnixNano(), idx)
	}
	order.BatchID = batchID  // NEW

	a.pendingOrders[order.ID] = *order
	...
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `ginkgo run ./portfolio -focus "batch history"`
Expected: PASS.

- [ ] **Step 6: Run full portfolio suite to check for regressions**

Run: `ginkgo run -race ./portfolio`
Expected: all existing tests pass.

- [ ] **Step 7: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go portfolio/export_test.go
git commit -m "feat(portfolio): assign BatchID in ExecuteBatch and stamp orders"
```

---

### Task 6: Propagate BatchID from orders to fill transactions

**Files:**
- Modify: `portfolio/account.go:2014-2093` (drainFillsFromChannel)
- Modify: `portfolio/account.go:2000-2009` (deferredExitInfo)
- Modify: `portfolio/account.go:2129-2180` (submitBracketExits)

- [ ] **Step 1: Write a failing test**

Append to `portfolio/account_test.go` within the `Describe("batch history", ...)` block:

```go
It("copies order.BatchID onto the recorded transaction", func() {
	ctx := context.Background()
	acct := newTestAccount()
	broker := newTestBroker(acct)
	acct.SetBroker(broker)

	batch := portfolio.NewBatch(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), acct)
	Expect(batch.Order(ctx, testAssetSPY, portfolio.Buy, 10)).To(Succeed())
	Expect(acct.ExecuteBatch(ctx, batch)).To(Succeed())

	var tradeTxn portfolio.Transaction
	for _, txn := range acct.Transactions() {
		if txn.Type == asset.BuyTransaction {
			tradeTxn = txn
			break
		}
	}
	Expect(tradeTxn.BatchID).To(Equal(1))
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `ginkgo run ./portfolio -focus "copies order.BatchID onto the recorded transaction"`
Expected: FAIL — `BatchID` on fill txn is 0.

- [ ] **Step 3: Update the fill-to-transaction mapping**

Edit `portfolio/account.go:2047-2056` inside `drainFillsFromChannel`. Change the `a.Record(Transaction{...})` to include `BatchID: order.BatchID`:

```go
a.Record(Transaction{
	Date:          fill.FilledAt,
	Asset:         order.Asset,
	Type:          txType,
	Qty:           fill.Qty,
	Price:         fill.Price,
	Amount:        amount,
	Justification: order.Justification,
	LotSelection:  LotSelection(order.LotSelection),
	BatchID:       order.BatchID,
})
```

- [ ] **Step 4: Carry BatchID through deferred bracket exits**

Edit `portfolio/account.go:2000-2009`, add a field to `deferredExitInfo`:

```go
type deferredExitInfo struct {
	groupID   string
	spec      OrderGroupSpec
	entrySide broker.Side
	fillPrice float64
	asset     asset.Asset
	qty       float64
	batchID   int // NEW — inherited from the bracket entry order.
}
```

Then update the place where `deferredExitInfo` is built at `account.go:2066-2073`:

```go
pendingExits = append(pendingExits, deferredExitInfo{
	groupID:   order.GroupID,
	spec:      spec,
	entrySide: order.Side,
	fillPrice: fill.Price,
	asset:     order.Asset,
	qty:       fill.Qty,
	batchID:   order.BatchID, // NEW
})
```

Finally, in `submitBracketExits` (`portfolio/account.go:2129-2180`), set `BatchID` on both exit orders:

```go
stopLossOrder := broker.Order{
	ID:          exitGroupID + "-sl",
	Asset:       info.asset,
	Side:        exitSide,
	Qty:         info.qty,
	OrderType:   broker.Stop,
	StopPrice:   stopPrice,
	TimeInForce: broker.GTC,
	GroupID:     exitGroupID,
	GroupRole:   broker.RoleStopLoss,
	BatchID:     info.batchID, // NEW
}

takeProfitOrder := broker.Order{
	ID:          exitGroupID + "-tp",
	Asset:       info.asset,
	Side:        exitSide,
	Qty:         info.qty,
	OrderType:   broker.Limit,
	LimitPrice:  takeProfitPrice,
	TimeInForce: broker.GTC,
	GroupID:     exitGroupID,
	GroupRole:   broker.RoleTakeProfit,
	BatchID:     info.batchID, // NEW
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `ginkgo run ./portfolio -focus "copies order.BatchID onto the recorded transaction"`
Expected: PASS.

- [ ] **Step 6: Run the full portfolio + engine suites**

Run: `ginkgo run -race ./portfolio ./engine ./engine/middleware/...`
Expected: all tests pass — bracket/OCO flows continue to work.

- [ ] **Step 7: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go
git commit -m "feat(portfolio): propagate BatchID through fills and bracket exits"
```

---

### Task 7: Bump schema version and expand `createSchema`

**Files:**
- Modify: `portfolio/sqlite.go:31` (schemaVersion)
- Modify: `portfolio/sqlite.go:35-94` (createSchema)
- Modify: `portfolio/sqlite.go:459-461` (doc comment)

- [ ] **Step 1: Bump the schema version constant**

At `portfolio/sqlite.go:31`:

```go
const schemaVersion = "4"
```

- [ ] **Step 2: Expand `createSchema`**

Replace the `createSchema` DDL block starting at `portfolio/sqlite.go:35`. The two tables that grow a column are `transactions` and `annotations`; a new `batches` table is added. Unchanged tables stay exactly as they are — only the three blocks below change:

```sql
CREATE TABLE batches (
    batch_id  INTEGER PRIMARY KEY,
    timestamp INTEGER NOT NULL
);

CREATE INDEX idx_batches_timestamp ON batches(timestamp);

CREATE TABLE transactions (
    batch_id      INTEGER NOT NULL DEFAULT 0,
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

CREATE INDEX idx_transactions_batch ON transactions(batch_id);
CREATE INDEX idx_transactions_date ON transactions(date);

CREATE TABLE annotations (
    batch_id  INTEGER NOT NULL DEFAULT 0,
    timestamp INTEGER NOT NULL,
    key       TEXT NOT NULL,
    value     TEXT NOT NULL
);

CREATE INDEX idx_annotations_batch ON annotations(batch_id);
CREATE INDEX idx_annotations_timestamp ON annotations(timestamp);
```

Keep the existing `idx_transactions_date` creation and the existing `idx_annotations_timestamp` creation — if they were redundantly declared elsewhere in `createSchema`, remove the duplicates when you paste these blocks in.

- [ ] **Step 3: Update the FromSQLite doc comment**

At `portfolio/sqlite.go:459-461`, replace `schema_version "3"` with `schema_version "4"`.

- [ ] **Step 4: Verify compilation and failing tests**

Run: `go build ./portfolio/... && ginkgo run ./portfolio`
Expected: build succeeds; many tests fail because the read path has not yet been updated and the DB no longer matches the Go structs. That is fine — the next tasks wire up reads/writes.

- [ ] **Step 5: Commit**

```bash
git add portfolio/sqlite.go
git commit -m "feat(portfolio): schema v4 adds batches table and batch_id columns"
```

---

### Task 8: Write and read the `batches` table

**Files:**
- Modify: `portfolio/sqlite.go:170-215` (ToSQLite call order)
- Modify: `portfolio/sqlite.go` near other `write*` helpers — add `writeBatches`
- Modify: `portfolio/sqlite.go` near other `read*` helpers — add `readBatches`
- Modify: `portfolio/sqlite.go:520-545` (FromSQLite call order)

- [ ] **Step 1: Write a failing round-trip test**

Create `portfolio/batch_history_test.go`:

```go
package portfolio_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("batch history round-trip", func() {
	It("persists and restores batches with their timestamps", func() {
		ctx := context.Background()
		acct := newTestAccount()
		broker := newTestBroker(acct)
		acct.SetBroker(broker)

		ts1 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		ts2 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

		b1 := portfolio.NewBatch(ts1, acct)
		Expect(b1.Order(ctx, testAssetSPY, portfolio.Buy, 10)).To(Succeed())
		Expect(acct.ExecuteBatch(ctx, b1)).To(Succeed())

		b2 := portfolio.NewBatch(ts2, acct)
		Expect(acct.ExecuteBatch(ctx, b2)).To(Succeed())

		tmp := filepath.Join(GinkgoT().TempDir(), "out.db")
		Expect(acct.ToSQLite(tmp)).To(Succeed())
		defer os.Remove(tmp)

		restored, err := portfolio.FromSQLite(tmp)
		Expect(err).NotTo(HaveOccurred())

		batches := portfolio.GetAccountBatches(restored)
		Expect(batches).To(HaveLen(2))
		Expect(batches[0].BatchID).To(Equal(1))
		Expect(batches[0].Timestamp.UTC()).To(Equal(ts1))
		Expect(batches[1].BatchID).To(Equal(2))
		Expect(batches[1].Timestamp.UTC()).To(Equal(ts2))
	})
})
```

- [ ] **Step 2: Run it to verify failure**

Run: `ginkgo run ./portfolio -focus "persists and restores batches"`
Expected: FAIL — `FromSQLite` does not know about the `batches` table.

- [ ] **Step 3: Add `writeBatches`**

In `portfolio/sqlite.go`, near the other `write*` helpers, add:

```go
func (a *Account) writeBatches(tx *sql.Tx) error {
	if len(a.batches) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO batches (batch_id, timestamp) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare batches: %w", err)
	}
	defer stmt.Close()

	for _, rec := range a.batches {
		if _, err := stmt.Exec(rec.BatchID, rec.Timestamp.UnixNano()); err != nil {
			return fmt.Errorf("insert batch: %w", err)
		}
	}

	return nil
}
```

- [ ] **Step 4: Add `readBatches`**

Also in `portfolio/sqlite.go`:

```go
func (a *Account) readBatches(db *sql.DB) error {
	rows, err := db.Query("SELECT batch_id, timestamp FROM batches ORDER BY batch_id")
	if err != nil {
		return fmt.Errorf("query batches: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var rec batchRecord

		var nanos int64

		if err := rows.Scan(&rec.BatchID, &nanos); err != nil {
			return fmt.Errorf("scan batch: %w", err)
		}

		rec.Timestamp = time.Unix(0, nanos).UTC()
		a.batches = append(a.batches, rec)
	}

	return rows.Err()
}
```

- [ ] **Step 5: Wire up writeBatches in ToSQLite**

In `ToSQLite` (`portfolio/sqlite.go:152-214`), add a call to `writeBatches` immediately after `writeTransactions`:

```go
if err := a.writeBatches(dbTx); err != nil {
	return err
}
```

- [ ] **Step 6: Wire up readBatches in FromSQLite**

In `FromSQLite` (around `portfolio/sqlite.go:520-545`), add a call to `readBatches` immediately after `readTransactions`:

```go
if err := acct.readBatches(database); err != nil {
	return nil, err
}
```

- [ ] **Step 7: Run the test to verify success**

Run: `ginkgo run ./portfolio -focus "persists and restores batches"`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add portfolio/sqlite.go portfolio/batch_history_test.go
git commit -m "feat(portfolio): round-trip batches table in SQLite snapshot"
```

---

### Task 9: Round-trip `batch_id` on transactions

**Files:**
- Modify: `portfolio/sqlite.go:298-324` (writeTransactions)
- Modify: `portfolio/sqlite.go:669-721` (readTransactions)

- [ ] **Step 1: Extend the round-trip test**

Append to the `Describe("batch history round-trip", ...)` block in `portfolio/batch_history_test.go`:

```go
It("persists and restores transaction BatchID", func() {
	ctx := context.Background()
	acct := newTestAccount()
	broker := newTestBroker(acct)
	acct.SetBroker(broker)

	b1 := portfolio.NewBatch(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), acct)
	Expect(b1.Order(ctx, testAssetSPY, portfolio.Buy, 10)).To(Succeed())
	Expect(acct.ExecuteBatch(ctx, b1)).To(Succeed())

	tmp := filepath.Join(GinkgoT().TempDir(), "out.db")
	Expect(acct.ToSQLite(tmp)).To(Succeed())
	defer os.Remove(tmp)

	restored, err := portfolio.FromSQLite(tmp)
	Expect(err).NotTo(HaveOccurred())

	var seenBuy bool
	for _, txn := range restored.Transactions() {
		if txn.Type == asset.BuyTransaction {
			Expect(txn.BatchID).To(Equal(1))
			seenBuy = true
		}
	}
	Expect(seenBuy).To(BeTrue())
})
```

- [ ] **Step 2: Run it to verify failure**

Run: `ginkgo run ./portfolio -focus "persists and restores transaction BatchID"`
Expected: FAIL — BatchID is 0 on the restored transaction.

- [ ] **Step 3: Update writeTransactions**

Replace the prepared statement and its Exec call in `portfolio/sqlite.go:298-324`:

```go
func (a *Account) writeTransactions(tx *sql.Tx) error {
	if len(a.transactions) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO transactions (batch_id, date, type, ticker, figi, quantity, price, amount, qualified, justification) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare transactions: %w", err)
	}
	defer stmt.Close()

	for _, txn := range a.transactions {
		dateStr := txn.Date.Format(dateFormat)
		typStr := transactionTypeToString(txn.Type)

		qualified := 0
		if txn.Qualified {
			qualified = 1
		}

		if _, err := stmt.Exec(
			txn.BatchID,
			dateStr, typStr,
			txn.Asset.Ticker, txn.Asset.CompositeFigi,
			txn.Qty, txn.Price, txn.Amount,
			qualified,
			sql.NullString{String: txn.Justification, Valid: txn.Justification != ""},
		); err != nil {
			return fmt.Errorf("insert transaction: %w", err)
		}
	}

	return nil
}
```

- [ ] **Step 4: Update readTransactions**

In `portfolio/sqlite.go:669-721`, change the SELECT to include `batch_id` first and scan it into `txn.BatchID`:

```go
rows, err := db.Query("SELECT batch_id, date, type, ticker, figi, quantity, price, amount, qualified, justification FROM transactions ORDER BY batch_id, date")
```

Add `var batchID int` to the scan locals and pass it as the first scan destination; then set `txn.BatchID = batchID`.

- [ ] **Step 5: Run the test to verify success**

Run: `ginkgo run ./portfolio -focus "persists and restores transaction BatchID"`
Expected: PASS.

- [ ] **Step 6: Run the full portfolio suite**

Run: `ginkgo run -race ./portfolio`
Expected: PASS — existing sqlite round-trip tests still work because `batch_id` defaults to 0 for transactions that never ran through a batch.

- [ ] **Step 7: Commit**

```bash
git add portfolio/sqlite.go portfolio/batch_history_test.go
git commit -m "feat(portfolio): round-trip transaction batch_id in SQLite"
```

---

### Task 10: Round-trip `batch_id` on annotations

**Files:**
- Modify: `portfolio/sqlite.go:416-457` (writeAnnotations + readAnnotations)

- [ ] **Step 1: Extend the round-trip test**

Append to `portfolio/batch_history_test.go`:

```go
It("persists and restores annotation BatchID", func() {
	ctx := context.Background()
	acct := newTestAccount()
	broker := newTestBroker(acct)
	acct.SetBroker(broker)

	b1 := portfolio.NewBatch(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), acct)
	b1.Annotate("score", "0.42")
	Expect(acct.ExecuteBatch(ctx, b1)).To(Succeed())

	tmp := filepath.Join(GinkgoT().TempDir(), "out.db")
	Expect(acct.ToSQLite(tmp)).To(Succeed())
	defer os.Remove(tmp)

	restored, err := portfolio.FromSQLite(tmp)
	Expect(err).NotTo(HaveOccurred())

	anns := restored.Annotations()
	Expect(anns).To(HaveLen(1))
	Expect(anns[0].Key).To(Equal("score"))
	Expect(anns[0].BatchID).To(Equal(1))
})
```

- [ ] **Step 2: Run it to verify failure**

Run: `ginkgo run ./portfolio -focus "persists and restores annotation BatchID"`
Expected: FAIL.

- [ ] **Step 3: Update writeAnnotations**

Replace the body of `writeAnnotations` (`portfolio/sqlite.go:416-434`):

```go
func (a *Account) writeAnnotations(tx *sql.Tx) error {
	if len(a.annotations) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO annotations (batch_id, timestamp, key, value) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare annotations: %w", err)
	}
	defer stmt.Close()

	for _, ann := range a.annotations {
		if _, err := stmt.Exec(ann.BatchID, ann.Timestamp.UnixNano(), ann.Key, ann.Value); err != nil {
			return fmt.Errorf("insert annotation: %w", err)
		}
	}

	return nil
}
```

Note: this also upgrades the stored timestamp from `Unix()` (seconds) to `UnixNano()` so ExecuteBatch timestamps round-trip without truncation. That matches the granularity we already use in `writeBatches`.

- [ ] **Step 4: Update readAnnotations**

Replace the body of `readAnnotations` (`portfolio/sqlite.go:436-457`):

```go
func (a *Account) readAnnotations(db *sql.DB) error {
	rows, err := db.Query("SELECT batch_id, timestamp, key, value FROM annotations ORDER BY batch_id, timestamp, key")
	if err != nil {
		return fmt.Errorf("query annotations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			nanos int64
			ann   Annotation
		)

		if err := rows.Scan(&ann.BatchID, &nanos, &ann.Key, &ann.Value); err != nil {
			return fmt.Errorf("scan annotation: %w", err)
		}

		ann.Timestamp = time.Unix(0, nanos).UTC()
		a.annotations = append(a.annotations, ann)
	}

	return rows.Err()
}
```

- [ ] **Step 5: Run the test to verify success**

Run: `ginkgo run ./portfolio -focus "persists and restores annotation BatchID"`
Expected: PASS.

- [ ] **Step 6: Run the full portfolio suite**

Run: `ginkgo run -race ./portfolio`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add portfolio/sqlite.go portfolio/batch_history_test.go
git commit -m "feat(portfolio): round-trip annotation batch_id and nanosecond timestamp"
```

---

### Task 11: Verify the full-stack query for holdings-after-batch-N

**Files:**
- Test: `portfolio/batch_history_test.go`

- [ ] **Step 1: Write the verification test**

Append to `portfolio/batch_history_test.go`:

```go
It("supports reconstructing holdings after each batch via SQL replay", func() {
	ctx := context.Background()
	acct := newTestAccount()
	broker := newTestBroker(acct)
	acct.SetBroker(broker)

	// Batch 1: buy 10 SPY.
	b1 := portfolio.NewBatch(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), acct)
	Expect(b1.Order(ctx, testAssetSPY, portfolio.Buy, 10)).To(Succeed())
	Expect(acct.ExecuteBatch(ctx, b1)).To(Succeed())

	// Batch 2: sell 4 SPY.
	b2 := portfolio.NewBatch(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), acct)
	Expect(b2.Order(ctx, testAssetSPY, portfolio.Sell, 4)).To(Succeed())
	Expect(acct.ExecuteBatch(ctx, b2)).To(Succeed())

	tmp := filepath.Join(GinkgoT().TempDir(), "out.db")
	Expect(acct.ToSQLite(tmp)).To(Succeed())
	defer os.Remove(tmp)

	db, err := sql.Open("sqlite", tmp)
	Expect(err).NotTo(HaveOccurred())
	defer db.Close()

	query := `
		SELECT ticker,
		       SUM(CASE type WHEN 'buy' THEN quantity
		                     WHEN 'sell' THEN -quantity
		                     WHEN 'split' THEN quantity ELSE 0 END) AS qty
		FROM transactions
		WHERE batch_id > 0 AND batch_id <= ?
		GROUP BY ticker
		HAVING qty != 0`

	checkQty := func(n int, expected float64) {
		rows, err := db.Query(query, n)
		Expect(err).NotTo(HaveOccurred())
		defer rows.Close()

		Expect(rows.Next()).To(BeTrue())
		var (
			ticker string
			qty    float64
		)
		Expect(rows.Scan(&ticker, &qty)).To(Succeed())
		Expect(ticker).To(Equal("SPY"))
		Expect(qty).To(BeNumerically("~", expected, 1e-9))
	}

	checkQty(1, 10)
	checkQty(2, 6)
})
```

- [ ] **Step 2: Run the test**

Run: `ginkgo run ./portfolio -focus "reconstructing holdings after each batch"`
Expected: PASS first try (no production code change).

- [ ] **Step 3: Commit**

```bash
git add portfolio/batch_history_test.go
git commit -m "test(portfolio): verify holdings-per-batch SQL replay"
```

---

### Task 12: Update changelog

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add entry**

Under the `[Unreleased]` section of `CHANGELOG.md`, add an `Added` bullet (create the section header if absent):

```markdown
### Added
- Snapshot files now record a monotonic batch id on every transaction and annotation, and include a new `batches` table so tools can reconstruct the portfolio's holdings after each batch.
```

And under `### Changed`:

```markdown
### Changed
- Snapshot schema version bumped to `4`. Earlier snapshots are not read by this release; re-run the backtest to produce a v4 file.
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog for per-batch snapshot history"
```

---

## Self-Review

**1. Spec coverage:**
- BatchID on `broker.Order` — Task 1.
- BatchID on `Transaction` and `Annotation` — Task 2.
- Account-level batch state — Task 3.
- Annotate stamping — Task 4.
- ExecuteBatch id assignment + order stamping — Task 5.
- Fill path + bracket-exit propagation — Task 6.
- Schema version bump + `batches`/`transactions`/`annotations` DDL — Task 7.
- `writeBatches`/`readBatches` — Task 8.
- `writeTransactions`/`readTransactions` batch_id — Task 9.
- `writeAnnotations`/`readAnnotations` batch_id — Task 10.
- End-to-end SQL replay demonstration — Task 11.
- User-visible release note — Task 12.

**2. Placeholders:** none — every code block is concrete.

**3. Type consistency:** `batchRecord{BatchID int, Timestamp time.Time}` referenced identically in Tasks 3, 5, 8. `deferredExitInfo.batchID` (lowercase — it is unexported) referenced identically in Task 6. `currentBatchID int` referenced in Tasks 3, 4, 5. Helper names `SetAccountCurrentBatchID`, `GetAccountBatches`, `GetAccountCurrentBatchID` used consistently across Tasks 4 and 5.

**Known limitation** (documented, not fixed): non-batch transactions (deposits via `WithCash`, splits via `ApplySplit`, broker-synced transactions) keep `BatchID = 0`. The holdings-after-batch-N query in Task 11 therefore reconstructs the *batch-caused* holdings only — it ignores deposits/splits. This matches the user's stated goal ("one set of holdings per batch") and keeps the plan small. A follow-on change could attribute split adjustments to the currently-active batch by calling through `Record` instead of direct append.
