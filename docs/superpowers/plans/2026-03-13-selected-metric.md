# Selected Metric Column Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix two portfolio selection bugs by adding a `Selected` metric column to the DataFrame instead of filtering assets.

**Architecture:** `Select` mutates the DataFrame in place, adding a `Selected` column (1.0/0.0 per asset per timestep). Weighting functions read `Selected` per-timestep to decide which assets get weight. Fallback assets are inserted into the DataFrame by individual Selector implementations.

**Tech Stack:** Go, Ginkgo v2/Gomega testing, column-major DataFrame

**Spec:** `docs/superpowers/specs/2026-03-13-selected-metric-design.md`

---

## Chunk 1: Selected constant and EqualWeight

### Task 1: Add Selected constant

**Files:**
- Create: `portfolio/selected.go`

- [ ] **Step 1: Create the Selected constant**

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

package portfolio

import "github.com/penny-vault/pvbt/data"

// Selected is the metric key used by Selectors to mark which assets
// are chosen at each timestep. Values > 0 mean selected; 0 or NaN
// means not selected. Supports fractional values for future selectors.
const Selected data.Metric = "selected"
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./portfolio/...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add portfolio/selected.go
git commit -m "feat(portfolio): add Selected metric constant"
```

### Task 2: Update EqualWeight to read Selected column

**Files:**
- Modify: `portfolio/equal_weight.go`
- Modify: `portfolio/weighting_test.go`

- [ ] **Step 1: Write failing tests for new EqualWeight behavior**

Add new test cases to `portfolio/weighting_test.go`. These tests use the `Selected` column and expect `(PortfolioPlan, error)` return. Add them after the existing `EqualWeight edge cases` Describe block:

```go
var _ = Describe("EqualWeight with Selected column", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		bil  asset.Asset
		t1   time.Time
		t2   time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
		t1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		t2 = time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
	})

	It("returns error when Selected column is missing", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = portfolio.EqualWeight(df)
		Expect(err).To(HaveOccurred())
	})

	It("assigns equal weight only to selected assets at each timestep", func() {
		// 3 assets, 2 timesteps. SPY selected at t1, AAPL selected at t2.
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MetricClose},
			[]float64{
				100, 101, // SPY
				200, 201, // AAPL
				50, 51,   // BIL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		// Insert Selected columns: SPY=1 at t1, 0 at t2
		Expect(df.Insert(spy, portfolio.Selected, []float64{1, 0})).To(Succeed())
		// AAPL=0 at t1, 1 at t2
		Expect(df.Insert(aapl, portfolio.Selected, []float64{0, 1})).To(Succeed())
		// BIL=0 at both
		Expect(df.Insert(bil, portfolio.Selected, []float64{0, 0})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(2))

		// t1: only SPY selected => weight 1.0
		Expect(plan[0].Date).To(Equal(t1))
		Expect(plan[0].Members).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))

		// t2: only AAPL selected => weight 1.0
		Expect(plan[1].Date).To(Equal(t2))
		Expect(plan[1].Members).To(HaveLen(1))
		Expect(plan[1].Members[aapl]).To(Equal(1.0))
	})

	It("assigns equal weight when multiple assets selected at same timestep", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MetricClose},
			[]float64{100, 200, 50},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(bil, portfolio.Selected, []float64{0})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members).To(HaveLen(2))
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("treats fractional Selected > 0 as selected", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{100, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{0.5})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1.0})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		// Both selected (magnitude ignored), equal weight
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("treats NaN in Selected column as not selected", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{100, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1.0})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{math.NaN()})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))
	})

	It("produces empty members when no assets are selected at a timestep", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{100, 200},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{0})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{0})).To(Succeed())

		plan, err := portfolio.EqualWeight(df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members).To(HaveLen(0))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "EqualWeight with Selected" -v`
Expected: compilation errors (EqualWeight returns one value, tests expect two)

- [ ] **Step 3: Update EqualWeight implementation**

Replace the entire `EqualWeight` function body in `portfolio/equal_weight.go`:

```go
package portfolio

import (
	"fmt"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// EqualWeight builds a PortfolioPlan from a DataFrame by assigning equal
// weights to all selected assets at each timestep. It reads the Selected
// metric column to determine which assets are chosen. Any asset with
// Selected > 0 at a given timestep receives equal weight; magnitude is
// ignored. Returns an error if the Selected column is absent.
func EqualWeight(df *data.DataFrame) (PortfolioPlan, error) {
	times := df.Times()
	assets := df.AssetList()

	// Verify Selected column exists.
	hasSelected := false
	for _, m := range df.MetricList() {
		if m == Selected {
			hasSelected = true
			break
		}
	}
	if !hasSelected {
		return nil, fmt.Errorf("EqualWeight: DataFrame missing %q column", Selected)
	}

	plan := make(PortfolioPlan, len(times))
	for i, t := range times {
		// Collect selected assets at this timestep.
		var chosen []asset.Asset
		for _, a := range assets {
			v := df.ValueAt(a, Selected, t)
			if v > 0 {
				chosen = append(chosen, a)
			}
		}

		members := make(map[asset.Asset]float64, len(chosen))
		if len(chosen) > 0 {
			w := 1.0 / float64(len(chosen))
			for _, a := range chosen {
				members[a] = w
			}
		}
		plan[i] = Allocation{Date: t, Members: members}
	}

	return plan, nil
}
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "EqualWeight with Selected" -v`
Expected: PASS

- [ ] **Step 5: Update existing EqualWeight tests to match new signature**

The existing tests in the `EqualWeight` and `EqualWeight edge cases` Describe blocks call `portfolio.EqualWeight(df)` and assign to a single variable. Update each call to use two return values and assert no error. For example:

Change:
```go
plan := portfolio.EqualWeight(df)
```
To:
```go
plan, err := portfolio.EqualWeight(df)
Expect(err).NotTo(HaveOccurred())
```

Do this for every call in the existing `EqualWeight` and `EqualWeight edge cases` Describe blocks.

The existing tests also need `Selected` columns inserted before calling `EqualWeight`. For each existing test, insert `Selected=1.0` for every asset at every timestep so the existing behavior is preserved. For example, in the "assigns 1/N to all assets" test:

```go
Expect(df.Insert(spy, portfolio.Selected, []float64{1, 1})).To(Succeed())
Expect(df.Insert(aapl, portfolio.Selected, []float64{1, 1})).To(Succeed())
```

The "returns empty plan for a DataFrame with zero timestamps" test also needs `Selected`. Since there are no timestamps, insert an empty slice:

```go
Expect(df.Insert(spy, portfolio.Selected, []float64{})).To(Succeed())
```

The "returns allocations with empty members for a DataFrame with zero assets" test needs the `Selected` metric added to the DataFrame construction since there are no assets to insert on:

```go
df, err := data.NewDataFrame(
    []time.Time{t1},
    nil,
    []data.Metric{data.MetricClose, portfolio.Selected},
    nil,
)
```

Note: the new tests use `math.NaN()`, so ensure `"math"` is in the import block for `weighting_test.go` (it is already imported in the existing file).

- [ ] **Step 6: Run all EqualWeight tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "EqualWeight" -v`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add portfolio/equal_weight.go portfolio/weighting_test.go
git commit -m "feat(portfolio): EqualWeight reads Selected column per-timestep"
```

## Chunk 2: WeightedBySignal

### Task 3: Update WeightedBySignal to read Selected column

**Files:**
- Modify: `portfolio/weighted_by_signal.go`
- Modify: `portfolio/weighting_test.go`

- [ ] **Step 1: Write failing tests for new WeightedBySignal behavior**

Add new test cases to `portfolio/weighting_test.go`:

```go
var _ = Describe("WeightedBySignal with Selected column", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		bil  asset.Asset
		t1   time.Time
		t2   time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
		t1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		t2 = time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
	})

	It("returns error when Selected column is missing", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MarketCap},
			[]float64{300},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).To(HaveOccurred())
	})

	It("weights only selected assets by signal", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MarketCap},
			[]float64{300, 100, 500},
		)
		Expect(err).NotTo(HaveOccurred())

		// Only SPY and AAPL selected; BIL not selected.
		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(bil, portfolio.Selected, []float64{0})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))

		// SPY=300/(300+100)=0.75, AAPL=100/400=0.25, BIL excluded
		Expect(plan[0].Members).To(HaveLen(2))
		Expect(plan[0].Members[spy]).To(Equal(0.75))
		Expect(plan[0].Members[aapl]).To(Equal(0.25))
	})

	It("uses per-timestep selection for weighting", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			[]float64{
				300, 100, // SPY
				100, 300, // AAPL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		// SPY selected at t1 only, AAPL selected at t2 only
		Expect(df.Insert(spy, portfolio.Selected, []float64{1, 0})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{0, 1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(2))

		// t1: only SPY selected => weight 1.0
		Expect(plan[0].Members).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))

		// t2: only AAPL selected => weight 1.0
		Expect(plan[1].Members).To(HaveLen(1))
		Expect(plan[1].Members[aapl]).To(Equal(1.0))
	})

	It("falls back to equal weight among selected when all signal values are zero", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl, bil},
			[]data.Metric{data.MarketCap},
			[]float64{0, 0, 500},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(bil, portfolio.Selected, []float64{0})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))

		// Equal weight among selected (SPY, AAPL), not all assets
		Expect(plan[0].Members).To(HaveLen(2))
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("discards zero signal values in normalization", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			[]float64{300, 0},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))

		// SPY=300/300=1.0, AAPL discarded (zero signal, omitted from map)
		Expect(plan[0].Members).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "WeightedBySignal with Selected" -v`
Expected: compilation errors (WeightedBySignal returns one value, tests expect two)

- [ ] **Step 3: Update WeightedBySignal implementation**

Replace the entire `WeightedBySignal` function body in `portfolio/weighted_by_signal.go`:

```go
package portfolio

import (
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// WeightedBySignal builds a PortfolioPlan from a DataFrame by weighting
// each selected asset proportionally to the values in the named metric
// column. It reads the Selected metric column to determine which assets
// are chosen at each timestep. Any asset with Selected > 0 is included;
// magnitude is ignored. Weights are normalized to sum to 1.0.
//
// Zero, NaN, and negative metric values are discarded. If all selected
// assets have non-positive metric values at a timestep, equal weight is
// assigned among the selected assets. Returns an error if the Selected
// column is absent.
func WeightedBySignal(df *data.DataFrame, metric data.Metric) (PortfolioPlan, error) {
	times := df.Times()
	assets := df.AssetList()

	// Verify Selected column exists.
	hasSelected := false
	for _, m := range df.MetricList() {
		if m == Selected {
			hasSelected = true
			break
		}
	}
	if !hasSelected {
		return nil, fmt.Errorf("WeightedBySignal: DataFrame missing %q column", Selected)
	}

	plan := make(PortfolioPlan, len(times))

	for i, t := range times {
		// Collect selected assets and their signal values.
		var chosen []asset.Asset
		var values []float64
		sum := 0.0

		for _, a := range assets {
			sel := df.ValueAt(a, Selected, t)
			if sel <= 0 || math.IsNaN(sel) {
				continue
			}
			chosen = append(chosen, a)

			v := df.ValueAt(a, metric, t)
			if math.IsNaN(v) || v <= 0 {
				values = append(values, 0)
			} else {
				values = append(values, v)
				sum += v
			}
		}

		members := make(map[asset.Asset]float64, len(chosen))

		if sum == 0 && len(chosen) > 0 {
			// Fall back to equal weight among selected assets.
			w := 1.0 / float64(len(chosen))
			for _, a := range chosen {
				members[a] = w
			}
		} else {
			for j, a := range chosen {
				w := values[j] / sum
				if w > 0 {
					members[a] = w
				}
			}
		}

		plan[i] = Allocation{Date: t, Members: members}
	}

	return plan, nil
}
```

- [ ] **Step 4: Run new tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "WeightedBySignal with Selected" -v`
Expected: PASS

- [ ] **Step 5: Update existing WeightedBySignal tests to match new signature**

Update every call in the existing `WeightedBySignal` Describe block from:
```go
plan := portfolio.WeightedBySignal(df, data.MarketCap)
```
To:
```go
plan, err := portfolio.WeightedBySignal(df, data.MarketCap)
Expect(err).NotTo(HaveOccurred())
```

Also insert `Selected=1.0` for all assets at all timesteps in each existing test so the old behavior is preserved. For example, in "weights proportionally by signal":
```go
Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())
```

For multi-timestep tests (e.g., "computes weights independently at each timestep"), use the correct length:
```go
Expect(df.Insert(spy, portfolio.Selected, []float64{1, 1})).To(Succeed())
Expect(df.Insert(aapl, portfolio.Selected, []float64{1, 1})).To(Succeed())
```

**Behavioral change:** The new implementation discards zero metric values (the old code included them). This affects two existing tests:

1. "skips NaN values and weights positive values proportionally" -- currently expects `plan[0].Members[aapl]` to equal 0.0, but aapl will now be omitted from the map entirely since its weight is 0. Update assertion to check `Expect(plan[0].Members).To(HaveLen(1))` instead.

2. "ignores negative values and weights positive values proportionally" -- currently expects `plan[0].Members[aapl]` to equal 0.0 (negative signal). Update similarly: aapl will be omitted from the map. Check `HaveLen(2)` (SPY and BIL only).

- [ ] **Step 6: Run all WeightedBySignal tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "WeightedBySignal" -v`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add portfolio/weighted_by_signal.go portfolio/weighting_test.go
git commit -m "feat(portfolio): WeightedBySignal reads Selected column per-timestep"
```

## Chunk 3: MaxAboveZero

### Task 4: Update MaxAboveZero to write Selected column

**Files:**
- Modify: `portfolio/max_above_zero.go`
- Modify: `portfolio/selector_test.go`
- Modify: `portfolio/testutil_test.go` (update compile-time check)

- [ ] **Step 1: Write failing tests for new MaxAboveZero behavior**

Replace the existing `MaxAboveZero` Describe block in `portfolio/selector_test.go` with tests that verify the new behavior. The key differences from the old tests:

1. Constructor now takes `(metric, fallbackDF)` instead of `([]asset.Asset)`
2. Result DataFrame contains all original assets plus fallback assets (not filtered)
3. Result DataFrame has a `Selected` column with per-timestep values
4. Fallback assets are inserted into the DataFrame when nothing qualifies

```go
var _ = Describe("MaxAboveZero", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		bil  asset.Asset

		t1 time.Time
		t2 time.Time
		t3 time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL001", Ticker: "AAPL"}
		bil = asset.Asset{CompositeFigi: "BIL001", Ticker: "BIL"}

		t1 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		t2 = time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
		t3 = time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)
	})

	It("marks the asset with the highest positive value as selected", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2, t3},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				5, 3, 8, // SPY
				2, 1, 4, // AAPL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		// Both assets remain in the DataFrame.
		Expect(result.AssetList()).To(HaveLen(2))

		// SPY selected at all timesteps (highest positive).
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t2)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t3)).To(Equal(1.0))

		// AAPL not selected at any timestep.
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t2)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t3)).To(Equal(0.0))
	})

	It("inserts fallback assets when no asset is above zero", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{-1, -2},
		)
		Expect(err).NotTo(HaveOccurred())

		// Fallback DataFrame with BIL.
		fbDF, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			[]float64{90},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		// BIL is now in the DataFrame.
		Expect(result.AssetList()).To(HaveLen(3))

		// BIL selected, SPY and AAPL not.
		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))

		// BIL's price data was inserted.
		Expect(result.ValueAt(bil, data.MetricClose, t1)).To(Equal(90.0))
	})

	It("handles leadership changes across timesteps", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				10, 1, // SPY
				5, 20, // AAPL
			},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		// SPY selected at t1, AAPL selected at t2.
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t2)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t2)).To(Equal(1.0))
	})

	It("uses fallback when all values are NaN", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{math.NaN(), math.NaN()},
		)
		Expect(err).NotTo(HaveOccurred())

		fbDF, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			[]float64{90},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("selects first asset when all values are equal positive", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{5, 5},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		// SPY wins (strict >), AAPL not selected.
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("selects no assets with nil fallback when none are positive", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{-1, 0},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("uses fallback at some timesteps and regular selection at others", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{
				10, -5, // SPY: positive at t1, negative at t2
				5, -3,  // AAPL: positive at t1, negative at t2
			},
		)
		Expect(err).NotTo(HaveOccurred())

		fbDF, err := data.NewDataFrame(
			[]time.Time{t1, t2},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			[]float64{90, 91},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		// t1: SPY wins (highest positive)
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(0.0))

		// t2: all negative, fallback to BIL
		Expect(result.ValueAt(spy, portfolio.Selected, t2)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t2)).To(Equal(0.0))
		Expect(result.ValueAt(bil, portfolio.Selected, t2)).To(Equal(1.0))
	})

	It("trivially selects a single asset with positive value", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			[]float64{42},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
	})

	It("treats +Inf as above zero and selects it", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{math.Inf(1), 5},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("treats -Inf as not above zero and falls back", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{math.Inf(-1), math.Inf(-1)},
		)
		Expect(err).NotTo(HaveOccurred())

		fbDF, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			[]float64{90},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("selects the positive asset when mixed with NaN at the same timestep", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			[]float64{10, math.NaN()},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(aapl, portfolio.Selected, t1)).To(Equal(0.0))
	})

	It("handles fallback asset that overlaps with input asset", func() {
		// BIL is in both the input DataFrame and the fallback DataFrame.
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, bil},
			[]data.Metric{data.MetricClose},
			[]float64{-1, 80},
		)
		Expect(err).NotTo(HaveOccurred())

		fbDF, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{bil},
			[]data.Metric{data.MetricClose},
			[]float64{90},
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, fbDF)
		result := sel.Select(df)

		// BIL selected via fallback (SPY negative, BIL's input value 80 is
		// not evaluated because BIL is also in fallback and its metric data
		// gets overwritten by Insert).
		Expect(result.ValueAt(bil, portfolio.Selected, t1)).To(Equal(1.0))
		Expect(result.ValueAt(spy, portfolio.Selected, t1)).To(Equal(0.0))
		// BIL's close price is overwritten by fallback data.
		Expect(result.ValueAt(bil, data.MetricClose, t1)).To(Equal(90.0))
	})

	It("returns empty DataFrame with Selected for zero timestamps", func() {
		df, err := data.NewDataFrame(
			nil,
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			nil,
		)
		Expect(err).NotTo(HaveOccurred())

		sel := portfolio.MaxAboveZero(data.MetricClose, nil)
		result := sel.Select(df)

		Expect(result.Times()).To(HaveLen(0))
	})
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "MaxAboveZero" -v`
Expected: compilation errors (MaxAboveZero signature changed)

- [ ] **Step 3: Update compile-time interface check in testutil_test.go**

Change line 16 in `portfolio/testutil_test.go` from:
```go
var _ portfolio.Selector = portfolio.MaxAboveZero(nil)
```
To:
```go
var _ portfolio.Selector = portfolio.MaxAboveZero(data.MetricClose, nil)
```

- [ ] **Step 4: Update MaxAboveZero implementation**

Replace the entire content of `portfolio/max_above_zero.go`:

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

package portfolio

import (
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/rs/zerolog/log"
)

type maxAboveZero struct {
	metric   data.Metric
	fallback *data.DataFrame
}

// Select marks the asset with the highest positive value in the
// configured metric as selected at each timestep. If no asset has a
// positive value, fallback assets are inserted into the DataFrame and
// marked as selected. The DataFrame is mutated in place.
//
// Insert errors are logged but not returned because the Selector
// interface does not support error returns. A mismatched fallback
// DataFrame (e.g., different timestamps) will produce log warnings.
func (m maxAboveZero) Select(df *data.DataFrame) *data.DataFrame {
	times := df.Times()
	assets := df.AssetList()
	T := len(times)

	// Build Selected column per asset.
	selCols := make(map[string][]float64)
	for _, a := range assets {
		selCols[a.CompositeFigi] = make([]float64, T)
	}

	// Track which fallback assets need to be inserted.
	needsFallback := false
	var fbAssets []asset.Asset
	fbSelCols := make(map[string][]float64)
	if m.fallback != nil {
		fbAssets = m.fallback.AssetList()
		for _, a := range fbAssets {
			fbSelCols[a.CompositeFigi] = make([]float64, T)
		}
	}

	for ti, t := range times {
		bestVal := math.Inf(-1)
		var bestFigi string

		for _, a := range assets {
			v := df.ValueAt(a, m.metric, t)
			if math.IsNaN(v) {
				continue
			}
			if v > 0 && v > bestVal {
				bestVal = v
				bestFigi = a.CompositeFigi
			}
		}

		if bestFigi != "" {
			selCols[bestFigi][ti] = 1.0
		} else if m.fallback != nil {
			needsFallback = true
			for _, a := range fbAssets {
				fbSelCols[a.CompositeFigi][ti] = 1.0
			}
		}
	}

	// Insert fallback asset data into the DataFrame.
	if needsFallback {
		fbMetrics := m.fallback.MetricList()
		for _, a := range fbAssets {
			for _, met := range fbMetrics {
				vals := m.fallback.Column(a, met)
				if err := df.Insert(a, met, vals); err != nil {
					log.Warn().Err(err).
						Str("asset", a.CompositeFigi).
						Str("metric", string(met)).
						Msg("MaxAboveZero: failed to insert fallback data")
				}
			}
		}
	}

	// Write Selected columns for original assets.
	for _, a := range assets {
		if err := df.Insert(a, Selected, selCols[a.CompositeFigi]); err != nil {
			log.Warn().Err(err).
				Str("asset", a.CompositeFigi).
				Msg("MaxAboveZero: failed to insert Selected column")
		}
	}

	// Write Selected columns for fallback assets.
	for _, a := range fbAssets {
		if err := df.Insert(a, Selected, fbSelCols[a.CompositeFigi]); err != nil {
			log.Warn().Err(err).
				Str("asset", a.CompositeFigi).
				Msg("MaxAboveZero: failed to insert fallback Selected column")
		}
	}

	return df
}

// MaxAboveZero returns a Selector that picks the asset with the highest
// value above zero in the given metric column. If no assets qualify at
// a timestep, the fallback DataFrame's assets are inserted and marked
// as selected. Pass nil for fallback if no fallback is needed.
func MaxAboveZero(metric data.Metric, fallback *data.DataFrame) Selector {
	return maxAboveZero{metric: metric, fallback: fallback}
}
```

- [ ] **Step 5: Run MaxAboveZero tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -run "MaxAboveZero" -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add portfolio/max_above_zero.go portfolio/selector_test.go portfolio/testutil_test.go
git commit -m "feat(portfolio): MaxAboveZero writes Selected column, inserts fallback assets"
```

## Chunk 4: Selector interface doc and example update

### Task 5: Update Selector interface documentation

**Files:**
- Modify: `portfolio/selector.go`

- [ ] **Step 1: Update the Selector doc comment**

Replace the comment in `portfolio/selector.go`:

```go
// Selector marks which assets should be held at each timestep by
// inserting a Selected metric column into the DataFrame. Values > 0
// mean the asset is selected at that timestep; 0 means it is not.
// Select mutates the DataFrame in place and returns the same pointer.
type Selector interface {
	Select(df *data.DataFrame) *data.DataFrame
}
```

- [ ] **Step 2: Commit**

```bash
git add portfolio/selector.go
git commit -m "docs(portfolio): update Selector interface doc for Selected column"
```

### Task 6: Update momentum-rotation example

**Files:**
- Modify: `examples/momentum-rotation/main.go`

- [ ] **Step 1: Update the Compute method**

Change the Compute method in `examples/momentum-rotation/main.go` to use the new API:

```go
func (s *MomentumRotation) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {
	log := zerolog.Ctx(ctx)

	// Fetch close prices for the lookback period.
	df, err := s.RiskOn.Window(ctx, portfolio.Months(s.Lookback), data.MetricClose)
	if err != nil {
		log.Error().Err(err).Msg("Window fetch failed")
		return
	}
	if df.Len() < 2 {
		return
	}

	// Compute total return over the full window, take the last row.
	momentum := df.Pct(df.Len() - 1).Last()

	// Build fallback DataFrame from the risk-off universe.
	riskOffDF, err := s.RiskOff.Window(ctx, portfolio.Months(s.Lookback), data.MetricClose)
	if err != nil {
		log.Error().Err(err).Msg("RiskOff window fetch failed")
		return
	}
	riskOffMomentum := riskOffDF.Pct(riskOffDF.Len() - 1).Last()

	// Select the asset with the highest positive return; fall back to risk-off.
	portfolio.MaxAboveZero(data.MetricClose, riskOffMomentum).Select(momentum)
	plan, err := portfolio.EqualWeight(momentum)
	if err != nil {
		log.Error().Err(err).Msg("equal weight failed")
		return
	}

	if err := p.RebalanceTo(ctx, plan...); err != nil {
		log.Error().Err(err).Msg("rebalance failed")
	}
}
```

Note: `Pct()` transforms values in-place under the same metric key (`MetricClose`), so passing `data.MetricClose` to `MaxAboveZero` is correct. Verify this by checking that `momentum.MetricList()` still contains `MetricClose` after the Pct/Last chain.

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go build ./examples/momentum-rotation/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add examples/momentum-rotation/main.go
git commit -m "refactor(examples): update momentum-rotation for Selected column API"
```

### Task 7: Run full test suite

- [ ] **Step 1: Run all portfolio tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./portfolio/ -v`
Expected: all PASS

- [ ] **Step 2: Run full project tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt2 && go test ./...`
Expected: all PASS (fix any compilation errors from callers of the old API)

- [ ] **Step 3: Fix any remaining callers**

Search the codebase for other callers of `MaxAboveZero`, `EqualWeight`, or `WeightedBySignal` that use the old signatures:

```bash
grep -rn 'MaxAboveZero\|EqualWeight\|WeightedBySignal' --include='*.go' .
```

Update any remaining callers to match the new signatures. Commit each fix.
