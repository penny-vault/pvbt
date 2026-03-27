# Broker Batch Prefetch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate per-order data fetches in the simulated broker by having the engine batch-prefetch prices for all order assets before individual submission.

**Architecture:** The engine calls `Prices()` with all batch order assets between `strategy.Compute` and `acct.ExecuteBatch`, warming the year-chunk cache. Individual `Submit` calls then hit the cache. Also adds `Volume` to `Prices()` and batches `evaluatePartialRemainders`.

**Tech Stack:** Go, Ginkgo/Gomega

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `engine/engine.go:633-638` | Modify | Add `Volume` to `Prices()` metric list |
| `engine/backtest.go:366-398` | Modify | Prefetch before `ExecuteBatch` (parent + child) |
| `engine/simulated_broker.go:422-475` | Modify | Batch `evaluatePartialRemainders` |
| `engine/simulated_broker_test.go` | Modify | Add test for batched partial remainders |
| `engine/backtest_test.go` | Modify | Add test for prefetch before ExecuteBatch |

---

### Task 1: Add Volume to Engine.Prices()

**Files:**
- Modify: `engine/engine.go:633-638`

- [ ] **Step 1: Add `data.Volume` to the metric list in `Prices()`**

In `engine/engine.go`, change `Prices()` from:

```go
func (e *Engine) Prices(ctx context.Context, assets ...asset.Asset) (*data.DataFrame, error) {
	return e.FetchAt(ctx, assets, e.currentDate, []data.Metric{
		data.MetricClose, data.MetricHigh, data.MetricLow,
		data.Dividend, data.SplitFactor,
	})
}
```

to:

```go
func (e *Engine) Prices(ctx context.Context, assets ...asset.Asset) (*data.DataFrame, error) {
	return e.FetchAt(ctx, assets, e.currentDate, []data.Metric{
		data.MetricClose, data.MetricHigh, data.MetricLow,
		data.Volume, data.Dividend, data.SplitFactor,
	})
}
```

Update the doc comment to mention volume:

```go
// Prices implements broker.PriceProvider. It returns close, high, low,
// and volume prices plus dividend/split data for the requested assets at
// the engine's current simulation date. High and low are needed by
// EvaluatePending for intrabar bracket order evaluation. Volume is needed
// by the MarketImpact fill adjuster.
```

- [ ] **Step 2: Run tests to verify nothing breaks**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt/.worktrees/broker-batch-prefetch && ginkgo run -race ./engine/`
Expected: PASS (existing tests don't assert on the exact metric list)

- [ ] **Step 3: Commit**

```bash
git add engine/engine.go
git commit -m "engine: add Volume to Prices() for MarketImpact fill adjuster"
```

---

### Task 2: Prefetch broker prices before ExecuteBatch in backtest loop

**Files:**
- Modify: `engine/backtest.go:366-398`

- [ ] **Step 1: Add prefetch before parent strategy ExecuteBatch**

In `engine/backtest.go`, between the `strategy.Compute` call (line 390) and the `acct.ExecuteBatch` call (line 396), insert a prefetch. The block currently reads:

```go
		// 16. Create batch and run strategy.
		batch := acct.NewBatch(date)
		if err := e.strategy.Compute(stepCtx, e, acct, batch); err != nil {
			return nil, fmt.Errorf("engine: strategy %q compute on %v: %w",
				e.strategy.Name(), date, err)
		}

		// Execute batch through middleware chain.
		if err := acct.ExecuteBatch(stepCtx, batch); err != nil {
			return nil, fmt.Errorf("engine: execute batch on %v: %w", date, err)
		}
```

Change it to:

```go
		// 16. Create batch and run strategy.
		batch := acct.NewBatch(date)
		if err := e.strategy.Compute(stepCtx, e, acct, batch); err != nil {
			return nil, fmt.Errorf("engine: strategy %q compute on %v: %w",
				e.strategy.Name(), date, err)
		}

		// Prefetch broker prices for all order assets so that
		// per-order Submit calls hit the year-chunk cache.
		if err := e.prefetchBrokerPrices(stepCtx, batch.Orders); err != nil {
			return nil, fmt.Errorf("engine: prefetch broker prices on %v: %w", date, err)
		}

		// Execute batch through middleware chain.
		if err := acct.ExecuteBatch(stepCtx, batch); err != nil {
			return nil, fmt.Errorf("engine: execute batch on %v: %w", date, err)
		}
```

- [ ] **Step 2: Add prefetch before child strategy ExecuteBatch**

In the same file, the child strategy loop (lines 366-374) currently reads:

```go
		childBatch := child.account.NewBatch(date)
		if err := child.strategy.Compute(stepCtx, e, child.account, childBatch); err != nil {
			return nil, fmt.Errorf("engine: child %q compute on %v: %w", childName, date, err)
		}

		if err := child.account.ExecuteBatch(stepCtx, childBatch); err != nil {
			return nil, fmt.Errorf("engine: child %q execute batch on %v: %w", childName, date, err)
		}
```

Change it to:

```go
		childBatch := child.account.NewBatch(date)
		if err := child.strategy.Compute(stepCtx, e, child.account, childBatch); err != nil {
			return nil, fmt.Errorf("engine: child %q compute on %v: %w", childName, date, err)
		}

		if err := e.prefetchBrokerPrices(stepCtx, childBatch.Orders); err != nil {
			return nil, fmt.Errorf("engine: child %q prefetch broker prices on %v: %w", childName, date, err)
		}

		if err := child.account.ExecuteBatch(stepCtx, childBatch); err != nil {
			return nil, fmt.Errorf("engine: child %q execute batch on %v: %w", childName, date, err)
		}
```

- [ ] **Step 3: Add `prefetchBrokerPrices` method to Engine**

Add the following method to `engine/engine.go`, after the `Prices` method (after line 638):

```go
// prefetchBrokerPrices batch-fetches price data for all unique assets
// in the given orders, warming the year-chunk cache so that subsequent
// per-order Prices calls in Submit are cache hits.
func (e *Engine) prefetchBrokerPrices(ctx context.Context, orders []broker.Order) error {
	if len(orders) == 0 {
		return nil
	}

	assetSet := make(map[string]asset.Asset, len(orders))
	for idx := range orders {
		assetSet[orders[idx].Asset.CompositeFigi] = orders[idx].Asset
	}

	assets := make([]asset.Asset, 0, len(assetSet))
	for _, held := range assetSet {
		assets = append(assets, held)
	}

	_, err := e.Prices(ctx, assets...)
	if err != nil {
		return fmt.Errorf("prefetch broker prices: %w", err)
	}

	return nil
}
```

This requires adding `"github.com/penny-vault/pvbt/broker"` to the imports in `engine/engine.go` if not already present. Check before adding.

- [ ] **Step 4: Run tests to verify nothing breaks**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt/.worktrees/broker-batch-prefetch && ginkgo run -race ./engine/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add engine/engine.go engine/backtest.go
git commit -m "engine: prefetch broker prices before ExecuteBatch"
```

---

### Task 3: Batch-fetch in evaluatePartialRemainders

**Files:**
- Modify: `engine/simulated_broker.go:422-475`

- [ ] **Step 1: Write the failing test**

Add a test in `engine/simulated_broker_test.go` inside the existing `Describe("SimulatedBroker", ...)` block. This test verifies that partial remainder evaluation uses one batched `Prices` call, not N individual calls.

Add a `countingPriceProvider` mock near the other mocks (around line 80):

```go
// countingPriceProvider wraps another PriceProvider and counts Prices calls.
type countingPriceProvider struct {
	inner     broker.PriceProvider
	callCount int
}

func (cp *countingPriceProvider) Prices(ctx context.Context, assets ...asset.Asset) (*data.DataFrame, error) {
	cp.callCount++
	return cp.inner.Prices(ctx, assets...)
}
```

Add a `drainFills` helper after the mocks:

```go
func drainFills(sb *engine.SimulatedBroker, count int) []broker.Fill {
	fills := make([]broker.Fill, 0, count)
	ch := sb.Fills()
	for range count {
		select {
		case fl := <-ch:
			fills = append(fills, fl)
		default:
			return fills
		}
	}
	return fills
}
```

Add the test inside the `SimulatedBroker` Describe:

```go
	Context("evaluatePartialRemainders batching", func() {
		It("fetches prices for all partial remainders in a single call", func() {
			date1 := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
			date2 := time.Date(2024, 6, 16, 0, 0, 0, 0, time.UTC)
			msft := asset.Asset{CompositeFigi: "BBG000BPH459", Ticker: "MSFT"}

			partialPipeline := broker.NewPipeline(
				broker.FillAtClose(),
				[]broker.FillAdjuster{broker.VolumeLimit(0.10)},
			)

			provider1 := &countingPriceProvider{
				inner: &mockVolumePriceProvider{
					close:  map[asset.Asset]float64{aapl: 150.0, msft: 400.0},
					volume: map[asset.Asset]float64{aapl: 50, msft: 50},
					date:   date1,
				},
			}

			simBroker := engine.NewSimulatedBroker()
			simBroker.SetFillPipeline(partialPipeline)
			simBroker.SetPriceProvider(provider1, date1)

			// Submit orders that will be partially filled
			// (qty 100 but volume limit caps at 10% of 50 = 5 shares).
			err := simBroker.Submit(context.Background(), broker.Order{
				ID: "o1", Asset: aapl, Side: broker.Buy, Qty: 100,
			})
			Expect(err).NotTo(HaveOccurred())

			err = simBroker.Submit(context.Background(), broker.Order{
				ID: "o2", Asset: msft, Side: broker.Buy, Qty: 100,
			})
			Expect(err).NotTo(HaveOccurred())

			// Drain the partial fills from bar 1.
			drainFills(simBroker, 2)

			// Advance to next bar with a fresh counting provider.
			provider2 := &countingPriceProvider{
				inner: &mockVolumePriceProvider{
					close:  map[asset.Asset]float64{aapl: 155.0, msft: 410.0},
					volume: map[asset.Asset]float64{aapl: 1000, msft: 1000},
					date:   date2,
				},
			}
			simBroker.SetPriceProvider(provider2, date2)

			// EvaluatePending triggers evaluatePartialRemainders.
			simBroker.EvaluatePending()

			// The partial remainder evaluation should make exactly
			// 1 batched call for both assets, not 2 individual calls.
			Expect(provider2.callCount).To(Equal(1))
		})
	})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt/.worktrees/broker-batch-prefetch && ginkgo run -race -focus "fetches prices for all partial remainders" ./engine/`
Expected: FAIL -- `provider2.callCount` is 2 (one per asset), not 1.

- [ ] **Step 3: Refactor evaluatePartialRemainders to batch**

Replace the `evaluatePartialRemainders` method in `engine/simulated_broker.go` (lines 422-475):

```go
// evaluatePartialRemainders retries partial fill remainders from prior bars.
// After two bars without a full fill, the remainder is cancelled.
func (b *SimulatedBroker) evaluatePartialRemainders() {
	if b.prices == nil || len(b.partialRemainders) == 0 {
		return
	}

	ctx := context.Background()

	// Prune expired remainders and collect unique assets.
	assetSet := make(map[string]asset.Asset)

	for orderID, pr := range b.partialRemainders {
		if pr.bars >= 2 {
			delete(b.partialRemainders, orderID)
			continue
		}

		assetSet[pr.order.Asset.CompositeFigi] = pr.order.Asset
	}

	if len(b.partialRemainders) == 0 {
		return
	}

	// Batch-fetch prices for all remaining assets.
	assets := make([]asset.Asset, 0, len(assetSet))
	for _, held := range assetSet {
		assets = append(assets, held)
	}

	df, err := b.prices.Prices(ctx, assets...)
	if err != nil {
		return
	}

	// Evaluate each remainder against the batched DataFrame.
	for orderID, pr := range b.partialRemainders {
		result, fillErr := b.fillPipeline.Fill(ctx, pr.order, df)
		if fillErr != nil {
			continue
		}

		if result.Quantity > 0 {
			b.fills <- broker.Fill{
				OrderID:  orderID,
				Price:    result.Price,
				Qty:      result.Quantity,
				FilledAt: b.date,
			}
		}

		if !result.Partial {
			delete(b.partialRemainders, orderID)
		} else {
			remainderQty := pr.order.Qty - result.Quantity
			if remainderQty > 0 {
				updatedOrder := pr.order
				updatedOrder.Qty = remainderQty

				b.partialRemainders[orderID] = partialRemainder{
					order: updatedOrder,
					bars:  pr.bars + 1,
				}
			} else {
				delete(b.partialRemainders, orderID)
			}
		}
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt/.worktrees/broker-batch-prefetch && ginkgo run -race -focus "fetches prices for all partial remainders" ./engine/`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt/.worktrees/broker-batch-prefetch && ginkgo run -race ./...`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add engine/simulated_broker.go engine/simulated_broker_test.go
git commit -m "engine: batch-fetch prices in evaluatePartialRemainders"
```

---

### Task 4: Lint and final verification

**Files:**
- All modified files

- [ ] **Step 1: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt/.worktrees/broker-batch-prefetch && make lint`
Expected: no lint errors. If there are errors, fix them.

- [ ] **Step 2: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt/.worktrees/broker-batch-prefetch && make test`
Expected: all tests pass.

- [ ] **Step 3: Fix any issues and commit**

If lint or tests revealed issues, fix and commit:

```bash
git add -u
git commit -m "fix: address lint issues from broker prefetch changes"
```
