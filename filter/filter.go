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

package filter

import (
	"fmt"
	"time"

	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/rs/zerolog/log"
)

type Filterable interface {
	GetMeasurements(field1 string, field2 string, since time.Time) ([]byte, error)
	GetHoldings(frequency dataframe.Frequency, since time.Time) ([]byte, error)
	GetTransactions(since time.Time) ([]byte, error)
}

func getPortfolio(portfolioID string) *portfolio.Portfolio {
	manager := data.GetManagerInstance()
	raw := manager.GetLRU(portfolioID)
	subLog := log.With().Str("PortfolioID", portfolioID).Logger()
	if len(raw) > 0 {
		port := portfolio.Portfolio{}
		_, err := port.Unmarshal(raw)
		if err != nil {
			subLog.Error().Stack().Err(err).Msg("failed to deserialize portfolio")
			return nil
		}
		return &port
	}
	return nil
}

func getPerformance(portfolioID string) *portfolio.Performance {
	subLog := log.With().Str("PortfolioID", portfolioID).Logger()
	manager := data.GetManagerInstance()
	raw := manager.GetLRU(fmt.Sprintf("%s:performance", portfolioID))
	if len(raw) > 0 {
		perf := portfolio.Performance{}
		_, err := perf.Unmarshal(raw)
		if err != nil {
			subLog.Error().Stack().Err(err).Msg("failed to deserialize portfolio")
			return nil
		}
		return &perf
	}
	return nil
}

func New(portfolioID string, userID string) Filterable {
	var perf *portfolio.Performance

	port := getPortfolio(portfolioID)
	if port != nil {
		perf = getPerformance(portfolioID)
		return &InMemory{
			Performance: perf,
			Portfolio:   port,
		}
	}

	db := Database{
		PortfolioID: portfolioID,
		UserID:      userID,
	}

	return &db
}

func getPeriodReturnFieldForFrequency(frequency dataframe.Frequency) string {
	switch frequency {
	case dataframe.Annually:
		return database.TWRRYtd
	case dataframe.Monthly:
		return database.TWRRMtd
	case dataframe.Weekly:
		return database.TWRRWtd
	case dataframe.Daily:
		return database.TWRROneDay
	default:
		return database.TWRRMtd
	}
}
