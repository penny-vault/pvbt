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
	"fmt"
	"time"

	"github.com/goccy/go-json"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
)

var backtestStartTime string
var backtestEndTime string

func init() {
	backtestCmd.LocalFlags().StringVarP(&backtestStartTime, "start", "s", "1980-01-01", "start time for back test of form yyyy-mm-dd")
	backtestCmd.LocalFlags().StringVarP(&backtestEndTime, "end", "e", "1980-01-01", "end time for back test of form yyyy-mm-dd")
	rootCmd.AddCommand(backtestCmd)
}

var backtestCmd = &cobra.Command{
	Use:        "backtest [flags] StrategyShortcode StrategyArguments",
	Short:      "Run a backtest of a strategy",
	Args:       cobra.MinimumNArgs(2),
	ArgAliases: []string{"StrategyShortcode", "StrategyArguments"},
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		// setup database
		err := database.Connect(ctx)
		if err != nil {
			log.Panic().Err(err).Msg("could not connect to database")
		}

		// parse start and end time
		startTime, err := time.Parse("2006-01-02", backtestStartTime)
		if err != nil {
			log.Fatal().Err(err).Str("input", backtestStartTime).Msg("invalid format for start time")
		}

		endTime, err := time.Parse("2006-01-02", backtestEndTime)
		if err != nil {
			log.Fatal().Err(err).Str("input", backtestEndTime).Msg("invalid format for end time")
		}

		// initialize strategies
		strategies.InitializeStrategyMap()

		strategies.LoadStrategyMetricsFromDb()
		strat := strategies.StrategyMap[args[0]]

		var arguments map[string]json.RawMessage
		err = json.Unmarshal([]byte(args[1]), &arguments)
		if err != nil {
			log.Error().Err(err).Msg("Could not unmarshal json")
			return
		}

		strategy, err := strat.Factory(arguments)
		if err != nil {
			log.Error().Err(err).Msg("Could not create strategy")
			return
		}

		target, predicted, err := strategy.Compute(ctx, startTime, endTime)
		if err != nil {
			log.Fatal().Err(err).Msg("Could not compute strategy positions")
		}

		if len(target.Dates) == 0 {
			log.Fatal().Msg("no tramsactions available over period")
		}

		startDate := target.Dates[0]

		target.Table()
		fmt.Printf("Start Date: %s\n", startDate.Format("2006-01-02"))
		fmt.Printf("Next Trade Date: %+v\n", predicted.TradeDate)
		fmt.Printf("Predicted Target: %+v\n", predicted.Target)
		fmt.Printf("Predicted Justification: %+v\n", predicted.Justification)

		pm := portfolio.NewPortfolio("Backtest Portfolio", startDate, 10_000)
		fmt.Println("Building portfolio...")
		if err := pm.TargetPortfolio(ctx, target); err != nil {
			log.Fatal().Stack().Err(err).Msg("could not invest portfolio")
		}

		fmt.Println("Computing performance metrics...")
		perf := portfolio.NewPerformance(pm.Portfolio)
		if err := perf.CalculateThrough(ctx, pm, time.Now()); err != nil {
			log.Fatal().Err(err).Msg("could not calculate portfolio performance")
		}

		for _, meas := range perf.Measurements {
			fmt.Printf("%s\t%.2f\n", meas.Time.Format("2006-01-02"), meas.UlcerIndex)
		}

		fmt.Printf("Ulcer Index Average: %.2f\n", perf.PortfolioMetrics.UlcerIndexAvg)
		fmt.Printf("            P50    : %.2f\n", perf.PortfolioMetrics.UlcerIndexP50)
		fmt.Printf("            P90    : %.2f\n", perf.PortfolioMetrics.UlcerIndexP90)
		fmt.Printf("            P99    : %.2f\n", perf.PortfolioMetrics.UlcerIndexP99)
	},
}
