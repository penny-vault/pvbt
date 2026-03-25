# Volatility Signals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Keltner Channel and Donchian Channel signals to the `signal` package, and relocate metric constants from the central `signal.go` file into each signal's own file.

**Architecture:** Each signal is a standalone function in its own file, returning a single-row DataFrame with upper/middle/lower band metrics via `MergeColumns`. Constants are co-located with the function that produces them.

**Tech Stack:** Go, Ginkgo/Gomega for tests, existing `data.DataFrame` and `data.RollingDataFrame` APIs.

**Spec:** `docs/superpowers/specs/2026-03-24-volatility-signals-design.md`

---

### Task 1: Relocate metric constants from signal.go to individual signal files

**Files:**
- Delete: `signal/signal.go`
- Modify: `signal/momentum.go` (add `MomentumSignal` constant)
- Modify: `signal/volatility.go` (add `VolatilitySignal` constant)
- Modify: `signal/earnings_yield.go` (add `EarningsYieldSignal` constant)
- Modify: `signal/rsi.go` (add `RSISignal` constant)
- Modify: `signal/atr.go` (add `ATRSignal` constant)
- Modify: `signal/macd.go` (add `MACDLineSignal`, `MACDSignalLineSignal`, `MACDHistogramSignal` constants)
- Modify: `signal/bollinger.go` (add `BollingerUpperSignal`, `BollingerMiddleSignal`, `BollingerLowerSignal` constants)
- Modify: `signal/crossover.go` (add `CrossoverFastSignal`, `CrossoverSlowSignal`, `CrossoverSignal` constants)

- [ ] **Step 1: Add constants to each signal file**

Add a `const` block before the function in each file. Each file already imports `"github.com/penny-vault/pvbt/data"` so no import changes are needed.

In `signal/momentum.go`, add before the `Momentum` func:

```go
// MomentumSignal is the metric name for momentum signal output.
const MomentumSignal data.Metric = "Momentum"
```

In `signal/volatility.go`, add before the `Volatility` func:

```go
// VolatilitySignal is the metric name for volatility signal output.
const VolatilitySignal data.Metric = "Volatility"
```

In `signal/earnings_yield.go`, add before the `EarningsYield` func:

```go
// EarningsYieldSignal is the metric name for earnings yield signal output.
const EarningsYieldSignal data.Metric = "EarningsYield"
```

In `signal/rsi.go`, add before the `RSI` func:

```go
// RSISignal is the metric name for RSI signal output.
const RSISignal data.Metric = "RSI"
```

In `signal/atr.go`, add before the `ATR` func:

```go
// ATRSignal is the metric name for ATR signal output.
const ATRSignal data.Metric = "ATR"
```

In `signal/macd.go`, add before the `MACD` func:

```go
const (
	// MACDLineSignal is the metric name for the MACD line output.
	MACDLineSignal data.Metric = "MACDLine"
	// MACDSignalLineSignal is the metric name for the MACD signal line output.
	MACDSignalLineSignal data.Metric = "MACDSignalLine"
	// MACDHistogramSignal is the metric name for the MACD histogram output.
	MACDHistogramSignal data.Metric = "MACDHistogram"
)
```

In `signal/bollinger.go`, add before the `BollingerBands` func:

```go
const (
	// BollingerUpperSignal is the metric name for the upper Bollinger Band.
	BollingerUpperSignal data.Metric = "BollingerUpper"
	// BollingerMiddleSignal is the metric name for the middle Bollinger Band.
	BollingerMiddleSignal data.Metric = "BollingerMiddle"
	// BollingerLowerSignal is the metric name for the lower Bollinger Band.
	BollingerLowerSignal data.Metric = "BollingerLower"
)
```

In `signal/crossover.go`, add before the `Crossover` func:

```go
const (
	// CrossoverFastSignal is the metric name for the fast moving average.
	CrossoverFastSignal data.Metric = "CrossoverFast"
	// CrossoverSlowSignal is the metric name for the slow moving average.
	CrossoverSlowSignal data.Metric = "CrossoverSlow"
	// CrossoverSignal is the metric name for the crossover signal.
	CrossoverSignal data.Metric = "Crossover"
)
```

- [ ] **Step 2: Delete signal.go**

Delete `signal/signal.go` entirely. It only contains the constant block and its imports.

- [ ] **Step 3: Run tests to verify nothing broke**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: All existing tests pass.

- [ ] **Step 4: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No new lint errors.

- [ ] **Step 5: Commit**

```bash
git add signal/
git commit -m "refactor: relocate signal metric constants to their respective files"
```

---

### Task 2: Implement Donchian Channels with tests (TDD)

**Files:**
- Create: `signal/donchian.go`
- Create: `signal/donchian_test.go`

- [ ] **Step 1: Write failing test for basic Donchian computation**

Create `signal/donchian_test.go`:

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

var _ = Describe("DonchianChannels", func() {
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

	It("computes correct upper, middle, and lower channels", func() {
		// AAPL: H=[12,15,11,14,13], L=[9,10,8,11,10]
		//   Upper = max(H) = 15, Lower = min(L) = 8, Middle = (15+8)/2 = 11.5
		// GOOG: H=[120,150,110,140,130], L=[90,100,80,110,100]
		//   Upper = max(H) = 150, Lower = min(L) = 80, Middle = (150+80)/2 = 115
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{
			{12, 15, 11, 14, 13},  // AAPL High
			{9, 10, 8, 11, 10},    // AAPL Low
			{120, 150, 110, 140, 130}, // GOOG High
			{90, 100, 80, 110, 100},   // GOOG Low
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog},
			[]data.Metric{data.MetricHigh, data.MetricLow}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.DonchianChannels(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(ConsistOf(
			signal.DonchianUpperSignal,
			signal.DonchianMiddleSignal,
			signal.DonchianLowerSignal,
		))

		Expect(result.Value(aapl, signal.DonchianUpperSignal)).To(BeNumerically("~", 15.0, 1e-10))
		Expect(result.Value(aapl, signal.DonchianLowerSignal)).To(BeNumerically("~", 8.0, 1e-10))
		Expect(result.Value(aapl, signal.DonchianMiddleSignal)).To(BeNumerically("~", 11.5, 1e-10))

		Expect(result.Value(goog, signal.DonchianUpperSignal)).To(BeNumerically("~", 150.0, 1e-10))
		Expect(result.Value(goog, signal.DonchianLowerSignal)).To(BeNumerically("~", 80.0, 1e-10))
		Expect(result.Value(goog, signal.DonchianMiddleSignal)).To(BeNumerically("~", 115.0, 1e-10))
	})

	It("works with a single data point", func() {
		// A single row is valid: upper=high, lower=low, middle=midpoint.
		times := []time.Time{now}
		vals := [][]float64{{12}, {9}}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.DonchianChannels(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.DonchianUpperSignal)).To(BeNumerically("~", 12.0, 1e-10))
		Expect(result.Value(aapl, signal.DonchianLowerSignal)).To(BeNumerically("~", 9.0, 1e-10))
		Expect(result.Value(aapl, signal.DonchianMiddleSignal)).To(BeNumerically("~", 10.5, 1e-10))
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.DonchianChannels(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/... --focus "DonchianChannels"`
Expected: FAIL -- `signal.DonchianChannels` undefined, `signal.DonchianUpperSignal` undefined.

- [ ] **Step 3: Implement DonchianChannels**

Create `signal/donchian.go`:

```go
// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package signal

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

const (
	// DonchianUpperSignal is the metric name for the upper Donchian Channel.
	DonchianUpperSignal data.Metric = "DonchianUpper"
	// DonchianMiddleSignal is the metric name for the middle Donchian Channel.
	DonchianMiddleSignal data.Metric = "DonchianMiddle"
	// DonchianLowerSignal is the metric name for the lower Donchian Channel.
	DonchianLowerSignal data.Metric = "DonchianLower"
)

// DonchianChannels computes the Donchian Channels (upper, middle, lower) for
// each asset in the universe over the given period. Upper is the rolling max
// of High, lower is the rolling min of Low, middle is the midpoint. Returns a
// single-row DataFrame with DonchianUpperSignal, DonchianMiddleSignal,
// DonchianLowerSignal.
func DonchianChannels(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow)
	if err != nil {
		return data.WithErr(fmt.Errorf("DonchianChannels: %w", err))
	}

	if df.Len() < 1 {
		return data.WithErr(fmt.Errorf("DonchianChannels: need at least 1 data point, got %d", df.Len()))
	}

	windowSize := df.Len()

	upper := df.Rolling(windowSize).Max().Last().
		RenameMetric(data.MetricHigh, DonchianUpperSignal).
		Metrics(DonchianUpperSignal)
	lower := df.Rolling(windowSize).Min().Last().
		RenameMetric(data.MetricLow, DonchianLowerSignal).
		Metrics(DonchianLowerSignal)

	middle := upper.Add(lower.RenameMetric(DonchianLowerSignal, DonchianUpperSignal)).
		DivScalar(2).
		RenameMetric(DonchianUpperSignal, DonchianMiddleSignal)

	result, err := data.MergeColumns(upper, middle, lower)
	if err != nil {
		return data.WithErr(fmt.Errorf("DonchianChannels: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/... --focus "DonchianChannels"`
Expected: All DonchianChannels tests PASS.

- [ ] **Step 5: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No lint errors.

- [ ] **Step 6: Commit**

```bash
git add signal/donchian.go signal/donchian_test.go
git commit -m "feat: add Donchian Channel volatility signal"
```

---

### Task 3: Implement Keltner Channels with tests (TDD)

**Files:**
- Create: `signal/keltner.go`
- Create: `signal/keltner_test.go`

- [ ] **Step 1: Write failing test for basic Keltner computation**

Create `signal/keltner_test.go`. The hand-calculation for EMA + ATR is more involved, so here is the worked example:

Given 5 data points for AAPL:
- Close = [100, 102, 101, 104, 103]
- High  = [101, 103, 102, 105, 104]
- Low   = [99, 101, 100, 103, 102]

**EMA of Close (window=5, alpha = 2/(5+1) = 1/3):**
- Seed (SMA of first 5) = (100+102+101+104+103)/5 = 102.0
- Only 5 data points, so EMA at last row = 102.0

**ATR (atrPeriod = 4, Wilder smoothing):**
- TR[0] = max(103-101, |103-100|, |101-100|) = max(2, 3, 1) = 3
- TR[1] = max(102-100, |102-102|, |100-102|) = max(2, 0, 2) = 2
- TR[2] = max(105-103, |105-101|, |103-101|) = max(2, 4, 2) = 4
- TR[3] = max(104-102, |104-104|, |102-104|) = max(2, 0, 2) = 2
- avgTR = (3+2+4+2)/4 = 2.75

**Bands (multiplier = 2.0):**
- Upper = 102.0 + 2.0 * 2.75 = 107.5
- Lower = 102.0 - 2.0 * 2.75 = 96.5

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

var _ = Describe("KeltnerChannels", func() {
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

	It("computes hand-calculated Keltner Channels for multiple assets", func() {
		// AAPL: Close=[100,102,101,104,103], High=[101,103,102,105,104], Low=[99,101,100,103,102]
		//   EMA(5) of Close: seed = SMA = (100+102+101+104+103)/5 = 102.0
		//   Only 5 points so EMA = 102.0
		//   TR from row 1: [3, 2, 4, 2], atrPeriod=4, ATR = (3+2+4+2)/4 = 2.75
		//   Upper = 102.0 + 2*2.75 = 107.5, Lower = 102.0 - 2*2.75 = 96.5
		//
		// GOOG: Close=[200,204,202,208,206], High=[202,206,204,210,208], Low=[198,202,200,206,204]
		//   EMA(5) of Close: seed = SMA = (200+204+202+208+206)/5 = 204.0
		//   Only 5 points so EMA = 204.0
		//   TR from row 1: [6, 4, 8, 4], atrPeriod=4, ATR = (6+4+8+4)/4 = 5.5
		//   Upper = 204.0 + 2*5.5 = 215.0, Lower = 204.0 - 2*5.5 = 193.0
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{
			{101, 103, 102, 105, 104},     // AAPL High
			{99, 101, 100, 103, 102},       // AAPL Low
			{100, 102, 101, 104, 103},      // AAPL Close
			{202, 206, 204, 210, 208},      // GOOG High
			{198, 202, 200, 206, 204},      // GOOG Low
			{200, 204, 202, 208, 206},      // GOOG Close
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
			data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.KeltnerChannels(ctx, uu, portfolio.Days(4), 2.0)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(ConsistOf(
			signal.KeltnerUpperSignal,
			signal.KeltnerMiddleSignal,
			signal.KeltnerLowerSignal,
		))

		Expect(result.Value(aapl, signal.KeltnerMiddleSignal)).To(BeNumerically("~", 102.0, 1e-10))
		Expect(result.Value(aapl, signal.KeltnerUpperSignal)).To(BeNumerically("~", 107.5, 1e-10))
		Expect(result.Value(aapl, signal.KeltnerLowerSignal)).To(BeNumerically("~", 96.5, 1e-10))

		Expect(result.Value(goog, signal.KeltnerMiddleSignal)).To(BeNumerically("~", 204.0, 1e-10))
		Expect(result.Value(goog, signal.KeltnerUpperSignal)).To(BeNumerically("~", 215.0, 1e-10))
		Expect(result.Value(goog, signal.KeltnerLowerSignal)).To(BeNumerically("~", 193.0, 1e-10))
	})

	It("uses custom metric for center line when provided", func() {
		// Use AdjClose instead of Close for the EMA center line.
		// ATR still uses High/Low/Close.
		adjCloses := []float64{100, 102, 101, 104, 103}
		closes := []float64{100, 102, 101, 104, 103}
		highs := []float64{101, 103, 102, 105, 104}
		lows := []float64{99, 101, 100, 103, 102}

		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{highs, lows, closes, adjCloses}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.AdjClose},
			data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.KeltnerChannels(ctx, uu, portfolio.Days(4), 2.0, data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		// With identical values, result should be same as default metric test.
		Expect(result.Value(aapl, signal.KeltnerMiddleSignal)).To(BeNumerically("~", 102.0, 1e-10))
	})

	It("returns error on degenerate window (fewer than 2 rows)", func() {
		times := []time.Time{now}
		vals := [][]float64{{101}, {99}, {100}}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose},
			data.Daily, vals)

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.KeltnerChannels(ctx, uu, portfolio.Days(0), 2.0)
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error to Err", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.KeltnerChannels(ctx, uu, portfolio.Days(4), 2.0)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/... --focus "KeltnerChannels"`
Expected: FAIL -- `signal.KeltnerChannels` undefined.

- [ ] **Step 3: Implement KeltnerChannels**

Create `signal/keltner.go`:

```go
// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

const (
	// KeltnerUpperSignal is the metric name for the upper Keltner Channel.
	KeltnerUpperSignal data.Metric = "KeltnerUpper"
	// KeltnerMiddleSignal is the metric name for the middle Keltner Channel (EMA).
	KeltnerMiddleSignal data.Metric = "KeltnerMiddle"
	// KeltnerLowerSignal is the metric name for the lower Keltner Channel.
	KeltnerLowerSignal data.Metric = "KeltnerLower"
)

// KeltnerChannels computes the Keltner Channels (upper, middle, lower) for
// each asset in the universe over the given period. The center line is an EMA
// of the price metric (defaults to Close). Bands are placed at +/- atrMultiplier
// times the ATR. Returns a single-row DataFrame with KeltnerUpperSignal,
// KeltnerMiddleSignal, KeltnerLowerSignal.
func KeltnerChannels(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, atrMultiplier float64, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	// Always fetch High, Low, Close for ATR, plus the custom metric if different.
	fetchMetrics := []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}
	if metric != data.MetricClose {
		fetchMetrics = append(fetchMetrics, metric)
	}

	df, err := assetUniverse.Window(ctx, period, fetchMetrics...)
	if err != nil {
		return data.WithErr(fmt.Errorf("KeltnerChannels: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("KeltnerChannels: need at least 2 data points, got %d", numRows))
	}

	// Compute EMA of the price metric for the center line.
	// Filter to only the price metric before EMA to avoid extra High/Low columns.
	windowSize := numRows
	emaDF := df.Metrics(metric).Rolling(windowSize).EMA()
	centerLine := emaDF.Last().RenameMetric(metric, KeltnerMiddleSignal)

	// Compute ATR inline (same logic as signal.ATR to avoid redundant data fetch).
	atrPeriod := numRows - 1
	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	atrCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		trValues := make([]float64, numRows-1)
		for jj := 1; jj < numRows; jj++ {
			highLow := highs[jj] - lows[jj]
			highPrevClose := math.Abs(highs[jj] - closes[jj-1])
			lowPrevClose := math.Abs(lows[jj] - closes[jj-1])
			trValues[jj-1] = math.Max(highLow, math.Max(highPrevClose, lowPrevClose))
		}

		avgTR := 0.0
		for kk := range atrPeriod {
			avgTR += trValues[kk]
		}

		avgTR /= float64(atrPeriod)

		for kk := atrPeriod; kk < len(trValues); kk++ {
			avgTR = (avgTR*float64(atrPeriod-1) + trValues[kk]) / float64(atrPeriod)
		}

		atrCols[ii] = []float64{avgTR}
	}

	atrDF, err := data.NewDataFrame(lastTime, assets, []data.Metric{KeltnerMiddleSignal}, df.Frequency(), atrCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("KeltnerChannels: %w", err))
	}

	upper := centerLine.Add(atrDF.MulScalar(atrMultiplier)).RenameMetric(KeltnerMiddleSignal, KeltnerUpperSignal)
	lower := centerLine.Sub(atrDF.MulScalar(atrMultiplier)).RenameMetric(KeltnerMiddleSignal, KeltnerLowerSignal)

	result, err := data.MergeColumns(centerLine, upper, lower)
	if err != nil {
		return data.WithErr(fmt.Errorf("KeltnerChannels: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/... --focus "KeltnerChannels"`
Expected: All KeltnerChannels tests PASS.

- [ ] **Step 5: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No lint errors.

- [ ] **Step 6: Commit**

```bash
git add signal/keltner.go signal/keltner_test.go
git commit -m "feat: add Keltner Channel volatility signal"
```

---

### Task 4: Update doc.go and changelog

**Files:**
- Modify: `signal/doc.go`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add new signals to doc.go Built-in Signals list**

Add two entries to the list in `signal/doc.go` after the ATR entry:

```go
//   - [KeltnerChannels](ctx, u, period, atrMultiplier, metrics...): Upper, middle, and lower Keltner Channels (EMA center, ATR bands).
//   - [DonchianChannels](ctx, u, period): Upper, middle, and lower Donchian Channels (rolling high/low).
```

- [ ] **Step 2: Add changelog entry**

Add to the `### Added` section under `## [Unreleased]` in `CHANGELOG.md`:

```markdown
- Strategies can use Keltner Channel and Donchian Channel volatility signals to adapt position sizing and stops to current market volatility.
```

- [ ] **Step 3: Run full test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: All tests pass.

- [ ] **Step 4: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No lint errors.

- [ ] **Step 5: Commit**

```bash
git add signal/doc.go CHANGELOG.md
git commit -m "docs: add Keltner and Donchian channels to signal doc and changelog"
```
