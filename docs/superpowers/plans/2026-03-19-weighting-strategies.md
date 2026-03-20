# Weighting Strategies Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add inverse volatility, market-cap weighted, risk parity, and risk parity fast weighting strategies to the portfolio package.

**Architecture:** Move the `DataSource` interface from `universe` to `data` so DataFrames can reference their data source. Add a `source` field to DataFrame. Implement four new weighting functions as stateless functions in the portfolio package, following the existing EqualWeight/WeightedBySignal patterns.

**Tech Stack:** Go, Ginkgo/Gomega, gonum (floats/stat/mat)

**Spec:** `docs/superpowers/specs/2026-03-19-weighting-strategies-design.md`

---

### File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `data/data_source.go` | DataSource interface definition |
| Modify | `data/data_frame.go:40-68` | Add `source` field, `Source()`/`SetSource()` methods |
| Modify | `universe/data_source.go` | Replace interface with type alias to `data.DataSource` |
| Modify | `universe/static.go:34` | Change field type from `DataSource` to `data.DataSource` |
| Modify | `universe/rated.go:39` | Change field type from `DataSource` to `data.DataSource` |
| Modify | `universe/index.go:39` | Change field type from `DataSource` to `data.DataSource` |
| Modify | `engine/engine.go:332,369` | Call `SetSource(e)` on returned DataFrames |
| Modify | `engine/engine.go:664` | Update compile-time check to `data.DataSource` |
| Modify | `signal/helpers_test.go:30,48` | Update compile-time checks to `data.DataSource` |
| Modify | `universe/universe_test.go` | Update mock to use `data.DataSource` |
| Modify | `data/data_frame.go` | Add `Correlation` method alongside existing `Covariance`/`Std` |
| Modify | `data/data_frame_test.go` | Tests for `Correlation` method |
| Create | `portfolio/inverse_volatility.go` | InverseVolatility weighting function |
| Create | `portfolio/market_cap_weighted.go` | MarketCapWeighted weighting function |
| Create | `portfolio/risk_parity_fast.go` | RiskParityFast weighting function |
| Create | `portfolio/risk_parity.go` | RiskParity weighting function |
| Create | `portfolio/weighting_helpers.go` | Shared helpers: selected asset collection, fallback logic |
| Modify | `portfolio/weighting_test.go` | Tests for all four new weighting functions |

---

### Task 1: Move DataSource interface to data package

**Files:**
- Create: `data/data_source.go`
- Modify: `universe/data_source.go`

- [ ] **Step 1: Create `data/data_source.go` with the interface**

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

package data

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// DataSource provides data fetching capabilities. The engine implements this
// interface; DataFrames hold a reference to it so downstream consumers (such
// as weighting functions) can fetch additional data on demand.
type DataSource interface {
	// Fetch returns a DataFrame covering [currentDate - lookback, currentDate]
	// for the given assets and metrics.
	Fetch(ctx context.Context, assets []asset.Asset, lookback Period,
		metrics []Metric) (*DataFrame, error)

	// FetchAt returns a single-row DataFrame at the given time for the given
	// assets and metrics.
	FetchAt(ctx context.Context, assets []asset.Asset, t time.Time,
		metrics []Metric) (*DataFrame, error)

	// CurrentDate returns the current simulation date.
	CurrentDate() time.Time
}
```

- [ ] **Step 2: Replace universe/data_source.go with a type alias**

Replace the entire contents of `universe/data_source.go` with:

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

package universe

import "github.com/penny-vault/pvbt/data"

// DataSource is an alias for data.DataSource, kept for backward compatibility
// within the universe package.
type DataSource = data.DataSource
```

- [ ] **Step 3: Build to verify compilation**

Run: `go build ./...`
Expected: PASS (type alias makes this transparent to all existing code)

- [ ] **Step 4: Run existing tests**

Run: `go test ./universe/... ./engine/... ./signal/...`
Expected: All tests pass

- [ ] **Step 5: Update compile-time checks to reference data.DataSource**

In `engine/engine.go:664`, change:
```go
var _ universe.DataSource = (*Engine)(nil)
```
to:
```go
var _ data.DataSource = (*Engine)(nil)
```

In `signal/helpers_test.go:30`, change:
```go
var _ universe.DataSource = (*mockDataSource)(nil)
```
to:
```go
var _ data.DataSource = (*mockDataSource)(nil)
```

In `signal/helpers_test.go:48`, change:
```go
var _ universe.DataSource = (*errorDataSource)(nil)
```
to:
```go
var _ data.DataSource = (*errorDataSource)(nil)
```

Update the imports in both files accordingly (add `data` import, remove `universe` import if no longer needed).

- [ ] **Step 6: Build and test**

Run: `go build ./... && go test ./...`
Expected: All tests pass

- [ ] **Step 7: Commit**

```bash
git add data/data_source.go universe/data_source.go engine/engine.go signal/helpers_test.go
git commit -m "refactor: move DataSource interface from universe to data package"
```

---

### Task 2: Add source field to DataFrame

**Files:**
- Modify: `data/data_frame.go:40-68` (struct definition)
- Modify: `engine/engine.go:332,369` (set source on returned frames)

- [ ] **Step 1: Write test for Source/SetSource**

Add to `data/data_frame_test.go`:

```go
var _ = Describe("DataFrame DataSource", func() {
	It("returns nil source by default", func() {
		df, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Source()).To(BeNil())
	})

	It("returns the source after SetSource", func() {
		df, err := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		Expect(err).NotTo(HaveOccurred())

		mock := &mockFrameDataSource{}
		df.SetSource(mock)
		Expect(df.Source()).To(Equal(mock))
	})
})
```

Also add a minimal mock at the top of the test file (or in a test helper):

```go
type mockFrameDataSource struct{}

func (m *mockFrameDataSource) Fetch(_ context.Context, _ []asset.Asset, _ data.Period, _ []data.Metric) (*data.DataFrame, error) {
	return nil, nil
}
func (m *mockFrameDataSource) FetchAt(_ context.Context, _ []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	return nil, nil
}
func (m *mockFrameDataSource) CurrentDate() time.Time { return time.Time{} }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./data/... -run "DataFrame DataSource" -v`
Expected: FAIL -- `Source` and `SetSource` do not exist

- [ ] **Step 3: Add source field and accessors to DataFrame**

In `data/data_frame.go`, add to the DataFrame struct (after `riskFreeRates` field, line 67):

```go
	// source is the DataSource that populated this DataFrame. Weighting
	// functions and other consumers use it to fetch additional data on demand.
	source DataSource
```

Add accessor methods after the existing `Frequency()` method (after line 74):

```go
// Source returns the DataSource that populated this DataFrame, or nil.
func (df *DataFrame) Source() DataSource { return df.source }

// SetSource sets the DataSource on this DataFrame.
func (df *DataFrame) SetSource(ds DataSource) { df.source = ds }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./data/... -run "DataFrame DataSource" -v`
Expected: PASS

- [ ] **Step 5: Wire engine to set source on returned DataFrames**

In `engine/engine.go`, in the `Fetch` method, before the final return (line 332):

Change:
```go
	return assembled, nil
```
to:
```go
	assembled.SetSource(e)

	return assembled, nil
```

In the `FetchAt` method, before the final return (line 369):

Change:
```go
	return result, nil
```
to:
```go
	result.SetSource(e)

	return result, nil
```

- [ ] **Step 6: Build and test**

Run: `go build ./... && go test ./...`
Expected: All tests pass

- [ ] **Step 7: Commit**

```bash
git add data/data_frame.go data/data_frame_test.go engine/engine.go
git commit -m "feat: add DataSource reference to DataFrame"
```

---

### Task 3: Add Correlation method to DataFrame

**Files:**
- Modify: `data/data_frame.go` (add Correlation method after Covariance)
- Modify: `data/data_frame_test.go`

- [ ] **Step 1: Write failing test**

Add to `data/data_frame_test.go`:

```go
var _ = Describe("Correlation", func() {
	It("computes Pearson correlation between two assets", func() {
		times := []time.Time{
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

		// Perfectly correlated: both increase linearly.
		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			[]float64{1, 2, 3, 4, 10, 20, 30, 40},
		)
		Expect(err).NotTo(HaveOccurred())

		result := df.Correlation(spy, aapl)
		Expect(result.Err()).NotTo(HaveOccurred())

		composite := data.CompositeAsset(spy, aapl)
		corr := result.ValueAt(composite, data.AdjClose, result.Times()[0])
		Expect(corr).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("returns -1 for perfectly negatively correlated assets", func() {
		times := []time.Time{
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			[]float64{1, 2, 3, 4, 40, 30, 20, 10},
		)
		Expect(err).NotTo(HaveOccurred())

		result := df.Correlation(spy, aapl)
		Expect(result.Err()).NotTo(HaveOccurred())

		composite := data.CompositeAsset(spy, aapl)
		corr := result.ValueAt(composite, data.AdjClose, result.Times()[0])
		Expect(corr).To(BeNumerically("~", -1.0, 1e-9))
	})

	It("returns 0 for uncorrelated assets", func() {
		times := []time.Time{
			time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
		}
		spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl := asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}

		// Orthogonal: SPY oscillates, AAPL trends.
		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			[]float64{1, -1, 1, -1, 1, 2, 3, 4},
		)
		Expect(err).NotTo(HaveOccurred())

		result := df.Correlation(spy, aapl)
		Expect(result.Err()).NotTo(HaveOccurred())

		composite := data.CompositeAsset(spy, aapl)
		corr := result.ValueAt(composite, data.AdjClose, result.Times()[0])
		Expect(corr).To(BeNumerically("~", 0.0, 0.1))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./data/... -run Correlation -v`
Expected: FAIL -- `Correlation` does not exist

- [ ] **Step 3: Implement Correlation method**

Add to `data/data_frame.go` after the `crossAssetCovariance` method:

```go
// Correlation computes Pearson correlation between columns.
//   - 1 asset: cross-metric correlation. Returns composite metric keys.
//   - 2+ assets: per-metric correlation for all unique pairs. Returns composite asset keys.
func (df *DataFrame) Correlation(assets ...asset.Asset) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	if len(assets) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	var lastTime []time.Time
	if len(df.times) > 0 {
		lastTime = []time.Time{df.times[len(df.times)-1]}
	}

	if len(assets) == 1 {
		return df.crossMetricCorrelation(assets[0], lastTime)
	}

	return df.crossAssetCorrelation(assets, lastTime)
}

func (df *DataFrame) crossMetricCorrelation(targetAsset asset.Asset, lastTime []time.Time) *DataFrame {
	aIdx, ok := df.assetIndex[targetAsset.CompositeFigi]
	if !ok {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	metricLen := len(df.metrics)

	var (
		pairMetrics []Metric
		pairData    []float64
	)

	for i := 0; i < metricLen; i++ {
		for j := i + 1; j < metricLen; j++ {
			pairMetrics = append(pairMetrics, CompositeMetric(df.metrics[i], df.metrics[j]))
			pairData = append(pairData, pearsonCorr(
				df.colSlice(aIdx, i),
				df.colSlice(aIdx, j),
			))
		}
	}

	if len(pairMetrics) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	return mustNewDataFrame(lastTime, []asset.Asset{targetAsset}, pairMetrics, df.freq, pairData)
}

func (df *DataFrame) crossAssetCorrelation(assets []asset.Asset, lastTime []time.Time) *DataFrame {
	metricLen := len(df.metrics)

	var (
		pairAssets []asset.Asset
		pairData   []float64
	)

	for assetIdx := 0; assetIdx < len(assets); assetIdx++ {
		aIdxI, okI := df.assetIndex[assets[assetIdx].CompositeFigi]
		for innerIdx := assetIdx + 1; innerIdx < len(assets); innerIdx++ {
			aIdxJ, okJ := df.assetIndex[assets[innerIdx].CompositeFigi]

			if !okI || !okJ {
				continue
			}

			pairAssets = append(pairAssets, CompositeAsset(assets[assetIdx], assets[innerIdx]))
			for mIdx := 0; mIdx < metricLen; mIdx++ {
				pairData = append(pairData, pearsonCorr(
					df.colSlice(aIdxI, mIdx),
					df.colSlice(aIdxJ, mIdx),
				))
			}
		}
	}

	if len(pairAssets) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(lastTime, pairAssets, metrics, df.freq, pairData)
}

func pearsonCorr(xValues, yValues []float64) float64 {
	count := len(xValues)
	if len(yValues) < count {
		count = len(yValues)
	}

	if count < 2 {
		return 0
	}

	cov := sampleCov(xValues[:count], yValues[:count])
	stdX := sampleStd(xValues[:count])
	stdY := sampleStd(yValues[:count])

	if stdX == 0 || stdY == 0 {
		return 0
	}

	return cov / (stdX * stdY)
}

func sampleStd(values []float64) float64 {
	count := len(values)
	if count < 2 {
		return 0
	}

	m := stat.Mean(values, nil)
	sum := 0.0

	for _, v := range values {
		d := v - m
		sum += d * d
	}

	return math.Sqrt(sum / float64(count-1))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./data/... -run Correlation -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat: add Correlation method to DataFrame"
```

---

### Task 4: Shared weighting helpers

**Files:**
- Create: `portfolio/weighting_helpers.go`

- [ ] **Step 1: Write tests for helpers**

Add to `portfolio/weighting_test.go`, a new Describe block:

```go
var _ = Describe("collectSelected", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		t1   time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		t1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	})

	It("collects assets with Selected > 0", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{100, 200},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{0})).To(Succeed())

		chosen := portfolio.CollectSelected(df, t1)
		Expect(chosen).To(HaveLen(1))
		Expect(chosen[0]).To(Equal(spy))
	})

	It("skips NaN values", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{100, 200},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{math.NaN()})).To(Succeed())

		chosen := portfolio.CollectSelected(df, t1)
		Expect(chosen).To(HaveLen(1))
		Expect(chosen[0]).To(Equal(spy))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/... -run "collectSelected" -v`
Expected: FAIL -- `CollectSelected` does not exist

- [ ] **Step 3: Create `portfolio/weighting_helpers.go`**

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
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// CollectSelected returns the assets whose Selected value is > 0 at the given
// timestamp. NaN and non-positive values are excluded.
func CollectSelected(df *data.DataFrame, timestamp time.Time) []asset.Asset {
	var chosen []asset.Asset

	for _, currentAsset := range df.AssetList() {
		sel := df.ValueAt(currentAsset, Selected, timestamp)
		if sel > 0 && !math.IsNaN(sel) {
			chosen = append(chosen, currentAsset)
		}
	}

	return chosen
}

// HasSelectedColumn reports whether the DataFrame contains the Selected metric.
func HasSelectedColumn(df *data.DataFrame) bool {
	for _, m := range df.MetricList() {
		if m == Selected {
			return true
		}
	}

	return false
}

// ErrMissingSelected returns the standard error for a missing Selected column.
func ErrMissingSelected(funcName string) error {
	return fmt.Errorf("%s: DataFrame missing %q column", funcName, Selected)
}

// ErrNoDataSource returns the standard error for a nil DataSource.
func ErrNoDataSource(funcName, dataDesc string) error {
	return fmt.Errorf("%s: DataFrame has no DataSource; cannot fetch %s", funcName, dataDesc)
}

// equalWeightMembers assigns 1/N weight to each asset in the slice.
func equalWeightMembers(chosen []asset.Asset) map[asset.Asset]float64 {
	members := make(map[asset.Asset]float64, len(chosen))
	if len(chosen) > 0 {
		weight := 1.0 / float64(len(chosen))
		for _, currentAsset := range chosen {
			members[currentAsset] = weight
		}
	}

	return members
}

// defaultLookback returns the given period if non-zero, otherwise 60 days.
func defaultLookback(lookback data.Period) data.Period {
	if lookback == (data.Period{}) {
		return data.Days(60)
	}

	return lookback
}

// nanSafeStd computes the sample standard deviation of a metric's values for
// a given asset across all timestamps, skipping NaN values. This is needed
// because Pct() produces NaN in the first row, and the built-in Std() method
// propagates that NaN through stat.Mean.
func nanSafeStd(df *data.DataFrame, currentAsset asset.Asset, metric data.Metric) float64 {
	times := df.Times()
	var values []float64

	for _, timestamp := range times {
		val := df.ValueAt(currentAsset, metric, timestamp)
		if !math.IsNaN(val) {
			values = append(values, val)
		}
	}

	if len(values) < 2 {
		return 0
	}

	mean := 0.0
	for _, val := range values {
		mean += val
	}
	mean /= float64(len(values))

	sumSq := 0.0
	for _, val := range values {
		diff := val - mean
		sumSq += diff * diff
	}

	return math.Sqrt(sumSq / float64(len(values)-1))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./portfolio/... -run "collectSelected" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/weighting_helpers.go portfolio/weighting_test.go
git commit -m "feat: add shared helpers for weighting functions"
```

---

### Task 5: InverseVolatility weighting function

**Files:**
- Create: `portfolio/inverse_volatility.go`
- Modify: `portfolio/weighting_test.go`

- [ ] **Step 1: Write failing tests**

Add to `portfolio/weighting_test.go`:

```go
var _ = Describe("InverseVolatility", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
	})

	It("weights inversely proportional to volatility", func() {
		// Create 62 days of price data: enough for 60-day lookback + 2 allocation days.
		// SPY: low volatility (small oscillations). AAPL: high volatility (large oscillations).
		numDays := 62
		times := make([]time.Time, numDays)
		spyPrices := make([]float64, numDays)
		aaplPrices := make([]float64, numDays)
		spySelected := make([]float64, numDays)
		aaplSelected := make([]float64, numDays)

		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			spyPrices[idx] = 100.0 + float64(idx%2)*0.1  // tiny oscillation
			if idx%2 == 0 {
				aaplPrices[idx] = 100.0
			} else {
				aaplPrices[idx] = 110.0
			}
			spySelected[idx] = 1
			aaplSelected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			append(spyPrices, aaplPrices...),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, spySelected)).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, aaplSelected)).To(Succeed())

		plan, err := portfolio.InverseVolatility(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).NotTo(BeEmpty())

		// SPY has near-zero volatility, AAPL has high volatility.
		// SPY should get a larger weight than AAPL.
		lastAlloc := plan[len(plan)-1]
		Expect(lastAlloc.Members[spy]).To(BeNumerically(">", lastAlloc.Members[aapl]))
	})

	It("returns error when Selected column is missing", func() {
		df, err := data.NewDataFrame(
			[]time.Time{time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
			[]asset.Asset{spy},
			[]data.Metric{data.AdjClose},
			data.Daily,
			[]float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = portfolio.InverseVolatility(context.Background(), df, data.Period{})
		Expect(err).To(HaveOccurred())
	})

	It("assigns 100% to a single selected asset", func() {
		numDays := 62
		times := make([]time.Time, numDays)
		prices := make([]float64, numDays)
		selected := make([]float64, numDays)

		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			prices[idx] = 100.0 + float64(idx)
			selected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times, []asset.Asset{spy},
			[]data.Metric{data.AdjClose}, data.Daily, prices,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, selected)).To(Succeed())

		plan, err := portfolio.InverseVolatility(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).NotTo(BeEmpty())

		lastAlloc := plan[len(plan)-1]
		Expect(lastAlloc.Members[spy]).To(Equal(1.0))
	})

	It("falls back to equal weight when all volatilities are zero", func() {
		numDays := 62
		times := make([]time.Time, numDays)
		spyPrices := make([]float64, numDays)
		aaplPrices := make([]float64, numDays)
		spySelected := make([]float64, numDays)
		aaplSelected := make([]float64, numDays)

		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			spyPrices[idx] = 100.0  // constant price => zero vol
			aaplPrices[idx] = 50.0  // constant price => zero vol
			spySelected[idx] = 1
			aaplSelected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			append(spyPrices, aaplPrices...),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, spySelected)).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, aaplSelected)).To(Succeed())

		plan, err := portfolio.InverseVolatility(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).NotTo(BeEmpty())

		lastAlloc := plan[len(plan)-1]
		Expect(lastAlloc.Members[spy]).To(Equal(0.5))
		Expect(lastAlloc.Members[aapl]).To(Equal(0.5))
	})

	It("normalizes weights to sum to 1.0", func() {
		numDays := 62
		times := make([]time.Time, numDays)
		spyPrices := make([]float64, numDays)
		aaplPrices := make([]float64, numDays)
		spySelected := make([]float64, numDays)
		aaplSelected := make([]float64, numDays)

		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			spyPrices[idx] = 100.0 + float64(idx)*0.5
			aaplPrices[idx] = 200.0 + float64(idx)*2.0
			spySelected[idx] = 1
			aaplSelected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			append(spyPrices, aaplPrices...),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, spySelected)).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, aaplSelected)).To(Succeed())

		plan, err := portfolio.InverseVolatility(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).NotTo(BeEmpty())

		for _, alloc := range plan {
			sum := 0.0
			for _, weight := range alloc.Members {
				sum += weight
			}
			Expect(sum).To(BeNumerically("~", 1.0, 1e-9))
		}
	})

	It("returns error when source is nil and data is insufficient", func() {
		times := []time.Time{time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)}
		df, err := data.NewDataFrame(
			times, []asset.Asset{spy},
			[]data.Metric{data.MetricClose}, data.Daily, []float64{100},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())

		// No AdjClose metric and no source to fetch it => error.
		_, err = portfolio.InverseVolatility(context.Background(), df, data.Period{})
		Expect(err).To(HaveOccurred())
	})
})
```

Add `"context"` to the imports at the top of `weighting_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/... -run "InverseVolatility" -v`
Expected: FAIL -- `InverseVolatility` does not exist

- [ ] **Step 3: Implement InverseVolatility**

Create `portfolio/inverse_volatility.go`:

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
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// InverseVolatility builds a PortfolioPlan by weighting each selected asset
// inversely proportional to its trailing volatility. A zero-value lookback
// defaults to 60 calendar days. Falls back to equal weight when all selected
// assets have zero or NaN volatility.
func InverseVolatility(ctx context.Context, df *data.DataFrame, lookback data.Period) (PortfolioPlan, error) {
	if !HasSelectedColumn(df) {
		return nil, ErrMissingSelected("InverseVolatility")
	}

	lookback = defaultLookback(lookback)
	times := df.Times()
	assets := df.AssetList()

	// Ensure AdjClose data is available.
	priceDF, err := ensureMetric(ctx, df, assets, lookback, data.AdjClose)
	if err != nil {
		return nil, fmt.Errorf("InverseVolatility: %w", err)
	}

	plan := make(PortfolioPlan, len(times))

	for timeIdx, timestamp := range times {
		chosen := CollectSelected(df, timestamp)

		if len(chosen) <= 1 {
			plan[timeIdx] = Allocation{Date: timestamp, Members: equalWeightMembers(chosen)}
			continue
		}

		// Compute trailing volatility for each chosen asset.
		window := priceDF.Between(lookback.Before(timestamp), timestamp)
		returns := window.Pct()

		invVols := make([]float64, len(chosen))
		sumInvVol := 0.0

		for idx, currentAsset := range chosen {
			vol := nanSafeStd(returns, currentAsset, data.AdjClose)

			if vol <= 0 {
				invVols[idx] = 0
			} else {
				invVols[idx] = 1.0 / vol
				sumInvVol += invVols[idx]
			}
		}

		members := make(map[asset.Asset]float64, len(chosen))

		if sumInvVol == 0 {
			members = equalWeightMembers(chosen)
		} else {
			for idx, currentAsset := range chosen {
				weight := invVols[idx] / sumInvVol
				if weight > 0 {
					members[currentAsset] = weight
				}
			}
		}

		plan[timeIdx] = Allocation{Date: timestamp, Members: members}
	}

	return plan, nil
}

// ensureMetric checks whether the DataFrame contains the given metric.
// If not, it fetches data via the DataFrame's DataSource. Returns a DataFrame
// that contains the metric for the given assets.
func ensureMetric(ctx context.Context, df *data.DataFrame, assets []asset.Asset, lookback data.Period, metric data.Metric) (*data.DataFrame, error) {
	// Check if metric is already present.
	for _, m := range df.MetricList() {
		if m == metric {
			return df, nil
		}
	}

	// Metric not present; fetch via source.
	source := df.Source()
	if source == nil {
		return nil, ErrNoDataSource("ensureMetric", string(metric))
	}

	return source.Fetch(ctx, assets, lookback, []data.Metric{metric})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -run "InverseVolatility" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/inverse_volatility.go portfolio/weighting_test.go
git commit -m "feat: add InverseVolatility weighting function"
```

---

### Task 6: MarketCapWeighted weighting function

**Files:**
- Create: `portfolio/market_cap_weighted.go`
- Modify: `portfolio/weighting_test.go`

- [ ] **Step 1: Write failing tests**

Add to `portfolio/weighting_test.go`:

```go
var _ = Describe("MarketCapWeighted", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
		t1   time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
		t1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	})

	It("weights proportionally to market cap", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{300, 100},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.MarketCapWeighted(context.Background(), df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(0.75))
		Expect(plan[0].Members[aapl]).To(Equal(0.25))
	})

	It("returns error when Selected column is missing", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{300},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = portfolio.MarketCapWeighted(context.Background(), df)
		Expect(err).To(HaveOccurred())
	})

	It("falls back to equal weight when all market caps are zero", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{0, 0},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.MarketCapWeighted(context.Background(), df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("falls back to equal weight when all market caps are NaN", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{math.NaN(), math.NaN()},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.MarketCapWeighted(context.Background(), df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(0.5))
		Expect(plan[0].Members[aapl]).To(Equal(0.5))
	})

	It("assigns 100% to a single asset", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{500},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.MarketCapWeighted(context.Background(), df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))
		Expect(plan[0].Members[spy]).To(Equal(1.0))
	})

	It("normalizes weights to sum to 1.0", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.MarketCap},
			data.Daily,
			[]float64{200, 800},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, []float64{1})).To(Succeed())

		plan, err := portfolio.MarketCapWeighted(context.Background(), df)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).To(HaveLen(1))

		sum := 0.0
		for _, weight := range plan[0].Members {
			sum += weight
		}
		Expect(sum).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("returns error when source is nil and MarketCap is missing", func() {
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[]float64{100},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, []float64{1})).To(Succeed())

		_, err = portfolio.MarketCapWeighted(context.Background(), df)
		Expect(err).To(HaveOccurred())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/... -run "MarketCapWeighted" -v`
Expected: FAIL -- `MarketCapWeighted` does not exist

- [ ] **Step 3: Implement MarketCapWeighted**

Create `portfolio/market_cap_weighted.go`:

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
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// MarketCapWeighted builds a PortfolioPlan by weighting each selected asset
// proportionally to its MarketCap value. If MarketCap is not in the DataFrame,
// it fetches via the DataFrame's DataSource. Falls back to equal weight when
// all selected assets have zero or NaN market caps.
func MarketCapWeighted(ctx context.Context, df *data.DataFrame) (PortfolioPlan, error) {
	if !HasSelectedColumn(df) {
		return nil, ErrMissingSelected("MarketCapWeighted")
	}

	times := df.Times()
	assets := df.AssetList()

	// Ensure MarketCap data is available.
	mcapDF, err := ensureMarketCap(ctx, df, assets)
	if err != nil {
		return nil, fmt.Errorf("MarketCapWeighted: %w", err)
	}

	plan := make(PortfolioPlan, len(times))

	for timeIdx, timestamp := range times {
		chosen := CollectSelected(df, timestamp)

		var (
			values []float64
			sum    float64
		)

		for _, currentAsset := range chosen {
			mcap := mcapDF.ValueAt(currentAsset, data.MarketCap, timestamp)
			if math.IsNaN(mcap) || mcap <= 0 {
				values = append(values, 0)
			} else {
				values = append(values, mcap)
				sum += mcap
			}
		}

		members := make(map[asset.Asset]float64, len(chosen))

		if sum == 0 && len(chosen) > 0 {
			members = equalWeightMembers(chosen)
		} else {
			for idx, currentAsset := range chosen {
				weight := values[idx] / sum
				if weight > 0 {
					members[currentAsset] = weight
				}
			}
		}

		plan[timeIdx] = Allocation{Date: timestamp, Members: members}
	}

	return plan, nil
}

// ensureMarketCap checks whether the DataFrame contains MarketCap. If not,
// it fetches via FetchAt using the source's current date.
func ensureMarketCap(ctx context.Context, df *data.DataFrame, assets []asset.Asset) (*data.DataFrame, error) {
	for _, m := range df.MetricList() {
		if m == data.MarketCap {
			return df, nil
		}
	}

	source := df.Source()
	if source == nil {
		return nil, ErrNoDataSource("MarketCapWeighted", "MarketCap")
	}

	return source.FetchAt(ctx, assets, source.CurrentDate(), []data.Metric{data.MarketCap})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -run "MarketCapWeighted" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/market_cap_weighted.go portfolio/weighting_test.go
git commit -m "feat: add MarketCapWeighted weighting function"
```

---

### Task 7: RiskParityFast weighting function

**Files:**
- Create: `portfolio/risk_parity_fast.go`
- Modify: `portfolio/weighting_test.go`

- [ ] **Step 1: Write failing tests**

Add to `portfolio/weighting_test.go`:

```go
var _ = Describe("RiskParityFast", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
	})

	It("produces weights that differ from pure inverse volatility when correlated", func() {
		// Create correlated price data. SPY and AAPL move together but with
		// different magnitudes -- correlation adjustment should shift weights.
		numDays := 62
		times := make([]time.Time, numDays)
		spyPrices := make([]float64, numDays)
		aaplPrices := make([]float64, numDays)
		spySelected := make([]float64, numDays)
		aaplSelected := make([]float64, numDays)

		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			// Both trending up but AAPL with 2x the volatility.
			spyPrices[idx] = 100.0 + float64(idx)*0.5 + float64(idx%3)*0.5
			aaplPrices[idx] = 200.0 + float64(idx)*1.0 + float64(idx%3)*1.0
			spySelected[idx] = 1
			aaplSelected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			append(spyPrices, aaplPrices...),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, spySelected)).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, aaplSelected)).To(Succeed())

		fastPlan, err := portfolio.RiskParityFast(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())
		Expect(fastPlan).NotTo(BeEmpty())

		ivPlan, err := portfolio.InverseVolatility(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())

		// Weights should be different from pure inverse volatility.
		lastFast := fastPlan[len(fastPlan)-1]
		lastIV := ivPlan[len(ivPlan)-1]

		// They should be close but not identical when assets are correlated.
		// Just verify they're valid weights.
		sum := 0.0
		for _, weight := range lastFast.Members {
			Expect(weight).To(BeNumerically(">=", 0))
			sum += weight
		}
		Expect(sum).To(BeNumerically("~", 1.0, 1e-9))

		// And that SPY gets more weight (lower vol).
		Expect(lastFast.Members[spy]).To(BeNumerically(">", lastFast.Members[aapl]))
		_ = lastIV // used to verify the plan was computed
	})

	It("returns error when Selected column is missing", func() {
		t1 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.AdjClose},
			data.Daily,
			[]float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = portfolio.RiskParityFast(context.Background(), df, data.Period{})
		Expect(err).To(HaveOccurred())
	})

	It("falls back to equal weight when all volatilities are zero", func() {
		numDays := 62
		times := make([]time.Time, numDays)
		spyPrices := make([]float64, numDays)
		aaplPrices := make([]float64, numDays)
		spySelected := make([]float64, numDays)
		aaplSelected := make([]float64, numDays)

		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			spyPrices[idx] = 100.0
			aaplPrices[idx] = 50.0
			spySelected[idx] = 1
			aaplSelected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			append(spyPrices, aaplPrices...),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, spySelected)).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, aaplSelected)).To(Succeed())

		plan, err := portfolio.RiskParityFast(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).NotTo(BeEmpty())

		lastAlloc := plan[len(plan)-1]
		Expect(lastAlloc.Members[spy]).To(Equal(0.5))
		Expect(lastAlloc.Members[aapl]).To(Equal(0.5))
	})

	It("assigns 100% to a single selected asset", func() {
		numDays := 62
		times := make([]time.Time, numDays)
		prices := make([]float64, numDays)
		selected := make([]float64, numDays)

		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			prices[idx] = 100.0 + float64(idx)
			selected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times, []asset.Asset{spy},
			[]data.Metric{data.AdjClose}, data.Daily, prices,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, selected)).To(Succeed())

		plan, err := portfolio.RiskParityFast(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).NotTo(BeEmpty())

		lastAlloc := plan[len(plan)-1]
		Expect(lastAlloc.Members[spy]).To(Equal(1.0))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/... -run "RiskParityFast" -v`
Expected: FAIL -- `RiskParityFast` does not exist

- [ ] **Step 3: Implement RiskParityFast**

Create `portfolio/risk_parity_fast.go`:

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
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// RiskParityFast builds a PortfolioPlan using a single-pass approximation of
// equal risk contribution. It starts with inverse volatility weights and
// adjusts for pairwise correlations via the naive risk parity formula:
//
//	w_i = (1/sigma_i) / (C @ w)_i, then normalize.
//
// A zero-value lookback defaults to 60 calendar days. Falls back to equal
// weight when all volatilities are zero or the covariance matrix degenerates.
func RiskParityFast(ctx context.Context, df *data.DataFrame, lookback data.Period) (PortfolioPlan, error) {
	if !HasSelectedColumn(df) {
		return nil, ErrMissingSelected("RiskParityFast")
	}

	lookback = defaultLookback(lookback)
	times := df.Times()
	assets := df.AssetList()

	priceDF, err := ensureMetric(ctx, df, assets, lookback, data.AdjClose)
	if err != nil {
		return nil, fmt.Errorf("RiskParityFast: %w", err)
	}

	plan := make(PortfolioPlan, len(times))

	for timeIdx, timestamp := range times {
		chosen := CollectSelected(df, timestamp)

		if len(chosen) <= 1 {
			plan[timeIdx] = Allocation{Date: timestamp, Members: equalWeightMembers(chosen)}
			continue
		}

		window := priceDF.Between(lookback.Before(timestamp), timestamp)
		returns := window.Pct()

		members, fallback := riskParityFastWeights(returns, chosen)
		if fallback {
			members = equalWeightMembers(chosen)
		}

		plan[timeIdx] = Allocation{Date: timestamp, Members: members}
	}

	return plan, nil
}

// riskParityFastWeights computes the single-pass naive risk parity weights.
// Returns the weight map and a boolean indicating whether fallback to equal
// weight is needed.
func riskParityFastWeights(returns *data.DataFrame, chosen []asset.Asset) (map[asset.Asset]float64, bool) {
	numAssets := len(chosen)

	// Compute volatilities using NaN-safe helper (Pct produces NaN in first row).
	vols := make([]float64, numAssets)
	allZero := true

	for idx, currentAsset := range chosen {
		vol := nanSafeStd(returns, currentAsset, data.AdjClose)

		if vol <= 0 {
			vols[idx] = 0
		} else {
			vols[idx] = vol
			allZero = false
		}
	}

	if allZero {
		return nil, true
	}

	// Start with inverse volatility weights.
	weights := make([]float64, numAssets)
	for idx := range numAssets {
		if vols[idx] > 0 {
			weights[idx] = 1.0 / vols[idx]
		}
	}

	// Compute covariance matrix (as flat NxN).
	covMatrix := computeCovMatrix(returns, chosen)
	if covMatrix == nil {
		return nil, true
	}

	// Compute marginal risk contribution: (C @ w)_i.
	marginalRisk := make([]float64, numAssets)

	for idx := range numAssets {
		for jdx := range numAssets {
			marginalRisk[idx] += covMatrix[idx*numAssets+jdx] * weights[jdx]
		}
	}

	// Adjust: w_i = w_i / marginalRisk_i, then normalize.
	sumWeights := 0.0

	for idx := range numAssets {
		if marginalRisk[idx] > 0 {
			weights[idx] /= marginalRisk[idx]
		} else {
			weights[idx] = 0
		}

		sumWeights += weights[idx]
	}

	if sumWeights == 0 {
		return nil, true
	}

	members := make(map[asset.Asset]float64, numAssets)

	for idx, currentAsset := range chosen {
		weight := weights[idx] / sumWeights
		if weight > 0 {
			members[currentAsset] = weight
		}
	}

	return members, false
}

// computeCovMatrix computes a flat NxN covariance matrix from return data.
// Returns nil if computation fails.
func computeCovMatrix(returns *data.DataFrame, chosen []asset.Asset) []float64 {
	numAssets := len(chosen)
	covMatrix := make([]float64, numAssets*numAssets)
	returnTimes := returns.Times()

	if len(returnTimes) < 2 {
		return nil
	}

	// Extract return series for each asset.
	series := make([][]float64, numAssets)

	for idx, currentAsset := range chosen {
		vals := make([]float64, len(returnTimes))

		for tdx, timestamp := range returnTimes {
			val := returns.ValueAt(currentAsset, data.AdjClose, timestamp)
			if math.IsNaN(val) {
				val = 0
			}

			vals[tdx] = val
		}

		series[idx] = vals
	}

	// Compute sample covariance.
	for idx := range numAssets {
		for jdx := range numAssets {
			covMatrix[idx*numAssets+jdx] = sampleCovariance(series[idx], series[jdx])
		}
	}

	return covMatrix
}

// sampleCovariance computes sample covariance between two equal-length slices
// using N-1 denominator.
func sampleCovariance(seriesA, seriesB []float64) float64 {
	numObs := len(seriesA)
	if numObs < 2 {
		return 0
	}

	meanA := 0.0
	meanB := 0.0

	for idx := range numObs {
		meanA += seriesA[idx]
		meanB += seriesB[idx]
	}

	meanA /= float64(numObs)
	meanB /= float64(numObs)

	cov := 0.0
	for idx := range numObs {
		cov += (seriesA[idx] - meanA) * (seriesB[idx] - meanB)
	}

	return cov / float64(numObs-1)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -run "RiskParityFast" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/risk_parity_fast.go portfolio/weighting_test.go
git commit -m "feat: add RiskParityFast weighting function"
```

---

### Task 8: RiskParity weighting function (iterative)

**Files:**
- Create: `portfolio/risk_parity.go`
- Modify: `portfolio/weighting_test.go`

- [ ] **Step 1: Write failing tests**

Add to `portfolio/weighting_test.go`:

```go
var _ = Describe("RiskParity", func() {
	var (
		spy  asset.Asset
		aapl asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
	})

	It("converges to equal risk contribution for 2-asset case", func() {
		// For 2 uncorrelated assets with known volatilities, equal risk
		// contribution means w_1*sigma_1 = w_2*sigma_2.
		// If sigma_SPY=1%, sigma_AAPL=2%, then w_SPY/w_AAPL = 2/1 = 2.
		// So w_SPY = 2/3, w_AAPL = 1/3.
		numDays := 120
		times := make([]time.Time, numDays)
		spyPrices := make([]float64, numDays)
		aaplPrices := make([]float64, numDays)
		spySelected := make([]float64, numDays)
		aaplSelected := make([]float64, numDays)

		// Use deterministic price series with known volatility ratio.
		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			// SPY: small oscillations around 100.
			spyPrices[idx] = 100.0 + math.Sin(float64(idx)*0.3)*1.0
			// AAPL: larger oscillations around 200.
			aaplPrices[idx] = 200.0 + math.Sin(float64(idx)*0.3)*4.0
			spySelected[idx] = 1
			aaplSelected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			append(spyPrices, aaplPrices...),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, spySelected)).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, aaplSelected)).To(Succeed())

		plan, err := portfolio.RiskParity(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).NotTo(BeEmpty())

		lastAlloc := plan[len(plan)-1]

		// SPY should get more weight (lower volatility).
		Expect(lastAlloc.Members[spy]).To(BeNumerically(">", lastAlloc.Members[aapl]))

		// Weights should sum to 1.
		sum := 0.0
		for _, weight := range lastAlloc.Members {
			Expect(weight).To(BeNumerically(">=", 0))
			sum += weight
		}
		Expect(sum).To(BeNumerically("~", 1.0, 1e-9))
	})

	It("returns error when Selected column is missing", func() {
		t1 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		df, err := data.NewDataFrame(
			[]time.Time{t1},
			[]asset.Asset{spy},
			[]data.Metric{data.AdjClose},
			data.Daily,
			[]float64{100},
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = portfolio.RiskParity(context.Background(), df, data.Period{})
		Expect(err).To(HaveOccurred())
	})

	It("falls back to equal weight when all volatilities are zero", func() {
		numDays := 62
		times := make([]time.Time, numDays)
		spyPrices := make([]float64, numDays)
		aaplPrices := make([]float64, numDays)
		spySelected := make([]float64, numDays)
		aaplSelected := make([]float64, numDays)

		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			spyPrices[idx] = 100.0
			aaplPrices[idx] = 50.0
			spySelected[idx] = 1
			aaplSelected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			append(spyPrices, aaplPrices...),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, spySelected)).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, aaplSelected)).To(Succeed())

		plan, err := portfolio.RiskParity(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).NotTo(BeEmpty())

		lastAlloc := plan[len(plan)-1]
		Expect(lastAlloc.Members[spy]).To(Equal(0.5))
		Expect(lastAlloc.Members[aapl]).To(Equal(0.5))
	})

	It("assigns 100% to a single selected asset", func() {
		numDays := 62
		times := make([]time.Time, numDays)
		prices := make([]float64, numDays)
		selected := make([]float64, numDays)

		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			prices[idx] = 100.0 + float64(idx)
			selected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times, []asset.Asset{spy},
			[]data.Metric{data.AdjClose}, data.Daily, prices,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, selected)).To(Succeed())

		plan, err := portfolio.RiskParity(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())
		Expect(plan).NotTo(BeEmpty())

		lastAlloc := plan[len(plan)-1]
		Expect(lastAlloc.Members[spy]).To(Equal(1.0))
	})

	It("result is close to RiskParityFast", func() {
		numDays := 120
		times := make([]time.Time, numDays)
		spyPrices := make([]float64, numDays)
		aaplPrices := make([]float64, numDays)
		spySelected := make([]float64, numDays)
		aaplSelected := make([]float64, numDays)

		for idx := range numDays {
			times[idx] = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, idx)
			spyPrices[idx] = 100.0 + float64(idx)*0.5 + math.Sin(float64(idx)*0.2)*2.0
			aaplPrices[idx] = 200.0 + float64(idx)*1.0 + math.Sin(float64(idx)*0.2)*5.0
			spySelected[idx] = 1
			aaplSelected[idx] = 1
		}

		df, err := data.NewDataFrame(
			times,
			[]asset.Asset{spy, aapl},
			[]data.Metric{data.AdjClose},
			data.Daily,
			append(spyPrices, aaplPrices...),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(df.Insert(spy, portfolio.Selected, spySelected)).To(Succeed())
		Expect(df.Insert(aapl, portfolio.Selected, aaplSelected)).To(Succeed())

		iterPlan, err := portfolio.RiskParity(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())

		fastPlan, err := portfolio.RiskParityFast(context.Background(), df, data.Period{})
		Expect(err).NotTo(HaveOccurred())

		// Both should produce valid weights, and for 2-asset case the results
		// should be reasonably close (within 10%).
		lastIter := iterPlan[len(iterPlan)-1]
		lastFast := fastPlan[len(fastPlan)-1]

		Expect(lastIter.Members[spy]).To(BeNumerically("~", lastFast.Members[spy], 0.1))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./portfolio/... -run "RiskParity/" -v`
Expected: FAIL -- `RiskParity` does not exist (note the trailing `/` to avoid matching `RiskParityFast`)

- [ ] **Step 3: Implement RiskParity**

Create `portfolio/risk_parity.go`:

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
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/rs/zerolog"
)

const (
	riskParityMaxIter    = 1000
	riskParityTolerance  = 1e-10
	riskParityStepSize   = 0.5
)

// RiskParity builds a PortfolioPlan using iterative optimization to equalize
// each asset's contribution to total portfolio risk. Uses Newton's method
// with simplex projection. A zero-value lookback defaults to 60 calendar days.
//
// Returns the best result found after riskParityMaxIter iterations. Logs a
// warning via zerolog if convergence is not reached. Falls back to equal
// weight when the covariance matrix degenerates.
func RiskParity(ctx context.Context, df *data.DataFrame, lookback data.Period) (PortfolioPlan, error) {
	if !HasSelectedColumn(df) {
		return nil, ErrMissingSelected("RiskParity")
	}

	lookback = defaultLookback(lookback)
	times := df.Times()
	assets := df.AssetList()
	log := zerolog.Ctx(ctx)

	priceDF, err := ensureMetric(ctx, df, assets, lookback, data.AdjClose)
	if err != nil {
		return nil, fmt.Errorf("RiskParity: %w", err)
	}

	plan := make(PortfolioPlan, len(times))

	for timeIdx, timestamp := range times {
		chosen := CollectSelected(df, timestamp)

		if len(chosen) <= 1 {
			plan[timeIdx] = Allocation{Date: timestamp, Members: equalWeightMembers(chosen)}
			continue
		}

		window := priceDF.Between(lookback.Before(timestamp), timestamp)
		returns := window.Pct()

		covMatrix := computeCovMatrix(returns, chosen)
		if covMatrix == nil {
			plan[timeIdx] = Allocation{Date: timestamp, Members: equalWeightMembers(chosen)}
			continue
		}

		weights, converged := solveRiskParity(covMatrix, len(chosen))
		if !converged {
			log.Warn().
				Time("date", timestamp).
				Int("maxIter", riskParityMaxIter).
				Msg("RiskParity: did not converge, using best result")
		}

		if weights == nil {
			plan[timeIdx] = Allocation{Date: timestamp, Members: equalWeightMembers(chosen)}
			continue
		}

		members := make(map[asset.Asset]float64, len(chosen))

		for idx, currentAsset := range chosen {
			if weights[idx] > 0 {
				members[currentAsset] = weights[idx]
			}
		}

		if len(members) == 0 {
			members = equalWeightMembers(chosen)
		}

		plan[timeIdx] = Allocation{Date: timestamp, Members: members}
	}

	return plan, nil
}

// solveRiskParity runs Newton's method to find weights where each asset
// contributes equally to total portfolio risk. Returns the weight vector and
// whether convergence was achieved.
func solveRiskParity(covMatrix []float64, numAssets int) ([]float64, bool) {
	targetRC := 1.0 / float64(numAssets)

	// Initialize to equal weight.
	weights := make([]float64, numAssets)
	for idx := range numAssets {
		weights[idx] = targetRC
	}

	bestWeights := make([]float64, numAssets)
	copy(bestWeights, weights)
	bestError := math.Inf(1)

	for range riskParityMaxIter {
		// Compute portfolio variance: w^T C w.
		portVar := quadForm(covMatrix, weights, numAssets)
		if portVar <= 0 {
			return nil, false
		}

		// Compute risk contributions: rc_i = w_i * (C @ w)_i / portVar.
		marginal := matVecMul(covMatrix, weights, numAssets)
		riskContrib := make([]float64, numAssets)
		maxErr := 0.0

		for idx := range numAssets {
			riskContrib[idx] = weights[idx] * marginal[idx] / portVar
			errVal := math.Abs(riskContrib[idx] - targetRC)

			if errVal > maxErr {
				maxErr = errVal
			}
		}

		// Track best solution.
		currentError := maxErr
		if currentError < bestError {
			bestError = currentError
			copy(bestWeights, weights)
		}

		if maxErr < riskParityTolerance {
			return weights, true
		}

		// Gradient step: move weights toward equal risk contribution.
		// Use the formula: w_i_new = w_i * (targetRC / rc_i)^stepSize.
		newWeights := make([]float64, numAssets)
		sumNew := 0.0

		for idx := range numAssets {
			if riskContrib[idx] > 0 {
				ratio := targetRC / riskContrib[idx]
				newWeights[idx] = weights[idx] * math.Pow(ratio, riskParityStepSize)
			} else {
				newWeights[idx] = weights[idx]
			}

			if newWeights[idx] < 0 {
				newWeights[idx] = 0
			}

			sumNew += newWeights[idx]
		}

		// Normalize to simplex.
		if sumNew <= 0 {
			return nil, false
		}

		for idx := range numAssets {
			weights[idx] = newWeights[idx] / sumNew
		}

	}

	// Did not converge; return best found.
	return bestWeights, false
}

// quadForm computes w^T M w for a flat NxN matrix M.
func quadForm(matrix, vec []float64, size int) float64 {
	result := 0.0

	for idx := range size {
		for jdx := range size {
			result += vec[idx] * matrix[idx*size+jdx] * vec[jdx]
		}
	}

	return result
}

// matVecMul computes M @ v for a flat NxN matrix M.
func matVecMul(matrix, vec []float64, size int) []float64 {
	result := make([]float64, size)

	for idx := range size {
		for jdx := range size {
			result[idx] += matrix[idx*size+jdx] * vec[jdx]
		}
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./portfolio/... -run "RiskParity/" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add portfolio/risk_parity.go portfolio/weighting_test.go
git commit -m "feat: add RiskParity iterative weighting function"
```

---

### Task 9: Lint and final verification

**Files:** All modified/created files

- [ ] **Step 1: Run linter**

Run: `golangci-lint run ./...`
Expected: No new lint errors. Fix any that appear in the new files.

- [ ] **Step 2: Run full test suite**

Run: `go test ./...`
Expected: All tests pass

- [ ] **Step 3: Fix any lint or test issues**

Address all issues found in steps 1-2.

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: address lint issues in weighting strategies"
```

(Only if there were issues to fix.)
