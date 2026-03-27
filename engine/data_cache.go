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

package engine

import (
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

const (
	defaultMaxBytes int64 = 512 * 1024 * 1024 // 512 MB
	daysPerChunk          = 366               // always 366 to avoid leap-year branching
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

// chunkCol identifies an (asset, metric) pair that has been fetched.
type chunkCol struct {
	figi   string
	metric data.Metric
}

// chunkEntry holds a full year of data in a flat slab indexed by calendar day.
// Layout: values[dayOffset * numAssets * numMetrics + assetIdx * numMetrics + metricIdx]
// dayOffset = (timestamp - baseUnix) / 86400, range [0, 365].
// Non-trading days and missing data are NaN.
type chunkEntry struct {
	baseUnix  int64               // Jan 1 00:00 Eastern, Unix seconds
	assets    []asset.Asset       // ordered asset list
	metrics   []data.Metric       // ordered metric list
	assetIdx  map[string]int      // figi -> index into assets
	metricIdx map[data.Metric]int // metric -> index into metrics
	fetched   map[chunkCol]bool   // tracks which (figi, metric) pairs have been fetched
	values    []float64           // flat slab
	times     []time.Time         // original provider timestamp per day slot (zero = no data)
	bytes     int64               // estimated memory footprint
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
		fetched:   make(map[chunkCol]bool, len(assets)*len(metrics)),
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

// set writes a value into the slab for the given asset, metric, and day offset.
func (ce *chunkEntry) set(day, aIdx, mIdx int, val float64) {
	ce.values[ce.valueIndex(day, aIdx, mIdx)] = val
}

// hasColumn returns true if the chunk has fetched data for the given (figi, metric) pair.
func (ce *chunkEntry) hasColumn(figi string, metric data.Metric) bool {
	return ce.fetched[chunkCol{figi: figi, metric: metric}]
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
