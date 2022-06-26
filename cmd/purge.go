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
	"time"

	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var User string

func init() {
	viper.BindEnv("database.max_temp_portfolio_age_secs", "MAX_PORTFOLIO_AGE_SECS")
	purgeCmd.Flags().IntP("max_temp_portfolio_age_secs", "s", 86400, "Maximum temporary portfolio age in seconds")
	viper.BindPFlag("database.max_temp_portfolio_age_secs", purgeCmd.Flags().Lookup("max_temp_portfolio_age_secs"))

	purgeCmd.Flags().StringVar(&User, "user", "", "Only purge portfolios")

	rootCmd.AddCommand(purgeCmd)
}

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Delete temporary portfolios older than max_temp_portfolio_age_secs",
	Run: func(cmd *cobra.Command, args []string) {
		// setup database
		err := database.Connect()
		if err != nil {
			log.Panic().Err(err).Msg("could not connect to database")
		}

		userList := make([]string, 0)

		// build query
		if User != "" {
			userList = append(userList, User)
		} else {
			// get a list of users from the database
			users, _ := database.GetUsers()
			userList = append(userList, users...)
		}

		maxAgeDuration := viper.GetDuration("database.max_temp_portfolio_age_secs") * -1 * time.Second
		maxAge := time.Now().Add(maxAgeDuration)

		for _, u := range userList {
			subLog := log.With().Str("User", u).Logger()
			trx, err := database.TrxForUser(u)
			if err != nil {
				subLog.Error().Err(err).Msg("could not get database transaction")
			}

			var cnt int64
			err = trx.QueryRow(context.Background(), "SELECT count(*) FROM portfolios WHERE temporary=true AND created < $1", maxAge).Scan(&cnt)
			if err != nil {
				subLog.Error().Err(err).Msg("could not get expired portfolio count")
				trx.Rollback(context.Background())
				continue
			}

			subLog.Info().Int64("NumExpiredPortfolios", cnt).Time("MaxAge", maxAge).Msg("number of expired portfolios")

			_, err = trx.Exec(context.Background(), "DELETE FROM portfolios WHERE temporary=true AND created < $1", maxAge)
			if err != nil {
				subLog.Error().Err(err).Msg("could not delete portfolios")
				trx.Rollback(context.Background())
				continue
			}

			err = trx.Commit(context.Background())
			if err != nil {
				subLog.Error().Err(err).Msg("could not delete portfolios")
			}
		}
	},
}
