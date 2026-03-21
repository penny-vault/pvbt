# Tax Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tax loss harvesting middleware, wash sale detection, configurable lot selection, substitute asset mapping, and tax drag metric to pvbt.

**Architecture:** Two-layer design. The portfolio layer (always-on) handles wash sale tracking, configurable lot selection, and substitution mapping behind the `Portfolio` and `TaxAware` interfaces. A `TaxLossHarvester` middleware in a new `tax` package actively harvests losses. Tax drag is a new metric on `TaxMetrics`.

**Tech Stack:** Go, Ginkgo/Gomega for tests, existing `portfolio.Middleware` interface.

**Spec:** `docs/superpowers/specs/2026-03-20-tax-optimization-design.md`

---

### Task 1: LotSelection Type and WithLotSelection OrderModifier

Add the `LotSelection` type, constants, and an `OrderModifier` so orders can specify which lot selection method to use.

**Files:**
- Create: `portfolio/lot_selection.go`
- Modify: `portfolio/order.go:113` (add new modifier after `WithJustification`)
- Modify: `portfolio/batch.go:113-114` (add case to modifier switch)
- Modify: `portfolio/account.go:240-242` (add case to modifier switch)
- Modify: `broker/broker.go:86-99` (add `LotSelection` field to `Order`)
- Create: `portfolio/lot_selection_test.go`

- [ ] **Step 1: Write failing tests for LotSelection type and modifier**

Create `portfolio/lot_selection_test.go` with Ginkgo tests:

```go
package portfolio_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("LotSelection", func() {
	It("has four lot selection methods", func() {
		Expect(portfolio.LotFIFO).To(BeNumerically(">=", 0))
		Expect(portfolio.LotLIFO).To(BeNumerically(">=", 0))
		Expect(portfolio.LotHighestCost).To(BeNumerically(">=", 0))
		Expect(portfolio.LotSpecificID).To(BeNumerically(">=", 0))
	})

	It("defaults to FIFO", func() {
		Expect(portfolio.LotFIFO).To(Equal(portfolio.LotSelection(0)))
	})
})

var _ = Describe("WithLotSelection modifier", func() {
	It("sets LotSelection on broker.Order via batch.Order()", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		timestamp := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

		df, err := data.NewDataFrame(
			[]time.Time{timestamp},
			[]asset.Asset{spy},
			map[data.Metric][]float64{
				data.AdjustedClose: {150.0},
				data.Close:         {150.0},
			},
		)
		Expect(err).ToNot(HaveOccurred())
		acct.UpdatePrices(df, timestamp)

		// Record a buy so we have something to sell.
		acct.Record(portfolio.Transaction{
			Date:   timestamp,
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    10,
			Price:  150.0,
			Amount: -1500.0,
		})

		batch := acct.NewBatch(timestamp)
		err = batch.Order(context.Background(), spy, portfolio.Sell, 5,
			portfolio.WithLotSelection(portfolio.LotHighestCost))
		Expect(err).ToNot(HaveOccurred())
		Expect(batch.Orders).To(HaveLen(1))
		Expect(batch.Orders[0].LotSelection).To(Equal(int(portfolio.LotHighestCost)))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "LotSelection" -v`
Expected: compilation errors (types not defined)

- [ ] **Step 3: Create lot_selection.go with type and constants**

Create `portfolio/lot_selection.go`:

```go
package portfolio

// LotSelection determines which tax lots are consumed when selling a position.
type LotSelection int

const (
	// LotFIFO sells the earliest-acquired lots first (default).
	LotFIFO LotSelection = iota
	// LotLIFO sells the most-recently-acquired lots first.
	LotLIFO
	// LotHighestCost sells the lot with the highest cost basis first,
	// producing the largest realized loss when the position is underwater.
	LotHighestCost
	// LotSpecificID sells a specific lot identified by ID.
	LotSpecificID
)
```

- [ ] **Step 4: Add LotSelection field to broker.Order**

In `broker/broker.go`, add a `LotSelection` field to the `Order` struct after the `Justification` field (line 98). Use `int` type to avoid a circular import (broker should not import portfolio). Add a comment explaining the mapping.

```go
// LotSelection controls which tax lots are consumed on a sell.
// 0=FIFO (default), 1=LIFO, 2=HighestCost, 3=SpecificID.
// Set via portfolio.WithLotSelection order modifier.
LotSelection int
```

- [ ] **Step 5: Add WithLotSelection OrderModifier**

In `portfolio/order.go`, after the `WithJustification` block (line 113), add:

```go
type lotSelectionModifier struct{ method LotSelection }

func (lotSelectionModifier) orderModifier() {}

// WithLotSelection overrides the portfolio default lot selection for this order.
func WithLotSelection(method LotSelection) OrderModifier {
	return lotSelectionModifier{method: method}
}
```

- [ ] **Step 6: Handle lotSelectionModifier in batch.Order() and account.Order()**

In `portfolio/batch.go`, add a case to the modifier switch (after the `justificationModifier` case at line 114):

```go
case lotSelectionModifier:
	order.LotSelection = int(modifier.method)
```

In `portfolio/account.go`, add the same case to the modifier switch (after the `justificationModifier` case at line 240-241). Set it directly on the `order` struct: `order.LotSelection = int(modifier.method)`. The `LotSelection` flows through `submitAndRecord` on the `broker.Order` struct and is available when `Record()` processes the fill.

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "LotSelection" -v`
Expected: PASS

- [ ] **Step 8: Commit**

```
git add portfolio/lot_selection.go portfolio/lot_selection_test.go portfolio/order.go portfolio/batch.go portfolio/account.go broker/broker.go
git commit -m "feat: add LotSelection type and WithLotSelection order modifier"
```

---

### Task 2: Configurable Lot Selection in Account

Wire up the lot selection logic so the account's sell path uses the configured method instead of hardcoded FIFO.

**Files:**
- Modify: `portfolio/lot_selection.go` (add sort helpers)
- Modify: `portfolio/account.go:63-80` (add lotSelection field and WithLotSelection option)
- Modify: `portfolio/account.go:621-642` (replace FIFO sell logic with method dispatch)
- Modify: `portfolio/transaction.go` (add LotSelection field to Transaction for per-order override propagation)
- Modify: `portfolio/snapshot.go:27-31` (add ID field to TaxLot)
- Create: `portfolio/lot_selection_account_test.go`

- [ ] **Step 1: Write failing tests for LIFO and HighestCost lot consumption**

Create `portfolio/lot_selection_account_test.go` with Ginkgo tests covering:

1. **FIFO (default)** -- buy at $100 then $200, sell some, verify $100 lot consumed first
2. **LIFO** -- buy at $100 then $200, sell some, verify $200 lot consumed first
3. **HighestCost** -- buy at $100, $300, $200, sell some, verify $300 lot consumed first
4. **Account-level default** -- set via `WithLotSelection(LotLIFO)`, all sells use LIFO
5. **Per-order override** -- account default is FIFO, but order with `WithLotSelection(LotHighestCost)` uses HighestCost

Each test should: create account, record multiple buys at different prices, record a sell, then check `acct.TaxLots()` to verify the correct lots remain.

```go
package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Lot selection in sell path", func() {
	var (
		spy asset.Asset
		t1  time.Time
		t2  time.Time
		t3  time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		t1 = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
		t3 = time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	})

	buyLots := func(acct *portfolio.Account) {
		acct.Record(portfolio.Transaction{Date: t1, Asset: spy, Type: portfolio.BuyTransaction, Qty: 10, Price: 100, Amount: -1000})
		acct.Record(portfolio.Transaction{Date: t2, Asset: spy, Type: portfolio.BuyTransaction, Qty: 10, Price: 300, Amount: -3000})
		acct.Record(portfolio.Transaction{Date: t3, Asset: spy, Type: portfolio.BuyTransaction, Qty: 10, Price: 200, Amount: -2000})
	}

	It("uses FIFO by default", func() {
		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
		buyLots(acct)
		acct.Record(portfolio.Transaction{Date: t3, Asset: spy, Type: portfolio.SellTransaction, Qty: 10, Price: 250, Amount: 2500})
		lots := acct.TaxLots()[spy]
		Expect(lots).To(HaveLen(2))
		Expect(lots[0].Price).To(Equal(300.0)) // $100 lot consumed
	})

	It("uses LIFO when configured", func() {
		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithDefaultLotSelection(portfolio.LotLIFO))
		buyLots(acct)
		acct.Record(portfolio.Transaction{Date: t3, Asset: spy, Type: portfolio.SellTransaction, Qty: 10, Price: 250, Amount: 2500})
		lots := acct.TaxLots()[spy]
		Expect(lots).To(HaveLen(2))
		Expect(lots[0].Price).To(Equal(100.0)) // $200 lot consumed (last in)
		Expect(lots[1].Price).To(Equal(300.0))
	})

	It("uses HighestCost when configured", func() {
		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}), portfolio.WithDefaultLotSelection(portfolio.LotHighestCost))
		buyLots(acct)
		acct.Record(portfolio.Transaction{Date: t3, Asset: spy, Type: portfolio.SellTransaction, Qty: 10, Price: 250, Amount: 2500})
		lots := acct.TaxLots()[spy]
		Expect(lots).To(HaveLen(2))
		Expect(lots[0].Price).To(Equal(100.0)) // $300 lot consumed (highest cost)
		Expect(lots[1].Price).To(Equal(200.0))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Lot selection in sell path" -v`
Expected: compilation errors

- [ ] **Step 3: Add ID field to TaxLot**

In `portfolio/snapshot.go`, add an `ID` field to `TaxLot` (line 28):

```go
type TaxLot struct {
	ID    string
	Date  time.Time
	Qty   float64
	Price float64
}
```

- [ ] **Step 4: Add lotSelection field and WithDefaultLotSelection option to Account**

In `portfolio/account.go`, add a `lotSelection LotSelection` field to the `Account` struct. Add a `WithDefaultLotSelection` option function:

```go
// WithDefaultLotSelection sets the lot selection method for all sells.
// The default is LotFIFO.
func WithDefaultLotSelection(method LotSelection) Option {
	return func(acct *Account) {
		acct.lotSelection = method
	}
}
```

- [ ] **Step 5: Add lot ID generation on buy**

In `portfolio/account.go`, in the `BuyTransaction` case of `Record()` (lines 614-620), generate a unique ID for each new tax lot:

```go
case BuyTransaction:
	a.holdings[txn.Asset] += txn.Qty
	a.taxLots[txn.Asset] = append(a.taxLots[txn.Asset], TaxLot{
		ID:    fmt.Sprintf("lot-%d-%d", txn.Date.UnixNano(), len(a.taxLots[txn.Asset])),
		Date:  txn.Date,
		Qty:   txn.Qty,
		Price: txn.Price,
	})
```

- [ ] **Step 6: Replace FIFO sell logic with lot selection dispatch**

In `portfolio/account.go`, replace the sell lot consumption logic (lines 621-637) with a method that selects lots based on the lot selection method. Add a helper `consumeLots(ast asset.Asset, qty float64, method LotSelection)` that:

- `LotFIFO`: consumes from front of slice (current behavior)
- `LotLIFO`: consumes from back of slice
- `LotHighestCost`: sorts by price descending, consumes highest first, then reorders remaining lots by date

The `Record()` sell path should determine the lot selection method by checking the transaction's `LotSelection` field for a per-order override; if zero (the default), use the account's `lotSelection` default.

**LotSelection propagation path:** `broker.Order.LotSelection` (set by `WithLotSelection` modifier in `batch.Order()`) -> `Transaction.LotSelection` (set in `submitAndRecord()` when creating the `SellTransaction` from the fill, around line 294) -> `Record()` reads `txn.LotSelection` to dispatch lot consumption.

Add a `LotSelection int` field to the `Transaction` struct in `portfolio/transaction.go`. In `submitAndRecord()` (around line 294 in `account.go`), copy `order.LotSelection` to the transaction. In `Record()`, use `txn.LotSelection` if non-zero, otherwise fall back to `a.lotSelection`.

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Lot selection in sell path" -v`
Expected: PASS

- [ ] **Step 8: Run full portfolio test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v`
Expected: all existing tests still pass (FIFO is the default, so behavior is unchanged)

- [ ] **Step 9: Commit**

```
git add portfolio/lot_selection.go portfolio/lot_selection_account_test.go portfolio/account.go portfolio/snapshot.go
git commit -m "feat: configurable lot selection method for sell path (FIFO, LIFO, HighestCost)"
```

---

### Task 3: TaxLot ID in SQLite Persistence

Add the `ID` field to the tax lots SQLite schema so lot IDs survive persistence round-trips.

**Files:**
- Modify: `portfolio/sqlite.go:67-73` (add id column to tax_lots table)
- Modify: `portfolio/sqlite.go:354-375` (write ID in writeTaxLots)
- Modify: `portfolio/sqlite.go:readTaxLots` (read ID field)
- Modify: `portfolio/sqlite_test.go` (add round-trip test for lot ID)

- [ ] **Step 1: Write failing test for lot ID persistence**

Add a test in `portfolio/sqlite_test.go` that creates an account, records a buy (generating a lot with an ID), persists to SQLite, restores, and verifies the lot ID matches.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "lot ID" -v`
Expected: FAIL (ID not persisted)

- [ ] **Step 3: Update SQLite schema and read/write functions**

In `portfolio/sqlite.go`:
- Add `id TEXT` column to the `tax_lots` CREATE TABLE statement
- In `writeTaxLots`, include the ID in the INSERT statement
- In `readTaxLots`, scan the ID field

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "lot ID" -v`
Expected: PASS

- [ ] **Step 5: Run full SQLite test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "SQLite" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```
git add portfolio/sqlite.go portfolio/sqlite_test.go
git commit -m "feat: persist TaxLot ID field in SQLite"
```

---

### Task 4: Wash Sale Detection in Record()

Add always-on wash sale detection to `Record()`. Both directions: buy-after-loss-sale and loss-sale-after-buy within 30 calendar days.

**Files:**
- Create: `portfolio/wash_sale.go`
- Create: `portfolio/wash_sale_test.go`
- Modify: `portfolio/account.go:605-643` (add wash sale checks to Record)
- Modify: `portfolio/account.go` (add wash sale tracking fields to Account struct)

- [ ] **Step 1: Write failing tests for wash sale detection**

Create `portfolio/wash_sale_test.go` with Ginkgo tests:

1. **Buy after loss sale within 30 days** -- sell at loss, buy within 30 days, verify disallowed loss added to new lot's cost basis
2. **Buy after loss sale beyond 30 days** -- sell at loss, buy after 31 days, verify no wash sale
3. **Loss sale after buy within 30 days** -- buy, then sell at loss within 30 days, verify wash sale detected and basis adjusted on the recent buy's lot
4. **No wash sale on gain** -- sell at a gain, buy within 30 days, verify no wash sale
5. **Partial wash sale** -- sell 10 shares at loss, buy 5 within 30 days, verify only 5 shares' worth of loss disallowed
6. **WashSaleRecords accessible** -- verify wash sale records are stored and retrievable

```go
package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Wash sale detection", func() {
	var (
		spy asset.Asset
		t0  time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		t0 = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	})

	It("adjusts basis when buying within 30 days of a loss sale", func() {
		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

		// Buy at $100
		acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: portfolio.BuyTransaction, Qty: 10, Price: 100, Amount: -1000})
		// Sell at $80 (loss of $200)
		sellDate := t0.AddDate(0, 0, 60)
		acct.Record(portfolio.Transaction{Date: sellDate, Asset: spy, Type: portfolio.SellTransaction, Qty: 10, Price: 80, Amount: 800})
		// Buy back within 30 days
		rebuyDate := sellDate.AddDate(0, 0, 15)
		acct.Record(portfolio.Transaction{Date: rebuyDate, Asset: spy, Type: portfolio.BuyTransaction, Qty: 10, Price: 85, Amount: -850})

		// New lot basis should be $85 + $20 (disallowed loss) = $105
		lots := acct.TaxLots()[spy]
		Expect(lots).To(HaveLen(1))
		Expect(lots[0].Price).To(BeNumerically("~", 105.0, 0.01))
	})

	It("does not trigger when rebuy is beyond 30 days", func() {
		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

		acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: portfolio.BuyTransaction, Qty: 10, Price: 100, Amount: -1000})
		sellDate := t0.AddDate(0, 0, 60)
		acct.Record(portfolio.Transaction{Date: sellDate, Asset: spy, Type: portfolio.SellTransaction, Qty: 10, Price: 80, Amount: 800})
		rebuyDate := sellDate.AddDate(0, 0, 31)
		acct.Record(portfolio.Transaction{Date: rebuyDate, Asset: spy, Type: portfolio.BuyTransaction, Qty: 10, Price: 85, Amount: -850})

		lots := acct.TaxLots()[spy]
		Expect(lots).To(HaveLen(1))
		Expect(lots[0].Price).To(Equal(85.0)) // no adjustment
	})

	It("does not trigger on a gain sale", func() {
		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

		acct.Record(portfolio.Transaction{Date: t0, Asset: spy, Type: portfolio.BuyTransaction, Qty: 10, Price: 100, Amount: -1000})
		sellDate := t0.AddDate(0, 0, 60)
		acct.Record(portfolio.Transaction{Date: sellDate, Asset: spy, Type: portfolio.SellTransaction, Qty: 10, Price: 120, Amount: 1200})
		rebuyDate := sellDate.AddDate(0, 0, 15)
		acct.Record(portfolio.Transaction{Date: rebuyDate, Asset: spy, Type: portfolio.BuyTransaction, Qty: 10, Price: 115, Amount: -1150})

		lots := acct.TaxLots()[spy]
		Expect(lots).To(HaveLen(1))
		Expect(lots[0].Price).To(Equal(115.0)) // no adjustment
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Wash sale" -v`
Expected: FAIL

- [ ] **Step 3: Create wash_sale.go with types**

Create `portfolio/wash_sale.go`:

```go
package portfolio

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// WashSaleRecord tracks a wash sale event where a loss was disallowed
// because the same asset was repurchased within 30 calendar days.
type WashSaleRecord struct {
	Asset           asset.Asset
	SellDate        time.Time
	RebuyDate       time.Time
	DisallowedLoss  float64
	AdjustedLotID   string
}

// recentLossSale tracks a loss-generating sell for wash sale window checking.
type recentLossSale struct {
	date       time.Time
	lossPerShare float64
	qty        float64
}

// recentBuy tracks a recent buy for the reverse wash sale direction.
type recentBuy struct {
	date  time.Time
	lotID string
	qty   float64
}

const washSaleWindowDays = 30
```

- [ ] **Step 4: Add wash sale tracking fields to Account**

In `portfolio/account.go`, add to the Account struct:

```go
recentLossSales map[asset.Asset][]recentLossSale
recentBuys      map[asset.Asset][]recentBuy
washSales       []WashSaleRecord
```

Initialize these maps in `New()`.

- [ ] **Step 5: Implement wash sale detection in Record()**

In `portfolio/account.go`, modify `Record()`:

**On buy:** After appending the new tax lot, check `recentLossSales[asset]` for any entries within 30 days. If found, compute the disallowed loss, add it to the new lot's `Price` (cost basis), create a `WashSaleRecord`, and remove the consumed loss sale entry. Also record this buy in `recentBuys[asset]`.

**On sell:** After consuming lots, compute the realized gain/loss. If it's a loss, check `recentBuys[asset]` for any entries within 30 days. If found, it's a wash sale -- adjust the recent buy's lot cost basis and record it. Also record this loss sale in `recentLossSales[asset]`.

**Pruning:** At the start of `Record()`, prune `recentLossSales` and `recentBuys` entries older than 30 days from `txn.Date`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Wash sale" -v`
Expected: PASS

- [ ] **Step 7: Run full portfolio test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v`
Expected: all existing tests pass (wash sale only triggers on specific loss+rebuy patterns)

- [ ] **Step 8: Commit**

```
git add portfolio/wash_sale.go portfolio/wash_sale_test.go portfolio/account.go
git commit -m "feat: always-on wash sale detection with basis adjustment in Record()"
```

---

### Task 5: TaxAware Interface

Define the `TaxAware` interface and implement it on `Account`.

**Files:**
- Create: `portfolio/tax_aware.go`
- Create: `portfolio/tax_aware_test.go`
- Modify: `portfolio/account.go` (add method implementations)

- [ ] **Step 1: Write failing tests for TaxAware methods**

Create `portfolio/tax_aware_test.go` with Ginkgo tests:

1. **WashSaleWindow** -- create account with wash sale history, verify records returned
2. **UnrealizedLots** -- create account with open lots, verify they're returned
3. **RealizedGainsYTD** -- record some sells, verify LTCG/STCG returned correctly
4. **Type assertion** -- verify `Account` satisfies `TaxAware` interface via type assertion

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "TaxAware" -v`
Expected: FAIL

- [ ] **Step 3: Create tax_aware.go with interface definition**

Create `portfolio/tax_aware.go`:

```go
package portfolio

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// TaxAware provides tax-lot-level access for tax optimization middleware.
// The concrete Account type implements this interface. Tax middleware
// type-asserts the batch's Portfolio reference to TaxAware; strategies
// and risk middleware only see Portfolio.
type TaxAware interface {
	WashSaleWindow(ast asset.Asset) []WashSaleRecord
	UnrealizedLots(ast asset.Asset) []TaxLot
	RealizedGainsYTD() (ltcg, stcg float64)
	RegisterSubstitution(original, substitute asset.Asset, until time.Time)
	ActiveSubstitutions() map[asset.Asset]Substitution
}

// Substitution records an active asset substitution for tax purposes.
type Substitution struct {
	Original   asset.Asset
	Substitute asset.Asset
	Until      time.Time
}
```

- [ ] **Step 4: Implement TaxAware methods on Account**

In `portfolio/account.go`, add:

- `WashSaleWindow(ast)` -- returns `a.washSales` filtered to the given asset
- `UnrealizedLots(ast)` -- returns a copy of `a.taxLots[ast]`
- `RealizedGainsYTD()` -- calls `realizedGains(a.Transactions())` filtering to current year, returns ltcg and stcg

Leave `RegisterSubstitution` and `ActiveSubstitutions` as stubs returning nil/empty for now (implemented in Task 7).

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "TaxAware" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```
git add portfolio/tax_aware.go portfolio/tax_aware_test.go portfolio/account.go
git commit -m "feat: add TaxAware interface with WashSaleWindow, UnrealizedLots, RealizedGainsYTD"
```

---

### Task 6: Tax Drag Metric

Add `TaxDrag` field to `TaxMetrics` and implement the metric computation.

**Files:**
- Create: `portfolio/tax_drag.go`
- Modify: `portfolio/tax_metrics.go:21-29` (add TaxDrag field)
- Modify: `portfolio/account.go` (add TaxDrag to TaxMetrics() computation -- find the method that populates TaxMetrics)
- Create: `portfolio/tax_drag_test.go`

- [ ] **Step 1: Write failing tests for tax drag metric**

Create `portfolio/tax_drag_test.go` with Ginkgo tests:

1. **Tax drag from short-term gains** -- strategy with high turnover (all STCG), verify TaxDrag = 0.25 * STCG / PreTaxReturn
2. **Tax drag from long-term gains** -- low turnover (all LTCG), verify TaxDrag = 0.15 * LTCG / PreTaxReturn
3. **Tax drag excludes dividends** -- portfolio with dividends but no trading, verify TaxDrag is 0
4. **Tax drag with no gain** -- portfolio with no pre-tax return, verify TaxDrag is 0
5. **Tax drag with losses** -- realized losses only, verify TaxDrag is 0 (no tax on losses)

Follow the same pattern as `portfolio/tax_cost_ratio.go` for the metric implementation and as `portfolio/tax_metrics_test.go` for test structure.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "TaxDrag" -v`
Expected: FAIL

- [ ] **Step 3: Add TaxDrag field to TaxMetrics**

In `portfolio/tax_metrics.go`, add:

```go
TaxDrag float64 // percentage of pre-tax return lost to trading-related taxes (excludes dividends)
```

- [ ] **Step 4: Create tax_drag.go metric**

Create `portfolio/tax_drag.go` following the pattern in `tax_cost_ratio.go`:

```go
package portfolio

import "github.com/penny-vault/pvbt/data"

type taxDrag struct{}

func (taxDrag) Name() string { return "TaxDrag" }

func (taxDrag) Description() string {
	return "Percentage of pre-tax return consumed by taxes from trading activity, excluding dividend taxation. Uses 25% for short-term gains and 15% for long-term gains."
}

func (taxDrag) Compute(acct *Account, _ *Period) (float64, error) {
	perfData := acct.PerfData()
	if perfData == nil {
		return 0, nil
	}

	ec := perfData.Column(portfolioAsset, data.PortfolioEquity)
	if len(ec) < 2 {
		return 0, nil
	}

	preTaxReturn := ec[len(ec)-1] - ec[0]
	if preTaxReturn <= 0 {
		return 0, nil
	}

	ltcg, stcg, _, _ := realizedGains(acct.Transactions())

	estimatedTax := 0.0
	if stcg > 0 {
		estimatedTax += 0.25 * stcg
	}
	if ltcg > 0 {
		estimatedTax += 0.15 * ltcg
	}

	return estimatedTax / preTaxReturn, nil
}

func (taxDrag) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// TaxDragMetric measures the percentage of return lost to trading-related taxes.
var TaxDragMetric PerformanceMetric = taxDrag{}
```

- [ ] **Step 5: Register the metric and wire into TaxMetrics()**

Register `TaxDragMetric` in the metric registration file (same location where `TaxCostRatioMetric` is registered). Update the `TaxMetrics()` method on `Account` to populate the `TaxDrag` field.

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "TaxDrag" -v`
Expected: PASS

- [ ] **Step 7: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```
git add portfolio/tax_drag.go portfolio/tax_drag_test.go portfolio/tax_metrics.go portfolio/account.go
git commit -m "feat: add TaxDrag metric measuring return lost to trading taxes"
```

---

### Task 7: Substitution Mapping

Implement substitution registration and the Holdings/ProjectedHoldings/ProjectedWeights mapping that presents the logical view to strategies.

**Files:**
- Create: `portfolio/substitution.go`
- Create: `portfolio/substitution_test.go`
- Modify: `portfolio/account.go` (add substitution fields, implement RegisterSubstitution/ActiveSubstitutions, modify Holdings)
- Modify: `portfolio/batch.go:216-248` (modify ProjectedHoldings to map substituted assets)

- [ ] **Step 1: Write failing tests for substitution mapping**

Create `portfolio/substitution_test.go` with Ginkgo tests:

1. **RegisterSubstitution and ActiveSubstitutions** -- register a substitution, verify it appears in ActiveSubstitutions
2. **Holdings returns logical view** -- hold IVV with SPY->IVV substitution active, `Holdings()` returns SPY not IVV
3. **Holdings returns real view after expiry** -- same setup but substitution has expired, `Holdings()` returns IVV
4. **Value is unaffected** -- verify portfolio value is the same regardless of substitution mapping
5. **ProjectedHoldings maps substitute orders** -- batch with buy order for IVV (substitute), projected holdings shows SPY
6. **ProjectedWeights maps substitute assets** -- verify `ProjectedWeights()` returns weights keyed by the original asset (SPY), not the substitute (IVV). Since `ProjectedWeights()` calls `ProjectedHoldings()` internally, this should propagate automatically, but the test confirms it.
7. **Multiple substitutions** -- register two substitutions, verify both map correctly

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Substitution" -v`
Expected: FAIL

- [ ] **Step 3: Create substitution.go**

Create `portfolio/substitution.go` with helper functions for the mapping logic:

```go
package portfolio

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// mapToLogical converts a real asset to its logical original if an active
// substitution exists. If no substitution is active, returns the asset unchanged.
func mapToLogical(ast asset.Asset, subs map[asset.Asset]Substitution, asOf time.Time) asset.Asset {
	for _, sub := range subs {
		if sub.Substitute == ast && asOf.Before(sub.Until) {
			return sub.Original
		}
	}
	return ast
}
```

- [ ] **Step 4: Implement RegisterSubstitution and ActiveSubstitutions on Account**

In `portfolio/account.go`, add a `substitutions map[asset.Asset]Substitution` field to Account. Initialize in `New()`. Implement:

```go
func (a *Account) RegisterSubstitution(original, substitute asset.Asset, until time.Time) {
	a.substitutions[original] = Substitution{
		Original:   original,
		Substitute: substitute,
		Until:      until,
	}
}

func (a *Account) ActiveSubstitutions() map[asset.Asset]Substitution {
	result := make(map[asset.Asset]Substitution, len(a.substitutions))
	for k, v := range a.substitutions {
		result[k] = v
	}
	return result
}
```

- [ ] **Step 5: Modify Holdings() to apply substitution mapping**

In `portfolio/account.go`, modify `Holdings()` (around line 346-350) to map substituted assets to their logical originals using `mapToLogical`. The callback receives the logical asset name instead of the real one. Only apply the mapping for substitutions that haven't expired (check against the account's current date).

- [ ] **Step 6: Modify ProjectedHoldings() to handle substituted orders**

In `portfolio/batch.go`, modify `ProjectedHoldings()` (lines 216-248) so that when processing orders, if an order's asset is a substitute for an active substitution, the projected holding is recorded under the original asset name.

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Substitution" -v`
Expected: PASS

- [ ] **Step 8: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v`
Expected: PASS (Holdings mapping should not break existing tests since no substitutions are registered by default)

- [ ] **Step 9: Commit**

```
git add portfolio/substitution.go portfolio/substitution_test.go portfolio/account.go portfolio/batch.go
git commit -m "feat: substitution mapping for Holdings and ProjectedHoldings"
```

---

### Task 8: Ginkgo Suite Setup for tax Package

The `tax` package needs a Ginkgo test suite bootstrap file before any tests can run.

**Files:**
- Create: `tax/tax_suite_test.go`

- [ ] **Step 1: Create Ginkgo suite file**

Create `tax/tax_suite_test.go`:

```go
package tax_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTax(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tax Suite")
}
```

- [ ] **Step 2: Verify suite runs**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./tax/ -v`
Expected: PASS (no tests yet, but suite initializes)

- [ ] **Step 3: Commit**

```
git add tax/tax_suite_test.go
git commit -m "feat: add Ginkgo test suite for tax package"
```

---

### Task 9: Tax Loss Harvester Middleware

Implement the `TaxLossHarvester` middleware in a new `tax` package.

**Files:**
- Create: `tax/tax.go`
- Create: `tax/tax_loss_harvester.go`
- Create: `tax/tax_loss_harvester_test.go`

- [ ] **Step 1: Write failing tests for the harvester**

Create `tax/tax_loss_harvester_test.go` with Ginkgo tests. Reference `risk/max_position_size_test.go` for the pattern of building accounts, recording buys, creating batches, and calling `Process()`.

Test cases:

1. **Harvests loss exceeding threshold** -- position down 10%, threshold 5%, verify sell order injected with HighestCost lot selection and justification
2. **Skips position below threshold** -- position down 3%, threshold 5%, verify no orders injected
3. **Gain-offset mode skips when no gains** -- `GainOffsetOnly: true`, no realized gains, verify no harvest
4. **Gain-offset mode harvests when gains exist** -- `GainOffsetOnly: true`, realized gains exist, verify harvest occurs
5. **Respects wash sale window** -- position in wash sale window (sold at loss within 30 days), no substitute configured, verify no harvest
6. **Harvests despite wash sale when substitute configured** -- wash sale window active but substitute mapped, verify sell original + buy substitute
7. **Silent when nothing harvestable** -- no positions at a loss, verify batch.Orders unchanged and no annotations
8. **Substitute buy matches sold lots' dollar value** -- verify the substitute buy order amount matches the sold lots, not the full position

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./tax/ -v`
Expected: compilation errors

- [ ] **Step 3: Create tax/tax.go with DataSource interface and config**

Create `tax/tax.go`:

```go
package tax

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// DataSource provides market data access for tax middleware. The engine
// satisfies this interface.
type DataSource interface {
	Fetch(ctx context.Context, assets []asset.Asset, lookback data.Period, metrics []data.Metric) (*data.DataFrame, error)
	FetchAt(ctx context.Context, assets []asset.Asset, t time.Time, metrics []data.Metric) (*data.DataFrame, error)
	CurrentDate() time.Time
}

// HarvesterConfig configures the TaxLossHarvester middleware.
type HarvesterConfig struct {
	LossThreshold  float64                    // minimum unrealized loss % to harvest (e.g., 0.05 for 5%)
	GainOffsetOnly bool                       // only harvest when realized gains exist to offset
	Substitutes    map[asset.Asset]asset.Asset // optional asset-to-substitute mapping
	DataSource     DataSource
}
```

- [ ] **Step 4: Create tax/tax_loss_harvester.go**

Create `tax/tax_loss_harvester.go` implementing `portfolio.Middleware`:

```go
package tax

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/portfolio"
)

type taxLossHarvester struct {
	config HarvesterConfig
}

// LossHarvester returns a middleware that identifies positions with unrealized
// losses exceeding the threshold and injects sell orders to harvest the loss.
// When a substitute is configured, it buys the substitute and registers the
// substitution on the portfolio.
func LossHarvester(config HarvesterConfig) portfolio.Middleware {
	return &taxLossHarvester{config: config}
}

func (h *taxLossHarvester) Process(ctx context.Context, batch *portfolio.Batch) error {
	taxPortfolio, ok := batch.Portfolio().(portfolio.TaxAware)
	if !ok {
		return fmt.Errorf("tax: portfolio does not implement TaxAware")
	}

	// Step 1: Check gain-offset mode.
	if h.config.GainOffsetOnly {
		ltcg, stcg := taxPortfolio.RealizedGainsYTD()
		if ltcg+stcg <= 0 {
			return nil
		}
	}

	// Step 2: Check for expired substitutions and inject swap-back orders.
	currentDate := h.config.DataSource.CurrentDate()
	for _, sub := range taxPortfolio.ActiveSubstitutions() {
		if !currentDate.Before(sub.Until) {
			// Swap back: sell substitute, buy original.
			// ... inject orders and unregister substitution
		}
	}

	// Step 3: Scan positions for harvestable losses.
	batch.Portfolio().Holdings(func(ast asset.Asset, qty float64) {
		lots := taxPortfolio.UnrealizedLots(ast)
		// ... compute unrealized loss per lot
		// ... check threshold
		// ... check wash sale window
		// ... inject sell with WithLotSelection(HighestCost) + WithJustification
		// ... if substitute configured, inject buy and RegisterSubstitution
	})

	return nil
}
```

The implementation needs access to current prices to compute unrealized P&L per lot. Use the batch's price data or fetch via DataSource.

- [ ] **Step 5: Implement the full Process() logic**

Note: `Batch` already exposes a `Portfolio()` method that returns the `Portfolio` interface. The tax middleware uses `batch.Portfolio().(TaxAware)` to type-assert for tax-specific access.

Complete the harvester implementation following the spec's Process() logic (steps 1-7). Key details:

- For each position, iterate `UnrealizedLots()` and compute `(currentPrice - lot.Price) / lot.Price` to get the loss percentage
- If loss exceeds threshold, check `WashSaleWindow()` -- if the asset appears in recent wash sale records and no substitute is configured, skip
- Inject sell via `batch.Order(ctx, ast, Sell, qty, WithLotSelection(LotHighestCost), WithJustification(...))`
- If substitute exists, inject buy via `batch.Order(ctx, substitute, Buy, qty)` and call `taxPortfolio.RegisterSubstitution(ast, substitute, currentDate.AddDate(0, 0, 31))`

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./tax/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```
git add tax/tax.go tax/tax_loss_harvester.go tax/tax_loss_harvester_test.go
git commit -m "feat: TaxLossHarvester middleware with loss harvesting and substitute support"
```

---

### Task 10: Tax Profiles

Add convenience profile constructors in the `tax` package, following the pattern from `risk/profiles.go`.

**Files:**
- Create: `tax/profiles.go`
- Create: `tax/profiles_test.go`

- [ ] **Step 1: Write failing test for TaxEfficient profile**

Create `tax/profiles_test.go`:

```go
package tax_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tax"
)

var _ = Describe("Tax profiles", func() {
	It("TaxEfficient returns a middleware slice", func() {
		mw := tax.TaxEfficient(tax.HarvesterConfig{
			LossThreshold: 0.05,
		})
		Expect(mw).To(HaveLen(1))
		// Verify it implements Middleware.
		var _ portfolio.Middleware = mw[0]
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./tax/ -run "Tax profiles" -v`
Expected: FAIL

- [ ] **Step 3: Create tax/profiles.go**

```go
package tax

import "github.com/penny-vault/pvbt/portfolio"

// TaxEfficient returns a middleware chain for tax-optimized trading.
// Currently contains only the TaxLossHarvester; future tax middleware
// will be added to this chain.
func TaxEfficient(config HarvesterConfig) []portfolio.Middleware {
	return []portfolio.Middleware{
		LossHarvester(config),
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./tax/ -run "Tax profiles" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add tax/profiles.go tax/profiles_test.go
git commit -m "feat: add tax.TaxEfficient convenience profile"
```

---

### Task 11: Integration Test and Full Verification

End-to-end test verifying the complete tax optimization flow: lot selection, wash sale detection, loss harvesting middleware, substitution mapping, and tax drag metric working together.

**Files:**
- Create: `tax/integration_test.go`

- [ ] **Step 1: Write integration test**

Create `tax/integration_test.go` with a scenario:

1. Create account with `WithDefaultLotSelection(LotHighestCost)` and cash
2. Set up price data with two assets (SPY, IVV) and a DataSource mock
3. Record buys of SPY at various prices over time
4. Configure `TaxLossHarvester` with 5% threshold, substitute map {SPY: IVV}
5. Register middleware on account
6. Create a batch where SPY price has dropped > 5% from some lots
7. Execute the batch
8. Verify: sell order for SPY injected with justification, buy order for IVV injected, substitution registered
9. Verify: `Holdings()` returns SPY (logical view), actual holdings are IVV
10. Verify: wash sale records are empty (first harvest, no prior loss sale)
11. Advance past 30 days, create new batch, verify swap-back orders injected
12. Check `TaxMetrics().TaxDrag` is computed correctly

- [ ] **Step 2: Run integration test**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./tax/ -run "Integration" -v`
Expected: PASS

- [ ] **Step 3: Run full project test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -v`
Expected: all tests pass

- [ ] **Step 4: Commit**

```
git add tax/integration_test.go
git commit -m "test: add end-to-end integration test for tax optimization"
```

---

### Task 12: Documentation and Changelog

Update documentation and changelog for the tax optimization feature.

**Files:**
- Modify: `CHANGELOG.md` (add entry for tax optimization feature)

- [ ] **Step 1: Read current CHANGELOG.md format**

Read `CHANGELOG.md` to understand the format and where to add the new entry.

- [ ] **Step 2: Add changelog entry**

Add an entry under the appropriate version section describing the tax optimization feature. Follow the existing changelog style: complete sentences, active voice, user-facing language, related items combined into a single entry.

- [ ] **Step 3: Commit**

```
git add CHANGELOG.md
git commit -m "docs: add changelog entry for tax optimization feature"
```
