// Copyright 2021 JD Fergason
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
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/penny-vault/pv-api/strategies/strategy"
	"github.com/penny-vault/pv-api/tradecron"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
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
		// setup database
		err := database.Connect()
		if err != nil {
			log.Fatal(err)
		}

		// get time
		var dt time.Time
		if ToDate == "" {
			dt = time.Now()
		} else {
			dt, err = time.Parse("2006-01-02", ToDate)
			if err != nil {
				log.Fatal(err)
			}

			// convert to EST
			nyc, _ := time.LoadLocation("America/New_York")
			dt = time.Date(dt.Year(), dt.Month(), dt.Day(), 18, 0, 0, 0, nyc)
		}

		tradecron.InitializeTradeCron()

		// Initialize data framework
		data.InitializeDataManager()
		log.Info("Initialized data framework")

		// initialize strategies
		strategies.InitializeStrategyMap()

		credentials := make(map[string]string)
		credentials["tiingo"] = viper.GetString("tiingo.system_token")
		dataManager := data.NewManager(credentials)

		strategies.LoadStrategyMetricsFromDb()
		/*
			for _, strat := range strategies.StrategyList {
				if _, ok := strategies.StrategyMetricsMap[strat.Shortcode]; !ok {
					log.WithFields(log.Fields{
						"Strategy": strat.Shortcode,
					}).Info("create portfolio for strategy")
					createStrategyPortfolio(strat, &dataManager)
				}
			}
		*/
		// get a list of portfolio id's to update
		portfolios := make([]*portfolio.PortfolioModel, 0, 100)
		if PortfolioID != "" {
			portfolioParts := strings.Split(PortfolioID, ":")
			if len(portfolioParts) != 2 {
				log.Fatal("Must specify portfolioID as {userID}:{portfolioID}")
			}
			u := portfolioParts[0]
			pIDStr := portfolioParts[1]
			ids := []string{
				pIDStr,
			}
			p, err := portfolio.LoadFromDB(ids, u, &dataManager)
			if err != nil {
				log.Fatal(err)
			}
			p[0].LoadTransactionsFromDB()
			portfolios = append(portfolios, p[0])
		} else {
			// load portfolio ids from database
			users, err := database.GetUsers()
			users = append(users, "pvuser")
			if err != nil {
				log.Fatal(err)
			}

			for _, u := range users {
				trx, err := database.TrxForUser(u)
				if err != nil {
					log.Fatal(err)
				}

				rows, err := trx.Query(context.Background(), "SELECT id FROM portfolios")
				if err != nil {
					log.Fatal(err)
				}

				for rows.Next() {
					var pIDStr string
					err := rows.Scan(&pIDStr)
					if err != nil {
						log.WithFields(log.Fields{
							"Error": err,
							"User":  u,
						}).Warn("get portfolio ids failed")
						continue
					}

					ids := []string{
						pIDStr,
					}
					p, err := portfolio.LoadFromDB(ids, u, &dataManager)
					if err != nil {
						log.Fatal(err)
					}
					p[0].LoadTransactionsFromDB()
					portfolios = append(portfolios, p[0])
				}
			}
		}

		log.WithFields(log.Fields{
			"NumPortfolios": len(portfolios),
			"Date":          dt,
		}).Info("updating portfolios")

		for _, pm := range portfolios {
			log.WithFields(log.Fields{
				"PortfolioID": hex.EncodeToString(pm.Portfolio.ID),
				"StartDate":   pm.Portfolio.StartDate.Format("2006-01-02"),
				"EndDate":     pm.Portfolio.EndDate.Format("2006-01-02"),
			}).Info("updating portfolio")

			pm.LoadTransactionsFromDB()
			err = pm.UpdateTransactions(dt)
			if err != nil {
				// NOTE: error is logged by caller
				continue
			}

			// Try and load from the DB
			var perf *portfolio.Performance
			portfolioID, _ := uuid.FromBytes(pm.Portfolio.ID)
			perf, err = portfolio.LoadPerformanceFromDB(portfolioID, pm.Portfolio.UserID)
			if err != nil {
				log.WithFields(log.Fields{
					"Error":       err,
					"PortfolioID": hex.EncodeToString(pm.Portfolio.ID),
				}).Warn("could not load portfolio performance -- may be due to the portfolio's performance never being calculated")

				// just create a new performance record
				perf = portfolio.NewPerformance(pm.Portfolio)
			} else {
				perf.LoadMeasurementsFromDB(pm.Portfolio.UserID)
			}

			err = perf.CalculateThrough(pm, dt)
			if err != nil {
				log.WithFields(log.Fields{
					"Error": err,
				}).Error("error while calculating portfolio performance -- refusing to save")
				continue
			}

			/*
				fmt.Printf("Performance from %s through %s\n", perf.PeriodStart, perf.PeriodEnd)
				for idx, m := range perf.Measurements {
					fmt.Printf("%d) %s\t%.2f\n", idx, m.Time, m.Value)
				}
			*/

			err = pm.Save(pm.Portfolio.UserID)
			if err != nil {
				log.WithFields(log.Fields{
					"Error": err,
				}).Error("error while saving portfolio updates")
				continue
			}
			err = perf.Save(pm.Portfolio.UserID)
			if err != nil {
				log.WithFields(log.Fields{
					"Error": err,
				}).Error("error while saving portfolio measurements")
			}
		}
	},
}

func createStrategyPortfolio(strat *strategy.StrategyInfo, manager *data.Manager) error {
	tz, _ := time.LoadLocation("America/New_York") // New York is the reference time

	// build arguments
	argumentsMap := make(map[string]interface{})
	for k, v := range strat.Arguments {
		var output interface{}
		if v.Typecode == "string" || v.Typecode == "choice" {
			output = v.Default
		} else {
			json.Unmarshal([]byte(v.Default), &output)
		}
		argumentsMap[k] = output
	}
	arguments, err := json.Marshal(argumentsMap)
	if err != nil {
		log.WithFields(log.Fields{
			"Shortcode":    strat.Shortcode,
			"StrategyName": strat.Name,
			"Error":        err,
		}).Warn("Unable to build arguments for metrics calculation")
		return err
	}

	params := make(map[string]json.RawMessage)
	json.Unmarshal(arguments, &params)

	b, err := backtest.New(strat.Shortcode, params, time.Date(1980, 1, 1, 0, 0, 0, 0, tz), time.Now(), manager)
	if err != nil {
		log.WithFields(log.Fields{
			"Shortcode":    strat.Shortcode,
			"StrategyName": strat.Name,
			"Error":        err,
		}).Warn("Unable to build arguments for metrics calculation")
		return err
	}

	b.Save("pvuser", true)
	return nil
}
