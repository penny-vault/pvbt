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
	"time"

	"github.com/google/uuid"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var notifyUser string
var notifyDate string
var notifyTest bool

func init() {
	if err := viper.BindEnv("database.max_temp_portfolio_age_secs", "MAX_PORTFOLIO_AGE_SECS"); err != nil {
		log.Panic().Err(err).Msg("could not bind database.max_temp_portfolio_age_secs")
	}
	notifyCmd.Flags().IntP("max_temp_portfolio_age_secs", "s", 86400, "Maximum temporary portfolio age in seconds")
	if err := viper.BindPFlag("database.max_temp_portfolio_age_secs", notifyCmd.Flags().Lookup("max_temp_portfolio_age_secs")); err != nil {
		log.Panic().Err(err).Msg("could not bind database.max_temp_portfolio_age_secs")
	}

	notifyCmd.Flags().StringVar(&notifyUser, "user", "", "only purge portfolios from specified user")
	notifyCmd.Flags().StringVarP(&notifyDate, "date", "d", "", "date to run notifications for 2006-01-02")
	notifyCmd.Flags().BoolVarP(&notifyTest, "test", "t", false, "test the notifier and don't send notifications")

	rootCmd.AddCommand(notifyCmd)
}

var notifyCmd = &cobra.Command{
	Use:   "notify",
	Short: "send notification emails for saved portfolios",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		// setup database
		err := database.Connect(ctx)
		if err != nil {
			log.Panic().Err(err).Msg("could not connect to database")
		}

		strategies.InitializeStrategyMap()

		// calculate notification date
		var forDate time.Time
		if notifyDate != "" {
			forDate, err = time.Parse("2006-01-02", notifyDate)
			if err != nil {
				log.Panic().Err(err).Str("DateStr", notifyDate).Msg("could not parse DateStr with format 2006-01-02")
			}
		} else {
			// NOTE: default date is yesterday because portfolio performance isn't updated until
			// all EOD prices are available which is between 4 - 5am EST
			forDate = time.Now().AddDate(0, 0, -1)

		}

		log.Info().Time("NotifyDate", forDate).Msg("processing notifications for date")

		var users []string
		if notifyUser != "" {
			users = []string{notifyUser}
		} else {
			users = getUsers(ctx)
		}
		portfolios := getPortfolios(ctx, "", users)

		for _, pm := range portfolios {
			var perf *portfolio.Performance
			portfolioID, _ := uuid.FromBytes(pm.Portfolio.ID)
			perf, err = portfolio.LoadPerformanceFromDB(ctx, portfolioID, pm.Portfolio.UserID)
			subLog := log.With().Str("UserID", pm.Portfolio.UserID).Str("PortfolioName", pm.Portfolio.Name).Str("PortfolioID", hex.EncodeToString(pm.Portfolio.ID)).Logger()
			if err != nil {
				subLog.Error().Err(err).Msg("could not load portfolio performance; skipping...")
				continue
			}

			notifications := pm.NotificationsForDate(ctx, forDate, perf)
			subLog.Debug().Int("NumNotifications", len(notifications)).Msg("portfolio has notifications")
			for _, notification := range notifications {
				contact, err := common.GetAuth0User(pm.Portfolio.UserID)
				if err != nil {
					subLog.Error().Err(err).Msg("could not get user contact info")
					continue
				}
				if err := notification.SendEmail(contact.Name, contact.Email); err != nil {
					subLog.Error().Err(err).Msg("could not send notification")
				}
			}
		}
	},
}
