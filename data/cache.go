// Copyright 2021-2022
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
	"sort"
	"sync"
	"time"

	"github.com/alphadose/haxmap"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type CacheItem struct {
	Values        []float64
	Period        *Interval
	CoveredPeriod *Interval
	isLocalDate   bool
	localDates    []time.Time
	startIdx      int
}

type SecurityMetricCache struct {
	sizeBytes    int64
	maxSizeBytes int64
	values       map[string][]*CacheItem
	lastSeen     *haxmap.Map[string, time.Time]
	periods      []time.Time
	locker       sync.RWMutex
}

type pair struct {
	key  string
	last time.Time
}

// ByDate implements sort.Interface for []pair based on
// `last` the time field.
type ByDate []pair

func (a ByDate) Len() int           { return len(a) }
func (a ByDate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDate) Less(i, j int) bool { return a[i].last.Before(a[j].last) }

// BySecurityMetric implements sort.Interface for []SecurityMetric
type BySecurityMetric []SecurityMetric

func (a BySecurityMetric) Len() int      { return len(a) }
func (a BySecurityMetric) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a BySecurityMetric) Less(i, j int) bool {
	return a[i].String() < a[j].String()
}

// Functions

func dateOnly(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
}

func NewSecurityMetricCache(sz int64, periods []time.Time) *SecurityMetricCache {
	return &SecurityMetricCache{
		sizeBytes:    0,
		maxSizeBytes: sz,
		values:       make(map[string][]*CacheItem, 10),
		lastSeen:     haxmap.New[string, time.Time](),
		periods:      periods,
		locker:       sync.RWMutex{},
	}
}

// Check returns if the requested range is in the cache. If the range is not completely covered by the cache
// returns false and a list of intervals covered by the cache that partially match the requested range.
func (cache *SecurityMetricCache) Check(security *Security, metric Metric, begin, end time.Time) (bool, []*Interval) {
	cache.locker.RLock()
	defer cache.locker.RUnlock()

	k := SecurityMetric{
		SecurityObject: *security,
		MetricObject:   metric,
	}

	requestedInterval := &Interval{
		Begin: begin,
		End:   end,
	}

	if err := requestedInterval.Valid(); err != nil {
		log.Error().Err(err).Time("Begin", requestedInterval.Begin).Time("End", requestedInterval.End).Msg("cannot set cache value with invalid interval")
		return false, []*Interval{}
	}

	touchingIntervals := []*Interval{}
	if items, ok := cache.values[k.String()]; ok {
		for _, item := range items {
			if item.Period.Contains(requestedInterval) {
				return true, []*Interval{item.Period}
			}
			if item.Period.Overlaps(requestedInterval) {
				touchingIntervals = append(touchingIntervals, item.Period)
			}
		}
	}

	return false, touchingIntervals
}

// Get returns the requested data over the range. If no data matches the hash key return ErrRangeDoesNotExist
func (cache *SecurityMetricCache) Get(security *Security, metric Metric, begin, end time.Time) (*dataframe.DataFrame, error) {
	cache.locker.RLock()
	defer cache.locker.RUnlock()

	begin = dateOnly(begin)
	end = dateOnly(end)

	k := SecurityMetric{
		SecurityObject: *security,
		MetricObject:   metric,
	}

	requestedInterval := &Interval{
		Begin: begin,
		End:   end,
	}

	if err := requestedInterval.Valid(); err != nil {
		log.Error().Err(err).Time("Begin", requestedInterval.Begin).Time("End", requestedInterval.End).Msg("cannot set cache value with invalid interval")
		return nil, ErrInvalidTimeRange
	}

	myKey := k.String()

	if items, ok := cache.values[myKey]; ok {
		for _, item := range items {
			if item.Period.Contains(requestedInterval) {
				var periodSubset []time.Time
				if item.isLocalDate {
					periodSubset = item.localDates
				} else {
					periodSubset = cache.periods[item.startIdx:]
					if !item.Period.Begin.Equal(item.CoveredPeriod.Begin) {
						coveredPeriodStart := sort.Search(len(periodSubset), func(i int) bool {
							idxVal := periodSubset[i]
							return (idxVal.After(item.CoveredPeriod.Begin) || idxVal.Equal(item.CoveredPeriod.Begin))
						})
						periodSubset = periodSubset[coveredPeriodStart:]
					}
				}

				var beginIdx int
				var endIdx int

				endExactMatch := false
				noValuesFound := false

				if end.After(item.CoveredPeriod.End) {
					end = item.CoveredPeriod.End
				}

				if item.CoveredPeriod.Begin.Equal(begin) {
					beginIdx = 0
				} else {
					beginIdx = sort.Search(len(periodSubset), func(i int) bool {
						idxVal := periodSubset[i]
						return (idxVal.After(begin) || idxVal.Equal(begin))
					})
					if begin.After(item.CoveredPeriod.End) {
						noValuesFound = true
					}
					if beginIdx == len(periodSubset) && item.isLocalDate {
						noValuesFound = true
					}
				}

				if item.CoveredPeriod.End.Equal(end) {
					endIdx = len(item.Values) - 1
					endExactMatch = true
				} else {
					endIdx = sort.Search(len(periodSubset), func(i int) bool {
						idxVal := periodSubset[i]
						return (idxVal.After(end) || idxVal.Equal(end))
					})
					if end.Before(item.CoveredPeriod.Begin) {
						noValuesFound = true
					}
					if endIdx == len(periodSubset) {
						endIdx -= 1
					}
					if periodSubset[endIdx].Equal(end) {
						endExactMatch = true
					}
				}

				if !endExactMatch && endIdx != 0 && beginIdx != endIdx {
					endIdx--
				}

				endModified := time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 999999999, common.GetTimezone())
				if beginIdx < len(periodSubset) && periodSubset[beginIdx].After(endModified) {
					noValuesFound = true
				}

				vals := make([][]float64, 1)
				var dates []time.Time
				if noValuesFound {
					vals[0] = []float64{}
					dates = []time.Time{}
				} else {
					vals[0] = item.Values[beginIdx : endIdx+1]
					dates = periodSubset[beginIdx : endIdx+1]
				}

				df := &dataframe.DataFrame{
					Dates:    dates,
					Vals:     vals,
					ColNames: []string{myKey},
				}

				return df, nil
			}
		}
	}

	return nil, ErrRangeDoesNotExist
}

// Set adds the data for the specified security and metric to the cache
func (cache *SecurityMetricCache) Set(security *Security, metric Metric, begin, end time.Time, df *dataframe.DataFrame) error {
	if len(df.Vals) == 0 || len(df.Vals[0]) == 0 {
		return ErrNoData
	}

	// check if the dataframe's dates match those of the date index
	startDate := dateOnly(df.Dates[0])
	startIdx := sort.Search(len(cache.periods), func(i int) bool {
		idxVal := cache.periods[i]
		return (idxVal.After(startDate) || idxVal.Equal(startDate))
	})

	prevIdx := 0
	offset := 0
	for idx, date := range df.Dates {
		myIdx := idx + startIdx - offset
		if !cache.periods[myIdx].Equal(date) {
			df2 := &dataframe.DataFrame{
				Dates:    df.Dates[prevIdx:idx],
				Vals:     [][]float64{df.Vals[0][prevIdx:idx]},
				ColNames: df.ColNames,
			}
			myEnd := df2.Dates[len(df2.Dates)-1]
			err := cache.SetMatched(security, metric, begin, myEnd, df2)
			if err != nil {
				return err
			}
			begin = cache.periods[myIdx]
			prevIdx = idx

			startIdx = sort.Search(len(cache.periods), func(i int) bool {
				idxVal := cache.periods[i]
				return (idxVal.After(date) || idxVal.Equal(date))
			})
			offset = idx
		}
	}

	if prevIdx == 0 {
		return cache.SetMatched(security, metric, begin, end, df)
	}

	df2 := &dataframe.DataFrame{
		Dates:    df.Dates[prevIdx:],
		Vals:     [][]float64{df.Vals[0][prevIdx:]},
		ColNames: df.ColNames,
	}

	return cache.SetMatched(security, metric, begin, end, df2)
}

// SetMatched adds the data for the specified security and metric to the cache assuming df's date frame matches cache.periods exactly
func (cache *SecurityMetricCache) SetMatched(security *Security, metric Metric, begin, end time.Time, df *dataframe.DataFrame) error {
	cache.locker.Lock()
	defer cache.locker.Unlock()

	begin = dateOnly(begin)
	end = dateOnly(end)

	if len(df.Vals) == 0 || len(df.Vals[0]) == 0 {
		return ErrNoData
	}

	toAddBytes := int64(len(df.Vals[0]) * 8)

	if cache.maxSizeBytes < toAddBytes {
		log.Error().Int64("maxSizeBytes", cache.maxSizeBytes).Int64("toAddBytes", toAddBytes).Msg("insufficient space to cache data")
		return ErrDataLargerThanCache
	}

	newTotalSize := toAddBytes + cache.sizeBytes
	if newTotalSize > cache.maxSizeBytes {
		cache.deleteLRU(toAddBytes)
	}

	k := SecurityMetric{
		SecurityObject: *security,
		MetricObject:   metric,
	}.String()

	// create an interval and check that it's valid
	interval := &Interval{
		Begin: begin,
		End:   end,
	}

	// check if the covered period differs from the interval, if it does set it
	coveredInterval := &Interval{
		Begin: dateOnly(df.Dates[0]),
		End:   dateOnly(df.Dates[len(df.Dates)-1]),
	}

	if err := interval.Valid(); err != nil {
		log.Error().Err(err).Time("Begin", interval.Begin).Time("End", interval.End).Msg("cannot set cache value with invalid interval")
		return ErrInvalidTimeRange
	}

	// check if this key already exists
	var items []*CacheItem
	var ok bool

	if items, ok = cache.values[k]; !ok {
		items = []*CacheItem{}
	}

	startIdx := sort.Search(len(cache.periods), func(i int) bool {
		idxVal := cache.periods[i]
		return (idxVal.After(interval.Begin) || idxVal.Equal(interval.Begin))
	})

	cacheItem := &CacheItem{
		Values:        df.Vals[0],
		Period:        interval,
		CoveredPeriod: coveredInterval,
		startIdx:      startIdx,
	}

	items, sizeAdded := cache.insertItem(cacheItem, items)
	if len(items) > 1 {
		items = cache.defrag(items)
	}

	cache.values[k] = items
	cache.lastSeen.Set(k, time.Now())
	cache.sizeBytes += int64(sizeAdded * 8)

	return nil
}

// Set adds the data for the specified security and metric to the cache
func (cache *SecurityMetricCache) SetWithLocalDates(security *Security, metric Metric, begin, end time.Time, df *dataframe.DataFrame) error {
	cache.locker.Lock()
	defer cache.locker.Unlock()

	begin = dateOnly(begin)
	end = dateOnly(end)

	if len(df.Vals) > 0 && len(df.Vals[0]) != len(df.Dates) {
		return ErrDateLengthDoesNotMatch
	}

	toAddBytes := int64(len(df.Vals[0]) * 8)

	if cache.maxSizeBytes < toAddBytes {
		log.Error().Int64("maxSizeBytes", cache.maxSizeBytes).Int64("toAddBytes", toAddBytes).Msg("insufficient space to cache data")
		return ErrDataLargerThanCache
	}

	newTotalSize := toAddBytes + cache.sizeBytes
	if newTotalSize > cache.maxSizeBytes {
		cache.deleteLRU(toAddBytes)
	}

	k := SecurityMetric{
		SecurityObject: *security,
		MetricObject:   metric,
	}.String()

	// create an interval and check that it's valid
	interval := &Interval{
		Begin: begin,
		End:   end,
	}

	var coveredInterval *Interval
	if len(df.Dates) > 0 {
		coveredInterval = &Interval{
			Begin: dateOnly(df.Dates[0]),
			End:   dateOnly(df.Dates[len(df.Dates)-1]),
		}
	} else {
		coveredInterval = &Interval{
			Begin: begin,
			End:   end,
		}
	}

	if err := interval.Valid(); err != nil {
		log.Error().Err(err).Time("Begin", interval.Begin).Time("End", interval.End).Msg("cannot set cache value with invalid interval")
		return ErrInvalidTimeRange
	}

	// check if this key already exists
	var items []*CacheItem
	var ok bool

	if items, ok = cache.values[k]; !ok {
		items = []*CacheItem{}
	}

	startIdx := sort.Search(len(cache.periods), func(i int) bool {
		idxVal := cache.periods[i]
		return (idxVal.After(interval.Begin) || idxVal.Equal(interval.Begin))
	})

	cacheItem := &CacheItem{
		Values:        df.Vals[0],
		Period:        interval,
		CoveredPeriod: coveredInterval,
		isLocalDate:   true,
		localDates:    df.Dates,
		startIdx:      startIdx,
	}

	items, sizeAdded := cache.insertItem(cacheItem, items)
	if len(items) > 1 {
		items = cache.defrag(items)
	}

	cache.values[k] = items
	cache.lastSeen.Set(k, time.Now())
	cache.sizeBytes += int64(sizeAdded * 8)

	return nil
}

// Items returns the items in the cache for a given SecurityMetric. This method is only intended for testing.
func (cache *SecurityMetricCache) Items(security *Security, metric Metric) []*CacheItem {
	k := SecurityMetric{
		SecurityObject: *security,
		MetricObject:   metric,
	}.String()

	if v, ok := cache.values[k]; ok {
		return v
	}

	return []*CacheItem{}
}

// ItemCount returns the number of non-contiguous items in the cache for the given SecurityMetric
func (cache *SecurityMetricCache) ItemCount(security *Security, metric Metric) int {
	k := SecurityMetric{
		SecurityObject: *security,
		MetricObject:   metric,
	}.String()

	if v, ok := cache.values[k]; ok {
		return len(v)
	}

	return 0
}

// Count returns the number of securities + metrics in the cache
func (cache *SecurityMetricCache) Count() int {
	return len(cache.values)
}

// Size returns the number of bytes currently stored in the SecurityMetricCache
func (cache *SecurityMetricCache) Size() int64 {
	return cache.sizeBytes
}

// IsLocalDateIndex returns true if a the CacheItem uses a local date index
func (item *CacheItem) IsLocalDateIndex() bool {
	return item.isLocalDate
}

// LocalDateIndex returns the local date index
func (item *CacheItem) LocalDateIndex() []time.Time {
	return item.localDates
}

// Private Implementation

func (cache *SecurityMetricCache) deleteLRU(bytesToDelete int64) {
	lastAccess := make([]pair, 0, cache.lastSeen.Len())
	cache.lastSeen.ForEach(func(s string, t time.Time) bool {
		lastAccess = append(lastAccess, pair{
			key:  s,
			last: t,
		})
		return true
	})

	sort.Sort(ByDate(lastAccess))

	cleared := int64(0)
	for _, keyPair := range lastAccess {
		entry := cache.values[keyPair.key]
		delete(cache.values, keyPair.key)

		for _, item := range entry {
			cleared += int64(len(item.Values) * 8)
		}

		if cleared > bytesToDelete {
			cache.sizeBytes -= cleared
			break
		}
	}
}

// contiguousByDateIndex checks if two items are contiguous according to the date index
func (cache *SecurityMetricCache) contiguousByDateIndex(a, b *CacheItem) bool {
	// if a is after b swap a and b
	if a.startIdx > b.startIdx {
		c := a
		a = b
		b = c
	}

	dateIdx := cache.periods[a.startIdx:]
	searchVal := a.CoveredPeriod.End
	endIdx := sort.Search(len(dateIdx), func(i int) bool {
		idxVal := dateIdx[i]
		return (idxVal.After(searchVal) || idxVal.Equal(searchVal))
	}) + a.startIdx

	dateIdx = cache.periods[b.startIdx:]
	searchVal = b.CoveredPeriod.Begin
	bStartIdx := sort.Search(len(dateIdx), func(i int) bool {
		idxVal := dateIdx[i]
		return (idxVal.After(searchVal) || idxVal.Equal(searchVal))
	}) + b.startIdx

	return endIdx >= (bStartIdx - 1)
}

// defrag merges contiguous cache items in an array of cache items
func (cache *SecurityMetricCache) defrag(items []*CacheItem) []*CacheItem {
	cnt := len(items)
	newItems := make([]*CacheItem, 0, cnt)
	skip := false
	for idx, item := range items[:cnt-1] {
		if skip {
			skip = false
			continue
		}

		next := items[idx+1]
		if item.Period.Contains(next.Period) {
			newItems = append(newItems, item)
			skip = true
			continue
		}

		if item.Period.Contiguous(next.Period) && item.CoveredPeriod.Contiguous(next.CoveredPeriod) {
			// need to merge
			mergedItem, _ := cache.merge(item, next)
			newItems = append(newItems, mergedItem)
			skip = true
			continue
		}

		newItems = append(newItems, item)
	}

	if !skip {
		newItems = append(newItems, items[cnt-1])
	}

	return newItems
}

// insertItem takes a list of intervals and adds a new interval to the list. If the new interval
// overlaps with an existing interval they are merged, otherwise it is inserted in the
// interval.Begin time sorted location in the list, returns the updated list of cache items and number
// of values that were added
func (cache *SecurityMetricCache) insertItem(new *CacheItem, items []*CacheItem) ([]*CacheItem, int) {
	//log.Debug().Time("A", new.CoveredPeriod.Begin).Time("B", new.CoveredPeriod.End).Msg("covered period for insertItem")
	if len(items) == 0 {
		return []*CacheItem{new}, len(new.Values)
	}

	insertIdx := len(items)
	for idx, item := range items {
		if item.Period.Contains(new.Period) {
			// nothing to be done data already in cache
			log.Debug().Msg("skipping item insert because item is already in []*CacheItem list")
			return items, 0
		}

		if new.Period.Contains(item.Period) {
			log.Debug().Msg("new is a superset of item --- replacing")
			added := len(new.Values) - len(item.Values)
			item.copyFrom(new)
			return items, added
		}

		if (item.Period.Contiguous(new.Period) && item.CoveredPeriod.Contiguous(new.CoveredPeriod)) || cache.contiguousByDateIndex(new, item) {
			log.Debug().Bool("contiguousByDateIndex", cache.contiguousByDateIndex(new, item)).Bool("CoveredContiguous", item.CoveredPeriod.Contiguous(new.CoveredPeriod)).Bool("Combo", (item.Period.Contiguous(new.Period) && item.CoveredPeriod.Contiguous(new.CoveredPeriod))).Object("item.CoveredPeriod", item.CoveredPeriod).Object("new.CoveredPeriod", new.CoveredPeriod).Msg("item and new are contiguous")
			merged, added := cache.merge(new, item)
			item.copyFrom(merged)
			return items, added
		}

		if item.Period.Begin.After(new.Period.Begin) {
			// insert at the index
			insertIdx = idx
		}
	}

	// insert into array
	log.Debug().Msg("insert new into []*CacheItem array")
	if insertIdx >= len(items) {
		items = append(items, new)
	} else {
		items = append(items[:insertIdx+1], items[insertIdx:]...)
		items[insertIdx] = new
	}

	return items, len(new.Values)
}

// merge takes to cache items and merges them into one
// ```mermaid
// graph TD
//
//	A[Merge: a < b] -->|A| B{do Periods overlap?}
//	A -->|B| B
//	B --> E[merge period: min begin, max end]
//	E --> C{isLocalDate?}
//	B -->|No| D[don't merge]
//	C -->|Yes| F{do localDateIdx's overlap?}
//	F -->|No| G[append localDateIdx]
//	G --> H[append values]
//	F -->|Yes| I[find intersection of localDateIdx]
//	I --> J[slice localDateIdx and append]
//	J --> K[slice values and append]
//	C -->|No| M[find intersection of covered period]
//	M --> N[slice covered period and append]
//	N --> O[slice values and append]
//
// ```
func (cache *SecurityMetricCache) merge(a, b *CacheItem) (*CacheItem, int) {
	// combined intervals
	mergedInterval := &Interval{
		Begin: minTime(a.Period.Begin, b.Period.Begin),
		End:   maxTime(a.Period.End, b.Period.End),
	}

	mergedCoveredInterval := &Interval{
		Begin: minTime(a.CoveredPeriod.Begin, b.CoveredPeriod.Begin),
		End:   maxTime(a.CoveredPeriod.End, b.CoveredPeriod.End),
	}

	log.Debug().Time("A", mergedCoveredInterval.Begin).Time("B", mergedCoveredInterval.End).Msg("mergedCoveredInterval")

	// mergedStartIdx is modified in the next step where we check whether item is before or after the CacheItem
	mergedStartIdx := b.startIdx

	added := 0
	// new values occur before current values
	mergedValues := make([]float64, 0, len(a.Values)+len(b.Values))
	isLocalDate := b.isLocalDate
	mergedLocalDates := make([]time.Time, 0, len(a.localDates)+len(b.localDates))

	if a.Period.Begin.Before(b.Period.Begin) {
		// find the start index based on the period begin
		periodStartIdx := sort.Search(len(cache.periods), func(i int) bool {
			idxVal := cache.periods[i]
			return (idxVal.After(a.Period.Begin) || idxVal.Equal(a.Period.Begin))
		})
		mergedStartIdx = periodStartIdx

		if a.isLocalDate {
			// cannot rely upon index in cache.periods. must search through local dates
			// the end date of new should be the start of the current item (hence using b.Period.Begin and not .End)
			searchVal := b.Period.Begin.AddDate(0, 0, -1)
			endIdx := sort.Search(len(a.localDates), func(i int) bool {
				idxVal := a.localDates[i]
				return (idxVal.After(searchVal) || idxVal.Equal(searchVal))
			})
			endIdx += 1
			added += endIdx
			if endIdx > len(a.Values) {
				mergedValues = a.Values[:endIdx-1]
				mergedLocalDates = append(mergedLocalDates, a.localDates[:endIdx-1]...)
			} else {
				mergedValues = a.Values[:endIdx]
				mergedLocalDates = append(mergedLocalDates, a.localDates[:endIdx]...)
			}
		} else {
			coveredStartIdx := sort.Search(len(cache.periods), func(i int) bool {
				idxVal := cache.periods[i]
				return (idxVal.After(a.CoveredPeriod.Begin) || idxVal.Equal(a.CoveredPeriod.Begin))
			})

			// A*    *   AB*  *      B --- DON'T MERGE!
			// A   *  *  AB*  *      B --- DON'T MERGE!
			// A   *    *AB*  *      B
			// A*    *   AB   *  *   B --- DON'T MERGE!
			// A   *  *  AB   *  *   B --- DON'T MERGE!
			// A   *    *AB   *  *   B --- DON'T MERGE!
			// A*    *   AB      *  *B --- DON'T MERGE!
			// A   *  *  AB      *  *B --- DON'T MERGE!
			// A   *    *AB      *  *B --- DON'T MERGE!

			// the end date of new should be the start of the current item (hence using b.Period.Begin and not .End)
			searchVal := b.CoveredPeriod.Begin.AddDate(0, 0, -1)
			coveredEndIdx := sort.Search(len(cache.periods), func(i int) bool {
				idxVal := cache.periods[i]
				return (idxVal.After(searchVal) || idxVal.Equal(searchVal))
			})

			numDates := coveredEndIdx - coveredStartIdx + 1
			if len(a.Values) < numDates {
				numDates = len(a.Values)
			}
			added += numDates
			mergedValues = a.Values[:numDates]
		}
	}

	mergedValues = append(mergedValues, b.Values...)
	if b.isLocalDate {
		mergedLocalDates = append(mergedLocalDates, b.localDates...)
	}

	// new values after b.Values
	if a.Period.End.After(b.Period.End) {
		periodStartIdx := sort.Search(len(cache.periods), func(i int) bool {
			idxVal := cache.periods[i]
			return (idxVal.After(a.Period.Begin) || idxVal.Equal(a.Period.Begin))
		})

		if a.isLocalDate {
			searchVal := b.Period.End.AddDate(0, 0, 1)
			startIdx := sort.Search(len(a.localDates), func(i int) bool {
				idxVal := a.localDates[i]
				return (idxVal.After(searchVal) || idxVal.Equal(searchVal))
			})

			added += len(a.Values[startIdx:])
			mergedValues = append(mergedValues, a.Values[startIdx:]...)
			mergedLocalDates = append(mergedLocalDates, a.localDates[startIdx:]...)
		} else {
			searchVal := b.CoveredPeriod.End.AddDate(0, 0, 1)
			sliceStartIdx := sort.Search(len(cache.periods), func(i int) bool {
				idxVal := cache.periods[i]
				return (idxVal.After(searchVal) || idxVal.Equal(searchVal))
			})

			startIdx := sliceStartIdx - periodStartIdx
			added += len(a.Values[startIdx:])
			mergedValues = append(mergedValues, a.Values[startIdx:]...)
		}
	}

	mergedCacheItem := &CacheItem{
		Period:        mergedInterval,
		CoveredPeriod: mergedCoveredInterval,
		Values:        mergedValues,
		startIdx:      mergedStartIdx,
		isLocalDate:   isLocalDate,
		localDates:    mergedLocalDates,
	}

	return mergedCacheItem, added
}

func (item *CacheItem) copyFrom(new *CacheItem) {
	item.Period = new.Period
	item.Values = new.Values
	item.CoveredPeriod = new.CoveredPeriod
	item.isLocalDate = new.isLocalDate
	item.localDates = new.localDates
	item.startIdx = new.startIdx
}

func (item *CacheItem) MarshalZerologObject(e *zerolog.Event) {
	e.Object("Period", item.Period)
	e.Object("CoveredPeriod", item.CoveredPeriod)
	e.Int("Length", len(item.Values))
	//e.Floats64("Values", item.Values)
}

func minTime(a, b time.Time) time.Time {
	if a.After(b) {
		return b
	}

	return a
}

func maxTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}
