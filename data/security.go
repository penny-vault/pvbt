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

	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"
)

// Security represents a tradeable asset
type Security struct {
	Ticker        string `json:"ticker"`
	CompositeFigi string `json:"compositeFigi"`
}

// SecurityFromFigi loads a security from database using the Composite FIGI as the lookup key
func SecurityFromFigi(figi string) (*Security, error) {
	ctx := context.Background()

	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		log.Error().Stack().Err(err).Msg("could not get transaction when querying trading days")
		return nil, err
	}

	if err := trx.Commit(ctx); err != nil {
		log.Warn().Stack().Err(err).Msg("could not commit transaction")
	}
	return nil, nil
}

// SecurityFromTicker loads a security from database using the ticker as the lookup key
func SecurityFromTicker(ticker string) (*Security, error) {
	ctx := context.Background()

	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		log.Error().Stack().Err(err).Msg("could not get transaction when querying trading days")
		return nil, err
	}

	if err := trx.Commit(ctx); err != nil {
		log.Warn().Stack().Err(err).Msg("could not commit transaction")
	}
	return nil, nil
}

// SecurityFromTickerList loads securities from database using the ticker as the lookup key
func SecurityFromTickerList(tickers []string) ([]*Security, error) {
	ctx := context.Background()

	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		log.Error().Stack().Err(err).Msg("could not get transaction when querying trading days")
		return nil, err
	}

	if err := trx.Commit(ctx); err != nil {
		log.Warn().Stack().Err(err).Msg("could not commit transaction")
	}
	return nil, nil
}
