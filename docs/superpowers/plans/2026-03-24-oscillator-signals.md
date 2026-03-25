# Oscillator Signals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Stochastic Oscillator (fast/slow), Williams %R, and CCI signals to the signal package, plus a comprehensive signal reference doc.

**Architecture:** Each oscillator is a standalone function in the `signal` package following the ATR pattern (multi-metric column access for High/Low/Close). Multi-output signals use `data.MergeColumns`. Constants live in their respective signal files.

**Tech Stack:** Go, Ginkgo/Gomega for tests, DataFrame API for computation.

**Spec:** `docs/superpowers/specs/2026-03-24-oscillator-signals-design.md`

---

### Task 1: Williams %R Signal

The simplest oscillator -- single metric output, straightforward formula. Good starting point.

**Files:**
- Create: `signal/williams_r.go`
- Create: `signal/williams_r_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/williams_r_test.go`:

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

var _ = Describe("WilliamsR", func() {
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

	It("computes hand-calculated Williams %%R correctly", func() {
		// 5-day window: H=[12,11,13,14,12], L=[9,8,10,11,9], C=[10,10,12,13,11]
		// Highest High = 14, Lowest Low = 8, Close = 11
		// %R = (14 - 11) / (14 - 8) * -100 = 3/6 * -100 = -50
		highs := []float64{12, 11, 13, 14, 12}
		lows := []float64{9, 8, 10, 11, 9}
		closes := []float64{10, 10, 12, 13, 11}

		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.WilliamsRSignal}))
		Expect(result.Value(aapl, signal.WilliamsRSignal)).To(BeNumerically("~", -50.0, 1e-10))
	})

	It("returns 0 when close equals highest high", func() {
		// Close == Highest High -> %R = 0
		highs := []float64{10, 12, 15}
		lows := []float64{8, 9, 11}
		closes := []float64{9, 11, 15}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.WilliamsRSignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("returns -100 when close equals lowest low", func() {
		// Close == Lowest Low -> %R = -100
		highs := []float64{15, 14, 13}
		lows := []float64{10, 9, 8}
		closes := []float64{12, 10, 8}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.WilliamsRSignal)).To(BeNumerically("~", -100.0, 1e-10))
	})

	It("produces NaN for flat market", func() {
		// All prices identical -> HH == LL -> division by zero -> NaN
		highs := []float64{10, 10, 10}
		lows := []float64{10, 10, 10}
		closes := []float64{10, 10, 10}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.WilliamsRSignal))).To(BeTrue())
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		// AAPL: H=[12,14], L=[9,9], C=[10,11] -> HH=14, LL=9, %R=(14-11)/(14-9)*-100 = -60
		// MSFT: H=[20,22], L=[18,17], C=[19,20] -> HH=22, LL=17, %R=(22-20)/(22-17)*-100 = -40
		aaplHighs := []float64{12, 14}
		aaplLows := []float64{9, 9}
		aaplCloses := []float64{10, 11}
		msftHighs := []float64{20, 22}
		msftLows := []float64{18, 17}
		msftCloses := []float64{19, 20}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{aaplHighs, aaplLows, aaplCloses, msftHighs, msftLows, msftCloses}
		df, err := data.NewDataFrame(times, assets, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.WilliamsRSignal)).To(BeNumerically("~", -60.0, 1e-10))
		Expect(result.Value(msft, signal.WilliamsRSignal)).To(BeNumerically("~", -40.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.WilliamsR(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/ --focus "WilliamsR"`
Expected: FAIL -- `WilliamsR` and `WilliamsRSignal` not defined.

- [ ] **Step 3: Implement Williams %R**

Create `signal/williams_r.go`:

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

// WilliamsRSignal is the output metric for [WilliamsR]. Values range from
// -100 (close at the lowest low) to 0 (close at the highest high).
const WilliamsRSignal data.Metric = "WilliamsR"

// WilliamsR computes Williams %R for each asset in the universe over the
// given period. It measures where the current close sits relative to the
// high-low range, scaled to -100..0. Returns a single-row DataFrame with
// [WilliamsRSignal].
func WilliamsR(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("WilliamsR: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("WilliamsR: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	wrCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		highestHigh := math.Inf(-1)
		lowestLow := math.Inf(1)

		for jj := range numRows {
			if highs[jj] > highestHigh {
				highestHigh = highs[jj]
			}
			if lows[jj] < lowestLow {
				lowestLow = lows[jj]
			}
		}

		rangeHL := highestHigh - lowestLow
		var wr float64
		if rangeHL == 0 {
			wr = math.NaN()
		} else {
			wr = (highestHigh - closes[numRows-1]) / rangeHL * -100
		}

		wrCols[ii] = []float64{wr}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{WilliamsRSignal}, df.Frequency(), wrCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("WilliamsR: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/ --focus "WilliamsR"`
Expected: PASS

- [ ] **Step 5: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No errors. Fix any issues before proceeding.

- [ ] **Step 6: Commit**

```bash
git add signal/williams_r.go signal/williams_r_test.go
git commit -m "feat(signal): add Williams %R oscillator signal (#23)"
```

---

### Task 2: CCI Signal

Single metric output, slightly more complex formula (typical price, mean deviation).

**Files:**
- Create: `signal/cci.go`
- Create: `signal/cci_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/cci_test.go`:

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

var _ = Describe("CCI", func() {
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

	It("computes hand-calculated CCI correctly", func() {
		// 3-day window:
		// Day 0: H=24, L=22, C=23 -> TP=23.0
		// Day 1: H=25, L=23, C=24 -> TP=24.0
		// Day 2: H=26, L=22, C=25 -> TP=24.333...
		// SMA of TP = (23 + 24 + 24.333...) / 3 = 23.777...
		// Mean Dev = (|23-23.777..| + |24-23.777..| + |24.333..-23.777..|) / 3
		//          = (0.777.. + 0.222.. + 0.555..) / 3 = 1.555../3 = 0.518..
		// CCI = (24.333.. - 23.777..) / (0.015 * 0.518..) = 0.555.. / 0.00777.. = 71.428..
		highs := []float64{24, 25, 26}
		lows := []float64{22, 23, 22}
		closes := []float64{23, 24, 25}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CCI(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.CCISignal}))

		// TP[2]=73/3, SMA=(69/3+72/3+73/3)/3 = 214/9
		// CCI = (73/3 - 214/9) / (0.015 * meanDev)
		// Verify numerically:
		tp2 := (26.0 + 22.0 + 25.0) / 3.0
		sma := (23.0 + 24.0 + tp2) / 3.0
		md := (math.Abs(23.0-sma) + math.Abs(24.0-sma) + math.Abs(tp2-sma)) / 3.0
		expected := (tp2 - sma) / (0.015 * md)
		Expect(result.Value(aapl, signal.CCISignal)).To(BeNumerically("~", expected, 1e-10))
	})

	It("produces NaN for flat market", func() {
		// All prices identical -> Mean Deviation = 0 -> NaN
		highs := []float64{10, 10, 10}
		lows := []float64{10, 10, 10}
		closes := []float64{10, 10, 10}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CCI(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.CCISignal))).To(BeTrue())
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		// 2-day window, just verify they produce different values
		aaplHighs := []float64{12, 14}
		aaplLows := []float64{9, 10}
		aaplCloses := []float64{10, 13}
		msftHighs := []float64{22, 20}
		msftLows := []float64{18, 17}
		msftCloses := []float64{20, 18}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{aaplHighs, aaplLows, aaplCloses, msftHighs, msftLows, msftCloses}
		df, err := data.NewDataFrame(times, assets, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.CCI(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())

		aaplCCI := result.Value(aapl, signal.CCISignal)
		msftCCI := result.Value(msft, signal.CCISignal)
		Expect(aaplCCI).NotTo(Equal(msftCCI))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CCI(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CCI(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/ --focus "CCI"`
Expected: FAIL -- `CCI` and `CCISignal` not defined.

- [ ] **Step 3: Implement CCI**

Create `signal/cci.go`:

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

// CCISignal is the output metric for [CCI]. Values are unbounded; readings
// above +100 suggest overbought conditions, below -100 suggest oversold.
const CCISignal data.Metric = "CCI"

// cciConstant is Lambert's constant, chosen so that roughly 70-80% of CCI
// values fall between -100 and +100 under normal market conditions.
const cciConstant = 0.015

// CCI computes the Commodity Channel Index for each asset in the universe
// over the given period. It measures deviation of the typical price from
// its simple moving average, scaled by the mean absolute deviation. Returns
// a single-row DataFrame with [CCISignal].
func CCI(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("CCI: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("CCI: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	cciCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		// Compute typical prices.
		tp := make([]float64, numRows)
		for jj := range numRows {
			tp[jj] = (highs[jj] + lows[jj] + closes[jj]) / 3.0
		}

		// SMA of typical prices.
		sma := 0.0
		for jj := range numRows {
			sma += tp[jj]
		}
		sma /= float64(numRows)

		// Mean absolute deviation from SMA.
		meanDev := 0.0
		for jj := range numRows {
			meanDev += math.Abs(tp[jj] - sma)
		}
		meanDev /= float64(numRows)

		var cciVal float64
		if meanDev == 0 {
			cciVal = math.NaN()
		} else {
			cciVal = (tp[numRows-1] - sma) / (cciConstant * meanDev)
		}

		cciCols[ii] = []float64{cciVal}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{CCISignal}, df.Frequency(), cciCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("CCI: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/ --focus "CCI"`
Expected: PASS

- [ ] **Step 5: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No errors. Fix any issues before proceeding.

- [ ] **Step 6: Commit**

```bash
git add signal/cci.go signal/cci_test.go
git commit -m "feat(signal): add CCI oscillator signal (#23)"
```

---

### Task 3: Stochastic Oscillator (Fast and Slow)

Most complex -- two functions, four output metrics, multi-metric output, adjusted period fetching.

**Files:**
- Create: `signal/stochastic.go`
- Create: `signal/stochastic_test.go`

- [ ] **Step 1: Write the failing tests for StochasticFast**

Create `signal/stochastic_test.go` with the StochasticFast tests:

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

var _ = Describe("StochasticFast", func() {
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

	It("computes hand-calculated fast stochastic correctly", func() {
		// 7-day data with 5-day %K period. We need period.N + 2 = 7 rows
		// to compute 3 %K values for %D SMA.
		//
		// Prices:
		// Day 0: H=12, L=9,  C=10
		// Day 1: H=11, L=8,  C=10
		// Day 2: H=13, L=10, C=12
		// Day 3: H=14, L=11, C=13
		// Day 4: H=12, L=9,  C=11
		// Day 5: H=15, L=10, C=14
		// Day 6: H=13, L=11, C=12
		//
		// %K windows (5-day rolling):
		// Window [0-4]: HH=14, LL=8,  C=11 -> %K = (11-8)/(14-8)*100 = 50.0
		// Window [1-5]: HH=15, LL=8,  C=14 -> %K = (14-8)/(15-8)*100 = 85.714..
		// Window [2-6]: HH=15, LL=9,  C=12 -> %K = (12-9)/(15-9)*100 = 50.0
		//
		// %D = SMA of last 3 %K = (50 + 85.714.. + 50) / 3 = 61.904..
		// Final %K = 50.0 (last window)
		highs := []float64{12, 11, 13, 14, 12, 15, 13}
		lows := []float64{9, 8, 10, 11, 9, 10, 11}
		closes := []float64{10, 10, 12, 13, 11, 14, 12}

		times := make([]time.Time, 7)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-6)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticFast(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))

		Expect(result.Value(aapl, signal.StochasticKSignal)).To(BeNumerically("~", 50.0, 1e-10))
		Expect(result.Value(aapl, signal.StochasticDSignal)).To(BeNumerically("~", (50.0+600.0/7.0+50.0)/3.0, 1e-10))
	})

	It("produces NaN for flat market", func() {
		highs := []float64{10, 10, 10, 10, 10}
		lows := []float64{10, 10, 10, 10, 10}
		closes := []float64{10, 10, 10, 10, 10}

		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticFast(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.StochasticKSignal))).To(BeTrue())
		Expect(math.IsNaN(result.Value(aapl, signal.StochasticDSignal))).To(BeTrue())
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		// 4-day data with 2-day %K period (need period+2=4 rows for 3 %K values)
		aaplHighs := []float64{12, 14, 13, 15}
		aaplLows := []float64{9, 10, 11, 12}
		aaplCloses := []float64{11, 13, 12, 14}
		msftHighs := []float64{22, 20, 24, 21}
		msftLows := []float64{18, 17, 20, 18}
		msftCloses := []float64{20, 19, 23, 20}

		times := make([]time.Time, 4)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-3)
		}
		vals := [][]float64{aaplHighs, aaplLows, aaplCloses, msftHighs, msftLows, msftCloses}
		df, err := data.NewDataFrame(times, assets, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.StochasticFast(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())

		aaplK := result.Value(aapl, signal.StochasticKSignal)
		msftK := result.Value(msft, signal.StochasticKSignal)
		Expect(aaplK).NotTo(Equal(msftK))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticFast(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticFast(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/ --focus "StochasticFast"`
Expected: FAIL -- `StochasticFast`, `StochasticKSignal`, `StochasticDSignal` not defined.

- [ ] **Step 3: Implement StochasticFast**

Create `signal/stochastic.go`:

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

// Output metrics for [StochasticFast].
const (
	// StochasticKSignal is the raw %K line (0-100).
	StochasticKSignal data.Metric = "StochasticK"

	// StochasticDSignal is the 3-period SMA of %K (0-100).
	StochasticDSignal data.Metric = "StochasticD"
)

// Output metrics for [StochasticSlow].
const (
	// StochasticSlowKSignal is the smoothed %K line (0-100).
	StochasticSlowKSignal data.Metric = "StochasticSlowK"

	// StochasticSlowDSignal is the 3-period SMA of Slow %K (0-100).
	StochasticSlowDSignal data.Metric = "StochasticSlowD"
)

// stochasticDPeriod is the universal convention for the %D smoothing period.
const stochasticDPeriod = 3

// StochasticFast computes the Fast Stochastic Oscillator for each asset in
// the universe. %K measures where the close sits relative to the high-low
// range over the period; %D is a 3-period SMA of %K. The function fetches
// period.N + 2 extra bars to compute the %D smoothing. Returns a single-row
// DataFrame with [StochasticKSignal] and [StochasticDSignal].
func StochasticFast(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	// Need period.N + 2 total bars: period.N bars per %K window, sliding
	// 3 times to get 3 %K values for the %D SMA.
	adjustedPeriod := portfolio.Days(period.N + stochasticDPeriod - 1)

	df, err := assetUniverse.Window(ctx, adjustedPeriod, data.MetricHigh, data.MetricLow, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticFast: %w", err))
	}

	numRows := df.Len()
	minRows := period.N + stochasticDPeriod - 1
	if numRows < minRows {
		return data.WithErr(fmt.Errorf("StochasticFast: need at least %d data points, got %d", minRows, numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	kCols := make([][]float64, len(assets))
	dCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		kValues := stochasticKSeries(highs, lows, closes, period.N)

		lastK := kValues[len(kValues)-1]

		// %D = SMA of last 3 %K values.
		dSum := 0.0
		allNaN := true

		for jj := len(kValues) - stochasticDPeriod; jj < len(kValues); jj++ {
			if math.IsNaN(kValues[jj]) {
				dSum = math.NaN()
				break
			}

			dSum += kValues[jj]
			allNaN = false
		}

		var lastD float64
		if allNaN || math.IsNaN(dSum) {
			lastD = math.NaN()
		} else {
			lastD = dSum / float64(stochasticDPeriod)
		}

		kCols[ii] = []float64{lastK}
		dCols[ii] = []float64{lastD}
	}

	kDF, err := data.NewDataFrame(lastTime, assets, []data.Metric{StochasticKSignal}, df.Frequency(), kCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticFast: %w", err))
	}

	dDF, err := data.NewDataFrame(lastTime, assets, []data.Metric{StochasticDSignal}, df.Frequency(), dCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticFast: %w", err))
	}

	result, err := data.MergeColumns(kDF, dDF)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticFast: %w", err))
	}

	return result
}

// stochasticKSeries computes the rolling %K values for the given window size.
// Returns one %K per rolling window position.
func stochasticKSeries(highs, lows, closes []float64, windowSize int) []float64 {
	numRows := len(highs)
	numK := numRows - windowSize + 1
	kValues := make([]float64, numK)

	for ii := range numK {
		start := ii
		end := ii + windowSize

		highestHigh := math.Inf(-1)
		lowestLow := math.Inf(1)

		for jj := start; jj < end; jj++ {
			if highs[jj] > highestHigh {
				highestHigh = highs[jj]
			}

			if lows[jj] < lowestLow {
				lowestLow = lows[jj]
			}
		}

		rangeHL := highestHigh - lowestLow
		if rangeHL == 0 {
			kValues[ii] = math.NaN()
		} else {
			kValues[ii] = (closes[end-1] - lowestLow) / rangeHL * 100
		}
	}

	return kValues
}
```

- [ ] **Step 4: Run StochasticFast tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/ --focus "StochasticFast"`
Expected: PASS

- [ ] **Step 5: Write the failing tests for StochasticSlow**

Append to `signal/stochastic_test.go`:

```go
var _ = Describe("StochasticSlow", func() {
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

	It("computes hand-calculated slow stochastic correctly", func() {
		// 9-day data with 3-day %K period and 3-day smoothing.
		// Need period.N + smoothing.N + 2 = 3+3+2 = 8 rows minimum.
		// Using 9 rows for clarity.
		//
		// Prices:
		// Day 0: H=12, L=9,  C=10
		// Day 1: H=11, L=8,  C=10
		// Day 2: H=13, L=10, C=12
		// Day 3: H=14, L=11, C=13
		// Day 4: H=12, L=9,  C=11
		// Day 5: H=15, L=10, C=14
		// Day 6: H=13, L=11, C=12
		// Day 7: H=16, L=12, C=15
		// Day 8: H=14, L=10, C=13
		//
		// Raw %K (3-day rolling):
		// Window [0-2]: HH=13, LL=8,  C=12 -> %K = (12-8)/(13-8)*100 = 80.0
		// Window [1-3]: HH=14, LL=8,  C=13 -> %K = (13-8)/(14-8)*100 = 83.333..
		// Window [2-4]: HH=14, LL=9,  C=11 -> %K = (11-9)/(14-9)*100 = 40.0
		// Window [3-5]: HH=15, LL=9,  C=14 -> %K = (14-9)/(15-9)*100 = 83.333..
		// Window [4-6]: HH=15, LL=9,  C=12 -> %K = (12-9)/(15-9)*100 = 50.0
		// Window [5-7]: HH=16, LL=10, C=15 -> %K = (15-10)/(16-10)*100 = 83.333..
		// Window [6-8]: HH=16, LL=10, C=13 -> %K = (13-10)/(16-10)*100 = 50.0
		//
		// Slow %K (SMA of raw %K, smoothing=3):
		// SlowK[0] = (80 + 83.333 + 40) / 3 = 67.777..
		// SlowK[1] = (83.333 + 40 + 83.333) / 3 = 68.888..
		// SlowK[2] = (40 + 83.333 + 50) / 3 = 57.777..
		// SlowK[3] = (83.333 + 50 + 83.333) / 3 = 72.222..
		// SlowK[4] = (50 + 83.333 + 50) / 3 = 61.111..
		//
		// Slow %D = SMA of last 3 Slow %K:
		// = (57.777.. + 72.222.. + 61.111..) / 3 = 63.703..
		//
		// Final Slow %K = 61.111..
		highs := []float64{12, 11, 13, 14, 12, 15, 13, 16, 14}
		lows := []float64{9, 8, 10, 11, 9, 10, 11, 12, 10}
		closes := []float64{10, 10, 12, 13, 11, 14, 12, 15, 13}

		times := make([]time.Time, 9)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-8)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))

		expectedSlowK := (50.0 + 250.0/3.0 + 50.0) / 3.0
		Expect(result.Value(aapl, signal.StochasticSlowKSignal)).To(BeNumerically("~", expectedSlowK, 1e-6))

		// Slow %D = SMA of last 3 Slow %K values
		slowK2 := (40.0 + 250.0/3.0 + 50.0) / 3.0
		slowK3 := (250.0/3.0 + 50.0 + 250.0/3.0) / 3.0
		expectedSlowD := (slowK2 + slowK3 + expectedSlowK) / 3.0
		Expect(result.Value(aapl, signal.StochasticSlowDSignal)).To(BeNumerically("~", expectedSlowD, 1e-6))
	})

	It("works with non-default smoothing period", func() {
		// 10-day data with 3-day %K period and 5-day smoothing.
		// Need 3 + 5 + 2 = 10 rows minimum.
		highs := []float64{12, 11, 13, 14, 12, 15, 13, 16, 14, 13}
		lows := []float64{9, 8, 10, 11, 9, 10, 11, 12, 10, 9}
		closes := []float64{10, 10, 12, 13, 11, 14, 12, 15, 13, 11}

		times := make([]time.Time, 10)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-9)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))

		// Just verify it produces finite values (detailed math verified in default smoothing test)
		slowK := result.Value(aapl, signal.StochasticSlowKSignal)
		slowD := result.Value(aapl, signal.StochasticSlowDSignal)
		Expect(math.IsNaN(slowK)).To(BeFalse())
		Expect(math.IsNaN(slowD)).To(BeFalse())
		Expect(slowK).To(BeNumerically(">=", 0.0))
		Expect(slowK).To(BeNumerically("<=", 100.0))
	})

	It("produces NaN for flat market", func() {
		n := 10
		highs := make([]float64, n)
		lows := make([]float64, n)
		closes := make([]float64, n)
		for ii := range n {
			highs[ii] = 10
			lows[ii] = 10
			closes[ii] = 10
		}

		times := make([]time.Time, n)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-n+1)
		}
		vals := [][]float64{highs, lows, closes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.StochasticSlowKSignal))).To(BeTrue())
		Expect(math.IsNaN(result.Value(aapl, signal.StochasticSlowDSignal))).To(BeTrue())
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(3))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.StochasticSlow(ctx, uu, portfolio.Days(3), portfolio.Days(3))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

- [ ] **Step 6: Run StochasticSlow tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/ --focus "StochasticSlow"`
Expected: FAIL -- `StochasticSlow`, `StochasticSlowKSignal`, `StochasticSlowDSignal` not defined.

- [ ] **Step 7: Implement StochasticSlow**

Add to `signal/stochastic.go`:

```go
// StochasticSlow computes the Slow Stochastic Oscillator for each asset in
// the universe. Slow %K is an SMA of the raw %K over the smoothing period;
// Slow %D is a 3-period SMA of Slow %K. The function fetches
// period.N + smoothing.N + 2 bars to compute the required values. Returns a
// single-row DataFrame with [StochasticSlowKSignal] and [StochasticSlowDSignal].
func StochasticSlow(ctx context.Context, assetUniverse universe.Universe, period, smoothing portfolio.Period) *data.DataFrame {
	// Need enough bars for: period.N per raw %K window, smoothing.N raw %K
	// values for each Slow %K, and 3 Slow %K values for %D.
	adjustedN := period.N + smoothing.N + stochasticDPeriod - 2
	adjustedPeriod := portfolio.Days(adjustedN)

	df, err := assetUniverse.Window(ctx, adjustedPeriod, data.MetricHigh, data.MetricLow, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticSlow: %w", err))
	}

	numRows := df.Len()
	minRows := period.N + smoothing.N + stochasticDPeriod - 2
	if numRows < minRows {
		return data.WithErr(fmt.Errorf("StochasticSlow: need at least %d data points, got %d", minRows, numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	slowKCols := make([][]float64, len(assets))
	slowDCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)

		rawK := stochasticKSeries(highs, lows, closes, period.N)

		// Slow %K = SMA of raw %K over smoothing window.
		numSlowK := len(rawK) - smoothing.N + 1
		slowKValues := make([]float64, numSlowK)

		for jj := range numSlowK {
			sum := 0.0
			hasNaN := false

			for kk := jj; kk < jj+smoothing.N; kk++ {
				if math.IsNaN(rawK[kk]) {
					hasNaN = true
					break
				}

				sum += rawK[kk]
			}

			if hasNaN {
				slowKValues[jj] = math.NaN()
			} else {
				slowKValues[jj] = sum / float64(smoothing.N)
			}
		}

		lastSlowK := slowKValues[len(slowKValues)-1]

		// Slow %D = SMA of last 3 Slow %K values.
		dSum := 0.0
		hasNaN := false

		for jj := len(slowKValues) - stochasticDPeriod; jj < len(slowKValues); jj++ {
			if math.IsNaN(slowKValues[jj]) {
				hasNaN = true
				break
			}

			dSum += slowKValues[jj]
		}

		var lastSlowD float64
		if hasNaN {
			lastSlowD = math.NaN()
		} else {
			lastSlowD = dSum / float64(stochasticDPeriod)
		}

		slowKCols[ii] = []float64{lastSlowK}
		slowDCols[ii] = []float64{lastSlowD}
	}

	kDF, err := data.NewDataFrame(lastTime, assets, []data.Metric{StochasticSlowKSignal}, df.Frequency(), slowKCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticSlow: %w", err))
	}

	dDF, err := data.NewDataFrame(lastTime, assets, []data.Metric{StochasticSlowDSignal}, df.Frequency(), slowDCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticSlow: %w", err))
	}

	result, err := data.MergeColumns(kDF, dDF)
	if err != nil {
		return data.WithErr(fmt.Errorf("StochasticSlow: %w", err))
	}

	return result
}
```

- [ ] **Step 8: Run all Stochastic tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/ --focus "Stochastic"`
Expected: PASS

- [ ] **Step 9: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No errors. Fix any issues before proceeding.

- [ ] **Step 10: Commit**

```bash
git add signal/stochastic.go signal/stochastic_test.go
git commit -m "feat(signal): add Stochastic Oscillator (fast and slow) signals (#23)"
```

---

### Task 4: Update Package Documentation

**Files:**
- Modify: `signal/doc.go`

- [ ] **Step 1: Add new signals to the Built-in Signals list**

In `signal/doc.go`, add these lines after the existing ATR entry in the Built-in Signals list:

```go
//   - [StochasticFast](ctx, u, period): Fast Stochastic Oscillator (%K and %D).
//   - [StochasticSlow](ctx, u, period, smoothing): Slow Stochastic Oscillator (smoothed %K and %D).
//   - [WilliamsR](ctx, u, period): Williams %R momentum oscillator (-100 to 0).
//   - [CCI](ctx, u, period): Commodity Channel Index (unbounded).
```

- [ ] **Step 2: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./signal/...`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add signal/doc.go
git commit -m "docs(signal): add oscillator signals to package documentation (#23)"
```

---

### Task 5: Create Comprehensive Signal Reference Doc

**Files:**
- Create: `docs/signals.md`
- Modify: `docs/data.md`

- [ ] **Step 1: Create `docs/signals.md`**

Write the comprehensive signal reference document. It should contain:

1. Overview section (adapted from `docs/data.md` lines 410-429 and `signal/doc.go`)
2. A table of all built-in signals (all 12: existing 8 + new 4)
3. Per-signal reference sections with: description, signature, parameters, output metrics, value range, and a brief usage example
4. Composing signals section (from `signal/doc.go` lines 49-57)
5. Custom signals section (from `docs/data.md` lines 444-456)
6. Error handling section (from `signal/doc.go` lines 59-67)

Existing signals to document:
- `Momentum(ctx, u, period, metrics...)` -- percent change, output: MomentumSignal
- `EarningsYield(ctx, u, t...)` -- EPS/price, output: EarningsYieldSignal
- `Volatility(ctx, u, period, metrics...)` -- rolling std of returns, output: VolatilitySignal
- `RSI(ctx, u, period, metrics...)` -- Wilder's RSI 0-100, output: RSISignal
- `MACD(ctx, u, fast, slow, signalPeriod, metrics...)` -- 3 outputs: MACDLineSignal, MACDSignalLineSignal, MACDHistogramSignal
- `BollingerBands(ctx, u, period, numStdDev, metrics...)` -- 3 outputs: BollingerUpperSignal, BollingerMiddleSignal, BollingerLowerSignal
- `Crossover(ctx, u, fastPeriod, slowPeriod, metrics...)` -- 3 outputs: CrossoverFastSignal, CrossoverSlowSignal, CrossoverSignal
- `ATR(ctx, u, period)` -- Wilder's ATR, output: ATRSignal

New signals to document:
- `StochasticFast(ctx, u, period)` -- 2 outputs: StochasticKSignal, StochasticDSignal, range 0-100
- `StochasticSlow(ctx, u, period, smoothing)` -- 2 outputs: StochasticSlowKSignal, StochasticSlowDSignal, range 0-100
- `WilliamsR(ctx, u, period)` -- output: WilliamsRSignal, range -100 to 0
- `CCI(ctx, u, period)` -- output: CCISignal, unbounded

- [ ] **Step 2: Update `docs/data.md`**

Replace the "## Signals" section (lines 410-456) with a brief paragraph linking to the new reference:

```markdown
## Signals

See the [Signal Reference](signals.md) for a complete guide to built-in and custom signals.
```

- [ ] **Step 3: Commit**

```bash
git add docs/signals.md docs/data.md
git commit -m "docs: create comprehensive signal reference and update data.md (#23)"
```

---

### Task 6: Update CHANGELOG

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Add under `## [Unreleased]` > `### Added`:

```markdown
- Stochastic Oscillator (fast and slow), Williams %R, and CCI signals are now available in the signal package for identifying overbought/oversold conditions.
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add oscillator signals to changelog (#23)"
```

---

### Task 7: Full Test Suite Verification

- [ ] **Step 1: Run full signal test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: All tests PASS, no race conditions.

- [ ] **Step 2: Run full project lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && make lint`
Expected: No errors.

- [ ] **Step 3: Run full project test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && make test`
Expected: All tests PASS.
