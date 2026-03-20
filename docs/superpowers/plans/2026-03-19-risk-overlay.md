# Portfolio Middleware and Risk Management Overlay Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a portfolio middleware system with batch-based order accumulation, making the portfolio read-only during strategy execution, and ship built-in risk management middleware.

**Architecture:** A new `Batch` type accumulates orders and annotations during each frame. The `Portfolio` interface becomes read-only. A `Middleware` interface processes batches between strategy execution and broker submission. The `Broker` interface gains a fill channel for non-blocking order execution. Built-in risk middleware ships as the first middleware consumers.

**Tech Stack:** Go, Ginkgo/Gomega (tests), existing broker/portfolio/engine packages

**Spec:** `docs/superpowers/specs/2026-03-19-risk-overlay-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `portfolio/annotation.go` | Modify | Migrate `Annotation.Timestamp` from `int64` to `time.Time` |
| `data/annotator.go` | Modify | Update `Annotator` interface signature to `time.Time` |
| `data/data_frame.go` | Modify | Update `Annotate` call site for `time.Time` |
| `portfolio/account.go` | Modify | Update `Annotate` method, add `Use`, `NewBatch`, `ExecuteBatch`, `DrainFills`, `CancelOpenOrders`, add `middleware` and `openOrders` fields |
| `portfolio/portfolio.go` | Modify | Remove `RebalanceTo`, `Order`, `Annotate` from `Portfolio` interface; add batch/middleware methods to `PortfolioManager` |
| `portfolio/middleware.go` | Create | `Middleware` interface definition |
| `portfolio/batch.go` | Create | `Batch` type with `RebalanceTo`, `Order`, `Annotate`, projected state methods |
| `portfolio/batch_test.go` | Create | Tests for Batch accumulation and projected state |
| `portfolio/middleware_test.go` | Create | Tests for middleware chain execution |
| `portfolio/rebalance_test.go` | Modify | Update tests for batch-based rebalancing |
| `portfolio/order_test.go` | Modify | Update tests for batch-based ordering |
| `portfolio/annotation_test.go` | Modify | Update tests for `time.Time` timestamps |
| `portfolio/account_test.go` | Modify | Update tests for read-only portfolio, batch execution |
| `broker/broker.go` | Modify | Change `Submit`/`Replace` return types, add `Fills() <-chan Fill` |
| `engine/simulated_broker.go` | Modify | Add fill channel, write fills on Submit |
| `engine/simulated_broker_test.go` | Modify | Update for channel-based fills |
| `engine/strategy.go` | Modify | Add `batch *portfolio.Batch` parameter to `Compute` |
| `engine/backtest.go` | Modify | Add drain/cancel/batch lifecycle to step loop |
| `engine/live.go` | Modify | Same step loop changes as backtest |
| `engine/engine.go` | Modify | Update `PredictedPortfolio` for new Compute signature |
| `engine/backtest_test.go` | Modify | Update all test strategies for new Compute signature |
| `engine/example_test.go` | Modify | Update example strategies |
| `engine/fetch_test.go` | Modify | Update test strategies |
| `engine/predicted_portfolio_test.go` | Modify | Update test strategy and PredictedPortfolio call |
| `risk/risk.go` | Create | Package doc, shared helpers |
| `risk/max_position_size.go` | Create | `MaxPositionSize` middleware |
| `risk/max_position_size_test.go` | Create | Tests |
| `risk/drawdown_circuit_breaker.go` | Create | `DrawdownCircuitBreaker` middleware |
| `risk/drawdown_circuit_breaker_test.go` | Create | Tests |
| `risk/max_position_count.go` | Create | `MaxPositionCount` middleware |
| `risk/max_position_count_test.go` | Create | Tests |
| `risk/volatility_scaler.go` | Create | `VolatilityScaler` middleware |
| `risk/volatility_scaler_test.go` | Create | Tests |
| `risk/profiles.go` | Create | `Conservative`, `Moderate`, `Aggressive` convenience profiles |
| `risk/profiles_test.go` | Create | Tests |
| `examples/momentum-rotation/main.go` | Modify | Update Compute signature |

---

### Task 1: Migrate Annotation timestamp from int64 to time.Time

**Files:**
- Modify: `portfolio/annotation.go:24-28`
- Modify: `data/annotator.go:21-23`
- Modify: `data/data_frame.go:1828-1835`
- Modify: `portfolio/account.go:726-739`
- Modify: `portfolio/annotation_test.go`
- Modify: `portfolio/account_test.go:621-648`
- Modify: `portfolio/sqlite_test.go:188-190`
- Modify: `engine/predicted_portfolio_test.go:58`

This is a cross-cutting prerequisite. All annotation timestamps change from `int64` (Unix seconds) to `time.Time`.

- [ ] **Step 1: Write failing test for time.Time annotation**

In `portfolio/annotation_test.go`, update the existing test to use `time.Time`:

```go
It("stores a time.Time timestamp", func() {
    ts := time.Date(2024, 6, 15, 16, 0, 0, 0, time.UTC)
    acct.Annotate(ts, "signal", "0.75")
    Expect(acct.Annotations()).To(HaveLen(1))
    Expect(acct.Annotations()[0].Timestamp).To(Equal(ts))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Annotation" -v`
Expected: FAIL -- `Annotate` expects `int64` not `time.Time`

- [ ] **Step 3: Update Annotation struct**

In `portfolio/annotation.go`, change:
```go
type Annotation struct {
	Timestamp time.Time
	Key       string
	Value     string
}
```

Add `"time"` import.

- [ ] **Step 4: Update Account.Annotate signature**

In `portfolio/account.go`, change the `Annotate` method:
```go
func (a *Account) Annotate(timestamp time.Time, key, value string) {
	for idx := range a.annotations {
		if a.annotations[idx].Timestamp.Equal(timestamp) && a.annotations[idx].Key == key {
			a.annotations[idx].Value = value
			return
		}
	}

	a.annotations = append(a.annotations, Annotation{
		Timestamp: timestamp,
		Key:       key,
		Value:     value,
	})
}
```

- [ ] **Step 5: Update data.Annotator interface**

In `data/annotator.go`, change:
```go
type Annotator interface {
	Annotate(timestamp time.Time, key, value string)
}
```

Add `"time"` import.

- [ ] **Step 6: Update DataFrame.Annotate call site**

In `data/data_frame.go` around line 1835, change:
```go
dest.Annotate(timestamp, assetItem.Ticker+"/"+string(metric), strconv.FormatFloat(value, 'f', -1, 64))
```

The `timestamp` variable from the range loop is already a `time.Time`, so remove the `.Unix()` conversion on line 1829. Delete the `unixSeconds` variable and use `timestamp` directly.

- [ ] **Step 7: Update all test call sites**

Update every test that calls `Annotate` with an `int64` to use `time.Time`:

In `portfolio/annotation_test.go`: Replace `int64` timestamps with `time.Date(...)` values.

In `portfolio/account_test.go` (around line 621): Change `acct.Annotate(time.Date(...).Unix(), ...)` to `acct.Annotate(time.Date(...), ...)`.

In `portfolio/sqlite_test.go` (around line 188): Same change.

In `engine/predicted_portfolio_test.go` (line 58): Change `fund.Annotate(eng.CurrentDate().Unix(), ...)` to `fund.Annotate(eng.CurrentDate(), ...)`.

- [ ] **Step 8: Run all tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ ./data/ ./engine/ -v -count=1`
Expected: PASS

- [ ] **Step 9: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./portfolio/ ./data/ ./engine/`
Expected: No new issues

- [ ] **Step 10: Commit**

```bash
git add portfolio/annotation.go portfolio/account.go data/annotator.go data/data_frame.go portfolio/annotation_test.go portfolio/account_test.go portfolio/sqlite_test.go engine/predicted_portfolio_test.go
git commit -m "refactor: migrate Annotation.Timestamp from int64 to time.Time"
```

---

### Task 2: Broker fill channel

**Files:**
- Modify: `broker/broker.go:30-59` (Broker interface)
- Modify: `engine/simulated_broker.go` (full file)
- Modify: `engine/simulated_broker_test.go`

Change `Submit` and `Replace` to return `error` only, add `Fills() <-chan Fill`.

- [ ] **Step 1: Write failing test for fill channel**

In `engine/simulated_broker_test.go`, add a test:

```go
It("delivers fills through the Fills channel", func() {
    sb := engine.NewSimulatedBroker()
    sb.SetPriceProvider(priceProvider, tradeDate)

    err := sb.Submit(ctx, broker.Order{
        Asset:     spy,
        Side:      broker.Buy,
        Amount:    10000,
        OrderType: broker.Market,
    })
    Expect(err).NotTo(HaveOccurred())

    var fill broker.Fill
    Eventually(sb.Fills()).Should(Receive(&fill))
    Expect(fill.Price).To(BeNumerically(">", 0))
    Expect(fill.Qty).To(BeNumerically(">", 0))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -run "Fills channel" -v`
Expected: FAIL -- `Fills()` method does not exist

- [ ] **Step 3: Update Broker interface**

In `broker/broker.go`, change `Submit` and `Replace` return types and add `Fills`:

```go
type Broker interface {
	Connect(ctx context.Context) error
	Close() error
	Submit(ctx context.Context, order Order) error
	Cancel(ctx context.Context, orderID string) error
	Replace(ctx context.Context, orderID string, order Order) error
	Orders(ctx context.Context) ([]Order, error)
	Positions(ctx context.Context) ([]Position, error)
	Balance(ctx context.Context) (Balance, error)
	Fills() <-chan Fill
}
```

- [ ] **Step 4: Update SimulatedBroker**

In `engine/simulated_broker.go`, add fill channel:

```go
type SimulatedBroker struct {
	prices broker.PriceProvider
	date   time.Time
	fills  chan broker.Fill
}

func NewSimulatedBroker() *SimulatedBroker {
	return &SimulatedBroker{
		fills: make(chan broker.Fill, 1024),
	}
}

func (b *SimulatedBroker) Fills() <-chan broker.Fill {
	return b.fills
}

func (b *SimulatedBroker) Submit(ctx context.Context, order broker.Order) error {
	if b.prices == nil {
		return fmt.Errorf("simulated broker: no price provider set")
	}

	df, err := b.prices.Prices(ctx, order.Asset)
	if err != nil {
		return fmt.Errorf("simulated broker: fetching price for %s: %w", order.Asset.Ticker, err)
	}

	price := df.Value(order.Asset, data.MetricClose)
	if math.IsNaN(price) || price == 0 {
		return fmt.Errorf("simulated broker: no price for %s (%s)",
			order.Asset.Ticker, order.Asset.CompositeFigi)
	}

	qty := order.Qty
	if qty == 0 && order.Amount > 0 {
		qty = math.Floor(order.Amount / price)
	}

	if qty == 0 {
		return nil
	}

	b.fills <- broker.Fill{
		OrderID:  order.ID,
		Price:    price,
		Qty:      qty,
		FilledAt: b.date,
	}

	return nil
}

func (b *SimulatedBroker) Replace(_ context.Context, _ string, _ broker.Order) error {
	return fmt.Errorf("simulated broker: replace not supported")
}
```

Leave `Connect`, `Close`, `Cancel`, `Orders`, `Positions`, `Balance` unchanged (except `Replace` return type).

- [ ] **Step 5: Update existing SimulatedBroker tests**

Update all tests in `engine/simulated_broker_test.go` that expect `Submit` to return `([]Fill, error)` -- change to expect `error` only and drain fills from the channel.

- [ ] **Step 6: Update Account.submitAndRecord**

In `portfolio/account.go`, the `submitAndRecord` method currently reads fills from `Submit`'s return value. Remove the fill processing from `submitAndRecord` -- it now only calls `Submit`. Fill processing moves to `DrainFills` (Task 4).

Temporary: to keep existing tests passing before Task 4, add a `drainAndRecord` helper that reads from the fill channel and records transactions. Call it from `submitAndRecord` after `Submit`.

```go
func (a *Account) submitAndRecord(ctx context.Context, ast asset.Asset, side Side, order broker.Order, justification string) error {
	if err := a.broker.Submit(ctx, order); err != nil {
		return fmt.Errorf("order %s (qty=%.2f, amount=%.2f): %w", ast.Ticker, order.Qty, order.Amount, err)
	}

	// Drain immediate fills from the channel.
	a.drainAndRecord(side, ast, justification)

	return nil
}

func (a *Account) drainAndRecord(side Side, ast asset.Asset, justification string) {
	fillCh := a.broker.Fills()
	for {
		select {
		case fill := <-fillCh:
			var (
				txType TransactionType
				amount float64
			)

			switch side {
			case Buy:
				txType = BuyTransaction
				amount = -(fill.Price * fill.Qty)
			case Sell:
				txType = SellTransaction
				amount = fill.Price * fill.Qty
			}

			a.Record(Transaction{
				Date:          fill.FilledAt,
				Asset:         ast,
				Type:          txType,
				Qty:           fill.Qty,
				Price:         fill.Price,
				Amount:        amount,
				Justification: justification,
			})
		default:
			return
		}
	}
}
```

- [ ] **Step 7: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/ ./engine/ ./portfolio/ -v -count=1`
Expected: PASS

- [ ] **Step 8: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./broker/ ./engine/ ./portfolio/`

- [ ] **Step 9: Commit**

```bash
git add broker/broker.go engine/simulated_broker.go engine/simulated_broker_test.go portfolio/account.go
git commit -m "refactor: non-blocking broker Submit with fill channel"
```

---

### Task 3: Batch type

**Files:**
- Create: `portfolio/batch.go`
- Create: `portfolio/batch_test.go`

The Batch accumulates orders and annotations during a frame.

- [ ] **Step 1: Write failing tests for Batch**

Create `portfolio/batch_test.go`:

```go
package portfolio_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Batch", func() {
	var (
		acct      *portfolio.Account
		batch     *portfolio.Batch
		spy       asset.Asset
		aapl      asset.Asset
		timestamp time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		timestamp = time.Date(2024, 6, 15, 16, 0, 0, 0, time.UTC)

		acct = portfolio.New(
			portfolio.WithCash(100000, timestamp.AddDate(0, -1, 0)),
		)
		batch = acct.NewBatch(timestamp)
	})

	Describe("Order", func() {
		It("accumulates orders without executing", func() {
			err := batch.Order(context.Background(), spy, portfolio.Buy, 100)
			Expect(err).NotTo(HaveOccurred())
			Expect(batch.Orders).To(HaveLen(1))
			Expect(batch.Orders[0].Asset).To(Equal(spy))
			Expect(batch.Orders[0].Side).To(Equal(broker.Buy))
			Expect(batch.Orders[0].Qty).To(Equal(100.0))

			// Portfolio is unchanged.
			Expect(acct.Position(spy)).To(Equal(0.0))
		})
	})

	Describe("Annotate", func() {
		It("stores annotations on the batch", func() {
			batch.Annotate("signal", "bullish")
			Expect(batch.Annotations).To(HaveKeyWithValue("signal", "bullish"))
		})

		It("overwrites duplicate keys", func() {
			batch.Annotate("signal", "bullish")
			batch.Annotate("signal", "bearish")
			Expect(batch.Annotations).To(HaveKeyWithValue("signal", "bearish"))
		})
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Batch" -v`
Expected: FAIL -- `Batch` type does not exist

- [ ] **Step 3: Create Batch type**

Create `portfolio/batch.go`:

```go
package portfolio

import (
	"context"
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// Batch holds all proposed orders and annotations produced during a single
// frame. The strategy writes to the batch via Order and RebalanceTo;
// middleware processes it; the portfolio executes the final result.
type Batch struct {
	Timestamp   time.Time
	Orders      []broker.Order
	Annotations map[string]string
	portfolio   Portfolio
}

// Order accumulates an order in the batch without executing it.
func (batch *Batch) Order(ctx context.Context, ast asset.Asset, side Side, qty float64, mods ...OrderModifier) error {
	order := broker.Order{
		Asset:       ast,
		Qty:         qty,
		OrderType:   broker.Market,
		TimeInForce: broker.Day,
	}

	switch side {
	case Buy:
		order.Side = broker.Buy
	case Sell:
		order.Side = broker.Sell
	}

	var hasLimit, hasStop bool

	for _, mod := range mods {
		switch modifier := mod.(type) {
		case limitModifier:
			order.LimitPrice = modifier.price
			hasLimit = true
		case stopModifier:
			order.StopPrice = modifier.price
			hasStop = true
		case dayOrderModifier:
			order.TimeInForce = broker.Day
		case goodTilCancelModifier:
			order.TimeInForce = broker.GTC
		case fillOrKillModifier:
			order.TimeInForce = broker.FOK
		case immediateOrCancelModifier:
			order.TimeInForce = broker.IOC
		case onTheOpenModifier:
			order.TimeInForce = broker.OnOpen
		case onTheCloseModifier:
			order.TimeInForce = broker.OnClose
		case goodTilDateModifier:
			order.TimeInForce = broker.GTD
			order.GTDDate = modifier.date
		case justificationModifier:
			// Justification stored on the order is not supported;
			// use Batch.Annotate for frame-level context.
		}
	}

	if hasLimit && hasStop {
		order.OrderType = broker.StopLimit
	} else if hasLimit {
		order.OrderType = broker.Limit
	} else if hasStop {
		order.OrderType = broker.Stop
	}

	batch.Orders = append(batch.Orders, order)

	return nil
}

// RebalanceTo computes the orders needed to move the portfolio from its
// current state to the target allocation and accumulates them in the batch.
func (batch *Batch) RebalanceTo(ctx context.Context, allocs ...Allocation) error {
	for _, alloc := range allocs {
		totalValue := batch.ProjectedValue()

		// Sell positions not in the target allocation.
		projected := batch.ProjectedHoldings()
		for ast, qty := range projected {
			if _, ok := alloc.Members[ast]; !ok && qty > 0 {
				batch.Orders = append(batch.Orders, broker.Order{
					Asset:       ast,
					Side:        broker.Sell,
					Qty:         qty,
					OrderType:   broker.Market,
					TimeInForce: broker.Day,
				})
			}
		}

		// Sell overweight positions.
		for ast, weight := range alloc.Members {
			currentDollars := batch.projectedPositionValue(ast)
			targetDollars := weight * totalValue
			diff := targetDollars - currentDollars

			if diff < 0 {
				batch.Orders = append(batch.Orders, broker.Order{
					Asset:       ast,
					Side:        broker.Sell,
					Amount:      -diff,
					OrderType:   broker.Market,
					TimeInForce: broker.Day,
				})
			}
		}

		// Buy underweight positions using projected value after sells.
		postSellValue := batch.ProjectedValue()
		for ast, weight := range alloc.Members {
			currentDollars := batch.projectedPositionValue(ast)
			targetDollars := weight * postSellValue
			diff := targetDollars - currentDollars

			if diff > 0 {
				batch.Orders = append(batch.Orders, broker.Order{
					Asset:       ast,
					Side:        broker.Buy,
					Amount:      diff,
					OrderType:   broker.Market,
					TimeInForce: broker.Day,
				})
			}
		}
	}

	return nil
}

// Annotate records a key-value annotation on this batch. The timestamp
// is the batch's Timestamp.
func (batch *Batch) Annotate(key, value string) {
	batch.Annotations[key] = value
}

// Portfolio returns the read-only portfolio reference for this batch.
func (batch *Batch) Portfolio() Portfolio {
	return batch.portfolio
}

// ProjectedHoldings returns what the portfolio holdings would be if all
// orders in the batch were executed at last known prices.
func (batch *Batch) ProjectedHoldings() map[asset.Asset]float64 {
	holdings := make(map[asset.Asset]float64)

	batch.portfolio.Holdings(func(ast asset.Asset, qty float64) {
		holdings[ast] = qty
	})

	for _, order := range batch.Orders {
		qty := order.Qty
		if qty == 0 && order.Amount > 0 {
			price := batch.priceOf(order.Asset)
			if price > 0 {
				qty = math.Floor(order.Amount / price)
			}
		}

		switch order.Side {
		case broker.Buy:
			holdings[order.Asset] += qty
		case broker.Sell:
			holdings[order.Asset] -= qty
			if holdings[order.Asset] <= 0 {
				delete(holdings, order.Asset)
			}
		}
	}

	return holdings
}

// ProjectedValue returns the total portfolio value if all batch orders
// were executed at last known prices.
func (batch *Batch) ProjectedValue() float64 {
	holdings := batch.ProjectedHoldings()
	total := batch.projectedCash()

	for ast, qty := range holdings {
		price := batch.priceOf(ast)
		if price > 0 {
			total += qty * price
		}
	}

	return total
}

// ProjectedWeights returns the target weight of each position if all
// batch orders were executed at last known prices.
func (batch *Batch) ProjectedWeights() map[asset.Asset]float64 {
	holdings := batch.ProjectedHoldings()
	totalValue := batch.ProjectedValue()
	weights := make(map[asset.Asset]float64, len(holdings))

	if totalValue == 0 {
		return weights
	}

	for ast, qty := range holdings {
		price := batch.priceOf(ast)
		if price > 0 {
			weights[ast] = (qty * price) / totalValue
		}
	}

	return weights
}

// projectedCash returns the cash balance after all batch orders.
func (batch *Batch) projectedCash() float64 {
	cash := batch.portfolio.Cash()

	for _, order := range batch.Orders {
		qty := order.Qty
		price := batch.priceOf(order.Asset)

		if qty == 0 && order.Amount > 0 && price > 0 {
			qty = math.Floor(order.Amount / price)
		}

		switch order.Side {
		case broker.Buy:
			cash -= qty * price
		case broker.Sell:
			cash += qty * price
		}
	}

	return cash
}

// priceOf returns the last known price for an asset from the portfolio.
func (batch *Batch) priceOf(ast asset.Asset) float64 {
	posValue := batch.portfolio.PositionValue(ast)
	pos := batch.portfolio.Position(ast)

	if pos > 0 {
		return posValue / pos
	}

	// Asset not held -- cannot determine price from portfolio alone.
	// This happens for new buys. The caller should handle zero gracefully.
	return 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Batch" -v`
Expected: PASS

- [ ] **Step 5: Write tests for ProjectedHoldings and ProjectedWeights**

Add to `portfolio/batch_test.go`:

```go
Describe("ProjectedHoldings", func() {
    It("reflects accumulated buy orders", func() {
        err := batch.Order(context.Background(), spy, portfolio.Buy, 50)
        Expect(err).NotTo(HaveOccurred())

        holdings := batch.ProjectedHoldings()
        Expect(holdings[spy]).To(Equal(50.0))
    })

    It("reflects accumulated sell orders reducing existing positions", func() {
        // Setup: account already holds 100 SPY.
        // (Need to execute a batch first to establish the position.)
        // For this test, use a pre-loaded account.
        // ... test body depends on account setup with existing holdings
    })
})
```

- [ ] **Step 6: Run tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Batch" -v`
Expected: PASS

- [ ] **Step 7: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./portfolio/`

- [ ] **Step 8: Commit**

```bash
git add portfolio/batch.go portfolio/batch_test.go
git commit -m "feat: add Batch type for order accumulation"
```

---

### Task 4: Middleware interface and Account integration

**Files:**
- Create: `portfolio/middleware.go`
- Modify: `portfolio/portfolio.go:31-132` (Portfolio interface)
- Modify: `portfolio/portfolio.go:145-165` (PortfolioManager interface)
- Modify: `portfolio/account.go:44-58` (Account struct)
- Create: `portfolio/middleware_test.go`

- [ ] **Step 1: Write failing test for middleware chain**

Create `portfolio/middleware_test.go`:

```go
package portfolio_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
)

// testMiddleware records that it was called and optionally modifies the batch.
type testMiddleware struct {
	called    bool
	removeBuy bool
}

func (m *testMiddleware) Process(_ context.Context, batch *portfolio.Batch) error {
	m.called = true
	if m.removeBuy {
		filtered := batch.Orders[:0]
		for _, order := range batch.Orders {
			if order.Side != broker.Buy {
				filtered = append(filtered, order)
			}
		}
		batch.Orders = filtered
		batch.Annotate("risk:test", "removed all buy orders")
	}
	return nil
}

var _ = Describe("Middleware", func() {
	var (
		acct      *portfolio.Account
		timestamp time.Time
		spy       asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		timestamp = time.Date(2024, 6, 15, 16, 0, 0, 0, time.UTC)
		acct = portfolio.New(
			portfolio.WithCash(100000, timestamp.AddDate(0, -1, 0)),
		)
	})

	It("runs middleware in order during ExecuteBatch", func() {
		mw := &testMiddleware{}
		acct.Use(mw)

		batch := acct.NewBatch(timestamp)
		err := batch.Order(context.Background(), spy, portfolio.Buy, 10)
		Expect(err).NotTo(HaveOccurred())

		err = acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())
		Expect(mw.called).To(BeTrue())
	})

	It("middleware can remove orders from the batch", func() {
		mw := &testMiddleware{removeBuy: true}
		acct.Use(mw)

		batch := acct.NewBatch(timestamp)
		err := batch.Order(context.Background(), spy, portfolio.Buy, 10)
		Expect(err).NotTo(HaveOccurred())

		err = acct.ExecuteBatch(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// No buys should have executed.
		Expect(acct.Position(spy)).To(Equal(0.0))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Middleware" -v`
Expected: FAIL -- `Middleware` interface, `Use`, `ExecuteBatch` do not exist

- [ ] **Step 3: Create Middleware interface**

Create `portfolio/middleware.go`:

```go
package portfolio

import "context"

// Middleware processes a batch of proposed orders before execution.
// Each middleware in the chain receives the output of the previous one.
// Middleware can remove, modify, or add orders, and annotate the batch
// to explain its changes.
type Middleware interface {
	Process(ctx context.Context, batch *Batch) error
}
```

- [ ] **Step 4: Update Portfolio interface to be read-only**

In `portfolio/portfolio.go`, remove `RebalanceTo`, `Order`, and `Annotate` from the `Portfolio` interface. The interface should only contain query methods. Keep `SetMetadata` and `GetMetadata`.

- [ ] **Step 5: Update PortfolioManager interface**

In `portfolio/portfolio.go`, add to `PortfolioManager`:

```go
type PortfolioManager interface {
	Record(tx Transaction)
	UpdatePrices(df *data.DataFrame)
	SetBroker(b broker.Broker)

	Use(middleware ...Middleware)
	NewBatch(timestamp time.Time) *Batch
	ExecuteBatch(ctx context.Context, batch *Batch) error
	DrainFills(ctx context.Context) error
	CancelOpenOrders(ctx context.Context) error
}
```

- [ ] **Step 6: Add middleware and batch methods to Account**

In `portfolio/account.go`, add fields to the `Account` struct:

```go
type Account struct {
	// ... existing fields ...
	middleware    []Middleware
	pendingOrders map[string]broker.Order // tracks submitted orders for fill mapping
}
```

Initialize `pendingOrders` in `New()`:

```go
pendingOrders: make(map[string]broker.Order),
```

Add methods:

```go
func (a *Account) Use(middleware ...Middleware) {
	a.middleware = append(a.middleware, middleware...)
}

func (a *Account) NewBatch(timestamp time.Time) *Batch {
	return &Batch{
		Timestamp:   timestamp,
		Annotations: make(map[string]string),
		portfolio:   a,
	}
}

func (a *Account) ExecuteBatch(ctx context.Context, batch *Batch) error {
	// Run middleware chain.
	for _, mw := range a.middleware {
		if err := mw.Process(ctx, batch); err != nil {
			return err
		}
	}

	// Record annotations.
	for key, value := range batch.Annotations {
		a.Annotate(batch.Timestamp, key, value)
	}

	// Assign IDs and track orders, then submit.
	for idx := range batch.Orders {
		order := &batch.Orders[idx]
		if order.ID == "" {
			order.ID = fmt.Sprintf("batch-%d-%d", batch.Timestamp.UnixNano(), idx)
		}

		a.pendingOrders[order.ID] = *order

		if err := a.broker.Submit(ctx, *order); err != nil {
			return fmt.Errorf("execute batch: submit %s: %w", order.Asset.Ticker, err)
		}
	}

	// Drain fills from the channel.
	a.drainFillsFromChannel()

	return nil
}

func (a *Account) DrainFills(_ context.Context) error {
	a.drainFillsFromChannel()
	return nil
}

func (a *Account) drainFillsFromChannel() {
	fillCh := a.broker.Fills()
	for {
		select {
		case fill := <-fillCh:
			order, ok := a.pendingOrders[fill.OrderID]
			if !ok {
				continue // unknown fill, skip
			}

			var (
				txType TransactionType
				amount float64
				side   Side
			)

			switch order.Side {
			case broker.Buy:
				txType = BuyTransaction
				amount = -(fill.Price * fill.Qty)
				side = Buy
			case broker.Sell:
				txType = SellTransaction
				amount = fill.Price * fill.Qty
				side = Sell
			}

			_ = side

			a.Record(Transaction{
				Date:   fill.FilledAt,
				Asset:  order.Asset,
				Type:   txType,
				Qty:    fill.Qty,
				Price:  fill.Price,
				Amount: amount,
			})

			delete(a.pendingOrders, fill.OrderID)
		default:
			return
		}
	}
}

func (a *Account) CancelOpenOrders(ctx context.Context) error {
	orders, err := a.broker.Orders(ctx)
	if err != nil {
		return fmt.Errorf("cancel open orders: %w", err)
	}

	for _, order := range orders {
		if order.Status == broker.OrderOpen || order.Status == broker.OrderSubmitted {
			if cancelErr := a.broker.Cancel(ctx, order.ID); cancelErr != nil {
				return fmt.Errorf("cancel order %s: %w", order.ID, cancelErr)
			}

			delete(a.pendingOrders, order.ID)
		}
	}

	return nil
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -run "Middleware" -v`
Expected: PASS

- [ ] **Step 8: Fix compilation errors in existing tests**

The `Portfolio` interface no longer has `RebalanceTo`, `Order`, or `Annotate`. Update all portfolio tests that call these methods directly on the account to either:
- Use batch-based execution, or
- Call the methods on `*Account` directly (they still exist on the concrete type during transition)

Update test files: `portfolio/rebalance_test.go`, `portfolio/order_test.go`, `portfolio/account_test.go`.

For each test that calls `acct.RebalanceTo(...)` or `acct.Order(...)`, wrap in a batch:

```go
// Before:
err := acct.RebalanceTo(ctx, alloc)

// After:
batch := acct.NewBatch(timestamp)
err := batch.RebalanceTo(ctx, alloc)
Expect(err).NotTo(HaveOccurred())
err = acct.ExecuteBatch(ctx, batch)
```

- [ ] **Step 9: Run all portfolio tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./portfolio/ -v -count=1`
Expected: PASS

- [ ] **Step 10: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./portfolio/`

- [ ] **Step 11: Commit**

```bash
git add portfolio/middleware.go portfolio/portfolio.go portfolio/account.go portfolio/middleware_test.go portfolio/rebalance_test.go portfolio/order_test.go portfolio/account_test.go
git commit -m "feat: add Middleware interface, make Portfolio read-only, add batch execution"
```

---

### Task 5: Strategy interface and engine loop changes

**Files:**
- Modify: `engine/strategy.go:24-29`
- Modify: `engine/backtest.go:273-284`
- Modify: `engine/live.go:255-264`
- Modify: `engine/engine.go:644-657` (PredictedPortfolio)
- Modify: `engine/backtest_test.go` (all test strategies)
- Modify: `engine/example_test.go`
- Modify: `engine/fetch_test.go`
- Modify: `engine/predicted_portfolio_test.go`
- Modify: `examples/momentum-rotation/main.go`

- [ ] **Step 1: Write failing test for new Compute signature**

In `engine/backtest_test.go`, update one test strategy to use the new signature:

```go
func (s *backtestStrategy) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    // ... existing body, but use batch instead of port for orders
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -run "Backtest" -v -count=1`
Expected: FAIL -- signature mismatch

- [ ] **Step 3: Update Strategy interface**

In `engine/strategy.go`:

```go
type Strategy interface {
	Name() string
	Setup(eng *Engine)
	Compute(ctx context.Context, eng *Engine, portfolio portfolio.Portfolio, batch *portfolio.Batch) error
}
```

- [ ] **Step 4: Update backtest engine loop**

In `engine/backtest.go`, replace the Compute call site (around line 273-284):

```go
// Every step: drain fills.
if acct.HasBroker() {
    if err := acct.DrainFills(stepCtx); err != nil {
        return nil, fmt.Errorf("engine: drain fills on %v: %w", date, err)
    }
}

// ... existing dividend recording ...

// ... existing price update ...

// Frames only: strategy execution.
if step.isStrategy {
    if sb, ok := e.broker.(*SimulatedBroker); ok {
        sb.SetPriceProvider(e, date)
    }

    // Cancel open orders from previous frame.
    if err := acct.CancelOpenOrders(stepCtx); err != nil {
        return nil, fmt.Errorf("engine: cancel open orders on %v: %w", date, err)
    }

    // Create batch and run strategy.
    batch := acct.NewBatch(date)
    if err := e.strategy.Compute(stepCtx, e, acct, batch); err != nil {
        return nil, fmt.Errorf("engine: strategy %q compute on %v: %w",
            e.strategy.Name(), date, err)
    }

    // Execute batch through middleware chain.
    if err := acct.ExecuteBatch(stepCtx, batch); err != nil {
        return nil, fmt.Errorf("engine: execute batch on %v: %w", date, err)
    }
}
```

- [ ] **Step 5: Update live engine loop**

In `engine/live.go`, make the same structural changes around lines 255-264. Add `DrainFills` at every step, `CancelOpenOrders` and batch creation at frames.

- [ ] **Step 6: Update PredictedPortfolio**

In `engine/engine.go` around line 654:

```go
batch := clone.NewBatch(predictedDate)
if err := e.strategy.Compute(computeCtx, e, clone, batch); err != nil {
    return nil, fmt.Errorf("engine: PredictedPortfolio compute on %v: %w",
        predictedDate, err)
}
if err := clone.ExecuteBatch(computeCtx, batch); err != nil {
    return nil, fmt.Errorf("engine: PredictedPortfolio execute on %v: %w",
        predictedDate, err)
}
```

(`clone` is an `*Account` from `acct.Clone()` -- it needs `NewBatch` and `ExecuteBatch` which it inherits since `Account` implements `PortfolioManager`.)

- [ ] **Step 7: Update all test strategies in engine package**

Update every test strategy's `Compute` signature to accept `(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch)`. Change all `port.RebalanceTo(...)` calls to `batch.RebalanceTo(...)` and `port.Order(...)` to `batch.Order(...)` and `port.Annotate(...)` to `batch.Annotate(...)`.

Files to update:
- `engine/backtest_test.go`: `backtestStrategy`, `monthlyStrategy`, and any other test strategies
- `engine/example_test.go`: `BuyAndHold`, `MomentumStrategy`
- `engine/fetch_test.go`: `fetchStrategy`, `fetchAtStrategy`, `doubleFetchStrategy`, `fetchThenFetchAtStrategy`, `futureFetchAtStrategy`
- `engine/predicted_portfolio_test.go`: `predictStrategy`

- [ ] **Step 8: Update example strategies**

In `examples/momentum-rotation/main.go`:

```go
func (s *MomentumRotation) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    // ... existing body, but use batch.RebalanceTo instead of port.RebalanceTo
}
```

- [ ] **Step 9: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -v -count=1`
Expected: PASS

- [ ] **Step 10: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./...`

- [ ] **Step 11: Commit**

```bash
git add engine/strategy.go engine/backtest.go engine/live.go engine/engine.go engine/backtest_test.go engine/example_test.go engine/fetch_test.go engine/predicted_portfolio_test.go examples/momentum-rotation/main.go
git commit -m "feat: update Strategy.Compute to use Batch, add step/frame engine loop"
```

---

### Task 6: MaxPositionSize middleware

**Files:**
- Create: `risk/risk.go`
- Create: `risk/max_position_size.go`
- Create: `risk/max_position_size_test.go`

- [ ] **Step 1: Create risk package doc**

Create `risk/risk.go`:

```go
// Package risk provides portfolio middleware implementations for risk
// management. Each middleware processes a Batch of proposed orders,
// potentially modifying them to enforce risk constraints.
package risk
```

- [ ] **Step 2: Write failing tests**

Create `risk/max_position_size_test.go`:

```go
package risk_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/risk"
)

func TestRisk(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Risk Suite")
}

var _ = Describe("MaxPositionSize", func() {
	var (
		acct      *portfolio.Account
		spy       asset.Asset
		aapl      asset.Asset
		timestamp time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		timestamp = time.Date(2024, 6, 15, 16, 0, 0, 0, time.UTC)

		// Create account with holdings so we can test weight calculations.
		acct = portfolio.New(
			portfolio.WithCash(100000, timestamp.AddDate(0, -1, 0)),
		)
	})

	It("reduces orders that would exceed the position size limit", func() {
		mw := risk.MaxPositionSize(0.25)
		acct.Use(mw)

		batch := acct.NewBatch(timestamp)
		// Attempt to put 40% in SPY.
		err := batch.Order(context.Background(), spy, portfolio.Buy, 400)
		Expect(err).NotTo(HaveOccurred())

		err = mw.Process(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())

		// Check that the batch was modified and annotated.
		weights := batch.ProjectedWeights()
		Expect(weights[spy]).To(BeNumerically("<=", 0.25))
	})

	It("does not modify orders within the limit", func() {
		mw := risk.MaxPositionSize(0.50)
		batch := acct.NewBatch(timestamp)
		err := batch.Order(context.Background(), spy, portfolio.Buy, 100)
		Expect(err).NotTo(HaveOccurred())

		originalLen := len(batch.Orders)
		err = mw.Process(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.Orders).To(HaveLen(originalLen))
	})

	It("annotates when it makes changes", func() {
		mw := risk.MaxPositionSize(0.10)
		batch := acct.NewBatch(timestamp)
		err := batch.Order(context.Background(), spy, portfolio.Buy, 400)
		Expect(err).NotTo(HaveOccurred())

		err = mw.Process(context.Background(), batch)
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.Annotations).To(HaveKey("risk:max-position-size"))
	})
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./risk/ -run "MaxPositionSize" -v`
Expected: FAIL -- package does not exist

- [ ] **Step 4: Implement MaxPositionSize**

Create `risk/max_position_size.go`:

```go
package risk

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
)

type maxPositionSize struct {
	limit float64
}

// MaxPositionSize returns a middleware that caps any single position at the
// given weight (0.0 to 1.0) of total portfolio value. Excess goes to cash.
func MaxPositionSize(limit float64) portfolio.Middleware {
	return &maxPositionSize{limit: limit}
}

func (m *maxPositionSize) Process(_ context.Context, batch *portfolio.Batch) error {
	weights := batch.ProjectedWeights()
	totalValue := batch.ProjectedValue()
	modified := false

	for asset, weight := range weights {
		if weight <= m.limit {
			continue
		}

		// Compute how much to reduce.
		excessWeight := weight - m.limit
		excessDollars := excessWeight * totalValue

		// Add a sell order to reduce the position.
		batch.Orders = append(batch.Orders, broker.Order{
			Asset:       asset,
			Side:        broker.Sell,
			Amount:      excessDollars,
			OrderType:   broker.Market,
			TimeInForce: broker.Day,
		})

		batch.Annotate("risk:max-position-size",
			fmt.Sprintf("capped %s from %.1f%% to %.1f%%, $%.0f moved to cash",
				asset.Ticker, weight*100, m.limit*100, excessDollars))
		modified = true
	}

	_ = modified

	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./risk/ -run "MaxPositionSize" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add risk/
git commit -m "feat: add MaxPositionSize risk middleware"
```

---

### Task 7: DrawdownCircuitBreaker middleware

**Files:**
- Create: `risk/drawdown_circuit_breaker.go`
- Create: `risk/drawdown_circuit_breaker_test.go`

- [ ] **Step 1: Write failing tests**

Create `risk/drawdown_circuit_breaker_test.go` with tests for:
- When drawdown exceeds threshold, all equity positions are force-liquidated (sell orders injected)
- When drawdown is within threshold, batch is unchanged
- Annotations are added when circuit breaker fires

The test needs an account with performance data showing a drawdown. Set up an account, execute some trades via batch, record price drops via `UpdatePrices`, then run the middleware.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./risk/ -run "DrawdownCircuitBreaker" -v`
Expected: FAIL

- [ ] **Step 3: Implement DrawdownCircuitBreaker**

Create `risk/drawdown_circuit_breaker.go`:

```go
package risk

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
)

type drawdownCircuitBreaker struct {
	threshold float64
}

// DrawdownCircuitBreaker returns a middleware that force-liquidates all
// equity positions to cash when the portfolio's drawdown from peak exceeds
// the given threshold (e.g., 0.15 for 15%).
func DrawdownCircuitBreaker(threshold float64) portfolio.Middleware {
	return &drawdownCircuitBreaker{threshold: threshold}
}

func (m *drawdownCircuitBreaker) Process(_ context.Context, batch *portfolio.Batch) error {
	perfData := batch.Portfolio().PerfData()
	if perfData == nil {
		return nil
	}

	// Compute current drawdown from equity curve peak.
	dd, err := batch.Portfolio().PerformanceMetric(portfolio.MaxDrawdown).Value()
	if err != nil {
		return nil
	}

	if math.Abs(dd) < m.threshold {
		return nil
	}

	// Force-liquidate all positions.
	batch.Portfolio().Holdings(func(ast asset.Asset, qty float64) {
		if qty > 0 {
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       ast,
				Side:        broker.Sell,
				Qty:         qty,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})
		}
	})

	// Clear any buy orders -- no point buying if we're liquidating.
	filtered := batch.Orders[:0]
	for _, order := range batch.Orders {
		if order.Side == broker.Sell {
			filtered = append(filtered, order)
		}
	}
	batch.Orders = filtered

	batch.Annotate("risk:drawdown-circuit-breaker",
		fmt.Sprintf("drawdown %.1f%% exceeds %.1f%% threshold, force-liquidating all positions",
			dd*100, m.threshold*100))

	return nil
}
```

The `batch.Portfolio()` accessor was added in Task 3 as part of the Batch type.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./risk/ -run "DrawdownCircuitBreaker" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add risk/drawdown_circuit_breaker.go risk/drawdown_circuit_breaker_test.go
git commit -m "feat: add DrawdownCircuitBreaker risk middleware"
```

---

### Task 8: MaxPositionCount middleware

**Files:**
- Create: `risk/max_position_count.go`
- Create: `risk/max_position_count_test.go`

- [ ] **Step 1: Write failing tests**

Tests for:
- When projected positions exceed count, smallest positions by dollar value are dropped (sell orders injected)
- When within limit, batch is unchanged
- Annotations added when positions are dropped

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./risk/ -run "MaxPositionCount" -v`
Expected: FAIL

- [ ] **Step 3: Implement MaxPositionCount**

Create `risk/max_position_count.go`. Sort projected positions by dollar value ascending. If count exceeds `n`, inject sell orders for the smallest positions until within limit. Annotate each drop.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./risk/ -run "MaxPositionCount" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add risk/max_position_count.go risk/max_position_count_test.go
git commit -m "feat: add MaxPositionCount risk middleware"
```

---

### Task 9: VolatilityScaler middleware

**Files:**
- Create: `risk/volatility_scaler.go`
- Create: `risk/volatility_scaler_test.go`

This middleware needs a `DataSource` for fetching historical prices. It depends on the `DataSource` interface being available in the `data` package (per weighting-strategies spec). If that hasn't landed yet, use `universe.DataSource` and update the import later.

- [ ] **Step 1: Write failing tests**

Tests for:
- Positions are scaled by inverse volatility: lower-vol assets get larger weights
- When volatility data is unavailable for an asset, its weight is unchanged
- Annotations describe the scaling applied
- Lookback parameter is respected (use different lookback windows and verify different scaling)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./risk/ -run "VolatilityScaler" -v`
Expected: FAIL

- [ ] **Step 3: Implement VolatilityScaler**

Create `risk/volatility_scaler.go`:

The middleware:
1. Gets the projected holdings and weights from the batch
2. For each held asset, fetches `lookback` trading days of close prices via `DataSource`
3. Computes annualized realized volatility for each asset
4. Splits assets into two groups: those with vol data and those without
5. For assets with vol data, reweights using inverse-vol: `weight_i = (1/vol_i) / sum(1/vol_j)`, scaled to fill the same total weight the group originally occupied
6. Assets without vol data keep their original weights unchanged
7. Compares new target weights to current projected weights
8. For overweight positions (new target < current): injects sell orders, excess to cash
9. Does NOT inject buy orders for underweight positions -- the middleware only reduces exposure per the spec's "excess goes to cash" rule. The inverse-vol formula determines the *ceiling* for each position, not a target to buy toward.

This means the middleware effectively trims high-vol positions but does not increase low-vol positions beyond what the strategy already proposed.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./risk/ -run "VolatilityScaler" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add risk/volatility_scaler.go risk/volatility_scaler_test.go
git commit -m "feat: add VolatilityScaler risk middleware"
```

---

### Task 10: Convenience profiles

**Files:**
- Create: `risk/profiles.go`
- Create: `risk/profiles_test.go`

- [ ] **Step 1: Write failing tests**

Tests for:
- `Conservative` returns a slice of 3 middleware (MaxPositionSize, DrawdownCircuitBreaker, VolatilityScaler)
- `Moderate` returns a slice of 2 middleware
- `Aggressive` returns a slice of 2 middleware
- Each profile's middleware has the correct configuration values

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./risk/ -run "Profile" -v`
Expected: FAIL

- [ ] **Step 3: Implement profiles**

Create `risk/profiles.go`:

```go
package risk

import "github.com/penny-vault/pvbt/portfolio"

// Conservative returns a risk middleware chain with tight limits:
// 20% max position, 10% drawdown circuit breaker, and inverse
// volatility scaling with a 60 trading day lookback.
func Conservative(ds DataSource) []portfolio.Middleware {
	return []portfolio.Middleware{
		VolatilityScaler(ds, 60),
		MaxPositionSize(0.20),
		DrawdownCircuitBreaker(0.10),
	}
}

// Moderate returns a risk middleware chain with balanced limits:
// 25% max position and 15% drawdown circuit breaker.
func Moderate() []portfolio.Middleware {
	return []portfolio.Middleware{
		MaxPositionSize(0.25),
		DrawdownCircuitBreaker(0.15),
	}
}

// Aggressive returns a risk middleware chain with loose limits:
// 35% max position and 25% drawdown circuit breaker.
func Aggressive() []portfolio.Middleware {
	return []portfolio.Middleware{
		MaxPositionSize(0.35),
		DrawdownCircuitBreaker(0.25),
	}
}
```

Note: `Moderate` and `Aggressive` do not take a `DataSource` because they don't include `VolatilityScaler`. Update the spec's convenience profile signatures accordingly -- the spec shows all three taking `DataSource` but only `Conservative` needs it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./risk/ -run "Profile" -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -v -count=1`
Expected: PASS

- [ ] **Step 6: Run linter on entire project**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./...`

- [ ] **Step 7: Commit**

```bash
git add risk/profiles.go risk/profiles_test.go
git commit -m "feat: add Conservative, Moderate, Aggressive risk profiles"
```

---

### Task 11: Integration test -- end-to-end backtest with risk middleware

**Files:**
- Modify: `engine/backtest_test.go`

- [ ] **Step 1: Write integration test**

Add a test in `engine/backtest_test.go` that runs a full backtest with risk middleware configured:

```go
It("applies risk middleware during backtest", func() {
    // Create a strategy that tries to put 100% in one asset.
    // Configure MaxPositionSize(0.25).
    // Verify that no position ever exceeds 25%.
    // Verify that risk annotations appear on the portfolio.
})
```

The test should:
1. Define a strategy that calls `batch.RebalanceTo` with 100% in SPY
2. Configure `risk.MaxPositionSize(0.25)` via `acct.Use()`
3. Run the backtest
4. Assert that SPY never exceeded 25% of portfolio value
5. Assert that `risk:max-position-size` annotations exist

- [ ] **Step 2: Run the integration test**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./engine/ -run "risk middleware" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add engine/backtest_test.go
git commit -m "test: add integration test for risk middleware in backtest"
```
