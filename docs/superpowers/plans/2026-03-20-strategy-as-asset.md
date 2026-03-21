# Strategy-as-Asset Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable meta-strategies that allocate across child strategies, expanding child holdings into real underlying trades.

**Architecture:** Child strategies are struct fields with `weight` tags, discovered during hydration. The engine manages child portfolios with simulated brokers, merges all schedules into one timeline, runs children before the parent at each frame, and provides `ChildAllocations()` to expand weights into a flat `Allocation` of real assets.

**Tech Stack:** Go, Ginkgo/Gomega (tests), tradecron (schedule merging)

**Spec:** `docs/superpowers/specs/2026-03-20-strategy-as-asset-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `asset/asset.go` | Modify | Add `CashAsset` sentinel |
| `engine/child.go` | Create | `childEntry` struct, `discoverChildren`, preset/params application, cycle detection |
| `engine/child_test.go` | Create | Tests for child discovery, presets, params, weight validation, recursion |
| `engine/child_allocations.go` | Create | `ChildAllocations()` and `ChildPortfolios()` methods on Engine |
| `engine/child_allocations_test.go` | Create | Tests for allocation expansion, $CASH, weight overrides |
| `engine/engine.go` | Modify | Add `children` and `childrenByName` fields to Engine |
| `engine/hydrate.go` | Modify | Skip Strategy-typed fields with `weight` tags during parent hydration |
| `engine/parameter.go` | Modify | Skip Strategy-typed fields in `StrategyParameters` |
| `engine/backtest.go` | Modify | Extract housekeeping helper; wire child discovery; merge child schedules; run children in frames; rename `isStrategy` |
| `engine/warmup.go` | Modify | Include child warmup in max calculation; collect child assets |
| `portfolio/batch.go` | Modify | Skip `$CASH` in `RebalanceTo` |
| `portfolio/account.go` | Modify | Skip `$CASH` in `RebalanceTo` |
| `engine/exports_test.go` | Modify | Test exports for discoverChildren |
| `docs/engine.md` | Modify | Document meta-strategies, ChildAllocations, ChildPortfolios |
| `docs/strategy-guide.md` | Modify | Add meta-strategy authoring guide |
| `CHANGELOG.md` | Modify | Add changelog entry |

---

## Initialization Order in Backtest

For reference, the modified Backtest initialization sequence will be:

1. Load asset registry
2. Load market holidays
3. **Discover children** (extract Strategy-typed fields with `weight` tags, apply presets/params)
4. Hydrate parent fields (skips children already extracted)
5. Build provider routing
6. Call parent `Setup(eng)`
7. For each child: hydrate child fields, call `child.Setup(eng)`, extract schedule, create portfolio
8. Describe() fallback for schedule/benchmark
9. Validate schedule
10. Validate warmup (considering child warmups and child assets)
11. Create parent account
12. ... rest of initialization

Children must be discovered before `hydrateFields` (step 4) so it does not encounter Strategy-typed fields. Children are fully initialized (hydrated, Setup called) before schedule validation so their schedules are available for merging.

---

### Task 1: $CASH sentinel asset

**Files:**
- Modify: `asset/asset.go`

- [ ] **Step 1: Add CashAsset sentinel**

In `asset/asset.go`, add after the `EconomicIndicator` var:

```go
// CashAsset is a sentinel asset representing uninvested cash in a portfolio.
// Used by ChildAllocations to represent a child strategy's cash position.
var CashAsset = Asset{Ticker: "$CASH", CompositeFigi: "$CASH"}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./asset/`

- [ ] **Step 3: Commit**

```bash
git add asset/asset.go
git commit -m "feat: add CashAsset sentinel for strategy-as-asset cash tracking"
```

---

### Task 2: Skip $CASH in RebalanceTo

**Files:**
- Modify: `portfolio/batch.go`
- Modify: `portfolio/account.go`
- Create: `portfolio/cash_skip_test.go`

- [ ] **Step 1: Write failing test for Batch.RebalanceTo skipping $CASH**

Create `portfolio/cash_skip_test.go`. The test needs price data so
`RebalanceTo` can compute order amounts. Use a pre-seeded account with
a position so `priceOf` returns a non-zero value:

```go
var _ = Describe("RebalanceTo $CASH handling", func() {
    It("skips $CASH entries in Batch.RebalanceTo", func() {
        spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}

        acct := New(WithCash(100000, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)))

        // Seed a price DataFrame so priceOf can resolve SPY.
        priceDF, _ := data.NewDataFrame(
            []time.Time{time.Date(2024, 2, 1, 16, 0, 0, 0, time.UTC)},
            []asset.Asset{spy},
            []data.Metric{data.MetricClose},
            data.Daily,
            []float64{100.0},
        )
        acct.UpdatePrices(priceDF)

        batch := acct.NewBatch(time.Date(2024, 2, 1, 16, 0, 0, 0, time.UTC))

        alloc := Allocation{
            Members: map[asset.Asset]float64{
                spy:             0.60,
                asset.CashAsset: 0.40,
            },
        }

        err := batch.RebalanceTo(context.Background(), alloc)
        Expect(err).NotTo(HaveOccurred())

        // Should have orders for SPY, not $CASH
        Expect(batch.Orders).NotTo(BeEmpty(), "expected orders for SPY")
        for _, order := range batch.Orders {
            Expect(order.Asset.Ticker).NotTo(Equal("$CASH"))
        }
    })

    It("skips $CASH entries in Account.RebalanceTo", func() {
        spy := asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}

        simBroker := broker.NewSimulated()
        acct := New(
            WithCash(100000, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
            WithBroker(simBroker),
        )

        // Seed prices.
        priceDF, _ := data.NewDataFrame(
            []time.Time{time.Date(2024, 2, 1, 16, 0, 0, 0, time.UTC)},
            []asset.Asset{spy},
            []data.Metric{data.MetricClose},
            data.Daily,
            []float64{100.0},
        )
        acct.UpdatePrices(priceDF)

        alloc := Allocation{
            Members: map[asset.Asset]float64{
                spy:             0.60,
                asset.CashAsset: 0.40,
            },
        }

        err := acct.RebalanceTo(context.Background(), alloc)
        Expect(err).NotTo(HaveOccurred())

        // Should have transactions for SPY only.
        for _, tx := range acct.Transactions() {
            Expect(tx.Asset.Ticker).NotTo(Equal("$CASH"))
        }
    })
})
```

Note: Adjust broker construction to match the actual API (check if there is a `broker.NewSimulated()` or if the engine's `NewSimulatedBroker()` is the right one).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/ -run "CASH" -v`
Expected: FAIL

- [ ] **Step 3: Add $CASH skip to Batch.RebalanceTo**

In `portfolio/batch.go`, at the top of the `for _, alloc := range allocs` loop body in `RebalanceTo`:

```go
        // Filter out $CASH entries -- cash is the implicit remainder.
        filtered := make(map[asset.Asset]float64, len(alloc.Members))
        for memberAsset, weight := range alloc.Members {
            if memberAsset.Ticker != "$CASH" {
                filtered[memberAsset] = weight
            }
        }

        alloc = Allocation{
            Date:          alloc.Date,
            Members:       filtered,
            Justification: alloc.Justification,
        }
```

- [ ] **Step 4: Add same $CASH skip to Account.RebalanceTo**

Same pattern at the top of the `for _, alloc := range allocs` loop in `portfolio/account.go` `RebalanceTo`.

- [ ] **Step 5: Run tests**

Run: `go test ./portfolio/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add portfolio/batch.go portfolio/account.go portfolio/cash_skip_test.go
git commit -m "feat: skip \$CASH entries in RebalanceTo"
```

---

### Task 3: Extract housekeeping helpers from backtest step loop

**Files:**
- Modify: `engine/backtest.go`

Pure refactor -- no behavior change.

- [ ] **Step 1: Create housekeepAccount helper**

Extract lines 225-277 of `engine/backtest.go` (dividend recording + fill draining) into:

```go
// housekeepAccount records dividends and drains broker fills for an account.
func (e *Engine) housekeepAccount(ctx context.Context, acct *portfolio.Account, date time.Time, benchmark asset.Asset) error {
    // Fetch housekeeping data for held assets.
    // Record dividends.
    // Drain fills.
    // (move the existing code here)
}
```

- [ ] **Step 2: Create updateAccountPrices helper**

Extract lines 304-347 (price fetching, risk-free rate, UpdatePrices) into:

```go
// updateAccountPrices fetches current prices and updates equity for an account.
func (e *Engine) updateAccountPrices(ctx context.Context, acct *portfolio.Account, date time.Time, benchmark asset.Asset) error {
    // Fetch prices for held assets + benchmark.
    // Update risk-free cumulative value.
    // Call acct.UpdatePrices.
    // (move the existing code here)
}
```

- [ ] **Step 3: Replace inlined code with helper calls**

The step loop should call:
```go
if err := e.housekeepAccount(stepCtx, acct, date, e.benchmark); err != nil {
    return nil, err
}
// ... strategy execution ...
if err := e.updateAccountPrices(stepCtx, acct, date, e.benchmark); err != nil {
    return nil, err
}
```

- [ ] **Step 4: Run full engine test suite**

Run: `go test ./engine/ -v`
Expected: PASS (pure refactor)

- [ ] **Step 5: Commit**

```bash
git add engine/backtest.go
git commit -m "refactor: extract housekeeping helpers from backtest step loop"
```

---

### Task 4: Skip Strategy-typed fields in hydration and parameters

**Files:**
- Modify: `engine/hydrate.go`
- Modify: `engine/parameter.go`

- [ ] **Step 1: Add strategyType var to hydrate.go**

In `engine/hydrate.go`, add to the `var` block:

```go
strategyType = reflect.TypeOf((*Strategy)(nil)).Elem()
```

- [ ] **Step 2: Skip Strategy fields in hydrateFields**

In the field loop of `hydrateFields`, add an early continue before the `default` tag check:

```go
        // Skip Strategy-typed fields -- these are children handled by discoverChildren.
        if field.Type.Implements(strategyType) || (field.Type.Kind() == reflect.Pointer && field.Type.Elem().Implements(strategyType)) {
            continue
        }
```

- [ ] **Step 3: Skip Strategy fields in StrategyParameters**

In `engine/parameter.go`, add the same skip in the `StrategyParameters` field loop:

```go
        // Skip Strategy-typed fields -- these are children, not parameters.
        if field.Type.Implements(strategyType) || (field.Type.Kind() == reflect.Pointer && field.Type.Elem().Implements(strategyType)) {
            continue
        }
```

- [ ] **Step 4: Write test**

Add a test in `engine/descriptor_test.go` or `engine/parameter_test.go` verifying that a struct with a Strategy-typed field does not include it in `StrategyParameters` output.

- [ ] **Step 5: Run tests**

Run: `go test ./engine/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add engine/hydrate.go engine/parameter.go engine/descriptor_test.go
git commit -m "feat: skip Strategy-typed fields in hydration and parameter listing"
```

---

### Task 5: Child discovery and initialization

**Files:**
- Create: `engine/child.go`
- Create: `engine/child_test.go`
- Modify: `engine/engine.go`
- Modify: `engine/exports_test.go`

- [ ] **Step 1: Define childEntry struct and add fields to Engine**

Create `engine/child.go` with:

```go
package engine

import (
    "fmt"
    "reflect"
    "strconv"
    "strings"

    "github.com/penny-vault/pvbt/portfolio"
    "github.com/penny-vault/pvbt/tradecron"
)

// childEntry holds a child strategy and its associated runtime state.
type childEntry struct {
    strategy Strategy
    name     string  // pvbt tag value
    weight   float64 // declared weight
    schedule *tradecron.TradeCron
    account  *portfolio.Account
    broker   *SimulatedBroker
}
```

In `engine/engine.go`, add to the Engine struct (in the "populated during initialization" section):

```go
    children       []*childEntry
    childrenByName map[string]*childEntry
```

- [ ] **Step 2: Implement discoverChildren**

In `engine/child.go`. The function:

1. Reflects over the parent strategy's exported fields.
2. For each field whose type implements `Strategy` (directly or as a pointer) and has a `weight` tag:
   a. Parse `weight` as float64.
   b. Read `pvbt` tag for the child name (or kebab-case the field name).
   c. If `preset` tag is present, call `DescribeStrategy(child)` to get `Suggestions`, look up preset, apply values to child struct fields.
   d. If `params` tag is present, parse space-separated `key=value` pairs, apply to child struct fields.
   e. Order: preset first, then params override.
3. Validate weights sum to <= 1.0.
4. Return ordered slice and map.

Application order for preset and params: set the child's struct fields
using reflection before `hydrateFields` runs on the child. Match param
keys to the child's fields by `pvbt` tag name or kebab-cased field name.

Cycle detection: pass a `visited map[uintptr]bool` and check the child
strategy pointer before recursing.

```go
func (e *Engine) discoverChildren(parentStrategy Strategy, visited map[uintptr]bool) error {
    // ... reflection, weight parsing, preset/params, validation
}
```

For recursive meta-strategies: after setting up a child, check if the
child itself has Strategy-typed fields with `weight` tags. If so, recurse
`discoverChildren` on the child. Store grandchildren on the engine in
bottom-up order so the step loop can execute them before their parent.

- [ ] **Step 3: Write tests**

Create `engine/child_test.go` covering:
- Strategy field with `weight` tag is discovered
- Non-strategy fields are ignored
- Weight validation: sum > 1.0 errors
- Weight validation: sum = 0.90 succeeds (implicit 10% cash)
- `preset` tag applies correct suggestion values
- `params` tag overrides individual parameters (space-separated)
- Params applied after preset (preset sets value, params overrides it)
- Missing preset name errors
- Recursive discovery: child with own children discovered bottom-up
- Cycle detection: circular reference errors

- [ ] **Step 4: Add test exports**

In `engine/exports_test.go`, add exports for any internal functions
needed by black-box tests.

- [ ] **Step 5: Run tests**

Run: `go test ./engine/ -run "discoverChildren|childEntry" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add engine/child.go engine/child_test.go engine/engine.go engine/exports_test.go
git commit -m "feat: add child strategy discovery with presets, params, and recursion"
```

---

### Task 6: Wire children into Backtest and merge schedules

**Files:**
- Modify: `engine/backtest.go`

- [ ] **Step 1: Wire discoverChildren into Backtest initialization**

In `Backtest()`, insert the call after loading the asset registry and
holidays (step 1b) but before `hydrateFields` (step 2):

```go
    // 1c. Discover child strategies before hydrating parent.
    e.childrenByName = make(map[string]*childEntry)
    if err := e.discoverChildren(e.strategy, make(map[uintptr]bool)); err != nil {
        return nil, fmt.Errorf("engine: %w", err)
    }
```

After `hydrateFields` on the parent (step 2), hydrate and initialize each
child:

```go
    // 2b. Initialize child strategies.
    for _, child := range e.children {
        if err := hydrateFields(e, child.strategy); err != nil {
            return nil, fmt.Errorf("engine: hydrating child %q: %w", child.name, err)
        }
        child.strategy.Setup(e)

        // Extract schedule from Describe().
        if desc, ok := child.strategy.(Descriptor); ok {
            description := desc.Describe()
            if description.Schedule != "" {
                tc, tcErr := tradecron.New(description.Schedule, tradecron.RegularHours)
                if tcErr != nil {
                    return nil, fmt.Errorf("engine: child %q schedule: %w", child.name, tcErr)
                }
                child.schedule = tc
            }
        }

        // Create child portfolio with simulated broker.
        childBroker := NewSimulatedBroker()
        child.broker = childBroker
        child.account = portfolio.New(
            portfolio.WithCash(100, start),
            portfolio.WithBroker(childBroker),
        )
    }
```

- [ ] **Step 2: Rename isStrategy and extend backtestStep**

```go
type backtestStep struct {
    date             time.Time
    isParentStrategy bool
    childStrategies  []string
}
```

Update all `step.isStrategy` references to `step.isParentStrategy`.

- [ ] **Step 3: Merge child schedules into date enumeration**

After collecting parent strategy dates, add child schedule dates:

```go
    // Collect child strategy dates.
    childCalDates := make(map[string]map[string]bool) // childName -> set of date strings
    for _, child := range e.children {
        if child.schedule == nil {
            continue
        }
        dates := make(map[string]bool)
        childCur := child.schedule.Next(start.Add(-time.Nanosecond))
        for !childCur.After(end) {
            dates[childCur.Format("2006-01-02")] = true
            childCur = child.schedule.Next(childCur.Add(time.Nanosecond))
        }
        childCalDates[child.name] = dates
    }
```

When building steps, populate `childStrategies`:

```go
    cur = dailySchedule.Next(start.Add(-time.Nanosecond))
    for !cur.After(end) {
        calKey := cur.Format("2006-01-02")

        var scheduledChildren []string
        for _, child := range e.children {
            if childCalDates[child.name][calKey] {
                scheduledChildren = append(scheduledChildren, child.name)
            }
        }

        steps = append(steps, backtestStep{
            date:             cur,
            isParentStrategy: parentCalDates[calKey],
            childStrategies:  scheduledChildren,
        })

        cur = dailySchedule.Next(cur.Add(time.Nanosecond))
    }
```

- [ ] **Step 4: Add child execution to step loop**

Before the parent strategy block, add:

```go
    // Run scheduled child strategies (children before parent).
    for _, childName := range step.childStrategies {
        child := e.childrenByName[childName]
        child.broker.SetPriceProvider(e, date)

        if err := child.account.CancelOpenOrders(stepCtx); err != nil {
            return nil, fmt.Errorf("engine: child %q cancel orders on %v: %w", childName, date, err)
        }

        childBatch := child.account.NewBatch(date)
        if err := child.strategy.Compute(stepCtx, e, child.account, childBatch); err != nil {
            return nil, fmt.Errorf("engine: child %q compute on %v: %w", childName, date, err)
        }

        if err := child.account.ExecuteBatch(stepCtx, childBatch); err != nil {
            return nil, fmt.Errorf("engine: child %q execute batch on %v: %w", childName, date, err)
        }
    }
```

After parent price updates, add child housekeeping and price updates at every step:

```go
    // Housekeep and update prices for all child portfolios.
    for _, child := range e.children {
        if err := e.housekeepAccount(stepCtx, child.account, date, asset.Asset{}); err != nil {
            return nil, fmt.Errorf("engine: child %q housekeeping on %v: %w", child.name, date, err)
        }

        if err := e.updateAccountPrices(stepCtx, child.account, date, asset.Asset{}); err != nil {
            return nil, fmt.Errorf("engine: child %q price update on %v: %w", child.name, date, err)
        }
    }
```

- [ ] **Step 5: Run full engine test suite**

Run: `go test ./engine/ -v`
Expected: PASS (existing tests have no children)

- [ ] **Step 6: Commit**

```bash
git add engine/backtest.go
git commit -m "feat: wire child discovery and merge schedules into step loop"
```

---

### Task 7: ChildAllocations and ChildPortfolios

**Files:**
- Create: `engine/child_allocations.go`
- Create: `engine/child_allocations_test.go`

- [ ] **Step 1: Write failing tests**

Test cases:
- No children: returns empty Allocation, no error
- Static weights: expands child holdings (worked example from spec -- ADM 60/40 SPY/SHY at 10%, BAA 100% TLT at 40%, DAA 50/50 SPY/IEF at 50%)
- $CASH: child with 40% cash includes $CASH at correct scaled weight
- All cash: child 100% cash produces only $CASH
- Override weights: dynamic map overrides tag values
- Override validation: weights > 1.0 returns error
- Partial override: override one child, others use tag weight
- Child with no price data (Value() == 0): maps entire weight to $CASH

- [ ] **Step 2: Implement ChildAllocations**

In `engine/child_allocations.go`:

```go
func (e *Engine) ChildAllocations(overrides ...map[string]float64) (portfolio.Allocation, error) {
    if len(e.children) == 0 {
        return portfolio.Allocation{}, nil
    }

    weights := make(map[string]float64, len(e.children))
    for _, child := range e.children {
        weights[child.name] = child.weight
    }

    if len(overrides) > 0 && overrides[0] != nil {
        for name, weight := range overrides[0] {
            weights[name] = weight
        }
    }

    // Validate weight sum.
    totalWeight := 0.0
    for _, weight := range weights {
        totalWeight += weight
    }

    if totalWeight > 1.0+1e-9 {
        return portfolio.Allocation{}, fmt.Errorf(
            "engine: child weights sum to %.4f, must be at most 1.0", totalWeight)
    }

    members := make(map[asset.Asset]float64)

    for _, child := range e.children {
        childWeight := weights[child.name]
        if childWeight == 0 {
            continue
        }

        childValue := child.account.Value()
        if childValue == 0 {
            members[asset.CashAsset] += childWeight
            continue
        }

        cashFraction := child.account.Cash() / childValue
        if cashFraction > 0 {
            members[asset.CashAsset] += childWeight * cashFraction
        }

        child.account.Holdings(func(held asset.Asset, _ float64) {
            posValue := child.account.PositionValue(held)
            posWeight := posValue / childValue

            members[held] += childWeight * posWeight
        })
    }

    return portfolio.Allocation{
        Date:    e.currentDate,
        Members: members,
    }, nil
}

func (e *Engine) ChildPortfolios() map[string]portfolio.Portfolio {
    result := make(map[string]portfolio.Portfolio, len(e.children))
    for _, child := range e.children {
        result[child.name] = child.account
    }

    return result
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./engine/ -run "ChildAllocations|ChildPortfolios" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add engine/child_allocations.go engine/child_allocations_test.go
git commit -m "feat: add ChildAllocations and ChildPortfolios engine methods"
```

---

### Task 8: Warmup integration for children

**Files:**
- Modify: `engine/warmup.go`

- [ ] **Step 1: Write failing test**

A meta-strategy with a child that declares Warmup: 200 while the parent
declares Warmup: 14 should use 200 as the effective warmup.

- [ ] **Step 2: Update validateWarmup**

After extracting the parent warmup, iterate children and take the max:

```go
    for _, child := range e.children {
        if childDesc, ok := child.strategy.(Descriptor); ok {
            childWarmup := childDesc.Describe().Warmup
            if childWarmup > e.warmup {
                e.warmup = childWarmup
            }
        }
    }
```

- [ ] **Step 3: Update collectStrategyAssets to include child assets**

In `engine/warmup.go`, modify `collectStrategyAssets` (or add a wrapper)
to also recurse into child strategy structs and collect their asset/universe
fields.

- [ ] **Step 4: Run tests**

Run: `go test ./engine/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add engine/warmup.go engine/warmup_test.go
git commit -m "feat: include child strategy warmup and assets in validation"
```

---

### Task 9: Integration test -- full meta-strategy backtest

**Files:**
- Create: `engine/meta_strategy_test.go`

- [ ] **Step 1: Create test strategies**

Two simple child strategies:
- `spyStrategy`: buys SPY on every compute date
- `tltStrategy`: buys TLT on every compute date

A meta-strategy:
```go
type testMetaStrategy struct {
    SPYChild *spyStrategy `pvbt:"spy-child" weight:"0.60"`
    TLTChild *tltStrategy `pvbt:"tlt-child" weight:"0.40"`
}
```

- [ ] **Step 2: Write integration tests**

Test cases:
- Run a complete backtest with the meta-strategy
- Verify parent portfolio holds SPY and TLT (not any child-specific assets)
- Verify approximate weights match 60/40 split
- Verify `ChildPortfolios()` returns two entries
- Verify `ChildAllocations()` with overrides works end-to-end

- [ ] **Step 3: Run tests**

Run: `go test ./engine/ -run "MetaStrategy" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add engine/meta_strategy_test.go
git commit -m "test: add meta-strategy integration tests"
```

---

### Task 10: PredictedPortfolio support

**Files:**
- Modify: `engine/engine.go`

- [ ] **Step 1: Clone child portfolios in PredictedPortfolio**

In `PredictedPortfolio`, after cloning the parent account, clone each
child's account and create a temporary `children` slice for the prediction
run. Run scheduled child Computes on the predicted date before the
parent's Compute.

Save and restore the engine's `children` slice around the prediction
(same pattern as saving/restoring `currentDate` and `predicting`).

- [ ] **Step 2: Write test**

Test that `PredictedPortfolio` on a meta-strategy returns a portfolio
with expanded child allocations.

- [ ] **Step 3: Run tests**

Run: `go test ./engine/ -run "PredictedPortfolio" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add engine/engine.go
git commit -m "feat: support child strategies in PredictedPortfolio"
```

---

### Task 11: Documentation and changelog

**Files:**
- Modify: `docs/engine.md`
- Modify: `docs/strategy-guide.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update docs/engine.md**

- Add `ChildAllocations()` and `ChildPortfolios()` to the "Engine methods for strategy authors" section.
- Add a "Meta-strategies" section explaining child strategy fields, `weight` tags, and the execution model (children before parent).
- Document the `$CASH` sentinel behavior.

- [ ] **Step 2: Update docs/strategy-guide.md**

- Add a "Meta-strategies" section after "Engine configuration" showing the full pattern: declaring children with `weight`/`preset`/`params` tags, calling `ChildAllocations()` in Compute, and using `ChildPortfolios()` for dynamic decisions.
- Include the worked example from the spec (10% ADM, 40% BAA, 50% DAA).

- [ ] **Step 3: Update CHANGELOG.md**

Add entry to the Unreleased section under Added. Use a complete sentence with a subject, active voice, combining related items into one bullet.

- [ ] **Step 4: Commit**

```bash
git add docs/engine.md docs/strategy-guide.md CHANGELOG.md
git commit -m "docs: document meta-strategy support and child allocations"
```

---

### Task 12: Lint and final verification

- [ ] **Step 1: Run linter**

Run: `golangci-lint run ./...`
Fix any issues.

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -v`
Expected: All pass.

- [ ] **Step 3: Commit any fixes**

```bash
git add -A
git commit -m "fix: resolve lint issues in strategy-as-asset implementation"
```
