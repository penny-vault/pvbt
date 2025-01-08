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

package cmd

import (
	"context"
	"time"

	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var purgeUser string

func init() {
	if err := viper.BindEnv("database.max_temp_portfolio_age_secs", "MAX_PORTFOLIO_AGE_SECS"); err != nil {
		log.Panic().Err(err).Msg("could not bind database.max_temp_portfolio_age_secs")
	}
	purgeCmd.Flags().IntP("max_temp_portfolio_age_secs", "s", 86400, "Maximum temporary portfolio age in seconds")
	if err := viper.BindPFlag("database.max_temp_portfolio_age_secs", purgeCmd.Flags().Lookup("max_temp_portfolio_age_secs")); err != nil {
		log.Panic().Err(err).Msg("could not bind database.max_temp_portfolio_age_secs")
	}

	purgeCmd.Flags().StringVar(&purgeUser, "user", "", "Only purge portfolios from specified user")

	rootCmd.AddCommand(purgeCmd)
}

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Delete temporary portfolios older than max_temp_portfolio_age_secs",
	Run: func(_ *cobra.Command, _ []string) {
		ctx := context.Background()
		// setup database
		err := database.Connect(ctx)
		if err != nil {
			log.Panic().Err(err).Msg("could not connect to database")
		}

		userList := make([]string, 0)

		// build query
		if purgeUser != "" {
			userList = append(userList, purgeUser)
		} else {
			// get a list of users from the database
			users, _ := database.GetUsers(ctx)
			userList = append(userList, users...)
		}

		maxAgeDuration := viper.GetDuration("database.max_temp_portfolio_age_secs") * -1 * time.Second
		maxAge := time.Now().Add(maxAgeDuration)

		for _, u := range userList {
			subLog := log.With().Str("User", u).Logger()
			trx, err := database.TrxForUser(ctx, u)
			if err != nil {
				subLog.Error().Stack().Err(err).Msg("could not get database transaction")
			}

			var cnt int64
			err = trx.QueryRow(ctx, "SELECT count(*) FROM portfolios WHERE temporary=true AND created < $1", maxAge).Scan(&cnt)
			if err != nil {
				subLog.Error().Stack().Err(err).Msg("could not get expired portfolio count")
				if err := trx.Rollback(ctx); err != nil {
					log.Error().Stack().Err(err).Msg("could not rollback transaction")
				}

				continue
			}

			subLog.Info().Int64("NumExpiredPortfolios", cnt).Time("MaxAge", maxAge).Msg("number of expired portfolios")

			_, err = trx.Exec(ctx, "DELETE FROM portfolios WHERE temporary=true AND created < $1", maxAge)
			if err != nil {
				subLog.Error().Stack().Err(err).Msg("could not delete portfolios")
				if err := trx.Rollback(ctx); err != nil {
					log.Error().Stack().Err(err).Msg("could not rollback transaction")
				}

				continue
			}

			err = trx.Commit(ctx)
			if err != nil {
				subLog.Error().Stack().Err(err).Msg("could not delete portfolios")
			}
		}
	},
}
