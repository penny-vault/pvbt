package engine

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// Test-only exports for black-box testing of dataCache.

// DataCacheForTest is a type alias for dataCache, allowing the black-box
// test file to hold references without knowing the concrete type name.
type DataCacheForTest = dataCache

// ColCacheEntryForTest is a type alias for colCacheEntry.
type ColCacheEntryForTest = colCacheEntry

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

// NewColCacheKeyForTest constructs a colCacheKey from exported parameters.
func NewColCacheKeyForTest(figi string, metric data.Metric, chunkStart int64) colCacheKey {
	return colCacheKey{figi: figi, metric: metric, chunkStart: chunkStart}
}

// NewColCacheEntryForTest constructs a colCacheEntry from exported parameters.
func NewColCacheEntryForTest(times []time.Time, values []float64) *colCacheEntry {
	return &colCacheEntry{times: times, values: values}
}

// EntryValuesForTest returns the values slice from a colCacheEntry.
func EntryValuesForTest(entry *colCacheEntry) []float64 {
	return entry.values
}

// CurBytesForTest returns the current byte count tracked by the cache.
func CurBytesForTest(cache *dataCache) int64 {
	return cache.curBytes
}

// GetForTest exposes dataCache.get.
func GetForTest(cache *dataCache, key colCacheKey) (*colCacheEntry, bool) {
	return cache.get(key)
}

// PutForTest exposes dataCache.put.
func PutForTest(cache *dataCache, key colCacheKey, entry *colCacheEntry) {
	cache.put(key, entry)
}

// EvictBeforeForTest exposes dataCache.evictBefore.
func EvictBeforeForTest(cache *dataCache, t time.Time) {
	cache.evictBefore(t)
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
