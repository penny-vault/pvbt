# Fill Models Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add composable fill models to the SimulatedBroker so strategy authors can choose realistic fill behavior (VWAP, spread, market impact, slippage) instead of always filling at close price.

**Architecture:** A new `fill` package defines `BaseModel` and `Adjuster` interfaces. A `Pipeline` composes one base model with zero or more adjusters. The `SimulatedBroker` delegates to the pipeline in `Submit`. Fill models only apply to market orders; bracket/stop/limit orders keep their existing `EvaluatePending` logic.

**Tech Stack:** Go, Ginkgo/Gomega for tests

**Spec:** `docs/superpowers/specs/2026-03-22-fill-models-design.md`

---

## Metric Constant Names

The `data` package uses inconsistent naming. Reference these exact constants:
- `data.MetricOpen`, `data.MetricHigh`, `data.MetricLow`, `data.MetricClose` (prefixed)
- `data.Volume`, `data.Dividend`, `data.SplitFactor` (no prefix)
- `data.Bid`, `data.Ask`, `data.Price` (no prefix)

## File Structure

```
fill/                          (new package)
    fill.go                    # BaseModel, Adjuster, FillResult, DataFetcher, DataFetcherAware, Pipeline
    fill_suite_test.go         # Ginkgo suite wiring
    helpers_test.go            # buildBar test helper used across all test files
    close.go                   # CloseFill base model
    close_test.go              # Tests for CloseFill
    vwap.go                    # VWAPFill base model
    vwap_test.go               # Tests for VWAPFill
    spread.go                  # SpreadAware adjuster
    spread_test.go             # Tests for SpreadAware
    impact.go                  # MarketImpact adjuster with presets
    impact_test.go             # Tests for MarketImpact
    slippage.go                # Slippage adjuster
    slippage_test.go           # Tests for Slippage
    pipeline_test.go           # Integration tests for Pipeline composition

engine/
    simulated_broker.go        # Modified: add fillPipeline field, delegate in Submit
    option.go                  # Modified: add WithFillModel engine option
    simulated_broker_test.go   # Modified: add tests for fill model integration and partial fills
```

---

### Task 1: Core Types and Pipeline

**Files:**
- Create: `fill/fill.go`
- Create: `fill/fill_suite_test.go`
- Create: `fill/pipeline_test.go`

- [ ] **Step 1: Create the Ginkgo test suite file and test helpers**

Create `fill/fill_suite_test.go`:

```go
package fill_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
)

func TestFill(t *testing.T) {
	RegisterFailHandler(Fail)
	log.Logger = log.Output(GinkgoWriter)
	RunSpecs(t, "Fill Suite")
}
```

Create `fill/helpers_test.go` with the `buildBar` helper used by all test files:

```go
package fill_test

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// buildBar creates a single-row DataFrame for one asset with the given metrics.
// Panics on construction error (safe in tests).
func buildBar(date time.Time, aa asset.Asset, metrics map[data.Metric]float64) *data.DataFrame {
	times := []time.Time{date}
	assets := []asset.Asset{aa}

	metricNames := make([]data.Metric, 0, len(metrics))
	columns := make([][]float64, 0, len(metrics))

	for metric, val := range metrics {
		metricNames = append(metricNames, metric)
		columns = append(columns, []float64{val})
	}

	df, err := data.NewDataFrame(times, assets, metricNames, data.Daily, columns)
	if err != nil {
		panic(err)
	}

	return df
}
```

- [ ] **Step 2: Write pipeline tests**

Create `fill/pipeline_test.go`. Test that the pipeline calls the base model, then each adjuster in order, and propagates errors:

```go
package fill_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/fill"
)

// stubBase always returns a fixed FillResult.
type stubBase struct {
	price float64
}

func (sb *stubBase) Fill(_ context.Context, order broker.Order, _ *data.DataFrame) (fill.FillResult, error) {
	return fill.FillResult{Price: sb.price, Quantity: order.Qty}, nil
}

// errBase always returns an error.
type errBase struct{}

func (eb *errBase) Fill(_ context.Context, _ broker.Order, _ *data.DataFrame) (fill.FillResult, error) {
	return fill.FillResult{}, fmt.Errorf("base model error")
}

// addAdjuster adds a fixed amount to the price.
type addAdjuster struct {
	amount float64
}

func (aa *addAdjuster) Adjust(_ context.Context, _ broker.Order, _ *data.DataFrame, current fill.FillResult) (fill.FillResult, error) {
	current.Price += aa.amount
	return current, nil
}

// errAdjuster always returns an error.
type errAdjuster struct{}

func (ea *errAdjuster) Adjust(_ context.Context, _ broker.Order, _ *data.DataFrame, _ fill.FillResult) (fill.FillResult, error) {
	return fill.FillResult{}, fmt.Errorf("adjuster error")
}

var _ = Describe("Pipeline", func() {
	It("returns the base model result when no adjusters are present", func() {
		pipe := fill.NewPipeline(&stubBase{price: 100.0}, nil)
		order := broker.Order{Qty: 50}
		result, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(100.0))
		Expect(result.Quantity).To(Equal(50.0))
	})

	It("chains adjusters in order", func() {
		pipe := fill.NewPipeline(
			&stubBase{price: 100.0},
			[]fill.Adjuster{&addAdjuster{amount: 5.0}, &addAdjuster{amount: 3.0}},
		)
		order := broker.Order{Qty: 50}
		result, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(108.0))
	})

	It("propagates base model errors", func() {
		pipe := fill.NewPipeline(&errBase{}, nil)
		order := broker.Order{Qty: 50}
		_, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("base model error"))
	})

	It("propagates adjuster errors and stops the chain", func() {
		pipe := fill.NewPipeline(
			&stubBase{price: 100.0},
			[]fill.Adjuster{&errAdjuster{}, &addAdjuster{amount: 5.0}},
		)
		order := broker.Order{Qty: 50}
		_, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("adjuster error"))
	})
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`
Expected: compilation errors (package does not exist)

- [ ] **Step 4: Implement core types and pipeline**

Create `fill/fill.go` with:

```go
package fill

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// FillResult describes the outcome of a fill computation.
type FillResult struct {
	Price    float64
	Partial  bool
	Quantity float64
}

// BaseModel produces the initial fill price from market data.
type BaseModel interface {
	Fill(ctx context.Context, order broker.Order, bar *data.DataFrame) (FillResult, error)
}

// Adjuster modifies a FillResult produced by a BaseModel or prior Adjuster.
type Adjuster interface {
	Adjust(ctx context.Context, order broker.Order, bar *data.DataFrame, current FillResult) (FillResult, error)
}

// DataFetcher provides on-demand data access for models that need more than the current bar.
type DataFetcher interface {
	FetchAt(ctx context.Context, assets []asset.Asset, timestamp time.Time, metrics []data.Metric) (*data.DataFrame, error)
}

// DataFetcherAware is implemented by models that need a DataFetcher injected.
type DataFetcherAware interface {
	SetDataFetcher(DataFetcher)
}

// Pipeline composes a BaseModel with zero or more Adjusters.
type Pipeline struct {
	base      BaseModel
	adjusters []Adjuster
}

// NewPipeline creates a Pipeline from a base model and optional adjusters.
func NewPipeline(base BaseModel, adjusters []Adjuster) *Pipeline {
	return &Pipeline{base: base, adjusters: adjusters}
}

// Fill runs the base model then each adjuster in sequence.
func (pp *Pipeline) Fill(ctx context.Context, order broker.Order, bar *data.DataFrame) (FillResult, error) {
	result, err := pp.base.Fill(ctx, order, bar)
	if err != nil {
		return FillResult{}, err
	}

	for _, adj := range pp.adjusters {
		result, err = adj.Adjust(ctx, order, bar, result)
		if err != nil {
			return FillResult{}, err
		}
	}

	return result, nil
}

// SetDataFetcher propagates the fetcher to any base model or adjuster that implements DataFetcherAware.
func (pp *Pipeline) SetDataFetcher(df DataFetcher) {
	if aware, ok := pp.base.(DataFetcherAware); ok {
		aware.SetDataFetcher(df)
	}

	for _, adj := range pp.adjusters {
		if aware, ok := adj.(DataFetcherAware); ok {
			aware.SetDataFetcher(df)
		}
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`
Expected: PASS

- [ ] **Step 6: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./fill/...`
Expected: no issues. Fix any that appear.

- [ ] **Step 7: Commit**

```bash
git add fill/
git commit -m "feat(fill): add core types, interfaces, and pipeline"
```

---

### Task 2: CloseFill Base Model

**Files:**
- Create: `fill/close.go`
- Create: `fill/close_test.go`

- [ ] **Step 1: Write tests for CloseFill**

Test that it returns the close price from the bar, errors when close price is NaN or zero, and preserves order quantity.

```go
package fill_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/fill"
)

var _ = Describe("CloseFill", func() {
	var (
		aapl asset.Asset
		date time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		date = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("fills at the close price", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{data.MetricClose: 150.0})
		model := fill.Close()

		result, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(150.0))
		Expect(result.Quantity).To(Equal(100.0))
		Expect(result.Partial).To(BeFalse())
	})

	It("returns an error when close price is zero", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{data.MetricClose: 0})
		model := fill.Close()

		_, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).To(HaveOccurred())
	})
})
```

Use a `buildBar` helper function (defined in the suite or a helpers file) that creates a single-row DataFrame from a map of metrics.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`
Expected: compilation error (fill.Close not defined)

- [ ] **Step 3: Implement CloseFill**

Create `fill/close.go`:

```go
package fill

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// closeFill fills orders at the bar's close price.
type closeFill struct{}

// Close returns a BaseModel that fills at the close price.
func Close() BaseModel {
	return &closeFill{}
}

func (cf *closeFill) Fill(_ context.Context, order broker.Order, bar *data.DataFrame) (FillResult, error) {
	price := bar.Value(order.Asset, data.MetricClose)
	if math.IsNaN(price) || price == 0 {
		return FillResult{}, fmt.Errorf("close fill: no close price for %s", order.Asset.Ticker)
	}

	return FillResult{
		Price:    price,
		Quantity: order.Qty,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./fill/...`

- [ ] **Step 6: Commit**

```bash
git add fill/close.go fill/close_test.go
git commit -m "feat(fill): add CloseFill base model"
```

---

### Task 3: VWAPFill Base Model

**Files:**
- Create: `fill/vwap.go`
- Create: `fill/vwap_test.go`

- [ ] **Step 1: Write tests for VWAPFill**

Create `fill/vwap_test.go`:

```go
package fill_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/fill"
)

// mockDataFetcher returns pre-configured intraday DataFrames.
type mockDataFetcher struct {
	df  *data.DataFrame
	err error
}

func (mf *mockDataFetcher) FetchAt(_ context.Context, _ []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	return mf.df, mf.err
}

var _ = Describe("VWAPFill", func() {
	var (
		aapl asset.Asset
		date time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		date = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("returns typical price (H+L+C)/3 when no DataFetcher is set", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{
			data.MetricHigh:  155.0,
			data.MetricLow:   145.0,
			data.MetricClose: 150.0,
		})
		model := fill.VWAP()

		result, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(150.0)) // (155+145+150)/3 = 150
		Expect(result.Quantity).To(Equal(100.0))
	})

	It("returns typical price when DataFetcher returns an error", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{
			data.MetricHigh:  155.0,
			data.MetricLow:   145.0,
			data.MetricClose: 150.0,
		})
		model := fill.VWAP()
		model.(fill.DataFetcherAware).SetDataFetcher(&mockDataFetcher{
			err: fmt.Errorf("no intraday data"),
		})

		result, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(150.0))
	})

	It("returns an error when no OHLC data is available", func() {
		bar := buildBar(date, aapl, map[data.Metric]float64{})
		model := fill.VWAP()

		_, err := model.Fill(context.Background(), broker.Order{Asset: aapl, Qty: 100}, bar)

		Expect(err).To(HaveOccurred())
	})

	// Additional test: computes true VWAP from intraday bars when DataFetcher provides them.
	// Build a multi-row DataFrame with OHLCV at minute intervals, verify the weighted average.
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`

- [ ] **Step 3: Implement VWAPFill**

Create `fill/vwap.go`. The model implements both `BaseModel` and `DataFetcherAware`. When a `DataFetcher` is available, it tries to fetch intraday OHLCV for the order's asset on the bar's date and computes `sum(typicalPrice_i * volume_i) / sum(volume_i)`. On failure or when no fetcher is set, it falls back to `(High + Low + Close) / 3`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./fill/...`

- [ ] **Step 6: Commit**

```bash
git add fill/vwap.go fill/vwap_test.go
git commit -m "feat(fill): add VWAPFill base model with typical-price fallback"
```

---

### Task 4: Slippage Adjuster

**Files:**
- Create: `fill/slippage.go`
- Create: `fill/slippage_test.go`

- [ ] **Step 1: Write tests for Slippage**

Test cases:
- Percentage slippage increases price for buys: `price * (1 + pct)`
- Percentage slippage decreases price for sells: `price * (1 - pct)`
- Fixed slippage adds to price for buys: `price + amount`
- Fixed slippage subtracts from price for sells: `price - amount`
- Quantity and Partial are passed through unchanged
- Zero slippage returns price unchanged

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`

- [ ] **Step 3: Implement Slippage**

Create `fill/slippage.go`. Two constructor options:

```go
func Slippage(opt SlippageOption) Adjuster { ... }
func Percent(pct float64) SlippageOption { ... }
func Fixed(amount float64) SlippageOption { ... }
```

The adjuster checks `order.Side` to determine direction.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./fill/...`

- [ ] **Step 6: Commit**

```bash
git add fill/slippage.go fill/slippage_test.go
git commit -m "feat(fill): add Slippage adjuster with percentage and fixed modes"
```

---

### Task 5: SpreadAware Adjuster

**Files:**
- Create: `fill/spread.go`
- Create: `fill/spread_test.go`

- [ ] **Step 1: Write tests for SpreadAware**

Test cases:
- Uses real bid/ask from bar when `data.Bid` and `data.Ask` are present: buy fills at `ask`, sell fills at `bid`
- Falls back to configured BPS when bid/ask absent: half-spread = `price * bps / 10000`, buy at `price + halfSpread`, sell at `price - halfSpread`
- Returns error when neither bid/ask nor BPS fallback is configured
- Quantity and Partial are passed through unchanged
- Correctly uses the `current.Price` (not bar close) as the base for BPS calculation

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`

- [ ] **Step 3: Implement SpreadAware**

Create `fill/spread.go`:

```go
func SpreadAware(opts ...SpreadOption) Adjuster { ... }
func SpreadBPS(bps int) SpreadOption { ... }
```

The adjuster checks the bar for `data.Bid` and `data.Ask` first. If both are present and nonzero, it uses the actual spread. Otherwise it falls back to the configured BPS estimate applied to `current.Price`. If neither source is available, it returns an error.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./fill/...`

- [ ] **Step 6: Commit**

```bash
git add fill/spread.go fill/spread_test.go
git commit -m "feat(fill): add SpreadAware adjuster with bid/ask and BPS fallback"
```

---

### Task 6: MarketImpact Adjuster

**Files:**
- Create: `fill/impact.go`
- Create: `fill/impact_test.go`

- [ ] **Step 1: Write tests for MarketImpact**

Test cases:
- Small order (1% of volume) with LargeCap preset: minimal price impact, full fill
- Large order exceeding SmallCap threshold (>2% of volume): partial fill, quantity capped
- MicroCap preset has higher impact coefficient than LargeCap
- Buy side: price increases by `price * coefficient * sqrt(qty / volume)`
- Sell side: price decreases by same formula
- Returns error when volume is zero or NaN (no volume data)
- Partial fill sets `Partial: true` and reduces `Quantity`

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`

- [ ] **Step 3: Implement MarketImpact**

Create `fill/impact.go`:

```go
// ImpactPreset bundles coefficient and volume threshold.
type ImpactPreset struct {
	Coefficient      float64
	PartialThreshold float64 // fraction of daily volume above which fill is partial
}

var (
	LargeCap = ImpactPreset{Coefficient: 0.1, PartialThreshold: 0.05}
	SmallCap = ImpactPreset{Coefficient: 0.3, PartialThreshold: 0.02}
	MicroCap = ImpactPreset{Coefficient: 0.5, PartialThreshold: 0.01}
)

func MarketImpact(preset ImpactPreset) Adjuster { ... }
```

The adjuster reads `data.Volume` from the bar. Computes `participation = qty / volume`. If `participation > preset.PartialThreshold`, caps quantity at `volume * preset.PartialThreshold` and sets `Partial: true`. Computes `impact = preset.Coefficient * math.Sqrt(participation)` and adjusts price directionally.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./fill/...`

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./fill/...`

- [ ] **Step 6: Commit**

```bash
git add fill/impact.go fill/impact_test.go
git commit -m "feat(fill): add MarketImpact adjuster with LargeCap/SmallCap/MicroCap presets"
```

---

### Task 7: Integrate Fill Pipeline into SimulatedBroker

**Files:**
- Modify: `engine/simulated_broker.go`
- Modify: `engine/option.go`
- Modify: `engine/simulated_broker_test.go`

- [ ] **Step 1: Write integration tests**

Add new test cases to `engine/simulated_broker_test.go`:

1. "uses configured fill model instead of close price" -- create a `SimulatedBroker` with a custom `fill.Pipeline` using a stub base model that returns a different price than close. Verify the fill price matches the stub.
2. "defaults to close-price fill when no fill model is configured" -- verify existing behavior is unchanged (existing tests already cover this, but add an explicit assertion that `fillPipeline` defaults to `fill.Close()`)
3. "applies adjusters in order" -- configure slippage adjuster and verify price is adjusted
4. "handles partial fills from market impact" -- configure MarketImpact with low volume bar, verify partial fill emitted and remainder queued as pending
5. "cancels partial fill remainder after second bar" -- verify the 2-bar lifecycle: partial on bar 1, retry on bar 2, cancel if still partial
6. "converts dollar-amount orders using base model price before adjusters" -- configure a stub base model returning price=200 with a slippage adjuster. For an order with Amount=1000, verify qty = floor(1000/200) = 5, and the final fill price includes slippage applied to 200.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./engine/...`

- [ ] **Step 3: Add fill pipeline to SimulatedBroker**

Modify `engine/simulated_broker.go`:
- Add `fillPipeline *fill.Pipeline` field to the struct
- In `NewSimulatedBroker()`, initialize with `fill.NewPipeline(fill.Close(), nil)`
- Add `SetFillPipeline(p *fill.Pipeline)` method

Modify `engine/option.go`:
- Add `fillBaseModel fill.BaseModel` and `fillAdjusters []fill.Adjuster` fields to the `Engine` struct
- Add `WithFillModel(base fill.BaseModel, adjusters ...fill.Adjuster) Option` that stores these on the engine
- In `createAccount()` (engine.go), after creating the `SimulatedBroker`, check if `e.fillBaseModel != nil` and call `sb.SetFillPipeline(fill.NewPipeline(e.fillBaseModel, e.fillAdjusters))`
- If `WithBroker` is used (user supplies their own broker), `WithFillModel` is silently ignored since the user's broker handles its own fill logic

- [ ] **Step 4: Delegate Submit to fill pipeline with correct dollar-amount ordering**

In `SimulatedBroker.Submit`, replace the hardcoded close-price logic (lines 93-107) with a two-phase approach per the spec:

**Phase 1: Base model determines the price**
```go
baseResult, err := b.fillPipeline.FillBase(ctx, order, df)
if err != nil {
    return fmt.Errorf("simulated broker: fill model: %w", err)
}

// Convert dollar-amount orders BETWEEN base and adjusters
qty := baseResult.Quantity
if qty == 0 && order.Amount > 0 {
    qty = math.Floor(order.Amount / baseResult.Price)
}
baseResult.Quantity = qty
```

**Phase 2: Run adjusters on the result with computed quantity**
```go
result, err := b.fillPipeline.Adjust(ctx, order, df, baseResult)
if err != nil {
    return fmt.Errorf("simulated broker: fill adjuster: %w", err)
}
```

This requires splitting the `Pipeline.Fill` method into `FillBase` (runs base model only) and `Adjust` (runs adjusters only), plus keeping the combined `Fill` for non-broker callers. Add these methods to `Pipeline`:

```go
func (pp *Pipeline) FillBase(ctx context.Context, order broker.Order, bar *data.DataFrame) (FillResult, error) {
    return pp.base.Fill(ctx, order, bar)
}

func (pp *Pipeline) Adjust(ctx context.Context, order broker.Order, bar *data.DataFrame, current FillResult) (FillResult, error) {
    result := current
    for _, adj := range pp.adjusters {
        var err error
        result, err = adj.Adjust(ctx, order, bar, result)
        if err != nil {
            return FillResult{}, err
        }
    }
    return result, nil
}
```

Keep the margin check logic unchanged but use `result.Price` instead of `price`.

Handle partial fills: if `result.Partial`, emit a fill for the partial quantity, then store the remainder as a pending market order with a `pendingBars int` field (add to the struct alongside the existing `pending` map). In `EvaluatePending`, also process partial remainders: run the fill pipeline again with the reduced quantity. If still partial after the second bar (`pendingBars >= 2`), cancel the remainder and remove it from pending.

- [ ] **Step 5: Wire DataFetcher propagation**

Add a `SetDataFetcher(df fill.DataFetcher)` method to `SimulatedBroker` that calls `b.fillPipeline.SetDataFetcher(df)`.

In `engine/backtest.go`, after calling `sb.SetPriceProvider(e, date)` (line 325), also call `sb.SetDataFetcher(e)` since the `Engine` already has a `FetchAt` method matching the `fill.DataFetcher` interface. Add a compile-time check:

```go
var _ fill.DataFetcher = (*Engine)(nil)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./engine/...`

- [ ] **Step 7: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./...`

- [ ] **Step 8: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./...`

- [ ] **Step 9: Commit**

```bash
git add engine/simulated_broker.go engine/option.go engine/simulated_broker_test.go
git commit -m "feat(engine): integrate fill pipeline into SimulatedBroker"
```

---

### Task 8: Changelog

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Under the `[Unreleased]` section, add to the `Added` subsection:

```markdown
- Strategy authors can configure fill models on the simulated broker for more realistic backtesting (VWAP, spread-aware, market impact, slippage), composable via `WithFillModel`
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add changelog entry for fill models"
```
