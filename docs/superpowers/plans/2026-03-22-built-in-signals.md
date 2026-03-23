# Built-in Signals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add RSI, MACD, Bollinger Bands, Crossover, and ATR signals to the `signal` package, plus EMA on `RollingDataFrame`.

**Architecture:** EMA is added as a public method on `RollingDataFrame` (standard smoothing, `alpha = 2/(n+1)`). Five new signal functions in `signal/` follow the existing pattern: take `context.Context`, `universe.Universe`, `portfolio.Period`, return `*data.DataFrame`. RSI and ATR use Wilder's smoothing (`alpha = 1/n`) via `Apply()` instead of `Rolling(n).EMA()`.

**Tech Stack:** Go, Ginkgo/Gomega, gonum/stat

**Spec:** `docs/superpowers/specs/2026-03-22-built-in-signals-design.md`

---

### Task 1: Add EMA to RollingDataFrame

**Files:**
- Modify: `data/rolling_data_frame.go` (append after `Percentile` method, line 234)
- Modify: `data/rolling_data_frame_test.go` (append new `Describe` block)

- [ ] **Step 1: Write the failing tests**

Add to `data/rolling_data_frame_test.go` inside the existing `Describe("RollingDataFrame")` block, after the last `It`:

```go
It("EMA computes exponential moving average", func() {
    // Window=3, alpha=2/(3+1)=0.5
    // Values: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10
    // SMA seed (first 3): (1+2+3)/3 = 2.0
    // idx 2: 2.0 (seed)
    // idx 3: 0.5*4 + 0.5*2.0 = 3.0
    // idx 4: 0.5*5 + 0.5*3.0 = 4.0
    // idx 5: 0.5*6 + 0.5*4.0 = 5.0
    // idx 6: 0.5*7 + 0.5*5.0 = 6.0
    // idx 7: 0.5*8 + 0.5*6.0 = 7.0
    // idx 8: 0.5*9 + 0.5*7.0 = 8.0
    // idx 9: 0.5*10 + 0.5*8.0 = 9.0
    result := df.Rolling(3).EMA()
    Expect(result.Err()).NotTo(HaveOccurred())
    col := result.Column(aapl, data.Price)

    Expect(math.IsNaN(col[0])).To(BeTrue())
    Expect(math.IsNaN(col[1])).To(BeTrue())
    Expect(col[2]).To(BeNumerically("~", 2.0, 1e-10))
    Expect(col[3]).To(BeNumerically("~", 3.0, 1e-10))
    Expect(col[4]).To(BeNumerically("~", 4.0, 1e-10))
    Expect(col[5]).To(BeNumerically("~", 5.0, 1e-10))
    Expect(col[9]).To(BeNumerically("~", 9.0, 1e-10))
})

It("EMA window size 1 returns original values", func() {
    // alpha = 2/(1+1) = 1.0, so EMA = current value
    result := df.Rolling(1).EMA()
    Expect(result.Err()).NotTo(HaveOccurred())
    col := result.Column(aapl, data.Price)
    for idx, val := range col {
        Expect(val).To(Equal(float64(idx + 1)))
    }
})

It("EMA works with multiple assets and metrics", func() {
    base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
    times := make([]time.Time, 5)
    for idx := range times {
        times[idx] = base.AddDate(0, 0, idx)
    }

    vals := [][]float64{
        {10, 20, 30, 40, 50}, // AAPL Price
        {100, 200, 300, 400, 500}, // GOOG Price
    }
    multi, err := data.NewDataFrame(times, []asset.Asset{aapl, goog},
        []data.Metric{data.Price}, data.Daily, vals)
    Expect(err).NotTo(HaveOccurred())

    // Window=3, alpha=0.5
    // AAPL: seed=(10+20+30)/3=20, idx3: 0.5*40+0.5*20=30, idx4: 0.5*50+0.5*30=40
    // GOOG: seed=(100+200+300)/3=200, idx3: 0.5*400+0.5*200=300, idx4: 0.5*500+0.5*300=400
    result := multi.Rolling(3).EMA()
    Expect(result.Err()).NotTo(HaveOccurred())

    aaplCol := result.Column(aapl, data.Price)
    Expect(aaplCol[2]).To(BeNumerically("~", 20.0, 1e-10))
    Expect(aaplCol[3]).To(BeNumerically("~", 30.0, 1e-10))
    Expect(aaplCol[4]).To(BeNumerically("~", 40.0, 1e-10))

    googCol := result.Column(goog, data.Price)
    Expect(googCol[2]).To(BeNumerically("~", 200.0, 1e-10))
    Expect(googCol[3]).To(BeNumerically("~", 300.0, 1e-10))
    Expect(googCol[4]).To(BeNumerically("~", 400.0, 1e-10))
})

It("EMA first n-1 values are NaN", func() {
    result := df.Rolling(5).EMA()
    col := result.Column(aapl, data.Price)
    for idx := 0; idx < 4; idx++ {
        Expect(math.IsNaN(col[idx])).To(BeTrue())
    }
    Expect(math.IsNaN(col[4])).To(BeFalse())
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./data/...`
Expected: FAIL -- `EMA` method not found

- [ ] **Step 3: Implement EMA on RollingDataFrame**

Add to `data/rolling_data_frame.go` after the `Percentile` method:

```go
// EMA returns a DataFrame with the exponential moving average over the
// window. The smoothing factor is alpha = 2 / (n + 1) where n is the
// window size. The first n-1 rows are NaN. The EMA is seeded with the
// simple moving average of the first n values.
func (r *RollingDataFrame) EMA() *DataFrame {
	if r.df.err != nil {
		return WithErr(r.df.err)
	}

	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		windowSize := r.window
		alpha := 2.0 / float64(windowSize+1)

		for idx := range col {
			if idx < windowSize-1 {
				out[idx] = math.NaN()

				continue
			}

			if idx == windowSize-1 {
				out[idx] = stat.Mean(col[:windowSize], nil)

				continue
			}

			out[idx] = alpha*col[idx] + (1-alpha)*out[idx-1]
		}

		return out
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./data/...`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./data/...`
Expected: No issues

- [ ] **Step 6: Commit**

```bash
git add data/rolling_data_frame.go data/rolling_data_frame_test.go
git commit -m "feat: add EMA method to RollingDataFrame"
```

---

### Task 2: Add new signal metric constants

**Files:**
- Modify: `signal/signal.go:24-28` (add constants to the const block)

- [ ] **Step 1: Add constants**

Extend the existing const block in `signal/signal.go`:

```go
const (
	MomentumSignal      data.Metric = "Momentum"
	VolatilitySignal    data.Metric = "Volatility"
	EarningsYieldSignal data.Metric = "EarningsYield"

	RSISignal data.Metric = "RSI"
	ATRSignal data.Metric = "ATR"

	MACDLineSignal       data.Metric = "MACDLine"
	MACDSignalLineSignal data.Metric = "MACDSignalLine"
	MACDHistogramSignal  data.Metric = "MACDHistogram"

	BollingerUpperSignal  data.Metric = "BollingerUpper"
	BollingerMiddleSignal data.Metric = "BollingerMiddle"
	BollingerLowerSignal  data.Metric = "BollingerLower"

	CrossoverFastSignal data.Metric = "CrossoverFast"
	CrossoverSlowSignal data.Metric = "CrossoverSlow"
	CrossoverSignal     data.Metric = "Crossover"
)
```

- [ ] **Step 2: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No issues (unused constants are allowed in Go)

- [ ] **Step 3: Commit**

```bash
git add signal/signal.go
git commit -m "feat: add metric constants for new technical signals"
```

---

### Task 3: Implement Bollinger Bands

Bollinger Bands is the simplest signal -- it only uses `Rolling(n).Mean()` and `Rolling(n).Std()` which already exist. No Wilder's smoothing or multi-metric fetch needed.

**Files:**
- Create: `signal/bollinger.go`
- Create: `signal/bollinger_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/bollinger_test.go`:

```go
package signal_test

import (
	"context"
	"errors"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("BollingerBands", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		goog asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("computes upper, middle, and lower bands", func() {
		// 5 prices, windowSize = df.Len() = 5, 2.0 std devs
		// AAPL: 10, 20, 30, 40, 50
		// Mean of all 5: 30.0
		// Sample std (N-1): sqrt(((10-30)^2+(20-30)^2+(30-30)^2+(40-30)^2+(50-30)^2)/4)
		//                 = sqrt((400+100+0+100+400)/4) = sqrt(250) = 15.8113883...
		// Upper = 30 + 2*15.8113883 = 61.6227766
		// Lower = 30 - 2*15.8113883 = -1.6227766
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{
			{10, 20, 30, 40, 50},
			{100, 200, 300, 400, 500},
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.BollingerBands(ctx, u, portfolio.Days(4), 2.0)
		Expect(result.Err()).NotTo(HaveOccurred())

		aaplStd := math.Sqrt(250.0)
		Expect(result.Value(aapl, signal.BollingerMiddleSignal)).To(BeNumerically("~", 30.0, 1e-10))
		Expect(result.Value(aapl, signal.BollingerUpperSignal)).To(BeNumerically("~", 30.0+2*aaplStd, 1e-10))
		Expect(result.Value(aapl, signal.BollingerLowerSignal)).To(BeNumerically("~", 30.0-2*aaplStd, 1e-10))

		// GOOG: 100,200,300,400,500 => mean=300, sample std=sqrt(25000)=158.1138...
		googStd := math.Sqrt(25000.0)
		Expect(result.Value(goog, signal.BollingerMiddleSignal)).To(BeNumerically("~", 300.0, 1e-10))
		Expect(result.Value(goog, signal.BollingerUpperSignal)).To(BeNumerically("~", 300.0+2*googStd, 1e-10))
		Expect(result.Value(goog, signal.BollingerLowerSignal)).To(BeNumerically("~", 300.0-2*googStd, 1e-10))
	})

	It("uses custom metric when provided", func() {
		times := make([]time.Time, 4)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-3)
		}
		vals := [][]float64{{10, 20, 30, 40}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.BollingerBands(ctx, u, portfolio.Days(3), 1.0, data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		// All 4 values: mean(10,20,30,40)=25, windowSize=df.Len()=4
		Expect(result.Value(aapl, signal.BollingerMiddleSignal)).To(BeNumerically("~", 25.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now}
		vals := [][]float64{{100}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.BollingerBands(ctx, u, portfolio.Days(0), 2.0)
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("timeout")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.BollingerBands(ctx, u, portfolio.Days(20), 2.0)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("timeout"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: FAIL -- `BollingerBands` not defined

- [ ] **Step 3: Implement BollingerBands**

Create `signal/bollinger.go`:

```go
package signal

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// BollingerBands computes upper, middle, and lower Bollinger Bands for
// each asset in the universe. The middle band is a simple moving average
// over the window returned by period. Upper and lower bands are offset
// by numStdDev sample standard deviations.
func BollingerBands(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, numStdDev float64, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := assetUniverse.Window(ctx, period, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("BollingerBands: %w", err))
	}

	windowSize := df.Len()
	if windowSize < 2 {
		return data.WithErr(fmt.Errorf("BollingerBands: need at least 2 data points, got %d", windowSize))
	}

	middle := df.Rolling(windowSize).Mean().Last().RenameMetric(metric, BollingerMiddleSignal)
	stdDev := df.Rolling(windowSize).Std().Last().RenameMetric(metric, BollingerMiddleSignal)

	upper := middle.Add(stdDev.MulScalar(numStdDev)).RenameMetric(BollingerMiddleSignal, BollingerUpperSignal)
	lower := middle.Sub(stdDev.MulScalar(numStdDev)).RenameMetric(BollingerMiddleSignal, BollingerLowerSignal)

	result, mergeErr := data.MergeColumns(middle, upper, lower)
	if mergeErr != nil {
		return data.WithErr(fmt.Errorf("BollingerBands: %w", mergeErr))
	}

	return result
}

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No issues

- [ ] **Step 6: Commit**

```bash
git add signal/bollinger.go signal/bollinger_test.go
git commit -m "feat: add BollingerBands signal"
```

---

### Task 4: Implement Crossover

Crossover uses `Rolling(n).Mean()` (SMA) for both fast and slow periods, plus a comparison to produce +1/-1. Returns three metrics.

**Files:**
- Create: `signal/crossover.go`
- Create: `signal/crossover_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/crossover_test.go`:

```go
package signal_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("Crossover", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("returns +1 when fast SMA is above slow SMA", func() {
		// Uptrending prices: fast SMA will be above slow SMA
		// Prices: 10, 20, 30, 40, 50, 60, 70
		// Fast SMA(2) at last row: (60+70)/2 = 65
		// Slow SMA(5) at last row: (30+40+50+60+70)/5 = 50
		// Fast > Slow => +1
		times := make([]time.Time, 7)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-6)
		}
		vals := [][]float64{{10, 20, 30, 40, 50, 60, 70}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		// fastPeriod.N=2, slowPeriod.N=5 used as rolling window sizes
		result := signal.Crossover(ctx, u, portfolio.Days(2), portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.CrossoverSignal)).To(Equal(1.0))
		Expect(result.Value(aapl, signal.CrossoverFastSignal)).To(BeNumerically("~", 65.0, 1e-10))
		Expect(result.Value(aapl, signal.CrossoverSlowSignal)).To(BeNumerically("~", 50.0, 1e-10))
	})

	It("returns -1 when fast SMA is below slow SMA", func() {
		// Downtrending prices: fast SMA will be below slow SMA
		// Prices: 70, 60, 50, 40, 30, 20, 10
		// Fast SMA(2): (20+10)/2 = 15
		// Slow SMA(5): (50+40+30+20+10)/5 = 30
		// Fast < Slow => -1
		times := make([]time.Time, 7)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-6)
		}
		vals := [][]float64{{70, 60, 50, 40, 30, 20, 10}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Crossover(ctx, u, portfolio.Days(2), portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.CrossoverSignal)).To(Equal(-1.0))
	})

	It("uses custom metric when provided", func() {
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{{10, 20, 30, 40, 50}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		// Fast SMA(2)=45, Slow SMA(4)=35 => +1
		result := signal.Crossover(ctx, u, portfolio.Days(2), portfolio.Days(4), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.CrossoverSignal)).To(Equal(1.0))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now}
		vals := [][]float64{{100}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Crossover(ctx, u, portfolio.Days(0), portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("connection refused")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.Crossover(ctx, u, portfolio.Days(5), portfolio.Days(20))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("connection refused"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: FAIL -- `Crossover` not defined

- [ ] **Step 3: Implement Crossover**

Create `signal/crossover.go`:

```go
package signal

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// Crossover computes a moving average crossover signal for each asset in
// the universe. It returns three metrics: the fast SMA, the slow SMA,
// and a crossover indicator (+1 when fast > slow, -1 when fast < slow).
// The slowPeriod determines the data window fetched from the universe;
// fastPeriod.N and slowPeriod.N are used directly as rolling window sizes.
func Crossover(ctx context.Context, assetUniverse universe.Universe, fastPeriod, slowPeriod portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := assetUniverse.Window(ctx, slowPeriod, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("Crossover: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("Crossover: need at least 2 data points, got %d", df.Len()))
	}

	// Use Period.N directly as rolling window sizes. For daily data,
	// Days(50) and Days(200) give windows of 50 and 200 bars.
	// Cap to available data length to avoid all-NaN results.
	fastWindow := fastPeriod.N
	slowWindow := slowPeriod.N
	if fastWindow > df.Len() {
		fastWindow = df.Len()
	}
	if slowWindow > df.Len() {
		slowWindow = df.Len()
	}

	fastSMA := df.Rolling(fastWindow).Mean().Last().RenameMetric(metric, CrossoverFastSignal)
	slowSMA := df.Rolling(slowWindow).Mean().Last().RenameMetric(metric, CrossoverSlowSignal)

	// Build crossover indicator: +1 if fast > slow, -1 if fast <= slow
	assets := fastSMA.AssetList()
	crossoverCols := make([][]float64, len(assets))
	for ii, ast := range assets {
		fv := fastSMA.Value(ast, CrossoverFastSignal)
		sv := slowSMA.Value(ast, CrossoverSlowSignal)
		if fv > sv {
			crossoverCols[ii] = []float64{1.0}
		} else {
			crossoverCols[ii] = []float64{-1.0}
		}
	}

	crossoverDF, buildErr := data.NewDataFrame(
		fastSMA.Times(), assets,
		[]data.Metric{CrossoverSignal}, fastSMA.Frequency(),
		crossoverCols,
	)
	if buildErr != nil {
		return data.WithErr(fmt.Errorf("Crossover: %w", buildErr))
	}

	result, mergeErr := data.MergeColumns(fastSMA, slowSMA, crossoverDF)
	if mergeErr != nil {
		return data.WithErr(fmt.Errorf("Crossover: %w", mergeErr))
	}

	return result
}

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No issues

- [ ] **Step 6: Commit**

```bash
git add signal/crossover.go signal/crossover_test.go
git commit -m "feat: add Crossover signal"
```

---

### Task 5: Implement RSI

RSI needs Wilder's smoothing (`alpha = 1/n`) via `Apply()`, not `Rolling(n).EMA()`.

**Files:**
- Create: `signal/rsi.go`
- Create: `signal/rsi_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/rsi_test.go`:

```go
package signal_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("RSI", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("computes RSI with Wilder smoothing", func() {
		// 15 prices => 14 changes, period=14 (need 15 rows)
		// Prices: 44, 44.34, 44.09, 43.61, 44.33, 44.83, 45.10,
		//         45.42, 45.84, 46.08, 45.89, 46.03, 45.61, 46.28, 46.28
		// Changes:    +0.34, -0.25, -0.48, +0.72, +0.50, +0.27,
		//         +0.32, +0.42, +0.24, -0.19, +0.14, -0.42, +0.67, 0.00
		// Gains: 0.34, 0, 0, 0.72, 0.50, 0.27, 0.32, 0.42, 0.24, 0, 0.14, 0, 0.67, 0
		// Losses: 0, 0.25, 0.48, 0, 0, 0, 0, 0, 0, 0.19, 0, 0.42, 0, 0
		// Avg gain (SMA 14): (0.34+0.72+0.50+0.27+0.32+0.42+0.24+0.14+0.67)/14 = 3.62/14 = 0.258571...
		// Avg loss (SMA 14): (0.25+0.48+0.19+0.42)/14 = 1.34/14 = 0.095714...
		// RS = 0.258571.../0.095714... = 2.70149...
		// RSI = 100 - 100/(1+2.70149...) = 72.97...
		prices := []float64{
			44, 44.34, 44.09, 43.61, 44.33, 44.83, 45.10,
			45.42, 45.84, 46.08, 45.89, 46.03, 45.61, 46.28, 46.28,
		}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.RSI(ctx, u, portfolio.Days(14))
		Expect(result.Err()).NotTo(HaveOccurred())

		rsiValue := result.Value(aapl, signal.RSISignal)
		Expect(rsiValue).To(BeNumerically("~", 72.97, 0.01))
		Expect(rsiValue).To(BeNumerically(">=", 0.0))
		Expect(rsiValue).To(BeNumerically("<=", 100.0))
	})

	It("returns RSI values between 0 and 100", func() {
		// All gains => RSI should be near 100
		prices := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.RSI(ctx, u, portfolio.Days(14))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.RSISignal)).To(BeNumerically("~", 100.0, 0.01))
	})

	It("uses custom metric when provided", func() {
		prices := make([]float64, 15)
		for ii := range prices {
			prices[ii] = float64(50 + ii)
		}
		times := make([]time.Time, 15)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-14)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.RSI(ctx, u, portfolio.Days(14), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.RSISignal)).To(BeNumerically(">=", 0.0))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now, now.AddDate(0, 0, 1)}
		vals := [][]float64{{100, 110}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now.AddDate(0, 0, 1), fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.RSI(ctx, u, portfolio.Days(1))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("api rate limit")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.RSI(ctx, u, portfolio.Days(14))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("api rate limit"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: FAIL -- `RSI` not defined

- [ ] **Step 3: Implement RSI**

Create `signal/rsi.go`:

```go
package signal

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// RSI computes the Relative Strength Index for each asset in the
// universe using Wilder's smoothing (alpha = 1/n). Returns values
// between 0 and 100.
func RSI(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := assetUniverse.Window(ctx, period, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("RSI: %w", err))
	}

	windowSize := df.Len()
	if windowSize < 3 {
		return data.WithErr(fmt.Errorf("RSI: need at least 3 data points, got %d", windowSize))
	}

	// Number of changes is windowSize - 1; this is the RSI period.
	rsiPeriod := windowSize - 1

	// Compute price changes, then separate gains and losses, smooth
	// with Wilder's method, and compute RSI.
	result := df.Diff().Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))

		// Separate gains and losses from changes (skip index 0 which is NaN from Diff)
		gains := make([]float64, len(col))
		losses := make([]float64, len(col))
		for ii := 1; ii < len(col); ii++ {
			if col[ii] > 0 {
				gains[ii] = col[ii]
			} else {
				losses[ii] = math.Abs(col[ii])
			}
		}

		// First average: simple mean of first rsiPeriod values
		avgGain := 0.0
		avgLoss := 0.0
		for ii := 1; ii <= rsiPeriod; ii++ {
			avgGain += gains[ii]
			avgLoss += losses[ii]
		}
		avgGain /= float64(rsiPeriod)
		avgLoss /= float64(rsiPeriod)

		// Set all rows to NaN except the last
		for ii := range out {
			out[ii] = math.NaN()
		}

		// Wilder's smoothing for any additional rows beyond the initial period
		for ii := rsiPeriod + 1; ii < len(col); ii++ {
			avgGain = (avgGain*float64(rsiPeriod-1) + gains[ii]) / float64(rsiPeriod)
			avgLoss = (avgLoss*float64(rsiPeriod-1) + losses[ii]) / float64(rsiPeriod)
		}

		// RSI at last row
		if avgLoss == 0 {
			out[len(out)-1] = 100.0
		} else {
			rs := avgGain / avgLoss
			out[len(out)-1] = 100.0 - 100.0/(1.0+rs)
		}

		return out
	}).Last().RenameMetric(metric, RSISignal)

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No issues

- [ ] **Step 6: Commit**

```bash
git add signal/rsi.go signal/rsi_test.go
git commit -m "feat: add RSI signal with Wilder smoothing"
```

---

### Task 6: Implement MACD

MACD uses standard EMA via `Rolling(n).EMA()` for fast, slow, and signal lines.

**Files:**
- Create: `signal/macd.go`
- Create: `signal/macd_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/macd_test.go`:

```go
package signal_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("MACD", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("returns three metrics: MACD line, signal line, and histogram", func() {
		// 10 prices, fast=3, slow=5, signal=2
		// Period.N values used as rolling window sizes
		prices := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, u, portfolio.Days(3), portfolio.Days(5), portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.MetricList()).To(ContainElements(
			signal.MACDLineSignal,
			signal.MACDSignalLineSignal,
			signal.MACDHistogramSignal,
		))
	})

	It("MACD line is fast EMA minus slow EMA", func() {
		// Steady uptrend: fast EMA tracks closer to recent prices
		// than slow EMA, so MACD line > 0
		prices := make([]float64, 30)
		for ii := range prices {
			prices[ii] = float64(100 + ii*2)
		}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		// fast.N=5, slow.N=15, signal.N=4 -- used as rolling window sizes
		result := signal.MACD(ctx, u, portfolio.Days(5), portfolio.Days(15), portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MACDLineSignal)).To(BeNumerically(">", 0))
	})

	It("histogram equals MACD line minus signal line", func() {
		prices := make([]float64, 30)
		for ii := range prices {
			prices[ii] = float64(100 + ii)
		}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, u, portfolio.Days(5), portfolio.Days(15), portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())

		macdLine := result.Value(aapl, signal.MACDLineSignal)
		signalLine := result.Value(aapl, signal.MACDSignalLineSignal)
		histogram := result.Value(aapl, signal.MACDHistogramSignal)
		Expect(histogram).To(BeNumerically("~", macdLine-signalLine, 1e-10))
	})

	It("uses custom metric when provided", func() {
		prices := make([]float64, 15)
		for ii := range prices {
			prices[ii] = float64(50 + ii)
		}
		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}
		vals := [][]float64{prices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, u, portfolio.Days(3), portfolio.Days(5), portfolio.Days(2), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MACDLineSignal)).To(BeNumerically(">", 0))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now, now.AddDate(0, 0, 1)}
		vals := [][]float64{{100, 110}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, vals)

		ds := &mockDataSource{currentDate: now.AddDate(0, 0, 1), fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, u, portfolio.Days(1), portfolio.Days(1), portfolio.Days(1))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("server error")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MACD(ctx, u, portfolio.Days(11), portfolio.Days(25), portfolio.Days(8))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("server error"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: FAIL -- `MACD` not defined

- [ ] **Step 3: Implement MACD**

Create `signal/macd.go`:

```go
package signal

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// MACD computes the Moving Average Convergence Divergence for each
// asset in the universe. Returns three metrics: MACDLine (fast EMA -
// slow EMA), MACDSignalLine (EMA of MACD line), and MACDHistogram
// (MACD line - signal line).
//
// The slow period determines the data window fetched from the universe.
// fast.N, slow.N, and signalPeriod.N are used directly as rolling
// window sizes for their respective EMAs.
func MACD(ctx context.Context, assetUniverse universe.Universe, fast, slow, signalPeriod portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := assetUniverse.Window(ctx, slow, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("MACD: %w", err))
	}

	if df.Len() < 3 {
		return data.WithErr(fmt.Errorf("MACD: need at least 3 data points, got %d", df.Len()))
	}

	// Use Period.N directly as rolling window sizes.
	fastWindow := fast.N
	slowWindow := slow.N
	sigWindow := signalPeriod.N

	// Fast EMA and slow EMA of the price series
	fastEMA := df.Rolling(fastWindow).EMA()
	slowEMA := df.Rolling(slowWindow).EMA()

	// MACD line = fast EMA - slow EMA
	// Drop NaN rows so the signal line EMA gets a clean seed.
	macdLine := fastEMA.Sub(slowEMA).Drop(math.NaN())

	// Signal line = EMA of MACD line
	signalLine := macdLine.Rolling(sigWindow).EMA()

	// Histogram = MACD - signal
	histogram := macdLine.Sub(signalLine)

	// Take the last row of each and rename metrics
	macdLast := macdLine.Last().RenameMetric(metric, MACDLineSignal)
	signalLast := signalLine.Last().RenameMetric(metric, MACDSignalLineSignal)
	histLast := histogram.Last().RenameMetric(metric, MACDHistogramSignal)

	result, mergeErr := data.MergeColumns(macdLast, signalLast, histLast)
	if mergeErr != nil {
		return data.WithErr(fmt.Errorf("MACD: %w", mergeErr))
	}

	return result
}

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No issues

- [ ] **Step 6: Commit**

```bash
git add signal/macd.go signal/macd_test.go
git commit -m "feat: add MACD signal"
```

---

### Task 7: Implement ATR

ATR fetches three metrics (High, Low, Close) and uses Wilder's smoothing. No `metrics` variadic.

**Files:**
- Create: `signal/atr.go`
- Create: `signal/atr_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/atr_test.go`:

```go
package signal_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/universe"
)

var _ = Describe("ATR", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("computes ATR with Wilder smoothing", func() {
		// 5 rows, period that gives 5 rows, ATR period = 4 changes
		// High:  12, 11, 13, 14, 12
		// Low:    9,  8, 10, 11,  9
		// Close: 10, 10, 12, 13, 11
		//
		// True Range (starting from row 1):
		// Row 1: max(11-8, |11-10|, |8-10|) = max(3, 1, 2) = 3
		// Row 2: max(13-10, |13-10|, |10-10|) = max(3, 3, 0) = 3
		// Row 3: max(14-11, |14-12|, |11-12|) = max(3, 2, 1) = 3
		// Row 4: max(12-9, |12-13|, |9-13|) = max(3, 1, 4) = 4
		//
		// ATR with Wilder (n=4 changes):
		// First ATR = SMA of TRs: (3+3+3+4)/4 = 3.25
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{
			{12, 11, 13, 14, 12}, // AAPL High
			{9, 8, 10, 11, 9},    // AAPL Low
			{10, 10, 12, 13, 11}, // AAPL Close
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
			data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ATR(ctx, u, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.ATRSignal)).To(BeNumerically("~", 3.25, 1e-10))
	})

	It("handles case where |high - prevClose| dominates", func() {
		// Gap up: prevClose=10, high=16, low=14, close=15
		// TR = max(16-14, |16-10|, |14-10|) = max(2, 6, 4) = 6
		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{
			{11, 16, 16},  // High
			{9, 14, 14},   // Low
			{10, 15, 15},  // Close
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
			data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ATR(ctx, u, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		// TR row1 = max(16-14, |16-10|, |14-10|) = 6
		// TR row2 = max(16-14, |16-15|, |14-15|) = 2
		// ATR = (6+2)/2 = 4
		Expect(result.Value(aapl, signal.ATRSignal)).To(BeNumerically("~", 4.0, 1e-10))
	})

	It("handles case where |low - prevClose| dominates", func() {
		// Gap down: prevClose=20, high=16, low=13, close=14
		// TR = max(16-13, |16-20|, |13-20|) = max(3, 4, 7) = 7
		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{
			{21, 16, 16},  // High
			{19, 13, 13},  // Low
			{20, 14, 14},  // Close
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
			data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ATR(ctx, u, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		// TR row1 = max(3, 4, 7) = 7
		// TR row2 = max(3, |16-14|, |13-14|) = max(3, 2, 1) = 3
		// ATR = (7+3)/2 = 5
		Expect(result.Value(aapl, signal.ATRSignal)).To(BeNumerically("~", 5.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now}
		vals := [][]float64{{10}, {9}, {10}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
			data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ATR(ctx, u, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		u := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ATR(ctx, u, portfolio.Days(14))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

Note: the `math` import is included for potential use of `math.IsNaN`. If the linter flags it as unused, remove it.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: FAIL -- `ATR` not defined

- [ ] **Step 3: Implement ATR**

Create `signal/atr.go`:

```go
package signal

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// ATR computes the Average True Range for each asset in the universe
// using Wilder's smoothing (alpha = 1/n). Always uses High, Low, and
// Close metrics.
func ATR(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("ATR: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("ATR: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	atrPeriod := numRows - 1 // number of TR values

	cols := make([][]float64, len(assets))
	for ii, ast := range assets {
		high := df.Column(ast, data.MetricHigh)
		low := df.Column(ast, data.MetricLow)
		close := df.Column(ast, data.MetricClose)

		// Compute True Range for each row starting from 1
		trValues := make([]float64, atrPeriod)
		for jj := 1; jj < numRows; jj++ {
			highLow := high[jj] - low[jj]
			highPrevClose := math.Abs(high[jj] - close[jj-1])
			lowPrevClose := math.Abs(low[jj] - close[jj-1])
			trValues[jj-1] = math.Max(highLow, math.Max(highPrevClose, lowPrevClose))
		}

		// Wilder's smoothing: start with SMA, then apply alpha = 1/n
		avgTR := 0.0
		for _, tr := range trValues {
			avgTR += tr
		}
		avgTR /= float64(atrPeriod)

		// If we have more TR values than the period, apply Wilder's
		// smoothing iteratively (but with Window() returning exactly
		// period+1 rows, atrPeriod == period, so we just use SMA).
		// For longer series with additional rows beyond period:
		// avgTR = (avgTR*(n-1) + newTR) / n

		cols[ii] = []float64{avgTR}
	}

	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	result, buildErr := data.NewDataFrame(lastTime, assets, []data.Metric{ATRSignal}, df.Frequency(), cols)
	if buildErr != nil {
		return data.WithErr(fmt.Errorf("ATR: %w", buildErr))
	}

	return result
}
```

Note: The implementation manually constructs the result DataFrame since ATR operates across multiple input metrics (High, Low, Close) and produces a single output metric -- it cannot use `Apply()` which works per-column.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No issues

- [ ] **Step 6: Commit**

```bash
git add signal/atr.go signal/atr_test.go
git commit -m "feat: add ATR signal with Wilder smoothing"
```

---

### Task 8: Update doc.go and verify all tests pass

**Files:**
- Modify: `signal/doc.go:27-31` (update built-in signals list)

- [ ] **Step 1: Update signal/doc.go**

Update the `# Built-in Signals` section to list all eight signals:

```go
// # Built-in Signals
//
//   - [Momentum](ctx, u, period, metrics...): Percent change over a lookback period.
//   - [EarningsYield](ctx, u, t...): Earnings per share divided by price.
//   - [Volatility](ctx, u, period, metrics...): Rolling standard deviation of returns.
//   - [RSI](ctx, u, period, metrics...): Relative Strength Index with Wilder smoothing.
//   - [MACD](ctx, u, fast, slow, signalPeriod, metrics...): Moving average convergence divergence (line, signal, histogram).
//   - [BollingerBands](ctx, u, period, numStdDev, metrics...): Upper, middle, and lower Bollinger Bands.
//   - [Crossover](ctx, u, fastPeriod, slowPeriod, metrics...): Moving average crossover signal with fast/slow SMAs.
//   - [ATR](ctx, u, period): Average True Range with Wilder smoothing.
```

- [ ] **Step 2: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./...`
Expected: PASS

- [ ] **Step 3: Run full linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./...`
Expected: No issues

- [ ] **Step 4: Commit**

```bash
git add signal/doc.go
git commit -m "docs: list all eight built-in signals in signal package documentation"
```
