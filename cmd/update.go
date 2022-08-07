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
	"encoding/hex"
	"fmt"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/penny-vault/pv-api/backtest"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/messenger"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var updateCmdPortfolioID string
var updateCmdCalculateToDate string
var updateCmdFromWorkQueue bool
var testUpdateCMD bool

func init() {
	updateCmd.Flags().StringVar(&updateCmdPortfolioID, "portfolioID", "", "Portfolio to update specified as {userID}:{portfolioID}")
	updateCmd.Flags().StringVar(&updateCmdCalculateToDate, "date", "", "Date specified as YYYY-MM-dd to compute measurements through")
	updateCmd.Flags().BoolVar(&updateCmdFromWorkQueue, "work-queue", false, "Check for portfolio simulation requests from work queue")
	updateCmd.Flags().BoolVarP(&testUpdateCMD, "test", "t", false, "Run update in test mode that does not save results to DB.")

	rootCmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the metrics associated with portfolio's for a given date",
	Run: func(cmd *cobra.Command, args []string) {
		common.SetupLogging()
		common.SetupCache()
		log.Info().Msg("initialized logging")
		nyc := common.GetTimezone()

		// setup database
		err := database.Connect()
		if err != nil {
			log.Fatal().Err(err).Msg("database connection failed")
		}

		// get time
		var dt time.Time
		if updateCmdCalculateToDate == "" {
			dt = time.Now()
		} else {
			dt, err = time.Parse("2006-01-02", updateCmdCalculateToDate)
			if err != nil {
				log.Fatal().Err(err).Str("InputStr", updateCmdCalculateToDate).Msg("could not parse to date - expected format 2006-01-02")
			}

			// convert to EST
			dt = time.Date(dt.Year(), dt.Month(), dt.Day(), 18, 0, 0, 0, nyc)
		}

		// initialize message passing interface
		if err := messenger.Initialize(); err != nil {
			log.Info().Err(err).Msg("could not initialize message passing interface")
		}

		// Initialize data framework
		data.InitializeDataManager()
		log.Info().Msg("initialized data framework")

		// initialize strategies
		strategies.InitializeStrategyMap()

		dataManager := data.NewManager()

		strategies.LoadStrategyMetricsFromDb()
		for _, strat := range strategies.StrategyList {
			if _, ok := strategies.StrategyMetricsMap[strat.Shortcode]; !ok && !testUpdateCMD {
				if err := createStrategyPortfolio(strat, dt, dataManager); err != nil {
					log.Panic().Err(err).Msg("could not create portfolio")
				}
			}
		}

		users := getUsers()
		users = append(users, "pvuser")
		portfolios := getPortfolios(updateCmdPortfolioID, users)
		log.Info().Int("NumPortfolios", len(portfolios)).Time("Date", dt).Msg("updating portfolios")

		for _, pm := range portfolios {
			subLog := log.With().Str("PortfolioName", pm.Portfolio.Name).Str("PortfolioID", hex.EncodeToString(pm.Portfolio.ID)).Time("StartDate", pm.Portfolio.StartDate).Time("EndDate", pm.Portfolio.EndDate).Logger()
			subLog.Info().Msg("updating portfolio")

			if err := pm.SetStatus(fmt.Sprintf("calculating... [%s]", time.Now().In(nyc).Format("RFC822"))); err != nil {
				log.Error().Err(err).Msg("could not set portfolio status")
			}

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

			lastTime := perf.Measurements[len(perf.Measurements)-1].Time
			pm.Portfolio.EndDate = lastTime

			if !testUpdateCMD {
				subLog.Debug().Msg("saving portfolio to DB")
				err = pm.Save(pm.Portfolio.UserID)
				if err != nil {
					subLog.Error().Stack().Err(err).Msg("error while saving portfolio updates")
					continue
				}
				lastMeas := perf.Measurements[len(perf.Measurements)-1]
				log.Info().Object("PortfolioMetrics", perf.PortfolioMetrics).Time("PerformanceStart", perf.Measurements[0].Time).Time("PerformanceEnd", lastMeas.Time).Msg("Saving portfolio performance")
				err = perf.Save(pm.Portfolio.UserID)
				if err != nil {
					subLog.Error().Stack().Err(err).Msg("error while saving portfolio measurements")
				}

				if err := pm.SetStatus(fmt.Sprintf("updated on %s", time.Now().In(nyc).Format("RFC822"))); err != nil {
					log.Error().Err(err).Msg("could not set portfolio status")
				}
				if err := pm.AddActivity(time.Now().In(nyc), "updated portfolio", []string{"update"}); err != nil {
					log.Error().Err(err).Msg("could not set portfolio activity")
				}
			} else {
				// since we are testing print results out
				perf.LogSummary()
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
