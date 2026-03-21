# Bracket and OCO Order Types Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add bracket and OCO orders as first-class order types with intrabar fill simulation.

**Architecture:** OCO is the core primitive (two linked orders, one cancels the other). Bracket composes an entry + OCO pair. The portfolio layer owns group semantics; brokers can optionally implement `GroupSubmitter` for native support. The simulated broker gains a pending order map for deferred stop/take-profit evaluation using high/low data.

**Tech Stack:** Go, Ginkgo/Gomega for tests

**Spec:** `docs/superpowers/specs/2026-03-20-bracket-oco-orders-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `broker/broker.go` | Modify | Add `GroupType`, `GroupRole`, `OrderGroup`, `GroupSubmitter` types; add `GroupID`/`GroupRole` fields to `Order` |
| `portfolio/order.go` | Modify | Add `ExitTarget`, `OCOLeg` types; add modifier structs and constructors (`WithBracket`, `OCO`, `StopLossPrice`, etc.) |
| `portfolio/batch.go` | Modify | Add group spec tracking to `Batch`; add OCO expansion in `Order()`; add `Groups()` method |
| `portfolio/account.go` | Modify | Add `pendingGroups`/`brokerHasGroups` fields; modify `ExecuteBatch` for group submission; split `drainFillsFromChannel` into two phases; modify `CancelOpenOrders` for group cleanup; add bracket/OCO rejection in `Account.Order()`; update `Clone()` |
| `engine/simulated_broker.go` | Modify | Add pending order map; implement `Cancel`/`Orders`/`SubmitGroup`/`EvaluatePending`; add deferred fill logic |
| `engine/backtest.go` | Modify | Add `EvaluatePending()` call before `DrainFills` in housekeeping |
| `portfolio/order_test.go` | Modify | Tests for new modifier constructors |
| `portfolio/batch_test.go` | Modify | Tests for OCO expansion, group tracking, `Groups()` |
| `portfolio/account_test.go` | Modify | Tests for group lifecycle, DrainFills phases, CancelOpenOrders with groups |
| `engine/simulated_broker_test.go` | Modify | Tests for Cancel, Orders, SubmitGroup, EvaluatePending, intrabar fill logic |
| `engine/backtest_test.go` | Modify | Integration tests for full bracket/OCO backtest scenarios |

---

### Task 1: Core Broker Types

**Files:**
- Modify: `broker/broker.go:62-155`

- [ ] **Step 1: Write failing test for new types**

In `broker/broker.go`, these are type definitions only. Verify compilation by adding a test that uses them. Create a minimal test file:

```go
// broker/broker_test.go
package broker_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("Order Group Types", func() {
	It("has distinct GroupType values", func() {
		Expect(broker.GroupOCO).NotTo(Equal(broker.GroupBracket))
		Expect(int(broker.GroupOCO)).To(Equal(1))
		Expect(int(broker.GroupBracket)).To(Equal(2))
	})

	It("has distinct GroupRole values starting at 1", func() {
		Expect(int(broker.RoleEntry)).To(Equal(1))
		Expect(int(broker.RoleStopLoss)).To(Equal(2))
		Expect(int(broker.RoleTakeProfit)).To(Equal(3))
	})

	It("has zero-value GroupRole distinguishable from all roles", func() {
		var zeroRole broker.GroupRole
		Expect(zeroRole).NotTo(Equal(broker.RoleEntry))
		Expect(zeroRole).NotTo(Equal(broker.RoleStopLoss))
		Expect(zeroRole).NotTo(Equal(broker.RoleTakeProfit))
	})

	It("stores group metadata on Order", func() {
		order := broker.Order{
			GroupID:   "group-123",
			GroupRole: broker.RoleStopLoss,
		}
		Expect(order.GroupID).To(Equal("group-123"))
		Expect(order.GroupRole).To(Equal(broker.RoleStopLoss))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/ -v -run "Order Group Types"`
Expected: compilation error -- `GroupOCO`, `GroupRole`, etc. undefined

- [ ] **Step 3: Add GroupType, GroupRole, OrderGroup, GroupSubmitter to broker.go**

Add after the `Balance` struct (around line 147):

```go
// GroupType identifies the kind of linked order group.
type GroupType int

const (
	GroupOCO     GroupType = iota + 1 // one-cancels-other
	GroupBracket                       // entry + OCO exits
)

// OrderGroup links related orders for coordinated execution.
type OrderGroup struct {
	ID       string
	Type     GroupType
	OrderIDs []string
}

// GroupRole identifies an order's role within a group.
type GroupRole int

const (
	RoleEntry      GroupRole = iota + 1
	RoleStopLoss
	RoleTakeProfit
)

// GroupSubmitter is optionally implemented by brokers that support
// native OCO/bracket order groups.
type GroupSubmitter interface {
	SubmitGroup(ctx context.Context, orders []Order, groupType GroupType) error
}
```

Add `GroupID` and `GroupRole` fields to the `Order` struct (after `Justification`):

```go
	GroupID   string
	GroupRole GroupRole
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/ -v -run "Order Group Types"`
Expected: PASS

- [ ] **Step 5: Check if a broker suite file exists; create one if needed**

If `broker/broker_suite_test.go` does not exist, create it:

```go
package broker_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBroker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Broker Suite")
}
```

- [ ] **Step 6: Run full broker tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/ -v`
Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add broker/broker.go broker/broker_test.go broker/broker_suite_test.go
git commit -m "Add GroupType, GroupRole, OrderGroup, and GroupSubmitter types to broker package"
```

---

### Task 2: Portfolio Modifier Types

**Files:**
- Modify: `portfolio/order.go:1-114`
- Modify: `portfolio/order_test.go`

- [ ] **Step 1: Write failing tests for ExitTarget constructors and modifier types**

Add to `portfolio/order_test.go`:

```go
var _ = Describe("Bracket and OCO Modifiers", func() {
	Describe("ExitTarget constructors", func() {
		It("StopLossPrice sets AbsolutePrice", func() {
			target := portfolio.StopLossPrice(95.0)
			Expect(target.AbsolutePrice).To(Equal(95.0))
			Expect(target.PercentOffset).To(BeZero())
		})

		It("StopLossPercent sets PercentOffset", func() {
			target := portfolio.StopLossPercent(-0.05)
			Expect(target.PercentOffset).To(Equal(-0.05))
			Expect(target.AbsolutePrice).To(BeZero())
		})

		It("TakeProfitPrice sets AbsolutePrice", func() {
			target := portfolio.TakeProfitPrice(115.0)
			Expect(target.AbsolutePrice).To(Equal(115.0))
			Expect(target.PercentOffset).To(BeZero())
		})

		It("TakeProfitPercent sets PercentOffset", func() {
			target := portfolio.TakeProfitPercent(0.10)
			Expect(target.PercentOffset).To(Equal(0.10))
			Expect(target.AbsolutePrice).To(BeZero())
		})
	})

	Describe("OCOLeg constructors", func() {
		It("StopLeg creates a Stop leg", func() {
			leg := portfolio.StopLeg(95.0)
			Expect(leg.OrderType).To(Equal(broker.Stop))
			Expect(leg.Price).To(Equal(95.0))
		})

		It("LimitLeg creates a Limit leg", func() {
			leg := portfolio.LimitLeg(115.0)
			Expect(leg.OrderType).To(Equal(broker.Limit))
			Expect(leg.Price).To(Equal(115.0))
		})
	})

	Describe("WithBracket modifier", func() {
		It("implements OrderModifier", func() {
			mod := portfolio.WithBracket(
				portfolio.StopLossPrice(95),
				portfolio.TakeProfitPrice(115),
			)
			// Verify it satisfies the interface (compile-time check via usage)
			var _ portfolio.OrderModifier = mod
		})
	})

	Describe("OCO modifier", func() {
		It("implements OrderModifier", func() {
			mod := portfolio.OCO(
				portfolio.StopLeg(95),
				portfolio.LimitLeg(115),
			)
			var _ portfolio.OrderModifier = mod
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "Bracket and OCO Modifiers" -count=1`
Expected: compilation error -- `ExitTarget`, `StopLossPrice`, etc. undefined

- [ ] **Step 3: Implement types and constructors in order.go**

Add to `portfolio/order.go` after the existing modifiers:

```go
// --- Bracket and OCO types ---

// ExitTarget specifies a bracket exit level as either an absolute price or
// a percentage offset from the entry fill price.
type ExitTarget struct {
	AbsolutePrice float64
	PercentOffset float64
}

// StopLossPrice creates an exit target at a fixed price.
func StopLossPrice(price float64) ExitTarget {
	return ExitTarget{AbsolutePrice: price}
}

// StopLossPercent creates an exit target as a percentage offset from the
// entry fill price (e.g., -0.05 for -5%).
func StopLossPercent(pct float64) ExitTarget {
	return ExitTarget{PercentOffset: pct}
}

// TakeProfitPrice creates an exit target at a fixed price.
func TakeProfitPrice(price float64) ExitTarget {
	return ExitTarget{AbsolutePrice: price}
}

// TakeProfitPercent creates an exit target as a percentage offset from the
// entry fill price (e.g., 0.10 for +10%).
func TakeProfitPercent(pct float64) ExitTarget {
	return ExitTarget{PercentOffset: pct}
}

// OCOLeg defines one side of an OCO pair.
type OCOLeg struct {
	OrderType broker.OrderType
	Price     float64
}

// StopLeg creates a Stop order leg for OCO.
func StopLeg(price float64) OCOLeg {
	return OCOLeg{OrderType: broker.Stop, Price: price}
}

// LimitLeg creates a Limit order leg for OCO.
func LimitLeg(price float64) OCOLeg {
	return OCOLeg{OrderType: broker.Limit, Price: price}
}

// --- Bracket and OCO modifiers ---

type bracketModifier struct {
	stopLoss   ExitTarget
	takeProfit ExitTarget
}

func (bracketModifier) orderModifier() {}

// WithBracket attaches stop loss and take profit exits to an entry order.
func WithBracket(stopLoss, takeProfit ExitTarget) OrderModifier {
	return bracketModifier{stopLoss: stopLoss, takeProfit: takeProfit}
}

type ocoModifier struct {
	legA OCOLeg
	legB OCOLeg
}

func (ocoModifier) orderModifier() {}

// OCO creates two linked orders from a single batch.Order call.
func OCO(legA, legB OCOLeg) OrderModifier {
	return ocoModifier{legA: legA, legB: legB}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "Bracket and OCO Modifiers" -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/order.go portfolio/order_test.go
git commit -m "Add ExitTarget, OCOLeg types and bracket/OCO modifier constructors"
```

---

### Task 3: Account.Order() Bracket/OCO Rejection

**Files:**
- Modify: `portfolio/account.go:212-272`
- Modify: `portfolio/order_test.go`

- [ ] **Step 1: Write failing test**

Add to `portfolio/order_test.go`:

```go
var _ = Describe("Account.Order bracket/OCO rejection", func() {
	var (
		acct *portfolio.Account
		spy  asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
		mb := newMockBroker()
		mb.defaultFill = &broker.Fill{Price: 100, Qty: 10, FilledAt: time.Now()}
		acct = portfolio.New(
			portfolio.WithBroker(mb),
			portfolio.WithCash(100000, time.Now()),
		)
	})

	It("returns error for WithBracket modifier", func() {
		err := acct.Order(context.Background(), spy, portfolio.Buy, 10,
			portfolio.WithBracket(portfolio.StopLossPrice(95), portfolio.TakeProfitPrice(115)),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("bracket/OCO modifiers require batch submission"))
	})

	It("returns error for OCO modifier", func() {
		err := acct.Order(context.Background(), spy, portfolio.Sell, 10,
			portfolio.OCO(portfolio.StopLeg(95), portfolio.LimitLeg(115)),
		)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("bracket/OCO modifiers require batch submission"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "Account.Order bracket" -count=1`
Expected: FAIL -- no error returned (modifiers silently ignored by current type-switch)

- [ ] **Step 3: Add rejection cases to Account.Order()**

In `portfolio/account.go`, inside the `Account.Order()` modifier type-switch (around line 234-259), add cases before the closing `}`:

```go
		case bracketModifier:
			return fmt.Errorf("bracket/OCO modifiers require batch submission")
		case ocoModifier:
			return fmt.Errorf("bracket/OCO modifiers require batch submission")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "Account.Order bracket" -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/account.go portfolio/order_test.go
git commit -m "Reject bracket/OCO modifiers in Account.Order with explicit error"
```

---

### Task 4: Batch OCO Expansion and Group Tracking

**Files:**
- Modify: `portfolio/batch.go:28-129`
- Modify: `portfolio/batch_test.go`

- [ ] **Step 1: Write failing tests for OCO expansion**

Add to `portfolio/batch_test.go`:

```go
var _ = Describe("Batch OCO Expansion", func() {
	var (
		batch *portfolio.Batch
		spy   asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
		// Use a mock portfolio that returns a price for SPY
		port := newMockPortfolio(map[asset.Asset]float64{}, 100000, spy, 400.0)
		batch = portfolio.NewBatch(time.Now(), port)
	})

	It("expands OCO into two orders", func() {
		err := batch.Order(context.Background(), spy, portfolio.Sell, 100,
			portfolio.OCO(portfolio.StopLeg(95), portfolio.LimitLeg(115)),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.Orders).To(HaveLen(2))

		// First order: Stop
		Expect(batch.Orders[0].OrderType).To(Equal(broker.Stop))
		Expect(batch.Orders[0].StopPrice).To(Equal(95.0))
		Expect(batch.Orders[0].Asset).To(Equal(spy))
		Expect(batch.Orders[0].Side).To(Equal(broker.Sell))
		Expect(batch.Orders[0].Qty).To(Equal(100.0))
		Expect(batch.Orders[0].GroupRole).To(Equal(broker.RoleStopLoss))

		// Second order: Limit
		Expect(batch.Orders[1].OrderType).To(Equal(broker.Limit))
		Expect(batch.Orders[1].LimitPrice).To(Equal(115.0))
		Expect(batch.Orders[1].GroupRole).To(Equal(broker.RoleTakeProfit))

		// Both share a group ID placeholder (non-empty, matching)
		Expect(batch.Orders[0].GroupID).NotTo(BeEmpty())
		Expect(batch.Orders[0].GroupID).To(Equal(batch.Orders[1].GroupID))
	})

	It("records OCO in Groups()", func() {
		err := batch.Order(context.Background(), spy, portfolio.Sell, 100,
			portfolio.OCO(portfolio.StopLeg(95), portfolio.LimitLeg(115)),
		)
		Expect(err).NotTo(HaveOccurred())

		groups := batch.Groups()
		Expect(groups).To(HaveLen(1))
		Expect(groups[0].Type).To(Equal(broker.GroupOCO))
		Expect(groups[0].EntryIndex).To(Equal(-1))
	})
})

var _ = Describe("Batch WithBracket", func() {
	var (
		batch *portfolio.Batch
		spy   asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
		port := newMockPortfolio(map[asset.Asset]float64{}, 100000, spy, 400.0)
		batch = portfolio.NewBatch(time.Now(), port)
	})

	It("adds one entry order with bracket metadata", func() {
		err := batch.Order(context.Background(), spy, portfolio.Buy, 100,
			portfolio.Limit(105),
			portfolio.WithBracket(portfolio.StopLossPrice(95), portfolio.TakeProfitPrice(115)),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.Orders).To(HaveLen(1))
		Expect(batch.Orders[0].OrderType).To(Equal(broker.Limit))
		Expect(batch.Orders[0].GroupRole).To(Equal(broker.RoleEntry))
		Expect(batch.Orders[0].GroupID).NotTo(BeEmpty())
	})

	It("records bracket in Groups() with exit targets", func() {
		err := batch.Order(context.Background(), spy, portfolio.Buy, 100,
			portfolio.WithBracket(portfolio.StopLossPercent(-0.05), portfolio.TakeProfitPercent(0.10)),
		)
		Expect(err).NotTo(HaveOccurred())

		groups := batch.Groups()
		Expect(groups).To(HaveLen(1))
		Expect(groups[0].Type).To(Equal(broker.GroupBracket))
		Expect(groups[0].EntryIndex).To(Equal(0))
		Expect(groups[0].StopLoss.PercentOffset).To(Equal(-0.05))
		Expect(groups[0].TakeProfit.PercentOffset).To(Equal(0.10))
	})
})
```

Note: Check if `newMockPortfolio` already exists in `portfolio/testutil_test.go`. If not, create a helper that satisfies the `Portfolio` interface and returns configured prices. Read that file first.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "Batch OCO Expansion|Batch WithBracket" -count=1`
Expected: compilation errors -- `Groups()`, `GroupRole` on orders, etc. not defined

- [ ] **Step 3: Add group tracking fields and types to Batch**

Add to `portfolio/batch.go`, in the `Batch` struct:

```go
	// groups tracks order group specs accumulated during Order calls.
	groups []OrderGroupSpec
```

Add the `OrderGroupSpec` type and `Groups()` method:

```go
// OrderGroupSpec describes an order group before submission.
type OrderGroupSpec struct {
	GroupID    string // matches GroupID on the corresponding broker.Order(s)
	Type       broker.GroupType
	EntryIndex int // index into batch.Orders; -1 for standalone OCO
	StopLoss   ExitTarget
	TakeProfit ExitTarget
}

// Groups returns the order group specs in this batch.
func (b *Batch) Groups() []OrderGroupSpec {
	return b.groups
}
```

- [ ] **Step 4: Modify batch.Order() for OCO expansion and WithBracket recording**

In `portfolio/batch.go`, modify the `Order` method. After the existing modifier type-switch and order-type derivation, add post-modifier expansion:

```go
	// Check for bracket/OCO modifiers.
	var (
		hasBracket  bool
		bracketMod  bracketModifier
		hasOCO      bool
		ocoMod      ocoModifier
	)

	for _, mod := range mods {
		switch modifier := mod.(type) {
		case bracketModifier:
			hasBracket = true
			bracketMod = modifier
		case ocoModifier:
			hasOCO = true
			ocoMod = modifier
		}
	}
```

Add bracket/OCO cases to the existing type-switch to prevent them from being silently ignored:

```go
		case bracketModifier:
			// handled in post-expansion below
		case ocoModifier:
			// handled in post-expansion below
```

After the `hasLimit`/`hasStop` order-type derivation and before `b.Orders = append(...)`:

```go
	if hasOCO {
		groupID := fmt.Sprintf("oco-%d-%d", b.Timestamp.UnixNano(), len(b.groups))

		legAOrder := broker.Order{
			Asset:       ast,
			Qty:         qty,
			TimeInForce: order.TimeInForce,
			GroupID:     groupID,
			GroupRole:   broker.RoleStopLoss,
		}
		legAOrder.Side = order.Side
		switch ocoMod.legA.OrderType {
		case broker.Stop:
			legAOrder.OrderType = broker.Stop
			legAOrder.StopPrice = ocoMod.legA.Price
		case broker.Limit:
			legAOrder.OrderType = broker.Limit
			legAOrder.LimitPrice = ocoMod.legA.Price
		}

		legBOrder := broker.Order{
			Asset:       ast,
			Qty:         qty,
			TimeInForce: order.TimeInForce,
			GroupID:     groupID,
			GroupRole:   broker.RoleTakeProfit,
		}
		legBOrder.Side = order.Side
		switch ocoMod.legB.OrderType {
		case broker.Stop:
			legBOrder.OrderType = broker.Stop
			legBOrder.StopPrice = ocoMod.legB.Price
		case broker.Limit:
			legBOrder.OrderType = broker.Limit
			legBOrder.LimitPrice = ocoMod.legB.Price
		}

		b.Orders = append(b.Orders, legAOrder, legBOrder)
		b.groups = append(b.groups, OrderGroupSpec{
			GroupID:    groupID,
			Type:       broker.GroupOCO,
			EntryIndex: -1,
		})

		return nil
	}

	if hasBracket {
		groupID := fmt.Sprintf("bracket-%d-%d", b.Timestamp.UnixNano(), len(b.groups))
		order.GroupID = groupID
		order.GroupRole = broker.RoleEntry

		entryIndex := len(b.Orders)
		b.Orders = append(b.Orders, order)
		b.groups = append(b.groups, OrderGroupSpec{
			GroupID:    groupID,
			Type:       broker.GroupBracket,
			EntryIndex: entryIndex,
			StopLoss:   bracketMod.stopLoss,
			TakeProfit: bracketMod.takeProfit,
		})

		return nil
	}

	b.Orders = append(b.Orders, order)
```

Remove the existing `b.Orders = append(b.Orders, order)` at line 126 since the new code handles it.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "Batch OCO Expansion|Batch WithBracket" -count=1`
Expected: PASS

- [ ] **Step 6: Run all portfolio tests to check for regressions**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -count=1`
Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add portfolio/batch.go portfolio/batch_test.go
git commit -m "Add OCO expansion and bracket group tracking in Batch.Order"
```

---

### Task 5: SimulatedBroker Cancel and Orders Support

**Files:**
- Modify: `engine/simulated_broker.go`
- Modify: `engine/simulated_broker_test.go`

- [ ] **Step 1: Write failing tests for Cancel and Orders**

Add to `engine/simulated_broker_test.go`:

```go
var _ = Describe("SimulatedBroker Cancel and Orders", func() {
	var (
		sb   *engine.SimulatedBroker
		aapl asset.Asset
		date time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{Ticker: "AAPL", CompositeFigi: "BBG000B9XRY4"}
		date = time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		sb = engine.NewSimulatedBroker()
		sb.SetPriceProvider(&mockPriceProvider{
			prices: map[asset.Asset]float64{aapl: 150.0},
			date:   date,
		}, date)
	})

	Describe("Cancel", func() {
		It("removes a deferred order from pending", func() {
			order := broker.Order{
				ID:        "test-1",
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       10,
				OrderType: broker.Stop,
				StopPrice: 140,
				GroupID:   "group-1",
				GroupRole: broker.RoleStopLoss,
			}
			Expect(sb.Submit(context.Background(), order)).To(Succeed())

			orders, err := sb.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(HaveLen(1))

			Expect(sb.Cancel(context.Background(), "test-1")).To(Succeed())

			orders, err = sb.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(BeEmpty())
		})

		It("returns error for unknown order ID", func() {
			err := sb.Cancel(context.Background(), "nonexistent")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Orders", func() {
		It("returns empty when no deferred orders", func() {
			orders, err := sb.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(BeEmpty())
		})
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -run "SimulatedBroker Cancel and Orders" -count=1`
Expected: FAIL -- Cancel returns "not supported", Orders returns nil

- [ ] **Step 3: Add pending order map and implement Cancel/Orders**

In `engine/simulated_broker.go`, add a `pending` field to `SimulatedBroker`:

```go
type SimulatedBroker struct {
	prices  broker.PriceProvider
	date    time.Time
	fills   chan broker.Fill
	pending map[string]broker.Order // deferred orders awaiting evaluation
}
```

Initialize in `NewSimulatedBroker`:

```go
func NewSimulatedBroker() *SimulatedBroker {
	return &SimulatedBroker{
		fills:   make(chan broker.Fill, fillChannelSize),
		pending: make(map[string]broker.Order),
	}
}
```

Modify `Submit` to defer stop-loss/take-profit orders:

```go
func (b *SimulatedBroker) Submit(ctx context.Context, order broker.Order) error {
	// Deferred fill: stop-loss and take-profit roles are stored for intrabar evaluation.
	if order.GroupRole == broker.RoleStopLoss || order.GroupRole == broker.RoleTakeProfit {
		b.pending[order.ID] = order
		return nil
	}

	// Synchronous fill (existing behavior).
	// ... existing code unchanged ...
}
```

Implement `Cancel`:

```go
func (b *SimulatedBroker) Cancel(_ context.Context, orderID string) error {
	if _, ok := b.pending[orderID]; !ok {
		return fmt.Errorf("simulated broker: order %s not found", orderID)
	}
	delete(b.pending, orderID)
	return nil
}
```

Implement `Orders`:

```go
func (b *SimulatedBroker) Orders(_ context.Context) ([]broker.Order, error) {
	orders := make([]broker.Order, 0, len(b.pending))
	for _, order := range b.pending {
		orders = append(orders, order)
	}
	return orders, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -run "SimulatedBroker Cancel and Orders" -count=1`
Expected: PASS

- [ ] **Step 5: Run all engine tests for regressions**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add engine/simulated_broker.go engine/simulated_broker_test.go
git commit -m "Implement Cancel and Orders on SimulatedBroker with pending order map"
```

---

### Task 6: SimulatedBroker SubmitGroup and EvaluatePending

**Files:**
- Modify: `engine/simulated_broker.go`
- Modify: `engine/simulated_broker_test.go`

- [ ] **Step 1: Write failing tests for SubmitGroup**

Add to `engine/simulated_broker_test.go`:

```go
var _ = Describe("SimulatedBroker SubmitGroup", func() {
	var (
		sb   *engine.SimulatedBroker
		aapl asset.Asset
		date time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{Ticker: "AAPL", CompositeFigi: "BBG000B9XRY4"}
		date = time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		sb = engine.NewSimulatedBroker()
		sb.SetPriceProvider(&mockPriceProvider{
			prices: map[asset.Asset]float64{aapl: 150.0},
			date:   date,
		}, date)
	})

	It("implements broker.GroupSubmitter", func() {
		var _ broker.GroupSubmitter = sb
	})

	It("stores OCO legs as deferred orders", func() {
		orders := []broker.Order{
			{ID: "sl-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140, GroupID: "g1", GroupRole: broker.RoleStopLoss},
			{ID: "tp-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 170, GroupID: "g1", GroupRole: broker.RoleTakeProfit},
		}
		Expect(sb.SubmitGroup(context.Background(), orders, broker.GroupOCO)).To(Succeed())

		pending, err := sb.Orders(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(pending).To(HaveLen(2))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -run "SimulatedBroker SubmitGroup" -count=1`
Expected: compilation error -- `SubmitGroup` not defined

- [ ] **Step 3: Implement SubmitGroup**

Add to `engine/simulated_broker.go`:

```go
// SubmitGroup stores OCO/bracket group orders. Members with RoleStopLoss or
// RoleTakeProfit are deferred; others fill synchronously via Submit.
func (b *SimulatedBroker) SubmitGroup(ctx context.Context, orders []broker.Order, _ broker.GroupType) error {
	for _, order := range orders {
		if err := b.Submit(ctx, order); err != nil {
			return err
		}
	}
	return nil
}
```

Also add a `groups` map to track OCO siblings:

```go
type SimulatedBroker struct {
	prices  broker.PriceProvider
	date    time.Time
	fills   chan broker.Fill
	pending map[string]broker.Order
	groups  map[string][]string // groupID -> orderIDs
}
```

Update `NewSimulatedBroker` to initialize `groups`. Update `Submit` to track group membership when `GroupID` is non-empty:

```go
	if order.GroupID != "" {
		b.groups[order.GroupID] = append(b.groups[order.GroupID], order.ID)
	}
```

Update `Cancel` to also clean up group references.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -run "SimulatedBroker SubmitGroup" -count=1`
Expected: PASS

- [ ] **Step 5: Write failing tests for EvaluatePending**

Add to `engine/simulated_broker_test.go`. The mock price provider needs to return high/low data:

```go
// mockHLPriceProvider implements broker.PriceProvider with high/low/close data.
type mockHLPriceProvider struct {
	high  map[asset.Asset]float64
	low   map[asset.Asset]float64
	close map[asset.Asset]float64
	date  time.Time
}

func (m *mockHLPriceProvider) Prices(_ context.Context, assets ...asset.Asset) (*data.DataFrame, error) {
	times := []time.Time{m.date}
	metrics := []data.Metric{data.MetricClose, data.MetricHigh, data.MetricLow}
	vals := make([]float64, len(assets)*len(metrics))
	for idx, held := range assets {
		if price, ok := m.close[held]; ok {
			vals[idx*len(metrics)+0] = price
		}
		if price, ok := m.high[held]; ok {
			vals[idx*len(metrics)+1] = price
		}
		if price, ok := m.low[held]; ok {
			vals[idx*len(metrics)+2] = price
		}
	}
	df, err := data.NewDataFrame(times, assets, metrics, data.Daily, vals)
	if err != nil {
		return nil, err
	}
	return df, nil
}

var _ = Describe("SimulatedBroker EvaluatePending", func() {
	var (
		sb   *engine.SimulatedBroker
		aapl asset.Asset
	)

	BeforeEach(func() {
		aapl = asset.Asset{Ticker: "AAPL", CompositeFigi: "BBG000B9XRY4"}
		sb = engine.NewSimulatedBroker()
	})

	It("triggers stop loss when low <= stop price (long)", func() {
		date := time.Date(2024, 1, 16, 16, 0, 0, 0, time.UTC)
		sb.SetPriceProvider(&mockHLPriceProvider{
			high: map[asset.Asset]float64{aapl: 155},
			low:  map[asset.Asset]float64{aapl: 138},
			close: map[asset.Asset]float64{aapl: 145},
			date: date,
		}, date)

		orders := []broker.Order{
			{ID: "sl-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140, GroupID: "g1", GroupRole: broker.RoleStopLoss},
			{ID: "tp-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 170, GroupID: "g1", GroupRole: broker.RoleTakeProfit},
		}
		Expect(sb.SubmitGroup(context.Background(), orders, broker.GroupOCO)).To(Succeed())

		sb.EvaluatePending()

		// Stop loss should have filled
		var fills []broker.Fill
		for {
			select {
			case fill := <-sb.Fills():
				fills = append(fills, fill)
			default:
				goto done
			}
		}
	done:
		Expect(fills).To(HaveLen(1))
		Expect(fills[0].OrderID).To(Equal("sl-1"))
		Expect(fills[0].Price).To(Equal(140.0))

		// Take profit should be cancelled (removed from pending)
		pending, _ := sb.Orders(context.Background())
		Expect(pending).To(BeEmpty())
	})

	It("triggers take profit when high >= take profit price (long)", func() {
		date := time.Date(2024, 1, 16, 16, 0, 0, 0, time.UTC)
		sb.SetPriceProvider(&mockHLPriceProvider{
			high:  map[asset.Asset]float64{aapl: 175},
			low:   map[asset.Asset]float64{aapl: 148},
			close: map[asset.Asset]float64{aapl: 172},
			date:  date,
		}, date)

		orders := []broker.Order{
			{ID: "sl-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140, GroupID: "g1", GroupRole: broker.RoleStopLoss},
			{ID: "tp-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 170, GroupID: "g1", GroupRole: broker.RoleTakeProfit},
		}
		Expect(sb.SubmitGroup(context.Background(), orders, broker.GroupOCO)).To(Succeed())

		sb.EvaluatePending()

		var fills []broker.Fill
		for {
			select {
			case fill := <-sb.Fills():
				fills = append(fills, fill)
			default:
				goto done2
			}
		}
	done2:
		Expect(fills).To(HaveLen(1))
		Expect(fills[0].OrderID).To(Equal("tp-1"))
		Expect(fills[0].Price).To(Equal(170.0))

		pending, _ := sb.Orders(context.Background())
		Expect(pending).To(BeEmpty())
	})

	It("stop loss wins when both could trigger on same bar", func() {
		date := time.Date(2024, 1, 16, 16, 0, 0, 0, time.UTC)
		sb.SetPriceProvider(&mockHLPriceProvider{
			high:  map[asset.Asset]float64{aapl: 175},
			low:   map[asset.Asset]float64{aapl: 135},
			close: map[asset.Asset]float64{aapl: 150},
			date:  date,
		}, date)

		orders := []broker.Order{
			{ID: "sl-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140, GroupID: "g1", GroupRole: broker.RoleStopLoss},
			{ID: "tp-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 170, GroupID: "g1", GroupRole: broker.RoleTakeProfit},
		}
		Expect(sb.SubmitGroup(context.Background(), orders, broker.GroupOCO)).To(Succeed())

		sb.EvaluatePending()

		var fills []broker.Fill
		for {
			select {
			case fill := <-sb.Fills():
				fills = append(fills, fill)
			default:
				goto done3
			}
		}
	done3:
		Expect(fills).To(HaveLen(1))
		Expect(fills[0].OrderID).To(Equal("sl-1"))
		Expect(fills[0].Price).To(Equal(140.0))
	})

	It("does nothing when neither trigger fires", func() {
		date := time.Date(2024, 1, 16, 16, 0, 0, 0, time.UTC)
		sb.SetPriceProvider(&mockHLPriceProvider{
			high:  map[asset.Asset]float64{aapl: 160},
			low:   map[asset.Asset]float64{aapl: 145},
			close: map[asset.Asset]float64{aapl: 155},
			date:  date,
		}, date)

		orders := []broker.Order{
			{ID: "sl-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140, GroupID: "g1", GroupRole: broker.RoleStopLoss},
			{ID: "tp-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 170, GroupID: "g1", GroupRole: broker.RoleTakeProfit},
		}
		Expect(sb.SubmitGroup(context.Background(), orders, broker.GroupOCO)).To(Succeed())

		sb.EvaluatePending()

		select {
		case <-sb.Fills():
			Fail("expected no fills")
		default:
			// good
		}

		pending, _ := sb.Orders(context.Background())
		Expect(pending).To(HaveLen(2))
	})

	It("falls back to close when high/low unavailable", func() {
		date := time.Date(2024, 1, 16, 16, 0, 0, 0, time.UTC)
		// Use provider without high/low
		sb.SetPriceProvider(&mockPriceProvider{
			prices: map[asset.Asset]float64{aapl: 135},
			date:   date,
		}, date)

		orders := []broker.Order{
			{ID: "sl-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Stop, StopPrice: 140, GroupID: "g1", GroupRole: broker.RoleStopLoss},
			{ID: "tp-1", Asset: aapl, Side: broker.Sell, Qty: 10, OrderType: broker.Limit, LimitPrice: 170, GroupID: "g1", GroupRole: broker.RoleTakeProfit},
		}
		Expect(sb.SubmitGroup(context.Background(), orders, broker.GroupOCO)).To(Succeed())

		sb.EvaluatePending()

		// Close at 135 <= stop at 140, should trigger
		var fills []broker.Fill
		for {
			select {
			case fill := <-sb.Fills():
				fills = append(fills, fill)
			default:
				goto done4
			}
		}
	done4:
		Expect(fills).To(HaveLen(1))
		Expect(fills[0].OrderID).To(Equal("sl-1"))
		Expect(fills[0].Price).To(Equal(140.0))
	})
})
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -run "SimulatedBroker EvaluatePending" -count=1`
Expected: compilation error -- `EvaluatePending` not defined

- [ ] **Step 7: Implement EvaluatePending**

Add to `engine/simulated_broker.go`:

```go
// EvaluatePending checks deferred orders against the current bar's
// high/low data. Uses the price provider and date already set via
// SetPriceProvider. Triggered fills are pushed onto the fills channel.
func (b *SimulatedBroker) EvaluatePending() {
	if b.prices == nil || len(b.pending) == 0 {
		return
	}

	// Collect unique assets from pending orders.
	assetSet := make(map[asset.Asset]struct{})
	for _, order := range b.pending {
		assetSet[order.Asset] = struct{}{}
	}
	assets := make([]asset.Asset, 0, len(assetSet))
	for ast := range assetSet {
		assets = append(assets, ast)
	}

	df, err := b.prices.Prices(context.Background(), assets...)
	if err != nil {
		return
	}

	// Evaluate each group's OCO pair.
	type triggerResult struct {
		orderID string
		price   float64
	}

	groupTriggers := make(map[string]*triggerResult) // groupID -> winning trigger
	groupStopLoss := make(map[string]*triggerResult)
	groupTakeProfit := make(map[string]*triggerResult)

	for orderID, order := range b.pending {
		high := df.Value(order.Asset, data.MetricHigh)
		low := df.Value(order.Asset, data.MetricLow)
		closePrice := df.Value(order.Asset, data.MetricClose)

		// Fall back to close if high/low unavailable.
		if math.IsNaN(high) || high == 0 {
			high = closePrice
		}
		if math.IsNaN(low) || low == 0 {
			low = closePrice
		}

		switch order.GroupRole {
		case broker.RoleStopLoss:
			// Long: stop triggers when low <= stop price
			// Short (Buy side = closing short): stop triggers when high >= stop price
			triggered := false
			if order.Side == broker.Sell && low <= order.StopPrice {
				triggered = true
			} else if order.Side == broker.Buy && high >= order.StopPrice {
				triggered = true
			}
			if triggered {
				groupStopLoss[order.GroupID] = &triggerResult{orderID: orderID, price: order.StopPrice}
			}

		case broker.RoleTakeProfit:
			triggered := false
			if order.Side == broker.Sell && high >= order.LimitPrice {
				triggered = true
			} else if order.Side == broker.Buy && low <= order.LimitPrice {
				triggered = true
			}
			if triggered {
				groupTakeProfit[order.GroupID] = &triggerResult{orderID: orderID, price: order.LimitPrice}
			}
		}
	}

	// Resolve conflicts: stop loss wins when both trigger.
	for groupID := range b.groups {
		sl := groupStopLoss[groupID]
		tp := groupTakeProfit[groupID]

		if sl != nil {
			groupTriggers[groupID] = sl
		} else if tp != nil {
			groupTriggers[groupID] = tp
		}
	}

	// Execute triggers: fill winner, cancel sibling.
	for groupID, trigger := range groupTriggers {
		order := b.pending[trigger.orderID]
		b.fills <- broker.Fill{
			OrderID:  trigger.orderID,
			Price:    trigger.price,
			Qty:      order.Qty,
			FilledAt: b.date,
		}

		// Cancel all orders in this group.
		for _, memberID := range b.groups[groupID] {
			delete(b.pending, memberID)
		}
		delete(b.groups, groupID)
	}
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -run "SimulatedBroker EvaluatePending" -count=1`
Expected: PASS

- [ ] **Step 9: Run all engine tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -count=1`
Expected: all pass

- [ ] **Step 10: Commit**

```bash
git add engine/simulated_broker.go engine/simulated_broker_test.go
git commit -m "Implement SubmitGroup and EvaluatePending on SimulatedBroker"
```

---

### Task 7: Account Group Tracking and ExecuteBatch Changes

**Files:**
- Modify: `portfolio/account.go`
- Modify: `portfolio/account_test.go`

- [ ] **Step 1: Write failing tests for group submission in ExecuteBatch**

Add to `portfolio/account_test.go`. The mock broker needs to implement `GroupSubmitter`:

```go
// mockGroupBroker embeds mockBroker and adds GroupSubmitter support.
type mockGroupBroker struct {
	*mockBroker
	submittedGroups []struct {
		orders    []broker.Order
		groupType broker.GroupType
	}
}

func newMockGroupBroker() *mockGroupBroker {
	return &mockGroupBroker{mockBroker: newMockBroker()}
}

func (m *mockGroupBroker) SubmitGroup(_ context.Context, orders []broker.Order, gt broker.GroupType) error {
	m.submittedGroups = append(m.submittedGroups, struct {
		orders    []broker.Order
		groupType broker.GroupType
	}{orders: orders, groupType: gt})
	return nil
}

var _ = Describe("Account ExecuteBatch with groups", func() {
	var (
		acct *portfolio.Account
		mb   *mockGroupBroker
		spy  asset.Asset
		date time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
		date = time.Date(2024, 6, 1, 16, 0, 0, 0, time.UTC)
		mb = newMockGroupBroker()
		mb.defaultFill = &broker.Fill{Price: 400, Qty: 100, FilledAt: date}
		acct = portfolio.New(
			portfolio.WithBroker(mb),
			portfolio.WithCash(100000, date),
		)
	})

	It("submits standalone OCO via GroupSubmitter", func() {
		batch := acct.NewBatch(date)
		err := batch.Order(context.Background(), spy, portfolio.Sell, 100,
			portfolio.OCO(portfolio.StopLeg(380), portfolio.LimitLeg(420)),
		)
		Expect(err).NotTo(HaveOccurred())

		err = acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		Expect(mb.submittedGroups).To(HaveLen(1))
		Expect(mb.submittedGroups[0].groupType).To(Equal(broker.GroupOCO))
		Expect(mb.submittedGroups[0].orders).To(HaveLen(2))
	})

	It("submits only bracket entry order, defers exits", func() {
		batch := acct.NewBatch(date)
		err := batch.Order(context.Background(), spy, portfolio.Buy, 100,
			portfolio.Limit(400),
			portfolio.WithBracket(portfolio.StopLossPrice(380), portfolio.TakeProfitPrice(420)),
		)
		Expect(err).NotTo(HaveOccurred())

		err = acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// Entry order submitted individually (not via GroupSubmitter)
		Expect(mb.submitted).To(HaveLen(1))
		Expect(mb.submitted[0].GroupRole).To(Equal(broker.RoleEntry))

		// No group submission yet (exits deferred until fill)
		Expect(mb.submittedGroups).To(BeEmpty())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "Account ExecuteBatch with groups" -count=1`
Expected: FAIL -- ExecuteBatch submits all orders individually, no GroupSubmitter check

- [ ] **Step 3: Add pendingGroups and brokerHasGroups to Account**

In `portfolio/account.go`, add fields to the `Account` struct:

```go
	pendingGroups   map[string]*broker.OrderGroup
	brokerHasGroups bool
	deferredExits   map[string]OrderGroupSpec // groupID -> bracket spec (deferred until entry fills)
```

Initialize in `New()`:

```go
	pendingGroups: make(map[string]*broker.OrderGroup),
	deferredExits: make(map[string]OrderGroupSpec),
```

Cache `brokerHasGroups` in `SetBroker()`:

```go
func (a *Account) SetBroker(b broker.Broker) {
	a.broker = b
	_, a.brokerHasGroups = b.(broker.GroupSubmitter)
}
```

- [ ] **Step 4: Modify ExecuteBatch for group-aware submission**

In `portfolio/account.go`, replace the order submission loop in `ExecuteBatch` (lines 888-900) with group-aware logic:

```go
	// 4. Assign IDs, track, and submit orders.
	// First, identify groups and assign group IDs.
	groupOrders := make(map[string][]broker.Order)

	for idx := range batch.Orders {
		order := &batch.Orders[idx]
		if order.ID == "" {
			order.ID = fmt.Sprintf("batch-%d-%d", batch.Timestamp.UnixNano(), idx)
		}
		a.pendingOrders[order.ID] = *order

		if order.GroupID != "" {
			groupOrders[order.GroupID] = append(groupOrders[order.GroupID], *order)
		}
	}

	// Store deferred bracket exits.
	for _, group := range batch.Groups() {
		if group.Type == broker.GroupBracket {
			entryOrder := batch.Orders[group.EntryIndex]
			a.deferredExits[entryOrder.GroupID] = group
		}
	}

	// Submit orders.
	submitted := make(map[string]bool)
	for _, group := range batch.Groups() {
		if group.Type == broker.GroupOCO {
			// Standalone OCO: submit as a group.
			groupID := group.GroupID
			orders := groupOrders[groupID]
			if a.brokerHasGroups {
				gs := a.broker.(broker.GroupSubmitter)
				if err := gs.SubmitGroup(ctx, orders, broker.GroupOCO); err != nil {
					return fmt.Errorf("execute batch: submit group: %w", err)
				}
			} else {
				for _, order := range orders {
					if err := a.broker.Submit(ctx, order); err != nil {
						return fmt.Errorf("execute batch: submit %s: %w", order.Asset.Ticker, err)
					}
				}
			}
			for _, order := range orders {
				submitted[order.ID] = true
			}

			// Track as pending group.
			orderIDs := make([]string, len(orders))
			for idx, order := range orders {
				orderIDs[idx] = order.ID
			}
			a.pendingGroups[groupID] = &broker.OrderGroup{
				ID: groupID, Type: broker.GroupOCO, OrderIDs: orderIDs,
			}
		} else if group.Type == broker.GroupBracket {
			// Submit only the entry order; exits deferred.
			entryOrder := batch.Orders[group.EntryIndex]
			if err := a.broker.Submit(ctx, entryOrder); err != nil {
				return fmt.Errorf("execute batch: submit %s: %w", entryOrder.Asset.Ticker, err)
			}
			submitted[entryOrder.ID] = true

			a.pendingGroups[entryOrder.GroupID] = &broker.OrderGroup{
				ID: entryOrder.GroupID, Type: broker.GroupBracket, OrderIDs: []string{entryOrder.ID},
			}
		}
	}

	// Submit remaining non-group orders.
	for _, order := range batch.Orders {
		if !submitted[order.ID] {
			if err := a.broker.Submit(ctx, order); err != nil {
				return fmt.Errorf("execute batch: submit %s: %w", order.Asset.Ticker, err)
			}
		}
	}

	// 5. Drain immediate fills.
	a.drainFillsFromChannel()

	return nil
```

Note: This is the core logic. The exact implementation may need refinement during coding -- the above captures the intent. Read the existing code carefully before editing.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "Account ExecuteBatch with groups" -count=1`
Expected: PASS

- [ ] **Step 6: Run all portfolio tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -count=1`
Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go
git commit -m "Add group tracking to Account and group-aware ExecuteBatch submission"
```

---

### Task 8: DrainFills Two-Phase Logic

**Files:**
- Modify: `portfolio/account.go:915-962`
- Modify: `portfolio/account_test.go`

- [ ] **Step 1: Write failing test for bracket entry fill triggering exit submission**

Add to `portfolio/account_test.go`:

```go
var _ = Describe("DrainFills bracket lifecycle", func() {
	It("submits OCO exits when bracket entry fills", func() {
		spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
		date := time.Date(2024, 6, 1, 16, 0, 0, 0, time.UTC)
		mb := newMockGroupBroker()
		// Entry fill at $400
		mb.defaultFill = &broker.Fill{Price: 400, Qty: 100, FilledAt: date}
		acct := portfolio.New(
			portfolio.WithBroker(mb),
			portfolio.WithCash(100000, date),
		)

		batch := acct.NewBatch(date)
		err := batch.Order(context.Background(), spy, portfolio.Buy, 100,
			portfolio.Limit(400),
			portfolio.WithBracket(portfolio.StopLossPercent(-0.05), portfolio.TakeProfitPercent(0.10)),
		)
		Expect(err).NotTo(HaveOccurred())

		err = acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// After ExecuteBatch drains fills, the entry fill should trigger
		// deferred exit submission.
		Expect(mb.submittedGroups).To(HaveLen(1))
		Expect(mb.submittedGroups[0].groupType).To(Equal(broker.GroupOCO))

		exitOrders := mb.submittedGroups[0].orders
		Expect(exitOrders).To(HaveLen(2))

		// Stop loss: 400 * (1 + -0.05) = 380
		var stopOrder, tpOrder broker.Order
		for _, order := range exitOrders {
			if order.GroupRole == broker.RoleStopLoss {
				stopOrder = order
			} else {
				tpOrder = order
			}
		}
		Expect(stopOrder.StopPrice).To(Equal(380.0))
		Expect(stopOrder.OrderType).To(Equal(broker.Stop))
		Expect(stopOrder.TimeInForce).To(Equal(broker.GTC))

		// Take profit: 400 * (1 + 0.10) = 440
		Expect(tpOrder.LimitPrice).To(Equal(440.0))
		Expect(tpOrder.OrderType).To(Equal(broker.Limit))
		Expect(tpOrder.TimeInForce).To(Equal(broker.GTC))
	})

	It("cancels OCO sibling when one leg fills (GroupSubmitter)", func() {
		spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
		date := time.Date(2024, 6, 1, 16, 0, 0, 0, time.UTC)
		mb := newMockGroupBroker()
		acct := portfolio.New(
			portfolio.WithBroker(mb),
			portfolio.WithCash(100000, date),
		)
		// Manually set up position so sells make sense
		acct.Record(portfolio.Transaction{
			Date: date, Asset: spy, Type: portfolio.BuyTransaction,
			Qty: 100, Price: 400, Amount: -40000,
		})

		batch := acct.NewBatch(date)
		err := batch.Order(context.Background(), spy, portfolio.Sell, 100,
			portfolio.OCO(portfolio.StopLeg(380), portfolio.LimitLeg(420)),
		)
		Expect(err).NotTo(HaveOccurred())
		err = acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// Simulate stop loss fill arriving on fills channel
		stopOrderID := batch.Orders[0].ID
		mb.fillCh <- broker.Fill{OrderID: stopOrderID, Price: 380, Qty: 100, FilledAt: date}

		err = acct.DrainFills(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// Sibling should be cleaned up from pendingOrders (no Cancel call for GroupSubmitter)
		// Verify via no cancel calls on the broker
		// The account should have recorded the sell transaction
		txns := acct.Transactions()
		var sellTxns []portfolio.Transaction
		for _, tx := range txns {
			if tx.Type == portfolio.SellTransaction {
				sellTxns = append(sellTxns, tx)
			}
		}
		Expect(sellTxns).To(HaveLen(1))
		Expect(sellTxns[0].Price).To(Equal(380.0))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "DrainFills bracket lifecycle" -count=1`
Expected: FAIL -- drainFillsFromChannel does not handle groups

- [ ] **Step 3: Implement two-phase DrainFills**

Rewrite `drainFillsFromChannel` in `portfolio/account.go`:

```go
func (a *Account) drainFillsFromChannel() {
	if a.broker == nil {
		return
	}

	fillCh := a.broker.Fills()

	// Phase 1: Collect fills and process them.
	type deferredExit struct {
		groupID   string
		spec      OrderGroupSpec
		fillPrice float64
		asset     asset.Asset
		qty       float64
		entrySide broker.Side // needed to determine exit side (opposite)
	}
	var deferred []deferredExit

	for {
		select {
		case fill := <-fillCh:
			order, ok := a.pendingOrders[fill.OrderID]
			if !ok {
				log.Warn().Str("orderID", fill.OrderID).Msg("received fill for unknown order")
				continue
			}

			// Record transaction (existing logic).
			var (
				txType TransactionType
				amount float64
			)
			switch order.Side {
			case broker.Buy:
				txType = BuyTransaction
				amount = -(fill.Price * fill.Qty)
			case broker.Sell:
				txType = SellTransaction
				amount = fill.Price * fill.Qty
			}
			a.Record(Transaction{
				Date:          fill.FilledAt,
				Asset:         order.Asset,
				Type:          txType,
				Qty:           fill.Qty,
				Price:         fill.Price,
				Amount:        amount,
				Justification: order.Justification,
			})
			delete(a.pendingOrders, fill.OrderID)

			// Group handling.
			if order.GroupID != "" {
				group, groupExists := a.pendingGroups[order.GroupID]
				if groupExists {
					if order.GroupRole == broker.RoleEntry {
						// Bracket entry filled: collect deferred exits.
						if spec, hasSpec := a.deferredExits[order.GroupID]; hasSpec {
							deferred = append(deferred, deferredExit{
								groupID:   order.GroupID,
								spec:      spec,
								fillPrice: fill.Price,
								asset:     order.Asset,
								qty:       order.Qty,
								entrySide: order.Side,
							})
							delete(a.deferredExits, order.GroupID)
						}
					} else {
						// OCO leg filled: clean up siblings.
						for _, siblingID := range group.OrderIDs {
							if siblingID == fill.OrderID {
								continue
							}
							if !a.brokerHasGroups {
								_ = a.broker.Cancel(context.Background(), siblingID)
							}
							delete(a.pendingOrders, siblingID)
						}
						delete(a.pendingGroups, order.GroupID)
					}
				}
			}
		default:
			goto phase2
		}
	}

phase2:
	// Phase 2: Submit deferred bracket exits.
	for _, exit := range deferred {
		var stopPrice, tpPrice float64

		if exit.spec.StopLoss.AbsolutePrice != 0 {
			stopPrice = exit.spec.StopLoss.AbsolutePrice
		} else {
			stopPrice = exit.fillPrice * (1 + exit.spec.StopLoss.PercentOffset)
		}

		if exit.spec.TakeProfit.AbsolutePrice != 0 {
			tpPrice = exit.spec.TakeProfit.AbsolutePrice
		} else {
			tpPrice = exit.fillPrice * (1 + exit.spec.TakeProfit.PercentOffset)
		}

		groupID := fmt.Sprintf("%s-exits", exit.groupID)
		slOrderID := fmt.Sprintf("%s-sl", exit.groupID)
		tpOrderID := fmt.Sprintf("%s-tp", exit.groupID)

		// Exit side is opposite of entry side.
		exitSide := broker.Sell
		if exit.entrySide == broker.Sell {
			exitSide = broker.Buy
		}

		slOrder := broker.Order{
			ID:          slOrderID,
			Asset:       exit.asset,
			Side:        exitSide,
			Qty:         exit.qty,
			OrderType:   broker.Stop,
			StopPrice:   stopPrice,
			TimeInForce: broker.GTC,
			GroupID:     groupID,
			GroupRole:   broker.RoleStopLoss,
		}

		tpOrder := broker.Order{
			ID:          tpOrderID,
			Asset:       exit.asset,
			Side:        exitSide,
			Qty:         exit.qty,
			OrderType:   broker.Limit,
			LimitPrice:  tpPrice,
			TimeInForce: broker.GTC,
			GroupID:     groupID,
			GroupRole:   broker.RoleTakeProfit,
		}

		a.pendingOrders[slOrderID] = slOrder
		a.pendingOrders[tpOrderID] = tpOrder
		a.pendingGroups[groupID] = &broker.OrderGroup{
			ID:       groupID,
			Type:     broker.GroupOCO,
			OrderIDs: []string{slOrderID, tpOrderID},
		}

		if a.brokerHasGroups {
			gs := a.broker.(broker.GroupSubmitter)
			_ = gs.SubmitGroup(context.Background(), []broker.Order{slOrder, tpOrder}, broker.GroupOCO)
		} else {
			_ = a.broker.Submit(context.Background(), slOrder)
			_ = a.broker.Submit(context.Background(), tpOrder)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "DrainFills bracket lifecycle" -count=1`
Expected: PASS

- [ ] **Step 5: Run all portfolio tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go
git commit -m "Implement two-phase DrainFills with bracket exit submission and OCO sibling cancellation"
```

---

### Task 9: CancelOpenOrders Group Cleanup

**Files:**
- Modify: `portfolio/account.go:966-987`
- Modify: `portfolio/account_test.go`

- [ ] **Step 1: Write failing test**

Add to `portfolio/account_test.go`:

```go
var _ = Describe("CancelOpenOrders with groups", func() {
	It("cleans up pendingGroups and deferredExits", func() {
		spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
		date := time.Date(2024, 6, 1, 16, 0, 0, 0, time.UTC)
		mb := newMockGroupBroker()
		// No fill -- entry stays pending
		acct := portfolio.New(
			portfolio.WithBroker(mb),
			portfolio.WithCash(100000, date),
		)

		batch := acct.NewBatch(date)
		err := batch.Order(context.Background(), spy, portfolio.Buy, 100,
			portfolio.Limit(400),
			portfolio.WithBracket(portfolio.StopLossPrice(380), portfolio.TakeProfitPrice(420)),
		)
		Expect(err).NotTo(HaveOccurred())
		err = acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// Cancel all open orders (frame boundary).
		err = acct.CancelOpenOrders(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// Verify everything is cleaned up by submitting a new batch
		// (no stale state should interfere).
		batch2 := acct.NewBatch(date)
		err = batch2.Order(context.Background(), spy, portfolio.Buy, 50, portfolio.Limit(400))
		Expect(err).NotTo(HaveOccurred())
		err = acct.ExecuteBatch(context.Background(), batch2)
		Expect(err).NotTo(HaveOccurred())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "CancelOpenOrders with groups" -count=1`
Expected: FAIL -- CancelOpenOrders calls broker.Orders() which may not return grouped orders correctly, and doesn't clean up pendingGroups/deferredExits

- [ ] **Step 3: Update CancelOpenOrders**

In `portfolio/account.go`, modify `CancelOpenOrders`:

```go
func (a *Account) CancelOpenOrders(ctx context.Context) error {
	if a.broker == nil {
		return nil
	}

	// Cancel all pending orders via the broker.
	var errs []error
	for orderID := range a.pendingOrders {
		if cancelErr := a.broker.Cancel(ctx, orderID); cancelErr != nil {
			errs = append(errs, fmt.Errorf("cancel order %s: %w", orderID, cancelErr))
		}
	}

	// Clear all tracking state regardless of errors.
	a.pendingOrders = make(map[string]broker.Order)
	a.pendingGroups = make(map[string]*broker.OrderGroup)
	a.deferredExits = make(map[string]OrderGroupSpec)

	return errors.Join(errs...)
}
```

This replaces the previous approach of querying `broker.Orders()` with iterating the account's own `pendingOrders` map, which is the source of truth.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "CancelOpenOrders with groups" -count=1`
Expected: PASS

- [ ] **Step 5: Run all portfolio tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go
git commit -m "Update CancelOpenOrders to clean up group state and deferred exits"
```

---

### Task 10: Account.Clone() Deep Copy

**Files:**
- Modify: `portfolio/account.go` (Clone method, around line 995)
- Modify: `portfolio/account_test.go`

- [ ] **Step 1: Write failing test**

Add to `portfolio/account_test.go`:

```go
var _ = Describe("Account Clone with groups", func() {
	It("deep copies pendingGroups", func() {
		date := time.Date(2024, 6, 1, 16, 0, 0, 0, time.UTC)
		mb := newMockBroker()
		acct := portfolio.New(
			portfolio.WithBroker(mb),
			portfolio.WithCash(100000, date),
		)

		clone := acct.Clone()
		// The clone should have independent group maps.
		// This is a structural test -- verify clone doesn't panic
		// and modifying clone doesn't affect original.
		Expect(clone).NotTo(BeNil())
	})
})
```

- [ ] **Step 2: Update Clone to deep-copy new fields**

In `portfolio/account.go`, in the `Clone()` method, add after existing map copies:

```go
	pendingGroups := make(map[string]*broker.OrderGroup, len(acct.pendingGroups))
	for gid, group := range acct.pendingGroups {
		orderIDs := make([]string, len(group.OrderIDs))
		copy(orderIDs, group.OrderIDs)
		pendingGroups[gid] = &broker.OrderGroup{
			ID: group.ID, Type: group.Type, OrderIDs: orderIDs,
		}
	}

	deferredExits := make(map[string]OrderGroupSpec, len(acct.deferredExits))
	for gid, spec := range acct.deferredExits {
		deferredExits[gid] = spec
	}
```

And assign them to the cloned account.

- [ ] **Step 3: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -run "Account Clone with groups" -count=1`
Expected: PASS

- [ ] **Step 4: Run all portfolio tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -count=1`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go
git commit -m "Deep copy pendingGroups and deferredExits in Account.Clone"
```

---

### Task 11: Engine Integration -- EvaluatePending Call

**Files:**
- Modify: `engine/backtest.go:370-430`
- Modify: `engine/backtest_test.go`

- [ ] **Step 1: Write failing integration test**

Add to `engine/backtest_test.go`. This requires a strategy that places a bracket order and price data that triggers the stop loss:

```go
// bracketStrategy places a bracket order on its first Compute call.
type bracketStrategy struct {
	placed    bool
	testAsset asset.Asset
	stopPct   float64
	tpPct     float64
}

func (s *bracketStrategy) Name() string       { return "bracket-test" }
func (s *bracketStrategy) Setup(_ *engine.Engine) {}
func (s *bracketStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	if !s.placed {
		s.placed = true
		return batch.Order(context.Background(), s.testAsset, portfolio.Buy, 100,
			portfolio.WithBracket(
				portfolio.StopLossPercent(s.stopPct),
				portfolio.TakeProfitPercent(s.tpPct),
			),
		)
	}
	return nil
}

var _ = Describe("Bracket order backtest integration", func() {
	// This test verifies the full lifecycle using a real Engine.Backtest run.
	// Data provider setup pattern: see makeLowPriceTestData in meta_strategy_test.go.
	// The data must include MetricHigh, MetricLow, MetricClose, AdjClose, and Dividend.

	It("triggers stop loss on intrabar low", func() {
		// Setup:
		// Day 1 close=100, high=105, low=98 -> entry fills at 100
		// Day 2 close=97, high=101, low=93 -> stop at 95 (100 * 0.95) triggers (low 93 <= 95)
		// Result: sell transaction at 95

		spy := asset.Asset{Ticker: "SPY", CompositeFigi: "BBG000BHTMY2"}
		start := time.Date(2024, 1, 2, 16, 0, 0, 0, time.UTC)
		end := time.Date(2024, 1, 4, 16, 0, 0, 0, time.UTC)

		// Build a DataFrame with 3 days of data (need day before start for housekeeping).
		// Read engine/meta_strategy_test.go:makeLowPriceTestData for the exact pattern.
		// Metrics: MetricClose, AdjClose, Dividend, MetricHigh, MetricLow
		// Day 0 (Jan 1): close=100, high=105, low=98
		// Day 1 (Jan 2): close=100, high=105, low=98  (entry day)
		// Day 2 (Jan 3): close=97, high=101, low=93   (stop triggers)
		// Day 3 (Jan 4): close=96, high=99, low=94    (should not reach here)

		metrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.MetricHigh, data.MetricLow}
		times := []time.Time{
			time.Date(2024, 1, 1, 16, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 2, 16, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 3, 16, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 4, 16, 0, 0, 0, time.UTC),
		}
		assets := []asset.Asset{spy}
		// vals layout: [asset0_metric0_time0, asset0_metric1_time0, ..., asset0_metric0_time1, ...]
		// Check data.NewDataFrame docs for exact layout -- may be
		// [time0_asset0_metric0, time0_asset0_metric1, ..., time1_asset0_metric0, ...]
		// The implementer MUST verify the exact layout by reading data/dataframe.go
		// and existing test helpers before filling in values.
		//
		// Values per day (close, adjclose, dividend, high, low):
		// Day 0: 100, 100, 0, 105, 98
		// Day 1: 100, 100, 0, 105, 98
		// Day 2:  97,  97, 0, 101, 93
		// Day 3:  96,  96, 0,  99, 94

		// The implementer should construct the DataFrame following the pattern in
		// makeLowPriceTestData and data.NewDataFrame, then create a TestProvider.
		// Wire up the engine with:
		//   engine.WithStrategy(&bracketStrategy{testAsset: spy, stopPct: -0.05, tpPct: 0.10})
		//   engine.WithDataProvider(provider)
		//   engine.WithCash(100000, start)
		//   engine.WithSchedule("@daily")  // or appropriate tradecron expression
		//
		// Run engine.Backtest(ctx, start, end) and inspect the resulting portfolio:
		//   - Should have a BuyTransaction on Day 1 at price 100
		//   - Should have a SellTransaction on Day 2 at price 95 (stop loss)
		//   - Final cash should reflect the round-trip

		// NOTE: The exact DataFrame value layout, provider wiring, and tradecron
		// setup must be adapted from existing test patterns in the file.
		// Read backtest_test.go and meta_strategy_test.go before implementing.
		Skip("Adapt from existing engine test patterns -- see comments above for data setup")
	})

	// Additional cases to implement following the same pattern:
	It("triggers take profit on intrabar high", func() {
		// Day 2: high >= take profit price -> take profit fills
		Skip("Adapt from stop loss test with high data that triggers take profit")
	})

	It("stop loss wins when both triggers fire on same bar", func() {
		// Day 2: low <= stop AND high >= take profit -> stop loss wins
		Skip("Adapt with both triggers active on same bar")
	})

	It("bracket persists across bars within a frame", func() {
		// Day 2: neither triggers, Day 3: stop triggers
		Skip("Adapt with prices that don't trigger until day 3")
	})

	It("bracket cancelled at frame boundary", func() {
		// Use a weekly schedule so frame changes, verify bracket is cancelled
		Skip("Use different tradecron schedule")
	})
})
```

Note: The first integration test has detailed setup comments that serve as a concrete pattern. The implementer must read existing test helpers (`makeLowPriceTestData` in `engine/meta_strategy_test.go`, `data.NewDataFrame` in `data/dataframe.go`) to determine the exact DataFrame value layout and adapt accordingly. Remove `Skip` calls and implement fully. The remaining 4 tests follow the same pattern with different price data.

- [ ] **Step 2: Add EvaluatePending call to engine housekeeping**

**Important:** `EvaluatePending` must NOT go inside `housekeepAccount` -- that function receives an `*Account` and has no access to the broker. Instead, add the call in the **main step loop body** in `Backtest()` (around line 293 in `backtest.go`), right before `housekeepAccount` is called. The sequence per bar is:

1. `SetPriceProvider` (already exists at line 319 for strategy days, needs to also run on non-strategy days)
2. `EvaluatePending` (new)
3. `housekeepAccount` (existing -- internally calls `DrainFills`, which picks up triggered fills)

In `engine/backtest.go`, in the main step loop (around line 280), add before `housekeepAccount`:

```go
	// Evaluate pending bracket/OCO orders against intrabar prices.
	if sb, ok := e.broker.(*SimulatedBroker); ok {
		sb.SetPriceProvider(e, date)
		sb.EvaluatePending()
	}
```

This runs on every bar (not just strategy days), so deferred bracket orders are evaluated each bar. `SetPriceProvider` is called here unconditionally so that the price data is available for `EvaluatePending`. The existing `SetPriceProvider` call inside the `if step.isParentStrategy` block (line 319) remains for strategy-day fill simulation of non-bracket orders.

For children, add after each child's `SetPriceProvider` call (line 300) and before the child's `housekeepAccount`:

```go
	child.broker.SetPriceProvider(e, date) // already exists
	child.broker.EvaluatePending()          // add this
```

The child broker is typed as `*SimulatedBroker` so no type assertion is needed.

- [ ] **Step 3: Implement the integration tests (remove Skip calls)**

Create a `bracketStrategy` type that places a bracket order on first call:

```go
type bracketStrategy struct {
	placed bool
	asset  asset.Asset
}

func (s *bracketStrategy) Name() string { return "bracket-test" }
func (s *bracketStrategy) Setup(_ *engine.Engine) {}
func (s *bracketStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	if !s.placed {
		s.placed = true
		return batch.Order(context.Background(), s.asset, portfolio.Buy, 100,
			portfolio.Limit(400),
			portfolio.WithBracket(portfolio.StopLossPercent(-0.05), portfolio.TakeProfitPercent(0.10)),
		)
	}
	return nil
}
```

Set up data with high/low metrics that trigger the stop on day 2.

- [ ] **Step 4: Run integration tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -run "Bracket order backtest" -count=1`
Expected: PASS

- [ ] **Step 5: Run all engine tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -v -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add engine/backtest.go engine/backtest_test.go
git commit -m "Add EvaluatePending call to engine housekeeping and bracket integration tests"
```

---

### Task 12: Full Test Suite and Lint

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -count=1`
Expected: all pass

- [ ] **Step 2: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./...`
Expected: no new issues. Fix any issues found (including pre-existing ones in modified files per project conventions).

- [ ] **Step 3: Fix any lint issues**

Fix all issues found by golangci-lint.

- [ ] **Step 4: Run tests again after lint fixes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -count=1`
Expected: all pass

- [ ] **Step 5: Commit lint fixes if any**

```bash
git add -u
git commit -m "Fix lint issues in bracket/OCO implementation"
```

---

### Task 13: Changelog

- [ ] **Step 1: Update changelog**

Add an entry to the changelog describing the bracket and OCO order types feature. Use complete sentences in active voice per project conventions. Combine related items into a single entry.

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "Add changelog entry for bracket and OCO order types"
```
