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
	"sync"
	"time"

	"github.com/penny-vault/pv-api/common"
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

	tradingDays, err := manager.pvdb.TradingDays(ctx, begin, end, FrequencyDaily)
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

func (manager *Manager) getData(securities []*Security, metrics []Metric, begin, end time.Time) (map[string]float64, error) {
	return nil, nil
}
