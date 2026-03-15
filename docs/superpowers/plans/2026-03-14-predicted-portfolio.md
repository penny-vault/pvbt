# Predicted Portfolio Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `Engine.PredictedPortfolio` that runs the strategy's Compute against a shadow portfolio with forward-filled data to predict what trades would happen on the next scheduled trade date.

**Architecture:** Clone the current Account, set `currentDate` to the next trade date, enable a forward-fill flag on the engine so `Fetch`/`FetchAt` extend returned DataFrames to cover the predicted date, run Compute against the clone with a fresh SimulatedBroker, restore engine state, return the clone.

**Tech Stack:** Go, Ginkgo/Gomega, tradecron

**Spec:** `docs/superpowers/specs/2026-03-14-predicted-portfolio-design.md`

---

## Chunk 1: Account.Clone and forwardFillTo

### Task 1: Account.Clone

**Files:**
- Modify: `portfolio/account.go`
- Test: `portfolio/account_test.go`

- [ ] **Step 1: Write tests for Account.Clone**

Add to `portfolio/account_test.go`:

```go
var _ = Describe("Account.Clone", func() {
	It("preserves holdings, cash, metadata, and annotations", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}

		acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))
		acct.SetMetadata("strategy", "adm")
		acct.Annotate(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Unix(), "signal", "0.5")

		// Simulate a holding by recording a buy.
		acct.Record(portfolio.Transaction{
			Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    100,
			Price:  500,
			Amount: -50_000,
		})

		clone := acct.Clone()

		Expect(clone.Cash()).To(Equal(acct.Cash()))
		Expect(clone.Position(spy)).To(Equal(acct.Position(spy)))
		Expect(clone.GetMetadata("strategy")).To(Equal("adm"))
		Expect(clone.Annotations()).To(HaveLen(1))
	})

	It("isolates clone mutations from original", func() {
		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
		acct.SetMetadata("key", "original")
		acct.Annotate(100, "signal", "0.5")

		clone := acct.Clone()

		// Mutate clone.
		clone.SetMetadata("key", "mutated")
		clone.Annotate(200, "other", "1.0")

		// Original unchanged.
		Expect(acct.GetMetadata("key")).To(Equal("original"))
		Expect(acct.Annotations()).To(HaveLen(1))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/ -v -count=1 2>&1 | grep -i clone`
Expected: FAIL (Clone method does not exist)

- [ ] **Step 3: Implement Account.Clone**

Add to `portfolio/account.go`:

```go
// Clone returns a deep copy of the Account suitable for prediction runs.
// Holdings, metadata, and annotations are independent copies. PerfData is
// deep-copied via DataFrame.Copy. Transactions and tax lots are shallow-copied
// (the clone gets its own slice header but shares the underlying elements,
// which is safe since appending to the clone's slice does not affect the
// original).
func (acct *Account) Clone() *Account {
	holdings := make(map[asset.Asset]float64, len(acct.holdings))
	for asset, qty := range acct.holdings {
		holdings[asset] = qty
	}

	metadata := make(map[string]string, len(acct.metadata))
	for key, val := range acct.metadata {
		metadata[key] = val
	}

	annotations := make([]Annotation, len(acct.annotations))
	copy(annotations, acct.annotations)

	transactions := make([]Transaction, len(acct.transactions))
	copy(transactions, acct.transactions)

	taxLots := make(map[asset.Asset][]TaxLot, len(acct.taxLots))
	for asset, lots := range acct.taxLots {
		lotsCopy := make([]TaxLot, len(lots))
		copy(lotsCopy, lots)
		taxLots[asset] = lotsCopy
	}

	clone := &Account{
		cash:              acct.cash,
		holdings:          holdings,
		transactions:      transactions,
		broker:            acct.broker,
		prices:            acct.prices,
		benchmark:         acct.benchmark,
		riskFree:          acct.riskFree,
		taxLots:           taxLots,
		metadata:          metadata,
		metrics:           acct.metrics,
		registeredMetrics: acct.registeredMetrics,
		annotations:       annotations,
	}

	if acct.perfData != nil {
		clone.perfData = acct.perfData.Copy()
	}

	return clone
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./portfolio/ -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go
git commit -m "feat: add Account.Clone for prediction shadow copies"
```

### Task 2: Engine.forwardFillTo helper

**Files:**
- Modify: `engine/engine.go`
- Create: `engine/forward_fill_test.go`

- [ ] **Step 1: Write tests for forwardFillTo**

Create `engine/forward_fill_test.go`:

```go
package engine_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
)

var _ = Describe("ForwardFillTo", func() {
	var spy asset.Asset

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
	})

	It("fills daily data across multiple days", func() {
		lastDate := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		targetDate := time.Date(2024, 1, 19, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{lastDate},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500.0},
		)
		Expect(err).NotTo(HaveOccurred())

		result, err := engine.ForwardFillTo(df, targetDate)
		Expect(err).NotTo(HaveOccurred())

		// Original + 4 days (16th, 17th, 18th, 19th)
		Expect(result.Len()).To(Equal(5))
		Expect(result.End()).To(Equal(targetDate))

		// All filled values should match the last available row.
		lastValue := result.ValueAt(spy, data.MetricClose, targetDate)
		Expect(lastValue).To(Equal(500.0))
	})

	It("is a no-op when DataFrame already covers the target date", func() {
		targetDate := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{targetDate},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{500.0},
		)
		Expect(err).NotTo(HaveOccurred())

		result, err := engine.ForwardFillTo(df, targetDate)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
	})

	It("returns error for Tick frequency", func() {
		lastDate := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		targetDate := time.Date(2024, 1, 16, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{lastDate},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Tick,
			[]float64{500.0},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = engine.ForwardFillTo(df, targetDate)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("tick"))
	})

	It("fills weekly data with 7-day spacing", func() {
		lastDate := time.Date(2024, 1, 5, 16, 0, 0, 0, time.UTC)
		targetDate := time.Date(2024, 1, 26, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{lastDate},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Weekly,
			[]float64{500.0},
		)
		Expect(err).NotTo(HaveOccurred())

		result, err := engine.ForwardFillTo(df, targetDate)
		Expect(err).NotTo(HaveOccurred())

		// Original + 3 weeks (12th, 19th, 26th)
		Expect(result.Len()).To(Equal(4))
		Expect(result.End()).To(Equal(targetDate))
	})

	It("fills monthly data with 1-month spacing", func() {
		lastDate := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		targetDate := time.Date(2024, 3, 15, 16, 0, 0, 0, time.UTC)

		df, err := data.NewDataFrame(
			[]time.Time{lastDate},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Monthly,
			[]float64{500.0},
		)
		Expect(err).NotTo(HaveOccurred())

		result, err := engine.ForwardFillTo(df, targetDate)
		Expect(err).NotTo(HaveOccurred())

		// Original + 2 months (Feb 15, Mar 15)
		Expect(result.Len()).To(Equal(3))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./engine/ -run ForwardFillTo -v -count=1`
Expected: FAIL (ForwardFillTo does not exist)

- [ ] **Step 3: Implement ForwardFillTo**

Add to `engine/engine.go`:

```go
// ForwardFillTo extends a DataFrame by copying the last row's values forward
// to the target date, spaced according to the DataFrame's frequency. Returns
// the original DataFrame unchanged if it already covers the target date.
// Returns an error for Tick frequency since it has no regular interval.
func ForwardFillTo(df *data.DataFrame, targetDate time.Time) (*data.DataFrame, error) {
	if df.Len() == 0 {
		return df, nil
	}

	if !df.End().Before(targetDate) {
		return df, nil
	}

	freq := df.Frequency()
	if freq == data.Tick {
		return nil, fmt.Errorf("cannot forward-fill tick-frequency data: no regular interval")
	}

	// Extract the last row's values.
	assets := df.AssetList()
	metrics := df.MetricList()
	lastRow := make([]float64, len(assets)*len(metrics))
	for assetIdx, asset := range assets {
		for metricIdx, metric := range metrics {
			lastRow[assetIdx*len(metrics)+metricIdx] = df.Value(asset, metric)
		}
	}

	// Generate fill timestamps and append rows.
	cursor := df.End()
	for {
		cursor = nextTimestamp(cursor, freq)
		if cursor.After(targetDate) {
			break
		}
		if err := df.AppendRow(cursor, lastRow); err != nil {
			return nil, fmt.Errorf("forward-fill append at %v: %w", cursor, err)
		}
	}

	return df, nil
}

// nextTimestamp advances a timestamp by one frequency step.
func nextTimestamp(current time.Time, freq data.Frequency) time.Time {
	switch freq {
	case data.Daily:
		return current.AddDate(0, 0, 1)
	case data.Weekly:
		return current.AddDate(0, 0, 7)
	case data.Monthly:
		return current.AddDate(0, 1, 0)
	case data.Quarterly:
		return current.AddDate(0, 3, 0)
	case data.Yearly:
		return current.AddDate(1, 0, 0)
	default:
		return current.AddDate(0, 0, 1) // fallback to daily
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./engine/ -run ForwardFillTo -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add engine/engine.go engine/forward_fill_test.go
git commit -m "feat: add ForwardFillTo helper for prediction data gaps"
```

## Chunk 2: PredictedPortfolio method and integration tests

### Task 3: PredictedPortfolio method

**Files:**
- Modify: `engine/engine.go`

- [ ] **Step 1: Add predicting flag to Engine struct**

In `engine/engine.go`, add to the Engine struct (after `metricProvider`):

```go
	predicting bool
```

- [ ] **Step 2: Modify Fetch to forward-fill when predicting**

In `engine/engine.go`, at the end of the `Fetch` method (around line 283,
before the `return` statement), add:

```go
	if e.predicting && assembled.Len() > 0 && assembled.End().Before(e.currentDate) {
		filled, fillErr := ForwardFillTo(assembled, e.currentDate)
		if fillErr != nil {
			return nil, fmt.Errorf("engine: forward-fill in Fetch: %w", fillErr)
		}
		assembled = filled
	}
```

Note: Find the exact return point in `Fetch` where `assembled` (the result
from `fetchRange`) is about to be returned. The forward-fill goes just before
that return.

- [ ] **Step 3: Modify FetchAt to skip future-date check when predicting**

In `engine/engine.go`, the `FetchAt` method (around line 286) currently
rejects future dates:

```go
if !e.currentDate.IsZero() && t.After(e.currentDate) {
    return nil, fmt.Errorf("FetchAt: requested future date ...")
}
```

Change to:

```go
if !e.predicting && !e.currentDate.IsZero() && t.After(e.currentDate) {
    return nil, fmt.Errorf("FetchAt: requested future date ...")
}
```

Also add forward-fill after `fetchRange` returns in FetchAt (before the return):

```go
result, err := e.fetchRange(ctx, assets, metrics, t, t)
if err != nil {
    return nil, err
}

if e.predicting && result.Len() > 0 && result.End().Before(t) {
    filled, fillErr := ForwardFillTo(result, t)
    if fillErr != nil {
        return nil, fmt.Errorf("engine: forward-fill in FetchAt: %w", fillErr)
    }
    result = filled
}

return result, nil
```

- [ ] **Step 4: Implement PredictedPortfolio**

Add to `engine/engine.go`:

```go
// PredictedPortfolio runs the strategy's Compute against a shadow copy of the
// current portfolio using the next scheduled trade date. Data is forward-filled
// from the last available date to the predicted date. The strategy is unaware
// it is a prediction run. The returned Portfolio reflects what trades the
// strategy would make.
func (e *Engine) PredictedPortfolio(ctx context.Context) (portfolio.Portfolio, error) {
	if e.schedule == nil {
		return nil, fmt.Errorf("engine: PredictedPortfolio requires a schedule")
	}

	if e.account == nil {
		return nil, fmt.Errorf("engine: PredictedPortfolio requires an initialized account")
	}

	// 1. Determine the next trade date.
	predictedDate := e.schedule.Next(e.currentDate)

	// 2. Clone the current account.
	clone := e.account.Clone()

	// 3. Set up the shadow broker.
	shadowBroker := NewSimulatedBroker()
	clone.SetBroker(shadowBroker)

	// 4. Save and restore engine state.
	savedDate := e.currentDate
	e.predicting = true
	e.currentDate = predictedDate
	defer func() {
		e.currentDate = savedDate
		e.predicting = false
	}()

	// 5. Set the broker's price provider.
	shadowBroker.SetPriceProvider(e, predictedDate)

	// 6. Run Compute.
	if err := e.strategy.Compute(ctx, e, clone); err != nil {
		return nil, fmt.Errorf("engine: PredictedPortfolio compute on %v: %w",
			predictedDate, err)
	}

	return clone, nil
}
```

- [ ] **Step 5: Verify the engine stores the account**

Check that the `Backtest` method stores the account on the engine. Look for
`e.account = acct` in `backtest.go`. If it does not, add it after account
creation (around line 67 of backtest.go, after `acct := e.createAccount(start)`):

```go
e.account = acct
```

Similarly check `RunLive` in `live.go`.

- [ ] **Step 6: Run build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add engine/engine.go engine/backtest.go engine/live.go
git commit -m "feat: add PredictedPortfolio method with forward-fill support"
```

### Task 4: Integration tests for PredictedPortfolio

**Files:**
- Create: `engine/predicted_portfolio_test.go`

- [ ] **Step 1: Write the test file**

Create `engine/predicted_portfolio_test.go` with a test strategy and multiple
scenarios. The test strategy buys SPY on every Compute call (simple and
predictable):

```go
package engine_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
)

// predictStrategy always rebalances to 100% SPY.
type predictStrategy struct {
	spy      asset.Asset
	schedule string
}

func (s *predictStrategy) Name() string { return "predict-test" }

func (s *predictStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New(s.schedule, tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("predictStrategy.Setup: %v", err))
	}
	eng.Schedule(tc)
	s.spy = eng.Asset("SPY")
}

func (s *predictStrategy) Compute(ctx context.Context, eng *engine.Engine, portfolio portfolio.Portfolio) error {
	df, err := eng.FetchAt(ctx, []asset.Asset{s.spy}, eng.CurrentDate(), []data.Metric{data.MetricClose})
	if err != nil {
		return err
	}

	price := df.Value(s.spy, data.MetricClose)
	if price <= 0 {
		return nil
	}

	portfolio.Annotate(eng.CurrentDate().Unix(), "action", "buy SPY")

	return portfolio.RebalanceTo(ctx, portfolio.Allocation{
		Date:          eng.CurrentDate(),
		Members:       map[asset.Asset]float64{s.spy: 1.0},
		Justification: "always buy SPY",
	})
}

var _ = Describe("PredictedPortfolio", func() {
	var (
		spy           asset.Asset
		testAssets    []asset.Asset
		assetProvider *mockAssetProvider
		metrics       []data.Metric
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		testAssets = []asset.Asset{spy}
		assetProvider = &mockAssetProvider{assets: testAssets}
		metrics = []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
	})

	It("predicts trades mid-month for a monthly strategy", func() {
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		df := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, df)

		strategy := &predictStrategy{schedule: "@monthend"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000),
		)

		// Run backtest through mid-January.
		start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		// Predict what would happen at month-end.
		predicted, err := eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(predicted).NotTo(BeNil())

		// Should have buy transactions from the prediction.
		txns := predicted.Transactions()
		hasBuy := false
		for _, tx := range txns {
			if tx.Type == portfolio.BuyTransaction {
				hasBuy = true
				break
			}
		}
		Expect(hasBuy).To(BeTrue(), "predicted portfolio should contain buy transactions")
	})

	It("predicts with minimal forward-fill for day-before", func() {
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		df := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, df)

		strategy := &predictStrategy{schedule: "@monthend"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000),
		)

		// Run backtest through Jan 30 (day before month-end).
		start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		predicted, err := eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(predicted).NotTo(BeNil())
	})

	It("does not mutate the original account", func() {
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		df := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, df)

		strategy := &predictStrategy{schedule: "@monthend"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000),
		)

		start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		original, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		originalCash := original.Cash()
		originalTxnCount := len(original.Transactions())

		_, err = eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// Original should be unchanged.
		Expect(original.Cash()).To(Equal(originalCash))
		Expect(original.Transactions()).To(HaveLen(originalTxnCount))
	})

	It("includes annotations and justifications on predicted portfolio", func() {
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		df := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, df)

		strategy := &predictStrategy{schedule: "@monthend"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000),
		)

		start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		predicted, err := eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// Strategy adds an annotation.
		annotations := predicted.Annotations()
		Expect(annotations).NotTo(BeEmpty())

		// Strategy sets justification on the allocation.
		txns := predicted.Transactions()
		for _, tx := range txns {
			if tx.Type == portfolio.BuyTransaction {
				Expect(tx.Justification).To(Equal("always buy SPY"))
				break
			}
		}
	})

	It("works with a daily strategy", func() {
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		df := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, df)

		strategy := &predictStrategy{schedule: "0 16 * * 1-5"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000),
		)

		start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 2, 5, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		predicted, err := eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(predicted).NotTo(BeNil())
	})

	It("works with a weekly strategy", func() {
		dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		df := makeDailyTestData(dataStart, 400, testAssets, metrics)
		provider := data.NewTestProvider(metrics, df)

		// Every Monday at 16:00.
		strategy := &predictStrategy{schedule: "0 16 * * 1"}
		eng := engine.New(strategy,
			engine.WithDataProvider(provider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000),
		)

		start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2024, 2, 7, 0, 0, 0, 0, time.UTC)

		_, err := eng.Backtest(context.Background(), start, end)
		Expect(err).NotTo(HaveOccurred())

		predicted, err := eng.PredictedPortfolio(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(predicted).NotTo(BeNil())
	})

	It("returns error when no schedule is set", func() {
		strategy := &noScheduleStrategy{}
		eng := engine.New(strategy, engine.WithAssetProvider(assetProvider))

		_, err := eng.PredictedPortfolio(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("schedule"))
	})
})
```

Note: This test file uses `mockAssetProvider`, `makeDailyTestData`, and
`noScheduleStrategy` which are already defined in `engine/backtest_test.go`
(same package `engine_test`).

- [ ] **Step 2: Run tests**

Run: `go test ./engine/ -run PredictedPortfolio -v -count=1`
Expected: PASS (if PredictedPortfolio is already implemented from Step 4 above)

- [ ] **Step 3: Run full test suite**

Run: `go build ./... && go test ./...`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add engine/predicted_portfolio_test.go
git commit -m "test: add PredictedPortfolio integration tests"
```

### Task 5: Run linter and fix issues

- [ ] **Step 1: Run linter**

Run: `golangci-lint run ./...`

- [ ] **Step 2: Fix any issues introduced by this feature**

Only fix issues in files modified by this plan. Do not fix pre-existing issues
in unrelated files.

- [ ] **Step 3: Run tests to verify fixes**

Run: `go build ./... && go test ./...`
Expected: All pass

- [ ] **Step 4: Commit (if any fixes were needed)**

```bash
git add -A
git commit -m "fix: address lint issues from predicted portfolio implementation"
```
