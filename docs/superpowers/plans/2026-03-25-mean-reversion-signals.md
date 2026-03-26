# Mean Reversion Signals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add five mean reversion signals (Z-Score, Hurst R/S, Hurst DFA, PairsResidual, PairsRatio) to the `signal/` package.

**Architecture:** Each signal is a standalone function following the existing pattern: take a `universe.Universe`, return a single-row `*data.DataFrame`. Z-Score and the two Hurst variants use the standard single-universe signature. The two pairs signals add a second `universe.Universe` parameter for reference assets. A shared unexported `zScore` helper is extracted for reuse across Z-Score, PairsResidual, and PairsRatio.

**Tech Stack:** Go, Ginkgo/Gomega (tests), existing `data.DataFrame` and `universe.Universe` APIs.

---

## Design Decisions

1. **Shared z-score helper:** Z-Score, PairsResidual, and PairsRatio all need to compute the z-score of a `[]float64` slice. An unexported `zScore(values []float64) (float64, error)` function avoids duplication. It returns an error when stddev is zero.

2. **Hurst sub-period sizes:** Both Hurst signals need a set of sub-period sizes `n` to regress against. The sizes are powers of 2 from 4 up to `len(series)/2`. This ensures at least 2 segments per size and gives enough points for a meaningful regression.

3. **Pairs metric naming:** Output metrics are `PairsResidual_{Ticker}` and `PairsRatio_{Ticker}`, using the reference asset's `Ticker` field. This produces readable metric names like `PairsResidual_SPY`.

4. **OLS regression:** Implemented inline as simple least-squares (slope and intercept via the normal equations). No external library needed for two-variable regression.

## File Structure

| File | Responsibility |
|------|---------------|
| `signal/helpers.go` | Unexported `zScore` and `linRegress` helpers |
| `signal/zscore.go` | Z-Score signal |
| `signal/zscore_test.go` | Z-Score tests |
| `signal/hurst_rs.go` | Hurst R/S signal |
| `signal/hurst_rs_test.go` | Hurst R/S tests |
| `signal/hurst_dfa.go` | Hurst DFA signal |
| `signal/hurst_dfa_test.go` | Hurst DFA tests |
| `signal/pairs_residual.go` | Pairs residual signal |
| `signal/pairs_residual_test.go` | Pairs residual tests |
| `signal/pairs_ratio.go` | Pairs ratio signal |
| `signal/pairs_ratio_test.go` | Pairs ratio tests |

Existing files modified:
- `signal/doc.go` -- add new signals to the catalog

---

## Task 1: Shared Helpers

**Files:**
- Create: `signal/helpers.go`

- [ ] **Step 1: Create helpers.go with zScore and linRegress**

```go
package signal

import (
	"fmt"
	"math"
)

// zScore computes the z-score of the last element in values relative to the
// mean and standard deviation of the full slice. Returns an error if the
// standard deviation is zero (constant series) or the slice has fewer than
// 2 elements.
func zScore(values []float64) (float64, error) {
	nn := len(values)
	if nn < 2 {
		return 0, fmt.Errorf("zScore: need at least 2 values, got %d", nn)
	}

	sum := 0.0
	for _, vv := range values {
		sum += vv
	}

	mean := sum / float64(nn)

	sumSq := 0.0
	for _, vv := range values {
		diff := vv - mean
		sumSq += diff * diff
	}

	stddev := math.Sqrt(sumSq / float64(nn))
	if stddev == 0 {
		return 0, fmt.Errorf("zScore: standard deviation is zero (constant series)")
	}

	return (values[nn-1] - mean) / stddev, nil
}

// linRegress performs simple linear regression of yy on xx, returning the
// slope and intercept. Both slices must have the same length (>= 2).
func linRegress(xx, yy []float64) (slope, intercept float64, err error) {
	nn := len(xx)
	if nn < 2 {
		return 0, 0, fmt.Errorf("linRegress: need at least 2 points, got %d", nn)
	}

	if len(yy) != nn {
		return 0, 0, fmt.Errorf("linRegress: x and y lengths differ (%d vs %d)", nn, len(yy))
	}

	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0

	for ii := range nn {
		sumX += xx[ii]
		sumY += yy[ii]
		sumXY += xx[ii] * yy[ii]
		sumX2 += xx[ii] * xx[ii]
	}

	nf := float64(nn)
	denom := nf*sumX2 - sumX*sumX

	if denom == 0 {
		return 0, 0, fmt.Errorf("linRegress: all x values are identical")
	}

	slope = (nf*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / nf

	return slope, intercept, nil
}
```

- [ ] **Step 2: Verify the file compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./signal/...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add signal/helpers.go
git commit -m "feat(signal): add shared zScore and linRegress helpers (#27)"
```

---

## Task 2: Z-Score Signal

**Files:**
- Create: `signal/zscore.go`
- Create: `signal/zscore_test.go`

- [ ] **Step 1: Write the failing test**

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

var _ = Describe("ZScore", func() {
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

	It("computes hand-calculated z-score correctly", func() {
		// Prices: 100, 102, 98, 104, 106
		// Mean = (100+102+98+104+106)/5 = 102
		// Variance = ((100-102)^2 + (102-102)^2 + (98-102)^2 + (104-102)^2 + (106-102)^2) / 5
		//          = (4 + 0 + 16 + 4 + 16) / 5 = 8
		// StdDev = sqrt(8) = 2.8284...
		// Z = (106 - 102) / 2.8284... = 1.4142...
		prices := []float64{100, 102, 98, 104, 106}
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.ZScoreSignal}))
		Expect(result.Value(aapl, signal.ZScoreSignal)).To(BeNumerically("~", math.Sqrt(2), 1e-10))
	})

	It("computes independently per asset", func() {
		// AAPL: 100, 100, 100, 100, 110 => mean=102, stddev>0, z>0
		// GOOG: 200, 200, 200, 200, 190 => mean=198, stddev>0, z<0
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}

		vals := [][]float64{
			{100, 100, 100, 100, 110},
			{200, 200, 200, 200, 190},
		}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.ZScoreSignal)).To(BeNumerically(">", 0))
		Expect(result.Value(goog, signal.ZScoreSignal)).To(BeNumerically("<", 0))
	})

	It("uses custom metric when provided", func() {
		prices := []float64{50, 60, 70}
		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.AdjClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(2), data.AdjClose)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.ZScoreSignal)).To(BeNumerically(">", 0))
	})

	It("returns error on constant price series (zero stddev)", func() {
		prices := []float64{100, 100, 100}
		times := make([]time.Time, 3)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-2)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(2))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("zero"))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now}
		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(0))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.ZScore(ctx, uu, portfolio.Days(10))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "ZScore" ./signal/...`
Expected: compilation failure (ZScore not defined)

- [ ] **Step 3: Write the implementation**

```go
package signal

import (
	"context"
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// ZScoreSignal is the metric name for z-score signal output.
const ZScoreSignal data.Metric = "ZScore"

// ZScore computes the z-score of each asset's current value relative to its
// rolling mean and standard deviation over the lookback period. Positive values
// indicate the asset is above its rolling mean; negative values indicate below.
// Values beyond +/-2 indicate significant deviation. Returns a single-row
// DataFrame with [ZScoreSignal].
func ZScore(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, metrics ...data.Metric) *data.DataFrame {
	metric := data.MetricClose
	if len(metrics) > 0 {
		metric = metrics[0]
	}

	df, err := assetUniverse.Window(ctx, period, metric)
	if err != nil {
		return data.WithErr(fmt.Errorf("ZScore: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("ZScore: need at least 2 data points, got %d", df.Len()))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	zCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		col := df.Column(aa, metric)

		zz, zErr := zScore(col)
		if zErr != nil {
			return data.WithErr(fmt.Errorf("ZScore [%s]: %w", aa.Ticker, zErr))
		}

		zCols[ii] = []float64{zz}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{ZScoreSignal}, df.Frequency(), zCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("ZScore: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "ZScore" ./signal/...`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add signal/zscore.go signal/zscore_test.go
git commit -m "feat(signal): add ZScore mean reversion signal (#27)"
```

---

## Task 3: Hurst Exponent (R/S)

**Files:**
- Create: `signal/hurst_rs.go`
- Create: `signal/hurst_rs_test.go`

- [ ] **Step 1: Write the failing test**

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

var _ = Describe("HurstRS", func() {
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

	It("produces H > 0.5 for a trending series", func() {
		// Monotonically rising prices => strongly trending.
		prices := make([]float64, 64)
		for ii := range prices {
			prices[ii] = 100.0 + float64(ii)*2.0
		}

		times := make([]time.Time, 64)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-63)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstRS(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.HurstRSSignal}))
		Expect(result.Value(aapl, signal.HurstRSSignal)).To(BeNumerically(">", 0.5))
	})

	It("produces H < 0.5 for a mean-reverting series", func() {
		// Alternating up/down prices => mean-reverting.
		prices := make([]float64, 64)
		for ii := range prices {
			if ii%2 == 0 {
				prices[ii] = 100.0
			} else {
				prices[ii] = 110.0
			}
		}

		times := make([]time.Time, 64)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-63)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstRS(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.HurstRSSignal)).To(BeNumerically("<", 0.5))
	})

	It("produces value between 0 and 1", func() {
		// Arbitrary price series.
		prices := []float64{100, 103, 97, 105, 99, 108, 95, 110, 100, 102,
			98, 106, 94, 112, 101, 99, 107, 93, 111, 100,
			104, 96, 108, 92, 113, 101, 97, 109, 91, 114, 100, 105}

		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstRS(ctx, uu, portfolio.Days(int64(len(prices)-1)))
		Expect(result.Err()).NotTo(HaveOccurred())

		hh := result.Value(aapl, signal.HurstRSSignal)
		Expect(hh).To(BeNumerically(">=", 0))
		Expect(hh).To(BeNumerically("<=", 1))
	})

	It("computes independently per asset", func() {
		// AAPL trending, GOOG mean-reverting.
		aaplPrices := make([]float64, 64)
		googPrices := make([]float64, 64)
		for ii := range 64 {
			aaplPrices[ii] = 100.0 + float64(ii)*2.0
			if ii%2 == 0 {
				googPrices[ii] = 100.0
			} else {
				googPrices[ii] = 110.0
			}
		}

		times := make([]time.Time, 64)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-63)
		}

		vals := [][]float64{aaplPrices, googPrices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.HurstRS(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.HurstRSSignal)).To(BeNumerically(">", 0.5))
		Expect(result.Value(goog, signal.HurstRSSignal)).To(BeNumerically("<", 0.5))
	})

	It("returns error on insufficient data", func() {
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}

		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100, 101, 102, 103, 104}})
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		// 5 data points yields 4 returns, which is not enough for multiple sub-period sizes.
		result := signal.HurstRS(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstRS(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "HurstRS" ./signal/...`
Expected: compilation failure (HurstRS not defined)

- [ ] **Step 3: Write the implementation**

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

// HurstRSSignal is the metric name for Hurst exponent (R/S) output.
const HurstRSSignal data.Metric = "HurstRS"

// HurstRS computes the Hurst exponent via Rescaled Range (R/S) analysis for
// each asset in the universe. H < 0.5 indicates mean reversion, H = 0.5
// indicates a random walk, and H > 0.5 indicates trending behavior. Returns a
// single-row DataFrame with [HurstRSSignal].
func HurstRS(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("HurstRS: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("HurstRS: need at least 2 data points, got %d", df.Len()))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	hCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		col := df.Column(aa, data.MetricClose)

		hh, hErr := hurstRS(col)
		if hErr != nil {
			return data.WithErr(fmt.Errorf("HurstRS [%s]: %w", aa.Ticker, hErr))
		}

		hCols[ii] = []float64{hh}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{HurstRSSignal}, df.Frequency(), hCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("HurstRS: %w", err))
	}

	return result
}

// hurstRS computes the Hurst exponent from a price series using R/S analysis.
func hurstRS(prices []float64) (float64, error) {
	nn := len(prices)
	if nn < 2 {
		return 0, fmt.Errorf("need at least 2 prices, got %d", nn)
	}

	// Compute returns.
	returns := make([]float64, nn-1)
	for ii := 1; ii < nn; ii++ {
		if prices[ii-1] == 0 {
			return 0, fmt.Errorf("zero price at index %d", ii-1)
		}

		returns[ii-1] = (prices[ii] - prices[ii-1]) / prices[ii-1]
	}

	// Generate sub-period sizes: powers of 2 from 4 up to len(returns)/2.
	var sizes []int

	for ss := 4; ss <= len(returns)/2; ss *= 2 {
		sizes = append(sizes, ss)
	}

	if len(sizes) < 2 {
		return 0, fmt.Errorf("insufficient data for R/S analysis: need at least 20 data points")
	}

	logN := make([]float64, len(sizes))
	logRS := make([]float64, len(sizes))

	for si, ss := range sizes {
		numSegments := len(returns) / ss
		rsSum := 0.0

		for seg := range numSegments {
			start := seg * ss
			segment := returns[start : start+ss]

			// Mean of segment.
			segMean := 0.0
			for _, rr := range segment {
				segMean += rr
			}

			segMean /= float64(ss)

			// Cumulative deviations from mean.
			cumDev := make([]float64, ss)
			cumDev[0] = segment[0] - segMean

			for jj := 1; jj < ss; jj++ {
				cumDev[jj] = cumDev[jj-1] + (segment[jj] - segMean)
			}

			// Range of cumulative deviations.
			maxDev := cumDev[0]
			minDev := cumDev[0]

			for _, dd := range cumDev[1:] {
				if dd > maxDev {
					maxDev = dd
				}

				if dd < minDev {
					minDev = dd
				}
			}

			rangeVal := maxDev - minDev

			// Standard deviation of segment.
			segVarSum := 0.0
			for _, rr := range segment {
				diff := rr - segMean
				segVarSum += diff * diff
			}

			stddev := math.Sqrt(segVarSum / float64(ss))

			if stddev > 0 {
				rsSum += rangeVal / stddev
			}
		}

		logN[si] = math.Log(float64(ss))
		logRS[si] = math.Log(rsSum / float64(numSegments))
	}

	slope, _, regErr := linRegress(logN, logRS)
	if regErr != nil {
		return 0, fmt.Errorf("R/S regression: %w", regErr)
	}

	return slope, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "HurstRS" ./signal/...`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add signal/hurst_rs.go signal/hurst_rs_test.go
git commit -m "feat(signal): add HurstRS mean reversion signal (#27)"
```

---

## Task 4: Hurst Exponent (DFA)

**Files:**
- Create: `signal/hurst_dfa.go`
- Create: `signal/hurst_dfa_test.go`

- [ ] **Step 1: Write the failing test**

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

var _ = Describe("HurstDFA", func() {
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

	It("produces H > 0.5 for a trending series", func() {
		prices := make([]float64, 64)
		for ii := range prices {
			prices[ii] = 100.0 + float64(ii)*2.0
		}

		times := make([]time.Time, 64)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-63)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstDFA(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{signal.HurstDFASignal}))
		Expect(result.Value(aapl, signal.HurstDFASignal)).To(BeNumerically(">", 0.5))
	})

	It("produces H < 0.5 for a mean-reverting series", func() {
		prices := make([]float64, 64)
		for ii := range prices {
			if ii%2 == 0 {
				prices[ii] = 100.0
			} else {
				prices[ii] = 110.0
			}
		}

		times := make([]time.Time, 64)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-63)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstDFA(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.HurstDFASignal)).To(BeNumerically("<", 0.5))
	})

	It("produces value between 0 and 1", func() {
		prices := []float64{100, 103, 97, 105, 99, 108, 95, 110, 100, 102,
			98, 106, 94, 112, 101, 99, 107, 93, 111, 100,
			104, 96, 108, 92, 113, 101, 97, 109, 91, 114, 100, 105}

		times := make([]time.Time, len(prices))
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-len(prices)+1)
		}

		df, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{prices})
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstDFA(ctx, uu, portfolio.Days(int64(len(prices)-1)))
		Expect(result.Err()).NotTo(HaveOccurred())

		hh := result.Value(aapl, signal.HurstDFASignal)
		Expect(hh).To(BeNumerically(">=", 0))
		Expect(hh).To(BeNumerically("<=", 2))
	})

	It("computes independently per asset", func() {
		aaplPrices := make([]float64, 64)
		googPrices := make([]float64, 64)
		for ii := range 64 {
			aaplPrices[ii] = 100.0 + float64(ii)*2.0
			if ii%2 == 0 {
				googPrices[ii] = 100.0
			} else {
				googPrices[ii] = 110.0
			}
		}

		times := make([]time.Time, 64)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-63)
		}

		vals := [][]float64{aaplPrices, googPrices}
		df, err := data.NewDataFrame(times, []asset.Asset{aapl, goog}, []data.Metric{data.MetricClose}, data.Daily, vals)
		Expect(err).NotTo(HaveOccurred())

		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl, goog}, ds)

		result := signal.HurstDFA(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Value(aapl, signal.HurstDFASignal)).To(BeNumerically(">", 0.5))
		Expect(result.Value(goog, signal.HurstDFASignal)).To(BeNumerically("<", 0.5))
	})

	It("returns error on insufficient data", func() {
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}

		df, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100, 101, 102, 103, 104}})
		ds := &mockDataSource{currentDate: now, fetchResult: df}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstDFA(ctx, uu, portfolio.Days(4))
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		uu := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)

		result := signal.HurstDFA(ctx, uu, portfolio.Days(63))
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "HurstDFA" ./signal/...`
Expected: compilation failure (HurstDFA not defined)

- [ ] **Step 3: Write the implementation**

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

// HurstDFASignal is the metric name for Hurst exponent (DFA) output.
const HurstDFASignal data.Metric = "HurstDFA"

// HurstDFA computes the Hurst exponent via Detrended Fluctuation Analysis for
// each asset in the universe. Interpretation is the same as [HurstRS]: H < 0.5
// indicates mean reversion, H = 0.5 indicates a random walk, and H > 0.5
// indicates trending behavior. DFA is more robust to short-term correlations
// than R/S analysis. Returns a single-row DataFrame with [HurstDFASignal].
func HurstDFA(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period) *data.DataFrame {
	df, err := assetUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("HurstDFA: %w", err))
	}

	if df.Len() < 2 {
		return data.WithErr(fmt.Errorf("HurstDFA: need at least 2 data points, got %d", df.Len()))
	}

	assets := df.AssetList()
	times := df.Times()
	lastTime := []time.Time{times[len(times)-1]}

	hCols := make([][]float64, len(assets))

	for ii, aa := range assets {
		col := df.Column(aa, data.MetricClose)

		hh, hErr := hurstDFA(col)
		if hErr != nil {
			return data.WithErr(fmt.Errorf("HurstDFA [%s]: %w", aa.Ticker, hErr))
		}

		hCols[ii] = []float64{hh}
	}

	result, err := data.NewDataFrame(lastTime, assets, []data.Metric{HurstDFASignal}, df.Frequency(), hCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("HurstDFA: %w", err))
	}

	return result
}

// hurstDFA computes the Hurst exponent from a price series using DFA.
func hurstDFA(prices []float64) (float64, error) {
	nn := len(prices)
	if nn < 2 {
		return 0, fmt.Errorf("need at least 2 prices, got %d", nn)
	}

	// Compute returns.
	returns := make([]float64, nn-1)
	for ii := 1; ii < nn; ii++ {
		if prices[ii-1] == 0 {
			return 0, fmt.Errorf("zero price at index %d", ii-1)
		}

		returns[ii-1] = (prices[ii] - prices[ii-1]) / prices[ii-1]
	}

	// Build cumulative deviation profile (integrate the mean-subtracted returns).
	retMean := 0.0
	for _, rr := range returns {
		retMean += rr
	}

	retMean /= float64(len(returns))

	profile := make([]float64, len(returns))
	profile[0] = returns[0] - retMean

	for ii := 1; ii < len(returns); ii++ {
		profile[ii] = profile[ii-1] + (returns[ii] - retMean)
	}

	// Generate window sizes: powers of 2 from 4 up to len(profile)/2.
	var sizes []int

	for ss := 4; ss <= len(profile)/2; ss *= 2 {
		sizes = append(sizes, ss)
	}

	if len(sizes) < 2 {
		return 0, fmt.Errorf("insufficient data for DFA: need at least 20 data points")
	}

	logN := make([]float64, len(sizes))
	logF := make([]float64, len(sizes))

	for si, ss := range sizes {
		numSegments := len(profile) / ss
		fluctSum := 0.0

		for seg := range numSegments {
			start := seg * ss
			segment := profile[start : start+ss]

			// Fit linear trend to the segment via least-squares.
			xx := make([]float64, ss)
			for jj := range ss {
				xx[jj] = float64(jj)
			}

			slope, intercept, regErr := linRegress(xx, segment)
			if regErr != nil {
				return 0, fmt.Errorf("DFA trend fit: %w", regErr)
			}

			// Compute RMS of detrended values.
			rmsSum := 0.0
			for jj := range ss {
				detrended := segment[jj] - (intercept + slope*float64(jj))
				rmsSum += detrended * detrended
			}

			fluctSum += math.Sqrt(rmsSum / float64(ss))
		}

		logN[si] = math.Log(float64(ss))
		logF[si] = math.Log(fluctSum / float64(numSegments))
	}

	slope, _, regErr := linRegress(logN, logF)
	if regErr != nil {
		return 0, fmt.Errorf("DFA regression: %w", regErr)
	}

	return slope, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "HurstDFA" ./signal/...`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add signal/hurst_dfa.go signal/hurst_dfa_test.go
git commit -m "feat(signal): add HurstDFA mean reversion signal (#27)"
```

---

## Task 5: Pairs Residual Signal

**Files:**
- Create: `signal/pairs_residual.go`
- Create: `signal/pairs_residual_test.go`

- [ ] **Step 1: Write the failing test**

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

var _ = Describe("PairsResidual", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		msft asset.Asset
		spy  asset.Asset
		efa  asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		spy = asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		efa = asset.Asset{CompositeFigi: "FIGI-EFA", Ticker: "EFA"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("produces a z-score of residuals against a single reference", func() {
		times := make([]time.Time, 20)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-19)
		}

		// AAPL tracks SPY closely at first, then diverges up.
		aaplPrices := make([]float64, 20)
		spyPrices := make([]float64, 20)
		for ii := range 20 {
			spyPrices[ii] = 100.0 + float64(ii)*0.5
			aaplPrices[ii] = 100.0 + float64(ii)*0.5
		}

		aaplPrices[19] = 130.0 // Diverge up at the end.

		aaplDF, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices})
		Expect(err).NotTo(HaveOccurred())

		spyDF, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{"PairsResidual_SPY"}))

		// AAPL diverged up from SPY, so the residual z-score should be positive.
		Expect(result.Value(aapl, "PairsResidual_SPY")).To(BeNumerically(">", 0))
	})

	It("produces metrics for multiple reference assets", func() {
		times := make([]time.Time, 20)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-19)
		}

		aaplPrices := make([]float64, 20)
		spyPrices := make([]float64, 20)
		efaPrices := make([]float64, 20)
		for ii := range 20 {
			aaplPrices[ii] = 100.0 + float64(ii)*1.0
			spyPrices[ii] = 100.0 + float64(ii)*0.8
			efaPrices[ii] = 50.0 + float64(ii)*0.3
		}

		aaplDF, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices})
		Expect(err).NotTo(HaveOccurred())

		refDF, err := data.NewDataFrame(times, []asset.Asset{spy, efa}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices, efaPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: refDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy, efa}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))

		metricList := result.MetricList()
		Expect(metricList).To(HaveLen(2))
		Expect(metricList).To(ContainElement(data.Metric("PairsResidual_SPY")))
		Expect(metricList).To(ContainElement(data.Metric("PairsResidual_EFA")))
	})

	It("computes independently per primary asset", func() {
		times := make([]time.Time, 20)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-19)
		}

		aaplPrices := make([]float64, 20)
		msftPrices := make([]float64, 20)
		spyPrices := make([]float64, 20)
		for ii := range 20 {
			spyPrices[ii] = 100.0 + float64(ii)*0.5
			aaplPrices[ii] = 100.0 + float64(ii)*0.5
			msftPrices[ii] = 80.0 + float64(ii)*0.5
		}

		aaplPrices[19] = 130.0 // AAPL diverges up.
		msftPrices[19] = 70.0  // MSFT diverges down.

		primaryDF, err := data.NewDataFrame(times, []asset.Asset{aapl, msft}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices, msftPrices})
		Expect(err).NotTo(HaveOccurred())

		spyDF, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: primaryDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl, msft}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())

		// AAPL diverged up => positive; MSFT diverged down => negative.
		Expect(result.Value(aapl, "PairsResidual_SPY")).To(BeNumerically(">", 0))
		Expect(result.Value(msft, "PairsResidual_SPY")).To(BeNumerically("<", 0))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now}
		aaplDF, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})
		spyDF, _ := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(0), refU)
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates primary fetch error", func() {
		ds := &errorDataSource{err: errors.New("primary unavailable")}
		refDF, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		refDS := &mockDataSource{currentDate: now, fetchResult: refDF}

		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("primary unavailable"))
	})

	It("propagates reference fetch error", func() {
		aaplDF, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &errorDataSource{err: errors.New("ref unavailable")}

		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsResidual(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("ref unavailable"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "PairsResidual" ./signal/...`
Expected: compilation failure (PairsResidual not defined)

- [ ] **Step 3: Write the implementation**

```go
package signal

import (
	"context"
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// PairsResidual computes the z-score of OLS regression residuals between each
// primary asset and each reference asset. For each (primary, reference) pair,
// it regresses primary returns on reference returns, computes the residual
// series, and returns the z-score of the last residual. Positive values mean
// the primary asset is rich relative to the reference; negative means cheap.
//
// The reference universe may contain multiple assets. Each reference asset
// produces its own output metric named PairsResidual_{Ticker}. For example,
// with references SPY and EFA, the output has metrics PairsResidual_SPY and
// PairsResidual_EFA.
//
// Returns a single-row DataFrame with one metric per reference asset.
func PairsResidual(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, referenceUniverse universe.Universe) *data.DataFrame {
	primaryDF, err := assetUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsResidual: primary fetch: %w", err))
	}

	refDF, err := referenceUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsResidual: reference fetch: %w", err))
	}

	numRows := primaryDF.Len()
	if numRows < 3 {
		return data.WithErr(fmt.Errorf("PairsResidual: need at least 3 data points, got %d", numRows))
	}

	primaryAssets := primaryDF.AssetList()
	refAssets := refDF.AssetList()
	times := primaryDF.Times()
	lastTime := []time.Time{times[len(times)-1]}

	// Build return series for reference assets.
	refReturns := make(map[string][]float64, len(refAssets))

	for _, ra := range refAssets {
		prices := refDF.Column(ra, data.MetricClose)
		rets := make([]float64, numRows-1)

		for ii := 1; ii < numRows; ii++ {
			if prices[ii-1] == 0 {
				return data.WithErr(fmt.Errorf("PairsResidual: zero price for reference %s at index %d", ra.Ticker, ii-1))
			}

			rets[ii-1] = (prices[ii] - prices[ii-1]) / prices[ii-1]
		}

		refReturns[ra.Ticker] = rets
	}

	// Build metric names and output columns.
	metricNames := make([]data.Metric, len(refAssets))
	for ri, ra := range refAssets {
		metricNames[ri] = data.Metric("PairsResidual_" + ra.Ticker)
	}

	// Columns are ordered: asset0_ref0, asset0_ref1, ..., asset1_ref0, ...
	allCols := make([][]float64, len(primaryAssets)*len(refAssets))

	for pi, pa := range primaryAssets {
		prices := primaryDF.Column(pa, data.MetricClose)
		primaryRets := make([]float64, numRows-1)

		for ii := 1; ii < numRows; ii++ {
			if prices[ii-1] == 0 {
				return data.WithErr(fmt.Errorf("PairsResidual: zero price for %s at index %d", pa.Ticker, ii-1))
			}

			primaryRets[ii-1] = (prices[ii] - prices[ii-1]) / prices[ii-1]
		}

		for ri, ra := range refAssets {
			rr := refReturns[ra.Ticker]

			// OLS: regress primaryRets on rr.
			slope, intercept, regErr := linRegress(rr, primaryRets)
			if regErr != nil {
				return data.WithErr(fmt.Errorf("PairsResidual [%s vs %s]: %w", pa.Ticker, ra.Ticker, regErr))
			}

			// Compute residuals.
			residuals := make([]float64, len(primaryRets))
			for ii := range primaryRets {
				residuals[ii] = primaryRets[ii] - (intercept + slope*rr[ii])
			}

			// Z-score of residuals.
			zz, zErr := zScore(residuals)
			if zErr != nil {
				return data.WithErr(fmt.Errorf("PairsResidual [%s vs %s]: %w", pa.Ticker, ra.Ticker, zErr))
			}

			allCols[pi*len(refAssets)+ri] = []float64{zz}
		}
	}

	result, err := data.NewDataFrame(lastTime, primaryAssets, metricNames, primaryDF.Frequency(), allCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsResidual: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "PairsResidual" ./signal/...`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add signal/pairs_residual.go signal/pairs_residual_test.go
git commit -m "feat(signal): add PairsResidual mean reversion signal (#27)"
```

---

## Task 6: Pairs Ratio Signal

**Files:**
- Create: `signal/pairs_ratio.go`
- Create: `signal/pairs_ratio_test.go`

- [ ] **Step 1: Write the failing test**

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

var _ = Describe("PairsRatio", func() {
	var (
		ctx  context.Context
		aapl asset.Asset
		msft asset.Asset
		spy  asset.Asset
		efa  asset.Asset
		now  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		spy = asset.Asset{CompositeFigi: "FIGI-SPY", Ticker: "SPY"}
		efa = asset.Asset{CompositeFigi: "FIGI-EFA", Ticker: "EFA"}
		now = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	It("produces a z-score of price ratio against a single reference", func() {
		times := make([]time.Time, 20)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-19)
		}

		// AAPL and SPY track closely, then AAPL spikes at the end.
		aaplPrices := make([]float64, 20)
		spyPrices := make([]float64, 20)
		for ii := range 20 {
			spyPrices[ii] = 100.0 + float64(ii)*0.5
			aaplPrices[ii] = 150.0 + float64(ii)*0.75 // Ratio ~1.5 throughout.
		}

		aaplPrices[19] = 200.0 // Spike the ratio at the end.

		aaplDF, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices})
		Expect(err).NotTo(HaveOccurred())

		spyDF, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())
		Expect(result.Len()).To(Equal(1))
		Expect(result.MetricList()).To(Equal([]data.Metric{"PairsRatio_SPY"}))

		// Ratio spiked up at the end, so z-score should be positive.
		Expect(result.Value(aapl, "PairsRatio_SPY")).To(BeNumerically(">", 0))
	})

	It("produces metrics for multiple reference assets", func() {
		times := make([]time.Time, 20)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-19)
		}

		aaplPrices := make([]float64, 20)
		spyPrices := make([]float64, 20)
		efaPrices := make([]float64, 20)
		for ii := range 20 {
			aaplPrices[ii] = 150.0 + float64(ii)*1.0
			spyPrices[ii] = 100.0 + float64(ii)*0.5
			efaPrices[ii] = 50.0 + float64(ii)*0.3
		}

		aaplDF, err := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices})
		Expect(err).NotTo(HaveOccurred())

		refDF, err := data.NewDataFrame(times, []asset.Asset{spy, efa}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices, efaPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: refDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy, efa}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())

		metricList := result.MetricList()
		Expect(metricList).To(HaveLen(2))
		Expect(metricList).To(ContainElement(data.Metric("PairsRatio_SPY")))
		Expect(metricList).To(ContainElement(data.Metric("PairsRatio_EFA")))
	})

	It("computes independently per primary asset", func() {
		times := make([]time.Time, 20)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-19)
		}

		aaplPrices := make([]float64, 20)
		msftPrices := make([]float64, 20)
		spyPrices := make([]float64, 20)
		for ii := range 20 {
			spyPrices[ii] = 100.0 + float64(ii)*0.5
			aaplPrices[ii] = 150.0 + float64(ii)*0.75
			msftPrices[ii] = 120.0 + float64(ii)*0.60
		}

		aaplPrices[19] = 200.0 // AAPL ratio spikes up.
		msftPrices[19] = 100.0 // MSFT ratio drops.

		primaryDF, err := data.NewDataFrame(times, []asset.Asset{aapl, msft}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices, msftPrices})
		Expect(err).NotTo(HaveOccurred())

		spyDF, err := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices})
		Expect(err).NotTo(HaveOccurred())

		primaryDS := &mockDataSource{currentDate: now, fetchResult: primaryDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl, msft}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).NotTo(HaveOccurred())

		Expect(result.Value(aapl, "PairsRatio_SPY")).To(BeNumerically(">", 0))
		Expect(result.Value(msft, "PairsRatio_SPY")).To(BeNumerically("<", 0))
	})

	It("returns error when reference price is zero", func() {
		times := make([]time.Time, 5)
		for ii := range times {
			times[ii] = now.AddDate(0, 0, ii-4)
		}

		aaplPrices := []float64{100, 101, 102, 103, 104}
		spyPrices := []float64{100, 0, 102, 103, 104} // Zero price.

		aaplDF, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{aaplPrices})
		spyDF, _ := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{spyPrices})

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(4), refU)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("zero"))
	})

	It("returns error on insufficient data", func() {
		times := []time.Time{now}
		aaplDF, _ := data.NewDataFrame(times, []asset.Asset{aapl}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})
		spyDF, _ := data.NewDataFrame(times, []asset.Asset{spy}, []data.Metric{data.MetricClose}, data.Daily, [][]float64{{100}})

		primaryDS := &mockDataSource{currentDate: now, fetchResult: aaplDF}
		refDS := &mockDataSource{currentDate: now, fetchResult: spyDF}
		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, primaryDS)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(0), refU)
		Expect(result.Err()).To(HaveOccurred())
	})

	It("propagates fetch error", func() {
		ds := &errorDataSource{err: errors.New("network failure")}
		refDF, _ := data.NewDataFrame(nil, nil, nil, data.Daily, nil)
		refDS := &mockDataSource{currentDate: now, fetchResult: refDF}

		primaryU := universe.NewStaticWithSource([]asset.Asset{aapl}, ds)
		refU := universe.NewStaticWithSource([]asset.Asset{spy}, refDS)

		result := signal.PairsRatio(ctx, primaryU, portfolio.Days(19), refU)
		Expect(result.Err()).To(HaveOccurred())
		Expect(result.Err().Error()).To(ContainSubstring("network failure"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "PairsRatio" ./signal/...`
Expected: compilation failure (PairsRatio not defined)

- [ ] **Step 3: Write the implementation**

```go
package signal

import (
	"context"
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

// PairsRatio computes the z-score of the price ratio between each primary asset
// and each reference asset. For each (primary, reference) pair, it computes
// ratio = primary_price / reference_price at each bar, then returns the z-score
// of the ratio series. Positive values mean the ratio is above its rolling
// mean (primary is rich); negative means below (primary is cheap).
//
// The reference universe may contain multiple assets. Each reference asset
// produces its own output metric named PairsRatio_{Ticker}. For example,
// with references SPY and EFA, the output has metrics PairsRatio_SPY and
// PairsRatio_EFA.
//
// Returns a single-row DataFrame with one metric per reference asset.
func PairsRatio(ctx context.Context, assetUniverse universe.Universe, period portfolio.Period, referenceUniverse universe.Universe) *data.DataFrame {
	primaryDF, err := assetUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsRatio: primary fetch: %w", err))
	}

	refDF, err := referenceUniverse.Window(ctx, period, data.MetricClose)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsRatio: reference fetch: %w", err))
	}

	numRows := primaryDF.Len()
	if numRows < 2 {
		return data.WithErr(fmt.Errorf("PairsRatio: need at least 2 data points, got %d", numRows))
	}

	primaryAssets := primaryDF.AssetList()
	refAssets := refDF.AssetList()
	times := primaryDF.Times()
	lastTime := []time.Time{times[len(times)-1]}

	// Cache reference price columns.
	refPrices := make(map[string][]float64, len(refAssets))

	for _, ra := range refAssets {
		refPrices[ra.Ticker] = refDF.Column(ra, data.MetricClose)
	}

	// Build metric names.
	metricNames := make([]data.Metric, len(refAssets))
	for ri, ra := range refAssets {
		metricNames[ri] = data.Metric("PairsRatio_" + ra.Ticker)
	}

	allCols := make([][]float64, len(primaryAssets)*len(refAssets))

	for pi, pa := range primaryAssets {
		paPrices := primaryDF.Column(pa, data.MetricClose)

		for ri, ra := range refAssets {
			raPrices := refPrices[ra.Ticker]

			// Compute price ratios.
			ratios := make([]float64, numRows)
			for ii := range numRows {
				if raPrices[ii] == 0 {
					return data.WithErr(fmt.Errorf("PairsRatio: zero reference price for %s at index %d", ra.Ticker, ii))
				}

				ratios[ii] = paPrices[ii] / raPrices[ii]
			}

			// Z-score of ratios.
			zz, zErr := zScore(ratios)
			if zErr != nil {
				return data.WithErr(fmt.Errorf("PairsRatio [%s vs %s]: %w", pa.Ticker, ra.Ticker, zErr))
			}

			allCols[pi*len(refAssets)+ri] = []float64{zz}
		}
	}

	result, err := data.NewDataFrame(lastTime, primaryAssets, metricNames, primaryDF.Frequency(), allCols)
	if err != nil {
		return data.WithErr(fmt.Errorf("PairsRatio: %w", err))
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race -focus "PairsRatio" ./signal/...`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add signal/pairs_ratio.go signal/pairs_ratio_test.go
git commit -m "feat(signal): add PairsRatio mean reversion signal (#27)"
```

---

## Task 7: Update Documentation

**Files:**
- Modify: `signal/doc.go`
- Modify: `docs/signals.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add the five new signals to the doc.go catalog**

Add the following entries to the `Built-in Signals` list in `signal/doc.go`, after the MFI entry:

```go
//   - [ZScore](ctx, u, period, metrics...): Z-score of current value relative to rolling mean (unbounded).
//   - [HurstRS](ctx, u, period): Hurst exponent via Rescaled Range analysis (0 to 1).
//   - [HurstDFA](ctx, u, period): Hurst exponent via Detrended Fluctuation Analysis (0 to 1).
//   - [PairsResidual](ctx, u, period, refUniverse): Z-score of OLS regression residuals vs reference assets.
//   - [PairsRatio](ctx, u, period, refUniverse): Z-score of price ratio vs reference assets.
```

- [ ] **Step 2: Add the five new signals to docs/signals.md**

Add entries to the built-in signals table:

```markdown
| `ZScore` | `(ctx, u, period, metrics...)` | Z-score of current value relative to rolling mean (unbounded) |
| `HurstRS` | `(ctx, u, period)` | Hurst exponent via Rescaled Range analysis (0 to 1) |
| `HurstDFA` | `(ctx, u, period)` | Hurst exponent via Detrended Fluctuation Analysis (0 to 1) |
| `PairsResidual` | `(ctx, u, period, refUniverse)` | Z-score of OLS regression residuals vs reference assets |
| `PairsRatio` | `(ctx, u, period, refUniverse)` | Z-score of price ratio vs reference assets |
```

Add a new "Mean reversion" section under "Signal reference" with detailed documentation for each signal, following the style of existing sections (Trend and momentum, Volume, etc.). Include output metric names, parameter descriptions, computation overview, and usage examples.

- [ ] **Step 3: Add changelog entry**

Add an entry under `[Unreleased]` in `CHANGELOG.md`:

```markdown
### Added
- Mean reversion signals: Z-Score, Hurst exponent (R/S and DFA variants), and pairs trading signals (PairsResidual, PairsRatio) for identifying stretched conditions and pair relationships
```

- [ ] **Step 4: Verify the package compiles and docs render**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./signal/... && go doc ./signal/ | head -60`
Expected: compiles cleanly, new signals visible in doc output

- [ ] **Step 5: Run the full signal test suite**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./signal/...`
Expected: all tests pass

- [ ] **Step 6: Run lint**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && make lint`
Expected: no lint errors

- [ ] **Step 7: Commit**

```bash
git add signal/doc.go docs/signals.md CHANGELOG.md
git commit -m "docs: add mean reversion signals to reference and changelog (#27)"
```
