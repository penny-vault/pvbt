// Copyright 2021-2025
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
	"fmt"
	"strings"
	"sync"

	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"
)

var (
	securitiesByFigi   map[string]*Security
	securitiesByTicker map[string]*Security
	writeLocker        sync.RWMutex
)

func LoadSecuritiesFromDB() error {
	ctx := context.Background()

	securitiesByFigi = make(map[string]*Security)
	securitiesByTicker = make(map[string]*Security)

	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		log.Error().Stack().Err(err).Msg("could not get transaction when creating securities list")
		return err
	}

	rows, err := trx.Query(ctx, "SELECT ticker, composite_figi, active FROM assets ORDER BY active")
	if err != nil {
		log.Error().Err(err).Msg("could not query assets from database")
		return err
	}

	writeLocker.Lock()
	defer writeLocker.Unlock()

	for rows.Next() {
		var ticker string
		var compositeFigi string
		var active bool
		err := rows.Scan(&ticker, &compositeFigi, &active)
		if err != nil {
			log.Error().Err(err).Msg("could not scan database results")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return err
		}
		s := &Security{
			CompositeFigi: compositeFigi,
			Ticker:        ticker,
		}

		securitiesByFigi[compositeFigi] = s
		if active {
			securitiesByTicker[ticker] = s
		}
	}

	if err := trx.Commit(ctx); err != nil {
		log.Warn().Stack().Err(err).Msg("could not commit transaction")
	}

	return nil
}

// MustSecurityFromFigi loads a security from database using the Composite FIGI as the lookup key
func MustSecurityFromFigi(figi string) *Security {
	security, err := SecurityFromFigi(figi)
	if err != nil {
		log.Panic().Err(err).Str("figi", figi).Msg("security not found for specified figi")
	}
	return security
}

// SecurityFromFigi loads a security from database using the Composite FIGI as the lookup key
func SecurityFromFigi(figi string) (*Security, error) {
	if managerInstance == nil {
		GetManagerInstance()
	}

	writeLocker.RLock()
	defer writeLocker.RUnlock()

	if s, ok := securitiesByFigi[figi]; ok {
		return s, nil
	}
	return nil, ErrSecurityNotFound
}

// SecurityFromTicker loads a security from database using the ticker as the lookup key
func SecurityFromTicker(ticker string) (*Security, error) {
	if managerInstance == nil {
		GetManagerInstance()
	}

	writeLocker.RLock()
	defer writeLocker.RUnlock()

	if s, ok := securitiesByTicker[ticker]; ok {
		return s, nil
	}
	return nil, ErrSecurityNotFound
}

// SecurityFromTickerList loads securities from database using the ticker as the lookup key
func SecurityFromTickerList(tickers []string) ([]*Security, error) {
	writeLocker.RLock()
	defer writeLocker.RUnlock()

	securities := make([]*Security, 0, len(tickers))
	for _, ticker := range tickers {
		if s, ok := securitiesByTicker[ticker]; ok {
			securities = append(securities, s)
		} else {
			return nil, ErrSecurityNotFound
		}
	}
	return securities, nil
}

// SecurityMetric functions

// String returns a string representation of the security metric
func (sm SecurityMetric) String() string {
	return fmt.Sprintf("%s:%s", sm.SecurityObject.CompositeFigi, sm.MetricObject)
}

func NewSecurityMetricFromString(s string) SecurityMetric {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return SecurityMetric{}
	}

	security, err := SecurityFromFigi(parts[0])
	if err != nil {
		return SecurityMetric{}
	}

	m := Metric(parts[1])
	return SecurityMetric{
		SecurityObject: *security,
		MetricObject:   m,
	}
}
