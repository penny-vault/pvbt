# Volume Signals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add five volume-based signals (OBV, VWMA, Accumulation/Distribution, CMF, MFI) to the signal package.

**Architecture:** Each signal is a standalone function in the `signal` package following the CCI/WilliamsR pattern. All fetch via `u.Window()`, iterate per asset, and return a single-row DataFrame. A/D and CMF share a `moneyFlowVolumeSeries` helper. Constants live in their respective signal files.

**Tech Stack:** Go, Ginkgo/Gomega for tests, DataFrame API for computation.

**Spec:** `docs/superpowers/specs/2026-03-25-volume-signals-design.md`

---

### Task 1: OBV Signal

The simplest volume signal -- cumulative running total based on close direction. Good starting point.

**Files:**
- Create: `signal/obv.go`
- Create: `signal/obv_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/obv_test.go`:

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

var _ = Describe("OBV", func() {
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

	It("computes hand-calculated OBV correctly", func() {
		// Day 0: close=10, vol=100 (baseline, no comparison)
		// Day 1: close=12 (up),   vol=150 => OBV = 0 + 150 = 150
		// Day 2: close=11 (down), vol=130 => OBV = 150 - 130 = 20
		// Day 3: close=11 (flat), vol=90  => OBV = 20 + 0 = 20
		// Day 4: close=13 (up),   vol=200 => OBV = 20 + 200 = 220
		closes := []float64{10, 12, 11, 11, 13}
		volumes := []float64{100, 150, 130, 90, 200}

		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.OBVSignal}))
		Expect(result.Value(aapl, signal.OBVSignal)).To(BeNumerically("~", 220.0, 1e-10))
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		// AAPL: close goes up then down => OBV = 200 - 100 = 100
		// MSFT: close goes down then up => OBV = -500 + 600 = 100
		aaplCloses := []float64{10, 12, 11}
		aaplVolumes := []float64{300, 200, 100}
		msftCloses := []float64{50, 48, 52}
		msftVolumes := []float64{400, 500, 600}

		times := []time.Time{now.AddDate(0, 0, -2), now.AddDate(0, 0, -1), now}
		vals := [][]float64{aaplCloses, aaplVolumes, msftCloses, msftVolumes}
		df, err := data.NewDataFrame(times, assets,
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.OBVSignal)).To(BeNumerically("~", 100.0, 1e-10))
		Expect(result.Value(msft, signal.OBVSignal)).To(BeNumerically("~", 100.0, 1e-10))
	})

	It("computes correctly with minimum data", func() {
		// Exactly 2 rows -- the minimum for OBV.
		// Close goes up: OBV = +500.
		closes := []float64{10, 12}
		volumes := []float64{300, 500}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.OBVSignal)).To(BeNumerically("~", 500.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("handles flat prices", func() {
		closes := []float64{10, 10, 10}
		volumes := []float64{100, 200, 300}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		// Flat prices: no volume added or subtracted.
		Expect(result.Value(aapl, signal.OBVSignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.OBV(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "OBV" ./signal/`
Expected: FAIL -- `signal.OBV` and `signal.OBVSignal` are undefined.

- [ ] **Step 3: Write the implementation**

Create `signal/obv.go`:

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
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// OBVSignal is the output metric for [OBV]. Values are unbounded cumulative
// volume; positive indicates net buying pressure, negative indicates net selling.
const OBVSignal data.Metric = "OBV"

// OBV computes On-Balance Volume for each asset in the universe over the
// given period. Starting from zero, volume is added when the close rises
// and subtracted when it falls. Returns a single-row DataFrame with [OBVSignal].
func OBV(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricClose, data.Volume)
	if err != nil {
		return data.WithErr(fmt.Errorf("OBV: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("OBV: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	obvCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		closes := df.Column(aa, data.MetricClose)
		volumes := df.Column(aa, data.Volume)

		obv := 0.0
		for jj := 1; jj < numRows; jj++ {
			if closes[jj] > closes[jj-1] {
				obv += volumes[jj]
			} else if closes[jj] < closes[jj-1] {
				obv -= volumes[jj]
			}
		}

		obvCols[ii] = []float64{obv}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{OBVSignal}, df.Frequency(), obvCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("OBV: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "OBV" ./signal/`
Expected: PASS (all 5 test cases)

- [ ] **Step 5: Commit**

```bash
git add signal/obv.go signal/obv_test.go
git commit -m "feat: add OBV volume signal (#25)"
```

---

### Task 2: VWMA Signal

Volume-weighted moving average. Single metric, straightforward sum ratio.

**Files:**
- Create: `signal/vwma.go`
- Create: `signal/vwma_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/vwma_test.go`:

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

var _ = Describe("VWMA", func() {
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

	It("computes hand-calculated VWMA correctly", func() {
		// VWMA = Sum(Close * Volume) / Sum(Volume)
		// = (10*100 + 12*200 + 14*300) / (100 + 200 + 300)
		// = (1000 + 2400 + 4200) / 600
		// = 7600 / 600 = 12.666...
		closes := []float64{10, 12, 14}
		volumes := []float64{100, 200, 300}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.VWMASignal}))
		Expect(result.Value(aapl, signal.VWMASignal)).To(BeNumerically("~", 7600.0/600.0, 1e-10))
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		aaplCloses := []float64{10, 20}
		aaplVolumes := []float64{100, 100}
		msftCloses := []float64{50, 60}
		msftVolumes := []float64{300, 100}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{aaplCloses, aaplVolumes, msftCloses, msftVolumes}
		df, err := data.NewDataFrame(times, assets,
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		// AAPL: (10*100 + 20*100) / 200 = 15
		Expect(result.Value(aapl, signal.VWMASignal)).To(BeNumerically("~", 15.0, 1e-10))
		// MSFT: (50*300 + 60*100) / 400 = 21000/400 = 52.5
		Expect(result.Value(msft, signal.VWMASignal)).To(BeNumerically("~", 52.5, 1e-10))
	})

	It("returns NaN for zero total volume", func() {
		closes := []float64{10, 12, 14}
		volumes := []float64{0, 0, 0}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.VWMASignal))).To(BeTrue())
	})

	It("computes correctly with minimum data", func() {
		// Exactly 1 row -- the minimum for VWMA.
		closes := []float64{42}
		volumes := []float64{1000}

		times := []time.Time{now}
		vals := [][]float64{closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(1))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.VWMASignal)).To(BeNumerically("~", 42.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.VWMA(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "VWMA" ./signal/`
Expected: FAIL -- `signal.VWMA` and `signal.VWMASignal` are undefined.

- [ ] **Step 3: Write the implementation**

Create `signal/vwma.go`:

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

// VWMASignal is the output metric for [VWMA]. Values are in the same units
// as the input price, weighted by volume.
const VWMASignal data.Metric = "VWMA"

// VWMA computes the Volume-Weighted Moving Average for each asset in the
// universe over the given period. It weights each bar's close price by its
// volume, giving more influence to high-volume bars. Returns a single-row
// DataFrame with [VWMASignal].
func VWMA(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricClose, data.Volume)
	if err != nil {
		return data.WithErr(fmt.Errorf("VWMA: %w", err))
	}

	numRows := df.Len()
	if numRows < 1 {
		return data.WithErr(fmt.Errorf("VWMA: need at least 1 data point, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	vwmaCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		closes := df.Column(aa, data.MetricClose)
		volumes := df.Column(aa, data.Volume)

		sumCV := 0.0
		sumVol := 0.0

		for jj := range numRows {
			sumCV += closes[jj] * volumes[jj]
			sumVol += volumes[jj]
		}

		var vwma float64
		if sumVol == 0 {
			vwma = math.NaN()
		} else {
			vwma = sumCV / sumVol
		}

		vwmaCols[ii] = []float64{vwma}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{VWMASignal}, df.Frequency(), vwmaCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("VWMA: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "VWMA" ./signal/`
Expected: PASS (all 5 test cases)

- [ ] **Step 5: Commit**

```bash
git add signal/vwma.go signal/vwma_test.go
git commit -m "feat: add VWMA volume signal (#25)"
```

---

### Task 3: Accumulation/Distribution Signal

Cumulative money flow volume. Introduces the `moneyFlowVolumeSeries` helper that CMF will reuse.

**Files:**
- Create: `signal/accumulation_distribution.go`
- Create: `signal/accumulation_distribution_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/accumulation_distribution_test.go`:

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

var _ = Describe("AccumulationDistribution", func() {
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

	It("computes hand-calculated A/D correctly", func() {
		// Bar 0: H=12, L=8, C=10, V=1000
		//   MFM = ((10-8) - (12-10)) / (12-8) = (2-2)/4 = 0
		//   MFV = 0 * 1000 = 0. A/D = 0
		// Bar 1: H=15, L=10, C=14, V=2000
		//   MFM = ((14-10) - (15-14)) / (15-10) = (4-1)/5 = 0.6
		//   MFV = 0.6 * 2000 = 1200. A/D = 1200
		// Bar 2: H=14, L=11, C=12, V=1500
		//   MFM = ((12-11) - (14-12)) / (14-11) = (1-2)/3 = -1/3
		//   MFV = -1/3 * 1500 = -500. A/D = 1200 - 500 = 700
		highs := []float64{12, 15, 14}
		lows := []float64{8, 10, 11}
		closes := []float64{10, 14, 12}
		volumes := []float64{1000, 2000, 1500}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.AccumulationDistributionSignal}))
		Expect(result.Value(aapl, signal.AccumulationDistributionSignal)).To(BeNumerically("~", 700.0, 1e-10))
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		aaplHighs := []float64{12, 15}
		aaplLows := []float64{8, 10}
		aaplCloses := []float64{10, 14}
		aaplVolumes := []float64{1000, 2000}
		msftHighs := []float64{50, 55}
		msftLows := []float64{45, 48}
		msftCloses := []float64{46, 50}
		msftVolumes := []float64{3000, 4000}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{aaplHighs, aaplLows, aaplCloses, aaplVolumes,
			msftHighs, msftLows, msftCloses, msftVolumes}
		df, err := data.NewDataFrame(times, assets,
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())

		aaplAD := result.Value(aapl, signal.AccumulationDistributionSignal)
		msftAD := result.Value(msft, signal.AccumulationDistributionSignal)
		Expect(aaplAD).NotTo(Equal(msftAD))
	})

	It("yields MFM=0 when high equals low", func() {
		// H=10, L=10 => range=0, MFM=0, MFV=0
		highs := []float64{10, 10}
		lows := []float64{10, 10}
		closes := []float64{10, 10}
		volumes := []float64{500, 500}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.AccumulationDistributionSignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("computes correctly with minimum data", func() {
		// Exactly 2 rows -- the minimum for A/D.
		// Bar 0: H=10, L=8, C=9, V=1000 => MFM=(1-1)/2=0, MFV=0
		// Bar 1: H=12, L=8, C=11, V=2000 => MFM=(3-1)/4=0.5, MFV=1000
		// A/D = 0 + 1000 = 1000
		highs := []float64{10, 12}
		lows := []float64{8, 8}
		closes := []float64{9, 11}
		volumes := []float64{1000, 2000}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.AccumulationDistributionSignal)).To(BeNumerically("~", 1000.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.AccumulationDistribution(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "AccumulationDistribution" ./signal/`
Expected: FAIL -- `signal.AccumulationDistribution` and `signal.AccumulationDistributionSignal` are undefined.

- [ ] **Step 3: Write the implementation**

Create `signal/accumulation_distribution.go`:

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
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// AccumulationDistributionSignal is the output metric for
// [AccumulationDistribution]. Values are unbounded cumulative money flow
// volume; positive indicates accumulation, negative indicates distribution.
const AccumulationDistributionSignal data.Metric = "AccumulationDistribution"

// AccumulationDistribution computes the Accumulation/Distribution line for
// each asset in the universe over the given period. It uses the Money Flow
// Multiplier to weight each bar's volume by where the close falls within the
// high-low range. Returns a single-row DataFrame with
// [AccumulationDistributionSignal].
func AccumulationDistribution(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume)
	if err != nil {
		return data.WithErr(fmt.Errorf("AccumulationDistribution: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("AccumulationDistribution: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	adCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)
		volumes := df.Column(aa, data.Volume)

		mfv := moneyFlowVolumeSeries(highs, lows, closes, volumes)

		ad := 0.0
		for _, val := range mfv {
			ad += val
		}

		adCols[ii] = []float64{ad}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{AccumulationDistributionSignal}, df.Frequency(), adCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("AccumulationDistribution: %w", err))
	}

	return result
}

// moneyFlowVolumeSeries computes the Money Flow Volume for each bar.
// MFM = ((Close - Low) - (High - Close)) / (High - Low).
// MFV = MFM * Volume.
// When High == Low, MFM is 0 (no range to measure position within).
func moneyFlowVolumeSeries(highs, lows, closes, volumes []float64) []float64 {
	mfv := make([]float64, len(highs))
	for ii := range highs {
		rangeHL := highs[ii] - lows[ii]
		if rangeHL == 0 {
			mfv[ii] = 0
		} else {
			mfm := ((closes[ii] - lows[ii]) - (highs[ii] - closes[ii])) / rangeHL
			mfv[ii] = mfm * volumes[ii]
		}
	}

	return mfv
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "AccumulationDistribution" ./signal/`
Expected: PASS (all 5 test cases)

- [ ] **Step 5: Commit**

```bash
git add signal/accumulation_distribution.go signal/accumulation_distribution_test.go
git commit -m "feat: add Accumulation/Distribution volume signal (#25)"
```

---

### Task 4: CMF Signal

Chaikin Money Flow. Reuses the `moneyFlowVolumeSeries` helper from Task 3.

**Files:**
- Create: `signal/cmf.go`
- Create: `signal/cmf_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/cmf_test.go`:

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

var _ = Describe("CMF", func() {
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

	It("computes hand-calculated CMF correctly", func() {
		// Bar 0: H=12, L=8, C=10, V=1000
		//   MFM = ((10-8)-(12-10))/(12-8) = 0/4 = 0, MFV = 0
		// Bar 1: H=15, L=10, C=14, V=2000
		//   MFM = ((14-10)-(15-14))/(15-10) = 3/5 = 0.6, MFV = 1200
		// Bar 2: H=14, L=11, C=12, V=1500
		//   MFM = ((12-11)-(14-12))/(14-11) = -1/3, MFV = -500
		// CMF = Sum(MFV) / Sum(V) = (0 + 1200 - 500) / (1000+2000+1500)
		//     = 700 / 4500 = 0.15555...
		highs := []float64{12, 15, 14}
		lows := []float64{8, 10, 11}
		closes := []float64{10, 14, 12}
		volumes := []float64{1000, 2000, 1500}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.CMFSignal}))
		Expect(result.Value(aapl, signal.CMFSignal)).To(BeNumerically("~", 700.0/4500.0, 1e-10))
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		aaplHighs := []float64{12, 15}
		aaplLows := []float64{8, 10}
		aaplCloses := []float64{10, 14}
		aaplVolumes := []float64{1000, 2000}
		msftHighs := []float64{50, 55}
		msftLows := []float64{45, 48}
		msftCloses := []float64{46, 50}
		msftVolumes := []float64{3000, 4000}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{aaplHighs, aaplLows, aaplCloses, aaplVolumes,
			msftHighs, msftLows, msftCloses, msftVolumes}
		df, err := data.NewDataFrame(times, assets,
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())

		aaplCMF := result.Value(aapl, signal.CMFSignal)
		msftCMF := result.Value(msft, signal.CMFSignal)
		Expect(aaplCMF).NotTo(Equal(msftCMF))
	})

	It("returns NaN for zero total volume", func() {
		highs := []float64{12, 15}
		lows := []float64{8, 10}
		closes := []float64{10, 14}
		volumes := []float64{0, 0}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(math.IsNaN(result.Value(aapl, signal.CMFSignal))).To(BeTrue())
	})

	It("computes correctly with minimum data", func() {
		// Exactly 2 rows -- the minimum for CMF.
		// Bar 0: H=10, L=8, C=10, V=1000 => MFM=(2-0)/2=1.0, MFV=1000
		// Bar 1: H=12, L=8, C=8, V=2000 => MFM=(0-4)/4=-1.0, MFV=-2000
		// CMF = (1000-2000) / (1000+2000) = -1000/3000 = -1/3
		highs := []float64{10, 12}
		lows := []float64{8, 8}
		closes := []float64{10, 8}
		volumes := []float64{1000, 2000}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.CMFSignal)).To(BeNumerically("~", -1.0/3.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.CMF(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "CMF" ./signal/`
Expected: FAIL -- `signal.CMF` and `signal.CMFSignal` are undefined.

- [ ] **Step 3: Write the implementation**

Create `signal/cmf.go`:

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

// CMFSignal is the output metric for [CMF]. Values range from -1
// (maximum selling pressure) to +1 (maximum buying pressure).
const CMFSignal data.Metric = "CMF"

// CMF computes the Chaikin Money Flow for each asset in the universe over
// the given period. It measures the ratio of cumulative Money Flow Volume to
// cumulative Volume. Returns a single-row DataFrame with [CMFSignal].
func CMF(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume)
	if err != nil {
		return data.WithErr(fmt.Errorf("CMF: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("CMF: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	cmfCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)
		volumes := df.Column(aa, data.Volume)

		mfv := moneyFlowVolumeSeries(highs, lows, closes, volumes)

		sumMFV := 0.0
		sumVol := 0.0

		for jj := range numRows {
			sumMFV += mfv[jj]
			sumVol += volumes[jj]
		}

		var cmf float64
		if sumVol == 0 {
			cmf = math.NaN()
		} else {
			cmf = sumMFV / sumVol
		}

		cmfCols[ii] = []float64{cmf}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{CMFSignal}, df.Frequency(), cmfCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("CMF: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "CMF" ./signal/`
Expected: PASS (all 5 test cases)

- [ ] **Step 5: Commit**

```bash
git add signal/cmf.go signal/cmf_test.go
git commit -m "feat: add CMF volume signal (#25)"
```

---

### Task 5: MFI Signal

Money Flow Index. Needs one extra bar for TP comparison, similar to how Stochastic extends the period.

**Files:**
- Create: `signal/mfi.go`
- Create: `signal/mfi_test.go`

- [ ] **Step 1: Write the failing tests**

Create `signal/mfi_test.go`:

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

var _ = Describe("MFI", func() {
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

	It("computes hand-calculated MFI correctly", func() {
		// 4 bars total (period=3, need 1 extra for initial TP comparison)
		// Bar 0: H=12, L=8,  C=10, V=1000 => TP = 10.0
		// Bar 1: H=14, L=10, C=13, V=2000 => TP = 12.333..
		//   TP up => positive flow = 12.333.. * 2000 = 24666.67
		// Bar 2: H=13, L=9,  C=11, V=1500 => TP = 11.0
		//   TP down => negative flow = 11.0 * 1500 = 16500
		// Bar 3: H=15, L=11, C=14, V=1800 => TP = 13.333..
		//   TP up => positive flow += 13.333.. * 1800 = 24000
		// Positive sum = 24666.67 + 24000 = 48666.67
		// Negative sum = 16500
		// Ratio = 48666.67 / 16500 = 2.9495..
		// MFI = 100 - 100/(1 + 2.9495..) = 100 - 25.32.. = 74.68..
		highs := []float64{12, 14, 13, 15}
		lows := []float64{8, 10, 9, 11}
		closes := []float64{10, 13, 11, 14}
		volumes := []float64{1000, 2000, 1500, 1800}

		times := make([]time.Time, 4)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-3)
		}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(3))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.MFISignal}))

		tp0 := (12.0 + 8.0 + 10.0) / 3.0
		tp1 := (14.0 + 10.0 + 13.0) / 3.0
		tp2 := (13.0 + 9.0 + 11.0) / 3.0
		tp3 := (15.0 + 11.0 + 14.0) / 3.0
		posFlow := tp1*2000 + tp3*1800
		negFlow := tp2 * 1500
		ratio := posFlow / negFlow
		expected := 100.0 - 100.0/(1.0+ratio)
		Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", expected, 1e-6))
	})

	It("computes independently per asset", func() {
		msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		assets := []asset.Asset{aapl, msft}

		aaplHighs := []float64{12, 14, 15}
		aaplLows := []float64{8, 10, 11}
		aaplCloses := []float64{10, 13, 14}
		aaplVolumes := []float64{1000, 2000, 1800}
		msftHighs := []float64{50, 48, 52}
		msftLows := []float64{45, 44, 47}
		msftCloses := []float64{48, 45, 50}
		msftVolumes := []float64{3000, 4000, 3500}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{aaplHighs, aaplLows, aaplCloses, aaplVolumes,
			msftHighs, msftLows, msftCloses, msftVolumes}
		df, err := data.NewDataFrame(times, assets,
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource(assets, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())

		aaplMFI := result.Value(aapl, signal.MFISignal)
		msftMFI := result.Value(msft, signal.MFISignal)
		Expect(aaplMFI).NotTo(Equal(msftMFI))
	})

	It("returns MFI=100 when all flows are positive", func() {
		// Monotonically rising TP => all positive flows, no negative.
		highs := []float64{10, 12, 14}
		lows := []float64{8, 10, 12}
		closes := []float64{9, 11, 13}
		volumes := []float64{100, 200, 300}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", 100.0, 1e-10))
	})

	It("returns MFI=0 when all flows are negative", func() {
		// Monotonically falling TP => all negative flows, no positive.
		highs := []float64{14, 12, 10}
		lows := []float64{12, 10, 8}
		closes := []float64{13, 11, 9}
		volumes := []float64{300, 200, 100}

		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", 0.0, 1e-10))
	})

	It("computes correctly with minimum data", func() {
		// period=1, so adjustedPeriod = Days(2), fetches 2 bars -- the minimum.
		// Bar 0: H=10, L=8, C=9, V=1000 => TP=9.0
		// Bar 1: H=12, L=9, C=11, V=2000 => TP=10.667
		// TP rose => positive flow = 10.667 * 2000 = 21333.33
		// No negative flow => MFI = 100
		highs := []float64{10, 12}
		lows := []float64{8, 9}
		closes := []float64{9, 11}
		volumes := []float64{1000, 2000}

		times := []time.Time{now.AddDate(0, 0, -1), now}
		vals := [][]float64{highs, lows, closes, volumes}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl},
			[]data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(1))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.MFISignal)).To(BeNumerically("~", 100.0, 1e-10))
	})

	It("returns error on insufficient data", func() {
		df, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("data unavailable")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.MFI(ctx, uu, portfolio.Days(5))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("data unavailable"))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "MFI" ./signal/`
Expected: FAIL -- `signal.MFI` and `signal.MFISignal` are undefined.

- [ ] **Step 3: Write the implementation**

Create `signal/mfi.go`:

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
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// MFISignal is the output metric for [MFI]. Values range from 0 (maximum
// selling pressure) to 100 (maximum buying pressure).
const MFISignal data.Metric = "MFI"

// MFI computes the Money Flow Index for each asset in the universe over the
// given period. It classifies each bar's money flow as positive or negative
// based on whether the typical price rose or fell, then computes a ratio
// analogous to RSI. Returns a single-row DataFrame with [MFISignal].
func MFI(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	// Fetch one extra bar for the initial TP comparison.
	adjustedPeriod := portfolio.Days(period.N + 1)

	df, err := assetUniverse.Window(ctx, adjustedPeriod, data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume)
	if err != nil {
		return data.WithErr(fmt.Errorf("MFI: %w", err))
	}

	numRows := df.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("MFI: need at least 2 data points, got %d", numRows))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	mfiCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		highs := df.Column(aa, data.MetricHigh)
		lows := df.Column(aa, data.MetricLow)
		closes := df.Column(aa, data.MetricClose)
		volumes := df.Column(aa, data.Volume)

		// Compute typical prices.
		tp := make([]float64, numRows)
		for jj := range numRows {
			tp[jj] = (highs[jj] + lows[jj] + closes[jj]) / 3.0
		}

		// Classify money flows.
		posFlow := 0.0
		negFlow := 0.0

		for jj := 1; jj < numRows; jj++ {
			rawMF := tp[jj] * volumes[jj]
			if tp[jj] > tp[jj-1] {
				posFlow += rawMF
			} else if tp[jj] < tp[jj-1] {
				negFlow += rawMF
			}
			// Equal TPs are discarded.
		}

		var mfi float64
		if negFlow == 0 {
			mfi = 100.0
		} else if posFlow == 0 {
			mfi = 0.0
		} else {
			ratio := posFlow / negFlow
			mfi = 100.0 - 100.0/(1.0+ratio)
		}

		mfiCols[ii] = []float64{mfi}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{MFISignal}, df.Frequency(), mfiCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("MFI: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "MFI" ./signal/`
Expected: PASS (all 7 test cases)

- [ ] **Step 5: Commit**

```bash
git add signal/mfi.go signal/mfi_test.go
git commit -m "feat: add MFI volume signal (#25)"
```

---

### Task 6: Documentation Updates

Update doc.go, signals.md, and the changelog.

**Files:**
- Modify: `signal/doc.go`
- Modify: `docs/signals.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update `signal/doc.go`**

Add volume signals to the Built-in Signals list, after the CCI entry (line 40):

```go
//   - [OBV](ctx, u, period): On-Balance Volume (cumulative).
//   - [VWMA](ctx, u, period): Volume-Weighted Moving Average.
//   - [AccumulationDistribution](ctx, u, period): Accumulation/Distribution line (cumulative).
//   - [CMF](ctx, u, period): Chaikin Money Flow (-1 to 1).
//   - [MFI](ctx, u, period): Money Flow Index (0 to 100).
```

- [ ] **Step 2: Update `docs/signals.md` overview table**

Add the five volume signals to the "Built-in signals" table (after the CCI row):

```markdown
| `OBV` | `(ctx, u, period)` | On-Balance Volume (cumulative) |
| `VWMA` | `(ctx, u, period)` | Volume-Weighted Moving Average |
| `AccumulationDistribution` | `(ctx, u, period)` | Accumulation/Distribution line (cumulative) |
| `CMF` | `(ctx, u, period)` | Chaikin Money Flow (-1 to 1) |
| `MFI` | `(ctx, u, period)` | Money Flow Index (0 to 100) |
```

- [ ] **Step 3: Add Volume section to `docs/signals.md`**

Insert a new "### Volume" section after the Oscillators section (before "### Fundamental"), following the same format (description, code example, signature, parameters, output metric, value range, `---` separator) for each of the five signals:

**OBV:**
```markdown
#### OBV

Computes On-Balance Volume, a cumulative indicator that adds volume on up-close bars and subtracts volume on down-close bars. Rising OBV confirms an uptrend; divergence between OBV and price warns of potential reversal.

\`\`\`go
df := signal.OBV(ctx, u, portfolio.Days(50))
\`\`\`

**Signature:** `OBV(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window for cumulative OBV

**Output metric:** `OBVSignal`

**Value range:** Unbounded. Positive values indicate net buying pressure; negative indicate net selling pressure.
```

**VWMA:**
```markdown
#### VWMA

Computes the Volume-Weighted Moving Average. Unlike a simple moving average, VWMA gives more weight to bars with higher volume. When price is above VWMA, buying pressure dominates; when below, selling pressure dominates.

\`\`\`go
df := signal.VWMA(ctx, u, portfolio.Days(20))
\`\`\`

**Signature:** `VWMA(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window for the weighted average

**Output metric:** `VWMASignal`

**Value range:** Same units as the input price.
```

**AccumulationDistribution:**
```markdown
#### AccumulationDistribution

Computes the Accumulation/Distribution line, a cumulative volume indicator that uses the relationship of close to the high-low range to weight volume. When close is near the high, most of the volume is classified as accumulation; when near the low, as distribution.

\`\`\`go
df := signal.AccumulationDistribution(ctx, u, portfolio.Days(50))
\`\`\`

**Signature:** `AccumulationDistribution(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window for cumulative A/D

**Output metric:** `AccumulationDistributionSignal`

**Value range:** Unbounded. Positive values indicate net accumulation; negative indicate net distribution.
```

**CMF:**
```markdown
#### CMF

Computes Chaikin Money Flow, the ratio of cumulative Money Flow Volume to cumulative Volume over the period. It measures buying and selling pressure over a fixed window rather than cumulatively.

\`\`\`go
df := signal.CMF(ctx, u, portfolio.Days(21))
\`\`\`

**Signature:** `CMF(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window (conventionally 20 or 21 bars)

**Output metric:** `CMFSignal`

**Value range:** -1 to +1. Positive values indicate buying pressure; negative indicate selling pressure.
```

**MFI:**
```markdown
#### MFI

Computes the Money Flow Index, a volume-weighted RSI. It classifies each bar's money flow as positive or negative based on whether the typical price rose or fell, then computes a bounded oscillator.

\`\`\`go
df := signal.MFI(ctx, u, portfolio.Days(14))
\`\`\`

**Signature:** `MFI(ctx context.Context, u universe.Universe, period portfolio.Period) *data.DataFrame`

**Parameters:**
- `period` — lookback window (conventionally 14 bars)

**Output metric:** `MFISignal`

**Value range:** 0 to 100. Values above 80 are conventionally overbought; values below 20 are oversold.
```

- [ ] **Step 4: Update `CHANGELOG.md`**

Add under `[Unreleased] > Added`:

```markdown
- Volume signals (OBV, VWMA, Accumulation/Distribution, Chaikin Money Flow, and Money Flow Index) confirm price moves and detect accumulation/distribution patterns.
```

- [ ] **Step 5: Run full signal test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/`
Expected: All tests PASS.

- [ ] **Step 6: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && make lint`
Expected: No lint errors.

- [ ] **Step 7: Commit**

```bash
git add signal/doc.go docs/signals.md CHANGELOG.md
git commit -m "docs: add volume signals to reference and changelog (#25)"
```
