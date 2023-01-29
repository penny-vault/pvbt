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

package cmd

import (
	"context"

	"github.com/google/uuid"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(metricCmd)
}

var metricCmd = &cobra.Command{
	Use:   "metric <user:portfolio id> {allDrawDowns} ",
	Short: "calculate a metric for the portfolio (mostly useful for debugging)",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		common.SetupLogging()
		// setup database
		err := database.Connect(ctx)
		if err != nil {
			log.Fatal().Err(err).Msg("database connection failed")
		}
		// setup manaager and other odds and ends
		data.GetManagerInstance()

		myPortfolio := getPortfolioFromID(ctx, args[0])
		portfolioID, err := uuid.FromBytes(myPortfolio.Portfolio.ID)
		if err != nil {
			log.Error().Err(err).Msg("could not convert portfolio ID to UUID")
		}

		subLog := log.With().Str("ID", portfolioID.String()).Logger()
		if err != nil {
			subLog.Error().Err(err).Msg("could not load portfolio from DB")
		}

		subLog.Debug().Msg("Loading performance from DB...")
		perf, err := portfolio.LoadPerformanceFromDB(ctx, portfolioID, myPortfolio.Portfolio.UserID)
		if err != nil {
			subLog.Error().Err(err).Msg("could not load portfolio from DB")
		}

		err = perf.LoadMeasurementsFromDB(ctx, myPortfolio.Portfolio.UserID)
		if err != nil {
			subLog.Error().Err(err).Msg("could not load performance measurements from DB")
		}

		subLog.Debug().Msg("measurements loaded...")

		switch args[1] {
		case "allDrawDowns":
			drawDowns := perf.AllDrawDowns(999999, portfolio.STRATEGY)
			log.Info().Int("NumDrawDowns", len(drawDowns)).Send()
			for _, drawDown := range drawDowns {
				log.Info().Object("drawDown", drawDown).Send()
			}
		}

	},
}
