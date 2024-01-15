// Copyright 2021-2023
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
	"math"
	"sort"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
)

type Manager struct {
	metricCache *SecurityMetricCache
	lruCache    *lru.Cache[string, []byte]
	pvdb        *PvDb
	locker      sync.RWMutex
	tradingDays []time.Time
}

var (
	managerOnce     sync.Once
	managerInstance *Manager
)

func GetManagerInstance() *Manager {
	managerOnce.Do(func() {
		log.Debug().Msg("initializing manager instance for the first time")
		tradecron.LoadMarketHolidays()
		err := LoadSecuritiesFromDB()
		if err != nil {
			log.Error().Err(err).Msg("could not load securities database")
		}

		pvdb := NewPvDb()

		cacheMaxSize := viper.GetInt64("cache.metric_bytes")
		if cacheMaxSize == 0 {
			cacheMaxSize = 10485760 // 10 MB
		}

		cacheSize := viper.GetInt("cache.lru_size")
		if cacheSize <= 0 {
			cacheSize = 32
		}
		lruCache, err := lru.New[string, []byte](cacheSize)
		if err != nil {
			log.Error().Err(err).Msg("could not create lru cache")
		}

		managerInstance = &Manager{
			metricCache: NewSecurityMetricCache(cacheMaxSize, []time.Time{}),
			lruCache:    lruCache,
			pvdb:        pvdb,
			locker:      sync.RWMutex{},
		}

		managerInstance.getTradingDays()
	})
	return managerInstance
}

// GetMetrics returns metrics for the requested securities over the specified time range
func (manager *Manager) GetMetrics(securities []*Security, metrics []Metric, begin, end time.Time) (dataframe.Map[time.Time], error) {
	ctx := context.Background()
	subLog := log.With().Time("Begin", begin).Time("End", end).Logger()

	normalizedMetrics := normalizeMetrics(metrics)
	normalizedSecurities, err := normalizeSecurities(securities)
	if err != nil {
		log.Error().Err(err).Msg("normalizing securities failed")
		return nil, err
	}

	// if the end time is before begin then error
	if end.Before(begin) {
		subLog.Error().Err(ErrBeginAfterEnd).Msg("manager.GetMetrics called with an invalid time period (begin > end)")
		return nil, ErrBeginAfterEnd
	}

	// check what needs to be pulled
	toPullSecuritiesMap := make(map[*Security]bool, len(normalizedSecurities))
	for _, security := range normalizedSecurities {
		for _, metric := range normalizedMetrics {
			contains, _ := manager.metricCache.Check(security, metric, begin, end)
			if !contains {
				toPullSecuritiesMap[security] = true
			}
		}
	}

	toPullSecurities := make([]*Security, 0, len(toPullSecuritiesMap))
	for k := range toPullSecuritiesMap {
		toPullSecurities = append(toPullSecurities, k)
	}

	// adjust request date range to cover the minimum request duration
	duration := end.Sub(begin)
	modifiedEnd := end
	minDur := viper.GetDuration("database.min_request_duration")
	if minDur == 0 {
		minDur = time.Hour * 24 * 366
	}
	if duration < minDur {
		modifiedEnd = begin.Add(minDur)
	}

	dates := manager.TradingDaysAtFrequency(dataframe.Daily, begin, modifiedEnd)

	if len(dates) == 0 {
		return dataframe.Map[time.Time]{}, nil
	}

	myBegin := dates[0]
	myEnd := dates[len(dates)-1]

	// pull required data not currently in cache
	if len(toPullSecurities) > 0 {
		if result, err := manager.pvdb.GetEOD(ctx, toPullSecurities, normalizedMetrics, myBegin, myEnd); err == nil {
			for k, df := range result {
				securityMetric := NewSecurityMetricFromString(k)
				security := securityMetric.SecurityObject
				metric := securityMetric.MetricObject
				subLog := log.With().Str("Security", security.Ticker).Str("Metric", string(metric)).Logger()
				if isSparseMetric(metric) {
					switch metric {
					case MetricSplitFactor:
						df = df.Drop(1)
					case MetricDividendCash:
						df = df.Drop(0)
					default:
						subLog.Error().Str("metric", string(metric)).Msg("metric is listed as sparse but there is no decimator configured")
					}

					err = manager.metricCache.SetWithLocalDates(&security, metric, begin, modifiedEnd, df)
					if err != nil {
						subLog.Error().Err(err).Msg("couldn't set local dates in cache")
						return nil, err
					}
				} else {
					err = manager.metricCache.Set(&security, metric, begin, modifiedEnd, df)
					if err != nil {
						subLog.Error().Err(err).Msg("couldn't set cache")
						return nil, err
					}
				}
			}
		} else {
			subLog.Error().Err(err).Msg("could not fetch data")
			return nil, err
		}
	}

	// get specific time period
	dfMap := make(dataframe.Map[time.Time])
	for _, security := range normalizedSecurities {
		for _, metric := range metrics {
			df := manager.metricCache.GetPartial(security, metric, begin, end)
			k := SecurityMetric{
				SecurityObject: *security,
				MetricObject:   metric,
			}
			dfMap[k.String()] = df
		}
	}

	return dfMap, nil
}

func (manager *Manager) cacheFetchOnOrBefore(security *Security, metric Metric, date time.Time) (float64, time.Time, bool) {
	if contains, intervals := manager.metricCache.Check(security, metric, date, date); contains {
		if vals, err := manager.metricCache.Get(security, metric, date, date); err == nil {
			if vals.Len() > 0 {
				// the date is part of the covered period
				return vals.Vals[0][0], date, true
			}

			// the date is not part of the covered period
			// get the last interval
			lastInterval := intervals[len(intervals)-1]
			if vals2, err := manager.metricCache.Get(security, metric, lastInterval.Begin, date); err == nil {
				last := vals2.Last()
				if len(last.Vals) < 1 || len(last.Vals[0]) < 1 {
					return math.NaN(), time.Time{}, false
				}
				return last.Vals[0][0], last.Index[0], true
			}
			log.Error().Err(err).Object("LastInterval", lastInterval).Msg("error while fetching value from cache in GetMetricOnOrBefore in second query")
		} else {
			log.Error().Err(err).Msg("error while fetching value from cache in GetMetricOnOrBefore")
		}
	}
	return math.NaN(), time.Time{}, false
}

// GetMetricsOnOrBefore finds a single metric value on the requested date or the closest date available prior to the requested date
func (manager *Manager) GetMetricOnOrBefore(security *Security, metric Metric, date time.Time) (float64, time.Time, error) {
	ctx := context.Background()
	subLog := log.With().Time("Date", date).Str("SecurityFigi", security.CompositeFigi).Str("SecurityTicker", security.Ticker).Str("Metric", string(metric)).Logger()

	date = dateOnly(date)

	var val float64
	var forDate time.Time
	var ok bool

	// check if the date is currently in the cache
	if val, forDate, ok = manager.cacheFetchOnOrBefore(security, metric, date); ok && !math.IsNaN(val) {
		return val, forDate, nil
	}

	// first try and pull the data into the cache
	adjustedDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	_, err := manager.GetMetrics([]*Security{security}, []Metric{metric}, adjustedDate, adjustedDate)
	if err != nil {
		subLog.Error().Err(err).Msg("could not fetch metrics from DB")
	} else {
		// check if the date is now in the cache
		if val, forDate, ok = manager.cacheFetchOnOrBefore(security, metric, date); ok && !math.IsNaN(val) {
			return val, forDate, nil
		}
	}

	// still not in the cache ... load from DB
	val, forDate, err = manager.pvdb.GetEODOnOrBefore(ctx, security, metric, date)
	if err != nil {
		subLog.Error().Err(err).Msg("could not fetch data")
		return 0.0, time.Time{}, err
	}
	return val, forDate, nil
}

// GetPortfolio returns a cached portfolio
func (manager *Manager) GetLRU(key string) []byte {
	val, ok := manager.lruCache.Get(key)
	if !ok {
		return []byte{}
	}
	return val
}

// PreloadMetrics pre-emptively loads metrics into the data cache
func (manager *Manager) PreloadMetrics(ctx context.Context, plan PortfolioPlan) {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "preloadMetrics")
	defer span.End()

	log.Debug().Msg("pre-populate data cache")

	begin := plan.StartDate()
	end := plan.EndDate()

	tickerSet := make(map[Security]bool)
	for _, alloc := range plan {
		for security := range alloc.Members {
			tickerSet[security] = true
		}
	}

	tickerList := make([]*Security, len(tickerSet))
	ii := 0
	for k := range tickerSet {
		tickerList[ii] = &Security{
			CompositeFigi: k.CompositeFigi,
			Ticker:        k.Ticker,
		}
		ii++
	}

	log.Debug().Time("Begin", begin).Time("End", end).Int("NumAssets", len(tickerList)).Msg("querying database for eod metrics")
	if _, err := manager.GetMetrics(tickerList, []Metric{MetricAdjustedClose, MetricClose, MetricDividendCash, MetricSplitFactor}, begin, end); err != nil {
		log.Error().Stack().Err(err).Msg("preload metrics errored")
	}
}

// Reset restores the data manager to its initial state; clearing all cache - this is mostly used in testing
func (manager *Manager) Reset() {
	cacheMaxSize := viper.GetInt64("cache.local_bytes")
	if cacheMaxSize == 0 {
		cacheMaxSize = 10485760 // 10 MB
	}

	periods := manager.metricCache.periods
	manager.metricCache = NewSecurityMetricCache(cacheMaxSize, periods)

	lruCacheSize := viper.GetInt("cache.lru_size")
	if lruCacheSize < 1 {
		lruCacheSize = 64
	}
	lruCache, err := lru.New[string, []byte](lruCacheSize)
	if err != nil {
		log.Error().Err(err).Int("lruCacheSize", lruCacheSize).Msg("could not create lru cache")
	}
	manager.lruCache = lruCache
}

// SetLRU saves the portfolio in the online cache
func (manager *Manager) SetLRU(key string, val []byte) {
	manager.lruCache.Add(key, val)
}

// TradingDays returns a dataframe over the specified date period, all values in the dataframe are 0
func (manager *Manager) TradingDays(begin, end time.Time) *dataframe.DataFrame[time.Time] {
	df := &dataframe.DataFrame[time.Time]{
		Index:    manager.tradingDays,
		Vals:     [][]float64{make([]float64, len(manager.tradingDays))},
		ColNames: []string{"zeros"},
	}
	return df.Trim(begin, end)
}

// Private methods

func (manager *Manager) getTradingDays() {
	ctx := context.Background()
	begin := time.Date(1980, 1, 1, 0, 0, 0, 0, common.GetTimezone())
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, common.GetTimezone())

	tradingDays, err := manager.pvdb.TradingDays(ctx, begin, end)
	if err != nil {
		log.Panic().Err(err).Msg("could not load trading days")
	}

	manager.locker.Lock()
	manager.tradingDays = tradingDays
	manager.metricCache.periods = tradingDays
	manager.locker.Unlock()

	refreshTimer := time.NewTimer(24 * time.Hour)
	go func() {
		<-refreshTimer.C
		log.Info().Msg("refreshing trading days")
		manager.getTradingDays()
	}()
}

func isSparseMetric(metric Metric) bool {
	switch metric {
	case MetricSplitFactor:
		return true
	case MetricDividendCash:
		return true
	default:
		return false
	}
}

func normalizeMetrics(metrics []Metric) []Metric {
	metricMap := make(map[Metric]bool, len(metrics))
	for _, metric := range metrics {
		metricMap[metric] = true
	}

	// if metric is open, high, low, close, or adjusted close also pre-fetch splits
	// and dividends
	_, hasOpen := metricMap[MetricOpen]
	_, hasHigh := metricMap[MetricHigh]
	_, hasLow := metricMap[MetricLow]
	_, hasClose := metricMap[MetricClose]
	_, hasAdjustedClose := metricMap[MetricAdjustedClose]

	if hasOpen || hasHigh || hasLow || hasClose || hasAdjustedClose {
		metrics = append(metrics, MetricSplitFactor)
		metrics = append(metrics, MetricDividendCash)
	}

	return metrics
}

func normalizeSecurities(securities []*Security) ([]*Security, error) {
	for idx, security := range securities {
		if security.Ticker == "$CASH" {
			continue
		}
		res, err := SecurityFromFigi(security.CompositeFigi)
		if err != nil {
			res, err = SecurityFromTicker(security.Ticker)
			if err != nil {
				log.Error().Str("Ticker", security.Ticker).Str("CompositeFigi", security.CompositeFigi).Msg("security could not be found by ticker or composite figi")
				return nil, err
			}
		}
		securities[idx] = res
	}

	return securities, nil
}

func (manager *Manager) TradingDaysAtFrequency(frequency dataframe.Frequency, begin, end time.Time) []time.Time {
	// set end to the end of the day
	end = end.AddDate(0, 0, 1).Add(time.Nanosecond * -1)

	beginIdx := sort.Search(len(manager.tradingDays), func(i int) bool {
		idxVal := manager.tradingDays[i]
		return (idxVal.After(begin) || idxVal.Equal(begin))
	})

	endIdx := sort.Search(len(manager.tradingDays), func(i int) bool {
		idxVal := manager.tradingDays[i]
		return (idxVal.After(end) || idxVal.Equal(end))
	})

	if len(manager.tradingDays) == 0 {
		log.Fatal().Msg("manager trading days not initialized")
	}

	if endIdx < len(manager.tradingDays) {
		endIdx++
	}

	myDays := manager.tradingDays[beginIdx:endIdx]

	// if no dates match return an empty set
	if len(myDays) == 0 {
		return myDays
	}

	// check if we got 1 too many
	if myDays[len(myDays)-1].After(end) {
		myDays = myDays[:len(myDays)-1]
	}

	days := myDays

	if frequency != dataframe.Daily {
		days = FilterDays(frequency, myDays)
	}

	return days
}
