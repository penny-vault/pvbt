# DataFrame Stats API and Resample Builder Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Aggregation type with Downsample/Upsample builders, add column-wise stats methods to DataFrame, implement Covariance with composite keys, and fix Rolling.Std to use sample (N-1) denominator.

**Architecture:** Downsample and Upsample are separate builder types (like the existing Rolling builder). Column-wise stats methods (Mean, Std, Variance, Sum, Max, Min) use Reduce internally. Covariance is a cross-column method with composite asset/metric keys for pair results. Existing cross-asset Max/Min/IdxMax are renamed to *AcrossAssets.

**Tech Stack:** Go, ginkgo v2 + gomega, gonum (floats, stat)

**Spec:** `docs/superpowers/specs/2026-03-12-dataframe-stats-api-design.md`

---

## Chunk 1: Column-wise stats methods and Rolling updates

### Task 1: Add column-wise Mean, Sum, Max, Min to DataFrame

These are implemented via `Reduce` -- each collapses the time dimension into a single-row DataFrame.

**Files:**
- Modify: `data/data_frame.go`
- Modify: `data/data_frame_test.go`

- [ ] **Step 1: Write failing tests**

Add to `data/data_frame_test.go` inside the top-level `Describe("DataFrame", ...)`:

```go
Describe("Column-wise stats", func() {
    var df *data.DataFrame
    var spy, efa asset.Asset

    BeforeEach(func() {
        spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
        efa = asset.Asset{CompositeFigi: "EFA", Ticker: "EFA"}
        // 4 timestamps, 2 assets, 1 metric
        t := []time.Time{
            time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
            time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
            time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
            time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
        }
        // SPY.Price: 1,2,3,4  EFA.Price: 10,20,30,40
        vals := []float64{1, 2, 3, 4, 10, 20, 30, 40}
        var err error
        df, err = data.NewDataFrame(t, []asset.Asset{spy, efa}, []data.Metric{data.Price}, vals)
        Expect(err).NotTo(HaveOccurred())
    })

    Describe("Mean", func() {
        It("returns single-row DataFrame with mean of each column", func() {
            result := df.Mean()
            Expect(result.Len()).To(Equal(1))
            Expect(result.Value(spy, data.Price)).To(BeNumerically("~", 2.5, 1e-12))
            Expect(result.Value(efa, data.Price)).To(BeNumerically("~", 25.0, 1e-12))
        })

        It("preserves asset and metric dimensions", func() {
            result := df.Mean()
            Expect(result.AssetList()).To(HaveLen(2))
            Expect(result.MetricList()).To(HaveLen(1))
        })

        It("returns empty DataFrame for empty input", func() {
            empty, err := data.NewDataFrame(nil, nil, nil, nil)
            Expect(err).NotTo(HaveOccurred())
            Expect(empty.Mean().Len()).To(Equal(0))
        })
    })

    Describe("Sum", func() {
        It("returns single-row DataFrame with sum of each column", func() {
            result := df.Sum()
            Expect(result.Value(spy, data.Price)).To(BeNumerically("~", 10.0, 1e-12))
            Expect(result.Value(efa, data.Price)).To(BeNumerically("~", 100.0, 1e-12))
        })
    })

    Describe("Max", func() {
        It("returns single-row DataFrame with max of each column over time", func() {
            result := df.Max()
            Expect(result.Value(spy, data.Price)).To(BeNumerically("~", 4.0, 1e-12))
            Expect(result.Value(efa, data.Price)).To(BeNumerically("~", 40.0, 1e-12))
        })
    })

    Describe("Min", func() {
        It("returns single-row DataFrame with min of each column over time", func() {
            result := df.Min()
            Expect(result.Value(spy, data.Price)).To(BeNumerically("~", 1.0, 1e-12))
            Expect(result.Value(efa, data.Price)).To(BeNumerically("~", 10.0, 1e-12))
        })
    })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./data/... -run "Column-wise" -v`
Expected: FAIL -- `Mean`, `Sum`, `Max`, `Min` methods not defined (compile error because existing `Max`/`Min` have different behavior -- rename must happen first, see Step 3)

- [ ] **Step 3: Rename existing Max/Min/IdxMax to *AcrossAssets**

In `data/data_frame.go`, rename:
- `Max()` -> `MaxAcrossAssets()`
- `Min()` -> `MinAcrossAssets()`
- `IdxMax()` -> `IdxMaxAcrossAssets()`

Update doc comments accordingly. Also update all references in `data/data_frame_test.go`:
- `df.Max()` -> `df.MaxAcrossAssets()` (in the existing "Aggregation across assets" test section)
- `df.Min()` -> `df.MinAcrossAssets()`
- `single.Max()` -> `single.MaxAcrossAssets()`
- `single.Min()` -> `single.MinAcrossAssets()`
- `df.IdxMax()` -> `df.IdxMaxAcrossAssets()`

- [ ] **Step 4: Implement Mean, Sum, Max, Min**

Add to `data/data_frame.go`:

```go
// Mean returns a single-row DataFrame with the arithmetic mean of each
// column over the time dimension.
func (df *DataFrame) Mean() *DataFrame {
    return df.Reduce(func(col []float64) float64 {
        if len(col) == 0 {
            return math.NaN()
        }
        return stat.Mean(col, nil)
    })
}

// Sum returns a single-row DataFrame with the sum of each column over
// the time dimension.
func (df *DataFrame) Sum() *DataFrame {
    return df.Reduce(func(col []float64) float64 {
        return floats.Sum(col)
    })
}

// Max returns a single-row DataFrame with the maximum value of each
// column over the time dimension.
func (df *DataFrame) Max() *DataFrame {
    return df.Reduce(func(col []float64) float64 {
        if len(col) == 0 {
            return math.NaN()
        }
        return floats.Max(col)
    })
}

// Min returns a single-row DataFrame with the minimum value of each
// column over the time dimension.
func (df *DataFrame) Min() *DataFrame {
    return df.Reduce(func(col []float64) float64 {
        if len(col) == 0 {
            return math.NaN()
        }
        return floats.Min(col)
    })
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./data/... -v`
Expected: ALL PASS (both new tests and renamed old tests)

- [ ] **Step 6: Commit**

```
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add column-wise Mean, Sum, Max, Min; rename cross-asset Max/Min to *AcrossAssets"
```

---

### Task 2: Add column-wise Std and Variance to DataFrame

**Files:**
- Modify: `data/data_frame.go`
- Modify: `data/data_frame_test.go`

- [ ] **Step 1: Write failing tests**

Add to the "Column-wise stats" `Describe` block in `data/data_frame_test.go`:

```go
Describe("Variance", func() {
    It("returns single-row DataFrame with sample variance (N-1) of each column", func() {
        result := df.Variance()
        // SPY: [1,2,3,4], mean=2.5, sum sq diffs=5, var=5/3
        Expect(result.Value(spy, data.Price)).To(BeNumerically("~", 5.0/3.0, 1e-12))
    })

    It("returns 0 for single timestamp", func() {
        t := []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
        single, err := data.NewDataFrame(t, []asset.Asset{spy}, []data.Metric{data.Price}, []float64{42})
        Expect(err).NotTo(HaveOccurred())
        Expect(single.Variance().Value(spy, data.Price)).To(BeNumerically("==", 0))
    })
})

Describe("Std", func() {
    It("returns single-row DataFrame with sample std (N-1) of each column", func() {
        result := df.Std()
        expectedVariance := 5.0 / 3.0
        Expect(result.Value(spy, data.Price)).To(BeNumerically("~", math.Sqrt(expectedVariance), 1e-12))
    })

    It("returns 0 for single timestamp", func() {
        t := []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
        single, err := data.NewDataFrame(t, []asset.Asset{spy}, []data.Metric{data.Price}, []float64{42})
        Expect(err).NotTo(HaveOccurred())
        Expect(single.Std().Value(spy, data.Price)).To(BeNumerically("==", 0))
    })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./data/... -run "Variance|Std" -v`
Expected: FAIL -- methods not defined

- [ ] **Step 3: Implement Std and Variance**

Add to `data/data_frame.go`:

```go
// Variance returns a single-row DataFrame with the sample variance (N-1
// denominator) of each column over the time dimension.
func (df *DataFrame) Variance() *DataFrame {
    return df.Reduce(func(col []float64) float64 {
        if len(col) < 2 {
            return 0
        }
        m := stat.Mean(col, nil)
        sum := 0.0
        for _, v := range col {
            d := v - m
            sum += d * d
        }
        return sum / float64(len(col)-1)
    })
}

// Std returns a single-row DataFrame with the sample standard deviation
// (N-1 denominator) of each column over the time dimension.
func (df *DataFrame) Std() *DataFrame {
    return df.Reduce(func(col []float64) float64 {
        if len(col) < 2 {
            return 0
        }
        m := stat.Mean(col, nil)
        sum := 0.0
        for _, v := range col {
            d := v - m
            sum += d * d
        }
        return math.Sqrt(sum / float64(len(col)-1))
    })
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./data/... -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add column-wise Std and Variance with sample (N-1) denominator"
```

---

### Task 3: Fix Rolling.Std to use sample (N-1) denominator and add Rolling.Variance

**Files:**
- Modify: `data/rolling_data_frame.go`
- Modify: `data/rolling_data_frame_test.go`

- [ ] **Step 1: Write failing test for Variance**

Add to `data/rolling_data_frame_test.go` inside the existing Rolling describe block:

```go
It("Variance computes rolling sample variance (N-1)", func() {
    // col = [1,2,3,4,5], window=3
    // idx 2: var([1,2,3]) = 1.0  (mean=2, sum sq diffs=2, 2/2=1.0)
    // idx 3: var([2,3,4]) = 1.0
    // idx 4: var([3,4,5]) = 1.0
    result := df.Rolling(3).Variance()
    col := result.Column(aapl, data.Price)
    Expect(math.IsNaN(col[0])).To(BeTrue())
    Expect(math.IsNaN(col[1])).To(BeTrue())
    Expect(col[2]).To(BeNumerically("~", 1.0, 1e-12))
    Expect(col[3]).To(BeNumerically("~", 1.0, 1e-12))
    Expect(col[4]).To(BeNumerically("~", 1.0, 1e-12))
})
```

- [ ] **Step 2: Update existing Std test for N-1 denominator**

The existing Rolling Std test expects population std. Update it to expect sample std (N-1):

The existing test at line 115 of `data/rolling_data_frame_test.go` tests `Rolling(3).Std()`. The values are `[1,2,3,4,5]` with window=3. For window `[1,2,3]`: mean=2, sample variance = ((1-2)^2 + (2-2)^2 + (3-2)^2) / 2 = 1.0, sample std = 1.0. Update expected values accordingly.

Read the existing test first to see the current expected values, then adjust.

- [ ] **Step 3: Implement Variance and fix Std**

In `data/rolling_data_frame.go`, add `Variance()` and fix `Std()`:

```go
// Variance returns a DataFrame with the rolling sample variance (N-1
// denominator) over the window.
func (r *RollingDataFrame) Variance() *DataFrame {
    return r.df.Apply(func(col []float64) []float64 {
        out := make([]float64, len(col))
        n := r.window

        for i := range col {
            if i < n-1 {
                out[i] = math.NaN()
                continue
            }

            window := col[i-n+1 : i+1]
            mean := stat.Mean(window, nil)
            variance := 0.0
            for _, v := range window {
                d := v - mean
                variance += d * d
            }
            out[i] = variance / float64(n-1)
        }

        return out
    })
}
```

Fix `Std()` -- change line 137 from `variance / float64(n)` to `variance / float64(n-1)`:

```go
out[i] = math.Sqrt(variance / float64(n-1))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./data/... -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```
git add data/rolling_data_frame.go data/rolling_data_frame_test.go
git commit -m "feat(data): add Rolling.Variance; fix Rolling.Std to use sample (N-1) denominator"
```

---

### Task 4: Composite key helpers

**Files:**
- Modify: `data/data_frame.go`
- Modify: `data/data_frame_test.go`

- [ ] **Step 1: Write failing tests**

Add to `data/data_frame_test.go`:

```go
Describe("Composite keys", func() {
    It("CompositeAsset joins two assets with colon separator", func() {
        spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
        efa := asset.Asset{CompositeFigi: "EFA", Ticker: "EFA"}
        result := data.CompositeAsset(spy, efa)
        Expect(result.CompositeFigi).To(Equal("SPY:EFA"))
        Expect(result.Ticker).To(Equal("SPY:EFA"))
    })

    It("CompositeMetric joins two metrics with colon separator", func() {
        result := data.CompositeMetric(data.Price, data.Volume)
        Expect(string(result)).To(Equal("Price:Volume"))
    })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./data/... -run "Composite" -v`
Expected: FAIL -- functions not defined

- [ ] **Step 3: Implement composite key helpers**

Add to `data/data_frame.go`:

```go
// CompositeAsset creates an asset representing a pair, with fields
// joined by ":". Used by Covariance for multi-asset results.
func CompositeAsset(a, b asset.Asset) asset.Asset {
    return asset.Asset{
        CompositeFigi: a.CompositeFigi + ":" + b.CompositeFigi,
        Ticker:        a.Ticker + ":" + b.Ticker,
    }
}

// CompositeMetric creates a metric representing a pair, joined by ":".
// Used by Covariance for cross-metric results.
func CompositeMetric(a, b Metric) Metric {
    return Metric(string(a) + ":" + string(b))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./data/... -run "Composite" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add CompositeAsset and CompositeMetric helpers"
```

---

### Task 5: Covariance method

**Files:**
- Modify: `data/data_frame.go`
- Modify: `data/data_frame_test.go`

- [ ] **Step 1: Write failing tests**

Add to `data/data_frame_test.go`:

```go
Describe("Covariance", func() {
    var df *data.DataFrame
    var spy, efa, voo asset.Asset

    BeforeEach(func() {
        spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
        efa = asset.Asset{CompositeFigi: "EFA", Ticker: "EFA"}
        voo = asset.Asset{CompositeFigi: "VOO", Ticker: "VOO"}
        t := []time.Time{
            time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
            time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
            time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
            time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
            time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
        }
        // SPY.Price: 1,2,3,4,5   EFA.Price: 2,4,6,8,10   VOO.Price: 10,9,8,7,6
        // SPY.Volume: 100,200,300,400,500  EFA.Volume: 50,50,50,50,50  VOO.Volume: 10,20,30,40,50
        vals := []float64{
            1, 2, 3, 4, 5,    // SPY.Price
            100, 200, 300, 400, 500, // SPY.Volume
            2, 4, 6, 8, 10,   // EFA.Price
            50, 50, 50, 50, 50, // EFA.Volume
            10, 9, 8, 7, 6,   // VOO.Price
            10, 20, 30, 40, 50, // VOO.Volume
        }
        var err error
        df, err = data.NewDataFrame(t, []asset.Asset{spy, efa, voo}, []data.Metric{data.Price, data.Volume}, vals)
        Expect(err).NotTo(HaveOccurred())
    })

    Context("two assets (per-metric covariance)", func() {
        It("computes covariance between SPY and EFA for each metric", func() {
            result := df.Covariance(spy, efa)
            Expect(result.Len()).To(Equal(1))

            composite := data.CompositeAsset(spy, efa)
            // SPY.Price=[1,2,3,4,5], EFA.Price=[2,4,6,8,10] => perfect linear, cov = 5.0
            Expect(result.Value(composite, data.Price)).To(BeNumerically("~", 5.0, 1e-12))

            // SPY.Volume=[100..500], EFA.Volume=[50,50,50,50,50] => cov = 0
            Expect(result.Value(composite, data.Volume)).To(BeNumerically("~", 0.0, 1e-12))
        })
    })

    Context("three assets (all unique pairs)", func() {
        It("returns N*(N-1)/2 composite assets", func() {
            result := df.Covariance(spy, efa, voo)
            Expect(result.AssetList()).To(HaveLen(3)) // SPY:EFA, SPY:VOO, EFA:VOO
        })

        It("computes correct covariance for each pair", func() {
            result := df.Covariance(spy, efa, voo)
            spyEfa := data.CompositeAsset(spy, efa)
            spyVoo := data.CompositeAsset(spy, voo)

            // SPY.Price and EFA.Price: perfect positive correlation, cov = 5.0
            Expect(result.Value(spyEfa, data.Price)).To(BeNumerically("~", 5.0, 1e-12))

            // SPY.Price=[1,2,3,4,5] and VOO.Price=[10,9,8,7,6]: perfect negative, cov = -2.5
            Expect(result.Value(spyVoo, data.Price)).To(BeNumerically("~", -2.5, 1e-12))
        })
    })

    Context("single asset (cross-metric covariance)", func() {
        It("returns composite metric keys for each metric pair", func() {
            result := df.Covariance(spy)
            // SPY has Price and Volume => one pair: Price:Volume
            compositeMetric := data.CompositeMetric(data.Price, data.Volume)
            Expect(result.MetricList()).To(ContainElement(compositeMetric))
        })

        It("computes covariance between metrics for that asset", func() {
            result := df.Covariance(spy)
            compositeMetric := data.CompositeMetric(data.Price, data.Volume)
            // SPY.Price=[1,2,3,4,5], SPY.Volume=[100,200,300,400,500]
            // Both have identical shape (linear), cov = 250.0
            Expect(result.Value(spy, compositeMetric)).To(BeNumerically("~", 250.0, 1e-12))
        })
    })

    Context("edge cases", func() {
        It("returns empty DataFrame for zero assets", func() {
            result := df.Covariance()
            Expect(result.Len()).To(Equal(0))
        })

        It("returns 0 for fewer than 2 timestamps", func() {
            t := []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
            short, err := data.NewDataFrame(t, []asset.Asset{spy, efa}, []data.Metric{data.Price}, []float64{1, 2})
            Expect(err).NotTo(HaveOccurred())
            result := short.Covariance(spy, efa)
            composite := data.CompositeAsset(spy, efa)
            Expect(result.Value(composite, data.Price)).To(BeNumerically("==", 0))
        })
    })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./data/... -run "Covariance" -v`
Expected: FAIL -- `Covariance` method not defined

- [ ] **Step 3: Implement Covariance**

Add to `data/data_frame.go`:

```go
// Covariance computes sample covariance (N-1 denominator) between columns.
//   - 1 asset: cross-metric covariance. Returns composite metric keys.
//   - 2+ assets: per-metric covariance for all unique pairs. Returns composite asset keys.
func (df *DataFrame) Covariance(assets ...asset.Asset) *DataFrame {
    if len(assets) == 0 {
        return mustNewDataFrame(nil, nil, nil, nil)
    }

    var lastTime []time.Time
    if len(df.times) > 0 {
        lastTime = []time.Time{df.times[len(df.times)-1]}
    }

    if len(assets) == 1 {
        return df.crossMetricCovariance(assets[0], lastTime)
    }

    return df.crossAssetCovariance(assets, lastTime)
}

func (df *DataFrame) crossMetricCovariance(a asset.Asset, lastTime []time.Time) *DataFrame {
    aIdx, ok := df.assetIndex[a.CompositeFigi]
    if !ok {
        return mustNewDataFrame(nil, nil, nil, nil)
    }

    metricLen := len(df.metrics)
    var pairMetrics []Metric
    var pairData []float64

    for i := 0; i < metricLen; i++ {
        for j := i + 1; j < metricLen; j++ {
            pairMetrics = append(pairMetrics, CompositeMetric(df.metrics[i], df.metrics[j]))
            pairData = append(pairData, sampleCov(
                df.colSlice(aIdx, i),
                df.colSlice(aIdx, j),
            ))
        }
    }

    if len(pairMetrics) == 0 {
        return mustNewDataFrame(nil, nil, nil, nil)
    }

    return mustNewDataFrame(lastTime, []asset.Asset{a}, pairMetrics, pairData)
}

func (df *DataFrame) crossAssetCovariance(assets []asset.Asset, lastTime []time.Time) *DataFrame {
    var pairAssets []asset.Asset
    for i := 0; i < len(assets); i++ {
        for j := i + 1; j < len(assets); j++ {
            pairAssets = append(pairAssets, CompositeAsset(assets[i], assets[j]))
        }
    }

    metricLen := len(df.metrics)
    metrics := make([]Metric, metricLen)
    copy(metrics, df.metrics)

    newData := make([]float64, len(pairAssets)*metricLen)
    pairIdx := 0

    for i := 0; i < len(assets); i++ {
        for j := i + 1; j < len(assets); j++ {
            aIdxI, okI := df.assetIndex[assets[i].CompositeFigi]
            aIdxJ, okJ := df.assetIndex[assets[j].CompositeFigi]

            for mIdx := 0; mIdx < metricLen; mIdx++ {
                var covVal float64
                if okI && okJ {
                    covVal = sampleCov(
                        df.colSlice(aIdxI, mIdx),
                        df.colSlice(aIdxJ, mIdx),
                    )
                }
                dstOff := (pairIdx*metricLen + mIdx)
                newData[dstOff] = covVal
            }

            pairIdx++
        }
    }

    return mustNewDataFrame(lastTime, pairAssets, metrics, newData)
}

func sampleCov(x, y []float64) float64 {
    n := len(x)
    if len(y) < n {
        n = len(y)
    }
    if n < 2 {
        return 0
    }
    mx := stat.Mean(x[:n], nil)
    my := stat.Mean(y[:n], nil)
    sum := 0.0
    for i := 0; i < n; i++ {
        sum += (x[i] - mx) * (y[i] - my)
    }
    return sum / float64(n-1)
}
```

**Note:** `assetIndex` is an unexported `map[string]int` field on DataFrame, keyed by `CompositeFigi`. Use the comma-ok idiom (`aIdx, ok := df.assetIndex[a.CompositeFigi]`) for lookups. `colSlice(aIdx, mIdx)` returns the contiguous `[]float64` column. `colOffset(aIdx, mIdx)` returns the offset into the data slab.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./data/... -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```
git add data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add Covariance method with composite asset/metric keys"
```

---

## Chunk 2: Downsample and Upsample builders

### Task 6: Create DownsampledDataFrame builder

This replaces the first half of the old `Resample(freq, agg)` method. The grouping logic from the existing `Resample` is reused.

**Files:**
- Create: `data/downsample.go`
- Modify: `data/data_frame.go` (add `Downsample` constructor, keep `periodChanged` helper)
- Modify: `data/data_frame_test.go` (update existing Resample tests)

- [ ] **Step 1: Write failing tests**

Replace the entire existing "Resample" `Describe` block in `data/data_frame_test.go` with:

```go
Describe("Downsample", func() {
    var weeklyDF *data.DataFrame
    var aapl asset.Asset

    BeforeEach(func() {
        aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
        base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
        t := make([]time.Time, 10)
        vals := make([]float64, 10)
        for i := range t {
            t[i] = base.AddDate(0, 0, i)
            vals[i] = float64(i + 1)
        }
        var err error
        weeklyDF, err = data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
        Expect(err).NotTo(HaveOccurred())
    })

    It("Last picks last value per week", func() {
        result := weeklyDF.Downsample(data.Weekly).Last()
        Expect(result.Len()).To(Equal(2))
        col := result.Column(aapl, data.Price)
        Expect(col[0]).To(Equal(7.0))
        Expect(col[1]).To(Equal(10.0))
    })

    It("First picks first value per week", func() {
        result := weeklyDF.Downsample(data.Weekly).First()
        col := result.Column(aapl, data.Price)
        Expect(col[0]).To(Equal(1.0))
        Expect(col[1]).To(Equal(8.0))
    })

    It("Mean computes mean per week", func() {
        result := weeklyDF.Downsample(data.Weekly).Mean()
        col := result.Column(aapl, data.Price)
        Expect(col[0]).To(Equal(4.0))
        Expect(col[1]).To(Equal(9.0))
    })

    It("Max picks max per week", func() {
        result := weeklyDF.Downsample(data.Weekly).Max()
        col := result.Column(aapl, data.Price)
        Expect(col[0]).To(Equal(7.0))
        Expect(col[1]).To(Equal(10.0))
    })

    It("Min picks min per week", func() {
        result := weeklyDF.Downsample(data.Weekly).Min()
        col := result.Column(aapl, data.Price)
        Expect(col[0]).To(Equal(1.0))
        Expect(col[1]).To(Equal(8.0))
    })

    It("Sum sums values per month", func() {
        base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
        t := make([]time.Time, 60)
        vals := make([]float64, 60)
        for i := range t {
            t[i] = base.AddDate(0, 0, i)
            vals[i] = 1.0
        }
        monthDF, err := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
        Expect(err).NotTo(HaveOccurred())
        result := monthDF.Downsample(data.Monthly).Sum()
        Expect(result.Len()).To(Equal(2))
        col := result.Column(aapl, data.Price)
        Expect(col[0]).To(Equal(31.0))
        Expect(col[1]).To(Equal(29.0))
    })

    It("Std uses sample (N-1) denominator", func() {
        // [1,2,3,4,5,6,7] in one week: mean=4, sum sq diffs=28, var=28/6, std=sqrt(28/6)
        result := weeklyDF.Downsample(data.Weekly).Std()
        col := result.Column(aapl, data.Price)
        expectedStd := math.Sqrt(28.0 / 6.0)
        Expect(col[0]).To(BeNumerically("~", expectedStd, 1e-12))
    })

    It("Variance uses sample (N-1) denominator", func() {
        result := weeklyDF.Downsample(data.Weekly).Variance()
        col := result.Column(aapl, data.Price)
        Expect(col[0]).To(BeNumerically("~", 28.0/6.0, 1e-12))
    })

    It("on empty frame returns empty", func() {
        empty, err := data.NewDataFrame(nil, nil, nil, nil)
        Expect(err).NotTo(HaveOccurred())
        result := empty.Downsample(data.Weekly).Last()
        Expect(result.Len()).To(Equal(0))
    })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./data/... -run "Downsample" -v`
Expected: FAIL -- `Downsample` method not defined

- [ ] **Step 3: Create downsample.go**

Create `data/downsample.go`:

```go
package data

import (
    "math"
    "time"

    "github.com/penny-vault/pvbt/asset"
    "gonum.org/v1/gonum/floats"
    "gonum.org/v1/gonum/stat"
)

// DownsampledDataFrame groups timestamps by the target frequency and
// aggregates values within each period. Created by DataFrame.Downsample(freq).
type DownsampledDataFrame struct {
    df   *DataFrame
    freq Frequency
}

func (d *DownsampledDataFrame) aggregate(fn func([]float64) float64) *DataFrame {
    if len(d.df.times) == 0 {
        return mustNewDataFrame(nil, nil, nil, nil)
    }

    type group struct {
        start int
        end   int
    }

    var groups []group
    groupStart := 0

    for i := 1; i < len(d.df.times); i++ {
        if periodChanged(d.df.times[i-1], d.df.times[i], d.freq) {
            groups = append(groups, group{groupStart, i})
            groupStart = i
        }
    }
    groups = append(groups, group{groupStart, len(d.df.times)})

    newTimeLen := len(groups)
    assetLen := len(d.df.assets)
    metricLen := len(d.df.metrics)
    newData := make([]float64, assetLen*metricLen*newTimeLen)
    newTimes := make([]time.Time, newTimeLen)

    for gIdx, g := range groups {
        newTimes[gIdx] = d.df.times[g.end-1]

        for aIdx := 0; aIdx < assetLen; aIdx++ {
            for mIdx := 0; mIdx < metricLen; mIdx++ {
                srcOff := d.df.colOffset(aIdx, mIdx)
                vals := d.df.data[srcOff+g.start : srcOff+g.end]
                dstOff := (aIdx*metricLen + mIdx) * newTimeLen
                newData[dstOff+gIdx] = fn(vals)
            }
        }
    }

    assets := make([]asset.Asset, assetLen)
    copy(assets, d.df.assets)
    metrics := make([]Metric, metricLen)
    copy(metrics, d.df.metrics)

    return mustNewDataFrame(newTimes, assets, metrics, newData)
}

func (d *DownsampledDataFrame) Mean() *DataFrame {
    return d.aggregate(func(vals []float64) float64 {
        return stat.Mean(vals, nil)
    })
}

func (d *DownsampledDataFrame) Sum() *DataFrame {
    return d.aggregate(func(vals []float64) float64 {
        return floats.Sum(vals)
    })
}

func (d *DownsampledDataFrame) Max() *DataFrame {
    return d.aggregate(func(vals []float64) float64 {
        return floats.Max(vals)
    })
}

func (d *DownsampledDataFrame) Min() *DataFrame {
    return d.aggregate(func(vals []float64) float64 {
        return floats.Min(vals)
    })
}

func (d *DownsampledDataFrame) First() *DataFrame {
    return d.aggregate(func(vals []float64) float64 {
        return vals[0]
    })
}

func (d *DownsampledDataFrame) Last() *DataFrame {
    return d.aggregate(func(vals []float64) float64 {
        return vals[len(vals)-1]
    })
}

func (d *DownsampledDataFrame) Std() *DataFrame {
    return d.aggregate(func(vals []float64) float64 {
        n := len(vals)
        if n < 2 {
            return 0
        }
        m := stat.Mean(vals, nil)
        sum := 0.0
        for _, v := range vals {
            diff := v - m
            sum += diff * diff
        }
        return math.Sqrt(sum / float64(n-1))
    })
}

func (d *DownsampledDataFrame) Variance() *DataFrame {
    return d.aggregate(func(vals []float64) float64 {
        n := len(vals)
        if n < 2 {
            return 0
        }
        m := stat.Mean(vals, nil)
        sum := 0.0
        for _, v := range vals {
            diff := v - m
            sum += diff * diff
        }
        return sum / float64(n-1)
    })
}
```

- [ ] **Step 4: Add Downsample constructor to data_frame.go**

Add to `data/data_frame.go`:

```go
// Downsample returns a DownsampledDataFrame that aggregates values when
// converting to a lower frequency.
func (df *DataFrame) Downsample(freq Frequency) *DownsampledDataFrame {
    return &DownsampledDataFrame{df: df, freq: freq}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./data/... -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```
git add data/downsample.go data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add Downsample builder replacing Resample for downsampling"
```

---

### Task 7: Create UpsampledDataFrame builder

**Files:**
- Create: `data/upsample.go`
- Modify: `data/data_frame.go` (add `Upsample` constructor)
- Modify: `data/data_frame_test.go`

- [ ] **Step 1: Write failing tests**

Add to `data/data_frame_test.go`:

```go
Describe("Upsample", func() {
    var monthlyDF *data.DataFrame
    var aapl asset.Asset

    BeforeEach(func() {
        aapl = asset.Asset{CompositeFigi: "AAPL", Ticker: "AAPL"}
        t := []time.Time{
            time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
            time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
            time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
        }
        vals := []float64{100, 200, 300}
        var err error
        monthlyDF, err = data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
        Expect(err).NotTo(HaveOccurred())
    })

    Describe("ForwardFill", func() {
        It("carries last known value forward to fill gaps", func() {
            result := monthlyDF.Upsample(data.Weekly).ForwardFill()
            // Should have weekly timestamps between Jan 1 and Mar 1
            Expect(result.Len()).To(BeNumerically(">", 3))

            // First value should be 100 (Jan 1)
            col := result.Column(aapl, data.Price)
            Expect(col[0]).To(Equal(100.0))

            // Values between Jan and Feb should be forward-filled as 100
            for i := 0; i < len(col); i++ {
                t := result.Times()[i]
                if t.Before(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)) {
                    Expect(col[i]).To(Equal(100.0))
                }
            }
        })
    })

    Describe("BackFill", func() {
        It("uses next known value to fill gaps", func() {
            result := monthlyDF.Upsample(data.Weekly).BackFill()
            col := result.Column(aapl, data.Price)
            times := result.Times()

            // Values before Feb 1 (but after first) should be back-filled with 200
            for i := 1; i < len(col); i++ {
                if times[i].Before(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)) {
                    Expect(col[i]).To(Equal(200.0))
                }
            }
        })
    })

    Describe("Interpolate", func() {
        It("linearly interpolates between known values", func() {
            // Use daily with simple monthly data for easy verification
            t := []time.Time{
                time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
                time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC),
            }
            vals := []float64{0, 100}
            simple, err := data.NewDataFrame(t, []asset.Asset{aapl}, []data.Metric{data.Price}, vals)
            Expect(err).NotTo(HaveOccurred())
            result := simple.Upsample(data.Daily).Interpolate()

            col := result.Column(aapl, data.Price)
            Expect(col[0]).To(Equal(0.0))
            Expect(col[len(col)-1]).To(Equal(100.0))
            // Middle values should be linearly interpolated
            Expect(result.Len()).To(Equal(11)) // Jan 1 through Jan 11
            Expect(col[5]).To(BeNumerically("~", 50.0, 1e-12))
        })
    })

    It("on empty frame returns empty", func() {
        empty, err := data.NewDataFrame(nil, nil, nil, nil)
        Expect(err).NotTo(HaveOccurred())
        result := empty.Upsample(data.Daily).ForwardFill()
        Expect(result.Len()).To(Equal(0))
    })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./data/... -run "Upsample" -v`
Expected: FAIL -- `Upsample` method not defined

- [ ] **Step 3: Create upsample.go**

Create `data/upsample.go`. The builder generates timestamps at the target frequency between existing timestamps, then fills values using the chosen method.

```go
package data

import (
    "time"
)

// UpsampledDataFrame fills gaps when converting to a higher frequency.
// Created by DataFrame.Upsample(freq).
type UpsampledDataFrame struct {
    df   *DataFrame
    freq Frequency
}

// generateTimes creates a time axis at the target frequency spanning
// from the first to the last timestamp of the source DataFrame.
func (u *UpsampledDataFrame) generateTimes() []time.Time {
    if len(u.df.times) < 2 {
        return u.df.times
    }

    start := u.df.times[0]
    end := u.df.times[len(u.df.times)-1]
    var times []time.Time

    for t := start; !t.After(end); {
        times = append(times, t)
        switch u.freq {
        case Daily:
            t = t.AddDate(0, 0, 1)
        case Weekly:
            t = t.AddDate(0, 0, 7)
        case Monthly:
            t = t.AddDate(0, 1, 0)
        case Quarterly:
            t = t.AddDate(0, 3, 0)
        case Yearly:
            t = t.AddDate(1, 0, 0)
        default:
            t = t.AddDate(0, 0, 1) // default to daily
        }
    }

    return times
}

func (u *UpsampledDataFrame) ForwardFill() *DataFrame {
    if len(u.df.times) == 0 {
        return mustNewDataFrame(nil, nil, nil, nil)
    }

    newTimes := u.generateTimes()
    assetLen := len(u.df.assets)
    metricLen := len(u.df.metrics)
    newData := make([]float64, assetLen*metricLen*len(newTimes))

    for aIdx := 0; aIdx < assetLen; aIdx++ {
        for mIdx := 0; mIdx < metricLen; mIdx++ {
            srcCol := u.df.colSlice(aIdx, mIdx)
            dstOff := (aIdx*metricLen + mIdx) * len(newTimes)
            srcIdx := 0

            for i, t := range newTimes {
                // Advance source index to the latest timestamp <= t
                for srcIdx < len(u.df.times)-1 && !u.df.times[srcIdx+1].After(t) {
                    srcIdx++
                }
                newData[dstOff+i] = srcCol[srcIdx]
            }
        }
    }

    assets := make([]asset.Asset, assetLen)
    copy(assets, u.df.assets)
    metrics := make([]Metric, metricLen)
    copy(metrics, u.df.metrics)

    return mustNewDataFrame(newTimes, assets, metrics, newData)
}

func (u *UpsampledDataFrame) BackFill() *DataFrame {
    if len(u.df.times) == 0 {
        return mustNewDataFrame(nil, nil, nil, nil)
    }

    newTimes := u.generateTimes()
    assetLen := len(u.df.assets)
    metricLen := len(u.df.metrics)
    newData := make([]float64, assetLen*metricLen*len(newTimes))

    for aIdx := 0; aIdx < assetLen; aIdx++ {
        for mIdx := 0; mIdx < metricLen; mIdx++ {
            srcCol := u.df.colSlice(aIdx, mIdx)
            dstOff := (aIdx*metricLen + mIdx) * len(newTimes)
            srcIdx := 0

            for i, t := range newTimes {
                // Advance source index to the earliest timestamp >= t
                for srcIdx < len(u.df.times)-1 && u.df.times[srcIdx].Before(t) {
                    srcIdx++
                }
                newData[dstOff+i] = srcCol[srcIdx]
            }
        }
    }

    assets := make([]asset.Asset, assetLen)
    copy(assets, u.df.assets)
    metrics := make([]Metric, metricLen)
    copy(metrics, u.df.metrics)

    return mustNewDataFrame(newTimes, assets, metrics, newData)
}

func (u *UpsampledDataFrame) Interpolate() *DataFrame {
    if len(u.df.times) == 0 {
        return mustNewDataFrame(nil, nil, nil, nil)
    }

    newTimes := u.generateTimes()
    assetLen := len(u.df.assets)
    metricLen := len(u.df.metrics)
    newData := make([]float64, assetLen*metricLen*len(newTimes))

    for aIdx := 0; aIdx < assetLen; aIdx++ {
        for mIdx := 0; mIdx < metricLen; mIdx++ {
            srcCol := u.df.colSlice(aIdx, mIdx)
            dstOff := (aIdx*metricLen + mIdx) * len(newTimes)
            srcIdx := 0

            for i, t := range newTimes {
                // Find surrounding source timestamps.
                for srcIdx < len(u.df.times)-1 && u.df.times[srcIdx+1].Before(t) {
                    srcIdx++
                }

                if srcIdx >= len(u.df.times)-1 || t.Equal(u.df.times[srcIdx]) {
                    newData[dstOff+i] = srcCol[srcIdx]
                } else {
                    // Linear interpolation.
                    t0 := u.df.times[srcIdx]
                    t1 := u.df.times[srcIdx+1]
                    v0 := srcCol[srcIdx]
                    v1 := srcCol[srcIdx+1]
                    frac := float64(t.Sub(t0)) / float64(t1.Sub(t0))
                    newData[dstOff+i] = v0 + frac*(v1-v0)
                }
            }
        }
    }

    assets := make([]asset.Asset, assetLen)
    copy(assets, u.df.assets)
    metrics := make([]Metric, metricLen)
    copy(metrics, u.df.metrics)

    return mustNewDataFrame(newTimes, assets, metrics, newData)
}
```

- [ ] **Step 4: Add Upsample constructor to data_frame.go**

Add to `data/data_frame.go`:

```go
// Upsample returns an UpsampledDataFrame that fills gaps when converting
// to a higher frequency.
func (df *DataFrame) Upsample(freq Frequency) *UpsampledDataFrame {
    return &UpsampledDataFrame{df: df, freq: freq}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./data/... -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```
git add data/upsample.go data/data_frame.go data/data_frame_test.go
git commit -m "feat(data): add Upsample builder with ForwardFill, BackFill, Interpolate"
```

---

## Chunk 3: Cleanup and verification

### Task 8: Delete Aggregation type and old Resample

**Files:**
- Delete: `data/aggregation.go`
- Modify: `data/data_frame.go` (remove `Resample` method, `aggregate` helper)
- Modify: `data/data_frame_test.go` (remove old Resample tests, Aggregation string tests)

- [ ] **Step 1: Remove old Resample method and aggregate helper from data_frame.go**

Delete the `Resample(freq Frequency, agg Aggregation) *DataFrame` method (lines ~910-961 of `data/data_frame.go`).

Delete the `aggregate(vals []float64, agg Aggregation) float64` helper (lines ~980-997).

Keep `periodChanged` -- it's still used by the Downsample builder.

- [ ] **Step 2: Delete aggregation.go**

```
rm data/aggregation.go
```

- [ ] **Step 3: Remove old Resample and Aggregation tests from data_frame_test.go**

Delete:
- Any remaining `Resample(data.Weekly, data.Last)` style tests that weren't already replaced in Task 6
- The `Aggregation.String()` test block (around line 1154)
- The unknown aggregation test

Remove `data.Last`, `data.First`, `data.Sum`, `data.Mean`, `data.Max`, `data.Min`, `data.Aggregation` references from test imports.

- [ ] **Step 4: Verify compilation and tests pass**

Run: `go build ./data/...`
Run: `go test ./data/... -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```
git add -A
git commit -m "refactor(data): delete Aggregation type, old Resample method, and aggregate helper"
```

---

### Task 9: Clean up stats.go -- remove functions replaced by DataFrame methods

**Files:**
- Modify: `data/stats.go`
- Modify: `data/stats_test.go`

- [ ] **Step 1: Remove replaced functions from stats.go**

Delete from `data/stats.go`:
- `SliceMean`
- `Variance`
- `Stddev`
- `Covariance`
- `PeriodsReturns`

Keep only `AnnualizationFactor`. Remove unused imports (`math` may no longer be needed).

- [ ] **Step 2: Remove corresponding tests from stats_test.go**

Delete the test blocks for `SliceMean`, `Variance`, `Stddev`, `Covariance`, `PeriodsReturns` from `data/stats_test.go`. Keep only the `AnnualizationFactor` tests.

- [ ] **Step 3: Verify compilation and tests pass**

Run: `go build ./...`
Expected: May fail if portfolio package still references `data.SliceMean` etc. That's OK -- those will be fixed in the metric helpers refactor. For now, just verify the data package compiles:
Run: `go build ./data/...`
Run: `go test ./data/... -v`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```
git add data/stats.go data/stats_test.go
git commit -m "refactor(data): remove standalone stats functions replaced by DataFrame methods"
```

---

### Task 10: Full verification

- [ ] **Step 1: Build the data package**

Run: `go build ./data/...`
Expected: SUCCESS

- [ ] **Step 2: Run all data package tests**

Run: `go test ./data/... -v`
Expected: ALL PASS

- [ ] **Step 3: Verify no remaining references to deleted identifiers in data package**

Run: `grep -rn 'Aggregation\|\.Resample(' data/*.go | grep -v _test.go`
Expected: NO OUTPUT (all removed)

Run: `grep -rn 'SliceMean\|data\.Variance\|data\.Stddev\|data\.Covariance\|PeriodsReturns' data/*.go`
Expected: NO OUTPUT (all removed from stats.go)

- [ ] **Step 4: Verify the API surface**

Confirm the data package exports these new items:
- `DataFrame.Mean()`, `.Std()`, `.Variance()`, `.Sum()`, `.Max()`, `.Min()`
- `DataFrame.Covariance(assets ...asset.Asset)`
- `DataFrame.MaxAcrossAssets()`, `.MinAcrossAssets()`, `.IdxMaxAcrossAssets()`
- `DataFrame.Downsample(freq)` returning `*DownsampledDataFrame`
- `DataFrame.Upsample(freq)` returning `*UpsampledDataFrame`
- `DownsampledDataFrame.Mean()`, `.Sum()`, `.Max()`, `.Min()`, `.Std()`, `.Variance()`, `.First()`, `.Last()`
- `UpsampledDataFrame.ForwardFill()`, `.BackFill()`, `.Interpolate()`
- `RollingDataFrame.Variance()` (new)
- `CompositeAsset()`, `CompositeMetric()`

Run: `go doc ./data/ | grep -E "func.*DataFrame"` to verify

- [ ] **Step 5: Final commit if any stragglers**

```
git add -A
git commit -m "chore: clean up after DataFrame stats API refactor"
```
