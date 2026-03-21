# Bracket and OCO Order Types Design

## Overview

Add bracket orders and one-cancels-other (OCO) orders as first-class order types. OCO is the core primitive: two linked orders where filling one cancels the other. A bracket order composes an entry order with an OCO pair (stop loss + take profit) that activates when the entry fills.

The design uses a layered approach: the portfolio layer owns group semantics and orchestrates cancellation, while brokers can optionally implement native group submission for atomic execution. The simulated broker gains intrabar fill simulation using high/low price data.

## Core Types

### broker/broker.go

```go
// GroupType identifies the kind of linked order group.
type GroupType int

const (
    GroupOCO     GroupType = iota + 1 // one-cancels-other
    GroupBracket                       // entry + OCO exits
)

// OrderGroup links related orders for coordinated execution.
type OrderGroup struct {
    ID       string    // assigned by account, like order IDs
    Type     GroupType
    OrderIDs []string  // member order IDs
}

// GroupRole identifies an order's role within a group.
type GroupRole int

const (
    RoleEntry      GroupRole = iota + 1
    RoleStopLoss
    RoleTakeProfit
)
```

### New fields on broker.Order

```go
type Order struct {
    // ... existing fields ...

    GroupID   string    // empty if standalone
    GroupRole GroupRole // entry, stop loss, take profit
}
```

### portfolio/order.go -- Exit Targets

```go
// ExitTarget specifies a bracket exit level as either an absolute price or
// a percentage offset from the entry fill price.
type ExitTarget struct {
    AbsolutePrice float64 // used when non-zero
    PercentOffset float64 // used when AbsolutePrice is zero; e.g., -0.05 for -5%
}
```

## Modifier API

Explicit constructors for exit targets avoid ambiguity between prices and percentages:

```go
func StopLossPrice(price float64) ExitTarget
func StopLossPercent(pct float64) ExitTarget   // e.g., -0.05 for -5%
func TakeProfitPrice(price float64) ExitTarget
func TakeProfitPercent(pct float64) ExitTarget // e.g., 0.10 for +10%
```

Order modifiers for bracket and OCO:

```go
// WithBracket attaches stop loss and take profit exits to an entry order.
// The exit legs become an OCO group activated when the entry fills.
func WithBracket(stopLoss, takeProfit ExitTarget) OrderModifier

// OCO creates two linked sell orders from a single batch.Order call.
// The batch expands this into two separate broker.Order entries with a
// shared group ID. When one fills, the other is cancelled.
func OCO(legA, legB OCOLeg) OrderModifier

// OCOLeg defines one side of an OCO pair.
type OCOLeg struct {
    OrderType  broker.OrderType
    Price      float64 // stop price for Stop orders, limit price for Limit orders
}

// Helper constructors for OCO legs:
func StopLeg(price float64) OCOLeg    // creates a Stop order leg
func LimitLeg(price float64) OCOLeg   // creates a Limit order leg
```

### Batch Expansion Mechanism

`batch.Order()` gains a post-modifier expansion step that runs **after** the existing type-switch and `hasLimit`/`hasStop` order-type derivation. The expansion step checks whether an `OCO` modifier was applied. If so, it discards the original order (whose `OrderType` was derived from the normal path) and instead creates two new `broker.Order` entries. Each clone gets its `OrderType` and price fields set directly from the corresponding `OCOLeg`, bypassing the `hasLimit`/`hasStop` derivation entirely. Both orders are tagged with matching group metadata and appended to `b.Orders`.

For `WithBracket`, no expansion happens at batch-build time. The modifier records the exit targets on the entry order (the entry order itself goes through the normal `hasLimit`/`hasStop` derivation as usual). Expansion into OCO legs is deferred to `DrainFills` when the entry fill price is known (for percentage targets) or to `ExecuteBatch` (for absolute price targets). The deferred exit orders also set their `OrderType` and price fields directly from the exit target definitions, not through the modifier type-switch.

### Usage Examples

```go
// Bracket: buy entry with percentage-based exits
batch.Order(spy, portfolio.Buy, 100,
    portfolio.Limit(105),
    portfolio.WithBracket(portfolio.StopLossPercent(-0.05), portfolio.TakeProfitPercent(0.10)),
)

// Bracket: buy entry with absolute price exits
batch.Order(spy, portfolio.Buy, 100,
    portfolio.Limit(105),
    portfolio.WithBracket(portfolio.StopLossPrice(95), portfolio.TakeProfitPrice(115)),
)

// OCO: protect existing position with stop + limit target.
// This single batch.Order call expands into two broker.Order entries
// sharing a group ID. The asset, side, and qty are shared; the order
// type and price differ per leg.
batch.Order(spy, portfolio.Sell, 100,
    portfolio.OCO(portfolio.StopLeg(95), portfolio.LimitLeg(115)),
)
```

### Constraint: Batch-Only

`WithBracket` and `OCO` modifiers are only supported through `batch.Order()`. The direct `Account.Order()` path does not support them. If either modifier is detected in `Account.Order()`, it returns a hard error immediately (e.g., `fmt.Errorf("bracket/OCO modifiers require batch submission")`). It does not silently strip the modifier or proceed without it. Strategies should use batches for all bracket/OCO orders, which is already the preferred submission path.

## Broker Interface Changes

The base `Broker` interface is unchanged. Brokers that support native OCO/bracket group submission implement an optional interface:

```go
// GroupSubmitter is optionally implemented by brokers that support
// native OCO/bracket order groups.
type GroupSubmitter interface {
    SubmitGroup(ctx context.Context, orders []Order, groupType GroupType) error
}
```

The account layer checks at submission time:

```go
if gs, ok := broker.(broker.GroupSubmitter); ok {
    gs.SubmitGroup(ctx, groupOrders, groupType)
} else {
    // submit individually, account manages cancellation
    for _, order := range groupOrders {
        broker.Submit(ctx, order)
    }
}
```

- **SimulatedBroker**: implements `GroupSubmitter`. Tracks groups internally for intrabar fill simulation with stop-loss-wins priority.
- **TastytradeBroker**: will implement `GroupSubmitter` in a follow-on update, mapping to tastytrade's native OCO order type.
- **Brokers that don't implement it**: fallback path -- account submits individually and cancels siblings on fill.

## Account Layer -- Group Tracking and Cancellation

The account gains group tracking alongside the existing `pendingOrders` map:

```go
type Account struct {
    // ... existing fields ...

    pendingGroups map[string]*broker.OrderGroup  // groupID -> group
}
```

### Lifecycle Within a Frame

1. **Batch building**: When `batch.Order()` receives a `WithBracket` modifier, the batch records the entry order plus two pending exit targets. When it receives an `OCO` modifier, the batch **expands the single call into two `broker.Order` entries** sharing the same asset, side, and quantity but with different order types and prices (one per `OCOLeg`). Both orders are tagged with matching group metadata.

2. **ExecuteBatch**: Assigns group IDs (e.g., `group-<timestamp>-<idx>`) and order IDs to all orders including expanded OCO pairs. For brackets with percentage offsets, the exit legs are not yet fully formed -- they need the entry fill price. These are stored as deferred exits in the group. For brackets with absolute prices and for standalone OCO groups, all orders are ready and submitted (via `GroupSubmitter` or individual submit with fallback).

3. **DrainFills**: The drain loop is split into two phases to avoid re-entrancy issues (the simulated broker's `Submit` synchronously pushes fills onto the same channel):

   **Phase 1 -- Collect**: Non-blocking read loop drains all pending fills from the channel into a local slice. Record each fill as a transaction and update `pendingOrders` as today. Collect any deferred bracket exits that need submission (entry fills with pending exit targets).

   **Phase 2 -- Submit deferred exits**: After the drain loop completes, iterate the collected deferred exits. For each:
   - Resolve percentage offsets using the entry fill price.
   - Create the two exit orders (stop loss and take profit), assign order IDs, add to `pendingOrders`.
   - Submit to the broker as an OCO group (via `GroupSubmitter.SubmitGroup` or individual `Submit` calls).
   - Any fills produced by these submissions are picked up on the next `DrainFills` call (at the next housekeeping step), not in this pass.

   **Group handling during Phase 1**:
   - **OCO leg filled (fallback broker)**: Call `broker.Cancel` on each sibling order and remove them from `pendingOrders`.
   - **OCO leg filled (GroupSubmitter broker)**: The broker already cancelled the sibling internally. The account removes all sibling order IDs from `pendingOrders` directly without calling `broker.Cancel`.
   - Remove completed groups from `pendingGroups`.

4. **CancelOpenOrders** (at frame boundary): Iterates `pendingOrders` and calls `broker.Cancel` for each. Also cleans up `pendingGroups` -- cancelling any order in a group cancels all members.

### Cancellation Ownership

The account distinguishes cancellation paths by checking whether the broker implements `GroupSubmitter` (the same check used during submission). `GroupSubmitter` brokers own sibling cancellation internally and only emit a fill for the winning leg. Fallback brokers require the account to call `broker.Cancel` explicitly. See step 3 above for the detailed flow.

**Key invariant**: Group state is always consistent -- if any order in a group is cancelled or filled, the group reacts atomically. The account layer is the single source of truth for group membership.

## Simulated Broker -- Intrabar Fill Simulation

The simulated broker currently fills all orders at the bar's close price. It gains intrabar price checking for OCO/bracket legs.

### Prerequisite: Cancel and Orders Support

The simulated broker currently returns errors for `Cancel()` and returns nil for `Orders()`. Both must be implemented:

- **Cancel(ctx, orderID)**: Remove the order from the simulated broker's internal pending order map and do not emit a fill for it.
- **Orders(ctx)**: Return all submitted-but-unfilled orders from the internal pending map.

These are required because `CancelOpenOrders` (called at frame boundaries) relies on `Orders()` to enumerate open orders and `Cancel()` to remove them. The account-layer OCO fallback path also calls `Cancel()` to remove siblings.

### Fill Evaluation Order Within a Bar

For long positions:

1. Check pending stop loss orders against the bar's **low** price. If low <= stop price, the stop triggers.
2. Check pending take profit orders against the bar's **high** price. If high >= take profit price, the take profit triggers.
3. If both legs of an OCO could trigger on the same bar, **stop loss wins** (pessimistic assumption).
4. All other pending orders (market, limit, stop, stop-limit) continue to fill as today.

For short positions, the logic inverts: stop loss triggers when **high** >= stop price, take profit triggers when **low** <= take profit price. The stop-loss-wins priority applies in both cases.

### Fill Prices for Triggered Orders

- Stop loss: fills at the stop price (assumes the stop was hit and executed at the trigger level).
- Take profit: fills at the take profit price.
- This is a simplification. Real slippage modeling is out of scope.

### Data Requirement

The simulated broker already receives price data via `SetPriceProvider`. It needs access to High and Low metrics in addition to Close. If the data provider doesn't supply high/low, the broker falls back to close-only evaluation (current behavior), and bracket legs can only fill at close if the close price crosses the trigger.

### Group Awareness

The simulated broker implements `GroupSubmitter` and maintains an internal map of active groups. When it fills an OCO leg, it immediately marks the sibling as cancelled and sends only the winning fill on the channel. No fill is emitted for the cancelled leg.

## Middleware Interaction

No changes to the `Middleware` interface. Middleware already receives the full `Batch` and can inspect/modify all orders.

The batch exposes group metadata for middleware access:

```go
// Groups returns the order groups in this batch for middleware inspection.
func (b *Batch) Groups() []OrderGroupSpec

// OrderGroupSpec describes a group before submission (before IDs are assigned).
type OrderGroupSpec struct {
    Type       broker.GroupType
    EntryIndex int         // index into batch orders; -1 for standalone OCO
    StopLoss   ExitTarget
    TakeProfit ExitTarget
}
```

Middleware can:

- Inspect exit targets on bracket orders (e.g., a risk middleware reads the stop loss distance)
- Tighten or widen exit targets (modify the ExitTarget values)
- Reject a bracket entirely by removing the entry order (which removes its deferred exits too)
- Add bracket exits to orders that don't have them (e.g., a mandatory stop-loss middleware)

Middleware sees groups as specs tied to batch order indices. After middleware runs, `ExecuteBatch` converts specs into `broker.OrderGroup` with assigned IDs.

## Testing Strategy

All tests use Ginkgo and Gomega.

### Unit Tests

- **`portfolio/order_test.go`**: Modifier constructors -- `StopLossPrice`, `StopLossPercent`, `TakeProfitPrice`, `TakeProfitPercent`, `WithBracket`, `OCO`. Verify they produce correct `ExitTarget` values and attach to orders properly.
- **`portfolio/batch_test.go`**: Batch accumulates groups correctly. `Groups()` returns correct specs. Removing an entry order cleans up its group.
- **`portfolio/account_test.go`**: Group lifecycle -- bracket entry fill triggers OCO submission, OCO fill cancels sibling, percentage offsets resolve from fill price, `CancelOpenOrders` cleans up groups.
- **`engine/simulated_broker_test.go`**: Intrabar fill simulation -- stop loss triggers on low, take profit triggers on high, stop loss wins when both trigger on same bar, fallback to close-only when high/low unavailable.

### Middleware Tests

- Risk middleware can read and modify exit targets.
- Removing an entry order from a batch also removes its bracket group.

### Integration Tests

- Full backtest with a bracket strategy: entry fills, price drops to stop loss, position exits. Verify transaction log.
- Full backtest where take profit hits first. Verify stop loss is cancelled.
- Same-bar ambiguity: both legs could trigger, verify stop loss wins.
- Multi-frame: bracket persists across bars within a frame, cancelled at frame boundary.

## Scope and Non-Goals

### In Scope

- OCO as the core primitive (two linked orders, one cancels the other)
- Bracket as composition (entry + OCO exits activated on fill)
- Explicit exit target constructors (price and percent variants)
- `GroupSubmitter` optional broker interface
- Simulated broker intrabar fill simulation using high/low
- Stop-loss-wins priority for same-bar ambiguity
- Middleware access to group specs
- Account-layer group tracking and cancellation fallback

### Out of Scope

- More than two legs in an OCO group (pairs only)
- Trailing stops
- Nested brackets (bracket on a bracket leg)
- Slippage modeling beyond filling at trigger price
- Tastytrade `GroupSubmitter` implementation (follow-on update to tastytrade spec)
- Options or multi-leg instrument orders

### Follow-On Work

- Update tastytrade broker spec to implement `GroupSubmitter` using tastytrade's native OCO API
- Trailing stop support could build on the same group infrastructure later
