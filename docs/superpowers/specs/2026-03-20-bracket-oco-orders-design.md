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
    RoleEntry      GroupRole = iota
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

// OCO links two orders as one-cancels-other. Used for attaching exits
// to an existing position without an entry order.
func OCO(legA, legB OrderModifier) OrderModifier
```

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

// OCO: protect existing position with stop + limit target
batch.Order(spy, portfolio.Sell, 100,
    portfolio.OCO(portfolio.Stop(95), portfolio.Limit(115)),
)
```

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

1. **Batch building**: When `batch.Order()` receives a `WithBracket` modifier, the batch records the entry order plus two pending exit targets. When it receives an `OCO` modifier, it records two linked orders.

2. **ExecuteBatch**: Assigns group IDs (e.g., `group-<timestamp>-<idx>`). For brackets with percentage offsets, the exit legs are not yet fully formed -- they need the entry fill price. These are stored as deferred exits in the group. For brackets with absolute prices and for standalone OCO groups, all orders are ready and submitted (via `GroupSubmitter` or individual submit with fallback).

3. **DrainFills**: When a fill arrives:
   - Look up `fill.OrderID` in `pendingOrders` as today.
   - If the filled order belongs to a group, check the group type:
     - **Bracket entry filled**: Resolve percentage offsets using the fill price, create the stop loss and take profit orders, submit them as an OCO group.
     - **OCO leg filled**: Cancel all other orders in the group via `broker.Cancel`.
   - Remove completed groups from `pendingGroups`.

4. **CancelOpenOrders** (at frame boundary): Cancels all pending orders including group members. Cleans up `pendingGroups`.

**Key invariant**: Group state is always consistent -- if any order in a group is cancelled or filled, the group reacts atomically. The account layer is the single source of truth for group membership, even when the broker handles native OCO.

## Simulated Broker -- Intrabar Fill Simulation

The simulated broker currently fills all orders at the bar's close price. It gains intrabar price checking for OCO/bracket legs.

### Fill Evaluation Order Within a Bar

1. Check pending stop loss orders against the bar's **low** price. If low <= stop price, the stop triggers.
2. Check pending take profit orders against the bar's **high** price. If high >= take profit price, the take profit triggers.
3. If both legs of an OCO could trigger on the same bar, **stop loss wins** (pessimistic assumption).
4. All other pending orders (market, limit, stop, stop-limit) continue to fill as today.

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
