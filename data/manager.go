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

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type Manager struct {
	cache       *SecurityMetricCache
	pvdb        *PvDb
	locker      sync.RWMutex
	tradingDays []time.Time
}

var (
	managerOnce     sync.Once
	managerInstance *Manager
)

func (manager *Manager) GetMetrics(securities []*Security, metrics []Metric, begin, end time.Time) (*dataframe.DataFrame, error) {
	ctx := context.Background()
	subLog := log.With().Time("Begin", begin).Time("End", end).Logger()

	normalizedMetrics := normalizeMetrics(metrics)

	// check what needs to be pulled
	toPullSecuritiesMap := make(map[*Security]bool, len(securities))
	for _, security := range securities {
		for _, metric := range normalizedMetrics {
			contains, _ := manager.cache.Check(security, metric, begin, end)
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

	dates := manager.tradingDaysAtFrequency(FrequencyDaily, begin, modifiedEnd)

	// pull required data not currently in cache
	if result, err := manager.pvdb.GetEOD(ctx, toPullSecuritiesArray, normalizedMetrics, dates[0], dates[len(dates)-1]); err == nil {
		for k, v := range result {
			manager.cache.Set(&k.SecurityObject, k.MetricObject, begin, modifiedEnd, v)
		}
	} else {
		subLog.Error().Err(err).Msg("could not fetch data")
		return nil, err
	}

	// get specific time period
	data := make(map[SecurityMetric][]float64)
	for _, security := range securities {
		for _, metric := range normalizedMetrics {
			if vals, err := manager.cache.Get(security, metric, begin, end); err == nil {
				data[SecurityMetric{
					SecurityObject: *security,
					MetricObject:   metric,
				}] = vals
			} else {
				subLog.Error().Err(err).Msg("could not fetch data")
				return nil, err
			}
		}
	}

	df := securityMetricMapToDataFrame(data, dates)
	return df, nil
}

func getManagerInstance() *Manager {
	managerOnce.Do(func() {
		err := LoadSecuritiesFromDB()
		if err != nil {
			log.Error().Err(err).Msg("could not load securities database")
		}

		pvdb := NewPvDb()

		managerInstance = &Manager{
			cache:  NewSecurityMetricCache(viper.GetInt64("data.cacheBytes"), []time.Time{}),
			pvdb:   pvdb,
			locker: sync.RWMutex{},
		}

		managerInstance.getTradingDays()
	})
	return managerInstance
}

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
	manager.cache.periods = tradingDays
	manager.locker.Unlock()

	refreshTimer := time.NewTimer(24 * time.Hour)
	go func() {
		<-refreshTimer.C
		log.Info().Msg("refreshing trading days")
		manager.getTradingDays()
	}()
}

func (manager *Manager) tradingDaysAtFrequency(frequency Frequency, begin, end time.Time) []time.Time {
	beginIdx := sort.Search(len(manager.tradingDays), func(i int) bool {
		idxVal := manager.tradingDays[i]
		return (idxVal.After(begin) || idxVal.Equal(begin))
	})

	endIdx := sort.Search(len(manager.tradingDays), func(i int) bool {
		idxVal := manager.tradingDays[i]
		return (idxVal.After(begin) || idxVal.Equal(begin))
	})

	days := FilterDays(frequency, manager.tradingDays[beginIdx:endIdx])
	return days
}

func normalizeMetrics(metrics []Metric) []Metric {
	metricMap := make(map[Metric]int, len(metrics))

	// if metric is open, high, low, close, or adjusted close also pre-fetch splits
	// and dividends
	_, hasOpen := metricMap[MetricOpen]
	_, hasHigh := metricMap[MetricHigh]
	_, hasLow := metricMap[MetricLow]
	_, hasClose := metricMap[MetricClose]
	_, hasAdjustedClose := metricMap[MetricAdjustedClose]

	if hasOpen || hasHigh || hasLow || hasClose || hasAdjustedClose {
		metricMap[MetricSplitFactor] = 3
		metricMap[MetricDividendCash] = 3
	}

	normalizedMetrics := make([]Metric, 0, len(metricMap))
	for k := range metricMap {
		normalizedMetrics = append(normalizedMetrics, k)
	}

	return normalizedMetrics
}
