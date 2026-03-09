# Portfolio Package Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the portfolio package -- Account internals, order execution, selection/weighting, tax lot tracking, and all 30+ performance metrics -- so the package is fully functional and tested.

**Architecture:** The `Account` struct implements both `Portfolio` (strategy-facing) and `PortfolioManager` (engine-facing) interfaces. It stores holdings, cash, a transaction log, an equity curve, benchmark/risk-free price series, and FIFO tax lots. Order execution is always delegated to a `broker.Broker`. Selection and weighting functions transform DataFrames into PortfolioPlan allocations. Performance metrics compute from equity curve, benchmark, risk-free, and transaction data.

**Tech Stack:** Go, ginkgo/gomega (testing), gonum (numerical operations), zerolog (logging)

---

### Task 1: Test suite setup and Account construction

**Files:**
- Create: `portfolio/portfolio_suite_test.go`
- Create: `portfolio/account_test.go`
- Modify: `portfolio/account.go`

**Step 1: Create test suite file**

```go
package portfolio_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPortfolio(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Portfolio Suite")
}
```

**Step 2: Write failing tests for Account construction**

In `portfolio/account_test.go`:

```go
package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ portfolio.Portfolio = (*portfolio.Account)(nil)
var _ portfolio.PortfolioManager = (*portfolio.Account)(nil)

var _ = Describe("Account", func() {
	var (
		spy      asset.Asset
		bil      asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
	})

	Describe("New", func() {
		It("creates an account with default zero cash", func() {
			a := portfolio.New()
			Expect(a.Cash()).To(Equal(0.0))
			Expect(a.Value()).To(Equal(0.0))
		})

		It("sets initial cash balance via WithCash", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			Expect(a.Cash()).To(Equal(10_000.0))
			Expect(a.Value()).To(Equal(10_000.0))
		})

		It("records a DepositTransaction for initial cash", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			txns := a.Transactions()
			Expect(txns).To(HaveLen(1))
			Expect(txns[0].Type).To(Equal(portfolio.DepositTransaction))
			Expect(txns[0].Amount).To(Equal(10_000.0))
		})

		It("stores benchmark and risk-free assets", func() {
			a := portfolio.New(
				portfolio.WithCash(10_000),
				portfolio.WithBenchmark(spy),
				portfolio.WithRiskFree(bil),
			)
			Expect(a.Benchmark()).To(Equal(spy))
			Expect(a.RiskFree()).To(Equal(bil))
		})
	})

	Describe("Holdings", func() {
		It("starts with no holdings", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			count := 0
			a.Holdings(func(_ asset.Asset, _ float64) { count++ })
			Expect(count).To(Equal(0))
		})

		It("returns zero for unknown positions", func() {
			a := portfolio.New(portfolio.WithCash(10_000))
			Expect(a.Position(spy)).To(Equal(0.0))
			Expect(a.PositionValue(spy)).To(Equal(0.0))
		})
	})
})
```

**Step 3: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL -- missing fields, methods, WithBenchmark, WithRiskFree, Benchmark(), RiskFree()

**Step 4: Implement Account struct fields and construction**

Update `portfolio/account.go` with the full Account struct, fields, and construction options. Add `Benchmark()` and `RiskFree()` accessor methods. Implement `Cash()`, `Value()`, `Transactions()`, `Position()`, `PositionValue()`, `Holdings()` to read from internal state. `WithCash` should record a `DepositTransaction`.

Account struct fields:
- `cash float64`
- `holdings map[asset.Asset]float64`
- `transactions []Transaction`
- `broker broker.Broker`
- `prices *data.DataFrame` -- latest prices for mark-to-market
- `equityCurve []float64` -- total portfolio value at each time step
- `equityTimes []time.Time` -- timestamps for equity curve
- `benchmarkPrices []float64` -- AdjClose series for benchmark
- `riskFreePrices []float64` -- AdjClose series for risk-free
- `benchmark asset.Asset`
- `riskFree asset.Asset`
- `taxLots map[asset.Asset][]taxLot`

`taxLot` is an unexported struct: `{ Date time.Time; Qty float64; Price float64 }`.

Initialize `holdings` and `taxLots` maps in `New`.

For `Value()`: return `cash` if no prices stored yet. When prices are available, sum `holdings[a] * df.Value(a, data.MetricClose)` for each held asset plus cash.

For `PositionValue(a)`: return `holdings[a] * df.Value(a, data.MetricClose)`, or 0 if no prices or no position.

**Step 5: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 6: Commit**

```bash
git add portfolio/portfolio_suite_test.go portfolio/account_test.go portfolio/account.go
git commit -m "feat(portfolio): implement Account struct with construction options and state accessors"
```

---

### Task 2: TransactionType.String() and Record

**Files:**
- Modify: `portfolio/transaction.go`
- Modify: `portfolio/account.go`
- Modify: `portfolio/account_test.go`

**Step 1: Write failing tests for TransactionType.String()**

Add to `account_test.go`:

```go
var _ = Describe("TransactionType", func() {
	It("returns correct string for each type", func() {
		Expect(portfolio.BuyTransaction.String()).To(Equal("Buy"))
		Expect(portfolio.SellTransaction.String()).To(Equal("Sell"))
		Expect(portfolio.DividendTransaction.String()).To(Equal("Dividend"))
		Expect(portfolio.FeeTransaction.String()).To(Equal("Fee"))
		Expect(portfolio.DepositTransaction.String()).To(Equal("Deposit"))
		Expect(portfolio.WithdrawalTransaction.String()).To(Equal("Withdrawal"))
	})
})
```

**Step 2: Write failing tests for Record**

Add to the Account Describe block in `account_test.go`:

```go
Describe("Record", func() {
	It("records a dividend and increases cash", func() {
		a := portfolio.New(portfolio.WithCash(10_000))
		a.Record(portfolio.Transaction{
			Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   portfolio.DividendTransaction,
			Amount: 50.0,
		})
		Expect(a.Cash()).To(Equal(10_050.0))
		Expect(a.Transactions()).To(HaveLen(2)) // deposit + dividend
	})

	It("records a fee and decreases cash", func() {
		a := portfolio.New(portfolio.WithCash(10_000))
		a.Record(portfolio.Transaction{
			Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Type:   portfolio.FeeTransaction,
			Amount: -25.0,
		})
		Expect(a.Cash()).To(Equal(9_975.0))
	})

	It("records a deposit and increases cash", func() {
		a := portfolio.New(portfolio.WithCash(10_000))
		a.Record(portfolio.Transaction{
			Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			Type:   portfolio.DepositTransaction,
			Amount: 5_000.0,
		})
		Expect(a.Cash()).To(Equal(15_000.0))
	})

	It("records a withdrawal and decreases cash", func() {
		a := portfolio.New(portfolio.WithCash(10_000))
		a.Record(portfolio.Transaction{
			Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			Type:   portfolio.WithdrawalTransaction,
			Amount: -3_000.0,
		})
		Expect(a.Cash()).To(Equal(7_000.0))
	})

	It("records a buy: decreases cash, increases holdings, creates tax lot", func() {
		a := portfolio.New(portfolio.WithCash(10_000))
		a.Record(portfolio.Transaction{
			Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    10,
			Price:  300.0,
			Amount: -3_000.0,
		})
		Expect(a.Cash()).To(Equal(7_000.0))
		Expect(a.Position(spy)).To(Equal(10.0))
	})

	It("records a sell: increases cash, decreases holdings, consumes tax lots FIFO", func() {
		a := portfolio.New(portfolio.WithCash(10_000))
		a.Record(portfolio.Transaction{
			Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    10,
			Price:  300.0,
			Amount: -3_000.0,
		})
		a.Record(portfolio.Transaction{
			Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   portfolio.SellTransaction,
			Qty:    5,
			Price:  320.0,
			Amount: 1_600.0,
		})
		Expect(a.Cash()).To(Equal(8_600.0))
		Expect(a.Position(spy)).To(Equal(5.0))
	})
})
```

**Step 3: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 4: Implement TransactionType.String()**

In `portfolio/transaction.go`, add:

```go
func (t TransactionType) String() string {
	switch t {
	case BuyTransaction:
		return "Buy"
	case SellTransaction:
		return "Sell"
	case DividendTransaction:
		return "Dividend"
	case FeeTransaction:
		return "Fee"
	case DepositTransaction:
		return "Deposit"
	case WithdrawalTransaction:
		return "Withdrawal"
	default:
		return fmt.Sprintf("TransactionType(%d)", int(t))
	}
}
```

**Step 5: Implement Record on Account**

In `portfolio/account.go`, implement `Record(tx Transaction)`:

1. Append tx to `a.transactions`
2. Update `a.cash += tx.Amount` (Amount is already signed: positive for inflows, negative for outflows)
3. For BuyTransaction: `a.holdings[tx.Asset] += tx.Qty`, append tax lot `{tx.Date, tx.Qty, tx.Price}`
4. For SellTransaction: `a.holdings[tx.Asset] -= tx.Qty`, consume tax lots FIFO (iterate oldest lots, decrement qty, remove fully consumed lots, split partially consumed lots)
5. If holdings drop to zero, delete the map entry

**Step 6: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 7: Commit**

```bash
git add portfolio/transaction.go portfolio/account.go portfolio/account_test.go
git commit -m "feat(portfolio): implement TransactionType.String() and Account.Record with FIFO tax lots"
```

---

### Task 3: UpdatePrices

**Files:**
- Modify: `portfolio/account.go`
- Modify: `portfolio/account_test.go`

**Step 1: Write failing tests for UpdatePrices**

Add to the Account Describe block:

```go
Describe("UpdatePrices", func() {
	It("updates equity curve with current portfolio value", func() {
		a := portfolio.New(
			portfolio.WithCash(10_000),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bil),
		)

		t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, bil},
			[]data.Metric{data.MetricClose, data.AdjClose},
			[]float64{
				// SPY Close, BIL Close
				300, 100,
				// SPY AdjClose, BIL AdjClose
				300, 100,
			},
		)
		Expect(err).NotTo(HaveOccurred())

		a.UpdatePrices(df)
		// no holdings, value should be just cash
		Expect(a.Value()).To(Equal(10_000.0))
		Expect(a.EquityCurve()).To(Equal([]float64{10_000.0}))
	})

	It("marks holdings to current prices", func() {
		a := portfolio.New(portfolio.WithCash(7_000))
		// manually add a position via Record
		a.Record(portfolio.Transaction{
			Date:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    10,
			Price:  300.0,
			Amount: -3_000.0,
		})

		t1 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{310},
		)
		Expect(err).NotTo(HaveOccurred())

		a.UpdatePrices(df)
		Expect(a.Value()).To(BeNumerically("~", 10_100.0, 0.01))
		Expect(a.PositionValue(spy)).To(BeNumerically("~", 3_100.0, 0.01))
	})

	It("accumulates benchmark and risk-free prices", func() {
		a := portfolio.New(
			portfolio.WithCash(10_000),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bil),
		)

		for i, price := range []float64{300, 303, 306} {
			t := time.Date(2024, 1, 2+i, 0, 0, 0, 0, time.UTC)
			df, err := data.NewDataFrame(
				[]time.Time{t},
				[]asset.Asset{spy, bil},
				[]data.Metric{data.MetricClose, data.AdjClose},
				[]float64{price, 100.0 + float64(i)*0.01, price, 100.0 + float64(i)*0.01},
			)
			Expect(err).NotTo(HaveOccurred())
			a.UpdatePrices(df)
		}

		Expect(a.BenchmarkPrices()).To(HaveLen(3))
		Expect(a.RiskFreePrices()).To(HaveLen(3))
		Expect(a.EquityCurve()).To(HaveLen(3))
	})
})
```

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL -- missing EquityCurve(), BenchmarkPrices(), RiskFreePrices() accessors, UpdatePrices not implemented

**Step 3: Implement UpdatePrices and accessor methods**

In `portfolio/account.go`:

1. `UpdatePrices(df *data.DataFrame)`:
   - Store `df` as `a.prices`
   - Compute total value: `cash + sum(holdings[asset] * df.Value(asset, data.MetricClose))`
   - Append total value to `a.equityCurve`
   - Append the DataFrame's latest time to `a.equityTimes`
   - If benchmark is set: append `df.Value(a.benchmark, data.AdjClose)` to `a.benchmarkPrices`
   - If riskFree is set: append `df.Value(a.riskFree, data.AdjClose)` to `a.riskFreePrices`

2. Add accessor methods:
   - `EquityCurve() []float64` -- returns a copy of the equity curve slice
   - `EquityTimes() []time.Time` -- returns a copy of the equity times slice
   - `BenchmarkPrices() []float64` -- returns a copy
   - `RiskFreePrices() []float64` -- returns a copy

3. Update `Value()` and `PositionValue()` to use `a.prices` when available.

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/account.go portfolio/account_test.go
git commit -m "feat(portfolio): implement UpdatePrices with equity curve, benchmark, and risk-free tracking"
```

---

### Task 4: Order execution via broker

**Files:**
- Modify: `portfolio/account.go`
- Create: `portfolio/order_test.go`

**Step 1: Write failing tests for Order**

Create `portfolio/order_test.go`. Use a mock broker that implements `broker.Broker` and records submitted orders, returning configurable fills.

```go
package portfolio_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// mockBroker records submitted orders and returns preconfigured fills.
type mockBroker struct {
	submitted []broker.Order
	fills     [][]broker.Fill // one []Fill per Submit call, consumed in order
	callIdx   int
}

func (m *mockBroker) Connect(_ context.Context) error                      { return nil }
func (m *mockBroker) Close() error                                         { return nil }
func (m *mockBroker) Cancel(_ string) error                                { return nil }
func (m *mockBroker) Replace(_ string, o broker.Order) ([]broker.Fill, error) { return nil, nil }
func (m *mockBroker) Orders() ([]broker.Order, error)                      { return nil, nil }
func (m *mockBroker) Positions() ([]broker.Position, error)                { return nil, nil }
func (m *mockBroker) Balance() (broker.Balance, error)                     { return broker.Balance{}, nil }

func (m *mockBroker) Submit(o broker.Order) ([]broker.Fill, error) {
	m.submitted = append(m.submitted, o)
	if m.callIdx < len(m.fills) {
		f := m.fills[m.callIdx]
		m.callIdx++
		return f, nil
	}
	// default: fill entirely at a fixed price
	return []broker.Fill{{
		OrderID:  "fill-1",
		Price:    100.0,
		Qty:      o.Qty,
		FilledAt: time.Now(),
	}}, nil
}

var _ = Describe("Order", func() {
	var (
		spy asset.Asset
		mb  *mockBroker
		a   *portfolio.Account
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		mb = &mockBroker{}
		a = portfolio.New(
			portfolio.WithCash(100_000),
			portfolio.WithBroker(mb),
		)
		// provide prices so Value() works
		t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		df, _ := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{300},
		)
		a.UpdatePrices(df)
	})

	It("places a market buy order via broker", func() {
		mb.fills = [][]broker.Fill{{
			{OrderID: "1", Price: 300.0, Qty: 10, FilledAt: time.Now()},
		}}
		a.Order(spy, portfolio.Buy, 10)
		Expect(mb.submitted).To(HaveLen(1))
		Expect(mb.submitted[0].Side).To(Equal(broker.Buy))
		Expect(mb.submitted[0].Qty).To(Equal(10.0))
		Expect(mb.submitted[0].OrderType).To(Equal(broker.Market))
		Expect(mb.submitted[0].TimeInForce).To(Equal(broker.Day))
		Expect(a.Cash()).To(Equal(97_000.0))
		Expect(a.Position(spy)).To(Equal(10.0))
	})

	It("places a limit order", func() {
		mb.fills = [][]broker.Fill{{
			{OrderID: "1", Price: 295.0, Qty: 10, FilledAt: time.Now()},
		}}
		a.Order(spy, portfolio.Buy, 10, portfolio.Limit(295.0))
		Expect(mb.submitted[0].OrderType).To(Equal(broker.Limit))
		Expect(mb.submitted[0].LimitPrice).To(Equal(295.0))
	})

	It("places a stop order", func() {
		// first buy some shares
		mb.fills = [][]broker.Fill{
			{{OrderID: "1", Price: 300.0, Qty: 10, FilledAt: time.Now()}},
			{{OrderID: "2", Price: 280.0, Qty: 10, FilledAt: time.Now()}},
		}
		a.Order(spy, portfolio.Buy, 10)
		a.Order(spy, portfolio.Sell, 10, portfolio.Stop(280.0))
		Expect(mb.submitted[1].OrderType).To(Equal(broker.Stop))
		Expect(mb.submitted[1].StopPrice).To(Equal(280.0))
	})

	It("places a stop-limit order", func() {
		mb.fills = [][]broker.Fill{
			{{OrderID: "1", Price: 300.0, Qty: 10, FilledAt: time.Now()}},
			{{OrderID: "2", Price: 275.0, Qty: 10, FilledAt: time.Now()}},
		}
		a.Order(spy, portfolio.Buy, 10)
		a.Order(spy, portfolio.Sell, 10, portfolio.Stop(280.0), portfolio.Limit(275.0))
		Expect(mb.submitted[1].OrderType).To(Equal(broker.StopLimit))
		Expect(mb.submitted[1].StopPrice).To(Equal(280.0))
		Expect(mb.submitted[1].LimitPrice).To(Equal(275.0))
	})

	It("handles time-in-force modifiers", func() {
		mb.fills = [][]broker.Fill{{
			{OrderID: "1", Price: 300.0, Qty: 10, FilledAt: time.Now()},
		}}
		a.Order(spy, portfolio.Buy, 10, portfolio.GoodTilCancel)
		Expect(mb.submitted[0].TimeInForce).To(Equal(broker.GTC))
	})

	It("handles multiple fills for a single order", func() {
		mb.fills = [][]broker.Fill{{
			{OrderID: "1", Price: 300.0, Qty: 6, FilledAt: time.Now()},
			{OrderID: "1", Price: 299.0, Qty: 4, FilledAt: time.Now()},
		}}
		a.Order(spy, portfolio.Buy, 10)
		Expect(a.Position(spy)).To(Equal(10.0))
		// cash: 100_000 - (6*300 + 4*299) = 100_000 - 2996 = 97_004
		Expect(a.Cash()).To(Equal(97_004.0))
		// should produce 2 BuyTransactions (one per fill)
		txns := a.Transactions()
		buyCount := 0
		for _, tx := range txns {
			if tx.Type == portfolio.BuyTransaction {
				buyCount++
			}
		}
		Expect(buyCount).To(Equal(2))
	})

	It("places a sell order", func() {
		mb.fills = [][]broker.Fill{
			{{OrderID: "1", Price: 300.0, Qty: 10, FilledAt: time.Now()}},
			{{OrderID: "2", Price: 320.0, Qty: 5, FilledAt: time.Now()}},
		}
		a.Order(spy, portfolio.Buy, 10)
		a.Order(spy, portfolio.Sell, 5)
		Expect(a.Position(spy)).To(Equal(5.0))
		Expect(a.Cash()).To(Equal(98_600.0)) // 100k - 3000 + 1600
	})
})
```

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement Order on Account**

In `portfolio/account.go`, implement `Order(a asset.Asset, side Side, qty float64, mods ...OrderModifier)`:

1. Build a `broker.Order` from the arguments:
   - Map `portfolio.Buy`/`portfolio.Sell` to `broker.Buy`/`broker.Sell`
   - Default: `OrderType = broker.Market`, `TimeInForce = broker.Day`
   - Iterate modifiers:
     - `limitModifier`: set `LimitPrice`, mark has-limit
     - `stopModifier`: set `StopPrice`, mark has-stop
     - If both has-limit and has-stop: `OrderType = broker.StopLimit`
     - Else if has-limit: `OrderType = broker.Limit`
     - Else if has-stop: `OrderType = broker.Stop`
     - `dayOrderModifier`: `TimeInForce = broker.Day`
     - `goodTilCancelModifier`: `TimeInForce = broker.GTC`
     - `fillOrKillModifier`: `TimeInForce = broker.FOK`
     - `immediateOrCancelModifier`: `TimeInForce = broker.IOC`
     - `onTheOpenModifier`: `TimeInForce = broker.OnOpen`
     - `onTheCloseModifier`: `TimeInForce = broker.OnClose`
     - `goodTilDateModifier`: `TimeInForce = broker.GTD`, set `GTDDate`
2. Call `a.broker.Submit(order)` to get `[]Fill`
3. For each fill, call `a.Record(Transaction{...})` with the appropriate type and amounts

Note: the modifier types are unexported but `Order` is on `*Account` in the same package, so it can type-assert on the modifier interface values. Use a type switch.

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/account.go portfolio/order_test.go
git commit -m "feat(portfolio): implement Order with modifier translation and broker submission"
```

---

### Task 5: RebalanceTo

**Files:**
- Modify: `portfolio/account.go`
- Create: `portfolio/rebalance_test.go`

**Step 1: Write failing tests for RebalanceTo**

Create `portfolio/rebalance_test.go`:

```go
package portfolio_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("RebalanceTo", func() {
	var (
		spy asset.Asset
		aapl asset.Asset
		mb  *mockBroker
		a   *portfolio.Account
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		mb = &mockBroker{}
		a = portfolio.New(
			portfolio.WithCash(100_000),
			portfolio.WithBroker(mb),
		)

		t1 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		df, _ := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{500, 200},
		)
		a.UpdatePrices(df)
	})

	It("buys to match a single allocation from cash", func() {
		// Target: 60% SPY, 40% AAPL
		// Value = 100_000
		// SPY: 60_000 / 500 = 120 shares
		// AAPL: 40_000 / 200 = 200 shares
		alloc := portfolio.Allocation{
			Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			Members: map[asset.Asset]float64{
				spy:  0.60,
				aapl: 0.40,
			},
		}

		// broker fills at the requested prices
		mb.fills = [][]broker.Fill{
			{{OrderID: "1", Price: 500.0, Qty: 120, FilledAt: time.Now()}},
			{{OrderID: "2", Price: 200.0, Qty: 200, FilledAt: time.Now()}},
		}

		a.RebalanceTo(alloc)
		Expect(a.Position(spy)).To(Equal(120.0))
		Expect(a.Position(aapl)).To(Equal(200.0))
		Expect(len(mb.submitted)).To(Equal(2))
	})

	It("sells excess and buys new to rebalance", func() {
		// Start with 200 shares of SPY
		mb.fills = [][]broker.Fill{
			{{OrderID: "1", Price: 500.0, Qty: 200, FilledAt: time.Now()}},
		}
		a.Order(spy, portfolio.Buy, 200) // cost 100_000, cash = 0

		// Now rebalance to 50/50
		// Total value = 200 * 500 = 100_000
		// Target SPY: 50_000 / 500 = 100 (sell 100)
		// Target AAPL: 50_000 / 200 = 250 (buy 250)
		alloc := portfolio.Allocation{
			Date: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
			Members: map[asset.Asset]float64{
				spy:  0.50,
				aapl: 0.50,
			},
		}

		// sells processed before buys
		mb.fills = append(mb.fills,
			[]broker.Fill{{OrderID: "s1", Price: 500.0, Qty: 100, FilledAt: time.Now()}},
			[]broker.Fill{{OrderID: "b1", Price: 200.0, Qty: 250, FilledAt: time.Now()}},
		)

		a.RebalanceTo(alloc)
		Expect(a.Position(spy)).To(Equal(100.0))
		Expect(a.Position(aapl)).To(Equal(250.0))
	})

	It("handles assets not in the target (sell everything)", func() {
		mb.fills = [][]broker.Fill{
			{{OrderID: "1", Price: 500.0, Qty: 100, FilledAt: time.Now()}},
		}
		a.Order(spy, portfolio.Buy, 100) // cost 50_000, cash = 50_000

		// Rebalance to 100% AAPL -- SPY should be sold
		alloc := portfolio.Allocation{
			Date: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
			Members: map[asset.Asset]float64{
				aapl: 1.0,
			},
		}

		mb.fills = append(mb.fills,
			[]broker.Fill{{OrderID: "s1", Price: 500.0, Qty: 100, FilledAt: time.Now()}},
			[]broker.Fill{{OrderID: "b1", Price: 200.0, Qty: 500, FilledAt: time.Now()}},
		)

		a.RebalanceTo(alloc)
		Expect(a.Position(spy)).To(Equal(0.0))
		Expect(a.Position(aapl)).To(Equal(500.0))
	})
})
```

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement RebalanceTo**

In `portfolio/account.go`, implement `RebalanceTo(allocs ...Allocation)`:

1. For each allocation (in date order -- they should already be sorted since PortfolioPlan is time-ordered):
   a. Compute total portfolio value using current prices
   b. For each currently held asset NOT in the target allocation: queue a sell for full position
   c. For each target asset: compute target shares = floor(weight * totalValue / currentPrice). Diff against current holdings. If diff > 0, queue a buy. If diff < 0, queue a sell.
   d. Process sells first (to free up cash), then buys
   e. For each queued order: build a `broker.Order` (market order, day TIF), submit via broker, record transactions from fills

Use `math.Floor` for share quantities to avoid fractional shares in the default case.

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/account.go portfolio/rebalance_test.go
git commit -m "feat(portfolio): implement RebalanceTo with sell-first rebalancing"
```

---

### Task 6: Selection -- MaxAboveZero

**Files:**
- Modify: `portfolio/max_above_zero.go`
- Create: `portfolio/selector_test.go`

**Step 1: Write failing tests**

Create `portfolio/selector_test.go`:

```go
package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("MaxAboveZero", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		bil  asset.Asset
		times []time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		times = make([]time.Time, 3)
		for i := range times {
			times[i] = base.AddDate(0, 0, i)
		}
	})

	It("selects the asset with the highest positive value at each timestep", func() {
		// SPY has higher signal than AAPL at all timesteps
		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				10, 20, 30, // SPY
				5, 15, 25,  // AAPL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(nil)
		result := sel.Select(df)

		// result should only have SPY (highest at every timestep)
		Expect(result.AssetList()).To(HaveLen(1))
		Expect(result.AssetList()[0]).To(Equal(spy))
	})

	It("falls back when no assets are above zero", func() {
		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				-10, -20, -30,
				-5, -15, -25,
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero([]asset.Asset{bil})
		result := sel.Select(df)

		// When all negative, should return a DataFrame containing the fallback assets
		Expect(result.AssetList()).To(ContainElement(bil))
	})

	It("switches selection when leadership changes", func() {
		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				10, 5, 30,  // SPY: high, low, high
				5, 15, 25,  // AAPL: low, high, low
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(nil)
		result := sel.Select(df)

		// result should contain both assets since leadership switches
		Expect(result.AssetList()).To(HaveLen(2))
	})
})
```

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement MaxAboveZero.Select**

In `portfolio/max_above_zero.go`:

The Select method needs to iterate each timestamp, find the asset with the highest value (across the first metric column) above zero, and build a result DataFrame containing only the winning assets. If no asset is above zero at a timestep and fallback assets are specified, use the fallback.

Implementation approach:
1. Get times, assets, metrics from input DataFrame
2. For each timestamp, find the max value and its asset using `df.At(t)` or by iterating columns
3. Build a set of selected assets (those that win at any timestep)
4. If fallback assets are needed (timesteps where nothing was above zero), include them in the asset set
5. Return `df.Assets(selectedAssets...)` to filter the DataFrame

Note: The returned DataFrame should contain all timesteps but only the selected assets. The doc says "the same structure but with unselected assets removed."

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/max_above_zero.go portfolio/selector_test.go
git commit -m "feat(portfolio): implement MaxAboveZero selector"
```

---

### Task 7: Weighting -- EqualWeight and WeightedBySignal

**Files:**
- Modify: `portfolio/equal_weight.go`
- Modify: `portfolio/weighted_by_signal.go`
- Create: `portfolio/weighting_test.go`

**Step 1: Write failing tests**

Create `portfolio/weighting_test.go`:

```go
package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Weighting", func() {
	var (
		spy   asset.Asset
		aapl  asset.Asset
		times []time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		times = make([]time.Time, 3)
		for i := range times {
			times[i] = base.AddDate(0, 0, i)
		}
	})

	Describe("EqualWeight", func() {
		It("assigns equal weights to all assets", func() {
			df, err := data.NewDataFrame(
				times,
				[]asset.Asset{spy, aapl},
				[]data.Metric{data.MetricClose},
				[]float64{100, 101, 102, 200, 202, 204},
			)
			Expect(err).NotTo(HaveOccurred())

			plan := portfolio.EqualWeight(df)
			Expect(plan).To(HaveLen(3))
			for _, alloc := range plan {
				Expect(alloc.Members).To(HaveLen(2))
				Expect(alloc.Members[spy]).To(BeNumerically("~", 0.5, 0.001))
				Expect(alloc.Members[aapl]).To(BeNumerically("~", 0.5, 0.001))
			}
		})

		It("handles a single asset", func() {
			df, err := data.NewDataFrame(
				times,
				[]asset.Asset{spy},
				[]data.Metric{data.MetricClose},
				[]float64{100, 101, 102},
			)
			Expect(err).NotTo(HaveOccurred())

			plan := portfolio.EqualWeight(df)
			Expect(plan).To(HaveLen(3))
			Expect(plan[0].Members[spy]).To(Equal(1.0))
		})

		It("sets the correct date on each allocation", func() {
			df, err := data.NewDataFrame(
				times,
				[]asset.Asset{spy},
				[]data.Metric{data.MetricClose},
				[]float64{100, 101, 102},
			)
			Expect(err).NotTo(HaveOccurred())

			plan := portfolio.EqualWeight(df)
			for i, alloc := range plan {
				Expect(alloc.Date).To(Equal(times[i]))
			}
		})
	})

	Describe("WeightedBySignal", func() {
		It("weights proportionally to the named metric", func() {
			// SPY has MarketCap 300, AAPL has MarketCap 100
			// SPY weight = 300/400 = 0.75, AAPL weight = 100/400 = 0.25
			df, err := data.NewDataFrame(
				times[:1], // single timestep
				[]asset.Asset{spy, aapl},
				[]data.Metric{data.MetricClose, data.MarketCap},
				[]float64{
					100, 200, // Close
					300, 100, // MarketCap
				},
			)
			Expect(err).NotTo(HaveOccurred())

			plan := portfolio.WeightedBySignal(df, data.MarketCap)
			Expect(plan).To(HaveLen(1))
			Expect(plan[0].Members[spy]).To(BeNumerically("~", 0.75, 0.001))
			Expect(plan[0].Members[aapl]).To(BeNumerically("~", 0.25, 0.001))
		})

		It("normalizes weights to sum to 1.0", func() {
			df, err := data.NewDataFrame(
				times[:1],
				[]asset.Asset{spy, aapl},
				[]data.Metric{data.MetricClose, data.MarketCap},
				[]float64{
					100, 200,
					500, 500,
				},
			)
			Expect(err).NotTo(HaveOccurred())

			plan := portfolio.WeightedBySignal(df, data.MarketCap)
			total := 0.0
			for _, w := range plan[0].Members {
				total += w
			}
			Expect(total).To(BeNumerically("~", 1.0, 0.001))
		})
	})
})
```

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement EqualWeight**

In `portfolio/equal_weight.go`:

1. Get times and assets from the DataFrame
2. For each timestamp: create an Allocation with `Date = t` and `Members` mapping each asset to `1.0 / float64(len(assets))`
3. Return the PortfolioPlan

**Step 4: Implement WeightedBySignal**

In `portfolio/weighted_by_signal.go`:

1. Get times and assets from the DataFrame
2. For each timestamp:
   a. Read the metric value for each asset: `df.ValueAt(asset, metric, t)`
   b. Sum all values
   c. Normalize: weight = value / sum
   d. Create an Allocation with the normalized weights
3. Return the PortfolioPlan

Handle edge case: if sum is zero or negative values exist, fall back to equal weight for that timestep.

**Step 5: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 6: Commit**

```bash
git add portfolio/equal_weight.go portfolio/weighted_by_signal.go portfolio/weighting_test.go
git commit -m "feat(portfolio): implement EqualWeight and WeightedBySignal"
```

---

### Task 8: Metric helpers -- return series derivation

**Files:**
- Create: `portfolio/metric_helpers.go`
- Create: `portfolio/metric_helpers_test.go`

**Step 1: Write failing tests for helper functions**

Create `portfolio/metric_helpers_test.go`:

```go
package portfolio

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("metric helpers", func() {
	Describe("returns", func() {
		It("computes period-over-period returns", func() {
			prices := []float64{100, 110, 105, 115}
			r := returns(prices)
			Expect(r).To(HaveLen(3))
			Expect(r[0]).To(BeNumerically("~", 0.10, 0.001))
			Expect(r[1]).To(BeNumerically("~", -0.04545, 0.001))
			Expect(r[2]).To(BeNumerically("~", 0.09524, 0.001))
		})

		It("returns empty for single-element input", func() {
			Expect(returns([]float64{100})).To(BeEmpty())
		})
	})

	Describe("excessReturns", func() {
		It("subtracts risk-free returns element-wise", func() {
			r := []float64{0.10, 0.05, 0.08}
			rf := []float64{0.01, 0.01, 0.01}
			er := excessReturns(r, rf)
			Expect(er[0]).To(BeNumerically("~", 0.09, 0.001))
			Expect(er[1]).To(BeNumerically("~", 0.04, 0.001))
			Expect(er[2]).To(BeNumerically("~", 0.07, 0.001))
		})
	})

	Describe("windowSlice", func() {
		It("trims series to a trailing window", func() {
			times := []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
			}
			series := []float64{10, 20, 30, 40}
			w := Months(2)
			result := windowSlice(series, times, &w)
			// last 2 months: March and April
			Expect(result).To(HaveLen(2))
			Expect(result).To(Equal([]float64{30, 40}))
		})

		It("returns full series when window is nil", func() {
			times := []time.Time{
				time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			}
			series := []float64{10, 20}
			result := windowSlice(series, times, nil)
			Expect(result).To(Equal([]float64{10, 20}))
		})
	})

	Describe("mean", func() {
		It("computes arithmetic mean", func() {
			Expect(mean([]float64{1, 2, 3, 4})).To(Equal(2.5))
		})
	})

	Describe("stddev", func() {
		It("computes sample standard deviation", func() {
			// known: stddev of {2, 4, 4, 4, 5, 5, 7, 9} = 2.0
			s := stddev([]float64{2, 4, 4, 4, 5, 5, 7, 9})
			Expect(s).To(BeNumerically("~", 2.138, 0.01))
		})
	})

	Describe("cagr", func() {
		It("computes compound annual growth rate", func() {
			// 100 -> 200 over 3 years = 2^(1/3) - 1 ~ 0.2599
			r := cagr(100, 200, 3.0)
			Expect(r).To(BeNumerically("~", 0.2599, 0.001))
		})
	})

	Describe("drawdownSeries", func() {
		It("computes drawdowns from equity curve", func() {
			equity := []float64{100, 110, 105, 115, 100}
			dd := drawdownSeries(equity)
			Expect(dd[0]).To(Equal(0.0))
			Expect(dd[1]).To(Equal(0.0))
			Expect(dd[2]).To(BeNumerically("~", -0.04545, 0.001)) // (105-110)/110
			Expect(dd[3]).To(Equal(0.0))
			Expect(dd[4]).To(BeNumerically("~", -0.13043, 0.001)) // (100-115)/115
		})
	})

	Describe("covariance", func() {
		It("computes sample covariance", func() {
			x := []float64{1, 2, 3, 4, 5}
			y := []float64{2, 4, 6, 8, 10}
			c := covariance(x, y)
			Expect(c).To(BeNumerically("~", 5.0, 0.01))
		})
	})

	Describe("variance", func() {
		It("computes sample variance", func() {
			x := []float64{1, 2, 3, 4, 5}
			v := variance(x)
			Expect(v).To(BeNumerically("~", 2.5, 0.01))
		})
	})
})
```

Note: This test file uses `package portfolio` (not `portfolio_test`) to test unexported helpers.

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement helper functions**

Create `portfolio/metric_helpers.go`:

```go
package portfolio

import (
	"math"
	"time"
)

// returns computes period-over-period returns from a price series.
func returns(prices []float64) []float64 {
	if len(prices) < 2 {
		return nil
	}
	r := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		r[i-1] = (prices[i] - prices[i-1]) / prices[i-1]
	}
	return r
}

// excessReturns subtracts risk-free returns from portfolio returns.
func excessReturns(r, rf []float64) []float64 {
	n := len(r)
	if len(rf) < n {
		n = len(rf)
	}
	er := make([]float64, n)
	for i := 0; i < n; i++ {
		er[i] = r[i] - rf[i]
	}
	return er
}

// windowSlice trims a series to the trailing window based on timestamps.
// If window is nil, the full series is returned.
func windowSlice(series []float64, times []time.Time, window *Period) []float64 {
	if window == nil || len(times) == 0 {
		return series
	}
	last := times[len(times)-1]
	var cutoff time.Time
	switch window.Unit {
	case UnitDay:
		cutoff = last.AddDate(0, 0, -window.N)
	case UnitMonth:
		cutoff = last.AddDate(0, -window.N, 0)
	case UnitYear:
		cutoff = last.AddDate(-window.N, 0, 0)
	}
	for i, t := range times {
		if !t.Before(cutoff) {
			return series[i:]
		}
	}
	return nil
}

// mean computes the arithmetic mean.
func mean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

// stddev computes the sample standard deviation.
func stddev(x []float64) float64 {
	return math.Sqrt(variance(x))
}

// variance computes the sample variance.
func variance(x []float64) float64 {
	if len(x) < 2 {
		return 0
	}
	m := mean(x)
	s := 0.0
	for _, v := range x {
		d := v - m
		s += d * d
	}
	return s / float64(len(x)-1)
}

// covariance computes the sample covariance between x and y.
func covariance(x, y []float64) float64 {
	n := len(x)
	if n < 2 || len(y) < n {
		return 0
	}
	mx := mean(x)
	my := mean(y)
	s := 0.0
	for i := 0; i < n; i++ {
		s += (x[i] - mx) * (y[i] - my)
	}
	return s / float64(n-1)
}

// cagr computes the compound annual growth rate.
func cagr(startValue, endValue, years float64) float64 {
	if startValue <= 0 || years <= 0 {
		return 0
	}
	return math.Pow(endValue/startValue, 1.0/years) - 1
}

// drawdownSeries computes the drawdown at each point in the equity curve.
// Values are negative (or zero at peaks).
func drawdownSeries(equity []float64) []float64 {
	dd := make([]float64, len(equity))
	peak := 0.0
	for i, v := range equity {
		if v > peak {
			peak = v
		}
		if peak > 0 {
			dd[i] = (v - peak) / peak
		}
	}
	return dd
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/metric_helpers.go portfolio/metric_helpers_test.go
git commit -m "feat(portfolio): add metric helper functions (returns, stddev, covariance, drawdowns, etc.)"
```

---

### Task 9: Return metrics -- TWRR, MWRR, ActiveReturn

**Files:**
- Modify: `portfolio/twrr.go`
- Modify: `portfolio/mwrr.go`
- Modify: `portfolio/active_return.go`
- Create: `portfolio/return_metrics_test.go`

**Step 1: Write failing tests**

Create `portfolio/return_metrics_test.go` (uses `package portfolio` to access unexported helpers and Account internals via the exported accessors):

```go
package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Return Metrics", func() {
	var (
		spy asset.Asset
		bil asset.Asset
		a   *portfolio.Account
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
		a = portfolio.New(
			portfolio.WithCash(10_000),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bil),
		)

		// Simulate an equity curve: 10000, 10100, 10300, 10200, 10500
		// Benchmark: 300, 303, 309, 306, 315
		// Risk-free: 100, 100.01, 100.02, 100.03, 100.04
		equityValues := []float64{10_000, 10_100, 10_300, 10_200, 10_500}
		benchmarkValues := []float64{300, 303, 309, 306, 315}
		riskFreeValues := []float64{100, 100.01, 100.02, 100.03, 100.04}

		for i := 0; i < 5; i++ {
			t := time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC)
			df, _ := data.NewDataFrame(
				[]time.Time{t},
				[]asset.Asset{spy, bil},
				[]data.Metric{data.MetricClose, data.AdjClose},
				[]float64{
					benchmarkValues[i], riskFreeValues[i],
					benchmarkValues[i], riskFreeValues[i],
				},
			)
			a.UpdatePrices(df)
		}
		// Override the equity curve for testing (we need a way to do this,
		// or we can test via the full Account flow with mock broker + trades).
		// For now, test that the metric returns a non-zero value given
		// a realistic equity curve built through UpdatePrices.
	})

	Describe("TWRR", func() {
		It("returns a non-zero value for a portfolio with history", func() {
			val := a.PerformanceMetric(portfolio.TWRR).Value()
			// Equity went from 10000 to 10500, TWRR should be positive
			Expect(val).To(BeNumerically(">", 0))
		})

		It("returns a series of correct length", func() {
			series := a.PerformanceMetric(portfolio.TWRR).Series()
			Expect(series).To(HaveLen(4)) // n-1 returns for n equity points
		})
	})

	Describe("MWRR", func() {
		It("returns a value for a portfolio with no external cash flows", func() {
			val := a.PerformanceMetric(portfolio.MWRR).Value()
			// With no cash flows, MWRR should approximate TWRR
			Expect(val).To(BeNumerically(">", 0))
		})
	})

	Describe("ActiveReturn", func() {
		It("computes the difference between portfolio and benchmark return", func() {
			val := a.PerformanceMetric(portfolio.ActiveReturn).Value()
			// Both have positive returns, active return could be positive or negative
			Expect(val).To(BeNumerically("~", 0, 0.5)) // within a reasonable range
		})
	})
})
```

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL (metrics return 0)

**Step 3: Implement TWRR**

In `portfolio/twrr.go`:

- `Compute`: Get equity curve from Account, apply window if specified, compute sub-period returns, compound them: `product(1 + r_i) - 1`
- `ComputeSeries`: Return the cumulative return at each point

The Account needs to expose its equity curve and times for metrics. Since metrics receive `*Account`, they can call the exported accessors `EquityCurve()`, `EquityTimes()`, `BenchmarkPrices()`, `RiskFreePrices()`.

**Step 4: Implement MWRR**

In `portfolio/mwrr.go`:

- `Compute`: Use XIRR (extended internal rate of return) on the cash flow series. Cash flows are deposits (negative, outflow from investor perspective) and withdrawals (positive), plus the ending portfolio value as a final positive cash flow. Use Newton's method to solve for the rate.
- `ComputeSeries`: Return nil for now (MWRR is typically a single scalar).

XIRR implementation: iterate with Newton-Raphson to find rate `r` such that `sum(cf_i / (1+r)^(t_i/365)) = 0`.

**Step 5: Implement ActiveReturn**

In `portfolio/active_return.go`:

- `Compute`: Portfolio total return minus benchmark total return. Total return = `(end/start) - 1` for both equity curve and benchmark prices.
- `ComputeSeries`: Element-wise difference of return series.

**Step 6: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 7: Commit**

```bash
git add portfolio/twrr.go portfolio/mwrr.go portfolio/active_return.go portfolio/return_metrics_test.go
git commit -m "feat(portfolio): implement TWRR, MWRR, and ActiveReturn metrics"
```

---

### Task 10: Risk-adjusted ratios -- Sharpe, Sortino, Calmar, StdDev

**Files:**
- Modify: `portfolio/sharpe.go`
- Modify: `portfolio/sortino.go`
- Modify: `portfolio/calmar.go`
- Modify: `portfolio/std_dev.go`
- Modify: `portfolio/max_drawdown.go`
- Modify: `portfolio/downside_deviation.go`
- Create: `portfolio/risk_adjusted_metrics_test.go`

**Step 1: Write failing tests**

Create `portfolio/risk_adjusted_metrics_test.go`. Build an Account with a known equity curve (via UpdatePrices calls in a loop) and verify each metric returns a value in the expected range. Use well-known test data where possible.

Test cases:
- Sharpe: for a steadily rising equity curve with low volatility, should be positive and > 1
- Sortino: should be >= Sharpe (since it only penalizes downside)
- Calmar: CAGR / MaxDrawdown, verify against manual calculation
- StdDev: verify against known standard deviation of the return series
- MaxDrawdown: verify largest peak-to-trough decline
- DownsideDeviation: verify it only considers negative excess returns

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement metrics**

- **StdDev**: `stddev(returns(equityCurve)) * sqrt(12)` (monthly annualization). If data is daily, use `sqrt(252)`. Determine frequency from equity times.
- **MaxDrawdown**: `min(drawdownSeries(equityCurve))`
- **DownsideDeviation**: `stddev` of only the negative excess returns, annualized
- **Sharpe**: `mean(excessReturns) / stddev(excessReturns) * sqrt(annualizationFactor)`
- **Sortino**: `mean(excessReturns) / downsideDeviation * sqrt(annualizationFactor)`
- **Calmar**: `cagr(start, end, years) / abs(maxDrawdown)`

For `ComputeSeries`: compute the metric over a rolling window of the same size as the total history, shifted by one period at each step.

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/sharpe.go portfolio/sortino.go portfolio/calmar.go portfolio/std_dev.go portfolio/max_drawdown.go portfolio/downside_deviation.go portfolio/risk_adjusted_metrics_test.go
git commit -m "feat(portfolio): implement Sharpe, Sortino, Calmar, StdDev, MaxDrawdown, DownsideDeviation"
```

---

### Task 11: Benchmark-dependent metrics -- Beta, Alpha, TrackingError, InformationRatio, Treynor, RSquared

**Files:**
- Modify: `portfolio/beta.go`
- Modify: `portfolio/alpha.go`
- Modify: `portfolio/tracking_error.go`
- Modify: `portfolio/information_ratio.go`
- Modify: `portfolio/treynor.go`
- Modify: `portfolio/r_squared.go`
- Create: `portfolio/benchmark_metrics_test.go`

**Step 1: Write failing tests**

Create `portfolio/benchmark_metrics_test.go`. Build an Account with known equity curve and benchmark series. Verify:
- Beta: for a portfolio perfectly correlated with benchmark, beta ~ 1.0
- Alpha: for a portfolio matching benchmark, alpha ~ 0.0
- TrackingError: for identical returns, tracking error ~ 0.0
- InformationRatio: active return / tracking error
- Treynor: excess return / beta
- RSquared: for perfectly correlated series, R^2 ~ 1.0

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement metrics**

- **Beta**: `covariance(portfolioReturns, benchmarkReturns) / variance(benchmarkReturns)`
- **Alpha**: `portfolioReturn - (riskFreeReturn + beta * (benchmarkReturn - riskFreeReturn))`
- **TrackingError**: `stddev(activeReturns)` where activeReturns = portfolioReturns - benchmarkReturns
- **InformationRatio**: `mean(activeReturns) / trackingError * sqrt(annualizationFactor)`
- **Treynor**: `(portfolioReturn - riskFreeReturn) / beta`
- **RSquared**: `correlation(portfolioReturns, benchmarkReturns)^2` where correlation = covariance / (stddev_p * stddev_b)

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/beta.go portfolio/alpha.go portfolio/tracking_error.go portfolio/information_ratio.go portfolio/treynor.go portfolio/r_squared.go portfolio/benchmark_metrics_test.go
git commit -m "feat(portfolio): implement Beta, Alpha, TrackingError, InformationRatio, Treynor, RSquared"
```

---

### Task 12: Capture ratios and drawdown metrics

**Files:**
- Modify: `portfolio/upside_capture.go`
- Modify: `portfolio/downside_capture.go`
- Modify: `portfolio/avg_drawdown.go`
- Create: `portfolio/capture_drawdown_metrics_test.go`

**Step 1: Write failing tests**

Test cases:
- UpsideCaptureRatio: when benchmark is up, portfolio return / benchmark return * 100
- DownsideCaptureRatio: when benchmark is down, portfolio return / benchmark return * 100
- AvgDrawdown: mean of all drawdown magnitudes

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement metrics**

- **UpsideCaptureRatio**: Filter periods where benchmark return > 0. Compute geometric mean of portfolio returns in those periods / geometric mean of benchmark returns in those periods.
- **DownsideCaptureRatio**: Same but for benchmark return < 0.
- **AvgDrawdown**: Identify all drawdown episodes in the equity curve (sequences of negative drawdown values between peaks). Average the trough values.

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/upside_capture.go portfolio/downside_capture.go portfolio/avg_drawdown.go portfolio/capture_drawdown_metrics_test.go
git commit -m "feat(portfolio): implement UpsideCaptureRatio, DownsideCaptureRatio, AvgDrawdown"
```

---

### Task 13: Distribution metrics -- ExcessKurtosis, Skewness, NPositivePeriods, GainLossRatio

**Files:**
- Modify: `portfolio/excess_kurtosis.go`
- Modify: `portfolio/skewness.go`
- Modify: `portfolio/n_positive_periods.go`
- Modify: `portfolio/gain_loss_ratio.go`
- Create: `portfolio/distribution_metrics_test.go`

**Step 1: Write failing tests**

Test cases:
- ExcessKurtosis: for normally distributed returns, should be near 0
- Skewness: for symmetric returns, should be near 0
- NPositivePeriods: count positive return periods / total periods
- GainLossRatio: mean positive return / abs(mean negative return)

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement metrics**

- **ExcessKurtosis**: fourth standardized central moment minus 3. `mean((x - mean)^4) / stddev^4 - 3`
- **Skewness**: third standardized central moment. `mean((x - mean)^3) / stddev^3`
- **NPositivePeriods**: count returns > 0, divide by total returns
- **GainLossRatio**: mean of positive returns / abs(mean of negative returns)

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/excess_kurtosis.go portfolio/skewness.go portfolio/n_positive_periods.go portfolio/gain_loss_ratio.go portfolio/distribution_metrics_test.go
git commit -m "feat(portfolio): implement ExcessKurtosis, Skewness, NPositivePeriods, GainLossRatio"
```

---

### Task 14: Specialized risk metrics -- UlcerIndex, ValueAtRisk, KRatio, KellerRatio

**Files:**
- Modify: `portfolio/ulcer_index.go`
- Modify: `portfolio/value_at_risk.go`
- Modify: `portfolio/k_ratio.go`
- Modify: `portfolio/keller_ratio.go`
- Create: `portfolio/specialized_metrics_test.go`

**Step 1: Write failing tests**

Test cases:
- UlcerIndex: sqrt(mean of squared percentage drawdowns) over 14-day lookback
- ValueAtRisk: 5th percentile of return distribution (95% confidence)
- KRatio: slope of log-VAMI regression / (N * stderr of slope)
- KellerRatio: R * (1 - D/(1-D)) when R >= 0 and D <= 0.5

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement metrics**

- **UlcerIndex**: Compute percentage drawdowns over a 14-day window, square them, take the mean, take the square root.
- **ValueAtRisk**: Sort returns, pick the value at the `(1-confidence) * N`th index. Default 95% confidence.
- **KRatio**: Compute log(VAMI) where VAMI = 1000 * product(1 + r_i). Fit a linear regression to log(VAMI) vs time index. K = slope / (N * stderr_of_slope). Use gonum's `stat.LinearRegression` or manual OLS.
- **KellerRatio**: `R = total return`, `D = max drawdown (as positive number)`. If R >= 0 and D <= 0.5: `K = R * (1 - D/(1-D))`. Else: `K = 0`.

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/ulcer_index.go portfolio/value_at_risk.go portfolio/k_ratio.go portfolio/keller_ratio.go portfolio/specialized_metrics_test.go
git commit -m "feat(portfolio): implement UlcerIndex, ValueAtRisk, KRatio, KellerRatio"
```

---

### Task 15: Trade metrics

**Files:**
- Modify: `portfolio/trade_metrics.go` (the bundle struct already exists)
- Modify: `portfolio/account.go` (the `TradeMetrics()` method)
- Create: `portfolio/trade_metrics_test.go`

**Step 1: Write failing tests**

Create `portfolio/trade_metrics_test.go`. Build an Account with a series of buy/sell transactions (via Record), then verify:

- WinRate: 2 winning trades out of 3 = 66.7%
- AverageWin: mean profit on winning trades
- AverageLoss: mean loss on losing trades
- ProfitFactor: gross wins / gross losses
- AverageHoldingPeriod: mean days between buy and matching sell
- Turnover: total value traded / average portfolio value

Test setup: Record several buy/sell pairs with known profits and holding periods.

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement TradeMetrics**

Update `Account.TradeMetrics()` to compute metrics from the transaction log:

1. Pair buy and sell transactions for each asset to form "round-trip trades"
2. For each round-trip: compute P/L = sell amount - buy amount, holding period = sell date - buy date
3. Classify as win (P/L > 0) or loss (P/L <= 0)
4. Compute aggregates: WinRate, AverageWin, AverageLoss, ProfitFactor, AverageHoldingPeriod
5. Turnover: sum of sell values over the period / average portfolio value, annualized

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/trade_metrics.go portfolio/account.go portfolio/trade_metrics_test.go
git commit -m "feat(portfolio): implement TradeMetrics from transaction log round-trip analysis"
```

---

### Task 16: Tax metrics

**Files:**
- Modify: `portfolio/tax_metrics.go` (the bundle struct already exists)
- Modify: `portfolio/account.go` (the `TaxMetrics()` method)
- Create: `portfolio/tax_metrics_test.go`

**Step 1: Write failing tests**

Create `portfolio/tax_metrics_test.go`. Build an Account with buy/sell/dividend transactions where holding periods cross the 1-year boundary:

- Buy 100 shares at $100 on Jan 1, 2023
- Sell 50 shares at $120 on Jun 1, 2023 (STCG: held < 1 year)
- Sell 50 shares at $130 on Feb 1, 2024 (LTCG: held > 1 year)
- Record a dividend of $200 (qualified)

Verify:
- STCG = 50 * (120 - 100) = $1,000
- LTCG = 50 * (130 - 100) = $1,500
- QualifiedDividends = $200
- TaxCostRatio: estimated taxes / total return

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement TaxMetrics**

Update `Account.TaxMetrics()` to compute from tax lots and transaction log:

1. For each sell transaction, match against consumed tax lots (already tracked by Record's FIFO logic). Compare sell date - lot date: > 365 days = LTCG, else STCG.
2. For unrealized gains: iterate current tax lots, compare current price to lot price, classify by holding period.
3. QualifiedDividends: sum dividend transaction amounts. (For simplicity in v1, treat all dividends as qualified. A more sophisticated approach would require dividend type data.)
4. TaxCostRatio: estimated tax liability / portfolio return. Use simplified tax rates (e.g., 15% LTCG, ordinary income rate for STCG).

Note: The Account needs to store the gain/loss information when lots are consumed during sells. Add a `realizedGains` field or compute lazily from the transaction + lot history.

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/tax_metrics.go portfolio/account.go portfolio/tax_metrics_test.go
git commit -m "feat(portfolio): implement TaxMetrics with FIFO lot-based gain classification"
```

---

### Task 17: Withdrawal metrics

**Files:**
- Modify: `portfolio/safe_withdrawal_rate.go`
- Modify: `portfolio/perpetual_withdrawal_rate.go`
- Modify: `portfolio/dynamic_withdrawal_rate.go`
- Create: `portfolio/withdrawal_metrics_test.go`

**Step 1: Write failing tests**

Create `portfolio/withdrawal_metrics_test.go`. Build an Account with a long equity curve (simulating several years of returns). Verify:

- SafeWithdrawalRate: returns a value between 0 and 1 (a percentage)
- PerpetualWithdrawalRate: returns a value <= SafeWithdrawalRate
- DynamicWithdrawalRate: returns a value >= SafeWithdrawalRate (dynamic adjustments allow higher rates)

Use a stable equity curve (steady growth) where the safe rate should be relatively high.

**Step 2: Run tests to verify they fail**

Run: `go test ./portfolio/... -v -count=1`
Expected: FAIL

**Step 3: Implement withdrawal metrics**

All three use circular bootstrap Monte Carlo simulation:

1. Compute monthly return series from equity curve
2. For each candidate withdrawal rate (binary search between 0% and 20%):
   a. Run N simulations (e.g., 1000):
      - Sample returns with replacement (circular block bootstrap to preserve autocorrelation)
      - Simulate 30-year withdrawal path: start with $1M, withdraw rate% annually, apply sampled returns
   b. Check success criterion:
      - SafeWithdrawalRate: balance never reaches zero in >= 95% of simulations
      - PerpetualWithdrawalRate: ending balance >= inflation-adjusted starting balance in >= 95%
      - DynamicWithdrawalRate: same as safe, but each year's withdrawal = min(inflation-adjusted initial, currentBalance * rate)
3. Return the highest rate that meets the criterion

Use `math/rand` with a fixed seed for reproducibility in tests.

**Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add portfolio/safe_withdrawal_rate.go portfolio/perpetual_withdrawal_rate.go portfolio/dynamic_withdrawal_rate.go portfolio/withdrawal_metrics_test.go
git commit -m "feat(portfolio): implement withdrawal rate metrics with Monte Carlo simulation"
```

---

### Task 18: Final build, vet, and test

**Step 1: Run full build**

Run: `go build ./...`
Expected: clean build

**Step 2: Run all tests**

Run: `go test ./portfolio/... -v -count=1`
Expected: all tests pass

**Step 3: Run go vet**

Run: `go vet ./...`
Expected: no issues

**Step 4: Run existing tests to ensure no regressions**

Run: `go test ./... -count=1`
Expected: all tests pass

**Step 5: Commit any final cleanup**

```bash
git add -A
git commit -m "chore(portfolio): final cleanup and vet fixes"
```
