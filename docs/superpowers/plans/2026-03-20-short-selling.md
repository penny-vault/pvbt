# Short Selling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add short selling support including short tax lots, split handling, margin accounting, borrow fees, dividend debits, margin calls, risk middleware updates, simulated broker margin enforcement, and long/short P&L reporting.

**Architecture:** Bottom-up implementation starting from the data model (short tax lots, transaction types), then engine housekeeping (splits, borrow fees, dividends), margin accounting, margin call handling, risk middleware, simulated broker, and finally P&L reporting. Each layer is independently testable before the next builds on it.

**Tech Stack:** Go, Ginkgo/Gomega testing framework

**Spec:** `docs/superpowers/specs/2026-03-20-short-selling-design.md`

---

### Task 1: Add SplitTransaction Type and TradeDirection

**Files:**
- Modify: `portfolio/transaction.go:49-67` (TransactionType enum and String())
- Create: `portfolio/trade_direction.go`
- Modify: `portfolio/trade_detail.go:27-38` (TradeDetail struct)
- Test: `portfolio/transaction_test.go`

- [ ] **Step 1: Write failing test for SplitTransaction String()**

```go
It("returns Split for SplitTransaction", func() {
    Expect(portfolio.SplitTransaction.String()).To(Equal("Split"))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/ -run "Split" -v`
Expected: FAIL -- SplitTransaction is undefined

- [ ] **Step 3: Add SplitTransaction to TransactionType enum**

In `portfolio/transaction.go`, add `SplitTransaction` after `WithdrawalTransaction` in the const block and add the `"Split"` case to `String()`.

```go
// SplitTransaction records a stock split adjustment.
SplitTransaction
```

```go
case SplitTransaction:
    return "Split"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./portfolio/ -run "Split" -v`
Expected: PASS

- [ ] **Step 5: Create trade_direction.go with TradeDirection type**

Create `portfolio/trade_direction.go`:

```go
package portfolio

// TradeDirection indicates whether a round-trip trade was a long or short position.
type TradeDirection int

const (
    TradeLong  TradeDirection = iota
    TradeShort
)

// String returns "Long" or "Short".
func (d TradeDirection) String() string {
    if d == TradeShort {
        return "Short"
    }
    return "Long"
}
```

- [ ] **Step 6: Add Direction field to TradeDetail**

In `portfolio/trade_detail.go`, add `Direction TradeDirection` field to `TradeDetail` struct after `MAE`:

```go
type TradeDetail struct {
    Asset      asset.Asset
    EntryDate  time.Time
    ExitDate   time.Time
    EntryPrice float64
    ExitPrice  float64
    Qty        float64
    PnL        float64
    HoldDays   float64
    MFE        float64
    MAE        float64
    Direction  TradeDirection
}
```

- [ ] **Step 7: Run full portfolio test suite**

Run: `go test ./portfolio/ -v`
Expected: PASS (existing tests should not break -- Direction defaults to TradeLong which is zero value)

- [ ] **Step 8: Commit**

```bash
git add portfolio/transaction.go portfolio/trade_direction.go portfolio/trade_detail.go portfolio/transaction_test.go
git commit -m "feat: add SplitTransaction type and TradeDirection for short selling"
```

---

### Task 2: Add shortLots Map and SkipMiddleware Flag

**Files:**
- Modify: `portfolio/account.go:44-67` (Account struct)
- Modify: `portfolio/account.go:69-91` (New() constructor)
- Modify: `portfolio/batch.go:33-47` (Batch struct)
- Modify: `portfolio/account.go:1434-1440` (ExecuteBatch middleware loop)
- Test: `portfolio/account_test.go`
- Test: `portfolio/batch_test.go`

- [ ] **Step 1: Write failing test for ShortLots accessor**

```go
It("returns empty short lots for new account", func() {
    acct := portfolio.New()
    var count int
    acct.ShortLots(func(a asset.Asset, lots []portfolio.TaxLot) {
        count += len(lots)
    })
    Expect(count).To(Equal(0))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/ -run "short lots" -v`
Expected: FAIL -- ShortLots method does not exist

- [ ] **Step 3: Add shortLots field to Account and initialize in New()**

In `portfolio/account.go`, add `shortLots map[asset.Asset][]TaxLot` field to `Account` struct after `taxLots`. Initialize it in `New()`:

```go
shortLots: make(map[asset.Asset][]TaxLot),
```

Add `ShortLots` accessor:

```go
// ShortLots iterates over all open short tax lots, calling fn with each asset and its lots.
func (a *Account) ShortLots(fn func(asset.Asset, []TaxLot)) {
    for ast, lots := range a.shortLots {
        fn(ast, lots)
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./portfolio/ -run "short lots" -v`
Expected: PASS

- [ ] **Step 5: Write failing test for SkipMiddleware**

```go
It("skips middleware when SkipMiddleware is true", func() {
    called := false
    mw := &mockMiddleware{processFn: func(_ context.Context, _ *portfolio.Batch) error {
        called = true
        return nil
    }}

    acct := portfolio.New(portfolio.WithCash(10000), portfolio.WithBroker(newMockBroker()))
    acct.Use(mw)

    batch := acct.NewBatch(time.Now())
    batch.SkipMiddleware = true
    Expect(acct.ExecuteBatch(context.Background(), batch)).To(Succeed())
    Expect(called).To(BeFalse())
})
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./portfolio/ -run "SkipMiddleware" -v`
Expected: FAIL -- SkipMiddleware field does not exist

- [ ] **Step 7: Add SkipMiddleware field to Batch and check in ExecuteBatch**

In `portfolio/batch.go`, add to `Batch` struct:

```go
// SkipMiddleware bypasses the middleware chain when true. Used for
// margin-call response batches where risk limits must not block
// emergency position adjustments.
SkipMiddleware bool
```

In `portfolio/account.go`, modify `ExecuteBatch` to check the flag:

```go
// 1. Run middleware chain.
if !batch.SkipMiddleware {
    for _, mw := range a.middleware {
        if err := mw.Process(ctx, batch); err != nil {
            return err
        }
    }
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./portfolio/ -run "SkipMiddleware" -v`
Expected: PASS

- [ ] **Step 9: Run full portfolio test suite**

Run: `go test ./portfolio/ -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add portfolio/account.go portfolio/batch.go portfolio/account_test.go portfolio/batch_test.go
git commit -m "feat: add shortLots map and SkipMiddleware flag for short selling"
```

---

### Task 3: Refactor Record() Sell Path for Short Lot Creation

**Files:**
- Modify: `portfolio/account.go:744-816` (Record() sell case)
- Test: `portfolio/account_test.go`

- [ ] **Step 1: Write failing test -- sell without long lots creates short lots**

```go
It("creates short lots when selling without long positions", func() {
    acct := portfolio.New(portfolio.WithCash(100000))
    testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.SellTransaction,
        Qty:   100,
        Price: 150.0,
        Amount: 15000.0, // cash inflow from short sale
    })

    Expect(acct.Position(testAsset)).To(Equal(-100.0))
    Expect(acct.Cash()).To(Equal(115000.0))

    var shortLotCount int
    var shortLotQty float64
    var shortLotPrice float64
    acct.ShortLots(func(a asset.Asset, lots []portfolio.TaxLot) {
        if a == testAsset {
            shortLotCount = len(lots)
            if len(lots) > 0 {
                shortLotQty = lots[0].Qty
                shortLotPrice = lots[0].Price
            }
        }
    })
    Expect(shortLotCount).To(Equal(1))
    Expect(shortLotQty).To(Equal(100.0))
    Expect(shortLotPrice).To(Equal(150.0))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/ -run "creates short lots" -v`
Expected: FAIL -- short lots are empty (current sell path does not create them)

- [ ] **Step 3: Write failing test -- sell partially closes long, remainder opens short**

```go
It("closes long lots then creates short lots for the remainder", func() {
    acct := portfolio.New(portfolio.WithCash(100000))
    testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

    // Buy 50 shares
    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.BuyTransaction,
        Qty:   50,
        Price: 140.0,
        Amount: -7000.0,
    })

    // Sell 80 shares -- closes 50 long, opens 30 short
    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.SellTransaction,
        Qty:   80,
        Price: 150.0,
        Amount: 12000.0,
    })

    Expect(acct.Position(testAsset)).To(Equal(-30.0))

    // Long lots should be fully consumed
    longLots := acct.UnrealizedLots(testAsset)
    Expect(longLots).To(BeEmpty())

    // Short lots should have the remainder
    var shortLotQty float64
    acct.ShortLots(func(a asset.Asset, lots []portfolio.TaxLot) {
        if a == testAsset {
            for _, lot := range lots {
                shortLotQty += lot.Qty
            }
        }
    })
    Expect(shortLotQty).To(Equal(30.0))
})
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./portfolio/ -run "closes long lots then creates short" -v`
Expected: FAIL

- [ ] **Step 5: Refactor Record() sell path**

Replace the `case SellTransaction:` block in `Record()` (lines 744-816) with the two-phase approach:

```go
case SellTransaction:
    a.holdings[txn.Asset] -= txn.Qty

    method := txn.LotSelection
    if method == LotFIFO && a.lotSelection != LotFIFO {
        method = a.lotSelection
    }

    // Phase 1: Close long lots (if any exist).
    longLots := a.taxLots[txn.Asset]
    longQty := 0.0
    for _, lot := range longLots {
        longQty += lot.Qty
    }

    closeLongQty := txn.Qty
    if closeLongQty > longQty {
        closeLongQty = longQty
    }

    if closeLongQty > 0 {
        // Generate TradeDetail entries from excursion data.
        if excursion, hasExcursion := a.excursions[txn.Asset]; hasExcursion {
            mfe := (excursion.HighPrice - excursion.EntryPrice) / excursion.EntryPrice
            mae := (excursion.LowPrice - excursion.EntryPrice) / excursion.EntryPrice

            tdRemaining := closeLongQty
            tdLots := a.taxLots[txn.Asset]
            for tdLotIdx := 0; tdLotIdx < len(tdLots) && tdRemaining > 0; tdLotIdx++ {
                matched := tdLots[tdLotIdx].Qty
                if matched > tdRemaining {
                    matched = tdRemaining
                }

                a.tradeDetails = append(a.tradeDetails, TradeDetail{
                    Asset:      txn.Asset,
                    EntryDate:  tdLots[tdLotIdx].Date,
                    ExitDate:   txn.Date,
                    EntryPrice: tdLots[tdLotIdx].Price,
                    ExitPrice:  txn.Price,
                    Qty:        matched,
                    PnL:        (txn.Price - tdLots[tdLotIdx].Price) * matched,
                    HoldDays:   txn.Date.Sub(tdLots[tdLotIdx].Date).Hours() / 24.0,
                    MFE:        mfe,
                    MAE:        mae,
                    Direction:  TradeLong,
                })

                tdRemaining -= matched
            }
        }

        consumed := a.computeConsumedLotInfo(txn.Asset, closeLongQty, method)
        a.consumeLots(txn.Asset, closeLongQty, method)

        // Check for wash sale on the long close.
        lossPerShare := consumed.avgCostBasis - txn.Price
        if lossPerShare > 0 {
            disallowedQty := a.checkWashSaleOnSell(txn.Asset, txn.Date, closeLongQty, lossPerShare, consumed.latestBuyDate)

            remainingLossQty := closeLongQty - disallowedQty
            if remainingLossQty > 0 {
                a.recentLossSales[txn.Asset] = append(a.recentLossSales[txn.Asset], recentLossSale{
                    date:         txn.Date,
                    lossPerShare: lossPerShare,
                    qty:          remainingLossQty,
                })
            }
        }
    }

    // Phase 2: Open short lots for the remainder.
    shortQty := txn.Qty - closeLongQty
    if shortQty > 0 {
        lotID := fmt.Sprintf("short-%d-%d", txn.Date.UnixNano(), len(a.shortLots[txn.Asset]))
        a.shortLots[txn.Asset] = append(a.shortLots[txn.Asset], TaxLot{
            ID:    lotID,
            Date:  txn.Date,
            Qty:   shortQty,
            Price: txn.Price,
        })

        // Initialize excursion tracking for the short position.
        if _, exists := a.excursions[txn.Asset]; !exists {
            a.excursions[txn.Asset] = ExcursionRecord{
                EntryPrice: txn.Price,
                HighPrice:  txn.Price,
                LowPrice:   txn.Price,
            }
        }
    }

    // Cleanup: remove tracking when fully flat.
    if a.holdings[txn.Asset] == 0 {
        delete(a.holdings, txn.Asset)
        delete(a.taxLots, txn.Asset)
        delete(a.shortLots, txn.Asset)
        delete(a.excursions, txn.Asset)
    }
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./portfolio/ -v`
Expected: PASS (both new and existing tests)

- [ ] **Step 7: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go
git commit -m "feat: refactor Record() sell path to create short lots"
```

---

### Task 4: Refactor Record() Buy Path for Short Lot Consumption

**Files:**
- Modify: `portfolio/account.go:714-743` (Record() buy case)
- Test: `portfolio/account_test.go`

- [ ] **Step 1: Write failing test -- buy covers short lots**

```go
It("covers short lots on buy when short position exists", func() {
    acct := portfolio.New(portfolio.WithCash(100000))
    testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

    // Open short: sell 100 at 150
    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.SellTransaction,
        Qty:   100,
        Price: 150.0,
        Amount: 15000.0,
    })

    // Cover: buy 100 at 140 (profit $10/share)
    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.BuyTransaction,
        Qty:   100,
        Price: 140.0,
        Amount: -14000.0,
    })

    Expect(acct.Position(testAsset)).To(Equal(0.0))
    Expect(acct.Cash()).To(Equal(101000.0)) // 100k + 15k - 14k

    // Short lots should be consumed
    var shortLotCount int
    acct.ShortLots(func(a asset.Asset, lots []portfolio.TaxLot) {
        if a == testAsset {
            shortLotCount = len(lots)
        }
    })
    Expect(shortLotCount).To(Equal(0))

    // TradeDetail should be generated with short direction
    details := acct.TradeDetails()
    Expect(details).To(HaveLen(1))
    Expect(details[0].Direction).To(Equal(portfolio.TradeShort))
    Expect(details[0].PnL).To(Equal(1000.0)) // (150 - 140) * 100
    Expect(details[0].EntryPrice).To(Equal(150.0))
    Expect(details[0].ExitPrice).To(Equal(140.0))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/ -run "covers short lots" -v`
Expected: FAIL -- buy path creates long lots instead of consuming short lots

- [ ] **Step 3: Write failing test -- buy partially covers short, remainder creates long lots**

```go
It("partially covers short then creates long lots for remainder", func() {
    acct := portfolio.New(portfolio.WithCash(100000))
    testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

    // Open short: sell 50 at 150
    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.SellTransaction,
        Qty:   50,
        Price: 150.0,
        Amount: 7500.0,
    })

    // Buy 80: covers 50 short, opens 30 long
    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.BuyTransaction,
        Qty:   80,
        Price: 140.0,
        Amount: -11200.0,
    })

    Expect(acct.Position(testAsset)).To(Equal(30.0))

    // Short lots should be consumed
    var shortLotCount int
    acct.ShortLots(func(a asset.Asset, lots []portfolio.TaxLot) {
        if a == testAsset {
            shortLotCount = len(lots)
        }
    })
    Expect(shortLotCount).To(Equal(0))

    // Long lots should have the remainder
    longLots := acct.UnrealizedLots(testAsset)
    totalLongQty := 0.0
    for _, lot := range longLots {
        totalLongQty += lot.Qty
    }
    Expect(totalLongQty).To(Equal(30.0))
})
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./portfolio/ -run "partially covers short" -v`
Expected: FAIL

- [ ] **Step 5: Refactor Record() buy path**

Replace the `case BuyTransaction:` block in `Record()` with short-aware routing:

```go
case BuyTransaction:
    a.holdings[txn.Asset] += txn.Qty

    // Phase 1: Cover short lots (if any exist).
    shortLots := a.shortLots[txn.Asset]
    shortQty := 0.0
    for _, lot := range shortLots {
        shortQty += lot.Qty
    }

    coverQty := txn.Qty
    if coverQty > shortQty {
        coverQty = shortQty
    }

    if coverQty > 0 {
        method := txn.LotSelection
        if method == LotFIFO && a.lotSelection != LotFIFO {
            method = a.lotSelection
        }

        // Generate TradeDetail entries for the short cover.
        if excursion, hasExcursion := a.excursions[txn.Asset]; hasExcursion {
            // For shorts: MFE is entry-low (price fell), MAE is entry-high (price rose)
            mfe := (excursion.EntryPrice - excursion.LowPrice) / excursion.EntryPrice
            mae := (excursion.EntryPrice - excursion.HighPrice) / excursion.EntryPrice

            tdRemaining := coverQty
            tdLots := a.shortLots[txn.Asset]
            for tdLotIdx := 0; tdLotIdx < len(tdLots) && tdRemaining > 0; tdLotIdx++ {
                matched := tdLots[tdLotIdx].Qty
                if matched > tdRemaining {
                    matched = tdRemaining
                }

                a.tradeDetails = append(a.tradeDetails, TradeDetail{
                    Asset:      txn.Asset,
                    EntryDate:  tdLots[tdLotIdx].Date,
                    ExitDate:   txn.Date,
                    EntryPrice: tdLots[tdLotIdx].Price,
                    ExitPrice:  txn.Price,
                    Qty:        matched,
                    PnL:        (tdLots[tdLotIdx].Price - txn.Price) * matched,
                    HoldDays:   txn.Date.Sub(tdLots[tdLotIdx].Date).Hours() / 24.0,
                    MFE:        mfe,
                    MAE:        mae,
                    Direction:  TradeShort,
                })

                tdRemaining -= matched
            }
        }

        // Consume short lots using the same selection methods as longs.
        a.consumeShortLots(txn.Asset, coverQty, method)

        // Wash sale check: covering a short at a loss.
        avgShortEntry := a.avgShortEntryPrice(txn.Asset, coverQty, method)
        lossPerShare := txn.Price - avgShortEntry
        if lossPerShare > 0 {
            a.recentLossSales[txn.Asset] = append(a.recentLossSales[txn.Asset], recentLossSale{
                date:         txn.Date,
                lossPerShare: lossPerShare,
                qty:          coverQty,
            })
        }
    }

    // Phase 2: Create long lots for the remainder.
    longQty := txn.Qty - coverQty
    if longQty > 0 {
        lotID := fmt.Sprintf("lot-%d-%d", txn.Date.UnixNano(), len(a.taxLots[txn.Asset]))
        newLot := TaxLot{
            ID:    lotID,
            Date:  txn.Date,
            Qty:   longQty,
            Price: txn.Price,
        }

        a.taxLots[txn.Asset] = append(a.taxLots[txn.Asset], newLot)

        a.checkWashSaleOnBuy(txn.Asset, txn.Date, longQty, lotID)

        a.recentBuys[txn.Asset] = append(a.recentBuys[txn.Asset], recentBuy{
            date:  txn.Date,
            lotID: lotID,
            qty:   longQty,
        })

        if _, exists := a.excursions[txn.Asset]; !exists {
            a.excursions[txn.Asset] = ExcursionRecord{
                EntryPrice: txn.Price,
                HighPrice:  txn.Price,
                LowPrice:   txn.Price,
            }
        }
    }

    // Cleanup: remove tracking when fully flat.
    if a.holdings[txn.Asset] == 0 {
        delete(a.holdings, txn.Asset)
        delete(a.taxLots, txn.Asset)
        delete(a.shortLots, txn.Asset)
        delete(a.excursions, txn.Asset)
    }
```

- [ ] **Step 6: Add consumeShortLots and avgShortEntryPrice helper methods**

Add `consumeShortLots` to `portfolio/account.go` mirroring `consumeLots` but operating on `a.shortLots`:

```go
// consumeShortLots removes qty shares from the short lots for the given
// asset using the specified lot selection method. Mirrors consumeLots.
func (a *Account) consumeShortLots(ast asset.Asset, qty float64, method LotSelection) {
    lots := a.shortLots[ast]

    switch method {
    case LotLIFO:
        remaining := qty
        end := len(lots)
        for end > 0 && remaining > 0 {
            idx := end - 1
            if lots[idx].Qty <= remaining {
                remaining -= lots[idx].Qty
                end--
            } else {
                lots[idx].Qty -= remaining
                remaining = 0
            }
        }
        a.shortLots[ast] = lots[:end]

    case LotHighestCost:
        sortLotsByPriceDesc(lots)
        remaining := qty
        lotIdx := 0
        for lotIdx < len(lots) && remaining > 0 {
            if lots[lotIdx].Qty <= remaining {
                remaining -= lots[lotIdx].Qty
                lotIdx++
            } else {
                lots[lotIdx].Qty -= remaining
                remaining = 0
            }
        }
        remainingLots := lots[lotIdx:]
        sortLotsByDateAsc(remainingLots)
        a.shortLots[ast] = remainingLots

    default: // LotFIFO
        remaining := qty
        lotIdx := 0
        for lotIdx < len(lots) && remaining > 0 {
            if lots[lotIdx].Qty <= remaining {
                remaining -= lots[lotIdx].Qty
                lotIdx++
            } else {
                lots[lotIdx].Qty -= remaining
                remaining = 0
            }
        }
        a.shortLots[ast] = lots[lotIdx:]
    }
}

// avgShortEntryPrice computes the weighted average entry price of short
// lots that would be consumed for the given qty and method.
func (a *Account) avgShortEntryPrice(ast asset.Asset, qty float64, method LotSelection) float64 {
    lots := make([]TaxLot, len(a.shortLots[ast]))
    copy(lots, a.shortLots[ast])

    switch method {
    case LotLIFO:
        // Reverse iterate
        remaining := qty
        totalCost := 0.0
        totalQty := 0.0
        for idx := len(lots) - 1; idx >= 0 && remaining > 0; idx-- {
            matched := lots[idx].Qty
            if matched > remaining {
                matched = remaining
            }
            totalCost += matched * lots[idx].Price
            totalQty += matched
            remaining -= matched
        }
        if totalQty == 0 {
            return 0
        }
        return totalCost / totalQty

    case LotHighestCost:
        sortLotsByPriceDesc(lots)
        fallthrough

    default: // FIFO or HighestCost after sort
        remaining := qty
        totalCost := 0.0
        totalQty := 0.0
        for idx := 0; idx < len(lots) && remaining > 0; idx++ {
            matched := lots[idx].Qty
            if matched > remaining {
                matched = remaining
            }
            totalCost += matched * lots[idx].Price
            totalQty += matched
            remaining -= matched
        }
        if totalQty == 0 {
            return 0
        }
        return totalCost / totalQty
    }
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./portfolio/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go
git commit -m "feat: refactor Record() buy path to consume short lots"
```

---

### Task 5: Split Handling in Engine Housekeeping

**Files:**
- Modify: `portfolio/account.go` (add ApplySplit method)
- Modify: `engine/backtest.go:379-436` (housekeepAccount)
- Modify: `engine/live.go` (housekeeping section)
- Test: `portfolio/account_test.go`
- Test: `engine/backtest_test.go`

- [ ] **Step 1: Write failing test for ApplySplit on Account**

```go
It("adjusts long position quantity and tax lot for a 2:1 split", func() {
    acct := portfolio.New(portfolio.WithCash(100000))
    testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.BuyTransaction,
        Qty:   100,
        Price: 200.0,
        Amount: -20000.0,
    })

    err := acct.ApplySplit(testAsset, time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), 2.0)
    Expect(err).NotTo(HaveOccurred())

    Expect(acct.Position(testAsset)).To(Equal(200.0))

    lots := acct.UnrealizedLots(testAsset)
    Expect(lots).To(HaveLen(1))
    Expect(lots[0].Qty).To(Equal(200.0))
    Expect(lots[0].Price).To(Equal(100.0))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/ -run "adjusts long position.*split" -v`
Expected: FAIL -- ApplySplit does not exist

- [ ] **Step 3: Write failing test for split on short position**

```go
It("adjusts short position quantity and short lots for a 2:1 split", func() {
    acct := portfolio.New(portfolio.WithCash(100000))
    testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.SellTransaction,
        Qty:   100,
        Price: 200.0,
        Amount: 20000.0,
    })

    err := acct.ApplySplit(testAsset, time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), 2.0)
    Expect(err).NotTo(HaveOccurred())

    Expect(acct.Position(testAsset)).To(Equal(-200.0))

    var shortLotQty, shortLotPrice float64
    acct.ShortLots(func(a asset.Asset, lots []portfolio.TaxLot) {
        if a == testAsset && len(lots) > 0 {
            shortLotQty = lots[0].Qty
            shortLotPrice = lots[0].Price
        }
    })
    Expect(shortLotQty).To(Equal(200.0))
    Expect(shortLotPrice).To(Equal(100.0))
})
```

- [ ] **Step 4: Write failing test for split factor of 0 (error)**

```go
It("rejects split factor of zero", func() {
    acct := portfolio.New(portfolio.WithCash(100000))
    testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}
    err := acct.ApplySplit(testAsset, time.Now(), 0.0)
    Expect(err).To(HaveOccurred())
})
```

- [ ] **Step 5: Implement ApplySplit on Account**

Add to `portfolio/account.go`:

```go
// ApplySplit adjusts position quantities and tax lot prices/quantities for a
// stock split. A splitFactor of 2.0 means a 2-for-1 split (quantity doubles,
// price halves). Returns an error if splitFactor is zero. Records a
// SplitTransaction in the transaction log.
func (a *Account) ApplySplit(ast asset.Asset, date time.Time, splitFactor float64) error {
    if splitFactor == 0 {
        return fmt.Errorf("apply split: split factor cannot be zero for %s", ast.Ticker)
    }

    qty := a.holdings[ast]
    if qty == 0 {
        return nil
    }

    oldQty := qty
    newQty := qty * splitFactor
    a.holdings[ast] = newQty

    // Adjust long tax lots.
    for idx := range a.taxLots[ast] {
        a.taxLots[ast][idx].Qty *= splitFactor
        a.taxLots[ast][idx].Price /= splitFactor
    }

    // Adjust short tax lots.
    for idx := range a.shortLots[ast] {
        a.shortLots[ast][idx].Qty *= splitFactor
        a.shortLots[ast][idx].Price /= splitFactor
    }

    // Adjust excursion record prices.
    if excursion, exists := a.excursions[ast]; exists {
        excursion.EntryPrice /= splitFactor
        excursion.HighPrice /= splitFactor
        excursion.LowPrice /= splitFactor
        a.excursions[ast] = excursion
    }

    // Record the split transaction.
    a.transactions = append(a.transactions, Transaction{
        Date:  date,
        Asset: ast,
        Type:  SplitTransaction,
        Qty:   newQty,
        Price: splitFactor,
        Amount: 0, // splits have no cash impact
        Justification: fmt.Sprintf("split %.4g:1 old_qty=%.4g new_qty=%.4g", splitFactor, oldQty, newQty),
    })

    return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./portfolio/ -run "split" -v`
Expected: PASS

- [ ] **Step 7: Reorder housekeepAccount and add splits in backtest engine**

In `engine/backtest.go`, modify `housekeepAccount`:

1. Add `data.SplitFactor` to the `housekeepMetrics` slice.
2. **Reorder the function**: the current code records dividends (lines 407-426) *before* draining fills (lines 428-433). The correct order for short selling is: drain fills first, then apply splits, then record dividends. Move the fill-draining block before the dividend recording block.
3. After fill draining and before dividend recording, add split processing:

```go
// Apply stock splits before dividends (dividend values are post-split).
if housekeepDF != nil {
    for _, heldAsset := range heldAssets {
        splitFactor := housekeepDF.ValueAt(heldAsset, data.SplitFactor, date)
        if math.IsNaN(splitFactor) || splitFactor == 1.0 {
            continue
        }
        if err := acct.ApplySplit(heldAsset, date, splitFactor); err != nil {
            return fmt.Errorf("engine: apply split for %s on %v: %w", heldAsset.Ticker, date, err)
        }
    }
}
```

- [ ] **Step 8: Add same split handling to live engine**

Apply the same change to `engine/live.go` in the housekeeping section (add `data.SplitFactor` to metrics and add the split processing loop before dividends).

- [ ] **Step 9: Run engine tests**

Run: `go test ./engine/ -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go engine/backtest.go engine/live.go
git commit -m "feat: add split handling for long and short positions"
```

---

### Task 6: Margin Accounting on Account

**Files:**
- Modify: `portfolio/account.go` (add margin fields, methods)
- Modify: `portfolio/portfolio.go:38-125` (add to Portfolio interface)
- Create: `portfolio/margin.go` (margin calculation methods)
- Create: `portfolio/margin_option.go` (WithInitialMargin, WithMaintenanceMargin options)
- Test: `portfolio/margin_test.go`

- [ ] **Step 1: Write failing test for margin ratio with no shorts**

```go
Describe("Margin", func() {
    It("returns NaN margin ratio when no short positions exist", func() {
        acct := portfolio.New(portfolio.WithCash(100000))
        Expect(math.IsNaN(acct.MarginRatio())).To(BeTrue())
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/ -run "NaN margin ratio" -v`
Expected: FAIL -- MarginRatio does not exist

- [ ] **Step 3: Write failing test for margin ratio with short position**

```go
It("computes correct margin ratio with a short position", func() {
    acct := portfolio.New(portfolio.WithCash(100000))
    testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

    // Short 100 shares at $150 -> cash = 115000, short value = 15000
    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.SellTransaction,
        Qty:   100,
        Price: 150.0,
        Amount: 15000.0,
    })

    // Set prices so Value() works correctly
    priceDF := buildDF(testAsset, 150.0)
    acct.UpdatePrices(priceDF)

    // Equity = cash + long - short = 115000 + 0 - 15000 = 100000
    // Short market value = 15000
    // Margin ratio = 100000 / 15000 = 6.667
    ratio := acct.MarginRatio()
    Expect(ratio).To(BeNumerically("~", 6.667, 0.001))
})
```

- [ ] **Step 4: Write failing tests for ShortMarketValue, MarginDeficiency, BuyingPower**

```go
It("computes short market value", func() {
    acct := portfolio.New(portfolio.WithCash(100000))
    testAsset := asset.Asset{Ticker: "AAPL", CompositeFigi: "AAPL"}

    acct.Record(portfolio.Transaction{
        Date:  time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
        Asset: testAsset,
        Type:  portfolio.SellTransaction,
        Qty:   100,
        Price: 150.0,
        Amount: 15000.0,
    })

    priceDF := buildDF(testAsset, 150.0)
    acct.UpdatePrices(priceDF)

    Expect(acct.ShortMarketValue()).To(Equal(15000.0))
})

It("returns zero margin deficiency when margin is healthy", func() {
    acct := portfolio.New(portfolio.WithCash(100000))
    Expect(acct.MarginDeficiency()).To(Equal(0.0))
})
```

- [ ] **Step 5: Create margin.go with calculation methods**

Create `portfolio/margin.go`:

```go
package portfolio

import (
    "math"

    "github.com/penny-vault/pvbt/data"
)

const (
    defaultInitialMarginRate     = 0.50
    defaultMaintenanceMarginRate = 0.30
)

// ShortMarketValue returns the total absolute market value of all short
// positions at current prices.
func (a *Account) ShortMarketValue() float64 {
    total := 0.0
    if a.prices == nil {
        return total
    }
    for ast, qty := range a.holdings {
        if qty >= 0 {
            continue
        }
        price := a.prices.Value(ast, data.MetricClose)
        if !math.IsNaN(price) {
            total += math.Abs(qty) * price
        }
    }
    return total
}

// LongMarketValue returns the total market value of all long positions
// at current prices.
func (a *Account) LongMarketValue() float64 {
    total := 0.0
    if a.prices == nil {
        return total
    }
    for ast, qty := range a.holdings {
        if qty <= 0 {
            continue
        }
        price := a.prices.Value(ast, data.MetricClose)
        if !math.IsNaN(price) {
            total += qty * price
        }
    }
    return total
}

// Equity returns cash plus long market value minus short market value.
func (a *Account) Equity() float64 {
    return a.cash + a.LongMarketValue() - a.ShortMarketValue()
}

// MarginRatio returns equity divided by short market value. Returns NaN
// when no short positions exist.
func (a *Account) MarginRatio() float64 {
    smv := a.ShortMarketValue()
    if smv == 0 {
        return math.NaN()
    }
    return a.Equity() / smv
}

// MarginDeficiency returns the dollar amount needed to restore the
// maintenance margin requirement. Returns 0 if margin is not breached.
func (a *Account) MarginDeficiency() float64 {
    smv := a.ShortMarketValue()
    if smv == 0 {
        return 0
    }
    requiredEquity := smv * (1 + a.maintenanceMarginRate())
    deficit := requiredEquity - a.Equity()
    if deficit < 0 {
        return 0
    }
    return deficit
}

// BuyingPower returns the cash available for new positions, accounting
// for margin requirements on existing short positions.
func (a *Account) BuyingPower() float64 {
    smv := a.ShortMarketValue()
    reservedForShorts := smv * a.initialMarginRate()
    available := a.cash - reservedForShorts
    if available < 0 {
        return 0
    }
    return available
}

func (a *Account) initialMarginRate() float64 {
    if a.initialMargin > 0 {
        return a.initialMargin
    }
    return defaultInitialMarginRate
}

func (a *Account) maintenanceMarginRate() float64 {
    if a.maintenanceMargin > 0 {
        return a.maintenanceMargin
    }
    return defaultMaintenanceMarginRate
}
```

- [ ] **Step 6: Add margin fields to Account struct**

In `portfolio/account.go`, add to `Account` struct:

```go
initialMargin     float64
maintenanceMargin float64
borrowRate        float64
```

- [ ] **Step 7: Create margin_option.go with configuration options**

Create `portfolio/margin_option.go`:

```go
package portfolio

// WithInitialMargin sets the initial margin rate for short positions.
// Default is 0.50 (Reg T).
func WithInitialMargin(rate float64) Option {
    return func(a *Account) {
        a.initialMargin = rate
    }
}

// WithMaintenanceMargin sets the maintenance margin rate for short positions.
// Default is 0.30 (Reg T).
func WithMaintenanceMargin(rate float64) Option {
    return func(a *Account) {
        a.maintenanceMargin = rate
    }
}

// WithBorrowRate sets the annualized borrow fee rate for short positions.
// Default is 0.005 (0.5%).
func WithBorrowRate(rate float64) Option {
    return func(a *Account) {
        a.borrowRate = rate
    }
}
```

- [ ] **Step 8: Add margin and exposure methods to Portfolio interface**

In `portfolio/portfolio.go`, add to the `Portfolio` interface:

```go
// Equity returns cash + long market value - short market value.
Equity() float64

// LongMarketValue returns total market value of long positions.
LongMarketValue() float64

// MarginRatio returns equity / short market value. NaN if no shorts.
MarginRatio() float64

// MarginDeficiency returns dollars needed to restore maintenance margin.
// Returns 0 if not breached.
MarginDeficiency() float64

// ShortMarketValue returns total absolute value of short positions.
ShortMarketValue() float64

// BuyingPower returns cash available accounting for margin requirements.
BuyingPower() float64
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./portfolio/ -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add portfolio/margin.go portfolio/margin_option.go portfolio/account.go portfolio/portfolio.go portfolio/margin_test.go
git commit -m "feat: add margin accounting with configurable rates"
```

---

### Task 7: Borrow Fees and Dividend Debits on Shorts

**Files:**
- Modify: `engine/backtest.go:379-436` (housekeepAccount)
- Modify: `engine/live.go` (housekeeping section)
- Modify: `portfolio/account.go` (add BorrowRate accessor)
- Test: `engine/backtest_test.go`

- [ ] **Step 1: Write failing test for borrow fee recording**

Write an integration test in `engine/backtest_test.go` that runs a backtest with a short position and verifies a FeeTransaction is recorded for borrow costs.

```go
It("records daily borrow fees for short positions", func() {
    // Set up a strategy that shorts an asset on day 1 and holds
    // Test that FeeTransactions appear in the transaction log
    // with amount = abs(qty) * price * (borrowRate / 252)
})
```

The exact test setup should follow existing patterns in `backtest_test.go` using `backtestStrategy` and `mockAssetProvider`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run "borrow fees" -v`
Expected: FAIL

- [ ] **Step 3: Write failing test for short dividend debit**

```go
It("debits cash for dividends on short positions", func() {
    // Set up a strategy that shorts a dividend-paying stock
    // Verify a negative-amount DividendTransaction is recorded
    // with justification containing "short dividend obligation"
})
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./engine/ -run "short dividend" -v`
Expected: FAIL

- [ ] **Step 5: Add BorrowRate accessor to Account**

In `portfolio/account.go`:

```go
// BorrowRate returns the configured annualized borrow fee rate.
func (a *Account) BorrowRate() float64 {
    if a.borrowRate > 0 {
        return a.borrowRate
    }
    return 0.005 // default 0.5%
}
```

- [ ] **Step 6: Add borrow fees and short dividends to housekeepAccount**

In `engine/backtest.go`, modify `housekeepAccount` to add after split processing and before/during dividend recording:

```go
// Record borrow fees for short positions.
if housekeepDF != nil {
    borrowRate := acct.BorrowRate()
    for _, heldAsset := range heldAssets {
        qty := acct.Position(heldAsset)
        if qty >= 0 {
            continue
        }
        closePrice := housekeepDF.ValueAt(heldAsset, data.MetricClose, date)
        if math.IsNaN(closePrice) || closePrice == 0 {
            continue
        }
        dailyFee := math.Abs(qty) * closePrice * (borrowRate / 252.0)
        acct.Record(portfolio.Transaction{
            Date:          date,
            Asset:         heldAsset,
            Type:          portfolio.FeeTransaction,
            Amount:        -dailyFee,
            Justification: fmt.Sprintf("borrow fee: %s %.2f%% annualized", heldAsset.Ticker, borrowRate*100),
        })
    }
}
```

Modify the dividend recording loop to handle shorts:

```go
for _, heldAsset := range heldAssets {
    qty := acct.Position(heldAsset)
    if qty == 0 {
        continue
    }

    divPerShare := housekeepDF.ValueAt(heldAsset, data.Dividend, date)
    if math.IsNaN(divPerShare) || divPerShare <= 0 {
        continue
    }

    if qty > 0 {
        // Long position: receive dividend
        acct.Record(portfolio.Transaction{
            Date:   date,
            Asset:  heldAsset,
            Type:   portfolio.DividendTransaction,
            Amount: divPerShare * qty,
            Qty:    qty,
            Price:  divPerShare,
        })
    } else {
        // Short position: owe dividend
        acct.Record(portfolio.Transaction{
            Date:          date,
            Asset:         heldAsset,
            Type:          portfolio.DividendTransaction,
            Amount:        divPerShare * qty, // negative (qty is negative)
            Qty:           qty,
            Price:         divPerShare,
            Justification: fmt.Sprintf("short dividend obligation: %s ex-date %s", heldAsset.Ticker, date.Format("2006-01-02")),
        })
    }
}
```

- [ ] **Step 7: Apply same changes to live engine**

Apply the borrow fee and short dividend logic to `engine/live.go`.

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./engine/ -v && go test ./portfolio/ -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add engine/backtest.go engine/live.go portfolio/account.go engine/backtest_test.go
git commit -m "feat: add borrow fees and short dividend debits to engine housekeeping"
```

---

### Task 8: Margin Call Detection and MarginCallHandler

**Files:**
- Create: `engine/margin_call.go` (MarginCallHandler interface, check and auto-liquidate functions)
- Modify: `engine/backtest.go:276-369` (main loop -- add margin check step)
- Modify: `engine/live.go` (add margin check step)
- Test: `engine/margin_call_test.go`

- [ ] **Step 1: Write failing test for margin call auto-liquidation**

```go
It("auto-liquidates short positions when maintenance margin is breached", func() {
    // Create a backtest where:
    // 1. Strategy shorts heavily on day 1
    // 2. Price rises significantly, breaching maintenance margin
    // 3. Verify proportional covering occurs automatically
    // 4. Verify margin is restored after auto-liquidation
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run "auto-liquidates" -v`
Expected: FAIL

- [ ] **Step 3: Write failing test for MarginCallHandler interface**

```go
It("calls OnMarginCall when strategy implements MarginCallHandler", func() {
    // Create a strategy implementing MarginCallHandler
    // Verify OnMarginCall is called when margin breaches
    // Verify the strategy can place orders to address the shortfall
})
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./engine/ -run "OnMarginCall" -v`
Expected: FAIL

- [ ] **Step 5: Create margin_call.go**

Create `engine/margin_call.go`:

```go
package engine

import (
    "context"
    "fmt"
    "math"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "github.com/penny-vault/pvbt/broker"
    "github.com/penny-vault/pvbt/portfolio"
    "github.com/rs/zerolog"
)

// MarginCallHandler is an optional interface that strategies may implement
// to handle margin calls. When a margin breach is detected, the engine
// calls OnMarginCall before resorting to automatic liquidation.
type MarginCallHandler interface {
    OnMarginCall(ctx context.Context, eng *Engine, port portfolio.Portfolio, batch *portfolio.Batch) error
}

// checkAndHandleMarginCall checks if maintenance margin is breached and
// handles the margin call via the strategy handler or auto-liquidation.
func (eng *Engine) checkAndHandleMarginCall(ctx context.Context, acct *portfolio.Account, date time.Time) error {
    deficiency := acct.MarginDeficiency()
    if deficiency == 0 {
        return nil
    }

    zerolog.Ctx(ctx).Warn().
        Float64("deficiency", deficiency).
        Float64("margin_ratio", acct.MarginRatio()).
        Msg("margin call triggered")

    // Try strategy handler first.
    if handler, ok := eng.strategy.(MarginCallHandler); ok {
        batch := acct.NewBatch(date)
        batch.SkipMiddleware = true

        if err := handler.OnMarginCall(ctx, eng, acct, batch); err != nil {
            return fmt.Errorf("engine: margin call handler: %w", err)
        }

        if err := acct.ExecuteBatch(ctx, batch); err != nil {
            return fmt.Errorf("engine: execute margin call batch: %w", err)
        }

        // Re-check: if still breached, fall through to auto-liquidation.
        if acct.MarginDeficiency() == 0 {
            return nil
        }
    }

    // Auto-liquidate: cover short positions proportionally.
    return eng.autoLiquidateShorts(ctx, acct, date)
}

// autoLiquidateShorts covers short positions proportionally until
// maintenance margin is restored.
func (eng *Engine) autoLiquidateShorts(ctx context.Context, acct *portfolio.Account, date time.Time) error {
    deficiency := acct.MarginDeficiency()
    if deficiency == 0 {
        return nil
    }

    smv := acct.ShortMarketValue()
    if smv == 0 {
        return nil
    }

    // Calculate fraction of shorts to cover.
    // We need to cover enough to restore margin.
    coverFraction := deficiency / smv
    if coverFraction > 1 {
        coverFraction = 1
    }

    batch := acct.NewBatch(date)
    batch.SkipMiddleware = true

    acct.Holdings(func(ast asset.Asset, qty float64) {
        if qty >= 0 {
            return
        }
        coverQty := math.Ceil(math.Abs(qty) * coverFraction)
        if coverQty > math.Abs(qty) {
            coverQty = math.Abs(qty)
        }
        batch.Orders = append(batch.Orders, broker.Order{
            Asset:         ast,
            Side:          broker.Buy,
            Qty:           coverQty,
            OrderType:     broker.Market,
            TimeInForce:   broker.Day,
            Justification: "margin call auto-liquidation",
        })
    })

    if len(batch.Orders) == 0 {
        return nil
    }

    if err := acct.ExecuteBatch(ctx, batch); err != nil {
        return fmt.Errorf("engine: auto-liquidate shorts: %w", err)
    }

    // Final check.
    if acct.MarginDeficiency() > 0 {
        zerolog.Ctx(ctx).Error().
            Float64("remaining_deficiency", acct.MarginDeficiency()).
            Msg("margin still breached after auto-liquidation")
    }

    return nil
}
```

- [ ] **Step 6: Add margin check to backtest loop**

In `engine/backtest.go`, after housekeeping (line 298) and before the child strategy loop (line 303), add:

```go
// Check margin and handle margin calls (runs every trading day).
if err := eng.checkAndHandleMarginCall(stepCtx, acct, date); err != nil {
    return nil, fmt.Errorf("engine: margin call on %v: %w", date, err)
}
```

- [ ] **Step 7: Add margin check to live engine**

Add the same call in `engine/live.go` after housekeeping and before strategy compute.

- [ ] **Step 8: Run tests**

Run: `go test ./engine/ -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add engine/margin_call.go engine/backtest.go engine/live.go engine/margin_call_test.go
git commit -m "feat: add margin call detection and MarginCallHandler interface"
```

---

### Task 9: Simulated Broker Margin Enforcement

**Files:**
- Modify: `engine/simulated_broker.go:32-36` (add Portfolio reference)
- Modify: `engine/simulated_broker.go:59-92` (Submit -- add margin check)
- Modify: `engine/backtest.go` (pass portfolio to broker)
- Test: `engine/simulated_broker_test.go`

- [ ] **Step 1: Write failing test for margin rejection**

```go
It("rejects short orders that would violate initial margin", func() {
    mockPort := &mockPortfolio{
        cashVal:     10000,
        valueVal:    10000,
        shortMktVal: 0,
        equityVal:   10000,
        positionVal: 0,
    }

    sb := engine.NewSimulatedBroker()
    sb.SetPortfolio(mockPort)
    sb.SetPriceProvider(mockPrices, time.Now())

    // Try to short $50,000 of stock with only $10,000 equity
    // Initial margin requires 50%, so max short = $20,000
    order := broker.Order{
        Asset: testAsset,
        Side:  broker.Sell,
        Qty:   500, // at $100 = $50,000 short
        OrderType: broker.Market,
    }
    order.ID = "test-1"

    err := sb.Submit(context.Background(), order)
    Expect(err).NotTo(HaveOccurred()) // no error, just no fill

    // Should not produce a fill
    select {
    case <-sb.Fills():
        Fail("should not have produced a fill")
    default:
        // expected -- no fill
    }
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run "rejects short orders" -v`
Expected: FAIL -- SetPortfolio does not exist

- [ ] **Step 3: Add Portfolio reference to SimulatedBroker**

In `engine/simulated_broker.go`:

```go
type SimulatedBroker struct {
    prices            broker.PriceProvider
    date              time.Time
    fills             chan broker.Fill
    portfolio         portfolio.Portfolio
    initialMarginRate float64
}

// SetPortfolio sets the portfolio reference for margin checks.
func (b *SimulatedBroker) SetPortfolio(p portfolio.Portfolio) {
    b.portfolio = p
}

// SetInitialMarginRate sets the initial margin rate used for order rejection.
func (b *SimulatedBroker) SetInitialMarginRate(rate float64) {
    b.initialMarginRate = rate
}
```

- [ ] **Step 4: Add initial margin check to Submit()**

In `engine/simulated_broker.go`, modify `Submit()` to check margin before filling sell orders:

```go
// Check initial margin for short-opening sells.
if order.Side == broker.Sell && b.portfolio != nil {
    currentPos := b.portfolio.Position(order.Asset)
    // If this sell would open or increase a short position...
    if currentPos-qty < 0 {
        shortIncrease := qty
        if currentPos > 0 {
            shortIncrease = qty - currentPos // only the short-opening portion
        }
        newShortValue := b.portfolio.ShortMarketValue() + shortIncrease*price
        // Equity is unchanged at short open (cash up, short liability up by same amount).
        // Check: equity / newShortValue >= initialMarginRate
        equity := b.portfolio.Equity()
        initialRate := b.initialMarginRate
        if initialRate == 0 {
            initialRate = 0.50 // default Reg T
        }
        if newShortValue > 0 && equity/newShortValue < initialRate {
            zerolog.Ctx(ctx).Warn().
                Str("asset", order.Asset.Ticker).
                Float64("equity", equity).
                Float64("new_short_value", newShortValue).
                Msg("order rejected: insufficient margin")
            return nil
        }
    }
}
```

- [ ] **Step 5: Wire portfolio to broker in backtest engine**

In `engine/backtest.go`, after creating the simulated broker, set the portfolio reference:

```go
if sb, ok := eng.broker.(*SimulatedBroker); ok {
    sb.SetPortfolio(acct)
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./engine/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add engine/simulated_broker.go engine/backtest.go engine/simulated_broker_test.go
git commit -m "feat: add initial margin enforcement to simulated broker"
```

---

### Task 10: Risk Middleware Short-Awareness

**Files:**
- Modify: `risk/max_position_size.go` (handle short positions)
- Modify: `risk/volatility_scaler.go` (handle short positions symmetrically)
- Create: `risk/exposure_limits.go` (gross/net exposure middleware)
- Test: `risk/max_position_size_test.go`
- Test: `risk/exposure_limits_test.go`

- [ ] **Step 1: Write failing test for position size limit on short orders**

```go
It("limits short position size the same as long", func() {
    // Set up MaxPositionSize middleware at 10%
    // Submit a sell order that would open a short exceeding 10%
    // Verify the order is scaled down
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./risk/ -run "limits short position" -v`
Expected: FAIL

- [ ] **Step 3: Update max_position_size.go to handle shorts**

Modify the `Process` method to check the projected position after each sell order. If the result would be a negative position (short) exceeding the max size, scale it down. Use `batch.Portfolio().Position(order.Asset)` to determine current holdings.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./risk/ -run "limits short position" -v`
Expected: PASS

- [ ] **Step 5: Write failing test for gross exposure limits**

```go
It("rejects orders that would exceed gross exposure limit", func() {
    // Set up GrossExposureLimit at 2.0 (200%)
    // Already have 150% gross exposure
    // Submit an order that would push to 210%
    // Verify order is rejected/scaled
})
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./risk/ -run "gross exposure" -v`
Expected: FAIL

- [ ] **Step 7: Create exposure_limits.go**

```go
package risk

import (
    "context"
    "math"

    "github.com/penny-vault/pvbt/portfolio"
)

type grossExposureLimit struct {
    maxGross float64
}

// GrossExposureLimit returns a middleware that rejects orders when gross
// exposure (long + short market value) / equity would exceed the limit.
func GrossExposureLimit(maxGross float64) portfolio.Middleware {
    return &grossExposureLimit{maxGross: maxGross}
}

func (g *grossExposureLimit) Process(_ context.Context, batch *portfolio.Batch) error {
    port := batch.Portfolio()
    equity := port.Equity()
    if equity <= 0 {
        return nil
    }
    longMV := port.LongMarketValue()
    shortMV := port.ShortMarketValue()
    gross := (longMV + shortMV) / equity
    if gross > g.maxGross {
        // Remove orders that increase exposure
        batch.Orders = filterOrdersByExposure(batch.Orders, port, equity, g.maxGross)
    }
    return nil
}

type netExposureLimit struct {
    maxNet float64
}

// NetExposureLimit returns a middleware that constrains net exposure
// (long - short market value) / equity to the given range.
func NetExposureLimit(maxNet float64) portfolio.Middleware {
    return &netExposureLimit{maxNet: maxNet}
}

func (n *netExposureLimit) Process(_ context.Context, batch *portfolio.Batch) error {
    port := batch.Portfolio()
    equity := port.Equity()
    if equity <= 0 {
        return nil
    }
    longMV := port.LongMarketValue()
    shortMV := port.ShortMarketValue()
    net := math.Abs(longMV-shortMV) / equity
    if net > n.maxNet {
        batch.Orders = filterOrdersByExposure(batch.Orders, port, equity, n.maxNet)
    }
    return nil
}

func filterOrdersByExposure(orders []broker.Order, port portfolio.Portfolio, equity, limit float64) []broker.Order {
    longMV := port.LongMarketValue()
    shortMV := port.ShortMarketValue()

    var filtered []broker.Order
    for _, order := range orders {
        // Estimate the impact of this order on exposure.
        prices := port.Prices()
        if prices == nil {
            filtered = append(filtered, order)
            continue
        }
        price := prices.Value(order.Asset, data.MetricClose)
        if math.IsNaN(price) {
            filtered = append(filtered, order)
            continue
        }

        orderValue := order.Qty * price
        if order.Qty == 0 && order.Amount > 0 {
            orderValue = order.Amount
        }

        projLong := longMV
        projShort := shortMV
        if order.Side == broker.Buy {
            projLong += orderValue
        } else {
            // Check if this sell increases short exposure
            currentPos := port.Position(order.Asset)
            if currentPos-order.Qty < 0 {
                shortIncrease := math.Min(orderValue, math.Abs(currentPos-order.Qty)*price)
                projShort += shortIncrease
            }
        }

        projGross := (projLong + projShort) / equity
        if projGross <= limit {
            filtered = append(filtered, order)
            longMV = projLong
            shortMV = projShort
        }
        // Orders that breach the limit are dropped.
    }
    return filtered
}
```

- [ ] **Step 8: Run tests**

Run: `go test ./risk/ -v`
Expected: PASS

- [ ] **Step 9: Update volatility_scaler.go for short symmetry**

Modify the volatility scaler to apply scaling to short positions using `math.Abs(qty)` for position size calculations.

- [ ] **Step 10: Commit**

```bash
git add risk/max_position_size.go risk/volatility_scaler.go risk/exposure_limits.go risk/max_position_size_test.go risk/exposure_limits_test.go
git commit -m "feat: make risk middleware short-aware with exposure limits"
```

---

### Task 11: Negative Weights in RebalanceTo

**Files:**
- Modify: `portfolio/account.go:136-179` (RebalanceTo on Account)
- Modify: `portfolio/batch.go:138-225` (RebalanceTo on Batch)
- Test: `portfolio/account_test.go`
- Test: `portfolio/batch_test.go`

- [ ] **Step 1: Write failing test for negative weight creating short position**

```go
It("generates sell order for negative weight target", func() {
    acct := buildPricedAccount(100000, testAsset, 0) // no current position
    batch := acct.NewBatch(time.Now())

    alloc := portfolio.Allocation{
        Members: map[asset.Asset]float64{
            testAsset: -0.50, // short 50%
        },
    }

    err := batch.RebalanceTo(context.Background(), alloc)
    Expect(err).NotTo(HaveOccurred())

    Expect(batch.Orders).To(HaveLen(1))
    Expect(batch.Orders[0].Side).To(Equal(broker.Sell))
    Expect(batch.Orders[0].Amount).To(BeNumerically(">", 0))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/ -run "negative weight" -v`
Expected: FAIL -- negative weight produces no orders (diff > 0 check blocks it)

- [ ] **Step 3: Write failing test for covering short positions not in target**

```go
It("covers short positions not in target allocation", func() {
    // Set up account with a short position in MSFT
    // RebalanceTo with only AAPL
    // Verify a buy order is generated for MSFT to cover the short
})
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./portfolio/ -run "covers short positions" -v`
Expected: FAIL -- `qty > 0` guard prevents short position liquidation

- [ ] **Step 5: Update Batch.RebalanceTo() for negative weights**

In `portfolio/batch.go`, modify `RebalanceTo`:

1. Change the unlisted position liquidation to handle shorts:

```go
// Liquidate/cover all holdings not in the target allocation.
b.portfolio.Holdings(func(ast asset.Asset, qty float64) {
    if _, ok := alloc.Members[ast]; !ok && qty != 0 {
        if qty > 0 {
            sells = append(sells, pendingOrder{asset: ast, side: Sell, qty: qty})
        } else {
            // Cover short position not in target
            buys = append(buys, pendingOrder{asset: ast, side: Buy, qty: math.Abs(qty)})
        }
    }
})
```

2. Handle negative weight targets in the overweight/underweight calculation. A negative target means the position should be short. The diff calculation already works: if targetDollars is negative and currentDollars is 0, diff is negative, generating a sell. But the buy loop needs to handle the case where diff is positive for a negative target (covering a short that's too large):

```go
for ast, weight := range alloc.Members {
    targetDollars := weight * totalValue
    currentDollars := b.projectedPositionValue(ast)
    diff := targetDollars - currentDollars

    if diff < 0 {
        sells = append(sells, pendingOrder{asset: ast, side: Sell, amount: -diff})
    }
}

// ... after sells ...

for ast, weight := range alloc.Members {
    targetDollars := weight * postSellValue
    currentDollars := b.projectedPositionValue(ast)
    diff := targetDollars - currentDollars

    if diff > 0 {
        buys = append(buys, pendingOrder{asset: ast, side: Buy, amount: diff})
    }
}
```

Note: `projectedPositionValue` must return negative values for short positions so the delta math works correctly.

- [ ] **Step 6: Update Account.RebalanceTo() identically**

Apply the same changes to `portfolio/account.go:136-179`.

- [ ] **Step 7: Run tests**

Run: `go test ./portfolio/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add portfolio/batch.go portfolio/account.go portfolio/account_test.go portfolio/batch_test.go
git commit -m "feat: support negative weights in RebalanceTo for short positions"
```

---

### Task 12: Long/Short P&L Metrics

**Files:**
- Create: `portfolio/short_win_rate.go`
- Create: `portfolio/long_win_rate.go`
- Create: `portfolio/short_profit_factor.go`
- Create: `portfolio/long_profit_factor.go`
- Modify: `portfolio/metric_registration.go` (register new metrics)
- Modify: `portfolio/trade_metrics.go` (add long/short fields)
- Test: `portfolio/trade_metrics_test.go`

- [ ] **Step 1: Write failing test for ShortWinRate metric**

```go
It("computes win rate for short trades only", func() {
    acct := portfolio.New(portfolio.WithCash(100000))

    // Two short trades: one win, one loss
    // Short AAPL at 150, cover at 140 (win)
    // Short MSFT at 200, cover at 210 (loss)
    // Short win rate should be 0.5

    shortWR, err := acct.PerformanceMetric(portfolio.ShortWinRate).Value()
    Expect(err).NotTo(HaveOccurred())
    Expect(shortWR).To(BeNumerically("~", 0.5, 0.01))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/ -run "short trades only" -v`
Expected: FAIL -- ShortWinRate does not exist

- [ ] **Step 3: Create short_win_rate.go**

```go
package portfolio

type shortWinRate struct{}

func (shortWinRate) Name() string { return "ShortWinRate" }

func (shortWinRate) Description() string {
    return "Percentage of short round-trip trades that were profitable."
}

func (shortWinRate) Compute(a *Account, _ *Period) (float64, error) {
    details := a.TradeDetails()
    var shortTrades []TradeDetail
    for _, td := range details {
        if td.Direction == TradeShort {
            shortTrades = append(shortTrades, td)
        }
    }
    if len(shortTrades) == 0 {
        return 0, nil
    }
    wins := 0
    for _, td := range shortTrades {
        if td.PnL > 0 {
            wins++
        }
    }
    return float64(wins) / float64(len(shortTrades)), nil
}

func (shortWinRate) ComputeSeries(_ *Account, _ *Period) ([]float64, error) { return nil, nil }

var ShortWinRate PerformanceMetric = shortWinRate{}
```

- [ ] **Step 4: Create long_win_rate.go, short_profit_factor.go, long_profit_factor.go**

Follow the same pattern, filtering `TradeDetails()` by `Direction`.

- [ ] **Step 5: Add long/short fields to TradeMetrics struct**

In `portfolio/trade_metrics.go`:

```go
type TradeMetrics struct {
    // ... existing fields ...
    LongWinRate      float64
    ShortWinRate     float64
    LongProfitFactor float64
    ShortProfitFactor float64
}
```

- [ ] **Step 6: Register new metrics**

In `portfolio/metric_registration.go`, add `ShortWinRate`, `LongWinRate`, `ShortProfitFactor`, `LongProfitFactor` to the registration list.

- [ ] **Step 7: Update TradeMetrics() computation**

Update the `TradeMetrics()` method on Account to populate the new fields by querying the new metrics.

- [ ] **Step 8: Run tests**

Run: `go test ./portfolio/ -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add portfolio/short_win_rate.go portfolio/long_win_rate.go portfolio/short_profit_factor.go portfolio/long_profit_factor.go portfolio/metric_registration.go portfolio/trade_metrics.go portfolio/trade_metrics_test.go
git commit -m "feat: add long/short P&L metrics (win rate, profit factor)"
```

---

### Task 13: Add ShortLots to PortfolioSnapshot Interface

**Files:**
- Modify: `portfolio/snapshot.go:37-47` (PortfolioSnapshot interface)
- Test: `portfolio/account_test.go`

- [ ] **Step 1: Add ShortLots to PortfolioSnapshot**

In `portfolio/snapshot.go`, add to `PortfolioSnapshot`:

```go
// ShortLots returns all open short tax lots grouped by asset.
ShortLots(fn func(asset.Asset, []TaxLot))
```

- [ ] **Step 2: Update snapshot restoration logic**

If `WithPortfolioSnapshot` or similar functions exist that restore an account from a snapshot, update them to also restore short lots from the snapshot.

- [ ] **Step 3: Run tests to verify compile and existing behavior**

Run: `go test ./... -v`
Expected: PASS (may need to update any mock implementations of PortfolioSnapshot)

- [ ] **Step 4: Commit**

```bash
git add portfolio/snapshot.go
git commit -m "feat: add ShortLots to PortfolioSnapshot interface"
```

---

### Task 14: End-to-End Integration Test

**Files:**
- Test: `engine/backtest_test.go`

- [ ] **Step 1: Write an end-to-end backtest test with a pairs trading strategy**

Create a test strategy that goes long one asset and short another, verifying:
- Short position is correctly opened and tracked
- Borrow fees are recorded daily
- Short dividend obligations are recorded
- Margin accounting is correct throughout
- Covering the short generates correct P&L
- TradeDetails have correct Direction field
- Long/short metrics are computed correctly

```go
Describe("Short Selling Integration", func() {
    It("runs a complete long/short backtest", func() {
        // Strategy: long AAPL at 50% weight, short MSFT at -50% weight
        // Run for multiple days
        // Verify all accounting is correct
    })
})
```

- [ ] **Step 2: Run the integration test**

Run: `go test ./engine/ -run "Short Selling Integration" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add engine/backtest_test.go
git commit -m "test: add end-to-end short selling integration test"
```

---

### Task 15: Final Lint and Test Pass

**Files:** All modified files

- [ ] **Step 1: Run linter**

Run: `golangci-lint run ./...`
Expected: No new issues. Fix any that appear.

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass

- [ ] **Step 3: Fix any issues found**

Address all lint warnings and test failures.

- [ ] **Step 4: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address lint and test issues from short selling implementation"
```
