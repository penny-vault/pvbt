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
	"context"
	"sort"
	"sync"
	"time"

	"github.com/coocood/freecache"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type Manager struct {
	metricCache *SecurityMetricCache
	lruCache    *freecache.Cache
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

		managerInstance = &Manager{
			metricCache: NewSecurityMetricCache(cacheMaxSize, []time.Time{}),
			lruCache:    freecache.NewCache(viper.GetInt("cache.lru_bytes")),
			pvdb:        pvdb,
			locker:      sync.RWMutex{},
		}

		managerInstance.getTradingDays()
	})
	return managerInstance
}

// GetMetrics returns metrics for the requested securities over the specified time range
func (manager *Manager) GetMetrics(securities []*Security, metrics []Metric, begin, end time.Time) (dataframe.DataFrameMap, error) {
	ctx := context.Background()
	subLog := log.With().Time("Begin", begin).Time("End", end).Logger()

	normalizedMetrics := normalizeMetrics(metrics)
	normalizedSecurities, err := normalizeSecurities(securities)
	if err != nil {
		log.Error().Err(err).Msg("normalizing securities failed")
		return nil, err
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

	toPullSecuritiesArray := make([]*Security, 0, len(toPullSecuritiesMap))
	for k := range toPullSecuritiesMap {
		toPullSecuritiesArray = append(toPullSecuritiesArray, k)
	}

	// adjust request date range
	duration := end.Sub(begin)
	modifiedEnd := end
	if duration < viper.GetDuration("database.min_request_duration") {
		modifiedEnd = begin.Add(viper.GetDuration("database.min_request_duration"))
	}

	dates := manager.tradingDaysAtFrequency(dataframe.Daily, begin, modifiedEnd)

	// pull required data not currently in cache
	if len(toPullSecuritiesArray) > 0 {
		if result, err := manager.pvdb.GetEOD(ctx, toPullSecuritiesArray, normalizedMetrics, dates[0],
			dates[len(dates)-1]); err == nil {
			for k, df := range result {
				securityMetric := NewSecurityMetricFromString(k)
				security := securityMetric.SecurityObject
				metric := securityMetric.MetricObject
				subLog := log.With().Str("Security", security.Ticker).Str("Metric", string(metric)).Logger()
				if isSparseMetric(metric) {
					if metric == MetricSplitFactor {
						df = df.Drop(1)
					} else {
						df = df.Drop(0)
					}
					err = manager.metricCache.SetWithLocalDates(&security, metric, begin, modifiedEnd, df)
					if err != nil {
						subLog.Error().Err(err).Msg("couldn't set local dates in cache")
						return nil, err
					}
				} else {
					actualBegin := df.Dates[0]
					actualEnd := df.Last().Dates[0]
					err = manager.metricCache.Set(&security, metric, actualBegin, actualEnd, df)
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
	dfMap := make(dataframe.DataFrameMap)
	for _, security := range normalizedSecurities {
		for _, metric := range metrics {
			df, err := manager.metricCache.Get(security, metric, begin, end)
			if err != nil {
				log.Error().Err(err).Time("Begin", begin).Time("End", end).Str("Figi", security.CompositeFigi).
					Str("Ticker", security.Ticker).Str("Metric", string(metric)).Msg("fetching from cache failed")
				return nil, err
			}
			k := SecurityMetric{
				SecurityObject: *security,
				MetricObject:   metric,
			}
			dfMap[k.String()] = df
		}
	}

	return dfMap, nil
}

// GetMetricsOnOrBefore finds a single metric value on the requested date or the closest date available prior to the requested date
func (manager *Manager) GetMetricOnOrBefore(security *Security, metric Metric, date time.Time) (float64, time.Time, error) {
	ctx := context.Background()
	subLog := log.With().Time("Date", date).Str("SecurityFigi", security.CompositeFigi).Str("SecurityTicker", security.Ticker).Str("Metric", string(metric)).Logger()

	// check if the date is currently in the cache
	if vals, err := manager.metricCache.Get(security, metric, date, date); err != nil {
		return vals.Vals[0][0], date, nil
	}

	// not currently in the cache ... load from DB
	val, forDate, err := manager.pvdb.GetEODOnOrBefore(ctx, security, metric, date)
	if err != nil {
		subLog.Error().Err(err).Msg("could not fetch data")
		return 0.0, time.Time{}, err
	}
	return val, forDate, err
}

// GetPortfolio returns a cached portfolio
func (manager *Manager) GetLRU(key string) []byte {
	val, err := manager.lruCache.Get([]byte(key))
	if err != nil {
		return []byte{}
	}
	return val
}

// Reset restores the manager connection to its initial state - this is mostly used in testing
func (manager *Manager) Reset() {
	cacheMaxSize := viper.GetInt64("cache.local_bytes")
	if cacheMaxSize == 0 {
		cacheMaxSize = 10485760 // 10 MB
	}

	periods := manager.metricCache.periods
	manager.metricCache = NewSecurityMetricCache(cacheMaxSize, periods)
}

// SetLRU saves the portfolio in the online cache
func (manager *Manager) SetLRU(key string, val []byte) {
	err := manager.lruCache.Set([]byte(key), val, viper.GetInt("cache.ttl"))
	if err != nil {
		log.Error().Err(err).Str("key", key).Msg("could not set cache value")
	}
}

// TradingDays returns a dataframe over the specified date period, all values in the dataframe are 0
func (manager *Manager) TradingDays(begin, end time.Time) *dataframe.DataFrame {
	df := &dataframe.DataFrame{
		Dates:    manager.tradingDays,
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

func (manager *Manager) tradingDaysAtFrequency(frequency dataframe.Frequency, begin, end time.Time) []time.Time {
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

	days := FilterDays(frequency, manager.tradingDays[beginIdx:endIdx+1])
	return days
}