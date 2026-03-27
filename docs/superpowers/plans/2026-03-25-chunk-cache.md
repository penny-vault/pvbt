# Chunk-Level Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the per-column data cache with a chunk-level cache using calendar-day-indexed slabs to eliminate map overhead and GC pressure in fetchRange.

**Architecture:** Each year chunk stores a flat `[]float64` slab indexed by `dayOffset * numAssets * numMetrics + assetIdx * numMetrics + metricIdx` with 366 day slots. Lookups use pure arithmetic instead of maps. The slab expands (reallocate + scatter) when new assets/metrics are added. Assembly extracts a date range directly from the slab without sorting or building intermediate maps.

**Tech Stack:** Go, Ginkgo/Gomega

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `engine/data_cache.go` | Rewrite | New `chunkEntry` struct with slab, `dataCache` with chunk-level get/put/evict/expand |
| `engine/exports_test.go` | Rewrite | New test exports for chunk-level cache API |
| `engine/data_cache_test.go` | Rewrite | Tests for chunk slab indexing, expansion, eviction, assembly |
| `engine/engine.go` | Modify | Rewrite `fetchRange` to use chunk cache |

---

### Task 1: Implement the chunk cache data structure

This task replaces the entire `data_cache.go` file and its test exports. The old per-column types (`colCacheKey`, `colCacheEntry`) are removed.

**Files:**
- Rewrite: `engine/data_cache.go`
- Rewrite: `engine/exports_test.go` (cache-related exports only; keep other exports)

- [ ] **Step 1: Rewrite `engine/data_cache.go`**

Replace the entire file with:

```go
package engine

import (
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

const (
	defaultMaxBytes int64 = 512 * 1024 * 1024 // 512 MB
	daysPerChunk          = 366                // always 366 to avoid leap-year branching
	secondsPerDay         = 86400
)

var nyc *time.Location

func init() {
	var err error

	nyc, err = time.LoadLocation("America/New_York")
	if err != nil {
		panic("engine: load America/New_York: " + err.Error())
	}
}

// chunkCol identifies an (asset, metric) pair within a chunk.
type chunkCol struct {
	figi   string
	metric data.Metric
}

// chunkEntry holds a full year of data in a flat slab indexed by calendar day.
// Layout: values[dayOffset * numAssets * numMetrics + assetIdx * numMetrics + metricIdx]
// dayOffset = (timestamp - baseUnix) / 86400, range [0, 365].
// Non-trading days and missing data are NaN.
type chunkEntry struct {
	baseUnix  int64              // Jan 1 00:00 Eastern, Unix seconds
	assets    []asset.Asset      // ordered asset list
	metrics   []data.Metric      // ordered metric list
	assetIdx  map[string]int     // figi -> index into assets
	metricIdx map[data.Metric]int // metric -> index into metrics
	values    []float64          // flat slab
	times     []time.Time        // original provider timestamp per day slot (zero = no data)
	bytes     int64              // estimated memory footprint
}

// newChunkEntry creates a chunk for the given year with the specified assets
// and metrics. The slab is initialized to NaN.
func newChunkEntry(yearStart int64, assets []asset.Asset, metrics []data.Metric) *chunkEntry {
	ce := &chunkEntry{
		baseUnix:  yearStart,
		assets:    make([]asset.Asset, len(assets)),
		metrics:   make([]data.Metric, len(metrics)),
		assetIdx:  make(map[string]int, len(assets)),
		metricIdx: make(map[data.Metric]int, len(metrics)),
	}

	copy(ce.assets, assets)
	copy(ce.metrics, metrics)

	for idx, ast := range assets {
		ce.assetIdx[ast.CompositeFigi] = idx
	}

	for idx, met := range metrics {
		ce.metricIdx[met] = idx
	}

	ce.values = make([]float64, daysPerChunk*len(assets)*len(metrics))
	for idx := range ce.values {
		ce.values[idx] = math.NaN()
	}

	ce.times = make([]time.Time, daysPerChunk)
	ce.bytes = int64(len(ce.values)*8 + daysPerChunk*24)

	return ce
}

// dayOffset returns the calendar day offset for a Unix timestamp relative to
// the chunk's base. Returns -1 if out of range.
func (ce *chunkEntry) dayOffset(unixSec int64) int {
	off := int((unixSec - ce.baseUnix) / secondsPerDay)
	if off < 0 || off >= daysPerChunk {
		return -1
	}

	return off
}

// stride returns the number of float64 values per day.
func (ce *chunkEntry) stride() int {
	return len(ce.assets) * len(ce.metrics)
}

// valueIndex returns the flat slab index for (dayOffset, assetIdx, metricIdx).
func (ce *chunkEntry) valueIndex(day, aIdx, mIdx int) int {
	return day*ce.stride() + aIdx*len(ce.metrics) + mIdx
}

// get returns the value for the given asset, metric, and Unix timestamp.
// Returns NaN if any index is out of range or the column is not present.
func (ce *chunkEntry) get(figi string, metric data.Metric, unixSec int64) float64 {
	aIdx, aOK := ce.assetIdx[figi]
	mIdx, mOK := ce.metricIdx[metric]

	if !aOK || !mOK {
		return math.NaN()
	}

	day := ce.dayOffset(unixSec)
	if day < 0 {
		return math.NaN()
	}

	return ce.values[ce.valueIndex(day, aIdx, mIdx)]
}

// set writes a value into the slab for the given asset, metric, and day offset.
func (ce *chunkEntry) set(day, aIdx, mIdx int, val float64) {
	ce.values[ce.valueIndex(day, aIdx, mIdx)] = val
}

// hasColumn returns true if the chunk contains data for the given (figi, metric).
func (ce *chunkEntry) hasColumn(figi string, metric data.Metric) bool {
	_, aOK := ce.assetIdx[figi]
	_, mOK := ce.metricIdx[metric]

	return aOK && mOK
}

// expand grows the chunk to include additional assets and/or metrics.
// Existing values are scattered into the new layout. New slots are NaN.
func (ce *chunkEntry) expand(newAssets []asset.Asset, newMetrics []data.Metric) {
	// Build merged asset list: existing + new.
	mergedAssets := make([]asset.Asset, len(ce.assets))
	copy(mergedAssets, ce.assets)

	for _, ast := range newAssets {
		if _, ok := ce.assetIdx[ast.CompositeFigi]; !ok {
			mergedAssets = append(mergedAssets, ast)
		}
	}

	// Build merged metric list: existing + new.
	mergedMetrics := make([]data.Metric, len(ce.metrics))
	copy(mergedMetrics, ce.metrics)

	for _, met := range newMetrics {
		if _, ok := ce.metricIdx[met]; !ok {
			mergedMetrics = append(mergedMetrics, met)
		}
	}

	// If nothing changed, return early.
	if len(mergedAssets) == len(ce.assets) && len(mergedMetrics) == len(ce.metrics) {
		return
	}

	// Build new index maps.
	newAssetIdx := make(map[string]int, len(mergedAssets))
	for idx, ast := range mergedAssets {
		newAssetIdx[ast.CompositeFigi] = idx
	}

	newMetricIdx := make(map[data.Metric]int, len(mergedMetrics))
	for idx, met := range mergedMetrics {
		newMetricIdx[met] = idx
	}

	// Allocate new slab and fill with NaN.
	newStride := len(mergedAssets) * len(mergedMetrics)
	newValues := make([]float64, daysPerChunk*newStride)

	for idx := range newValues {
		newValues[idx] = math.NaN()
	}

	// Scatter old values into new layout.
	oldStride := ce.stride()

	for day := range daysPerChunk {
		for oldAIdx, ast := range ce.assets {
			newAIdx := newAssetIdx[ast.CompositeFigi]

			for oldMIdx, met := range ce.metrics {
				newMIdx := newMetricIdx[met]
				oldIdx := day*oldStride + oldAIdx*len(ce.metrics) + oldMIdx
				newIdx := day*newStride + newAIdx*len(mergedMetrics) + newMIdx
				newValues[newIdx] = ce.values[oldIdx]
			}
		}
	}

	ce.assets = mergedAssets
	ce.metrics = mergedMetrics
	ce.assetIdx = newAssetIdx
	ce.metricIdx = newMetricIdx
	ce.values = newValues
	// ce.times is unchanged -- day slots are the same, only values layout changes.
	ce.bytes = int64(len(newValues)*8 + daysPerChunk*24)
}

// dataCache holds chunk entries keyed by year start (Unix seconds).
type dataCache struct {
	chunks   map[int64]*chunkEntry
	curBytes int64
	maxBytes int64
}

func newDataCache(maxBytes int64) *dataCache {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}

	return &dataCache{
		chunks:   make(map[int64]*chunkEntry),
		maxBytes: maxBytes,
	}
}

// getChunk returns the chunk entry for the given year, or nil if not cached.
func (dc *dataCache) getChunk(yearStart int64) *chunkEntry {
	return dc.chunks[yearStart]
}

// putChunk stores a chunk entry, updating byte tracking.
func (dc *dataCache) putChunk(yearStart int64, ce *chunkEntry) {
	if old, ok := dc.chunks[yearStart]; ok {
		dc.curBytes -= old.bytes
	}

	dc.chunks[yearStart] = ce
	dc.curBytes += ce.bytes
}

// evictBefore removes all chunks whose year is more than one year before t.
func (dc *dataCache) evictBefore(t time.Time) {
	year := t.In(nyc).Year()

	for yearStart, ce := range dc.chunks {
		chunkYear := time.Unix(yearStart, 0).In(nyc).Year()
		if chunkYear < year-1 {
			dc.curBytes -= ce.bytes
			delete(dc.chunks, yearStart)
		}
	}
}

// chunkYears returns the chunkStart values (Unix seconds) for every
// calendar year that overlaps [start, end].
func chunkYears(start, end time.Time) []int64 {
	startYear := start.In(nyc).Year()
	endYear := end.In(nyc).Year()

	years := make([]int64, 0, endYear-startYear+1)

	for yr := startYear; yr <= endYear; yr++ {
		jan1 := time.Date(yr, 1, 1, 0, 0, 0, 0, nyc)
		years = append(years, jan1.Unix())
	}

	return years
}
```

- [ ] **Step 2: Update test exports in `engine/exports_test.go`**

Replace the cache-related exports (lines 12-72: `DataCacheForTest` through `EvictBeforeForTest`) with new chunk-level exports. Keep all other exports unchanged (lines 74 onward).

```go
// Test-only exports for black-box testing of dataCache.

// DataCacheForTest is a type alias for dataCache.
type DataCacheForTest = dataCache

// ChunkEntryForTest is a type alias for chunkEntry.
type ChunkEntryForTest = chunkEntry

// NewDataCacheForTest exposes newDataCache.
var NewDataCacheForTest = newDataCache

// ChunkYearsForTest exposes chunkYears.
var ChunkYearsForTest = chunkYears

// NYCForTest returns the engine-internal nyc time.Location.
func NYCForTest() *time.Location {
	return nyc
}

// ChunkStartForTest returns the Unix seconds of Jan 1 00:00 Eastern for the
// year containing t (in Eastern time).
func ChunkStartForTest(t time.Time) int64 {
	et := t.In(nyc)
	jan1 := time.Date(et.Year(), 1, 1, 0, 0, 0, 0, nyc)
	return jan1.Unix()
}

// NewChunkEntryForTest exposes newChunkEntry.
var NewChunkEntryForTest = newChunkEntry

// ChunkEntryGetForTest exposes chunkEntry.get.
func ChunkEntryGetForTest(ce *chunkEntry, figi string, metric data.Metric, unixSec int64) float64 {
	return ce.get(figi, metric, unixSec)
}

// ChunkEntrySetForTest exposes chunkEntry.set with named parameters.
func ChunkEntrySetForTest(ce *chunkEntry, day, aIdx, mIdx int, val float64) {
	ce.set(day, aIdx, mIdx, val)
}

// ChunkEntryHasColumnForTest exposes chunkEntry.hasColumn.
func ChunkEntryHasColumnForTest(ce *chunkEntry, figi string, metric data.Metric) bool {
	return ce.hasColumn(figi, metric)
}

// ChunkEntryExpandForTest exposes chunkEntry.expand.
func ChunkEntryExpandForTest(ce *chunkEntry, newAssets []asset.Asset, newMetrics []data.Metric) {
	ce.expand(newAssets, newMetrics)
}

// GetChunkForTest exposes dataCache.getChunk.
func GetChunkForTest(dc *dataCache, yearStart int64) *chunkEntry {
	return dc.getChunk(yearStart)
}

// PutChunkForTest exposes dataCache.putChunk.
func PutChunkForTest(dc *dataCache, yearStart int64, ce *chunkEntry) {
	dc.putChunk(yearStart, ce)
}

// EvictBeforeForTest exposes dataCache.evictBefore.
func EvictBeforeForTest(dc *dataCache, t time.Time) {
	dc.evictBefore(t)
}

// CurBytesForTest returns the current byte count tracked by the cache.
func CurBytesForTest(dc *dataCache) int64 {
	return dc.curBytes
}

// DayOffsetForTest exposes chunkEntry.dayOffset.
func DayOffsetForTest(ce *chunkEntry, unixSec int64) int {
	return ce.dayOffset(unixSec)
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./engine/`
Expected: compilation errors from `fetchRange` which still references old types. That's expected -- Task 2 fixes it.

- [ ] **Step 4: Commit**

```bash
git add engine/data_cache.go engine/exports_test.go
git commit -m "engine: replace per-column cache with chunk-level slab cache"
```

---

### Task 2: Rewrite cache tests

**Files:**
- Rewrite: `engine/data_cache_test.go`

- [ ] **Step 1: Rewrite cache tests**

Replace the entire `engine/data_cache_test.go` with tests for the new chunk structure:

```go
package engine_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
)

var _ = Describe("dataCache", func() {
	var nyc *time.Location

	BeforeEach(func() {
		nyc = engine.NYCForTest()
	})

	Describe("ChunkStartForTest", func() {
		It("maps a mid-year date to Jan 1 of that year in Eastern time", func() {
			date := time.Date(2025, 6, 15, 16, 0, 0, 0, nyc)
			got := engine.ChunkStartForTest(date)
			want := time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()
			Expect(got).To(Equal(want))
		})

		It("converts UTC to Eastern before computing the chunk year", func() {
			date := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
			got := engine.ChunkStartForTest(date)
			want := time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()
			Expect(got).To(Equal(want))
		})
	})

	Describe("chunkYears", func() {
		It("returns chunks for each year spanned", func() {
			start := time.Date(2025, 11, 1, 16, 0, 0, 0, nyc)
			end := time.Date(2026, 2, 1, 16, 0, 0, 0, nyc)
			got := engine.ChunkYearsForTest(start, end)
			Expect(got).To(HaveLen(2))
			Expect(got[0]).To(Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc).Unix()))
			Expect(got[1]).To(Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, nyc).Unix()))
		})

		It("returns a single chunk when start and end are in the same year", func() {
			start := time.Date(2025, 3, 1, 16, 0, 0, 0, nyc)
			end := time.Date(2025, 9, 1, 16, 0, 0, 0, nyc)
			got := engine.ChunkYearsForTest(start, end)
			Expect(got).To(HaveLen(1))
		})
	})

	Describe("chunkEntry", func() {
		var (
			aapl    asset.Asset
			msft    asset.Asset
			yr2025  int64
		)

		BeforeEach(func() {
			aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
			msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
			yr2025 = engine.ChunkStartForTest(time.Date(2025, 1, 1, 0, 0, 0, 0, nyc))
		})

		It("initializes all values to NaN", func() {
			ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.MetricClose,
				time.Date(2025, 6, 15, 16, 0, 0, 0, nyc).Unix())
			Expect(math.IsNaN(val)).To(BeTrue())
		})

		It("stores and retrieves a value by day offset", func() {
			ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			day := engine.DayOffsetForTest(ce, time.Date(2025, 6, 15, 16, 0, 0, 0, nyc).Unix())
			engine.ChunkEntrySetForTest(ce, day, 0, 0, 150.0)
			val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.MetricClose,
				time.Date(2025, 6, 15, 16, 0, 0, 0, nyc).Unix())
			Expect(val).To(Equal(150.0))
		})

		It("returns NaN for an unknown asset", func() {
			ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			val := engine.ChunkEntryGetForTest(ce, "FIGI-UNKNOWN", data.MetricClose,
				time.Date(2025, 6, 15, 16, 0, 0, 0, nyc).Unix())
			Expect(math.IsNaN(val)).To(BeTrue())
		})

		It("returns NaN for an unknown metric", func() {
			ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.Volume,
				time.Date(2025, 6, 15, 16, 0, 0, 0, nyc).Unix())
			Expect(math.IsNaN(val)).To(BeTrue())
		})

		It("detects present and missing columns", func() {
			ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			Expect(engine.ChunkEntryHasColumnForTest(ce, "FIGI-AAPL", data.MetricClose)).To(BeTrue())
			Expect(engine.ChunkEntryHasColumnForTest(ce, "FIGI-AAPL", data.Volume)).To(BeFalse())
			Expect(engine.ChunkEntryHasColumnForTest(ce, "FIGI-MSFT", data.MetricClose)).To(BeFalse())
		})

		Context("expand", func() {
			It("adds new assets and preserves existing values", func() {
				ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
				day := engine.DayOffsetForTest(ce, time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				engine.ChunkEntrySetForTest(ce, day, 0, 0, 150.0)

				engine.ChunkEntryExpandForTest(ce, []asset.Asset{msft}, nil)

				// Old value preserved.
				val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.MetricClose,
					time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				Expect(val).To(Equal(150.0))

				// New asset present but NaN.
				Expect(engine.ChunkEntryHasColumnForTest(ce, "FIGI-MSFT", data.MetricClose)).To(BeTrue())
				val2 := engine.ChunkEntryGetForTest(ce, "FIGI-MSFT", data.MetricClose,
					time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				Expect(math.IsNaN(val2)).To(BeTrue())
			})

			It("adds new metrics and preserves existing values", func() {
				ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
				day := engine.DayOffsetForTest(ce, time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				engine.ChunkEntrySetForTest(ce, day, 0, 0, 150.0)

				engine.ChunkEntryExpandForTest(ce, nil, []data.Metric{data.Volume})

				// Old value preserved.
				val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.MetricClose,
					time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				Expect(val).To(Equal(150.0))

				// New metric present but NaN.
				Expect(engine.ChunkEntryHasColumnForTest(ce, "FIGI-AAPL", data.Volume)).To(BeTrue())
			})

			It("is a no-op when all assets and metrics already present", func() {
				ce := engine.NewChunkEntryForTest(yr2025, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
				day := engine.DayOffsetForTest(ce, time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				engine.ChunkEntrySetForTest(ce, day, 0, 0, 150.0)

				engine.ChunkEntryExpandForTest(ce, []asset.Asset{aapl}, []data.Metric{data.MetricClose})

				val := engine.ChunkEntryGetForTest(ce, "FIGI-AAPL", data.MetricClose,
					time.Date(2025, 3, 15, 16, 0, 0, 0, nyc).Unix())
				Expect(val).To(Equal(150.0))
			})
		})
	})

	Describe("dataCache", func() {
		It("returns nil for an unknown chunk", func() {
			dc := engine.NewDataCacheForTest(0)
			Expect(engine.GetChunkForTest(dc, 0)).To(BeNil())
		})

		It("stores and retrieves a chunk", func() {
			dc := engine.NewDataCacheForTest(0)
			aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
			yr := engine.ChunkStartForTest(time.Date(2025, 1, 1, 0, 0, 0, 0, engine.NYCForTest()))
			ce := engine.NewChunkEntryForTest(yr, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			engine.PutChunkForTest(dc, yr, ce)
			Expect(engine.GetChunkForTest(dc, yr)).To(Equal(ce))
		})

		It("tracks bytes", func() {
			dc := engine.NewDataCacheForTest(0)
			aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
			yr := engine.ChunkStartForTest(time.Date(2025, 1, 1, 0, 0, 0, 0, engine.NYCForTest()))
			ce := engine.NewChunkEntryForTest(yr, []asset.Asset{aapl}, []data.Metric{data.MetricClose})
			engine.PutChunkForTest(dc, yr, ce)
			Expect(engine.CurBytesForTest(dc)).To(Equal(int64(366 * 1 * 1 * 8)))
		})

		It("evicts old chunks but keeps previous and current year", func() {
			dc := engine.NewDataCacheForTest(0)
			loc := engine.NYCForTest()
			aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}

			yr2023 := engine.ChunkStartForTest(time.Date(2023, 6, 1, 0, 0, 0, 0, loc))
			yr2024 := engine.ChunkStartForTest(time.Date(2024, 6, 1, 0, 0, 0, 0, loc))
			yr2025 := engine.ChunkStartForTest(time.Date(2025, 6, 1, 0, 0, 0, 0, loc))

			for _, yr := range []int64{yr2023, yr2024, yr2025} {
				engine.PutChunkForTest(dc, yr, engine.NewChunkEntryForTest(yr, []asset.Asset{aapl}, []data.Metric{data.MetricClose}))
			}

			engine.EvictBeforeForTest(dc, time.Date(2025, 3, 1, 0, 0, 0, 0, loc))

			Expect(engine.GetChunkForTest(dc, yr2023)).To(BeNil(), "expected 2023 chunk evicted")
			Expect(engine.GetChunkForTest(dc, yr2024)).NotTo(BeNil(), "expected 2024 chunk kept")
			Expect(engine.GetChunkForTest(dc, yr2025)).NotTo(BeNil(), "expected 2025 chunk kept")
		})
	})
})
```

- [ ] **Step 2: Run cache tests**

Run: `ginkgo run -race -focus "dataCache" ./engine/`
Expected: PASS (the cache tests are self-contained -- they don't touch fetchRange)

- [ ] **Step 3: Commit**

```bash
git add engine/data_cache_test.go
git commit -m "engine: rewrite cache tests for chunk-level slab structure"
```

---

### Task 3: Rewrite fetchRange to use chunk cache

This is the core task. The `fetchRange` method in `engine/engine.go` (lines 420-606) is rewritten to use chunk-level miss detection, fetching, and slab-based assembly.

**Files:**
- Modify: `engine/engine.go:420-606`

- [ ] **Step 1: Rewrite fetchRange**

Replace the `fetchRange` method (lines 420-606 in `engine/engine.go`) with:

```go
// fetchRange is the shared implementation for Fetch and FetchAt.
// It checks the chunk-level cache, bulk-fetches misses grouped by
// calendar-year chunk, and assembles the result from slab-indexed chunks.
func (e *Engine) fetchRange(ctx context.Context, assets []asset.Asset, metrics []data.Metric, rangeStart, rangeEnd time.Time) (*data.DataFrame, error) {
	log := zerolog.Ctx(ctx)

	years := chunkYears(rangeStart, rangeEnd)

	// Identify cache misses grouped by chunk year.
	type chunkMiss struct {
		assets  map[string]asset.Asset
		metrics map[data.Metric]bool
	}

	misses := make(map[int64]*chunkMiss)

	for _, year := range years {
		chunk := e.cache.getChunk(year)

		for _, assetItem := range assets {
			for _, metric := range metrics {
				if chunk != nil && chunk.hasColumn(assetItem.CompositeFigi, metric) {
					continue
				}

				miss, exists := misses[year]
				if !exists {
					miss = &chunkMiss{
						assets:  make(map[string]asset.Asset),
						metrics: make(map[data.Metric]bool),
					}
					misses[year] = miss
				}

				miss.assets[assetItem.CompositeFigi] = assetItem
				miss.metrics[metric] = true
			}
		}
	}

	// Fetch misses: one bulk call per chunk year.
	for year, miss := range misses {
		missAssets := make([]asset.Asset, 0, len(miss.assets))
		for _, assetItem := range miss.assets {
			missAssets = append(missAssets, assetItem)
		}

		missMetrics := make([]data.Metric, 0, len(miss.metrics))
		for metric := range miss.metrics {
			missMetrics = append(missMetrics, metric)
		}

		chunkStart := time.Unix(year, 0).In(nyc)
		chunkEnd := time.Date(chunkStart.Year()+1, 1, 1, 0, 0, 0, 0, nyc).Add(-time.Nanosecond)

		log.Debug().
			Int("missAssets", len(missAssets)).
			Int("missMetrics", len(missMetrics)).
			Time("chunkStart", chunkStart).
			Time("chunkEnd", chunkEnd).
			Msg("engine.fetchRange cache miss")

		df, err := e.fetchFromProviders(ctx, missAssets, missMetrics, chunkStart, chunkEnd)
		if err != nil {
			return nil, err
		}

		// Get or create chunk entry.
		chunk := e.cache.getChunk(year)
		if chunk == nil {
			chunk = newChunkEntry(year, missAssets, missMetrics)
			e.cache.putChunk(year, chunk)
		} else {
			chunk.expand(missAssets, missMetrics)
			e.cache.curBytes = e.cache.curBytes - chunk.bytes + int64(len(chunk.values)*8)
			chunk.bytes = int64(len(chunk.values) * 8)
		}

		// Scatter provider results into the chunk slab.
		dfTimes := df.Times()
		dfAssets := df.AssetList()
		dfMetrics := df.MetricList()

		for _, ast := range dfAssets {
			aIdx, aOK := chunk.assetIdx[ast.CompositeFigi]
			if !aOK {
				continue
			}

			for _, met := range dfMetrics {
				mIdx, mOK := chunk.metricIdx[met]
				if !mOK {
					continue
				}

				col := df.Column(ast, met)
				if col == nil {
					continue
				}

				for ti, ts := range dfTimes {
					day := chunk.dayOffset(ts.Unix())
					if day >= 0 {
						chunk.set(day, aIdx, mIdx, col[ti])
						chunk.times[day] = ts
					}
				}
			}
		}
	}

	// Assemble result DataFrame from chunk slabs.
	startDay := rangeStart.In(nyc)
	endDay := rangeEnd.In(nyc)

	// Collect trading days and values.
	var resultTimes []time.Time
	numCols := len(assets) * len(metrics)
	var resultSlab []float64

	for _, year := range years {
		chunk := e.cache.getChunk(year)
		if chunk == nil {
			continue
		}

		// Determine day range within this chunk.
		chunkJan1 := time.Unix(year, 0).In(nyc)
		dayStart := 0
		dayEnd := daysPerChunk - 1

		if startDay.Year() == chunkJan1.Year() {
			dayStart = int(startDay.Sub(chunkJan1).Hours() / 24)
			if dayStart < 0 {
				dayStart = 0
			}
		}

		if endDay.Year() == chunkJan1.Year() {
			dayEnd = int(endDay.Sub(chunkJan1).Hours() / 24)
			if dayEnd >= daysPerChunk {
				dayEnd = daysPerChunk - 1
			}
		}

		for day := dayStart; day <= dayEnd; day++ {
			// Skip days with no provider data (weekends, holidays).
			if chunk.times[day].IsZero() {
				continue
			}

			// Use the original provider timestamp for this day.
			resultTimes = append(resultTimes, chunk.times[day])

			for _, assetItem := range assets {
				aIdx, aOK := chunk.assetIdx[assetItem.CompositeFigi]

				for _, metric := range metrics {
					mIdx, mOK := chunk.metricIdx[metric]

					if aOK && mOK {
						resultSlab = append(resultSlab, chunk.values[chunk.valueIndex(day, aIdx, mIdx)])
					} else {
						resultSlab = append(resultSlab, math.NaN())
					}
				}
			}
		}
	}

	if len(resultTimes) == 0 {
		return data.NewDataFrame(nil, nil, nil, data.Daily, nil)
	}

	columns := data.SlabToColumns(resultSlab, numCols, len(resultTimes))
	assembled, err := data.NewDataFrame(resultTimes, assets, metrics, data.Daily, columns)
	if err != nil {
		return nil, fmt.Errorf("engine: assemble cached data: %w", err)
	}

	log.Debug().
		Int("len", assembled.Len()).
		Int("assets", len(assembled.AssetList())).
		Int("metrics", len(assembled.MetricList())).
		Msg("engine.Fetch final result")

	return assembled, nil
}
```

- [ ] **Step 2: Remove the `sort` import if no longer used**

Check if `sort` is still used elsewhere in `engine/engine.go`. If not, remove it from the import block.

- [ ] **Step 3: Run engine tests**

Run: `ginkgo run -race ./engine/`
Expected: PASS

- [ ] **Step 4: Run full test suite**

Run: `ginkgo run -race --skip-package=broker/ibkr ./...`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add engine/engine.go
git commit -m "engine: rewrite fetchRange to use chunk-level slab cache"
```

---

### Task 4: Lint and final verification

**Files:**
- All modified files

- [ ] **Step 1: Run linter**

Run: `make lint`
Expected: 0 issues. Fix any that appear.

- [ ] **Step 2: Run full test suite**

Run: `make test`
Expected: all tests pass.

- [ ] **Step 3: Fix any issues and commit**

If lint or tests revealed issues, fix and commit:

```bash
git add -u
git commit -m "fix: address lint issues from chunk cache rewrite"
```
