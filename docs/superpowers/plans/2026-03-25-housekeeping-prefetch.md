# Housekeeping Prefetch and Holdings API Cleanup

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate redundant per-step data fetches for held assets by prefetching housekeeping prices, and clean up the `Holdings` API from callback to map return.

**Architecture:** Change `Holdings()` from a callback to returning `map[asset.Asset]float64` across the `Portfolio`, `PortfolioManager`, and `PortfolioSnapshot` interfaces and all callers. Then add a `prefetchHousekeepingPrices` method to the engine that batch-fetches all metrics needed by `Transactions`, `setMarginPrices`, and `updateAccountPrices` at the start of each step.

**Tech Stack:** Go, Ginkgo/Gomega

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `portfolio/portfolio.go` | Modify | Change `Holdings` signature in `Portfolio` interface |
| `portfolio/snapshot.go` | Modify | Change `Holdings` signature in `PortfolioSnapshot` interface, update `WithPortfolioSnapshot` caller |
| `portfolio/account.go` | Modify | Change `Holdings` implementation to return map |
| `portfolio/batch.go` | Modify | Update 2 Holdings callers in `RebalanceTo` and `ProjectedHoldings` |
| `portfolio/account_test.go` | Modify | Update 3 test callers |
| `portfolio/sqlite_test.go` | Modify | Update 2 test callers |
| `portfolio/substitution_test.go` | Modify | Update 3 test callers |
| `engine/simulated_broker.go` | Modify | Update `Transactions` caller |
| `engine/simulated_broker_test.go` | Modify | Update `mockPortfolio.Holdings` implementation |
| `engine/backtest.go` | Modify | Update `updateAccountPrices` caller, add prefetch call |
| `engine/engine.go` | Modify | Add `prefetchHousekeepingPrices` method |
| `engine/live.go` | Modify | Update Holdings caller |
| `engine/margin_call.go` | Modify | Update 2 Holdings callers |
| `engine/margin_call_test.go` | Modify | Update Holdings caller |
| `engine/child_allocations.go` | Modify | Update Holdings caller |
| `engine/meta_strategy_test.go` | Modify | Update 2 Holdings callers |
| `engine/backtest_test.go` | Modify | Update 2 Holdings callers |
| `engine/middleware/risk/drawdown_circuit_breaker.go` | Modify | Update Holdings caller |
| `engine/middleware/tax/tax_loss_harvester.go` | Modify | Update Holdings caller |
| `study/montecarlo/analyze_test.go` | Modify | Update `fakePortfolio.Holdings` implementation |
| `docs/portfolio.md` | Modify | Update Holdings documentation |
| `docs/strategy-guide.md` | Modify | Update Holdings examples |

---

### Task 1: Change Holdings signature and implementation

This task changes the interface and implementation. All callers will break until updated in subsequent tasks.

**Files:**
- Modify: `portfolio/portfolio.go:54-56`
- Modify: `portfolio/snapshot.go:39`
- Modify: `portfolio/account.go:422-449`

- [ ] **Step 1: Change the Portfolio interface**

In `portfolio/portfolio.go`, change:

```go
// Holdings iterates over all current positions, calling fn with
// each asset and its held quantity.
Holdings(fn func(asset.Asset, float64))
```

to:

```go
// Holdings returns all current positions as a map of asset to quantity.
// When substitutions are active, real assets are mapped to their logical
// originals so strategy code sees canonical asset names.
Holdings() map[asset.Asset]float64
```

- [ ] **Step 2: Change the PortfolioSnapshot interface**

In `portfolio/snapshot.go`, change line 39:

```go
Holdings(func(asset.Asset, float64))
```

to:

```go
Holdings() map[asset.Asset]float64
```

- [ ] **Step 3: Change the Account implementation**

In `portfolio/account.go`, replace the `Holdings` method (lines 422-449):

```go
// Holdings returns all current positions as a map of asset to quantity.
// When substitutions are active, real assets are mapped to their logical
// originals so strategy code sees canonical asset names.
func (a *Account) Holdings() map[asset.Asset]float64 {
	if len(a.substitutions) == 0 {
		result := make(map[asset.Asset]float64, len(a.holdings))
		for ast, qty := range a.holdings {
			result[ast] = qty
		}

		return result
	}

	var asOf time.Time
	if a.prices != nil {
		asOf = a.prices.End()
	}

	logical := make(map[asset.Asset]float64, len(a.holdings))
	for realAsset, qty := range a.holdings {
		key := mapToLogical(realAsset, a.substitutions, asOf)
		logical[key] += qty
	}

	return logical
}
```

- [ ] **Step 4: Verify it compiles (it won't -- callers need updating)**

Run: `go build ./... 2>&1 | head -30`
Expected: compilation errors from all callers that still use the callback signature. This confirms we've identified all call sites.

- [ ] **Step 5: Commit (broken build, will be fixed in next tasks)**

```bash
git add portfolio/portfolio.go portfolio/snapshot.go portfolio/account.go
git commit -m "portfolio: change Holdings from callback to map return (callers broken)"
```

---

### Task 2: Update portfolio package callers

**Files:**
- Modify: `portfolio/batch.go:271, 363`
- Modify: `portfolio/snapshot.go:56`
- Modify: `portfolio/account_test.go:624, 654, 733`
- Modify: `portfolio/sqlite_test.go:135, 139`
- Modify: `portfolio/substitution_test.go:111, 130, 274`

- [ ] **Step 1: Update batch.go RebalanceTo (line 271)**

Change:

```go
b.portfolio.Holdings(func(ast asset.Asset, qty float64) {
	if _, ok := alloc.Members[ast]; !ok && qty != 0 {
		if qty > 0 {
			sells = append(sells, pendingOrder{asset: ast, side: Sell, qty: qty})
		} else {
			coverBuys = append(coverBuys, pendingOrder{asset: ast, side: Buy, qty: math.Abs(qty)})
		}
	}
})
```

to:

```go
for ast, qty := range b.portfolio.Holdings() {
	if _, ok := alloc.Members[ast]; !ok && qty != 0 {
		if qty > 0 {
			sells = append(sells, pendingOrder{asset: ast, side: Sell, qty: qty})
		} else {
			coverBuys = append(coverBuys, pendingOrder{asset: ast, side: Buy, qty: math.Abs(qty)})
		}
	}
}
```

- [ ] **Step 2: Update batch.go ProjectedHoldings (line 363)**

Change:

```go
b.portfolio.Holdings(func(ast asset.Asset, qty float64) {
	holdings[ast] = qty
})
```

to:

```go
for ast, qty := range b.portfolio.Holdings() {
	holdings[ast] = qty
}
```

- [ ] **Step 3: Update snapshot.go WithPortfolioSnapshot (line 56)**

Change:

```go
snap.Holdings(func(ast asset.Asset, qty float64) {
	acct.holdings[ast] = qty
})
```

to:

```go
for ast, qty := range snap.Holdings() {
	acct.holdings[ast] = qty
}
```

- [ ] **Step 4: Update account_test.go (lines 624, 654, 733)**

Line 624 -- change count test:

```go
a.Holdings(func(_ asset.Asset, _ float64) { count++ })
```

to:

```go
count = len(a.Holdings())
```

Lines 654 and 733 -- change map-building tests from:

```go
a.Holdings(func(ast asset.Asset, qty float64) {
	seen[ast] = qty
})
```

to:

```go
seen = a.Holdings()
```

- [ ] **Step 5: Update sqlite_test.go (lines 135, 139)**

Change both:

```go
acct.Holdings(func(a asset.Asset, q float64) {
	original[a] = q
})
```

to:

```go
original = acct.Holdings()
```

And same pattern for `restored.Holdings(...)`.

- [ ] **Step 6: Update substitution_test.go (lines 111, 130, 274)**

Change all three from callback pattern:

```go
acct.Holdings(func(ast asset.Asset, qty float64) {
	seen[ast] = qty
})
```

to:

```go
seen = acct.Holdings()
```

- [ ] **Step 7: Verify portfolio package compiles and tests pass**

Run: `ginkgo run -race ./portfolio/`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add portfolio/batch.go portfolio/snapshot.go portfolio/account_test.go portfolio/sqlite_test.go portfolio/substitution_test.go
git commit -m "portfolio: update all portfolio package Holdings callers"
```

---

### Task 3: Update engine package callers

**Files:**
- Modify: `engine/simulated_broker.go:521`
- Modify: `engine/simulated_broker_test.go:46`
- Modify: `engine/backtest.go:481`
- Modify: `engine/live.go:299`
- Modify: `engine/margin_call.go:79, 118`
- Modify: `engine/margin_call_test.go:83`
- Modify: `engine/child_allocations.go:75`
- Modify: `engine/backtest_test.go:78, 86`
- Modify: `engine/meta_strategy_test.go:66, 114`

- [ ] **Step 1: Update mockPortfolio in simulated_broker_test.go (line 46)**

Change:

```go
func (m *mockPortfolio) Holdings(fn func(asset.Asset, float64)) {
	for ast, qty := range m.holdings {
		fn(ast, qty)
	}
}
```

to:

```go
func (m *mockPortfolio) Holdings() map[asset.Asset]float64 {
	return m.holdings
}
```

- [ ] **Step 2: Update SimulatedBroker.Transactions (line 521)**

Change:

```go
var heldAssets []asset.Asset

b.portfolio.Holdings(func(ast asset.Asset, _ float64) {
	heldAssets = append(heldAssets, ast)
})
```

to:

```go
holdings := b.portfolio.Holdings()
heldAssets := make([]asset.Asset, 0, len(holdings))
for ast := range holdings {
	heldAssets = append(heldAssets, ast)
}
```

- [ ] **Step 3: Update backtest.go updateAccountPrices (line 481)**

Change:

```go
var priceAssets []asset.Asset

acct.Holdings(func(a asset.Asset, _ float64) {
	priceAssets = append(priceAssets, a)
})
```

to:

```go
holdings := acct.Holdings()
priceAssets := make([]asset.Asset, 0, len(holdings))
for ast := range holdings {
	priceAssets = append(priceAssets, ast)
}
```

- [ ] **Step 4: Update live.go (line 299)**

Change:

```go
var priceAssets []asset.Asset

acct.Holdings(func(a asset.Asset, _ float64) {
	priceAssets = append(priceAssets, a)
})
```

to:

```go
holdings := acct.Holdings()
priceAssets := make([]asset.Asset, 0, len(holdings))
for ast := range holdings {
	priceAssets = append(priceAssets, ast)
}
```

- [ ] **Step 5: Update margin_call.go setMarginPrices (line 79)**

Change:

```go
var heldAssets []asset.Asset

acct.Holdings(func(held asset.Asset, _ float64) {
	heldAssets = append(heldAssets, held)
})
```

to:

```go
holdings := acct.Holdings()
heldAssets := make([]asset.Asset, 0, len(holdings))
for ast := range holdings {
	heldAssets = append(heldAssets, ast)
}
```

- [ ] **Step 6: Update margin_call.go autoLiquidateShorts (line 118)**

Change:

```go
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
```

to:

```go
for ast, qty := range acct.Holdings() {
	if qty >= 0 {
		continue
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
}
```

- [ ] **Step 7: Update margin_call_test.go OnMarginCall (line 83)**

Change:

```go
port.Holdings(func(held asset.Asset, qty float64) {
	if qty < 0 {
		batch.Order(ctx, held, portfolio.Buy, -qty)
	}
})
```

to:

```go
for held, qty := range port.Holdings() {
	if qty < 0 {
		batch.Order(ctx, held, portfolio.Buy, -qty)
	}
}
```

- [ ] **Step 8: Update child_allocations.go (line 75)**

Change:

```go
child.account.Holdings(func(held asset.Asset, _ float64) {
	posValue := child.account.PositionValue(held)
	posWeight := posValue / childValue

	members[held] += childWeight * posWeight
})
```

to:

```go
for held := range child.account.Holdings() {
	posValue := child.account.PositionValue(held)
	posWeight := posValue / childValue

	members[held] += childWeight * posWeight
}
```

- [ ] **Step 9: Update backtest_test.go (lines 78 and 86)**

Line 78 -- change:

```go
fund.Holdings(func(held asset.Asset, qty float64) {
	price := priceDF.ValueAt(held, data.MetricClose, eng.CurrentDate())
	if !math.IsNaN(price) {
		totalValue += qty * price
	}
})
```

to:

```go
for held, qty := range fund.Holdings() {
	price := priceDF.ValueAt(held, data.MetricClose, eng.CurrentDate())
	if !math.IsNaN(price) {
		totalValue += qty * price
	}
}
```

Line 86 -- change:

```go
fund.Holdings(func(held asset.Asset, qty float64) {
	inTarget := false
	for _, target := range s.assets {
		if target == held {
			inTarget = true
			break
		}
	}
	if !inTarget && qty > 0 {
		batch.Order(ctx, held, portfolio.Sell, qty)
	}
})
```

to:

```go
for held, qty := range fund.Holdings() {
	inTarget := false
	for _, target := range s.assets {
		if target == held {
			inTarget = true
			break
		}
	}
	if !inTarget && qty > 0 {
		batch.Order(ctx, held, portfolio.Sell, qty)
	}
}
```

- [ ] **Step 10: Update meta_strategy_test.go (lines 66 and 114)**

Both use the same pattern. Change:

```go
fund.Holdings(func(held asset.Asset, qty float64) {
	holdingPrice := priceDF.ValueAt(held, data.MetricClose, eng.CurrentDate())
	if !math.IsNaN(holdingPrice) {
		totalValue += qty * holdingPrice
	}
})
```

to:

```go
for held, qty := range fund.Holdings() {
	holdingPrice := priceDF.ValueAt(held, data.MetricClose, eng.CurrentDate())
	if !math.IsNaN(holdingPrice) {
		totalValue += qty * holdingPrice
	}
}
```

- [ ] **Step 11: Verify engine package compiles and tests pass**

Run: `ginkgo run -race ./engine/`
Expected: PASS

- [ ] **Step 12: Commit**

```bash
git add engine/simulated_broker.go engine/simulated_broker_test.go engine/backtest.go engine/live.go engine/margin_call.go engine/margin_call_test.go engine/child_allocations.go engine/backtest_test.go engine/meta_strategy_test.go
git commit -m "engine: update all engine package Holdings callers"
```

---

### Task 4: Update middleware and study callers

**Files:**
- Modify: `engine/middleware/risk/drawdown_circuit_breaker.go:55`
- Modify: `engine/middleware/tax/tax_loss_harvester.go:70`
- Modify: `study/montecarlo/analyze_test.go:48`

- [ ] **Step 1: Update drawdown_circuit_breaker.go (line 55)**

Change:

```go
batch.Portfolio().Holdings(func(ast asset.Asset, qty float64) {
	if qty > 0 {
		sells = append(sells, broker.Order{
			Asset:       ast,
			Side:        broker.Sell,
			Qty:         qty,
			OrderType:   broker.Market,
			TimeInForce: broker.Day,
		})
	}
})
```

to:

```go
for ast, qty := range batch.Portfolio().Holdings() {
	if qty > 0 {
		sells = append(sells, broker.Order{
			Asset:       ast,
			Side:        broker.Sell,
			Qty:         qty,
			OrderType:   broker.Market,
			TimeInForce: broker.Day,
		})
	}
}
```

- [ ] **Step 2: Update tax_loss_harvester.go (line 70)**

Change:

```go
batch.Portfolio().Holdings(func(ast asset.Asset, qty float64) {
	if qty > 0 {
		heldAssets = append(heldAssets, ast)
	}
})
```

to:

```go
for ast, qty := range batch.Portfolio().Holdings() {
	if qty > 0 {
		heldAssets = append(heldAssets, ast)
	}
}
```

- [ ] **Step 3: Update fakePortfolio in study/montecarlo/analyze_test.go (line 48)**

Change:

```go
func (fp *fakePortfolio) Holdings(_ func(asset.Asset, float64)) {}
```

to:

```go
func (fp *fakePortfolio) Holdings() map[asset.Asset]float64 { return nil }
```

- [ ] **Step 4: Verify all packages compile and tests pass**

Run: `ginkgo run -race ./engine/middleware/... ./study/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add engine/middleware/risk/drawdown_circuit_breaker.go engine/middleware/tax/tax_loss_harvester.go study/montecarlo/analyze_test.go
git commit -m "middleware, study: update remaining Holdings callers"
```

---

### Task 5: Update documentation

**Files:**
- Modify: `docs/portfolio.md`
- Modify: `docs/strategy-guide.md`

- [ ] **Step 1: Update docs/portfolio.md (line 336)**

Find the Holdings example block and change the callback pattern to a range loop:

```go
for ast, qty := range p.Holdings() {
    fmt.Printf("%s: %.2f shares\n", ast.Ticker, qty)
}
```

- [ ] **Step 2: Update docs/strategy-guide.md (lines 661, 682)**

Find both Holdings examples and change from callback to range loop pattern. Example:

```go
for held, qty := range port.Holdings() {
    price := priceDF.ValueAt(held, data.MetricClose, eng.CurrentDate())
    if !math.IsNaN(price) {
        totalValue += qty * price
    }
}
```

- [ ] **Step 3: Commit**

```bash
git add docs/portfolio.md docs/strategy-guide.md
git commit -m "docs: update Holdings examples to map return API"
```

---

### Task 6: Add prefetchHousekeepingPrices

**Files:**
- Modify: `engine/engine.go`
- Modify: `engine/backtest.go`

- [ ] **Step 1: Add `prefetchHousekeepingPrices` method to Engine**

Add to `engine/engine.go`, after the `prefetchBrokerPrices` method:

```go
// prefetchHousekeepingPrices batch-fetches all metrics needed by the
// housekeeping callers (Transactions, setMarginPrices, updateAccountPrices)
// for all held assets, warming the year-chunk cache so each caller's
// individual FetchAt calls are cache hits.
func (e *Engine) prefetchHousekeepingPrices(ctx context.Context, acct portfolio.Portfolio, date time.Time, benchmark asset.Asset) error {
	holdings := acct.Holdings()
	if len(holdings) == 0 && benchmark == (asset.Asset{}) {
		return nil
	}

	assets := make([]asset.Asset, 0, len(holdings)+1)
	for ast := range holdings {
		assets = append(assets, ast)
	}

	if benchmark != (asset.Asset{}) {
		assets = append(assets, benchmark)
	}

	_, err := e.FetchAt(ctx, assets, date, []data.Metric{
		data.MetricClose, data.AdjClose, data.MetricHigh, data.MetricLow,
		data.Volume, data.Dividend, data.SplitFactor,
	})
	if err != nil {
		return fmt.Errorf("prefetch housekeeping prices: %w", err)
	}

	return nil
}
```

- [ ] **Step 2: Call prefetch before housekeeping in backtest loop**

In `engine/backtest.go`, find the housekeeping call for the parent account (around line 339):

```go
// 13-14b. Housekeep parent account (dividends + fill draining).
if err := e.housekeepAccount(stepCtx, acct, date, e.benchmark); err != nil {
	return nil, err
}
```

Insert the prefetch before it:

```go
// Prefetch housekeeping prices for all held assets so that
// Transactions, setMarginPrices, and updateAccountPrices hit cache.
if err := e.prefetchHousekeepingPrices(stepCtx, acct, date, e.benchmark); err != nil {
	return nil, fmt.Errorf("engine: prefetch housekeeping prices on %v: %w", date, err)
}

// 13-14b. Housekeep parent account (dividends + fill draining).
if err := e.housekeepAccount(stepCtx, acct, date, e.benchmark); err != nil {
	return nil, err
}
```

- [ ] **Step 3: Call prefetch before child housekeeping**

Find the child housekeeping loop in `engine/backtest.go` (around line 418):

```go
for _, child := range e.children {
	if err := e.housekeepAccount(stepCtx, child.account, date, asset.Asset{}); err != nil {
		return nil, fmt.Errorf("engine: child %q housekeep on %v: %w", child.name, date, err)
	}
}
```

Insert the prefetch:

```go
for _, child := range e.children {
	if err := e.prefetchHousekeepingPrices(stepCtx, child.account, date, asset.Asset{}); err != nil {
		return nil, fmt.Errorf("engine: child %q prefetch housekeeping on %v: %w", child.name, date, err)
	}

	if err := e.housekeepAccount(stepCtx, child.account, date, asset.Asset{}); err != nil {
		return nil, fmt.Errorf("engine: child %q housekeep on %v: %w", child.name, date, err)
	}
}
```

- [ ] **Step 4: Run full test suite**

Run: `ginkgo run -race --skip-package=broker/ibkr ./...`
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add engine/engine.go engine/backtest.go
git commit -m "engine: prefetch housekeeping prices for all held assets"
```

---

### Task 7: Changelog and lint

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entries**

In the `[Unreleased]` section, add under `### Changed`:

```
- **Breaking:** `Portfolio.Holdings` now returns `map[asset.Asset]float64` instead of taking a callback. Update strategy code from `port.Holdings(func(a asset.Asset, qty float64) { ... })` to `for a, qty := range port.Holdings() { ... }`.
```

Add under `### Fixed`:

```
- Housekeeping data fetches (dividends, splits, margin prices, equity recording) are now batched into a single query per step instead of three separate queries.
```

- [ ] **Step 2: Run linter**

Run: `make lint`
Expected: 0 issues.

- [ ] **Step 3: Run full test suite**

Run: `make test`
Expected: all tests pass.

- [ ] **Step 4: Fix any issues and commit**

```bash
git add CHANGELOG.md
git commit -m "changelog: document Holdings API change and housekeeping prefetch"
```
