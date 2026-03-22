# Monte Carlo Simulation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Monte Carlo simulation to assess strategy robustness by running thousands of backtests against resampled synthetic price data.

**Architecture:** A resampling data provider wraps any existing `BatchProvider` and produces synthetic price series via pluggable resampling methods (block bootstrap, return-level bootstrap, permutation). A Monte Carlo study orchestrates N simulation paths through the existing study runner, using a new `EngineCustomizer` interface to inject per-run resampling providers. The `Analyze` method computes percentile distributions and composes a report with fan charts, confidence intervals, and probability of ruin.

**Tech Stack:** Go, Ginkgo/Gomega (tests), existing `data`, `study`, `report`, and `engine` packages.

**Spec:** `docs/superpowers/specs/2026-03-22-monte-carlo-simulation-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `data/resampler.go` | `Resampler` interface and three implementations (block bootstrap, return-level bootstrap, permutation) |
| `data/resampler_test.go` | Unit tests for all three resamplers |
| `data/resampling_provider.go` | `ResamplingProvider` implementing `BatchProvider` -- wraps cached historical data with resampled synthetic series |
| `data/resampling_provider_test.go` | Unit tests for the resampling provider |
| `study/engine_customizer.go` | `EngineCustomizer` interface definition |
| `study/runner.go` | Add type assertion for `EngineCustomizer` in `runSingle` |
| `study/runner_test.go` | Test that `EngineCustomizer` is called when implemented |
| `study/montecarlo/montecarlo.go` | `MonteCarloStudy` struct, `Study` + `EngineCustomizer` implementation |
| `study/montecarlo/analyze.go` | `Analyze` method -- percentile computation, report composition |
| `study/montecarlo/analyze_test.go` | Unit tests for analysis/report logic |
| `study/montecarlo/montecarlo_suite_test.go` | Ginkgo test suite bootstrap |
| `study/integration_test.go` | Integration test for Monte Carlo through the full runner pipeline |

---

### Task 1: Resampler Interface and Block Bootstrap

**Files:**
- Create: `data/resampler.go`
- Create: `data/resampler_test.go`

- [ ] **Step 1: Write the Resampler interface and BlockBootstrap struct**

In `data/resampler.go`:

```go
package data

import "math/rand/v2"

// Resampler produces a synthetic return series from historical returns.
// Input is a 2D slice (assets x time steps) of daily returns.
// Output is a 2D slice (assets x targetLen) of resampled returns.
// All methods synchronize cross-asset indices to preserve correlations.
type Resampler interface {
	Resample(returns [][]float64, targetLen int, rng *rand.Rand) [][]float64
}

// BlockBootstrap resamples by picking random contiguous blocks of returns
// across all assets simultaneously, preserving short-term autocorrelation
// and cross-asset correlations within blocks.
type BlockBootstrap struct {
	BlockSize int // number of trading days per block
}

func (bb *BlockBootstrap) Resample(returns [][]float64, targetLen int, rng *rand.Rand) [][]float64 {
	if len(returns) == 0 || len(returns[0]) == 0 {
		return returns
	}

	blockSize := bb.BlockSize
	if blockSize <= 0 {
		blockSize = 20
	}

	numAssets := len(returns)
	histLen := len(returns[0])

	result := make([][]float64, numAssets)
	for assetIdx := range result {
		result[assetIdx] = make([]float64, 0, targetLen)
	}

	filled := 0
	for filled < targetLen {
		// Pick a random start index in the historical series.
		startIdx := rng.IntN(histLen)
		blockEnd := startIdx + blockSize
		if blockEnd > histLen {
			blockEnd = histLen
		}

		// How many values to copy from this block.
		copyLen := blockEnd - startIdx
		if filled+copyLen > targetLen {
			copyLen = targetLen - filled
		}

		for assetIdx := range numAssets {
			result[assetIdx] = append(result[assetIdx], returns[assetIdx][startIdx:startIdx+copyLen]...)
		}

		filled += copyLen
	}

	return result
}
```

- [ ] **Step 2: Write failing tests for BlockBootstrap**

In `data/resampler_test.go`:

```go
package data_test

import (
	"math/rand/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("Resampler", func() {
	Describe("BlockBootstrap", func() {
		It("satisfies the Resampler interface", func() {
			var resampler data.Resampler = &data.BlockBootstrap{BlockSize: 5}
			Expect(resampler).NotTo(BeNil())
		})

		It("produces output of the requested length", func() {
			resampler := &data.BlockBootstrap{BlockSize: 5}
			returns := [][]float64{
				{0.01, -0.02, 0.03, 0.01, -0.01, 0.02, 0.01, -0.03, 0.02, 0.01},
				{0.02, -0.01, 0.02, 0.03, -0.02, 0.01, 0.03, -0.01, 0.01, 0.02},
			}

			rng := rand.New(rand.NewPCG(42, 0))
			result := resampler.Resample(returns, 15, rng)

			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(HaveLen(15))
			Expect(result[1]).To(HaveLen(15))
		})

		It("preserves cross-asset correlation by using same time indices", func() {
			// With a fixed seed, two assets should get the same block selections.
			resampler := &data.BlockBootstrap{BlockSize: 3}
			returns := [][]float64{
				{0.10, 0.20, 0.30, 0.40, 0.50, 0.60},
				{1.10, 1.20, 1.30, 1.40, 1.50, 1.60},
			}

			rng := rand.New(rand.NewPCG(99, 0))
			result := resampler.Resample(returns, 6, rng)

			// For each position in the output, the two assets should come from
			// the same historical time index. We verify by checking that the
			// difference between asset 1 and asset 0 is always 1.0.
			for idx := range result[0] {
				diff := result[1][idx] - result[0][idx]
				Expect(diff).To(BeNumerically("~", 1.0, 1e-10),
					"at index %d: assets should come from same historical block", idx)
			}
		})

		It("is reproducible with the same seed", func() {
			resampler := &data.BlockBootstrap{BlockSize: 5}
			returns := [][]float64{
				{0.01, -0.02, 0.03, 0.01, -0.01, 0.02, 0.01, -0.03, 0.02, 0.01},
			}

			rng1 := rand.New(rand.NewPCG(42, 0))
			result1 := resampler.Resample(returns, 10, rng1)

			rng2 := rand.New(rand.NewPCG(42, 0))
			result2 := resampler.Resample(returns, 10, rng2)

			Expect(result1).To(Equal(result2))
		})

		It("handles empty input", func() {
			resampler := &data.BlockBootstrap{BlockSize: 5}
			result := resampler.Resample([][]float64{}, 10, rand.New(rand.NewPCG(1, 0)))
			Expect(result).To(BeEmpty())
		})

		It("handles targetLen shorter than block size", func() {
			resampler := &data.BlockBootstrap{BlockSize: 20}
			returns := [][]float64{
				{0.01, -0.02, 0.03, 0.01, -0.01},
			}

			rng := rand.New(rand.NewPCG(42, 0))
			result := resampler.Resample(returns, 3, rng)
			Expect(result[0]).To(HaveLen(3))
		})
	})
})
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run Resampler -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add data/resampler.go data/resampler_test.go
git commit -m "feat: add Resampler interface and BlockBootstrap implementation"
```

---

### Task 2: Return-Level Bootstrap and Permutation Resamplers

**Files:**
- Modify: `data/resampler.go`
- Modify: `data/resampler_test.go`

- [ ] **Step 1: Add ReturnBootstrap and Permutation to resampler.go**

Append to `data/resampler.go`:

```go
// ReturnBootstrap resamples individual time steps with replacement.
// For each output time step, picks a random historical time step and copies
// returns for all assets at that step. Preserves cross-asset correlations
// at each point but destroys all temporal structure.
type ReturnBootstrap struct{}

func (rb *ReturnBootstrap) Resample(returns [][]float64, targetLen int, rng *rand.Rand) [][]float64 {
	if len(returns) == 0 || len(returns[0]) == 0 {
		return returns
	}

	numAssets := len(returns)
	histLen := len(returns[0])

	result := make([][]float64, numAssets)
	for assetIdx := range result {
		result[assetIdx] = make([]float64, targetLen)
	}

	for timeIdx := range targetLen {
		srcIdx := rng.IntN(histLen)
		for assetIdx := range numAssets {
			result[assetIdx][timeIdx] = returns[assetIdx][srcIdx]
		}
	}

	return result
}

// Permutation randomly shuffles the time indices of the historical return
// series without replacement. All assets are permuted with the same index
// mapping. The marginal distribution is exactly preserved.
type Permutation struct{}

func (p *Permutation) Resample(returns [][]float64, targetLen int, rng *rand.Rand) [][]float64 {
	if len(returns) == 0 || len(returns[0]) == 0 {
		return returns
	}

	numAssets := len(returns)
	histLen := len(returns[0])

	// Generate a permuted index mapping.
	indices := rng.Perm(histLen)

	// If targetLen differs from histLen, truncate or wrap.
	actualLen := targetLen
	if actualLen > histLen {
		actualLen = histLen
	}

	result := make([][]float64, numAssets)
	for assetIdx := range result {
		result[assetIdx] = make([]float64, actualLen)
		for timeIdx := range actualLen {
			result[assetIdx][timeIdx] = returns[assetIdx][indices[timeIdx]]
		}
	}

	return result
}
```

- [ ] **Step 2: Write tests for ReturnBootstrap and Permutation**

Append to `data/resampler_test.go`:

```go
	Describe("ReturnBootstrap", func() {
		It("satisfies the Resampler interface", func() {
			var resampler data.Resampler = &data.ReturnBootstrap{}
			Expect(resampler).NotTo(BeNil())
		})

		It("produces output of the requested length", func() {
			resampler := &data.ReturnBootstrap{}
			returns := [][]float64{
				{0.01, -0.02, 0.03, 0.01, -0.01},
				{0.02, -0.01, 0.02, 0.03, -0.02},
			}

			rng := rand.New(rand.NewPCG(42, 0))
			result := resampler.Resample(returns, 10, rng)

			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(HaveLen(10))
			Expect(result[1]).To(HaveLen(10))
		})

		It("preserves cross-asset correlation at each time step", func() {
			resampler := &data.ReturnBootstrap{}
			returns := [][]float64{
				{0.10, 0.20, 0.30, 0.40, 0.50},
				{1.10, 1.20, 1.30, 1.40, 1.50},
			}

			rng := rand.New(rand.NewPCG(42, 0))
			result := resampler.Resample(returns, 5, rng)

			for idx := range result[0] {
				diff := result[1][idx] - result[0][idx]
				Expect(diff).To(BeNumerically("~", 1.0, 1e-10))
			}
		})

		It("only contains values from the original series", func() {
			resampler := &data.ReturnBootstrap{}
			returns := [][]float64{
				{0.10, 0.20, 0.30},
			}
			validValues := map[float64]bool{0.10: true, 0.20: true, 0.30: true}

			rng := rand.New(rand.NewPCG(42, 0))
			result := resampler.Resample(returns, 20, rng)

			for _, value := range result[0] {
				Expect(validValues).To(HaveKey(value))
			}
		})
	})

	Describe("Permutation", func() {
		It("satisfies the Resampler interface", func() {
			var resampler data.Resampler = &data.Permutation{}
			Expect(resampler).NotTo(BeNil())
		})

		It("preserves the exact distribution of returns", func() {
			resampler := &data.Permutation{}
			original := []float64{0.01, -0.02, 0.03, 0.04, -0.05}
			returns := [][]float64{make([]float64, len(original))}
			copy(returns[0], original)

			rng := rand.New(rand.NewPCG(42, 0))
			result := resampler.Resample(returns, len(original), rng)

			// Same values, possibly different order.
			Expect(result[0]).To(HaveLen(len(original)))
			Expect(result[0]).To(ConsistOf(
				BeNumerically("~", 0.01, 1e-10),
				BeNumerically("~", -0.02, 1e-10),
				BeNumerically("~", 0.03, 1e-10),
				BeNumerically("~", 0.04, 1e-10),
				BeNumerically("~", -0.05, 1e-10),
			))
		})

		It("preserves cross-asset correlation", func() {
			resampler := &data.Permutation{}
			returns := [][]float64{
				{0.10, 0.20, 0.30, 0.40, 0.50},
				{1.10, 1.20, 1.30, 1.40, 1.50},
			}

			rng := rand.New(rand.NewPCG(42, 0))
			result := resampler.Resample(returns, 5, rng)

			for idx := range result[0] {
				diff := result[1][idx] - result[0][idx]
				Expect(diff).To(BeNumerically("~", 1.0, 1e-10))
			}
		})

		It("truncates when targetLen exceeds history length", func() {
			resampler := &data.Permutation{}
			returns := [][]float64{
				{0.01, -0.02, 0.03},
			}

			rng := rand.New(rand.NewPCG(42, 0))
			result := resampler.Resample(returns, 10, rng)

			// Permutation without replacement can only produce histLen values.
			Expect(result[0]).To(HaveLen(3))
		})
	})
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run Resampler -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add data/resampler.go data/resampler_test.go
git commit -m "feat: add ReturnBootstrap and Permutation resamplers"
```

---

### Task 3: Resampling Data Provider

**Files:**
- Create: `data/resampling_provider.go`
- Create: `data/resampling_provider_test.go`

- [ ] **Step 1: Write the ResamplingProvider**

In `data/resampling_provider.go`:

```go
package data

import (
	"context"
	"math"
	"math/rand/v2"
)

// Compile-time interface check.
var _ BatchProvider = (*ResamplingProvider)(nil)

// ResamplingProvider is a BatchProvider that serves synthetic price data
// by resampling historical returns from a pre-fetched DataFrame. It is
// used by Monte Carlo simulation to produce randomized yet statistically
// grounded price series for backtesting.
type ResamplingProvider struct {
	historicalData *DataFrame
	resampler      Resampler
	seed           uint64
	metrics        []Metric
}

// NewResamplingProvider creates a provider that resamples the given historical
// data using the specified resampler and seed. The historicalData DataFrame is
// read-only and can be shared across multiple providers.
func NewResamplingProvider(historicalData *DataFrame, resampler Resampler, seed uint64, metrics []Metric) *ResamplingProvider {
	return &ResamplingProvider{
		historicalData: historicalData,
		resampler:      resampler,
		seed:           seed,
		metrics:        metrics,
	}
}

// Provides returns the set of metrics this provider can supply.
func (rp *ResamplingProvider) Provides() []Metric {
	return rp.metrics
}

// Fetch produces a synthetic DataFrame by resampling historical returns.
// The output has the same assets and time structure as the requested range
// but with resampled price data. Dividends, splits, and other corporate
// actions are zeroed out.
func (rp *ResamplingProvider) Fetch(_ context.Context, req DataRequest) (*DataFrame, error) {
	// Narrow historical data to the requested assets and price metrics only.
	priceMetrics := []Metric{MetricClose}
	historical := rp.historicalData.Assets(req.Assets...).Metrics(priceMetrics...)

	if historical.Len() == 0 {
		return historical, nil
	}

	// Extract returns for each asset from the historical close prices.
	numAssets := len(req.Assets)
	histLen := historical.Len()

	historicalReturns := make([][]float64, numAssets)
	for assetIdx, assetItem := range req.Assets {
		prices := historical.Column(assetItem, MetricClose)
		returns := make([]float64, histLen-1)

		for dayIdx := 1; dayIdx < histLen; dayIdx++ {
			if prices[dayIdx-1] != 0 {
				returns[dayIdx-1] = (prices[dayIdx] - prices[dayIdx-1]) / prices[dayIdx-1]
			}
		}

		historicalReturns[assetIdx] = returns
	}

	// Resample returns.
	rng := rand.New(rand.NewPCG(rp.seed, 0))

	targetLen := histLen - 1 // returns are one shorter than prices
	syntheticReturns := rp.resampler.Resample(historicalReturns, targetLen, rng)

	// Reconstruct prices from synthetic returns, starting at the historical
	// first price for each asset.
	numOutputMetrics := len(req.Metrics)
	numOutputTimes := len(syntheticReturns[0]) + 1
	columns := make([][]float64, numAssets*numOutputMetrics)

	for assetIdx, assetItem := range req.Assets {
		firstPrice := historical.Column(assetItem, MetricClose)[0]

		// Build synthetic price series.
		syntheticPrices := make([]float64, numOutputTimes)
		syntheticPrices[0] = firstPrice

		for dayIdx, dailyReturn := range syntheticReturns[assetIdx] {
			syntheticPrices[dayIdx+1] = syntheticPrices[dayIdx] * (1 + dailyReturn)
		}

		// Assign columns for each requested metric.
		for metricIdx, metric := range req.Metrics {
			colIdx := assetIdx*numOutputMetrics + metricIdx
			col := make([]float64, numOutputTimes)

			switch metric {
			case Dividend, SplitFactor:
				// Zero out corporate actions.
				for dayIdx := range col {
					if metric == SplitFactor {
						col[dayIdx] = 1.0
					}
					// Dividend defaults to 0.0 (zero value).
				}
			default:
				// All price metrics (Close, AdjClose, High, Low, Open) use
				// the synthetic price series. High/Low get small offsets.
				copy(col, syntheticPrices)

				if metric == MetricHigh {
					for dayIdx := range col {
						col[dayIdx] *= 1.005
					}
				} else if metric == MetricLow {
					for dayIdx := range col {
						col[dayIdx] *= 0.995
					}
				}
			}

			columns[colIdx] = col
		}
	}

	// Use the historical times for the output DataFrame. If the historical
	// data is longer than our output, truncate to match.
	times := historical.Times()
	if len(times) > numOutputTimes {
		times = times[:numOutputTimes]
	}

	return NewDataFrame(times, req.Assets, req.Metrics, Daily, columns)
}

// Close is a no-op for ResamplingProvider.
func (rp *ResamplingProvider) Close() error {
	return nil
}
```

Note: the `Times()` method on DataFrame needs to be checked. If it doesn't exist, the implementation should use an equivalent accessor. Check `data/data_frame.go` for how to access the times slice.

- [ ] **Step 2: Write tests for ResamplingProvider**

In `data/resampling_provider_test.go`:

```go
package data_test

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("ResamplingProvider", func() {
	var (
		testAsset  asset.Asset
		testAsset2 asset.Asset
		startDate  time.Time
		historical *data.DataFrame
		metrics    []data.Metric
	)

	BeforeEach(func() {
		testAsset = asset.Asset{CompositeFigi: "FIGI-TEST", Ticker: "TEST"}
		testAsset2 = asset.Asset{CompositeFigi: "FIGI-TEST2", Ticker: "TEST2"}
		startDate = time.Date(2024, 1, 1, 16, 0, 0, 0, time.UTC)
		metrics = []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.SplitFactor}

		// Create 20 days of historical data with known prices.
		numDays := 20
		times := make([]time.Time, numDays)
		for dayIdx := range times {
			times[dayIdx] = startDate.AddDate(0, 0, dayIdx)
		}

		assets := []asset.Asset{testAsset, testAsset2}
		numCols := len(assets) * len(metrics)
		vals := make([]float64, numDays*numCols)

		for assetIdx := range assets {
			for metricIdx, metric := range metrics {
				colStart := (assetIdx*len(metrics) + metricIdx) * numDays
				for dayIdx := 0; dayIdx < numDays; dayIdx++ {
					switch metric {
					case data.SplitFactor:
						vals[colStart+dayIdx] = 1.0
					case data.Dividend:
						vals[colStart+dayIdx] = 0.0
					default:
						vals[colStart+dayIdx] = 100.0 + float64(dayIdx)*0.5 + float64(assetIdx)*10.0
					}
				}
			}
		}

		columns := data.SlabToColumns(vals, numCols, numDays)
		var err error
		historical, err = data.NewDataFrame(times, assets, metrics, data.Daily, columns)
		Expect(err).NotTo(HaveOccurred())
	})

	It("satisfies the BatchProvider interface", func() {
		var provider data.BatchProvider = data.NewResamplingProvider(
			historical, &data.BlockBootstrap{BlockSize: 5}, 42, metrics,
		)
		Expect(provider).NotTo(BeNil())
	})

	It("returns a DataFrame with the correct dimensions", func() {
		provider := data.NewResamplingProvider(
			historical, &data.BlockBootstrap{BlockSize: 5}, 42, metrics,
		)

		req := data.DataRequest{
			Assets:    []asset.Asset{testAsset},
			Metrics:   metrics,
			Start:     startDate,
			End:       startDate.AddDate(0, 0, 19),
			Frequency: data.Daily,
		}

		result, err := provider.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Len()).To(BeNumerically(">", 0))
	})

	It("zeroes out dividends", func() {
		provider := data.NewResamplingProvider(
			historical, &data.BlockBootstrap{BlockSize: 5}, 42, metrics,
		)

		req := data.DataRequest{
			Assets:    []asset.Asset{testAsset},
			Metrics:   metrics,
			Start:     startDate,
			End:       startDate.AddDate(0, 0, 19),
			Frequency: data.Daily,
		}

		result, err := provider.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		dividends := result.Column(testAsset, data.Dividend)
		for _, dividend := range dividends {
			Expect(dividend).To(Equal(0.0))
		}
	})

	It("sets split factor to 1.0", func() {
		provider := data.NewResamplingProvider(
			historical, &data.BlockBootstrap{BlockSize: 5}, 42, metrics,
		)

		req := data.DataRequest{
			Assets:    []asset.Asset{testAsset},
			Metrics:   metrics,
			Start:     startDate,
			End:       startDate.AddDate(0, 0, 19),
			Frequency: data.Daily,
		}

		result, err := provider.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		splits := result.Column(testAsset, data.SplitFactor)
		for _, split := range splits {
			Expect(split).To(Equal(1.0))
		}
	})

	It("produces different results with different seeds", func() {
		provider1 := data.NewResamplingProvider(
			historical, &data.BlockBootstrap{BlockSize: 5}, 42, metrics,
		)
		provider2 := data.NewResamplingProvider(
			historical, &data.BlockBootstrap{BlockSize: 5}, 99, metrics,
		)

		req := data.DataRequest{
			Assets:    []asset.Asset{testAsset},
			Metrics:   metrics,
			Start:     startDate,
			End:       startDate.AddDate(0, 0, 19),
			Frequency: data.Daily,
		}

		result1, err := provider1.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		result2, err := provider2.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		prices1 := result1.Column(testAsset, data.MetricClose)
		prices2 := result2.Column(testAsset, data.MetricClose)

		// At least some values should differ.
		hasDifference := false
		for idx := range prices1 {
			if math.Abs(prices1[idx]-prices2[idx]) > 1e-10 {
				hasDifference = true
				break
			}
		}

		Expect(hasDifference).To(BeTrue(), "different seeds should produce different results")
	})

	It("is reproducible with the same seed", func() {
		provider1 := data.NewResamplingProvider(
			historical, &data.BlockBootstrap{BlockSize: 5}, 42, metrics,
		)
		provider2 := data.NewResamplingProvider(
			historical, &data.BlockBootstrap{BlockSize: 5}, 42, metrics,
		)

		req := data.DataRequest{
			Assets:    []asset.Asset{testAsset},
			Metrics:   metrics,
			Start:     startDate,
			End:       startDate.AddDate(0, 0, 19),
			Frequency: data.Daily,
		}

		result1, err := provider1.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		result2, err := provider2.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		prices1 := result1.Column(testAsset, data.MetricClose)
		prices2 := result2.Column(testAsset, data.MetricClose)

		Expect(prices1).To(Equal(prices2))
	})

	It("produces valid prices (no NaN, no negative)", func() {
		provider := data.NewResamplingProvider(
			historical, &data.BlockBootstrap{BlockSize: 5}, 42, metrics,
		)

		req := data.DataRequest{
			Assets:    []asset.Asset{testAsset},
			Metrics:   metrics,
			Start:     startDate,
			End:       startDate.AddDate(0, 0, 19),
			Frequency: data.Daily,
		}

		result, err := provider.Fetch(context.Background(), req)
		Expect(err).NotTo(HaveOccurred())

		prices := result.Column(testAsset, data.MetricClose)
		for idx, price := range prices {
			Expect(math.IsNaN(price)).To(BeFalse(), "NaN at index %d", idx)
			Expect(price).To(BeNumerically(">", 0), "negative price at index %d", idx)
		}
	})
})
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./data/ -run ResamplingProvider -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add data/resampling_provider.go data/resampling_provider_test.go
git commit -m "feat: add ResamplingProvider wrapping historical data with resampled prices"
```

---

### Task 4: EngineCustomizer Interface and Runner Integration

**Files:**
- Create: `study/engine_customizer.go`
- Modify: `study/runner.go:105-116`
- Modify: `study/runner_test.go` (or create if needed)

- [ ] **Step 1: Define the EngineCustomizer interface**

In `study/engine_customizer.go`:

```go
package study

import "github.com/penny-vault/pvbt/engine"

// EngineCustomizer is an optional interface that a Study can implement to
// customize per-run engine construction. When the runner detects that a study
// implements this interface, it calls EngineOptions for each run and appends
// the returned options to the base options before constructing the engine.
type EngineCustomizer interface {
	EngineOptions(cfg RunConfig) []engine.Option
}
```

- [ ] **Step 2: Write a failing test for the runner calling EngineCustomizer**

Add to the study package's test file (check if `study/runner_test.go` exists; if not, add to `study/integration_test.go`):

```go
// customizableStudy is a test study that implements both Study and EngineCustomizer.
type customizableStudy struct {
	optionsCalled int
	configs       []study.RunConfig
}

func (cs *customizableStudy) Name() string        { return "CustomizableStudy" }
func (cs *customizableStudy) Description() string  { return "Test study with EngineCustomizer" }

func (cs *customizableStudy) Configurations(_ context.Context) ([]study.RunConfig, error) {
	return cs.configs, nil
}

func (cs *customizableStudy) Analyze(results []study.RunResult) (report.Report, error) {
	return report.Report{Title: "Test"}, nil
}

func (cs *customizableStudy) EngineOptions(cfg study.RunConfig) []engine.Option {
	cs.optionsCalled++
	return nil
}

// Compile-time check.
var _ study.EngineCustomizer = (*customizableStudy)(nil)
```

Then add a test:

```go
It("calls EngineCustomizer.EngineOptions when the study implements it", func() {
	// This test verifies the runner type-asserts to EngineCustomizer
	// and calls EngineOptions for each run config.
	testStudy := &customizableStudy{
		configs: []study.RunConfig{
			{
				Name:  "Run 1",
				Start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2024, 4, 30, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	// ... set up runner with testStudy, a simple strategy, and test data provider
	// ... run and verify testStudy.optionsCalled == 1
})
```

The exact test setup should follow the pattern in `study/integration_test.go` -- use `makeSyntheticDailyData`, `buyAndHoldStrategy`, and `integrationAssetProvider`.

- [ ] **Step 3: Modify runner.go to call EngineCustomizer**

In `study/runner.go`, modify `runSingle` to add the type assertion after copying base options (around line 110):

```go
func (runner *Runner) runSingle(ctx context.Context, cfg RunConfig) RunResult {
	strategy := runner.NewStrategy()

	// Build engine options: base options + config-specific overrides.
	opts := make([]engine.Option, len(runner.Options))
	copy(opts, runner.Options)

	// If the study implements EngineCustomizer, append per-run options.
	if customizer, ok := runner.Study.(EngineCustomizer); ok {
		opts = append(opts, customizer.EngineOptions(cfg)...)
	}

	if cfg.Deposit > 0 {
		opts = append(opts, engine.WithInitialDeposit(cfg.Deposit))
	}

	eng := engine.New(strategy, opts...)
	// ... rest unchanged
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./study/ -v -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add study/engine_customizer.go study/runner.go study/integration_test.go
git commit -m "feat: add EngineCustomizer interface and runner integration"
```

---

### Task 5: Monte Carlo Study -- Structure and Configurations

**Files:**
- Create: `study/montecarlo/montecarlo.go`
- Create: `study/montecarlo/montecarlo_suite_test.go`
- Create: `study/montecarlo/montecarlo_test.go`

- [ ] **Step 1: Write the MonteCarloStudy struct and Configurations method**

In `study/montecarlo/montecarlo.go`:

```go
package montecarlo

import (
	"context"
	"fmt"
	"strconv"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
)

// Compile-time interface checks.
var (
	_ study.Study            = (*MonteCarloStudy)(nil)
	_ study.EngineCustomizer = (*MonteCarloStudy)(nil)
)

// MonteCarloStudy implements study.Study and study.EngineCustomizer to run
// a strategy against thousands of synthetic price series constructed by
// resampling historical returns.
type MonteCarloStudy struct {
	// Configuration
	Simulations   int
	Resampler     data.Resampler
	Seed          uint64
	RuinThreshold float64 // negative, e.g. -0.30

	// Data
	historicalData *data.DataFrame
	metrics        []data.Metric

	// Optional historical result for percentile ranking.
	HistoricalResult report.ReportablePortfolio

	// RunConfig template (date range, deposit).
	StartDate     time.Time
	EndDate       time.Time
	InitialDeposit float64
}

// New creates a MonteCarloStudy with sensible defaults. The historicalData
// DataFrame is pre-fetched and shared (read-only) across all simulation paths.
func New(historicalData *data.DataFrame, metrics []data.Metric) *MonteCarloStudy {
	return &MonteCarloStudy{
		Simulations:   1000,
		Resampler:     &data.BlockBootstrap{BlockSize: 20},
		Seed:          42,
		RuinThreshold: -0.30,
		historicalData: historicalData,
		metrics:        metrics,
	}
}

// Name returns the human-readable study name.
func (mcs *MonteCarloStudy) Name() string { return "Monte Carlo Simulation" }

// Description returns a short explanation of what the study does.
func (mcs *MonteCarloStudy) Description() string {
	return "Assess strategy robustness via resampled historical data"
}

// Configurations returns one RunConfig per simulation path.
func (mcs *MonteCarloStudy) Configurations(_ context.Context) ([]study.RunConfig, error) {
	configs := make([]study.RunConfig, mcs.Simulations)

	for pathIdx := range mcs.Simulations {
		seed := mcs.Seed + uint64(pathIdx)

		configs[pathIdx] = study.RunConfig{
			Name:    fmt.Sprintf("Path %d", pathIdx+1),
			Start:   mcs.StartDate,
			End:     mcs.EndDate,
			Deposit: mcs.InitialDeposit,
			Metadata: map[string]string{
				"study":           "monte-carlo",
				"simulation_seed": strconv.FormatUint(seed, 10),
			},
		}
	}

	return configs, nil
}

// EngineOptions constructs a ResamplingProvider for each run using the seed
// stored in the config's metadata.
func (mcs *MonteCarloStudy) EngineOptions(cfg study.RunConfig) []engine.Option {
	seedStr, hasSeed := cfg.Metadata["simulation_seed"]
	if !hasSeed {
		return nil
	}

	seed, err := strconv.ParseUint(seedStr, 10, 64)
	if err != nil {
		return nil
	}

	provider := data.NewResamplingProvider(mcs.historicalData, mcs.Resampler, seed, mcs.metrics)
	return []engine.Option{engine.WithDataProvider(provider)}
}

// Analyze collects all run results and composes the Monte Carlo report.
func (mcs *MonteCarloStudy) Analyze(results []study.RunResult) (report.Report, error) {
	return analyzeResults(results, mcs.HistoricalResult, mcs.RuinThreshold)
}
```

Note: add `"time"` to the imports.

- [ ] **Step 2: Create the test suite bootstrap**

In `study/montecarlo/montecarlo_suite_test.go`:

```go
package montecarlo_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
)

func TestMonteCarlo(t *testing.T) {
	RegisterFailHandler(Fail)
	log.Logger = log.Output(GinkgoWriter)
	RunSpecs(t, "Monte Carlo Suite")
}
```

- [ ] **Step 3: Write tests for Configurations and EngineOptions**

In `study/montecarlo/montecarlo_test.go`:

```go
package montecarlo_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/study/montecarlo"
)

var _ = Describe("MonteCarloStudy", func() {
	var (
		testAsset  asset.Asset
		historical *data.DataFrame
		metrics    []data.Metric
		mcStudy    *montecarlo.MonteCarloStudy
	)

	BeforeEach(func() {
		testAsset = asset.Asset{CompositeFigi: "FIGI-TEST", Ticker: "TEST"}
		metrics = []data.Metric{data.MetricClose, data.AdjClose, data.Dividend, data.SplitFactor}

		// Build a small historical DataFrame.
		numDays := 30
		startDate := time.Date(2024, 1, 1, 16, 0, 0, 0, time.UTC)
		times := make([]time.Time, numDays)
		for dayIdx := range times {
			times[dayIdx] = startDate.AddDate(0, 0, dayIdx)
		}

		assets := []asset.Asset{testAsset}
		numCols := len(assets) * len(metrics)
		vals := make([]float64, numDays*numCols)
		for metricIdx, metric := range metrics {
			colStart := metricIdx * numDays
			for dayIdx := 0; dayIdx < numDays; dayIdx++ {
				switch metric {
				case data.SplitFactor:
					vals[colStart+dayIdx] = 1.0
				case data.Dividend:
					vals[colStart+dayIdx] = 0.0
				default:
					vals[colStart+dayIdx] = 100.0 + float64(dayIdx)*0.5
				}
			}
		}

		columns := data.SlabToColumns(vals, numCols, numDays)
		var err error
		historical, err = data.NewDataFrame(times, assets, metrics, data.Daily, columns)
		Expect(err).NotTo(HaveOccurred())

		mcStudy = montecarlo.New(historical, metrics)
		mcStudy.Simulations = 5 // keep tests fast
		mcStudy.StartDate = startDate
		mcStudy.EndDate = startDate.AddDate(0, 0, numDays-1)
	})

	It("satisfies the Study interface", func() {
		Expect(mcStudy.Name()).To(Equal("Monte Carlo Simulation"))
	})

	It("returns the correct number of configurations", func() {
		configs, err := mcStudy.Configurations(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(configs).To(HaveLen(5))
	})

	It("assigns unique seeds to each configuration", func() {
		configs, err := mcStudy.Configurations(context.Background())
		Expect(err).NotTo(HaveOccurred())

		seeds := make(map[string]bool)
		for _, cfg := range configs {
			seed := cfg.Metadata["simulation_seed"]
			Expect(seeds).NotTo(HaveKey(seed), "duplicate seed: %s", seed)
			seeds[seed] = true
		}
	})

	It("EngineOptions returns a data provider option", func() {
		configs, err := mcStudy.Configurations(context.Background())
		Expect(err).NotTo(HaveOccurred())

		opts := mcStudy.EngineOptions(configs[0])
		Expect(opts).To(HaveLen(1))
	})

	It("EngineOptions returns nil for config without seed metadata", func() {
		cfg := study.RunConfig{Name: "no-seed"}
		opts := mcStudy.EngineOptions(cfg)
		Expect(opts).To(BeEmpty())
	})
})
```

Note: add `"github.com/penny-vault/pvbt/study"` to imports for the last test.

- [ ] **Step 4: Run tests (they will fail because analyzeResults doesn't exist yet)**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./study/montecarlo/ -v -count=1`
Expected: Compilation error for `analyzeResults` -- create a stub.

- [ ] **Step 5: Create a stub analyzeResults**

In `study/montecarlo/analyze.go`:

```go
package montecarlo

import (
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
)

func analyzeResults(results []study.RunResult, historicalResult report.ReportablePortfolio, ruinThreshold float64) (report.Report, error) {
	return report.Report{Title: "Monte Carlo Simulation"}, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./study/montecarlo/ -v -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add study/montecarlo/
git commit -m "feat: add MonteCarloStudy struct with Configurations and EngineOptions"
```

---

### Task 6: Analyze -- Percentile Computation and Report Sections

**Files:**
- Modify: `study/montecarlo/analyze.go`
- Create: `study/montecarlo/analyze_test.go`

- [ ] **Step 1: Write tests for the analysis helpers**

In `study/montecarlo/analyze_test.go`:

```go
package montecarlo_test

// Tests for percentile computation and report composition.
// The exact test structure depends on whether helpers are exported.
// Test through Analyze() by providing mock RunResults with known portfolios.

// Use a fakePortfolio that implements report.ReportablePortfolio with
// predetermined equity curves and metrics. Check that the report sections
// are correct.
```

The test should:
- Create multiple `RunResult` values with fake portfolios that return known equity curves
- Call `Analyze()` on the study
- Verify the report has the expected sections (TimeSeries for fan chart, Table for terminal wealth, Table for confidence intervals, MetricPairs for ruin probability, MetricPairs for historical rank, Text for summary)
- Verify percentile values are computed correctly for known inputs

Look at the existing `fakePortfolio` pattern in the test suite or create one. The fake needs to implement `report.ReportablePortfolio` which is `portfolio.Portfolio` + `portfolio.PortfolioStats`. Check what methods those interfaces require by reading `report/report.go` and `portfolio/portfolio.go`.

- [ ] **Step 2: Implement analyzeResults with all report sections**

In `study/montecarlo/analyze.go`, implement the full analysis:

1. Filter successful results (where `Err == nil`)
2. Extract equity curves from each path via `result.Portfolio.PerfData().Column(portfolioAsset, data.PortfolioEquity)`
3. Compute percentile bands at each time step for the fan chart
4. Collect terminal values (last equity value per path)
5. Extract CAGR, max drawdown, Sharpe from each path via `PerformanceMetric()`
6. Compute probability of ruin (paths where max drawdown exceeds threshold)
7. If historical result provided, compute its percentile rank
8. Compose report sections

Key helper functions to implement:
- `percentile(sorted []float64, pct float64) float64` -- given a sorted slice, return the value at the given percentile
- `percentileRank(sorted []float64, value float64) float64` -- what percentile does this value fall at

Use the same `portfolioAsset` sentinel as the stress test:
```go
var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}
```

The report sections should use the existing report types:
- `&report.TimeSeries{SectionName: "Equity Curve Distribution", Series: [...]}`
- `&report.Table{SectionName: "Terminal Wealth Distribution", ...}`
- `&report.Table{SectionName: "Confidence Intervals", ...}`
- `&report.MetricPairs{SectionName: "Probability of Ruin", ...}`
- `&report.MetricPairs{SectionName: "Historical Rank", ...}` (only if historical result provided)
- `&report.Text{SectionName: "Summary", Body: ...}`

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./study/montecarlo/ -v -count=1`
Expected: PASS

- [ ] **Step 4: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./study/montecarlo/ ./data/`
Expected: No errors. Fix any that appear.

- [ ] **Step 5: Commit**

```bash
git add study/montecarlo/analyze.go study/montecarlo/analyze_test.go
git commit -m "feat: implement Monte Carlo analysis with percentile distributions and report"
```

---

### Task 7: Integration Test

**Files:**
- Modify: `study/integration_test.go`

- [ ] **Step 1: Write an integration test for Monte Carlo through the full pipeline**

Add to `study/integration_test.go`, following the existing stress test integration pattern:

```go
It("runs a Monte Carlo simulation through the engine and analysis pipeline", func() {
	assetAlpha := asset.Asset{CompositeFigi: "FIGI-ALPHA", Ticker: "ALPHA"}
	testAssets := []asset.Asset{assetAlpha}

	metrics := []data.Metric{
		data.MetricClose,
		data.AdjClose,
		data.Dividend,
		data.MetricHigh,
		data.MetricLow,
		data.SplitFactor,
	}

	dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	syntheticData := makeSyntheticDailyData(dataStart, 60, testAssets, metrics)
	testProvider := data.NewTestProvider(metrics, syntheticData)
	assetProvider := &integrationAssetProvider{assets: testAssets}

	mcStudy := montecarlo.New(syntheticData, metrics)
	mcStudy.Simulations = 5 // keep integration test fast
	mcStudy.StartDate = dataStart
	mcStudy.EndDate = dataStart.AddDate(0, 0, 59)
	mcStudy.InitialDeposit = 100_000.0

	runner := &study.Runner{
		Study: mcStudy,
		NewStrategy: func() engine.Strategy {
			return &buyAndHoldStrategy{targetAssets: testAssets}
		},
		Options: []engine.Option{
			engine.WithDataProvider(testProvider),
			engine.WithAssetProvider(assetProvider),
			engine.WithInitialDeposit(100_000.0),
		},
		Workers: 2,
	}

	progressCh, resultCh, runErr := runner.Run(context.Background())
	Expect(runErr).NotTo(HaveOccurred())

	// Drain progress.
	for range progressCh {
	}

	result := <-resultCh
	Expect(result.Err).NotTo(HaveOccurred())
	Expect(result.Runs).To(HaveLen(5))

	// All runs should succeed.
	for _, run := range result.Runs {
		Expect(run.Err).NotTo(HaveOccurred())
		Expect(run.Portfolio).NotTo(BeNil())
	}

	// Report should have sections.
	Expect(result.Report.Title).To(Equal("Monte Carlo Simulation"))
	Expect(result.Report.Sections).NotTo(BeEmpty())

	// Render in text and JSON.
	var textBuffer bytes.Buffer
	Expect(result.Report.Render(report.FormatText, &textBuffer)).To(Succeed())
	Expect(textBuffer.Len()).To(BeNumerically(">", 0))

	var jsonBuffer bytes.Buffer
	Expect(result.Report.Render(report.FormatJSON, &jsonBuffer)).To(Succeed())
	Expect(jsonBuffer.String()).To(ContainSubstring(`"title"`))
})
```

Add `"github.com/penny-vault/pvbt/study/montecarlo"` to imports.

- [ ] **Step 2: Run the integration test**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./study/ -run "Monte Carlo" -v -count=1`
Expected: PASS

- [ ] **Step 3: Run all tests to verify nothing is broken**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -count=1`
Expected: PASS

- [ ] **Step 4: Run linter on all changed packages**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./data/ ./study/ ./study/montecarlo/`
Expected: No errors. Fix any that appear.

- [ ] **Step 5: Commit**

```bash
git add study/integration_test.go
git commit -m "test: add Monte Carlo integration test through full runner pipeline"
```
