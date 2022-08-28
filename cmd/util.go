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

package cmd

import (
	"context"
	"strings"

	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/rs/zerolog/log"
)

// getPortoflios retrieves a list of portfolios from the database
//
//	dataManager - interface to the database
//	portfolioID - specified as {userID}:{portfolioID} only pull requested portfolio
//	userList    - list of users to include portfolios for
func getPortfolios(ctx context.Context, portfolioID string, userList []string) []*portfolio.Model {
	dataManager := data.NewManager()

	// get a list of portfolio id's to update
	portfolios := make([]*portfolio.Model, 0, 100)
	if portfolioID != "" {
		portfolioParts := strings.Split(updateCmdPortfolioID, ":")
		if len(portfolioParts) != 2 {
			log.Fatal().Str("InputStr", updateCmdPortfolioID).Int("LenPortfolioParts", len(portfolioParts)).Msg("must specify portfolioID as {userID}:{portfolioID}")
		}
		u := portfolioParts[0]
		pIDStr := portfolioParts[1]
		ids := []string{
			pIDStr,
		}
		log.Info().Str("PortfolioID", updateCmdPortfolioID).Msg("load portfolio from DB")
		p, err := portfolio.LoadFromDB(ctx, ids, u, dataManager)
		if err != nil {
			log.Fatal().Err(err).Msg("could not load portfolio from DB")
		}
		portfolios = append(portfolios, p[0])
	} else {
		for _, u := range userList {
			trx, err := database.TrxForUser(ctx, u)
			if err != nil {
				log.Panic().Err(err).Str("User", u).Msg("could not create transaction for user")
			}

			rows, err := trx.Query(ctx, "SELECT id FROM portfolios WHERE temporary='f'")
			if err != nil {
				if err := trx.Rollback(ctx); err != nil {
					log.Error().Stack().Err(err).Msg("could not rollback transaction")
				}
				log.Panic().Err(err).Msg("could not get portfolio IDs")
			}

			for rows.Next() {
				var pIDStr string
				err := rows.Scan(&pIDStr)
				if err != nil {
					if err := trx.Rollback(ctx); err != nil {
						log.Error().Stack().Err(err).Msg("could not rollback transaction")
					}
					log.Warn().Stack().Err(err).Str("User", u).Msg("get portfolio ids failed")
					continue
				}

				ids := []string{pIDStr}
				log.Debug().Str("PortfolioID", pIDStr).Msg("load portfolio from DB")
				p, err := portfolio.LoadFromDB(ctx, ids, u, dataManager)
				if err != nil {
					if err := trx.Rollback(ctx); err != nil {
						log.Error().Stack().Err(err).Msg("could not rollback transaction")
					}
					log.Panic().Err(err).Strs("IDs", ids).Msg("could not load portfolio from DB")
				}
				portfolios = append(portfolios, p[0])
			}

			if err := trx.Commit(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not commit transaction")
			}
		}
	}
	return portfolios
}

func getUsers(ctx context.Context) []string {
	// load portfolio ids from database
	users, err := database.GetUsers(ctx)
	if err != nil {
		log.Panic().Err(err).Msg("could not load users from database")
	}
	return users
}
