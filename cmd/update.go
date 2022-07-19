// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/penny-vault/pv-api/backtest"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/penny-vault/pv-api/strategies/strategy"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var PortfolioID string
var ToDate string

func init() {
	updateCmd.Flags().StringVar(&PortfolioID, "portfolioID", "", "Portfolio to update specified as {userID}:{portfolioID}")
	updateCmd.Flags().StringVar(&ToDate, "date", "", "Date specified as YYYY-MM-dd to compute measurements through")
	rootCmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the metrics associated with portfolio's for a given date",
	Run: func(cmd *cobra.Command, args []string) {
		common.SetupLogging()
		common.SetupCache()
		log.Info().Msg("initialized logging")

		// setup database
		err := database.Connect()
		if err != nil {
			log.Fatal().Err(err).Msg("database connection failed")
		}

		// get time
		var dt time.Time
		if ToDate == "" {
			dt = time.Now()
		} else {
			dt, err = time.Parse("2006-01-02", ToDate)
			if err != nil {
				log.Fatal().Err(err).Str("InputStr", ToDate).Msg("could not parse to date - expected format 2006-01-02")
			}

			// convert to EST
			nyc, _ := time.LoadLocation("America/New_York")
			dt = time.Date(dt.Year(), dt.Month(), dt.Day(), 18, 0, 0, 0, nyc)
		}

		// Initialize data framework
		data.InitializeDataManager()
		log.Info().Msg("initialized data framework")

		// initialize strategies
		strategies.InitializeStrategyMap()

		credentials := make(map[string]string)
		credentials["tiingo"] = viper.GetString("tiingo.system_token")
		dataManager := data.NewManager(credentials)

		strategies.LoadStrategyMetricsFromDb()
		for _, strat := range strategies.StrategyList {
			if _, ok := strategies.StrategyMetricsMap[strat.Shortcode]; !ok {
				if err := createStrategyPortfolio(strat, dt, &dataManager); err != nil {
					log.Panic().Err(err).Msg("could not create portfolio")
				}
			}
		}

		portfolios := getPortfolios(&dataManager)
		log.Info().Int("NumPortfolios", len(portfolios)).Time("Date", dt).Msg("updating portfolios")

		for _, pm := range portfolios {
			subLog := log.With().Str("PortfolioName", pm.Portfolio.Name).Str("PortfolioID", hex.EncodeToString(pm.Portfolio.ID)).Time("StartDate", pm.Portfolio.StartDate).Time("EndDate", pm.Portfolio.EndDate).Logger()
			subLog.Info().Msg("updating portfolio")

			subLog.Debug().Msg("loading transactions from DB")
			err = pm.LoadTransactionsFromDB()
			if err != nil {
				// NOTE: error is logged by caller
				continue
			}

			subLog.Debug().Time("Date", dt).Msg("update transactions")
			err = pm.UpdateTransactions(context.Background(), dt)
			if err != nil {
				subLog.Error().Msg("skipping portfolio due to error")
				continue
			}

			// Try and load from the DB
			subLog.Debug().Msg("load portfolio performance")
			var perf *portfolio.Performance
			portfolioID, _ := uuid.FromBytes(pm.Portfolio.ID)
			perf, err = portfolio.LoadPerformanceFromDB(portfolioID, pm.Portfolio.UserID)
			if err != nil {
				subLog.Warn().Stack().Err(err).Msg("could not load portfolio performance -- may be due to the portfolio's performance never being calculated")
				// just create a new performance record
				perf = portfolio.NewPerformance(pm.Portfolio)
			} else {
				if err := perf.LoadMeasurementsFromDB(pm.Portfolio.UserID); err != nil {
					log.Error().Stack().Err(err).Msg("could not load measurements from database")
					continue
				}
			}

			subLog.Debug().Time("Date", dt).Msg("calculate performance through")
			err = perf.CalculateThrough(context.Background(), pm, dt)
			if err != nil {
				subLog.Error().Stack().Err(err).Msg("error while calculating portfolio performance -- refusing to save")
				continue
			}

			subLog.Debug().Msg("saving portfolio to DB")
			err = pm.Save(pm.Portfolio.UserID)
			if err != nil {
				subLog.Error().Stack().Err(err).Msg("error while saving portfolio updates")
				continue
			}
			log.Info().Object("PortfolioMetrics", perf.PortfolioMetrics).Msg("Saving portfolio performance")
			err = perf.Save(pm.Portfolio.UserID)
			if err != nil {
				subLog.Error().Stack().Err(err).Msg("error while saving portfolio measurements")
			}

			subLog.Debug().Msg("finished updating portfolio")
		}
	},
}

func createStrategyPortfolio(strat *strategy.Info, endDate time.Time, manager *data.Manager) error {
	tz := common.GetTimezone()
	subLog := log.With().Str("Shortcode", strat.Shortcode).Time("EndDate", endDate).Str("StrategyName", strat.Name).Logger()
	subLog.Info().Msg("create portfolio")

	// build arguments
	argumentsMap := make(map[string]interface{})
	for k, v := range strat.Arguments {
		var output interface{}
		if v.Typecode == "string" || v.Typecode == "choice" {
			output = v.Default
		} else {
			if err := json.Unmarshal([]byte(v.Default), &output); err != nil {
				log.Warn().Stack().Err(err).Str("JsonValue", v.Default).Msg("could not unmarshal value")
			}
		}
		argumentsMap[k] = output
	}
	arguments, err := json.Marshal(argumentsMap)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("unable to build arguments for metrics calculation")
		return err
	}

	params := make(map[string]json.RawMessage)
	if err := json.Unmarshal(arguments, &params); err != nil {
		log.Error().Stack().Err(err).Msg("could not unmarshal strategy arguments")
		return err
	}

	b, err := backtest.New(context.Background(), strat.Shortcode, params, strat.Benchmark, time.Date(1980, 1, 1, 0, 0, 0, 0, tz), endDate, manager)
	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("unable to build arguments for metrics calculation")
		return err
	}

	b.Save("pvuser", true)
	return nil
}

func getPortfolios(dataManager *data.Manager) []*portfolio.Model {
	// get a list of portfolio id's to update
	portfolios := make([]*portfolio.Model, 0, 100)
	if PortfolioID != "" {
		portfolioParts := strings.Split(PortfolioID, ":")
		if len(portfolioParts) != 2 {
			log.Fatal().Str("InputStr", PortfolioID).Int("LenPortfolioParts", len(portfolioParts)).Msg("must specify portfolioID as {userID}:{portfolioID}")
		}
		u := portfolioParts[0]
		pIDStr := portfolioParts[1]
		ids := []string{
			pIDStr,
		}
		log.Info().Str("PortfolioID", PortfolioID).Msg("load portfolio from DB")
		p, err := portfolio.LoadFromDB(ids, u, dataManager)
		if err != nil {
			log.Fatal().Err(err).Msg("could not load portfolio from DB")
		}
		portfolios = append(portfolios, p[0])
	} else {
		// load portfolio ids from database
		users, err := database.GetUsers()
		users = append(users, "pvuser")
		if err != nil {
			log.Panic().Err(err).Msg("could not load users from database")
		}

		for _, u := range users {
			trx, err := database.TrxForUser(u)
			if err != nil {
				log.Panic().Err(err).Str("User", u).Msg("could not create transaction for user")
			}

			rows, err := trx.Query(context.Background(), "SELECT id FROM portfolios WHERE temporary='f'")
			if err != nil {
				if err := trx.Rollback(context.Background()); err != nil {
					log.Error().Stack().Err(err).Msg("could not rollback transaction")
				}
				log.Panic().Err(err).Msg("could not get portfolio IDs")
			}

			for rows.Next() {
				var pIDStr string
				err := rows.Scan(&pIDStr)
				if err != nil {
					if err := trx.Rollback(context.Background()); err != nil {
						log.Error().Stack().Err(err).Msg("could not rollback transaction")
					}
					log.Warn().Stack().Err(err).Str("User", u).Msg("get portfolio ids failed")
					continue
				}

				ids := []string{pIDStr}
				log.Debug().Str("PortfolioID", pIDStr).Msg("load portfolio from DB")
				p, err := portfolio.LoadFromDB(ids, u, dataManager)
				if err != nil {
					if err := trx.Rollback(context.Background()); err != nil {
						log.Error().Stack().Err(err).Msg("could not rollback transaction")
					}
					log.Panic().Err(err).Strs("IDs", ids).Msg("could not load portfolio from DB")
				}
				portfolios = append(portfolios, p[0])
			}

			if err := trx.Commit(context.Background()); err != nil {
				log.Error().Stack().Err(err).Msg("could not commit transaction")
			}
		}
	}
	return portfolios
}
