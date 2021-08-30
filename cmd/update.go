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
	"main/data"
	"main/database"
	"main/portfolio"
	"main/strategies"
	"strings"
	"time"

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
		}

		// Initialize data framework
		data.InitializeDataManager()
		log.Info("Initialized data framework")

		// initialize strategies
		strategies.InitializeStrategyMap()

		credentials := make(map[string]string)
		credentials["tiingo"] = viper.GetString("tiingo.system_token")
		dataManager := data.NewManager(credentials)

		// get a list of portfolio id's to update
		portfolios := make([]*portfolio.PortfolioModel, 0, 100)
		if PortfolioID == "" {
			portfolioParts := strings.Split(PortfolioID, ":")
			if len(portfolioParts) != 2 {
				log.Fatal("Must specify portfolioID as {userID}:{portfolioID}")
			}
			u := portfolioParts[0]
			pIDStr := portfolioParts[1]
			pID := uuid.MustParse(pIDStr)
			p, err := portfolio.LoadFromDB(pID, u, &dataManager)
			if err != nil {
				log.Fatal(err)
			}
			portfolios = append(portfolios, p)
		} else {
			// load portfolio ids from database
			users, err := database.GetUsers()
			if err != nil {
				log.Fatal(err)
			}

			for _, u := range users {
				trx, err := database.TrxForUser(u)
				if err != nil {
					log.Fatal(err)
				}

				rows, err := trx.Query(context.Background(), "SELECT id FROM portfolio_v1")
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

					pID := uuid.MustParse(pIDStr)

					p, err := portfolio.LoadFromDB(pID, u, &dataManager)
					if err != nil {
						log.Fatal(err)
					}

					portfolios = append(portfolios, p)
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

			pm.UpdateTransactions(dt)
			perf, err := pm.CalculatePerformance(dt)
			if err != nil {
				log.WithFields(log.Fields{
					"Error": err,
				}).Error("error while calculating portfolio performance -- refusing to save")
				continue
			}
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
