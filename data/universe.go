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
	"time"

	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"
)

// Universes define a dynamic set of equities that a strategy may consider for investment
// Example universes include USTradeableEquities, SP500, etc.
// To use a universe simply set the desired universe on a data request, e.g.:
// NewDataRequest().Universe(NewUSTradeableEquities()).On(...)
type Universe interface {
	Securities(context.Context, time.Time) ([]*Security, error)
}

// USTradeableEquities is a list of securities that meet the following criteria:
// 1. Is traded on the NYSE or NASDAQ exchanges
// 2. Has average daily volume > 1,000,000 USD
type USTradeableEquities struct {
	limit int
}

// NEWUSTradeableEquities returns a new instance of USTradeableEquities with no limit
// on the size of the universe.
func NewUSTradeableEquities() *USTradeableEquities {
	return &USTradeableEquities{
		limit: 3000,
	}
}

// Limit sets a maximum size of the universe (when sorted by market cap)
func (usEquities *USTradeableEquities) Limit(myLimit int) *USTradeableEquities {
	usEquities.limit = myLimit
	return usEquities
}

// Securities returns a list of securities in the Universe at `forDate`
func (usEquities *USTradeableEquities) Securities(ctx context.Context, forDate time.Time) ([]*Security, error) {
	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		log.Error().Err(err).Msg("could not get transaction for pvuser")
	}
	defer trx.Commit(ctx)

	// First filter to the top equities by marketcap that are traded as 'Common Stock' (as defined by their 10-Q filing)
	// the default setting of including the top 3000 securities (roughly like the russell 3000) means that approximately
	// 98% of the tradeable universe is included
	rows, err := trx.Query(ctx, "SELECT ticker, composite_figi, close, market_cap FROM eod WHERE event_date='$1' AND asset_type='Common Stock' AND market_cap != 'NaN'::float8 ORDER BY market_cap DESC LIMIT $2", forDate, usEquities.limit)
	if err != nil {
		log.Error().Stack().Err(err).Msg("could not query for US Equities Tradeable Universe")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return nil, err
	}

	// next filter out securities where the close price is less than $5. This is to prevent high transaction costs
	// and is done as a second step to prevent lower capitalization stocks from seeping into the universe.
	securities := make([]*Security, 0, 3000)
	var marketCap float64
	var closePrice float64
	for rows.Next() {
		sec := Security{}
		if err := rows.Scan(&sec.Ticker, &sec.CompositeFigi, &closePrice, &marketCap); err != nil {
			log.Error().Err(err).Msg("could not scan query results for USTradeableEquities")
			return nil, err
		}
		if closePrice > 5 {
			securities = append(securities, &sec)
		}
	}

	return securities, nil
}
