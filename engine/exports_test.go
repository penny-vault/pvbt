package engine

import (
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

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

// ChunkEntryGetForTest reads a value from the chunk slab by figi, metric, and timestamp.
func ChunkEntryGetForTest(ce *chunkEntry, figi string, metric data.Metric, unixSec int64) float64 {
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

// ChunkEntrySetForTest exposes chunkEntry.set with named parameters.
func ChunkEntrySetForTest(ce *chunkEntry, day, aIdx, mIdx int, val float64) {
	ce.set(day, aIdx, mIdx, val)
}

// ChunkEntryHasColumnForTest exposes chunkEntry.hasColumn.
func ChunkEntryHasColumnForTest(ce *chunkEntry, figi string, metric data.Metric) bool {
	return ce.hasColumn(figi, metric)
}

// MarkFetchedForTest marks a (figi, metric) pair as fetched in the chunk.
func MarkFetchedForTest(ce *chunkEntry, figi string, metric data.Metric) {
	ce.fetched[chunkCol{figi: figi, metric: metric}] = true
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

// WalkBackTradingDaysForTest exposes walkBackTradingDays.
var WalkBackTradingDaysForTest = walkBackTradingDays

// CollectStrategyAssetsForTest exposes collectStrategyAssets.
var CollectStrategyAssetsForTest func(strategy any, benchmark asset.Asset) []asset.Asset = collectStrategyAssets

// HydrateFieldsForTest exposes hydrateFields for white-box testing.
func HydrateFieldsForTest(eng *Engine, target interface{}) error {
	return hydrateFields(eng, target)
}

// ChildEntryForTest is a type alias for childEntry.
type ChildEntryForTest = childEntry

// DiscoverChildrenForTest exposes discoverChildren for black-box testing.
func DiscoverChildrenForTest(eng *Engine, parentStrategy Strategy, visited map[uintptr]bool) error {
	return eng.discoverChildren(parentStrategy, visited)
}

// EngineChildrenForTest returns the engine's discovered children slice.
func EngineChildrenForTest(eng *Engine) []*childEntry {
	return eng.children
}

// EngineChildrenByNameForTest returns the engine's childrenByName map.
func EngineChildrenByNameForTest(eng *Engine) map[string]*childEntry {
	return eng.childrenByName
}

// ChildEntryStrategy returns the strategy from a childEntry.
func ChildEntryStrategy(entry *childEntry) Strategy {
	return entry.strategy
}

// ChildEntryName returns the name from a childEntry.
func ChildEntryName(entry *childEntry) string {
	return entry.name
}

// ChildEntryWeight returns the weight from a childEntry.
func ChildEntryWeight(entry *childEntry) float64 {
	return entry.weight
}

// SetChildrenForTest directly injects children into the engine for unit tests
// that need to verify ChildAllocations and ChildPortfolios without running a
// full backtest.
func SetChildrenForTest(eng *Engine, entries []*childEntry) {
	eng.children = entries
	eng.childrenByName = make(map[string]*childEntry, len(entries))
	for _, entry := range entries {
		eng.childrenByName[entry.name] = entry
	}
}

// NewChildEntryForTest constructs a childEntry for use in unit tests.
func NewChildEntryForTest(name string, weight float64, account portfolio.PortfolioManager) *childEntry {
	return &childEntry{
		name:    name,
		weight:  weight,
		account: account,
	}
}

// NewChildEntryWithStrategyForTest constructs a childEntry with a strategy for use in unit tests.
func NewChildEntryWithStrategyForTest(name string, weight float64, strategy Strategy) *childEntry {
	return &childEntry{
		name:     name,
		weight:   weight,
		strategy: strategy,
	}
}

// SetEngineDateForTest sets the engine's currentDate for unit tests.
func SetEngineDateForTest(eng *Engine, date time.Time) {
	eng.currentDate = date
}

// BuildMiddlewareFromConfigForTest exposes buildMiddlewareFromConfig for testing.
func BuildMiddlewareFromConfigForTest(eng *Engine) error {
	return eng.buildMiddlewareFromConfig()
}

// EngineMiddlewareConfigForTest returns the engine's middlewareConfig for testing.
func EngineMiddlewareConfigForTest(eng *Engine) *MiddlewareConfig {
	return eng.middlewareConfig
}

// SetAccountForTest sets the engine's account directly for unit tests that
// need to inspect middleware registrations without running a full backtest.
func SetAccountForTest(eng *Engine, acct portfolio.PortfolioManager) {
	eng.account = acct
}
