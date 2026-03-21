# Strategy-as-Asset Design

## Problem

A meta-strategy should be able to allocate across child strategies the same
way it allocates across assets. For example, a portfolio that is 10% ADM,
40% BAA, and 50% DAA should produce real underlying trades: if ADM is
currently 60% SPY / 40% SHY, the meta-strategy holds 6% SPY and 4% SHY
from the ADM slice.

## Solution

Child strategies are struct fields on the parent with a `weight` tag,
following the same pattern as `asset.Asset` and `universe.Universe` fields.
The engine discovers them during hydration, runs them on their own
schedules, and provides `eng.ChildAllocations()` to expand their weights
into a single `Allocation` of real underlying assets.

## Design

### 1. Declaring children as struct fields

Child strategies are exported fields that implement the `Strategy` interface
and carry a `weight` struct tag:

```go
type MetaStrategy struct {
    ADM *admpkg.ADM `pvbt:"adm" desc:"ADM strategy" weight:"0.10"`
    BAA *baapkg.BAA `pvbt:"baa" desc:"BAA strategy" weight:"0.40"`
    DAA *daapkg.DAA `pvbt:"daa" desc:"DAA strategy" weight:"0.50"`
}

func (s *MetaStrategy) Name() string { return "meta" }
func (s *MetaStrategy) Setup(_ *engine.Engine) {}

func (s *MetaStrategy) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        Schedule: "@monthend",
    }
}

func (s *MetaStrategy) Compute(ctx context.Context, eng *engine.Engine,
    port portfolio.Portfolio, batch *portfolio.Batch) error {
    alloc, err := eng.ChildAllocations()
    if err != nil {
        return err
    }
    return batch.RebalanceTo(ctx, alloc)
}
```

For dynamic weight adjustments the parent can pass overrides:

```go
func (s *MetaStrategy) Compute(ctx context.Context, eng *engine.Engine,
    port portfolio.Portfolio, batch *portfolio.Batch) error {
    children := eng.ChildPortfolios()

    admWeight := 0.10
    admSummary, _ := children["adm"].Summary()
    if admSummary.MaxDrawdown > 0.15 {
        admWeight = 0.05
    }

    alloc, err := eng.ChildAllocations(map[string]float64{
        "adm": admWeight,
        "baa": 0.45,
        "daa": 0.50,
    })
    if err != nil {
        return err
    }
    return batch.RebalanceTo(ctx, alloc)
}
```

### 2. Engine discovery and initialization of children

During backtest initialization the engine detects child strategies before
calling `hydrateFields` on the parent. It reflects over the parent
strategy's exported fields (same flat pattern as `hydrateFields`). For each
field whose type implements the `Strategy` interface and has a `weight` tag,
the engine extracts the child and removes it from the parent's field set so
`hydrateFields` does not attempt to process it.

For each child:

1. Parse the weight from the `weight` tag as `float64`.
2. If a `preset` tag is present, look up the preset name in the child's
   `engine.DescribeStrategy(child).Suggestions` map and apply those
   parameter values to the child's struct fields.
3. Hydrate the child's own struct fields from its `default` tags (after
   preset values have been applied).
4. Call `child.Setup(eng)`. Children share the parent's engine for data
   access (asset registry, data cache, provider routing).
5. Extract the child's schedule from `Describe()`.
6. Create a child portfolio with a simulated broker and an initial deposit
   normalized to 100.
7. Store the child in an ordered slice (for declaration-order iteration)
   and a map keyed by the `pvbt` tag value (for lookup).

After all children are extracted, the engine validates that the declared
`weight` values sum to at most 1.0. If they exceed 1.0, the engine returns
an error. Weights summing to less than 1.0 leave the remainder as implicit
cash.

### 3. Presets and parameter overrides

The `preset` struct tag on the child field selects a named preset:

```go
type MetaStrategy struct {
    ADM *admpkg.ADM `pvbt:"adm" weight:"0.10" preset:"aggressive"`
}
```

During child initialization the engine calls
`engine.DescribeStrategy(child)` to get the `StrategyInfo` (which includes
the `Suggestions` map built from the child's `suggest` tags). It looks up
the preset name in `Suggestions` and applies those values to the child's
struct fields before hydration.

If the preset name is not found in the child's suggestions, the engine
returns an error during initialization.

The `preset` tag is distinct from `suggest` to avoid overloading. `suggest`
on a parameter field declares what presets the field participates in.
`preset` on a child strategy field selects which preset to apply.

Individual parameter overrides use a `params` struct tag with
space-separated `key=value` pairs:

```go
type MetaStrategy struct {
    ADM *admpkg.ADM `pvbt:"adm" weight:"0.10" preset:"aggressive" params:"risk-off=BIL lookback=12"`
}
```

`params` values are applied after preset values and before field hydration.
Parameter keys use the same names as CLI flags (the `pvbt` struct tag or
the kebab-cased field name).

### 4. Merged schedule and step loop

During date enumeration the engine merges all schedules (parent + each
child) into the step timeline. The existing `isStrategy` field on
`backtestStep` is renamed:

```go
type backtestStep struct {
    date             time.Time
    isParentStrategy bool
    childStrategies  []string // pvbt tag names of children scheduled on this date
}
```

A step requires strategy execution if `isParentStrategy` is true OR
`len(childStrategies) > 0`.

The engine sets `e.currentDate` to the step date before running any
strategy Compute calls on that step.

On any given frame:
1. The engine runs each scheduled child's Compute in declaration order,
   with the child's own portfolio and batch. The child's batch executes
   against the child's simulated broker directly (no middleware, since
   middleware is a final-portfolio concern).
2. Then, if the parent is scheduled, the engine runs the parent's Compute
   with the parent's portfolio and batch. The parent's batch goes through
   the normal middleware chain and broker.

Child housekeeping mirrors the parent at every step: dividends are
recorded, broker fills are drained, and prices are updated. To avoid
duplicating the ~50 lines of housekeeping logic in the step loop, this
should be extracted into a helper function
(e.g., `housekeepAccount(ctx, acct, date)`) that both parent and child
portfolios call.

If a child's Compute returns an error, the engine aborts the entire
backtest, same as the parent.

### 5. Recursive meta-strategies

A meta-strategy can itself be a child of a higher-level meta-strategy:

```go
type TopLevel struct {
    Conservative *ConservativeMeta `pvbt:"conservative" weight:"0.60"`
    Aggressive   *AggressiveMeta   `pvbt:"aggressive"   weight:"0.40"`
}
```

Hydration is recursive: when the engine hydrates `Conservative` and finds
it has Strategy-typed fields with `weight` tags, it sets up those
grandchildren too. The engine builds the full tree and executes bottom-up:
grandchildren first, then children, then the parent.

To prevent infinite recursion from circular references (possible through
interface-typed fields), the engine tracks visited strategy pointers and
returns an error if a cycle is detected.

### 6. ChildAllocations and ChildPortfolios

Two new engine methods:

**`eng.ChildAllocations(overrides ...map[string]float64)`** returns a
single `portfolio.Allocation` with the expanded underlying weights.

When called with no arguments, it uses the declared `weight` tag values.
When called with a weight map, those values override the tag weights. Keys
in the map are `pvbt` tag names (e.g., `"adm"`). Any child not in the
override map uses its declared weight. Override weights are validated to
sum to at most 1.0; the method returns an error if they exceed it. This
enables dynamic allocation:

```go
// Static (use declared weights)
alloc, err := eng.ChildAllocations()

// Dynamic (override some or all weights)
alloc, err := eng.ChildAllocations(map[string]float64{
    "adm": 0.20,
    "baa": 0.30,
    "daa": 0.50,
})
```

When called on an engine with no children, it returns an empty
`Allocation` (no members). `RebalanceTo` with an empty allocation sells
all positions, which is the correct behavior if the caller explicitly
asked for child allocations on a non-meta-strategy.

For each child:

1. Get the child's current portfolio weights (position values as fractions
   of the child's total value).
2. Include cash in the weight map using the sentinel asset `$CASH`
   (`asset.Asset{Ticker: "$CASH", CompositeFigi: "$CASH"}`). If a child
   is 60% SPY / 40% cash, its weights are `{SPY: 0.60, $CASH: 0.40}`.
3. Scale each weight by the child's target weight (from tag or override).
4. Merge across all children. Overlapping real assets sum their weights.
   `$CASH` entries also sum.

Both `Batch.RebalanceTo` and `Account.RebalanceTo` skip `$CASH` entries
when generating orders. Unallocated cash stays as cash naturally.

Example: ADM (weight 0.10) is 60% SPY / 40% SHY. BAA (weight 0.40) is
100% TLT. DAA (weight 0.50) is 50% SPY / 50% IEF. The expanded allocation:
- SPY: 0.10 * 0.60 + 0.50 * 0.50 = 0.31
- SHY: 0.10 * 0.40 = 0.04
- TLT: 0.40 * 1.00 = 0.40
- IEF: 0.50 * 0.50 = 0.25
- Total: 1.00

**`eng.ChildPortfolios()`** returns `map[string]portfolio.Portfolio` keyed
by the `pvbt` tag name. The parent can inspect child holdings, weights,
performance metrics, or any other `Portfolio` interface method for dynamic
allocation decisions.

### 7. Warmup interaction

Each child declares its own warmup period in `Describe()`. The engine
computes the effective warmup as `max(parent_warmup, max(child_warmups))`.
The child's warmup already ensures its portfolio is meaningful by the time
the parent's backtest begins, so taking the maximum (not the sum) is
correct. Children start running from the warmup start date.

### 8. PredictedPortfolio

`PredictedPortfolio` clones each child's portfolio alongside the parent.
If any children are scheduled on the predicted date, their Computes run
first (with cloned portfolios and batches), then the parent's Compute runs.
This ensures `ChildAllocations()` returns correct weights during prediction.

### 9. Implementation files

| File | Change |
|------|--------|
| `engine/hydrate.go` | Detect Strategy-typed fields with `weight` tags before hydrating parent; extract children recursively with cycle detection |
| `engine/engine.go` | Add children slice/map to Engine struct; `ChildAllocations()` and `ChildPortfolios()` methods; update `PredictedPortfolio` to clone children |
| `engine/backtest.go` | Merge child schedules into step timeline; rename `isStrategy` to `isParentStrategy`; run children before parent in frames; extract housekeeping into helper function for reuse |
| `engine/warmup.go` | Use max of parent and child warmups in validation |
| `portfolio/batch.go` | Skip `$CASH` entries in `RebalanceTo` |
| `portfolio/account.go` | Skip `$CASH` entries in `RebalanceTo` |
| `asset/asset.go` | Define `CashAsset` sentinel (`$CASH`) |

### 10. What does not change

- The Strategy interface is unchanged. Child strategies implement the same
  interface as any other strategy.
- Data providers, universe, and data fetching are unchanged. Children share
  the parent engine's data layer.
- The parent's middleware chain is unchanged. Children do not go through
  middleware since middleware is a final-portfolio concern, not a strategy
  concern.
- Portfolio and Account interfaces are unchanged.

### 11. Deferred

- Exposing child equity curves as fetchable price series
- `universe.Strategy()` for querying child internals beyond
  `ChildPortfolios()`
